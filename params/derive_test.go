package params

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/TimLai666/coimnet/internal/extsort"
)

// wantEdge is one hand-calculated row of the fixture derivation. Every
// probability in the fixture is an exact sixteenth, so the mean of one or two
// of them is exact in float32 and float64 and the values below are written
// down, not recomputed by the test.
type wantEdge struct {
	pair            [2]uint64
	rawWeight       int64
	matched         uint32
	transmitter     uint8
	confidence      float32
	sign            int8
	weightPostTotal float64
	weightPreTotal  float64
	weightNone      float64
}

// fixtureEdges is the expected result of the base rules (gain 2,
// min_probability 0.5, min_matched_fraction 0.25) in canonical edge order.
//
//	edge   pair    raw  matched  mean of the winning probability  transmitter    sign  matched fraction
//	0    (0,1)     4       2     (0.75+0.5)/2   = 0.625           acetylcholine   +1   2/4 = 0.5
//	1    (0,2)     2       1     0.6875                           gaba            -1   1/2 = 0.5
//	2    (0,3)     4       1     0.625                            dopamine         0   1/4 = 0.25 (inclusive bound)
//	3    (1,2)     3       1     0.5625                           glutamate       -1   1/3 = 0.333...
//	4    (2,3)     1       0     none: the only synapse hit a null-probability T-bar
//	5    (3,0)     8       0     none: the only synapse is one voxel off in z
//
//	weights: post_total normalizer uses the target's body-stats post
//	(node 0 = 7, node 1 = 4, node 2 = 6, node 3 = 0 because body 40 is missing);
//	pre_total uses the source's pre (5, 3, 2 and 0).
var fixtureEdges = []wantEdge{
	{pair: [2]uint64{0, 1}, rawWeight: 4, matched: 2, transmitter: 0, confidence: 0.625, sign: 1, weightPostTotal: 2 * 4.0 / 4.0, weightPreTotal: 2 * 4.0 / 5.0, weightNone: 8},
	{pair: [2]uint64{0, 2}, rawWeight: 2, matched: 1, transmitter: 2, confidence: 0.6875, sign: -1, weightPostTotal: 2 * 2.0 / 6.0, weightPreTotal: 2 * 2.0 / 5.0, weightNone: 4},
	{pair: [2]uint64{0, 3}, rawWeight: 4, matched: 1, transmitter: 1, confidence: 0.625, sign: 0, weightPostTotal: 0, weightPreTotal: 2 * 4.0 / 5.0, weightNone: 8},
	{pair: [2]uint64{1, 2}, rawWeight: 3, matched: 1, transmitter: 3, confidence: 0.5625, sign: -1, weightPostTotal: 2 * 3.0 / 6.0, weightPreTotal: 2 * 3.0 / 3.0, weightNone: 6},
	{pair: [2]uint64{2, 3}, rawWeight: 1, matched: 0, transmitter: NoTransmitter, confidence: 0, sign: 0, weightPostTotal: 0, weightPreTotal: 2 * 1.0 / 2.0, weightNone: 2},
	{pair: [2]uint64{3, 0}, rawWeight: 8, matched: 0, transmitter: NoTransmitter, confidence: 0, sign: 0, weightPostTotal: 2 * 8.0 / 7.0, weightPreTotal: 0, weightNone: 16},
}

func deriveFixture(t *testing.T, normalizer string, sign SignRule) (*Set, Report) {
	t.Helper()
	dir := t.TempDir()
	graph := graphFixture(t, dir)
	rules := fixtureRules(t, dir, normalizer, sign)
	set, report, err := Derive(context.Background(), graph, rules, dir, fixtureLimits(t))
	if err != nil {
		t.Fatalf("Derive(%s): %v", normalizer, err)
	}
	return set, report
}

func TestDeriveMatchesTheHandCalculatedFixture(t *testing.T) {
	set, report := deriveFixture(t, NormalizerPostTotal, baseSignRule())

	if set.Source != SetSource || len(set.EdgeWeight) != 6 || len(set.NodePreTotal) != 4 {
		t.Fatalf("set shape = source %q, %d edges, %d nodes", set.Source, len(set.EdgeWeight), len(set.NodePreTotal))
	}
	for i, want := range fixtureEdges {
		if set.EdgeTransmitter[i] != want.transmitter {
			t.Errorf("edge %d transmitter = %d, want %d", i, set.EdgeTransmitter[i], want.transmitter)
		}
		if set.EdgeSign[i] != want.sign {
			t.Errorf("edge %d sign = %d, want %d", i, set.EdgeSign[i], want.sign)
		}
		if set.EdgeSignConfidence[i] != want.confidence {
			t.Errorf("edge %d confidence = %v, want %v", i, set.EdgeSignConfidence[i], want.confidence)
		}
		if set.EdgeMatchedSynapses[i] != want.matched {
			t.Errorf("edge %d matched synapses = %d, want %d", i, set.EdgeMatchedSynapses[i], want.matched)
		}
		if set.EdgeWeight[i] != want.weightPostTotal {
			t.Errorf("edge %d weight = %v, want %v", i, set.EdgeWeight[i], want.weightPostTotal)
		}
	}

	// body-stats: body 40 is absent, body 20 appears twice and the first row
	// wins, one row has a null body and body 99 is not selected.
	if got := []int64{5, 3, 2, 0}; !equalInt64(set.NodePreTotal, got) {
		t.Errorf("node pre totals = %v, want %v", set.NodePreTotal, got)
	}
	if got := []int64{7, 4, 6, 0}; !equalInt64(set.NodePostTotal, got) {
		t.Errorf("node post totals = %v, want %v", set.NodePostTotal, got)
	}
	stats := report.BodyStats
	if stats.Rows != 6 || stats.RowsSelected != 3 || stats.RowsNotSelected != 1 || stats.DuplicateRows != 1 || stats.NullBodyRows != 1 || stats.BodiesMissing != 1 {
		t.Errorf("body stats summary = %+v", stats)
	}

	tbar := report.Tbar
	if tbar.Rows != 12 || tbar.RowsSkippedNullKey != 1 || tbar.RowsSkippedNullBody != 1 || tbar.RowsSorted != 10 {
		t.Errorf("tbar counts = %+v", tbar)
	}
	if tbar.Keys != 9 || tbar.AmbiguousKeys != 1 || tbar.RowsInAmbiguousKeys != 2 || tbar.KeysWithNullProbability != 1 || tbar.RowsWithNullProbability != 1 || tbar.UsableKeys != 7 {
		t.Errorf("tbar key counts = %+v", tbar)
	}
	if tbar.KeysWithoutSynapse != 1 {
		t.Errorf("tbar keys without a synapse = %d, want 1 (key (16,17,18))", tbar.KeysWithoutSynapse)
	}

	syn := report.SynPartners
	if syn.Rows != 14 || syn.RowsSorted != 10 {
		t.Errorf("syn rows = %d sorted = %d, want 14 and 10", syn.Rows, syn.RowsSorted)
	}
	if syn.RowsNullBody != 1 || syn.RowsNullCoordinate != 1 || syn.RowsBodyNotSelected != 2 {
		t.Errorf("syn exclusions = %+v", syn)
	}
	if syn.MatchedSynapses != 6 || syn.UnmatchedSynapses != 2 || syn.SynapsesOnAmbiguousKey != 1 || syn.SynapsesOnNullProbabilityKey != 1 {
		t.Errorf("syn join counts = %+v", syn)
	}

	if report.Pairs.Pairs != 5 || report.Pairs.NotInGraph != 1 {
		t.Errorf("pair counts = %+v", report.Pairs)
	}

	edges := report.Edges
	if edges.Edges != 6 || edges.WithMatch != 4 || edges.WithoutMatch != 2 || edges.DuplicatePairEdges != 0 {
		t.Errorf("edge counts = %+v", edges)
	}
	if edges.Sign.Positive != 1 || edges.Sign.Negative != 2 || edges.Sign.Unknown != 3 {
		t.Errorf("sign histogram = %+v, want 1 2 3", edges.Sign)
	}
	if edges.UnknownRatio.Numerator != 3 || edges.UnknownRatio.Denominator != 6 || edges.UnknownRatio.Value != 0.5 || !edges.UnknownRatio.Defined {
		t.Errorf("unknown ratio = %+v", edges.UnknownRatio)
	}
	if edges.UnknownByMapping != 1 || edges.UnknownWithoutMatch != 2 || edges.UnknownBelowProbability != 0 || edges.UnknownBelowMatchedFraction != 0 {
		t.Errorf("unknown reasons = %+v", edges)
	}
	wantTransmitters := map[string]uint64{"acetylcholine": 1, "dopamine": 1, "gaba": 1, "glutamate": 1, "histamine": 0, "octopamine": 0, "serotonin": 0}
	for name, want := range wantTransmitters {
		if edges.TransmitterEdges[name] != want {
			t.Errorf("transmitter %s edges = %d, want %d", name, edges.TransmitterEdges[name], want)
		}
	}
	if edges.TransmitterUnmatched != 2 {
		t.Errorf("edges without a transmitter = %d, want 2", edges.TransmitterUnmatched)
	}
	if edges.NormalizerZeroEdges != 2 {
		t.Errorf("normalizer zero edges = %d, want 2 (both target node 3)", edges.NormalizerZeroEdges)
	}
	// Sorted weights are 0, 0, 0.666..., 1, 2, 2.2857...; the report uses the
	// nearest-rank quantile, so p25 is element 1 and p75 is element 4.
	q := edges.WeightQuantiles
	if q.Status != StatusMeasured || q.P0 != 0 || q.P25 != 0 || q.P50 != 2*2.0/6.0 || q.P75 != 2 || q.P100 != 2*8.0/7.0 {
		t.Errorf("weight quantiles = %+v", q)
	}

	// primary_post mode per selected post body: node 0 has only a null ROI,
	// node 1 has AL four times against LH twice, node 2 only MB, node 3 CX
	// twice against MB once.
	if want := []string{"", "AL", "MB", "CX"}; !equalString(set.NodePrimaryROI, want) {
		t.Errorf("primary ROI = %v, want %v", set.NodePrimaryROI, want)
	}
	roi := report.PrimaryROI
	if roi.Rows != 11 || roi.NullROIRows != 2 || roi.Names != 4 || roi.NodesWithoutROI != 1 {
		t.Errorf("primary ROI summary = %+v", roi)
	}

	if report.Meta.PostHighAccuracyThreshold != 0.5 || report.Meta.PreHPThreshold != 0 || report.Meta.PostHPThreshold != 0.7 {
		t.Errorf("meta thresholds = %+v", report.Meta)
	}
	if want := []int{1, 2, 4}; !equalInt(report.Meta.ROILevelCounts, want) {
		t.Errorf("roi hierarchy level counts = %v, want %v", report.Meta.ROILevelCounts, want)
	}
	if report.Meta.ROILevels != 3 || report.Meta.ROIRoot != "CNS" {
		t.Errorf("roi hierarchy = %+v", report.Meta)
	}

	if report.SchemaVersion != ReportSchemaVersion || report.RulesHash == "" || len(report.RulesHash) != 64 {
		t.Errorf("report identity = %q %q", report.SchemaVersion, report.RulesHash)
	}
	if len(report.Sources) != 4 {
		t.Errorf("report lists %d sources, want 4", len(report.Sources))
	}
	if len(report.Sorts) != 4 {
		t.Errorf("report lists %d sorts, want 4", len(report.Sorts))
	}
	// Two sorted intermediate files: the grouped T-bar keys and the pair
	// aggregates. Nine distinct keys and five pairs reach them.
	if len(report.Spills) != 2 || report.Spills[0].Records != 9 || report.Spills[1].Records != 5 {
		t.Errorf("report spills = %+v", report.Spills)
	}
	if report.WallTimeSeconds < 0 {
		t.Errorf("wall time = %v", report.WallTimeSeconds)
	}
}

func TestDeriveNormalizersAndZeroCounts(t *testing.T) {
	for _, tc := range []struct {
		normalizer string
		want       func(wantEdge) float64
		zeroEdges  uint64
	}{
		{NormalizerNone, func(e wantEdge) float64 { return e.weightNone }, 0},
		{NormalizerPreTotal, func(e wantEdge) float64 { return e.weightPreTotal }, 1},
	} {
		set, report := deriveFixture(t, tc.normalizer, baseSignRule())
		for i, want := range fixtureEdges {
			if set.EdgeWeight[i] != tc.want(want) {
				t.Errorf("%s edge %d weight = %v, want %v", tc.normalizer, i, set.EdgeWeight[i], tc.want(want))
			}
		}
		if report.Edges.NormalizerZeroEdges != tc.zeroEdges {
			t.Errorf("%s normalizer zero edges = %d, want %d", tc.normalizer, report.Edges.NormalizerZeroEdges, tc.zeroEdges)
		}
		if report.Edges.WeightQuantiles.Status != StatusMeasured {
			t.Errorf("%s quantile status = %q", tc.normalizer, report.Edges.WeightQuantiles.Status)
		}
	}
}

func TestDeriveThresholdsTurnEdgesUnknown(t *testing.T) {
	// min_probability 0.65 leaves only gaba (0.6875) above the bar.
	sign := baseSignRule()
	sign.MinProbability = 0.65
	_, report := deriveFixture(t, NormalizerPostTotal, sign)
	if report.Edges.Sign.Positive != 0 || report.Edges.Sign.Negative != 1 || report.Edges.Sign.Unknown != 5 {
		t.Errorf("min_probability 0.65 histogram = %+v, want 0 1 5", report.Edges.Sign)
	}
	// Acetylcholine 0.625, dopamine 0.625 and glutamate 0.5625 all fall below.
	if report.Edges.UnknownBelowProbability != 3 {
		t.Errorf("edges below min_probability = %d, want 3", report.Edges.UnknownBelowProbability)
	}

	// min_matched_fraction 0.4 excludes the glutamate edge at 1/3 and the
	// dopamine edge at exactly 0.25, which was inside the base bound.
	sign = baseSignRule()
	sign.MinMatchedFraction = 0.4
	_, report = deriveFixture(t, NormalizerPostTotal, sign)
	if report.Edges.Sign.Positive != 1 || report.Edges.Sign.Negative != 1 || report.Edges.Sign.Unknown != 4 {
		t.Errorf("min_matched_fraction 0.4 histogram = %+v, want 1 1 4", report.Edges.Sign)
	}
	if report.Edges.UnknownBelowMatchedFraction != 2 {
		t.Errorf("edges below min_matched_fraction = %d, want 2", report.Edges.UnknownBelowMatchedFraction)
	}
}

func TestDeriveKeepsConfidenceForUnknownSigns(t *testing.T) {
	set, _ := deriveFixture(t, NormalizerPostTotal, baseSignRule())
	// The dopamine edge is unknown by rule but keeps its measured confidence.
	if set.EdgeSign[2] != 0 || set.EdgeSignConfidence[2] != 0.625 || set.EdgeTransmitter[2] != 1 {
		t.Fatalf("dopamine edge = sign %d confidence %v transmitter %d", set.EdgeSign[2], set.EdgeSignConfidence[2], set.EdgeTransmitter[2])
	}
}

func TestDeriveIsDeterministic(t *testing.T) {
	first, _ := deriveFixture(t, NormalizerPostTotal, baseSignRule())
	second, _ := deriveFixture(t, NormalizerPostTotal, baseSignRule())
	if first.RulesHash != second.RulesHash {
		t.Fatalf("rules hash differs: %s vs %s", first.RulesHash, second.RulesHash)
	}
	for i := range first.EdgeWeight {
		if first.EdgeWeight[i] != second.EdgeWeight[i] || first.EdgeSign[i] != second.EdgeSign[i] || first.EdgeTransmitter[i] != second.EdgeTransmitter[i] {
			t.Fatalf("edge %d differs between runs", i)
		}
	}
}

func TestDeriveRejectsChangedSources(t *testing.T) {
	dir := t.TempDir()
	graph := graphFixture(t, dir)
	rules := fixtureRules(t, dir, NormalizerPostTotal, baseSignRule())
	rules.Sources.Tbar.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	_, _, err := Derive(context.Background(), graph, rules, dir, fixtureLimits(t))
	if !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("Derive with a wrong fingerprint returned %v, want ErrSourceChanged", err)
	}
}

// bigFixture repeats the matched pair (0,1) with distinct coordinates so both
// sorts spill several runs under a small memory limit.
func bigFixture(t *testing.T, rows int) ([]tbarRow, []synRow) {
	t.Helper()
	tbar := make([]tbarRow, 0, rows)
	syn := make([]synRow, 0, rows)
	for i := 0; i < rows; i++ {
		x := int32(1000 + i)
		tbar = append(tbar, tbarRow{x: x, y: 1, z: 1, body: 10, probs: p16(12, 1, 1, 1, 1, 0, 0), nullProb: -1})
		syn = append(syn, synRow{x: x, y: 1, z: 1, pre: 10, post: 20, roi: "AL"})
	}
	return tbar, syn
}

func TestDeriveUsesMultipleSortRuns(t *testing.T) {
	dir := t.TempDir()
	graph := graphFixture(t, dir)
	tbar, syn := bigFixture(t, 2000)
	rules := Rules{
		SchemaVersion: RulesSchemaVersion,
		Sources:       sourceFixture(t, dir, tbar, syn, bodyStatsFixtureRows),
		Sign:          baseSignRule(),
		Strength:      StrengthRule{Gain: 1, Normalizer: NormalizerNone},
	}
	limits := fixtureLimits(t)
	limits.MaxMemoryBytes = 200 << 10 // forces small sort buffers
	set, report, err := Derive(context.Background(), graph, rules, dir, limits)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if set.EdgeMatchedSynapses[0] != 2000 {
		t.Fatalf("edge 0 matched %d synapses, want 2000", set.EdgeMatchedSynapses[0])
	}
	multi := 0
	for _, sort := range report.Sorts {
		if sort.Runs > 1 {
			multi++
		}
		if sort.TempBytes <= 0 {
			t.Errorf("sort %s wrote %d temporary bytes", sort.Name, sort.TempBytes)
		}
	}
	if multi < 2 {
		t.Fatalf("only %d of %d sorts used more than one run: %+v", multi, len(report.Sorts), report.Sorts)
	}
	if err := assertEmptyDir(limits.TempDir); err != nil {
		t.Fatal(err)
	}
}

func TestDeriveCancellationAndCapacityLeaveNoTemporaryFiles(t *testing.T) {
	t.Run("cancelled", func(t *testing.T) {
		dir := t.TempDir()
		graph := graphFixture(t, dir)
		tbar, syn := bigFixture(t, 2000)
		rules := Rules{SchemaVersion: RulesSchemaVersion, Sources: sourceFixture(t, dir, tbar, syn, bodyStatsFixtureRows), Sign: baseSignRule(), Strength: StrengthRule{Gain: 1, Normalizer: NormalizerNone}}
		limits := fixtureLimits(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, _, err := Derive(ctx, graph, rules, dir, limits); !errors.Is(err, context.Canceled) {
			t.Fatalf("Derive on a cancelled context returned %v", err)
		}
		if err := assertEmptyDir(limits.TempDir); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("capacity", func(t *testing.T) {
		dir := t.TempDir()
		graph := graphFixture(t, dir)
		tbar, syn := bigFixture(t, 2000)
		rules := Rules{SchemaVersion: RulesSchemaVersion, Sources: sourceFixture(t, dir, tbar, syn, bodyStatsFixtureRows), Sign: baseSignRule(), Strength: StrengthRule{Gain: 1, Normalizer: NormalizerNone}}
		limits := fixtureLimits(t)
		limits.MaxMemoryBytes = 200 << 10
		limits.MaxTempBytes = 6000 // far below what 2000 rows need
		_, _, err := Derive(context.Background(), graph, rules, dir, limits)
		if !errors.Is(err, ErrCapacity) || !errors.Is(err, extsort.ErrCapacity) {
			t.Fatalf("Derive over the temporary budget returned %v, want ErrCapacity", err)
		}
		if err := assertEmptyDir(limits.TempDir); err != nil {
			t.Fatal(err)
		}
	})
}

func TestDeriveRejectsInvalidInput(t *testing.T) {
	dir := t.TempDir()
	graph := graphFixture(t, dir)
	rules := fixtureRules(t, dir, NormalizerPostTotal, baseSignRule())
	limits := fixtureLimits(t)
	if _, _, err := Derive(context.Background(), nil, rules, dir, limits); err == nil {
		t.Error("accepted a nil graph")
	}
	bad := rules
	bad.Strength.Normalizer = "sqrt"
	if _, _, err := Derive(context.Background(), graph, bad, dir, limits); err == nil {
		t.Error("accepted an unknown normalizer")
	}
	bad = rules
	bad.Sign.Mapping = map[string]string{"acetylcholine": SignPositive}
	if _, _, err := Derive(context.Background(), graph, bad, dir, limits); err == nil {
		t.Error("accepted a mapping missing six transmitters")
	}
	for _, limit := range []Limits{
		{MaxMemoryBytes: 0, MaxTempBytes: 1 << 20, MaxRunFiles: 4, MaxArrowBytes: 1 << 20, MaxRows: 10, TempDir: dir},
		{MaxMemoryBytes: 1 << 20, MaxTempBytes: 0, MaxRunFiles: 4, MaxArrowBytes: 1 << 20, MaxRows: 10, TempDir: dir},
		{MaxMemoryBytes: 1 << 20, MaxTempBytes: 1 << 20, MaxRunFiles: 0, MaxArrowBytes: 1 << 20, MaxRows: 10, TempDir: dir},
		{MaxMemoryBytes: 1 << 20, MaxTempBytes: 1 << 20, MaxRunFiles: 4, MaxArrowBytes: 1 << 20, MaxRows: 10, TempDir: filepath.Join(dir, "absent")},
	} {
		if _, _, err := Derive(context.Background(), graph, rules, dir, limit); err == nil {
			t.Errorf("accepted limits %+v", limit)
		}
	}
	if _, _, err := Derive(context.Background(), graph, rules, dir, Limits{MaxMemoryBytes: 1 << 20, MaxTempBytes: 1 << 20, MaxRunFiles: 4, MaxArrowBytes: 1 << 20, MaxRows: 1, TempDir: dir}); err == nil {
		t.Error("accepted a row limit below the fixture row count")
	}
}

func assertEmptyDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		return errors.New("temporary directory still holds " + names[0])
	}
	return nil
}

func equalInt64(got, want []int64) bool {
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

func equalInt(got, want []int) bool {
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

func equalString(got, want []string) bool {
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

func TestQuantileOfSortedValues(t *testing.T) {
	values := []float64{0, 0, 2.0 / 3.0, 1, 2, 16.0 / 7.0}
	q := quantiles(append([]float64(nil), values...))
	if q.Status != StatusMeasured || q.P0 != 0 || q.P25 != 0 || q.P50 != 2.0/3.0 || q.P75 != 2 || q.P100 != 16.0/7.0 {
		t.Fatalf("quantiles = %+v", q)
	}
	empty := quantiles(nil)
	if empty.Status != StatusNotMeasured || empty.P50 != 0 {
		t.Fatalf("empty quantiles = %+v", empty)
	}
	if math.IsNaN(q.P50) {
		t.Fatal("quantiles must not produce NaN")
	}
}
