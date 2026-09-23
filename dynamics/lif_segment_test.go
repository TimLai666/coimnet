package dynamics

import (
	"context"
	"math"
	"math/rand/v2"
	"testing"
)

// lifSegmentBaseConfig is the shared 5-node fixture topology: 8 edges with the
// last one a self connection, delays {0,1,0,2,0,2,1,0} so maxDelay = 2.
func lifSegmentBaseConfig() LIFConfig {
	return LIFConfig{
		Nodes:           5,
		Sources:         []int{0, 1, 2, 3, 4, 0, 2, 1},
		Targets:         []int{1, 2, 3, 4, 0, 2, 4, 1},
		Delays:          []int{0, 1, 0, 2, 0, 2, 1, 0},
		DT:              .5,
		TauSyn:          .7,
		ThetaMin:        .3,
		ThetaMax:        1.4,
		VReset:          -.5,
		RefractorySteps: 1,
		Surrogate:       LIFSurrogate{Kind: "fast_sigmoid", Scale: 1.7},
		Workers:         0,
	}
}

// lifSegmentFixture builds the deterministic parameters, initial voltage and a
// T = 23 row input sequence for the shared topology.
func lifSegmentFixture() (LIFConfig, LIFParameters, []float64, [][]float64) {
	const n, e, T = 5, 8, 23
	cfg := lifSegmentBaseConfig()
	wr := rand.New(rand.NewPCG(1, 0))
	weights := make([]float64, e)
	for i := range weights {
		weights[i] = -1.5 + 3*wr.Float64()
	}
	p := LIFParameters{
		Weights:  weights,
		Bias:     []float64{0.1, 0.1, 0.1, 0.1, 0.1},
		LogTau:   []float64{0.2, 0.2, 0.2, 0.2, 0.2},
		ThetaRaw: []float64{0, 0, 0, 0, 0},
	}
	initial := []float64{0.2, 0.2, 0.2, 0.2, 0.2}
	ir := rand.New(rand.NewPCG(1, 1))
	inputs := make([][]float64, T)
	for t := range inputs {
		inputs[t] = make([]float64, n)
		for j := range inputs[t] {
			inputs[t][j] = 3 * ir.Float64()
		}
	}
	return cfg, p, initial, inputs
}

func lifSegmentMaxDelay(delays []int) int {
	maxDelay := 0
	for _, d := range delays {
		if d > maxDelay {
			maxDelay = d
		}
	}
	return maxDelay
}

func lifSegCheckRow(t *testing.T, setting string, size int, name string, segRow, trRow []float64, row int) {
	t.Helper()
	for i := range trRow {
		if math.Float64bits(segRow[i]) != math.Float64bits(trRow[i]) {
			t.Fatalf("setting %q segment=%d %s row=%d neuron=%d: bits got %#016x want %#016x (%g vs %g)",
				setting, size, name, row, i, math.Float64bits(segRow[i]), math.Float64bits(trRow[i]), segRow[i], trRow[i])
		}
	}
}

func lifSegCheckRefractRow(t *testing.T, setting string, size int, segRow, trRow []int, row int) {
	t.Helper()
	for i := range trRow {
		if segRow[i] != trRow[i] {
			t.Fatalf("setting %q segment=%d refract row=%d neuron=%d: got %d want %d",
				setting, size, row, i, segRow[i], trRow[i])
		}
	}
}

func TestLIFSegmentReplaysForwardBitForBit(t *testing.T) {
	base, p, initial, inputs := lifSegmentFixture()
	T := len(inputs)
	settings := []struct {
		name string
		cfg  LIFConfig
	}{
		{"basic", base},
		{"adaptation", func() LIFConfig {
			c := base
			c.Adaptation = LIFAdaptation{Enabled: true, TauAdapt: .9, Beta: .4}
			return c
		}()},
		{"homeostasis", func() LIFConfig {
			c := base
			c.Homeostasis = &LIFHomeostasis{Enabled: true, TauRate: 5, TargetRate: .2, Eta: .05, HMax: 5}
			return c
		}()},
		{"workers2", func() LIFConfig { c := base; c.Workers = 2; return c }()},
	}
	for _, st := range settings {
		m, err := NewLIF(st.cfg)
		if err != nil {
			t.Fatalf("setting %q: NewLIF: %v", st.name, err)
		}
		lambda, alpha, thetaBase, _, err := m.lifConstants(p)
		if err != nil {
			t.Fatalf("setting %q: lifConstants: %v", st.name, err)
		}
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			t.Fatalf("setting %q: Forward: %v", st.name, err)
		}
		if st.name == "basic" {
			spikes := 0
			for t := 1; t <= T; t++ {
				for _, s := range tr.spike[t] {
					if s != 0 {
						spikes++
					}
				}
			}
			if spikes < 5 {
				t.Fatalf("setting %q: fixture must fire at least 5 spikes, got %d", st.name, spikes)
			}
			sawRefract := false
			for t := 1; t <= T && !sawRefract; t++ {
				for _, v := range tr.refract[t] {
					if v > 0 {
						sawRefract = true
						break
					}
				}
			}
			if !sawRefract {
				t.Fatalf("setting %q: fixture must step into refract at least once", st.name)
			}
		}
		maxDelay := lifSegmentMaxDelay(st.cfg.Delays)
		for _, size := range []int{1, 4, 7, 23} {
			cp := m.lifInitialCheckpoint(initial)
			for start := 0; start < T; {
				end := min(start+size, T)
				seg, err := m.lifRunSegment(context.Background(), p, inputs, cp, end, lambda, alpha, thetaBase)
				if err != nil {
					t.Fatalf("setting %q segment=%d start=%d: lifRunSegment: %v", st.name, size, start, err)
				}
				for r := seg.Start + 1; r <= seg.End; r++ {
					rel := r - seg.Start
					lifSegCheckRow(t, st.name, size, "voltage", seg.voltage[rel], tr.voltage[r], r)
					lifSegCheckRow(t, st.name, size, "adapt", seg.adapt[rel], tr.adapt[r], r)
					lifSegCheckRow(t, st.name, size, "rate", seg.rate[rel], tr.rate[r], r)
					lifSegCheckRow(t, st.name, size, "homeo", seg.homeo[rel], tr.homeo[r], r)
					lifSegCheckRow(t, st.name, size, "spike", seg.spike[rel], tr.spike[r], r)
					lifSegCheckRefractRow(t, st.name, size, seg.refract[rel], tr.refract[r], r)
					lifSegCheckRow(t, st.name, size, "syn", seg.SynRow(r), tr.syn[r], r)
				}
				for s := seg.Start; s < seg.End; s++ {
					cand, drive, reset, dspike := seg.Step(s)
					lifSegCheckRow(t, st.name, size, "cand", cand, tr.cand[s], s)
					lifSegCheckRow(t, st.name, size, "drive", drive, tr.drive[s], s)
					lifSegCheckRow(t, st.name, size, "reset", reset, tr.reset[s], s)
					lifSegCheckRow(t, st.name, size, "dspike", dspike, tr.dspike[s], s)
				}
				if end < T {
					cp = seg.checkpointAt(end, maxDelay)
				}
				start = end
			}
		}
	}
}

func TestLIFSegmentChecksContext(t *testing.T) {
	base, p, initial, inputs := lifSegmentFixture()
	m, err := NewLIF(base)
	if err != nil {
		t.Fatalf("NewLIF: %v", err)
	}
	lambda, alpha, thetaBase, _, err := m.lifConstants(p)
	if err != nil {
		t.Fatalf("lifConstants: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.lifRunSegment(ctx, p, inputs, m.lifInitialCheckpoint(initial), 1, lambda, alpha, thetaBase); err == nil {
		t.Fatalf("cancelled context must make lifRunSegment error, got nil")
	}
}
