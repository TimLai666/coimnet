package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
	"github.com/apache/arrow/go/v17/arrow/ipc"
	"github.com/apache/arrow/go/v17/arrow/memory"
)

type exampleOutput struct {
	SchemaVersion string `json:"schema_version"`
	Profile       string `json:"profile"`
	NodeCount     uint64 `json:"node_count"`
	EdgeCount     uint64 `json:"edge_count"`
	PredicateHash string `json:"predicate_hash"`
	NeuronIDs     []struct {
		Namespace  string `json:"namespace"`
		ExternalID string `json:"external_id"`
	} `json:"neuron_ids"`
	GraphReport connectome.GraphReport  `json:"graph_report"`
	Store       connectome.StoreReceipt `json:"store"`
	Model       struct {
		Config                     learning.Config  `json:"config"`
		Options                    learning.Options `json:"options"`
		TopologyFingerprint        string           `json:"topology_fingerprint"`
		InitialParameterHash       string           `json:"initial_parameter_hash"`
		FinalParameterHash         string           `json:"final_parameter_hash"`
		InitialWeightsHash         string           `json:"initial_weights_hash"`
		FinalWeightsHash           string           `json:"final_weights_hash"`
		InitialFrozenParameterHash string           `json:"initial_frozen_parameter_hash"`
		FinalFrozenParameterHash   string           `json:"final_frozen_parameter_hash"`
	} `json:"model"`
	Training struct {
		Updates        int     `json:"updates"`
		BeforeMeanLoss float64 `json:"before_mean_loss"`
		AfterMeanLoss  float64 `json:"after_mean_loss"`
		Steps          []struct {
			GradientNorm    float64 `json:"gradient_norm"`
			UpdateNorm      float64 `json:"update_norm"`
			WeightDeltaNorm float64 `json:"weight_delta_norm"`
		} `json:"steps"`
	} `json:"training"`
	Assumptions map[string]string `json:"assumptions"`
}

func TestRunUsageAndArgumentValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "no arguments", want: "Usage: go run ./examples/realsubgraph --store FILE --expected-predicate-hash SHA256"},
		{name: "help", args: []string{"--help"}, want: "--updates"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			err := run(context.Background(), tc.args, &out, &errOut)
			if tc.name == "help" {
				if err != nil {
					t.Fatalf("help: %v", err)
				}
			} else if err == nil {
				t.Fatal("missing arguments accepted")
			}
			if !strings.Contains(out.String(), tc.want) || !strings.Contains(out.String(), "Errors:") || !strings.Contains(out.String(), "Limitations:") {
				t.Fatalf("usage = %q", out.String())
			}
		})
	}

	for _, args := range [][]string{
		{"--store", "store", "--expected-predicate-hash", strings.Repeat("0", 64), "--unknown"},
		{"--store", "store", "--expected-predicate-hash", strings.Repeat("0", 64), "extra"},
		{"--store", "store", "--expected-predicate-hash", "bad"},
		{"--store", "store", "--expected-predicate-hash", strings.Repeat("0", 64), "--updates", "0"},
		{"--store", "store", "--expected-predicate-hash", strings.Repeat("0", 64), "--updates", "1001"},
		{"--store", "store", "--expected-predicate-hash", strings.Repeat("0", 64), "--profile", "wrong"},
	} {
		var out, errOut bytes.Buffer
		if err := run(context.Background(), args, &out, &errOut); err == nil {
			t.Fatalf("accepted invalid arguments %v", args)
		}
	}
}

func TestRunFixtureIsDeterministicAndReadOnly(t *testing.T) {
	path, predicateHash, before := makeFixtureStore(t, fixtureWeights)
	args := []string{"--store", path, "--expected-predicate-hash", predicateHash, "--profile", "fixture", "--updates", "20"}

	var first, firstErr bytes.Buffer
	if err := run(context.Background(), args, &first, &firstErr); err != nil {
		t.Fatalf("first run: %v; stderr=%s", err, firstErr.String())
	}
	if firstErr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", firstErr.String())
	}
	var report exampleOutput
	if err := json.Unmarshal(first.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, first.String())
	}
	if report.SchemaVersion != "coimnet-real-subgraph-example/v1" || report.Profile != "fixture" || report.NodeCount != 3 || report.EdgeCount != 3 || report.PredicateHash != predicateHash || len(report.NeuronIDs) != 3 {
		t.Fatalf("report identity: %#v", report)
	}
	if report.GraphReport.Predicate.Hash != predicateHash || report.GraphReport.Annotated.DuplicatePairs != 0 {
		t.Fatalf("report provenance: %#v", report.GraphReport)
	}
	if report.Store.Path != path || report.Store.Bytes <= 0 || len(report.Store.SHA256) != sha256.Size*2 || report.Store.DurabilityConfirmed {
		t.Fatalf("store receipt: %#v", report.Store)
	}
	if report.Training.Updates != 20 || len(report.Training.Steps) != 20 || report.Training.AfterMeanLoss >= report.Training.BeforeMeanLoss {
		t.Fatalf("training summary: %#v", report.Training)
	}
	sawWeightUpdate := false
	for i, step := range report.Training.Steps {
		for name, value := range map[string]float64{"gradient": step.GradientNorm, "update": step.UpdateNorm, "weight_delta": step.WeightDeltaNorm} {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				t.Fatalf("step %d %s = %g", i, name, value)
			}
		}
		if step.WeightDeltaNorm > 0 {
			sawWeightUpdate = true
		}
	}
	if !sawWeightUpdate {
		t.Fatal("fixture produced no core weight update")
	}
	if report.Model.TopologyFingerprint != "a859e6059ccd8a05b8c8e62511c1b798b31c1a917e84be2ca6e2594a088b5afe" {
		t.Fatalf("model fingerprints: %#v", report.Model)
	}
	wantConfig := learning.Config{
		Dynamics:  dynamics.Config{Nodes: 3, Sources: []int{0, 1, 2}, Targets: []int{1, 2, 0}, Delays: []int{0, 0, 0}, DT: .1, Activation: "tanh"},
		InputSize: 1, OutputSize: 1, ReadoutNodes: []int{0, 1, 2},
	}
	if !reflect.DeepEqual(report.Model.Config, wantConfig) {
		t.Fatalf("topology config = %#v, want %#v", report.Model.Config, wantConfig)
	}
	if report.Model.Options.Trainable != (learning.Trainable{Weights: true}) {
		t.Fatalf("trainable groups = %#v", report.Model.Options.Trainable)
	}
	if len(report.Model.InitialParameterHash) != sha256.Size*2 || len(report.Model.FinalParameterHash) != sha256.Size*2 || report.Model.InitialParameterHash == report.Model.FinalParameterHash || len(report.Model.InitialWeightsHash) != sha256.Size*2 || len(report.Model.FinalWeightsHash) != sha256.Size*2 || report.Model.InitialWeightsHash == report.Model.FinalWeightsHash {
		t.Fatalf("model parameter fingerprints: %#v", report.Model)
	}
	if len(report.Model.InitialFrozenParameterHash) != sha256.Size*2 || len(report.Model.FinalFrozenParameterHash) != sha256.Size*2 || report.Model.InitialFrozenParameterHash != report.Model.FinalFrozenParameterHash {
		t.Fatalf("frozen parameter fingerprints: %#v", report.Model)
	}
	for _, key := range []string{"task", "normalization", "delay", "sign"} {
		if strings.TrimSpace(report.Assumptions[key]) == "" {
			t.Fatalf("missing assumption %q: %#v", key, report.Assumptions)
		}
	}
	for i, id := range report.NeuronIDs {
		if id.Namespace != "fixture-v1" || id.ExternalID == "" {
			t.Fatalf("neuron id %d: %#v", i, id)
		}
	}
	if report.NeuronIDs[2].ExternalID != "9007199254740993" {
		t.Fatalf("large external ID = %q", report.NeuronIDs[2].ExternalID)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("run changed the store bytes")
	}

	var second, secondErr bytes.Buffer
	if err := run(context.Background(), args, &second, &secondErr); err != nil {
		t.Fatalf("second run: %v; stderr=%s", err, secondErr.String())
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatalf("same store produced different JSON\nfirst=%s\nsecond=%s", first.String(), second.String())
	}
}

func TestRunRejectsUnsafeGraphContent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		weight []fixtureWeight
		want   string
	}{
		{name: "duplicate pair", weight: fixtureWeightsDuplicate, want: "duplicate"},
		{name: "null weight", weight: fixtureWeightsNull, want: "null"},
		{name: "negative weight", weight: fixtureWeightsNegative, want: "negative"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, predicateHash, _ := makeFixtureStore(t, tc.weight)
			var out, errOut bytes.Buffer
			err := run(context.Background(), []string{"--store", path, "--expected-predicate-hash", predicateHash, "--profile", "fixture"}, &out, &errOut)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if out.Len() != 0 {
				t.Fatalf("failure wrote stdout: %s", out.String())
			}
		})
	}
}

func TestRunHonorsCancellationWithoutOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, errOut bytes.Buffer
	err := run(ctx, []string{"--store", "store", "--expected-predicate-hash", strings.Repeat("0", 64)}, &out, &errOut)
	if !errors.Is(err, context.Canceled) || out.Len() != 0 {
		t.Fatalf("canceled run: err=%v stdout=%q", err, out.String())
	}
}

func TestRunPropagatesOutputFailure(t *testing.T) {
	path, predicateHash, _ := makeFixtureStore(t, fixtureWeights)
	err := run(context.Background(), []string{"--store", path, "--expected-predicate-hash", predicateHash, "--profile", "fixture", "--updates", "1"}, failingWriter{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "output failed") {
		t.Fatalf("output error = %v", err)
	}
}

func TestRunPropagatesUsageOutputFailure(t *testing.T) {
	if err := run(context.Background(), []string{"--help"}, failingWriter{}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "output failed") {
		t.Fatalf("usage output error = %v", err)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("output failed")
}

type fixtureWeight struct {
	source, target int64
	weight         *int64
}

var (
	fixtureWeightTwo   = int64(2)
	fixtureWeightOne   = int64(1)
	fixtureWeightThree = int64(3)
	fixtureWeightFive  = int64(5)
	fixtureWeights     = []fixtureWeight{
		{102, 103, &fixtureWeightTwo},
		{103, 9007199254740993, &fixtureWeightOne},
		{9007199254740993, 102, &fixtureWeightThree},
	}
	fixtureWeightsDuplicate = []fixtureWeight{
		{102, 103, &fixtureWeightTwo},
		{102, 103, &fixtureWeightOne},
		{103, 9007199254740993, &fixtureWeightThree},
	}
	fixtureWeightsNull = []fixtureWeight{
		{102, 103, nil},
		{103, 9007199254740993, &fixtureWeightOne},
		{9007199254740993, 102, &fixtureWeightThree},
	}
	fixtureWeightsNegative = []fixtureWeight{
		{102, 103, &fixtureWeightFive},
		{103, 9007199254740993, &fixtureWeightOne},
		{9007199254740993, 102, func() *int64 { v := int64(-3); return &v }()},
	}
)

func makeFixtureStore(t *testing.T, weights []fixtureWeight, changes ...func(*connectome.DatasetManifest)) (path, predicateHash string, bytesOnDisk []byte) {
	t.Helper()
	dir := t.TempDir()
	annotationsPath := filepath.Join(dir, "annotations.feather")
	weightsPath := filepath.Join(dir, "weights.feather")
	neurotransmittersPath := filepath.Join(dir, "neurotransmitters.feather")
	writeFixtureAnnotations(t, annotationsPath)
	writeFixtureWeights(t, weightsPath, weights)
	writeFixtureNeurotransmitters(t, neurotransmittersPath)
	manifest := connectome.DatasetManifest{
		SchemaVersion: connectome.ManifestSchemaVersion,
		Dataset:       "fixture",
		Namespace:     "fixture-v1",
		SourceVersion: "v1",
		License:       connectome.License{Name: "CC-BY-4.0", URL: "https://creativecommons.org/licenses/by/4.0/"},
		AcquiredAt:    "2026-09-13T00:00:00Z",
		Files: []connectome.SourceFile{
			fixtureSource(t, connectome.RoleWeights, weightsPath),
			fixtureSource(t, connectome.RoleAnnotations, annotationsPath),
			fixtureSource(t, connectome.RoleNeurotransmitters, neurotransmittersPath),
		},
		FieldMapping: connectome.FieldMapping{
			Weights:           connectome.WeightsFields{Source: "body_pre", Target: "body_post", Value: "weight"},
			Annotations:       connectome.AnnotationFields{ID: "bodyId", Class: "class"},
			Neurotransmitters: connectome.NeurotransmitterFields{ID: "body"},
		},
		Identity:           connectome.IdentityMapping{WeightsEndpointsAreAnnotationIDs: true, NeurotransmitterIDsAreAnnotationIDs: true, Evidence: "fixture uses one declared body ID space"},
		Selection:          connectome.SelectionPredicate{Source: connectome.RoleAnnotations, Field: "class", Equals: "ALIN", Label: "fixture class selection"},
		DuplicateSemantics: connectome.DuplicateSemanticsUnknown,
		CoordinateUnit:     "unverified",
		TransformHistory:   []connectome.TransformStep{{Step: "generate", Description: "synthetic fixture", Version: "test"}},
	}
	for _, change := range changes {
		change(&manifest)
	}
	predicateHash, _ = manifest.Selection.Hash()
	result, err := connectome.Build(context.Background(), connectome.BuildRequest{
		Manifest: manifest,
		Limits:   connectome.ResourceLimits{MaxMemoryBytes: 64 << 20, MaxTempBytes: 64 << 20, MaxRunFiles: 64, MaxArrowBytes: 16 << 20, MaxFooterBytes: 1 << 20, MaxRows: 1 << 20},
		TempDir:  t.TempDir(),
		EdgeView: connectome.EdgeViewRows,
	})
	if err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(dir, "graph.coimgraph")
	if _, err := connectome.Save(context.Background(), path, result.Graph); err != nil {
		t.Fatal(err)
	}
	bytesOnDisk, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, predicateHash, bytesOnDisk
}

func fixtureSource(t *testing.T, role connectome.FileRole, path string) connectome.SourceFile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return connectome.SourceFile{Role: role, Path: path, Bytes: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), HashStatus: connectome.HashLocallyRecorded}
}

func writeFixtureAnnotations(t *testing.T, path string) {
	t.Helper()
	schema := arrow.NewSchema([]arrow.Field{{Name: "bodyId", Type: arrow.PrimitiveTypes.Int64}, {Name: "class", Type: arrow.BinaryTypes.String}}, nil)
	writeFixtureFeather(t, path, schema, func(builder *array.RecordBuilder) {
		ids := builder.Field(0).(*array.Int64Builder)
		classes := builder.Field(1).(*array.StringBuilder)
		for _, id := range []int64{102, 103, 9007199254740993} {
			ids.Append(id)
			classes.Append("ALIN")
		}
	})
}

func writeFixtureWeights(t *testing.T, path string, rows []fixtureWeight) {
	t.Helper()
	schema := arrow.NewSchema([]arrow.Field{{Name: "body_pre", Type: arrow.PrimitiveTypes.Int64}, {Name: "body_post", Type: arrow.PrimitiveTypes.Int64}, {Name: "weight", Type: arrow.PrimitiveTypes.Int64, Nullable: true}}, nil)
	writeFixtureFeather(t, path, schema, func(builder *array.RecordBuilder) {
		sources := builder.Field(0).(*array.Int64Builder)
		targets := builder.Field(1).(*array.Int64Builder)
		weights := builder.Field(2).(*array.Int64Builder)
		for _, row := range rows {
			sources.Append(row.source)
			targets.Append(row.target)
			if row.weight == nil {
				weights.AppendNull()
			} else {
				weights.Append(*row.weight)
			}
		}
	})
}

func writeFixtureNeurotransmitters(t *testing.T, path string) {
	t.Helper()
	schema := arrow.NewSchema([]arrow.Field{{Name: "body", Type: arrow.PrimitiveTypes.Int64}}, nil)
	writeFixtureFeather(t, path, schema, func(builder *array.RecordBuilder) {
		ids := builder.Field(0).(*array.Int64Builder)
		ids.AppendValues([]int64{102, 103, 9007199254740993}, nil)
	})
}

func writeFixtureFeather(t *testing.T, path string, schema *arrow.Schema, fill func(*array.RecordBuilder)) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	fill(builder)
	record := builder.NewRecord()
	builder.Release()
	writer, err := ipc.NewFileWriter(file, ipc.WithSchema(schema))
	if err != nil {
		record.Release()
		file.Close()
		t.Fatal(err)
	}
	if err := writer.Write(record); err != nil {
		record.Release()
		writer.Close()
		file.Close()
		t.Fatal(err)
	}
	record.Release()
	if err := writer.Close(); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
