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

// TestAttributionEvidence runs the seven-group attribution protocol for ticket 28 / TSK-12
// and writes the JSON report and summary table to $COIMNET_TSK12_EVIDENCE.
//
// Reproduction command (from repository root):
//
//	COIMNET_TSK12_EVIDENCE=$PWD/evidence/TSK-12 go test -count=1 -timeout 0 -v -run '^TestAttributionEvidence$' ./experiment/
func TestAttributionEvidence(t *testing.T) {
	dir := os.Getenv("COIMNET_TSK12_EVIDENCE")
	if dir == "" {
		t.Skip("COIMNET_TSK12_EVIDENCE is not set")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	jsonPath := filepath.Join(dir, "attribution.json")
	if _, err := os.Stat(jsonPath); err == nil {
		t.Fatalf("%s already exists", jsonPath)
	}
	summaryPath := filepath.Join(dir, "summary.txt")
	if _, err := os.Stat(summaryPath); err == nil {
		t.Fatalf("%s already exists", summaryPath)
	}

	ctx := context.Background()
	cfg := DefaultAttributionConfig()
	report, err := RunAttribution(ctx, cfg)
	if err != nil {
		t.Fatalf("RunAttribution: %v", err)
	}

	for _, run := range report.Runs {
		if run.Failed {
			t.Fatalf("run failed: group %s seed %d: %s", run.Group, run.Seed, run.Error)
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
	sb.WriteString(fmt.Sprintf("%-28s %10s %15s %10s %16s %16s %16s %16s %16s %16s %16s %16s\n",
		"group", "parameters", "free_parameters", "updates",
		"before_success", "before_agree",
		"seen_success", "seen_agree",
		"unseen_success", "unseen_agree",
		"activity_before", "activity_after",
	))

	for _, group := range cfg.Groups {
		var runs []AttributionRun
		for _, r := range report.Runs {
			if r.Group == group {
				runs = append(runs, r)
			}
		}
		if len(runs) == 0 {
			continue
		}
		n := float64(len(runs))
		var updatesSum, beforeSR, beforeEA, seenSR, seenEA, unseenSR, unseenEA, actBefore, actAfter float64
		for _, r := range runs {
			updatesSum += float64(r.Updates)
			beforeSR += r.Before.SuccessRate
			beforeEA += r.Before.ExpertAgreement
			seenSR += r.Seen.SuccessRate
			seenEA += r.Seen.ExpertAgreement
			unseenSR += r.Unseen.SuccessRate
			unseenEA += r.Unseen.ExpertAgreement
			actBefore += r.ActivityBefore
			actAfter += r.ActivityAfter
		}
		sb.WriteString(fmt.Sprintf("%-28s %10d %15d %10.1f %16.4f %16.4f %16.4f %16.4f %16.4f %16.4f %16.4f %16.4f\n",
			group, runs[0].Model.Parameters, runs[0].Model.FreeParameters,
			updatesSum/n,
			beforeSR/n, beforeEA/n,
			seenSR/n, seenEA/n,
			unseenSR/n, unseenEA/n,
			actBefore/n, actAfter/n,
		))
	}

	sb.WriteString("\nGroup Comparisons:\n")
	for _, gc := range report.Comparisons {
		sb.WriteString(fmt.Sprintf("group: %-28s metric: %-24s mean: %+8.4f [%+8.4f, %+8.4f] dropped: %d\n",
			gc.Group, gc.Metric, gc.Mean, gc.Lower, gc.Upper, gc.Dropped))
	}

	sb.WriteString(fmt.Sprintf("\nConclusion: %s\n", report.Conclusion))

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
