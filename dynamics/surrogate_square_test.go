package dynamics

import (
	"context"
	"math"
	"math/big"
	"testing"
)

type surrogateSquareReference struct {
	bias, logTau, thetaRaw, initial float64
	inputs                          [][]float64
}

func surrogateSquareRat(value float64) *big.Rat {
	return new(big.Rat).SetFloat64(value)
}

func surrogateSquareRounded(value *big.Rat) float64 {
	result, _ := value.Float64()
	return result
}

func surrogateSquareRationalPsi(den float64) float64 {
	if math.IsInf(den, 0) || math.IsNaN(den) {
		return 0
	}
	square := new(big.Rat).Mul(surrogateSquareRat(den), surrogateSquareRat(den))
	return surrogateSquareRounded(new(big.Rat).Inv(square))
}

func surrogateSquareExpectedPsi(u, scale float64) float64 {
	den := 1 + scale*math.Abs(u)
	if !math.IsInf(den, 0) && !math.IsInf(den*den, 0) {
		return 1 / (den * den)
	}
	return surrogateSquareRationalPsi(den)
}

func surrogateSquareReferenceFor(input float64, steps int) surrogateSquareReference {
	den := 1 + math.Abs(.5*input-1)
	psi := surrogateSquareRationalPsi(den)
	du := 1e308 * psi
	logTauRat := new(big.Rat).Mul(
		new(big.Rat).Mul(surrogateSquareRat(du), surrogateSquareRat(-input)),
		surrogateSquareRat(.5*math.Log(2)),
	)
	lastInput := .5 * du
	firstInput := lastInput
	bias := lastInput
	inputs := [][]float64{{lastInput}}
	if steps == 2 {
		firstInput = .5 * lastInput
		bias = firstInput + lastInput
		inputs = [][]float64{{firstInput}, {lastInput}}
	}
	return surrogateSquareReference{
		bias:     bias,
		logTau:   surrogateSquareRounded(logTauRat),
		thetaRaw: -.5 * du,
		initial:  firstInput,
		inputs:   inputs,
	}
}

func surrogateSquareNewLIF(t *testing.T, refractory int, scale float64) *LIF {
	t.Helper()
	m, err := NewLIF(LIFConfig{
		Nodes: 1, DT: math.Log(2), TauSyn: 1,
		ThetaMin: 0, ThetaMax: 2, VReset: -1,
		RefractorySteps: refractory,
		Surrogate:       LIFSurrogate{Kind: "fast_sigmoid", Scale: scale},
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func surrogateSquareLIFFixture(t *testing.T, input float64, steps int) (*LIF, LIFParameters, []float64, [][]float64, [][]float64) {
	t.Helper()
	m := surrogateSquareNewLIF(t, 0, 1)
	inputs := make([][]float64, steps)
	upstream := make([][]float64, steps)
	for step := range inputs {
		inputs[step] = []float64{0}
		upstream[step] = []float64{0}
	}
	inputs[steps-1][0] = input
	upstream[steps-1][0] = 1e308
	if steps == 2 {
		// The second row carries 0.5*upstream back through the synaptic trace.
		// Cancel that independent path so the first row only tests membrane
		// propagation, whose exact input gradient is 0.5*the final one.
		upstream[0][0] = -5e307
	}
	p := LIFParameters{Weights: []float64{}, Bias: []float64{0}, LogTau: []float64{0}, ThetaRaw: []float64{0}}
	return m, p, []float64{0}, inputs, upstream
}

func surrogateSquareNear(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Fatalf("%s=%v, want finite %v", name, got, want)
	}
	limit := 1e-15 * math.Max(1, math.Abs(want))
	if math.Abs(got-want) > limit {
		t.Fatalf("%s=%v, want %v (limit %v)", name, got, want, limit)
	}
}

func surrogateSquareAssertFiniteRows(t *testing.T, name string, rows [][]float64) {
	t.Helper()
	for row, values := range rows {
		for column, value := range values {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				t.Fatalf("%s[%d][%d]=%v, want finite", name, row, column, value)
			}
		}
	}
}

func surrogateSquareAssertLIFTrace(t *testing.T, tr *LIFTrace, input float64, steps int) {
	t.Helper()
	outputs, spikes, voltages := tr.Outputs(), tr.Spikes(), tr.Voltages()
	if len(outputs) != steps || len(spikes) != steps || len(voltages) != steps {
		t.Fatalf("trace rows outputs=%d spikes=%d voltages=%d, want %d", len(outputs), len(spikes), len(voltages), steps)
	}
	surrogateSquareAssertFiniteRows(t, "outputs", outputs)
	surrogateSquareAssertFiniteRows(t, "spikes", spikes)
	surrogateSquareAssertFiniteRows(t, "voltages", voltages)
	for step := 0; step < steps; step++ {
		if len(outputs[step]) != 1 || len(spikes[step]) != 1 || len(voltages[step]) != 1 {
			t.Fatalf("trace row %d lengths outputs=%d spikes=%d voltages=%d, want 1", step, len(outputs[step]), len(spikes[step]), len(voltages[step]))
		}
		wantSpike, wantVoltage := 0.0, 0.0
		if step == steps-1 {
			if input >= 0 {
				wantSpike, wantVoltage = 1, -1
			} else {
				wantVoltage = .5 * input
			}
		}
		if outputs[step][0] != wantSpike || spikes[step][0] != wantSpike || voltages[step][0] != wantVoltage {
			t.Fatalf("trace row %d outputs=%v spikes=%v voltages=%v, want [%v] [%v] [%v]", step, outputs[step], spikes[step], voltages[step], wantSpike, wantSpike, wantVoltage)
		}
	}
}

func surrogateSquareAssertLIFOutputs(t *testing.T, outputs [][]float64, input float64, steps int) {
	t.Helper()
	if len(outputs) != steps {
		t.Fatalf("output rows=%d, want %d", len(outputs), steps)
	}
	surrogateSquareAssertFiniteRows(t, "outputs", outputs)
	for step, row := range outputs {
		if len(row) != 1 {
			t.Fatalf("output row %d length=%d, want 1", step, len(row))
		}
		want := 0.0
		if step == steps-1 && input >= 0 {
			want = 1
		}
		if row[0] != want {
			t.Fatalf("output row %d=%v, want [%v]", step, row, want)
		}
	}
}

func surrogateSquareAssertLIFGradient(t *testing.T, got LIFGradient, want surrogateSquareReference) {
	t.Helper()
	if len(got.Weights) != 0 || len(got.Bias) != 1 || len(got.LogTau) != 1 || len(got.ThetaRaw) != 1 || len(got.Initial) != 1 || len(got.Inputs) != len(want.inputs) {
		t.Fatalf("gradient lengths weights=%d bias=%d log_tau=%d theta=%d initial=%d inputs=%d, want 0,1,1,1,1,%d", len(got.Weights), len(got.Bias), len(got.LogTau), len(got.ThetaRaw), len(got.Initial), len(got.Inputs), len(want.inputs))
	}
	surrogateSquareNear(t, "bias", got.Bias[0], want.bias)
	surrogateSquareNear(t, "log_tau", got.LogTau[0], want.logTau)
	surrogateSquareNear(t, "theta_raw", got.ThetaRaw[0], want.thetaRaw)
	surrogateSquareNear(t, "initial", got.Initial[0], want.initial)
	for step, row := range got.Inputs {
		if len(row) != 1 || len(want.inputs[step]) != 1 {
			t.Fatalf("input row %d lengths got=%d want=%d", step, len(row), len(want.inputs[step]))
		}
		surrogateSquareNear(t, "input", row[0], want.inputs[step][0])
	}
}

func TestSurrogateSquareEventDerivative(t *testing.T) {
	m := surrogateSquareNewLIF(t, 0, 1)
	mixed, err := NewMixed(MixedConfig{
		Nodes: 1, DT: math.Log(2), NodeRule: []uint8{RuleLIF},
		LIF: LIFRule{TauSyn: 1, ThetaMin: 0, ThetaMax: 2, VReset: -1,
			Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	finiteDen := math.Sqrt(math.MaxFloat64)
	for math.IsInf(finiteDen*finiteDen, 0) {
		finiteDen = math.Nextafter(finiteDen, 0)
	}
	overflowDen := math.Nextafter(finiteDen, math.Inf(1))
	for !math.IsInf(overflowDen*overflowDen, 0) {
		overflowDen = math.Nextafter(overflowDen, math.Inf(1))
	}
	minSubnormalDen := math.Ldexp(1, 537)
	tieDen := math.Sqrt2 * minSubnormalDen
	cases := []struct {
		name string
		u    float64
	}{
		{name: "zero", u: 0},
		{name: "positive-normal", u: 2},
		{name: "negative-normal", u: -2},
		{name: "positive-subnormal-1", u: 1e154},
		{name: "negative-subnormal-1", u: -1e154},
		{name: "positive-subnormal-2", u: 1e155},
		{name: "negative-subnormal-2", u: -1e155},
		{name: "positive-subnormal-3", u: 2e155},
		{name: "last-finite-denominator", u: finiteDen},
		{name: "first-overflowing-denominator", u: overflowDen},
		{name: "first-subnormal-denominator", u: minSubnormalDen - 1},
		{name: "last-nonzero-before-tie", u: math.Nextafter(tieDen, 0) - 1},
		{name: "subnormal-rounding-tie", u: tieDen - 1},
		{name: "first-zero-after-tie", u: math.Nextafter(tieDen, math.Inf(1)) - 1},
		{name: "two-to-538-denominator", u: math.Ldexp(1, 538) - 1},
		{name: "true-local-slope-underflow", u: math.MaxFloat64},
		{name: "positive-infinite-denominator", u: math.Inf(1)},
		{name: "negative-infinite-denominator", u: math.Inf(-1)},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gotSpike, gotPsi := m.event(tc.u)
			wantSpike := 0.0
			if tc.u >= 0 {
				wantSpike = 1
			}
			if gotSpike != wantSpike {
				t.Fatalf("spike=%v, want %v", gotSpike, wantSpike)
			}
			wantPsi := surrogateSquareExpectedPsi(tc.u, 1)
			if math.Float64bits(gotPsi) != math.Float64bits(wantPsi) {
				t.Fatalf("derivative=%g (%x), want %g (%x)", gotPsi, math.Float64bits(gotPsi), wantPsi, math.Float64bits(wantPsi))
			}
			den := 1 + math.Abs(tc.u)
			if !math.IsInf(den*den, 0) {
				original := 1 / (den * den)
				if math.Float64bits(gotPsi) != math.Float64bits(original) {
					t.Fatalf("finite expression changed: derivative=%x original=%x", math.Float64bits(gotPsi), math.Float64bits(original))
				}
			}
			mixedSpike, mixedPsi := mixed.event(tc.u)
			if math.Float64bits(mixedSpike) != math.Float64bits(gotSpike) || math.Float64bits(mixedPsi) != math.Float64bits(wantPsi) {
				t.Fatalf("mixed event=(%x,%x), want=(%x,%x)", math.Float64bits(mixedSpike), math.Float64bits(mixedPsi), math.Float64bits(gotSpike), math.Float64bits(wantPsi))
			}
		})
	}
}

func TestSurrogateSquareSmoothEventOrder(t *testing.T) {
	m := surrogateSquareNewLIF(t, 0, 2)
	m.smooth = true
	mixed, err := NewMixed(MixedConfig{
		Nodes: 1, DT: math.Log(2), NodeRule: []uint8{RuleLIF},
		LIF: LIFRule{TauSyn: 1, ThetaMin: 0, ThetaMax: 2, VReset: -1,
			Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	mixed.smooth = true
	for _, tc := range []struct {
		name string
		u    float64
	}{
		{name: "positive-normal", u: .25},
		{name: "negative-normal", u: -.25},
		{name: "positive-recovered-subnormal", u: 1e155},
		{name: "negative-recovered-subnormal", u: -1e155},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gotSpike, gotPsi := m.event(tc.u)
			den := 1 + 2*math.Abs(tc.u)
			wantSpike := .5 * (1 + 2*tc.u/den)
			wantPsi := .5 * 2 * surrogateSquareExpectedPsi(tc.u, 2)
			if math.Float64bits(gotSpike) != math.Float64bits(wantSpike) {
				t.Fatalf("smooth output=%x, want %x", math.Float64bits(gotSpike), math.Float64bits(wantSpike))
			}
			if math.Float64bits(gotPsi) != math.Float64bits(wantPsi) {
				t.Fatalf("smooth derivative=%x, want %x", math.Float64bits(gotPsi), math.Float64bits(wantPsi))
			}
			mixedSpike, mixedPsi := mixed.event(tc.u)
			if math.Float64bits(mixedSpike) != math.Float64bits(wantSpike) || math.Float64bits(mixedPsi) != math.Float64bits(wantPsi) {
				t.Fatalf("mixed smooth event=(%x,%x), want=(%x,%x)", math.Float64bits(mixedSpike), math.Float64bits(mixedPsi), math.Float64bits(wantSpike), math.Float64bits(wantPsi))
			}
		})
	}
}

func TestSurrogateSquareHeldRefractoryDerivative(t *testing.T) {
	m := surrogateSquareNewLIF(t, 1, 1)
	p := LIFParameters{Weights: []float64{}, Bias: []float64{0}, LogTau: []float64{0}, ThetaRaw: []float64{0}}
	inputs := [][]float64{{3}, {2e155}}
	tr, err := m.Forward(context.Background(), p, []float64{0}, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.dspike) != 2 || len(tr.dspike[1]) != 1 || tr.dspike[1][0] != 0 {
		t.Fatalf("held row derivative=%v, want zero", tr.dspike)
	}
	if got := tr.Spikes(); len(got) != 2 || len(got[0]) != 1 || len(got[1]) != 1 || got[0][0] != 1 || got[1][0] != 0 {
		t.Fatalf("held row spikes=%v, want [[1] [0]]", got)
	}
	g, err := m.Backward(context.Background(), tr, [][]float64{{0}, {1e308}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Inputs) != 2 || len(g.Inputs[1]) != 1 {
		t.Fatalf("input gradient shape=%v, want two one-element rows", g.Inputs)
	}
	if g.Inputs[1][0] != 0 {
		t.Fatalf("held row input gradient=%v, want zero", g.Inputs[1][0])
	}
}

func TestSurrogateSquareLIFPaths(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input float64
	}{
		{name: "positive", input: 2e155},
		{name: "negative", input: -2e155},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Run("ForwardBackward", func(t *testing.T) {
				m, p, initial, inputs, upstream := surrogateSquareLIFFixture(t, tc.input, 1)
				tr, err := m.Forward(context.Background(), p, initial, inputs)
				if err != nil {
					t.Fatal(err)
				}
				surrogateSquareAssertLIFTrace(t, tr, tc.input, 1)
				g, err := m.Backward(context.Background(), tr, upstream, 0)
				if err != nil {
					t.Fatal(err)
				}
				surrogateSquareAssertLIFGradient(t, g, surrogateSquareReferenceFor(tc.input, 1))
			})
			t.Run("ForwardRecompute", func(t *testing.T) {
				m, p, initial, inputs, _ := surrogateSquareLIFFixture(t, tc.input, 1)
				outputs, err := m.ForwardRecompute(context.Background(), p, initial, inputs, 1)
				if err != nil {
					t.Fatal(err)
				}
				surrogateSquareAssertLIFOutputs(t, outputs, tc.input, 1)
			})
			for _, segment := range []int{1, 2} {
				segment := segment
				t.Run("BackwardRecompute/segment="+string(rune('0'+segment)), func(t *testing.T) {
					m, p, initial, inputs, upstream := surrogateSquareLIFFixture(t, tc.input, 2)
					outputs, err := m.ForwardRecompute(context.Background(), p, initial, inputs, segment)
					if err != nil {
						t.Fatal(err)
					}
					surrogateSquareAssertLIFOutputs(t, outputs, tc.input, 2)
					g, err := m.BackwardRecompute(context.Background(), p, initial, inputs, upstream, 0, segment)
					if err != nil {
						t.Fatal(err)
					}
					surrogateSquareAssertLIFGradient(t, g, surrogateSquareReferenceFor(tc.input, 2))
				})
			}
			t.Run("ForwardCoupledBackward", func(t *testing.T) {
				m, p, initial, inputs, upstream := surrogateSquareLIFFixture(t, tc.input, 1)
				tr, err := m.ForwardCoupled(context.Background(), p, initial, inputs, &zeroTauSequence{mods: zeroTauMods(1), nodes: 1})
				if err != nil {
					t.Fatal(err)
				}
				surrogateSquareAssertLIFTrace(t, tr, tc.input, 1)
				g, err := m.Backward(context.Background(), tr, upstream, 0)
				if err != nil {
					t.Fatal(err)
				}
				surrogateSquareAssertLIFGradient(t, g, surrogateSquareReferenceFor(tc.input, 1))
			})
			t.Run("ForwardCoupledRecomputeBackward", func(t *testing.T) {
				m, p, initial, inputs, upstream := surrogateSquareLIFFixture(t, tc.input, 2)
				tr, err := m.ForwardCoupledRecompute(context.Background(), p, initial, inputs, &zeroTauRecomputeSequence{mods: zeroTauMods(2), nodes: 1}, 1)
				if err != nil {
					t.Fatal(err)
				}
				surrogateSquareAssertLIFOutputs(t, tr.Outputs(), tc.input, 2)
				if len(tr.FinalVoltage()) != 1 {
					t.Fatalf("final voltage=%v, want one value", tr.FinalVoltage())
				}
				surrogateSquareAssertFiniteRows(t, "final voltage", [][]float64{tr.FinalVoltage()})
				g, err := m.BackwardCoupledRecompute(context.Background(), tr, upstream, 0)
				if err != nil {
					t.Fatal(err)
				}
				surrogateSquareAssertLIFGradient(t, g, surrogateSquareReferenceFor(tc.input, 2))
			})
		})
	}
}

func surrogateSquareMixedLIFFixture(t *testing.T, input float64) (*Mixed, MixedParameters, []float64, [][]float64, [][]float64) {
	t.Helper()
	m, err := NewMixed(MixedConfig{
		Nodes: 1, DT: math.Log(2), NodeRule: []uint8{RuleLIF},
		LIF: LIFRule{TauSyn: 1, ThetaMin: 0, ThetaMax: 2, VReset: -1, Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	p := MixedParameters{Weights: []float64{}, Bias: []float64{0}, LogTau: []float64{0}, ThetaRaw: []float64{0}}
	return m, p, []float64{0}, [][]float64{{input}}, [][]float64{{1e308}}
}

func surrogateSquareAssertMixedLIFFixture(t *testing.T, tr *MixedTrace, input float64) {
	t.Helper()
	outputs, spikes, voltages := tr.Outputs(), tr.Spikes(), tr.Voltages()
	if len(outputs) != 1 || len(spikes) != 1 || len(voltages) != 1 || len(outputs[0]) != 1 || len(spikes[0]) != 1 || len(voltages[0]) != 1 {
		t.Fatalf("mixed trace shape outputs=%v spikes=%v voltages=%v", outputs, spikes, voltages)
	}
	surrogateSquareAssertFiniteRows(t, "mixed outputs", outputs)
	surrogateSquareAssertFiniteRows(t, "mixed spikes", spikes)
	surrogateSquareAssertFiniteRows(t, "mixed voltages", voltages)
	wantSpike, wantVoltage := 0.0, .5*input
	if input >= 0 {
		wantSpike, wantVoltage = 1, -1
	}
	if outputs[0][0] != wantSpike || spikes[0][0] != wantSpike || voltages[0][0] != wantVoltage {
		t.Fatalf("mixed trace outputs=%v spikes=%v voltages=%v", outputs, spikes, voltages)
	}
}

func surrogateSquareAssertMixedLIFGradient(t *testing.T, got MixedGradient, want surrogateSquareReference) {
	t.Helper()
	if len(got.Weights) != 0 || len(got.Bias) != 1 || len(got.LogTau) != 1 || len(got.ThetaRaw) != 1 || len(got.Initial) != 1 || len(got.Inputs) != 1 || len(got.Inputs[0]) != 1 {
		t.Fatalf("mixed gradient lengths weights=%d bias=%d log_tau=%d theta=%d initial=%d inputs=%d", len(got.Weights), len(got.Bias), len(got.LogTau), len(got.ThetaRaw), len(got.Initial), len(got.Inputs))
	}
	surrogateSquareNear(t, "mixed bias", got.Bias[0], want.bias)
	surrogateSquareNear(t, "mixed log_tau", got.LogTau[0], want.logTau)
	surrogateSquareNear(t, "mixed theta_raw", got.ThetaRaw[0], want.thetaRaw)
	surrogateSquareNear(t, "mixed initial", got.Initial[0], want.initial)
	surrogateSquareNear(t, "mixed input", got.Inputs[0][0], want.inputs[0][0])
}

func TestSurrogateSquareMixedPaths(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input float64
	}{
		{name: "positive", input: 2e155},
		{name: "negative", input: -2e155},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			want := surrogateSquareReferenceFor(tc.input, 1)
			t.Run("all-LIF/ForwardBackward", func(t *testing.T) {
				m, p, initial, inputs, upstream := surrogateSquareMixedLIFFixture(t, tc.input)
				tr, err := m.Forward(context.Background(), p, initial, inputs)
				if err != nil {
					t.Fatal(err)
				}
				surrogateSquareAssertMixedLIFFixture(t, tr, tc.input)
				g, err := m.Backward(context.Background(), tr, upstream, 0)
				if err != nil {
					t.Fatal(err)
				}
				surrogateSquareAssertMixedLIFGradient(t, g, want)
			})
			t.Run("all-LIF/ForwardCoupledBackward", func(t *testing.T) {
				m, p, initial, inputs, upstream := surrogateSquareMixedLIFFixture(t, tc.input)
				tr, err := m.ForwardCoupled(context.Background(), p, initial, inputs, &zeroTauSequence{mods: zeroTauMods(1), nodes: 1})
				if err != nil {
					t.Fatal(err)
				}
				surrogateSquareAssertMixedLIFFixture(t, tr, tc.input)
				g, err := m.Backward(context.Background(), tr, upstream, 0)
				if err != nil {
					t.Fatal(err)
				}
				surrogateSquareAssertMixedLIFGradient(t, g, want)
			})

			t.Run("continuous-plus-LIF/ForwardBackward", func(t *testing.T) {
				m, err := NewMixed(MixedConfig{
					Nodes: 2, DT: math.Log(2), NodeRule: []uint8{RuleContinuous, RuleLIF},
					Continuous: ContinuousRule{Activation: "tanh"},
					LIF:        LIFRule{TauSyn: 1, ThetaMin: 0, ThetaMax: 2, VReset: -1, Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 1}},
				})
				if err != nil {
					t.Fatal(err)
				}
				p := MixedParameters{Weights: []float64{}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}, ThetaRaw: []float64{0}}
				inputs := [][]float64{{0, tc.input}}
				tr, err := m.Forward(context.Background(), p, []float64{0, 0}, inputs)
				if err != nil {
					t.Fatal(err)
				}
				if len(tr.Outputs()) != 1 || len(tr.Outputs()[0]) != 2 || len(tr.Spikes()) != 1 || len(tr.Spikes()[0]) != 2 || len(tr.Voltages()) != 1 || len(tr.Voltages()[0]) != 2 {
					t.Fatalf("mixed trace shape outputs=%v spikes=%v voltages=%v", tr.Outputs(), tr.Spikes(), tr.Voltages())
				}
				surrogateSquareAssertFiniteRows(t, "mixed outputs", tr.Outputs())
				surrogateSquareAssertFiniteRows(t, "mixed spikes", tr.Spikes())
				surrogateSquareAssertFiniteRows(t, "mixed voltages", tr.Voltages())
				wantLIFSpike := 0.0
				wantLIFVoltage := .5 * tc.input
				if tc.input >= 0 {
					wantLIFSpike, wantLIFVoltage = 1, -1
				}
				if tr.Outputs()[0][0] != 0 || tr.Spikes()[0][0] != 0 || tr.Voltages()[0][0] != 0 || tr.Outputs()[0][1] != wantLIFSpike || tr.Spikes()[0][1] != wantLIFSpike || tr.Voltages()[0][1] != wantLIFVoltage {
					t.Fatalf("mixed outputs=%v spikes=%v, want continuous zero and LIF event", tr.Outputs(), tr.Spikes())
				}
				g, err := m.Backward(context.Background(), tr, [][]float64{{0, 1e308}}, 0)
				if err != nil {
					t.Fatal(err)
				}
				if len(g.Weights) != 0 || len(g.Bias) != 2 || len(g.LogTau) != 2 || len(g.ThetaRaw) != 1 || len(g.Initial) != 2 || len(g.Inputs) != 1 || len(g.Inputs[0]) != 2 {
					t.Fatalf("mixed gradient shape weights=%d bias=%d log_tau=%d theta=%d initial=%d inputs=%d", len(g.Weights), len(g.Bias), len(g.LogTau), len(g.ThetaRaw), len(g.Initial), len(g.Inputs))
				}
				surrogateSquareNear(t, "continuous bias", g.Bias[0], 0)
				surrogateSquareNear(t, "continuous log_tau", g.LogTau[0], 0)
				surrogateSquareNear(t, "continuous initial", g.Initial[0], 0)
				surrogateSquareNear(t, "continuous input", g.Inputs[0][0], 0)
				surrogateSquareNear(t, "LIF bias", g.Bias[1], want.bias)
				surrogateSquareNear(t, "LIF log_tau", g.LogTau[1], want.logTau)
				surrogateSquareNear(t, "LIF theta_raw", g.ThetaRaw[0], want.thetaRaw)
				surrogateSquareNear(t, "LIF initial", g.Initial[1], want.initial)
				surrogateSquareNear(t, "LIF input", g.Inputs[0][1], want.inputs[0][0])
			})

			t.Run("continuous-plus-LIF/ForwardCoupledBackward", func(t *testing.T) {
				m, err := NewMixed(MixedConfig{
					Nodes: 2, DT: math.Log(2), NodeRule: []uint8{RuleContinuous, RuleLIF},
					Continuous: ContinuousRule{Activation: "tanh"},
					LIF:        LIFRule{TauSyn: 1, ThetaMin: 0, ThetaMax: 2, VReset: -1, Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 1}},
				})
				if err != nil {
					t.Fatal(err)
				}
				p := MixedParameters{Weights: []float64{}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}, ThetaRaw: []float64{0}}
				inputs := [][]float64{{0, tc.input}}
				seq := &zeroTauSequence{mods: zeroTauMods(1), nodes: 2}
				tr, err := m.ForwardCoupled(context.Background(), p, []float64{0, 0}, inputs, seq)
				if err != nil {
					t.Fatal(err)
				}
				if len(tr.Outputs()) != 1 || len(tr.Outputs()[0]) != 2 || len(tr.Spikes()) != 1 || len(tr.Spikes()[0]) != 2 || len(tr.Voltages()) != 1 || len(tr.Voltages()[0]) != 2 {
					t.Fatalf("mixed coupled trace shape outputs=%v spikes=%v voltages=%v", tr.Outputs(), tr.Spikes(), tr.Voltages())
				}
				surrogateSquareAssertFiniteRows(t, "mixed coupled outputs", tr.Outputs())
				surrogateSquareAssertFiniteRows(t, "mixed coupled spikes", tr.Spikes())
				surrogateSquareAssertFiniteRows(t, "mixed coupled voltages", tr.Voltages())
				wantLIFSpike := 0.0
				wantLIFVoltage := .5 * tc.input
				if tc.input >= 0 {
					wantLIFSpike, wantLIFVoltage = 1, -1
				}
				if tr.Outputs()[0][0] != 0 || tr.Spikes()[0][0] != 0 || tr.Voltages()[0][0] != 0 || tr.Outputs()[0][1] != wantLIFSpike || tr.Spikes()[0][1] != wantLIFSpike || tr.Voltages()[0][1] != wantLIFVoltage {
					t.Fatalf("mixed coupled outputs=%v spikes=%v voltages=%v", tr.Outputs(), tr.Spikes(), tr.Voltages())
				}
				g, err := m.Backward(context.Background(), tr, [][]float64{{0, 1e308}}, 0)
				if err != nil {
					t.Fatal(err)
				}
				if len(g.Weights) != 0 || len(g.Bias) != 2 || len(g.LogTau) != 2 || len(g.ThetaRaw) != 1 || len(g.Initial) != 2 || len(g.Inputs) != 1 || len(g.Inputs[0]) != 2 {
					t.Fatalf("mixed coupled gradient shape weights=%d bias=%d log_tau=%d theta=%d initial=%d inputs=%d", len(g.Weights), len(g.Bias), len(g.LogTau), len(g.ThetaRaw), len(g.Initial), len(g.Inputs))
				}
				surrogateSquareNear(t, "coupled continuous bias", g.Bias[0], 0)
				surrogateSquareNear(t, "coupled continuous log_tau", g.LogTau[0], 0)
				surrogateSquareNear(t, "coupled continuous initial", g.Initial[0], 0)
				surrogateSquareNear(t, "coupled continuous input", g.Inputs[0][0], 0)
				surrogateSquareNear(t, "coupled LIF bias", g.Bias[1], want.bias)
				surrogateSquareNear(t, "coupled LIF log_tau", g.LogTau[1], want.logTau)
				surrogateSquareNear(t, "coupled LIF theta_raw", g.ThetaRaw[0], want.thetaRaw)
				surrogateSquareNear(t, "coupled LIF initial", g.Initial[1], want.initial)
				surrogateSquareNear(t, "coupled LIF input", g.Inputs[0][1], want.inputs[0][0])
			})
		})
	}
}
