package ctc_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TimLai666/coimnet/tasks/ocr/ctc"
)

func fixtureLogits() [][]float64 {
	return [][]float64{
		{0.6, -1.2, 0.2},
		{-0.5, 0.8, 0.1},
		{0.3, -0.4, 1.1},
		{-0.2, 0.9, -0.7},
	}
}

func lseOf(row []float64) float64 {
	m := row[0]
	for _, v := range row[1:] {
		if v > m {
			m = v
		}
	}
	s := 0.0
	for _, v := range row {
		s += math.Exp(v - m)
	}
	return m + math.Log(s)
}

func probs(logits [][]float64) [][]float64 {
	y := make([][]float64, len(logits))
	for t, row := range logits {
		lz := lseOf(row)
		y[t] = make([]float64, len(row))
		for k, v := range row {
			y[t][k] = math.Exp(v - lz)
		}
	}
	return y
}

func collapse(path []int) []int {
	merged := make([]int, 0, len(path))
	for _, v := range path {
		if n := len(merged); n > 0 && merged[n-1] == v {
			continue
		}
		merged = append(merged, v)
	}
	out := make([]int, 0, len(merged))
	for _, v := range merged {
		if v != 0 {
			out = append(out, v)
		}
	}
	return out
}

func equalInts(a, b []int) bool {
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

func cloneLogits(logits [][]float64) [][]float64 {
	out := make([][]float64, len(logits))
	for t, row := range logits {
		out[t] = append([]float64(nil), row...)
	}
	return out
}

func enumerateP(logits [][]float64, target []int) float64 {
	y := probs(logits)
	T, C := len(logits), len(logits[0])
	p := 0.0
	path := make([]int, T)
	var walk func(int)
	walk = func(t int) {
		if t == T {
			if equalInts(collapse(path), target) {
				acc := 1.0
				for i, c := range path {
					acc *= y[i][c]
				}
				p += acc
			}
			return
		}
		for c := 0; c < C; c++ {
			path[t] = c
			walk(t + 1)
		}
	}
	walk(0)
	return p
}

func TestLossMatchesPathEnumeration(t *testing.T) {
	logits := fixtureLogits()
	cases := []struct {
		name   string
		target []int
	}{
		{name: "hi two distinct", target: []int{1, 2}},
		{name: "repeated needs blank", target: []int{1, 1}},
		{name: "single label", target: []int{2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := enumerateP(logits, tc.target)
			loss, grad, err := ctc.Loss(logits, tc.target)
			if err != nil {
				t.Fatalf("Loss: unexpected error %v", err)
			}
			const tol = 1e-12
			want := -math.Log(p)
			if math.Abs(loss-want) >= tol {
				t.Fatalf("loss %0.17g != enumeration %0.17g (diff %g)", loss, want, math.Abs(loss-want))
			}
			if len(grad) != len(logits) {
				t.Fatalf("grad has %d rows, want %d", len(grad), len(logits))
			}
			for _, row := range grad {
				if len(row) != len(logits[0]) {
					t.Fatalf("grad row width %d, want %d", len(row), len(logits[0]))
				}
			}
		})
	}
}

func TestGradientMatchesFiniteDifference(t *testing.T) {
	logits := fixtureLogits()
	target := []int{1, 2}
	_, grad, err := ctc.Loss(logits, target)
	if err != nil {
		t.Fatal(err)
	}
	const h = 1e-6
	for ti, row := range logits {
		for k := range row {
			plus := cloneLogits(logits)
			plus[ti][k] += h
			lPlus, _, err := ctc.Loss(plus, target)
			if err != nil {
				t.Fatalf("Loss(+h) t=%d k=%d: %v", ti, k, err)
			}
			minus := cloneLogits(logits)
			minus[ti][k] -= h
			lMinus, _, err := ctc.Loss(minus, target)
			if err != nil {
				t.Fatalf("Loss(-h) t=%d k=%d: %v", ti, k, err)
			}
			est := (lPlus - lMinus) / (2 * h)
			got := grad[ti][k]
			if math.Abs(est) < 1e-8 && math.Abs(got) < 1e-8 {
				continue
			}
			denom := math.Abs(est)
			if math.Abs(got) > denom {
				denom = math.Abs(got)
			}
			if diff := math.Abs(est - got); diff/denom >= 1e-6 {
				t.Errorf("t=%d k=%d: grad %0.12g vs finite-diff %0.12g, rel error %g", ti, k, got, est, diff/denom)
			}
		}
	}
}

func TestRepeatedCharactersNeedABlank(t *testing.T) {
	two := [][]float64{
		{0.5, 0.2, 0.1},
		{-0.3, 0.7, 0.6},
	}
	if _, _, err := ctc.Loss(two, []int{1, 1}); !errors.Is(err, ctc.ErrNoValidPath) {
		t.Fatalf("T=2 target [1,1]: got err %v, want ErrNoValidPath", err)
	}
	three := [][]float64{
		{0.5, 0.2, 0.1},
		{-0.3, 0.7, 0.6},
		{0.4, -0.1, 0.2},
	}
	loss, _, err := ctc.Loss(three, []int{1, 1})
	if err != nil {
		t.Fatalf("T=3 target [1,1]: unexpected error %v", err)
	}
	if math.IsNaN(loss) || math.IsInf(loss, 0) {
		t.Fatalf("T=3 target [1,1]: loss %v is not finite", loss)
	}
}

func TestEmptyTargetIsAllBlanks(t *testing.T) {
	logits := fixtureLogits()
	target := []int{}
	loss, grad, err := ctc.Loss(logits, target)
	if err != nil {
		t.Fatal(err)
	}
	y := probs(logits)
	wantLoss := 0.0
	for ti, row := range logits {
		wantLoss -= row[0] - lseOf(row)
		if math.Abs(grad[ti][0]-(y[ti][0]-1)) >= 1e-12 {
			t.Errorf("grad[%d][0] = %0.17g, want y-1 = %0.17g", ti, grad[ti][0], y[ti][0]-1)
		}
		for k := 1; k < len(row); k++ {
			if math.Abs(grad[ti][k]-y[ti][k]) >= 1e-12 {
				t.Errorf("grad[%d][%d] = %0.17g, want y = %0.17g", ti, k, grad[ti][k], y[ti][k])
			}
		}
	}
	if math.Abs(loss-wantLoss) >= 1e-12 {
		t.Errorf("loss %0.17g, want %0.17g", loss, wantLoss)
	}
}

func TestImpossibleTargetsAreRefusedUnlessZeroOnImpossible(t *testing.T) {
	logits := [][]float64{
		{0.5, 0.2, 0.1},
		{-0.3, 0.7, 0.6},
		{0.4, -0.1, 0.2},
	}
	target := []int{1, 2, 1, 2, 1}
	if _, _, err := ctc.Loss(logits, target); !errors.Is(err, ctc.ErrNoValidPath) {
		t.Fatalf("got err %v, want ErrNoValidPath", err)
	}
	loss, grad, r, err := ctc.LossWith(logits, target, ctc.Options{ZeroOnImpossible: true})
	if err != nil {
		t.Fatalf("ZeroOnImpossible: unexpected error %v", err)
	}
	if !r.Impossible {
		t.Errorf("report.Impossible = false, want true")
	}
	if r.Frames != 3 || r.Labels != 5 {
		t.Errorf("report = %+v, want Frames 3 Labels 5", r)
	}
	if loss != 0 {
		t.Errorf("loss = %v, want 0", loss)
	}
	for ti, row := range grad {
		for k, v := range row {
			if v != 0 {
				t.Errorf("grad[%d][%d] = %v, want 0", ti, k, v)
			}
		}
	}
}

func TestRejectsBadInputs(t *testing.T) {
	good := fixtureLogits()
	cases := []struct {
		name   string
		logits [][]float64
		target []int
	}{
		{name: "zero frames", logits: [][]float64{}, target: nil},
		{name: "single class", logits: [][]float64{{1}, {2}}, target: []int{1}},
		{name: "ragged rows", logits: [][]float64{{0, 1, 2}, {0, 1}}, target: []int{1}},
		{name: "nan", logits: [][]float64{{0, 1}, {math.NaN(), 0.5}}, target: []int{1}},
		{name: "blank in target", logits: good, target: []int{1, 0}},
		{name: "label out of range", logits: good, target: []int{1, 3}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := ctc.Loss(tc.logits, tc.target); err == nil {
				t.Fatalf("expect error for %s", tc.name)
			}
		})
	}
}

func TestDecodeCollapses(t *testing.T) {
	logits := [][]float64{
		{0.0, 2.0, 1.0},
		{0.0, 2.0, 1.0},
		{2.0, 0.0, 1.0},
		{0.0, 2.0, 1.0},
		{1.0, 0.0, 2.0},
		{1.0, 0.0, 2.0},
		{2.0, 0.0, 1.0},
	}
	got, err := ctc.Decode(logits)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{1, 1, 2}; !equalInts(got, want) {
		t.Fatalf("Decode = %v, want %v", got, want)
	}

	allBlank := [][]float64{
		{2.0, 0.0, 1.0},
		{2.0, 0.0, 1.0},
	}
	got, err = ctc.Decode(allBlank)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("all-blank Decode = %v, want length 0", got)
	}

	tie := [][]float64{{1.0, 0.3, 1.0}}
	got, err = ctc.Decode(tie)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("tie Decode = %v, want length 0 (lower index blank wins)", got)
	}

	bad := [][]float64{
		{0.0, 1.0, 2.0},
		{math.NaN(), 0.0, 1.0},
	}
	if _, err := ctc.Decode(bad); err == nil {
		t.Fatal("Decode with NaN: expected error")
	}
}
