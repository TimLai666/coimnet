package experiment

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/signal"
)

// ablationControllerResource is the resource the trainable controller writes
// its release into. The chemistry reads it by this name and by the declared
// internal-resource rule, so the path from the controller to the concentration
// is one named, recomputable step rather than a direct injection.
const ablationControllerResource = "controller"

// ablationController is the trainable controller of one seed: window 3,
// UseFeedback, no resources, Nodes 3, Hidden hidden, Channel 0, parameters
// uniform in [-0.5, 0.5] from rand.New(rand.NewPCG(seed, 99)). The stream is
// counter-based and seeded from the group's own seed, so two runs of the same
// seed start from the same policy and no mutable random state is shared.
func ablationController(seed uint64, hidden int) (*modulation.Controller, error) {
	if hidden < 1 {
		return nil, fmt.Errorf("ablation controller declares %d hidden units, want at least 1", hidden)
	}
	c := &modulation.Controller{
		Inputs:  modulation.ControllerInputs{SummaryWindow: 3, UseFeedback: true},
		Nodes:   3,
		Hidden:  hidden,
		Channel: 0,
	}
	r := rand.New(rand.NewPCG(seed, 99))
	c.Parameters = make([]float64, c.ParameterCount())
	for i := range c.Parameters {
		c.Parameters[i] = r.Float64() - .5
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// controllerContext is what the controller may read after episode e: the
// individual's last node-activity row and the previous episode's score as an
// already-available feedback (none for e == 0). No target ever enters it.
//
// The feedback is stamped at episode e-1 on the release clock, so it has
// arrived by the step the controller is released for and the source's own
// arrival check passes on the value rather than around it. A nil score is no
// feedback at all: it is the state before the first episode has produced one,
// not a zero score.
func controllerContext(ind *learning.Individual, e int, previousScore *float64) (modulation.SourceContext, error) {
	var source modulation.SourceContext
	if ind == nil {
		return source, fmt.Errorf("ablation controller context needs an individual")
	}
	if e < 0 {
		return source, fmt.Errorf("ablation controller context is asked for episode %d", e)
	}
	activity, err := ablationActivity(ind.Snapshot().Neural)
	if err != nil {
		return source, err
	}
	source.Activity = activity
	if e == 0 || previousScore == nil {
		return source, nil
	}
	previous := int64(e - 1)
	stamp := signal.Timestamp{Value: previous, Unit: modulation.ReleaseTimeUnit}
	feedback, err := signal.NewFeedback(signal.FeedbackSpec{
		SchemaVersion: signal.CurrentSchemaVersion(),
		ExperienceID:  fmt.Sprintf("ablation-episode-%d", previous),
		ActionID:      fmt.Sprintf("ablation-action-%d", previous),
		ProducedAt:    stamp,
		AvailableAt:   stamp,
		Source:        "ablation",
		Score:         *previousScore,
		ModelVersion:  signal.CurrentSchemaVersion(),
	})
	if err != nil {
		return modulation.SourceContext{}, err
	}
	source.Feedback = []signal.Feedback{feedback}
	return source, nil
}

// ablationActivity is the last node-activity row of an individual's persistent
// neural state. Before the first step there is no observed row: the history
// still holds the initial state, which is a declaration and not activity, so
// the controller is handed nothing and summarises it as the zeros it keeps for
// steps it has not seen.
func ablationActivity(s learning.NeuralState) ([]float64, error) {
	if s.Continuous == nil {
		return nil, fmt.Errorf("ablation controller reads the continuous core, this individual drives %q", s.Core)
	}
	if s.Continuous.Steps == 0 || len(s.Continuous.History) == 0 {
		return nil, nil
	}
	return append([]float64(nil), s.Continuous.History[len(s.Continuous.History)-1]...), nil
}

// ablationControllerChemistry is the fixed_decay declaration with one source
// replaced: the same single region, the same hypothesized receptor, the same
// sensitivity effect and the same learning gate, released from the resource
// the controller writes instead of from a fixed external timeline. Everything
// but where the release comes from is held equal on purpose.
func ablationControllerChemistry(episodes int) modulation.ChemistryConfig {
	config := ablationChemistry(episodes)
	config.Sources = []modulation.SourceSpec{{
		Kind: modulation.SourceInternalResource, Channel: 0,
		Resource: &modulation.InternalResource{
			Resource: ablationControllerResource, Coefficient: 1, Threshold: 0, Channel: 0,
		},
	}}
	return config
}

// trainAblationController trains one seed of the trainable_controller group and
// hands back both halves it produced: the individual and the controller that
// drove it. Every episode is one release, one walk, one isolated gradient step
// of the individual and one gradient step of the controller against its own
// declared objective, in that order. The returned parameter count is the
// individual's alone; the caller adds the controller's.
//
// The controller is returned whenever it was built, including alongside an
// error, so a failed run still reports the capacity it was going to carry.
func trainAblationController(ctx context.Context, c AblationConfig, seed uint64, episodes []Episode) (*learning.Individual, *modulation.Controller, int, error) {
	ind, count, err := ablationIndividual(seed, GroupFixedDecay, c)
	if err != nil {
		return nil, nil, count, err
	}
	// The gate, the receptor and the regions come from ablationIndividual's
	// fixed_decay path; only the source is replaced here, which is why the
	// chemistry is declared a second time rather than from scratch.
	if err := ind.EnableChemistry(ablationControllerChemistry(c.Episodes)); err != nil {
		return nil, nil, count, err
	}
	if err := ind.SetResource(ablationControllerResource, 0); err != nil {
		return nil, nil, count, err
	}
	controller, err := ablationController(seed, c.ControllerHidden)
	if err != nil {
		return nil, nil, count, err
	}
	objective := modulation.ControllerObjective{Kind: modulation.ObjectiveRewardProxy}
	var previous *float64
	for e, ep := range episodes {
		source, err := controllerContext(ind, e, previous)
		if err != nil {
			return ind, controller, count, err
		}
		rates, err := controller.Release(uint64(e), source)
		if err != nil {
			return ind, controller, count, err
		}
		if err := ind.SetResource(ablationControllerResource, rates[controller.Channel]); err != nil {
			return ind, controller, count, err
		}
		out, err := ind.Advance(ctx, ep.Input)
		if err != nil {
			return ind, controller, count, err
		}
		if _, err := ind.TrainEpisode(ctx, ep.Input, ep.Target); err != nil {
			return ind, controller, count, err
		}
		d := out[len(out)-1][0] - ep.Target[0]
		score := math.Max(0, 1-d*d)
		_, grad, err := controller.Gradient(objective)
		if err != nil {
			return ind, controller, count, err
		}
		if _, err := controller.Update(grad, c.ControllerRate); err != nil {
			return ind, controller, count, err
		}
		previous = &score
	}
	return ind, controller, count, nil
}

// runTrainableController is one seed of the trainable_controller group: the
// trained individual is scored exactly like every other group's, on the same
// restored twin and the same held-out metric, so only the training differs.
func runTrainableController(ctx context.Context, c AblationConfig, seed uint64, episodes []Episode, held []Episode) (GroupRun, []float64, error) {
	run := GroupRun{Group: GroupTrainableController, Seed: seed}
	if len(held) != c.EvalEpisodes {
		return ablationFailed(ctx, run, fmt.Errorf("trainable_controller was handed %d held-out episodes, the protocol declares %d", len(held), c.EvalEpisodes))
	}
	ind, controller, count, err := trainAblationController(ctx, c, seed, episodes)
	run.Parameters = count
	if controller != nil {
		run.Parameters += controller.ParameterCount()
	}
	if err != nil {
		return ablationFailed(ctx, run, err)
	}
	score, preds, err := scoreAblationIndividual(ctx, ind, c)
	if err != nil {
		return ablationFailed(ctx, run, err)
	}
	run.Score = score
	return run, preds, nil
}

// trainAblationCapacityMatched trains one seed of the capacity_matched group:
// the no_modulation individual, with a control that has the controller's
// parameter count and adds its scalar output to the readout instead of
// releasing anything. The individual's own parameters keep training against the
// unchanged loss, so the offset never enters their gradient; only the metric
// this group reports is computed from the corrected readout.
//
// The last offset is returned with the control, because the evaluation adds
// that one constant to every held-out readout.
func trainAblationCapacityMatched(ctx context.Context, c AblationConfig, seed uint64, episodes []Episode) (*learning.Individual, *modulation.MemoryController, int, float64, error) {
	var offset float64
	ind, count, err := ablationIndividual(seed, GroupNoModulation, c)
	if err != nil {
		return nil, nil, count, offset, err
	}
	controller, err := ablationController(seed, c.ControllerHidden)
	if err != nil {
		return nil, nil, count, offset, err
	}
	memory, err := modulation.NewCapacityMatched(controller)
	if err != nil {
		return nil, nil, count, offset, err
	}
	var previous *float64
	for e, ep := range episodes {
		source, err := controllerContext(ind, e, previous)
		if err != nil {
			return ind, memory, count, offset, err
		}
		if offset, err = memory.Offset(uint64(e), source); err != nil {
			return ind, memory, count, offset, err
		}
		out, err := ind.Advance(ctx, ep.Input)
		if err != nil {
			return ind, memory, count, offset, err
		}
		readout := out[len(out)-1][0]
		if _, err := ind.TrainEpisode(ctx, ep.Input, ep.Target); err != nil {
			return ind, memory, count, offset, err
		}
		d := readout + offset - ep.Target[0]
		score := math.Max(0, 1-d*d)
		// The control's own target is the residual the readout leaves: what the
		// offset would have had to be for this episode to land on its target.
		_, grad, err := memory.Gradient(ep.Target[0] - readout)
		if err != nil {
			return ind, memory, count, offset, err
		}
		if _, err := memory.Update(grad, c.ControllerRate); err != nil {
			return ind, memory, count, offset, err
		}
		previous = &score
	}
	return ind, memory, count, offset, nil
}

// runCapacityMatched is one seed of the capacity_matched group. The evaluation
// is the restored twin of every other group, with the last training episode's
// offset added to each held-out readout as the constant it is: the control
// carries the controller's capacity, not its access to the running task.
func runCapacityMatched(ctx context.Context, c AblationConfig, seed uint64, episodes []Episode, held []Episode) (GroupRun, []float64, error) {
	run := GroupRun{Group: GroupCapacityMatched, Seed: seed}
	if len(held) != c.EvalEpisodes {
		return ablationFailed(ctx, run, fmt.Errorf("capacity_matched was handed %d held-out episodes, the protocol declares %d", len(held), c.EvalEpisodes))
	}
	ind, memory, count, offset, err := trainAblationCapacityMatched(ctx, c, seed, episodes)
	run.Parameters = count
	if memory != nil {
		run.Parameters += memory.ParameterCount()
	}
	if err != nil {
		return ablationFailed(ctx, run, err)
	}
	score, preds, err := scoreAblationOffset(ctx, ind, held, offset)
	if err != nil {
		return ablationFailed(ctx, run, err)
	}
	run.Score = score
	return run, preds, nil
}

// scoreAblationOffset is scoreAblationIndividual's evaluation with a constant
// added to every readout: a restored twin, a fresh zero-voltage episode per
// held-out sample and the negative mean squared error of the last output row.
func scoreAblationOffset(ctx context.Context, ind *learning.Individual, held []Episode, offset float64) (float64, []float64, error) {
	twin, err := learning.RestoreIndividual(ind.Snapshot())
	if err != nil {
		return 0, nil, err
	}
	preds := make([]float64, len(held))
	var sumSq float64
	for i, ep := range held {
		if err := twin.ResetNeural(ctx, make([]float64, 3)); err != nil {
			return 0, nil, err
		}
		out, err := twin.Advance(ctx, ep.Input)
		if err != nil {
			return 0, nil, err
		}
		pred := out[len(out)-1][0] + offset
		preds[i] = pred
		d := pred - ep.Target[0]
		sumSq += d * d
	}
	return -sumSq / float64(len(held)), preds, nil
}

// ablationFailed records a non-context error as a failed run, exactly the way
// runAblationSeed does: a canceled context stops the protocol, anything else is
// a missing cell the report keeps its reason for.
func ablationFailed(ctx context.Context, run GroupRun, err error) (GroupRun, []float64, error) {
	if ctx.Err() != nil {
		return run, nil, ctx.Err()
	}
	run.Failed, run.Error = true, err.Error()
	return run, nil, nil
}
