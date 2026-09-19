package experiment

import (
	"context"
	"math"
	"strings"
	"testing"
)

func TestPairedBootstrapConstantDifferences(t *testing.T) {
	mean, lower, upper := pairedBootstrap([]float64{1, 1, 1}, 0.9, 100, 7)
	for name, v := range map[string]float64{"mean": mean, "lower": lower, "upper": upper} {
		if v != 1 {
			t.Errorf("%s = %g, want 1", name, v)
		}
	}
}

func TestPairedBootstrapBoundsContainTheMean(t *testing.T) {
	diffs := []float64{0, 2, 1, 3}
	mean, lower, upper := pairedBootstrap(diffs, 0.9, 1000, 7)
	if !(lower <= mean && mean <= upper) {
		t.Errorf("bounds %g..%g do not contain mean %g", lower, upper, mean)
	}
	if lower < 0 {
		t.Errorf("lower %g < 0", lower)
	}
	if upper > 3 {
		t.Errorf("upper %g > 3", upper)
	}
	m2, l2, u2 := pairedBootstrap(diffs, 0.9, 1000, 7)
	if m2 != mean || l2 != lower || u2 != upper {
		t.Errorf("same seed not deterministic: (%g,%g,%g) vs (%g,%g,%g)", m2, l2, u2, mean, lower, upper)
	}
	_, lowOther, upOther := pairedBootstrap(diffs, 0.9, 1000, 1)
	if lowOther == lower && upOther == upper {
		t.Errorf("different seed produced identical bounds (%g,%g)", lower, upper)
	}
}

func TestComparisonAbsentWithoutDeclaration(t *testing.T) {
	p := continualFixtureProtocol(5)
	report, err := RunContinualMatrix(context.Background(), p, ContinualFixture)
	if err != nil {
		t.Fatalf("RunContinualMatrix: %v", err)
	}
	if report.Comparison != nil {
		t.Errorf("Comparison = %v, want nil when none is declared", report.Comparison)
	}
	found := false
	for _, a := range report.Assumptions {
		if strings.Contains(a, "no claim") {
			found = true
		}
	}
	if !found {
		t.Errorf("assumptions %v do not state that no superiority claim is made", report.Assumptions)
	}
}

func TestComparisonUsesOnlyPairsWhereBothSucceeded(t *testing.T) {
	protocol := ContinualProtocol{
		Tasks: []TaskSpec{
			{Name: "A", Generator: GeneratorDelayedCorrelation, Params: map[string]float64{"delay": 2, "channel": 0}},
		},
		Stages: []Stage{{Kind: StageTrainTask, Task: "A", Budget: 5}},
		Seeds:  []uint64{1, 2, 3},
	}
	report := ContinualReport{
		Protocol: protocol,
		Seeds: []SeedMatrix{
			{Seed: 1, Scores: [][]float64{{10}}, Failed: [][]bool{{false}}},
			{Seed: 2, Scores: [][]float64{{0}}, Failed: [][]bool{{true}}},
			{Seed: 3, Scores: [][]float64{{24}}, Failed: [][]bool{{false}}},
		},
		Independent: []IndependentResult{
			{Task: "A", Seeds: []SeedScore{
				{Seed: 1, Score: 4}, {Seed: 2, Score: 5}, {Seed: 3, Score: 9},
			}},
		},
	}
	c := PreRegistered{Method: ComparisonPairedBootstrap, Interval: 0.9, Baseline: BaselineIndependent, Resamples: 200}
	res, err := compareToIndependent(report, c)
	if err != nil {
		t.Fatalf("compareToIndependent: %v", err)
	}
	if len(res.Tasks) != 1 {
		t.Fatalf("tasks: got %d want 1", len(res.Tasks))
	}
	if res.Method != ComparisonPairedBootstrap || res.Baseline != BaselineIndependent {
		t.Errorf("method/baseline = %q/%q, want paired_bootstrap/independent", res.Method, res.Baseline)
	}
	if res.Interval != 0.9 || res.Resamples != 200 || res.Seed != 1 {
		t.Errorf("interval/resamples/seed = %g/%d/%d, want 0.9/200/1", res.Interval, res.Resamples, res.Seed)
	}
	tc := res.Tasks[0]
	if tc.Task != "A" {
		t.Errorf("task = %q, want A", tc.Task)
	}
	if tc.Pairs != 2 {
		t.Errorf("pairs = %d, want 2 (seed 2 last-stage row failed)", tc.Pairs)
	}
	if tc.Insufficient {
		t.Error("Insufficient true, want false with 2 pairs")
	}
	if math.Abs(tc.MeanDifference-10.5) > 1e-12 {
		t.Errorf("mean_difference = %g, want 10.5 = ((10-4)+(24-9))/2", tc.MeanDifference)
	}
	if tc.Lower <= 0 || tc.Upper <= 0 {
		t.Errorf("bounds %g..%g, want positive for the resampled differences {6, 15}", tc.Lower, tc.Upper)
	}

	onePair := ContinualReport{
		Protocol: protocol,
		Seeds: []SeedMatrix{
			{Seed: 1, Scores: [][]float64{{10}}, Failed: [][]bool{{false}}},
			{Seed: 2, Scores: [][]float64{{0}}, Failed: [][]bool{{true}}},
			{Seed: 3, Scores: [][]float64{{24}}, Failed: [][]bool{{false}}},
		},
		Independent: []IndependentResult{
			{Task: "A", Seeds: []SeedScore{
				{Seed: 1, Score: 4}, {Seed: 2, Score: 5}, {Seed: 3, Score: 0, Failed: true},
			}},
		},
	}
	res1, err := compareToIndependent(onePair, c)
	if err != nil {
		t.Fatalf("compareToIndependent (one pair): %v", err)
	}
	tc1 := res1.Tasks[0]
	if tc1.Pairs != 1 {
		t.Errorf("pairs = %d, want 1", tc1.Pairs)
	}
	if !tc1.Insufficient {
		t.Error("Insufficient false, want true with 1 pair")
	}
	if tc1.Lower != 0 || tc1.Upper != 0 {
		t.Errorf("bounds %g..%g, want 0 with insufficient pairs", tc1.Lower, tc1.Upper)
	}
}
