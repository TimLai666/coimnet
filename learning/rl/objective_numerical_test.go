package rl

import (
	"fmt"
	"math"
	"testing"
)

func objectiveNumericalConfig() PPOConfig {
	return PPOConfig{
		Gamma:       1,
		Lambda:      1,
		ClipEpsilon: 0.2,
		ValueCoef:   0.5,
		EntropyCoef: 0.25,
		TimeLimit:   1,
		Epochs:      1,
		MiniBatch:   1,
	}
}

func objectiveNumericalNear(t *testing.T, name string, got, want float64) {
	t.Helper()
	const tol = 2e-12
	if math.IsNaN(got) || math.IsInf(got, 0) || math.IsNaN(want) || math.IsInf(want, 0) {
		t.Fatalf("%s: got %.17g, want finite %.17g", name, got, want)
	}
	if math.Abs(got-want) > tol*math.Max(1, math.Max(math.Abs(got), math.Abs(want))) {
		t.Errorf("%s: got %.17g, want %.17g", name, got, want)
	}
}

func objectiveNumericalSameBits(got, want []float64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if math.Float64bits(got[i]) != math.Float64bits(want[i]) {
			return false
		}
	}
	return true
}

func objectiveNumericalAssertLossFailure(t *testing.T, logits []float64, action int, oldLogProb, advantage, value, target float64, c PPOConfig) {
	t.Helper()
	wantLogits := append([]float64(nil), logits...)
	st, grad, gradValue, err := Loss(logits, action, oldLogProb, advantage, value, target, c)
	if err == nil {
		t.Fatal("Loss returned nil error for an invalid or overflowing input")
	}
	if st != (StepLoss{}) {
		t.Errorf("failed Loss returned %+v, want zero StepLoss", st)
	}
	if grad != nil {
		t.Errorf("failed Loss returned gradient %v, want nil", grad)
	}
	if gradValue != 0 {
		t.Errorf("failed Loss returned value gradient %.17g, want 0", gradValue)
	}
	if !objectiveNumericalSameBits(logits, wantLogits) {
		t.Errorf("failed Loss modified logits: got %v, want %v", logits, wantLogits)
	}
}

func TestObjectiveNumericalEqualFiniteScoresUseHalfProbability(t *testing.T) {
	oldLogProb := -math.Log(2)
	for _, score := range []float64{0, 1e16, -1e16, 1e308, -1e308} {
		score := score
		t.Run(fmt.Sprintf("score_%g", score), func(t *testing.T) {
			logits := []float64{score, score}
			before := append([]float64(nil), logits...)
			for action := range logits {
				got, err := LogProb(logits, action)
				if err != nil {
					t.Fatalf("LogProb action %d: %v", action, err)
				}
				objectiveNumericalNear(t, fmt.Sprintf("log_prob[%d]", action), got, oldLogProb)
			}
			if !objectiveNumericalSameBits(logits, before) {
				t.Fatalf("LogProb modified logits: got %v, want %v", logits, before)
			}

			c := objectiveNumericalConfig()
			c.ValueCoef = 0
			wantEntropy := -c.EntropyCoef * math.Log(2)
			for _, advantage := range []float64{1, -1} {
				st, grad, gradValue, err := Loss(logits, 0, oldLogProb, advantage, 0, 0, c)
				if err != nil {
					t.Fatalf("Loss advantage %v: %v", advantage, err)
				}
				objectiveNumericalNear(t, "ratio", st.Ratio, 1)
				objectiveNumericalNear(t, "policy", st.Policy, -advantage)
				objectiveNumericalNear(t, "value", st.Value, 0)
				objectiveNumericalNear(t, "entropy", st.Entropy, wantEntropy)
				objectiveNumericalNear(t, "loss", st.Loss, -advantage+wantEntropy)
				if st.Clipped {
					t.Errorf("advantage %v: same-policy ratio must not be clipped", advantage)
				}
				if len(grad) != 2 {
					t.Fatalf("gradient length %d, want 2", len(grad))
				}
				objectiveNumericalNear(t, "gradient[action]", grad[0], -advantage/2)
				objectiveNumericalNear(t, "gradient[other]", grad[1], advantage/2)
				if gradValue != 0 {
					t.Errorf("value gradient %.17g, want 0", gradValue)
				}
			}
			if !objectiveNumericalSameBits(logits, before) {
				t.Errorf("Loss modified logits: got %v, want %v", logits, before)
			}
		})
	}
}

func TestObjectiveNumericalFiniteTranslationInvariance(t *testing.T) {
	base := []float64{1, 2, 3}
	shifted := []float64{1e15 + 1, 1e15 + 2, 1e15 + 3}
	const (
		action     = 2
		oldLogProb = -0.5
		advantage  = 0.7
		value      = 0.25
		target     = -0.5
	)
	c := objectiveNumericalConfig()
	c.ValueCoef = 0.2
	c.EntropyCoef = 0.03

	denom := 1 + math.Exp(-1) + math.Exp(-2)
	wantLogProb := -math.Log(denom)
	p := []float64{math.Exp(-2) / denom, math.Exp(-1) / denom, 1 / denom}
	logP := []float64{-2 - math.Log(denom), -1 - math.Log(denom), -math.Log(denom)}
	h := math.Log(denom) + 2*p[0] + p[1]
	ratio := math.Exp(wantLogProb - oldLogProb)
	wantPolicy := -ratio * advantage
	wantValue := c.ValueCoef * (value - target) * (value - target)
	wantEntropy := -c.EntropyCoef * h
	wantLoss := wantPolicy + wantValue + wantEntropy
	wantGrad := make([]float64, len(p))
	for j := range p {
		ind := 0.0
		if j == action {
			ind = 1
		}
		wantGrad[j] = -advantage*ratio*(ind-p[j]) + c.EntropyCoef*p[j]*(h+logP[j])
	}
	wantValueGrad := 2 * c.ValueCoef * (value - target)

	baseLogProb, err := LogProb(base, action)
	if err != nil {
		t.Fatal(err)
	}
	shiftedLogProb, err := LogProb(shifted, action)
	if err != nil {
		t.Fatal(err)
	}
	objectiveNumericalNear(t, "base log_prob", baseLogProb, wantLogProb)
	objectiveNumericalNear(t, "shifted log_prob", shiftedLogProb, wantLogProb)

	baseStep, baseGrad, baseValueGrad, err := Loss(base, action, oldLogProb, advantage, value, target, c)
	if err != nil {
		t.Fatal(err)
	}
	shiftedStep, shiftedGrad, shiftedValueGrad, err := Loss(shifted, action, oldLogProb, advantage, value, target, c)
	if err != nil {
		t.Fatal(err)
	}
	for name, pair := range map[string][2]float64{
		"base ratio":    {baseStep.Ratio, ratio},
		"base policy":   {baseStep.Policy, wantPolicy},
		"base value":    {baseStep.Value, wantValue},
		"base entropy":  {baseStep.Entropy, wantEntropy},
		"base loss":     {baseStep.Loss, wantLoss},
		"shift ratio":   {shiftedStep.Ratio, ratio},
		"shift policy":  {shiftedStep.Policy, wantPolicy},
		"shift value":   {shiftedStep.Value, wantValue},
		"shift entropy": {shiftedStep.Entropy, wantEntropy},
		"shift loss":    {shiftedStep.Loss, wantLoss},
	} {
		objectiveNumericalNear(t, name, pair[0], pair[1])
	}
	if baseStep.Clipped || shiftedStep.Clipped {
		t.Errorf("translation-invariant finite ratio should not be clipped: base=%v shifted=%v", baseStep.Clipped, shiftedStep.Clipped)
	}
	if len(baseGrad) != len(wantGrad) || len(shiftedGrad) != len(wantGrad) {
		t.Fatalf("gradient lengths: base=%d shifted=%d want=%d", len(baseGrad), len(shiftedGrad), len(wantGrad))
	}
	for j := range wantGrad {
		objectiveNumericalNear(t, fmt.Sprintf("base gradient[%d]", j), baseGrad[j], wantGrad[j])
		objectiveNumericalNear(t, fmt.Sprintf("shifted gradient[%d]", j), shiftedGrad[j], wantGrad[j])
		objectiveNumericalNear(t, fmt.Sprintf("gradient translation[%d]", j), shiftedGrad[j], baseGrad[j])
	}
	objectiveNumericalNear(t, "base value gradient", baseValueGrad, wantValueGrad)
	objectiveNumericalNear(t, "shifted value gradient", shiftedValueGrad, wantValueGrad)
	objectiveNumericalNear(t, "value-gradient translation", shiftedValueGrad, baseValueGrad)
}

func TestObjectiveNumericalRejectsNaNAndPositiveInfinityScores(t *testing.T) {
	cases := []struct {
		name   string
		logits []float64
	}{
		{"nan", []float64{0, math.NaN()}},
		{"positive_inf", []float64{0, math.Inf(1)}},
		{"nan_selected", []float64{math.NaN(), 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logits := append([]float64(nil), tc.logits...)
			before := append([]float64(nil), logits...)
			got, err := LogProb(logits, 0)
			if err == nil {
				t.Fatal("LogProb returned nil error")
			}
			if got != 0 {
				t.Errorf("failed LogProb returned %.17g, want 0", got)
			}
			if !objectiveNumericalSameBits(logits, before) {
				t.Errorf("LogProb modified logits: got %v, want %v", logits, before)
			}
			objectiveNumericalAssertLossFailure(t, append([]float64(nil), tc.logits...), 0, -math.Log(2), 1, 0, 0, objectiveNumericalConfig())
		})
	}
}

func TestObjectiveNumericalPartialNegativeInfinityMaskKeepsFiniteEntropy(t *testing.T) {
	logits := []float64{0, math.Inf(-1), 0}
	c := objectiveNumericalConfig()
	c.ValueCoef = 0
	oldLogProb := -math.Log(2)

	finiteLogProb, err := LogProb(logits, 0)
	if err != nil {
		t.Fatal(err)
	}
	objectiveNumericalNear(t, "unmasked log_prob", finiteLogProb, oldLogProb)
	maskedLogProb, err := LogProb(logits, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !math.IsInf(maskedLogProb, -1) {
		t.Errorf("masked log_prob = %.17g, want -Inf", maskedLogProb)
	}

	step, grad, valueGrad, err := Loss(logits, 0, oldLogProb, 1, 0, 0, c)
	if err != nil {
		t.Fatal(err)
	}
	wantEntropy := -c.EntropyCoef * math.Log(2)
	objectiveNumericalNear(t, "unmasked ratio", step.Ratio, 1)
	objectiveNumericalNear(t, "unmasked policy", step.Policy, -1)
	objectiveNumericalNear(t, "unmasked entropy", step.Entropy, wantEntropy)
	objectiveNumericalNear(t, "unmasked loss", step.Loss, -1+wantEntropy)
	if step.Clipped {
		t.Error("unmasked action at ratio 1 must not be clipped")
	}
	if valueGrad != 0 || len(grad) != 3 {
		t.Fatalf("unmasked gradients: logits=%v value=%.17g", grad, valueGrad)
	}
	objectiveNumericalNear(t, "gradient[0]", grad[0], -0.5)
	objectiveNumericalNear(t, "gradient[1]", grad[1], 0)
	objectiveNumericalNear(t, "gradient[2]", grad[2], 0.5)

	selectedMasked, maskedGrad, maskedValueGrad, err := Loss(logits, 1, oldLogProb, 1, 0, 0, c)
	if err != nil {
		t.Fatal(err)
	}
	objectiveNumericalNear(t, "selected-masked ratio", selectedMasked.Ratio, 0)
	objectiveNumericalNear(t, "selected-masked policy", selectedMasked.Policy, 0)
	objectiveNumericalNear(t, "selected-masked entropy", selectedMasked.Entropy, wantEntropy)
	objectiveNumericalNear(t, "selected-masked loss", selectedMasked.Loss, wantEntropy)
	if selectedMasked.Clipped {
		t.Error("positive advantage with zero ratio must remain on the unclipped side")
	}
	if maskedValueGrad != 0 || len(maskedGrad) != len(logits) {
		t.Fatalf("selected-masked gradients: logits=%v value=%.17g", maskedGrad, maskedValueGrad)
	}
	for j, got := range maskedGrad {
		objectiveNumericalNear(t, fmt.Sprintf("selected-masked gradient[%d]", j), got, 0)
	}
}

func TestObjectiveNumericalRejectsAllNegativeInfinityMask(t *testing.T) {
	logits := []float64{math.Inf(-1), math.Inf(-1)}
	if got, err := LogProb(logits, 0); err == nil {
		t.Errorf("LogProb returned %.17g with nil error for an all-masked policy", got)
	} else if got != 0 {
		t.Errorf("failed LogProb returned %.17g, want 0", got)
	}
	objectiveNumericalAssertLossFailure(t, logits, 0, -math.Log(2), 1, 0, 0, objectiveNumericalConfig())
}

func TestObjectiveNumericalLossRejectsNonFiniteScalars(t *testing.T) {
	fields := []string{"old_log_prob", "advantage", "value", "target"}
	badValues := []struct {
		name  string
		value float64
	}{
		{"nan", math.NaN()},
		{"positive_inf", math.Inf(1)},
		{"negative_inf", math.Inf(-1)},
	}
	for _, field := range fields {
		field := field
		for _, bad := range badValues {
			bad := bad
			t.Run(fmt.Sprintf("%s_%s", field, bad.name), func(t *testing.T) {
				oldLogProb := -math.Log(2)
				advantage, value, target := 1.0, 0.25, -0.5
				switch field {
				case "old_log_prob":
					oldLogProb = bad.value
				case "advantage":
					advantage = bad.value
				case "value":
					value = bad.value
				case "target":
					target = bad.value
				}
				objectiveNumericalAssertLossFailure(t, []float64{0, 0}, 0, oldLogProb, advantage, value, target, objectiveNumericalConfig())
			})
		}
	}
}

func TestObjectiveNumericalLossRejectsNonFiniteCoefficients(t *testing.T) {
	for _, field := range []string{"value_coef", "entropy_coef"} {
		field := field
		for _, bad := range []struct {
			name  string
			value float64
		}{
			{"nan", math.NaN()},
			{"positive_inf", math.Inf(1)},
			{"negative_inf", math.Inf(-1)},
		} {
			bad := bad
			t.Run(fmt.Sprintf("%s_%s", field, bad.name), func(t *testing.T) {
				c := objectiveNumericalConfig()
				if field == "value_coef" {
					c.ValueCoef = bad.value
				} else {
					c.EntropyCoef = bad.value
				}
				objectiveNumericalAssertLossFailure(t, []float64{0, 0}, 0, -math.Log(2), 1, 0, 0, c)
			})
		}
	}
}

func TestObjectiveNumericalRejectsFiniteLogitDifferenceOverflow(t *testing.T) {
	for _, logits := range [][]float64{
		{math.MaxFloat64, -math.MaxFloat64},
		{-math.MaxFloat64, math.MaxFloat64},
	} {
		logits := logits
		t.Run(fmt.Sprintf("%g_%g", logits[0], logits[1]), func(t *testing.T) {
			for action := range logits {
				got, err := LogProb(logits, action)
				if err == nil {
					t.Errorf("LogProb action %d returned %.17g with nil error", action, got)
				} else if got != 0 {
					t.Errorf("LogProb action %d returned %.17g, want 0", action, got)
				}
			}
			objectiveNumericalAssertLossFailure(t, logits, 0, -math.Log(2), 1, 0, 0, objectiveNumericalConfig())
		})
	}
}

func TestObjectiveNumericalLossRejectsRatioValueGradientAndTotalOverflow(t *testing.T) {
	base := objectiveNumericalConfig()
	base.EntropyCoef = 0
	cases := []struct {
		name                  string
		logits                []float64
		oldLogProb, advantage float64
		value, target         float64
		config                PPOConfig
	}{
		{
			name:       "ratio",
			logits:     []float64{0, 0},
			oldLogProb: -math.MaxFloat64,
			advantage:  1,
			value:      0,
			target:     0,
			config:     func() PPOConfig { c := base; c.ValueCoef = 0; return c }(),
		},
		{
			name:       "value_loss",
			logits:     []float64{0, 0},
			oldLogProb: -math.Log(2),
			advantage:  0,
			value:      1e308,
			target:     0,
			config:     func() PPOConfig { c := base; c.ValueCoef = 1; return c }(),
		},
		{
			name:       "value_gradient",
			logits:     []float64{0, 0},
			oldLogProb: -math.Log(2),
			advantage:  0,
			value:      0.75,
			target:     0,
			config:     func() PPOConfig { c := base; c.ValueCoef = math.MaxFloat64; return c }(),
		},
		{
			name:       "total_loss",
			logits:     []float64{0, 0},
			oldLogProb: -math.Log(2),
			advantage:  -math.MaxFloat64,
			value:      2,
			target:     0,
			config:     func() PPOConfig { c := base; c.ValueCoef = math.MaxFloat64 / 4; return c }(),
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			objectiveNumericalAssertLossFailure(t, append([]float64(nil), tc.logits...), 0, tc.oldLogProb, tc.advantage, tc.value, tc.target, tc.config)
		})
	}
}

func TestObjectiveNumericalLossRejectsEntropyOverflow(t *testing.T) {
	c := objectiveNumericalConfig()
	c.ValueCoef = 0
	c.EntropyCoef = math.MaxFloat64
	objectiveNumericalAssertLossFailure(t, []float64{0, 0, 0, 0}, 0, -math.Log(4), 0, 0, 0, c)
}

func TestObjectiveNumericalZeroValueCoefSkipsExtremeValueDifference(t *testing.T) {
	logits := []float64{0, 0}
	c := objectiveNumericalConfig()
	c.ValueCoef = 0
	c.EntropyCoef = 0
	st, grad, gradValue, err := Loss(logits, 0, -math.Log(2), 1, math.MaxFloat64, -math.MaxFloat64, c)
	if err != nil {
		t.Fatal(err)
	}
	objectiveNumericalNear(t, "ratio", st.Ratio, 1)
	objectiveNumericalNear(t, "policy", st.Policy, -1)
	objectiveNumericalNear(t, "value", st.Value, 0)
	objectiveNumericalNear(t, "entropy", st.Entropy, 0)
	objectiveNumericalNear(t, "loss", st.Loss, -1)
	if st.Clipped {
		t.Error("same-policy ratio must not be clipped")
	}
	if len(grad) != 2 {
		t.Fatalf("gradient length %d, want 2", len(grad))
	}
	objectiveNumericalNear(t, "gradient[0]", grad[0], -0.5)
	objectiveNumericalNear(t, "gradient[1]", grad[1], 0.5)
	if gradValue != 0 {
		t.Errorf("value gradient %.17g, want 0", gradValue)
	}
}

func TestObjectiveNumericalEmptyAndInvalidActionReturnZero(t *testing.T) {
	if got, err := LogProb(nil, 0); err == nil {
		t.Errorf("empty LogProb returned %.17g with nil error", got)
	} else if got != 0 {
		t.Errorf("empty LogProb returned %.17g, want 0", got)
	}
	for _, tc := range []struct {
		name   string
		logits []float64
		action int
	}{
		{"empty", nil, 0},
		{"negative_action", []float64{0, 0}, -1},
		{"too_large_action", []float64{0, 0}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := LogProb(tc.logits, tc.action); err == nil {
				t.Errorf("LogProb returned %.17g with nil error", got)
			} else if got != 0 {
				t.Errorf("failed LogProb returned %.17g, want 0", got)
			}
			objectiveNumericalAssertLossFailure(t, append([]float64(nil), tc.logits...), tc.action, -math.Log(2), 1, 0, 0, objectiveNumericalConfig())
		})
	}
}
