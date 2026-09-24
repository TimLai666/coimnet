package webgpu

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

func TestNewSparseDriveRejectsInvalidTopologyBeforeDeviceOpen(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{name: "zero nodes", cfg: Config{}},
		{name: "negative nodes", cfg: Config{Nodes: -1}},
		{name: "different edge shapes", cfg: Config{Nodes: 2, Sources: []int{0}, Targets: nil}},
		{name: "negative source", cfg: Config{Nodes: 2, Sources: []int{-1}, Targets: []int{0}}},
		{name: "source out of range", cfg: Config{Nodes: 2, Sources: []int{2}, Targets: []int{0}}},
		{name: "target out of range", cfg: Config{Nodes: 2, Sources: []int{0}, Targets: []int{2}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := NewSparseDrive(ctx, tc.cfg)
			if err == nil {
				t.Fatal("NewSparseDrive unexpectedly accepted invalid topology")
			}
			if errors.Is(err, ErrUnavailable) {
				t.Fatalf("invalid topology was checked after device open: %v", err)
			}
		})
	}
}

func TestNewSparseDriveRejectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewSparseDrive(ctx, Config{Nodes: 1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("NewSparseDrive error = %v, want context.Canceled", err)
	}
}

func TestSparseDriveGPUMatchesFloat32Reference(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The three-neuron chapter-9 fixture, with an additional repeated and zero
	// edge to prove that CSR construction does not deduplicate or reorder edges.
	drive, err := NewSparseDrive(ctx, Config{
		Nodes:   3,
		Sources: []int{0, 2, 1, 1, 1},
		Targets: []int{1, 1, 2, 1, 2},
	})
	if err != nil {
		if errors.Is(err, ErrUnavailable) {
			t.Skipf("GPU unavailable: %v", err)
		}
		t.Fatalf("NewSparseDrive: %v", err)
	}
	defer func() {
		if err := drive.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	base := []float32{0, 0, 0}
	sourceOutput := []float32{2, 4, 8}
	weights := []float32{0.5, -0.25, 0.75, 0, 0}
	want := cpuSparseDrive(base, sourceOutput, weights, []int{0, 2, 1, 1, 1}, []int{1, 1, 2, 1, 2})

	result, err := drive.Execute(ctx, base, sourceOutput, weights)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Output) != len(want) {
		t.Fatalf("output length = %d, want %d", len(result.Output), len(want))
	}
	for i := range want {
		if !closeFloat32(result.Output[i], want[i]) {
			t.Errorf("output[%d] = %v, want %v", i, result.Output[i], want[i])
		}
	}
	if result.Metrics.Adapter == "" || result.Metrics.Backend == "" {
		t.Fatalf("device identity missing from metrics: %+v", result.Metrics)
	}
	if result.Metrics.Initialization <= 0 || result.Metrics.Compilation <= 0 || result.Metrics.TopologyTransfer <= 0 || result.Metrics.Warmup <= 0 || result.Metrics.Transfer <= 0 || result.Metrics.Dispatch <= 0 || result.Metrics.Readback <= 0 {
		t.Fatalf("phase timings are not measured: %+v", result.Metrics)
	}
	if result.Metrics.BytesUploaded == 0 {
		t.Fatal("Execute reported no uploaded bytes")
	}
	if got := drive.Info(); got.Adapter != result.Metrics.Adapter || got.Backend != result.Metrics.Backend {
		t.Fatalf("Info identity %+v differs from result metrics %+v", got, result.Metrics)
	}
	t.Logf("adapter=%q backend=%q vendor=%q driver=%q device_type=%q initialization=%s compilation=%s topology_transfer=%s warmup=%s transfer=%s dispatch=%s readback=%s cpu_reference_max_abs_diff=%g", result.Metrics.Adapter, result.Metrics.Backend, result.Metrics.Vendor, result.Metrics.Driver, result.Metrics.DeviceType, result.Metrics.Initialization, result.Metrics.Compilation, result.Metrics.TopologyTransfer, result.Metrics.Warmup, result.Metrics.Transfer, result.Metrics.Dispatch, result.Metrics.Readback, maxAbsDiff(result.Output, want))

	secondInput := []float32{1, -2, 0.25}
	secondOutput := []float32{3, 5, -1}
	secondWeights := []float32{0.5, -0.25, 0.75, 0, 0}
	second, err := drive.Execute(ctx, secondInput, secondOutput, secondWeights)
	if err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	secondWant := cpuSparseDrive(secondInput, secondOutput, secondWeights, []int{0, 2, 1, 1, 1}, []int{1, 1, 2, 1, 2})
	for i := range secondWant {
		if !closeFloat32(second.Output[i], secondWant[i]) {
			t.Errorf("second output[%d] = %v, want %v", i, second.Output[i], secondWant[i])
		}
	}
	if second.Metrics.TopologyTransfer != result.Metrics.TopologyTransfer {
		t.Fatalf("topology transfer changed between executions: first=%s second=%s", result.Metrics.TopologyTransfer, second.Metrics.TopologyTransfer)
	}

	// The third edge for target 1 is deliberately after the large cancellation.
	// With the declared order, (1e20 + -1e20) + 1 is 1 in f32. Reordering that
	// duplicate target's edges would round the small term away and produce 0.
	ordered, err := drive.Execute(ctx,
		[]float32{0, 0, 0},
		[]float32{1e20, 4, 1e20},
		[]float32{1, -1, 0.75, 0.25, 0},
	)
	if err != nil {
		t.Fatalf("order-sensitive Execute: %v", err)
	}
	orderedWant := cpuSparseDrive([]float32{0, 0, 0}, []float32{1e20, 4, 1e20}, []float32{1, -1, 0.75, 0.25, 0}, []int{0, 2, 1, 1, 1}, []int{1, 1, 2, 1, 2})
	for i := range orderedWant {
		if !closeFloat32(ordered.Output[i], orderedWant[i]) {
			t.Errorf("order-sensitive output[%d] = %v, want %v", i, ordered.Output[i], orderedWant[i])
		}
	}
	if ordered.Metrics.TopologyTransfer != result.Metrics.TopologyTransfer {
		t.Fatalf("topology transfer changed during order-sensitive execution: first=%s ordered=%s", result.Metrics.TopologyTransfer, ordered.Metrics.TopologyTransfer)
	}
}

func TestSparseDriveCrossesWorkgroupBoundaries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const nodes = 130 // More than two 64-invocation workgroups.
	sources := make([]int, 0, nodes*7)
	targets := make([]int, 0, nodes*7)
	weights := make([]float32, 0, nodes*7)
	for connection := 0; connection < 7; connection++ {
		for target := 0; target < nodes; target++ {
			sources = append(sources, (target*13+connection*17)%nodes)
			targets = append(targets, target)
			weights = append(weights, float32(connection-3)/8)
		}
	}
	base := make([]float32, nodes)
	output := make([]float32, nodes)
	for i := range base {
		base[i] = float32(i%11-5) / 4
		output[i] = float32(i%17-8) / 8
	}
	drive, err := NewSparseDrive(ctx, Config{Nodes: nodes, Sources: sources, Targets: targets})
	if err != nil {
		if errors.Is(err, ErrUnavailable) {
			t.Skipf("GPU unavailable: %v", err)
		}
		t.Fatalf("NewSparseDrive: %v", err)
	}
	defer func() {
		if err := drive.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()
	got, err := drive.Execute(ctx, base, output, weights)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := cpuSparseDrive(base, output, weights, sources, targets)
	for i := range want {
		if !closeFloat32(got.Output[i], want[i]) {
			t.Errorf("output[%d] = %v, want %v", i, got.Output[i], want[i])
		}
	}
	t.Logf("adapter=%q backend=%q nodes=%d edges=%d", got.Metrics.Adapter, got.Metrics.Backend, nodes, len(sources))
}

func TestSparseDriveRejectsInvalidExecuteInputs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	drive, err := NewSparseDrive(ctx, Config{
		Nodes:   2,
		Sources: []int{0},
		Targets: []int{1},
	})
	if err != nil {
		if errors.Is(err, ErrUnavailable) {
			t.Skipf("GPU unavailable: %v", err)
		}
		t.Fatalf("NewSparseDrive: %v", err)
	}
	defer drive.Close()

	cases := []struct {
		name    string
		input   []float32
		output  []float32
		weights []float32
		wantErr error
	}{
		{name: "input shape", input: []float32{0}, output: []float32{0, 0}, weights: []float32{1}, wantErr: ErrShape},
		{name: "source shape", input: []float32{0, 0}, output: []float32{0}, weights: []float32{1}, wantErr: ErrShape},
		{name: "weights shape", input: []float32{0, 0}, output: []float32{0, 0}, weights: nil, wantErr: ErrShape},
		{name: "non-finite input", input: []float32{float32(math.NaN()), 0}, output: []float32{0, 0}, weights: []float32{1}, wantErr: ErrNonFinite},
		{name: "non-finite output", input: []float32{0, 0}, output: []float32{0, float32(math.Inf(1))}, weights: []float32{1}, wantErr: ErrNonFinite},
		{name: "non-finite weight", input: []float32{0, 0}, output: []float32{0, 0}, weights: []float32{float32(math.Inf(-1))}, wantErr: ErrNonFinite},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := drive.Execute(ctx, tc.input, tc.output, tc.weights)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Execute error = %v, want errors.Is(..., %v)", err, tc.wantErr)
			}
		})
	}
}

func TestSparseDriveExecuteHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	drive, err := NewSparseDrive(ctx, Config{Nodes: 1})
	if err != nil {
		if errors.Is(err, ErrUnavailable) {
			t.Skipf("GPU unavailable: %v", err)
		}
		t.Fatalf("NewSparseDrive: %v", err)
	}
	closed := false
	defer func() {
		if !closed {
			if err := drive.Close(); err != nil {
				t.Errorf("deferred Close: %v", err)
			}
		}
	}()

	result, err := drive.Execute(ctx, []float32{2.5}, []float32{3}, nil)
	if err != nil {
		t.Fatalf("zero-edge Execute: %v", err)
	}
	if len(result.Output) != 1 || !closeFloat32(result.Output[0], 2.5) {
		t.Fatalf("zero-edge output = %v, want [2.5]", result.Output)
	}

	canceled, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	_, err = drive.Execute(canceled, []float32{0}, []float32{0}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute error = %v, want context.Canceled", err)
	}

	if err := drive.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	closed = true
	if err := drive.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	_, err = drive.Execute(ctx, []float32{0}, []float32{0}, nil)
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("Execute after Close error = %v, want ErrClosed", err)
	}
}

func closeFloat32(got, want float32) bool {
	diff := float64(got - want)
	if diff < 0 {
		diff = -diff
	}
	limit := 1e-5 + 1e-4*math.Abs(float64(want))
	return diff <= limit
}

func cpuSparseDrive(input, sourceOutput, weights []float32, sources, targets []int) []float32 {
	result := append([]float32(nil), input...)
	for edge, source := range sources {
		target := targets[edge]
		result[target] = result[target] + weights[edge]*sourceOutput[source]
	}
	return result
}

func maxAbsDiff(got, want []float32) float64 {
	if len(got) != len(want) {
		return math.Inf(1)
	}
	var max float64
	for i := range got {
		diff := math.Abs(float64(got[i] - want[i]))
		if diff > max {
			max = diff
		}
	}
	return max
}
