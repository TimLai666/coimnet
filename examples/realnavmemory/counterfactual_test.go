package main

import (
	"bytes"
	"context"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/nav2d/trajectory"
)

func TestCounterfactualOnlyErasesPastVisibleStimulus(t *testing.T) {
	trial := testHistoryTrial(64)
	trial.RolloutIndex = 40
	for i := range trial.Steps {
		trial.Steps[i].Input[5] = float64(i % 2)
	}
	originalTrial := trial
	originalTrial.Steps = append([]historyStep(nil), trial.Steps...)
	for _, control := range memoryControls {
		original, erased, changes, err := counterfactualInputs(trial, 24, control, 123)
		if err != nil {
			t.Fatal(err)
		}
		expected, err := trialInputs(trial, control, 123)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(original, expected) {
			t.Fatal("reference input differs from legacy transformation")
		}
		count := 0
		for i := range original {
			for j := range original[i] {
				if j == 5 && i < 24 {
					if erased[i][j] != 0 {
						t.Fatal("past stimulus not erased")
					}
					if original[i][j] != 0 {
						count++
					}
				} else if original[i][j] != erased[i][j] {
					t.Fatalf("changed non-intervened value [%d][%d]", i, j)
				}
			}
		}
		if count != changes {
			t.Fatalf("changes %d != %d", changes, count)
		}
		erased[0][0]++
		if original[0][0] != expected[0][0] {
			t.Fatal("paired rows alias")
		}
	}
	if !reflect.DeepEqual(trial, originalTrial) {
		t.Fatal("source history mutated")
	}
	for _, cutoff := range []int{-1, 0, 41, 65} {
		if _, _, _, err := counterfactualInputs(trial, cutoff, "delivered", 123); err == nil {
			t.Fatalf("accepted cutoff %d", cutoff)
		}
	}
}

func counterfactualFixture(t *testing.T) (learning.TrainingSnapshot, historyTrial) {
	t.Helper()
	config, parameters, err := newModel(123)
	if err != nil {
		t.Fatal(err)
	}
	for i := range parameters.Core.Weights {
		parameters.Core.Weights[i] = 0
	}
	for i := range parameters.Encoder {
		parameters.Encoder[i] = 0
	}
	for i := range parameters.Readout {
		parameters.Readout[i] = 0
	}
	parameters.Encoder[5*8] = 1
	parameters.Readout[0] = 1
	parameters.Core.LogTau[0] = math.Log(4)
	trial := testHistoryTrial(64)
	trial.RolloutIndex = 32
	for i := range trial.Steps {
		trial.Steps[i].Input = [6]float64{.1, .1, 0, 0, .1, 0}
		trial.Steps[i].Scored = i >= 32
	}
	trial.Steps[1].Input[5] = 1
	return learning.TrainingSnapshot{Config: config, Parameters: parameters, Options: trainingOptions()}, trial
}

func TestCounterfactualHistoryAffectsLaterOutputAndFreezesModel(t *testing.T) {
	snapshot, trial := counterfactualFixture(t)
	before := memoryJSONHash(snapshot)
	target := memoryRolloutTarget{TrialID: trial.ID, XCM: 20, YCM: 20, EvaluationOnly: true}
	result, err := compareMemoryTrial(context.Background(), snapshot, trial, 24, target, "delivered", 123)
	if err != nil {
		t.Fatal(err)
	}
	if result.Recorded.ChangedRows == 0 || result.Rollout.MaxActionDeltaCM == 0 {
		t.Fatal("early stimulus lost before late output")
	}
	expected := math.Tanh((1 - math.Exp(-.25)) * math.Exp(-.25*31))
	if math.Abs(result.Rollout.MaxActionDeltaCM-expected) > 1e-6 {
		t.Fatalf("first action difference %g, independent recurrence %g", result.Rollout.MaxActionDeltaCM, expected)
	}
	if memoryJSONHash(snapshot) != before || !result.Frozen {
		t.Fatal("snapshot mutated")
	}
	zero, err := compareMemoryTrial(context.Background(), snapshot, trial, 24, target, "no_stimulus", 123)
	if err != nil {
		t.Fatal(err)
	}
	if zero.Recorded.ChangedRows != 0 || zero.Rollout.MaxPositionSeparationCM != 0 || zero.Rollout.MaxActionDeltaCM != 0 {
		t.Fatal("no-stimulus control changed")
	}
	for i := range snapshot.Parameters.Readout {
		snapshot.Parameters.Readout[i] = 0
	}
	zero, err = compareMemoryTrial(context.Background(), snapshot, trial, 24, target, "delivered", 123)
	if err != nil {
		t.Fatal(err)
	}
	if zero.Recorded.ChangedRows != 0 || zero.Rollout.MaxPositionSeparationCM != 0 {
		t.Fatal("zero readout control changed")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := compareMemoryTrial(canceled, snapshot, trial, 24, target, "delivered", 123); err == nil {
		t.Fatal("cancellation accepted")
	}
	if _, err := compareMemoryTrial(nil, snapshot, trial, 24, target, "delivered", 123); err == nil {
		t.Fatal("nil context accepted")
	}
}

func TestCounterfactualCommandDiscoveryAndInvalidInputs(t *testing.T) {
	for _, args := range [][]string{nil, {"counterfactual", "--help"}} {
		var out, diagnostics bytes.Buffer
		if err := run(context.Background(), args, &out, &diagnostics); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "counterfactual") {
			t.Fatal("counterfactual absent from help")
		}
	}
	for _, args := range [][]string{{"counterfactual"}, {"counterfactual", "positional"}, {"counterfactual", "--unknown"}} {
		if err := run(context.Background(), args, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestCounterfactualRelocationBoundaryUsesFirstSourceRow(t *testing.T) {
	trial := testHistoryTrial(64)
	trial.RolloutIndex = 40
	rows := make([]trajectory.Point, len(trial.Steps))
	for i := range rows {
		rows[i] = trajectory.Point{TrialID: trial.ID, Segment: "baseline"}
		if i >= 24 {
			rows[i].Segment = "relocation"
		}
		if i >= 38 {
			rows[i].Segment = "after_relocation"
		}
	}
	cutoffs, err := memoryRelocationCutoffs(rows, []historyTrial{trial})
	if err != nil || cutoffs[trial.ID] != 24 {
		t.Fatalf("source cutoff %v: %v", cutoffs, err)
	}
	for _, name := range []string{"missing", "zero", "after_start", "missing_rows", "extra_trial"} {
		t.Run(name, func(t *testing.T) {
			bad := append([]trajectory.Point(nil), rows...)
			switch name {
			case "missing":
				for i := range bad {
					bad[i].Segment = "baseline"
				}
			case "zero":
				bad[0].Segment = "relocation"
			case "after_start":
				for i := range bad {
					bad[i].Segment = "baseline"
				}
				bad[41].Segment = "relocation"
			case "missing_rows":
				bad = bad[:len(bad)-1]
			case "extra_trial":
				bad = append(bad, trajectory.Point{TrialID: "extra", Segment: "relocation"})
			}
			if _, err := memoryRelocationCutoffs(bad, []historyTrial{trial}); err == nil {
				t.Fatal("accepted invalid source boundary")
			}
		})
	}
}

func TestCounterfactualShuffledContinuationRetainsOriginalRawBlocks(t *testing.T) {
	snapshot, trial := counterfactualFixture(t)
	trial.RolloutIndex = 35
	for i := range trial.Steps {
		trial.Steps[i].Input[5] = 0
		trial.Steps[i].Scored = i >= 35
	}
	for i := 16; i < 24; i++ {
		trial.Steps[i].Input[5] = 1
	}
	target := memoryRolloutTarget{TrialID: trial.ID, XCM: 20, YCM: 20, EvaluationOnly: true}
	traces := make([][]memoryRolloutStep, 2)
	for side, cutoff := range []int{0, 24} {
		individual, err := learning.NewIndividual(snapshot.Config, snapshot.Parameters, snapshot.Options, make([]float64, snapshot.Config.Dynamics.Nodes))
		if err != nil {
			t.Fatal(err)
		}
		result, err := memoryModelTrialErase(context.Background(), individual, trial, target, "shuffled_stimulus", 123, cutoff)
		if err != nil {
			t.Fatal(err)
		}
		traces[side] = result.Trace
	}
	laterPositive := 0
	for i, original := range traces[0] {
		if original.Input[5] != traces[1][i].Input[5] {
			t.Fatalf("later stimulus changed at decision %d", i)
		}
		if i > 0 && original.Input[5] > 0 {
			laterPositive++
		}
	}
	if laterPositive == 0 {
		t.Fatal("fixture did not exercise delayed positive continuation")
	}
	if _, err := compareMemoryTrial(context.Background(), snapshot, trial, 24, target, "shuffled_stimulus", 123); err != nil {
		t.Fatal(err)
	}
	rawErased := trial
	rawErased.Steps = append([]historyStep(nil), trial.Steps...)
	for i := 0; i < 24; i++ {
		rawErased.Steps[i].Input[5] = 0
	}
	original, err := trialInputs(trial, "shuffled_stimulus", 123)
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := trialInputs(rawErased, "shuffled_stimulus", 123)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(original[24:], wrong[24:]) {
		t.Fatal("fixture cannot detect erase-before-transform error")
	}
}
