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
	"testing"
	"time"

	"github.com/TimLai666/coimnet/backend/webgpu"
	"github.com/TimLai666/coimnet/dynamics"
)

func TestGPUTrainerMatchesCPUUpdatesAndMoments(t *testing.T) {
	ctx := context.Background()
	c, p, o := gpuTrainerFixture()
	cpu, err := NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	gpu := newGPUTrainerOrSkip(t, ctx, c, p, o)
	defer func() {
		if err := gpu.Close(); err != nil {
			t.Errorf("Close() error: %v", err)
		}
	}()

	info, ok := gpu.GPUAdapterInfo()
	if !ok || info.Forward.Adapter == "" || info.Forward.Backend == "" || info.Backward.Adapter == "" || info.Backward.Backend == "" {
		t.Fatalf("GPUAdapterInfo() = %+v, %v; want forward and backward device identities", info, ok)
	}
	t.Logf("GPU adapters: forward=%+v backward=%+v", info.Forward, info.Backward)
	input := gpuTrainerInput()
	wantPrediction, err := cpu.Predict(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	gotPrediction, err := gpu.Predict(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	assertGPUValuesNear(t, "prediction", gotPrediction, wantPrediction, 4e-5, 4e-5)

	for step := range 4 {
		want, err := cpu.Step(ctx, input, []float64{0.2 - 0.1*float64(step)})
		if err != nil {
			t.Fatalf("CPU Step %d: %v", step, err)
		}
		got, err := gpu.Step(ctx, input, []float64{0.2 - 0.1*float64(step)})
		if err != nil {
			t.Fatalf("GPU Step %d: %v", step, err)
		}
		if !want.Applied || !got.Applied || want.Updates != got.Updates {
			t.Fatalf("CPU/GPU step %d status differs: CPU=%+v GPU=%+v", step, want, got)
		}
		assertGPUTrainerStateNear(t, cpu.Snapshot(), gpu.Snapshot())
	}
	if reflect.DeepEqual(p.Core.Weights, gpu.Snapshot().Parameters.Core.Weights) {
		t.Fatal("GPU training did not update core weights")
	}
}

func TestGPUTrainerStepFromMatchesCPU(t *testing.T) {
	ctx := context.Background()
	c, p, o := gpuTrainerFixture()
	cpu, err := NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	gpu := newGPUTrainerOrSkip(t, ctx, c, p, o)
	defer gpu.Close()

	upstream := [][]float64{{0}, {0}, {0}, {0.65}}
	want, err := cpu.StepFrom(ctx, gpuTrainerInput(), upstream)
	if err != nil {
		t.Fatalf("CPU StepFrom: %v", err)
	}
	got, err := gpu.StepFrom(ctx, gpuTrainerInput(), upstream)
	if err != nil {
		t.Fatalf("GPU StepFrom: %v", err)
	}
	if !want.Applied || !got.Applied || want.LossKnown || got.LossKnown {
		t.Fatalf("unexpected StepFrom result: CPU=%+v GPU=%+v", want, got)
	}
	assertGPUTrainerStateNear(t, cpu.Snapshot(), gpu.Snapshot())
}

func TestGPUTrainerRestoreContinuesWithoutInterruption(t *testing.T) {
	ctx := context.Background()
	c, p, o := gpuTrainerFixture()
	continuous := newGPUTrainerOrSkip(t, ctx, c, p, o)
	defer continuous.Close()
	for step := range 2 {
		if _, err := continuous.Step(ctx, gpuTrainerInput(), []float64{0.15 + float64(step)*0.1}); err != nil {
			t.Fatalf("uninterrupted prefix step %d: %v", step, err)
		}
	}

	snapshot := continuous.Snapshot()
	resumed, err := RestoreGPUTrainer(ctx, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	for step := range 3 {
		target := []float64{-0.1 + float64(step)*0.2}
		if _, err := continuous.Step(ctx, gpuTrainerInput(), target); err != nil {
			t.Fatalf("uninterrupted suffix step %d: %v", step, err)
		}
		if _, err := resumed.Step(ctx, gpuTrainerInput(), target); err != nil {
			t.Fatalf("restored suffix step %d: %v", step, err)
		}
		if got, want := resumed.Snapshot(), continuous.Snapshot(); !reflect.DeepEqual(got, want) {
			t.Fatalf("restored and uninterrupted GPU state differ at suffix step %d", step)
		}
	}
}

func TestGPUTrainerPersistedSnapshotResumesInNewProcess(t *testing.T) {
	ctx := context.Background()
	c, p, o := gpuTrainerFixture()
	uninterrupted := newGPUTrainerOrSkip(t, ctx, c, p, o)
	defer uninterrupted.Close()
	for step := range 2 {
		if _, err := uninterrupted.Step(ctx, gpuTrainerInput(), []float64{0.15 + float64(step)*0.1}); err != nil {
			t.Fatalf("prefix step %d: %v", step, err)
		}
	}
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "snapshot.json")
	outputPath := filepath.Join(directory, "resumed.json")
	encoded, err := json.Marshal(uninterrupted.Snapshot())
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
	childContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	command := exec.CommandContext(childContext, executable, "-test.run=^TestGPUTrainerSubprocessRestoreHelper$")
	command.Env = append(os.Environ(), "COIMNET_GPU_RESTORE_INPUT="+inputPath, "COIMNET_GPU_RESTORE_OUTPUT="+outputPath)
	childOutput, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("new-process GPU restore: %v: %s", err, childOutput)
	}
	for step := range 3 {
		if _, err := uninterrupted.Step(ctx, gpuTrainerInput(), []float64{-0.1 + float64(step)*0.2}); err != nil {
			t.Fatalf("uninterrupted suffix step %d: %v", step, err)
		}
	}
	resumedJSON, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var resumed TrainingSnapshot
	if err := json.Unmarshal(resumedJSON, &resumed); err != nil {
		t.Fatal(err)
	}
	if want := uninterrupted.Snapshot(); !reflect.DeepEqual(resumed, want) {
		t.Fatalf("new-process persisted GPU resume differs from uninterrupted training")
	}
}

func TestGPUTrainerSubprocessRestoreHelper(t *testing.T) {
	inputPath := os.Getenv("COIMNET_GPU_RESTORE_INPUT")
	if inputPath == "" {
		return
	}
	outputPath := os.Getenv("COIMNET_GPU_RESTORE_OUTPUT")
	if outputPath == "" {
		t.Fatal("missing child output path")
	}
	encoded, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot TrainingSnapshot
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		t.Fatal(err)
	}
	trainer, err := RestoreGPUTrainer(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer trainer.Close()
	for step := range 3 {
		if _, err := trainer.Step(context.Background(), gpuTrainerInput(), []float64{-0.1 + float64(step)*0.2}); err != nil {
			t.Fatalf("resumed suffix step %d: %v", step, err)
		}
	}
	result, err := json.Marshal(trainer.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outputPath, result, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestNewGPUTrainerRejectsUnsupportedConfigurations(t *testing.T) {
	c, p, o := gpuTrainerFixture()
	cases := []struct {
		name string
		edit func(*Config, *Options)
	}{
		{name: "LIF", edit: func(c *Config, _ *Options) { c.LIF = &dynamics.LIFConfig{} }},
		{name: "mixed", edit: func(c *Config, _ *Options) { c.Mixed = &dynamics.MixedConfig{} }},
		{name: "positive delay", edit: func(c *Config, _ *Options) { c.Dynamics.Delays = []int{0, 1, 0, 0} }},
		{name: "vector state", edit: func(c *Config, _ *Options) { c.Dynamics.StateDimension = 2 }},
		{name: "matrix edge", edit: func(c *Config, _ *Options) { c.Dynamics.EdgeShape = "matrix" }},
		{name: "recompute", edit: func(_ *Config, o *Options) { o.Recompute = &Recompute{SegmentSteps: 1} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			configured, options := c, o
			tc.edit(&configured, &options)
			trainer, err := NewGPUTrainer(context.Background(), configured, p, options)
			if trainer != nil {
				_ = trainer.Close()
				t.Fatal("NewGPUTrainer returned a trainer for an unsupported configuration")
			}
			if !errors.Is(err, ErrGPUTrainerUnsupported) {
				t.Fatalf("NewGPUTrainer() error = %v, want ErrGPUTrainerUnsupported", err)
			}
		})
	}
}

func TestGPUTrainerCloseRefusesFurtherExecutionAndCPUCloseIsNoop(t *testing.T) {
	ctx := context.Background()
	c, p, o := gpuTrainerFixture()
	model, err := NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Close(); err != nil {
		t.Fatalf("CPU Close(): %v", err)
	}
	if _, err := model.Predict(ctx, gpuTrainerInput()); err != nil {
		t.Fatalf("CPU Predict after no-op Close(): %v", err)
	}

	gpu := newGPUTrainerOrSkip(t, ctx, c, p, o)
	before := gpu.Snapshot()
	if err := gpu.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gpu.Close(); err != nil {
		t.Fatalf("second Close(): %v", err)
	}
	if _, err := gpu.Predict(ctx, gpuTrainerInput()); !errors.Is(err, dynamics.ErrGPUContinuousClosed) {
		t.Fatalf("Predict after Close() error = %v, want closed GPU error", err)
	}
	if _, err := gpu.Step(ctx, gpuTrainerInput(), []float64{0.2}); !errors.Is(err, dynamics.ErrGPUContinuousClosed) {
		t.Fatalf("Step after Close() error = %v, want closed GPU error", err)
	}
	if _, err := gpu.StepFrom(ctx, gpuTrainerInput(), [][]float64{{0}, {0}, {0}, {0.5}}); !errors.Is(err, dynamics.ErrGPUContinuousClosed) {
		t.Fatalf("StepFrom after Close() error = %v, want closed GPU error", err)
	}
	if got := gpu.Snapshot(); !reflect.DeepEqual(got, before) {
		t.Fatal("execution after Close() changed the trainer state")
	}
}

func TestGPUContinuousCoreRejectsPersistentStateOperations(t *testing.T) {
	ctx := context.Background()
	c, p, o := gpuTrainerFixture()
	trainer := newGPUTrainerOrSkip(t, ctx, c, p, o)
	defer trainer.Close()
	core, ok := trainer.network.core.(gpuContinuousCore)
	if !ok {
		t.Fatalf("network core type is %T, want gpuContinuousCore", trainer.network.core)
	}
	state, err := core.newState(make([]float64, c.Dynamics.Nodes))
	if !errors.Is(err, ErrGPUTrainerStateUnsupported) {
		t.Fatalf("newState() error = %v, want ErrGPUTrainerStateUnsupported", err)
	}
	if err := core.validateState(state); !errors.Is(err, ErrGPUTrainerStateUnsupported) {
		t.Fatalf("validateState() error = %v, want ErrGPUTrainerStateUnsupported", err)
	}
	if _, _, _, err := core.advance(ctx, p, state, gpuTrainerInput()); !errors.Is(err, ErrGPUTrainerStateUnsupported) {
		t.Fatalf("advance() error = %v, want ErrGPUTrainerStateUnsupported", err)
	}
	if _, _, _, err := core.advanceModulated(ctx, p, state, gpuTrainerInput(), nil); !errors.Is(err, ErrGPUTrainerStateUnsupported) {
		t.Fatalf("advanceModulated() error = %v, want ErrGPUTrainerStateUnsupported", err)
	}
}

func gpuTrainerFixture() (Config, Parameters, Options) {
	config := Config{
		Dynamics: dynamics.Config{
			Nodes: 4, Sources: []int{0, 1, 2, 3, 0}, Targets: []int{1, 2, 3, 0, 2},
			DT: 0.4, Activation: "tanh",
		},
		InputSize: 2, OutputSize: 1, ReadoutNodes: []int{2, 3},
	}
	parameters := Parameters{
		Core: dynamics.Parameters{
			Weights: []float64{0.35, -0.2, 0.4, 0.15, -0.1},
			Bias:    []float64{0.03, -0.02, 0.01, 0.04},
			LogTau:  []float64{0.1, -0.05, 0.02, 0.08},
		},
		Encoder: []float64{0.4, -0.15, 0.25, -0.3, 0.2, 0.1, 0.05, -0.25},
		Readout: []float64{0.7, -0.45},
	}
	options := DefaultOptions()
	options.LearningRate = 0.002
	options.WeightDecay = 0.01
	options.ClipNorm = 0
	return config, parameters, options
}

func newGPUTrainerOrSkip(t *testing.T, ctx context.Context, c Config, p Parameters, o Options) *Trainer {
	t.Helper()
	trainer, err := NewGPUTrainer(ctx, c, p, o)
	if errors.Is(err, webgpu.ErrUnavailable) {
		t.Skipf("GPU unavailable: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	return trainer
}

func gpuTrainerInput() [][]float64 {
	return [][]float64{{0.7, -0.2}, {-0.3, 0.5}, {0.2, 0.4}, {-0.1, -0.6}}
}

func assertGPUTrainerStateNear(t *testing.T, want, got TrainingSnapshot) {
	t.Helper()
	if !reflect.DeepEqual(want.Config, got.Config) || !reflect.DeepEqual(want.Options, got.Options) || want.Updates != got.Updates {
		t.Fatalf("GPU trainer metadata differs from CPU: want config/options/updates=%+v/%+v/%d got=%+v/%+v/%d", want.Config, want.Options, want.Updates, got.Config, got.Options, got.Updates)
	}
	assertGPUValuesNear(t, "parameters", flatParameters(got.Parameters), flatParameters(want.Parameters), 5e-5, 5e-4)
	assertGPUValuesNear(t, "Adam first moments", got.Optimizer.First, want.Optimizer.First, 2e-6, 2e-3)
	assertGPUValuesNear(t, "Adam second moments", got.Optimizer.Second, want.Optimizer.Second, 2e-8, 5e-3)
	if !reflect.DeepEqual(want.Optimizer.Steps, got.Optimizer.Steps) {
		t.Fatal("GPU trainer optimizer step counts differ from CPU")
	}
}

func assertGPUValuesNear(t *testing.T, name string, got, want []float64, atol, rtol float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s length = %d, want %d", name, len(got), len(want))
	}
	for i := range got {
		if math.Abs(got[i]-want[i]) > atol+rtol*math.Abs(want[i]) {
			t.Fatalf("%s[%d] = %.12g, want %.12g (atol=%g rtol=%g)", name, i, got[i], want[i], atol, rtol)
		}
	}
}
