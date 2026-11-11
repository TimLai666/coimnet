package dynamics

import (
	"math"
	"testing"
)

func BenchmarkInfer32SpeechParts(b *testing.B) {
	k := newInfer32CaseWith(b, 416, 54, 28, true, GateCurrent, "tanh")
	f, err := k.m.CompileInfer32(k.p)
	if err != nil {
		b.Fatal(err)
	}
	if err := f.Reset(make([]float32, 416)); err != nil {
		b.Fatal(err)
	}
	f.accumulate(k.inputs3[0])
	drive := append([]float32(nil), f.drive...)
	v0 := append([]float32(nil), f.v...)
	b.Run("accumulate", func(b *testing.B) {
		for b.Loop() {
			f.accumulate(k.inputs3[0])
		}
	})
	for _, c := range []struct {
		name string
		step func() int
	}{{"step", f.stepSigmoidTanh}, {"step-go", func() int { f.noSIMD = true; defer func() { f.noSIMD = false }(); return f.stepSigmoidTanh() }}} {
		b.Run(c.name, func(b *testing.B) {
			for b.Loop() {
				copy(f.drive, drive)
				copy(f.v, v0)
				c.step()
			}
		})
	}
}

// TestSSEStepIsBitIdentical runs the speech step with and without the SSE
// path on drives that cover tiny, ordinary, clamped and huge values.
func TestSSEStepIsBitIdentical(t *testing.T) {
	if !haveSSEStep {
		t.Skip("no SSE path on this architecture")
	}
	for _, n := range []int{4, 7, 417} {
		k := newInfer32CaseWith(t, n, 3, 1, true, GateCurrent, "tanh")
		a, _ := k.m.CompileInfer32(k.p)
		b, _ := k.m.CompileInfer32(k.p)
		b.noSIMD = true
		scales := []float32{1e-30, 1e-4, .3, 1, 5, 40, 200, 1e30}
		for step := range 40 {
			drive := make([]float32, n)
			for i := range drive {
				drive[i] = float32(math.Sin(float64(i*7+step*13))) * scales[(i+step)%len(scales)]
			}
			copy(a.drive, drive)
			copy(b.drive, drive)
			ba, bb := a.stepSigmoidTanh(), b.stepSigmoidTanh()
			if ba != bb {
				t.Fatalf("n %d step %d: bad %d vs %d", n, step, ba, bb)
			}
			for i := range n {
				if math.Float32bits(a.v[i]) != math.Float32bits(b.v[i]) || math.Float32bits(a.y[i]) != math.Float32bits(b.y[i]) {
					t.Fatalf("n %d step %d node %d: SSE v %g y %g, Go v %g y %g", n, step, i, a.v[i], a.y[i], b.v[i], b.y[i])
				}
			}
		}
	}
}
