package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestExamplesRunRoles(t *testing.T) {
	out := filepath.Join(t.TempDir(), "r.json")
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"examples", "run", "roles", "--seeds", "1", "--epochs", "2", "--out", out}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run roles: %v; stderr: %s", err, stderr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var report struct {
		SchemaVersion string `json:"schema_version"`
		Runs          []struct {
			CoreGenerated struct {
				Mode string `json:"mode"`
			} `json:"core_generated"`
			FixedDecoder struct {
				Mode string `json:"mode"`
			} `json:"fixed_decoder"`
			ExternalTool struct {
				Mode string `json:"mode"`
			} `json:"external_tool"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.SchemaVersion != "coimnet-roles/v1" || len(report.Runs) != 1 {
		t.Fatalf("schema/runs = %q/%d, want coimnet-roles/v1/1", report.SchemaVersion, len(report.Runs))
	}
	run := report.Runs[0]
	if run.CoreGenerated.Mode != "core_generated" || run.FixedDecoder.Mode != "fixed_decoder" || run.ExternalTool.Mode != "external_tool" {
		t.Fatalf("mode strings = %q, %q, %q", run.CoreGenerated.Mode, run.FixedDecoder.Mode, run.ExternalTool.Mode)
	}
	pattern := `^roles: 1 seeds; core_generated seen [0-9]+\.[0-9]{2} of [0-9]+, held-out [0-9]+\.[0-9]{2} of [0-9]+; fixed_decoder seen [0-9]+\.[0-9]{2} of [0-9]+, held-out [0-9]+\.[0-9]{2} of [0-9]+ \(decoder [0-9a-f]{12}\); external_tool requests seen [0-9]+\.[0-9]{2} of [0-9]+, held-out [0-9]+\.[0-9]{2} of [0-9]+; tool pixels seen [0-9]+\.[0-9]{2}, held-out [0-9]+\.[0-9]{2} \(not core capability\); tool calls [0-9]+, budget refused [0-9]+$`
	if ok, err := regexp.MatchString(pattern, strings.TrimSpace(stdout.String())); err != nil || !ok {
		t.Fatalf("summary = %q, match %t, error %v", stdout.String(), ok, err)
	}
	t.Log(strings.TrimSpace(stdout.String()))
}

func TestExamplesRunRolesRejects(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing.json")
	if err := os.WriteFile(existing, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "existing output", args: []string{"--seeds", "1", "--out", existing}},
		{name: "missing output", args: []string{"--seeds", "1"}},
		{name: "zero epochs", args: []string{"--seeds", "1", "--epochs", "0", "--out", filepath.Join(dir, "epochs.json")}},
		{name: "zero tool calls", args: []string{"--seeds", "1", "--max-tool-calls", "0", "--out", filepath.Join(dir, "calls.json")}},
		{name: "duplicate seeds", args: []string{"--seeds", "1,1", "--out", filepath.Join(dir, "seeds.json")}},
		{name: "positional argument", args: []string{"--seeds", "1", "--out", filepath.Join(dir, "positional.json"), "extra"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"examples", "run", "roles"}, test.args...)
			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), args, &stdout, &stderr)
			if err == nil || ExitCode(err) != exitUsage {
				t.Fatalf("Run(%v) error = %v, want usage error", args, err)
			}
		})
	}
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"examples", "run", "roles", "--help"}, &stdout, &stderr)
	if err != nil || (!strings.Contains(stdout.String(), "not core capability") && !strings.Contains(stdout.String(), "never added")) {
		t.Fatalf("help error = %v, output: %s", err, stdout.String())
	}
}

func TestExamplesListIncludesRoles(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "list"}, &stdout, &stderr); err != nil {
		t.Fatalf("examples list: %v", err)
	}
	var entries []map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &entries); err != nil {
		t.Fatalf("decode examples list: %v", err)
	}
	for _, entry := range entries {
		if entry["name"] == "roles" && entry["profile"] == "fixture" && entry["description"] != "" {
			return
		}
	}
	t.Fatalf("examples list omits roles: %s", stdout.String())
}
