package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestExamplesRunTextgenWritesReport(t *testing.T) {
	out := filepath.Join(t.TempDir(), "textgen.json")
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"examples", "run", "textgen", "--seeds", "1", "--epochs", "1", "--out", out}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run textgen: %v; stderr=%s", err, stderr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var report struct {
		SchemaVersion string `json:"schema_version"`
		Runs          []any  `json:"runs"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.SchemaVersion != "coimnet-textgen/v1" || len(report.Runs) != 1 {
		t.Fatalf("report schema/runs = %q/%d", report.SchemaVersion, len(report.Runs))
	}
	if !regexp.MustCompile(`^textgen: 1 seeds, holdout perplexity [0-9]+\.[0-9]{4} -> [0-9]+\.[0-9]{4}, task accuracy [0-9]+\.[0-9]{4} -> [0-9]+\.[0-9]{4}, teacher calls [0-9]+\n$`).MatchString(stdout.String()) {
		t.Fatalf("summary has wrong format: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestExamplesRunTextgenReadsACorpus(t *testing.T) {
	dir := t.TempDir()
	docs := make([]map[string]any, 4)
	for i := range docs {
		name := fmt.Sprintf("doc-%d.txt", i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("貓吃米。狗看水。鳥吃肉。"), 0o600); err != nil {
			t.Fatal(err)
		}
		docs[i] = map[string]any{
			"id": fmt.Sprintf("doc-%d", i), "source": fmt.Sprintf("source-%d", i/2), "path": name,
			"license": map[string]string{"holder": "Example holder", "terms": "Research use", "source": "manifest"},
		}
	}
	manifest := map[string]any{
		"schema":    "coimnet-textgen-corpus/v1",
		"scope":     map[string]string{"kind": "real", "language": "zh-Hant", "note": "Licensed sample corpus"},
		"documents": docs,
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestData, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err = Run(context.Background(), []string{"examples", "run", "textgen", "--seeds", "1", "--epochs", "1", "--corpus", manifestPath}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run with corpus: %v; stderr=%s", err, stderr.String())
	}
	var report struct {
		DataScope struct {
			Kind     string `json:"kind"`
			Language string `json:"language"`
			Note     string `json:"note"`
		} `json:"data_scope"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.DataScope.Kind != "real" || report.DataScope.Language != "zh-Hant" || report.DataScope.Note != "Licensed sample corpus" {
		t.Fatalf("data_scope = %+v", report.DataScope)
	}
}

func TestExamplesRunTextgenRejects(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "existing.json")
	if err := os.WriteFile(existing, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
		code int
		flag string
	}{
		{name: "empty seeds", args: []string{"--seeds", ""}, code: exitUsage, flag: "--seeds"},
		{name: "zero epochs", args: []string{"--epochs", "0"}, code: exitUsage, flag: "--epochs"},
		{name: "existing output", args: []string{"--out", existing}, code: exitUsage, flag: "--out"},
		{name: "missing corpus", args: []string{"--corpus", filepath.Join(t.TempDir(), "missing.json")}, code: 1, flag: "--corpus"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := append([]string{"examples", "run", "textgen"}, tc.args...)
			err := Run(context.Background(), args, &stdout, &stderr)
			if err == nil {
				t.Fatal("expected error")
			}
			if code := ExitCode(err); code != tc.code {
				t.Fatalf("exit code = %d, want %d: %v", code, tc.code, err)
			}
			if !strings.Contains(err.Error(), tc.flag) {
				t.Errorf("error %q does not name %s", err, tc.flag)
			}
		})
	}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "run", "textgen", "--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("help: %v; stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "student mode") {
		t.Fatalf("help does not describe student mode:\n%s", stdout.String())
	}
}

func TestExamplesListIncludesTextgen(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("examples help: %v; stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "run textgen [flags]") {
		t.Fatalf("examples help omits textgen:\n%s", stdout.String())
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"examples", "list"}, &stdout, &stderr); err != nil {
		t.Fatalf("examples list: %v; stderr=%s", err, stderr.String())
	}
	var entries []struct {
		Name    string `json:"name"`
		Profile string `json:"profile"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &entries); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name == "textgen" && entry.Profile == "fixture" {
			return
		}
	}
	t.Fatal("examples list has no textgen fixture entry")
}
