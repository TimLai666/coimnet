package main

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/tasks/nav2d/trajectory"
)

func TestBuildHistoryKeepsEveryRowAndUsesOnlyPastCausalValues(t *testing.T) {
	dataset, stimuli := historyFixture()
	trials, err := buildHistory(dataset, stimuli)
	if err != nil {
		t.Fatalf("buildHistory: %v", err)
	}
	if len(trials) != 1 {
		t.Fatalf("trials = %d, want 1", len(trials))
	}
	trial := trials[0]
	if trial.ID != "trial-a" || trial.Condition != "rewarded" {
		t.Fatalf("trial metadata = %+v", trial)
	}
	if len(trial.Steps) != len(dataset.Rows) {
		t.Fatalf("steps = %d, want every source row %d", len(trial.Steps), len(dataset.Rows))
	}
	if trial.RolloutIndex != 3 {
		t.Fatalf("rollout index = %d, want first scored row 3", trial.RolloutIndex)
	}

	wantInputs := [][6]float64{
		{0, 0, 0, 0, 0, 0},
		{10.0 / 30, 10.0 / 30, 0, 0, 0.1, 1},
		{1.0 / 30, 1.0 / 30, 0, 0, 0.1, 0}, // segment boundary zeros movement, keeps dt
		{2.0 / 30, 1.0 / 30, 0.5, 0, 0.1, 1},
		{3.0 / 30, 2.0 / 30, 0.5, 0.5, 0.1, 0},
		{4.0 / 30, 2.0 / 30, 0, 0, 0.1, 0}, // segment boundary
		{5.0 / 30, 2.0 / 30, 0, 0, 0.1, 0},
	}
	for i, want := range wantInputs {
		for j := range want {
			if math.Abs(trial.Steps[i].Input[j]-want[j]) > 1e-12 {
				t.Errorf("step %d input[%d] = %v, want %v", i, j, trial.Steps[i].Input[j], want[j])
			}
		}
		if trial.Steps[i].T != dataset.Rows[i].T {
			t.Errorf("step %d T = %v, want %v", i, trial.Steps[i].T, dataset.Rows[i].T)
		}
	}
	for i := range trial.Steps {
		if i == 3 {
			if !trial.Steps[i].Scored || trial.Steps[i].Target != [2]float64{1, 1} {
				t.Errorf("step %d score/target = %v/%v, want true/[1 1]", i, trial.Steps[i].Scored, trial.Steps[i].Target)
			}
			continue
		}
		if trial.Steps[i].Scored || trial.Steps[i].Target != [2]float64{} {
			t.Errorf("step %d unexpectedly scored: %v/%v", i, trial.Steps[i].Scored, trial.Steps[i].Target)
		}
	}

	mutated := cloneDataset(dataset)
	mutated.Rows[len(mutated.Rows)-1].XCM = 500
	mutated.Rows[len(mutated.Rows)-1].YCM = -400
	mutatedTrials, err := buildHistory(mutated, stimuli)
	if err != nil {
		t.Fatalf("buildHistory after future pose mutation: %v", err)
	}
	if !reflect.DeepEqual(trial.Steps[:len(trial.Steps)-1], mutatedTrials[0].Steps[:len(trial.Steps)-1]) {
		t.Fatal("future pose changed earlier model inputs or targets")
	}
}

func TestBuildHistoryRejectsMalformedRowsStimuliAndTrials(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*trajectory.Dataset, *[]trajectory.StimulusRecord)
		want   string
	}{
		{name: "stimulus length", mutate: func(_ *trajectory.Dataset, stimuli *[]trajectory.StimulusRecord) {
			*stimuli = (*stimuli)[:len(*stimuli)-1]
		}, want: "length"},
		{name: "stimulus trial mismatch", mutate: func(_ *trajectory.Dataset, stimuli *[]trajectory.StimulusRecord) { (*stimuli)[0].TrialID = "other" }, want: "trial"},
		{name: "stimulus time mismatch", mutate: func(_ *trajectory.Dataset, stimuli *[]trajectory.StimulusRecord) { (*stimuli)[0].T = 99 }, want: "time"},
		{name: "non-finite point", mutate: func(dataset *trajectory.Dataset, _ *[]trajectory.StimulusRecord) { dataset.Rows[2].XCM = math.NaN() }, want: "finite"},
		{name: "negative time", mutate: func(dataset *trajectory.Dataset, _ *[]trajectory.StimulusRecord) { dataset.Rows[0].T = -1 }, want: "time"},
		{name: "time rollback", mutate: func(dataset *trajectory.Dataset, _ *[]trajectory.StimulusRecord) {
			dataset.Rows[3].T = dataset.Rows[2].T
		}, want: "time"},
		{name: "interleaved trials", mutate: func(dataset *trajectory.Dataset, stimuli *[]trajectory.StimulusRecord) {
			dataset.Rows = append(dataset.Rows, trajectory.Point{TrialID: "trial-b", Condition: "rewarded", Segment: "after_relocation", T: 0, XCM: 0, YCM: 0})
			*stimuli = append(*stimuli, trajectory.StimulusRecord{TrialID: "trial-b", T: 0})
			dataset.Rows = append(dataset.Rows, trajectory.Point{TrialID: "trial-a", Condition: "rewarded", Segment: "after_relocation", T: 0.6, XCM: 6, YCM: 2})
			*stimuli = append(*stimuli, trajectory.StimulusRecord{TrialID: "trial-a", T: 0.6})
		}, want: "contiguous"},
		{name: "unknown condition", mutate: func(dataset *trajectory.Dataset, _ *[]trajectory.StimulusRecord) {
			dataset.Rows[0].Condition = "unknown"
		}, want: "condition"},
		{name: "fabricated delivered zero", mutate: func(_ *trajectory.Dataset, stimuli *[]trajectory.StimulusRecord) { (*stimuli)[0].Delivered = true }, want: "delivered"},
		{name: "no legal after relocation", mutate: func(dataset *trajectory.Dataset, _ *[]trajectory.StimulusRecord) {
			for i := range dataset.Rows {
				dataset.Rows[i].Segment = "baseline"
			}
		}, want: "after_relocation"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dataset, stimuli := historyFixture()
			test.mutate(&dataset, &stimuli)
			if _, err := buildHistory(dataset, stimuli); err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.want)) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}

	t.Run("trial capacity", func(t *testing.T) {
		dataset := trajectory.Dataset{}
		stimuli := make([]trajectory.StimulusRecord, 0, 16385)
		for i := 0; i < 16385; i++ {
			dataset.Rows = append(dataset.Rows, trajectory.Point{TrialID: "large", Condition: "rewarded", Segment: "after_relocation", T: float64(i) * 0.1, XCM: float64(i), YCM: 0})
			stimuli = append(stimuli, trajectory.StimulusRecord{TrialID: "large", T: float64(i) * 0.1})
		}
		if _, err := buildHistory(dataset, stimuli); err == nil || !strings.Contains(err.Error(), "16384") {
			t.Fatalf("error = %v, want per-trial capacity error", err)
		}
	})
}

func TestImportHistoryRejectsNilContext(t *testing.T) {
	if _, err := importHistory(nil, "fixture.csv"); err == nil || !strings.Contains(err.Error(), "context") {
		t.Fatalf("error = %v, want nil-context error", err)
	}
}

func TestRealSourceIsFixed(t *testing.T) {
	source := realSource()
	if source.SHA256 != "83e13b7057957cf41a45dc91996a4519be7066519242b6912c65b2418cb91670" {
		t.Fatalf("source SHA256 = %q", source.SHA256)
	}
	if source.PinnedCommit != "0eb07940a9ffdedd97ada81f75e9d5f7bface579" {
		t.Fatalf("source commit = %q", source.PinnedCommit)
	}
}

func historyFixture() (trajectory.Dataset, []trajectory.StimulusRecord) {
	rows := []trajectory.Point{
		{TrialID: "trial-a", Condition: "rewarded", Segment: "baseline", T: 0, XCM: 0, YCM: 0},
		{TrialID: "trial-a", Condition: "rewarded", Segment: "relocation", T: 0.1, XCM: 10, YCM: 10},
		{TrialID: "trial-a", Condition: "rewarded", Segment: "after_relocation", T: 0.2, XCM: 1, YCM: 1},
		{TrialID: "trial-a", Condition: "rewarded", Segment: "after_relocation", T: 0.3, XCM: 2, YCM: 1},
		{TrialID: "trial-a", Condition: "rewarded", Segment: "after_relocation", T: 0.4, XCM: 3, YCM: 2},
		{TrialID: "trial-a", Condition: "rewarded", Segment: "cooldown", T: 0.5, XCM: 4, YCM: 2},
		{TrialID: "trial-a", Condition: "rewarded", Segment: "post", T: 0.6, XCM: 5, YCM: 2},
	}
	stimuli := []trajectory.StimulusRecord{
		{TrialID: "trial-a", T: 0},
		{TrialID: "trial-a", T: 0.1, LED: 2, Delivered: true},
		{TrialID: "trial-a", T: 0.2},
		{TrialID: "trial-a", T: 0.3, LED: 1, Delivered: true},
		{TrialID: "trial-a", T: 0.4},
		{TrialID: "trial-a", T: 0.5},
		{TrialID: "trial-a", T: 0.6},
	}
	return trajectory.Dataset{Schema: trajectory.SchemaVersion, Rows: rows}, stimuli
}

func cloneDataset(dataset trajectory.Dataset) trajectory.Dataset {
	clone := dataset
	clone.Rows = append([]trajectory.Point(nil), dataset.Rows...)
	return clone
}
