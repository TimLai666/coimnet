package dynamics

import (
	"context"
	"math"
	"math/big"
	"testing"
)

type eventAdjointReference struct {
	weights, bias, logTau, thetaRaw, initial []float64
	inputs                                   [][]float64
}

func eventAdjointRat(value float64) *big.Rat {
	return new(big.Rat).SetFloat64(value)
}

func eventAdjointMul(left, right *big.Rat) *big.Rat {
	return new(big.Rat).Mul(left, right)
}

func eventAdjointAdd(left, right *big.Rat) *big.Rat {
	return new(big.Rat).Add(left, right)
}

func eventAdjointFloat(value *big.Rat) float64 {
	result, _ := value.Float64()
	return result
}

// eventAdjointReferenceFor is the fixed hand calculation for the three rows.
// The event adjoints are composed as rationals before conversion, so the
// nonrepresentable intermediate (1-2*beta) is never formed as a float.
func eventAdjointReferenceFor(adaptation bool) eventAdjointReference {
	half := eventAdjointRat(.5)
	firstPsi := eventAdjointRat(.25)
	finalDu := eventAdjointRat(0)
	if adaptation {
		finalDu = eventAdjointRat(4)
	}
	firstAdjoint := eventAdjointRat(1)
	if adaptation {
		firstAdjoint = eventAdjointAdd(firstAdjoint, eventAdjointMul(eventAdjointRat(-2), eventAdjointRat(1e308)))
	}
	firstDu := eventAdjointMul(firstAdjoint, firstPsi)
	firstInput := eventAdjointMul(firstDu, half)
	finalInput := eventAdjointMul(finalDu, half)
	dlambda := eventAdjointMul(half, eventAdjointRat(math.Log(2)))
	logTau := eventAdjointAdd(
		eventAdjointMul(eventAdjointMul(firstDu, eventAdjointRat(-3)), dlambda),
		eventAdjointMul(eventAdjointMul(finalDu, eventAdjointAdd(eventAdjointRat(-1), eventAdjointRat(-1e308))), dlambda),
	)
	theta := eventAdjointMul(eventAdjointRat(-.5), eventAdjointAdd(firstDu, finalDu))
	return eventAdjointReference{
		weights:  []float64{eventAdjointFloat(finalInput)},
		bias:     []float64{eventAdjointFloat(eventAdjointAdd(firstInput, finalInput))},
		logTau:   []float64{eventAdjointFloat(logTau)},
		thetaRaw: []float64{eventAdjointFloat(theta)},
		inputs: [][]float64{
			{eventAdjointFloat(firstInput)},
			{0},
			{eventAdjointFloat(finalInput)},
		},
		initial: []float64{eventAdjointFloat(firstInput)},
	}
}

func eventAdjointTruncatedFinalReference() eventAdjointReference {
	half := eventAdjointRat(.5)
	finalDu := eventAdjointRat(4)
	finalInput := eventAdjointMul(finalDu, half)
	dlambda := eventAdjointMul(half, eventAdjointRat(math.Log(2)))
	logTau := eventAdjointMul(eventAdjointMul(finalDu, eventAdjointAdd(eventAdjointRat(-1), eventAdjointRat(-1e308))), dlambda)
	return eventAdjointReference{
		weights:  []float64{eventAdjointFloat(finalInput)},
		bias:     []float64{eventAdjointFloat(finalInput)},
		logTau:   []float64{eventAdjointFloat(logTau)},
		thetaRaw: []float64{-2},
		inputs:   [][]float64{{0}, {0}, {eventAdjointFloat(finalInput)}},
		initial:  []float64{0},
	}
}

func eventAdjointSmallSurrogateReference() eventAdjointReference {
	half := eventAdjointRat(.5)
	firstDu := eventAdjointRat(1.0 / 8.0)
	secondDu := eventAdjointRat(1.0 / 16.0)
	dlambda := eventAdjointMul(half, eventAdjointRat(math.Log(2)))
	logTau := eventAdjointAdd(
		eventAdjointMul(eventAdjointMul(firstDu, eventAdjointRat(-3)), dlambda),
		eventAdjointMul(eventAdjointMul(secondDu, eventAdjointRat(-1)), dlambda),
	)
	return eventAdjointReference{
		weights:  []float64{0},
		bias:     []float64{3.0 / 32.0},
		logTau:   []float64{eventAdjointFloat(logTau)},
		thetaRaw: []float64{-3.0 / 32.0},
		inputs:   [][]float64{{1.0 / 16.0}, {1.0 / 32.0}, {0}},
		initial:  []float64{1.0 / 16.0},
	}
}

func eventAdjointLIFFixture(t *testing.T, adaptation bool) (*LIF, LIFParameters, []float64, [][]float64, [][]float64) {
	t.Helper()
	m, err := NewLIF(LIFConfig{
		Nodes: 1, Sources: []int{0}, Targets: []int{0}, Delays: []int{1},
		DT: math.Log(2), TauSyn: 1, ThetaMin: 0, ThetaMax: 2, VReset: -1,
		RefractorySteps: 1,
		Adaptation:      LIFAdaptation{Enabled: adaptation, TauAdapt: 1, Beta: 1e308},
		Surrogate:       LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	p := LIFParameters{Weights: []float64{0}, Bias: []float64{0}, LogTau: []float64{0}, ThetaRaw: []float64{0}}
	return m, p, []float64{0}, [][]float64{{3}, {0}, {1e308}}, [][]float64{{0}, {0}, {4}}
}

func eventAdjointAssertLIFForward(t *testing.T, outputs, spikes [][]float64) {
	t.Helper()
	if len(outputs) != 3 || len(spikes) != 3 {
		t.Fatalf("forward rows outputs=%d spikes=%d, want 3", len(outputs), len(spikes))
	}
	wantOutputs := []float64{1, .5, 1.25}
	wantSpikes := []float64{1, 0, 1}
	for step := 0; step < 3; step++ {
		if len(outputs[step]) != 1 || len(spikes[step]) != 1 {
			t.Fatalf("forward row %d lengths outputs=%d spikes=%d, want 1", step, len(outputs[step]), len(spikes[step]))
		}
		if outputs[step][0] != wantOutputs[step] || spikes[step][0] != wantSpikes[step] {
			t.Fatalf("forward row %d outputs=%v spikes=%v", step, outputs[step], spikes[step])
		}
	}
}

func eventAdjointAssertRows(t *testing.T, outputs [][]float64) {
	t.Helper()
	if len(outputs) != 3 {
		t.Fatalf("output rows=%d, want 3", len(outputs))
	}
	for step, row := range outputs {
		if len(row) != 1 || row[0] != []float64{1, .5, 1.25}[step] {
			t.Fatalf("output row %d=%v, want one of [1 .5 1.25]", step, row)
		}
	}
}

func eventAdjointNear(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Fatalf("%s=%v, want finite %v", name, got, want)
	}
	limit := 1e-15 * math.Max(1, math.Abs(want))
	if math.Abs(got-want) > limit {
		t.Fatalf("%s=%v, want %v (limit %v)", name, got, want, limit)
	}
}

func eventAdjointAssertFields(t *testing.T, name string, weights, bias, logTau, thetaRaw, initial []float64, inputs [][]float64, want eventAdjointReference) {
	t.Helper()
	if len(weights) != 1 || len(bias) != 1 || len(logTau) != 1 || len(thetaRaw) != 1 || len(initial) != 1 || len(inputs) != 3 {
		t.Fatalf("%s gradient lengths weights=%d bias=%d log_tau=%d theta=%d initial=%d inputs=%d", name, len(weights), len(bias), len(logTau), len(thetaRaw), len(initial), len(inputs))
	}
	for step, row := range inputs {
		if len(row) != 1 || len(want.inputs[step]) != 1 {
			t.Fatalf("%s input row %d lengths got=%d want=%d", name, step, len(row), len(want.inputs[step]))
		}
	}
	eventAdjointNear(t, name+" weights", weights[0], want.weights[0])
	eventAdjointNear(t, name+" bias", bias[0], want.bias[0])
	eventAdjointNear(t, name+" log_tau", logTau[0], want.logTau[0])
	eventAdjointNear(t, name+" theta_raw", thetaRaw[0], want.thetaRaw[0])
	eventAdjointNear(t, name+" initial", initial[0], want.initial[0])
	for step := range inputs {
		eventAdjointNear(t, name+" input", inputs[step][0], want.inputs[step][0])
	}
}

func eventAdjointAssertLIFGradient(t *testing.T, g LIFGradient, want eventAdjointReference) {
	t.Helper()
	eventAdjointAssertFields(t, "LIF", g.Weights, g.Bias, g.LogTau, g.ThetaRaw, g.Initial, g.Inputs, want)
}

func eventAdjointAssertMixedGradient(t *testing.T, g MixedGradient, want eventAdjointReference) {
	t.Helper()
	eventAdjointAssertFields(t, "Mixed", g.Weights, g.Bias, g.LogTau, g.ThetaRaw, g.Initial, g.Inputs, want)
}

func eventAdjointMixedLIFFixture(t *testing.T) (*Mixed, MixedParameters, []float64, [][]float64, [][]float64) {
	t.Helper()
	m, err := NewMixed(MixedConfig{
		Nodes: 1, Sources: []int{0}, Targets: []int{0}, Delays: []int{1}, DT: math.Log(2),
		NodeRule: []uint8{RuleLIF},
		LIF: LIFRule{TauSyn: 1, ThetaMin: 0, ThetaMax: 2, VReset: -1, RefractorySteps: 1,
			Adaptation: LIFAdaptation{Enabled: true, TauAdapt: 1, Beta: 1e308},
			Surrogate:  LIFSurrogate{Kind: "fast_sigmoid", Scale: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	p := MixedParameters{Weights: []float64{0}, Bias: []float64{0}, LogTau: []float64{0}, ThetaRaw: []float64{0}}
	return m, p, []float64{0}, [][]float64{{3}, {0}, {1e308}}, [][]float64{{0}, {0}, {4}}
}

func TestEventAdjointLIF(t *testing.T) {
	want := eventAdjointReferenceFor(true)
	t.Run("Backward", func(t *testing.T) {
		m, p, initial, inputs, upstream := eventAdjointLIFFixture(t, true)
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			t.Fatal(err)
		}
		eventAdjointAssertLIFForward(t, tr.Outputs(), tr.Spikes())
		g, err := m.Backward(context.Background(), tr, upstream, 0)
		if err != nil {
			t.Fatal(err)
		}
		eventAdjointAssertLIFGradient(t, g, want)
	})
	for _, segment := range []int{1, 2} {
		segment := segment
		t.Run("BackwardRecompute/segment="+string(rune('0'+segment)), func(t *testing.T) {
			m, p, initial, inputs, upstream := eventAdjointLIFFixture(t, true)
			g, err := m.BackwardRecompute(context.Background(), p, initial, inputs, upstream, 0, segment)
			if err != nil {
				t.Fatal(err)
			}
			eventAdjointAssertLIFGradient(t, g, want)
		})
	}
	t.Run("BackwardCoupled", func(t *testing.T) {
		m, p, initial, inputs, upstream := eventAdjointLIFFixture(t, true)
		inputs[2][0] = 0
		mods := zeroTauMods(3)
		// Use a known offset for the same final effective drive, avoiding an unrepresentable gain gradient.
		mods[2].Offset = [][]float64{{1e308}}
		seq := &zeroTauSequence{mods: mods, nodes: 1}
		tr, err := m.ForwardCoupled(context.Background(), p, initial, inputs, seq)
		if err != nil {
			t.Fatal(err)
		}
		eventAdjointAssertLIFForward(t, tr.Outputs(), tr.Spikes())
		g, err := m.Backward(context.Background(), tr, upstream, 0)
		if err != nil {
			t.Fatal(err)
		}
		eventAdjointAssertLIFGradient(t, g, want)
	})
	t.Run("BackwardCoupledRecompute", func(t *testing.T) {
		m, p, initial, inputs, upstream := eventAdjointLIFFixture(t, true)
		inputs[2][0] = 0
		mods := zeroTauMods(3)
		mods[2].Offset = [][]float64{{1e308}}
		seq := &zeroTauRecomputeSequence{mods: mods, nodes: 1}
		tr, err := m.ForwardCoupledRecompute(context.Background(), p, initial, inputs, seq, 1)
		if err != nil {
			t.Fatal(err)
		}
		eventAdjointAssertRows(t, tr.Outputs())
		g, err := m.BackwardCoupledRecompute(context.Background(), tr, upstream, 0)
		if err != nil {
			t.Fatal(err)
		}
		eventAdjointAssertLIFGradient(t, g, want)
	})
}

func TestEventAdjointMixed(t *testing.T) {
	want := eventAdjointReferenceFor(true)
	t.Run("all-LIF/Backward", func(t *testing.T) {
		m, p, initial, inputs, upstream := eventAdjointMixedLIFFixture(t)
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			t.Fatal(err)
		}
		eventAdjointAssertLIFForward(t, tr.Outputs(), tr.Spikes())
		g, err := m.Backward(context.Background(), tr, upstream, 0)
		if err != nil {
			t.Fatal(err)
		}
		eventAdjointAssertMixedGradient(t, g, want)
	})
	t.Run("all-LIF/BackwardCoupled", func(t *testing.T) {
		m, p, initial, inputs, upstream := eventAdjointMixedLIFFixture(t)
		inputs[2][0] = 0
		mods := zeroTauMods(3)
		mods[2].Offset = [][]float64{{1e308}}
		seq := &zeroTauSequence{mods: mods, nodes: 1}
		tr, err := m.ForwardCoupled(context.Background(), p, initial, inputs, seq)
		if err != nil {
			t.Fatal(err)
		}
		eventAdjointAssertLIFForward(t, tr.Outputs(), tr.Spikes())
		g, err := m.Backward(context.Background(), tr, upstream, 0)
		if err != nil {
			t.Fatal(err)
		}
		eventAdjointAssertMixedGradient(t, g, want)
	})
}

func TestEventAdjointMixedContinuousLIF(t *testing.T) {
	m, err := NewMixed(MixedConfig{
		Nodes: 2, Sources: []int{1}, Targets: []int{1}, Delays: []int{1}, DT: math.Log(2),
		NodeRule: []uint8{RuleContinuous, RuleLIF}, Continuous: ContinuousRule{Activation: "tanh"},
		LIF: LIFRule{TauSyn: 1, ThetaMin: 0, ThetaMax: 2, VReset: -1, RefractorySteps: 1,
			Adaptation: LIFAdaptation{Enabled: true, TauAdapt: 1, Beta: 1e308},
			Surrogate:  LIFSurrogate{Kind: "fast_sigmoid", Scale: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	p := MixedParameters{Weights: []float64{0}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}, ThetaRaw: []float64{0}}
	initial := []float64{.25, 0}
	inputs := [][]float64{{0, 3}, {0, 0}, {0, 1e308}}
	upstream := [][]float64{{0, 0}, {0, 0}, {0, 4}}
	tr, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	outputs, spikes := tr.Outputs(), tr.Spikes()
	if len(outputs) != 3 || len(spikes) != 3 {
		t.Fatalf("mixed rows outputs=%d spikes=%d, want 3", len(outputs), len(spikes))
	}
	for step := 0; step < 3; step++ {
		if len(outputs[step]) != 2 || len(spikes[step]) != 2 {
			t.Fatalf("mixed row %d lengths outputs=%d spikes=%d, want 2", step, len(outputs[step]), len(spikes[step]))
		}
		if spikes[step][0] != 0 || spikes[step][1] != []float64{1, 0, 1}[step] {
			t.Fatalf("mixed row %d spikes=%v, want continuous zero and LIF event", step, spikes[step])
		}
	}
	if outputs[0][0] == 0 {
		t.Fatal("continuous node output is zero; mixed fixture did not exercise it")
	}
	g, err := m.Backward(context.Background(), tr, upstream, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := eventAdjointReferenceFor(true)
	if len(g.Weights) != 1 || len(g.Bias) != 2 || len(g.LogTau) != 2 || len(g.ThetaRaw) != 1 || len(g.Initial) != 2 || len(g.Inputs) != 3 {
		t.Fatalf("mixed gradient lengths weights=%d bias=%d log_tau=%d theta=%d initial=%d inputs=%d", len(g.Weights), len(g.Bias), len(g.LogTau), len(g.ThetaRaw), len(g.Initial), len(g.Inputs))
	}
	for step, row := range g.Inputs {
		if len(row) != 2 {
			t.Fatalf("mixed gradient input row %d length=%d, want 2", step, len(row))
		}
		eventAdjointNear(t, "continuous input", row[0], 0)
		eventAdjointNear(t, "LIF input", row[1], want.inputs[step][0])
	}
	eventAdjointNear(t, "continuous bias", g.Bias[0], 0)
	eventAdjointNear(t, "continuous log_tau", g.LogTau[0], 0)
	eventAdjointNear(t, "continuous initial", g.Initial[0], 0)
	eventAdjointNear(t, "LIF bias", g.Bias[1], want.bias[0])
	eventAdjointNear(t, "LIF log_tau", g.LogTau[1], want.logTau[0])
	eventAdjointNear(t, "LIF initial", g.Initial[1], want.initial[0])
	eventAdjointNear(t, "LIF weight", g.Weights[0], want.weights[0])
	eventAdjointNear(t, "LIF theta", g.ThetaRaw[0], want.thetaRaw[0])
}

func TestEventAdjointDisabledAdaptation(t *testing.T) {
	m, p, initial, inputs, upstream := eventAdjointLIFFixture(t, false)
	tr, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	g, err := m.Backward(context.Background(), tr, upstream, 0)
	if err != nil {
		t.Fatal(err)
	}
	eventAdjointAssertLIFGradient(t, g, eventAdjointReferenceFor(false))
}

func TestEventAdjointFiniteSurrogateControl(t *testing.T) {
	m, err := NewLIF(LIFConfig{
		Nodes: 1, Sources: []int{0}, Targets: []int{0}, Delays: []int{1}, DT: math.Log(2), TauSyn: 1,
		ThetaMin: 0, ThetaMax: 2, VReset: -1, RefractorySteps: 0,
		Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	p := LIFParameters{Weights: []float64{0}, Bias: []float64{0}, LogTau: []float64{0}, ThetaRaw: []float64{0}}
	tr, err := m.Forward(context.Background(), p, []float64{0}, [][]float64{{3}, {0}, {0}})
	if err != nil {
		t.Fatal(err)
	}
	g, err := m.Backward(context.Background(), tr, [][]float64{{0}, {1}, {0}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	eventAdjointAssertLIFGradient(t, g, eventAdjointSmallSurrogateReference())
}

func TestEventAdjointTruncationPreservesWithinWindowState(t *testing.T) {
	m, p, initial, inputs, _ := eventAdjointLIFFixture(t, true)
	tr, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	g, err := m.Backward(context.Background(), tr, [][]float64{{0}, {1}, {0}}, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := eventAdjointReference{weights: []float64{0}, bias: []float64{1.0 / 16.0}, logTau: []float64{-3 * math.Log(2) / 16}, thetaRaw: []float64{-1.0 / 16.0}, inputs: [][]float64{{1.0 / 16.0}, {0}, {0}}, initial: []float64{1.0 / 16.0}}
	eventAdjointAssertLIFGradient(t, g, want)
}

func eventAdjointLIFGradientEmpty(g LIFGradient) bool {
	return len(g.Weights) == 0 && len(g.Bias) == 0 && len(g.LogTau) == 0 && len(g.ThetaRaw) == 0 && len(g.Inputs) == 0 && len(g.Initial) == 0
}

func eventAdjointMixedGradientEmpty(g MixedGradient) bool {
	return len(g.Weights) == 0 && len(g.Bias) == 0 && len(g.LogTau) == 0 && len(g.ThetaRaw) == 0 && len(g.Inputs) == 0 && len(g.Initial) == 0
}

func TestEventAdjointRejectsTrueFinalOverflow(t *testing.T) {
	upstream := [][]float64{{0}, {0}, {64}}
	t.Run("LIF/Backward", func(t *testing.T) {
		m, p, initial, inputs, _ := eventAdjointLIFFixture(t, true)
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			t.Fatal(err)
		}
		g, err := m.Backward(context.Background(), tr, upstream, 0)
		if err == nil {
			t.Fatal("Backward with final upstream 64 succeeded, want final representability error")
		}
		if !eventAdjointLIFGradientEmpty(g) {
			t.Fatalf("overflow returned partial gradient: %+v", g)
		}
	})
	t.Run("LIF/BackwardRecompute", func(t *testing.T) {
		m, p, initial, inputs, _ := eventAdjointLIFFixture(t, true)
		g, err := m.BackwardRecompute(context.Background(), p, initial, inputs, upstream, 0, 1)
		if err == nil {
			t.Fatal("BackwardRecompute with final upstream 64 succeeded, want final representability error")
		}
		if !eventAdjointLIFGradientEmpty(g) {
			t.Fatalf("overflow returned partial gradient: %+v", g)
		}
	})
	t.Run("LIF/BackwardCoupled", func(t *testing.T) {
		m, p, initial, inputs, _ := eventAdjointLIFFixture(t, true)
		inputs[2][0] = 0
		mods := zeroTauMods(3)
		mods[2].Offset = [][]float64{{1e308}}
		seq := &zeroTauSequence{mods: mods, nodes: 1}
		tr, err := m.ForwardCoupled(context.Background(), p, initial, inputs, seq)
		if err != nil {
			t.Fatal(err)
		}
		g, err := m.Backward(context.Background(), tr, upstream, 0)
		if err == nil {
			t.Fatal("BackwardCoupled with final upstream 64 succeeded, want final representability error")
		}
		if !eventAdjointLIFGradientEmpty(g) {
			t.Fatalf("overflow returned partial gradient: %+v", g)
		}
	})
	t.Run("LIF/BackwardCoupledRecompute", func(t *testing.T) {
		m, p, initial, inputs, _ := eventAdjointLIFFixture(t, true)
		inputs[2][0] = 0
		mods := zeroTauMods(3)
		mods[2].Offset = [][]float64{{1e308}}
		seq := &zeroTauRecomputeSequence{mods: mods, nodes: 1}
		tr, err := m.ForwardCoupledRecompute(context.Background(), p, initial, inputs, seq, 1)
		if err != nil {
			t.Fatal(err)
		}
		g, err := m.BackwardCoupledRecompute(context.Background(), tr, upstream, 0)
		if err == nil {
			t.Fatal("BackwardCoupledRecompute with final upstream 64 succeeded, want final representability error")
		}
		if !eventAdjointLIFGradientEmpty(g) {
			t.Fatalf("overflow returned partial gradient: %+v", g)
		}
	})
	t.Run("Mixed/Backward", func(t *testing.T) {
		m, p, initial, inputs, _ := eventAdjointMixedLIFFixture(t)
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			t.Fatal(err)
		}
		g, err := m.Backward(context.Background(), tr, upstream, 0)
		if err == nil {
			t.Fatal("Mixed Backward with final upstream 64 succeeded, want final representability error")
		}
		if !eventAdjointMixedGradientEmpty(g) {
			t.Fatalf("overflow returned partial gradient: %+v", g)
		}
	})
	t.Run("Mixed/BackwardCoupled", func(t *testing.T) {
		m, p, initial, inputs, _ := eventAdjointMixedLIFFixture(t)
		inputs[2][0] = 0
		mods := zeroTauMods(3)
		mods[2].Offset = [][]float64{{1e308}}
		seq := &zeroTauSequence{mods: mods, nodes: 1}
		tr, err := m.ForwardCoupled(context.Background(), p, initial, inputs, seq)
		if err != nil {
			t.Fatal(err)
		}
		g, err := m.Backward(context.Background(), tr, upstream, 0)
		if err == nil {
			t.Fatal("Mixed BackwardCoupled with final upstream 64 succeeded, want final representability error")
		}
		if !eventAdjointMixedGradientEmpty(g) {
			t.Fatalf("overflow returned partial gradient: %+v", g)
		}
	})
}

func TestEventAdjointTruncationPreservesFinalForwardState(t *testing.T) {
	want := eventAdjointTruncatedFinalReference()
	for _, tc := range []struct {
		name   string
		window int
	}{
		{name: "window=1", window: 1},
		{name: "window=2", window: 2},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			m, p, initial, inputs, _ := eventAdjointLIFFixture(t, true)
			tr, err := m.Forward(context.Background(), p, initial, inputs)
			if err != nil {
				t.Fatal(err)
			}
			eventAdjointAssertLIFForward(t, tr.Outputs(), tr.Spikes())
			g, err := m.Backward(context.Background(), tr, [][]float64{{0}, {0}, {4}}, tc.window)
			if err != nil {
				t.Fatal(err)
			}
			eventAdjointAssertLIFGradient(t, g, want)
			eventAdjointAssertLIFForward(t, tr.Outputs(), tr.Spikes())
		})
	}
}
