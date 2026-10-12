package webgpu

import (
	"context"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

//go:embed device_adam.wgsl
var deviceAdamWGSL string

type deviceAdamChunk struct {
	start  int
	count  int
	groups uint32

	state       [2]*wgpu.Buffer
	input       *wgpu.Buffer
	status      *wgpu.Buffer
	statusStage *wgpu.Buffer
	uniform     *wgpu.Buffer
	uploadStage *wgpu.Buffer

	stateBytes    uint64
	inputBytes    uint64
	uploadBytes   uint64
	uniformOffset uint64
	statusOffset  uint64
	bindGroups    [2]*wgpu.BindGroup
}

func newDeviceAdamW(ctx context.Context, parameters, first, second []float32, steps []uint32, cfg AdamWConfig) (*DeviceAdamW, error) {
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

	// A nil descriptor requests the adapter's actual limits. Do not replace
	// them with WebGPU minimums or a guessed storage-buffer size.
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
	optimizer := &DeviceAdamW{
		instance: instance,
		adapter:  adapter,
		device:   device,
		info:     info,
		limits:   device.Limits(),
		values:   len(parameters),
		cfg:      cfg,
		metrics: AdamWMetrics{
			Device: info,
			Values: len(parameters),
		},
	}
	cleanup = func() {
		if optimizer.device != nil {
			_ = optimizer.device.WaitIdle()
		}
		optimizer.releaseAdamResources()
	}

	if err := optimizer.planAdamChunks(); err != nil {
		cleanup()
		return nil, err
	}
	compileStart := time.Now()
	if err := optimizer.compileAdamPipeline(); err != nil {
		cleanup()
		return nil, err
	}
	optimizer.metrics.Compilation = time.Since(compileStart)
	if err := ctx.Err(); err != nil {
		cleanup()
		return nil, err
	}
	if err := optimizer.createAdamBuffers(); err != nil {
		cleanup()
		return nil, err
	}
	if err := optimizer.createAdamBindGroups(); err != nil {
		cleanup()
		return nil, err
	}

	initialTransferStart := time.Now()
	if err := optimizer.uploadInitialAdamState(ctx, parameters, first, second, steps); err != nil {
		cleanup()
		return nil, err
	}
	optimizer.metrics.InitialTransfer = time.Since(initialTransferStart)

	// The first hardware dispatch copies the initial live bank into the shadow
	// bank with every mask disabled. It initializes the second bank without a
	// CPU state update and provides the constructor's measured warmup phase.
	warmupGradient := make([]float32, len(parameters))
	warmupMask := make([]bool, len(parameters))
	warmupStart := time.Now()
	if _, err := optimizer.executeAdamStepLocked(ctx, warmupGradient, warmupMask, 1); err != nil {
		cleanup()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, errors.Join(fmt.Errorf("%w: warmup", ErrExecution), err)
	}
	optimizer.metrics.Warmup = time.Since(warmupStart)
	optimizer.metrics.Initialization = time.Since(initStart)
	return optimizer, nil
}

func (o *DeviceAdamW) planAdamChunks() error {
	limits := o.limits
	if limits.MaxStorageBuffersPerShaderStage != 0 && limits.MaxStorageBuffersPerShaderStage < 4 {
		return fmt.Errorf("%w: adapter exposes %d storage bindings, need 4", ErrCapacity, limits.MaxStorageBuffersPerShaderStage)
	}
	if limits.MaxUniformBuffersPerShaderStage != 0 && limits.MaxUniformBuffersPerShaderStage < 1 {
		return fmt.Errorf("%w: adapter exposes no uniform binding", ErrCapacity)
	}
	if limits.MaxComputeInvocationsPerWorkgroup != 0 && limits.MaxComputeInvocationsPerWorkgroup < uint32(deviceAdamWorkgroupSize) {
		return fmt.Errorf("%w: adapter workgroup invocation limit %d is below %d", ErrCapacity, limits.MaxComputeInvocationsPerWorkgroup, deviceAdamWorkgroupSize)
	}
	if limits.MaxComputeWorkgroupSizeX != 0 && limits.MaxComputeWorkgroupSizeX < uint32(deviceAdamWorkgroupSize) {
		return fmt.Errorf("%w: adapter workgroup x limit %d is below %d", ErrCapacity, limits.MaxComputeWorkgroupSizeX, deviceAdamWorkgroupSize)
	}
	if limits.MaxUniformBufferBindingSize != 0 && limits.MaxUniformBufferBindingSize < deviceAdamUniformBytes {
		return fmt.Errorf("%w: uniform binding limit %d is below %d", ErrCapacity, limits.MaxUniformBufferBindingSize, deviceAdamUniformBytes)
	}

	maxByState := maxAdamBufferValues(limits.MaxStorageBufferBindingSize, deviceAdamStateBytes)
	maxByInput := maxAdamBufferValues(limits.MaxStorageBufferBindingSize, deviceAdamInputBytes)
	maxByBuffer := maxAdamBufferValues(limits.MaxBufferSize, deviceAdamStateBytes)
	maxByUpload := maxAdamUploadValues(limits.MaxBufferSize)
	maxValues := maxByState
	if maxByInput < maxValues {
		maxValues = maxByInput
	}
	if maxByBuffer < maxValues {
		maxValues = maxByBuffer
	}
	if maxByUpload < maxValues {
		maxValues = maxByUpload
	}
	if maxValues > uint64(math.MaxUint32) {
		maxValues = uint64(math.MaxUint32)
	}
	maxGroups := uint64(math.MaxUint32)
	if limits.MaxComputeWorkgroupsPerDimension != 0 {
		maxGroups = uint64(limits.MaxComputeWorkgroupsPerDimension)
	}
	maxByGroups, ok := checkedAdamMul(maxGroups, deviceAdamWorkgroupSize)
	if !ok || maxByGroups < maxValues {
		maxValues = maxByGroups
	}
	if maxValues == 0 {
		return fmt.Errorf("%w: adapter cannot hold one AdamW value", ErrCapacity)
	}

	maxChunk := maxValues
	if uint64(o.values) < maxChunk {
		maxChunk = uint64(o.values)
	}
	chunkCount := (uint64(o.values) + maxChunk - 1) / maxChunk
	if chunkCount > uint64(math.MaxInt) {
		return fmt.Errorf("%w: chunk count %d exceeds host index range", ErrCapacity, chunkCount)
	}
	// Two resident state banks and one resident input buffer are accompanied
	// by one explicitly-owned upload stage per chunk. The stage repeats each
	// chunk's input bytes and adds one uniform plus one status reset.
	total, ok := checkedAdamMul(uint64(o.values), deviceAdamStateBytes*2+deviceAdamInputBytes*2)
	if !ok {
		return fmt.Errorf("%w: resident state allocation overflows", ErrCapacity)
	}
	perChunk := deviceAdamStatusBytes + deviceAdamStagingBytes + deviceAdamUniformBytes + deviceAdamUniformBytes + deviceAdamStatusBytes
	chunkBytes, ok := checkedAdamMul(chunkCount, perChunk)
	if !ok {
		return fmt.Errorf("%w: per-chunk allocation overflows", ErrCapacity)
	}
	total, ok = checkedAdamAdd(total, chunkBytes)
	if !ok {
		return fmt.Errorf("%w: total allocation overflows", ErrCapacity)
	}
	snapshotBytes, ok := checkedAdamMul(deviceAdamStateBytes, maxChunk)
	if !ok {
		return fmt.Errorf("%w: snapshot staging allocation overflows", ErrCapacity)
	}
	total, ok = checkedAdamAdd(total, snapshotBytes)
	if !ok || total > o.cfg.MaxBytes {
		return fmt.Errorf("%w: required %d bytes exceeds max bytes %d", ErrCapacity, total, o.cfg.MaxBytes)
	}
	o.maxChunk = int(maxChunk)
	o.snapshotSize = snapshotBytes
	o.allocated = total
	o.chunks = make([]deviceAdamChunk, int(chunkCount))
	o.metrics.Chunks = int(chunkCount)
	o.metrics.AllocatedBytes = total
	for i := range o.chunks {
		start := uint64(i) * maxChunk
		count := maxChunk
		if remaining := uint64(o.values) - start; remaining < count {
			count = remaining
		}
		groups := (count-1)/deviceAdamWorkgroupSize + 1
		o.chunks[i] = deviceAdamChunk{
			start:      int(start),
			count:      int(count),
			groups:     uint32(groups),
			stateBytes: count * deviceAdamStateBytes,
			inputBytes: count * deviceAdamInputBytes,
		}
		o.chunks[i].uniformOffset = o.chunks[i].inputBytes
		o.chunks[i].statusOffset = o.chunks[i].uniformOffset + deviceAdamUniformBytes
		o.chunks[i].uploadBytes = o.chunks[i].statusOffset + deviceAdamStatusBytes
	}
	return nil
}

func maxAdamBufferValues(limit, bytesPerValue uint64) uint64 {
	if limit == 0 {
		return math.MaxUint64 / bytesPerValue
	}
	return limit / bytesPerValue
}

func maxAdamUploadValues(limit uint64) uint64 {
	const uploadOverhead = deviceAdamUniformBytes + deviceAdamStatusBytes
	if limit == 0 {
		return math.MaxUint64 / deviceAdamInputBytes
	}
	if limit < uploadOverhead {
		return 0
	}
	return (limit - uploadOverhead) / deviceAdamInputBytes
}

func (o *DeviceAdamW) compileAdamPipeline() error {
	shader, err := o.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "coimnet-device-adamw-shader",
		WGSL:  deviceAdamWGSL,
	})
	if err != nil {
		return fmt.Errorf("%w: create WGSL module: %v", ErrShaderCompile, err)
	}
	defer shader.Release()

	readOnlyStorage := &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage}
	storage := &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}
	uniform := &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, MinBindingSize: deviceAdamUniformBytes}
	layout, err := o.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "coimnet-device-adamw-bindings",
		Entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: wgpu.ShaderStageCompute, Buffer: readOnlyStorage},
			{Binding: 1, Visibility: wgpu.ShaderStageCompute, Buffer: readOnlyStorage},
			{Binding: 2, Visibility: wgpu.ShaderStageCompute, Buffer: storage},
			{Binding: 3, Visibility: wgpu.ShaderStageCompute, Buffer: storage},
			{Binding: 4, Visibility: wgpu.ShaderStageCompute, Buffer: uniform},
		},
	})
	if err != nil {
		return fmt.Errorf("%w: create bind group layout: %v", ErrShaderCompile, err)
	}
	pipelineLayout, err := o.device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "coimnet-device-adamw-pipeline-layout",
		BindGroupLayouts: []*wgpu.BindGroupLayout{layout},
	})
	if err != nil {
		layout.Release()
		return fmt.Errorf("%w: create pipeline layout: %v", ErrShaderCompile, err)
	}
	pipeline, err := o.device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "coimnet-device-adamw-pipeline",
		Layout:     pipelineLayout,
		Module:     shader,
		EntryPoint: "main",
	})
	pipelineLayout.Release()
	if err != nil {
		layout.Release()
		return fmt.Errorf("%w: create compute pipeline: %v", ErrShaderCompile, err)
	}
	o.bindGroupLayout = layout
	o.pipeline = pipeline
	return nil
}

func (o *DeviceAdamW) createAdamBuffers() error {
	var err error
	o.snapshotStaging, err = o.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "coimnet-device-adamw-snapshot-staging",
		Size:  o.snapshotSize,
		Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		return fmt.Errorf("%w: snapshot staging: %v", ErrCapacity, err)
	}
	for i := range o.chunks {
		chunk := &o.chunks[i]
		for bank := range chunk.state {
			chunk.state[bank], err = o.device.CreateBuffer(&wgpu.BufferDescriptor{
				Label:            fmt.Sprintf("coimnet-device-adamw-state-%d-%d", i, bank),
				Size:             chunk.stateBytes,
				Usage:            wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst | wgpu.BufferUsageCopySrc,
				MappedAtCreation: true,
			})
			if err != nil {
				return fmt.Errorf("%w: state chunk %d bank %d: %v", ErrCapacity, i, bank, err)
			}
		}
		chunk.input, err = o.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: fmt.Sprintf("coimnet-device-adamw-input-%d", i),
			Size:  chunk.inputBytes,
			Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
		})
		if err != nil {
			return fmt.Errorf("%w: input chunk %d: %v", ErrCapacity, i, err)
		}
		chunk.status, err = o.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: fmt.Sprintf("coimnet-device-adamw-status-%d", i),
			Size:  deviceAdamStatusBytes,
			Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst | wgpu.BufferUsageCopySrc,
		})
		if err != nil {
			return fmt.Errorf("%w: status chunk %d: %v", ErrCapacity, i, err)
		}
		chunk.statusStage, err = o.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: fmt.Sprintf("coimnet-device-adamw-status-staging-%d", i),
			Size:  deviceAdamStagingBytes,
			Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst,
		})
		if err != nil {
			return fmt.Errorf("%w: status staging chunk %d: %v", ErrCapacity, i, err)
		}
		chunk.uniform, err = o.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: fmt.Sprintf("coimnet-device-adamw-uniform-%d", i),
			Size:  deviceAdamUniformBytes,
			Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
		})
		if err != nil {
			return fmt.Errorf("%w: uniform chunk %d: %v", ErrCapacity, i, err)
		}
		chunk.uploadStage, err = o.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: fmt.Sprintf("coimnet-device-adamw-upload-stage-%d", i),
			Size:  chunk.uploadBytes,
			Usage: wgpu.BufferUsageMapWrite | wgpu.BufferUsageCopySrc,
		})
		if err != nil {
			return fmt.Errorf("%w: upload stage chunk %d: %v", ErrCapacity, i, err)
		}
	}
	return nil
}

func (o *DeviceAdamW) createAdamBindGroups() error {
	for i := range o.chunks {
		chunk := &o.chunks[i]
		for bank := range chunk.bindGroups {
			other := 1 - bank
			bindGroup, err := o.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
				Label:  fmt.Sprintf("coimnet-device-adamw-bind-group-%d-%d", i, bank),
				Layout: o.bindGroupLayout,
				Entries: []wgpu.BindGroupEntry{
					{Binding: 0, Buffer: chunk.state[bank], Size: chunk.stateBytes},
					{Binding: 1, Buffer: chunk.input, Size: chunk.inputBytes},
					{Binding: 2, Buffer: chunk.state[other], Size: chunk.stateBytes},
					{Binding: 3, Buffer: chunk.status, Size: deviceAdamStatusBytes},
					{Binding: 4, Buffer: chunk.uniform, Size: deviceAdamUniformBytes},
				},
			})
			if err != nil {
				return fmt.Errorf("%w: bind group chunk %d bank %d: %v", ErrExecution, i, bank, err)
			}
			chunk.bindGroups[bank] = bindGroup
		}
	}
	return nil
}

func (o *DeviceAdamW) uploadInitialAdamState(ctx context.Context, parameters, first, second []float32, steps []uint32) error {
	for i := range o.chunks {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk := &o.chunks[i]
		data := encodeAdamState(parameters, first, second, steps, chunk.start, chunk.count)
		rangeView, err := chunk.state[0].MappedRange(0, chunk.stateBytes)
		if err != nil {
			_ = chunk.state[0].Unmap()
			_ = chunk.state[1].Unmap()
			return fmt.Errorf("%w: map initial state chunk %d: %v", ErrExecution, i, err)
		}
		copy(rangeView.BytesMut(), data)
		rangeView.Release()
		if err := chunk.state[0].Unmap(); err != nil {
			_ = chunk.state[1].Unmap()
			return fmt.Errorf("%w: unmap initial state chunk %d bank 0: %v", ErrExecution, i, err)
		}
		if err := chunk.state[1].Unmap(); err != nil {
			return fmt.Errorf("%w: unmap initial state chunk %d bank 1: %v", ErrExecution, i, err)
		}
		o.metrics.BytesUploaded += chunk.stateBytes
	}
	return ctx.Err()
}

func encodeAdamState(parameters, first, second []float32, steps []uint32, start, count int) []byte {
	data := make([]byte, count*int(deviceAdamStateBytes))
	for i := 0; i < count; i++ {
		at := i * int(deviceAdamStateBytes)
		index := start + i
		binary.LittleEndian.PutUint32(data[at:], math.Float32bits(parameters[index]))
		binary.LittleEndian.PutUint32(data[at+4:], math.Float32bits(first[index]))
		binary.LittleEndian.PutUint32(data[at+8:], math.Float32bits(second[index]))
		binary.LittleEndian.PutUint32(data[at+12:], steps[index])
	}
	return data
}

func (o *DeviceAdamW) releaseAdamResources() {
	for i := range o.chunks {
		chunk := &o.chunks[i]
		for bank := range chunk.bindGroups {
			if chunk.bindGroups[bank] != nil {
				chunk.bindGroups[bank].Release()
				chunk.bindGroups[bank] = nil
			}
		}
	}
	if o.pipeline != nil {
		o.pipeline.Release()
		o.pipeline = nil
	}
	if o.bindGroupLayout != nil {
		o.bindGroupLayout.Release()
		o.bindGroupLayout = nil
	}
	if o.snapshotStaging != nil {
		o.snapshotStaging.Release()
		o.snapshotStaging = nil
	}
	for i := range o.chunks {
		chunk := &o.chunks[i]
		for bank := range chunk.state {
			if chunk.state[bank] != nil {
				chunk.state[bank].Release()
				chunk.state[bank] = nil
			}
		}
		for _, buffer := range []*wgpu.Buffer{chunk.input, chunk.status, chunk.statusStage, chunk.uniform, chunk.uploadStage} {
			if buffer != nil {
				buffer.Release()
			}
		}
		chunk.input = nil
		chunk.status = nil
		chunk.statusStage = nil
		chunk.uniform = nil
		chunk.uploadStage = nil
	}
	o.chunks = nil
	if o.device != nil {
		o.device.Release()
		o.device = nil
	}
	if o.adapter != nil {
		o.adapter.Release()
		o.adapter = nil
	}
	if o.instance != nil {
		o.instance.Release()
		o.instance = nil
	}
}
