package learning_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
)

// The consolidation fixture is two continuous neurons joined by one
// instantaneous edge weight 0.9, with plasticity on that edge, a chemical layer
// whose single hypothesized receptor on node 0 reaches occupancy chemOcc1 after
// two rows, and a consolidation layer budgeted for the caller's test. Two gated
// rows have already run, so the fast state holds a measured P1 the hand
// computation can predict against.
func consolidationConfig() learning.Config {
	return learning.Config{
		Dynamics:     dynamics.Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{0}, DT: 1, Activation: "tanh"},
		InputSize:    1,
		OutputSize:   1,
		InputNodes:   []int{0},
		ReadoutNodes: []int{1},
	}
}

func consolidationParameters() learning.Parameters {
	return learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{0.9}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}},
		Encoder: []float64{1},
		Readout: []float64{1},
	}
}

func consolidationChemistry() modulation.ChemistryConfig {
	return modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{2}},
		Sources: []modulation.SourceSpec{{
			Kind: modulation.SourceExternalTimeline, Channel: 0,
			Timeline: &modulation.ExternalTimeline{ChannelCount: 1, Entries: []modulation.TimelineEntry{{Step: 1, Channel: 0, Rate: 1}}},
		}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{
			{Cells: []int{0}, Signal: "octopamine", Channel: 0, Status: modulation.StatusHypothesized, Kd: 0.5, N: 1,
				Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "consol-fixture/v1"},
		}},
		Regions: modulation.RegionAssignment{NodeRegion: []int{0, 0}},
	}
}

// consolidationIndividual removes the two rows against the same concentration
// table the chemistry tests hand-compute: row 1 releases the pulse, so the last
// row's occupancy is chemOcc1. P1 is the fast change those rows accumulated,
// measured from the snapshot the same way the hand computation reads it.
func consolidationIndividual(t *testing.T, budget uint64) (*learning.Individual, float64) {
	t.Helper()
	a := newIndividual(t, consolidationConfig(), consolidationParameters())
	if err := a.EnablePlasticity(plasticConfig(0)); err != nil {
		t.Fatal(err)
	}
	enableChemistry(t, a, consolidationChemistry())
	if err := a.EnableConsolidation(budget); err != nil {
		t.Fatal(err)
	}
	advanceGated(t, a, [][]float64{{1}, {1}}, []float64{1, 1})
	p1 := a.Snapshot().Plastic.State.Plastic[0]
	if p1 == 0 {
		t.Fatal("the fixture accumulated no fast change")
	}
	return a, p1
}

// TestConsolidateHandComputed writes one consolidation by hand: two gated rows
// left the fast change at a measured P1, and the write is exactly
// slow = Rate*P1 followed by plastic *= Retain, with the occupancy the last row
// produced at the declared threshold, the counters advanced and the L2 norms
// split per layer.
func TestConsolidateHandComputed(t *testing.T) {
	a, p1 := consolidationIndividual(t, 1)
	report, err := a.Consolidate(context.Background(), learning.ConsolidationTrigger{
		Receptor: 0, Threshold: 0.5, Rate: 0.5, Retain: 0.25,
	})
	if err != nil {
		t.Fatalf("Consolidate() error = %v", err)
	}
	if !report.Applied || report.GateClosed || report.Already || report.BudgetExhausted {
		t.Fatalf("report = %+v, want Applied only", report)
	}
	if report.Episode != 0 || report.LastEpisode != 1 || report.Used != 1 {
		t.Fatalf("report counters = {episode %d last %d used %d}, want {0 1 1}", report.Episode, report.LastEpisode, report.Used)
	}
	if !closeEnough(report.Occupancy, chemOcc1) {
		t.Fatalf("report.Occupancy = %v, want %v", report.Occupancy, chemOcc1)
	}
	if !closeEnough(report.BaseL2, 0.9) {
		t.Fatalf("report.BaseL2 = %v, want 0.9", report.BaseL2)
	}
	snap := a.Snapshot()
	slow := snap.Plastic.Slow
	if slow == nil {
		t.Fatal("the snapshot lost the slow layer")
	}
	if slow.Values[0] != 0.5*p1 {
		t.Fatalf("slow[0] = %v, want %v", slow.Values[0], 0.5*p1)
	}
	if snap.Plastic.State.Plastic[0] != 0.25*p1 {
		t.Fatalf("plastic[0] = %v, want %v", snap.Plastic.State.Plastic[0], 0.25*p1)
	}
	if !closeEnough(report.SlowL2, math.Abs(0.5*p1)) {
		t.Fatalf("report.SlowL2 = %v, want %v", report.SlowL2, math.Abs(0.5*p1))
	}
	if !closeEnough(report.PlasticL2, math.Abs(0.25*p1)) {
		t.Fatalf("report.PlasticL2 = %v, want %v", report.PlasticL2, math.Abs(0.25*p1))
	}
	if slow.LastEpisode != 1 || slow.Budget != 1 || slow.Used != 1 {
		t.Fatalf("slow state counters = %+v, want one used slot", slow)
	}
}

// TestConsolidateSameEpisodeRefused writes once and then again on the same
// episode: the second call refuses before touching any state.
func TestConsolidateSameEpisodeRefused(t *testing.T) {
	a, _ := consolidationIndividual(t, 1)
	trigger := learning.ConsolidationTrigger{Receptor: 0, Threshold: 0.5, Rate: 0.5, Retain: 0.25}
	if _, err := a.Consolidate(context.Background(), trigger); err != nil {
		t.Fatalf("first Consolidate() error = %v", err)
	}
	before := a.Snapshot()
	report, err := a.Consolidate(context.Background(), trigger)
	if !errors.Is(err, learning.ErrAlreadyConsolidated) {
		t.Fatalf("second Consolidate() error = %v, want ErrAlreadyConsolidated", err)
	}
	if !report.Already || report.Applied {
		t.Fatalf("refused report = %+v", report)
	}
	after := a.Snapshot()
	if !reflect.DeepEqual(after.Plastic.Slow, before.Plastic.Slow) {
		t.Fatalf("refused consolidation changed the slow layer: %+v against %+v", after.Plastic.Slow, before.Plastic.Slow)
	}
	if !sameBits(after.Plastic.State.Plastic, before.Plastic.State.Plastic) {
		t.Fatalf("refused consolidation changed the fast layer: %v against %v", after.Plastic.State.Plastic, before.Plastic.State.Plastic)
	}
}

// TestConsolidateBudgetExhausted claims the only budget slot and then runs one
// more episode: the next attempt is refused as exhausted, with the counters
// intact.
func TestConsolidateBudgetExhausted(t *testing.T) {
	a, _ := consolidationIndividual(t, 1)
	trigger := learning.ConsolidationTrigger{Receptor: 0, Threshold: 0.5, Rate: 0.5, Retain: 0.25}
	if _, err := a.TrainEpisode(context.Background(), [][]float64{{1}}, []float64{0.5}); err != nil {
		t.Fatalf("first TrainEpisode() error = %v", err)
	}
	if _, err := a.Consolidate(context.Background(), trigger); err != nil {
		t.Fatalf("first Consolidate() error = %v", err)
	}
	if _, err := a.TrainEpisode(context.Background(), [][]float64{{1}}, []float64{0.5}); err != nil {
		t.Fatalf("second TrainEpisode() error = %v", err)
	}
	report, err := a.Consolidate(context.Background(), trigger)
	if !errors.Is(err, learning.ErrBudgetExhausted) {
		t.Fatalf("Consolidate() error = %v, want ErrBudgetExhausted", err)
	}
	if !report.BudgetExhausted || report.Applied {
		t.Fatalf("budget-exhausted report = %+v", report)
	}
	snap := a.Snapshot()
	if snap.Optimizer.Episodes != 2 {
		t.Fatalf("episodes = %d, want 2", snap.Optimizer.Episodes)
	}
	if snap.Plastic.Slow.Used != 1 || snap.Plastic.Slow.Budget != 1 {
		t.Fatalf("slow counters = {used %d budget %d}, want {1 1}", snap.Plastic.Slow.Used, snap.Plastic.Slow.Budget)
	}
}

// TestConsolidateGateClosed runs the fixture against a threshold no occupancy
// the model ever reaches: the write is refused and no state changes.
func TestConsolidateGateClosed(t *testing.T) {
	a, _ := consolidationIndividual(t, 1)
	before := a.Snapshot()
	_, err := a.Consolidate(context.Background(), learning.ConsolidationTrigger{Receptor: 0, Threshold: 1.0, Rate: 0.5, Retain: 0.25})
	if !errors.Is(err, learning.ErrConsolidationGateClosed) {
		t.Fatalf("Consolidate() error = %v, want ErrConsolidationGateClosed", err)
	}
	after := a.Snapshot()
	if !reflect.DeepEqual(after, before) {
		t.Fatal("a refused consolidation changed the snapshot")
	}
}

// TestSlowStateEntersEffectiveWeights proves the slow layer is not decoration:
// folding the whole slow layer into the base weight of its edge and declaring
// no slow layer must reproduce every output, report and voltage bit for bit,
// because the effective weight of a free edge is (base + slow) + plastic in
// both individuals.
func TestSlowStateEntersEffectiveWeights(t *testing.T) {
	a, _ := consolidationIndividual(t, 1)
	if _, err := a.Consolidate(context.Background(), learning.ConsolidationTrigger{
		Receptor: 0, Threshold: 0.5, Rate: 0.5, Retain: 0.25,
	}); err != nil {
		t.Fatalf("Consolidate() error = %v", err)
	}
	snapshot := a.Snapshot()
	folded := snapshot
	for k, edge := range folded.Plastic.Config.Edges {
		folded.Parameters.Core.Weights[edge] = snapshot.Parameters.Core.Weights[edge] + snapshot.Plastic.Slow.Values[k]
	}
	folded.Plastic.Slow = nil

	kept, err := learning.RestoreIndividual(snapshot)
	if err != nil {
		t.Fatalf("RestoreIndividual(kept) error = %v", err)
	}
	moved, err := learning.RestoreIndividual(folded)
	if err != nil {
		t.Fatalf("RestoreIndividual(folded) error = %v", err)
	}
	rows := [][]float64{{1}, {0}, {1}}
	keptOut, keptReport := advanceGated(t, kept, rows, []float64{0, 0, 0})
	movedOut, movedReport := advanceGated(t, moved, rows, []float64{0, 0, 0})
	if !sameRows(keptOut, movedOut) {
		t.Fatalf("outputs diverged between the slow layer and the folded base: %v against %v", movedOut, keptOut)
	}
	if keptReport != movedReport {
		t.Fatalf("plastic reports diverged: %+v against %+v", movedReport, keptReport)
	}
	if !sameBits(voltageOf(t, kept), voltageOf(t, moved)) {
		t.Fatalf("voltages diverged: %v against %v", voltageOf(t, moved), voltageOf(t, kept))
	}
}

// TestConsolidationSnapshotRoundTrip carries the slow layer and the episode
// clock through a JSON snapshot exactly like a checkpoint, resumes with one
// more episode and confirms the per-episode rule and the alias isolation.
func TestConsolidationSnapshotRoundTrip(t *testing.T) {
	a, _ := consolidationIndividual(t, 2)
	if _, err := a.TrainEpisode(context.Background(), [][]float64{{1}}, []float64{0.5}); err != nil {
		t.Fatalf("TrainEpisode() error = %v", err)
	}
	trigger := learning.ConsolidationTrigger{Receptor: 0, Threshold: 0.3, Rate: 0.5, Retain: 0.25}
	if _, err := a.Consolidate(context.Background(), trigger); err != nil {
		t.Fatalf("Consolidate() error = %v", err)
	}
	before := a.Snapshot()
	if before.Optimizer.Episodes != 1 || before.Plastic.Slow.Used != 1 {
		t.Fatalf("fixture counters = {episodes %d used %d}, want {1 1}", before.Optimizer.Episodes, before.Plastic.Slow.Used)
	}
	data, err := json.Marshal(before)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded learning.IndividualSnapshot
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	restored, err := learning.RestoreIndividual(decoded)
	if err != nil {
		t.Fatalf("RestoreIndividual() error = %v", err)
	}
	after := restored.Snapshot()
	if !reflect.DeepEqual(after.Plastic.Slow, before.Plastic.Slow) {
		t.Fatalf("the slow layer changed: %+v against %+v", after.Plastic.Slow, before.Plastic.Slow)
	}
	if after.Optimizer.Episodes != before.Optimizer.Episodes {
		t.Fatalf("episodes changed: %d against %d", after.Optimizer.Episodes, before.Optimizer.Episodes)
	}
	// The occupancy report is transient, so the resumed run refreshes it with one
	// gated row before training and writing exactly like an uninterrupted run
	// has to advance before its consolidate.
	advanceGated(t, restored, [][]float64{{1}}, []float64{1})
	if _, err := restored.TrainEpisode(context.Background(), [][]float64{{1}}, []float64{0.5}); err != nil {
		t.Fatalf("resumed TrainEpisode() error = %v", err)
	}
	r1, err := restored.Consolidate(context.Background(), trigger)
	if err != nil {
		t.Fatalf("resumed Consolidate() error = %v", err)
	}
	if !r1.Applied || r1.LastEpisode != 3 {
		t.Fatalf("resumed write report = %+v", r1)
	}
	if _, err := restored.Consolidate(context.Background(), trigger); !errors.Is(err, learning.ErrAlreadyConsolidated) {
		t.Fatalf("resumed same-episode Consolidate() error = %v", err)
	}
	before.Plastic.Slow.Values[0] = 99
	if a.Snapshot().Plastic.Slow.Values[0] == 99 {
		t.Fatal("a snapshot shares the slow buffer with the running individual")
	}
}
