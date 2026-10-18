package learning_test

import (
	"context"
	"encoding/json"
	"math"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

// classifierFixture is a six-node continuous core with two input nodes, three
// readout nodes and three classes, with seeded weights so the logits differ.
func classifierFixture() (learning.Config, learning.Parameters) {
	rng := rand.New(rand.NewPCG(7, 11))
	var sources, targets []int
	for s := 0; s < 6; s++ {
		for t := 2; t < 6; t++ {
			if (s+t)%3 != 0 {
				sources, targets = append(sources, s), append(targets, t)
			}
		}
	}
	fill := func(n int, scale float64) []float64 {
		v := make([]float64, n)
		for i := range v {
			v[i] = (2*rng.Float64() - 1) * scale
		}
		return v
	}
	c := learning.Config{
		Dynamics:     dynamics.Config{Nodes: 6, Sources: sources, Targets: targets, DT: 1, Activation: "tanh"},
		InputSize:    2,
		OutputSize:   3,
		InputNodes:   []int{0, 1},
		ReadoutNodes: []int{3, 4, 5},
	}
	logTau := make([]float64, 6)
	for i := range logTau {
		logTau[i] = math.Log(2)
	}
	p := learning.Parameters{
		Core:    dynamics.Parameters{Weights: fill(len(sources), .8), Bias: fill(6, .2), LogTau: logTau},
		Encoder: fill(2*2, 1),
		Readout: fill(3*3, 1),
	}
	return c, p
}

func classifierEpisodes(count int) ([][][]float64, [][]float64) {
	rng := rand.New(rand.NewPCG(3, 5))
	inputs := make([][][]float64, count)
	targets := make([][]float64, count)
	for i := range inputs {
		inputs[i] = make([][]float64, 4)
		for t := range inputs[i] {
			inputs[i][t] = []float64{2*rng.Float64() - 1, 2*rng.Float64() - 1}
		}
		targets[i] = make([]float64, 3)
		targets[i][i%3] = 1
	}
	return inputs, targets
}

// TestCrossEntropyGradientMatchesSoftmaxUpstream checks the loss value
// against log-sum-exp written out in float64 and the gradient against the
// same network driven by the softmax(logits) - onehot upstream.
func TestCrossEntropyGradientMatchesSoftmaxUpstream(t *testing.T) {
	ctx := context.Background()
	c, p := classifierFixture()
	n, err := learning.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	inputs, targets := classifierEpisodes(3)
	for i := range inputs {
		logits, err := n.Predict(ctx, p, inputs[i])
		if err != nil {
			t.Fatal(err)
		}
		label := i % 3
		maxLogit := math.Max(logits[0], math.Max(logits[1], logits[2]))
		var sum float64
		for _, z := range logits {
			sum += math.Exp(z - maxLogit)
		}
		wantLoss := -(logits[label] - maxLogit) + math.Log(sum)
		upstream := make([][]float64, len(inputs[i]))
		for r := range upstream {
			upstream[r] = make([]float64, 3)
		}
		for k, z := range logits {
			upstream[len(upstream)-1][k] = math.Exp(z-maxLogit) / sum
		}
		upstream[len(upstream)-1][label]--
		loss, got, err := n.LossGradientCrossEntropy(ctx, p, inputs[i], targets[i], 0, 0)
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

func TestCrossEntropyRejectsTargetsThatAreNotOneHot(t *testing.T) {
	c, p := classifierFixture()
	n, err := learning.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	inputs, _ := classifierEpisodes(1)
	for _, target := range [][]float64{{0, 0, 0}, {1, 1, 0}, {.5, .5, 0}} {
		if _, _, err := n.LossGradientCrossEntropy(context.Background(), p, inputs[0], target, 0, 0); err == nil {
			t.Fatalf("target %v accepted", target)
		}
	}
}

func TestLossOptionValidationAndJSON(t *testing.T) {
	c, p := classifierFixture()
	o := learning.DefaultOptions()
	o.Loss = "hinge"
	if _, err := learning.NewTrainer(c, p, o); err == nil || !strings.Contains(err.Error(), "unsupported loss") {
		t.Fatalf("unknown loss: %v", err)
	}
	b, err := json.Marshal(learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "loss\"") {
		t.Fatalf("default options mention a loss: %s", b)
	}
}

// TestStepBatchMatchesSequentialSteps drives ten pairs through windows of
// four starting from a partly filled window, so the batch crosses update
// boundaries, and requires the exact trainer state of ten Step calls.
func TestStepBatchMatchesSequentialSteps(t *testing.T) {
	ctx := context.Background()
	c, p := classifierFixture()
	o := learning.DefaultOptions()
	o.LearningRate = .05
	o.AccumulateSteps = 4
	o.Loss = learning.LossSoftmaxCrossEntropy
	inputs, targets := classifierEpisodes(11)
	sequential, err := learning.NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	batched, err := learning.NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	var want []learning.StepResult
	for i := range inputs {
		r, err := sequential.Step(ctx, inputs[i], targets[i])
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, r)
	}
	first, err := batched.Step(ctx, inputs[0], targets[0])
	if err != nil {
		t.Fatal(err)
	}
	got, err := batched.StepBatch(ctx, inputs[1:], targets[1:], 3)
	if err != nil {
		t.Fatal(err)
	}
	got = append([]learning.StepResult{first}, got...)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("results differ:\n got %+v\nwant %+v", got, want)
	}
	if applied := want[3].Applied && want[7].Applied && !want[10].Applied; !applied {
		t.Fatalf("fixture does not cross window boundaries: %+v", want)
	}
	if !reflect.DeepEqual(batched.Snapshot(), sequential.Snapshot()) {
		t.Fatal("trainer snapshots differ")
	}
}

func TestStepBatchRejectsMismatchedPairs(t *testing.T) {
	c, p := classifierFixture()
	tr, err := learning.NewTrainer(c, p, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	inputs, targets := classifierEpisodes(2)
	if _, err := tr.StepBatch(context.Background(), inputs, targets[:1], 2); err == nil {
		t.Fatal("mismatched pairs accepted")
	}
	inputs[1] = inputs[1][:0]
	results, err := tr.StepBatch(context.Background(), inputs, targets, 2)
	if err == nil || !strings.Contains(err.Error(), "pair 1") || len(results) != 1 {
		t.Fatalf("failing pair: %v, %d results", err, len(results))
	}
}

// gatedClassifierFixture is classifierFixture with the current gate.
func gatedClassifierFixture() (learning.Config, learning.Parameters) {
	c, p := classifierFixture()
	c.Dynamics.Gate = dynamics.GateCurrent
	p.Core.GateGain = []float64{.3, -.2, .5, .1, -.4, .2}
	p.Core.GateBias = []float64{0, .1, -.1, .2, 0, -.3}
	return c, p
}

// TestGatedTrainerUpdatesGateOnlyWhenTrainable trains the gated fixture with
// the gate frozen and then trainable, round-trips the snapshot through JSON,
// and checks the shape validation that guards the gate groups.
func TestGatedTrainerUpdatesGateOnlyWhenTrainable(t *testing.T) {
	ctx := context.Background()
	c, p := gatedClassifierFixture()
	inputs, targets := classifierEpisodes(4)
	for _, trainGate := range []bool{false, true} {
		o := learning.DefaultOptions()
		o.Loss = learning.LossSoftmaxCrossEntropy
		o.Trainable.Gate = trainGate
		tr, err := learning.NewTrainer(c, p, o)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tr.StepBatch(ctx, inputs, targets, 2); err != nil {
			t.Fatal(err)
		}
		s := tr.Snapshot()
		moved := !reflect.DeepEqual(s.Parameters.Core.GateGain, p.Core.GateGain) && !reflect.DeepEqual(s.Parameters.Core.GateBias, p.Core.GateBias)
		if moved != trainGate {
			t.Fatalf("trainable gate %v: gate parameters moved %v", trainGate, moved)
		}
		if reflect.DeepEqual(s.Parameters.Core.Weights, p.Core.Weights) {
			t.Fatal("weights did not train")
		}
		b, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		var back learning.TrainingSnapshot
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatal(err)
		}
		restored, err := learning.RestoreTrainer(back)
		if err != nil {
			t.Fatal(err)
		}
		want, err := tr.Predict(ctx, inputs[0])
		if err != nil {
			t.Fatal(err)
		}
		got, err := restored.Predict(ctx, inputs[0])
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatal("restored gated trainer predicts differently")
		}
	}
	o := learning.DefaultOptions()
	missing := p
	missing.Core.GateGain, missing.Core.GateBias = nil, nil
	if _, err := learning.NewTrainer(c, missing, o); err == nil {
		t.Fatal("gated core accepted without gate parameters")
	}
	plain, plainParams := classifierFixture()
	o.Trainable.Gate = true
	if _, err := learning.NewTrainer(plain, plainParams, o); err == nil {
		t.Fatal("trainable gate accepted on an ungated core")
	}
	o.Trainable.Gate = false
	o.Recompute = &learning.Recompute{SegmentSteps: 2}
	tr, err := learning.NewTrainer(c, p, o)
	if err == nil {
		_, err = tr.Step(ctx, inputs[0], targets[0])
	}
	if err == nil {
		t.Fatal("recompute ran a gated core")
	}
}
