package plasticity_test

import (
	"testing"

	"github.com/TimLai666/coimnet/plasticity"
)

func TestEdgeMetadataUsesEnabledPosition(t *testing.T) {
	m, err := plasticity.New(plasticity.Config{Edges: []int{1, 3}, Rule: plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 1, WMin: .0625}}, 4)
	if err != nil {
		t.Fatal(err)
	}
	for position, want := range []int{1, 3} {
		edge, minimum, err := m.Edge(position)
		if err != nil || edge != want || minimum != .0625 {
			t.Fatalf("Edge(%d) = %d, %g, %v; want %d, .0625, nil", position, edge, minimum, err, want)
		}
	}
	for _, position := range []int{-1, 2, 4} {
		if _, _, err := m.Edge(position); err == nil {
			t.Fatalf("Edge(%d) accepted an invalid enabled position", position)
		}
	}
	var absent *plasticity.Model
	if _, _, err := absent.Edge(0); err == nil {
		t.Fatal("nil model accepted an edge lookup")
	}
}
