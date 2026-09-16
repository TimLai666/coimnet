package modulation

import "fmt"

// The two effect kinds. A sensitivity effect changes the input current of a
// cell for one step; a threshold effect changes its effective threshold for one
// step. Neither ever writes back into a learnable parameter.
const (
	EffectSensitivity = "sensitivity"
	EffectThreshold   = "threshold"
)

// The bounds an effect gets when it declares none. A zero bound means "use the
// default", so the defaults cannot be declared away by accident; ThetaAbsMax
// has no default and a threshold effect must declare it.
const (
	DefaultGammaMin   = 0.5
	DefaultGammaMax   = 2
	DefaultBetaAbsMax = 1
)

// Effect maps the occupancy of one receptor onto a bounded per-step quantity.
// A sensitivity effect reads GammaScale, BetaScale and the gamma and beta
// bounds; a threshold effect reads ThetaScale and ThetaAbsMax. Declaring a
// field the kind never reads is refused rather than ignored.
type Effect struct {
	Kind        string  `json:"kind"`
	Receptor    int     `json:"receptor"`
	GammaScale  float64 `json:"gamma_scale,omitempty"`
	BetaScale   float64 `json:"beta_scale,omitempty"`
	GammaMin    float64 `json:"gamma_min,omitempty"`
	GammaMax    float64 `json:"gamma_max,omitempty"`
	BetaAbsMax  float64 `json:"beta_abs_max,omitempty"`
	ThetaScale  float64 `json:"theta_scale,omitempty"`
	ThetaAbsMax float64 `json:"theta_abs_max,omitempty"`
}

// ClampReport counts how many per-node values each bound had to hold back in
// one call, so saturation is reported rather than silent.
type ClampReport struct {
	Gamma int `json:"gamma"`
	Beta  int `json:"beta"`
	Theta int `json:"theta"`
}

// effectGroup accumulates the occupancy that reaches each cell through one kind
// of effect, and remembers which effect first reached that cell so that two
// effects of the same kind cannot quietly disagree about the coefficients.
type effectGroup struct {
	occupancy []float64
	owner     []int
}

func newEffectGroup(nodes int) *effectGroup {
	g := &effectGroup{occupancy: make([]float64, nodes), owner: make([]int, nodes)}
	for i := range g.owner {
		g.owner[i] = -1
	}
	return g
}

// ApplyEffects turns one step of occupancies into the per-node gain, offset and
// threshold arrays a core reads. It starts from the neutral arrays (gain 1,
// offset 0, threshold 0) and touches only the cells an effect reaches.
//
// Every occupancy record of the receptor an effect names contributes to that
// cell's total for that kind of effect, mixed by mix: MixSum adds them, MixMax
// keeps the largest. An empty mix means MixSum. The totals then become
//
//	gamma = clamp(1 + GammaScale*occ, GammaMin, GammaMax)
//	beta  = clamp(BetaScale*occ, -BetaAbsMax, BetaAbsMax)
//	m     = clamp(ThetaScale*occ, -ThetaAbsMax, ThetaAbsMax)
//
// and each bound that held a value back is counted in the ClampReport.
//
// With every occupancy at zero the result is exactly the neutral arrays, so a
// declared effect with no chemical present is bit for bit the same as declaring
// no effect at all.
//
// The formulas name one set of coefficients per cell, so two effects of the
// same kind that reach the same cell must declare the same coefficients;
// disagreeing declarations are refused rather than resolved by an invented
// rule. Effects of the same kind on cells that never meet are unaffected.
func ApplyEffects(effects []Effect, occ []OccupancyRecord, mix string, nodes int) (gain, offset, threshold []float64, clamped ClampReport, err error) {
	switch mix {
	case "":
		mix = MixSum
	case MixSum, MixMax:
	default:
		return nil, nil, nil, ClampReport{}, fmt.Errorf("modulation: unsupported effect mix %q", mix)
	}
	if nodes < 1 {
		return nil, nil, nil, ClampReport{}, fmt.Errorf("modulation: effects need at least one node, got %d nodes", nodes)
	}
	for i, record := range occ {
		if record.Cell < 0 || record.Cell >= nodes {
			return nil, nil, nil, ClampReport{}, fmt.Errorf("modulation: occupancy record %d names cell %d, outside [0, %d)", i, record.Cell, nodes)
		}
		if !finite(record.Occupancy) || record.Occupancy < 0 || record.Occupancy > 1 {
			return nil, nil, nil, ClampReport{}, fmt.Errorf("modulation: occupancy record %d has occupancy %v, outside [0, 1]", i, record.Occupancy)
		}
	}
	declared := make([]Effect, len(effects))
	var sensitivity, threshholds *effectGroup
	for i, effect := range effects {
		if declared[i], err = checkedEffect(i, effect); err != nil {
			return nil, nil, nil, ClampReport{}, err
		}
		switch declared[i].Kind {
		case EffectSensitivity:
			if sensitivity == nil {
				sensitivity = newEffectGroup(nodes)
			}
		case EffectThreshold:
			if threshholds == nil {
				threshholds = newEffectGroup(nodes)
			}
		}
	}
	for i, effect := range declared {
		group := sensitivity
		if effect.Kind == EffectThreshold {
			group = threshholds
		}
		for _, record := range occ {
			if record.Receptor != effect.Receptor {
				continue
			}
			owner := group.owner[record.Cell]
			if owner < 0 {
				group.owner[record.Cell] = i
			} else if !sameCoefficients(declared[owner], effect) {
				return nil, nil, nil, ClampReport{}, fmt.Errorf("modulation: effects %d and %d are both %s effects on cell %d and must declare the same coefficients", owner, i, effect.Kind, record.Cell)
			}
			if mix == MixMax {
				if record.Occupancy > group.occupancy[record.Cell] {
					group.occupancy[record.Cell] = record.Occupancy
				}
				continue
			}
			group.occupancy[record.Cell] += record.Occupancy
		}
	}

	gain, offset, threshold = make([]float64, nodes), make([]float64, nodes), make([]float64, nodes)
	for i := range gain {
		gain[i] = 1
	}
	if sensitivity != nil {
		for cell, owner := range sensitivity.owner {
			if owner < 0 {
				continue
			}
			effect, total := declared[owner], sensitivity.occupancy[cell]
			gamma, held := clamped64(1+effect.GammaScale*total, effect.GammaMin, effect.GammaMax)
			if held {
				clamped.Gamma++
			}
			gain[cell] = gamma
			beta, held := clamped64(effect.BetaScale*total, -effect.BetaAbsMax, effect.BetaAbsMax)
			if held {
				clamped.Beta++
			}
			offset[cell] = positiveZero(beta)
		}
	}
	if threshholds != nil {
		for cell, owner := range threshholds.owner {
			if owner < 0 {
				continue
			}
			effect, total := declared[owner], threshholds.occupancy[cell]
			m, held := clamped64(effect.ThetaScale*total, -effect.ThetaAbsMax, effect.ThetaAbsMax)
			if held {
				clamped.Theta++
			}
			threshold[cell] = positiveZero(m)
		}
	}
	return gain, offset, threshold, clamped, nil
}

// checkedEffect validates one declaration and fills in the bounds it left at
// zero. A field the kind never reads would be a knob with no effect, so it is
// refused rather than ignored.
func checkedEffect(i int, e Effect) (Effect, error) {
	switch e.Kind {
	case EffectSensitivity:
		if e.ThetaScale != 0 || e.ThetaAbsMax != 0 {
			return Effect{}, fmt.Errorf("modulation: effect %d is a %s effect and never reads theta_scale or theta_abs_max", i, e.Kind)
		}
		if e.GammaMin == 0 {
			e.GammaMin = DefaultGammaMin
		}
		if e.GammaMax == 0 {
			e.GammaMax = DefaultGammaMax
		}
		if e.BetaAbsMax == 0 {
			e.BetaAbsMax = DefaultBetaAbsMax
		}
		if !finite(e.GammaScale) || !finite(e.BetaScale) {
			return Effect{}, fmt.Errorf("modulation: effect %d needs finite gamma_scale and beta_scale", i)
		}
		if !finite(e.GammaMin) || !finite(e.GammaMax) || e.GammaMin <= 0 || e.GammaMin > e.GammaMax {
			return Effect{}, fmt.Errorf("modulation: effect %d needs finite gamma_min and gamma_max with 0 < gamma_min <= gamma_max, got %v and %v", i, e.GammaMin, e.GammaMax)
		}
		if !finite(e.BetaAbsMax) || e.BetaAbsMax < 0 {
			return Effect{}, fmt.Errorf("modulation: effect %d needs a finite non-negative beta_abs_max, got %v", i, e.BetaAbsMax)
		}
	case EffectThreshold:
		if e.GammaScale != 0 || e.BetaScale != 0 || e.GammaMin != 0 || e.GammaMax != 0 || e.BetaAbsMax != 0 {
			return Effect{}, fmt.Errorf("modulation: effect %d is a %s effect and never reads gamma_scale, beta_scale, gamma_min, gamma_max or beta_abs_max", i, e.Kind)
		}
		if !finite(e.ThetaScale) {
			return Effect{}, fmt.Errorf("modulation: effect %d needs a finite theta_scale", i)
		}
		if !finite(e.ThetaAbsMax) || e.ThetaAbsMax <= 0 {
			return Effect{}, fmt.Errorf("modulation: effect %d must declare a finite positive theta_abs_max, got %v", i, e.ThetaAbsMax)
		}
	default:
		return Effect{}, fmt.Errorf("modulation: unsupported effect kind %q", e.Kind)
	}
	if e.Receptor < 0 {
		return Effect{}, fmt.Errorf("modulation: effect %d names receptor %d, which is negative", i, e.Receptor)
	}
	return e, nil
}

// sameCoefficients compares two validated effects on everything but which
// receptor drives them.
func sameCoefficients(a, b Effect) bool {
	a.Receptor, b.Receptor = 0, 0
	return a == b
}

// clamped64 holds a value inside a closed interval and says whether it had to.
func clamped64(v, low, high float64) (float64, bool) {
	if v < low {
		return low, true
	}
	if v > high {
		return high, true
	}
	return v, false
}

// positiveZero keeps a zero positive so that a neutral result cannot differ
// from an unmodulated one in the sign of a zero.
func positiveZero(v float64) float64 {
	if v == 0 {
		return 0
	}
	return v
}
