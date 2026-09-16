package simulate

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/plasticity"
)

// fixtureRule is the hebbian_rate declaration every hand calculation below
// uses: both decays are one half, the bound is far above anything the fixture
// reaches and the floor only matters for the fixed sign test.
func fixtureRule() plasticity.Rule {
	return plasticity.Rule{
		Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 8, WMin: .0625,
	}
}

// plasticStimulus is the two channel inline stimulus the plastic fixtures run
// on: channel 0 carries the same single two-unit pulse at step 0 that
// lifProtocol's pulse generator produces, channel 1 carries a constant gate.
// Two channels are needed because the gate channel must be distinct from every
// injection channel and the pulse generator writes only one channel.
func plasticStimulus(steps int, gate float64) [][]float64 {
	rows := make([][]float64, steps)
	for t := range rows {
		rows[t] = []float64{0, gate}
		if t == 0 {
			rows[t][0] = 2
		}
	}
	return rows
}

// plasticLIFProtocol is lifProtocol with that stimulus and, when enable is
// true, the plasticity block with the given gate scale.
func plasticLIFProtocol(steps int, gate, scale float64, enable bool) Protocol {
	p := lifProtocol(steps)
	p.Stimulus = StimulusSpec{Inline: plasticStimulus(steps, gate)}
	if enable {
		p.Plasticity = &Plasticity{
			Rule: fixtureRule(), All: true, GateChannel: 1, GateScale: scale,
		}
	}
	return p
}

// TestPlasticRunnerMatchesTheHandCalculatedTwoStepFixture pins the two step
// hand calculation of the ticket. The fixture graph is 0->1 (edge 0, raw weight
// 2) and 1->2 (edge 1, raw weight 3); gain 2 makes the base weights 4 and 6 and
// the uniform source declares no signs, so both edges are free and
// w_eff = w_base + plastic exactly.
//
// The LIF core of lifProtocol has dt = tau_syn = 1, so lambda = kappa = k with
// k = exp(-1) and alpha = 1-k, and theta_raw 0 puts the base threshold at 1.
// The pre signal of an edge is the synaptic trace of its source, the post
// signal is the 0/1 event of its target, and the gate is 1 at every step.
//
//	step 0: drive = [2,0,0]; cand[0] = alpha*2 = 1.264 >= 1 so node 0 spikes.
//	        outputs = [1,0,0], events = [1,0,0].
//	        edge 0 = (0->1): elig = 0.5*0 + pre[0]*post[1] = 1*0 = 0
//	        edge 1 = (1->2): elig = 0.5*0 + pre[1]*post[2] = 0*0 = 0
//	        both fast changes stay 0, so step 1 still integrates 4 and 6.
//	step 1: drive = [0, 4*1, 6*0]; cand[1] = alpha*4 = 2.528 >= 1 so node 1
//	        spikes. outputs = [k,1,0], events = [0,1,0].
//	        edge 0: elig = 0.5*0 + k*1 = k, plastic = 0.5*0 + 1*k = k
//	        edge 1: elig = 0.5*0 + 1*0 = 0, plastic = 0
//
// Two steps therefore end with plastic [k, 0] and eligibility [k, 0], and the
// probe series are still those of the run without the block, because the only
// non-zero fast change was produced by the last step of the run.
func TestPlasticRunnerMatchesTheHandCalculatedTwoStepFixture(t *testing.T) {
	g := fixtureGraph(t)
	k := math.Exp(-1)

	_, plain := mustRun(t, g, plasticLIFProtocol(2, 1, 1, false))
	runner, report := mustRun(t, g, plasticLIFProtocol(2, 1, 1, true))

	if report.Plasticity == nil {
		t.Fatal("the report carries no plasticity block")
	}
	got := *report.Plasticity
	if got.Rule != plasticity.RuleHebbianRate || got.EnabledEdges != 2 || got.GateChannel != 1 {
		t.Fatalf("plasticity report identity = %+v", got)
	}
	if got.ChunkSize != 1 {
		t.Fatalf("chunk size = %d, want 1 while plasticity is enabled", got.ChunkSize)
	}
	if got.Frozen {
		t.Fatalf("plasticity report = %+v, want updates enabled", got)
	}
	if got.ClampedByWMin != 0 || got.ClampedByPlasticMax != 0 {
		t.Fatalf("clamp counts = %d w_min, %d plastic_max, want none", got.ClampedByWMin, got.ClampedByPlasticMax)
	}
	if got.PlasticL2Before != 0 {
		t.Fatalf("plastic l2 before = %v, want 0", got.PlasticL2Before)
	}
	if want := math.Sqrt(k*k + 0*0); got.PlasticL2After != want {
		t.Fatalf("plastic l2 after = %v, want %v", got.PlasticL2After, want)
	}
	if got.WallClockPenaltyNote == "" {
		t.Fatal("the plasticity report states no wall clock penalty")
	}

	state := runner.PlasticState()
	if !equalSeries(state.Eligibility, []float64{k, 0}) {
		t.Fatalf("eligibility = %v, want [%v 0]", state.Eligibility, k)
	}
	if !equalSeries(state.Plastic, []float64{k, 0}) {
		t.Fatalf("plastic = %v, want [%v 0]", state.Plastic, k)
	}
	if state.PreTrace != nil || state.PostTrace != nil {
		t.Fatalf("hebbian_rate carries pair traces %v %v", state.PreTrace, state.PostTrace)
	}

	// Nothing the two steps produced has reached a weight yet, so every probe
	// is still the series of the run without the block.
	for _, name := range []string{"first", "all", "spikes", "counted"} {
		if want, have := probeSeries(t, plain, name), probeSeries(t, report, name); !equalSeries(have, want) {
			t.Fatalf("probe %q = %v, want %v", name, have, want)
		}
	}
}

// TestPlasticWeightsReachTheNextStep is the same fixture with gate_scale 4 and
// one more step, which is the smallest change that makes the fast weight
// visible in the output rather than only in the state.
//
// After step 1 the fast change of edge 0 is 4k = 1.4715, so step 2 integrates
// 4+4k = 5.4715 instead of 4:
//
//	cand[1] = k*(-0.5) + alpha*(4+4k)*k = 1.0884 >= 1, node 1 spikes
//	without the block cand[1] = k*(-0.5) + alpha*4*k = 0.7463 < 1, it does not
//
// Node 2 receives 6*1 and spikes in both runs, so the spike fraction of step 2
// is 2/3 with plasticity and 1/3 without. The fast state after step 2 is
//
//	edge 0: elig = 0.5*k + k*k, plastic = 0.5*(4k) + 4*(0.5*k + k*k)
//	edge 1: elig = 0.5*0 + (k+1)*1, plastic = 0.5*0 + 4*(k+1)
//
// because the pre signal of step 2 is the trace [k*k, k+1, 1] and the events
// are [0,1,1].
func TestPlasticWeightsReachTheNextStep(t *testing.T) {
	g := fixtureGraph(t)
	k := math.Exp(-1)

	_, plain := mustRun(t, g, plasticLIFProtocol(3, 1, 4, false))
	runner, report := mustRun(t, g, plasticLIFProtocol(3, 1, 4, true))

	if got := probeSeries(t, plain, "spikes"); !equalSeries(got, []float64{1. / 3, 1. / 3, 1. / 3}) {
		t.Fatalf("spike fraction without plasticity = %v", got)
	}
	if got := probeSeries(t, report, "spikes"); !equalSeries(got, []float64{1. / 3, 1. / 3, 2. / 3}) {
		t.Fatalf("spike fraction with plasticity = %v, want the extra step 2 spike", got)
	}

	elig := []float64{0.5*k + k*k, 0.5*0 + (k+1)*1}
	plastic := []float64{0.5*(4*k) + 4*elig[0], 0.5*0 + 4*elig[1]}
	state := runner.PlasticState()
	if !equalSeries(state.Eligibility, elig) {
		t.Fatalf("eligibility = %v, want %v", state.Eligibility, elig)
	}
	if !equalSeries(state.Plastic, plastic) {
		t.Fatalf("plastic = %v, want %v", state.Plastic, plastic)
	}
	want := math.Sqrt(plastic[0]*plastic[0] + plastic[1]*plastic[1])
	if report.Plasticity.PlasticL2After != want {
		t.Fatalf("plastic l2 after = %v, want %v", report.Plasticity.PlasticL2After, want)
	}
	// The weights the next step would integrate are base + plastic, bit for
	// bit, because the uniform source declares no signs.
	effective, held, err := runner.plastic.Effective(runner.params.Weights, runner.params.Signs, state)
	if err != nil {
		t.Fatalf("effective: %v", err)
	}
	if held.HeldAtWMin != 0 || !equalSeries(effective, []float64{4 + plastic[0], 6 + plastic[1]}) {
		t.Fatalf("effective weights = %v, held %d", effective, held.HeldAtWMin)
	}
}

// TestFrozenZeroPlasticityReproducesTheReportWithoutTheBlock is root decision 4
// of the ticket. A declared block whose fast changes are all zero and whose
// updates are frozen must leave every observable of the run where it was.
//
// protocol_hash is the one field that cannot be equal: it fingerprints the
// declaration, and a protocol that declares the block is a different
// declaration. The test therefore requires byte equality of the whole report
// JSON once the plasticity block is removed and protocol_hash is taken out, and
// checks separately that the hash is exactly the hash of the protocol that was
// actually run. The literal, whole-report form of the decision is checked in
// TestCompareOriginalCellIsTheReportWithoutTheBlock, where the original cell of
// a plastic comparison runs the protocol with the block stripped.
func TestFrozenZeroPlasticityReproducesTheReportWithoutTheBlock(t *testing.T) {
	g := fixtureGraph(t)
	plainProtocol := plasticLIFProtocol(4, 1, 1, false)
	frozenProtocol := plasticLIFProtocol(4, 1, 1, true)
	frozenProtocol.Plasticity.Frozen = true

	plainRunner, plain := mustRun(t, g, plainProtocol)
	frozenRunner, frozen := mustRun(t, g, frozenProtocol)

	if frozen.Plasticity == nil || !frozen.Plasticity.Frozen {
		t.Fatalf("frozen report = %+v", frozen.Plasticity)
	}
	if frozen.Plasticity.PlasticL2Before != 0 || frozen.Plasticity.PlasticL2After != 0 {
		t.Fatalf("a frozen run moved the fast changes: %+v", frozen.Plasticity)
	}
	state := frozenRunner.PlasticState()
	for i, v := range state.Plastic {
		if v != 0 || state.Eligibility[i] != 0 {
			t.Fatalf("frozen state entry %d = %v %v, want zero", i, v, state.Eligibility[i])
		}
	}
	plainHash, err := plainProtocol.Hash()
	if err != nil {
		t.Fatal(err)
	}
	frozenHash, err := frozenProtocol.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if plain.ProtocolHash != plainHash || frozen.ProtocolHash != frozenHash {
		t.Fatalf("protocol hashes = %q %q", plain.ProtocolHash, frozen.ProtocolHash)
	}
	if plainHash == frozenHash {
		t.Fatal("declaring the block left the protocol hash unchanged")
	}
	if got, want := stripped(t, frozen), stripped(t, plain); got != want {
		t.Fatalf("frozen report differs from the report without the block:\n%s\n%s", got, want)
	}
	sameAssumptionsPlusOne(t, plain, frozen, plasticity.RuleHebbianRate)
	// The neural trajectories are the same object, not only the same numbers
	// in the report.
	if got, want := mustJSON(t, frozenRunner.State()), mustJSON(t, plainRunner.State()); got != want {
		t.Fatalf("frozen state = %s, want %s", got, want)
	}
}

// stripped encodes a report without the three fields a declared block is meant
// to change: the plasticity block itself, protocol_hash, which fingerprints the
// declaration and therefore cannot be equal, and assumptions, which gains
// exactly one sentence naming the rule. Every other field, including every
// probe value, every monitor and every hash of the wiring and the parameters,
// must be byte identical. sameAssumptionsPlusOne pins the third exclusion so it
// stays an addition rather than a change.
func stripped(t *testing.T, report RunReport) string {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal([]byte(mustJSON(t, report)), &document); err != nil {
		t.Fatal(err)
	}
	delete(document, "plasticity")
	delete(document, "protocol_hash")
	delete(document, "assumptions")
	return mustJSON(t, document)
}

// sameAssumptionsPlusOne requires the plastic report to repeat every assumption
// of the plain report in order and to add exactly one sentence, which names the
// declared rule.
func sameAssumptionsPlusOne(t *testing.T, plain, plastic RunReport, rule string) {
	t.Helper()
	if len(plastic.Assumptions) != len(plain.Assumptions)+1 {
		t.Fatalf("plastic report has %d assumptions, want %d plus one", len(plastic.Assumptions), len(plain.Assumptions))
	}
	for i, line := range plain.Assumptions {
		if plastic.Assumptions[i] != line {
			t.Fatalf("assumption %d changed to %q", i, plastic.Assumptions[i])
		}
	}
	added := plastic.Assumptions[len(plain.Assumptions)]
	if !strings.Contains(added, rule) || !strings.Contains(added, "never written back") {
		t.Fatalf("the added assumption is %q", added)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

// TestPlasticityForcesChunkSizeOne checks the forced chunk size against the
// same protocol without the block, which the fixture graph runs in a single
// chunk. TestChunkBoundariesDoNotChangeTheRun already pins that an explicit
// chunk size changes no value; this one pins that the plastic path takes that
// route and that a disabled rule therefore reproduces the plain run exactly.
func TestPlasticityForcesChunkSizeOne(t *testing.T) {
	g := fixtureGraph(t)
	disabled := plasticLIFProtocol(5, 1, 0, true) // gate_scale 0: nothing is ever added
	params := mustParameters(t, g, *disabled.Uniform)

	plainRunner, err := Build(context.Background(), g, params, plasticLIFProtocol(5, 1, 0, false), testLimits())
	if err != nil {
		t.Fatalf("build plain: %v", err)
	}
	if plainRunner.maxChunk < 5 {
		t.Fatalf("the fixture graph already chunks at %d steps", plainRunner.maxChunk)
	}
	plain, err := plainRunner.Run(context.Background(), plainRunner.Stimulus())
	if err != nil {
		t.Fatalf("run plain: %v", err)
	}
	runner, err := Build(context.Background(), g, params, disabled, testLimits())
	if err != nil {
		t.Fatalf("build plastic: %v", err)
	}
	if runner.maxChunk != 1 {
		t.Fatalf("plastic runner chunk = %d, want 1", runner.maxChunk)
	}
	report, err := runner.Run(context.Background(), runner.Stimulus())
	if err != nil {
		t.Fatalf("run plastic: %v", err)
	}
	if report.Plasticity.PlasticL2After != 0 {
		t.Fatalf("a zero gate produced fast changes: %+v", report.Plasticity)
	}
	if got, want := stripped(t, report), stripped(t, plain); got != want {
		t.Fatalf("five steps of chunk 1 differ from one chunk of five:\n%s\n%s", got, want)
	}
	sameAssumptionsPlusOne(t, plain, report, plasticity.RuleHebbianRate)
}

// TestPlasticContinuousCoreUsesOutputsAsBothSignals pins the continuous branch:
// the core emits no events, so the rule reads the activated output on both
// sides of every edge. The expected values are re-derived here from the two
// published step rules rather than read back from the implementation.
//
// The continuous step with dt 1 and log_tau 0 is
//
//	v(t+1) = k*v(t) + a*drive(t),  y(t+1) = tanh(v(t+1))
//
// with k = exp(-1) and a = 1-exp(-1); drive carries the injected stimulus plus
// the weighted outputs of the previous step, which are 4*y[0] into node 1 and
// 6*y[1] into node 2. The rule then reads
//
//	elig_e(t+1) = 0.5*elig_e(t) + y_source * y_target
//	plastic_e(t+1) = 0.5*plastic_e(t) + gate * elig_e(t+1),  gate = 1
//
// Node 0 keeps a decaying output after the pulse instead of returning to zero,
// so edge 0 already has a non-zero eligibility after step 1, while edge 1 needs
// step 2, where node 1 and node 2 are active at the same time. The fast change
// of edge 0 is therefore already in the weight of step 2, which the reference
// below carries as w = base + plastic exactly as Effective does.
func TestPlasticContinuousCoreUsesOutputsAsBothSignals(t *testing.T) {
	g := fixtureGraph(t)
	protocol := plasticLIFProtocol(3, 1, 1, true)
	protocol.Core = CoreContinuous
	protocol.LIF = nil
	protocol.Continuous = continuousFixtureConfig()
	protocol.Probes = []Probe{{Name: "all", Nodes: []int{0, 1, 2}, Reduce: ReduceSumOutput}}

	k, a := math.Exp(-1), -math.Expm1(-1)
	v := []float64{0, 0, 0}
	y := []float64{0, 0, 0}
	elig := []float64{0, 0}
	plastic := []float64{0, 0}
	for step := range 3 {
		// Both edges are free, so the weights of this step are base + plastic.
		w := []float64{4 + plastic[0], 6 + plastic[1]}
		drive := []float64{0, w[0] * y[0], w[1] * y[1]}
		if step == 0 {
			drive[0] = 2
		}
		v = []float64{k*v[0] + a*drive[0], k*v[1] + a*drive[1], k*v[2] + a*drive[2]}
		y = []float64{math.Tanh(v[0]), math.Tanh(v[1]), math.Tanh(v[2])}
		elig = []float64{0.5*elig[0] + y[0]*y[1], 0.5*elig[1] + y[1]*y[2]}
		plastic = []float64{0.5*plastic[0] + 1*elig[0], 0.5*plastic[1] + 1*elig[1]}
	}

	runner, report := mustRun(t, g, protocol)
	state := runner.PlasticState()
	if !equalSeries(state.Eligibility, elig) {
		t.Fatalf("eligibility = %v, want %v", state.Eligibility, elig)
	}
	if !equalSeries(state.Plastic, plastic) {
		t.Fatalf("plastic = %v, want %v", state.Plastic, plastic)
	}
	if report.Plasticity.EnabledEdges != 2 || report.Plasticity.ChunkSize != 1 {
		t.Fatalf("plasticity report = %+v", report.Plasticity)
	}
	// The first step cannot produce a fast change on either edge: node 1 and
	// node 2 are both silent while only node 0 has been driven.
	if elig[0] == 0 || elig[1] == 0 {
		t.Fatalf("the reference calculation left an eligibility at zero: %v", elig)
	}
}

// TestPlasticitySelectorEnablesOnlyTheEdgesInsideTheSet checks the selector
// form of the block. class ALPN resolves to nodes 1 and 2, whose only internal
// edge is 1->2, so exactly one of the two fixture edges is enabled and the
// other keeps its base weight for the whole run.
func TestPlasticitySelectorEnablesOnlyTheEdgesInsideTheSet(t *testing.T) {
	g := fixtureGraph(t)
	protocol := plasticLIFProtocol(3, 1, 4, true)
	protocol.Plasticity.All = false
	protocol.Plasticity.Edges = &Selector{Field: "class", Equals: "ALPN"}

	runner, report := mustRun(t, g, protocol)
	if report.Plasticity.EnabledEdges != 1 {
		t.Fatalf("enabled edges = %d, want only 1->2", report.Plasticity.EnabledEdges)
	}
	if got := len(runner.PlasticState().Plastic); got != 1 {
		t.Fatalf("fast state has %d entries", got)
	}
	// Edge 0 is not enabled, so the step 2 spike of TestPlasticWeightsReachTheNextStep
	// does not happen here: only 1->2 can change.
	if got := probeSeries(t, report, "spikes"); !equalSeries(got, []float64{1. / 3, 1. / 3, 1. / 3}) {
		t.Fatalf("spike fraction = %v, want the unchanged series", got)
	}
}

// TestPlasticFixedSignEdgeIsHeldAtWMin runs the derived fixture, whose edges
// carry rule derived signs, with a rule whose bound is large enough to push a
// fixed sign edge past zero. The floor must hold it at w_min with the declared
// sign and the report must count every edge it held.
func TestPlasticFixedSignEdgeIsHeldAtWMin(t *testing.T) {
	set, g := derivedFixtureSet(t)
	protocol := derivedProtocol(CoreLIF, UnknownSignExclude, fixtureWeightScale, 3)
	protocol.Stimulus = StimulusSpec{Inline: plasticStimulus(3, 1)}
	protocol.Plasticity = &Plasticity{
		Rule: plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 8, WMin: .0625},
		All:  true, GateChannel: 1, GateScale: 1,
	}
	parameters, _, err := FromDerived(set, fixtureSetSHA256, protocol)
	if err != nil {
		t.Fatalf("from derived: %v", err)
	}
	if len(parameters.Signs) != len(parameters.Weights) {
		t.Fatalf("the derived parameter set carries %d signs for %d weights", len(parameters.Signs), len(parameters.Weights))
	}
	runner, err := Build(context.Background(), g, parameters, protocol, testLimits())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// Drive every enabled edge below zero by hand and read the weights back.
	state := runner.PlasticState()
	for i := range state.Plastic {
		state.Plastic[i] = -8
	}
	if err := runner.RestorePlasticState(state); err != nil {
		t.Fatalf("restore plastic state: %v", err)
	}
	effective, held, err := runner.plastic.Effective(parameters.Weights, parameters.Signs, runner.PlasticState())
	if err != nil {
		t.Fatalf("effective: %v", err)
	}
	fixed := 0
	for i, sign := range parameters.Signs {
		switch sign {
		case 0:
			if effective[i] != parameters.Weights[i]-8 {
				t.Fatalf("free edge %d = %v", i, effective[i])
			}
		default:
			fixed++
			if effective[i] != float64(sign)*.0625 {
				t.Fatalf("fixed sign edge %d = %v, want %v times the floor", i, effective[i], sign)
			}
		}
	}
	if fixed == 0 {
		t.Fatal("the derived fixture declared no fixed sign edge")
	}
	if held.HeldAtWMin != fixed {
		t.Fatalf("held at w_min = %d, want %d", held.HeldAtWMin, fixed)
	}
	report, err := runner.Run(context.Background(), runner.Stimulus())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if report.Plasticity.ClampedByWMin < uint64(fixed) {
		t.Fatalf("the run counted %d w_min clamps over three steps", report.Plasticity.ClampedByWMin)
	}
}

// TestPlasticMaxIsCountedAndHolds pins the other bound: a gate large enough to
// drive the fast change past plastic_max leaves it at the bound and the report
// counts every entry the bound held.
func TestPlasticMaxIsCountedAndHolds(t *testing.T) {
	g := fixtureGraph(t)
	protocol := plasticLIFProtocol(3, 1, 64, true)
	protocol.Plasticity.Rule.PlasticMax = 1

	runner, report := mustRun(t, g, protocol)
	state := runner.PlasticState()
	for i, v := range state.Plastic {
		if math.Abs(v) > 1 {
			t.Fatalf("plastic[%d] = %v, outside the declared bound", i, v)
		}
	}
	if report.Plasticity.ClampedByPlasticMax == 0 {
		t.Fatalf("no clamp was counted: %+v", report.Plasticity)
	}
}

// TestPlasticStateRoundTripsAndIsIndependent checks the two accessors the
// frozen variant needs: the returned state is a copy, and restoring one is
// validated against the declared rule and edge selection.
func TestPlasticStateRoundTripsAndIsIndependent(t *testing.T) {
	g := fixtureGraph(t)
	runner, _ := mustRun(t, g, plasticLIFProtocol(3, 1, 4, true))

	state := runner.PlasticState()
	state.Plastic[0] = 99
	if runner.PlasticState().Plastic[0] == 99 {
		t.Fatal("PlasticState shares its buffers with the runner")
	}
	if err := runner.RestorePlasticState(state); err == nil {
		t.Fatal("a fast change outside plastic_max was accepted")
	}
	state.Plastic[0] = 1.5
	if err := runner.RestorePlasticState(state); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if runner.PlasticState().Plastic[0] != 1.5 {
		t.Fatal("the restored fast change did not reach the runner")
	}
	if err := runner.RestorePlasticState(plasticity.State{Eligibility: []float64{0}, Plastic: []float64{0}}); err == nil {
		t.Fatal("a state of the wrong length was accepted")
	}

	plain, _ := mustRun(t, g, plasticLIFProtocol(3, 1, 4, false))
	if got := plain.PlasticState(); got.Plastic != nil || got.Eligibility != nil {
		t.Fatalf("a runner without the block carries a fast state %+v", got)
	}
	if err := plain.RestorePlasticState(plasticity.State{}); err == nil {
		t.Fatal("a runner without the block accepted a fast state")
	}
}

// TestPlasticityValidationRejectsTheDeclarationErrors covers every rule the
// block adds to Validate. Each case is graph independent, so Validate catches
// it before anything is built.
func TestPlasticityValidationRejectsTheDeclarationErrors(t *testing.T) {
	cases := []struct {
		name  string
		mut   func(*Protocol)
		wants string
	}{
		{"neither edges nor all", func(p *Protocol) { p.Plasticity.All = false }, "exactly one"},
		{"both edges and all", func(p *Protocol) {
			p.Plasticity.Edges = &Selector{Field: "class", Equals: "ALPN"}
		}, "exactly one"},
		{"gate channel outside the stimulus", func(p *Protocol) { p.Plasticity.GateChannel = 2 }, "gate_channel"},
		{"negative gate channel", func(p *Protocol) { p.Plasticity.GateChannel = -1 }, "gate_channel"},
		{"gate channel is an injection channel", func(p *Protocol) { p.Plasticity.GateChannel = 0 }, "injection"},
		{"non-finite gate scale", func(p *Protocol) { p.Plasticity.GateScale = math.Inf(1) }, "gate_scale"},
		{"unknown rule", func(p *Protocol) { p.Plasticity.Rule.Kind = "hebbian" }, "hebbian_rate"},
		{"decay outside the range", func(p *Protocol) { p.Plasticity.Rule.DecayE = 1 }, "decay_e"},
		{"rate rule carrying a pair field", func(p *Protocol) { p.Plasticity.Rule.APlus = 1 }, "a_plus"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := plasticLIFProtocol(3, 1, 1, true)
			tc.mut(&p)
			err := p.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.wants) {
				t.Fatalf("Validate() = %v, want an error naming %q", err, tc.wants)
			}
		})
	}
}

// TestSTDPPairNeedsASpikingCore rejects the pair rule on the continuous core at
// declaration time rather than at the first step, which is the same choice
// learning.EnablePlasticity made.
func TestSTDPPairNeedsASpikingCore(t *testing.T) {
	p := plasticLIFProtocol(3, 1, 1, true)
	p.Core = CoreContinuous
	p.LIF = nil
	p.Continuous = continuousFixtureConfig()
	p.Probes = []Probe{{Name: "all", Nodes: []int{0, 1, 2}, Reduce: ReduceSumOutput}}
	p.Plasticity.Rule = plasticity.Rule{
		Kind: plasticity.RuleSTDPPair, DecayE: .5, DecayP: .5, PlasticMax: 8, WMin: .0625,
		DecayPre: .5, DecayPost: .5, APlus: 1, AMinus: 1,
	}
	err := p.Validate()
	if err == nil || !strings.Contains(err.Error(), plasticity.RuleSTDPPair) {
		t.Fatalf("Validate() = %v, want a refusal naming the pair rule", err)
	}
}

// TestSTDPPairRunsOnTheSpikingCore is the hand calculation of the pair rule on
// the runner: the traces decay before they read and add their own event last,
// exactly as the plasticity package documents.
//
// With decay_pre = decay_post = 0.5, a_plus = a_minus = 1 and the fixture's
// pre-before-post order, node 0 spikes at step 0 and node 1 at step 1:
//
//	step 0 edge 0 = (0->1): no post event, elig = 0; pre_trace = 0.5*0+1 = 1
//	step 1 edge 0: post event, elig = 0.5*0 + 1*(0.5*1) = 0.5;
//	               plastic = 0.5*0 + 1*0.5 = 0.5
func TestSTDPPairRunsOnTheSpikingCore(t *testing.T) {
	g := fixtureGraph(t)
	p := plasticLIFProtocol(2, 1, 1, true)
	p.Plasticity.Rule = plasticity.Rule{
		Kind: plasticity.RuleSTDPPair, DecayE: .5, DecayP: .5, PlasticMax: 8, WMin: .0625,
		DecayPre: .5, DecayPost: .5, APlus: 1, AMinus: 1,
	}
	runner, report := mustRun(t, g, p)
	state := runner.PlasticState()
	if !equalSeries(state.Eligibility, []float64{0.5, 0}) {
		t.Fatalf("eligibility = %v, want [0.5 0]", state.Eligibility)
	}
	if !equalSeries(state.Plastic, []float64{0.5, 0}) {
		t.Fatalf("plastic = %v, want [0.5 0]", state.Plastic)
	}
	if !equalSeries(state.PreTrace, []float64{0.5, 1}) || !equalSeries(state.PostTrace, []float64{1, 0}) {
		t.Fatalf("pair traces = %v %v", state.PreTrace, state.PostTrace)
	}
	if report.Plasticity.Rule != plasticity.RuleSTDPPair {
		t.Fatalf("report rule = %q", report.Plasticity.Rule)
	}
}

// TestProtocolWithoutTheBlockKeepsTheRecordedHash is the compatibility test the
// ticket asks for: the NAT-01 protocol, written out again here byte for byte
// from evidence/NAT-01/protocol-fullgraph-uniform.json, must still hash to the
// value recorded in evidence/NAT-01/verification.json.
func TestProtocolWithoutTheBlockKeepsTheRecordedHash(t *testing.T) {
	const recorded = "befa9f1d340a49d8dbeed50a67ca72d711bc601710764d22cdfbef353ea6ce42"
	protocol, err := DecodeProtocol(strings.NewReader(nat01Protocol))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if protocol.Plasticity != nil {
		t.Fatal("the recorded protocol declares a plasticity block")
	}
	hash, err := protocol.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if hash != recorded {
		t.Fatalf("protocol hash = %q, want the recorded %q", hash, recorded)
	}
	// The field is a pointer with omitempty, so it is absent from the encoding
	// of a protocol that does not declare it.
	if strings.Contains(mustJSON(t, protocol), "plasticity") {
		t.Fatal("the encoded protocol mentions plasticity")
	}
}

// nat01Protocol is evidence/NAT-01/protocol-fullgraph-uniform.json, copied here
// so the test does not depend on the repository layout.
const nat01Protocol = `{
  "schema_version": "coimnet-simulate-protocol/v1",
  "core": "lif",
  "lif": {
    "dt": 1, "tau_syn": 5, "theta_min": 0.1, "theta_max": 2, "v_reset": -0.5,
    "refractory_steps": 1,
    "adaptation": {"enabled": false, "tau_adapt": 0, "beta": 0},
    "surrogate": {"kind": "fast_sigmoid", "scale": 2}
  },
  "injection_groups": [
    {"channel": 0, "selector": {"field": "class", "equals": "ALIN"}, "gain": 1}
  ],
  "probes": [
    {"name": "alin_injected_spikes", "selector": {"field": "class", "equals": "ALIN"}, "reduce": "spike_fraction"},
    {"name": "alin_injected_trace", "selector": {"field": "class", "equals": "ALIN"}, "reduce": "mean_output"},
    {"name": "descending_neuron_spikes", "selector": {"field": "superclass", "equals": "descending_neuron"}, "reduce": "spike_fraction"},
    {"name": "vnc_motor_spikes", "selector": {"field": "superclass", "equals": "vnc_motor"}, "reduce": "spike_fraction"}
  ],
  "stimulus": {"pulse": {"channels": 1, "steps": 300, "channel": 0, "onset": 10, "duration": 20, "amplitude": 2}},
  "thresholds": {"max_population_rate": 0.2, "min_active_fraction": 0.01},
  "parameter_source": "engineering_uniform_positive",
  "uniform": {"gain": 0.025, "bias": 0, "log_tau": 0, "theta_raw": 0}
}`

// TestPlasticRunIsRepeatableAndLeavesNothingBehindOnFailure repeats the
// determinism guarantee of the package for the plastic path and checks that a
// refused run leaves both the neural and the fast state where they were.
func TestPlasticRunIsRepeatableAndLeavesNothingBehindOnFailure(t *testing.T) {
	g := fixtureGraph(t)
	protocol := plasticLIFProtocol(3, 1, 4, true)
	_, first := mustRun(t, g, protocol)
	_, second := mustRun(t, g, protocol)
	if got, want := mustJSON(t, first), mustJSON(t, second); got != want {
		t.Fatal("two identical plastic runs produced different reports")
	}

	params := mustParameters(t, g, *protocol.Uniform)
	runner, err := Build(context.Background(), g, params, protocol, testLimits())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, err := runner.Run(context.Background(), runner.Stimulus()); err != nil {
		t.Fatalf("run: %v", err)
	}
	before := mustJSON(t, runner.PlasticState())
	bad := runner.Stimulus()
	bad[1][1] = math.Inf(1)
	if _, err := runner.Run(context.Background(), bad); err == nil {
		t.Fatal("a non-finite gate was accepted")
	}
	if after := mustJSON(t, runner.PlasticState()); after != before {
		t.Fatalf("a failed run moved the fast state to %s", after)
	}
}

// continuousFixtureConfig is the continuous core the fixture protocols run on:
// dt 1 and tanh, so one step is y(t+1) = tanh(W y(t) + input).
func continuousFixtureConfig() *dynamics.Config {
	return &dynamics.Config{DT: 1, Activation: "tanh"}
}

// TestPlasticL2OverflowIsRefused covers the one place a bounded rule can still
// produce a value the report cannot carry: the Euclidean norm squares every
// fast change, so a plastic_max above 1e150 overflows the sum even though every
// individual value is finite. The run must fail with an error rather than
// publish a report JSON that cannot be encoded.
func TestPlasticL2OverflowIsRefused(t *testing.T) {
	g := fixtureGraph(t)
	protocol := plasticLIFProtocol(3, 1, 1e200, true)
	protocol.Plasticity.Rule.PlasticMax = 1e200

	params := mustParameters(t, g, *protocol.Uniform)
	runner, err := Build(context.Background(), g, params, protocol, testLimits())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	before := mustJSON(t, runner.PlasticState())
	_, err = runner.Run(context.Background(), runner.Stimulus())
	if err == nil || !strings.Contains(err.Error(), "plastic l2") {
		t.Fatalf("Run() = %v, want a refusal naming the norm", err)
	}
	if after := mustJSON(t, runner.PlasticState()); after != before {
		t.Fatalf("the refused run moved the fast state to %s", after)
	}
}
