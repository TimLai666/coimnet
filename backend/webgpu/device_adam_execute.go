package webgpu

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"runtime"
	"time"

	"github.com/gogpu/wgpu"
)

func (o *DeviceAdamW) executeAdamStepLocked(ctx context.Context, gradient []float32, mask []bool, learningRate float32) (metrics AdamWMetrics, err error) {
	shadowBank, metrics, err := o.executeAdamCandidateLocked(ctx, gradient, mask, learningRate)
	if err == nil {
		// Legacy Step commits immediately after the per-chunk device status
		// checks. StepChecked performs the additional host candidate readback.
		o.currentBank = shadowBank
	}
	o.metrics = metrics
	return metrics, err
}

// executeAdamCandidateLocked uploads inputs and dispatches every chunk into
// the bank opposite currentBank. It reads every per-chunk status but leaves
// currentBank unchanged so callers can validate the complete candidate before
// committing it. The caller must hold o.mu.
func (o *DeviceAdamW) executeAdamCandidateLocked(ctx context.Context, gradient []float32, mask []bool, learningRate float32) (shadowBank int, metrics AdamWMetrics, err error) {
	metrics = o.metrics
	metrics.Transfer = 0
	metrics.Dispatch = 0
	metrics.Readback = 0
	shadowBank = 1 - o.currentBank
	submitted := false
	defer func() {
		if err != nil && submitted {
			err = o.drainAdamSubmission(err)
		}
	}()

	if err := ctx.Err(); err != nil {
		return shadowBank, metrics, err
	}
	transferStart := time.Now()
	for i := range o.chunks {
		if err := ctx.Err(); err != nil {
			return shadowBank, metrics, err
		}
		chunk := &o.chunks[i]
		input := encodeAdamInput(gradient, mask, chunk.start, chunk.count)
		uniform := encodeAdamUniform(o.cfg, learningRate, chunk.count)
		if err := o.writeAdamUpload(ctx, chunk, input, uniform); err != nil {
			return shadowBank, metrics, errors.Join(fmt.Errorf("%w: upload chunk %d", ErrExecution, i), err)
		}
		metrics.BytesUploaded += chunk.inputBytes + deviceAdamUniformBytes + deviceAdamStatusBytes
	}
	if err := ctx.Err(); err != nil {
		return shadowBank, metrics, err
	}
	// wgpu tracks copy destinations for the entire command buffer. Use a
	// separate ordered submission for uploads so resetting the status flag
	// cannot conflict with copying that same flag back after the dispatch.
	uploadEncoder, err := o.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "coimnet-device-adamw-upload-encoder"})
	if err != nil {
		return shadowBank, metrics, fmt.Errorf("%w: create upload encoder: %v", ErrExecution, err)
	}
	for i := range o.chunks {
		chunk := &o.chunks[i]
		uploadEncoder.CopyBufferToBuffer(chunk.uploadStage, 0, chunk.input, 0, chunk.inputBytes)
		uploadEncoder.CopyBufferToBuffer(chunk.uploadStage, chunk.uniformOffset, chunk.uniform, 0, deviceAdamUniformBytes)
		uploadEncoder.CopyBufferToBuffer(chunk.uploadStage, chunk.statusOffset, chunk.status, 0, deviceAdamStatusBytes)
	}
	uploadCommands, err := uploadEncoder.Finish()
	if err != nil {
		uploadEncoder.DiscardEncoding()
		return shadowBank, metrics, fmt.Errorf("%w: finish upload commands: %v", ErrExecution, err)
	}
	if err := ctx.Err(); err != nil {
		uploadCommands.Release()
		return shadowBank, metrics, err
	}
	submitted = true
	if _, err := o.device.Queue().Submit(uploadCommands); err != nil {
		uploadCommands.Release()
		return shadowBank, metrics, fmt.Errorf("%w: submit uploads: %v", ErrExecution, err)
	}
	metrics.Transfer = time.Since(transferStart)
	if err := ctx.Err(); err != nil {
		return shadowBank, metrics, err
	}

	dispatchStart := time.Now()
	encoder, err := o.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "coimnet-device-adamw-step-encoder"})
	if err != nil {
		return shadowBank, metrics, fmt.Errorf("%w: create command encoder: %v", ErrExecution, err)
	}
	pass, err := encoder.BeginComputePass(&wgpu.ComputePassDescriptor{Label: "coimnet-device-adamw-step-pass"})
	if err != nil {
		encoder.DiscardEncoding()
		return shadowBank, metrics, fmt.Errorf("%w: begin compute pass: %v", ErrExecution, err)
	}
	pass.SetPipeline(o.pipeline)
	for i := range o.chunks {
		chunk := &o.chunks[i]
		if uint64(chunk.groups) > uint64(o.limits.MaxComputeWorkgroupsPerDimension) && o.limits.MaxComputeWorkgroupsPerDimension != 0 {
			_ = pass.End()
			encoder.DiscardEncoding()
			return shadowBank, metrics, fmt.Errorf("%w: chunk %d dispatch groups %d exceed device limit %d", ErrCapacity, i, chunk.groups, o.limits.MaxComputeWorkgroupsPerDimension)
		}
		pass.SetBindGroup(0, chunk.bindGroups[o.currentBank], nil)
		pass.Dispatch(chunk.groups, 1, 1)
	}
	if err := pass.End(); err != nil {
		encoder.DiscardEncoding()
		return shadowBank, metrics, fmt.Errorf("%w: end compute pass: %v", ErrExecution, err)
	}
	for i := range o.chunks {
		chunk := &o.chunks[i]
		encoder.CopyBufferToBuffer(chunk.status, 0, chunk.statusStage, 0, deviceAdamStatusBytes)
	}
	commands, err := encoder.Finish()
	if err != nil {
		encoder.DiscardEncoding()
		return shadowBank, metrics, fmt.Errorf("%w: finish commands: %v", ErrExecution, err)
	}
	if err := ctx.Err(); err != nil {
		commands.Release()
		return shadowBank, metrics, err
	}
	if _, err := o.device.Queue().Submit(commands); err != nil {
		commands.Release()
		return shadowBank, metrics, fmt.Errorf("%w: submit: %v", ErrExecution, err)
	}
	metrics.Dispatch = time.Since(dispatchStart)
	if err := ctx.Err(); err != nil {
		return shadowBank, metrics, err
	}

	readbackStart := time.Now()
	var statusErr error
	for i := range o.chunks {
		if err := ctx.Err(); err != nil {
			return shadowBank, metrics, err
		}
		chunk := &o.chunks[i]
		status, err := o.readAdamStatus(ctx, chunk.statusStage)
		if err != nil {
			return shadowBank, metrics, errors.Join(fmt.Errorf("%w: read status chunk %d", ErrExecution, i), err)
		}
		metrics.BytesReadBack += deviceAdamStagingBytes
		if statusErr == nil && status != 0 {
			statusErr = deviceAdamStatusError(status, i)
		}
	}
	metrics.Readback = time.Since(readbackStart)
	if statusErr != nil {
		return shadowBank, metrics, statusErr
	}
	if err := ctx.Err(); err != nil {
		return shadowBank, metrics, err
	}
	return shadowBank, metrics, nil
}

var zeroAdamStatus = []byte{0, 0, 0, 0}

func encodeAdamInput(gradient []float32, mask []bool, start, count int) []byte {
	data := make([]byte, count*int(deviceAdamInputBytes))
	for i := 0; i < count; i++ {
		at := i * int(deviceAdamInputBytes)
		index := start + i
		binary.LittleEndian.PutUint32(data[at:], math.Float32bits(gradient[index]))
		if mask[index] {
			binary.LittleEndian.PutUint32(data[at+4:], 1)
		}
	}
	return data
}

// mapAdamBuffer owns polling and drains a cancelled map before buffer reuse.
func (o *DeviceAdamW) mapAdamBuffer(ctx context.Context, buffer *wgpu.Buffer, mode wgpu.MapMode, size uint64) error {
	if ctx == nil {
		return fmt.Errorf("%w: nil map context", ErrInvalidConfig)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if o == nil || o.device == nil {
		return fmt.Errorf("%w: device is unavailable for mapping", ErrExecution)
	}
	if buffer == nil {
		return fmt.Errorf("%w: nil buffer for mapping", ErrExecution)
	}
	pending, err := buffer.MapAsync(mode, 0, size)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return errors.Join(ctxErr, fmt.Errorf("%w: map async: %v", ErrExecution, err))
		}
		return fmt.Errorf("%w: map async: %v", ErrExecution, err)
	}
	for {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return o.cancelAdamMap(ctxErr, buffer, pending)
		}
		o.device.Poll(wgpu.PollPoll)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return o.cancelAdamMap(ctxErr, buffer, pending)
		}
		ready, mapErr := pending.Status()
		if ready {
			if mapErr != nil {
				return o.cancelAdamMap(mapErr, buffer, pending)
			}
			pending.Release()
			return nil
		}
		runtime.Gosched()
	}
}

func (o *DeviceAdamW) cancelAdamMap(cause error, buffer *wgpu.Buffer, pending *wgpu.MapPending) error {
	var cleanupErrs []error
	if buffer != nil {
		if err := buffer.Unmap(); err != nil && !errors.Is(err, wgpu.ErrMapNotMapped) {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("unmap cancelled buffer: %w", err))
		}
	}
	if o == nil || o.device == nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("device unavailable while draining cancelled map"))
	} else {
		if err := o.device.WaitIdle(); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("wait for cancelled map: %w", err))
		}
		// WaitIdle drains GPU work, while PollPoll drains the pending map
		// bucket before the handle is returned to its pool.
		o.device.Poll(wgpu.PollPoll)
	}
	if pending != nil {
		pending.Release()
	}
	if len(cleanupErrs) == 0 {
		return cause
	}
	cleanupErr := errors.Join(cleanupErrs...)
	combined := errors.Join(cause, fmt.Errorf("%w: map cleanup failed", ErrExecution), cleanupErr)
	if o != nil {
		o.markAdamUnusable(combined)
	}
	return combined
}

func (o *DeviceAdamW) markAdamUnusable(cause error) {
	if o == nil || cause == nil || o.unusable != nil {
		return
	}
	o.unusable = errors.Join(fmt.Errorf("%w: device cannot be safely reused", ErrExecution), cause)
}

func (o *DeviceAdamW) drainAdamSubmission(cause error) error {
	if o == nil || o.device == nil {
		if cause != nil {
			return cause
		}
		return fmt.Errorf("%w: device unavailable while draining submission", ErrExecution)
	}
	waitErr := o.device.WaitIdle()
	o.device.Poll(wgpu.PollPoll)
	if waitErr == nil {
		return cause
	}
	combined := errors.Join(cause, fmt.Errorf("%w: wait for submitted work: %v", ErrExecution, waitErr))
	o.markAdamUnusable(combined)
	return combined
}

func (o *DeviceAdamW) unmapAdamBuffer(buffer *wgpu.Buffer) error {
	if buffer == nil {
		return fmt.Errorf("%w: nil buffer", ErrExecution)
	}
	if err := buffer.Unmap(); err != nil && !errors.Is(err, wgpu.ErrMapNotMapped) {
		o.markAdamUnusable(err)
		return err
	}
	return nil
}

func (o *DeviceAdamW) writeAdamUpload(ctx context.Context, chunk *deviceAdamChunk, input, uniform []byte) error {
	if err := o.mapAdamBuffer(ctx, chunk.uploadStage, wgpu.MapModeWrite, chunk.uploadBytes); err != nil {
		return err
	}
	rangeView, err := chunk.uploadStage.MappedRange(0, chunk.uploadBytes)
	if err != nil {
		return errors.Join(err, o.unmapAdamBuffer(chunk.uploadStage))
	}
	raw := rangeView.BytesMut()
	copy(raw[:int(chunk.inputBytes)], input)
	copy(raw[int(chunk.uniformOffset):int(chunk.uniformOffset+deviceAdamUniformBytes)], uniform)
	copy(raw[int(chunk.statusOffset):int(chunk.statusOffset+deviceAdamStatusBytes)], zeroAdamStatus)
	rangeView.Release()
	if err := ctx.Err(); err != nil {
		return errors.Join(err, o.unmapAdamBuffer(chunk.uploadStage))
	}
	return o.unmapAdamBuffer(chunk.uploadStage)
}

func encodeAdamUniform(cfg AdamWConfig, learningRate float32, count int) []byte {
	data := make([]byte, deviceAdamUniformBytes)
	binary.LittleEndian.PutUint32(data[0:], math.Float32bits(cfg.Beta1))
	binary.LittleEndian.PutUint32(data[4:], math.Float32bits(cfg.Beta2))
	binary.LittleEndian.PutUint32(data[8:], math.Float32bits(cfg.Epsilon))
	binary.LittleEndian.PutUint32(data[12:], math.Float32bits(cfg.WeightDecay))
	binary.LittleEndian.PutUint32(data[16:], math.Float32bits(learningRate))
	binary.LittleEndian.PutUint32(data[20:], uint32(count))
	return data
}

func (o *DeviceAdamW) readAdamStatus(ctx context.Context, buffer *wgpu.Buffer) (uint32, error) {
	if err := o.mapAdamBuffer(ctx, buffer, wgpu.MapModeRead, deviceAdamStagingBytes); err != nil {
		return 0, err
	}
	rangeView, err := buffer.MappedRange(0, deviceAdamStagingBytes)
	if err != nil {
		return 0, errors.Join(err, o.unmapAdamBuffer(buffer))
	}
	status := binary.LittleEndian.Uint32(rangeView.Bytes())
	rangeView.Release()
	if err := o.unmapAdamBuffer(buffer); err != nil {
		return 0, err
	}
	return status, nil
}

func deviceAdamStatusError(status uint32, chunk int) error {
	switch {
	case status&deviceAdamStatusCounterOverflow != 0:
		return fmt.Errorf("%w: chunk %d parameter update counter overflow", ErrCapacity, chunk)
	case status&(deviceAdamStatusNonFinite|deviceAdamStatusInvalidState) != 0:
		return fmt.Errorf("%w: chunk %d produced a non-finite or invalid candidate", ErrNonFinite, chunk)
	default:
		return fmt.Errorf("%w: chunk %d returned status 0x%x", ErrExecution, chunk, status)
	}
}

func (o *DeviceAdamW) snapshotAdamLocked(ctx context.Context) (snapshot AdamWSnapshot, err error) {
	metrics := o.metrics
	metrics.Readback = 0
	snapshot, err = o.snapshotAdamBankLocked(ctx, o.currentBank, &metrics)
	o.metrics = metrics
	return snapshot, err
}

// snapshotAdamBankLocked reads one complete resident bank into fresh host
// slices. It adds full state readback bytes and duration to metrics while
// leaving o.metrics untouched; callers can therefore combine candidate
// readback with the execute phase without a deferred metrics assignment
// erasing either measurement. The caller must hold o.mu.
func (o *DeviceAdamW) snapshotAdamBankLocked(ctx context.Context, bank int, metrics *AdamWMetrics) (snapshot AdamWSnapshot, err error) {
	if metrics == nil {
		return AdamWSnapshot{}, fmt.Errorf("%w: nil snapshot metrics", ErrInvalidConfig)
	}
	if bank < 0 || bank >= 2 {
		return AdamWSnapshot{}, fmt.Errorf("%w: invalid snapshot bank %d", ErrExecution, bank)
	}
	snapshot = AdamWSnapshot{
		SchemaVersion: deviceAdamSchema,
		Config:        o.cfg,
		Parameters:    make([]float32, o.values),
		First:         make([]float32, o.values),
		Second:        make([]float32, o.values),
		Steps:         make([]uint32, o.values),
	}
	readbackStart := time.Now()
	defer func() {
		metrics.Readback += time.Since(readbackStart)
	}()
	for i := range o.chunks {
		if err := ctx.Err(); err != nil {
			return AdamWSnapshot{}, err
		}
		chunk := &o.chunks[i]
		encoder, err := o.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "coimnet-device-adamw-snapshot-encoder"})
		if err != nil {
			return AdamWSnapshot{}, fmt.Errorf("%w: create snapshot encoder: %v", ErrExecution, err)
		}
		encoder.CopyBufferToBuffer(chunk.state[bank], 0, o.snapshotStaging, 0, chunk.stateBytes)
		commands, err := encoder.Finish()
		if err != nil {
			encoder.DiscardEncoding()
			return AdamWSnapshot{}, fmt.Errorf("%w: finish snapshot commands: %v", ErrExecution, err)
		}
		if err := ctx.Err(); err != nil {
			commands.Release()
			return AdamWSnapshot{}, err
		}
		if _, err := o.device.Queue().Submit(commands); err != nil {
			commands.Release()
			return AdamWSnapshot{}, o.drainAdamSubmission(fmt.Errorf("%w: submit snapshot: %v", ErrExecution, err))
		}
		if err := ctx.Err(); err != nil {
			return AdamWSnapshot{}, o.drainAdamSubmission(err)
		}
		if err := o.snapshotChunk(ctx, snapshot, chunk); err != nil {
			return AdamWSnapshot{}, o.drainAdamSubmission(errors.Join(fmt.Errorf("snapshot chunk %d", i), err))
		}
		metrics.BytesReadBack += chunk.stateBytes
	}
	return snapshot, nil
}

func (o *DeviceAdamW) snapshotChunk(ctx context.Context, snapshot AdamWSnapshot, chunk *deviceAdamChunk) error {
	if err := o.mapAdamBuffer(ctx, o.snapshotStaging, wgpu.MapModeRead, chunk.stateBytes); err != nil {
		return err
	}
	rangeView, err := o.snapshotStaging.MappedRange(0, chunk.stateBytes)
	if err != nil {
		return errors.Join(err, o.unmapAdamBuffer(o.snapshotStaging))
	}
	raw := rangeView.Bytes()
	var stateErr error
	for i := 0; i < chunk.count; i++ {
		at := i * int(deviceAdamStateBytes)
		index := chunk.start + i
		parameter := math.Float32frombits(binary.LittleEndian.Uint32(raw[at:]))
		first := math.Float32frombits(binary.LittleEndian.Uint32(raw[at+4:]))
		second := math.Float32frombits(binary.LittleEndian.Uint32(raw[at+8:]))
		step := binary.LittleEndian.Uint32(raw[at+12:])
		if !isFinite32(parameter) || !isFinite32(first) || !isFinite32(second) || second < 0 {
			stateErr = fmt.Errorf("%w: state[%d] is non-finite or invalid", ErrNonFinite, index)
			break
		}
		if step == 0 && (first != 0 || second != 0) {
			stateErr = fmt.Errorf("%w: state[%d] has nonzero moments at step zero", ErrInvalidConfig, index)
			break
		}
		snapshot.Parameters[index] = parameter
		snapshot.First[index] = first
		snapshot.Second[index] = second
		snapshot.Steps[index] = step
	}
	rangeView.Release()
	if err := o.unmapAdamBuffer(o.snapshotStaging); err != nil {
		return errors.Join(stateErr, err)
	}
	return stateErr
}
