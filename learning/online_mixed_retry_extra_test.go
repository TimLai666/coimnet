package learning_test

import (
	"context"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

// extraLearnerWithGate keeps the fixture and public loop from online_test.go,
// while allowing this file to exercise gate failures that are not represented
// by the original transient error helper.
func extraLearnerWithGate(t *testing.T, every uint64, accumulate int, gate learning.RewardGate) (*learning.Individual, *learning.OnlineLearner) {
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

// extraFirstGateFailure returns a valid closed gate at construction time and
// on every retry. The first positive score returns first, then every later
// positive score returns a usable gate. The invalid result therefore reaches
// the public Update path exactly once without a private hook.
func extraFirstGateFailure(first []float64) learning.RewardGate {
	positiveCalls := 0
	return learning.RewardGateFunc(func(score float64) ([]float64, error) {
		if score <= 0 {
			return []float64{0}, nil
		}
		positiveCalls++
		if positiveCalls == 1 {
			return append([]float64(nil), first...), nil
		}
		return []float64{1}, nil
	})
}

func enqueueExtraFeedback(t *testing.T, learner *learning.OnlineLearner, input [][]float64, source string) learning.ActionRecord {
	t.Helper()
	action := act(t, learner, input)
	if err := learner.SetTargets(map[string][]float64{action.ActionID: {.4}}); err != nil {
		t.Fatalf("SetTargets(%s): %v", action.ActionID, err)
	}
	if err := learner.Receive(context.Background(), feedback(t, action.ActionID, source, int64(action.Step), int64(action.Step), 1)); err != nil {
		t.Fatalf("Receive(%s): %v", action.ActionID, err)
	}
	return action
}

// TestOnlineMixedRetryKeepsFirstUpdateEverySkip covers the other interval
// boundary: the first feedback is a legal UpdateEvery=2 skip, but its plastic
// half fails. Retry must not turn that already-completed skip into a gradient
// attempt. The following new feedback is the one that becomes gradient-eligible.
func TestOnlineMixedRetryKeepsFirstUpdateEverySkip(t *testing.T) {
	for _, tc := range []struct {
		name       string
		accumulate int
		wantNext   int
	}{
		{name: "accumulate_1", accumulate: 1, wantNext: 1},
		{name: "accumulate_2", accumulate: 2, wantNext: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			individual, learner := mixedRetryLearner(t, 2, tc.accumulate, 1)
			first := enqueueMixedFeedback(t, learner, "teacher-first-skip")

			failedReport, err := learner.Update(context.Background(), at(int64(first.Step)))
			if err == nil || !strings.Contains(err.Error(), "transient reward gate failure") {
				t.Fatalf("first Update report=%+v err=%v, want the one-shot gate failure", failedReport, err)
			}
			if failedReport.Consumed != 0 || failedReport.Deferred != 0 || failedReport.GradientSteps != 0 || failedReport.PlasticSteps != 0 {
				t.Fatalf("first failed skip reported %+v, want consumed=0 gradient_steps=0 plastic_steps=0", failedReport)
			}
			afterFailure := individual.Snapshot()
			if afterFailure.Optimizer.Episodes != 0 || afterFailure.Optimizer.Updates != 0 || afterFailure.Optimizer.Accumulator != nil {
				t.Fatalf("a legal interval skip changed optimizer=%+v", afterFailure.Optimizer)
			}

			retryReport, err := learner.Update(context.Background(), at(int64(first.Step)))
			if err != nil {
				t.Fatalf("retry Update: %v", err)
			}
			if retryReport.Consumed != 1 || retryReport.Deferred != 0 || retryReport.GradientSteps != 0 || retryReport.PlasticSteps != 1 {
				t.Fatalf("retry reported %+v, want consumed=1 gradient_steps=0 plastic_steps=1", retryReport)
			}
			afterRetry := individual.Snapshot()
			if !reflect.DeepEqual(afterRetry.Optimizer, afterFailure.Optimizer) {
				t.Fatalf("retry advanced a completed interval skip: before=%+v after=%+v", afterFailure.Optimizer, afterRetry.Optimizer)
			}
			if !reflect.DeepEqual(afterRetry.Parameters, afterFailure.Parameters) {
				t.Fatal("retry of a skip changed base parameters")
			}

			next := enqueueMixedFeedback(t, learner, "teacher-next-gradient")
			nextReport, err := learner.Update(context.Background(), at(int64(next.Step)))
			if err != nil {
				t.Fatalf("next Update: %v", err)
			}
			if nextReport.Consumed != 1 || nextReport.Deferred != 0 || nextReport.GradientSteps != tc.wantNext || nextReport.PlasticSteps != 1 {
				t.Fatalf("next Update reported %+v, want consumed=1 gradient_steps=%d plastic_steps=1", nextReport, tc.wantNext)
			}
		})
	}
}

// TestOnlineMixedRetryRejectsInvalidGateWithoutRepeatingGradient covers the
// gate boundary after a completed gradient or partial accumulation. Empty
// release is rejected by OnlineLearner; NaN and Inf fail gate validation.
// MaxFloat64 overflows plastic arithmetic. Every case becomes valid on retry.
func TestOnlineMixedRetryRejectsInvalidGateWithoutRepeatingGradient(t *testing.T) {
	for _, gateCase := range []struct {
		name      string
		first     []float64
		errorPart string
	}{
		{name: "nan", first: []float64{math.NaN()}, errorPart: "not finite"},
		{name: "positive_inf", first: []float64{math.Inf(1)}, errorPart: "not finite"},
		{name: "empty_release", first: []float64{}, errorPart: "no release channel"},
		{name: "max_float", first: []float64{math.MaxFloat64}, errorPart: "non-finite"},
	} {
		for _, accumulate := range []int{1, 2} {
			t.Run(gateCase.name+"/accumulate_"+string(rune('0'+accumulate)), func(t *testing.T) {
				individual, learner := extraLearnerWithGate(t, 1, accumulate, extraFirstGateFailure(gateCase.first))
				input := onlineInput()
				if gateCase.name == "max_float" {
					// Saturating inputs make the third eligibility value exceed one,
					// so MaxFloat64*eligibility must overflow inside AdvanceGated
					// before plastic_max clamping can hide the failure.
					input = [][]float64{{100}, {100}, {100}}
				}
				first := enqueueExtraFeedback(t, learner, input, "teacher-invalid-gate")

				failedReport, err := learner.Update(context.Background(), at(int64(first.Step)))
				if err == nil || !strings.Contains(err.Error(), gateCase.errorPart) {
					t.Fatalf("first Update report=%+v err=%v, want gate failure containing %q", failedReport, err, gateCase.errorPart)
				}
				wantGradient := 1
				if accumulate == 2 {
					wantGradient = 0
				}
				if failedReport.Consumed != 0 || failedReport.Deferred != 0 || failedReport.GradientSteps != wantGradient || failedReport.PlasticSteps != 0 {
					t.Fatalf("first invalid-gate Update reported %+v, want consumed=0 gradient_steps=%d plastic_steps=0", failedReport, wantGradient)
				}
				afterFailure := individual.Snapshot()
				if afterFailure.Optimizer.Episodes != 1 {
					t.Fatalf("first gradient phase episodes=%d, want 1", afterFailure.Optimizer.Episodes)
				}
				if accumulate == 1 && afterFailure.Optimizer.Updates != 1 {
					t.Fatalf("AccumulateSteps=1 updates=%d, want 1", afterFailure.Optimizer.Updates)
				}
				if accumulate == 2 && (afterFailure.Optimizer.Updates != 0 || afterFailure.Optimizer.Accumulator == nil || afterFailure.Optimizer.Accumulator.Count != 1) {
					t.Fatalf("AccumulateSteps=2 optimizer=%+v, want one partial accumulation", afterFailure.Optimizer)
				}

				retryReport, err := learner.Update(context.Background(), at(int64(first.Step)))
				if err != nil {
					t.Fatalf("retry Update: %v", err)
				}
				if retryReport.Consumed != 1 || retryReport.Deferred != 0 || retryReport.GradientSteps != 0 || retryReport.PlasticSteps != 1 {
					t.Fatalf("retry invalid-gate Update reported %+v, want consumed=1 gradient_steps=0 plastic_steps=1", retryReport)
				}
				afterRetry := individual.Snapshot()
				if !reflect.DeepEqual(afterRetry.Optimizer, afterFailure.Optimizer) {
					t.Fatalf("retry repeated gradient or accumulation: before=%+v after=%+v", afterFailure.Optimizer, afterRetry.Optimizer)
				}
				if !reflect.DeepEqual(afterRetry.Parameters, afterFailure.Parameters) {
					t.Fatal("retry invalid-gate recovery changed base parameters")
				}
			})
		}
	}
}

// TestOnlineMixedRetryKeepsMultiSourceConsumptionAndDeferredQueue proves that
// one successful source for an action is consumed, the next source may fail
// after its gradient half, and a later action remains deferred. Retry must
// consume exactly the failed source and leave the delayed source for its own
// later Update without repeating the failed source's gradient.
func TestOnlineMixedRetryKeepsMultiSourceConsumptionAndDeferredQueue(t *testing.T) {
	for _, tc := range []struct {
		name       string
		accumulate int
		failedGrad int
		finalGrad  int
	}{
		{name: "accumulate_1", accumulate: 1, failedGrad: 2, finalGrad: 1},
		{name: "accumulate_2", accumulate: 2, failedGrad: 1, finalGrad: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			individual, learner := mixedRetryLearner(t, 1, tc.accumulate, 2)
			first := act(t, learner, onlineInput())
			if err := learner.SetTargets(map[string][]float64{first.ActionID: {.4}}); err != nil {
				t.Fatalf("SetTargets(first): %v", err)
			}
			if err := learner.Receive(context.Background(), feedback(t, first.ActionID, "teacher", int64(first.Step), int64(first.Step), 1)); err != nil {
				t.Fatalf("Receive(first teacher): %v", err)
			}
			if err := learner.Receive(context.Background(), feedback(t, first.ActionID, "critic", int64(first.Step), int64(first.Step), 1)); err != nil {
				t.Fatalf("Receive(first critic): %v", err)
			}

			second := act(t, learner, onlineInput())
			if err := learner.SetTargets(map[string][]float64{second.ActionID: {.4}}); err != nil {
				t.Fatalf("SetTargets(second): %v", err)
			}
			delayedAt := int64(second.Step) + 3
			if err := learner.Receive(context.Background(), feedback(t, second.ActionID, "delayed", int64(second.Step), delayedAt, 1)); err != nil {
				t.Fatalf("Receive(delayed): %v", err)
			}

			failedReport, err := learner.Update(context.Background(), at(int64(second.Step)))
			if err == nil || !strings.Contains(err.Error(), "transient reward gate failure") {
				t.Fatalf("first batch report=%+v err=%v, want second source gate failure", failedReport, err)
			}
			if failedReport.Consumed != 1 || failedReport.Deferred != 1 || failedReport.GradientSteps != tc.failedGrad || failedReport.PlasticSteps != 1 {
				t.Fatalf("first batch reported %+v, want consumed=1 deferred=1 gradient_steps=%d plastic_steps=1", failedReport, tc.failedGrad)
			}
			afterFailure := individual.Snapshot()

			retryReport, err := learner.Update(context.Background(), at(int64(second.Step)))
			if err != nil {
				t.Fatalf("retry second source: %v", err)
			}
			if retryReport.Consumed != 1 || retryReport.Deferred != 1 || retryReport.GradientSteps != 0 || retryReport.PlasticSteps != 1 {
				t.Fatalf("retry second source reported %+v, want consumed=1 deferred=1 gradient_steps=0 plastic_steps=1", retryReport)
			}
			if afterRetry := individual.Snapshot(); !reflect.DeepEqual(afterRetry.Optimizer, afterFailure.Optimizer) {
				t.Fatalf("retry repeated the failed source gradient: before=%+v after=%+v", afterFailure.Optimizer, afterRetry.Optimizer)
			}

			finalReport, err := learner.Update(context.Background(), at(delayedAt))
			if err != nil {
				t.Fatalf("delayed source Update: %v", err)
			}
			if finalReport.Consumed != 1 || finalReport.Deferred != 0 || finalReport.GradientSteps != tc.finalGrad || finalReport.PlasticSteps != 1 {
				t.Fatalf("delayed source reported %+v, want consumed=1 deferred=0 gradient_steps=%d plastic_steps=1", finalReport, tc.finalGrad)
			}
		})
	}
}
