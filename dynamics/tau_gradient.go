package dynamics

import "math/big"

// membraneTauGradient recovers finite-input overflow; callers retain their original expression for normal rounding.
func membraneTauGradient(adjoint, previous, drive, slope float64) float64 {
	result := adjoint * (previous - drive) * slope
	if finite(result) || !finite(adjoint) || !finite(previous) || !finite(drive) || !finite(slope) {
		return result
	}
	exact := new(big.Rat).SetFloat64(previous)
	exact.Sub(exact, new(big.Rat).SetFloat64(drive))
	exact.Mul(exact, new(big.Rat).SetFloat64(adjoint))
	exact.Mul(exact, new(big.Rat).SetFloat64(slope))
	recovered, _ := exact.Float64()
	return recovered
}
