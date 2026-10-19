package dynamics

import (
	"context"
	"math"
	"math/rand/v2"
	"strings"
	"testing"
)

type gateFixture struct {
	model   *Continuous
	params  Parameters
	initial []float64
	inputs  [][]float64
	up      [][]float64
}

// newGateFixture builds a random delayed graph with the current gate. logTau
// sets every time constant; a small one makes 2*alpha*sigmoid exceed one at
// some steps, so the clamp branch is exercised.
func newGateFixture(t *testing.T, seed uint64, logTau float64, gate, activation string) gateFixture {
	t.Helper()
	rnd := rand.New(rand.NewPCG(seed, 3))
	const nodes, edges, steps = 7, 30, 9
	c := Config{Nodes: nodes, DT: 1, Activation: activation, Gate: gate}
	for range edges {
		c.Sources = append(c.Sources, rnd.IntN(nodes))
		c.Targets = append(c.Targets, rnd.IntN(nodes))
		c.Delays = append(c.Delays, rnd.IntN(3))
	}
	m, err := NewContinuous(c)
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
	f := gateFixture{model: m, initial: normal(nodes, .3, 0), inputs: make([][]float64, steps), up: make([][]float64, steps)}
	f.params = Parameters{Weights: normal(edges, .5, 0), Bias: normal(nodes, .2, 0), LogTau: normal(nodes, .2, logTau), GateGain: normal(nodes, .8, 0), GateBias: normal(nodes, .5, 0)}
	for i := range f.inputs {
		f.inputs[i], f.up[i] = normal(nodes, 1, 0), normal(nodes, 1, 0)
	}
	return f
}

func (f gateFixture) loss(t *testing.T, p Parameters, inputs [][]float64) float64 {
	t.Helper()
	tr, err := f.model.Forward(context.Background(), p, f.initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	var l float64
	for s, row := range tr.Outputs() {
		for i, y := range row {
			l += f.up[s][i] * y
		}
	}
	return l
}

// TestGateGradientMatchesFiniteDifferences checks every gradient group of the
// gated core, inputs included, against central differences of the forward
// pass, once without and once with clamped update rates, for both gate
// squashings and with the softsign activation.
func TestGateGradientMatchesFiniteDifferences(t *testing.T) {
	type variant struct{ gate, activation string }
	for _, v := range []variant{{GateCurrent, "tanh"}, {GateCurrentSoftsign, "tanh"}, {GateCurrentSoftsign, "softsign"}} {
		for _, tc := range []struct {
			name   string
			logTau float64
		}{{"unclamped", math.Log(2)}, {"clamped", math.Log(.4)}} {
			t.Run(v.gate+"/"+v.activation+"/"+tc.name, func(t *testing.T) {
				f := newGateFixture(t, 11, tc.logTau, v.gate, v.activation)
				tr, err := f.model.Forward(context.Background(), f.params, f.initial, f.inputs)
				if err != nil {
					t.Fatal(err)
				}
				clamped := 0
				for _, row := range tr.gate {
					for _, a := range row {
						if a == 1 {
							clamped++
						}
					}
				}
				if (clamped > 0) != (tc.name == "clamped") {
					t.Fatalf("%d clamped rates in the %s fixture", clamped, tc.name)
				}
				g, err := f.model.Backward(context.Background(), tr, f.up, 0)
				if err != nil {
					t.Fatal(err)
				}
				const h = 1e-6
				check := func(name string, values, grad []float64, eval func() float64) {
					t.Helper()
					for i := range values {
						orig := values[i]
						values[i] = orig + h
						plus := eval()
						values[i] = orig - h
						minus := eval()
						values[i] = orig
						want := (plus - minus) / (2 * h)
						if math.Abs(grad[i]-want) > 1e-6*(1+math.Abs(want)) {
							t.Fatalf("%s[%d] gradient %.10g, finite difference %.10g", name, i, grad[i], want)
						}
					}
				}
				p := cloneParameters(f.params)
				eval := func() float64 { return f.loss(t, p, f.inputs) }
				check("weights", p.Weights, g.Weights, eval)
				check("bias", p.Bias, g.Bias, eval)
				check("log_tau", p.LogTau, g.LogTau, eval)
				check("gate_gain", p.GateGain, g.GateGain, eval)
				check("gate_bias", p.GateBias, g.GateBias, eval)
				inputs := cloneRows(f.inputs)
				for s := range inputs {
					check("input", inputs[s], g.Inputs[s], func() float64 { return f.loss(t, f.params, inputs) })
				}
			})
		}
	}
}

// TestZeroGateIsThePlainIntegrator compares a gated model whose gain and bias
// are zero with the ungated model: a = alpha, so only the rounding of
// (1-alpha) against lambda can differ.
func TestZeroGateIsThePlainIntegrator(t *testing.T) {
	for _, gate := range []string{GateCurrent, GateCurrentSoftsign} {
		t.Run(gate, func(t *testing.T) { checkZeroGate(t, gate) })
	}
}

func checkZeroGate(t *testing.T, gate string) {
	f := newGateFixture(t, 4, math.Log(2), gate, "tanh")
	p := cloneParameters(f.params)
	clear(p.GateGain)
	clear(p.GateBias)
	gated, err := f.model.Forward(context.Background(), p, f.initial, f.inputs)
	if err != nil {
		t.Fatal(err)
	}
	c := f.model.Config()
	c.Gate = ""
	plain, err := NewContinuous(c)
	if err != nil {
		t.Fatal(err)
	}
	p.GateGain, p.GateBias = nil, nil
	ref, err := plain.Forward(context.Background(), p, f.initial, f.inputs)
	if err != nil {
		t.Fatal(err)
	}
	for s := range ref.voltage {
		for i := range ref.voltage[s] {
			if d := math.Abs(gated.voltage[s][i] - ref.voltage[s][i]); d > 1e-12 {
				t.Fatalf("step %d node %d differs by %g", s, i, d)
			}
		}
	}
}

func TestGateConfigurationAndUnsupportedPaths(t *testing.T) {
	ctx := context.Background()
	base := Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, DT: 1, Activation: "tanh"}
	bad := base
	bad.Gate = "voltage"
	if _, err := NewContinuous(bad); err == nil {
		t.Fatal("unknown gate accepted")
	}
	plain, err := NewContinuous(base)
	if err != nil {
		t.Fatal(err)
	}
	p := Parameters{Weights: []float64{.5}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}}
	withGate := p
	withGate.GateGain, withGate.GateBias = []float64{0, 0}, []float64{0, 0}
	inputs := [][]float64{{1, 0}}
	if _, err := plain.Forward(ctx, withGate, []float64{0, 0}, inputs); err == nil {
		t.Fatal("gate parameters accepted by an ungated model")
	}
	gatedConfig := base
	gatedConfig.Gate = GateCurrent
	gated, err := NewContinuous(gatedConfig)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gated.Forward(ctx, p, []float64{0, 0}, inputs); err == nil {
		t.Fatal("gated model ran without gate parameters")
	}
	if _, err := gated.ForwardRecompute(ctx, withGate, []float64{0, 0}, inputs, 1); err == nil || !strings.Contains(err.Error(), "gated") {
		t.Fatalf("recompute: %v", err)
	}
	if _, err := gated.NewState([]float64{0, 0}); err == nil || !strings.Contains(err.Error(), "gated") {
		t.Fatalf("persistent state: %v", err)
	}
	vector := gatedConfig
	vector.StateDimension = 2
	if _, err := NewVectorContinuous(vector); err == nil {
		t.Fatal("vector core accepted a gate")
	}
}

// TestInferMatchesForwardOutputs checks that the record-free passes return
// exactly Forward's outputs (InferLast its last row) for delayed and undelayed graphs, serial and
// partitioned, with and without each gate.
func TestInferMatchesForwardOutputs(t *testing.T) {
	for _, gate := range []string{"", GateCurrent, GateCurrentSoftsign} {
		for _, activation := range []string{"tanh", "softsign", "softplus"} {
			for _, delayed := range []bool{true, false} {
				for _, workers := range []int{1, 3} {
					f := newGateFixture(t, 7, math.Log(.4), GateCurrent, activation)
					c := f.model.Config()
					c.Gate, c.Workers = gate, workers
					if !delayed {
						c.Delays = nil
					}
					m, err := NewContinuous(c)
					if err != nil {
						t.Fatal(err)
					}
					p := cloneParameters(f.params)
					if gate == "" {
						p.GateGain, p.GateBias = nil, nil
					}
					tr, err := m.Forward(context.Background(), p, f.initial, f.inputs)
					if err != nil {
						t.Fatal(err)
					}
					want := tr.Outputs()
					got, err := m.Infer(context.Background(), p, f.initial, f.inputs)
					if err != nil {
						t.Fatal(err)
					}
					if len(got) != len(want) {
						t.Fatalf("%d rows, want %d", len(got), len(want))
					}
					for s := range want {
						for i := range want[s] {
							if math.Float64bits(got[s][i]) != math.Float64bits(want[s][i]) {
								t.Fatalf("gate %q %s delayed=%v workers=%d: step %d node %d = %v, want %v", gate, activation, delayed, workers, s, i, got[s][i], want[s][i])
							}
						}
					}
					last, err := m.InferLast(context.Background(), p, f.initial, f.inputs)
					if err != nil {
						t.Fatal(err)
					}
					final := want[len(want)-1]
					if len(last) != len(final) {
						t.Fatalf("last row has %d nodes, want %d", len(last), len(final))
					}
					for i := range final {
						if math.Float64bits(last[i]) != math.Float64bits(final[i]) {
							t.Fatalf("gate %q %s delayed=%v workers=%d: InferLast node %d = %v, want %v", gate, activation, delayed, workers, i, last[i], final[i])
						}
					}
				}
			}
		}
	}
}

func TestOverflowingDriveIsRejected(t *testing.T) {
	for _, gate := range []string{"", GateCurrent, GateCurrentSoftsign} {
		for _, activation := range []string{"tanh", "softsign", "softplus"} {
			for _, gain := range []float64{0, 1} {
				f := newGateFixture(t, 7, math.Log(.4), GateCurrent, activation)
				c := f.model.Config()
				c.Gate = gate
				m, err := NewContinuous(c)
				if err != nil {
					t.Fatal(err)
				}
				p := cloneParameters(f.params)
				if gate == "" {
					p.GateGain, p.GateBias = nil, nil
				} else {
					for i := range p.GateGain {
						p.GateGain[i] = gain
					}
				}
				// Bias plus a same-signed drive overflows to an infinite drive.
				for i := range p.Bias {
					p.Bias[i] = math.MaxFloat64
				}
				for i := range p.Weights {
					p.Weights[i] = math.MaxFloat64
				}
				inputs := cloneRows(f.inputs)
				for s := range inputs {
					for i := range inputs[s] {
						inputs[s][i] = math.MaxFloat64
					}
				}
				if _, err := m.Forward(context.Background(), p, f.initial, inputs); err == nil || !strings.Contains(err.Error(), "non-finite state") {
					t.Fatalf("gate %q %s gain %v: Forward error %v", gate, activation, gain, err)
				}
				if _, err := m.Infer(context.Background(), p, f.initial, inputs); err == nil || !strings.Contains(err.Error(), "non-finite state") {
					t.Fatalf("gate %q %s gain %v: Infer error %v", gate, activation, gain, err)
				}
			}
		}
	}
}
