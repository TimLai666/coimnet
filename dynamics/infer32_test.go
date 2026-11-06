package dynamics

import (
	"context"
	"math"
	"math/rand/v2"
	"testing"
)

type infer32Case struct {
	m       *Continuous
	p       Parameters
	inputs  [][]float64
	inputs3 [][]float32
}

func newInfer32Case(tb testing.TB, nodes, degree, steps int, shuffled bool) infer32Case {
	rnd := rand.New(rand.NewPCG(1, 2))
	c := Config{Nodes: nodes, DT: 1, Activation: "softsign", Gate: GateCurrentSoftsign}
	for target := range nodes {
		for range degree {
			c.Sources = append(c.Sources, rnd.IntN(nodes))
			c.Targets = append(c.Targets, target)
		}
	}
	if shuffled {
		rnd.Shuffle(len(c.Sources), func(i, j int) {
			c.Sources[i], c.Sources[j] = c.Sources[j], c.Sources[i]
			c.Targets[i], c.Targets[j] = c.Targets[j], c.Targets[i]
		})
	}
	m, err := NewContinuous(c)
	if err != nil {
		tb.Fatal(err)
	}
	p := Parameters{Weights: make([]float64, len(c.Sources)), Bias: make([]float64, nodes), LogTau: make([]float64, nodes), GateGain: make([]float64, nodes), GateBias: make([]float64, nodes)}
	for e := range p.Weights {
		p.Weights[e] = rnd.NormFloat64() * .2
	}
	for i := range nodes {
		p.Bias[i] = rnd.NormFloat64() * .1
		p.LogTau[i] = rnd.Float64() * 2
		p.GateGain[i] = rnd.NormFloat64()
		p.GateBias[i] = rnd.NormFloat64()
	}
	k := infer32Case{m: m, p: p, inputs: make([][]float64, steps), inputs3: make([][]float32, steps)}
	for t := range steps {
		k.inputs[t] = make([]float64, nodes)
		k.inputs3[t] = make([]float32, nodes)
		for i := range nodes {
			x := rnd.NormFloat64()
			k.inputs[t][i] = x
			k.inputs3[t][i] = float32(x)
		}
	}
	return k
}

func TestInfer32MatchesInfer(t *testing.T) {
	for _, shuffled := range []bool{false, true} {
		k := newInfer32Case(t, 300, 23, 28, shuffled)
		ctx := context.Background()
		want, err := k.m.InferLast(ctx, k.p, make([]float64, 300), k.inputs)
		if err != nil {
			t.Fatal(err)
		}
		f, err := k.m.CompileInfer32(k.p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := f.InferLast(ctx, make([]float32, 300), k.inputs3)
		if err != nil {
			t.Fatal(err)
		}
		worst := 0.0
		for i := range want {
			worst = math.Max(worst, math.Abs(want[i]-float64(got[i])))
		}
		if worst > 1e-4 {
			t.Fatalf("shuffled=%v: max |float32 - float64| = %g, want <= 1e-4", shuffled, worst)
		}
	}
}

func TestCompileInfer32Refuses(t *testing.T) {
	c := Config{Nodes: 2, DT: 1, Activation: "tanh", Sources: []int{0}, Targets: []int{1}}
	m, err := NewContinuous(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.CompileInfer32(Parameters{Weights: []float64{1}, Bias: make([]float64, 2), LogTau: make([]float64, 2)}); err == nil {
		t.Fatal("ungated tanh model compiled")
	}
}

func benchInferPair(b *testing.B, degree int) {
	k := newInfer32Case(b, 512, degree, 28, false)
	ctx := context.Background()
	b.Run("float64", func(b *testing.B) {
		init := make([]float64, 512)
		for b.Loop() {
			if _, err := k.m.InferLast(ctx, k.p, init, k.inputs); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("float32", func(b *testing.B) {
		f, err := k.m.CompileInfer32(k.p)
		if err != nil {
			b.Fatal(err)
		}
		init := make([]float32, 512)
		b.ReportAllocs()
		for b.Loop() {
			if _, err := f.InferLast(ctx, init, k.inputs3); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkInfer32K22(b *testing.B) { benchInferPair(b, 22) }
func BenchmarkInfer32K36(b *testing.B) { benchInferPair(b, 36) }
