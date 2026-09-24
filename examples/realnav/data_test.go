package main

import (
	"math"
	"testing"

	"github.com/TimLai666/coimnet/tasks/nav2d/trajectory"
)

func TestConvertAdapterSamplesKeepsCausalFeaturesAndTargetSeparate(t *testing.T) {
	dataset := trajectory.Dataset{Rows: []trajectory.Point{
		{TrialID: "trial-a", Condition: "rewarded", Segment: "after_relocation", T: 0, XCM: 0, YCM: 0},
		{TrialID: "trial-a", Condition: "rewarded", Segment: "after_relocation", T: 0.2, XCM: 1, YCM: 2},
		{TrialID: "trial-a", Condition: "rewarded", Segment: "after_relocation", T: 0.5, XCM: 3, YCM: 3},
		{TrialID: "trial-b", Condition: "non-rewarded", Segment: "after_relocation", T: 0, XCM: 10, YCM: 20},
		{TrialID: "trial-b", Condition: "non-rewarded", Segment: "after_relocation", T: 0.1, XCM: 11, YCM: 20},
		{TrialID: "trial-b", Condition: "non-rewarded", Segment: "after_relocation", T: 0.2, XCM: 12, YCM: 21},
	}}
	set, err := trajectory.BuildSamples(dataset, trajectory.SampleConfig{MaxDeltaT: 1, MaxStepDistanceCM: 10})
	if err != nil {
		t.Fatal(err)
	}
	conditions, err := trialConditions(dataset)
	if err != nil {
		t.Fatal(err)
	}
	samples, err := convertSamples(set, conditions)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 {
		t.Fatalf("samples = %d, want 2", len(samples))
	}
	got := samples[0]
	if got.TrialID != "trial-a" || got.Condition != "rewarded" || math.Abs(got.Timestamp-.2) > 1e-12 {
		t.Fatalf("sample identity = %+v", got)
	}
	if got.Target != [2]float64{2, 1} {
		t.Fatalf("target = %v, want [2 1]", got.Target)
	}
	if got.Input[0] != 1/poseScale || got.Input[1] != 2/poseScale || got.Input[2] != 1/displacementScale || got.Input[3] != 2/displacementScale || got.Input[4] != .2/timeScale {
		t.Fatalf("normalized causal input = %v", got.Input)
	}
	if got.Input[0] == got.Target[0] || got.Input[1] == got.Target[1] {
		t.Fatal("target appears to be copied into the model input")
	}
}

func TestConvertSamplesRejectsMissingTrialConditionAndNonFiniteValues(t *testing.T) {
	base := trajectory.SampleSet{Samples: []trajectory.Sample{{
		TrialID: "trial-a",
		Input:   trajectory.Input{CurrentT: .2, CurrentXCM: 1, CurrentYCM: 2, PreviousDXCM: 1, PreviousDYCM: 2, PreviousDT: .2},
		Target:  trajectory.Target{NextDXCM: 2, NextDYCM: 1, NextDT: .3},
	}}}
	if _, err := convertSamples(base, map[string]string{}); err == nil {
		t.Fatal("accepted sample without condition metadata")
	}
	base.Samples[0].Input.CurrentXCM = math.NaN()
	if _, err := convertSamples(base, map[string]string{"trial-a": "rewarded"}); err == nil {
		t.Fatal("accepted non-finite input")
	}
}

func TestRealSourceRecordsPinnedFileAndDryadCC0Terms(t *testing.T) {
	source := realSource()
	wantURL := "https://github.com/strawlab/titova_et_al_displacement_supplemental/raw/" + realPinnedCommit + "/all_ds_t01_d2_cm_no2.csv.gz"
	if source.URL != wantURL {
		t.Fatalf("source URL = %q, want pinned raw file %q", source.URL, wantURL)
	}
	if source.License.Terms != "CC0 1.0" {
		t.Fatalf("license terms = %q, want CC0 1.0", source.License.Terms)
	}
	if source.License.Source != "https://datadryad.org/help/guides/reuse" {
		t.Fatalf("license source = %q, want official Dryad reuse guide", source.License.Source)
	}
}

func TestOnlyAfterRelocationDropsOtherSegments(t *testing.T) {
	dataset := trajectory.Dataset{Schema: trajectory.SchemaVersion, Source: realSource(), Rows: []trajectory.Point{
		{TrialID: "trial-a", Condition: "rewarded", Segment: "before_relocation", T: 0, XCM: 1, YCM: 2},
		{TrialID: "trial-a", Condition: "rewarded", Segment: "after_relocation", T: 1, XCM: 3, YCM: 4},
		{TrialID: "trial-a", Condition: "rewarded", Segment: "after_relocation", T: 2, XCM: 5, YCM: 6},
	}}
	filtered := onlyAfterRelocation(dataset)
	if len(filtered.Rows) != 2 || filtered.Rows[0].Segment != "after_relocation" || filtered.Rows[1].Segment != "after_relocation" {
		t.Fatalf("filtered rows = %+v", filtered.Rows)
	}
	if filtered.Source != dataset.Source || filtered.Schema != dataset.Schema {
		t.Fatal("filter did not preserve dataset provenance")
	}
}

func TestSnapshotRejectsPreprocessingContractMismatch(t *testing.T) {
	contract := currentPreprocessingContract()
	model := savedModel{Preprocessing: contract}
	if err := validateSnapshotPreprocessing(model); err != nil {
		t.Fatalf("current preprocessing contract rejected: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*preprocessingContract)
	}{
		{name: "version", mutate: func(value *preprocessingContract) { value.Version = "future" }},
		{name: "segment", mutate: func(value *preprocessingContract) { value.Segment = "baseline" }},
		{name: "max delta", mutate: func(value *preprocessingContract) { value.MaxDeltaTSeconds += .1 }},
		{name: "max step", mutate: func(value *preprocessingContract) { value.MaxStepDistanceCM += 1 }},
		{name: "pose scale", mutate: func(value *preprocessingContract) { value.PoseScaleCM += 1 }},
		{name: "displacement scale", mutate: func(value *preprocessingContract) { value.DisplacementScaleCM += 1 }},
		{name: "time scale", mutate: func(value *preprocessingContract) { value.TimeScaleSeconds += 1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := contract
			test.mutate(&changed)
			if err := validateSnapshotPreprocessing(savedModel{Preprocessing: changed}); err == nil {
				t.Fatal("accepted mismatched preprocessing contract")
			}
		})
	}
}
