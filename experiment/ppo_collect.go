package experiment

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/TimLai666/coimnet/experiment/gridnav"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/learning/rl"
)

// collectPPOEpisode collects one complete gridnav episode from a zero-state
// copy of ind. The caller's individual is only snapshotted; all recurrent
// inference state belongs to the restored copy.
func collectPPOEpisode(ctx context.Context, ind *learning.Individual, c gridnav.Config, envSeed uint64, rng *rand.Rand) (rl.Rollout, float64, error) {
	var empty rl.Rollout
	if ctx == nil {
		return empty, 0, fmt.Errorf("ppo collector needs a context")
	}
	if err := ctx.Err(); err != nil {
		return empty, 0, err
	}
	if ind == nil {
		return empty, 0, fmt.Errorf("ppo collector needs an individual")
	}
	if rng == nil {
		return empty, 0, fmt.Errorf("ppo collector needs an rng")
	}
	if !finitePPOCollect(c.StepPenalty) || !finitePPOCollect(c.GoalReward) {
		return empty, 0, fmt.Errorf("ppo collector requires finite corridor configuration")
	}
	env, err := gridnav.New(c)
	if err != nil {
		return empty, 0, fmt.Errorf("ppo collector corridor: %w", err)
	}

	base := ind.Snapshot()
	if base.SchemaVersion == "" {
		return empty, 0, fmt.Errorf("ppo collector needs an initialized individual")
	}
	if base.Plastic != nil {
		return empty, 0, fmt.Errorf("ppo collector does not support plasticity")
	}
	if base.Chemical != nil {
		return empty, 0, fmt.Errorf("ppo collector does not support chemistry")
	}
	if base.Config.InputSize != 4 {
		return empty, 0, fmt.Errorf("ppo collector needs a 4-dimensional observation input, got %d", base.Config.InputSize)
	}
	if base.Config.OutputSize != gridnav.Actions+1 {
		return empty, 0, fmt.Errorf("ppo collector needs %d outputs, got %d", gridnav.Actions+1, base.Config.OutputSize)
	}
	if !base.Config.ReadoutEveryStep {
		return empty, 0, fmt.Errorf("ppo collector needs readout_every_step")
	}

	episode, err := learning.RestoreIndividual(base)
	if err != nil {
		return empty, 0, fmt.Errorf("ppo collector restore: %w", err)
	}
	if err := episode.ResetNeural(ctx, make([]float64, len(base.Parameters.Core.Bias))); err != nil {
		return empty, 0, fmt.Errorf("ppo collector reset neural state: %w", err)
	}
	initial := episode.Snapshot()
	rollout := rl.Rollout{
		PolicyVersion:   rl.PolicyVersion(episode),
		InitialNeural:   initial.Neural,
		InitialPlastic:  initial.Plastic,
		InitialChemical: initial.Chemical,
	}

	obs, _ := env.Reset(envSeed)
	var total float64
	for stepIndex := 0; stepIndex < c.TimeLimit; stepIndex++ {
		if err := ctx.Err(); err != nil {
			return empty, 0, err
		}
		vector := obs.Vector()
		if err := validatePPOCollectObservation(vector); err != nil {
			return empty, 0, fmt.Errorf("ppo collector observation at step %d: %w", stepIndex, err)
		}
		out, err := episode.Advance(ctx, [][]float64{vector})
		if err != nil {
			return empty, 0, fmt.Errorf("ppo collector advance at step %d: %w", stepIndex, err)
		}
		if len(out) != 1 || len(out[0]) != gridnav.Actions+1 {
			return empty, 0, fmt.Errorf("ppo collector output at step %d has shape %dx%d, want 1x%d", stepIndex, len(out), ppoCollectOutputWidth(out), gridnav.Actions+1)
		}
		row := out[0]
		for j, value := range row {
			if !finitePPOCollect(value) {
				return empty, 0, fmt.Errorf("ppo collector output at step %d index %d is non-finite", stepIndex, j)
			}
		}
		action, err := samplePPOCollectAction(row[:gridnav.Actions], rng)
		if err != nil {
			return empty, 0, fmt.Errorf("ppo collector action at step %d: %w", stepIndex, err)
		}
		logProb, err := rl.LogProb(row[:gridnav.Actions], action)
		if err != nil {
			return empty, 0, fmt.Errorf("ppo collector log probability at step %d: %w", stepIndex, err)
		}
		if !finitePPOCollect(logProb) || logProb > 0 {
			return empty, 0, fmt.Errorf("ppo collector log probability at step %d is invalid: %v", stepIndex, logProb)
		}

		next, reward, _, info, err := env.Step(action)
		if err != nil {
			return empty, 0, fmt.Errorf("ppo collector environment step %d: %w", stepIndex, err)
		}
		if !finitePPOCollect(reward) {
			return empty, 0, fmt.Errorf("ppo collector reward at step %d is non-finite: %v", stepIndex, reward)
		}
		total += reward
		if !finitePPOCollect(total) {
			return empty, 0, fmt.Errorf("ppo collector return at step %d is non-finite", stepIndex)
		}
		transition := rl.Transition{
			Obs:     append([]float64(nil), vector...),
			Action:  action,
			LogProb: logProb,
			Value:   row[gridnav.Actions],
			Reward:  reward,
			Done:    info.Reached,
			Timeout: info.TimedOut,
		}
		if transition.Done && transition.Timeout {
			return empty, 0, fmt.Errorf("ppo collector environment marked a step both reached and timed out")
		}
		if transition.Timeout {
			nextVector := next.Vector()
			if err := validatePPOCollectObservation(nextVector); err != nil {
				return empty, 0, fmt.Errorf("ppo collector timeout observation at step %d: %w", stepIndex, err)
			}
			bootstrap, err := episode.Advance(ctx, [][]float64{nextVector})
			if err != nil {
				return empty, 0, fmt.Errorf("ppo collector timeout bootstrap at step %d: %w", stepIndex, err)
			}
			if len(bootstrap) != 1 || len(bootstrap[0]) != gridnav.Actions+1 {
				return empty, 0, fmt.Errorf("ppo collector timeout bootstrap output has invalid shape")
			}
			transition.BootstrapValue = bootstrap[0][gridnav.Actions]
			if !finitePPOCollect(transition.BootstrapValue) {
				return empty, 0, fmt.Errorf("ppo collector timeout bootstrap is non-finite")
			}
		}
		rollout.Steps = append(rollout.Steps, transition)
		if transition.Done || transition.Timeout {
			if err := ctx.Err(); err != nil {
				return empty, 0, err
			}
			return rollout, total, nil
		}
		if stepIndex == c.TimeLimit-1 {
			return empty, 0, fmt.Errorf("ppo collector episode reached time limit without an end marker")
		}
		obs = next
	}
	return empty, 0, fmt.Errorf("ppo collector episode did not end")
}

func samplePPOCollectAction(logits []float64, rng *rand.Rand) (int, error) {
	if len(logits) != gridnav.Actions {
		return 0, fmt.Errorf("got %d action logits, want %d", len(logits), gridnav.Actions)
	}
	maxLogit := logits[0]
	for _, logit := range logits[1:] {
		if logit > maxLogit {
			maxLogit = logit
		}
	}
	probabilities := make([]float64, len(logits))
	var sum float64
	for i, logit := range logits {
		probabilities[i] = math.Exp(logit - maxLogit)
		sum += probabilities[i]
	}
	if !finitePPOCollect(sum) || sum <= 0 {
		return 0, fmt.Errorf("softmax normalization is invalid: %v", sum)
	}
	draw := rng.Float64() * sum
	var cumulative float64
	for action, probability := range probabilities {
		cumulative += probability
		if draw < cumulative {
			return action, nil
		}
	}
	return len(probabilities) - 1, nil
}

func validatePPOCollectObservation(vector []float64) error {
	if len(vector) != 4 {
		return fmt.Errorf("width %d, want 4", len(vector))
	}
	for i, value := range vector {
		if !finitePPOCollect(value) {
			return fmt.Errorf("value %d is non-finite", i)
		}
	}
	return nil
}

func ppoCollectOutputWidth(output [][]float64) int {
	if len(output) == 0 {
		return 0
	}
	return len(output[0])
}

func finitePPOCollect(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
