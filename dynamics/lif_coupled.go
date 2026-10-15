package dynamics

import (
	"context"
	"fmt"
	"math"

	"github.com/TimLai666/coimnet/internal/thresholdtail"
)

// lifCoupledTrace is the extra tape owned by a coupled LIF forward pass. The
// raw current is the input plus delayed synapses before gain, offset and bias.
// thresholdFloor records the nonzero shift rows whose effective threshold was
// clamped to theta_min, so their threshold paths are detached in reverse.
type lifCoupledTrace struct {
	sequence       ModulationSequence
	gain           [][]float64
	rawDrive       [][]float64
	thresholdFloor [][]bool
}

// ForwardCoupled evolves the LIF core with a private activity-dependent
// sequence. Sources observe the previous raw synaptic trace, initially zero.
// Current is gain*(input+synapses)+offset+bias. A nonzero threshold shift
// applies max(theta_base+adaptation+homeostasis+shift, theta_min); at or below
// that floor its derivatives to theta_raw, adaptation and shift are zero.
// The hard event keeps the declared surrogate and detached reset semantics.
// Ordinary Backward includes source feedback, adds all prior-row signals, and
// uses a fresh reverse session respecting its fixed truncation boundaries.
// Nil sequence is exactly Forward, including its allocation behavior.
// Invalid effects, cancellation, sequence/reverse errors or overflow publish
// no partial trace or gradient and never change caller-owned buffers.
func (m *LIF) ForwardCoupled(ctx context.Context, p LIFParameters, initial []float64, inputs [][]float64, sequence ModulationSequence) (*LIFTrace, error) {
	if sequence == nil {
		return m.Forward(ctx, p, initial, inputs)
	}
	return m.forwardLIFCoupled(ctx, p, initial, inputs, sequence, nil)
}

// A non-nil saved checkpoint starts the local finite segment at its owned
// voltage, synaptic history, adaptation, refractory and homeostatic boundary.
// It is an internal replay boundary, independent of persistent-state capacity.
func (m *LIF) forwardLIFCoupled(ctx context.Context, p LIFParameters, initial []float64, inputs [][]float64, sequence ModulationSequence, saved *lifCheckpoint) (*LIFTrace, error) {
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
	if len(inputs) == 0 {
		return nil, fmt.Errorf("empty sequence")
	}
	if err := vector(initial, n, "initial voltage"); err != nil {
		return nil, err
	}
	if err := vector(p.Weights, len(m.config.Sources), "weights"); err != nil {
		return nil, err
	}
	if err := vector(p.Bias, n, "bias"); err != nil {
		return nil, err
	}
	if err := vector(p.LogTau, n, "log_tau"); err != nil {
		return nil, err
	}
	if err := vector(p.ThetaRaw, n, "theta_raw"); err != nil {
		return nil, err
	}
	if len(inputs) > int(^uint(0)>>1)/n/8-1 {
		return nil, fmt.Errorf("sequence shape overflows")
	}
	lambda, alpha := make([]float64, n), make([]float64, n)
	for i, raw := range p.LogTau {
		tau := math.Exp(raw)
		if !finite(tau) || tau <= 0 {
			return nil, fmt.Errorf("tau[%d] is not representable and positive", i)
		}
		lambda[i] = math.Exp(-m.config.DT / tau)
		alpha[i] = -math.Expm1(-m.config.DT / tau)
	}
	span := m.config.ThetaMax - m.config.ThetaMin
	base, slope := make([]float64, n), make([]float64, n)
	for i, raw := range p.ThetaRaw {
		if recoveredBase, recoveredSlope, recovered := thresholdtail.Recover(raw, m.config.ThetaMin, m.config.ThetaMax); recovered {
			base[i], slope[i] = recoveredBase, recoveredSlope
			if !finite(base[i]) || !finite(slope[i]) {
				return nil, fmt.Errorf("theta_base[%d] is not representable", i)
			}
			continue
		}
		s := logistic(raw)
		base[i] = m.config.ThetaMin + span*s
		slope[i] = span * s * (1 - s)
		if math.IsInf(span, 1) {
			// Scale finite endpoints before combining an overflowing span.
			base[i] = m.config.ThetaMin*(1-s) + m.config.ThetaMax*s
			q := s * (1 - s)
			slope[i] = m.config.ThetaMax*q - m.config.ThetaMin*q
		}
		if !finite(base[i]) || !finite(slope[i]) {
			return nil, fmt.Errorf("theta_base[%d] is not representable", i)
		}
	}
	for t, in := range inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := vector(in, n, fmt.Sprintf("input[%d]", t)); err != nil {
			return nil, err
		}
	}
	steps := len(inputs)
	var history [][]float64
	var startSteps uint64
	if saved != nil {
		if saved.S < 0 {
			return nil, fmt.Errorf("checkpoint step must not be negative")
		}
		maxDelay := 0
		for _, delay := range m.config.Delays {
			if delay > maxDelay {
				maxDelay = delay
			}
		}
		lo := max(0, saved.S-maxDelay)
		if saved.Lo != lo || len(saved.Syn) != saved.S-saved.Lo+1 {
			return nil, fmt.Errorf("checkpoint does not cover the synaptic rows")
		}
		if err := vector(saved.Voltage, n, "checkpoint voltage"); err != nil {
			return nil, err
		}
		if err := vector(saved.Adapt, n, "checkpoint adaptation"); err != nil {
			return nil, err
		}
		if err := vector(saved.Rate, n, "checkpoint rate"); err != nil {
			return nil, err
		}
		if err := vector(saved.Homeo, n, "checkpoint homeostasis"); err != nil {
			return nil, err
		}
		if len(saved.Refract) != n {
			return nil, fmt.Errorf("checkpoint refractory shape")
		}
		for i, value := range saved.Refract {
			if value < 0 {
				return nil, fmt.Errorf("checkpoint refractory[%d] is negative", i)
			}
		}
		for i, row := range saved.Syn {
			if err := vector(row, n, "checkpoint synaptic output"); err != nil {
				return nil, fmt.Errorf("checkpoint row %d: %w", i, err)
			}
		}
		history = saved.Syn
		startSteps = uint64(saved.S)
	}
	tr := &LIFTrace{
		model: m, parameters: cloneLIFParameters(p),
		voltage: make([][]float64, steps+1), cand: make([][]float64, steps),
		drive: make([][]float64, steps), syn: make([][]float64, steps+1),
		spike: make([][]float64, steps+1), adapt: make([][]float64, steps+1),
		rate: make([][]float64, steps+1), homeo: make([][]float64, steps+1),
		refract: make([][]int, steps+1), reset: make([][]float64, steps),
		dspike: make([][]float64, steps), thetaSlope: slope,
		lambda: lambda, alpha: alpha,
		coupled: &lifCoupledTrace{
			sequence: sequence, gain: make([][]float64, steps),
			rawDrive: make([][]float64, steps), thresholdFloor: make([][]bool, steps),
		},
	}
	tr.voltage[0] = append([]float64(nil), initial...)
	tr.syn[0], tr.spike[0], tr.adapt[0] = make([]float64, n), make([]float64, n), make([]float64, n)
	tr.rate[0], tr.homeo[0] = make([]float64, n), make([]float64, n)
	tr.refract[0] = make([]int, n)
	if saved != nil {
		tr.voltage[0] = append([]float64(nil), saved.Voltage...)
		tr.syn[0] = saved.Syn[len(saved.Syn)-1]
		tr.adapt[0] = append([]float64(nil), saved.Adapt...)
		tr.rate[0] = append([]float64(nil), saved.Rate...)
		tr.homeo[0] = append([]float64(nil), saved.Homeo...)
		tr.refract[0] = append([]int(nil), saved.Refract...)
	}
	stabilising := m.homeostatic()
	for t, in := range inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rawDrive := append([]float64(nil), in...)
		r := forwardReader{output: tr.syn, sources: m.config.Sources, delays: m.config.Delays, t: t, history: history, startSteps: startSteps}
		if m.partition != nil {
			if err := m.lifSynapticDriveParallel(rawDrive, p.Weights, &r); err != nil {
				return nil, err
			}
		} else {
			r.accumulateSerial(rawDrive, p.Weights, m.config.Targets)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		previous := append([]float64(nil), tr.syn[t]...)
		mod, err := sequence.Forward(ctx, t, previous)
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if mod == nil {
			return nil, fmt.Errorf("modulation sequence returned nil modulation at step %d", t)
		}
		if err := mod.validate(1, n, true); err != nil {
			return nil, fmt.Errorf("modulation at step %d: %w", t, err)
		}
		gain := make([]float64, n)
		for i := range gain {
			gain[i] = 1
		}
		if mod.Gain != nil {
			copy(gain, mod.Gain[0])
		}
		offset := make([]float64, n)
		if mod.Offset != nil {
			copy(offset, mod.Offset[0])
		}
		drive := append([]float64(nil), rawDrive...)
		cand, voltage := make([]float64, n), make([]float64, n)
		spike, synapse := make([]float64, n), make([]float64, n)
		adapt, reset := make([]float64, n), make([]float64, n)
		rate, homeo := make([]float64, n), make([]float64, n)
		derivative := make([]float64, n)
		refract := make([]int, n)
		thresholdFloor := make([]bool, n)
		for i := range drive {
			if !finite(rawDrive[i]) {
				return nil, fmt.Errorf("non-finite raw drive at step %d neuron %d", t, i)
			}
			if gain[i] != 1 || offset[i] != 0 {
				drive[i] = gain[i]*rawDrive[i] + offset[i]
			}
			drive[i] += p.Bias[i]
			cand[i] = lambda[i]*tr.voltage[t][i] + alpha[i]*drive[i]
			theta := base[i]
			if m.config.Adaptation.Enabled {
				theta += tr.adapt[t][i]
			}
			if stabilising {
				theta += tr.homeo[t][i]
			}
			if mod.Threshold != nil {
				shift := mod.Threshold[0][i]
				if shift != 0 {
					shifted := theta + shift
					if math.IsInf(theta, 1) && shift < 0 {
						rawAdaptation, rawHomeostasis := 0.0, 0.0
						if m.config.Adaptation.Enabled {
							rawAdaptation = tr.adapt[t][i]
						}
						if stabilising {
							rawHomeostasis = tr.homeo[t][i]
						}
						shifted = recoverThresholdSum(base[i], rawAdaptation, rawHomeostasis, shift)
					}
					if shifted <= m.config.ThetaMin {
						theta = m.config.ThetaMin
						thresholdFloor[i] = true
					} else if !finite(shifted) {
						return nil, fmt.Errorf("non-finite threshold at step %d neuron %d", t, i)
					} else {
						theta = shifted
					}
				}
			}
			switch {
			case !m.smooth && tr.refract[t][i] > 0:
				voltage[i] = m.config.VReset
				refract[i] = tr.refract[t][i] - 1
			default:
				spike[i], derivative[i] = m.event(cand[i] - theta)
				reset[i] = 1 - spike[i]
				voltage[i] = reset[i]*cand[i] + spike[i]*m.config.VReset
				if !m.smooth && spike[i] != 0 {
					refract[i] = m.config.RefractorySteps
				}
			}
			synapse[i] = m.kappa*tr.syn[t][i] + spike[i]
			if m.config.Adaptation.Enabled {
				adapt[i] = m.rho*tr.adapt[t][i] + m.config.Adaptation.Beta*spike[i]
			}
			if stabilising {
				rate[i], homeo[i] = m.stabilise(tr.rate[t][i], tr.homeo[t][i], spike[i])
			}
			if !finite(drive[i]) || !finite(cand[i]) || !finite(theta) || !finite(voltage[i]) || !finite(synapse[i]) || !finite(adapt[i]) || !finite(rate[i]) || !finite(homeo[i]) {
				return nil, fmt.Errorf("non-finite state at step %d neuron %d", t, i)
			}
		}
		tr.coupled.gain[t], tr.coupled.rawDrive[t], tr.coupled.thresholdFloor[t] = gain, rawDrive, thresholdFloor
		tr.drive[t], tr.cand[t], tr.reset[t], tr.dspike[t] = drive, cand, reset, derivative
		tr.voltage[t+1], tr.spike[t+1], tr.syn[t+1] = voltage, spike, synapse
		tr.adapt[t+1], tr.refract[t+1] = adapt, refract
		tr.rate[t+1], tr.homeo[t+1] = rate, homeo
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return tr, nil
}

func (m *LIF) backwardLIFCoupled(ctx context.Context, tr *LIFTrace, upstream [][]float64, window int) (LIFGradient, error) {
	var empty LIFGradient
	steps, n := len(tr.drive), m.config.Nodes
	if tr.coupled == nil || tr.coupled.sequence == nil {
		return empty, fmt.Errorf("coupled trace has no modulation sequence")
	}
	if len(tr.coupled.gain) != steps || len(tr.coupled.rawDrive) != steps || len(tr.coupled.thresholdFloor) != steps || len(tr.voltage) != steps+1 || len(tr.cand) != steps || len(tr.syn) != steps+1 || len(tr.spike) != steps+1 || len(tr.adapt) != steps+1 || len(tr.refract) != steps+1 || len(tr.reset) != steps || len(tr.dspike) != steps || len(tr.alpha) != n || len(tr.lambda) != n || len(tr.thetaSlope) != n {
		return empty, fmt.Errorf("invalid coupled trace")
	}
	for t := 0; t < steps; t++ {
		if len(tr.coupled.gain[t]) != n || len(tr.coupled.rawDrive[t]) != n || len(tr.coupled.thresholdFloor[t]) != n {
			return empty, fmt.Errorf("invalid coupled trace row %d", t)
		}
		for i := 0; i < n; i++ {
			if !finite(tr.coupled.gain[t][i]) || !finite(tr.coupled.rawDrive[t][i]) {
				return empty, fmt.Errorf("non-finite coupled trace at step %d neuron %d", t, i)
			}
		}
	}
	return m.backwardLIFCoupledTape(ctx, tr, upstream, window, func(_ context.Context, step int) (*LIFTrace, int, error) { return tr, step, nil })
}
