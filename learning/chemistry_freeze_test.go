package learning_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

// Freezing holds the concentration at chemC1 (the value after the two warm
// rows of the shared fixture) while the individual keeps stepping. A thawed
// twin starting from the same snapshot decays on every row, so after the same
// three rows it sits at lambda*chemC3, the row-t=4 value of the hand table.
const chemC4 = 0.17558975382363426

// TestFreezeChemistryHoldsConcentration pins LRN-08's evaluation posture: once
// FreezeChemistry(true) is set, the state passes through bit-identical on every
// later row, nothing releases, and occupancy and effects keep reading the held
// concentration, so the core sees exactly the modulation of the frozen value.
func TestFreezeChemistryHoldsConcentration(t *testing.T) {
	a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	enableChemistry(t, a, sensitivityDeclaration())
	advance(t, a, [][]float64{{1, 0}})
	advance(t, a, [][]float64{{1, 0}})
	if c := a.ChemistryReport().Concentration[0][0]; !closeEnough(c, chemC1) {
		t.Fatalf("pre-freeze concentration = %.17g, want %.17g", c, chemC1)
	}
	before := a.Snapshot()
	if before.Chemical.State.Steps != 2 {
		t.Fatalf("pre-freeze steps = %d, want 2", before.Chemical.State.Steps)
	}
	if err := a.FreezeChemistry(true); err != nil {
		t.Fatalf("FreezeChemistry(true) error = %v", err)
	}
	frozenOut := advance(t, a, [][]float64{{1, 0}, {1, 0}, {1, 0}})
	report := a.ChemistryReport()
	if report.Steps != 3 {
		t.Fatalf("report.Steps = %d, want 3", report.Steps)
	}
	if !report.Frozen {
		t.Fatal("report.Frozen = false under a frozen advance")
	}
	if !sameBits(report.Concentration[0], before.Chemical.State.Concentration[0]) {
		t.Fatalf("concentration after freezing = %v, want the held %v", report.Concentration[0], before.Chemical.State.Concentration[0])
	}
	for _, total := range report.ReleaseTotal {
		if total != 0 {
			t.Fatalf("frozen ReleaseTotal entry = %v, want 0", total)
		}
	}
	after := a.Snapshot()
	if !reflect.DeepEqual(after.Chemical.State, before.Chemical.State) {
		t.Fatalf("the held state drifted:\n got %+v\nwant %+v", after.Chemical.State, before.Chemical.State)
	}

	twin, err := learning.RestoreIndividual(before)
	if err != nil {
		t.Fatalf("RestoreIndividual() error = %v", err)
	}
	thawedOut := advance(t, twin, [][]float64{{1, 0}, {1, 0}, {1, 0}})
	thawed := twin.ChemistryReport()
	if thawed.Frozen {
		t.Fatal("the restored twin is frozen")
	}
	if c := thawed.Concentration[0][0]; !closeEnough(c, chemC4) {
		t.Fatalf("thawed concentration = %.17g, want the decayed %.17g", c, chemC4)
	}
	if sameRows(frozenOut, thawedOut) {
		t.Fatalf("the frozen and the thawed individual produced the same output %v", frozenOut)
	}
}

// TestFreezeChemistryTwiceFromSameSnapshotIsBitIdentical pins that the freeze
// contributes no choice: two individuals restored from one snapshot and frozen
// are indistinguishable in output and in report.
func TestFreezeChemistryTwiceFromSameSnapshotIsBitIdentical(t *testing.T) {
	a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	enableChemistry(t, a, sensitivityDeclaration())
	advance(t, a, [][]float64{{1, 0}, {1, 0}})
	feed := [][]float64{{1, 0}, {1, 0}, {1, 0}}
	snap := a.Snapshot()
	first, err := learning.RestoreIndividual(snap)
	if err != nil {
		t.Fatalf("RestoreIndividual(first) error = %v", err)
	}
	second, err := learning.RestoreIndividual(snap)
	if err != nil {
		t.Fatalf("RestoreIndividual(second) error = %v", err)
	}
	if err := first.FreezeChemistry(true); err != nil {
		t.Fatalf("FreezeChemistry(first) error = %v", err)
	}
	if err := second.FreezeChemistry(true); err != nil {
		t.Fatalf("FreezeChemistry(second) error = %v", err)
	}
	firstOut := advance(t, first, feed)
	secondOut := advance(t, second, feed)
	if !sameRows(firstOut, secondOut) {
		t.Fatalf("twins diverged: %v against %v", firstOut, secondOut)
	}
	if got, want := first.ChemistryReport(), second.ChemistryReport(); !reflect.DeepEqual(got, want) {
		t.Fatalf("twin reports diverged:\n got %+v\nwant %+v", got, want)
	}
}

// TestFreezeChemistryIsNotInTheSnapshot pins that the freeze flag lives on the
// runtime only: a restored individual is never frozen and keeps evolving its
// concentration.
func TestFreezeChemistryIsNotInTheSnapshot(t *testing.T) {
	a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	enableChemistry(t, a, sensitivityDeclaration())
	advance(t, a, [][]float64{{1, 0}, {1, 0}})
	if err := a.FreezeChemistry(true); err != nil {
		t.Fatalf("FreezeChemistry(true) error = %v", err)
	}
	restored, err := learning.RestoreIndividual(a.Snapshot())
	if err != nil {
		t.Fatalf("RestoreIndividual() error = %v", err)
	}
	advance(t, restored, [][]float64{{1, 0}, {1, 0}, {1, 0}})
	report := restored.ChemistryReport()
	if report.Frozen {
		t.Fatal("a restored individual reports frozen")
	}
	if c := report.Concentration[0][0]; !closeEnough(c, chemC4) {
		t.Fatalf("restored concentration = %.17g, want the decayed %.17g", c, chemC4)
	}
}

// TestFreezeChemistryNeedsChemistry pins that freezing without an enabled
// layer is an error, and that DisableChemistry drops the flag with the runtime,
// so a fresh EnableChemistry starts unfrozen.
func TestFreezeChemistryNeedsChemistry(t *testing.T) {
	a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	if err := a.FreezeChemistry(true); err == nil || !strings.Contains(err.Error(), "chemistry") {
		t.Fatalf("FreezeChemistry without chemistry error = %v, want one containing \"chemistry\"", err)
	}
	enableChemistry(t, a, chemistryDeclaration())
	if err := a.FreezeChemistry(true); err != nil {
		t.Fatalf("FreezeChemistry(true) error = %v", err)
	}
	advance(t, a, [][]float64{{1, 0}})
	if report := a.ChemistryReport(); !report.Frozen {
		t.Fatal("report.Frozen = false under a frozen advance")
	}
	if err := a.DisableChemistry(); err != nil {
		t.Fatalf("DisableChemistry() error = %v", err)
	}
	if err := a.FreezeChemistry(true); err == nil || !strings.Contains(err.Error(), "chemistry") {
		t.Fatalf("FreezeChemistry after DisableChemistry error = %v, want one containing \"chemistry\"", err)
	}
	enableChemistry(t, a, chemistryDeclaration())
	advance(t, a, [][]float64{{1, 0}})
	if report := a.ChemistryReport(); report.Frozen {
		t.Fatal("a re-enabled layer is frozen")
	}
}
