package experiment

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMultiTaskEvidence runs all three TSK-09 schedules and writes their reports.
//
// Reproduce from the repository root with:
//
//	COIMNET_TSK09_EVIDENCE=$PWD/evidence/TSK-09 go test -count=1 -timeout 0 -v -run '^TestMultiTaskEvidence$' ./experiment/
func TestMultiTaskEvidence(t *testing.T) {
	dir := os.Getenv("COIMNET_TSK09_EVIDENCE")
	if dir == "" {
		t.Skip("COIMNET_TSK09_EVIDENCE is not set")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	jsonPath := filepath.Join(dir, "multitask.json")
	summaryPath := filepath.Join(dir, "summary.txt")
	for _, path := range []string{jsonPath, summaryPath} {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("%s already exists", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", path, err)
		}
	}

	runs := make([]MultiTaskReport, 0, 3)
	for _, schedule := range []string{MultiTaskInterleaved, MultiTaskSameExperience, MultiTaskMissingModality} {
		c := DefaultMultiTaskConfig()
		c.Schedule.Schedule = schedule
		if schedule != MultiTaskMissingModality {
			c.Schedule.MissingEvery = 0
		}
		report, err := RunMultiTask(context.Background(), c)
		if err != nil {
			t.Fatalf("RunMultiTask(%s): %v", schedule, err)
		}
		if !report.SameBrain || report.TrainersBuilt != 1 {
			t.Fatalf("schedule %s: same_brain=%t trainers_built=%d", schedule, report.SameBrain, report.TrainersBuilt)
		}
		runs = append(runs, report)
	}
	suite := struct {
		SchemaVersion string            `json:"schema_version"`
		Runs          []MultiTaskReport `json:"runs"`
	}{SchemaVersion: "coimnet-multitask-suite/v1", Runs: runs}
	data, err := json.MarshalIndent(suite, "", "  ")
	if err != nil {
		t.Fatalf("marshal reports: %v", err)
	}
	if err := writeMultiTaskEvidence(jsonPath, append(data, '\n')); err != nil {
		t.Fatal(err)
	}

	var summary strings.Builder
	summary.WriteString("schedule task updates share declared flagged before after\n")
	for _, report := range runs {
		for i, share := range report.Shares {
			result := report.Results[i]
			summary.WriteString(fmt.Sprintf("%s %s %d %.6f %.6f %t %.6f %.6f\n",
				report.Config.Schedule.Schedule, share.Task, share.Updates, share.Share,
				share.Declared, share.Flagged, result.Before, result.After))
			t.Logf("schedule=%s task=%s updates=%d share=%.6f declared=%.6f flagged=%t before=%.6f after=%.6f",
				report.Config.Schedule.Schedule, share.Task, share.Updates, share.Share,
				share.Declared, share.Flagged, result.Before, result.After)
		}
		first := report.Results[0]
		summary.WriteString(fmt.Sprintf("%s trainers_built=%d same_brain=%t brain_topology_hash=%s base_parameter_hash=%s\n",
			report.Config.Schedule.Schedule, report.TrainersBuilt, report.SameBrain,
			first.BrainTopologyHash, first.BaseParameterHash))
		t.Logf("schedule=%s trainers_built=%d same_brain=%t brain_topology_hash=%s base_parameter_hash=%s",
			report.Config.Schedule.Schedule, report.TrainersBuilt, report.SameBrain,
			first.BrainTopologyHash, first.BaseParameterHash)
	}
	if err := writeMultiTaskEvidence(summaryPath, []byte(summary.String())); err != nil {
		t.Fatal(err)
	}
}

// writeMultiTaskEvidence creates one evidence file without replacing an existing result.
func writeMultiTaskEvidence(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}
