package fullgraph

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// TestFullGraphEvidence is the OPS-07 short training on the whole real graph.
// It runs Run with DefaultOptions on the store, parameter set and compare
// protocol the environment names, and Run leaves its output directory,
// report.json included, at $COIMNET_OPS07_EVIDENCE/run. Without
// COIMNET_OPS07_EVIDENCE the test is skipped, so an ordinary go test never
// loads real data. Every path must be absolute, because go test runs in the
// package directory. The run directory holds multi-gigabyte artifacts, so it
// belongs under the git-ignored runs/ rather than evidence/.
// COIMNET_OPS07_MAX_MEMORY_MIB optionally replaces the 12288 MiB default
// limit; a refusal prints Run's error, estimate included, as it is.
//
// Reproduction command (from the repository root):
//
//	COIMNET_OPS07_EVIDENCE=$PWD/runs/OPS-07 \
//	COIMNET_OPS07_STORE=$PWD/data/malecns-v1.0/graph-v1.coimgraph \
//	COIMNET_OPS07_PARAMS=$PWD/data/malecns-v1.0/params-derive-v1.coimparams \
//	COIMNET_OPS07_PROTOCOL=$PWD/evidence/NAT-05/compare-fullgraph-derived-continuous.json \
//	go test -count=1 -timeout 0 -v -run '^TestFullGraphEvidence$' ./experiment/fullgraph/
//
// The resume check then runs in a separate go test process; see
// TestFullGraphResumeEvidence.
func TestFullGraphEvidence(t *testing.T) {
	evidence := os.Getenv("COIMNET_OPS07_EVIDENCE")
	if evidence == "" {
		t.Skip("COIMNET_OPS07_EVIDENCE is not set")
	}
	o := DefaultOptions()
	o.Store = os.Getenv("COIMNET_OPS07_STORE")
	o.Params = os.Getenv("COIMNET_OPS07_PARAMS")
	o.Protocol = os.Getenv("COIMNET_OPS07_PROTOCOL")
	o.OutDir = filepath.Join(evidence, "run")
	for _, env := range []struct {
		name string
		path string
	}{
		{"COIMNET_OPS07_EVIDENCE", evidence},
		{"COIMNET_OPS07_STORE", o.Store},
		{"COIMNET_OPS07_PARAMS", o.Params},
		{"COIMNET_OPS07_PROTOCOL", o.Protocol},
	} {
		if !filepath.IsAbs(env.path) {
			t.Fatalf("%s is %q, want an absolute path (go test runs in the package directory)", env.name, env.path)
		}
	}
	if limit := os.Getenv("COIMNET_OPS07_MAX_MEMORY_MIB"); limit != "" {
		mib, err := strconv.Atoi(limit)
		if err != nil {
			t.Fatalf("COIMNET_OPS07_MAX_MEMORY_MIB is %q: %v", limit, err)
		}
		o.MaxMemoryMiB = mib
	}
	report, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("nodes=%d edges=%d updates=%d", report.Nodes, report.Edges, report.Run.Updates)
	t.Logf("mechanisms=%+v", report.Mechanisms)
	t.Logf("continuation_digest=%s", report.ContinuationDigest)
}

// TestFullGraphResumeEvidence is the second half of the OPS-07 evidence and
// runs in a go test process of its own, after TestFullGraphEvidence has
// finished. It calls Resume on the run directory $COIMNET_OPS07_RESUME, writes
// the ResumeReport as indented JSON to resume.json next to that directory (a
// new file; an existing one fails the test before anything is loaded) and
// fails when either digest differs from the run's report.json. Without
// COIMNET_OPS07_RESUME the test is skipped.
//
// Reproduction command (from the repository root, after the
// TestFullGraphEvidence command has finished):
//
//	COIMNET_OPS07_RESUME=$PWD/runs/OPS-07/run \
//	go test -count=1 -timeout 0 -v -run '^TestFullGraphResumeEvidence$' ./experiment/fullgraph/
func TestFullGraphResumeEvidence(t *testing.T) {
	dir := os.Getenv("COIMNET_OPS07_RESUME")
	if dir == "" {
		t.Skip("COIMNET_OPS07_RESUME is not set")
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("COIMNET_OPS07_RESUME is %q, want an absolute path (go test runs in the package directory)", dir)
	}
	dir = filepath.Clean(dir)
	path := filepath.Join(filepath.Dir(dir), "resume.json")
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("%s already exists", path)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	report, err := Resume(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := refuseExistingWrite(path, append(data, '\n')); err != nil {
		t.Fatal(err)
	}
	t.Logf("rows=%d matches=%t continuation_digest=%s neural_digest=%s", report.Rows, report.Matches, report.ContinuationDigest, report.NeuralDigest)
	if !report.Matches {
		t.Fatalf("the resumed digests differ from %s: %+v", filepath.Join(dir, "report.json"), report)
	}
}
