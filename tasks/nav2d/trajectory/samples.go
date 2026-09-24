package trajectory

import (
	"fmt"
	"math"
	"strings"
)

// SampleConfig controls the causal three-point window. A candidate is kept
// only when both adjacent intervals are positive and no larger than MaxDeltaT,
// and both adjacent displacements are no larger than MaxStepDistanceCM. Rows
// explicitly marked as the source's "relocation" segment are also excluded.
// These are data-cleaning rules, not inferred action labels.
type SampleConfig struct {
	MaxDeltaT         float64
	MaxStepDistanceCM float64
}

// DefaultSampleConfig uses a half-second maximum interval and a 5 cm maximum
// one-step displacement. These are explicit data-cleaning rules, not inferred
// action labels or a claim about the animal's movement mechanism.
func DefaultSampleConfig() SampleConfig {
	return SampleConfig{MaxDeltaT: 0.5, MaxStepDistanceCM: 5}
}

// Input contains only information available at the current time t: the current
// position and time, plus the previous displacement and elapsed time. It has no
// condition, segment, reward, fictive target, trial ID or future position.
type Input struct {
	CurrentXCM   float64 `json:"current_x_cm"`
	CurrentYCM   float64 `json:"current_y_cm"`
	CurrentT     float64 `json:"current_t"`
	PreviousDXCM float64 `json:"previous_dx_cm"`
	PreviousDYCM float64 `json:"previous_dy_cm"`
	PreviousDT   float64 `json:"previous_dt"`
}

// Target is the t+1 supervision label. No Target field is copied to Input.
type Target struct {
	NextDXCM float64 `json:"next_dx_cm"`
	NextDYCM float64 `json:"next_dy_cm"`
	NextDT   float64 `json:"next_dt"`
}

// Sample is one causal next-displacement regression example. TrialID is
// evaluation metadata and never part of Input.
type Sample struct {
	TrialID string `json:"trial_id"`
	Input   Input  `json:"input"`
	Target  Target `json:"target"`
}

// SampleStats records why candidate windows were discarded. Each skipped
// candidate increments one primary reason counter; SkippedRelocation is also
// the aggregate of its explicit-segment and large-step subcounters.
type SampleStats struct {
	Trials                  int `json:"trials"`
	CandidateWindows        int `json:"candidate_windows"`
	Samples                 int `json:"samples"`
	SkippedBoundary         int `json:"skipped_boundary"`
	SkippedLargeGap         int `json:"skipped_large_gap"`
	SkippedRelocation       int `json:"skipped_relocation"`
	SkippedManualRelocation int `json:"skipped_manual_relocation"`
	SkippedLargeStep        int `json:"skipped_large_step"`
}

// SampleSet carries the versioned rule and the audit counters for causal
// sample construction.
type SampleSet struct {
	RuleVersion string      `json:"rule_version"`
	Samples     []Sample    `json:"samples"`
	Stats       SampleStats `json:"stats"`
}

// BuildSamples forms causal three-point windows from source-ordered rows. For
// rows (previous, current, next), Input is computed from previous and current,
// while Target is computed only from current and next. Windows never cross a
// TrialID, condition or segment boundary.
func BuildSamples(dataset Dataset, config SampleConfig) (SampleSet, error) {
	if !isFinite(config.MaxDeltaT) || config.MaxDeltaT <= 0 {
		return SampleSet{}, fmt.Errorf("trajectory: max delta t %v must be positive and finite", config.MaxDeltaT)
	}
	if !isFinite(config.MaxStepDistanceCM) || config.MaxStepDistanceCM <= 0 {
		return SampleSet{}, fmt.Errorf("trajectory: max step distance %v must be positive and finite", config.MaxStepDistanceCM)
	}
	if err := validateDatasetRows(dataset.Rows); err != nil {
		return SampleSet{}, err
	}

	stats := SampleStats{}
	seenTrials := make(map[string]struct{})
	for _, row := range dataset.Rows {
		seenTrials[row.TrialID] = struct{}{}
	}
	stats.Trials = len(seenTrials)
	if len(dataset.Rows) < 3 {
		return SampleSet{RuleVersion: SampleRuleVersion, Samples: nil, Stats: stats}, nil
	}

	set := SampleSet{
		RuleVersion: SampleRuleVersion,
		Samples:     make([]Sample, 0, len(dataset.Rows)-2),
		Stats:       stats,
	}
	for i := 1; i < len(dataset.Rows)-1; i++ {
		set.Stats.CandidateWindows++
		previous, current, next := dataset.Rows[i-1], dataset.Rows[i], dataset.Rows[i+1]
		if !sameSegment(previous, current) || !sameSegment(current, next) {
			set.Stats.SkippedBoundary++
			continue
		}
		if manualRelocation(previous) || manualRelocation(current) || manualRelocation(next) {
			set.Stats.SkippedManualRelocation++
			set.Stats.SkippedRelocation++
			continue
		}
		previousDT := current.T - previous.T
		nextDT := next.T - current.T
		if previousDT <= 0 || nextDT <= 0 || previousDT > config.MaxDeltaT || nextDT > config.MaxDeltaT {
			set.Stats.SkippedLargeGap++
			continue
		}
		previousDX, previousDY := current.XCM-previous.XCM, current.YCM-previous.YCM
		nextDX, nextDY := next.XCM-current.XCM, next.YCM-current.YCM
		if stepDistance(previousDX, previousDY) > config.MaxStepDistanceCM || stepDistance(nextDX, nextDY) > config.MaxStepDistanceCM {
			set.Stats.SkippedLargeStep++
			set.Stats.SkippedRelocation++
			continue
		}
		set.Samples = append(set.Samples, Sample{
			TrialID: current.TrialID,
			Input: Input{
				CurrentXCM:   current.XCM,
				CurrentYCM:   current.YCM,
				CurrentT:     current.T,
				PreviousDXCM: previousDX,
				PreviousDYCM: previousDY,
				PreviousDT:   previousDT,
			},
			Target: Target{NextDXCM: nextDX, NextDYCM: nextDY, NextDT: nextDT},
		})
	}
	set.Stats.Samples = len(set.Samples)
	return set, nil
}

func sameSegment(a, b Point) bool {
	return a.TrialID == b.TrialID && a.FName == b.FName && a.Fly == b.Fly &&
		a.Condition == b.Condition && a.Segment == b.Segment
}

func stepDistance(dx, dy float64) float64 {
	return math.Hypot(dx, dy)
}

func manualRelocation(point Point) bool {
	return strings.EqualFold(strings.TrimSpace(point.Segment), "relocation")
}
