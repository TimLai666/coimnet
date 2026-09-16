package dynamics

import (
	"context"
	"math"
	"math/rand/v2"
	"testing"
)

func workerFixture(t *testing.T) (*Continuous, Parameters, []float64, [][]float64) {
	t.Helper()
	m, err := NewContinuous(Config{
		Nodes:      3,
		Sources:    []int{0, 1, 2, 1},
		Targets:    []int{1, 2, 0, 0},
		Delays:     []int{0, 1, 3, 2},
		DT:         0.3,
		Activation: "tanh",
	})
	if err != nil {
		t.Fatal(err)
	}
	p := Parameters{
		Weights: []float64{0.2, -0.15, 0.1, 0.05},
		Bias:    []float64{0.1, -0.2, 0.05},
		LogTau:  []float64{0.2, -0.1, 0.3},
	}
	initial := []float64{0.2, -0.3, 0.1}
	inputs := [][]float64{
		{0.1, 0.3, -0.2},
		{-0.2, 0.4, 0.1},
		{0.5, -0.1, 0.2},
		{0.2, 0.2, -0.4},
		{-0.3, 0.1, 0.3},
		{0.1, -0.5, 0.2},
		{0.4, 0.2, 0.1},
	}
	return m, p, initial, inputs
}

func newWithWorkers(t *testing.T, c Config, workers int) *Continuous {
	t.Helper()
	c2 := c
	c2.Workers = workers
	m, err := NewContinuous(c2)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestContinuousWorkersAreBitIdentical(t *testing.T) {
	for _, activation := range []string{"tanh", "softplus"} {
		t.Run(activation, func(t *testing.T) {
			base := Config{
				Nodes:      3,
				Sources:    []int{0, 1, 2, 1},
				Targets:    []int{1, 2, 0, 0},
				Delays:     []int{0, 1, 3, 2},
				DT:         0.3,
				Activation: activation,
			}
			p := Parameters{
				Weights: []float64{0.2, -0.15, 0.1, 0.05},
				Bias:    []float64{0.1, -0.2, 0.05},
				LogTau:  []float64{0.2, -0.1, 0.3},
			}
			initial := []float64{0.2, -0.3, 0.1}
			inputs := [][]float64{
				{0.1, 0.3, -0.2},
				{-0.2, 0.4, 0.1},
				{0.5, -0.1, 0.2},
				{0.2, 0.2, -0.4},
				{-0.3, 0.1, 0.3},
				{0.1, -0.5, 0.2},
				{0.4, 0.2, 0.1},
			}

			models := make(map[int]*Continuous)
			for _, w := range []int{0, 1, 2, 7} {
				models[w] = newWithWorkers(t, base, w)
			}

			// Forward: Outputs must be bit-identical.
			type fwdResult struct {
				outputs [][]float64
				volt    []float64
			}
			fwds := make(map[int]fwdResult)
			for w, m := range models {
				tr, err := m.Forward(context.Background(), p, initial, inputs)
				if err != nil {
					t.Fatalf("w=%d Forward: %v", w, err)
				}
				fwds[w] = fwdResult{outputs: tr.Outputs(), volt: tr.FinalVoltage()}
			}
			ref := fwds[1]
			for w, fr := range fwds {
				if len(fr.outputs) != len(ref.outputs) {
					t.Fatalf("w=%d output rows %d, want %d", w, len(fr.outputs), len(ref.outputs))
				}
				for i := range fr.outputs {
					for j := range fr.outputs[i] {
						if fr.outputs[i][j] != ref.outputs[i][j] {
							t.Fatalf("w=%d Forward output[%d][%d] = %v, want %v", w, i, j, fr.outputs[i][j], ref.outputs[i][j])
						}
					}
				}
				for i := range fr.volt {
					if fr.volt[i] != ref.volt[i] {
						t.Fatalf("w=%d FinalVoltage[%d] = %v, want %v", w, i, fr.volt[i], ref.volt[i])
					}
				}
			}

			// Advance (3 segments): state & outputs bit-identical.
			for _, w := range []int{0, 1, 2, 7} {
				m := models[w]
				s, err := m.NewState(initial)
				if err != nil {
					t.Fatal(err)
				}
				var allOutputs [][]float64
				boundaries := []int{2, 5, 7}
				prev := 0
				for _, b := range boundaries {
					var outputs [][]float64
					s, outputs, err = m.Advance(context.Background(), p, s, inputs[prev:b])
					if err != nil {
						t.Fatalf("w=%d Advance segment: %v", w, err)
					}
					allOutputs = append(allOutputs, outputs...)
					prev = b
				}
				if len(allOutputs) != len(ref.outputs) {
					t.Fatalf("w=%d Advance output rows %d, want %d", w, len(allOutputs), len(ref.outputs))
				}
				for i := range allOutputs {
					for j := range allOutputs[i] {
						if allOutputs[i][j] != ref.outputs[i][j] {
							t.Fatalf("w=%d Advance output[%d][%d] = %v, want %v", w, i, j, allOutputs[i][j], ref.outputs[i][j])
						}
					}
				}
				for i := range s.Voltage {
					if s.Voltage[i] != ref.volt[i] {
						t.Fatalf("w=%d Advance FinalVoltage[%d] = %v, want %v", w, i, s.Voltage[i], ref.volt[i])
					}
				}
			}
		})
	}
}

func TestContinuousWorkersGradientBitIdentical(t *testing.T) {
	base := Config{
		Nodes:      3,
		Sources:    []int{0, 1, 2, 1},
		Targets:    []int{1, 2, 0, 0},
		Delays:     []int{0, 1, 3, 2},
		DT:         0.3,
		Activation: "tanh",
	}
	p := Parameters{
		Weights: []float64{0.2, -0.15, 0.1, 0.05},
		Bias:    []float64{0.1, -0.2, 0.05},
		LogTau:  []float64{0.2, -0.1, 0.3},
	}
	initial := []float64{0.2, -0.3, 0.1}
	inputs := [][]float64{
		{0.1, 0.3, -0.2},
		{-0.2, 0.4, 0.1},
		{0.5, -0.1, 0.2},
	}
	up := [][]float64{
		{0.2, -0.3, 0.1},
		{0.1, 0.4, -0.5},
		{-0.5, 0.6, 0.3},
	}

	type gradResult struct {
		weights, bias, logtau, initial []float64
		inputs                         [][]float64
	}
	grads := make(map[int]gradResult)
	for _, w := range []int{1, 7} {
		m := newWithWorkers(t, base, w)
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			t.Fatal(err)
		}
		g, err := m.Backward(context.Background(), tr, up, 0)
		if err != nil {
			t.Fatal(err)
		}
		grads[w] = gradResult{
			weights: g.Weights, bias: g.Bias, logtau: g.LogTau, initial: g.Initial, inputs: g.Inputs,
		}
	}
	ref := grads[1]
	for w, gr := range grads {
		for i, v := range gr.weights {
			if v != ref.weights[i] {
				t.Fatalf("w=%d Weights[%d] = %v, want %v", w, i, v, ref.weights[i])
			}
		}
		for i, v := range gr.bias {
			if v != ref.bias[i] {
				t.Fatalf("w=%d Bias[%d] = %v, want %v", w, i, v, ref.bias[i])
			}
		}
		for i, v := range gr.logtau {
			if v != ref.logtau[i] {
				t.Fatalf("w=%d LogTau[%d] = %v, want %v", w, i, v, ref.logtau[i])
			}
		}
		for i, v := range gr.initial {
			if v != ref.initial[i] {
				t.Fatalf("w=%d Initial[%d] = %v, want %v", w, i, v, ref.initial[i])
			}
		}
		for i := range gr.inputs {
			for j, v := range gr.inputs[i] {
				if v != ref.inputs[i][j] {
					t.Fatalf("w=%d Inputs[%d][%d] = %v, want %v", w, i, j, v, ref.inputs[i][j])
				}
			}
		}
	}
}

func TestContinuousWorkersRejectBadCount(t *testing.T) {
	base := Config{Nodes: 2, DT: 1, Activation: "tanh"}
	for _, w := range []int{-1, 1025} {
		c := base
		c.Workers = w
		if _, err := NewContinuous(c); err == nil {
			t.Fatalf("workers=%d should be rejected", w)
		}
	}
}

func TestContinuousPartitionResolves(t *testing.T) {
	base := Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{1}, DT: 1, Activation: "tanh"}
	for _, w := range []int{0, 1, 2} {
		m := newWithWorkers(t, base, w)
		if w == 0 || w == 1 {
			if m.partition != nil {
				t.Fatalf("workers=%d: reference mode must not build a partition", w)
			}
			continue
		}
		if m.partition == nil {
			t.Fatalf("workers=%d: partition not built", w)
		}
		if got := m.partition.Workers(); got != w {
			t.Fatalf("workers=%d: partition.Workers()=%d, want %d", w, got, w)
		}
	}
}

func bigFixture(t testing.TB, nodes, edges int) (*Continuous, Parameters, []float64, [][]float64) {
	t.Helper()
	rnd := rand.New(rand.NewPCG(42, 42))
	sources := make([]int, edges)
	targets := make([]int, edges)
	delays := make([]int, edges)
	for e := 0; e < edges; e++ {
		sources[e] = rnd.IntN(nodes)
		targets[e] = rnd.IntN(nodes)
		delays[e] = rnd.IntN(4)
	}
	m, err := NewContinuous(Config{
		Nodes:      nodes,
		Sources:    sources,
		Targets:    targets,
		Delays:     delays,
		DT:         0.1,
		Activation: "tanh",
	})
	if err != nil {
		t.Fatal(err)
	}
	p := Parameters{
		Weights: make([]float64, edges),
		Bias:    make([]float64, nodes),
		LogTau:  make([]float64, nodes),
	}
	for i := range p.Weights {
		p.Weights[i] = rnd.NormFloat64() * 0.1
	}
	for i := range p.Bias {
		p.Bias[i] = rnd.NormFloat64() * 0.05
	}
	for i := range p.LogTau {
		p.LogTau[i] = rnd.NormFloat64() * 0.1
	}
	initial := make([]float64, nodes)
	for i := range initial {
		initial[i] = rnd.NormFloat64() * 0.1
	}
	inputs := make([][]float64, 32)
	for t := range inputs {
		row := make([]float64, nodes)
		for i := range row {
			row[i] = rnd.NormFloat64() * 0.1
		}
		inputs[t] = row
	}
	return m, p, initial, inputs
}

func BenchmarkContinuousForwardWorkers1(b *testing.B) {
	m, p, initial, inputs := bigFixture(b, 64, 512)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			b.Fatal(err)
		}
		_ = tr
	}
}

func BenchmarkContinuousForwardWorkers4(b *testing.B) {
	base := Config{
		Nodes:      64,
		Sources:    nil,
		Targets:    nil,
		Delays:     nil,
		DT:         0.1,
		Activation: "tanh",
	}
	rnd := rand.New(rand.NewPCG(42, 42))
	const edges = 512
	base.Sources = make([]int, edges)
	base.Targets = make([]int, edges)
	base.Delays = make([]int, edges)
	for e := 0; e < edges; e++ {
		base.Sources[e] = rnd.IntN(64)
		base.Targets[e] = rnd.IntN(64)
		base.Delays[e] = rnd.IntN(4)
	}
	base.Workers = 4
	m, err := NewContinuous(base)
	if err != nil {
		b.Fatal(err)
	}
	p := Parameters{
		Weights: make([]float64, edges),
		Bias:    make([]float64, 64),
		LogTau:  make([]float64, 64),
	}
	for i := range p.Weights {
		p.Weights[i] = rnd.NormFloat64() * 0.1
	}
	for i := range p.Bias {
		p.Bias[i] = rnd.NormFloat64() * 0.05
	}
	for i := range p.LogTau {
		p.LogTau[i] = rnd.NormFloat64() * 0.1
	}
	initial := make([]float64, 64)
	for i := range initial {
		initial[i] = rnd.NormFloat64() * 0.1
	}
	inputs := make([][]float64, 32)
	for t := range inputs {
		row := make([]float64, 64)
		for i := range row {
			row[i] = rnd.NormFloat64() * 0.1
		}
		inputs[t] = row
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			b.Fatal(err)
		}
		_ = tr
	}
}

func TestContinuousForwardAllocsAreConstant(t *testing.T) {
	rnd := rand.New(rand.NewPCG(42, 42))
	const nodes = 64
	const edges = 512
	sources := make([]int, edges)
	targets := make([]int, edges)
	delays := make([]int, edges)
	for e := 0; e < edges; e++ {
		sources[e] = rnd.IntN(nodes)
		targets[e] = rnd.IntN(nodes)
		delays[e] = rnd.IntN(4)
	}
	p := Parameters{
		Weights: make([]float64, edges),
		Bias:    make([]float64, nodes),
		LogTau:  make([]float64, nodes),
	}
	for i := range p.Weights {
		p.Weights[i] = rnd.NormFloat64() * 0.1
	}
	for i := range p.Bias {
		p.Bias[i] = rnd.NormFloat64() * 0.05
	}
	for i := range p.LogTau {
		p.LogTau[i] = rnd.NormFloat64() * 0.1
	}
	initial := make([]float64, nodes)
	for i := range initial {
		initial[i] = rnd.NormFloat64() * 0.1
	}

	makeInputs := func(n int) [][]float64 {
		r := rand.New(rand.NewPCG(99, 99))
		in := make([][]float64, n)
		for t := range in {
			row := make([]float64, nodes)
			for i := range row {
				row[i] = r.NormFloat64() * 0.1
			}
			in[t] = row
		}
		return in
	}

	measure := func(workers, steps int) float64 {
		c := Config{Nodes: nodes, Sources: sources, Targets: targets, Delays: delays, DT: 0.1, Activation: "tanh", Workers: workers}
		m, err := NewContinuous(c)
		if err != nil {
			t.Fatal(err)
		}
		inputs := makeInputs(steps)
		return testing.AllocsPerRun(20, func() {
			tr, err := m.Forward(context.Background(), p, initial, inputs)
			if err != nil {
				t.Fatal(err)
			}
			_ = tr
		})
	}

	ref8 := measure(1, 8)
	ref32 := measure(1, 32)
	w48 := measure(4, 8)
	w432 := measure(4, 32)

	t.Logf("workers=1 allocs/run:  8 steps=%.0f 32 steps=%.0f", ref8, ref32)
	t.Logf("workers=4 allocs/run:  8 steps=%.0f 32 steps=%.0f", w48, w432)

	// 每步 4 次是既有 eager 迴圈的基準，Workers 1 的參考模式不得增加每步配置。
	delta := ref32 - ref8
	t.Logf("workers=1 alloc delta: %.0f (max allowed %d)", math.Abs(delta), 4*24)
	if math.Abs(delta) > 4*24 {
		t.Fatalf("allocs not constant: workers=1 8-step=%.0f 32-step=%.0f delta=%.0f > %d", ref8, ref32, delta, 4*24)
	}
}
