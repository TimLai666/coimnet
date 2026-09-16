package experiment_test

import (
	"context"
	"testing"

	"github.com/TimLai666/coimnet/experiment"
)

func TestReplayRejectionCountRefusesEveryScoringItem(t *testing.T) {
	e := experiment.AdaptiveEvaluation{
		Scoring: []experiment.EvaluationItem{
			{ID: "score-1", Input: [][]float64{{.1}}, Target: []float64{.1}},
			{ID: "score-2", Input: [][]float64{{.2}}, Target: []float64{.2}},
			{ID: "score-3", Input: [][]float64{{.3}}, Target: []float64{.3}},
		},
		Seed: 7,
	}
	count, err := experiment.ReplayRejectionCount(e)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("replay_rejected = %d, want 3", count)
	}
}

func TestShuffleInvarianceHoldsWithFullReset(t *testing.T) {
	ctx := context.Background()
	ind := newAdaptiveIndividual(t)
	snapshot := ind.Snapshot()
	e := experiment.AdaptiveEvaluation{
		Scoring: []experiment.EvaluationItem{
			{ID: "a", Input: [][]float64{{.1}}, Target: []float64{.2}},
			{ID: "b", Input: [][]float64{{.3}}, Target: []float64{.1}},
			{ID: "c", Input: [][]float64{{.5}}, Target: []float64{.6}},
			{ID: "d", Input: [][]float64{{.7}}, Target: []float64{.2}},
		},
		Reset: experiment.ResetPolicy{
			NeuralAtItemStart:   true,
			PlasticAtItemStart:  true,
			ChemicalAtItemStart: true,
		},
		Seed: 7,
	}
	ok, err := experiment.ShuffleInvariance(ctx, snapshot, e)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("shuffle_invariance = false with full reset, want true")
	}
}

func TestShuffleInvarianceFailsWithoutNeuralReset(t *testing.T) {
	ctx := context.Background()
	ind := newAdaptiveIndividual(t)
	snapshot := ind.Snapshot()
	e := experiment.AdaptiveEvaluation{
		Scoring: []experiment.EvaluationItem{
			{ID: "score-0", Input: [][]float64{{1.0}, {1.0}, {1.0}}, Target: []float64{0}},
			{ID: "score-1", Input: [][]float64{{0.1}}, Target: []float64{0}},
			{ID: "score-2", Input: [][]float64{{0.1}}, Target: []float64{0}},
			{ID: "score-3", Input: [][]float64{{0.1}}, Target: []float64{0}},
		},
		Reset: experiment.ResetPolicy{
			NeuralAtItemStart:   false,
			PlasticAtItemStart:  true,
			ChemicalAtItemStart: true,
		},
		Seed: 7,
	}
	ok, err := experiment.ShuffleInvariance(ctx, snapshot, e)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("shuffle_invariance = true without neural reset, want false")
	}
}

func TestRunAdaptiveEvaluationFillsContaminationChecks(t *testing.T) {
	ctx := context.Background()

	t.Run("full reset", func(t *testing.T) {
		ind := newAdaptiveIndividual(t)
		e := fixedEvaluation()
		report, err := experiment.RunAdaptiveEvaluation(ctx, ind, e)
		if err != nil {
			t.Fatal(err)
		}
		if report.Contamination.ReplayRejected != len(e.Scoring) {
			t.Fatalf("replay_rejected = %d, want %d", report.Contamination.ReplayRejected, len(e.Scoring))
		}
		if report.Contamination.ShuffleInvariant == nil || !*report.Contamination.ShuffleInvariant {
			t.Fatalf("shuffle_invariant = %v, want non-nil true", report.Contamination.ShuffleInvariant)
		}
	})

	t.Run("partial reset", func(t *testing.T) {
		ind := newAdaptiveIndividual(t)
		e := fixedEvaluation()
		e.Reset.NeuralAtItemStart = false
		report, err := experiment.RunAdaptiveEvaluation(ctx, ind, e)
		if err != nil {
			t.Fatal(err)
		}
		if report.Contamination.ShuffleInvariant != nil {
			t.Fatalf("shuffle_invariant = %v, want nil", *report.Contamination.ShuffleInvariant)
		}
		if report.Contamination.ReplayRejected != len(e.Scoring) {
			t.Fatalf("replay_rejected = %d, want %d", report.Contamination.ReplayRejected, len(e.Scoring))
		}
	})
}
