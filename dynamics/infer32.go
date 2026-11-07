package dynamics

import (
	"context"
	"fmt"
	"math"
)

// Infer32 is a float32 prediction path for an undelayed continuous model
// without chemistry, gated by GateCurrentSoftsign, GateCurrent or
// GateCurrentState, with the softsign or tanh activation. It stores each
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
	// gstate is the GateCurrentState voltage weight, nil for other gates.
	gstate []float32
	alpha  []float32
	// sigmoid selects the sigmoid gate (GateCurrent, GateCurrentState) over
	// the softsign one; tanh selects the tanh activation over softsign.
	sigmoid, tanh bool
	v, y          []float32
	drive         []float32
}

// CompileInfer32 packs m and p for Infer32. It refuses delayed edges,
// chemistry, partitions, an ungated model, and activations other than
// softsign and tanh.
func (m *Continuous) CompileInfer32(p Parameters) (*Infer32, error) {
	if m == nil || m.config.Nodes <= 0 {
		return nil, fmt.Errorf("uninitialized model")
	}
	gate := m.config.Gate
	if !m.zeroDelay || m.chem != nil || gate == "" || (m.act != activationSoftsign && m.act != activationTanh) {
		return nil, fmt.Errorf("Infer32 needs an undelayed gated softsign or tanh model without chemistry")
	}
	n := m.config.Nodes
	if len(m.config.Sources) > math.MaxInt32 {
		return nil, fmt.Errorf("Infer32 indexes edges with int32; %d edges do not fit", len(m.config.Sources))
	}
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
		v: make([]float32, n), y: make([]float32, n), drive: make([]float32, n),
		sigmoid: gate != GateCurrentSoftsign, tanh: m.act == activationTanh}
	if gate == GateCurrentState {
		if err := vector(p.GateState, n, "gate_state"); err != nil {
			return nil, err
		}
		f.gstate = make([]float32, n)
		for i, x := range p.GateState {
			f.gstate[i] = float32(x)
		}
	}
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

// Nodes is the core width every input row and the initial voltage must have.
func (f *Infer32) Nodes() int { return f.n }

// Reset starts an episode at the initial voltage (length Nodes).
func (f *Infer32) Reset(initial []float32) error {
	if len(initial) != f.n {
		return fmt.Errorf("initial voltage length %d, want %d", len(initial), f.n)
	}
	copy(f.v, initial)
	for i, v := range f.v {
		f.y[i] = f.activate(v)
	}
	return nil
}

// Step advances one step with in (length Nodes) as the external drive.
func (f *Infer32) Step(in []float32) error {
	if len(in) != f.n {
		return fmt.Errorf("input length %d, want %d", len(in), f.n)
	}
	f.accumulate(in)
	if i := f.step(); i >= 0 {
		return fmt.Errorf("non-finite state at neuron %d", i)
	}
	return nil
}

// Output is the activated output after the last Step, owned by f and
// overwritten by the next Reset or Step.
func (f *Infer32) Output() []float32 { return f.y }

// InferLast runs the sequence from initial voltage and returns the output
// after the last step. inputs[t] is the external drive (length n). The returned
// slice is owned by f and overwritten by the next call.
func (f *Infer32) InferLast(ctx context.Context, initial []float32, inputs [][]float32) ([]float32, error) {
	if err := f.Reset(initial); err != nil {
		return nil, err
	}
	for t, in := range inputs {
		if t%8 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if err := f.Step(in); err != nil {
			return nil, fmt.Errorf("step %d: %w", t, err)
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

func (f *Infer32) activate(v float32) float32 {
	if f.tanh {
		return float32(math.Tanh(float64(v)))
	}
	return v / (1 + abs32(v))
}

// step applies the gate and activation in place. The drive reads the
// previous outputs, so y can be overwritten only after accumulate finished.
func (f *Infer32) step() int {
	if f.sigmoid || f.tanh {
		return f.stepGeneral()
	}
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

// stepGeneral is step for the sigmoid gates or the tanh activation. The
// sigmoid and tanh run in float64 and are rounded to float32.
func (f *Infer32) stepGeneral() int {
	n := f.n
	drive, v, y, bias, gain, gb, alpha := f.drive[:n], f.v[:n], f.y[:n], f.bias[:n], f.gain[:n], f.gbias[:n], f.alpha[:n]
	bad := -1
	for i, x := range drive {
		d := x + bias[i]
		z := gain[i]*d + gb[i]
		if f.gstate != nil {
			z += f.gstate[i] * v[i]
		}
		var a float32
		if f.sigmoid {
			a = 2 * alpha[i] * float32(1/(1+math.Exp(-float64(z))))
		} else {
			a = alpha[i] * (1 + z/(1+abs32(z)))
		}
		if a > 1 {
			a = 1
		}
		nv := v[i] + a*(d-v[i])
		v[i] = nv
		y[i] = f.activate(nv)
		if bad < 0 && nv-nv != 0 {
			bad = i
		}
	}
	return bad
}

func abs32(x float32) float32 { return math.Float32frombits(math.Float32bits(x) &^ (1 << 31)) }
