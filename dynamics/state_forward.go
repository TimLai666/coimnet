package dynamics

import (
	"context"
	"fmt"
)

// ForwardFromState starts a differentiable CPU scalar continuous segment at a
// saved state. The state version, configuration hash, shapes, finite values,
// retained-history capacity, and step counter are validated before any forward
// result is published. Saved history is copied and treated as a constant
// boundary; Backward differentiates only this segment. Gradient.Initial reports
// sensitivity to the segment-start Voltage through the membrane leak path, so
// an optimizer must not update the saved state from it.
func (m *Continuous) ForwardFromState(ctx context.Context, p Parameters, initial State, inputs [][]float64) (*Trace, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := m.ValidateState(initial); err != nil {
		return nil, err
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("empty sequence")
	}
	n := m.config.Nodes
	// Bound the input and returned output matrices before allocating a trace.
	// Trace also retains its private initial row; MaxStateValues is not a
	// bound on internal trace storage or process memory.
	if len(inputs) > MaxStateValues/n {
		return nil, fmt.Errorf("continuous output exceeds %d values", MaxStateValues)
	}
	if uint64(len(inputs)) > ^uint64(0)-initial.Steps {
		return nil, fmt.Errorf("continuous step counter overflow")
	}
	finalSteps := initial.Steps + uint64(len(inputs))
	if _, err := m.stateHistoryRows(finalSteps); err != nil {
		return nil, err
	}
	trace, err := m.forward(ctx, p, initial.Voltage, inputs, &initial)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return trace, nil
}
