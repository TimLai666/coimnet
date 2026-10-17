package ocr

import (
	"math"
	"testing"
)

func TestSingleCharacterGradientTail147(t *testing.T) {
	const (
		// These are the frozen float64 conversions of the independent Decimal180
		// references in root-ocr-gradient-plan.json.
		wantTarget = -0x1.39792499b1a24p-57
		wantOther  = 0x1.39792499b1a24p-58
	)

	for _, tc := range []struct {
		name   string
		target int
	}{
		{name: "winner_1", target: 1},
		{name: "winner_2", target: 2},
		{name: "winner_3", target: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logits := []float64{1000, -40, -40, -40}
			logits[tc.target] = 0
			before := append([]float64(nil), logits...)

			loss, gradient, err := singleCharacterLoss(logits, tc.target)
			if err != nil {
				t.Fatalf("loss/gradient error: loss=%.17g, err=%v", loss, err)
			}
			if len(gradient) != len(logits) {
				t.Fatalf("gradient length=%d, want %d", len(gradient), len(logits))
			}
			if math.IsNaN(loss) || math.IsInf(loss, 0) || loss < 0 {
				t.Errorf("loss=%.17g, want finite nonnegative value", loss)
			}
			for i := range logits {
				if math.Float64bits(logits[i]) != math.Float64bits(before[i]) {
					t.Errorf("input logits[%d]=%.17g, want unchanged %.17g", i, logits[i], before[i])
				}
			}

			for class, got := range gradient {
				if math.IsNaN(got) || math.IsInf(got, 0) {
					t.Errorf("gradient[%d]=%.17g, want finite", class, got)
					continue
				}
				if class == 0 {
					if math.Float64bits(got) != 0 {
						t.Errorf("blank gradient[%d]=%.17g, want exact +0", class, got)
					}
					continue
				}

				want := wantOther
				if class == tc.target {
					want = wantTarget
				}
				if class == tc.target {
					if got >= 0 {
						t.Errorf("gradient[%d] target=true = %.17g, want negative", class, got)
						continue
					}
				} else if got <= 0 {
					t.Errorf("gradient[%d] target=false = %.17g, want positive", class, got)
					continue
				}
				// Sign is checked before the bit distance so zero or opposite-sign
				// values cannot produce a misleading ULP diagnostic.
				gotBits, wantBits := math.Float64bits(got), math.Float64bits(want)
				var ulp uint64
				if gotBits > wantBits {
					ulp = gotBits - wantBits
				} else {
					ulp = wantBits - gotBits
				}
				if ulp > 4 {
					t.Errorf("gradient[%d] target=%t = %.17g, want %.17g within 4 ULP; got_bits=0x%016x want_bits=0x%016x ulp=%d", class, class == tc.target, got, want, gotBits, wantBits, ulp)
				}
			}
		})
	}
}
