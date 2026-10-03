package main

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func TestMemoryArenaRejectsOutsideStepAndPreservesPosition(t *testing.T) {
	position, realized, collision := memoryArenaStep(29, 0, [2]float64{2, 0})
	if !collision || position != [2]float64{29, 0} || realized != [2]float64{} {
		t.Fatalf("outside step = position %v realized %v collision %t", position, realized, collision)
	}
	position, realized, collision = memoryArenaStep(1, 2, [2]float64{1, -1})
	if collision || position != [2]float64{2, 1} || realized != [2]float64{1, -1} {
		t.Fatalf("inside step = position %v realized %v collision %t", position, realized, collision)
	}
}

func TestMemoryBaselinesDoNotReadGoalForActionPath(t *testing.T) {
	trial := testHistoryTrial(32)
	far := memoryRolloutTarget{TrialID: trial.ID, XCM: 20, YCM: 20, EvaluationOnly: true}
	near := memoryRolloutTarget{TrialID: trial.ID, XCM: -20, YCM: -20, EvaluationOnly: true}
	left, err := memoryBaselineTrial(context.Background(), trial, far, "random", 20261003, .2)
	if err != nil {
		t.Fatal(err)
	}
	right, err := memoryBaselineTrial(context.Background(), trial, near, "random", 20261003, .2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(left.Trace, right.Trace) {
		t.Fatal("random baseline action path changed when only evaluation target changed")
	}
}

func TestMemoryModelRolloutDoesNotReadFutureRowsAndFreezesSnapshot(t *testing.T) {
	config, parameters, err := newModel(20261003)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := learning.TrainingSnapshot{SchemaVersion: "coimnet-episode-training/v1", Config: config, Parameters: parameters, Options: trainingOptions()}
	trial := testHistoryTrial(32)
	target := memoryRolloutTarget{TrialID: trial.ID, XCM: 20, YCM: 20, EvaluationOnly: true}
	first, err := learning.NewIndividual(snapshot.Config, snapshot.Parameters, snapshot.Options, make([]float64, config.Dynamics.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	left, err := memoryModelTrial(context.Background(), first, trial, target, "delivered", 20261003)
	if err != nil {
		t.Fatal(err)
	}
	future := trial
	future.Steps = append([]historyStep(nil), trial.Steps...)
	for i := 17; i < len(future.Steps); i++ {
		future.Steps[i].Input[0] += 100
		future.Steps[i].Input[1] -= 100
		future.Steps[i].Input[5] = 1 - future.Steps[i].Input[5]
	}
	second, err := learning.NewIndividual(snapshot.Config, snapshot.Parameters, snapshot.Options, make([]float64, config.Dynamics.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	right, err := memoryModelTrial(context.Background(), second, future, target, "delivered", 20261003)
	if err != nil {
		t.Fatal(err)
	}
	if len(left.Trace) == 0 || len(right.Trace) == 0 || left.Trace[0].Action != right.Trace[0].Action {
		t.Fatalf("future rows changed first generated action: %v vs %v", left.Trace[0].Action, right.Trace[0].Action)
	}
	for i := range left.Trace {
		if left.Trace[i].Action != right.Trace[i].Action {
			t.Fatalf("future row changed generated action at step %d", i)
		}
	}
	if !finite(left.FinalDistCM) || math.IsNaN(left.FinalDistCM) {
		t.Fatalf("invalid rollout distance %v", left.FinalDistCM)
	}
	if memoryJSONHash(first.Snapshot().Parameters) != memoryJSONHash(snapshot.Parameters) {
		t.Fatal("rollout changed frozen snapshot parameters")
	}
}

func TestMemoryRolloutCancellationDoesNotProducePartialTrial(t *testing.T) {
	trial := testHistoryTrial(32)
	target := memoryRolloutTarget{TrialID: trial.ID, XCM: 20, YCM: 20, EvaluationOnly: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	config, parameters, err := newModel(20261003)
	if err != nil {
		t.Fatal(err)
	}
	individual, err := learning.NewIndividual(config, parameters, trainingOptions(), make([]float64, config.Dynamics.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memoryModelTrial(ctx, individual, trial, target, "delivered", 20261003); err == nil {
		t.Fatal("canceled rollout returned a partial result")
	}
}

func TestMemoryModelRolloutResetsStateForEachTrial(t *testing.T) {
	config, parameters, err := newModel(20261003)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := learning.TrainingSnapshot{SchemaVersion: "coimnet-episode-training/v1", Config: config, Parameters: parameters, Options: trainingOptions()}
	first := testHistoryTrial(32)
	second := testHistoryTrial(32)
	second.ID = "trial-b"
	targets := map[string]memoryRolloutTarget{
		first.ID:  {TrialID: first.ID, XCM: 20, YCM: 20, EvaluationOnly: true},
		second.ID: {TrialID: second.ID, XCM: -20, YCM: -20, EvaluationOnly: true},
	}
	forward, err := memoryRunRollout(context.Background(), snapshot, []historyTrial{first, second}, targets, "delivered", 20261003, .2)
	if err != nil {
		t.Fatal(err)
	}
	reverse, err := memoryRunRollout(context.Background(), snapshot, []historyTrial{second, first}, targets, "delivered", 20261003, .2)
	if err != nil {
		t.Fatal(err)
	}
	forwardByID := make(map[string]memoryRolloutTrial, len(forward.Trials))
	for _, trial := range forward.Trials {
		forwardByID[trial.TrialID] = trial
	}
	for _, trial := range reverse.Trials {
		if !reflect.DeepEqual(forwardByID[trial.TrialID].Trace, trial.Trace) {
			t.Fatalf("trial %s action path depends on preceding trial state", trial.TrialID)
		}
	}
	if forward.OptimizerSHA256 != memoryJSONHash(snapshot.Optimizer) || !forward.OptimizerFrozen {
		t.Fatal("rollout report did not preserve the source optimizer hash")
	}
}

func TestMemoryShuffledRolloutContinuesCausallyAfterPrefix(t *testing.T) {
	trial := testHistoryTrial(64)
	trial.RolloutIndex = 20
	for i := range trial.Steps {
		trial.Steps[i].Input[5] = float64((i / 3) % 2)
	}
	target := memoryRolloutTarget{TrialID: trial.ID, XCM: 20, YCM: 20, EvaluationOnly: true}
	config, parameters, err := newModel(77)
	if err != nil {
		t.Fatal(err)
	}
	first, err := learning.NewIndividual(config, parameters, trainingOptions(), make([]float64, config.Dynamics.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	left, err := memoryModelTrial(context.Background(), first, trial, target, "shuffled_stimulus", 77)
	if err != nil {
		t.Fatal(err)
	}
	prefix := trial
	prefix.Steps = append([]historyStep(nil), trial.Steps[:trial.RolloutIndex+1]...)
	inputs, err := trialInputs(prefix, "shuffled_stimulus", 77)
	if err != nil {
		t.Fatal(err)
	}
	if left.Trace[0].Input[5] != inputs[trial.RolloutIndex][5] {
		t.Fatalf("rollout dropped transformed prefix stimulus: got %g want %g", left.Trace[0].Input[5], inputs[trial.RolloutIndex][5])
	}
	future := trial
	future.Steps = append([]historyStep(nil), trial.Steps...)
	for i := trial.RolloutIndex + 1; i < len(future.Steps); i++ {
		future.Steps[i].Input[0] += 1000
		future.Steps[i].Input[5] = 1 - future.Steps[i].Input[5]
	}
	second, err := learning.NewIndividual(config, parameters, trainingOptions(), make([]float64, config.Dynamics.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	right, err := memoryModelTrial(context.Background(), second, future, target, "shuffled_stimulus", 77)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(left.Trace, right.Trace) {
		t.Fatal("future stimulus or trajectory rows changed causal shuffled continuation")
	}
}

func TestMemoryShuffledRolloutMatchesRawCausalBlocksAfterGeneratedExtension(t *testing.T) {
	const seed uint64 = 77
	const rolloutIndex = 20

	trial := testHistoryTrial(rolloutIndex + 1)
	trial.RolloutIndex = rolloutIndex
	for i := range trial.Steps {
		trial.Steps[i].Input[5] = 0
	}
	// The last complete raw prefix block contains a positive stimulus. The
	// current row is in the next, incomplete block and has raw zero stimulus.
	for i := 0; i < memoryBlockRows; i++ {
		trial.Steps[i].Input[5] = 1
	}

	extended := trial
	extended.Steps = append([]historyStep(nil), trial.Steps...)
	for i := 0; i < memoryDecisionRows; i++ {
		step := historyStep{T: float64(len(extended.Steps)) * 0.1}
		step.Input[4] = 0.1
		extended.Steps = append(extended.Steps, step)
	}
	wantInputs, err := trialInputs(extended, "shuffled_stimulus", int64(seed))
	if err != nil {
		t.Fatal(err)
	}

	config, parameters, err := newModel(int64(seed))
	if err != nil {
		t.Fatal(err)
	}
	individual, err := learning.NewIndividual(config, parameters, trainingOptions(), make([]float64, config.Dynamics.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	target := memoryRolloutTarget{TrialID: trial.ID, XCM: 20, YCM: 20, EvaluationOnly: true}
	got, err := memoryModelTrial(context.Background(), individual, trial, target, "shuffled_stimulus", seed)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Trace) != memoryDecisionRows {
		t.Fatalf("rollout steps = %d, want %d", len(got.Trace), memoryDecisionRows)
	}
	for i, step := range got.Trace {
		want := wantInputs[rolloutIndex+i][5]
		if step.Input[5] != want {
			t.Fatalf("stimulus at rollout step %d (absolute row %d) = %g, want %g", i, rolloutIndex+i, step.Input[5], want)
		}
	}
	if got.Trace[0].Input[5] == 0 || got.Trace[1].Input[5] == 0 {
		t.Fatalf("positive raw prefix block was not carried into the first shuffled generated block: %g, %g", got.Trace[0].Input[5], got.Trace[1].Input[5])
	}
	if got.Trace[memoryBlockRows+1].Input[5] != 0 {
		t.Fatalf("generated zero block was echoed into the next shuffled block: %g", got.Trace[memoryBlockRows+1].Input[5])
	}
}

func TestMemoryShuffledRolloutPreservesRawCurrentRowStimulus(t *testing.T) {
	const seed uint64 = 77
	const rolloutIndex = memoryBlockRows

	trial := testHistoryTrial(rolloutIndex + 1)
	trial.RolloutIndex = rolloutIndex
	for i := range trial.Steps {
		trial.Steps[i].Input[5] = 0
	}
	trial.Steps[rolloutIndex].Input[5] = 1

	extended := trial
	extended.Steps = append([]historyStep(nil), trial.Steps...)
	for i := 0; i < memoryDecisionRows; i++ {
		step := historyStep{T: float64(len(extended.Steps)) * 0.1}
		step.Input[4] = 0.1
		extended.Steps = append(extended.Steps, step)
	}
	wantInputs, err := trialInputs(extended, "shuffled_stimulus", int64(seed))
	if err != nil {
		t.Fatal(err)
	}

	config, parameters, err := newModel(int64(seed))
	if err != nil {
		t.Fatal(err)
	}
	individual, err := learning.NewIndividual(config, parameters, trainingOptions(), make([]float64, config.Dynamics.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	target := memoryRolloutTarget{TrialID: trial.ID, XCM: 20, YCM: 20, EvaluationOnly: true}
	got, err := memoryModelTrial(context.Background(), individual, trial, target, "shuffled_stimulus", seed)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Trace) != memoryDecisionRows {
		t.Fatalf("rollout steps = %d, want %d", len(got.Trace), memoryDecisionRows)
	}
	for i, step := range got.Trace {
		want := wantInputs[rolloutIndex+i][5]
		if step.Input[5] != want {
			t.Fatalf("stimulus at rollout step %d (absolute row %d) = %g, want %g", i, rolloutIndex+i, step.Input[5], want)
		}
	}
}
