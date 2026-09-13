package connectome

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rewriteStoreFooter regenerates the footer checksum so failures exercise
// validation of the content, rather than a stale checksum.
func rewriteStoreFooter(t *testing.T, data []byte, mutate func(*storeFooter), padding int) []byte {
	t.Helper()
	trailer := data[len(data)-storeTrailerBytes:]
	start := len(data) - storeTrailerBytes - int(binary.BigEndian.Uint32(trailer[:4]))
	var footer storeFooter
	if err := json.Unmarshal(data[start:len(data)-storeTrailerBytes], &footer); err != nil {
		t.Fatal(err)
	}
	mutate(&footer)
	encoded, err := json.Marshal(footer)
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, []byte(strings.Repeat(" ", padding))...)
	sum := sha256.Sum256(encoded)
	var tail [storeTrailerBytes]byte
	binary.BigEndian.PutUint32(tail[:4], uint32(len(encoded)))
	copy(tail[4:], sum[:])
	copy(tail[4+sha256.Size:], storeMagic)
	out := append([]byte(nil), data[:start]...)
	out = append(out, encoded...)
	return append(out, tail[:]...)
}

func TestSaveRejectsUninitializedGraphWithoutPublishing(t *testing.T) {
	dir := t.TempDir()
	if _, err := Save(context.Background(), filepath.Join(dir, "graph"), &Graph{}); err == nil {
		t.Fatal("uninitialized graph accepted")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("unexpected files: %v, %v", entries, err)
	}
}

func TestLoadBoundsJSONBuffersAndRejectsUnknownConverter(t *testing.T) {
	manifest, _ := happyFixture(t, []int{11})
	g := build(t, manifest, fixtureLimits()).Graph
	path := filepath.Join(t.TempDir(), "original")
	if _, err := Save(context.Background(), path, g); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		mutate   func(*storeFooter)
		padding  int
		capacity bool
	}{
		{"footer-buffer", func(*storeFooter) {}, 10 << 20, true},
		{"unknown-converter", func(f *storeFooter) { f.ConverterVersion = "unknown/v99" }, 0, false},
		{"node-overflow", func(f *storeFooter) { f.IndexWidth = 8; f.NodeCount = 51240955760304311 }, 0, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := rewriteStoreFooter(t, original, test.mutate, test.padding)
			p := filepath.Join(t.TempDir(), "modified")
			if err := os.WriteFile(p, data, 0600); err != nil {
				t.Fatal(err)
			}
			limits := storeLimits()
			limits.MaxFooterBytes = 16 << 20
			limits.MaxMemoryBytes = 8 << 20
			_, err := Load(context.Background(), p, limits)
			if test.capacity {
				if !errors.Is(err, ErrCapacity) {
					t.Fatalf("want capacity rejection, got %v", err)
				}
			} else if !errors.Is(err, ErrStoreCorrupt) {
				t.Fatalf("want corrupt rejection, got %v", err)
			}
		})
	}
	// A large, otherwise valid report must also be charged before allocation.
	g.report.Sources[0].URL = strings.Repeat("d", 10<<20)
	g.report.Hashes.Report, err = g.report.hash()
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "report")
	if _, err := Save(context.Background(), p, g); err != nil {
		t.Fatal(err)
	}
	limits := storeLimits()
	limits.MaxFooterBytes = 16 << 20
	limits.MaxMemoryBytes = 8 << 20
	if _, err := Load(context.Background(), p, limits); !errors.Is(err, ErrCapacity) {
		t.Fatalf("report capacity: %v", err)
	}
}

func TestStoreRejectsOverflowingNodeAllocation(t *testing.T) {
	f := storeFooter{SchemaVersion: StoreSchemaVersion, ConverterVersion: ConverterVersion, Namespace: "n", Dataset: "d", EdgeView: EdgeViewRows, IndexWidth: 8, NodeCount: 51240955760304311}
	if err := validateFooter(f, math.MaxInt64); err == nil || !strings.Contains(err.Error(), "overflow") {
		t.Fatalf("want count overflow before section validation: %v", err)
	}
}
