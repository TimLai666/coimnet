package webgpu

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

// These tests open the batched executor on gogpu's software adapter, which
// interprets the kernels' SPIR-V on the CPU, so the device-side bookkeeping
// runs where no GPU exists. They are host checks, not hardware results.

// TestContinuousBatchTwoDimensionalDispatchMatches runs one episode with the
// device's workgroup limit and again with a limit of three, which folds every
// launch into a two-dimensional grid. The flat index must cover each element
// once, so both runs agree bit for bit.
func TestContinuousBatchTwoDimensionalDispatchMatches(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	const nodes, batch, steps = 7, 40, 4
	cfg := ContinuousBatchConfig{Nodes: nodes, Activation: ActivationTanh, Gate: GateSigmoid, InputNodes: []int{0, 2}, ReadoutNodes: []int{6, 4}, ReadoutEveryStep: true}
	for e := 0; e < 20; e++ {
		cfg.Sources = append(cfg.Sources, (3*e+1)%nodes)
		cfg.Targets = append(cfg.Targets, (5*e+2)%nodes)
	}
	exec := openSoftwareBatch(t, ctx, cfg)
	defer exec.Close()
	ramp := func(n int, scale float32) []float32 {
		v := make([]float32, n)
		for i := range v {
			v[i] = scale * float32((i*7)%11-5) / 5
		}
		return v
	}
	p := ContinuousBatchParameters{Weights: ramp(20, .4), Bias: ramp(nodes, .1), GateGain: ramp(nodes, .5), GateBias: ramp(nodes, .3)}
	for range nodes {
		p.Lambda, p.Alpha, p.TauSlope = append(p.Lambda, .6), append(p.Alpha, .4), append(p.TauSlope, .3)
	}
	inputs, upstream := ramp(batch*steps*2, 1), ramp(batch*steps*2, .7)
	run := func() ([]float32, ContinuousBatchGradient) {
		t.Helper()
		out, err := exec.Forward(ctx, p, batch, steps, inputs)
		if err != nil {
			t.Fatal(err)
		}
		g, err := exec.Backward(ctx, upstream)
		if err != nil {
			t.Fatal(err)
		}
		return out, g
	}
	out1, g1 := run()
	if x, y, _ := dispatchGroups(batch*nodes, 3); y < 2 || x != 3 {
		t.Fatalf("limit 3 gives a %dx%d grid, not two-dimensional", x, y)
	}
	exec.limits.MaxComputeWorkgroupsPerDimension = 3
	out2, g2 := run()
	equalF32(t, "readout", out2, out1)
	equalF32(t, "weights", g2.Weights, g1.Weights)
	equalF32(t, "bias", g2.Bias, g1.Bias)
	equalF32(t, "log tau", g2.LogTau, g1.LogTau)
	equalF32(t, "gate gain", g2.GateGain, g1.GateGain)
	equalF32(t, "gate bias", g2.GateBias, g1.GateBias)
	equalF32(t, "inputs", g2.Inputs, g1.Inputs)
	nonzero := false
	for _, v := range g1.Weights {
		nonzero = nonzero || v != 0
	}
	if !nonzero {
		t.Fatal("every weight gradient is zero; the fixture exercises nothing")
	}
}

// TestContinuousBatchGrowsAndReusesEpisodeBuffers runs a small batch, a
// larger one that reallocates, and the small one again, which must reproduce
// its first result exactly.
func TestContinuousBatchGrowsAndReusesEpisodeBuffers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	exec := openSoftwareBatch(t, ctx, ContinuousBatchConfig{Nodes: 3, Sources: []int{0, 1, 2, 0}, Targets: []int{1, 2, 0, 2}, Activation: ActivationSoftsign, ReadoutNodes: []int{2}})
	defer exec.Close()
	p := ContinuousBatchParameters{Weights: []float32{.5, -.7, .9, .3}, Bias: []float32{.1, 0, -.1}, Lambda: []float32{.5, .6, .7}, Alpha: []float32{.5, .4, .3}, TauSlope: []float32{.2, .3, .4}}
	small := func() ([]float32, ContinuousBatchGradient) {
		t.Helper()
		out, err := exec.Forward(ctx, p, 2, 3, []float32{1, 0, -1, .5, .2, -.3, 1, 1, 1, -1, -1, -1, 0, .1, .2, .3, .4, .5})
		if err != nil {
			t.Fatal(err)
		}
		g, err := exec.Backward(ctx, []float32{1, -.5})
		if err != nil {
			t.Fatal(err)
		}
		return out, g
	}
	out1, g1 := small()
	large := make([]float32, 5*6*3)
	for i := range large {
		large[i] = float32(i%5) / 4
	}
	if _, err := exec.Forward(ctx, p, 5, 6, large); err != nil {
		t.Fatal(err)
	}
	if m := exec.Metrics(); m.Batch != 5 || m.Steps != 6 || m.AllocatedBytes == 0 {
		t.Fatalf("metrics after the large batch: %+v", m)
	}
	out2, g2 := small()
	equalF32(t, "readout", out2, out1)
	equalF32(t, "weights", g2.Weights, g1.Weights)
	equalF32(t, "inputs", g2.Inputs, g1.Inputs)
}

func TestContinuousBatchBackwardNeedsForwardAndCloses(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	exec := openSoftwareBatch(t, ctx, ContinuousBatchConfig{Nodes: 2, Sources: []int{0}, Targets: []int{1}, Activation: ActivationTanh, ReadoutNodes: []int{1}})
	if _, err := exec.Backward(ctx, []float32{1}); !errors.Is(err, ErrShape) {
		t.Fatalf("Backward before Forward: %v", err)
	}
	p := ContinuousBatchParameters{Weights: []float32{1}, Bias: []float32{0, 0}, Lambda: []float32{.5, .5}, Alpha: []float32{.5, .5}, TauSlope: []float32{.3, .3}}
	if _, err := exec.Forward(ctx, p, 1, 2, []float32{1, 2}); !errors.Is(err, ErrShape) {
		t.Fatalf("short inputs: %v", err)
	}
	if _, err := exec.Backward(ctx, []float32{1}); !errors.Is(err, ErrShape) {
		t.Fatalf("Backward after a failed Forward: %v", err)
	}
	if _, err := exec.Forward(ctx, p, 1, 1, []float32{1, 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := exec.Backward(ctx, []float32{1, 2}); !errors.Is(err, ErrShape) {
		t.Fatalf("long upstream: %v", err)
	}
	if _, err := exec.Backward(ctx, []float32{float32(math.NaN())}); !errors.Is(err, ErrNonFinite) {
		t.Fatalf("NaN upstream: %v", err)
	}
	if err := exec.Close(); err != nil {
		t.Fatal(err)
	}
	if err := exec.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := exec.Forward(ctx, p, 1, 1, []float32{1, 2}); !errors.Is(err, ErrClosed) {
		t.Fatalf("Forward after Close: %v", err)
	}
}

// TestContinuousBatchRefusesSoftwareAdapter checks that the production
// constructor never runs on a software adapter: where the only adapter is
// gogpu's CPU renderer it reports ErrUnavailable.
func TestContinuousBatchRefusesSoftwareAdapter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	exec, err := NewContinuousBatch(ctx, ContinuousBatchConfig{Nodes: 1, Activation: ActivationTanh, ReadoutNodes: []int{0}})
	if err == nil {
		info := exec.Info()
		_ = exec.Close()
		if info.DeviceType == "CPU" || info.Backend == "Empty" {
			t.Fatalf("production constructor accepted software adapter %+v", info)
		}
		t.Skipf("hardware adapter %q present", info.Adapter)
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Skipf("adapter could not be opened for another reason: %v", err)
	}
}

func openSoftwareBatch(t *testing.T, ctx context.Context, cfg ContinuousBatchConfig) *ContinuousBatch {
	t.Helper()
	exec, err := newContinuousBatch(ctx, cfg, true)
	if err != nil {
		if errors.Is(err, ErrUnavailable) {
			t.Skipf("no adapter: %v", err)
		}
		t.Fatal(err)
	}
	return exec
}
