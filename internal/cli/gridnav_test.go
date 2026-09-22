package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strings"
	"testing"
)

// gridnavPPOReport is the slice of the gridnav PPO report the CLI tests read
// back from stdout. It mirrors experiment.PPOExperimentReport for the fields
// these tests assert, so a field rename shows up here as a failure.
type gridnavPPOReport struct {
	Passed  bool `json:"passed"`
	Results []struct {
		Seed   uint64 `json:"seed"`
		Failed bool   `json:"failed"`
	} `json:"results"`
	Config struct {
		Updates int `json:"updates"`
	} `json:"config"`
	Baseline float64 `json:"random_policy_mean_return"`
}

// gridnavImitationReport mirrors experiment.ImitationReport for the fields
// these tests assert.
type gridnavImitationReport struct {
	SchemaVersion string `json:"schema_version"`
	Config        struct {
		Episodes int `json:"episodes"`
	} `json:"config"`
	Results []struct {
		Seed   uint64 `json:"seed"`
		Failed bool   `json:"failed"`
	} `json:"results"`
	Baseline float64 `json:"random_policy_mean_return"`
}

func TestGridnavHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "run", "gridnav", "--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("help: %v %s", err, stderr.String())
	}
	for _, want := range []string{
		"Usage: coimnet examples run gridnav",
		"--method",
		"--updates",
		`default "ppo"`,
		"default 200",
		"Example:",
		"Errors:",
		"Options:",
		"synthetic",
		"not a claim about navigation",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("help does not mention %q:\n%s", want, stdout.String())
		}
	}
}

func TestGridnavListAndTopOverview(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "list"}, &stdout, &stderr); err != nil {
		t.Fatalf("examples list: %v %s", err, stderr.String())
	}
	var entries []map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &entries); err != nil {
		t.Fatalf("examples list is not parseable JSON: %v\n%s", err, stdout.String())
	}
	var found map[string]string
	for _, e := range entries {
		if e["name"] == "gridnav" {
			found = e
		}
	}
	if found == nil {
		t.Fatalf("examples list has no gridnav entry:\n%s", stdout.String())
	}
	if found["profile"] != "fixture" {
		t.Fatalf("gridnav profile = %q, want %q", found["profile"], "fixture")
	}
	if found["description"] == "" {
		t.Fatalf("gridnav entry has no description: %v", found)
	}

	stdout.Reset()
	stderr.Reset()
	if err := Run(context.Background(), nil, &stdout, &stderr); err != nil {
		t.Fatalf("top-level help: %v %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "examples run gridnav") {
		t.Fatalf("top-level overview does not mention gridnav:\n%s", stdout.String())
	}
}

func TestGridnavRejects(t *testing.T) {
	for _, args := range [][]string{
		{"examples", "run", "gridnav", "--method", "flappy"},
		{"examples", "run", "gridnav", "--method", "ppo", "--updates", "0"},
		{"examples", "run", "gridnav", "--method", "ppo", "--updates", "100001"},
		{"examples", "run", "gridnav", "--method", "ppo", "--updates", "-1"},
		{"examples", "run", "gridnav", "--method", "ppo", "--updates", "oops"},
		{"examples", "run", "gridnav", "--method", "ppo", "extra"},
		{"examples", "run", "gridnav", "--method", "ppo", "--updates", "1", "extra"},
		{"examples", "run", "gridnav", "--typo"},
	} {
		var stdout, stderr bytes.Buffer
		if Run(context.Background(), args, &stdout, &stderr) == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestGridnavHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, args := range [][]string{
		{"examples", "run", "gridnav", "--method", "imitation", "--updates", "1"},
		{"examples", "run", "gridnav", "--method", "ppo", "--updates", "1"},
	} {
		var stdout, stderr bytes.Buffer
		if err := Run(ctx, args, &stdout, &stderr); !errors.Is(err, context.Canceled) {
			t.Fatalf("%v: error = %v, want context.Canceled", args, err)
		}
	}
}

func TestGridnavImitationLowBudgetWritesJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "run", "gridnav", "--method", "imitation", "--updates", "1"}, &stdout, &stderr); err != nil {
		t.Fatalf("imitation low budget: %v; stderr=%s", err, stderr.String())
	}
	var report gridnavImitationReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("imitation report is not parseable JSON: %v\n%s", err, stdout.String())
	}
	if report.SchemaVersion != "coimnet-imitation/v1" {
		t.Fatalf("schema_version = %q, want %q", report.SchemaVersion, "coimnet-imitation/v1")
	}
	if report.Config.Episodes != 1 {
		t.Fatalf("config.episodes = %d, want 1", report.Config.Episodes)
	}
	if len(report.Results) != 3 {
		t.Fatalf("results = %d, want 3", len(report.Results))
	}
	for _, r := range report.Results {
		if r.Failed {
			t.Fatalf("seed %d failed under the one-episode budget", r.Seed)
		}
	}
	if math.IsNaN(report.Baseline) || math.IsInf(report.Baseline, 0) {
		t.Fatalf("random_policy_mean_return = %v, want finite", report.Baseline)
	}
}

func TestGridnavPPOLowBudgetWritesFullJSONEvenWhenGateFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"examples", "run", "gridnav", "--method", "ppo", "--updates", "1"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("one PPO update should not meet the learning gate")
	}
	var report gridnavPPOReport
	if e := json.Unmarshal(stdout.Bytes(), &report); e != nil {
		t.Fatalf("failing run did not print a complete report: %v\n%s", e, stdout.String())
	}
	if report.Passed {
		t.Fatal("failing run reported passed=true")
	}
	if report.Config.Updates != 1 {
		t.Fatalf("config.updates = %d, want 1", report.Config.Updates)
	}
	if len(report.Results) != 3 {
		t.Fatalf("results = %d, want 3", len(report.Results))
	}
}

func TestGridnavWriterErrorsPropagate(t *testing.T) {
	writeErr := errors.New("gridnav output failed")
	testcases := []struct {
		args []string
	}{
		{[]string{"examples", "run", "gridnav", "--help"}},
		{[]string{"examples", "run", "gridnav", "--method", "imitation", "--updates", "1"}},
		{[]string{"examples", "run", "gridnav", "--method", "ppo", "--updates", "1"}},
	}
	for _, tc := range testcases {
		err := Run(context.Background(), tc.args, failingWriter{err: writeErr}, io.Discard)
		if !errors.Is(err, writeErr) {
			t.Errorf("Run(%v) error = %v, want output write error", tc.args, err)
		}
	}
	for _, tc := range testcases {
		if err := Run(context.Background(), tc.args, shortWriter{}, io.Discard); !errors.Is(err, io.ErrShortWrite) {
			t.Errorf("Run(%v) error = %v, want io.ErrShortWrite", tc.args, err)
		}
	}
}
