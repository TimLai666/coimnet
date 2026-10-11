package learning

import (
	"context"
	"fmt"

	"github.com/TimLai666/coimnet/dynamics"
)

// Recompute trades compute for memory in the reverse pass: the core keeps only segment checkpoints of the forward and re-runs
// each segment during the backward, giving the same gradient as the full history. SegmentSteps is the segment length in steps
// and must be at least 1. Only the continuous and LIF cores support it.
type Recompute struct {
	SegmentSteps int `json:"segment_steps"`
}

// recomputeSupported returns nil for continuousCore and lifCore and, for every other core, the error
// fmt.Errorf("learning: recompute supports the continuous and LIF cores, not %s", name) where name is "mixed" for mixedCore,
// "vector" for the vector core (*vectorCore, learning/vector_core.go) and fmt.Sprintf("%T", core) otherwise.
func recomputeSupported(core coreModel) error {
	switch core.(type) {
	case continuousCore, lifCore:
		return nil
	case mixedCore:
		return fmt.Errorf("learning: recompute supports the continuous and LIF cores, not %s", "mixed")
	case *vectorCore:
		return fmt.Errorf("learning: recompute supports the continuous and LIF cores, not %s", "vector")
	default:
		return fmt.Errorf("learning: recompute supports the continuous and LIF cores, not %s", fmt.Sprintf("%T", core))
	}
}

// matchForwardTheta repeats the theta_raw length checks of the matching forward
// pass, so observe and backwardRecompute refuse exactly the parameter groups
// the reference core.forward would refuse.
func matchForwardTheta(core coreModel, p Parameters) error {
	switch c := core.(type) {
	case continuousCore:
		if len(p.ThetaRaw) != 0 {
			return fmt.Errorf("theta_raw requires a LIF core")
		}
	case lifCore:
		if len(p.ThetaRaw) != c.nodeCount {
			return fmt.Errorf("theta_raw has %d values, want %d", len(p.ThetaRaw), c.nodeCount)
		}
	}
	return nil
}

// observe runs the core forward without keeping a trace and returns exactly the rows core.forward returns as its second result
// (activated outputs for the continuous core, the synaptic trace for the LIF core), bit for bit. It starts a fresh sequence at
// initial and delegates to the matching core's segmented forward, so the persistent-state capacity limit does not apply. It
// applies the same theta_raw length checks as the matching forward.
func observe(ctx context.Context, core coreModel, p Parameters, initial []float64, inputs [][]float64, segment int) ([][]float64, error) {
	if segment <= 0 {
		return nil, fmt.Errorf("recompute segment must be positive")
	}
	if err := recomputeSupported(core); err != nil {
		return nil, err
	}
	if err := matchForwardTheta(core, p); err != nil {
		return nil, err
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("empty sequence")
	}
	switch c := core.(type) {
	case continuousCore:
		return c.model.ForwardRecompute(ctx, p.Core, initial, inputs, segment)
	case lifCore:
		return c.model.ForwardRecompute(ctx, dynamics.LIFParameters{
			Weights: p.Core.Weights, Bias: p.Core.Bias, LogTau: p.Core.LogTau, ThetaRaw: p.ThetaRaw,
		}, initial, inputs, segment)
	}
	return nil, fmt.Errorf("learning: unsupported recompute core %T", core)
}

// backwardRecompute returns core.backward(core.forward(p, initial, inputs), upstream, window) through the core's
// BackwardRecompute with the given segment, mapped to coreGradient exactly as continuousCore.backward and lifCore.backward map
// theirs (for the LIF core: core = Weights, Bias, LogTau, Inputs, Initial and thetaRaw = ThetaRaw).
func backwardRecompute(ctx context.Context, core coreModel, p Parameters, initial []float64, inputs, upstream [][]float64, window, segment int) (coreGradient, error) {
	if segment <= 0 {
		return coreGradient{}, fmt.Errorf("recompute segment must be positive")
	}
	if err := recomputeSupported(core); err != nil {
		return coreGradient{}, err
	}
	if err := matchForwardTheta(core, p); err != nil {
		return coreGradient{}, err
	}
	switch c := core.(type) {
	case continuousCore:
		g, err := c.model.BackwardRecompute(ctx, p.Core, initial, inputs, upstream, window, segment)
		if err != nil {
			return coreGradient{}, err
		}
		return coreGradient{core: g}, nil
	case lifCore:
		g, err := c.model.BackwardRecompute(ctx, dynamics.LIFParameters{
			Weights: p.Core.Weights, Bias: p.Core.Bias, LogTau: p.Core.LogTau, ThetaRaw: p.ThetaRaw,
		}, initial, inputs, upstream, window, segment)
		if err != nil {
			return coreGradient{}, err
		}
		// ThetaRaw carries the base-threshold gradient; the four core groups are
		// mapped exactly as lifCore.backward maps its LIFGradient.
		return coreGradient{
			core:     dynamics.Gradient{Weights: g.Weights, Bias: g.Bias, LogTau: g.LogTau, Inputs: g.Inputs, Initial: g.Initial},
			thetaRaw: g.ThetaRaw,
		}, nil
	}
	return coreGradient{}, nil
}
