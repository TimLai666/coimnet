package learning_test

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

// runScaledTrainer drives the three accumulation batches twice through a
// trainer that differs from the baseline only in its loss scale.
func runScaledTrainer(t *testing.T, scale float64) learning.TrainingSnapshot {
	t.Helper()
	o := accumulationOptions()
	o.LossScale = scale
	tr, err := learning.NewTrainer(continuousConfig(), continuousParameters(), o)
	if err != nil {
		t.Fatal(err)
	}
	inputs, targets := accumulationBatches()
	for range 2 {
		for i := range inputs {
			if _, err := tr.Step(context.Background(), inputs[i], targets[i]); err != nil {
				t.Fatalf("loss scale %g: %v", scale, err)
			}
		}
	}
	return tr.Snapshot()
}

// TestLossScaleLeavesTheUpdateUnchanged pins the first half of root decision 4:
// the gradient is multiplied by the scale and divided back before clipping, so
// the parameters a scaled run reaches are the parameters the unscaled run
// reaches. A power of two is exact in binary floating point and must round trip
// bit for bit; a scale that is not a power of two only has to stay inside 1e-12.
func TestLossScaleLeavesTheUpdateUnchanged(t *testing.T) {
	baseline := runScaledTrainer(t, 0)
	if baseline.Options.LossScale != 0 {
		t.Fatal("the baseline declared a loss scale")
	}
	exact := runScaledTrainer(t, 1024)
	if !reflect.DeepEqual(exact.Parameters, baseline.Parameters) || !reflect.DeepEqual(exact.Optimizer, baseline.Optimizer) {
		t.Fatalf("a scale of 1024 changed the run:\n%+v\n%+v", exact.Parameters, baseline.Parameters)
	}
	for _, scale := range []float64{1e3, 1e6, 65536, 1e-3} {
		got := runScaledTrainer(t, scale)
		requireSlicesClose(t, "parameters at loss scale", flatTestParameters(got.Parameters), flatTestParameters(baseline.Parameters), 1e-12)
		requireSlicesClose(t, "first moments at loss scale", got.Optimizer.First, baseline.Optimizer.First, 1e-12)
		requireSlicesClose(t, "second moments at loss scale", got.Optimizer.Second, baseline.Optimizer.Second, 1e-12)
	}
	if baseline.Options.LossScale != 0 || exact.Options.LossScale != 1024 {
		t.Fatal("the loss scale did not travel with the snapshot")
	}
}

// overflowEpisode is one episode whose gradient is large enough that a scale of
// 1e300 leaves the representable range of a float64.
func overflowEpisode() ([][]float64, []float64) {
	return [][]float64{{.7}, {-.2}, {.1}}, []float64{1e9}
}

// TestNonFiniteScaledGradientRejectsTheWholeStep proves the rejection is the
// scaling, not the episode: the same episode at scale 1 updates normally, and
// the rejected step leaves parameters, moments, step counts, the update count
// and the accumulator bit-identical.
func TestNonFiniteScaledGradientRejectsTheWholeStep(t *testing.T) {
	input, target := overflowEpisode()
	unscaled, err := learning.NewTrainer(continuousConfig(), continuousParameters(), accumulationOptions())
	if err != nil {
		t.Fatal(err)
	}
	result, err := unscaled.Step(context.Background(), input, target)
	if err != nil {
		t.Fatalf("the unscaled episode was rejected: %v", err)
	}
	if !result.Applied || !(result.GradientNorm > 1e9) {
		t.Fatalf("the unscaled episode did not produce a large gradient: %+v", result)
	}

	o := accumulationOptions()
	o.LossScale = 1e300
	scaled, err := learning.NewTrainer(continuousConfig(), continuousParameters(), o)
	if err != nil {
		t.Fatal(err)
	}
	before := scaled.Snapshot()
	if _, err := scaled.Step(context.Background(), input, target); err == nil {
		t.Fatal("accepted a gradient that overflowed under the loss scale")
	}
	if !reflect.DeepEqual(before, scaled.Snapshot()) {
		t.Fatal("the rejected step left state behind")
	}

	// The same rejection in the middle of an accumulation window must not
	// disturb the gradients already accumulated.
	o.AccumulateSteps = 3
	window, err := learning.NewTrainer(continuousConfig(), continuousParameters(), o)
	if err != nil {
		t.Fatal(err)
	}
	inputs, targets := accumulationBatches()
	if _, err := window.Step(context.Background(), inputs[0], targets[0]); err != nil {
		t.Fatal(err)
	}
	half := window.Snapshot()
	if half.Accumulator == nil || half.Accumulator.Count != 1 {
		t.Fatalf("the first batch did not open a window: %+v", half.Accumulator)
	}
	if _, err := window.Step(context.Background(), input, target); err == nil {
		t.Fatal("accepted an overflowing gradient in the middle of a window")
	}
	if !reflect.DeepEqual(half, window.Snapshot()) {
		t.Fatalf("the rejected step changed the accumulator:\n%+v\n%+v", half.Accumulator, window.Snapshot().Accumulator)
	}
}

// TestLossScaleValidation rejects a scale that cannot describe a multiplier.
func TestLossScaleValidation(t *testing.T) {
	for name, scale := range map[string]float64{
		"negative":  -1,
		"not a num": math.NaN(),
		"infinite":  math.Inf(1),
	} {
		t.Run(name, func(t *testing.T) {
			o := accumulationOptions()
			o.LossScale = scale
			if _, err := learning.NewTrainer(continuousConfig(), continuousParameters(), o); err == nil {
				t.Fatalf("accepted loss scale %g", scale)
			}
		})
	}
}
