package dynamics

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/TimLai666/coimnet/backend/webgpu"
)

type zeroTauSequence struct {
	mods  []*Modulation
	nodes int
}

func (s *zeroTauSequence) Forward(ctx context.Context, step int, previous []float64) (*Modulation, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || step < 0 || step >= len(s.mods) || len(previous) != s.nodes {
		return nil, fmt.Errorf("invalid zero-tau sequence step")
	}
	return s.mods[step], nil
}

func (s *zeroTauSequence) NewReverse(ctx context.Context, steps, window int) (ModulationReverse, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || steps != len(s.mods) || window < 0 {
		return nil, fmt.Errorf("invalid zero-tau reverse shape")
	}
	return &zeroTauReverse{}, nil
}

type zeroTauReverse struct{}

func (r *zeroTauReverse) BackwardStep(ctx context.Context, step int, gradient ModulationGradient) ([]ActivityGradient, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, nil
}

type zeroTauRecomputeSequence struct {
	mods  []*Modulation
	nodes int
}

func (s *zeroTauRecomputeSequence) Forward(ctx context.Context, step int, previous []float64) (*Modulation, error) {
	return (&zeroTauSequence{mods: s.mods, nodes: s.nodes}).Forward(ctx, step, previous)
}

func (s *zeroTauRecomputeSequence) NewReverse(ctx context.Context, steps, window int) (ModulationReverse, error) {
	return (&zeroTauSequence{mods: s.mods, nodes: s.nodes}).NewReverse(ctx, steps, window)
}

func (s *zeroTauRecomputeSequence) Checkpoint(ctx context.Context, nextStep int) (ModulationRestart, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || nextStep < 0 || nextStep >= len(s.mods) {
		return nil, fmt.Errorf("invalid zero-tau checkpoint")
	}
	replay := append([]*Modulation(nil), s.mods[nextStep:]...)
	return func(restartCtx context.Context) (ModulationSequence, error) {
		if restartCtx == nil {
			return nil, fmt.Errorf("nil zero-tau restart context")
		}
		if err := restartCtx.Err(); err != nil {
			return nil, err
		}
		return &zeroTauSequence{mods: replay, nodes: s.nodes}, nil
	}, nil
}

func zeroTauMods(steps int) []*Modulation {
	mods := make([]*Modulation, steps)
	for i := range mods {
		mods[i] = &Modulation{}
	}
	return mods
}

func zeroTauLIFFixture(t *testing.T) (*LIF, LIFParameters, []float64, [][]float64, [][]float64) {
	t.Helper()
	m, err := NewLIF(LIFConfig{
		Nodes: 1, DT: math.Log(2), TauSyn: 1, ThetaMin: 0, ThetaMax: 2,
		VReset: -1e308, RefractorySteps: 1,
		Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	return m, LIFParameters{Bias: []float64{0}, LogTau: []float64{0}, ThetaRaw: []float64{0}},
		[]float64{0}, [][]float64{{3}, {1e308}}, [][]float64{{0}, {1}}
}

func zeroTauMixedLIFFixture(t *testing.T) (*Mixed, MixedParameters, []float64, [][]float64, [][]float64) {
	t.Helper()
	m, err := NewMixed(MixedConfig{
		Nodes: 1, DT: math.Log(2), NodeRule: []uint8{RuleLIF},
		LIF: LIFRule{TauSyn: 1, ThetaMin: 0, ThetaMax: 2, VReset: -1e308,
			RefractorySteps: 1, Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return m, MixedParameters{Bias: []float64{0}, LogTau: []float64{0}, ThetaRaw: []float64{0}},
		[]float64{0}, [][]float64{{3}, {1e308}}, [][]float64{{0}, {1}}
}

func zeroTauContinuousFixture(t *testing.T) (*Continuous, Parameters, []float64, [][]float64) {
	t.Helper()
	m, err := NewContinuous(Config{Nodes: 1, DT: math.Log(2), Activation: "tanh"})
	if err != nil {
		t.Fatal(err)
	}
	return m, Parameters{Bias: []float64{0}, LogTau: []float64{0}}, []float64{-1e308}, [][]float64{{1e308}}
}

func zeroTauAssertLIFForward(t *testing.T, outputs, spikes [][]float64) {
	t.Helper()
	if len(outputs) != 2 || len(spikes) != 2 {
		t.Fatalf("LIF rows outputs=%d spikes=%d, want 2", len(outputs), len(spikes))
	}
	for row := 0; row < 2; row++ {
		if len(outputs[row]) != 1 || len(spikes[row]) != 1 {
			t.Fatalf("LIF row %d lengths outputs=%d spikes=%d, want 1", row, len(outputs[row]), len(spikes[row]))
		}
	}
	if outputs[0][0] != 1 || outputs[1][0] != .5 || spikes[0][0] != 1 || spikes[1][0] != 0 {
		t.Fatalf("LIF outputs=%v spikes=%v, want [[1] [.5]] and [[1] [0]]", outputs, spikes)
	}
}

func zeroTauAssertLIFGradient(t *testing.T, g LIFGradient) {
	t.Helper()
	wantTau := -3 * math.Log(2) / 16
	if len(g.Weights) != 0 || len(g.Bias) != 1 || len(g.LogTau) != 1 || len(g.ThetaRaw) != 1 || len(g.Initial) != 1 || len(g.Inputs) != 2 {
		t.Fatalf("LIF gradient lengths are weights=%d bias=%d log_tau=%d theta=%d initial=%d inputs=%d", len(g.Weights), len(g.Bias), len(g.LogTau), len(g.ThetaRaw), len(g.Initial), len(g.Inputs))
	}
	for row := range g.Inputs {
		if len(g.Inputs[row]) != 1 {
			t.Fatalf("LIF input gradient row %d has length %d, want 1", row, len(g.Inputs[row]))
		}
	}
	if g.Inputs[0][0] != .0625 || g.Inputs[1][0] != 0 || g.Bias[0] != .0625 || g.ThetaRaw[0] != -.0625 || g.Initial[0] != .0625 {
		t.Fatalf("LIF gradient bias=%v theta=%v inputs=%v initial=%v", g.Bias, g.ThetaRaw, g.Inputs, g.Initial)
	}
	if math.IsNaN(g.LogTau[0]) || math.IsInf(g.LogTau[0], 0) || math.Abs(g.LogTau[0]-wantTau) > 1e-15*math.Abs(wantTau) {
		t.Fatalf("LIF log_tau gradient=%v, want %v", g.LogTau[0], wantTau)
	}
}

func zeroTauAssertMixedForward(t *testing.T, outputs, spikes [][]float64) {
	t.Helper()
	if len(outputs) != 2 || len(spikes) != 2 {
		t.Fatalf("Mixed rows outputs=%d spikes=%d, want 2", len(outputs), len(spikes))
	}
	for row := 0; row < 2; row++ {
		if len(outputs[row]) != 1 || len(spikes[row]) != 1 {
			t.Fatalf("Mixed row %d lengths outputs=%d spikes=%d, want 1", row, len(outputs[row]), len(spikes[row]))
		}
	}
	if outputs[0][0] != 1 || outputs[1][0] != .5 || spikes[0][0] != 1 || spikes[1][0] != 0 {
		t.Fatalf("Mixed outputs=%v spikes=%v, want [[1] [.5]] and [[1] [0]]", outputs, spikes)
	}
}

func zeroTauAssertMixedGradient(t *testing.T, g MixedGradient) {
	t.Helper()
	if len(g.Weights) != 0 || len(g.Bias) != 1 || len(g.LogTau) != 1 || len(g.ThetaRaw) != 1 || len(g.Initial) != 1 || len(g.Inputs) != 2 {
		t.Fatalf("Mixed gradient lengths are weights=%d bias=%d log_tau=%d theta=%d initial=%d inputs=%d", len(g.Weights), len(g.Bias), len(g.LogTau), len(g.ThetaRaw), len(g.Initial), len(g.Inputs))
	}
	for row := range g.Inputs {
		if len(g.Inputs[row]) != 1 {
			t.Fatalf("Mixed input gradient row %d has length %d, want 1", row, len(g.Inputs[row]))
		}
	}
	if g.Inputs[0][0] != .0625 || g.Inputs[1][0] != 0 || g.Bias[0] != .0625 || g.ThetaRaw[0] != -.0625 || g.Initial[0] != .0625 {
		t.Fatalf("Mixed gradient bias=%v theta=%v inputs=%v initial=%v", g.Bias, g.ThetaRaw, g.Inputs, g.Initial)
	}
	if math.IsNaN(g.LogTau[0]) || math.IsInf(g.LogTau[0], 0) || math.Abs(g.LogTau[0]+3*math.Log(2)/16) > 1e-15 {
		t.Fatalf("Mixed log_tau gradient=%v, want %v", g.LogTau[0], -3*math.Log(2)/16)
	}
}

func zeroTauAssertContinuousZero(t *testing.T, g Gradient) {
	t.Helper()
	if len(g.Weights) != 0 || len(g.Bias) != 1 || len(g.LogTau) != 1 || len(g.Initial) != 1 || len(g.Inputs) != 1 || len(g.Inputs[0]) != 1 {
		t.Fatalf("continuous gradient has unexpected lengths: weights=%d bias=%d log_tau=%d initial=%d inputs=%d", len(g.Weights), len(g.Bias), len(g.LogTau), len(g.Initial), len(g.Inputs))
	}
	values := []float64{g.Bias[0], g.LogTau[0], g.Initial[0], g.Inputs[0][0]}
	for i, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) || value != 0 {
			t.Fatalf("continuous gradient value %d=%v, want finite zero", i, value)
		}
	}
}

func zeroTauAssertContinuousCancellation(t *testing.T, g Gradient) {
	t.Helper()
	if len(g.Weights) != 0 || len(g.Bias) != 1 || len(g.LogTau) != 1 || len(g.Initial) != 1 || len(g.Inputs) != 1 || len(g.Inputs[0]) != 1 {
		t.Fatalf("continuous cancellation gradient has unexpected lengths")
	}
	wantTau := -(1e308 * math.Log(2))
	if g.Inputs[0][0] != .5 || g.Bias[0] != .5 || g.Initial[0] != .5 || math.IsNaN(g.LogTau[0]) || math.IsInf(g.LogTau[0], 0) || math.Abs(g.LogTau[0]-wantTau) > 1e-15*math.Abs(wantTau) {
		t.Fatalf("continuous cancellation gradient log_tau=%v bias=%v input=%v initial=%v, want log_tau=%v and .5 terms", g.LogTau[0], g.Bias, g.Inputs, g.Initial, wantTau)
	}
}

func TestZeroTauAdjointLIF(t *testing.T) {
	upstream := func() [][]float64 { return [][]float64{{0}, {1}} }
	t.Run("Forward", func(t *testing.T) {
		m, p, initial, inputs, _ := zeroTauLIFFixture(t)
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			t.Fatal(err)
		}
		zeroTauAssertLIFForward(t, tr.Outputs(), tr.Spikes())
		g, err := m.Backward(context.Background(), tr, upstream(), 0)
		if err != nil {
			t.Fatal(err)
		}
		zeroTauAssertLIFGradient(t, g)
	})
	t.Run("ForwardCoupled", func(t *testing.T) {
		m, p, initial, inputs, _ := zeroTauLIFFixture(t)
		seq := &zeroTauSequence{mods: zeroTauMods(2), nodes: 1}
		tr, err := m.ForwardCoupled(context.Background(), p, initial, inputs, seq)
		if err != nil {
			t.Fatal(err)
		}
		zeroTauAssertLIFForward(t, tr.Outputs(), tr.Spikes())
		g, err := m.Backward(context.Background(), tr, upstream(), 0)
		if err != nil {
			t.Fatal(err)
		}
		zeroTauAssertLIFGradient(t, g)
	})
	for _, segment := range []int{1, 2} {
		t.Run(fmt.Sprintf("BackwardRecompute/segment=%d", segment), func(t *testing.T) {
			m, p, initial, inputs, _ := zeroTauLIFFixture(t)
			g, err := m.BackwardRecompute(context.Background(), p, initial, inputs, upstream(), 0, segment)
			if err != nil {
				t.Fatal(err)
			}
			zeroTauAssertLIFGradient(t, g)
		})
	}
	for _, segment := range []int{1, 2} {
		t.Run(fmt.Sprintf("ForwardCoupledRecompute/segment=%d", segment), func(t *testing.T) {
			m, p, initial, inputs, _ := zeroTauLIFFixture(t)
			seq := &zeroTauRecomputeSequence{mods: zeroTauMods(2), nodes: 1}
			tr, err := m.ForwardCoupledRecompute(context.Background(), p, initial, inputs, seq, segment)
			if err != nil {
				t.Fatal(err)
			}
			outputs := tr.Outputs()
			if len(outputs) != 2 || len(outputs[0]) != 1 || len(outputs[1]) != 1 || outputs[0][0] != 1 || outputs[1][0] != .5 {
				t.Fatalf("recomputed outputs=%v, want [[1] [.5]]", outputs)
			}
			g, err := m.BackwardCoupledRecompute(context.Background(), tr, upstream(), 0)
			if err != nil {
				t.Fatal(err)
			}
			zeroTauAssertLIFGradient(t, g)
		})
	}
}

func TestZeroTauAdjointMixedLIF(t *testing.T) {
	upstream := [][]float64{{0}, {1}}
	t.Run("Forward", func(t *testing.T) {
		m, p, initial, inputs, _ := zeroTauMixedLIFFixture(t)
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			t.Fatal(err)
		}
		zeroTauAssertMixedForward(t, tr.Outputs(), tr.Spikes())
		g, err := m.Backward(context.Background(), tr, upstream, 0)
		if err != nil {
			t.Fatal(err)
		}
		zeroTauAssertMixedGradient(t, g)
	})
	t.Run("ForwardCoupled", func(t *testing.T) {
		m, p, initial, inputs, _ := zeroTauMixedLIFFixture(t)
		seq := &zeroTauSequence{mods: zeroTauMods(2), nodes: 1}
		tr, err := m.ForwardCoupled(context.Background(), p, initial, inputs, seq)
		if err != nil {
			t.Fatal(err)
		}
		zeroTauAssertMixedForward(t, tr.Outputs(), tr.Spikes())
		g, err := m.Backward(context.Background(), tr, upstream, 0)
		if err != nil {
			t.Fatal(err)
		}
		zeroTauAssertMixedGradient(t, g)
	})
}

func TestZeroTauAdjointContinuous(t *testing.T) {
	zero := [][]float64{{0}}
	t.Run("Forward", func(t *testing.T) {
		m, p, initial, inputs := zeroTauContinuousFixture(t)
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			t.Fatal(err)
		}
		if outputs := tr.Outputs(); len(outputs) != 1 || len(outputs[0]) != 1 || outputs[0][0] != 0 {
			t.Fatalf("continuous outputs=%v, want [[0]]", tr.Outputs())
		}
		g, err := m.Backward(context.Background(), tr, zero, 0)
		if err != nil {
			t.Fatal(err)
		}
		zeroTauAssertContinuousZero(t, g)
	})
	t.Run("ForwardCoupled", func(t *testing.T) {
		m, p, initial, inputs := zeroTauContinuousFixture(t)
		seq := &zeroTauSequence{mods: zeroTauMods(1), nodes: 1}
		tr, err := m.ForwardCoupled(context.Background(), p, initial, inputs, seq)
		if err != nil {
			t.Fatal(err)
		}
		if outputs := tr.Outputs(); len(outputs) != 1 || len(outputs[0]) != 1 || outputs[0][0] != 0 {
			t.Fatalf("coupled continuous outputs=%v, want [[0]]", tr.Outputs())
		}
		g, err := m.Backward(context.Background(), tr, zero, 0)
		if err != nil {
			t.Fatal(err)
		}
		zeroTauAssertContinuousZero(t, g)
	})
	t.Run("ForwardCoupledRecompute", func(t *testing.T) {
		m, p, initial, inputs := zeroTauContinuousFixture(t)
		seq := &zeroTauRecomputeSequence{mods: zeroTauMods(1), nodes: 1}
		tr, err := m.ForwardCoupledRecompute(context.Background(), p, initial, inputs, seq, 1)
		if err != nil {
			t.Fatal(err)
		}
		if outputs := tr.Outputs(); len(outputs) != 1 || len(outputs[0]) != 1 || outputs[0][0] != 0 {
			t.Fatalf("recomputed continuous outputs=%v, want [[0]]", tr.Outputs())
		}
		g, err := m.BackwardCoupledRecompute(context.Background(), tr, zero, 0)
		if err != nil {
			t.Fatal(err)
		}
		zeroTauAssertContinuousZero(t, g)
	})
	t.Run("BackwardRecompute/segment=1", func(t *testing.T) {
		m, p, initial, inputs := zeroTauContinuousFixture(t)
		g, err := m.BackwardRecompute(context.Background(), p, initial, inputs, zero, 0, 1)
		if err != nil {
			t.Fatal(err)
		}
		zeroTauAssertContinuousZero(t, g)
	})
	t.Run("underflowed lambda has zero derivative", func(t *testing.T) {
		m, err := NewContinuous(Config{Nodes: 1, DT: 1000, Activation: "tanh"})
		if err != nil {
			t.Fatal(err)
		}
		p := Parameters{Bias: []float64{0}, LogTau: []float64{0}}
		tr, err := m.Forward(context.Background(), p, []float64{math.MaxFloat64}, [][]float64{{0}})
		if err != nil {
			t.Fatal(err)
		}
		g, err := m.Backward(context.Background(), tr, zero, 0)
		if err != nil {
			t.Fatal(err)
		}
		zeroTauAssertContinuousZero(t, g)
	})
}

func TestZeroTauAdjointGPU(t *testing.T) {
	gpu, err := NewGPUContinuous(context.Background(), Config{Nodes: 1, DT: math.Log(2), Activation: "tanh"})
	if err != nil {
		if errors.Is(err, ErrGPUContinuousBackwardUnavailable) {
			t.Skipf("GPU backward primitive unavailable: %v", err)
		}
		if errors.Is(err, webgpu.ErrUnavailable) || errors.Is(err, webgpu.ErrDevice) {
			t.Skipf("GPU unavailable: %v", err)
		}
		t.Fatalf("NewGPUContinuous() error = %v", err)
	}
	defer gpu.Close()
	p := Parameters{Bias: []float64{1e308}, LogTau: []float64{0}}
	tr, err := gpu.Forward(context.Background(), p, []float64{-1e308}, [][]float64{{0}})
	if err != nil {
		t.Fatal(err)
	}
	if outputs := tr.Outputs(); len(outputs) != 1 || len(outputs[0]) != 1 || outputs[0][0] != 0 {
		t.Fatalf("GPU continuous outputs=%v, want [[0]]", tr.Outputs())
	}
	g, err := gpu.Backward(context.Background(), tr, [][]float64{{0}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	zeroTauAssertContinuousZero(t, g)
}

func TestZeroTauAdjointContinuousCancellation(t *testing.T) {
	upstream := [][]float64{{1}}
	t.Run("Forward", func(t *testing.T) {
		m, p, initial, inputs := zeroTauContinuousFixture(t)
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			t.Fatal(err)
		}
		g, err := m.Backward(context.Background(), tr, upstream, 0)
		if err != nil {
			t.Fatal(err)
		}
		zeroTauAssertContinuousCancellation(t, g)
	})
	t.Run("ForwardCoupled", func(t *testing.T) {
		m, p, initial, inputs := zeroTauContinuousFixture(t)
		seq := &zeroTauSequence{mods: zeroTauMods(1), nodes: 1}
		tr, err := m.ForwardCoupled(context.Background(), p, initial, inputs, seq)
		if err != nil {
			t.Fatal(err)
		}
		g, err := m.Backward(context.Background(), tr, upstream, 0)
		if err != nil {
			t.Fatal(err)
		}
		zeroTauAssertContinuousCancellation(t, g)
	})
	t.Run("ForwardCoupledRecompute", func(t *testing.T) {
		m, p, initial, inputs := zeroTauContinuousFixture(t)
		seq := &zeroTauRecomputeSequence{mods: zeroTauMods(1), nodes: 1}
		tr, err := m.ForwardCoupledRecompute(context.Background(), p, initial, inputs, seq, 1)
		if err != nil {
			t.Fatal(err)
		}
		g, err := m.BackwardCoupledRecompute(context.Background(), tr, upstream, 0)
		if err != nil {
			t.Fatal(err)
		}
		zeroTauAssertContinuousCancellation(t, g)
	})
	t.Run("upstream 16 still rejects true overflow", func(t *testing.T) {
		m, p, initial, inputs := zeroTauContinuousFixture(t)
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := m.Backward(context.Background(), tr, [][]float64{{16}}, 0); err == nil {
			t.Fatal("upstream 16 backward succeeded, want true overflow error")
		}
	})
}

func TestZeroTauAdjointVector(t *testing.T) {
	m, err := NewVectorContinuous(Config{Nodes: 1, DT: math.Log(2), Activation: "tanh", StateDimension: 1})
	if err != nil {
		t.Fatal(err)
	}
	p := VectorParameters{Bias: []float64{0}, LogTau: []float64{0}}
	tr, err := m.Forward(context.Background(), p, []float64{-1e308}, [][]float64{{1e308}})
	if err != nil {
		t.Fatal(err)
	}
	outputs := tr.Outputs()
	if len(outputs) != 2 || len(outputs[0]) != 1 || len(outputs[1]) != 1 {
		t.Fatalf("vector output shape=%v, want two one-value rows", outputs)
	}
	if outputs[0][0] != -1 || outputs[1][0] != 0 {
		t.Fatalf("vector outputs=%v, want [[-1] [0]]", outputs)
	}
	g, err := m.Backward(context.Background(), tr, [][]float64{{0}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Weights) != 0 || len(g.Bias) != 1 || len(g.LogTau) != 1 || len(g.Initial) != 1 || len(g.Inputs) != 1 || len(g.Inputs[0]) != 1 {
		t.Fatalf("vector gradient has unexpected lengths")
	}
	if g.Bias[0] != 0 || g.LogTau[0] != 0 || g.Initial[0] != 0 || g.Inputs[0][0] != 0 {
		t.Fatalf("vector gradient=%+v, want all zero", g)
	}
}

func TestZeroTauAdjointRejectsNonFiniteInput(t *testing.T) {
	t.Run("LIF", func(t *testing.T) {
		m, p, initial, _, _ := zeroTauLIFFixture(t)
		tr, err := m.Forward(context.Background(), p, initial, [][]float64{{math.NaN()}})
		if err == nil || tr != nil {
			t.Fatalf("LIF trace=%v err=%v, want nil trace and error", tr, err)
		}
	})
	t.Run("Continuous", func(t *testing.T) {
		m, p, initial, _ := zeroTauContinuousFixture(t)
		tr, err := m.Forward(context.Background(), p, initial, [][]float64{{math.Inf(1)}})
		if err == nil || tr != nil {
			t.Fatalf("continuous trace=%v err=%v, want nil trace and error", tr, err)
		}
	})
	t.Run("Vector", func(t *testing.T) {
		m, err := NewVectorContinuous(Config{Nodes: 1, DT: math.Log(2), Activation: "tanh", StateDimension: 1})
		if err != nil {
			t.Fatal(err)
		}
		tr, err := m.Forward(context.Background(), VectorParameters{Bias: []float64{0}, LogTau: []float64{0}}, []float64{0}, [][]float64{{math.NaN()}})
		if err == nil || tr != nil {
			t.Fatalf("vector trace=%v err=%v, want nil trace and error", tr, err)
		}
	})
}
