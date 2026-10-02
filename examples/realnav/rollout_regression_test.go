package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func nonzeroRolloutModel(t *testing.T) savedModel {
	t.Helper()
	model := rolloutTestModel(t, []string{"trial-a"})
	for i := range model.Snapshot.Parameters.Readout {
		model.Snapshot.Parameters.Readout[i] = float64(i%5+1) * .03
	}
	trainer, err := learning.NewTrainer(model.Snapshot.Config, model.Snapshot.Parameters, model.Snapshot.Options)
	if err != nil {
		t.Fatal(err)
	}
	model.Snapshot = trainer.Snapshot()
	model.TrainMeanDisplacement = .01
	return model
}

func TestRolloutNonzeroReadoutMatchesChunkOracleAndFreezesWholeSnapshot(t *testing.T) {
	model := nonzeroRolloutModel(t)
	before, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	start := rolloutStart{TrialID: "trial-a", Condition: "rewarded", XCM: 3, YCM: 2, PreviousDT: .2, Target: rolloutTarget{XCM: -20, YCM: -10}}
	trace, err := simulateRolloutTrace(context.Background(), model, start, trainingChunk+3, rolloutStrategyModel)
	if err != nil {
		t.Fatal(err)
	}
	trainer, err := restoreTrainer(model.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	inputs := make([][]float64, len(trace))
	for i, step := range trace {
		inputs[i] = sampleInput(step.Input)
	}
	var nonzero bool
	for offset := 0; offset < len(trace); offset += trainingChunk {
		end := min(offset+trainingChunk, len(trace))
		predictions, err := trainer.PredictAll(context.Background(), inputs[offset:end])
		if err != nil {
			t.Fatal(err)
		}
		for i, residual := range predictions {
			base := directionBaseline(causalSample{Input: inputs[offset+i]}, model.TrainMeanDisplacement)
			want := [2]float64{residual[0] + base[0], residual[1] + base[1]}
			if math.Hypot(want[0], want[1]) > maxActionLengthCM {
				t.Fatal("oracle fixture unexpectedly needs action clipping")
			}
			got := [2]float64{trace[offset+i].ActionDXCM, trace[offset+i].ActionDYCM}
			if got != want {
				t.Fatalf("decision %d action = %v, want episode-chunk oracle %v", offset+i, got, want)
			}
			nonzero = nonzero || residual[0] != 0 || residual[1] != 0
		}
	}
	if !nonzero {
		t.Fatal("oracle did not exercise a nonzero neural readout")
	}
	unreset, err := trainer.PredictAll(context.Background(), inputs)
	if err != nil {
		t.Fatal(err)
	}
	reset, err := trainer.PredictAll(context.Background(), inputs[trainingChunk:])
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(unreset[trainingChunk:], reset) {
		t.Fatal("fixture cannot distinguish persistent state from the required chunk reset")
	}
	after, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("rollout changed saved parameters, optimizer, progress, or configuration")
	}
}

func TestRolloutAllActionsIgnoreTargetsAndFutureRecordedPositions(t *testing.T) {
	model := nonzeroRolloutModel(t)
	dataset := rolloutDataset([]string{"trial-a"}, false)
	targets := []rolloutTarget{{TrialID: "trial-a", XCM: 20, YCM: 10}}
	starts, _, err := buildRolloutStarts(dataset, model.Split.Test, targets)
	if err != nil {
		t.Fatal(err)
	}
	first, err := simulateRolloutTrace(context.Background(), model, starts[0], 30, rolloutStrategyModel)
	if err != nil {
		t.Fatal(err)
	}
	dataset.Rows[len(dataset.Rows)-1].XCM = -29
	dataset.Rows[len(dataset.Rows)-1].YCM = 0
	targets[0].XCM = -20
	targets[0].YCM = -10
	changedStarts, _, err := buildRolloutStarts(dataset, model.Split.Test, targets)
	if err != nil {
		t.Fatal(err)
	}
	second, err := simulateRolloutTrace(context.Background(), model, changedStarts[0], 30, rolloutStrategyModel)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("target or future recorded positions changed generated observations/actions")
	}
}

func TestRolloutInitialHitHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := rolloutStart{TrialID: "trial-a", XCM: 5, YCM: 0, PreviousDT: .1, Target: rolloutTarget{XCM: 5}}
	if _, err := simulateRolloutTrial(ctx, rolloutTestModel(t, []string{"trial-a"}), start, 1, rolloutStrategyZero); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled initial hit returned %v", err)
	}
}

func TestRolloutInitialHitStillValidatesStartAndStrategy(t *testing.T) {
	model := rolloutTestModel(t, []string{"trial-a"})
	start := rolloutStart{TrialID: "trial-a", XCM: 31, PreviousDT: .1, Target: rolloutTarget{XCM: 31}}
	if _, err := simulateRolloutTrial(context.Background(), model, start, 1, rolloutStrategyZero); err == nil {
		t.Fatal("initial hit accepted a start and target outside the arena")
	}
	start.XCM, start.Target.XCM = 5, 5
	if _, err := simulateRolloutTrial(context.Background(), model, start, 1, "unknown"); err == nil {
		t.Fatal("initial hit accepted an unknown strategy")
	}
}

func TestRolloutOutputRejectsRepositoryAndSymlink(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	data := filepath.Join(root, "source", "data.csv")
	if err := os.Mkdir(filepath.Dir(data), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(data, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "repo-link")
	if err := os.Symlink(cwd, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	for _, out := range []string{filepath.Join(cwd, "forbidden-rollout-output"), filepath.Join(link, "forbidden-rollout-output")} {
		if _, err := prepareOutputDirectory(out, data); err == nil {
			t.Fatalf("output under repository was accepted: %s", out)
		}
		if _, err := os.Lstat(out); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("rejected output changed filesystem: %v", err)
		}
	}
}

func TestRolloutExtremeFiniteActionRetainsDirectionWhenClipped(t *testing.T) {
	start := rolloutStart{TrialID: "trial-a", PreviousDT: .1, Target: rolloutTarget{XCM: 20, YCM: 10}}
	trace, err := simulateRolloutTrace(context.Background(), rolloutTestModel(t, []string{"trial-a"}), start, 1, rolloutStrategyFixedAction, [2]float64{math.MaxFloat64, math.MaxFloat64})
	if err != nil {
		t.Fatal(err)
	}
	want := 5 / math.Sqrt(2)
	if len(trace) != 1 || trace[0].Clipped != 1 || math.Abs(trace[0].RealizedDXCM-want) > 1e-12 || math.Abs(trace[0].RealizedDYCM-want) > 1e-12 {
		t.Fatalf("extreme finite action lost its direction: %+v", trace)
	}
}

func TestRolloutProductionStopsAtTheFirstOffOriginHit(t *testing.T) {
	start := rolloutStart{TrialID: "trial-a", XCM: 1, PreviousDXCM: 1, PreviousDT: .1, Target: rolloutTarget{XCM: 5}}
	got, err := simulateRolloutTrial(context.Background(), rolloutTestModel(t, []string{"trial-a"}), start, 200, rolloutStrategyPersistent)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Success || got.StartSuccess || got.HitStep != 1 || got.Steps != 1 || got.PathCM != 2 || got.FinalDistanceCM != 2 || got.Collisions != 0 {
		t.Fatalf("first-hit result = %+v, want one 2cm action ending at x=3", got)
	}
}
