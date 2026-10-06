package rl

import (
	"math"
	"testing"
)

// The weighted loss and its derivative can remain finite even when computing
// the unweighted square or doubling the coefficient first would overflow.
func TestObjectiveNumericalWeightedValueExtremesRemainFinite(t *testing.T) {
	cases := []struct {
		name                                string
		coef, value, wantLoss, wantGradient float64
	}{
		{"tiny_coefficient_large_value", 1e-308, 1e200, 1e92, 2e-108},
		{"large_coefficient_zero_difference", math.MaxFloat64, 0, 0, 0},
		{"large_coefficient_small_difference", math.MaxFloat64, 0.25, math.MaxFloat64 / 16, math.MaxFloat64 / 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := objectiveNumericalConfig()
			c.ValueCoef, c.EntropyCoef = tc.coef, 0
			st, grad, valueGrad, err := Loss([]float64{0, 0}, 0, -math.Ln2, 0, tc.value, 0, c)
			if err != nil {
				t.Fatal(err)
			}
			objectiveNumericalNear(t, "value loss", st.Value, tc.wantLoss)
			objectiveNumericalNear(t, "total loss", st.Loss, tc.wantLoss)
			objectiveNumericalNear(t, "value gradient", valueGrad, tc.wantGradient)
			if tc.wantGradient != 0 {
				objectiveNumericalNear(t, "scaled value gradient", valueGrad/tc.wantGradient, 1)
			}
			if len(grad) != 2 || grad[0] != 0 || grad[1] != 0 {
				t.Fatalf("policy gradient=%v, want [0 0]", grad)
			}
		})
	}
}
