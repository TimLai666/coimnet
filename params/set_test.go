package params

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func setFixture(t *testing.T) (*Set, string) {
	t.Helper()
	dir := t.TempDir()
	graph := graphFixture(t, dir)
	rules := fixtureRules(t, dir, NormalizerPostTotal, baseSignRule())
	set, _, err := Derive(context.Background(), graph, rules, dir, fixtureLimits(t))
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	return set, dir
}

func testLoadLimits() LoadLimits {
	return LoadLimits{MaxFileBytes: 8 << 20, MaxFooterBytes: 1 << 20, MaxMemoryBytes: 32 << 20}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	set, dir := setFixture(t)
	path := filepath.Join(dir, "params.coimparams")
	receipt, err := Save(context.Background(), path, set)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Path != path || receipt.Bytes != info.Size() || len(receipt.SHA256) != 64 || receipt.NodeCount != 4 || receipt.EdgeCount != 6 {
		t.Fatalf("receipt = %+v", receipt)
	}
	if len(receipt.Sections) == 0 {
		t.Fatal("receipt lists no sections")
	}

	loaded, err := Load(context.Background(), path, testLoadLimits())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Source != set.Source || loaded.RulesHash != set.RulesHash || loaded.GraphHashes != set.GraphHashes {
		t.Fatalf("identity differs: %+v vs %+v", loaded.GraphHashes, set.GraphHashes)
	}
	for i := range set.EdgeWeight {
		if loaded.EdgeWeight[i] != set.EdgeWeight[i] || loaded.EdgeSign[i] != set.EdgeSign[i] ||
			loaded.EdgeSignConfidence[i] != set.EdgeSignConfidence[i] || loaded.EdgeTransmitter[i] != set.EdgeTransmitter[i] ||
			loaded.EdgeMatchedSynapses[i] != set.EdgeMatchedSynapses[i] {
			t.Fatalf("edge %d differs after a round trip", i)
		}
	}
	if !equalInt64(loaded.NodePreTotal, set.NodePreTotal) || !equalInt64(loaded.NodePostTotal, set.NodePostTotal) || !equalString(loaded.NodePrimaryROI, set.NodePrimaryROI) {
		t.Fatal("node arrays differ after a round trip")
	}
	if loaded.Report.SchemaVersion != ReportSchemaVersion || loaded.Report.Edges.Edges != 6 {
		t.Fatalf("embedded report = %+v", loaded.Report.Edges)
	}
}

func TestSaveIsByteIdenticalAndNeverOverwrites(t *testing.T) {
	set, dir := setFixture(t)
	first := filepath.Join(dir, "a.coimparams")
	second := filepath.Join(dir, "b.coimparams")
	if _, err := Save(context.Background(), first, set); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(context.Background(), second, set); err != nil {
		t.Fatal(err)
	}
	left, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	right, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(left, right) {
		t.Fatal("two saves of the same set produced different bytes")
	}
	if _, err := Save(context.Background(), first, set); err == nil {
		t.Fatal("Save overwrote an existing file")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if len(entry.Name()) > 0 && entry.Name()[0] == '.' {
			t.Fatalf("a refused save left the temporary file %s behind", entry.Name())
		}
	}
}

func TestLoadRejectsEveryTamperedByte(t *testing.T) {
	set, dir := setFixture(t)
	path := filepath.Join(dir, "params.coimparams")
	if _, err := Save(context.Background(), path, set); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]func([]byte) []byte{
		"header magic":  func(b []byte) []byte { b[0] ^= 0xff; return b },
		"trailer magic": func(b []byte) []byte { b[len(b)-1] ^= 0xff; return b },
		"section byte":  func(b []byte) []byte { b[16] ^= 0x01; return b },
		"middle byte":   func(b []byte) []byte { b[len(b)/2] ^= 0x01; return b },
		"footer length": func(b []byte) []byte { b[len(b)-40] ^= 0x7f; return b },
		"footer hash":   func(b []byte) []byte { b[len(b)-20] ^= 0x01; return b },
		"truncated":     func(b []byte) []byte { return b[:len(b)-1] },
		"truncated section": func(b []byte) []byte {
			return append(append([]byte(nil), b[:len(b)/2]...), b[len(b)/2+1:]...)
		},
	}
	for name, tamper := range cases {
		target := filepath.Join(t.TempDir(), "tampered.coimparams")
		data := tamper(append([]byte(nil), original...))
		if err := os.WriteFile(target, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(context.Background(), target, testLoadLimits()); err == nil {
			t.Errorf("%s: Load accepted a tampered file", name)
		}
	}

	for name, limits := range map[string]LoadLimits{
		"file limit":   {MaxFileBytes: 8, MaxFooterBytes: 1 << 20, MaxMemoryBytes: 1 << 20},
		"footer limit": {MaxFileBytes: 8 << 20, MaxFooterBytes: 4, MaxMemoryBytes: 1 << 20},
		"memory limit": {MaxFileBytes: 8 << 20, MaxFooterBytes: 1 << 20, MaxMemoryBytes: 1},
		"zero limit":   {},
	} {
		if _, err := Load(context.Background(), path, limits); err == nil {
			t.Errorf("%s: Load ignored its limit", name)
		} else if name != "zero limit" && !errors.Is(err, ErrCapacity) {
			t.Errorf("%s: Load returned %v, want ErrCapacity", name, err)
		}
	}

	if _, err := Load(context.Background(), filepath.Join(dir, "absent"), testLoadLimits()); err == nil {
		t.Error("Load accepted a missing file")
	}
	if _, err := Load(context.Background(), dir, testLoadLimits()); err == nil {
		t.Error("Load accepted a directory")
	}
}

func TestCheckGraphAcceptsOnlyTheSourceGraph(t *testing.T) {
	dir := t.TempDir()
	graph := graphFixture(t, dir)
	rules := fixtureRules(t, dir, NormalizerPostTotal, baseSignRule())
	set, _, err := Derive(context.Background(), graph, rules, dir, fixtureLimits(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := set.CheckGraph(graph); err != nil {
		t.Fatalf("CheckGraph on the source graph: %v", err)
	}
	if err := set.CheckGraph(nil); err == nil {
		t.Error("CheckGraph accepted a nil graph")
	}

	if err := set.CheckGraph(differentGraphFixture(t)); err == nil {
		t.Error("CheckGraph accepted a graph with different hashes")
	}

	// A set whose stored hashes were edited must be refused too.
	tampered := *set
	tampered.GraphHashes.EdgeOrder = "0000000000000000000000000000000000000000000000000000000000000000"
	if err := tampered.CheckGraph(graph); err == nil {
		t.Error("CheckGraph accepted a set with a wrong edge order hash")
	}
}

func TestSaveRejectsInvalidInput(t *testing.T) {
	set, dir := setFixture(t)
	if _, err := Save(context.Background(), "", set); err == nil {
		t.Error("Save accepted an empty path")
	}
	if _, err := Save(context.Background(), filepath.Join(dir, "params.coimparams"), nil); err == nil {
		t.Error("Save accepted a nil set")
	}
	if _, err := Save(context.Background(), filepath.Join(dir, "absent", "params.coimparams"), set); err == nil {
		t.Error("Save accepted a missing parent directory")
	}
	short := *set
	short.EdgeSign = short.EdgeSign[:1]
	if _, err := Save(context.Background(), filepath.Join(dir, "short.coimparams"), &short); err == nil {
		t.Error("Save accepted mismatched array lengths")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	path := filepath.Join(dir, "cancelled.coimparams")
	if _, err := Save(ctx, path, set); err == nil {
		t.Error("Save accepted a cancelled context")
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("a cancelled Save published a file")
	}
}
