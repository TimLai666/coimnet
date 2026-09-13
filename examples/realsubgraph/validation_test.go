package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/connectome"
)

func TestRunRejectsWrongStoreOrSelection(t *testing.T) {
	path, predicate, original := makeFixtureStore(t, fixtureWeights)
	corruptPath := filepath.Join(t.TempDir(), "corrupt.coimgraph")
	corrupt := append([]byte(nil), original...)
	corrupt[0] ^= 1
	if err := os.WriteFile(corruptPath, corrupt, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, store, hash, profile, message string
	}{
		{"missing", filepath.Join(t.TempDir(), "missing"), predicate, "fixture", ""},
		{"corrupt", corruptPath, predicate, "fixture", ""},
		{"predicate", path, strings.Repeat("0", 64), "fixture", "does not match"},
		{"profile", path, predicate, "real-subgraph", "provenance"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, stderr bytes.Buffer
			err := run(context.Background(), []string{"--store", tc.store, "--expected-predicate-hash", tc.hash, "--profile", tc.profile}, &out, &stderr)
			if err == nil || !strings.Contains(err.Error(), tc.message) || out.Len() != 0 {
				t.Fatalf("err=%v stdout=%q", err, out.String())
			}
		})
	}
	unchanged, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(unchanged, original) {
		t.Fatalf("input store changed: %v", err)
	}
}

func TestRunRejectsImpostorRealProfile(t *testing.T) {
	for _, version := range []string{"impostor-v0", "v1.0"} {
		t.Run(version, func(t *testing.T) {
			path, predicate, _ := makeFixtureStore(t, fixtureWeights, func(m *connectome.DatasetManifest) {
				m.Dataset, m.Namespace, m.SourceVersion = "MaleCNS", "malecns-v1.0", version
			})
			var out, stderr bytes.Buffer
			err := run(context.Background(), []string{"--store", path, "--expected-predicate-hash", predicate}, &out, &stderr)
			if err == nil || !strings.Contains(err.Error(), "provenance") || out.Len() != 0 {
				t.Fatalf("impostor accepted: err=%v stdout bytes=%d", err, out.Len())
			}
		})
	}
}

func TestRunPropagatesFlagDiagnosticFailure(t *testing.T) {
	err := run(context.Background(), []string{"--unknown"}, &bytes.Buffer{}, failingWriter{})
	if err == nil || !strings.Contains(err.Error(), "output failed") {
		t.Fatalf("diagnostic output error = %v", err)
	}
}

func TestRunRejectsSubgraphWithoutInternalEdges(t *testing.T) {
	path, predicate, original := makeFixtureStore(t, nil)
	var out, stderr bytes.Buffer
	err := run(context.Background(), []string{"--store", path, "--expected-predicate-hash", predicate, "--profile", "fixture"}, &out, &stderr)
	if err == nil || !strings.Contains(err.Error(), "0 edges") || out.Len() != 0 {
		t.Fatalf("err=%v stdout=%q", err, out.String())
	}
	unchanged, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(unchanged, original) {
		t.Fatalf("input store changed: %v", err)
	}
}
