// This test file is an internal package test because the mixed core carries the
// same package-private smooth spike mode the LIF core does: it exists only as a
// finite difference reference and is never a product mode. The hard event model
// is the product behaviour, and hard events are never finite differenced.
package dynamics

import (
	"context"
	"fmt"
	"math"
	"testing"
)

const mixedTol = 1e-12

func mixedNear(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol*(1+math.Abs(want)) {
		t.Fatalf("%s: got %.17g want %.17g", name, got, want)
	}
}

func mixedRows(t *testing.T, name string, got, want [][]float64, tol float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d steps want %d", name, len(got), len(want))
	}
	for s := range want {
		if len(got[s]) != len(want[s]) {
			t.Fatalf("%s[%d]: got %d nodes want %d", name, s, len(got[s]), len(want[s]))
		}
		for i := range want[s] {
			mixedNear(t, fmt.Sprintf("%s[%d][%d]", name, s, i), got[s][i], want[s][i], tol)
		}
	}
}

// mixedHandConfig is the three node fixture of root decision 2: node 0 is
// continuous, node 1 is LIF and node 2 is continuous. Edge 0 carries 0 -> 1 with
// delay 0 and edge 1 carries 1 -> 2 with delay 1, so one cross-type edge runs in
// each direction and the LIF -> continuous edge reads an older synaptic trace.
// dt = ln 2 with tau = tau_syn = 1 makes lambda = alpha = kappa = 0.5 exactly,
// and theta_raw = 0 with theta_min 0, theta_max 2 makes theta_base = 1 exactly.
func mixedHandConfig() MixedConfig {
	return MixedConfig{
		Nodes:      3,
		Sources:    []int{0, 1},
		Targets:    []int{1, 2},
		Delays:     []int{0, 1},
		DT:         math.Ln2,
		NodeRule:   []uint8{0, 1, 0},
		Continuous: ContinuousRule{Activation: "tanh"},
		LIF: LIFRule{
			TauSyn: 1, ThetaMin: 0, ThetaMax: 2, VReset: -1,
			RefractorySteps: 1,
			Surrogate:       LIFSurrogate{Kind: "fast_sigmoid", Scale: 1},
		},
	}
}

func mixedHandInputs() (MixedParameters, []float64, [][]float64) {
	p := MixedParameters{
		Weights:  []float64{4, 2},
		Bias:     []float64{0, 0, 0},
		LogTau:   []float64{0, 0, 0},
		ThetaRaw: []float64{0},
	}
	return p, []float64{0, 0, 0}, [][]float64{{4, 0, 0}, {0, 0, 0}, {0, 0, 0}, {0, 0, 0}}
}

// mixedHandTable is the hand computed expectation of mixedHandConfig over four
// steps. Every value was derived from the declared rules:
//
//	step 1: node 0 v = .5*0 + .5*4 = 2, out = tanh 2; node 1 drive = 4*out_0(0) = 0,
//	        cand = 0 < 1, no event, x = 0; node 2 drive = 2*x_1(0) = 0, v = 0.
//	step 2: node 0 v = .5*2 = 1, out = tanh 1; node 1 drive = 4*tanh 2,
//	        cand = 2*tanh 2 = 1.928... >= 1, event, v = v_reset = -1, x = 1,
//	        refractory counter = 1; node 2 drive = 2*x_1(0) = 0, v = 0.
//	step 3: node 0 v = .5, out = tanh .5; node 1 is refractory: it holds v_reset,
//	        ignores its drive and emits nothing, x = .5*1 = .5; node 2 reads the
//	        delay 1 edge at x_1(1) = 0, v = 0.
//	step 4: node 0 v = .25; node 1 leaves the refractory period,
//	        cand = .5*(-1) + .5*4*tanh .5 = -.5 + 2*tanh .5 = .4242... < 1, no event,
//	        x = .5*.5 = .25; node 2 reads the delay 1 edge at x_1(2) = 1,
//	        v = .5*0 + .5*2 = 1, out = tanh 1.
func mixedHandTable() (voltages, outputs, events [][]float64) {
	tanh := math.Tanh
	voltages = [][]float64{
		{2, 0, 0},
		{1, -1, 0},
		{.5, -1, 0},
		{.25, -.5 + 2*tanh(.5), 1},
	}
	outputs = [][]float64{
		{tanh(2), 0, 0},
		{tanh(1), 1, 0},
		{tanh(.5), .5, 0},
		{tanh(.25), .25, tanh(1)},
	}
	events = [][]float64{
		{0, 0, 0},
		{0, 1, 0},
		{0, 0, 0},
		{0, 0, 0},
	}
	return voltages, outputs, events
}

func TestMixedHandCalculatedThreeNodeGraph(t *testing.T) {
	m, err := NewMixed(mixedHandConfig())
	if err != nil {
		t.Fatal(err)
	}
	p, initial, inputs := mixedHandInputs()
	tr, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	voltages, outputs, events := mixedHandTable()
	mixedRows(t, "voltage", tr.Voltages(), voltages, mixedTol)
	mixedRows(t, "output", tr.Outputs(), outputs, mixedTol)
	mixedRows(t, "event", tr.Spikes(), events, mixedTol)

	// The same fixture through the persistent path, in one call and split in two,
	// reproduces the same table: Advance is the same step rule as Forward.
	state, err := m.NewState(initial)
	if err != nil {
		t.Fatal(err)
	}
	whole, got, gotEvents, err := m.Advance(context.Background(), p, state, inputs)
	if err != nil {
		t.Fatal(err)
	}
	mixedRows(t, "advance output", got, outputs, mixedTol)
	mixedRows(t, "advance event", gotEvents, events, mixedTol)
	first, firstOut, firstEvents, err := m.Advance(context.Background(), p, state, inputs[:2])
	if err != nil {
		t.Fatal(err)
	}
	second, secondOut, secondEvents, err := m.Advance(context.Background(), p, first, inputs[2:])
	if err != nil {
		t.Fatal(err)
	}
	assertRowsIdentical(t, "split output", append(append([][]float64{}, firstOut...), secondOut...), got)
	assertRowsIdentical(t, "split event", append(append([][]float64{}, firstEvents...), secondEvents...), gotEvents)
	assertVectorIdentical(t, "split continuous voltage", second.Continuous.Voltage, whole.Continuous.Voltage)
	assertVectorIdentical(t, "split lif voltage", second.LIF.Voltage, whole.LIF.Voltage)
	assertRowsIdentical(t, "split continuous history", second.Continuous.History, whole.Continuous.History)
	assertRowsIdentical(t, "split lif history", second.LIF.History, whole.LIF.History)
	if second.Steps != whole.Steps || whole.Steps != 4 {
		t.Fatalf("steps %d and %d, want 4", second.Steps, whole.Steps)
	}
}

// TestMixedNoDoubleCounting puts one LIF node in front of one continuous and one
// LIF target. Each target receives w*x once and there is no second path from the
// event itself: the hand computed voltages below only work if the drive of each
// target is exactly its own weight times the source's synaptic trace.
//
//	node 0 LIF, node 1 continuous, node 2 LIF; edges 0->1 (w 2) and 0->2 (w 3),
//	both delay 0, refractory 0.
//	step 1: node 0 cand = .5*4 = 2 >= 1, event, v = -1, x = 1;
//	        node 1 drive = 2*x_0(0) = 0; node 2 drive = 3*x_0(0) = 0.
//	step 2: node 0 cand = .5*(-1) = -.5, no event, x = .5;
//	        node 1 drive = 2*x_0(1) = 2, v = .5*0 + .5*2 = 1, out = tanh 1;
//	        node 2 drive = 3*x_0(1) = 3, cand = .5*3 = 1.5 >= 1, event, v = -1, x = 1.
//	step 3: node 0 x = .25; node 1 drive = 2*.5 = 1, v = .5*1 + .5*1 = 1;
//	        node 2 drive = 3*.5 = 1.5, cand = .5*(-1) + .5*1.5 = .25 < 1, v = .25, x = .5.
func TestMixedNoDoubleCounting(t *testing.T) {
	c := MixedConfig{
		Nodes: 3, Sources: []int{0, 0}, Targets: []int{1, 2}, Delays: []int{0, 0},
		DT: math.Ln2, NodeRule: []uint8{1, 0, 1},
		Continuous: ContinuousRule{Activation: "tanh"},
		LIF: LIFRule{
			TauSyn: 1, ThetaMin: 0, ThetaMax: 2, VReset: -1,
			Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 1},
		},
	}
	m, err := NewMixed(c)
	if err != nil {
		t.Fatal(err)
	}
	p := MixedParameters{Weights: []float64{2, 3}, Bias: []float64{0, 0, 0}, LogTau: []float64{0, 0, 0}, ThetaRaw: []float64{0, 0}}
	tr, err := m.Forward(context.Background(), p, []float64{0, 0, 0}, [][]float64{{4, 0, 0}, {0, 0, 0}, {0, 0, 0}})
	if err != nil {
		t.Fatal(err)
	}
	mixedRows(t, "voltage", tr.Voltages(), [][]float64{
		{-1, 0, 0},
		{-.5, 1, -1},
		{-.25, 1, .25},
	}, mixedTol)
	mixedRows(t, "output", tr.Outputs(), [][]float64{
		{1, 0, 0},
		{.5, math.Tanh(1), 1},
		{.25, math.Tanh(1), .5},
	}, mixedTol)
	mixedRows(t, "event", tr.Spikes(), [][]float64{
		{1, 0, 0},
		{0, 0, 1},
		{0, 0, 0},
	}, mixedTol)
}

// mixedTwinTopology is the shared topology of the bit identity fixtures.
func mixedTwinTopology() (nodes int, sources, targets, delays []int) {
	return 3, []int{0, 1, 2, 0}, []int{1, 2, 0, 2}, []int{0, 1, 2, 0}
}

func mixedContinuousTwin(activation string) (Config, MixedConfig) {
	nodes, sources, targets, delays := mixedTwinTopology()
	return Config{Nodes: nodes, Sources: sources, Targets: targets, Delays: delays, DT: .3, Activation: activation},
		MixedConfig{
			Nodes: nodes, Sources: sources, Targets: targets, Delays: delays, DT: .3,
			NodeRule:   []uint8{0, 0, 0},
			Continuous: ContinuousRule{Activation: activation},
		}
}

func mixedTwinParameters() (Parameters, []float64, [][]float64, [][]float64) {
	p := Parameters{Weights: []float64{.6, -.5, .4, .35}, Bias: []float64{.2, -.15, .1}, LogTau: []float64{.1, -.2, .3}}
	initial := []float64{.1, -.2, .05}
	inputs := [][]float64{{.5, -.3, .2}, {-.4, .6, .1}, {.3, .2, -.5}, {.8, -.1, .3}}
	up := [][]float64{{.2, -.3, .1}, {-.1, .5, .3}, {.4, .1, -.3}, {.3, -.4, .2}}
	return p, initial, inputs, up
}

func TestMixedAllContinuousRulesMatchContinuousBitForBit(t *testing.T) {
	for _, activation := range []string{"tanh", "softplus"} {
		t.Run(activation, func(t *testing.T) {
			cc, mc := mixedContinuousTwin(activation)
			reference, err := NewContinuous(cc)
			if err != nil {
				t.Fatal(err)
			}
			m, err := NewMixed(mc)
			if err != nil {
				t.Fatal(err)
			}
			p, initial, inputs, up := mixedTwinParameters()
			mp := MixedParameters{Weights: p.Weights, Bias: p.Bias, LogTau: p.LogTau}
			refTrace, err := reference.Forward(context.Background(), p, initial, inputs)
			if err != nil {
				t.Fatal(err)
			}
			gotTrace, err := m.Forward(context.Background(), mp, initial, inputs)
			if err != nil {
				t.Fatal(err)
			}
			assertRowsIdentical(t, "outputs", gotTrace.Outputs(), refTrace.Outputs())
			assertVectorIdentical(t, "final voltage", gotTrace.FinalVoltage(), refTrace.FinalVoltage())

			refGrad, err := reference.Backward(context.Background(), refTrace, up, 0)
			if err != nil {
				t.Fatal(err)
			}
			gotGrad, err := m.Backward(context.Background(), gotTrace, up, 0)
			if err != nil {
				t.Fatal(err)
			}
			assertVectorIdentical(t, "weight gradient", gotGrad.Weights, refGrad.Weights)
			assertVectorIdentical(t, "bias gradient", gotGrad.Bias, refGrad.Bias)
			assertVectorIdentical(t, "log_tau gradient", gotGrad.LogTau, refGrad.LogTau)
			assertVectorIdentical(t, "initial gradient", gotGrad.Initial, refGrad.Initial)
			assertRowsIdentical(t, "input gradient", gotGrad.Inputs, refGrad.Inputs)
			if len(gotGrad.ThetaRaw) != 0 {
				t.Fatalf("an assignment without a LIF node produced %d theta gradients", len(gotGrad.ThetaRaw))
			}

			refState, err := reference.NewState(initial)
			if err != nil {
				t.Fatal(err)
			}
			refNext, refOut, err := reference.Advance(context.Background(), p, refState, inputs)
			if err != nil {
				t.Fatal(err)
			}
			gotState, err := m.NewState(initial)
			if err != nil {
				t.Fatal(err)
			}
			gotNext, gotOut, gotEvents, err := m.Advance(context.Background(), mp, gotState, inputs)
			if err != nil {
				t.Fatal(err)
			}
			assertRowsIdentical(t, "advance outputs", gotOut, refOut)
			assertVectorIdentical(t, "advance voltage", gotNext.Continuous.Voltage, refNext.Voltage)
			assertRowsIdentical(t, "advance history", gotNext.Continuous.History, refNext.History)
			if gotNext.Steps != refNext.Steps {
				t.Fatalf("steps %d, want %d", gotNext.Steps, refNext.Steps)
			}
			for _, row := range gotEvents {
				for i, v := range row {
					if v != 0 {
						t.Fatalf("continuous node %d reported event %v", i, v)
					}
				}
			}
			if len(gotNext.LIF.Voltage) != 0 || len(gotNext.Index.LIFNodes) != 0 {
				t.Fatal("an all-continuous assignment produced a LIF half")
			}
		})
	}
}

func mixedLIFTwin(adapt bool, homeostasis *LIFHomeostasis) (LIFConfig, MixedConfig) {
	nodes, sources, targets, delays := mixedTwinTopology()
	rule := LIFRule{
		TauSyn: .8, ThetaMin: .2, ThetaMax: 1.5, VReset: -.4, RefractorySteps: 2,
		Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
	}
	if adapt {
		rule.Adaptation = LIFAdaptation{Enabled: true, TauAdapt: .9, Beta: .35}
	}
	rule.Homeostasis = homeostasis
	lc := LIFConfig{
		Nodes: nodes, Sources: sources, Targets: targets, Delays: delays, DT: .4,
		TauSyn: rule.TauSyn, ThetaMin: rule.ThetaMin, ThetaMax: rule.ThetaMax,
		VReset: rule.VReset, RefractorySteps: rule.RefractorySteps,
		Adaptation: rule.Adaptation, Homeostasis: homeostasis, Surrogate: rule.Surrogate,
	}
	mc := MixedConfig{
		Nodes: nodes, Sources: sources, Targets: targets, Delays: delays, DT: .4,
		NodeRule: []uint8{1, 1, 1}, LIF: rule,
	}
	return lc, mc
}

func TestMixedAllLIFRulesMatchLIFBitForBit(t *testing.T) {
	for _, tc := range []struct {
		name        string
		adapt       bool
		homeostasis *LIFHomeostasis
	}{
		{"plain", false, nil},
		{"adaptation", true, nil},
		{"adaptation_and_homeostasis", true, &LIFHomeostasis{Enabled: true, TauRate: 2, TargetRate: .2, Eta: .5, HMax: .4}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lc, mc := mixedLIFTwin(tc.adapt, tc.homeostasis)
			reference, err := NewLIF(lc)
			if err != nil {
				t.Fatal(err)
			}
			m, err := NewMixed(mc)
			if err != nil {
				t.Fatal(err)
			}
			p, initial, inputs, up := mixedTwinParameters()
			theta := []float64{.2, -.3, .5}
			lp := LIFParameters{Weights: p.Weights, Bias: p.Bias, LogTau: p.LogTau, ThetaRaw: theta}
			mp := MixedParameters{Weights: p.Weights, Bias: p.Bias, LogTau: p.LogTau, ThetaRaw: theta}

			refTrace, err := reference.Forward(context.Background(), lp, initial, inputs)
			if err != nil {
				t.Fatal(err)
			}
			gotTrace, err := m.Forward(context.Background(), mp, initial, inputs)
			if err != nil {
				t.Fatal(err)
			}
			assertMatchesLIFReference(t, "outputs", gotTrace.Outputs(), refTrace.Outputs())
			// The events are the hard 0/1 decisions, not membrane values, so they
			// stay exact in every build.
			assertRowsIdentical(t, "events", gotTrace.Spikes(), refTrace.Spikes())
			assertMatchesLIFReference(t, "voltages", gotTrace.Voltages(), refTrace.Voltages())

			refGrad, err := reference.Backward(context.Background(), refTrace, up, 0)
			if err != nil {
				t.Fatal(err)
			}
			gotGrad, err := m.Backward(context.Background(), gotTrace, up, 0)
			if err != nil {
				t.Fatal(err)
			}
			assertVectorMatchesLIFReference(t, "weight gradient", gotGrad.Weights, refGrad.Weights)
			assertVectorMatchesLIFReference(t, "bias gradient", gotGrad.Bias, refGrad.Bias)
			assertVectorMatchesLIFReference(t, "log_tau gradient", gotGrad.LogTau, refGrad.LogTau)
			assertVectorMatchesLIFReference(t, "theta_raw gradient", gotGrad.ThetaRaw, refGrad.ThetaRaw)
			assertVectorMatchesLIFReference(t, "initial gradient", gotGrad.Initial, refGrad.Initial)
			assertMatchesLIFReference(t, "input gradient", gotGrad.Inputs, refGrad.Inputs)

			refState, err := reference.NewState(initial)
			if err != nil {
				t.Fatal(err)
			}
			refNext, refOut, refEvents, err := reference.Advance(context.Background(), lp, refState, inputs)
			if err != nil {
				t.Fatal(err)
			}
			gotState, err := m.NewState(initial)
			if err != nil {
				t.Fatal(err)
			}
			gotNext, gotOut, gotEvents, err := m.Advance(context.Background(), mp, gotState, inputs)
			if err != nil {
				t.Fatal(err)
			}
			assertMatchesLIFReference(t, "advance outputs", gotOut, refOut)
			assertRowsIdentical(t, "advance events", gotEvents, refEvents)
			assertVectorMatchesLIFReference(t, "advance voltage", gotNext.LIF.Voltage, refNext.Voltage)
			assertMatchesLIFReference(t, "advance history", gotNext.LIF.History, refNext.History)
			// The adaptation and the slow stabiliser are driven by the events,
			// not by the membrane, so they stay exact in every build.
			assertVectorIdentical(t, "advance adaptation", gotNext.LIF.Adaptation, refNext.Adaptation)
			assertVectorIdentical(t, "advance rate", gotNext.LIF.Rate, refNext.Rate)
			assertVectorIdentical(t, "advance homeostasis", gotNext.LIF.Homeostasis, refNext.Homeostasis)
			for i := range refNext.Refractory {
				if gotNext.LIF.Refractory[i] != refNext.Refractory[i] {
					t.Fatalf("refractory[%d] = %d, want %d", i, gotNext.LIF.Refractory[i], refNext.Refractory[i])
				}
			}
			if len(gotNext.Continuous.Voltage) != 0 || len(gotNext.Index.ContinuousNodes) != 0 {
				t.Fatal("an all-LIF assignment produced a continuous half")
			}
		})
	}
}

// mixedOrderFixture is a four node graph with two rules on each side.
func mixedOrderFixture() (MixedConfig, MixedParameters, []float64, [][]float64) {
	c := MixedConfig{
		Nodes:      4,
		Sources:    []int{0, 1, 2, 3, 1},
		Targets:    []int{1, 2, 3, 0, 3},
		Delays:     []int{0, 1, 0, 2, 1},
		DT:         .4,
		NodeRule:   []uint8{0, 1, 0, 1},
		Continuous: ContinuousRule{Activation: "tanh"},
		LIF: LIFRule{
			TauSyn: .8, ThetaMin: .2, ThetaMax: 1.5, VReset: -.4, RefractorySteps: 1,
			Adaptation: LIFAdaptation{Enabled: true, TauAdapt: .9, Beta: .35},
			Surrogate:  LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
		},
	}
	p := MixedParameters{
		Weights:  []float64{.9, -.5, .8, .35, .6},
		Bias:     []float64{.2, -.15, .1, .05},
		LogTau:   []float64{.1, -.2, .3, 0},
		ThetaRaw: []float64{.2, -.3}, // nodes 1 and 3, ascending
	}
	initial := []float64{.1, -.2, .05, .3}
	inputs := [][]float64{
		{1.5, -.3, .2, .7}, {-.4, .6, 1.1, -.2}, {.3, .2, -.5, 1.4}, {.8, -.1, .3, .2},
	}
	return c, p, initial, inputs
}

func TestMixedNodeNumberingOrderIndependence(t *testing.T) {
	c, p, initial, inputs := mixedOrderFixture()
	// old node i becomes new node permutation[i]; the LIF nodes 1 and 3 become
	// new nodes 3 and 2, so the ascending theta_raw order flips.
	permutation := []int{1, 3, 0, 2}
	n := c.Nodes
	permuted := MixedConfig{
		Nodes: n, DT: c.DT, Continuous: c.Continuous, LIF: c.LIF,
		Sources: make([]int, len(c.Sources)), Targets: make([]int, len(c.Targets)),
		Delays: append([]int(nil), c.Delays...), NodeRule: make([]uint8, n),
	}
	for e := range c.Sources {
		permuted.Sources[e] = permutation[c.Sources[e]]
		permuted.Targets[e] = permutation[c.Targets[e]]
	}
	pp := MixedParameters{
		Weights: append([]float64(nil), p.Weights...),
		Bias:    make([]float64, n), LogTau: make([]float64, n),
	}
	permutedInitial := make([]float64, n)
	for i := range n {
		permuted.NodeRule[permutation[i]] = c.NodeRule[i]
		pp.Bias[permutation[i]] = p.Bias[i]
		pp.LogTau[permutation[i]] = p.LogTau[i]
		permutedInitial[permutation[i]] = initial[i]
	}
	// theta_raw follows the ascending LIF node order of the new numbering.
	type thetaEntry struct {
		node  int
		value float64
	}
	var entries []thetaEntry
	k := 0
	for i := range n {
		if c.NodeRule[i] == 1 {
			entries = append(entries, thetaEntry{permutation[i], p.ThetaRaw[k]})
			k++
		}
	}
	for node := range n {
		for _, entry := range entries {
			if entry.node == node {
				pp.ThetaRaw = append(pp.ThetaRaw, entry.value)
			}
		}
	}
	permutedInputs := make([][]float64, len(inputs))
	for t := range inputs {
		permutedInputs[t] = make([]float64, n)
		for i := range n {
			permutedInputs[t][permutation[i]] = inputs[t][i]
		}
	}

	m, err := NewMixed(c)
	if err != nil {
		t.Fatal(err)
	}
	pm, err := NewMixed(permuted)
	if err != nil {
		t.Fatal(err)
	}
	original, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := pm.Forward(context.Background(), pp, permutedInitial, permutedInputs)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range []struct {
		name       string
		got, want  [][]float64
		isVoltages bool
	}{
		{"output", renamed.Outputs(), original.Outputs(), false},
		{"event", renamed.Spikes(), original.Spikes(), false},
		{"voltage", renamed.Voltages(), original.Voltages(), false},
	} {
		for tt := range pair.want {
			for i := range n {
				got, want := pair.got[tt][permutation[i]], pair.want[tt][i]
				if math.Float64bits(got) != math.Float64bits(want) {
					t.Fatalf("%s[%d][node %d] = %.17g, want %.17g after renumbering", pair.name, tt, i, got, want)
				}
			}
		}
	}
	// A renumbering that changed nothing would make this test vacuous.
	spiked := false
	for _, row := range original.Spikes() {
		for _, v := range row {
			if v != 0 {
				spiked = true
			}
		}
	}
	if !spiked {
		t.Fatal("the order independence fixture produced no event at all")
	}
}

func mixedSmoothFixture() (MixedConfig, MixedParameters, []float64, [][]float64, [][]float64) {
	c := MixedConfig{
		Nodes:      4,
		Sources:    []int{0, 1, 2, 3, 1, 0},
		Targets:    []int{1, 2, 3, 0, 3, 2},
		Delays:     []int{0, 1, 2, 0, 1, 2},
		DT:         .4,
		NodeRule:   []uint8{0, 1, 0, 1},
		Continuous: ContinuousRule{Activation: "tanh"},
		LIF: LIFRule{
			TauSyn: .8, ThetaMin: .2, ThetaMax: 1.5, VReset: -.4, RefractorySteps: 2,
			Adaptation: LIFAdaptation{Enabled: true, TauAdapt: .9, Beta: .35},
			Surrogate:  LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
		},
	}
	p := MixedParameters{
		Weights:  []float64{.6, -.5, .4, .35, -.3, .45},
		Bias:     []float64{.2, -.15, .1, .05},
		LogTau:   []float64{.1, -.2, .3, 0},
		ThetaRaw: []float64{-.3, -.1},
	}
	initial := []float64{.1, -.2, .05, .3}
	inputs := [][]float64{
		{.5, -.3, .2, .7}, {-.4, .6, .1, -.2}, {.3, .2, -.5, .4},
		{.8, -.1, .3, .2}, {-.2, .4, .6, -.3}, {.1, .5, -.2, .6},
	}
	up := [][]float64{
		{.2, -.3, .1, .4}, {-.1, .5, .3, -.2}, {.4, .1, -.3, .2},
		{.3, -.4, .2, .1}, {-.2, .2, .5, .3}, {.1, .3, -.1, -.4},
	}
	return c, p, initial, inputs, up
}

// TestMixedSmoothModeFiniteDifference differentiates the mixed model with its
// LIF half in the package-private smooth reference mode, which is continuous
// everywhere, using central differences. The continuous half is already smooth,
// so the whole chain, including both cross-type edges, is covered. Hard events
// are never finite differenced.
func TestMixedSmoothModeFiniteDifference(t *testing.T) {
	c, p, initial, inputs, up := mixedSmoothFixture()
	m, err := NewMixed(c)
	if err != nil {
		t.Fatal(err)
	}
	m.smooth = true
	objective := func() float64 {
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			t.Fatal(err)
		}
		var total float64
		for s, row := range tr.Outputs() {
			for i, v := range row {
				total += v * up[s][i]
			}
		}
		return total
	}
	tr, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	for s, row := range tr.Spikes() {
		for i, v := range row {
			if c.NodeRule[i] == 0 {
				if v != 0 {
					t.Fatalf("continuous node %d reported event %v", i, v)
				}
				continue
			}
			if v <= 0 || v >= 1 {
				t.Fatalf("smooth spike %v at step %d node %d is not strictly inside (0,1)", v, s, i)
			}
		}
	}
	g, err := m.Backward(context.Background(), tr, up, 0)
	if err != nil {
		t.Fatal(err)
	}
	pairs := []struct {
		name          string
		values, grads []float64
	}{
		{"weights", p.Weights, g.Weights},
		{"bias", p.Bias, g.Bias},
		{"log_tau", p.LogTau, g.LogTau},
		{"theta_raw", p.ThetaRaw, g.ThetaRaw},
		{"initial", initial, g.Initial},
	}
	for s := range inputs {
		pairs = append(pairs, struct {
			name          string
			values, grads []float64
		}{fmt.Sprintf("inputs[%d]", s), inputs[s], g.Inputs[s]})
	}
	for _, pair := range pairs {
		for i := range pair.values {
			old := pair.values[i]
			const eps = 1e-5
			pair.values[i] = old + eps
			plus := objective()
			pair.values[i] = old - eps
			minus := objective()
			pair.values[i] = old
			fd := (plus - minus) / (2 * eps)
			diff := math.Abs(pair.grads[i] - fd)
			if diff > 1e-4*math.Abs(fd) && diff > 1e-7 {
				t.Fatalf("%s[%d]: gradient %.12g finite difference %.12g", pair.name, i, pair.grads[i], fd)
			}
		}
	}
}

// TestMixedHandCalculatedTwoStepGradient checks the hard event reverse pass of
// the three node fixture against a gradient derived by hand from the declared
// rules. Two steps of the hand fixture produce, with lambda = alpha = kappa = .5,
// dlambda = lambda*dt/tau = .5*ln 2, theta slope = (2-0)*sigma(0)*(1-sigma(0)) = .5
// and psi(u) = 1/(1+|u|)^2:
//
//	dv0  = b0*(1-tanh(1)^2)                       node 0 at step 1
//	du1  = b1*psi(2*tanh 2 - 1)                   node 1 event at step 1
//	dv0b = .5*dv0 + (a0 + 2*du1)*(1-tanh(2)^2)    node 0 at step 0
//
//	weights  = [.5*du1*tanh 2, 0]
//	bias     = [.5*dv0 + .5*dv0b, .75*du1, -.5*b2 + .5*(a2 + .5*b2)]
//	log_tau  = [ln2*(dv0 - 2*dv0b), -2*ln2*du1*tanh 2, 0]
//	theta_raw= [-.5*du1]
//	inputs[0]= [.5*dv0b, .25*du1, .5*(a2 + .5*b2)]
//	inputs[1]= [.5*dv0, .5*du1, .5*b2]
//	initial  = [.5*dv0b + du1, .25*du1, .5*(a2 + .5*b2)]
//
// The second weight is exactly zero: the delay 1 edge 1 -> 2 only ever read the
// zero synaptic prehistory over two steps, which is the contract that a delayed
// LIF output reaches its target one step later and not sooner.
func TestMixedHandCalculatedTwoStepGradient(t *testing.T) {
	m, err := NewMixed(mixedHandConfig())
	if err != nil {
		t.Fatal(err)
	}
	p, initial, inputs := mixedHandInputs()
	inputs = inputs[:2]
	up := [][]float64{{.3, -.2, .5}, {.7, .4, -.6}}
	a0, a2 := up[0][0], up[0][2]
	b0, b1, b2 := up[1][0], up[1][1], up[1][2]
	tanh := math.Tanh
	psi := func(u float64) float64 { return 1 / ((1 + math.Abs(u)) * (1 + math.Abs(u))) }
	dv0 := b0 * (1 - tanh(1)*tanh(1))
	du1 := b1 * psi(2*tanh(2)-1)
	dv0b := .5*dv0 + (a0+2*du1)*(1-tanh(2)*tanh(2))
	dv2b := a2 + .5*b2

	tr, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	g, err := m.Backward(context.Background(), tr, up, 0)
	if err != nil {
		t.Fatal(err)
	}
	mixedRows(t, "gradient", [][]float64{g.Weights, g.Bias, g.LogTau, g.ThetaRaw, g.Initial, g.Inputs[0], g.Inputs[1]}, [][]float64{
		{.5 * du1 * tanh(2), 0},
		{.5*dv0 + .5*dv0b, .75 * du1, .5*b2 + .5*dv2b},
		{math.Ln2 * (dv0 - 2*dv0b), -2 * math.Ln2 * du1 * tanh(2), 0},
		{-.5 * du1},
		{.5*dv0b + du1, .25 * du1, .5 * dv2b},
		{.5 * dv0b, .25 * du1, .5 * dv2b},
		{.5 * dv0, .5 * du1, .5 * b2},
	}, mixedTol)
}

func TestMixedAdvanceModulatedNeutralMatchesAdvance(t *testing.T) {
	m, err := NewMixed(mixedHandConfig())
	if err != nil {
		t.Fatal(err)
	}
	p, initial, inputs := mixedHandInputs()
	state, err := m.NewState(initial)
	if err != nil {
		t.Fatal(err)
	}
	plain, outputs, events, err := m.Advance(context.Background(), p, state, inputs)
	if err != nil {
		t.Fatal(err)
	}
	neutral := &Modulation{Gain: make([][]float64, len(inputs)), Offset: make([][]float64, len(inputs)), Threshold: make([][]float64, len(inputs))}
	for t := range inputs {
		neutral.Gain[t] = []float64{1, 1, 1}
		neutral.Offset[t] = []float64{0, 0, 0}
		neutral.Threshold[t] = []float64{0, 0, 0}
	}
	modulated, modOutputs, modEvents, err := m.AdvanceModulated(context.Background(), p, state, inputs, neutral)
	if err != nil {
		t.Fatal(err)
	}
	assertRowsIdentical(t, "outputs", modOutputs, outputs)
	assertRowsIdentical(t, "events", modEvents, events)
	assertVectorIdentical(t, "continuous voltage", modulated.Continuous.Voltage, plain.Continuous.Voltage)
	assertVectorIdentical(t, "lif voltage", modulated.LIF.Voltage, plain.LIF.Voltage)
}

func TestMixedAdvanceModulatedThresholdOnlyOnLIFNodes(t *testing.T) {
	m, err := NewMixed(mixedHandConfig())
	if err != nil {
		t.Fatal(err)
	}
	p, initial, inputs := mixedHandInputs()
	state, err := m.NewState(initial)
	if err != nil {
		t.Fatal(err)
	}
	onContinuous := &Modulation{Threshold: [][]float64{{0, 0, 0}, {0, 0, .5}, {0, 0, 0}, {0, 0, 0}}}
	if _, _, _, err := m.AdvanceModulated(context.Background(), p, state, inputs, onContinuous); err == nil {
		t.Fatal("accepted a threshold entry on a continuous node")
	}
	// The same shift on the LIF node is accepted and suppresses the event the
	// hand calculated table shows at step 2 (index 1).
	_, _, plain, err := m.Advance(context.Background(), p, state, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if plain[1][1] != 1 {
		t.Fatalf("the unmodulated fixture does not spike at step 2: %v", plain[1])
	}
	onLIF := &Modulation{Threshold: [][]float64{{0, 0, 0}, {0, 1, 0}, {0, 0, 0}, {0, 0, 0}}}
	_, _, events, err := m.AdvanceModulated(context.Background(), p, state, inputs, onLIF)
	if err != nil {
		t.Fatal(err)
	}
	if events[1][1] != 0 {
		t.Fatalf("the event at step 2 survived a threshold shift of 1: %v", events[1])
	}
}

func TestMixedConstructionRejectsInvalidConfig(t *testing.T) {
	base := mixedHandConfig()
	for _, tc := range []struct {
		name  string
		mutup func(*MixedConfig)
	}{
		{"zero nodes", func(c *MixedConfig) { c.Nodes = 0 }},
		{"negative dt", func(c *MixedConfig) { c.DT = -1 }},
		{"short node rule", func(c *MixedConfig) { c.NodeRule = []uint8{0, 1} }},
		{"missing node rule", func(c *MixedConfig) { c.NodeRule = nil }},
		{"unknown rule value", func(c *MixedConfig) { c.NodeRule = []uint8{0, 2, 0} }},
		{"unknown activation", func(c *MixedConfig) { c.Continuous.Activation = "relu" }},
		{"missing activation", func(c *MixedConfig) { c.Continuous.Activation = "" }},
		{"activation without continuous node", func(c *MixedConfig) { c.NodeRule = []uint8{1, 1, 1} }},
		{"lif rule without lif node", func(c *MixedConfig) { c.NodeRule = []uint8{0, 0, 0} }},
		{"unsupported surrogate", func(c *MixedConfig) { c.LIF.Surrogate.Kind = "boxcar" }},
		{"thresholds out of order", func(c *MixedConfig) { c.LIF.ThetaMin = 3 }},
		{"negative refractory", func(c *MixedConfig) { c.LIF.RefractorySteps = -1 }},
		{"edge arrays differ", func(c *MixedConfig) { c.Targets = []int{1} }},
		{"endpoint out of range", func(c *MixedConfig) { c.Targets = []int{1, 3} }},
		{"negative delay", func(c *MixedConfig) { c.Delays = []int{0, -1} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			c.Sources = append([]int(nil), base.Sources...)
			c.Targets = append([]int(nil), base.Targets...)
			c.Delays = append([]int(nil), base.Delays...)
			c.NodeRule = append([]uint8(nil), base.NodeRule...)
			tc.mutup(&c)
			if _, err := NewMixed(c); err == nil {
				t.Fatalf("accepted %s", tc.name)
			}
		})
	}
}

func TestMixedForwardRejectsInvalidParameters(t *testing.T) {
	m, err := NewMixed(mixedHandConfig())
	if err != nil {
		t.Fatal(err)
	}
	p, initial, inputs := mixedHandInputs()
	for _, tc := range []struct {
		name string
		p    MixedParameters
	}{
		{"short weights", MixedParameters{Weights: []float64{1}, Bias: p.Bias, LogTau: p.LogTau, ThetaRaw: p.ThetaRaw}},
		{"short bias", MixedParameters{Weights: p.Weights, Bias: []float64{0}, LogTau: p.LogTau, ThetaRaw: p.ThetaRaw}},
		{"theta for every node", MixedParameters{Weights: p.Weights, Bias: p.Bias, LogTau: p.LogTau, ThetaRaw: []float64{0, 0, 0}}},
		{"missing theta", MixedParameters{Weights: p.Weights, Bias: p.Bias, LogTau: p.LogTau}},
		{"non-finite bias", MixedParameters{Weights: p.Weights, Bias: []float64{math.Inf(1), 0, 0}, LogTau: p.LogTau, ThetaRaw: p.ThetaRaw}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := m.Forward(context.Background(), tc.p, initial, inputs); err == nil {
				t.Fatalf("accepted %s", tc.name)
			}
		})
	}
	if _, err := m.Forward(context.Background(), p, initial, nil); err == nil {
		t.Fatal("accepted an empty sequence")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Forward(ctx, p, initial, inputs); err == nil {
		t.Fatal("accepted a canceled context")
	}
}

func TestMixedStateRoundTripAndValidation(t *testing.T) {
	m, err := NewMixed(mixedHandConfig())
	if err != nil {
		t.Fatal(err)
	}
	p, initial, inputs := mixedHandInputs()
	state, err := m.NewState(initial)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.ValidateState(state); err != nil {
		t.Fatal(err)
	}
	if len(state.Index.ContinuousNodes) != 2 || state.Index.ContinuousNodes[0] != 0 || state.Index.ContinuousNodes[1] != 2 {
		t.Fatalf("continuous index %v, want [0 2]", state.Index.ContinuousNodes)
	}
	if len(state.Index.LIFNodes) != 1 || state.Index.LIFNodes[0] != 1 {
		t.Fatalf("lif index %v, want [1]", state.Index.LIFNodes)
	}
	next, _, _, err := m.Advance(context.Background(), p, state, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.ValidateState(next); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		corrupt func(*MixedState)
	}{
		{"schema", func(s *MixedState) { s.SchemaVersion = "other" }},
		{"config hash", func(s *MixedState) { s.ConfigHash = "00" }},
		{"continuous index", func(s *MixedState) { s.Index.ContinuousNodes = []int{0, 1} }},
		{"lif index", func(s *MixedState) { s.Index.LIFNodes = []int{2} }},
		{"continuous voltage width", func(s *MixedState) { s.Continuous.Voltage = []float64{0} }},
		{"lif voltage width", func(s *MixedState) { s.LIF.Voltage = []float64{0, 0} }},
		{"negative trace", func(s *MixedState) { s.LIF.History[len(s.LIF.History)-1][0] = -1 }},
		{"refractory out of range", func(s *MixedState) { s.LIF.Refractory[0] = 9 }},
		{"adaptation while disabled", func(s *MixedState) { s.LIF.Adaptation[0] = 1 }},
		{"rate while disabled", func(s *MixedState) { s.LIF.Rate = []float64{0} }},
		{"sub step mismatch", func(s *MixedState) { s.LIF.Steps++ }},
		{"non-finite voltage", func(s *MixedState) { s.Continuous.Voltage[0] = math.NaN() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			broken := mixedCloneState(next)
			tc.corrupt(&broken)
			if err := m.ValidateState(broken); err == nil {
				t.Fatalf("accepted a state with a broken %s", tc.name)
			}
		})
	}
}

func mixedCloneState(s MixedState) MixedState {
	owned := s
	owned.Continuous.Voltage = append([]float64(nil), s.Continuous.Voltage...)
	owned.Continuous.History = cloneRows(s.Continuous.History)
	owned.LIF.Voltage = append([]float64(nil), s.LIF.Voltage...)
	owned.LIF.History = cloneRows(s.LIF.History)
	owned.LIF.Adaptation = append([]float64(nil), s.LIF.Adaptation...)
	owned.LIF.Refractory = append([]int(nil), s.LIF.Refractory...)
	owned.LIF.Rate = append([]float64(nil), s.LIF.Rate...)
	owned.LIF.Homeostasis = append([]float64(nil), s.LIF.Homeostasis...)
	owned.Index.ContinuousNodes = append([]int(nil), s.Index.ContinuousNodes...)
	owned.Index.LIFNodes = append([]int(nil), s.Index.LIFNodes...)
	return owned
}

func TestMixedConfigAndTraceAreIndependentCopies(t *testing.T) {
	c := mixedHandConfig()
	m, err := NewMixed(c)
	if err != nil {
		t.Fatal(err)
	}
	c.Sources[0] = 2
	c.NodeRule[0] = 1
	if got := m.Config(); got.Sources[0] != 0 || got.NodeRule[0] != 0 {
		t.Fatal("the model aliases the caller's configuration")
	}
	first := m.Config()
	first.Targets[0] = 0
	first.NodeRule[1] = 0
	if got := m.Config(); got.Targets[0] != 1 || got.NodeRule[1] != 1 {
		t.Fatal("Config returns an aliased copy")
	}
	p, initial, inputs := mixedHandInputs()
	tr, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	out := tr.Outputs()
	out[0][0] = 99
	if tr.Outputs()[0][0] == 99 {
		t.Fatal("Outputs returns an aliased copy")
	}
}

// withinAdjacentValues reports whether got is want or at most n adjacent
// float64 values away from it.
func withinAdjacentValues(got, want float64, n int) bool {
	if math.Float64bits(got) == math.Float64bits(want) {
		return true
	}
	lower, upper := want, want
	for range n {
		lower = math.Nextafter(lower, math.Inf(-1))
		upper = math.Nextafter(upper, math.Inf(1))
	}
	return got >= lower && got <= upper
}

// assertMatchesLIFReference compares a result of an all-LIF assignment against
// the LIF core it must reproduce.
//
// In an ordinary build the two are bit-identical and this is assertRowsIdentical.
// The race instrumented build is the one exception, and it is a property of the
// existing spiking core rather than of this one: the compiler fuses the multiply
// and the add of LIF.Forward's membrane expression
//
//	cand = lambda*v + alpha*drive
//
// in an ordinary build and does not fuse it under -race, so that core's own
// output moves by one adjacent float64 value between the two builds while this
// package's membrane helper, which is compiled once in its own function, does
// not. TestMixedMembraneContraction pins exactly that difference. Everything a
// step computes from the membrane, including the reverse pass, inherits that one
// value, so the race build compares within lifReferenceTolerance, which is below
// one adjacent float64 value at unit scale, instead of exactly. Nothing else is
// relaxed: the continuous comparisons, the events, the adaptation, the slow
// stabiliser, the refractory counters and every structural field stay exact in
// both builds.
func assertMatchesLIFReference(t *testing.T, name string, got, want [][]float64) {
	t.Helper()
	if lifReferenceContractsLikeMixed {
		assertRowsIdentical(t, name, got, want)
		return
	}
	if len(got) != len(want) {
		t.Fatalf("%s has %d rows, want %d", name, len(got), len(want))
	}
	for r := range want {
		if len(got[r]) != len(want[r]) {
			t.Fatalf("%s row %d has %d values, want %d", name, r, len(got[r]), len(want[r]))
		}
		for i := range want[r] {
			if math.Float64bits(got[r][i]) == math.Float64bits(want[r][i]) {
				continue
			}
			diff := math.Abs(got[r][i] - want[r][i])
			if diff > lifReferenceTolerance*(1+math.Abs(want[r][i])) {
				t.Fatalf("%s[%d][%d] = %.17g (bits %#x), want %.17g (bits %#x), difference %.3g exceeds %.3g",
					name, r, i, got[r][i], math.Float64bits(got[r][i]), want[r][i], math.Float64bits(want[r][i]),
					diff, lifReferenceTolerance*(1+math.Abs(want[r][i])))
			}
		}
	}
}

func assertVectorMatchesLIFReference(t *testing.T, name string, got, want []float64) {
	t.Helper()
	assertMatchesLIFReference(t, name, [][]float64{got}, [][]float64{want})
}

// TestMixedMembraneContraction states, as a test rather than as a comment, what
// the mixed core's membrane helper rounds to compared with the two reference
// cores on identical operands.
//
// Both reference cores and this package write the same expression,
// lambda*v + alpha*drive. Go is allowed to evaluate it with a fused multiply-add,
// which rounds once instead of twice, and the compiler decides that per basic
// block, so the same source line can round differently in different functions,
// architectures, and builds. The two roundings are adjacent float64 values.
//
// Measured here: Continuous.Forward rounds like the helper in every build, and
// LIF.Forward rounds like the helper in an ordinary build and like the
// explicitly twice-rounded form under -race. The helper itself can also use the
// explicitly twice-rounded form on an architecture without a fused instruction.
// math.FMA supplies a portable single-rounding witness so the fixture cannot
// become vacuous when that happens; the actual helper and LIF result are still
// checked against their two permitted compiler roundings below.
func TestMixedMembraneContraction(t *testing.T) {
	lc, _ := mixedLIFTwin(false, nil)
	cc, _ := mixedContinuousTwin("tanh")
	reference, err := NewLIF(lc)
	if err != nil {
		t.Fatal(err)
	}
	continuous, err := NewContinuous(cc)
	if err != nil {
		t.Fatal(err)
	}
	p, initial, inputs, _ := mixedTwinParameters()
	lifTrace, err := reference.Forward(context.Background(), LIFParameters{
		Weights: p.Weights, Bias: p.Bias, LogTau: p.LogTau, ThetaRaw: []float64{.2, -.3, .5},
	}, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	continuousTrace, err := continuous.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	for i := range cc.Nodes {
		// The continuous core is bit-identical to the helper in every build.
		want := membrane(continuousTrace.lambda[i], initial[i], continuousTrace.alpha[i], continuousTrace.drive[0][i])
		if math.Float64bits(continuousTrace.voltage[1][i]) != math.Float64bits(want) {
			t.Fatalf("continuous membrane[%d] = %#x, the helper gives %#x",
				i, math.Float64bits(continuousTrace.voltage[1][i]), math.Float64bits(want))
		}
	}
	discriminating := 0
	for i := range lc.Nodes {
		lambda, alpha, drive := lifTrace.lambda[i], lifTrace.alpha[i], lifTrace.drive[0][i]
		fused := membrane(lambda, initial[i], alpha, drive)
		// The explicit conversions round each product before the sum, which is
		// the value a build without the fused multiply-add produces.
		twice := float64(lambda*initial[i]) + float64(alpha*drive)
		// math.FMA explicitly computes one legal single-rounding contraction of
		// the same expression. It is the portable witness; membrane above remains
		// the reference for the compiler's actual contraction choice.
		fma := math.FMA(alpha, drive, lambda*initial[i])
		if !withinAdjacentValues(fma, twice, 1) {
			t.Fatalf("the explicit fused and twice-rounded values of node %d are not adjacent: %#x and %#x",
				i, math.Float64bits(fma), math.Float64bits(twice))
		}
		if !withinAdjacentValues(fused, twice, 1) {
			t.Fatalf("the two roundings of node %d are not adjacent: %#x and %#x",
				i, math.Float64bits(fused), math.Float64bits(twice))
		}
		got := lifTrace.cand[0][i]
		matches := math.Float64bits(got) == math.Float64bits(fused)
		if !matches && math.Float64bits(got) != math.Float64bits(twice) {
			t.Fatalf("lif membrane[%d] = %#x is neither rounding of lambda*v + alpha*drive (%#x, %#x)",
				i, math.Float64bits(got), math.Float64bits(fused), math.Float64bits(twice))
		}
		if math.Float64bits(fma) != math.Float64bits(twice) {
			discriminating++
		}
		if math.Float64bits(fused) == math.Float64bits(twice) {
			// The two roundings coincide on these operands, which says nothing
			// about which one the build chose.
			continue
		}
		if matches != lifReferenceContractsLikeMixed {
			t.Fatalf("lif membrane[%d] matches the helper = %v, but this build declares %v; update the build tagged constant",
				i, matches, lifReferenceContractsLikeMixed)
		}
	}
	if discriminating == 0 {
		t.Fatal("no node of the fixture separates the two roundings, so this test proved nothing")
	}
}
