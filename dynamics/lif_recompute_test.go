package dynamics

import (
	"context"
	"math"
	"math/rand/v2"
	"testing"
)

// lifRecomputeUpstream builds one uniform [-1, 1) upstream row per input step,
// the same draw the whole recompute suite uses so cross tests share the seed.
func lifRecomputeUpstream(t *testing.T, steps, n int) [][]float64 {
	t.Helper()
	ur := rand.New(rand.NewPCG(2, 0))
	upstream := make([][]float64, steps)
	for s := range upstream {
		upstream[s] = make([]float64, n)
		for i := range upstream[s] {
			upstream[s][i] = 2*ur.Float64() - 1
		}
	}
	return upstream
}

// lifRecomputeFullHistory runs the reference path Backward(Forward(...)) that
// BackwardRecompute must reproduce bit for bit.
func lifRecomputeFullHistory(t *testing.T, m *LIF, p LIFParameters, initial []float64, inputs, upstream [][]float64, window int) LIFGradient {
	t.Helper()
	tr, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	g, err := m.Backward(context.Background(), tr, upstream, window)
	if err != nil {
		t.Fatalf("Backward: %v", err)
	}
	return g
}

// lifRecomputeCheckRow compares two node rows by raw float bits and names the
// setting, segment, window, field and position of a mismatch.
func lifRecomputeCheckRow(t *testing.T, setting string, segment, window int, name string, got, want []float64, at string) {
	t.Helper()
	for i := range want {
		if math.Float64bits(got[i]) != math.Float64bits(want[i]) {
			t.Fatalf("setting %q segment=%d window=%d %s%s[%d]: bits got %#016x want %#016x (%g vs %g)",
				setting, segment, window, name, at, i, math.Float64bits(got[i]), math.Float64bits(want[i]), got[i], want[i])
		}
	}
}

// lifRecomputeAssertGradient compares every LIFGradient field by float bits.
func lifRecomputeAssertGradient(t *testing.T, setting string, segment, window int, got, want LIFGradient) {
	t.Helper()
	lifRecomputeCheckRow(t, setting, segment, window, "weights", got.Weights, want.Weights, "")
	lifRecomputeCheckRow(t, setting, segment, window, "bias", got.Bias, want.Bias, "")
	lifRecomputeCheckRow(t, setting, segment, window, "log_tau", got.LogTau, want.LogTau, "")
	lifRecomputeCheckRow(t, setting, segment, window, "theta_raw", got.ThetaRaw, want.ThetaRaw, "")
	lifRecomputeCheckRow(t, setting, segment, window, "initial", got.Initial, want.Initial, "")
	if len(got.Inputs) != len(want.Inputs) {
		t.Fatalf("setting %q segment=%d window=%d inputs row count %d, want %d", setting, segment, window, len(got.Inputs), len(want.Inputs))
	}
	for step, row := range want.Inputs {
		lifRecomputeCheckRow(t, setting, segment, window, "inputs", got.Inputs[step], row, "[t]")
	}
}

func lifRecomputeSettings(t *testing.T) []struct {
	name string
	cfg  LIFConfig
	p    LIFParameters
	init []float64
	ins  [][]float64
} {
	t.Helper()
	base, p, initial, inputs := lifSegmentFixture()
	return []struct {
		name string
		cfg  LIFConfig
		p    LIFParameters
		init []float64
		ins  [][]float64
	}{
		{"basic", base, p, initial, inputs},
		{"adaptation", func() LIFConfig {
			c := base
			c.Adaptation = LIFAdaptation{Enabled: true, TauAdapt: .9, Beta: .4}
			return c
		}(), p, initial, inputs},
		{"homeostasis", func() LIFConfig {
			c := base
			c.Homeostasis = &LIFHomeostasis{Enabled: true, TauRate: 5, TargetRate: .2, Eta: .05, HMax: 5}
			return c
		}(), p, initial, inputs},
		{"workers2", func() LIFConfig { c := base; c.Workers = 2; return c }(), p, initial, inputs},
	}
}

// TestLIFRecomputeMatchesFullHistoryBitForBit pins BackwardRecompute to
// Backward(Forward(...)) across the shared fixture settings, segment sizes that
// split the delay window (maxDelay = 2) in different ways and truncation
// windows.
func TestLIFRecomputeMatchesFullHistoryBitForBit(t *testing.T) {
	settings := lifRecomputeSettings(t)
	upstream := lifRecomputeUpstream(t, len(settings[0].ins), settings[0].cfg.Nodes)
	for _, st := range settings {
		m, err := NewLIF(st.cfg)
		if err != nil {
			t.Fatalf("setting %q: NewLIF: %v", st.name, err)
		}
		for _, segment := range []int{1, 4, 7, 23, 50} {
			for _, window := range []int{0, 3, 5} {
				want := lifRecomputeFullHistory(t, m, st.p, st.init, st.ins, upstream, window)
				got, err := m.BackwardRecompute(context.Background(), st.p, st.init, st.ins, upstream, window, segment)
				if err != nil {
					t.Fatalf("setting %q segment=%d window=%d: BackwardRecompute: %v", st.name, segment, window, err)
				}
				lifRecomputeAssertGradient(t, st.name, segment, window, got, want)
			}
		}
	}
}

// TestLIFRecomputeSmoothModeMatches pins the smooth reference mode, which
// changes the event, the reset coefficient of the reverse pass and therefore
// the whole gradient, to the same bit-for-bit identity.
func TestLIFRecomputeSmoothModeMatches(t *testing.T) {
	base, p, initial, inputs := lifSegmentFixture()
	upstream := lifRecomputeUpstream(t, len(inputs), base.Nodes)
	m, err := NewLIF(base)
	if err != nil {
		t.Fatalf("NewLIF: %v", err)
	}
	m.smooth = true
	want := lifRecomputeFullHistory(t, m, p, initial, inputs, upstream, 0)
	got, err := m.BackwardRecompute(context.Background(), p, initial, inputs, upstream, 0, 3)
	if err != nil {
		t.Fatalf("BackwardRecompute: %v", err)
	}
	lifRecomputeAssertGradient(t, "smooth", 3, 0, got, want)
}

// TestLIFRecomputeKeepsLessHistory checks the retained-history bound for a
// T = 256, N = 64, S = 16 run over a graph with three incoming edges per node
// and delays {0, 1, 2} in rotation (maxDelay = 2): the total checkpoint rows
// and the largest single-segment recompute stay under their segment-scaled
// bounds, and a full history would have carried 257 rows.
func TestLIFRecomputeKeepsLessHistory(t *testing.T) {
	const nodes, steps, segment = 64, 256, 16
	const maxDelay = 2
	sr := rand.New(rand.NewPCG(3, 0))
	edgeCount := nodes * 3
	sources := make([]int, edgeCount)
	for e := range sources {
		sources[e] = sr.IntN(nodes)
	}
	targets, delays := make([]int, edgeCount), make([]int, edgeCount)
	for i := 0; i < nodes; i++ {
		for j := 0; j < 3; j++ {
			e := i*3 + j
			targets[e] = i
			delays[e] = j
		}
	}
	cfg := lifSegmentBaseConfig()
	cfg.Nodes = nodes
	cfg.Sources, cfg.Targets, cfg.Delays = sources, targets, delays
	m, err := NewLIF(cfg)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewPCG(3, 1))
	p := LIFParameters{
		Weights:  make([]float64, edgeCount),
		Bias:     make([]float64, nodes),
		LogTau:   make([]float64, nodes),
		ThetaRaw: make([]float64, nodes),
	}
	for _, v := range [][]float64{p.Weights, p.Bias, p.LogTau, p.ThetaRaw} {
		for i := range v {
			v[i] = 2*rng.Float64() - 1
		}
	}
	initial := make([]float64, nodes)
	for i := range initial {
		initial[i] = 2*rng.Float64() - 1
	}
	inputs := make([][]float64, steps)
	upstream := make([][]float64, steps)
	for s := 0; s < steps; s++ {
		inputs[s], upstream[s] = make([]float64, nodes), make([]float64, nodes)
		for i := 0; i < nodes; i++ {
			inputs[s][i] = 2*rng.Float64() - 1
			upstream[s][i] = 2*rng.Float64() - 1
		}
	}
	_, stats, err := m.BackwardRecomputeStats(context.Background(), p, initial, inputs, upstream, 0, segment)
	if err != nil {
		t.Fatal(err)
	}
	if stats.CheckpointRows > (steps/segment)*(maxDelay+1) {
		t.Fatalf("checkpoint rows %d exceeds bound %d (stats %+v)", stats.CheckpointRows, (steps/segment)*(maxDelay+1), stats)
	}
	if stats.MaxSegmentRows > segment+maxDelay+1 {
		t.Fatalf("max segment rows %d exceeds bound %d (stats %+v)", stats.MaxSegmentRows, segment+maxDelay+1, stats)
	}
	if stats.FullHistoryRows != steps+1 {
		t.Fatalf("full history rows %d, want %d", stats.FullHistoryRows, steps+1)
	}
	t.Logf("recompute memory: checkpoints=%d checkpointRows=%d maxSegmentRows=%d fullHistoryRows=%d",
		stats.Checkpoints, stats.CheckpointRows, stats.MaxSegmentRows, stats.FullHistoryRows)
}

// TestLIFRecomputeRejects checks the recompute-specific error paths: a
// nonpositive segment, a negative window, a mismatched upstream row count, a
// cancelled context and a truncated weights vector.
func TestLIFRecomputeRejects(t *testing.T) {
	base, p, initial, inputs := lifSegmentFixture()
	upstream := lifRecomputeUpstream(t, len(inputs), base.Nodes)
	m, err := NewLIF(base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.BackwardRecompute(context.Background(), p, initial, inputs, upstream, 0, 0); err == nil {
		t.Fatal("accepted segment 0")
	}
	if _, err := m.BackwardRecompute(context.Background(), p, initial, inputs, upstream, -1, 1); err == nil {
		t.Fatal("accepted window -1")
	}
	if _, err := m.BackwardRecompute(context.Background(), p, initial, inputs, upstream[:len(upstream)-1], 0, 1); err == nil {
		t.Fatal("accepted wrong upstream row count")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.BackwardRecompute(ctx, p, initial, inputs, upstream, 0, 1); err == nil {
		t.Fatal("accepted canceled context")
	}
	truncated := p
	truncated.Weights = truncated.Weights[:len(truncated.Weights)-1]
	if _, err := m.BackwardRecompute(context.Background(), truncated, initial, inputs, upstream, 0, 1); err == nil {
		t.Fatal("accepted truncated weights")
	}
}
