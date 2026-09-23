package textgen

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTextgenEvidence writes the TSK-03 fixture report and summary to $COIMNET_TSK03_EVIDENCE.
//
// Reproduction command (from repository root):
//
//	COIMNET_TSK03_EVIDENCE=$PWD/evidence/TSK-03 go test -count=1 -timeout 0 -v -run '^TestTextgenEvidence$' ./tasks/textgen/
func TestTextgenEvidence(t *testing.T) {
	dir := os.Getenv("COIMNET_TSK03_EVIDENCE")
	if dir == "" {
		t.Skip("COIMNET_TSK03_EVIDENCE is not set")
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("COIMNET_TSK03_EVIDENCE must be an absolute path: %q", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	config := DefaultRunConfig()
	report, err := RunTextgen(context.Background(), config)
	if err != nil {
		t.Fatalf("RunTextgen: %v", err)
	}

	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	writeEvidenceFile(t, filepath.Join(dir, "textgen.json"), append(jsonData, '\n'))

	var summary strings.Builder
	summary.WriteString("seed updates perplexity_before perplexity_after task_accuracy_before task_accuracy_after validity_after teacher_calls core_disconnect_max_abs_delta\n")
	for _, run := range report.Runs {
		_, _ = fmt.Fprintf(&summary, "%d %d %.6f %.6f %.6f %.6f %.6f %d %.12g\n",
			run.Seed, run.Updates, run.HoldoutBefore.Perplexity, run.HoldoutAfter.Perplexity,
			run.TaskBefore.Accuracy, run.TaskAfter.Accuracy, run.TaskAfter.Validity,
			run.TeacherCalls, run.CoreDisconnect.MaxAbsDelta)
	}
	_, _ = fmt.Fprintf(&summary, "split train_sources=%s test_sources=%s\n", strings.Join(report.Split.TrainSources, ","), strings.Join(report.Split.TestSources, ","))
	_, _ = fmt.Fprintf(&summary, "teacher offered=%d removed=%d kept=%d\n", report.Teacher.Offered, report.Teacher.Removed, report.Teacher.Kept)
	writeEvidenceFile(t, filepath.Join(dir, "summary.txt"), []byte(summary.String()))
	t.Logf("%s", summary.String())
	for _, run := range report.Runs {
		if run.Failed {
			t.Fatalf("run failed for seed %d: %s", run.Seed, run.Error)
		}
		if run.TeacherCalls != 0 {
			t.Fatalf("teacher called for seed %d: %d", run.Seed, run.TeacherCalls)
		}
	}
}

func writeEvidenceFile(t *testing.T, path string, contents []byte) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		t.Fatalf("write %s: %v", path, err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}
}
