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

func TestExamplesRunMediaImage(t *testing.T) {
	testExamplesRunMedia(t, "image")
}

func TestExamplesRunMediaAudio(t *testing.T) {
	testExamplesRunMedia(t, "audio")
}

func testExamplesRunMedia(t *testing.T, modality string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "m")
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"examples", "run", "media", "--modality", modality, "--seeds", "1", "--epochs", "2", "--out-dir", dir}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run media %s: %v; stderr: %s", modality, err, stderr.String())
	}
	report, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var decoded struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(report, &decoded); err != nil || decoded.SchemaVersion != "coimnet-media/v1" {
		t.Fatalf("report schema = %q, error %v", decoded.SchemaVersion, err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "samples"))
	if err != nil || len(entries) != 9 {
		t.Fatalf("sample files = %d, error %v; want 9", len(entries), err)
	}
	if ok, err := regexp.MatchString(`^media: `+modality+`, 1 seeds, seen correct [0-9.]+ of [0-9]+ \(frozen core [0-9.]+\), held-out correct [0-9.]+ of [0-9]+, core disconnect changed [0-9]+/1$`, strings.TrimSpace(stdout.String())); err != nil || !ok {
		t.Fatalf("summary = %q, match %t, error %v", stdout.String(), ok, err)
	}
}

func TestExamplesRunMediaRejects(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "existing")
	if err := os.Mkdir(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "missing modality", args: []string{"--out-dir", t.TempDir()}},
		{name: "unknown modality", args: []string{"--modality", "video", "--out-dir", t.TempDir()}},
		{name: "missing output directory", args: []string{"--modality", "image"}},
		{name: "existing output directory", args: []string{"--modality", "image", "--out-dir", existing}},
		{name: "zero epochs", args: []string{"--modality", "image", "--epochs", "0", "--out-dir", t.TempDir()}},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"examples", "run", "media"}, test.args...)
			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), args, &stdout, &stderr)
			if err == nil || ExitCode(err) != exitUsage {
				t.Fatalf("Run(%v) error = %v, want usage error", args, err)
			}
		})
	}
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"examples", "run", "media", "--help"}, &stdout, &stderr)
	if err != nil || !strings.Contains(stdout.String(), "fixed decoder") {
		t.Fatalf("help error = %v, output: %s", err, stdout.String())
	}
}

func TestExamplesListIncludesMedia(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "list"}, &stdout, &stderr); err != nil {
		t.Fatalf("examples list: %v", err)
	}
	var entries []map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &entries); err != nil {
		t.Fatalf("decode examples list: %v", err)
	}
	for _, entry := range entries {
		if entry["name"] == "media" && entry["profile"] == "fixture" && entry["description"] != "" {
			return
		}
	}
	t.Fatalf("examples list omits media: %s", stdout.String())
}
