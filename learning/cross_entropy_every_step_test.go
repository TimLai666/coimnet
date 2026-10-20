package learning_test

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

// everyStepTarget is a steps x 3 target whose row r names class labels[r],
// or is all zeros where labels[r] is negative.
func everyStepTarget(labels []int) []float64 {
	target := make([]float64, 3*len(labels))
	for r, label := range labels {
		if label >= 0 {
			target[3*r+label] = 1
		}
	}
	return target
}

// TestCrossEntropyEveryStepMatchesSoftmaxUpstream checks the per-step
// objective against log-sum-exp written out in float64 for every scored row,
// averaged over the scored rows, and the gradient against the same network
// driven by (softmax(logits) - onehot) / scored on those rows.
func TestCrossEntropyEveryStepMatchesSoftmaxUpstream(t *testing.T) {
	ctx := context.Background()
	c, p := classifierFixture()
	c.ReadoutEveryStep = true
	n, err := learning.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	inputs, _ := classifierEpisodes(2)
	for i, labels := range [][]int{{-1, 2, 0, 1}, {1, -1, -1, 2}} {
		logits, err := n.PredictAll(ctx, p, inputs[i])
		if err != nil {
			t.Fatal(err)
		}
		scored := 0
		for _, label := range labels {
			if label >= 0 {
				scored++
			}
		}
		var wantLoss float64
		upstream := make([][]float64, len(inputs[i]))
		for r, row := range logits {
			upstream[r] = make([]float64, 3)
			if labels[r] < 0 {
				continue
			}
			maxLogit := math.Max(row[0], math.Max(row[1], row[2]))
			var sum float64
			for _, z := range row {
				sum += math.Exp(z - maxLogit)
			}
			wantLoss += (-(row[labels[r]] - maxLogit) + math.Log(sum)) / float64(scored)
			for k, z := range row {
				upstream[r][k] = math.Exp(z-maxLogit) / sum / float64(scored)
			}
			upstream[r][labels[r]] -= 1 / float64(scored)
		}
		loss, got, err := n.LossGradientCrossEntropyEveryStep(ctx, p, inputs[i], everyStepTarget(labels), 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(loss-wantLoss) > 1e-6 {
			t.Fatalf("episode %d loss %v, want %v", i, loss, wantLoss)
		}
		want, err := n.LossGradientFrom(ctx, p, inputs[i], upstream, 0)
		if err != nil {
			t.Fatal(err)
		}
		g, w := flatTestGradient(got), flatTestGradient(want)
		for k := range g {
			if math.Abs(g[k]-w[k]) > 1e-5*(1+math.Abs(w[k])) {
				t.Fatalf("episode %d gradient[%d] = %v, want %v", i, k, g[k], w[k])
			}
		}
	}
}

func TestCrossEntropyEveryStepRejectsBadTargets(t *testing.T) {
	ctx := context.Background()
	c, p := classifierFixture()
	c.ReadoutEveryStep = true
	n, err := learning.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	inputs, _ := classifierEpisodes(1)
	bad := map[string][]float64{
		"no scored row":   everyStepTarget([]int{-1, -1, -1, -1}),
		"two ones in row": {1, 1, 0, 0, 1, 0, 0, 0, 1, 1, 0, 0},
		"fractional":      {.5, .5, 0, 0, 1, 0, 0, 0, 1, 1, 0, 0},
		"one row only":    {1, 0, 0},
	}
	for name, target := range bad {
		if _, _, err := n.LossGradientCrossEntropyEveryStep(ctx, p, inputs[0], target, 0, 0); err == nil {
			t.Fatalf("%s: target %v accepted", name, target)
		}
	}
	c.ReadoutEveryStep = false
	last, err := learning.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := last.LossGradientCrossEntropyEveryStep(ctx, p, inputs[0], everyStepTarget([]int{0, 1, 2, 0}), 0, 0); err == nil {
		t.Fatal("a last-step readout accepted the per-step objective")
	}
	o := learning.DefaultOptions()
	o.Loss = learning.LossSoftmaxCrossEntropyEveryStep
	if _, err := learning.NewTrainer(c, p, o); err == nil || !strings.Contains(err.Error(), "readout_every_step") {
		t.Fatalf("trainer without per-step readout: %v", err)
	}
}

// TestStepCrossEntropyEveryStepMatchesNetworkGradient pins that Trainer.Step
// with the per-step loss reports the network's loss and moves the parameters.
func TestStepCrossEntropyEveryStepMatchesNetworkGradient(t *testing.T) {
	ctx := context.Background()
	c, p := classifierFixture()
	c.ReadoutEveryStep = true
	o := learning.DefaultOptions()
	o.Loss = learning.LossSoftmaxCrossEntropyEveryStep
	tr, err := learning.NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	n, err := learning.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	inputs, _ := classifierEpisodes(1)
	target := everyStepTarget([]int{-1, 2, 0, 1})
	want, _, err := n.LossGradientCrossEntropyEveryStep(ctx, p, inputs[0], target, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	r, err := tr.Step(ctx, inputs[0], target)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(r.Loss-want) > 1e-12 {
		t.Fatalf("step loss %v, want %v", r.Loss, want)
	}
}
