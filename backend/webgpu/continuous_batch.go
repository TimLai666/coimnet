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
)

// The ContinuousBatchConfig values of Activation and Gate.
const (
	ActivationTanh     = "tanh"
	ActivationSoftsign = "softsign"
	ActivationSoftplus = "softplus"
	GateNone           = ""
	GateSigmoid        = "current"
	GateSoftsign       = "current_softsign"
)

const (
	continuousBatchUniformBytes = uint64(64)
	noneIndex                   = ^uint32(0)
)

// ContinuousBatchConfig fixes the topology and the per-episode layout of a
// batched continuous core: a scalar, undelayed leaky integrator, optionally
// gated, that starts every sequence from zero voltage.
//
// Sources[e] and Targets[e] form one directed edge; duplicates are separate
// additive connections, and each target sums its edges in declared order.
// InputNodes names the node that receives each input column (nil means every
// node, column i feeding node i). ReadoutNodes names the nodes Forward reads
// back, after every step when ReadoutEveryStep is set and after the last step
// otherwise.
type ContinuousBatchConfig struct {
	Nodes            int
	Sources          []int
	Targets          []int
	Activation       string
	Gate             string
	InputNodes       []int
	ReadoutNodes     []int
	ReadoutEveryStep bool
}

// ContinuousBatchParameters are the float32 values one Forward integrates.
// Weights follow the declared edge order. Lambda is exp(-dt/tau), Alpha is
// 1-Lambda, and TauSlope is dLambda/dlog_tau = Lambda*dt/tau, all computed by
// the caller so their precision is the caller's choice. GateGain and GateBias
// are required exactly when the configuration declares a gate.
type ContinuousBatchParameters struct {
	Weights  []float32
	Bias     []float32
	Lambda   []float32
	Alpha    []float32
	TauSlope []float32
	GateGain []float32
	GateBias []float32
}

// ContinuousBatchGradient is the reverse pass of the last Forward. Weights
// (declared edge order), Bias, LogTau, GateGain and GateBias are summed over
// every sequence and step; GateGain and GateBias are nil without a gate.
// Inputs is the gradient of every input column, laid out [batch][step][width]
// like the Forward inputs.
type ContinuousBatchGradient struct {
	Weights  []float32
	Bias     []float32
	LogTau   []float32
	GateGain []float32
	GateBias []float32
	Inputs   []float32
}

// ContinuousBatchMetrics are host wall-clock measurements of construction and
// of the most recent Forward and Backward. Dispatch is the time to encode and
// submit; Readback includes waiting for the device to finish.
type ContinuousBatchMetrics struct {
	Device                       DeviceInfo
	Nodes, Edges, Width, Readout int
	Batch, Steps                 int
	Initialization, Compilation  time.Duration
	Allocation                   time.Duration
	ForwardTransfer              time.Duration
	ForwardDispatch              time.Duration
	ForwardReadback              time.Duration
	BackwardTransfer             time.Duration
	BackwardDispatch             time.Duration
	BackwardReadback             time.Duration
	// AllocatedBytes is the logical size of every episode buffer.
	AllocatedBytes, BytesUploaded, BytesReadBack uint64
}

// ContinuousBatch keeps a whole minibatch episode on the device. Forward runs
// every step of every sequence in one submission, keeps the voltage, output,
// drive and gate history resident, and reads back only the readout nodes.
// Backward then runs the full reverse pass over that resident history, reduces
// every parameter gradient on the device and reads back the sums and the input
// gradients. Calls are serialized. There is no CPU fallback: a missing
// adapter is ErrUnavailable (ErrDevice once HardwareAdapterSeen) and an
// execution failure is returned as is. Close releases every buffer, pipeline,
// layout, the device and the adapter; the package's shared instance stays.
type ContinuousBatch struct {
	mu     sync.Mutex
	closed bool

	adapter *wgpu.Adapter
	device  *wgpu.Device
	info    DeviceInfo
	limits  gputypes.Limits

	layout continuousBatchLayout
	kernel [continuousBatchKernels]continuousBatchPipeline

	topology, nodeIndex, coef, slotWeights *wgpu.Buffer

	// Episode buffers, sized for capBatch sequences of capSteps steps.
	capBatch, capSteps                             int
	episode                                        []*wgpu.Buffer
	inputs, voltage, output, history, upstream     *wgpu.Buffer
	driveGrad, partial, carry, readout, gradient   *wgpu.Buffer
	inputGrad, readoutStage, gradientStage         *wgpu.Buffer
	inputGradStage                                 *wgpu.Buffer
	uniforms                                       []*wgpu.Buffer
	forwardGroups, nodeGroups, scatterGroups       []*wgpu.BindGroup
	readoutGroup, weightGroup, reduceGroup, gather *wgpu.BindGroup
	allocated                                      uint64

	// The shape and gate of the resident forward history Backward reads;
	// zero batch means there is none.
	batch, steps int

	metrics ContinuousBatchMetrics
}

// continuousBatchLayout is the validated host-side topology and index maps.
type continuousBatchLayout struct {
	nodes, edges, width, readout int
	activation, gate             uint32
	everyStep                    bool
	// slotEdge[slot] is the declared edge stored in CSR slot slot.
	slotEdge  []int
	topology  []uint32
	nodeIndex []uint32
}

type continuousBatchPipeline struct {
	pipeline *wgpu.ComputePipeline
	// gogpu/wgpu's Vulkan pipelines keep only the raw layout handle, so a
	// pipeline layout must live until its pipeline is released.
	pipelineLayout  *wgpu.PipelineLayout
	bindGroupLayout *wgpu.BindGroupLayout
}

const (
	kernelForward = iota
	kernelReadout
	kernelNodeBackward
	kernelScatter
	kernelWeightReduce
	kernelNodeReduce
	kernelInputGather
	continuousBatchKernels
)

// continuousBatchKernelSpec lists each kernel's source and its read-only and
// read-write storage bindings, in binding order before the uniform.
var continuousBatchKernelSpec = [continuousBatchKernels]struct {
	label     string
	source    string
	readOnly  int
	readWrite int
}{
	kernelForward:      {"forward", continuousBatchForwardWGSL, 5, 3},
	kernelReadout:      {"readout", continuousBatchReadoutWGSL, 2, 1},
	kernelNodeBackward: {"node-backward", continuousBatchNodeBackwardWGSL, 5, 3},
	kernelScatter:      {"scatter", continuousBatchScatterWGSL, 3, 1},
	kernelWeightReduce: {"weight-reduce", continuousBatchWeightReduceWGSL, 3, 1},
	kernelNodeReduce:   {"node-reduce", continuousBatchNodeReduceWGSL, 2, 1},
	kernelInputGather:  {"input-gather", continuousBatchInputGatherWGSL, 2, 1},
}

// buildContinuousBatchLayout validates cfg and builds the CSR topology (by
// target, declared order within a target), its transpose (by source, declared
// order within a source) and the node index maps. It opens no device.
func buildContinuousBatchLayout(cfg ContinuousBatchConfig) (continuousBatchLayout, error) {
	var l continuousBatchLayout
	if err := validateConfig(Config{Nodes: cfg.Nodes, Sources: cfg.Sources, Targets: cfg.Targets}); err != nil {
		return l, err
	}
	switch cfg.Activation {
	case ActivationTanh:
		l.activation = 0
	case ActivationSoftsign:
		l.activation = 1
	case ActivationSoftplus:
		l.activation = 2
	default:
		return l, fmt.Errorf("%w: unsupported activation %q", ErrInvalidConfig, cfg.Activation)
	}
	switch cfg.Gate {
	case GateNone:
		l.gate = 0
	case GateSigmoid:
		l.gate = 1
	case GateSoftsign:
		l.gate = 2
	default:
		return l, fmt.Errorf("%w: unsupported gate %q", ErrInvalidConfig, cfg.Gate)
	}
	n, e := cfg.Nodes, len(cfg.Sources)
	// Every index the kernels form, up to the transpose slot table, must fit
	// a u32.
	if uint64(2*n)+uint64(4*e)+2 > maxUint32 {
		return l, fmt.Errorf("%w: %d nodes and %d edges overflow the u32 topology index", ErrInvalidConfig, n, e)
	}
	inputNodes := cfg.InputNodes
	if inputNodes == nil {
		inputNodes = make([]int, n)
		for i := range inputNodes {
			inputNodes[i] = i
		}
	}
	if len(inputNodes) == 0 {
		return l, fmt.Errorf("%w: input nodes must be nil or non-empty", ErrInvalidConfig)
	}
	if len(cfg.ReadoutNodes) == 0 {
		return l, fmt.Errorf("%w: readout nodes must be non-empty", ErrInvalidConfig)
	}
	if uint64(len(inputNodes)) > uint64(n) || uint64(len(cfg.ReadoutNodes)) > uint64(n) {
		return l, fmt.Errorf("%w: more input or readout nodes than nodes", ErrInvalidConfig)
	}
	l.nodes, l.edges, l.width, l.readout, l.everyStep = n, e, len(inputNodes), len(cfg.ReadoutNodes), cfg.ReadoutEveryStep
	l.nodeIndex = make([]uint32, 2*n+l.width+l.readout)
	for i := 0; i < 2*n; i++ {
		l.nodeIndex[i] = noneIndex
	}
	for column, node := range inputNodes {
		if node < 0 || node >= n || l.nodeIndex[node] != noneIndex {
			return l, fmt.Errorf("%w: invalid or duplicate input node %d", ErrInvalidConfig, node)
		}
		l.nodeIndex[node] = uint32(column)
		l.nodeIndex[2*n+column] = uint32(node)
	}
	for column, node := range cfg.ReadoutNodes {
		if node < 0 || node >= n || l.nodeIndex[n+node] != noneIndex {
			return l, fmt.Errorf("%w: invalid or duplicate readout node %d", ErrInvalidConfig, node)
		}
		l.nodeIndex[n+node] = uint32(column)
		l.nodeIndex[2*n+l.width+column] = uint32(node)
	}

	byTarget, err := buildTopology(Config{Nodes: n, Sources: cfg.Sources, Targets: cfg.Targets})
	if err != nil {
		return l, err
	}
	// The transpose orders slots by source with a stable counting sort over
	// the CSR slots, so a source's outgoing edges stay in declared order.
	slotTarget := make([]uint32, e)
	for target := 0; target < n; target++ {
		for slot := byTarget.offsets[target]; slot < byTarget.offsets[target+1]; slot++ {
			slotTarget[slot] = uint32(target)
		}
	}
	l.slotEdge = make([]int, e)
	for slot, edge := range byTarget.edgeOrder {
		l.slotEdge[slot] = int(edge)
	}
	transposeOffsets := make([]uint32, n+1)
	for _, source := range cfg.Sources {
		transposeOffsets[source+1]++
	}
	for j := 0; j < n; j++ {
		transposeOffsets[j+1] += transposeOffsets[j]
	}
	// Visit slots in declared edge order so each source's list is declared order.
	edgeSlot := make([]uint32, e)
	for slot, edge := range byTarget.edgeOrder {
		edgeSlot[edge] = uint32(slot)
	}
	transposeTarget := make([]uint32, e)
	transposeSlot := make([]uint32, e)
	cursor := append([]uint32(nil), transposeOffsets[:n]...)
	for edge, source := range cfg.Sources {
		k := cursor[source]
		transposeTarget[k] = uint32(cfg.Targets[edge])
		transposeSlot[k] = edgeSlot[edge]
		cursor[source]++
	}
	l.topology = make([]uint32, 0, 2*n+2+4*e)
	l.topology = append(l.topology, byTarget.offsets...)
	l.topology = append(l.topology, byTarget.sources...)
	l.topology = append(l.topology, slotTarget...)
	l.topology = append(l.topology, transposeOffsets...)
	l.topology = append(l.topology, transposeTarget...)
	l.topology = append(l.topology, transposeSlot...)
	return l, nil
}

// NewContinuousBatch validates cfg, opens a hardware adapter, compiles the
// batched kernels and uploads the immutable topology. Episode buffers are
// allocated by the first Forward of each larger shape.
func NewContinuousBatch(ctx context.Context, cfg ContinuousBatchConfig) (*ContinuousBatch, error) {
	return newContinuousBatch(ctx, cfg, false)
}

// newContinuousBatch is NewContinuousBatch that, for tests only, may accept
// the gogpu software adapter, which interprets the same SPIR-V on the CPU.
func newContinuousBatch(ctx context.Context, cfg ContinuousBatchConfig, allowSoftware bool) (*ContinuousBatch, error) {
	if ctx == nil {
		return nil, errors.New("webgpu continuous batch: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	layout, err := buildContinuousBatchLayout(cfg)
	if err != nil {
		return nil, err
	}
	initStart := time.Now()
	adapter, device, info, err := openDevice(ctx, allowSoftware)
	if err != nil {
		return nil, err
	}
	c := &ContinuousBatch{adapter: adapter, device: device, info: info, limits: device.Limits(), layout: layout}
	c.metrics = ContinuousBatchMetrics{Device: info, Nodes: layout.nodes, Edges: layout.edges, Width: layout.width, Readout: layout.readout}
	if err := c.checkStorageLimit(); err != nil {
		c.releaseAll()
		return nil, err
	}
	compileStart := time.Now()
	for k := range c.kernel {
		if err := c.compileKernel(k); err != nil {
			c.releaseAll()
			return nil, err
		}
	}
	c.metrics.Compilation = time.Since(compileStart)
	if err := ctx.Err(); err != nil {
		c.releaseAll()
		return nil, err
	}
	if err := c.createModelBuffers(); err != nil {
		c.releaseAll()
		return nil, err
	}
	c.metrics.Initialization = time.Since(initStart)
	return c, nil
}

// checkStorageLimit refuses a device whose per-stage storage binding limit is
// below the eight bindings the forward and reverse kernels use.
func (c *ContinuousBatch) checkStorageLimit() error {
	if c.limits.MaxStorageBuffersPerShaderStage != 0 && c.limits.MaxStorageBuffersPerShaderStage < 8 {
		return fmt.Errorf("%w: device allows %d storage buffers per stage, the kernels need 8", ErrCapacity, c.limits.MaxStorageBuffersPerShaderStage)
	}
	if c.limits.MaxUniformBufferBindingSize != 0 && c.limits.MaxUniformBufferBindingSize < continuousBatchUniformBytes {
		return fmt.Errorf("%w: uniform binding limit %d is below %d", ErrCapacity, c.limits.MaxUniformBufferBindingSize, continuousBatchUniformBytes)
	}
	return nil
}

func (c *ContinuousBatch) compileKernel(k int) error {
	spec := continuousBatchKernelSpec[k]
	shader, err := c.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "coimnet-continuous-batch-" + spec.label, WGSL: spec.source})
	if err != nil {
		return fmt.Errorf("%w: %s WGSL module: %v", ErrShaderCompile, spec.label, err)
	}
	defer shader.Release()
	entries := make([]gputypes.BindGroupLayoutEntry, 0, spec.readOnly+spec.readWrite+1)
	for b := 0; b < spec.readOnly+spec.readWrite; b++ {
		kind := gputypes.BufferBindingTypeReadOnlyStorage
		if b >= spec.readOnly {
			kind = gputypes.BufferBindingTypeStorage
		}
		entries = append(entries, gputypes.BindGroupLayoutEntry{Binding: uint32(b), Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: kind}})
	}
	entries = append(entries, gputypes.BindGroupLayoutEntry{Binding: uint32(spec.readOnly + spec.readWrite), Visibility: wgpu.ShaderStageCompute,
		Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, MinBindingSize: continuousBatchUniformBytes}})
	layout, err := c.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{Label: "coimnet-continuous-batch-" + spec.label + "-bindings", Entries: entries})
	if err != nil {
		return fmt.Errorf("%w: %s bind group layout: %v", ErrShaderCompile, spec.label, err)
	}
	pipelineLayout, err := c.device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "coimnet-continuous-batch-" + spec.label + "-pipeline-layout", BindGroupLayouts: []*wgpu.BindGroupLayout{layout}})
	if err != nil {
		layout.Release()
		return fmt.Errorf("%w: %s pipeline layout: %v", ErrShaderCompile, spec.label, err)
	}
	pipeline, err := c.device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{Label: "coimnet-continuous-batch-" + spec.label, Layout: pipelineLayout, Module: shader, EntryPoint: "main"})
	if err != nil {
		pipelineLayout.Release()
		layout.Release()
		return fmt.Errorf("%w: %s compute pipeline: %v", ErrShaderCompile, spec.label, err)
	}
	c.kernel[k] = continuousBatchPipeline{pipeline: pipeline, pipelineLayout: pipelineLayout, bindGroupLayout: layout}
	return nil
}

func (c *ContinuousBatch) bufferBytes(count uint64) (uint64, error) {
	if count > math.MaxUint64/4 {
		return 0, fmt.Errorf("element count %d overflows byte size", count)
	}
	size := count * 4
	if size == 0 {
		size = 4
	}
	if c.limits.MaxBufferSize != 0 && size > c.limits.MaxBufferSize {
		return 0, fmt.Errorf("%d bytes exceeds MaxBufferSize %d", size, c.limits.MaxBufferSize)
	}
	if c.limits.MaxStorageBufferBindingSize != 0 && size > c.limits.MaxStorageBufferBindingSize {
		return 0, fmt.Errorf("%d bytes exceeds MaxStorageBufferBindingSize %d", size, c.limits.MaxStorageBufferBindingSize)
	}
	return size, nil
}

func (c *ContinuousBatch) newBuffer(label string, count uint64, usage gputypes.BufferUsage) (*wgpu.Buffer, error) {
	size, err := c.bufferBytes(count)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrCapacity, label, err)
	}
	buffer, err := c.device.CreateBuffer(&wgpu.BufferDescriptor{Label: "coimnet-continuous-batch-" + label, Size: size, Usage: usage})
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrCapacity, label, err)
	}
	return buffer, nil
}

func (c *ContinuousBatch) createModelBuffers() error {
	l := c.layout
	storage := wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst
	var err error
	if c.topology, err = c.newBuffer("topology", uint64(len(l.topology)), storage); err != nil {
		return err
	}
	if c.nodeIndex, err = c.newBuffer("node-index", uint64(len(l.nodeIndex)), storage); err != nil {
		return err
	}
	if c.coef, err = c.newBuffer("coefficients", 6*uint64(l.nodes), storage); err != nil {
		return err
	}
	if c.slotWeights, err = c.newBuffer("slot-weights", uint64(l.edges), storage); err != nil {
		return err
	}
	if err := c.device.Queue().WriteBuffer(c.topology, 0, uint32Bytes(l.topology)); err != nil {
		return fmt.Errorf("%w: upload topology: %v", ErrExecution, err)
	}
	if err := c.device.Queue().WriteBuffer(c.nodeIndex, 0, uint32Bytes(l.nodeIndex)); err != nil {
		return fmt.Errorf("%w: upload node index: %v", ErrExecution, err)
	}
	return nil
}

// readSteps is how many readout rows each sequence returns.
func (l continuousBatchLayout) readSteps(steps int) int {
	if l.everyStep {
		return steps
	}
	return 1
}

// episodeCounts returns the element count of every episode buffer for a
// batch of b sequences of t steps, refusing a shape whose largest flat index
// does not fit a u32.
func (l continuousBatchLayout) episodeCounts(b, t int) (map[string]uint64, error) {
	if b <= 0 || t <= 0 {
		return nil, fmt.Errorf("%w: batch %d and steps %d must be positive", ErrShape, b, t)
	}
	bn := uint64(b) * uint64(l.nodes)
	plane := bn * uint64(t)
	counts := map[string]uint64{
		"inputs":   uint64(b) * uint64(t) * uint64(l.width),
		"voltage":  plane + bn,
		"output":   plane + bn,
		"history":  4 * plane,
		"upstream": uint64(b) * uint64(l.readSteps(t)) * uint64(l.readout),
		"drive":    plane,
		"partial":  3 * plane,
		"carry":    2 * bn,
		"readout":  uint64(b) * uint64(l.readSteps(t)) * uint64(l.readout),
		"gradient": uint64(l.edges) + 4*uint64(l.nodes),
		"inputGrad": uint64(b) * uint64(t) *
			uint64(l.width),
	}
	for name, count := range counts {
		if count > maxUint32 {
			return nil, fmt.Errorf("%w: %s needs %d elements, beyond the u32 index range", ErrCapacity, name, count)
		}
	}
	return counts, nil
}

// ensureEpisode allocates episode buffers and bind groups for b sequences of
// t steps unless the current ones are already large enough.
func (c *ContinuousBatch) ensureEpisode(b, t int) error {
	if c.episode != nil && b <= c.capBatch && t <= c.capSteps {
		return nil
	}
	counts, err := c.layout.episodeCounts(b, t)
	if err != nil {
		return err
	}
	start := time.Now()
	if c.device != nil && c.episode != nil {
		if err := c.device.WaitIdle(); err != nil {
			return fmt.Errorf("%w: wait before reallocation: %v", ErrExecution, err)
		}
	}
	c.releaseEpisode()
	storage := wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst
	readable := wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc
	staging := wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst
	specs := []struct {
		dst   **wgpu.Buffer
		label string
		count uint64
		usage gputypes.BufferUsage
	}{
		{&c.inputs, "inputs", counts["inputs"], storage},
		{&c.voltage, "voltage", counts["voltage"], storage},
		{&c.output, "output", counts["output"], storage},
		{&c.history, "history", counts["history"], storage},
		{&c.upstream, "upstream", counts["upstream"], storage},
		{&c.driveGrad, "drive-gradient", counts["drive"], storage},
		{&c.partial, "partial", counts["partial"], storage},
		{&c.carry, "carry", counts["carry"], storage},
		{&c.readout, "readout", counts["readout"], readable},
		{&c.gradient, "gradient", counts["gradient"], readable},
		{&c.inputGrad, "input-gradient", counts["inputGrad"], readable},
		{&c.readoutStage, "readout-readback", counts["readout"], staging},
		{&c.gradientStage, "gradient-readback", counts["gradient"], staging},
		{&c.inputGradStage, "input-gradient-readback", counts["inputGrad"], staging},
	}
	var allocated uint64
	for _, spec := range specs {
		buffer, err := c.newBuffer(spec.label, spec.count, spec.usage)
		if err != nil {
			c.releaseEpisode()
			return err
		}
		*spec.dst = buffer
		c.episode = append(c.episode, buffer)
		allocated += buffer.Size()
	}
	c.uniforms = make([]*wgpu.Buffer, t)
	for s := range c.uniforms {
		buffer, err := c.device.CreateBuffer(&wgpu.BufferDescriptor{Label: "coimnet-continuous-batch-step", Size: continuousBatchUniformBytes, Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst})
		if err != nil {
			c.releaseEpisode()
			return fmt.Errorf("%w: step uniform: %v", ErrCapacity, err)
		}
		c.uniforms[s] = buffer
		allocated += continuousBatchUniformBytes
	}
	if err := c.createBindGroups(t); err != nil {
		c.releaseEpisode()
		return err
	}
	c.capBatch, c.capSteps, c.allocated = b, t, allocated
	c.metrics.Allocation = time.Since(start)
	c.metrics.AllocatedBytes = allocated
	return nil
}

func (c *ContinuousBatch) bindGroup(k int, uniform *wgpu.Buffer, buffers ...*wgpu.Buffer) (*wgpu.BindGroup, error) {
	entries := make([]wgpu.BindGroupEntry, 0, len(buffers)+1)
	for i, buffer := range buffers {
		entries = append(entries, wgpu.BindGroupEntry{Binding: uint32(i), Buffer: buffer, Size: buffer.Size()})
	}
	entries = append(entries, wgpu.BindGroupEntry{Binding: uint32(len(buffers)), Buffer: uniform, Size: continuousBatchUniformBytes})
	group, err := c.device.CreateBindGroup(&wgpu.BindGroupDescriptor{Label: "coimnet-continuous-batch-" + continuousBatchKernelSpec[k].label, Layout: c.kernel[k].bindGroupLayout, Entries: entries})
	if err != nil {
		return nil, fmt.Errorf("%w: %s bind group: %v", ErrExecution, continuousBatchKernelSpec[k].label, err)
	}
	return group, nil
}

func (c *ContinuousBatch) createBindGroups(t int) error {
	c.forwardGroups = make([]*wgpu.BindGroup, t)
	c.nodeGroups = make([]*wgpu.BindGroup, t)
	c.scatterGroups = make([]*wgpu.BindGroup, t)
	var err error
	for s := 0; s < t; s++ {
		u := c.uniforms[s]
		if c.forwardGroups[s], err = c.bindGroup(kernelForward, u, c.inputs, c.nodeIndex, c.coef, c.topology, c.slotWeights, c.voltage, c.output, c.history); err != nil {
			return err
		}
		if c.nodeGroups[s], err = c.bindGroup(kernelNodeBackward, u, c.nodeIndex, c.coef, c.upstream, c.voltage, c.history, c.driveGrad, c.partial, c.carry); err != nil {
			return err
		}
		if c.scatterGroups[s], err = c.bindGroup(kernelScatter, u, c.topology, c.slotWeights, c.driveGrad, c.carry); err != nil {
			return err
		}
	}
	u := c.uniforms[0]
	if c.readoutGroup, err = c.bindGroup(kernelReadout, u, c.nodeIndex, c.output, c.readout); err != nil {
		return err
	}
	if c.weightGroup, err = c.bindGroup(kernelWeightReduce, u, c.topology, c.driveGrad, c.output, c.gradient); err != nil {
		return err
	}
	if c.reduceGroup, err = c.bindGroup(kernelNodeReduce, u, c.driveGrad, c.partial, c.gradient); err != nil {
		return err
	}
	if c.gather, err = c.bindGroup(kernelInputGather, u, c.nodeIndex, c.driveGrad, c.inputGrad); err != nil {
		return err
	}
	return nil
}

// encodeDims is the Dims uniform of step s.
func (l continuousBatchLayout) encodeDims(s, b, t int) []byte {
	values := []uint32{uint32(s), uint32(b), uint32(l.nodes), uint32(t), uint32(l.width), uint32(l.readout), uint32(l.readSteps(t)), uint32(l.edges), l.activation, l.gate, 0}
	if l.everyStep {
		values[10] = 1
	}
	data := make([]byte, continuousBatchUniformBytes)
	for i, v := range values {
		binary.LittleEndian.PutUint32(data[4*i:], v)
	}
	return data
}

// dispatchGroups splits count invocations of 64 into a two-dimensional
// launch within the per-dimension workgroup limit.
func dispatchGroups(count uint64, limit uint32) (uint32, uint32, error) {
	if limit == 0 {
		limit = 65535
	}
	groups := (count + 63) / 64
	if groups == 0 {
		groups = 1
	}
	x := min(groups, uint64(limit))
	y := (groups + x - 1) / x
	if y > uint64(limit) {
		return 0, 0, fmt.Errorf("%w: %d invocations exceed the %d x %d workgroup grid", ErrCapacity, count, limit, limit)
	}
	return uint32(x), uint32(y), nil
}

func (l continuousBatchLayout) validateParameters(p ContinuousBatchParameters) error {
	checks := []struct {
		name   string
		values []float32
		want   int
	}{
		{"weights", p.Weights, l.edges}, {"bias", p.Bias, l.nodes}, {"lambda", p.Lambda, l.nodes},
		{"alpha", p.Alpha, l.nodes}, {"tau slope", p.TauSlope, l.nodes},
	}
	gates := 0
	if l.gate != 0 {
		gates = l.nodes
	}
	checks = append(checks, struct {
		name   string
		values []float32
		want   int
	}{"gate gain", p.GateGain, gates}, struct {
		name   string
		values []float32
		want   int
	}{"gate bias", p.GateBias, gates})
	for _, check := range checks {
		if err := validateValues(check.values, check.want, check.name); err != nil {
			return err
		}
	}
	return nil
}

// Forward runs batch sequences of steps steps from zero voltage. inputs is
// [batch][step][width] row-major, width being the input node count. It
// returns the readout nodes as [batch][readout step][readout] and keeps the
// episode resident for Backward.
func (c *ContinuousBatch) Forward(ctx context.Context, p ContinuousBatchParameters, batch, steps int, inputs []float32) ([]float32, error) {
	if ctx == nil {
		return nil, errors.New("webgpu continuous batch: nil context")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l := c.layout
	if err := l.validateParameters(p); err != nil {
		return nil, err
	}
	counts, err := l.episodeCounts(batch, steps)
	if err != nil {
		return nil, err
	}
	if err := validateValues(inputs, int(counts["inputs"]), "inputs"); err != nil {
		return nil, err
	}
	// A failed Forward leaves no history for Backward.
	c.batch, c.steps = 0, 0
	if err := c.ensureEpisode(batch, steps); err != nil {
		return nil, err
	}
	transferStart := time.Now()
	queue := c.device.Queue()
	coef := make([]float32, 0, 6*l.nodes)
	coef = append(coef, p.Bias...)
	coef = append(coef, p.Lambda...)
	coef = append(coef, p.Alpha...)
	coef = append(coef, p.TauSlope...)
	if l.gate != 0 {
		coef = append(coef, p.GateGain...)
		coef = append(coef, p.GateBias...)
	} else {
		coef = append(coef, make([]float32, 2*l.nodes)...)
	}
	slotWeights := make([]float32, l.edges)
	for slot, edge := range l.slotEdge {
		slotWeights[slot] = p.Weights[edge]
	}
	bn := batch * l.nodes
	initialOutput := make([]float32, bn)
	if l.activation == 2 {
		// softplus(0) = log 2; tanh and softsign are zero at zero.
		for i := range initialOutput {
			initialOutput[i] = float32(math.Ln2)
		}
	}
	writes := []struct {
		name   string
		buffer *wgpu.Buffer
		data   []byte
	}{
		{"coefficients", c.coef, float32Bytes(coef)},
		{"weights", c.slotWeights, float32Bytes(slotWeights)},
		{"inputs", c.inputs, float32Bytes(inputs)},
		{"initial voltage", c.voltage, float32Bytes(make([]float32, bn))},
		{"initial output", c.output, float32Bytes(initialOutput)},
	}
	var uploaded uint64
	for s := 0; s < steps; s++ {
		writes = append(writes, struct {
			name   string
			buffer *wgpu.Buffer
			data   []byte
		}{"step uniform", c.uniforms[s], l.encodeDims(s, batch, steps)})
	}
	for _, w := range writes {
		if err := queue.WriteBuffer(w.buffer, 0, w.data); err != nil {
			return nil, fmt.Errorf("%w: upload %s: %v", ErrExecution, w.name, err)
		}
		uploaded += uint64(len(w.data))
	}
	c.metrics.ForwardTransfer = time.Since(transferStart)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	dispatchStart := time.Now()
	limit := c.limits.MaxComputeWorkgroupsPerDimension
	nodeX, nodeY, err := dispatchGroups(uint64(bn), limit)
	if err != nil {
		return nil, err
	}
	readX, readY, err := dispatchGroups(counts["readout"], limit)
	if err != nil {
		return nil, err
	}
	readBytes := counts["readout"] * 4
	err = c.submit("forward", func(pass *wgpu.ComputePassEncoder) {
		pass.SetPipeline(c.kernel[kernelForward].pipeline)
		for s := 0; s < steps; s++ {
			pass.SetBindGroup(0, c.forwardGroups[s], nil)
			pass.Dispatch(nodeX, nodeY, 1)
		}
		pass.SetPipeline(c.kernel[kernelReadout].pipeline)
		pass.SetBindGroup(0, c.readoutGroup, nil)
		pass.Dispatch(readX, readY, 1)
	}, func(encoder *wgpu.CommandEncoder) {
		encoder.CopyBufferToBuffer(c.readout, 0, c.readoutStage, 0, readBytes)
	})
	c.metrics.ForwardDispatch = time.Since(dispatchStart)
	if err != nil {
		return nil, err
	}
	readbackStart := time.Now()
	values, err := c.readFloats(ctx, c.readoutStage, int(counts["readout"]), "readout")
	c.metrics.ForwardReadback = time.Since(readbackStart)
	if err != nil {
		return nil, err
	}
	c.batch, c.steps = batch, steps
	c.metrics.Batch, c.metrics.Steps = batch, steps
	c.metrics.BytesUploaded, c.metrics.BytesReadBack = uploaded, readBytes
	return values, nil
}

// Backward runs the full reverse pass over the episode of the last Forward
// with the readout gradient upstream, laid out like Forward's result. The
// resident episode stays valid, so Backward may be repeated with another
// upstream.
func (c *ContinuousBatch) Backward(ctx context.Context, upstream []float32) (ContinuousBatchGradient, error) {
	var empty ContinuousBatchGradient
	if ctx == nil {
		return empty, errors.New("webgpu continuous batch: nil context")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return empty, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if c.batch == 0 {
		return empty, fmt.Errorf("%w: Backward needs a successful Forward first", ErrShape)
	}
	l, batch, steps := c.layout, c.batch, c.steps
	counts, err := l.episodeCounts(batch, steps)
	if err != nil {
		return empty, err
	}
	if err := validateValues(upstream, int(counts["upstream"]), "upstream"); err != nil {
		return empty, err
	}
	transferStart := time.Now()
	queue := c.device.Queue()
	if err := queue.WriteBuffer(c.upstream, 0, float32Bytes(upstream)); err != nil {
		return empty, fmt.Errorf("%w: upload upstream: %v", ErrExecution, err)
	}
	if err := queue.WriteBuffer(c.carry, 0, float32Bytes(make([]float32, counts["carry"]))); err != nil {
		return empty, fmt.Errorf("%w: clear carry: %v", ErrExecution, err)
	}
	c.metrics.BackwardTransfer = time.Since(transferStart)
	uploaded := 4 * (counts["upstream"] + counts["carry"])
	if err := ctx.Err(); err != nil {
		return empty, err
	}

	dispatchStart := time.Now()
	limit := c.limits.MaxComputeWorkgroupsPerDimension
	bn := uint64(batch) * uint64(l.nodes)
	nodeX, nodeY, err := dispatchGroups(bn, limit)
	if err != nil {
		return empty, err
	}
	edgeX, edgeY, err := dispatchGroups(uint64(l.edges), limit)
	if err != nil {
		return empty, err
	}
	reduceX, reduceY, err := dispatchGroups(uint64(l.nodes), limit)
	if err != nil {
		return empty, err
	}
	gatherX, gatherY, err := dispatchGroups(counts["inputGrad"], limit)
	if err != nil {
		return empty, err
	}
	gradientBytes, inputBytes := counts["gradient"]*4, counts["inputGrad"]*4
	err = c.submit("backward", func(pass *wgpu.ComputePassEncoder) {
		for s := steps - 1; s >= 0; s-- {
			pass.SetPipeline(c.kernel[kernelNodeBackward].pipeline)
			pass.SetBindGroup(0, c.nodeGroups[s], nil)
			pass.Dispatch(nodeX, nodeY, 1)
			// The initial output is a constant, so step zero sends nothing back.
			if s > 0 {
				pass.SetPipeline(c.kernel[kernelScatter].pipeline)
				pass.SetBindGroup(0, c.scatterGroups[s], nil)
				pass.Dispatch(nodeX, nodeY, 1)
			}
		}
		pass.SetPipeline(c.kernel[kernelWeightReduce].pipeline)
		pass.SetBindGroup(0, c.weightGroup, nil)
		pass.Dispatch(edgeX, edgeY, 1)
		pass.SetPipeline(c.kernel[kernelNodeReduce].pipeline)
		pass.SetBindGroup(0, c.reduceGroup, nil)
		pass.Dispatch(reduceX, reduceY, 1)
		pass.SetPipeline(c.kernel[kernelInputGather].pipeline)
		pass.SetBindGroup(0, c.gather, nil)
		pass.Dispatch(gatherX, gatherY, 1)
	}, func(encoder *wgpu.CommandEncoder) {
		encoder.CopyBufferToBuffer(c.gradient, 0, c.gradientStage, 0, gradientBytes)
		encoder.CopyBufferToBuffer(c.inputGrad, 0, c.inputGradStage, 0, inputBytes)
	})
	c.metrics.BackwardDispatch = time.Since(dispatchStart)
	if err != nil {
		return empty, err
	}
	readbackStart := time.Now()
	sums, err := c.readFloats(ctx, c.gradientStage, int(counts["gradient"]), "gradient")
	if err != nil {
		c.metrics.BackwardReadback = time.Since(readbackStart)
		return empty, err
	}
	inputGrad, err := c.readFloats(ctx, c.inputGradStage, int(counts["inputGrad"]), "input gradient")
	c.metrics.BackwardReadback = time.Since(readbackStart)
	if err != nil {
		return empty, err
	}
	c.metrics.BytesUploaded, c.metrics.BytesReadBack = uploaded, gradientBytes+inputBytes
	return l.splitGradient(sums, inputGrad), nil
}

// splitGradient restores declared edge order and splits the node sums.
func (l continuousBatchLayout) splitGradient(sums, inputs []float32) ContinuousBatchGradient {
	e, n := l.edges, l.nodes
	g := ContinuousBatchGradient{Weights: make([]float32, e), Inputs: inputs}
	for slot, edge := range l.slotEdge {
		g.Weights[edge] = sums[slot]
	}
	g.Bias = append([]float32(nil), sums[e:e+n]...)
	g.LogTau = append([]float32(nil), sums[e+n:e+2*n]...)
	if l.gate != 0 {
		g.GateGain = append([]float32(nil), sums[e+2*n:e+3*n]...)
		g.GateBias = append([]float32(nil), sums[e+3*n:e+4*n]...)
	}
	return g
}

// submit encodes one compute pass and the copies after it, and submits them.
func (c *ContinuousBatch) submit(label string, record func(*wgpu.ComputePassEncoder), copies func(*wgpu.CommandEncoder)) error {
	encoder, err := c.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "coimnet-continuous-batch-" + label})
	if err != nil {
		return fmt.Errorf("%w: %s command encoder: %v", ErrExecution, label, err)
	}
	pass, err := encoder.BeginComputePass(&wgpu.ComputePassDescriptor{Label: "coimnet-continuous-batch-" + label})
	if err != nil {
		encoder.DiscardEncoding()
		return fmt.Errorf("%w: %s compute pass: %v", ErrExecution, label, err)
	}
	record(pass)
	if err := pass.End(); err != nil {
		encoder.DiscardEncoding()
		return fmt.Errorf("%w: end %s pass: %v", ErrExecution, label, err)
	}
	copies(encoder)
	commands, err := encoder.Finish()
	if err != nil {
		return fmt.Errorf("%w: finish %s commands: %v", ErrExecution, label, err)
	}
	// A successfully submitted command buffer is owned and recycled by wgpu;
	// release it only when submission fails.
	if _, err := c.device.Queue().Submit(commands); err != nil {
		commands.Release()
		return fmt.Errorf("%w: submit %s: %v", ErrExecution, label, err)
	}
	return nil
}

// readFloats maps a staging buffer, copies count float32 values and refuses
// a non-finite one.
func (c *ContinuousBatch) readFloats(ctx context.Context, staging *wgpu.Buffer, count int, name string) ([]float32, error) {
	size := uint64(count) * 4
	if err := staging.Map(ctx, wgpu.MapModeRead, 0, size); err != nil {
		_ = staging.Unmap()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: map %s readback: %v", ErrExecution, name, err)
	}
	view, err := staging.MappedRange(0, size)
	if err != nil {
		_ = staging.Unmap()
		return nil, fmt.Errorf("%w: read %s readback: %v", ErrExecution, name, err)
	}
	raw := view.Bytes()
	values := make([]float32, count)
	for i := range values {
		values[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[4*i:]))
	}
	if err := staging.Unmap(); err != nil {
		return nil, fmt.Errorf("%w: unmap %s readback: %v", ErrExecution, name, err)
	}
	for i, v := range values {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return nil, fmt.Errorf("%w: %s[%d] is %v", ErrNonFinite, name, i, v)
		}
	}
	return values, nil
}

// Info returns the selected adapter identity, also after Close.
func (c *ContinuousBatch) Info() DeviceInfo {
	if c == nil {
		return DeviceInfo{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.info
}

// Metrics returns the construction and most recent call measurements.
func (c *ContinuousBatch) Metrics() ContinuousBatchMetrics {
	if c == nil {
		return ContinuousBatchMetrics{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.metrics
}

// Close waits for queued work, releases every resource and is idempotent.
func (c *ContinuousBatch) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	var waitErr error
	if c.device != nil {
		waitErr = c.device.WaitIdle()
	}
	c.releaseAll()
	if waitErr != nil {
		return fmt.Errorf("%w: wait for device idle during close: %v", ErrExecution, waitErr)
	}
	return nil
}

func (c *ContinuousBatch) releaseEpisode() {
	for _, groups := range [][]*wgpu.BindGroup{c.forwardGroups, c.nodeGroups, c.scatterGroups, {c.readoutGroup, c.weightGroup, c.reduceGroup, c.gather}} {
		for _, group := range groups {
			if group != nil {
				group.Release()
			}
		}
	}
	c.forwardGroups, c.nodeGroups, c.scatterGroups = nil, nil, nil
	c.readoutGroup, c.weightGroup, c.reduceGroup, c.gather = nil, nil, nil, nil
	for _, buffer := range c.uniforms {
		if buffer != nil {
			buffer.Release()
		}
	}
	for _, buffer := range c.episode {
		buffer.Release()
	}
	c.uniforms, c.episode = nil, nil
	c.inputs, c.voltage, c.output, c.history, c.upstream = nil, nil, nil, nil, nil
	c.driveGrad, c.partial, c.carry, c.readout, c.gradient = nil, nil, nil, nil, nil
	c.inputGrad, c.readoutStage, c.gradientStage, c.inputGradStage = nil, nil, nil, nil
	c.capBatch, c.capSteps, c.allocated, c.batch, c.steps = 0, 0, 0, 0, 0
}

func (c *ContinuousBatch) releaseAll() {
	c.releaseEpisode()
	// Each pipeline is released before the layouts it was built with.
	for k := range c.kernel {
		if c.kernel[k].pipeline != nil {
			c.kernel[k].pipeline.Release()
		}
		if c.kernel[k].pipelineLayout != nil {
			c.kernel[k].pipelineLayout.Release()
		}
		if c.kernel[k].bindGroupLayout != nil {
			c.kernel[k].bindGroupLayout.Release()
		}
		c.kernel[k] = continuousBatchPipeline{}
	}
	for _, buffer := range []*wgpu.Buffer{c.topology, c.nodeIndex, c.coef, c.slotWeights} {
		if buffer != nil {
			buffer.Release()
		}
	}
	c.topology, c.nodeIndex, c.coef, c.slotWeights = nil, nil, nil, nil
	if c.device != nil {
		c.device.Release()
		c.device = nil
	}
	if c.adapter != nil {
		c.adapter.Release()
		c.adapter = nil
	}
}
