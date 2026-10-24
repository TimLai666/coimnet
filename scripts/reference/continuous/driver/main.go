// Command driver dumps random scalar continuous-core and learning.Network /
// learning.Trainer cases as JSON for the independent PyTorch reference in
// ../reference.py, and checks internal bit-identity claims (Workers>1,
// recompute segments, StepFrom with the MSE upstream, stateful Advance
// splits) directly in Go. It is an audit tool, not part of the framework.
//
//	go run ./scripts/reference/continuous/driver -cases 200 -out runs/.tmp/ref-continuous/cases.json
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

	"github.com/HazelnutParadise/insyra/nn"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

type stepRecord struct {
	Input    [][]float64               `json:"input"`
	Target   []float64                 `json:"target"`
	Before   learning.TrainingSnapshot `json:"before"`
	Loss     float64                   `json:"loss"`
	Grad     gradJSON                  `json:"grad"`
	Result   learning.StepResult       `json:"result"`
	After    learning.TrainingSnapshot `json:"after"`
	Error    string                    `json:"error,omitempty"`
	Internal []string                  `json:"internal_mismatch,omitempty"`
}

type gradJSON struct {
	Weights, Bias, LogTau, Initial []float64
	CoreInputs                     [][]float64
	Encoder, Readout               []float64
	Inputs                         [][]float64
}

func toJSON(g learning.Gradient) gradJSON {
	return gradJSON{g.Core.Weights, g.Core.Bias, g.Core.LogTau, g.Core.Initial, g.Core.Inputs, g.Encoder, g.Readout, g.Inputs}
}

type netCase struct {
	Kind     string              `json:"kind"`
	Seed     uint64              `json:"seed"`
	Config   learning.Config     `json:"config"`
	Params   learning.Parameters `json:"params"`
	Options  learning.Options    `json:"options"`
	Window   int                 `json:"window"`
	Offset   int                 `json:"offset"`
	Input    [][]float64         `json:"input"`
	Target   []float64           `json:"target"`
	Upstream [][]float64         `json:"upstream"`
	Loss     float64             `json:"loss"`
	Grad     gradJSON            `json:"grad"`
	GradFrom gradJSON            `json:"grad_from"`
	Steps    []stepRecord        `json:"steps"`
	Error    string              `json:"error,omitempty"`
	Internal []string            `json:"internal_mismatch,omitempty"`
}

type coreCase struct {
	Kind     string              `json:"kind"`
	Seed     uint64              `json:"seed"`
	Config   dynamics.Config     `json:"config"`
	Params   dynamics.Parameters `json:"params"`
	Initial  []float64           `json:"initial"`
	Inputs   [][]float64         `json:"inputs"`
	Upstream [][]float64         `json:"upstream"`
	Window   int                 `json:"window"`
	Offset   int                 `json:"offset"`
	Outputs  [][]float64         `json:"outputs"`
	Grad     dynamics.Gradient   `json:"grad"`
	// Stateful: Prefix inputs advance from NewState(Initial), then a
	// differentiable segment of Inputs from that state.
	Prefix    [][]float64       `json:"prefix,omitempty"`
	State     *dynamics.State   `json:"state,omitempty"`
	SegOut    [][]float64       `json:"seg_outputs,omitempty"`
	SegGrad   dynamics.Gradient `json:"seg_grad"`
	SegWindow int               `json:"seg_window"`
	Error     string            `json:"error,omitempty"`
	Internal  []string          `json:"internal_mismatch,omitempty"`
}

func normals(r *rand.Rand, n int, mu, sd float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = mu + sd*r.NormFloat64()
	}
	return out
}
func matrix(r *rand.Rand, rows, cols int, sd float64) [][]float64 {
	out := make([][]float64, rows)
	for i := range out {
		out[i] = normals(r, cols, 0, sd)
	}
	return out
}

func randomTopology(r *rand.Rand, workers int) dynamics.Config {
	n := 5 + r.IntN(8)
	e := r.IntN(3*n + 1)
	c := dynamics.Config{Nodes: n, DT: 0.1 + 1.4*r.Float64(), Workers: workers}
	if r.IntN(2) == 0 {
		c.Activation = "tanh"
	} else {
		c.Activation = "softplus"
	}
	delays := r.IntN(3) != 0
	for i := 0; i < e; i++ {
		s, t := r.IntN(n), r.IntN(n)
		if i > 0 && r.IntN(6) == 0 { // duplicate of an earlier edge
			k := r.IntN(i)
			s, t = c.Sources[k], c.Targets[k]
		}
		if r.IntN(8) == 0 {
			t = s // self loop
		}
		c.Sources = append(c.Sources, s)
		c.Targets = append(c.Targets, t)
		c.Delays = append(c.Delays, r.IntN(4))
	}
	if !delays {
		c.Delays = nil
	}
	return c
}

func subset(r *rand.Rand, n, k int) []int {
	p := r.Perm(n)
	return append([]int(nil), p[:k]...)
}

func randomNetCase(seed uint64) netCase {
	r := rand.New(rand.NewPCG(seed, 99))
	dc := randomTopology(r, 0)
	n, e := dc.Nodes, len(dc.Sources)
	c := learning.Config{Dynamics: dc, InputSize: 1 + r.IntN(4), OutputSize: 1 + r.IntN(3)}
	c.ReadoutNodes = subset(r, n, 1+r.IntN(n))
	if r.IntN(2) == 0 {
		c.InputNodes = subset(r, n, 1+r.IntN(n))
	}
	c.ReadoutEveryStep = r.IntN(3) == 0
	p := learning.Parameters{}
	p.Core.Weights = normals(r, e, 0, 0.6)
	p.Core.Bias = normals(r, n, 0, 0.3)
	p.Core.LogTau = normals(r, n, 0.3, 0.4)
	if e > 0 && r.IntN(2) == 0 {
		c.EdgeSigns = make([]int8, e)
		for i := range c.EdgeSigns {
			c.EdgeSigns[i] = int8(r.IntN(3) - 1)
			if c.EdgeSigns[i] != 0 {
				p.Core.Weights[i] = -1 + 0.6*r.NormFloat64()
			}
		}
		if r.IntN(2) == 0 {
			c.MinLogMagnitude = -1.2 // close enough to be hit by updates
		}
	}
	width := n
	if c.InputNodes != nil {
		width = len(c.InputNodes)
	}
	p.Encoder = normals(r, c.InputSize*width, 0, 0.7)
	p.Readout = normals(r, len(c.ReadoutNodes)*c.OutputSize, 0, 0.7)
	if r.IntN(3) == 0 {
		s := &learning.ParameterSharing{Weights: fill(e), Bias: fill(n), LogTau: fill(n)}
		g := int32(0)
		if e >= 3 {
			// a weight group whose members share one sign class
			first := r.IntN(e)
			sign := int8(0)
			if c.EdgeSigns != nil {
				sign = c.EdgeSigns[first]
			}
			members := []int{first}
			for i := 0; i < e && len(members) < 3; i++ {
				if i != first && (c.EdgeSigns == nil || c.EdgeSigns[i] == sign) && r.IntN(2) == 0 {
					members = append(members, i)
				}
			}
			if len(members) >= 2 {
				for _, m := range members {
					s.Weights[m] = g
					p.Core.Weights[m] = p.Core.Weights[first]
				}
				g++
			}
		}
		b := subset(r, n, 2)
		for _, m := range b {
			s.Bias[m] = g
			p.Core.Bias[m] = p.Core.Bias[b[0]]
		}
		g++
		t := subset(r, n, 3)
		for _, m := range t {
			s.LogTau[m] = g
			p.Core.LogTau[m] = p.Core.LogTau[t[0]]
		}
		g++
		s.Groups = int(g)
		c.Sharing = s
	}
	o := learning.DefaultOptions()
	o.LearningRate = 0.005 + 0.05*r.Float64()
	o.Beta1 = []float64{0.9, 0.5, 0}[r.IntN(3)]
	o.Beta2 = []float64{0.999, 0.9}[r.IntN(2)]
	o.Epsilon = []float64{1e-8, 1e-3}[r.IntN(2)]
	o.WeightDecay = []float64{0, 0.01, 0.3}[r.IntN(3)]
	o.ClipNorm = []float64{0, 0.05, 1}[r.IntN(3)]
	o.Truncation = r.IntN(4)
	if o.Truncation > 0 && seed%2 == 0 {
		o.TruncationAlign = learning.TruncationAlignEnd
	}
	o.Trainable = learning.Trainable{Encoder: r.IntN(5) != 0, Weights: r.IntN(5) != 0, Bias: r.IntN(5) != 0, Tau: r.IntN(5) != 0, Readout: r.IntN(5) != 0}
	if r.IntN(3) == 0 {
		m := &learning.UpdateMasks{}
		if r.IntN(2) == 0 && e > 0 {
			m.Edges = make([]bool, e)
			for i := range m.Edges {
				m.Edges[i] = r.IntN(3) != 0
			}
		}
		if r.IntN(2) == 0 {
			m.Nodes = make([]bool, n)
			for i := range m.Nodes {
				m.Nodes[i] = r.IntN(3) != 0
			}
		}
		o.Masks = m
	}
	if r.IntN(3) == 0 {
		o.Ranges = &learning.ParameterRanges{WeightMagnitudeMax: []float64{0, 0.5, 1.2}[r.IntN(3)], BiasAbsMax: []float64{0, 0.25}[r.IntN(2)], LogTauMin: []float64{0, -0.1}[r.IntN(2)], LogTauMax: []float64{0, 0.6}[r.IntN(2)]}
	}
	o.LossScale = []float64{0, 2, 3, 1024}[r.IntN(4)]
	o.AccumulateSteps = r.IntN(4)
	switch r.IntN(4) {
	case 1:
		o.Schedule = &learning.Schedule{Kind: "constant", WarmupUpdates: 2}
	case 2:
		o.Schedule = &learning.Schedule{Kind: "step", WarmupUpdates: uint64(r.IntN(2)), StepEvery: 2, StepFactor: 0.5}
	case 3:
		o.Schedule = &learning.Schedule{Kind: "cosine", WarmupUpdates: uint64(r.IntN(3)), DecayUpdates: 3, FinalFactor: 0.2}
	}
	steps := 1 + r.IntN(9)
	cs := netCase{Kind: "net", Seed: seed, Config: c, Params: p, Options: o, Window: r.IntN(4)}
	cs.Offset = seedOffset(seed, cs.Window)
	cs.Input = matrix(r, steps, c.InputSize, 0.8)
	cs.Target = normals(r, c.OutputSize, 0, 0.5)
	cs.Upstream = matrix(r, steps, c.OutputSize, 0.5)
	ctx := context.Background()
	net, err := learning.NewNetwork(c)
	if err != nil {
		cs.Error = "network: " + err.Error()
		return cs
	}
	loss, g, err := net.LossGradientAligned(ctx, p, cs.Input, cs.Target, cs.Window, cs.Offset)
	if err != nil {
		cs.Error = "lossgradient: " + err.Error()
		return cs
	}
	cs.Loss, cs.Grad = loss, toJSON(g)
	gf, err := net.LossGradientFromAligned(ctx, p, cs.Input, cs.Upstream, cs.Window, cs.Offset)
	if err != nil {
		cs.Error = "lossgradientfrom: " + err.Error()
		return cs
	}
	cs.GradFrom = toJSON(gf)
	// Workers>1 must be bit-identical.
	cw := c
	cw.Dynamics.Workers = 3
	if netw, err := learning.NewNetwork(cw); err == nil {
		lw, gw, err := netw.LossGradientAligned(ctx, p, cs.Input, cs.Target, cs.Window, cs.Offset)
		if err != nil || lw != loss || !reflect.DeepEqual(gw, g) {
			cs.Internal = append(cs.Internal, fmt.Sprintf("workers3 LossGradient differs (err=%v)", err))
		}
	}
	// LossGradientFromState from NewState(zero) must give LossGradientFrom's
	// parameter and boundary gradients (only Core.Initial may differ: the saved
	// initial output is a constant boundary there).
	{
		dm, _ := dynamics.NewContinuous(c.Dynamics)
		zs, err := dm.NewState(make([]float64, n))
		// LossGradientFromState has no aligned form; it is compared only on the
		// start-aligned layout.
		if err == nil && cs.Offset == 0 {
			ns := learning.NeuralState{Core: learning.NeuralCoreContinuous, Continuous: &zs}
			gs, err := net.LossGradientFromState(ctx, p, ns, cs.Input, cs.Upstream, cs.Window)
			if err != nil || !reflect.DeepEqual(gs.Core.Weights, gf.Core.Weights) || !reflect.DeepEqual(gs.Core.Bias, gf.Core.Bias) || !reflect.DeepEqual(gs.Core.LogTau, gf.Core.LogTau) || !reflect.DeepEqual(gs.Encoder, gf.Encoder) || !reflect.DeepEqual(gs.Readout, gf.Readout) || !reflect.DeepEqual(gs.Inputs, gf.Inputs) {
				cs.Internal = append(cs.Internal, fmt.Sprintf("LossGradientFromState(zero state) differs from LossGradientFrom (err=%v)", err))
			}
		}
	}
	// Trainer: run several steps.
	tr, err := learning.NewTrainer(c, p, o)
	if err != nil {
		cs.Error = "trainer: " + err.Error()
		return cs
	}
	variants := map[string]*learning.Trainer{}
	for _, seg := range []int{1, 2, 3} {
		ov := o
		ov.Recompute = &learning.Recompute{SegmentSteps: seg}
		if t2, err := learning.NewTrainer(c, p, ov); err == nil {
			variants[fmt.Sprintf("recompute%d", seg)] = t2
		} else {
			cs.Internal = append(cs.Internal, "recompute trainer: "+err.Error())
		}
	}
	if t2, err := learning.NewTrainer(cw, p, o); err == nil {
		variants["workers3"] = t2
	}
	stepFromTrainer, _ := learning.NewTrainer(c, p, o)
	for k := 0; k < 6; k++ {
		rec := stepRecord{Input: cs.Input, Target: cs.Target}
		if k > 0 {
			rec.Input = matrix(r, steps, c.InputSize, 0.8)
			rec.Target = normals(r, c.OutputSize, 0, 0.5)
		}
		rec.Before = tr.Snapshot()
		offset := 0
		if o.TruncationAlign == learning.TruncationAlignEnd {
			offset = dynamics.EndAlignedOffset(len(rec.Input), o.Truncation)
		}
		l, gg, err := net.LossGradientAligned(ctx, rec.Before.Parameters, rec.Input, rec.Target, o.Truncation, offset)
		if err != nil {
			rec.Error = "lossgradient: " + err.Error()
			cs.Steps = append(cs.Steps, rec)
			break
		}
		rec.Loss, rec.Grad = l, toJSON(gg)
		res, err := tr.Step(ctx, rec.Input, rec.Target)
		if err != nil {
			rec.Error = "step: " + err.Error()
			cs.Steps = append(cs.Steps, rec)
			break
		}
		rec.Result, rec.After = res, tr.Snapshot()
		for name, v := range variants {
			rv, err := v.Step(ctx, rec.Input, rec.Target)
			if err != nil || !reflect.DeepEqual(rv, res) || !reflect.DeepEqual(v.Snapshot().Parameters, rec.After.Parameters) || !reflect.DeepEqual(v.Snapshot().Optimizer, rec.After.Optimizer) {
				rec.Internal = append(rec.Internal, fmt.Sprintf("%s differs from Step (err=%v)", name, err))
			}
		}
		// StepFrom with the MSE VJP upstream (float32 dL/dpred of the last row).
		up, err := mseUpstream(ctx, net, stepFromTrainer.Snapshot().Parameters, rec.Input, rec.Target, c)
		if err == nil {
			rf, err := stepFromTrainer.StepFrom(ctx, rec.Input, up)
			sf := stepFromTrainer.Snapshot()
			if err != nil || !reflect.DeepEqual(sf.Parameters, rec.After.Parameters) || !reflect.DeepEqual(sf.Optimizer, rec.After.Optimizer) || rf.GradientNorm != res.GradientNorm {
				rec.Internal = append(rec.Internal, fmt.Sprintf("StepFrom(MSE upstream) differs from Step (err=%v)", err))
			}
		} else {
			rec.Internal = append(rec.Internal, "mse upstream: "+err.Error())
		}
		cs.Steps = append(cs.Steps, rec)
	}
	return cs
}

func fill(n int) []int32 {
	out := make([]int32, n)
	for i := range out {
		out[i] = -1
	}
	return out
}

// mseUpstream is dMSE/dpred of the final readout row, computed on an Insyra
// float32 tape exactly as a caller of StepFrom would.
func mseUpstream(ctx context.Context, net *learning.Network, p learning.Parameters, input [][]float64, target []float64, c learning.Config) ([][]float64, error) {
	var last []float64
	if c.ReadoutEveryStep {
		all, err := net.PredictAll(ctx, p, input)
		if err != nil {
			return nil, err
		}
		last = all[len(all)-1]
	} else {
		var err error
		if last, err = net.Predict(ctx, p, input); err != nil {
			return nil, err
		}
	}
	f32 := func(v []float64) []float32 {
		o := make([]float32, len(v))
		for i := range v {
			o[i] = float32(v[i])
		}
		return o
	}
	pt, err := nn.NewTensor([]int{len(last)}, f32(last))
	if err != nil {
		return nil, err
	}
	tt, err := nn.NewTensor([]int{len(target)}, f32(target))
	if err != nil {
		return nil, err
	}
	tape := nn.NewTape()
	param, err := tape.Param(pt)
	if err != nil {
		return nil, err
	}
	loss, err := tape.MSELoss(param.Value(), tt)
	if err != nil {
		return nil, err
	}
	if err := tape.Backward(loss); err != nil {
		return nil, err
	}
	up := make([][]float64, len(input))
	for i := range up {
		up[i] = make([]float64, c.OutputSize)
	}
	grad, err := param.Grad().Float32Data()
	if err != nil {
		return nil, err
	}
	for i, v := range grad {
		up[len(up)-1][i] = float64(v)
	}
	return up, nil
}

// seedOffset picks a truncation offset from the seed alone, so adding it does
// not shift the random stream every other field draws from.
func seedOffset(seed uint64, window int) int {
	if window <= 0 {
		return 0
	}
	return int(seed % uint64(window))
}

func randomCoreCase(seed uint64) coreCase {
	r := rand.New(rand.NewPCG(seed, 7))
	c := randomTopology(r, 0)
	n, e := c.Nodes, len(c.Sources)
	cs := coreCase{Kind: "core", Seed: seed, Config: c, Window: r.IntN(4), SegWindow: r.IntN(4)}
	cs.Offset = seedOffset(seed, cs.Window)
	cs.Params = dynamics.Parameters{Weights: normals(r, e, 0, 0.6), Bias: normals(r, n, 0, 0.3), LogTau: normals(r, n, 0.3, 0.4)}
	cs.Initial = normals(r, n, 0, 0.5)
	steps := 1 + r.IntN(10)
	cs.Inputs = matrix(r, steps, n, 0.7)
	cs.Upstream = matrix(r, steps, n, 0.5)
	ctx := context.Background()
	m, err := dynamics.NewContinuous(c)
	if err != nil {
		cs.Error = err.Error()
		return cs
	}
	tr, err := m.Forward(ctx, cs.Params, cs.Initial, cs.Inputs)
	if err != nil {
		cs.Error = err.Error()
		return cs
	}
	cs.Outputs = tr.Outputs()
	if cs.Grad, err = m.BackwardAligned(ctx, tr, cs.Upstream, cs.Window, cs.Offset); err != nil {
		cs.Error = err.Error()
		return cs
	}
	bad := func(s string, a ...any) { cs.Internal = append(cs.Internal, fmt.Sprintf(s, a...)) }
	// Workers > 1.
	for _, w := range []int{2, 3, 5} {
		cw := c
		cw.Workers = w
		mw, err := dynamics.NewContinuous(cw)
		if err != nil {
			bad("workers %d: %v", w, err)
			continue
		}
		tw, err := mw.Forward(ctx, cs.Params, cs.Initial, cs.Inputs)
		if err != nil || !reflect.DeepEqual(tw.Outputs(), cs.Outputs) {
			bad("workers %d outputs differ", w)
			continue
		}
		gw, err := mw.BackwardAligned(ctx, tw, cs.Upstream, cs.Window, cs.Offset)
		if err != nil || !reflect.DeepEqual(gw, cs.Grad) {
			bad("workers %d gradient differs", w)
		}
	}
	// Recompute.
	for _, s := range []int{1, 2, 3, 4, 100} {
		g, err := m.BackwardRecomputeAligned(ctx, cs.Params, cs.Initial, cs.Inputs, cs.Upstream, cs.Window, cs.Offset, s)
		if err != nil || !reflect.DeepEqual(g, cs.Grad) {
			bad("BackwardRecompute segment %d differs (err=%v)", s, err)
		}
		y, err := m.ForwardRecompute(ctx, cs.Params, cs.Initial, cs.Inputs, s)
		if err != nil || !reflect.DeepEqual(y, cs.Outputs) {
			bad("ForwardRecompute segment %d differs (err=%v)", s, err)
		}
	}
	// Advance from NewState equals Forward; split points too.
	st, err := m.NewState(cs.Initial)
	if err != nil {
		bad("NewState: %v", err)
		return cs
	}
	for split := 1; split < steps; split++ {
		s1, y1, err := m.Advance(ctx, cs.Params, st, cs.Inputs[:split])
		if err != nil {
			bad("advance: %v", err)
			break
		}
		s2, y2, err := m.Advance(ctx, cs.Params, s1, cs.Inputs[split:])
		if err != nil {
			bad("advance2: %v", err)
			break
		}
		if !reflect.DeepEqual(append(y1, y2...), cs.Outputs) || !reflect.DeepEqual(s2.Voltage, tr.FinalVoltage()) {
			bad("Advance split %d differs from Forward", split)
		}
		// ForwardFromState segment outputs equal the tail of Forward.
		tfs, err := m.ForwardFromState(ctx, cs.Params, s1, cs.Inputs[split:])
		if err != nil || !reflect.DeepEqual(tfs.Outputs(), cs.Outputs[split:]) {
			bad("ForwardFromState split %d outputs differ (err=%v)", split, err)
		}
	}
	{
		cw := c
		cw.Workers = 3
		mw, _ := dynamics.NewContinuous(cw)
		sw, _ := mw.NewState(cs.Initial)
		_, yw, err := mw.Advance(ctx, cs.Params, sw, cs.Inputs)
		if err != nil || !reflect.DeepEqual(yw, cs.Outputs) {
			bad("Advance with workers 3 differs (err=%v)", err)
		}
	}
	_, yall, err := m.Advance(ctx, cs.Params, st, cs.Inputs)
	if err != nil || !reflect.DeepEqual(yall, cs.Outputs) {
		bad("Advance(NewState) differs from Forward (err=%v)", err)
	}
	// Stateful differentiable segment after a random prefix.
	cs.Prefix = matrix(r, 1+r.IntN(5), n, 0.7)
	sp, _, err := m.Advance(ctx, cs.Params, st, cs.Prefix)
	if err != nil {
		bad("prefix advance: %v", err)
		return cs
	}
	cs.State = &sp
	ts, err := m.ForwardFromState(ctx, cs.Params, sp, cs.Inputs)
	if err != nil {
		bad("ForwardFromState: %v", err)
		return cs
	}
	cs.SegOut = ts.Outputs()
	if cs.SegGrad, err = m.Backward(ctx, ts, cs.Upstream, cs.SegWindow); err != nil {
		bad("segment backward: %v", err)
	}
	return cs
}

func main() {
	cases := flag.Int("cases", 100, "number of cases of each kind")
	out := flag.String("out", "runs/.tmp/ref-continuous/cases.json", "output path")
	base := flag.Uint64("seed", 1, "base seed")
	flag.Parse()
	var all []any
	internal := 0
	for i := 0; i < *cases; i++ {
		cc := randomCoreCase(*base + uint64(i))
		internal += len(cc.Internal)
		for _, s := range cc.Internal {
			fmt.Fprintf(os.Stderr, "core seed %d: %s\n", cc.Seed, s)
		}
		all = append(all, cc)
		nc := randomNetCase(*base + uint64(i))
		internal += len(nc.Internal)
		for _, s := range nc.Internal {
			fmt.Fprintf(os.Stderr, "net seed %d: %s\n", nc.Seed, s)
		}
		for k, st := range nc.Steps {
			internal += len(st.Internal)
			for _, s := range st.Internal {
				fmt.Fprintf(os.Stderr, "net seed %d step %d: %s\n", nc.Seed, k, s)
			}
		}
		if nc.Error != "" {
			fmt.Fprintf(os.Stderr, "net seed %d error: %s\n", nc.Seed, nc.Error)
		}
		all = append(all, nc)
	}
	data, err := json.Marshal(all)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		panic(err)
	}
	fmt.Fprintf(os.Stderr, "wrote %d cases, %d internal mismatches\n", len(all), internal)
	_ = math.Pi
}
