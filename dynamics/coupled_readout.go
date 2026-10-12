package dynamics

import (
	"context"
	"fmt"
)

// ModulationReadoutReverse is an optional part of a fresh modulation reverse
// session. BackwardReadout receives only the caller's direct readout adjoint,
// before recurrent or chemical feedback is added. It returns the raw core
// adjoint and may retain private receptor adjoints for BackwardStep. Calls use
// increasing output step order. The session never mutates a shared trace.
type ModulationReadoutReverse interface {
	BackwardReadout(context.Context, int, []float64) ([]float64, error)
}

// coupledReadoutAdjoints returns owned transformed rows for an optional
// readout reverse. A session without that interface returns nil, preserving
// the existing path. Callback errors, invalid shape, nonfinite values or
// cancellation publish no partial rows.
func coupledReadoutAdjoints(ctx context.Context, reverse ModulationReverse, upstream [][]float64, nodes int) ([][]float64, error) {
	readout, ok := reverse.(ModulationReadoutReverse)
	if !ok {
		return nil, nil
	}
	result := make([][]float64, len(upstream))
	for step, row := range upstream {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := vector(row, nodes, "readout upstream"); err != nil {
			return nil, err
		}
		owned := append([]float64(nil), row...)
		values, err := readout.BackwardReadout(ctx, step, owned)
		if err != nil {
			return nil, fmt.Errorf("readout reverse step %d: %w", step, err)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := vector(values, nodes, "raw readout adjoint"); err != nil {
			return nil, fmt.Errorf("readout reverse step %d: %w", step, err)
		}
		result[step] = append([]float64(nil), values...)
	}
	return result, nil
}
