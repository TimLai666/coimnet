package connectome

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// mappingGraph builds a small graph under the given namespace from annotation
// IDs and weight endpoint pairs, reusing the fixture writers of this package.
func mappingGraph(t *testing.T, namespace string, ids []int64, pairs [][2]int64) *Graph {
	t.Helper()
	nodes := make([]annotationRow, 0, len(ids))
	for _, id := range ids {
		nodes = append(nodes, annotationRow{id: i64(id), status: str("Traced"), statusLabel: str("Reviewed")})
	}
	edges := make([]weightRow, 0, len(pairs))
	for _, pair := range pairs {
		edges = append(edges, weightRow{i64(pair[0]), i64(pair[1]), i64(1)})
	}
	dir := t.TempDir()
	annotations := writeAnnotations(t, dir, nodes, []int{len(nodes)})
	weights := writeWeights(t, dir, edges, []int{len(edges)})
	transmitters := writeNeurotransmitters(t, dir, []ntRow{{id: i64(ids[0]), consensus: str("acetylcholine")}})
	manifest := fixtureManifest(t, weights, annotations, transmitters)
	manifest.Namespace = namespace
	return build(t, manifest, fixtureLimits()).Graph
}

// otherNamespace is the second dataset of the mapping tests. It never shares a
// neuron ID space with testNamespace(), which is the whole point of the ticket.
func otherNamespace() string { return "other-fixture-v1" }

// completeEvidence is a MappingEvidence with every required field filled in.
func completeEvidence() *MappingEvidence {
	return &MappingEvidence{
		Source:   "a published cross-dataset mapping",
		Method:   "coordinate registration plus manual review",
		Coverage: "the 2 neurons both datasets declare",
		Version:  "v1",
	}
}

func TestMappingEvidenceValidateNamesTheMissingField(t *testing.T) {
	if err := completeEvidence().Validate(); err != nil {
		t.Fatalf("complete evidence: %v", err)
	}
	for field, blank := range map[string]func(*MappingEvidence){
		"source":   func(e *MappingEvidence) { e.Source = "" },
		"method":   func(e *MappingEvidence) { e.Method = "" },
		"coverage": func(e *MappingEvidence) { e.Coverage = "" },
		"version":  func(e *MappingEvidence) { e.Version = "" },
	} {
		evidence := completeEvidence()
		blank(evidence)
		err := evidence.Validate()
		if err == nil {
			t.Fatalf("evidence without %s was accepted", field)
		}
		if !strings.Contains(err.Error(), field) {
			t.Fatalf("evidence without %s reported %q, which does not name the field", field, err)
		}
	}
}

func TestMergeRequiresEvidenceAcrossNamespaces(t *testing.T) {
	a := mappingGraph(t, testNamespace(), []int64{10, 20, 30}, [][2]int64{{10, 20}, {20, 30}})
	b := mappingGraph(t, otherNamespace(), []int64{20, 30, 40, 50}, [][2]int64{{20, 30}, {30, 40}, {40, 50}})

	incomplete := completeEvidence()
	incomplete.Coverage = ""
	for name, evidence := range map[string]*MappingEvidence{
		"no evidence":         nil,
		"incomplete evidence": incomplete,
	} {
		graph, err := Merge(context.Background(), a, b, evidence)
		if graph != nil {
			t.Fatalf("%s: Merge returned a graph", name)
		}
		if !errors.Is(err, ErrMappingEvidenceRequired) {
			t.Fatalf("%s: Merge error = %v, want ErrMappingEvidenceRequired", name, err)
		}
		if !strings.Contains(err.Error(), testNamespace()) || !strings.Contains(err.Error(), otherNamespace()) {
			t.Fatalf("%s: Merge error %q does not name both datasets", name, err)
		}
	}
	if err := incomplete.Validate(); err == nil {
		t.Fatal("the incomplete evidence of this test validates")
	} else if graph, mergeErr := Merge(context.Background(), a, b, incomplete); graph != nil || !strings.Contains(mergeErr.Error(), "coverage") {
		t.Fatalf("Merge error %q does not carry the validation reason", mergeErr)
	}
}

func TestMergeRefusesEvenWithCompleteEvidence(t *testing.T) {
	a := mappingGraph(t, testNamespace(), []int64{10, 20, 30}, [][2]int64{{10, 20}, {20, 30}})
	b := mappingGraph(t, otherNamespace(), []int64{20, 30, 40, 50}, [][2]int64{{20, 30}, {30, 40}, {40, 50}})

	graph, err := Merge(context.Background(), a, b, completeEvidence())
	if graph != nil {
		t.Fatal("Merge returned a graph")
	}
	if !errors.Is(err, ErrNoFormalMapping) {
		t.Fatalf("Merge error = %v, want ErrNoFormalMapping", err)
	}
	if errors.Is(err, ErrMappingEvidenceRequired) {
		t.Fatalf("Merge error %q still asks for evidence that was supplied", err)
	}
	if !strings.Contains(err.Error(), testNamespace()) || !strings.Contains(err.Error(), otherNamespace()) {
		t.Fatalf("Merge error %q does not name both datasets", err)
	}
}

func TestMergeWithinNamespaceIsUndefined(t *testing.T) {
	a := mappingGraph(t, testNamespace(), []int64{10, 20, 30}, [][2]int64{{10, 20}, {20, 30}})
	b := mappingGraph(t, testNamespace(), []int64{20, 30, 40, 50}, [][2]int64{{20, 30}, {30, 40}, {40, 50}})

	for name, evidence := range map[string]*MappingEvidence{"no evidence": nil, "complete evidence": completeEvidence()} {
		graph, err := Merge(context.Background(), a, b, evidence)
		if graph != nil {
			t.Fatalf("%s: Merge returned a graph", name)
		}
		if err == nil {
			t.Fatalf("%s: Merge within one namespace was accepted", name)
		}
		if !strings.Contains(err.Error(), "merge within one namespace is not defined; rebuild from one manifest") {
			t.Fatalf("%s: Merge error = %q", name, err)
		}
		if errors.Is(err, ErrMappingEvidenceRequired) || errors.Is(err, ErrNoFormalMapping) {
			t.Fatalf("%s: Merge reported a cross-dataset error for one namespace: %q", name, err)
		}
	}
}

func TestCompareWithinNamespaceCountsSharedIDs(t *testing.T) {
	a := mappingGraph(t, testNamespace(), []int64{10, 20, 30}, [][2]int64{{10, 20}, {20, 30}})
	b := mappingGraph(t, testNamespace(), []int64{20, 30, 40, 50}, [][2]int64{{20, 30}, {30, 40}, {40, 50}})

	want := GraphComparison{
		NamespaceA: testNamespace(), NamespaceB: testNamespace(),
		NodesA: 3, NodesB: 4, EdgesA: 2, EdgesB: 3,
		SharedIDs: 2, // bodies 20 and 30
	}
	got, err := Compare(context.Background(), a, b, nil)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if got != want {
		t.Fatalf("Compare = %+v, want %+v", got, want)
	}
	// The counts come from the graphs, not from the evidence, so supplying
	// evidence inside one namespace changes nothing.
	withEvidence, err := Compare(context.Background(), a, b, completeEvidence())
	if err != nil || withEvidence != want {
		t.Fatalf("Compare with evidence = %+v, %v", withEvidence, err)
	}
	// Comparing a graph with itself shares every ID.
	self, err := Compare(context.Background(), a, a, nil)
	if err != nil {
		t.Fatalf("Compare a with a: %v", err)
	}
	if self.SharedIDs != 3 || self.NodesA != 3 || self.NodesB != 3 {
		t.Fatalf("Compare a with a = %+v", self)
	}
}

func TestCompareAcrossNamespacesRequiresEvidence(t *testing.T) {
	a := mappingGraph(t, testNamespace(), []int64{10, 20, 30}, [][2]int64{{10, 20}, {20, 30}})
	b := mappingGraph(t, otherNamespace(), []int64{20, 30, 40, 50}, [][2]int64{{20, 30}, {30, 40}, {40, 50}})

	incomplete := completeEvidence()
	incomplete.Version = ""
	for name, evidence := range map[string]*MappingEvidence{"no evidence": nil, "incomplete evidence": incomplete} {
		got, err := Compare(context.Background(), a, b, evidence)
		if !errors.Is(err, ErrMappingEvidenceRequired) {
			t.Fatalf("%s: Compare error = %v, want ErrMappingEvidenceRequired", name, err)
		}
		if got != (GraphComparison{}) {
			t.Fatalf("%s: Compare reported counts anyway: %+v", name, got)
		}
		if !strings.Contains(err.Error(), testNamespace()) || !strings.Contains(err.Error(), otherNamespace()) {
			t.Fatalf("%s: Compare error %q does not name both datasets", name, err)
		}
	}

	got, err := Compare(context.Background(), a, b, completeEvidence())
	if !errors.Is(err, ErrNoFormalMapping) {
		t.Fatalf("Compare error = %v, want ErrNoFormalMapping", err)
	}
	if got != (GraphComparison{}) {
		t.Fatalf("Compare reported counts anyway: %+v", got)
	}
}

func TestMergeAndCompareRejectNilArgumentsAndCancellation(t *testing.T) {
	a := mappingGraph(t, testNamespace(), []int64{10, 20, 30}, [][2]int64{{10, 20}, {20, 30}})
	b := mappingGraph(t, otherNamespace(), []int64{20, 30, 40, 50}, [][2]int64{{20, 30}, {30, 40}, {40, 50}})

	//nolint:staticcheck // a nil context is exactly what this test passes.
	if _, err := Merge(nil, a, b, completeEvidence()); err == nil {
		t.Fatal("Merge accepted a nil context")
	}
	//nolint:staticcheck // a nil context is exactly what this test passes.
	if _, err := Compare(nil, a, b, nil); err == nil {
		t.Fatal("Compare accepted a nil context")
	}
	for name, pair := range map[string][2]*Graph{"nil a": {nil, b}, "nil b": {a, nil}, "both nil": {nil, nil}} {
		if _, err := Merge(context.Background(), pair[0], pair[1], completeEvidence()); err == nil {
			t.Fatalf("Merge accepted %s", name)
		}
		if _, err := Compare(context.Background(), pair[0], pair[1], nil); err == nil {
			t.Fatalf("Compare accepted %s", name)
		}
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Merge(cancelled, a, b, completeEvidence()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Merge on a cancelled context = %v", err)
	}
	if _, err := Compare(cancelled, a, a, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Compare on a cancelled context = %v", err)
	}
}
