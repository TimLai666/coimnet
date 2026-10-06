package rl_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/learning/rl"
)

// The child loads the actual checkpoint bytes, not a recreated fixture, and
// repeats a multi-epoch update from the saved nonzero rollout boundary.
func TestStatefulPPOCheckpointNewProcess(t *testing.T) {
	ctx := context.Background()
	if dir := os.Getenv("COIMNET_STATEFUL_PPO_CHILD"); dir != "" {
		s, err := checkpoint.LoadIndividual(ctx, filepath.Join(dir, "before.json"))
		if err != nil {
			t.Fatal(err)
		}
		ind, err := learning.RestoreIndividual(s)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(dir, "rollout.json"))
		if err != nil {
			t.Fatal(err)
		}
		var roll rl.Rollout
		if err := json.Unmarshal(data, &roll); err != nil {
			t.Fatal(err)
		}
		updated, _, err := rl.Update(ctx, ind, []rl.Rollout{roll}, ppoActions, ppoUpdateConfig(3, 1, .03))
		if err != nil {
			t.Fatal(err)
		}
		if err := checkpoint.SaveIndividual(ctx, filepath.Join(dir, "child.json"), updated.Snapshot()); err != nil {
			t.Fatal(err)
		}
		return
	}
	ind := newPPOIndividual(t, 103, .015)
	seedRoll := collectRollout(ctx, t, ind, 1)
	ind, _, err := rl.Update(ctx, ind, []rl.Rollout{seedRoll}, ppoActions, ppoUpdateConfig(1, 0, .03))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := ind.Snapshot()
	snapshot.Optimizer.Options.AccumulateSteps = 2
	ind, err = learning.RestoreIndividual(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ind.Advance(ctx, [][]float64{{.5, -.2, .3, .1}, {.2, .1, -.1, .4}}); err != nil {
		t.Fatal(err)
	}
	roll := rl.Rollout{PolicyVersion: rl.PolicyVersion(ind), InitialNeural: ind.Snapshot().Neural}
	for i, obs := range [][]float64{{.2, .3, -.1, .4}, {-.2, .1, .6, 0}, {.5, 0, .2, -.1}, {.1, -.1, .2, .7}} {
		y, err := ind.Advance(ctx, [][]float64{obs})
		if err != nil {
			t.Fatal(err)
		}
		action := i % ppoActions
		logp, err := rl.LogProb(y[0][:ppoActions], action)
		if err != nil {
			t.Fatal(err)
		}
		roll.Steps = append(roll.Steps, rl.Transition{Obs: obs, Action: action, LogProb: logp, Value: y[0][ppoActions], Reward: .2, Done: i == 3})
	}
	ind, _, err = rl.Update(ctx, ind, []rl.Rollout{roll}, ppoActions, ppoUpdateConfig(1, 1, .03))
	if err != nil {
		t.Fatal(err)
	}
	before := ind.Snapshot()
	if before.Optimizer.Accumulator == nil || before.Optimizer.Updates == 0 {
		t.Fatal("fixture must persist both prior Adam updates and an open accumulation window")
	}
	dir := t.TempDir()
	if err := checkpoint.SaveIndividual(ctx, filepath.Join(dir, "before.json"), ind.Snapshot()); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(roll)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rollout.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	updated, _, err := rl.Update(ctx, ind, []rl.Rollout{roll}, ppoActions, ppoUpdateConfig(3, 1, .03))
	if err != nil {
		t.Fatal(err)
	}
	if err := checkpoint.SaveIndividual(ctx, filepath.Join(dir, "direct.json"), updated.Snapshot()); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestStatefulPPOCheckpointNewProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), "COIMNET_STATEFUL_PPO_CHILD="+dir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child: %v\n%s", err, output)
	}
	direct, err := os.ReadFile(filepath.Join(dir, "direct.json"))
	if err != nil {
		t.Fatal(err)
	}
	child, err := os.ReadFile(filepath.Join(dir, "child.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(direct, child) {
		t.Fatal("new-process checkpoint differs from uninterrupted stateful PPO update")
	}
	t.Log("checkpoint bytes, parameters, optimizer, accumulator and neural state match across a new process")
}
