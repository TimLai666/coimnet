package experiment

import (
	"context"
	"fmt"
	"reflect"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/signal"
)

// ItemScore records the score of one evaluation item.
type ItemScore struct {
	ID     string      `json:"id"`
	MSE    float64     `json:"mse"`
	Output [][]float64 `json:"output"`
}

// ContaminationChecks records whether parameters were modified during scoring
// and how many policy violations occurred.
type ContaminationChecks struct {
	ParametersUnchanged     bool  `json:"parameters_unchanged"`
	ScoringFeedbackRejected int   `json:"scoring_feedback_rejected"`
	ReplayRejected          int   `json:"replay_rejected"`
	ShuffleInvariant        *bool `json:"shuffle_invariant,omitempty"`
}

// EvaluationReport is the full output of one adaptive evaluation run.
type EvaluationReport struct {
	SchemaVersion     string              `json:"schema_version"`
	Mode              string              `json:"mode"`
	Reset             ResetPolicy         `json:"reset"`
	FeedbackAvailable []string            `json:"feedback_available,omitempty"`
	AdaptationItems   int                 `json:"adaptation_items"`
	ScoringItems      int                 `json:"scoring_items"`
	Scores            []ItemScore         `json:"scores"`
	Contamination     ContaminationChecks `json:"contamination_checks"`
}

// RunAdaptiveEvaluation runs the declared evaluation on one individual.
func RunAdaptiveEvaluation(ctx context.Context, ind *learning.Individual, e AdaptiveEvaluation) (EvaluationReport, error) {
	if ctx == nil {
		return EvaluationReport{}, fmt.Errorf("context must not be nil")
	}
	if ind == nil {
		return EvaluationReport{}, fmt.Errorf("individual must not be nil")
	}
	if err := e.Validate(); err != nil {
		return EvaluationReport{}, err
	}
	if e.Mode == EvaluationModeAdaptive {
		return runAdaptiveEvaluation(ctx, ind, e)
	}
	// Fixed mode implementation.
	startSnapshot := ind.Snapshot()
	scores, err := scoreItems(ctx, ind, e.Scoring, e.Reset)
	if err != nil {
		return EvaluationReport{}, err
	}
	after := ind.Snapshot().Parameters

	replayRejected, err := ReplayRejectionCount(e)
	if err != nil {
		return EvaluationReport{}, err
	}
	var shuffleInvariant *bool
	if e.Reset.NeuralAtItemStart && e.Reset.PlasticAtItemStart && e.Reset.ChemicalAtItemStart {
		ok, err := ShuffleInvariance(ctx, startSnapshot, e)
		if err != nil {
			return EvaluationReport{}, err
		}
		shuffleInvariant = &ok
	}

	return EvaluationReport{
		SchemaVersion:     evalSchemaVersion,
		Mode:              e.Mode,
		Reset:             e.Reset,
		FeedbackAvailable: e.FeedbackAvailable,
		AdaptationItems:   len(e.Adaptation),
		ScoringItems:      len(e.Scoring),
		Scores:            scores,
		Contamination: ContaminationChecks{
			ParametersUnchanged: reflect.DeepEqual(startSnapshot.Parameters, after),
			ReplayRejected:      replayRejected,
			ShuffleInvariant:    shuffleInvariant,
		},
	}, nil
}

// runAdaptiveEvaluation runs the adaptive mode: the adaptation partition is
// replayed through an OnlineLearner with a plastic policy (base parameters and
// fast state are changed only by the plasticity the learner applies), and the
// scoring partition runs the same closed-gate evaluation as the fixed mode.
func runAdaptiveEvaluation(ctx context.Context, ind *learning.Individual, e AdaptiveEvaluation) (EvaluationReport, error) {
	before := ind.Snapshot().Parameters
	gate := learning.RewardGateFunc(func(score float64) ([]float64, error) {
		if score < 0 {
			score = 0
		}
		if score > 1 {
			score = 1
		}
		return []float64{score}, nil
	})
	learner, err := learning.NewOnlineLearner(ind, learning.OnlinePolicy{
		AllowGradient: false,
		AllowPlastic:  true,
		Evaluate:      false,
		UpdateEvery:   1,
		ClockUnit:     signal.TimeUnitModelStep,
	}, gate)
	if err != nil {
		return EvaluationReport{}, fmt.Errorf("adaptive mode needs local plasticity: %w", err)
	}

	scoringRejected := 0
	for _, item := range e.Scoring {
		if len(item.AllowedFeedback) != 0 {
			scoringRejected++
		}
	}
	version := signal.CurrentSchemaVersion()
	for _, item := range e.Adaptation {
		if err := resetForItem(ctx, ind, e.Reset, item.ID); err != nil {
			return EvaluationReport{}, err
		}
		obs, err := signal.NewObservation(version, item.ID, "adaptive", nil)
		if err != nil {
			return EvaluationReport{}, err
		}
		record, err := learner.Act(ctx, obs, item.Input)
		if err != nil {
			return EvaluationReport{}, err
		}
		mse, err := itemMSE(record.Output, item)
		if err != nil {
			return EvaluationReport{}, err
		}
		score := 1 - mse
		if score < 0 {
			score = 0
		}
		if score > 1 {
			score = 1
		}
		at := signal.Timestamp{Value: int64(record.Step), Unit: signal.TimeUnitModelStep}
		fb, err := signal.NewFeedback(signal.FeedbackSpec{
			SchemaVersion: version,
			ExperienceID:  item.ID,
			ActionID:      record.ActionID,
			ProducedAt:    at,
			AvailableAt:   at,
			Source:        "score",
			Score:         score,
			ModelVersion:  version,
		})
		if err != nil {
			return EvaluationReport{}, err
		}
		if err := learner.Receive(ctx, fb); err != nil {
			return EvaluationReport{}, err
		}
		if _, err := learner.Update(ctx, at); err != nil {
			return EvaluationReport{}, err
		}
	}

	startSnapshot := ind.Snapshot()
	scores, err := scoreItems(ctx, ind, e.Scoring, e.Reset)
	if err != nil {
		return EvaluationReport{}, err
	}

	after := ind.Snapshot().Parameters

	replayRejected, err := ReplayRejectionCount(e)
	if err != nil {
		return EvaluationReport{}, err
	}
	var shuffleInvariant *bool
	if e.Reset.NeuralAtItemStart && e.Reset.PlasticAtItemStart && e.Reset.ChemicalAtItemStart {
		ok, err := ShuffleInvariance(ctx, startSnapshot, e)
		if err != nil {
			return EvaluationReport{}, err
		}
		shuffleInvariant = &ok
	}

	return EvaluationReport{
		SchemaVersion:     evalSchemaVersion,
		Mode:              EvaluationModeAdaptive,
		Reset:             e.Reset,
		FeedbackAvailable: e.FeedbackAvailable,
		AdaptationItems:   len(e.Adaptation),
		ScoringItems:      len(e.Scoring),
		Scores:            scores,
		Contamination: ContaminationChecks{
			ParametersUnchanged:     reflect.DeepEqual(before, after),
			ScoringFeedbackRejected: scoringRejected,
			ReplayRejected:          replayRejected,
			ShuffleInvariant:        shuffleInvariant,
		},
	}, nil
}

// scoreItems runs the closed-gate scoring flow of one partition on an
// individual: it applies the declared reset before every item, advances the
// individual, records the output and its MSE, and returns the item scores in
// the order the items were given.
func scoreItems(ctx context.Context, ind *learning.Individual, items []EvaluationItem, reset ResetPolicy) ([]ItemScore, error) {
	scores := make([]ItemScore, 0, len(items))
	for _, item := range items {
		if err := resetForItem(ctx, ind, reset, item.ID); err != nil {
			return nil, err
		}
		out, err := ind.Advance(ctx, item.Input)
		if err != nil {
			return nil, err
		}
		mse, err := itemMSE(out, item)
		if err != nil {
			return nil, err
		}
		scores = append(scores, ItemScore{ID: item.ID, MSE: mse, Output: out})
	}
	return scores, nil
}

// resetForItem applies the declared reset policy before one item. The plasticity
// reset re-enables the mechanism from the config that was active before the
// reset, which starts the fast state over.
func resetForItem(ctx context.Context, ind *learning.Individual, reset ResetPolicy, itemID string) error {
	if reset.NeuralAtItemStart {
		nodes := len(ind.Snapshot().Parameters.Core.Bias)
		if err := ind.ResetNeural(ctx, make([]float64, nodes)); err != nil {
			return err
		}
	}
	if reset.PlasticAtItemStart {
		snap := ind.Snapshot()
		if snap.Plastic != nil {
			ind.DisablePlasticity()
			if err := ind.EnablePlasticity(snap.Plastic.Config); err != nil {
				return err
			}
		}
	}
	if reset.ChemicalAtItemStart {
		snap := ind.Snapshot()
		if snap.Chemical != nil {
			ind.DisableChemistry()
			if err := ind.EnableChemistry(snap.Chemical.Config); err != nil {
				return err
			}
		}
	}
	return nil
}

// itemMSE is the mean squared error between the last output row and the item's
// target, after checking the widths match.
func itemMSE(out [][]float64, item EvaluationItem) (float64, error) {
	lastRow := out[len(out)-1]
	if len(lastRow) != len(item.Target) {
		return 0, fmt.Errorf("item %q: output width %d does not match target width %d", item.ID, len(lastRow), len(item.Target))
	}
	var mse float64
	for i := range lastRow {
		d := lastRow[i] - item.Target[i]
		mse += d * d
	}
	return mse / float64(len(lastRow)), nil
}

const evalSchemaVersion = "coimnet-adaptive-evaluation/v1"
