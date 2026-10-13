package learning

import (
	"context"
	"fmt"

	"github.com/TimLai666/coimnet/signal"
)

// chemicalEpisodeNetwork builds the same private zero-state episode boundary
// used by TrainEpisode. The caller owns the individual's lock and trainer lock;
// this helper never acquires either lock or changes persistent state.
func (i *Individual) chemicalEpisodeNetwork() (*Network, error) {
	if i == nil || i.trainer == nil || i.trainer.network == nil {
		return nil, fmt.Errorf("uninitialized individual")
	}
	if i.chemical == nil {
		return nil, fmt.Errorf("chemistry is not enabled")
	}
	tr := i.trainer
	chemical := i.chemical
	var resources map[string]float64
	if chemical.resources != nil {
		resources = make(map[string]float64, len(chemical.resources))
		for name, amount := range chemical.resources {
			resources[name] = amount
		}
	}
	var pending []signal.FeedbackSpec
	if chemical.pending != nil {
		pending = make([]signal.FeedbackSpec, len(chemical.pending))
		for index, feedback := range chemical.pending {
			pending[index] = feedback.Spec()
		}
	}
	initialActivity := previousActivity(i.neural)
	if initialActivity == nil {
		// No neural step has been observed, matching the persistent controller
		// source rather than the standalone episode's activated initial state.
		initialActivity = make([]float64, tr.network.core.nodes())
	}
	episode := &ChemicalEpisode{
		Part: ChemicalPart{
			Config:          chemical.config.Clone(),
			State:           copyChemicalState(chemical.state),
			Resources:       resources,
			PendingFeedback: pending,
			SourceStates:    chemical.sourceStates(),
			Expression:      copyExpression(i.expression),
		},
		SourceStep:      neuralSteps(i.neural),
		InitialActivity: initialActivity,
	}
	config := tr.network.Config()
	config.Chemistry = episode
	network, err := NewNetwork(config)
	if err != nil {
		return nil, err
	}
	network.chemicalFrozen = chemical.frozen
	return network, nil
}

// trainChemicalEpisode trains an independent zero-neural-state episode from
// the individual's current chemical/source/feedback/expression boundary and
// neural source clock. Its first source reads the current persistent raw
// activity as a constant; later sources read the private episode's raw output.
// The candidate update must be validated on that same isolated boundary.
// Success changes parameters, optimizer and the caller's episode counter only.
// Persistent neural/chemical/controller state, reports and queued feedback
// remain unchanged. Errors/cancellation publish no partial update. The caller
// owns the individual lock; this method owns the trainer lock and never swaps
// its shared network pointer. nil chemistry retains the existing Trainer.Step.
func (i *Individual) trainChemicalEpisode(ctx context.Context, input [][]float64, target []float64) (StepResult, error) {
	tr := i.trainer
	if i.chemical == nil {
		return tr.Step(ctx, input, target)
	}
	if err := tr.mu.LockContext(ctx); err != nil {
		return StepResult{}, err
	}
	defer tr.mu.Unlock()
	if tr.deviceAdam != nil {
		return StepResult{}, fmt.Errorf("individual chemical training does not support device AdamW")
	}

	network, err := i.chemicalEpisodeNetwork()
	if err != nil {
		return StepResult{}, err
	}
	candidate := &Trainer{
		network:     network,
		parameters:  tr.parameters,
		options:     tr.options,
		optimizer:   tr.optimizer,
		updates:     tr.updates,
		accumulator: tr.accumulator,
	}
	result, err := candidate.Step(ctx, input, target)
	if err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return StepResult{}, err
	}
	tr.parameters = candidate.parameters
	tr.optimizer = candidate.optimizer
	tr.updates = candidate.updates
	tr.accumulator = candidate.accumulator
	return result, nil
}
