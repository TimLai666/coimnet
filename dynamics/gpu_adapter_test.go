package dynamics

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/TimLai666/coimnet/backend/webgpu"
)

func TestNewGPUContinuousRejectsUnsupportedConfigurations(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{
			name: "positive delay",
			cfg: Config{
				Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{1},
				DT: 0.1, Activation: "tanh",
			},
		},
		{
			name: "vector state",
			cfg: Config{
				Nodes: 2, Sources: []int{0}, Targets: []int{1},
				DT: 0.1, Activation: "tanh", StateDimension: 2,
			},
		},
		{
			name: "matrix edge shape",
			cfg: Config{
				Nodes: 2, Sources: []int{0}, Targets: []int{1},
				DT: 0.1, Activation: "tanh", EdgeShape: "matrix",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gpu, err := NewGPUContinuousForward(context.Background(), tc.cfg)
			if gpu != nil {
				_ = gpu.Close()
			}
			if !errors.Is(err, ErrGPUContinuousUnsupported) {
				t.Fatalf("NewGPUContinuousForward() error = %v, want ErrGPUContinuousUnsupported", err)
			}
		})
	}
}

func TestGPUContinuousForwardMatchesCPUWithDuplicatesAndWorkgroups(t *testing.T) {
	cfg, p, initial, inputs := gpuTestFixture(130, 3)
	cpu, err := NewContinuous(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cpuTrace, err := cpu.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}

	gpu := newGPUContinuousForwardOrSkip(t, cfg)
	defer gpu.Close()
	gpuTrace, err := gpu.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(gpuTrace.output) != len(cpuTrace.output) || len(gpuTrace.voltage) != len(cpuTrace.voltage) {
		t.Fatalf("trace lengths differ: gpu output=%d voltage=%d, cpu output=%d voltage=%d", len(gpuTrace.output), len(gpuTrace.voltage), len(cpuTrace.output), len(cpuTrace.voltage))
	}
	for tstep := range cpuTrace.output {
		assertFloat64RowsNear(t, "output", tstep, gpuTrace.output[tstep], cpuTrace.output[tstep], 2e-5, 2e-5)
		assertFloat64RowsNear(t, "voltage", tstep, gpuTrace.voltage[tstep], cpuTrace.voltage[tstep], 2e-5, 2e-5)
	}
	for tstep := range cpuTrace.drive {
		assertFloat64RowsNear(t, "drive", tstep, gpuTrace.drive[tstep], cpuTrace.drive[tstep], 2e-5, 2e-5)
	}
}

func TestGPUContinuousBackwardMatchesCPUWithTruncation(t *testing.T) {
	cfg, p, initial, inputs := gpuTestFixture(130, 4)
	cpu, err := NewContinuous(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cpuTrace, err := cpu.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	upstream := make([][]float64, len(inputs))
	for step := range upstream {
		upstream[step] = make([]float64, cfg.Nodes)
		for node := range upstream[step] {
			upstream[step][node] = float64((step+1)*(node%11-5)) / 17
		}
	}
	want, err := cpu.Backward(context.Background(), cpuTrace, upstream, 2)
	if err != nil {
		t.Fatal(err)
	}

	gpu, err := NewGPUContinuous(context.Background(), cfg)
	if err != nil {
		if errors.Is(err, ErrGPUContinuousBackwardUnavailable) {
			t.Skipf("GPU backward primitive unavailable: %v", err)
		}
		if errors.Is(err, webgpu.ErrUnavailable) || errors.Is(err, webgpu.ErrDevice) {
			t.Skipf("GPU unavailable: %v", err)
		}
		t.Fatalf("NewGPUContinuous() error = %v", err)
	}
	defer gpu.Close()
	gpuTrace, err := gpu.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	got, err := gpu.Backward(context.Background(), gpuTrace, upstream, 2)
	if err != nil {
		t.Fatal(err)
	}
	assertFloat64RowsNear(t, "weights", 0, got.Weights, want.Weights, 4e-5, 4e-5)
	assertFloat64RowsNear(t, "bias", 0, got.Bias, want.Bias, 4e-5, 4e-5)
	assertFloat64RowsNear(t, "log tau", 0, got.LogTau, want.LogTau, 4e-5, 4e-5)
	assertFloat64RowsNear(t, "initial", 0, got.Initial, want.Initial, 4e-5, 4e-5)
	if len(got.Inputs) != len(want.Inputs) {
		t.Fatalf("input gradient rows: got %d want %d", len(got.Inputs), len(want.Inputs))
	}
	for step := range want.Inputs {
		assertFloat64RowsNear(t, "input gradient", step, got.Inputs[step], want.Inputs[step], 4e-5, 4e-5)
	}
}

func TestGPUFloat32ConversionRejectsNonzeroUnderflow(t *testing.T) {
	for _, value := range []float64{math.SmallestNonzeroFloat64, -math.SmallestNonzeroFloat64} {
		_, err := gpuFloat32Values([]float64{value}, "gradient")
		if !errors.Is(err, ErrGPUContinuousPrecision) {
			t.Fatalf("gpuFloat32Values(%g) error = %v, want ErrGPUContinuousPrecision", value, err)
		}
	}
}

func TestGPUContinuousRejectsValuesThatCannotBeRepresentedAsFloat32(t *testing.T) {
	cfg := Config{Nodes: 1, DT: 0.1, Activation: "tanh"}
	gpu := newGPUContinuousForwardOrSkip(t, cfg)
	defer gpu.Close()
	p := Parameters{Bias: []float64{0}, LogTau: []float64{0}}
	_, err := gpu.Forward(context.Background(), p, []float64{0}, [][]float64{{math.MaxFloat64}})
	if !errors.Is(err, ErrGPUContinuousPrecision) {
		t.Fatalf("Forward() error = %v, want ErrGPUContinuousPrecision", err)
	}
}

func TestGPUContinuousForwardOnlyDoesNotFallbackInBackward(t *testing.T) {
	cfg := Config{Nodes: 1, DT: 0.1, Activation: "tanh"}
	gpu := newGPUContinuousForwardOrSkip(t, cfg)
	defer gpu.Close()
	p := Parameters{Bias: []float64{0}, LogTau: []float64{0}}
	tr, err := gpu.Forward(context.Background(), p, []float64{0.25}, [][]float64{{0.5}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = gpu.Backward(context.Background(), tr, [][]float64{{1}}, 0)
	if !errors.Is(err, ErrGPUContinuousBackwardUnavailable) {
		t.Fatalf("Backward() error = %v, want ErrGPUContinuousBackwardUnavailable", err)
	}
}

func gpuTestFixture(nodes, steps int) (Config, Parameters, []float64, [][]float64) {
	sources := make([]int, 0, nodes*5)
	targets := make([]int, 0, nodes*5)
	for target := 0; target < nodes; target++ {
		for connection := 0; connection < 4; connection++ {
			sources = append(sources, (target*17+connection*29)%nodes)
			targets = append(targets, target)
		}
	}
	// Keep two declared duplicate edges. The adapter must preserve both terms
	// and their declaration order in the f32 sparse operation.
	sources = append(sources, 0, 0)
	targets = append(targets, 0, 0)
	cfg := Config{
		Nodes:      nodes,
		Sources:    sources,
		Targets:    targets,
		DT:         0.2,
		Activation: "tanh",
	}
	p := Parameters{
		Weights: make([]float64, len(sources)),
		Bias:    make([]float64, nodes),
		LogTau:  make([]float64, nodes),
	}
	for edge := range p.Weights {
		p.Weights[edge] = float64((edge%13)-6) / 29
	}
	// Make the duplicate terms different so a dropped or coalesced edge is
	// observable without relying on a large unstable cancellation.
	p.Weights[len(p.Weights)-2] = 0.31
	p.Weights[len(p.Weights)-1] = -0.07
	for node := range p.Bias {
		p.Bias[node] = float64(node%9-4) / 23
		p.LogTau[node] = float64(node%7-3) / 8
	}
	initial := make([]float64, nodes)
	for node := range initial {
		initial[node] = float64(node%17-8) / 19
	}
	inputs := make([][]float64, steps)
	for step := range inputs {
		inputs[step] = make([]float64, nodes)
		for node := range inputs[step] {
			inputs[step][node] = float64((step+2)*(node%13-6)) / 31
		}
	}
	return cfg, p, initial, inputs
}

func newGPUContinuousForwardOrSkip(t *testing.T, cfg Config) *GPUContinuous {
	t.Helper()
	gpu, err := NewGPUContinuousForward(context.Background(), cfg)
	if err != nil {
		if errors.Is(err, webgpu.ErrUnavailable) || errors.Is(err, webgpu.ErrDevice) {
			t.Skipf("GPU unavailable: %v", err)
		}
		t.Fatalf("NewGPUContinuousForward() error = %v", err)
	}
	return gpu
}

func assertFloat64RowsNear(t *testing.T, name string, row int, got, want []float64, absTol, relTol float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s[%d] length: got %d want %d", name, row, len(got), len(want))
	}
	for i := range want {
		delta := math.Abs(got[i] - want[i])
		limit := absTol + relTol*math.Abs(want[i])
		if delta > limit {
			t.Fatalf("%s[%d][%d]: got %.9g want %.9g delta %.9g limit %.9g", name, row, i, got[i], want[i], delta, limit)
		}
	}
}
