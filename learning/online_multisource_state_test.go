package learning_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func multiSourceSnapshot(t *testing.T, individual *learning.Individual) string {
	t.Helper()
	b, err := json.Marshal(individual.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestOnlineMixedSameActionIndependentSourceState leaves two sources for one
// action between gradient and plasticity, then retries them independently as
// now moves backwards. The reference composes public training/advance calls in
// the actual successful phase order, rather than assuming phases commute.
func TestOnlineMixedSameActionIndependentSourceState(t *testing.T) {
	for _, every := range []uint64{1, 2} {
		for _, accumulate := range []int{1, 2} {
			t.Run(fmt.Sprintf("every_%d/accumulate_%d", every, accumulate), func(t *testing.T) {
				ctx := context.Background()
				transient := errors.New("independent source transient gate")
				calls := 0
				gate := learning.RewardGateFunc(func(score float64) ([]float64, error) {
					if score == 0 {
						return []float64{0}, nil
					}
					calls++
					if calls == 1 || calls == 2 || calls == 4 {
						return nil, transient
					}
					return []float64{1}, nil
				})
				individual, learner := stageLearnerWithGate(t, every, accumulate, gate)
				reference, referenceLearner := stageLearnerWithGate(t, every, accumulate, stageSequenceGate(0))
				action := act(t, learner, onlineInput())
				act(t, referenceLearner, onlineInput())
				stageSetTarget(t, learner, action, 0)
				initial := individual.Snapshot()
				start := int64(action.Step)
				late, future := start+6, start+12
				for _, fb := range []struct {
					source string
					at     int64
				}{{"late", late}, {"early", start}, {"future-a", future}, {"future-b", future}} {
					stageReceive(t, learner, action, fb.source, fb.at, 1)
				}
				logBefore := learner.Log()
				if multiSourceSnapshot(t, individual) != multiSourceSnapshot(t, reference) || !reflect.DeepEqual(logBefore, referenceLearner.Log()) {
					t.Fatal("reference did not start from identical model state and answer")
				}
				// For target=0 and score=1, weightedTarget is p+(0-p)=0.
				// This direct public call does not reproduce the learner's queue
				// or marker implementation.
				eligible := uint64(0)
				gradient := func() int {
					eligible++
					if eligible%every != 0 {
						return 0
					}
					result, err := reference.TrainEpisode(ctx, onlineInput(), []float64{0})
					if err != nil {
						t.Fatal(err)
					}
					if result.Applied {
						return 1
					}
					return 0
				}
				plastic := func() {
					if _, _, err := reference.AdvanceGated(ctx, onlineInput(), []float64{1, 1, 1}); err != nil {
						t.Fatal(err)
					}
				}
				check := func(now int64, want learning.UpdateReport, failed bool) {
					t.Helper()
					got, err := learner.Update(ctx, at(now))
					if got != want || (failed && !errors.Is(err, transient)) || (!failed && err != nil) {
						t.Fatalf("Update(%d)=(%+v,%v), want (%+v,failed=%v)", now, got, err, want, failed)
					}
					if multiSourceSnapshot(t, individual) != multiSourceSnapshot(t, reference) {
						t.Fatalf("Update(%d) differs from complete independent phase state", now)
					}
					if !reflect.DeepEqual(learner.Log(), logBefore) {
						t.Fatal("feedback processing rewrote an answer already given")
					}
				}
				check(late, learning.UpdateReport{GradientSteps: gradient(), Deferred: 2}, true)
				if err := learner.Receive(ctx, feedback(t, action.ActionID, "early", start, start, 1)); err == nil {
					t.Fatal("duplicate queued source was accepted")
				}
				check(start, learning.UpdateReport{GradientSteps: gradient(), Deferred: 3}, true)
				plastic()
				check(late, learning.UpdateReport{Consumed: 1, PlasticSteps: 1, Deferred: 2}, true)
				plastic()
				check(start, learning.UpdateReport{Consumed: 1, PlasticSteps: 1, Deferred: 2}, false)
				check(start, learning.UpdateReport{Deferred: 2}, false)
				futureGradients := gradient()
				plastic()
				futureGradients += gradient()
				plastic()
				check(future, learning.UpdateReport{Consumed: 2, GradientSteps: futureGradients, PlasticSteps: 2}, false)
				check(future, learning.UpdateReport{}, false)
				if calls != 7 || reflect.DeepEqual(initial.Parameters, individual.Snapshot().Parameters) {
					t.Fatal("positive control did not perform the expected gate calls and parameter update")
				}
				if err := learner.Receive(ctx, feedback(t, action.ActionID, "late", start, late, 1)); err == nil {
					t.Fatal("duplicate consumed source was accepted")
				}
				check(future, learning.UpdateReport{}, false)
			})
		}
	}
}
