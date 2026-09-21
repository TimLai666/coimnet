package learning_test

import (
	"context"
	"math"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

// runAccumulatedWindow drives one full accumulation window of the three
// accumulation batches and returns the parameters the single update produced.
func runAccumulatedWindow(t *testing.T, clipNorm float64) []float64 {
	t.Helper()
	o := accumulationOptions()
	o.ClipNorm = clipNorm
	o.AccumulateSteps = 3
	tr, err := learning.NewTrainer(continuousConfig(), continuousParameters(), o)
	if err != nil {
		t.Fatal(err)
	}
	inputs, targets := accumulationBatches()
	for i := range inputs {
		result, err := tr.Step(context.Background(), inputs[i], targets[i])
		if err != nil {
			t.Fatalf("batch %d: %v", i+1, err)
		}
		if applied := i == len(inputs)-1; result.Applied != applied {
			t.Fatalf("batch %d reported applied=%v", i+1, result.Applied)
		}
	}
	if s := tr.Snapshot(); s.Updates != 1 {
		t.Fatalf("the window produced %d updates", s.Updates)
	}
	return flatTestParameters(tr.Snapshot().Parameters)
}

// TestClippingRunsAfterTheAccumulationAverageAndBeforeAdamW pins the order of
// root decision 4. The three batch gradients have norms 0.4980, 0.1848 and
// 1.1050 while their mean has norm 0.4724, so a clip norm of 1 clips the third
// batch on its own but leaves the mean untouched: clipping before the average
// would shrink that batch and move the update. A clip norm of 0.2 then clips
// the mean itself, and the update has to be AdamW's first step on the clipped
// mean rather than on the raw one.
//
// The ticket's other construction, a mean above the clip norm with every batch
// below it, cannot exist: the norm of a mean is never larger than the largest
// norm it averages, so only this direction can separate the two orders.
func TestClippingRunsAfterTheAccumulationAverageAndBeforeAdamW(t *testing.T) {
	grads := batchGradients(t)
	norms := make([]float64, len(grads))
	for i := range grads {
		norms[i] = euclideanNorm(grads[i])
	}
	mean := make([]float64, len(grads[0]))
	for _, gradient := range grads {
		for i, value := range gradient {
			mean[i] += value
		}
	}
	for i := range mean {
		mean[i] /= float64(len(grads))
	}
	meanNorm := euclideanNorm(mean)
	if !(norms[2] > 1 && meanNorm < 1) {
		t.Fatal("the fixture no longer separates a clipped batch from an unclipped mean")
	}

	o := accumulationOptions()
	// Clip norm 1: the mean passes through unchanged.
	unclipped := adamWFirstUpdate(accumulationParameters, mean, o.LearningRate, o.Epsilon)
	requireSlicesClose(t, "update with an unclipped mean", runAccumulatedWindow(t, 1), unclipped, 1e-12)
	requireSlicesClose(t, "the same numbers as literals", unclipped, []float64{
		0.31342124819047262,
		-0.20084305181155127,
		0.13508800675043228,
		-0.018060506480751623,
		0.096876956772948858,
		0.20914742116033291,
		0.4981064847423784,
		0.15297383294342642,
		0.77294816536760513,
	}, gradientReferenceTolerance)

	// What clipping each batch before the average would have produced. It is a
	// different answer, so the test above can only pass in the declared order.
	perBatch := make([]float64, len(accumulationMean))
	for _, g := range grads {
		scale := 1.0
		if n := euclideanNorm(g); n > 1 {
			scale = 1 / n
		}
		for j := range perBatch {
			perBatch[j] += g[j] * scale / 3
		}
	}
	wrong := adamWFirstUpdate(accumulationParameters, perBatch, o.LearningRate, o.Epsilon)
	separation := 0.0
	for i := range wrong {
		separation = math.Max(separation, math.Abs(wrong[i]-unclipped[i]))
	}
	if separation < 1e-6 {
		t.Fatalf("clipping before the average is only %g away; the test cannot tell the two orders apart", separation)
	}

	// Clip norm 0.2: the mean itself is clipped, before AdamW consumes it.
	const clip = .2
	scaled := make([]float64, len(mean))
	for i := range scaled {
		scaled[i] = mean[i] * (clip / meanNorm)
	}
	clipped := adamWFirstUpdate(accumulationParameters, scaled, o.LearningRate, o.Epsilon)
	requireSlicesClose(t, "update with a clipped mean", runAccumulatedWindow(t, clip), clipped, 1e-12)
	requireSlicesClose(t, "the same numbers as literals", clipped, []float64{
		0.3061585311248784,
		-0.20035865052532303,
		0.11862253035545056,
		-0.034238130590445115,
		0.098653608872326312,
		0.20408821877930369,
		0.49918952863175292,
		0.13229036954243426,
		0.7864308703323335,
	}, gradientReferenceTolerance)
}
