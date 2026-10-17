package media

import (
	"math"
	"testing"
)

func TestRequestLossGradientScale147(t *testing.T) {
	// Decimal700 references use the exact binary64 inputs, including -743.65.
	// Two maxima make the normalizer ordinary while the weak class is tiny.
	logits := []float64{0, 0, -743.65, 0, 0, -743.65}
	before := append([]float64(nil), logits...)
	loss, gradient, err := requestLossGradient(logits, 0, 0)
	if err != nil || len(gradient) != 6 || math.IsNaN(loss) || math.IsInf(loss, 0) || loss <= 0 {
		t.Fatalf("loss=%g gradient=%v err=%v", loss, gradient, err)
	}
	if !sameMediaFloat64Slice147(logits, before) {
		t.Fatal("input changed")
	}
	want := []float64{-.25, .25, math.SmallestNonzeroFloat64}
	for i, value := range gradient {
		checkMediaGradientBoundary147(t, value, want[i%3], "mixed-tail gradient")
	}
}
