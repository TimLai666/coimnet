package simulate

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
)

// evidenceNAT01Protocol reads the official NAT-01 protocol document and the
// protocol hash the verification record pinned, so the compatibility test
// proves itself against the repository documents instead of a local constant.
func evidenceNAT01Protocol(t *testing.T) (Protocol, string) {
	t.Helper()
	dir := filepath.Join("..", "evidence", "NAT-01")
	reader, err := os.Open(filepath.Join(dir, "protocol-fullgraph-uniform.json"))
	if err != nil {
		t.Fatalf("open the recorded protocol: %v", err)
	}
	defer reader.Close()
	protocol, err := DecodeProtocol(reader)
	if err != nil {
		t.Fatalf("decode the recorded protocol: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "verification.json"))
	if err != nil {
		t.Fatalf("open the verification record: %v", err)
	}
	match := regexp.MustCompile(`report protocol_hash ([0-9a-f]{64})`).FindSubmatch(data)
	if match == nil {
		t.Fatal("the verification record names no report protocol_hash")
	}
	return protocol, string(match[1])
}

// TestInterventionsKeepProtocolHash is the compatibility contract of the
// ticket: a protocol that does not declare an interventions block must encode
// exactly as it did before the field existed, and its hash must stay the value
// recorded for the whole-brain NAT-01 run. A protocol that declares the block
// is a different declaration, so its hash must change.
func TestInterventionsKeepProtocolHash(t *testing.T) {
	protocol, recorded := evidenceNAT01Protocol(t)
	if protocol.Interventions != nil {
		t.Fatal("the recorded protocol declares an interventions block")
	}
	hash, err := protocol.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if hash != recorded {
		t.Fatalf("protocol hash = %q, want the recorded %q", hash, recorded)
	}
	if strings.Contains(mustJSON(t, protocol), "interventions") {
		t.Fatal("the encoded protocol mentions interventions")
	}

	experiment := protocol
	experiment.Interventions = &InterventionPlan{
		Authorized: true,
		Reason:     "compatibility test",
		Items: []Intervention{{
			Kind: InterventionKindSilence, Targets: []int{0}, Start: 0, End: 1,
		}},
	}
	experimentHash, err := experiment.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if experimentHash == recorded {
		t.Fatal("declaring the interventions block left the protocol hash unchanged")
	}
	if !strings.Contains(mustJSON(t, experiment), `"interventions"`) {
		t.Fatal("the encoded protocol does not carry the interventions block")
	}
}

// TestClampVoltageHoldsThenReleases runs the continuous core with node 0
// clamped to 0.25 on the rows [1, 3). Node 0 is driven by the single pulse at
// step 0, so without the clamp its output is y(t) = tanh(k^t * alpha * 2) with
// k = exp(-1) and alpha = 1-k. Clamped, its voltage is forced to 0.25 at steps
// 1 and 2, reads tanh(0.25) from the probe, and is handed back to the free rule
// at step 3, which the state of the clamp window makes different from the
// no-intervention run.
func TestClampVoltageHoldsThenReleases(t *testing.T) {
	g := fixtureGraph(t)
	protocol := lifProtocol(4)
	protocol.Core = CoreContinuous
	protocol.LIF = nil
	protocol.Continuous = continuousFixtureConfig()
	protocol.Probes = []Probe{
		{Name: "first", Nodes: []int{0}, Reduce: ReduceMeanOutput},
		{Name: "all", Nodes: []int{0, 1, 2}, Reduce: ReduceSumOutput},
	}
	protocol.Interventions = &InterventionPlan{
		Authorized: true, Reason: "clamp the driven node",
		Items: []Intervention{{
			Kind: InterventionKindClampVoltage, Targets: []int{0}, Value: .25, Start: 1, End: 3,
		}},
	}

	k, alpha := math.Exp(-1), -math.Expm1(-1)
	want := []float64{
		math.Tanh(alpha * 2),
		math.Tanh(.25),
		math.Tanh(.25),
		math.Tanh(k * .25),
	}
	_, report := mustRun(t, g, protocol)
	if got := probeSeries(t, report, "first"); !equalSeries(got, want) {
		t.Fatalf("first = %v, want %v", got, want)
	}
	if report.Interventions == nil || len(report.Interventions.Entries) != 1 {
		t.Fatalf("intervention report = %+v", report.Interventions)
	}
	if report.Interventions.Reason != "clamp the driven node" {
		t.Fatalf("intervention report reason = %q", report.Interventions.Reason)
	}
	entry := report.Interventions.Entries[0]
	if entry.Kind != InterventionKindClampVoltage || entry.Start != 1 || entry.End != 3 || entry.AppliedSteps != 2 {
		t.Fatalf("intervention entry = %+v", entry)
	}
	plain := protocol
	plain.Interventions = nil
	_, plainReport := mustRun(t, g, plain)
	free := probeSeries(t, plainReport, "first")
	if free[1] == want[1] || free[2] == want[2] {
		t.Fatal("a clamped step matches the free run")
	}
	if free[3] == want[3] {
		t.Fatal("the released step 3 collapsed onto the free run")
	}
}

// TestSilenceZeroesOutputAndDownstream silences node 1 of the LIF fixture on
// the rows [0, 2). Step 0 would have changed nothing, but at step 1 node 1
// would have spiked and its trace would have driven node 2. Silenced, node 1's
// trace and event stay zero, node 2 is never driven, and the only activity
// left in the whole graph is node 0's decaying trace.
func TestSilenceZeroesOutputAndDownstream(t *testing.T) {
	g := fixtureGraph(t)
	protocol := lifProtocol(4)
	protocol.Interventions = &InterventionPlan{
		Authorized: true, Reason: "silence the middle neuron",
		Items: []Intervention{{
			Kind: InterventionKindSilence, Targets: []int{1}, Start: 0, End: 2,
		}},
	}
	_, report := mustRun(t, g, protocol)
	k := math.Exp(-1)
	if got := probeSeries(t, report, "first"); !equalSeries(got, []float64{1, k, k * k, k * k * k}) {
		t.Fatalf("first = %v", got)
	}
	if got := probeSeries(t, report, "all"); !equalSeries(got, []float64{1, k, k * k, k * k * k}) {
		t.Fatalf("all = %v", got)
	}
	if got := probeSeries(t, report, "spikes"); !equalSeries(got, []float64{1. / 3, 0, 0, 0}) {
		t.Fatalf("spikes = %v", got)
	}
	if report.Interventions.Entries[0].AppliedSteps != 2 {
		t.Fatalf("applied steps = %d, want 2", report.Interventions.Entries[0].AppliedSteps)
	}
}

// TestForceSpikeOnLIF forces node 1 to fire at step 0 of a single-step run,
// where it would not have fired naturally. The forced event must reach the
// reported spike count, add one to the synaptic trace and reset the membrane
// and the refractory counter for the declared length.
func TestForceSpikeOnLIF(t *testing.T) {
	g := fixtureGraph(t)
	protocol := lifProtocol(1)
	protocol.LIF.RefractorySteps = 3
	protocol.Interventions = &InterventionPlan{
		Authorized: true, Reason: "fire the silent node",
		Items: []Intervention{{
			Kind: InterventionKindForceSpike, Targets: []int{1}, Start: 0, End: 1,
		}},
	}
	runner, report := mustRun(t, g, protocol)
	if got := probeSeries(t, report, "counted"); !equalSeries(got, []float64{2}) {
		t.Fatalf("counted = %v", got)
	}
	if got := probeSeries(t, report, "spikes"); !equalSeries(got, []float64{2. / 3}) {
		t.Fatalf("spikes = %v", got)
	}
	state := runner.State().LIF
	if state.Voltage[1] != -.5 || state.Refractory[1] != 3 {
		t.Fatalf("node 1 state = voltage %v refractory %d, want the reset", state.Voltage[1], state.Refractory[1])
	}
	lastTrace := state.History[len(state.History)-1]
	if lastTrace[1] != 1 {
		t.Fatalf("node 1 trace = %v, want the forced event in the trace", lastTrace[1])
	}
}

// TestClampOnLIFHoldsTheMembraneVoltage pins the semantics of a clamp on the
// spiking core: node 0 spikes at step 0 and would be reset by the event, but
// the clamp forces the membrane to 0.1. The trace and the event keep the
// values the core computed; only the voltage is overridden.
func TestClampOnLIFHoldsTheMembraneVoltage(t *testing.T) {
	g := fixtureGraph(t)
	protocol := lifProtocol(1)
	protocol.Interventions = &InterventionPlan{
		Authorized: true, Reason: "override the reset",
		Items: []Intervention{{
			Kind: InterventionKindClampVoltage, Targets: []int{0}, Value: .1, Start: 0, End: 1,
		}},
	}
	runner, report := mustRun(t, g, protocol)
	if state := runner.State().LIF; state.Voltage[0] != .1 {
		t.Fatalf("node 0 voltage = %v, want the clamp to override the reset", state.Voltage[0])
	}
	if got := probeSeries(t, report, "first"); !equalSeries(got, []float64{1}) {
		t.Fatalf("first = %v", got)
	}
	if got := probeSeries(t, report, "counted"); !equalSeries(got, []float64{1}) {
		t.Fatalf("counted = %v", got)
	}
}

func TestForceSpikeRejectedOnContinuous(t *testing.T) {
	g := fixtureGraph(t)
	protocol := lifProtocol(4)
	protocol.Core = CoreContinuous
	protocol.LIF = nil
	protocol.Continuous = continuousFixtureConfig()
	protocol.Probes = []Probe{{Name: "all", Nodes: []int{0, 1, 2}, Reduce: ReduceSumOutput}}
	protocol.Interventions = &InterventionPlan{
		Authorized: true, Reason: "cannot fire the continuous core",
		Items: []Intervention{{
			Kind: InterventionKindForceSpike, Targets: []int{0}, Start: 0, End: 1,
		}},
	}
	params := mustParameters(t, g, *protocol.Uniform)
	if _, err := Build(context.Background(), g, params, protocol, testLimits()); err == nil || !strings.Contains(err.Error(), "spiking core") {
		t.Fatalf("Build() = %v, want a refusal naming a spiking core", err)
	}
}

func TestUnauthorizedPlanRejected(t *testing.T) {
	for name, plan := range map[string]*InterventionPlan{
		"not authorized": {Authorized: false, Reason: "experiment", Items: []Intervention{{Kind: InterventionKindClampVoltage, Targets: []int{0}, Value: .5, Start: 0, End: 1}}},
		"blank reason":   {Authorized: true, Reason: "   ", Items: []Intervention{{Kind: InterventionKindClampVoltage, Targets: []int{0}, Value: .5, Start: 0, End: 1}}},
	} {
		protocol := lifProtocol(4)
		protocol.Interventions = plan
		if err := protocol.Validate(); err == nil || !strings.Contains(err.Error(), "authorized") {
			t.Fatalf("%s: Validate() = %v", name, err)
		}
	}
}

func TestUnsupportedKindRejected(t *testing.T) {
	protocol := lifProtocol(4)
	protocol.Interventions = &InterventionPlan{
		Authorized: true, Reason: "the native runner has no channels",
		Items: []Intervention{{
			Kind: "block_channel", Channel: 0, Start: 0, End: 1,
		}},
	}
	if err := protocol.Validate(); err == nil || !strings.Contains(err.Error(), "not supported on the native runner") {
		t.Fatalf("Validate() = %v, want the native runner refusal", err)
	}
}

func TestInterventionTargetsOutOfRangeRejected(t *testing.T) {
	g := fixtureGraph(t)
	protocol := lifProtocol(4)
	protocol.Interventions = &InterventionPlan{
		Authorized: true, Reason: "outside the graph",
		Items: []Intervention{{
			Kind: InterventionKindSilence, Targets: []int{3}, Start: 0, End: 1,
		}},
	}
	params := mustParameters(t, g, *protocol.Uniform)
	if _, err := Build(context.Background(), g, params, protocol, testLimits()); err == nil || !strings.Contains(err.Error(), "outside [0,") {
		t.Fatalf("Build() = %v, want an out-of-range target refusal", err)
	}
}

func TestInterventionConflictsRejected(t *testing.T) {
	g := fixtureGraph(t)
	protocol := lifProtocol(4)
	protocol.Interventions = &InterventionPlan{
		Authorized: true, Reason: "overwrite the same target",
		Items: []Intervention{
			{Kind: InterventionKindClampVoltage, Targets: []int{0}, Value: .25, Start: 0, End: 3},
			{Kind: InterventionKindClampVoltage, Targets: []int{0}, Value: .75, Start: 2, End: 4},
		},
	}
	params := mustParameters(t, g, *protocol.Uniform)
	build := func(p Protocol) error {
		_, err := Build(context.Background(), g, params, p, testLimits())
		return err
	}
	if err := build(protocol); err == nil || !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("Build() = %v, want an overlap refusal", err)
	}
	// The same two clamps on adjacent rows do not overlap and must build.
	ok := protocol
	ok.Interventions.Items[1].Start = 3
	if err := build(ok); err != nil {
		t.Fatalf("non-overlapping items refused: %v", err)
	}
}

func TestInterventionTargetsMustBeStrictlyIncreasing(t *testing.T) {
	for name, targets := range map[string][]int{
		"duplicate":  {1, 1},
		"descending": {2, 1},
	} {
		protocol := lifProtocol(4)
		protocol.Interventions = &InterventionPlan{
			Authorized: true, Reason: "bad indices",
			Items: []Intervention{{
				Kind: InterventionKindClampVoltage, Targets: targets, Start: 0, End: 1,
			}},
		}
		if err := protocol.Validate(); err == nil || !strings.Contains(err.Error(), "strictly increasing") {
			t.Fatalf("%s: Validate() = %v", name, err)
		}
	}
}

func TestSilenceNeedsAZeroActivation(t *testing.T) {
	g := fixtureGraph(t)
	protocol := lifProtocol(4)
	protocol.Core = CoreContinuous
	protocol.LIF = nil
	protocol.Continuous = &dynamics.Config{DT: 1, Activation: "softplus"}
	protocol.Probes = []Probe{{Name: "all", Nodes: []int{0, 1, 2}, Reduce: ReduceSumOutput}}
	protocol.Interventions = &InterventionPlan{
		Authorized: true, Reason: "softplus has no zero",
		Items: []Intervention{{
			Kind: InterventionKindSilence, Targets: []int{0}, Start: 0, End: 1,
		}},
	}
	params := mustParameters(t, g, *protocol.Uniform)
	if _, err := Build(context.Background(), g, params, protocol, testLimits()); err == nil || !strings.Contains(err.Error(), "silence needs an activation with a zero") {
		t.Fatalf("Build() = %v, want the silence zero activation refusal", err)
	}
}
