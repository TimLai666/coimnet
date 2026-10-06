package rl_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/learning/rl"
)

func TestStatefulPPODelayedHistoryMatchesIndependentReplay(t *testing.T) {
	ctx := context.Background()
	base := newPPOIndividual(t, 107, .015).Snapshot()
	base.Config.Dynamics.Delays = make([]int, len(base.Config.Dynamics.Sources))
	for edge := range base.Config.Dynamics.Delays {
		base.Config.Dynamics.Delays[edge] = edge % 4
	}
	ind, err := learning.NewIndividual(base.Config, base.Parameters, base.Optimizer.Options, make([]float64, ppoNodes))
	if err != nil {
		t.Fatal(err)
	}
	// At step two, delay-three edges still read the saved time-zero row.
	if _, err := ind.Advance(ctx, [][]float64{{.7, -.2, .1, .4}, {-.3, .1, .5, -.2}}); err != nil {
		t.Fatal(err)
	}
	initial := ind.Snapshot()
	if initial.Neural.Continuous.Steps != 2 || len(initial.Neural.Continuous.History) != 3 {
		t.Fatal("fixture must retain a nonzero state and delayed prefix")
	}
	input := [][]float64{{.2, -.1, .5, .4}, {-.2, .3, 0, .1}, {.5, .2, -.1, .3}, {0, .4, -.3, .2}}
	outputs, err := ind.Advance(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	roll := statefulRollout(t, ind, initial, input, outputs)
	before := snapshotJSON(t, ind)
	cfg := ppoUpdateConfig(2, 1, .03)
	updated, _, err := rl.Update(ctx, ind, []rl.Rollout{roll}, ppoActions, cfg)
	if err != nil {
		t.Fatal(err)
	}
	wantParameters, wantOptimizer := manualStatefulPPO(t, ctx, initial, ind.Snapshot(), roll, cfg)
	got := updated.Snapshot()
	if !reflect.DeepEqual(got.Parameters, wantParameters) || !reflect.DeepEqual(got.Optimizer, wantOptimizer) {
		t.Fatal("delayed PPO differs from independent same-state scoring and gradient replay")
	}
	if string(snapshotJSON(t, ind)) != string(before) || !reflect.DeepEqual(got.Neural, ind.Snapshot().Neural) {
		t.Fatal("delayed PPO changed the caller or its current neural state")
	}
}
