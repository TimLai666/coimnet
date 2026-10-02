package trajectory

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ReturnTarget is evaluation metadata for the virtual reward-zone center of a
// trial. It is not part of Dataset rows, samples, inputs or model targets.
type ReturnTarget struct {
	TrialID string  `json:"trial_id"`
	XCM     float64 `json:"x_cm"`
	YCM     float64 `json:"y_cm"`
}

// ReadWithReturnTargets imports the trajectory and, from after_relocation
// rows, collects one fixed virtual reward-zone center per trial. It verifies
// the same source bytes and parses the same CSV stream as Read. ReturnTarget
// metadata is returned separately and is never added to Dataset.
func ReadWithReturnTargets(ctx context.Context, path string, source Source, limits Limits) (Dataset, []ReturnTarget, error) {
	collector := &returnTargetCollector{targets: make([]ReturnTarget, 0)}
	dataset, err := read(ctx, path, source, limits, collector)
	if err != nil {
		return Dataset{}, nil, err
	}
	targets := make([]ReturnTarget, len(collector.targets))
	copy(targets, collector.targets)
	return dataset, targets, nil
}

type returnTargetIndices struct {
	x, y int
}

type returnTargetCollector struct {
	indices returnTargetIndices
	byTrial map[string]ReturnTarget
	targets []ReturnTarget
}

func validateReturnTargetHeader(header []string) (returnTargetIndices, error) {
	indices := returnTargetIndices{x: -1, y: -1}
	for i, rawName := range header {
		name := strings.TrimSpace(rawName)
		if i == 0 {
			name = strings.TrimPrefix(name, "\ufeff")
		}
		switch name {
		case "estimated_food_x_cm":
			indices.x = i
		case "estimated_food_y_cm":
			indices.y = i
		}
	}
	missing := make([]string, 0, 2)
	if indices.x < 0 {
		missing = append(missing, "estimated_food_x_cm")
	}
	if indices.y < 0 {
		missing = append(missing, "estimated_food_y_cm")
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return returnTargetIndices{}, fmt.Errorf("trajectory: required return-target columns missing: %s", strings.Join(missing, ", "))
	}
	return indices, nil
}

func (c *returnTargetCollector) collect(point Point, record []string, rowNumber int) error {
	if !strings.EqualFold(strings.TrimSpace(point.Segment), "after_relocation") {
		return nil
	}
	get := func(index int) string {
		if index < 0 || index >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[index])
	}
	parseFinite := func(name string, index int) (float64, error) {
		value, err := strconv.ParseFloat(get(index), 64)
		if err != nil {
			return 0, fmt.Errorf("trajectory: row %d parse %s: %w", rowNumber, name, err)
		}
		if !isFinite(value) {
			return 0, fmt.Errorf("trajectory: row %d %s must be finite", rowNumber, name)
		}
		return value, nil
	}
	x, err := parseFinite("estimated_food_x_cm", c.indices.x)
	if err != nil {
		return err
	}
	y, err := parseFinite("estimated_food_y_cm", c.indices.y)
	if err != nil {
		return err
	}
	target := ReturnTarget{TrialID: point.TrialID, XCM: x, YCM: y}
	if c.byTrial == nil {
		c.byTrial = make(map[string]ReturnTarget)
	}
	previous, exists := c.byTrial[point.TrialID]
	if exists {
		if previous.XCM != target.XCM || previous.YCM != target.YCM {
			return fmt.Errorf("trajectory: after_relocation return target conflict for trial %q at row %d: got (%v, %v), want (%v, %v)", point.TrialID, rowNumber, target.XCM, target.YCM, previous.XCM, previous.YCM)
		}
		return nil
	}
	c.byTrial[point.TrialID] = target
	c.targets = append(c.targets, target)
	return nil
}
