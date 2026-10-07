package experiment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func ownershipNav2DConfig() Nav2DConfig {
	c := DefaultNav2DConfig()
	c.Env.TimeLimit = 4
	c.Seeds = make([]uint64, 2, 4)
	copy(c.Seeds, []uint64{7, 9})
	c.Policies = make([]string, 2, 4)
	copy(c.Policies, []string{Nav2DRandom, Nav2DFeedforward})
	c.Episodes = 1
	c.Hidden = 4
	c.Recurrent = 1
	c.EvalEpisodes = 1
	return c
}

func ownershipJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return data
}

func ownershipConfigHash(t *testing.T, config Nav2DConfig) string {
	t.Helper()
	data := ownershipJSON(t, config)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func assertOwnershipNav2DReport(t *testing.T, report Nav2DReport) {
	t.Helper()
	if len(report.Runs) != 4 {
		t.Fatalf("report has %d runs, want 4", len(report.Runs))
	}
	if report.ConfigHash != ownershipConfigHash(t, report.Config) {
		t.Fatalf("ConfigHash %q does not match independently marshaled config", report.ConfigHash)
	}
	for i, run := range report.Runs {
		if run.Failed {
			t.Fatalf("run %d (%s seed %d) failed: %s", i, run.Policy, run.Seed, run.Error)
		}
	}
}

func assertOwnershipNav2DSuite(t *testing.T, report Nav2DSuiteReport) {
	t.Helper()
	if len(report.Tasks) != 4 {
		t.Fatalf("suite has %d tasks, want 4", len(report.Tasks))
	}
	for i, task := range report.Tasks {
		assertOwnershipNav2DReport(t, task)
		t.Logf("task %d %q: %d runs", i, task.Task, len(task.Runs))
	}
}

func TestNav2DReportConfigOwnership(t *testing.T) {
	t.Run("caller_to_report", func(t *testing.T) {
		config := ownershipNav2DConfig()
		report, err := RunNav2D(context.Background(), config)
		if err != nil {
			t.Fatalf("RunNav2D: %v", err)
		}
		assertOwnershipNav2DReport(t, report)
		beforeJSON := ownershipJSON(t, report)
		beforeHash := report.ConfigHash

		config.Seeds[0] = 1007
		if got := ownershipJSON(t, report); !bytes.Equal(got, beforeJSON) {
			t.Fatalf("caller Seeds edit changed the complete report JSON")
		}
		t.Log("Seeds control preserved: caller seed edit left report JSON unchanged")

		config.Policies[0] = Nav2DRewired

		afterJSON := ownershipJSON(t, report)
		if !bytes.Equal(afterJSON, beforeJSON) {
			t.Fatalf("caller element edits changed the complete report JSON")
		}
		if report.ConfigHash != beforeHash {
			t.Fatalf("caller element edits changed report ConfigHash from %q to %q", beforeHash, report.ConfigHash)
		}
		if report.ConfigHash != ownershipConfigHash(t, report.Config) {
			t.Fatalf("report ConfigHash no longer matches the unchanged report config")
		}
	})

	t.Run("report_to_caller", func(t *testing.T) {
		config := ownershipNav2DConfig()
		beforeCallerJSON := ownershipJSON(t, config)
		report, err := RunNav2D(context.Background(), config)
		if err != nil {
			t.Fatalf("RunNav2D: %v", err)
		}
		assertOwnershipNav2DReport(t, report)

		report.Config.Seeds[0] = 2007
		if got := ownershipJSON(t, config); !bytes.Equal(got, beforeCallerJSON) {
			t.Fatalf("report Seeds edit changed the caller config JSON")
		}
		t.Log("Seeds control preserved: report seed edit left caller JSON unchanged")

		report.Config.Policies[0] = Nav2DRewired

		afterCallerJSON := ownershipJSON(t, config)
		if !bytes.Equal(afterCallerJSON, beforeCallerJSON) {
			t.Fatalf("report element edits changed the caller config JSON")
		}
	})
}

func TestNav2DSuiteConfigOwnership(t *testing.T) {
	t.Run("caller_to_all", func(t *testing.T) {
		config := ownershipNav2DConfig()
		suite, err := RunNav2DSuite(context.Background(), config)
		if err != nil {
			t.Fatalf("RunNav2DSuite: %v", err)
		}
		assertOwnershipNav2DSuite(t, suite)
		beforeSuiteJSON := ownershipJSON(t, suite)
		beforeTaskJSON := make([][]byte, len(suite.Tasks))
		beforeTaskHash := make([]string, len(suite.Tasks))
		for i, task := range suite.Tasks {
			beforeTaskJSON[i] = ownershipJSON(t, task)
			beforeTaskHash[i] = task.ConfigHash
		}

		config.Seeds[0] = 3007
		if got := ownershipJSON(t, suite); !bytes.Equal(got, beforeSuiteJSON) {
			t.Fatalf("caller Seeds edit changed the complete suite JSON")
		}
		for i, task := range suite.Tasks {
			if got := ownershipJSON(t, task); !bytes.Equal(got, beforeTaskJSON[i]) {
				t.Fatalf("caller Seeds edit changed task %d complete report JSON", i)
			}
			if task.ConfigHash != beforeTaskHash[i] {
				t.Fatalf("caller Seeds edit changed task %d ConfigHash from %q to %q", i, beforeTaskHash[i], task.ConfigHash)
			}
		}
		t.Log("Seeds control preserved: caller seed edit left suite and all task reports unchanged")

		config.Policies[0] = Nav2DRewired

		afterSuiteJSON := ownershipJSON(t, suite)
		if !bytes.Equal(afterSuiteJSON, beforeSuiteJSON) {
			t.Fatalf("caller element edits changed the complete suite JSON")
		}
		for i, task := range suite.Tasks {
			if got := ownershipJSON(t, task); !bytes.Equal(got, beforeTaskJSON[i]) {
				t.Errorf("caller element edits changed task %d complete report JSON", i)
			}
			if task.ConfigHash != beforeTaskHash[i] {
				t.Errorf("caller element edits changed task %d ConfigHash from %q to %q", i, beforeTaskHash[i], task.ConfigHash)
			}
		}
	})

	for target := 0; target < 4; target++ {
		t.Run("report_to_caller_and_other_tasks_"+string(rune('0'+target)), func(t *testing.T) {
			config := ownershipNav2DConfig()
			beforeCallerJSON := ownershipJSON(t, config)
			suite, err := RunNav2DSuite(context.Background(), config)
			if err != nil {
				t.Fatalf("RunNav2DSuite: %v", err)
			}
			assertOwnershipNav2DSuite(t, suite)
			beforeTaskJSON := make([][]byte, len(suite.Tasks))
			beforeTaskHash := make([]string, len(suite.Tasks))
			for i, task := range suite.Tasks {
				beforeTaskJSON[i] = ownershipJSON(t, task)
				beforeTaskHash[i] = task.ConfigHash
			}

			suite.Tasks[target].Config.Seeds[0] = uint64(4007 + target)
			if got := ownershipJSON(t, config); !bytes.Equal(got, beforeCallerJSON) {
				t.Fatalf("task %d Seeds edit changed the caller config JSON", target)
			}
			for i, task := range suite.Tasks {
				if i == target {
					continue
				}
				if got := ownershipJSON(t, task); !bytes.Equal(got, beforeTaskJSON[i]) {
					t.Fatalf("task %d Seeds edit changed other task %d complete report JSON", target, i)
				}
				if task.ConfigHash != beforeTaskHash[i] {
					t.Fatalf("task %d Seeds edit changed other task %d ConfigHash from %q to %q", target, i, beforeTaskHash[i], task.ConfigHash)
				}
			}
			t.Logf("Seeds control preserved: task %d seed edit left caller and other task reports unchanged", target)

			suite.Tasks[target].Config.Policies[0] = Nav2DRewired

			if got := ownershipJSON(t, config); !bytes.Equal(got, beforeCallerJSON) {
				t.Fatalf("task %d element edits changed the caller config JSON", target)
			}
			for i, task := range suite.Tasks {
				if i == target {
					continue
				}
				if got := ownershipJSON(t, task); !bytes.Equal(got, beforeTaskJSON[i]) {
					t.Errorf("task %d element edits changed other task %d complete report JSON", target, i)
				}
				if task.ConfigHash != beforeTaskHash[i] {
					t.Errorf("task %d element edits changed other task %d ConfigHash from %q to %q", target, i, beforeTaskHash[i], task.ConfigHash)
				}
			}
		})
	}
}

func TestNav2DReportConfigAppendOwnership(t *testing.T) {
	config := ownershipNav2DConfig()
	report, err := RunNav2D(context.Background(), config)
	if err != nil {
		t.Fatalf("RunNav2D: %v", err)
	}
	assertOwnershipNav2DReport(t, report)

	report.Config.Seeds = append(report.Config.Seeds, 5011)
	report.Config.Policies = append(report.Config.Policies, Nav2DRecurrent)
	config.Seeds = append(config.Seeds, 5013)
	config.Policies = append(config.Policies, Nav2DRewired)

	if got := report.Config.Seeds[2]; got != 5011 {
		t.Errorf("report appended seed = %d, want 5011 after caller append", got)
	}
	if got := report.Config.Policies[2]; got != Nav2DRecurrent {
		t.Errorf("report appended policy = %q, want %q after caller append", got, Nav2DRecurrent)
	}
	if got := config.Seeds[2]; got != 5013 {
		t.Errorf("caller appended seed = %d, want 5013 after report append", got)
	}
	if got := config.Policies[2]; got != Nav2DRewired {
		t.Errorf("caller appended policy = %q, want %q after report append", got, Nav2DRewired)
	}
}
