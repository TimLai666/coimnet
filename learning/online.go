package learning

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/TimLai666/coimnet/signal"
)

// ErrEvaluateMode is returned by Update while the policy declares the test
// mode and feedback has arrived. The refusal is the whole answer: no gradient
// step, no plastic step, and nothing leaves the queue, so a caller cannot get
// half an update out of a mode that was meant to change nothing.
var ErrEvaluateMode = errors.New("learning: evaluate mode refuses online updates")

// RewardGate is the rule that turns one raw feedback score into the
// non-negative release a gated plastic update consumes. The reward mappers of
// the modulation package are exactly this rule, and RewardGateFunc is how one
// is handed in.
//
// The interface exists instead of the concrete mapper type because the
// modulation package sits above this one in the dependency order: it reaches
// the native runner, which reaches the connectome store, whose own tests use
// this package, so naming modulation here closes an import cycle. Declaring
// the rule as a value the caller supplies keeps the dependency pointing one
// way and still leaves the mapping rule where it was decided.
type RewardGate interface {
	// Applied returns the release per modulation channel for one raw score.
	// The first channel is the gate an online plastic update opens.
	Applied(score float64) ([]float64, error)
}

// RewardGateFunc adapts a plain function to RewardGate, which is how a
// modulation reward mapper reaches an online learner:
//
//	gate := learning.RewardGateFunc(func(score float64) ([]float64, error) {
//		record, err := mapper.Map(score)
//		return record.Applied, err
//	})
type RewardGateFunc func(score float64) ([]float64, error)

// Applied calls the wrapped function.
func (f RewardGateFunc) Applied(score float64) ([]float64, error) { return f(score) }

// OnlinePolicy declares what an online learner is allowed to do with the
// feedback it receives. AllowGradient permits base-parameter updates, which
// still run inside the individual's own Options and therefore inside its update
// masks; AllowPlastic permits gated local plasticity, which never reaches base
// parameters. Evaluate is the test mode, in which neither is allowed.
//
// UpdateEvery is how many gradient-eligible feedbacks one gradient step covers.
// Zero and one run a step for every one of them.
//
// ClockUnit is the clock the learner and its feedback share. It must be
// signal.TimeUnitModelStep: an action's own time is a model step count, and
// comparing it against a wall-clock timestamp would silently admit feedback
// that has not arrived.
type OnlinePolicy struct {
	AllowGradient bool            `json:"allow_gradient"`
	AllowPlastic  bool            `json:"allow_plastic"`
	Evaluate      bool            `json:"evaluate"`
	UpdateEvery   uint64          `json:"update_every"`
	ClockUnit     signal.TimeUnit `json:"clock_unit"`
}

func (p OnlinePolicy) validate() error {
	if p.ClockUnit != signal.TimeUnitModelStep {
		return fmt.Errorf("learning: an online learner runs on %q, not %q", signal.TimeUnitModelStep, p.ClockUnit)
	}
	if !p.AllowGradient && !p.AllowPlastic {
		return fmt.Errorf("learning: an online policy must allow gradient updates, plastic updates or both")
	}
	return nil
}

// updateEvery resolves UpdateEvery: zero means one, which is a gradient step
// for every gradient-eligible feedback.
func (p OnlinePolicy) updateEvery() uint64 {
	if p.UpdateEvery == 0 {
		return 1
	}
	return p.UpdateEvery
}

// ActionRecord is one answer the individual has already given. It is written
// once, by Act, and never rewritten: no feedback and no update can change
// Output or OutputHash, which is what makes "the answer already given" a fact
// rather than something a later teacher can edit.
//
// Step is the model step at which the last output row was produced, counted by
// the learner itself from the rows every Act consumed. OutputHash is SHA-256
// over the little-endian IEEE-754 bits of every output value in row-major
// order; the shape is not part of the preimage, because one individual's output
// width never changes. ModelVersion is SHA-256 of the canonical JSON of the
// parameters that produced this answer, so a reader can tell whether two
// answers came from the same model.
type ActionRecord struct {
	ActionID     string      `json:"action_id"`
	Step         uint64      `json:"step"`
	Output       [][]float64 `json:"output"`
	OutputHash   string      `json:"output_hash"`
	ModelVersion string      `json:"model_version"`
}

// UpdateReport counts what one Update did. Consumed is how much feedback left
// the queue, GradientSteps and PlasticSteps how many updates of each kind ran,
// and Deferred how much feedback is still waiting for its declared arrival
// time.
type UpdateReport struct {
	Consumed      int `json:"consumed"`
	GradientSteps int `json:"gradient_steps"`
	PlasticSteps  int `json:"plastic_steps"`
	Deferred      int `json:"deferred"`
}

// onlineEntry is the learner's own record of one action: the public record, the
// input that produced it, the experience the observation belonged to, and which
// feedback sources have already been counted for it.
type onlineEntry struct {
	record       ActionRecord
	experienceID string
	input        [][]float64
	sources      map[string]bool
}

// OnlineLearner runs the three separated steps of the online loop over one
// persistent individual: Act produces an answer, Receive queues feedback about
// an answer already given, and Update applies the policy to the feedback that
// has reached its declared arrival time.
//
// The three steps are separated on purpose. Act never consumes feedback, so an
// answer cannot depend on its own outcome; Receive never changes the model, so
// arriving feedback cannot rewrite an answer; Update is the only step that
// changes anything, and it only ever changes what the policy allows.
//
// Targets live on the learner, set by the trainer side through SetTargets, and
// never arrive with a feedback. A feedback carries a score, which becomes a
// loss weight; the answer it should have given is a training-side fact.
type OnlineLearner struct {
	mu         cancellableMutex
	individual *Individual
	policy     OnlinePolicy
	gate       RewardGate
	log        []onlineEntry
	index      map[string]int
	targets    map[string][]float64
	pending    []signal.Feedback
	step       uint64
	actions    uint64
	eligible   uint64
	outputSize int
}

// NewOnlineLearner validates the whole loop before it can run once. The reward
// gate is required by a plastic policy, because that is what turns a feedback
// score into a gate, and is refused by a policy that has no plastic half,
// because an unread gate would be a second knob with no effect. A plastic
// policy also needs an individual that has the mechanism enabled, so the
// refusal happens here rather than at the first gated advance.
func NewOnlineLearner(ind *Individual, p OnlinePolicy, gate RewardGate) (*OnlineLearner, error) {
	if ind == nil || ind.trainer == nil {
		return nil, fmt.Errorf("learning: an online learner needs an initialized individual")
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	if p.AllowPlastic {
		if gate == nil {
			return nil, fmt.Errorf("learning: a plastic online policy needs a reward gate")
		}
		applied, err := gate.Applied(0)
		if err != nil {
			return nil, fmt.Errorf("learning: reward gate: %w", err)
		}
		if len(applied) == 0 {
			return nil, fmt.Errorf("learning: the reward gate produced no release channel")
		}
		if !ind.plasticityEnabled() {
			return nil, fmt.Errorf("learning: a plastic online policy needs an individual with local plasticity enabled")
		}
	} else if gate != nil {
		return nil, fmt.Errorf("learning: a reward gate without a plastic policy would never be read")
	}
	return &OnlineLearner{
		individual: ind, policy: p, gate: gate,
		index: map[string]int{}, targets: map[string][]float64{},
		outputSize: ind.trainer.network.config.OutputSize,
	}, nil
}

// Policy returns the declared policy.
func (l *OnlineLearner) Policy() OnlinePolicy { return l.policy }

// Log returns an independent copy of every answer given so far, in order. The
// learner keeps its own copy, so writing into a returned record changes
// nothing.
func (l *OnlineLearner) Log() []ActionRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]ActionRecord, len(l.log))
	for i, entry := range l.log {
		out[i] = copyRecord(entry.record)
	}
	return out
}

// SetTargets declares what each named action should have produced. The target
// comes from the trainer side and never from a feedback, so a source that can
// score an answer still cannot dictate the answer. Targets are merged into what
// the learner already holds, and every one of them must name an action that has
// happened and have this individual's output width.
func (l *OnlineLearner) SetTargets(targets map[string][]float64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	owned := make(map[string][]float64, len(targets))
	for id, target := range targets {
		if _, ok := l.index[id]; !ok {
			return fmt.Errorf("learning: no action %q has been taken", id)
		}
		if len(target) != l.outputSize {
			return fmt.Errorf("learning: target for %q has width %d, the model produces %d", id, len(target), l.outputSize)
		}
		for i, v := range target {
			if !finite(v) {
				return fmt.Errorf("learning: target for %q is not finite at %d", id, i)
			}
		}
		owned[id] = append([]float64(nil), target...)
	}
	for id, target := range owned {
		l.targets[id] = target
	}
	return nil
}

// Act produces one answer and appends it to the append-only log. It runs the
// individual's ordinary Advance, so with local plasticity enabled the fast
// state keeps tracking activity under a closed gate and nothing learns here.
// The observation is validated and its experience is remembered, so feedback
// about a different experience cannot be attached to this action.
func (l *OnlineLearner) Act(ctx context.Context, obs signal.Observation, input [][]float64) (ActionRecord, error) {
	if err := obs.Validate(); err != nil {
		return ActionRecord{}, fmt.Errorf("learning: online observation: %w", err)
	}
	if len(input) == 0 {
		return ActionRecord{}, fmt.Errorf("learning: an action needs at least one input row")
	}
	if err := l.mu.LockContext(ctx); err != nil {
		return ActionRecord{}, err
	}
	defer l.mu.Unlock()
	if uint64(len(input)) > math.MaxUint64-l.step {
		return ActionRecord{}, fmt.Errorf("learning: online step counter overflow")
	}
	output, err := l.individual.Advance(ctx, input)
	if err != nil {
		return ActionRecord{}, err
	}
	version, err := parameterVersion(l.individual.Snapshot().Parameters)
	if err != nil {
		return ActionRecord{}, err
	}
	l.step += uint64(len(input))
	record := ActionRecord{
		ActionID:     fmt.Sprintf("act-%d", l.actions),
		Step:         l.step,
		Output:       copyRows(output),
		OutputHash:   rowMajorHash(output),
		ModelVersion: version,
	}
	l.actions++
	l.index[record.ActionID] = len(l.log)
	l.log = append(l.log, onlineEntry{
		record:       record,
		experienceID: obs.ExperienceID(),
		input:        copyRows(input),
		sources:      map[string]bool{},
	})
	return copyRecord(record), nil
}

// Receive queues feedback about an answer already given. It accepts only
// feedback whose action exists, whose experience is the one that action
// answered, that was produced no earlier than the action's own step, and whose
// source has not already been counted for that action. A feedback that claims
// to be available before it was produced is already refused by signal, so a
// forged earlier availability never reaches the queue.
//
// Receive changes no model state at all, in any policy, including the test
// mode: queueing is not consuming.
func (l *OnlineLearner) Receive(ctx context.Context, fb signal.Feedback) error {
	if err := fb.Validate(); err != nil {
		return fmt.Errorf("learning: online feedback: %w", err)
	}
	if err := l.mu.LockContext(ctx); err != nil {
		return err
	}
	defer l.mu.Unlock()
	position, ok := l.index[fb.ActionID()]
	if !ok {
		return fmt.Errorf("learning: feedback names action %q, which has not been taken", fb.ActionID())
	}
	entry := &l.log[position]
	if fb.ExperienceID() != entry.experienceID {
		return fmt.Errorf("learning: feedback about experience %q cannot describe action %q of experience %q", fb.ExperienceID(), fb.ActionID(), entry.experienceID)
	}
	if fb.AvailableAt().Unit != l.policy.ClockUnit {
		return fmt.Errorf("learning: feedback uses time unit %q; this learner runs on %q", fb.AvailableAt().Unit, l.policy.ClockUnit)
	}
	actionAt := signal.Timestamp{Value: int64(entry.record.Step), Unit: l.policy.ClockUnit}
	if entry.record.Step > math.MaxInt64 {
		return fmt.Errorf("learning: action step %d does not fit a timestamp", entry.record.Step)
	}
	if err := fb.ValidateForAction(actionAt); err != nil {
		return fmt.Errorf("learning: feedback for %q: %w", fb.ActionID(), err)
	}
	if entry.sources[fb.Source()] {
		return fmt.Errorf("learning: source %q has already been counted for action %q", fb.Source(), fb.ActionID())
	}
	entry.sources[fb.Source()] = true
	l.pending = append(l.pending, fb)
	return nil
}

// Update applies the policy to the feedback that has reached its declared
// arrival time at now. Feedback that has not arrived stays in the queue and is
// counted as deferred.
//
// In the test mode any arrived feedback makes the whole call fail with
// ErrEvaluateMode and change nothing, so a run declared as evaluation cannot
// learn by accident and no teacher-side source is ever consulted.
//
// Otherwise each arrived feedback is applied in the order it was received, the
// gradient half first and the plastic half second, and then leaves the queue.
// A gradient step runs only for an action that has a declared target, and only
// on every UpdateEvery such feedback. A plastic step maps the score through the
// reward mapper and runs the stored input again under that gate.
//
// An error stops the call at the feedback that failed: the feedback already
// consumed has left the queue and its updates stand, and the partial report is
// returned alongside the error, so a caller can see how far the update got
// rather than having to guess.
func (l *OnlineLearner) Update(ctx context.Context, now signal.Timestamp) (UpdateReport, error) {
	if err := now.Validate(); err != nil {
		return UpdateReport{}, fmt.Errorf("learning: online update time: %w", err)
	}
	if now.Unit != l.policy.ClockUnit {
		return UpdateReport{}, fmt.Errorf("learning: update time uses %q; this learner runs on %q", now.Unit, l.policy.ClockUnit)
	}
	if err := l.mu.LockContext(ctx); err != nil {
		return UpdateReport{}, err
	}
	defer l.mu.Unlock()
	available, err := signal.AvailableFeedback(l.pending, now)
	if err != nil {
		return UpdateReport{}, err
	}
	var report UpdateReport
	report.Deferred = len(l.pending) - len(available)
	if l.policy.Evaluate {
		if len(available) > 0 {
			return UpdateReport{}, ErrEvaluateMode
		}
		return report, nil
	}
	// The queue is rebuilt from the deferred feedback only after the whole
	// batch has been applied, so a failed step leaves exactly the feedback it
	// did not reach.
	applied := 0
	for _, fb := range available {
		if err = l.apply(ctx, fb, &report); err != nil {
			break
		}
		applied++
	}
	report.Consumed = applied
	l.pending = l.remainingFeedback(available[:applied])
	return report, err
}

// apply runs one feedback through the policy.
func (l *OnlineLearner) apply(ctx context.Context, fb signal.Feedback, report *UpdateReport) error {
	entry := &l.log[l.index[fb.ActionID()]]
	if l.policy.AllowGradient {
		if target, ok := l.targets[fb.ActionID()]; ok {
			l.eligible++
			if l.eligible%l.policy.updateEvery() == 0 {
				prediction, err := l.individual.episodePredict(ctx, entry.input)
				if err != nil {
					return err
				}
				effective, err := weightedTarget(prediction, target, fb.Score())
				if err != nil {
					return err
				}
				if _, err = l.individual.TrainEpisode(ctx, entry.input, effective); err != nil {
					return err
				}
				report.GradientSteps++
			}
		}
	}
	if !l.policy.AllowPlastic {
		return nil
	}
	applied, err := l.gate.Applied(fb.Score())
	if err != nil {
		return fmt.Errorf("learning: reward gate: %w", err)
	}
	if len(applied) == 0 {
		return fmt.Errorf("learning: the reward gate produced no release channel")
	}
	gate := make([]float64, len(entry.input))
	for i := range gate {
		gate[i] = applied[0]
	}
	if _, _, err = l.individual.AdvanceGated(ctx, entry.input, gate); err != nil {
		return err
	}
	report.PlasticSteps++
	return nil
}

// remainingFeedback keeps the feedback that was not consumed, in the order it
// was received. Feedback is compared by the action and source pair, which
// Receive already keeps unique.
func (l *OnlineLearner) remainingFeedback(consumed []signal.Feedback) []signal.Feedback {
	if len(consumed) == 0 {
		return l.pending
	}
	gone := make(map[string]bool, len(consumed))
	for _, fb := range consumed {
		gone[fb.ActionID()+"\x00"+fb.Source()] = true
	}
	kept := make([]signal.Feedback, 0, len(l.pending))
	for _, fb := range l.pending {
		if !gone[fb.ActionID()+"\x00"+fb.Source()] {
			kept = append(kept, fb)
		}
	}
	return kept
}

// weightedTarget is the one place the feedback score becomes a loss weight.
// The rule is w = clamp(score, 0, 1), applied by moving the target onto the
// line between the model's own episode prediction and the declared target:
//
//	effective[i] = prediction[i] + w * (target[i] - prediction[i])
//
// The trainer's squared-error gradient is proportional to the residual, so the
// residual this leaves is exactly w times the declared one and the gradient is
// exactly the w-weighted gradient. w = 1 is the unweighted update; w = 0 leaves
// a zero residual, so the step still runs the optimizer but moves no parameter
// that weight decay does not move.
func weightedTarget(prediction, target []float64, score float64) ([]float64, error) {
	if len(prediction) != len(target) {
		return nil, fmt.Errorf("learning: the model produced %d values, the target has %d", len(prediction), len(target))
	}
	weight := score
	if !finite(weight) || weight < 0 {
		weight = 0
	}
	if weight > 1 {
		weight = 1
	}
	effective := make([]float64, len(target))
	for i := range target {
		effective[i] = prediction[i] + weight*(target[i]-prediction[i])
		if !finite(effective[i]) {
			return nil, fmt.Errorf("learning: the weighted target is not finite at %d", i)
		}
	}
	return effective, nil
}

// episodePredict is the independent zero-state prediction a weighted target is
// built from: the same episode TrainEpisode would run, without changing
// anything. It takes the individual's own lock in the same order TrainEpisode
// does.
func (i *Individual) episodePredict(ctx context.Context, input [][]float64) ([]float64, error) {
	if i == nil || i.trainer == nil {
		return nil, fmt.Errorf("uninitialized individual")
	}
	if err := i.mu.LockContext(ctx); err != nil {
		return nil, err
	}
	defer i.mu.Unlock()
	return i.trainer.Predict(ctx, input)
}

// plasticityEnabled reports whether the local mechanism is on, so an online
// policy that needs it can refuse at construction rather than at the first
// gated advance.
func (i *Individual) plasticityEnabled() bool {
	if i == nil || i.trainer == nil {
		return false
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.plastic != nil
}

// parameterVersion is SHA-256 of the canonical parameter JSON.
func parameterVersion(p Parameters) (string, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// rowMajorHash is SHA-256 over the little-endian IEEE-754 bits of every value
// in row-major order.
func rowMajorHash(values [][]float64) string {
	digest := sha256.New()
	var buf [8]byte
	for _, row := range values {
		for _, v := range row {
			binary.LittleEndian.PutUint64(buf[:], math.Float64bits(v))
			digest.Write(buf[:])
		}
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func copyRecord(r ActionRecord) ActionRecord {
	r.Output = copyRows(r.Output)
	return r
}
