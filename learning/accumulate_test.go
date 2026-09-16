package learning_test

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

// accumulationBatches are the three fixed episodes the accumulation, clipping
// and loss-scaling tests drive through the two-node continuous fixture. They
// are ordinary episodes, so their gradients come from the real forward and
// backward pass rather than from injected numbers; parameters do not move
// during an accumulation window, so all three gradients are taken at the same
// initial parameters and the hand computations below are exact.
func accumulationBatches() ([][][]float64, [][]float64) {
	return [][][]float64{
			{{.7}, {-.2}, {.1}},
			{{.1}, {.5}, {-.3}},
			{{-.4}, {.2}, {.6}},
		}, [][]float64{
			{.4}, {-.2}, {.9},
		}
}

// The flat gradients of those batches at the fixture's initial parameters, in
// the AdamState order weights, bias, log_tau, theta_raw, encoder, readout (a
// continuous core has no theta_raw). Every test that uses them re-reads the
// gradient from the network and fails if these numbers are no longer what the
// model produces, so they can never drift into being a copy of the output.
var (
	accumulationGradient1 = []float64{
		-0.049219285556409649,
		0.00025227810424057455,
		-0.057322452996913582,
		-0.48634491125380319,
		0.0082881247222598101,
		-0.015322364640653214,
		-0.017595326527953148,
		-0.062781214714050293,
		0.034588254988193512,
	}
	accumulationSum12 = []float64{
		-0.031976093417745197,
		-2.5998813459017435e-05,
		-0.035996943422051078,
		-0.30459971432476396,
		0.0046557850440655975,
		-0.0089369673050338147,
		-0.011779957916587591,
		-0.056247394066303968,
		0.019059579819440842,
	}
	accumulationMean = []float64{
		-0.015501780644746763,
		0.00085021960331920128,
		-0.05405473625733398,
		-0.45369432804432802,
		0.0032237214411788783,
		-0.010068422137445212,
		0.001930061262100935,
		-0.1126475667891403,
		0.03708363945285479,
	}
)

// accumulationParameters is the fixture's initial flat parameter vector, in the
// same order.
var accumulationParameters = []float64{.3, -.2, .1, -.1, .1, .2, .5, .1, .8}

// accumulationOptions makes the update sensitive to the size of the gradient
// rather than only to its sign: AdamW's first update is
// lr * d / (|d| + epsilon), which with the default epsilon of 1e-8 is lr times
// the sign of d for any gradient this fixture produces. A large epsilon keeps
// the hand computations able to tell one gradient from another.
func accumulationOptions() learning.Options {
	o := learning.DefaultOptions()
	o.LearningRate = .1
	o.Epsilon = .1
	return o
}

// adamWFirstUpdate writes out AdamW's first update for every parameter. At step
// count 1 the two bias corrections cancel the (1-beta) factors exactly, so the
// first moment is d and the square root of the second is |d|.
func adamWFirstUpdate(parameters, d []float64, lr, epsilon float64) []float64 {
	out := make([]float64, len(parameters))
	for i := range parameters {
		out[i] = parameters[i] - lr*d[i]/(math.Abs(d[i])+epsilon)
	}
	return out
}

func flatTestParameters(p learning.Parameters) []float64 {
	out := []float64{}
	for _, v := range [][]float64{p.Core.Weights, p.Core.Bias, p.Core.LogTau, p.ThetaRaw, p.Encoder, p.Readout} {
		out = append(out, v...)
	}
	return out
}

func flatTestGradient(g learning.Gradient) []float64 {
	out := []float64{}
	for _, v := range [][]float64{g.Core.Weights, g.Core.Bias, g.Core.LogTau, g.ThetaRaw, g.Encoder, g.Readout} {
		out = append(out, v...)
	}
	return out
}

func euclideanNorm(v []float64) float64 {
	var n float64
	for _, x := range v {
		n = math.Hypot(n, x)
	}
	return n
}

// batchGradients returns the flat gradient of every accumulation batch at the
// fixture's initial parameters.
func batchGradients(t *testing.T) [][]float64 {
	t.Helper()
	n, err := learning.NewNetwork(continuousConfig())
	if err != nil {
		t.Fatal(err)
	}
	inputs, targets := accumulationBatches()
	out := make([][]float64, len(inputs))
	for i := range inputs {
		_, g, err := n.LossGradient(context.Background(), continuousParameters(), inputs[i], targets[i], 0)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = flatTestGradient(g)
	}
	return out
}

// TestAccumulatedWindowAppliesOneAdamWStepOnTheMeanGradient is the hand
// computation of root decision 4: three batches, one update, and that update is
// exactly what a single AdamW step on the mean of the three gradients produces.
func TestAccumulatedWindowAppliesOneAdamWStepOnTheMeanGradient(t *testing.T) {
	grads := batchGradients(t)
	requireSlicesClose(t, "gradient of batch 1", grads[0], accumulationGradient1, 1e-12)
	sum12 := make([]float64, len(grads[0]))
	mean := make([]float64, len(grads[0]))
	for i := range sum12 {
		sum12[i] = grads[0][i] + grads[1][i]
		mean[i] = (grads[0][i] + grads[1][i] + grads[2][i]) / 3
	}
	requireSlicesClose(t, "sum of batches 1 and 2", sum12, accumulationSum12, 1e-12)
	requireSlicesClose(t, "mean gradient", mean, accumulationMean, 1e-12)

	o := accumulationOptions()
	o.AccumulateSteps = 3
	tr, err := learning.NewTrainer(continuousConfig(), continuousParameters(), o)
	if err != nil {
		t.Fatal(err)
	}
	inputs, targets := accumulationBatches()
	partial := [][]float64{accumulationGradient1, accumulationSum12}
	for i := range inputs {
		result, err := tr.Step(context.Background(), inputs[i], targets[i])
		if err != nil {
			t.Fatalf("batch %d: %v", i+1, err)
		}
		s := tr.Snapshot()
		if i < len(partial) {
			if result.Applied {
				t.Fatalf("batch %d applied an update with only %d of 3 gradients", i+1, i+1)
			}
			if result.Updates != 0 || result.UpdateNorm != 0 || result.LearningRate != 0 || result.Projected != nil {
				t.Fatalf("batch %d reported an update: %+v", i+1, result)
			}
			if result.Accumulated != i+1 {
				t.Fatalf("batch %d reported %d accumulated gradients", i+1, result.Accumulated)
			}
			if s.Accumulator == nil || s.Accumulator.Count != i+1 {
				t.Fatalf("batch %d left accumulator %+v", i+1, s.Accumulator)
			}
			requireSlicesClose(t, "accumulator sum", s.Accumulator.Sum, partial[i], 1e-12)
			if !reflect.DeepEqual(flatTestParameters(s.Parameters), accumulationParameters) {
				t.Fatalf("batch %d moved parameters to %v", i+1, flatTestParameters(s.Parameters))
			}
			if s.Updates != 0 {
				t.Fatalf("batch %d raised the update count to %d", i+1, s.Updates)
			}
			for j, steps := range s.Optimizer.Steps {
				if steps != 0 || s.Optimizer.First[j] != 0 || s.Optimizer.Second[j] != 0 {
					t.Fatalf("batch %d moved optimizer state at parameter %d", i+1, j)
				}
			}
			continue
		}
		if !result.Applied || result.Updates != 1 || result.Accumulated != 0 {
			t.Fatalf("the full window did not apply an update: %+v", result)
		}
		if result.LearningRate != o.LearningRate {
			t.Fatalf("applied learning rate %g, want %g", result.LearningRate, o.LearningRate)
		}
		if s.Accumulator != nil {
			t.Fatalf("a completed window kept an accumulator: %+v", s.Accumulator)
		}
	}
	// One AdamW step on the mean gradient, written out. The mean norm is below
	// the clip norm of 1, so no clipping takes part in this expectation.
	if n := euclideanNorm(accumulationMean); n > o.ClipNorm {
		t.Fatalf("the mean gradient norm %g is above the clip norm", n)
	}
	want := adamWFirstUpdate(accumulationParameters, accumulationMean, o.LearningRate, o.Epsilon)
	requireSlicesClose(t, "parameters after one accumulated update", flatTestParameters(tr.Snapshot().Parameters), want, 1e-12)
	requireSlicesClose(t, "the same numbers as literals", want, []float64{
		0.31342124819047262,
		-0.20084305181155127,
		0.13508800675043228,
		-0.018060506480751623,
		0.096876956772948858,
		0.20914742116033291,
		0.4981064847423784,
		0.15297383294342642,
		0.77294816536760513,
	}, 1e-12)
}

// TestAccumulationSnapshotInTheMiddleOfAWindowResumesBitIdentically stops one
// gradient into a three-gradient window, restores from the snapshot and
// finishes the window in the restored trainer.
func TestAccumulationSnapshotInTheMiddleOfAWindowResumesBitIdentically(t *testing.T) {
	o := accumulationOptions()
	o.AccumulateSteps = 3
	inputs, targets := accumulationBatches()
	run := func(interrupt bool) learning.TrainingSnapshot {
		t.Helper()
		tr, err := learning.NewTrainer(continuousConfig(), continuousParameters(), o)
		if err != nil {
			t.Fatal(err)
		}
		for i := range inputs {
			if _, err := tr.Step(context.Background(), inputs[i], targets[i]); err != nil {
				t.Fatal(err)
			}
			if interrupt && i == 0 {
				middle := tr.Snapshot()
				if middle.Accumulator == nil || middle.Accumulator.Count != 1 {
					t.Fatalf("the interrupted snapshot holds %+v", middle.Accumulator)
				}
				// The snapshot must own its accumulator.
				middle.Accumulator.Sum[0] = 42
				if tr.Snapshot().Accumulator.Sum[0] == 42 {
					t.Fatal("the snapshot aliases the trainer's accumulator")
				}
				middle.Accumulator.Sum[0] = accumulationGradient1[0]
				if tr, err = learning.RestoreTrainer(middle); err != nil {
					t.Fatal(err)
				}
			}
		}
		return tr.Snapshot()
	}
	uninterrupted, resumed := run(false), run(true)
	if !reflect.DeepEqual(uninterrupted, resumed) {
		t.Fatalf("resuming mid-window diverged:\n%+v\n%+v", uninterrupted, resumed)
	}
	if uninterrupted.Updates != 1 || uninterrupted.Accumulator != nil {
		t.Fatalf("the window did not close: updates %d accumulator %+v", uninterrupted.Updates, uninterrupted.Accumulator)
	}
}

// TestAccumulateStepsAndAccumulatorValidation covers the option and the
// restored state: a negative window, a sum that does not cover the parameters,
// a count outside the window and an empty count carrying a nonzero sum.
func TestAccumulateStepsAndAccumulatorValidation(t *testing.T) {
	o := accumulationOptions()
	o.AccumulateSteps = -1
	if _, err := learning.NewTrainer(continuousConfig(), continuousParameters(), o); err == nil {
		t.Fatal("accepted a negative accumulation window")
	}
	o.AccumulateSteps = 3
	tr, err := learning.NewTrainer(continuousConfig(), continuousParameters(), o)
	if err != nil {
		t.Fatal(err)
	}
	inputs, targets := accumulationBatches()
	if _, err := tr.Step(context.Background(), inputs[0], targets[0]); err != nil {
		t.Fatal(err)
	}
	valid := tr.Snapshot()
	if _, err := learning.RestoreTrainer(valid); err != nil {
		t.Fatalf("a valid mid-window snapshot was rejected: %v", err)
	}
	for name, mutate := range map[string]func(s *learning.TrainingSnapshot){
		"short sum":            func(s *learning.TrainingSnapshot) { s.Accumulator.Sum = s.Accumulator.Sum[:2] },
		"count at the window":  func(s *learning.TrainingSnapshot) { s.Accumulator.Count = 3 },
		"count above window":   func(s *learning.TrainingSnapshot) { s.Accumulator.Count = 4 },
		"negative count":       func(s *learning.TrainingSnapshot) { s.Accumulator.Count = -1 },
		"empty nonzero sum":    func(s *learning.TrainingSnapshot) { s.Accumulator.Count = 0 },
		"non-finite sum entry": func(s *learning.TrainingSnapshot) { s.Accumulator.Sum[0] = math.Inf(1) },
		"accumulator without a window": func(s *learning.TrainingSnapshot) {
			s.Options.AccumulateSteps = 0
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := tr.Snapshot()
			mutate(&s)
			if _, err := learning.RestoreTrainer(s); err == nil {
				t.Fatalf("accepted %s", name)
			}
		})
	}
}
