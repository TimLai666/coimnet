package learning_test

import (
	"context"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/plasticity"
)

// The receptor fixture reuses the chemical declaration and its hand occupancy
// rows (0, 0.611481100422978, 0.48838764641507565 on the hypothesized
// receptor) and adds one edge 0 -> 1 with weight 0.9 to the otherwise isolated
// pair, so the plasticity on that edge is observable while the chemistry keeps
// the forward dynamics unchanged. dt = tau = 1 on the continuous core, so the
// membrane update is v' = exp(-1)*v + (1-exp(-1))*drive and the delivered
// rows are node 0 driven, node 1 carried only by the synapse.
//
// The rate rule halves every trace, so the closed-form fast state after the
// steps with pre*post at rows 0, 1, 2 = 0, 0.2151700772189281,
// 0.3503314102179744 is the table below. The gate rule opens each row with the
// receptor occupancy; row 2 then integrates the weight 0.9 + plastic(1), so
// its pre*post row is 0.3827559807209244.
const (
	gateEligClosed   = 0.4579164488274385
	gateEligMid      = 0.2151700772189281
	gatePlasticMid   = 0.1315724355959273
	gateEligFinal    = 0.4903410193303885
	gatePlasticFinal = 0.3052627141695012

	windowDecayE1   = 0.2554075598308088
	windowEligFinal = 0.4158818857906775
)

// receptorContinuousConfig is the chemistry fixture's isolated tanh pair plus
// the edge that makes the plasticity observable.
func receptorContinuousConfig() learning.Config {
	return learning.Config{
		Dynamics:     dynamics.Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{0}, DT: 1, Activation: "tanh"},
		InputSize:    2,
		OutputSize:   1,
		ReadoutNodes: []int{0},
		InputNodes:   []int{0, 1},
	}
}

func receptorContinuousParameters() learning.Parameters {
	return learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{.9}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}},
		Encoder: []float64{1, 0, 0, 1},
		Readout: []float64{1},
	}
}

func receptorInput() [][]float64 { return [][]float64{{1, 0}, {1, 0}, {1, 0}} }

// silentChemistryDeclaration never releases, so the hypothesized receptor
// occupancy is 0 on every row.
func silentChemistryDeclaration() modulation.ChemistryConfig {
	c := chemistryDeclaration()
	c.Sources[0].Timeline.Entries = nil
	return c
}

// receptorGateRule lets receptor 0, gate_scale 1, open the rate update.
func receptorGateRule() plasticity.Rule {
	r := plasticRule()
	idx := 0
	r.GateReceptor = &idx
	r.GateScale = 1
	return r
}

// receptorWindowRule shortens the eligibility trace as occupancy rises:
// decay_e = clamp(0.5 - 0.4*occ, 0.1, 0.9).
func receptorWindowRule() plasticity.Rule {
	r := plasticRule()
	idx := 0
	r.DecayEReceptor = &idx
	r.DecayEBase = .5
	r.DecayESpan = -.4
	r.DecayEMin = .1
	r.DecayEMax = .9
	return r
}

// TestReceptorGateClosedWhenConcentrationIsZero runs the receptor-driven gate
// on a timeline that never releases. The plastic change only decays and stays
// exactly zero, the forward is bit for bit the chemistry-only run, and the
// eligibility keeps tracking the activity. The report records that the gate
// was read from the receptor.
func TestReceptorGateClosedWhenConcentrationIsZero(t *testing.T) {
	a := newIndividual(t, receptorContinuousConfig(), receptorContinuousParameters())
	enableChemistry(t, a, silentChemistryDeclaration())
	if err := a.EnablePlasticity(plasticity.Config{Rule: receptorGateRule(), Edges: []int{0}}); err != nil {
		t.Fatal(err)
	}

	closed := newIndividual(t, receptorContinuousConfig(), receptorContinuousParameters())
	enableChemistry(t, closed, silentChemistryDeclaration())
	reference := advance(t, closed, receptorInput())

	out, report := advanceGated(t, a, receptorInput(), make([]float64, len(receptorInput())))
	if !sameRows(out, reference) {
		t.Fatalf("a never-open receptor gate changed the forward:\n got %v\nwant %v", out, reference)
	}
	if report.Steps != 3 || !report.GateFromReceptor || report.WindowFromReceptor {
		t.Fatalf("report = %+v", report)
	}
	s := a.Snapshot()
	if p := s.Plastic.State.Plastic[0]; p != 0 {
		t.Fatalf("plastic = %v, want 0 while the gate never opens", p)
	}
	if !closeEnough(s.Plastic.State.Eligibility[0], gateEligClosed) {
		t.Fatalf("eligibility = %.17g, want %.17g", s.Plastic.State.Eligibility[0], gateEligClosed)
	}
}

// TestReceptorGateOpensWithOccupancy pulses receptor 0 at step 1. The mid
// plastic equals occ1 * elig1 exactly, and the last row integrates the raised
// weight, so the final fast state is above the closed-gate run.
func TestReceptorGateOpensWithOccupancy(t *testing.T) {
	mid := newIndividual(t, receptorContinuousConfig(), receptorContinuousParameters())
	enableChemistry(t, mid, chemistryDeclaration())
	if err := mid.EnablePlasticity(plasticity.Config{Rule: receptorGateRule(), Edges: []int{0}}); err != nil {
		t.Fatal(err)
	}
	_, midReport := advanceGated(t, mid, receptorInput()[:2], make([]float64, 2))
	if !midReport.GateFromReceptor || midReport.WindowFromReceptor {
		t.Fatalf("mid report = %+v", midReport)
	}
	ms := mid.Snapshot()
	if !closeEnough(ms.Plastic.State.Eligibility[0], gateEligMid) {
		t.Fatalf("mid eligibility = %.17g, want %.17g", ms.Plastic.State.Eligibility[0], gateEligMid)
	}
	if !closeEnough(ms.Plastic.State.Plastic[0], gatePlasticMid) {
		t.Fatalf("mid plastic = %.17g, want %.17g", ms.Plastic.State.Plastic[0], gatePlasticMid)
	}
	if !closeEnough(gatePlasticMid, chemOcc1*gateEligMid) {
		t.Fatalf("mid plastic should equal occ1*elig1 = %.17g", chemOcc1*gateEligMid)
	}

	a := newIndividual(t, receptorContinuousConfig(), receptorContinuousParameters())
	enableChemistry(t, a, chemistryDeclaration())
	if err := a.EnablePlasticity(plasticity.Config{Rule: receptorGateRule(), Edges: []int{0}}); err != nil {
		t.Fatal(err)
	}
	_, report := advanceGated(t, a, receptorInput(), make([]float64, len(receptorInput())))
	if !report.GateFromReceptor || report.WindowFromReceptor {
		t.Fatalf("report = %+v", report)
	}
	s := a.Snapshot()
	if !closeEnough(s.Plastic.State.Eligibility[0], gateEligFinal) {
		t.Fatalf("eligibility = %.17g, want %.17g", s.Plastic.State.Eligibility[0], gateEligFinal)
	}
	if !closeEnough(s.Plastic.State.Plastic[0], gatePlasticFinal) {
		t.Fatalf("plastic = %.17g, want %.17g", s.Plastic.State.Plastic[0], gatePlasticFinal)
	}
}

// TestReceptorWindowChangesDecayE pulses receptor 0 and shortens the
// eligibility decay from 0.5 to the clamped values. Row 1 applies
// window = 0.2554075598308088, and the final eligibility
// 0.4158818857906775 is the amount the clamped row-2 decay kept, below the
// 0.4579164488274385 a constant 0.5 decay would leave. The plastic change
// only decays because no gate is declared.
func TestReceptorWindowChangesDecayE(t *testing.T) {
	if !closeEnough(windowDecayE(chemOcc1), windowDecayE1) {
		t.Fatalf("window(occ1) = %.17g, want %.17g", windowDecayE(chemOcc1), windowDecayE1)
	}

	a := newIndividual(t, receptorContinuousConfig(), receptorContinuousParameters())
	enableChemistry(t, a, chemistryDeclaration())
	if err := a.EnablePlasticity(plasticity.Config{Rule: receptorWindowRule(), Edges: []int{0}}); err != nil {
		t.Fatal(err)
	}
	_, report := advanceGated(t, a, receptorInput(), make([]float64, len(receptorInput())))
	if !report.WindowFromReceptor || report.GateFromReceptor {
		t.Fatalf("report = %+v", report)
	}
	s := a.Snapshot()
	if !closeEnough(s.Plastic.State.Eligibility[0], windowEligFinal) {
		t.Fatalf("eligibility = %.17g, want %.17g", s.Plastic.State.Eligibility[0], windowEligFinal)
	}
	if p := s.Plastic.State.Plastic[0]; p != 0 {
		t.Fatalf("plastic = %v, want 0 with no gate declared", p)
	}
}

func windowDecayE(occ float64) float64 {
	w := .5 - .4*occ
	if w < .1 {
		return .1
	}
	if w > .9 {
		return .9
	}
	return w
}

// TestReceptorRuleRejectedWithoutChemistry refuses a rule that names a receptor
// the enabled chemistry does not serve, on every ordering that would otherwise
// read a missing gate or window at the first step.
func TestReceptorRuleRejectedWithoutChemistry(t *testing.T) {
	for _, tc := range []struct {
		name string
		rule plasticity.Rule
	}{
		{"gate", receptorGateRule()},
		{"window", receptorWindowRule()},
	} {
		a := newIndividual(t, receptorContinuousConfig(), receptorContinuousParameters())
		err := a.EnablePlasticity(plasticity.Config{Rule: tc.rule, Edges: []int{0}})
		if err == nil || !strings.Contains(err.Error(), "receptor") {
			t.Fatalf("%s rule without chemistry, error = %v", tc.name, err)
		}
	}

	// A receptor index beyond the declared records is refused on the
	// individual too, not just in the plastic model.
	oob := newIndividual(t, receptorContinuousConfig(), receptorContinuousParameters())
	enableChemistry(t, oob, chemistryDeclaration())
	r := receptorGateRule()
	idx := 7
	r.GateReceptor = &idx
	err := oob.EnablePlasticity(plasticity.Config{Rule: r, Edges: []int{0}})
	if err == nil || !strings.Contains(err.Error(), "receptor") {
		t.Fatalf("out-of-range receptor, error = %v", err)
	}

	// Chemistry first, then the receptor rule: accepted in that order.
	ok := newIndividual(t, receptorContinuousConfig(), receptorContinuousParameters())
	enableChemistry(t, ok, chemistryDeclaration())
	if err := ok.EnablePlasticity(plasticity.Config{Rule: receptorGateRule(), Edges: []int{0}}); err != nil {
		t.Fatalf("chemistry-first ordering, error = %v", err)
	}

	// Plasticity first (with a plain rule), then chemistry: accepted too.
	both := newIndividual(t, receptorContinuousConfig(), receptorContinuousParameters())
	if err := both.EnablePlasticity(plasticity.Config{Rule: plasticRule(), Edges: []int{0}}); err != nil {
		t.Fatal(err)
	}
	enableChemistry(t, both, silentChemistryDeclaration())
}

// TestExplicitGateCannotCombineWithReceptorGate rejects an explicit nonzero
// gate on a rule whose gate the receptor also drives, and commits nothing on
// the failed call.
func TestExplicitGateCannotCombineWithReceptorGate(t *testing.T) {
	a := newIndividual(t, receptorContinuousConfig(), receptorContinuousParameters())
	enableChemistry(t, a, chemistryDeclaration())
	if err := a.EnablePlasticity(plasticity.Config{Rule: receptorGateRule(), Edges: []int{0}}); err != nil {
		t.Fatal(err)
	}
	_, _, err := a.AdvanceGated(context.Background(), receptorInput(), []float64{.5, 0, 0})
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") || !strings.Contains(err.Error(), "receptor") {
		t.Fatalf("AdvanceGated() error = %v", err)
	}
	s := a.Snapshot()
	if s.Plastic.State.Plastic[0] != 0 || s.Plastic.State.Eligibility[0] != 0 {
		t.Fatalf("failed call committed fast state: plastic %v eligibility %v", s.Plastic.State.Plastic[0], s.Plastic.State.Eligibility[0])
	}
}

// TestDisableChemistryRefusedWhileRuleNeedsReceptor keeps the layer on until
// the receptor-referencing rule is gone, on both the gate and the window path.
func TestDisableChemistryRefusedWhileRuleNeedsReceptor(t *testing.T) {
	for _, tc := range []struct {
		name string
		rule plasticity.Rule
	}{
		{"gate", receptorGateRule()},
		{"window", receptorWindowRule()},
	} {
		a := newIndividual(t, receptorContinuousConfig(), receptorContinuousParameters())
		enableChemistry(t, a, chemistryDeclaration())
		if err := a.EnablePlasticity(plasticity.Config{Rule: tc.rule, Edges: []int{0}}); err != nil {
			t.Fatal(err)
		}
		err := a.DisableChemistry()
		if err == nil || !strings.Contains(err.Error(), "receptor") {
			t.Fatalf("%s rule, DisableChemistry() error = %v", tc.name, err)
		}
		if a.Snapshot().Chemical == nil {
			t.Fatalf("%s rule: refused disable still dropped the chemistry layer", tc.name)
		}
		a.DisablePlasticity()
		if err := a.DisableChemistry(); err != nil {
			t.Fatalf("%s rule after DisablePlasticity, error = %v", tc.name, err)
		}
		if a.Snapshot().Chemical != nil {
			t.Fatalf("%s rule: chemistry stayed after a successful disable", tc.name)
		}
	}
}
