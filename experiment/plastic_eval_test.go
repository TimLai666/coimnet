package experiment

import (
	"context"
	"math"
	"reflect"
	"testing"
)

// TestScoreTwinFreezesPlasticTwins pins that an evaluation of an individual
// with local plasticity is never a training step: the twin is frozen, so its
// eligibility cannot accumulate across the evaluation episodes, the
// "evaluation changed the individual" invariant holds without dropping the
// layer, and the individual the twin was restored from is untouched.
func TestScoreTwinFreezesPlasticTwins(t *testing.T) {
	ctx := context.Background()
	ind, err := bioInspiredIndividual(1, 1)
	if err != nil {
		t.Fatalf("bioInspiredIndividual: %v", err)
	}
	if err := ind.EnableConsolidation(3); err != nil {
		t.Fatalf("EnableConsolidation: %v", err)
	}
	spec := TaskSpec{Name: "a", Generator: GeneratorDelayedCorrelation, Params: map[string]float64{"delay": 2, "channel": 0}}
	if err := trainEcdysoneSeed(ctx, ind, spec, 2, 1, 3); err != nil {
		t.Fatalf("trainEcdysoneSeed: %v", err)
	}
	before := ind.Snapshot()
	if before.Plastic == nil {
		t.Fatal("the trained individual carries no plastic part")
	}
	score, err := evaluateTask(ctx, ind, spec, 2, false, 8, evalSeed(1, 0), true)
	if err != nil {
		t.Fatalf("evaluateTask on a plastic individual: %v", err)
	}
	if math.IsNaN(score) || math.IsInf(score, 0) {
		t.Fatalf("score = %g, want a finite value", score)
	}
	t.Logf("retest score of the frozen plastic twin = %g", score)
	if after := ind.Snapshot(); !reflect.DeepEqual(after, before) {
		t.Fatal("the evaluation changed the individual it scored")
	}
}
