package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/experiment/fullgraph"
	"github.com/TimLai666/coimnet/simulate"
	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
)

func TestExamplesRunFullGraphEndToEnd(t *testing.T) {
	dir, store, rules := deriveFixture(t)
	store = addFullGraphFixtureClasses(t, dir)
	paramsPath, _ := deriveParameterSet(t, dir, store, rules, "fullgraph-params.coimparams")
	protocol := writeCompareProtocol(t, dir, "fullgraph-compare.json", fullGraphCompareProtocol())
	outDir := filepath.Join(dir, "fullgraph-out")

	args := []string{"examples", "run", "full-graph-short-training", "--store", store, "--params", paramsPath,
		"--protocol", protocol, "--out-dir", outDir, "--input-set", "in", "--readout-set", "out",
		"--steps", "4", "--truncation", "2", "--continue-rows", "2", "--plastic-edges", "1", "--max-memory-mib", "64"}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("full graph run: %v; stderr=%s", err, stderr.String())
	}
	var report struct {
		SchemaVersion string `json:"schema_version"`
		Nodes         int    `json:"nodes"`
		Edges         int    `json:"edges"`
		Run           struct {
			Updates int `json:"updates"`
		} `json:"run"`
		Memory struct {
			TotalBytes uint64 `json:"total_bytes"`
		} `json:"memory"`
		ContinuationDigest string `json:"continuation_digest"`
	}
	data, err := os.ReadFile(filepath.Join(outDir, "report.json"))
	if err != nil {
		t.Fatalf("read full graph report: %v", err)
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode full graph report: %v", err)
	}
	if report.SchemaVersion != fullgraph.ReportSchemaVersion || report.Nodes != 2 || report.Edges != 2 || report.Run.Updates != 1 {
		t.Fatalf("report summary = %+v", report)
	}
	const mib = uint64(1 << 20)
	estimate := report.Memory.TotalBytes / mib
	if report.Memory.TotalBytes%mib != 0 {
		estimate++
	}
	wantSummary := fmt.Sprintf("full-graph-short-training: 2 nodes, 2 edges, updates 1, estimate %d MiB, report %s/report.json, continuation %s\n",
		estimate, outDir, report.ContinuationDigest[:12])
	if stdout.String() != wantSummary {
		t.Fatalf("summary = %q, want %q", stdout.String(), wantSummary)
	}
	t.Logf("report summary: schema=%s nodes=%d edges=%d updates=%d continuation=%s", report.SchemaVersion, report.Nodes, report.Edges, report.Run.Updates, report.ContinuationDigest[:12])
	for _, name := range []string{"model.coimbundle", "individual.coimbundle", "training.coimbundle"} {
		for _, file := range []string{"manifest.json", "document.json", "arrays.bin"} {
			if info, err := os.Stat(filepath.Join(outDir, name, file)); err != nil || !info.Mode().IsRegular() {
				t.Errorf("snapshot file %s/%s missing or not regular: %v", name, file, err)
			}
		}
	}

	stdout.Reset()
	stderr.Reset()
	if err := Run(context.Background(), []string{"examples", "run", "full-graph-short-training", "--resume", outDir}, &stdout, &stderr); err != nil {
		t.Fatalf("resume full graph: %v; stderr=%s", err, stderr.String())
	}
	var resumed struct {
		Matches bool `json:"matches"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &resumed); err != nil || !resumed.Matches {
		t.Fatalf("resume report = %q, decode error = %v", stdout.String(), err)
	}
	t.Logf("resume summary: matches=%t", resumed.Matches)
	var reportDocument map[string]any
	if err := json.Unmarshal(data, &reportDocument); err != nil {
		t.Fatal(err)
	}
	reportDocument["continuation_digest"] = strings.Repeat("0", 64)
	data, err = json.MarshalIndent(reportDocument, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "report.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	err = Run(context.Background(), []string{"examples", "run", "full-graph-short-training", "--resume", outDir}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("digest mismatch error = %v, want non-usage error", err)
	}
	if _, isUsage := err.(*ExitError); isUsage {
		t.Fatalf("digest mismatch returned usage error: %v", err)
	}
	if err := json.Unmarshal(stdout.Bytes(), &resumed); err != nil || resumed.Matches {
		t.Fatalf("mismatch resume report = %q, decode error = %v", stdout.String(), err)
	}
}

func TestExamplesRunFullGraphRejects(t *testing.T) {
	dir, store, rules := deriveFixture(t)
	store = addFullGraphFixtureClasses(t, dir)
	paramsPath, _ := deriveParameterSet(t, dir, store, rules, "fullgraph-reject-params.coimparams")
	protocol := writeCompareProtocol(t, dir, "fullgraph-reject-compare.json", fullGraphCompareProtocol())
	args := []string{"examples", "run", "full-graph-short-training", "--store", store, "--params", paramsPath,
		"--protocol", protocol, "--out-dir", filepath.Join(dir, "output"), "--input-set", "in", "--readout-set", "out"}
	existing := filepath.Join(dir, "existing")
	if err := os.Mkdir(existing, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
		flag string
	}{
		{name: "missing store", args: append(append([]string(nil), args[:3]...), args[5:]...), flag: "--store"},
		{name: "invalid steps", args: append(append([]string(nil), args...), "--steps", "1"), flag: "--steps"},
		{name: "resume with run flags", args: append(append([]string(nil), args...), "--resume", existing), flag: "--resume"},
		{name: "existing out dir", args: replaceFlagValue(args, "--out-dir", existing), flag: "--out-dir"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), tc.args, &stdout, &stderr)
			if err == nil || ExitCode(err) != exitUsage {
				t.Fatalf("error = %v, exit code = %d, want usage error", err, ExitCode(err))
			}
			if !strings.Contains(err.Error(), tc.flag) {
				t.Errorf("error %q does not name %s", err, tc.flag)
			}
		})
	}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "run", "full-graph-short-training", "--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("help: %v", err)
	}
	if !strings.Contains(stdout.String(), "Usage: coimnet examples run full-graph-short-training") || !strings.Contains(stdout.String(), "--resume") {
		t.Fatalf("help missing Usage or --resume:\n%s", stdout.String())
	}
}

func replaceFlagValue(args []string, flagName, value string) []string {
	copyArgs := append([]string(nil), args...)
	for i := 0; i+1 < len(copyArgs); i++ {
		if copyArgs[i] == flagName {
			copyArgs[i+1] = value
			return copyArgs
		}
	}
	return copyArgs
}

func TestExamplesListIncludesFullGraph(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "list"}, &stdout, &stderr); err != nil {
		t.Fatalf("examples list: %v", err)
	}
	var entries []struct {
		Name        string `json:"name"`
		Profile     string `json:"profile"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &entries); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name == "full-graph-short-training" {
			if entry.Profile != "full_graph" || entry.Description == "" {
				t.Fatalf("full graph list entry = %+v", entry)
			}
			return
		}
	}
	t.Fatal("examples list does not include full-graph-short-training")
}

func fullGraphCompareProtocol() simulate.CompareProtocol {
	run := derivedSimulateProtocol(4, "exclude", 1)
	run.Core = simulate.CoreContinuous
	run.LIF = nil
	run.Continuous = &dynamics.Config{DT: 1, Activation: "tanh"}
	run.Probes = []simulate.Probe{{Name: "population", Nodes: []int{0, 1}, Reduce: simulate.ReduceMeanOutput}}
	return simulate.CompareProtocol{
		SchemaVersion: simulate.CompareProtocolSchemaVersion,
		Run:           run,
		Sets: []simulate.NamedSet{
			{Name: "in", Selectors: []simulate.Selector{{Field: "class", Equals: "input"}}},
			{Name: "out", Selectors: []simulate.Selector{{Field: "class", Equals: "readout"}}},
		},
		Metrics:    []simulate.Metric{{Name: "readout", Kind: simulate.MetricMeanOutput, Set: "out", Window: [2]int{0, 4}}},
		Thresholds: []simulate.Threshold{{Metric: "readout", Op: simulate.OpAtLeast, Value: 0}},
	}
}

func addFullGraphFixtureClasses(t *testing.T, dir string) string {
	t.Helper()
	manifestPath := filepath.Join(dir, "manifest.json")
	var manifest map[string]any
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	annotationsPath := filepath.Join(dir, "annotations-fullgraph.feather")
	writeFullGraphAnnotations(t, annotationsPath)
	files := manifest["files"].([]any)
	for _, raw := range files {
		file := raw.(map[string]any)
		if file["role"] == "annotations" {
			file["path"] = filepath.Base(annotationsPath)
			annotationBytes := mustReadFile(t, annotationsPath)
			file["bytes"] = len(annotationBytes)
			sum := sha256.Sum256(annotationBytes)
			file["sha256"] = hex.EncodeToString(sum[:])
		}
	}
	manifest["field_mapping"].(map[string]any)["annotations"].(map[string]any)["class"] = "class"
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	newStore := filepath.Join(dir, "fullgraph.coimgraph")
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"data", "import", "--manifest", manifestPath, "--temp-dir", t.TempDir(), "--out-store", newStore}, &stdout, &stderr); err != nil {
		t.Fatalf("import annotated full graph fixture: %v; stderr=%s", err, stderr.String())
	}
	return newStore
}

func writeFullGraphAnnotations(t *testing.T, path string) {
	t.Helper()
	writeImportFeather(t, path, arrow.NewSchema([]arrow.Field{
		{Name: "bodyId", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "status", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "class", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues([]int64{1, 2}, nil)
		b.Field(1).(*array.StringBuilder).AppendValues([]string{"Traced", "Traced"}, nil)
		b.Field(2).(*array.StringBuilder).AppendValues([]string{"input", "readout"}, nil)
	})
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
