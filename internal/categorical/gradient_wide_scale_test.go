package categorical

import (
	"math"
	"testing"
)

// Exact binary64-input Decimal1000 references are preserved in
// categorical-wide-scale-reference.json. Nonzero answers must remain nonzero.
func TestGradientWideScale147(t *testing.T) {
	tests := []struct {
		name                 string
		gap, factor, divisor float64
		target               int
		bits                 [3]uint64
	}{
		{name: "large_factor_gap1400_winner_target", gap: -1400.0, factor: 1e+308, divisor: 1.0, target: 0, bits: [3]uint64{0x81b4d53ce9fce1c1, 0x01a4d53ce9fce1c1, 0x01a4d53ce9fce1c1}},
		{name: "large_factor_gap1400_weak_target", gap: -1400.0, factor: 1e+308, divisor: 1.0, target: 1, bits: [3]uint64{0x7fe1ccf385ebc8a0, 0xffe1ccf385ebc8a0, 0x01a4d53ce9fce1c1}},
		{name: "small_final_gap1200_factor1e200", gap: -1200.0, factor: 1e+200, divisor: 3.0, target: 0, bits: [3]uint64{0x800000000000005f, 0x000000000000002f, 0x000000000000002f}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logits := []float64{0, tc.gap, tc.gap}
			dst := make([]float64, len(logits))
			if err := Gradient(dst, logits, tc.target, tc.factor, tc.divisor); err != nil {
				t.Fatalf("Gradient: %v", err)
			}
			for i, got := range dst {
				checkCategoricalGradientValue147(t, got, math.Float64frombits(tc.bits[i]), 8, "wide-scale derivative")
			}
		})
	}
}
