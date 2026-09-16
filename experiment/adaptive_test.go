package experiment_test

import (
	"math"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/experiment"
)

func adaptiveEvaluation() experiment.AdaptiveEvaluation {
	return experiment.AdaptiveEvaluation{
		Mode: experiment.EvaluationModeAdaptive,
		Adaptation: []experiment.EvaluationItem{
			{ID: "adapt-a", Input: [][]float64{{1, 0}, {0, 1}}, Target: []float64{.5}, AllowedFeedback: []string{"score"}},
			{ID: "adapt-b", Input: [][]float64{{0, 1}}, Target: []float64{-.5}, AllowedFeedback: []string{"score"}},
		},
		Scoring: []experiment.EvaluationItem{
			{ID: "score-a", Input: [][]float64{{1, 0}}, Target: []float64{.25}},
			{ID: "score-b", Input: [][]float64{{0, 1}}, Target: []float64{-.25}},
		},
		FeedbackAvailable: []string{"score"},
		Reset:             experiment.ResetPolicy{NeuralAtItemStart: true},
		Seed:              7,
	}
}

func TestAdaptiveEvaluationValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*experiment.AdaptiveEvaluation)
		want   []string
	}{
		{name: "valid adaptive declaration passes"},
		{name: "valid fixed declaration passes", mutate: func(e *experiment.AdaptiveEvaluation) {
			e.Mode = experiment.EvaluationModeFixed
			e.Adaptation = nil
		}},
		{name: "invalid mode is rejected", mutate: func(e *experiment.AdaptiveEvaluation) { e.Mode = "supervised" }, want: []string{"mode"}},
		{name: "empty scoring partition is rejected", mutate: func(e *experiment.AdaptiveEvaluation) { e.Scoring = nil }, want: []string{"scoring"}},
		{name: "fixed mode with adaptation is rejected", mutate: func(e *experiment.AdaptiveEvaluation) { e.Mode = experiment.EvaluationModeFixed }, want: []string{"mode"}},
		{name: "duplicate id across partitions is rejected", mutate: func(e *experiment.AdaptiveEvaluation) { e.Scoring[0].ID = "adapt-a" }, want: []string{"adapt-a"}},
		{name: "empty item id is rejected", mutate: func(e *experiment.AdaptiveEvaluation) { e.Adaptation[0].ID = "" }, want: []string{"id"}},
		{name: "input with no rows is rejected", mutate: func(e *experiment.AdaptiveEvaluation) { e.Adaptation[0].Input = nil }, want: []string{"adapt-a", "input"}},
		{name: "input rows of unequal length are rejected", mutate: func(e *experiment.AdaptiveEvaluation) { e.Adaptation[0].Input = [][]float64{{1, 0}, {0}} }, want: []string{"adapt-a", "input"}},
		{name: "input containing NaN is rejected", mutate: func(e *experiment.AdaptiveEvaluation) { e.Adaptation[0].Input[0][0] = math.NaN() }, want: []string{"adapt-a", "input"}},
		{name: "input containing Inf is rejected", mutate: func(e *experiment.AdaptiveEvaluation) { e.Adaptation[0].Input[0][0] = math.Inf(1) }, want: []string{"adapt-a", "input"}},
		{name: "empty target is rejected", mutate: func(e *experiment.AdaptiveEvaluation) { e.Scoring[0].Target = nil }, want: []string{"score-a", "target"}},
		{name: "target containing Inf is rejected", mutate: func(e *experiment.AdaptiveEvaluation) { e.Scoring[0].Target[0] = math.Inf(-1) }, want: []string{"score-a", "target"}},
		{name: "feedback outside feedback_available is rejected", mutate: func(e *experiment.AdaptiveEvaluation) { e.Adaptation[0].AllowedFeedback = []string{"hint"} }, want: []string{"adapt-a", "allowed_feedback"}},
		{name: "scoring item with allowed_feedback is rejected", mutate: func(e *experiment.AdaptiveEvaluation) { e.Scoring[0].AllowedFeedback = []string{"score"} }, want: []string{"score-a", "allowed_feedback"}},
		{name: "duplicate feedback_available is rejected", mutate: func(e *experiment.AdaptiveEvaluation) { e.FeedbackAvailable = []string{"score", "score"} }, want: []string{"feedback_available"}},
		{name: "empty feedback_available string is rejected", mutate: func(e *experiment.AdaptiveEvaluation) { e.FeedbackAvailable = []string{"", "score"} }, want: []string{"feedback_available"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eval := adaptiveEvaluation()
			if tt.mutate != nil {
				tt.mutate(&eval)
			}
			err := eval.Validate()
			if len(tt.want) == 0 {
				if err != nil {
					t.Fatalf("expected valid declaration, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected validation error containing %q, got nil", strings.Join(tt.want, ", "))
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}
