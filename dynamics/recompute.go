package dynamics

import (
	"context"
	"fmt"
	"math"
)

// RecomputeStats reports what the last call kept, so tests and evidence can
// show the saving without measuring the heap. FullHistoryRows is the number of
// voltage rows a whole-history forward would produce (len(inputs)+1).
// CheckpointRows is the total number of voltage rows saved across all
// segment-start checkpoints. MaxSegmentRows is the largest number of voltage
// rows held at once while any single segment is recomputed: that segment's
// checkpoint rows plus the rows newly produced for it.
type RecomputeStats struct {
	Steps, Segment, Checkpoints, CheckpointRows, MaxSegmentRows, FullHistoryRows int
}

// BackwardRecompute returns exactly what Backward(Forward(p, initial, inputs), upstream, window) returns, bit for bit, while
// keeping only checkpoints instead of the whole forward history. It runs the forward once and stores, at the start of every
// segment of `segment` steps, the voltages of the maxDelay+1 most recent rows (maxDelay = the largest edge delay, 0 when
// there are no delays), which is everything a segment needs to restart: outputs are activate(voltage). The backward then
// walks the segments from last to first; for each it re-runs the forward of that segment from its checkpoint (the same
// floating-point operations in the same order as Forward, so the recomputed rows are bit-identical), and runs the backward
// loop of Backward over the segment's rows with the adjoints carried from the later segment: gv, and the output adjoints gy
// of the rows [start−maxDelay, start] that later steps reached through delays. Parameter gradients are accumulated in the
// same global order as Backward (t descending, edges in declared order), so the sums are bit-identical.
// segment ≤ 0 is an error; segment ≥ len(inputs) is one segment (the whole history).
// Peak retained history is O((T/segment)·(maxDelay+1)·N + (segment+maxDelay+1)·N) instead of O(T·N).
func (m *Continuous) BackwardRecompute(ctx context.Context, p Parameters, initial []float64, inputs, upstream [][]float64, window, segment int) (Gradient, error) {
	g, _, err := m.BackwardRecomputeStats(ctx, p, initial, inputs, upstream, window, segment)
	return g, err
}

// BackwardRecomputeStats is BackwardRecompute and additionally reports what the
// recompute kept in a RecomputeStats (see RecomputeStats for the fields).
func (m *Continuous) BackwardRecomputeStats(ctx context.Context, p Parameters, initial []float64, inputs, upstream [][]float64, window, segment int) (Gradient, RecomputeStats, error) {
	var empty Gradient
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
	if err := vector(p.LogTau, n, "log_tau"); err != nil {
		return empty, stats, err
	}
	if steps > int(^uint(0)>>1)/n/3-1 {
		return empty, stats, fmt.Errorf("sequence shape overflows")
	}
	lambda := make([]float64, n)
	alpha := make([]float64, n)
	for i, raw := range p.LogTau {
		tau := math.Exp(raw)
		if !finite(tau) || tau <= 0 {
			return empty, stats, fmt.Errorf("tau[%d] is not representable and positive", i)
		}
		lambda[i] = math.Exp(-m.config.DT / tau)
		// Avoid cancellation in 1-exp(-dt/tau) for small positive dt/tau.
		alpha[i] = -math.Expm1(-m.config.DT / tau)
	}
	for t, in := range inputs {
		if err := ctx.Err(); err != nil {
			return empty, stats, err
		}
		// Validate inline without building an input[%d] name string on the
		// success path, mirroring Forward.
		if len(in) != n {
			return empty, stats, fmt.Errorf("input[%d] length %d, want %d", t, len(in), n)
		}
		for i, x := range in {
			if !finite(x) {
				return empty, stats, fmt.Errorf("input[%d][%d] is non-finite", t, i)
			}
		}
	}
	for _, row := range upstream {
		if err := vector(row, n, "upstream"); err != nil {
			return empty, stats, err
		}
	}
	maxDelay := m.maxEdgeDelay()
	// Forward once, keeping only a checkpoint per segment start. checkpoint[k]
	// restarts segment k (steps [k*segment, min((k+1)*segment, steps))) from
	// the voltage rows [max(0, k*segment-maxDelay), k*segment]; the first
	// checkpoint is just the initial voltage row.
	checkpoints := []recomputeCheckpoint{{lo: 0, rows: [][]float64{append([]float64(nil), initial...)}}}
	for s := 0; s < steps; {
		end := min(s+segment, steps)
		rows, err := m.recomputeSegmentForward(ctx, p, inputs, checkpoints[len(checkpoints)-1], s, end, lambda, alpha)
		if err != nil {
			return empty, stats, err
		}
		if end < steps {
			cpLo := max(0, end-maxDelay)
			cpRows := make([][]float64, end-cpLo+1)
			base := checkpoints[len(checkpoints)-1].lo
			for r := cpLo; r <= end; r++ {
				cpRows[r-cpLo] = append([]float64(nil), rows[r-base]...)
			}
			checkpoints = append(checkpoints, recomputeCheckpoint{lo: cpLo, rows: cpRows})
		}
		s = end
	}
	stats = RecomputeStats{Steps: steps, Segment: segment, Checkpoints: len(checkpoints), FullHistoryRows: steps + 1}
	for k, cp := range checkpoints {
		stats.CheckpointRows += len(cp.rows)
		if w := len(cp.rows) + (min(k*segment+segment, steps) - k*segment); w > stats.MaxSegmentRows {
			stats.MaxSegmentRows = w
		}
	}
	g := Gradient{
		Weights: make([]float64, len(p.Weights)),
		Bias:    make([]float64, n),
		LogTau:  make([]float64, n),
		Inputs:  make([][]float64, steps),
		Initial: make([]float64, n),
	}
	// Walk the segments from last to first. gv and carryRows are the adjoints
	// collected from all steps >= the next segment's start; carryRows holds the
	// output adjoints of the rows [carryLo, nextStart] reached by later steps
	// through delays.
	var gv []float64
	var carryLo int
	var carryRows [][]float64
	for k := len(checkpoints) - 1; k >= 0; k-- {
		start := k * segment
		end := min(start+segment, steps)
		rows, err := m.recomputeSegmentForward(ctx, p, inputs, checkpoints[k], start, end, lambda, alpha)
		if err != nil {
			return empty, stats, err
		}
		lo := checkpoints[k].lo
		gySeg := make([][]float64, end-lo+1)
		for r := lo; r <= end; r++ {
			j := r - lo
			if carryRows != nil && r >= carryLo {
				gySeg[j] = append([]float64(nil), carryRows[r-carryLo]...)
			} else if r == 0 {
				gySeg[j] = make([]float64, n)
			} else {
				gySeg[j] = append([]float64(nil), upstream[r-1]...)
			}
		}
		if gv == nil {
			gv = make([]float64, n)
		}
		for t := end - 1; t >= start; t-- {
			if err := ctx.Err(); err != nil {
				return empty, stats, err
			}
			wstart := 0
			if window > 0 {
				wstart = t / window * window
			}
			prev := make([]float64, n)
			g.Inputs[t] = make([]float64, n)
			drive := m.driveRow(p, inputs, rows, lo, t)
			for i := 0; i < n; i++ {
				dv := gv[i] + gySeg[t+1-lo][i]*m.derivative(rows[t+1-lo][i])
				g.Inputs[t][i] = alpha[i] * dv
				g.Bias[i] += g.Inputs[t][i]
				// d exp(-dt/exp(log_tau)) / d log_tau = lambda*dt/tau.
				dlambda := lambda[i] * (m.config.DT / math.Exp(p.LogTau[i]))
				// Underflowed lambda has zero derivative, even if dt/tau overflowed.
				if lambda[i] == 0 {
					dlambda = 0
				}
				g.LogTau[i] += dv * (rows[t-lo][i] - drive[i]) * dlambda
				if t > wstart || wstart == 0 {
					prev[i] = lambda[i] * dv
				}
			}
			for e, src := range m.config.Sources {
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
				g.Weights[e] += driveGrad * m.activate(rows[past-lo][src])
				if past > wstart || wstart == 0 {
					gySeg[past-lo][src] += driveGrad * p.Weights[e]
				}
			}
			gv = prev
		}
		outRows := make([][]float64, start-lo+1)
		for r := lo; r <= start; r++ {
			outRows[r-lo] = append([]float64(nil), gySeg[r-lo]...)
		}
		carryRows = outRows
		carryLo = lo
		if k == 0 {
			for i := range gv {
				g.Initial[i] = gv[i] + gySeg[0][i]*m.derivative(rows[0][i])
			}
		}
	}
	for _, v := range [][]float64{g.Weights, g.Bias, g.LogTau, g.Initial} {
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

// maxEdgeDelay returns the largest declared delay, 0 when the graph has no
// edges or delays.
func (m *Continuous) maxEdgeDelay() int {
	maxDelay := 0
	for _, d := range m.config.Delays {
		if d > maxDelay {
			maxDelay = d
		}
	}
	return maxDelay
}

// recomputeCheckpoint holds the voltage rows a segment needs to restart from:
// rows[0] = voltage[lo] through rows[len(rows)-1] = voltage[start], where
// start is the segment's first step and lo = max(0, start-maxDelay).
type recomputeCheckpoint struct {
	lo   int
	rows [][]float64
}

// recomputeSegmentForward re-runs Forward's per-step loop for the steps
// [start, end) from the checkpoint rows and returns the voltage rows
// [checkpoint.lo, end]. The checkpoint rows are shared (read only); the rows
// newly produced for the steps are fresh buffers. Every floating-point
// operation mirrors Forward's reference pass in the same order, so the returned
// rows are bit-identical to a full Forward run.
func (m *Continuous) recomputeSegmentForward(ctx context.Context, p Parameters, inputs [][]float64, cp recomputeCheckpoint, start, end int, lambda, alpha []float64) ([][]float64, error) {
	rows := make([][]float64, 0, end-cp.lo+1)
	rows = append(rows, cp.rows...)
	for t := start; t < end; t++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		drive := m.driveRow(p, inputs, rows, cp.lo, t)
		vnext := make([]float64, m.config.Nodes)
		for i := range drive {
			vnext[i] = lambda[i]*rows[t-cp.lo][i] + alpha[i]*drive[i]
			if !finite(drive[i]) || !finite(vnext[i]) {
				return nil, fmt.Errorf("non-finite state at step %d neuron %d", t, i)
			}
		}
		rows = append(rows, vnext)
	}
	return rows, nil
}

// driveRow recomputes step t's drive exactly as Forward computed it: a copy of
// inputs[t], each edge's weight times the activated delayed output added in
// declared edge order, then the bias. rows holds the recomputed voltage
// history covering [lo, t]; output[max(0, t-delay)] is
// activate(rows[max(0, t-delay)-lo]).
func (m *Continuous) driveRow(p Parameters, inputs [][]float64, rows [][]float64, lo, t int) []float64 {
	drive := append([]float64(nil), inputs[t]...)
	for e, src := range m.config.Sources {
		past := 0
		if m.config.Delays[e] < t {
			past = t - m.config.Delays[e]
		}
		drive[m.config.Targets[e]] += p.Weights[e] * m.activate(rows[past-lo][src])
	}
	for i := range drive {
		drive[i] += p.Bias[i]
	}
	return drive
}
