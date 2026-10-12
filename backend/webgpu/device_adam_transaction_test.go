package webgpu

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

func TestDeviceAdamWPlanIncludesExplicitUploadBuffers(t *testing.T) {
	const values, chunkSize, chunks = 130, 64, 3
	want := uint64(48*values + 76*chunks + 16*chunkSize)
	optimizer := DeviceAdamW{values: values, cfg: AdamWConfig{MaxBytes: want}, limits: gputypes.Limits{
		MaxStorageBufferBindingSize: 16 * chunkSize, MaxBufferSize: 16 * chunkSize,
	}}
	if err := optimizer.planAdamChunks(); err != nil {
		t.Fatal(err)
	}
	if optimizer.allocated != want || len(optimizer.chunks) != chunks {
		t.Fatalf("logical GPU buffers = %d bytes in %d chunks, want %d in %d including upload staging", optimizer.allocated, len(optimizer.chunks), want, chunks)
	}
	optimizer.cfg.MaxBytes = want - 1
	if err := optimizer.planAdamChunks(); !errors.Is(err, ErrCapacity) {
		t.Fatalf("insufficient complete buffer budget accepted: %v", err)
	}
}

type cancelDuringAdamMap struct {
	context.Context
	checks int
	done   chan struct{}
}

func (c *cancelDuringAdamMap) Done() <-chan struct{} { return c.done }
func (c *cancelDuringAdamMap) Err() error {
	c.checks++
	if c.checks == 2 {
		close(c.done)
	}
	if c.checks >= 2 {
		return context.Canceled
	}
	return nil
}

func TestDeviceAdamWCancelledMapDrainsBeforeReuse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	optimizer := newTestDeviceAdamW(t, ctx, []float32{1, -1}, AdamWConfig{
		Beta1: 0.9, Beta2: 0.99, Epsilon: 1e-4, MaxBytes: 1 << 20,
	})
	before := mustDeviceAdamSnapshot(t, optimizer, ctx)
	canceled := &cancelDuringAdamMap{Context: context.Background(), done: make(chan struct{})}
	optimizer.mu.Lock()
	err := optimizer.mapAdamBuffer(canceled, optimizer.chunks[0].statusStage, wgpu.MapModeRead, deviceAdamStagingBytes)
	optimizer.mu.Unlock()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("map cancellation identity lost: %v", err)
	}
	// Reuse both status and shared snapshot staging after the old map is drained.
	for i := 0; i < 3; i++ {
		if _, err := optimizer.Step(ctx, []float32{0.25, -0.5}, []bool{true, false}, 0.01); err != nil {
			t.Fatal(err)
		}
		snapshot := mustDeviceAdamSnapshot(t, optimizer, ctx)
		if snapshot.Steps[0] != uint32(i+1) || snapshot.Steps[1] != before.Steps[1] || snapshot.Parameters[1] != before.Parameters[1] {
			t.Fatal("old cancelled map contaminated the next successful update")
		}
	}
}
