package trajectory

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// StimulusRecord preserves the recorded LED value and the two source-level
// stimulus flags for one trajectory row. It does not infer neural reward,
// arrival time or a target position.
type StimulusRecord struct {
	TrialID       string  `json:"trial_id"`
	T             float64 `json:"t"`
	LED           float64 `json:"led_1"`
	Delivered     bool    `json:"delivered"`
	ScheduledOnly bool    `json:"scheduled_only"`
}

// ReadWithStimulusHistory imports the trajectory and records one source LED
// value and its explicit condition-derived flags for every dataset row. The
// stimulus records stay separate from Dataset and are collected from the same
// verified bytes and CSV stream as Read.
func ReadWithStimulusHistory(ctx context.Context, path string, source Source, limits Limits) (Dataset, []StimulusRecord, error) {
	collector := &stimulusCollector{records: make([]StimulusRecord, 0)}
	dataset, err := read(ctx, path, source, limits, nil, collector)
	if err != nil {
		return Dataset{}, nil, err
	}
	records := make([]StimulusRecord, len(collector.records))
	copy(records, collector.records)
	return dataset, records, nil
}

type stimulusCollector struct {
	ledIndex   int
	conditions map[string]string
	records    []StimulusRecord
}

func validateStimulusHeader(header []string) (int, error) {
	for i, rawName := range header {
		name := strings.TrimSpace(rawName)
		if i == 0 {
			name = strings.TrimPrefix(name, "\ufeff")
		}
		if name == "led_1" {
			return i, nil
		}
	}
	return -1, fmt.Errorf("trajectory: required stimulus column missing: led_1")
}

func (c *stimulusCollector) collect(point Point, record []string, rowNumber int) error {
	get := func(index int) string {
		if index < 0 || index >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[index])
	}
	led, err := strconv.ParseFloat(get(c.ledIndex), 64)
	if err != nil {
		return fmt.Errorf("trajectory: row %d parse led_1: %w", rowNumber, err)
	}
	if !isFinite(led) {
		return fmt.Errorf("trajectory: row %d led_1 must be finite", rowNumber)
	}
	if led < 0 {
		return fmt.Errorf("trajectory: row %d led_1 must be non-negative", rowNumber)
	}

	if c.conditions == nil {
		c.conditions = make(map[string]string)
	}
	if previous, exists := c.conditions[point.TrialID]; exists && previous != point.Condition {
		return fmt.Errorf("trajectory: condition conflict for trial %q at row %d: got %q, want %q", point.TrialID, rowNumber, point.Condition, previous)
	}
	switch point.Condition {
	case "rewarded", "non-rewarded":
		c.conditions[point.TrialID] = point.Condition
	default:
		return fmt.Errorf("trajectory: row %d unknown condition %q: want exactly rewarded or non-rewarded", rowNumber, point.Condition)
	}

	c.records = append(c.records, StimulusRecord{
		TrialID:       point.TrialID,
		T:             point.T,
		LED:           led,
		Delivered:     point.Condition == "rewarded" && led > 0,
		ScheduledOnly: point.Condition == "non-rewarded" && led > 0,
	})
	return nil
}
