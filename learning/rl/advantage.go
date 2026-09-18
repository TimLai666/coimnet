package rl

import (
	"errors"
	"fmt"
	"math"
)

// GAEConfig fixes gamma and lambda in (0, 1].
type GAEConfig struct {
	Gamma  float64 `json:"gamma"`
	Lambda float64 `json:"lambda"`
}

// Validate reports whether gamma and lambda lie in (0, 1].
func (c GAEConfig) Validate() error {
	if !(c.Gamma > 0 && c.Gamma <= 1) {
		return fmt.Errorf("rl: gamma must be in (0, 1], got %v", c.Gamma)
	}
	if !(c.Lambda > 0 && c.Lambda <= 1) {
		return fmt.Errorf("rl: lambda must be in (0, 1], got %v", c.Lambda)
	}
	return nil
}

// Advantages returns GAE advantages and the value targets (advantage + value)
// for one rollout, walking backwards: delta_t = r_t + gamma*V_{t+1}*(1−done_t) − V_t,
// A_t = delta_t + gamma*lambda*(1−done_t)*A_{t+1}, where V_{t+1} is the next
// transition's Value, or BootstrapValue when the step is a Timeout, or 0 when
// Done. A Done or Timeout step ends the recursion (A_{t+1} treated as 0).
// The last transition of a rollout must be Done or Timeout ("rollout must end").
// Done and Timeout on the same step is an error. Non-finite fields are errors.
func Advantages(steps []Transition, c GAEConfig) (advantages, targets []float64, err error) {
	if err := c.Validate(); err != nil {
		return nil, nil, err
	}
	if len(steps) == 0 {
		return nil, nil, errors.New("rl: rollout must end: empty rollout")
	}
	for i := range steps {
		s := &steps[i]
		if s.Done && s.Timeout {
			return nil, nil, fmt.Errorf("rl: step %d is both done and timeout", i)
		}
		if !finite(s.LogProb) || !finite(s.Value) || !finite(s.Reward) || !finite(s.BootstrapValue) {
			return nil, nil, fmt.Errorf("rl: step %d has a non-finite field", i)
		}
		for j := range s.Obs {
			if !finite(s.Obs[j]) {
				return nil, nil, fmt.Errorf("rl: step %d has a non-finite observation", i)
			}
		}
	}
	last := steps[len(steps)-1]
	if !last.Done && !last.Timeout {
		return nil, nil, errors.New("rl: rollout must end: last transition is neither done nor timeout")
	}

	n := len(steps)
	advantages = make([]float64, n)
	targets = make([]float64, n)
	var aNext float64 // A_{t+1}; a Done or Timeout step restarts it at 0
	for t := n - 1; t >= 0; t-- {
		s := &steps[t]
		var vNext float64
		switch {
		case s.Timeout:
			vNext = s.BootstrapValue
		case !s.Done && t+1 < n:
			vNext = steps[t+1].Value
		}
		delta := s.Reward + c.Gamma*vNext - s.Value
		a := delta
		if !s.Done && !s.Timeout {
			a += c.Gamma * c.Lambda * aNext
		}
		aNext = a
		advantages[t] = a
		targets[t] = a + s.Value
	}
	return advantages, targets, nil
}

func finite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}
