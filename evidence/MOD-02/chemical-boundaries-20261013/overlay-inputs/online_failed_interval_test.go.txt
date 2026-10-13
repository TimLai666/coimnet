package learning_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// TestOnlineFailedGradientIntervalIsRetriedWithoutConsumption uses a legal
// restored public snapshot whose episode clock is exhausted. UpdateEvery=2
// first consumes an interval skip, then reaches TrainEpisode and fails. Every
// retry must preserve that same feedback and the same individual snapshot; a
// retry must never become a skip merely because the private interval counter
// advanced before the failed call.
func TestOnlineFailedGradientIntervalIsRetriedWithoutConsumption(t *testing.T) {
	individual := episodeClockIndividual(t, false, ^uint64(0))
	if got := individual.Snapshot().Optimizer.Episodes; got != ^uint64(0) {
		t.Fatalf("restored episode clock = %d, want MaxUint64", got)
	}
	policy := onlinePolicy()
	policy.UpdateEvery = 2
	learner := newLearner(t, individual, policy, nil)

	first := act(t, learner, onlineInput())
	if err := learner.SetTargets(map[string][]float64{first.ActionID: {.4}}); err != nil {
		t.Fatalf("SetTargets first: %v", err)
	}
	if err := learner.Receive(context.Background(), feedback(t, first.ActionID, "teacher-first", int64(first.Step), int64(first.Step), 1)); err != nil {
		t.Fatalf("Receive first: %v", err)
	}
	beforeSkip := individual.Snapshot()
	firstReport, err := learner.Update(context.Background(), at(int64(first.Step)))
	if err != nil {
		t.Fatalf("the first UpdateEvery=2 interval skip failed: %v", err)
	}
	if firstReport.Consumed != 1 || firstReport.Deferred != 0 || firstReport.GradientSteps != 0 || firstReport.PlasticSteps != 0 {
		t.Fatalf("the first interval skip reported %+v", firstReport)
	}
	if afterSkip := individual.Snapshot(); !reflect.DeepEqual(afterSkip, beforeSkip) {
		t.Fatal("the interval skip changed the individual")
	}

	second := act(t, learner, onlineInput())
	if err := learner.SetTargets(map[string][]float64{second.ActionID: {.4}}); err != nil {
		t.Fatalf("SetTargets second: %v", err)
	}
	if err := learner.Receive(context.Background(), feedback(t, second.ActionID, "teacher-second", int64(second.Step), int64(second.Step), 1)); err != nil {
		t.Fatalf("Receive second: %v", err)
	}
	beforeFailure := individual.Snapshot()
	for attempt := 1; attempt <= 3; attempt++ {
		report, err := learner.Update(context.Background(), at(int64(second.Step)))
		if err == nil || !strings.Contains(err.Error(), "episode count overflow") {
			t.Fatalf("failed feedback attempt %d returned report=%+v err=%v, want episode count overflow", attempt, report, err)
		}
		if report.Consumed != 0 || report.Deferred != 0 || report.GradientSteps != 0 || report.PlasticSteps != 0 {
			t.Fatalf("failed feedback attempt %d reported %+v, want no consumption or updates", attempt, report)
		}
		if afterFailure := individual.Snapshot(); !reflect.DeepEqual(afterFailure, beforeFailure) {
			t.Fatalf("failed feedback attempt %d changed the individual", attempt)
		}
	}
}

// TestOnlineUpdateEveryNormalSkipAndAppliedControl verifies the public
// interval behavior with a usable episode clock: UpdateEvery=1 applies the
// first eligible feedback, while UpdateEvery=2 consumes the first as a skip
// and applies the second. These controls never inspect OnlineLearner.eligible.
func TestOnlineUpdateEveryNormalSkipAndAppliedControl(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		every    uint64
		gradient []int
	}{
		{name: "every_1_applies", every: 1, gradient: []int{1}},
		{name: "every_2_skips_then_applies", every: 2, gradient: []int{0, 1}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			individual := onlineIndividual(t)
			policy := onlinePolicy()
			policy.UpdateEvery = testCase.every
			learner := newLearner(t, individual, policy, nil)
			for step, wantGradient := range testCase.gradient {
				action := act(t, learner, onlineInput())
				if err := learner.SetTargets(map[string][]float64{action.ActionID: {.4}}); err != nil {
					t.Fatalf("SetTargets %d: %v", step, err)
				}
				if err := learner.Receive(context.Background(), feedback(t, action.ActionID, "teacher", int64(action.Step), int64(action.Step), 1)); err != nil {
					t.Fatalf("Receive %d: %v", step, err)
				}
				before := individual.Snapshot()
				report := update(t, learner, int64(action.Step))
				if report.Consumed != 1 || report.Deferred != 0 || report.GradientSteps != wantGradient || report.PlasticSteps != 0 {
					t.Fatalf("UpdateEvery=%d feedback %d reported %+v", testCase.every, step, report)
				}
				after := individual.Snapshot()
				if wantGradient == 0 {
					if !reflect.DeepEqual(after, before) {
						t.Fatalf("UpdateEvery=%d skip %d changed the individual", testCase.every, step)
					}
				} else if after.Optimizer.Updates != before.Optimizer.Updates+1 {
					t.Fatalf("UpdateEvery=%d applied step %d changed Updates from %d to %d", testCase.every, step, before.Optimizer.Updates, after.Optimizer.Updates)
				}
			}
		})
	}
}
