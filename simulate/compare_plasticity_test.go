package simulate

import (
	"context"
	"math"
	"strings"
	"testing"
)

// learningCompareProtocol is the three-cell comparison of the ticket on the
// three-neuron fixture: the same protocol and the same stimulus as
// TestPlasticWeightsReachTheNextStep, one named set per class, two metrics over
// the whole run and one null model kind so the learning variants and the
// ticket 14 matrix are shown to coexist.
func learningCompareProtocol(steps int, variants []string, enable bool) CompareProtocol {
	return CompareProtocol{
		SchemaVersion: CompareProtocolSchemaVersion,
		Run:           plasticLIFProtocol(steps, 1, 4, enable),
		Sets: []NamedSet{
			{Name: "alin", Selectors: []Selector{{Field: "class", Equals: "ALIN"}}},
			{Name: "alpn", Selectors: []Selector{{Field: "class", Equals: "ALPN"}}},
		},
		Metrics: []Metric{
			{Name: "alpn_rate", Kind: MetricMeanRate, Set: "alpn", Window: [2]int{0, steps}},
			{Name: "alpn_fraction", Kind: MetricSpikeFraction, Set: "alpn", Window: [2]int{0, steps}},
			{Name: "alin_rate", Kind: MetricMeanRate, Set: "alin", Window: [2]int{0, steps}},
		},
		Thresholds:       []Threshold{{Metric: "alpn_rate", Op: OpAtLeast, Value: 0.4}},
		NullModels:       []NullModelSpec{{Kind: NullWeightShuffle}},
		Seeds:            []uint64{7},
		LearningVariants: variants,
	}
}

// TestCompareLearningVariantsRunTheThreeCells is the acceptance test of root
// decision 3. The three cells are one matrix over the same stimulus:
//
//	original            node 0 spikes at step 0, node 1 at step 1, node 2 at
//	                    step 2; the two ALPN neurons fire twice in six neuron
//	                    steps, so mean_rate is 2/6.
//	plastic             the fast change of edge 0 adds 4k to its weight, so
//	                    node 1 also fires at step 2 and mean_rate is 3/6.
//	learned_then_frozen the same stimulus with w_eff fixed at base + the final
//	                    fast changes and no further update: the learned spike
//	                    survives, so mean_rate stays 3/6.
func TestCompareLearningVariantsRunTheThreeCells(t *testing.T) {
	g := fixtureGraph(t)
	cp := learningCompareProtocol(3, []string{"original", "plastic", "learned_then_frozen"}, true)
	report, err := Compare(context.Background(), g, nil, "", cp, testLimits())
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if len(report.Cells) != 4 {
		t.Fatalf("got %d cells, want three learning variants and one null model seed", len(report.Cells))
	}
	for i, want := range []string{VariantOriginal, VariantPlastic, VariantLearnedThenFrozen} {
		cell := report.Cells[i]
		if cell.Variant != want || cell.Index != i || cell.Kind != "" || cell.Null != nil {
			t.Fatalf("cell %d = %+v, want the %q learning variant", i, cell, want)
		}
	}
	if report.Cells[3].Kind != NullWeightShuffle || report.Cells[3].Run.Plasticity != nil {
		t.Fatalf("cell 3 = %+v, want the ticket 14 null model cell without plasticity", report.Cells[3])
	}

	// The original cell declares no plasticity at all: it is the run of the
	// protocol with the block stripped.
	if report.Cells[0].Run.Plasticity != nil {
		t.Fatalf("the original cell reports plasticity %+v", report.Cells[0].Run.Plasticity)
	}
	plastic := report.Cells[1].Run.Plasticity
	frozen := report.Cells[2].Run.Plasticity
	if plastic == nil || frozen == nil {
		t.Fatal("a learning cell carries no plasticity block")
	}
	if plastic.Frozen || !frozen.Frozen {
		t.Fatalf("frozen flags = %v %v", plastic.Frozen, frozen.Frozen)
	}
	if frozen.PlasticL2Before != plastic.PlasticL2After || frozen.PlasticL2After != plastic.PlasticL2After {
		t.Fatalf("frozen l2 = %v..%v, want the final %v of the plastic cell",
			frozen.PlasticL2Before, frozen.PlasticL2After, plastic.PlasticL2After)
	}

	metric := func(cell CompareCell, name string) MetricResult {
		t.Helper()
		for _, result := range cell.Metrics {
			if result.Name == name {
				return result
			}
		}
		t.Fatalf("cell %d has no metric %q", cell.Index, name)
		return MetricResult{}
	}
	delta := func(cell CompareCell, name string) MetricDelta {
		t.Helper()
		for _, result := range cell.Deltas {
			if result.Metric == name {
				return result
			}
		}
		t.Fatalf("cell %d has no delta for %q", cell.Index, name)
		return MetricDelta{}
	}
	// The two rates and their difference are float64 arithmetic, not an exact
	// constant expression: the delta is the difference of the two rounded
	// rates, which is not the rounding of one sixth.
	originalRate, learnedRate := 2.0/6.0, 3.0/6.0
	if got := metric(report.Cells[0], "alpn_rate"); !got.Defined || got.Value != originalRate {
		t.Fatalf("original alpn_rate = %+v, want 2/6", got)
	}
	for _, index := range []int{1, 2} {
		if got := metric(report.Cells[index], "alpn_rate"); !got.Defined || got.Value != learnedRate {
			t.Fatalf("cell %d alpn_rate = %+v, want 3/6", index, got)
		}
		if got := delta(report.Cells[index], "alpn_rate"); !got.Defined || got.Value != learnedRate-originalRate {
			t.Fatalf("cell %d delta = %+v, want 3/6 - 2/6 = %v", index, got, learnedRate-originalRate)
		}
		// Both ALPN neurons fire in every variant, so this one is unchanged and
		// the report says so with a zero difference rather than with a word.
		if got := delta(report.Cells[index], "alpn_fraction"); !got.Defined || got.Value != 0 {
			t.Fatalf("cell %d alpn_fraction delta = %+v", index, got)
		}
	}
	for _, d := range report.Cells[0].Deltas {
		if d.Defined && d.Value != 0 {
			t.Fatalf("the original cell has a non-zero delta %+v", d)
		}
	}
	if len(report.Cells[1].Thresholds) != 1 || len(report.Summary) != len(cp.Metrics) {
		t.Fatalf("cell thresholds = %d, summary = %d", len(report.Cells[1].Thresholds), len(report.Summary))
	}
}

// TestCompareOriginalCellIsTheReportWithoutTheBlock is the literal form of root
// decision 4: with the learning variants declared, the original cell is a run
// of the protocol without the plasticity block, so its whole run report,
// protocol_hash included, is byte identical to the same comparison declared
// without the block at all.
func TestCompareOriginalCellIsTheReportWithoutTheBlock(t *testing.T) {
	g := fixtureGraph(t)
	reference, err := Compare(context.Background(), g, nil, "",
		learningCompareProtocol(3, nil, false), testLimits())
	if err != nil {
		t.Fatalf("reference compare: %v", err)
	}
	learning, err := Compare(context.Background(), g, nil, "",
		learningCompareProtocol(3, []string{"original", "plastic", "learned_then_frozen"}, true), testLimits())
	if err != nil {
		t.Fatalf("learning compare: %v", err)
	}
	if got, want := mustJSON(t, learning.Cells[0].Run), mustJSON(t, reference.Cells[0].Run); got != want {
		t.Fatalf("the original cell is not the ticket 14 report:\n%s\n%s", got, want)
	}
	if got, want := mustJSON(t, learning.Cells[0].Metrics), mustJSON(t, reference.Cells[0].Metrics); got != want {
		t.Fatalf("the original cell metrics changed:\n%s\n%s", got, want)
	}
	// The null model cell is the ticket 14 cell too, so declaring the learning
	// variants moves nothing but the index.
	last, reference0 := learning.Cells[len(learning.Cells)-1], reference.Cells[len(reference.Cells)-1]
	if got, want := mustJSON(t, last.Run), mustJSON(t, reference0.Run); got != want {
		t.Fatalf("the null model cell changed:\n%s\n%s", got, want)
	}
}

// TestLearnedThenFrozenUsesThePlasticCellsFinalWeights checks the one value the
// report cannot carry: the weight array the frozen cell integrates. It is
// recomputed here from an independent plastic run and compared bit for bit.
func TestLearnedThenFrozenUsesThePlasticCellsFinalWeights(t *testing.T) {
	g := fixtureGraph(t)
	protocol := plasticLIFProtocol(3, 1, 4, true)
	runner, _ := mustRun(t, g, protocol)
	want, _, err := runner.plastic.Effective(runner.params.Weights, runner.params.Signs, runner.PlasticState())
	if err != nil {
		t.Fatalf("effective: %v", err)
	}

	cp := learningCompareProtocol(3, []string{"original", "plastic", "learned_then_frozen"}, true)
	cp.NullModels, cp.Seeds = nil, nil
	report, err := Compare(context.Background(), g, nil, "", cp, testLimits())
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	got := report.Cells[2].Run.Plasticity
	if got == nil || !got.Frozen {
		t.Fatalf("cell 2 plasticity = %+v", got)
	}
	// The frozen cell holds w_eff constant, so a run of the same protocol with
	// those weights as the base parameters reproduces its probe series exactly.
	frozenProtocol := plasticLIFProtocol(3, 1, 4, false)
	parameters := mustParameters(t, g, *frozenProtocol.Uniform)
	parameters.Weights = append([]float64(nil), want...)
	parameters.Hash = parameters.fingerprint()
	fixed, err := Build(context.Background(), g, parameters, frozenProtocol, testLimits())
	if err != nil {
		t.Fatalf("build fixed: %v", err)
	}
	fixedReport, err := fixed.Run(context.Background(), fixed.Stimulus())
	if err != nil {
		t.Fatalf("run fixed: %v", err)
	}
	for _, name := range []string{"first", "all", "spikes", "counted"} {
		if have, expect := probeSeries(t, report.Cells[2].Run, name), probeSeries(t, fixedReport, name); !equalSeries(have, expect) {
			t.Fatalf("frozen probe %q = %v, want %v", name, have, expect)
		}
	}
	if math.Abs(got.PlasticL2After-l2Norm(runner.PlasticState().Plastic)) != 0 {
		t.Fatalf("frozen l2 = %v, want %v", got.PlasticL2After, l2Norm(runner.PlasticState().Plastic))
	}
}

// TestCompareLearningVariantValidation covers the declaration rules the new
// field adds.
func TestCompareLearningVariantValidation(t *testing.T) {
	cases := []struct {
		name  string
		mut   func(*CompareProtocol)
		wants string
	}{
		{"unknown variant", func(c *CompareProtocol) {
			c.LearningVariants = []string{"original", "frozen"}
		}, "frozen"},
		{"duplicate variant", func(c *CompareProtocol) {
			c.LearningVariants = []string{"original", "plastic", "plastic"}
		}, "duplicate"},
		{"original is not first", func(c *CompareProtocol) {
			c.LearningVariants = []string{"plastic", "original"}
		}, "original"},
		{"frozen without plastic", func(c *CompareProtocol) {
			c.LearningVariants = []string{"original", "learned_then_frozen"}
		}, "plastic"},
		{"variants without the block", func(c *CompareProtocol) {
			c.Run.Plasticity = nil
		}, "plasticity"},
		{"block without variants", func(c *CompareProtocol) {
			c.LearningVariants = nil
		}, "learning_variants"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cp := learningCompareProtocol(3, []string{"original", "plastic", "learned_then_frozen"}, true)
			tc.mut(&cp)
			err := cp.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.wants) {
				t.Fatalf("Validate() = %v, want an error naming %q", err, tc.wants)
			}
		})
	}
}
