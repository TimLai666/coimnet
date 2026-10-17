package media

import (
	"math"
	"testing"
)

func TestRequestLossMeanRoundingBoundary146(t *testing.T) {
	// Opposite targets cancel exactly, so the exact gap mean is MaxFloat64.
	a := math.Float64frombits(0x7fc56716e5f76e21)
	for _, tc := range []struct {
		name   string
		target float64
		finite bool
	}{
		{"balanced_targets", a, true},
		{"rounds_to_maximum", math.Nextafter(a, math.Inf(-1)), true},
		{"beyond_finite_rounding", math.Float64frombits(math.Float64bits(a) - 8), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loss, gradient, err := requestLossGradient([]float64{math.MaxFloat64, -a, 0, math.MaxFloat64, tc.target, 0}, 1, 1)
			if tc.finite {
				if err != nil || loss != math.MaxFloat64 || len(gradient) != 6 {
					t.Fatalf("finite rounded mean = %g, gradient length=%d, err=%v; want MaxFloat64", loss, len(gradient), err)
				}
			} else if err == nil || loss != 0 || gradient != nil {
				t.Fatalf("non-finite mean = %g, %v, %v; want zero, nil, error", loss, gradient, err)
			}
		})
	}
}
