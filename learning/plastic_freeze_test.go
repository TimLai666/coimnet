package learning_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

// plasticFreezeWarmUp is the open-gate prologue every case here shares: three
// rows on the continuous fixture leave both an eligibility and a fast change
// behind, so a later rule update would be visible in the state and in the
// output.
func plasticFreezeWarmUp(t *testing.T) *learning.Individual {
	t.Helper()
	a := newIndividual(t, continuousConfig(), continuousParameters())
	if err := a.EnablePlasticity(plasticConfig(0)); err != nil {
		t.Fatal(err)
	}
	advanceGated(t, a, [][]float64{{1}, {1}, {1}}, []float64{1, 1, 1})
	s := a.Snapshot()
	if s.Plastic == nil {
		t.Fatal("an enabled individual must carry a plastic part")
	}
	if s.Plastic.State.Plastic[0] == 0 || s.Plastic.State.Eligibility[0] == 0 {
		t.Fatalf("the warm rows left nothing to hold: %+v", s.Plastic.State)
	}
	return a
}

// TestFreezePlasticityHoldsFastState is the evaluation posture of the local
// mechanism: once FreezePlasticity(true) is set the core keeps integrating the
// effective weights of the held fast change, but the rule update is skipped, so
// the fast state passes through bit for bit and no step or bound is counted.
// The first row of an un-frozen twin restored from the same snapshot matches
// bit for bit, because a rule update lands after the step it follows; the
// second row is where the twin has already moved and the frozen individual has
// not.
func TestFreezePlasticityHoldsFastState(t *testing.T) {
	a := plasticFreezeWarmUp(t)
	before := a.Snapshot()
	if err := a.FreezePlasticity(true); err != nil {
		t.Fatalf("FreezePlasticity(true) error = %v", err)
	}
	// A zero gate is exactly Advance with plasticity enabled; it is spelled as
	// AdvanceGated only because Advance returns no report.
	first, firstReport := advanceGated(t, a, [][]float64{{1}}, []float64{0})
	second, secondReport := advanceGated(t, a, [][]float64{{1}}, []float64{0})
	for i, report := range []learning.PlasticReport{firstReport, secondReport} {
		if !report.Frozen {
			t.Fatalf("report %d Frozen = false under a frozen advance", i)
		}
		if report.Steps != 0 || report.Clamped != 0 || report.HeldAtWMin != 0 {
			t.Fatalf("frozen advance %d counted %+v, want no step and no bound", i, report)
		}
	}
	after := a.Snapshot()
	if !reflect.DeepEqual(after.Plastic, before.Plastic) {
		t.Fatalf("the held fast state drifted:\n got %+v\nwant %+v", after.Plastic, before.Plastic)
	}

	twin, err := learning.RestoreIndividual(before)
	if err != nil {
		t.Fatalf("RestoreIndividual() error = %v", err)
	}
	twinFirst := advance(t, twin, [][]float64{{1}})
	twinSecond := advance(t, twin, [][]float64{{1}})
	if !sameRows(first, twinFirst) {
		t.Fatalf("the first frozen row differs from the twin's:\n got %v\nwant %v", first, twinFirst)
	}
	if sameRows(second, twinSecond) {
		t.Fatalf("the second row of the twin still equals the frozen one: %v", second)
	}
	if reflect.DeepEqual(twin.Snapshot().Plastic, before.Plastic) {
		t.Fatal("the un-frozen twin held its fast state")
	}

	// An open gate is the receptor-driven case of the evaluation path: a frozen
	// row ignores it rather than turning it into a fast change.
	advanceGated(t, a, [][]float64{{1}}, []float64{1})
	if gated := a.Snapshot(); !reflect.DeepEqual(gated.Plastic, before.Plastic) {
		t.Fatalf("an open gate moved the frozen state:\n got %+v\nwant %+v", gated.Plastic, before.Plastic)
	}
}

// TestFreezePlasticityIsNotInTheSnapshot pins that the flag lives on the
// runtime only: an individual restored from a frozen one's snapshot keeps
// updating its fast state.
func TestFreezePlasticityIsNotInTheSnapshot(t *testing.T) {
	a := plasticFreezeWarmUp(t)
	if err := a.FreezePlasticity(true); err != nil {
		t.Fatalf("FreezePlasticity(true) error = %v", err)
	}
	before := a.Snapshot()
	restored, err := learning.RestoreIndividual(before)
	if err != nil {
		t.Fatalf("RestoreIndividual() error = %v", err)
	}
	_, report := advanceGated(t, restored, [][]float64{{1}}, []float64{1})
	if report.Frozen {
		t.Fatal("a restored individual reports frozen")
	}
	if report.Steps != 1 {
		t.Fatalf("report.Steps = %d, want 1", report.Steps)
	}
	if reflect.DeepEqual(restored.Snapshot().Plastic, before.Plastic) {
		t.Fatal("a restored individual held its fast state")
	}
}

// TestFreezePlasticityNeedsPlasticity pins that freezing without an enabled
// mechanism is an error, and that the flag dies with the runtime: disabling
// drops it, and a fresh EnablePlasticity starts un-frozen.
func TestFreezePlasticityNeedsPlasticity(t *testing.T) {
	a := newIndividual(t, continuousConfig(), continuousParameters())
	if err := a.FreezePlasticity(true); err == nil || !strings.Contains(err.Error(), "plasticity") {
		t.Fatalf("FreezePlasticity without plasticity error = %v, want one containing \"plasticity\"", err)
	}
	if err := a.EnablePlasticity(plasticConfig(0)); err != nil {
		t.Fatal(err)
	}
	if err := a.FreezePlasticity(true); err != nil {
		t.Fatalf("FreezePlasticity(true) error = %v", err)
	}
	if _, report := advanceGated(t, a, [][]float64{{1}}, []float64{1}); !report.Frozen {
		t.Fatal("report.Frozen = false under a frozen advance")
	}
	a.DisablePlasticity()
	if _, report := advanceGated(t, a, [][]float64{{1}}, []float64{0}); report.Frozen {
		t.Fatalf("a disabled individual reported %+v", report)
	}
	if err := a.FreezePlasticity(true); err == nil || !strings.Contains(err.Error(), "plasticity") {
		t.Fatalf("FreezePlasticity after DisablePlasticity error = %v, want one containing \"plasticity\"", err)
	}
	if err := a.EnablePlasticity(plasticConfig(0)); err != nil {
		t.Fatal(err)
	}
	if _, report := advanceGated(t, a, [][]float64{{1}}, []float64{1}); report.Frozen {
		t.Fatal("a re-enabled mechanism is frozen")
	}
}
