package rl_test

import (
	"context"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/learning/rl"
	"github.com/TimLai666/coimnet/plasticity"
)

func TestUpdateRejectsNonFreshInitialNeuralState(t *testing.T) {
	ctx := context.Background()
	ind := newPPOIndividual(t, 31, .02)
	roll := collectRollout(ctx, t, ind, 0)
	if err := ind.ResetNeural(ctx, make([]float64, ppoNodes)); err != nil {
		t.Fatalf("ResetNeural: %v", err)
	}
	if _, err := ind.Advance(ctx, [][]float64{{1, 0, 0, 0}}); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	legalNonZero := ind.Snapshot().Neural
	if legalNonZero.Continuous == nil {
		t.Fatalf("fixture did not produce a continuous state: %+v", legalNonZero)
	}
	if reflect.DeepEqual(legalNonZero, roll.InitialNeural) {
		t.Fatal("fixture did not produce a distinct legal non-zero state")
	}
	roll.InitialNeural = legalNonZero
	before := snapshotJSON(t, ind)
	_, _, err := rl.Update(ctx, ind, []rl.Rollout{roll}, ppoActions, ppoUpdateConfig(1, 0, 0))
	if err == nil || !strings.Contains(err.Error(), "zero-voltage") {
		t.Fatalf("Update error = %v, want fresh zero-voltage refusal", err)
	}
	if got := snapshotJSON(t, ind); string(got) != string(before) {
		t.Fatal("individual changed after rejecting a non-fresh initial state")
	}
}

func TestPolicyVersionBindsConfigAndCoreBias(t *testing.T) {
	ind := newPPOIndividual(t, 32, .02)
	base := ind.Snapshot()

	config := base.Config
	config.Dynamics.DT += .25
	changedConfig, err := learning.NewIndividual(config, base.Parameters, base.Optimizer.Options, make([]float64, ppoNodes))
	if err != nil {
		t.Fatalf("NewIndividual with changed config: %v", err)
	}
	if got, want := rl.PolicyVersion(changedConfig), rl.PolicyVersion(ind); got == want {
		t.Fatalf("PolicyVersion ignored config change: %q", got)
	}

	parameters := base.Parameters
	parameters.Core.Bias[0] = .125
	changedBias, err := learning.NewIndividual(base.Config, parameters, base.Optimizer.Options, make([]float64, ppoNodes))
	if err != nil {
		t.Fatalf("NewIndividual with changed core bias: %v", err)
	}
	if got, want := rl.PolicyVersion(changedBias), rl.PolicyVersion(ind); got == want {
		t.Fatalf("PolicyVersion ignored trainable core bias change: %q", got)
	}
}

func TestUpdateRejectsUnsupportedMechanisms(t *testing.T) {
	ctx := context.Background()
	ind := newPPOIndividual(t, 33, .02)
	if err := ind.EnablePlasticity(plasticity.Config{
		Rule:  plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: .1, WMin: .0001},
		Edges: []int{0},
	}); err != nil {
		t.Fatalf("EnablePlasticity: %v", err)
	}
	base := ind.Snapshot()
	roll := rl.Rollout{
		PolicyVersion:   rl.PolicyVersion(ind),
		InitialNeural:   base.Neural,
		InitialPlastic:  base.Plastic,
		InitialChemical: base.Chemical,
		Steps:           []rl.Transition{{Obs: make([]float64, ppoInputs), Action: 0, LogProb: -math.Log(ppoActions), Done: true}},
	}
	before := snapshotJSON(t, ind)
	_, _, err := rl.Update(ctx, ind, []rl.Rollout{roll}, ppoActions, ppoUpdateConfig(1, 0, 0))
	if err == nil || !strings.Contains(err.Error(), "unsupported") || !strings.Contains(err.Error(), "plastic") {
		t.Fatalf("Update error = %v, want unsupported plasticity refusal", err)
	}
	if got := snapshotJSON(t, ind); string(got) != string(before) {
		t.Fatal("individual changed after rejecting unsupported plasticity")
	}
}

func TestUpdateRejectsMiniBatchAndTimeLimitMismatch(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		edit func(*rl.PPOConfig, *rl.Rollout)
		want string
	}{
		{
			name: "mini batch",
			edit: func(c *rl.PPOConfig, _ *rl.Rollout) { c.MiniBatch = 2 },
			want: "mini_batch",
		},
		{
			name: "rollout exceeds time limit",
			edit: func(c *rl.PPOConfig, r *rl.Rollout) { c.TimeLimit = len(r.Steps) - 1 },
			want: "time_limit",
		},
		{
			name: "timeout before declared limit",
			edit: func(c *rl.PPOConfig, r *rl.Rollout) {
				c.TimeLimit = len(r.Steps) + 1
				r.Steps[len(r.Steps)-1].Timeout = true
				r.Steps[len(r.Steps)-1].Done = false
			},
			want: "timeout",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ind := newPPOIndividual(t, 34, .02)
			r := collectRollout(ctx, t, ind, 0)
			c := ppoUpdateConfig(1, 0, 0)
			tc.edit(&c, &r)
			before := snapshotJSON(t, ind)
			_, _, err := rl.Update(ctx, ind, []rl.Rollout{r}, ppoActions, c)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Update error = %v, want %q refusal", err, tc.want)
			}
			if got := snapshotJSON(t, ind); string(got) != string(before) {
				t.Fatal("individual changed after validation refusal")
			}
		})
	}
}

func TestUpdateRejectsFullBurnInAndInvalidBurnInAction(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		edit func(rl.Rollout, rl.PPOConfig) (rl.Rollout, rl.PPOConfig)
		want string
	}{
		{
			name: "full burn in",
			edit: func(r rl.Rollout, c rl.PPOConfig) (rl.Rollout, rl.PPOConfig) {
				c.BurnIn = len(r.Steps)
				c.ValueCoef = 0
				c.EntropyCoef = 0
				return r, c
			},
			want: "burn_in",
		},
		{
			name: "invalid burn in action",
			edit: func(r rl.Rollout, c rl.PPOConfig) (rl.Rollout, rl.PPOConfig) {
				c.BurnIn = 1
				r.Steps[0].Action = ppoActions
				return r, c
			},
			want: "action",
		},
		{
			name: "non-finite burn in log prob",
			edit: func(r rl.Rollout, c rl.PPOConfig) (rl.Rollout, rl.PPOConfig) {
				c.BurnIn = 1
				r.Steps[0].LogProb = math.NaN()
				return r, c
			},
			want: "non-finite",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ind := newPPOIndividual(t, 35, .02)
			r := collectRollout(ctx, t, ind, 0)
			if len(r.Steps) < 2 {
				t.Fatalf("fixture rollout too short for burn-in validation: %d", len(r.Steps))
			}
			c := ppoUpdateConfig(1, 0, 0)
			r, c = tc.edit(r, c)
			before := snapshotJSON(t, ind)
			_, _, err := rl.Update(ctx, ind, []rl.Rollout{r}, ppoActions, c)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Update error = %v, want %q refusal", err, tc.want)
			}
			if got := snapshotJSON(t, ind); string(got) != string(before) {
				t.Fatal("individual changed after burn-in validation refusal")
			}
		})
	}
}

func TestUpdateValidatesAllRolloutsBeforeCandidateUpdate(t *testing.T) {
	ctx := context.Background()
	ind := newPPOIndividual(t, 36, .02)
	first := collectRollout(ctx, t, ind, 0)
	second := collectRollout(ctx, t, ind, 1)
	if len(second.Steps) < 2 {
		t.Fatalf("fixture rollout too short for validation test: %d", len(second.Steps))
	}
	second.Steps[0].Action = ppoActions
	before := snapshotJSON(t, ind)
	_, _, err := rl.Update(ctx, ind, []rl.Rollout{first, second}, ppoActions, ppoUpdateConfig(2, 1, 0))
	if err == nil || !strings.Contains(err.Error(), "action") {
		t.Fatalf("Update error = %v, want invalid action refusal", err)
	}
	if got := snapshotJSON(t, ind); string(got) != string(before) {
		t.Fatal("individual changed before all rollouts were validated")
	}
}
