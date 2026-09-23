package learning

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
)

const recomputeNodes = 5

// recomputeEdgeGraph is the 5 node, 8 edge fixture of ticket 25's third stage:
// every pair of consecutive nodes is connected, the ring returns through edges
// 4 and 6, and edge 3 carries a delay of 2 so the recompute checkpoints need
// more than one history row.
func recomputeEdgeGraph() (sources, targets, delays []int) {
	return []int{0, 1, 2, 3, 4, 1, 3, 0},
		[]int{1, 2, 3, 4, 0, 3, 1, 4},
		[]int{1, 0, 1, 2, 0, 1, 0, 1}
}

func continuousRecomputeConfig() Config {
	s, t, d := recomputeEdgeGraph()
	return Config{
		Dynamics:     dynamics.Config{Nodes: recomputeNodes, Sources: s, Targets: t, Delays: d, DT: .5, Activation: "tanh"},
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: []int{0},
	}
}

func lifRecomputeConfig() Config {
	s, t, d := recomputeEdgeGraph()
	lif := dynamics.LIFConfig{
		Nodes: recomputeNodes, Sources: s, Targets: t, Delays: d,
		DT: .5, TauSyn: .7, ThetaMin: .3, ThetaMax: 1.4, VReset: -.5,
		RefractorySteps: 1,
		Adaptation:      dynamics.LIFAdaptation{Enabled: true, TauAdapt: .9, Beta: .4},
		Surrogate:       dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 1.7},
	}
	return Config{LIF: &lif, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{0}}
}

func recomputeRandVec(seed, stream uint64, n int) []float64 {
	r := rand.New(rand.NewPCG(seed, stream))
	out := make([]float64, n)
	for i := range out {
		out[i] = r.Float64()
	}
	return out
}

func continuousRecomputeParameters() Parameters {
	return Parameters{Core: dynamics.Parameters{
		Weights: recomputeRandVec(1, 0, 8),
		Bias:    recomputeRandVec(1, 0, recomputeNodes),
		LogTau:  recomputeRandVec(1, 0, recomputeNodes),
	}}
}

func lifRecomputeParameters() Parameters {
	return Parameters{
		Core: dynamics.Parameters{
			Weights: recomputeRandVec(1, 0, 8),
			Bias:    recomputeRandVec(1, 0, recomputeNodes),
			LogTau:  recomputeRandVec(1, 0, recomputeNodes),
		},
		ThetaRaw: recomputeRandVec(1, 0, recomputeNodes),
	}
}

// recomputeInputs draws the 23 step fixture rows from PCG(3, 0); the LIF core
// scales them to [0, 3) so the membrane really crosses the threshold.
func recomputeInputs(scale float64) [][]float64 {
	r := rand.New(rand.NewPCG(3, 0))
	rows := make([][]float64, 23)
	for i := range rows {
		row := make([]float64, recomputeNodes)
		for j := range row {
			row[j] = r.Float64() * scale
		}
		rows[i] = row
	}
	return rows
}

// recomputeUpstream draws the 23 step upstream fixture rows from PCG(2, 0) and
// shifts them to [1, 2) so no entry is zero.
func recomputeUpstream() [][]float64 {
	r := rand.New(rand.NewPCG(2, 0))
	rows := make([][]float64, 23)
	for i := range rows {
		row := make([]float64, recomputeNodes)
		for j := range row {
			row[j] = 1 + r.Float64()
		}
		rows[i] = row
	}
	return rows
}

func vectorRecomputeConfig() Config {
	return Config{
		Dynamics:     dynamics.Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{0}, DT: .5, Activation: "tanh", StateDimension: 2},
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: []int{1},
	}
}

type recomputeFixture struct {
	name   string
	core   coreModel
	params Parameters
	inputs [][]float64
}

func recomputeFixtures(t *testing.T) []recomputeFixture {
	t.Helper()
	cont, err := NewNetwork(continuousRecomputeConfig())
	if err != nil {
		t.Fatal(err)
	}
	lif, err := NewNetwork(lifRecomputeConfig())
	if err != nil {
		t.Fatal(err)
	}
	return []recomputeFixture{
		{"continuous", cont.core, continuousRecomputeParameters(), recomputeInputs(1)},
		{"lif", lif.core, lifRecomputeParameters(), recomputeInputs(3)},
	}
}

func assertRecomputeVecEqual(t *testing.T, label string, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: length %d, want %d", label, len(got), len(want))
	}
	for i := range want {
		if math.Float64bits(got[i]) != math.Float64bits(want[i]) {
			t.Fatalf("%s[%d]: got %g (bits %x), want %g (bits %x)", label, i, got[i], math.Float64bits(got[i]), want[i], math.Float64bits(want[i]))
		}
	}
}

func assertRecomputeRowsEqual(t *testing.T, label string, got, want [][]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d rows, want %d", label, len(got), len(want))
	}
	for r := range want {
		if len(got[r]) != len(want[r]) {
			t.Fatalf("%s: row %d has length %d, want %d", label, r, len(got[r]), len(want[r]))
		}
		for i := range want[r] {
			if math.Float64bits(got[r][i]) != math.Float64bits(want[r][i]) {
				t.Fatalf("%s: row %d col %d: got %g (bits %x), want %g (bits %x)", label, r, i, got[r][i], math.Float64bits(got[r][i]), want[r][i], math.Float64bits(want[r][i]))
			}
		}
	}
}

func assertRecomputeGradientEqual(t *testing.T, label string, got, want coreGradient) {
	t.Helper()
	assertRecomputeVecEqual(t, label+"/weights", got.core.Weights, want.core.Weights)
	assertRecomputeVecEqual(t, label+"/bias", got.core.Bias, want.core.Bias)
	assertRecomputeVecEqual(t, label+"/log_tau", got.core.LogTau, want.core.LogTau)
	assertRecomputeVecEqual(t, label+"/initial", got.core.Initial, want.core.Initial)
	assertRecomputeRowsEqual(t, label+"/inputs", got.core.Inputs, want.core.Inputs)
	assertRecomputeVecEqual(t, label+"/theta_raw", got.thetaRaw, want.thetaRaw)
}

func TestRecomputeObserveMatchesForward(t *testing.T) {
	ctx := context.Background()
	for _, fx := range recomputeFixtures(t) {
		initial := make([]float64, recomputeNodes)
		_, want, err := fx.core.forward(ctx, fx.params, initial, fx.inputs)
		if err != nil {
			t.Fatalf("%s: forward: %v", fx.name, err)
		}
		for _, segment := range []int{1, 4, 7, 50} {
			got, err := observe(ctx, fx.core, fx.params, initial, fx.inputs, segment)
			if err != nil {
				t.Fatalf("%s segment %d: observe: %v", fx.name, segment, err)
			}
			assertRecomputeRowsEqual(t, fmt.Sprintf("%s observe segment %d", fx.name, segment), got, want)
		}
	}
}

func TestRecomputeBackwardMatchesFullHistory(t *testing.T) {
	ctx := context.Background()
	upstream := recomputeUpstream()
	for _, fx := range recomputeFixtures(t) {
		initial := make([]float64, recomputeNodes)
		trace, _, err := fx.core.forward(ctx, fx.params, initial, fx.inputs)
		if err != nil {
			t.Fatalf("%s: forward: %v", fx.name, err)
		}
		for _, window := range []int{0, 3} {
			want, err := fx.core.backward(ctx, trace, upstream, window)
			if err != nil {
				t.Fatalf("%s window %d: backward: %v", fx.name, window, err)
			}
			for _, segment := range []int{1, 4, 7} {
				got, err := backwardRecompute(ctx, fx.core, fx.params, initial, fx.inputs, upstream, window, segment)
				if err != nil {
					t.Fatalf("%s window %d segment %d: backwardRecompute: %v", fx.name, window, segment, err)
				}
				assertRecomputeGradientEqual(t, fmt.Sprintf("%s window %d segment %d", fx.name, window, segment), got, want)
			}
		}
	}
}

func TestRecomputeRejects(t *testing.T) {
	ctx := context.Background()

	t.Run("zero segment", func(t *testing.T) {
		cont, err := NewNetwork(continuousRecomputeConfig())
		if err != nil {
			t.Fatal(err)
		}
		initial := make([]float64, recomputeNodes)
		inputs := recomputeInputs(1)
		if _, err := observe(ctx, cont.core, continuousRecomputeParameters(), initial, inputs, 0); err == nil {
			t.Fatal("observe accepted segment 0")
		}
		if _, err := backwardRecompute(ctx, cont.core, continuousRecomputeParameters(), initial, inputs, recomputeUpstream(), 0, 0); err == nil {
			t.Fatal("backwardRecompute accepted segment 0")
		}
	})

	t.Run("vector core", func(t *testing.T) {
		n, err := NewNetwork(vectorRecomputeConfig())
		if err != nil {
			t.Fatal(err)
		}
		err = recomputeSupported(n.core)
		if err == nil {
			t.Fatal("recomputeSupported accepted the vector core")
		}
		if !strings.Contains(err.Error(), "not vector") {
			t.Fatalf("error %q does not mention the vector core", err)
		}
	})

	t.Run("mixed core", func(t *testing.T) {
		n, err := NewNetwork(mixedIndividualConfig())
		if err != nil {
			t.Fatal(err)
		}
		initial := make([]float64, 3)
		inputs := [][]float64{{0, 0, 0}, {0, 0, 0}}
		upstream := [][]float64{{1, 1, 1}, {1, 1, 1}}
		p := mixedIndividualParameters()
		if err := recomputeSupported(n.core); err == nil || !strings.Contains(err.Error(), "recompute supports the continuous and LIF cores") {
			t.Fatalf("recomputeSupported: %v", err)
		}
		if _, err := observe(ctx, n.core, p, initial, inputs, 2); err == nil || !strings.Contains(err.Error(), "recompute supports the continuous and LIF cores") {
			t.Fatalf("observe: %v", err)
		}
		if _, err := backwardRecompute(ctx, n.core, p, initial, inputs, upstream, 0, 2); err == nil || !strings.Contains(err.Error(), "recompute supports the continuous and LIF cores") {
			t.Fatalf("backwardRecompute: %v", err)
		}
	})

	t.Run("continuous theta raw", func(t *testing.T) {
		cont, err := NewNetwork(continuousRecomputeConfig())
		if err != nil {
			t.Fatal(err)
		}
		p := continuousRecomputeParameters()
		p.ThetaRaw = make([]float64, recomputeNodes)
		initial := make([]float64, recomputeNodes)
		inputs := recomputeInputs(1)
		if _, err := observe(ctx, cont.core, p, initial, inputs, 2); err == nil {
			t.Fatal("observe accepted theta_raw on the continuous core")
		}
		if _, err := backwardRecompute(ctx, cont.core, p, initial, inputs, recomputeUpstream(), 0, 2); err == nil {
			t.Fatal("backwardRecompute accepted theta_raw on the continuous core")
		}
	})
}
