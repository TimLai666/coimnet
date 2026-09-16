package experiment

import (
	"context"
	"fmt"
	"reflect"

	"github.com/TimLai666/coimnet/learning"
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
// This ticket implements only Mode == EvaluationModeFixed; Mode == adaptive
// returns an error "adaptive mode is not implemented yet".
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
		return EvaluationReport{}, fmt.Errorf("adaptive mode is not implemented yet")
	}
	// Fixed mode implementation.
	before := ind.Snapshot().Parameters
	scores := make([]ItemScore, 0, len(e.Scoring))
	for _, item := range e.Scoring {
		// Reset neural state.
		if e.Reset.NeuralAtItemStart {
			nodes := len(ind.Snapshot().Parameters.Core.Bias)
			if err := ind.ResetNeural(ctx, make([]float64, nodes)); err != nil {
				return EvaluationReport{}, err
			}
		}
		// Reset plasticity.
		if e.Reset.PlasticAtItemStart {
			snap := ind.Snapshot()
			if snap.Plastic != nil {
				ind.DisablePlasticity()
				if err := ind.EnablePlasticity(snap.Plastic.Config); err != nil {
					return EvaluationReport{}, err
				}
			}
		}
		// Reset chemistry.
		if e.Reset.ChemicalAtItemStart {
			snap := ind.Snapshot()
			if snap.Chemical != nil {
				ind.DisableChemistry()
				if err := ind.EnableChemistry(snap.Chemical.Config); err != nil {
					return EvaluationReport{}, err
				}
			}
		}
		out, err := ind.Advance(ctx, item.Input)
		if err != nil {
			return EvaluationReport{}, err
		}
		lastRow := out[len(out)-1]
		if len(lastRow) != len(item.Target) {
			return EvaluationReport{}, fmt.Errorf("item %q: output width %d does not match target width %d", item.ID, len(lastRow), len(item.Target))
		}
		var mse float64
		for i := range lastRow {
			d := lastRow[i] - item.Target[i]
			mse += d * d
		}
		mse /= float64(len(lastRow))
		scores = append(scores, ItemScore{ID: item.ID, MSE: mse, Output: out})
	}
	after := ind.Snapshot().Parameters
	return EvaluationReport{
		SchemaVersion:     evalSchemaVersion,
		Mode:              e.Mode,
		Reset:             e.Reset,
		FeedbackAvailable: e.FeedbackAvailable,
		AdaptationItems:   len(e.Adaptation),
		ScoringItems:      len(e.Scoring),
		Scores:            scores,
		Contamination: ContaminationChecks{
			ParametersUnchanged: reflect.DeepEqual(before, after),
		},
	}, nil
}

const evalSchemaVersion = "coimnet-adaptive-evaluation/v1"
