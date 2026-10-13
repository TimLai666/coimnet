package learning_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func consolidationReportTrigger(threshold float64) learning.ConsolidationTrigger {
	return learning.ConsolidationTrigger{
		Receptor:  0,
		Threshold: threshold,
		Rate:      0.5,
		Retain:    0.25,
	}
}

func assertConsolidationReportApplied(t *testing.T, report learning.ConsolidationReport) {
	t.Helper()
	if !report.Applied || report.GateClosed || report.Already || report.BudgetExhausted {
		t.Fatalf("successful consolidation report = %+v", report)
	}
	if report.Episode != 0 || report.LastEpisode != 1 || report.Used != 1 {
		t.Fatalf("successful consolidation counters = {episode %d last %d used %d}, want {0 1 1}", report.Episode, report.LastEpisode, report.Used)
	}
	if !closeEnough(report.Occupancy, chemOcc1) {
		t.Fatalf("successful consolidation occupancy = %v, want %v", report.Occupancy, chemOcc1)
	}
}

func assertConsolidationRefusal(t *testing.T, report learning.ConsolidationReport, err, want error, gate, already, budget bool, episode, last, used uint64) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("Consolidate() error = %v, want %v", err, want)
	}
	if report.Applied || report.GateClosed != gate || report.Already != already || report.BudgetExhausted != budget {
		t.Fatalf("refused consolidation report = %+v, want applied=false flags={gate:%t already:%t budget:%t}", report, gate, already, budget)
	}
	if report.Episode != episode || report.LastEpisode != last || report.Used != used {
		t.Fatalf("refused consolidation counters = {episode %d last %d used %d}, want {%d %d %d}", report.Episode, report.LastEpisode, report.Used, episode, last, used)
	}
	if !closeEnough(report.Occupancy, chemOcc1) {
		t.Fatalf("refused consolidation occupancy = %v, want %v", report.Occupancy, chemOcc1)
	}
}

func TestConsolidationRefusalCounters(t *testing.T) {
	t.Run("gate closed", func(t *testing.T) {
		a, _ := consolidationIndividual(t, 2)
		if report, err := a.Consolidate(context.Background(), consolidationReportTrigger(0.5)); err != nil {
			t.Fatalf("initial Consolidate() error = %v", err)
		} else {
			assertConsolidationReportApplied(t, report)
		}
		before := a.Snapshot()
		report, err := a.Consolidate(context.Background(), consolidationReportTrigger(1))
		assertConsolidationRefusal(t, report, err, learning.ErrConsolidationGateClosed, true, false, false, 0, 1, 1)
		if after := a.Snapshot(); !reflect.DeepEqual(after, before) {
			t.Fatalf("gate refusal changed the snapshot: before=%+v after=%+v", before, after)
		}
	})

	t.Run("already consolidated", func(t *testing.T) {
		a, _ := consolidationIndividual(t, 2)
		if report, err := a.Consolidate(context.Background(), consolidationReportTrigger(0.5)); err != nil {
			t.Fatalf("initial Consolidate() error = %v", err)
		} else {
			assertConsolidationReportApplied(t, report)
		}
		before := a.Snapshot()
		report, err := a.Consolidate(context.Background(), consolidationReportTrigger(0.5))
		assertConsolidationRefusal(t, report, err, learning.ErrAlreadyConsolidated, false, true, false, 0, 1, 1)
		if after := a.Snapshot(); !reflect.DeepEqual(after, before) {
			t.Fatalf("already refusal changed the snapshot: before=%+v after=%+v", before, after)
		}
	})

	t.Run("budget exhausted", func(t *testing.T) {
		a, _ := consolidationIndividual(t, 1)
		if report, err := a.Consolidate(context.Background(), consolidationReportTrigger(0.5)); err != nil {
			t.Fatalf("initial Consolidate() error = %v", err)
		} else {
			assertConsolidationReportApplied(t, report)
		}
		if _, err := a.TrainEpisode(context.Background(), [][]float64{{1}}, []float64{0.5}); err != nil {
			t.Fatalf("TrainEpisode() error = %v", err)
		}
		before := a.Snapshot()
		report, err := a.Consolidate(context.Background(), consolidationReportTrigger(0.5))
		assertConsolidationRefusal(t, report, err, learning.ErrBudgetExhausted, false, false, true, 1, 1, 1)
		if after := a.Snapshot(); !reflect.DeepEqual(after, before) {
			t.Fatalf("budget refusal changed the snapshot: before=%+v after=%+v", before, after)
		}
	})
}
