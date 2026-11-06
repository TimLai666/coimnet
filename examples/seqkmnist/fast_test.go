package main

import (
	"math/rand/v2"
	"testing"
)

func TestBlock8Wiring(t *testing.T) {
	const nodes = 64
	for _, degree := range []int{8, 13, 36} {
		sources, targets := block8Wiring(rand.New(rand.NewPCG(1, 2)), nodes, degree)
		if len(sources) != nodes*degree || len(targets) != len(sources) {
			t.Fatalf("degree %d: %d edges, want %d", degree, len(sources), nodes*degree)
		}
		for target := range nodes {
			edges := sources[target*degree : (target+1)*degree]
			seen := map[int]bool{}
			for j, src := range edges {
				if targets[target*degree+j] != target {
					t.Fatalf("degree %d: edge %d of node %d targets %d", degree, j, target, targets[target*degree+j])
				}
				if seen[src] {
					t.Fatalf("degree %d: node %d repeats source %d", degree, target, src)
				}
				seen[src] = true
				// Within a window the sources are consecutive from an aligned start.
				if j%8 == 0 && src%8 != 0 {
					t.Fatalf("degree %d: node %d window %d starts at %d", degree, target, j/8, src)
				}
				if j%8 != 0 && src != edges[j-1]+1 {
					t.Fatalf("degree %d: node %d edge %d source %d does not follow %d", degree, target, j, src, edges[j-1])
				}
			}
		}
	}
}
