package experiment

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

// continualFixtureProtocol fixes the fixture protocol of the matrix tests:
// tasks A (delay 2, channel 0) and B (delay 3, channel 1), the stages
// train A, train B, rule_change A, the seeds 7, 42 and 123, and 8 evaluation
// episodes. The train budgets are filled by each test.
func continualFixtureProtocol(budget uint64) ContinualProtocol {
	return ContinualProtocol{
		Tasks: []TaskSpec{
			{Name: "A", Generator: GeneratorDelayedCorrelation, Params: map[string]float64{"delay": 2, "channel": 0}},
			{Name: "B", Generator: GeneratorDelayedCorrelation, Params: map[string]float64{"delay": 3, "channel": 1}},
		},
		Stages: []Stage{
			{Kind: StageTrainTask, Task: "A", Budget: budget},
			{Kind: StageTrainTask, Task: "B", Budget: budget},
			{Kind: StageRuleChange, Task: "A", Budget: 0},
		},
		Seeds:      []uint64{7, 42, 123},
		Evaluation: Evaluation{Episodes: 8, Metric: MetricNegMSE},
	}
}

func TestContinualMatrixUntrainedCellsMatchDirectScores(t *testing.T) {
	p := continualFixtureProtocol(0)
	ctx := context.Background()
	report, err := RunContinualMatrix(ctx, p, ContinualFixture)
	if err != nil {
		t.Fatalf("RunContinualMatrix: %v", err)
	}
	if len(report.Seeds) != len(p.Seeds) {
		t.Fatalf("seeds: got %d want %d", len(report.Seeds), len(p.Seeds))
	}
	a := p.TaskIndex("A")
	b := p.TaskIndex("B")
	for s, seed := range p.Seeds {
		ind, err := ContinualFixture(seed)
		if err != nil {
			t.Fatalf("ContinualFixture(%d): %v", seed, err)
		}
		flipped := make([]bool, len(p.Tasks))
		expected := make([][]float64, len(p.Stages))
		for i, st := range p.Stages {
			expected[i] = make([]float64, len(p.Tasks))
			if st.Kind == StageRuleChange {
				flipped[p.TaskIndex(st.Task)] = !flipped[p.TaskIndex(st.Task)]
			}
			for j := range p.Tasks {
				v, err := evaluateTask(ctx, ind, p.Tasks[j], report.Width, flipped[j], p.Evaluation.Episodes, evalSeed(seed, j), p.Evaluation.FixedChemistry)
				if err != nil {
					t.Fatalf("seed %d direct evaluateTask stage %d task %d: %v", seed, i, j, err)
				}
				expected[i][j] = v
			}
		}
		m := report.Seeds[s]
		if m.Seed != seed {
			t.Fatalf("seed %d: matrix named seed %d", seed, m.Seed)
		}
		for i := range p.Stages {
			for j := range p.Tasks {
				if m.Scores[i][j] != expected[i][j] {
					t.Errorf("seed %d cell (%d,%d): score %g, direct %g", seed, i, j, m.Scores[i][j], expected[i][j])
				}
			}
		}
		for j := range p.Tasks {
			if m.Scores[0][j] != m.Scores[1][j] {
				t.Errorf("seed %d rows 0/1 differ on task %d: %g vs %g", seed, j, m.Scores[0][j], m.Scores[1][j])
			}
		}
		if m.Scores[2][a] == m.Scores[1][a] {
			t.Errorf("seed %d rule_change task A: scores %g == %g, want them to differ", seed, m.Scores[2][a], m.Scores[1][a])
		}
		if m.Scores[2][b] != m.Scores[1][b] {
			t.Errorf("seed %d rule_change task B: scores %g vs %g, want equal", seed, m.Scores[2][b], m.Scores[1][b])
		}
		for j := range p.Tasks {
			if m.Forgetting[0][j] != 0 {
				t.Errorf("seed %d forgetting row 0 task %d = %g, want 0", seed, j, m.Forgetting[0][j])
			}
		}
	}
}

func TestContinualMatrixTrainsAndKeepsEveryRecord(t *testing.T) {
	p := continualFixtureProtocol(20)
	ctx := context.Background()
	report, err := RunContinualMatrix(ctx, p, ContinualFixture)
	if err != nil {
		t.Fatalf("RunContinualMatrix: %v", err)
	}
	want := len(p.Seeds) * (len(p.Stages) + len(p.Stages)*len(p.Tasks))
	if len(report.Runs) != want {
		t.Fatalf("runs: got %d want %d", len(report.Runs), want)
	}
	for i, r := range report.Runs {
		if r.Status != RunStatusOK {
			t.Errorf("run %d (seed %d, stage %d, task %q): status %q, error %q", i, r.Seed, r.Stage, r.Task, r.Status, r.Error)
		}
	}
	if !reflect.DeepEqual(report.Protocol, p) {
		t.Error("report protocol differs from the protocol it ran")
	}
	if len(report.ProtocolHash) != 64 {
		t.Errorf("protocol hash length %d, want 64", len(report.ProtocolHash))
	}
	if len(report.Cells) != len(p.Stages) {
		t.Fatalf("cells rows %d, want %d", len(report.Cells), len(p.Stages))
	}
	for i := range report.Cells {
		if len(report.Cells[i]) != len(p.Tasks) {
			t.Fatalf("cells row %d width %d, want %d", i, len(report.Cells[i]), len(p.Tasks))
		}
		for j := range report.Cells[i] {
			c := report.Cells[i][j]
			if c.Succeeded != len(p.Seeds) || c.Total != len(p.Seeds) {
				t.Errorf("cell (%d,%d): succeeded/total = %d/%d, want %d/%d", i, j, c.Succeeded, c.Total, len(p.Seeds), len(p.Seeds))
			}
		}
	}
	for s := range report.Seeds {
		for i := range report.Seeds[s].Scores {
			for j := range report.Seeds[s].Scores[i] {
				if math.IsNaN(report.Seeds[s].Scores[i][j]) || math.IsInf(report.Seeds[s].Scores[i][j], 0) {
					t.Errorf("seed %d cell (%d,%d) not finite: %g", report.Seeds[s].Seed, i, j, report.Seeds[s].Scores[i][j])
				}
			}
		}
		t.Logf("seed %d scores=%v failed=%v forgetting=%v", report.Seeds[s].Seed, report.Seeds[s].Scores, report.Seeds[s].Failed, report.Seeds[s].Forgetting)
	}
}

func TestContinualMatrixKeepsFailedRuns(t *testing.T) {
	p := continualFixtureProtocol(20)
	ctx := context.Background()
	build := func(seed uint64) (*learning.Individual, error) {
		if seed == 42 {
			c := learning.Config{Dynamics: dynamics.Config{Nodes: 3, Sources: []int{0, 1, 1}, Targets: []int{1, 1, 2}, DT: 1, Activation: "tanh"}, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{2}}
			prm := learning.Parameters{Core: dynamics.Parameters{Weights: []float64{.15 + float64(mix(42)%100)/1000, .2, .2}, Bias: []float64{0, 0, 0}, LogTau: []float64{math.Log(2), math.Log(2), math.Log(2)}}, Encoder: []float64{1, 0, 0}, Readout: []float64{1}}
			o := learning.DefaultOptions()
			o.LearningRate = 0.02
			o.Trainable = learning.Trainable{Weights: true}
			return learning.NewIndividual(c, prm, o, make([]float64, 3))
		}
		return ContinualFixture(seed)
	}
	report, err := RunContinualMatrix(ctx, p, build)
	if err != nil {
		t.Fatalf("RunContinualMatrix: %v", err)
	}
	if len(report.Runs) != 27 {
		t.Fatalf("runs: got %d want 27", len(report.Runs))
	}
	failed := 0
	for _, r := range report.Runs {
		if r.Status != RunStatusOK {
			failed++
			if r.Error == "" {
				t.Errorf("failed run seed %d, stage %d, task %q: empty error", r.Seed, r.Stage, r.Task)
			}
		}
	}
	if failed == 0 {
		t.Error("no failed runs recorded")
	}
	for _, m := range report.Seeds {
		if m.Seed == 42 {
			for i := range m.Failed {
				for j := range m.Failed[i] {
					if !m.Failed[i][j] {
						t.Errorf("seed 42 cell (%d,%d): not marked failed", i, j)
					}
				}
			}
		} else {
			for i := range m.Failed {
				for j := range m.Failed[i] {
					if m.Failed[i][j] {
						t.Errorf("seed %d cell (%d,%d): unexpectedly failed", m.Seed, i, j)
					}
				}
			}
		}
	}
	for i := range report.Cells {
		for j := range report.Cells[i] {
			c := report.Cells[i][j]
			if c.Succeeded != 2 || c.Total != 3 {
				t.Errorf("cell (%d,%d): succeeded/total = %d/%d, want 2/3", i, j, c.Succeeded, c.Total)
			}
		}
	}
	if _, err := json.Marshal(report); err != nil {
		t.Fatalf("json.Marshal(report): %v", err)
	}
}

func TestContinualMatrixIsDeterministic(t *testing.T) {
	p := continualFixtureProtocol(20)
	ctx := context.Background()
	a, err := RunContinualMatrix(ctx, p, ContinualFixture)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	b, err := RunContinualMatrix(ctx, p, ContinualFixture)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	ac, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal first: %v", err)
	}
	bc, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal second: %v", err)
	}
	if !bytes.Equal(ac, bc) {
		t.Error("two identical runs produced different JSON reports")
	}
}

func TestContinualMatrixRejects(t *testing.T) {
	ctx := context.Background()
	good := continualFixtureProtocol(0)

	if _, err := RunContinualMatrix(ctx, ContinualProtocol{}, ContinualFixture); err == nil {
		t.Error("invalid protocol accepted")
	}
	if _, err := RunContinualMatrix(ctx, good, nil); err == nil {
		t.Error("nil build accepted")
	}
	withSwitch := good
	withSwitch.Evaluation.StateSwitch = &StateSwitch{Concentration: [][]float64{{0.5}}}
	if _, err := RunContinualMatrix(ctx, withSwitch, ContinualFixture); err == nil || !strings.Contains(err.Error(), "later ticket") {
		t.Errorf("state switch: got %v, want a later-ticket refusal", err)
	}
	withComparison := good
	withComparison.Comparison = &PreRegistered{Method: ComparisonPairedBootstrap, Interval: 0.95, Baseline: BaselineIndependent, Resamples: 1000}
	if _, err := RunContinualMatrix(ctx, withComparison, ContinualFixture); err == nil || !strings.Contains(err.Error(), "later ticket") {
		t.Errorf("comparison: got %v, want a later-ticket refusal", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := RunContinualMatrix(canceled, good, ContinualFixture); err == nil {
		t.Error("canceled context accepted as a run")
	}
}
