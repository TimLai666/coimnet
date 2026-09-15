package learning_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/params"
	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
	"github.com/apache/arrow/go/v17/arrow/ipc"
	"github.com/apache/arrow/go/v17/arrow/memory"
)

// The derivation fixture of ticket 13, rebuilt here because Go cannot import
// another package's test files. Only the writers and the row tables are copied
// from simulate/derived_fixture_test.go; every expected value used by the
// tests in this package is written out again from the ticket table.
//
// Bodies 10, 20, 30 and 40 become node indices 0..3. The canonical edge order
// is (0,1) (0,2) (0,3) (1,2) (2,3) (3,0), and the derivation leaves the signs
// +1, -1, unknown, -1, unknown, unknown.

func derivedWriteFeather(t *testing.T, path string, schema *arrow.Schema, fill func(*array.RecordBuilder)) {
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

func derivedFileSum(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func derivedSourceFile(t *testing.T, dir, name string, role connectome.FileRole) connectome.SourceFile {
	t.Helper()
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return connectome.SourceFile{
		Role:       role,
		Path:       path,
		Bytes:      int64(len(data)),
		SHA256:     hex.EncodeToString(sum[:]),
		HashStatus: connectome.HashLocallyRecorded,
	}
}

func derivedGraphFixture(t *testing.T, dir string) *connectome.Graph {
	t.Helper()
	derivedWriteFeather(t, filepath.Join(dir, "annotations.feather"), arrow.NewSchema([]arrow.Field{
		{Name: "bodyId", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "status", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "class", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues([]int64{10, 20, 30, 40, 99}, nil)
		b.Field(1).(*array.StringBuilder).AppendValues([]string{"Traced", "Traced", "Traced", "Traced", "Orphan"}, nil)
		b.Field(2).(*array.StringBuilder).AppendValues([]string{"ALIN", "ALPN", "ALPN", "ALIN", "ALIN"}, nil)
	})
	derivedWriteFeather(t, filepath.Join(dir, "weights.feather"), arrow.NewSchema([]arrow.Field{
		{Name: "body_pre", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "body_post", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "weight", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues([]int64{10, 10, 10, 20, 30, 40}, nil)
		b.Field(1).(*array.Int64Builder).AppendValues([]int64{20, 30, 40, 30, 40, 10}, nil)
		b.Field(2).(*array.Int64Builder).AppendValues([]int64{4, 2, 4, 3, 1, 8}, nil)
	})
	derivedWriteFeather(t, filepath.Join(dir, "nt.feather"), arrow.NewSchema([]arrow.Field{
		{Name: "body", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "consensus_nt", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues([]int64{10}, nil)
		b.Field(1).(*array.StringBuilder).AppendValues([]string{"gaba"}, nil)
	})
	manifest := connectome.DatasetManifest{
		SchemaVersion: connectome.ManifestSchemaVersion,
		Dataset:       "derived-fixture",
		Namespace:     "learning-derived-fixture-v1",
		SourceVersion: "v1",
		License:       connectome.License{Name: "CC-BY-4.0", URL: "https://creativecommons.org/licenses/by/4.0/"},
		AcquiredAt:    "2026-09-15T00:00:00Z",
		Files: []connectome.SourceFile{
			derivedSourceFile(t, dir, "weights.feather", connectome.RoleWeights),
			derivedSourceFile(t, dir, "annotations.feather", connectome.RoleAnnotations),
			derivedSourceFile(t, dir, "nt.feather", connectome.RoleNeurotransmitters),
		},
		FieldMapping: connectome.FieldMapping{
			Weights:           connectome.WeightsFields{Source: "body_pre", Target: "body_post", Value: "weight"},
			Annotations:       connectome.AnnotationFields{ID: "bodyId", Status: "status", Class: "class"},
			Neurotransmitters: connectome.NeurotransmitterFields{ID: "body", Consensus: "consensus_nt"},
		},
		Identity: connectome.IdentityMapping{
			WeightsEndpointsAreAnnotationIDs:    true,
			NeurotransmitterIDsAreAnnotationIDs: true,
			Evidence:                            "fixture",
		},
		Selection:          connectome.SelectionPredicate{Source: connectome.RoleAnnotations, Field: "status", Equals: "Traced", Label: "engineering selection"},
		DuplicateSemantics: connectome.DuplicateSemanticsUnknown,
		CoordinateUnit:     "unverified",
		TransformHistory:   []connectome.TransformStep{{Step: "generate", Description: "fixture", Version: "test"}},
	}
	result, err := connectome.Build(context.Background(), connectome.BuildRequest{
		Manifest: manifest,
		Limits: connectome.ResourceLimits{
			MaxMemoryBytes: 8 << 20, MaxTempBytes: 8 << 20, MaxRunFiles: 16,
			MaxArrowBytes: 8 << 20, MaxFooterBytes: 1 << 20, MaxRows: 1000,
		},
		TempDir:  t.TempDir(),
		EdgeView: connectome.EdgeViewRows,
	})
	if err != nil {
		t.Fatalf("build derived fixture graph: %v", err)
	}
	if result.Graph.NodeCount() != 4 || result.Graph.EdgeCount() != 6 {
		t.Fatalf("derived fixture graph has %d nodes and %d edges, want 4 and 6", result.Graph.NodeCount(), result.Graph.EdgeCount())
	}
	return result.Graph
}

// derivedP16 builds seven probabilities as exact sixteenths.
func derivedP16(values ...int) [7]float32 {
	var probs [7]float32
	if len(values) != 7 {
		panic("derivedP16 needs seven values")
	}
	for i, v := range values {
		probs[i] = float32(v) / 16
	}
	return probs
}

type derivedTbarRow struct {
	x, y, z  int32
	xNull    bool
	body     int64
	bodyNull bool
	probs    [7]float32
	nullProb int
}

type derivedSynRow struct {
	x, y, z           int32
	xNull             bool
	pre, post         int64
	preNull, postNull bool
	roi               string
}

type derivedBodyStatRow struct {
	body      int64
	bodyNull  bool
	pre, post int32
}

// Probabilities are listed as acetylcholine, dopamine, gaba, glutamate,
// histamine, octopamine, serotonin.
var derivedTbarRows = []derivedTbarRow{
	{x: 1, y: 2, z: 3, body: 10, probs: derivedP16(12, 1, 1, 1, 1, 0, 0), nullProb: -1},
	{x: 4, y: 5, z: 6, body: 10, probs: derivedP16(8, 2, 3, 1, 1, 1, 0), nullProb: -1},
	{x: 7, y: 8, z: 9, body: 10, probs: derivedP16(2, 1, 11, 1, 1, 0, 0), nullProb: -1},
	{x: -1, y: -2, z: -3, body: 20, probs: derivedP16(3, 1, 2, 9, 1, 0, 0), nullProb: -1},
	{x: 10, y: 11, z: 12, body: 30, probs: derivedP16(0, 1, 1, 1, 1, 0, 0), nullProb: 0},
	{x: 13, y: 14, z: 15, body: 10, probs: derivedP16(14, 1, 1, 0, 0, 0, 0), nullProb: -1},
	{x: 13, y: 14, z: 15, body: 10, probs: derivedP16(1, 1, 14, 0, 0, 0, 0), nullProb: -1},
	{x: 16, y: 17, z: 18, body: 40, probs: derivedP16(8, 2, 2, 2, 1, 1, 0), nullProb: -1},
	{x: 20, y: 21, z: 22, body: 10, probs: derivedP16(3, 10, 1, 1, 1, 0, 0), nullProb: -1},
	{x: 0, y: 0, z: 0, xNull: true, body: 10, probs: derivedP16(16, 0, 0, 0, 0, 0, 0), nullProb: -1},
	{x: 30, y: 30, z: 30, bodyNull: true, probs: derivedP16(16, 0, 0, 0, 0, 0, 0), nullProb: -1},
	{x: 2, y: 2, z: 2, body: 20, probs: derivedP16(2, 1, 11, 1, 1, 0, 0), nullProb: -1},
}

var derivedSynRows = []derivedSynRow{
	{x: 1, y: 2, z: 3, pre: 10, post: 20, roi: "AL"},
	{x: 4, y: 5, z: 6, pre: 10, post: 20, roi: "AL"},
	{x: 7, y: 8, z: 9, pre: 10, post: 30, roi: "MB"},
	{x: -1, y: -2, z: -3, pre: 20, post: 30, roi: "MB"},
	{x: 10, y: 11, z: 12, pre: 30, post: 40, roi: "CX"},
	{x: 16, y: 17, z: 19, pre: 40, post: 10, roi: ""},
	{x: 13, y: 14, z: 15, pre: 10, post: 20, roi: "LH"},
	{x: 50, y: 50, z: 50, pre: 10, post: 99, roi: "AL"},
	{x: 51, y: 51, z: 51, preNull: true, post: 20, roi: ""},
	{x: 52, y: 52, z: 52, pre: 99, post: 20, roi: "LH"},
	{x: 53, y: 53, z: 53, pre: 10, post: 20, roi: "AL"},
	{x: 20, y: 21, z: 22, pre: 10, post: 40, roi: "CX"},
	{x: 2, y: 2, z: 2, pre: 20, post: 40, roi: "MB"},
	{x: 60, y: 60, z: 60, xNull: true, pre: 10, post: 20, roi: "AL"},
}

var derivedBodyStatRows = []derivedBodyStatRow{
	{body: 10, pre: 5, post: 7},
	{body: 20, pre: 3, post: 4},
	{body: 30, pre: 2, post: 6},
	{body: 99, pre: 1, post: 1},
	{body: 20, pre: 999, post: 999},
	{bodyNull: true},
}

func derivedWriteBodyStats(t *testing.T, path string, rows []derivedBodyStatRow) {
	t.Helper()
	schema := arrow.NewSchema([]arrow.Field{
		{Name: "body", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "pre", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "post", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
	}, nil)
	derivedWriteFeather(t, path, schema, func(b *array.RecordBuilder) {
		body := b.Field(0).(*array.Int64Builder)
		pre := b.Field(1).(*array.Int32Builder)
		post := b.Field(2).(*array.Int32Builder)
		for _, row := range rows {
			if row.bodyNull {
				body.AppendNull()
			} else {
				body.Append(row.body)
			}
			pre.Append(row.pre)
			post.Append(row.post)
		}
	})
}

func derivedWriteTbar(t *testing.T, path string, rows []derivedTbarRow) {
	t.Helper()
	fields := []arrow.Field{
		{Name: "x", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "y", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "z", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "body", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
	}
	for _, name := range params.Transmitters {
		fields = append(fields, arrow.Field{Name: "nt_" + name + "_prob", Type: arrow.PrimitiveTypes.Float32, Nullable: true})
	}
	derivedWriteFeather(t, path, arrow.NewSchema(fields, nil), func(b *array.RecordBuilder) {
		x := b.Field(0).(*array.Int32Builder)
		y := b.Field(1).(*array.Int32Builder)
		z := b.Field(2).(*array.Int32Builder)
		body := b.Field(3).(*array.Int64Builder)
		for _, row := range rows {
			if row.xNull {
				x.AppendNull()
			} else {
				x.Append(row.x)
			}
			y.Append(row.y)
			z.Append(row.z)
			if row.bodyNull {
				body.AppendNull()
			} else {
				body.Append(row.body)
			}
			for p := range params.Transmitters {
				column := b.Field(4 + p).(*array.Float32Builder)
				if row.nullProb == p {
					column.AppendNull()
					continue
				}
				column.Append(row.probs[p])
			}
		}
	})
}

func derivedWriteSynPartners(t *testing.T, path string, rows []derivedSynRow) {
	t.Helper()
	schema := arrow.NewSchema([]arrow.Field{
		{Name: "x_pre", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "y_pre", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "z_pre", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "body_pre", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "body_post", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "primary_post", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil)
	derivedWriteFeather(t, path, schema, func(b *array.RecordBuilder) {
		x := b.Field(0).(*array.Int32Builder)
		y := b.Field(1).(*array.Int32Builder)
		z := b.Field(2).(*array.Int32Builder)
		pre := b.Field(3).(*array.Int64Builder)
		post := b.Field(4).(*array.Int64Builder)
		roi := b.Field(5).(*array.StringBuilder)
		for _, row := range rows {
			if row.xNull {
				x.AppendNull()
			} else {
				x.Append(row.x)
			}
			y.Append(row.y)
			z.Append(row.z)
			if row.preNull {
				pre.AppendNull()
			} else {
				pre.Append(row.pre)
			}
			if row.postNull {
				post.AppendNull()
			} else {
				post.Append(row.post)
			}
			if row.roi == "" {
				roi.AppendNull()
				continue
			}
			roi.Append(row.roi)
		}
	})
}

const derivedMetaCSV = `dataset:string,postHighAccuracyThreshold:float,preHPThreshold:float,postHPThreshold:float,roiHierarchy:string
fixture,0.5,0.0,0.7,"{""name"": ""CNS"", ""children"": [{""name"": ""AL""}, {""name"": ""MB""}, {""name"": ""LH""}, {""name"": ""CX""}]}"
`

// derivedRules writes the four derivation sources into dir and returns the
// rule document of the ticket 13 fixture: gain 2, post_total normalizer,
// min_probability 0.5 and min_matched_fraction 0.25.
func derivedRules(t *testing.T, dir string) params.Rules {
	t.Helper()
	derivedWriteBodyStats(t, filepath.Join(dir, "body-stats.feather"), derivedBodyStatRows)
	derivedWriteTbar(t, filepath.Join(dir, "tbar.feather"), derivedTbarRows)
	derivedWriteSynPartners(t, filepath.Join(dir, "syn-partners.feather"), derivedSynRows)
	if err := os.WriteFile(filepath.Join(dir, "meta.csv"), []byte(derivedMetaCSV), 0o600); err != nil {
		t.Fatal(err)
	}
	ref := func(name string) params.SourceRef {
		return params.SourceRef{Path: name, SHA256: derivedFileSum(t, dir, name)}
	}
	return params.Rules{
		SchemaVersion: params.RulesSchemaVersion,
		Sources: params.Sources{
			BodyStats:    ref("body-stats.feather"),
			Tbar:         ref("tbar.feather"),
			SynPartners:  ref("syn-partners.feather"),
			NeuprintMeta: ref("meta.csv"),
		},
		Sign: params.SignRule{
			SchemaVersion: params.SignRuleSchemaVersion,
			Mapping: map[string]string{
				"acetylcholine": params.SignPositive,
				"dopamine":      params.SignUnknown,
				"gaba":          params.SignNegative,
				"glutamate":     params.SignNegative,
				"histamine":     params.SignNegative,
				"octopamine":    params.SignUnknown,
				"serotonin":     params.SignUnknown,
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
		},
		Strength: params.StrengthRule{Gain: 2, Normalizer: params.NormalizerPostTotal},
	}
}

// derivedFixtureSet runs the real derivation over the fixture.
func derivedFixtureSet(t *testing.T) *params.Set {
	t.Helper()
	dir := t.TempDir()
	graph := derivedGraphFixture(t, dir)
	rules := derivedRules(t, dir)
	if err := rules.Validate(); err != nil {
		t.Fatalf("fixture rules are invalid: %v", err)
	}
	set, _, err := params.Derive(context.Background(), graph, rules, dir, params.Limits{
		MaxMemoryBytes: 8 << 20,
		MaxTempBytes:   8 << 20,
		MaxRunFiles:    32,
		MaxArrowBytes:  8 << 20,
		MaxRows:        100000,
		TempDir:        t.TempDir(),
	})
	if err != nil {
		t.Fatalf("derive fixture parameter set: %v", err)
	}
	return set
}
