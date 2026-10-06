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
// policy gradient is exactly zero. Scalar inputs must be finite, and logits
// follow LogProb's masking rules. Non-finite results return zero loss pieces,
// nil logits gradient and zero value gradient with an error. ValueCoef=0 skips
// the value difference calculation. Inputs are never modified.
func Loss(logits []float64, action int, oldLogProb, advantage, value, target float64, c PPOConfig) (StepLoss, []float64, float64, error) {
	if err := c.Validate(); err != nil {
		return StepLoss{}, nil, 0, err
	}
	if !finite(oldLogProb) {
		return StepLoss{}, nil, 0, fmt.Errorf("rl: old_log_prob must be finite, got %v", oldLogProb)
	}
	if !finite(advantage) {
		return StepLoss{}, nil, 0, fmt.Errorf("rl: advantage must be finite, got %v", advantage)
	}
	if !finite(value) {
		return StepLoss{}, nil, 0, fmt.Errorf("rl: value must be finite, got %v", value)
	}
	if !finite(target) {
		return StepLoss{}, nil, 0, fmt.Errorf("rl: target must be finite, got %v", target)
	}
	if len(logits) == 0 {
		return StepLoss{}, nil, 0, fmt.Errorf("rl: logits must be non-empty")
	}
	if action < 0 || action >= len(logits) {
		return StepLoss{}, nil, 0, fmt.Errorf("rl: action %d out of range [0, %d)", action, len(logits))
	}
	logP, err := logSoftmax(logits)
	if err != nil {
		return StepLoss{}, nil, 0, err
	}
	p := make([]float64, len(logits))
	var h float64
	for j := range logP {
		p[j] = math.Exp(logP[j])
		// Masked and underflowed actions contribute nothing to entropy.
		if p[j] > 0 {
			h -= p[j] * logP[j]
		}
	}
	r := math.Exp(logP[action] - oldLogProb)
	if !finite(r) {
		return StepLoss{}, nil, 0, fmt.Errorf("rl: probability ratio is non-finite")
	}
	clipped := math.Min(math.Max(r, 1-c.ClipEpsilon), 1+c.ClipEpsilon)
	policyTerm, clippedTerm := r*advantage, clipped*advantage
	policy := -math.Min(policyTerm, clippedTerm)
	var valueLoss, valueGradient float64
	if c.ValueCoef != 0 {
		delta := value - target
		// Apply the coefficient before squaring or doubling, so a small
		// coefficient can keep a large difference representable (and vice versa).
		weighted := c.ValueCoef * delta
		valueLoss = weighted * delta
		valueGradient = 2 * weighted
	}
	entropyLoss := -c.EntropyCoef * h
	st := StepLoss{Loss: policy + valueLoss + entropyLoss, Policy: policy, Value: valueLoss, Entropy: entropyLoss, Ratio: r}
	if !finite(st.Loss) || !finite(st.Policy) || !finite(st.Value) || !finite(st.Entropy) || !finite(valueGradient) {
		return StepLoss{}, nil, 0, fmt.Errorf("rl: loss or value gradient is non-finite")
	}
	grad := make([]float64, len(p))
	if policyTerm <= clippedTerm {
		for j := range p {
			var ind float64
			if j == action {
				ind = 1
			}
			grad[j] = -policyTerm * (ind - p[j])
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
	for j, g := range grad {
		if !finite(g) {
			return StepLoss{}, nil, 0, fmt.Errorf("rl: gradient[%d] is non-finite", j)
		}
	}
	return st, grad, valueGradient, nil
}

// LogProb is log softmax(logits)[action] via a shifted normalization.
// -Inf masks an action; at least one score must be finite. NaN, +Inf, an empty
// policy, an invalid action or an overflowing finite difference returns an error.
// The log probability of a masked action is -Inf. Inputs are never modified.
func LogProb(logits []float64, action int) (float64, error) {
	if len(logits) == 0 {
		return 0, fmt.Errorf("rl: logits must be non-empty")
	}
	if action < 0 || action >= len(logits) {
		return 0, fmt.Errorf("rl: action %d out of range [0, %d)", action, len(logits))
	}
	logP, err := logSoftmax(logits)
	if err != nil {
		return 0, err
	}
	return logP[action], nil
}

func logSoftmax(x []float64) ([]float64, error) {
	if len(x) == 0 {
		return nil, fmt.Errorf("rl: logits must be non-empty")
	}
	max := math.Inf(-1)
	for i, v := range x {
		if math.IsNaN(v) || math.IsInf(v, 1) {
			return nil, fmt.Errorf("rl: logits[%d] must be finite or -Inf, got %v", i, v)
		}
		if v > max {
			max = v
		}
	}
	if math.IsInf(max, -1) {
		return nil, fmt.Errorf("rl: logits must contain at least one finite score")
	}
	logP := make([]float64, len(x))
	var sum float64
	for i, v := range x {
		if math.IsInf(v, -1) {
			logP[i] = v
			continue
		}
		logP[i] = v - max
		if !finite(logP[i]) {
			return nil, fmt.Errorf("rl: logits[%d] finite difference overflow", i)
		}
		// Each exponential is in [0, 1], with at least one equal to 1.
		sum += math.Exp(logP[i])
	}
	logSum := math.Log(sum)
	for i := range logP {
		if math.IsInf(logP[i], -1) {
			continue
		}
		logP[i] -= logSum
		if !finite(logP[i]) {
			return nil, fmt.Errorf("rl: log probability[%d] is non-finite", i)
		}
	}
	return logP, nil
}

func softmax(x []float64) []float64 {
	logP, err := logSoftmax(x)
	if err != nil {
		return nil
	}
	p := make([]float64, len(logP))
	for i, v := range logP {
		p[i] = math.Exp(v)
	}
	return p
}
