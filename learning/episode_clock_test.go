package learning_test

import (
	"context"
	"errors"
	"github.com/TimLai666/coimnet/learning"
	"reflect"
	"testing"
)

func episodeClockIndividual(t *testing.T, chemistry bool, episodes uint64) *learning.Individual {
	t.Helper()
	var a *learning.Individual
	if chemistry {
		a, _ = consolidationIndividual(t, 3)
	} else {
		a = newIndividual(t, consolidationConfig(), consolidationParameters())
	}
	s := a.Snapshot()
	s.Optimizer.Episodes = episodes
	a, err := learning.RestoreIndividual(s)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func TestTrainEpisodeClockOverflowIsAtomic(t *testing.T) {
	for _, chemistry := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain", true: "chemistry"}[chemistry], func(t *testing.T) {
			a := episodeClockIndividual(t, chemistry, ^uint64(0))
			before := a.Snapshot()
			if _, err := a.TrainEpisode(context.Background(), [][]float64{{1}}, []float64{.5}); err == nil {
				t.Error("TrainEpisode accepted exhausted episode clock")
			}
			if !reflect.DeepEqual(before, a.Snapshot()) {
				t.Error("exhausted clock changed parameters, optimizer or persistent state")
			}
		})
	}
}
func TestConsolidateEpisodeClockOverflowIsAtomic(t *testing.T) {
	a := episodeClockIndividual(t, true, ^uint64(0))
	before := a.Snapshot()
	trigger := learning.ConsolidationTrigger{Receptor: 0, Threshold: 0, Rate: .5, Retain: .25}
	for attempt := 0; attempt < 2; attempt++ {
		report, err := a.Consolidate(context.Background(), trigger)
		if err == nil || report.Applied {
			t.Errorf("attempt %d accepted exhausted clock: %+v %v", attempt, report, err)
		}
	}
	if !reflect.DeepEqual(before, a.Snapshot()) {
		t.Error("exhausted clock changed memory or budget")
	}
}
func TestEpisodeClockBoundaryAndReset(t *testing.T) {
	a := episodeClockIndividual(t, false, ^uint64(0)-1)
	if _, err := a.TrainEpisode(context.Background(), [][]float64{{1}}, []float64{.5}); err != nil {
		t.Fatal(err)
	}
	if a.Snapshot().Optimizer.Episodes != ^uint64(0) {
		t.Fatal("final representable episode was lost")
	}
	if err := a.ResetOptimizer(context.Background(), learning.DefaultOptions()); err != nil {
		t.Fatal(err)
	}
	if _, err := a.TrainEpisode(context.Background(), [][]float64{{1}}, []float64{.5}); err != nil {
		t.Fatal(err)
	}
	if a.Snapshot().Optimizer.Episodes != 1 {
		t.Fatal("reset did not create fresh episode clock")
	}
}
func TestLegacyFutureConsolidationMarkerLoadsAndRebases(t *testing.T) {
	a := episodeClockIndividual(t, true, 0)
	trigger := learning.ConsolidationTrigger{Receptor: 0, Threshold: 0, Rate: .5, Retain: .25}
	if _, err := a.Consolidate(context.Background(), trigger); err != nil {
		t.Fatal(err)
	}
	s := a.Snapshot()
	s.Plastic.Slow.LastEpisode = 3 // old ResetOptimizer could retain this marker with Episodes=0
	a, err := learning.RestoreIndividual(s)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s, a.Snapshot()) {
		t.Fatal("restore rewrote a legacy model")
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := a.ResetOptimizer(context.Background(), learning.DefaultOptions()); err != nil {
			t.Fatal(err)
		}
	}
	if a.Snapshot().Plastic.Slow.LastEpisode != 1 {
		t.Error("legacy current write guard was not rebased")
	}
	if _, err := a.Consolidate(context.Background(), trigger); !errors.Is(err, learning.ErrAlreadyConsolidated) {
		t.Fatal("reset allowed the same experience to consume budget twice")
	}
	if _, err := a.TrainEpisode(context.Background(), [][]float64{{1}}, []float64{.5}); err != nil {
		t.Fatal(err)
	}
	if report, err := a.Consolidate(context.Background(), trigger); err != nil || !report.Applied || report.Used != 2 {
		t.Fatalf("new episode did not consolidate: %+v %v", report, err)
	}
	after := a.Snapshot()
	if after.Plastic.Slow.Budget != s.Plastic.Slow.Budget {
		t.Fatal("reset changed budget")
	}
}
