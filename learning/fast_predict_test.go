package learning

import (
	"context"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
)

func fastPredictFixture(t *testing.T, inputNodes bool, signs bool) (*Network, Parameters, [][]float64) {
	t.Helper()
	return fastPredictFixtureGate(t, inputNodes, signs, dynamics.GateCurrentSoftsign)
}

func fastPredictFixtureGate(t *testing.T, inputNodes bool, signs bool, gate string) (*Network, Parameters, [][]float64) {
	t.Helper()
	const nodes, degree, inputs, outputs, steps = 64, 9, 7, 5, 12
	rnd := rand.New(rand.NewPCG(3, 4))
	c := Config{
		Dynamics:  dynamics.Config{Nodes: nodes, DT: 1, Activation: "softsign", Gate: gate},
		InputSize: inputs, OutputSize: outputs,
	}
	for target := range nodes {
		for _, src := range rnd.Perm(nodes)[:degree] {
			c.Dynamics.Sources = append(c.Dynamics.Sources, src)
			c.Dynamics.Targets = append(c.Dynamics.Targets, target)
		}
	}
	for i := nodes - 20; i < nodes; i++ {
		c.ReadoutNodes = append(c.ReadoutNodes, i)
	}
	width := nodes
	if inputNodes {
		c.InputNodes = []int{0, 3, 5, 8, 13, 21}
		width = len(c.InputNodes)
	}
	edges := len(c.Dynamics.Sources)
	if signs {
		c.EdgeSigns = make([]int8, edges)
		for e := range c.EdgeSigns {
			c.EdgeSigns[e] = int8(rnd.IntN(3) - 1)
		}
	}
	n, err := NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	p := Parameters{
		Core: dynamics.Parameters{Weights: make([]float64, edges), Bias: make([]float64, nodes), LogTau: make([]float64, nodes),
			GateGain: make([]float64, nodes), GateBias: make([]float64, nodes)},
		Encoder: make([]float64, inputs*width), Readout: make([]float64, len(c.ReadoutNodes)*outputs),
	}
	for e := range p.Core.Weights {
		p.Core.Weights[e] = rnd.NormFloat64() * .3
		if signs && c.EdgeSigns[e] != 0 {
			p.Core.Weights[e] = math.Log(math.Abs(p.Core.Weights[e]) + 1e-3)
		}
	}
	for i := range nodes {
		p.Core.Bias[i], p.Core.LogTau[i] = rnd.NormFloat64()*.1, rnd.Float64()*2
		p.Core.GateGain[i], p.Core.GateBias[i] = rnd.NormFloat64(), rnd.NormFloat64()
	}
	if gate == dynamics.GateCurrentState {
		p.Core.GateState = make([]float64, nodes)
		for i := range nodes {
			p.Core.GateState[i] = rnd.NormFloat64()
		}
	}
	for i := range p.Encoder {
		p.Encoder[i] = rnd.NormFloat64() * .4
	}
	for i := range p.Readout {
		p.Readout[i] = rnd.NormFloat64() * .3
	}
	x := make([][]float64, steps)
	for s := range x {
		x[s] = make([]float64, inputs)
		for i := range x[s] {
			x[s][i] = rnd.Float64()
		}
	}
	return n, p, x
}

func TestFastPredictMatchesPredict(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name               string
		inputNodes, signed bool
		gate               string
	}{{"all-nodes", false, false, dynamics.GateCurrentSoftsign}, {"input-nodes", true, false, dynamics.GateCurrentSoftsign},
		{"edge-signs", true, true, dynamics.GateCurrentSoftsign}, {"state-gate", true, false, dynamics.GateCurrentState}} {
		t.Run(tc.name, func(t *testing.T) {
			n, p, x := fastPredictFixtureGate(t, tc.inputNodes, tc.signed, tc.gate)
			want, err := n.Predict(ctx, p, x)
			if err != nil {
				t.Fatal(err)
			}
			fp, err := n.CompileFastPredict(p)
			if err != nil {
				t.Fatal(err)
			}
			for range 2 { // the second call must not see the first one's state
				got, err := fp.Predict(ctx, x)
				if err != nil {
					t.Fatal(err)
				}
				for i := range want {
					if d := math.Abs(want[i] - got[i]); d > 1e-4 {
						t.Fatalf("logit %d: fast %g, Predict %g (|d| = %g)", i, got[i], want[i], d)
					}
				}
			}
		})
	}
}

func TestCompileFastPredictRefusesEveryStepReadout(t *testing.T) {
	n, p, _ := fastPredictFixture(t, true, false)
	c := n.Config()
	c.ReadoutEveryStep = true
	every, err := NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := every.CompileFastPredict(p); err == nil {
		t.Fatal("compiled a network that reads every step")
	}
}
