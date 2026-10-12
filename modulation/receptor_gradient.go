package modulation

import (
	"fmt"
	"math"
)

// BackwardOccupancies differentiates Occupancies with fixed receptor curves
// and location mapping. Upstream has one signed finite value per occupancy
// record in the same declaration order. Skipped/unresponsive records have
// zero derivatives; a zero concentration has slope 1/Kd when N=1 and zero
// when N>1. Region/channel contributions add. Errors publish no partial grid.
func (r *Receptors) BackwardOccupancies(state ChemistryState, regionOf []int, upstream []float64) ([][]float64, error) {
	records, _, err := r.Occupancies(state, regionOf)
	if err != nil {
		return nil, err
	}
	if len(upstream) != len(records) {
		return nil, fmt.Errorf("modulation: upstream has %d values, want %d occupancy records", len(upstream), len(records))
	}
	for i, value := range upstream {
		if !finite(value) {
			return nil, fmt.Errorf("modulation: upstream occupancy record %d is not finite", i)
		}
	}

	gradient := make([][]float64, len(state.Concentration))
	channels := len(state.Concentration[0])
	for region := range gradient {
		gradient[region] = make([]float64, channels)
	}

	for i, record := range records {
		if record.Skipped || record.Status == StatusUnresponsive ||
			(record.Status == StatusUnknown && !record.Assumed) {
			continue
		}
		receptor := r.Records[record.Receptor]
		concentration := state.Concentration[record.Region][record.Channel]
		var contribution float64
		if concentration == 0 {
			var slope float64
			if receptor.N == 1 {
				slope = 1 / receptor.Kd
			}
			if !finite(slope) {
				return nil, fmt.Errorf("modulation: occupancy record %d Hill slope is not finite", i)
			}
			contribution = upstream[i] * slope
		} else {
			// d(c^N/(Kd^N+c^N))/dc = N*o*(1-o)/c. Keep the
			// upstream multiplication in the checked chain so an overflowing
			// intermediate is refused rather than silently relying on later
			// factors to bring it back into range.
			contribution = upstream[i] * receptor.N
			if !finite(contribution) {
				return nil, fmt.Errorf("modulation: occupancy record %d Hill gradient numerator is not finite", i)
			}
			contribution *= record.Occupancy
			if !finite(contribution) {
				return nil, fmt.Errorf("modulation: occupancy record %d Hill gradient numerator is not finite", i)
			}
			contribution *= 1 - record.Occupancy
			if !finite(contribution) {
				return nil, fmt.Errorf("modulation: occupancy record %d Hill gradient numerator is not finite", i)
			}
			contribution /= concentration
			if contribution == 0 && upstream[i] != 0 {
				// The occupancy or its product can round to zero even when
				// the Hill derivative is representable. Evaluate that lost
				// magnitude in the log domain; keep ordinary arithmetic above.
				distance := math.Abs(receptor.N * (math.Log(receptor.Kd) - math.Log(concentration)))
				logMagnitude := math.Log(math.Abs(upstream[i])) + math.Log(receptor.N) -
					math.Log(concentration) - distance - 2*math.Log1p(math.Exp(-distance))
				contribution = math.Copysign(math.Exp(logMagnitude), upstream[i])
			}
		}
		if !finite(contribution) {
			return nil, fmt.Errorf("modulation: occupancy record %d gradient contribution is not finite", i)
		}
		value := gradient[record.Region][record.Channel] + contribution
		if !finite(value) {
			return nil, fmt.Errorf("modulation: region %d channel %d gradient is not finite", record.Region, record.Channel)
		}
		gradient[record.Region][record.Channel] = value
	}
	return gradient, nil
}

// BackwardEffects differentiates ApplyEffects with fixed declarations. Every
// upstream array has one signed finite value per node. Results have one value
// per occupancy record; sum mixing adds all contributions, while max mixing
// sends them to the first positive maximum in declaration/record order.
// Clamped values (including a bound itself) have zero derivatives. Invalid
// declarations, shape, nonfinite inputs or gradients return nil and an error.
func BackwardEffects(effects []Effect, occ []OccupancyRecord, mix string, nodes int, gain, offset, threshold []float64) ([]float64, error) {
	forwardGain, forwardOffset, forwardThreshold, _, err := ApplyEffects(effects, occ, mix, nodes)
	if err != nil {
		return nil, err
	}
	if len(gain) != nodes {
		return nil, fmt.Errorf("modulation: gain upstream has %d values, want %d nodes", len(gain), nodes)
	}
	if len(offset) != nodes {
		return nil, fmt.Errorf("modulation: offset upstream has %d values, want %d nodes", len(offset), nodes)
	}
	if len(threshold) != nodes {
		return nil, fmt.Errorf("modulation: threshold upstream has %d values, want %d nodes", len(threshold), nodes)
	}
	for node := 0; node < nodes; node++ {
		if !finite(gain[node]) {
			return nil, fmt.Errorf("modulation: gain upstream at node %d is not finite", node)
		}
		if !finite(offset[node]) {
			return nil, fmt.Errorf("modulation: offset upstream at node %d is not finite", node)
		}
		if !finite(threshold[node]) {
			return nil, fmt.Errorf("modulation: threshold upstream at node %d is not finite", node)
		}
		if !finite(forwardGain[node]) {
			return nil, fmt.Errorf("modulation: gain at node %d is not finite", node)
		}
		if !finite(forwardOffset[node]) {
			return nil, fmt.Errorf("modulation: offset at node %d is not finite", node)
		}
		if !finite(forwardThreshold[node]) {
			return nil, fmt.Errorf("modulation: threshold at node %d is not finite", node)
		}
	}

	declared := make([]Effect, len(effects))
	for i, effect := range effects {
		declared[i], err = checkedEffect(i, effect)
		if err != nil {
			return nil, err
		}
	}
	if mix == "" {
		mix = MixSum
	}

	// These totals and owners are built in the same declaration/record order as
	// ApplyEffects. For MixMax, the winner pair is kept so only the first
	// positive maximum receives a VJP.
	sensitivityTotal := make([]float64, nodes)
	sensitivityOwner := make([]int, nodes)
	sensitivityWinnerEffect := make([]int, nodes)
	sensitivityWinnerRecord := make([]int, nodes)
	thresholdTotal := make([]float64, nodes)
	thresholdOwner := make([]int, nodes)
	thresholdWinnerEffect := make([]int, nodes)
	thresholdWinnerRecord := make([]int, nodes)
	for node := 0; node < nodes; node++ {
		sensitivityOwner[node] = -1
		sensitivityWinnerEffect[node] = -1
		sensitivityWinnerRecord[node] = -1
		thresholdOwner[node] = -1
		thresholdWinnerEffect[node] = -1
		thresholdWinnerRecord[node] = -1
	}

	for effectIndex, effect := range declared {
		for recordIndex, record := range occ {
			if record.Receptor != effect.Receptor {
				continue
			}
			var total []float64
			var owner []int
			var winnerEffect []int
			var winnerRecord []int
			if effect.Kind == EffectSensitivity {
				total, owner = sensitivityTotal, sensitivityOwner
				winnerEffect, winnerRecord = sensitivityWinnerEffect, sensitivityWinnerRecord
			} else {
				total, owner = thresholdTotal, thresholdOwner
				winnerEffect, winnerRecord = thresholdWinnerEffect, thresholdWinnerRecord
			}
			cell := record.Cell
			if owner[cell] < 0 {
				owner[cell] = effectIndex
			} else if !sameCoefficients(declared[owner[cell]], effect) {
				return nil, fmt.Errorf("modulation: effects %d and %d are both %s effects on cell %d and must declare the same coefficients", owner[cell], effectIndex, effect.Kind, cell)
			}
			if mix == MixMax {
				if record.Occupancy > total[cell] {
					total[cell] = record.Occupancy
					winnerEffect[cell] = effectIndex
					winnerRecord[cell] = recordIndex
				}
				continue
			}
			total[cell] += record.Occupancy
			if !finite(total[cell]) {
				return nil, fmt.Errorf("modulation: %s occupancy total at node %d is not finite", effect.Kind, cell)
			}
		}
	}

	gradient := make([]float64, len(occ))
	add := func(recordIndex int, upstreamValue, slope float64, kind string) error {
		contribution := upstreamValue * slope
		if !finite(contribution) {
			return fmt.Errorf("modulation: occupancy record %d %s gradient contribution is not finite", recordIndex, kind)
		}
		value := gradient[recordIndex] + contribution
		if !finite(value) {
			return fmt.Errorf("modulation: occupancy record %d gradient is not finite", recordIndex)
		}
		gradient[recordIndex] = value
		return nil
	}

	if mix == MixSum {
		for _, effect := range declared {
			for recordIndex, record := range occ {
				if record.Receptor != effect.Receptor {
					continue
				}
				cell := record.Cell
				if effect.Kind == EffectSensitivity {
					total := sensitivityTotal[cell]
					gamma := 1 + effect.GammaScale*total
					if interiorBound(gamma, effect.GammaMin, effect.GammaMax) {
						if err := add(recordIndex, gain[cell], effect.GammaScale, "gain"); err != nil {
							return nil, err
						}
					}
					beta := effect.BetaScale * total
					if interiorBound(beta, -effect.BetaAbsMax, effect.BetaAbsMax) {
						if err := add(recordIndex, offset[cell], effect.BetaScale, "offset"); err != nil {
							return nil, err
						}
					}
					continue
				}
				total := thresholdTotal[cell]
				theta := effect.ThetaScale * total
				if interiorBound(theta, -effect.ThetaAbsMax, effect.ThetaAbsMax) {
					if err := add(recordIndex, threshold[cell], effect.ThetaScale, "threshold"); err != nil {
						return nil, err
					}
				}
			}
		}
		return gradient, nil
	}

	for cell, effectIndex := range sensitivityWinnerEffect {
		recordIndex := sensitivityWinnerRecord[cell]
		if effectIndex < 0 || recordIndex < 0 {
			continue
		}
		effect := declared[effectIndex]
		total := sensitivityTotal[cell]
		gamma := 1 + effect.GammaScale*total
		if interiorBound(gamma, effect.GammaMin, effect.GammaMax) {
			if err := add(recordIndex, gain[cell], effect.GammaScale, "gain"); err != nil {
				return nil, err
			}
		}
		beta := effect.BetaScale * total
		if interiorBound(beta, -effect.BetaAbsMax, effect.BetaAbsMax) {
			if err := add(recordIndex, offset[cell], effect.BetaScale, "offset"); err != nil {
				return nil, err
			}
		}
	}
	for cell, effectIndex := range thresholdWinnerEffect {
		recordIndex := thresholdWinnerRecord[cell]
		if effectIndex < 0 || recordIndex < 0 {
			continue
		}
		effect := declared[effectIndex]
		total := thresholdTotal[cell]
		theta := effect.ThetaScale * total
		if interiorBound(theta, -effect.ThetaAbsMax, effect.ThetaAbsMax) {
			if err := add(recordIndex, threshold[cell], effect.ThetaScale, "threshold"); err != nil {
				return nil, err
			}
		}
	}
	return gradient, nil
}

func interiorBound(value, low, high float64) bool {
	return finite(value) && value > low && value < high
}
