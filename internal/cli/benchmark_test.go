package cli

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBenchmarkWritesReport(t *testing.T) {
	out := filepath.Join(t.TempDir(), "b.json")
	var stdout, stderr bytes.Buffer
	args := []string{"benchmark", "--nodes", "8", "--edges", "16", "--steps", "10", "--repeat", "2", "--out", out}
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("benchmark: %v %s", err, stderr.String())
	}
	if want := "benchmark: 6 stages, nodes 8, edges 16, steps 10, repeat 2, forward reproducible true"; !strings.Contains(stdout.String(), want) {
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
			Name           string          `json:"name"`
			InitMS         float64         `json:"init_ms"`
			TransferMS     float64         `json:"transfer_ms"`
			WarmupMS       float64         `json:"warmup_ms"`
			SteadyMedianMS float64         `json:"steady_median_ms"`
			SteadyMinMS    float64         `json:"steady_min_ms"`
			SteadyMaxMS    float64         `json:"steady_max_ms"`
			SteadyRepeat   int             `json:"steady_repeat"`
			RSSAfter       float64         `json:"rss_mib_after"`
			Activity       json.RawMessage `json:"activity"`
		} `json:"stages"`
		Reproducibility struct {
			ForwardDigestFirst  string  `json:"forward_digest_first"`
			ForwardDigestSecond string  `json:"forward_digest_second"`
			Identical           bool    `json:"identical"`
			SteadyRatio         float64 `json:"steady_ratio"`
		} `json:"reproducibility"`
		Energy struct {
			Measured bool   `json:"measured"`
			Note     string `json:"note"`
		} `json:"energy"`
		Assumptions []string `json:"assumptions"`
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("report is not parseable JSON: %v\n%s", err, raw)
	}
	if report.SchemaVersion != "coimnet-benchmark/v2" {
		t.Errorf("schema_version = %q, want coimnet-benchmark/v2", report.SchemaVersion)
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
	if report.Config.Nodes != 8 || report.Config.Edges != 16 || report.Config.Steps != 10 || report.Config.Repeat != 2 {
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
		if got.InitMS < 0 || got.TransferMS != 0 || got.WarmupMS < 0 || got.SteadyMedianMS < 0 || got.SteadyMinMS < 0 || got.SteadyMaxMS < 0 {
			t.Errorf("stage %q timings = %+v", want, got)
		}
		if got.SteadyRepeat != report.Config.Repeat-1 {
			t.Errorf("stage %q steady_repeat = %d, want %d", got.Name, got.SteadyRepeat, report.Config.Repeat-1)
		}
		if got.RSSAfter < 0 {
			t.Errorf("stage %q rss_mib_after = %f", got.Name, got.RSSAfter)
		}
	}
	if len(report.Stages) > 1 {
		if len(report.Stages[1].Activity) == 0 || string(report.Stages[1].Activity) == "null" {
			t.Fatal("forward activity is missing")
		}
		var activity struct {
			Rows            int      `json:"rows"`
			Nodes           int      `json:"nodes"`
			NonzeroFraction float64  `json:"nonzero_fraction"`
			MeanAbsOutput   float64  `json:"mean_abs_output"`
			SpikesPerStep   *float64 `json:"spikes_per_step"`
			MeanRate        *float64 `json:"mean_rate"`
			Digest          string   `json:"digest"`
		}
		if err := json.Unmarshal(report.Stages[1].Activity, &activity); err != nil {
			t.Fatalf("forward activity JSON: %v", err)
		}
		if activity.Rows != 10 || activity.Nodes != 8 || activity.NonzeroFraction < 0 || activity.NonzeroFraction > 1 || activity.MeanAbsOutput < 0 {
			t.Errorf("forward activity = %+v", activity)
		}
		if activity.SpikesPerStep != nil || activity.MeanRate != nil {
			t.Errorf("continuous-core spike metrics = %v, %v, want null", activity.SpikesPerStep, activity.MeanRate)
		}
		if decoded, err := hex.DecodeString(activity.Digest); err != nil || len(decoded) != 32 {
			t.Errorf("forward activity digest = %q, want 64 hexadecimal characters", activity.Digest)
		}
	}
	first, errFirst := hex.DecodeString(report.Reproducibility.ForwardDigestFirst)
	second, errSecond := hex.DecodeString(report.Reproducibility.ForwardDigestSecond)
	if errFirst != nil || errSecond != nil || len(first) != 32 || len(second) != 32 {
		t.Errorf("reproducibility digests = %q %q, want 64 hexadecimal characters", report.Reproducibility.ForwardDigestFirst, report.Reproducibility.ForwardDigestSecond)
	}
	if !report.Reproducibility.Identical || report.Reproducibility.ForwardDigestFirst != report.Reproducibility.ForwardDigestSecond {
		t.Errorf("reproducibility = %+v", report.Reproducibility)
	}
	if report.Reproducibility.SteadyRatio < 0 {
		t.Errorf("steady_ratio = %f", report.Reproducibility.SteadyRatio)
	}
	if report.Energy.Measured || report.Energy.Note != "no power measurement" {
		t.Errorf("energy = %+v", report.Energy)
	}
	if !contains(report.Assumptions, "continuous core: spike counts and firing rates do not apply") {
		t.Errorf("assumptions missing continuous-core activity statement: %q", report.Assumptions)
	}
	var values any
	if err := json.Unmarshal(raw, &values); err != nil {
		t.Fatal(err)
	}
	if key := energyMeasurementKey(values); key != "" {
		t.Errorf("report contains energy measurement key %q", key)
	}
}

func energyMeasurementKey(value any) string {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "joule") || strings.Contains(lower, "watt") {
				return key
			}
			if found := energyMeasurementKey(child); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range value {
			if found := energyMeasurementKey(child); found != "" {
				return found
			}
		}
	}
	return ""
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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
		{"benchmark", "--out", out, "--repeat", "1"},
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
		if strings.Contains(strings.Join(args, " "), "--repeat 1") && !strings.Contains(err.Error(), "warmup and steady") {
			t.Errorf("--repeat 1 error does not explain warmup and steady requirements: %v", err)
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
