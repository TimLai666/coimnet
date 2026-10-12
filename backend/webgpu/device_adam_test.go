package webgpu

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

const adamTestSchema = "coimnet-device-adamw/v1"

func TestDeviceAdamWMatchesFloat32ReferenceAcrossWorkgroups(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	const values = 130
	parameters := make([]float32, values)
	for i := range parameters {
		parameters[i] = float32((i%19)-9) / 7
	}
	parameters[0] = 1
	cfg := AdamWConfig{
		Beta1:       0.5,
		Beta2:       0.25,
		Epsilon:     0.1,
		WeightDecay: 0.2,
		MaxBytes:    1 << 20,
	}
	optimizer := newTestDeviceAdamW(t, ctx, parameters, cfg)

	reference := newAdamReference(parameters)
	parameters[1] = 99
	assertAdamReferenceSnapshot(t, 0, mustDeviceAdamSnapshot(t, optimizer, ctx), reference)
	updates := []struct {
		gradient     []float32
		mask         []bool
		learningRate float32
	}{
		{gradient: adamGradient(values, 0, 13), mask: adamMask(values, 0), learningRate: 0.1},
		{gradient: adamGradient(values, 1, 17), mask: adamMask(values, 1), learningRate: 0.07},
		{gradient: adamGradient(values, 2, 23), mask: adamMask(values, 2), learningRate: 0.03},
		{gradient: adamGradient(values, 3, 29), mask: adamMask(values, 3), learningRate: 0.11},
		{gradient: make([]float32, values), mask: make([]bool, values), learningRate: 0.09},
	}

	initialMetrics := optimizer.Metrics()
	if initialMetrics.Values != values {
		t.Fatalf("Metrics Values = %d, want %d", initialMetrics.Values, values)
	}
	if initialMetrics.Chunks < 1 {
		t.Fatalf("Metrics Chunks = %d, want at least one chunk for %d values", initialMetrics.Chunks, values)
	}
	if initialMetrics.AllocatedBytes < uint64(40*values) {
		t.Fatalf("Metrics AllocatedBytes = %d, want at least live/shadow state and gradient/mask state %d", initialMetrics.AllocatedBytes, 40*values)
	}
	if initialMetrics.Device.Adapter == "" || initialMetrics.Device.Backend == "" {
		t.Fatalf("device identity missing from metrics: %+v", initialMetrics.Device)
	}
	if initialMetrics.Initialization <= 0 || initialMetrics.Compilation <= 0 || initialMetrics.InitialTransfer <= 0 || initialMetrics.Warmup <= 0 {
		t.Fatalf("constructor timings are not measured: %+v", initialMetrics)
	}

	for updateIndex, update := range updates {
		before := mustDeviceAdamSnapshot(t, optimizer, ctx)
		beforeMetrics := optimizer.Metrics()
		applyAdamReference(&reference, cfg, update.gradient, update.mask, update.learningRate)

		gotMetrics, err := optimizer.Step(ctx, update.gradient, update.mask, update.learningRate)
		if err != nil {
			t.Fatalf("Step %d: %v", updateIndex, err)
		}
		if gotMetrics.Values != values || gotMetrics.Chunks != initialMetrics.Chunks {
			t.Fatalf("Step %d metrics dimensions = values %d chunks %d, want %d and %d", updateIndex, gotMetrics.Values, gotMetrics.Chunks, values, initialMetrics.Chunks)
		}
		assertAdamStepTransferBudget(t, updateIndex, beforeMetrics, gotMetrics, values, gotMetrics.Chunks)

		got := mustDeviceAdamSnapshot(t, optimizer, ctx)
		assertAdamReferenceSnapshot(t, updateIndex, got, reference)
		// For parameter 0 on the first update, m=.25, v=.1875,
		// mhat=.5, vhat=.25, and p=1-.1*(.5/(.5+.1))-.1*.2*1.
		if updateIndex == 0 && !adamCloseFloat32(got.Parameters[0], float32(0.89666665)) {
			t.Fatalf("hand-calculated first parameter[0] = %.9g, want approximately 0.89666665", got.Parameters[0])
		}
		for i := range got.Parameters {
			if update.mask[i] {
				continue
			}
			if math.Float32bits(got.Parameters[i]) != math.Float32bits(before.Parameters[i]) ||
				math.Float32bits(got.First[i]) != math.Float32bits(before.First[i]) ||
				math.Float32bits(got.Second[i]) != math.Float32bits(before.Second[i]) ||
				got.Steps[i] != before.Steps[i] {
				t.Errorf("Step %d masked value %d changed: before=%+v after=%+v", updateIndex, i, adamStateAt(before, i), adamStateAt(got, i))
			}
		}
	}

	got := mustDeviceAdamSnapshot(t, optimizer, ctx)
	if got.Steps[0] == got.Steps[5] {
		t.Fatalf("per-parameter steps did not diverge after masks: step[0]=%d step[5]=%d", got.Steps[0], got.Steps[5])
	}
}

func TestDeviceAdamWSnapshotRestoreContinuesExactly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	parameters := []float32{0.75, -1.25, 0.5, 2, -0.125}
	cfg := AdamWConfig{Beta1: 0.8, Beta2: 0.9, Epsilon: 1e-3, WeightDecay: 0.015, MaxBytes: 1 << 20}
	original := newTestDeviceAdamW(t, ctx, parameters, cfg)

	warmup := []struct {
		gradient     []float32
		mask         []bool
		learningRate float32
	}{
		{gradient: []float32{0.5, -0.25, 0.75, -1, 0.125}, mask: []bool{true, true, false, true, false}, learningRate: 0.05},
		{gradient: []float32{-0.2, 0.6, -0.4, 0.8, -0.1}, mask: []bool{false, true, true, false, true}, learningRate: 0.025},
	}
	for i, update := range warmup {
		if _, err := original.Step(ctx, update.gradient, update.mask, update.learningRate); err != nil {
			t.Fatalf("warmup Step %d: %v", i, err)
		}
	}

	beforeSnapshotMetrics := original.Metrics()
	saved := mustDeviceAdamSnapshot(t, original, ctx)
	if saved.SchemaVersion != adamTestSchema {
		t.Fatalf("Snapshot schema = %q, want %q", saved.SchemaVersion, adamTestSchema)
	}
	afterSnapshotMetrics := original.Metrics()
	wantSnapshotReadBack := uint64(16 * len(saved.Parameters))
	hasSnapshotReadBack := afterSnapshotMetrics.BytesReadBack >= wantSnapshotReadBack
	if !hasSnapshotReadBack && afterSnapshotMetrics.BytesReadBack >= beforeSnapshotMetrics.BytesReadBack {
		hasSnapshotReadBack = afterSnapshotMetrics.BytesReadBack-beforeSnapshotMetrics.BytesReadBack >= wantSnapshotReadBack
	}
	if !hasSnapshotReadBack {
		t.Fatalf("Snapshot readback bytes = before %d after %d, want at least %d resident-state bytes", beforeSnapshotMetrics.BytesReadBack, afterSnapshotMetrics.BytesReadBack, 16*len(saved.Parameters))
	}
	expected := cloneAdamSnapshot(saved)
	restoreInput := cloneAdamSnapshot(saved)
	saved.Parameters[0] = 99
	saved.First[0] = 98
	saved.Second[0] = 97
	saved.Steps[0] = 96
	assertAdamSnapshotsEqual(t, mustDeviceAdamSnapshot(t, original, ctx), expected)

	restored, err := RestoreDeviceAdamW(ctx, restoreInput)
	if err != nil {
		if errors.Is(err, ErrUnavailable) {
			t.Skipf("GPU unavailable while restoring: %v", err)
		}
		t.Fatalf("RestoreDeviceAdamW: %v", err)
	}
	defer func() {
		if err := restored.Close(); err != nil {
			t.Errorf("restored Close: %v", err)
		}
	}()

	restoreInput.Parameters[0] = 95
	restoreInput.First[0] = 94
	restoreInput.Second[0] = 93
	restoreInput.Steps[0] = 92
	assertAdamSnapshotsEqual(t, mustDeviceAdamSnapshot(t, restored, ctx), expected)
	continuation := []struct {
		gradient     []float32
		mask         []bool
		learningRate float32
	}{
		{gradient: []float32{1.25, -0.75, 0.2, 0.4, -0.6}, mask: []bool{true, false, true, true, false}, learningRate: 0.04},
		{gradient: []float32{-0.3, 0.1, -0.9, 0.7, 0.2}, mask: []bool{false, true, true, false, true}, learningRate: 0.08},
		{gradient: []float32{0, 0, 0, 0, 0}, mask: []bool{false, false, false, false, false}, learningRate: 0.2},
	}
	for i, update := range continuation {
		if _, err := original.Step(ctx, update.gradient, update.mask, update.learningRate); err != nil {
			t.Fatalf("original continuation Step %d: %v", i, err)
		}
		if _, err := restored.Step(ctx, update.gradient, update.mask, update.learningRate); err != nil {
			t.Fatalf("restored continuation Step %d: %v", i, err)
		}
		assertAdamSnapshotsEqual(t, mustDeviceAdamSnapshot(t, restored, ctx), mustDeviceAdamSnapshot(t, original, ctx))
	}
}

func TestDeviceAdamWRejectsInvalidStepInputsAtomically(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	parameters := []float32{0.5, -0.25, 1.25}
	cfg := AdamWConfig{Beta1: 0.7, Beta2: 0.8, Epsilon: 1e-3, WeightDecay: 0.01, MaxBytes: 1 << 20}
	optimizer := newTestDeviceAdamW(t, ctx, parameters, cfg)
	if _, err := optimizer.Step(ctx, []float32{0.2, -0.3, 0.4}, []bool{true, true, true}, 0.05); err != nil {
		t.Fatalf("baseline Step: %v", err)
	}

	cases := []struct {
		name         string
		ctx          context.Context
		gradient     []float32
		mask         []bool
		learningRate float32
		wantErr      error
	}{
		{name: "gradient shape", ctx: ctx, gradient: []float32{0, 0}, mask: []bool{true, true, true}, learningRate: 0.1, wantErr: ErrShape},
		{name: "mask shape", ctx: ctx, gradient: []float32{0, 0, 0}, mask: []bool{true}, learningRate: 0.1, wantErr: ErrShape},
		{name: "gradient NaN", ctx: ctx, gradient: []float32{float32(math.NaN()), 0, 0}, mask: []bool{true, true, true}, learningRate: 0.1, wantErr: ErrNonFinite},
		{name: "gradient positive Inf", ctx: ctx, gradient: []float32{float32(math.Inf(1)), 0, 0}, mask: []bool{true, true, true}, learningRate: 0.1, wantErr: ErrNonFinite},
		{name: "gradient negative Inf", ctx: ctx, gradient: []float32{float32(math.Inf(-1)), 0, 0}, mask: []bool{true, true, true}, learningRate: 0.1, wantErr: ErrNonFinite},
		{name: "learning rate zero", ctx: ctx, gradient: []float32{0, 0, 0}, mask: []bool{true, true, true}, learningRate: 0, wantErr: nil},
		{name: "learning rate negative", ctx: ctx, gradient: []float32{0, 0, 0}, mask: []bool{true, true, true}, learningRate: -0.1, wantErr: nil},
		{name: "learning rate NaN", ctx: ctx, gradient: []float32{0, 0, 0}, mask: []bool{true, true, true}, learningRate: float32(math.NaN()), wantErr: ErrNonFinite},
		{name: "learning rate Inf", ctx: ctx, gradient: []float32{0, 0, 0}, mask: []bool{true, true, true}, learningRate: float32(math.Inf(1)), wantErr: ErrNonFinite},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := mustDeviceAdamSnapshot(t, optimizer, ctx)
			_, err := optimizer.Step(tc.ctx, tc.gradient, tc.mask, tc.learningRate)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Step error = %v, want errors.Is(..., %v)", err, tc.wantErr)
				}
			} else if err == nil {
				t.Fatal("Step unexpectedly accepted invalid learning rate")
			}
			after := mustDeviceAdamSnapshot(t, optimizer, ctx)
			assertAdamSnapshotsEqual(t, after, before)
		})
	}

	canceled, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	before := mustDeviceAdamSnapshot(t, optimizer, ctx)
	if _, err := optimizer.Step(canceled, []float32{0, 0, 0}, []bool{true, true, true}, 0.1); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Step error = %v, want context.Canceled", err)
	}
	assertAdamSnapshotsEqual(t, mustDeviceAdamSnapshot(t, optimizer, ctx), before)

	if _, err := optimizer.Step(nil, []float32{0, 0, 0}, []bool{true, true, true}, 0.1); err == nil {
		t.Fatal("nil-context Step unexpectedly succeeded")
	}
	assertAdamSnapshotsEqual(t, mustDeviceAdamSnapshot(t, optimizer, ctx), before)
}

func TestDeviceAdamWRejectsNonFiniteCandidateAndCounterOverflowAtomically(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	cfg := AdamWConfig{Beta1: 0.5, Beta2: 0.25, Epsilon: 0.1, WeightDecay: 0, MaxBytes: 1 << 20}
	nonFinite := newTestDeviceAdamW(t, ctx, []float32{0}, cfg)
	before := mustDeviceAdamSnapshot(t, nonFinite, ctx)
	if _, err := nonFinite.Step(ctx, []float32{float32(math.MaxFloat32)}, []bool{true}, 1); err == nil {
		t.Fatal("Step unexpectedly accepted a finite gradient that creates a non-finite GPU candidate")
	}
	assertAdamSnapshotsEqual(t, mustDeviceAdamSnapshot(t, nonFinite, ctx), before)

	seed := newTestDeviceAdamW(t, ctx, []float32{0, 1}, cfg)
	seedSnapshot := mustDeviceAdamSnapshot(t, seed, ctx)
	seedSnapshot.Steps[0] = ^uint32(0)
	overflow, err := RestoreDeviceAdamW(ctx, seedSnapshot)
	if err != nil {
		if errors.Is(err, ErrUnavailable) {
			t.Skipf("GPU unavailable while restoring overflow fixture: %v", err)
		}
		t.Fatalf("RestoreDeviceAdamW accepted-state overflow fixture: %v", err)
	}
	defer func() {
		if err := overflow.Close(); err != nil {
			t.Errorf("overflow Close: %v", err)
		}
	}()
	overflowBefore := mustDeviceAdamSnapshot(t, overflow, ctx)
	if _, err := overflow.Step(ctx, []float32{0.5, 0.5}, []bool{true, false}, 0.1); err == nil {
		t.Fatal("Step unexpectedly accepted uint32 step overflow")
	}
	assertAdamSnapshotsEqual(t, mustDeviceAdamSnapshot(t, overflow, ctx), overflowBefore)
}

func TestRestoreDeviceAdamWRejectsCorruptSnapshotsBeforeDeviceOpen(t *testing.T) {
	base := AdamWSnapshot{
		SchemaVersion: adamTestSchema,
		Config:        AdamWConfig{Beta1: 0.9, Beta2: 0.99, Epsilon: 1e-4, WeightDecay: 0.01, MaxBytes: 1 << 20},
		Parameters:    []float32{0},
		First:         []float32{0},
		Second:        []float32{0},
		Steps:         []uint32{0},
	}
	cases := []struct {
		name   string
		mutate func(*AdamWSnapshot)
	}{
		{name: "empty schema", mutate: func(s *AdamWSnapshot) { s.SchemaVersion = "" }},
		{name: "unknown schema", mutate: func(s *AdamWSnapshot) { s.SchemaVersion = "coimnet-device-adamw/v2" }},
		{name: "empty parameters", mutate: func(s *AdamWSnapshot) { s.Parameters = nil }},
		{name: "first shape", mutate: func(s *AdamWSnapshot) { s.First = nil }},
		{name: "second shape", mutate: func(s *AdamWSnapshot) { s.Second = nil }},
		{name: "steps shape", mutate: func(s *AdamWSnapshot) { s.Steps = nil }},
		{name: "parameter NaN", mutate: func(s *AdamWSnapshot) { s.Parameters[0] = float32(math.NaN()) }},
		{name: "first Inf", mutate: func(s *AdamWSnapshot) { s.First[0] = float32(math.Inf(1)) }},
		{name: "second negative", mutate: func(s *AdamWSnapshot) { s.Second[0] = -1 }},
		{name: "nonzero moments at step zero", mutate: func(s *AdamWSnapshot) { s.First[0], s.Second[0] = 0.25, 0.125 }},
		{name: "beta1 NaN", mutate: func(s *AdamWSnapshot) { s.Config.Beta1 = float32(math.NaN()) }},
		{name: "beta2 one", mutate: func(s *AdamWSnapshot) { s.Config.Beta2 = 1 }},
		{name: "epsilon zero", mutate: func(s *AdamWSnapshot) { s.Config.Epsilon = 0 }},
		{name: "weight decay negative", mutate: func(s *AdamWSnapshot) { s.Config.WeightDecay = -1 }},
		{name: "zero budget", mutate: func(s *AdamWSnapshot) { s.Config.MaxBytes = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := cloneAdamSnapshot(base)
			tc.mutate(&candidate)
			_, err := RestoreDeviceAdamW(context.Background(), candidate)
			if err == nil {
				t.Fatal("RestoreDeviceAdamW unexpectedly accepted corrupt snapshot")
			}
			if errors.Is(err, ErrUnavailable) {
				t.Fatalf("corrupt snapshot reached hardware setup: %v", err)
			}
		})
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := RestoreDeviceAdamW(canceled, base)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled RestoreDeviceAdamW error = %v, want context.Canceled", err)
	}
}

func TestNewDeviceAdamWRejectsInvalidConfigBeforeDeviceOpen(t *testing.T) {
	valid := AdamWConfig{Beta1: 0.9, Beta2: 0.99, Epsilon: 1e-4, WeightDecay: 0.01, MaxBytes: 1 << 20}
	cases := []struct {
		name       string
		ctx        context.Context
		parameters []float32
		cfg        AdamWConfig
		want       error
	}{
		{name: "nil context", ctx: nil, parameters: []float32{0}, cfg: valid},
		{name: "canceled context", ctx: canceledContext(), parameters: []float32{0}, cfg: valid},
		{name: "empty parameters", ctx: context.Background(), parameters: nil, cfg: valid, want: ErrShape},
		{name: "parameter NaN", ctx: context.Background(), parameters: []float32{float32(math.NaN())}, cfg: valid, want: ErrNonFinite},
		{name: "parameter Inf", ctx: context.Background(), parameters: []float32{float32(math.Inf(1))}, cfg: valid, want: ErrNonFinite},
		{name: "beta1 NaN", ctx: context.Background(), parameters: []float32{0}, cfg: AdamWConfig{Beta1: float32(math.NaN()), Beta2: valid.Beta2, Epsilon: valid.Epsilon, MaxBytes: valid.MaxBytes}},
		{name: "beta1 one", ctx: context.Background(), parameters: []float32{0}, cfg: AdamWConfig{Beta1: 1, Beta2: valid.Beta2, Epsilon: valid.Epsilon, MaxBytes: valid.MaxBytes}},
		{name: "beta2 NaN", ctx: context.Background(), parameters: []float32{0}, cfg: AdamWConfig{Beta1: valid.Beta1, Beta2: float32(math.NaN()), Epsilon: valid.Epsilon, MaxBytes: valid.MaxBytes}},
		{name: "beta2 negative", ctx: context.Background(), parameters: []float32{0}, cfg: AdamWConfig{Beta1: valid.Beta1, Beta2: -0.1, Epsilon: valid.Epsilon, MaxBytes: valid.MaxBytes}},
		{name: "epsilon zero", ctx: context.Background(), parameters: []float32{0}, cfg: AdamWConfig{Beta1: valid.Beta1, Beta2: valid.Beta2, Epsilon: 0, MaxBytes: valid.MaxBytes}},
		{name: "epsilon Inf", ctx: context.Background(), parameters: []float32{0}, cfg: AdamWConfig{Beta1: valid.Beta1, Beta2: valid.Beta2, Epsilon: float32(math.Inf(1)), MaxBytes: valid.MaxBytes}},
		{name: "weight decay negative", ctx: context.Background(), parameters: []float32{0}, cfg: AdamWConfig{Beta1: valid.Beta1, Beta2: valid.Beta2, Epsilon: valid.Epsilon, WeightDecay: -0.1, MaxBytes: valid.MaxBytes}},
		{name: "weight decay Inf", ctx: context.Background(), parameters: []float32{0}, cfg: AdamWConfig{Beta1: valid.Beta1, Beta2: valid.Beta2, Epsilon: valid.Epsilon, WeightDecay: float32(math.Inf(1)), MaxBytes: valid.MaxBytes}},
		{name: "max bytes zero", ctx: context.Background(), parameters: []float32{0}, cfg: AdamWConfig{Beta1: valid.Beta1, Beta2: valid.Beta2, Epsilon: valid.Epsilon, MaxBytes: 0}},
		{name: "max bytes too small", ctx: context.Background(), parameters: []float32{0}, cfg: AdamWConfig{Beta1: valid.Beta1, Beta2: valid.Beta2, Epsilon: valid.Epsilon, MaxBytes: 1}, want: ErrCapacity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewDeviceAdamW(tc.ctx, tc.parameters, tc.cfg)
			if err == nil {
				t.Fatal("NewDeviceAdamW unexpectedly accepted invalid input")
			}
			if errors.Is(err, ErrUnavailable) {
				t.Fatalf("invalid input reached hardware setup: %v", err)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("NewDeviceAdamW error = %v, want errors.Is(..., %v)", err, tc.want)
			}
		})
	}
}

func TestDeviceAdamWNilReceiverAndCloseIdempotent(t *testing.T) {
	var optimizer *DeviceAdamW
	if _, err := optimizer.Step(context.Background(), []float32{0}, []bool{true}, 0.1); err == nil {
		t.Fatal("nil receiver Step unexpectedly succeeded")
	}
	if _, err := optimizer.Snapshot(context.Background()); err == nil {
		t.Fatal("nil receiver Snapshot unexpectedly succeeded")
	}
	if err := optimizer.Close(); err == nil {
		t.Fatal("nil receiver Close unexpectedly succeeded")
	}
	if got := optimizer.Metrics(); got != (AdamWMetrics{}) {
		t.Fatalf("nil receiver Metrics = %+v, want zero metrics", got)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	valid := newTestDeviceAdamW(t, ctx, []float32{0.25}, AdamWConfig{Beta1: 0.9, Beta2: 0.99, Epsilon: 1e-4, MaxBytes: 1 << 20})
	if err := valid.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := valid.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := valid.Step(ctx, []float32{0}, []bool{true}, 0.1); !errors.Is(err, ErrClosed) {
		t.Fatalf("Step after Close error = %v, want ErrClosed", err)
	}
	if _, err := valid.Snapshot(ctx); !errors.Is(err, ErrClosed) {
		t.Fatalf("Snapshot after Close error = %v, want ErrClosed", err)
	}
}

type adamReference struct {
	parameters []float32
	first      []float32
	second     []float32
	steps      []uint32
}

func newAdamReference(parameters []float32) adamReference {
	return adamReference{
		parameters: append([]float32(nil), parameters...),
		first:      make([]float32, len(parameters)),
		second:     make([]float32, len(parameters)),
		steps:      make([]uint32, len(parameters)),
	}
}

func applyAdamReference(state *adamReference, cfg AdamWConfig, gradient []float32, mask []bool, learningRate float32) {
	for i, enabled := range mask {
		if !enabled {
			continue
		}
		step := state.steps[i] + 1
		g := gradient[i]
		one := float32(1)
		m := cfg.Beta1*state.first[i] + (one-cfg.Beta1)*g
		v := cfg.Beta2*state.second[i] + (one-cfg.Beta2)*(g*g)
		beta1Power := float32(math.Pow(float64(cfg.Beta1), float64(step)))
		beta2Power := float32(math.Pow(float64(cfg.Beta2), float64(step)))
		mHat := m / (one - beta1Power)
		vHat := v / (one - beta2Power)
		denominator := float32(math.Sqrt(float64(vHat))) + cfg.Epsilon
		adamUpdate := learningRate * (mHat / denominator)
		decayUpdate := learningRate * cfg.WeightDecay * state.parameters[i]
		state.parameters[i] = state.parameters[i] - adamUpdate - decayUpdate
		state.first[i], state.second[i], state.steps[i] = m, v, step
	}
}

func adamGradient(values, round, period int) []float32 {
	gradient := make([]float32, values)
	for i := range gradient {
		gradient[i] = float32(((i+1)*(round+3))%period-(period/2)) / float32(period/3)
	}
	gradient[0] = 0.5
	return gradient
}

func adamMask(values, round int) []bool {
	mask := make([]bool, values)
	for i := range mask {
		mask[i] = (i+round)%5 != 0
	}
	mask[0] = true
	return mask
}

func newTestDeviceAdamW(t *testing.T, ctx context.Context, parameters []float32, cfg AdamWConfig) *DeviceAdamW {
	t.Helper()
	optimizer, err := NewDeviceAdamW(ctx, parameters, cfg)
	if err != nil {
		if errors.Is(err, ErrUnavailable) {
			t.Skipf("GPU unavailable: %v", err)
		}
		t.Fatalf("NewDeviceAdamW: %v", err)
	}
	t.Cleanup(func() {
		if err := optimizer.Close(); err != nil {
			t.Errorf("DeviceAdamW Close: %v", err)
		}
	})
	return optimizer
}

func mustDeviceAdamSnapshot(t *testing.T, optimizer *DeviceAdamW, ctx context.Context) AdamWSnapshot {
	t.Helper()
	snapshot, err := optimizer.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	return snapshot
}

func assertAdamReferenceSnapshot(t *testing.T, update int, got AdamWSnapshot, want adamReference) {
	t.Helper()
	if len(got.Parameters) != len(want.parameters) || len(got.First) != len(want.first) || len(got.Second) != len(want.second) || len(got.Steps) != len(want.steps) {
		t.Fatalf("Step %d snapshot shapes = parameters %d first %d second %d steps %d, want %d each", update, len(got.Parameters), len(got.First), len(got.Second), len(got.Steps), len(want.parameters))
	}
	for i := range want.parameters {
		if !adamCloseFloat32(got.Parameters[i], want.parameters[i]) {
			t.Errorf("Step %d parameter[%d] = %.9g, want %.9g", update, i, got.Parameters[i], want.parameters[i])
		}
		if !adamCloseFloat32(got.First[i], want.first[i]) {
			t.Errorf("Step %d first[%d] = %.9g, want %.9g", update, i, got.First[i], want.first[i])
		}
		if !adamCloseFloat32(got.Second[i], want.second[i]) {
			t.Errorf("Step %d second[%d] = %.9g, want %.9g", update, i, got.Second[i], want.second[i])
		}
		if got.Steps[i] != want.steps[i] {
			t.Errorf("Step %d step[%d] = %d, want %d", update, i, got.Steps[i], want.steps[i])
		}
	}
}

func assertAdamSnapshotsEqual(t *testing.T, got, want AdamWSnapshot) {
	t.Helper()
	if got.SchemaVersion != want.SchemaVersion || got.Config != want.Config {
		t.Fatalf("snapshot metadata differs: got schema=%q config=%+v, want schema=%q config=%+v", got.SchemaVersion, got.Config, want.SchemaVersion, want.Config)
	}
	if len(got.Parameters) != len(want.Parameters) || len(got.First) != len(want.First) || len(got.Second) != len(want.Second) || len(got.Steps) != len(want.Steps) {
		t.Fatalf("snapshot shapes differ: got (%d,%d,%d,%d), want (%d,%d,%d,%d)", len(got.Parameters), len(got.First), len(got.Second), len(got.Steps), len(want.Parameters), len(want.First), len(want.Second), len(want.Steps))
	}
	for i := range got.Parameters {
		if math.Float32bits(got.Parameters[i]) != math.Float32bits(want.Parameters[i]) ||
			math.Float32bits(got.First[i]) != math.Float32bits(want.First[i]) ||
			math.Float32bits(got.Second[i]) != math.Float32bits(want.Second[i]) ||
			got.Steps[i] != want.Steps[i] {
			t.Fatalf("snapshot value[%d] differs: got=%+v want=%+v", i, adamStateAt(got, i), adamStateAt(want, i))
		}
	}
}

func assertAdamStepTransferBudget(t *testing.T, update int, before, after AdamWMetrics, values, chunks int) {
	t.Helper()
	stepUploadBound := uint64(8*values + 32*chunks + 4*chunks)
	stepReadbackBound := uint64(4 * chunks)
	readbackOK := after.BytesReadBack <= stepReadbackBound
	if !readbackOK && after.BytesReadBack >= before.BytesReadBack {
		readbackOK = after.BytesReadBack-before.BytesReadBack <= stepReadbackBound
	}
	if !readbackOK {
		t.Fatalf("Step %d read back resident state or reported invalid transfer counters: before=%d after=%d bound=%d", update, before.BytesReadBack, after.BytesReadBack, stepReadbackBound)
	}
	uploadOK := after.BytesUploaded <= stepUploadBound
	if !uploadOK && after.BytesUploaded >= before.BytesUploaded {
		uploadOK = after.BytesUploaded-before.BytesUploaded <= stepUploadBound
	}
	if !uploadOK {
		t.Fatalf("Step %d uploaded resident state or reported invalid transfer counters: before=%d after=%d bound=%d", update, before.BytesUploaded, after.BytesUploaded, stepUploadBound)
	}
}

func adamStateAt(snapshot AdamWSnapshot, i int) struct {
	parameter float32
	first     float32
	second    float32
	step      uint32
} {
	return struct {
		parameter float32
		first     float32
		second    float32
		step      uint32
	}{snapshot.Parameters[i], snapshot.First[i], snapshot.Second[i], snapshot.Steps[i]}
}

func cloneAdamSnapshot(snapshot AdamWSnapshot) AdamWSnapshot {
	snapshot.Parameters = append([]float32(nil), snapshot.Parameters...)
	snapshot.First = append([]float32(nil), snapshot.First...)
	snapshot.Second = append([]float32(nil), snapshot.Second...)
	snapshot.Steps = append([]uint32(nil), snapshot.Steps...)
	return snapshot
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func adamCloseFloat32(got, want float32) bool {
	diff := math.Abs(float64(got - want))
	return diff <= 3e-5+2e-4*math.Abs(float64(want))
}
