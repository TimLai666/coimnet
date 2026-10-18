package main

import "testing"

func TestSplitIDMatchesPythonWriter(t *testing.T) {
	labels := []int{3, 0, 0, 1, 2, 3, 1, 4, 5, 6, 7, 8, 9, 2}
	got, err := balancedIndices(labels, 12)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1, 3, 4, 0, 7, 8, 9, 10, 11, 12, 2, 6}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("indices = %v, want %v", got, want)
		}
	}
	// Written by scripts/bench/kmnist_mlp.py split_id for the same selection.
	const pythonSplit = "balanced_round_robin_by_label_v1:12:6f90361676a1b8788bc3304a65fc4ff6ca1dd2c38f14ac8148d219b0c8618c19"
	if id := SplitID(got); id != pythonSplit {
		t.Fatalf("SplitID = %q, Python writes %q", id, pythonSplit)
	}
	if _, err := balancedIndices(labels, 15); err == nil {
		t.Fatal("asking for more images than exist must fail")
	}
}
