package webgpu

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	_ "github.com/gogpu/wgpu/hal/allbackends"
)

// BackwardMetrics contains host-observed timings for one GPU sparse backward
// execution. Durations include the corresponding WebGPU API calls and are not
// device timestamp queries.
type BackwardMetrics struct {
	DeviceInfo
	Nodes            int
	Edges            int
	Initialization   time.Duration
	Compilation      time.Duration
	TopologyTransfer time.Duration
	Warmup           time.Duration
	Transfer         time.Duration
	Dispatch         time.Duration
	Readback         time.Duration
	BytesUploaded    uint64
}

// BackwardResult holds gradients for y = input + sparse(weights * sourceOutput).
// Upstream is dL/dy. Input is the identity-path gradient dL/dinput, Source is
// dL/dsourceOutput, and Weights contains one dL/dweight value per declared edge.
// Any activation derivative must be applied to Upstream by the caller.
type BackwardResult struct {
	Input   []float32
	Source  []float32
	Weights []float32
	Metrics BackwardMetrics
}

// SparseBackward owns a WebGPU device, source-grouped CSR topology, and buffers
// for one sparse weighted-sum gradient operation. Edges within each source
// retain their declaration order, so source gradients use deterministic f32
// accumulation order without atomics.
type SparseBackward struct {
	mu     sync.Mutex
	closed bool

	instance *wgpu.Instance
	adapter  *wgpu.Adapter
	device   *wgpu.Device
	info     DeviceInfo
	limits   wgpu.Limits

	nodes        int
	edges        int
	cpuEdgeOrder []uint32

	pipeline              *wgpu.ComputePipeline
	bindGroupLayout       *wgpu.BindGroupLayout
	bindGroup             *wgpu.BindGroup
	weightPipeline        *wgpu.ComputePipeline
	weightBindGroupLayout *wgpu.BindGroupLayout
	weightBindGroup       *wgpu.BindGroup
	firstPipeline         *wgpu.ComputePipeline
	firstBindGroupLayout  *wgpu.BindGroupLayout
	firstBindGroup        *wgpu.BindGroup

	offsets             *wgpu.Buffer
	sources             *wgpu.Buffer
	targets             *wgpu.Buffer
	edgeOrder           *wgpu.Buffer
	upstream            *wgpu.Buffer
	source              *wgpu.Buffer
	weights             *wgpu.Buffer
	firstWeightGradient *wgpu.Buffer
	sourceGradients     *wgpu.Buffer
	weightGradients     *wgpu.Buffer
	staging             *wgpu.Buffer

	nodeBytes        uint64
	weightBytes      uint64
	gradientBytes    uint64
	gradientValues   int
	firstSourceIndex uint32
	firstTargetIndex uint32
	metrics          BackwardMetrics
}

// sparseBackwardWGSL assigns one invocation to each source node and gathers
// the source derivative in deterministic edge order. No atomics are used.
const sparseBackwardWGSL = `
@group(0) @binding(0) var<storage, read> upstream: array<f32>;
@group(0) @binding(1) var<storage, read> weights: array<f32>;
@group(0) @binding(2) var<storage, read> targets: array<u32>;
@group(0) @binding(3) var<storage, read> edgeOrder: array<u32>;
@group(0) @binding(4) var<storage, read> offsets: array<u32>;
@group(0) @binding(5) var<storage, read_write> sourceGradients: array<f32>;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
    let source = gid.x;
    let nodeCount = arrayLength(&upstream);
    if (source >= nodeCount) {
        return;
    }

    let begin = offsets[source];
    let end = offsets[source + 1u];
    var sourceGradient = 0.0;
    for (var slot = begin; slot < end; slot = slot + 1u) {
        let edge = edgeOrder[slot];
        let target = targets[slot];
        let targetGradient = upstream[target];
        sourceGradient = sourceGradient + weights[edge] * targetGradient;
    }
    sourceGradients[source] = sourceGradient;
}
`

// sparseWeightBackwardWGSL assigns one invocation to each CSR edge slot. It
// writes gradients contiguously in CSR order; slot zero is isolated in the
// scalar pass below for the Metal first-slot workaround.
const sparseWeightBackwardWGSL = `
@group(0) @binding(0) var<storage, read> upstream: array<f32>;
@group(0) @binding(1) var<storage, read> sourceOutput: array<f32>;
@group(0) @binding(2) var<storage, read> sources: array<u32>;
@group(0) @binding(3) var<storage, read> targets: array<u32>;
@group(0) @binding(4) var<storage, read_write> weightGradients: array<f32>;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
    let slot = gid.x;
    if (slot == 0u || slot >= arrayLength(&targets)) {
        return;
    }
    let source = sources[slot];
    let target = targets[slot];
    weightGradients[slot] = sourceOutput[source] * upstream[target];
}
`

// sparseFirstWeightBackwardWGSL computes the first declared edge using
// topology indices embedded as constants. This isolates Metal's intermittent
// zero write for the first CSR weight-gradient slot.
const sparseFirstWeightBackwardWGSL = `
@group(0) @binding(0) var<storage, read> upstream: array<f32>;
@group(0) @binding(1) var<storage, read> sourceOutput: array<f32>;
@group(0) @binding(2) var<storage, read_write> firstWeightGradient: array<f32>;

@compute @workgroup_size(1)
fn main() {
    firstWeightGradient[0] = sourceOutput[%du] * upstream[%du];
}
`

type sourceTopology struct {
	offsets   []uint32
	sources   []uint32
	targets   []uint32
	edgeOrder []uint32
}

// NewSparseBackward validates cfg, selects a hardware WebGPU adapter, compiles
// the f32 sparse backward shader, uploads a source-grouped CSR topology, and
// performs a zero-input warmup. It never substitutes a CPU implementation.
func NewSparseBackward(ctx context.Context, cfg Config) (*SparseBackward, error) {
	if ctx == nil {
		return nil, errors.New("webgpu sparse backward: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	gradientValues, err := backwardGradientValueCount(cfg.Nodes, len(cfg.Sources))
	if err != nil {
		return nil, err
	}

	initStart := time.Now()
	instance, err := wgpu.CreateInstance(nil)
	if err != nil {
		return nil, fmt.Errorf("%w: create instance: %v", ErrUnavailable, err)
	}
	cleanup := func() { instance.Release() }
	if err := ctx.Err(); err != nil {
		cleanup()
		return nil, err
	}
	adapter, err := instance.RequestAdapter(&wgpu.RequestAdapterOptions{PowerPreference: wgpu.PowerPreferenceHighPerformance})
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("%w: request adapter: %v", ErrUnavailable, err)
	}
	cleanup = func() {
		adapter.Release()
		instance.Release()
	}
	adapterInfo := adapter.Info()
	if !usableHardwareAdapter(adapterInfo) {
		cleanup()
		return nil, fmt.Errorf("%w: adapter %q backend=%s type=%s", ErrUnavailable, adapterInfo.Name, adapterInfo.Backend, adapterInfo.DeviceType)
	}
	if err := ctx.Err(); err != nil {
		cleanup()
		return nil, err
	}
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("%w: request device for adapter %q: %v", ErrDevice, adapterInfo.Name, err)
	}
	info := DeviceInfo{
		Adapter:    adapterInfo.Name,
		Backend:    adapterInfo.Backend.String(),
		Vendor:     adapterInfo.Vendor,
		Driver:     adapterInfo.Driver,
		DeviceType: adapterInfo.DeviceType.String(),
	}
	var firstSourceIndex, firstTargetIndex uint32
	firstEdge := -1
	for edge, source := range cfg.Sources {
		if firstEdge < 0 || source < cfg.Sources[firstEdge] {
			firstEdge = edge
		}
	}
	if firstEdge >= 0 {
		firstSourceIndex = uint32(cfg.Sources[firstEdge])
		firstTargetIndex = uint32(cfg.Targets[firstEdge])
	}
	b := &SparseBackward{
		instance:         instance,
		adapter:          adapter,
		device:           device,
		info:             info,
		limits:           adapter.Limits(),
		nodes:            cfg.Nodes,
		edges:            len(cfg.Sources),
		gradientValues:   gradientValues,
		firstSourceIndex: firstSourceIndex,
		firstTargetIndex: firstTargetIndex,
		metrics:          BackwardMetrics{DeviceInfo: info, Nodes: cfg.Nodes, Edges: len(cfg.Sources)},
	}
	cleanup = b.releaseResources

	for _, check := range []struct {
		label string
		count int
	}{
		{label: "node buffers", count: cfg.Nodes},
		{label: "CSR offsets", count: cfg.Nodes + 1},
		{label: "edge buffers", count: len(cfg.Sources)},
		{label: "gradient buffer", count: gradientValues},
	} {
		if _, err := b.checkedBufferBytes(check.count, true); err != nil {
			cleanup()
			return nil, fmt.Errorf("%w: %s: %v", ErrCapacity, check.label, err)
		}
	}

	compileStart := time.Now()
	if err := b.compilePipeline(); err != nil {
		cleanup()
		return nil, err
	}
	b.metrics.Compilation = time.Since(compileStart)
	if err := ctx.Err(); err != nil {
		cleanup()
		return nil, err
	}
	topo, err := buildSourceTopology(cfg)
	if err != nil {
		cleanup()
		return nil, err
	}
	b.cpuEdgeOrder = append([]uint32(nil), topo.edgeOrder...)
	if err := b.createBuffers(topo); err != nil {
		cleanup()
		return nil, err
	}
	if err := b.createBindGroup(); err != nil {
		cleanup()
		return nil, err
	}
	topologyStart := time.Now()
	if err := b.uploadTopology(ctx, topo); err != nil {
		cleanup()
		return nil, err
	}
	b.metrics.TopologyTransfer = time.Since(topologyStart)
	warmupStart := time.Now()
	zerosNodes := make([]float32, cfg.Nodes)
	zerosWeights := make([]float32, len(cfg.Sources))
	if _, _, err := b.executeLocked(ctx, zerosNodes, zerosNodes, zerosWeights); err != nil {
		cleanup()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: warmup: %v", ErrExecution, err)
	}
	b.metrics.Warmup = time.Since(warmupStart)
	b.metrics.Initialization = time.Since(initStart)
	return b, nil
}

func backwardGradientValueCount(nodes, edges int) (int, error) {
	maxInt := int(^uint(0) >> 1)
	if edges > maxInt-nodes {
		return 0, fmt.Errorf("%w: gradient result length overflows int", ErrCapacity)
	}
	total := uint64(nodes) + uint64(edges)
	if total > maxUint32 {
		return 0, fmt.Errorf("%w: gradient result length %d exceeds uint32 shader indexing", ErrCapacity, total)
	}
	return int(total), nil
}

func buildSourceTopology(cfg Config) (sourceTopology, error) {
	if err := validateConfig(cfg); err != nil {
		return sourceTopology{}, err
	}
	counts := make([]uint64, cfg.Nodes)
	for _, source := range cfg.Sources {
		counts[source]++
	}
	offsets := make([]uint32, cfg.Nodes+1)
	var total uint64
	for source, count := range counts {
		total += count
		if total > maxUint32 {
			return sourceTopology{}, fmt.Errorf("%w: source CSR edge offset overflow at node %d", ErrInvalidConfig, source)
		}
		offsets[source+1] = uint32(total)
	}
	sources := make([]uint32, len(cfg.Sources))
	targets := make([]uint32, len(cfg.Targets))
	edgeOrder := make([]uint32, len(cfg.Sources))
	cursor := append([]uint32(nil), offsets[:cfg.Nodes]...)
	for edge, source := range cfg.Sources {
		slot := cursor[source]
		sources[slot] = uint32(source)
		targets[slot] = uint32(cfg.Targets[edge])
		edgeOrder[slot] = uint32(edge)
		cursor[source]++
	}
	return sourceTopology{offsets: offsets, sources: sources, targets: targets, edgeOrder: edgeOrder}, nil
}

func (b *SparseBackward) compilePipeline() error {
	shader, err := b.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "coimnet-sparse-backward-shader", WGSL: sparseBackwardWGSL})
	if err != nil {
		return fmt.Errorf("%w: create WGSL module: %v", ErrShaderCompile, err)
	}
	defer shader.Release()
	readOnlyStorage := &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage}
	storage := &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}
	entries := make([]wgpu.BindGroupLayoutEntry, 6)
	for i := 0; i < 5; i++ {
		entries[i] = wgpu.BindGroupLayoutEntry{Binding: uint32(i), Visibility: wgpu.ShaderStageCompute, Buffer: readOnlyStorage}
	}
	entries[5] = wgpu.BindGroupLayoutEntry{Binding: 5, Visibility: wgpu.ShaderStageCompute, Buffer: storage}
	layout, err := b.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{Label: "coimnet-sparse-backward-bindings", Entries: entries})
	if err != nil {
		return fmt.Errorf("%w: create bind group layout: %v", ErrShaderCompile, err)
	}
	pipelineLayout, err := b.device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "coimnet-sparse-backward-pipeline-layout", BindGroupLayouts: []*wgpu.BindGroupLayout{layout}})
	if err != nil {
		layout.Release()
		return fmt.Errorf("%w: create pipeline layout: %v", ErrShaderCompile, err)
	}
	pipeline, err := b.device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{Label: "coimnet-sparse-backward-pipeline", Layout: pipelineLayout, Module: shader, EntryPoint: "main"})
	pipelineLayout.Release()
	if err != nil {
		layout.Release()
		return fmt.Errorf("%w: create compute pipeline: %v", ErrShaderCompile, err)
	}
	b.bindGroupLayout = layout
	b.pipeline = pipeline

	weightShader, err := b.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "coimnet-sparse-weight-backward-shader", WGSL: sparseWeightBackwardWGSL})
	if err != nil {
		return fmt.Errorf("%w: create weight-gradient WGSL module: %v", ErrShaderCompile, err)
	}
	defer weightShader.Release()
	weightEntries := []wgpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: wgpu.ShaderStageCompute, Buffer: readOnlyStorage},
		{Binding: 1, Visibility: wgpu.ShaderStageCompute, Buffer: readOnlyStorage},
		{Binding: 2, Visibility: wgpu.ShaderStageCompute, Buffer: readOnlyStorage},
		{Binding: 3, Visibility: wgpu.ShaderStageCompute, Buffer: readOnlyStorage},
		{Binding: 4, Visibility: wgpu.ShaderStageCompute, Buffer: storage},
	}
	weightLayout, err := b.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{Label: "coimnet-sparse-weight-backward-bindings", Entries: weightEntries})
	if err != nil {
		return fmt.Errorf("%w: create weight-gradient bind group layout: %v", ErrShaderCompile, err)
	}
	weightPipelineLayout, err := b.device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "coimnet-sparse-weight-backward-pipeline-layout", BindGroupLayouts: []*wgpu.BindGroupLayout{weightLayout}})
	if err != nil {
		weightLayout.Release()
		return fmt.Errorf("%w: create weight-gradient pipeline layout: %v", ErrShaderCompile, err)
	}
	weightPipeline, err := b.device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{Label: "coimnet-sparse-weight-backward-pipeline", Layout: weightPipelineLayout, Module: weightShader, EntryPoint: "main"})
	weightPipelineLayout.Release()
	if err != nil {
		weightLayout.Release()
		return fmt.Errorf("%w: create weight-gradient compute pipeline: %v", ErrShaderCompile, err)
	}
	b.weightBindGroupLayout = weightLayout
	b.weightPipeline = weightPipeline

	firstWGSL := fmt.Sprintf(sparseFirstWeightBackwardWGSL, b.firstSourceIndex, b.firstTargetIndex)
	firstShader, err := b.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "coimnet-sparse-first-weight-backward-shader", WGSL: firstWGSL})
	if err != nil {
		return fmt.Errorf("%w: create first edge WGSL module: %v", ErrShaderCompile, err)
	}
	defer firstShader.Release()
	firstEntries := []wgpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: wgpu.ShaderStageCompute, Buffer: readOnlyStorage},
		{Binding: 1, Visibility: wgpu.ShaderStageCompute, Buffer: readOnlyStorage},
		{Binding: 2, Visibility: wgpu.ShaderStageCompute, Buffer: storage},
	}
	firstLayout, err := b.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{Label: "coimnet-sparse-first-weight-backward-bindings", Entries: firstEntries})
	if err != nil {
		return fmt.Errorf("%w: create first edge bind group layout: %v", ErrShaderCompile, err)
	}
	firstPipelineLayout, err := b.device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "coimnet-sparse-first-weight-backward-pipeline-layout", BindGroupLayouts: []*wgpu.BindGroupLayout{firstLayout}})
	if err != nil {
		firstLayout.Release()
		return fmt.Errorf("%w: create first edge pipeline layout: %v", ErrShaderCompile, err)
	}
	firstPipeline, err := b.device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{Label: "coimnet-sparse-first-weight-backward-pipeline", Layout: firstPipelineLayout, Module: firstShader, EntryPoint: "main"})
	firstPipelineLayout.Release()
	if err != nil {
		firstLayout.Release()
		return fmt.Errorf("%w: create first edge compute pipeline: %v", ErrShaderCompile, err)
	}
	b.firstBindGroupLayout = firstLayout
	b.firstPipeline = firstPipeline
	return nil
}

func (b *SparseBackward) checkedBufferBytes(count int, storage bool) (uint64, error) {
	if count < 0 || uint64(count) > math.MaxUint64/4 {
		return 0, fmt.Errorf("element count %d overflows byte size", count)
	}
	size := uint64(count) * 4
	if size == 0 {
		size = 4
	}
	if b.limits.MaxBufferSize != 0 && size > b.limits.MaxBufferSize {
		return 0, fmt.Errorf("%d bytes exceeds MaxBufferSize %d", size, b.limits.MaxBufferSize)
	}
	if storage && b.limits.MaxStorageBufferBindingSize != 0 && size > b.limits.MaxStorageBufferBindingSize {
		return 0, fmt.Errorf("%d bytes exceeds MaxStorageBufferBindingSize %d", size, b.limits.MaxStorageBufferBindingSize)
	}
	return size, nil
}

func (b *SparseBackward) createBuffers(topo sourceTopology) error {
	var err error
	if b.nodeBytes, err = b.checkedBufferBytes(b.nodes, true); err != nil {
		return fmt.Errorf("%w: node inputs: %v", ErrCapacity, err)
	}
	if b.weightBytes, err = b.checkedBufferBytes(b.edges, true); err != nil {
		return fmt.Errorf("%w: weights: %v", ErrCapacity, err)
	}
	if b.gradientBytes, err = b.checkedBufferBytes(b.gradientValues, false); err != nil {
		return fmt.Errorf("%w: gradients: %v", ErrCapacity, err)
	}
	makeStorage := func(label string, count int, usage gputypes.BufferUsage) (*wgpu.Buffer, error) {
		size, err := b.checkedBufferBytes(count, true)
		if err != nil {
			return nil, err
		}
		return b.device.CreateBuffer(&wgpu.BufferDescriptor{Label: label, Size: size, Usage: usage})
	}
	var usage gputypes.BufferUsage = wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst
	if b.offsets, err = makeStorage("coimnet-sparse-backward-offsets", len(topo.offsets), usage); err != nil {
		return fmt.Errorf("%w: offsets: %v", ErrCapacity, err)
	}
	if b.sources, err = makeStorage("coimnet-sparse-backward-sources", len(topo.sources), usage); err != nil {
		return fmt.Errorf("%w: sources: %v", ErrCapacity, err)
	}
	if b.targets, err = makeStorage("coimnet-sparse-backward-targets", len(topo.targets), usage); err != nil {
		return fmt.Errorf("%w: targets: %v", ErrCapacity, err)
	}
	if b.edgeOrder, err = makeStorage("coimnet-sparse-backward-edge-order", len(topo.edgeOrder), usage); err != nil {
		return fmt.Errorf("%w: edge order: %v", ErrCapacity, err)
	}
	if b.upstream, err = makeStorage("coimnet-sparse-backward-upstream", b.nodes, usage); err != nil {
		return fmt.Errorf("%w: upstream: %v", ErrCapacity, err)
	}
	if b.source, err = makeStorage("coimnet-sparse-backward-source-output", b.nodes, usage); err != nil {
		return fmt.Errorf("%w: source output: %v", ErrCapacity, err)
	}
	if b.weights, err = makeStorage("coimnet-sparse-backward-weights", b.edges, usage); err != nil {
		return fmt.Errorf("%w: weights: %v", ErrCapacity, err)
	}
	if b.firstWeightGradient, err = makeStorage("coimnet-sparse-backward-first-weight-gradient", 1, gputypes.BufferUsageStorage|wgpu.BufferUsageCopySrc); err != nil {
		return fmt.Errorf("%w: first weight gradient: %v", ErrCapacity, err)
	}
	if b.sourceGradients, err = makeStorage("coimnet-sparse-backward-source-gradients", b.nodes, gputypes.BufferUsageStorage|wgpu.BufferUsageCopySrc); err != nil {
		return fmt.Errorf("%w: source gradients: %v", ErrCapacity, err)
	}
	if b.weightGradients, err = makeStorage("coimnet-sparse-backward-weight-gradients", b.edges, gputypes.BufferUsageStorage|wgpu.BufferUsageCopySrc); err != nil {
		return fmt.Errorf("%w: weight gradients: %v", ErrCapacity, err)
	}
	b.staging, err = b.device.CreateBuffer(&wgpu.BufferDescriptor{Label: "coimnet-sparse-backward-readback", Size: b.gradientBytes, Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst})
	if err != nil {
		return fmt.Errorf("%w: readback: %v", ErrCapacity, err)
	}
	return nil
}

func (b *SparseBackward) createBindGroup() error {
	entries := []wgpu.BindGroupEntry{
		{Binding: 0, Buffer: b.upstream, Size: b.nodeBytes},
		{Binding: 1, Buffer: b.weights, Size: b.weightBytes},
		{Binding: 2, Buffer: b.targets, Size: b.targets.Size()},
		{Binding: 3, Buffer: b.edgeOrder, Size: b.edgeOrder.Size()},
		{Binding: 4, Buffer: b.offsets, Size: b.offsets.Size()},
		{Binding: 5, Buffer: b.sourceGradients, Size: b.nodeBytes},
	}
	var err error
	b.bindGroup, err = b.device.CreateBindGroup(&wgpu.BindGroupDescriptor{Label: "coimnet-sparse-backward-bind-group", Layout: b.bindGroupLayout, Entries: entries})
	if err != nil {
		return fmt.Errorf("%w: create bind group: %v", ErrDevice, err)
	}
	weightEntries := []wgpu.BindGroupEntry{
		{Binding: 0, Buffer: b.upstream, Size: b.nodeBytes},
		{Binding: 1, Buffer: b.source, Size: b.nodeBytes},
		{Binding: 2, Buffer: b.sources, Size: b.sources.Size()},
		{Binding: 3, Buffer: b.targets, Size: b.targets.Size()},
		{Binding: 4, Buffer: b.weightGradients, Size: b.weightBytes},
	}
	b.weightBindGroup, err = b.device.CreateBindGroup(&wgpu.BindGroupDescriptor{Label: "coimnet-sparse-weight-backward-bind-group", Layout: b.weightBindGroupLayout, Entries: weightEntries})
	if err != nil {
		return fmt.Errorf("%w: create weight-gradient bind group: %v", ErrDevice, err)
	}
	firstEntries := []wgpu.BindGroupEntry{
		{Binding: 0, Buffer: b.upstream, Size: b.nodeBytes},
		{Binding: 1, Buffer: b.source, Size: b.nodeBytes},
		{Binding: 2, Buffer: b.firstWeightGradient, Size: b.firstWeightGradient.Size()},
	}
	b.firstBindGroup, err = b.device.CreateBindGroup(&wgpu.BindGroupDescriptor{Label: "coimnet-sparse-first-weight-backward-bind-group", Layout: b.firstBindGroupLayout, Entries: firstEntries})
	if err != nil {
		return fmt.Errorf("%w: create first edge bind group: %v", ErrDevice, err)
	}
	return nil
}

func (b *SparseBackward) uploadTopology(ctx context.Context, topo sourceTopology) error {
	for _, upload := range []struct {
		buffer *wgpu.Buffer
		data   []byte
		name   string
	}{
		{buffer: b.offsets, data: uint32Bytes(topo.offsets), name: "offsets"},
		{buffer: b.sources, data: uint32Bytes(topo.sources), name: "sources"},
		{buffer: b.targets, data: uint32Bytes(topo.targets), name: "targets"},
		{buffer: b.edgeOrder, data: uint32Bytes(topo.edgeOrder), name: "edge order"},
	} {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := b.device.Queue().WriteBuffer(upload.buffer, 0, upload.data); err != nil {
			return fmt.Errorf("%w: upload %s: %v", ErrExecution, upload.name, err)
		}
	}
	return nil
}

// Backward computes gradients for y[i] = input[i] + sum_e(weights[e] *
// sourceOutput[source[e]]) over edges whose target is i. Upstream supplies
// dL/dy. The caller applies any neuron activation derivative before this call.
func (b *SparseBackward) Backward(ctx context.Context, upstream, sourceOutput, weights []float32) (BackwardResult, error) {
	var empty BackwardResult
	if ctx == nil {
		return empty, errors.New("webgpu sparse backward: nil context")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return empty, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if err := validateValues(upstream, b.nodes, "upstream"); err != nil {
		return empty, err
	}
	if err := validateValues(sourceOutput, b.nodes, "source output"); err != nil {
		return empty, err
	}
	if err := validateValues(weights, b.edges, "weights"); err != nil {
		return empty, err
	}
	result, metrics, err := b.executeLocked(ctx, upstream, sourceOutput, weights)
	if err == nil {
		b.metrics = metrics
	}
	return result, err
}

func (b *SparseBackward) executeLocked(ctx context.Context, upstream, sourceOutput, weights []float32) (BackwardResult, BackwardMetrics, error) {
	metrics := b.metrics
	metrics.Transfer = 0
	metrics.Dispatch = 0
	metrics.Readback = 0
	metrics.BytesUploaded = 0
	if err := ctx.Err(); err != nil {
		return BackwardResult{Metrics: metrics}, metrics, err
	}
	transferStart := time.Now()
	for _, upload := range []struct {
		buffer *wgpu.Buffer
		data   []float32
		name   string
	}{
		{buffer: b.upstream, data: upstream, name: "upstream"},
		{buffer: b.source, data: sourceOutput, name: "source output"},
		{buffer: b.weights, data: weights, name: "weights"},
	} {
		if err := b.device.Queue().WriteBuffer(upload.buffer, 0, float32Bytes(upload.data)); err != nil {
			return BackwardResult{Metrics: metrics}, metrics, fmt.Errorf("%w: upload %s: %v", ErrExecution, upload.name, err)
		}
	}
	metrics.Transfer = time.Since(transferStart)
	metrics.BytesUploaded = b.nodeBytes*2 + b.weightBytes
	if err := ctx.Err(); err != nil {
		return BackwardResult{Metrics: metrics}, metrics, err
	}
	dispatchStart := time.Now()
	encoder, err := b.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "coimnet-sparse-backward-encoder"})
	if err != nil {
		return BackwardResult{Metrics: metrics}, metrics, fmt.Errorf("%w: create command encoder: %v", ErrExecution, err)
	}
	pass, err := encoder.BeginComputePass(&wgpu.ComputePassDescriptor{Label: "coimnet-sparse-backward-pass"})
	if err != nil {
		encoder.DiscardEncoding()
		return BackwardResult{Metrics: metrics}, metrics, fmt.Errorf("%w: begin compute pass: %v", ErrExecution, err)
	}
	pass.SetPipeline(b.pipeline)
	pass.SetBindGroup(0, b.bindGroup, nil)
	groups64 := (uint64(b.nodes) + workgroupSize - 1) / workgroupSize
	if groups64 > uint64(b.limits.MaxComputeWorkgroupsPerDimension) && b.limits.MaxComputeWorkgroupsPerDimension != 0 {
		_ = pass.End()
		encoder.DiscardEncoding()
		return BackwardResult{Metrics: metrics}, metrics, fmt.Errorf("%w: dispatch groups %d exceed device limit %d", ErrCapacity, groups64, b.limits.MaxComputeWorkgroupsPerDimension)
	}
	pass.Dispatch(uint32(groups64), 1, 1)
	if err := pass.End(); err != nil {
		encoder.DiscardEncoding()
		return BackwardResult{Metrics: metrics}, metrics, fmt.Errorf("%w: end compute pass: %v", ErrExecution, err)
	}
	if b.edges > 1 {
		weightPass, err := encoder.BeginComputePass(&wgpu.ComputePassDescriptor{Label: "coimnet-sparse-weight-backward-pass"})
		if err != nil {
			encoder.DiscardEncoding()
			return BackwardResult{Metrics: metrics}, metrics, fmt.Errorf("%w: begin weight-gradient compute pass: %v", ErrExecution, err)
		}
		weightPass.SetPipeline(b.weightPipeline)
		weightPass.SetBindGroup(0, b.weightBindGroup, nil)
		weightGroups64 := (uint64(b.edges) + workgroupSize - 1) / workgroupSize
		if weightGroups64 > uint64(b.limits.MaxComputeWorkgroupsPerDimension) && b.limits.MaxComputeWorkgroupsPerDimension != 0 {
			_ = weightPass.End()
			encoder.DiscardEncoding()
			return BackwardResult{Metrics: metrics}, metrics, fmt.Errorf("%w: weight-gradient dispatch groups %d exceed device limit %d", ErrCapacity, weightGroups64, b.limits.MaxComputeWorkgroupsPerDimension)
		}
		weightPass.Dispatch(uint32(weightGroups64), 1, 1)
		if err := weightPass.End(); err != nil {
			encoder.DiscardEncoding()
			return BackwardResult{Metrics: metrics}, metrics, fmt.Errorf("%w: end weight-gradient compute pass: %v", ErrExecution, err)
		}
	}
	if b.edges > 0 {
		firstPass, err := encoder.BeginComputePass(&wgpu.ComputePassDescriptor{Label: "coimnet-sparse-first-weight-backward-pass"})
		if err != nil {
			encoder.DiscardEncoding()
			return BackwardResult{Metrics: metrics}, metrics, fmt.Errorf("%w: begin first edge compute pass: %v", ErrExecution, err)
		}
		firstPass.SetPipeline(b.firstPipeline)
		firstPass.SetBindGroup(0, b.firstBindGroup, nil)
		firstPass.Dispatch(1, 1, 1)
		if err := firstPass.End(); err != nil {
			encoder.DiscardEncoding()
			return BackwardResult{Metrics: metrics}, metrics, fmt.Errorf("%w: end first edge compute pass: %v", ErrExecution, err)
		}
	}
	encoder.CopyBufferToBuffer(b.sourceGradients, 0, b.staging, 0, b.nodeBytes)
	if b.edges > 0 {
		encoder.CopyBufferToBuffer(b.firstWeightGradient, 0, b.staging, b.nodeBytes, 4)
	}
	if b.edges > 1 {
		restBytes := uint64(b.edges-1) * 4
		encoder.CopyBufferToBuffer(b.weightGradients, 4, b.staging, b.nodeBytes+4, restBytes)
	}
	commands, err := encoder.Finish()
	if err != nil {
		return BackwardResult{Metrics: metrics}, metrics, fmt.Errorf("%w: finish commands: %v", ErrExecution, err)
	}
	if _, err := b.device.Queue().Submit(commands); err != nil {
		commands.Release()
		return BackwardResult{Metrics: metrics}, metrics, fmt.Errorf("%w: submit: %v", ErrExecution, err)
	}
	metrics.Dispatch = time.Since(dispatchStart)
	if err := ctx.Err(); err != nil {
		return BackwardResult{Metrics: metrics}, metrics, err
	}
	readbackStart := time.Now()
	if err := b.staging.Map(ctx, wgpu.MapModeRead, 0, b.gradientBytes); err != nil {
		_ = b.staging.Unmap()
		metrics.Readback = time.Since(readbackStart)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return BackwardResult{Metrics: metrics}, metrics, err
		}
		return BackwardResult{Metrics: metrics}, metrics, fmt.Errorf("%w: map readback: %v", ErrExecution, err)
	}
	rangeView, err := b.staging.MappedRange(0, b.gradientBytes)
	if err != nil {
		_ = b.staging.Unmap()
		metrics.Readback = time.Since(readbackStart)
		return BackwardResult{Metrics: metrics}, metrics, fmt.Errorf("%w: read mapped readback: %v", ErrExecution, err)
	}
	raw := rangeView.Bytes()
	result := BackwardResult{
		Input:   append([]float32(nil), upstream...),
		Source:  make([]float32, b.nodes),
		Weights: make([]float32, b.edges),
		Metrics: metrics,
	}
	for i := 0; i < b.gradientValues; i++ {
		value := math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			_ = b.staging.Unmap()
			metrics.Readback = time.Since(readbackStart)
			return BackwardResult{Metrics: metrics}, metrics, fmt.Errorf("%w: gradient[%d] is %v", ErrNonFinite, i, value)
		}
		switch {
		case i < b.nodes:
			result.Source[i] = value
		default:
			slot := i - b.nodes
			result.Weights[b.cpuEdgeOrder[slot]] = value
		}
	}
	if err := b.staging.Unmap(); err != nil {
		metrics.Readback = time.Since(readbackStart)
		return BackwardResult{Metrics: metrics}, metrics, fmt.Errorf("%w: unmap readback: %v", ErrExecution, err)
	}
	metrics.Readback = time.Since(readbackStart)
	result.Metrics = metrics
	return result, metrics, nil
}

// Info returns the selected adapter identity.
func (b *SparseBackward) Info() DeviceInfo {
	if b == nil {
		return DeviceInfo{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.info
}

// Metrics returns the most recently completed execution measurements.
func (b *SparseBackward) Metrics() BackwardMetrics {
	if b == nil {
		return BackwardMetrics{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.metrics
}

// Close releases all resources owned by the sparse backward executor.
func (b *SparseBackward) Close() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	var waitErr error
	if b.device != nil {
		waitErr = b.device.WaitIdle()
	}
	b.releaseResources()
	if waitErr != nil {
		return fmt.Errorf("%w: wait for device idle during close: %v", ErrExecution, waitErr)
	}
	return nil
}

func (b *SparseBackward) releaseResources() {
	if b.bindGroup != nil {
		b.bindGroup.Release()
		b.bindGroup = nil
	}
	if b.firstBindGroup != nil {
		b.firstBindGroup.Release()
		b.firstBindGroup = nil
	}
	if b.weightBindGroup != nil {
		b.weightBindGroup.Release()
		b.weightBindGroup = nil
	}
	if b.pipeline != nil {
		b.pipeline.Release()
		b.pipeline = nil
	}
	if b.firstPipeline != nil {
		b.firstPipeline.Release()
		b.firstPipeline = nil
	}
	if b.weightPipeline != nil {
		b.weightPipeline.Release()
		b.weightPipeline = nil
	}
	if b.bindGroupLayout != nil {
		b.bindGroupLayout.Release()
		b.bindGroupLayout = nil
	}
	if b.firstBindGroupLayout != nil {
		b.firstBindGroupLayout.Release()
		b.firstBindGroupLayout = nil
	}
	if b.weightBindGroupLayout != nil {
		b.weightBindGroupLayout.Release()
		b.weightBindGroupLayout = nil
	}
	for _, buffer := range []*wgpu.Buffer{b.staging, b.weightGradients, b.sourceGradients, b.firstWeightGradient, b.weights, b.source, b.upstream, b.edgeOrder, b.targets, b.sources, b.offsets} {
		if buffer != nil {
			buffer.Release()
		}
	}
	b.staging = nil
	b.weightGradients = nil
	b.sourceGradients = nil
	b.firstWeightGradient = nil
	b.weights = nil
	b.source = nil
	b.upstream = nil
	b.edgeOrder = nil
	b.targets = nil
	b.sources = nil
	b.offsets = nil
	if b.device != nil {
		b.device.Release()
		b.device = nil
	}
	if b.adapter != nil {
		b.adapter.Release()
		b.adapter = nil
	}
	if b.instance != nil {
		b.instance.Release()
		b.instance = nil
	}
}
