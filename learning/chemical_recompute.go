package learning

import (
	"context"
	"fmt"

	"github.com/TimLai666/coimnet/dynamics"
)

// Checkpoint captures the private source/kinetics boundary before nextStep.
// Restarts own fresh runtime, concentration, resources, available feedback and
// controller windows. The two clocks stay separate. Only the original first
// step uses the explicit initial activity; later segments read checkpoint raw
// output. Replay never consumes or changes the original forward observations.
func (s *chemicalSequence) Checkpoint(ctx context.Context, nextStep int) (dynamics.ModulationRestart, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.runtime == nil || s.kinetics == nil {
		return nil, fmt.Errorf("uninitialized chemical sequence")
	}
	if nextStep != len(s.rows) || nextStep < 0 || nextStep >= s.steps {
		return nil, fmt.Errorf("chemical checkpoint step %d does not match the completed forward boundary", nextStep)
	}
	part := ChemicalPart{
		Config: s.config.Clone(), State: copyChemicalState(s.runtime.state),
		Resources:    make(map[string]float64, len(s.runtime.resources)),
		SourceStates: s.runtime.sourceStates(), Expression: copyExpression(s.expression),
	}
	for name, amount := range s.runtime.resources {
		part.Resources[name] = amount
	}
	for _, feedback := range s.runtime.pending {
		part.PendingFeedback = append(part.PendingFeedback, feedback.Spec())
	}
	episode := &ChemicalEpisode{Part: part, SourceStep: s.sourceStep + uint64(nextStep)}
	if nextStep == 0 && s.initialActivity != nil {
		episode.InitialActivity = append([]float64(nil), s.initialActivity...)
	}
	// newChemicalSequence makes an independent copy for every restart, so
	// the captured boundary is immutable even after an earlier replay.
	nodes, remaining, frozen := s.nodes, s.steps-nextStep, s.frozen
	return func(replayContext context.Context) (dynamics.ModulationSequence, error) {
		return newChemicalSequence(replayContext, episode, nodes, frozen, remaining)
	}, nil
}
