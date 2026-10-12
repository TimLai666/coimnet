package dynamics

import "context"

// ModulationRestart constructs a fresh private replay session from an owned
// segment-start chemical boundary. Each call must return independent state;
// replay cannot advance the original sequence or any persistent individual.
type ModulationRestart func(context.Context) (ModulationSequence, error)

// RecomputableModulationSequence checkpoints the completed forward boundary
// immediately before nextStep. Its ordinary forward and reverse retain the
// same numerical and truncation contracts as ModulationSequence. A checkpoint
// owns concentration, source memory and source/kinetics clocks; replay starts
// at local step zero while the original sequence uses global step indices.
// Neither checkpoint nor restart accepts targets or commits external state.
type RecomputableModulationSequence interface {
	ModulationSequence
	Checkpoint(context.Context, int) (ModulationRestart, error)
}

// offsetModulationSequence adapts local segment steps to the original global
// forward sequence. Reverse arithmetic always uses the original sequence.
type offsetModulationSequence struct {
	sequence ModulationSequence
	start    int
}

func (s offsetModulationSequence) Forward(ctx context.Context, step int, previous []float64) (*Modulation, error) {
	return s.sequence.Forward(ctx, s.start+step, previous)
}

func (s offsetModulationSequence) NewReverse(ctx context.Context, steps, window int) (ModulationReverse, error) {
	return s.sequence.NewReverse(ctx, steps, window)
}
