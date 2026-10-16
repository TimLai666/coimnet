package experiment_test

import (
	"context"
	"encoding/json"
	"math"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/experiment"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/plasticity"
)

func adaptiveMSE134Input() [][]float64 { return [][]float64{{0}} }

func adaptiveMSE134FreshSnapshot(t *testing.T) learning.IndividualSnapshot {
	t.Helper()
	config := learning.Config{
		Dynamics:     dynamics.Config{Nodes: 2, DT: 1, Activation: "tanh", Sources: []int{0}, Targets: []int{1}},
		InputSize:    1,
		OutputSize:   2,
		InputNodes:   []int{0},
		ReadoutNodes: []int{1},
	}
	parameters := learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{0}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}},
		Encoder: []float64{0},
		Readout: []float64{0, 0},
	}
	ind, err := learning.NewIndividual(config, parameters, learning.DefaultOptions(), []float64{0, 0})
	if err != nil {
		t.Fatalf("new two-node fixture: %v", err)
	}
	if err := ind.EnablePlasticity(plasticity.Config{
		Rule:  plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 1, WMin: .01},
		Edges: []int{0},
	}); err != nil {
		t.Fatalf("enable local plasticity: %v", err)
	}
	snapshot := ind.Snapshot()
	if snapshot.Plastic == nil || len(snapshot.Plastic.State.Plastic) != 1 {
		t.Fatalf("plastic fixture = %+v, want one edge", snapshot.Plastic)
	}
	// A non-zero fast state makes the no-feedback adaptation path observably run.
	snapshot.Plastic.State.Plastic[0] = .25
	return snapshot
}

func adaptiveMSE134Evaluation(mode, id string, target []float64) experiment.AdaptiveEvaluation {
	e := experiment.AdaptiveEvaluation{
		Mode:              mode,
		FeedbackAvailable: []string{"score"},
		Scoring:           []experiment.EvaluationItem{{ID: id, Input: adaptiveMSE134Input(), Target: target}},
	}
	if mode == experiment.EvaluationModeAdaptive {
		e.Adaptation = []experiment.EvaluationItem{{ID: id + "-adapt", Input: adaptiveMSE134Input(), Target: []float64{0, 0}}}
	}
	return e
}

func adaptiveMSE134ExactMSE(target []float64) float64 {
	var exact big.Rat
	for _, value := range target {
		var term big.Rat
		term.SetFloat64(value)
		term.Mul(&term, &term)
		exact.Add(&exact, &term)
	}
	exact.Quo(&exact, big.NewRat(int64(len(target)), 1))
	want, _ := exact.Float64()
	return want
}

func adaptiveMSE134OriginalMSE(output [][]float64, target []float64) float64 {
	last := output[len(output)-1]
	var sum float64
	for i, value := range last {
		delta := value - target[i]
		sum += delta * delta
	}
	return sum / float64(len(last))
}

func adaptiveMSE134AssertOverflow(t *testing.T, snapshot learning.IndividualSnapshot, evaluation experiment.AdaptiveEvaluation, id string) {
	t.Helper()
	ind, err := learning.RestoreIndividual(snapshot)
	if err != nil {
		t.Fatalf("restore %q fixture: %v", id, err)
	}
	report, err := experiment.RunAdaptiveEvaluation(context.Background(), ind, evaluation)
	if err == nil {
		t.Fatalf("%q: expected unrepresentable MSE error, got report %+v", id, report)
	}
	if !strings.Contains(err.Error(), id) {
		t.Fatalf("error %q does not contain item ID %q", err, id)
	}
	if !reflect.DeepEqual(report, experiment.EvaluationReport{}) {
		t.Fatalf("%q: error returned non-zero report %+v", id, report)
	}
}

func TestAdaptiveMSE134(t *testing.T) {
	initial := adaptiveMSE134FreshSnapshot(t)
	finiteTarget := []float64{1e154, 1e154}
	for _, mode := range []string{experiment.EvaluationModeFixed, experiment.EvaluationModeAdaptive} {
		t.Run("finite-"+mode, func(t *testing.T) {
			ind, err := learning.RestoreIndividual(initial)
			if err != nil {
				t.Fatal(err)
			}
			evaluation := adaptiveMSE134Evaluation(mode, "finite-"+mode, finiteTarget)
			report, err := experiment.RunAdaptiveEvaluation(context.Background(), ind, evaluation)
			if err != nil {
				t.Fatalf("finite %s evaluation: %v", mode, err)
			}
			if len(report.Scores) != 1 || math.IsInf(report.Scores[0].MSE, 0) || math.IsNaN(report.Scores[0].MSE) {
				t.Fatalf("finite %s scores = %+v, want one finite score", mode, report.Scores)
			}
			want := adaptiveMSE134ExactMSE(finiteTarget)
			if math.Float64bits(report.Scores[0].MSE) != math.Float64bits(want) {
				t.Fatalf("finite %s MSE = %g (%x), want exact Rat reference %g (%x)", mode, report.Scores[0].MSE, math.Float64bits(report.Scores[0].MSE), want, math.Float64bits(want))
			}
			if _, err := json.Marshal(report); err != nil {
				t.Fatalf("finite %s report does not marshal: %v", mode, err)
			}
		})
	}

	t.Run("ordinary-control", func(t *testing.T) {
		ind, err := learning.RestoreIndividual(initial)
		if err != nil {
			t.Fatal(err)
		}
		target := []float64{.5, .5}
		evaluation := adaptiveMSE134Evaluation(experiment.EvaluationModeFixed, "ordinary", target)
		report, err := experiment.RunAdaptiveEvaluation(context.Background(), ind, evaluation)
		if err != nil {
			t.Fatal(err)
		}
		output := report.Scores[0].Output
		if got, want := report.Scores[0].MSE, adaptiveMSE134OriginalMSE(output, target); got != want {
			t.Fatalf("ordinary MSE = %g, want original formula %g", got, want)
		}
		if _, err := json.Marshal(report); err != nil {
			t.Fatalf("ordinary report does not marshal: %v", err)
		}
	})

	maxTarget := []float64{math.MaxFloat64, math.MaxFloat64}
	t.Run("fixed-overflow", func(t *testing.T) {
		adaptiveMSE134AssertOverflow(t, initial, adaptiveMSE134Evaluation(experiment.EvaluationModeFixed, "fixed-overflow", maxTarget), "fixed-overflow")
	})
	t.Run("adaptive-overflow", func(t *testing.T) {
		adaptiveMSE134AssertOverflow(t, initial, adaptiveMSE134Evaluation(experiment.EvaluationModeAdaptive, "adaptive-overflow", maxTarget), "adaptive-overflow")
	})
	t.Run("allowed-score-overflow", func(t *testing.T) {
		allowed := adaptiveMSE134Evaluation(experiment.EvaluationModeAdaptive, "allowed-score-overflow", []float64{0, 0})
		allowed.Adaptation[0].ID = "allowed-score-overflow-adaptation"
		allowed.Adaptation[0].Target = maxTarget
		allowed.Adaptation[0].AllowedFeedback = []string{"score"}
		adaptiveMSE134AssertOverflow(t, initial, allowed, "allowed-score-overflow-adaptation")
	})

	t.Run("closed-feedback", func(t *testing.T) {
		ordinary := adaptiveMSE134Evaluation(experiment.EvaluationModeAdaptive, "closed-score", []float64{.5, .5})
		ordinary.Adaptation[0].ID = "closed-adaptation"
		bigTarget := ordinary
		bigTarget.Adaptation = append([]experiment.EvaluationItem(nil), ordinary.Adaptation...)
		bigTarget.Adaptation[0].Target = maxTarget
		ordinaryInd, err := learning.RestoreIndividual(initial)
		if err != nil {
			t.Fatal(err)
		}
		bigInd, err := learning.RestoreIndividual(initial)
		if err != nil {
			t.Fatal(err)
		}
		ordinaryReport, err := experiment.RunAdaptiveEvaluation(context.Background(), ordinaryInd, ordinary)
		if err != nil {
			t.Fatalf("ordinary closed-gate adaptation: %v", err)
		}
		bigReport, err := experiment.RunAdaptiveEvaluation(context.Background(), bigInd, bigTarget)
		if err != nil {
			t.Fatalf("huge closed-gate adaptation: %v", err)
		}
		if !reflect.DeepEqual(ordinaryReport, bigReport) || !reflect.DeepEqual(ordinaryInd.Snapshot(), bigInd.Snapshot()) {
			t.Fatalf("forbidden score target changed report or individual snapshot:\nordinary=%+v\nbig=%+v", ordinaryReport, bigReport)
		}
		if reflect.DeepEqual(initial.Plastic.State.Plastic, bigInd.Snapshot().Plastic.State.Plastic) {
			t.Fatal("closed-gate adaptation did not advance the seeded fast state")
		}
	})
}
