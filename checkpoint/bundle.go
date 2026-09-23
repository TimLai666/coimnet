package checkpoint

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/TimLai666/coimnet/internal/fileio"
)

// BundleSchemaVersion names the directory format for snapshots too large for the single-file documents.
const BundleSchemaVersion = "coimnet-checkpoint-bundle/v1"

// BundleModelPackage identifies a model package bundle.
// BundleIndividual identifies an individual snapshot bundle.
// BundleTraining identifies a training snapshot bundle.
const (
	BundleModelPackage = "model_package"
	BundleIndividual   = "individual"
	BundleTraining     = "training"
)

// bundlePayloadFile and bundleManifestFile are the only two files of a bundle directory.
const (
	bundlePayloadFile      = "payload.gob"
	bundleManifestFile     = "manifest.json"
	maxBundleManifestBytes = 1 << 20
)

// BundleManifest is manifest.json. It is written last, so a directory without it is an incomplete bundle.
type BundleManifest struct {
	SchemaVersion  string              `json:"schema_version"`
	Kind           string              `json:"kind"`
	DocumentSchema string              `json:"document_schema"`
	Topology       TopologyFingerprint `json:"topology"`
	PayloadFile    string              `json:"payload_file"`
	PayloadBytes   int64               `json:"payload_bytes"`
	PayloadSHA256  string              `json:"payload_sha256"`
}

// bundleByteCounter counts bytes written to a bundle payload.
type bundleByteCounter struct {
	count int64
}

// Write counts bytes and reports the full write as successful.
func (c *bundleByteCounter) Write(p []byte) (int, error) {
	c.count += int64(len(p))
	return len(p), nil
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
	if err := os.Mkdir(dir, 0o755); err != nil {
		return fmt.Errorf("create bundle directory %q: %w", dir, err)
	}
	complete := false
	var payload *os.File
	closed := false
	defer func() {
		if payload != nil && !closed {
			closed = true
			if err := payload.Close(); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("close bundle payload %q: %w", payload.Name(), err))
			}
		}
		if !complete {
			if err := os.RemoveAll(dir); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("remove incomplete bundle directory %q: %w", dir, err))
			}
		}
	}()

	payloadPath := filepath.Join(dir, bundlePayloadFile)
	var err error
	payload, err = os.OpenFile(payloadPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create bundle payload %q: %w", payloadPath, err)
	}
	hasher := sha256.New()
	counter := &bundleByteCounter{}
	writer := bufio.NewWriterSize(io.MultiWriter(payload, hasher, counter), 1<<20)
	if err := gob.NewEncoder(writer).Encode(value); err != nil {
		return fmt.Errorf("encode bundle payload %q: %w", payloadPath, err)
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush bundle payload %q: %w", payloadPath, err)
	}
	if err := payload.Sync(); err != nil {
		return fmt.Errorf("sync bundle payload %q: %w", payloadPath, err)
	}
	if err := payload.Close(); err != nil {
		closed = true
		return fmt.Errorf("close bundle payload %q: %w", payloadPath, err)
	}
	closed = true
	if err := contextError(ctx); err != nil {
		return err
	}
	manifest.SchemaVersion = BundleSchemaVersion
	manifest.PayloadFile = bundlePayloadFile
	manifest.PayloadBytes = counter.count
	manifest.PayloadSHA256 = hex.EncodeToString(hasher.Sum(nil))
	document, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal bundle manifest: %w", err)
	}
	if err := publishDocument(ctx, filepath.Join(dir, bundleManifestFile), document); err != nil {
		return fmt.Errorf("publish bundle manifest: %w", err)
	}
	if err := syncDirectory(ctx, dir); err != nil {
		return fmt.Errorf("sync bundle directory %q: %w", dir, err)
	}
	complete = true
	return nil
}

// readBundle validates a bundle directory and decodes its payload into value.
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
	if manifest.SchemaVersion != BundleSchemaVersion {
		return empty, fmt.Errorf("unsupported bundle schema %q", manifest.SchemaVersion)
	}
	if manifest.Kind != kind {
		return empty, fmt.Errorf("bundle kind %q does not match requested kind %q", manifest.Kind, kind)
	}
	if manifest.PayloadFile != bundlePayloadFile {
		return empty, fmt.Errorf("invalid bundle payload_file %q", manifest.PayloadFile)
	}
	if !validBundleSHA256(manifest.PayloadSHA256) {
		return empty, fmt.Errorf("invalid bundle payload SHA-256 %q", manifest.PayloadSHA256)
	}
	if manifest.PayloadBytes <= 0 {
		return empty, fmt.Errorf("bundle payload size must be positive, got %d", manifest.PayloadBytes)
	}
	if manifest.DocumentSchema == "" {
		return empty, fmt.Errorf("bundle document schema must not be empty")
	}
	payloadPath := filepath.Join(dir, bundlePayloadFile)
	info, err := os.Stat(payloadPath)
	if err != nil {
		return empty, fmt.Errorf("stat bundle payload %q: %w", payloadPath, err)
	}
	if !info.Mode().IsRegular() {
		return empty, fmt.Errorf("bundle payload %q is not a regular file", payloadPath)
	}
	if info.Size() != manifest.PayloadBytes {
		return empty, fmt.Errorf("bundle payload size mismatch: manifest has %d bytes, file has %d bytes", manifest.PayloadBytes, info.Size())
	}
	payload, err := fileio.OpenRegular(ctx, payloadPath)
	if err != nil {
		return empty, fmt.Errorf("open bundle payload for checksum %q: %w", payloadPath, err)
	}
	hasher := sha256.New()
	_, copyErr := io.CopyBuffer(hasher, payload, make([]byte, 1<<20))
	closeErr := payload.Close()
	if copyErr != nil || closeErr != nil {
		return empty, errors.Join(wrapError("read bundle payload for checksum", copyErr), wrapError("close bundle payload after checksum", closeErr))
	}
	if hex.EncodeToString(hasher.Sum(nil)) != manifest.PayloadSHA256 {
		return empty, fmt.Errorf("bundle payload checksum mismatch for %q", payloadPath)
	}
	if err := contextError(ctx); err != nil {
		return empty, err
	}
	payload, err = fileio.OpenRegular(ctx, payloadPath)
	if err != nil {
		return empty, fmt.Errorf("open bundle payload for decoding %q: %w", payloadPath, err)
	}
	decodeErr := gob.NewDecoder(bufio.NewReaderSize(payload, 1<<20)).Decode(value)
	closeErr = payload.Close()
	if decodeErr != nil || closeErr != nil {
		return empty, errors.Join(wrapError(fmt.Sprintf("decode bundle payload %q", payloadPath), decodeErr), wrapError(fmt.Sprintf("close bundle payload %q", payloadPath), closeErr))
	}
	return manifest, nil
}

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
