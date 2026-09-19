package learning_test

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

// mseUpstream builds the readout MSE VJP 2*(prediction-target)/N that
// LossGradient's internal path seeds the reverse pass with. Only the last row
// is non-zero, because the readout predicts the final step alone. The fixture
// uses N = 1, where the float32 division is exact and the bits agree with the
// model's own tape, so the rows below are not an approximation.
func mseUpstream(pred, target []float64, steps, width int) [][]float64 {
	out := make([][]float64, steps)
	for i := range out {
		out[i] = make([]float64, width)
	}
	last := out[len(out)-1]
	for j := range last {
		last[j] = float64(float32(2) * (float32(pred[j]) - float32(target[j])) / float32(width))
	}
	return out
}

func TestLossGradientFromMatchesLossGradient(t *testing.T) {
	n, p := network(t)
	input := [][]float64{{.7}, {-.2}, {.1}}
	target := []float64{.4}
	pred, err := n.Predict(context.Background(), p, input)
	if err != nil {
		t.Fatal(err)
	}
	upstream := mseUpstream(pred, target, len(input), 1)
	from, err := n.LossGradientFrom(context.Background(), p, input, upstream, 0)
	if err != nil {
		t.Fatal(err)
	}
	loss, direct, err := n.LossGradient(context.Background(), p, input, target, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !(loss > 0) {
		t.Fatalf("expected a positive loss, got %g", loss)
	}
	if !reflect.DeepEqual(from, direct) {
		t.Fatalf("LossGradientFrom diverged from LossGradient:\nfrom   %+v\ndirect %+v", from, direct)
	}
}

func TestLossGradientFromRejectsBadUpstream(t *testing.T) {
	n, p := network(t)
	input := [][]float64{{.7}, {-.2}, {.1}}
	pred, err := n.Predict(context.Background(), p, input)
	if err != nil {
		t.Fatal(err)
	}
	good := mseUpstream(pred, []float64{.4}, len(input), 1)
	for name, upstream := range map[string][][]float64{
		"row count":    {good[0]},
		"too few rows": {good[0], good[1]},
		"row width":    {good[0], {0, 0}, good[2]},
		"nan":          {good[0], good[1], {math.NaN()}},
		"infinite":     {good[0], good[1], {math.Inf(1)}},
	} {
		if _, err := n.LossGradientFrom(context.Background(), p, input, upstream, 0); err == nil {
			t.Fatalf("accepted %s upstream", name)
		}
	}
}

// TestStepFromMatchesStep pins that the caller-supplied upstream path commits
// exactly the parameters and optimizer state that a target-computed Step does.
func TestStepFromMatchesStep(t *testing.T) {
	c, p := continuousConfig(), continuousParameters()
	o := learning.DefaultOptions()
	step, err := learning.NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	from, err := learning.NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	input, target := maskInput()
	pred, err := step.Predict(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	upstream := mseUpstream(pred, target, len(input), c.OutputSize)

	stepResult, err := step.Step(context.Background(), input, target)
	if err != nil {
		t.Fatal(err)
	}
	fromResult, err := from.StepFrom(context.Background(), input, upstream)
	if err != nil {
		t.Fatal(err)
	}
	stepSnapshot, fromSnapshot := step.Snapshot(), from.Snapshot()
	if !reflect.DeepEqual(stepSnapshot.Parameters, fromSnapshot.Parameters) {
		t.Fatalf("StepFrom moved parameters differently:\nstep %+v\nfrom %+v", stepSnapshot.Parameters, fromSnapshot.Parameters)
	}
	if !reflect.DeepEqual(stepSnapshot.Optimizer, fromSnapshot.Optimizer) {
		t.Fatalf("StepFrom moved optimizer state differently:\nstep %+v\nfrom %+v", stepSnapshot.Optimizer, fromSnapshot.Optimizer)
	}
	if stepResult.LossKnown != true {
		t.Fatalf("Step did not mark the loss as known: %+v", stepResult)
	}
	if fromResult.LossKnown != false || fromResult.Loss != 0 {
		t.Fatalf("StepFrom reported loss as %g with LossKnown %v", fromResult.Loss, fromResult.LossKnown)
	}
	for name, pair := range map[string][2]float64{
		"gradient norm": {stepResult.GradientNorm, fromResult.GradientNorm},
		"update norm":   {stepResult.UpdateNorm, fromResult.UpdateNorm},
		"learning rate": {stepResult.LearningRate, fromResult.LearningRate},
	} {
		if pair[0] != pair[1] {
			t.Fatalf("%s diverged: %g != %g", name, pair[0], pair[1])
		}
	}
	if stepResult.Updates != fromResult.Updates || stepResult.Applied != fromResult.Applied {
		t.Fatalf("step and from diverged: %+v vs %+v", stepResult, fromResult)
	}
}

// TestStepFromRespectsMasksAndAccumulation pins the update path behind StepFrom:
// a masked edge never moves, and a two-gradient window applies on the second
// StepFrom call only.
func TestStepFromRespectsMasksAndAccumulation(t *testing.T) {
	c, p := continuousConfig(), continuousParameters()
	o := learning.DefaultOptions()
	o.AccumulateSteps = 2
	o.Masks = &learning.UpdateMasks{Edges: []bool{false, true}}
	tr, err := learning.NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	input, target := maskInput()
	pred, err := tr.Predict(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	upstream := mseUpstream(pred, target, len(input), c.OutputSize)

	first, err := tr.StepFrom(context.Background(), input, upstream)
	if err != nil {
		t.Fatal(err)
	}
	if first.Applied || first.Accumulated != 1 || first.Updates != 0 {
		t.Fatalf("the first StepFrom did not just fill the window: %+v", first)
	}
	second, err := tr.StepFrom(context.Background(), input, upstream)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Applied || second.Accumulated != 0 || second.Updates != 1 {
		t.Fatalf("the second StepFrom did not close the window: %+v", second)
	}
	s := tr.Snapshot()
	if s.Parameters.Core.Weights[0] != p.Core.Weights[0] {
		t.Fatalf("masked edge 0 moved: %.17g != %.17g", s.Parameters.Core.Weights[0], p.Core.Weights[0])
	}
}
