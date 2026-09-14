package simulate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
	"github.com/apache/arrow/go/v17/arrow/ipc"
	"github.com/apache/arrow/go/v17/arrow/memory"
)

// writeFeather writes one record batch as a Feather V2 file, the same shape the
// official MaleCNS sources use. It is the minimal fixture writer for this
// package; the builder itself is covered by the connectome tests.
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
	return connectome.SourceFile{
		Role:       role,
		Path:       path,
		Bytes:      int64(len(data)),
		SHA256:     hex.EncodeToString(sum[:]),
		HashStatus: connectome.HashLocallyRecorded,
	}
}

// fixtureGraph builds the three-neuron graph every test in this package uses.
// Node IDs 1, 2 and 3 are selected in that order, so their continuous indices
// are 0, 1 and 2. Edges are 0->1 with raw weight 2 and 1->2 with raw weight 3.
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
	manifest := connectome.DatasetManifest{
		SchemaVersion: connectome.ManifestSchemaVersion,
		Dataset:       "fixture",
		Namespace:     "simulate-fixture-v1",
		SourceVersion: "v1",
		License:       connectome.License{Name: "CC-BY-4.0", URL: "https://creativecommons.org/licenses/by/4.0/"},
		AcquiredAt:    "2026-09-14T00:00:00Z",
		Files: []connectome.SourceFile{
			fixtureSource(t, dir, "weights.feather", connectome.RoleWeights),
			fixtureSource(t, dir, "annotations.feather", connectome.RoleAnnotations),
			fixtureSource(t, dir, "nt.feather", connectome.RoleNeurotransmitters),
		},
		FieldMapping: connectome.FieldMapping{
			Weights: connectome.WeightsFields{Source: "body_pre", Target: "body_post", Value: "weight"},
			Annotations: connectome.AnnotationFields{
				ID: "bodyId", Status: "status", Class: "class", Superclass: "superclass", SomaSide: "somaSide",
			},
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
			MaxMemoryBytes: 1 << 20, MaxTempBytes: 1 << 20, MaxRunFiles: 8,
			MaxArrowBytes: 1 << 20, MaxFooterBytes: 1 << 20, MaxRows: 1000,
		},
		TempDir:  t.TempDir(),
		EdgeView: connectome.EdgeViewRows,
	})
	if err != nil {
		t.Fatalf("build fixture graph: %v", err)
	}
	if result.Graph.NodeCount() != 3 || result.Graph.EdgeCount() != 2 {
		t.Fatalf("fixture graph has %d nodes and %d edges", result.Graph.NodeCount(), result.Graph.EdgeCount())
	}
	return result.Graph
}

// lifProtocol is the fixture protocol whose probe series are hand calculated in
// the tests: dt 1, tau_syn 1, theta_base 1, v_reset -0.5, no refractory step and
// no adaptation, driven by one two-unit pulse into node 0 at step 0.
func lifProtocol(steps int) Protocol {
	return Protocol{
		SchemaVersion: ProtocolSchemaVersion,
		Core:          CoreLIF,
		LIF: &dynamics.LIFConfig{
			DT: 1, TauSyn: 1, ThetaMin: .5, ThetaMax: 1.5, VReset: -.5,
			RefractorySteps: 0,
			Surrogate:       dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
		},
		Injections: []Injection{{Channel: 0, Node: 0, Gain: 1}},
		Probes: []Probe{
			{Name: "first", Nodes: []int{0}, Reduce: ReduceMeanOutput},
			{Name: "all", Nodes: []int{0, 1, 2}, Reduce: ReduceSumOutput},
			{Name: "spikes", Nodes: []int{0, 1, 2}, Reduce: ReduceSpikeFraction},
			{Name: "counted", Nodes: []int{0, 1, 2}, Reduce: ReduceSpikeCount},
		},
		Stimulus:        StimulusSpec{Pulse: &Pulse{Channels: 1, Steps: steps, Channel: 0, Onset: 0, Duration: 1, Amplitude: 2}},
		ParameterSource: ParameterSourceUniform,
		Uniform:         &UniformParameters{Gain: 2, Bias: 0, LogTau: 0, ThetaRaw: 0},
	}
}
