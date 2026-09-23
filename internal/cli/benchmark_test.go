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
			Source         string `json:"source"`
			Nodes          int    `json:"nodes"`
			Edges          int    `json:"edges"`
			Steps          int    `json:"steps"`
			Repeat         int    `json:"repeat"`
			SnapshotFormat string `json:"snapshot_format"`
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
	if report.Config.Source != "synthetic" || report.Config.SnapshotFormat != "json" {
		t.Errorf("synthetic config source/snapshot_format = %q/%q", report.Config.Source, report.Config.SnapshotFormat)
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

func TestBenchmarkStoreMode(t *testing.T) {
	dir, store, rules := deriveFixture(t)
	store = addFullGraphFixtureClasses(t, dir)
	paramsPath, _ := deriveParameterSet(t, dir, store, rules, "benchmark-store-params.coimparams")
	protocol := writeCompareProtocol(t, dir, "benchmark-store-compare.json", fullGraphCompareProtocol())
	out := filepath.Join(t.TempDir(), "b.json")
	args := []string{"benchmark", "--store", store, "--params", paramsPath, "--protocol", protocol,
		"--input-set", "in", "--readout-set", "out", "--steps", "4", "--repeat", "2",
		"--max-memory-mib", "64", "--out", out}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("benchmark store mode: %v; stderr=%s", err, stderr.String())
	}
	if !strings.HasSuffix(strings.TrimSpace(stdout.String()), ", source store") {
		t.Fatalf("summary = %q, want suffix %q", stdout.String(), ", source store")
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("report file: %v", err)
	}
	var report struct {
		Config struct {
			Source         string            `json:"source"`
			Nodes          int               `json:"nodes"`
			Edges          int               `json:"edges"`
			Files          map[string]string `json:"files"`
			InputSet       string            `json:"input_set"`
			ReadoutSet     string            `json:"readout_set"`
			MaxMemoryMiB   int               `json:"max_memory_mib"`
			SnapshotFormat string            `json:"snapshot_format"`
		} `json:"config"`
		Assumptions []string `json:"assumptions"`
		Stages      []struct {
			Name string `json:"name"`
		} `json:"stages"`
		Memory json.RawMessage `json:"memory"`
		Energy struct {
			Measured bool `json:"measured"`
		} `json:"energy"`
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("report JSON: %v\n%s", err, raw)
	}
	if report.Config.Source != "store" || report.Config.Nodes != 2 || report.Config.Edges != 2 {
		t.Errorf("store config source/nodes/edges = %q/%d/%d", report.Config.Source, report.Config.Nodes, report.Config.Edges)
	}
	if report.Config.InputSet != "in" || report.Config.ReadoutSet != "out" || report.Config.MaxMemoryMiB != 64 || report.Config.SnapshotFormat != "bundle" {
		t.Errorf("store config sets/memory/snapshot = %q/%q/%d/%q", report.Config.InputSet, report.Config.ReadoutSet, report.Config.MaxMemoryMiB, report.Config.SnapshotFormat)
	}
	wantAssumption := "MaleCNS graph from --store with derived parameters from --params; the backward encoder reaches node 0 and the readout reads the last node, not the protocol's named sets"
	if len(report.Assumptions) != 3 || report.Assumptions[0] != wantAssumption {
		t.Errorf("store assumptions = %q", report.Assumptions)
	}
	if len(report.Config.Files) != 3 {
		t.Fatalf("files = %v, want three hashes", report.Config.Files)
	}
	for _, name := range []string{"store", "params", "protocol"} {
		digest, ok := report.Config.Files[name]
		if !ok || len(digest) != 64 {
			t.Errorf("files[%q] = %q, want 64-character SHA-256", name, digest)
		}
	}
	if len(report.Stages) != 6 {
		t.Errorf("stages = %v, want six stages", report.Stages)
	}
	if len(report.Memory) == 0 || string(report.Memory) == "null" {
		t.Error("memory report is missing")
	}
	if report.Energy.Measured {
		t.Error("energy.measured = true, want false")
	}
	if _, err := os.Stat(out + ".snapshot.tmp.coimbundle"); !os.IsNotExist(err) {
		t.Errorf("snapshot bundle still exists or stat failed unexpectedly: %v", err)
	}
}

func TestBenchmarkStoreModeRejects(t *testing.T) {
	dir, store, rules := deriveFixture(t)
	store = addFullGraphFixtureClasses(t, dir)
	paramsPath, _ := deriveParameterSet(t, dir, store, rules, "benchmark-store-reject-params.coimparams")
	protocol := writeCompareProtocol(t, dir, "benchmark-store-reject-compare.json", fullGraphCompareProtocol())
	base := []string{"benchmark", "--store", store, "--params", paramsPath, "--protocol", protocol, "--input-set", "in", "--readout-set", "out", "--steps", "4", "--repeat", "2"}
	partialOut := filepath.Join(t.TempDir(), "partial.json")
	nodesOut := filepath.Join(t.TempDir(), "nodes.json")
	loadMemoryOut := filepath.Join(t.TempDir(), "memory.json")
	planMemoryOut := filepath.Join(t.TempDir(), "memory-plan.json")
	cases := []struct {
		name      string
		args      []string
		out       string
		wantUsage bool
		wantText  string
	}{
		{name: "partial store flags", args: []string{"benchmark", "--store", store, "--out", partialOut}, wantUsage: true},
		{name: "nodes with store", args: append(append([]string(nil), base...), "--nodes", "8", "--out", nodesOut), wantUsage: true, wantText: "--nodes and --edges come from --store"},
		{name: "load memory refusal", args: append(append([]string(nil), base...), "--max-memory-mib", "1", "--steps", "100000", "--out", loadMemoryOut), out: loadMemoryOut, wantText: "MiB"},
		{name: "plan memory refusal", args: append(append([]string(nil), base...), "--max-memory-mib", "6", "--steps", "250000", "--out", planMemoryOut), out: planMemoryOut, wantText: "the plan estimates"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), tc.args, &stdout, &stderr)
			if err == nil {
				t.Fatalf("accepted %v", tc.args)
			}
			_, usageErr := err.(*ExitError)
			if usageErr != tc.wantUsage {
				t.Errorf("usage error = %t, want %t: %v", usageErr, tc.wantUsage, err)
			}
			if tc.wantText != "" && !strings.Contains(err.Error(), tc.wantText) {
				t.Errorf("error %q missing %q", err, tc.wantText)
			}
			if strings.Contains(tc.name, "memory refusal") && !strings.Contains(err.Error(), "MiB") {
				t.Errorf("memory error %q does not name MiB", err)
			}
			if tc.out != "" {
				if _, statErr := os.Stat(tc.out); !os.IsNotExist(statErr) {
					t.Errorf("memory refusal wrote report or stat failed unexpectedly: %v", statErr)
				}
			}
		})
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
