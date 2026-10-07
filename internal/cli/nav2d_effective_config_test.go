package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/TimLai666/coimnet/experiment"
	"github.com/TimLai666/coimnet/experiment/nav2d"
)

func TestExamplesEffectiveConfigReports(t *testing.T) {
	for _, kind := range []string{"single", "suite", "attribution"} {
		t.Run(kind, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := []string{"examples", "run", "nav2d", "--seeds", "7", "--episodes", "1", "--hidden", "4", "--recurrent", "1", "--eval", "1", "--policies", "random"}
			if kind == "single" {
				args = append(args, "--task", "avoid_obstacles")
			}
			if kind == "attribution" {
				args = []string{"examples", "run", "attribution", "--seeds", "1,2,3", "--episodes", "1", "--hidden", "4", "--recurrent", "1", "--eval", "1", "--groups", "normal", "--resamples", "100"}
			}
			path := ""
			if kind == "suite" {
				path = filepath.Join(t.TempDir(), "suite.json")
				args = append(args, "--out", path)
			}
			if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
				t.Fatalf("%v: %v", args, err)
			}
			data := stdout.Bytes()
			if path != "" {
				var err error
				data, err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if stderr.Len() != 0 {
					t.Errorf("suite stderr = %s", stderr.String())
				}
			} else if stderr.Len() == 0 {
				t.Error("missing stderr summary")
			}
			check := func(task string, env nav2d.Config) {
				t.Helper()
				want := nav2d.Config{Width: 9, Height: 9, WallDensity: .2, ViewDepth: 3, TimeLimit: 60, StepPenalty: .01, CollisionPenalty: .05, GoalReward: 1, Task: task}
				if env != want {
					t.Errorf("CLI config.env = %+v, want %+v", env, want)
				}
			}
			switch kind {
			case "single":
				var r experiment.Nav2DReport
				if err := json.Unmarshal(data, &r); err != nil {
					t.Fatal(err)
				}
				check(r.Task, r.Config.Env)
			case "suite":
				var r experiment.Nav2DSuiteReport
				if err := json.Unmarshal(data, &r); err != nil {
					t.Fatal(err)
				}
				if len(r.Tasks) != 4 {
					t.Fatalf("tasks = %d", len(r.Tasks))
				}
				for _, task := range r.Tasks {
					check(task.Task, task.Config.Env)
				}
				saved := append([]byte(nil), data...)
				stdout.Reset()
				stderr.Reset()
				if err := Run(context.Background(), args, &stdout, &stderr); err == nil {
					t.Error("existing report overwritten")
				}
				after, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(saved, after) {
					t.Error("existing output changed")
				}
			case "attribution":
				var r experiment.AttributionReport
				if err := json.Unmarshal(data, &r); err != nil {
					t.Fatal(err)
				}
				check(nav2d.TaskAvoidObstacles, r.Config.Env)
			}
		})
	}
}
