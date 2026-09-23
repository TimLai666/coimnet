package experiment

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/experiment/gridnav"
	"github.com/TimLai666/coimnet/learning"
)

// TestMultiTaskIndividualScoresMatchTheTrainer pins individual and trainer scores bit-for-bit.
func TestMultiTaskIndividualScoresMatchTheTrainer(t *testing.T) {
	ctx := context.Background()
	c := DefaultMultiTaskConfig()
	m, err := newMultiTaskModel(c.Seed, c.Hidden, c.LearningRate)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		obs, actions, err := imitationExpertEpisode(c.Corridor, imitationTrainSeed(c.Seed, i))
		if err != nil {
			t.Fatal(err)
		}
		if err := m.corridorStep(ctx, obs, actions, 1); err != nil {
			t.Fatal(err)
		}
		if err := m.delayedStep(ctx, DelayedEpisode(1001, uint64(i)), 1); err != nil {
			t.Fatal(err)
		}
	}
	pkg, err := m.corePackage()
	if err != nil {
		t.Fatal(err)
	}
	wantCorridor, err := m.corridorScore(ctx, c.Corridor, c.EvalEpisodes)
	if err != nil {
		t.Fatal(err)
	}
	wantDelayed, err := m.delayedScore(ctx, c.EvalEpisodes)
	if err != nil {
		t.Fatal(err)
	}
	gotCorridor, _, err := evaluateOnIndividual(ctx, pkg, MultiTaskCorridor, c.Corridor, c.EvalEpisodes)
	if err != nil {
		t.Fatal(err)
	}
	gotDelayed, _, err := evaluateOnIndividual(ctx, pkg, MultiTaskDelayed, c.Corridor, c.EvalEpisodes)
	if err != nil {
		t.Fatal(err)
	}
	if math.Float64bits(gotCorridor) != math.Float64bits(wantCorridor) {
		t.Errorf("corridor individual score %.17g != trainer score %.17g", gotCorridor, wantCorridor)
	}
	if math.Float64bits(gotDelayed) != math.Float64bits(wantDelayed) {
		t.Errorf("delayed individual score %.17g != trainer score %.17g", gotDelayed, wantDelayed)
	}
}

// TestMultiTaskLineageAndIndependence checks parent lineage and neural-state isolation.
func TestMultiTaskLineageAndIndependence(t *testing.T) {
	ctx := context.Background()
	c := DefaultMultiTaskConfig()
	m, err := newMultiTaskModel(c.Seed, c.Hidden, c.LearningRate)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := m.corePackage()
	if err != nil {
		t.Fatal(err)
	}
	_, corridorLineage, err := evaluateOnIndividual(ctx, pkg, MultiTaskCorridor, c.Corridor, 2)
	if err != nil {
		t.Fatal(err)
	}
	_, delayedLineage, err := evaluateOnIndividual(ctx, pkg, MultiTaskDelayed, c.Corridor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(corridorLineage) != 3 || len(delayedLineage) != 3 {
		t.Fatalf("lineage lengths = %d and %d, want 3", len(corridorLineage), len(delayedLineage))
	}
	if corridorLineage[0] != delayedLineage[0] || corridorLineage[1] != delayedLineage[1] {
		t.Fatalf("lineages do not share package and initial-state hashes: %v, %v", corridorLineage, delayedLineage)
	}
	if corridorLineage[2] == delayedLineage[2] {
		t.Fatalf("task evaluations produced same final state hash: %v", corridorLineage[2])
	}
	a, err := checkpoint.NewIndividualFromPackage(pkg, learning.DefaultOptions(), make([]float64, pkg.Config.Dynamics.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	b, err := checkpoint.NewIndividualFromPackage(pkg, learning.DefaultOptions(), make([]float64, pkg.Config.Dynamics.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	before := b.Snapshot().Neural
	if _, err := a.Advance(ctx, multiTaskCorridorRows([][]float64{{1, 0, 0, 0}, {0, 1, 0, 0}})); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.Snapshot().Neural, before) {
		t.Fatal("advancing corridor individual changed the other individual's neural state")
	}
}

// TestRunMultiTaskReportsOneBrain checks the shared-brain report contract.
func TestRunMultiTaskReportsOneBrain(t *testing.T) {
	c := DefaultMultiTaskConfig()
	c.Schedule.Budget = 40
	c.EvalEpisodes = 4
	report, err := RunMultiTask(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != MultiTaskSchemaVersion || report.TrainersBuilt != 1 || !report.SameBrain {
		t.Fatalf("schema/build/brain = %q/%d/%t", report.SchemaVersion, report.TrainersBuilt, report.SameBrain)
	}
	if len(report.Results) != 2 || report.Results[0].Task != MultiTaskCorridor || report.Results[1].Task != MultiTaskDelayed {
		t.Fatalf("results order/content = %+v", report.Results)
	}
	if report.Results[0].BrainTopologyHash == "" || report.Results[0].BrainTopologyHash != report.Results[1].BrainTopologyHash || report.Results[0].BaseParameterHash == "" || report.Results[0].BaseParameterHash != report.Results[1].BaseParameterHash {
		t.Fatalf("result brain fingerprints differ or are empty: %+v", report.Results)
	}
	m, err := newMultiTaskModel(c.Seed, c.Hidden, c.LearningRate)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := runMultiTaskSchedule(context.Background(), m, c.Schedule, c.Corridor, c.Seed); err != nil {
		t.Fatal(err)
	}
	pkg, err := m.corePackage()
	if err != nil {
		t.Fatal(err)
	}
	if report.Results[0].BrainTopologyHash != pkg.Topology.SHA256 {
		t.Fatalf("brain topology hash %q != package hash %q", report.Results[0].BrainTopologyHash, pkg.Topology.SHA256)
	}
	if report.Results[0].BaseParameterHash != hash(pkg.Parameters.Core) {
		t.Fatalf("base parameter hash %q != package core hash %q", report.Results[0].BaseParameterHash, hash(pkg.Parameters.Core))
	}
	if report.Results[0].AdapterVersion == report.Results[1].AdapterVersion {
		t.Fatalf("task adapter versions unexpectedly match: %q", report.Results[0].AdapterVersion)
	}
	if report.Results[0].Metric != "expert_agreement" || report.Results[1].Metric != "negative_mse" || report.Results[0].IndividualID != "corridor-eval" || report.Results[1].IndividualID != "delayed-eval" {
		t.Errorf("result task metadata = %+v", report.Results)
	}
	if len(report.Results[0].StateLineage) != 3 || len(report.Results[1].StateLineage) != 3 || report.Results[0].StateLineage[0] != report.Results[1].StateLineage[0] || report.Results[0].StateLineage[1] != report.Results[1].StateLineage[1] || report.Results[0].StateLineage[2] == report.Results[1].StateLineage[2] {
		t.Errorf("result state lineages = %v, %v", report.Results[0].StateLineage, report.Results[1].StateLineage)
	}
	if len(report.Shares) != 2 {
		t.Fatalf("shares = %+v, want two", report.Shares)
	}
	for i, task := range c.Schedule.Tasks {
		share := report.Shares[i]
		if share.Task != task.Name || share.LossScale != task.LossScale || share.MixWeight != task.MixWeight || share.SamplingRatio != task.SamplingRatio || share.UpdateEvery != task.UpdateEvery {
			t.Errorf("share %d = %+v, want declaration %+v", i, share, task)
		}
	}
	wantAssumptions := []string{
		"Both tasks train one core; a task's adapter is its encoder rows and readout columns, and one AdamW state serves both, so momentum can move an idle task's adapter.",
		"Each task is scored on its own learning.Individual built from the shared model package; the individuals share the brain fingerprints and nothing else.",
		"The corridor is the one-dimensional gridnav fixture and the delayed task the pulse-association fixture; these are fixtures, not the fly connectome.",
	}
	if !reflect.DeepEqual(report.Assumptions, wantAssumptions) {
		t.Errorf("assumptions = %q, want %q", report.Assumptions, wantAssumptions)
	}
}

// TestRunMultiTaskEachSchedule executes every supported schedule.
func TestRunMultiTaskEachSchedule(t *testing.T) {
	for _, schedule := range []string{MultiTaskInterleaved, MultiTaskSameExperience, MultiTaskMissingModality} {
		t.Run(schedule, func(t *testing.T) {
			c := DefaultMultiTaskConfig()
			c.Schedule.Schedule = schedule
			c.Schedule.Budget = 40
			c.EvalEpisodes = 4
			if schedule != MultiTaskMissingModality {
				c.Schedule.MissingEvery = 0
			}
			report, err := RunMultiTask(context.Background(), c)
			if err != nil {
				t.Fatal(err)
			}
			if schedule == MultiTaskInterleaved && report.Steps != c.Schedule.Budget {
				t.Errorf("steps = %d, want budget %d", report.Steps, c.Schedule.Budget)
			}
			if schedule == MultiTaskSameExperience && report.Shares[0].Updates != report.Shares[1].Updates {
				t.Errorf("same_experience updates = %d, %d", report.Shares[0].Updates, report.Shares[1].Updates)
			}
			for i, share := range report.Shares {
				result := report.Results[i]
				t.Logf("schedule=%s task=%s updates=%d share=%.6f before=%.6f after=%.6f", schedule, share.Task, share.Updates, share.Share, result.Before, result.After)
			}
		})
	}
}

// TestRunMultiTaskIsDeterministic checks serialized reports for repeatability.
func TestRunMultiTaskIsDeterministic(t *testing.T) {
	c := DefaultMultiTaskConfig()
	c.Schedule.Budget = 40
	c.EvalEpisodes = 4
	a, err := RunMultiTask(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	b, err := RunMultiTask(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	ja, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	jb, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ja, jb) {
		t.Fatal("RunMultiTask reports differ for identical config")
	}
}

// TestDefaultMultiTaskConfig pins the evidence protocol and slice ownership.
func TestDefaultMultiTaskConfig(t *testing.T) {
	c := DefaultMultiTaskConfig()
	if c.Schedule.Schedule != MultiTaskMissingModality || c.Schedule.Budget != 400 || c.Schedule.Tolerance != 0.1 || c.Schedule.MissingEvery != 4 || c.Seed != 2 || c.Hidden != 16 || c.LearningRate != 0.05 || c.EvalEpisodes != 20 || c.Corridor != (gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: 0.01, GoalReward: 1}) {
		t.Fatalf("DefaultMultiTaskConfig() = %+v", c)
	}
	wantTasks := []TaskBinding{
		{Name: MultiTaskCorridor, LossScale: 1, MixWeight: 1, SamplingRatio: 1, UpdateEvery: 1},
		{Name: MultiTaskDelayed, LossScale: 1, MixWeight: 1, SamplingRatio: 1, UpdateEvery: 1},
	}
	if !reflect.DeepEqual(c.Schedule.Tasks, wantTasks) {
		t.Fatalf("tasks = %+v, want %+v", c.Schedule.Tasks, wantTasks)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Schedule.Tasks[0].Name = "changed"
	if DefaultMultiTaskConfig().Schedule.Tasks[0].Name != MultiTaskCorridor {
		t.Fatal("DefaultMultiTaskConfig shared task slices")
	}
}

// TestMultiTaskConfigRejects checks field-specific errors for invalid settings.
func TestMultiTaskConfigRejects(t *testing.T) {
	base := DefaultMultiTaskConfig()
	cases := []struct {
		name  string
		field string
		edit  func(*MultiTaskConfig)
	}{
		{"hidden", "hidden", func(c *MultiTaskConfig) { c.Hidden = 0 }},
		{"learning rate", "learning_rate", func(c *MultiTaskConfig) { c.LearningRate = 0 }},
		{"eval episodes", "eval_episodes", func(c *MultiTaskConfig) { c.EvalEpisodes = 0 }},
		{"corridor", "corridor", func(c *MultiTaskConfig) { c.Corridor.Length = 0 }},
		{"schedule", "schedule", func(c *MultiTaskConfig) { c.Schedule.Schedule = "bad" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			c.Schedule.Tasks = append([]TaskBinding(nil), base.Schedule.Tasks...)
			tc.edit(&c)
			err := c.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("Validate() error = %v, want field %q", err, tc.field)
			}
		})
	}
}
