package connectome

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func storeLimits() StoreLimits {
	return StoreLimits{MaxFileBytes: 64 << 20, MaxFooterBytes: 1 << 20, MaxMemoryBytes: 64 << 20}
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func assertSameGraph(t *testing.T, want, got *Graph) {
	t.Helper()
	if want.NodeCount() != got.NodeCount() || want.EdgeCount() != got.EdgeCount() || want.Namespace() != got.Namespace() {
		t.Fatalf("counts differ: %d/%d vs %d/%d", want.NodeCount(), want.EdgeCount(), got.NodeCount(), got.EdgeCount())
	}
	if !reflect.DeepEqual(want.NeuronIDs(), got.NeuronIDs()) {
		t.Fatalf("neuron IDs differ")
	}
	for i := uint64(0); i < want.NodeCount(); i++ {
		a, _ := want.Node(i)
		b, err := got.Node(i)
		if err != nil || !reflect.DeepEqual(a, b) {
			t.Fatalf("node %d differs:\n%#v\n%#v err=%v", i, a, b, err)
		}
		index, err := got.IndexOf(a.ID)
		if err != nil || index != i {
			t.Fatalf("IndexOf(%v) = %d, %v", a.ID, index, err)
		}
	}
	var wantEdges, gotEdges []EdgeRecord
	_ = want.StreamAnnotatedEdges(context.Background(), func(e EdgeRecord) error { wantEdges = append(wantEdges, e); return nil })
	if err := got.StreamAnnotatedEdges(context.Background(), func(e EdgeRecord) error { gotEdges = append(gotEdges, e); return nil }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wantEdges, gotEdges) {
		t.Fatalf("edges differ:\n%#v\n%#v", wantEdges, gotEdges)
	}
	if !reflect.DeepEqual(want.Report(), got.Report()) {
		t.Fatalf("reports differ:\n%#v\n%#v", want.Report(), got.Report())
	}
}

func TestStoreRoundTripPreservesGraphReportAndBytes(t *testing.T) {
	manifest, _ := happyFixture(t, []int{7, 4})
	built := build(t, manifest, fixtureLimits())
	dir := t.TempDir()
	first := filepath.Join(dir, "graph.coimgraph")
	receipt, err := Save(context.Background(), first, built.Graph)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(first)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Path != first || receipt.Bytes != info.Size() || receipt.SHA256 != fileSHA256(t, first) || receipt.NodeCount != 5 || receipt.EdgeCount != 7 || len(receipt.Sections) != 5 || !receipt.DurabilityConfirmed || len(receipt.FooterSHA256) != 64 {
		t.Fatalf("receipt = %#v", receipt)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v", entries)
	}
	second := filepath.Join(dir, "again.coimgraph")
	if _, err := Save(context.Background(), second, built.Graph); err != nil {
		t.Fatal(err)
	}
	if fileSHA256(t, first) != fileSHA256(t, second) {
		t.Fatal("saving the same graph twice produced different bytes")
	}

	loaded, err := Load(context.Background(), first, storeLimits())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	assertSameGraph(t, built.Graph, loaded)

	// The loaded graph still streams the raw view from the verified source.
	rows := 0
	if err := loaded.StreamRawSegments(context.Background(), func(RawSegment) error { rows++; return nil }); err != nil || rows != 11 {
		t.Fatalf("raw rows after load = %d, err=%v", rows, err)
	}
	weights := manifest.Files[0].Path
	contents, _ := os.ReadFile(weights)
	if err := os.WriteFile(weights, append(contents, 0), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := loaded.StreamRawSegments(context.Background(), func(RawSegment) error { return nil }); !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("changed weights after load: %v", err)
	}
	// Loading does not depend on the weights file being unchanged.
	if _, err := Load(context.Background(), first, storeLimits()); err != nil {
		t.Fatalf("Load after weights change: %v", err)
	}
}

func TestStoreRoundTripAggregatedAndEmptyGraphs(t *testing.T) {
	dir := t.TempDir()
	rows := []weightRow{{i64(10), i64(20), i64(3)}, {i64(10), i64(20), i64(4)}, {i64(20), i64(30), i64(1)}, {i64(30), i64(30), i64(2)}, {i64(30), i64(30), i64(5)}}
	weights := writeWeights(t, dir, rows, []int{3, 2})
	annotations := writeAnnotations(t, dir, happyAnnotations(), []int{10})
	nt := writeNeurotransmitters(t, dir, happyNeurotransmitters())
	additive := fixtureManifest(t, weights, annotations, nt)
	additive.DuplicateSemantics = DuplicateSemanticsAdditivePartitions
	aggregated, err := Build(context.Background(), BuildRequest{Manifest: additive, Limits: fixtureLimits(), TempDir: t.TempDir(), EdgeView: EdgeViewAggregatedPairs})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "aggregated.coimgraph")
	if _, err := Save(context.Background(), path, aggregated.Graph); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(context.Background(), path, storeLimits())
	if err != nil {
		t.Fatal(err)
	}
	assertSameGraph(t, aggregated.Graph, loaded)

	emptyDir := t.TempDir()
	emptyAnnotations := writeAnnotations(t, emptyDir, []annotationRow{{id: i64(7), status: str("Orphan")}}, []int{1})
	emptyWeights := writeWeights(t, emptyDir, nil, []int{0})
	emptyNT := writeNeurotransmitters(t, emptyDir, nil)
	empty := build(t, fixtureManifest(t, emptyWeights, emptyAnnotations, emptyNT), fixtureLimits())
	emptyPath := filepath.Join(t.TempDir(), "empty.coimgraph")
	if _, err := Save(context.Background(), emptyPath, empty.Graph); err != nil {
		t.Fatal(err)
	}
	loadedEmpty, err := Load(context.Background(), emptyPath, storeLimits())
	if err != nil {
		t.Fatal(err)
	}
	assertSameGraph(t, empty.Graph, loadedEmpty)
}

func TestSaveRefusesExistingTargetMissingParentAndCancellation(t *testing.T) {
	manifest, _ := happyFixture(t, []int{11})
	built := build(t, manifest, fixtureLimits())
	dir := t.TempDir()
	path := filepath.Join(dir, "graph.coimgraph")
	if _, err := Save(context.Background(), path, built.Graph); err != nil {
		t.Fatal(err)
	}
	before := fileSHA256(t, path)
	if _, err := Save(context.Background(), path, built.Graph); err == nil {
		t.Fatal("Save overwrote an existing store")
	}
	if fileSHA256(t, path) != before {
		t.Fatal("existing store was modified")
	}
	if _, err := Save(context.Background(), filepath.Join(dir, "missing", "graph.coimgraph"), built.Graph); err == nil {
		t.Fatal("Save accepted a missing parent directory")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Save(canceled, filepath.Join(dir, "canceled.coimgraph"), built.Graph); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Save: %v", err)
	}
	if _, err := Save(context.Background(), "", built.Graph); err == nil {
		t.Fatal("Save accepted an empty path")
	}
	if _, err := Save(context.Background(), filepath.Join(dir, "nil.coimgraph"), nil); err == nil {
		t.Fatal("Save accepted a nil graph")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "graph.coimgraph" {
		t.Fatalf("directory after failed saves: %v", entries)
	}
	if _, err := Load(canceled, path, storeLimits()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Load: %v", err)
	}
}

func TestLoadRejectsTamperingTruncationAndLimits(t *testing.T) {
	manifest, _ := happyFixture(t, []int{11})
	built := build(t, manifest, fixtureLimits())
	dir := t.TempDir()
	path := filepath.Join(dir, "graph.coimgraph")
	receipt, err := Save(context.Background(), path, built.Graph)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	write := func(name string, data []byte) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	corruptSections := map[string]int64{}
	for _, section := range receipt.Sections {
		corruptSections[section.Name] = section.Offset + section.Length/2
	}
	for name, offset := range corruptSections {
		if offset >= int64(len(original)) {
			continue
		}
		data := bytes.Clone(original)
		data[offset] ^= 0x01
		if _, err := Load(context.Background(), write(name+".corrupt", data), storeLimits()); !errors.Is(err, ErrStoreCorrupt) {
			t.Fatalf("flipped byte in %s: %v", name, err)
		}
	}
	footer := bytes.Clone(original)
	footer[len(footer)-8-32-4-2] ^= 0x01
	if _, err := Load(context.Background(), write("footer.corrupt", footer), storeLimits()); !errors.Is(err, ErrStoreCorrupt) {
		t.Fatalf("flipped footer byte: %v", err)
	}
	if _, err := Load(context.Background(), write("truncated", original[:len(original)-1]), storeLimits()); err == nil {
		t.Fatal("truncated store accepted")
	}
	magic := bytes.Clone(original)
	magic[0] = 'X'
	if _, err := Load(context.Background(), write("magic", magic), storeLimits()); err == nil {
		t.Fatal("wrong magic accepted")
	}
	versioned := bytes.Clone(original)
	versioned = bytes.Replace(versioned, []byte(StoreSchemaVersion), []byte("coimnet-graph-store/v9"), 1)
	if _, err := Load(context.Background(), write("version", versioned), storeLimits()); err == nil {
		t.Fatal("unknown schema version accepted")
	}
	if _, err := Load(context.Background(), write("tiny", []byte("COIMGRF1")), storeLimits()); err == nil {
		t.Fatal("tiny file accepted")
	}
	small := storeLimits()
	small.MaxFileBytes = int64(len(original)) - 1
	if _, err := Load(context.Background(), path, small); !errors.Is(err, ErrCapacity) {
		t.Fatalf("file limit: %v", err)
	}
	smallFooter := storeLimits()
	smallFooter.MaxFooterBytes = 16
	if _, err := Load(context.Background(), path, smallFooter); !errors.Is(err, ErrCapacity) {
		t.Fatalf("footer limit: %v", err)
	}
	smallMemory := storeLimits()
	smallMemory.MaxMemoryBytes = 64
	if _, err := Load(context.Background(), path, smallMemory); !errors.Is(err, ErrCapacity) {
		t.Fatalf("memory limit: %v", err)
	}
	for _, limits := range []StoreLimits{{}, {MaxFileBytes: -1, MaxFooterBytes: 1, MaxMemoryBytes: 1}} {
		if _, err := Load(context.Background(), path, limits); err == nil {
			t.Fatalf("accepted limits %#v", limits)
		}
	}
	if _, err := Load(context.Background(), filepath.Join(dir, "absent.coimgraph"), storeLimits()); err == nil {
		t.Fatal("missing file accepted")
	}
}

func TestLoadRejectsInvalidContentWithValidSectionHashes(t *testing.T) {
	manifest, _ := happyFixture(t, []int{11})
	built := build(t, manifest, fixtureLimits())
	dir := t.TempDir()

	// Edge order violated: swap the first two edges in the private store.
	swapped := *built.Graph
	swapped.edges.src32 = append([]uint32(nil), built.Graph.edges.src32...)
	swapped.edges.dst32 = append([]uint32(nil), built.Graph.edges.dst32...)
	swapped.edges.absRow = append([]int64(nil), built.Graph.edges.absRow...)
	swapped.edges.weight = append([]int64(nil), built.Graph.edges.weight...)
	swapped.edges.weightValid = append([]bool(nil), built.Graph.edges.weightValid...)
	swapped.edges.absRow[0], swapped.edges.absRow[1] = swapped.edges.absRow[1], swapped.edges.absRow[0]
	path := filepath.Join(dir, "swapped.coimgraph")
	if _, err := Save(context.Background(), path, &swapped); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(context.Background(), path, storeLimits()); !errors.Is(err, ErrStoreCorrupt) || !strings.Contains(err.Error(), "canonical order") {
		t.Fatalf("out-of-order edges: %v", err)
	}

	// Edge index beyond the node count.
	outOfRange := *built.Graph
	outOfRange.edges.src32 = append([]uint32(nil), built.Graph.edges.src32...)
	outOfRange.edges.src32[0] = uint32(built.Graph.NodeCount())
	path = filepath.Join(dir, "range.coimgraph")
	if _, err := Save(context.Background(), path, &outOfRange); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(context.Background(), path, storeLimits()); !errors.Is(err, ErrStoreCorrupt) || !strings.Contains(err.Error(), "references node index") {
		t.Fatalf("out-of-range index: %v", err)
	}

	// Node IDs not strictly increasing.
	unsorted := *built.Graph
	unsorted.nodeIDs = append([]uint64(nil), built.Graph.nodeIDs...)
	unsorted.nodeIDs[0], unsorted.nodeIDs[1] = unsorted.nodeIDs[1], unsorted.nodeIDs[0]
	path = filepath.Join(dir, "unsorted.coimgraph")
	if _, err := Save(context.Background(), path, &unsorted); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(context.Background(), path, storeLimits()); !errors.Is(err, ErrStoreCorrupt) || !strings.Contains(err.Error(), "not strictly increasing") {
		t.Fatalf("unsorted node IDs: %v", err)
	}

	// Report that does not describe the stored graph.
	mismatched := *built.Graph
	mismatched.report = built.Graph.report.clone()
	mismatched.report.Annotated.Edges++
	path = filepath.Join(dir, "report.coimgraph")
	if _, err := Save(context.Background(), path, &mismatched); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(context.Background(), path, storeLimits()); !errors.Is(err, ErrStoreCorrupt) {
		t.Fatalf("mismatched report: %v", err)
	}
	if err := errors.Unwrap(errors.New("x")); err != nil {
		t.Fatal("unexpected")
	}
	if !strings.HasPrefix(StoreSchemaVersion, "coimnet-graph-store/") {
		t.Fatalf("schema version = %q", StoreSchemaVersion)
	}
}
