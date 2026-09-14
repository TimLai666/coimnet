package simulate

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"testing"
)

// compareFixtureProtocol runs the four node derivation fixture on the spiking
// core for six steps with two named sets. The fixture annotations make bodies
// 10 and 40 class ALIN (node indices 0 and 3) and bodies 20 and 30 class ALPN
// (node indices 1 and 2).
func compareFixtureProtocol(steps int) CompareProtocol {
	return CompareProtocol{
		SchemaVersion: CompareProtocolSchemaVersion,
		Run:           derivedProtocol(CoreLIF, UnknownSignExclude, fixtureWeightScale, steps),
		Sets: []NamedSet{
			{Name: "alin", Selectors: []Selector{{Field: "class", Equals: "ALIN"}}},
			{Name: "alpn", Selectors: []Selector{{Field: "class", Equals: "ALPN"}}},
		},
		Metrics: []Metric{
			{Name: "alin_fraction", Kind: MetricSpikeFraction, Set: "alin", Window: [2]int{0, steps}},
			{Name: "alin_rate", Kind: MetricMeanRate, Set: "alin", Window: [2]int{0, steps}},
			{Name: "alpn_rate", Kind: MetricMeanRate, Set: "alpn", Window: [2]int{0, steps}},
		},
		Thresholds: []Threshold{
			{Metric: "alin_fraction", Op: OpAtLeast, Value: 0.5},
			{Metric: "alpn_rate", Op: OpAbove, Value: 1},
		},
		NullModels: []NullModelSpec{
			{Kind: NullDegreePreservingRewire, SwapFactor: 2},
			{Kind: NullSignShuffle},
			{Kind: NullWeightShuffle},
		},
		Seeds: []uint64{1, 2},
	}
}

// TestCompareRunsTheOriginalAndEveryNullModelSeed checks the matrix shape and
// the hand-calculated original cell. With the exclude policy the six fixture
// edges carry the weights 8, -4*(4/6), 0, -4, 0 and 0, so the two-unit pulse
// into node 0 makes node 0 spike at step 0 and node 1 spike at steps 1 and 2;
// nodes 2 and 3 receive no positive drive and stay silent for all six steps.
func TestCompareRunsTheOriginalAndEveryNullModelSeed(t *testing.T) {
	set, g := derivedFixtureSet(t)
	cp := compareFixtureProtocol(6)
	report, err := Compare(context.Background(), g, set, fixtureSetSHA256, cp, testLimits())
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if report.SchemaVersion != CompareReportSchemaVersion || len(report.ProtocolHash) != 64 {
		t.Fatalf("report identity = %+v", report.SchemaVersion)
	}
	if report.ParameterSource != ParameterSourceDerived || report.ParameterSetSHA256 != fixtureSetSHA256 {
		t.Fatalf("report provenance = %q %q", report.ParameterSource, report.ParameterSetSHA256)
	}
	if len(report.Cells) != 7 {
		t.Fatalf("got %d cells, want 1 original and 3 kinds x 2 seeds", len(report.Cells))
	}
	if report.Cells[0].Variant != VariantOriginal || report.Cells[0].Null != nil || report.Cells[0].Kind != "" {
		t.Fatalf("cell 0 = %+v", report.Cells[0])
	}
	wantOrder := []struct {
		kind string
		seed uint64
	}{
		{NullDegreePreservingRewire, 1}, {NullDegreePreservingRewire, 2},
		{NullSignShuffle, 1}, {NullSignShuffle, 2},
		{NullWeightShuffle, 1}, {NullWeightShuffle, 2},
	}
	// The six fixture edges 0->1, 0->2, 0->3, 1->2, 2->3 and 3->0 admit exactly
	// one double edge swap: (1->2) and (3->0) can become (1->0) and (3->2).
	// Every other pair of edges would repeat an existing pair or make a self
	// loop, so almost every attempt on this fixture is refused. The rewire is
	// exercised properly on the forty node graph of the null model tests; here
	// the point is that the bookkeeping adds up and the matrix still runs.
	for i, want := range wantOrder {
		cell := report.Cells[i+1]
		if want.kind == NullDegreePreservingRewire {
			if cell.Null.Attempts != 12 {
				t.Fatalf("cell %d attempted %d swaps, want ceil(2*6)", i+1, cell.Null.Attempts)
			}
			if cell.Null.Applied+cell.Null.Rejected.SelfLoop+cell.Null.Rejected.Duplicate != cell.Null.Attempts {
				t.Fatalf("cell %d bookkeeping = %+v", i+1, cell.Null)
			}
		}
		if cell.Kind != want.kind || cell.Seed != want.seed || cell.Null == nil {
			t.Fatalf("cell %d = %s seed %d", i+1, cell.Kind, cell.Seed)
		}
		if cell.Index != i+1 || !strings.Contains(cell.Variant, want.kind) {
			t.Fatalf("cell %d name = %q index %d", i+1, cell.Variant, cell.Index)
		}
		if cell.Run.NullModel == nil || cell.Run.NullModel.Seed != want.seed {
			t.Fatalf("cell %d run report carries no null model", i+1)
		}
		if len(cell.Metrics) != len(cp.Metrics) || len(cell.Thresholds) != len(cp.Thresholds) {
			t.Fatalf("cell %d has %d metrics and %d thresholds", i+1, len(cell.Metrics), len(cell.Thresholds))
		}
	}
	original := map[string]MetricResult{}
	for _, result := range report.Cells[0].Metrics {
		original[result.Name] = result
	}
	// One of the two ALIN neurons (node 0) spikes, once, over six steps.
	if got := original["alin_fraction"]; !got.Defined || got.Value != 0.5 || got.Numerator != 1 || got.Denominator != 2 {
		t.Fatalf("original alin_fraction = %+v", got)
	}
	if got := original["alin_rate"]; !got.Defined || got.Numerator != 1 || got.Denominator != 12 {
		t.Fatalf("original alin_rate = %+v", got)
	}
	// Node 1 spikes at steps 1 and 2; node 2 never does.
	if got := original["alpn_rate"]; !got.Defined || got.Numerator != 2 || got.Denominator != 12 {
		t.Fatalf("original alpn_rate = %+v", got)
	}
	// The declared thresholds are reported, and a failing one is not an error.
	thresholds := report.Cells[0].Thresholds
	if !thresholds[0].Passed || thresholds[1].Passed {
		t.Fatalf("original thresholds = %+v", thresholds)
	}
	// Hand-checked summary of alpn_rate: the original is 2/12, both sign
	// shuffle seeds silence the ALPN set (0) and both weight shuffle seeds
	// halve it (1/12). With two defined values per kind the nearest rank
	// quantiles are [v0,v0,v0,v1,v1] and the percentile of the original is
	// (values below + half the values equal)/2, so an original that is above
	// both values scores 1 and one that equals both scores 0.5.
	for _, want := range []struct {
		kind       string
		quantile   float64
		percentile float64
	}{
		{NullDegreePreservingRewire, 2.0 / 12.0, 0.5},
		{NullSignShuffle, 0, 1},
		{NullWeightShuffle, 1.0 / 12.0, 1},
	} {
		got := report.Summary[2].PerKind[0]
		for _, perKind := range report.Summary[2].PerKind {
			if perKind.Kind == want.kind {
				got = perKind
			}
		}
		if got.Kind != want.kind || got.Defined != 2 {
			t.Fatalf("alpn_rate summary for %s = %+v", want.kind, got)
		}
		if got.Quantiles != [5]float64{want.quantile, want.quantile, want.quantile, want.quantile, want.quantile} {
			t.Fatalf("alpn_rate %s quantiles = %v, want five times %v", want.kind, got.Quantiles, want.quantile)
		}
		if !got.OriginalPercentileDefined || got.OriginalPercentile != want.percentile {
			t.Fatalf("alpn_rate %s percentile = %v, want %v", want.kind, got.OriginalPercentile, want.percentile)
		}
	}
	if report.Reproduction == "" || !strings.Contains(report.Reproduction, "simulate run") {
		t.Fatalf("reproduction = %q", report.Reproduction)
	}
	if len(report.Sets) != 2 || report.Sets[0].Count != 2 || len(report.Sets[0].NodeHash) != 64 {
		t.Fatalf("resolved sets = %+v", report.Sets)
	}
}

// TestCompareSummaryQuantilesAndPercentile recomputes the declared rules from
// the cells themselves: nearest rank quantiles over the defined values and the
// mid-rank percentile of the original inside that distribution.
func TestCompareSummaryQuantilesAndPercentile(t *testing.T) {
	set, g := derivedFixtureSet(t)
	cp := compareFixtureProtocol(6)
	report, err := Compare(context.Background(), g, set, fixtureSetSHA256, cp, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Summary) != len(cp.Metrics) {
		t.Fatalf("got %d summaries for %d metrics", len(report.Summary), len(cp.Metrics))
	}
	for s, summary := range report.Summary {
		metric := cp.Metrics[s]
		if summary.Metric != metric.Name || summary.Kind != metric.Kind || summary.Set != metric.Set {
			t.Fatalf("summary %d = %+v", s, summary)
		}
		if summary.Original != report.Cells[0].Metrics[s] {
			t.Fatalf("summary %q does not carry the original result", summary.Metric)
		}
		if len(summary.PerKind) != len(cp.NullModels) {
			t.Fatalf("summary %q has %d kinds", summary.Metric, len(summary.PerKind))
		}
		for k, perKind := range summary.PerKind {
			if perKind.Kind != cp.NullModels[k].Kind || perKind.Seeds != len(cp.Seeds) {
				t.Fatalf("summary %q kind %d = %+v", summary.Metric, k, perKind)
			}
			var values []float64
			for _, cell := range report.Cells[1:] {
				if cell.Kind != perKind.Kind {
					continue
				}
				if cell.Metrics[s].Defined {
					values = append(values, cell.Metrics[s].Value)
				}
			}
			if perKind.Defined != len(values) {
				t.Fatalf("summary %q kind %s counted %d defined values, found %d", summary.Metric, perKind.Kind, perKind.Defined, len(values))
			}
			sort.Float64s(values)
			var want [5]float64
			if len(values) > 0 {
				// Nearest rank: rank = ceil(q*n), clamped into [1,n].
				for i, q := range []float64{0, .25, .5, .75, 1} {
					rank := int(ceilRank(q, len(values)))
					want[i] = values[rank-1]
				}
			}
			if perKind.Quantiles != want {
				t.Fatalf("summary %q kind %s quantiles = %v, want %v (values %v)", summary.Metric, perKind.Kind, perKind.Quantiles, want, values)
			}
			if !summary.Original.Defined || len(values) == 0 {
				if perKind.OriginalPercentileDefined {
					t.Fatalf("summary %q kind %s claims a percentile without a distribution", summary.Metric, perKind.Kind)
				}
				continue
			}
			below, equal := 0.0, 0.0
			for _, v := range values {
				switch {
				case v < summary.Original.Value:
					below++
				case v == summary.Original.Value:
					equal++
				}
			}
			want1 := (below + 0.5*equal) / float64(len(values))
			if !perKind.OriginalPercentileDefined || perKind.OriginalPercentile != want1 {
				t.Fatalf("summary %q kind %s percentile = %v, want %v", summary.Metric, perKind.Kind, perKind.OriginalPercentile, want1)
			}
		}
	}
}

func ceilRank(q float64, n int) float64 {
	rank := math.Ceil(q * float64(n))
	if rank < 1 {
		rank = 1
	}
	if rank > float64(n) {
		rank = float64(n)
	}
	return rank
}

func TestCompareIsDeterministicApartFromWallTime(t *testing.T) {
	set, g := derivedFixtureSet(t)
	cp := compareFixtureProtocol(4)
	first, err := Compare(context.Background(), g, set, fixtureSetSHA256, cp, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	second, err := Compare(context.Background(), g, set, fixtureSetSHA256, cp, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if maskedJSON(t, first) != maskedJSON(t, second) {
		t.Fatalf("two identical compares differ:\n%s\n%s", maskedJSON(t, first), maskedJSON(t, second))
	}
	for _, cell := range first.Cells {
		if cell.WallSeconds < 0 {
			t.Fatalf("cell %d wall seconds = %v", cell.Index, cell.WallSeconds)
		}
	}
}

func maskedJSON(t *testing.T, report CompareReport) string {
	t.Helper()
	masked := report
	masked.Cells = append([]CompareCell(nil), report.Cells...)
	for i := range masked.Cells {
		masked.Cells[i].WallSeconds = 0
	}
	encoded, err := json.Marshal(masked)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestCompareProtocolValidation(t *testing.T) {
	set, g := derivedFixtureSet(t)
	base := compareFixtureProtocol(4)

	bad := map[string]func(*CompareProtocol){
		"wrong schema":         func(c *CompareProtocol) { c.SchemaVersion = "coimnet-simulate-compare/v2" },
		"no sets":              func(c *CompareProtocol) { c.Sets = nil },
		"no metrics":           func(c *CompareProtocol) { c.Metrics = nil },
		"duplicate set":        func(c *CompareProtocol) { c.Sets = append(c.Sets, c.Sets[0]) },
		"duplicate metric":     func(c *CompareProtocol) { c.Metrics = append(c.Metrics, c.Metrics[0]) },
		"metric unknown set":   func(c *CompareProtocol) { c.Metrics[0].Set = "nowhere" },
		"metric unknown kind":  func(c *CompareProtocol) { c.Metrics[0].Kind = "population_sync" },
		"window past the run":  func(c *CompareProtocol) { c.Metrics[0].Window = [2]int{0, 99} },
		"empty window":         func(c *CompareProtocol) { c.Metrics[0].Window = [2]int{2, 2} },
		"stray baseline":       func(c *CompareProtocol) { c.Metrics[0].Baseline = [2]int{0, 1} },
		"missing baseline":     func(c *CompareProtocol) { c.Metrics[0].Kind = MetricActivityRatioVsBaseline },
		"threshold no metric":  func(c *CompareProtocol) { c.Thresholds[0].Metric = "nowhere" },
		"threshold bad op":     func(c *CompareProtocol) { c.Thresholds[0].Op = "==" },
		"threshold bad value":  func(c *CompareProtocol) { c.Thresholds[0].Value = math.NaN() },
		"no seed":              func(c *CompareProtocol) { c.Seeds = nil },
		"duplicate seed":       func(c *CompareProtocol) { c.Seeds = []uint64{1, 1} },
		"duplicate kind":       func(c *CompareProtocol) { c.NullModels = append(c.NullModels, c.NullModels[0]) },
		"unknown kind":         func(c *CompareProtocol) { c.NullModels[0].Kind = "rewire" },
		"seed inside the spec": func(c *CompareProtocol) { c.NullModels[0].Seed = 3 },
		"invalid run":          func(c *CompareProtocol) { c.Run.Probes = nil },
	}
	for name, mutate := range bad {
		cp := base
		cp.Sets = append([]NamedSet(nil), base.Sets...)
		cp.Metrics = append([]Metric(nil), base.Metrics...)
		cp.Thresholds = append([]Threshold(nil), base.Thresholds...)
		cp.NullModels = append([]NullModelSpec(nil), base.NullModels...)
		cp.Seeds = append([]uint64(nil), base.Seeds...)
		mutate(&cp)
		if err := cp.Validate(); err == nil {
			t.Errorf("%s accepted by Validate", name)
			continue
		}
		if _, err := Compare(context.Background(), g, set, fixtureSetSHA256, cp, testLimits()); err == nil {
			t.Errorf("%s accepted by Compare", name)
		}
	}

	// The continuous core rejects every metric that needs events.
	continuous := base
	continuous.Run = derivedProtocol(CoreContinuous, UnknownSignExclude, fixtureWeightScale, 4)
	if err := continuous.Validate(); err == nil {
		t.Error("the continuous core accepted a spike_fraction metric")
	}
	continuous.Metrics = []Metric{{Name: "alin_output", Kind: MetricMeanOutput, Set: "alin", Window: [2]int{0, 4}}}
	continuous.Thresholds = []Threshold{{Metric: "alin_output", Op: OpAtLeast, Value: 0}}
	if err := continuous.Validate(); err != nil {
		t.Errorf("mean_output on the continuous core: %v", err)
	}
	// A sign shuffle needs the derived source whatever the core.
	uniform := base
	uniform.Run = lifProtocol(4)
	uniform.NullModels = []NullModelSpec{{Kind: NullSignShuffle}}
	if err := uniform.Validate(); err == nil {
		t.Error("a sign shuffle was accepted against the uniform parameter source")
	}
}

func TestDecodeCompareProtocolIsStrict(t *testing.T) {
	cp := compareFixtureProtocol(4)
	encoded, err := json.Marshal(cp)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCompareProtocol(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	firstHash, err := decoded.Hash()
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := cp.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash || len(firstHash) != 64 {
		t.Fatalf("hash %q != %q", firstHash, secondHash)
	}
	for name, document := range map[string]string{
		"unknown field": `{"schema_version":"coimnet-simulate-compare/v1","surprise":1}`,
		"trailing data": string(encoded) + `{}`,
		"not an object": `[]`,
		"empty":         ``,
	} {
		if _, err := DecodeCompareProtocol(strings.NewReader(document)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestCompareRefusesAMismatchedParameterSource(t *testing.T) {
	set, g := derivedFixtureSet(t)
	cp := compareFixtureProtocol(4)
	if _, err := Compare(context.Background(), g, nil, "", cp, testLimits()); err == nil {
		t.Error("a derived compare ran without a parameter set")
	}
	uniform := compareFixtureProtocol(4)
	uniform.Run = lifProtocol(4)
	uniform.Run.Probes = []Probe{{Name: "all", Nodes: []int{0, 1, 2, 3}, Reduce: ReduceSumOutput}}
	uniform.NullModels = []NullModelSpec{{Kind: NullWeightShuffle}}
	if _, err := Compare(context.Background(), g, set, fixtureSetSHA256, uniform, testLimits()); err == nil {
		t.Error("a uniform compare accepted a derived parameter set")
	}
	report, err := Compare(context.Background(), g, nil, "", uniform, testLimits())
	if err != nil {
		t.Fatalf("uniform compare: %v", err)
	}
	if report.ParameterSource != ParameterSourceUniform || report.ParameterSetSHA256 != "" {
		t.Fatalf("uniform report provenance = %q %q", report.ParameterSource, report.ParameterSetSHA256)
	}
	if len(report.Cells) != 3 {
		t.Fatalf("uniform compare produced %d cells, want 1 original and 1 kind x 2 seeds", len(report.Cells))
	}
}

// TestCompareRunsBothCoresOnTheSameGraph runs the same wiring, the same
// parameter set and the same stimulus through the spiking and the continuous
// core with one null model, which is the two core comparison of the ticket at
// fixture scale.
func TestCompareRunsBothCoresOnTheSameGraph(t *testing.T) {
	set, g := derivedFixtureSet(t)
	spiking := compareFixtureProtocol(4)
	spiking.NullModels = []NullModelSpec{{Kind: NullWeightShuffle}}
	spikingReport, err := Compare(context.Background(), g, set, fixtureSetSHA256, spiking, testLimits())
	if err != nil {
		t.Fatalf("spiking compare: %v", err)
	}

	continuous := spiking
	continuous.Run = derivedProtocol(CoreContinuous, UnknownSignExclude, fixtureWeightScale, 4)
	continuous.Metrics = []Metric{
		{Name: "alin_output", Kind: MetricMeanOutput, Set: "alin", Window: [2]int{0, 4}},
		{Name: "alpn_output", Kind: MetricMeanOutput, Set: "alpn", Window: [2]int{0, 4}},
	}
	continuous.Thresholds = []Threshold{{Metric: "alin_output", Op: OpAtLeast, Value: 0}}
	continuousReport, err := Compare(context.Background(), g, set, fixtureSetSHA256, continuous, testLimits())
	if err != nil {
		t.Fatalf("continuous compare: %v", err)
	}
	if len(spikingReport.Cells) != 3 || len(continuousReport.Cells) != 3 {
		t.Fatalf("cells = %d spiking, %d continuous", len(spikingReport.Cells), len(continuousReport.Cells))
	}
	if spikingReport.Cells[0].Run.Core != CoreLIF || continuousReport.Cells[0].Run.Core != CoreContinuous {
		t.Fatalf("cores = %q and %q", spikingReport.Cells[0].Run.Core, continuousReport.Cells[0].Run.Core)
	}
	// Both matrices ran the same wiring and the same parameter set.
	if spikingReport.GraphHashes != continuousReport.GraphHashes {
		t.Fatal("the two cores ran different graphs")
	}
	for i := range spikingReport.Cells {
		if spikingReport.Cells[i].Run.TopologyHash != continuousReport.Cells[i].Run.TopologyHash {
			t.Fatalf("cell %d ran a different topology on the two cores", i)
		}
		if spikingReport.Cells[i].Run.ParameterHash != continuousReport.Cells[i].Run.ParameterHash {
			t.Fatalf("cell %d ran different parameters on the two cores", i)
		}
	}
	for _, summary := range continuousReport.Summary {
		if !summary.Original.Defined || summary.PerKind[0].Defined != 2 {
			t.Fatalf("continuous summary %q = %+v", summary.Metric, summary.PerKind)
		}
	}
}
