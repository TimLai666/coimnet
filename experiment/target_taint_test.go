// This file is the runtime half of the SIG-03 separation proof. The compile
// time half is scripts/check-target-flow.sh, which shows that no forward or
// modulation package so much as names signal.Target.
//
// The proof here is a taint run: the episode target is filled with NaN, a value
// that cannot survive any arithmetic, and then every boundary that is supposed
// to be blind to the target is exercised. If any of them could read the target,
// its output would stop being finite.
//
// Function boundaries exercised, in the order the test runs them:
//
//	learning.Trainer.Predict(ctx, input)         forward pass, no target parameter
//	modulation.ExternalTimeline.Release(step, c) declared release timeline
//	modulation.NeuralActivity.Release(step, c)   mean activity of a resolved set
//	modulation.InternalResource.Release(step, c) declared resource rule
//	modulation.Replay.Release(step, c)           recorded release trace
//	learning.Trainer.Step(ctx, input, target)    the one boundary that takes a target
//
// Only the last one takes a target, and it takes it as a plain []float64 that
// reaches the loss function. With a NaN target it fails and commits nothing.
package experiment_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/experiment"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/signal"
	"github.com/TimLai666/coimnet/simulate"
	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
	"github.com/apache/arrow/go/v17/arrow/ipc"
	"github.com/apache/arrow/go/v17/arrow/memory"
)

func TestATaintedTargetReachesTheLossAndNothingElse(t *testing.T) {
	ctx := context.Background()
	trainer, err := experiment.NewDelayedTrainer(7, 0.05, false)
	if err != nil {
		t.Fatalf("build the delayed trainer: %v", err)
	}
	clean := experiment.DelayedEpisode(1001, 0)
	tainted := experiment.DelayedEpisode(1001, 0)
	tainted.Target[0] = math.NaN()
	if !math.IsNaN(tainted.Target[0]) || math.IsNaN(clean.Target[0]) {
		t.Fatal("the taint fixture did not separate the two episodes")
	}

	// Forward: the prediction of the tainted episode is not merely finite, it
	// is the same prediction as the clean episode, because the forward path
	// takes only the input.
	cleanPrediction, err := trainer.Predict(ctx, clean.Input)
	if err != nil {
		t.Fatalf("predict the clean episode: %v", err)
	}
	taintedPrediction, err := trainer.Predict(ctx, tainted.Input)
	if err != nil {
		t.Fatalf("predict the tainted episode: %v", err)
	}
	if !reflect.DeepEqual(cleanPrediction, taintedPrediction) {
		t.Fatalf("prediction changed from %v to %v when only the target was tainted", cleanPrediction, taintedPrediction)
	}
	for i, v := range taintedPrediction {
		if !finite(v) {
			t.Fatalf("prediction %d is %v after the target was tainted", i, v)
		}
	}

	// Modulation: every implemented source runs on the tainted episode. The
	// context it is offered carries the observation, the activity of the step
	// and feedback that has already arrived. None of them can carry a target:
	// the modulation package has no such type.
	observation := taintObservation(t, tainted.Input)
	feedback := taintFeedback(t, 2)
	available, err := signal.AvailableFeedback([]signal.Feedback{feedback}, signal.Timestamp{Value: 2, Unit: modulation.ReleaseTimeUnit})
	if err != nil {
		t.Fatalf("filter feedback: %v", err)
	}
	if len(available) != 1 {
		t.Fatalf("feedback available at step 2 = %d, want 1", len(available))
	}
	c := modulation.SourceContext{
		Observation: &observation,
		Activity:    []float64{taintedPrediction[0], 0.5, 0.25},
		Feedback:    available,
		Resources:   map[string]float64{"energy": 1.25},
	}
	for name, source := range map[string]modulation.Source{
		"external_timeline": modulation.ExternalTimeline{ChannelCount: 2, Entries: []modulation.TimelineEntry{{Step: 2, Channel: 1, Rate: 0.5}}},
		"neural_activity":   taintNeural(t),
		"internal_resource": modulation.InternalResource{Resource: "energy", Coefficient: 2, Threshold: 0.25},
		"replay":            modulation.Replay{Trace: [][]float64{{0, 1.5}, {2, 0}, {0.25, 0.25}}},
	} {
		rates, err := source.Release(2, c)
		if err != nil {
			t.Fatalf("%s: Release() error = %v", name, err)
		}
		if len(rates) != source.Channels() {
			t.Fatalf("%s: Release() returned %d rates for %d channels", name, len(rates), source.Channels())
		}
		for i, v := range rates {
			if !finite(v) || v < 0 {
				t.Fatalf("%s: channel %d released %v on a tainted episode", name, i, v)
			}
		}
	}

	// The loss is the one place a target is allowed to arrive, and it arrives
	// as a plain []float64. A NaN target must stop the update instead of
	// quietly writing NaN parameters.
	before := trainer.Snapshot()
	result, err := trainer.Step(ctx, tainted.Input, tainted.Target)
	if err == nil && finite(result.Loss) {
		t.Fatalf("Step() with a NaN target returned loss %v and no error", result.Loss)
	}
	t.Logf("prediction on the tainted episode %v; Step with the NaN target returned loss %v and error %v", taintedPrediction, result.Loss, err)
	after := trainer.Snapshot()
	if !reflect.DeepEqual(before.Parameters, after.Parameters) || before.Updates != after.Updates {
		t.Fatal("the failed step changed the trainer")
	}

	// The same step with the clean target is a normal update, so the failure
	// above is the target and not the fixture.
	good, err := trainer.Step(ctx, clean.Input, clean.Target)
	if err != nil {
		t.Fatalf("Step() with the clean target: %v", err)
	}
	if !finite(good.Loss) || good.Updates != before.Updates+1 {
		t.Fatalf("clean step returned loss %v after %d updates", good.Loss, good.Updates)
	}
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// taintObservation turns the episode input into an observation. An observation
// cannot hold a target: signal.Target is a different type with its own
// constructor, and NewObservation takes neither.
func taintObservation(t *testing.T, input [][]float64) signal.Observation {
	t.Helper()
	values := make([]float64, 0, len(input))
	for _, step := range input {
		values = append(values, step...)
	}
	s, err := signal.NewSignal(signal.SignalSpec{
		SchemaVersion:  signal.CurrentSchemaVersion(),
		ExperienceID:   "delayed-1001-0",
		StreamID:       "pulse",
		Channel:        "pulse",
		Kind:           signal.KindContinuous,
		Start:          signal.Timestamp{Value: 0, Unit: modulation.ReleaseTimeUnit},
		Duration:       int64(len(input)),
		Shape:          []int{len(input), len(input[0])},
		Values:         values,
		Unit:           signal.UnitNormalized,
		EncoderVersion: signal.CurrentSchemaVersion(),
		SourceSequence: 1,
	})
	if err != nil {
		t.Fatalf("build the observation signal: %v", err)
	}
	o, err := signal.NewObservation(signal.CurrentSchemaVersion(), "delayed-1001-0", "pulse", []signal.Signal{s})
	if err != nil {
		t.Fatalf("build the observation: %v", err)
	}
	return o
}

func taintFeedback(t *testing.T, availableAt int64) signal.Feedback {
	t.Helper()
	f, err := signal.NewFeedback(signal.FeedbackSpec{
		SchemaVersion: signal.CurrentSchemaVersion(),
		ExperienceID:  "delayed-1001-0",
		ActionID:      "action-1",
		ProducedAt:    signal.Timestamp{Value: 1, Unit: modulation.ReleaseTimeUnit},
		AvailableAt:   signal.Timestamp{Value: availableAt, Unit: modulation.ReleaseTimeUnit},
		Source:        "fixture-environment",
		Score:         0.25,
		ModelVersion:  signal.CurrentSchemaVersion(),
	})
	if err != nil {
		t.Fatalf("build the feedback: %v", err)
	}
	return f
}

// taintSet resolves the named set the neural source averages over. The set has
// to be resolved against a real graph: simulate.ResolvedSet keeps its node
// indices private, so a test cannot declare one into existence either.
func taintNeural(t *testing.T) modulation.NeuralActivity {
	t.Helper()
	set := taintSet(t)
	return modulation.NeuralActivity{Nodes: set.Nodes(), SetName: set.Name, Gain: 0.5, Channel: 1}
}

func taintSet(t *testing.T) simulate.ResolvedSet {
	t.Helper()
	resolved, err := simulate.ResolveSets(context.Background(), taintGraph(t), []simulate.NamedSet{
		{Name: "alpn", Selectors: []simulate.Selector{{Field: "class", Equals: "ALPN"}}},
	})
	if err != nil {
		t.Fatalf("resolve the named set: %v", err)
	}
	return resolved[0]
}

// taintGraph is the same three-neuron annotated fixture the simulate package
// builds, rebuilt here because a test helper of another package cannot be
// imported. Bodies 1, 2 and 3 are Traced and become nodes 0, 1 and 2; the ALPN
// set is nodes 1 and 2.
func taintGraph(t *testing.T) *connectome.Graph {
	t.Helper()
	dir := t.TempDir()
	write := func(name string, schema *arrow.Schema, fill func(*array.RecordBuilder)) {
		builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
		fill(builder)
		record := builder.NewRecord()
		builder.Release()
		defer record.Release()
		file, err := os.Create(filepath.Join(dir, name))
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
	source := func(name string, role connectome.FileRole) connectome.SourceFile {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		return connectome.SourceFile{Role: role, Path: filepath.Join(dir, name), Bytes: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), HashStatus: connectome.HashLocallyRecorded}
	}
	write("annotations.feather", arrow.NewSchema([]arrow.Field{
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
	write("weights.feather", arrow.NewSchema([]arrow.Field{
		{Name: "body_pre", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "body_post", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "weight", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues([]int64{1, 2}, nil)
		b.Field(1).(*array.Int64Builder).AppendValues([]int64{2, 3}, nil)
		b.Field(2).(*array.Int64Builder).AppendValues([]int64{2, 3}, nil)
	})
	write("nt.feather", arrow.NewSchema([]arrow.Field{
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
			Namespace:     "target-taint-v1",
			SourceVersion: "v1",
			License:       connectome.License{Name: "CC-BY-4.0", URL: "https://creativecommons.org/licenses/by/4.0/"},
			AcquiredAt:    "2026-09-15T00:00:00Z",
			Files: []connectome.SourceFile{
				source("weights.feather", connectome.RoleWeights),
				source("annotations.feather", connectome.RoleAnnotations),
				source("nt.feather", connectome.RoleNeurotransmitters),
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
		t.Fatalf("build the fixture graph: %v", err)
	}
	return result.Graph
}
