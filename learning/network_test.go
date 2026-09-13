package learning_test

import (
	"context"
	"math"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

func network(t *testing.T) (*learning.Network, learning.Parameters) {
	t.Helper()
	n, err := learning.NewNetwork(learning.Config{Dynamics: dynamics.Config{Nodes: 2, Sources: []int{0, 1}, Targets: []int{1, 0}, DT: .5, Activation: "tanh"}, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	p := learning.Parameters{Core: dynamics.Parameters{Weights: []float64{.3, -.2}, Bias: []float64{.1, -.1}, LogTau: []float64{.1, .2}}, Encoder: []float64{.5, .1}, Readout: []float64{.8}}
	return n, p
}

func TestInsyraBridgeFullGradient(t *testing.T) {
	n, p := network(t)
	x := [][]float64{{.7}, {-.2}, {.1}}
	target := []float64{.4}
	loss, g, err := n.LossGradient(context.Background(), p, x, target, 0)
	if err != nil {
		t.Fatal(err)
	}
	if loss <= 0 {
		t.Fatal("expected positive loss")
	}
	pairs := []struct{ v, g []float64 }{{p.Core.Weights, g.Core.Weights}, {p.Core.Bias, g.Core.Bias}, {p.Core.LogTau, g.Core.LogTau}, {p.Encoder, g.Encoder}, {p.Readout, g.Readout}}
	for i := range x {
		pairs = append(pairs, struct{ v, g []float64 }{x[i], g.Inputs[i]})
	}
	for _, pair := range pairs {
		for i := range pair.v {
			old := pair.v[i]
			const eps = 2e-3
			pair.v[i] = old + eps
			yp, err := n.Predict(context.Background(), p, x)
			if err != nil {
				t.Fatal(err)
			}
			pair.v[i] = old - eps
			ym, err := n.Predict(context.Background(), p, x)
			if err != nil {
				t.Fatal(err)
			}
			pair.v[i] = old
			fd := (math.Pow(yp[0]-target[0], 2) - math.Pow(ym[0]-target[0], 2)) / (2 * eps)
			if math.Abs(fd-pair.g[i]) > 2e-5+1e-3*math.Abs(fd) {
				t.Fatalf("gradient got=%g finite difference=%g", pair.g[i], fd)
			}
		}
	}
	if g.Core.Weights[0] == 0 || g.Encoder[0] == 0 || g.Readout[0] == 0 {
		t.Fatal("broken full gradient path")
	}
}

func TestNetworkRejectsInvalidAndCopiesConfiguration(t *testing.T) {
	n, p := network(t)
	for _, x := range [][][]float64{nil, {{}}, {{math.Inf(1)}}, {{math.MaxFloat64}}} {
		if _, err := n.Predict(context.Background(), p, x); err == nil {
			t.Fatalf("accepted %v", x)
		}
	}
	if _, _, err := n.LossGradient(context.Background(), p, [][]float64{{1}}, nil, 0); err == nil {
		t.Fatal("accepted missing target")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := n.Predict(ctx, p, [][]float64{{1}}); err == nil {
		t.Fatal("accepted canceled context")
	}
	if _, err := n.Predict(nil, p, [][]float64{{1}}); err == nil {
		t.Fatal("accepted nil context")
	}
	config := n.Config()
	config.ReadoutNodes[0] = 0
	config.Dynamics.Sources[0] = 1
	if n.Config().ReadoutNodes[0] != 1 || n.Config().Dynamics.Sources[0] != 0 {
		t.Fatal("configuration aliases model")
	}
}
