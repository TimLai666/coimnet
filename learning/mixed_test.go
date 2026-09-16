// This file is an internal package test because the flat AdamW mask of a mixed
// core has to be checked directly: theta_raw has one entry per LIF node, not
// one per node, so the node half of an update mask has to be routed through the
// core's own index and that routing is not visible from outside.
package learning

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/plasticity"
)

// mixedIndividualCore is the three node fixture of ticket 25: node 0 is
// continuous, node 1 is LIF and node 2 is continuous, with one cross-type edge
// in each direction. dt = ln 2 with tau = tau_syn = 1 makes every coefficient
// exactly one half, and the encoder is an exact float32 scale, so the core
// integrates exactly the hand computed table of dynamics/mixed_test.go.
func mixedIndividualCore() dynamics.MixedConfig {
	return dynamics.MixedConfig{
		Nodes:      3,
		Sources:    []int{0, 1},
		Targets:    []int{1, 2},
		Delays:     []int{0, 1},
		DT:         math.Ln2,
		NodeRule:   []uint8{0, 1, 0},
		Continuous: dynamics.ContinuousRule{Activation: "tanh"},
		LIF: dynamics.LIFRule{
			TauSyn: 1, ThetaMin: 0, ThetaMax: 2, VReset: -1, RefractorySteps: 1,
			Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 1},
		},
	}
}

func mixedIndividualConfig() Config {
	core := mixedIndividualCore()
	return Config{Mixed: &core, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{2}}
}

func mixedIndividualParameters() Parameters {
	return Parameters{
		Core:     dynamics.Parameters{Weights: []float64{4, 2}, Bias: []float64{0, 0, 0}, LogTau: []float64{0, 0, 0}},
		ThetaRaw: []float64{0},
		Encoder:  []float64{4, 0, 0},
		Readout:  []float64{1},
	}
}

func newMixedIndividual(t *testing.T) *Individual {
	t.Helper()
	individual, err := NewIndividual(mixedIndividualConfig(), mixedIndividualParameters(), DefaultOptions(), []float64{0, 0, 0})
	if err != nil {
		t.Fatal(err)
	}
	return individual
}

func TestMixedCoreIsExclusiveWithTheOtherTwo(t *testing.T) {
	core := mixedIndividualCore()
	lif := dynamics.LIFConfig{
		Nodes: 3, Sources: []int{0}, Targets: []int{1}, DT: 1, TauSyn: 1,
		ThetaMin: .05, ThetaMax: 1, VReset: -.5,
		Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
	}
	for _, tc := range []struct {
		name string
		c    Config
	}{
		{"mixed and dynamics", Config{
			Mixed:     &core,
			Dynamics:  dynamics.Config{Nodes: 3, Sources: []int{0}, Targets: []int{1}, DT: .5, Activation: "tanh"},
			InputSize: 1, OutputSize: 1, ReadoutNodes: []int{2},
		}},
		{"mixed and lif", Config{
			Mixed: &core, LIF: &lif,
			InputSize: 1, OutputSize: 1, ReadoutNodes: []int{2},
		}},
		{"no core at all", Config{InputSize: 1, OutputSize: 1, ReadoutNodes: []int{0}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewNetwork(tc.c); err == nil {
				t.Fatalf("accepted %s", tc.name)
			}
		})
	}
}

func TestMixedNetworkConfigCarriesTheLIFIndex(t *testing.T) {
	n, err := NewNetwork(mixedIndividualConfig())
	if err != nil {
		t.Fatal(err)
	}
	c := n.Config()
	if c.Mixed == nil {
		t.Fatal("Config lost the mixed core")
	}
	if !reflect.DeepEqual(c.LIFIndex, []int{1}) {
		t.Fatalf("lif_index = %v, want [1]", c.LIFIndex)
	}
	if c.LIF != nil || c.Dynamics.Nodes != 0 {
		t.Fatalf("Config carries a second core: %+v", c)
	}
	// The returned configuration is an independent copy.
	c.Mixed.Sources[0] = 2
	c.LIFIndex[0] = 0
	again := n.Config()
	if again.Mixed.Sources[0] != 0 || again.LIFIndex[0] != 1 {
		t.Fatal("Config returns aliased buffers")
	}
	// A declared index that does not match the assignment is refused rather
	// than silently replaced.
	wrong := mixedIndividualConfig()
	wrong.LIFIndex = []int{0}
	if _, err := NewNetwork(wrong); err == nil {
		t.Fatal("accepted a lif_index that contradicts the rule assignment")
	}
	// A continuous core owns no theta entries, so it may not carry an index.
	stray := Config{
		Dynamics:  dynamics.Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, DT: .5, Activation: "tanh"},
		LIFIndex:  []int{0},
		InputSize: 1, OutputSize: 1, ReadoutNodes: []int{1},
	}
	if _, err := NewNetwork(stray); err == nil {
		t.Fatal("accepted a lif_index on a continuous core")
	}
}

// TestMixedIndividualAdvanceReproducesTheCoreHandTable drives the persistent
// individual with the encoder scale that turns input 1 into the core drive 4,
// so the readout of node 2 is exactly the hand computed table: zero for three
// steps and tanh 1 at the fourth, when the delay 1 edge finally carries the
// event's synaptic trace.
func TestMixedIndividualAdvanceReproducesTheCoreHandTable(t *testing.T) {
	individual := newMixedIndividual(t)
	out, err := individual.Advance(context.Background(), [][]float64{{1}, {0}, {0}, {0}})
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{0, 0, 0, math.Tanh(1)}
	if len(out) != 4 {
		t.Fatalf("got %d readout rows", len(out))
	}
	for i, row := range out {
		if len(row) != 1 {
			t.Fatalf("row %d has %d values", i, len(row))
		}
		if math.Abs(row[0]-want[i]) > 1e-6 {
			t.Fatalf("readout[%d] = %v, want %v", i, row[0], want[i])
		}
	}
	s := individual.Snapshot()
	if s.Profile != IndividualProfileMixed {
		t.Fatalf("profile = %q, want %q", s.Profile, IndividualProfileMixed)
	}
	if s.Neural.Core != NeuralCoreMixed || s.Neural.Mixed == nil {
		t.Fatalf("neural union = %+v", s.Neural)
	}
	if s.Neural.Continuous != nil || s.Neural.LIF != nil {
		t.Fatal("a mixed individual filled a single-rule half of the union")
	}
	if s.Neural.Mixed.Steps != 4 {
		t.Fatalf("steps = %d", s.Neural.Mixed.Steps)
	}
	if !reflect.DeepEqual(s.Neural.Mixed.Index.ContinuousNodes, []int{0, 2}) ||
		!reflect.DeepEqual(s.Neural.Mixed.Index.LIFNodes, []int{1}) {
		t.Fatalf("index = %+v", s.Neural.Mixed.Index)
	}
	if len(s.Parameters.ThetaRaw) != 1 {
		t.Fatalf("theta_raw has %d entries, want one per LIF node", len(s.Parameters.ThetaRaw))
	}
}

func TestMixedIndividualSnapshotRoundTrip(t *testing.T) {
	individual := newMixedIndividual(t)
	if _, err := individual.Advance(context.Background(), [][]float64{{1}, {0}}); err != nil {
		t.Fatal(err)
	}
	snapshot := individual.Snapshot()
	restored, err := RestoreIndividual(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Snapshot(), snapshot) {
		t.Fatal("restore changed the mixed individual snapshot")
	}
	// Continuing the restored individual matches continuing the original one.
	wantOut, err := individual.Advance(context.Background(), [][]float64{{0}, {0}})
	if err != nil {
		t.Fatal(err)
	}
	gotOut, err := restored.Advance(context.Background(), [][]float64{{0}, {0}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotOut, wantOut) {
		t.Fatalf("resumed readout %v, want %v", gotOut, wantOut)
	}
}

func TestMixedSnapshotWithTwoHalvesIsRefused(t *testing.T) {
	individual := newMixedIndividual(t)
	if _, err := individual.Advance(context.Background(), [][]float64{{1}}); err != nil {
		t.Fatal(err)
	}
	base := individual.Snapshot()
	for _, tc := range []struct {
		name    string
		corrupt func(*IndividualSnapshot)
	}{
		{"continuous half alongside the mixed one", func(s *IndividualSnapshot) {
			s.Neural.Continuous = &dynamics.State{SchemaVersion: dynamics.ContinuousStateVersion}
		}},
		{"lif half alongside the mixed one", func(s *IndividualSnapshot) {
			s.Neural.LIF = &dynamics.LIFState{SchemaVersion: dynamics.LIFStateVersion}
		}},
		{"declared core does not match", func(s *IndividualSnapshot) { s.Neural.Core = NeuralCoreLIF }},
		{"missing mixed half", func(s *IndividualSnapshot) { s.Neural.Mixed = nil }},
		{"wrong profile", func(s *IndividualSnapshot) { s.Profile = IndividualProfileLIF }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			broken := base
			broken.Neural = copyNeural(base.Neural)
			tc.corrupt(&broken)
			if _, err := RestoreIndividual(broken); err == nil {
				t.Fatalf("accepted %s", tc.name)
			}
		})
	}
}

// TestMixedTrainableThetaCoversOnlyTheLIFNodes checks both halves of the rule:
// the theta group has one entry per LIF node, and the node half of an update
// mask reaches that entry through the core's own LIF index rather than by
// position.
func TestMixedTrainableThetaCoversOnlyTheLIFNodes(t *testing.T) {
	p := mixedIndividualParameters()
	flat := flatParameters(p)
	// weights 2, bias 3, log_tau 3, theta_raw 1, encoder 3, readout 1.
	if len(flat) != 13 {
		t.Fatalf("flat layout has %d values, want 13", len(flat))
	}
	mask := parameterMask(p, Options{Trainable: Trainable{Theta: true}}, []int{1})
	for i, enabled := range mask {
		if enabled != (i == 8) {
			t.Fatalf("theta-only mask at %d = %v", i, enabled)
		}
	}
	// Masking the LIF node closes its theta entry; masking a continuous node
	// leaves it open, which position-indexed masking would get wrong.
	closed := parameterMask(p, Options{
		Trainable: Trainable{Theta: true},
		Masks:     &UpdateMasks{Nodes: []bool{true, false, true}},
	}, []int{1})
	if closed[8] {
		t.Fatal("a node mask on the LIF node left its theta entry updatable")
	}
	open := parameterMask(p, Options{
		Trainable: Trainable{Theta: true},
		Masks:     &UpdateMasks{Nodes: []bool{false, true, false}},
	}, []int{1})
	if !open[8] {
		t.Fatal("a node mask that only opens the LIF node closed its theta entry")
	}
}

func TestMixedThetaMaskIsHonouredByTheTrainer(t *testing.T) {
	run := func(nodes []bool) []float64 {
		t.Helper()
		options := DefaultOptions()
		options.LearningRate = .1
		options.Trainable = Trainable{Theta: true}
		options.Masks = &UpdateMasks{Nodes: nodes}
		individual, err := NewIndividual(mixedIndividualConfig(), mixedIndividualParameters(), options, []float64{0, 0, 0})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := individual.TrainEpisode(context.Background(), [][]float64{{1}, {0}, {0}, {0}}, []float64{1}); err != nil {
			t.Fatal(err)
		}
		return individual.Snapshot().Parameters.ThetaRaw
	}
	if got := run([]bool{false, true, false}); got[0] == 0 {
		t.Fatal("an open LIF node did not update theta_raw")
	}
	if got := run([]bool{true, false, true}); got[0] != 0 {
		t.Fatalf("a masked LIF node updated theta_raw to %v", got[0])
	}
}

// TestMixedPlasticitySignals pins the pre and post signals of ticket 25:
// pre is always the node's own output series, post is the 0/1 event on a LIF
// target and the output on a continuous target. Running with a closed gate
// leaves the effective weights at their base values, so the forward pass stays
// exactly the hand computed table and the eligibility can be checked by hand.
//
// Edge 0 is 0 -> 1 (continuous source, LIF target) and edge 1 is 1 -> 2 (LIF
// source, continuous target), with decay_e = .5:
//
//	step 1: e0 = .5*0 + tanh2*event_1(1)=0   -> 0        e1 = x_1(1)*y_2(1) = 0
//	step 2: e0 = .5*0 + tanh1*event_1(2)=1   -> tanh 1   e1 = 1*0 = 0
//	step 3: e0 = .5*tanh1 + tanh.5*0         -> .5*tanh1 e1 = .5*0 = 0
//	step 4: e0 = .25*tanh1 + tanh.25*0       -> .25*tanh1 e1 = .25*tanh 1
//
// Step 3 is the discriminating one for the LIF target: its synaptic trace is .5
// there while its event is 0, so an implementation that used the trace as the
// post signal would add tanh(.5)*.5 instead of nothing. Step 4 is the
// discriminating one for the continuous target: node 2 has no event at all, so
// an implementation that used the event row would leave e1 at zero.
func TestMixedPlasticitySignals(t *testing.T) {
	individual := newMixedIndividual(t)
	if err := individual.EnablePlasticity(plasticity.Config{
		Rule:  plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 8, WMin: .0625},
		Edges: []int{0, 1},
	}); err != nil {
		t.Fatal(err)
	}
	tanh := math.Tanh
	steps := [][]float64{{1}, {0}, {0}, {0}}
	want := [][]float64{
		{0, 0},
		{tanh(1), 0},
		{.5 * tanh(1), 0},
		{.25 * tanh(1), .25 * tanh(1)},
	}
	for s, row := range steps {
		if _, _, err := individual.AdvanceGated(context.Background(), [][]float64{row}, []float64{0}); err != nil {
			t.Fatal(err)
		}
		state := individual.Snapshot().Plastic.State
		for e := range want[s] {
			if math.Abs(state.Eligibility[e]-want[s][e]) > 1e-12 {
				t.Fatalf("step %d eligibility[%d] = %.17g, want %.17g", s+1, e, state.Eligibility[e], want[s][e])
			}
			if state.Plastic[e] != 0 {
				t.Fatalf("step %d plastic[%d] = %v with a closed gate", s+1, e, state.Plastic[e])
			}
		}
	}
}

func TestMixedRefusesPairRuleAndWrongThetaLength(t *testing.T) {
	individual := newMixedIndividual(t)
	err := individual.EnablePlasticity(plasticity.Config{
		Rule: plasticity.Rule{
			Kind: plasticity.RuleSTDPPair, DecayE: .5, DecayP: .5, PlasticMax: 8, WMin: .0625,
			DecayPre: .5, DecayPost: .5, APlus: 1, AMinus: 1,
		},
		Edges: []int{0},
	})
	if err == nil {
		t.Fatal("accepted the pair rule on a core whose continuous nodes have no events")
	}
	for _, theta := range [][]float64{nil, {0, 0, 0}, {0, 0}} {
		p := mixedIndividualParameters()
		p.ThetaRaw = theta
		if _, err := NewIndividual(mixedIndividualConfig(), p, DefaultOptions(), []float64{0, 0, 0}); err == nil {
			t.Fatalf("accepted theta_raw of length %d", len(theta))
		}
	}
}

func TestMixedTrainEpisodeProducesFiniteGradientsForEveryGroup(t *testing.T) {
	n, err := NewNetwork(mixedIndividualConfig())
	if err != nil {
		t.Fatal(err)
	}
	p := mixedIndividualParameters()
	loss, g, err := n.LossGradient(context.Background(), p, [][]float64{{1}, {0}, {0}, {0}}, []float64{1}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if loss <= 0 {
		t.Fatalf("loss = %v", loss)
	}
	if len(g.ThetaRaw) != 1 {
		t.Fatalf("theta gradient has %d entries, want one per LIF node", len(g.ThetaRaw))
	}
	for name, values := range map[string][]float64{
		"weights": g.Core.Weights, "bias": g.Core.Bias, "log_tau": g.Core.LogTau,
		"theta_raw": g.ThetaRaw, "encoder": g.Encoder, "readout": g.Readout,
	} {
		for i, v := range values {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("%s[%d] is %v", name, i, v)
			}
		}
	}
	if g.ThetaRaw[0] == 0 || g.Core.Weights[1] == 0 {
		t.Fatalf("the cross-type path carried no gradient: theta %v weights %v", g.ThetaRaw, g.Core.Weights)
	}
}

// TestMixedOpenGateMovesOnlyTheFastState runs the mixed core with a gate that is
// actually open, so the plastic change reaches the effective weights of the next
// step, and checks that it never reaches the base parameters.
func TestMixedOpenGateMovesOnlyTheFastState(t *testing.T) {
	individual := newMixedIndividual(t)
	if err := individual.EnablePlasticity(plasticity.Config{
		Rule:  plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 8, WMin: .0625},
		Edges: []int{0, 1},
	}); err != nil {
		t.Fatal(err)
	}
	before := individual.Snapshot().Parameters
	out, report, err := individual.AdvanceGated(context.Background(), [][]float64{{1}, {0}, {0}, {0}}, []float64{0, 1, 1, 1})
	if err != nil {
		t.Fatal(err)
	}
	if report.Steps != 4 {
		t.Fatalf("report covered %d steps, want 4", report.Steps)
	}
	if len(out) != 4 {
		t.Fatalf("got %d readout rows", len(out))
	}
	after := individual.Snapshot()
	moved := false
	for _, v := range after.Plastic.State.Plastic {
		if v != 0 {
			moved = true
		}
	}
	if !moved {
		t.Fatal("an open gate left every fast change at zero")
	}
	if !reflect.DeepEqual(after.Parameters, before) {
		t.Fatal("a fast change reached the base parameters")
	}
}
