package learning_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func stageLearnerWithGate(t *testing.T, every uint64, accumulate int, gate learning.RewardGate) (*learning.Individual, *learning.OnlineLearner) {
	t.Helper()
	individual := onlinePlasticIndividual(t)
	options := individual.Snapshot().Optimizer.Options
	options.AccumulateSteps = accumulate
	if err := individual.ResetOptimizer(context.Background(), options); err != nil {
		t.Fatalf("ResetOptimizer(accumulate=%d): %v", accumulate, err)
	}
	policy := onlinePolicy()
	policy.AllowPlastic = true
	policy.UpdateEvery = every
	return individual, newLearner(t, individual, policy, gate)
}

func stageSetTarget(t *testing.T, learner *learning.OnlineLearner, action learning.ActionRecord, value float64) {
	t.Helper()
	if err := learner.SetTargets(map[string][]float64{action.ActionID: {value}}); err != nil {
		t.Fatalf("SetTargets(%s): %v", action.ActionID, err)
	}
}

func stageReceive(t *testing.T, learner *learning.OnlineLearner, action learning.ActionRecord, source string, available int64, score float64) {
	t.Helper()
	if err := learner.Receive(context.Background(), feedback(t, action.ActionID, source, int64(action.Step), available, score)); err != nil {
		t.Fatalf("Receive(%s/%s): %v", action.ActionID, source, err)
	}
}

func stageSequenceGate(failures int) learning.RewardGate {
	positiveCalls := 0
	return learning.RewardGateFunc(func(score float64) ([]float64, error) {
		if score <= 0 {
			return []float64{0}, nil
		}
		positiveCalls++
		if positiveCalls <= failures {
			return nil, errors.New("stage gate failure")
		}
		return []float64{1}, nil
	})
}

func stageScoreGate(failures map[float64]int) learning.RewardGate {
	remaining := make(map[float64]int, len(failures))
	for score, count := range failures {
		remaining[score] = count
	}
	return learning.RewardGateFunc(func(score float64) ([]float64, error) {
		if score <= 0 {
			return []float64{0}, nil
		}
		if remaining[score] > 0 {
			remaining[score]--
			return nil, errors.New("stage score gate failure")
		}
		return []float64{1}, nil
	})
}

// TestOnlineMixedRetryStageMarksMissingTargetComplete verifies that the
// gradient phase is decided even when no target exists. Adding a target after
// the plastic half failed must not make retry run a new gradient phase.
func TestOnlineMixedRetryStageMarksMissingTargetComplete(t *testing.T) {
	individual, learner := stageLearnerWithGate(t, 1, 1, stageSequenceGate(1))
	action := act(t, learner, onlineInput())
	stageReceive(t, learner, action, "teacher", int64(action.Step), 1)

	failedReport, err := learner.Update(context.Background(), at(int64(action.Step)))
	if err == nil || !strings.Contains(err.Error(), "stage gate failure") {
		t.Fatalf("first Update report=%+v err=%v, want gate failure", failedReport, err)
	}
	if failedReport.Consumed != 0 || failedReport.Deferred != 0 || failedReport.GradientSteps != 0 || failedReport.PlasticSteps != 0 {
		t.Fatalf("missing-target failure reported %+v, want consumed=0 gradient_steps=0 plastic_steps=0", failedReport)
	}
	afterFailure := individual.Snapshot()
	if afterFailure.Optimizer.Episodes != 0 || afterFailure.Optimizer.Updates != 0 {
		t.Fatalf("missing target unexpectedly trained: optimizer=%+v", afterFailure.Optimizer)
	}

	stageSetTarget(t, learner, action, .4)
	retryReport, err := learner.Update(context.Background(), at(int64(action.Step)))
	if err != nil {
		t.Fatalf("retry Update: %v", err)
	}
	if retryReport.Consumed != 1 || retryReport.Deferred != 0 || retryReport.GradientSteps != 0 || retryReport.PlasticSteps != 1 {
		t.Fatalf("retry reported %+v, want consumed=1 gradient_steps=0 plastic_steps=1", retryReport)
	}
	afterRetry := individual.Snapshot()
	if !reflect.DeepEqual(afterRetry.Optimizer, afterFailure.Optimizer) || !reflect.DeepEqual(afterRetry.Parameters, afterFailure.Parameters) {
		t.Fatalf("adding a target after failure changed the gradient state: before=%+v after=%+v", afterFailure.Optimizer, afterRetry.Optimizer)
	}
}

// TestOnlineMixedRetryStageIgnoresTargetReplacement verifies that a target
// used by a completed gradient phase is not re-read when only plasticity is
// being retried.
func TestOnlineMixedRetryStageIgnoresTargetReplacement(t *testing.T) {
	for _, tc := range []struct {
		name          string
		every         uint64
		firstGradient int
	}{
		{name: "completed_gradient", every: 1, firstGradient: 1},
		{name: "completed_skip", every: 2, firstGradient: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			individual, learner := stageLearnerWithGate(t, tc.every, 1, stageSequenceGate(1))
			action := act(t, learner, onlineInput())
			stageSetTarget(t, learner, action, .4)
			stageReceive(t, learner, action, "teacher", int64(action.Step), 1)

			failedReport, err := learner.Update(context.Background(), at(int64(action.Step)))
			if err == nil || !strings.Contains(err.Error(), "stage gate failure") {
				t.Fatalf("first Update report=%+v err=%v, want gate failure", failedReport, err)
			}
			if failedReport.Consumed != 0 || failedReport.Deferred != 0 || failedReport.GradientSteps != tc.firstGradient || failedReport.PlasticSteps != 0 {
				t.Fatalf("first target failure reported %+v, want consumed=0 gradient_steps=%d plastic_steps=0", failedReport, tc.firstGradient)
			}
			afterFailure := individual.Snapshot()

			stageSetTarget(t, learner, action, .9)
			retryReport, err := learner.Update(context.Background(), at(int64(action.Step)))
			if err != nil {
				t.Fatalf("retry Update: %v", err)
			}
			if retryReport.Consumed != 1 || retryReport.Deferred != 0 || retryReport.GradientSteps != 0 || retryReport.PlasticSteps != 1 {
				t.Fatalf("retry target replacement reported %+v, want consumed=1 gradient_steps=0 plastic_steps=1", retryReport)
			}
			afterRetry := individual.Snapshot()
			if !reflect.DeepEqual(afterRetry.Optimizer, afterFailure.Optimizer) || !reflect.DeepEqual(afterRetry.Parameters, afterFailure.Parameters) {
				t.Fatal("retry after target replacement changed the completed gradient or skip result")
			}
		})
	}
}

// TestOnlineMixedRetryStageKeepsPendingStagesByActionAndTime uses feedback
// queued in late-then-early order. A late feedback fails first, an earlier one
// fails while now moves backwards, then both are retried one at a time. Their
// gradient decisions must remain attached to their own action/source keys.
func TestOnlineMixedRetryStageKeepsPendingStagesByActionAndTime(t *testing.T) {
	individual, learner := stageLearnerWithGate(t, 1, 1, stageScoreGate(map[float64]int{1: 1, 2: 2}))

	late := act(t, learner, onlineInput())
	stageSetTarget(t, learner, late, .4)
	stageReceive(t, learner, late, "late", int64(late.Step)+6, 1)

	early := act(t, learner, onlineInput())
	stageSetTarget(t, learner, early, .4)
	stageReceive(t, learner, early, "early", int64(early.Step), 2)

	lateFailure, err := learner.Update(context.Background(), at(int64(late.Step)+6))
	if err == nil || !strings.Contains(err.Error(), "stage score gate failure") {
		t.Fatalf("late failure report=%+v err=%v, want score gate failure", lateFailure, err)
	}
	if lateFailure.Consumed != 0 || lateFailure.Deferred != 0 || lateFailure.GradientSteps != 1 || lateFailure.PlasticSteps != 0 {
		t.Fatalf("late failure reported %+v, want consumed=0 deferred=0 gradient_steps=1 plastic_steps=0", lateFailure)
	}

	earlyFailure, err := learner.Update(context.Background(), at(int64(early.Step)))
	if err == nil || !strings.Contains(err.Error(), "stage score gate failure") {
		t.Fatalf("early failure report=%+v err=%v, want score gate failure", earlyFailure, err)
	}
	if earlyFailure.Consumed != 0 || earlyFailure.Deferred != 1 || earlyFailure.GradientSteps != 1 || earlyFailure.PlasticSteps != 0 {
		t.Fatalf("early failure reported %+v, want consumed=0 deferred=1 gradient_steps=1 plastic_steps=0", earlyFailure)
	}
	afterFailures := individual.Snapshot()
	if afterFailures.Optimizer.Episodes != 2 || afterFailures.Optimizer.Updates != 2 {
		t.Fatalf("two independent gradient phases produced optimizer=%+v, want episodes=2 updates=2", afterFailures.Optimizer)
	}

	lateRetry, err := learner.Update(context.Background(), at(int64(late.Step)+6))
	if err == nil || !strings.Contains(err.Error(), "stage score gate failure") {
		t.Fatalf("late retry report=%+v err=%v, want the isolated early retry failure", lateRetry, err)
	}
	if lateRetry.Consumed != 1 || lateRetry.Deferred != 0 || lateRetry.GradientSteps != 0 || lateRetry.PlasticSteps != 1 {
		t.Fatalf("late retry reported %+v, want consumed=1 deferred=0 gradient_steps=0 plastic_steps=1", lateRetry)
	}
	if afterRetry := individual.Snapshot(); !reflect.DeepEqual(afterRetry.Optimizer, afterFailures.Optimizer) {
		t.Fatalf("late retry repeated its gradient: before=%+v after=%+v", afterFailures.Optimizer, afterRetry.Optimizer)
	}

	earlyRetry, err := learner.Update(context.Background(), at(int64(late.Step)+6))
	if err != nil {
		t.Fatalf("early retry: %v", err)
	}
	if earlyRetry.Consumed != 1 || earlyRetry.Deferred != 0 || earlyRetry.GradientSteps != 0 || earlyRetry.PlasticSteps != 1 {
		t.Fatalf("early retry reported %+v, want consumed=1 deferred=0 gradient_steps=0 plastic_steps=1", earlyRetry)
	}
	if afterRetry := individual.Snapshot(); !reflect.DeepEqual(afterRetry.Optimizer, afterFailures.Optimizer) {
		t.Fatalf("early retry repeated its gradient: before=%+v after=%+v", afterFailures.Optimizer, afterRetry.Optimizer)
	}
	if final := update(t, learner, int64(late.Step)+6); final.Consumed != 0 || final.Deferred != 0 || final.GradientSteps != 0 || final.PlasticSteps != 0 {
		t.Fatalf("completed pending stages replayed: %+v", final)
	}
}

// TestOnlineMixedRetryStageSurvivesRepeatedFailureAndCancellation checks two
// public failure schedules: two gate errors followed by success, and a gate
// that cancels the Update context immediately before AdvanceGated. Both must
// preserve the completed gradient decision for the later retry.
func TestOnlineMixedRetryStageSurvivesRepeatedFailureAndCancellation(t *testing.T) {
	t.Run("two_gate_failures_then_success", func(t *testing.T) {
		individual, learner := stageLearnerWithGate(t, 1, 1, stageSequenceGate(2))
		action := act(t, learner, onlineInput())
		stageSetTarget(t, learner, action, .4)
		stageReceive(t, learner, action, "teacher", int64(action.Step), 1)

		first, err := learner.Update(context.Background(), at(int64(action.Step)))
		if err == nil || !strings.Contains(err.Error(), "stage gate failure") || first.Consumed != 0 || first.Deferred != 0 || first.GradientSteps != 1 || first.PlasticSteps != 0 {
			t.Fatalf("first repeated-failure Update report=%+v err=%v, want consumed=0 gradient_steps=1 plastic_steps=0 plus gate error", first, err)
		}
		afterFirst := individual.Snapshot()
		second, err := learner.Update(context.Background(), at(int64(action.Step)))
		if err == nil || !strings.Contains(err.Error(), "stage gate failure") || second.Consumed != 0 || second.Deferred != 0 || second.GradientSteps != 0 || second.PlasticSteps != 0 {
			t.Fatalf("second repeated-failure Update report=%+v err=%v, want consumed=0 gradient_steps=0 plastic_steps=0 plus gate error", second, err)
		}
		if afterSecond := individual.Snapshot(); !reflect.DeepEqual(afterSecond.Optimizer, afterFirst.Optimizer) {
			t.Fatalf("second failure repeated gradient state: before=%+v after=%+v", afterFirst.Optimizer, afterSecond.Optimizer)
		}

		third, err := learner.Update(context.Background(), at(int64(action.Step)))
		if err != nil || third.Consumed != 1 || third.Deferred != 0 || third.GradientSteps != 0 || third.PlasticSteps != 1 {
			t.Fatalf("recovery Update report=%+v err=%v, want consumed=1 gradient_steps=0 plastic_steps=1", third, err)
		}
		if afterThird := individual.Snapshot(); !reflect.DeepEqual(afterThird.Optimizer, afterFirst.Optimizer) {
			t.Fatalf("successful plastic retry changed gradient state: before=%+v after=%+v", afterFirst.Optimizer, afterThird.Optimizer)
		}
	})

	t.Run("context_cancellation_then_retry", func(t *testing.T) {
		updateContext, cancel := context.WithCancel(context.Background())
		positiveCalls := 0
		gate := learning.RewardGateFunc(func(score float64) ([]float64, error) {
			if score <= 0 {
				return []float64{0}, nil
			}
			positiveCalls++
			if positiveCalls == 1 {
				cancel()
			}
			return []float64{1}, nil
		})
		individual, learner := stageLearnerWithGate(t, 1, 1, gate)
		action := act(t, learner, onlineInput())
		stageSetTarget(t, learner, action, .4)
		stageReceive(t, learner, action, "teacher", int64(action.Step), 1)

		failed, err := learner.Update(updateContext, at(int64(action.Step)))
		if !errors.Is(err, context.Canceled) || failed.Consumed != 0 || failed.Deferred != 0 || failed.GradientSteps != 1 || failed.PlasticSteps != 0 {
			t.Fatalf("canceled Update report=%+v err=%v, want context cancellation after gradient", failed, err)
		}
		afterFailure := individual.Snapshot()
		retry, err := learner.Update(context.Background(), at(int64(action.Step)))
		if err != nil || retry.Consumed != 1 || retry.Deferred != 0 || retry.GradientSteps != 0 || retry.PlasticSteps != 1 {
			t.Fatalf("cancellation retry report=%+v err=%v, want consumed=1 gradient_steps=0 plastic_steps=1", retry, err)
		}
		if afterRetry := individual.Snapshot(); !reflect.DeepEqual(afterRetry.Optimizer, afterFailure.Optimizer) {
			t.Fatalf("cancellation retry repeated gradient state: before=%+v after=%+v", afterFailure.Optimizer, afterRetry.Optimizer)
		}
	})
}
