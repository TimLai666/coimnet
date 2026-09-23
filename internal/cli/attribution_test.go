package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/experiment"
)

func TestExamplesRunAttributionWritesReport(t *testing.T) {
	out := filepath.Join(t.TempDir(), "a.json")
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{
		"examples", "run", "attribution", "--seeds", "1,2,3", "--episodes", "2",
		"--hidden", "8", "--recurrent", "2", "--eval", "2", "--resamples", "100", "--out", out,
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run attribution: %v; stderr=%s", err, stderr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var report struct {
		SchemaVersion string `json:"schema_version"`
		Runs          []any  `json:"runs"`
		Comparisons   []any  `json:"comparisons"`
		Conclusion    string `json:"conclusion"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.SchemaVersion != "coimnet-attribution/v1" {
		t.Errorf("schema_version = %q", report.SchemaVersion)
	}
	if len(report.Runs) != 21 {
		t.Errorf("runs = %d, want 21", len(report.Runs))
	}
	if len(report.Comparisons) != 12 {
		t.Errorf("comparisons = %d, want 12", len(report.Comparisons))
	}
	if report.Conclusion != experiment.AttributionConclusion {
		t.Errorf("conclusion = %q, want %q", report.Conclusion, experiment.AttributionConclusion)
	}
	wantSummary := fmt.Sprintf("attribution: 7 groups x 3 seeds, %d failed runs; conclusion: %s\n", 0, experiment.AttributionConclusion)
	if stdout.String() != wantSummary {
		t.Errorf("stdout summary = %q, want %q", stdout.String(), wantSummary)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestExamplesRunAttributionGroupsSubset(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{
		"examples", "run", "attribution", "--seeds", "1,2,3", "--episodes", "2",
		"--hidden", "8", "--recurrent", "2", "--eval", "2", "--resamples", "100",
		"--groups", "normal,core_only",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run attribution subset: %v; stderr=%s", err, stderr.String())
	}
	var report struct {
		Runs        []any `json:"runs"`
		Comparisons []any `json:"comparisons"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode report from stdout: %v", err)
	}
	if len(report.Runs) != 6 {
		t.Errorf("runs = %d, want 6", len(report.Runs))
	}
	if len(report.Comparisons) != 2 {
		t.Errorf("comparisons = %d, want 2", len(report.Comparisons))
	}
	if !strings.Contains(stderr.String(), "attribution: 2 groups x 3 seeds, 0 failed runs; conclusion: "+experiment.AttributionConclusion) {
		t.Errorf("stderr summary missing: %s", stderr.String())
	}
}

func TestExamplesRunAttributionRejects(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "existing.json")
	if err := os.WriteFile(existing, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		args       []string
		wantPrefix string
	}{
		{name: "too few seeds", args: []string{"--seeds", "1,2"}},
		{name: "groups lack normal", args: []string{"--groups", "core_only"}},
		{name: "unknown group", args: []string{"--groups", "normal,nope"}},
		{name: "invalid interval", args: []string{"--interval", "1"}},
		{name: "too few resamples", args: []string{"--resamples", "50"}},
		{name: "zero eval episodes", args: []string{"--eval", "0"}, wantPrefix: "--eval:"},
		{name: "zero training episodes", args: []string{"--episodes", "0"}, wantPrefix: "--episodes:"},
		{name: "existing output", args: []string{"--out", existing}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := append([]string{"examples", "run", "attribution"}, tc.args...)
			err := Run(context.Background(), args, &stdout, &stderr)
			if err == nil {
				t.Fatal("expected usage error")
			}
			if code := ExitCode(err); code != exitUsage {
				t.Fatalf("exit code = %d, want %d: %v", code, exitUsage, err)
			}
			if len(tc.args) > 0 && strings.HasPrefix(tc.args[0], "--") && !strings.Contains(err.Error(), tc.args[0]) {
				t.Errorf("error %q does not name flag %q", err, tc.args[0])
			}
			if tc.wantPrefix != "" && !strings.HasPrefix(err.Error(), tc.wantPrefix) {
				t.Errorf("error = %q, want prefix %q", err, tc.wantPrefix)
			}
		})
	}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "run", "attribution", "--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("help: %v", err)
	}
	if !strings.Contains(stdout.String(), "Usage: coimnet examples run attribution") || !strings.Contains(stdout.String(), "core_only") {
		t.Fatalf("help missing Usage or core_only:\n%s", stdout.String())
	}
	for _, r := range stdout.String() {
		if (r >= '\u4e00' && r <= '\u9fff') || (r >= '\u3000' && r <= '\u303f') || (r >= '\uff00' && r <= '\uffef') {
			t.Fatalf("help contains CJK/full-width rune U+%04X:\n%s", r, stdout.String())
		}
	}
}

func TestExamplesHelpWrapsAttributionOverview(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("help: %v; stderr=%s", err, stderr.String())
	}
	lines := strings.Split(stdout.String(), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "Run the seven-group synthetic navigation" {
			continue
		}
		if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) == "attribution matrix" &&
			len(line)-len(strings.TrimLeft(line, " ")) == len(lines[i+1])-len(strings.TrimLeft(lines[i+1], " ")) {
			return
		}
	}
	t.Fatalf("attribution overview description is not wrapped as two aligned lines:\n%s", stdout.String())
}

func TestExamplesListIncludesAttribution(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "list"}, &stdout, &stderr); err != nil {
		t.Fatalf("examples list: %v; stderr=%s", err, stderr.String())
	}
	var entries []struct {
		Name        string `json:"name"`
		Profile     string `json:"profile"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &entries); err != nil {
		t.Fatalf("decode examples list: %v", err)
	}
	for _, entry := range entries {
		if entry.Name == "attribution" {
			if entry.Profile != "fixture" || entry.Description == "" {
				t.Fatalf("attribution entry = %+v, want fixture profile and description", entry)
			}
			return
		}
	}
	t.Fatal("examples list has no attribution entry")
}
