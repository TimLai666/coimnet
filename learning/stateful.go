package learning

import (
	"context"
	"fmt"
	"math"
)

// LossGradientFromState differentiates the supplied input segment from a saved
// neural state. The state and its delayed history are validated, copied by the
// core forward path, and treated as a constant boundary. Only CPU scalar
// continuous cores are supported; window uses the same recurrent truncation
// semantics as LossGradientFrom. The returned Gradient.Core.Initial reports the
// segment-start voltage sensitivity and never updates the saved state. Any
// validation, cancellation, or numerical failure returns a zero Gradient.
func (n *Network) LossGradientFromState(ctx context.Context, p Parameters, initial NeuralState, input, upstream [][]float64, window int) (Gradient, error) {
	var empty Gradient
	if ctx == nil {
		return empty, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if n == nil || n.core == nil {
		return empty, fmt.Errorf("nil network")
	}
	if window < 0 {
		return empty, fmt.Errorf("negative truncation window")
	}
	e, err := n.forwardSegmentState(ctx, p, input, 0, &initial)
	if err != nil {
		return empty, err
	}
	g, err := n.lossGradientReverse(ctx, input, e, upstream, window)
	if err != nil {
		return empty, err
	}
	return g, nil
}

// StepFromState applies an external upstream from a saved neural state through
// the existing mask, accumulation, clipping, schedule, AdamW, and projection
// path. Only CPU scalar continuous cores are supported. The state is held
// constant, Recompute is unsupported, and LossKnown is false because the caller
// supplies the upstream rather than an objective.
// Candidate parameters are validated from the same saved state before commit.
// Errors leave the trainer, optimizer, and accumulator unchanged.
func (tr *Trainer) StepFromState(ctx context.Context, initial NeuralState, input, upstream [][]float64) (StepResult, error) {
	var zero StepResult
	if tr == nil || ctx == nil {
		return zero, fmt.Errorf("nil trainer or context")
	}
	if tr.network == nil {
		return zero, fmt.Errorf("nil trainer")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if err := tr.mu.LockContext(ctx); err != nil {
		return zero, err
	}
	defer tr.mu.Unlock()
	if tr.updates == math.MaxUint64 {
		return zero, fmt.Errorf("update counter overflow")
	}
	if tr.options.Recompute != nil {
		return zero, fmt.Errorf("stateful continuous step does not support Recompute")
	}
	g, err := tr.network.LossGradientFromState(ctx, tr.parameters, initial, input, upstream, tr.options.Truncation)
	if err != nil {
		return zero, err
	}
	result, err := tr.stepWithGradient(ctx, input, 0, false, g, &initial)
	if err != nil {
		return zero, err
	}
	result.GradientHorizonSteps = gradientHorizon(len(input), tr.options.Truncation)
	return result, nil
}
