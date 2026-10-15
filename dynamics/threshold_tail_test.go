package dynamics

import (
	"context"
	"math"
	"testing"
)

type thresholdTailCase struct {
	name                                  string
	minimum, maximum, raw                 float64
	initial, input, reset                 float64
	base, slope, spike, voltage           float64
	bias, logTau, thetaRaw, inputGradient float64
	initialGradient                       float64
}

// These values are embedded from the independent Decimal-200/280 reference.
// No production sigmoid or threshold helper is used to construct this oracle.
func thresholdTailCases() []thresholdTailCase {
	return []thresholdTailCase{
		{name: "ordinary-positive-0", maximum: 2, raw: 0, input: 1, reset: -1,
			base: 1, slope: .5, voltage: .5, bias: .2222222222222222,
			logTau: -.15403270679109896, thetaRaw: -.2222222222222222,
			inputGradient: .2222222222222222, initialGradient: .2222222222222222},
		{name: "ordinary-positive-8", maximum: 2, raw: 8, input: 1, reset: -1,
			base: 1.999329299739067, slope: .0006704753415129485, voltage: .5,
			bias: .08004294209669222, logTau: -.05548153963804516,
			thetaRaw: -.00010733363787596175, inputGradient: .08004294209669222,
			initialGradient: .08004294209669222},
		{name: "ordinary-positive-12", maximum: 2, raw: 12, input: 1, reset: -1,
			base: 1.9999877116507956, slope: 1.2288273702666351e-05, voltage: .5,
			bias: .08000078646014766, logTau: -.0554523195774296,
			thetaRaw: -1.9661431209017176e-06, inputGradient: .08000078646014766,
			initialGradient: .08000078646014766},
		{name: "ordinary-positive-20", maximum: 2, raw: 20, input: 1, reset: -1,
			base: 1.9999999958776928, slope: 4.122307227883698e-09, voltage: .5,
			bias: .08000000026382764, logTau: -.05545177462766701,
			thetaRaw: -6.595691586365489e-10, inputGradient: .08000000026382764,
			initialGradient: .08000000026382764},
		{name: "ordinary-positive-40", maximum: 2, raw: 40, input: 1, reset: -1,
			base: 2, slope: 8.496708510583178e-18, voltage: .5, bias: .08,
			logTau: -.055451774444795626, thetaRaw: -1.3594733616933085e-18,
			inputGradient: .08, initialGradient: .08},
		{name: "ordinary-positive-50", maximum: 2, raw: 50, input: 1, reset: -1,
			base: 2, slope: 3.8574996959278356e-22, voltage: .5, bias: .08,
			logTau: -.055451774444795626, thetaRaw: -6.171999513484537e-23,
			inputGradient: .08, initialGradient: .08},
		{name: "wide-positive-40", maximum: 1e308, raw: 40, initial: 1e308, input: 1e308, reset: -1,
			base: 1e308, slope: 4.248354255291589e290, spike: 1, voltage: -1,
			bias: .5, thetaRaw: -4.248354255291589e290, inputGradient: .5, initialGradient: .5},
		{name: "wide-positive-50", maximum: 1e308, raw: 50, initial: 1e308, input: 1e308, reset: -1,
			base: 1e308, slope: 1.928749847963918e286, spike: 1, voltage: -1,
			bias: .5, thetaRaw: -1.928749847963918e286, inputGradient: .5, initialGradient: .5},
		{name: "wide-positive-740", maximum: 1e308, raw: 740, initial: 1e308, input: 1e308, reset: -1,
			base: 1e308, slope: 4.188739880048049e-14, spike: 1, voltage: -1,
			bias: .5, thetaRaw: -4.188739880048049e-14, inputGradient: .5, initialGradient: .5},
		{name: "wide-positive-750", maximum: 1e308, raw: 750, initial: 1e308, input: 1e308, reset: -1,
			base: 1e308, slope: 1.9016849634750064e-18, spike: 1, voltage: -1,
			bias: .5, thetaRaw: -1.9016849634750064e-18, inputGradient: .5, initialGradient: .5},
		{name: "wide-positive-1000", maximum: 1e308, raw: 1000, initial: 1e308, input: 1e308, reset: -1,
			base: 1e308, slope: 5.075958897549457e-127, spike: 1, voltage: -1,
			bias: .5, thetaRaw: -5.075958897549457e-127, inputGradient: .5, initialGradient: .5},
		{name: "wide-negative--40", maximum: 1e308, raw: -40, reset: -1,
			base: 4.248354255291589e290, slope: 4.248354255291589e290},
		{name: "wide-negative--740", maximum: 1e308, raw: -740, reset: -1,
			base: 4.188739880048049e-14, slope: 4.188739880048049e-14,
			bias: .49999999999995803, thetaRaw: -4.188739880047698e-14,
			inputGradient: .49999999999995803, initialGradient: .49999999999995803},
		{name: "wide-negative--750", maximum: 1e308, raw: -750, reset: -1,
			base: 1.9016849634750064e-18, slope: 1.9016849634750064e-18,
			bias: .5, thetaRaw: -1.9016849634750064e-18, inputGradient: .5, initialGradient: .5},
		{name: "wide-negative--1000", maximum: 1e308, raw: -1000, reset: -1,
			base: 5.075958897549457e-127, slope: 5.075958897549457e-127,
			bias: .5, thetaRaw: -5.075958897549457e-127, inputGradient: .5, initialGradient: .5},
		{name: "forward-cancellation-positive-40", minimum: -1e308, raw: 40,
			input: -1e200, reset: -math.MaxFloat64, base: -4.248354255291589e290,
			slope: 4.248354255291589e290, spike: 1, voltage: -math.MaxFloat64},
		{name: "true-coefficient-underflow--1000", maximum: 2, raw: -1000, reset: -1,
			spike: 1, voltage: -1, bias: .5, inputGradient: .5, initialGradient: .5},
		{name: "true-coefficient-underflow-1000", maximum: 2, raw: 1000, reset: -1,
			base: 2, slope: 0, bias: .05555555555555555,
			inputGradient: .05555555555555555, initialGradient: .05555555555555555},
	}
}

type thresholdTailCoefficientCase struct {
	name                  string
	minimum, maximum, raw float64
	base, slope           float64
}

func thresholdTailCoefficientCases() []thresholdTailCoefficientCase {
	a := math.Ldexp(1, 1023)
	return []thresholdTailCoefficientCase{
		{name: "overflow-span-40", minimum: -a, maximum: a, raw: 40, base: a, slope: 7.637237279200796e290},
		{name: "overflow-span-negative-40", minimum: -a, maximum: a, raw: -40, base: -a, slope: 7.637237279200796e290},
		{name: "overflow-span-740", minimum: -a, maximum: a, raw: 740, base: a, slope: 7.530068926086378e-14},
		{name: "overflow-span-negative-740", minimum: -a, maximum: a, raw: -740, base: -a, slope: 7.530068926086378e-14},
		{name: "overflow-span-750", minimum: -a, maximum: a, raw: 750, base: a, slope: 3.418646003509913e-18},
		{name: "overflow-span-negative-750", minimum: -a, maximum: a, raw: -750, base: -a, slope: 3.418646003509913e-18},
		{name: "overflow-span-1000", minimum: -a, maximum: a, raw: 1000, base: a, slope: 9.125016462967948e-127},
		{name: "overflow-span-negative-1000", minimum: -a, maximum: a, raw: -1000, base: -a, slope: 9.125016462967948e-127},
		{name: "last-bit-1454.3", minimum: -a, maximum: a, raw: 1454.3, base: a, slope: math.SmallestNonzeroFloat64},
		{name: "last-bit-negative-1454.3", minimum: -a, maximum: a, raw: -1454.3, base: -a, slope: math.SmallestNonzeroFloat64},
		{name: "last-bit-1455", minimum: -a, maximum: a, raw: 1455, base: a, slope: 0},
		{name: "last-bit-negative-1455", minimum: -a, maximum: a, raw: -1455, base: -a, slope: 0},
		{name: "close-endpoints", minimum: 1e308, maximum: math.Nextafter(1e308, math.Inf(1)), raw: 40,
			base: math.Nextafter(1e308, math.Inf(1)), slope: 8.479036671894309e274},
		{name: "positive-cutoff-before", maximum: 2, raw: math.Nextafter(7, math.Inf(-1)), base: 1.9981778976111988, slope: .0018204423602436546},
		{name: "positive-cutoff", maximum: 2, raw: 7, base: 1.9981778976111988, slope: .001820442360243653},
		{name: "negative-cutoff-before", maximum: 2, raw: math.Nextafter(-700, math.Inf(1)), base: 1.9719353087521784e-304, slope: 1.9719353087521784e-304},
		{name: "negative-cutoff", maximum: 2, raw: -700, base: 1.971935308751954e-304, slope: 1.971935308751954e-304},
		{name: "subnormal-final", maximum: 2, raw: 740, base: 2, slope: 8.4e-322},
	}
}

func thresholdTailLIFConfig(c thresholdTailCase) LIFConfig {
	return LIFConfig{Nodes: 1, DT: math.Ln2, TauSyn: 1,
		ThetaMin: c.minimum, ThetaMax: c.maximum, VReset: c.reset,
		Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 1}}
}

func thresholdTailParameters(c thresholdTailCase) LIFParameters {
	return LIFParameters{Weights: []float64{}, Bias: []float64{0}, LogTau: []float64{0}, ThetaRaw: []float64{c.raw}}
}

func thresholdTailCheckFloat(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Fatalf("%s=%v, want finite %v", name, got, want)
	}
	if want == 0 {
		if got != 0 {
			t.Fatalf("%s=%v, want exactly zero", name, got)
		}
		return
	}
	if math.Abs((got-want)/want) > 1e-12 {
		t.Fatalf("%s=%g, want %g", name, got, want)
	}
}

func thresholdTailCheckCoefficient(t *testing.T, name string, got, want float64) {
	t.Helper()
	if want == 0 {
		thresholdTailCheckFloat(t, name, got, want)
		return
	}
	if got == 0 || math.IsNaN(got) || math.IsInf(got, 0) {
		t.Fatalf("%s=%v, want nonzero finite %v", name, got, want)
	}
	ulp := math.Abs(math.Nextafter(want, math.Inf(1)) - want)
	limit := math.Max(1e-12*math.Abs(want), ulp)
	if math.Abs(got-want) > limit {
		t.Fatalf("%s=%g, want %g (limit %g)", name, got, want, limit)
	}
}

func thresholdTailAssertRows(t *testing.T, name string, rows [][]float64, wantRows, wantCols int) {
	t.Helper()
	if len(rows) != wantRows {
		t.Fatalf("%s rows=%d, want %d", name, len(rows), wantRows)
	}
	for row, values := range rows {
		if len(values) != wantCols {
			t.Fatalf("%s[%d] length=%d, want %d", name, row, len(values), wantCols)
		}
		for column, value := range values {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				t.Fatalf("%s[%d][%d]=%v, want finite", name, row, column, value)
			}
		}
	}
}

func thresholdTailAssertLIFForward(t *testing.T, c thresholdTailCase, outputs, spikes, voltages [][]float64) {
	t.Helper()
	thresholdTailAssertRows(t, "outputs", outputs, 1, 1)
	thresholdTailAssertRows(t, "spikes", spikes, 1, 1)
	thresholdTailAssertRows(t, "voltages", voltages, 1, 1)
	if outputs[0][0] != c.spike || spikes[0][0] != c.spike || voltages[0][0] != c.voltage {
		t.Fatalf("forward outputs=%v spikes=%v voltages=%v, want [%v] [%v] [%v]", outputs, spikes, voltages, c.spike, c.spike, c.voltage)
	}
}

func thresholdTailAssertLIFRecomputeOutputs(t *testing.T, c thresholdTailCase, outputs [][]float64) {
	t.Helper()
	thresholdTailAssertRows(t, "recompute outputs", outputs, 1, 1)
	if outputs[0][0] != c.spike {
		t.Fatalf("recompute outputs=%v, want [[%v]]", outputs, c.spike)
	}
}

func thresholdTailAssertLIFGradient(t *testing.T, g LIFGradient, c thresholdTailCase) {
	t.Helper()
	if len(g.Weights) != 0 || len(g.Bias) != 1 || len(g.LogTau) != 1 || len(g.ThetaRaw) != 1 || len(g.Initial) != 1 || len(g.Inputs) != 1 || len(g.Inputs[0]) != 1 {
		t.Fatalf("gradient shape weights=%d bias=%d logtau=%d theta=%d initial=%d inputs=%d, want 0,1,1,1,1,1", len(g.Weights), len(g.Bias), len(g.LogTau), len(g.ThetaRaw), len(g.Initial), len(g.Inputs))
	}
	thresholdTailCheckFloat(t, "bias", g.Bias[0], c.bias)
	thresholdTailCheckFloat(t, "log_tau", g.LogTau[0], c.logTau)
	thresholdTailCheckFloat(t, "theta_raw", g.ThetaRaw[0], c.thetaRaw)
	thresholdTailCheckFloat(t, "input", g.Inputs[0][0], c.inputGradient)
	thresholdTailCheckFloat(t, "initial", g.Initial[0], c.initialGradient)
}

func thresholdTailAssertLIFCoefficients(t *testing.T, m *LIF, p LIFParameters, minimum, maximum, base, slope float64) {
	t.Helper()
	lambda, alpha, gotBase, gotSlope, err := m.lifConstants(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(lambda) != 1 || len(alpha) != 1 || len(gotBase) != 1 || len(gotSlope) != 1 {
		t.Fatalf("lifConstants lengths lambda=%d alpha=%d base=%d slope=%d, want one each", len(lambda), len(alpha), len(gotBase), len(gotSlope))
	}
	thresholdTailCheckFloat(t, "lambda", lambda[0], .5)
	thresholdTailCheckFloat(t, "alpha", alpha[0], .5)
	thresholdTailCheckCoefficient(t, "lifConstants base", gotBase[0], base)
	thresholdTailCheckCoefficient(t, "lifConstants slope", gotSlope[0], slope)
	baseOnly, err := m.lifBaseThreshold(p.ThetaRaw)
	if err != nil {
		t.Fatal(err)
	}
	if len(baseOnly) != 1 {
		t.Fatalf("lifBaseThreshold length=%d, want 1", len(baseOnly))
	}
	thresholdTailCheckCoefficient(t, "lifBaseThreshold base", baseOnly[0], base)
	mm, err := NewMixed(MixedConfig{Nodes: 1, DT: math.Ln2, NodeRule: []uint8{RuleLIF}, LIF: LIFRule{
		TauSyn: 1, ThetaMin: minimum, ThetaMax: maximum, VReset: m.config.VReset,
		Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 1},
	}})
	if err != nil {
		t.Fatal(err)
	}
	mbase, mslope, err := mm.baseThreshold(p.ThetaRaw, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(mbase) != 1 || len(mslope) != 1 {
		t.Fatalf("mixed baseThreshold lengths base=%d slope=%d, want one each", len(mbase), len(mslope))
	}
	thresholdTailCheckCoefficient(t, "mixed baseThreshold base", mbase[0], base)
	thresholdTailCheckCoefficient(t, "mixed baseThreshold slope", mslope[0], slope)
}

func thresholdTailRunLIF(t *testing.T, c thresholdTailCase, route string) {
	t.Helper()
	m, err := NewLIF(thresholdTailLIFConfig(c))
	if err != nil {
		t.Fatal(err)
	}
	p := thresholdTailParameters(c)
	initial := []float64{c.initial}
	inputs := [][]float64{{c.input}}
	upstream := [][]float64{{1}}
	switch route {
	case "ordinary":
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			t.Fatal(err)
		}
		thresholdTailAssertLIFForward(t, c, tr.Outputs(), tr.Spikes(), tr.Voltages())
		g, err := m.Backward(context.Background(), tr, upstream, 0)
		if err != nil {
			t.Fatal(err)
		}
		thresholdTailAssertLIFGradient(t, g, c)
	case "recompute-1", "recompute-2":
		segment := 1
		if route == "recompute-2" {
			segment = 2
		}
		outputs, err := m.ForwardRecompute(context.Background(), p, initial, inputs, segment)
		if err != nil {
			t.Fatal(err)
		}
		thresholdTailAssertLIFRecomputeOutputs(t, c, outputs)
		g, err := m.BackwardRecompute(context.Background(), p, initial, inputs, upstream, 0, segment)
		if err != nil {
			t.Fatal(err)
		}
		thresholdTailAssertLIFGradient(t, g, c)
	case "coupled":
		tr, err := m.ForwardCoupled(context.Background(), p, initial, inputs, &zeroTauSequence{mods: zeroTauMods(1), nodes: 1})
		if err != nil {
			t.Fatal(err)
		}
		thresholdTailAssertLIFForward(t, c, tr.Outputs(), tr.Spikes(), tr.Voltages())
		g, err := m.Backward(context.Background(), tr, upstream, 0)
		if err != nil {
			t.Fatal(err)
		}
		thresholdTailAssertLIFGradient(t, g, c)
	case "coupled-recompute":
		tr, err := m.ForwardCoupledRecompute(context.Background(), p, initial, inputs, &zeroTauRecomputeSequence{mods: zeroTauMods(1), nodes: 1}, 1)
		if err != nil {
			t.Fatal(err)
		}
		finalVoltage := tr.FinalVoltage()
		if len(finalVoltage) != 1 {
			t.Fatalf("coupled recompute final voltage length=%d, want 1", len(finalVoltage))
		}
		thresholdTailAssertLIFForward(t, c, tr.Outputs(), tr.Outputs(), [][]float64{finalVoltage})
		g, err := m.BackwardCoupledRecompute(context.Background(), tr, upstream, 0)
		if err != nil {
			t.Fatal(err)
		}
		thresholdTailAssertLIFGradient(t, g, c)
	default:
		t.Fatalf("unknown route %q", route)
	}
}

func TestThresholdTailLIFPaths(t *testing.T) {
	for _, c := range thresholdTailCases() {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Run("coefficients", func(t *testing.T) {
				m, err := NewLIF(thresholdTailLIFConfig(c))
				if err != nil {
					t.Fatal(err)
				}
				thresholdTailAssertLIFCoefficients(t, m, thresholdTailParameters(c), c.minimum, c.maximum, c.base, c.slope)
			})
			for _, route := range []string{"ordinary", "recompute-1", "recompute-2", "coupled", "coupled-recompute"} {
				route := route
				t.Run(route, func(t *testing.T) { thresholdTailRunLIF(t, c, route) })
			}
		})
	}
}

func TestThresholdTailAdditionalCoefficientOracles(t *testing.T) {
	for _, tc := range thresholdTailCoefficientCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			reset := -1.0
			if tc.minimum < 0 {
				reset = -math.MaxFloat64
			}
			c := thresholdTailCase{
				minimum: tc.minimum, maximum: tc.maximum, raw: tc.raw, reset: reset,
			}
			m, err := NewLIF(thresholdTailLIFConfig(c))
			if err != nil {
				t.Fatal(err)
			}
			thresholdTailAssertLIFCoefficients(t, m, thresholdTailParameters(c), tc.minimum, tc.maximum, tc.base, tc.slope)
		})
	}
}

func thresholdTailAssertTwoRowLIFGradient(t *testing.T, g LIFGradient, psi, slope float64) {
	t.Helper()
	if len(g.Weights) != 0 || len(g.Bias) != 1 || len(g.LogTau) != 1 || len(g.ThetaRaw) != 1 || len(g.Initial) != 1 || len(g.Inputs) != 2 || len(g.Inputs[0]) != 1 || len(g.Inputs[1]) != 1 {
		t.Fatalf("two-row gradient shape is invalid: %+v", g)
	}
	thresholdTailCheckFloat(t, "two-row bias", g.Bias[0], .75*psi)
	thresholdTailCheckFloat(t, "two-row log_tau", g.LogTau[0], -.5*math.Ln2*psi)
	thresholdTailCheckFloat(t, "two-row theta_raw", g.ThetaRaw[0], -psi*slope)
	thresholdTailCheckFloat(t, "two-row input[0]", g.Inputs[0][0], .25*psi)
	thresholdTailCheckFloat(t, "two-row input[1]", g.Inputs[1][0], .5*psi)
	thresholdTailCheckFloat(t, "two-row initial", g.Initial[0], .25*psi)
}

func TestThresholdTailLIFRecomputeCrossSegment(t *testing.T) {
	c := thresholdTailCases()[4] // raw=40, [0,2], base=2 and nonzero slope.
	m, err := NewLIF(thresholdTailLIFConfig(c))
	if err != nil {
		t.Fatal(err)
	}
	p := thresholdTailParameters(c)
	initial := []float64{0}
	inputs := [][]float64{{0}, {1}}
	upstream := [][]float64{{-.5}, {1}}
	psi := 1 / math.Pow(1+math.Abs(.5-c.base), 2)
	for _, segment := range []int{1, 2} {
		t.Run("segment="+string(rune('0'+segment)), func(t *testing.T) {
			outputs, err := m.ForwardRecompute(context.Background(), p, initial, inputs, segment)
			if err != nil {
				t.Fatal(err)
			}
			thresholdTailAssertRows(t, "outputs", outputs, 2, 1)
			if outputs[0][0] != 0 || outputs[1][0] != 0 {
				t.Fatalf("outputs=%v, want [[0] [0]]", outputs)
			}
			g, err := m.BackwardRecompute(context.Background(), p, initial, inputs, upstream, 0, segment)
			if err != nil {
				t.Fatal(err)
			}
			thresholdTailAssertTwoRowLIFGradient(t, g, psi, c.slope)
		})
	}
}

func thresholdTailMixedConfig(c thresholdTailCase, continuous bool) MixedConfig {
	lif := LIFRule{TauSyn: 1, ThetaMin: c.minimum, ThetaMax: c.maximum, VReset: c.reset,
		Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 1}}
	if !continuous {
		return MixedConfig{Nodes: 1, DT: math.Ln2, NodeRule: []uint8{RuleLIF}, LIF: lif}
	}
	return MixedConfig{Nodes: 2, DT: math.Ln2, NodeRule: []uint8{RuleContinuous, RuleLIF},
		Continuous: ContinuousRule{Activation: "tanh"}, LIF: lif}
}

func thresholdTailRunMixed(t *testing.T, c thresholdTailCase, continuous, coupled bool) {
	t.Helper()
	m, err := NewMixed(thresholdTailMixedConfig(c, continuous))
	if err != nil {
		t.Fatal(err)
	}
	var p MixedParameters
	var initial, input, upstream []float64
	if continuous {
		p = MixedParameters{Weights: []float64{}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}, ThetaRaw: []float64{c.raw}}
		initial, input, upstream = []float64{0, c.initial}, []float64{0, c.input}, []float64{0, 1}
	} else {
		p = MixedParameters{Weights: []float64{}, Bias: []float64{0}, LogTau: []float64{0}, ThetaRaw: []float64{c.raw}}
		initial, input, upstream = []float64{c.initial}, []float64{c.input}, []float64{1}
	}
	inputs, upstreamRows := [][]float64{input}, [][]float64{upstream}
	var tr *MixedTrace
	if coupled {
		tr, err = m.ForwardCoupled(context.Background(), p, initial, inputs, &zeroTauSequence{mods: zeroTauMods(1), nodes: len(initial)})
	} else {
		tr, err = m.Forward(context.Background(), p, initial, inputs)
	}
	if err != nil {
		t.Fatal(err)
	}
	outputs, spikes, voltages := tr.Outputs(), tr.Spikes(), tr.Voltages()
	thresholdTailAssertRows(t, "mixed outputs", outputs, 1, len(initial))
	thresholdTailAssertRows(t, "mixed spikes", spikes, 1, len(initial))
	thresholdTailAssertRows(t, "mixed voltages", voltages, 1, len(initial))
	if continuous {
		if outputs[0][0] != 0 || spikes[0][0] != 0 || voltages[0][0] != 0 || outputs[0][1] != c.spike || spikes[0][1] != c.spike || voltages[0][1] != c.voltage {
			t.Fatalf("mixed outputs=%v spikes=%v voltages=%v", outputs, spikes, voltages)
		}
	} else if outputs[0][0] != c.spike || spikes[0][0] != c.spike || voltages[0][0] != c.voltage {
		t.Fatalf("all-LIF outputs=%v spikes=%v voltages=%v", outputs, spikes, voltages)
	}
	g, err := m.Backward(context.Background(), tr, upstreamRows, 0)
	if err != nil {
		t.Fatal(err)
	}
	if continuous {
		if len(g.Weights) != 0 || len(g.Bias) != 2 || len(g.LogTau) != 2 || len(g.ThetaRaw) != 1 || len(g.Initial) != 2 || len(g.Inputs) != 1 || len(g.Inputs[0]) != 2 {
			t.Fatalf("continuous mixed gradient shape=%+v", g)
		}
		thresholdTailCheckFloat(t, "continuous bias", g.Bias[0], 0)
		thresholdTailCheckFloat(t, "continuous log_tau", g.LogTau[0], 0)
		thresholdTailCheckFloat(t, "continuous initial", g.Initial[0], 0)
		thresholdTailCheckFloat(t, "continuous input", g.Inputs[0][0], 0)
		thresholdTailCheckFloat(t, "mixed LIF bias", g.Bias[1], c.bias)
		thresholdTailCheckFloat(t, "mixed LIF log_tau", g.LogTau[1], c.logTau)
		thresholdTailCheckFloat(t, "mixed LIF theta", g.ThetaRaw[0], c.thetaRaw)
		thresholdTailCheckFloat(t, "mixed LIF initial", g.Initial[1], c.initialGradient)
		thresholdTailCheckFloat(t, "mixed LIF input", g.Inputs[0][1], c.inputGradient)
		return
	}
	if len(g.Weights) != 0 || len(g.Bias) != 1 || len(g.LogTau) != 1 || len(g.ThetaRaw) != 1 || len(g.Initial) != 1 || len(g.Inputs) != 1 || len(g.Inputs[0]) != 1 {
		t.Fatalf("all-LIF mixed gradient shape=%+v", g)
	}
	thresholdTailCheckFloat(t, "all-LIF bias", g.Bias[0], c.bias)
	thresholdTailCheckFloat(t, "all-LIF log_tau", g.LogTau[0], c.logTau)
	thresholdTailCheckFloat(t, "all-LIF theta", g.ThetaRaw[0], c.thetaRaw)
	thresholdTailCheckFloat(t, "all-LIF initial", g.Initial[0], c.initialGradient)
	thresholdTailCheckFloat(t, "all-LIF input", g.Inputs[0][0], c.inputGradient)
}

func TestThresholdTailMixedPaths(t *testing.T) {
	for _, c := range thresholdTailCases() {
		c := c
		t.Run(c.name, func(t *testing.T) {
			for _, route := range []struct {
				name       string
				continuous bool
				coupled    bool
			}{
				{name: "all-LIF/ordinary"},
				{name: "all-LIF/coupled", coupled: true},
				{name: "continuous-plus-LIF/ordinary", continuous: true},
				{name: "continuous-plus-LIF/coupled", continuous: true, coupled: true},
			} {
				route := route
				t.Run(route.name, func(t *testing.T) { thresholdTailRunMixed(t, c, route.continuous, route.coupled) })
			}
		})
	}
}

func thresholdTailAssertLIFAdvance(t *testing.T, c thresholdTailCase, state LIFState, next LIFState, outputs, spikes [][]float64) {
	t.Helper()
	if state.Steps != 0 || len(state.Voltage) != 1 || state.Voltage[0] != c.initial || len(state.History) != 1 || len(state.History[0]) != 1 || state.History[0][0] != 0 || len(state.Adaptation) != 1 || state.Adaptation[0] != 0 || len(state.Refractory) != 1 || state.Refractory[0] != 0 {
		t.Fatalf("Advance mutated input LIF state: %+v", state)
	}
	if next.SchemaVersion != LIFStateVersion || len(next.Voltage) != 1 || next.Voltage[0] != c.voltage {
		t.Fatalf("Advance next state=%+v, want voltage [%v]", next, c.voltage)
	}
	thresholdTailAssertRows(t, "advance outputs", outputs, 1, 1)
	thresholdTailAssertRows(t, "advance spikes", spikes, 1, 1)
	if outputs[0][0] != c.spike || spikes[0][0] != c.spike {
		t.Fatalf("Advance outputs=%v spikes=%v, want [[%v]]", outputs, spikes, c.spike)
	}
}

func thresholdTailAssertMixedAdvance(t *testing.T, c thresholdTailCase, state, next MixedState, outputs, spikes [][]float64, continuous bool) {
	t.Helper()
	if state.Steps != 0 || state.Continuous.Steps != 0 || state.LIF.Steps != 0 || len(state.LIF.Voltage) != 1 || state.LIF.Voltage[0] != c.initial || len(state.LIF.History) != 1 || state.LIF.History[0][0] != 0 || len(state.LIF.Adaptation) != 1 || state.LIF.Adaptation[0] != 0 || len(state.LIF.Refractory) != 1 || state.LIF.Refractory[0] != 0 {
		t.Fatalf("Advance mutated input Mixed state: %+v", state)
	}
	if continuous {
		if len(state.Continuous.Voltage) != 1 || state.Continuous.Voltage[0] != 0 || len(state.Continuous.History) != 1 || len(state.Continuous.History[0]) != 1 || state.Continuous.History[0][0] != 0 {
			t.Fatalf("Advance mutated continuous input state: %+v", state.Continuous)
		}
	} else if len(state.Continuous.Voltage) != 0 || len(state.Continuous.History) != 1 || len(state.Continuous.History[0]) != 0 {
		t.Fatalf("all-LIF input state unexpectedly owns continuous values: %+v", state.Continuous)
	}
	if next.SchemaVersion != MixedStateVersion || len(next.LIF.Voltage) != 1 || next.LIF.Voltage[0] != c.voltage {
		t.Fatalf("Advance next mixed state=%+v, want LIF voltage [%v]", next, c.voltage)
	}
	width := 1
	if continuous {
		width = 2
	}
	thresholdTailAssertRows(t, "mixed advance outputs", outputs, 1, width)
	thresholdTailAssertRows(t, "mixed advance spikes", spikes, 1, width)
	if continuous {
		if outputs[0][0] != 0 || spikes[0][0] != 0 || outputs[0][1] != c.spike || spikes[0][1] != c.spike {
			t.Fatalf("continuous Mixed Advance outputs=%v spikes=%v", outputs, spikes)
		}
		return
	}
	if outputs[0][0] != c.spike || spikes[0][0] != c.spike {
		t.Fatalf("all-LIF Mixed Advance outputs=%v spikes=%v", outputs, spikes)
	}
}

func TestThresholdTailAdvanceFiniteForwardCases(t *testing.T) {
	cases := []thresholdTailCase{
		{name: "wide-negative--750", maximum: 1e308, raw: -750, reset: -1},
		{name: "wide-negative--1000", maximum: 1e308, raw: -1000, reset: -1},
		{name: "forward-cancellation-positive-40", minimum: -1e308, raw: 40, input: -1e200, reset: -math.MaxFloat64, spike: 1, voltage: -math.MaxFloat64},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Run("lif-forward-and-advance", func(t *testing.T) {
				m, err := NewLIF(thresholdTailLIFConfig(c))
				if err != nil {
					t.Fatal(err)
				}
				p := thresholdTailParameters(c)
				tr, err := m.Forward(context.Background(), p, []float64{c.initial}, [][]float64{{c.input}})
				if err != nil {
					t.Fatal(err)
				}
				thresholdTailAssertLIFForward(t, c, tr.Outputs(), tr.Spikes(), tr.Voltages())
				state, err := m.NewState([]float64{c.initial})
				if err != nil {
					t.Fatal(err)
				}
				next, outputs, spikes, err := m.Advance(context.Background(), p, state, [][]float64{{c.input}})
				if err != nil {
					t.Fatal(err)
				}
				thresholdTailAssertLIFAdvance(t, c, state, next, outputs, spikes)
			})
			t.Run("mixed-all-LIF", func(t *testing.T) {
				m, err := NewMixed(thresholdTailMixedConfig(c, false))
				if err != nil {
					t.Fatal(err)
				}
				p := MixedParameters{Weights: []float64{}, Bias: []float64{0}, LogTau: []float64{0}, ThetaRaw: []float64{c.raw}}
				tr, err := m.Forward(context.Background(), p, []float64{c.initial}, [][]float64{{c.input}})
				if err != nil {
					t.Fatal(err)
				}
				thresholdTailAssertRows(t, "mixed forward outputs", tr.Outputs(), 1, 1)
				thresholdTailAssertRows(t, "mixed forward spikes", tr.Spikes(), 1, 1)
				thresholdTailAssertRows(t, "mixed forward voltages", tr.Voltages(), 1, 1)
				if tr.Outputs()[0][0] != c.spike || tr.Spikes()[0][0] != c.spike || tr.Voltages()[0][0] != c.voltage {
					t.Fatalf("mixed forward outputs=%v spikes=%v voltages=%v", tr.Outputs(), tr.Spikes(), tr.Voltages())
				}
				state, err := m.NewState([]float64{c.initial})
				if err != nil {
					t.Fatal(err)
				}
				next, outputs, spikes, err := m.Advance(context.Background(), p, state, [][]float64{{c.input}})
				if err != nil {
					t.Fatal(err)
				}
				thresholdTailAssertMixedAdvance(t, c, state, next, outputs, spikes, false)
			})
			t.Run("mixed-continuous-plus-LIF", func(t *testing.T) {
				m, err := NewMixed(thresholdTailMixedConfig(c, true))
				if err != nil {
					t.Fatal(err)
				}
				p := MixedParameters{Weights: []float64{}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}, ThetaRaw: []float64{c.raw}}
				initial := []float64{0, c.initial}
				inputs := [][]float64{{0, c.input}}
				tr, err := m.Forward(context.Background(), p, initial, inputs)
				if err != nil {
					t.Fatal(err)
				}
				thresholdTailAssertRows(t, "mixed forward outputs", tr.Outputs(), 1, 2)
				thresholdTailAssertRows(t, "mixed forward spikes", tr.Spikes(), 1, 2)
				thresholdTailAssertRows(t, "mixed forward voltages", tr.Voltages(), 1, 2)
				if tr.Outputs()[0][0] != 0 || tr.Spikes()[0][0] != 0 || tr.Voltages()[0][0] != 0 || tr.Outputs()[0][1] != c.spike || tr.Spikes()[0][1] != c.spike || tr.Voltages()[0][1] != c.voltage {
					t.Fatalf("mixed forward outputs=%v spikes=%v voltages=%v", tr.Outputs(), tr.Spikes(), tr.Voltages())
				}
				state, err := m.NewState(initial)
				if err != nil {
					t.Fatal(err)
				}
				next, outputs, spikes, err := m.Advance(context.Background(), p, state, inputs)
				if err != nil {
					t.Fatal(err)
				}
				thresholdTailAssertMixedAdvance(t, c, state, next, outputs, spikes, true)
			})
		})
	}
}

func TestThresholdTailForwardErrorsAndAdvance(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  float64
	}{
		{name: "nan-raw", raw: math.NaN()},
		{name: "positive-infinite-raw", raw: math.Inf(1)},
		{name: "negative-infinite-raw", raw: math.Inf(-1)},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			c := thresholdTailCase{maximum: 2, raw: tc.raw, reset: -1}
			p := thresholdTailParameters(c)
			inputs := [][]float64{{0}}
			m, err := NewLIF(thresholdTailLIFConfig(c))
			if err != nil {
				t.Fatal(err)
			}
			if tr, err := m.Forward(context.Background(), p, []float64{0}, inputs); err == nil || tr != nil {
				t.Fatalf("LIF Forward trace=%v err=%v, want nil trace and error", tr, err)
			}
			if outputs, err := m.ForwardRecompute(context.Background(), p, []float64{0}, inputs, 1); err == nil || outputs != nil {
				t.Fatalf("LIF ForwardRecompute outputs=%v err=%v, want nil outputs and error", outputs, err)
			}
			if tr, err := m.ForwardCoupled(context.Background(), p, []float64{0}, inputs, &zeroTauSequence{mods: zeroTauMods(1), nodes: 1}); err == nil || tr != nil {
				t.Fatalf("LIF ForwardCoupled trace=%v err=%v, want nil trace and error", tr, err)
			}
			state, err := m.NewState([]float64{0})
			if err != nil {
				t.Fatal(err)
			}
			if next, outputs, spikes, err := m.Advance(context.Background(), p, state, inputs); err == nil || next.SchemaVersion != "" || outputs != nil || spikes != nil {
				t.Fatalf("LIF Advance state=%+v outputs=%v spikes=%v err=%v, want zero result and error", next, outputs, spikes, err)
			}

			mm, err := NewMixed(thresholdTailMixedConfig(c, false))
			if err != nil {
				t.Fatal(err)
			}
			mp := MixedParameters{Weights: []float64{}, Bias: []float64{0}, LogTau: []float64{0}, ThetaRaw: []float64{tc.raw}}
			if tr, err := mm.Forward(context.Background(), mp, []float64{0}, inputs); err == nil || tr != nil {
				t.Fatalf("Mixed Forward trace=%v err=%v, want nil trace and error", tr, err)
			}
			if tr, err := mm.ForwardCoupled(context.Background(), mp, []float64{0}, inputs, &zeroTauSequence{mods: zeroTauMods(1), nodes: 1}); err == nil || tr != nil {
				t.Fatalf("Mixed ForwardCoupled trace=%v err=%v, want nil trace and error", tr, err)
			}
			mixedState, err := mm.NewState([]float64{0})
			if err != nil {
				t.Fatal(err)
			}
			if next, outputs, spikes, err := mm.Advance(context.Background(), mp, mixedState, inputs); err == nil || next.SchemaVersion != "" || outputs != nil || spikes != nil {
				t.Fatalf("Mixed Advance state=%+v outputs=%v spikes=%v err=%v, want zero result and error", next, outputs, spikes, err)
			}
		})
	}
}
