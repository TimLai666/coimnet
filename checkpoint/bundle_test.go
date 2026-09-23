package checkpoint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type bundleTestValue struct {
	Name   string
	Values []float64
	Rows   [][]float64
	Counts map[string]int
}

func bundleTestManifest() BundleManifest {
	return BundleManifest{
		Kind:           BundleIndividual,
		DocumentSchema: "test-document/v1",
		Topology:       TopologyFingerprint{Nodes: 2, Edges: 1, SHA256: "topology"},
	}
}

func bundleTestValueFixture() bundleTestValue {
	return bundleTestValue{
		Name:   "snapshot",
		Values: []float64{1.25, -2.5},
		Rows:   [][]float64{{1, 2}, {3}},
		Counts: map[string]int{"a": 3, "b": 5},
	}
}

func writeTestBundle(t *testing.T, dir string) {
	t.Helper()
	if err := writeBundle(context.Background(), dir, bundleTestManifest(), bundleTestValueFixture()); err != nil {
		t.Fatal(err)
	}
}

func TestBundleRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "snapshot")
	want := bundleTestValueFixture()
	if err := writeBundle(context.Background(), dir, bundleTestManifest(), want); err != nil {
		t.Fatal(err)
	}

	var got bundleTestValue
	manifest, err := readBundle(context.Background(), dir, BundleIndividual, &got)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch:\n got: %#v\nwant: %#v", got, want)
	}
	payloadPath := filepath.Join(dir, bundlePayloadFile)
	payload, err := os.ReadFile(payloadPath)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.PayloadBytes != int64(len(payload)) {
		t.Fatalf("manifest payload size = %d, file size = %d", manifest.PayloadBytes, len(payload))
	}
	digest := sha256.Sum256(payload)
	if manifest.PayloadSHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("manifest checksum = %q, file checksum = %x", manifest.PayloadSHA256, digest)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name() != bundleManifestFile || entries[1].Name() != bundlePayloadFile {
		t.Fatalf("bundle directory entries = %#v, want manifest.json and payload.gob", entries)
	}
}

func TestBundleRefusesExistingDir(t *testing.T) {
	for _, existingFile := range []bool{false, true} {
		name := "directory"
		if existingFile {
			name = "file"
		}
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			dir := filepath.Join(parent, "existing")
			if existingFile {
				if err := os.WriteFile(dir, []byte("keep"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			err := writeBundle(context.Background(), dir, bundleTestManifest(), bundleTestValueFixture())
			if err == nil || !strings.Contains(err.Error(), dir) {
				t.Fatalf("writeBundle error = %v, want error naming %q", err, dir)
			}
			if existingFile {
				data, readErr := os.ReadFile(dir)
				if readErr != nil || string(data) != "keep" {
					t.Fatalf("existing file changed: %q, %v", data, readErr)
				}
			} else {
				entries, readErr := os.ReadDir(dir)
				if readErr != nil || len(entries) != 0 {
					t.Fatalf("existing directory changed: %#v, %v", entries, readErr)
				}
			}
		})
	}

	missingParent := filepath.Join(t.TempDir(), "missing", "bundle")
	if err := writeBundle(context.Background(), missingParent, bundleTestManifest(), bundleTestValueFixture()); err == nil {
		t.Fatal("writeBundle succeeded with a missing parent")
	}
}

func TestBundleIncompleteWithoutManifest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "snapshot")
	writeTestBundle(t, dir)
	if err := os.Remove(filepath.Join(dir, bundleManifestFile)); err != nil {
		t.Fatal(err)
	}
	var got bundleTestValue
	if _, err := readBundle(context.Background(), dir, BundleIndividual, &got); err == nil || !strings.Contains(err.Error(), "incomplete bundle") {
		t.Fatalf("readBundle error = %v, want incomplete bundle", err)
	}
}

func TestBundleDetectsTampering(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, dir string)
		want   string
	}{
		{
			name: "payload byte",
			mutate: func(t *testing.T, dir string) {
				path := filepath.Join(dir, bundlePayloadFile)
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				data[0] ^= 1
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: "checksum mismatch",
		},
		{
			name: "short payload",
			mutate: func(t *testing.T, dir string) {
				path := filepath.Join(dir, bundlePayloadFile)
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data[:len(data)-1], 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: "size",
		},
		{
			name: "unknown manifest field",
			mutate: func(t *testing.T, dir string) {
				mutateBundleManifest(t, dir, func(fields map[string]any) { fields["unexpected"] = true })
			},
			want: "unknown field",
		},
		{
			name: "kind mismatch",
			mutate: func(t *testing.T, dir string) {
				mutateBundleManifest(t, dir, func(fields map[string]any) { fields["kind"] = BundleTraining })
			},
			want: "kind",
		},
		{
			name: "schema mismatch",
			mutate: func(t *testing.T, dir string) {
				mutateBundleManifest(t, dir, func(fields map[string]any) { fields["schema_version"] = "other/v1" })
			},
			want: "schema",
		},
		{
			name: "payload path traversal",
			mutate: func(t *testing.T, dir string) {
				mutateBundleManifest(t, dir, func(fields map[string]any) { fields["payload_file"] = "../x" })
			},
			want: "payload_file",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "snapshot")
			writeTestBundle(t, dir)
			tt.mutate(t, dir)
			var got bundleTestValue
			if _, err := readBundle(context.Background(), dir, BundleIndividual, &got); err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.want)) {
				t.Fatalf("readBundle error = %v, want text %q", err, tt.want)
			}
		})
	}
}

func mutateBundleManifest(t *testing.T, dir string, mutate func(map[string]any)) {
	t.Helper()
	path := filepath.Join(dir, bundleManifestFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	mutate(fields)
	data, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBundleWriteFailureLeavesNothing(t *testing.T) {
	badDir := filepath.Join(t.TempDir(), "bad-value")
	badValue := struct{ Channel chan int }{Channel: make(chan int)}
	if err := writeBundle(context.Background(), badDir, bundleTestManifest(), badValue); err == nil {
		t.Fatal("writeBundle succeeded with an unencodable value")
	}
	if _, err := os.Stat(badDir); !os.IsNotExist(err) {
		t.Fatalf("failed bundle directory exists: %v", err)
	}

	canceledDir := filepath.Join(t.TempDir(), "canceled")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := writeBundle(ctx, canceledDir, bundleTestManifest(), bundleTestValueFixture()); err == nil {
		t.Fatal("writeBundle succeeded with a canceled context")
	}
	if _, err := os.Stat(canceledDir); !os.IsNotExist(err) {
		t.Fatalf("canceled bundle directory exists: %v", err)
	}
}

func TestBundleRejectsBadManifestInput(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*BundleManifest)
	}{
		{name: "unknown kind", change: func(m *BundleManifest) { m.Kind = "other" }},
		{name: "empty document schema", change: func(m *BundleManifest) { m.DocumentSchema = "" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "invalid")
			manifest := bundleTestManifest()
			tt.change(&manifest)
			if err := writeBundle(context.Background(), dir, manifest, bundleTestValueFixture()); err == nil {
				t.Fatal("writeBundle accepted invalid manifest input")
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatalf("invalid manifest created bundle directory: %v", err)
			}
		})
	}
}
