package dynamics

import (
	"context"
	"fmt"
	"math"
)

// BackwardRecompute returns exactly what Backward(Forward(p, initial, inputs), upstream, window) returns, bit for bit, while
// keeping only one lifCheckpoint per segment start instead of the whole forward history.
func (m *LIF) BackwardRecompute(ctx context.Context, p LIFParameters, initial []float64, inputs, upstream [][]float64, window, segment int) (LIFGradient, error) {
	g, _, err := m.BackwardRecomputeStats(ctx, p, initial, inputs, upstream, window, segment)
	return g, err
}

// BackwardRecomputeStats is BackwardRecompute plus the rows it kept: FullHistoryRows = T+1; CheckpointRows = the sum of
// len(cp.Syn) over all checkpoints; MaxSegmentRows = the largest len(cp.Syn) + (end − start) over the segments.
func (m *LIF) BackwardRecomputeStats(ctx context.Context, p LIFParameters, initial []float64, inputs, upstream [][]float64, window, segment int) (LIFGradient, RecomputeStats, error) {
	var empty LIFGradient
	var stats RecomputeStats
	if ctx == nil {
		return empty, stats, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return empty, stats, err
	}
	if m == nil {
		return empty, stats, fmt.Errorf("nil model")
	}
	steps := len(inputs)
	if steps == 0 {
		return empty, stats, fmt.Errorf("empty sequence")
	}
	n := m.config.Nodes
	if n <= 0 {
		return empty, stats, fmt.Errorf("uninitialized model")
	}
	if segment <= 0 {
		return empty, stats, fmt.Errorf("recompute segment must be positive")
	}
	if window < 0 || len(upstream) != steps {
		return empty, stats, fmt.Errorf("invalid backward window or sequence length")
	}
	if err := vector(initial, n, "initial voltage"); err != nil {
		return empty, stats, err
	}
	if err := vector(p.Weights, len(m.config.Sources), "weights"); err != nil {
		return empty, stats, err
	}
	if err := vector(p.Bias, n, "bias"); err != nil {
		return empty, stats, err
	}
	lambda, alpha, base, slope, err := m.lifConstants(p)
	if err != nil {
		return empty, stats, err
	}
	for t, in := range inputs {
		if err := ctx.Err(); err != nil {
			return empty, stats, err
		}
		if err := vector(in, n, fmt.Sprintf("input[%d]", t)); err != nil {
			return empty, stats, err
		}
	}
	for _, row := range upstream {
		if err := vector(row, n, "upstream"); err != nil {
			return empty, stats, err
		}
	}
	maxDelay := 0
	for _, d := range m.config.Delays {
		if d > maxDelay {
			maxDelay = d
		}
	}
	// Forward once, keeping only a checkpoint per segment start. cps[k] restarts
	// segment k (steps [k*segment, min((k+1)*segment, steps))) from the state of
	// row k*segment plus the syn rows max(0, k*segment-maxDelay)..k*segment.
	cps := []lifCheckpoint{m.lifInitialCheckpoint(initial)}
	for s := 0; s < steps; {
		end := min(s+segment, steps)
		seg, err := m.lifRunSegment(ctx, p, inputs, cps[len(cps)-1], end, lambda, alpha, base)
		if err != nil {
			return empty, stats, err
		}
		if end < steps {
			cps = append(cps, seg.checkpointAt(end, maxDelay))
		}
		s = end
	}
	stats = RecomputeStats{Steps: steps, Segment: segment, Checkpoints: len(cps), FullHistoryRows: steps + 1}
	for k, cp := range cps {
		stats.CheckpointRows += len(cp.Syn)
		if w := len(cp.Syn) + (min(k*segment+segment, steps) - k*segment); w > stats.MaxSegmentRows {
			stats.MaxSegmentRows = w
		}
	}
	// The output adjoints gx stay complete for the whole backward: the rows a
	// segment touches are exactly the rows later segments already accumulated.
	// Only the voltage and adaptation adjoints cross segment boundaries.
	gx := make([][]float64, steps+1)
	for t := range gx {
		gx[t] = make([]float64, n)
	}
	for t, row := range upstream {
		copy(gx[t+1], row)
	}
	g := LIFGradient{
		Weights:  make([]float64, len(p.Weights)),
		Bias:     make([]float64, n),
		LogTau:   make([]float64, n),
		ThetaRaw: make([]float64, n),
		Inputs:   make([][]float64, steps),
		Initial:  make([]float64, n),
	}
	adapting := m.config.Adaptation.Enabled
	gv, ga := make([]float64, n), make([]float64, n)
	for k := len(cps) - 1; k >= 0; k-- {
		start := k * segment
		end := min(start+segment, steps)
		seg, err := m.lifRunSegment(ctx, p, inputs, cps[k], end, lambda, alpha, base)
		if err != nil {
			return empty, stats, err
		}
		// The body below is Backward's per-step loop with every trace read
		// replaced by the segment accessors; the floating-point expressions are
		// unchanged so the recomputed gradient is bit-identical.
		for t := end - 1; t >= start; t-- {
			if err := ctx.Err(); err != nil {
				return empty, stats, err
			}
			wstart := 0
			if window > 0 {
				wstart = t / window * window
			}
			keep := t > wstart || wstart == 0
			prevV, prevA := make([]float64, n), make([]float64, n)
			g.Inputs[t] = make([]float64, n)
			cand, drive, reset, dspike := seg.Step(t)
			for i := 0; i < n; i++ {
				// The event feeds the synaptic trace and the adaptation.
				dsp := gx[t+1][i]
				if adapting {
					dsp += ga[i] * m.config.Adaptation.Beta
				}
				if m.smooth {
					// The smooth reference mixes v_cand and v_reset continuously,
					// so its reset coefficient is a real dependency. The product
					// model keeps the declared detached reset instead.
					dsp += gv[i] * (m.config.VReset - cand[i])
				}
				if keep {
					gx[t][i] += m.kappa * gx[t+1][i]
					if adapting {
						prevA[i] = m.rho * ga[i]
					}
				}
				du := dsp * dspike[i]
				dcand := gv[i]*reset[i] + du
				// theta_effective = theta_base + a(t) + h(t), so the threshold
				// gradient is -du on both the bounded transform and the adaptation
				// state. The slow stabiliser offset h is a declared constant of the
				// reverse pass, exactly like the adaptation inside one step: it
				// carries no gradient and opens no path back to the events that
				// raised it.
				if adapting && keep {
					prevA[i] -= du
				}
				g.ThetaRaw[i] -= du * slope[i]
				g.Inputs[t][i] = alpha[i] * dcand
				g.Bias[i] += g.Inputs[t][i]
				// d exp(-dt/exp(log_tau)) / d log_tau = lambda*dt/tau.
				dlambda := lambda[i] * (m.config.DT / math.Exp(p.LogTau[i]))
				// Underflowed lambda has zero derivative, even if dt/tau overflowed.
				if lambda[i] == 0 {
					dlambda = 0
				}
				g.LogTau[i] += dcand * (seg.VoltageRow(t)[i] - drive[i]) * dlambda
				if keep {
					prevV[i] = lambda[i] * dcand
				}
			}
			for e, s := range m.config.Sources {
				if e%4096 == 0 {
					if err := ctx.Err(); err != nil {
						return empty, stats, err
					}
				}
				past := 0
				if m.config.Delays[e] < t {
					past = t - m.config.Delays[e]
				}
				driveGrad := g.Inputs[t][m.config.Targets[e]]
				g.Weights[e] += driveGrad * seg.SynRow(past)[s]
				if past > wstart || wstart == 0 {
					gx[past][s] += driveGrad * p.Weights[e]
				}
			}
			gv, ga = prevV, prevA
		}
	}
	// The synaptic trace, adaptation and refractory counters start at zero and
	// are not caller supplied, so only the voltage carries an initial gradient.
	copy(g.Initial, gv)
	for _, v := range [][]float64{g.Weights, g.Bias, g.LogTau, g.ThetaRaw, g.Initial} {
		if err := vector(v, len(v), "gradient"); err != nil {
			return empty, stats, err
		}
	}
	for _, v := range g.Inputs {
		if err := vector(v, len(v), "input gradient"); err != nil {
			return empty, stats, err
		}
	}
	return g, stats, nil
}
