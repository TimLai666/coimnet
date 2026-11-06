package dynamics

import (
	"context"
	"fmt"
	"math"
)

// Infer32 is a float32 prediction path for an undelayed continuous model with
// the softsign gate and softsign activation and no chemistry. It stores each
// node's in-edges contiguously (int32 sources, float32 weights) and sums them
// with four independent accumulators, so its outputs differ from Infer by
// float32 rounding and summation order; it is not bit-identical and is meant
// for latency, not for training or reference results.
type Infer32 struct {
	n     int
	start []int32 // node i's edges are [start[i], start[i+1])
	src   []int32
	w     []float32
	bias  []float32
	gain  []float32
	gbias []float32
	alpha []float32
	v, y  []float32
	drive []float32
}

// CompileInfer32 packs m and p for Infer32. It refuses delayed edges,
// chemistry, partitions, and gate or activation kinds other than softsign.
func (m *Continuous) CompileInfer32(p Parameters) (*Infer32, error) {
	if m == nil || m.config.Nodes <= 0 {
		return nil, fmt.Errorf("uninitialized model")
	}
	if !m.zeroDelay || m.chem != nil || m.config.Gate != GateCurrentSoftsign || m.act != activationSoftsign {
		return nil, fmt.Errorf("Infer32 needs an undelayed softsign-gated softsign model without chemistry")
	}
	n := m.config.Nodes
	if err := vector(p.Weights, len(m.config.Sources), "weights"); err != nil {
		return nil, err
	}
	for name, v := range map[string][]float64{"bias": p.Bias, "log_tau": p.LogTau, "gate_gain": p.GateGain, "gate_bias": p.GateBias} {
		if err := vector(v, n, name); err != nil {
			return nil, err
		}
	}
	f := &Infer32{n: n, start: make([]int32, n+1), src: make([]int32, len(p.Weights)), w: make([]float32, len(p.Weights)),
		bias: make([]float32, n), gain: make([]float32, n), gbias: make([]float32, n), alpha: make([]float32, n),
		v: make([]float32, n), y: make([]float32, n), drive: make([]float32, n)}
	for _, t := range m.config.Targets {
		f.start[t+1]++
	}
	for i := range n {
		f.start[i+1] += f.start[i]
	}
	next := append([]int32(nil), f.start[:n]...)
	for e, t := range m.config.Targets {
		k := next[t]
		next[t]++
		f.src[k] = int32(m.config.Sources[e])
		f.w[k] = float32(p.Weights[e])
	}
	for i := range n {
		tau := math.Exp(p.LogTau[i])
		if !finite(tau) || tau <= 0 {
			return nil, fmt.Errorf("tau[%d] is not representable and positive", i)
		}
		f.alpha[i] = float32(-math.Expm1(-m.config.DT / tau))
		f.bias[i], f.gain[i], f.gbias[i] = float32(p.Bias[i]), float32(p.GateGain[i]), float32(p.GateBias[i])
	}
	return f, nil
}

// InferLast runs the sequence from initial voltage and returns the output
// after the last step. inputs[t] is the external drive (length n) and is read
// as float32. The returned slice is owned by f and overwritten by the next call.
func (f *Infer32) InferLast(ctx context.Context, initial []float32, inputs [][]float32) ([]float32, error) {
	if len(initial) != f.n {
		return nil, fmt.Errorf("initial voltage length %d, want %d", len(initial), f.n)
	}
	copy(f.v, initial)
	for i, v := range f.v {
		f.y[i] = v / (1 + abs32(v))
	}
	for t, in := range inputs {
		if len(in) != f.n {
			return nil, fmt.Errorf("input[%d] length %d, want %d", t, len(in), f.n)
		}
		if t%8 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		f.accumulate(in)
		if i := f.step(); i >= 0 {
			return nil, fmt.Errorf("non-finite state at step %d neuron %d", t, i)
		}
	}
	return f.y, nil
}

func (f *Infer32) accumulate(in []float32) {
	row, src, w, drive := f.y, f.src, f.w, f.drive[:f.n]
	in = in[:f.n]
	for i := range drive {
		lo, hi := f.start[i], f.start[i+1]
		ss, ws := src[lo:hi:hi], w[lo:hi:hi]
		ws = ws[:len(ss)]
		var a0, a1, a2, a3 float32
		j := 0
		for ; j+4 <= len(ss); j += 4 {
			a0 += ws[j] * row[ss[j]]
			a1 += ws[j+1] * row[ss[j+1]]
			a2 += ws[j+2] * row[ss[j+2]]
			a3 += ws[j+3] * row[ss[j+3]]
		}
		for ; j < len(ss); j++ {
			a0 += ws[j] * row[ss[j]]
		}
		drive[i] = in[i] + (a0 + a1) + (a2 + a3)
	}
}

// step applies the softsign gate and activation in place. The drive reads the
// previous outputs, so y can be overwritten only after accumulate finished.
func (f *Infer32) step() int {
	n := f.n
	drive, v, y, bias, gain, gb, alpha := f.drive[:n], f.v[:n], f.y[:n], f.bias[:n], f.gain[:n], f.gbias[:n], f.alpha[:n]
	bad := -1
	for i, x := range drive {
		d := x + bias[i]
		z := gain[i]*d + gb[i]
		a := alpha[i] * (1 + z/(1+abs32(z)))
		if a > 1 {
			a = 1
		}
		nv := v[i] + a*(d-v[i])
		v[i] = nv
		y[i] = nv / (1 + abs32(nv))
		if bad < 0 && nv-nv != 0 {
			bad = i
		}
	}
	return bad
}

func abs32(x float32) float32 { return math.Float32frombits(math.Float32bits(x) &^ (1 << 31)) }
