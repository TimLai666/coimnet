package kmnist

import "testing"

func TestSelectionsMatchPythonWriter(t *testing.T) {
	s := Split{Labels: []int{3, 0, 0, 1, 2, 3, 1, 4, 5, 6, 7, 8, 9, 2}}
	got, err := s.Balanced(12)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1, 3, 4, 0, 7, 8, 9, 10, 11, 12, 2, 6}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Balanced = %v, want %v", got, want)
		}
	}
	// Written by scripts/bench/kmnist_mlp.py split_id for the same selection.
	const pythonSplit = "balanced_round_robin_by_label_v1:12:6f90361676a1b8788bc3304a65fc4ff6ca1dd2c38f14ac8148d219b0c8618c19"
	if id := SplitID("balanced_round_robin_by_label_v1", got); id != pythonSplit {
		t.Fatalf("SplitID = %q, Python writes %q", id, pythonSplit)
	}
	if _, err := s.Balanced(15); err == nil {
		t.Fatal("asking for more images than exist must fail")
	}
	rr, err := s.RoundRobin([]int{0, 1, 2, 3}, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rr) != 4 || rr[0] != 2 || rr[1] != 6 || rr[2] != 13 || rr[3] != 5 {
		t.Fatalf("RoundRobin = %v, want [2 6 13 5]", rr)
	}
	if _, err := s.RoundRobin([]int{4}, 1, 1); err == nil {
		t.Fatal("a class without that many images must fail")
	}
	all, err := s.RoundRobin([]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	for i := range all {
		if all[i] != got[i] {
			t.Fatalf("RoundRobin from zero = %v, Balanced = %v", all, got[:10])
		}
	}
}
