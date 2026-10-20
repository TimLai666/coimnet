package dynamics

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"strings"
	"testing"
)

type chemFixture struct {
	model   *Continuous
	params  Parameters
	initial []float64
	inputs  [][]float64
	up      [][]float64
}

// newChemFixture builds a random delayed graph with two chemistry channels
// whose receptors cover about half of the (node, channel) pairs. A narrow
// gamma and beta range makes the clamps bind at some steps.
func newChemFixture(t *testing.T, seed uint64, gate, activation string, narrow bool, workers int) chemFixture {
	t.Helper()
	rnd := rand.New(rand.NewPCG(seed, 5))
	const nodes, edges, steps = 7, 30, 9
	chem := &Chemistry{GammaMin: -5, GammaMax: 8, BetaAbsMax: 5}
	if narrow {
		chem.GammaMin, chem.GammaMax, chem.BetaAbsMax = .8, 1.25, .2
	}
	chem.Channels = []ChemistryChannel{{Tau: 3, Kd: 1.5, Hill: 2}, {Tau: 1.5, Kd: .8, Hill: 1}}
	for k := range chem.Channels {
		for j := range nodes {
			if rnd.IntN(2) == 0 || len(chem.Channels[k].Sources) == 0 && j == nodes-1 {
				chem.Channels[k].Sources = append(chem.Channels[k].Sources, j)
			}
		}
	}
	for i := range nodes {
		for k := range chem.Channels {
			if rnd.IntN(2) == 0 {
				chem.Receptors = append(chem.Receptors, ChemistryReceptor{Node: i, Channel: k})
			}
		}
	}
	c := Config{Nodes: nodes, DT: 1, Activation: activation, Gate: gate, Chemistry: chem, Workers: workers}
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
	f := chemFixture{model: m, initial: normal(nodes, .3, 0), inputs: make([][]float64, steps), up: make([][]float64, steps)}
	r := len(chem.Receptors)
	f.params = Parameters{Weights: normal(edges, .5, 0), Bias: normal(nodes, .2, 0), LogTau: normal(nodes, .2, math.Log(2)),
		ChemRelease: normal(2, 1.5, 0), ChemGain: normal(r, .8, 0), ChemOffset: normal(r, .5, 0)}
	if gate != "" {
		f.params.GateGain, f.params.GateBias = normal(nodes, .8, 0), normal(nodes, .5, 0)
	}
	for i := range f.inputs {
		f.inputs[i], f.up[i] = normal(nodes, 1, 0), normal(nodes, 1, 0)
	}
	return f
}

func (f chemFixture) loss(t *testing.T, p Parameters, initial []float64, inputs [][]float64) float64 {
	t.Helper()
	tr, err := f.model.Forward(context.Background(), p, initial, inputs)
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

// clampCounts reports how many receptor-node steps of a trace had gamma and
// beta outside and inside their range.
func clampCounts(tr *Trace) (gamma, beta, gammaFree, betaFree int) {
	pl := tr.model.chem
	for t := range tr.chemG {
		for i := range tr.chemG[t] {
			if len(pl.nodeReceptors[i]) == 0 {
				continue
			}
			_, gf := clampRange(tr.chemG[t][i], pl.gammaMin, pl.gammaMax)
			_, bf := clampRange(tr.chemB[t][i], -pl.betaAbs, pl.betaAbs)
			if gf {
				gammaFree++
			} else {
				gamma++
			}
			if bf {
				betaFree++
			} else {
				beta++
			}
		}
	}
	return
}

// TestChemistryGradientMatchesFiniteDifferences checks every gradient group of
// a core with chemistry, the inputs and the initial voltage included, against
// central differences of the forward pass: several seeds, with and without
// the gate, tanh and softsign, and a narrow range where the clamps bind.
func TestChemistryGradientMatchesFiniteDifferences(t *testing.T) {
	type variant struct{ gate, activation string }
	worst := map[string]float64{}
	for _, v := range []variant{{"", "tanh"}, {"", "softsign"}, {GateCurrent, "tanh"}, {GateCurrentSoftsign, "softsign"}} {
		for _, narrow := range []bool{false, true} {
			for _, seed := range []uint64{11, 23, 37} {
				name := fmt.Sprintf("%s/%s/seed%d", v.gate, v.activation, seed)
				if narrow {
					name += "/clamped"
				}
				t.Run(name, func(t *testing.T) {
					f := newChemFixture(t, seed, v.gate, v.activation, narrow, 0)
					tr, err := f.model.Forward(context.Background(), f.params, f.initial, f.inputs)
					if err != nil {
						t.Fatal(err)
					}
					gc, bc, gf, bf := clampCounts(tr)
					if narrow && (gc == 0 || bc == 0) {
						t.Fatalf("narrow fixture clamps gamma %d and beta %d times", gc, bc)
					}
					if !narrow && (gc != 0 || bc != 0) {
						t.Fatalf("wide fixture clamps gamma %d and beta %d times", gc, bc)
					}
					if gf == 0 || bf == 0 {
						t.Fatalf("unclamped gamma %d and beta %d node steps", gf, bf)
					}
					g, err := f.model.Backward(context.Background(), tr, f.up, 0)
					if err != nil {
						t.Fatal(err)
					}
					const h = 1e-6
					check := func(group string, values, grad []float64, eval func() float64) {
						t.Helper()
						for i := range values {
							orig := values[i]
							values[i] = orig + h
							plus := eval()
							values[i] = orig - h
							minus := eval()
							values[i] = orig
							want := (plus - minus) / (2 * h)
							rel := math.Abs(grad[i]-want) / (1 + math.Abs(want))
							worst[group] = math.Max(worst[group], rel)
							if rel > 1e-6 {
								t.Fatalf("%s[%d] gradient %.10g, finite difference %.10g", group, i, grad[i], want)
							}
						}
					}
					p := cloneParameters(f.params)
					initial := append([]float64(nil), f.initial...)
					eval := func() float64 { return f.loss(t, p, initial, f.inputs) }
					check("weights", p.Weights, g.Weights, eval)
					check("bias", p.Bias, g.Bias, eval)
					check("log_tau", p.LogTau, g.LogTau, eval)
					check("chem_release", p.ChemRelease, g.ChemRelease, eval)
					check("chem_gain", p.ChemGain, g.ChemGain, eval)
					check("chem_offset", p.ChemOffset, g.ChemOffset, eval)
					if v.gate != "" {
						check("gate_gain", p.GateGain, g.GateGain, eval)
						check("gate_bias", p.GateBias, g.GateBias, eval)
					}
					check("initial", initial, g.Initial, eval)
					inputs := cloneRows(f.inputs)
					for s := range inputs {
						check("input", inputs[s], g.Inputs[s], func() float64 { return f.loss(t, f.params, f.initial, inputs) })
					}
				})
			}
		}
	}
	groups := make([]string, 0, len(worst))
	for group := range worst {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	for _, group := range groups {
		t.Logf("max |grad-fd|/(1+|fd|) %s: %.3g", group, worst[group])
	}
}

// TestChemistryReleaseGradientIsNotTrivial guards the finite-difference test
// against a fixture where chemistry never reaches the loss: the release and
// receptor gradients must be clearly nonzero.
func TestChemistryReleaseGradientIsNotTrivial(t *testing.T) {
	f := newChemFixture(t, 11, "", "tanh", false, 0)
	tr, err := f.model.Forward(context.Background(), f.params, f.initial, f.inputs)
	if err != nil {
		t.Fatal(err)
	}
	g, err := f.model.Backward(context.Background(), tr, f.up, 0)
	if err != nil {
		t.Fatal(err)
	}
	for name, v := range map[string][]float64{"chem_release": g.ChemRelease, "chem_gain": g.ChemGain, "chem_offset": g.ChemOffset} {
		var norm float64
		for _, x := range v {
			norm = math.Hypot(norm, x)
		}
		if norm < 1e-3 {
			t.Fatalf("%s gradient norm %g", name, norm)
		}
	}
}

// TestChemistryMatchesHandComputedStep follows one node through two steps with
// an independent arithmetic of the documented formulas (math.Pow occupancy,
// plain softplus), so a change of order or kinetics fails here.
func TestChemistryMatchesHandComputedStep(t *testing.T) {
	const dt, tau, kd, hill = 0.5, 2.0, 0.3, 1.5
	chem := &Chemistry{Channels: []ChemistryChannel{{Tau: tau, Kd: kd, Hill: hill, Sources: []int{0}}}, Receptors: []ChemistryReceptor{{Node: 0, Channel: 0}}, GammaMin: 0, GammaMax: 5, BetaAbsMax: 2}
	m, err := NewContinuous(Config{Nodes: 1, DT: dt, Activation: "tanh", Chemistry: chem})
	if err != nil {
		t.Fatal(err)
	}
	p := Parameters{Bias: []float64{.1}, LogTau: []float64{math.Log(1.5)}, ChemRelease: []float64{1.7}, ChemGain: []float64{.9}, ChemOffset: []float64{-.4}}
	inputs := [][]float64{{.8}, {-.3}}
	v0 := .25
	tr, err := m.Forward(context.Background(), p, []float64{v0}, inputs)
	if err != nil {
		t.Fatal(err)
	}
	lam, alpha := math.Exp(-dt/1.5), 1-math.Exp(-dt/1.5)
	lamC := math.Exp(-dt / tau)
	v, c := v0, 0.0
	for s, in := range inputs {
		y := math.Tanh(v)
		q := math.Log(1 + math.Exp(1.7*y))
		c = lamC*c + tau*(1-lamC)*q
		occ := math.Pow(c, hill) / (math.Pow(kd, hill) + math.Pow(c, hill))
		gamma, beta := 1+.9*occ, -.4*occ
		drive := gamma*in[0] + beta + .1
		v = lam*v + alpha*drive
		if got := tr.voltage[s+1][0]; math.Abs(got-v) > 1e-12 {
			t.Fatalf("step %d voltage %.15g, hand value %.15g", s, got, v)
		}
		if got := tr.chemConc[s][0]; math.Abs(got-c) > 1e-12 {
			t.Fatalf("step %d concentration %.15g, hand value %.15g", s, got, c)
		}
	}
}

func zeroChemistry(p Parameters) Parameters {
	q := cloneParameters(p)
	clear(q.ChemRelease)
	clear(q.ChemGain)
	clear(q.ChemOffset)
	return q
}

func sameBitsRows(t *testing.T, name string, a, b [][]float64) {
	t.Helper()
	if len(a) != len(b) {
		t.Fatalf("%s: %d rows, want %d", name, len(a), len(b))
	}
	for s := range a {
		sameBitsRow(t, name, a[s], b[s])
	}
}

func sameBitsRow(t *testing.T, name string, a, b []float64) {
	t.Helper()
	if len(a) != len(b) {
		t.Fatalf("%s: length %d, want %d", name, len(a), len(b))
	}
	for i := range a {
		if math.Float64bits(a[i]) != math.Float64bits(b[i]) {
			t.Fatalf("%s[%d] = %v, want %v bit for bit", name, i, a[i], b[i])
		}
	}
}

// TestZeroChemistryIsBitIdentical compares a declared block whose release
// gains, receptor gains and offsets are all zero with the same model without
// the block: outputs, voltages, Infer, InferLast and every shared gradient
// must agree bit for bit, with and without the gate and with workers.
func TestZeroChemistryIsBitIdentical(t *testing.T) {
	for _, tc := range []struct {
		gate, activation string
		workers          int
	}{{"", "tanh", 0}, {"", "softsign", 3}, {GateCurrent, "tanh", 0}, {GateCurrentSoftsign, "softsign", 0}, {GateCurrentSoftsign, "softsign", 2}} {
		t.Run(tc.gate+"/"+tc.activation, func(t *testing.T) {
			ctx := context.Background()
			f := newChemFixture(t, 4, tc.gate, tc.activation, false, tc.workers)
			p := zeroChemistry(f.params)
			// A zero current must keep its sign through the neutral block.
			f.inputs[0][0] = math.Copysign(0, -1)
			declared, err := f.model.Forward(ctx, p, f.initial, f.inputs)
			if err != nil {
				t.Fatal(err)
			}
			c := f.model.Config()
			c.Chemistry = nil
			plain, err := NewContinuous(c)
			if err != nil {
				t.Fatal(err)
			}
			q := cloneParameters(p)
			q.ChemRelease, q.ChemGain, q.ChemOffset = nil, nil, nil
			ref, err := plain.Forward(ctx, q, f.initial, f.inputs)
			if err != nil {
				t.Fatal(err)
			}
			sameBitsRows(t, "output", declared.output, ref.output)
			sameBitsRows(t, "voltage", declared.voltage, ref.voltage)
			sameBitsRows(t, "drive", declared.drive, ref.drive)
			inferred, err := f.model.Infer(ctx, p, f.initial, f.inputs)
			if err != nil {
				t.Fatal(err)
			}
			sameBitsRows(t, "infer", inferred, ref.Outputs())
			last, err := f.model.InferLast(ctx, p, f.initial, f.inputs)
			if err != nil {
				t.Fatal(err)
			}
			sameBitsRow(t, "infer last", last, ref.output[len(ref.output)-1])
			for _, window := range []int{0, 4} {
				gd, err := f.model.BackwardAligned(ctx, declared, f.up, window, EndAlignedOffset(len(f.inputs), window))
				if err != nil {
					t.Fatal(err)
				}
				gr, err := plain.BackwardAligned(ctx, ref, f.up, window, EndAlignedOffset(len(f.inputs), window))
				if err != nil {
					t.Fatal(err)
				}
				sameBitsRow(t, "weights", gd.Weights, gr.Weights)
				sameBitsRow(t, "bias", gd.Bias, gr.Bias)
				sameBitsRow(t, "log_tau", gd.LogTau, gr.LogTau)
				sameBitsRow(t, "gate_gain", gd.GateGain, gr.GateGain)
				sameBitsRow(t, "gate_bias", gd.GateBias, gr.GateBias)
				sameBitsRow(t, "initial", gd.Initial, gr.Initial)
				sameBitsRows(t, "inputs", gd.Inputs, gr.Inputs)
				if gr.ChemRelease != nil || len(gd.ChemRelease) != 2 {
					t.Fatalf("chemistry gradient groups: declared %d, plain %v", len(gd.ChemRelease), gr.ChemRelease)
				}
			}
		})
	}
}

// TestChemistryInferAndWorkersMatchForward checks that the record-free paths
// and the parallel drive run the same modulated trajectory as Forward.
func TestChemistryInferAndWorkersMatchForward(t *testing.T) {
	ctx := context.Background()
	for _, gate := range []string{"", GateCurrentSoftsign} {
		f := newChemFixture(t, 9, gate, "softsign", true, 0)
		ref, err := f.model.Forward(ctx, f.params, f.initial, f.inputs)
		if err != nil {
			t.Fatal(err)
		}
		inferred, err := f.model.Infer(ctx, f.params, f.initial, f.inputs)
		if err != nil {
			t.Fatal(err)
		}
		sameBitsRows(t, "infer", inferred, ref.Outputs())
		c := f.model.Config()
		c.Workers = 3
		parallel, err := NewContinuous(c)
		if err != nil {
			t.Fatal(err)
		}
		got, err := parallel.Forward(ctx, f.params, f.initial, f.inputs)
		if err != nil {
			t.Fatal(err)
		}
		sameBitsRows(t, "workers", got.output, ref.output)
		// An undelayed copy exercises InferLast's in-place output row.
		c.Workers, c.Delays = 0, nil
		undelayed, err := NewContinuous(c)
		if err != nil {
			t.Fatal(err)
		}
		full, err := undelayed.Forward(ctx, f.params, f.initial, f.inputs)
		if err != nil {
			t.Fatal(err)
		}
		last, err := undelayed.InferLast(ctx, f.params, f.initial, f.inputs)
		if err != nil {
			t.Fatal(err)
		}
		sameBitsRow(t, "infer last", last, full.output[len(full.output)-1])
	}
}

// TestChemistryWindowEqualsFullWhenItCoversTheSequence checks truncation at
// its two ends: a window as long as the sequence is the full gradient bit for
// bit, and a short window cuts the concentration and release paths, so the
// release gradient changes.
func TestChemistryWindowEqualsFullWhenItCoversTheSequence(t *testing.T) {
	ctx := context.Background()
	f := newChemFixture(t, 23, "", "tanh", false, 0)
	tr, err := f.model.Forward(ctx, f.params, f.initial, f.inputs)
	if err != nil {
		t.Fatal(err)
	}
	full, err := f.model.Backward(ctx, tr, f.up, 0)
	if err != nil {
		t.Fatal(err)
	}
	long, err := f.model.Backward(ctx, tr, f.up, len(f.inputs))
	if err != nil {
		t.Fatal(err)
	}
	sameBitsRow(t, "chem_release", long.ChemRelease, full.ChemRelease)
	short, err := f.model.Backward(ctx, tr, f.up, 3)
	if err != nil {
		t.Fatal(err)
	}
	same := true
	for k := range short.ChemRelease {
		if short.ChemRelease[k] != full.ChemRelease[k] {
			same = false
		}
	}
	if same {
		t.Fatal("a window of 3 left the release gradient unchanged")
	}
}

func TestChemistryConfigurationAndUnsupportedPaths(t *testing.T) {
	ctx := context.Background()
	chem := func() *Chemistry {
		return &Chemistry{Channels: []ChemistryChannel{{Tau: 2, Kd: 1, Hill: 1, Sources: []int{0}}}, Receptors: []ChemistryReceptor{{Node: 1, Channel: 0}}, GammaMin: .5, GammaMax: 2, BetaAbsMax: 1}
	}
	base := Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, DT: 1, Activation: "tanh"}
	for name, mutate := range map[string]func(*Chemistry){
		"no channel":         func(c *Chemistry) { c.Channels = nil },
		"zero tau":           func(c *Chemistry) { c.Channels[0].Tau = 0 },
		"zero kd":            func(c *Chemistry) { c.Channels[0].Kd = 0 },
		"zero hill":          func(c *Chemistry) { c.Channels[0].Hill = 0 },
		"no source":          func(c *Chemistry) { c.Channels[0].Sources = nil },
		"duplicate source":   func(c *Chemistry) { c.Channels[0].Sources = []int{0, 0} },
		"source range":       func(c *Chemistry) { c.Channels[0].Sources = []int{2} },
		"receptor node":      func(c *Chemistry) { c.Receptors[0].Node = 2 },
		"receptor channel":   func(c *Chemistry) { c.Receptors[0].Channel = 1 },
		"duplicate receptor": func(c *Chemistry) { c.Receptors = append(c.Receptors, c.Receptors[0]) },
		"gamma above one":    func(c *Chemistry) { c.GammaMin = 1.5 },
		"gamma below one":    func(c *Chemistry) { c.GammaMax = .5 },
		"negative beta":      func(c *Chemistry) { c.BetaAbsMax = -1 },
		"non-finite gamma":   func(c *Chemistry) { c.GammaMax = math.Inf(1) },
	} {
		c := chem()
		mutate(c)
		bad := base
		bad.Chemistry = c
		if _, err := NewContinuous(bad); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	plain, err := NewContinuous(base)
	if err != nil {
		t.Fatal(err)
	}
	p := Parameters{Weights: []float64{.5}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}}
	withChem := p
	withChem.ChemRelease, withChem.ChemGain, withChem.ChemOffset = []float64{1}, []float64{.5}, []float64{.1}
	inputs := [][]float64{{1, 0}}
	if _, err := plain.Forward(ctx, withChem, []float64{0, 0}, inputs); err == nil {
		t.Fatal("chemistry parameters accepted without Config.Chemistry")
	}
	declared := base
	declared.Chemistry = chem()
	m, err := NewContinuous(declared)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Forward(ctx, p, []float64{0, 0}, inputs); err == nil {
		t.Fatal("chemistry model ran without its parameters")
	}
	short := withChem
	short.ChemGain = []float64{}
	if _, err := m.Forward(ctx, short, []float64{0, 0}, inputs); err == nil {
		t.Fatal("receptor gain shape mismatch accepted")
	}
	if _, err := m.ForwardRecompute(ctx, withChem, []float64{0, 0}, inputs, 1); err == nil || !strings.Contains(err.Error(), "chemistry") {
		t.Fatalf("recompute: %v", err)
	}
	if _, err := m.BackwardRecompute(ctx, withChem, []float64{0, 0}, inputs, [][]float64{{0, 0}}, 0, 1); err == nil || !strings.Contains(err.Error(), "chemistry") {
		t.Fatalf("backward recompute: %v", err)
	}
	if _, err := m.NewState([]float64{0, 0}); err == nil || !strings.Contains(err.Error(), "chemistry") {
		t.Fatalf("persistent state: %v", err)
	}
	vec := declared
	vec.StateDimension = 1
	if _, err := NewVectorContinuous(vec); err == nil || !strings.Contains(err.Error(), "chemistry") {
		t.Fatalf("vector nodes: %v", err)
	}
	if _, err := NewGPUContinuous(ctx, declared); err == nil || !strings.Contains(err.Error(), "chemistry") {
		t.Fatalf("gpu adapter: %v", err)
	}
	if _, err := NewGPUContinuousBatch(ctx, declared, BatchLayout{ReadoutNodes: []int{1}}); err == nil || !strings.Contains(err.Error(), "chemistry") {
		t.Fatalf("gpu batch: %v", err)
	}
	// The owned configuration is a deep copy.
	got := m.Config()
	got.Chemistry.Channels[0].Sources[0] = 1
	if m.Config().Chemistry.Channels[0].Sources[0] != 0 {
		t.Fatal("Config exposed the owned chemistry")
	}
}

// TestUndeclaredChemistryKeepsJSON pins that a configuration and parameters
// without the block serialize without any chemistry key, so fingerprints of
// existing models are unchanged.
func TestUndeclaredChemistryKeepsJSON(t *testing.T) {
	c := Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, DT: 1, Activation: "tanh"}
	m, err := NewContinuous(c)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(m.Config())
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"nodes":2,"sources":[0],"targets":[1],"delays":[0],"dt":1,"activation":"tanh"}`; string(b) != want {
		t.Fatalf("config JSON %s, want %s", b, want)
	}
	pb, err := json.Marshal(Parameters{Weights: []float64{1}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(pb), `"chem_`) {
		t.Fatalf("parameters JSON %s", pb)
	}
}

// TestZeroChemistryKeepsSignedZeros feeds a negative zero current through a
// receptor node whose bias and voltage are negative zeros as well: a neutral
// block must return the current untouched rather than 1*(-0)+0 = +0.
func TestZeroChemistryKeepsSignedZeros(t *testing.T) {
	ctx := context.Background()
	negZero := math.Copysign(0, -1)
	base := Config{Nodes: 2, Sources: []int{0}, Targets: []int{0}, DT: 1, Activation: "tanh"}
	withChem := base
	withChem.Chemistry = &Chemistry{Channels: []ChemistryChannel{{Tau: 2, Kd: 1, Hill: 1, Sources: []int{0}}}, Receptors: []ChemistryReceptor{{Node: 1, Channel: 0}}, GammaMin: 0, GammaMax: 2, BetaAbsMax: 1}
	plain, err := NewContinuous(base)
	if err != nil {
		t.Fatal(err)
	}
	declared, err := NewContinuous(withChem)
	if err != nil {
		t.Fatal(err)
	}
	p := Parameters{Weights: []float64{.5}, Bias: []float64{.1, negZero}, LogTau: []float64{0, 0}}
	q := p
	q.ChemRelease, q.ChemGain, q.ChemOffset = []float64{0}, []float64{0}, []float64{0}
	initial, inputs := []float64{.3, negZero}, [][]float64{{1, negZero}, {0, negZero}}
	ref, err := plain.Forward(ctx, p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	got, err := declared.Forward(ctx, q, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if !math.Signbit(ref.voltage[2][1]) {
		t.Fatal("fixture lost its negative zero")
	}
	sameBitsRows(t, "voltage", got.voltage, ref.voltage)
}

// TestChemistryWindowCutsTheCarry checks the truncated release gradient
// against an independent computation: with a window boundary at b, the
// gradient of the steps from b on is the gradient of the same loss restricted
// to those steps with the concentration at b-1 and the outputs before b held
// fixed. That restricted gradient is taken by central differences of a
// forward pass in which a perturbed release gain acts only from step b on.
func TestChemistryWindowCutsTheCarry(t *testing.T) {
	ctx := context.Background()
	f := newChemFixture(t, 37, "", "tanh", false, 0)
	// Only the last window carries a loss, so the truncated gradient is the
	// gradient of that window alone.
	const window = 4
	steps := len(f.inputs)
	offset := EndAlignedOffset(steps, window)
	b := WindowStart(steps-1, window, offset)
	up := make([][]float64, steps)
	for s := range up {
		up[s] = make([]float64, f.model.config.Nodes)
		if s >= b {
			copy(up[s], f.up[s])
		}
	}
	tr, err := f.model.Forward(ctx, f.params, f.initial, f.inputs)
	if err != nil {
		t.Fatal(err)
	}
	g, err := f.model.BackwardAligned(ctx, tr, up, window, offset)
	if err != nil {
		t.Fatal(err)
	}
	// Restricted forward: steps before b run with the original release gain
	// k0, steps from b on with the perturbed one; the voltage path into b is
	// cut by holding the voltage at b fixed as well, which the window does.
	restricted := func(k int, delta float64) float64 {
		pl := f.model.chem
		conc := append([]float64(nil), tr.chemConc[b-1]...)
		mean, occ := make([]float64, len(conc)), make([]float64, len(conc))
		n := f.model.config.Nodes
		G, B := make([]float64, n), make([]float64, n)
		p := cloneParameters(f.params)
		p.ChemRelease[k] += delta
		v := append([]float64(nil), tr.voltage[b]...)
		outputs := cloneRows(tr.output)
		var l float64
		for s := b; s < steps; s++ {
			pl.step(p, outputs[s], conc, mean, occ, G, B)
			drive := make([]float64, n)
			copy(drive, f.inputs[s])
			for e, src := range f.model.config.Sources {
				past := max(s-f.model.config.Delays[e], 0)
				drive[f.model.config.Targets[e]] += p.Weights[e] * outputs[past][src]
			}
			for i := range drive {
				d := pl.modulate(drive[i], G[i], B[i]) + p.Bias[i]
				v[i] = tr.lambda[i]*v[i] + tr.alpha[i]*d
				outputs[s+1][i] = math.Tanh(v[i])
				l += up[s][i] * outputs[s+1][i]
			}
		}
		return l
	}
	const h = 1e-6
	for k := range f.params.ChemRelease {
		want := (restricted(k, h) - restricted(k, -h)) / (2 * h)
		if math.Abs(g.ChemRelease[k]-want) > 1e-6*(1+math.Abs(want)) {
			t.Fatalf("truncated chem_release[%d] gradient %.10g, restricted finite difference %.10g", k, g.ChemRelease[k], want)
		}
	}
}
