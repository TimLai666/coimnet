package experiment

import (
	"fmt"
	"math"
)

// Modes of an adaptive evaluation.
const (
	EvaluationModeFixed    = "fixed"
	EvaluationModeAdaptive = "adaptive"
)

// EvaluationItem is one pre-declared item of a partition.
type EvaluationItem struct {
	ID              string      `json:"id"`
	Input           [][]float64 `json:"input"`
	Target          []float64   `json:"target"`
	AllowedFeedback []string    `json:"allowed_feedback,omitempty"`
}

// ResetPolicy says which individual state is reset before every item.
type ResetPolicy struct {
	NeuralAtItemStart   bool `json:"neural_at_item_start"`
	PlasticAtItemStart  bool `json:"plastic_at_item_start"`
	ChemicalAtItemStart bool `json:"chemical_at_item_start"`
}

// AdaptiveEvaluation declares an evaluation in advance: the two partitions,
// which feedback kinds exist at all, the reset policy and the mode.
type AdaptiveEvaluation struct {
	Mode              string           `json:"mode"`
	Adaptation        []EvaluationItem `json:"adaptation,omitempty"`
	Scoring           []EvaluationItem `json:"scoring"`
	FeedbackAvailable []string         `json:"feedback_available,omitempty"`
	Reset             ResetPolicy      `json:"reset"`
	Seed              uint64           `json:"seed"`
}

// Validate rejects a declaration that could contaminate the scoring partition.
func (e AdaptiveEvaluation) Validate() error {
	if e.Mode != EvaluationModeFixed && e.Mode != EvaluationModeAdaptive {
		return fmt.Errorf("invalid mode %q", e.Mode)
	}
	if len(e.Scoring) == 0 {
		return fmt.Errorf("scoring: at least one item required")
	}
	if e.Mode == EvaluationModeFixed && len(e.Adaptation) != 0 {
		return fmt.Errorf("mode %q: adaptation must be empty", EvaluationModeFixed)
	}
	if err := checkIDs(e.Adaptation, e.Scoring); err != nil {
		return err
	}
	if err := checkFeedbackAvailable(e.FeedbackAvailable); err != nil {
		return err
	}
	for i, item := range e.Adaptation {
		if err := validateItem(item, "adaptation", i, e.FeedbackAvailable); err != nil {
			return err
		}
	}
	for i, item := range e.Scoring {
		if err := validateItem(item, "scoring", i, e.FeedbackAvailable); err != nil {
			return err
		}
		if len(item.AllowedFeedback) != 0 {
			return fmt.Errorf("scoring item %q: allowed_feedback must be empty", item.ID)
		}
	}
	return nil
}

func checkIDs(adaptation, scoring []EvaluationItem) error {
	seen := make(map[string]bool, len(adaptation)+len(scoring))
	check := func(partition string, i int, item EvaluationItem) error {
		if item.ID == "" {
			return fmt.Errorf("%s item %d: id must not be empty", partition, i)
		}
		if seen[item.ID] {
			return fmt.Errorf("item %q: duplicate id", item.ID)
		}
		seen[item.ID] = true
		return nil
	}
	for i, item := range adaptation {
		if err := check("adaptation", i, item); err != nil {
			return err
		}
	}
	for i, item := range scoring {
		if err := check("scoring", i, item); err != nil {
			return err
		}
	}
	return nil
}

func checkFeedbackAvailable(kinds []string) error {
	seen := make(map[string]bool, len(kinds))
	for _, kind := range kinds {
		if kind == "" {
			return fmt.Errorf("feedback_available: kind must not be empty")
		}
		if seen[kind] {
			return fmt.Errorf("feedback_available: duplicate kind %q", kind)
		}
		seen[kind] = true
	}
	return nil
}

func validateItem(item EvaluationItem, partition string, index int, available []string) error {
	if len(item.Input) == 0 {
		return fmt.Errorf("%s item %q: input must have at least one row", partition, item.ID)
	}
	width := len(item.Input[0])
	for row, values := range item.Input {
		if len(values) != width {
			return fmt.Errorf("%s item %q: input row %d has length %d, want %d", partition, item.ID, row, len(values), width)
		}
		for _, v := range values {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("%s item %q: input contains non-finite value", partition, item.ID)
			}
		}
	}
	if len(item.Target) == 0 {
		return fmt.Errorf("%s item %q: target must not be empty", partition, item.ID)
	}
	for _, v := range item.Target {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("%s item %q: target contains non-finite value", partition, item.ID)
		}
	}
	allowed := make(map[string]bool, len(available))
	for _, kind := range available {
		allowed[kind] = true
	}
	for _, kind := range item.AllowedFeedback {
		if !allowed[kind] {
			return fmt.Errorf("%s item %q: allowed_feedback %q is not in feedback_available", partition, item.ID, kind)
		}
	}
	return nil
}
