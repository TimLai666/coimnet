package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/media/realdata"
)

var (
	realMediaFitOnce  sync.Once
	realMediaFitValue fitResult
	realMediaFitErr   error
)

func TestRealMediaModelConstructorBindsExistingModelContract(t *testing.T) {
	loaded := realMediaManifest(t)
	config := newMediaModelConfig(loaded.SHA256, loaded.Manifest.Output)
	if config.ManifestSHA256 != loaded.SHA256 {
		t.Fatalf("manifest SHA = %q, want %q", config.ManifestSHA256, loaded.SHA256)
	}
	if config.Output != loaded.Manifest.Output {
		t.Fatalf("output clock = %+v, want %+v", config.Output, loaded.Manifest.Output)
	}
	if config.TextFeatures != textFeatureCount || config.TimeFeatures != timeFeatureCount {
		t.Fatalf("feature dimensions = (%d,%d), want (%d,%d)", config.TextFeatures, config.TimeFeatures, textFeatureCount, timeFeatureCount)
	}
	if config.Seed != modelSeed {
		t.Fatalf("seed = %d, want existing model seed %d", config.Seed, modelSeed)
	}
	if config.ImageScale != imageOutputScale() || config.AudioScale != audioOutputScale() {
		t.Fatalf("decoder scales = (%g,%g), want (%g,%g)", config.ImageScale, config.AudioScale, imageOutputScale(), audioOutputScale())
	}
	for name, profile := range map[string]string{
		"encoder": config.EncoderProfile,
		"decoder": config.DecoderProfile,
		"scale":   config.ScaleProfile,
	} {
		if strings.TrimSpace(profile) == "" {
			t.Fatalf("%s profile is empty", name)
		}
	}

	modelConfig, parameters, err := newModel()
	if err != nil {
		t.Fatal(err)
	}
	if modelConfig.InputSize != config.TextFeatures+config.TimeFeatures || modelConfig.InputSize != modelInputSize {
		t.Fatalf("newModel input size = %d, config declares (%d text + %d time)", modelConfig.InputSize, config.TextFeatures, config.TimeFeatures)
	}
	if modelConfig.OutputSize != frameOutputSize || modelConfig.OutputSize != config.Output.FrameWidth*config.Output.FrameHeight*3+config.Output.AudioSampleRate/config.Output.FrameRate {
		t.Fatalf("newModel output size = %d, config clock implies %d", modelConfig.OutputSize, frameOutputSize)
	}
	if !modelConfig.ReadoutEveryStep || len(modelConfig.InputNodes) != modelEmbed || len(modelConfig.ReadoutNodes) != modelHidden {
		t.Fatalf("newModel readout/input contract changed: %+v", modelConfig)
	}
	if len(parameters.Encoder) != modelInputSize*modelEmbed || len(parameters.Readout) != modelHidden*frameOutputSize {
		t.Fatalf("newModel parameter shapes do not match existing constants: encoder=%d readout=%d", len(parameters.Encoder), len(parameters.Readout))
	}
	trainer, err := learning.NewTrainer(modelConfig, parameters, learning.DefaultOptions())
	if err != nil {
		t.Fatalf("default options rejected existing realmedia model: %v", err)
	}
	if got := trainer.Snapshot().Options; !reflect.DeepEqual(got, learning.DefaultOptions()) {
		t.Fatalf("default options changed while constructing the realmedia model: got %+v want %+v", got, learning.DefaultOptions())
	}
}

func TestRealMediaModelSaveLoadRoundTripPreservesFullTrainingState(t *testing.T) {
	fit := realMediaFit(t)
	config := realMediaConfig(t)
	directory := filepath.Join(t.TempDir(), "model")
	if err := saveMediaModel(context.Background(), directory, config, fit.Snapshot); err != nil {
		t.Fatal(err)
	}
	loaded, verifiedSHA, err := loadMediaModel(context.Background(), directory, config)
	if err != nil {
		t.Fatal(err)
	}
	if !sameRealMediaSnapshot(loaded, fit.Snapshot) {
		t.Fatalf("loaded snapshot differs from the complete trained snapshot\nloaded=%s\nwant=%s", snapshotJSON(t, loaded), snapshotJSON(t, fit.Snapshot))
	}
	trainInputs, trainTargets, err := trainingSequence(realMediaSamples()[0])
	if err != nil {
		t.Fatal(err)
	}
	continuousTrainer, err := learning.RestoreTrainer(fit.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	restoredTrainer, err := learning.RestoreTrainer(loaded)
	if err != nil {
		t.Fatal(err)
	}
	upstream, err := mseGradient(mustPredictAll(t, continuousTrainer, trainInputs), trainTargets)
	if err != nil {
		t.Fatal(err)
	}
	wantStep, err := continuousTrainer.StepFrom(context.Background(), trainInputs, upstream)
	if err != nil {
		t.Fatal(err)
	}
	gotStep, err := restoredTrainer.StepFrom(context.Background(), trainInputs, upstream)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotStep, wantStep) || !sameRealMediaSnapshot(restoredTrainer.Snapshot(), continuousTrainer.Snapshot()) {
		t.Fatalf("next StepFrom after load diverged from uninterrupted trainer: got=%+v want=%+v", gotStep, wantStep)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(manifestBytes)
	if verifiedSHA != hex.EncodeToString(digest[:]) {
		t.Fatalf("verified manifest SHA = %q, want completion manifest digest %q", verifiedSHA, hex.EncodeToString(digest[:]))
	}
	if verifiedSHA == config.ManifestSHA256 {
		t.Fatalf("verified bundle manifest SHA unexpectedly equals source manifest SHA %q", verifiedSHA)
	}
	for _, path := range []string{"manifest.json", "document.json", "arrays.bin"} {
		if _, err := os.Stat(filepath.Join(directory, path)); err != nil {
			t.Fatalf("saved bundle missing %s: %v", path, err)
		}
	}
}

func TestRealMediaModelFitFromSnapshotIsFrozenAndMatchesIndependentInference(t *testing.T) {
	fit := realMediaFit(t)
	samples := realMediaSamples()
	got, err := fitFromSnapshot(context.Background(), samples, fit.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !sameRealMediaSnapshot(got.Before, fit.Snapshot) || !sameRealMediaSnapshot(got.Snapshot, fit.Snapshot) {
		t.Fatalf("restored inference changed the supplied snapshot: before=%d snapshot=%d want=%d", got.Before.Updates, got.Snapshot.Updates, fit.Snapshot.Updates)
	}
	if !reflect.DeepEqual(got.BeforeScores, got.AfterScores) {
		t.Fatalf("restored inference reports different before/after scores: before=%+v after=%+v", got.BeforeScores, got.AfterScores)
	}
	direct, err := predictSamples(context.Background(), fit.Snapshot.Config, fit.Snapshot.Parameters, samples)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Predictions, direct) {
		t.Fatal("fitFromSnapshot predictions differ from direct frozen prediction")
	}
	if got.Snapshot.Updates != fit.Snapshot.Updates || got.Snapshot.Options != fit.Snapshot.Options {
		t.Fatalf("fitFromSnapshot changed update/options state: got updates=%d options=%+v want updates=%d options=%+v", got.Snapshot.Updates, got.Snapshot.Options, fit.Snapshot.Updates, fit.Snapshot.Options)
	}
}

func TestRealMediaModelRejectsInvalidConfigSnapshotAndContextWithoutPartialBundle(t *testing.T) {
	fit := realMediaFit(t)
	validConfig := realMediaConfig(t)
	cases := map[string]struct {
		ctx    context.Context
		dir    string
		config mediaModelConfig
		snap   learning.TrainingSnapshot
	}{
		"nil context": {
			ctx: nil, dir: filepath.Join(t.TempDir(), "nil-context"), config: validConfig, snap: fit.Snapshot,
		},
		"canceled context": {
			ctx: canceledContext(), dir: filepath.Join(t.TempDir(), "canceled"), config: validConfig, snap: fit.Snapshot,
		},
		"invalid manifest SHA": {
			ctx: context.Background(), dir: filepath.Join(t.TempDir(), "bad-source"), config: func() mediaModelConfig { c := validConfig; c.ManifestSHA256 = "not-a-sha"; return c }(), snap: fit.Snapshot,
		},
		"wrong output clock": {
			ctx: context.Background(), dir: filepath.Join(t.TempDir(), "bad-clock"), config: func() mediaModelConfig { c := validConfig; c.Output.FrameRate = 8; return c }(), snap: fit.Snapshot,
		},
		"unknown snapshot schema": {
			ctx: context.Background(), dir: filepath.Join(t.TempDir(), "bad-schema"), config: validConfig, snap: func() learning.TrainingSnapshot { s := fit.Snapshot; s.SchemaVersion = "unknown"; return s }(),
		},
		"optimizer shape": {
			ctx: context.Background(), dir: filepath.Join(t.TempDir(), "bad-optimizer"), config: validConfig, snap: func() learning.TrainingSnapshot { s := fit.Snapshot; s.Optimizer.First = nil; return s }(),
		},
		"config shape": {
			ctx: context.Background(), dir: filepath.Join(t.TempDir(), "bad-config-shape"), config: validConfig, snap: func() learning.TrainingSnapshot { s := fit.Snapshot; s.Config.InputSize++; return s }(),
		},
		"parameter shape": {
			ctx: context.Background(), dir: filepath.Join(t.TempDir(), "bad-parameter-shape"), config: validConfig, snap: func() learning.TrainingSnapshot { s := fit.Snapshot; s.Parameters.Readout = nil; return s }(),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := saveMediaModel(tc.ctx, tc.dir, tc.config, tc.snap)
			if err == nil {
				t.Fatal("invalid model save unexpectedly succeeded")
			}
			rejectSkeletonError(t, err)
			if _, statErr := os.Stat(tc.dir); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("invalid model save left a published or partial directory: %v", statErr)
			}
		})
	}

	directory := filepath.Join(t.TempDir(), "existing")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := []byte("existing destination must not be overwritten")
	markerPath := filepath.Join(directory, "marker")
	if err := os.WriteFile(markerPath, marker, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := saveMediaModel(context.Background(), directory, validConfig, fit.Snapshot); err == nil {
		t.Fatal("existing model directory was overwritten")
	} else {
		rejectSkeletonError(t, err)
	}
	gotMarker, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotMarker, marker) {
		t.Fatal("existing model directory contents changed after rejected save")
	}
}

func TestRealMediaModelLoadRejectsConfigMismatchAndCorruptBundle(t *testing.T) {
	fit := realMediaFit(t)
	expected := realMediaConfig(t)
	directory := filepath.Join(t.TempDir(), "model")
	if err := saveMediaModel(context.Background(), directory, expected, fit.Snapshot); err != nil {
		t.Fatal(err)
	}
	for name, ctx := range map[string]context.Context{"nil context": nil, "canceled context": canceledContext()} {
		t.Run("load "+name, func(t *testing.T) {
			got, verifiedSHA, err := loadMediaModel(ctx, directory, expected)
			if err == nil {
				t.Fatal("invalid load context unexpectedly succeeded")
			}
			rejectSkeletonError(t, err)
			if !reflect.DeepEqual(got, learning.TrainingSnapshot{}) || verifiedSHA != "" {
				t.Fatalf("invalid load context returned partial model state: snapshot updates=%d verified SHA=%q", got.Updates, verifiedSHA)
			}
		})
	}
	mutations := map[string]func(*mediaModelConfig){
		"manifest": func(c *mediaModelConfig) { c.ManifestSHA256 = strings.Repeat("a", 64) },
		"output":   func(c *mediaModelConfig) { c.Output.FrameWidth++ },
		"encoder":  func(c *mediaModelConfig) { c.EncoderProfile += "-other" },
		"decoder":  func(c *mediaModelConfig) { c.DecoderProfile += "-other" },
		"scale":    func(c *mediaModelConfig) { c.ScaleProfile += "-other" },
		"text":     func(c *mediaModelConfig) { c.TextFeatures++ },
		"time":     func(c *mediaModelConfig) { c.TimeFeatures++ },
		"seed":     func(c *mediaModelConfig) { c.Seed++ },
		"image scale": func(c *mediaModelConfig) {
			c.ImageScale *= 2
		},
		"audio scale": func(c *mediaModelConfig) {
			c.AudioScale *= 2
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			candidate := expected
			mutate(&candidate)
			got, verifiedSHA, err := loadMediaModel(context.Background(), directory, candidate)
			if err == nil {
				t.Fatal("mismatched model configuration was accepted")
			}
			rejectSkeletonError(t, err)
			if !reflect.DeepEqual(got, learning.TrainingSnapshot{}) || verifiedSHA != "" {
				t.Fatalf("mismatch returned partial model state: snapshot updates=%d verified SHA=%q", got.Updates, verifiedSHA)
			}
		})
	}

	corruptions := map[string]func(string) error{
		"missing document": func(dir string) error {
			return os.Remove(filepath.Join(dir, "document.json"))
		},
		"corrupt arrays": func(dir string) error {
			path := filepath.Join(dir, "arrays.bin")
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if len(data) == 0 {
				return fmt.Errorf("saved arrays are empty")
			}
			data[0] ^= 0xff
			return os.WriteFile(path, data, 0o600)
		},
		"corrupt manifest": func(dir string) error {
			return os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("{}"), 0o600)
		},
	}
	for name, mutate := range corruptions {
		t.Run(name, func(t *testing.T) {
			copyDir := filepath.Join(t.TempDir(), "corrupt")
			if err := os.CopyFS(copyDir, os.DirFS(directory)); err != nil {
				t.Fatal(err)
			}
			if err := mutate(copyDir); err != nil {
				t.Fatal(err)
			}
			got, verifiedSHA, err := loadMediaModel(context.Background(), copyDir, expected)
			if err == nil {
				t.Fatal("corrupt model bundle was accepted")
			}
			rejectSkeletonError(t, err)
			if !reflect.DeepEqual(got, learning.TrainingSnapshot{}) || verifiedSHA != "" {
				t.Fatalf("corrupt bundle returned partial model state: snapshot updates=%d verified SHA=%q", got.Updates, verifiedSHA)
			}
		})
	}
}

func TestRealMediaModelFitFromSnapshotRejectsNilCancelInvalidSamplesAndSnapshot(t *testing.T) {
	fit := realMediaFit(t)
	canceled := canceledContext()
	cases := map[string]struct {
		ctx      context.Context
		samples  []realdata.Sample
		snapshot learning.TrainingSnapshot
	}{
		"nil context":      {ctx: nil, samples: realMediaSamples(), snapshot: fit.Snapshot},
		"canceled context": {ctx: canceled, samples: realMediaSamples(), snapshot: fit.Snapshot},
		"empty samples":    {ctx: context.Background(), samples: nil, snapshot: fit.Snapshot},
		"invalid snapshot": {
			ctx: context.Background(), samples: realMediaSamples(), snapshot: func() learning.TrainingSnapshot { s := fit.Snapshot; s.SchemaVersion = "unknown"; return s }(),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := fitFromSnapshot(tc.ctx, tc.samples, tc.snapshot)
			if err == nil {
				t.Fatal("invalid restored inference input unexpectedly succeeded")
			}
			rejectSkeletonError(t, err)
			if !reflect.DeepEqual(got, fitResult{}) {
				t.Fatalf("invalid restored inference returned partial report: %+v", got)
			}
		})
	}
}

func realMediaManifest(t *testing.T) realdata.LoadedManifest {
	t.Helper()
	loaded, err := realdata.ParseManifest(manifestJSON)
	if err != nil {
		t.Fatalf("parse embedded realmedia manifest: %v", err)
	}
	if err := validateExampleOutput(loaded.Manifest.Output); err != nil {
		t.Fatalf("embedded realmedia output no longer matches the fixture trainer: %v", err)
	}
	return loaded
}

func realMediaConfig(t *testing.T) mediaModelConfig {
	t.Helper()
	loaded := realMediaManifest(t)
	return newMediaModelConfig(loaded.SHA256, loaded.Manifest.Output)
}

func realMediaSamples() []realdata.Sample {
	return []realdata.Sample{
		sampleForTest("train-source", "train-clip", "train", "a machine in a dark corridor", .7, .12),
		sampleForTest("heldout-source", "test-clip", "test", "a rabbit in a sunny clearing", .25, -.08),
	}
}

func realMediaFit(t *testing.T) fitResult {
	t.Helper()
	realMediaFitOnce.Do(func() {
		realMediaFitValue, realMediaFitErr = fitSamples(context.Background(), realMediaSamples(), 1)
	})
	if realMediaFitErr != nil {
		t.Fatalf("fit licensed realmedia fixture through the learning API: %v", realMediaFitErr)
	}
	return realMediaFitValue
}

func sameRealMediaSnapshot(left, right learning.TrainingSnapshot) bool {
	return reflect.DeepEqual(canonicalRealMediaSnapshot(left), canonicalRealMediaSnapshot(right))
}

func canonicalRealMediaSnapshot(snapshot learning.TrainingSnapshot) learning.TrainingSnapshot {
	if snapshot.Config.Dynamics.Sources == nil {
		snapshot.Config.Dynamics.Sources = []int{}
	}
	if snapshot.Config.Dynamics.Targets == nil {
		snapshot.Config.Dynamics.Targets = []int{}
	}
	if snapshot.Config.Dynamics.Delays == nil {
		snapshot.Config.Dynamics.Delays = []int{}
	}
	if snapshot.Config.ReadoutNodes == nil {
		snapshot.Config.ReadoutNodes = []int{}
	}
	if snapshot.Parameters.Core.Weights == nil {
		snapshot.Parameters.Core.Weights = []float64{}
	}
	if snapshot.Parameters.Core.Bias == nil {
		snapshot.Parameters.Core.Bias = []float64{}
	}
	if snapshot.Parameters.Core.LogTau == nil {
		snapshot.Parameters.Core.LogTau = []float64{}
	}
	if snapshot.Parameters.Encoder == nil {
		snapshot.Parameters.Encoder = []float64{}
	}
	if snapshot.Parameters.Readout == nil {
		snapshot.Parameters.Readout = []float64{}
	}
	if snapshot.Optimizer.First == nil {
		snapshot.Optimizer.First = []float64{}
	}
	if snapshot.Optimizer.Second == nil {
		snapshot.Optimizer.Second = []float64{}
	}
	if snapshot.Optimizer.Steps == nil {
		snapshot.Optimizer.Steps = []uint64{}
	}
	return snapshot
}

func snapshotJSON(t *testing.T, snapshot learning.TrainingSnapshot) string {
	t.Helper()
	data, err := json.Marshal(canonicalRealMediaSnapshot(snapshot))
	if err != nil {
		return fmt.Sprintf("<snapshot marshal error: %v>", err)
	}
	return string(data)
}

func mustPredictAll(t *testing.T, trainer *learning.Trainer, input [][]float64) [][]float64 {
	t.Helper()
	rows, err := trainer.PredictAll(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func rejectSkeletonError(t *testing.T, err error) {
	t.Helper()
	if strings.Contains(err.Error(), "skeleton is unimplemented") {
		t.Fatalf("received the placeholder skeleton error instead of exercising the requested branch: %v", err)
	}
}
