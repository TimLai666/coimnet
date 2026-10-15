package dynamics

import (
	"context"
	"fmt"
	"math"
)

// backwardCoupledTape shares the established reverse arithmetic between full-tape
// and private segment replay. Metadata holds complete raw outputs and the
// original chemical observations; load supplies only this step's local tape.
// Global step, truncation and declared edge accumulation order are unchanged.
func (m *Continuous) backwardCoupledTape(ctx context.Context, tr *Trace, upstream [][]float64, window int, load func(context.Context, int) (*Trace, int, error)) (Gradient, error) {
	var empty Gradient
	steps, n := len(tr.output)-1, m.config.Nodes
	gy := make([][]float64, steps+1)
	for t := range gy {
		gy[t] = make([]float64, n)
	}
	for t, row := range upstream {
		if err := vector(row, n, "upstream"); err != nil {
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
	g := Gradient{Weights: make([]float64, len(tr.parameters.Weights)), Bias: make([]float64, n), LogTau: make([]float64, n), Inputs: make([][]float64, steps), Initial: make([]float64, n)}
	gv := make([]float64, n)
	for t := steps - 1; t >= 0; t-- {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		row, local, err := load(ctx, t)
		if err != nil {
			return empty, err
		}
		start := 0
		if window > 0 {
			start = t / window * window
		}
		prev := make([]float64, n)
		g.Inputs[t] = make([]float64, n)
		modGradient := ModulationGradient{Gain: make([]float64, n), Offset: make([]float64, n), Threshold: make([]float64, n)}
		for i := 0; i < n; i++ {
			dv := gv[i] + gy[t+1][i]*m.derivative(row.voltage[local+1][i])
			if !finite(dv) {
				return empty, fmt.Errorf("non-finite coupled voltage adjoint at step %d neuron %d", t, i)
			}
			dJ := tr.alpha[i] * dv
			if !finite(dJ) {
				return empty, fmt.Errorf("non-finite coupled drive adjoint at step %d neuron %d", t, i)
			}
			gain := row.coupled.gain[local][i]
			if !finite(gain) {
				return empty, fmt.Errorf("non-finite coupled gain at step %d neuron %d", t, i)
			}
			inputGradient := dJ
			if gain != 1 {
				inputGradient = dJ * gain
			}
			if !finite(inputGradient) {
				return empty, fmt.Errorf("non-finite coupled input adjoint at step %d neuron %d", t, i)
			}
			g.Inputs[t][i] = inputGradient
			// Round the current adjoint before adding it, matching the ordinary
			// path's stored Inputs row rather than fusing the two operations.
			g.Bias[i] += float64(dJ)
			if !finite(g.Bias[i]) {
				return empty, fmt.Errorf("coupled bias adjoint overflow at step %d neuron %d", t, i)
			}
			rawDrive := row.coupled.rawDrive[local][i]
			modGradient.Gain[i] = rawDrive * dJ
			modGradient.Offset[i] = dJ
			if !finite(modGradient.Gain[i]) || !finite(modGradient.Offset[i]) {
				return empty, fmt.Errorf("coupled modulation adjoint overflow at step %d neuron %d", t, i)
			}
			dlambda := tr.lambda[i] * (m.config.DT / math.Exp(tr.parameters.LogTau[i]))
			if tr.lambda[i] == 0 {
				dlambda = 0
			}
			logTauGradient := dv * (row.voltage[local][i] - row.drive[local][i]) * dlambda
			if !finite(logTauGradient) {
				recovered := membraneTauGradient(dv, row.voltage[local][i], row.drive[local][i], dlambda)
				if !finite(recovered) {
					return empty, fmt.Errorf("coupled log_tau adjoint overflow at step %d neuron %d", t, i)
				}
				g.LogTau[i] += recovered
			} else {
				g.LogTau[i] += logTauGradient
			}
			if !finite(g.LogTau[i]) {
				return empty, fmt.Errorf("coupled log_tau adjoint overflow at step %d neuron %d", t, i)
			}
			if t > start || start == 0 {
				prev[i] = tr.lambda[i] * dv
				if !finite(prev[i]) {
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
			target := activity.Step
			if target == -1 {
				if start != 0 || tr.history != nil {
					continue
				}
				target = 0
			} else if target < start {
				continue
			} else {
				target++
			}
			for i, value := range activity.Values {
				gy[target][i] += value
				if !finite(gy[target][i]) {
					return empty, fmt.Errorf("activity adjoint overflow at step %d target %d neuron %d", t, activity.Step, i)
				}
			}
		}
		r := forwardReader{output: tr.output, sources: m.config.Sources, delays: m.config.Delays, t: t, history: tr.history, startSteps: tr.startSteps}
		for e, s := range m.config.Sources {
			if e%4096 == 0 {
				if err := ctx.Err(); err != nil {
					return empty, err
				}
			}
			driveGradient := g.Inputs[t][m.config.Targets[e]]
			weightGradient := driveGradient * r.at(e)
			if !finite(weightGradient) {
				return empty, fmt.Errorf("coupled weight adjoint overflow at step %d edge %d", t, e)
			}
			g.Weights[e] += weightGradient
			if !finite(g.Weights[e]) {
				return empty, fmt.Errorf("coupled weight adjoint overflow at step %d edge %d", t, e)
			}
			past := 0
			if m.config.Delays[e] < t {
				past = t - m.config.Delays[e]
			}
			if (tr.history == nil || past > 0) && (past > start || start == 0) {
				activityGradient := driveGradient * tr.parameters.Weights[e]
				if !finite(activityGradient) {
					return empty, fmt.Errorf("coupled activity adjoint overflow at step %d edge %d", t, e)
				}
				gy[past][s] += activityGradient
				if !finite(gy[past][s]) {
					return empty, fmt.Errorf("coupled activity adjoint overflow at step %d edge %d", t, e)
				}
			}
		}
		gv = prev
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if tr.history != nil {
		copy(g.Initial, gv)
	} else {
		for i := range gv {
			initialGradient := gv[i] + gy[0][i]*m.derivative(tr.voltage[0][i])
			if !finite(initialGradient) {
				return empty, fmt.Errorf("coupled initial adjoint overflow at neuron %d", i)
			}
			g.Initial[i] = initialGradient
		}
	}
	for _, values := range [][]float64{g.Weights, g.Bias, g.LogTau, g.Initial} {
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

// backwardLIFCoupledTape shares the established reverse arithmetic between full-tape
// and private segment replay. Metadata holds complete raw outputs and the
// original chemical observations; load supplies only this step's local tape.
// Global step, truncation and declared edge accumulation order are unchanged.
func (m *LIF) backwardLIFCoupledTape(ctx context.Context, tr *LIFTrace, upstream [][]float64, window int, load func(context.Context, int) (*LIFTrace, int, error)) (LIFGradient, error) {
	var empty LIFGradient
	steps, n := len(tr.syn)-1, m.config.Nodes
	gy := make([][]float64, steps+1)
	for t := range gy {
		gy[t] = make([]float64, n)
	}
	for t, row := range upstream {
		if err := vector(row, n, "upstream"); err != nil {
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
	g := LIFGradient{
		Weights: make([]float64, len(tr.parameters.Weights)), Bias: make([]float64, n),
		LogTau: make([]float64, n), ThetaRaw: make([]float64, n),
		Inputs: make([][]float64, steps), Initial: make([]float64, n),
	}
	adapting := m.config.Adaptation.Enabled
	gv, ga := make([]float64, n), make([]float64, n)
	for t := steps - 1; t >= 0; t-- {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		row, local, err := load(ctx, t)
		if err != nil {
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
			dspike := gy[t+1][i]
			if adapting {
				dspike += ga[i] * m.config.Adaptation.Beta
			}
			if m.smooth {
				dspike += gv[i] * (m.config.VReset - row.cand[local][i])
			}
			if keep {
				gx := m.kappa * gy[t+1][i]
				if !finite(gx) || !finite(gy[t][i]+gx) {
					return empty, fmt.Errorf("coupled synaptic adjoint overflow at step %d neuron %d", t, i)
				}
				gy[t][i] += gx
				if adapting {
					prevA[i] = m.rho * ga[i]
					if !finite(prevA[i]) {
						return empty, fmt.Errorf("coupled adaptation adjoint overflow at step %d neuron %d", t, i)
					}
				}
			}
			du := dspike * row.dspike[local][i]
			dcand := gv[i]*row.reset[local][i] + du
			if !finite(dspike) || !finite(du) || !finite(dcand) {
				return empty, fmt.Errorf("non-finite coupled event adjoint at step %d neuron %d", t, i)
			}
			if !row.coupled.thresholdFloor[local][i] {
				if adapting && keep {
					prevA[i] -= du
					if !finite(prevA[i]) {
						return empty, fmt.Errorf("coupled adaptation adjoint overflow at step %d neuron %d", t, i)
					}
				}
				thetaGradient := du * tr.thetaSlope[i]
				if !finite(thetaGradient) {
					return empty, fmt.Errorf("coupled theta adjoint overflow at step %d neuron %d", t, i)
				}
				g.ThetaRaw[i] -= thetaGradient
				modGradient.Threshold[i] = -du
			}
			dCurrent := tr.alpha[i] * dcand
			if !finite(dCurrent) {
				return empty, fmt.Errorf("coupled current adjoint overflow at step %d neuron %d", t, i)
			}
			gain := row.coupled.gain[local][i]
			inputGradient := dCurrent
			if gain != 1 {
				inputGradient = dCurrent * gain
			}
			if !finite(inputGradient) {
				return empty, fmt.Errorf("coupled input adjoint overflow at step %d neuron %d", t, i)
			}
			g.Inputs[t][i] = inputGradient
			// Match the ordinary path's separately stored current adjoint.
			g.Bias[i] += float64(dCurrent)
			if !finite(g.Bias[i]) {
				return empty, fmt.Errorf("coupled bias adjoint overflow at step %d neuron %d", t, i)
			}
			modGradient.Gain[i] = row.coupled.rawDrive[local][i] * dCurrent
			modGradient.Offset[i] = dCurrent
			if !finite(modGradient.Gain[i]) || !finite(modGradient.Offset[i]) {
				return empty, fmt.Errorf("coupled modulation adjoint overflow at step %d neuron %d", t, i)
			}
			dlambda := tr.lambda[i] * (m.config.DT / math.Exp(tr.parameters.LogTau[i]))
			if tr.lambda[i] == 0 {
				dlambda = 0
			}
			logTauGradient := dcand * (row.voltage[local][i] - row.drive[local][i]) * dlambda
			if !finite(logTauGradient) {
				recovered := membraneTauGradient(dcand, row.voltage[local][i], row.drive[local][i], dlambda)
				if !finite(recovered) {
					return empty, fmt.Errorf("coupled log_tau adjoint overflow at step %d neuron %d", t, i)
				}
				g.LogTau[i] += recovered
			} else {
				g.LogTau[i] += logTauGradient
			}
			if !finite(g.LogTau[i]) {
				return empty, fmt.Errorf("coupled log_tau adjoint overflow at step %d neuron %d", t, i)
			}
			if keep {
				prevV[i] = tr.lambda[i] * dcand
				if !finite(prevV[i]) {
					return empty, fmt.Errorf("coupled voltage adjoint overflow at step %d neuron %d", t, i)
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
			if activity.Step < 0 || activity.Step < start {
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
		reader := forwardReader{output: tr.syn, sources: m.config.Sources, delays: m.config.Delays, t: t}
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
			weightGradient := driveGradient * reader.at(e)
			if !finite(weightGradient) {
				return empty, fmt.Errorf("coupled weight adjoint overflow at step %d edge %d", t, e)
			}
			g.Weights[e] += weightGradient
			if !finite(g.Weights[e]) {
				return empty, fmt.Errorf("coupled weight adjoint overflow at step %d edge %d", t, e)
			}
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
	copy(g.Initial, gv)
	for _, v := range [][]float64{g.Weights, g.Bias, g.LogTau, g.ThetaRaw, g.Initial} {
		if err := vector(v, len(v), "gradient"); err != nil {
			return empty, err
		}
	}
	for _, v := range g.Inputs {
		if err := vector(v, n, "input gradient"); err != nil {
			return empty, err
		}
	}
	return g, nil
}
