package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/bench"
)

func writeBenchResult(t *testing.T, dir, name string, r bench.Result) string {
	t.Helper()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func benchResult(framework string, accuracy, ms float64, params int64, rss uint64) bench.Result {
	return bench.Result{
		SchemaVersion: bench.SchemaVersion, Framework: framework, Model: "m", Task: "kmnist", Split: "abc",
		Host:       bench.Host{Machine: "box", CPU: "cpu"},
		Parameters: params, Trainable: params, PeakRSSBytes: rss, IdleRSSBytes: rss / 2,
		Device: "cpu", Threads: 1, Latency: bench.Latency{Samples: 1, P50MS: ms, P90MS: ms},
		Quality: map[string]float64{"accuracy": accuracy}, Higher: []string{"accuracy"},
	}
}

func TestBenchCompareCommand(t *testing.T) {
	dir := t.TempDir()
	cand := writeBenchResult(t, dir, "c.json", benchResult("coimnet", .8, 1, 100, 1<<20))
	base := writeBenchResult(t, dir, "b.json", benchResult("torch", .7, 2, 200, 4<<20))
	out := filepath.Join(dir, "cmp.json")
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"bench", "compare", "--candidate", cand, "--baseline", base, "--json", out}, &stdout, &stderr); err != nil {
		t.Fatalf("bench compare: %v; stderr=%s", err, &stderr)
	}
	if !strings.Contains(stdout.String(), "| torch/m | +0.1000 | 0.50 | 0.25 | 0.50 | yes |") {
		t.Fatalf("unexpected table:\n%s", &stdout)
	}
	var cmp bench.Comparison
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &cmp); err != nil || len(cmp.Versus) != 1 || !cmp.Versus[0].Wins {
		t.Fatalf("comparison JSON = %+v, err %v", cmp, err)
	}
	if err := Run(context.Background(), []string{"bench", "compare", "--candidate", cand, "--baseline", base, "--json", out}, &stdout, &stderr); err == nil {
		t.Fatal("bench compare overwrote an existing --json path")
	}
	other := benchResult("torch", .7, 2, 200, 4<<20)
	other.Host.Machine = "elsewhere"
	far := writeBenchResult(t, dir, "far.json", other)
	if err := Run(context.Background(), []string{"bench", "compare", "--candidate", cand, "--baseline", far}, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "host") {
		t.Fatalf("bench compare across machines: %v", err)
	}
}
