package main

import "testing"

func TestArgmaxClassSkipsBlank(t *testing.T) {
	// Column 0 is the blank: even when it is largest, the answer is a label.
	if got := argmaxClass([]float64{9, .1, .7, .3}); got != 1 {
		t.Fatalf("argmaxClass = %d, want label 1", got)
	}
	if got := argmaxClass([]float64{0, .5, .5}); got != 0 {
		t.Fatalf("a tie keeps the first label, got %d", got)
	}
}

func TestScenarioClasses(t *testing.T) {
	if c, inv, err := scenarioClasses("new-classes"); err != nil || inv || len(c) != 5 {
		t.Fatalf("new-classes = %v %v %v", c, inv, err)
	}
	if c, inv, err := scenarioClasses("domain"); err != nil || !inv || len(c) != 10 {
		t.Fatalf("domain = %v %v %v", c, inv, err)
	}
	if _, _, err := scenarioClasses("other"); err == nil {
		t.Fatal("an unknown scenario must fail")
	}
}
