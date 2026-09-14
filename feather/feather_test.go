package feather

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
	"github.com/apache/arrow/go/v17/arrow/ipc"
	"github.com/apache/arrow/go/v17/arrow/memory"
)

func TestScanReportsSchemaRowsAndRetainLifecycle(t *testing.T) {
	path := writeRichFixture(t)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	var batches int
	var retained arrow.Record
	report, err := Scan(context.Background(), path, Options{
		MaxFileBytes:   info.Size(),
		MaxFooterBytes: 1 << 20,
		MaxArrowBytes:  1 << 20,
		MaxRows:        10,
	}, func(record arrow.Record) error {
		batches++
		if batches == 1 {
			record.Retain()
			retained = record
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || report.Format != "feather_v2" || report.RecordBatches != 2 || report.Rows != 4 || report.FileBytes != info.Size() {
		t.Fatalf("report = %#v", report)
	}
	if report.PeakArrowBytes <= 0 || len(report.Fields) != 8 {
		t.Fatalf("report memory/fields = %#v", report)
	}
	wantTypes := []string{"int8", "int32", "int64", "uint64", "float64", "utf8", "list<item: int32, nullable>", "dictionary<values=utf8, indices=int32, ordered=false>"}
	for i, field := range report.Fields {
		if field.Name == "" || field.Type != wantTypes[i] {
			t.Fatalf("field[%d] = %#v, want name and type %q", i, field, wantTypes[i])
		}
	}
	if !report.Fields[0].Nullable || !report.Fields[7].Nullable {
		t.Fatalf("nullable schema fields were lost: %#v", report.Fields)
	}
	if batches != 2 {
		t.Fatalf("callback batches = %d, want 2", batches)
	}
	if retained == nil || retained.NumRows() != 3 {
		t.Fatalf("retained record = %#v", retained)
	}
	large := retained.Column(2).(*array.Int64)
	unsigned := retained.Column(3).(*array.Uint64)
	if large.Value(0) != 9007199254740993 || unsigned.Value(0) != 9007199254740995 {
		t.Fatalf("large integers = %d and %d", large.Value(0), unsigned.Value(0))
	}
	i32 := retained.Column(1).(*array.Int32)
	if !i32.IsNull(1) || i32.IsNull(0) || i32.Value(0) != 10 {
		t.Fatalf("nullable int32 values = %v", i32)
	}
	labels := retained.Column(5).(*array.String)
	if !labels.IsNull(1) || labels.IsNull(0) || labels.Value(0) != "label" {
		t.Fatalf("nullable string values = %v", labels)
	}
	tags := retained.Column(6).(*array.List)
	if !tags.IsNull(1) || tags.IsNull(0) {
		t.Fatalf("nullable list values = %v", tags)
	}
	start, end := tags.ValueOffsets(0)
	listValues := tags.ListValues().(*array.Int32)
	if start != 0 || end != 2 || listValues.Value(0) != 0 || listValues.Value(1) != 1 {
		t.Fatalf("list values offsets=(%d,%d) values=%v", start, end, listValues)
	}
	kinds := retained.Column(7).(*array.Dictionary)
	if kinds.IsNull(0) || kinds.ValueStr(0) != "kind" || kinds.IsNull(1) || kinds.ValueStr(1) != "kind" {
		t.Fatalf("dictionary values = %v", kinds)
	}
	retained.Release()

	withoutCallback, err := Scan(context.Background(), path, validOptions(info.Size()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if withoutCallback.Rows != report.Rows || withoutCallback.RecordBatches != report.RecordBatches || !withoutCallback.Complete {
		t.Fatalf("nil callback report = %#v, want %#v", withoutCallback, report)
	}
}

func TestScanRejectsInvalidOptionsAndReturnsPartialReports(t *testing.T) {
	path := writeRichFixture(t)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	valid := validOptions(info.Size())
	cases := []struct {
		name string
		edit func(*Options)
	}{
		{name: "file", edit: func(o *Options) { o.MaxFileBytes = 0 }},
		{name: "footer", edit: func(o *Options) { o.MaxFooterBytes = -1 }},
		{name: "arrow", edit: func(o *Options) { o.MaxArrowBytes = 0 }},
		{name: "rows", edit: func(o *Options) { o.MaxRows = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			options := valid
			tc.edit(&options)
			report, err := Scan(context.Background(), path, options, nil)
			if err == nil || report.Complete || report.SchemaVersion != SchemaVersion {
				t.Fatalf("report=%#v err=%v", report, err)
			}
		})
	}
}

func TestScanRejectsMagicFooterTypeAndCapacityLimitsWithoutPanic(t *testing.T) {
	path := writeRichFixture(t)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	options := validOptions(info.Size())

	cases := []struct {
		name   string
		path   string
		edit   func(*Options)
		wantIn string
	}{
		{name: "file size", path: path, edit: func(o *Options) { o.MaxFileBytes-- }, wantIn: "file"},
		{name: "footer size", path: path, edit: func(o *Options) { o.MaxFooterBytes = 1 }, wantIn: "footer"},
		{name: "row count", path: path, edit: func(o *Options) { o.MaxRows = 3 }, wantIn: "row"},
		{name: "arrow allocation", path: path, edit: func(o *Options) { o.MaxArrowBytes = 1 }, wantIn: "arrow"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caseOptions := options
			tc.edit(&caseOptions)
			report, err := scanWithoutPanic(t, path, caseOptions)
			if err == nil || report.Complete || !strings.Contains(strings.ToLower(err.Error()), tc.wantIn) {
				t.Fatalf("report=%#v err=%v, want partial %q error", report, err, tc.wantIn)
			}
		})
	}

	invalidMagic := filepath.Join(t.TempDir(), "invalid-magic.feather")
	if err := os.WriteFile(invalidMagic, []byte("FEA1invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := scanWithoutPanic(t, invalidMagic, options)
	if err == nil || report.Complete {
		t.Fatalf("accepted Feather V1/invalid magic: report=%#v err=%v", report, err)
	}

	corruptFooter := filepath.Join(t.TempDir(), "corrupt-footer.feather")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	contents[len(contents)-10] = 0xff
	if err := os.WriteFile(corruptFooter, contents, 0600); err != nil {
		t.Fatal(err)
	}
	report, err = scanWithoutPanic(t, corruptFooter, options)
	if err == nil || report.Complete {
		t.Fatalf("accepted corrupt footer: report=%#v err=%v", report, err)
	}

	unsupported := writeUnsupportedFixture(t)
	unsupportedInfo, err := os.Stat(unsupported)
	if err != nil {
		t.Fatal(err)
	}
	unsupportedOptions := validOptions(unsupportedInfo.Size())
	report, err = scanWithoutPanic(t, unsupported, unsupportedOptions)
	if err == nil || report.Complete || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("accepted unsupported type: report=%#v err=%v", report, err)
	}

	duplicate := writeDuplicateFixture(t)
	duplicateInfo, err := os.Stat(duplicate)
	if err != nil {
		t.Fatal(err)
	}
	report, err = scanWithoutPanic(t, duplicate, validOptions(duplicateInfo.Size()))
	if err == nil || report.Complete || !strings.Contains(err.Error(), "duplicate field") {
		t.Fatalf("accepted duplicate fields: report=%#v err=%v", report, err)
	}

	corruptRecord := writeCorruptRecordFixture(t)
	report, err = scanWithoutPanic(t, corruptRecord, validOptions(fileSize(t, corruptRecord)))
	if err == nil || report.Complete || !strings.Contains(strings.ToLower(err.Error()), "record") {
		t.Fatalf("accepted corrupt record: report=%#v err=%v", report, err)
	}
}

func TestScanCancellationCallbackErrorAndCallbackPanic(t *testing.T) {
	path := writeRichFixture(t)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	options := validOptions(info.Size())

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	report, err := Scan(canceled, path, options, nil)
	if !errors.Is(err, context.Canceled) || report.Complete {
		t.Fatalf("canceled scan report=%#v err=%v", report, err)
	}

	callbackErr := errors.New("stop callback")
	callbackCalls := 0
	report, err = Scan(context.Background(), path, options, func(arrow.Record) error {
		callbackCalls++
		return callbackErr
	})
	if !errors.Is(err, callbackErr) || report.Complete || callbackCalls != 1 || report.Rows != 0 || report.RecordBatches != 0 {
		t.Fatalf("callback failure report=%#v err=%v calls=%d", report, err, callbackCalls)
	}

	callbackCalls = 0
	canceledAfterCallback, cancelAfterCallback := context.WithCancel(context.Background())
	report, err = Scan(canceledAfterCallback, path, options, func(arrow.Record) error {
		callbackCalls++
		cancelAfterCallback()
		return nil
	})
	if !errors.Is(err, context.Canceled) || report.Complete || callbackCalls != 1 || report.Rows != 0 || report.RecordBatches != 0 {
		t.Fatalf("callback cancellation report=%#v err=%v calls=%d", report, err, callbackCalls)
	}

	panicValue := "callback panic"
	func() {
		defer func() {
			if got := recover(); got != panicValue {
				t.Fatalf("callback panic recovery = %#v, want %q", got, panicValue)
			}
		}()
		_, _ = Scan(context.Background(), path, options, func(arrow.Record) error {
			panic(panicValue)
		})
	}()
}

func TestScanCompressedFixtureAndArrowDecompressionLimit(t *testing.T) {
	path := writeCompressedFixture(t)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(contents, []byte{0x28, 0xb5, 0x2f, 0xfd}) {
		t.Fatal("compressed fixture does not contain a ZSTD frame")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	options := validOptions(info.Size())
	options.MaxRows = 4096
	report, err := Scan(context.Background(), path, options, nil)
	if err != nil || !report.Complete || report.Rows != 4096 || report.RecordBatches != 1 {
		t.Fatalf("compressed scan = %#v, %v", report, err)
	}
	if report.PeakArrowBytes <= 1 {
		t.Fatalf("compressed scan did not account for Arrow allocations: %#v", report)
	}
	limited := options
	limited.MaxArrowBytes = report.PeakArrowBytes - 1
	partial, err := scanWithoutPanic(t, path, limited)
	if err == nil || partial.Complete || partial.PeakArrowBytes > limited.MaxArrowBytes || !strings.Contains(strings.ToLower(err.Error()), "arrow") {
		t.Fatalf("compressed allocation limit accepted: %#v, %v", partial, err)
	}
}

func TestScanRejectsSymlinkAndLeavesSourceUnchanged(t *testing.T) {
	path := writeRichFixture(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if report, err := Scan(context.Background(), path, validOptions(info.Size()), nil); err != nil || !report.Complete {
		t.Fatalf("scan = %#v, %v", report, err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("scan modified the source file")
	}

	link := filepath.Join(t.TempDir(), "link.feather")
	if err := os.Symlink(path, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	linkInfo := info
	if report, err := Scan(context.Background(), link, validOptions(linkInfo.Size()), nil); err == nil || report.Complete {
		t.Fatalf("accepted symlink: %#v, %v", report, err)
	}
}

func scanWithoutPanic(t *testing.T, path string, options Options) (report Report, err error) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Scan panicked: %v", recovered)
		}
	}()
	return Scan(context.Background(), path, options, nil)
}

func validOptions(fileBytes int64) Options {
	return Options{
		MaxFileBytes:   fileBytes,
		MaxFooterBytes: 1 << 20,
		MaxArrowBytes:  1 << 20,
		MaxRows:        100,
	}
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

func writeRichFixture(t *testing.T) string {
	t.Helper()
	schema := arrow.NewSchema([]arrow.Field{
		{Name: "small", Type: arrow.PrimitiveTypes.Int8, Nullable: true},
		{Name: "i32", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "large", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "unsigned", Type: arrow.PrimitiveTypes.Uint64, Nullable: true},
		{Name: "score", Type: arrow.PrimitiveTypes.Float64, Nullable: true},
		{Name: "label", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "tags", Type: arrow.ListOf(arrow.PrimitiveTypes.Int32), Nullable: true},
		{Name: "kind", Type: &arrow.DictionaryType{IndexType: arrow.PrimitiveTypes.Int32, ValueType: arrow.BinaryTypes.String}, Nullable: true},
	}, nil)
	return writeRecords(t, schema, richRecord(t, schema, 3), richRecord(t, schema, 1))
}

func richRecord(t *testing.T, schema *arrow.Schema, rows int) arrow.Record {
	t.Helper()
	builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	t.Cleanup(builder.Release)
	fields := builder.Fields()
	i8 := fields[0].(*array.Int8Builder)
	i32 := fields[1].(*array.Int32Builder)
	i64 := fields[2].(*array.Int64Builder)
	u64 := fields[3].(*array.Uint64Builder)
	f64 := fields[4].(*array.Float64Builder)
	str := fields[5].(*array.StringBuilder)
	list := fields[6].(*array.ListBuilder)
	listValues := list.ValueBuilder().(*array.Int32Builder)
	dict := fields[7].(*array.BinaryDictionaryBuilder)
	for row := 0; row < rows; row++ {
		i8.Append(int8(row + 1))
		if row == 1 {
			i32.AppendNull()
		} else {
			i32.Append(int32(row + 10))
		}
		i64.Append(9007199254740993 + int64(row))
		u64.Append(9007199254740995 + uint64(row))
		f64.Append(math.Pi + float64(row))
		if row == 1 {
			str.AppendNull()
		} else {
			str.AppendString("label")
		}
		if row == 1 {
			list.AppendNull()
		} else {
			list.Append(true)
			listValues.AppendValues([]int32{int32(row), int32(row + 1)}, nil)
		}
		if err := dict.AppendString("kind"); err != nil {
			t.Fatal(err)
		}
	}
	return builder.NewRecord()
}

func writeUnsupportedFixture(t *testing.T) string {
	t.Helper()
	schema := arrow.NewSchema([]arrow.Field{{Name: "flag", Type: arrow.FixedWidthTypes.Boolean, Nullable: false}}, nil)
	builder := array.NewBooleanBuilder(memory.DefaultAllocator)
	builder.Append(true)
	arrayValue := builder.NewArray()
	builder.Release()
	record := array.NewRecord(schema, []arrow.Array{arrayValue}, 1)
	arrayValue.Release()
	return writeRecords(t, schema, record)
}

func writeCompressedFixture(t *testing.T) string {
	t.Helper()
	schema := arrow.NewSchema([]arrow.Field{{Name: "message", Type: arrow.BinaryTypes.String}}, nil)
	builder := array.NewStringBuilder(memory.DefaultAllocator)
	for i := 0; i < 4096; i++ {
		builder.AppendString("repeated-compressed-value")
	}
	values := builder.NewArray()
	builder.Release()
	record := array.NewRecord(schema, []arrow.Array{values}, 4096)
	values.Release()
	return writeRecordsWithOptions(t, schema, []ipc.Option{ipc.WithZstd()}, record)
}

func writeDuplicateFixture(t *testing.T) string {
	t.Helper()
	schema := arrow.NewSchema([]arrow.Field{
		{Name: "duplicate", Type: arrow.PrimitiveTypes.Int32},
		{Name: "duplicate", Type: arrow.PrimitiveTypes.Int32},
	}, nil)
	builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	fields := builder.Fields()
	fields[0].(*array.Int32Builder).Append(1)
	fields[1].(*array.Int32Builder).Append(2)
	record := builder.NewRecord()
	builder.Release()
	return writeRecords(t, schema, record)
}

func writeCorruptRecordFixture(t *testing.T) string {
	t.Helper()
	marker := "corrupt-record-unique"
	schema := arrow.NewSchema([]arrow.Field{{Name: "message", Type: arrow.BinaryTypes.String}}, nil)
	builder := array.NewStringBuilder(memory.DefaultAllocator)
	builder.AppendString(marker)
	values := builder.NewArray()
	builder.Release()
	record := array.NewRecord(schema, []arrow.Array{values}, 1)
	values.Release()
	path := writeRecords(t, schema, record)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	markerAt := bytes.Index(contents, []byte(marker))
	if markerAt < 0 {
		t.Fatalf("fixture marker %q not found", marker)
	}
	var offsetPattern [8]byte
	binary.LittleEndian.PutUint32(offsetPattern[4:], uint32(len(marker)))
	searchStart := markerAt - 128
	if searchStart < 0 {
		searchStart = 0
	}
	candidate := bytes.LastIndex(contents[searchStart:markerAt], offsetPattern[:])
	if candidate < 0 {
		t.Fatalf("string offset pattern not found before record body")
	}
	candidate += searchStart
	binary.LittleEndian.PutUint32(contents[candidate+4:candidate+8], uint32(0x7fffffff))
	corrupt := filepath.Join(filepath.Dir(path), "corrupt-record.feather")
	if err := os.WriteFile(corrupt, contents, 0600); err != nil {
		t.Fatal(err)
	}
	return corrupt
}

func writeRecords(t *testing.T, schema *arrow.Schema, records ...arrow.Record) string {
	t.Helper()
	return writeRecordsWithOptions(t, schema, nil, records...)
}

func writeRecordsWithOptions(t *testing.T, schema *arrow.Schema, options []ipc.Option, records ...arrow.Record) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.feather")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writerOptions := append(append([]ipc.Option(nil), options...), ipc.WithSchema(schema))
	writer, err := ipc.NewFileWriter(file, writerOptions...)
	if err != nil {
		file.Close()
		t.Fatal(err)
	}
	for _, record := range records {
		if err := writer.Write(record); err != nil {
			record.Release()
			_ = writer.Close()
			_ = file.Close()
			t.Fatal(err)
		}
		record.Release()
	}
	if err := writer.Close(); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestScanAcceptsListColumnWithNoChildValues(t *testing.T) {
	schema := arrow.NewSchema([]arrow.Field{
		{Name: "id", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "tags", Type: arrow.ListOf(arrow.PrimitiveTypes.Int64), Nullable: true},
	}, nil)
	builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	builder.Field(0).(*array.Int64Builder).Append(7)
	builder.Field(1).(*array.ListBuilder).AppendNull()
	record := builder.NewRecord()
	builder.Release()
	path := writeRecords(t, schema, record)
	report, err := Scan(context.Background(), path, validOptions(fileSize(t, path)), nil)
	if err != nil || !report.Complete || report.Rows != 1 {
		t.Fatalf("scan of list column without child values: report=%#v err=%v", report, err)
	}
}

func TestScanAcceptsAllFixedWidthPrimitiveTypes(t *testing.T) {
	path := writeFixedWidthFixture(t)
	var retained arrow.Record
	report, err := Scan(context.Background(), path, validOptions(fileSize(t, path)), func(record arrow.Record) error {
		record.Retain()
		retained = record
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || report.RecordBatches != 1 || report.Rows != 4 {
		t.Fatalf("report = %#v", report)
	}
	wantTypes := []string{"int16", "uint8", "uint16", "uint32", "float32"}
	if len(report.Fields) != len(wantTypes) {
		t.Fatalf("fields = %#v, want %d", report.Fields, len(wantTypes))
	}
	for i, field := range report.Fields {
		if field.Type != wantTypes[i] || !field.Nullable {
			t.Fatalf("field[%d] = %#v, want nullable type %q", i, field, wantTypes[i])
		}
	}
	if retained == nil {
		t.Fatal("callback did not retain a record")
	}
	defer retained.Release()

	i16 := retained.Column(0).(*array.Int16)
	if i16.Value(0) != math.MinInt16 || !i16.IsNull(1) || i16.Value(2) != math.MaxInt16 || i16.Value(3) != -1 {
		t.Fatalf("int16 values = %v", i16)
	}
	u8 := retained.Column(1).(*array.Uint8)
	if u8.Value(0) != 0 || !u8.IsNull(1) || u8.Value(2) != math.MaxUint8 || u8.Value(3) != 7 {
		t.Fatalf("uint8 values = %v", u8)
	}
	u16 := retained.Column(2).(*array.Uint16)
	if u16.Value(0) != 0 || !u16.IsNull(1) || u16.Value(2) != math.MaxUint16 || u16.Value(3) != 513 {
		t.Fatalf("uint16 values = %v", u16)
	}
	u32 := retained.Column(3).(*array.Uint32)
	if u32.Value(0) != 0 || !u32.IsNull(1) || u32.Value(2) != math.MaxUint32 || u32.Value(3) != 70000 {
		t.Fatalf("uint32 values = %v", u32)
	}
	// NaN and +Inf are reported as stored; the reader must not reject them.
	f32 := retained.Column(4).(*array.Float32)
	if !math.IsNaN(float64(f32.Value(0))) || !f32.IsNull(1) || !math.IsInf(float64(f32.Value(2)), 1) || f32.Value(3) != math.MaxFloat32 {
		t.Fatalf("float32 values = %v", f32)
	}
}

func TestScanRejectsBooleanColumn(t *testing.T) {
	path := writeUnsupportedFixture(t)
	report, err := scanWithoutPanic(t, path, validOptions(fileSize(t, path)))
	if err == nil || report.Complete {
		t.Fatalf("accepted bool column: report=%#v err=%v", report, err)
	}
	if !strings.Contains(err.Error(), `unsupported Arrow type "bool"`) {
		t.Fatalf("bool rejection error = %v", err)
	}
}

func writeFixedWidthFixture(t *testing.T) string {
	t.Helper()
	schema := arrow.NewSchema([]arrow.Field{
		{Name: "i16", Type: arrow.PrimitiveTypes.Int16, Nullable: true},
		{Name: "u8", Type: arrow.PrimitiveTypes.Uint8, Nullable: true},
		{Name: "u16", Type: arrow.PrimitiveTypes.Uint16, Nullable: true},
		{Name: "u32", Type: arrow.PrimitiveTypes.Uint32, Nullable: true},
		{Name: "conf", Type: arrow.PrimitiveTypes.Float32, Nullable: true},
	}, nil)
	builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	valid := []bool{true, false, true, true}
	builder.Field(0).(*array.Int16Builder).AppendValues([]int16{math.MinInt16, 0, math.MaxInt16, -1}, valid)
	builder.Field(1).(*array.Uint8Builder).AppendValues([]uint8{0, 0, math.MaxUint8, 7}, valid)
	builder.Field(2).(*array.Uint16Builder).AppendValues([]uint16{0, 0, math.MaxUint16, 513}, valid)
	builder.Field(3).(*array.Uint32Builder).AppendValues([]uint32{0, 0, math.MaxUint32, 70000}, valid)
	builder.Field(4).(*array.Float32Builder).AppendValues([]float32{float32(math.NaN()), 0, float32(math.Inf(1)), math.MaxFloat32}, valid)
	record := builder.NewRecord()
	builder.Release()
	return writeRecords(t, schema, record)
}
