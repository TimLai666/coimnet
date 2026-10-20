package learning

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/TimLai666/coimnet/backend/webgpu"
	"github.com/TimLai666/coimnet/dynamics"
)

// cpuBatchCore is a host stand-in for the batched device core: it runs one
// float64 dynamics.Continuous Forward per sequence and sums the per-sequence
// Backward gradients in pair order. Driving the batched trainer path with it
// checks the host plumbing (encoder, readout, upstream, gradient averaging)
// without a GPU.
type cpuBatchCore struct {
	model  *dynamics.Continuous
	layout dynamics.BatchLayout
	traces []*dynamics.Trace
	steps  int
	closed bool
}

func newCPUBatchCore(tr *Trainer) *cpuBatchCore {
	c := tr.network.config
	return &cpuBatchCore{model: tr.network.core.(continuousCore).model, layout: dynamics.BatchLayout{InputNodes: c.InputNodes, ReadoutNodes: c.ReadoutNodes, ReadoutEveryStep: c.ReadoutEveryStep}}
}

func (f *cpuBatchCore) inputNodes() []int {
	if f.layout.InputNodes != nil {
		return f.layout.InputNodes
	}
	nodes := make([]int, f.model.Config().Nodes)
	for i := range nodes {
		nodes[i] = i
	}
	return nodes
}

func (f *cpuBatchCore) readRows(steps int) []int {
	if f.layout.ReadoutEveryStep {
		rows := make([]int, steps)
		for i := range rows {
			rows[i] = i
		}
		return rows
	}
	return []int{steps - 1}
}

func (f *cpuBatchCore) Forward(ctx context.Context, p dynamics.Parameters, inputs [][][]float64) ([][][]float64, error) {
	nodes := f.model.Config().Nodes
	f.traces, f.steps = nil, len(inputs[0])
	out := make([][][]float64, len(inputs))
	for b, seq := range inputs {
		full := make([][]float64, len(seq))
		for t, row := range seq {
			full[t] = make([]float64, nodes)
			for w, node := range f.inputNodes() {
				full[t][node] = row[w]
			}
		}
		tr, err := f.model.Forward(ctx, p, make([]float64, nodes), full)
		if err != nil {
			return nil, err
		}
		f.traces = append(f.traces, tr)
		outputs := tr.Outputs()
		for _, t := range f.readRows(len(seq)) {
			row := make([]float64, 0, len(f.layout.ReadoutNodes))
			for _, node := range f.layout.ReadoutNodes {
				row = append(row, outputs[t][node])
			}
			out[b] = append(out[b], row)
		}
	}
	return out, nil
}

func (f *cpuBatchCore) Backward(ctx context.Context, upstream [][][]float64) (dynamics.BatchGradient, error) {
	nodes := f.model.Config().Nodes
	var sum dynamics.BatchGradient
	for b, tr := range f.traces {
		up := make([][]float64, f.steps)
		for t := range up {
			up[t] = make([]float64, nodes)
		}
		for s, t := range f.readRows(f.steps) {
			for r, node := range f.layout.ReadoutNodes {
				up[t][node] = upstream[b][s][r]
			}
		}
		g, err := f.model.Backward(ctx, tr, up, 0)
		if err != nil {
			return sum, err
		}
		if b == 0 {
			sum.Weights, sum.Bias, sum.LogTau = make([]float64, len(g.Weights)), make([]float64, nodes), make([]float64, nodes)
			if g.GateGain != nil {
				sum.GateGain, sum.GateBias = make([]float64, nodes), make([]float64, nodes)
			}
		}
		for _, pair := range [][2][]float64{{sum.Weights, g.Weights}, {sum.Bias, g.Bias}, {sum.LogTau, g.LogTau}, {sum.GateGain, g.GateGain}, {sum.GateBias, g.GateBias}} {
			for i, v := range pair[1] {
				pair[0][i] += v
			}
		}
		inputs := make([][]float64, f.steps)
		for t := range inputs {
			for _, node := range f.inputNodes() {
				inputs[t] = append(inputs[t], g.Inputs[t][node])
			}
		}
		sum.Inputs = append(sum.Inputs, inputs)
	}
	return sum, nil
}

func (f *cpuBatchCore) Info() webgpu.DeviceInfo { return webgpu.DeviceInfo{Adapter: "cpu stand-in"} }
func (f *cpuBatchCore) Close() error            { f.closed = true; return nil }

type batchFixture struct {
	gate, loss              string
	everyStep, input, signs bool
}

// newBatchFixture is a seeded eight-node core with three readout nodes and
// three classes; input selects InputNodes (two of them) instead of every node.
func newBatchFixture(f batchFixture, accumulate int) (Config, Parameters, Options) {
	rng := rand.New(rand.NewPCG(5, 9))
	const nodes = 8
	var sources, targets []int
	for t := 0; t < nodes; t++ {
		for _, s := range rng.Perm(nodes)[:3] {
			sources, targets = append(sources, s), append(targets, t)
		}
	}
	fill := func(n int, scale float64) []float64 {
		v := make([]float64, n)
		for i := range v {
			v[i] = (2*rng.Float64() - 1) * scale
		}
		return v
	}
	c := Config{Dynamics: dynamics.Config{Nodes: nodes, Sources: sources, Targets: targets, DT: 1, Activation: "tanh", Gate: f.gate}, InputSize: 2, OutputSize: 3, ReadoutNodes: []int{7, 5, 6}, ReadoutEveryStep: f.everyStep}
	width := nodes
	if f.input {
		c.InputNodes, width = []int{0, 3}, 2
	}
	logTau := make([]float64, nodes)
	for i := range logTau {
		logTau[i] = math.Log(2) + .1*rng.NormFloat64()
	}
	p := Parameters{Core: dynamics.Parameters{Weights: fill(len(sources), .8), Bias: fill(nodes, .2), LogTau: logTau}, Encoder: fill(2*width, 1), Readout: fill(3*3, 1)}
	if f.signs {
		// Edges cycle through excitatory, inhibitory and free; a fixed-sign
		// edge stores the log magnitude of its weight.
		c.EdgeSigns = make([]int8, len(sources))
		for i := range c.EdgeSigns {
			c.EdgeSigns[i] = int8(1 - i%3)
			if c.EdgeSigns[i] != 0 {
				p.Core.Weights[i] = math.Log(math.Abs(p.Core.Weights[i]) + .05)
			}
		}
	}
	o := DefaultOptions()
	o.Loss, o.AccumulateSteps, o.LearningRate = f.loss, accumulate, .02
	if f.gate != "" {
		p.Core.GateGain, p.Core.GateBias = fill(nodes, .5), fill(nodes, .5)
		o.Trainable.Gate = true
	}
	return c, p, o
}

func batchEpisodes(count, steps int, seed uint64) ([][][]float64, [][]float64) {
	rng := rand.New(rand.NewPCG(seed, 5))
	inputs := make([][][]float64, count)
	targets := make([][]float64, count)
	for i := range inputs {
		inputs[i] = make([][]float64, steps)
		for t := range inputs[i] {
			inputs[i][t] = []float64{2*rng.Float64() - 1, 2*rng.Float64() - 1}
		}
		targets[i] = make([]float64, 3)
		targets[i][(i+int(seed))%3] = 1
	}
	return inputs, targets
}

// batchTargets returns targets as they are, or for the per-step loss one row
// per step: the first row unscored, then classes that cycle from the pair's.
func batchTargets(f batchFixture, targets [][]float64, steps int) [][]float64 {
	if f.loss != LossSoftmaxCrossEntropyEveryStep {
		return targets
	}
	out := make([][]float64, len(targets))
	for i, target := range targets {
		label := 0
		for k, v := range target {
			if v == 1 {
				label = k
			}
		}
		out[i] = make([]float64, 3*steps)
		for r := 1; r < steps; r++ {
			out[i][3*r+(label+r)%3] = 1
		}
	}
	return out
}

func newBatchCoreTrainers(t *testing.T, f batchFixture, accumulate int) (*Trainer, *Trainer) {
	t.Helper()
	c, p, o := newBatchFixture(f, accumulate)
	cpu, err := NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateGPUBatchTrainerSupport(c, o); err != nil {
		t.Fatal(err)
	}
	batched, err := NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	batched.batch = newCPUBatchCore(batched)
	return cpu, batched
}

// TestBatchCoreStepMatchesStepBatch drives the batched trainer path with the
// float64 stand-in core. When every chunk starts a fresh accumulation window
// the sums are formed in the same order as StepBatch's accumulator, so the
// parameters, optimizer state and per-pair losses must match bit for bit.
func TestBatchCoreStepMatchesStepBatch(t *testing.T) {
	ctx := context.Background()
	for name, f := range map[string]batchFixture{
		"mse last step":         {loss: LossMSE, input: true},
		"cross entropy gated":   {loss: LossSoftmaxCrossEntropy, gate: dynamics.GateCurrent, input: true},
		"softsign gate all in":  {loss: LossSoftmaxCrossEntropy, gate: dynamics.GateCurrentSoftsign},
		"every step readout":    {loss: LossMSE, everyStep: true, input: true},
		"every step cross gate": {loss: LossSoftmaxCrossEntropy, everyStep: true, gate: dynamics.GateCurrent},
		"per-step cross gate":   {loss: LossSoftmaxCrossEntropyEveryStep, everyStep: true, gate: dynamics.GateCurrentSoftsign, input: true},
	} {
		t.Run(name, func(t *testing.T) {
			cpu, batched := newBatchCoreTrainers(t, f, 4)
			for round := range 3 {
				inputs, targets := batchEpisodes(4, 5, uint64(round))
				targets = batchTargets(f, targets, 5)
				want, err := cpu.StepBatch(ctx, inputs, targets, 2)
				if err != nil {
					t.Fatal(err)
				}
				got, err := batched.StepBatch(ctx, inputs, targets, 3)
				if err != nil {
					t.Fatal(err)
				}
				for i := range want {
					w, g := want[i], got[i]
					if w.Loss != g.Loss || w.Applied != g.Applied || w.Accumulated != g.Accumulated || w.Updates != g.Updates || w.LearningRate != g.LearningRate || w.GradientHorizonSteps != g.GradientHorizonSteps {
						t.Fatalf("round %d pair %d: CPU %+v, batched %+v", round, i, w, g)
					}
				}
				if !got[3].Applied || got[3].UpdateNorm != want[3].UpdateNorm {
					t.Fatalf("round %d: last pair %+v, want %+v", round, got[3], want[3])
				}
			}
			ws, gs := cpu.Snapshot(), batched.Snapshot()
			if !reflect.DeepEqual(ws.Parameters, gs.Parameters) || !reflect.DeepEqual(ws.Optimizer, gs.Optimizer) || ws.Updates != gs.Updates {
				t.Fatal("batched path diverged from StepBatch")
			}
		})
	}
}

// TestBatchCoreAccumulatesAcrossCalls splits accumulation windows across
// StepBatch calls and chunks (window 3, batches of 4), so chunks join open
// windows and the sum order differs from the accumulator's; the trainers must
// agree to float64 rounding and hold the same open window.
func TestBatchCoreAccumulatesAcrossCalls(t *testing.T) {
	ctx := context.Background()
	cpu, batched := newBatchCoreTrainers(t, batchFixture{loss: LossSoftmaxCrossEntropy, gate: dynamics.GateCurrent, input: true}, 3)
	for round := range 2 {
		inputs, targets := batchEpisodes(4, 6, uint64(10+round))
		want, err := cpu.StepBatch(ctx, inputs, targets, 1)
		if err != nil {
			t.Fatal(err)
		}
		got, err := batched.StepBatch(ctx, inputs, targets, 4)
		if err != nil {
			t.Fatal(err)
		}
		for i := range want {
			if want[i].Applied != got[i].Applied || want[i].Accumulated != got[i].Accumulated || want[i].Updates != got[i].Updates || want[i].Loss != got[i].Loss {
				t.Fatalf("round %d pair %d: CPU %+v, batched %+v", round, i, want[i], got[i])
			}
		}
	}
	ws, gs := cpu.Snapshot(), batched.Snapshot()
	if ws.Updates != 2 || gs.Updates != 2 || ws.Accumulator == nil || gs.Accumulator == nil || ws.Accumulator.Count != gs.Accumulator.Count {
		t.Fatalf("updates %d/%d, accumulators %+v / %+v", ws.Updates, gs.Updates, ws.Accumulator, gs.Accumulator)
	}
	near := func(name string, a, b []float64) {
		t.Helper()
		for i := range a {
			if math.Abs(a[i]-b[i]) > 1e-12*(1+math.Abs(a[i])) {
				t.Fatalf("%s[%d] = %.17g, CPU %.17g", name, i, b[i], a[i])
			}
		}
	}
	near("parameters", flatParameters(ws.Parameters), flatParameters(gs.Parameters))
	near("first moment", ws.Optimizer.First, gs.Optimizer.First)
	near("second moment", ws.Optimizer.Second, gs.Optimizer.Second)
	near("accumulator", ws.Accumulator.Sum, gs.Accumulator.Sum)
}

// TestBatchCoreFixedSignsMatchesStepBatch trains a core whose edges are
// excitatory, inhibitory or free. The batched path converts the summed weight
// gradient to log magnitudes once per chunk while StepBatch converts each
// pair's, so the trainers agree to float64 rounding, and the fixed-sign
// edges keep their signs.
func TestBatchCoreFixedSignsMatchesStepBatch(t *testing.T) {
	ctx := context.Background()
	f := batchFixture{loss: LossSoftmaxCrossEntropy, gate: dynamics.GateCurrentSoftsign, input: true, signs: true}
	cpu, batched := newBatchCoreTrainers(t, f, 4)
	if !batched.network.fixed {
		t.Fatal("fixture declares no fixed sign")
	}
	before := batched.Snapshot().Parameters.Core.Weights
	for round := range 3 {
		inputs, targets := batchEpisodes(4, 5, uint64(20+round))
		want, err := cpu.StepBatch(ctx, inputs, targets, 2)
		if err != nil {
			t.Fatal(err)
		}
		got, err := batched.StepBatch(ctx, inputs, targets, 3)
		if err != nil {
			t.Fatal(err)
		}
		for i := range want {
			if want[i].Applied != got[i].Applied || want[i].Updates != got[i].Updates || want[i].Loss != got[i].Loss {
				t.Fatalf("round %d pair %d: CPU %+v, batched %+v", round, i, want[i], got[i])
			}
		}
	}
	ws, gs := cpu.Snapshot(), batched.Snapshot()
	a, b := flatParameters(ws.Parameters), flatParameters(gs.Parameters)
	for i := range a {
		if math.Abs(a[i]-b[i]) > 1e-12*(1+math.Abs(a[i])) {
			t.Fatalf("parameter %d = %.17g, CPU %.17g", i, b[i], a[i])
		}
	}
	moved := false
	for i, w := range gs.Parameters.Core.Weights {
		if w != before[i] {
			moved = true
		}
	}
	if !moved {
		t.Fatal("training left every weight unchanged")
	}
	effective, err := EffectiveWeights(gs.Config, gs.Parameters)
	if err != nil {
		t.Fatal(err)
	}
	for i, sign := range gs.Config.EdgeSigns {
		if sign != 0 && (effective[i] > 0) != (sign > 0) {
			t.Fatalf("edge %d sign %d has effective weight %g", i, sign, effective[i])
		}
	}
}

// TestBatchCoreWindowOfOneUpdatesEveryPair keeps AccumulateSteps at one, so
// every chunk is a single pair and the batched path reproduces StepBatch bit
// for bit, gradient norms included.
func TestBatchCoreWindowOfOneUpdatesEveryPair(t *testing.T) {
	ctx := context.Background()
	cpu, batched := newBatchCoreTrainers(t, batchFixture{loss: LossMSE, input: true}, 0)
	inputs, targets := batchEpisodes(3, 4, 2)
	want, err := cpu.StepBatch(ctx, inputs, targets, 1)
	if err != nil {
		t.Fatal(err)
	}
	got, err := batched.StepBatch(ctx, inputs, targets, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("results differ:\nCPU     %+v\nbatched %+v", want, got)
	}
	if !reflect.DeepEqual(cpu.Snapshot(), batched.Snapshot()) {
		t.Fatal("snapshots differ")
	}
}

// TestBatchCoreGradientNormIsTheChunkMean checks the documented norm: every
// pair of a chunk reports the norm of the chunk's masked mean gradient.
func TestBatchCoreGradientNormIsTheChunkMean(t *testing.T) {
	ctx := context.Background()
	c, p, o := newBatchFixture(batchFixture{loss: LossMSE, input: true}, 4)
	n, err := NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	_, batched := newBatchCoreTrainers(t, batchFixture{loss: LossMSE, input: true}, 4)
	inputs, targets := batchEpisodes(4, 5, 7)
	mean := make([]float64, len(flatParameters(p)))
	for i := range inputs {
		_, g, err := n.lossGradient(ctx, p, inputs[i], targets[i], 0, 0, 0, o.Loss)
		if err != nil {
			t.Fatal(err)
		}
		for j, v := range flatGradient(g) {
			mean[j] += v / 4
		}
	}
	mask := parameterMask(p, o, nil, n.core)
	var want float64
	for j, v := range mean {
		if mask[j] {
			want = math.Hypot(want, v)
		}
	}
	got, err := batched.StepBatch(ctx, inputs, targets, 2)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range got {
		if math.Abs(r.GradientNorm-want) > 1e-12*want {
			t.Fatalf("pair %d gradient norm %.17g, chunk mean %.17g", i, r.GradientNorm, want)
		}
	}
}

func TestBatchCoreRefusesUnequalLengthsWithoutCommitting(t *testing.T) {
	ctx := context.Background()
	_, batched := newBatchCoreTrainers(t, batchFixture{loss: LossMSE, input: true}, 4)
	before := batched.Snapshot()
	inputs, targets := batchEpisodes(4, 5, 1)
	inputs[2] = inputs[2][:4]
	if _, err := batched.StepBatch(ctx, inputs, targets, 2); err == nil || !strings.Contains(err.Error(), "equal lengths") {
		t.Fatalf("unequal lengths: %v", err)
	}
	if !reflect.DeepEqual(before, batched.Snapshot()) {
		t.Fatal("a failed chunk changed the trainer")
	}
	if info, ok := batched.GPUAdapterInfo(); !ok || info.Forward.Adapter != "cpu stand-in" {
		t.Fatalf("GPUAdapterInfo = %+v, %v", info, ok)
	}
	core := batched.batch.(*cpuBatchCore)
	if err := batched.Close(); err != nil || !core.closed {
		t.Fatalf("Close: %v, core closed %v", err, core.closed)
	}
}

func TestGPUBatchTrainerRefusesUnsupportedBeforeDeviceOpen(t *testing.T) {
	base := func() (Config, Parameters, Options) {
		return newBatchFixture(batchFixture{loss: LossMSE, input: true}, 4)
	}
	cases := map[string]func(*Config, *Parameters, *Options){
		"truncation": func(_ *Config, _ *Parameters, o *Options) { o.Truncation = 2 },
		"recompute":  func(_ *Config, _ *Parameters, o *Options) { o.Recompute = &Recompute{SegmentSteps: 2} },
		"loss scale": func(_ *Config, _ *Parameters, o *Options) { o.LossScale = 4 },
		"delay": func(c *Config, _ *Parameters, _ *Options) {
			c.Dynamics.Delays = make([]int, len(c.Dynamics.Sources))
			c.Dynamics.Delays[0] = 1
		},
		"vector state": func(c *Config, _ *Parameters, _ *Options) { c.Dynamics.StateDimension = 2 },
		"sharing": func(c *Config, _ *Parameters, _ *Options) {
			c.Sharing = &ParameterSharing{}
		},
		"lif": func(c *Config, _ *Parameters, _ *Options) { c.LIF = &dynamics.LIFConfig{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c, p, o := base()
			mutate(&c, &p, &o)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err := NewGPUBatchTrainer(ctx, c, p, o)
			if !errors.Is(err, ErrGPUTrainerUnsupported) {
				t.Fatalf("error %v, want ErrGPUTrainerUnsupported", err)
			}
		})
	}
}

// TestGPUBatchTrainerMatchesCPUStepBatch runs the batched device trainer
// against the CPU trainer for a few minibatch updates and compares the
// parameters at float32 tolerance. It skips with webgpu.ErrUnavailable when
// no hardware adapter exists; elsewhere it shows that construction fails
// with that error instead of falling back to the CPU.
func TestGPUBatchTrainerMatchesCPUStepBatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for name, f := range map[string]batchFixture{
		"cross entropy gated": {loss: LossSoftmaxCrossEntropy, gate: dynamics.GateCurrentSoftsign, input: true},
		"mse every step":      {loss: LossMSE, everyStep: true},
		"per-step cross":      {loss: LossSoftmaxCrossEntropyEveryStep, everyStep: true, gate: dynamics.GateCurrentSoftsign, input: true},
	} {
		t.Run(name, func(t *testing.T) {
			c, p, o := newBatchFixture(f, 4)
			cpu, err := NewTrainer(c, p, o)
			if err != nil {
				t.Fatal(err)
			}
			gpu, err := NewGPUBatchTrainer(ctx, c, p, o)
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
			for round := range 3 {
				inputs, targets := batchEpisodes(4, 6, uint64(round))
				targets = batchTargets(f, targets, 6)
				want, err := cpu.StepBatch(ctx, inputs, targets, 2)
				if err != nil {
					t.Fatal(err)
				}
				got, err := gpu.StepBatch(ctx, inputs, targets, 2)
				if err != nil {
					t.Fatal(err)
				}
				for i := range want {
					if math.Abs(want[i].Loss-got[i].Loss) > 1e-4*(1+math.Abs(want[i].Loss)) || want[i].Applied != got[i].Applied {
						t.Fatalf("round %d pair %d: CPU %+v, GPU %+v", round, i, want[i], got[i])
					}
				}
			}
			wp, gp := flatParameters(cpu.Snapshot().Parameters), flatParameters(gpu.Snapshot().Parameters)
			for i := range wp {
				if math.Abs(wp[i]-gp[i]) > 2e-4*(1+math.Abs(wp[i])) {
					t.Fatalf("parameter %d = %.8g, CPU %.8g", i, gp[i], wp[i])
				}
			}
			info, ok := gpu.GPUAdapterInfo()
			t.Logf("adapter %+v %v", info.Forward, ok)
		})
	}
}
