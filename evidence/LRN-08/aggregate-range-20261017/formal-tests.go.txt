package experiment_test

import (
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/experiment"
)

// TestAggregateCellsRange136 uses exact float64 rationals and a high-precision
// square root, independently of the production accumulator.
func TestAggregateCellsRange136(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		exact  bool
	}{
		{name: "equal-positive-max", values: []float64{math.MaxFloat64, math.MaxFloat64}, exact: true},
		{name: "equal-negative-max", values: []float64{-math.MaxFloat64, -math.MaxFloat64}, exact: true},
		{name: "repeated-negative-1e308", values: []float64{-1e308, -1e308}},
		{name: "spread-negative-8e307-7e307", values: []float64{-8e307, -7e307}},
		{name: "negative-max-and-two-zeroes", values: []float64{-math.MaxFloat64, 0, 0}},
		{name: "symmetric-max", values: []float64{-math.MaxFloat64, math.MaxFloat64}, exact: true},
		{name: "square-underflow", values: []float64{-1e-200, 1e-200}},
		{name: "square-underflow-at-min-subnormal", values: []float64{-math.SmallestNonzeroFloat64, math.SmallestNonzeroFloat64}, exact: true},
		{name: "variance-below-smallest-normal", values: []float64{-1e-161, 0, 1e-161}},
		{name: "mean-cancellation-with-max", values: []float64{-math.MaxFloat64, 1, math.MaxFloat64}},
		{name: "ordinary-exact", values: []float64{-8, -7}, exact: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			perSeed := make([][][]float64, len(tc.values))
			failed := make([][][]bool, len(tc.values))
			for seed, value := range tc.values {
				perSeed[seed] = [][]float64{{value}}
				failed[seed] = [][]bool{{false}}
			}

			cells, err := experiment.AggregateCells(perSeed, failed)
			if err != nil {
				t.Fatalf("AggregateCells: %v", err)
			}
			if len(cells) != 1 {
				t.Fatalf("cells row count = %d, want 1", len(cells))
			}
			if len(cells[0]) != 1 {
				t.Fatalf("cells column count = %d, want 1", len(cells[0]))
			}
			got := cells[0][0]
			if got.Succeeded != len(tc.values) || got.Total != len(tc.values) {
				t.Fatalf("counts = %d/%d, want %d/%d", got.Succeeded, got.Total, len(tc.values), len(tc.values))
			}

			wantMean, wantStd := exactPopulationStats136(tc.values)
			assertFinite136(t, "mean", got.Mean)
			assertFinite136(t, "std", got.Std)
			assertFinite136(t, "want mean", wantMean)
			assertFinite136(t, "want std", wantStd)
			if tc.exact {
				if math.Float64bits(got.Mean) != math.Float64bits(wantMean) ||
					math.Float64bits(got.Std) != math.Float64bits(wantStd) {
					t.Fatalf("ordinary stats = (%g,%g), want exact (%g,%g)", got.Mean, got.Std, wantMean, wantStd)
				}
			} else {
				assertClose136(t, "mean", got.Mean, wantMean)
				assertClose136(t, "std", got.Std, wantStd)
			}
			wantMin, wantMax := tc.values[0], tc.values[0]
			for _, value := range tc.values[1:] {
				if value < wantMin {
					wantMin = value
				}
				if value > wantMax {
					wantMax = value
				}
			}
			if math.Float64bits(got.Min) != math.Float64bits(wantMin) ||
				math.Float64bits(got.Max) != math.Float64bits(wantMax) {
				t.Fatalf("min/max = (%g,%g), want (%g,%g)", got.Min, got.Max, wantMin, wantMax)
			}
		})
	}
}

func TestAggregateCellsRange136IgnoresFailedNonFinite(t *testing.T) {
	perSeed := [][][]float64{
		{{math.NaN()}},
		{{math.Inf(1)}},
		{{math.Inf(-1)}},
		{{-8}},
		{{-7}},
	}
	failed := [][][]bool{
		{{true}},
		{{true}},
		{{true}},
		{{false}},
		{{false}},
	}
	cells, err := experiment.AggregateCells(perSeed, failed)
	if err != nil {
		t.Fatalf("AggregateCells: %v", err)
	}
	got := cells[0][0]
	if got.Total != 5 || got.Succeeded != 2 {
		t.Fatalf("counts = %d/%d, want 2/5", got.Succeeded, got.Total)
	}
	if got.Mean != -7.5 || got.Std != .5 || got.Min != -8 || got.Max != -7 {
		t.Fatalf("cell = %+v, want mean -7.5 std .5 min -8 max -7", got)
	}
}

func TestAggregateCellsRange136RejectsSuccessfulNonFinite(t *testing.T) {
	for _, value := range []struct {
		name  string
		value float64
	}{
		{name: "nan", value: math.NaN()},
		{name: "positive-inf", value: math.Inf(1)},
		{name: "negative-inf", value: math.Inf(-1)},
	} {
		t.Run(value.name, func(t *testing.T) {
			cells, err := experiment.AggregateCells(
				[][][]float64{{{value.value}}},
				[][][]bool{{{false}}},
			)
			if err == nil {
				t.Fatalf("accepted successful %s with cells %+v", value.name, cells)
			}
			if cells != nil {
				t.Fatalf("returned cells with error: %+v", cells)
			}
			message := strings.ToLower(err.Error())
			if !strings.Contains(message, "seed") ||
				(!strings.Contains(message, "cell") && !strings.Contains(message, "row")) {
				t.Fatalf("error %q lacks seed/cell context", err)
			}
		})
	}
}

func TestAggregateCellsRange136EmptyAndAllFailed(t *testing.T) {
	empty, err := experiment.AggregateCells(nil, nil)
	if err != nil {
		t.Fatalf("empty AggregateCells: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty cells has %d rows, want zero", len(empty))
	}

	allFailed, err := experiment.AggregateCells(
		[][][]float64{{{math.NaN()}}, {{math.Inf(1)}}, {{-8}}},
		[][][]bool{{{true}}, {{true}}, {{true}}},
	)
	if err != nil {
		t.Fatalf("all-failed AggregateCells: %v", err)
	}
	if len(allFailed) != 1 {
		t.Fatalf("all-failed row count = %d, want 1", len(allFailed))
	}
	if len(allFailed[0]) != 1 {
		t.Fatalf("all-failed column count = %d, want 1", len(allFailed[0]))
	}
	if got, want := allFailed[0][0], (experiment.CellStat{Total: 3}); got != want {
		t.Fatalf("all-failed cell = %+v, want %+v", got, want)
	}
}

func TestAggregateCellsRange136RejectsShapeErrors(t *testing.T) {
	cases := []struct {
		name    string
		perSeed [][][]float64
		failed  [][][]bool
	}{
		{
			name:    "seed-count",
			perSeed: [][][]float64{{{1}}},
			failed:  nil,
		},
		{
			name:    "row-count",
			perSeed: [][][]float64{{{1}}, {}},
			failed:  [][][]bool{{{false}}, {}},
		},
		{
			name:    "column-count",
			perSeed: [][][]float64{{{1}}, {{1, 2}}},
			failed:  [][][]bool{{{false}}, {{false, false}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cells, err := experiment.AggregateCells(tc.perSeed, tc.failed)
			if err == nil {
				t.Fatalf("accepted malformed shape with cells %+v", cells)
			}
			if cells != nil {
				t.Fatalf("returned cells with shape error: %+v", cells)
			}
		})
	}
}

func exactPopulationStats136(values []float64) (float64, float64) {
	count := new(big.Rat).SetInt64(int64(len(values)))
	valuesRat := make([]*big.Rat, len(values))
	sum := new(big.Rat)
	for i, value := range values {
		valuesRat[i] = new(big.Rat).SetFloat64(value)
		sum.Add(sum, valuesRat[i])
	}
	mean := new(big.Rat).Quo(sum, count)
	varianceNumerator := new(big.Rat)
	for _, value := range valuesRat {
		delta := new(big.Rat).Sub(value, mean)
		delta.Mul(delta, delta)
		varianceNumerator.Add(varianceNumerator, delta)
	}
	variance := new(big.Rat).Quo(varianceNumerator, count)
	mean64, _ := mean.Float64()
	varianceFloat := new(big.Float).SetPrec(512).SetRat(variance)
	stdFloat := new(big.Float).SetPrec(512)
	stdFloat.Sqrt(varianceFloat)
	std64, _ := stdFloat.Float64()
	return mean64, std64
}

func assertFinite136(t *testing.T, name string, value float64) {
	t.Helper()
	if math.IsNaN(value) || math.IsInf(value, 0) {
		t.Fatalf("%s = %g, want finite", name, value)
	}
}

func assertClose136(t *testing.T, name string, got, want float64) {
	t.Helper()
	// Four relative binary ulps for normal values, with an eight-subnormal-ulp
	// floor near zero.  Both values are checked finite before this comparison.
	scale := math.Max(math.Abs(got), math.Abs(want))
	tolerance := math.Ldexp(scale, -50)
	if floor := 8 * math.SmallestNonzeroFloat64; tolerance < floor {
		tolerance = floor
	}
	delta := math.Abs(got - want)
	if math.IsInf(delta, 0) || math.IsNaN(delta) || delta > tolerance {
		t.Fatalf("%s = %g, want %g (delta %g > tolerance %g)", name, got, want, delta, tolerance)
	}
}
