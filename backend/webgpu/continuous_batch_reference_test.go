package webgpu_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/TimLai666/coimnet/backend/webgpu"
	"github.com/TimLai666/coimnet/dynamics"
)

type batchOpener func(context.Context, webgpu.ContinuousBatchConfig) (*webgpu.ContinuousBatch, error)

// batchCase is one random undelayed graph, in arbitrary edge order, with a
// subset of input and readout nodes. logTau ln 0.4 makes 2*alpha*s exceed one
// at some steps, so a gated model exercises the clamp; ln 2 keeps every rate
// below it.
type batchCase struct {
	gate, activation string
	logTau           float64
	everyStep        bool
}

func batchCases() []batchCase {
	var cases []batchCase
	for _, gate := range []string{"", dynamics.GateCurrent, dynamics.GateCurrentSoftsign} {
		for _, activation := range []string{"tanh", "softsign"} {
			for _, logTau := range []float64{math.Log(2), math.Log(.4)} {
				cases = append(cases, batchCase{gate: gate, activation: activation, logTau: logTau, everyStep: logTau < 0})
			}
		}
	}
	return append(cases, batchCase{gate: dynamics.GateCurrent, activation: "softplus", logTau: math.Log(.4)}, batchCase{activation: "softplus", logTau: math.Log(2), everyStep: true})
}

func (c batchCase) name() string {
	gate := c.gate
	if gate == "" {
		gate = "ungated"
	}
	return fmt.Sprintf("%s/%s/logtau%.2f/every%v", gate, c.activation, c.logTau, c.everyStep)
}

// compareBatchWithContinuous runs the device executor on one batch and checks
// the readout and every gradient group against the sum of per-sequence
// float64 dynamics.Continuous Forward/Backward passes.
func compareBatchWithContinuous(t *testing.T, open batchOpener, c batchCase, seed uint64) {
	t.Helper()
	rnd := rand.New(rand.NewPCG(seed, 3))
	const nodes, edges, steps, batch = 7, 30, 9, 3
	inputNodes := []int{5, 0, 3, 2}
	readoutNodes := []int{6, 1, 3}
	cfg := dynamics.Config{Nodes: nodes, DT: 1, Activation: c.activation, Gate: c.gate}
	for range edges {
		cfg.Sources = append(cfg.Sources, rnd.IntN(nodes))
		cfg.Targets = append(cfg.Targets, rnd.IntN(nodes))
	}
	model, err := dynamics.NewContinuous(cfg)
	if err != nil {
		t.Fatal(err)
	}
	normal := func(n int, std, mean float64) []float64 {
		v := make([]float64, n)
		for i := range v {
			v[i] = mean + std*rnd.NormFloat64()
		}
		return v
	}
	p := dynamics.Parameters{Weights: normal(edges, .5, 0), Bias: normal(nodes, .2, 0), LogTau: normal(nodes, .2, c.logTau)}
	if c.gate != "" {
		p.GateGain, p.GateBias = normal(nodes, .8, 0), normal(nodes, .5, 0)
	}
	readSteps := 1
	if c.everyStep {
		readSteps = steps
	}
	inputs := make([][][]float64, batch)
	upstream := make([][][]float64, batch)
	for b := range inputs {
		inputs[b] = make([][]float64, steps)
		for s := range inputs[b] {
			inputs[b][s] = normal(len(inputNodes), 1, 0)
		}
		upstream[b] = make([][]float64, readSteps)
		for s := range upstream[b] {
			upstream[b][s] = normal(len(readoutNodes), 1, 0)
		}
	}
	// Round every value through float32 once, so the reference integrates the
	// same inputs and parameters the device receives.
	round := func(v []float64) {
		for i := range v {
			v[i] = float64(float32(v[i]))
		}
	}
	for _, v := range [][]float64{p.Weights, p.Bias, p.GateGain, p.GateBias} {
		round(v)
	}
	for b := range inputs {
		for s := range inputs[b] {
			round(inputs[b][s])
		}
		for s := range upstream[b] {
			round(upstream[b][s])
		}
	}

	// Reference: one fresh Forward and full Backward per sequence, summed.
	want := dynamics.Gradient{Weights: make([]float64, edges), Bias: make([]float64, nodes), LogTau: make([]float64, nodes)}
	if c.gate != "" {
		want.GateGain, want.GateBias = make([]float64, nodes), make([]float64, nodes)
	}
	wantReadout := make([][][]float64, batch)
	wantInputs := make([][][]float64, batch)
	for b := range inputs {
		full := make([][]float64, steps)
		up := make([][]float64, steps)
		for s := range full {
			full[s], up[s] = make([]float64, nodes), make([]float64, nodes)
			for w, node := range inputNodes {
				full[s][node] = inputs[b][s][w]
			}
		}
		for s := range upstream[b] {
			row := steps - 1
			if c.everyStep {
				row = s
			}
			for r, node := range readoutNodes {
				up[row][node] = upstream[b][s][r]
			}
		}
		tr, err := model.Forward(context.Background(), p, make([]float64, nodes), full)
		if err != nil {
			t.Fatal(err)
		}
		outputs := tr.Outputs()
		wantReadout[b] = make([][]float64, readSteps)
		for s := range wantReadout[b] {
			row := steps - 1
			if c.everyStep {
				row = s
			}
			for _, node := range readoutNodes {
				wantReadout[b][s] = append(wantReadout[b][s], outputs[row][node])
			}
		}
		g, err := model.Backward(context.Background(), tr, up, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, pair := range [][2][]float64{{want.Weights, g.Weights}, {want.Bias, g.Bias}, {want.LogTau, g.LogTau}, {want.GateGain, g.GateGain}, {want.GateBias, g.GateBias}} {
			for i, v := range pair[1] {
				pair[0][i] += v
			}
		}
		wantInputs[b] = make([][]float64, steps)
		for s := range wantInputs[b] {
			for _, node := range inputNodes {
				wantInputs[b][s] = append(wantInputs[b][s], g.Inputs[s][node])
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	exec, err := open(ctx, webgpu.ContinuousBatchConfig{Nodes: nodes, Sources: cfg.Sources, Targets: cfg.Targets, Activation: c.activation, Gate: c.gate,
		InputNodes: inputNodes, ReadoutNodes: readoutNodes, ReadoutEveryStep: c.everyStep})
	if err != nil {
		if errors.Is(err, webgpu.ErrUnavailable) {
			t.Skipf("GPU unavailable: %v", err)
		}
		t.Fatal(err)
	}
	defer func() {
		if err := exec.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()
	params := webgpu.ContinuousBatchParameters{Weights: f32(p.Weights), Bias: f32(p.Bias), GateGain: f32(p.GateGain), GateBias: f32(p.GateBias)}
	for _, raw := range p.LogTau {
		tau := math.Exp(raw)
		lambda := math.Exp(-1 / tau)
		params.Lambda = append(params.Lambda, float32(lambda))
		params.Alpha = append(params.Alpha, float32(-math.Expm1(-1/tau)))
		params.TauSlope = append(params.TauSlope, float32(lambda/tau))
	}
	flatInputs := make([]float32, 0, batch*steps*len(inputNodes))
	flatUp := make([]float32, 0, batch*readSteps*len(readoutNodes))
	for b := range inputs {
		for s := range inputs[b] {
			flatInputs = append(flatInputs, f32(inputs[b][s])...)
		}
		for s := range upstream[b] {
			flatUp = append(flatUp, f32(upstream[b][s])...)
		}
	}
	// Two rounds check that a second Forward and Backward on the same
	// executor reuse the resident buffers without stale state.
	for round := 0; round < 2; round++ {
		readout, err := exec.Forward(ctx, params, batch, steps, flatInputs)
		if err != nil {
			t.Fatal(err)
		}
		var flatWant []float64
		for b := range wantReadout {
			for s := range wantReadout[b] {
				flatWant = append(flatWant, wantReadout[b][s]...)
			}
		}
		assertClose(t, "readout", readout, flatWant)
		got, err := exec.Backward(ctx, flatUp)
		if err != nil {
			t.Fatal(err)
		}
		assertClose(t, "weights", got.Weights, want.Weights)
		assertClose(t, "bias", got.Bias, want.Bias)
		assertClose(t, "log_tau", got.LogTau, want.LogTau)
		assertClose(t, "gate_gain", got.GateGain, want.GateGain)
		assertClose(t, "gate_bias", got.GateBias, want.GateBias)
		var flatInputGrad []float64
		for b := range wantInputs {
			for s := range wantInputs[b] {
				flatInputGrad = append(flatInputGrad, wantInputs[b][s]...)
			}
		}
		assertClose(t, "inputs", got.Inputs, flatInputGrad)
	}
}

func f32(v []float64) []float32 {
	if v == nil {
		return nil
	}
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(x)
	}
	return out
}

// assertClose allows float32 rounding of a short recurrent sum.
func assertClose(t *testing.T, name string, got []float32, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s has %d values, want %d", name, len(got), len(want))
	}
	for i := range want {
		if d := math.Abs(float64(got[i]) - want[i]); d > 1e-4*(1+math.Abs(want[i])) {
			t.Fatalf("%s[%d] = %.8g, float64 reference %.8g (difference %.3g)", name, i, got[i], want[i], d)
		}
	}
}

// TestContinuousBatchHardwareMatchesContinuous checks the device forward
// outputs and every gradient group against dynamics.Continuous on a hardware
// adapter. It skips with ErrUnavailable where none exists.
func TestContinuousBatchHardwareMatchesContinuous(t *testing.T) {
	for k, c := range batchCases() {
		t.Run(c.name(), func(t *testing.T) { compareBatchWithContinuous(t, webgpu.NewContinuousBatch, c, uint64(11+k)) })
	}
}

// TestContinuousBatchSoftwareAdapterMatchesContinuous runs the same kernels
// through gogpu's software adapter, which interprets their SPIR-V on the CPU.
// It is a host-side check of the kernels' arithmetic, not a hardware result.
func TestContinuousBatchSoftwareAdapterMatchesContinuous(t *testing.T) {
	for k, c := range batchCases() {
		t.Run(c.name(), func(t *testing.T) {
			compareBatchWithContinuous(t, webgpu.NewContinuousBatchSoftwareForTest, c, uint64(11+k))
		})
	}
}
