package params

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
	"github.com/apache/arrow/go/v17/arrow/ipc"
	"github.com/apache/arrow/go/v17/arrow/memory"
)

// writeFeather writes one record batch as a Feather V2 file. One batch keeps
// every dictionary column backed by a single dictionary array, which the
// Arrow IPC writer requires.
func writeFeather(t *testing.T, path string, schema *arrow.Schema, fill func(*array.RecordBuilder)) {
	t.Helper()
	builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	fill(builder)
	record := builder.NewRecord()
	builder.Release()
	defer record.Release()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := ipc.NewFileWriter(file, ipc.WithSchema(schema))
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Write(record); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func fileFingerprint(t *testing.T, dir, name string) (int64, string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return int64(len(data)), hex.EncodeToString(sum[:])
}

// graphFixture writes the three connectome sources and a manifest into dir and
// builds the four-node, six-edge graph used by every derivation test.
//
// Bodies 10, 20, 30 and 40 are selected in that order, so the node indices are
// 0, 1, 2 and 3. Body 99 is present but not selected. The canonical edge order
// is (0,1) (0,2) (0,3) (1,2) (2,3) (3,0) with raw weights 4, 2, 4, 3, 1 and 8.
func graphFixture(t *testing.T, dir string) *connectome.Graph {
	t.Helper()
	writeFeather(t, filepath.Join(dir, "annotations.feather"), arrow.NewSchema([]arrow.Field{
		{Name: "bodyId", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "status", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues([]int64{10, 20, 30, 40, 99}, nil)
		b.Field(1).(*array.StringBuilder).AppendValues([]string{"Traced", "Traced", "Traced", "Traced", "Orphan"}, nil)
	})
	writeFeather(t, filepath.Join(dir, "weights.feather"), arrow.NewSchema([]arrow.Field{
		{Name: "body_pre", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "body_post", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "weight", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues([]int64{10, 10, 10, 20, 30, 40}, nil)
		b.Field(1).(*array.Int64Builder).AppendValues([]int64{20, 30, 40, 30, 40, 10}, nil)
		b.Field(2).(*array.Int64Builder).AppendValues([]int64{4, 2, 4, 3, 1, 8}, nil)
	})
	writeFeather(t, filepath.Join(dir, "nt.feather"), arrow.NewSchema([]arrow.Field{
		{Name: "body", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "consensus_nt", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues([]int64{10}, nil)
		b.Field(1).(*array.StringBuilder).AppendValues([]string{"gaba"}, nil)
	})

	return buildFixtureGraph(t, dir, "params-fixture-v1", 4, 6)
}

// buildFixtureGraph writes a manifest for the three sources already in dir and
// builds the graph, checking the expected shape.
func buildFixtureGraph(t *testing.T, dir, namespace string, wantNodes, wantEdges uint64) *connectome.Graph {
	t.Helper()
	source := func(name, role string) map[string]any {
		size, sum := fileFingerprint(t, dir, name)
		return map[string]any{"path": name, "bytes": size, "sha256": sum, "hash_status": "locally_recorded", "role": role}
	}
	manifest := map[string]any{
		"schema_version": "coimnet-dataset-manifest/v1",
		"dataset":        "params-fixture",
		"namespace":      namespace,
		"source_version": "v1",
		"license":        map[string]any{"name": "CC-BY-4.0", "url": "https://creativecommons.org/licenses/by/4.0/"},
		"acquired_at":    "2026-09-15T00:00:00Z",
		"files": []any{
			source("weights.feather", "weights"),
			source("annotations.feather", "annotations"),
			source("nt.feather", "neurotransmitters"),
		},
		"field_mapping": map[string]any{
			"weights":           map[string]any{"source": "body_pre", "target": "body_post", "value": "weight"},
			"annotations":       map[string]any{"id": "bodyId", "status": "status"},
			"neurotransmitters": map[string]any{"id": "body", "consensus": "consensus_nt"},
		},
		"identity": map[string]any{
			"weights_endpoints_are_annotation_ids":    true,
			"neurotransmitter_ids_are_annotation_ids": true,
			"evidence": "fixture",
		},
		"selection":           map[string]any{"source": "annotations", "field": "status", "equals": "Traced", "label": "engineering selection"},
		"duplicate_semantics": "unknown",
		"coordinate_unit":     "unverified",
		"transform_history":   []any{map[string]any{"step": "generate", "description": "fixture", "version": "test"}},
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := connectome.DecodeManifest(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	for i := range decoded.Files {
		decoded.Files[i].Path = filepath.Join(dir, decoded.Files[i].Path)
	}
	result, err := connectome.Build(context.Background(), connectome.BuildRequest{
		Manifest: decoded,
		Limits: connectome.ResourceLimits{
			MaxMemoryBytes: 8 << 20,
			MaxTempBytes:   8 << 20,
			MaxRunFiles:    16,
			MaxArrowBytes:  8 << 20,
			MaxFooterBytes: 1 << 20,
			MaxRows:        1000,
		},
		TempDir:  t.TempDir(),
		EdgeView: connectome.EdgeViewRows,
	})
	if err != nil {
		t.Fatalf("build fixture graph: %v", err)
	}
	if result.Graph.NodeCount() != wantNodes || result.Graph.EdgeCount() != wantEdges {
		t.Fatalf("fixture graph has %d nodes and %d edges, want %d and %d", result.Graph.NodeCount(), result.Graph.EdgeCount(), wantNodes, wantEdges)
	}
	return result.Graph
}

// differentGraphFixture builds a graph with a different node set, so its node
// index and edge order hashes cannot match the main fixture.
func differentGraphFixture(t *testing.T) *connectome.Graph {
	t.Helper()
	other := t.TempDir()
	writeFeather(t, filepath.Join(other, "annotations.feather"), arrow.NewSchema([]arrow.Field{
		{Name: "bodyId", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "status", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues([]int64{10, 20, 30}, nil)
		b.Field(1).(*array.StringBuilder).AppendValues([]string{"Traced", "Traced", "Traced"}, nil)
	})
	writeFeather(t, filepath.Join(other, "weights.feather"), arrow.NewSchema([]arrow.Field{
		{Name: "body_pre", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "body_post", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "weight", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues([]int64{10, 20}, nil)
		b.Field(1).(*array.Int64Builder).AppendValues([]int64{20, 30}, nil)
		b.Field(2).(*array.Int64Builder).AppendValues([]int64{1, 2}, nil)
	})
	writeFeather(t, filepath.Join(other, "nt.feather"), arrow.NewSchema([]arrow.Field{
		{Name: "body", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "consensus_nt", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues([]int64{10}, nil)
		b.Field(1).(*array.StringBuilder).AppendValues([]string{"gaba"}, nil)
	})
	return buildFixtureGraph(t, other, "params-fixture-alt-v1", 3, 2)
}

// p16 builds seven probabilities as exact sixteenths, so float32 storage and
// float64 averaging are both exact and the expected values can be written down.
func p16(values ...int) [7]float32 {
	var probs [7]float32
	if len(values) != 7 {
		panic("p16 needs seven values")
	}
	for i, v := range values {
		probs[i] = float32(v) / 16
	}
	return probs
}

type tbarRow struct {
	x, y, z  int32
	xNull    bool
	body     int64
	bodyNull bool
	probs    [7]float32
	nullProb int // index of the null probability, -1 for none
}

type synRow struct {
	x, y, z           int32
	xNull             bool
	pre, post         int64
	preNull, postNull bool
	roi               string // "" means a null primary_post
}

type bodyStatRow struct {
	body      int64
	bodyNull  bool
	pre, post int32
}

// tbarFixtureRows are the twelve T-bar rows of the hand-calculated fixture.
// Probabilities are listed as acetylcholine, dopamine, gaba, glutamate,
// histamine, octopamine, serotonin.
var tbarFixtureRows = []tbarRow{
	{x: 1, y: 2, z: 3, body: 10, probs: p16(12, 1, 1, 1, 1, 0, 0), nullProb: -1},    // A: acetylcholine 0.75
	{x: 4, y: 5, z: 6, body: 10, probs: p16(8, 2, 3, 1, 1, 1, 0), nullProb: -1},     // B: acetylcholine 0.5
	{x: 7, y: 8, z: 9, body: 10, probs: p16(2, 1, 11, 1, 1, 0, 0), nullProb: -1},    // C: gaba 0.6875
	{x: -1, y: -2, z: -3, body: 20, probs: p16(3, 1, 2, 9, 1, 0, 0), nullProb: -1},  // D: glutamate 0.5625
	{x: 10, y: 11, z: 12, body: 30, probs: p16(0, 1, 1, 1, 1, 0, 0), nullProb: 0},   // E: null probability
	{x: 13, y: 14, z: 15, body: 10, probs: p16(14, 1, 1, 0, 0, 0, 0), nullProb: -1}, // F1: duplicate key
	{x: 13, y: 14, z: 15, body: 10, probs: p16(1, 1, 14, 0, 0, 0, 0), nullProb: -1}, // F2: duplicate key
	{x: 16, y: 17, z: 18, body: 40, probs: p16(8, 2, 2, 2, 1, 1, 0), nullProb: -1},  // G: no exact synapse
	{x: 20, y: 21, z: 22, body: 10, probs: p16(3, 10, 1, 1, 1, 0, 0), nullProb: -1}, // I: dopamine 0.625
	{x: 0, y: 0, z: 0, xNull: true, body: 10, probs: p16(16, 0, 0, 0, 0, 0, 0), nullProb: -1},
	{x: 30, y: 30, z: 30, bodyNull: true, probs: p16(16, 0, 0, 0, 0, 0, 0), nullProb: -1},
	{x: 2, y: 2, z: 2, body: 20, probs: p16(2, 1, 11, 1, 1, 0, 0), nullProb: -1}, // H: gaba 0.6875, pair not in graph
}

// synFixtureRows are the fourteen synapse rows of the hand-calculated fixture.
var synFixtureRows = []synRow{
	{x: 1, y: 2, z: 3, pre: 10, post: 20, roi: "AL"},
	{x: 4, y: 5, z: 6, pre: 10, post: 20, roi: "AL"},
	{x: 7, y: 8, z: 9, pre: 10, post: 30, roi: "MB"},
	{x: -1, y: -2, z: -3, pre: 20, post: 30, roi: "MB"},
	{x: 10, y: 11, z: 12, pre: 30, post: 40, roi: "CX"},
	{x: 16, y: 17, z: 19, pre: 40, post: 10, roi: ""}, // one voxel off in z
	{x: 13, y: 14, z: 15, pre: 10, post: 20, roi: "LH"},
	{x: 50, y: 50, z: 50, pre: 10, post: 99, roi: "AL"},
	{x: 51, y: 51, z: 51, preNull: true, post: 20, roi: ""},
	{x: 52, y: 52, z: 52, pre: 99, post: 20, roi: "LH"},
	{x: 53, y: 53, z: 53, pre: 10, post: 20, roi: "AL"}, // key absent from tbar
	{x: 20, y: 21, z: 22, pre: 10, post: 40, roi: "CX"},
	{x: 2, y: 2, z: 2, pre: 20, post: 40, roi: "MB"},
	{x: 60, y: 60, z: 60, xNull: true, pre: 10, post: 20, roi: "AL"},
}

var bodyStatsFixtureRows = []bodyStatRow{
	{body: 10, pre: 5, post: 7},
	{body: 20, pre: 3, post: 4},
	{body: 30, pre: 2, post: 6},
	{body: 99, pre: 1, post: 1},
	{body: 20, pre: 999, post: 999}, // duplicate; the first row wins
	{bodyNull: true},
}

func bodyStatsSchema() *arrow.Schema {
	return arrow.NewSchema([]arrow.Field{
		{Name: "body", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "pre", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "post", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "status_fine", Type: &arrow.DictionaryType{IndexType: arrow.PrimitiveTypes.Int8, ValueType: arrow.BinaryTypes.String, Ordered: true}, Nullable: true},
		{Name: "superclass", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "class", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "type", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "instance", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "downstream", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "synweight", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "rank", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
	}, nil)
}

func writeBodyStats(t *testing.T, path string, rows []bodyStatRow) {
	t.Helper()
	writeFeather(t, path, bodyStatsSchema(), func(b *array.RecordBuilder) {
		body := b.Field(0).(*array.Int64Builder)
		pre := b.Field(1).(*array.Int32Builder)
		post := b.Field(2).(*array.Int32Builder)
		status := b.Field(3).(*array.BinaryDictionaryBuilder)
		for _, row := range rows {
			if row.bodyNull {
				body.AppendNull()
			} else {
				body.Append(row.body)
			}
			pre.Append(row.pre)
			post.Append(row.post)
			if err := status.AppendString("Traced"); err != nil {
				t.Fatal(err)
			}
			for _, index := range []int{4, 5, 6, 7} {
				b.Field(index).(*array.StringBuilder).Append("x")
			}
			for _, index := range []int{8, 9, 10} {
				b.Field(index).(*array.Int64Builder).Append(1)
			}
		}
	})
}

func tbarSchema() *arrow.Schema {
	fields := []arrow.Field{
		{Name: "point_id", Type: arrow.PrimitiveTypes.Uint64, Nullable: true},
		{Name: "x", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "y", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "z", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "conf", Type: arrow.PrimitiveTypes.Float32, Nullable: true},
		{Name: "sv", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "body", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "major", Type: &arrow.DictionaryType{IndexType: arrow.PrimitiveTypes.Int8, ValueType: arrow.BinaryTypes.String, Ordered: true}, Nullable: true},
		{Name: "primary", Type: &arrow.DictionaryType{IndexType: arrow.PrimitiveTypes.Int16, ValueType: arrow.BinaryTypes.String, Ordered: true}, Nullable: true},
	}
	for _, name := range Transmitters {
		fields = append(fields, arrow.Field{Name: "nt_" + name + "_prob", Type: arrow.PrimitiveTypes.Float32, Nullable: true})
	}
	fields = append(fields, arrow.Field{Name: "split", Type: &arrow.DictionaryType{IndexType: arrow.PrimitiveTypes.Int8, ValueType: arrow.BinaryTypes.String, Ordered: false}, Nullable: true})
	return arrow.NewSchema(fields, nil)
}

func writeTbar(t *testing.T, path string, rows []tbarRow) {
	t.Helper()
	writeFeather(t, path, tbarSchema(), func(b *array.RecordBuilder) {
		pointID := b.Field(0).(*array.Uint64Builder)
		x := b.Field(1).(*array.Int32Builder)
		y := b.Field(2).(*array.Int32Builder)
		z := b.Field(3).(*array.Int32Builder)
		conf := b.Field(4).(*array.Float32Builder)
		sv := b.Field(5).(*array.Int64Builder)
		body := b.Field(6).(*array.Int64Builder)
		major := b.Field(7).(*array.BinaryDictionaryBuilder)
		primary := b.Field(8).(*array.BinaryDictionaryBuilder)
		split := b.Field(16).(*array.BinaryDictionaryBuilder)
		for i, row := range rows {
			pointID.Append(uint64(i))
			if row.xNull {
				x.AppendNull()
			} else {
				x.Append(row.x)
			}
			y.Append(row.y)
			z.Append(row.z)
			conf.Append(0.9)
			sv.Append(int64(i))
			if row.bodyNull {
				body.AppendNull()
			} else {
				body.Append(row.body)
			}
			if err := major.AppendString("acetylcholine"); err != nil {
				t.Fatal(err)
			}
			if err := primary.AppendString("AL"); err != nil {
				t.Fatal(err)
			}
			for p := range Transmitters {
				column := b.Field(9 + p).(*array.Float32Builder)
				if row.nullProb == p {
					column.AppendNull()
					continue
				}
				column.Append(row.probs[p])
			}
			if err := split.AppendString("none"); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func synPartnersSchema() *arrow.Schema {
	return arrow.NewSchema([]arrow.Field{
		{Name: "x_pre", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "y_pre", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "z_pre", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "body_pre", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "conf_pre", Type: arrow.PrimitiveTypes.Float32, Nullable: true},
		{Name: "x_post", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "y_post", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "z_post", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "body_post", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "conf_post", Type: arrow.PrimitiveTypes.Float32, Nullable: true},
		{Name: "primary_post", Type: &arrow.DictionaryType{IndexType: arrow.PrimitiveTypes.Int16, ValueType: arrow.BinaryTypes.String, Ordered: true}, Nullable: true},
	}, nil)
}

func writeSynPartners(t *testing.T, path string, rows []synRow) {
	t.Helper()
	writeFeather(t, path, synPartnersSchema(), func(b *array.RecordBuilder) {
		xPre := b.Field(0).(*array.Int32Builder)
		yPre := b.Field(1).(*array.Int32Builder)
		zPre := b.Field(2).(*array.Int32Builder)
		bodyPre := b.Field(3).(*array.Int64Builder)
		confPre := b.Field(4).(*array.Float32Builder)
		xPost := b.Field(5).(*array.Int32Builder)
		yPost := b.Field(6).(*array.Int32Builder)
		zPost := b.Field(7).(*array.Int32Builder)
		bodyPost := b.Field(8).(*array.Int64Builder)
		confPost := b.Field(9).(*array.Float32Builder)
		primaryPost := b.Field(10).(*array.BinaryDictionaryBuilder)
		for _, row := range rows {
			if row.xNull {
				xPre.AppendNull()
			} else {
				xPre.Append(row.x)
			}
			yPre.Append(row.y)
			zPre.Append(row.z)
			if row.preNull {
				bodyPre.AppendNull()
			} else {
				bodyPre.Append(row.pre)
			}
			confPre.Append(0.8)
			xPost.Append(row.x + 1)
			yPost.Append(row.y + 1)
			zPost.Append(row.z + 1)
			if row.postNull {
				bodyPost.AppendNull()
			} else {
				bodyPost.Append(row.post)
			}
			confPost.Append(0.8)
			if row.roi == "" {
				primaryPost.AppendNull()
				continue
			}
			if err := primaryPost.AppendString(row.roi); err != nil {
				t.Fatal(err)
			}
		}
	})
}

// metaCSV is a one-row Neuprint_Meta.csv in the official neo4j import shape:
// header names carry a ":type" suffix and roiHierarchy is a JSON object in a
// CSV cell.
const metaCSV = `dataset:string,postHighAccuracyThreshold:float,preHPThreshold:float,postHPThreshold:float,totalPreCount:int,totalPostCount:int,roiHierarchy:string,roiInfo:string
fixture,0.5,0.0,0.7,12,14,"{""name"": ""CNS"", ""children"": [{""name"": ""CentralBrain"", ""children"": [{""name"": ""AL""}, {""name"": ""MB""}]}, {""name"": ""Optic"", ""children"": [{""name"": ""LH""}, {""name"": ""CX""}]}]}","{}"
`

// sourceFixture writes the four derivation sources into dir and returns the
// Sources block with the fingerprints they actually have.
func sourceFixture(t *testing.T, dir string, tbar []tbarRow, syn []synRow, stats []bodyStatRow) Sources {
	t.Helper()
	writeBodyStats(t, filepath.Join(dir, "body-stats.feather"), stats)
	writeTbar(t, filepath.Join(dir, "tbar.feather"), tbar)
	writeSynPartners(t, filepath.Join(dir, "syn-partners.feather"), syn)
	if err := os.WriteFile(filepath.Join(dir, "meta.csv"), []byte(metaCSV), 0o600); err != nil {
		t.Fatal(err)
	}
	ref := func(name string) SourceRef {
		_, sum := fileFingerprint(t, dir, name)
		return SourceRef{Path: name, SHA256: sum}
	}
	return Sources{
		BodyStats:    ref("body-stats.feather"),
		Tbar:         ref("tbar.feather"),
		SynPartners:  ref("syn-partners.feather"),
		NeuprintMeta: ref("meta.csv"),
	}
}

// baseSignRule is the rule set every derivation test starts from.
func baseSignRule() SignRule {
	return SignRule{
		SchemaVersion: SignRuleSchemaVersion,
		Mapping: map[string]string{
			"acetylcholine": SignPositive,
			"dopamine":      SignUnknown,
			"gaba":          SignNegative,
			"glutamate":     SignNegative,
			"histamine":     SignNegative,
			"octopamine":    SignUnknown,
			"serotonin":     SignUnknown,
		},
		Basis: map[string]string{
			"acetylcholine": "fast excitatory nAChR transmission reported for the fly CNS",
			"dopamine":      "modulatory, receptor dependent; kept unknown",
			"gaba":          "GABA-A chloride conductance reported as inhibitory",
			"glutamate":     "多為 GluCl 抑制，屬工程假設 (engineering assumption)",
			"histamine":     "HisCl chloride conductance reported as inhibitory",
			"octopamine":    "modulatory, receptor dependent; kept unknown",
			"serotonin":     "modulatory, receptor dependent; kept unknown",
		},
		MinProbability:     0.5,
		MinMatchedFraction: 0.25,
	}
}

func fixtureRules(t *testing.T, dir string, normalizer string, sign SignRule) Rules {
	t.Helper()
	return Rules{
		SchemaVersion: RulesSchemaVersion,
		Sources:       sourceFixture(t, dir, tbarFixtureRows, synFixtureRows, bodyStatsFixtureRows),
		Sign:          sign,
		Strength:      StrengthRule{Gain: 2, Normalizer: normalizer},
	}
}

func fixtureLimits(t *testing.T) Limits {
	t.Helper()
	return Limits{
		MaxMemoryBytes: 8 << 20,
		MaxTempBytes:   8 << 20,
		MaxRunFiles:    32,
		MaxArrowBytes:  8 << 20,
		MaxRows:        100000,
		TempDir:        t.TempDir(),
	}
}
