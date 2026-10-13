package learning_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func TestResetOptimizerPreservesConsolidationLifecycle(t *testing.T) {
	a, _ := consolidationIndividual(t, 3)
	trigger := learning.ConsolidationTrigger{Receptor: 0, Threshold: 0, Rate: 0.5, Retain: 0.25}

	for episode := 0; episode < 2; episode++ {
		if _, err := a.TrainEpisode(context.Background(), [][]float64{{1}}, []float64{0.5}); err != nil {
			t.Fatalf("TrainEpisode(%d) error = %v", episode+1, err)
		}
	}
	trained := a.Snapshot()
	if trained.Optimizer.Episodes != 2 {
		t.Fatalf("episodes before consolidation = %d, want 2", trained.Optimizer.Episodes)
	}

	first, err := a.Consolidate(context.Background(), trigger)
	if err != nil {
		t.Fatalf("first Consolidate() error = %v", err)
	}
	if !first.Applied || first.LastEpisode != 3 || first.Used != 1 {
		t.Fatalf("first consolidation report = %+v, want applied episode 3 used 1", first)
	}
	before := a.Snapshot()
	if before.Optimizer.Episodes != 2 || before.Plastic == nil || before.Plastic.Slow == nil || before.Plastic.Slow.LastEpisode != 3 {
		t.Fatalf("fixture lifecycle = episodes %d slow %+v, want episodes 2 and last episode 3", before.Optimizer.Episodes, before.Plastic)
	}

	if err := a.ResetOptimizer(context.Background(), learning.DefaultOptions()); err != nil {
		t.Fatalf("ResetOptimizer() error = %v", err)
	}
	after := a.Snapshot()
	assertResetPreservedState(t, before, after, true)
	if after.Plastic.Slow.LastEpisode != 1 {
		t.Errorf("last episode after reset = %d, want 1 for an already consolidated current episode", after.Plastic.Slow.LastEpisode)
	}

	report, err := a.Consolidate(context.Background(), trigger)
	if !errors.Is(err, learning.ErrAlreadyConsolidated) || !report.Already || report.Applied {
		t.Fatalf("same-episode Consolidate() after reset = report %+v, error %v", report, err)
	}

	if _, err := a.TrainEpisode(context.Background(), [][]float64{{1}}, []float64{0.5}); err != nil {
		t.Fatalf("TrainEpisode() after reset error = %v", err)
	}
	afterTrain := a.Snapshot()
	if afterTrain.Optimizer.Episodes != 1 {
		t.Fatalf("episodes after the new TrainEpisode() = %d, want 1", afterTrain.Optimizer.Episodes)
	}

	report, err = a.Consolidate(context.Background(), trigger)
	if err != nil {
		t.Fatalf("first new-episode Consolidate() error = %v", err)
	}
	if !report.Applied || report.LastEpisode != 2 || report.Used != 2 {
		t.Fatalf("first new-episode consolidation report = %+v, want applied episode 2 used 2", report)
	}
	if _, err := a.Consolidate(context.Background(), trigger); !errors.Is(err, learning.ErrAlreadyConsolidated) {
		t.Fatalf("second new-episode Consolidate() error = %v, want ErrAlreadyConsolidated", err)
	}
}

func TestResetOptimizerAllowsLaterUnconsolidatedEpisode(t *testing.T) {
	a, _ := consolidationIndividual(t, 2)
	trigger := learning.ConsolidationTrigger{Receptor: 0, Threshold: 0, Rate: 0.5, Retain: 0.25}
	if _, err := a.TrainEpisode(context.Background(), [][]float64{{1}}, []float64{0.5}); err != nil {
		t.Fatalf("first TrainEpisode() error = %v", err)
	}
	if _, err := a.Consolidate(context.Background(), trigger); err != nil {
		t.Fatalf("old-episode Consolidate() error = %v", err)
	}
	if _, err := a.TrainEpisode(context.Background(), [][]float64{{1}}, []float64{0.5}); err != nil {
		t.Fatalf("later unconsolidated TrainEpisode() error = %v", err)
	}
	before := a.Snapshot()
	if before.Optimizer.Episodes != 2 || before.Plastic == nil || before.Plastic.Slow == nil || before.Plastic.Slow.LastEpisode != 2 || before.Plastic.Slow.Used != 1 {
		t.Fatalf("fixture lifecycle = episodes %d slow %+v, want episode 2 unconsolidated after marker 2", before.Optimizer.Episodes, before.Plastic)
	}

	if err := a.ResetOptimizer(context.Background(), learning.DefaultOptions()); err != nil {
		t.Fatalf("ResetOptimizer() error = %v", err)
	}
	after := a.Snapshot()
	assertResetPreservedState(t, before, after, false)
	if after.Plastic.Slow.LastEpisode != 0 {
		t.Errorf("last episode after reset = %d, want 0 for an unconsolidated current episode", after.Plastic.Slow.LastEpisode)
	}

	report, err := a.Consolidate(context.Background(), trigger)
	if err != nil {
		t.Fatalf("Consolidate() after a later unconsolidated reset error = %v", err)
	}
	if !report.Applied || report.LastEpisode != 1 || report.Used != 2 {
		t.Fatalf("consolidation after a later unconsolidated reset = %+v, want applied episode 1 used 2", report)
	}
	if _, err := a.Consolidate(context.Background(), trigger); !errors.Is(err, learning.ErrAlreadyConsolidated) {
		t.Fatalf("same-episode Consolidate() error = %v, want ErrAlreadyConsolidated", err)
	}
}

func TestResetOptimizerFailureAtomicityAndRestore(t *testing.T) {
	a, _ := consolidationIndividual(t, 2)
	trigger := learning.ConsolidationTrigger{Receptor: 0, Threshold: 0, Rate: 0.5, Retain: 0.25}
	for episode := 0; episode < 2; episode++ {
		if _, err := a.TrainEpisode(context.Background(), [][]float64{{1}}, []float64{0.5}); err != nil {
			t.Fatalf("TrainEpisode(%d) error = %v", episode+1, err)
		}
	}
	if _, err := a.Consolidate(context.Background(), trigger); err != nil {
		t.Fatalf("fixture Consolidate() error = %v", err)
	}
	before := a.Snapshot()
	if before.Optimizer.Episodes != 2 || before.Plastic == nil || before.Plastic.Slow == nil || before.Plastic.Slow.LastEpisode != 3 {
		t.Fatalf("fixture lifecycle = episodes %d slow %+v, want episodes 2 and last episode 3", before.Optimizer.Episodes, before.Plastic)
	}

	invalid := learning.DefaultOptions()
	invalid.Beta1 = 1
	if err := a.ResetOptimizer(context.Background(), invalid); err == nil {
		t.Fatal("ResetOptimizer() accepted invalid options")
	}
	if after := a.Snapshot(); !reflect.DeepEqual(after, before) {
		t.Fatalf("failed ResetOptimizer() changed the snapshot: before %+v after %+v", before, after)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.ResetOptimizer(canceled, learning.DefaultOptions()); err == nil {
		t.Fatal("ResetOptimizer() accepted a canceled context")
	}
	if after := a.Snapshot(); !reflect.DeepEqual(after, before) {
		t.Fatalf("canceled ResetOptimizer() changed the snapshot: before %+v after %+v", before, after)
	}

	if err := a.ResetOptimizer(context.Background(), learning.DefaultOptions()); err != nil {
		t.Fatalf("valid ResetOptimizer() error = %v", err)
	}
	reset := a.Snapshot()
	assertResetPreservedState(t, before, reset, true)
	if reset.Optimizer.Episodes != 0 || reset.Plastic.Slow.LastEpisode != 1 {
		t.Fatalf("reset lifecycle = episodes %d slow %+v, want episodes 0 and last episode 1", reset.Optimizer.Episodes, reset.Plastic.Slow)
	}
	restored, err := learning.RestoreIndividual(reset)
	if err != nil {
		t.Fatalf("RestoreIndividual() after ResetOptimizer() error = %v", err)
	}
	if after := restored.Snapshot(); !reflect.DeepEqual(after, reset) {
		t.Fatalf("restored reset snapshot differs: reset %+v restored %+v", reset, after)
	}
}

func assertResetPreservedState(t *testing.T, before, after learning.IndividualSnapshot, currentEpisodeConsolidated bool) {
	t.Helper()
	if after.Optimizer.Episodes != 0 {
		t.Fatalf("episodes after ResetOptimizer() = %d, want 0", after.Optimizer.Episodes)
	}
	if !reflect.DeepEqual(after.Config, before.Config) || !reflect.DeepEqual(after.Parameters, before.Parameters) {
		t.Fatal("ResetOptimizer() changed configuration or parameters")
	}
	if !reflect.DeepEqual(after.Neural, before.Neural) {
		t.Fatalf("ResetOptimizer() changed neural state: before %+v after %+v", before.Neural, after.Neural)
	}
	if !reflect.DeepEqual(after.Chemical, before.Chemical) {
		t.Fatalf("ResetOptimizer() changed chemical state: before %+v after %+v", before.Chemical, after.Chemical)
	}
	if before.Plastic == nil || after.Plastic == nil || before.Plastic.Slow == nil || after.Plastic.Slow == nil {
		t.Fatal("fixture is missing the slow plastic state")
	}
	wantPlastic := *before.Plastic
	wantSlow := *before.Plastic.Slow
	wantSlow.Values = append([]float64(nil), before.Plastic.Slow.Values...)
	if currentEpisodeConsolidated {
		wantSlow.LastEpisode = 1
	} else {
		wantSlow.LastEpisode = 0
	}
	wantPlastic.Slow = &wantSlow
	if !reflect.DeepEqual(after.Plastic, &wantPlastic) {
		t.Errorf("ResetOptimizer() changed plastic state beyond LastEpisode remapping: before %+v after %+v want %+v", before.Plastic, after.Plastic, &wantPlastic)
	}
	if !reflect.DeepEqual(after.Plastic.Slow.Values, before.Plastic.Slow.Values) || after.Plastic.Slow.Budget != before.Plastic.Slow.Budget || after.Plastic.Slow.Used != before.Plastic.Slow.Used {
		t.Fatalf("ResetOptimizer() changed slow consolidation state: before %+v after %+v", before.Plastic.Slow, after.Plastic.Slow)
	}
}
