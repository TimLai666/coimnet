package learning

import (
	"context"
	"errors"
	"fmt"

	"github.com/TimLai666/coimnet/backend/webgpu"
	"github.com/TimLai666/coimnet/dynamics"
)

var (
	// ErrGPUTrainerUnsupported marks a valid trainer configuration that this
	// opt-in GPU episode trainer does not implement.
	ErrGPUTrainerUnsupported = errors.New("learning gpu trainer: unsupported configuration")
	// ErrGPUTrainerStateUnsupported marks persistent-state operations that are
	// outside the GPU trainer's independent-episode lifecycle.
	ErrGPUTrainerStateUnsupported = errors.New("learning gpu trainer: persistent state is unsupported")
)

// GPUAdapterInfo identifies the adapters used for sparse forward and backward
// work. The optimizer remains on the CPU.
type GPUAdapterInfo struct {
	Forward  webgpu.DeviceInfo
	Backward webgpu.DeviceInfo
}

// gpuContinuousCore keeps the existing continuous-core metadata and training
// layout, replacing only the sparse episode forward and reverse operations.
type gpuContinuousCore struct {
	continuousCore
	gpu *dynamics.GPUContinuous
}

func (g gpuContinuousCore) forward(ctx context.Context, p Parameters, initial []float64, inputs [][]float64) (coreTrace, [][]float64, error) {
	if len(p.ThetaRaw) != 0 {
		return nil, nil, fmt.Errorf("theta_raw requires a LIF core")
	}
	trace, err := g.gpu.Forward(ctx, p.Core, initial, inputs)
	if err != nil {
		return nil, nil, err
	}
	return trace, trace.Outputs(), nil
}

func (g gpuContinuousCore) backward(ctx context.Context, trace coreTrace, upstream [][]float64, window int) (coreGradient, error) {
	gpuTrace, ok := trace.(*dynamics.Trace)
	if !ok {
		return coreGradient{}, fmt.Errorf("trace belongs to a different core")
	}
	gradient, err := g.gpu.Backward(ctx, gpuTrace, upstream, window)
	if err != nil {
		return coreGradient{}, err
	}
	return coreGradient{core: gradient}, nil
}

func (gpuContinuousCore) newState([]float64) (NeuralState, error) {
	return NeuralState{}, ErrGPUTrainerStateUnsupported
}

func (gpuContinuousCore) validateState(NeuralState) error {
	return ErrGPUTrainerStateUnsupported
}

func (gpuContinuousCore) advance(context.Context, Parameters, NeuralState, [][]float64) (NeuralState, [][]float64, [][]float64, error) {
	return NeuralState{}, nil, nil, ErrGPUTrainerStateUnsupported
}

func (gpuContinuousCore) advanceModulated(context.Context, Parameters, NeuralState, [][]float64, *dynamics.Modulation) (NeuralState, [][]float64, [][]float64, error) {
	return NeuralState{}, nil, nil, ErrGPUTrainerStateUnsupported
}

// NewGPUTrainer constructs an independent-episode trainer whose sparse core
// forward and backward operations use WebGPU. Encoding and readout retain the
// existing Insyra path; AdamW remains on the host. GPU errors are returned
// directly; there is no CPU sparse-core execution fallback.
func NewGPUTrainer(ctx context.Context, c Config, p Parameters, o Options) (*Trainer, error) {
	if err := validateGPUTrainerSupport(c, o); err != nil {
		return nil, err
	}
	trainer, err := NewTrainer(c, p, o)
	if err != nil {
		return nil, err
	}
	return attachGPUTrainer(ctx, trainer)
}

// RestoreGPUTrainer restores an episode snapshot into the opt-in GPU trainer.
// The snapshot schema is unchanged, and an unsupported GPU configuration is
// rejected rather than restored as a CPU trainer.
func RestoreGPUTrainer(ctx context.Context, s TrainingSnapshot) (*Trainer, error) {
	if err := validateGPUTrainerSupport(s.Config, s.Options); err != nil {
		return nil, err
	}
	trainer, err := RestoreTrainer(s)
	if err != nil {
		return nil, err
	}
	return attachGPUTrainer(ctx, trainer)
}

func validateGPUTrainerSupport(c Config, o Options) error {
	if c.LIF != nil {
		return fmt.Errorf("%w: LIF core", ErrGPUTrainerUnsupported)
	}
	if c.Mixed != nil {
		return fmt.Errorf("%w: mixed core", ErrGPUTrainerUnsupported)
	}
	if c.Dynamics.StateDimension > 1 {
		return fmt.Errorf("%w: vector state dimension %d", ErrGPUTrainerUnsupported, c.Dynamics.StateDimension)
	}
	if c.Dynamics.EdgeShape == "matrix" {
		return fmt.Errorf("%w: matrix edges", ErrGPUTrainerUnsupported)
	}
	for edge, delay := range c.Dynamics.Delays {
		if delay > 0 {
			return fmt.Errorf("%w: edge %d has positive delay %d", ErrGPUTrainerUnsupported, edge, delay)
		}
	}
	if o.Recompute != nil {
		return fmt.Errorf("%w: recompute mode", ErrGPUTrainerUnsupported)
	}
	return nil
}

func attachGPUTrainer(ctx context.Context, trainer *Trainer) (*Trainer, error) {
	if trainer == nil || trainer.network == nil {
		return nil, fmt.Errorf("nil trainer")
	}
	core, ok := trainer.network.core.(continuousCore)
	if !ok {
		return nil, fmt.Errorf("%w: GPU episode training requires a continuous core", ErrGPUTrainerUnsupported)
	}
	gpu, err := dynamics.NewGPUContinuous(ctx, core.model.Config())
	if err != nil {
		return nil, fmt.Errorf("construct GPU episode core: %w", err)
	}
	trainer.network.core = gpuContinuousCore{continuousCore: core, gpu: gpu}
	return trainer, nil
}

// GPUAdapterInfo reports the forward and backward adapter identities. The
// boolean is false for a CPU trainer.
func (tr *Trainer) GPUAdapterInfo() (GPUAdapterInfo, bool) {
	if tr == nil || tr.network == nil {
		return GPUAdapterInfo{}, false
	}
	tr.mu.Lock()
	defer tr.mu.Unlock()
	core, ok := tr.network.core.(gpuContinuousCore)
	if !ok || core.gpu == nil {
		return GPUAdapterInfo{}, false
	}
	backward, hasBackward := core.gpu.BackwardInfo()
	return GPUAdapterInfo{Forward: core.gpu.DriveInfo(), Backward: backward}, hasBackward
}
