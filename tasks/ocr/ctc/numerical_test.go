package ctc_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TimLai666/coimnet/tasks/ocr/ctc"
)

func directCTCProbabilities(base [][]float64) [][]float64 {
	prob := make([][]float64, len(base))
	for t, row := range base {
		prob[t] = make([]float64, len(row))
		sum := 0.0
		for _, v := range row {
			sum += math.Exp(v)
		}
		for k, v := range row {
			prob[t][k] = math.Exp(v) / sum
		}
	}
	return prob
}

func numericalCTCCollapse(path []int) []int {
	collapsed := make([]int, 0, len(path))
	for _, v := range path {
		if len(collapsed) == 0 || collapsed[len(collapsed)-1] != v {
			collapsed = append(collapsed, v)
		}
	}
	out := make([]int, 0, len(collapsed))
	for _, v := range collapsed {
		if v != 0 {
			out = append(out, v)
		}
	}
	return out
}

func equalNumericalLabels(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// enumerateCTC computes the target probability and conditional frame/class
// occupancy by enumerating every C^T path. Probabilities come directly from
// the small base logits, so this reference does not reproduce log-space LSE.
func enumerateCTC(base [][]float64, target []int) (float64, [][]float64) {
	prob := directCTCProbabilities(base)
	T, classes := len(base), len(base[0])
	occupancy := make([][]float64, T)
	for t := range occupancy {
		occupancy[t] = make([]float64, classes)
	}
	path := make([]int, T)
	targetProbability := 0.0
	var walk func(int)
	walk = func(t int) {
		if t == T {
			if !equalNumericalLabels(numericalCTCCollapse(path), target) {
				return
			}
			pathProbability := 1.0
			for i, k := range path {
				pathProbability *= prob[i][k]
			}
			targetProbability += pathProbability
			for i, k := range path {
				occupancy[i][k] += pathProbability
			}
			return
		}
		for k := 0; k < classes; k++ {
			path[t] = k
			walk(t + 1)
		}
	}
	walk(0)
	for t := range occupancy {
		for k := range occupancy[t] {
			occupancy[t][k] /= targetProbability
		}
	}
	grad := make([][]float64, T)
	for t := range grad {
		grad[t] = make([]float64, classes)
		for k := range grad[t] {
			grad[t][k] = prob[t][k] - occupancy[t][k]
		}
	}
	return -math.Log(targetProbability), grad
}

func shiftedCTCLogits(base [][]float64, shift float64) [][]float64 {
	shifted := make([][]float64, len(base))
	for t, row := range base {
		shifted[t] = make([]float64, len(row))
		for k, v := range row {
			shifted[t][k] = shift + v
		}
	}
	return shifted
}

func assertCTCNumericalClose(t *testing.T, got, want float64, label string) {
	t.Helper()
	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Fatalf("%s = %v, want finite value near %.17g", label, got, want)
	}
	if math.Abs(got-want) > 2e-12 {
		t.Fatalf("%s = %.17g, want %.17g (diff %.3g)", label, got, want, math.Abs(got-want))
	}
}

func assertCTCBits(t *testing.T, got, want [][]float64, label string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s rows = %d, want %d", label, len(got), len(want))
	}
	for i := range want {
		if len(got[i]) != len(want[i]) {
			t.Fatalf("%s row %d width = %d, want %d", label, i, len(got[i]), len(want[i]))
		}
		for k := range want[i] {
			if math.Float64bits(got[i][k]) != math.Float64bits(want[i][k]) {
				t.Fatalf("%s[%d][%d] bits = %#x, want %#x", label, i, k, math.Float64bits(got[i][k]), math.Float64bits(want[i][k]))
			}
		}
	}
}

func TestCTCNumericalSingleFrameHandValue(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value float64
	}{
		{name: "zero", value: 0},
		{name: "plus_1e16", value: 1e16},
		{name: "minus_1e16", value: -1e16},
		{name: "plus_1e308", value: 1e308},
		{name: "minus_1e308", value: -1e308},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logits := [][]float64{{tc.value, tc.value}}
			before := [][]float64{{tc.value, tc.value}}
			loss, grad, err := ctc.Loss(logits, []int{1})
			if err != nil {
				t.Fatalf("Loss = %v, want nil", err)
			}
			assertCTCNumericalClose(t, loss, math.Ln2, "single-frame loss")
			if len(grad) != 1 || len(grad[0]) != 2 {
				t.Fatalf("single-frame grad shape = %v, want 1x2", grad)
			}
			assertCTCNumericalClose(t, grad[0][0], 0.5, "single-frame grad[0][0]")
			assertCTCNumericalClose(t, grad[0][1], -0.5, "single-frame grad[0][1]")
			assertCTCBits(t, logits, before, "single-frame logits")
		})
	}
}

func TestCTCNumericalShiftMatchesEnumeratedPaths(t *testing.T) {
	base := [][]float64{
		{0, 2, 4},
		{4, 0, 2},
		{2, 4, 0},
		{0, 2, 4},
	}
	shifted := shiftedCTCLogits(base, 1e16)
	before := shiftedCTCLogits(base, 1e16)
	for _, tc := range []struct {
		name   string
		target []int
	}{
		{name: "empty", target: []int{}},
		{name: "repeated", target: []int{1, 1}},
		{name: "distinct", target: []int{1, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantLoss, wantGrad := enumerateCTC(base, tc.target)
			loss, grad, report, err := ctc.LossWith(shifted, tc.target, ctc.Options{})
			if err != nil {
				t.Fatalf("LossWith = %v, want nil", err)
			}
			assertCTCNumericalClose(t, loss, wantLoss, "loss")
			if len(grad) != len(wantGrad) {
				t.Fatalf("grad rows = %d, want %d", len(grad), len(wantGrad))
			}
			for i := range wantGrad {
				if len(grad[i]) != len(wantGrad[i]) {
					t.Fatalf("grad row %d width = %d, want %d", i, len(grad[i]), len(wantGrad[i]))
				}
				for k := range wantGrad[i] {
					assertCTCNumericalClose(t, grad[i][k], wantGrad[i][k], "grad")
				}
			}
			if report != (ctc.Report{Frames: 4, Labels: len(tc.target)}) {
				t.Fatalf("report = %+v, want frames=4 labels=%d", report, len(tc.target))
			}
			if tc.name == "distinct" {
				loss2, grad2, err := ctc.Loss(shifted, tc.target)
				if err != nil {
					t.Fatalf("Loss wrapper = %v, want nil", err)
				}
				assertCTCNumericalClose(t, loss2, wantLoss, "Loss wrapper loss")
				for i := range grad2 {
					for k := range grad2[i] {
						assertCTCNumericalClose(t, grad2[i][k], wantGrad[i][k], "Loss wrapper grad")
					}
				}
			}
		})
	}
	assertCTCBits(t, shifted, before, "shifted logits")
}

func TestCTCNumericalImpossiblePolicies(t *testing.T) {
	logits := [][]float64{{0, 2, 4}, {4, 0, 2}}
	before := shiftedCTCLogits(logits, 0)
	target := []int{1, 1}
	if loss, grad, err := ctc.Loss(logits, target); !errors.Is(err, ctc.ErrNoValidPath) || loss != 0 || grad != nil {
		t.Fatalf("Loss impossible = loss %v, grad %v, err %v; want cleared ErrNoValidPath", loss, grad, err)
	}
	loss, grad, report, err := ctc.LossWith(logits, target, ctc.Options{ZeroOnImpossible: true})
	if err != nil {
		t.Fatalf("ZeroOnImpossible LossWith = %v, want nil", err)
	}
	if loss != 0 || !report.Impossible || report.Frames != 2 || report.Labels != 2 {
		t.Fatalf("ZeroOnImpossible result = loss %v report %+v, want zero and Impossible", loss, report)
	}
	if len(grad) != 2 || len(grad[0]) != 3 || len(grad[1]) != 3 {
		t.Fatalf("ZeroOnImpossible grad shape = %v, want 2x3", grad)
	}
	for i := range grad {
		for k, v := range grad[i] {
			if v != 0 {
				t.Fatalf("ZeroOnImpossible grad[%d][%d] = %v, want 0", i, k, v)
			}
		}
	}
	assertCTCBits(t, logits, before, "impossible logits")
}

func TestCTCNumericalRejectsNonFiniteAndOverflow(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value float64
	}{
		{name: "nan", value: math.NaN()},
		{name: "plus inf", value: math.Inf(1)},
		{name: "minus inf", value: math.Inf(-1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logits := [][]float64{{tc.value, 0}}
			before := [][]float64{{tc.value, 0}}
			loss, grad, err := ctc.Loss(logits, []int{1})
			if err == nil {
				t.Fatalf("Loss = nil, want error for %s", tc.name)
			}
			if loss != 0 || grad != nil {
				t.Fatalf("error result = loss %v grad %v, want cleared outputs", loss, grad)
			}
			assertCTCBits(t, logits, before, "non-finite logits")
		})
	}

	t.Run("normalization_overflow", func(t *testing.T) {
		logits := [][]float64{{math.MaxFloat64, -math.MaxFloat64}}
		before := [][]float64{{math.MaxFloat64, -math.MaxFloat64}}
		loss, grad, err := ctc.Loss(logits, []int{1})
		if err == nil {
			t.Fatal("Loss with an overflowing normalization difference = nil, want error")
		}
		if loss != 0 || grad != nil {
			t.Fatalf("overflow result = loss %v grad %v, want cleared outputs", loss, grad)
		}
		assertCTCBits(t, logits, before, "overflow logits")
	})

	t.Run("result_overflow", func(t *testing.T) {
		resultOverflow := [][]float64{{0, -1e308}, {0, -1e308}, {0, -1e308}}
		resultBefore := shiftedCTCLogits(resultOverflow, 0)
		loss, grad, report, err := ctc.LossWith(resultOverflow, []int{1, 1}, ctc.Options{})
		if err == nil {
			t.Fatal("LossWith with an overflowing three-frame result = nil, want error")
		}
		if loss != 0 || grad != nil || report != (ctc.Report{}) {
			t.Fatalf("result overflow = loss %v grad %v report %+v, want cleared outputs", loss, grad, report)
		}
		assertCTCBits(t, resultOverflow, resultBefore, "result overflow logits")
	})
}
