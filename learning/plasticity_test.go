package learning_test

import (
	"context"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/plasticity"
)

// TestAdvanceWithoutPlasticityIsUnchanged is the "off restores the reference
// behaviour" clause: a never-enabled individual, an individual that had the
// mechanism enabled and switched off again, and the gated entry point with a
// closed gate all return the same bits on both cores.
func TestAdvanceWithoutPlasticityIsUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config learning.Config
		params learning.Parameters
		input  [][]float64
		edges  []int
	}{
		{"lif", lifConfig(), lifParameters(), [][]float64{{1}, {0}, {1}, {0}, {0}, {1}}, []int{0, 2}},
		{"continuous", continuousConfig(), continuousParameters(), [][]float64{{.7}, {.2}, {0}, {-.1}}, []int{0, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := advance(t, newIndividual(t, tc.config, tc.params), tc.input)
			toggled := newIndividual(t, tc.config, tc.params)
			if err := toggled.EnablePlasticity(plasticConfig(tc.edges...)); err != nil {
				t.Fatal(err)
			}
			toggled.DisablePlasticity()
			if got := advance(t, toggled, tc.input); !sameRows(got, want) {
				t.Fatalf("enable then disable changed the forward:\n got %v\nwant %v", got, want)
			}
			if toggled.Snapshot().Plastic != nil {
				t.Fatal("a disabled individual must not carry a plastic part")
			}
			gate := make([]float64, len(tc.input))
			got, report := advanceGated(t, newIndividual(t, tc.config, tc.params), tc.input, gate)
			if !sameRows(got, want) {
				t.Fatalf("AdvanceGated without plasticity changed the forward:\n got %v\nwant %v", got, want)
			}
			if (report != learning.PlasticReport{}) {
				t.Fatalf("a disabled individual reported %+v", report)
			}
		})
	}
}

// TestGatedAdvanceFollowsTheHandComputedOrder pins root decision 5 on the
// two-neuron LIF fixture. Every column is computed from the closed forms
// lambda = alpha' = kappa = exp(-1) = 0.36787944117144233,
// alpha = 1 - exp(-1) = 0.6321205588285577 and theta = 0.16324277592101166,
// with the rate rule decay_e = decay_p = 0.5 and the gate open at every step.
//
//	t w(before the row)  trace0    spike1  elig                plastic             readout = f32(trace1)
//	0 0.9                1         0       0.5*0 + 1*0   = 0   0.5*0 + 0      = 0   0
//	1 0.9                0.3678794 1       0   + 0.367879 ...  0.367879441...      1
//	2 1.2678794411714422 1.1353352 0       0.5*0.3678794 ...   0.5*0.3678794 ...   0.3678794503211975
//	3 1.2678794411714422 0.4176665 1       0.509636369...      0.693576090...      1.1353353261947632
//	4 1.593576090417888  0.1536509 1       0.408469107...      0.755257152...      1.4176665544509888
//
// The spike at step 4 exists only because the fast change had already raised
// the weight: at the base weight the same step stays below threshold, which is
// what the closed-gate table below shows.
func TestGatedAdvanceFollowsTheHandComputedOrder(t *testing.T) {
	a := newIndividual(t, plasticLIFConfig(), plasticLIFParameters())
	if err := a.EnablePlasticity(plasticConfig(0)); err != nil {
		t.Fatal(err)
	}
	input := plasticLIFInput()
	got, report := advanceGated(t, a, input, []float64{1, 1, 1, 1, 1})
	want := []float64{0, 1, 0.3678794503211975, 1.1353353261947632, 1.4176665544509888}
	for step, row := range got {
		if row[0] != want[step] {
			t.Fatalf("readout[%d] = %.17g, want %.17g", step, row[0], want[step])
		}
	}
	if report.Steps != len(input) || report.Clamped != 0 || report.HeldAtWMin != 0 {
		t.Fatalf("report = %+v", report)
	}
	s := a.Snapshot()
	if s.Plastic == nil {
		t.Fatal("an enabled individual must carry a plastic part")
	}
	if !closeEnough(s.Plastic.State.Plastic[0], 0.7552571522503744) {
		t.Fatalf("plastic = %.17g, want 0.7552571522503744", s.Plastic.State.Plastic[0])
	}
	if !closeEnough(s.Plastic.State.Eligibility[0], 0.40846910704143036) {
		t.Fatalf("eligibility = %.17g, want 0.40846910704143036", s.Plastic.State.Eligibility[0])
	}
}

// TestPlainAdvanceKeepsTheGateClosed is the gate-0 semantics of the unchanged
// entry point: eligibility still tracks the same activity, the fast change only
// decays, and the readout is exactly the one the model produces with no
// plasticity at all.
//
//	t elig                       plastic  readout
//	0 0                          0        0
//	1 0.36787944117144233        0        1
//	2 0.18393972058572117        0        0.3678794503211975
//	3 0.5096363698321669         0        1.1353353261947632
//	4 0.25481818491608343        0        0.417666494846344
func TestPlainAdvanceKeepsTheGateClosed(t *testing.T) {
	config, params, input := plasticLIFConfig(), plasticLIFParameters(), plasticLIFInput()
	reference := advance(t, newIndividual(t, config, params), input)
	a := newIndividual(t, config, params)
	if err := a.EnablePlasticity(plasticConfig(0)); err != nil {
		t.Fatal(err)
	}
	got := advance(t, a, input)
	if !sameRows(got, reference) {
		t.Fatalf("a closed gate changed the forward:\n got %v\nwant %v", got, reference)
	}
	want := []float64{0, 1, 0.3678794503211975, 1.1353353261947632, 0.417666494846344}
	for step, row := range got {
		if row[0] != want[step] {
			t.Fatalf("readout[%d] = %.17g, want %.17g", step, row[0], want[step])
		}
	}
	s := a.Snapshot()
	if s.Plastic.State.Plastic[0] != 0 {
		t.Fatalf("a closed gate produced plastic %.17g", s.Plastic.State.Plastic[0])
	}
	if !closeEnough(s.Plastic.State.Eligibility[0], 0.25481818491608343) {
		t.Fatalf("eligibility = %.17g, want 0.25481818491608343", s.Plastic.State.Eligibility[0])
	}
}

// TestDelayedGatePulseChangesLaterRows is the delayed-feedback clause at the
// individual level: the gate is closed while the activity happens and opens
// two steps after the pairing of step 1. The fast change then equals the
// residual eligibility 0.5096363698321669 exactly, and the weight it produces
// flips the step-4 event, so the readout of the last row differs from the
// closed-gate run.
func TestDelayedGatePulseChangesLaterRows(t *testing.T) {
	config, params, input := plasticLIFConfig(), plasticLIFParameters(), plasticLIFInput()
	closed := advance(t, newIndividual(t, config, params), input)
	a := newIndividual(t, config, params)
	if err := a.EnablePlasticity(plasticConfig(0)); err != nil {
		t.Fatal(err)
	}
	got, report := advanceGated(t, a, input, []float64{0, 0, 0, 1, 0})
	for step := range 4 {
		if got[step][0] != closed[step][0] {
			t.Fatalf("row %d changed before the gate arrived: %.17g != %.17g", step, got[step][0], closed[step][0])
		}
	}
	if got[4][0] == closed[4][0] {
		t.Fatalf("the delayed gate left the last row unchanged at %.17g", got[4][0])
	}
	if got[4][0] != 1.4176665544509888 {
		t.Fatalf("readout[4] = %.17g, want 1.4176665544509888", got[4][0])
	}
	if report.Steps != 5 {
		t.Fatalf("report = %+v", report)
	}
	// The plastic entry at the moment the gate fired is the residual
	// eligibility; the last row then halves it because the gate closed again.
	if !closeEnough(a.Snapshot().Plastic.State.Plastic[0], 0.25481818491608343) {
		t.Fatalf("plastic = %.17g, want 0.25481818491608343", a.Snapshot().Plastic.State.Plastic[0])
	}
}

// TestFixedSignEdgeStaysOnItsSignAndCountsTheHold is root decision 3 on a
// running individual. The single edge is declared excitatory and stores
// rho = log(0.9), and the fast state starts at -8. Without the floor the edge
// would integrate 0.9 - 8 = -7.1 and become inhibitory; the floor holds its
// magnitude at w_min = 0.9 instead, so the readout is exactly the closed-gate
// table of the unmodified fixture. The fast change halves every row while the
// gate is closed (-8, -4, -2, -1, -0.5) and stays below the floor throughout,
// so all five rows report a hold.
func TestFixedSignEdgeStaysOnItsSignAndCountsTheHold(t *testing.T) {
	rule := plasticRule()
	rule.WMin = .9
	config := plasticLIFConfig()
	config.EdgeSigns = []int8{1}
	params := plasticLIFParameters()
	params.Core.Weights = []float64{math.Log(.9)}

	seed := newIndividual(t, config, params)
	if err := seed.EnablePlasticity(plasticity.Config{Rule: rule, Edges: []int{0}}); err != nil {
		t.Fatal(err)
	}
	snapshot := seed.Snapshot()
	snapshot.Plastic.State.Plastic[0] = -8
	held, err := learning.RestoreIndividual(snapshot)
	if err != nil {
		t.Fatal(err)
	}

	input := plasticLIFInput()
	got, report := advanceGated(t, held, input, make([]float64, len(input)))
	want := []float64{0, 1, 0.3678794503211975, 1.1353353261947632, 0.417666494846344}
	for step, row := range got {
		if row[0] != want[step] {
			t.Fatalf("held readout[%d] = %.17g, want %.17g", step, row[0], want[step])
		}
	}
	if report.HeldAtWMin != len(input) {
		t.Fatalf("held at w_min %d times over %d rows", report.HeldAtWMin, len(input))
	}
	if p := held.Snapshot().Plastic.State.Plastic[0]; p != -.25 {
		t.Fatalf("plastic = %.17g, want -0.25 after five halvings of -8", p)
	}

	// The same fast change on a free edge is allowed to cross zero, which is
	// the difference the fixed-sign parametrization exists to prevent.
	free := plasticLIFConfig()
	freeParams := plasticLIFParameters()
	freeSeed := newIndividual(t, free, freeParams)
	if err := freeSeed.EnablePlasticity(plasticity.Config{Rule: rule, Edges: []int{0}}); err != nil {
		t.Fatal(err)
	}
	crossing := freeSeed.Snapshot()
	crossing.Plastic.State.Plastic[0] = -8
	flipped, err := learning.RestoreIndividual(crossing)
	if err != nil {
		t.Fatal(err)
	}
	other, freeReport := advanceGated(t, flipped, input, make([]float64, len(input)))
	if freeReport.HeldAtWMin != 0 {
		t.Fatalf("a free edge was held at w_min %d times", freeReport.HeldAtWMin)
	}
	if sameRows(other, got) {
		t.Fatal("a free edge under -8 produced the held trajectory")
	}
	for step, row := range other {
		if row[0] != 0 {
			t.Fatalf("row %d = %.17g; an edge driven to -7.1 cannot excite the readout", step, row[0])
		}
	}
}

// TestEnablePlasticityValidatesAndReplaces covers the lifecycle: the edge
// selection is checked against this individual's topology, a second call
// replaces the declaration and restarts the fast state, and the pair rule is
// refused on a core that produces no events.
func TestEnablePlasticityValidatesAndReplaces(t *testing.T) {
	a := newIndividual(t, lifConfig(), lifParameters())
	if err := a.EnablePlasticity(plasticConfig(3)); err == nil {
		t.Fatal("accepted an edge index outside the core")
	}
	if err := a.EnablePlasticity(plasticConfig(0, 1, 2)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Advance(context.Background(), [][]float64{{1}, {0}}); err != nil {
		t.Fatal(err)
	}
	first := a.Snapshot()
	if len(first.Plastic.State.Eligibility) != 3 {
		t.Fatalf("plastic state has %d entries", len(first.Plastic.State.Eligibility))
	}
	if err := a.EnablePlasticity(plasticConfig(1)); err != nil {
		t.Fatal(err)
	}
	second := a.Snapshot()
	if !reflect.DeepEqual(second.Plastic.Config.Edges, []int{1}) {
		t.Fatalf("second call kept %v", second.Plastic.Config.Edges)
	}
	if len(second.Plastic.State.Eligibility) != 1 || second.Plastic.State.Eligibility[0] != 0 || second.Plastic.State.Plastic[0] != 0 {
		t.Fatalf("second call did not reset the fast state: %+v", second.Plastic.State)
	}

	pair := plasticity.Config{Rule: plasticity.Rule{
		Kind: plasticity.RuleSTDPPair, DecayE: .5, DecayP: .5, PlasticMax: 8, WMin: .0625,
		DecayPre: .5, DecayPost: .5, APlus: 1, AMinus: 1,
	}, Edges: []int{0}}
	if err := a.EnablePlasticity(pair); err != nil {
		t.Fatalf("the spiking core must accept the pair rule: %v", err)
	}
	c := newIndividual(t, continuousConfig(), continuousParameters())
	err := c.EnablePlasticity(pair)
	if err == nil || !strings.Contains(err.Error(), "stdp_pair") {
		t.Fatalf("the continuous core must refuse the pair rule: %v", err)
	}
}

// TestSTDPPairRunsOnTheSpikingIndividual shows the pair rule reaching the
// events of a real core: the fixture's neuron 1 fires after neuron 0, so the
// enabled 0->1 edge accumulates a positive fast change under an open gate.
func TestSTDPPairRunsOnTheSpikingIndividual(t *testing.T) {
	a := newIndividual(t, plasticLIFConfig(), plasticLIFParameters())
	rule := plasticity.Rule{
		Kind: plasticity.RuleSTDPPair, DecayE: .5, DecayP: .5, PlasticMax: 8, WMin: .0625,
		DecayPre: .5, DecayPost: .5, APlus: 1, AMinus: 1,
	}
	if err := a.EnablePlasticity(plasticity.Config{Rule: rule, Edges: []int{0}}); err != nil {
		t.Fatal(err)
	}
	input := plasticLIFInput()
	if _, _, err := a.AdvanceGated(context.Background(), input, []float64{1, 1, 1, 1, 1}); err != nil {
		t.Fatal(err)
	}
	s := a.Snapshot().Plastic.State
	if len(s.PreTrace) != 1 || len(s.PostTrace) != 1 {
		t.Fatalf("the pair rule must persist both traces: %+v", s)
	}
	if s.Plastic[0] <= 0 {
		t.Fatalf("pre-before-post on the fixture must strengthen: %.17g", s.Plastic[0])
	}
}

// TestPlasticSnapshotRoundTrip is the persistence clause of root decision 6.
func TestPlasticSnapshotRoundTrip(t *testing.T) {
	a := newIndividual(t, plasticLIFConfig(), plasticLIFParameters())
	if err := a.EnablePlasticity(plasticConfig(0)); err != nil {
		t.Fatal(err)
	}
	advanceGated(t, a, plasticLIFInput(), []float64{1, 1, 1, 1, 1})
	want := a.Snapshot()
	b, err := learning.RestoreIndividual(want)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.Snapshot(), want) {
		t.Fatal("restoring an individual changed its plastic part")
	}
	want.Plastic.State.Plastic[0] = 99
	if again := b.Snapshot(); again.Plastic.State.Plastic[0] == 99 {
		t.Fatal("the restored individual aliases the caller's snapshot")
	}
	// Continuing from the restored copy equals continuing from the original.
	more := [][]float64{{1.5}, {0}}
	gate := []float64{1, 1}
	left, _ := advanceGated(t, a, more, gate)
	right, _ := advanceGated(t, b, more, gate)
	if !sameRows(left, right) {
		t.Fatalf("restored continuation diverged:\n%v\n%v", left, right)
	}
}

// TestRestoreIndividualRejectsMalformedPlasticParts keeps a hand-edited
// document from becoming a silently different mechanism.
func TestRestoreIndividualRejectsMalformedPlasticParts(t *testing.T) {
	a := newIndividual(t, plasticLIFConfig(), plasticLIFParameters())
	if err := a.EnablePlasticity(plasticConfig(0)); err != nil {
		t.Fatal(err)
	}
	valid := a.Snapshot()
	for name, damage := range map[string]func(*learning.IndividualSnapshot){
		"unknown rule":      func(s *learning.IndividualSnapshot) { s.Plastic.Config.Rule.Kind = "oja" },
		"edge out of range": func(s *learning.IndividualSnapshot) { s.Plastic.Config.Edges = []int{7} },
		"no edges":          func(s *learning.IndividualSnapshot) { s.Plastic.Config.Edges = nil },
		"short state":       func(s *learning.IndividualSnapshot) { s.Plastic.State.Plastic = nil },
		"extra state":       func(s *learning.IndividualSnapshot) { s.Plastic.State.Plastic = []float64{0, 0} },
		"pair traces on a rate rule": func(s *learning.IndividualSnapshot) {
			s.Plastic.State.PreTrace = []float64{0}
		},
		"plastic past the bound": func(s *learning.IndividualSnapshot) { s.Plastic.State.Plastic = []float64{99} },
		"non-finite eligibility": func(s *learning.IndividualSnapshot) {
			s.Plastic.State.Eligibility = []float64{math.NaN()}
		},
	} {
		t.Run(name, func(t *testing.T) {
			broken := valid
			part := *valid.Plastic
			part.Config.Edges = append([]int(nil), valid.Plastic.Config.Edges...)
			part.State = plasticity.State{
				Eligibility: append([]float64(nil), valid.Plastic.State.Eligibility...),
				Plastic:     append([]float64(nil), valid.Plastic.State.Plastic...),
			}
			broken.Plastic = &part
			damage(&broken)
			if _, err := learning.RestoreIndividual(broken); err == nil {
				t.Fatal("accepted a malformed plastic part")
			}
		})
	}
}

// TestAdvanceGatedRejectsMismatchedGates keeps a gate from being silently
// dropped: it must cover every row, and a gate that would change nothing
// because plasticity is off is an error rather than a no-op.
func TestAdvanceGatedRejectsMismatchedGates(t *testing.T) {
	input := plasticLIFInput()
	a := newIndividual(t, plasticLIFConfig(), plasticLIFParameters())
	if err := a.EnablePlasticity(plasticConfig(0)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.AdvanceGated(context.Background(), input, []float64{1, 1}); err == nil {
		t.Fatal("accepted a gate shorter than the input")
	}
	if _, _, err := a.AdvanceGated(context.Background(), input, []float64{1, 1, 1, 1, math.NaN()}); err == nil {
		t.Fatal("accepted a non-finite gate")
	}
	off := newIndividual(t, plasticLIFConfig(), plasticLIFParameters())
	if _, _, err := off.AdvanceGated(context.Background(), input, []float64{0, 0, 0, 1, 0}); err == nil {
		t.Fatal("accepted an open gate while plasticity is disabled")
	}
}
