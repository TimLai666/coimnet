package modulation

import (
	"fmt"
	"math"
)

// ChemistryGradient is the reverse signal for one Kinetics.Step. Matrices
// follow its [region][channel] layout and may contain signed values. Settings,
// transport fractions and clearance boosts are fixed, not learned here.
type ChemistryGradient struct {
	Concentration [][]float64
	Release       [][]float64
}

// Backward differentiates one validated Step, applying the transpose of
// regional transport before local decay and release. Upstream has the same
// shape as the next concentration, is finite and may be signed. Invalid
// forward inputs, invalid upstream or an overflowing gradient return an empty
// result. Inputs and the kinetics are unchanged, so repeated reverse passes
// and recomputation have no source side effects.
func (k *Kinetics) Backward(s ChemistryState, release, clearanceBoost, upstream [][]float64) (ChemistryGradient, error) {
	if _, err := k.Step(s, release, clearanceBoost); err != nil {
		return ChemistryGradient{}, err
	}
	if err := k.checkedUpstream(upstream); err != nil {
		return ChemistryGradient{}, err
	}

	concentration := make([][]float64, k.config.Regions)
	releaseGradient := make([][]float64, k.config.Regions)
	for r := 0; r < k.config.Regions; r++ {
		concentration[r] = make([]float64, k.config.Channels)
		releaseGradient[r] = make([]float64, k.config.Channels)
		for ch := 0; ch < k.config.Channels; ch++ {
			localAdj := upstream[r][ch]
			if k.outflow != nil {
				localAdj *= 1 - k.outflow[r]
				for target := 0; target < k.config.Regions; target++ {
					localAdj += k.config.Transport.Fraction[r][target] * upstream[target][ch]
				}
			}
			if !finite(localAdj) {
				return ChemistryGradient{}, fmt.Errorf("modulation: region %d channel %d transport adjoint is non-finite", r, ch)
			}

			var dConcentration, dRelease float64
			var boost float64
			if clearanceBoost != nil {
				boost = clearanceBoost[r][ch]
			}
			if boost == 0 {
				dConcentration = k.lambda[ch]
				dRelease = k.config.Tau[ch] * k.alpha[ch]
			} else {
				rate := k.rate[ch] + boost
				lambda := math.Exp(-k.config.DT * rate)
				dConcentration = lambda
				dRelease = (1 - lambda) / rate
			}
			if !finite(dConcentration) || !finite(dRelease) {
				return ChemistryGradient{}, fmt.Errorf("modulation: region %d channel %d local gradient coefficient is non-finite", r, ch)
			}

			concentration[r][ch] = localAdj * dConcentration
			releaseGradient[r][ch] = localAdj * dRelease
			if !finite(concentration[r][ch]) || !finite(releaseGradient[r][ch]) {
				return ChemistryGradient{}, fmt.Errorf("modulation: region %d channel %d gradient is non-finite", r, ch)
			}
		}
	}
	return ChemistryGradient{Concentration: concentration, Release: releaseGradient}, nil
}

func (k *Kinetics) checkedUpstream(upstream [][]float64) error {
	if len(upstream) != k.config.Regions {
		return fmt.Errorf("modulation: upstream has %d regions, want %d", len(upstream), k.config.Regions)
	}
	for r, row := range upstream {
		if len(row) != k.config.Channels {
			return fmt.Errorf("modulation: upstream region %d has %d channels, want %d", r, len(row), k.config.Channels)
		}
		for ch, value := range row {
			if !finite(value) {
				return fmt.Errorf("modulation: upstream region %d channel %d is not finite", r, ch)
			}
		}
	}
	return nil
}
