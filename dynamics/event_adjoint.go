package dynamics

import "math/big"

func (m *LIF) recoverEventAdjoint(synaptic, adaptation, voltage, candidate, derivative float64, smooth bool) float64 {
	if !finite(synaptic) {
		return synaptic
	}
	if m.config.Adaptation.Enabled {
		if !finite(adaptation) {
			return adaptation
		}
		if !finite(m.config.Adaptation.Beta) {
			return m.config.Adaptation.Beta
		}
	}
	if smooth {
		if !finite(voltage) {
			return voltage
		}
		if !finite(candidate) {
			return candidate
		}
		if !finite(m.config.VReset) {
			return m.config.VReset
		}
	}
	if !finite(derivative) {
		return derivative
	}

	total := new(big.Rat).SetFloat64(synaptic)
	if m.config.Adaptation.Enabled {
		term := new(big.Rat).SetFloat64(adaptation)
		term.Mul(term, new(big.Rat).SetFloat64(m.config.Adaptation.Beta))
		total.Add(total, term)
	}
	if smooth {
		term := new(big.Rat).SetFloat64(voltage)
		reset := new(big.Rat).SetFloat64(m.config.VReset)
		reset.Sub(reset, new(big.Rat).SetFloat64(candidate))
		term.Mul(term, reset)
		total.Add(total, term)
	}
	total.Mul(total, new(big.Rat).SetFloat64(derivative))
	recovered, _ := total.Float64()
	return recovered
}
