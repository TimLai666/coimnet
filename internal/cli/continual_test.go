package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/experiment"
)

// continualMatrixReport is the slice of a continual matrix report JSON the CLI
// tests read back from the --out file. It mirrors experiment.ContinualReport
// for the fields these tests assert.
type continualMatrixReport struct {
	SchemaVersion string `json:"schema_version"`
	Seeds         []struct {
		Seed     uint64      `json:"seed"`
		Switched [][]float64 `json:"switched"`
	} `json:"seeds"`
	Cells      [][]json.RawMessage `json:"cells"`
	Comparison *struct {
		Tasks []struct {
			Task string `json:"task"`
		} `json:"tasks"`
	} `json:"comparison"`
}

// runContinualMatrixExample runs `examples run continual-matrix` and returns
// the summary line and the report decoded from the --out file.
func runContinualMatrixExample(t *testing.T, args ...string) (string, continualMatrixReport) {
	t.Helper()
	outArg := ""
	for i := range args {
		if args[i] == "--out" && i+1 < len(args) {
			outArg = args[i+1]
		}
	}
	var stdout, stderr bytes.Buffer
	full := append([]string{"examples", "run", "continual-matrix"}, args...)
	if err := Run(context.Background(), full, &stdout, &stderr); err != nil {
		t.Fatalf("%v: %v; stderr=%s", full, err, stderr.String())
	}
	data, err := os.ReadFile(outArg)
	if err != nil {
		t.Fatalf("read --out %s: %v", outArg, err)
	}
	var report continualMatrixReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("report is not parseable JSON: %v\n%s", err, data)
	}
	return stdout.String(), report
}

func TestContinualMatrixWritesAReport(t *testing.T) {
	out := filepath.Join(t.TempDir(), "report.json")
	stdout, report := runContinualMatrixExample(t, "--budget", "2", "--episodes", "4", "--out", out)

	if report.SchemaVersion != experiment.ContinualSchemaVersion {
		t.Fatalf("schema_version = %q", report.SchemaVersion)
	}
	if len(report.Seeds) != 3 {
		t.Fatalf("seeds = %d, want 3", len(report.Seeds))
	}
	if len(report.Cells) != 3 {
		t.Fatalf("cells rows = %d, want 3", len(report.Cells))
	}
	for i := range report.Cells {
		if len(report.Cells[i]) != 2 {
			t.Fatalf("cells row %d has %d columns, want 2", i, len(report.Cells[i]))
		}
	}
	if report.Comparison == nil {
		t.Fatal("comparison is nil")
	}
	if len(report.Comparison.Tasks) != 2 {
		t.Fatalf("comparison tasks = %d, want 2", len(report.Comparison.Tasks))
	}
	if !strings.Contains(stdout, "continual-matrix:") {
		t.Fatalf("summary missing %q:\n%s", "continual-matrix:", stdout)
	}
}

func TestContinualMatrixWithChemistry(t *testing.T) {
	out := filepath.Join(t.TempDir(), "chemistry.json")
	_, report := runContinualMatrixExample(t, "--budget", "2", "--episodes", "4", "--chemistry", "--out", out)

	if len(report.Seeds) == 0 {
		t.Fatal("report has no seeds")
	}
	if len(report.Seeds[0].Switched) == 0 {
		t.Fatal("seeds[0].switched is empty with --chemistry")
	}
}

func TestContinualMatrixRejects(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "existing.json")
	if err := os.WriteFile(existing, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(t.TempDir(), "base.json")
	for _, args := range [][]string{
		{"--budget", "1", "--episodes", "1"},
		{"--budget", "1", "--episodes", "1", "--out", existing},
		{"--budget", "1", "--episodes", "1", "--out", base, "--seeds", "7,42"},
		{"--budget", "-1", "--episodes", "1", "--out", base},
		{"--budget", "1", "--episodes", "1", "--out", base, "extra"},
	} {
		var stdout, stderr bytes.Buffer
		full := append([]string{"examples", "run", "continual-matrix"}, args...)
		err := Run(context.Background(), full, &stdout, &stderr)
		if err == nil {
			t.Fatalf("accepted %v", args)
		}
		if code := ExitCode(err); code != exitUsage {
			t.Fatalf("%v: exit code = %d, want %d: %v", args, code, exitUsage, err)
		}
	}

	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "run", "continual-matrix", "--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("help: %v %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage: coimnet examples run continual-matrix") {
		t.Fatalf("help missing usage line:\n%s", stdout.String())
	}
}
