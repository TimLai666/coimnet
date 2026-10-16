package learning_test

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"sync"
	"testing"

	"github.com/TimLai666/coimnet/experiment"
	"github.com/TimLai666/coimnet/learning"
)

// The oracle uses the existing public freeze contract. It holds all fast
// state, while the same neural and chemical mechanisms still run.
func TestAdaptiveScoringHoldsFastState(t *testing.T) {
	ctx := context.Background()
	for _, core := range []string{"continuous", "lif", "mixed"} {
		for _, receptor := range []bool{false, true} {
			for _, reset := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/receptor=%t/reset=%t", core, receptor, reset), func(t *testing.T) {
					a := onlineReceptorPolicyIndividual(t, core, true, receptor, receptor, .25)
					before := a.Snapshot()
					oracle, err := learning.RestoreIndividual(before)
					if err != nil {
						t.Fatal(err)
					}
					if reset {
						oracle.DisablePlasticity()
						if err := oracle.EnablePlasticity(before.Plastic.Config); err != nil {
							t.Fatal(err)
						}
					}
					if err := oracle.FreezePlasticity(true); err != nil {
						t.Fatal(err)
					}
					want, err := oracle.Advance(ctx, onlineReceptorPolicyInput())
					if err != nil {
						t.Fatal(err)
					}
					e := experiment.AdaptiveEvaluation{
						Mode: experiment.EvaluationModeFixed,
						Scoring: []experiment.EvaluationItem{{
							ID: "held", Input: onlineReceptorPolicyInput(), Target: []float64{0},
						}},
						Reset: experiment.ResetPolicy{PlasticAtItemStart: reset},
					}
					r, err := experiment.RunAdaptiveEvaluation(ctx, a, e)
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(a.Snapshot(), oracle.Snapshot()) {
						t.Errorf("scoring changed fast state: got %+v, want %+v", a.Snapshot().Plastic.State, oracle.Snapshot().Plastic.State)
					}
					if !reflect.DeepEqual(r.Scores[0].Output, want) {
						t.Errorf("scoring output = %v, frozen oracle = %v", r.Scores[0].Output, want)
					}
					// Evaluation must not leave a runtime freeze behind. A later
					// native advance must behave like the unfrozen oracle.
					if err := oracle.FreezePlasticity(false); err != nil {
						t.Fatal(err)
					}
					gotNext, err := a.Advance(ctx, onlineReceptorPolicyInput())
					if err != nil {
						t.Fatal(err)
					}
					wantNext, err := oracle.Advance(ctx, onlineReceptorPolicyInput())
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(gotNext, wantNext) || !reflect.DeepEqual(a.Snapshot(), oracle.Snapshot()) {
						t.Error("scoring changed the later native trajectory or runtime freeze")
					}
				})
			}
		}
	}
}

func TestAdaptiveScoringDoesNotContinueAdaptation(t *testing.T) {
	for _, core := range []string{"continuous", "lif", "mixed"} {
		for _, receptor := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/receptor=%t", core, receptor), func(t *testing.T) {
				a := onlineReceptorPolicyIndividual(t, core, true, receptor, receptor, .25)
				b := onlineReceptorPolicyIndividual(t, core, true, receptor, receptor, .25)
				e := experiment.AdaptiveEvaluation{
					Mode: experiment.EvaluationModeAdaptive, FeedbackAvailable: []string{"score"},
					Adaptation: []experiment.EvaluationItem{{ID: "adapt", Input: onlineReceptorPolicyInput(), Target: []float64{.5}, AllowedFeedback: []string{"score"}}},
					Scoring:    []experiment.EvaluationItem{{ID: "score", Input: [][]float64{{2, 2}}, Target: []float64{0}}},
				}
				if _, err := experiment.RunAdaptiveEvaluation(context.Background(), a, e); err != nil {
					t.Fatal(err)
				}
				e.Scoring = append(e.Scoring, experiment.EvaluationItem{ID: "extra-score", Input: onlineReceptorPolicyInput(), Target: []float64{0}})
				if _, err := experiment.RunAdaptiveEvaluation(context.Background(), b, e); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(a.Snapshot().Plastic, b.Snapshot().Plastic) {
					t.Errorf("extra scoring changed the adapted fast state: %v versus %v", a.Snapshot().Plastic.State, b.Snapshot().Plastic.State)
				}
			})
		}
	}
}

// An interface keeps the pre-fix suite compilable so the original evaluator
// failures above are behavioral failures rather than missing-symbol errors.
type fixedAdvancer interface {
	AdvanceFixed(context.Context, [][]float64) ([][]float64, error)
}

func fixedAdvance(t *testing.T, a *learning.Individual, ctx context.Context, input [][]float64) ([][]float64, error) {
	t.Helper()
	fixed, ok := any(a).(fixedAdvancer)
	if !ok {
		t.Fatal("Individual has no per-call fixed inference operation")
	}
	return fixed.AdvanceFixed(ctx, input)
}

func TestAdvanceFixedPreservesRuntimePolicy(t *testing.T) {
	ctx := context.Background()
	for _, core := range []string{"continuous", "lif", "mixed"} {
		for _, frozen := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/frozen=%t", core, frozen), func(t *testing.T) {
				a := onlineReceptorPolicyIndividual(t, core, true, true, true, .25)
				oracle := onlineReceptorPolicyIndividual(t, core, true, true, true, .25)
				if err := a.FreezePlasticity(frozen); err != nil {
					t.Fatal(err)
				}
				if err := oracle.FreezePlasticity(true); err != nil {
					t.Fatal(err)
				}
				got, err := fixedAdvance(t, a, ctx, onlineReceptorPolicyInput())
				if err != nil {
					t.Fatal(err)
				}
				want, err := oracle.Advance(ctx, onlineReceptorPolicyInput())
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(a.Snapshot(), oracle.Snapshot()) {
					t.Fatal("fixed inference differs from the public frozen-state oracle")
				}
				if err := oracle.FreezePlasticity(frozen); err != nil {
					t.Fatal(err)
				}
				got, err = a.Advance(ctx, onlineReceptorPolicyInput())
				if err != nil {
					t.Fatal(err)
				}
				want, err = oracle.Advance(ctx, onlineReceptorPolicyInput())
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(a.Snapshot(), oracle.Snapshot()) {
					t.Error("per-call fixed inference changed the existing runtime policy")
				}
			})
		}
	}
}

func TestAdvanceFixedRefusalAndDisabledControl(t *testing.T) {
	ctx := context.Background()
	for _, failure := range []string{"nil-context", "canceled", "empty", "shape"} {
		t.Run(failure, func(t *testing.T) {
			a := onlineReceptorPolicyIndividual(t, "continuous", true, true, true, .25)
			control := onlineReceptorPolicyIndividual(t, "continuous", true, true, true, .25)
			before := a.Snapshot()
			badCtx, input := ctx, onlineReceptorPolicyInput()
			switch failure {
			case "nil-context":
				badCtx = nil
			case "canceled":
				var cancel context.CancelFunc
				badCtx, cancel = context.WithCancel(ctx)
				cancel()
			case "empty":
				input = nil
			case "shape":
				input = [][]float64{{1}}
			}
			out, err := fixedAdvance(t, a, badCtx, input)
			if err == nil || out != nil || !reflect.DeepEqual(before, a.Snapshot()) {
				t.Fatal("refused fixed inference published output or changed state")
			}
			_, err = a.Advance(ctx, onlineReceptorPolicyInput())
			if err != nil {
				t.Fatal(err)
			}
			_, err = control.Advance(ctx, onlineReceptorPolicyInput())
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(a.Snapshot(), control.Snapshot()) {
				t.Error("refused fixed inference changed the later native operation")
			}
		})
	}
	t.Run("nil-individual", func(t *testing.T) {
		var a *learning.Individual
		out, err := fixedAdvance(t, a, ctx, onlineReceptorPolicyInput())
		if err == nil || out != nil {
			t.Fatal("nil individual was accepted")
		}
	})
	t.Run("plasticity-disabled", func(t *testing.T) {
		a := onlineReceptorPolicyIndividual(t, "continuous", true, true, true, .25)
		b := onlineReceptorPolicyIndividual(t, "continuous", true, true, true, .25)
		a.DisablePlasticity()
		b.DisablePlasticity()
		got, err := fixedAdvance(t, a, ctx, onlineReceptorPolicyInput())
		if err != nil {
			t.Fatal(err)
		}
		want, err := b.Advance(ctx, onlineReceptorPolicyInput())
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(a.Snapshot(), b.Snapshot()) {
			t.Error("fixed inference changed the disabled-plasticity trajectory")
		}
	})
}

func TestAdvanceFixedSerializesWithNative(t *testing.T) {
	a := onlineReceptorPolicyIndividual(t, "continuous", false, false, false, .25)
	if err := a.DisableChemistry(); err != nil {
		t.Fatal(err)
	}
	fixed, ok := any(a).(fixedAdvancer)
	if !ok {
		t.Fatal("Individual has no per-call fixed inference operation")
	}
	const calls = 10
	start := make(chan struct{})
	errors := make(chan error, 2*calls)
	var workers sync.WaitGroup
	for _, advance := range []func(context.Context, [][]float64) ([][]float64, error){fixed.AdvanceFixed, a.Advance} {
		workers.Add(1)
		go func(advance func(context.Context, [][]float64) ([][]float64, error)) {
			defer workers.Done()
			<-start
			for range calls {
				_, err := advance(context.Background(), [][]float64{{0, 0}})
				errors <- err
			}
		}(advance)
	}
	close(start)
	workers.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	snap := a.Snapshot()
	if snap.Neural.Continuous.Steps != 2*calls || snap.Plastic.State.Plastic[0] != .25*math.Pow(.5, calls) {
		t.Fatalf("fixed and native operations did not retain their own policy: %+v", snap)
	}
}
