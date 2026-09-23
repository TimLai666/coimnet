package learning_test

import (
	"context"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

// vectorScalarConfig declares the same two-node chain as the scalar continuous
// fixture with StateDimension set explicitly (0 keeps the legacy field absent).
func vectorScalarConfig(stateDimension int) learning.Config {
	return learning.Config{
		Dynamics: dynamics.Config{
			Nodes:          2,
			Sources:        []int{0},
			Targets:        []int{1},
			Delays:         []int{0},
			DT:             .5,
			Activation:     "tanh",
			StateDimension: stateDimension,
		},
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: []int{1},
	}
}

func scalarParameters() learning.Parameters {
	return learning.Parameters{
		Core: dynamics.Parameters{
			Weights: []float64{.3},
			Bias:    []float64{.1, -.1},
			LogTau:  []float64{.2, -.05},
		},
		Encoder: []float64{.7, -.2},
		Readout: []float64{.8},
	}
}

// TestVectorCoreC1MatchesContinuousTrainer pins the Root decision 6 contract at
// the learning layer: a declared state_dimension of 1 keeps node rows and every
// learned buffer at scalar width, so Predict, LossGradient and one whole Step
// must be identical to the pre-existing state_dimension 0 model.
func TestVectorCoreC1MatchesContinuousTrainer(t *testing.T) {
	p := scalarParameters()
	input := [][]float64{{.8}, {-.2}, {.3}}
	target := []float64{.25}
	n0, err := learning.NewNetwork(vectorScalarConfig(0))
	if err != nil {
		t.Fatal(err)
	}
	n1, err := learning.NewNetwork(vectorScalarConfig(1))
	if err != nil {
		t.Fatal(err)
	}
	pred0, err := n0.Predict(context.Background(), p, input)
	if err != nil {
		t.Fatal(err)
	}
	pred1, err := n1.Predict(context.Background(), p, input)
	if err != nil {
		t.Fatal(err)
	}
	requireSlicesClose(t, "predict", pred1, pred0, 0)
	loss0, g0, err := n0.LossGradient(context.Background(), p, input, target, 0)
	if err != nil {
		t.Fatal(err)
	}
	loss1, g1, err := n1.LossGradient(context.Background(), p, input, target, 0)
	if err != nil {
		t.Fatal(err)
	}
	if loss1 != loss0 {
		t.Fatalf("loss = %v, want %v", loss1, loss0)
	}
	if !reflect.DeepEqual(g1, g0) {
		t.Fatalf("LossGradient diverged on a state_dimension 1 declaration:\n scalar=%+v\n declared=%+v", g0, g1)
	}
	tr0, err := learning.NewTrainer(vectorScalarConfig(0), p, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	tr1, err := learning.NewTrainer(vectorScalarConfig(1), p, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr0.Step(context.Background(), input, target); err != nil {
		t.Fatal(err)
	}
	if _, err := tr1.Step(context.Background(), input, target); err != nil {
		t.Fatal(err)
	}
	s0 := tr0.Snapshot()
	s1 := tr1.Snapshot()
	if d := s0.Config.Dynamics.StateDimension; d != 0 {
		t.Fatalf("scalar snapshot carries state_dimension %d", d)
	}
	if d := s1.Config.Dynamics.StateDimension; d != 1 {
		t.Fatalf("declared-1 snapshot carries state_dimension %d", d)
	}
	s1.Config.Dynamics.StateDimension = 0
	if !reflect.DeepEqual(s1, s0) {
		t.Fatalf("one Step diverged the snapshots:\n scalar=%+v\n declared=%+v", s0, s1)
	}
}

// vectorC2Config declares the two-node chain as vector nodes: every node row is
// two values wide, one edge carries a row-major 2x2 matrix, and the readout
// observes the full two-component output of node 1.
func vectorC2Config() learning.Config {
	return learning.Config{
		Dynamics: dynamics.Config{
			Nodes:          2,
			Sources:        []int{0},
			Targets:        []int{1},
			Delays:         []int{0},
			DT:             .5,
			Activation:     "tanh",
			StateDimension: 2,
			EdgeShape:      "matrix",
		},
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: []int{1},
	}
}

// vectorC2Parameters matches vectorC2Config: weights E*C*C = 4, bias N*C = 4,
// log_tau N = 2, encoder InputSize*(N*C) = 4 and readout len(Readout)*C*Output
// = 2.
func vectorC2Parameters() learning.Parameters {
	return learning.Parameters{
		Core: dynamics.Parameters{
			Weights: []float64{.3, -.1, .2, .15},
			Bias:    []float64{.1, -.2, .05, .3},
			LogTau:  []float64{.2, -.05},
		},
		Encoder: []float64{.7, -.2, .3, .6},
		Readout: []float64{.8, -.4},
	}
}

// TestVectorCoreC2GradientMatchesFiniteDifference checks the whole training
// graph every gradient flows through: the encoder, the per-component brain
// input, the vector core with a matrix edge and the two-component readout. The
// analytic LossGradient must match a central difference of the same Predicted
// MSE on every one of the 16 learnable parameters.
func TestVectorCoreC2GradientMatchesFiniteDifference(t *testing.T) {
	network, err := learning.NewNetwork(vectorC2Config())
	if err != nil {
		t.Fatal(err)
	}
	parameters := vectorC2Parameters()
	input := [][]float64{{1}, {.5}}
	target := []float64{.8}
	_, gradient, err := network.LossGradient(context.Background(), parameters, input, target, 0)
	if err != nil {
		t.Fatal(err)
	}
	const h = 1e-3
	worst := 0.0
	check := func(label string, value *float64, derivative float64) {
		t.Helper()
		original := *value
		*value = original + h
		plus := selectedLoss(t, network, parameters, input, target)
		*value = original - h
		minus := selectedLoss(t, network, parameters, input, target)
		*value = original
		finiteDifference := (plus - minus) / (2 * h)
		relative := math.Abs(derivative-finiteDifference) / math.Max(math.Abs(finiteDifference), 1e-9)
		if relative > worst {
			worst = relative
		}
		requireClose(t, label, derivative, finiteDifference, 1e-6+1e-3*math.Abs(finiteDifference))
	}
	for i := range parameters.Core.Weights {
		check("core weights finite difference", &parameters.Core.Weights[i], gradient.Core.Weights[i])
	}
	for i := range parameters.Core.Bias {
		check("core bias finite difference", &parameters.Core.Bias[i], gradient.Core.Bias[i])
	}
	for i := range parameters.Core.LogTau {
		check("core log tau finite difference", &parameters.Core.LogTau[i], gradient.Core.LogTau[i])
	}
	for i := range parameters.Encoder {
		check("encoder finite difference", &parameters.Encoder[i], gradient.Encoder[i])
	}
	for i := range parameters.Readout {
		check("readout finite difference", &parameters.Readout[i], gradient.Readout[i])
	}
	t.Logf("vector core C=2 worst relative finite-difference error: %.3g", worst)
}

// TestVectorCoreC2Trains runs the full optimizer on the C=2 network and checks
// that every parameter group actually moves: a core whose weight count, bias
// count or component routing silently dropped a gradient would leave one of
// these groups untouched.
func TestVectorCoreC2Trains(t *testing.T) {
	options := learning.DefaultOptions()
	options.LearningRate = .1
	trainer, err := learning.NewTrainer(vectorC2Config(), vectorC2Parameters(), options)
	if err != nil {
		t.Fatal(err)
	}
	input := [][]float64{{1}, {.5}}
	target := []float64{.8}
	var last learning.StepResult
	for step := 0; step < 5; step++ {
		last, err = trainer.Step(context.Background(), input, target)
		if err != nil {
			t.Fatal(err)
		}
		if !last.LossKnown || math.IsNaN(last.Loss) || math.IsInf(last.Loss, 0) {
			t.Fatalf("step %d loss %v is not finite (known %v)", step, last.Loss, last.LossKnown)
		}
		if math.IsNaN(last.GradientNorm) || math.IsInf(last.GradientNorm, 0) || last.GradientNorm <= 0 {
			t.Fatalf("step %d gradient norm %v is not positive and finite", step, last.GradientNorm)
		}
	}
	before := vectorC2Parameters()
	after := trainer.Snapshot().Parameters
	for _, group := range []struct {
		name          string
		before, after []float64
	}{
		{"weights", before.Core.Weights, after.Core.Weights},
		{"bias", before.Core.Bias, after.Core.Bias},
		{"log_tau", before.Core.LogTau, after.Core.LogTau},
		{"encoder", before.Encoder, after.Encoder},
		{"readout", before.Readout, after.Readout},
	} {
		if reflect.DeepEqual(group.before, group.after) {
			t.Fatalf("five steps left the %s group untouched", group.name)
		}
	}
}

// TestVectorCoreRejectsPersistentStateForNow keeps the persistent-individual
// door closed by name: the vector core is trainable through independent
// episodes today, while its persistent advance arrives with the next stage, so
// NewIndividual must refuse it immediately.
func TestVectorCoreRejectsPersistentStateForNow(t *testing.T) {
	if _, err := learning.NewTrainer(vectorC2Config(), vectorC2Parameters(), learning.DefaultOptions()); err != nil {
		t.Fatalf("vector core must be trainable now: %v", err)
	}
	_, err := learning.NewIndividual(vectorC2Config(), vectorC2Parameters(), learning.DefaultOptions(), make([]float64, 4))
	if err == nil {
		t.Fatal("NewIndividual accepted a vector core's persistent state")
	}
	if !strings.Contains(err.Error(), "next ticket") {
		t.Fatalf("persistent-state refusal %q does not name the next ticket", err)
	}
}
