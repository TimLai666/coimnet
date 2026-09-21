// This file is an internal package test because the modulated advance has to be
// compared against the unmodulated one on the same package-private fixtures the
// persistent state tests use.
package dynamics

import (
	"context"
	"math"
	"strings"
	"testing"
)

// modulatedContinuousFixture is a delayed four node graph. Node 3 has no
// incoming edge and carries a negative zero as its input, bias and initial
// voltage, so its whole trajectory stays at a negative zero: that is the node
// where a neutral modulation shows up as a changed sign of zero if the neutral
// case is multiplied out rather than skipped, because (-0)*1 + 0 is +0.
func modulatedContinuousFixture() (Config, Parameters, []float64, [][]float64) {
	minusZero := math.Copysign(0, -1)
	c := Config{
		Nodes:      4,
		Sources:    []int{0, 1, 2, 0},
		Targets:    []int{1, 2, 0, 2},
		Delays:     []int{0, 1, 2, 1},
		DT:         0.4,
		Activation: "tanh",
	}
	p := Parameters{
		Weights: []float64{0.5, -0.25, 0.75, 0.125},
		Bias:    []float64{0.1, -0.2, 0.05, minusZero},
		LogTau:  []float64{0, 0.2, -0.1, 0.3},
	}
	initial := []float64{0.3, -0.4, 0.2, minusZero}
	inputs := [][]float64{
		{0.5, -0.25, 0.125, minusZero},
		{-1, 0.75, 0, minusZero},
		{0.25, 0.25, -0.5, minusZero},
		{0, -0.125, 1, minusZero},
		{2, 0, 0.5, minusZero},
	}
	return c, p, initial, inputs
}

// neutralModulation is gain 1, offset 0 and threshold 0 everywhere.
func neutralModulation(steps, nodes int, withThreshold bool) *Modulation {
	mod := &Modulation{Gain: make([][]float64, steps), Offset: make([][]float64, steps)}
	if withThreshold {
		mod.Threshold = make([][]float64, steps)
	}
	for t := range steps {
		mod.Gain[t] = make([]float64, nodes)
		for i := range mod.Gain[t] {
			mod.Gain[t][i] = 1
		}
		mod.Offset[t] = make([]float64, nodes)
		if withThreshold {
			mod.Threshold[t] = make([]float64, nodes)
		}
	}
	return mod
}

func assertRowsIdentical(t *testing.T, name string, got, want [][]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s has %d rows, want %d", name, len(got), len(want))
	}
	for r := range want {
		if len(got[r]) != len(want[r]) {
			t.Fatalf("%s row %d has %d values, want %d", name, r, len(got[r]), len(want[r]))
		}
		for i := range want[r] {
			if math.Float64bits(got[r][i]) != math.Float64bits(want[r][i]) {
				t.Fatalf("%s[%d][%d] = %v (bits %#x), want %v (bits %#x)", name, r, i,
					got[r][i], math.Float64bits(got[r][i]), want[r][i], math.Float64bits(want[r][i]))
			}
		}
	}
}

func assertVectorIdentical(t *testing.T, name string, got, want []float64) {
	t.Helper()
	assertRowsIdentical(t, name, [][]float64{got}, [][]float64{want})
}

// A nil modulation is the path that exists today, and a fully neutral one has
// to land on the same bits, sign of zero included.
func TestContinuousAdvanceModulatedNilAndNeutralMatchAdvance(t *testing.T) {
	c, p, initial, inputs := modulatedContinuousFixture()
	m, err := NewContinuous(c)
	if err != nil {
		t.Fatal(err)
	}
	start, err := m.NewState(initial)
	if err != nil {
		t.Fatal(err)
	}
	plainState, plainOut, err := m.Advance(context.Background(), p, start, inputs)
	if err != nil {
		t.Fatal(err)
	}
	nilState, nilOut, err := m.AdvanceModulated(context.Background(), p, start, inputs, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertRowsIdentical(t, "nil outputs", nilOut, plainOut)
	assertVectorIdentical(t, "nil voltage", nilState.Voltage, plainState.Voltage)
	assertRowsIdentical(t, "nil history", nilState.History, plainState.History)

	for _, withThreshold := range []bool{false, true} {
		mod := neutralModulation(len(inputs), c.Nodes, withThreshold)
		neutralState, neutralOut, err := m.AdvanceModulated(context.Background(), p, start, inputs, mod)
		if err != nil {
			t.Fatalf("threshold present %v: %v", withThreshold, err)
		}
		assertRowsIdentical(t, "neutral outputs", neutralOut, plainOut)
		assertVectorIdentical(t, "neutral voltage", neutralState.Voltage, plainState.Voltage)
		assertRowsIdentical(t, "neutral history", neutralState.History, plainState.History)
		if neutralState.Steps != plainState.Steps || neutralState.ConfigHash != plainState.ConfigHash {
			t.Fatalf("neutral continuation %+v against %+v", neutralState.Steps, plainState.Steps)
		}
	}

	// A modulation that leaves one array out is neutral on that array.
	partial := &Modulation{Offset: make([][]float64, len(inputs))}
	for t := range partial.Offset {
		partial.Offset[t] = make([]float64, c.Nodes)
	}
	partialState, partialOut, err := m.AdvanceModulated(context.Background(), p, start, inputs, partial)
	if err != nil {
		t.Fatal(err)
	}
	assertRowsIdentical(t, "offset only outputs", partialOut, plainOut)
	assertVectorIdentical(t, "offset only voltage", partialState.Voltage, plainState.Voltage)
}

func TestLIFAdvanceModulatedNilAndNeutralMatchAdvance(t *testing.T) {
	for _, adapt := range []bool{false, true} {
		for _, refractory := range []int{0, 2} {
			c, p, initial, inputs := lifRandomFixture(21, adapt, refractory)
			c.Homeostasis = &LIFHomeostasis{Enabled: true, TauRate: 2, TargetRate: 0.2, Eta: 0.5, HMax: 0.4}
			m, err := NewLIF(c)
			if err != nil {
				t.Fatal(err)
			}
			start, err := m.NewState(initial)
			if err != nil {
				t.Fatal(err)
			}
			plainState, plainOut, plainSpikes, err := m.Advance(context.Background(), p, start, inputs)
			if err != nil {
				t.Fatal(err)
			}
			events := 0
			for _, row := range plainSpikes {
				for _, v := range row {
					events += int(v)
				}
			}
			if events == 0 {
				t.Fatalf("adapt %v refractory %d: the fixture never spiked, so the comparison proves nothing", adapt, refractory)
			}
			nilState, nilOut, nilSpikes, err := m.AdvanceModulated(context.Background(), p, start, inputs, nil)
			if err != nil {
				t.Fatal(err)
			}
			assertRowsIdentical(t, "nil traces", nilOut, plainOut)
			assertRowsIdentical(t, "nil spikes", nilSpikes, plainSpikes)
			assertVectorIdentical(t, "nil voltage", nilState.Voltage, plainState.Voltage)
			assertVectorIdentical(t, "nil adaptation", nilState.Adaptation, plainState.Adaptation)
			assertVectorIdentical(t, "nil homeostasis", nilState.Homeostasis, plainState.Homeostasis)

			mod := neutralModulation(len(inputs), c.Nodes, true)
			neutralState, neutralOut, neutralSpikes, err := m.AdvanceModulated(context.Background(), p, start, inputs, mod)
			if err != nil {
				t.Fatal(err)
			}
			assertRowsIdentical(t, "neutral traces", neutralOut, plainOut)
			assertRowsIdentical(t, "neutral spikes", neutralSpikes, plainSpikes)
			assertVectorIdentical(t, "neutral voltage", neutralState.Voltage, plainState.Voltage)
			assertVectorIdentical(t, "neutral adaptation", neutralState.Adaptation, plainState.Adaptation)
			assertVectorIdentical(t, "neutral rate", neutralState.Rate, plainState.Rate)
			assertVectorIdentical(t, "neutral homeostasis", neutralState.Homeostasis, plainState.Homeostasis)
			for i := range neutralState.Refractory {
				if neutralState.Refractory[i] != plainState.Refractory[i] {
					t.Fatalf("neutral refractory[%d] = %d, want %d", i, neutralState.Refractory[i], plainState.Refractory[i])
				}
			}
		}
	}
}

// One hand-computed step on the continuous core. With tau = 1 and dt = 1 the
// membrane coefficients are lambda = exp(-1) and alpha = 1-exp(-1) =
// 0.6321205588285577, and the initial voltage is zero, so v(1) = alpha*drive.
//
//	node 0: gain 2, offset 0   -> drive 2*1 + 0 = 2,   v = 1.2642411176571153
//	node 1: gain 1, offset 0.5 -> drive 1*1 + 0.5 = 1.5, v = 0.9481808382428365
//
// The output reference applies this package's math.Tanh to those exact
// voltages. Go's transcendental implementation can differ by one ULP across
// supported CPUs, while the voltage arithmetic remains exact.
func TestContinuousAdvanceModulatedScalesAndOffsetsOneNode(t *testing.T) {
	c := Config{Nodes: 2, DT: 1, Activation: "tanh"}
	p := Parameters{Bias: []float64{0, 0}, LogTau: []float64{0, 0}}
	m, err := NewContinuous(c)
	if err != nil {
		t.Fatal(err)
	}
	start, err := m.NewState([]float64{0, 0})
	if err != nil {
		t.Fatal(err)
	}
	inputs := [][]float64{{1, 1}}
	plain, plainOut, err := m.Advance(context.Background(), p, start, inputs)
	if err != nil {
		t.Fatal(err)
	}
	mod := &Modulation{Gain: [][]float64{{2, 1}}, Offset: [][]float64{{0, 0.5}}}
	got, gotOut, err := m.AdvanceModulated(context.Background(), p, start, inputs, mod)
	if err != nil {
		t.Fatal(err)
	}
	wantVoltage := []float64{1.2642411176571153, 0.9481808382428365}
	assertVectorIdentical(t, "voltage", got.Voltage, wantVoltage)
	wantOutput := []float64{math.Tanh(wantVoltage[0]), math.Tanh(wantVoltage[1])}
	assertVectorIdentical(t, "output", gotOut[0], wantOutput)
	if got.Voltage[0] != 2*plain.Voltage[0] {
		t.Fatalf("a gain of 2 gave %v, want twice the unmodulated %v", got.Voltage[0], plain.Voltage[0])
	}
	if plainOut[0][0] == gotOut[0][0] {
		t.Fatalf("the gain changed nothing: %v", gotOut[0][0])
	}
}

// The modulated current is the external input plus the delayed synaptic
// contribution. The bias is a parameter of the neuron, not an input current, so
// it is added after the gain and offset.
func TestContinuousModulationScalesTheSynapticDriveButNotTheBias(t *testing.T) {
	// One isolated node with a bias and no current at all: scaling zero
	// current leaves the step bit for bit unmodulated.
	biasOnly, err := NewContinuous(Config{Nodes: 1, DT: 1, Activation: "tanh"})
	if err != nil {
		t.Fatal(err)
	}
	p := Parameters{Bias: []float64{0.25}, LogTau: []float64{0}}
	start, err := biasOnly.NewState([]float64{0})
	if err != nil {
		t.Fatal(err)
	}
	plain, _, err := biasOnly.Advance(context.Background(), p, start, [][]float64{{0}})
	if err != nil {
		t.Fatal(err)
	}
	scaled, _, err := biasOnly.AdvanceModulated(context.Background(), p, start, [][]float64{{0}},
		&Modulation{Gain: [][]float64{{2}}})
	if err != nil {
		t.Fatal(err)
	}
	assertVectorIdentical(t, "bias only voltage", scaled.Voltage, plain.Voltage)

	// One edge, no external input and no bias: the delayed contribution is
	// 0.5*tanh(1) = 0.3807970779778824, and a gain of 2 doubles it to
	// tanh(1), giving v = alpha*tanh(1) = 0.4814193234633218.
	wired, err := NewContinuous(Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{0}, DT: 1, Activation: "tanh"})
	if err != nil {
		t.Fatal(err)
	}
	q := Parameters{Weights: []float64{0.5}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}}
	wiredStart, err := wired.NewState([]float64{1, 0})
	if err != nil {
		t.Fatal(err)
	}
	wiredPlain, _, err := wired.Advance(context.Background(), q, wiredStart, [][]float64{{0, 0}})
	if err != nil {
		t.Fatal(err)
	}
	if wiredPlain.Voltage[1] != 0.2407096617316609 {
		t.Fatalf("the unmodulated synaptic step gave %v, want alpha*0.5*tanh(1) = 0.2407096617316609", wiredPlain.Voltage[1])
	}
	wiredMod, _, err := wired.AdvanceModulated(context.Background(), q, wiredStart, [][]float64{{0, 0}},
		&Modulation{Gain: [][]float64{{1, 2}}})
	if err != nil {
		t.Fatal(err)
	}
	if wiredMod.Voltage[1] != 0.4814193234633218 || wiredMod.Voltage[1] != 2*wiredPlain.Voltage[1] {
		t.Fatalf("the modulated synaptic step gave %v, want alpha*tanh(1) = 0.4814193234633218", wiredMod.Voltage[1])
	}
}

// lifOneNode is a single neuron with no edge: dt = 1, tau = 1, so
// alpha = 0.6321205588285577, and theta_raw = 0 gives
// theta_base = 0.1 + 1.9*0.5 = 1.05.
func lifOneNode(t *testing.T) (*LIF, LIFParameters, LIFState) {
	t.Helper()
	m, err := NewLIF(LIFConfig{
		Nodes: 1, DT: 1, TauSyn: 1, ThetaMin: 0.1, ThetaMax: 2, VReset: -0.5,
		Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	p := LIFParameters{Bias: []float64{0}, LogTau: []float64{0}, ThetaRaw: []float64{0}}
	s, err := m.NewState([]float64{0})
	if err != nil {
		t.Fatal(err)
	}
	return m, p, s
}

func lifSpiked(t *testing.T, m *LIF, p LIFParameters, s LIFState, input float64, mod *Modulation) bool {
	t.Helper()
	_, _, spikes, err := m.AdvanceModulated(context.Background(), p, s, [][]float64{{input}}, mod)
	if err != nil {
		t.Fatal(err)
	}
	return spikes[0][0] == 1
}

// A threshold offset suppresses an event the same fixture produces without it.
// The candidate membrane is alpha*2 = 1.2642411176571153, above the base
// threshold 1.05 but below 1.05 + 0.5 = 1.55.
func TestLIFAdvanceModulatedThresholdSuppressesASpike(t *testing.T) {
	m, p, s := lifOneNode(t)
	if !lifSpiked(t, m, p, s, 2, nil) {
		t.Fatal("the fixture did not spike without a threshold offset")
	}
	if lifSpiked(t, m, p, s, 2, &Modulation{Threshold: [][]float64{{0.5}}}) {
		t.Fatal("a threshold offset of 0.5 did not suppress the spike")
	}
	// Just under the candidate: 1.05 + 0.2 = 1.25 < 1.2642411176571153.
	if !lifSpiked(t, m, p, s, 2, &Modulation{Threshold: [][]float64{{0.2}}}) {
		t.Fatal("a threshold offset of 0.2 suppressed a spike it should not reach")
	}
	// A gain works the same way on the spiking core: alpha*1 = 0.63 is below
	// the base threshold, and doubling the current carries it over.
	if lifSpiked(t, m, p, s, 1, nil) {
		t.Fatal("the fixture spiked on an input it should not reach")
	}
	if !lifSpiked(t, m, p, s, 1, &Modulation{Gain: [][]float64{{2}}}) {
		t.Fatal("a gain of 2 did not carry the membrane over the threshold")
	}
}

// The effective threshold is floored at theta_min, so a large negative offset
// lands on 0.1 and not below it: a candidate of alpha*0.2 = 0.126 spikes and
// one of alpha*0.1 = 0.063 does not.
func TestLIFAdvanceModulatedThresholdFloorsAtThetaMin(t *testing.T) {
	m, p, s := lifOneNode(t)
	deep := &Modulation{Threshold: [][]float64{{-1e9}}}
	if lifSpiked(t, m, p, s, 0.2, nil) || lifSpiked(t, m, p, s, 0.1, nil) {
		t.Fatal("the unmodulated fixture spiked below its base threshold")
	}
	if !lifSpiked(t, m, p, s, 0.2, deep) {
		t.Fatalf("a candidate of %v did not reach the floored threshold 0.1", 0.12642411176571153)
	}
	if lifSpiked(t, m, p, s, 0.1, deep) {
		t.Fatalf("a candidate of %v spiked, so the threshold fell below theta_min 0.1", 0.06321205588285576)
	}
}

func TestAdvanceModulatedRejectsAMalformedModulation(t *testing.T) {
	c, p, initial, inputs := modulatedContinuousFixture()
	m, err := NewContinuous(c)
	if err != nil {
		t.Fatal(err)
	}
	start, err := m.NewState(initial)
	if err != nil {
		t.Fatal(err)
	}
	steps, nodes := len(inputs), c.Nodes
	shortRow := neutralModulation(steps, nodes, false)
	shortRow.Gain[1] = shortRow.Gain[1][:nodes-1]
	nonFiniteGain := neutralModulation(steps, nodes, false)
	nonFiniteGain.Gain[2][1] = math.NaN()
	nonFiniteOffset := neutralModulation(steps, nodes, false)
	nonFiniteOffset.Offset[0][0] = math.Inf(1)
	shortOffset := neutralModulation(steps, nodes, false)
	shortOffset.Offset = shortOffset.Offset[:steps-1]
	activeThreshold := neutralModulation(steps, nodes, true)
	activeThreshold.Threshold[3][2] = 0.25

	cases := []struct {
		name string
		mod  *Modulation
		want string
	}{
		{"gain rows", &Modulation{Gain: neutralModulation(steps-1, nodes, false).Gain}, "gain"},
		{"gain row width", shortRow, "gain"},
		{"non-finite gain", nonFiniteGain, "gain"},
		{"offset rows", shortOffset, "offset"},
		{"non-finite offset", nonFiniteOffset, "offset"},
		{"threshold on the continuous core", activeThreshold, "threshold"},
		{"threshold rows on the continuous core", &Modulation{Threshold: make([][]float64, steps-1)}, "threshold"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, out, err := m.AdvanceModulated(context.Background(), p, start, inputs, tc.mod)
			if err == nil {
				t.Fatalf("accepted %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
			if out != nil || got.Steps != 0 || got.Voltage != nil {
				t.Fatalf("a refused call returned %+v and %v", got, out)
			}
			if start.Steps != 0 || start.Voltage[0] != initial[0] {
				t.Fatalf("a refused call changed the state it was given: %+v", start)
			}
		})
	}

	lc, lp, lifInitial, lifInputs := lifRandomFixture(7, false, 0)
	lm, err := NewLIF(lc)
	if err != nil {
		t.Fatal(err)
	}
	lifStart, err := lm.NewState(lifInitial)
	if err != nil {
		t.Fatal(err)
	}
	lifSteps, lifNodes := len(lifInputs), lc.Nodes
	shortThreshold := neutralModulation(lifSteps, lifNodes, true)
	shortThreshold.Threshold[0] = shortThreshold.Threshold[0][:lifNodes-1]
	nonFiniteThreshold := neutralModulation(lifSteps, lifNodes, true)
	nonFiniteThreshold.Threshold[1][0] = math.Inf(-1)
	lifCases := []struct {
		name string
		mod  *Modulation
		want string
	}{
		{"threshold row width", shortThreshold, "threshold"},
		{"non-finite threshold", nonFiniteThreshold, "threshold"},
		{"gain rows", &Modulation{Gain: neutralModulation(lifSteps+1, lifNodes, false).Gain}, "gain"},
		{"offset row width", func() *Modulation {
			mod := neutralModulation(lifSteps, lifNodes, false)
			mod.Offset[2] = mod.Offset[2][:lifNodes-1]
			return mod
		}(), "offset"},
	}
	for _, tc := range lifCases {
		t.Run("lif "+tc.name, func(t *testing.T) {
			got, out, spikes, err := lm.AdvanceModulated(context.Background(), lp, lifStart, lifInputs, tc.mod)
			if err == nil {
				t.Fatalf("accepted %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
			if out != nil || spikes != nil || got.Steps != 0 || got.Voltage != nil {
				t.Fatalf("a refused call returned %+v", got)
			}
		})
	}

	// A threshold array that is present but entirely zero is neutral, not an
	// error, on the continuous core.
	if _, _, err := m.AdvanceModulated(context.Background(), p, start, inputs, neutralModulation(steps, nodes, true)); err != nil {
		t.Fatalf("an all-zero threshold was refused on the continuous core: %v", err)
	}
}
