package dynamics

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/TimLai666/coimnet/backend/webgpu"
)

func TestGPUContinuousBatchRefusesUnsupportedBeforeDeviceOpen(t *testing.T) {
	ok := func() Config {
		return Config{Nodes: 3, Sources: []int{0, 1}, Targets: []int{1, 2}, DT: 1, Activation: "tanh"}
	}
	layout := BatchLayout{ReadoutNodes: []int{2}}
	cases := []struct {
		name   string
		mutate func(*Config)
		want   error
	}{
		{"positive delay", func(c *Config) { c.Delays = []int{0, 1} }, ErrGPUContinuousUnsupported},
		{"vector state", func(c *Config) { c.StateDimension = 2 }, ErrGPUContinuousUnsupported},
		{"matrix edges", func(c *Config) { c.EdgeShape = "matrix" }, ErrGPUContinuousUnsupported},
		{"unknown activation", func(c *Config) { c.Activation = "relu" }, nil},
		{"unknown gate", func(c *Config) { c.Gate = "voltage" }, nil},
		{"zero dt", func(c *Config) { c.DT = 0 }, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := ok()
			tc.mutate(&c)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := NewGPUContinuousBatch(ctx, c, layout)
			if err == nil {
				t.Fatal("unsupported configuration accepted")
			}
			if errors.Is(err, webgpu.ErrUnavailable) {
				t.Fatalf("refused only after opening a device: %v", err)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("error %v, want %v", err, tc.want)
			}
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for name, l := range map[string]BatchLayout{"no readout": {}, "readout out of range": {ReadoutNodes: []int{3}}, "duplicate input": {InputNodes: []int{1, 1}, ReadoutNodes: []int{0}}} {
		if _, err := NewGPUContinuousBatch(ctx, ok(), l); !errors.Is(err, webgpu.ErrInvalidConfig) {
			t.Fatalf("%s: error %v, want webgpu.ErrInvalidConfig", name, err)
		}
	}
	if _, err := NewGPUContinuousBatch(nil, ok(), layout); err == nil { //nolint:staticcheck // nil context is the case under test
		t.Fatal("nil context accepted")
	}
}

func TestBatchCoefficientsMatchTheIntegrator(t *testing.T) {
	m, err := NewContinuous(Config{Nodes: 3, DT: .5, Activation: "tanh"})
	if err != nil {
		t.Fatal(err)
	}
	p := Parameters{LogTau: []float64{math.Log(2), 0, math.Log(.25)}}
	lambda, alpha, slope, err := batchCoefficients(m, p)
	if err != nil {
		t.Fatal(err)
	}
	for i, raw := range p.LogTau {
		tau := math.Exp(raw)
		l := math.Exp(-.5 / tau)
		for name, pair := range map[string][2]float64{"lambda": {float64(lambda[i]), l}, "alpha": {float64(alpha[i]), 1 - l}, "slope": {float64(slope[i]), l * .5 / tau}} {
			if math.Abs(pair[0]-pair[1]) > 1e-7 {
				t.Fatalf("%s[%d] = %g, want %g", name, i, pair[0], pair[1])
			}
		}
	}
	// A lambda below the float32 range cannot be integrated as written.
	if _, _, _, err := batchCoefficients(m, Parameters{LogTau: []float64{-6, 0, 0}}); !errors.Is(err, ErrGPUContinuousPrecision) {
		t.Fatalf("float32-underflowing lambda: %v", err)
	}
}

// TestGPUContinuousBatchHardwareMatchesForwardBackward mirrors the gate
// fixtures on an undelayed graph: for each gate and activation, clamped and
// unclamped, a batch of independent sequences on the device must match the
// summed float64 Forward/Backward passes of dynamics.Continuous at float32
// tolerance, for the readout and every gradient group. It skips with
// webgpu.ErrUnavailable when no hardware adapter exists.
func TestGPUContinuousBatchHardwareMatchesForwardBackward(t *testing.T) {
	type variant struct{ gate, activation string }
	for _, v := range []variant{{"", "tanh"}, {"", "softsign"}, {GateCurrent, "tanh"}, {GateCurrent, "softsign"}, {GateCurrentSoftsign, "tanh"}, {GateCurrentSoftsign, "softsign"}} {
		for _, tc := range []struct {
			name      string
			logTau    float64
			everyStep bool
		}{{"unclamped", math.Log(2), false}, {"clamped", math.Log(.4), true}} {
			name := v.gate
			if name == "" {
				name = "ungated"
			}
			t.Run(name+"/"+v.activation+"/"+tc.name, func(t *testing.T) {
				checkGPUBatchAgainstContinuous(t, v.gate, v.activation, tc.logTau, tc.everyStep, tc.name == "clamped")
			})
		}
	}
}

func checkGPUBatchAgainstContinuous(t *testing.T, gate, activation string, logTau float64, everyStep, wantClamp bool) {
	const batch = 3
	inputNodes, readoutNodes := []int{5, 0, 3, 2}, []int{6, 1, 3}
	var fixtures []gateFixture
	for b := range batch {
		fixtures = append(fixtures, newGateFixture(t, uint64(21+b), logTau, gate, activation))
	}
	// The first fixture fixes the graph and parameters; every fixture adds one
	// sequence. Delays are dropped: the device runs undelayed graphs only.
	c := fixtures[0].model.Config()
	c.Delays = nil
	model, err := NewContinuous(c)
	if err != nil {
		t.Fatal(err)
	}
	p := cloneParameters(fixtures[0].params)
	if gate == "" {
		p.GateGain, p.GateBias = nil, nil
	}
	steps := len(fixtures[0].inputs)
	readSteps := 1
	if everyStep {
		readSteps = steps
	}
	want := Gradient{Weights: make([]float64, len(p.Weights)), Bias: make([]float64, c.Nodes), LogTau: make([]float64, c.Nodes)}
	if gate != "" {
		want.GateGain, want.GateBias = make([]float64, c.Nodes), make([]float64, c.Nodes)
	}
	inputs := make([][][]float64, batch)
	upstream := make([][][]float64, batch)
	wantReadout := make([][][]float64, batch)
	wantInputs := make([][][]float64, batch)
	clamped := 0
	for b, f := range fixtures {
		full := make([][]float64, steps)
		up := make([][]float64, steps)
		inputs[b] = make([][]float64, steps)
		for s := range full {
			full[s], up[s] = make([]float64, c.Nodes), make([]float64, c.Nodes)
			for _, node := range inputNodes {
				x := float64(float32(f.inputs[s][node]))
				full[s][node] = x
				inputs[b][s] = append(inputs[b][s], x)
			}
		}
		upstream[b] = make([][]float64, readSteps)
		for s := range upstream[b] {
			row := steps - 1
			if everyStep {
				row = s
			}
			for _, node := range readoutNodes {
				g := float64(float32(f.up[row][node]))
				up[row][node] = g
				upstream[b][s] = append(upstream[b][s], g)
			}
		}
		tr, err := model.Forward(context.Background(), p, make([]float64, c.Nodes), full)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range tr.gate {
			for _, a := range row {
				if a == 1 {
					clamped++
				}
			}
		}
		outputs := tr.Outputs()
		wantReadout[b] = make([][]float64, readSteps)
		for s := range wantReadout[b] {
			row := steps - 1
			if everyStep {
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
			for i, x := range pair[1] {
				pair[0][i] += x
			}
		}
		wantInputs[b] = make([][]float64, steps)
		for s := range wantInputs[b] {
			for _, node := range inputNodes {
				wantInputs[b][s] = append(wantInputs[b][s], g.Inputs[s][node])
			}
		}
	}
	if gate != "" && (clamped > 0) != wantClamp {
		t.Fatalf("%d clamped rates, want clamped=%v", clamped, wantClamp)
	}
	// The reference and the fixture checks above run everywhere; only the
	// device half needs an adapter.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	gpu, err := NewGPUContinuousBatch(ctx, c, BatchLayout{InputNodes: inputNodes, ReadoutNodes: readoutNodes, ReadoutEveryStep: everyStep})
	if err != nil {
		if errors.Is(err, webgpu.ErrUnavailable) {
			t.Skipf("GPU unavailable: %v", err)
		}
		t.Fatal(err)
	}
	defer func() {
		if err := gpu.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()
	// The device integrates float32 parameters; compare with a reference that
	// used the float64 ones, so the tolerance also covers that rounding.
	readout, err := gpu.Forward(ctx, p, inputs)
	if err != nil {
		t.Fatal(err)
	}
	for b := range readout {
		for s := range readout[b] {
			closeRows(t, "readout", readout[b][s], wantReadout[b][s])
		}
	}
	got, err := gpu.Backward(ctx, upstream)
	if err != nil {
		t.Fatal(err)
	}
	closeRows(t, "weights", got.Weights, want.Weights)
	closeRows(t, "bias", got.Bias, want.Bias)
	closeRows(t, "log_tau", got.LogTau, want.LogTau)
	closeRows(t, "gate_gain", got.GateGain, want.GateGain)
	closeRows(t, "gate_bias", got.GateBias, want.GateBias)
	for b := range wantInputs {
		for s := range wantInputs[b] {
			closeRows(t, "inputs", got.Inputs[b][s], wantInputs[b][s])
		}
	}
	t.Logf("adapter %+v metrics %+v", gpu.Info(), gpu.Metrics())
}

func closeRows(t *testing.T, name string, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s has %d values, want %d", name, len(got), len(want))
	}
	for i := range want {
		if d := math.Abs(got[i] - want[i]); d > 2e-4*(1+math.Abs(want[i])) {
			t.Fatalf("%s[%d] = %.8g, float64 reference %.8g (difference %.3g)", name, i, got[i], want[i], d)
		}
	}
}
