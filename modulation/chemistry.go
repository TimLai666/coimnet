package modulation

import (
	"fmt"
	"math"
)

// UnitNormalized is the unit a chemistry declares when it names none. It is a
// label, not a conversion: the framework never rescales a concentration, and a
// declaration that names another unit produces exactly the same numbers.
const UnitNormalized = "normalized_unit"

// Transport declares how much of a region's concentration moves to another
// region in one step. Fraction[r][s] is the share that leaves region r for
// region s, so the diagonal is zero and each row sums to at most one. Transport
// is chemical data, declared by the researcher; it is never derived from the
// wiring.
type Transport struct {
	Fraction [][]float64 `json:"fraction"`
}

// Chemistry declares the concentration layer: how many regions and channels
// there are, the step size, one positive time constant per channel, the unit
// the numbers are declared in and an optional regional transport.
type Chemistry struct {
	Regions   int        `json:"regions"`
	Channels  int        `json:"channels"`
	DT        float64    `json:"dt"`
	Tau       []float64  `json:"tau"`
	Units     string     `json:"units"`
	Transport *Transport `json:"transport,omitempty"`
}

// ChemistryState is the concentration of every channel in every region and the
// number of steps that produced it. Every entry is finite and non-negative.
type ChemistryState struct {
	Concentration [][]float64 `json:"concentration"`
	Steps         uint64      `json:"steps"`
}

// Kinetics is an immutable validated chemistry. It owns its declaration, so a
// caller that writes into the struct it passed to NewChemistry changes nothing.
type Kinetics struct {
	config Chemistry
	// lambda is exp(-dt/tau_k) and alpha is 1-lambda_k, the coefficients of the
	// unboosted step. rate is 1/tau_k, the clearance rate a boost adds to.
	lambda, alpha, rate []float64
	// outflow[r] is the declared share that leaves region r each step, the sum
	// of transport row r. It is nil when no transport is declared.
	outflow []float64
}

// NewChemistry validates and copies a declaration. An empty unit becomes
// UnitNormalized. A time constant so large that 1-exp(-dt/tau) rounds to zero
// is refused rather than accepted as a channel that can never change.
func NewChemistry(c Chemistry) (*Kinetics, error) {
	if c.Regions < 1 {
		return nil, fmt.Errorf("modulation: regions must be at least 1, got %d", c.Regions)
	}
	if c.Channels < 1 {
		return nil, fmt.Errorf("modulation: channels must be at least 1, got %d", c.Channels)
	}
	if !finite(c.DT) || c.DT <= 0 {
		return nil, fmt.Errorf("modulation: dt must be finite and positive, got %v", c.DT)
	}
	if len(c.Tau) != c.Channels {
		return nil, fmt.Errorf("modulation: tau has %d entries, want %d channels", len(c.Tau), c.Channels)
	}
	if c.Units == "" {
		c.Units = UnitNormalized
	}
	k := &Kinetics{
		lambda: make([]float64, c.Channels),
		alpha:  make([]float64, c.Channels),
		rate:   make([]float64, c.Channels),
	}
	for ch, tau := range c.Tau {
		if !finite(tau) || tau <= 0 {
			return nil, fmt.Errorf("modulation: tau[%d] must be finite and positive, got %v", ch, tau)
		}
		k.lambda[ch] = math.Exp(-c.DT / tau)
		// 1-lambda is the coefficient the specification names. It is written
		// out rather than computed with expm1 so that tau*q is an exact fixed
		// point of the recurrence below; the price is that dt/tau under the
		// float64 resolution leaves alpha at zero, which is refused here.
		k.alpha[ch] = 1 - k.lambda[ch]
		k.rate[ch] = 1 / tau
		if k.alpha[ch] == 0 {
			return nil, fmt.Errorf("modulation: tau[%d] = %v against dt %v leaves 1-exp(-dt/tau) at zero, which makes the channel a no-op", ch, tau, c.DT)
		}
		if !finite(k.rate[ch]) || k.rate[ch] <= 0 {
			return nil, fmt.Errorf("modulation: tau[%d] = %v has no representable positive clearance rate 1/tau", ch, tau)
		}
	}
	c.Tau = append([]float64(nil), c.Tau...)
	if c.Transport != nil {
		fraction, outflow, err := checkedTransport(c.Transport.Fraction, c.Regions)
		if err != nil {
			return nil, err
		}
		c.Transport = &Transport{Fraction: fraction}
		k.outflow = outflow
	}
	k.config = c
	return k, nil
}

// checkedTransport validates a transport matrix and returns an owned copy with
// the per-region outflow. Non-negative entries whose row sums to at most one
// make both the non-negativity and the mass bound structural: a region keeps
// 1-outflow of what it had and receives only shares of what others had.
func checkedTransport(fraction [][]float64, regions int) ([][]float64, []float64, error) {
	if len(fraction) != regions {
		return nil, nil, fmt.Errorf("modulation: transport has %d rows, want %d regions", len(fraction), regions)
	}
	owned := make([][]float64, regions)
	outflow := make([]float64, regions)
	for r, row := range fraction {
		if len(row) != regions {
			return nil, nil, fmt.Errorf("modulation: transport row %d has %d entries, want %d regions", r, len(row), regions)
		}
		var sum float64
		for s, v := range row {
			if !finite(v) {
				return nil, nil, fmt.Errorf("modulation: transport fraction [%d][%d] is not finite", r, s)
			}
			if r == s {
				if v != 0 {
					return nil, nil, fmt.Errorf("modulation: transport diagonal entry [%d][%d] is %v, want 0 because a region does not transport to itself", r, s, v)
				}
				continue
			}
			if v < 0 {
				return nil, nil, fmt.Errorf("modulation: transport fraction [%d][%d] is negative (%v)", r, s, v)
			}
			sum += v
		}
		if sum > 1 {
			return nil, nil, fmt.Errorf("modulation: transport row %d sums to %v, want at most 1", r, sum)
		}
		owned[r] = append([]float64(nil), row...)
		outflow[r] = sum
	}
	return owned, outflow, nil
}

// Config returns an independent copy of the validated declaration.
func (k *Kinetics) Config() Chemistry {
	if k == nil {
		return Chemistry{}
	}
	c := k.config
	c.Tau = append([]float64(nil), c.Tau...)
	if c.Transport != nil {
		fraction := make([][]float64, len(c.Transport.Fraction))
		for r, row := range c.Transport.Fraction {
			fraction[r] = append([]float64(nil), row...)
		}
		c.Transport = &Transport{Fraction: fraction}
	}
	return c
}

// NewState is the neutral state: every concentration at zero, no step taken.
func (k *Kinetics) NewState() ChemistryState {
	if k == nil {
		return ChemistryState{}
	}
	concentration := make([][]float64, k.config.Regions)
	for r := range concentration {
		concentration[r] = make([]float64, k.config.Channels)
	}
	return ChemistryState{Concentration: concentration, Steps: 0}
}

// Step advances the concentration by one step and returns an owned state. It
// modifies neither the state nor the slices it is given.
//
// The local update of region r and channel k is the specification rule with an
// optional extra clearance term b = clearanceBoost[r][k] >= 0, which the reward
// mapper produces for a punishment instead of a negative release:
//
//	rate   = 1/tau_k + b
//	lambda = exp(-dt*rate)
//	c'     = lambda*c + (1-lambda)*(q/rate)
//
// With b = 0 this is exactly the declared form lambda_k*c + tau_k*(1-lambda_k)*q,
// and that branch is taken literally so the unboosted step is bit for bit the
// specification's own arithmetic. A larger b means a faster decay and a lower
// steady state q/rate; b never makes a concentration negative.
//
// Regional transport then redistributes what the local update produced:
//
//	c''[s] = c'[s]*(1 - sum_t F[s][t]) + sum_{r != s} c'[r]*F[r][s]
//
// Both factors are non-negative and every row sums to at most one, so transport
// cannot create mass or a negative concentration. Clearance is the only sink.
//
// A nil clearanceBoost is the same as an all-zero one. A release that is
// negative, non-finite or the wrong shape, and a state whose concentration is
// negative, non-finite or the wrong shape, are refused before anything is
// computed.
func (k *Kinetics) Step(s ChemistryState, release, clearanceBoost [][]float64) (ChemistryState, error) {
	if k == nil || k.config.Regions < 1 {
		return ChemistryState{}, fmt.Errorf("modulation: uninitialized chemistry")
	}
	if err := k.checkedGrid(s.Concentration, "concentration"); err != nil {
		return ChemistryState{}, err
	}
	if err := k.checkedGrid(release, "release"); err != nil {
		return ChemistryState{}, err
	}
	if clearanceBoost != nil {
		if err := k.checkedGrid(clearanceBoost, "clearance boost"); err != nil {
			return ChemistryState{}, err
		}
	}
	if s.Steps == math.MaxUint64 {
		return ChemistryState{}, fmt.Errorf("modulation: chemistry step counter overflow")
	}
	local := make([][]float64, k.config.Regions)
	for r := range local {
		row := make([]float64, k.config.Channels)
		for ch := range row {
			var boost float64
			if clearanceBoost != nil {
				boost = clearanceBoost[r][ch]
			}
			if boost == 0 {
				row[ch] = k.lambda[ch]*s.Concentration[r][ch] + k.config.Tau[ch]*k.alpha[ch]*release[r][ch]
			} else {
				rate := k.rate[ch] + boost
				lambda := math.Exp(-k.config.DT * rate)
				row[ch] = lambda*s.Concentration[r][ch] + (1-lambda)*(release[r][ch]/rate)
			}
			if !finite(row[ch]) {
				return ChemistryState{}, fmt.Errorf("modulation: region %d channel %d reached a non-finite concentration", r, ch)
			}
			row[ch] = nonNegative(row[ch])
		}
		local[r] = row
	}
	next := local
	if k.outflow != nil {
		next = make([][]float64, k.config.Regions)
		for sink := range next {
			row := make([]float64, k.config.Channels)
			for ch := range row {
				v := local[sink][ch] * (1 - k.outflow[sink])
				for source := range local {
					if source == sink {
						continue
					}
					v += local[source][ch] * k.config.Transport.Fraction[source][sink]
				}
				if !finite(v) {
					return ChemistryState{}, fmt.Errorf("modulation: region %d channel %d reached a non-finite concentration after transport", sink, ch)
				}
				row[ch] = nonNegative(v)
			}
			next[sink] = row
		}
	}
	return ChemistryState{Concentration: next, Steps: s.Steps + 1}, nil
}

// checkedGrid is the shared gate for every region-by-channel matrix this layer
// reads. Concentrations, release rates and clearance boosts are all finite and
// non-negative, and all three are exactly one value per region per channel.
func (k *Kinetics) checkedGrid(grid [][]float64, name string) error {
	if len(grid) != k.config.Regions {
		return fmt.Errorf("modulation: %s has %d regions, want %d", name, len(grid), k.config.Regions)
	}
	for r, row := range grid {
		if len(row) != k.config.Channels {
			return fmt.Errorf("modulation: %s region %d has %d channels, want %d", name, r, len(row), k.config.Channels)
		}
		for ch, v := range row {
			if !finite(v) {
				return fmt.Errorf("modulation: %s region %d channel %d is not finite", name, r, ch)
			}
			if v < 0 {
				return fmt.Errorf("modulation: %s region %d channel %d is negative (%v)", name, r, ch, v)
			}
		}
	}
	return nil
}
