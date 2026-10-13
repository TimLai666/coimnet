package learning_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

// transientRetryGate is deliberately public-contract compatible: construction
// calls Applied(0), while the selected positive feedback call fails once and a
// later retry succeeds. The existing online test helpers provide the fixture,
// Act, SetTargets, Receive and Update wiring when this draft is copied into the
// learning package's external tests.
type transientRetryGate struct {
	positiveCalls  int
	failOnPositive int
}

func (g *transientRetryGate) Applied(score float64) ([]float64, error) {
	if score <= 0 {
		return []float64{0}, nil
	}
	g.positiveCalls++
	if g.failOnPositive > 0 && g.positiveCalls == g.failOnPositive {
		return nil, errors.New("transient reward gate failure")
	}
	return []float64{1}, nil
}

func mixedRetryLearner(t *testing.T, every uint64, accumulate, failOnPositive int) (*learning.Individual, *learning.OnlineLearner) {
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
	learner := newLearner(t, individual, policy, &transientRetryGate{failOnPositive: failOnPositive})
	return individual, learner
}

func enqueueMixedFeedback(t *testing.T, learner *learning.OnlineLearner, source string) learning.ActionRecord {
	t.Helper()
	action := act(t, learner, onlineInput())
	if err := learner.SetTargets(map[string][]float64{action.ActionID: {.4}}); err != nil {
		t.Fatalf("SetTargets(%s): %v", action.ActionID, err)
	}
	if err := learner.Receive(context.Background(), feedback(t, action.ActionID, source, int64(action.Step), int64(action.Step), 1)); err != nil {
		t.Fatalf("Receive(%s): %v", action.ActionID, err)
	}
	return action
}

// TestOnlineMixedRetryDoesNotRepeatGradientAfterGateFailure checks the
// completed-gradient/failed-plastic half boundary for UpdateEvery=1. The
// failed Update must leave the gradient result standing, but retrying the same
// queued feedback must run only the still-failed plastic half. For
// AccumulateSteps=2, the first successful gradient phase is the partial window
// itself and must not add the same feedback a second time.
func TestOnlineMixedRetryDoesNotRepeatGradientAfterGateFailure(t *testing.T) {
	for _, tc := range []struct {
		name       string
		accumulate int
		updates    uint64
	}{
		{name: "accumulate_1", accumulate: 1, updates: 1},
		{name: "accumulate_2", accumulate: 2, updates: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			individual, learner := mixedRetryLearner(t, 1, tc.accumulate, 1)
			first := enqueueMixedFeedback(t, learner, "teacher-first")

			firstReport, err := learner.Update(context.Background(), at(int64(first.Step)))
			if err == nil || !strings.Contains(err.Error(), "transient reward gate failure") {
				t.Fatalf("first Update report=%+v err=%v, want the one-shot gate failure", firstReport, err)
			}
			wantGradient := 1
			if tc.accumulate == 2 {
				wantGradient = 0
			}
			if firstReport.Consumed != 0 || firstReport.Deferred != 0 || firstReport.GradientSteps != wantGradient || firstReport.PlasticSteps != 0 {
				t.Fatalf("first failed Update reported %+v, want consumed=0 gradient_steps=%d plastic_steps=0", firstReport, wantGradient)
			}
			afterFailure := individual.Snapshot()
			if afterFailure.Optimizer.Episodes != 1 || afterFailure.Optimizer.Updates != tc.updates {
				t.Fatalf("first gradient phase snapshot optimizer=%+v, want episodes=1 updates=%d", afterFailure.Optimizer, tc.updates)
			}
			if tc.accumulate == 1 {
				if afterFailure.Optimizer.Accumulator != nil {
					t.Fatalf("AccumulateSteps=1 opened accumulator %+v", afterFailure.Optimizer.Accumulator)
				}
			} else if afterFailure.Optimizer.Accumulator == nil || afterFailure.Optimizer.Accumulator.Count != 1 {
				t.Fatalf("AccumulateSteps=2 first gradient snapshot accumulator=%+v, want count=1", afterFailure.Optimizer.Accumulator)
			}

			retryReport, err := learner.Update(context.Background(), at(int64(first.Step)))
			if err != nil {
				t.Fatalf("retry Update: %v", err)
			}
			if retryReport.Consumed != 1 || retryReport.Deferred != 0 || retryReport.GradientSteps != 0 || retryReport.PlasticSteps != 1 {
				t.Fatalf("retry Update reported %+v, want consumed=1 gradient_steps=0 plastic_steps=1", retryReport)
			}
			afterRetry := individual.Snapshot()
			if !reflect.DeepEqual(afterRetry.Optimizer, afterFailure.Optimizer) {
				t.Fatalf("retry repeated the gradient/accumulator: before=%+v after=%+v", afterFailure.Optimizer, afterRetry.Optimizer)
			}
			if !reflect.DeepEqual(afterRetry.Parameters, afterFailure.Parameters) {
				t.Fatal("retry plastic half changed base parameters")
			}
		})
	}
}

// TestOnlineMixedRetryDoesNotDriftUpdateEvery checks UpdateEvery=2 after the
// second feedback's gradient phase has succeeded and its plastic gate fails.
// The retry may consume only the remaining plastic phase; it must not count
// that same feedback a second time. A third feedback therefore remains an
// interval skip. The current implementation advances eligible again on retry,
// making this deterministic control fail at the third Update.
func TestOnlineMixedRetryDoesNotDriftUpdateEvery(t *testing.T) {
	for _, accumulate := range []int{1, 2} {
		t.Run("accumulate_"+string(rune('0'+accumulate)), func(t *testing.T) {
			individual, learner := mixedRetryLearner(t, 2, accumulate, 2)

			first := enqueueMixedFeedback(t, learner, "teacher-first")
			firstReport, err := learner.Update(context.Background(), at(int64(first.Step)))
			if err != nil || firstReport.Consumed != 1 || firstReport.GradientSteps != 0 || firstReport.PlasticSteps != 1 {
				t.Fatalf("first Update report=%+v err=%v, want interval skip plus plastic success", firstReport, err)
			}

			second := enqueueMixedFeedback(t, learner, "teacher-second")
			failedReport, err := learner.Update(context.Background(), at(int64(second.Step)))
			if err == nil || !strings.Contains(err.Error(), "transient reward gate failure") {
				t.Fatalf("second Update report=%+v err=%v, want the one-shot gate failure", failedReport, err)
			}
			wantGradient := 1
			if accumulate == 2 {
				wantGradient = 0
			}
			if failedReport.Consumed != 0 || failedReport.Deferred != 0 || failedReport.GradientSteps != wantGradient || failedReport.PlasticSteps != 0 {
				t.Fatalf("second failed Update reported %+v, want consumed=0 gradient_steps=%d plastic_steps=0", failedReport, wantGradient)
			}
			afterFailure := individual.Snapshot()
			if afterFailure.Optimizer.Episodes != 1 {
				t.Fatalf("second gradient phase episodes=%d, want 1", afterFailure.Optimizer.Episodes)
			}

			retryReport, err := learner.Update(context.Background(), at(int64(second.Step)))
			if err != nil || retryReport.Consumed != 1 || retryReport.GradientSteps != 0 || retryReport.PlasticSteps != 1 {
				t.Fatalf("retry Update report=%+v err=%v, want plastic-only completion", retryReport, err)
			}
			if afterRetry := individual.Snapshot(); !reflect.DeepEqual(afterRetry.Optimizer, afterFailure.Optimizer) {
				t.Fatalf("retry changed optimizer state while completing plastic half: before=%+v after=%+v", afterFailure.Optimizer, afterRetry.Optimizer)
			}

			third := enqueueMixedFeedback(t, learner, "teacher-third")
			thirdReport, err := learner.Update(context.Background(), at(int64(third.Step)))
			if err != nil {
				t.Fatalf("third Update: %v", err)
			}
			if thirdReport.Consumed != 1 || thirdReport.Deferred != 0 || thirdReport.GradientSteps != 0 || thirdReport.PlasticSteps != 1 {
				t.Fatalf("third Update reported %+v, want the post-retry interval skip", thirdReport)
			}
		})
	}
}

// TestOnlineMixedRetryControls covers the normal mixed path for all requested
// interval/window combinations. It is a control: the gate never fails, every
// feedback is consumed exactly once, and gradient reports occur only when the
// declared interval and accumulation window both close.
func TestOnlineMixedRetryControls(t *testing.T) {
	for _, tc := range []struct {
		name       string
		every      uint64
		accumulate int
		gradient   []int
	}{
		{name: "every_1_accumulate_1", every: 1, accumulate: 1, gradient: []int{1, 1}},
		{name: "every_1_accumulate_2", every: 1, accumulate: 2, gradient: []int{0, 1}},
		{name: "every_2_accumulate_1", every: 2, accumulate: 1, gradient: []int{0, 1, 0, 1}},
		{name: "every_2_accumulate_2", every: 2, accumulate: 2, gradient: []int{0, 0, 0, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, learner := mixedRetryLearner(t, tc.every, tc.accumulate, 0)
			for index, wantGradient := range tc.gradient {
				action := enqueueMixedFeedback(t, learner, "control-"+string(rune('0'+index)))
				report, err := learner.Update(context.Background(), at(int64(action.Step)))
				if err != nil {
					t.Fatalf("Update %d: %v", index, err)
				}
				if report.Consumed != 1 || report.Deferred != 0 || report.GradientSteps != wantGradient || report.PlasticSteps != 1 {
					t.Fatalf("Update %d reported %+v, want consumed=1 gradient_steps=%d plastic_steps=1", index, report, wantGradient)
				}
			}
		})
	}
}
