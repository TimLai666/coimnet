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

func TestExamplesRunVideo(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "v")
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"examples", "run", "video", "--seeds", "1", "--epochs", "2", "--packager", "none", "--out-dir", dir}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run video: %v; stderr: %s", err, stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var report struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &report); err != nil || report.SchemaVersion != "coimnet-video/v1" {
		t.Fatalf("report schema = %q, error %v", report.SchemaVersion, err)
	}
	clips, err := os.ReadDir(filepath.Join(dir, "clips"))
	if err != nil || len(clips) != 6 {
		t.Fatalf("clip directories = %d, error %v; want 6", len(clips), err)
	}
	if _, err := os.Stat(filepath.Join(dir, "packaging.json")); !os.IsNotExist(err) {
		t.Fatalf("packaging.json exists or could not be inspected: %v", err)
	}
	pattern := `^video: 1 seeds, seen correct [0-9]+\.[0-9]{2} of [0-9]+ \(frozen core [0-9]+\.[0-9]{2}\), held-out correct [0-9]+\.[0-9]{2} of [0-9]+, seen in sync [0-9]+\.[0-9]{2} of [0-9]+, core disconnect changed [0-9]+/1, packaging skipped\n$`
	if ok, err := regexp.MatchString(pattern, stdout.String()); err != nil || !ok {
		t.Fatalf("summary = %q, match %t, error %v", stdout.String(), ok, err)
	}
	t.Logf("%s", strings.TrimSpace(stdout.String()))
}

func TestExamplesRunVideoRecordsAbsentPackager(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "v")
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"examples", "run", "video", "--seeds", "1", "--epochs", "2", "--packager", "coimnet-missing-packager", "--out-dir", dir}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run video with missing packager: %v; stderr: %s", err, stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(dir, "packaging.json"))
	if err != nil {
		t.Fatalf("read packaging.json: %v", err)
	}
	var packaging struct {
		Packager map[string]struct {
			Tool   string `json:"tool"`
			Status string `json:"status"`
		} `json:"packager"`
	}
	if err := json.Unmarshal(data, &packaging); err != nil {
		t.Fatalf("decode packaging.json: %v", err)
	}
	if len(packaging.Packager) != 6 {
		t.Fatalf("packager entries = %d; want 6", len(packaging.Packager))
	}
	for clip, report := range packaging.Packager {
		if report.Status != "tool_absent" || report.Tool != "coimnet-missing-packager" {
			t.Errorf("%s package report = %#v; want tool_absent", clip, report)
		}
	}
	if !strings.Contains(stdout.String(), "6 absent") {
		t.Fatalf("summary = %q; want 6 absent", stdout.String())
	}
	for clip, report := range packaging.Packager {
		t.Logf("packaging.json %s.status=%s", clip, report.Status)
		break
	}
	t.Logf("%s", strings.TrimSpace(stdout.String()))
}

func TestExamplesRunVideoRejects(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "existing")
	if err := os.Mkdir(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "missing output directory", args: []string{"--seeds", "1", "--epochs", "2"}},
		{name: "existing output directory", args: []string{"--out-dir", existing}},
		{name: "zero epochs", args: []string{"--epochs", "0", "--out-dir", filepath.Join(t.TempDir(), "zero")}},
		{name: "duplicate seeds", args: []string{"--seeds", "1,1", "--out-dir", filepath.Join(t.TempDir(), "duplicate")}},
		{name: "positional argument", args: []string{"--out-dir", filepath.Join(t.TempDir(), "position"), "extra"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"examples", "run", "video"}, test.args...)
			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), args, &stdout, &stderr)
			if err == nil || ExitCode(err) != exitUsage {
				t.Fatalf("Run(%v) error = %v, want usage error", args, err)
			}
		})
	}
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"examples", "run", "video", "--help"}, &stdout, &stderr)
	if err != nil || !strings.Contains(stdout.String(), "fixed clamp decoder") {
		t.Fatalf("help error = %v, output: %s", err, stdout.String())
	}
}

func TestExamplesListIncludesVideo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "list"}, &stdout, &stderr); err != nil {
		t.Fatalf("examples list: %v", err)
	}
	var entries []map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &entries); err != nil {
		t.Fatalf("decode examples list: %v", err)
	}
	for _, entry := range entries {
		if entry["name"] == "video" && entry["profile"] == "fixture" && entry["description"] != "" {
			return
		}
	}
	t.Fatalf("examples list omits video: %s", stdout.String())
}
