package checkpoint

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/TimLai666/coimnet/internal/fileio"
)

// BundleSchemaVersion names the directory format for checkpoint bundles.
const BundleSchemaVersion = "coimnet-checkpoint-bundle/v2"

// BundleModelPackage identifies a model package bundle.
// BundleIndividual identifies an individual snapshot bundle.
// BundleTraining identifies a training snapshot bundle.
const (
	BundleModelPackage = "model_package"
	BundleIndividual   = "individual"
	BundleTraining     = "training"
)

const (
	bundleDocumentFile     = "document.json"
	bundleArraysFile       = "arrays.bin"
	bundleManifestFile     = "manifest.json"
	bundleArrayThreshold   = 4096
	maxBundleManifestBytes = 1 << 20
	maxBundleDocumentBytes = 256 << 20
	bundleBufferBytes      = 1 << 20
)

// BundleFile records the byte length and checksum of one bundle file.
type BundleFile struct {
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// BundleArray describes one extracted one-dimensional numeric array.
type BundleArray struct {
	Path   string `json:"path"`
	DType  string `json:"dtype"`
	Length int64  `json:"length"`
	Offset int64  `json:"offset"`
	SHA256 string `json:"sha256"`
}

// BundleManifest is manifest.json. It is written last, so a directory without it is incomplete.
type BundleManifest struct {
	SchemaVersion  string              `json:"schema_version"`
	Kind           string              `json:"kind"`
	DocumentSchema string              `json:"document_schema"`
	Topology       TopologyFingerprint `json:"topology"`
	Document       BundleFile          `json:"document"`
	Arrays         BundleFile          `json:"arrays"`
	Entries        []BundleArray       `json:"entries"`
}

type bundleArrayValue struct {
	entry BundleArray
	value reflect.Value
}

// writeBundle publishes value as an exclusive, checksummed bundle directory.
func writeBundle(ctx context.Context, dir string, manifest BundleManifest, value any) (retErr error) {
	if err := contextError(ctx); err != nil {
		return err
	}
	if !validBundleKind(manifest.Kind) {
		return fmt.Errorf("invalid bundle kind %q", manifest.Kind)
	}
	if manifest.DocumentSchema == "" {
		return fmt.Errorf("bundle document schema must not be empty")
	}
	if dir == "" {
		return fmt.Errorf("bundle directory path must not be empty")
	}
	if value == nil {
		return fmt.Errorf("bundle value must not be nil")
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		return fmt.Errorf("create bundle directory %q: %w", dir, err)
	}
	complete := false
	var arraysFile *os.File
	arraysClosed := false
	defer func() {
		if arraysFile != nil && !arraysClosed {
			arraysClosed = true
			if err := arraysFile.Close(); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("close bundle arrays %q: %w", arraysFile.Name(), err))
			}
		}
		if !complete {
			if err := os.RemoveAll(dir); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("remove incomplete bundle directory %q: %w", dir, err))
			}
		}
	}()

	arrays := make([]bundleArrayValue, 0)
	skeleton := bundleSkeleton(reflect.ValueOf(value), "", &arrays)
	document, err := json.Marshal(skeleton.Interface())
	if err != nil {
		return fmt.Errorf("marshal bundle document: %w", err)
	}
	if len(document) > maxBundleDocumentBytes {
		return fmt.Errorf("bundle document exceeds %d byte limit", maxBundleDocumentBytes)
	}
	manifest.SchemaVersion = BundleSchemaVersion
	manifest.Entries = make([]BundleArray, len(arrays))

	arraysPath := filepath.Join(dir, bundleArraysFile)
	arraysFile, err = os.OpenFile(arraysPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create bundle arrays %q: %w", arraysPath, err)
	}
	fileHash := sha256.New()
	writer := bufio.NewWriterSize(io.MultiWriter(arraysFile, fileHash), bundleBufferBytes)
	var offset int64
	for i, array := range arrays {
		entry := array.entry
		entry.Offset = offset
		entryHash := sha256.New()
		written, err := writeBundleArray(ctx, writer, entryHash, array.value)
		if err != nil {
			return fmt.Errorf("write bundle array %q: %w", entry.Path, err)
		}
		if err := writer.Flush(); err != nil {
			return fmt.Errorf("flush bundle arrays after %q: %w", entry.Path, err)
		}
		offset += written
		entry.SHA256 = hex.EncodeToString(entryHash.Sum(nil))
		manifest.Entries[i] = entry
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush bundle arrays: %w", err)
	}
	if err := arraysFile.Sync(); err != nil {
		return fmt.Errorf("sync bundle arrays %q: %w", arraysPath, err)
	}
	if err := arraysFile.Close(); err != nil {
		arraysClosed = true
		return fmt.Errorf("close bundle arrays %q: %w", arraysPath, err)
	}
	arraysClosed = true
	manifest.Arrays = BundleFile{Name: bundleArraysFile, Bytes: offset, SHA256: hex.EncodeToString(fileHash.Sum(nil))}

	documentPath := filepath.Join(dir, bundleDocumentFile)
	if err := writeBundleDocument(ctx, documentPath, document); err != nil {
		return err
	}
	manifest.Document = BundleFile{Name: bundleDocumentFile, Bytes: int64(len(document)), SHA256: digestHex(document)}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal bundle manifest: %w", err)
	}
	if len(manifestBytes) > maxBundleManifestBytes {
		return fmt.Errorf("bundle manifest exceeds %d byte limit", maxBundleManifestBytes)
	}
	if err := publishDocument(ctx, filepath.Join(dir, bundleManifestFile), manifestBytes); err != nil {
		return fmt.Errorf("publish bundle manifest: %w", err)
	}
	if err := syncDirectory(ctx, dir); err != nil {
		return fmt.Errorf("sync bundle directory %q: %w", dir, err)
	}
	complete = true
	return nil
}

// readBundle validates a bundle directory and decodes its document and arrays into value.
func readBundle(ctx context.Context, dir, kind string, value any) (BundleManifest, error) {
	var empty BundleManifest
	if err := contextError(ctx); err != nil {
		return empty, err
	}
	if !validBundleKind(kind) {
		return empty, fmt.Errorf("invalid requested bundle kind %q", kind)
	}
	if value == nil || reflect.ValueOf(value).Kind() != reflect.Pointer || reflect.ValueOf(value).IsNil() {
		return empty, fmt.Errorf("bundle destination must be a non-nil pointer")
	}
	manifestPath := filepath.Join(dir, bundleManifestFile)
	data, err := fileio.ReadRegular(ctx, manifestPath, maxBundleManifestBytes)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return empty, fmt.Errorf("incomplete bundle %q: manifest is missing", dir)
		}
		return empty, fmt.Errorf("read bundle manifest %q: %w", manifestPath, err)
	}
	if err := checkUniqueJSON(data); err != nil {
		return empty, fmt.Errorf("invalid bundle manifest %q: %w", manifestPath, err)
	}
	var manifest BundleManifest
	if err := decodeStrict(data, &manifest); err != nil {
		return empty, fmt.Errorf("decode bundle manifest %q: %w", manifestPath, err)
	}
	if err := validateBundleManifest(manifest, kind); err != nil {
		return empty, err
	}
	documentPath := filepath.Join(dir, bundleDocumentFile)
	document, err := fileio.ReadRegular(ctx, documentPath, maxBundleDocumentBytes)
	if err != nil {
		return empty, fmt.Errorf("read bundle document %q: %w", documentPath, err)
	}
	if int64(len(document)) != manifest.Document.Bytes {
		return empty, fmt.Errorf("bundle document size mismatch: manifest has %d bytes, file has %d bytes", manifest.Document.Bytes, len(document))
	}
	if digestHex(document) != manifest.Document.SHA256 {
		return empty, fmt.Errorf("bundle document checksum mismatch for %q", documentPath)
	}
	arraysPath := filepath.Join(dir, bundleArraysFile)
	if err := verifyBundleFile(ctx, arraysPath, manifest.Arrays); err != nil {
		return empty, err
	}
	if err := checkUniqueJSON(document); err != nil {
		return empty, fmt.Errorf("invalid bundle document %q: %w", documentPath, err)
	}
	if err := decodeStrict(document, value); err != nil {
		return empty, fmt.Errorf("decode bundle document %q: %w", documentPath, err)
	}
	if err := restoreBundleArrays(ctx, arraysPath, manifest.Entries, reflect.ValueOf(value).Elem()); err != nil {
		return empty, err
	}
	return manifest, nil
}

func validateBundleManifest(manifest BundleManifest, kind string) error {
	if manifest.SchemaVersion != BundleSchemaVersion {
		return fmt.Errorf("unsupported bundle schema %q", manifest.SchemaVersion)
	}
	if manifest.Kind != kind {
		return fmt.Errorf("bundle kind %q does not match requested kind %q", manifest.Kind, kind)
	}
	if manifest.DocumentSchema == "" {
		return fmt.Errorf("bundle document schema must not be empty")
	}
	if manifest.Document.Name != bundleDocumentFile || manifest.Arrays.Name != bundleArraysFile {
		return fmt.Errorf("invalid bundle file names %q and %q", manifest.Document.Name, manifest.Arrays.Name)
	}
	for _, file := range []BundleFile{manifest.Document, manifest.Arrays} {
		if file.Bytes < 0 || file.Name == bundleDocumentFile && file.Bytes == 0 || !validBundleSHA256(file.SHA256) {
			return fmt.Errorf("invalid bundle file metadata for %q", file.Name)
		}
	}
	if manifest.Entries == nil {
		return fmt.Errorf("bundle entries must be an array")
	}
	var offset int64
	seen := make(map[string]struct{}, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		if entry.Path == "" {
			return fmt.Errorf("bundle array path must not be empty")
		}
		if _, ok := seen[entry.Path]; ok {
			return fmt.Errorf("duplicate bundle array path %q", entry.Path)
		}
		seen[entry.Path] = struct{}{}
		if !knownBundleDType(entry.DType) {
			return fmt.Errorf("unknown bundle array dtype %q for %q", entry.DType, entry.Path)
		}
		if entry.Offset != offset {
			return fmt.Errorf("bundle array %q offset is %d, want %d", entry.Path, entry.Offset, offset)
		}
		if entry.Length < bundleArrayThreshold || !validBundleSHA256(entry.SHA256) {
			return fmt.Errorf("invalid bundle array metadata for %q", entry.Path)
		}
		byteLength, ok := bundleArrayBytes(entry.DType, entry.Length)
		if !ok || offset > math.MaxInt64-byteLength {
			return fmt.Errorf("bundle array size overflows for %q", entry.Path)
		}
		offset += byteLength
	}
	if offset != manifest.Arrays.Bytes {
		return fmt.Errorf("bundle arrays size mismatch: entries total %d bytes, manifest has %d", offset, manifest.Arrays.Bytes)
	}
	return nil
}

func verifyBundleFile(ctx context.Context, path string, file BundleFile) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat bundle file %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("bundle file %q is not a regular file", path)
	}
	if info.Size() != file.Bytes {
		return fmt.Errorf("bundle file %q size mismatch: manifest has %d bytes, file has %d bytes", path, file.Bytes, info.Size())
	}
	reader, err := fileio.OpenRegular(ctx, path)
	if err != nil {
		return fmt.Errorf("open bundle file for checksum %q: %w", path, err)
	}
	hasher := sha256.New()
	_, copyErr := io.CopyBuffer(hasher, reader, make([]byte, bundleBufferBytes))
	closeErr := reader.Close()
	if copyErr != nil || closeErr != nil {
		return errors.Join(wrapError("read bundle file for checksum", copyErr), wrapError("close bundle file after checksum", closeErr))
	}
	if hex.EncodeToString(hasher.Sum(nil)) != file.SHA256 {
		return fmt.Errorf("bundle %s checksum mismatch for %q", file.Name, path)
	}
	return contextError(ctx)
}

func restoreBundleArrays(ctx context.Context, path string, entries []BundleArray, root reflect.Value) (retErr error) {
	file, err := fileio.OpenRegular(ctx, path)
	if err != nil {
		return fmt.Errorf("open bundle arrays %q: %w", path, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close bundle arrays %q: %w", path, err))
		}
	}()
	for _, entry := range entries {
		if err := contextError(ctx); err != nil {
			return err
		}
		field, ok := findBundleArrayField(root, strings.Split(entry.Path, "."))
		if !ok {
			return fmt.Errorf("bundle array path %q was not found", entry.Path)
		}
		if field.Kind() != reflect.Slice || bundleDType(field.Type().Elem().Kind()) != entry.DType || field.Len() != 0 {
			return fmt.Errorf("bundle array path %q has incompatible type or skeleton value", entry.Path)
		}
		byteLength, _ := bundleArrayBytes(entry.DType, entry.Length)
		if int64(int(entry.Length)) != entry.Length {
			return fmt.Errorf("bundle array %q length exceeds platform limits", entry.Path)
		}
		if _, err := file.Seek(entry.Offset, io.SeekStart); err != nil {
			return fmt.Errorf("seek bundle array %q: %w", entry.Path, err)
		}
		arrayHash := sha256.New()
		decoded, err := readBundleArray(ctx, io.TeeReader(io.LimitReader(file, byteLength), arrayHash), field.Type(), int(entry.Length))
		if err != nil {
			return fmt.Errorf("read bundle array %q: %w", entry.Path, err)
		}
		if hex.EncodeToString(arrayHash.Sum(nil)) != entry.SHA256 {
			return fmt.Errorf("bundle array entry %q checksum mismatch", entry.Path)
		}
		field.Set(decoded)
	}
	return nil
}

// bundleSkeleton walks exported JSON fields and copies only structs and struct pointers on paths it changes.
func bundleSkeleton(value reflect.Value, path string, arrays *[]bundleArrayValue) reflect.Value {
	if value.Kind() == reflect.Pointer {
		if value.IsNil() || value.Type().Elem().Kind() != reflect.Struct {
			return value
		}
		child := bundleSkeleton(value.Elem(), path, arrays)
		if child.Kind() == reflect.Struct && bundleValueChanged(child, value.Elem()) {
			copy := reflect.New(value.Type().Elem())
			copy.Elem().Set(child)
			return copy
		}
		return value
	}
	if value.Kind() != reflect.Struct {
		return value
	}
	copy := reflect.New(value.Type()).Elem()
	copy.Set(value)
	changed := false
	typ := value.Type()
	for i := 0; i < value.NumField(); i++ {
		fieldInfo := typ.Field(i)
		if fieldInfo.PkgPath != "" {
			continue
		}
		name, ok := bundleJSONFieldName(fieldInfo)
		if !ok {
			continue
		}
		field := value.Field(i)
		fieldPath := name
		if path != "" {
			fieldPath = path + "." + name
		}
		if field.Kind() == reflect.Slice && field.Len() >= bundleArrayThreshold {
			if dtype := bundleDType(field.Type().Elem().Kind()); dtype != "" {
				*arrays = append(*arrays, bundleArrayValue{entry: BundleArray{Path: fieldPath, DType: dtype, Length: int64(field.Len())}, value: field})
				copy.Field(i).Set(reflect.MakeSlice(field.Type(), 0, 0))
				changed = true
				continue
			}
		}
		if field.Kind() == reflect.Struct || field.Kind() == reflect.Pointer && !field.IsNil() && field.Type().Elem().Kind() == reflect.Struct {
			child := bundleSkeleton(field, fieldPath, arrays)
			if bundleValueChanged(child, field) {
				copy.Field(i).Set(child)
				changed = true
			}
		}
	}
	if !changed {
		return value
	}
	return copy
}

func bundleValueChanged(a, b reflect.Value) bool {
	if a.Type() != b.Type() {
		return true
	}
	switch a.Kind() {
	case reflect.Pointer:
		if a.IsNil() || b.IsNil() {
			return a.IsNil() != b.IsNil()
		}
		return bundleValueChanged(a.Elem(), b.Elem())
	case reflect.Struct:
		for i := 0; i < a.NumField(); i++ {
			switch a.Field(i).Kind() {
			case reflect.Struct, reflect.Pointer:
				if bundleValueChanged(a.Field(i), b.Field(i)) {
					return true
				}
			case reflect.Slice:
				if a.Field(i).Len() != b.Field(i).Len() {
					return true
				}
			}
		}
	}
	return false
}

func bundleJSONFieldName(field reflect.StructField) (string, bool) {
	tag, tagged := field.Tag.Lookup("json")
	if tagged {
		name := strings.Split(tag, ",")[0]
		if name == "-" {
			return "", false
		}
		if name != "" {
			return name, true
		}
	}
	return field.Name, true
}

func findBundleArrayField(root reflect.Value, path []string) (reflect.Value, bool) {
	current := root
	for _, part := range path {
		for current.Kind() == reflect.Pointer {
			if current.IsNil() || current.Type().Elem().Kind() != reflect.Struct {
				return reflect.Value{}, false
			}
			current = current.Elem()
		}
		if current.Kind() != reflect.Struct {
			return reflect.Value{}, false
		}
		found := false
		for i := 0; i < current.NumField(); i++ {
			field := current.Type().Field(i)
			name, ok := bundleJSONFieldName(field)
			if ok && field.PkgPath == "" && name == part {
				current = current.Field(i)
				found = true
				break
			}
		}
		if !found {
			return reflect.Value{}, false
		}
	}
	return current, true
}

func writeBundleArray(ctx context.Context, writer io.Writer, hasher io.Writer, value reflect.Value) (int64, error) {
	width, ok := bundleDTypeWidth(bundleDType(value.Type().Elem().Kind()))
	if !ok {
		return 0, fmt.Errorf("unsupported array element type %s", value.Type().Elem())
	}
	buffer := make([]byte, bundleBufferBytes)
	used := 0
	var total int64
	flush := func() error {
		if used == 0 {
			return nil
		}
		if _, err := hasher.Write(buffer[:used]); err != nil {
			return err
		}
		n, err := writer.Write(buffer[:used])
		if err != nil {
			return err
		}
		if n != used {
			return io.ErrShortWrite
		}
		total += int64(used)
		used = 0
		return nil
	}
	for i := 0; i < value.Len(); i++ {
		if used+width > len(buffer) {
			if err := flush(); err != nil {
				return total, err
			}
		}
		dst := buffer[used : used+width]
		elem := value.Index(i)
		switch elem.Kind() {
		case reflect.Float64:
			binary.LittleEndian.PutUint64(dst, math.Float64bits(elem.Float()))
		case reflect.Float32:
			binary.LittleEndian.PutUint32(dst, math.Float32bits(float32(elem.Float())))
		case reflect.Int, reflect.Int64:
			binary.LittleEndian.PutUint64(dst, uint64(elem.Int()))
		case reflect.Int32:
			binary.LittleEndian.PutUint32(dst, uint32(elem.Int()))
		case reflect.Int8:
			dst[0] = byte(elem.Int())
		case reflect.Uint64:
			binary.LittleEndian.PutUint64(dst, elem.Uint())
		case reflect.Uint32:
			binary.LittleEndian.PutUint32(dst, uint32(elem.Uint()))
		case reflect.Uint8:
			dst[0] = byte(elem.Uint())
		case reflect.Bool:
			if elem.Bool() {
				dst[0] = 1
			} else {
				dst[0] = 0
			}
		default:
			return total, fmt.Errorf("unsupported array element type %s", elem.Type())
		}
		used += width
		if used == len(buffer) {
			if err := contextError(ctx); err != nil {
				return total, err
			}
			if err := flush(); err != nil {
				return total, err
			}
		}
	}
	if err := contextError(ctx); err != nil {
		return total, err
	}
	if err := flush(); err != nil {
		return total, err
	}
	return total, nil
}

func readBundleArray(ctx context.Context, reader io.Reader, sliceType reflect.Type, length int) (reflect.Value, error) {
	value := reflect.MakeSlice(sliceType, length, length)
	width, ok := bundleDTypeWidth(bundleDType(sliceType.Elem().Kind()))
	if !ok {
		return reflect.Value{}, fmt.Errorf("unsupported array element type %s", sliceType.Elem())
	}
	buffer := make([]byte, bundleBufferBytes)
	for start := 0; start < length; {
		if err := contextError(ctx); err != nil {
			return reflect.Value{}, err
		}
		count := len(buffer) / width
		if count > length-start {
			count = length - start
		}
		n := count * width
		if _, err := io.ReadFull(reader, buffer[:n]); err != nil {
			return reflect.Value{}, err
		}
		for i := 0; i < count; i++ {
			src := buffer[i*width : (i+1)*width]
			var decoded reflect.Value
			switch sliceType.Elem().Kind() {
			case reflect.Float64:
				decoded = reflect.ValueOf(math.Float64frombits(binary.LittleEndian.Uint64(src)))
			case reflect.Float32:
				decoded = reflect.ValueOf(math.Float32frombits(binary.LittleEndian.Uint32(src)))
			case reflect.Int, reflect.Int64:
				decoded = reflect.ValueOf(int64(binary.LittleEndian.Uint64(src)))
			case reflect.Int32:
				decoded = reflect.ValueOf(int32(binary.LittleEndian.Uint32(src)))
			case reflect.Int8:
				decoded = reflect.ValueOf(int8(src[0]))
			case reflect.Uint64:
				decoded = reflect.ValueOf(binary.LittleEndian.Uint64(src))
			case reflect.Uint32:
				decoded = reflect.ValueOf(binary.LittleEndian.Uint32(src))
			case reflect.Uint8:
				decoded = reflect.ValueOf(src[0])
			case reflect.Bool:
				if src[0] > 1 {
					return reflect.Value{}, fmt.Errorf("invalid boolean byte %d", src[0])
				}
				decoded = reflect.ValueOf(src[0] == 1)
			}
			if decoded.Type() != sliceType.Elem() {
				if sliceType.Elem().Kind() == reflect.Int && reflect.Zero(sliceType.Elem()).OverflowInt(decoded.Int()) {
					return reflect.Value{}, fmt.Errorf("integer value overflows %s", sliceType.Elem())
				}
				decoded = decoded.Convert(sliceType.Elem())
			}
			value.Index(start + i).Set(decoded)
		}
		start += count
	}
	return value, nil
}

func bundleDType(kind reflect.Kind) string {
	switch kind {
	case reflect.Float64:
		return "float64"
	case reflect.Float32:
		return "float32"
	case reflect.Int, reflect.Int64:
		return "int64"
	case reflect.Int32:
		return "int32"
	case reflect.Int8:
		return "int8"
	case reflect.Uint64:
		return "uint64"
	case reflect.Uint32:
		return "uint32"
	case reflect.Uint8:
		return "uint8"
	case reflect.Bool:
		return "bool"
	default:
		return ""
	}
}

func bundleDTypeWidth(dtype string) (int, bool) {
	switch dtype {
	case "float64", "int64", "uint64":
		return 8, true
	case "float32", "int32", "uint32":
		return 4, true
	case "int8", "uint8", "bool":
		return 1, true
	default:
		return 0, false
	}
}

func knownBundleDType(dtype string) bool { _, ok := bundleDTypeWidth(dtype); return ok }

func bundleArrayBytes(dtype string, length int64) (int64, bool) {
	width, ok := bundleDTypeWidth(dtype)
	if !ok || length < 0 || length > math.MaxInt64/int64(width) {
		return 0, false
	}
	return length * int64(width), true
}

func writeBundleDocument(ctx context.Context, path string, document []byte) (retErr error) {
	if err := contextError(ctx); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create bundle document %q: %w", path, err)
	}
	closed := false
	defer func() {
		if !closed {
			if err := file.Close(); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("close bundle document %q: %w", path, err))
			}
		}
	}()
	if err := writeContext(ctx, file, document); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync bundle document %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		closed = true
		return fmt.Errorf("close bundle document %q: %w", path, err)
	}
	closed = true
	return nil
}

func digestHex(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// validBundleKind reports whether kind names one of the supported bundle kinds.
func validBundleKind(kind string) bool {
	switch kind {
	case BundleModelPackage, BundleIndividual, BundleTraining:
		return true
	default:
		return false
	}
}

// validBundleSHA256 reports whether digest is 64 lowercase hexadecimal characters.
func validBundleSHA256(digest string) bool {
	if len(digest) != sha256.Size*2 || strings.ToLower(digest) != digest {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}
