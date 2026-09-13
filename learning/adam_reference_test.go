package learning_test

import (
	"context"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"math"
	"testing"
)

func TestAdamWFirstUpdateMatchesIndependentScalarReference(t *testing.T) {
	c := learning.Config{Dynamics: dynamics.Config{Nodes: 1, DT: math.Log(2), Activation: "tanh"}, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{0}}
	p := learning.Parameters{Core: dynamics.Parameters{Bias: []float64{0}, LogTau: []float64{0}}, Encoder: []float64{1}, Readout: []float64{2}}
	o := learning.DefaultOptions()
	o.Trainable = learning.Trainable{Readout: true}
	o.LearningRate = .05
	o.WeightDecay = .2
	o.ClipNorm = 0
	o.Epsilon = .01
	tr, err := learning.NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tr.Step(context.Background(), [][]float64{{1}}, []float64{.1}); err != nil {
		t.Fatal(err)
	}
	// One neuron, no edges: v=0.5, h=tanh(0.5), prediction=2h.
	// Insyra's readout and loss execute in float32. First bias-corrected Adam
	// moments are g and g², independently of the beta coefficients.
	h := float32(math.Tanh(.5))
	g := float64(float32(2*(2*h-float32(.1))) * h)
	want := 2*(1-.05*.2) - .05*g/(math.Abs(g)+.01)
	s := tr.Snapshot()
	if math.Abs(s.Parameters.Readout[0]-want) > 1e-12 {
		t.Fatalf("got %.17g want %.17g", s.Parameters.Readout[0], want)
	}
	index := len(s.Optimizer.First) - 1
	if math.Abs(s.Optimizer.First[index]-(1-o.Beta1)*g) > 1e-12 || math.Abs(s.Optimizer.Second[index]-(1-o.Beta2)*g*g) > 1e-12 || s.Optimizer.Steps[index] != 1 {
		t.Fatalf("unexpected moments: %+v", s.Optimizer)
	}
	for _, step := range s.Optimizer.Steps[:index] {
		if step != 0 {
			t.Fatal("frozen parameter accrued optimizer time")
		}
	}
}
