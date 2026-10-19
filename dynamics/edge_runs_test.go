package dynamics

import (
	"context"
	"math"
	"math/rand/v2"
	"reflect"
	"testing"
)

// TestUndelayedRunsMatchDeclaredOrderPartition checks the register-run path an
// undelayed single threaded model takes against the worker partition, which
// sums every target's inputs in declared edge order on its own code path. The
// edge list mixes grouped runs with targets that reappear later, so a run
// must not merge edges across a different target.
func TestUndelayedRunsMatchDeclaredOrderPartition(t *testing.T) {
	rnd := rand.New(rand.NewPCG(5, 8))
	const nodes = 40
	var sources, targets []int
	for len(sources) < 600 {
		target, length := rnd.IntN(nodes), 1+rnd.IntN(12)
		for range length {
			sources, targets = append(sources, rnd.IntN(nodes)), append(targets, target)
		}
	}
	p := Parameters{Weights: make([]float64, len(sources)), Bias: make([]float64, nodes), LogTau: make([]float64, nodes)}
	for i := range p.Weights {
		p.Weights[i] = rnd.NormFloat64() * .3
	}
	for i := range p.Bias {
		p.Bias[i] = rnd.NormFloat64() * .1
		p.LogTau[i] = rnd.NormFloat64()*.3 + math.Log(2)
	}
	initial := make([]float64, nodes)
	inputs := make([][]float64, 20)
	for i := range inputs {
		inputs[i] = make([]float64, nodes)
		for j := range inputs[i] {
			inputs[i][j] = rnd.NormFloat64()
		}
	}
	serial, err := NewContinuous(Config{Nodes: nodes, Sources: sources, Targets: targets, DT: 1, Activation: "tanh"})
	if err != nil {
		t.Fatal(err)
	}
	if !serial.zeroDelay || len(serial.runs) >= len(sources) {
		t.Fatalf("fixture does not exercise grouped undelayed runs: zeroDelay %v, %d runs for %d edges", serial.zeroDelay, len(serial.runs), len(sources))
	}
	parallel, err := NewContinuous(Config{Nodes: nodes, Sources: sources, Targets: targets, Delays: make([]int, len(sources)), DT: 1, Activation: "tanh", Workers: 3})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	a, err := serial.Forward(ctx, p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	b, err := parallel.Forward(ctx, p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.voltage, b.voltage) || !reflect.DeepEqual(a.output, b.output) || !reflect.DeepEqual(a.drive, b.drive) {
		t.Fatal("undelayed run path differs from the declared-order partition")
	}
	upstream := make([][]float64, len(inputs))
	for i := range upstream {
		upstream[i] = make([]float64, nodes)
		upstream[i][rnd.IntN(nodes)] = 1
	}
	ga, err := serial.Backward(ctx, a, upstream, 0)
	if err != nil {
		t.Fatal(err)
	}
	gb, err := parallel.Backward(ctx, b, upstream, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ga, gb) {
		t.Fatal("gradients differ between the run path and the partition")
	}
}
