package modulation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/simulate"
	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
	"github.com/apache/arrow/go/v17/arrow/ipc"
	"github.com/apache/arrow/go/v17/arrow/memory"
)

// A ResolvedSet keeps its node indices private, so a neural source cannot be
// tested against a forged set: the fixture below resolves a real one. It is the
// same three-neuron graph the simulate package uses (body IDs 1, 2 and 3 are
// Traced and become node indices 0, 1 and 2; body 4 is filtered out), rebuilt
// here because a test helper of another package is not importable.
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

func fixtureSource(t *testing.T, dir, name string, role connectome.FileRole) connectome.SourceFile {
	t.Helper()
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return connectome.SourceFile{Role: role, Path: path, Bytes: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), HashStatus: connectome.HashLocallyRecorded}
}

func fixtureGraph(t *testing.T) *connectome.Graph {
	t.Helper()
	dir := t.TempDir()
	writeFeather(t, filepath.Join(dir, "annotations.feather"), arrow.NewSchema([]arrow.Field{
		{Name: "bodyId", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "status", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "class", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "superclass", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "somaSide", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues([]int64{1, 2, 3, 4}, nil)
		b.Field(1).(*array.StringBuilder).AppendValues([]string{"Traced", "Traced", "Traced", "Orphan"}, nil)
		b.Field(2).(*array.StringBuilder).AppendValues([]string{"ALIN", "ALPN", "ALPN", "ALIN"}, nil)
		b.Field(3).(*array.StringBuilder).AppendValues([]string{"cb_intrinsic", "descending_neuron", "descending_neuron", "cb_intrinsic"}, nil)
		b.Field(4).(*array.StringBuilder).AppendValues([]string{"L", "R", "L", "R"}, nil)
	})
	writeFeather(t, filepath.Join(dir, "weights.feather"), arrow.NewSchema([]arrow.Field{
		{Name: "body_pre", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "body_post", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "weight", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues([]int64{1, 2}, nil)
		b.Field(1).(*array.Int64Builder).AppendValues([]int64{2, 3}, nil)
		b.Field(2).(*array.Int64Builder).AppendValues([]int64{2, 3}, nil)
	})
	writeFeather(t, filepath.Join(dir, "nt.feather"), arrow.NewSchema([]arrow.Field{
		{Name: "body", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "consensus_nt", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues([]int64{1}, nil)
		b.Field(1).(*array.StringBuilder).AppendValues([]string{"acetylcholine"}, nil)
	})
	result, err := connectome.Build(context.Background(), connectome.BuildRequest{
		Manifest: connectome.DatasetManifest{
			SchemaVersion: connectome.ManifestSchemaVersion,
			Dataset:       "fixture",
			Namespace:     "modulation-fixture-v1",
			SourceVersion: "v1",
			License:       connectome.License{Name: "CC-BY-4.0", URL: "https://creativecommons.org/licenses/by/4.0/"},
			AcquiredAt:    "2026-09-15T00:00:00Z",
			Files: []connectome.SourceFile{
				fixtureSource(t, dir, "weights.feather", connectome.RoleWeights),
				fixtureSource(t, dir, "annotations.feather", connectome.RoleAnnotations),
				fixtureSource(t, dir, "nt.feather", connectome.RoleNeurotransmitters),
			},
			FieldMapping: connectome.FieldMapping{
				Weights:           connectome.WeightsFields{Source: "body_pre", Target: "body_post", Value: "weight"},
				Annotations:       connectome.AnnotationFields{ID: "bodyId", Status: "status", Class: "class", Superclass: "superclass", SomaSide: "somaSide"},
				Neurotransmitters: connectome.NeurotransmitterFields{ID: "body", Consensus: "consensus_nt"},
			},
			Identity:           connectome.IdentityMapping{WeightsEndpointsAreAnnotationIDs: true, NeurotransmitterIDsAreAnnotationIDs: true, Evidence: "fixture"},
			Selection:          connectome.SelectionPredicate{Source: connectome.RoleAnnotations, Field: "status", Equals: "Traced", Label: "engineering selection"},
			DuplicateSemantics: connectome.DuplicateSemanticsUnknown,
			CoordinateUnit:     "unverified",
			TransformHistory:   []connectome.TransformStep{{Step: "generate", Description: "fixture", Version: "test"}},
		},
		Limits:   connectome.ResourceLimits{MaxMemoryBytes: 1 << 20, MaxTempBytes: 1 << 20, MaxRunFiles: 8, MaxArrowBytes: 1 << 20, MaxFooterBytes: 1 << 20, MaxRows: 1000},
		TempDir:  t.TempDir(),
		EdgeView: connectome.EdgeViewRows,
	})
	if err != nil {
		t.Fatalf("build fixture graph: %v", err)
	}
	return result.Graph
}

// fixtureSet resolves one named set of the fixture graph. "alpn" is nodes 1 and
// 2; "alin" is node 0; "none" is the declared empty intersection.
//
// The package under test no longer names simulate: a neural source carries the
// node indices themselves. The resolver still runs here, in the test, because
// that is how a caller obtains those indices, and because a ResolvedSet keeps
// them private, so the fixture cannot forge a resolution either.
func fixtureSet(t *testing.T, name string) simulate.ResolvedSet {
	t.Helper()
	declared := map[string][]simulate.Selector{
		"alpn": {{Field: "class", Equals: "ALPN"}},
		"alin": {{Field: "class", Equals: "ALIN"}},
		"none": {{Field: "class", Equals: "ALIN", AllowEmpty: true}, {Field: "soma_side", Equals: "R", AllowEmpty: true}},
	}
	selectors, known := declared[name]
	if !known {
		t.Fatalf("unknown fixture set %q", name)
	}
	resolved, err := simulate.ResolveSets(context.Background(), fixtureGraph(t), []simulate.NamedSet{{Name: name, Selectors: selectors}})
	if err != nil {
		t.Fatalf("resolve fixture set %q: %v", name, err)
	}
	return resolved[0]
}

// fixtureNeural builds the neural source of one resolved fixture set, the way a
// caller outside this package does: the ascending node indices of the
// resolution, plus its name as a label for reports.
func fixtureNeural(t *testing.T, name string, gain float64, channel int) NeuralActivity {
	t.Helper()
	set := fixtureSet(t, name)
	return NeuralActivity{Nodes: set.Nodes(), SetName: set.Name, Gain: gain, Channel: channel}
}
