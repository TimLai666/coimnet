package main

import (
	"context"
	"fmt"
	"math"

	"github.com/TimLai666/coimnet/tasks/nav2d/trajectory"
)

const (
	historyMaxSteps        = 16384
	historyPoseScale       = 30.0
	historyDisplacement    = 2.0
	historyMaxDeltaT       = 0.5
	historyMaxStepDistance = 5.0

	realSourceSHA256 = "83e13b7057957cf41a45dc91996a4519be7066519242b6912c65b2418cb91670"
	realDOI          = "10.5061/dryad.vdncjsz0b"
	realPinnedCommit = "0eb07940a9ffdedd97ada81f75e9d5f7bface579"
)

type historyStep struct {
	Input  [6]float64 `json:"input"`
	Target [2]float64 `json:"target"`
	Scored bool       `json:"scored"`
	T      float64    `json:"t"`
}

type historyTrial struct {
	ID           string        `json:"trial_id"`
	Condition    string        `json:"condition"`
	Steps        []historyStep `json:"steps"`
	RolloutIndex int           `json:"rollout_index"`
}

// buildHistory keeps the complete source-ordered trial while constructing only
// causal model inputs. A score is attached to the middle row of a legal
// three-row after_relocation window; all other rows remain in the history with
// a false score and a zero target.
func buildHistory(dataset trajectory.Dataset, stimuli []trajectory.StimulusRecord) ([]historyTrial, error) {
	if len(dataset.Rows) == 0 {
		return nil, fmt.Errorf("realnavmemory: dataset has no rows")
	}
	if len(stimuli) != len(dataset.Rows) {
		return nil, fmt.Errorf("realnavmemory: stimulus length %d does not match dataset rows %d", len(stimuli), len(dataset.Rows))
	}

	type trialRows struct {
		id        string
		condition string
		rows      []trajectory.Point
		stimuli   []trajectory.StimulusRecord
		previousT float64
	}
	trialsByID := make(map[string]*trialRows)
	order := make([]string, 0)
	activeID := ""

	for i, row := range dataset.Rows {
		stimulus := stimuli[i]
		if row.TrialID == "" {
			return nil, fmt.Errorf("realnavmemory: row %d has empty trial id", i)
		}
		if row.Condition != "rewarded" && row.Condition != "non-rewarded" {
			return nil, fmt.Errorf("realnavmemory: row %d trial %q has unknown condition %q", i, row.TrialID, row.Condition)
		}
		if row.Segment == "" {
			return nil, fmt.Errorf("realnavmemory: row %d trial %q has empty segment", i, row.TrialID)
		}
		if !historyFinite(row.T) || row.T < 0 {
			return nil, fmt.Errorf("realnavmemory: row %d trial %q time %v must be finite and non-negative", i, row.TrialID, row.T)
		}
		if !historyFinite(row.XCM) || !historyFinite(row.YCM) {
			return nil, fmt.Errorf("realnavmemory: row %d trial %q position must be finite", i, row.TrialID)
		}
		if stimulus.TrialID != row.TrialID {
			return nil, fmt.Errorf("realnavmemory: row %d stimulus trial %q does not match point trial %q", i, stimulus.TrialID, row.TrialID)
		}
		if !historyFinite(stimulus.T) || stimulus.T != row.T {
			return nil, fmt.Errorf("realnavmemory: row %d stimulus time %v does not match point time %v", i, stimulus.T, row.T)
		}
		if !historyFinite(stimulus.LED) || stimulus.LED < 0 {
			return nil, fmt.Errorf("realnavmemory: row %d trial %q LED %v must be finite and non-negative", i, row.TrialID, stimulus.LED)
		}
		expectedDelivered := row.Condition == "rewarded" && stimulus.LED > 0
		expectedScheduledOnly := row.Condition == "non-rewarded" && stimulus.LED > 0
		if stimulus.Delivered != expectedDelivered {
			return nil, fmt.Errorf("realnavmemory: row %d trial %q delivered flag is inconsistent with condition and LED", i, row.TrialID)
		}
		if stimulus.ScheduledOnly != expectedScheduledOnly {
			return nil, fmt.Errorf("realnavmemory: row %d trial %q scheduled-only flag is inconsistent with condition and LED", i, row.TrialID)
		}
		if stimulus.Delivered && stimulus.ScheduledOnly {
			return nil, fmt.Errorf("realnavmemory: row %d trial %q stimulus flags overlap", i, row.TrialID)
		}

		if row.TrialID != activeID {
			if _, exists := trialsByID[row.TrialID]; exists {
				return nil, fmt.Errorf("realnavmemory: trial %q is interleaved instead of contiguous", row.TrialID)
			}
			activeID = row.TrialID
			trialsByID[activeID] = &trialRows{id: activeID, condition: row.Condition}
			order = append(order, activeID)
		}
		trial := trialsByID[activeID]
		if row.Condition != trial.condition {
			return nil, fmt.Errorf("realnavmemory: trial %q changes condition from %q to %q", row.TrialID, trial.condition, row.Condition)
		}
		if len(trial.rows) > 0 {
			if row.T <= trial.previousT {
				return nil, fmt.Errorf("realnavmemory: trial %q time %v is not after %v", row.TrialID, row.T, trial.previousT)
			}
		}
		if len(trial.rows) >= historyMaxSteps {
			return nil, fmt.Errorf("realnavmemory: trial %q has more than %d rows", row.TrialID, historyMaxSteps)
		}
		trial.rows = append(trial.rows, row)
		trial.stimuli = append(trial.stimuli, stimulus)
		trial.previousT = row.T
	}

	result := make([]historyTrial, 0, len(order))
	for _, id := range order {
		trial := trialsByID[id]
		steps := make([]historyStep, len(trial.rows))
		for i, row := range trial.rows {
			step := historyStep{
				Input: [6]float64{
					row.XCM / historyPoseScale,
					row.YCM / historyPoseScale,
					0,
					0,
					0,
					historyBoolFloat(trial.stimuli[i].Delivered),
				},
				T: row.T,
			}
			if i > 0 {
				previous := trial.rows[i-1]
				dt := row.T - previous.T
				step.Input[4] = dt
				dx, dy := row.XCM-previous.XCM, row.YCM-previous.YCM
				if row.Segment == previous.Segment && dt <= historyMaxDeltaT && math.Hypot(dx, dy) <= historyMaxStepDistance {
					step.Input[2] = dx / historyDisplacement
					step.Input[3] = dy / historyDisplacement
				}
			}
			if !historyFinite(step.Input[0]) || !historyFinite(step.Input[1]) || !historyFinite(step.Input[2]) || !historyFinite(step.Input[3]) || !historyFinite(step.Input[4]) {
				return nil, fmt.Errorf("realnavmemory: trial %q row %d produced a non-finite input", id, i)
			}
			steps[i] = step
		}

		rolloutIndex := -1
		for i := 1; i+1 < len(trial.rows); i++ {
			previous, current, next := trial.rows[i-1], trial.rows[i], trial.rows[i+1]
			if previous.Segment != "after_relocation" || current.Segment != "after_relocation" || next.Segment != "after_relocation" {
				continue
			}
			previousDT := current.T - previous.T
			nextDT := next.T - current.T
			if previousDT <= 0 || nextDT <= 0 || previousDT > historyMaxDeltaT || nextDT > historyMaxDeltaT {
				continue
			}
			previousDX, previousDY := current.XCM-previous.XCM, current.YCM-previous.YCM
			nextDX, nextDY := next.XCM-current.XCM, next.YCM-current.YCM
			if !historyFinite(previousDX) || !historyFinite(previousDY) || !historyFinite(nextDX) || !historyFinite(nextDY) ||
				math.Hypot(previousDX, previousDY) > historyMaxStepDistance || math.Hypot(nextDX, nextDY) > historyMaxStepDistance {
				continue
			}
			steps[i].Scored = true
			steps[i].Target = [2]float64{nextDX, nextDY}
			if !historyFinite(nextDX) || !historyFinite(nextDY) {
				return nil, fmt.Errorf("realnavmemory: trial %q row %d produced a non-finite target", id, i)
			}
			if rolloutIndex < 0 {
				rolloutIndex = i
			}
		}
		if rolloutIndex < 0 {
			return nil, fmt.Errorf("realnavmemory: trial %q has no legal after_relocation history window", id)
		}
		result = append(result, historyTrial{ID: id, Condition: trial.condition, Steps: steps, RolloutIndex: rolloutIndex})
	}
	return result, nil
}

func importHistory(ctx context.Context, path string) ([]historyTrial, error) {
	if ctx == nil {
		return nil, fmt.Errorf("realnavmemory: nil context")
	}
	dataset, stimuli, err := trajectory.ReadWithStimulusHistory(ctx, path, realSource(), trajectory.DefaultLimits())
	if err != nil {
		return nil, fmt.Errorf("realnavmemory: import history: %w", err)
	}
	trials, err := buildHistory(dataset, stimuli)
	if err != nil {
		return nil, fmt.Errorf("realnavmemory: build history: %w", err)
	}
	return trials, nil
}

func realSource() trajectory.Source {
	return trajectory.Source{
		ID:           "titova-2023-displacement-trajectories",
		URL:          "https://github.com/strawlab/titova_et_al_displacement_supplemental/raw/" + realPinnedCommit + "/all_ds_t01_d2_cm_no2.csv.gz",
		DOI:          realDOI,
		PinnedCommit: realPinnedCommit,
		SHA256:       realSourceSHA256,
		License: trajectory.License{
			Holder: "Titova et al.",
			Terms:  "CC0 1.0",
			Source: "https://datadryad.org/help/guides/reuse",
		},
	}
}

func historyFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func historyBoolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
