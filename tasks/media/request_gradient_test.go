package media

import (
	"fmt"
	"math"
	"testing"
)

// These are the frozen float64 conversions of the independent Decimal160
// references in root-gradient-plan.json. The media loss divides each group's
// derivative by two.
const (
	mediaGradientWinner147 = -0x1.39792499b1a24p-58
	mediaGradientOther147  = 0x1.39792499b1a24p-59
)

func gradientULPDistance147(got, want float64) uint64 {
	gotBits, wantBits := math.Float64bits(got), math.Float64bits(want)
	if gotBits > wantBits {
		return gotBits - wantBits
	}
	return wantBits - gotBits
}

func checkMediaGradientEntry147(t *testing.T, index int, got, want float64, target bool) {
	t.Helper()
	gotBits, wantBits := math.Float64bits(got), math.Float64bits(want)
	ulp := gradientULPDistance147(got, want)
	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Errorf("gradient[%d] target=%t = %.17g, want finite %.17g; got_bits=0x%016x want_bits=0x%016x ulp=%d", index, target, got, want, gotBits, wantBits, ulp)
		return
	}
	if target && got >= 0 {
		t.Errorf("gradient[%d] target=%t = %.17g, want negative; got_bits=0x%016x want_bits=0x%016x ulp=%d", index, target, got, gotBits, wantBits, ulp)
	}
	if !target && got <= 0 {
		t.Errorf("gradient[%d] target=%t = %.17g, want positive; got_bits=0x%016x want_bits=0x%016x ulp=%d", index, target, got, gotBits, wantBits, ulp)
	}
	if ulp > 4 {
		t.Errorf("gradient[%d] target=%t = %.17g, want %.17g within 4 ULP; got_bits=0x%016x want_bits=0x%016x ulp=%d", index, target, got, want, gotBits, wantBits, ulp)
	}
}

func TestRequestLossGradientTail147(t *testing.T) {
	for _, winner := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("winner_%d", winner), func(t *testing.T) {
			logits := make([]float64, 6)
			for i := range logits {
				logits[i] = -40
			}
			logits[winner], logits[3+winner] = 0, 0
			loss, gradient, err := requestLossGradient(logits, winner, winner)
			if err != nil {
				t.Fatalf("loss/gradient error: loss=%.17g, err=%v", loss, err)
			}
			if len(gradient) != len(logits) {
				t.Fatalf("gradient length=%d, want %d", len(gradient), len(logits))
			}
			if math.IsNaN(loss) || math.IsInf(loss, 0) || loss < 0 {
				t.Errorf("loss=%.17g, want finite nonnegative value", loss)
			}
			for i, got := range gradient {
				target := i%3 == winner
				want := mediaGradientOther147
				if target {
					want = mediaGradientWinner147
				}
				checkMediaGradientEntry147(t, i, got, want, target)
			}
		})
	}
}
