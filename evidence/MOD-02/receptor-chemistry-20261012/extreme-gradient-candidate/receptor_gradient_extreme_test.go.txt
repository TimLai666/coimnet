package modulation

import (
	"math"
	"testing"
)

// Extreme Hill derivatives use independent closed-form limits. A representable
// slope must survive a forward occupancy rounded to zero or one. Normal
// concentrations retain the established arithmetic, and genuine gradient
// overflow remains an error. This file is owned by root; implementation workers
// may not modify these expectations.
func TestReceptorGradientExtremeConcentration(t *testing.T) {
	cases := []struct {
		name                     string
		c, kd, n, upstream, want float64
	}{
		{"tiny-linear", 1e-310, 2, 1, 1, 0.5},
		{"tiny-linear-scaled", 1e-300, 2, 1, 1e-100, 5e-101},
		{"tiny-quadratic", 1e-300, 1, 2, 1, 2e-300},
		{"tiny-cubic", 1e-150, 1, 3, 1, 3e-300},
		{"saturated-linear", 1e20, 2, 1, 1, 2e-40},
		{"signed", 1e-310, 2, 1, -3, -1.5},
		{"zero-upstream", 1e-310, 2, 1, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			receptors := &Receptors{Records: []Receptor{
				gradientReceptor([]int{0}, 0, StatusHypothesized, tc.kd, tc.n, nil),
			}}
			got, err := receptors.BackwardOccupancies(
				ChemistryState{Concentration: [][]float64{{tc.c}}}, []int{0}, []float64{tc.upstream})
			if err != nil {
				t.Fatal(err)
			}
			gradientAssertMatrixShape(t, got, 1, 1)
			value := got[0][0]
			// Scale by the expectation itself; an absolute tolerance near one
			// would incorrectly accept zero for the tiny nonzero slopes.
			if math.IsNaN(value) || math.IsInf(value, 0) ||
				(tc.want == 0 && value != 0) ||
				(tc.want != 0 && math.Abs(value/tc.want-1) > 2e-12) {
				t.Fatalf("gradient = %.17g, want %.17g", value, tc.want)
			}
		})
	}
}
