package fullgraph

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

// fullgraphResumeHelperEnv separates the resume subprocess from its parent.
// Using the same pattern the checkpoint package does, the helper is a second
// test function; the parent runs it with exec against its own binary.
const fullgraphResumeHelperEnv = "COIMNET_FULLGRAPH_RESUME_HELPER"

// buildFixtureConfig returns the OPS-07 fixture model: the 4-node, 5-edge
// variant with input node 0 and readout nodes 2 and 3.
func buildFixtureConfig(t *testing.T) (learning.Config, learning.Parameters, learning.Options) {
	t.Helper()
	c, p, o, _, err := buildModel(fixtureVariant(), 4, []int{0}, []int{2, 3}, fixtureOptions())
	if err != nil {
		t.Fatal(err)
	}
	return c, p, o
}

// fileSHA256 hashes a file the way the artifact records do, so a test can
// recompute it independently of the implementation.
func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// TestShortTrainingUpdatesEveryGroupAndSaves trains the whole fixture graph
// with plasticity and chemistry on. Every non-frozen parameter group of a
// continuous core has to move in that one update, the plastic and chemical
// layers have to run at least one row, and the three artifacts have to be on
// disk with sizes and hashes that agree with what the report records.
func TestShortTrainingUpdatesEveryGroupAndSaves(t *testing.T) {
	ctx := context.Background()
	c, p, o := buildFixtureConfig(t)
	dir := t.TempDir()
	run := runOptions{Steps: 8, ContinueRows: 4, PlasticEdges: 2, Chemistry: true, MaxCells: 0}
	ind, rep, err := shortTraining(ctx, c, p, o, run, dir)
	if err != nil {
		t.Fatal(err)
	}
	if ind == nil {
		t.Fatal("shortTraining returned a nil individual")
	}
	if rep.Rows != run.Steps {
		t.Errorf("rows = %d, want %d", rep.Rows, run.Steps)
	}
	if rep.Chunk != run.Steps {
		t.Errorf("chunk = %d, want %d", rep.Chunk, run.Steps)
	}
	if rep.ForwardCalls < 1 {
		t.Errorf("forward calls = %d, want at least 1", rep.ForwardCalls)
	}
	if rep.PlasticEdges != run.PlasticEdges {
		t.Errorf("plastic edges = %d, want %d", rep.PlasticEdges, run.PlasticEdges)
	}
	wantGroups := []string{"weights", "bias", "log_tau", "encoder", "readout"}
	for _, group := range wantGroups {
		if rep.ParametersChanged[group] <= 0 {
			t.Errorf("parameters_changed[%q] = %d, want > 0", group, rep.ParametersChanged[group])
		}
	}
	for group := range rep.ParametersChanged {
		if !contains(wantGroups, group) {
			t.Errorf("parameters_changed has unexpected group %q", group)
		}
	}
	if rep.Updates != 1 {
		t.Errorf("updates = %d, want 1", rep.Updates)
	}
	for name, v := range map[string]float64{"loss": rep.Loss, "gradient_norm": rep.GradientNorm, "update_norm": rep.UpdateNorm} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Errorf("%s = %v, want finite", name, v)
		}
	}
	if rep.PlasticSteps <= 0 {
		t.Errorf("plastic steps = %d, want > 0", rep.PlasticSteps)
	}
	if rep.ChemistrySteps <= 0 {
		t.Errorf("chemistry steps = %d, want > 0", rep.ChemistrySteps)
	}
	if len(rep.Artifacts) < 2 {
		t.Fatalf("saved %d artifacts, want at least 2", len(rep.Artifacts))
	}
	for _, a := range rep.Artifacts {
		if a.Path != filepath.Join(dir, a.Kind) {
			t.Errorf("artifact path %s, want %s", a.Path, filepath.Join(dir, a.Kind))
		}
		info, err := os.Stat(a.Path)
		if err != nil {
			t.Errorf("artifact %s: %v", a.Path, err)
			continue
		}
		if info.Size() != a.Bytes {
			t.Errorf("artifact %s sizes: report %d, disk %d", a.Path, a.Bytes, info.Size())
		}
		digest, err := fileSHA256(a.Path)
		if err != nil {
			t.Errorf("artifact %s: %v", a.Path, err)
			continue
		}
		if digest != a.SHA256 {
			t.Errorf("artifact %s hash %s, want %s", a.Path, digest, a.SHA256)
		}
	}
	for _, kind := range []string{"model.coimpkg", "individual.json", "training.json"} {
		if _, err := os.Stat(filepath.Join(dir, kind)); err != nil {
			t.Errorf("missing artifact %s: %v", kind, err)
		}
	}
}

func contains(items []string, item string) bool {
	for _, it := range items {
		if it == item {
			return true
		}
	}
	return false
}

// TestShortTrainingChunksMatchOneCall runs the same forward twice, once in
// three MaxCells-sized pieces and once in one piece, and requires the neural
// state reached right before the gradient update to be bit-identical. The
// chunk size only changes call boundaries, never the row-by-row order the
// stepwise path runs.
func TestShortTrainingChunksMatchOneCall(t *testing.T) {
	ctx := context.Background()
	c, p, o := buildFixtureConfig(t)
	_, chunked, err := shortTraining(ctx, c, p, o, runOptions{Steps: 8, ContinueRows: 4, PlasticEdges: 2, Chemistry: true, MaxCells: 12}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, whole, err := shortTraining(ctx, c, p, o, runOptions{Steps: 8, ContinueRows: 4, PlasticEdges: 2, Chemistry: true, MaxCells: 0}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if chunked.Chunk != 3 {
		t.Errorf("chunked chunk = %d, want 3", chunked.Chunk)
	}
	if whole.Chunk != 8 {
		t.Errorf("whole chunk = %d, want 8", whole.Chunk)
	}
	if chunked.ForwardCalls != 3 {
		t.Errorf("chunked forward calls = %d, want 3", chunked.ForwardCalls)
	}
	if whole.ForwardCalls != 1 {
		t.Errorf("whole forward calls = %d, want 1", whole.ForwardCalls)
	}
	if chunked.ForwardDigest != whole.ForwardDigest {
		t.Errorf("chunked forward digest %s differs from whole %s", chunked.ForwardDigest, whole.ForwardDigest)
	}
}

// TestShortTrainingResumesInANewProcess runs four more rows in the parent
// process, then resumes from the saved individual.json in a fresh process and
// requires the continued outputs to agree bit for bit and the snapshots to be
// the same JSON. An exact resume has to hold with plasticity and chemistry
// still enabled, whose states all live in the snapshot.
func TestShortTrainingResumesInANewProcess(t *testing.T) {
	if os.Getenv(fullgraphResumeHelperEnv) == "1" {
		t.Skip("helper is tested separately")
	}
	ctx := context.Background()
	c, p, o := buildFixtureConfig(t)
	dir := t.TempDir()
	run := runOptions{Steps: 8, ContinueRows: 4, PlasticEdges: 2, Chemistry: true, MaxCells: 0}
	ind, _, err := shortTraining(ctx, c, p, o, run, dir)
	if err != nil {
		t.Fatal(err)
	}

	const rows, chunk = 4, 2
	wantOutputs, err := continueRows(ctx, ind, rows, chunk)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := json.Marshal(ind.Snapshot())
	if err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(dir, "resumed-outputs.json")
	snapshotPath := filepath.Join(dir, "resumed-snapshot.json")
	runFullgraphResumeHelper(t, dir, strconv.Itoa(rows), strconv.Itoa(chunk), outputPath, snapshotPath)

	resumed, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(resumed, snapshot) {
		t.Fatal("resumed individual snapshot differs from the uninterrupted one")
	}
	gotData, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var gotOutputs [][]float64
	if err := json.Unmarshal(gotData, &gotOutputs); err != nil {
		t.Fatal(err)
	}
	if len(gotOutputs) != rows {
		t.Fatalf("resumed %d rows, want %d", len(gotOutputs), rows)
	}
	for r := range wantOutputs {
		if len(gotOutputs[r]) != len(wantOutputs[r]) {
			t.Fatalf("row %d width = %d, want %d", r, len(gotOutputs[r]), len(wantOutputs[r]))
		}
		for i := range wantOutputs[r] {
			if math.Float64bits(gotOutputs[r][i]) != math.Float64bits(wantOutputs[r][i]) {
				t.Fatalf("row %d value %d = %v, want %v", r, i, gotOutputs[r][i], wantOutputs[r][i])
			}
		}
	}
}

// TestFullgraphResumeHelperProcess is the subprocess half of the resume test.
// With the helper env var set it loads dir/individual.json, continues the
// declared number of rows in the declared chunk size and writes the outputs
// and the final snapshot as JSON; without it the test is a no-op.
func TestFullgraphResumeHelperProcess(t *testing.T) {
	if os.Getenv(fullgraphResumeHelperEnv) != "1" {
		return
	}
	args := os.Args
	separator := -1
	for i, arg := range args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator < 0 || len(args)-separator != 6 {
		t.Fatalf("helper arguments = %#v", args)
	}
	dir, rows, chunk := args[separator+1], args[separator+2], args[separator+3]
	outputPath, snapshotPath := args[separator+4], args[separator+5]
	n, err := strconv.Atoi(rows)
	if err != nil {
		t.Fatal(err)
	}
	c, err := strconv.Atoi(chunk)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	outputs, snapshot, err := resumeAndContinue(ctx, dir, n, c)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(outputs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outputPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	data, err = json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapshotPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// runFullgraphResumeHelper executes the helper test function in a fresh
// process of the running binary, with the helper env var set.
func runFullgraphResumeHelper(t *testing.T, args ...string) {
	t.Helper()
	cmdArgs := append([]string{"-test.run=^TestFullgraphResumeHelperProcess$", "--"}, args...)
	cmd := exec.Command(os.Args[0], cmdArgs...)
	cmd.Env = append(os.Environ(), fullgraphResumeHelperEnv+"=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("resume helper failed: %v\n%s", err, output)
	}
}

// TestShortTrainingRejects covers the run configurations the protocol refuses
// before touching the model, and the refusal to overwrite an artifact that
// already exists on disk.
func TestShortTrainingRejects(t *testing.T) {
	ctx := context.Background()
	c, p, o := buildFixtureConfig(t)
	invalid := []struct {
		name string
		run  runOptions
	}{
		{"steps below two", runOptions{Steps: 1, ContinueRows: 1}},
		{"continue rows zero", runOptions{Steps: 4, ContinueRows: 0}},
		{"plastic edges negative", runOptions{Steps: 4, ContinueRows: 1, PlasticEdges: -1}},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := shortTraining(ctx, c, p, o, tc.run, t.TempDir()); err == nil {
				t.Fatal("shortTraining accepted an invalid configuration")
			}
		})
	}
	t.Run("existing artifact refused", func(t *testing.T) {
		staging := t.TempDir()
		ind, _, err := shortTraining(ctx, c, p, o, runOptions{Steps: 8, ContinueRows: 4, PlasticEdges: 2, Chemistry: true, MaxCells: 0}, staging)
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		existing := filepath.Join(dir, "individual.json")
		if err := os.WriteFile(existing, []byte("occupied"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := saveArtifacts(dir, ind, c, p); err == nil {
			t.Fatal("saveArtifacts accepted an existing individual.json")
		}
	})
}
