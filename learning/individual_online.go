package learning

import (
	"context"
	"fmt"
)

// advanceVersioned performs an online advance with the receptor learning gate
// closed and derives the model version while the same individual lock is held.
// The chemistry decay window still runs. Hashing first means a version
// encoding refusal cannot publish an advance that Act cannot record.
func (i *Individual) advanceVersioned(ctx context.Context, input [][]float64) ([][]float64, string, error) {
	if i == nil || i.trainer == nil {
		return nil, "", fmt.Errorf("uninitialized individual")
	}
	if err := i.mu.LockContext(ctx); err != nil {
		return nil, "", err
	}
	defer i.mu.Unlock()
	version, err := parameterVersion(i.trainer.parameters)
	if err != nil {
		return nil, "", err
	}
	output, _, err := i.advanceRowsWithGateSourceLocked(ctx, input, nil, false)
	if err != nil {
		return nil, "", err
	}
	return output, version, nil
}

// trainWeightedEpisode keeps the prediction, weighted target construction and
// training commit inside one individual operation. The prediction path uses
// the same ordinary or chemistry episode boundary as the public trainer path.
func (i *Individual) trainWeightedEpisode(ctx context.Context, input [][]float64, target []float64, score float64) (StepResult, error) {
	if i == nil || i.trainer == nil {
		return StepResult{}, fmt.Errorf("uninitialized individual")
	}
	if err := i.mu.LockContext(ctx); err != nil {
		return StepResult{}, err
	}
	defer i.mu.Unlock()
	prediction, err := i.episodePredictLocked(ctx, input)
	if err != nil {
		return StepResult{}, err
	}
	effective, err := weightedTarget(prediction, target, score)
	if err != nil {
		return StepResult{}, err
	}
	return i.trainEpisodeLocked(ctx, input, effective)
}

// episodePredictLocked is the independent zero-state prediction a weighted
// target is built from. The caller owns the individual's lock. The ordinary
// path uses Trainer.Predict; enabled chemistry takes the trainer lock after the
// individual lock and predicts without advancing persistent state.
func (i *Individual) episodePredictLocked(ctx context.Context, input [][]float64) ([]float64, error) {
	tr := i.trainer
	if i.chemical == nil {
		return tr.Predict(ctx, input)
	}
	if err := tr.mu.LockContext(ctx); err != nil {
		return nil, err
	}
	defer tr.mu.Unlock()
	network, err := i.chemicalEpisodeNetwork()
	if err != nil {
		return nil, err
	}
	return network.Predict(ctx, tr.parameters, input)
}
