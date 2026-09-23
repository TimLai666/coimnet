package dynamics

import (
	"context"
	"math"
	"math/rand/v2"
	"testing"
)

// TestVectorDenseReference pins the Root decision 6 continuous vector core to
// the D7 dense reference: the C = 3 matrix fixture's forward outputs pass one
// by one, and its backward gradients agree on Inputs, Initial, Bias, the
// expanded Weights positions and the per-node LogTau as the sum of the three
// component scalar gradients, for the full window and for window 2.
func TestVectorDenseReference(t *testing.T) {
	const (
		C     = 3
		N     = 4
		steps = 6
	)
	sources := []int{0, 1, 2, 1, 0}
	targets := []int{1, 2, 2, 0, 2}
	delays := []int{0, 1, 0, 2, 0}
	vec, err := NewVectorContinuous(Config{Nodes: N, Sources: sources, Targets: targets, Delays: delays, DT: .5, Activation: "tanh", StateDimension: C, EdgeShape: "matrix"})
	if err != nil {
		t.Fatal(err)
	}
	nv := N * C
	rng := rand.New(rand.NewPCG(4, 0))
	fill := func(n int) []float64 {
		v := make([]float64, n)
		for i := range v {
			v[i] = .5 * (2*rng.Float64() - 1)
		}
		return v
	}
	p := VectorParameters{Weights: fill(len(sources) * C * C), Bias: fill(nv), LogTau: fill(N)}
	initial := fill(nv)
	inputs := make([][]float64, steps)
	for t := range inputs {
		inputs[t] = fill(nv)
	}

	denseSources := make([]int, 0, len(sources)*C*C)
	denseTargets := make([]int, 0, len(sources)*C*C)
	denseDelays := make([]int, 0, len(sources)*C*C)
	denseWeights := make([]float64, len(sources)*C*C)
	k := 0
	for e := range sources {
		for i := 0; i < C; i++ {
			for j := 0; j < C; j++ {
				denseSources = append(denseSources, C*sources[e]+j)
				denseTargets = append(denseTargets, C*targets[e]+i)
				denseDelays = append(denseDelays, delays[e])
				denseWeights[k] = p.Weights[e*C*C+i*C+j]
				k++
			}
		}
	}
	scalarLogTau := make([]float64, N*C)
	for n := 0; n < N; n++ {
		for i := 0; i < C; i++ {
			scalarLogTau[n*C+i] = p.LogTau[n]
		}
	}
	sca, err := NewContinuous(Config{Nodes: N * C, Sources: denseSources, Targets: denseTargets, Delays: denseDelays, DT: .5, Activation: "tanh"})
	if err != nil {
		t.Fatal(err)
	}
	sp := Parameters{Weights: append([]float64(nil), denseWeights...), Bias: append([]float64(nil), p.Bias...), LogTau: scalarLogTau}

	vtr, err := vec.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	ctr, err := sca.Forward(context.Background(), sp, append([]float64(nil), initial...), cloneTestRowsForVector(inputs))
	if err != nil {
		t.Fatal(err)
	}
	got := vtr.Outputs()
	want := ctr.Outputs()
	maxFwd := 0.0
	for r := 0; r < nv; r++ {
		if d := math.Abs(got[0][r] - math.Tanh(initial[r])); d > maxFwd {
			maxFwd = d
		}
	}
	for t := 0; t < steps; t++ {
		for r := 0; r < nv; r++ {
			if d := math.Abs(got[t+1][r] - want[t][r]); d > maxFwd {
				maxFwd = d
			}
		}
	}
	if maxFwd > 1e-12 {
		t.Fatalf("forward dense reference max absolute error %.3g exceeds 1e-12", maxFwd)
	}
	t.Logf("forward dense reference max absolute error %.3g", maxFwd)

	up := make([][]float64, steps)
	for t := range up {
		up[t] = fill(nv)
	}
	for _, window := range []int{0, 2} {
		vg, err := vec.Backward(context.Background(), vtr, cloneTestRowsForVector(up), window)
		if err != nil {
			t.Fatal(err)
		}
		cg, err := sca.Backward(context.Background(), ctr, cloneTestRowsForVector(up), window)
		if err != nil {
			t.Fatal(err)
		}
		maxBwd := 0.0
		for t := 0; t < steps; t++ {
			for r := 0; r < nv; r++ {
				if d := math.Abs(vg.Inputs[t][r] - cg.Inputs[t][r]); d > maxBwd {
					maxBwd = d
				}
			}
		}
		for r := 0; r < nv; r++ {
			if d := math.Abs(vg.Initial[r] - cg.Initial[r]); d > maxBwd {
				maxBwd = d
			}
			if d := math.Abs(vg.Bias[r] - cg.Bias[r]); d > maxBwd {
				maxBwd = d
			}
		}
		for e := range denseWeights {
			if d := math.Abs(vg.Weights[e] - cg.Weights[e]); d > maxBwd {
				maxBwd = d
			}
		}
		for n := 0; n < N; n++ {
			sum := 0.0
			for i := 0; i < C; i++ {
				sum += cg.LogTau[n*C+i]
			}
			if d := math.Abs(vg.LogTau[n] - sum); d > maxBwd {
				maxBwd = d
			}
		}
		if maxBwd > 1e-10 {
			t.Fatalf("window %d backward dense reference max absolute error %.3g exceeds 1e-10", window, maxBwd)
		}
		t.Logf("window %d backward dense reference max absolute error %.3g", window, maxBwd)
	}
}

// TestVectorZeroEdgesAndIsolatedNodes pins the degenerate anatomy of Root
// decision 6 with the next D7 fixture: no edge at all, so every component
// follows the closed-form v(t+1) = lambda*v + alpha*(x + b) recurrence and the
// reverse pass returns an empty Weights gradient without an error.
func TestVectorZeroEdgesAndIsolatedNodes(t *testing.T) {
	m, err := NewVectorContinuous(Config{Nodes: 2, DT: .5, Activation: "tanh", StateDimension: 3, EdgeShape: "scalar"})
	if err != nil {
		t.Fatal(err)
	}
	p := VectorParameters{
		Weights: nil,
		Bias:    []float64{.1, -.2, .05, -.1, .2, -.05},
		LogTau:  []float64{.2, -.1},
	}
	initial := []float64{.3, .1, -.2, -.3, .2, .4}
	inputs := [][]float64{
		{.5, -.4, .3, .2, -.1, .4},
		{-.2, .1, .6, -.3, .5, -.2},
		{.4, .3, -.1, -.2, .4, .1},
		{-.5, .2, .1, .3, -.4, .3},
		{.2, -.3, -.2, .1, -.3, -.5},
	}
	tr, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	lambda := []float64{math.Exp(-.5 / math.Exp(p.LogTau[0])), math.Exp(-.5 / math.Exp(p.LogTau[1]))}
	alpha := []float64{-math.Expm1(-.5 / math.Exp(p.LogTau[0])), -math.Expm1(-.5 / math.Exp(p.LogTau[1]))}
	expectedV := make([][]float64, len(inputs)+1)
	for t := range expectedV {
		expectedV[t] = make([]float64, 6)
	}
	copy(expectedV[0], initial)
	expectedOut := make([][]float64, len(inputs)+1)
	for t := range expectedOut {
		expectedOut[t] = make([]float64, 6)
	}
	for r := 0; r < 6; r++ {
		expectedOut[0][r] = math.Tanh(expectedV[0][r])
	}
	for t := 0; t < len(inputs); t++ {
		for r := 0; r < 6; r++ {
			l, a := lambda[0], alpha[0]
			if r >= 3 {
				l, a = lambda[1], alpha[1]
			}
			d := inputs[t][r] + p.Bias[r]
			expectedV[t+1][r] = l*expectedV[t][r] + a*d
			expectedOut[t+1][r] = math.Tanh(expectedV[t+1][r])
		}
	}
	got := tr.Outputs()
	maxFwd := 0.0
	for t := 0; t < len(expectedOut); t++ {
		for r := 0; r < 6; r++ {
			if d := math.Abs(got[t][r] - expectedOut[t][r]); d > maxFwd {
				maxFwd = d
			}
		}
	}
	if maxFwd > 1e-15 {
		t.Fatalf("zero-edge forward max absolute error %.3g exceeds 1e-15", maxFwd)
	}
	t.Logf("zero-edge forward max absolute error %.3g", maxFwd)

	up := [][]float64{
		{.2, -.4, .3, .1, -.3, .4},
		{-.1, .5, -.2, .4, .2, -.4},
		{.4, .2, -.5, -.3, .1, .3},
		{-.3, .1, .4, .2, -.5, .2},
		{.1, -.3, .5, -.4, .3, -.1},
	}
	g, err := m.Backward(context.Background(), tr, up, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Weights) != 0 {
		t.Fatalf("zero-edge backward weights gradient has %d values, want 0", len(g.Weights))
	}
	if len(g.Bias) != 6 || len(g.LogTau) != 2 || len(g.Initial) != 6 || len(g.Inputs) != len(inputs) {
		t.Fatalf("zero-edge backward gradient shapes: weights %d bias %d log_tau %d initial %d inputs %d",
			len(g.Weights), len(g.Bias), len(g.LogTau), len(g.Initial), len(g.Inputs))
	}
}
