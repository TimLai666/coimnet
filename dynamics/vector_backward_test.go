package dynamics

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"testing"
)

// vectorBackwardC1Fixture is the same delay fixture as
// TestVectorContinuousC1MatchesContinuousBitForBit: three nodes, the four
// declared edges and seven input steps, with a fixed upstream sequence.
func vectorBackwardC1Fixture(t *testing.T) (*VectorContinuous, *Continuous, VectorParameters, Parameters, []float64, [][]float64, [][]float64) {
	t.Helper()
	cfg := Config{Nodes: 3, Sources: []int{0, 1, 2, 1}, Targets: []int{1, 2, 0, 0}, Delays: []int{0, 1, 3, 2}, DT: 0.3, Activation: "tanh"}
	vec, err := NewVectorContinuous(Config{Nodes: 3, Sources: []int{0, 1, 2, 1}, Targets: []int{1, 2, 0, 0}, Delays: []int{0, 1, 3, 2}, DT: 0.3, Activation: "tanh", StateDimension: 1, EdgeShape: "scalar"})
	if err != nil {
		t.Fatal(err)
	}
	sca, err := NewContinuous(cfg)
	if err != nil {
		t.Fatal(err)
	}
	p := VectorParameters{Weights: []float64{0.2, -0.15, 0.1, 0.05}, Bias: []float64{0.1, -0.2, 0.05}, LogTau: []float64{0.2, -0.1, 0.3}}
	sp := Parameters{Weights: []float64{0.2, -0.15, 0.1, 0.05}, Bias: []float64{0.1, -0.2, 0.05}, LogTau: []float64{0.2, -0.1, 0.3}}
	initial := []float64{0.2, -0.3, 0.1}
	inputs := [][]float64{
		{0.1, 0.3, -0.2},
		{-0.2, 0.4, 0.1},
		{0.5, -0.1, 0.2},
		{0.2, 0.2, -0.4},
		{-0.3, 0.1, 0.3},
		{0.1, -0.5, 0.2},
		{0.4, 0.2, 0.1},
	}
	up := [][]float64{
		{0.1, -0.2, 0.4},
		{0.3, 0.2, -0.1},
		{-0.4, 0.5, 0.2},
		{0.2, -0.3, 0.1},
		{0.6, 0.1, -0.2},
		{-0.2, 0.3, 0.4},
		{0.1, -0.5, 0.6},
	}
	return vec, sca, p, sp, initial, inputs, up
}

// TestVectorBackwardC1MatchesContinuousBitForBit pins the vector core's C = 1
// scalar backward to the existing scalar core: every group of the returned
// gradient must equal Continuous.Backward bit for bit.
func TestVectorBackwardC1MatchesContinuousBitForBit(t *testing.T) {
	vec, sca, p, sp, initial, inputs, up := vectorBackwardC1Fixture(t)
	vtr, err := vec.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	ctr, err := sca.Forward(context.Background(), sp, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	vg, err := vec.Backward(context.Background(), vtr, up, 0)
	if err != nil {
		t.Fatal(err)
	}
	cg, err := sca.Backward(context.Background(), ctr, up, 0)
	if err != nil {
		t.Fatal(err)
	}
	assertBitEqual(t, vg.Weights, cg.Weights)
	assertBitEqual(t, vg.Bias, cg.Bias)
	assertBitEqual(t, vg.LogTau, cg.LogTau)
	assertBitEqual(t, vg.Initial, cg.Initial)
	assertBitEqualRows(t, vg.Inputs, cg.Inputs)
}

// TestVectorBackwardWindowMatchesScalarSemantics checks that the truncation
// window has exactly the scalar core's meaning: with window 2 and with window
// 0 (full) the vector gradient equals Continuous.Backward bit for bit.
func TestVectorBackwardWindowMatchesScalarSemantics(t *testing.T) {
	vec, sca, p, sp, initial, inputs, up := vectorBackwardC1Fixture(t)
	vtr, err := vec.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	ctr, err := sca.Forward(context.Background(), sp, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	for _, window := range []int{0, 2} {
		vg, err := vec.Backward(context.Background(), vtr, up, window)
		if err != nil {
			t.Fatal(err)
		}
		cg, err := sca.Backward(context.Background(), ctr, up, window)
		if err != nil {
			t.Fatal(err)
		}
		assertBitEqual(t, vg.Weights, cg.Weights)
		assertBitEqual(t, vg.Bias, cg.Bias)
		assertBitEqual(t, vg.LogTau, cg.LogTau)
		assertBitEqual(t, vg.Initial, cg.Initial)
		assertBitEqualRows(t, vg.Inputs, cg.Inputs)
	}
}

// TestVectorBackwardFiniteDifferences differentiates the C = 2 matrix edge core
// with central differences on every weight, bias, log_tau, initial value and
// per-step input. EdgeShape declares all four edges as C*C matrices. The loss is
// L = sum_t sum_k upstream[t][k] * out[t+1][k], matching Backward's upstream
// convention (upstream row t is dL/d out at step t).
func TestVectorBackwardFiniteDifferences(t *testing.T) {
	cfg := Config{Nodes: 3, Sources: []int{0, 1, 2, 0}, Targets: []int{1, 2, 0, 1}, Delays: []int{0, 1, 0, 1}, DT: 0.3, Activation: "tanh", StateDimension: 2, EdgeShape: "matrix"}
	m, err := NewVectorContinuous(cfg)
	if err != nil {
		t.Fatal(err)
	}
	nv := m.layout.NodeValues()
	if nv != cfg.Nodes*2 {
		t.Fatalf("node values %d, want %d", nv, cfg.Nodes*2)
	}
	rng := rand.New(rand.NewSource(42))
	fill := func(n int, scale float64) []float64 {
		v := make([]float64, n)
		for i := range v {
			v[i] = scale * (2*rng.Float64() - 1)
		}
		return v
	}
	p := VectorParameters{Weights: fill(m.layout.WeightValues(), 0.5), Bias: fill(nv, 0.3), LogTau: fill(cfg.Nodes, 0.3)}
	initial := fill(nv, 0.3)
	inputs := make([][]float64, 4)
	for s := range inputs {
		inputs[s] = fill(nv, 0.4)
	}
	up := make([][]float64, 4)
	for s := range up {
		up[s] = fill(nv, 0.5)
	}
	loss := func() float64 {
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			t.Fatal(err)
		}
		var total float64
		for s := range up {
			row := tr.Outputs()[s+1]
			for k, v := range row {
				total += v * up[s][k]
			}
		}
		return total
	}
	tr, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	g, err := m.Backward(context.Background(), tr, up, 0)
	if err != nil {
		t.Fatal(err)
	}
	groups := []struct {
		name          string
		values, grads []float64
	}{
		{"weights", p.Weights, g.Weights},
		{"bias", p.Bias, g.Bias},
		{"log_tau", p.LogTau, g.LogTau},
		{"initial", initial, g.Initial},
	}
	for s := range inputs {
		groups = append(groups, struct {
			name          string
			values, grads []float64
		}{fmt.Sprintf("inputs[%d]", s), inputs[s], g.Inputs[s]})
	}
	worst := 0.0
	for _, group := range groups {
		for i := range group.values {
			old := group.values[i]
			const eps = 1e-5
			group.values[i] = old + eps
			plus := loss()
			group.values[i] = old - eps
			minus := loss()
			group.values[i] = old
			fd := (plus - minus) / (2 * eps)
			diff := math.Abs(group.grads[i] - fd)
			if rel := diff / math.Max(math.Abs(fd), 1e-7); rel > worst {
				worst = rel
			}
			if diff > 1e-4*math.Abs(fd) && diff > 1e-7 {
				t.Fatalf("%s[%d]: gradient %.12g finite difference %.12g", group.name, i, group.grads[i], fd)
			}
		}
	}
	t.Logf("vector backward finite differences: counts weights=%d bias=%d log_tau=%d initial=%d inputs=%d, worst relative error %.3g",
		len(p.Weights), len(p.Bias), len(p.LogTau), len(initial), len(inputs)*nv, worst)
}

// TestVectorBackwardRejectsShapes checks the backward shape and trace guards:
// nil trace, wrong upstream row count, wrong upstream width and a negative
// window each return an error.
func TestVectorBackwardRejectsShapes(t *testing.T) {
	m, err := NewVectorContinuous(Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{0}, DT: 0.5, Activation: "tanh", StateDimension: 2, EdgeShape: "matrix"})
	if err != nil {
		t.Fatal(err)
	}
	const nv = 4
	p := VectorParameters{Weights: make([]float64, 4), Bias: make([]float64, nv), LogTau: []float64{0, 0}}
	inputs := [][]float64{{0.1, -0.1, 0.2, -0.2}, {-0.2, 0.1, 0.3, -0.1}}
	tr, err := m.Forward(context.Background(), p, make([]float64, nv), inputs)
	if err != nil {
		t.Fatal(err)
	}
	up := [][]float64{{0.3, -0.2, 0.1, 0.4}, {-0.1, 0.2, -0.3, 0.5}}
	if _, err := m.Backward(context.Background(), nil, up, 0); err == nil {
		t.Fatal("accepted nil trace")
	}
	if _, err := m.Backward(context.Background(), tr, [][]float64{{0.1, 0.2, 0.3, 0.4}}, 0); err == nil {
		t.Fatal("accepted wrong upstream row count")
	}
	if _, err := m.Backward(context.Background(), tr, [][]float64{{0.1, 0.2, 0.3}, {0.4, 0.5, 0.6}}, 0); err == nil {
		t.Fatal("accepted wrong upstream width")
	}
	if _, err := m.Backward(context.Background(), tr, up, -1); err == nil {
		t.Fatal("accepted negative window")
	}
}
