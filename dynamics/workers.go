package dynamics

import (
	"fmt"
	"sync"
)

// targetPartition groups the edge indices of a topology by target node so a
// worker pool can add each target's inputs in declared edge order without
// two workers writing the same node. Partition k owns the targets whose
// index i satisfies i % workers == k. Edges keeps, for each target, the
// edge indices with that target in increasing order.
type targetPartition struct {
	workers int
	edges   [][]int // indexed by target node
	owners  [][]int // owners[k] lists the target nodes worker k owns, ascending
}

// newTargetPartition validates sources/targets against nodes (each in [0, nodes)),
// resolves workers (0 or 1 -> 1) and builds the partition. It rejects
// workers < 0, workers > 1024, nodes <= 0 and mismatched source/target lengths.
func newTargetPartition(nodes, workers int, sources, targets []int) (*targetPartition, error) {
	if nodes <= 0 {
		return nil, fmt.Errorf("nodes must be positive")
	}
	if workers < 0 {
		return nil, fmt.Errorf("workers must not be negative")
	}
	if workers > 1024 {
		return nil, fmt.Errorf("workers exceed 1024")
	}
	if workers == 0 || workers == 1 {
		workers = 1
	}
	if len(sources) != len(targets) {
		return nil, fmt.Errorf("source and target arrays have different lengths")
	}
	edges := make([][]int, nodes)
	for e, s := range sources {
		t := targets[e]
		if s < 0 || s >= nodes || t < 0 || t >= nodes {
			return nil, fmt.Errorf("edge %d has endpoint out of range", e)
		}
		edges[t] = append(edges[t], e)
	}
	owners := make([][]int, workers)
	for i := 0; i < nodes; i++ {
		owners[i%workers] = append(owners[i%workers], i)
	}
	return &targetPartition{workers: workers, edges: edges, owners: owners}, nil
}

// Workers returns the resolved worker count (at least 1).
func (p *targetPartition) Workers() int { return p.workers }

// Run calls fn(target, edges) for every target node, on the worker that owns
// it, and waits for all of them. With one worker it calls fn inline in
// ascending target order and starts no goroutine. Any error returned by fn
// stops the run and is returned (the first one in worker order).
func (p *targetPartition) Run(fn func(target int, edges []int) error) error {
	if p.workers == 1 {
		for _, target := range p.owners[0] {
			if err := fn(target, p.edges[target]); err != nil {
				return err
			}
		}
		return nil
	}
	errs := make([]error, p.workers)
	var wg sync.WaitGroup
	wg.Add(p.workers)
	for k, owned := range p.owners {
		go func(k int, owned []int) {
			defer wg.Done()
			for _, target := range owned {
				if err := fn(target, p.edges[target]); err != nil {
					errs[k] = err
					return
				}
			}
		}(k, owned)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
