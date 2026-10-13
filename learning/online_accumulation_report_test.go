package learning_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func onlineAccumulationIndividual(t *testing.T, accumulate int, chemical bool) *learning.Individual {
	t.Helper()
	options := learning.DefaultOptions()
	options.AccumulateSteps = accumulate
	if !chemical {
		n, parameters := network(t)
		individual, err := learning.NewIndividual(
			n.Config(), parameters, options, make([]float64, n.Config().Dynamics.Nodes),
		)
		if err != nil {
			t.Fatalf("NewIndividual: %v", err)
		}
		return individual
	}

	episode := chemicalEpisodePulse(0, nil, nil, nil)
	config := chemicalEpisodeScalarConfig(episode, false)
	chemistry := episode.Part.Config
	config.Chemistry = nil
	individual, err := learning.NewIndividual(
		config, chemicalEpisodeScalarParameters(), options, make([]float64, config.Dynamics.Nodes),
	)
	if err != nil {
		t.Fatalf("NewIndividual chemical control: %v", err)
	}
	if err := individual.EnableChemistry(chemistry); err != nil {
		t.Fatalf("EnableChemistry: %v", err)
	}
	return individual
}

// TestOnlineGradientStepsReportsOnlyAppliedAccumulationUpdates keeps the
// online report equal to the optimizer update that the public Act -> SetTargets
// -> Receive -> Update loop actually applied. A deferred call is deliberately
// made before each consuming call so Deferred is checked independently from
// Consumed, while a score of one makes the chemistry control use its declared
// target without exercising the weighted-target boundary from ticket 81.
func TestOnlineGradientStepsReportsOnlyAppliedAccumulationUpdates(t *testing.T) {
	for _, control := range []struct {
		name     string
		chemical bool
	}{
		{name: "ordinary", chemical: false},
		{name: "chemical", chemical: true},
	} {
		for _, accumulate := range []int{1, 2, 3} {
			t.Run(fmt.Sprintf("%s/accumulate_%d", control.name, accumulate), func(t *testing.T) {
				individual := onlineAccumulationIndividual(t, accumulate, control.chemical)
				learner := newLearner(t, individual, onlinePolicy(), nil)

				for step := 0; step < accumulate; step++ {
					action := act(t, learner, onlineInput())
					if err := learner.SetTargets(map[string][]float64{action.ActionID: {.4}}); err != nil {
						t.Fatalf("SetTargets: %v", err)
					}
					if err := learner.Receive(context.Background(), feedback(
						t, action.ActionID, fmt.Sprintf("teacher-%d", step),
						int64(action.Step), int64(action.Step+1), 1,
					)); err != nil {
						t.Fatalf("Receive: %v", err)
					}

					beforeDeferred := individual.Snapshot()
					deferred := update(t, learner, int64(action.Step))
					if deferred.Consumed != 0 || deferred.Deferred != 1 || deferred.GradientSteps != 0 || deferred.PlasticSteps != 0 {
						t.Fatalf("feedback before AvailableAt reported %+v", deferred)
					}
					if afterDeferred := individual.Snapshot(); !reflect.DeepEqual(afterDeferred, beforeDeferred) {
						t.Fatal("deferred feedback changed the individual")
					}

					before := individual.Snapshot()
					arrived := update(t, learner, int64(action.Step+1))
					if arrived.Consumed != 1 || arrived.Deferred != 0 || arrived.PlasticSteps != 0 {
						t.Fatalf("arrived feedback reported %+v", arrived)
					}
					after := individual.Snapshot()
					if step+1 < accumulate {
						if arrived.GradientSteps != 0 {
							t.Fatalf("partial accumulation reported GradientSteps=%d, want 0", arrived.GradientSteps)
						}
						if after.Optimizer.Updates != before.Optimizer.Updates {
							t.Fatalf("partial accumulation changed Updates from %d to %d", before.Optimizer.Updates, after.Optimizer.Updates)
						}
						if after.Optimizer.Accumulator == nil || after.Optimizer.Accumulator.Count != step+1 {
							t.Fatalf("partial accumulation snapshot = %+v, want count %d", after.Optimizer.Accumulator, step+1)
						}
					} else {
						if arrived.GradientSteps != 1 {
							t.Fatalf("completed accumulation reported GradientSteps=%d, want 1", arrived.GradientSteps)
						}
						if after.Optimizer.Updates != before.Optimizer.Updates+1 {
							t.Fatalf("completed accumulation changed Updates from %d to %d", before.Optimizer.Updates, after.Optimizer.Updates)
						}
						if after.Optimizer.Accumulator != nil {
							t.Fatalf("completed accumulation kept accumulator %+v", after.Optimizer.Accumulator)
						}
					}

					// A fresh online learner has no action queue to restore, but the
					// individual snapshot must carry the partial optimizer window.
					// Continue the next feedback through the same public sequence
					// after restoring it, so a restore cannot reset the count or make
					// a partial step look like an applied update.
					if accumulate > 1 && step+1 == accumulate-1 {
						partial := individual.Snapshot()
						if partial.Optimizer.Accumulator == nil || partial.Optimizer.Accumulator.Count != accumulate-1 {
							t.Fatalf("partial snapshot before restore = %+v, want count %d", partial.Optimizer.Accumulator, accumulate-1)
						}
						restored, err := learning.RestoreIndividual(partial)
						if err != nil {
							t.Fatalf("RestoreIndividual partial window: %v", err)
						}
						if got := restored.Snapshot(); !reflect.DeepEqual(got.Optimizer, partial.Optimizer) {
							t.Fatalf("restored optimizer = %+v, want %+v", got.Optimizer, partial.Optimizer)
						}
						individual = restored
						learner = newLearner(t, individual, onlinePolicy(), nil)
					}
				}
			})
		}
	}
}
