package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/experiment"
)

// ablationGroupFile is the slice of one group summary JSON the CLI tests read
// back from DIR/ablation/<group>.json. It mirrors experiment.GroupSummary for
// the fields these tests assert.
type ablationGroupFile struct {
	Group string `json:"group"`
	Runs  []struct {
		Seed uint64 `json:"seed"`
	} `json:"runs"`
}

// ablationSummaryFile is the slice of DIR/ablation/summary.json these tests
// read back. It mirrors experiment.AblationReport for the asserted fields.
type ablationSummaryFile struct {
	SchemaVersion string `json:"schema_version"`
	Groups        []struct {
		Group string        `json:"group"`
		Runs  []interface{} `json:"runs"`
	} `json:"groups"`
}

var ablationAllGroups = []string{
	experiment.GroupNoModulation,
	experiment.GroupDirectReward,
	experiment.GroupFixedDecay,
	experiment.GroupTrainableController,
	experiment.GroupCapacityMatched,
}

// runAblateExample runs `examples run ablate` and fails the test unless the
// command succeeds, returning the summary line.
func runAblateExample(t *testing.T, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	full := append([]string{"examples", "run", "ablate"}, args...)
	if err := Run(context.Background(), full, &stdout, &stderr); err != nil {
		t.Fatalf("%v: %v; stderr=%s", full, err, stderr.String())
	}
	return stdout.String()
}

func TestAblateWritesEveryGroup(t *testing.T) {
	outDir := t.TempDir()
	stdout := runAblateExample(t, "--episodes", "3", "--eval", "4", "--out-dir", outDir)

	ablationDir := filepath.Join(outDir, "ablation")
	for _, group := range ablationAllGroups {
		data, err := os.ReadFile(filepath.Join(ablationDir, group+".json"))
		if err != nil {
			t.Fatalf("read %s.json: %v", group, err)
		}
		var gf ablationGroupFile
		if err := json.Unmarshal(data, &gf); err != nil {
			t.Fatalf("%s.json is not parseable JSON: %v\n%s", group, err, data)
		}
		if gf.Group != group {
			t.Fatalf("%s.json group = %q, want %q", group, gf.Group, group)
		}
		if len(gf.Runs) != 3 {
			t.Fatalf("%s.json runs = %d, want 3", group, len(gf.Runs))
		}
	}
	data, err := os.ReadFile(filepath.Join(ablationDir, "summary.json"))
	if err != nil {
		t.Fatalf("read summary.json: %v", err)
	}
	var sf ablationSummaryFile
	if err := json.Unmarshal(data, &sf); err != nil {
		t.Fatalf("summary.json is not parseable JSON: %v\n%s", err, data)
	}
	if sf.SchemaVersion != experiment.AblationSchemaVersion {
		t.Fatalf("summary schema_version = %q, want %q", sf.SchemaVersion, experiment.AblationSchemaVersion)
	}
	if len(sf.Groups) != 5 {
		t.Fatalf("summary groups = %d, want 5", len(sf.Groups))
	}
	for i, g := range sf.Groups {
		if g.Group != ablationAllGroups[i] {
			t.Fatalf("summary groups[%d] = %q, want %q", i, g.Group, ablationAllGroups[i])
		}
		if len(g.Runs) != 3 {
			t.Fatalf("summary groups[%d] runs = %d, want 3", i, len(g.Runs))
		}
	}
	if !strings.Contains(stdout, "ablate:") {
		t.Fatalf("summary missing %q:\n%s", "ablate:", stdout)
	}
}

func TestAblateSubsetOfGroups(t *testing.T) {
	outDir := t.TempDir()
	runAblateExample(t, "--episodes", "3", "--eval", "4", "--groups", "no_modulation,direct_reward", "--out-dir", outDir)

	ablationDir := filepath.Join(outDir, "ablation")
	for _, name := range []string{"no_modulation.json", "direct_reward.json", "summary.json"} {
		if _, err := os.Stat(filepath.Join(ablationDir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	for _, name := range []string{"fixed_decay.json", "trainable_controller.json", "capacity_matched.json"} {
		if _, err := os.Stat(filepath.Join(ablationDir, name)); err == nil {
			t.Fatalf("unexpected file %s in ablation subset", name)
		}
	}
}

func TestAblateRejects(t *testing.T) {
	existing := t.TempDir()
	if err := os.Mkdir(filepath.Join(existing, "ablation"), 0o755); err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	for _, args := range [][]string{
		{"--episodes", "3", "--eval", "4"},
		{"--episodes", "3", "--eval", "4", "--out-dir", existing},
		{"--episodes", "3", "--eval", "4", "--out-dir", base, "--groups", "direct_reward"},
		{"--episodes", "3", "--eval", "4", "--out-dir", base, "--seeds", "7,42"},
		{"--episodes", "0", "--eval", "4", "--out-dir", base},
		{"--episodes", "3", "--eval", "4", "--out-dir", base, "extra"},
	} {
		var stdout, stderr bytes.Buffer
		full := append([]string{"examples", "run", "ablate"}, args...)
		err := Run(context.Background(), full, &stdout, &stderr)
		if err == nil {
			t.Fatalf("accepted %v", args)
		}
		if code := ExitCode(err); code != exitUsage {
			t.Fatalf("%v: exit code = %d, want %d: %v", args, code, exitUsage, err)
		}
	}

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"examples", "run", "ablate", "--out-dir", existing, "--episodes", "3", "--eval", "4"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("an existing ablation/ directory was reused")
	}
	if !strings.Contains(err.Error(), "exists") {
		t.Fatalf("error %q does not report the existing ablation directory", err)
	}
	if code := ExitCode(err); code != exitUsage {
		t.Fatalf("existing ablation/: exit code = %d, want %d", code, exitUsage)
	}

	var helpOut, helpErr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "run", "ablate", "--help"}, &helpOut, &helpErr); err != nil {
		t.Fatalf("help: %v %s", err, helpErr.String())
	}
	if !strings.Contains(helpOut.String(), "Usage: coimnet examples run ablate") {
		t.Fatalf("help missing usage line:\n%s", helpOut.String())
	}
	var helpOut2, helpErr2 bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "--help"}, &helpOut2, &helpErr2); err != nil {
		t.Fatalf("examples help: %v %s", err, helpErr2.String())
	}
	if !strings.Contains(helpOut2.String(), "run ablate") {
		t.Fatalf("examples help missing ablate:\n%s", helpOut2.String())
	}
}
