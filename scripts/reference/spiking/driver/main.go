// Command driver dumps randomized LIF, mixed and vector core cases, their
// forward/backward results and a set of Go-side invariant checks as JSON, so
// that an independent PyTorch reference (../reference.py) can recompute the
// documented math. It is an audit tool only and changes nothing in the repo.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"reflect"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/plasticity"
)

type obj = map[string]any

var ctx = context.Background()

func normal(r *rand.Rand, mu, sd float64) float64 { return mu + sd*r.NormFloat64() }
func unif(r *rand.Rand, a, b float64) float64     { return a + (b-a)*r.Float64() }

func vec(r *rand.Rand, n int, mu, sd float64) []float64 {
	v := make([]float64, n)
	for i := range v {
		v[i] = normal(r, mu, sd)
	}
	return v
}
func mat(r *rand.Rand, t, n int, mu, sd float64) [][]float64 {
	m := make([][]float64, t)
	for i := range m {
		m[i] = vec(r, n, mu, sd)
	}
	return m
}

func topology(r *rand.Rand, n int, maxDelay int) (src, tgt, del []int) {
	e := 3 + r.IntN(3*n)
	for k := 0; k < e; k++ {
		src = append(src, r.IntN(n))
		tgt = append(tgt, r.IntN(n))
		del = append(del, r.IntN(maxDelay+1))
	}
	return
}

func lifConfig(r *rand.Rand, n int) dynamics.LIFConfig {
	src, tgt, del := topology(r, n, 3)
	tmin := unif(r, 0.2, 0.6)
	c := dynamics.LIFConfig{
		Nodes: n, Sources: src, Targets: tgt, Delays: del,
		DT: unif(r, 0.5, 1.0), TauSyn: unif(r, 1, 5),
		ThetaMin: tmin, ThetaMax: tmin + unif(r, 0.3, 1.0),
		VReset:          unif(r, -0.5, tmin-0.1),
		RefractorySteps: r.IntN(3),
		Surrogate:       dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: unif(r, 0.5, 5)},
	}
	if r.IntN(2) == 0 {
		c.Adaptation = dynamics.LIFAdaptation{Enabled: true, TauAdapt: unif(r, 2, 10), Beta: unif(r, 0, 0.5)}
	}
	if r.IntN(2) == 0 {
		c.Homeostasis = &dynamics.LIFHomeostasis{Enabled: true, TauRate: unif(r, 2, 10), TargetRate: unif(r, 0, 0.5), Eta: unif(r, 0, 0.8), HMax: unif(r, 0, 1)}
	}
	return c
}

func lifParams(r *rand.Rand, c dynamics.LIFConfig) dynamics.LIFParameters {
	lt := make([]float64, c.Nodes)
	for i := range lt {
		lt[i] = math.Log(unif(r, 1, 5))
	}
	return dynamics.LIFParameters{
		Weights: vec(r, len(c.Sources), 0, 1.2), Bias: vec(r, c.Nodes, 0.2, 0.3),
		LogTau: lt, ThetaRaw: vec(r, c.Nodes, 0, 1),
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func bitsEqual(a, b any) bool { return reflect.DeepEqual(a, b) }

func jsonRoundTrip[T any](v T) T {
	data, err := json.Marshal(v)
	must(err)
	var out T
	must(json.Unmarshal(data, &out))
	return out
}

func lifCase(r *rand.Rand, seed uint64) obj {
	n := 3 + r.IntN(4)
	c := lifConfig(r, n)
	m, err := dynamics.NewLIF(c)
	must(err)
	p := lifParams(r, c)
	T := 6 + r.IntN(9)
	initial := vec(r, n, 0, 0.3)
	inputs := mat(r, T, n, 0.6, 1.0)
	upstream := mat(r, T, n, 0, 1)
	tr, err := m.Forward(ctx, p, initial, inputs)
	must(err)
	window := 2 + r.IntN(3)
	grads := obj{}
	offset := dynamics.EndAlignedOffset(T, window)
	for _, w := range []int{0, window} {
		off := 0
		if w > 0 {
			off = offset
		}
		g, err := m.BackwardAligned(ctx, tr, upstream, w, off)
		must(err)
		grads[fmt.Sprint(w)] = g
		gr, err := m.BackwardRecomputeAligned(ctx, p, initial, inputs, upstream, w, off, 3)
		must(err)
		grads[fmt.Sprintf("recompute_equal_%d", w)] = bitsEqual(g, gr)
	}
	checks := obj{}
	// Workers bit identity.
	cw := c
	cw.Workers = 3
	mw, err := dynamics.NewLIF(cw)
	must(err)
	trw, err := mw.Forward(ctx, p, initial, inputs)
	must(err)
	checks["workers_forward_equal"] = bitsEqual(tr.Outputs(), trw.Outputs()) && bitsEqual(tr.Spikes(), trw.Spikes()) && bitsEqual(tr.Voltages(), trw.Voltages())
	gw, _ := mw.Backward(ctx, trw, upstream, 0)
	g0, _ := m.Backward(ctx, tr, upstream, 0)
	checks["workers_backward_equal"] = bitsEqual(g0, gw)
	fr, err := m.ForwardRecompute(ctx, p, initial, inputs, 3)
	must(err)
	checks["forward_recompute_equal"] = bitsEqual(fr, tr.Outputs())
	grw, err := mw.BackwardRecompute(ctx, p, initial, inputs, upstream, 0, 2)
	must(err)
	checks["workers_backward_recompute_equal"] = bitsEqual(grw, g0)
	// Advance equals Forward; split resume through JSON equals uninterrupted.
	s0, err := m.NewState(initial)
	must(err)
	sAll, outAll, spAll, err := m.Advance(ctx, p, s0, inputs)
	must(err)
	checks["advance_equals_forward"] = bitsEqual(outAll, tr.Outputs()) && bitsEqual(spAll, tr.Spikes()) && bitsEqual(sAll.Voltage, tr.FinalVoltage())
	k := 1 + r.IntN(T-1)
	s1, o1, sp1, err := m.Advance(ctx, p, s0, inputs[:k])
	must(err)
	s1 = jsonRoundTrip(s1)
	s2, o2, sp2, err := m.Advance(ctx, p, s1, inputs[k:])
	must(err)
	checks["resume_equal"] = bitsEqual(append(o1, o2...), outAll) && bitsEqual(append(sp1, sp2...), spAll) && bitsEqual(jsonRoundTrip(s2), jsonRoundTrip(sAll))
	_, ow, spw, err := mw.Advance(ctx, p, mustState(mw, initial), inputs)
	must(err)
	checks["workers_advance_equal"] = bitsEqual(ow, outAll) && bitsEqual(spw, spAll)
	// Neutral modulation equals nil.
	neutral := &dynamics.Modulation{Gain: ones(T, n), Offset: zeros(T, n), Threshold: zeros(T, n)}
	_, on, spn, err := m.AdvanceModulated(ctx, p, s0, inputs, neutral)
	must(err)
	checks["neutral_modulation_equal"] = bitsEqual(on, outAll) && bitsEqual(spn, spAll)
	// Random modulation, recomputed by the reference.
	mod := &dynamics.Modulation{Gain: mat(r, T, n, 1, 0.4), Offset: mat(r, T, n, 0, 0.3), Threshold: mat(r, T, n, 0, 0.4)}
	_, om, spm, err := m.AdvanceModulated(ctx, p, s0, inputs, mod)
	must(err)
	return obj{
		"seed": seed, "config": m.Config(), "params": p, "initial": initial, "inputs": inputs, "upstream": upstream,
		"window": window, "offset": offset, "outputs": tr.Outputs(), "spikes": tr.Spikes(), "voltages": tr.Voltages(),
		"grads": grads, "checks": checks,
		"modulation": mod, "mod_outputs": om, "mod_spikes": spm,
	}
}

func mustState(m *dynamics.LIF, initial []float64) dynamics.LIFState {
	s, err := m.NewState(initial)
	must(err)
	return s
}

func ones(t, n int) [][]float64 {
	m := zeros(t, n)
	for i := range m {
		for j := range m[i] {
			m[i][j] = 1
		}
	}
	return m
}
func zeros(t, n int) [][]float64 {
	m := make([][]float64, t)
	for i := range m {
		m[i] = make([]float64, n)
	}
	return m
}

func mixedCase(r *rand.Rand, seed uint64) obj {
	n := 3 + r.IntN(4)
	lc := lifConfig(r, n)
	rule := make([]uint8, n)
	nl := 0
	for i := range rule {
		rule[i] = uint8(r.IntN(2))
		nl += int(rule[i])
	}
	if nl == 0 {
		rule[0], nl = 1, 1
	}
	if nl == n {
		rule[n-1], nl = 0, nl-1
	}
	act := []string{"tanh", "softplus"}[r.IntN(2)]
	c := dynamics.MixedConfig{
		Nodes: n, Sources: lc.Sources, Targets: lc.Targets, Delays: lc.Delays, DT: lc.DT, NodeRule: rule,
		Continuous: dynamics.ContinuousRule{Activation: act},
		LIF: dynamics.LIFRule{TauSyn: lc.TauSyn, ThetaMin: lc.ThetaMin, ThetaMax: lc.ThetaMax, VReset: lc.VReset,
			RefractorySteps: lc.RefractorySteps, Adaptation: lc.Adaptation, Homeostasis: lc.Homeostasis, Surrogate: lc.Surrogate},
	}
	m, err := dynamics.NewMixed(c)
	must(err)
	lp := lifParams(r, lc)
	p := dynamics.MixedParameters{Weights: lp.Weights, Bias: lp.Bias, LogTau: lp.LogTau, ThetaRaw: vec(r, nl, 0, 1)}
	T := 6 + r.IntN(9)
	initial := vec(r, n, 0, 0.5)
	inputs := mat(r, T, n, 0.6, 1.0)
	upstream := mat(r, T, n, 0, 1)
	tr, err := m.Forward(ctx, p, initial, inputs)
	must(err)
	window := 2 + r.IntN(3)
	grads := obj{}
	offset := dynamics.EndAlignedOffset(T, window)
	for _, w := range []int{0, window} {
		off := 0
		if w > 0 {
			off = offset
		}
		g, err := m.BackwardAligned(ctx, tr, upstream, w, off)
		must(err)
		grads[fmt.Sprint(w)] = g
	}
	checks := obj{}
	s0, err := m.NewState(initial)
	must(err)
	sAll, outAll, spAll, err := m.Advance(ctx, p, s0, inputs)
	must(err)
	checks["advance_equals_forward"] = bitsEqual(outAll, tr.Outputs()) && bitsEqual(spAll, tr.Spikes())
	k := 1 + r.IntN(T-1)
	s1, o1, sp1, err := m.Advance(ctx, p, s0, inputs[:k])
	must(err)
	s1 = jsonRoundTrip(s1)
	s2, o2, sp2, err := m.Advance(ctx, p, s1, inputs[k:])
	must(err)
	checks["resume_equal"] = bitsEqual(append(o1, o2...), outAll) && bitsEqual(append(sp1, sp2...), spAll) && bitsEqual(jsonRoundTrip(s2), jsonRoundTrip(sAll))
	// All-one assignment reproduces LIF bit for bit (forward and backward).
	allOne := c
	allOne.NodeRule = make([]uint8, n)
	for i := range allOne.NodeRule {
		allOne.NodeRule[i] = 1
	}
	allOne.Continuous = dynamics.ContinuousRule{}
	mo, err := dynamics.NewMixed(allOne)
	must(err)
	lifEq, err := dynamics.NewLIF(lc)
	must(err)
	lpFull := dynamics.LIFParameters{Weights: p.Weights, Bias: p.Bias, LogTau: p.LogTau, ThetaRaw: vec(r, n, 0, 1)}
	tro, err := mo.Forward(ctx, dynamics.MixedParameters{Weights: lpFull.Weights, Bias: lpFull.Bias, LogTau: lpFull.LogTau, ThetaRaw: lpFull.ThetaRaw}, initial, inputs)
	must(err)
	trl, err := lifEq.Forward(ctx, lpFull, initial, inputs)
	must(err)
	gmo, _ := mo.Backward(ctx, tro, upstream, window)
	gl, _ := lifEq.Backward(ctx, trl, upstream, window)
	checks["all_lif_equals_lif"] = bitsEqual(tro.Outputs(), trl.Outputs()) && bitsEqual(tro.Spikes(), trl.Spikes()) &&
		bitsEqual([][]float64{gmo.Weights, gmo.Bias, gmo.LogTau, gmo.ThetaRaw, gmo.Initial}, [][]float64{gl.Weights, gl.Bias, gl.LogTau, gl.ThetaRaw, gl.Initial}) && bitsEqual(gmo.Inputs, gl.Inputs)
	// All-zero assignment reproduces Continuous bit for bit.
	allZero := dynamics.MixedConfig{Nodes: n, Sources: c.Sources, Targets: c.Targets, Delays: c.Delays, DT: c.DT, NodeRule: make([]uint8, n), Continuous: c.Continuous}
	mz, err := dynamics.NewMixed(allZero)
	must(err)
	cz, err := dynamics.NewContinuous(dynamics.Config{Nodes: n, Sources: c.Sources, Targets: c.Targets, Delays: c.Delays, DT: c.DT, Activation: act})
	must(err)
	trz, err := mz.Forward(ctx, dynamics.MixedParameters{Weights: p.Weights, Bias: p.Bias, LogTau: p.LogTau}, initial, inputs)
	must(err)
	trc, err := cz.Forward(ctx, dynamics.Parameters{Weights: p.Weights, Bias: p.Bias, LogTau: p.LogTau}, initial, inputs)
	must(err)
	gz, _ := mz.Backward(ctx, trz, upstream, window)
	gc, _ := cz.Backward(ctx, trc, upstream, window)
	checks["all_continuous_equals_continuous"] = bitsEqual(trz.Outputs(), trc.Outputs()) &&
		bitsEqual([][]float64{gz.Weights, gz.Bias, gz.LogTau, gz.Initial}, [][]float64{gc.Weights, gc.Bias, gc.LogTau, gc.Initial}) && bitsEqual(gz.Inputs, gc.Inputs)
	thr := mat(r, T, n, 0, 0.4)
	for t := range thr {
		for i := range thr[t] {
			if rule[i] == 0 {
				thr[t][i] = 0
			}
		}
	}
	mod := &dynamics.Modulation{Gain: mat(r, T, n, 1, 0.4), Offset: mat(r, T, n, 0, 0.3), Threshold: thr}
	_, om, spm, err := m.AdvanceModulated(ctx, p, s0, inputs, mod)
	must(err)
	return obj{
		"seed": seed, "config": m.Config(), "node_rule": rule, "params": p, "initial": initial, "inputs": inputs, "upstream": upstream,
		"window": window, "offset": offset, "outputs": tr.Outputs(), "spikes": tr.Spikes(), "voltages": tr.Voltages(),
		"grads": grads, "checks": checks, "modulation": mod, "mod_outputs": om, "mod_spikes": spm,
	}
}

func vectorCase(r *rand.Rand, seed uint64) obj {
	n := 2 + r.IntN(4)
	C := 2 + r.IntN(2)
	src, tgt, del := topology(r, n, 3)
	shape := []string{"scalar", "matrix"}[r.IntN(2)]
	act := []string{"tanh", "softplus"}[r.IntN(2)]
	c := dynamics.Config{Nodes: n, Sources: src, Targets: tgt, Delays: del, DT: unif(r, 0.5, 1), Activation: act, StateDimension: C, EdgeShape: shape}
	m, err := dynamics.NewVectorContinuous(c)
	must(err)
	l, _ := dynamics.NewVectorLayout(n, len(src), C, shape)
	lt := make([]float64, n)
	for i := range lt {
		lt[i] = math.Log(unif(r, 1, 5))
	}
	p := dynamics.VectorParameters{Weights: vec(r, l.WeightValues(), 0, 0.8), Bias: vec(r, n*C, 0, 0.3), LogTau: lt}
	T := 5 + r.IntN(8)
	initial := vec(r, n*C, 0, 0.5)
	inputs := mat(r, T, n*C, 0, 1)
	upstream := mat(r, T, n*C, 0, 1)
	tr, err := m.Forward(ctx, p, initial, inputs)
	must(err)
	window := 2 + r.IntN(3)
	grads := obj{}
	offset := dynamics.EndAlignedOffset(T, window)
	for _, w := range []int{0, window} {
		off := 0
		if w > 0 {
			off = offset
		}
		g, err := m.BackwardAligned(ctx, tr, upstream, w, off)
		must(err)
		grads[fmt.Sprint(w)] = g
	}
	checks := obj{}
	s0, err := m.NewState(initial)
	must(err)
	sAll, outAll, err := m.Advance(ctx, p, s0, inputs)
	must(err)
	checks["advance_equals_forward"] = bitsEqual(outAll, tr.Outputs()[1:])
	k := 1 + r.IntN(T-1)
	s1, o1, err := m.Advance(ctx, p, s0, inputs[:k])
	must(err)
	s1 = jsonRoundTrip(s1)
	s2, o2, err := m.Advance(ctx, p, s1, inputs[k:])
	must(err)
	checks["resume_equal"] = bitsEqual(append(o1, o2...), outAll) && bitsEqual(jsonRoundTrip(s2), jsonRoundTrip(sAll))
	// C=1 reduction check is the scalar agent's; here we only check workers.
	cw := c
	cw.Workers = 3
	mw, err := dynamics.NewVectorContinuous(cw)
	if err == nil {
		trw, err := mw.Forward(ctx, p, initial, inputs)
		must(err)
		checks["workers_forward_equal"] = bitsEqual(trw.Outputs(), tr.Outputs())
	}
	return obj{
		"seed": seed, "config": c, "params": p, "initial": initial, "inputs": inputs, "upstream": upstream,
		"window": window, "offset": offset, "outputs": tr.Outputs(), "voltages": tr.Voltages(), "grads": grads, "checks": checks,
	}
}

// netCase builds a learning.Network over a LIF, mixed or vector core and
// dumps LossGradient (MSE on the last readout) plus LossGradientFrom with
// ReadoutEveryStep.
func netCase(r *rand.Rand, seed uint64, kind string) obj {
	n := 4 + r.IntN(3)
	lc := lifConfig(r, n)
	cfg := learning.Config{InputSize: 2, OutputSize: 2}
	var p learning.Parameters
	lt := make([]float64, n)
	for i := range lt {
		lt[i] = math.Log(unif(r, 1, 5))
	}
	C := 1
	switch kind {
	case "lif":
		cfg.LIF = &lc
		p.ThetaRaw = vec(r, n, 0, 1)
	case "mixed":
		rule := make([]uint8, n)
		nl := 0
		for i := range rule {
			rule[i] = uint8(r.IntN(2))
			nl += int(rule[i])
		}
		if nl == 0 {
			rule[0], nl = 1, 1
		}
		if nl == n {
			rule[n-1], nl = 0, nl-1
		}
		cfg.Mixed = &dynamics.MixedConfig{Nodes: n, Sources: lc.Sources, Targets: lc.Targets, Delays: lc.Delays, DT: lc.DT, NodeRule: rule,
			Continuous: dynamics.ContinuousRule{Activation: "tanh"},
			LIF: dynamics.LIFRule{TauSyn: lc.TauSyn, ThetaMin: lc.ThetaMin, ThetaMax: lc.ThetaMax, VReset: lc.VReset,
				RefractorySteps: lc.RefractorySteps, Adaptation: lc.Adaptation, Homeostasis: lc.Homeostasis, Surrogate: lc.Surrogate}}
		p.ThetaRaw = vec(r, nl, 0, 1)
	case "vector":
		C = 2
		cfg.Dynamics = dynamics.Config{Nodes: n, Sources: lc.Sources, Targets: lc.Targets, Delays: lc.Delays, DT: lc.DT, Activation: "tanh", StateDimension: C, EdgeShape: []string{"scalar", "matrix"}[r.IntN(2)]}
	}
	E := len(lc.Sources)
	wcount := E
	if kind == "vector" && cfg.Dynamics.EdgeShape == "matrix" {
		wcount = E * C * C
	}
	cfg.InputNodes = []int{0, 1}
	cfg.ReadoutNodes = []int{n - 1, n - 2}
	if r.IntN(2) == 0 && kind != "vector" {
		signs := make([]int8, E)
		for i := range signs {
			signs[i] = int8(r.IntN(3) - 1)
		}
		cfg.EdgeSigns = signs
	}
	p.Core = dynamics.Parameters{Weights: vec(r, wcount, 0, 1.0), Bias: vec(r, n*C, 0.2, 0.3), LogTau: lt}
	p.Encoder = vec(r, 2*2*C, 0.8, 0.5)
	p.Readout = vec(r, 2*C*2, 0, 1)
	T := 6 + r.IntN(7)
	input := mat(r, T, 2, 0.8, 1)
	target := vec(r, 2, 0, 1)
	net, err := learning.NewNetwork(cfg)
	must(err)
	window := 0
	if r.IntN(2) == 0 {
		window = 2 + r.IntN(3)
	}
	loss, g, err := net.LossGradient(ctx, p, input, target, window)
	must(err)
	pred, err := net.Predict(ctx, p, input)
	must(err)
	cfg2 := net.Config()
	cfg2.ReadoutEveryStep = true
	net2, err := learning.NewNetwork(cfg2)
	must(err)
	up := mat(r, T, 2, 0, 1)
	g2, err := net2.LossGradientFrom(ctx, p, input, up, window)
	must(err)
	return obj{"seed": seed, "kind": kind, "config": net.Config(), "params": p, "input": input, "target": target, "window": window,
		"loss": loss, "grad": g, "pred": pred, "upstream_every": up, "grad_every": g2}
}

func plasticCase(r *rand.Rand, seed uint64) obj {
	n := 3 + r.IntN(4)
	src, tgt, _ := topology(r, n, 0)
	E := len(src)
	var edges []int
	for e := 0; e < E; e++ {
		if r.IntN(3) != 0 {
			edges = append(edges, e)
		}
	}
	if len(edges) == 0 {
		edges = []int{0}
	}
	kind := []string{plasticity.RuleHebbianRate, plasticity.RuleSTDPPair}[r.IntN(2)]
	rule := plasticity.Rule{Kind: kind, DecayE: unif(r, 0, 0.99), DecayP: unif(r, 0, 0.99), PlasticMax: unif(r, 0.05, 0.5), WMin: unif(r, 0.01, 0.2)}
	if kind == plasticity.RuleSTDPPair {
		rule.DecayPre, rule.DecayPost, rule.APlus, rule.AMinus = unif(r, 0, 0.99), unif(r, 0, 0.99), unif(r, 0, 1), unif(r, 0, 1)
	}
	m, err := plasticity.New(plasticity.Config{Rule: rule, Edges: edges}, E)
	must(err)
	s := m.NewState()
	steps := 8
	var seq []obj
	for t := 0; t < steps; t++ {
		pre, post := vec(r, n, 0.3, 0.5), vec(r, n, 0.3, 0.5)
		sp, sq := make([]float64, n), make([]float64, n)
		for i := range sp {
			sp[i], sq[i] = float64(r.IntN(2)), float64(r.IntN(2))
		}
		gate := normal(r, 0, 1)
		in := plasticity.StepInput{Pre: pre, Post: post, Gate: gate}
		if kind == plasticity.RuleSTDPPair {
			in.SpikesPre, in.SpikesPost = sp, sq
		}
		var override *float64
		if r.IntN(3) == 0 {
			d := unif(r, 0, 0.99)
			override = &d
			in.DecayE = override
		}
		next, rep, err := m.StepWith(s, in, src, tgt)
		must(err)
		seq = append(seq, obj{"pre": pre, "post": post, "spikes_pre": sp, "spikes_post": sq, "gate": gate, "decay_e": override, "state": next, "clamped": rep.Clamped})
		s = next
	}
	base := vec(r, E, 0, 1)
	signs := make([]int8, E)
	for i := range signs {
		signs[i] = int8(r.IntN(3) - 1)
	}
	slow := vec(r, len(edges), 0, 0.2)
	eff, rep, err := m.EffectiveWith(base, signs, s, slow)
	must(err)
	eff0, rep0, err := m.EffectiveWith(base, signs, s, nil)
	must(err)
	return obj{"seed": seed, "rule": rule, "edges": edges, "sources": src, "targets": tgt, "nodes": n, "steps": seq,
		"base": base, "signs": signs, "slow": slow, "effective": eff, "held": rep.HeldAtWMin, "effective_noslow": eff0, "held_noslow": rep0.HeldAtWMin}
}

func main() {
	out := flag.String("out", "cases.json", "output path")
	count := flag.Int("n", 40, "cases per family")
	base := flag.Uint64("seed", 1, "base seed")
	flag.Parse()
	all := obj{}
	var lif, mixed, vectorCases, nets, plastic []obj
	for k := 0; k < *count; k++ {
		seed := *base*1000 + uint64(k)
		r := rand.New(rand.NewPCG(seed, 7))
		lif = append(lif, lifCase(r, seed))
		mixed = append(mixed, mixedCase(r, seed))
		vectorCases = append(vectorCases, vectorCase(r, seed))
		for _, kind := range []string{"lif", "mixed", "vector"} {
			nets = append(nets, netCase(r, seed, kind))
		}
		plastic = append(plastic, plasticCase(r, seed))
	}
	all["lif"], all["mixed"], all["vector"], all["net"], all["plastic"] = lif, mixed, vectorCases, nets, plastic
	data, err := json.Marshal(all)
	must(err)
	must(os.WriteFile(*out, data, 0o644))
	fmt.Println("wrote", *out)
}
