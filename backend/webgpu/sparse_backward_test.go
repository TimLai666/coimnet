package webgpu

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

func TestSparseBackwardGPUMatchesFloat32Reference(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := Config{
		Nodes:   4,
		Sources: []int{0, 0, 1, 1, 1, 2, 0},
		Targets: []int{1, 1, 0, 1, 2, 0, 0},
	}
	backward, err := NewSparseBackward(ctx, cfg)
	if err != nil {
		if errors.Is(err, ErrUnavailable) {
			t.Skipf("GPU unavailable: %v", err)
		}
		t.Fatalf("NewSparseBackward: %v", err)
	}
	defer func() {
		if err := backward.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()
	upstream := []float32{0.5, -1, 2, 4}
	sourceOutput := []float32{3, 5, -2, 0.25}
	weights := []float32{0.25, -0.5, 1.5, 2, -1, 0.5, 0}
	want := cpuSparseBackward(upstream, sourceOutput, weights, cfg.Sources, cfg.Targets)

	got, err := backward.Backward(ctx, upstream, sourceOutput, weights)
	if err != nil {
		t.Fatalf("Backward: %v", err)
	}
	for name, pair := range map[string]struct{ got, want []float32 }{
		"input":  {got.Input, want.Input},
		"source": {got.Source, want.Source},
		"weight": {got.Weights, want.Weights},
	} {
		if len(pair.got) != len(pair.want) {
			t.Fatalf("%s gradient length = %d, want %d", name, len(pair.got), len(pair.want))
		}
		for i := range pair.want {
			if diff := math.Abs(float64(pair.got[i] - pair.want[i])); diff > 1e-6 {
				t.Errorf("%s gradient[%d] = %.9g, want %.9g", name, i, pair.got[i], pair.want[i])
			}
		}
	}
	if got.Metrics.Adapter == "" || got.Metrics.Backend == "" {
		t.Fatalf("device identity missing from metrics: %+v", got.Metrics)
	}
	if got.Metrics.Initialization <= 0 || got.Metrics.Compilation <= 0 || got.Metrics.TopologyTransfer <= 0 || got.Metrics.Warmup <= 0 || got.Metrics.Transfer <= 0 || got.Metrics.Dispatch <= 0 || got.Metrics.Readback <= 0 {
		t.Fatalf("phase timings are not measured: %+v", got.Metrics)
	}
	if got.Metrics.BytesUploaded == 0 {
		t.Fatal("Backward reported no uploaded bytes")
	}
	t.Logf("adapter=%q backend=%q vendor=%q driver=%q device_type=%q initialization=%s compilation=%s topology_transfer=%s warmup=%s transfer=%s dispatch=%s readback=%s cpu_reference_max_abs_diff=%g", got.Metrics.Adapter, got.Metrics.Backend, got.Metrics.Vendor, got.Metrics.Driver, got.Metrics.DeviceType, got.Metrics.Initialization, got.Metrics.Compilation, got.Metrics.TopologyTransfer, got.Metrics.Warmup, got.Metrics.Transfer, got.Metrics.Dispatch, got.Metrics.Readback, maxBackwardAbsDiff(got, want))

	// The source-0 edges are declared in the order +1e20, -1e20, +1. A
	// reorder would round the final source gradient to 0 instead of 1.
	orderedUpstream := []float32{4, 1e20, 0, 0}
	orderedSourceOutput := []float32{1, 1, 1, 1}
	orderedWeights := []float32{1, -1, 1.5, 0, 0, 0.5, 0.25}
	orderedWant := cpuSparseBackward(orderedUpstream, orderedSourceOutput, orderedWeights, cfg.Sources, cfg.Targets)
	ordered, err := backward.Backward(ctx, orderedUpstream, orderedSourceOutput, orderedWeights)
	if err != nil {
		t.Fatalf("order-sensitive Backward: %v", err)
	}
	if ordered.Source[0] != 1 {
		t.Fatalf("order-sensitive source gradient = %v, want 1", ordered.Source[0])
	}
	for name, pair := range map[string]struct{ got, want []float32 }{
		"input":  {ordered.Input, orderedWant.Input},
		"source": {ordered.Source, orderedWant.Source},
		"weight": {ordered.Weights, orderedWant.Weights},
	} {
		for i := range pair.want {
			if diff := math.Abs(float64(pair.got[i] - pair.want[i])); diff > 1e-6 {
				t.Errorf("order-sensitive %s gradient[%d] = %.9g, want %.9g", name, i, pair.got[i], pair.want[i])
			}
		}
	}
}

func TestSparseBackwardSingleEdgeRepeatedInputs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := Config{Nodes: 3, Sources: []int{2}, Targets: []int{1}}
	backward, err := NewSparseBackward(ctx, cfg)
	if err != nil {
		if errors.Is(err, ErrUnavailable) {
			t.Skipf("GPU unavailable: %v", err)
		}
		t.Fatalf("NewSparseBackward: %v", err)
	}
	defer func() {
		if err := backward.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	// Reuse one executor while changing all dynamic values. This isolates the
	// constant-index first-edge shader from the intermittent first-slot write
	// and catches stale uploads or outputs across dispatches.
	for iteration := 0; iteration < 12; iteration++ {
		upstream := []float32{0, float32(iteration-5) / 4, 0}
		sourceOutput := []float32{0, 0, float32(iteration+1) / 2}
		weights := []float32{float32(7-iteration) / 8}
		want := cpuSparseBackward(upstream, sourceOutput, weights, cfg.Sources, cfg.Targets)
		got, err := backward.Backward(ctx, upstream, sourceOutput, weights)
		if err != nil {
			t.Fatalf("iteration %d Backward: %v", iteration, err)
		}
		for name, pair := range map[string]struct{ got, want []float32 }{
			"input":  {got.Input, want.Input},
			"source": {got.Source, want.Source},
			"weight": {got.Weights, want.Weights},
		} {
			for i := range pair.want {
				if diff := math.Abs(float64(pair.got[i] - pair.want[i])); diff > 1e-6 {
					t.Errorf("iteration %d %s gradient[%d] = %.9g, want %.9g", iteration, name, i, pair.got[i], pair.want[i])
				}
			}
		}
	}
	t.Logf("adapter=%q backend=%q repeated_inputs=12", backward.Info().Adapter, backward.Info().Backend)
}

func TestSparseBackwardZeroEdges(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := Config{Nodes: 3}
	backward, err := NewSparseBackward(ctx, cfg)
	if err != nil {
		if errors.Is(err, ErrUnavailable) {
			t.Skipf("GPU unavailable: %v", err)
		}
		t.Fatalf("NewSparseBackward: %v", err)
	}
	defer backward.Close()

	upstream := []float32{0.25, -1.5, 2}
	got, err := backward.Backward(ctx, upstream, []float32{3, 4, 5}, nil)
	if err != nil {
		t.Fatalf("Backward: %v", err)
	}
	if len(got.Weights) != 0 {
		t.Fatalf("weight gradient length = %d, want 0", len(got.Weights))
	}
	for i, want := range []float32{0, 0, 0} {
		if got.Source[i] != want {
			t.Errorf("source gradient[%d] = %g, want %g", i, got.Source[i], want)
		}
	}
	for i, want := range upstream {
		if got.Input[i] != want {
			t.Errorf("input gradient[%d] = %g, want %g", i, got.Input[i], want)
		}
	}
}

func TestSparseBackwardCrossesWorkgroupBoundaries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const nodes = 130
	sources := make([]int, 0, nodes*7)
	targets := make([]int, 0, nodes*7)
	weights := make([]float32, 0, nodes*7)
	for source := 0; source < nodes; source++ {
		for connection := 0; connection < 7; connection++ {
			sources = append(sources, source)
			targets = append(targets, (source*13+connection*17)%nodes)
			weights = append(weights, float32(connection-3)/8)
		}
	}
	upstream := make([]float32, nodes)
	sourceOutput := make([]float32, nodes)
	for i := range upstream {
		upstream[i] = float32(i%17-8) / 8
		sourceOutput[i] = float32(i%11-5) / 4
	}

	backward, err := NewSparseBackward(ctx, Config{Nodes: nodes, Sources: sources, Targets: targets})
	if err != nil {
		if errors.Is(err, ErrUnavailable) {
			t.Skipf("GPU unavailable: %v", err)
		}
		t.Fatalf("NewSparseBackward: %v", err)
	}
	defer func() {
		if err := backward.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()
	if err := fillSparseBackwardGradientBuffers(ctx, backward); err != nil {
		t.Fatalf("initialize raw weight gradients with sentinel: %v", err)
	}

	got, err := backward.Backward(ctx, upstream, sourceOutput, weights)
	if err != nil {
		t.Fatalf("Backward: %v", err)
	}
	want := cpuSparseBackward(upstream, sourceOutput, weights, sources, targets)
	for name, pair := range map[string]struct{ got, want []float32 }{
		"input":  {got.Input, want.Input},
		"source": {got.Source, want.Source},
		"weight": {got.Weights, want.Weights},
	} {
		for i := range pair.want {
			if diff := math.Abs(float64(pair.got[i] - pair.want[i])); diff > 1e-6 {
				t.Errorf("%s gradient[%d] = %.9g, want %.9g", name, i, pair.got[i], pair.want[i])
			}
		}
	}
	t.Logf("adapter=%q backend=%q device_type=%q nodes=%d edges=%d initialization=%s compilation=%s topology_transfer=%s warmup=%s transfer=%s dispatch=%s readback=%s max_abs_diff=%g", got.Metrics.Adapter, got.Metrics.Backend, got.Metrics.DeviceType, nodes, len(sources), got.Metrics.Initialization, got.Metrics.Compilation, got.Metrics.TopologyTransfer, got.Metrics.Warmup, got.Metrics.Transfer, got.Metrics.Dispatch, got.Metrics.Readback, maxBackwardAbsDiff(got, want))
}

func TestSparseBackwardRejectsInvalidInputs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	backward, err := NewSparseBackward(ctx, Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}})
	if err != nil {
		if errors.Is(err, ErrUnavailable) {
			t.Skipf("GPU unavailable: %v", err)
		}
		t.Fatalf("NewSparseBackward: %v", err)
	}
	defer backward.Close()
	for _, tc := range []struct {
		name     string
		upstream []float32
		source   []float32
		weights  []float32
		wantErr  error
	}{
		{name: "upstream shape", upstream: []float32{0}, source: []float32{0, 0}, weights: []float32{1}, wantErr: ErrShape},
		{name: "source shape", upstream: []float32{0, 0}, source: []float32{0}, weights: []float32{1}, wantErr: ErrShape},
		{name: "weight shape", upstream: []float32{0, 0}, source: []float32{0, 0}, wantErr: ErrShape},
		{name: "non-finite upstream", upstream: []float32{float32(math.NaN()), 0}, source: []float32{0, 0}, weights: []float32{1}, wantErr: ErrNonFinite},
		{name: "non-finite source", upstream: []float32{0, 0}, source: []float32{0, float32(math.Inf(1))}, weights: []float32{1}, wantErr: ErrNonFinite},
		{name: "non-finite weight", upstream: []float32{0, 0}, source: []float32{0, 0}, weights: []float32{float32(math.Inf(-1))}, wantErr: ErrNonFinite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := backward.Backward(ctx, tc.upstream, tc.source, tc.weights)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Backward error = %v, want errors.Is(..., %v)", err, tc.wantErr)
			}
		})
	}
}

type sparseBackwardReference struct {
	Input   []float32
	Source  []float32
	Weights []float32
}

func cpuSparseBackward(upstream, sourceOutput, weights []float32, sources, targets []int) sparseBackwardReference {
	result := sparseBackwardReference{
		Input:   append([]float32(nil), upstream...),
		Source:  make([]float32, len(upstream)),
		Weights: make([]float32, len(sources)),
	}
	for edge, source := range sources {
		target := targets[edge]
		result.Source[source] += weights[edge] * upstream[target]
		result.Weights[edge] = sourceOutput[source] * upstream[target]
	}
	return result
}

func maxBackwardAbsDiff(got BackwardResult, want sparseBackwardReference) float64 {
	maximum := float64(0)
	for _, pair := range []struct{ got, want []float32 }{
		{got.Input, want.Input},
		{got.Source, want.Source},
		{got.Weights, want.Weights},
	} {
		for i := range pair.want {
			diff := math.Abs(float64(pair.got[i] - pair.want[i]))
			if diff > maximum {
				maximum = diff
			}
		}
	}
	return maximum
}

func fillSparseBackwardGradientBuffers(ctx context.Context, backward *SparseBackward) error {
	const wgsl = `
@group(0) @binding(0) var<storage, read_write> weightGradients: array<f32>;
@group(0) @binding(1) var<storage, read_write> firstWeightGradient: array<f32>;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
    if (gid.x < arrayLength(&weightGradients)) {
        weightGradients[gid.x] = -123.75;
    }
    if (gid.x == 0u) {
        firstWeightGradient[0] = -123.75;
    }
}
`
	device := backward.device
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "coimnet-sparse-backward-test-sentinel-shader", WGSL: wgsl})
	if err != nil {
		return err
	}
	defer shader.Release()
	storage := &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}
	layout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{Label: "coimnet-sparse-backward-test-sentinel-layout", Entries: []wgpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: wgpu.ShaderStageCompute, Buffer: storage},
		{Binding: 1, Visibility: wgpu.ShaderStageCompute, Buffer: storage},
	}})
	if err != nil {
		return err
	}
	defer layout.Release()
	pipelineLayout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "coimnet-sparse-backward-test-sentinel-pipeline-layout", BindGroupLayouts: []*wgpu.BindGroupLayout{layout}})
	if err != nil {
		return err
	}
	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{Label: "coimnet-sparse-backward-test-sentinel-pipeline", Layout: pipelineLayout, Module: shader, EntryPoint: "main"})
	pipelineLayout.Release()
	if err != nil {
		return err
	}
	defer pipeline.Release()
	bindGroup, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{Label: "coimnet-sparse-backward-test-sentinel-bind-group", Layout: layout, Entries: []wgpu.BindGroupEntry{
		{Binding: 0, Buffer: backward.weightGradients, Size: backward.weightBytes},
		{Binding: 1, Buffer: backward.firstWeightGradient, Size: backward.firstWeightGradient.Size()},
	}})
	if err != nil {
		return err
	}
	defer bindGroup.Release()
	encoder, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "coimnet-sparse-backward-test-sentinel-encoder"})
	if err != nil {
		return err
	}
	pass, err := encoder.BeginComputePass(&wgpu.ComputePassDescriptor{Label: "coimnet-sparse-backward-test-sentinel-pass"})
	if err != nil {
		encoder.DiscardEncoding()
		return err
	}
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bindGroup, nil)
	groups := (uint64(backward.edges) + workgroupSize - 1) / workgroupSize
	pass.Dispatch(uint32(groups), 1, 1)
	if err := pass.End(); err != nil {
		encoder.DiscardEncoding()
		return err
	}
	commands, err := encoder.Finish()
	if err != nil {
		return err
	}
	if _, err := device.Queue().Submit(commands); err != nil {
		commands.Release()
		return err
	}
	return ctx.Err()
}
