package dynamics

import (
	"context"
	"fmt"
)

// lifCoupledRecomputeCheckpoint owns the neural state at one segment start
// and the chemical restart that recreates the source state at that boundary.
// Syn rows are borrowed from the parent trace's immutable output history.
type lifCoupledRecomputeCheckpoint struct {
	start   int
	neural  lifCheckpoint
	restart ModulationRestart
}

// LIFCoupledRecomputeTrace keeps owned inputs, parameters, complete synaptic
// outputs and one joint checkpoint per segment start. It deliberately does not
// retain the full voltage, candidate, drive, event or modulation tape.
type LIFCoupledRecomputeTrace struct {
	model       *LIF
	parameters  LIFParameters
	inputs      [][]float64
	syn         [][]float64
	lambda      []float64
	alpha       []float64
	thetaSlope  []float64
	sequence    RecomputableModulationSequence
	checkpoints []lifCoupledRecomputeCheckpoint
	final       []float64
	segment     int
	stats       RecomputeStats
}

func (tr *LIFCoupledRecomputeTrace) Outputs() [][]float64 {
	if tr == nil || len(tr.syn) <= 1 {
		return nil
	}
	return cloneRows(tr.syn[1:])
}

func (tr *LIFCoupledRecomputeTrace) FinalVoltage() []float64 {
	if tr == nil {
		return nil
	}
	return append([]float64(nil), tr.final...)
}

func (tr *LIFCoupledRecomputeTrace) Stats() RecomputeStats {
	if tr == nil {
		return RecomputeStats{}
	}
	return tr.stats
}

// ForwardCoupledRecompute is the bounded-neural-history version of
// ForwardCoupled for a non-nil restartable sequence and positive segment size.
// It preserves hard events, detached resets, the declared surrogate,
// threshold-floor detachment and slow-stability state. Failure returns no
// trace or external update.
func (m *LIF) ForwardCoupledRecompute(ctx context.Context, p LIFParameters, initial []float64, inputs [][]float64, sequence RecomputableModulationSequence, segment int) (*LIFCoupledRecomputeTrace, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("nil model")
	}
	if sequence == nil {
		return nil, fmt.Errorf("nil modulation sequence")
	}
	n := m.config.Nodes
	if n <= 0 {
		return nil, fmt.Errorf("uninitialized model")
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("empty sequence")
	}
	if segment <= 0 {
		return nil, fmt.Errorf("recompute segment must be positive")
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
	lambda, alpha, _, thetaSlope, err := m.lifConstants(p)
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	ownedInputs := cloneRows(inputs)
	ownedSyn := make([][]float64, len(ownedInputs)+1)
	initialCheckpoint := m.lifInitialCheckpoint(initial)
	ownedSyn[0] = initialCheckpoint.Syn[0]
	initialCheckpoint.Syn[0] = ownedSyn[0]
	trace := &LIFCoupledRecomputeTrace{
		model:      m,
		parameters: cloneLIFParameters(p),
		inputs:     ownedInputs,
		syn:        ownedSyn,
		lambda:     lambda,
		alpha:      alpha,
		thetaSlope: thetaSlope,
		sequence:   sequence,
		segment:    segment,
		stats: RecomputeStats{
			Steps:           len(ownedInputs),
			Segment:         segment,
			FullHistoryRows: len(ownedInputs) + 1,
		},
	}
	maxDelay := 0
	for _, delay := range m.config.Delays {
		if delay > maxDelay {
			maxDelay = delay
		}
	}
	checkpoint := initialCheckpoint
	voltage := append([]float64(nil), initial...)
	for start := 0; start < len(ownedInputs); {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		restart, err := sequence.Checkpoint(ctx, start)
		if err != nil {
			return nil, err
		}
		if restart == nil {
			return nil, fmt.Errorf("nil modulation restart at step %d", start)
		}
		trace.checkpoints = append(trace.checkpoints, lifCoupledRecomputeCheckpoint{
			start:   start,
			neural:  checkpoint,
			restart: restart,
		})

		width := segment
		if remaining := len(ownedInputs) - start; width > remaining {
			width = remaining
		}
		end := start + width
		segmentSequence := offsetModulationSequence{sequence: sequence, start: start}
		segmentTrace, err := m.forwardLIFCoupled(ctx, p, checkpoint.Voltage, ownedInputs[start:end], segmentSequence, &checkpoint)
		if err != nil {
			return nil, err
		}
		if segmentTrace == nil || len(segmentTrace.syn) != end-start+1 || len(segmentTrace.voltage) != end-start+1 {
			return nil, fmt.Errorf("invalid coupled segment trace at step %d", start)
		}
		for local := 0; local < end-start; local++ {
			trace.syn[start+local+1] = append([]float64(nil), segmentTrace.syn[local+1]...)
		}
		voltage = append([]float64(nil), segmentTrace.voltage[len(segmentTrace.voltage)-1]...)
		if end < len(ownedInputs) {
			lo := max(0, end-maxDelay)
			synRows := make([][]float64, end-lo+1)
			for row := lo; row <= end; row++ {
				synRows[row-lo] = trace.syn[row]
			}
			last := len(segmentTrace.voltage) - 1
			checkpoint = lifCheckpoint{
				S:       end,
				Lo:      lo,
				Syn:     synRows,
				Voltage: append([]float64(nil), segmentTrace.voltage[last]...),
				Adapt:   append([]float64(nil), segmentTrace.adapt[last]...),
				Refract: append([]int(nil), segmentTrace.refract[last]...),
				Rate:    append([]float64(nil), segmentTrace.rate[last]...),
				Homeo:   append([]float64(nil), segmentTrace.homeo[last]...),
			}
		}
		start = end
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	trace.final = append([]float64(nil), voltage...)
	trace.stats.Checkpoints = len(trace.checkpoints)
	trace.stats.CheckpointRows = len(trace.checkpoints)
	for start := 0; start < len(ownedInputs); {
		width := segment
		if remaining := len(ownedInputs) - start; width > remaining {
			width = remaining
		}
		if rows := width + 1; rows > trace.stats.MaxSegmentRows {
			trace.stats.MaxSegmentRows = rows
		}
		start += width
	}
	return trace, nil
}

// BackwardCoupledRecompute preserves ForwardCoupled/Backward gradients,
// including activity-dependent chemistry, readout-expression seeds and global
// truncation boundaries, while replaying private segment-start checkpoints.
func (m *LIF) BackwardCoupledRecompute(ctx context.Context, tr *LIFCoupledRecomputeTrace, upstream [][]float64, window int) (LIFGradient, error) {
	var empty LIFGradient
	if ctx == nil {
		return empty, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if m == nil {
		return empty, fmt.Errorf("nil model")
	}
	if tr == nil || tr.model != m {
		return empty, fmt.Errorf("trace belongs to a different model")
	}
	if window < 0 {
		return empty, fmt.Errorf("invalid backward window or sequence length")
	}
	if err := validateLIFCoupledRecomputeTrace(m, tr); err != nil {
		return empty, err
	}
	steps := len(tr.inputs)
	if len(upstream) != steps {
		return empty, fmt.Errorf("invalid backward window or sequence length")
	}
	metadata := &LIFTrace{
		model:      m,
		parameters: cloneLIFParameters(tr.parameters),
		syn:        tr.syn,
		lambda:     tr.lambda,
		alpha:      tr.alpha,
		thetaSlope: tr.thetaSlope,
		coupled:    &lifCoupledTrace{sequence: tr.sequence},
	}

	currentStart := -1
	var current *LIFTrace
	load := func(loadCtx context.Context, step int) (*LIFTrace, int, error) {
		if step < 0 || step >= steps {
			return nil, 0, fmt.Errorf("invalid replay step %d", step)
		}
		start := step / tr.segment * tr.segment
		if current == nil || currentStart != start {
			checkpointIndex := start / tr.segment
			if checkpointIndex < 0 || checkpointIndex >= len(tr.checkpoints) || tr.checkpoints[checkpointIndex].start != start {
				return nil, 0, fmt.Errorf("missing coupled checkpoint at step %d", start)
			}
			checkpoint := tr.checkpoints[checkpointIndex]
			if checkpoint.restart == nil {
				return nil, 0, fmt.Errorf("nil modulation restart at step %d", start)
			}
			replaySequence, err := checkpoint.restart(loadCtx)
			if err != nil {
				return nil, 0, err
			}
			if replaySequence == nil {
				return nil, 0, fmt.Errorf("nil modulation replay at step %d", start)
			}
			width := tr.segment
			if remaining := steps - start; width > remaining {
				width = remaining
			}
			end := start + width
			current, err = m.forwardLIFCoupled(loadCtx, tr.parameters, checkpoint.neural.Voltage, tr.inputs[start:end], replaySequence, &checkpoint.neural)
			if err != nil {
				return nil, 0, err
			}
			if current == nil || len(current.syn) != end-start+1 || len(current.voltage) != end-start+1 {
				return nil, 0, fmt.Errorf("invalid replay trace at step %d", start)
			}
			currentStart = start
		}
		return current, step - currentStart, nil
	}
	return m.backwardLIFCoupledTape(ctx, metadata, upstream, window, load)
}

func validateLIFCoupledRecomputeTrace(m *LIF, tr *LIFCoupledRecomputeTrace) error {
	if tr.sequence == nil || len(tr.inputs) == 0 || tr.segment <= 0 {
		return fmt.Errorf("invalid coupled recompute trace")
	}
	steps, n := len(tr.inputs), m.config.Nodes
	if len(tr.syn) != steps+1 || len(tr.final) != n || len(tr.lambda) != n || len(tr.alpha) != n || len(tr.thetaSlope) != n {
		return fmt.Errorf("invalid coupled recompute trace")
	}
	if len(tr.parameters.Weights) != len(m.config.Sources) || len(tr.parameters.Bias) != n || len(tr.parameters.LogTau) != n || len(tr.parameters.ThetaRaw) != n {
		return fmt.Errorf("invalid coupled recompute trace parameters")
	}
	if err := vector(tr.parameters.Weights, len(m.config.Sources), "coupled weights"); err != nil {
		return err
	}
	if err := vector(tr.parameters.Bias, n, "coupled bias"); err != nil {
		return err
	}
	if err := vector(tr.parameters.LogTau, n, "coupled log_tau"); err != nil {
		return err
	}
	if err := vector(tr.parameters.ThetaRaw, n, "coupled theta_raw"); err != nil {
		return err
	}
	if err := vector(tr.final, n, "coupled final voltage"); err != nil {
		return err
	}
	if err := vector(tr.lambda, n, "coupled lambda"); err != nil {
		return err
	}
	if err := vector(tr.alpha, n, "coupled alpha"); err != nil {
		return err
	}
	if err := vector(tr.thetaSlope, n, "coupled theta slope"); err != nil {
		return err
	}
	for t, row := range tr.inputs {
		if err := vector(row, n, "coupled input"); err != nil {
			return fmt.Errorf("input[%d]: %w", t, err)
		}
	}
	for t, row := range tr.syn {
		if err := vector(row, n, "coupled output"); err != nil {
			return fmt.Errorf("output[%d]: %w", t, err)
		}
	}
	maxDelay := 0
	for _, delay := range m.config.Delays {
		if delay > maxDelay {
			maxDelay = delay
		}
	}
	if len(tr.checkpoints) == 0 || tr.checkpoints[0].start != 0 {
		return fmt.Errorf("invalid coupled recompute checkpoints")
	}
	want := 0
	for i, checkpoint := range tr.checkpoints {
		if checkpoint.start != want || checkpoint.start >= steps || checkpoint.restart == nil || checkpoint.neural.S != checkpoint.start {
			return fmt.Errorf("invalid coupled checkpoint %d", i)
		}
		if checkpoint.neural.Lo != max(0, checkpoint.start-maxDelay) || len(checkpoint.neural.Syn) != checkpoint.start-checkpoint.neural.Lo+1 {
			return fmt.Errorf("invalid coupled checkpoint %d synaptic history", i)
		}
		if err := vector(checkpoint.neural.Voltage, n, "checkpoint voltage"); err != nil {
			return fmt.Errorf("checkpoint[%d]: %w", i, err)
		}
		if err := vector(checkpoint.neural.Adapt, n, "checkpoint adaptation"); err != nil {
			return fmt.Errorf("checkpoint[%d]: %w", i, err)
		}
		if err := vector(checkpoint.neural.Rate, n, "checkpoint rate"); err != nil {
			return fmt.Errorf("checkpoint[%d]: %w", i, err)
		}
		if err := vector(checkpoint.neural.Homeo, n, "checkpoint homeostasis"); err != nil {
			return fmt.Errorf("checkpoint[%d]: %w", i, err)
		}
		if len(checkpoint.neural.Refract) != n {
			return fmt.Errorf("checkpoint[%d] refractory shape", i)
		}
		for row, values := range checkpoint.neural.Syn {
			if err := vector(values, n, "checkpoint synaptic output"); err != nil {
				return fmt.Errorf("checkpoint[%d] row %d: %w", i, row, err)
			}
		}
		for node, value := range checkpoint.neural.Refract {
			if value < 0 {
				return fmt.Errorf("checkpoint[%d] refractory[%d] is negative", i, node)
			}
		}
		want += tr.segment
		if want > steps {
			want = steps
		}
	}
	if want != steps {
		return fmt.Errorf("incomplete coupled checkpoints")
	}
	return nil
}
