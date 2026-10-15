package dynamics

import "math/big"

// recoverSurrogateDerivative rounds the reciprocal square of a finite den >= 1
// when the ordinary floating-point denominator square has overflowed.
func recoverSurrogateDerivative(den float64) float64 {
	// At this bound the exact slope is at most 2^-1076, below half the
	// smallest float64 subnormal. Avoid big-number work for true underflow.
	if den >= 0x1p538 {
		return 0
	}
	exact := new(big.Rat).SetFloat64(den)
	exact.Mul(exact, exact)
	exact.Inv(exact)
	recovered, _ := exact.Float64()
	return recovered
}
