package coimnet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoTelemetryProjectTempBoundary(t *testing.T) {
	for _, tc := range []struct {
		name    string
		path    string
		wantHit bool
	}{
		{"project-temp", "runs/.tmp/generated.go", false},
		{"production-source", "modulation/source.go", true},
		{"nested-temp-name", "example/runs/.tmp/source.go", true},
		{"run-outside-temp", "runs/source.go", true},
		{"temp-name-file", "runs/source.go", true},
		{"test-source", "modulation/source_test.go", false},
		{"git-internals", ".git/generated.go", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, filepath.FromSlash(tc.path))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if tc.name == "temp-name-file" {
				if err := os.WriteFile(filepath.Join(root, "runs", ".tmp"), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(path, []byte("package fixture\n// telemetry\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			hits, err := scanTelemetry(root)
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantHit {
				if len(hits) != 1 || !strings.Contains(hits[0], filepath.FromSlash(tc.path)+": telemetry") {
					t.Fatalf("hits = %v, want offending source %s", hits, tc.path)
				}
			} else if len(hits) != 0 {
				t.Fatalf("hits = %v, want none", hits)
			}
		})
	}
}

func TestNoTelemetrySourceErrorsRemainVisible(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing-source-root")
	hits, err := scanTelemetry(root)
	if !os.IsNotExist(err) || len(hits) != 0 {
		t.Fatalf("hits = %v, err = %v; want source walk error", hits, err)
	}
}
