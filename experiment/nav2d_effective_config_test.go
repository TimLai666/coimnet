package experiment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/TimLai666/coimnet/experiment/nav2d"
	"reflect"
	"testing"
)

func effectiveConfigExpected(task string) nav2d.Config {
	return nav2d.Config{Width: 9, Height: 9, WallDensity: .2, ViewDepth: 3, TimeLimit: 60, StepPenalty: .01, CollisionPenalty: .05, GoalReward: 1, Task: task}
}
func effectiveConfigHash(t *testing.T, c any) string {
	t.Helper()
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func effectiveConfigNav() Nav2DConfig {
	c := DefaultNav2DConfig()
	c.Env = nav2d.Config{}
	c.Seeds = []uint64{7}
	c.Episodes = 1
	c.Hidden = 4
	c.Recurrent = 1
	c.EvalEpisodes = 1
	c.Policies = []string{Nav2DRecurrent, Nav2DRandom}
	return c
}
func effectiveConfigAttribution() AttributionConfig {
	c := DefaultAttributionConfig()
	c.Seeds = []uint64{1, 2, 3}
	c.Episodes = 1
	c.Hidden = 4
	c.Recurrent = 1
	c.EvalEpisodes = 1
	c.Groups = []string{AttributionNormal, AttributionFrozenCore}
	c.Comparison.Resamples = 100
	return c
}
func TestNav2DEffectiveConfigReport(t *testing.T) {
	c := effectiveConfigNav()
	before, _ := json.Marshal(c)
	r, err := RunNav2D(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	want := effectiveConfigExpected(nav2d.TaskAvoidObstacles)
	if r.Config.Env != want || r.Task != want.Task {
		t.Errorf("reported env/task = %+v/%s, want %+v", r.Config.Env, r.Task, want)
	}
	if r.ConfigHash != effectiveConfigHash(t, r.Config) {
		t.Error("hash does not describe reported config")
	}
	explicit := c
	explicit.Env = want
	e, err := RunNav2D(context.Background(), explicit)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r, e) {
		t.Error("implicit and explicit defaults produced different reports")
	}
	after, _ := json.Marshal(c)
	if string(before) != string(after) {
		t.Error("caller config changed")
	}
	custom := nav2d.Config{Width: 7, Height: 11, WallDensity: .3, ViewDepth: 2, TimeLimit: 8, StepPenalty: .02, CollisionPenalty: .08, GoalReward: 2, Task: nav2d.TaskRememberGoal}
	c.Env = custom
	r, err = RunNav2D(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if r.Config.Env != custom || r.Task != custom.Task {
		t.Error("explicit environment changed")
	}
	if r.ConfigHash != effectiveConfigHash(t, r.Config) {
		t.Error("custom hash mismatch")
	}
	c.Env.GoalReward = 3
	changed, err := RunNav2D(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if changed.ConfigHash == r.ConfigHash {
		t.Error("effective change did not affect hash")
	}
	for _, run := range r.Runs {
		if run.Failed {
			t.Errorf("custom run failed: %s", run.Error)
		}
	}
	t.Run("errors", func(t *testing.T) {
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		for _, ctx := range []context.Context{nil, cancelled} {
			r, err := RunNav2D(ctx, c)
			if err == nil || !reflect.DeepEqual(r, Nav2DReport{}) {
				t.Errorf("ctx %v: partial report or missing error", ctx)
			}
		}
		c.Env.Width = 6
		r, err := RunNav2D(context.Background(), c)
		if err == nil || !reflect.DeepEqual(r, Nav2DReport{}) {
			t.Error("invalid environment accepted")
		}
	})
}
func TestNav2DSuiteEffectiveConfigReport(t *testing.T) {
	c := effectiveConfigNav()
	c.Policies = []string{Nav2DRandom}
	r, err := RunNav2DSuite(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	tasks := []string{nav2d.TaskRememberGoal, nav2d.TaskAvoidObstacles, nav2d.TaskAdaptAfterChange, nav2d.TaskLanguageGoal}
	if len(r.Tasks) != len(tasks) {
		t.Fatalf("tasks = %d", len(r.Tasks))
	}
	for i, task := range r.Tasks {
		want := effectiveConfigExpected(tasks[i])
		if task.Task != tasks[i] || task.Config.Env != want {
			t.Errorf("task %d env = %+v", i, task.Config.Env)
		}
		if task.ConfigHash != effectiveConfigHash(t, task.Config) {
			t.Errorf("task %d hash mismatch", i)
		}
	}
	if c.Env != (nav2d.Config{}) {
		t.Error("suite changed caller environment")
	}
	c.Env = effectiveConfigExpected(nav2d.TaskAvoidObstacles)
	explicit, err := RunNav2DSuite(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r, explicit) {
		t.Error("suite defaults differ from explicit values")
	}
}
func TestAttributionEffectiveConfigReport(t *testing.T) {
	c := effectiveConfigAttribution()
	before, _ := json.Marshal(c)
	r, err := RunAttribution(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	want := effectiveConfigExpected(nav2d.TaskAvoidObstacles)
	if r.Config.Env != want {
		t.Errorf("env = %+v, want %+v", r.Config.Env, want)
	}
	if r.ConfigHash != effectiveConfigHash(t, r.Config) {
		t.Error("attribution hash mismatch")
	}
	explicit := c
	explicit.Env = want
	e, err := RunAttribution(context.Background(), explicit)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r, e) {
		t.Error("attribution default reports differ")
	}
	after, _ := json.Marshal(c)
	if string(before) != string(after) {
		t.Error("attribution changed caller")
	}
	c.Env = nav2d.Config{Width: 7, Height: 11, WallDensity: .3, ViewDepth: 2, TimeLimit: 8, StepPenalty: .02, CollisionPenalty: .08, GoalReward: 2, Task: nav2d.TaskRememberGoal}
	custom, err := RunAttribution(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if custom.Config.Env != c.Env || custom.ConfigHash != effectiveConfigHash(t, custom.Config) {
		t.Error("custom attribution env/hash mismatch")
	}
	c.Env.GoalReward = 3
	changed, err := RunAttribution(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if changed.ConfigHash == custom.ConfigHash {
		t.Error("changed attribution config hash unchanged")
	}
	for _, run := range custom.Runs {
		if run.Failed {
			t.Errorf("custom attribution failed: %s", run.Error)
		}
	}
	t.Run("errors", func(t *testing.T) {
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		for _, ctx := range []context.Context{nil, cancelled} {
			r, err := RunAttribution(ctx, c)
			if err == nil || !reflect.DeepEqual(r, AttributionReport{}) {
				t.Error("missing context error or partial attribution")
			}
		}
		c.Env.Task = ""
		r, err := RunAttribution(context.Background(), c)
		if err == nil || !reflect.DeepEqual(r, AttributionReport{}) {
			t.Error("empty attribution task accepted")
		}
	})
}
