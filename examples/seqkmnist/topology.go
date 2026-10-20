package main

import (
	"context"
	"fmt"
	"math/rand/v2"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/learning"
)

// topology is a fixed directed edge list over nodes 0..nodes-1, edge e
// running from sources[e] to targets[e].
type topology struct {
	nodes            int
	sources, targets []int
	// types holds each node's annotated cell type, "" when unknown; it is
	// empty for a random topology.
	types []string
}

// storeLimits bounds the subgraph stores this example reads; a store for a
// few thousand neurons is well under a megabyte.
var storeLimits = connectome.StoreLimits{
	MaxFileBytes:   64 << 20,
	MaxFooterBytes: 1 << 20,
	MaxMemoryBytes: 128 << 20,
}

// loadStore reads the annotated (both endpoints selected) edges of a
// connectome.Save store, in its canonical edge order. Connectome weights are
// not used: the core weights start from the same seeded draw as the random
// topology, so only the wiring differs.
func loadStore(ctx context.Context, path string) (topology, error) {
	g, err := connectome.Load(ctx, path, storeLimits)
	if err != nil {
		return topology{}, err
	}
	t := topology{nodes: int(g.NodeCount())}
	t.types = make([]string, t.nodes)
	for i := range t.types {
		n, err := g.Node(uint64(i))
		if err != nil {
			return topology{}, err
		}
		if n.Type.Valid {
			t.types[i] = n.Type.Value
		}
	}
	seen := make(map[[2]int]bool)
	err = g.StreamAnnotatedEdges(ctx, func(e connectome.EdgeRecord) error {
		s, d := int(e.Source), int(e.Target)
		if s < 0 || d < 0 || s >= t.nodes || d >= t.nodes {
			return fmt.Errorf("edge (%d,%d) outside %d nodes", s, d, t.nodes)
		}
		if seen[[2]int{s, d}] {
			return fmt.Errorf("duplicate edge (%d,%d)", s, d)
		}
		seen[[2]int{s, d}] = true
		t.sources, t.targets = append(t.sources, s), append(t.targets, d)
		return nil
	})
	if err != nil {
		return topology{}, err
	}
	if len(t.sources) == 0 {
		return topology{}, fmt.Errorf("store %s has no edges between selected nodes", path)
	}
	return t, nil
}

// rewire returns a null-model copy of t. "in-degree" keeps every target's
// in-degree and draws its sources uniformly without repetition, as the random
// topology does. "swap" keeps every node's in- and out-degree: it performs
// 10*edges attempted double-edge swaps (a->b, c->d to a->d, c->b), rejecting
// any swap that would duplicate an edge. An empty mode returns t unchanged.
func rewire(t topology, mode string, seed uint64) (topology, error) {
	rng := rand.New(rand.NewPCG(seed, 2))
	out := topology{nodes: t.nodes, sources: append([]int(nil), t.sources...), targets: append([]int(nil), t.targets...), types: t.types}
	switch mode {
	case "":
		return out, nil
	case "in-degree":
		degree := make([]int, t.nodes)
		for _, d := range t.targets {
			degree[d]++
		}
		out.sources, out.targets = out.sources[:0], out.targets[:0]
		for d, k := range degree {
			for _, s := range rng.Perm(t.nodes)[:k] {
				out.sources, out.targets = append(out.sources, s), append(out.targets, d)
			}
		}
		return out, nil
	case "swap":
		edges := make(map[[2]int]bool, len(t.sources))
		for e := range out.sources {
			edges[[2]int{out.sources[e], out.targets[e]}] = true
		}
		for range 10 * len(out.sources) {
			i, j := rng.IntN(len(out.sources)), rng.IntN(len(out.sources))
			a, b, c, d := out.sources[i], out.targets[i], out.sources[j], out.targets[j]
			if i == j || edges[[2]int{a, d}] || edges[[2]int{c, b}] {
				continue
			}
			delete(edges, [2]int{a, b})
			delete(edges, [2]int{c, d})
			edges[[2]int{a, d}], edges[[2]int{c, b}] = true, true
			out.targets[i], out.targets[j] = d, b
		}
		return out, nil
	}
	return topology{}, fmt.Errorf(`--rewire takes "in-degree" or "swap", not %q`, mode)
}

// tieByType ties the core parameters by annotated cell type, as a
// connectome-constrained model shares one value per type: every edge between
// the same (source type, target type) pair shares a weight, and every node of
// a type shares its bias and time constant. A node of unknown type ("") and
// its edges stay individual. Groups are numbered in order of first
// appearance, weights first, and each starts at its first member's initial
// value, as learning requires. It returns the declaration and the number of
// distinct trained values it removes.
func tieByType(types []string, sources, targets []int, p *learning.Parameters) (*learning.ParameterSharing, int) {
	share := &learning.ParameterSharing{Weights: make([]int32, len(sources)), Bias: make([]int32, len(types)), LogTau: make([]int32, len(types))}
	members := 0
	assign := func(indices []int32, values []float64, key func(int) (string, bool)) {
		type group struct {
			id    int32
			first int
		}
		groups := map[string]group{}
		for i := range indices {
			k, ok := key(i)
			if !ok {
				indices[i] = -1
				continue
			}
			g, seen := groups[k]
			if !seen {
				g = group{id: int32(share.Groups), first: i}
				groups[k] = g
				share.Groups++
			}
			indices[i] = g.id
			members++
			values[i] = values[g.first]
		}
	}
	assign(share.Weights, p.Core.Weights, func(e int) (string, bool) {
		a, b := types[sources[e]], types[targets[e]]
		return a + "\x00" + b, a != "" && b != ""
	})
	node := func(i int) (string, bool) { return types[i], types[i] != "" }
	assign(share.Bias, p.Core.Bias, node)
	assign(share.LogTau, p.Core.LogTau, node)
	return share, members - share.Groups
}
