package learning

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/TimLai666/coimnet/backend/webgpu"
	"github.com/TimLai666/coimnet/dynamics"
)

func TestGPUDeviceTrainerMatchesCPUAndUsesUpdatedParameters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	c, p, o := gpuTrainerFixture()
	o.LearningRate = 0.037
	o.WeightDecay = 0.01
	o.ClipNorm = 0.4
	o.Masks = &UpdateMasks{
		Edges: []bool{true, false, true, true, false},
		Nodes: []bool{true, false, true, true},
	}
	o.Schedule = &Schedule{Kind: ScheduleConstant, WarmupUpdates: 1}
	cpu, err := NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cpu.GPUAdamWMetrics(); ok {
		t.Fatal("CPU trainer reported device Adam metrics")
	}
	gpu := newGPUDeviceTrainerOrSkip(t, ctx, c, p, o)
	defer func() {
		if err := gpu.Close(); err != nil {
			t.Errorf("GPU device trainer Close: %v", err)
		}
	}()
	legacy := newGPUTrainerOrSkip(t, ctx, c, p, o)
	defer func() {
		if err := legacy.Close(); err != nil {
			t.Errorf("legacy GPU trainer Close: %v", err)
		}
	}()
	if _, ok := legacy.GPUAdamWMetrics(); ok {
		t.Fatal("legacy GPU trainer reported device Adam metrics")
	}

	metrics, ok := gpu.GPUAdamWMetrics()
	if !ok {
		t.Fatal("GPUAdamWMetrics reported a CPU or legacy GPU trainer")
	}
	if metrics.Values <= 0 || metrics.AllocatedBytes == 0 {
		t.Fatalf("GPUAdamWMetrics has no device optimizer allocation: %+v", metrics)
	}
	if metrics.Device.Adapter == "" || metrics.Device.Backend == "" {
		t.Fatalf("GPUAdamWMetrics has no hardware identity: %+v", metrics.Device)
	}
	if info, ok := gpu.GPUAdapterInfo(); !ok || info.Forward.Adapter == "" || info.Backward.Adapter == "" {
		t.Fatalf("GPUAdapterInfo = %+v, %v; want forward and backward adapters", info, ok)
	}

	input := gpuTrainerInput()
	before, err := gpu.Predict(ctx, input)
	if err != nil {
		t.Fatalf("initial GPU Predict: %v", err)
	}
	wantBefore, err := cpu.Predict(ctx, input)
	if err != nil {
		t.Fatalf("initial CPU Predict: %v", err)
	}
	assertGPUValuesNear(t, "initial prediction", before, wantBefore, 5e-5, 5e-4)
	legacyBefore, err := legacy.Predict(ctx, input)
	if err != nil {
		t.Fatalf("initial legacy GPU Predict: %v", err)
	}
	assertGPUValuesNear(t, "initial legacy GPU prediction", legacyBefore, wantBefore, 5e-5, 5e-4)
	initialSnapshot := gpu.Snapshot()

	for step := range 4 {
		target := []float64{0.25 - 0.13*float64(step)}
		want, err := cpu.Step(ctx, input, target)
		if err != nil {
			t.Fatalf("CPU Step %d: %v", step, err)
		}
		legacyGot, err := legacy.Step(ctx, input, target)
		if err != nil {
			t.Fatalf("legacy GPU Step %d: %v", step, err)
		}
		got, err := gpu.Step(ctx, input, target)
		if err != nil {
			t.Fatalf("GPU device Step %d: %v", step, err)
		}
		if !want.Applied || !legacyGot.Applied || !got.Applied || legacyGot.Updates != want.Updates || got.Updates != want.Updates {
			t.Fatalf("Step %d status differs: CPU=%+v legacy GPU=%+v device GPU=%+v", step, want, legacyGot, got)
		}
		wantRate := float64(float32(LearningRateAt(o, uint64(step))))
		if got.LearningRate != wantRate {
			t.Fatalf("GPU Step %d LearningRate = %.9g, want actual float32 rate %.9g", step, got.LearningRate, wantRate)
		}
		assertGPUTrainerStateNear(t, cpu.Snapshot(), legacy.Snapshot())
		assertGPUDeviceTrainerStateNear(t, cpu.Snapshot(), gpu.Snapshot())
	}

	finalSnapshot := gpu.Snapshot()
	if reflect.DeepEqual(initialSnapshot.Parameters, finalSnapshot.Parameters) {
		t.Fatal("GPU device training did not change parameters")
	}
	after, err := gpu.Predict(ctx, input)
	if err != nil {
		t.Fatalf("final GPU Predict: %v", err)
	}
	if sameFloat64Vector(after, before, 1e-10) {
		t.Fatalf("prediction did not use updated device parameters: before=%v after=%v", before, after)
	}

	for edge, enabled := range o.Masks.Edges {
		if enabled {
			continue
		}
		if finalSnapshot.Optimizer.Steps[edge] != 0 || finalSnapshot.Optimizer.First[edge] != 0 || finalSnapshot.Optimizer.Second[edge] != 0 {
			t.Fatalf("frozen edge %d accumulated optimizer state: %+v", edge, finalSnapshot.Optimizer)
		}
		if finalSnapshot.Parameters.Core.Weights[edge] != float64(float32(p.Core.Weights[edge])) {
			t.Fatalf("frozen edge %d changed: got %.9g want float32 initial %.9g", edge, finalSnapshot.Parameters.Core.Weights[edge], float64(float32(p.Core.Weights[edge])))
		}
	}
}

func TestNewGPUDeviceTrainerQuantizesKernelOptionsAndRejectsFloat32Boundary(t *testing.T) {
	c, p, o := gpuTrainerFixture()
	cases := []struct {
		name  string
		field string
		edit  func(*Options)
	}{
		{name: "beta1 rounds to one", field: "beta1", edit: func(o *Options) { o.Beta1 = math.Nextafter(1, 0) }},
		{name: "beta2 rounds to one", field: "beta2", edit: func(o *Options) { o.Beta2 = math.Nextafter(1, 0) }},
		{name: "epsilon underflows to zero", field: "epsilon", edit: func(o *Options) { o.Epsilon = float64(math.Float32frombits(1)) / 2 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			options := o
			tc.edit(&options)
			trainer, err := NewGPUDeviceTrainer(context.Background(), c, p, options, 1<<20)
			if trainer != nil {
				_ = trainer.Close()
				t.Fatalf("NewGPUDeviceTrainer accepted %s after float32 conversion", tc.field)
			}
			if errors.Is(err, webgpu.ErrUnavailable) {
				t.Fatalf("%s preflight reached hardware setup: %v", tc.field, err)
			}
			if !errors.Is(err, webgpu.ErrInvalidConfig) {
				t.Fatalf("%s error = %v, want webgpu.ErrInvalidConfig", tc.field, err)
			}
			if !strings.Contains(strings.ToLower(err.Error()), tc.field) {
				t.Fatalf("%s error = %q, want the rejected option name", tc.field, err)
			}
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	o.Beta1 = 0.9000000007
	o.Beta2 = 0.8000000007
	o.Epsilon = 0.0010000007
	o.WeightDecay = 0.0123456789
	trainer := newGPUDeviceTrainerOrSkip(t, ctx, c, p, o)
	defer trainer.Close()
	snapshot := trainer.Snapshot()
	for _, field := range []struct {
		name string
		got  float64
		want float64
	}{
		{name: "beta1", got: snapshot.Options.Beta1, want: o.Beta1},
		{name: "beta2", got: snapshot.Options.Beta2, want: o.Beta2},
		{name: "epsilon", got: snapshot.Options.Epsilon, want: o.Epsilon},
		{name: "weight decay", got: snapshot.Options.WeightDecay, want: o.WeightDecay},
	} {
		if field.got != float64(float32(field.want)) {
			t.Fatalf("snapshot option %s = %.17g, want exact float32 value %.17g", field.name, field.got, float64(float32(field.want)))
		}
	}
}

func TestGPUDeviceTrainerAccumulatesMasksClipAndScheduleZero(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	c, p, o := gpuTrainerFixture()
	o.LearningRate = 0.031
	o.ClipNorm = 0.01
	o.AccumulateSteps = 2
	o.Schedule = &Schedule{Kind: ScheduleConstant, WarmupUpdates: 1}
	o.Masks = &UpdateMasks{Edges: []bool{true, false, true, false, true}, Nodes: []bool{true, false, true, false}}
	cpu, err := NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	gpu := newGPUDeviceTrainerOrSkip(t, ctx, c, p, o)
	defer gpu.Close()
	input := gpuTrainerInput()
	targets := [][]float64{{0.4}, {-0.3}, {0.2}, {-0.1}}

	for i := range targets {
		want, err := cpu.Step(ctx, input, targets[i])
		if err != nil {
			t.Fatalf("CPU Step %d: %v", i, err)
		}
		got, err := gpu.Step(ctx, input, targets[i])
		if err != nil {
			t.Fatalf("GPU device Step %d: %v", i, err)
		}
		if got.Applied != want.Applied || got.Updates != want.Updates || got.Accumulated != want.Accumulated {
			t.Fatalf("Step %d accumulation differs: CPU=%+v GPU=%+v", i, want, got)
		}
		if i == 0 {
			if got.Applied || got.Updates != 0 || got.Accumulated != 1 {
				t.Fatalf("partial accumulation Step = %+v, want no update and one stored gradient", got)
			}
			if snapshot := gpu.Snapshot(); snapshot.Accumulator == nil || snapshot.Accumulator.Count != 1 {
				t.Fatalf("partial GPU snapshot accumulator = %+v, want count 1", snapshot.Accumulator)
			}
		}
		if i == 1 {
			if !got.Applied || got.Updates != 1 || got.LearningRate != 0 {
				t.Fatalf("zero-rate applied Step = %+v, want update 1 at rate 0", got)
			}
		}
		if i == 3 && got.LearningRate != float64(float32(o.LearningRate)) {
			t.Fatalf("post-warmup LearningRate = %.9g, want %.9g", got.LearningRate, float64(float32(o.LearningRate)))
		}
		assertGPUDeviceTrainerStateNear(t, cpu.Snapshot(), gpu.Snapshot())
	}
	final := gpu.Snapshot()
	for edge, enabled := range o.Masks.Edges {
		if enabled {
			continue
		}
		if final.Optimizer.Steps[edge] != 0 || final.Optimizer.First[edge] != 0 || final.Optimizer.Second[edge] != 0 {
			t.Fatalf("frozen edge %d changed during accumulation: %+v", edge, final.Optimizer)
		}
	}
}

func TestGPUDeviceTrainerCandidateRollbackPreservesAccumulator(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	c, p, o := gpuTrainerFixture()
	o.LearningRate = 0.01
	o.WeightDecay = 0
	o.ClipNorm = 0
	o.AccumulateSteps = 2
	bad := newGPUDeviceTrainerOrSkip(t, ctx, c, p, o)
	defer bad.Close()
	reference := newGPUDeviceTrainerOrSkip(t, ctx, c, p, o)
	defer reference.Close()
	input := gpuTrainerInput()
	normalUpstream := [][]float64{{0}, {0}, {0}, {0.5}}
	// Keep the reverse pass finite while making the Adam second moment overflow
	// in the device candidate. The failed window must remain reusable.
	extremeUpstream := [][]float64{{0}, {0}, {0}, {float64(math.MaxFloat32) * 0.5}}
	for name, trainer := range map[string]*Trainer{"bad": bad, "reference": reference} {
		result, err := trainer.StepFrom(ctx, input, normalUpstream)
		if err != nil {
			t.Fatalf("%s prefix StepFrom: %v", name, err)
		}
		if result.Applied || result.Accumulated != 1 {
			t.Fatalf("%s prefix StepFrom = %+v, want one pending gradient", name, result)
		}
	}
	before := bad.Snapshot()
	beforeMetrics, ok := bad.GPUAdamWMetrics()
	if !ok {
		t.Fatal("candidate rollback fixture did not report device Adam metrics")
	}
	if _, err := bad.StepFrom(ctx, input, extremeUpstream); err == nil {
		t.Fatal("extreme StepFrom unexpectedly succeeded")
	} else if !errors.Is(err, webgpu.ErrNonFinite) && !strings.Contains(strings.ToLower(err.Error()), "non-finite") {
		t.Fatalf("extreme StepFrom error = %v, want a non-finite candidate error", err)
	}
	afterMetrics, ok := bad.GPUAdamWMetrics()
	if !ok || afterMetrics.BytesUploaded <= beforeMetrics.BytesUploaded || afterMetrics.Dispatch == 0 {
		t.Fatalf("extreme StepFrom did not reach a device candidate dispatch: before=%+v after=%+v", beforeMetrics, afterMetrics)
	}
	if after := bad.Snapshot(); !reflect.DeepEqual(after, before) {
		t.Fatalf("candidate rejection changed full trainer snapshot:\nbefore=%+v\nafter=%+v", before, after)
	}

	if _, err := bad.StepFrom(ctx, input, normalUpstream); err != nil {
		t.Fatalf("bad trainer legal continuation: %v", err)
	}
	if _, err := reference.StepFrom(ctx, input, normalUpstream); err != nil {
		t.Fatalf("reference legal continuation: %v", err)
	}
	if got, want := bad.Snapshot(), reference.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("legal continuation after rejection differs from no-failure reference:\ngot=%+v\nwant=%+v", got, want)
	}
}

func TestGPUDeviceTrainerCloseIsIdempotentAndSnapshotReadable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	c, p, o := gpuTrainerFixture()
	trainer := newGPUDeviceTrainerOrSkip(t, ctx, c, p, o)
	if _, err := trainer.Step(ctx, gpuTrainerInput(), []float64{0.2}); err != nil {
		_ = trainer.Close()
		t.Fatalf("baseline Step: %v", err)
	}
	before := trainer.Snapshot()
	if err := trainer.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := trainer.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := trainer.Step(ctx, gpuTrainerInput(), []float64{0.1}); err == nil {
		t.Fatal("Step after Close unexpectedly succeeded")
	} else if !strings.Contains(strings.ToLower(err.Error()), "closed") {
		t.Fatalf("Step after Close error = %v, want a closed-trainer error", err)
	}
	if got := trainer.Snapshot(); !reflect.DeepEqual(got, before) {
		t.Fatalf("Snapshot after Close changed trainer state:\nbefore=%+v\nafter=%+v", before, got)
	}
}

func TestNewGPUDeviceTrainerRejectsUnsupportedBeforeDeviceOpen(t *testing.T) {
	c, p, o := gpuTrainerFixture()
	cases := []struct {
		name string
		edit func(*Config, *Options)
	}{
		{name: "ranges", edit: func(_ *Config, o *Options) { o.Ranges = &ParameterRanges{WeightMagnitudeMax: 1} }},
		{name: "fixed signs", edit: func(c *Config, _ *Options) { c.EdgeSigns = []int8{1, 0, -1, 0, 0} }},
		{name: "sharing", edit: func(c *Config, _ *Options) { c.Sharing = &ParameterSharing{Weights: []int32{0, 1, 2, 3, 4}, Groups: 5} }},
		{name: "LIF", edit: func(c *Config, _ *Options) { c.LIF = &dynamics.LIFConfig{} }},
		{name: "mixed", edit: func(c *Config, _ *Options) { c.Mixed = &dynamics.MixedConfig{} }},
		{name: "vector state", edit: func(c *Config, _ *Options) { c.Dynamics.StateDimension = 2 }},
		{name: "matrix edges", edit: func(c *Config, _ *Options) { c.Dynamics.EdgeShape = "matrix" }},
		{name: "positive delay", edit: func(c *Config, _ *Options) { c.Dynamics.Delays = []int{1, 0, 0, 0, 0} }},
		{name: "recompute", edit: func(_ *Config, o *Options) { o.Recompute = &Recompute{SegmentSteps: 1} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			configured, options := c, o
			configured.Dynamics = c.Dynamics
			configured.Dynamics.Delays = append([]int(nil), c.Dynamics.Delays...)
			configured.EdgeSigns = append([]int8(nil), c.EdgeSigns...)
			tc.edit(&configured, &options)
			trainer, err := NewGPUDeviceTrainer(context.Background(), configured, p, options, 1<<20)
			if trainer != nil {
				_ = trainer.Close()
				t.Fatal("NewGPUDeviceTrainer returned a trainer for an unsupported configuration")
			}
			if errors.Is(err, webgpu.ErrUnavailable) {
				t.Fatalf("unsupported configuration reached hardware setup: %v", err)
			}
			if !errors.Is(err, ErrGPUTrainerUnsupported) {
				t.Fatalf("NewGPUDeviceTrainer() error = %v, want ErrGPUTrainerUnsupported", err)
			}
		})
	}

	trainer, err := NewGPUDeviceTrainer(context.Background(), c, p, o, 1)
	if trainer != nil {
		_ = trainer.Close()
		t.Fatal("NewGPUDeviceTrainer accepted an optimizer budget below the resident minimum")
	}
	if errors.Is(err, webgpu.ErrUnavailable) {
		t.Fatalf("too-small optimizer budget reached hardware setup: %v", err)
	}
	if !errors.Is(err, webgpu.ErrCapacity) {
		t.Fatalf("too-small optimizer budget error = %v, want ErrCapacity", err)
	}
}

func TestRestoreGPUDeviceTrainerContinuesPartialAccumulationAcrossProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	c, p, o := gpuTrainerFixture()
	o.LearningRate = 0.023
	o.AccumulateSteps = 3
	o.Schedule = &Schedule{Kind: ScheduleConstant, WarmupUpdates: 1}
	continuous := newGPUDeviceTrainerOrSkip(t, ctx, c, p, o)
	defer continuous.Close()
	input := gpuTrainerInput()
	if result, err := continuous.Step(ctx, input, []float64{0.4}); err != nil {
		t.Fatalf("prefix Step: %v", err)
	} else if result.Applied || result.Accumulated != 1 {
		t.Fatalf("prefix Step = %+v, want partial accumulation", result)
	}
	saved := continuous.Snapshot()
	if saved.Accumulator == nil || saved.Accumulator.Count != 1 {
		t.Fatalf("saved partial accumulation = %+v, want count 1", saved.Accumulator)
	}

	resumed, err := RestoreGPUDeviceTrainer(ctx, saved, 1<<20)
	if err != nil {
		t.Fatalf("same-process RestoreGPUDeviceTrainer: %v", err)
	}
	defer resumed.Close()
	suffix := [][]float64{{-0.2}, {0.3}, {-0.1}}
	for i, target := range suffix {
		if _, err := continuous.Step(ctx, input, target); err != nil {
			t.Fatalf("continuous suffix Step %d: %v", i, err)
		}
		if _, err := resumed.Step(ctx, input, target); err != nil {
			t.Fatalf("same-process resumed suffix Step %d: %v", i, err)
		}
		if got, want := resumed.Snapshot(), continuous.Snapshot(); !reflect.DeepEqual(got, want) {
			t.Fatalf("same-process restored state differs at suffix Step %d", i)
		}
	}
	wantPrediction, err := continuous.Predict(ctx, input)
	if err != nil {
		t.Fatalf("continuous final Predict: %v", err)
	}

	payload := gpuDeviceTrainerChildPayload{Snapshot: saved, Input: input, Targets: suffix}
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "training-snapshot.json")
	outputPath := filepath.Join(directory, "training-result.json")
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inputPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	childContext, childCancel := context.WithTimeout(ctx, 90*time.Second)
	defer childCancel()
	command := exec.CommandContext(childContext, executable, "-test.run=^TestGPUDeviceTrainerChildProcess$", "-test.v")
	command.Env = append(os.Environ(), gpuDeviceTrainerChildInputEnv+"="+inputPath, gpuDeviceTrainerChildOutputEnv+"="+outputPath)
	childOutput, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("new-process GPU device restore: %v\n%s", err, childOutput)
	}
	var childResult gpuDeviceTrainerChildResult
	childJSON, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(childJSON, &childResult); err != nil {
		t.Fatal(err)
	}
	if want := continuous.Snapshot(); !reflect.DeepEqual(childResult.Snapshot, want) {
		t.Fatal("new-process partial GPU restore differs from uninterrupted state")
	}
	if !reflect.DeepEqual(childResult.Prediction, wantPrediction) {
		t.Fatalf("new-process restored prediction differs: got=%v want=%v", childResult.Prediction, wantPrediction)
	}
}

func TestRestoreGPUDeviceTrainerRejectsUnrepresentableStateAndCounterOverflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	c, p, o := gpuTrainerFixture()
	trainer := newGPUDeviceTrainerOrSkip(t, ctx, c, p, o)
	defer trainer.Close()
	if _, err := trainer.Step(ctx, gpuTrainerInput(), []float64{0.2}); err != nil {
		t.Fatalf("baseline GPU device Step: %v", err)
	}
	base := trainer.Snapshot()
	for _, field := range []struct {
		name  string
		value float64
	}{
		{name: "beta1", value: base.Options.Beta1},
		{name: "beta2", value: base.Options.Beta2},
		{name: "epsilon", value: base.Options.Epsilon},
		{name: "weight decay", value: base.Options.WeightDecay},
	} {
		if field.value != float64(float32(field.value)) {
			t.Fatalf("actual GPU snapshot option %s = %.17g is not exact float32", field.name, field.value)
		}
	}

	unrepresentable := cloneTrainingSnapshotForGPUDeviceTest(t, base)
	unrepresentable.Parameters.Core.Weights[0] = nextNonFloat32(unrepresentable.Parameters.Core.Weights[0])
	requireGPUDeviceRestoreRejected(t, "nonrepresentable parameter", unrepresentable, "parameter", "float32", "represent")

	trainedState := cloneTrainingSnapshotForGPUDeviceTest(t, base)
	trainedState.Optimizer.First[0] = nextNonFloat32(0.125)
	requireGPUDeviceRestoreRejected(t, "nonrepresentable first moment", trainedState, "first", "float32", "represent")

	nonrepresentableBeta := cloneTrainingSnapshotForGPUDeviceTest(t, base)
	nonrepresentableBeta.Options.Beta1 = nextNonFloat32(base.Options.Beta1)
	requireGPUDeviceRestoreRejected(t, "nonrepresentable beta1", nonrepresentableBeta, "beta1", "float32", "represent")

	underflowEpsilon := cloneTrainingSnapshotForGPUDeviceTest(t, base)
	underflowEpsilon.Options.Epsilon = float64(math.Float32frombits(1)) / 2
	requireGPUDeviceRestoreRejected(t, "epsilon underflow", underflowEpsilon, "epsilon", "zero", "float32")

	overflow := cloneTrainingSnapshotForGPUDeviceTest(t, base)
	overflow.Updates = uint64(math.MaxUint32) + 1
	overflow.Optimizer.Steps[0] = uint64(math.MaxUint32) + 1
	requireGPUDeviceRestoreRejected(t, "uint32 counter overflow", overflow, "counter", "uint32", "overflow")
}

const (
	gpuDeviceTrainerChildInputEnv  = "COIMNET_GPU_DEVICE_TRAINER_INPUT"
	gpuDeviceTrainerChildOutputEnv = "COIMNET_GPU_DEVICE_TRAINER_OUTPUT"
)

type gpuDeviceTrainerChildPayload struct {
	Snapshot TrainingSnapshot `json:"snapshot"`
	Input    [][]float64      `json:"input"`
	Targets  [][]float64      `json:"targets"`
}

type gpuDeviceTrainerChildResult struct {
	Snapshot   TrainingSnapshot `json:"snapshot"`
	Prediction []float64        `json:"prediction"`
}

// This child process proves JSON restore of the learning component only. The
// actual training bundle integration stays outside learning to avoid a
// checkpoint-package import cycle and is covered by the root integration flow.
func TestGPUDeviceTrainerChildProcess(t *testing.T) {
	inputPath := os.Getenv(gpuDeviceTrainerChildInputEnv)
	if inputPath == "" {
		t.Skip("intentional subprocess restore helper")
	}
	outputPath := os.Getenv(gpuDeviceTrainerChildOutputEnv)
	if outputPath == "" {
		t.Fatal("missing child output path")
	}
	encoded, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	var payload gpuDeviceTrainerChildPayload
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	trainer, err := RestoreGPUDeviceTrainer(ctx, payload.Snapshot, 1<<20)
	if errors.Is(err, webgpu.ErrUnavailable) {
		t.Fatalf("child restore must not hardware-skip: %v", err)
	}
	if err != nil {
		t.Fatalf("child RestoreGPUDeviceTrainer: %v", err)
	}
	defer trainer.Close()
	for i, target := range payload.Targets {
		if _, err := trainer.Step(ctx, payload.Input, target); err != nil {
			t.Fatalf("child suffix Step %d: %v", i, err)
		}
	}
	prediction, err := trainer.Predict(ctx, payload.Input)
	if err != nil {
		t.Fatalf("child final Predict: %v", err)
	}
	result, err := json.Marshal(gpuDeviceTrainerChildResult{Snapshot: trainer.Snapshot(), Prediction: prediction})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outputPath, result, 0o600); err != nil {
		t.Fatal(err)
	}
}

func newGPUDeviceTrainerOrSkip(t *testing.T, ctx context.Context, c Config, p Parameters, o Options) *Trainer {
	t.Helper()
	trainer, err := NewGPUDeviceTrainer(ctx, c, p, o, 1<<20)
	if errors.Is(err, webgpu.ErrUnavailable) {
		t.Skipf("GPU unavailable: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	return trainer
}

func assertGPUDeviceTrainerStateNear(t *testing.T, want, got TrainingSnapshot) {
	t.Helper()
	if !reflect.DeepEqual(want.Config, got.Config) || want.Updates != got.Updates {
		t.Fatalf("GPU device trainer metadata differs: want config/updates=%+v/%d got=%+v/%d", want.Config, want.Updates, got.Config, got.Updates)
	}
	assertGPUValuesNear(t, "device parameters", flatParameters(got.Parameters), flatParameters(want.Parameters), 7e-5, 7e-4)
	assertGPUValuesNear(t, "device Adam first moments", got.Optimizer.First, want.Optimizer.First, 4e-6, 4e-3)
	assertGPUValuesNear(t, "device Adam second moments", got.Optimizer.Second, want.Optimizer.Second, 4e-8, 8e-3)
	if !reflect.DeepEqual(want.Optimizer.Steps, got.Optimizer.Steps) {
		t.Fatalf("device Adam step counts differ: want=%v got=%v", want.Optimizer.Steps, got.Optimizer.Steps)
	}
	if (want.Accumulator == nil) != (got.Accumulator == nil) {
		t.Fatalf("accumulator presence differs: want=%+v got=%+v", want.Accumulator, got.Accumulator)
	}
	if want.Accumulator != nil {
		if want.Accumulator.Count != got.Accumulator.Count {
			t.Fatalf("accumulator counts differ: want=%d got=%d", want.Accumulator.Count, got.Accumulator.Count)
		}
		assertGPUValuesNear(t, "gradient accumulator", got.Accumulator.Sum, want.Accumulator.Sum, 7e-5, 7e-4)
	}
}

func sameFloat64Vector(got, want []float64, tolerance float64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if math.Abs(got[i]-want[i]) > tolerance {
			return false
		}
	}
	return true
}

func cloneTrainingSnapshotForGPUDeviceTest(t *testing.T, snapshot TrainingSnapshot) TrainingSnapshot {
	t.Helper()
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var clone TrainingSnapshot
	if err := json.Unmarshal(encoded, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func nextNonFloat32(value float64) float64 {
	next := math.Nextafter(value, math.Inf(1))
	if float64(float32(next)) == next {
		return math.Nextafter(next, math.Inf(1))
	}
	return next
}

func requireGPUDeviceRestoreRejected(t *testing.T, name string, snapshot TrainingSnapshot, keywords ...string) {
	t.Helper()
	trainer, err := RestoreGPUDeviceTrainer(context.Background(), snapshot, 1<<20)
	if trainer != nil {
		_ = trainer.Close()
		t.Fatalf("%s: RestoreGPUDeviceTrainer accepted invalid state", name)
	}
	if err == nil {
		t.Fatalf("%s: RestoreGPUDeviceTrainer returned nil error", name)
	}
	if errors.Is(err, webgpu.ErrUnavailable) {
		t.Fatalf("%s: invalid state reached hardware setup: %v", name, err)
	}
	if errors.Is(err, ErrGPUTrainerUnsupported) {
		t.Fatalf("%s: corruption was reported as unsupported configuration: %v", name, err)
	}
	lower := strings.ToLower(err.Error())
	for _, keyword := range keywords {
		if !strings.Contains(lower, strings.ToLower(keyword)) {
			t.Fatalf("%s: error %q does not identify %q", name, err, keyword)
		}
	}
}
