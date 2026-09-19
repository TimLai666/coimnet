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

func TestBenchmarkWritesReport(t *testing.T) {
	out := filepath.Join(t.TempDir(), "b.json")
	var stdout, stderr bytes.Buffer
	args := []string{"benchmark", "--nodes", "8", "--edges", "16", "--steps", "10", "--repeat", "1", "--out", out}
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("benchmark: %v %s", err, stderr.String())
	}
	if want := "benchmark: 6 stages, nodes 8, edges 16, steps 10, repeat 1"; !strings.Contains(stdout.String(), want) {
		t.Fatalf("stdout summary missing %q:\n%s", want, stdout.String())
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("report file: %v", err)
	}
	var report struct {
		SchemaVersion string `json:"schema_version"`
		GoVersion     string `json:"go_version"`
		GOOS          string `json:"goos"`
		GOARCH        string `json:"goarch"`
		NumCPU        int    `json:"num_cpu"`
		Uptime        string `json:"uptime"`
		Config        struct {
			Nodes  int `json:"nodes"`
			Edges  int `json:"edges"`
			Steps  int `json:"steps"`
			Repeat int `json:"repeat"`
		} `json:"config"`
		Stages []struct {
			Name     string  `json:"name"`
			MedianMS float64 `json:"median_ms"`
			MinMS    float64 `json:"min_ms"`
			MaxMS    float64 `json:"max_ms"`
			Repeat   int     `json:"repeat"`
			RSSAfter float64 `json:"rss_mib_after"`
		} `json:"stages"`
		Assumptions []string `json:"assumptions"`
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("report is not parseable JSON: %v\n%s", err, raw)
	}
	if report.SchemaVersion != benchmarkSchemaVersion {
		t.Errorf("schema_version = %q, want %q", report.SchemaVersion, benchmarkSchemaVersion)
	}
	if report.GoVersion == "" {
		t.Error("go_version is empty")
	}
	if report.GOOS == "" || report.GOARCH == "" || report.NumCPU <= 0 {
		t.Errorf("runtime fields = %q %q %d", report.GOOS, report.GOARCH, report.NumCPU)
	}
	if report.Uptime == "" {
		t.Error("uptime is empty")
	}
	if report.Config.Nodes != 8 || report.Config.Edges != 16 || report.Config.Steps != 10 || report.Config.Repeat != 1 {
		t.Errorf("config = %+v", report.Config)
	}
	wantStages := []string{"import", "forward", "backward", "local_plasticity", "modulation", "snapshot"}
	if len(report.Stages) != len(wantStages) {
		t.Fatalf("stages = %+v", report.Stages)
	}
	for i, want := range wantStages {
		got := report.Stages[i]
		if got.Name != want {
			t.Errorf("stage %d name = %q, want %q", i, got.Name, want)
		}
		if got.MedianMS < 0 || got.MinMS < 0 || got.MaxMS < 0 {
			t.Errorf("stage %q negative timing: %+v", want, got)
		}
		if got.Repeat != 1 {
			t.Errorf("stage %q repeat = %d, want 1", got.Name, got.Repeat)
		}
		if got.RSSAfter < 0 {
			t.Errorf("stage %q rss_mib_after = %f", got.Name, got.RSSAfter)
		}
	}
	wantAssumptions := []string{"synthetic topology, not the MaleCNS graph", "wall-clock medians on a shared machine; see uptime"}
	if len(report.Assumptions) != len(wantAssumptions) {
		t.Fatalf("assumptions = %q", report.Assumptions)
	}
	for i, want := range wantAssumptions {
		if report.Assumptions[i] != want {
			t.Errorf("assumption %d = %q, want %q", i, report.Assumptions[i], want)
		}
	}
}

// TestBenchmarkSnapshotStageLeavesNoTempFile pins the snapshot stage's cleanup:
// the stage writes <out>.snapshot.tmp.json once per repetition and must remove
// it every time, so after the run the output directory holds the report alone.
func TestBenchmarkSnapshotStageLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "b.json")
	var stdout, stderr bytes.Buffer
	args := []string{"benchmark", "--nodes", "8", "--edges", "16", "--steps", "10", "--repeat", "2", "--out", out}
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("benchmark: %v %s", err, stderr.String())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read output directory: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if len(names) != 1 || names[0] != "b.json" {
		t.Fatalf("output directory = %q, want only the report file", names)
	}
}

func TestBenchmarkRejects(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "new.json")
	existing := filepath.Join(dir, "existing.json")
	if err := os.WriteFile(existing, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"benchmark"},
		{"benchmark", "--out", existing},
		{"benchmark", "--out", out, "--repeat", "0"},
		{"benchmark", "--out", out, "--nodes", "0"},
	} {
		var stdout, stderr bytes.Buffer
		err := Run(context.Background(), args, &stdout, &stderr)
		if err == nil {
			t.Fatalf("accepted %v", args)
		}
		if code := ExitCode(err); code != exitUsage {
			t.Fatalf("%v: exit code = %d, want %d: %v", args, code, exitUsage, err)
		}
	}

	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"benchmark", "--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("help: %v %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage: coimnet benchmark") {
		t.Fatalf("help missing usage line:\n%s", stdout.String())
	}
}
