package rl

import (
	"fmt"
	"math"
)

// PPOConfig fixes the coefficients and iteration shape of one PPO update.
type PPOConfig struct {
	Gamma       float64 `json:"gamma"`
	Lambda      float64 `json:"lambda"`
	ClipEpsilon float64 `json:"clip_epsilon"`
	ValueCoef   float64 `json:"value_coef"`
	EntropyCoef float64 `json:"entropy_coef"`
	BurnIn      int     `json:"burn_in"`
	TimeLimit   int     `json:"time_limit"`
	Epochs      int     `json:"epochs"`
	MiniBatch   int     `json:"mini_batch"`
}

// Validate reports whether the config is usable: gamma and lambda in (0, 1],
// clip epsilon in (0, 1), coefficients finite and non-negative, BurnIn >= 0,
// TimeLimit >= 1, Epochs >= 1 and MiniBatch >= 1.
func (c PPOConfig) Validate() error {
	if !(c.Gamma > 0 && c.Gamma <= 1) {
		return fmt.Errorf("rl: gamma must be in (0, 1], got %v", c.Gamma)
	}
	if !(c.Lambda > 0 && c.Lambda <= 1) {
		return fmt.Errorf("rl: lambda must be in (0, 1], got %v", c.Lambda)
	}
	if !(c.ClipEpsilon > 0 && c.ClipEpsilon < 1) {
		return fmt.Errorf("rl: clip epsilon must be in (0, 1), got %v", c.ClipEpsilon)
	}
	if !(c.ValueCoef >= 0) || math.IsNaN(c.ValueCoef) || math.IsInf(c.ValueCoef, 0) {
		return fmt.Errorf("rl: value_coef must be finite and >= 0, got %v", c.ValueCoef)
	}
	if !(c.EntropyCoef >= 0) || math.IsNaN(c.EntropyCoef) || math.IsInf(c.EntropyCoef, 0) {
		return fmt.Errorf("rl: entropy_coef must be finite and >= 0, got %v", c.EntropyCoef)
	}
	if c.BurnIn < 0 {
		return fmt.Errorf("rl: burn_in must be >= 0, got %v", c.BurnIn)
	}
	if c.TimeLimit < 1 {
		return fmt.Errorf("rl: time_limit must be >= 1, got %v", c.TimeLimit)
	}
	if c.Epochs < 1 {
		return fmt.Errorf("rl: epochs must be >= 1, got %v", c.Epochs)
	}
	if c.MiniBatch < 1 {
		return fmt.Errorf("rl: mini_batch must be >= 1, got %v", c.MiniBatch)
	}
	return nil
}

// StepLoss decomposes the PPO loss of one transition: Policy is the clipped
// surrogate objective, Value and Entropy the regularizer terms, Ratio the
// probability ratio r and Clipped whether the clipped term is the active
// minimum (where the policy gradient is exactly zero).
type StepLoss struct {
	Loss    float64 `json:"loss"`
	Policy  float64 `json:"policy"`
	Value   float64 `json:"value"`
	Entropy float64 `json:"entropy"`
	Ratio   float64 `json:"ratio"`
	Clipped bool    `json:"clipped"`
}

// Loss evaluates the PPO objective of one transition under the current policy:
// logp = log softmax(logits)[action], r = exp(logp - oldLogProb),
// policy = -min(r*A, clip(r, 1-eps, 1+eps)*A),
// value = ValueCoef*(V - target)^2 and
// entropy = -EntropyCoef*H(softmax(logits)), with loss summing the three.
// It returns the loss pieces and the gradients dloss/dlogits and dloss/dvalue.
// On the side of the clip region where the clipped term is the minimum, the
// policy gradient is exactly zero.
func Loss(logits []float64, action int, oldLogProb, advantage, value, target float64, c PPOConfig) (StepLoss, []float64, float64, error) {
	if err := c.Validate(); err != nil {
		return StepLoss{}, nil, 0, err
	}
	logp, err := LogProb(logits, action)
	if err != nil {
		return StepLoss{}, nil, 0, err
	}
	// Log-probabilities come from log-sum-exp so an underflowed class never
	// turns 0*log(0) into NaN: a zero probability contributes nothing.
	lse := logSumExp(logits)
	p := make([]float64, len(logits))
	logP := make([]float64, len(logits))
	var h float64
	for j, x := range logits {
		logP[j] = x - lse
		p[j] = math.Exp(logP[j])
		if p[j] > 0 {
			h -= p[j] * logP[j]
		}
	}
	r := math.Exp(logp - oldLogProb)
	clipped := math.Min(math.Max(r, 1-c.ClipEpsilon), 1+c.ClipEpsilon)
	term := math.Min(r*advantage, clipped*advantage)
	policy := -term
	valueLoss := c.ValueCoef * (value - target) * (value - target)
	entropyLoss := -c.EntropyCoef * h

	st := StepLoss{
		Loss:    policy + valueLoss + entropyLoss,
		Policy:  policy,
		Value:   valueLoss,
		Entropy: entropyLoss,
		Ratio:   r,
	}
	grad := make([]float64, len(p))
	if r*advantage <= clipped*advantage {
		for j := range p {
			var ind float64
			if j == action {
				ind = 1
			}
			grad[j] = -advantage * r * (ind - p[j])
		}
	} else {
		st.Clipped = true
	}
	if c.EntropyCoef != 0 {
		for j := range p {
			if p[j] > 0 {
				grad[j] += c.EntropyCoef * p[j] * (h + logP[j])
			}
		}
	}
	return st, grad, 2 * c.ValueCoef * (value - target), nil
}

// LogProb is log softmax(logits)[action] via log-sum-exp.
func LogProb(logits []float64, action int) (float64, error) {
	if len(logits) == 0 {
		return 0, fmt.Errorf("rl: logits must be non-empty")
	}
	if action < 0 || action >= len(logits) {
		return 0, fmt.Errorf("rl: action %d out of range [0, %d)", action, len(logits))
	}
	return logits[action] - logSumExp(logits), nil
}

func logSumExp(x []float64) float64 {
	max := x[0]
	for i := 1; i < len(x); i++ {
		if x[i] > max {
			max = x[i]
		}
	}
	var sum float64
	for _, v := range x {
		sum += math.Exp(v - max)
	}
	return max + math.Log(sum)
}

func softmax(x []float64) []float64 {
	lse := logSumExp(x)
	p := make([]float64, len(x))
	for i, v := range x {
		p[i] = math.Exp(v - lse)
	}
	return p
}
