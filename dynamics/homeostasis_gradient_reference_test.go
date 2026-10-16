package dynamics

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"
)

// This sequence has a fixed effect and no activity feedback. Positive threshold
// shifts never reach the coupled floor in these fixtures.
type homeostasisGradientSequence struct {
	nodes int
	shift float64
}

func (s homeostasisGradientSequence) Forward(context.Context, int, []float64) (*Modulation, error) {
	row := make([]float64, s.nodes)
	for i := range row {
		row[i] = s.shift
	}
	return &Modulation{Threshold: [][]float64{row}}, nil
}
func (s homeostasisGradientSequence) NewReverse(context.Context, int, int) (ModulationReverse, error) {
	return s, nil
}
func (s homeostasisGradientSequence) BackwardStep(context.Context, int, ModulationGradient) ([]ActivityGradient, error) {
	return nil, nil
}
func (s homeostasisGradientSequence) Checkpoint(context.Context, int) (ModulationRestart, error) {
	return func(context.Context) (ModulationSequence, error) { return s, nil }, nil
}

func homeostasisGradientReference(c LIFConfig, p LIFParameters, initial []float64, inputs, up [][]float64) (LIFGradient, [][]float64) {
	g := LIFGradient{Weights: make([]float64, len(p.Weights)), Bias: make([]float64, c.Nodes), LogTau: make([]float64, c.Nodes), ThetaRaw: make([]float64, c.Nodes), Initial: make([]float64, c.Nodes), Inputs: make([][]float64, len(inputs))}
	for t := range inputs {
		g.Inputs[t] = make([]float64, c.Nodes)
	}
	probe := func(seed func(*lifDirection)) float64 {
		d := newLIFDirection(c, len(inputs))
		seed(&d)
		v, _ := lifForwardTangent(c, p, initial, inputs, up, d)
		return v
	}
	for e := range g.Weights {
		g.Weights[e] = probe(func(d *lifDirection) { d.weights[e] = 1 })
	}
	for i := 0; i < c.Nodes; i++ {
		g.Bias[i] = probe(func(d *lifDirection) { d.bias[i] = 1 })
		g.LogTau[i] = probe(func(d *lifDirection) { d.logTau[i] = 1 })
		g.ThetaRaw[i] = probe(func(d *lifDirection) { d.thetaRaw[i] = 1 })
		g.Initial[i] = probe(func(d *lifDirection) { d.initial[i] = 1 })
		for t := range inputs {
			g.Inputs[t][i] = probe(func(d *lifDirection) { d.inputs[t][i] = 1 })
		}
	}
	_, out := lifForwardTangent(c, p, initial, inputs, up, newLIFDirection(c, len(inputs)))
	return g, out
}

func homeostasisGradientRows(t *testing.T, label string, got, want [][]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s rows %d, want %d", label, len(got), len(want))
	}
	for r := range want {
		if len(got[r]) != len(want[r]) {
			t.Fatalf("%s row %d width differs", label, r)
		}
		for i, w := range want[r] {
			v := got[r][i]
			if math.IsNaN(v) || math.IsInf(v, 0) || math.IsNaN(w) || math.IsInf(w, 0) || math.Abs(v-w) > lifTol {
				t.Fatalf("%s[%d][%d] %.17g, want %.17g", label, r, i, v, w)
			}
		}
	}
}

func homeostasisGradientEqual(t *testing.T, got, want LIFGradient) {
	t.Helper()
	homeostasisGradientRows(t, "parameter gradients", [][]float64{got.Weights, got.Bias, got.LogTau, got.ThetaRaw, got.Initial}, [][]float64{want.Weights, want.Bias, want.LogTau, want.ThetaRaw, want.Initial})
	homeostasisGradientRows(t, "input gradients", got.Inputs, want.Inputs)
}

func TestHomeostasisGradientHandReference(t *testing.T) {
	c := LIFConfig{Nodes: 1, DT: math.Ln2, TauSyn: 1, ThetaMin: 0, ThetaMax: 2, VReset: -1, Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 1}, Homeostasis: &LIFHomeostasis{Enabled: true, TauRate: 2 * math.Ln2, TargetRate: .25, Eta: 1 / math.Ln2, HMax: .4}}
	p := LIFParameters{Bias: []float64{0}, LogTau: []float64{0}, ThetaRaw: []float64{0}}
	initial, inputs, up := []float64{0}, [][]float64{{4}, {15.0 / 4}}, [][]float64{{0}, {1}}
	d := newLIFDirection(c, 2)
	d.inputs[0][0] = 1
	got, out := lifForwardTangent(c, p, initial, inputs, up, d)
	// Step 0: candidate=2, psi=1/4, dx=1/8, r=1/2, h=1/4.
	// Step 1: reset detaches candidate, so dx=(1/2)*(1/8)=1/16.
	// Incorrect dh=1/16 would subtract (64/81)/16, giving 17/1296.
	homeostasisGradientRows(t, "hand tangent", [][]float64{{got}}, [][]float64{{1.0 / 16}})
	homeostasisGradientRows(t, "hand outputs", out, [][]float64{{1}, {1.5}})
	m, err := NewLIF(c)
	if err != nil {
		t.Fatal(err)
	}
	tr, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	g, err := m.Backward(context.Background(), tr, up, 0)
	if err != nil {
		t.Fatal(err)
	}
	homeostasisGradientRows(t, "public hand tangent", [][]float64{{g.Inputs[0][0]}}, [][]float64{{1.0 / 16}})
}

func TestHomeostasisGradientLegacyReference(t *testing.T) {
	c, p, initial, inputs, up := lifGradientFixture()
	g, out := homeostasisGradientReference(c, p, initial, inputs, up)
	c.Homeostasis = &LIFHomeostasis{Enabled: false, TauRate: 2, TargetRate: .25, Eta: .5, HMax: .4}
	disabled, disabledOut := homeostasisGradientReference(c, p, initial, inputs, up)
	if !reflect.DeepEqual(g, disabled) || !reflect.DeepEqual(out, disabledOut) {
		t.Fatal("disabled reference differs")
	}
	b, err := json.Marshal([]any{g, out})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("legacy_reference_sha256=%x", sha256.Sum256(b))
}

func TestHomeostasisGradientPublicReference(t *testing.T) {
	type fixture struct {
		name       string
		c          LIFConfig
		p          LIFParameters
		initial    []float64
		inputs, up [][]float64
	}
	hc := lifHomeostasisHandConfig()
	hp, hInitial, hi := lifHomeostasisHandInputs()
	hu := make([][]float64, len(hi))
	for s := range hu {
		hu[s] = []float64{.3 - float64(s)*.07, -.2 + float64(s)*.08}
	}
	c, p, initial, inputs, up := lifGradientFixture()
	c.Homeostasis = &LIFHomeostasis{Enabled: true, TauRate: 1, TargetRate: .15, Eta: .75, HMax: .25}
	fixtures := []fixture{{"hand_clamps_fall", hc, hp, hInitial, hi, hu}, {"adaptation_refractory", c, p, initial, inputs, up}}
	for _, seed := range []uint64{3, 8} {
		for _, adapt := range []bool{false, true} {
			for _, refractory := range []int{0, 2} {
				c, p, initial, inputs := lifRandomFixture(seed, adapt, refractory)
				c.Homeostasis = &LIFHomeostasis{Enabled: true, TauRate: 2 * c.DT, TargetRate: .2, Eta: .6 / c.DT, HMax: .3}
				up := make([][]float64, len(inputs))
				for s := range up {
					up[s] = make([]float64, c.Nodes)
					for i := range up[s] {
						up[s][i] = .1 * float64((s+2*i)%7-3)
					}
				}
				fixtures = append(fixtures, fixture{fmt.Sprintf("seed%d_adapt%v_refractory%d", seed, adapt, refractory), c, p, initial, inputs, up})
			}
		}
	}
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			for _, mode := range []string{"ordinary", "neutral", "threshold"} {
				t.Run(mode, func(t *testing.T) {
					ctx := context.Background()
					shift := 0.0
					if mode == "threshold" {
						shift = .125
					}
					reference := f.c
					reference.ThetaMin += shift
					reference.ThetaMax += shift
					want, outputs := homeostasisGradientReference(reference, f.p, f.initial, f.inputs, f.up)
					m, err := NewLIF(f.c)
					if err != nil {
						t.Fatal(err)
					}
					var sequence RecomputableModulationSequence
					if mode != "ordinary" {
						sequence = homeostasisGradientSequence{f.c.Nodes, shift}
					}
					t.Run("lif_full", func(t *testing.T) {
						tr, err := m.ForwardCoupled(ctx, f.p, f.initial, f.inputs, sequence)
						if err != nil {
							t.Fatal(err)
						}
						homeostasisGradientRows(t, "outputs", tr.Outputs(), outputs)
						g, err := m.Backward(ctx, tr, f.up, 0)
						if err != nil {
							t.Fatal(err)
						}
						homeostasisGradientEqual(t, g, want)
					})
					for _, segment := range []int{1, 3} {
						t.Run(fmt.Sprintf("lif_segment%d", segment), func(t *testing.T) {
							var g LIFGradient
							var out [][]float64
							var err error
							if sequence == nil {
								out, err = m.ForwardRecompute(ctx, f.p, f.initial, f.inputs, segment)
								if err != nil {
									t.Fatal(err)
								}
								g, err = m.BackwardRecompute(ctx, f.p, f.initial, f.inputs, f.up, 0, segment)
							} else {
								tr, e := m.ForwardCoupledRecompute(ctx, f.p, f.initial, f.inputs, sequence, segment)
								if e != nil {
									t.Fatal(e)
								}
								out = tr.Outputs()
								g, err = m.BackwardCoupledRecompute(ctx, tr, f.up, 0)
							}
							if err != nil {
								t.Fatal(err)
							}
							homeostasisGradientRows(t, "outputs", out, outputs)
							homeostasisGradientEqual(t, g, want)
						})
					}
					t.Run("mixed_all_lif", func(t *testing.T) {
						rules := make([]uint8, f.c.Nodes)
						for i := range rules {
							rules[i] = RuleLIF
						}
						m, err := NewMixed(MixedConfig{Nodes: f.c.Nodes, Sources: f.c.Sources, Targets: f.c.Targets, Delays: f.c.Delays, DT: f.c.DT, NodeRule: rules, LIF: LIFRule{TauSyn: f.c.TauSyn, ThetaMin: f.c.ThetaMin, ThetaMax: f.c.ThetaMax, VReset: f.c.VReset, RefractorySteps: f.c.RefractorySteps, Adaptation: f.c.Adaptation, Homeostasis: f.c.Homeostasis, Surrogate: f.c.Surrogate}})
						if err != nil {
							t.Fatal(err)
						}
						p := MixedParameters{Weights: f.p.Weights, Bias: f.p.Bias, LogTau: f.p.LogTau, ThetaRaw: f.p.ThetaRaw}
						tr, err := m.ForwardCoupled(ctx, p, f.initial, f.inputs, sequence)
						if err != nil {
							t.Fatal(err)
						}
						homeostasisGradientRows(t, "outputs", tr.Outputs(), outputs)
						g, err := m.Backward(ctx, tr, f.up, 0)
						if err != nil {
							t.Fatal(err)
						}
						homeostasisGradientEqual(t, LIFGradient{Weights: g.Weights, Bias: g.Bias, LogTau: g.LogTau, ThetaRaw: g.ThetaRaw, Initial: g.Initial, Inputs: g.Inputs}, want)
					})
				})
			}
		})
	}
}
