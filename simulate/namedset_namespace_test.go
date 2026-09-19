package simulate

import (
	"context"
	"strings"
	"testing"
)

// TestNamedSetNamespaceMismatchIsRefused is the compare half of the no-splicing
// rule: a named set may declare which dataset it was written for, and a set
// written for another dataset is refused instead of silently resolving against
// whatever graph is loaded.
func TestNamedSetNamespaceMismatchIsRefused(t *testing.T) {
	g := fixtureGraph(t)
	const foreign = "flywire-v783"
	selectors := []Selector{{Field: "class", Equals: "ALPN"}}

	_, err := ResolveSets(context.Background(), g, []NamedSet{{Name: "alpn", Namespace: foreign, Selectors: selectors}})
	if err == nil {
		t.Fatal("a named set of another dataset was accepted")
	}
	if !strings.Contains(err.Error(), "declares namespace") {
		t.Fatalf("error = %q, which does not say the set declares a namespace", err)
	}
	if !strings.Contains(err.Error(), foreign) || !strings.Contains(err.Error(), g.Namespace()) {
		t.Fatalf("error = %q, want both the declared and the graph namespace", err)
	}

	matching, err := ResolveSets(context.Background(), g, []NamedSet{{Name: "alpn", Namespace: g.Namespace(), Selectors: selectors}})
	if err != nil {
		t.Fatalf("a set declaring the graph namespace: %v", err)
	}
	undeclared, err := ResolveSets(context.Background(), g, []NamedSet{{Name: "alpn", Selectors: selectors}})
	if err != nil {
		t.Fatalf("a set declaring no namespace: %v", err)
	}
	if matching[0].Count != 2 || matching[0].NodeHash != undeclared[0].NodeHash {
		t.Fatalf("declaring the graph namespace changed the resolution: %+v against %+v", matching[0], undeclared[0])
	}
}

// TestNamedSetWithoutNamespaceKeepsTheProtocolEncoding is the compatibility
// half: the new field is omitted from a set that does not declare it, so a
// compare protocol written before the field still hashes to the same value.
func TestNamedSetWithoutNamespaceKeepsTheProtocolEncoding(t *testing.T) {
	cp := compareFixtureProtocol(6)
	for _, set := range cp.Sets {
		if set.Namespace != "" {
			t.Fatalf("the fixture protocol declares a namespace on set %q", set.Name)
		}
	}
	encoded := mustJSON(t, cp)
	if strings.Contains(encoded, "namespace") {
		t.Fatalf("the encoded protocol mentions namespace: %s", encoded)
	}
	before, err := cp.Hash()
	if err != nil {
		t.Fatal(err)
	}

	declared := cp
	declared.Sets = append([]NamedSet(nil), cp.Sets...)
	declared.Sets[0].Namespace = "simulate-fixture-v1"
	after, err := declared.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if after == before {
		t.Fatal("declaring a namespace left the compare protocol hash unchanged")
	}
	if !strings.Contains(mustJSON(t, declared), `"namespace"`) {
		t.Fatal("the encoded protocol does not carry the declared namespace")
	}
}
