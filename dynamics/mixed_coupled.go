package dynamics

import (
	"context"
	"fmt"
	"math"
)

// mixedCoupledTrace is the extra tape owned by a coupled mixed forward pass.
// The ordinary MixedTrace remains unchanged on the nil-sequence path. Every
// row here is a private copy of the raw current, gain and threshold-floor
// decision used by that step.
type mixedCoupledTrace struct {
	sequence       ModulationSequence
	gain           [][]float64
	rawDrive       [][]float64
	thresholdFloor [][]bool
}

// ForwardCoupled evolves each mixed node under its declared rule with coupled
// chemical effects. Sources observe the previous raw mixed output: activated
// voltage on continuous nodes, synaptic trace on LIF nodes. Current effects
// precede bias. Nonzero threshold on a continuous node is rejected, while LIF
// thresholds follow the shifted floor and derivative rules of LIF.
// Its ordinary Backward includes every source/activity contribution under
// the same fixed truncation boundaries and fresh reverse-session contract.
// Nil is exactly Forward. Failures publish no partial trace or gradient.
func (m *Mixed) ForwardCoupled(ctx context.Context, p MixedParameters, initial []float64, inputs [][]float64, sequence ModulationSequence) (*MixedTrace, error) {
	if sequence == nil {
		return m.Forward(ctx, p, initial, inputs)
	}
	return m.forwardCoupled(ctx, p, initial, inputs, sequence)
}

func (m *Mixed) forwardCoupled(ctx context.Context, p MixedParameters, initial []float64, inputs [][]float64, sequence ModulationSequence) (*MixedTrace, error) {
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
	if err := m.validateParameters(p); err != nil {
		return nil, err
	}
	if len(inputs) > int(^uint(0)>>1)/n/8-1 {
		return nil, fmt.Errorf("sequence shape overflows")
	}
	lambda, alpha, err := m.leak(p.LogTau)
	if err != nil {
		return nil, err
	}
	base, slope, err := m.baseThreshold(p.ThetaRaw, true)
	if err != nil {
		return nil, err
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
	tr := &MixedTrace{
		model: m, parameters: cloneMixedParameters(p),
		out: make([][]float64, steps+1), voltage: make([][]float64, steps+1),
		drive: make([][]float64, steps), cand: make([][]float64, steps),
		spike: make([][]float64, steps+1), adapt: make([][]float64, steps+1),
		rate: make([][]float64, steps+1), homeo: make([][]float64, steps+1),
		refract: make([][]int, steps+1), reset: make([][]float64, steps),
		dspike: make([][]float64, steps), thetaSlope: slope,
		lambda: lambda, alpha: alpha,
		coupled: &mixedCoupledTrace{
			sequence:       sequence,
			gain:           make([][]float64, steps),
			rawDrive:       make([][]float64, steps),
			thresholdFloor: make([][]bool, steps),
		},
	}
	tr.voltage[0] = append([]float64(nil), initial...)
	tr.out[0] = make([]float64, n)
	for _, i := range m.continuousNodes {
		tr.out[0][i] = m.act.activate(initial[i])
	}
	tr.spike[0], tr.adapt[0] = make([]float64, n), make([]float64, n)
	tr.rate[0], tr.homeo[0] = make([]float64, n), make([]float64, n)
	tr.refract[0] = make([]int, n)
	adapting, stabilising := m.adapting(), m.stabilising()
	for t, in := range inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rawDrive := append([]float64(nil), in...)
		for e, s := range m.config.Sources {
			if e%4096 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			past := 0
			if m.config.Delays[e] < t {
				past = t - m.config.Delays[e]
			} // avoid subtraction overflow
			rawDrive[m.config.Targets[e]] += p.Weights[e] * tr.out[past][s]
		}
		for i, value := range rawDrive {
			if !finite(value) {
				return nil, fmt.Errorf("non-finite raw drive at step %d neuron %d", t, i)
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		previous := append([]float64(nil), tr.out[t]...)
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
		if err := m.refuseContinuousThreshold(mod); err != nil {
			return nil, fmt.Errorf("modulation at step %d: %w", t, err)
		}
		gain := make([]float64, n)
		for i := range gain {
			gain[i] = 1
		}
		offset := make([]float64, n)
		threshold := make([]float64, n)
		if mod.Gain != nil {
			copy(gain, mod.Gain[0])
		}
		if mod.Offset != nil {
			copy(offset, mod.Offset[0])
		}
		if mod.Threshold != nil {
			copy(threshold, mod.Threshold[0])
		}
		owned := &Modulation{Gain: [][]float64{gain}, Offset: [][]float64{offset}}
		voltage, out := make([]float64, n), make([]float64, n)
		cand, spike := make([]float64, n), make([]float64, n)
		adapt, reset := make([]float64, n), make([]float64, n)
		rate, homeo := make([]float64, n), make([]float64, n)
		derivative := make([]float64, n)
		refract := make([]int, n)
		thresholdFloor := make([]bool, n)
		drive := make([]float64, n)
		for i := range rawDrive {
			drive[i] = modulatedDrive(rawDrive[i], owned, 0, i)
			drive[i] += p.Bias[i]
			if !finite(drive[i]) {
				return nil, fmt.Errorf("non-finite drive at step %d neuron %d", t, i)
			}
			next := membrane(lambda[i], tr.voltage[t][i], alpha[i], drive[i])
			if !m.spiking(i) {
				voltage[i] = next
				out[i] = m.act.activate(voltage[i])
				if !finite(voltage[i]) || !finite(out[i]) {
					return nil, fmt.Errorf("non-finite state at step %d neuron %d", t, i)
				}
				continue
			}
			k := m.position[i]
			cand[i] = next
			theta := base[k]
			if adapting {
				theta += tr.adapt[t][i]
			}
			if stabilising {
				theta += tr.homeo[t][i]
			}
			if mod.Threshold != nil && threshold[i] != 0 {
				shifted := theta + threshold[i]
				if math.IsInf(theta, 1) && threshold[i] < 0 {
					rawAdaptation, rawHomeostasis := 0.0, 0.0
					if adapting {
						rawAdaptation = tr.adapt[t][i]
					}
					if stabilising {
						rawHomeostasis = tr.homeo[t][i]
					}
					shifted = recoverThresholdSum(base[k], rawAdaptation, rawHomeostasis, threshold[i])
				}
				if shifted <= m.config.LIF.ThetaMin {
					theta = m.config.LIF.ThetaMin
					thresholdFloor[i] = true
				} else if !finite(shifted) {
					return nil, fmt.Errorf("non-finite threshold at step %d neuron %d", t, i)
				} else {
					theta = shifted
				}
			}
			switch {
			case !m.smooth && tr.refract[t][i] > 0:
				// Hold and ignore: the drive of this step never reaches the
				// membrane, so it carries no gradient either.
				voltage[i] = m.config.LIF.VReset
				refract[i] = tr.refract[t][i] - 1
			default:
				spike[i], derivative[i] = m.event(cand[i] - theta)
				reset[i] = 1 - spike[i]
				voltage[i] = reset[i]*cand[i] + spike[i]*m.config.LIF.VReset
				if !m.smooth && spike[i] != 0 {
					refract[i] = m.config.LIF.RefractorySteps
				}
			}
			out[i] = m.spike.kappa*tr.out[t][i] + spike[i]
			if adapting {
				adapt[i] = m.spike.rho*tr.adapt[t][i] + m.config.LIF.Adaptation.Beta*spike[i]
			}
			if stabilising {
				rate[i], homeo[i] = m.spike.stabilise(tr.rate[t][i], tr.homeo[t][i], spike[i])
			}
			if !finite(cand[i]) || !finite(theta) || !finite(voltage[i]) || !finite(out[i]) || !finite(adapt[i]) || !finite(rate[i]) || !finite(homeo[i]) {
				return nil, fmt.Errorf("non-finite state at step %d neuron %d", t, i)
			}
		}
		tr.coupled.gain[t] = gain
		tr.coupled.rawDrive[t] = rawDrive
		tr.coupled.thresholdFloor[t] = thresholdFloor
		tr.drive[t], tr.cand[t], tr.reset[t], tr.dspike[t] = drive, cand, reset, derivative
		tr.voltage[t+1], tr.spike[t+1], tr.out[t+1] = voltage, spike, out
		tr.adapt[t+1], tr.refract[t+1] = adapt, refract
		tr.rate[t+1], tr.homeo[t+1] = rate, homeo
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return tr, nil
}

func (m *Mixed) backwardCoupled(ctx context.Context, tr *MixedTrace, upstream [][]float64, window int) (MixedGradient, error) {
	var empty MixedGradient
	if ctx == nil {
		return empty, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if m == nil || tr == nil || tr.model != m {
		return empty, fmt.Errorf("trace belongs to a different model")
	}
	steps, n := len(tr.drive), m.config.Nodes
	if window < 0 || len(upstream) != steps {
		return empty, fmt.Errorf("invalid backward window or sequence length")
	}
	if tr.coupled == nil || tr.coupled.sequence == nil {
		return empty, fmt.Errorf("coupled trace has no modulation sequence")
	}
	if len(tr.coupled.gain) != steps || len(tr.coupled.rawDrive) != steps || len(tr.coupled.thresholdFloor) != steps ||
		len(tr.out) != steps+1 || len(tr.voltage) != steps+1 || len(tr.spike) != steps+1 ||
		len(tr.adapt) != steps+1 || len(tr.rate) != steps+1 || len(tr.homeo) != steps+1 || len(tr.refract) != steps+1 ||
		len(tr.cand) != steps || len(tr.drive) != steps || len(tr.reset) != steps || len(tr.dspike) != steps ||
		len(tr.alpha) != n || len(tr.lambda) != n || len(tr.thetaSlope) != len(m.lifNodes) {
		return empty, fmt.Errorf("invalid coupled trace")
	}
	if err := m.validateParameters(tr.parameters); err != nil {
		return empty, fmt.Errorf("invalid coupled trace parameters: %w", err)
	}
	checkRow := func(row []float64, name string) error {
		return vector(row, n, name)
	}
	for t := 0; t <= steps; t++ {
		for name, row := range map[string][]float64{
			"output": tr.out[t], "voltage": tr.voltage[t], "spike": tr.spike[t],
			"adaptation": tr.adapt[t], "rate": tr.rate[t], "homeostasis": tr.homeo[t],
		} {
			if err := checkRow(row, fmt.Sprintf("trace %s[%d]", name, t)); err != nil {
				return empty, err
			}
		}
		if len(tr.refract[t]) != n {
			return empty, fmt.Errorf("trace refractory[%d] length %d, want %d", t, len(tr.refract[t]), n)
		}
	}
	for t := 0; t < steps; t++ {
		for name, row := range map[string][]float64{
			"drive": tr.drive[t], "candidate": tr.cand[t], "reset": tr.reset[t], "dspike": tr.dspike[t],
			"gain": tr.coupled.gain[t], "raw_drive": tr.coupled.rawDrive[t],
		} {
			if err := checkRow(row, fmt.Sprintf("trace %s[%d]", name, t)); err != nil {
				return empty, err
			}
		}
		if len(tr.coupled.thresholdFloor[t]) != n {
			return empty, fmt.Errorf("trace threshold floor[%d] length %d, want %d", t, len(tr.coupled.thresholdFloor[t]), n)
		}
	}
	for name, values := range map[string][]float64{"alpha": tr.alpha, "lambda": tr.lambda, "theta_slope": tr.thetaSlope} {
		for i, value := range values {
			if !finite(value) {
				return empty, fmt.Errorf("trace %s[%d] is non-finite", name, i)
			}
		}
	}
	gy := make([][]float64, steps+1)
	for t := range gy {
		gy[t] = make([]float64, n)
	}
	for t, row := range upstream {
		if err := vector(row, n, fmt.Sprintf("upstream[%d]", t)); err != nil {
			return empty, err
		}
		copy(gy[t+1], row)
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	reverse, err := tr.coupled.sequence.NewReverse(ctx, steps, window)
	if err != nil {
		return empty, err
	}
	if reverse == nil {
		return empty, fmt.Errorf("modulation sequence returned nil reverse")
	}
	readout, err := coupledReadoutAdjoints(ctx, reverse, upstream, n)
	if err != nil {
		return empty, err
	}
	if readout != nil {
		for t, row := range readout {
			copy(gy[t+1], row)
		}
	}
	g := MixedGradient{
		Weights: make([]float64, len(tr.parameters.Weights)), Bias: make([]float64, n),
		LogTau: make([]float64, n), ThetaRaw: make([]float64, len(m.lifNodes)),
		Inputs: make([][]float64, steps), Initial: make([]float64, n),
	}
	adapting := m.adapting()
	gv, ga := make([]float64, n), make([]float64, n)
	for t := steps - 1; t >= 0; t-- {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		start := 0
		if window > 0 {
			start = t / window * window
		}
		keep := t > start || start == 0
		prevV, prevA := make([]float64, n), make([]float64, n)
		g.Inputs[t] = make([]float64, n)
		modGradient := ModulationGradient{Gain: make([]float64, n), Offset: make([]float64, n), Threshold: make([]float64, n)}
		for i := 0; i < n; i++ {
			var dcand float64
			if !m.spiking(i) {
				dv := gv[i] + gy[t+1][i]*m.act.derivative(tr.voltage[t+1][i])
				if !finite(dv) {
					return empty, fmt.Errorf("non-finite coupled voltage adjoint at step %d neuron %d", t, i)
				}
				dcand = dv
			} else {
				k := m.position[i]
				dspike := gy[t+1][i]
				if adapting {
					dspike += ga[i] * m.config.LIF.Adaptation.Beta
				}
				if m.smooth {
					dspike += gv[i] * (m.config.LIF.VReset - tr.cand[t][i])
				}
				if keep {
					gx := m.spike.kappa * gy[t+1][i]
					if !finite(gx) || !finite(gy[t][i]+gx) {
						return empty, fmt.Errorf("coupled synaptic adjoint overflow at step %d neuron %d", t, i)
					}
					gy[t][i] += gx
					if adapting {
						prevA[i] = m.spike.rho * ga[i]
						if !finite(prevA[i]) {
							return empty, fmt.Errorf("coupled adaptation adjoint overflow at step %d neuron %d", t, i)
						}
					}
				}
				du := dspike * tr.dspike[t][i]
				dcand = gv[i]*tr.reset[t][i] + du
				if !finite(du) {
					du = m.spike.recoverEventAdjoint(gy[t+1][i], ga[i], gv[i], tr.cand[t][i], tr.dspike[t][i], m.smooth)
					dcand = gv[i]*tr.reset[t][i] + du
				}
				if !finite(du) || !finite(dcand) {
					return empty, fmt.Errorf("non-finite coupled event adjoint at step %d neuron %d", t, i)
				}
				if !tr.coupled.thresholdFloor[t][i] {
					if adapting && keep {
						prevA[i] -= du
						if !finite(prevA[i]) {
							return empty, fmt.Errorf("coupled adaptation adjoint overflow at step %d neuron %d", t, i)
						}
					}
					thetaGradient := du * tr.thetaSlope[k]
					if !finite(thetaGradient) || !finite(g.ThetaRaw[k]-thetaGradient) {
						return empty, fmt.Errorf("coupled theta adjoint overflow at step %d neuron %d", t, i)
					}
					g.ThetaRaw[k] -= thetaGradient
					modGradient.Threshold[i] = -du
				}
			}
			if !finite(dcand) {
				return empty, fmt.Errorf("non-finite coupled candidate adjoint at step %d neuron %d", t, i)
			}
			dCurrent := tr.alpha[i] * dcand
			if !finite(dCurrent) {
				return empty, fmt.Errorf("coupled current adjoint overflow at step %d neuron %d", t, i)
			}
			gain := tr.coupled.gain[t][i]
			inputGradient := dCurrent
			if gain != 1 {
				inputGradient = dCurrent * gain
			}
			if !finite(inputGradient) {
				return empty, fmt.Errorf("coupled input adjoint overflow at step %d neuron %d", t, i)
			}
			g.Inputs[t][i] = inputGradient
			if !finite(g.Bias[i] + dCurrent) {
				return empty, fmt.Errorf("coupled bias adjoint overflow at step %d neuron %d", t, i)
			}
			// Match the ordinary path's separately stored current adjoint.
			g.Bias[i] += float64(dCurrent)
			modGradient.Gain[i] = tr.coupled.rawDrive[t][i] * dCurrent
			modGradient.Offset[i] = dCurrent
			if !finite(modGradient.Gain[i]) || !finite(modGradient.Offset[i]) {
				return empty, fmt.Errorf("coupled modulation adjoint overflow at step %d neuron %d", t, i)
			}
			tau := math.Exp(tr.parameters.LogTau[i])
			dlambda := tr.lambda[i] * (m.config.DT / tau)
			if tr.lambda[i] == 0 {
				dlambda = 0
			}
			if !finite(dlambda) {
				return empty, fmt.Errorf("coupled log_tau adjoint overflow at step %d neuron %d", t, i)
			}
			logTauGradient := dcand * (tr.voltage[t][i] - tr.drive[t][i]) * dlambda
			if !finite(logTauGradient) {
				recovered := membraneTauGradient(dcand, tr.voltage[t][i], tr.drive[t][i], dlambda)
				if !finite(recovered) || !finite(g.LogTau[i]+recovered) {
					return empty, fmt.Errorf("coupled log_tau adjoint overflow at step %d neuron %d", t, i)
				}
				g.LogTau[i] += recovered
			} else {
				if !finite(g.LogTau[i] + logTauGradient) {
					return empty, fmt.Errorf("coupled log_tau adjoint overflow at step %d neuron %d", t, i)
				}
				g.LogTau[i] += logTauGradient
			}
			if keep {
				prevV[i] = tr.lambda[i] * dcand
				if !finite(prevV[i]) {
					return empty, fmt.Errorf("coupled recurrent adjoint overflow at step %d neuron %d", t, i)
				}
			}
		}
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		activityRows, err := reverse.BackwardStep(ctx, t, modGradient)
		if err != nil {
			return empty, err
		}
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		for _, activity := range activityRows {
			if activity.Step < -1 || activity.Step >= t {
				return empty, fmt.Errorf("invalid activity gradient step %d at step %d", activity.Step, t)
			}
			if err := vector(activity.Values, n, "activity gradient"); err != nil {
				return empty, fmt.Errorf("step %d: %w", t, err)
			}
		}
		for _, activity := range activityRows {
			if activity.Step == -1 {
				if start != 0 {
					continue
				}
				for i, value := range activity.Values {
					if m.spiking(i) {
						continue
					}
					if !finite(gy[0][i] + value) {
						return empty, fmt.Errorf("activity adjoint overflow at step %d target -1 neuron %d", t, i)
					}
					gy[0][i] += value
				}
				continue
			}
			if activity.Step < start {
				continue
			}
			target := activity.Step + 1
			for i, value := range activity.Values {
				if !finite(gy[target][i] + value) {
					return empty, fmt.Errorf("activity adjoint overflow at step %d target %d neuron %d", t, activity.Step, i)
				}
				gy[target][i] += value
			}
		}
		for e, s := range m.config.Sources {
			if e%4096 == 0 {
				if err := ctx.Err(); err != nil {
					return empty, err
				}
			}
			past := 0
			if m.config.Delays[e] < t {
				past = t - m.config.Delays[e]
			}
			driveGradient := g.Inputs[t][m.config.Targets[e]]
			weightGradient := driveGradient * tr.out[past][s]
			if !finite(weightGradient) || !finite(g.Weights[e]+weightGradient) {
				return empty, fmt.Errorf("coupled weight adjoint overflow at step %d edge %d", t, e)
			}
			g.Weights[e] += weightGradient
			if past > start || start == 0 {
				activityGradient := driveGradient * tr.parameters.Weights[e]
				if !finite(activityGradient) || !finite(gy[past][s]+activityGradient) {
					return empty, fmt.Errorf("coupled activity adjoint overflow at step %d edge %d", t, e)
				}
				gy[past][s] += activityGradient
			}
		}
		gv, ga = prevV, prevA
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	for i := range gv {
		g.Initial[i] = gv[i]
		if !m.spiking(i) {
			initialGradient := gy[0][i] * m.act.derivative(tr.voltage[0][i])
			if !finite(initialGradient) || !finite(g.Initial[i]+initialGradient) {
				return empty, fmt.Errorf("coupled initial adjoint overflow at neuron %d", i)
			}
			g.Initial[i] += initialGradient
		}
	}
	for _, values := range [][]float64{g.Weights, g.Bias, g.LogTau, g.ThetaRaw, g.Initial} {
		if err := vector(values, len(values), "gradient"); err != nil {
			return empty, err
		}
	}
	for _, row := range g.Inputs {
		if err := vector(row, n, "input gradient"); err != nil {
			return empty, err
		}
	}
	return g, nil
}
