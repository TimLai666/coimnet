// Package feather reads Feather V2 (Arrow IPC file) sources in bounded,
// sequential record batches without materializing a table or modifying the
// source file.
package feather

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/TimLai666/coimnet/internal/fileio"
	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/ipc"
)

const (
	// SchemaVersion identifies the public Scan report.
	SchemaVersion = "coimnet-feather-report/v1"

	arrowMagicBytes  = "ARROW1"
	footerTrailerLen = len(arrowMagicBytes) + 4
	maxTypeDepth     = 64
)

// Options contains mandatory limits for one scan. All limits must be
// positive. MaxArrowBytes counts bytes requested from Arrow's allocator; it
// excludes other process memory and includes temporary realloc buffers.
type Options struct {
	MaxFileBytes   int64
	MaxFooterBytes int64
	MaxArrowBytes  int64
	MaxRows        int64
}

// Field describes one source schema field without changing its Arrow type.
type Field struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
}

// Report is the result accumulated by Scan. RecordBatches and Rows count
// batches successfully consumed by the callback, or all decoded batches when
// visit is nil. On an error Complete is false and the preceding counts are
// retained.
type Report struct {
	SchemaVersion  string  `json:"schema_version"`
	Format         string  `json:"format"`
	Fields         []Field `json:"fields"`
	RecordBatches  int64   `json:"record_batches"`
	Rows           int64   `json:"rows"`
	FileBytes      int64   `json:"file_bytes"`
	PeakArrowBytes int64   `json:"peak_arrow_bytes"`
	Complete       bool    `json:"complete"`
}

// Scan validates and reads a Feather V2 / Arrow IPC file. The callback sees a
// borrowed record that Scan releases after the callback returns. A callback
// that needs to keep it must call record.Retain and later Release it. Scan
// never calls Release on the callback's behalf for a retained reference.
// A nil callback performs the same validation and batch decoding while only
// returning the report.
func Scan(ctx context.Context, path string, options Options, visit func(arrow.Record) error) (report Report, retErr error) {
	report = Report{SchemaVersion: SchemaVersion, Format: "unknown"}
	if ctx == nil {
		return report, errors.New("feather: nil context")
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if err := validateOptions(options); err != nil {
		return report, err
	}

	file, err := fileio.OpenRegular(ctx, path)
	if err != nil {
		return report, fmt.Errorf("feather: open source: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			report.Complete = false
			retErr = errors.Join(retErr, fmt.Errorf("feather: close source: %w", closeErr))
		}
	}()

	if err := checkContext(ctx); err != nil {
		return report, err
	}
	info, err := file.Stat()
	if err != nil {
		return report, fmt.Errorf("feather: stat source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return report, fmt.Errorf("feather: source is not a regular file")
	}
	if info.Size() < 0 {
		return report, fmt.Errorf("feather: source has negative size")
	}
	report.FileBytes = info.Size()
	if report.FileBytes > options.MaxFileBytes {
		return report, fmt.Errorf("feather: file size %d exceeds limit %d", report.FileBytes, options.MaxFileBytes)
	}

	if _, err := validateLayout(ctx, file, report.FileBytes, options.MaxFooterBytes); err != nil {
		return report, err
	}

	alloc := newBoundedAllocator(options.MaxArrowBytes)
	defer func() {
		report.PeakArrowBytes = alloc.Peak()
		if retErr != nil {
			report.Complete = false
		}
	}()

	reader, err := newFileReader(file, alloc)
	if err != nil {
		return report, err
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			report.Complete = false
			retErr = errors.Join(retErr, fmt.Errorf("feather: close Arrow reader: %w", closeErr))
		}
	}()

	if version := reader.Version(); version != ipc.MetadataV4 && version != ipc.MetadataV5 {
		return report, fmt.Errorf("feather: unsupported Arrow metadata version %v", version)
	}
	fields, err := reportFields(reader.Schema())
	if err != nil {
		return report, err
	}
	report.Format = "feather_v2"
	report.Fields = fields

	for batch := 0; batch < reader.NumRecords(); batch++ {
		if err := checkContext(ctx); err != nil {
			return report, err
		}
		record, err := recordAt(reader, batch, alloc)
		if err != nil {
			return report, fmt.Errorf("feather: read record batch %d: %w", batch, err)
		}
		batchErr := func() error {
			defer record.Release()
			if err := validateRecord(ctx, record); err != nil {
				return fmt.Errorf("feather: validate record batch %d: %w", batch, err)
			}
			rows := record.NumRows()
			if rows < 0 {
				return fmt.Errorf("feather: record batch %d has negative row count %d", batch, rows)
			}
			if rows > options.MaxRows-report.Rows {
				return fmt.Errorf("feather: row limit %d exceeded by batch %d", options.MaxRows, batch)
			}
			if visit != nil {
				if err := visit(record); err != nil {
					return fmt.Errorf("feather: callback at batch %d: %w", batch, err)
				}
			}
			if err := checkContext(ctx); err != nil {
				return err
			}
			report.Rows += rows
			report.RecordBatches++
			return nil
		}()
		if batchErr != nil {
			return report, batchErr
		}
	}

	if err := checkContext(ctx); err != nil {
		return report, err
	}
	report.Complete = true
	return report, nil
}

func validateOptions(options Options) error {
	if options.MaxFileBytes <= 0 || options.MaxFooterBytes <= 0 || options.MaxArrowBytes <= 0 || options.MaxRows <= 0 {
		return fmt.Errorf("feather: MaxFileBytes, MaxFooterBytes, MaxArrowBytes and MaxRows must all be positive")
	}
	return nil
}

func checkContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("feather: context: %w", err)
	}
	return nil
}

func validateLayout(ctx context.Context, file io.ReaderAt, size, maxFooterBytes int64) (int64, error) {
	if size < int64(footerTrailerLen+len(arrowMagicBytes)) {
		return 0, fmt.Errorf("feather: file is too small for Arrow IPC footer")
	}
	magic := []byte(arrowMagicBytes)
	head := make([]byte, len(magic))
	if err := readAt(ctx, file, head, 0, "header magic"); err != nil {
		return 0, err
	}
	if !bytes.Equal(head, magic) {
		return 0, fmt.Errorf("feather: invalid header magic %q", string(head))
	}
	trailer := make([]byte, footerTrailerLen)
	if err := readAt(ctx, file, trailer, size-int64(footerTrailerLen), "footer trailer"); err != nil {
		return 0, err
	}
	if !bytes.Equal(trailer[4:], magic) {
		return 0, fmt.Errorf("feather: invalid footer magic %q", string(trailer[4:]))
	}
	footerBytes := int64(binary.LittleEndian.Uint32(trailer[:4]))
	if footerBytes <= 0 {
		return 0, fmt.Errorf("feather: invalid footer length %d", footerBytes)
	}
	if footerBytes > maxFooterBytes {
		return 0, fmt.Errorf("feather: footer length %d exceeds limit %d", footerBytes, maxFooterBytes)
	}
	footerStart := size - int64(footerTrailerLen) - footerBytes
	if footerStart < int64(len(magic)) {
		return 0, fmt.Errorf("feather: footer length %d exceeds file layout", footerBytes)
	}
	return footerBytes, nil
}

func readAt(ctx context.Context, file io.ReaderAt, buf []byte, offset int64, label string) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	count, err := file.ReadAt(buf, offset)
	if contextErr := checkContext(ctx); contextErr != nil {
		return contextErr
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("feather: read %s: %w", label, err)
	}
	if count != len(buf) {
		return fmt.Errorf("feather: read %s returned %d bytes, want %d", label, count, len(buf))
	}
	return nil
}

func newFileReader(file ipc.ReadAtSeeker, alloc *boundedAllocator) (reader *ipc.FileReader, retErr error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			reader = nil
			if allocErr := alloc.Error(); allocErr != nil {
				retErr = allocErr
			} else {
				retErr = fmt.Errorf("feather: Arrow parser panic while opening file: %v", recovered)
			}
		}
	}()
	reader, err := ipc.NewFileReader(file, ipc.WithAllocator(alloc))
	if err != nil {
		if allocErr := alloc.Error(); allocErr != nil {
			return nil, allocErr
		}
		return nil, fmt.Errorf("feather: decode Arrow file: %w", err)
	}
	if allocErr := alloc.Error(); allocErr != nil {
		_ = reader.Close()
		return nil, allocErr
	}
	return reader, nil
}

func recordAt(reader *ipc.FileReader, batch int, alloc *boundedAllocator) (record arrow.Record, retErr error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			record = nil
			if allocErr := alloc.Error(); allocErr != nil {
				retErr = allocErr
			} else {
				retErr = fmt.Errorf("feather: Arrow parser panic in record batch %d: %v", batch, recovered)
			}
		}
	}()
	record, err := reader.RecordAt(batch)
	if err != nil {
		if allocErr := alloc.Error(); allocErr != nil {
			return nil, allocErr
		}
		return nil, err
	}
	if allocErr := alloc.Error(); allocErr != nil {
		record.Release()
		return nil, allocErr
	}
	return record, nil
}

func reportFields(schema *arrow.Schema) ([]Field, error) {
	if schema == nil {
		return nil, errors.New("feather: Arrow schema is missing")
	}
	fields := schema.Fields()
	seen := make(map[string]struct{}, len(fields))
	report := make([]Field, len(fields))
	for i, field := range fields {
		if _, ok := seen[field.Name]; ok {
			return nil, fmt.Errorf("feather: duplicate field name %q", field.Name)
		}
		seen[field.Name] = struct{}{}
		typeName, err := supportedType(field.Type, 0)
		if err != nil {
			return nil, fmt.Errorf("feather: field %q: %w", field.Name, err)
		}
		report[i] = Field{Name: field.Name, Type: typeName, Nullable: field.Nullable}
	}
	return report, nil
}

func supportedType(dataType arrow.DataType, depth int) (typeName string, retErr error) {
	if depth > maxTypeDepth {
		return "", fmt.Errorf("unsupported Arrow type nesting beyond %d levels", maxTypeDepth)
	}
	id, typeName, err := dataTypeDetails(dataType)
	if err != nil {
		return "", err
	}
	switch id {
	case arrow.INT8, arrow.INT32, arrow.INT64, arrow.UINT64, arrow.FLOAT64, arrow.STRING:
		return typeName, nil
	case arrow.LIST:
		list, ok := dataType.(arrow.ListLikeType)
		if !ok {
			return "", fmt.Errorf("unsupported Arrow list implementation %T", dataType)
		}
		elem, err := listElement(list)
		if err != nil {
			return "", err
		}
		if _, err := supportedType(elem, depth+1); err != nil {
			return "", fmt.Errorf("unsupported list element: %w", err)
		}
		return typeName, nil
	case arrow.DICTIONARY:
		dictionary, ok := dataType.(*arrow.DictionaryType)
		if !ok || dictionary == nil {
			return "", fmt.Errorf("unsupported Arrow dictionary implementation %T", dataType)
		}
		if !integerType(dictionary.IndexType) {
			return "", fmt.Errorf("unsupported dictionary index type %v", dictionary.IndexType)
		}
		if _, err := supportedType(dictionary.ValueType, depth+1); err != nil {
			return "", fmt.Errorf("unsupported dictionary value: %w", err)
		}
		return typeName, nil
	default:
		return "", fmt.Errorf("unsupported Arrow type %q", typeName)
	}
}

func dataTypeDetails(dataType arrow.DataType) (id arrow.Type, typeName string, retErr error) {
	if dataType == nil {
		return 0, "", errors.New("unsupported nil Arrow type")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			retErr = fmt.Errorf("unsupported Arrow type implementation: %v", recovered)
		}
	}()
	return dataType.ID(), dataType.String(), nil
}

func listElement(list arrow.ListLikeType) (element arrow.DataType, retErr error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			retErr = fmt.Errorf("unsupported Arrow list element: %v", recovered)
		}
	}()
	element = list.Elem()
	if element == nil {
		return nil, errors.New("unsupported nil Arrow list element")
	}
	return element, nil
}

func integerType(dataType arrow.DataType) bool {
	if dataType == nil {
		return false
	}
	id, _, err := dataTypeDetails(dataType)
	if err != nil {
		return false
	}
	switch id {
	case arrow.UINT8, arrow.UINT16, arrow.UINT32, arrow.UINT64,
		arrow.INT8, arrow.INT16, arrow.INT32, arrow.INT64:
		return true
	default:
		return false
	}
}
