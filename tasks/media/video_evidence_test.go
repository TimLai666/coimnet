package media

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type videoEvidencePackaging struct {
	Packager    map[string]PackageReport `json:"packager"`
	AbsentCheck PackageReport            `json:"absent_check"`
}

// TestVideoEvidence writes the complete TSK-06 fixture evidence when configured.
//
// Reproduce from the repository root with:
//
//	COIMNET_TSK06_EVIDENCE=$PWD/evidence/TSK-06 go test -count=1 -timeout 0 -v -run '^TestVideoEvidence$' ./tasks/media/
func TestVideoEvidence(t *testing.T) {
	dir := os.Getenv("COIMNET_TSK06_EVIDENCE")
	if dir == "" {
		t.Skip("COIMNET_TSK06_EVIDENCE is not set")
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("COIMNET_TSK06_EVIDENCE must be an absolute path")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create evidence directory: %v", err)
	}

	ctx := context.Background()
	config := DefaultVideoRunConfig()
	report, err := RunVideo(ctx, config)
	if err != nil {
		t.Fatalf("RunVideo: %v", err)
	}
	reportJSON, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal video report: %v", err)
	}
	if err := writeExclusive(filepath.Join(dir, "video.json"), append(reportJSON, '\n')); err != nil {
		t.Fatalf("write video.json: %v", err)
	}
	for _, run := range report.Runs {
		if run.Failed {
			t.Fatalf("seed %d failed: %s", run.Seed, run.Error)
		}
	}

	clipsDir := filepath.Join(dir, "clips")
	timelines, err := WriteVideoSamples(ctx, clipsDir, config, config.Seeds[0])
	if err != nil {
		t.Fatalf("WriteVideoSamples: %v", err)
	}
	clipNames := make([]string, 0, len(timelines))
	for name := range timelines {
		clipNames = append(clipNames, name)
	}
	sort.Strings(clipNames)
	packaging := videoEvidencePackaging{Packager: make(map[string]PackageReport, len(clipNames))}
	for _, name := range clipNames {
		result, err := PackageVideo(ctx, filepath.Join(clipsDir, name), "ffmpeg")
		if err != nil {
			t.Fatalf("PackageVideo %s: %v", name, err)
		}
		packaging.Packager[name] = result
	}
	if len(clipNames) == 0 {
		t.Fatal("WriteVideoSamples returned no clips")
	}
	packaging.AbsentCheck, err = PackageVideo(ctx, filepath.Join(clipsDir, clipNames[0]), "coimnet-missing-packager")
	if err != nil {
		t.Fatalf("PackageVideo absent check: %v", err)
	}
	if packaging.AbsentCheck.Status != "tool_absent" || packaging.AbsentCheck.Tool != "coimnet-missing-packager" {
		t.Fatalf("absent package report = %#v; want tool_absent", packaging.AbsentCheck)
	}
	packagingJSON, err := json.MarshalIndent(packaging, "", "  ")
	if err != nil {
		t.Fatalf("marshal packaging report: %v", err)
	}
	if err := writeExclusive(filepath.Join(dir, "packaging.json"), append(packagingJSON, '\n')); err != nil {
		t.Fatalf("write packaging.json: %v", err)
	}

	var summary strings.Builder
	summary.WriteString("seed group updates seen_correct_before seen_correct_after held_out_correct_before held_out_correct_after seen_in_sync\n")
	for _, run := range report.Runs {
		for _, group := range []VideoGroup{run.Core, run.FrozenCore} {
			fmt.Fprintf(&summary, "%d %s %d %d %d %d %d %d\n", run.Seed, group.Name, group.Updates,
				videoSeenCorrect(group.Before), group.SeenCorrect,
				videoHeldOutCorrect(group.Before), group.HeldOutCorrect, group.SeenInSync)
		}
		fmt.Fprintf(&summary, "%d core_disconnect changed=%t max_abs_delta=%.9g\n", run.Seed,
			run.CoreDisconnect.OutputChanged, run.CoreDisconnect.MaxAbsDelta)
	}
	firstRun := report.Runs[0]
	for _, name := range clipNames {
		score, ok := findVideoEvidenceScore(firstRun.Core.After, name)
		if !ok {
			t.Fatalf("first seed core report has no score for clip %s", name)
		}
		fmt.Fprintf(&summary, "clip %s judged=%q correct=%t path_matches=%d event_frame=%d sync_error=%d package_status=%s\n",
			name, score.Judged, score.Correct, score.PathMatches, score.EventFrame, score.SyncError, packaging.Packager[name].Status)
	}
	if err := writeExclusive(filepath.Join(dir, "summary.txt"), []byte(summary.String())); err != nil {
		t.Fatalf("write summary.txt: %v", err)
	}
	t.Logf("%s", summary.String())
}

func videoSeenCorrect(scores []VideoScore) int {
	count := 0
	for _, score := range scores {
		if score.Seen && score.Correct {
			count++
		}
	}
	return count
}

func videoHeldOutCorrect(scores []VideoScore) int {
	count := 0
	for _, score := range scores {
		if !score.Seen && score.Correct {
			count++
		}
	}
	return count
}

func findVideoEvidenceScore(scores []VideoScore, clip string) (VideoScore, bool) {
	for _, score := range scores {
		condition, err := ParseVideoPrompt(score.Prompt)
		if err == nil && condition.Direction+"_"+condition.Sound == clip {
			return score, true
		}
	}
	return VideoScore{}, false
}
