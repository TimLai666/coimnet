package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// evaluationReport is the slice of an evaluation report JSON the CLI tests read
// back from the --out file. It mirrors experiment.EvaluationReport for the
// fields these tests assert.
type evaluationReport struct {
	SchemaVersion   string `json:"schema_version"`
	Mode            string `json:"mode"`
	AdaptationItems int    `json:"adaptation_items"`
	ScoringItems    int    `json:"scoring_items"`
	Contamination   struct {
		ParametersUnchanged bool  `json:"parameters_unchanged"`
		ShuffleInvariant    *bool `json:"shuffle_invariant"`
	} `json:"contamination_checks"`
}

// runEvaluateExample runs `examples run evaluate` and returns the summary line
// and the report decoded from the --out file.
func runEvaluateExample(t *testing.T, args ...string) (string, evaluationReport) {
	t.Helper()
	outArg := ""
	for i := range args {
		if args[i] == "--out" && i+1 < len(args) {
			outArg = args[i+1]
		}
	}
	var stdout, stderr bytes.Buffer
	full := append([]string{"examples", "run", "evaluate"}, args...)
	if err := Run(context.Background(), full, &stdout, &stderr); err != nil {
		t.Fatalf("%v: %v; stderr=%s", full, err, stderr.String())
	}
	data, err := os.ReadFile(outArg)
	if err != nil {
		t.Fatalf("read --out %s: %v", outArg, err)
	}
	var report evaluationReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("report is not parseable JSON: %v\n%s", err, data)
	}
	return stdout.String(), report
}

func TestEvaluateFixedWritesReport(t *testing.T) {
	out := filepath.Join(t.TempDir(), "fixed.json")
	stdout, report := runEvaluateExample(t, "--mode", "fixed", "--reset", "neural,plastic,chemical", "--out", out)

	if report.SchemaVersion != "coimnet-adaptive-evaluation/v1" {
		t.Fatalf("schema_version = %q", report.SchemaVersion)
	}
	if report.Mode != "fixed" {
		t.Fatalf("mode = %q", report.Mode)
	}
	if report.ScoringItems != 8 {
		t.Fatalf("scoring_items = %d, want 8", report.ScoringItems)
	}
	if report.AdaptationItems != 0 {
		t.Fatalf("adaptation_items = %d, want 0 in fixed mode", report.AdaptationItems)
	}
	if !report.Contamination.ParametersUnchanged {
		t.Fatal("parameters_unchanged = false, want true")
	}
	if report.Contamination.ShuffleInvariant == nil || !*report.Contamination.ShuffleInvariant {
		t.Fatalf("shuffle_invariant = %v, want non-nil true", report.Contamination.ShuffleInvariant)
	}
	for _, want := range []string{"mode=fixed", "scoring=8", "parameters_unchanged=true", "shuffle_invariant=true", "out=" + out} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("summary missing %q:\n%s", want, stdout)
		}
	}
}

func TestEvaluateAdaptiveWritesReport(t *testing.T) {
	out := filepath.Join(t.TempDir(), "adaptive.json")
	stdout, report := runEvaluateExample(t, "--mode", "adaptive", "--out", out)

	if report.SchemaVersion != "coimnet-adaptive-evaluation/v1" {
		t.Fatalf("schema_version = %q", report.SchemaVersion)
	}
	if report.Mode != "adaptive" {
		t.Fatalf("mode = %q", report.Mode)
	}
	if report.AdaptationItems != 8 {
		t.Fatalf("adaptation_items = %d, want 8", report.AdaptationItems)
	}
	if report.ScoringItems != 8 {
		t.Fatalf("scoring_items = %d, want 8", report.ScoringItems)
	}
	if !report.Contamination.ParametersUnchanged {
		t.Fatal("parameters_unchanged = false, want true")
	}
	for _, want := range []string{"mode=adaptive", "scoring=8", "parameters_unchanged=true", "shuffle_invariant=n/a", "out=" + out} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("summary missing %q:\n%s", want, stdout)
		}
	}
}

func TestEvaluateRejectsUnknownMode(t *testing.T) {
	out := filepath.Join(t.TempDir(), "unknown-mode.json")
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"examples", "run", "evaluate", "--mode", "foo", "--out", out}, &stdout, &stderr)
	if err == nil {
		t.Fatal("accepted unknown --mode")
	}
	if code := ExitCode(err); code != 1 {
		t.Fatalf("exit code = %d, want 1: %v", code, err)
	}
	if !strings.Contains(err.Error(), "mode") {
		t.Fatalf("error %q does not name --mode", err)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatal("an unknown mode produced an output file")
	}
}

func TestEvaluateRefusesExistingOut(t *testing.T) {
	out := filepath.Join(t.TempDir(), "existing.json")
	if err := os.WriteFile(out, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"examples", "run", "evaluate", "--mode", "fixed", "--out", out}, &stdout, &stderr)
	if err == nil {
		t.Fatal("an existing --out path was overwritten")
	}
	if code := ExitCode(err); code != 1 {
		t.Fatalf("exit code = %d, want 1: %v", code, err)
	}
	if !strings.Contains(err.Error(), "exists") {
		t.Fatalf("error %q does not report the existing path", err)
	}
}

func TestEvaluateHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "run", "evaluate", "--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("help: %v %s", err, stderr.String())
	}
	for _, want := range []string{"--mode", "--out", "--seed", "--reset"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("help missing %q:\n%s", want, stdout.String())
		}
	}
}
