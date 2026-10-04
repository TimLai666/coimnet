package experiment

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/learning/rl"
)

func ppoHorizonFixtureRollout() rl.Rollout {
	return rl.Rollout{PolicyVersion: "ppo-horizon-validation/v1", Steps: []rl.Transition{
		{Obs: []float64{0, 0, 0, 0}, Action: 0, LogProb: -math.Log(3), Value: 1},
		{Obs: []float64{0, 1, 0, 0}, Action: 1, LogProb: -math.Log(3), Value: 2},
		{Obs: []float64{0, 0, 1, 0}, Action: 2, LogProb: -math.Log(3), Value: 3, Reward: 1, Timeout: true, BootstrapValue: 4},
	}}
}

func ppoHorizonMutated(mutate func(*rl.Rollout)) rl.Rollout {
	r := ppoHorizonFixtureRollout()
	mutate(&r)
	return r
}

// With gamma=.9/lambda=.8: A2=-2, A1=.7+.72*(-2)=-.74,
// A0=.8+.72*(-.74)=.2672. Targets add the original values.
func TestPPOHorizonDeadlineRolloutGAE(t *testing.T) {
	source := ppoHorizonFixtureRollout()
	before := hash(source)
	got, err := deadlineRollout(source, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Steps) != 3 {
		t.Fatalf("step count %d", len(got.Steps))
	}
	last := got.Steps[2]
	if !last.Done || last.Timeout || last.BootstrapValue != 0 {
		t.Fatalf("ending %+v", last)
	}
	if !reflect.DeepEqual(got.Steps[:2], source.Steps[:2]) {
		t.Fatal("earlier steps changed")
	}
	adv, tgt, err := rl.Advantages(got.Steps, rl.GAEConfig{Gamma: .9, Lambda: .8})
	if err != nil {
		t.Fatal(err)
	}
	wantAdv, wantTgt := []float64{.2672, -.74, -2}, []float64{1.2672, 1.26, 1}
	for i := range wantAdv {
		if !finite(adv[i]) || !finite(tgt[i]) || math.Abs(adv[i]-wantAdv[i]) > 1e-12 || math.Abs(tgt[i]-wantTgt[i]) > 1e-12 {
			t.Fatalf("GAE %v/%v", adv, tgt)
		}
	}
	if hash(source) != before {
		t.Fatal("original changed")
	}
	got.Steps[0].Obs[0] = 99
	if hash(source) != before {
		t.Fatal("observations alias original")
	}
	success := ppoHorizonMutated(func(r *rl.Rollout) { r.Steps[2].Timeout = false; r.Steps[2].Done = true; r.Steps[2].BootstrapValue = 0 })
	kept, err := deadlineRollout(success, 6)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(kept, success) {
		t.Fatal("successful episode changed")
	}
	kept.Steps[1].Obs[1] = -99
	if success.Steps[1].Obs[1] != 1 {
		t.Fatal("successful observations alias original")
	}
}

func TestPPOHorizonDeadlineRolloutRejectsInvalidEpisodes(t *testing.T) {
	cases := []struct {
		name  string
		limit int
		roll  rl.Rollout
		want  string
	}{
		{"nil", 3, rl.Rollout{}, "empty"},
		{"empty", 3, rl.Rollout{Steps: []rl.Transition{}}, "empty"},
		{"zero limit", 0, ppoHorizonFixtureRollout(), "limit"},
		{"negative limit", -1, ppoHorizonFixtureRollout(), "limit"},
		{"too long", 2, ppoHorizonFixtureRollout(), "limit"},
		{"short timeout", 6, ppoHorizonFixtureRollout(), "timeout"},
		{"early timeout", 3, ppoHorizonMutated(func(r *rl.Rollout) { r.Steps[1].Timeout = true }), "timeout"},
		{"no ending", 3, ppoHorizonMutated(func(r *rl.Rollout) { r.Steps[2].Timeout = false; r.Steps[2].BootstrapValue = 0 }), "end"},
		{"conflict", 3, ppoHorizonMutated(func(r *rl.Rollout) { r.Steps[2].Done = true }), "both"},
		{"early done", 3, ppoHorizonMutated(func(r *rl.Rollout) { r.Steps[0].Done = true }), "done"},
		{"nan value", 3, ppoHorizonMutated(func(r *rl.Rollout) { r.Steps[0].Value = math.NaN() }), "finite"},
		{"infinite bootstrap", 3, ppoHorizonMutated(func(r *rl.Rollout) { r.Steps[2].BootstrapValue = math.Inf(1) }), "finite"},
		{"nan observation", 3, ppoHorizonMutated(func(r *rl.Rollout) { r.Steps[2].Obs[2] = math.NaN() }), "finite"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := deadlineRollout(tc.roll, tc.limit); err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("error %v, want %q", err, tc.want)
			}
		})
	}
}

func TestPPOHorizonArmsShareTheSameCollection(t *testing.T) {
	ctx := context.Background()
	c := shortGoalConfig()
	c.Updates = 1
	ctrl, cp, err := trainPPOHorizonFixture(ctx, c, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	dl, dp, err := trainPPOHorizonFixture(ctx, c, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if ctrl == nil || dl == nil || len(cp) != 1 || len(dp) != 1 {
		t.Fatal("incomplete training")
	}
	want, err := trainPPOGoalFixture(ctx, c, 1)
	if err != nil {
		t.Fatal(err)
	}
	if hash(ctrl.Snapshot()) != hash(want.Snapshot()) {
		t.Fatal("control differs from original trainer")
	}
	fresh, err := newPPOIndividual(1, c.Hidden, c.LearningRate)
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range []ppoHorizonPoint{cp[0], dp[0]} {
		ind := ctrl
		if i == 1 {
			ind = dl
		}
		f := p.Feedback
		if f.Seed != 1 || f.Update != 1 || f.EnvSeed != imitationTrainSeed(1, 0) || f.SnapshotBefore != hash(fresh.Snapshot()) || f.SnapshotAfter != hash(ind.Snapshot()) || ind.Snapshot().Optimizer.Updates != 1 {
			t.Fatal("incorrect update bookkeeping")
		}
	}
	a, b := cp[0].Feedback.Rollout.Steps, dp[0].Feedback.Rollout.Steps
	if len(a) != 6 || len(b) != 6 {
		t.Fatal("expected six steps")
	}
	raw := append([]rl.Transition(nil), b...)
	raw[5] = dp[0].CollectedEnd
	if !reflect.DeepEqual(a, raw) || !reflect.DeepEqual(cp[0].CollectedEnd, raw[5]) {
		t.Fatal("original collection differs")
	}
	if !raw[5].Timeout || raw[5].Done || raw[5].BootstrapValue <= 0 {
		t.Fatal("expected original timeout and positive bootstrap")
	}
	if !b[5].Done || b[5].Timeout || b[5].BootstrapValue != 0 {
		t.Fatal("deadline ending incorrect")
	}
	fb := dp[0].Feedback
	adv, tgt, err := rl.Advantages(b, rl.GAEConfig{Gamma: c.PPO.Gamma, Lambda: c.PPO.Lambda})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fb.Advantages, adv) || !reflect.DeepEqual(fb.Targets, tgt) || math.Abs(fb.Return+.06) > 1e-12 || math.Abs(tgt[5]+.01) > 1e-12 {
		t.Fatal("deadline targets/return incorrect")
	}
	// The recurrent state and the separately recorded original end must not alias.
	source := cp[0].Feedback.Rollout
	before := hash(source)
	converted, err := deadlineRollout(source, 6)
	if err != nil {
		t.Fatal(err)
	}
	converted.InitialNeural.Continuous.Voltage[0] = 99
	if hash(source) != before {
		t.Fatal("initial neural state aliases original")
	}
	endHash := hash(dp[0].CollectedEnd)
	dp[0].Feedback.Rollout.Steps[5].Obs[0] = 99
	if hash(dp[0].CollectedEnd) != endHash {
		t.Fatal("collected end aliases transformed rollout")
	}
}

func TestPPOHorizonTrainingRejectsInvalidInput(t *testing.T) {
	c := shortGoalConfig()
	c.Updates = 1
	zeroUpdates, zeroRate := c, c
	zeroUpdates.Updates, zeroRate.LearningRate = 0, 0
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	cases := []struct {
		name string
		ctx  context.Context
		c    PPOExperimentConfig
		want string
	}{
		{"nil", nil, c, "context"}, {"canceled", canceled, c, "context"},
		{"zero updates", context.Background(), zeroUpdates, "config"}, {"zero rate", context.Background(), zeroRate, "config"},
	}
	for _, deadline := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				ind, points, err := trainPPOHorizonFixture(tc.ctx, tc.c, 1, deadline)
				if ind != nil || points != nil || err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.want) {
					t.Fatalf("input rejection: model=%v points=%d error=%v", ind != nil, len(points), err)
				}
				if tc.ctx == canceled && !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation identity lost: %v", err)
				}
			})
		}
	}
}
