package dynamics

import (
	"context"
	"fmt"
	"math"
)

// lifCheckpoint is what the LIF forward needs to restart at row S: the per-node
// state of row S and the syn rows Lo..S, Lo = max(0, S - maxDelay).
type lifCheckpoint struct {
	S, Lo                       int
	Syn                         [][]float64 // rows Lo..S
	Voltage, Adapt, Rate, Homeo []float64   // row S
	Refract                     []int       // row S
}

// lifSegment is Forward's trace for the steps [Start, End) re-run from a
// checkpoint. Rows are absolute-indexed through the accessors so callers never
// do offset arithmetic.
type lifSegment struct {
	Start, End, Lo       int
	syn                  [][]float64 // rows Lo..End
	voltage, adapt, rate [][]float64 // rows Start..End (row Start copied from the checkpoint)
	homeo, spike         [][]float64 // rows Start..End (homeo row Start copied; spike[Start] is nil)
	refract              [][]int     // rows Start..End
	cand, drive, reset   [][]float64 // steps Start..End-1
	dspike               [][]float64 // steps Start..End-1
}

// SynRow returns the synaptic trace of absolute row r, for r in [Lo, End].
func (g *lifSegment) SynRow(r int) []float64 {
	return g.syn[r-g.Lo]
}

// VoltageRow returns the post-reset membrane voltage of absolute row r, for
// r in [Start, End].
func (g *lifSegment) VoltageRow(r int) []float64 {
	return g.voltage[r-g.Start]
}

// Step returns the four per-step values cand, drive, reset and dspike for
// absolute step t, for t in [Start, End).
func (g *lifSegment) Step(t int) (cand, drive, reset, dspike []float64) {
	i := t - g.Start
	return g.cand[i], g.drive[i], g.reset[i], g.dspike[i]
}

// lifInitialCheckpoint is row 0: Voltage = a copy of initial, Syn = one zero
// row, Adapt/Rate/Homeo zeros, Refract zeros.
func (m *LIF) lifInitialCheckpoint(initial []float64) lifCheckpoint {
	n := m.config.Nodes
	return lifCheckpoint{
		S:       0,
		Lo:      0,
		Syn:     [][]float64{make([]float64, n)},
		Voltage: append([]float64(nil), initial...),
		Adapt:   make([]float64, n),
		Rate:    make([]float64, n),
		Homeo:   make([]float64, n),
		Refract: make([]int, n),
	}
}

// lifConstants returns lambda, alpha, base (theta_base) and slope exactly as
// Forward computes them, with the same errors.
func (m *LIF) lifConstants(p LIFParameters) (lambda, alpha, base, slope []float64, err error) {
	if m == nil {
		return nil, nil, nil, nil, fmt.Errorf("nil model")
	}
	n := m.config.Nodes
	if n <= 0 {
		return nil, nil, nil, nil, fmt.Errorf("uninitialized model")
	}
	if err := vector(p.LogTau, n, "log_tau"); err != nil {
		return nil, nil, nil, nil, err
	}
	if err := vector(p.ThetaRaw, n, "theta_raw"); err != nil {
		return nil, nil, nil, nil, err
	}
	lambda, alpha = make([]float64, n), make([]float64, n)
	for i, raw := range p.LogTau {
		tau := math.Exp(raw)
		if !finite(tau) || tau <= 0 {
			return nil, nil, nil, nil, fmt.Errorf("tau[%d] is not representable and positive", i)
		}
		lambda[i] = math.Exp(-m.config.DT / tau)
		// Avoid cancellation in 1-exp(-dt/tau) for small positive dt/tau.
		alpha[i] = -math.Expm1(-m.config.DT / tau)
	}
	span := m.config.ThetaMax - m.config.ThetaMin
	base, slope = make([]float64, n), make([]float64, n)
	for i, raw := range p.ThetaRaw {
		s := logistic(raw)
		base[i] = m.config.ThetaMin + span*s
		slope[i] = span * s * (1 - s)
		if !finite(base[i]) || !finite(slope[i]) {
			return nil, nil, nil, nil, fmt.Errorf("theta_base[%d] is not representable", i)
		}
	}
	return lambda, alpha, base, slope, nil
}

// lifRunSegment re-runs Forward's per-step loop for the steps [cp.S, end)
// starting from cp. Every floating-point expression is copied from Forward
// character for character and in the same order (arm64 fuses a*b + c, so even
// the expression shape must match); the edge drive uses
// forwardReader.accumulateSerial over a syn row slice indexed absolutely (a
// [][]float64 of length end+1 holding only the rows Lo..end). ctx is checked
// per step; non-finite state is an error exactly like Forward.
func (m *LIF) lifRunSegment(ctx context.Context, p LIFParameters, inputs [][]float64, cp lifCheckpoint, end int, lambda, alpha, base []float64) (*lifSegment, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("nil model")
	}
	n := m.config.Nodes
	if n <= 0 {
		return nil, fmt.Errorf("uninitialized model")
	}
	start := cp.S
	if start < 0 || end < start || end > len(inputs) {
		return nil, fmt.Errorf("segment range out of bounds")
	}
	if err := vector(p.Weights, len(m.config.Sources), "weights"); err != nil {
		return nil, err
	}
	if err := vector(p.Bias, n, "bias"); err != nil {
		return nil, err
	}
	maxDelay := 0
	for _, d := range m.config.Delays {
		if d > maxDelay {
			maxDelay = d
		}
	}
	lo := max(0, start-maxDelay)
	if cp.Lo != lo {
		return nil, fmt.Errorf("checkpoint does not cover the synaptic rows")
	}
	synAll := make([][]float64, end+1)
	for r := lo; r <= start; r++ {
		synAll[r] = cp.Syn[r-lo]
	}
	seg := &lifSegment{
		Start:   start,
		End:     end,
		Lo:      lo,
		syn:     make([][]float64, end-lo+1),
		voltage: make([][]float64, end-start+1),
		adapt:   make([][]float64, end-start+1),
		rate:    make([][]float64, end-start+1),
		homeo:   make([][]float64, end-start+1),
		spike:   make([][]float64, end-start+1),
		refract: make([][]int, end-start+1),
		cand:    make([][]float64, end-start),
		drive:   make([][]float64, end-start),
		reset:   make([][]float64, end-start),
		dspike:  make([][]float64, end-start),
	}
	for r := lo; r <= start; r++ {
		seg.syn[r-lo] = synAll[r]
	}
	seg.voltage[0] = cp.Voltage
	seg.adapt[0] = cp.Adapt
	seg.rate[0] = cp.Rate
	seg.homeo[0] = cp.Homeo
	seg.refract[0] = cp.Refract
	stabilising := m.homeostatic()
	for t := start; t < end; t++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		drive := append([]float64(nil), inputs[t]...)
		r := forwardReader{output: synAll, sources: m.config.Sources, delays: m.config.Delays, t: t}
		r.accumulateSerial(drive, p.Weights, m.config.Targets)
		cand, voltage := make([]float64, n), make([]float64, n)
		spike, synapse := make([]float64, n), make([]float64, n)
		adapt, reset := make([]float64, n), make([]float64, n)
		rate, homeo := make([]float64, n), make([]float64, n)
		derivative := make([]float64, n)
		refract := make([]int, n)
		si := t - start
		for i := range drive {
			drive[i] += p.Bias[i]
			cand[i] = lambda[i]*seg.voltage[si][i] + alpha[i]*drive[i]
			theta := base[i]
			if m.config.Adaptation.Enabled {
				theta += seg.adapt[si][i]
			}
			if stabilising {
				theta += seg.homeo[si][i]
			}
			switch {
			case !m.smooth && seg.refract[si][i] > 0:
				// Hold and ignore: the drive of this step never reaches the
				// membrane, so it carries no gradient either.
				voltage[i] = m.config.VReset
				refract[i] = seg.refract[si][i] - 1
			default:
				spike[i], derivative[i] = m.event(cand[i] - theta)
				reset[i] = 1 - spike[i]
				voltage[i] = reset[i]*cand[i] + spike[i]*m.config.VReset
				if !m.smooth && spike[i] != 0 {
					refract[i] = m.config.RefractorySteps
				}
			}
			synapse[i] = m.kappa*synAll[t][i] + spike[i]
			if m.config.Adaptation.Enabled {
				adapt[i] = m.rho*seg.adapt[si][i] + m.config.Adaptation.Beta*spike[i]
			}
			if stabilising {
				rate[i], homeo[i] = m.stabilise(seg.rate[si][i], seg.homeo[si][i], spike[i])
			}
			if !finite(drive[i]) || !finite(cand[i]) || !finite(voltage[i]) || !finite(synapse[i]) || !finite(adapt[i]) || !finite(rate[i]) || !finite(homeo[i]) {
				return nil, fmt.Errorf("non-finite state at step %d neuron %d", t, i)
			}
		}
		seg.drive[si], seg.cand[si], seg.reset[si], seg.dspike[si] = drive, cand, reset, derivative
		seg.voltage[si+1], seg.spike[si+1], seg.syn[t+1-lo] = voltage, spike, synapse
		seg.adapt[si+1], seg.refract[si+1] = adapt, refract
		seg.rate[si+1], seg.homeo[si+1] = rate, homeo
		synAll[t+1] = synapse
	}
	return seg, nil
}

// checkpointAt returns the checkpoint of row r (Start <= r <= End) with syn
// rows max(0, r-maxDelay)..r copied out of the segment (the caller guarantees
// r - maxDelay >= Lo or r - maxDelay < 0).
func (g *lifSegment) checkpointAt(r, maxDelay int) lifCheckpoint {
	lo := max(0, r-maxDelay)
	syn := make([][]float64, r-lo+1)
	for row := lo; row <= r; row++ {
		syn[row-lo] = append([]float64(nil), g.SynRow(row)...)
	}
	rel := r - g.Start
	return lifCheckpoint{
		S:       r,
		Lo:      lo,
		Syn:     syn,
		Voltage: append([]float64(nil), g.voltage[rel]...),
		Adapt:   append([]float64(nil), g.adapt[rel]...),
		Rate:    append([]float64(nil), g.rate[rel]...),
		Homeo:   append([]float64(nil), g.homeo[rel]...),
		Refract: append([]int(nil), g.refract[rel]...),
	}
}
