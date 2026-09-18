package rl

import (
	"math"
	"math/rand"
	"testing"
)

func ppoCfg() PPOConfig {
	return PPOConfig{
		Gamma:       0.99,
		Lambda:      0.95,
		ClipEpsilon: 0.2,
		ValueCoef:   0.5,
		EntropyCoef: 0.01,
		BurnIn:      0,
		TimeLimit:   100,
		Epochs:      3,
		MiniBatch:   4,
	}
}

func TestRatioIsOneUnderTheSamePolicy(t *testing.T) {
	logits := []float64{0.3, -0.7, 1.2, -0.2}
	action := 1
	old, err := LogProb(logits, action)
	if err != nil {
		t.Fatal(err)
	}
	st, _, _, err := Loss(logits, action, old, 0.5, 0.3, 1.1, ppoCfg())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("ratio=%v clipped=%v", st.Ratio, st.Clipped)
	if math.Abs(st.Ratio-1) > 1e-12 {
		t.Errorf("ratio = %.15g, want 1 within 1e-12", st.Ratio)
	}
	if st.Clipped {
		t.Error("same-policy ratio r=1 must not be clipped")
	}
}

func TestRatioMovesWithTheAdvantage(t *testing.T) {
	c := ppoCfg()
	c.ValueCoef = 0
	c.EntropyCoef = 0
	logits := []float64{0, 0}
	action := 0
	old, err := LogProb(logits, action)
	if err != nil {
		t.Fatal(err)
	}
	step := func(A float64) ([]float64, float64) {
		t.Helper()
		_, grad, _, err := Loss(logits, action, old, A, 0, 0, c)
		if err != nil {
			t.Fatal(err)
		}
		next := []float64{
			logits[0] - 0.1*grad[0],
			logits[1] - 0.1*grad[1],
		}
		logp, err := LogProb(next, action)
		if err != nil {
			t.Fatal(err)
		}
		return grad, math.Exp(logp - old)
	}

	grad, r := step(1)
	want := []float64{-0.5, 0.5}
	for j := range want {
		if math.Abs(grad[j]-want[j]) > 1e-12 {
			t.Errorf("A=+1 gradLogits[%d]=%.12g, want %.12g", j, grad[j], want[j])
		}
	}
	wantR := 2.0 / (1.0 + math.Exp(-0.1))
	t.Logf("A=+1 ratio=%v want=%v", r, wantR)
	if math.Abs(r-wantR) > 1e-12 {
		t.Errorf("A=+1 ratio=%.15g, want %.15g", r, wantR)
	}
	if !(r > 1) {
		t.Errorf("A=+1 ratio=%v must exceed 1", r)
	}

	grad, r = step(-1)
	want = []float64{0.5, -0.5}
	for j := range want {
		if math.Abs(grad[j]-want[j]) > 1e-12 {
			t.Errorf("A=-1 gradLogits[%d]=%.12g, want %.12g", j, grad[j], want[j])
		}
	}
	wantR = 2.0 / (1.0 + math.Exp(0.1))
	t.Logf("A=-1 ratio=%v want=%v", r, wantR)
	if math.Abs(r-wantR) > 1e-12 {
		t.Errorf("A=-1 ratio=%.15g, want %.15g", r, wantR)
	}
	if !(r < 1) {
		t.Errorf("A=-1 ratio=%v must be below 1", r)
	}
}

func TestClippedRegionHasZeroPolicyGradient(t *testing.T) {
	c := ppoCfg()
	c.ValueCoef = 0
	c.EntropyCoef = 0
	logits := []float64{0.4, -0.1, 0.9}
	action := 2
	logp, err := LogProb(logits, action)
	if err != nil {
		t.Fatal(err)
	}
	allZero := func(grad []float64) bool {
		for _, g := range grad {
			if math.Abs(g) > 1e-12 {
				return false
			}
		}
		return true
	}

	st, grad, _, err := Loss(logits, action, logp-math.Log(1.5), 1.0, 0, 0, c)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("A=+1 r=1.5: clipped=%v ratio=%v", st.Clipped, st.Ratio)
	if !st.Clipped {
		t.Error("A=+1, r=1.5 above 1+eps: expected clipped minimum")
	}
	if !allZero(grad) {
		t.Errorf("A=+1, r=1.5: clipped policy gradient must be zero, got %v", grad)
	}

	st, grad, _, err = Loss(logits, action, logp-math.Log(0.5), -1.0, 0, 0, c)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("A=-1 r=0.5: clipped=%v ratio=%v", st.Clipped, st.Ratio)
	if !st.Clipped {
		t.Error("A=-1, r=0.5 below 1-eps: expected clipped minimum")
	}
	if !allZero(grad) {
		t.Errorf("A=-1, r=0.5: clipped policy gradient must be zero, got %v", grad)
	}

	st, grad, _, err = Loss(logits, action, logp-math.Log(0.5), 1.0, 0, 0, c)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("A=+1 r=0.5: clipped=%v ratio=%v grad=%v", st.Clipped, st.Ratio, grad)
	if st.Clipped {
		t.Error("A=+1, r=0.5 on the unclipped side: must not be clipped")
	}
	if allZero(grad) {
		t.Error("A=+1, r=0.5 on the unclipped side: policy gradient must be non-zero")
	}
}

func TestLossFiniteDifferences(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	logits := make([]float64, 4)
	for j := range logits {
		logits[j] = rng.Float64()*2 - 1
	}
	action := 2
	cfg := ppoCfg()
	cfg.ClipEpsilon = 0.2
	cfg.ValueCoef = 0.5
	cfg.EntropyCoef = 0.01
	const (
		advantage = 0.7
		value     = 0.3
		target    = 1.1
	)
	old, err := LogProb(logits, action)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("logits=%v oldLogProb=%v", logits, old)
	st, grad, gradValue, err := Loss(logits, action, old, advantage, value, target, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("loss=%v policy=%v value=%v entropy=%v ratio=%v", st.Loss, st.Policy, st.Value, st.Entropy, st.Ratio)

	const h = 1e-6
	finiteDiffLogits := func(c PPOConfig) ([]float64, error) {
		t.Helper()
		fd := make([]float64, len(logits))
		for j := range logits {
			orig := logits[j]
			logits[j] = orig + h
			up, _, _, err := Loss(logits, action, old, advantage, value, target, c)
			if err != nil {
				return nil, err
			}
			logits[j] = orig - h
			down, _, _, err := Loss(logits, action, old, advantage, value, target, c)
			if err != nil {
				return nil, err
			}
			logits[j] = orig
			fd[j] = (up.Loss - down.Loss) / (2 * h)
		}
		return fd, nil
	}

	fd, err := finiteDiffLogits(cfg)
	if err != nil {
		t.Fatal(err)
	}
	worst := 0.0
	for j := range logits {
		denom := math.Max(math.Abs(grad[j]), math.Abs(fd[j]))
		if denom == 0 {
			continue
		}
		rel := math.Abs(grad[j]-fd[j]) / denom
		if rel > worst {
			worst = rel
		}
		t.Logf("logits[%d]: analytic=%.15g finite-diff=%.15g rel=%.3g", j, grad[j], fd[j], rel)
		if rel > 1e-6 {
			t.Errorf("logits[%d]: relative error %.3g exceeds 1e-6", j, rel)
		}
	}
	t.Logf("worst logits relative error %.3g", worst)

	up, _, _, err := Loss(logits, action, old, advantage, value+h, target, cfg)
	if err != nil {
		t.Fatal(err)
	}
	down, _, _, err := Loss(logits, action, old, advantage, value-h, target, cfg)
	if err != nil {
		t.Fatal(err)
	}
	fdV := (up.Loss - down.Loss) / (2 * h)
	fdVDenom := math.Max(math.Abs(gradValue), math.Abs(fdV))
	if fdVDenom > 0 {
		fdVRel := math.Abs(gradValue-fdV) / fdVDenom
		t.Logf("value: analytic=%.15g finite-diff=%.15g rel=%.3g", gradValue, fdV, fdVRel)
		if fdVRel > 1e-6 {
			t.Errorf("value gradient: relative error %.3g exceeds 1e-6", fdVRel)
		}
	}

	ecfg := ppoCfg()
	ecfg.ClipEpsilon = 0.2
	ecfg.ValueCoef = 0
	ecfg.EntropyCoef = 0.05
	est, eg, egValue, err := Loss(logits, action, old, advantage, value, target, ecfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("entropy-only loss=%v policy=%v entropy=%v", est.Loss, est.Policy, est.Entropy)
	efd, err := finiteDiffLogits(ecfg)
	if err != nil {
		t.Fatal(err)
	}
	eworst := 0.0
	for j := range logits {
		denom := math.Max(math.Abs(eg[j]), math.Abs(efd[j]))
		if denom == 0 {
			continue
		}
		rel := math.Abs(eg[j]-efd[j]) / denom
		if rel > eworst {
			eworst = rel
		}
		t.Logf("entropy-only logits[%d]: analytic=%.15g finite-diff=%.15g rel=%.3g", j, eg[j], efd[j], rel)
		if rel > 1e-6 {
			t.Errorf("entropy-only logits[%d]: relative error %.3g exceeds 1e-6", j, rel)
		}
	}
	t.Logf("entropy-only worst logits relative error %.3g", eworst)
	if egValue != 0 {
		t.Errorf("entropy-only gradValue=%v, want 0 when ValueCoef=0", egValue)
	}
}

func TestPPOConfigValidate(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*PPOConfig)
	}{
		{"gamma must be in (0,1]", func(c *PPOConfig) { c.Gamma = 0 }},
		{"lambda must be in (0,1]", func(c *PPOConfig) { c.Lambda = 1.5 }},
		{"clip epsilon must be in (0,1)", func(c *PPOConfig) { c.ClipEpsilon = 1 }},
		{"clip epsilon zero", func(c *PPOConfig) { c.ClipEpsilon = 0 }},
		{"value_coef negative", func(c *PPOConfig) { c.ValueCoef = -0.1 }},
		{"value_coef NaN", func(c *PPOConfig) { c.ValueCoef = math.NaN() }},
		{"entropy_coef negative", func(c *PPOConfig) { c.EntropyCoef = -0.1 }},
		{"entropy_coef +Inf", func(c *PPOConfig) { c.EntropyCoef = math.Inf(1) }},
		{"burn_in negative", func(c *PPOConfig) { c.BurnIn = -1 }},
		{"time_limit zero", func(c *PPOConfig) { c.TimeLimit = 0 }},
		{"epochs zero", func(c *PPOConfig) { c.Epochs = 0 }},
		{"mini_batch zero", func(c *PPOConfig) { c.MiniBatch = 0 }},
	}
	logits := []float64{1, -1}
	for _, tc := range cases {
		c := ppoCfg()
		tc.mutate(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: Validate expected error", tc.name)
		} else {
			t.Logf("%s: %v", tc.name, err)
		}
		if _, _, _, err := Loss(logits, 0, -math.Log(2), 1, 0, 0, c); err == nil {
			t.Errorf("%s: Loss expected error", tc.name)
		}
	}
	if err := ppoCfg().Validate(); err != nil {
		t.Errorf("valid config: unexpected error %v", err)
	}
}
