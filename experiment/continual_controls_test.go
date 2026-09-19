package experiment

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// continualFixtureStateSwitch is the fixture protocol of the control tests
// with the evaluation fixed-chemistry flag on and the declared switch state.
func continualFixtureStateSwitch(budget uint64, conc [][]float64) ContinualProtocol {
	p := continualFixtureProtocol(budget)
	p.Evaluation.FixedChemistry = true
	p.Evaluation.StateSwitch = &StateSwitch{Concentration: conc}
	return p
}

// TestIndependentMatchesDirectSingleTaskTraining checks that the independent
// control of one task equals training and scoring exactly that task alone:
// the report keeps it apart from the matrix, so it must not fold into Cells.
func TestIndependentMatchesDirectSingleTaskTraining(t *testing.T) {
	p := continualFixtureProtocol(10)
	ctx := context.Background()
	report, err := RunContinualMatrix(ctx, p, ContinualFixture)
	if err != nil {
		t.Fatalf("RunContinualMatrix: %v", err)
	}
	b := p.TaskIndex("B")
	if b == -1 {
		t.Fatal("the fixture protocol has no task B")
	}
	const seed = uint64(7)
	ind, err := ContinualFixture(seed)
	if err != nil {
		t.Fatalf("ContinualFixture(%d): %v", seed, err)
	}
	flipped := false
	for i, st := range p.Stages {
		if p.TaskIndex(st.Task) != b {
			continue
		}
		if st.Kind == StageRuleChange {
			flipped = !flipped
			continue
		}
		for e := uint64(0); e < st.Budget; e++ {
			ep, err := ContinualEpisode(p.Tasks[b], report.Width, flipped, trainSeed(seed, i), e)
			if err != nil {
				t.Fatalf("ContinualEpisode: %v", err)
			}
			if _, err := ind.TrainEpisode(ctx, ep.Input, ep.Target); err != nil {
				t.Fatalf("TrainEpisode: %v", err)
			}
		}
	}
	direct, err := evaluateTask(ctx, ind, p.Tasks[b], report.Width, flipped, p.Evaluation.Episodes, evalSeed(seed, b), p.Evaluation.FixedChemistry)
	if err != nil {
		t.Fatalf("direct evaluateTask: %v", err)
	}

	if len(report.Independent) != len(p.Tasks) {
		t.Fatalf("independent tasks: got %d want %d", len(report.Independent), len(p.Tasks))
	}
	ir := report.Independent[b]
	if ir.Task != "B" {
		t.Fatalf("independent[%d] task %q, want %q", b, ir.Task, "B")
	}
	if ir.Stat.Succeeded != len(p.Seeds) || ir.Stat.Total != len(p.Seeds) {
		t.Errorf("independent %s stat %d/%d, want %d/%d", ir.Task, ir.Stat.Succeeded, ir.Stat.Total, len(p.Seeds), len(p.Seeds))
	}
	sx := -1
	for s, sv := range p.Seeds {
		if sv == seed {
			sx = s
		}
	}
	if sx == -1 {
		t.Fatalf("seed %d not in the protocol seeds", seed)
	}
	if sx >= len(ir.Seeds) {
		t.Fatalf("independent %s has %d seeds, want one per protocol seed", ir.Task, len(ir.Seeds))
	}
	ss := ir.Seeds[sx]
	if ss.Failed {
		t.Fatalf("independent %s seed %d failed: %s", ir.Task, ss.Seed, ss.Error)
	}
	if ss.Score != direct {
		t.Errorf("independent %s seed %d score %g, direct single-task training %g", ir.Task, ss.Seed, ss.Score, direct)
	}

	again, err := RunContinualMatrix(ctx, p, ContinualFixture)
	if err != nil {
		t.Fatalf("second RunContinualMatrix: %v", err)
	}
	ac, err := json.Marshal(report.Cells)
	if err != nil {
		t.Fatalf("marshal first cells: %v", err)
	}
	bc, err := json.Marshal(again.Cells)
	if err != nil {
		t.Fatalf("marshal second cells: %v", err)
	}
	if !bytes.Equal(ac, bc) {
		t.Error("the independent control changed the Cells JSON of an identical run")
	}
}

// TestStateSwitchMovesScoreWithoutTouchingParameters runs the chemistry fixture
// under a switched concentration and checks every cell's switched score moves
// away from the frozen natural one while every record stays successful: state
// differs, parameters and plastic are checked unchanged by scoreTwin.
func TestStateSwitchMovesScoreWithoutTouchingParameters(t *testing.T) {
	p := continualFixtureStateSwitch(10, [][]float64{{2}})
	ctx := context.Background()
	report, err := RunContinualMatrix(ctx, p, ContinualFixtureWithChemistry)
	if err != nil {
		t.Fatalf("RunContinualMatrix: %v", err)
	}
	for _, sm := range report.Seeds {
		if len(sm.Switched) != len(p.Stages) {
			t.Fatalf("seed %d switched rows %d, want %d", sm.Seed, len(sm.Switched), len(p.Stages))
		}
		for i := range sm.Scores {
			if len(sm.Switched[i]) != len(p.Tasks) {
				t.Fatalf("seed %d switched row %d width %d, want %d", sm.Seed, i, len(sm.Switched[i]), len(p.Tasks))
			}
			for j := range sm.Scores[i] {
				if sm.Switched[i][j] == sm.Scores[i][j] {
					t.Errorf("seed %d cell (%d,%d): switched %g equals the frozen score %g, want a moved score", sm.Seed, i, j, sm.Switched[i][j], sm.Scores[i][j])
				}
				if sm.SwitchedFailed[i][j] {
					t.Errorf("seed %d cell (%d,%d): switched marked failed", sm.Seed, i, j)
				}
			}
		}
	}
	for _, r := range report.Runs {
		if r.Status == RunStatusFailed {
			t.Errorf("run seed %d stage %d task %q: %s", r.Seed, r.Stage, r.Task, r.Error)
		}
	}
	a, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal first report: %v", err)
	}
	again, err := RunContinualMatrix(ctx, p, ContinualFixtureWithChemistry)
	if err != nil {
		t.Fatalf("second RunContinualMatrix: %v", err)
	}
	bc, err := json.Marshal(again)
	if err != nil {
		t.Fatalf("marshal second report: %v", err)
	}
	if !bytes.Equal(a, bc) {
		t.Error("two identical runs produced different JSON reports")
	}
}

// TestStateSwitchWithoutChemistryIsRecorded checks that a switched evaluation
// on an individual the protocol never gave chemistry to fails every cell, keeps
// the failing record with the state_switch control and leaves the matrix scores
// themselves untouched.
func TestStateSwitchWithoutChemistryIsRecorded(t *testing.T) {
	p := continualFixtureStateSwitch(10, [][]float64{{2}})
	ctx := context.Background()
	report, err := RunContinualMatrix(ctx, p, ContinualFixture)
	if err != nil {
		t.Fatalf("RunContinualMatrix: %v", err)
	}
	for _, sm := range report.Seeds {
		for i := range sm.Scores {
			for j := range sm.Scores[i] {
				if sm.Failed[i][j] {
					t.Errorf("seed %d cell (%d,%d): the matrix score failed under a missing chemistry", sm.Seed, i, j)
				}
				if !sm.SwitchedFailed[i][j] {
					t.Errorf("seed %d cell (%d,%d): switched failure not recorded", sm.Seed, i, j)
				}
			}
		}
	}
	want := len(p.Seeds) * len(p.Stages) * len(p.Tasks)
	seen := 0
	for _, r := range report.Runs {
		if r.Control != ControlStateSwitch {
			continue
		}
		seen++
		if r.Status != RunStatusFailed {
			t.Errorf("state_switch run seed %d stage %d task %q: status %q", r.Seed, r.Stage, r.Task, r.Status)
		}
		if !strings.Contains(r.Error, "chemistry") {
			t.Errorf("state_switch run seed %d stage %d task %q: error %q lacks %q", r.Seed, r.Stage, r.Task, r.Error, "chemistry")
		}
	}
	if seen != want {
		t.Errorf("state_switch records: got %d want %d", seen, want)
	}
}

// TestStateSwitchShapeMismatchIsRecorded checks that a switched concentration
// whose shape does not fit the enabled chemistry fails every cell and is kept
// as a state_switch record without touching the matrix scores.
func TestStateSwitchShapeMismatchIsRecorded(t *testing.T) {
	p := continualFixtureStateSwitch(10, [][]float64{{1, 2}})
	ctx := context.Background()
	report, err := RunContinualMatrix(ctx, p, ContinualFixtureWithChemistry)
	if err != nil {
		t.Fatalf("RunContinualMatrix: %v", err)
	}
	for _, sm := range report.Seeds {
		for i := range sm.Scores {
			for j := range sm.Scores[i] {
				if sm.Failed[i][j] {
					t.Errorf("seed %d cell (%d,%d): the matrix score failed under a shape mismatch", sm.Seed, i, j)
				}
				if !sm.SwitchedFailed[i][j] {
					t.Errorf("seed %d cell (%d,%d): switched failure not recorded", sm.Seed, i, j)
				}
			}
		}
	}
	want := len(p.Seeds) * len(p.Stages) * len(p.Tasks)
	seen := 0
	for _, r := range report.Runs {
		if r.Control != ControlStateSwitch {
			continue
		}
		seen++
		if r.Status != RunStatusFailed {
			t.Errorf("state_switch run seed %d stage %d task %q: status %q", r.Seed, r.Stage, r.Task, r.Status)
		}
		if !strings.Contains(r.Error, "state_switch") {
			t.Errorf("state_switch run seed %d stage %d task %q: error %q lacks %q", r.Seed, r.Stage, r.Task, r.Error, "state_switch")
		}
	}
	if seen != want {
		t.Errorf("state_switch records: got %d want %d", seen, want)
	}
	if len(report.Independent) != len(p.Tasks) {
		t.Errorf("independent tasks: got %d want %d", len(report.Independent), len(p.Tasks))
	}
}
