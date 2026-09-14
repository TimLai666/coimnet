package simulate

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/dynamics"
)

func testLimits() Limits { return Limits{MaxMemoryBytes: 1 << 30} }

func mustParameters(t *testing.T, g *connectome.Graph, u UniformParameters) ParameterSet {
	t.Helper()
	params, err := UniformPositive(context.Background(), g, u)
	if err != nil {
		t.Fatalf("uniform parameters: %v", err)
	}
	return params
}

func mustRun(t *testing.T, g *connectome.Graph, protocol Protocol) (*Runner, RunReport) {
	t.Helper()
	params := mustParameters(t, g, *protocol.Uniform)
	runner, err := Build(context.Background(), g, params, protocol, testLimits())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	report, err := runner.Run(context.Background(), runner.Stimulus())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return runner, report
}

func probeSeries(t *testing.T, report RunReport, name string) []float64 {
	t.Helper()
	for _, probe := range report.Probes {
		if probe.Name == name {
			return probe.Series
		}
	}
	t.Fatalf("report has no probe %q", name)
	return nil
}

func equalSeries(got, want []float64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestLIFProbeSeriesMatchTheHandCalculatedFixture derives every value from the
// step rule by hand. With dt=1 and log_tau=0 the membrane coefficients are
// lambda=exp(-1) and alpha=1-exp(-1); theta_raw=0 puts the base threshold at
// the middle of [0.5, 1.5], so theta_base=1. Gain 2 turns the raw weights 2 and
// 3 into 4 and 6. The single two-unit pulse into node 0 at step 0 gives:
//
//	step 0: cand[0]=alpha*2=1.264>=1, node 0 spikes; x0=1
//	step 1: drive[1]=4*1=4, cand[1]=alpha*4=2.528>=1, node 1 spikes; x0=k
//	step 2: drive[2]=6*1=6, cand[2]=alpha*6=3.793>=1, node 2 spikes
//	step 3: drive[2]=6*k=2.207, cand[2]=lambda*(-0.5)+alpha*2.207=1.211>=1, spikes
//
// with k=exp(-1) the synaptic decay. No other neuron reaches the threshold, so
// the synaptic traces are exactly the repeated products below.
func TestLIFProbeSeriesMatchTheHandCalculatedFixture(t *testing.T) {
	g := fixtureGraph(t)
	_, report := mustRun(t, g, lifProtocol(4))

	k := math.Exp(-1)
	x0 := []float64{1, k, k * k, k * (k * k)}
	x1 := []float64{0, 1, k, k * k}
	x2 := []float64{0, 0, 1, k*1 + 1}
	if report.Steps != 4 || report.Nodes != 3 || report.Edges != 2 {
		t.Fatalf("report shape = %d steps, %d nodes, %d edges", report.Steps, report.Nodes, report.Edges)
	}
	if got := probeSeries(t, report, "first"); !equalSeries(got, x0) {
		t.Fatalf("mean_output of node 0 = %v, want %v", got, x0)
	}
	sum := make([]float64, 4)
	for i := range sum {
		sum[i] = x0[i] + x1[i] + x2[i]
	}
	if got := probeSeries(t, report, "all"); !equalSeries(got, sum) {
		t.Fatalf("sum_output = %v, want %v", got, sum)
	}
	wantFraction := []float64{1. / 3, 1. / 3, 1. / 3, 1. / 3}
	if got := probeSeries(t, report, "spikes"); !equalSeries(got, wantFraction) {
		t.Fatalf("spike_fraction = %v, want %v", got, wantFraction)
	}
	wantCount := []float64{1, 1, 1, 1}
	if got := probeSeries(t, report, "counted"); !equalSeries(got, wantCount) {
		t.Fatalf("spike_count = %v, want %v", got, wantCount)
	}
	if !equalSeries(report.Monitors.PopulationRatePerStep, wantFraction) {
		t.Fatalf("population rate = %v", report.Monitors.PopulationRatePerStep)
	}
	if report.Monitors.SilentFraction != 0 || report.Monitors.NonFinite {
		t.Fatalf("monitors = %+v", report.Monitors)
	}
	// Per-node spike rates over four steps are 0.25, 0.25 and 0.5.
	wantQuantiles := [5]float64{.25, .25, .25, .375, .5}
	if report.Monitors.RateQuantiles != wantQuantiles {
		t.Fatalf("rate quantiles = %v, want %v", report.Monitors.RateQuantiles, wantQuantiles)
	}
	if report.SchemaVersion != RunReportSchemaVersion || report.Core != CoreLIF {
		t.Fatalf("report identity = %q %q", report.SchemaVersion, report.Core)
	}
	if len(report.CoreConfigHash) != 64 || len(report.ParameterHash) != 64 || len(report.ProtocolHash) != 64 {
		t.Fatalf("hashes = %q %q %q", report.CoreConfigHash, report.ParameterHash, report.ProtocolHash)
	}
	graphHashes := g.Report().Hashes
	if report.GraphHashes.NodeIndex != graphHashes.NodeIndex || report.GraphHashes.EdgeOrder != graphHashes.EdgeOrder {
		t.Fatalf("graph hashes = %+v", report.GraphHashes)
	}
	if report.ParameterSource != ParameterSourceUniform {
		t.Fatalf("parameter source = %q", report.ParameterSource)
	}
	assumptions := strings.Join(report.Assumptions, " ")
	if !strings.Contains(assumptions, ParameterSourceUniform) || !strings.Contains(assumptions, "not a biological parameter set") {
		t.Fatalf("assumptions do not state the engineering assumption: %q", assumptions)
	}
	if len(report.StabilityFlags) != 0 {
		t.Fatalf("unexpected stability flags %v", report.StabilityFlags)
	}
}

func TestContinuousCoreRunsTheSameProtocolShape(t *testing.T) {
	g := fixtureGraph(t)
	protocol := lifProtocol(4)
	protocol.Core = CoreContinuous
	protocol.LIF = nil
	protocol.Continuous = &dynamics.Config{DT: 1, Activation: "tanh"}
	protocol.Probes = []Probe{
		{Name: "first", Nodes: []int{0}, Reduce: ReduceMeanOutput},
		{Name: "all", Nodes: []int{0, 1, 2}, Reduce: ReduceSumOutput},
	}
	_, report := mustRun(t, g, protocol)
	alpha := -math.Expm1(-1.0)
	want := math.Tanh(alpha * 2)
	series := probeSeries(t, report, "first")
	if len(series) != 4 || series[0] != want {
		t.Fatalf("continuous mean_output = %v, want first value %v", series, want)
	}
	if report.Core != CoreContinuous {
		t.Fatalf("core = %q", report.Core)
	}
	// The continuous core emits no events, so the spiking monitors stay empty
	// instead of reporting an invented rate.
	if len(report.Monitors.PopulationRatePerStep) != 0 || report.Monitors.RateQuantiles != [5]float64{} {
		t.Fatalf("continuous monitors = %+v", report.Monitors)
	}
	// Every node's output moves away from the zero prehistory within four steps.
	if report.Monitors.SilentFraction != 0 {
		t.Fatalf("silent fraction = %v", report.Monitors.SilentFraction)
	}

	short := protocol
	short.Stimulus = StimulusSpec{Pulse: &Pulse{Channels: 1, Steps: 1, Channel: 0, Onset: 0, Duration: 1, Amplitude: 2}}
	_, shortReport := mustRun(t, g, short)
	if shortReport.Monitors.SilentFraction != 2.0/3.0 {
		t.Fatalf("one-step silent fraction = %v, want 2/3", shortReport.Monitors.SilentFraction)
	}

	spiking := protocol
	spiking.Probes = []Probe{{Name: "spikes", Nodes: []int{0}, Reduce: ReduceSpikeFraction}}
	params := mustParameters(t, g, *spiking.Uniform)
	if _, err := Build(context.Background(), g, params, spiking, testLimits()); err == nil {
		t.Fatal("continuous core accepted a spike reduction")
	}
}

func TestRunsAreDeterministicAndSplitRunsContinueTheState(t *testing.T) {
	g := fixtureGraph(t)
	protocol := lifProtocol(4)
	_, first := mustRun(t, g, protocol)
	_, second := mustRun(t, g, protocol)
	encodedFirst, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	encodedSecond, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(encodedFirst) != string(encodedSecond) {
		t.Fatalf("two identical runs produced different JSON:\n%s\n%s", encodedFirst, encodedSecond)
	}

	params := mustParameters(t, g, *protocol.Uniform)
	runner, err := Build(context.Background(), g, params, protocol, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	stimulus := runner.Stimulus()
	partA, err := runner.Run(context.Background(), stimulus[:2])
	if err != nil {
		t.Fatal(err)
	}
	snapshot := runner.State()
	encodedState, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Build(context.Background(), g, params, protocol, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	var decoded StateSnapshot
	if err := json.Unmarshal(encodedState, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := restored.RestoreState(decoded); err != nil {
		t.Fatalf("restore state: %v", err)
	}
	partB, err := restored.Run(context.Background(), stimulus[2:])
	if err != nil {
		t.Fatal(err)
	}
	if partA.StepsBefore != 0 || partA.StepsAfter != 2 || partB.StepsBefore != 2 || partB.StepsAfter != 4 {
		t.Fatalf("step counters = %d..%d then %d..%d", partA.StepsBefore, partA.StepsAfter, partB.StepsBefore, partB.StepsAfter)
	}
	for _, name := range []string{"first", "all", "spikes", "counted"} {
		joined := append(append([]float64(nil), probeSeries(t, partA, name)...), probeSeries(t, partB, name)...)
		if want := probeSeries(t, first, name); !equalSeries(joined, want) {
			t.Fatalf("probe %q split run = %v, single run = %v", name, joined, want)
		}
	}
	joinedRate := append(append([]float64(nil), partA.Monitors.PopulationRatePerStep...), partB.Monitors.PopulationRatePerStep...)
	if !equalSeries(joinedRate, first.Monitors.PopulationRatePerStep) {
		t.Fatalf("split population rate = %v", joinedRate)
	}
}

func TestStateSnapshotIsValidatedAgainstTheCore(t *testing.T) {
	g := fixtureGraph(t)
	protocol := lifProtocol(4)
	params := mustParameters(t, g, *protocol.Uniform)
	runner, err := Build(context.Background(), g, params, protocol, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	snapshot := runner.State()
	if snapshot.Core != CoreLIF || snapshot.LIF == nil || snapshot.Continuous != nil {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if err := runner.RestoreState(snapshot); err != nil {
		t.Fatalf("restoring its own state failed: %v", err)
	}
	wrongCore := snapshot
	wrongCore.Core = CoreContinuous
	if err := runner.RestoreState(wrongCore); err == nil {
		t.Fatal("restored a snapshot of another core")
	}
	both := snapshot
	both.Continuous = &dynamics.State{}
	if err := runner.RestoreState(both); err == nil {
		t.Fatal("restored a snapshot carrying two cores")
	}
	empty := StateSnapshot{Core: CoreLIF}
	if err := runner.RestoreState(empty); err == nil {
		t.Fatal("restored a snapshot without a payload")
	}
	mismatched := snapshot
	changed := *snapshot.LIF
	changed.ConfigHash = strings.Repeat("0", 64)
	mismatched.LIF = &changed
	if err := runner.RestoreState(mismatched); err == nil {
		t.Fatal("restored a snapshot with a foreign configuration hash")
	}
	corrupted := snapshot
	broken := *snapshot.LIF
	broken.Voltage = []float64{math.Inf(1), 0, 0}
	corrupted.LIF = &broken
	if err := runner.RestoreState(corrupted); err == nil {
		t.Fatal("restored a non-finite state")
	}
	// A rejected restore must leave the runner able to continue from its own state.
	if _, err := runner.Run(context.Background(), runner.Stimulus()); err != nil {
		t.Fatalf("run after rejected restores: %v", err)
	}
}

func TestSelectorsResolveAnnotationsAndAreReported(t *testing.T) {
	g := fixtureGraph(t)
	protocol := lifProtocol(4)
	protocol.Injections = nil
	protocol.InjectionGroups = []InjectionGroup{{Channel: 0, Selector: &Selector{Field: "class", Equals: "ALIN"}, Gain: 1}}
	protocol.Probes = []Probe{
		{Name: "descending", Selector: &Selector{Field: "superclass", Equals: "descending_neuron"}, Reduce: ReduceSpikeFraction},
		{Name: "left", Selector: &Selector{Field: "soma_side", Equals: "L"}, Reduce: ReduceSpikeCount},
	}
	_, report := mustRun(t, g, protocol)
	if len(report.Injections) != 1 {
		t.Fatalf("injections = %+v", report.Injections)
	}
	injected := report.Injections[0]
	if injected.NodeCount != 1 || injected.FirstNode != 0 || injected.LastNode != 0 || injected.Gain != 1 || injected.Channel != 0 {
		t.Fatalf("resolved injection = %+v", injected)
	}
	if injected.Selector == nil || injected.Selector.Field != "class" || injected.Selector.Equals != "ALIN" || injected.Selector.Count != 1 {
		t.Fatalf("injection selector = %+v", injected.Selector)
	}
	descending := report.Probes[0]
	if descending.NodeCount != 2 || descending.Selector == nil || descending.Selector.FirstIndex != 1 || descending.Selector.LastIndex != 2 {
		t.Fatalf("descending probe = %+v, selector %+v", descending, descending.Selector)
	}
	// Nodes 1 and 2 spike once each, at steps 1 and 2, and node 2 again at step 3.
	if want := []float64{0, .5, .5, .5}; !equalSeries(descending.Series, want) {
		t.Fatalf("descending spike_fraction = %v, want %v", descending.Series, want)
	}
	if want := []float64{1, 0, 1, 1}; !equalSeries(report.Probes[1].Series, want) {
		t.Fatalf("left soma side spike_count = %v, want %v", report.Probes[1].Series, want)
	}

	params := mustParameters(t, g, *protocol.Uniform)
	for name, selector := range map[string]Selector{
		"unknown field": {Field: "hemilineage", Equals: "x"},
		"no match":      {Field: "class", Equals: "MBON"},
		"empty field":   {Field: "", Equals: "ALIN"},
	} {
		bad := protocol
		bad.Probes = []Probe{{Name: "bad", Selector: &selector, Reduce: ReduceSpikeFraction}}
		if _, err := Build(context.Background(), g, params, bad, testLimits()); err == nil {
			t.Fatalf("%s selector accepted", name)
		}
	}
	allowEmpty := protocol
	allowEmpty.Probes = []Probe{{Name: "empty", Selector: &Selector{Field: "class", Equals: "MBON", AllowEmpty: true}, Reduce: ReduceSpikeFraction}}
	_, emptyReport := mustRun(t, g, allowEmpty)
	if emptyReport.Probes[0].NodeCount != 0 || emptyReport.Probes[0].Selector.FirstIndex != -1 {
		t.Fatalf("allow_empty probe = %+v", emptyReport.Probes[0])
	}
	if want := []float64{0, 0, 0, 0}; !equalSeries(emptyReport.Probes[0].Series, want) {
		t.Fatalf("allow_empty series = %v", emptyReport.Probes[0].Series)
	}
}

func TestThresholdsRaiseStabilityFlagsWithoutChangingParameters(t *testing.T) {
	g := fixtureGraph(t)
	protocol := lifProtocol(4)
	protocol.Thresholds = Thresholds{MaxPopulationRate: .2, MinActiveFraction: .9}
	_, report := mustRun(t, g, protocol)
	flags := strings.Join(report.StabilityFlags, ",")
	if !strings.Contains(flags, "max_population_rate_exceeded") {
		t.Fatalf("flags = %v", report.StabilityFlags)
	}
	// Every node spikes at least once, so the active fraction threshold holds.
	if strings.Contains(flags, "min_active_fraction") {
		t.Fatalf("unexpected active fraction flag: %v", report.StabilityFlags)
	}
	calm := lifProtocol(4)
	calm.Thresholds = Thresholds{MaxPopulationRate: 1, MinActiveFraction: .9}
	calm.Injections = nil
	calm.InjectionGroups = []InjectionGroup{{Channel: 0, Selector: &Selector{Field: "class", Equals: "ALIN"}, Gain: 0}}
	_, calmReport := mustRun(t, g, calm)
	if calmReport.Monitors.SilentFraction != 1 {
		t.Fatalf("silent fraction without drive = %v", calmReport.Monitors.SilentFraction)
	}
	if !strings.Contains(strings.Join(calmReport.StabilityFlags, ","), "min_active_fraction_below_threshold") {
		t.Fatalf("flags = %v", calmReport.StabilityFlags)
	}
}

func TestBuildRefusesOversizedGraphsInsteadOfDownsizing(t *testing.T) {
	g := fixtureGraph(t)
	protocol := lifProtocol(4)
	params := mustParameters(t, g, *protocol.Uniform)
	_, err := Build(context.Background(), g, params, protocol, Limits{MaxMemoryBytes: 64})
	if err == nil {
		t.Fatal("tiny memory limit accepted")
	}
	if !errors.Is(err, ErrCapacity) {
		t.Fatalf("capacity error = %v", err)
	}
	if _, err := Build(context.Background(), g, params, protocol, Limits{MaxMemoryBytes: 0}); err == nil {
		t.Fatal("zero memory limit accepted")
	}
}

func TestNonFiniteAndCancellationLeaveNoState(t *testing.T) {
	g := fixtureGraph(t)
	protocol := lifProtocol(4)
	protocol.Injections = []Injection{{Channel: 0, Node: 0, Gain: math.MaxFloat64}}
	params := mustParameters(t, g, *protocol.Uniform)
	runner, err := Build(context.Background(), g, params, protocol, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	before := runner.State()
	if _, err := runner.Run(context.Background(), runner.Stimulus()); err == nil {
		t.Fatal("non-finite injection accepted")
	} else if !strings.Contains(err.Error(), "non-finite") {
		t.Fatalf("non-finite error = %v", err)
	}
	after := runner.State()
	if after.LIF.Steps != before.LIF.Steps {
		t.Fatalf("failed run advanced the state from %d to %d", before.LIF.Steps, after.LIF.Steps)
	}

	good := lifProtocol(4)
	goodParams := mustParameters(t, g, *good.Uniform)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Build(canceled, g, goodParams, good, testLimits()); !errors.Is(err, context.Canceled) {
		t.Fatalf("build with a canceled context = %v", err)
	}
	if _, err := UniformPositive(canceled, g, *good.Uniform); !errors.Is(err, context.Canceled) {
		t.Fatalf("parameters with a canceled context = %v", err)
	}
	runner2, err := Build(context.Background(), g, goodParams, good, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner2.Run(canceled, runner2.Stimulus()); !errors.Is(err, context.Canceled) {
		t.Fatalf("run with a canceled context = %v", err)
	}
	if runner2.State().LIF.Steps != 0 {
		t.Fatal("canceled run advanced the state")
	}
}

func TestUniformPositiveParametersAreDerivedFromRawWeights(t *testing.T) {
	g := fixtureGraph(t)
	params := mustParameters(t, g, UniformParameters{Gain: 2, Bias: .5, LogTau: -1, ThetaRaw: .25})
	if params.Source != ParameterSourceUniform {
		t.Fatalf("source = %q", params.Source)
	}
	if want := []float64{4, 6}; !equalSeries(params.Weights, want) {
		t.Fatalf("weights = %v, want %v", params.Weights, want)
	}
	for _, group := range [][2]any{{params.Bias, .5}, {params.LogTau, -1.0}, {params.ThetaRaw, .25}} {
		values := group[0].([]float64)
		if len(values) != 3 {
			t.Fatalf("group length %d", len(values))
		}
		for _, v := range values {
			if v != group[1].(float64) {
				t.Fatalf("group value %v, want %v", v, group[1])
			}
		}
	}
	if len(params.Hash) != 64 {
		t.Fatalf("hash = %q", params.Hash)
	}
	same := mustParameters(t, g, UniformParameters{Gain: 2, Bias: .5, LogTau: -1, ThetaRaw: .25})
	if same.Hash != params.Hash {
		t.Fatal("identical inputs produced different parameter hashes")
	}
	other := mustParameters(t, g, UniformParameters{Gain: 3, Bias: .5, LogTau: -1, ThetaRaw: .25})
	if other.Hash == params.Hash {
		t.Fatal("a different gain kept the parameter hash")
	}
	for name, u := range map[string]UniformParameters{
		"infinite gain": {Gain: math.Inf(1)},
		"nan bias":      {Gain: 1, Bias: math.NaN()},
		"huge gain":     {Gain: math.MaxFloat64, Bias: 0},
	} {
		if _, err := UniformPositive(context.Background(), g, u); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}

func TestProtocolJSONIsStrict(t *testing.T) {
	protocol := lifProtocol(4)
	encoded, err := json.Marshal(protocol)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeProtocol(strings.NewReader(string(encoded)))
	if err != nil {
		t.Fatalf("decode protocol: %v", err)
	}
	if decoded.Core != CoreLIF || decoded.Uniform == nil || decoded.Uniform.Gain != 2 || len(decoded.Probes) != 4 {
		t.Fatalf("decoded = %+v", decoded)
	}
	hashA, err := protocol.Hash()
	if err != nil {
		t.Fatal(err)
	}
	hashB, err := decoded.Hash()
	if err != nil || hashA != hashB || len(hashA) != 64 {
		t.Fatalf("protocol hashes %q %q, err=%v", hashA, hashB, err)
	}

	var generic map[string]any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		t.Fatal(err)
	}
	generic["unexpected"] = true
	withUnknown, _ := json.Marshal(generic)
	for name, input := range map[string]string{
		"unknown field": string(withUnknown),
		"trailing data": string(encoded) + " {}",
		"duplicate key": `{"schema_version":"a","Schema_Version":"b"}`,
		"too deep":      strings.Repeat(`{"k":`, 100) + "1" + strings.Repeat("}", 100),
		"empty":         ``,
	} {
		if _, err := DecodeProtocol(strings.NewReader(input)); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if _, err := DecodeProtocol(nil); err == nil {
		t.Fatal("nil reader accepted")
	}
}

func TestProtocolValidationRejectsUnsupportedConfigurations(t *testing.T) {
	g := fixtureGraph(t)
	base := lifProtocol(4)
	params := mustParameters(t, g, *base.Uniform)
	topology := base
	withTopology := *base.LIF
	withTopology.Nodes = 3
	withTopology.Sources = []int{0}
	withTopology.Targets = []int{1}
	topology.LIF = &withTopology

	bothCores := base
	bothCores.Continuous = &dynamics.Config{DT: 1, Activation: "tanh"}

	noCore := base
	noCore.LIF = nil

	badSource := base
	badSource.ParameterSource = "derived_from_release"

	noUniform := base
	noUniform.Uniform = nil

	bothStimuli := base
	bothStimuli.Stimulus = StimulusSpec{Inline: [][]float64{{1}}, Pulse: base.Stimulus.Pulse}

	noStimulus := base
	noStimulus.Stimulus = StimulusSpec{}

	badPulse := base
	badPulse.Stimulus = StimulusSpec{Pulse: &Pulse{Channels: 1, Steps: 4, Channel: 0, Onset: 3, Duration: 5, Amplitude: 1}}

	badChannel := base
	badChannel.Injections = []Injection{{Channel: 3, Node: 0, Gain: 1}}

	badNode := base
	badNode.Injections = []Injection{{Channel: 0, Node: 9, Gain: 1}}

	badReduce := base
	badReduce.Probes = []Probe{{Name: "x", Nodes: []int{0}, Reduce: "median_output"}}

	noProbes := base
	noProbes.Probes = nil

	duplicateProbe := base
	duplicateProbe.Probes = []Probe{{Name: "x", Nodes: []int{0}, Reduce: ReduceMeanOutput}, {Name: "x", Nodes: []int{1}, Reduce: ReduceMeanOutput}}

	badSchema := base
	badSchema.SchemaVersion = "coimnet-simulate-protocol/v2"

	bothProbeSets := base
	bothProbeSets.Probes = []Probe{{Name: "x", Nodes: []int{0}, Selector: &Selector{Field: "class", Equals: "ALIN"}, Reduce: ReduceMeanOutput}}

	for name, bad := range map[string]Protocol{
		"topology in protocol": topology,
		"two cores":            bothCores,
		"no core config":       noCore,
		"foreign source":       badSource,
		"missing uniform":      noUniform,
		"two stimuli":          bothStimuli,
		"no stimulus":          noStimulus,
		"pulse past the end":   badPulse,
		"channel out of range": badChannel,
		"node out of range":    badNode,
		"unknown reduce":       badReduce,
		"no probes":            noProbes,
		"duplicate probe name": duplicateProbe,
		"unknown schema":       badSchema,
		"nodes and a selector": bothProbeSets,
	} {
		if _, err := Build(context.Background(), g, params, bad, testLimits()); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if _, err := Build(context.Background(), nil, params, base, testLimits()); err == nil {
		t.Fatal("nil graph accepted")
	}
	// The refusal must name both sources that do exist, so the message tells
	// the reader what to write instead of what is missing.
	_, err := Build(context.Background(), g, params, badSource, testLimits())
	if err == nil || !strings.Contains(err.Error(), ParameterSourceUniform) || !strings.Contains(err.Error(), ParameterSourceDerived) {
		t.Fatalf("the foreign parameter source error must name both available sources: %v", err)
	}
}

func TestParameterSetMustMatchTheGraph(t *testing.T) {
	g := fixtureGraph(t)
	protocol := lifProtocol(4)
	params := mustParameters(t, g, *protocol.Uniform)
	short := params
	short.Weights = params.Weights[:1]
	if _, err := Build(context.Background(), g, short, protocol, testLimits()); err == nil {
		t.Fatal("a parameter set with the wrong edge count was accepted")
	}
	wrongNodes := params
	wrongNodes.Bias = []float64{0, 0}
	if _, err := Build(context.Background(), g, wrongNodes, protocol, testLimits()); err == nil {
		t.Fatal("a parameter set with the wrong node count was accepted")
	}
	foreign := params
	foreign.Source = "derived_from_release"
	if _, err := Build(context.Background(), g, foreign, protocol, testLimits()); err == nil {
		t.Fatal("a parameter set whose source disagrees with the protocol was accepted")
	}
}

func TestRunValidatesTheStimulusItIsGiven(t *testing.T) {
	g := fixtureGraph(t)
	protocol := lifProtocol(4)
	params := mustParameters(t, g, *protocol.Uniform)
	runner, err := Build(context.Background(), g, params, protocol, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	for name, stimulus := range map[string][][]float64{
		"empty":       {},
		"wrong width": {{1, 2}},
		"non-finite":  {{math.NaN()}},
		"ragged":      {{1}, {}},
	} {
		if _, err := runner.Run(context.Background(), stimulus); err == nil {
			t.Fatalf("%s stimulus accepted", name)
		}
	}
	if runner.State().LIF.Steps != 0 {
		t.Fatal("a rejected stimulus advanced the state")
	}
	inline := protocol
	inline.Stimulus = StimulusSpec{Inline: [][]float64{{2}, {0}, {0}, {0}}}
	_, inlineReport := mustRun(t, g, inline)
	_, pulseReport := mustRun(t, g, protocol)
	if !equalSeries(probeSeries(t, inlineReport, "all"), probeSeries(t, pulseReport, "all")) {
		t.Fatal("an inline stimulus and the equivalent pulse produced different series")
	}
}

// TestChunkBoundariesChangeNothing exercises the path the whole-brain run takes:
// dynamics bounds one Advance output matrix to MaxStateValues elements, so a
// large graph advances in chunks. The chunk size must not change any value.
func TestChunkBoundariesChangeNothing(t *testing.T) {
	g := fixtureGraph(t)
	protocol := lifProtocol(6)
	params := mustParameters(t, g, *protocol.Uniform)
	whole, err := Build(context.Background(), g, params, protocol, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if whole.maxChunk < 6 {
		t.Fatalf("the fixture graph already chunks at %d steps", whole.maxChunk)
	}
	wholeReport, err := whole.Run(context.Background(), whole.Stimulus())
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range []int{1, 2, 4, 5} {
		chunked, err := Build(context.Background(), g, params, protocol, testLimits())
		if err != nil {
			t.Fatal(err)
		}
		chunked.maxChunk = chunk
		report, err := chunked.Run(context.Background(), chunked.Stimulus())
		if err != nil {
			t.Fatalf("chunk %d: %v", chunk, err)
		}
		for _, name := range []string{"first", "all", "spikes", "counted"} {
			if got, want := probeSeries(t, report, name), probeSeries(t, wholeReport, name); !equalSeries(got, want) {
				t.Fatalf("chunk %d probe %q = %v, want %v", chunk, name, got, want)
			}
		}
		if !equalSeries(report.Monitors.PopulationRatePerStep, wholeReport.Monitors.PopulationRatePerStep) {
			t.Fatalf("chunk %d population rate = %v", chunk, report.Monitors.PopulationRatePerStep)
		}
		if report.Monitors.RateQuantiles != wholeReport.Monitors.RateQuantiles || report.StepsAfter != wholeReport.StepsAfter {
			t.Fatalf("chunk %d monitors = %+v", chunk, report.Monitors)
		}
	}
}

func TestDecodeStateIsStrictAboutTheUnion(t *testing.T) {
	g := fixtureGraph(t)
	protocol := lifProtocol(4)
	params := mustParameters(t, g, *protocol.Uniform)
	runner, err := Build(context.Background(), g, params, protocol, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Run(context.Background(), runner.Stimulus()); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(runner.State())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeState(strings.NewReader(string(encoded)))
	if err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if decoded.Core != CoreLIF || decoded.LIF == nil || decoded.LIF.Steps != 4 {
		t.Fatalf("decoded state = %+v", decoded)
	}
	fresh, err := Build(context.Background(), g, params, protocol, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := fresh.RestoreState(decoded); err != nil {
		t.Fatalf("restore decoded state: %v", err)
	}
	for name, input := range map[string]string{
		"no payload":    `{"schema_version":"` + StateSchemaVersion + `","core":"lif"}`,
		"two payloads":  `{"schema_version":"` + StateSchemaVersion + `","core":"lif","lif":{},"continuous":{}}`,
		"wrong schema":  `{"schema_version":"coimnet-simulate-state/v2","core":"lif","lif":{}}`,
		"unknown field": `{"schema_version":"` + StateSchemaVersion + `","core":"lif","lif":{},"extra":1}`,
		"duplicate key": `{"schema_version":"a","Schema_Version":"b"}`,
	} {
		if _, err := DecodeState(strings.NewReader(input)); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if _, err := DecodeState(nil); err == nil {
		t.Fatal("nil reader accepted")
	}
}
