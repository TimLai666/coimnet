package media

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRolesEvidence writes a full TSK-07 role report and a compact per-seed summary.
// Reproduce with: COIMNET_TSK07_EVIDENCE=$PWD/evidence/TSK-07 go test -count=1 -timeout 0 -v -run '^TestRolesEvidence$' ./tasks/media/.
func TestRolesEvidence(t *testing.T) {
	dir := os.Getenv("COIMNET_TSK07_EVIDENCE")
	if dir == "" {
		t.Skip("COIMNET_TSK07_EVIDENCE is not set")
	}
	if !filepath.IsAbs(dir) {
		t.Fatal("COIMNET_TSK07_EVIDENCE must be an absolute path")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create evidence directory: %v", err)
	}
	report, err := RunRoles(context.Background(), DefaultRolesConfig())
	if err != nil {
		t.Fatalf("run role evidence: %v", err)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal role report: %v", err)
	}
	if err := writeRolesEvidence(filepath.Join(dir, "roles.json"), append(data, '\n')); err != nil {
		t.Fatal(err)
	}
	var summary strings.Builder
	summary.WriteString("seed\tmode\trole\tupdates\tcore_measure\tcore_seen_before\tcore_seen_after\tcore_held_out_before\tcore_held_out_after\ttool_seen\ttool_held_out\n")
	var failedSeed string
	for _, run := range report.Runs {
		for _, mode := range []RoleMode{run.CoreGenerated, run.FixedDecoder, run.ExternalTool} {
			toolSeen, toolHeldOut := "-", "-"
			if mode.ToolResult != nil {
				toolSeen = fmt.Sprint(mode.ToolResult.SeenCorrect)
				toolHeldOut = fmt.Sprint(mode.ToolResult.HeldOutCorrect)
			}
			fmt.Fprintf(&summary, "%d\t%s\t%s\t%d\t%s\t%d\t%d\t%d\t%d\t%s\t%s\n",
				run.Seed, mode.Mode, roleForMode(mode.Mode), mode.Updates, mode.CoreCapability.Measure,
				mode.CoreBefore.SeenCorrect, mode.CoreCapability.SeenCorrect,
				mode.CoreBefore.HeldOutCorrect, mode.CoreCapability.HeldOutCorrect, toolSeen, toolHeldOut)
		}
		counts, err := json.Marshal(run.ExternalTool.ToolCounts)
		if err != nil {
			t.Fatalf("marshal tool counts for seed %d: %v", run.Seed, err)
		}
		fmt.Fprintf(&summary, "seed=%d decoder_alone_correct=%d decoder_alone_reconstruction_mse=%.12g latent_mean_abs_error=%.12g tool_counts=%s\n",
			run.Seed, run.DecoderAlone.Correct, run.DecoderAlone.ReconstructionMSE, run.LatentStats.MeanAbsError, counts)
		t.Logf("seed=%d decoder_alone_correct=%d decoder_alone_reconstruction_mse=%.12g latent_mean_abs_error=%.12g tool_counts=%s",
			run.Seed, run.DecoderAlone.Correct, run.DecoderAlone.ReconstructionMSE, run.LatentStats.MeanAbsError, counts)
		if run.Failed && failedSeed == "" {
			failedSeed = fmt.Sprintf("seed %d failed: %s", run.Seed, run.Error)
		}
	}
	if err := writeRolesEvidence(filepath.Join(dir, "summary.txt"), []byte(summary.String())); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSuffix(summary.String(), "\n"), "\n") {
		t.Logf("%s", line)
	}
	if failedSeed != "" {
		t.Fatal(failedSeed)
	}
}

func writeRolesEvidence(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create evidence file %q: %w", path, err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write evidence file %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close evidence file %q: %w", path, err)
	}
	return nil
}

func roleForMode(mode string) string {
	switch mode {
	case "core_generated":
		return "core"
	case "fixed_decoder":
		return "fixed_decoder"
	case "external_tool":
		return "external_tool"
	default:
		return ""
	}
}
