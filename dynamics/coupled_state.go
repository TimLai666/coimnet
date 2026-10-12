package dynamics

import (
	"context"
	"fmt"
)

// ForwardCoupledFromState starts a coupled scalar continuous segment at a
// validated saved state. The saved voltage and delayed outputs are constant
// boundaries. The first chemical source sees the saved latest raw output;
// source feedback never differentiates into saved history. A nil sequence is
// exactly ForwardFromState. Shape, capacity, clock overflow and cancellation
// are checked before any result is published, without mutating arguments.
func (m *Continuous) ForwardCoupledFromState(ctx context.Context, p Parameters, initial State, inputs [][]float64, sequence ModulationSequence) (*Trace, error) {
	if sequence == nil {
		return m.ForwardFromState(ctx, p, initial, inputs)
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
	if err := m.ValidateState(initial); err != nil {
		return nil, err
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("empty sequence")
	}
	if len(inputs) > MaxStateValues/m.config.Nodes {
		return nil, fmt.Errorf("continuous output exceeds %d values", MaxStateValues)
	}
	if uint64(len(inputs)) > ^uint64(0)-initial.Steps {
		return nil, fmt.Errorf("continuous step counter overflow")
	}
	if _, err := m.stateHistoryRows(initial.Steps + uint64(len(inputs))); err != nil {
		return nil, err
	}
	trace, err := m.forwardSequence(ctx, p, initial.Voltage, inputs, &initial, sequence)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return trace, nil
}
