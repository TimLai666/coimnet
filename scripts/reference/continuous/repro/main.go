// Command repro prints minimal reproductions of the two documentation /
// behaviour mismatches found by the continuous-core reference audit.
//
//	go run ./scripts/reference/continuous/repro
package main

import (
	"context"
	"fmt"
	"math"
	"reflect"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

func chain(n int) learning.Config {
	c := learning.Config{InputSize: 1, OutputSize: 1, ReadoutNodes: []int{n - 1}, InputNodes: []int{0}}
	c.Dynamics = dynamics.Config{Nodes: n, DT: 1, Activation: "tanh"}
	for i := 0; i+1 < n; i++ {
		c.Dynamics.Sources = append(c.Dynamics.Sources, i)
		c.Dynamics.Targets = append(c.Dynamics.Targets, i+1)
	}
	return c
}

func params(c learning.Config) learning.Parameters {
	n, e := c.Dynamics.Nodes, len(c.Dynamics.Sources)
	p := learning.Parameters{Encoder: []float64{1}, Readout: []float64{1}}
	p.Core.Weights = make([]float64, e)
	for i := range p.Core.Weights {
		p.Core.Weights[i] = 0.9
	}
	p.Core.Bias = make([]float64, n)
	p.Core.LogTau = make([]float64, n)
	return p
}

func main() {
	ctx := context.Background()

	// 1. GradientHorizonSteps vs the fixed-boundary truncation of Backward.
	fmt.Println("== 1. GradientHorizonSteps with a last-step loss ==")
	c := learning.Config{InputSize: 1, OutputSize: 1, ReadoutNodes: []int{0}}
	c.Dynamics = dynamics.Config{Nodes: 1, DT: 1, Activation: "tanh", Sources: []int{0}, Targets: []int{0}}
	p := learning.Parameters{Encoder: []float64{1}, Readout: []float64{1}}
	p.Core = dynamics.Parameters{Weights: []float64{0.5}, Bias: []float64{0}, LogTau: []float64{0}}
	input := [][]float64{{1}, {1}, {1}, {1}, {1}}
	for _, w := range []int{2, 3, 4, 5} {
		net, _ := learning.NewNetwork(c)
		_, g, err := net.LossGradient(ctx, p, input, []float64{0}, w)
		if err != nil {
			panic(err)
		}
		reached := 0
		for t := range g.Inputs {
			if g.Inputs[t][0] != 0 {
				reached = len(g.Inputs) - t
				break
			}
		}
		o := learning.DefaultOptions()
		o.Truncation = w
		tr, err := learning.NewTrainer(c, p, o)
		if err != nil {
			panic(err)
		}
		res, err := tr.Step(ctx, input, []float64{0})
		if err != nil {
			panic(err)
		}
		fmt.Printf("T=5 truncation=%d: input-gradient rows %v -> loss reaches back %d step(s); StepResult.GradientHorizonSteps=%d\n", w, flatten(g.Inputs), reached, res.GradientHorizonSteps)
	}

	// 2. LossScale cannot rescue an underflowing float32 gradient.
	fmt.Println("\n== 2. LossScale is applied after the gradient is complete ==")
	c2 := learning.Config{InputSize: 1, OutputSize: 1, ReadoutNodes: []int{0}}
	c2.Dynamics = dynamics.Config{Nodes: 1, DT: 1, Activation: "tanh"}
	p2 := learning.Parameters{Encoder: []float64{1}, Readout: []float64{1e-30}}
	p2.Core = dynamics.Parameters{Bias: []float64{0}, LogTau: []float64{0}}
	in2 := [][]float64{{0.5}}
	// prediction ~ tanh(0.5*alpha)*1e-30; target 0 gives dL/dpred ~ 1e-30 in
	// float32 and dL/dh = dL/dpred * readout ~ 1e-60, which float32 flushes to 0.
	net2, _ := learning.NewNetwork(c2)
	_, g2, err := net2.LossGradient(ctx, p2, in2, []float64{0}, 0)
	if err != nil {
		panic(err)
	}
	h := math.Tanh(0.5 * -math.Expm1(-1))
	exact := 2 * h * 1e-30 * 1e-30 * (1 - h*h) * -math.Expm1(-1) // dL/dbias in exact arithmetic
	fmt.Printf("dL/dbias: framework %g, exact float64 math %g\n", g2.Core.Bias[0], exact)
	var snaps []learning.TrainingSnapshot
	for _, scale := range []float64{1, math.Ldexp(1, 100)} {
		o := learning.DefaultOptions()
		o.LossScale = scale
		o.Trainable = learning.Trainable{Bias: true}
		tr, err := learning.NewTrainer(c2, p2, o)
		if err != nil {
			panic(err)
		}
		res, err := tr.Step(ctx, in2, []float64{0})
		if err != nil {
			panic(err)
		}
		s := tr.Snapshot()
		fmt.Printf("LossScale=%g: GradientNorm=%g bias after step=%g adam first=%g\n", scale, res.GradientNorm, s.Parameters.Core.Bias[0], s.Optimizer.First[0])
		snaps = append(snaps, s)
	}
	snaps[1].Options.LossScale = snaps[0].Options.LossScale
	fmt.Println("snapshots identical apart from options.loss_scale:", reflect.DeepEqual(snaps[0], snaps[1]))
}

func flatten(rows [][]float64) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = fmt.Sprintf("%.3g", r[0])
	}
	return out
}
