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

// TestNav2DSuiteEvidence runs the navigation benchmark suite for ticket 28 / TSK-08
// and writes the JSON report and summary table to $COIMNET_TSK08_EVIDENCE.
//
// Reproduction command (from repository root):
//
//	COIMNET_TSK08_EVIDENCE=$PWD/evidence/TSK-08 go test -count=1 -timeout 0 -v -run '^TestNav2DSuiteEvidence$' ./experiment/
func TestNav2DSuiteEvidence(t *testing.T) {
	dir := os.Getenv("COIMNET_TSK08_EVIDENCE")
	if dir == "" {
		t.Skip("COIMNET_TSK08_EVIDENCE is not set")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	jsonPath := filepath.Join(dir, "nav2d-suite.json")
	if _, err := os.Stat(jsonPath); err == nil {
		t.Fatalf("%s already exists", jsonPath)
	}
	summaryPath := filepath.Join(dir, "summary.txt")
	if _, err := os.Stat(summaryPath); err == nil {
		t.Fatalf("%s already exists", summaryPath)
	}

	ctx := context.Background()
	cfg := DefaultNav2DConfig()
	report, err := RunNav2DSuite(ctx, cfg)
	if err != nil {
		t.Fatalf("RunNav2DSuite: %v", err)
	}

	for _, task := range report.Tasks {
		for _, run := range task.Runs {
			if run.Failed {
				t.Fatalf("run failed: task %s policy %s seed %d: %s", task.Task, run.Policy, run.Seed, run.Error)
			}
		}
	}

	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	jsonData = append(jsonData, '\n')
	jsonFile, err := os.OpenFile(jsonPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		t.Fatalf("create %s: %v", jsonPath, err)
	}
	if _, err := jsonFile.Write(jsonData); err != nil {
		jsonFile.Close()
		t.Fatalf("write %s: %v", jsonPath, err)
	}
	if err := jsonFile.Close(); err != nil {
		t.Fatalf("close %s: %v", jsonPath, err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-22s %-14s %10s %16s %16s %16s %16s %16s %16s %16s\n",
		"task", "policy", "updates",
		"before_success", "before_agree",
		"seen_success", "seen_agree",
		"unseen_success", "unseen_agree",
		"unseen_colls",
	))

	for _, task := range report.Tasks {
		for _, policy := range cfg.Policies {
			var runs []Nav2DRun
			for _, r := range task.Runs {
				if r.Policy == policy {
					runs = append(runs, r)
				}
			}
			if len(runs) == 0 {
				continue
			}
			n := float64(len(runs))
			var updatesSum, beforeSR, beforeEA, seenSR, seenEA, unseenSR, unseenEA, unseenColls float64
			for _, r := range runs {
				updatesSum += float64(r.Updates)
				beforeSR += r.Before.SuccessRate
				beforeEA += r.Before.ExpertAgreement
				seenSR += r.Seen.SuccessRate
				seenEA += r.Seen.ExpertAgreement
				unseenSR += r.Unseen.SuccessRate
				unseenEA += r.Unseen.ExpertAgreement
				unseenColls += r.Unseen.MeanCollisions
			}
			sb.WriteString(fmt.Sprintf("%-22s %-14s %10.1f %16.4f %16.4f %16.4f %16.4f %16.4f %16.4f %16.4f\n",
				task.Task, policy,
				updatesSum/n,
				beforeSR/n, beforeEA/n,
				seenSR/n, seenEA/n,
				unseenSR/n, unseenEA/n,
				unseenColls/n,
			))
		}
	}

	sb.WriteString("\nRewired Controls:\n")
	for _, task := range report.Tasks {
		for _, r := range task.Runs {
			if r.Policy == Nav2DRewired && r.Rewire != nil {
				rw := r.Rewire
				sb.WriteString(fmt.Sprintf("task: %-22s seed: %-3d edges: %-4d attempts: %-5d accepted: %-4d degrees_kept: %-5t duplicates: %-2d self_loops_before: %-2d self_loops_after: %-2d direction_kept: %-5t readout_reachable: %-5t\n",
					task.Task, r.Seed, rw.Edges, rw.Attempts, rw.Accepted, rw.DegreesKept, rw.Duplicates, rw.SelfLoopsBefore, rw.SelfLoopsAfter, rw.DirectionKept, rw.ReadoutReachable))
			}
		}
	}

	summaryData := []byte(sb.String())
	summaryFile, err := os.OpenFile(summaryPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		t.Fatalf("create %s: %v", summaryPath, err)
	}
	if _, err := summaryFile.Write(summaryData); err != nil {
		summaryFile.Close()
		t.Fatalf("write %s: %v", summaryPath, err)
	}
	if err := summaryFile.Close(); err != nil {
		t.Fatalf("close %s: %v", summaryPath, err)
	}

	t.Logf("%s", sb.String())
}
