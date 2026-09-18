package rl

import (
	"math"
	"strings"
	"testing"
)

func TestAdvantagesHandTableTerminal(t *testing.T) {
	const gamma = 0.9
	const lambda = 0.8
	cfg := GAEConfig{Gamma: gamma, Lambda: lambda}
	steps := []Transition{
		{Value: 1, Reward: 0},
		{Value: 2, Reward: 0},
		{Value: 3, Reward: 1, Done: true},
	}
	adv, tgt, err := Advantages(steps, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	const (
		wantA0 = 0.2672
		wantA1 = -0.74
		wantA2 = -2.0
		wantT0 = 1.2672
		wantT1 = 1.26
		wantT2 = 1.0
	)
	wantAdv := []float64{wantA0, wantA1, wantA2}
	wantTgt := []float64{wantT0, wantT1, wantT2}
	t.Logf("gamma=%.1f lambda=%.1f advantages=%v targets=%v wantAdv=%v wantTgt=%v",
		gamma, lambda, adv, tgt, wantAdv, wantTgt)
	for i := range wantAdv {
		if math.Abs(adv[i]-wantAdv[i]) > 1e-12 {
			t.Errorf("advantage[%d]=%.12g want %.12g", i, adv[i], wantAdv[i])
		}
		if math.Abs(tgt[i]-wantTgt[i]) > 1e-12 {
			t.Errorf("target[%d]=%.12g want %.12g", i, tgt[i], wantTgt[i])
		}
	}
}

func TestAdvantagesTimeoutBootstraps(t *testing.T) {
	const gamma = 0.9
	const lambda = 0.8
	cfg := GAEConfig{Gamma: gamma, Lambda: lambda}
	steps := []Transition{
		{Value: 1, Reward: 0},
		{Value: 2, Reward: 0},
		{Value: 3, Reward: 1, Timeout: true, BootstrapValue: 4},
	}
	adv, tgt, err := Advantages(steps, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	const (
		wantA0 = 2.13344
		wantA1 = 1.852
		wantA2 = 1.6
		wantT0 = 3.13344
		wantT1 = 3.852
		wantT2 = 4.6
	)
	wantAdv := []float64{wantA0, wantA1, wantA2}
	wantTgt := []float64{wantT0, wantT1, wantT2}
	t.Logf("advantages=%v targets=%v wantAdv=%v wantTgt=%v", adv, tgt, wantAdv, wantTgt)
	for i := range wantAdv {
		if math.Abs(adv[i]-wantAdv[i]) > 1e-12 {
			t.Errorf("advantage[%d]=%.12g want %.12g", i, adv[i], wantAdv[i])
		}
		if math.Abs(tgt[i]-wantTgt[i]) > 1e-12 {
			t.Errorf("target[%d]=%.12g want %.12g", i, tgt[i], wantTgt[i])
		}
	}
	if math.Abs(adv[2]-(-2.0)) < 1e-6 {
		t.Errorf("timeout last-step advantage %v must differ from terminal (Done) value -2", adv[2])
	}
	if math.Abs(tgt[2]-1.0) < 1e-6 {
		t.Errorf("timeout last-step target %v must differ from terminal (Done) value 1", tgt[2])
	}
}

func TestAdvantagesRejects(t *testing.T) {
	cfg := GAEConfig{Gamma: 0.9, Lambda: 0.8}

	if adv, tgt, err := Advantages([]Transition{{Value: 1}, {Value: 2}}, cfg); err == nil {
		t.Error("last step neither done nor timeout: expected error")
	} else {
		t.Logf("must-end error: %v", err)
		if !strings.Contains(err.Error(), "must end") {
			t.Errorf("must-end error %q missing %q", err, "must end")
		}
		if adv != nil || tgt != nil {
			t.Error("on error advantages/targets must be nil")
		}
	}

	if _, _, err := Advantages([]Transition{{Value: 1, Reward: 0, Done: true, Timeout: true}}, cfg); err == nil {
		t.Error("same step done and timeout: expected error")
	} else {
		t.Logf("done+timeout error: %v", err)
	}

	if _, _, err := Advantages([]Transition{{Value: math.NaN(), Reward: 0, Done: true}}, cfg); err == nil {
		t.Error("NaN reward: expected error")
	} else {
		t.Logf("NaN error: %v", err)
	}

	if _, _, err := Advantages(nil, cfg); err == nil {
		t.Error("empty steps: expected error")
	} else {
		t.Logf("empty error: %v", err)
	}

	if err := (GAEConfig{Gamma: 0, Lambda: 0.8}).Validate(); err == nil {
		t.Error("gamma=0: expected Validate error")
	} else {
		t.Logf("gamma=0 error: %v", err)
	}
	if err := (GAEConfig{Gamma: 0.9, Lambda: 1.5}).Validate(); err == nil {
		t.Error("lambda=1.5: expected Validate error")
	} else {
		t.Logf("lambda=1.5 error: %v", err)
	}
	if err := (GAEConfig{Gamma: 1, Lambda: 1}).Validate(); err != nil {
		t.Errorf("valid config: unexpected error %v", err)
	}
}

func TestAdvantagesMultipleEpisodesInOneRollout(t *testing.T) {
	cfg := GAEConfig{Gamma: 0.9, Lambda: 0.8}
	steps := []Transition{
		{Value: 1, Reward: 0},
		{Value: 2, Reward: 0.5, Done: true},
		{Value: 3, Reward: 0},
		{Value: 2, Reward: 0},
		{Value: 3, Reward: 1, Timeout: true, BootstrapValue: 4},
	}
	adv, tgt, err := Advantages(steps, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sub := steps[2:]
	advSub, tgtSub, err := Advantages(sub, cfg)
	if err != nil {
		t.Fatalf("unexpected sub error: %v", err)
	}
	t.Logf("full advantages=%v targets=%v", adv, tgt)
	t.Logf("sub  advantages=%v targets=%v", advSub, tgtSub)
	for i := range sub {
		full := i + 2
		if math.Abs(adv[full]-advSub[i]) > 1e-12 {
			t.Errorf("advantage[%d]=%.12g in full rollout != %.12g alone", full, adv[full], advSub[i])
		}
		if math.Abs(tgt[full]-tgtSub[i]) > 1e-12 {
			t.Errorf("target[%d]=%.12g in full rollout != %.12g alone", full, tgt[full], tgtSub[i])
		}
	}
}
