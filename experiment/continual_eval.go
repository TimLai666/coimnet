package experiment

import (
	"context"
	"fmt"
	"reflect"

	"github.com/TimLai666/coimnet/learning"
)

// trainSeed and evalSeed derive the episode streams of one run: training
// episodes of stage i and evaluation episodes of task j never share a seed.
func trainSeed(seed uint64, stage int) uint64 {
	return mix(seed*0x9e3779b97f4a7c15 + uint64(stage) + 1)
}
func evalSeed(seed uint64, task int) uint64 {
	return mix(seed ^ 0xe5a1e5a1e5a1e5a1 + uint64(task)*0x9e3779b97f4a7c15)
}

// evaluateTask scores one task on a twin restored from the individual's
// snapshot, so the individual itself is never advanced or trained by an
// evaluation: for each of episodes fresh episodes from ContinualEpisode(spec,
// width, flipped, seed, e) it resets the twin's neural state to zeros, runs
// Advance and takes −MSE of the last output row against the target (mean over
// the output width, exactly itemMSE); the score is the mean over episodes.
// With fixedChemistry and a chemical part in the snapshot the twin is frozen
// (learning.Individual.FreezeChemistry(true)) before the first episode.
// After the episodes the twin's Parameters and Plastic part must equal the
// snapshot's (reflect.DeepEqual), otherwise the error "evaluation changed the
// individual" is returned: an evaluation is never a training step.
func evaluateTask(ctx context.Context, ind *learning.Individual, spec TaskSpec, width int, flipped bool, episodes int, seed uint64, fixedChemistry bool) (float64, error) {
	if ctx == nil {
		return 0, fmt.Errorf("evaluation needs a context")
	}
	if ind == nil {
		return 0, fmt.Errorf("evaluation needs an individual")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	reference := ind.Snapshot()
	twin, err := learning.RestoreIndividual(reference)
	if err != nil {
		return 0, err
	}
	if fixedChemistry && reference.Chemical != nil {
		if err := twin.FreezeChemistry(true); err != nil {
			return 0, err
		}
	}
	return scoreTwin(ctx, twin, reference, spec, width, flipped, episodes, seed)
}

// scoreTwin is the inner loop of evaluateTask on an already restored twin
// (reset, Advance, −MSE, mean; then the "evaluation changed the individual"
// check against reference, the snapshot the twin was restored from). A twin
// with a plastic part is frozen first (learning.Individual.FreezePlasticity),
// so the fast and slow layers reach the effective weights of the evaluation
// without the evaluation changing them; a twin without one takes the unchanged
// path. The state-switch control of the next ticket restores its own twin from
// a modified snapshot and calls this.
func scoreTwin(ctx context.Context, twin *learning.Individual, reference learning.IndividualSnapshot, spec TaskSpec, width int, flipped bool, episodes int, seed uint64) (float64, error) {
	if ctx == nil {
		return 0, fmt.Errorf("evaluation needs a context")
	}
	if twin == nil {
		return 0, fmt.Errorf("evaluation needs a twin")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if episodes <= 0 {
		return 0, fmt.Errorf("evaluation needs at least one episode, got %d", episodes)
	}
	if reference.Plastic != nil {
		// An evaluation is never a training step, so the twin's fast state is
		// held: it still carries the trained fast and slow layers into the
		// effective weights of every row, but its eligibility cannot accumulate
		// across the evaluation episodes, which is what the check below refuses.
		if err := twin.FreezePlasticity(true); err != nil {
			return 0, err
		}
	}
	var score float64
	for e := 0; e < episodes; e++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		ep, err := ContinualEpisode(spec, width, flipped, seed, uint64(e))
		if err != nil {
			return 0, err
		}
		if err := twin.ResetNeural(ctx, make([]float64, len(reference.Parameters.Core.Bias))); err != nil {
			return 0, err
		}
		out, err := twin.Advance(ctx, ep.Input)
		if err != nil {
			return 0, err
		}
		mse, err := itemMSE(out, EvaluationItem{Target: ep.Target})
		if err != nil {
			return 0, err
		}
		score += -mse / float64(episodes)
	}
	tw := twin.Snapshot()
	if !reflect.DeepEqual(tw.Parameters, reference.Parameters) || !reflect.DeepEqual(tw.Plastic, reference.Plastic) {
		return 0, fmt.Errorf("evaluation changed the individual")
	}
	return score, nil
}
