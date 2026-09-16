package simulate

import (
	"context"
	"reflect"
	"testing"
)

// A resolved set is the only honest way to name neurons: its node indices stay
// private so a caller cannot forge one. Nodes is the read accessor a consumer
// outside this package (a modulation source averaging over the set) needs, and
// it must hand out an owned copy so the resolution itself cannot be edited.
func TestResolvedSetNodesReturnsAnAscendingOwnedCopy(t *testing.T) {
	sets := resolveAll(t, metricSets())
	want := map[string][]int{"alpn": {1, 2}, "left_alpn": {2}, "alin": {0}}
	for _, set := range sets {
		nodes := set.Nodes()
		if !reflect.DeepEqual(nodes, want[set.Name]) {
			t.Fatalf("set %q Nodes() = %v, want %v", set.Name, nodes, want[set.Name])
		}
		for i := 1; i < len(nodes); i++ {
			if nodes[i] <= nodes[i-1] {
				t.Fatalf("set %q Nodes() = %v is not ascending", set.Name, nodes)
			}
		}
		if len(nodes) != set.Count {
			t.Fatalf("set %q Nodes() returned %d nodes but Count is %d", set.Name, len(nodes), set.Count)
		}
		hash := set.NodeHash
		for i := range nodes {
			nodes[i] = -1
		}
		again := set.Nodes()
		if !reflect.DeepEqual(again, want[set.Name]) {
			t.Fatalf("set %q Nodes() = %v after the first result was overwritten, want %v", set.Name, again, want[set.Name])
		}
		if set.NodeHash != hash || set.Count != len(want[set.Name]) {
			t.Fatalf("set %q changed to count %d hash %s", set.Name, set.Count, set.NodeHash)
		}
	}
}

func TestResolvedSetNodesReturnsAnEmptySliceForADeclaredEmptySet(t *testing.T) {
	g := fixtureGraph(t)
	resolved, err := ResolveSets(context.Background(), g, []NamedSet{{Name: "none", Selectors: []Selector{
		{Field: "class", Equals: "ALIN", AllowEmpty: true},
		{Field: "soma_side", Equals: "R", AllowEmpty: true},
	}}})
	if err != nil {
		t.Fatalf("resolve declared empty set: %v", err)
	}
	if nodes := resolved[0].Nodes(); len(nodes) != 0 {
		t.Fatalf("declared empty set Nodes() = %v, want no node", nodes)
	}
	if nodes := (ResolvedSet{}).Nodes(); len(nodes) != 0 {
		t.Fatalf("zero ResolvedSet Nodes() = %v, want no node", nodes)
	}
}
