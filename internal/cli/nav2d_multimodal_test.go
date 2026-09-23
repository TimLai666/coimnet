package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var nav2DSummaryLine = regexp.MustCompile(`^nav2d: 4 tasks x 2 policies x 1 seeds, 0 failed runs\n$`)
var multimodalSummaryLine = regexp.MustCompile(`^multimodal: 1 seeds, seen image->text [0-9]+\.[0-9]+, unseen image->text [0-9]+\.[0-9]+, chance 0\.111111\n$`)

func TestExamplesRunNav2DWritesSuite(t *testing.T) {
	out := filepath.Join(t.TempDir(), "n.json")
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"examples", "run", "nav2d", "--task", "all", "--seeds", "1", "--episodes", "10", "--hidden", "8", "--recurrent", "2", "--eval", "3", "--policies", "recurrent,random", "--out", out}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("examples run nav2d: %v; stderr=%s", err, stderr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		SchemaVersion string `json:"schema_version"`
		Tasks         []struct {
			Runs []json.RawMessage `json:"runs"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("invalid JSON report: %v\n%s", err, data)
	}
	if report.SchemaVersion != "coimnet-nav2d-suite/v1" || len(report.Tasks) != 4 {
		t.Fatalf("schema/tasks = %q/%d", report.SchemaVersion, len(report.Tasks))
	}
	for i, task := range report.Tasks {
		if len(task.Runs) != 2 {
			t.Fatalf("tasks[%d].runs = %d, want 2", i, len(task.Runs))
		}
	}
	if !nav2DSummaryLine.MatchString(stdout.String()) || stderr.Len() != 0 {
		t.Fatalf("summary/output streams = %q / %q", stdout.String(), stderr.String())
	}
}

func TestExamplesRunNav2DSingleTask(t *testing.T) {
	out := filepath.Join(t.TempDir(), "single.json")
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"examples", "run", "nav2d", "--task", "avoid_obstacles", "--seeds", "1", "--episodes", "10", "--hidden", "8", "--recurrent", "2", "--eval", "3", "--policies", "random", "--out", out}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("examples run nav2d: %v; stderr=%s", err, stderr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("invalid JSON report: %v\n%s", err, data)
	}
	if report.SchemaVersion != "coimnet-nav2d/v1" {
		t.Fatalf("schema_version = %q", report.SchemaVersion)
	}
}

func TestExamplesRunNav2DRejects(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "existing.json")
	if err := os.WriteFile(existing, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--task", "unknown"},
		{"--policies", "unknown"},
		{"--seeds", ""},
		{"--out", existing},
	} {
		var stdout, stderr bytes.Buffer
		full := append([]string{"examples", "run", "nav2d"}, args...)
		err := Run(context.Background(), full, &stdout, &stderr)
		if err == nil || ExitCode(err) != exitUsage {
			t.Fatalf("%v: err=%v, exit=%d", args, err, ExitCode(err))
		}
	}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "run", "nav2d", "--help"}, &stdout, &stderr); err != nil || !strings.Contains(stdout.String(), "Usage: coimnet examples run nav2d") {
		t.Fatalf("help: err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
}

func TestExamplesRunMultimodalWritesReport(t *testing.T) {
	out := filepath.Join(t.TempDir(), "multimodal.json")
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"examples", "run", "multimodal", "--seeds", "1", "--epochs", "2", "--out", out}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("examples run multimodal: %v; stderr=%s", err, stderr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		SchemaVersion string `json:"schema_version"`
		Chance        struct {
			Label float64 `json:"label"`
		} `json:"chance"`
		Runs []json.RawMessage `json:"runs"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("invalid JSON report: %v\n%s", err, data)
	}
	if report.SchemaVersion != "coimnet-multimodal-eval/v1" || len(report.Runs) != 1 {
		t.Fatalf("schema/runs = %q/%d", report.SchemaVersion, len(report.Runs))
	}
	if report.Chance.Label < 0.111110 || report.Chance.Label > 0.111112 {
		t.Fatalf("chance.label = %v, want approximately 1/9", report.Chance.Label)
	}
	if !multimodalSummaryLine.MatchString(stdout.String()) || stderr.Len() != 0 {
		t.Fatalf("summary/output streams = %q / %q", stdout.String(), stderr.String())
	}
}

func TestExamplesRunMultimodalRejects(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "existing.json")
	if err := os.WriteFile(existing, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--seeds", ""},
		{"--epochs", "0"},
		{"--hidden", "1"},
		{"--out", existing},
	} {
		var stdout, stderr bytes.Buffer
		full := append([]string{"examples", "run", "multimodal"}, args...)
		err := Run(context.Background(), full, &stdout, &stderr)
		if err == nil || ExitCode(err) != exitUsage {
			t.Fatalf("%v: err=%v, exit=%d", args, err, ExitCode(err))
		}
	}
}

func TestExamplesListIncludesNav2DAndMultimodal(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "list"}, &stdout, &stderr); err != nil {
		t.Fatalf("examples list: %v", err)
	}
	var items []struct {
		Name        string `json:"name"`
		Profile     string `json:"profile"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &items); err != nil {
		t.Fatalf("invalid examples list: %v\n%s", err, stdout.String())
	}
	found := map[string]bool{}
	for _, item := range items {
		if item.Name == "nav2d" || item.Name == "multimodal" {
			if item.Profile != "fixture" || item.Description == "" {
				t.Errorf("%s has profile=%q description=%q", item.Name, item.Profile, item.Description)
			}
			found[item.Name] = true
		}
	}
	if !found["nav2d"] || !found["multimodal"] {
		t.Fatalf("missing nav2d/multimodal entries: %+v", found)
	}
}
