package dynamics

import (
	"context"
	"fmt"
	"math"
)

// ForwardRecompute returns the activated outputs of a fresh continuous
// sequence using at most one segment's voltage history and its delay prefix.
// Segment must be positive. It does not construct or export persistent state.
// Errors and cancellation return no outputs and do not change any arguments.
func (m *Continuous) ForwardRecompute(ctx context.Context, p Parameters, initial []float64, inputs [][]float64, segment int) ([][]float64, error) {
	if err := m.refuseGate("recompute"); err != nil {
		return nil, err
	}
	if err := m.refuseChemistry("recompute"); err != nil {
		return nil, err
	}
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

	steps := len(inputs)
	outputs := make([][]float64, steps)
	maxDelay := m.maxEdgeDelay()
	needOutput := len(m.config.Sources) > 0
	cp := recomputeCheckpoint{lo: 0, rows: [][]float64{append([]float64(nil), initial...)}}
	for start := 0; start < steps; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		width := segment
		if remaining := steps - start; width > remaining {
			width = remaining
		}
		end := start + width
		rows, outputRows, err := m.recomputeSegmentForward(ctx, p, inputs, cp, start, end, lambda, alpha, needOutput)
		if err != nil {
			return nil, err
		}
		for t := start; t < end; t++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if needOutput {
				outputs[t] = outputRows[t+1-cp.lo]
				continue
			}
			voltage := rows[t+1-cp.lo]
			output := make([]float64, n)
			for i, value := range voltage {
				output[i] = m.activate(value)
				if !finite(output[i]) {
					return nil, fmt.Errorf("non-finite output at step %d neuron %d", t, i)
				}
			}
			outputs[t] = output
		}
		if end < steps {
			cpLo := max(0, end-maxDelay)
			cpRows := make([][]float64, end-cpLo+1)
			base := cp.lo
			for r := cpLo; r <= end; r++ {
				cpRows[r-cpLo] = append([]float64(nil), rows[r-base]...)
			}
			cp = recomputeCheckpoint{lo: cpLo, rows: cpRows}
		}
		start = end
	}
	return outputs, nil
}

// ForwardRecompute returns the synaptic outputs of a fresh LIF sequence using
// one segment's neural history and its delay prefix. Segment must be positive.
// It preserves adaptation, refractory and homeostatic state between segments.
// Errors and cancellation return no outputs and do not change any arguments.
func (m *LIF) ForwardRecompute(ctx context.Context, p LIFParameters, initial []float64, inputs [][]float64, segment int) ([][]float64, error) {
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
	if len(inputs) > int(^uint(0)>>1)/n/8-1 {
		return nil, fmt.Errorf("sequence shape overflows")
	}
	lambda, alpha, base, _, err := m.lifConstants(p)
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
	maxDelay := 0
	for _, delay := range m.config.Delays {
		if delay > maxDelay {
			maxDelay = delay
		}
	}
	outputs := make([][]float64, steps)
	cp := m.lifInitialCheckpoint(initial)
	for start := 0; start < steps; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		width := segment
		if remaining := steps - start; width > remaining {
			width = remaining
		}
		end := start + width
		seg, err := m.lifRunSegment(ctx, p, inputs, cp, end, lambda, alpha, base)
		if err != nil {
			return nil, err
		}
		for t := start; t < end; t++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			outputs[t] = append([]float64(nil), seg.SynRow(t+1)...)
		}
		if end < steps {
			cp = seg.checkpointAt(end, maxDelay)
		}
		start = end
	}
	return outputs, nil
}
