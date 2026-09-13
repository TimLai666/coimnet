package connectome

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/signal"
)

func build(t *testing.T, manifest DatasetManifest, limits ResourceLimits) *GraphBuildResult {
	t.Helper()
	result, err := Build(context.Background(), BuildRequest{Manifest: manifest, Limits: limits, TempDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return result
}

type edgeTuple struct {
	source, target uint64
	absoluteRow    int64
	weightValid    bool
	weight         int64
}

func collectEdges(t *testing.T, g *Graph) []edgeTuple {
	t.Helper()
	var edges []edgeTuple
	err := g.StreamAnnotatedEdges(context.Background(), func(e EdgeRecord) error {
		edges = append(edges, edgeTuple{e.Source, e.Target, e.Position.AbsoluteRow, e.Weight.Valid, e.Weight.Value})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return edges
}

func TestBuildHappyPathViewsAndReport(t *testing.T) {
	manifest, _ := happyFixture(t, []int{7, 4})
	result := build(t, manifest, fixtureLimits())
	g, report := result.Graph, result.Report

	if g.NodeCount() != 5 || g.EdgeCount() != 7 {
		t.Fatalf("nodes=%d edges=%d", g.NodeCount(), g.EdgeCount())
	}
	wantIDs := []string{"10", "20", "30", "60", "9007199254740993"}
	ids := g.NeuronIDs()
	if len(ids) != len(wantIDs) {
		t.Fatalf("neuron IDs = %v", ids)
	}
	for i, id := range ids {
		if id.Namespace != testNamespace() || id.ExternalID != wantIDs[i] {
			t.Fatalf("id[%d] = %#v, want %s", i, id, wantIDs[i])
		}
		index, err := g.IndexOf(id)
		if err != nil || index != uint64(i) {
			t.Fatalf("IndexOf(%v) = %d, %v", id, index, err)
		}
		node, err := g.Node(uint64(i))
		if err != nil || node.Index != uint64(i) || node.ID != id {
			t.Fatalf("Node(%d) = %#v, %v", i, node, err)
		}
	}
	if _, err := g.Node(5); err == nil {
		t.Fatal("Node(5) accepted an out-of-range index")
	}
	if _, err := g.IndexOf(signal.NeuronID{Namespace: "other", ExternalID: "10"}); err == nil {
		t.Fatal("IndexOf accepted a foreign namespace")
	}
	if _, err := g.IndexOf(signal.NeuronID{Namespace: testNamespace(), ExternalID: "40"}); err == nil {
		t.Fatal("IndexOf accepted an unselected neuron")
	}

	// Node fields keep null as null and dictionary values as strings.
	n0, _ := g.Node(0)
	if !n0.Status.Valid || n0.Status.Value != "Traced" || !n0.StatusLabel.Valid || n0.StatusLabel.Value != "Reviewed" || !n0.Class.Valid || n0.Class.Value != "A" || !n0.Type.Valid || n0.Type.Value != "T1" || n0.ReceptorType.Valid {
		t.Fatalf("node 0 = %#v", n0)
	}
	if n0.Transmitter.Status != TransmitterPredicted || n0.Transmitter.Consensus.Value != "acetylcholine" || !n0.Transmitter.Confidence.Valid || n0.Transmitter.Confidence.Value != 0.9 {
		t.Fatalf("node 0 transmitter = %#v", n0.Transmitter)
	}
	if n0.Position.Role != RoleAnnotations || n0.Position.Batch != 0 || n0.Position.Row != 0 || n0.Position.AbsoluteRow != 0 {
		t.Fatalf("node 0 position = %#v", n0.Position)
	}
	n1, _ := g.Node(1)
	if n1.Transmitter.Status != TransmitterUnknown || !n1.Transmitter.Consensus.Valid || n1.Transmitter.Consensus.Value != "unclear" || n1.Transmitter.Predicted.Value != "gaba" {
		t.Fatalf("node 1 transmitter = %#v", n1.Transmitter)
	}
	n2, _ := g.Node(2)
	if n2.StatusLabel.Valid || n2.Type.Valid || n2.Transmitter.Status != TransmitterUnknown || n2.Transmitter.Consensus.Valid {
		t.Fatalf("node 2 = %#v", n2)
	}
	n3, _ := g.Node(3)
	if n3.Transmitter.Status != TransmitterNotAvailable || n3.Position.AbsoluteRow != 9 || n3.Position.Batch != 1 || n3.Position.Row != 3 {
		t.Fatalf("node 3 = %#v", n3)
	}

	wantEdges := []edgeTuple{
		{0, 1, 0, true, 3}, {0, 1, 1, true, 4}, {1, 0, 10, true, 7}, {1, 2, 2, true, 1},
		{2, 0, 9, true, 0}, {2, 2, 3, true, 2}, {4, 0, 8, false, 0},
	}
	edges := collectEdges(t, g)
	if len(edges) != len(wantEdges) {
		t.Fatalf("edges = %v", edges)
	}
	for i := range wantEdges {
		if edges[i] != wantEdges[i] {
			t.Fatalf("edge[%d] = %v, want %v", i, edges[i], wantEdges[i])
		}
	}
	var lastEdge EdgeRecord
	_ = g.StreamAnnotatedEdges(context.Background(), func(e EdgeRecord) error { lastEdge = e; return nil })
	if lastEdge.Position.Role != RoleWeights || lastEdge.Position.Batch != 1 || lastEdge.Position.Row != 1 || lastEdge.Aggregation != AggregationNone || lastEdge.Transmitter != EvidenceUnknown || lastEdge.Sign != EvidenceUnknown {
		t.Fatalf("last edge = %#v", lastEdge)
	}

	var nodes int
	if err := g.StreamAnnotatedNodes(context.Background(), func(n NodeRecord) error { nodes++; return nil }); err != nil || nodes != 5 {
		t.Fatalf("streamed %d nodes, err=%v", nodes, err)
	}

	// Raw segments preserve every row including null and negative endpoints.
	var raw []RawSegment
	if err := g.StreamRawSegments(context.Background(), func(r RawSegment) error { raw = append(raw, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(raw) != 11 {
		t.Fatalf("raw rows = %d", len(raw))
	}
	if raw[6].Source.Valid || !raw[6].Target.Valid || raw[6].Target.Value != 20 || raw[7].Target.Value != -7 || raw[8].Weight.Valid || raw[8].Source.Value != hugeID() || raw[9].Weight.Value != 0 || !raw[9].Weight.Valid {
		t.Fatalf("raw rows = %#v", raw[6:10])
	}
	if raw[10].Position.Batch != 1 || raw[10].Position.Row != 3 || raw[10].Position.AbsoluteRow != 10 || raw[10].Namespace != testNamespace() || raw[10].Position.Role != RoleWeights {
		t.Fatalf("raw[10] = %#v", raw[10])
	}

	// Report counts.
	a := report.Annotated
	if a.Name != "annotated_neurons" || a.AnnotationRows != 10 || a.Nodes != 5 || a.Edges != 7 {
		t.Fatalf("annotated = %#v", a)
	}
	if a.AnnotationExclusions != (AnnotationExclusions{NullID: 1, InvalidID: 1, DuplicateID: 1, PredicateFalse: 2, PredicateFieldNull: 1}) {
		t.Fatalf("annotation exclusions = %#v", a.AnnotationExclusions)
	}
	if a.WeightExclusions != (WeightExclusions{NullEndpoint: 1, InvalidID: 1, MissingAnnotation: 1, PredicateFalse: 1}) {
		t.Fatalf("weight exclusions = %#v", a.WeightExclusions)
	}
	if a.UniquePairs != 6 || a.DuplicatePairs != 1 || a.DuplicateRows != 1 || a.SelfLoops != 1 || a.IsolatedNodes != 1 {
		t.Fatalf("annotated structure = %#v", a)
	}
	if a.WeightSum != (ValueSum{Value: 17, Rows: 6, NullRows: 1, Valid: true, Basis: "weight"}) {
		t.Fatalf("annotated weight sum = %#v", a.WeightSum)
	}
	if a.Transmitter.Predicted != 1 || a.Transmitter.Unknown != 4 || a.Transmitter.UnknownRatio != (Ratio{Numerator: 4, Denominator: 5, Value: 0.8, Defined: true, Basis: "consensus_nt"}) || a.Transmitter.Measured != EvidenceNotAvailable {
		t.Fatalf("annotated transmitter = %#v", a.Transmitter)
	}
	if a.Receptor.Status != EvidenceNotDerived || a.Receptor.LabelNullRatio != (Ratio{Numerator: 4, Denominator: 5, Value: 0.8, Defined: true, Basis: "receptorType"}) {
		t.Fatalf("annotated receptor = %#v", a.Receptor)
	}

	r := report.Raw
	if r.Name != "raw_segments" || r.Rows != 11 || r.Source != (EndpointStats{NullRows: 1}) || r.Target != (EndpointStats{NullRows: 0, NegativeRows: 1}) || r.Weight != (ValueStats{NullRows: 1, ZeroRows: 1}) {
		t.Fatalf("raw = %#v", r)
	}
	if r.SelfLoopRows != 1 || r.UniqueEndpoints != 6 || r.UnannotatedUniqueEndpoints != 1 || r.UnannotatedEndpointOccurrences != 1 {
		t.Fatalf("raw endpoints = %#v", r)
	}
	if r.ValidPairRows != 9 || r.UniquePairs != 8 || r.DuplicatePairs != 1 || r.DuplicateRows != 1 {
		t.Fatalf("raw pairs = %#v", r)
	}
	if r.WeightSum != (ValueSum{Value: 25, Rows: 10, NullRows: 1, Valid: true, Basis: "weight"}) {
		t.Fatalf("raw weight sum = %#v", r.WeightSum)
	}
	if r.DuplicateSemantics != DuplicateSemanticsUnknown || r.Aggregation != AggregationNotPermitted {
		t.Fatalf("raw semantics = %#v", r)
	}
	if r.Transmitter.Rows != 5 || r.Transmitter.UniqueIDs != 4 || r.Transmitter.DuplicateRows != 1 || r.Transmitter.UnknownRatio != (Ratio{Numerator: 2, Denominator: 5, Value: 0.4, Defined: true, Basis: "consensus_nt"}) {
		t.Fatalf("raw transmitter = %#v", r.Transmitter)
	}

	if report.SchemaVersion != ReportSchemaVersion || report.ConverterVersion != ConverterVersion || report.Namespace != testNamespace() || report.EdgeView != EdgeViewRows {
		t.Fatalf("report identity = %#v", report)
	}
	if report.Predicate.Canonical != `annotations.status == "Traced"` || len(report.Predicate.Hash) != 64 || report.Predicate.Label == "" {
		t.Fatalf("predicate report = %#v", report.Predicate)
	}
	if len(report.Sources) != 3 {
		t.Fatalf("sources = %#v", report.Sources)
	}
	for _, source := range report.Sources {
		if source.SHA256 == "" || source.SHA256After != source.SHA256 || !source.FingerprintStable || source.Rows == 0 || source.RecordBatches == 0 {
			t.Fatalf("source report = %#v", source)
		}
	}
	if len(report.Hashes.NodeIndex) != 64 || len(report.Hashes.EdgeOrder) != 64 || len(report.Hashes.Report) != 64 || len(report.ManifestHash) != 64 {
		t.Fatalf("hashes = %#v", report.Hashes)
	}
	if report.Sort.Runs < 3 || report.Sort.ReservedPeakBytes <= 0 || report.Sort.BufferedPeakBytes <= 0 || report.Sort.BufferedPeakBytes > report.Sort.ReservedPeakBytes {
		t.Fatalf("sort stats = %#v", report.Sort)
	}
	if a.Transmitter.Status != ColumnMapped || a.Receptor.LabelStatus != ColumnMapped || r.Transmitter.ConsensusStatus != ColumnMapped {
		t.Fatalf("column statuses: %#v %#v %#v", a.Transmitter, a.Receptor, r.Transmitter)
	}
	if len(report.Limitations) == 0 {
		t.Fatal("report has no limitations")
	}

	// Report copies do not share slices with the graph.
	copied := g.Report()
	copied.Limitations[0] = "mutated"
	copied.Sources[0].Path = "mutated"
	if g.Report().Limitations[0] == "mutated" || g.Report().Sources[0].Path == "mutated" {
		t.Fatal("Report shares slices with the caller")
	}
	ids[0].ExternalID = "mutated"
	if g.NeuronIDs()[0].ExternalID == "mutated" {
		t.Fatal("NeuronIDs shares its slice with the caller")
	}

	// Interop: the selected neurons validate a signal mapping.
	mapping, err := signal.NewMapping(signal.MappingSpec{SchemaVersion: signal.CurrentSchemaVersion(), Source: "test", InputShape: []int{2}, Neurons: g.NeuronIDs()[:2]})
	if err != nil {
		t.Fatal(err)
	}
	if err := mapping.ValidateAgainst(g.NeuronIDs()); err != nil {
		t.Fatal(err)
	}
}

func TestBuildIsDeterministicAcrossBatchSegmentationAndRunCounts(t *testing.T) {
	manifestA, _ := happyFixture(t, []int{7, 4})
	manifestB, _ := happyFixture(t, []int{2, 2, 2, 2, 2, 1})
	resultA := build(t, manifestA, fixtureLimits())
	small := fixtureLimits()
	small.SortBufferBytes = 64
	resultB := build(t, manifestB, small)
	if resultB.Report.Sort.Runs <= resultA.Report.Sort.Runs {
		t.Fatalf("small limits produced %d runs, large %d", resultB.Report.Sort.Runs, resultA.Report.Sort.Runs)
	}
	edgesA, edgesB := collectEdges(t, resultA.Graph), collectEdges(t, resultB.Graph)
	if len(edgesA) != len(edgesB) {
		t.Fatalf("edge counts differ: %d vs %d", len(edgesA), len(edgesB))
	}
	for i := range edgesA {
		if edgesA[i] != edgesB[i] {
			t.Fatalf("edge[%d] differs: %v vs %v", i, edgesA[i], edgesB[i])
		}
	}
	ha, hb := resultA.Report.Hashes, resultB.Report.Hashes
	if ha.NodeIndex != hb.NodeIndex || ha.EdgeOrder != hb.EdgeOrder {
		t.Fatalf("hashes differ: %#v vs %#v", ha, hb)
	}
	if resultA.Report.Annotated != resultB.Report.Annotated || resultA.Report.Raw != resultB.Report.Raw {
		t.Fatalf("views differ:\n%#v\n%#v", resultA.Report.Annotated, resultB.Report.Annotated)
	}
	// Byte-identical source files reproduce the report hash.
	resultC := build(t, manifestA, fixtureLimits())
	if resultC.Report.Hashes.Report != resultA.Report.Hashes.Report {
		t.Fatal("report hash is not reproducible for identical inputs and limits")
	}
}

func TestBuildRejectsInvalidManifestSelectionIdentityAndSourceChange(t *testing.T) {
	manifest, _ := happyFixture(t, []int{11})
	ctx := context.Background()
	limits := fixtureLimits()
	run := func(m DatasetManifest) error {
		_, err := Build(ctx, BuildRequest{Manifest: m, Limits: limits, TempDir: t.TempDir()})
		return err
	}

	bad := manifest
	bad.SchemaVersion = "coimnet-dataset-manifest/v9"
	if err := run(bad); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("schema version: %v", err)
	}
	bad = manifest
	bad.Files = manifest.Files[:2]
	if err := run(bad); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("missing role: %v", err)
	}
	bad = manifest
	bad.Files = append([]SourceFile(nil), manifest.Files...)
	bad.Files = append(bad.Files, manifest.Files[0])
	if err := run(bad); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("duplicate role: %v", err)
	}
	bad = manifest
	bad.FieldMapping.Annotations.ID = ""
	if err := run(bad); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("missing id mapping: %v", err)
	}
	bad = manifest
	bad.FieldMapping.Annotations.Status = "nope"
	if err := run(bad); err == nil || errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("unknown column should be a schema error, got %v", err)
	}
	bad = manifest
	bad.Selection.Field = ""
	if err := run(bad); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("empty predicate: %v", err)
	}
	bad = manifest
	bad.Selection.Source = RoleWeights
	if err := run(bad); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("predicate on weights: %v", err)
	}
	bad = manifest
	bad.Selection.Field = "somaLocation"
	if err := run(bad); err == nil || errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("list predicate column should fail as a schema error, got %v", err)
	}
	bad = manifest
	bad.Identity.WeightsEndpointsAreAnnotationIDs = false
	if err := run(bad); !errors.Is(err, ErrIdentityMapping) {
		t.Fatalf("identity mapping: %v", err)
	}
	bad = manifest
	bad.Identity.Evidence = ""
	if err := run(bad); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("identity without evidence: %v", err)
	}
	bad = manifest
	bad.Files = append([]SourceFile(nil), manifest.Files...)
	bad.Files[0].SHA256 = strings.Repeat("0", 64)
	if err := run(bad); !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("sha mismatch: %v", err)
	}
	bad = manifest
	bad.Files = append([]SourceFile(nil), manifest.Files...)
	bad.Files[1].Bytes++
	if err := run(bad); !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("size mismatch: %v", err)
	}
	bad = manifest
	bad.DuplicateSemantics = "made-up"
	if err := run(bad); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("duplicate semantics: %v", err)
	}
	negativeSort := fixtureLimits()
	negativeSort.SortBufferBytes = -1
	for _, limits := range []ResourceLimits{{}, {MaxMemoryBytes: -1, MaxTempBytes: 1, MaxRunFiles: 1, MaxArrowBytes: 1, MaxFooterBytes: 1, MaxRows: 1}, negativeSort} {
		if _, err := Build(ctx, BuildRequest{Manifest: manifest, Limits: limits, TempDir: t.TempDir()}); err == nil {
			t.Fatalf("accepted limits %#v", limits)
		}
	}
	if _, err := Build(ctx, BuildRequest{Manifest: manifest, Limits: limits, TempDir: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Fatal("accepted a missing temp directory")
	}
	if _, err := Build(ctx, BuildRequest{Manifest: manifest, Limits: limits, TempDir: t.TempDir(), EdgeView: "weird"}); err == nil {
		t.Fatal("accepted an unknown edge view")
	}
}

func TestBuildRejectsAggregationWithoutEvidenceAndAggregatesAdditivePartitions(t *testing.T) {
	manifest, _ := happyFixture(t, []int{11})
	_, err := Build(context.Background(), BuildRequest{Manifest: manifest, Limits: fixtureLimits(), TempDir: t.TempDir(), EdgeView: EdgeViewAggregatedPairs})
	if !errors.Is(err, ErrAggregationEvidence) {
		t.Fatalf("aggregation without evidence: %v", err)
	}
	manifest.DuplicateSemantics = DuplicateSemanticsTotalWithPartitions
	_, err = Build(context.Background(), BuildRequest{Manifest: manifest, Limits: fixtureLimits(), TempDir: t.TempDir(), EdgeView: EdgeViewAggregatedPairs})
	if !errors.Is(err, ErrAggregationEvidence) {
		t.Fatalf("total-with-partitions aggregation: %v", err)
	}

	// Additive partitions with a null weight inside a duplicated pair is rejected.
	manifest.DuplicateSemantics = DuplicateSemanticsAdditivePartitions
	_, err = Build(context.Background(), BuildRequest{Manifest: manifest, Limits: fixtureLimits(), TempDir: t.TempDir(), EdgeView: EdgeViewAggregatedPairs})
	if err == nil || errors.Is(err, ErrAggregationEvidence) {
		t.Fatalf("null weights must fail aggregation with a value error, got %v", err)
	}

	dir := t.TempDir()
	rows := []weightRow{{i64(10), i64(20), i64(3)}, {i64(10), i64(20), i64(4)}, {i64(20), i64(30), i64(1)}, {i64(30), i64(30), i64(2)}, {i64(30), i64(30), i64(5)}}
	weights := writeWeights(t, dir, rows, []int{3, 2})
	annotations := writeAnnotations(t, dir, happyAnnotations(), []int{10})
	nt := writeNeurotransmitters(t, dir, happyNeurotransmitters())
	additive := fixtureManifest(t, weights, annotations, nt)
	additive.DuplicateSemantics = DuplicateSemanticsAdditivePartitions
	result, err := Build(context.Background(), BuildRequest{Manifest: additive, Limits: fixtureLimits(), TempDir: t.TempDir(), EdgeView: EdgeViewAggregatedPairs})
	if err != nil {
		t.Fatal(err)
	}
	if result.Graph.EdgeCount() != 3 || result.Report.EdgeView != EdgeViewAggregatedPairs || result.Report.Annotated.Edges != 3 || result.Report.Annotated.DuplicatePairs != 2 || result.Report.Annotated.DuplicateRows != 2 || result.Report.Annotated.SelfLoops != 1 {
		t.Fatalf("aggregated report = %#v", result.Report.Annotated)
	}
	var edges []EdgeRecord
	_ = result.Graph.StreamAnnotatedEdges(context.Background(), func(e EdgeRecord) error { edges = append(edges, e); return nil })
	if edges[0].Weight.Value != 7 || edges[0].Aggregation != "sum_of_2_rows" || edges[0].Position.AbsoluteRow != 0 || edges[1].Weight.Value != 1 || edges[1].Aggregation != AggregationNone || edges[2].Weight.Value != 7 || edges[2].Aggregation != "sum_of_2_rows" {
		t.Fatalf("aggregated edges = %#v", edges)
	}
	if result.Report.Annotated.WeightSum != (ValueSum{Value: 15, Rows: 5, Valid: true, Basis: "weight"}) {
		t.Fatalf("aggregated weight sum = %#v", result.Report.Annotated.WeightSum)
	}
	// The rows view of the same manifest keeps every row.
	rowsView := build(t, additive, fixtureLimits())
	if rowsView.Graph.EdgeCount() != 5 {
		t.Fatalf("rows view edges = %d", rowsView.Graph.EdgeCount())
	}
}

func TestBuildCapacityCancellationAndCallbackErrors(t *testing.T) {
	manifest, _ := happyFixture(t, []int{11})
	tiny := fixtureLimits()
	tiny.MaxTempBytes = 16
	tiny.MaxMemoryBytes = 4096
	_, err := Build(context.Background(), BuildRequest{Manifest: manifest, Limits: tiny, TempDir: t.TempDir()})
	if !errors.Is(err, ErrCapacity) {
		t.Fatalf("temp capacity: %v", err)
	}
	tinyMemory := fixtureLimits()
	tinyMemory.MaxMemoryBytes = 64
	_, err = Build(context.Background(), BuildRequest{Manifest: manifest, Limits: tinyMemory, TempDir: t.TempDir()})
	if !errors.Is(err, ErrCapacity) {
		t.Fatalf("memory capacity: %v", err)
	}
	fewRows := fixtureLimits()
	fewRows.MaxRows = 5
	if _, err = Build(context.Background(), BuildRequest{Manifest: manifest, Limits: fewRows, TempDir: t.TempDir()}); err == nil {
		t.Fatal("row limit was ignored")
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = Build(canceled, BuildRequest{Manifest: manifest, Limits: fixtureLimits(), TempDir: t.TempDir()}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled build: %v", err)
	}

	temp := t.TempDir()
	result, err := Build(context.Background(), BuildRequest{Manifest: manifest, Limits: fixtureLimits(), TempDir: temp})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(temp)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary files left behind: %v", entries)
	}
	g := result.Graph
	boom := errors.New("boom")
	if err := g.StreamAnnotatedEdges(context.Background(), func(EdgeRecord) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("edge callback error: %v", err)
	}
	if err := g.StreamAnnotatedNodes(context.Background(), func(NodeRecord) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("node callback error: %v", err)
	}
	if err := g.StreamRawSegments(context.Background(), func(RawSegment) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("raw callback error: %v", err)
	}
	streamCtx, stop := context.WithCancel(context.Background())
	calls := 0
	err = g.StreamAnnotatedEdges(streamCtx, func(EdgeRecord) error {
		calls++
		stop()
		return nil
	})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("stream cancellation: err=%v calls=%d", err, calls)
	}
	stop()
	if err := g.StreamRawSegments(canceled, func(RawSegment) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("raw stream with canceled context: %v", err)
	}
}

func TestStreamRawSegmentsRejectsChangedSource(t *testing.T) {
	manifest, _ := happyFixture(t, []int{11})
	result := build(t, manifest, fixtureLimits())
	path := manifest.Files[0].Path
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(contents, 0), 0o600); err != nil {
		t.Fatal(err)
	}
	err = result.Graph.StreamRawSegments(context.Background(), func(RawSegment) error { return nil })
	if !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("changed source: %v", err)
	}
}

func TestBuildEmptyViewsAndZeroWeights(t *testing.T) {
	dir := t.TempDir()
	annotations := writeAnnotations(t, dir, []annotationRow{{id: i64(7), status: str("Orphan")}}, []int{1})
	weights := writeWeights(t, dir, nil, []int{0})
	nt := writeNeurotransmitters(t, dir, nil)
	manifest := fixtureManifest(t, weights, annotations, nt)
	result := build(t, manifest, fixtureLimits())
	if result.Graph.NodeCount() != 0 || result.Graph.EdgeCount() != 0 {
		t.Fatalf("empty view has nodes=%d edges=%d", result.Graph.NodeCount(), result.Graph.EdgeCount())
	}
	report := result.Report
	if report.Raw.Rows != 0 || report.Annotated.Nodes != 0 || report.Annotated.AnnotationExclusions.PredicateFalse != 1 {
		t.Fatalf("empty report = %#v", report)
	}
	if report.Annotated.Transmitter.UnknownRatio.Defined || report.Annotated.Transmitter.UnknownRatio.Denominator != 0 || report.Raw.Transmitter.UnknownRatio.Defined {
		t.Fatalf("empty ratios must be undefined: %#v", report.Annotated.Transmitter)
	}
	if len(report.Hashes.EdgeOrder) != 64 || len(report.Hashes.NodeIndex) != 64 {
		t.Fatalf("empty hashes = %#v", report.Hashes)
	}
	if err := result.Graph.StreamAnnotatedEdges(context.Background(), func(EdgeRecord) error { t.Fatal("unexpected edge"); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(result.Graph.NeuronIDs()) != 0 {
		t.Fatal("empty graph returned neuron IDs")
	}
}

func TestBuildRejectsWrongColumnTypesAndNonFiniteValues(t *testing.T) {
	manifest, dir := happyFixture(t, []int{11})
	// Weight column mapped to a string column has the wrong Arrow type.
	bad := manifest
	bad.FieldMapping.Weights.Value = "body_pre"
	bad.FieldMapping.Weights.Source = "weight"
	if _, err := Build(context.Background(), BuildRequest{Manifest: bad, Limits: fixtureLimits(), TempDir: t.TempDir()}); err != nil {
		t.Fatalf("swapping int64 columns is allowed: %v", err)
	}
	bad = manifest
	bad.FieldMapping.Neurotransmitters.Confidence = "consensus_nt"
	if _, err := Build(context.Background(), BuildRequest{Manifest: bad, Limits: fixtureLimits(), TempDir: t.TempDir()}); err == nil || errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("string column as float64 confidence must be a schema error: %v", err)
	}
	nan := writeNeurotransmitters(t, filepath.Join(dir), []ntRow{{id: i64(10), consensus: str("gaba"), confidence: f64(nanValue())}})
	nanManifest := manifest
	nanManifest.Files = append([]SourceFile(nil), manifest.Files...)
	nanManifest.Files[2] = sourceFile(t, RoleNeurotransmitters, nan)
	if _, err := Build(context.Background(), BuildRequest{Manifest: nanManifest, Limits: fixtureLimits(), TempDir: t.TempDir()}); err == nil {
		t.Fatal("NaN confidence was accepted")
	}
}

func TestBuildReportsUnmappedOptionalColumnsAsNotMapped(t *testing.T) {
	manifest, _ := happyFixture(t, []int{11})
	manifest.FieldMapping.Neurotransmitters.Consensus = ""
	manifest.FieldMapping.Annotations.ReceptorType = ""
	result := build(t, manifest, fixtureLimits())
	report := result.Report
	a := report.Annotated
	if a.Transmitter.Status != ColumnNotMapped || a.Transmitter.Predicted != 0 || a.Transmitter.Unknown != 0 || a.Transmitter.UnknownRatio.Defined || a.Transmitter.UnknownRatio.Denominator != 0 {
		t.Fatalf("unmapped consensus summary = %#v", a.Transmitter)
	}
	if a.Receptor.LabelStatus != ColumnNotMapped || a.Receptor.LabelNullRatio.Defined || a.Receptor.Status != EvidenceNotDerived {
		t.Fatalf("unmapped receptor summary = %#v", a.Receptor)
	}
	raw := report.Raw.Transmitter
	if raw.ConsensusStatus != ColumnNotMapped || raw.UnknownRows != 0 || raw.UnknownRatio.Defined || raw.Rows != 5 || raw.UniqueIDs != 4 {
		t.Fatalf("unmapped raw transmitter stats = %#v", raw)
	}
	for i := uint64(0); i < result.Graph.NodeCount(); i++ {
		node, err := result.Graph.Node(i)
		if err != nil {
			t.Fatal(err)
		}
		if node.Transmitter.Status != TransmitterNotMapped || node.Transmitter.Consensus.Valid || node.ReceptorType.Valid {
			t.Fatalf("node %d = %#v", i, node)
		}
	}
	// The predicted column is still mapped and its values are retained.
	n0, _ := result.Graph.Node(0)
	if !n0.Transmitter.Predicted.Valid || n0.Transmitter.Predicted.Value != "acetylcholine" {
		t.Fatalf("node 0 predicted = %#v", n0.Transmitter)
	}
}
