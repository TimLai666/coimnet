package simulate

import (
	"context"
	"fmt"

	"github.com/TimLai666/coimnet/connectome"
)

// SelectorResolution records what a selector actually matched so a report can
// be read without the graph. FirstIndex and LastIndex are -1 when the selector
// matched nothing under AllowEmpty.
type SelectorResolution struct {
	Field      string `json:"field"`
	Equals     string `json:"equals"`
	Count      int    `json:"count"`
	FirstIndex int    `json:"first_index"`
	LastIndex  int    `json:"last_index"`
}

// selectorFields maps the declared field names to the NodeRecord field they
// read. Only these six annotation columns are addressable; anything else is an
// error rather than a silent empty set.
var selectorFields = map[string]func(connectome.NodeRecord) connectome.NullString{
	"class":      func(n connectome.NodeRecord) connectome.NullString { return n.Class },
	"type":       func(n connectome.NodeRecord) connectome.NullString { return n.Type },
	"superclass": func(n connectome.NodeRecord) connectome.NullString { return n.Superclass },
	"subclass":   func(n connectome.NodeRecord) connectome.NullString { return n.Subclass },
	"instance":   func(n connectome.NodeRecord) connectome.NullString { return n.Instance },
	"soma_side":  func(n connectome.NodeRecord) connectome.NullString { return n.SomaSide },
}

// resolveSelector walks the annotated nodes in index order and returns the
// matching indices with their resolution record. A null annotation never
// matches, so "unknown" is not folded into any value.
func resolveSelector(ctx context.Context, g *connectome.Graph, s Selector) ([]int, SelectorResolution, error) {
	read, known := selectorFields[s.Field]
	if !known {
		return nil, SelectorResolution{}, fmt.Errorf("simulate: selector field %q is not one of class, type, superclass, subclass, instance, soma_side", s.Field)
	}
	if s.Equals == "" {
		return nil, SelectorResolution{}, fmt.Errorf("simulate: selector on %q has an empty value", s.Field)
	}
	var nodes []int
	err := g.StreamAnnotatedNodes(ctx, func(record connectome.NodeRecord) error {
		value := read(record)
		if value.Valid && value.Value == s.Equals {
			nodes = append(nodes, int(record.Index))
		}
		return nil
	})
	if err != nil {
		return nil, SelectorResolution{}, err
	}
	resolution := SelectorResolution{Field: s.Field, Equals: s.Equals, Count: len(nodes), FirstIndex: -1, LastIndex: -1}
	if len(nodes) == 0 {
		if !s.AllowEmpty {
			return nil, SelectorResolution{}, fmt.Errorf("simulate: selector %s == %q matched no node; set allow_empty to accept that", s.Field, s.Equals)
		}
		return nil, resolution, nil
	}
	resolution.FirstIndex, resolution.LastIndex = nodes[0], nodes[len(nodes)-1]
	return nodes, resolution, nil
}

// checkNodes validates explicit indices: in range, ascending and distinct, so
// a probe cannot count one neuron twice.
func checkNodes(nodes []int, count int, what string) ([]int, error) {
	previous := -1
	for i, node := range nodes {
		if node < 0 || node >= count {
			return nil, fmt.Errorf("simulate: %s node %d is outside [0,%d)", what, node, count)
		}
		if node <= previous {
			return nil, fmt.Errorf("simulate: %s nodes must be ascending and distinct; index %d breaks the order", what, i)
		}
		previous = node
	}
	return append([]int(nil), nodes...), nil
}
