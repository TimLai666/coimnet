package learning_test

import (
	"context"
	"math"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func TestStatefulGradientPreservesSelectedInputSignsSharingAndMasks(t *testing.T) {
	ctx := context.Background()
	c, p, o, prefix, input, upstream := statefulLearningFixture(t)
	c.InputNodes = []int{0}
	c.ReadoutEveryStep = false
	c.EdgeSigns = []int8{1, -1, 1}
	c.Sharing = &learning.ParameterSharing{Weights: []int32{0, -1, 0}, Groups: 1}
	p.Core.Weights = []float64{math.Log(.2), math.Log(.1), math.Log(.2)}
	p.Encoder = []float64{.5}
	o.LearningRate, o.WeightDecay, o.Epsilon, o.ClipNorm = .037, .02, .011, 0
	o.Trainable = learning.Trainable{Weights: true}
	o.Masks = &learning.UpdateMasks{Edges: []bool{true, true, false}}
	ind, err := learning.NewIndividual(c, p, o, []float64{.2, -.3})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ind.Advance(ctx, prefix); err != nil {
		t.Fatal(err)
	}
	base := ind.Snapshot()
	n, err := learning.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	g, err := n.LossGradientFromState(ctx, p, base.Neural, input, upstream, 0)
	if err != nil {
		t.Fatal(err)
	}
	objective := func(delta float64) float64 {
		s := base
		s.Parameters = cloneLearningParametersStateful(p)
		s.Parameters.Core.Weights[0] += delta
		s.Parameters.Core.Weights[2] += delta
		at, err := learning.RestoreIndividual(s)
		if err != nil {
			t.Fatal(err)
		}
		y, err := at.Advance(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		return y[len(y)-1][0] * upstream[len(upstream)-1][0]
	}
	const eps = 2e-3
	fd := (objective(eps) - objective(-eps)) / (2 * eps)
	sum := g.Core.Weights[0] + g.Core.Weights[2]
	if math.Abs(sum-fd) > 1e-5+1e-4*math.Abs(fd) {
		t.Fatalf("shared log-magnitude derivative %g, finite difference %g", sum, fd)
	}
	if g.Core.Weights[1] == 0 || sum == 0 {
		t.Fatal("fixture must produce nonzero fixed-sign and shared gradients")
	}
	tr, err := learning.NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.StepFromState(ctx, base.Neural, input, upstream); err != nil {
		t.Fatal(err)
	}
	after := tr.Snapshot()
	for _, i := range []int{0, 2} {
		if after.Parameters.Core.Weights[i] != p.Core.Weights[i] || after.Optimizer.Steps[i] != 0 || after.Optimizer.First[i] != 0 || after.Optimizer.Second[i] != 0 {
			t.Fatalf("mask conflict must freeze the whole sharing group, member %d changed", i)
		}
	}
	d := g.Core.Weights[1]
	want := p.Core.Weights[1]*(1-o.LearningRate*o.WeightDecay) - o.LearningRate*d/(math.Abs(d)+o.Epsilon)
	if math.Abs(after.Parameters.Core.Weights[1]-want) > 2e-12 || after.Optimizer.Steps[1] != 1 {
		t.Fatalf("individual negative edge update %g, want %g", after.Parameters.Core.Weights[1], want)
	}
}
