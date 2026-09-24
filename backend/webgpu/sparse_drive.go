package webgpu

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	_ "github.com/gogpu/wgpu/hal/allbackends"
)

const (
	workgroupSize = 64
	maxUint32     = uint64(^uint32(0))
)

// Errors returned by this package. The original error is wrapped so callers
// can use errors.Is while retaining the operation and resource in the message.
var (
	ErrUnavailable   = errors.New("webgpu sparse drive: no usable hardware adapter")
	ErrDevice        = errors.New("webgpu sparse drive: device initialization failed")
	ErrShaderCompile = errors.New("webgpu sparse drive: shader compilation failed")
	ErrExecution     = errors.New("webgpu sparse drive: GPU execution failed")
	ErrInvalidConfig = errors.New("webgpu sparse drive: invalid topology")
	ErrShape         = errors.New("webgpu sparse drive: invalid input shape")
	ErrNonFinite     = errors.New("webgpu sparse drive: non-finite value")
	ErrCapacity      = errors.New("webgpu sparse drive: GPU buffer capacity exceeded")
	ErrClosed        = errors.New("webgpu sparse drive: executor is closed")
)

// Config describes the immutable sparse topology. Sources[e] and Targets[e]
// form one directed edge. Duplicate edges are retained as separate additive
// connections.
type Config struct {
	Nodes   int
	Sources []int
	Targets []int
}

// DeviceInfo identifies the adapter selected by WebGPU.
type DeviceInfo struct {
	Adapter    string
	Backend    string
	Vendor     string
	Driver     string
	DeviceType string
}

// Metrics contains host-observed timings for construction and the most recent
// Execute call. WebGPU backends do not all expose GPU timestamp queries, so
// these are wall-clock measurements around the corresponding API phases.
type Metrics struct {
	Adapter          string
	Backend          string
	Vendor           string
	Driver           string
	DeviceType       string
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

// Result is the host-visible result of one sparse drive execution.
type Result struct {
	Output  []float32
	Metrics Metrics
}

// SparseDrive owns a WebGPU device, a compiled sparse-drive pipeline, and
// persistent topology/input/output buffers. Execute calls are serialized so a
// result readback cannot race with the next upload.
type SparseDrive struct {
	mu     sync.Mutex
	closed bool

	instance *wgpu.Instance
	adapter  *wgpu.Adapter
	device   *wgpu.Device
	info     DeviceInfo
	limits   wgpu.Limits

	nodes int
	edges int

	pipeline        *wgpu.ComputePipeline
	bindGroupLayout *wgpu.BindGroupLayout
	bindGroup       *wgpu.BindGroup

	offsets     *wgpu.Buffer
	sources     *wgpu.Buffer
	edgeOrder   *wgpu.Buffer
	input       *wgpu.Buffer
	sourceValue *wgpu.Buffer
	weights     *wgpu.Buffer
	result      *wgpu.Buffer
	staging     *wgpu.Buffer

	inputBytes       uint64
	sourceValueBytes uint64
	weightBytes      uint64
	resultBytes      uint64

	metrics Metrics
}

// sparseDriveWGSL assigns one invocation to each target. The offsets, source
// indices, and declaration-order edge indices are immutable CSR topology.
// The loop is intentionally serial per target: this preserves the declared
// edge order and therefore the operation's f32 rounding semantics.
const sparseDriveWGSL = `
@group(0) @binding(0) var<storage, read> input: array<f32>;
@group(0) @binding(1) var<storage, read> sourceOutput: array<f32>;
@group(0) @binding(2) var<storage, read> weights: array<f32>;
@group(0) @binding(3) var<storage, read> sources: array<u32>;
@group(0) @binding(4) var<storage, read> edgeOrder: array<u32>;
@group(0) @binding(5) var<storage, read> offsets: array<u32>;
@group(0) @binding(6) var<storage, read_write> result: array<f32>;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
    let target = gid.x;
    let nodeCount = arrayLength(&offsets) - 1u;
    if (target >= nodeCount) {
        return;
    }

    var acc = input[target];
    let begin = offsets[target];
    let end = offsets[target + 1u];
    for (var slot = begin; slot < end; slot = slot + 1u) {
        let edge = edgeOrder[slot];
        acc = acc + weights[edge] * sourceOutput[sources[slot]];
    }
    result[target] = acc;
}
`

type topology struct {
	offsets   []uint32
	sources   []uint32
	edgeOrder []uint32
}

func validateConfig(cfg Config) error {
	if cfg.Nodes <= 0 {
		return fmt.Errorf("%w: nodes must be positive, got %d", ErrInvalidConfig, cfg.Nodes)
	}
	if uint64(cfg.Nodes) > maxUint32 {
		return fmt.Errorf("%w: nodes %d exceed uint32 index range", ErrInvalidConfig, cfg.Nodes)
	}
	if cfg.Nodes == int(^uint(0)>>1) {
		return fmt.Errorf("%w: nodes %d overflow the CSR node-count increment", ErrInvalidConfig, cfg.Nodes)
	}
	if len(cfg.Sources) != len(cfg.Targets) {
		return fmt.Errorf("%w: sources length %d differs from targets length %d", ErrInvalidConfig, len(cfg.Sources), len(cfg.Targets))
	}
	if uint64(len(cfg.Sources)) > maxUint32 {
		return fmt.Errorf("%w: edge count %d exceeds uint32 index range", ErrInvalidConfig, len(cfg.Sources))
	}
	for e, source := range cfg.Sources {
		target := cfg.Targets[e]
		if source < 0 || uint64(source) >= uint64(cfg.Nodes) {
			return fmt.Errorf("%w: source[%d]=%d is outside [0,%d)", ErrInvalidConfig, e, source, cfg.Nodes)
		}
		if target < 0 || uint64(target) >= uint64(cfg.Nodes) {
			return fmt.Errorf("%w: target[%d]=%d is outside [0,%d)", ErrInvalidConfig, e, target, cfg.Nodes)
		}
	}
	return nil
}

func buildTopology(cfg Config) (topology, error) {
	if err := validateConfig(cfg); err != nil {
		return topology{}, err
	}
	counts := make([]uint64, cfg.Nodes)
	for _, target := range cfg.Targets {
		counts[target]++
	}

	offsets := make([]uint32, cfg.Nodes+1)
	var total uint64
	for target, count := range counts {
		total += count
		if total > maxUint32 {
			return topology{}, fmt.Errorf("%w: CSR edge offset overflow at target %d", ErrInvalidConfig, target)
		}
		offsets[target+1] = uint32(total)
	}

	sources := make([]uint32, len(cfg.Sources))
	edgeOrder := make([]uint32, len(cfg.Sources))
	cursor := append([]uint32(nil), offsets[:cfg.Nodes]...)
	for e, source := range cfg.Sources {
		target := cfg.Targets[e]
		slot := cursor[target]
		sources[slot] = uint32(source)
		edgeOrder[slot] = uint32(e)
		cursor[target]++
	}
	return topology{offsets: offsets, sources: sources, edgeOrder: edgeOrder}, nil
}

// NewSparseDrive validates cfg, selects a hardware WebGPU adapter, compiles
// the f32 sparse-drive shader, uploads the immutable CSR topology, and runs a
// zero-input warmup dispatch. It never substitutes a CPU implementation.
func NewSparseDrive(ctx context.Context, cfg Config) (*SparseDrive, error) {
	if ctx == nil {
		return nil, errors.New("webgpu sparse drive: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	initStart := time.Now()
	instance, err := wgpu.CreateInstance(nil)
	if err != nil {
		return nil, fmt.Errorf("%w: create instance: %v", ErrUnavailable, err)
	}
	cleanup := func() {
		if instance != nil {
			instance.Release()
		}
	}
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
	cleanup = func() {
		device.Release()
		adapter.Release()
		instance.Release()
	}
	if err := ctx.Err(); err != nil {
		cleanup()
		return nil, err
	}

	info := DeviceInfo{
		Adapter:    adapterInfo.Name,
		Backend:    adapterInfo.Backend.String(),
		Vendor:     adapterInfo.Vendor,
		Driver:     adapterInfo.Driver,
		DeviceType: adapterInfo.DeviceType.String(),
	}
	d := &SparseDrive{
		instance: instance,
		adapter:  adapter,
		device:   device,
		info:     info,
		limits:   adapter.Limits(),
		nodes:    cfg.Nodes,
		edges:    len(cfg.Sources),
		metrics: Metrics{
			Adapter:    info.Adapter,
			Backend:    info.Backend,
			Vendor:     info.Vendor,
			Driver:     info.Driver,
			DeviceType: info.DeviceType,
			Nodes:      cfg.Nodes,
			Edges:      len(cfg.Sources),
		},
	}
	// Reject configurations that cannot fit in the selected adapter before
	// allocating the dense CSR offset array. This keeps malformed or oversized
	// integer shapes from turning into a host allocation panic.
	capacityChecks := []struct {
		label string
		count int
	}{
		{label: "node buffers", count: cfg.Nodes},
		{label: "CSR offsets", count: cfg.Nodes + 1},
		{label: "edge buffers", count: len(cfg.Sources)},
	}
	for _, check := range capacityChecks {
		if _, err := d.checkedBufferBytes(check.count, true); err != nil {
			cleanup()
			return nil, fmt.Errorf("%w: %s: %v", ErrCapacity, check.label, err)
		}
	}

	compileStart := time.Now()
	if err := d.compilePipeline(); err != nil {
		cleanup()
		return nil, err
	}
	d.metrics.Compilation = time.Since(compileStart)
	if err := ctx.Err(); err != nil {
		d.releaseResources()
		return nil, err
	}

	topo, err := buildTopology(cfg)
	if err != nil {
		d.releaseResources()
		return nil, err
	}
	if err := d.createBuffers(topo); err != nil {
		d.releaseResources()
		return nil, err
	}
	if err := d.createBindGroup(); err != nil {
		d.releaseResources()
		return nil, err
	}

	topologyStart := time.Now()
	if err := d.uploadTopology(ctx, topo); err != nil {
		d.releaseResources()
		return nil, err
	}
	d.metrics.TopologyTransfer = time.Since(topologyStart)

	warmupStart := time.Now()
	zerosNodes := make([]float32, cfg.Nodes)
	zerosWeights := make([]float32, len(cfg.Sources))
	if _, _, err := d.executeLocked(ctx, zerosNodes, zerosNodes, zerosWeights); err != nil {
		d.releaseResources()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: warmup: %v", ErrExecution, err)
	}
	d.metrics.Warmup = time.Since(warmupStart)
	d.metrics.Initialization = time.Since(initStart)
	return d, nil
}

func usableHardwareAdapter(info gputypes.AdapterInfo) bool {
	if info.Backend == gputypes.BackendEmpty || info.DeviceType == gputypes.DeviceTypeCPU {
		return false
	}
	name := strings.ToLower(info.Name)
	return !strings.Contains(name, "software") && !strings.Contains(name, "llvmpipe") && !strings.Contains(name, "swiftshader")
}

func (d *SparseDrive) compilePipeline() error {
	shader, err := d.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "coimnet-sparse-drive-shader",
		WGSL:  sparseDriveWGSL,
	})
	if err != nil {
		return fmt.Errorf("%w: create WGSL module: %v", ErrShaderCompile, err)
	}
	defer shader.Release()

	readOnlyStorage := &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage}
	storage := &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}
	layout, err := d.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "coimnet-sparse-drive-bindings",
		Entries: []wgpu.BindGroupLayoutEntry{
			{Binding: 0, Visibility: wgpu.ShaderStageCompute, Buffer: readOnlyStorage},
			{Binding: 1, Visibility: wgpu.ShaderStageCompute, Buffer: readOnlyStorage},
			{Binding: 2, Visibility: wgpu.ShaderStageCompute, Buffer: readOnlyStorage},
			{Binding: 3, Visibility: wgpu.ShaderStageCompute, Buffer: readOnlyStorage},
			{Binding: 4, Visibility: wgpu.ShaderStageCompute, Buffer: readOnlyStorage},
			{Binding: 5, Visibility: wgpu.ShaderStageCompute, Buffer: readOnlyStorage},
			{Binding: 6, Visibility: wgpu.ShaderStageCompute, Buffer: storage},
		},
	})
	if err != nil {
		return fmt.Errorf("%w: create bind group layout: %v", ErrShaderCompile, err)
	}

	pipelineLayout, err := d.device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "coimnet-sparse-drive-pipeline-layout",
		BindGroupLayouts: []*wgpu.BindGroupLayout{layout},
	})
	if err != nil {
		layout.Release()
		return fmt.Errorf("%w: create pipeline layout: %v", ErrShaderCompile, err)
	}
	pipeline, err := d.device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "coimnet-sparse-drive-pipeline",
		Layout:     pipelineLayout,
		Module:     shader,
		EntryPoint: "main",
	})
	pipelineLayout.Release()
	if err != nil {
		layout.Release()
		return fmt.Errorf("%w: create compute pipeline: %v", ErrShaderCompile, err)
	}
	d.bindGroupLayout = layout
	d.pipeline = pipeline
	return nil
}

func (d *SparseDrive) createBuffers(topo topology) error {
	var err error
	if d.inputBytes, err = d.checkedBufferBytes(d.nodes, true); err != nil {
		return fmt.Errorf("%w: input: %v", ErrCapacity, err)
	}
	if d.sourceValueBytes, err = d.checkedBufferBytes(d.nodes, true); err != nil {
		return fmt.Errorf("%w: source output: %v", ErrCapacity, err)
	}
	if d.weightBytes, err = d.checkedBufferBytes(d.edges, true); err != nil {
		return fmt.Errorf("%w: weights: %v", ErrCapacity, err)
	}
	if d.resultBytes, err = d.checkedBufferBytes(d.nodes, true); err != nil {
		return fmt.Errorf("%w: result: %v", ErrCapacity, err)
	}
	makeStorage := func(label string, count int) (*wgpu.Buffer, error) {
		size, err := d.checkedBufferBytes(count, true)
		if err != nil {
			return nil, err
		}
		return d.device.CreateBuffer(&wgpu.BufferDescriptor{Label: label, Size: size, Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst})
	}
	makeReadOnly := func(label string, values []uint32) (*wgpu.Buffer, error) {
		b, err := makeStorage(label, len(values))
		if err != nil {
			return nil, err
		}
		return b, nil
	}

	if d.offsets, err = makeReadOnly("coimnet-sparse-drive-offsets", topo.offsets); err != nil {
		return fmt.Errorf("%w: offsets: %v", ErrCapacity, err)
	}
	if d.sources, err = makeReadOnly("coimnet-sparse-drive-sources", topo.sources); err != nil {
		return fmt.Errorf("%w: sources: %v", ErrCapacity, err)
	}
	if d.edgeOrder, err = makeReadOnly("coimnet-sparse-drive-edge-order", topo.edgeOrder); err != nil {
		return fmt.Errorf("%w: edge order: %v", ErrCapacity, err)
	}
	if d.input, err = d.device.CreateBuffer(&wgpu.BufferDescriptor{Label: "coimnet-sparse-drive-input", Size: d.inputBytes, Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst}); err != nil {
		return fmt.Errorf("%w: input: %v", ErrCapacity, err)
	}
	if d.sourceValue, err = d.device.CreateBuffer(&wgpu.BufferDescriptor{Label: "coimnet-sparse-drive-source-output", Size: d.sourceValueBytes, Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst}); err != nil {
		return fmt.Errorf("%w: source output: %v", ErrCapacity, err)
	}
	if d.weights, err = d.device.CreateBuffer(&wgpu.BufferDescriptor{Label: "coimnet-sparse-drive-weights", Size: d.weightBytes, Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst}); err != nil {
		return fmt.Errorf("%w: weights: %v", ErrCapacity, err)
	}
	if d.result, err = d.device.CreateBuffer(&wgpu.BufferDescriptor{Label: "coimnet-sparse-drive-result", Size: d.resultBytes, Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc}); err != nil {
		return fmt.Errorf("%w: result: %v", ErrCapacity, err)
	}
	if d.staging, err = d.device.CreateBuffer(&wgpu.BufferDescriptor{Label: "coimnet-sparse-drive-readback", Size: d.resultBytes, Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst}); err != nil {
		return fmt.Errorf("%w: readback: %v", ErrCapacity, err)
	}
	return nil
}

func (d *SparseDrive) checkedBufferBytes(count int, storage bool) (uint64, error) {
	if count < 0 || uint64(count) > math.MaxUint64/4 {
		return 0, fmt.Errorf("element count %d overflows byte size", count)
	}
	size := uint64(count) * 4
	if size == 0 {
		size = 4
	}
	if d.limits.MaxBufferSize != 0 && size > d.limits.MaxBufferSize {
		return 0, fmt.Errorf("%d bytes exceeds MaxBufferSize %d", size, d.limits.MaxBufferSize)
	}
	if storage && d.limits.MaxStorageBufferBindingSize != 0 && size > d.limits.MaxStorageBufferBindingSize {
		return 0, fmt.Errorf("%d bytes exceeds MaxStorageBufferBindingSize %d", size, d.limits.MaxStorageBufferBindingSize)
	}
	return size, nil
}

func (d *SparseDrive) createBindGroup() error {
	entries := []wgpu.BindGroupEntry{
		{Binding: 0, Buffer: d.input, Size: d.inputBytes},
		{Binding: 1, Buffer: d.sourceValue, Size: d.sourceValueBytes},
		{Binding: 2, Buffer: d.weights, Size: d.weightBytes},
		{Binding: 3, Buffer: d.sources, Size: d.sources.Size()},
		{Binding: 4, Buffer: d.edgeOrder, Size: d.edgeOrder.Size()},
		{Binding: 5, Buffer: d.offsets, Size: d.offsets.Size()},
		{Binding: 6, Buffer: d.result, Size: d.resultBytes},
	}
	var err error
	d.bindGroup, err = d.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:   "coimnet-sparse-drive-bind-group",
		Layout:  d.bindGroupLayout,
		Entries: entries,
	})
	if err != nil {
		return fmt.Errorf("%w: create bind group: %v", ErrExecution, err)
	}
	return nil
}

func (d *SparseDrive) uploadTopology(ctx context.Context, topo topology) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := d.device.Queue().WriteBuffer(d.offsets, 0, uint32Bytes(topo.offsets)); err != nil {
		return fmt.Errorf("%w: upload offsets: %v", ErrExecution, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := d.device.Queue().WriteBuffer(d.sources, 0, uint32Bytes(topo.sources)); err != nil {
		return fmt.Errorf("%w: upload sources: %v", ErrExecution, err)
	}
	if err := d.device.Queue().WriteBuffer(d.edgeOrder, 0, uint32Bytes(topo.edgeOrder)); err != nil {
		return fmt.Errorf("%w: upload edge order: %v", ErrExecution, err)
	}
	return nil
}

// Execute computes result[target] = input[target] + sum(weight[e] *
// output[source[e]]) on the selected GPU. The input slices are not modified.
func (d *SparseDrive) Execute(ctx context.Context, input, output, weights []float32) (Result, error) {
	var empty Result
	if ctx == nil {
		return empty, errors.New("webgpu sparse drive: nil context")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return empty, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if err := validateValues(input, d.nodes, "input"); err != nil {
		return empty, err
	}
	if err := validateValues(output, d.nodes, "source output"); err != nil {
		return empty, err
	}
	if err := validateValues(weights, d.edges, "weights"); err != nil {
		return empty, err
	}
	result, metrics, err := d.executeLocked(ctx, input, output, weights)
	if err == nil {
		d.metrics = metrics
	}
	return result, err
}

func validateValues(values []float32, want int, name string) error {
	if len(values) != want {
		return fmt.Errorf("%w: %s length %d, want %d", ErrShape, name, len(values), want)
	}
	for i, value := range values {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return fmt.Errorf("%w: %s[%d] is %v", ErrNonFinite, name, i, value)
		}
	}
	return nil
}

func (d *SparseDrive) executeLocked(ctx context.Context, input, output, weights []float32) (Result, Metrics, error) {
	metrics := d.metrics
	metrics.Transfer = 0
	metrics.Dispatch = 0
	metrics.Readback = 0
	metrics.BytesUploaded = 0
	if err := ctx.Err(); err != nil {
		return Result{Metrics: metrics}, metrics, err
	}

	transferStart := time.Now()
	if err := d.device.Queue().WriteBuffer(d.input, 0, float32Bytes(input)); err != nil {
		return Result{Metrics: metrics}, metrics, fmt.Errorf("%w: upload input: %v", ErrExecution, err)
	}
	if err := d.device.Queue().WriteBuffer(d.sourceValue, 0, float32Bytes(output)); err != nil {
		return Result{Metrics: metrics}, metrics, fmt.Errorf("%w: upload source output: %v", ErrExecution, err)
	}
	if err := d.device.Queue().WriteBuffer(d.weights, 0, float32Bytes(weights)); err != nil {
		return Result{Metrics: metrics}, metrics, fmt.Errorf("%w: upload weights: %v", ErrExecution, err)
	}
	metrics.Transfer = time.Since(transferStart)
	metrics.BytesUploaded = d.inputBytes + d.sourceValueBytes + d.weightBytes
	if err := ctx.Err(); err != nil {
		return Result{Metrics: metrics}, metrics, err
	}

	dispatchStart := time.Now()
	encoder, err := d.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "coimnet-sparse-drive-encoder"})
	if err != nil {
		return Result{Metrics: metrics}, metrics, fmt.Errorf("%w: create command encoder: %v", ErrExecution, err)
	}
	pass, err := encoder.BeginComputePass(&wgpu.ComputePassDescriptor{Label: "coimnet-sparse-drive-pass"})
	if err != nil {
		encoder.DiscardEncoding()
		return Result{Metrics: metrics}, metrics, fmt.Errorf("%w: begin compute pass: %v", ErrExecution, err)
	}
	pass.SetPipeline(d.pipeline)
	pass.SetBindGroup(0, d.bindGroup, nil)
	groups64 := (uint64(d.nodes) + uint64(workgroupSize) - 1) / uint64(workgroupSize)
	if groups64 > uint64(^uint32(0)) {
		_ = pass.End()
		encoder.DiscardEncoding()
		return Result{Metrics: metrics}, metrics, fmt.Errorf("%w: dispatch groups %d exceed uint32 range", ErrCapacity, groups64)
	}
	if groups64 > uint64(d.limits.MaxComputeWorkgroupsPerDimension) && d.limits.MaxComputeWorkgroupsPerDimension != 0 {
		_ = pass.End()
		encoder.DiscardEncoding()
		return Result{Metrics: metrics}, metrics, fmt.Errorf("%w: dispatch groups %d exceed device limit %d", ErrCapacity, groups64, d.limits.MaxComputeWorkgroupsPerDimension)
	}
	pass.Dispatch(uint32(groups64), 1, 1)
	if err := pass.End(); err != nil {
		encoder.DiscardEncoding()
		return Result{Metrics: metrics}, metrics, fmt.Errorf("%w: end compute pass: %v", ErrExecution, err)
	}
	encoder.CopyBufferToBuffer(d.result, 0, d.staging, 0, d.resultBytes)
	commands, err := encoder.Finish()
	if err != nil {
		return Result{Metrics: metrics}, metrics, fmt.Errorf("%w: finish commands: %v", ErrExecution, err)
	}
	// wgpu v0.30.35 takes ownership of a successfully submitted command
	// buffer and recycles it after completion. Release is required only when
	// submission fails and ownership remains with this call.
	if _, err := d.device.Queue().Submit(commands); err != nil {
		commands.Release()
		return Result{Metrics: metrics}, metrics, fmt.Errorf("%w: submit: %v", ErrExecution, err)
	}
	metrics.Dispatch = time.Since(dispatchStart)
	if err := ctx.Err(); err != nil {
		return Result{Metrics: metrics}, metrics, err
	}

	readbackStart := time.Now()
	if err := d.staging.Map(ctx, wgpu.MapModeRead, 0, d.resultBytes); err != nil {
		_ = d.staging.Unmap()
		metrics.Readback = time.Since(readbackStart)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Result{Metrics: metrics}, metrics, err
		}
		return Result{Metrics: metrics}, metrics, fmt.Errorf("%w: map readback: %v", ErrExecution, err)
	}
	rangeView, err := d.staging.MappedRange(0, d.resultBytes)
	if err != nil {
		_ = d.staging.Unmap()
		metrics.Readback = time.Since(readbackStart)
		return Result{Metrics: metrics}, metrics, fmt.Errorf("%w: read mapped readback: %v", ErrExecution, err)
	}
	raw := rangeView.Bytes()
	values := make([]float32, d.nodes)
	for i := range values {
		values[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
		if math.IsNaN(float64(values[i])) || math.IsInf(float64(values[i]), 0) {
			_ = d.staging.Unmap()
			metrics.Readback = time.Since(readbackStart)
			return Result{Metrics: metrics}, metrics, fmt.Errorf("%w: result[%d] is %v", ErrNonFinite, i, values[i])
		}
	}
	if err := d.staging.Unmap(); err != nil {
		metrics.Readback = time.Since(readbackStart)
		return Result{Metrics: metrics}, metrics, fmt.Errorf("%w: unmap readback: %v", ErrExecution, err)
	}
	metrics.Readback = time.Since(readbackStart)
	return Result{Output: values, Metrics: metrics}, metrics, nil
}

// Info returns the selected adapter identity. It remains available after
// Close so callers can retain the provenance of a completed measurement.
func (d *SparseDrive) Info() DeviceInfo {
	if d == nil {
		return DeviceInfo{}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.info
}

// Metrics returns the construction and most recent execution measurements.
func (d *SparseDrive) Metrics() Metrics {
	if d == nil {
		return Metrics{}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.metrics
}

// Close waits for queued GPU work, releases all resources, and is idempotent.
func (d *SparseDrive) Close() error {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	var waitErr error
	if d.device != nil {
		waitErr = d.device.WaitIdle()
	}
	d.releaseResources()
	if waitErr != nil {
		return fmt.Errorf("%w: wait for device idle during close: %v", ErrExecution, waitErr)
	}
	return nil
}

func (d *SparseDrive) releaseResources() {
	if d.bindGroup != nil {
		d.bindGroup.Release()
		d.bindGroup = nil
	}
	if d.pipeline != nil {
		d.pipeline.Release()
		d.pipeline = nil
	}
	if d.bindGroupLayout != nil {
		d.bindGroupLayout.Release()
		d.bindGroupLayout = nil
	}
	for _, buffer := range []*wgpu.Buffer{d.staging, d.result, d.weights, d.sourceValue, d.input, d.edgeOrder, d.sources, d.offsets} {
		if buffer != nil {
			buffer.Release()
		}
	}
	d.staging = nil
	d.result = nil
	d.weights = nil
	d.sourceValue = nil
	d.input = nil
	d.edgeOrder = nil
	d.sources = nil
	d.offsets = nil
	if d.device != nil {
		d.device.Release()
		d.device = nil
	}
	if d.adapter != nil {
		d.adapter.Release()
		d.adapter = nil
	}
	if d.instance != nil {
		d.instance.Release()
		d.instance = nil
	}
}

func uint32Bytes(values []uint32) []byte {
	if len(values) == 0 {
		return []byte{0, 0, 0, 0}
	}
	data := make([]byte, len(values)*4)
	for i, value := range values {
		binary.LittleEndian.PutUint32(data[i*4:], value)
	}
	return data
}

func float32Bytes(values []float32) []byte {
	if len(values) == 0 {
		return []byte{0, 0, 0, 0}
	}
	data := make([]byte, len(values)*4)
	for i, value := range values {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(value))
	}
	return data
}
