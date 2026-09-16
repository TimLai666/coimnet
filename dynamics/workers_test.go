package dynamics

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
)

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTargetPartitionKeepsDeclaredEdgeOrder(t *testing.T) {
	p, err := newTargetPartition(3, 1, []int{2, 0, 1, 0}, []int{1, 1, 0, 1})
	if err != nil {
		t.Fatal(err)
	}
	for target, want := range map[int][]int{0: {2}, 1: {0, 1, 3}, 2: nil} {
		if got := p.edges[target]; !equalInts(got, want) {
			t.Fatalf("edges[%d] = %v, want %v", target, got, want)
		}
	}
}

func TestTargetPartitionOwnersByModulo(t *testing.T) {
	p, err := newTargetPartition(5, 2, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Workers() != 2 || !equalInts(p.owners[0], []int{0, 2, 4}) || !equalInts(p.owners[1], []int{1, 3}) {
		t.Fatalf("workers %d owners %v", p.workers, p.owners)
	}
	for _, w := range []int{0, 1} {
		p, err := newTargetPartition(5, w, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := p.Workers(); got != 1 {
			t.Fatalf("workers %d resolves to %d, want 1", w, got)
		}
		if len(p.owners) != 1 || !equalInts(p.owners[0], []int{0, 1, 2, 3, 4}) {
			t.Fatalf("workers %d owners = %v", w, p.owners)
		}
	}
}

func TestTargetPartitionRunVisitsEveryTargetOnce(t *testing.T) {
	t.Run("parallel", func(t *testing.T) {
		p, err := newTargetPartition(7, 3, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		var mu sync.Mutex
		seen := make(map[int]int)
		if err := p.Run(func(target int, edges []int) error {
			mu.Lock()
			seen[target]++
			mu.Unlock()
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if len(seen) != 7 {
			t.Fatalf("visited %d targets, want 7", len(seen))
		}
		for target := 0; target < 7; target++ {
			if seen[target] != 1 {
				t.Fatalf("target %d visited %d times, want 1", target, seen[target])
			}
		}
	})
	t.Run("single threaded ascending order", func(t *testing.T) {
		p, err := newTargetPartition(7, 1, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		var order []int
		if err := p.Run(func(target int, edges []int) error {
			order = append(order, target)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if !equalInts(order, []int{0, 1, 2, 3, 4, 5, 6}) {
			t.Fatalf("call order = %v, want 0..6", order)
		}
	})
}

func TestTargetPartitionRunReturnsFirstError(t *testing.T) {
	p, err := newTargetPartition(7, 3, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom 3")
	if err := p.Run(func(target int, edges []int) error {
		if target == 3 {
			return boom
		}
		return nil
	}); err != boom {
		t.Fatalf("Run err = %v, want %v", err, boom)
	}
}

func TestTargetPartitionRejectsBadInput(t *testing.T) {
	cases := []struct {
		name             string
		nodes, workers   int
		sources, targets []int
	}{
		{"negative workers", 3, -1, nil, nil},
		{"too many workers", 3, 2000, nil, nil},
		{"zero nodes", 0, 1, nil, nil},
		{"length mismatch", 3, 1, []int{0}, []int{0, 1}},
		{"target out of range", 3, 1, []int{0}, []int{3}},
		{"source out of range", 3, 1, []int{3}, []int{0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := newTargetPartition(c.nodes, c.workers, c.sources, c.targets); err == nil {
				t.Fatalf("accepted %s", c.name)
			}
		})
	}
}

func TestConfigWorkersFieldValidation(t *testing.T) {
	base := Config{Nodes: 1, DT: 1, Activation: "tanh"}
	if _, err := NewContinuous(base); err != nil {
		t.Fatal(err)
	}
	neg := base
	neg.Workers = -1
	if _, err := NewContinuous(neg); err == nil || !strings.Contains(err.Error(), "workers") {
		t.Fatalf("negative workers error = %v", err)
	}
	max := base
	max.Workers = 1024
	if _, err := NewContinuous(max); err != nil {
		t.Fatalf("workers 1024: %v", err)
	}
	over := base
	over.Workers = 1025
	if _, err := NewContinuous(over); err == nil {
		t.Fatal("accepted workers 1025")
	}
	data, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "workers") {
		t.Fatalf("zero workers leaked into JSON: %s", data)
	}
}
