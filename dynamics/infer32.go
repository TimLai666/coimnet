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
// float32 rounding, summation order and float32 sigmoid and tanh
// approximations (within 1e-6); it is not bit-identical and is meant
// for latency, not for training or reference results.
type Infer32 struct {
	n     int
	start []int32 // node i's edges are [start[i], start[i+1])
	// edge packs each in-edge as its float32 weight's bits in the low half
	// and its source in the high half, so the sum loads one word per edge
	// besides the gathered output.
	edge  []uint64
	bias  []float32
	gain  []float32
	gbias []float32
	// gstate is the GateCurrentState voltage weight, nil for other gates.
	gstate []float32
	alpha  []float32
	// sigmoid selects the sigmoid gate (GateCurrent, GateCurrentState) over
	// the softsign one; tanh selects the tanh activation over softsign.
	sigmoid, tanh bool
	// noSIMD keeps stepSigmoidTanh on the Go loop; tests set it to compare
	// the SSE path with it.
	noSIMD bool
	v, y   []float32
	drive  []float32
}

// CompileInfer32 packs m and p for Infer32, leaving out edges whose weight
// rounds to zero in float32. It refuses delayed edges,
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
	f := &Infer32{n: n, start: make([]int32, n+1), edge: make([]uint64, len(p.Weights)),
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
	// An edge whose weight rounds to zero adds nothing and is left out, so a
	// caller prunes edges by zeroing their weights.
	for e, t := range m.config.Targets {
		if float32(p.Weights[e]) != 0 {
			f.start[t+1]++
		}
	}
	for i := range n {
		f.start[i+1] += f.start[i]
	}
	f.edge = f.edge[:f.start[n]]
	next := append([]int32(nil), f.start[:n]...)
	for e, t := range m.config.Targets {
		if float32(p.Weights[e]) == 0 {
			continue
		}
		k := next[t]
		next[t]++
		f.edge[k] = uint64(m.config.Sources[e])<<32 | uint64(math.Float32bits(float32(p.Weights[e])))
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

// Edges is the number of edges Infer32 sums, those with a nonzero float32
// weight.
func (f *Infer32) Edges() int { return len(f.edge) }

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
	row, edge, drive := f.y[:f.n], f.edge, f.drive[:f.n]
	in = in[:f.n]
	for i := range drive {
		lo, hi := f.start[i], f.start[i+1]
		es := edge[lo:hi:hi]
		var a0, a1, a2, a3 float32
		j := 0
		for ; j+4 <= len(es); j += 4 {
			e4 := es[j : j+4 : j+4]
			a0 += math.Float32frombits(uint32(e4[0])) * row[e4[0]>>32]
			a1 += math.Float32frombits(uint32(e4[1])) * row[e4[1]>>32]
			a2 += math.Float32frombits(uint32(e4[2])) * row[e4[2]>>32]
			a3 += math.Float32frombits(uint32(e4[3])) * row[e4[3]>>32]
		}
		for ; j < len(es); j++ {
			a0 += math.Float32frombits(uint32(es[j])) * row[es[j]>>32]
		}
		drive[i] = in[i] + (a0 + a1) + (a2 + a3)
	}
}

func (f *Infer32) activate(v float32) float32 {
	if f.tanh {
		return tanh32(v)
	}
	return v / (1 + abs32(v))
}

// expNeg32 is exp(x) for x <= 0 in float32 (Cephes expf: range reduction by
// ln 2 and a degree-6 polynomial, within about 2 ulp). Arguments are clamped
// at -87, where the result is below 2^-125. It has no branches, so a stream of
// mixed-sign arguments does not mispredict.
func expNeg32(x float32) float32 {
	x = max(x, -87)
	const (
		log2e = 1.44269504088896341
		c1    = 0.693359375
		c2    = -2.12194440e-4
	)
	n := float32(int32(x*log2e - .5))
	r := x - n*c1 - n*c2
	p := ((((1.9875691500e-4*r+1.3981999507e-3)*r+8.3334519073e-3)*r+4.1665795894e-2)*r+1.6666665459e-1)*r + 5.0000001201e-1
	p = p*r*r + r + 1
	return p * math.Float32frombits(uint32(int32(n)+127)<<23)
}

const sign32 = 1 << 31

// sigmoid32 is 1/(1+exp(-z)) through e = exp(-|z|), so it cannot overflow:
// sigma(|z|) is 1/(1+e) and sigma(-|z|) is e/(1+e). The sign picks between
// them with a bit mask rather than a branch, which a stream of mixed-sign
// gates would mispredict.
func sigmoid32(z float32) float32 {
	e := expNeg32(-abs32(z))
	s := 1 / (1 + e)
	m := uint32(int32(math.Float32bits(z)) >> 31)
	return math.Float32frombits(math.Float32bits(s)&^m | math.Float32bits(e*s)&m)
}

// tanh32 is tanh in float32 as a 13/6 odd rational function on |v| <= 7.9
// (the approximation Eigen uses for float tanh, within a few ulp; near zero
// it is v times a1/b0, 1.3e-7 below one). It needs no exponential and no
// branch.
func tanh32(v float32) float32 { return tanhRational(v) }

const tanhClamp = 7.90531110763549805

// tanhRational is tanh32's body, kept small enough for the compiler to inline.
func tanhRational(v float32) float32 {
	x := v
	if x > tanhClamp {
		x = tanhClamp
	}
	if x < -tanhClamp {
		x = -tanhClamp
	}
	y := x * x
	y2 := y * y
	p := x * ((4.89352455891786e-03 + 6.37261928875436e-04*y) + y2*(1.48572235717979e-05+5.12229709037114e-08*y) +
		y2*y2*((-8.60467152213735e-11+2.00018790482477e-13*y)+y2*-2.76076847742355e-16))
	q := (4.89352518554385e-03 + 2.26843463243900e-03*y) + y2*(1.18534705686654e-04+1.19825839466702e-06*y)
	return p / q
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

// stepGeneral is step for the sigmoid gates or the tanh activation, both
// computed in float32 (sigmoid32, tanh32).
func (f *Infer32) stepGeneral() int {
	if f.sigmoid && f.tanh {
		return f.stepSigmoidTanh()
	}
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
			a = 2 * alpha[i] * sigmoid32(z)
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

// stepSigmoidTanh is stepGeneral for a sigmoid gate with the tanh
// activation, the speech recipe, with sigmoid32 and tanh32 written out in the
// loop: the compiler does not inline them, and the calls keep neighbouring
// neurons from overlapping. On amd64, without the voltage gate term, it runs
// the bulk of the neurons four at a time through stepSigmoidTanhSSE, which is
// bit-identical to this loop.
func (f *Infer32) stepSigmoidTanh() int {
	n := f.n
	drive, v, y, bias, gain, gb, alpha := f.drive[:n], f.v[:n], f.y[:n], f.bias[:n], f.gain[:n], f.gbias[:n], f.alpha[:n]
	gstate := f.gstate
	if gstate != nil {
		gstate = gstate[:n]
	}
	start := 0
	if haveSSEStep && gstate == nil && !f.noSIMD && n >= 4 {
		start = n &^ 3
		stepSigmoidTanhSSE(&drive[0], &v[0], &y[0], &bias[0], &gain[0], &gb[0], &alpha[0], &sseTable, start)
	}
	bad := -1
	for i := range start {
		if nv := v[i]; bad < 0 && nv-nv != 0 {
			bad = i
		}
	}
	for i := start; i < n; i++ {
		x := drive[i]
		d := x + bias[i]
		z := gain[i]*d + gb[i]
		if gstate != nil {
			z += gstate[i] * v[i]
		}
		// sigmoid32(z)
		e := expNeg32(-abs32(z))
		s := 1 / (1 + e)
		m := uint32(int32(math.Float32bits(z)) >> 31)
		s = math.Float32frombits(math.Float32bits(s)&^m | math.Float32bits(e*s)&m)
		a := 2 * alpha[i] * s
		if a > 1 {
			a = 1
		}
		nv := v[i] + a*(d-v[i])
		v[i] = nv
		y[i] = tanhRational(nv)
		if bad < 0 && nv-nv != 0 {
			bad = i
		}
	}
	return bad
}

func abs32(x float32) float32 { return math.Float32frombits(math.Float32bits(x) &^ sign32) }
