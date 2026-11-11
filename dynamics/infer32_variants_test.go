package dynamics

import (
	"math"
	"testing"
)

// stepSigmoidTanhPaired is stepSigmoidTanh with two neurons per iteration
// sharing one division per stage: 1/(A*B) gives 1/A = B/(A*B) and
// 1/B = A/(A*B).
func (f *Infer32) stepSigmoidTanhPaired() int {
	n := f.n
	drive, v, y, bias, gain, gb, alpha := f.drive[:n], f.v[:n], f.y[:n], f.bias[:n], f.gain[:n], f.gbias[:n], f.alpha[:n]
	i := 0
	for ; i+2 <= n; i += 2 {
		d0, d1 := drive[i]+bias[i], drive[i+1]+bias[i+1]
		z0, z1 := gain[i]*d0+gb[i], gain[i+1]*d1+gb[i+1]
		e0, e1 := expNeg32(-abs32(z0)), expNeg32(-abs32(z1))
		A, B := 1+e0, 1+e1
		r := 1 / (A * B)
		s0, s1 := B*r, A*r
		m0 := uint32(int32(math.Float32bits(z0)) >> 31)
		m1 := uint32(int32(math.Float32bits(z1)) >> 31)
		s0 = math.Float32frombits(math.Float32bits(s0)&^m0 | math.Float32bits(e0*s0)&m0)
		s1 = math.Float32frombits(math.Float32bits(s1)&^m1 | math.Float32bits(e1*s1)&m1)
		a0, a1 := min(2*alpha[i]*s0, 1), min(2*alpha[i+1]*s1, 1)
		nv0 := v[i] + a0*(d0-v[i])
		nv1 := v[i+1] + a1*(d1-v[i+1])
		v[i], v[i+1] = nv0, nv1
		p0, q0 := tanhPQ(nv0)
		p1, q1 := tanhPQ(nv1)
		r = 1 / (q0 * q1)
		y[i], y[i+1] = p0*q1*r, p1*q0*r
	}
	for ; i < n; i++ {
		d := drive[i] + bias[i]
		z := gain[i]*d + gb[i]
		a := min(2*alpha[i]*sigmoid32(z), 1)
		nv := v[i] + a*(d-v[i])
		v[i] = nv
		y[i] = tanhRational(nv)
	}
	return -1
}

func tanhPQ(v float32) (float32, float32) {
	x := min(max(v, -tanhClamp), tanhClamp)
	y := x * x
	y2 := y * y
	p := x * ((4.89352455891786e-03 + 6.37261928875436e-04*y) + y2*(1.48572235717979e-05+5.12229709037114e-08*y) +
		y2*y2*((-8.60467152213735e-11+2.00018790482477e-13*y)+y2*-2.76076847742355e-16))
	q := (4.89352518554385e-03 + 2.26843463243900e-03*y) + y2*(1.18534705686654e-04+1.19825839466702e-06*y)
	return p, q
}

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
	}{{"step", f.stepSigmoidTanh}, {"step-paired", f.stepSigmoidTanhPaired}} {
		b.Run(c.name, func(b *testing.B) {
			for b.Loop() {
				copy(f.drive, drive)
				copy(f.v, v0)
				c.step()
			}
		})
	}
}

func TestPairedStepMatches(t *testing.T) {
	k := newInfer32CaseWith(t, 417, 20, 1, true, GateCurrent, "tanh")
	f, _ := k.m.CompileInfer32(k.p)
	_ = f.Reset(make([]float32, 417))
	f.accumulate(k.inputs3[0])
	drive := append([]float32(nil), f.drive...)
	f.stepSigmoidTanh()
	want := append([]float32(nil), f.y...)
	_ = f.Reset(make([]float32, 417))
	copy(f.drive, drive)
	f.stepSigmoidTanhPaired()
	for i := range want {
		if d := math.Abs(float64(want[i] - f.y[i])); d > 1e-6 {
			t.Fatalf("node %d: %g vs %g", i, f.y[i], want[i])
		}
	}
}
