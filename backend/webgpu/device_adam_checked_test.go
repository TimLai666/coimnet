package webgpu

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

func TestDeviceAdamWCheckedTransactions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const values = 130
	optimizer := newChunkedHardwareAdam(t, ctx, values)
	parameters := make([]float32, values)
	for i := range parameters {
		parameters[i] = float32(i%7) / 5
	}
	cfg := optimizer.cfg

	reference := newAdamReference(parameters)
	gradient := adamGradient(values, 0, 17)
	mask := adamMask(values, 0)
	applyAdamReference(&reference, cfg, gradient, mask, 0.07)
	callbackCalled := false
	metrics, err := optimizer.StepChecked(ctx, gradient, mask, 0.07, func(candidate AdamWSnapshot) error {
		callbackCalled = true
		assertAdamReferenceSnapshot(t, 0, candidate, reference)
		// The callback owns this view. Mutating every slice must not mutate the
		// candidate that the optimizer commits after the callback returns.
		candidate.Parameters[0] = 91
		candidate.First[0] = 92
		candidate.Second[0] = 93
		candidate.Steps[0] = 94
		return nil
	})
	if err != nil {
		t.Fatalf("StepChecked: %v", err)
	}
	if !callbackCalled {
		t.Fatal("StepChecked did not invoke its acceptance callback")
	}
	if metrics.Values != values || metrics.Chunks != 3 {
		t.Fatalf("StepChecked metrics dimensions = values %d chunks %d, want %d and 3", metrics.Values, metrics.Chunks, values)
	}
	if metrics.Device.Adapter == "" || metrics.Device.Backend == "" {
		t.Fatalf("StepChecked did not report a hardware device: %+v", metrics.Device)
	}
	assertAdamReferenceSnapshot(t, 0, mustDeviceAdamSnapshot(t, optimizer, ctx), reference)

	// A zero-rate update still advances moments and per-value counters, while
	// frozen values retain their exact float32 bits.
	zeroGradient := adamGradient(values, 1, 19)
	zeroMask := adamMask(values, 1)
	applyAdamReference(&reference, cfg, zeroGradient, zeroMask, 0)
	if _, err := optimizer.StepChecked(ctx, zeroGradient, zeroMask, 0, func(candidate AdamWSnapshot) error {
		assertAdamReferenceSnapshot(t, 1, candidate, reference)
		return nil
	}); err != nil {
		t.Fatalf("zero-rate StepChecked: %v", err)
	}
	assertAdamReferenceSnapshot(t, 1, mustDeviceAdamSnapshot(t, optimizer, ctx), reference)

	beforeReject := mustDeviceAdamSnapshot(t, optimizer, ctx)
	rejectErr := errors.New("reject candidate")
	if _, err := optimizer.StepChecked(ctx, adamGradient(values, 2, 23), adamMask(values, 2), 0.11, func(candidate AdamWSnapshot) error {
		candidate.Parameters[0] = -123
		return rejectErr
	}); !errors.Is(err, rejectErr) {
		t.Fatalf("callback rejection error = %v, want %v", err, rejectErr)
	}
	assertAdamSnapshotsEqual(t, mustDeviceAdamSnapshot(t, optimizer, ctx), beforeReject)

	// Cancelling after candidate readback exercises the final context check,
	// after all candidate chunks and callback work have already succeeded.
	cancelCtx, cancelStep := context.WithCancel(context.Background())
	beforeCancel := mustDeviceAdamSnapshot(t, optimizer, ctx)
	if _, err := optimizer.StepChecked(cancelCtx, adamGradient(values, 3, 29), adamMask(values, 3), 0.13, func(AdamWSnapshot) error {
		cancelStep()
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("callback cancellation error = %v, want context.Canceled", err)
	}
	assertAdamSnapshotsEqual(t, mustDeviceAdamSnapshot(t, optimizer, ctx), beforeCancel)

	// The rejected and cancelled candidates must not consume optimizer time.
	continuation := adamReferenceFromSnapshot(beforeCancel)
	continuationGradient := adamGradient(values, 4, 31)
	continuationMask := adamMask(values, 4)
	applyAdamReference(&continuation, cfg, continuationGradient, continuationMask, 0.09)
	if _, err := optimizer.StepChecked(ctx, continuationGradient, continuationMask, 0.09, func(candidate AdamWSnapshot) error {
		assertAdamReferenceSnapshot(t, 2, candidate, continuation)
		return nil
	}); err != nil {
		t.Fatalf("continuation StepChecked: %v", err)
	}
	assertAdamReferenceSnapshot(t, 2, mustDeviceAdamSnapshot(t, optimizer, ctx), continuation)
}

func TestDeviceAdamWCheckedRejectsInvalidInputsAtomically(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	optimizer := newTestDeviceAdamW(t, ctx, []float32{0.5, -0.25, 1.25}, AdamWConfig{
		Beta1: 0.7, Beta2: 0.9, Epsilon: 1e-3, WeightDecay: 0.01, MaxBytes: 1 << 20,
	})
	validGradient := []float32{0.2, -0.3, 0.4}
	validMask := []bool{true, true, true}
	cases := []struct {
		name     string
		gradient []float32
		mask     []bool
		rate     float32
		callback func(AdamWSnapshot) error
		want     error
	}{
		{name: "nil callback", gradient: validGradient, mask: validMask, rate: 0.1, want: ErrInvalidConfig},
		{name: "gradient shape", gradient: []float32{0, 0}, mask: validMask, rate: 0.1, callback: func(AdamWSnapshot) error { return nil }, want: ErrShape},
		{name: "mask shape", gradient: validGradient, mask: []bool{true}, rate: 0.1, callback: func(AdamWSnapshot) error { return nil }, want: ErrShape},
		{name: "gradient NaN", gradient: []float32{float32(math.NaN()), 0, 0}, mask: validMask, rate: 0.1, callback: func(AdamWSnapshot) error { return nil }, want: ErrNonFinite},
		{name: "negative rate", gradient: validGradient, mask: validMask, rate: -0.1, callback: func(AdamWSnapshot) error { return nil }, want: ErrInvalidConfig},
		{name: "rate NaN", gradient: validGradient, mask: validMask, rate: float32(math.NaN()), callback: func(AdamWSnapshot) error { return nil }, want: ErrNonFinite},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := mustDeviceAdamSnapshot(t, optimizer, ctx)
			_, err := optimizer.StepChecked(ctx, tc.gradient, tc.mask, tc.rate, tc.callback)
			if !errors.Is(err, tc.want) {
				t.Fatalf("StepChecked error = %v, want errors.Is(..., %v)", err, tc.want)
			}
			assertAdamSnapshotsEqual(t, mustDeviceAdamSnapshot(t, optimizer, ctx), before)
		})
	}

	beforeCandidate := mustDeviceAdamSnapshot(t, optimizer, ctx)
	gradient := []float32{math.MaxFloat32, 0, 0}
	_, err := optimizer.StepChecked(ctx, gradient, validMask, 1, func(AdamWSnapshot) error {
		t.Fatal("callback ran for a non-finite GPU candidate")
		return nil
	})
	if !errors.Is(err, ErrNonFinite) {
		t.Fatalf("non-finite candidate error = %v, want errors.Is(..., %v)", err, ErrNonFinite)
	}
	assertAdamSnapshotsEqual(t, mustDeviceAdamSnapshot(t, optimizer, ctx), beforeCandidate)
}

func TestDeviceAdamWCheckedCloseIsTerminalAndIdempotent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	optimizer := newTestDeviceAdamW(t, ctx, []float32{0.25}, AdamWConfig{
		Beta1: 0.9, Beta2: 0.99, Epsilon: 1e-4, MaxBytes: 1 << 20,
	})
	if err := optimizer.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := optimizer.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := optimizer.StepChecked(ctx, []float32{0}, []bool{true}, 0.1, func(AdamWSnapshot) error { return nil }); !errors.Is(err, ErrClosed) {
		t.Fatalf("StepChecked after Close error = %v, want ErrClosed", err)
	}
}

func TestDeviceAdamWCheckedRestoreOwnsStateAndIgnoresFrozenHugeGradient(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	parameters := []float32{0.25, -0.5, 0.75, -1.25}
	cfg := AdamWConfig{Beta1: 0.8, Beta2: 0.97, Epsilon: 1e-3, WeightDecay: 0.02, MaxBytes: 1 << 20}
	original := newTestDeviceAdamW(t, ctx, parameters, cfg)
	gradient := []float32{0.5, math.MaxFloat32, -0.25, 0.75}
	mask := []bool{true, false, true, false}
	reference := newAdamReference(parameters)
	applyAdamReference(&reference, cfg, gradient, mask, 0.04)
	if _, err := original.StepChecked(ctx, gradient, mask, 0.04, func(candidate AdamWSnapshot) error {
		assertAdamReferenceSnapshot(t, 0, candidate, reference)
		return nil
	}); err != nil {
		t.Fatalf("StepChecked with frozen huge gradient: %v", err)
	}
	expected := mustDeviceAdamSnapshot(t, original, ctx)
	assertAdamReferenceSnapshot(t, 0, expected, reference)

	restoreInput := cloneAdamSnapshot(expected)
	restored, err := RestoreDeviceAdamW(ctx, restoreInput)
	if err != nil {
		if errors.Is(err, ErrUnavailable) {
			t.Skipf("GPU unavailable while restoring checked snapshot: %v", err)
		}
		t.Fatalf("RestoreDeviceAdamW: %v", err)
	}
	defer func() {
		if err := restored.Close(); err != nil {
			t.Errorf("restored Close: %v", err)
		}
	}()

	// Restore must copy every resident slice and the exact kernel options.
	restoreInput.Config.Beta1 = 0.7
	restoreInput.Parameters[0] = 91
	restoreInput.First[0] = 92
	restoreInput.Second[0] = 93
	restoreInput.Steps[0] = 94
	assertAdamSnapshotsEqual(t, mustDeviceAdamSnapshot(t, restored, ctx), expected)

	continuationGradient := []float32{-0.3, math.MaxFloat32, 0.2, -0.6}
	continuationMask := []bool{true, false, true, false}
	applyAdamReference(&reference, cfg, continuationGradient, continuationMask, 0.06)
	if _, err := original.StepChecked(ctx, continuationGradient, continuationMask, 0.06, func(candidate AdamWSnapshot) error {
		assertAdamReferenceSnapshot(t, 1, candidate, reference)
		return nil
	}); err != nil {
		t.Fatalf("original continuation StepChecked: %v", err)
	}
	if _, err := restored.StepChecked(ctx, continuationGradient, continuationMask, 0.06, func(candidate AdamWSnapshot) error {
		assertAdamReferenceSnapshot(t, 1, candidate, reference)
		return nil
	}); err != nil {
		t.Fatalf("restored continuation StepChecked: %v", err)
	}
	assertAdamSnapshotsEqual(t, mustDeviceAdamSnapshot(t, restored, ctx), mustDeviceAdamSnapshot(t, original, ctx))
}

func adamReferenceFromSnapshot(snapshot AdamWSnapshot) adamReference {
	return adamReference{
		parameters: append([]float32(nil), snapshot.Parameters...),
		first:      append([]float32(nil), snapshot.First...),
		second:     append([]float32(nil), snapshot.Second...),
		steps:      append([]uint32(nil), snapshot.Steps...),
	}
}
