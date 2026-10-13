package learning_test

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func restoreConsolidationWithValues(t *testing.T, slow, plastic, plasticMax float64) *learning.Individual {
	t.Helper()
	a, _ := consolidationIndividual(t, 1)
	snapshot := a.Snapshot()
	snapshot.Plastic.Config.Rule.PlasticMax = plasticMax
	snapshot.Plastic.Slow.Values[0] = slow
	snapshot.Plastic.State.Plastic[0] = plastic
	restored, err := learning.RestoreIndividual(snapshot)
	if err != nil {
		t.Fatalf("RestoreIndividual() error = %v", err)
	}
	return restored
}

func TestConsolidateRejectsOverflowingSlowSumWithoutMutation(t *testing.T) {
	max := math.MaxFloat64
	a := restoreConsolidationWithValues(t, max, max, max)
	before := a.Snapshot()

	report, err := a.Consolidate(context.Background(), learning.ConsolidationTrigger{
		Receptor: 0, Threshold: 0, Rate: 1, Retain: .5,
	})
	if err == nil || report.Applied {
		t.Fatalf("Consolidate() report = %+v, error = %v; want an overflow refusal", report, err)
	}
	if after := a.Snapshot(); !reflect.DeepEqual(after, before) {
		t.Fatalf("overflowing consolidation mutated the complete snapshot:\nbefore=%+v\nafter=%+v", before, after)
	}
}

func TestConsolidateReportsFiniteRepresentableLargeNorm(t *testing.T) {
	const large = 1e200
	a := restoreConsolidationWithValues(t, 0, large, large)

	report, err := a.Consolidate(context.Background(), learning.ConsolidationTrigger{
		Receptor: 0, Threshold: 0, Rate: 1, Retain: .5,
	})
	if err != nil {
		t.Fatalf("Consolidate() error = %v", err)
	}
	if !report.Applied {
		t.Fatalf("report = %+v, want an applied consolidation", report)
	}
	for name, values := range map[string][2]float64{
		"slow L2":    {report.SlowL2, large},
		"plastic L2": {report.PlasticL2, large * .5},
	} {
		got, want := values[0], values[1]
		if math.IsNaN(got) || math.IsInf(got, 0) {
			t.Errorf("%s = %v, want finite %v", name, got, want)
			continue
		}
		if relative := math.Abs(got-want) / want; relative > 1e-12 {
			t.Errorf("%s = %v, want %v (relative error %g)", name, got, want, relative)
		}
	}
}

func twoEdgeConsolidationIndividual(t *testing.T) *learning.Individual {
	t.Helper()
	config := consolidationConfig()
	config.Dynamics.Sources = []int{0, 1}
	config.Dynamics.Targets = []int{1, 0}
	config.Dynamics.Delays = []int{0, 0}
	parameters := consolidationParameters()
	parameters.Core.Weights = []float64{.9, .9}
	a := newIndividual(t, config, parameters)
	if err := a.EnablePlasticity(plasticConfig(0, 1)); err != nil {
		t.Fatal(err)
	}
	enableChemistry(t, a, consolidationChemistry())
	if err := a.EnableConsolidation(1); err != nil {
		t.Fatal(err)
	}
	advanceGated(t, a, [][]float64{{1}, {1}}, []float64{1, 1})
	return a
}

func TestConsolidateRejectsUnrepresentableNormWithoutMutation(t *testing.T) {
	max := math.MaxFloat64
	a := twoEdgeConsolidationIndividual(t)
	snapshot := a.Snapshot()
	snapshot.Plastic.Slow.Values[0] = max
	snapshot.Plastic.Slow.Values[1] = max
	snapshot.Plastic.State.Plastic[0] = 0
	snapshot.Plastic.State.Plastic[1] = 0
	restored, err := learning.RestoreIndividual(snapshot)
	if err != nil {
		t.Fatalf("RestoreIndividual() error = %v", err)
	}
	before := restored.Snapshot()

	report, err := restored.Consolidate(context.Background(), learning.ConsolidationTrigger{
		Receptor: 0, Threshold: 0, Rate: 1, Retain: 1,
	})
	if err == nil || report.Applied {
		t.Fatalf("Consolidate() report = %+v, error = %v; want an unrepresentable-norm refusal", report, err)
	}
	if after := restored.Snapshot(); !reflect.DeepEqual(after, before) {
		t.Fatalf("unrepresentable norm mutated the complete snapshot:\nbefore=%+v\nafter=%+v", before, after)
	}
}
