package dynamics

import (
	"context"
	"fmt"
	"math"
)

// continuousCoupledCheckpoint keeps the single neural boundary row and the
// source restart needed to replay one coupled segment. The output history is
// kept on the parent trace and is copied into a private segment only while it
// is being replayed.
type continuousCoupledCheckpoint struct {
	start   int
	voltage []float64
	restart ModulationRestart
}

// ContinuousCoupledRecomputeTrace owns finite-sequence inputs, complete raw
// output history and joint neural/chemical segment checkpoints instead of the
// full voltage/drive history. RecomputeStats counts neural history only, not
// total process memory.
type ContinuousCoupledRecomputeTrace struct {
	model       *Continuous
	parameters  Parameters
	initial     []float64
	inputs      [][]float64
	output      [][]float64
	lambda      []float64
	alpha       []float64
	sequence    RecomputableModulationSequence
	checkpoints []continuousCoupledCheckpoint
	final       []float64
	segment     int
	stats       RecomputeStats
}

func (tr *ContinuousCoupledRecomputeTrace) Outputs() [][]float64 {
	if tr == nil || len(tr.output) <= 1 {
		return nil
	}
	return cloneRows(tr.output[1:])
}

func (tr *ContinuousCoupledRecomputeTrace) FinalVoltage() []float64 {
	if tr == nil {
		return nil
	}
	return append([]float64(nil), tr.final...)
}

func (tr *ContinuousCoupledRecomputeTrace) Stats() RecomputeStats {
	if tr == nil {
		return RecomputeStats{}
	}
	return tr.stats
}

// ForwardCoupledRecompute follows ForwardCoupled with positive segment steps.
// It stores owned joint checkpoints and requires a non-nil restartable source.
// A complete input episode is independent of persistent-state capacity limits.
// Cancellation, malformed input or checkpoint failure returns a nil trace.
func (m *Continuous) ForwardCoupledRecompute(ctx context.Context, p Parameters, initial []float64, inputs [][]float64, sequence RecomputableModulationSequence, segment int) (*ContinuousCoupledRecomputeTrace, error) {
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
	if len(inputs) > int(^uint(0)>>1)/n/3-1 {
		return nil, fmt.Errorf("sequence shape overflows")
	}
	lambda := make([]float64, n)
	alpha := make([]float64, n)
	for i, raw := range p.LogTau {
		tau := math.Exp(raw)
		if !finite(tau) || tau <= 0 {
			return nil, fmt.Errorf("tau[%d] is not representable and positive", i)
		}
		lambda[i] = math.Exp(-m.config.DT / tau)
		alpha[i] = -math.Expm1(-m.config.DT / tau)
	}
	for t, in := range inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(in) != n {
			return nil, fmt.Errorf("input[%d] length %d, want %d", t, len(in), n)
		}
		for i, x := range in {
			if !finite(x) {
				return nil, fmt.Errorf("input[%d][%d] is non-finite", t, i)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	ownedInputs := cloneRows(inputs)
	trace := &ContinuousCoupledRecomputeTrace{
		model:      m,
		parameters: cloneParameters(p),
		initial:    append([]float64(nil), initial...),
		inputs:     ownedInputs,
		output:     make([][]float64, len(ownedInputs)+1),
		lambda:     lambda,
		alpha:      alpha,
		sequence:   sequence,
		segment:    segment,
		stats: RecomputeStats{
			Steps:           len(ownedInputs),
			Segment:         segment,
			FullHistoryRows: len(ownedInputs) + 1,
		},
	}
	maxDelay := m.maxEdgeDelay()
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
		checkpoint := continuousCoupledCheckpoint{
			start:   start,
			voltage: append([]float64(nil), voltage...),
			restart: restart,
		}
		trace.checkpoints = append(trace.checkpoints, checkpoint)

		end := len(ownedInputs)
		if segment <= len(ownedInputs)-start {
			end = start + segment
		}
		var saved *State
		if start > 0 {
			lo := max(0, start-maxDelay)
			saved = &State{
				Voltage: append([]float64(nil), checkpoint.voltage...),
				History: trace.output[lo : start+1],
				Steps:   uint64(start),
			}
		}
		segmentSequence := offsetModulationSequence{sequence: sequence, start: start}
		segmentTrace, err := m.forwardSequence(ctx, p, checkpoint.voltage, ownedInputs[start:end], saved, segmentSequence)
		if err != nil {
			return nil, err
		}
		if segmentTrace == nil || len(segmentTrace.output) != end-start+1 || len(segmentTrace.voltage) != end-start+1 {
			return nil, fmt.Errorf("invalid coupled segment trace at step %d", start)
		}
		if start == 0 {
			trace.output[0] = append([]float64(nil), segmentTrace.output[0]...)
		}
		for local := 0; local < end-start; local++ {
			trace.output[start+local+1] = append([]float64(nil), segmentTrace.output[local+1]...)
		}
		voltage = append([]float64(nil), segmentTrace.voltage[len(segmentTrace.voltage)-1]...)
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

// BackwardCoupledRecompute replays one segment at a time in private sessions,
// accumulating parameter gradients in the exact global order of Backward.
// Chemical concentration and activity-window gradients cross segment joins;
// fixed truncation boundaries use global time, independent of segment size.
// A new reverse session is created for each call. Errors return zero Gradient.
func (m *Continuous) BackwardCoupledRecompute(ctx context.Context, tr *ContinuousCoupledRecomputeTrace, upstream [][]float64, window int) (Gradient, error) {
	var empty Gradient
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
	if err := validateContinuousCoupledTrace(m, tr); err != nil {
		return empty, err
	}
	steps := len(tr.inputs)
	if len(upstream) != steps {
		return empty, fmt.Errorf("invalid backward window or sequence length")
	}
	for _, row := range upstream {
		if err := vector(row, m.config.Nodes, "upstream"); err != nil {
			return empty, err
		}
	}
	metadata := &Trace{
		model:      m,
		parameters: cloneParameters(tr.parameters),
		voltage:    [][]float64{append([]float64(nil), tr.initial...)},
		output:     tr.output,
		lambda:     tr.lambda,
		alpha:      tr.alpha,
		coupled:    &coupledTrace{sequence: tr.sequence},
	}

	currentStart := -1
	var current *Trace
	load := func(loadCtx context.Context, step int) (*Trace, int, error) {
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
			replaySequence, err := checkpoint.restart(loadCtx)
			if err != nil {
				return nil, 0, err
			}
			if replaySequence == nil {
				return nil, 0, fmt.Errorf("nil modulation replay at step %d", start)
			}
			end := steps
			if tr.segment <= steps-start {
				end = start + tr.segment
			}
			var saved *State
			if start > 0 {
				lo := max(0, start-m.maxEdgeDelay())
				saved = &State{
					Voltage: append([]float64(nil), checkpoint.voltage...),
					History: tr.output[lo : start+1],
					Steps:   uint64(start),
				}
			}
			current, err = m.forwardSequence(loadCtx, tr.parameters, checkpoint.voltage, tr.inputs[start:end], saved, replaySequence)
			if err != nil {
				return nil, 0, err
			}
			if current == nil || len(current.output) != end-start+1 || len(current.voltage) != end-start+1 {
				return nil, 0, fmt.Errorf("invalid replay trace at step %d", start)
			}
			currentStart = start
		}
		return current, step - currentStart, nil
	}
	return m.backwardCoupledTape(ctx, metadata, upstream, window, load)
}

func validateContinuousCoupledTrace(m *Continuous, tr *ContinuousCoupledRecomputeTrace) error {
	if tr.sequence == nil || len(tr.inputs) == 0 || tr.segment <= 0 {
		return fmt.Errorf("invalid coupled recompute trace")
	}
	steps, n := len(tr.inputs), m.config.Nodes
	if len(tr.output) != steps+1 || len(tr.initial) != n || len(tr.final) != n || len(tr.lambda) != n || len(tr.alpha) != n {
		return fmt.Errorf("invalid coupled recompute trace")
	}
	if len(tr.parameters.Weights) != len(m.config.Sources) || len(tr.parameters.Bias) != n || len(tr.parameters.LogTau) != n {
		return fmt.Errorf("invalid coupled recompute trace parameters")
	}
	for i, value := range tr.initial {
		if !finite(value) {
			return fmt.Errorf("non-finite coupled initial voltage at neuron %d", i)
		}
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
	for t, row := range tr.inputs {
		if err := vector(row, n, "coupled input"); err != nil {
			return fmt.Errorf("input[%d]: %w", t, err)
		}
	}
	for t, row := range tr.output {
		if err := vector(row, n, "coupled output"); err != nil {
			return fmt.Errorf("output[%d]: %w", t, err)
		}
	}
	if len(tr.checkpoints) == 0 || tr.checkpoints[0].start != 0 {
		return fmt.Errorf("invalid coupled recompute checkpoints")
	}
	want := 0
	for i, checkpoint := range tr.checkpoints {
		if checkpoint.start != want || checkpoint.start >= steps || checkpoint.restart == nil || len(checkpoint.voltage) != n {
			return fmt.Errorf("invalid coupled checkpoint %d", i)
		}
		if err := vector(checkpoint.voltage, n, "checkpoint voltage"); err != nil {
			return fmt.Errorf("checkpoint[%d]: %w", i, err)
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
