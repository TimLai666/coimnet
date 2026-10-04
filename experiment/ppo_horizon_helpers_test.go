package experiment

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"

	"github.com/TimLai666/coimnet/experiment/gridnav"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/learning/rl"
)

// CollectedEnd preserves environment success/timeout independently of the
// terminal marker used by the six-step learning objective.
type ppoHorizonPoint struct {
	Feedback     ppoTrainingFeedbackPoint `json:"feedback"`
	CollectedEnd rl.Transition            `json:"collected_end"`
}

func copyHorizonRollout(r rl.Rollout) (rl.Rollout, error) {
	raw, err := json.Marshal(r)
	if err != nil {
		return rl.Rollout{}, fmt.Errorf("deadline copy marshal: %w", err)
	}
	var out rl.Rollout
	if err := json.Unmarshal(raw, &out); err != nil {
		return rl.Rollout{}, fmt.Errorf("deadline copy unmarshal: %w", err)
	}
	return out, nil
}

// deadlineRollout independently copies a valid single episode (1..limit
// steps), turns a final timeout into Done with zero bootstrap, and preserves
// successful episodes. Reject missing/conflicting/early end markers,
// non-finite records, invalid limit, and a timeout shorter than limit.
func deadlineRollout(r rl.Rollout, limit int) (rl.Rollout, error) {
	var empty rl.Rollout
	if limit <= 0 {
		return empty, fmt.Errorf("deadline limit %d, want a positive value", limit)
	}
	if len(r.Steps) == 0 {
		return empty, fmt.Errorf("deadline empty rollout")
	}
	if len(r.Steps) > limit {
		return empty, fmt.Errorf("deadline rollout has %d steps, exceeding limit %d", len(r.Steps), limit)
	}
	if _, _, err := rl.Advantages(r.Steps, rl.GAEConfig{Gamma: 1, Lambda: 1}); err != nil {
		return empty, fmt.Errorf("deadline original rollout invalid: %w", err)
	}
	last := len(r.Steps) - 1
	for i, s := range r.Steps[:last] {
		if s.Done {
			return empty, fmt.Errorf("deadline step %d ends early with done", i)
		}
		if s.Timeout {
			return empty, fmt.Errorf("deadline step %d ends early with timeout", i)
		}
	}
	end := r.Steps[last]
	switch {
	case end.Timeout && len(r.Steps) != limit:
		return empty, fmt.Errorf("deadline timeout has %d steps, want exactly limit %d", len(r.Steps), limit)
	}
	out, err := copyHorizonRollout(r)
	if err != nil {
		return empty, err
	}
	if out.Steps[last].Timeout {
		out.Steps[last].Done = true
		out.Steps[last].Timeout = false
		out.Steps[last].BootstrapValue = 0
	}
	return out, nil
}

// trainPPOHorizonFixture uses the existing initialization, PCG stream,
// environment seed and Update path. In deadline mode only the rollout's
// ending changes; collected success and bootstrap remain in CollectedEnd.
// It returns one complete point per update and an independent final model.
// Nil/canceled context, invalid config or upstream failure returns nil,nil,error.
func trainPPOHorizonFixture(ctx context.Context, c PPOExperimentConfig, seed uint64, deadline bool) (*learning.Individual, []ppoHorizonPoint, error) {
	if ctx == nil {
		return nil, nil, fmt.Errorf("horizon training needs a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if err := c.Validate(); err != nil {
		return nil, nil, fmt.Errorf("horizon training config invalid: %w", err)
	}
	ind, err := newPPOIndividual(seed, c.Hidden, c.LearningRate)
	if err != nil {
		return nil, nil, err
	}
	rng := rand.New(rand.NewPCG(seed, ppoTrainRNGStream))
	var points []ppoHorizonPoint
	for update := 1; update <= c.Updates; update++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		before := hash(ind.Snapshot())
		envSeed := imitationTrainSeed(seed, update-1)
		rollout, total, err := collectPPOEpisode(ctx, ind, c.Corridor, envSeed, rng)
		if err != nil {
			return nil, nil, err
		}
		if hash(ind.Snapshot()) != before {
			return nil, nil, fmt.Errorf("horizon collection changed caller snapshot at update %d", update)
		}
		cue, err := checkPPOTrainingEpisode(c.Corridor, envSeed, rollout.Steps, total)
		if err != nil {
			return nil, nil, err
		}
		if err := checkPPOTrainingPolicy(ctx, ind, c.Corridor, envSeed, rollout.Steps); err != nil {
			return nil, nil, err
		}
		collected := rollout.Steps[len(rollout.Steps)-1]
		collected.Obs = append([]float64(nil), collected.Obs...)
		used := rollout
		if deadline {
			used, err = deadlineRollout(rollout, c.Corridor.TimeLimit)
			if err != nil {
				return nil, nil, err
			}
		}
		advantages, targets, err := rl.Advantages(used.Steps, rl.GAEConfig{Gamma: c.PPO.Gamma, Lambda: c.PPO.Lambda})
		if err != nil {
			return nil, nil, err
		}
		updated, report, err := rl.Update(ctx, ind, []rl.Rollout{used}, gridnav.Actions, c.PPO)
		if err != nil {
			return nil, nil, err
		}
		if updated.Snapshot().Optimizer.Updates != uint64(update) {
			return nil, nil, fmt.Errorf("horizon optimizer step mismatch at update %d", update)
		}
		after := hash(updated.Snapshot())
		points = append(points, ppoHorizonPoint{
			Feedback: ppoTrainingFeedbackPoint{
				Seed: seed, Update: update, EnvSeed: envSeed, Cue: cue, Return: total,
				SnapshotBefore: before, SnapshotAfter: after, Rollout: used,
				Advantages: advantages, Targets: targets, Report: report,
			},
			CollectedEnd: collected,
		})
		ind = updated
	}
	return ind, points, nil
}
