package experiment_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/experiment"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/plasticity"
)

const evalSchemaVersion = "coimnet-adaptive-evaluation/v1"

func fixedEvaluation() experiment.AdaptiveEvaluation {
	return experiment.AdaptiveEvaluation{
		Mode: experiment.EvaluationModeFixed,
		Scoring: []experiment.EvaluationItem{
			{ID: "score-a", Input: [][]float64{{.5}, {0}, {.2}}, Target: []float64{.4}},
			{ID: "score-b", Input: [][]float64{{-.3}, {.1}}, Target: []float64{-.1}},
		},
		Reset: experiment.ResetPolicy{
			NeuralAtItemStart:   true,
			PlasticAtItemStart:  true,
			ChemicalAtItemStart: true,
		},
	}
}

func newAdaptiveIndividual(t *testing.T) *learning.Individual {
	t.Helper()
	config := learning.Config{
		Dynamics:     dynamics.Config{Nodes: 2, DT: 0.5, Activation: "tanh", Sources: []int{0}, Targets: []int{1}},
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: []int{1},
	}
	params := learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{.3}, Bias: []float64{.1, -.1}, LogTau: []float64{.1, .2}},
		Encoder: []float64{.5, .1},
		Readout: []float64{.8},
	}
	ind, err := learning.NewIndividual(config, params, learning.DefaultOptions(), make([]float64, config.Dynamics.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	return ind
}

func TestRunAdaptiveEvaluationFixedMode(t *testing.T) {
	ctx := context.Background()

	// (a) Basic properties of a fixed-mode run.
	t.Run("basic properties", func(t *testing.T) {
		ind := newAdaptiveIndividual(t)
		e := fixedEvaluation()
		report, err := experiment.RunAdaptiveEvaluation(ctx, ind, e)
		if err != nil {
			t.Fatal(err)
		}
		if report.SchemaVersion != evalSchemaVersion {
			t.Fatalf("schema_version = %q, want %q", report.SchemaVersion, evalSchemaVersion)
		}
		if report.Mode != experiment.EvaluationModeFixed {
			t.Fatalf("mode = %q, want %q", report.Mode, experiment.EvaluationModeFixed)
		}
		if report.ScoringItems != 2 {
			t.Fatalf("scoring_items = %d, want 2", report.ScoringItems)
		}
		if report.AdaptationItems != 0 {
			t.Fatalf("adaptation_items = %d, want 0", report.AdaptationItems)
		}
		if !report.Contamination.ParametersUnchanged {
			t.Fatal("parameters_unchanged = false, want true")
		}
		if report.Contamination.ScoringFeedbackRejected != 0 {
			t.Fatalf("scoring_feedback_rejected = %d, want 0", report.Contamination.ScoringFeedbackRejected)
		}
		if report.Contamination.ReplayRejected != len(e.Scoring) {
			t.Fatalf("replay_rejected = %d, want %d", report.Contamination.ReplayRejected, len(e.Scoring))
		}
		if report.Contamination.ShuffleInvariant == nil || !*report.Contamination.ShuffleInvariant {
			t.Fatalf("shuffle_invariant = %v, want non-nil true", report.Contamination.ShuffleInvariant)
		}
		if len(report.Scores) != 2 {
			t.Fatalf("len(scores) = %d, want 2", len(report.Scores))
		}
		for i, item := range e.Scoring {
			score := report.Scores[i]
			if score.ID != item.ID {
				t.Fatalf("score[%d].ID = %q, want %q", i, score.ID, item.ID)
			}
			if len(score.Output) != len(item.Input) {
				t.Fatalf("score[%d].Output rows = %d, want %d", i, len(score.Output), len(item.Input))
			}
			if len(score.Output) == 0 || len(score.Output[0]) == 0 {
				t.Fatalf("score[%d].Output is empty", i)
			}
			if len(item.Target) != len(score.Output[len(score.Output)-1]) {
				t.Fatalf("score[%d] target/output width mismatch: %d vs %d", i, len(item.Target), len(score.Output[len(score.Output)-1]))
			}
			if score.MSE < 0 {
				t.Fatalf("score[%d].MSE = %g, want >= 0", i, score.MSE)
			}
			if score.MSE != score.MSE {
				t.Fatalf("score[%d].MSE is NaN", i)
			}
		}
	})

	// (b) Deterministic reproducibility: two runs with the same reset give
	// identical scores.
	t.Run("deterministic", func(t *testing.T) {
		ind := newAdaptiveIndividual(t)
		e := fixedEvaluation()
		r1, err := experiment.RunAdaptiveEvaluation(ctx, ind, e)
		if err != nil {
			t.Fatal(err)
		}
		r2, err := experiment.RunAdaptiveEvaluation(ctx, ind, e)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(r1.Scores, r2.Scores) {
			t.Fatalf("scores differ between runs:\n%+v\nvs\n%+v", r1.Scores, r2.Scores)
		}
	})

	// (c) nil individual returns error.
	t.Run("nil individual", func(t *testing.T) {
		_, err := experiment.RunAdaptiveEvaluation(ctx, nil, fixedEvaluation())
		if err == nil {
			t.Fatal("expected error for nil individual")
		}
	})

	// (d) target length differs from output width → error.
	t.Run("target width mismatch", func(t *testing.T) {
		ind := newAdaptiveIndividual(t)
		e := experiment.AdaptiveEvaluation{
			Mode: experiment.EvaluationModeFixed,
			Scoring: []experiment.EvaluationItem{
				{ID: "bad-target", Input: [][]float64{{.5}}, Target: []float64{.1, .2}},
			},
			Reset: experiment.ResetPolicy{NeuralAtItemStart: true},
		}
		_, err := experiment.RunAdaptiveEvaluation(ctx, ind, e)
		if err == nil {
			t.Fatal("expected error for target width mismatch")
		}
	})
}

func TestRunAdaptiveEvaluationAdaptiveMode(t *testing.T) {
	ctx := context.Background()
	ind := newAdaptiveIndividual(t)
	rule := plasticity.Rule{
		Kind:       plasticity.RuleHebbianRate,
		DecayE:     0.5,
		DecayP:     0.5,
		PlasticMax: 1,
		WMin:       0.01,
	}
	if err := ind.EnablePlasticity(plasticity.Config{Rule: rule, Edges: []int{0}}); err != nil {
		t.Fatal(err)
	}
	beforePlastic := ind.Snapshot().Plastic.State.Plastic

	e := experiment.AdaptiveEvaluation{
		Mode: experiment.EvaluationModeAdaptive,
		Adaptation: []experiment.EvaluationItem{
			{ID: "adapt-a", Input: [][]float64{{.5}, {0}, {.2}}, Target: []float64{.4}},
			{ID: "adapt-b", Input: [][]float64{{-.3}, {.1}}, Target: []float64{-.1}},
		},
		Scoring: []experiment.EvaluationItem{
			{ID: "score-a", Input: [][]float64{{.5}, {0}, {.2}}, Target: []float64{.4}},
			{ID: "score-b", Input: [][]float64{{-.3}, {.1}}, Target: []float64{-.1}},
		},
		Reset: experiment.ResetPolicy{},
	}

	report, err := experiment.RunAdaptiveEvaluation(ctx, ind, e)
	if err != nil {
		t.Fatal(err)
	}

	if report.Mode != experiment.EvaluationModeAdaptive {
		t.Fatalf("mode = %q, want %q", report.Mode, experiment.EvaluationModeAdaptive)
	}
	if report.AdaptationItems != 2 {
		t.Fatalf("adaptation_items = %d, want 2", report.AdaptationItems)
	}
	if report.ScoringItems != 2 {
		t.Fatalf("scoring_items = %d, want 2", report.ScoringItems)
	}
	if !report.Contamination.ParametersUnchanged {
		t.Fatal("parameters_unchanged = false, want true")
	}
	afterPlastic := ind.Snapshot().Plastic.State.Plastic
	if reflect.DeepEqual(beforePlastic, afterPlastic) {
		t.Fatal("plastic state unchanged after adaptation, expected it to differ")
	}
	if report.Contamination.ScoringFeedbackRejected != 0 {
		t.Fatalf("scoring_feedback_rejected = %d, want 0", report.Contamination.ScoringFeedbackRejected)
	}
}

func TestRunAdaptiveEvaluationAdaptiveNeedsPlasticity(t *testing.T) {
	ctx := context.Background()
	ind := newAdaptiveIndividual(t)
	e := experiment.AdaptiveEvaluation{
		Mode: experiment.EvaluationModeAdaptive,
		Adaptation: []experiment.EvaluationItem{
			{ID: "adapt-a", Input: [][]float64{{.5}}, Target: []float64{.4}},
		},
		Scoring: []experiment.EvaluationItem{
			{ID: "score-a", Input: [][]float64{{.5}}, Target: []float64{.4}},
		},
		Reset: experiment.ResetPolicy{},
	}
	_, err := experiment.RunAdaptiveEvaluation(ctx, ind, e)
	if err == nil {
		t.Fatal("expected error for adaptive mode without plasticity")
	}
	if !strings.Contains(err.Error(), "plasticity") {
		t.Fatalf("error %q does not contain %q", err, "plasticity")
	}
}

func TestRunAdaptiveEvaluationFixedModeUnchanged(t *testing.T) {
	ctx := context.Background()
	ind := newAdaptiveIndividual(t)
	e := fixedEvaluation()
	r1, err := experiment.RunAdaptiveEvaluation(ctx, ind, e)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := experiment.RunAdaptiveEvaluation(ctx, ind, e)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r1, r2) {
		t.Fatalf("two fixed-mode runs differ:\n%+v\nvs\n%+v", r1, r2)
	}
}
