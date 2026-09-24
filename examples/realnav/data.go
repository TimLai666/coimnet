package main

import (
	"context"
	"fmt"
	"math"
	"reflect"

	"github.com/TimLai666/coimnet/tasks/nav2d/trajectory"
)

const (
	featureRuleVersion   = trajectory.SampleRuleVersion
	poseScale            = 20.0
	displacementScale    = 2.0
	timeScale            = 1.0
	realSourceSHA256     = "83e13b7057957cf41a45dc91996a4519be7066519242b6912c65b2418cb91670"
	realDOI              = "10.5061/dryad.vdncjsz0b"
	realPinnedCommit     = "0eb07940a9ffdedd97ada81f75e9d5f7bface579"
	realGitBlobSHA1      = "b87c2ca52eb3030a02b8e14b6510b358fba92c69"
	realReadmeBlobSHA1   = "9dabe23037de5279a21ebed66464275d173966c4"
	preprocessingVersion = "coimnet-realnav-preprocessing/v1"
)

type preprocessingContract struct {
	Version             string   `json:"version"`
	FeatureRule         string   `json:"feature_rule"`
	Segment             string   `json:"segment"`
	MaxDeltaTSeconds    float64  `json:"max_delta_t_seconds"`
	MaxStepDistanceCM   float64  `json:"max_step_distance_cm"`
	PoseScaleCM         float64  `json:"pose_scale_cm"`
	DisplacementScaleCM float64  `json:"displacement_scale_cm"`
	TimeScaleSeconds    float64  `json:"time_scale_seconds"`
	InputFeatures       []string `json:"input_features"`
	TargetFeatures      []string `json:"target_features"`
	ForbiddenFeatures   []string `json:"forbidden_features"`
}

func currentPreprocessingContract() preprocessingContract {
	config := trajectory.DefaultSampleConfig()
	return preprocessingContract{
		Version:             preprocessingVersion,
		FeatureRule:         featureRuleVersion,
		Segment:             "after_relocation",
		MaxDeltaTSeconds:    config.MaxDeltaT,
		MaxStepDistanceCM:   config.MaxStepDistanceCM,
		PoseScaleCM:         poseScale,
		DisplacementScaleCM: displacementScale,
		TimeScaleSeconds:    timeScale,
		InputFeatures:       []string{"current_x_cm", "current_y_cm", "previous_dx_cm", "previous_dy_cm", "previous_dt_seconds"},
		TargetFeatures:      []string{"next_dx_cm", "next_dy_cm"},
		ForbiddenFeatures:   []string{"reward", "fictive_reward", "condition", "segment", "future_pose", "future_target"},
	}
}

func (contract preprocessingContract) sampleConfig() trajectory.SampleConfig {
	return trajectory.SampleConfig{MaxDeltaT: contract.MaxDeltaTSeconds, MaxStepDistanceCM: contract.MaxStepDistanceCM}
}

func validateSnapshotPreprocessing(model savedModel) error {
	want := currentPreprocessingContract()
	if reflect.DeepEqual(model.Preprocessing, want) {
		return nil
	}
	return fmt.Errorf("realnav: snapshot preprocessing contract mismatch: got=%+v want=%+v", model.Preprocessing, want)
}

func sampleConfig() trajectory.SampleConfig {
	return currentPreprocessingContract().sampleConfig()
}

// Inputs are normalized by fixed, reportable units before they enter the
// model. Targets remain centimetres so reported MSE is cm^2.
func sampleInput(input trajectory.Input) []float64 {
	return sampleInputWithContract(input, currentPreprocessingContract())
}

func sampleInputWithContract(input trajectory.Input, contract preprocessingContract) []float64 {
	return []float64{
		input.CurrentXCM / contract.PoseScaleCM,
		input.CurrentYCM / contract.PoseScaleCM,
		input.PreviousDXCM / contract.DisplacementScaleCM,
		input.PreviousDYCM / contract.DisplacementScaleCM,
		input.PreviousDT / contract.TimeScaleSeconds,
	}
}

func sampleTarget(target trajectory.Target) [2]float64 {
	return [2]float64{target.NextDXCM, target.NextDYCM}
}

func convertSamples(set trajectory.SampleSet, conditions map[string]string) ([]causalSample, error) {
	if len(set.Samples) == 0 {
		return nil, fmt.Errorf("realnav: adapter returned no causal samples")
	}
	contract := currentPreprocessingContract()
	converted := make([]causalSample, 0, len(set.Samples))
	for i, sample := range set.Samples {
		if sample.TrialID == "" {
			return nil, fmt.Errorf("realnav: adapter sample %d has empty trial id", i)
		}
		input := sampleInputWithContract(sample.Input, contract)
		for j, value := range input {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, fmt.Errorf("realnav: adapter sample %d input[%d] is non-finite", i, j)
			}
		}
		target := sampleTarget(sample.Target)
		if math.IsNaN(target[0]) || math.IsInf(target[0], 0) || math.IsNaN(target[1]) || math.IsInf(target[1], 0) {
			return nil, fmt.Errorf("realnav: adapter sample %d target is non-finite", i)
		}
		condition := conditions[sample.TrialID]
		if condition == "" {
			return nil, fmt.Errorf("realnav: adapter sample %d trial %q has no condition", i, sample.TrialID)
		}
		converted = append(converted, causalSample{
			TrialID:      sample.TrialID,
			Condition:    condition,
			Timestamp:    sample.Input.CurrentT,
			DeltaSeconds: sample.Input.PreviousDT,
			Input:        input,
			Target:       target,
		})
	}
	return converted, nil
}

func trialConditions(dataset trajectory.Dataset) (map[string]string, error) {
	conditions := make(map[string]string)
	for _, row := range dataset.Rows {
		if row.TrialID == "" || row.Condition == "" {
			return nil, fmt.Errorf("realnav: adapter returned row with missing trial or condition")
		}
		if previous, ok := conditions[row.TrialID]; ok && previous != row.Condition {
			return nil, fmt.Errorf("realnav: trial %q has conflicting conditions %q and %q", row.TrialID, previous, row.Condition)
		}
		conditions[row.TrialID] = row.Condition
	}
	return conditions, nil
}

func onlyAfterRelocation(dataset trajectory.Dataset) trajectory.Dataset {
	return onlySegment(dataset, currentPreprocessingContract().Segment)
}

func onlySegment(dataset trajectory.Dataset, segment string) trajectory.Dataset {
	rows := make([]trajectory.Point, 0, len(dataset.Rows))
	for _, row := range dataset.Rows {
		if row.Segment == segment {
			rows = append(rows, row)
		}
	}
	return trajectory.Dataset{Schema: dataset.Schema, Source: dataset.Source, Rows: rows}
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

func importTrajectory(ctx context.Context, path string) (trajectory.Dataset, error) {
	if ctx == nil {
		return trajectory.Dataset{}, fmt.Errorf("realnav: nil context")
	}
	return trajectory.Read(ctx, path, realSource(), trajectory.DefaultLimits())
}
