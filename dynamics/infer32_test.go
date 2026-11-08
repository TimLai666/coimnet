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
	return newInfer32CaseWith(tb, nodes, degree, steps, shuffled, GateCurrentSoftsign, "softsign")
}

func newInfer32CaseWith(tb testing.TB, nodes, degree, steps int, shuffled bool, gate, activation string) infer32Case {
	rnd := rand.New(rand.NewPCG(1, 2))
	c := Config{Nodes: nodes, DT: 1, Activation: activation, Gate: gate}
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
	if gate == GateCurrentState {
		p.GateState = make([]float64, nodes)
		for i := range nodes {
			p.GateState[i] = rnd.NormFloat64()
		}
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

func TestInfer32MatchesInferForEveryGateAndActivation(t *testing.T) {
	ctx := context.Background()
	for _, gate := range []string{GateCurrentSoftsign, GateCurrent, GateCurrentState} {
		for _, act := range []string{"softsign", "tanh"} {
			k := newInfer32CaseWith(t, 200, 17, 40, true, gate, act)
			want, err := k.m.InferLast(ctx, k.p, make([]float64, 200), k.inputs)
			if err != nil {
				t.Fatal(err)
			}
			f, err := k.m.CompileInfer32(k.p)
			if err != nil {
				t.Fatalf("%s/%s: %v", gate, act, err)
			}
			got, err := f.InferLast(ctx, make([]float32, 200), k.inputs3)
			if err != nil {
				t.Fatal(err)
			}
			worst := 0.0
			for i := range want {
				worst = math.Max(worst, math.Abs(want[i]-float64(got[i])))
			}
			if worst > 1e-4 {
				t.Fatalf("%s/%s: max |float32 - float64| = %g, want <= 1e-4", gate, act, worst)
			}
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

// BenchmarkInfer32Speech is the speech model's core shape: 416 nodes, 54
// in-edges each (about 22.5k with self-loops), the sigmoid gate, tanh and 28
// steps.
func BenchmarkInfer32Speech(b *testing.B) {
	k := newInfer32CaseWith(b, 416, 54, 28, true, GateCurrent, "tanh")
	f, err := k.m.CompileInfer32(k.p)
	if err != nil {
		b.Fatal(err)
	}
	init := make([]float32, 416)
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := f.InferLast(ctx, init, k.inputs3); err != nil {
			b.Fatal(err)
		}
	}
}

// TestFloat32TanhAndSigmoidAreAccurate pins the float32 approximations
// Infer32 uses against float64 math on a dense grid.
func TestFloat32TanhAndSigmoidAreAccurate(t *testing.T) {
	var worstTanh, worstSigmoid float64
	for x := -30.0; x <= 30; x += 3e-4 {
		v := float32(x)
		worstTanh = max(worstTanh, math.Abs(float64(tanh32(v))-math.Tanh(float64(v))))
		want := 1 / (1 + math.Exp(-float64(v)))
		worstSigmoid = max(worstSigmoid, math.Abs(float64(sigmoid32(v))-want)/want)
	}
	if worstTanh > 1e-6 || worstSigmoid > 1e-6 {
		t.Fatalf("worst tanh32 absolute error %.3g, worst sigmoid32 relative error %.3g; want both below 1e-6", worstTanh, worstSigmoid)
	}
	for _, v := range []float32{0, 1e-30, -1e-30, 1e-8} {
		if got := tanh32(v); math.Abs(float64(got-v)) > 1e-6*math.Abs(float64(v))+1e-38 {
			t.Fatalf("tanh32(%g) = %g", v, got)
		}
	}
	if s := sigmoid32(-200); s < 0 || s > 1e-37 {
		t.Fatalf("sigmoid32(-200) = %g", s)
	}
	if s := sigmoid32(200); s != 1 {
		t.Fatalf("sigmoid32(200) = %g", s)
	}
}
