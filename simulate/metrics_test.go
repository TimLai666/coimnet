package simulate

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
)

// The metric fixture is the hand-calculated LIF run of lifProtocol: the three
// neuron graph 0->1->2 driven by one two-unit pulse into node 0 at step 0
// spikes exactly four times, at
//
//	step 0: node 0
//	step 1: node 1
//	step 2: node 2
//	step 3: node 2
//
// (derived step by step in TestLIFProbeSeriesMatchTheHandCalculatedFixture).
// The fixture annotations make node 0 class ALIN soma_side L, node 1 class
// ALPN soma_side R and node 2 class ALPN soma_side L, so the named sets below
// resolve to {1,2}, {2} and {0}.
func metricSets() []NamedSet {
	return []NamedSet{
		{Name: "alpn", Selectors: []Selector{{Field: "class", Equals: "ALPN"}}},
		{Name: "left_alpn", Selectors: []Selector{{Field: "class", Equals: "ALPN"}, {Field: "soma_side", Equals: "L"}}},
		{Name: "alin", Selectors: []Selector{{Field: "class", Equals: "ALIN"}}},
	}
}

func resolveAll(t *testing.T, sets []NamedSet) []ResolvedSet {
	t.Helper()
	g := fixtureGraph(t)
	resolved, err := ResolveSets(context.Background(), g, sets)
	if err != nil {
		t.Fatalf("resolve sets: %v", err)
	}
	return resolved
}

func TestNamedSetsResolveAsTheIntersectionOfTheirSelectors(t *testing.T) {
	g := fixtureGraph(t)
	resolved, err := ResolveSets(context.Background(), g, metricSets())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want := map[string][]int{"alpn": {1, 2}, "left_alpn": {2}, "alin": {0}}
	for _, set := range resolved {
		nodes, known := want[set.Name]
		if !known {
			t.Fatalf("unexpected set %q", set.Name)
		}
		if set.Count != len(nodes) {
			t.Fatalf("set %q resolved %d nodes, want %d", set.Name, set.Count, len(nodes))
		}
		digest := sha256.New()
		buffer := make([]byte, 4)
		for _, node := range nodes {
			binary.LittleEndian.PutUint32(buffer, uint32(node))
			digest.Write(buffer)
		}
		if set.NodeHash != hex.EncodeToString(digest.Sum(nil)) {
			t.Fatalf("set %q node hash = %q", set.Name, set.NodeHash)
		}
		if len(set.Selectors) != len(set.Selectors) || set.Selectors[0].Field == "" {
			t.Fatalf("set %q kept no selector resolution: %+v", set.Name, set.Selectors)
		}
	}

	for name, sets := range map[string][]NamedSet{
		"no selector":        {{Name: "empty"}},
		"no name":            {{Selectors: []Selector{{Field: "class", Equals: "ALPN"}}}},
		"duplicate name":     {{Name: "a", Selectors: []Selector{{Field: "class", Equals: "ALPN"}}}, {Name: "a", Selectors: []Selector{{Field: "class", Equals: "ALIN"}}}},
		"unknown field":      {{Name: "a", Selectors: []Selector{{Field: "hemilineage", Equals: "x"}}}},
		"empty intersection": {{Name: "a", Selectors: []Selector{{Field: "class", Equals: "ALIN"}, {Field: "soma_side", Equals: "R"}}}},
		"missing annotation": {{Name: "a", Selectors: []Selector{{Field: "class", Equals: "nothing"}}}},
	} {
		if _, err := ResolveSets(context.Background(), g, sets); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	// An intersection that every selector allows to be empty is reported, not
	// refused: the caller declared that it accepts an empty set.
	allowed, err := ResolveSets(context.Background(), g, []NamedSet{{Name: "a", Selectors: []Selector{
		{Field: "class", Equals: "ALIN", AllowEmpty: true},
		{Field: "soma_side", Equals: "R", AllowEmpty: true},
	}}})
	if err != nil {
		t.Fatalf("declared empty intersection: %v", err)
	}
	if allowed[0].Count != 0 {
		t.Fatalf("declared empty intersection resolved %d nodes", allowed[0].Count)
	}
}

// metricFixtureResults runs the hand-calculated fixture with the metrics below
// and returns the results by name.
func metricFixtureResults(t *testing.T, metrics []Metric) map[string]MetricResult {
	t.Helper()
	g := fixtureGraph(t)
	protocol := lifProtocol(4)
	sets, err := ResolveSets(context.Background(), g, metricSets())
	if err != nil {
		t.Fatal(err)
	}
	parameters := mustParameters(t, g, *protocol.Uniform)
	runner, err := Build(context.Background(), g, parameters, protocol, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.TrackSets(sets, MetricWindows(metrics)); err != nil {
		t.Fatalf("track sets: %v", err)
	}
	if _, err := runner.Run(context.Background(), runner.Stimulus()); err != nil {
		t.Fatal(err)
	}
	results, err := EvaluateMetrics(metrics, runner.Measurements())
	if err != nil {
		t.Fatalf("evaluate metrics: %v", err)
	}
	byName := map[string]MetricResult{}
	for _, result := range results {
		byName[result.Name] = result
	}
	if len(byName) != len(metrics) {
		t.Fatalf("got %d results for %d metrics", len(byName), len(metrics))
	}
	return byName
}

func handCalculatedMetrics() []Metric {
	return []Metric{
		{Name: "alpn_fraction", Kind: MetricSpikeFraction, Set: "alpn", Window: [2]int{0, 4}},
		{Name: "alpn_rate", Kind: MetricMeanRate, Set: "alpn", Window: [2]int{0, 4}},
		{Name: "alpn_latency", Kind: MetricLatencyToFirstSpike, Set: "alpn", Window: [2]int{0, 4}},
		{Name: "alpn_latency_late", Kind: MetricLatencyToFirstSpike, Set: "alpn", Window: [2]int{2, 4}},
		{Name: "left_fraction", Kind: MetricSpikeFraction, Set: "left_alpn", Window: [2]int{2, 4}},
		{Name: "left_latency_quiet", Kind: MetricLatencyToFirstSpike, Set: "left_alpn", Window: [2]int{0, 2}},
		{Name: "left_ratio", Kind: MetricActivityRatioVsBaseline, Set: "left_alpn", Window: [2]int{2, 4}, Baseline: [2]int{0, 4}},
		{Name: "left_ratio_zero_baseline", Kind: MetricActivityRatioVsBaseline, Set: "left_alpn", Window: [2]int{2, 4}, Baseline: [2]int{0, 2}},
		{Name: "alin_rate", Kind: MetricMeanRate, Set: "alin", Window: [2]int{1, 4}},
	}
}

func TestMetricsMatchTheHandCalculatedRun(t *testing.T) {
	results := metricFixtureResults(t, handCalculatedMetrics())
	for _, want := range []struct {
		name        string
		defined     bool
		value       float64
		numerator   float64
		denominator float64
	}{
		// Both ALPN neurons spike inside [0,4): 2 of 2 nodes.
		{"alpn_fraction", true, 1, 2, 2},
		// Three spike events (node 1 once, node 2 twice) over 2 nodes x 4 steps.
		{"alpn_rate", true, 0.375, 3, 8},
		// The first ALPN spike is node 1 at step 1, and the window starts at 0.
		{"alpn_latency", true, 1, 1, 0},
		// Inside [2,4) the first ALPN spike is node 2 at step 2.
		{"alpn_latency_late", true, 0, 2, 2},
		// Node 2 alone spikes at steps 2 and 3.
		{"left_fraction", true, 1, 1, 1},
		// Node 2 is silent before step 2, so the latency has no value at all.
		{"left_latency_quiet", false, 0, 0, 0},
		// mean_rate over [2,4) is 2/(1*2) = 1 against 2/(1*4) = 0.5 over [0,4).
		{"left_ratio", true, 2, 1, 0.5},
		// The baseline [0,2) has no spike, so the ratio is undefined, not zero.
		{"left_ratio_zero_baseline", false, 0, 1, 0},
		// Node 0 spikes only at step 0, which [1,4) excludes.
		{"alin_rate", true, 0, 0, 3},
	} {
		got := results[want.name]
		if got.Defined != want.defined {
			t.Errorf("%s defined = %v, want %v (%+v)", want.name, got.Defined, want.defined, got)
			continue
		}
		if got.Value != want.value || got.Numerator != want.numerator || got.Denominator != want.denominator {
			t.Errorf("%s = %v (%v/%v), want %v (%v/%v)", want.name, got.Value, got.Numerator, got.Denominator,
				want.value, want.numerator, want.denominator)
		}
		if got.Basis == "" {
			t.Errorf("%s has no basis", want.name)
		}
	}
}

func TestThresholdsCompareEveryOperatorAndFailOnUndefined(t *testing.T) {
	results := metricFixtureResults(t, handCalculatedMetrics())
	ordered := make([]MetricResult, 0, len(results))
	for _, metric := range handCalculatedMetrics() {
		ordered = append(ordered, results[metric.Name])
	}
	thresholds := []Threshold{
		{Metric: "alpn_fraction", Op: OpAtLeast, Value: 1},
		{Metric: "alpn_rate", Op: OpAbove, Value: 0.375},
		{Metric: "alin_rate", Op: OpAtMost, Value: 0},
		{Metric: "alpn_rate", Op: OpBelow, Value: 0.4},
		{Metric: "left_latency_quiet", Op: OpAtLeast, Value: 0},
	}
	got, err := EvaluateThresholds(thresholds, ordered)
	if err != nil {
		t.Fatalf("evaluate thresholds: %v", err)
	}
	want := []struct {
		passed   bool
		observed float64
		reason   string
	}{
		{true, 1, ""},
		{false, 0.375, ""},
		{true, 0, ""},
		{true, 0.375, ""},
		{false, 0, ReasonUndefined},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d threshold results, want %d", len(got), len(want))
	}
	for i, expect := range want {
		if got[i].Passed != expect.passed || got[i].Observed != expect.observed || got[i].Reason != expect.reason {
			t.Errorf("threshold %d = %+v, want passed %v observed %v reason %q", i, got[i], expect.passed, expect.observed, expect.reason)
		}
		if got[i].Metric != thresholds[i].Metric || got[i].Op != thresholds[i].Op || got[i].Value != thresholds[i].Value {
			t.Errorf("threshold %d lost its declaration: %+v", i, got[i])
		}
	}
	if _, err := EvaluateThresholds([]Threshold{{Metric: "absent", Op: OpAtLeast, Value: 1}}, ordered); err == nil {
		t.Error("a threshold on an undeclared metric was accepted")
	}
	if _, err := EvaluateThresholds([]Threshold{{Metric: "alpn_rate", Op: "==", Value: 1}}, ordered); err == nil {
		t.Error("an unsupported operator was accepted")
	}
}

func TestMeanOutputIsTheOnlyMetricTheContinuousCoreAllows(t *testing.T) {
	g := fixtureGraph(t)
	protocol := lifProtocol(4)
	protocol.Core = CoreContinuous
	protocol.LIF = nil
	protocol.Continuous = &dynamics.Config{DT: 1, Activation: "tanh"}
	protocol.Probes = []Probe{{Name: "all", Nodes: []int{0, 1, 2}, Reduce: ReduceSumOutput}}
	sets, err := ResolveSets(context.Background(), g, metricSets())
	if err != nil {
		t.Fatal(err)
	}
	metrics := []Metric{{Name: "alpn_output", Kind: MetricMeanOutput, Set: "alpn", Window: [2]int{0, 4}}}
	parameters := mustParameters(t, g, *protocol.Uniform)
	runner, err := Build(context.Background(), g, parameters, protocol, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.TrackSets(sets, MetricWindows(metrics)); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Run(context.Background(), runner.Stimulus()); err != nil {
		t.Fatal(err)
	}
	results, err := EvaluateMetrics(metrics, runner.Measurements())
	if err != nil {
		t.Fatalf("mean_output on the continuous core: %v", err)
	}
	if !results[0].Defined || results[0].Denominator != 8 {
		t.Fatalf("mean_output = %+v; the denominator is 2 nodes x 4 steps", results[0])
	}
	// The spiking metrics have no events to read on this core.
	for _, kind := range []string{MetricSpikeFraction, MetricMeanRate, MetricLatencyToFirstSpike, MetricActivityRatioVsBaseline} {
		bad := []Metric{{Name: "x", Kind: kind, Set: "alpn", Window: [2]int{0, 4}, Baseline: [2]int{0, 2}}}
		if _, err := EvaluateMetrics(bad, runner.Measurements()); err == nil {
			t.Errorf("%s was evaluated on a core without events", kind)
		}
	}
}

func TestTrackSetsValidatesItsWindowsAndBudget(t *testing.T) {
	g := fixtureGraph(t)
	protocol := lifProtocol(4)
	sets, err := ResolveSets(context.Background(), g, metricSets())
	if err != nil {
		t.Fatal(err)
	}
	parameters := mustParameters(t, g, *protocol.Uniform)
	runner, err := Build(context.Background(), g, parameters, protocol, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	for name, windows := range map[string][][2]int{
		"empty window":    {{2, 2}},
		"reversed window": {{3, 1}},
		"negative start":  {{-1, 2}},
		"past the end":    {{0, 5}},
	} {
		if err := runner.TrackSets(sets, windows); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := runner.TrackSets(sets, [][2]int{{0, 4}}); err != nil {
		t.Fatalf("valid tracking refused: %v", err)
	}
	// Without a Run there is nothing to read back.
	if len(runner.Measurements().Windows) != 0 {
		t.Fatal("measurements exist before the run")
	}
}
