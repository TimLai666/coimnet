package experiment

import (
	"math"
	"math/big"
	"reflect"
	"strings"
	"testing"
)

func TestItemMSE134RepresentableLargeTargets(t *testing.T) {
	cases := []struct {
		id     string
		target []float64
	}{
		{id: "two-1e154", target: []float64{1e154, 1e154}},
		{id: "1.4e154-and-zero", target: []float64{1.4e154, 0}},
		{id: "1.6e154-and-two-zeroes", target: []float64{1.6e154, 0, 0}},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			item := EvaluationItem{ID: tc.id, Target: tc.target}
			out := [][]float64{make([]float64, len(tc.target))}
			got, err := itemMSE(out, item)
			if err != nil {
				t.Fatalf("itemMSE returned error: %v", err)
			}
			want := mse134ExactZeroOutput(tc.target)
			if math.IsInf(want, 0) {
				t.Fatalf("independent reference unexpectedly overflowed: %v", want)
			}
			mse134RequireBits(t, got, want)
		})
	}
}

func TestItemMSE134TinySquaredMeanRoundsToMinSubnormal(t *testing.T) {
	target := []float64{math.Ldexp(1, -537), math.Ldexp(1, -538)}
	item := EvaluationItem{ID: "tiny-subnormal", Target: target}
	got, err := itemMSE([][]float64{{0, 0}}, item)
	if err != nil {
		t.Fatalf("itemMSE returned error: %v", err)
	}
	want := mse134ExactZeroOutput(target)
	mse134RequireBits(t, want, math.SmallestNonzeroFloat64)
	mse134RequireBits(t, got, want)
}

func TestItemMSE134MaxFloatSquareOverflowReturnsItemError(t *testing.T) {
	item := EvaluationItem{ID: "max-float-square", Target: []float64{math.MaxFloat64}}
	got, err := itemMSE([][]float64{{0}}, item)
	if got != 0 {
		t.Fatalf("got MSE %v, want zero on true overflow", got)
	}
	if err == nil || !strings.Contains(err.Error(), item.ID) {
		t.Fatalf("error %q does not contain item ID %q", err, item.ID)
	}
}

func TestItemMSE134OrdinaryLastRowMatchesOriginalAndPreservesInput(t *testing.T) {
	out := [][]float64{{7, -8, 9}, {-1.25, 0, 2.5}}
	target := []float64{0.25, -0.5, 1.5}
	item := EvaluationItem{ID: "ordinary-negative-zero-multirow", Target: target}
	outBefore := mse134CloneRows(out)
	targetBefore := append([]float64(nil), target...)
	got, err := itemMSE(out, item)
	if err != nil {
		t.Fatalf("itemMSE returned error: %v", err)
	}
	want := mse134OriginalFloat64(out[len(out)-1], target)
	mse134RequireBits(t, got, want)
	if !reflect.DeepEqual(out, outBefore) || !reflect.DeepEqual(item.Target, targetBefore) {
		t.Fatal("itemMSE modified its input")
	}
}

func TestItemMSE134WidthMismatchReturnsErrorAndPreservesInput(t *testing.T) {
	out := [][]float64{{7, -8}, {1, 2}}
	item := EvaluationItem{ID: "width-mismatch", Target: []float64{0}}
	outBefore := mse134CloneRows(out)
	targetBefore := append([]float64(nil), item.Target...)
	got, err := itemMSE(out, item)
	if got != 0 {
		t.Fatalf("got MSE %v on width mismatch, want zero", got)
	}
	if err == nil || !strings.Contains(err.Error(), item.ID) {
		t.Fatalf("error %q does not contain item ID %q", err, item.ID)
	}
	if !reflect.DeepEqual(out, outBefore) || !reflect.DeepEqual(item.Target, targetBefore) {
		t.Fatal("itemMSE modified its input")
	}
}

func mse134ExactZeroOutput(target []float64) float64 {
	sum := new(big.Rat)
	for _, value := range target {
		rational := new(big.Rat).SetFloat64(value)
		square := new(big.Rat).Mul(rational, rational)
		sum.Add(sum, square)
	}
	mean := new(big.Rat).Quo(sum, new(big.Rat).SetInt64(int64(len(target))))
	result, _ := mean.Float64()
	return result
}

func mse134OriginalFloat64(lastRow, target []float64) float64 {
	var mse float64
	for i, value := range lastRow {
		d := value - target[i]
		mse += d * d
	}
	return mse / float64(len(lastRow))
}

func mse134CloneRows(rows [][]float64) [][]float64 {
	clone := make([][]float64, len(rows))
	for i, row := range rows {
		clone[i] = append([]float64(nil), row...)
	}
	return clone
}

func mse134RequireBits(t *testing.T, got, want float64) {
	t.Helper()
	if math.Float64bits(got) != math.Float64bits(want) {
		t.Fatalf("MSE bits %#x, want %#x (%v)", math.Float64bits(got), math.Float64bits(want), want)
	}
}
