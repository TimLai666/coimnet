package dynamics

import (
	"context"
	"math/rand/v2"
	"testing"
)

// BenchmarkInferSeqKMNISTCore runs the record-free pass on the shape of the
// examples/seqkmnist core: 512 undelayed nodes with 22 inputs each, the
// softsign gate and activation, and 28 steps.
func BenchmarkInferSeqKMNISTCore(b *testing.B) {
	const nodes, degree, steps = 512, 22, 28
	rnd := rand.New(rand.NewPCG(1, 2))
	c := Config{Nodes: nodes, DT: 1, Activation: "softsign", Gate: GateCurrentSoftsign}
	for target := range nodes {
		for range degree {
			c.Sources = append(c.Sources, rnd.IntN(nodes))
			c.Targets = append(c.Targets, target)
		}
	}
	m, err := NewContinuous(c)
	if err != nil {
		b.Fatal(err)
	}
	edges := len(c.Sources)
	p := Parameters{Weights: make([]float64, edges), Bias: make([]float64, nodes), LogTau: make([]float64, nodes), GateGain: make([]float64, nodes), GateBias: make([]float64, nodes)}
	for e := range p.Weights {
		p.Weights[e] = rnd.NormFloat64() * .2
	}
	inputs := make([][]float64, steps)
	for t := range inputs {
		inputs[t] = make([]float64, nodes)
		for i := range 128 {
			inputs[t][i] = rnd.NormFloat64()
		}
	}
	initial := make([]float64, nodes)
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := m.Infer(ctx, p, initial, inputs); err != nil {
			b.Fatal(err)
		}
	}
}
