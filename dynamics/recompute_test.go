package dynamics

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"testing"
)

// recomputeFixture returns the ticket-25 stage-three fixture: five nodes, eight
// edges (a self loop plus delays 1, two 2s and one 3, maxDelay = 3), twenty
// three fixed-random input rows drawn from rand.NewPCG(1, 0) and a nonzero
// upstream row per step from rand.NewPCG(2, 0).
func recomputeFixture(t *testing.T) (*Continuous, Parameters, []float64, [][]float64, [][]float64) {
	t.Helper()
	m, err := NewContinuous(Config{
		Nodes:      5,
		Sources:    []int{0, 1, 2, 3, 0, 2, 4, 1},
		Targets:    []int{1, 2, 3, 4, 0, 4, 2, 0},
		Delays:     []int{0, 0, 0, 0, 2, 2, 3, 1},
		DT:         0.5,
		Activation: "tanh",
	})
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewPCG(1, 0))
	n := m.config.Nodes
	p := Parameters{
		Weights: make([]float64, len(m.config.Sources)),
		Bias:    make([]float64, n),
		LogTau:  make([]float64, n),
	}
	fill := func(v []float64) {
		for i := range v {
			v[i] = 2*rng.Float64() - 1
		}
	}
	fill(p.Weights)
	fill(p.Bias)
	fill(p.LogTau)
	initial := make([]float64, n)
	fill(initial)
	const steps = 23
	inputs := make([][]float64, steps)
	for s := range inputs {
		inputs[s] = make([]float64, n)
		fill(inputs[s])
	}
	ur := rand.New(rand.NewPCG(2, 0))
	upstream := make([][]float64, steps)
	for s := range upstream {
		upstream[s] = make([]float64, n)
		for i := range upstream[s] {
			upstream[s][i] = 2*ur.Float64() - 1
		}
	}
	return m, p, initial, inputs, upstream
}

func recomputeFullHistory(t *testing.T, m *Continuous, p Parameters, initial []float64, inputs, upstream [][]float64, window int) Gradient {
	t.Helper()
	tr, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	g, err := m.Backward(context.Background(), tr, upstream, window)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func recomputeBitEqual(t *testing.T, name string, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s length %d, want %d", name, len(got), len(want))
	}
	for i := range got {
		if math.Float64bits(got[i]) != math.Float64bits(want[i]) {
			t.Fatalf("%s[%d] got %.17g (%x) want %.17g (%x)", name, i, got[i], math.Float64bits(got[i]), want[i], math.Float64bits(want[i]))
		}
	}
}

func recomputeBitEqualRows(t *testing.T, name string, got, want [][]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s row count %d, want %d", name, len(got), len(want))
	}
	for i := range got {
		recomputeBitEqual(t, fmt.Sprintf("%s[%d]", name, i), got[i], want[i])
	}
}

func recomputeAssertGradient(t *testing.T, got, want Gradient) {
	t.Helper()
	recomputeBitEqual(t, "weights", got.Weights, want.Weights)
	recomputeBitEqual(t, "bias", got.Bias, want.Bias)
	recomputeBitEqual(t, "log_tau", got.LogTau, want.LogTau)
	recomputeBitEqual(t, "initial", got.Initial, want.Initial)
	recomputeBitEqualRows(t, "inputs", got.Inputs, want.Inputs)
}

// TestRecomputeMatchesFullHistoryBitForBit pins BackwardRecompute to
// Backward(Forward(...)) across segment sizes that split the delay window in
// different ways and across truncation windows.
func TestRecomputeMatchesFullHistoryBitForBit(t *testing.T) {
	m, p, initial, inputs, upstream := recomputeFixture(t)
	for _, segment := range []int{1, 4, 7, 23, 50} {
		for _, window := range []int{0, 3, 5} {
			want := recomputeFullHistory(t, m, p, initial, inputs, upstream, window)
			got, err := m.BackwardRecompute(context.Background(), p, initial, inputs, upstream, window, segment)
			if err != nil {
				t.Fatalf("segment %d window %d: %v", segment, window, err)
			}
			recomputeAssertGradient(t, got, want)
		}
	}
}

// TestRecomputeKeepsLessHistory checks the retained-history bound for a
// T = 256, N = 64, S = 16 run: the sum of all checkpoint rows and the largest
// single-segment recompute must stay under the segment-scaled bound and far
// below one full history, and the numbers are logged.
func TestRecomputeKeepsLessHistory(t *testing.T) {
	m, err := NewContinuous(Config{
		Nodes:      64,
		Sources:    []int{5, 32, 0, 63, 7, 1, 40, 12, 3, 55, 2, 9, 61, 21, 30, 44},
		Targets:    []int{12, 33, 0, 63, 8, 2, 41, 13, 4, 56, 3, 10, 62, 22, 31, 45},
		Delays:     []int{0, 0, 0, 0, 2, 2, 3, 1, 0, 0, 0, 0, 0, 0, 0, 0},
		DT:         0.3,
		Activation: "tanh",
	})
	if err != nil {
		t.Fatal(err)
	}
	maxDelay := 0
	for _, d := range m.config.Delays {
		if d > maxDelay {
			maxDelay = d
		}
	}
	n := m.config.Nodes
	rng := rand.New(rand.NewPCG(3, 0))
	p := Parameters{
		Weights: make([]float64, len(m.config.Sources)),
		Bias:    make([]float64, n),
		LogTau:  make([]float64, n),
	}
	fill := func(v []float64) {
		for i := range v {
			v[i] = 2*rng.Float64() - 1
		}
	}
	fill(p.Weights)
	fill(p.Bias)
	fill(p.LogTau)
	initial := make([]float64, n)
	fill(initial)
	const steps, nrows, segment = 256, 64, 16
	if n != nrows {
		t.Fatalf("nodes %d, want %d", n, nrows)
	}
	inputs := make([][]float64, steps)
	for s := range inputs {
		inputs[s] = make([]float64, n)
		fill(inputs[s])
	}
	upstream := make([][]float64, steps)
	for s := range upstream {
		upstream[s] = make([]float64, n)
		fill(upstream[s])
	}
	_, stats, err := m.BackwardRecomputeStats(context.Background(), p, initial, inputs, upstream, 0, segment)
	if err != nil {
		t.Fatal(err)
	}
	bound := (steps/segment)*(maxDelay+1) + segment + maxDelay + 1
	kept := stats.CheckpointRows + stats.MaxSegmentRows
	if stats.Steps != steps || stats.Segment != segment || stats.Checkpoints != steps/segment {
		t.Fatalf("stats %+v", stats)
	}
	if kept > bound {
		t.Fatalf("retained %d rows exceeds bound %d (maxDelay %d, stats %+v)", kept, bound, maxDelay, stats)
	}
	if kept >= stats.FullHistoryRows {
		t.Fatalf("retained %d rows is not clearly below one full history (%d)", kept, stats.FullHistoryRows)
	}
	t.Logf("recompute memory: checkpoints=%d checkpointRows=%d maxSegmentRows=%d kept=%d bound=%d fullHistoryRows=%d",
		stats.Checkpoints, stats.CheckpointRows, stats.MaxSegmentRows, kept, bound, stats.FullHistoryRows)
}

// TestRecomputeWithoutDelays pins the maxDelay = 0 boundary: no delays in the
// graph, checkpoints hold one row, and the gradient still matches the full
// history bit for bit.
func TestRecomputeWithoutDelays(t *testing.T) {
	m, err := NewContinuous(Config{
		Nodes:      4,
		Sources:    []int{0, 1, 2, 3},
		Targets:    []int{1, 2, 3, 0},
		DT:         0.4,
		Activation: "tanh",
	})
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewPCG(4, 0))
	n := m.config.Nodes
	p := Parameters{
		Weights: make([]float64, len(m.config.Sources)),
		Bias:    make([]float64, n),
		LogTau:  make([]float64, n),
	}
	fill := func(v []float64) {
		for i := range v {
			v[i] = 2*rng.Float64() - 1
		}
	}
	fill(p.Weights)
	fill(p.Bias)
	fill(p.LogTau)
	initial := make([]float64, n)
	fill(initial)
	const steps = 9
	inputs := make([][]float64, steps)
	for s := range inputs {
		inputs[s] = make([]float64, n)
		fill(inputs[s])
	}
	ur := rand.New(rand.NewPCG(5, 0))
	upstream := make([][]float64, steps)
	for s := range upstream {
		upstream[s] = make([]float64, n)
		for i := range upstream[s] {
			upstream[s][i] = 2*ur.Float64() - 1
		}
	}
	for _, segment := range []int{1, 4, 9} {
		for _, window := range []int{0, 5} {
			want := recomputeFullHistory(t, m, p, initial, inputs, upstream, window)
			got, err := m.BackwardRecompute(context.Background(), p, initial, inputs, upstream, window, segment)
			if err != nil {
				t.Fatalf("segment %d window %d: %v", segment, window, err)
			}
			recomputeAssertGradient(t, got, want)
		}
	}
}

// TestRecomputeRejects checks the recompute-specific error paths: a nonpositive
// segment, a mismatched upstream length, and a cancelled context.
func TestRecomputeRejects(t *testing.T) {
	m, p, initial, inputs, upstream := recomputeFixture(t)
	if _, err := m.BackwardRecompute(context.Background(), p, initial, inputs, upstream, 0, 0); err == nil {
		t.Fatal("accepted segment 0")
	}
	if _, err := m.BackwardRecompute(context.Background(), p, initial, inputs, upstream[:len(upstream)-1], 0, 1); err == nil {
		t.Fatal("accepted wrong upstream row count")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.BackwardRecompute(ctx, p, initial, inputs, upstream, 0, 1); err == nil {
		t.Fatal("accepted canceled context")
	}
}
