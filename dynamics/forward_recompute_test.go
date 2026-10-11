package dynamics

import (
	"context"
	"errors"
	"math"
	"strconv"
	"testing"
)

// forwardRecomputeContinuousFixture deliberately includes self edges and
// repeated source/target pairs with both zero and two-step delays. The
// nonzero initial voltage makes the prehistory observable on the first rows.
func forwardRecomputeContinuousFixture(t *testing.T) (*Continuous, Parameters, []float64, [][]float64) {
	t.Helper()
	m, err := NewContinuous(Config{
		Nodes:      4,
		Sources:    []int{0, 0, 1, 1, 2, 2, 3},
		Targets:    []int{0, 0, 1, 1, 2, 2, 0},
		Delays:     []int{0, 2, 0, 2, 0, 2, 2},
		DT:         0.4,
		Activation: "tanh",
	})
	if err != nil {
		t.Fatal(err)
	}
	p := Parameters{
		Weights: []float64{0.35, -0.2, 0.12, 0.05, -0.17, 0.08, 0.1},
		Bias:    []float64{0.03, -0.02, 0.04, -0.01},
		LogTau:  []float64{0.1, -0.15, 0.2, 0.05},
	}
	initial := []float64{0.2, -0.3, 0.4, -0.1}
	inputs := make([][]float64, 11)
	for step := range inputs {
		inputs[step] = []float64{
			0.15 + 0.03*float64(step),
			-0.2 + 0.02*float64(step%4),
			0.1 - 0.015*float64(step),
			-0.05 + 0.01*float64(step%3),
		}
	}
	return m, p, initial, inputs
}

// forwardRecomputeLIFFixture uses the same repeated self topology while
// choosing a drive that demonstrably exercises events, refractory holding,
// adaptation and homeostatic threshold movement.
func forwardRecomputeLIFFixture(t *testing.T) (*LIF, LIFParameters, []float64, [][]float64) {
	t.Helper()
	m, err := NewLIF(LIFConfig{
		Nodes:           4,
		Sources:         []int{0, 0, 1, 1, 2, 2, 3},
		Targets:         []int{0, 0, 1, 1, 2, 2, 0},
		Delays:          []int{0, 2, 0, 2, 0, 2, 2},
		DT:              0.5,
		TauSyn:          0.7,
		ThetaMin:        0.2,
		ThetaMax:        1.2,
		VReset:          -0.5,
		RefractorySteps: 2,
		Adaptation:      LIFAdaptation{Enabled: true, TauAdapt: 1.1, Beta: 0.6},
		Homeostasis:     &LIFHomeostasis{Enabled: true, TauRate: 3, TargetRate: 0, Eta: 0.2, HMax: 2},
		Surrogate:       LIFSurrogate{Kind: "fast_sigmoid", Scale: 1.7},
	})
	if err != nil {
		t.Fatal(err)
	}
	p := LIFParameters{
		Weights:  []float64{1.1, 0.5, 1.0, 0.4, 0.9, 0.3, 0.7},
		Bias:     []float64{0.2, 0.15, 0.1, 0.05},
		LogTau:   []float64{0, 0.05, -0.05, 0.1},
		ThetaRaw: []float64{-1, -0.8, -0.9, -0.7},
	}
	initial := []float64{0.25, 0.2, 0.15, 0.1}
	inputs := make([][]float64, 11)
	for step := range inputs {
		if step < 8 {
			inputs[step] = []float64{2.4, 2.1, 1.8, 0.2}
		} else {
			inputs[step] = []float64{0.4, 0.35, 0.3, 0.1}
		}
	}
	return m, p, initial, inputs
}

func forwardRecomputeBitsEqual(t *testing.T, label string, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s length %d, want %d", label, len(got), len(want))
	}
	for i := range want {
		if math.Float64bits(got[i]) != math.Float64bits(want[i]) {
			t.Fatalf("%s[%d] got %g (bits %#x), want %g (bits %#x)", label, i, got[i], math.Float64bits(got[i]), want[i], math.Float64bits(want[i]))
		}
	}
}

func forwardRecomputeRowsEqual(t *testing.T, label string, got, want [][]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s row count %d, want %d", label, len(got), len(want))
	}
	for row := range want {
		forwardRecomputeBitsEqual(t, label+" row "+strconv.Itoa(row), got[row], want[row])
	}
}

func forwardRecomputeParamsEqual(t *testing.T, got, want Parameters) {
	t.Helper()
	forwardRecomputeBitsEqual(t, "weights", got.Weights, want.Weights)
	forwardRecomputeBitsEqual(t, "bias", got.Bias, want.Bias)
	forwardRecomputeBitsEqual(t, "log_tau", got.LogTau, want.LogTau)
}

func forwardRecomputeLIFParamsEqual(t *testing.T, got, want LIFParameters) {
	t.Helper()
	forwardRecomputeBitsEqual(t, "weights", got.Weights, want.Weights)
	forwardRecomputeBitsEqual(t, "bias", got.Bias, want.Bias)
	forwardRecomputeBitsEqual(t, "log_tau", got.LogTau, want.LogTau)
	forwardRecomputeBitsEqual(t, "theta_raw", got.ThetaRaw, want.ThetaRaw)
}

func forwardRecomputeInputsEqual(t *testing.T, got, want [][]float64) {
	t.Helper()
	forwardRecomputeRowsEqual(t, "inputs", got, want)
}

func TestForwardRecomputeMatchesForwardBitForBit(t *testing.T) {
	ctx := context.Background()

	t.Run("continuous", func(t *testing.T) {
		m, p, initial, inputs := forwardRecomputeContinuousFixture(t)
		tr, err := m.Forward(ctx, p, initial, inputs)
		if err != nil {
			t.Fatalf("Forward: %v", err)
		}
		want := tr.Outputs()
		wantP := Parameters{Weights: append([]float64(nil), p.Weights...), Bias: append([]float64(nil), p.Bias...), LogTau: append([]float64(nil), p.LogTau...)}
		wantInitial, wantInputs := append([]float64(nil), initial...), cloneRows(inputs)
		for _, segment := range []int{1, 4, 7, len(inputs) + 1, int(^uint(0) >> 1)} {
			got, err := m.ForwardRecompute(ctx, p, initial, inputs, segment)
			if err != nil {
				t.Fatalf("segment %d: ForwardRecompute: %v", segment, err)
			}
			forwardRecomputeRowsEqual(t, "segment "+strconv.Itoa(segment), got, want)
			forwardRecomputeParamsEqual(t, p, wantP)
			forwardRecomputeBitsEqual(t, "initial", initial, wantInitial)
			forwardRecomputeInputsEqual(t, inputs, wantInputs)
		}
	})

	t.Run("lif mechanisms", func(t *testing.T) {
		m, p, initial, inputs := forwardRecomputeLIFFixture(t)
		tr, err := m.Forward(ctx, p, initial, inputs)
		if err != nil {
			t.Fatalf("Forward: %v", err)
		}
		cfg := m.Config()
		if !cfg.Adaptation.Enabled || cfg.Homeostasis == nil || !cfg.Homeostasis.Enabled {
			t.Fatal("fixture must enable adaptation and homeostasis")
		}
		spikeRows, voltageRows := tr.Spikes(), tr.Voltages()
		spikes, refractoryHold := 0, false
		for step, row := range spikeRows {
			for node, spike := range row {
				if spike != 0 {
					spikes++
				}
				if step > 0 && spikeRows[step-1][node] != 0 && spike == 0 && voltageRows[step][node] == cfg.VReset {
					refractoryHold = true
				}
			}
		}
		if spikes == 0 || !refractoryHold {
			t.Fatalf("fixture did not exercise public LIF event/refractory behavior: spikes=%d refractory_hold=%t", spikes, refractoryHold)
		}
		want := tr.Outputs()
		wantP := LIFParameters{Weights: append([]float64(nil), p.Weights...), Bias: append([]float64(nil), p.Bias...), LogTau: append([]float64(nil), p.LogTau...), ThetaRaw: append([]float64(nil), p.ThetaRaw...)}
		wantInitial, wantInputs := append([]float64(nil), initial...), cloneRows(inputs)
		for _, segment := range []int{1, 4, 7, len(inputs) + 1, int(^uint(0) >> 1)} {
			got, err := m.ForwardRecompute(ctx, p, initial, inputs, segment)
			if err != nil {
				t.Fatalf("segment %d: ForwardRecompute: %v", segment, err)
			}
			forwardRecomputeRowsEqual(t, "segment "+strconv.Itoa(segment), got, want)
			forwardRecomputeLIFParamsEqual(t, p, wantP)
			forwardRecomputeBitsEqual(t, "initial", initial, wantInitial)
			forwardRecomputeInputsEqual(t, inputs, wantInputs)
		}
	})
}

// A declared delay can be much larger than this short episode. Recompute must
// retain only the available time-zero prefix instead of allocating from the
// declaration itself.
func TestForwardRecomputeMaxIntDelayUsesFinitePrefix(t *testing.T) {
	ctx := context.Background()
	maxDelay := int(^uint(0) >> 1)

	t.Run("continuous", func(t *testing.T) {
		m, err := NewContinuous(Config{
			Nodes:      1,
			Sources:    []int{0},
			Targets:    []int{0},
			Delays:     []int{maxDelay},
			DT:         0.5,
			Activation: "tanh",
		})
		if err != nil {
			t.Fatal(err)
		}
		p := Parameters{Weights: []float64{0.7}, Bias: []float64{0.1}, LogTau: []float64{0}}
		initial, inputs := []float64{0.25}, [][]float64{{0.3}, {0.4}, {0.2}}
		tr, err := m.Forward(ctx, p, initial, inputs)
		if err != nil {
			t.Fatalf("Forward: %v", err)
		}
		got, err := m.ForwardRecompute(ctx, p, initial, inputs, 1)
		if err != nil {
			t.Fatalf("ForwardRecompute: %v", err)
		}
		forwardRecomputeRowsEqual(t, "continuous", got, tr.Outputs())
	})

	t.Run("lif", func(t *testing.T) {
		m, err := NewLIF(LIFConfig{
			Nodes: 1, Sources: []int{0}, Targets: []int{0}, Delays: []int{maxDelay},
			DT: 0.5, TauSyn: 0.7, ThetaMin: 0.2, ThetaMax: 1.2, VReset: -0.5,
			Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 1.7},
		})
		if err != nil {
			t.Fatal(err)
		}
		p := LIFParameters{Weights: []float64{0.7}, Bias: []float64{0.1}, LogTau: []float64{0}, ThetaRaw: []float64{-0.5}}
		initial, inputs := []float64{0.25}, [][]float64{{0.8}, {0.7}, {0.6}}
		tr, err := m.Forward(ctx, p, initial, inputs)
		if err != nil {
			t.Fatalf("Forward: %v", err)
		}
		got, err := m.ForwardRecompute(ctx, p, initial, inputs, 1)
		if err != nil {
			t.Fatalf("ForwardRecompute: %v", err)
		}
		forwardRecomputeRowsEqual(t, "lif", got, tr.Outputs())
	})
}

func TestForwardRecomputeRejectsNilAndZeroModels(t *testing.T) {
	t.Run("continuous", func(t *testing.T) {
		var nilModel *Continuous
		if got, err := nilModel.ForwardRecompute(context.Background(), Parameters{}, []float64{0}, [][]float64{{0}}, 1); err == nil || got != nil {
			t.Fatalf("nil continuous model got output=%v err=%v", got, err)
		}
		zeroModel := &Continuous{}
		if got, err := zeroModel.ForwardRecompute(context.Background(), Parameters{}, []float64{0}, [][]float64{{0}}, 1); err == nil || got != nil {
			t.Fatalf("zero continuous model got output=%v err=%v", got, err)
		}
	})

	t.Run("lif", func(t *testing.T) {
		var nilModel *LIF
		if got, err := nilModel.ForwardRecompute(context.Background(), LIFParameters{}, []float64{0}, [][]float64{{0}}, 1); err == nil || got != nil {
			t.Fatalf("nil LIF model got output=%v err=%v", got, err)
		}
		zeroModel := &LIF{}
		if got, err := zeroModel.ForwardRecompute(context.Background(), LIFParameters{}, []float64{0}, [][]float64{{0}}, 1); err == nil || got != nil {
			t.Fatalf("zero LIF model got output=%v err=%v", got, err)
		}
	})
}

// Finite parameters can still overflow during a later edge accumulation. The
// public method must discard the partial sequence and preserve every input.
func TestForwardRecomputeRejectsLateOverflowWithoutSideEffects(t *testing.T) {
	t.Run("continuous", func(t *testing.T) {
		m, err := NewContinuous(Config{Nodes: 1, Sources: []int{0, 0}, Targets: []int{0, 0}, Delays: []int{0, 0}, DT: 0.5, Activation: "tanh"})
		if err != nil {
			t.Fatal(err)
		}
		p := Parameters{Weights: []float64{1e308, 1e308}, Bias: []float64{0}, LogTau: []float64{0}}
		initial, inputs := []float64{0.2}, [][]float64{{0}, {0}}
		wantP := Parameters{Weights: append([]float64(nil), p.Weights...), Bias: append([]float64(nil), p.Bias...), LogTau: append([]float64(nil), p.LogTau...)}
		wantInitial, wantInputs := append([]float64(nil), initial...), cloneRows(inputs)
		got, err := m.ForwardRecompute(context.Background(), p, initial, inputs, 1)
		if err == nil || got != nil {
			t.Fatalf("late continuous overflow output=%v err=%v", got, err)
		}
		forwardRecomputeParamsEqual(t, p, wantP)
		forwardRecomputeBitsEqual(t, "initial", initial, wantInitial)
		forwardRecomputeInputsEqual(t, inputs, wantInputs)
	})

	t.Run("lif", func(t *testing.T) {
		m, err := NewLIF(LIFConfig{
			Nodes: 1, Sources: []int{0, 0}, Targets: []int{0, 0}, Delays: []int{0, 0},
			DT: 0.5, TauSyn: 0.7, ThetaMin: 0.2, ThetaMax: 1.2, VReset: -0.5,
			Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 1.7},
		})
		if err != nil {
			t.Fatal(err)
		}
		p := LIFParameters{Weights: []float64{1e308, 1e308}, Bias: []float64{0}, LogTau: []float64{0}, ThetaRaw: []float64{-1}}
		initial, inputs := []float64{0.2}, [][]float64{{2}, {2}}
		wantP := LIFParameters{Weights: append([]float64(nil), p.Weights...), Bias: append([]float64(nil), p.Bias...), LogTau: append([]float64(nil), p.LogTau...), ThetaRaw: append([]float64(nil), p.ThetaRaw...)}
		wantInitial, wantInputs := append([]float64(nil), initial...), cloneRows(inputs)
		got, err := m.ForwardRecompute(context.Background(), p, initial, inputs, 1)
		if err == nil || got != nil {
			t.Fatalf("late LIF overflow output=%v err=%v", got, err)
		}
		forwardRecomputeLIFParamsEqual(t, p, wantP)
		forwardRecomputeBitsEqual(t, "initial", initial, wantInitial)
		forwardRecomputeInputsEqual(t, inputs, wantInputs)
	})
}

func TestContinuousForwardRecomputeRejectsWithoutSideEffects(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*Continuous, *Parameters, *[]float64, *[][]float64) (context.Context, int)
	}{
		{name: "nil context", prepare: func(_ *Continuous, _ *Parameters, _ *[]float64, _ *[][]float64) (context.Context, int) { return nil, 1 }},
		{name: "cancelled context", prepare: func(_ *Continuous, _ *Parameters, _ *[]float64, _ *[][]float64) (context.Context, int) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx, 1
		}},
		{name: "empty sequence", prepare: func(_ *Continuous, _ *Parameters, _ *[]float64, inputs *[][]float64) (context.Context, int) {
			*inputs = nil
			return context.Background(), 1
		}},
		{name: "zero segment", prepare: func(_ *Continuous, _ *Parameters, _ *[]float64, _ *[][]float64) (context.Context, int) {
			return context.Background(), 0
		}},
		{name: "negative segment", prepare: func(_ *Continuous, _ *Parameters, _ *[]float64, _ *[][]float64) (context.Context, int) {
			return context.Background(), -1
		}},
		{name: "initial shape", prepare: func(_ *Continuous, _ *Parameters, initial *[]float64, _ *[][]float64) (context.Context, int) {
			*initial = (*initial)[:3]
			return context.Background(), 1
		}},
		{name: "input shape", prepare: func(_ *Continuous, _ *Parameters, _ *[]float64, inputs *[][]float64) (context.Context, int) {
			(*inputs)[0] = (*inputs)[0][:3]
			return context.Background(), 1
		}},
		{name: "parameter shape", prepare: func(_ *Continuous, p *Parameters, _ *[]float64, _ *[][]float64) (context.Context, int) {
			p.Weights = p.Weights[:len(p.Weights)-1]
			return context.Background(), 1
		}},
		{name: "NaN input", prepare: func(_ *Continuous, _ *Parameters, _ *[]float64, inputs *[][]float64) (context.Context, int) {
			(*inputs)[2][1] = math.NaN()
			return context.Background(), 1
		}},
		{name: "+Inf parameter", prepare: func(_ *Continuous, p *Parameters, _ *[]float64, _ *[][]float64) (context.Context, int) {
			p.LogTau[0] = math.Inf(1)
			return context.Background(), 1
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, p, initial, inputs := forwardRecomputeContinuousFixture(t)
			ctx, segment := tc.prepare(m, &p, &initial, &inputs)
			wantP := Parameters{
				Weights: append([]float64(nil), p.Weights...),
				Bias:    append([]float64(nil), p.Bias...),
				LogTau:  append([]float64(nil), p.LogTau...),
			}
			wantInitial, wantInputs := append([]float64(nil), initial...), cloneRows(inputs)
			got, err := m.ForwardRecompute(ctx, p, initial, inputs, segment)
			if err == nil {
				t.Fatal("ForwardRecompute unexpectedly succeeded")
			}
			if tc.name == "cancelled context" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled context error %v, want context.Canceled", err)
			}
			if got != nil {
				t.Fatalf("failed call returned %d output rows, want nil", len(got))
			}
			forwardRecomputeParamsEqual(t, p, wantP)
			forwardRecomputeBitsEqual(t, "initial", initial, wantInitial)
			forwardRecomputeInputsEqual(t, inputs, wantInputs)
		})
	}
}

func TestLIFForwardRecomputeRejectsWithoutSideEffects(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare func(*LIF, *LIFParameters, *[]float64, *[][]float64) (context.Context, int)
	}{
		{name: "nil context", prepare: func(_ *LIF, _ *LIFParameters, _ *[]float64, _ *[][]float64) (context.Context, int) { return nil, 1 }},
		{name: "cancelled context", prepare: func(_ *LIF, _ *LIFParameters, _ *[]float64, _ *[][]float64) (context.Context, int) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx, 1
		}},
		{name: "empty sequence", prepare: func(_ *LIF, _ *LIFParameters, _ *[]float64, inputs *[][]float64) (context.Context, int) {
			*inputs = nil
			return context.Background(), 1
		}},
		{name: "zero segment", prepare: func(_ *LIF, _ *LIFParameters, _ *[]float64, _ *[][]float64) (context.Context, int) {
			return context.Background(), 0
		}},
		{name: "negative segment", prepare: func(_ *LIF, _ *LIFParameters, _ *[]float64, _ *[][]float64) (context.Context, int) {
			return context.Background(), -1
		}},
		{name: "initial shape", prepare: func(_ *LIF, _ *LIFParameters, initial *[]float64, _ *[][]float64) (context.Context, int) {
			*initial = (*initial)[:3]
			return context.Background(), 1
		}},
		{name: "input shape", prepare: func(_ *LIF, _ *LIFParameters, _ *[]float64, inputs *[][]float64) (context.Context, int) {
			(*inputs)[0] = (*inputs)[0][:3]
			return context.Background(), 1
		}},
		{name: "parameter shape", prepare: func(_ *LIF, p *LIFParameters, _ *[]float64, _ *[][]float64) (context.Context, int) {
			p.ThetaRaw = p.ThetaRaw[:len(p.ThetaRaw)-1]
			return context.Background(), 1
		}},
		{name: "NaN input", prepare: func(_ *LIF, _ *LIFParameters, _ *[]float64, inputs *[][]float64) (context.Context, int) {
			(*inputs)[2][1] = math.NaN()
			return context.Background(), 1
		}},
		{name: "-Inf parameter", prepare: func(_ *LIF, p *LIFParameters, _ *[]float64, _ *[][]float64) (context.Context, int) {
			p.LogTau[0] = math.Inf(-1)
			return context.Background(), 1
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, p, initial, inputs := forwardRecomputeLIFFixture(t)
			ctx, segment := tc.prepare(m, &p, &initial, &inputs)
			wantP := LIFParameters{
				Weights:  append([]float64(nil), p.Weights...),
				Bias:     append([]float64(nil), p.Bias...),
				LogTau:   append([]float64(nil), p.LogTau...),
				ThetaRaw: append([]float64(nil), p.ThetaRaw...),
			}
			wantInitial, wantInputs := append([]float64(nil), initial...), cloneRows(inputs)
			got, err := m.ForwardRecompute(ctx, p, initial, inputs, segment)
			if err == nil {
				t.Fatal("ForwardRecompute unexpectedly succeeded")
			}
			if tc.name == "cancelled context" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled context error %v, want context.Canceled", err)
			}
			if got != nil {
				t.Fatalf("failed call returned %d output rows, want nil", len(got))
			}
			// The public method receives these slices by value but must not edit
			// their backing arrays on a failed validation or cancellation.
			forwardRecomputeLIFParamsEqual(t, p, wantP)
			forwardRecomputeBitsEqual(t, "initial", initial, wantInitial)
			forwardRecomputeInputsEqual(t, inputs, wantInputs)
		})
	}
}
