package connectome

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
	"github.com/apache/arrow/go/v17/arrow/ipc"
	"github.com/apache/arrow/go/v17/arrow/memory"
)

// annotationRow mirrors one official-like annotation row. nil means null.
type annotationRow struct {
	id           *int64
	status       *string
	statusLabel  *string
	class        *string
	typeName     *string
	receptorType *string
}

type weightRow struct {
	pre, post, weight *int64
}

type ntRow struct {
	id         *int64
	consensus  *string
	predicted  *string
	confidence *float64
}

func i64(v int64) *int64       { return &v }
func str(v string) *string     { return &v }
func f64(v float64) *float64   { return &v }
func hugeID() int64            { return 9007199254740993 } // 2^53 + 1, not exactly representable as float64
func testNamespace() string    { return "fixture-v1" }
func defaultBatches(n int) int { return n }

// happyAnnotations returns the annotation rows used by most Build tests.
// Selected (status == Traced with valid unique IDs): 10, 20, 30, huge, 60.
func happyAnnotations() []annotationRow {
	return []annotationRow{
		{id: i64(10), status: str("Traced"), statusLabel: str("Reviewed"), class: str("A"), typeName: str("T1")},
		{id: i64(20), status: str("Traced"), statusLabel: str("Roughly traced"), typeName: str("T2"), receptorType: str("R1")},
		{id: i64(30), status: str("Traced"), class: str("B")},
		{id: i64(40), status: str("Orphan"), statusLabel: str("Orphan")},
		{id: i64(50), statusLabel: str("Unimportant")},
		{id: i64(hugeID()), status: str("Traced"), statusLabel: str("Reviewed")},
		{id: i64(-5), status: str("Traced")},
		{status: str("Traced")},
		{id: i64(10), status: str("Traced")},
		{id: i64(60), status: str("Traced"), statusLabel: str("Reviewed")},
	}
}

// happyWeights returns the eleven weight rows used by most Build tests.
func happyWeights() []weightRow {
	return []weightRow{
		{i64(10), i64(20), i64(3)},
		{i64(10), i64(20), i64(4)},
		{i64(20), i64(30), i64(1)},
		{i64(30), i64(30), i64(2)},
		{i64(10), i64(40), i64(5)},
		{i64(20), i64(99), i64(1)},
		{nil, i64(20), i64(1)},
		{i64(10), i64(-7), i64(1)},
		{i64(hugeID()), i64(10), nil},
		{i64(30), i64(10), i64(0)},
		{i64(20), i64(10), i64(7)},
	}
}

func happyNeurotransmitters() []ntRow {
	return []ntRow{
		{id: i64(10), consensus: str("acetylcholine"), predicted: str("acetylcholine"), confidence: f64(0.9)},
		{id: i64(20), consensus: str("unclear"), predicted: str("gaba"), confidence: f64(0.4)},
		{id: i64(30)},
		{id: i64(99), consensus: str("gaba"), predicted: str("gaba"), confidence: f64(0.8)},
		{id: i64(10), consensus: str("gaba"), predicted: str("gaba"), confidence: f64(0.1)},
	}
}

func annotationSchema() *arrow.Schema {
	return arrow.NewSchema([]arrow.Field{
		{Name: "bodyId", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "status", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "statusLabel", Type: &arrow.DictionaryType{IndexType: arrow.PrimitiveTypes.Int8, ValueType: arrow.BinaryTypes.String, Ordered: true}, Nullable: true},
		{Name: "class", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "type", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "receptorType", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "somaLocation", Type: arrow.ListOf(arrow.PrimitiveTypes.Int64), Nullable: true},
	}, nil)
}

func weightSchema() *arrow.Schema {
	return arrow.NewSchema([]arrow.Field{
		{Name: "body_pre", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "body_post", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "weight", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
	}, nil)
}

func ntSchema() *arrow.Schema {
	return arrow.NewSchema([]arrow.Field{
		{Name: "body", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "consensus_nt", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "predicted_nt", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "predicted_nt_confidence", Type: arrow.PrimitiveTypes.Float64, Nullable: true},
	}, nil)
}

func appendString(b *array.StringBuilder, v *string) {
	if v == nil {
		b.AppendNull()
		return
	}
	b.Append(*v)
}

func appendInt64(b *array.Int64Builder, v *int64) {
	if v == nil {
		b.AppendNull()
		return
	}
	b.Append(*v)
}

// writeAnnotations writes rows into batches of the given sizes. The
// statusLabel dictionary is shared by every batch, as in the official file.
func writeAnnotations(t *testing.T, dir string, rows []annotationRow, batchSizes []int) string {
	t.Helper()
	schema := annotationSchema()
	labels := []string{}
	labelIndex := map[string]int8{}
	for _, row := range rows {
		if row.statusLabel != nil {
			if _, ok := labelIndex[*row.statusLabel]; !ok {
				labelIndex[*row.statusLabel] = int8(len(labels))
				labels = append(labels, *row.statusLabel)
			}
		}
	}
	dictBuilder := array.NewStringBuilder(memory.DefaultAllocator)
	dictBuilder.AppendValues(labels, nil)
	dictionary := dictBuilder.NewArray()
	dictBuilder.Release()
	t.Cleanup(dictionary.Release)

	var records []arrow.Record
	start := 0
	for _, size := range batchSizes {
		ids := array.NewInt64Builder(memory.DefaultAllocator)
		status := array.NewStringBuilder(memory.DefaultAllocator)
		indices := array.NewInt8Builder(memory.DefaultAllocator)
		class := array.NewStringBuilder(memory.DefaultAllocator)
		typeName := array.NewStringBuilder(memory.DefaultAllocator)
		receptor := array.NewStringBuilder(memory.DefaultAllocator)
		soma := array.NewListBuilder(memory.DefaultAllocator, arrow.PrimitiveTypes.Int64)
		somaValues := soma.ValueBuilder().(*array.Int64Builder)
		for _, row := range rows[start : start+size] {
			appendInt64(ids, row.id)
			appendString(status, row.status)
			if row.statusLabel == nil {
				indices.AppendNull()
			} else {
				indices.Append(labelIndex[*row.statusLabel])
			}
			appendString(class, row.class)
			appendString(typeName, row.typeName)
			appendString(receptor, row.receptorType)
			if row.id != nil && *row.id%2 == 0 {
				soma.Append(true)
				somaValues.AppendValues([]int64{1, 2, 3}, nil)
			} else {
				soma.AppendNull()
			}
		}
		indexArray := indices.NewArray()
		labelArray := array.NewDictionaryArray(schema.Field(2).Type, indexArray, dictionary)
		indexArray.Release()
		columns := []arrow.Array{ids.NewArray(), status.NewArray(), labelArray, class.NewArray(), typeName.NewArray(), receptor.NewArray(), soma.NewArray()}
		records = append(records, array.NewRecord(schema, columns, int64(size)))
		for _, column := range columns {
			column.Release()
		}
		for _, b := range []interface{ Release() }{ids, status, indices, class, typeName, receptor, soma} {
			b.Release()
		}
		start += size
	}
	if start != len(rows) {
		t.Fatalf("batch sizes cover %d rows, want %d", start, len(rows))
	}
	return writeFeather(t, filepath.Join(dir, "annotations.feather"), schema, records)
}

func writeWeights(t *testing.T, dir string, rows []weightRow, batchSizes []int) string {
	t.Helper()
	schema := weightSchema()
	var records []arrow.Record
	start := 0
	for _, size := range batchSizes {
		builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
		pre := builder.Field(0).(*array.Int64Builder)
		post := builder.Field(1).(*array.Int64Builder)
		weight := builder.Field(2).(*array.Int64Builder)
		for _, row := range rows[start : start+size] {
			appendInt64(pre, row.pre)
			appendInt64(post, row.post)
			appendInt64(weight, row.weight)
		}
		records = append(records, builder.NewRecord())
		builder.Release()
		start += size
	}
	if start != len(rows) {
		t.Fatalf("batch sizes cover %d rows, want %d", start, len(rows))
	}
	return writeFeather(t, filepath.Join(dir, "weights.feather"), schema, records)
}

func writeNeurotransmitters(t *testing.T, dir string, rows []ntRow) string {
	t.Helper()
	schema := ntSchema()
	builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	ids := builder.Field(0).(*array.Int64Builder)
	consensus := builder.Field(1).(*array.StringBuilder)
	predicted := builder.Field(2).(*array.StringBuilder)
	confidence := builder.Field(3).(*array.Float64Builder)
	for _, row := range rows {
		appendInt64(ids, row.id)
		appendString(consensus, row.consensus)
		appendString(predicted, row.predicted)
		if row.confidence == nil {
			confidence.AppendNull()
		} else {
			confidence.Append(*row.confidence)
		}
	}
	record := builder.NewRecord()
	builder.Release()
	return writeFeather(t, filepath.Join(dir, "neurotransmitters.feather"), schema, []arrow.Record{record})
}

func writeFeather(t *testing.T, path string, schema *arrow.Schema, records []arrow.Record) string {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := ipc.NewFileWriter(file, ipc.WithSchema(schema))
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if err := writer.Write(record); err != nil {
			t.Fatal(err)
		}
		record.Release()
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func fileFingerprint(t *testing.T, path string) (int64, string) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	h := sha256.New()
	n, err := io.Copy(h, file)
	if err != nil {
		t.Fatal(err)
	}
	return n, hex.EncodeToString(h.Sum(nil))
}

func sourceFile(t *testing.T, role FileRole, path string) SourceFile {
	t.Helper()
	size, sum := fileFingerprint(t, path)
	return SourceFile{Role: role, Path: path, Bytes: size, SHA256: sum, HashStatus: HashLocallyRecorded}
}

func fixtureManifest(t *testing.T, weights, annotations, neurotransmitters string) DatasetManifest {
	t.Helper()
	return DatasetManifest{
		SchemaVersion: ManifestSchemaVersion,
		Dataset:       "fixture",
		Namespace:     testNamespace(),
		SourceVersion: "v1",
		License:       License{Name: "CC-BY-4.0", URL: "https://creativecommons.org/licenses/by/4.0/"},
		AcquiredAt:    "2026-09-13T00:00:00Z",
		Files: []SourceFile{
			sourceFile(t, RoleWeights, weights),
			sourceFile(t, RoleAnnotations, annotations),
			sourceFile(t, RoleNeurotransmitters, neurotransmitters),
		},
		FieldMapping: FieldMapping{
			Weights:     WeightsFields{Source: "body_pre", Target: "body_post", Value: "weight"},
			Annotations: AnnotationFields{ID: "bodyId", Status: "status", StatusLabel: "statusLabel", Class: "class", Type: "type", ReceptorType: "receptorType"},
			Neurotransmitters: NeurotransmitterFields{
				ID: "body", Consensus: "consensus_nt", Predicted: "predicted_nt", Confidence: "predicted_nt_confidence",
			},
		},
		Identity: IdentityMapping{
			WeightsEndpointsAreAnnotationIDs:    true,
			NeurotransmitterIDsAreAnnotationIDs: true,
			Evidence:                            "fixture files share one synthetic body ID space",
		},
		Selection:          SelectionPredicate{Source: RoleAnnotations, Field: "status", Equals: "Traced", Label: "engineering selection; not biological completeness"},
		DuplicateSemantics: DuplicateSemanticsUnknown,
		CoordinateUnit:     "unverified",
		TransformHistory:   []TransformStep{{Step: "generate", Description: "synthetic fixture", Version: "test"}},
	}
}

func fixtureLimits() ResourceLimits {
	return ResourceLimits{
		MaxMemoryBytes: 64 << 20,
		MaxTempBytes:   64 << 20,
		MaxRunFiles:    64,
		MaxArrowBytes:  16 << 20,
		MaxFooterBytes: 1 << 20,
		MaxRows:        1 << 20,
	}
}

// happyFixture writes the standard three files and returns a manifest.
func happyFixture(t *testing.T, weightBatches []int) (DatasetManifest, string) {
	t.Helper()
	dir := t.TempDir()
	annotations := writeAnnotations(t, dir, happyAnnotations(), []int{6, 4})
	weights := writeWeights(t, dir, happyWeights(), weightBatches)
	nt := writeNeurotransmitters(t, dir, happyNeurotransmitters())
	return fixtureManifest(t, weights, annotations, nt), dir
}
