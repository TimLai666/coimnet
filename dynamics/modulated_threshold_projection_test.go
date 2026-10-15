package dynamics

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"testing"
)

func thresholdExtremeLIF(t *testing.T) (*LIF, LIFParameters, []float64, [][]float64) {
	t.Helper()
	thetaMin := -math.Ldexp(1, 1023)
	m, err := NewLIF(LIFConfig{
		Nodes: 1, DT: math.Ln2, TauSyn: 1, ThetaMin: thetaMin, ThetaMax: 0,
		VReset: -math.MaxFloat64, Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	p := LIFParameters{Bias: []float64{0}, LogTau: []float64{0}, ThetaRaw: []float64{0}}
	return m, p, []float64{thetaMin}, [][]float64{{thetaMin}}
}

func thresholdMixed(t *testing.T, thetaMin, thetaMax, vReset float64) (*Mixed, MixedParameters, []float64, [][]float64) {
	t.Helper()
	m, err := NewMixed(MixedConfig{
		Nodes: 2, DT: math.Ln2, NodeRule: []uint8{RuleLIF, RuleContinuous},
		Continuous: ContinuousRule{Activation: "tanh"},
		LIF:        LIFRule{TauSyn: 1, ThetaMin: thetaMin, ThetaMax: thetaMax, VReset: vReset, Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	p := MixedParameters{Bias: []float64{0, 0}, LogTau: []float64{0, 0}, ThetaRaw: []float64{0}}
	return m, p, []float64{0, 0}, [][]float64{{0, 0}}
}

func thresholdProjectionReference(t *testing.T, thetaMin, thetaMax, shift float64) (sum, projected *big.Rat) {
	t.Helper()
	min := new(big.Rat).SetFloat64(thetaMin)
	max := new(big.Rat).SetFloat64(thetaMax)
	shiftRat := new(big.Rat).SetFloat64(shift)
	if min == nil || max == nil || shiftRat == nil {
		t.Fatal("reference inputs must be finite")
	}
	half := big.NewRat(1, 2) // logistic(0), the raw threshold used by the fixture.
	base := new(big.Rat).Add(new(big.Rat).Mul(min, half), new(big.Rat).Mul(max, half))
	sum = new(big.Rat).Add(base, shiftRat)
	projected = new(big.Rat).Set(sum)
	if projected.Cmp(min) < 0 {
		projected.Set(min)
	}
	return sum, projected
}

func assertThresholdFinite(t *testing.T, name string, vectors ...[]float64) {
	t.Helper()
	for vector, values := range vectors {
		for i, value := range values {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				t.Fatalf("%s[%d][%d] = %v, want finite", name, vector, i, value)
			}
		}
	}
}

func assertThresholdFiniteRows(t *testing.T, name string, rows [][]float64) {
	t.Helper()
	for row, values := range rows {
		assertThresholdFinite(t, fmt.Sprintf("%s[%d]", name, row), values)
	}
}

func assertLIFThresholdGradientFinite(t *testing.T, g LIFGradient) {
	t.Helper()
	assertThresholdFinite(t, "LIF gradient", g.Weights, g.Bias, g.LogTau, g.ThetaRaw, g.Initial)
	assertThresholdFiniteRows(t, "LIF input gradient", g.Inputs)
}

func assertMixedThresholdGradientFinite(t *testing.T, g MixedGradient) {
	t.Helper()
	assertThresholdFinite(t, "mixed gradient", g.Weights, g.Bias, g.LogTau, g.ThetaRaw, g.Initial)
	assertThresholdFiniteRows(t, "mixed input gradient", g.Inputs)
}

// The existing lifCoupledSequence is the forward/reverse double. This wrapper
// only supplies the one checkpoint needed by a one-step recompute fixture.
type thresholdRecomputeSequence struct{ *lifCoupledSequence }

func (s *thresholdRecomputeSequence) Checkpoint(ctx context.Context, nextStep int) (ModulationRestart, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil checkpoint context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.lifCoupledSequence == nil || nextStep != 0 {
		return nil, fmt.Errorf("one-step checkpoint boundary is invalid")
	}
	mods := s.mods
	return func(restartCtx context.Context) (ModulationSequence, error) {
		if restartCtx == nil {
			return nil, fmt.Errorf("nil restart context")
		}
		if err := restartCtx.Err(); err != nil {
			return nil, err
		}
		return &thresholdRecomputeSequence{lifCoupledSequence: &lifCoupledSequence{mods: mods, reverseErrAt: -1}}, nil
	}, nil
}

func thresholdRecomputeModulation(shift float64) *thresholdRecomputeSequence {
	return &thresholdRecomputeSequence{lifCoupledSequence: &lifCoupledSequence{
		mods: []*Modulation{{Threshold: [][]float64{{shift}}}}, reverseErrAt: -1,
	}}
}

func TestThresholdProjectionNegativeOverflowAcrossCoupledPaths(t *testing.T) {
	thetaMin := -math.Ldexp(1, 1023)
	shift := -math.MaxFloat64
	sum, projected := thresholdProjectionReference(t, thetaMin, 0, shift)
	minRat := new(big.Rat).SetFloat64(thetaMin)
	if sum.Cmp(minRat) >= 0 {
		t.Fatalf("reference base+shift = %s, want below theta_min", sum.RatString())
	}
	if sumFloat, _ := sum.Float64(); !math.IsInf(sumFloat, -1) {
		t.Fatalf("float64(base+shift) = %v, want negative overflow", sumFloat)
	}
	if projected.Cmp(minRat) != 0 {
		t.Fatalf("reference projection = %s, want theta_min", projected.RatString())
	}
	projectedFloat, _ := projected.Float64()
	if projectedFloat != thetaMin || math.IsInf(projectedFloat, 0) || math.IsNaN(projectedFloat) {
		t.Fatalf("projected float = %v, want finite theta_min %v", projectedFloat, thetaMin)
	}

	t.Run("LIF ForwardCoupled", func(t *testing.T) {
		m, p, initial, inputs := thresholdExtremeLIF(t)
		seq := &lifCoupledSequence{mods: []*Modulation{{Threshold: [][]float64{{shift}}}}, reverseErrAt: -1}
		tr, err := m.ForwardCoupled(context.Background(), p, initial, inputs, seq)
		if err != nil || tr == nil {
			t.Fatalf("ForwardCoupled error=%v trace=%v, want finite projected trace", err, tr)
		}
		if tr.cand[0][0] != thetaMin || tr.dspike[0][0] != 1 || !tr.coupled.thresholdFloor[0][0] {
			t.Fatalf("candidate=%v surrogate=%v floor=%v, want theta_min, 1, true", tr.cand[0][0], tr.dspike[0][0], tr.coupled.thresholdFloor[0][0])
		}
		if tr.Spikes()[0][0] != 1 || tr.FinalVoltage()[0] != -math.MaxFloat64 {
			t.Fatalf("spike/final voltage = %v/%v, want 1/%v", tr.Spikes()[0][0], tr.FinalVoltage()[0], -math.MaxFloat64)
		}
		g, err := m.Backward(context.Background(), tr, [][]float64{{1}}, 0)
		if err != nil {
			t.Fatal(err)
		}
		assertLIFThresholdGradientFinite(t, g)
		if len(g.ThetaRaw) != 1 {
			t.Fatalf("ThetaRaw gradient length = %d, want 1", len(g.ThetaRaw))
			return
		}
		if len(seq.reverses) != 1 {
			t.Fatalf("reverse sessions = %d, want 1", len(seq.reverses))
			return
		}
		if len(seq.reverses[0].gradients) != 1 {
			t.Fatalf("reverse gradient rows = %d, want 1", len(seq.reverses[0].gradients))
			return
		}
		if len(seq.reverses[0].gradients[0].Threshold) != 1 {
			t.Fatalf("threshold gradient length = %d, want 1", len(seq.reverses[0].gradients[0].Threshold))
			return
		}
		if g.ThetaRaw[0] != 0 || seq.reverses[0].gradients[0].Threshold[0] != 0 {
			t.Fatalf("theta gradients = raw %v, modulation %v, want both zero", g.ThetaRaw, seq.reverses[0].gradients)
		}
	})

	t.Run("Mixed ForwardCoupled", func(t *testing.T) {
		m, p, _, _ := thresholdMixed(t, thetaMin, 0, -math.MaxFloat64)
		initial, inputs := []float64{thetaMin, 0}, [][]float64{{thetaMin, 0}}
		seq := &mixedCoupledSequence{mods: []*Modulation{{Threshold: [][]float64{{shift, 0}}}}, reverseErrAt: -1}
		tr, err := m.ForwardCoupled(context.Background(), p, initial, inputs, seq)
		if err != nil || tr == nil {
			t.Fatalf("ForwardCoupled error=%v trace=%v, want finite projected trace", err, tr)
		}
		if tr.cand[0][0] != thetaMin || tr.dspike[0][0] != 1 || !tr.coupled.thresholdFloor[0][0] {
			t.Fatalf("mixed candidate=%v surrogate=%v floor=%v, want theta_min, 1, true", tr.cand[0][0], tr.dspike[0][0], tr.coupled.thresholdFloor[0][0])
		}
		if tr.Spikes()[0][0] != 1 || tr.Spikes()[0][1] != 0 {
			t.Fatalf("mixed spikes = %v, want [1 0]", tr.Spikes()[0])
		}
		g, err := m.Backward(context.Background(), tr, [][]float64{{1, 0}}, 0)
		if err != nil {
			t.Fatal(err)
		}
		assertMixedThresholdGradientFinite(t, g)
		if len(g.ThetaRaw) != 1 {
			t.Fatalf("mixed ThetaRaw gradient length = %d, want 1", len(g.ThetaRaw))
			return
		}
		if len(seq.reverses) != 1 {
			t.Fatalf("mixed reverse sessions = %d, want 1", len(seq.reverses))
			return
		}
		if len(seq.reverses[0].gradients) != 1 {
			t.Fatalf("mixed reverse gradient rows = %d, want 1", len(seq.reverses[0].gradients))
			return
		}
		if len(seq.reverses[0].gradients[0].Threshold) != 2 {
			t.Fatalf("mixed threshold gradient length = %d, want 2", len(seq.reverses[0].gradients[0].Threshold))
			return
		}
		if g.ThetaRaw[0] != 0 || seq.reverses[0].gradients[0].Threshold[0] != 0 || seq.reverses[0].gradients[0].Threshold[1] != 0 {
			t.Fatalf("mixed theta gradients = raw %v, modulation %v, want zero", g.ThetaRaw, seq.reverses[0].gradients[0])
		}
	})

	t.Run("LIF ForwardCoupledRecompute", func(t *testing.T) {
		m, p, initial, inputs := thresholdExtremeLIF(t)
		seq := thresholdRecomputeModulation(shift)
		tr, err := m.ForwardCoupledRecompute(context.Background(), p, initial, inputs, seq, 1)
		if err != nil || tr == nil {
			t.Fatalf("ForwardCoupledRecompute error=%v trace=%v, want finite projected trace", err, tr)
		}
		if tr.Outputs()[0][0] != 1 || tr.FinalVoltage()[0] != -math.MaxFloat64 {
			t.Fatalf("recompute output/final voltage = %v/%v, want 1/%v", tr.Outputs()[0][0], tr.FinalVoltage()[0], -math.MaxFloat64)
		}
		g, err := m.BackwardCoupledRecompute(context.Background(), tr, [][]float64{{1}}, 0)
		if err != nil {
			t.Fatal(err)
		}
		assertLIFThresholdGradientFinite(t, g)
		if len(g.ThetaRaw) != 1 {
			t.Fatalf("recompute ThetaRaw gradient length = %d, want 1", len(g.ThetaRaw))
			return
		}
		if len(seq.reverses) != 1 {
			t.Fatalf("recompute reverse sessions = %d, want 1", len(seq.reverses))
			return
		}
		if len(seq.reverses[0].gradients) != 1 {
			t.Fatalf("recompute reverse gradient rows = %d, want 1", len(seq.reverses[0].gradients))
			return
		}
		if len(seq.reverses[0].gradients[0].Threshold) != 1 {
			t.Fatalf("recompute threshold gradient length = %d, want 1", len(seq.reverses[0].gradients[0].Threshold))
			return
		}
		if g.ThetaRaw[0] != 0 || seq.reverses[0].gradients[0].Threshold[0] != 0 {
			t.Fatalf("recompute theta gradients = raw %v, modulation %v, want zero", g.ThetaRaw, seq.reverses[0].gradients[0])
		}
	})
}

func TestThresholdProjectionControlsAndRejections(t *testing.T) {
	t.Run("ordinary AdvanceModulated negative overflow", func(t *testing.T) {
		m, p, initial, inputs := thresholdExtremeLIF(t)
		state, err := m.NewState(initial)
		if err != nil {
			t.Fatal(err)
		}
		mod := &Modulation{Threshold: [][]float64{{-math.MaxFloat64}}}
		final, outputs, spikes, err := m.AdvanceModulated(context.Background(), p, state, inputs, mod)
		if err != nil {
			t.Fatal(err)
		}
		if outputs[0][0] != 1 || spikes[0][0] != 1 || final.Voltage[0] != -math.MaxFloat64 {
			t.Fatalf("ordinary output/spike/final voltage = %v/%v/%v, want 1/1/%v", outputs[0][0], spikes[0][0], final.Voltage[0], -math.MaxFloat64)
		}
	})

	m, err := NewLIF(LIFConfig{Nodes: 1, DT: math.Ln2, TauSyn: 1, ThetaMin: -1, ThetaMax: 1, VReset: -2, Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 1}})
	if err != nil {
		t.Fatal(err)
	}
	p := LIFParameters{Bias: []float64{0}, LogTau: []float64{0}, ThetaRaw: []float64{0}}
	for _, tc := range []struct {
		name  string
		shift float64
	}{
		{name: "finite exact floor", shift: -1},
		{name: "finite no floor", shift: -.25},
		{name: "zero shift", shift: 0},
		{name: "finite positive shift", shift: .25},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			const candidate = -.5 // lambda=alpha=.5, initial=0, input=-1.
			sum, projected := thresholdProjectionReference(t, -1, 1, tc.shift)
			minRat := new(big.Rat).SetFloat64(-1)
			wantTheta, _ := projected.Float64()
			wantFloor := sum.Cmp(minRat) <= 0
			u := candidate - wantTheta
			psi := 1 / math.Pow(1+math.Abs(u), 2)
			wantSpike := float64(0)
			if u >= 0 {
				wantSpike = 1
			}
			seq := &lifCoupledSequence{mods: []*Modulation{{Threshold: [][]float64{{tc.shift}}}}, reverseErrAt: -1}
			tr, err := m.ForwardCoupled(context.Background(), p, []float64{0}, [][]float64{{-1}}, seq)
			if err != nil || tr == nil {
				t.Fatalf("ForwardCoupled error=%v trace=%v", err, tr)
			}
			if tr.cand[0][0] != candidate || tr.coupled.thresholdFloor[0][0] != wantFloor {
				t.Fatalf("candidate/floor = %v/%v, want %v/%v (projected theta %v)", tr.cand[0][0], tr.coupled.thresholdFloor[0][0], candidate, wantFloor, wantTheta)
			}
			if math.Abs(tr.dspike[0][0]-psi) > 1e-12 {
				t.Fatalf("surrogate psi = %.17g, want %.17g", tr.dspike[0][0], psi)
			}
			if tr.Spikes()[0][0] != wantSpike {
				t.Fatalf("spike = %v, want %v", tr.Spikes()[0][0], wantSpike)
			}
			g, err := m.Backward(context.Background(), tr, [][]float64{{1}}, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(g.ThetaRaw) != 1 {
				t.Fatalf("ThetaRaw gradient length = %d, want 1", len(g.ThetaRaw))
				return
			}
			if len(seq.reverses) != 1 {
				t.Fatalf("reverse sessions = %d, want 1", len(seq.reverses))
				return
			}
			if len(seq.reverses[0].gradients) != 1 {
				t.Fatalf("reverse gradient rows = %d, want 1", len(seq.reverses[0].gradients))
				return
			}
			row := seq.reverses[0].gradients[0]
			if len(row.Threshold) != 1 {
				t.Fatalf("threshold gradient length = %d, want 1", len(row.Threshold))
				return
			}
			assertLIFThresholdGradientFinite(t, g)
			wantThetaGradient, wantRawGradient := -psi, -.5*psi
			// Existing coupled reverse semantics treat a zero shift as neutral
			// arithmetic without a floor marker, so its derivative remains live.
			if wantFloor {
				wantThetaGradient, wantRawGradient = 0, 0
			}
			if math.Abs(row.Threshold[0]-wantThetaGradient) > 1e-12 || math.Abs(g.ThetaRaw[0]-wantRawGradient) > 1e-12 {
				t.Fatalf("threshold gradients = modulation %.17g raw %.17g, want %.17g/%.17g", row.Threshold[0], g.ThetaRaw[0], wantThetaGradient, wantRawGradient)
			}
		})
	}

	overflowModel, err := NewLIF(LIFConfig{Nodes: 1, DT: math.Ln2, TauSyn: 1, ThetaMin: -1, ThetaMax: math.MaxFloat64, VReset: -2, Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 1}})
	if err != nil {
		t.Fatal(err)
	}
	normalMixed, mixedP, mixedInitial, mixedInputs := thresholdMixed(t, -1, 1, -2)
	overflowMixed, _, _, _ := thresholdMixed(t, -1, math.MaxFloat64, -2)
	for _, tc := range []struct {
		name  string
		model *LIF
		mixed *Mixed
		shift float64
	}{
		{name: "positive finite overflow", model: overflowModel, mixed: overflowMixed, shift: math.MaxFloat64},
		{name: "NaN", model: m, mixed: normalMixed, shift: math.NaN()},
		{name: "positive Inf", model: m, mixed: normalMixed, shift: math.Inf(1)},
		{name: "negative Inf", model: m, mixed: normalMixed, shift: math.Inf(-1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lifSeq := &lifCoupledSequence{mods: []*Modulation{{Threshold: [][]float64{{tc.shift}}}}, reverseErrAt: -1}
			lifTrace, lifErr := tc.model.ForwardCoupled(context.Background(), p, []float64{0}, [][]float64{{0}}, lifSeq)
			mixedSeq := &mixedCoupledSequence{mods: []*Modulation{{Threshold: [][]float64{{tc.shift, 0}}}}, reverseErrAt: -1}
			mixedTrace, mixedErr := tc.mixed.ForwardCoupled(context.Background(), mixedP, mixedInitial, mixedInputs, mixedSeq)
			if lifErr == nil || lifTrace != nil {
				t.Errorf("LIF trace=%v error=%v, want nil trace and rejection", lifTrace, lifErr)
			}
			if mixedErr == nil || mixedTrace != nil {
				t.Errorf("Mixed trace=%v error=%v, want nil trace and rejection", mixedTrace, mixedErr)
			}
		})
	}
}
