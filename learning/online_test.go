package learning_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/plasticity"
	"github.com/TimLai666/coimnet/signal"
)

// onlineInput is three model steps, so an action's own step is three and a
// feedback produced at step two is a forged one.
func onlineInput() [][]float64 { return [][]float64{{.7}, {0}, {.2}} }

func onlinePolicy() learning.OnlinePolicy {
	return learning.OnlinePolicy{AllowGradient: true, ClockUnit: signal.TimeUnitModelStep}
}

func onlineIndividual(t *testing.T) *learning.Individual {
	t.Helper()
	n, p := network(t)
	c := n.Config()
	a, err := learning.NewIndividual(c, p, learning.DefaultOptions(), make([]float64, c.Dynamics.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func onlinePlasticIndividual(t *testing.T) *learning.Individual {
	t.Helper()
	a := onlineIndividual(t)
	rule := plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 8, WMin: .0625}
	if err := a.EnablePlasticity(plasticity.Config{Rule: rule, Edges: []int{0, 1}}); err != nil {
		t.Fatal(err)
	}
	return a
}

// rewardGate is the ticket-20 reward mapper wired in as the online learner's
// gate, so the online loop opens the gate by exactly the declared mapping rule
// rather than by a second rule of its own.
func rewardGate() learning.RewardGate {
	mapper := &modulation.RewardMapper{Baseline: modulation.BaselineNone, Kind: modulation.KindRelu}
	return learning.RewardGateFunc(func(score float64) ([]float64, error) {
		record, err := mapper.Map(score)
		return record.Applied, err
	})
}

// brokenGate is a mapper the modulation package itself refuses, so the learner
// has to find that out at construction rather than at the first update.
func brokenGate() learning.RewardGate {
	mapper := &modulation.RewardMapper{Baseline: "guess", Kind: modulation.KindRelu}
	return learning.RewardGateFunc(func(score float64) ([]float64, error) {
		record, err := mapper.Map(score)
		return record.Applied, err
	})
}

func newLearner(t *testing.T, a *learning.Individual, p learning.OnlinePolicy, gate learning.RewardGate) *learning.OnlineLearner {
	t.Helper()
	l, err := learning.NewOnlineLearner(a, p, gate)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func observation(t *testing.T) signal.Observation {
	t.Helper()
	o, err := signal.NewObservation(signal.CurrentSchemaVersion(), "exp-1", "stream-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func feedback(t *testing.T, actionID, source string, produced, available int64, score float64) signal.Feedback {
	t.Helper()
	version, err := signal.NewVersion(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	fb, err := signal.NewFeedback(signal.FeedbackSpec{
		SchemaVersion: signal.CurrentSchemaVersion(), ExperienceID: "exp-1", ActionID: actionID,
		ProducedAt:  signal.Timestamp{Value: produced, Unit: signal.TimeUnitModelStep},
		AvailableAt: signal.Timestamp{Value: available, Unit: signal.TimeUnitModelStep},
		Source:      source, Score: score, ModelVersion: version,
	})
	if err != nil {
		t.Fatal(err)
	}
	return fb
}

func at(value int64) signal.Timestamp {
	return signal.Timestamp{Value: value, Unit: signal.TimeUnitModelStep}
}

func act(t *testing.T, l *learning.OnlineLearner, input [][]float64) learning.ActionRecord {
	t.Helper()
	record, err := l.Act(context.Background(), observation(t), input)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func update(t *testing.T, l *learning.OnlineLearner, now int64) learning.UpdateReport {
	t.Helper()
	report, err := l.Update(context.Background(), at(now))
	if err != nil {
		t.Fatal(err)
	}
	return report
}

// TestNewOnlineLearnerRejectsAnUndeclaredLoop keeps every knob a declared one:
// the loop runs on the model-step clock, a plastic policy needs the mapper that
// turns a score into a gate, and a mapper without that policy would be a second
// knob with no effect.
func TestNewOnlineLearnerRejectsAnUndeclaredLoop(t *testing.T) {
	if _, err := learning.NewOnlineLearner(nil, onlinePolicy(), nil); err == nil {
		t.Fatal("a learner was built without an individual")
	}
	for name, build := range map[string]func(*testing.T) (*learning.Individual, learning.OnlinePolicy, learning.RewardGate){
		"no clock unit": func(t *testing.T) (*learning.Individual, learning.OnlinePolicy, learning.RewardGate) {
			p := onlinePolicy()
			p.ClockUnit = ""
			return onlineIndividual(t), p, nil
		},
		"wall clock unit": func(t *testing.T) (*learning.Individual, learning.OnlinePolicy, learning.RewardGate) {
			p := onlinePolicy()
			p.ClockUnit = signal.TimeUnitMilliseconds
			return onlineIndividual(t), p, nil
		},
		"no mechanism": func(t *testing.T) (*learning.Individual, learning.OnlinePolicy, learning.RewardGate) {
			return onlineIndividual(t), learning.OnlinePolicy{ClockUnit: signal.TimeUnitModelStep}, nil
		},
		"plastic without a gate": func(t *testing.T) (*learning.Individual, learning.OnlinePolicy, learning.RewardGate) {
			return onlinePlasticIndividual(t), learning.OnlinePolicy{AllowPlastic: true, ClockUnit: signal.TimeUnitModelStep}, nil
		},
		"gate without a plastic policy": func(t *testing.T) (*learning.Individual, learning.OnlinePolicy, learning.RewardGate) {
			return onlineIndividual(t), onlinePolicy(), rewardGate()
		},
		"plastic without the mechanism": func(t *testing.T) (*learning.Individual, learning.OnlinePolicy, learning.RewardGate) {
			return onlineIndividual(t), learning.OnlinePolicy{AllowPlastic: true, ClockUnit: signal.TimeUnitModelStep}, rewardGate()
		},
		"invalid mapper": func(t *testing.T) (*learning.Individual, learning.OnlinePolicy, learning.RewardGate) {
			p := learning.OnlinePolicy{AllowPlastic: true, ClockUnit: signal.TimeUnitModelStep}
			return onlinePlasticIndividual(t), p, brokenGate()
		},
	} {
		t.Run(name, func(t *testing.T) {
			a, p, mapper := build(t)
			if _, err := learning.NewOnlineLearner(a, p, mapper); err == nil {
				t.Fatal("an undeclared online policy was accepted")
			}
		})
	}
}

// TestOnlineOrderIsEnforced is the Act, Receive, Update separation: feedback for
// an action that has not happened is refused, and feedback that has not reached
// its declared arrival time is deferred rather than consumed.
func TestOnlineOrderIsEnforced(t *testing.T) {
	a := onlineIndividual(t)
	l := newLearner(t, a, onlinePolicy(), nil)
	if err := l.Receive(context.Background(), feedback(t, "act-0", "teacher", 3, 4, 1)); err == nil {
		t.Fatal("feedback was accepted for an action that never happened")
	}
	record := act(t, l, onlineInput())
	if record.ActionID != "act-0" || record.Step != 3 {
		t.Fatalf("first action = %q at step %d, want act-0 at step 3", record.ActionID, record.Step)
	}
	if err := l.SetTargets(map[string][]float64{record.ActionID: {.4}}); err != nil {
		t.Fatal(err)
	}
	if err := l.Receive(context.Background(), feedback(t, record.ActionID, "teacher", 3, 10, 1)); err != nil {
		t.Fatal(err)
	}
	before := a.Snapshot()
	deferred := update(t, l, 5)
	if deferred.Consumed != 0 || deferred.Deferred != 1 || deferred.GradientSteps != 0 {
		t.Fatalf("an update before the arrival time reported %+v", deferred)
	}
	if !reflect.DeepEqual(a.Snapshot(), before) {
		t.Fatal("an update that consumed nothing changed the individual")
	}
	arrived := update(t, l, 10)
	if arrived.Consumed != 1 || arrived.Deferred != 0 || arrived.GradientSteps != 1 {
		t.Fatalf("an update at the arrival time reported %+v", arrived)
	}
	// Consumed feedback leaves the queue, so a second update finds nothing.
	again := update(t, l, 20)
	if again.Consumed != 0 || again.Deferred != 0 || again.GradientSteps != 0 {
		t.Fatalf("consumed feedback was replayed: %+v", again)
	}
}

// TestReceiveRejectsForgedTimesAndDuplicates keeps the arrival rule honest: a
// feedback cannot claim to predate the action it describes, and one source
// cannot be counted twice for one action.
func TestReceiveRejectsForgedTimesAndDuplicates(t *testing.T) {
	l := newLearner(t, onlineIndividual(t), onlinePolicy(), nil)
	record := act(t, l, onlineInput())
	for name, fb := range map[string]signal.Feedback{
		"produced before the action": feedback(t, record.ActionID, "teacher", 2, 9, 1),
		"unknown action":             feedback(t, "act-9", "teacher", 3, 9, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if err := l.Receive(context.Background(), fb); err == nil {
				t.Fatal("a forged feedback was queued")
			}
		})
	}
	t.Run("available before produced", func(t *testing.T) {
		// The other half of the forged-time rule is enforced one layer down: a
		// feedback that claims to be available before it was produced cannot be
		// built at all, so it can never reach Receive.
		version, err := signal.NewVersion(1, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = signal.NewFeedback(signal.FeedbackSpec{
			SchemaVersion: signal.CurrentSchemaVersion(), ExperienceID: "exp-1", ActionID: record.ActionID,
			ProducedAt: at(9), AvailableAt: at(3), Source: "teacher", Score: 1, ModelVersion: version,
		}); err == nil {
			t.Fatal("a feedback available before it was produced was built")
		}
	})
	t.Run("another clock", func(t *testing.T) {
		version, err := signal.NewVersion(1, 0)
		if err != nil {
			t.Fatal(err)
		}
		other, err := signal.NewFeedback(signal.FeedbackSpec{
			SchemaVersion: signal.CurrentSchemaVersion(), ExperienceID: "exp-1", ActionID: record.ActionID,
			ProducedAt:  signal.Timestamp{Value: 3, Unit: signal.TimeUnitMilliseconds},
			AvailableAt: signal.Timestamp{Value: 9, Unit: signal.TimeUnitMilliseconds},
			Source:      "teacher", Score: 1, ModelVersion: version,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := l.Receive(context.Background(), other); err == nil {
			t.Fatal("feedback on another clock was queued")
		}
	})
	t.Run("another experience", func(t *testing.T) {
		version, err := signal.NewVersion(1, 0)
		if err != nil {
			t.Fatal(err)
		}
		other, err := signal.NewFeedback(signal.FeedbackSpec{
			SchemaVersion: signal.CurrentSchemaVersion(), ExperienceID: "exp-2", ActionID: record.ActionID,
			ProducedAt: at(3), AvailableAt: at(9), Source: "teacher", Score: 1, ModelVersion: version,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := l.Receive(context.Background(), other); err == nil {
			t.Fatal("feedback about another experience was queued")
		}
	})
	if err := l.Receive(context.Background(), feedback(t, record.ActionID, "teacher", 3, 9, 1)); err != nil {
		t.Fatal(err)
	}
	if err := l.Receive(context.Background(), feedback(t, record.ActionID, "teacher", 4, 9, .5)); err == nil {
		t.Fatal("a second feedback from the same source was queued for the same action")
	}
	if err := l.Receive(context.Background(), feedback(t, record.ActionID, "critic", 4, 9, .5)); err != nil {
		t.Fatalf("a different source was refused: %v", err)
	}
	if report := update(t, l, 9); report.Consumed != 2 {
		t.Fatalf("two sources produced %+v", report)
	}
}

// TestOnlineOutputIsNeverRewritten is the no-write-back rule: once an answer has
// been given, nothing that arrives later changes it or its fingerprint.
func TestOnlineOutputIsNeverRewritten(t *testing.T) {
	l := newLearner(t, onlineIndividual(t), onlinePolicy(), nil)
	record := act(t, l, onlineInput())
	if record.OutputHash != rowsHash(record.Output) {
		t.Fatalf("the record does not describe its own output: %s", record.OutputHash)
	}
	hash, output := record.OutputHash, copyRowsForTest(record.Output)
	if err := l.SetTargets(map[string][]float64{record.ActionID: {.4}}); err != nil {
		t.Fatal(err)
	}
	if err := l.Receive(context.Background(), feedback(t, record.ActionID, "teacher", 5, 5, 1)); err != nil {
		t.Fatal(err)
	}
	if report := update(t, l, 5); report.GradientSteps != 1 {
		t.Fatalf("the update did not run: %+v", report)
	}
	log := l.Log()
	if len(log) != 1 {
		t.Fatalf("the log holds %d records", len(log))
	}
	if log[0].OutputHash != hash || !reflect.DeepEqual(log[0].Output, output) {
		t.Fatalf("the logged answer was rewritten: %+v", log[0])
	}
	// The log is the learner's own copy: writing into a record a caller holds
	// cannot reach it.
	record.Output[0][0] = 99
	log[0].Output[0][0] = 77
	if again := l.Log(); again[0].Output[0][0] != output[0][0] || again[0].OutputHash != hash {
		t.Fatalf("the log aliased a caller's record: %+v", again[0])
	}
}

// TestOnlineEvaluateModeRefusesUpdates pins the test mode: available feedback
// makes Update refuse outright rather than apply part of a policy, and nothing
// about the individual moves.
func TestOnlineEvaluateModeRefusesUpdates(t *testing.T) {
	a := onlinePlasticIndividual(t)
	p := learning.OnlinePolicy{AllowGradient: true, AllowPlastic: true, Evaluate: true, ClockUnit: signal.TimeUnitModelStep}
	l := newLearner(t, a, p, rewardGate())
	record := act(t, l, onlineInput())
	if err := l.SetTargets(map[string][]float64{record.ActionID: {.4}}); err != nil {
		t.Fatal(err)
	}
	if err := l.Receive(context.Background(), feedback(t, record.ActionID, "teacher", 3, 8, 1)); err != nil {
		t.Fatal(err)
	}
	before := a.Snapshot()
	// Feedback that has not arrived leaves nothing to refuse.
	deferred, err := l.Update(context.Background(), at(4))
	if err != nil {
		t.Fatalf("evaluate mode refused an update with nothing available: %v", err)
	}
	if deferred.Consumed != 0 || deferred.Deferred != 1 {
		t.Fatalf("deferred update reported %+v", deferred)
	}
	if _, err = l.Update(context.Background(), at(8)); !errors.Is(err, learning.ErrEvaluateMode) {
		t.Fatalf("evaluate mode returned %v, want ErrEvaluateMode", err)
	}
	if !reflect.DeepEqual(a.Snapshot(), before) {
		t.Fatal("a refused update changed the individual")
	}
	// The refusal changes nothing, so the feedback is still queued and a
	// learner without the test mode would still consume it.
	if report, err := l.Update(context.Background(), at(8)); !errors.Is(err, learning.ErrEvaluateMode) || report.Consumed != 0 {
		t.Fatalf("the refused feedback left the queue: %+v %v", report, err)
	}
}

// TestGradientPathChangesOnlyBaseParameters and its plastic twin below are the
// separation the two mechanisms have to keep: a gradient step never touches the
// fast state, and a plastic step never touches the base parameters.
func TestGradientPathChangesOnlyBaseParameters(t *testing.T) {
	a := onlinePlasticIndividual(t)
	l := newLearner(t, a, onlinePolicy(), nil)
	record := act(t, l, onlineInput())
	if err := l.SetTargets(map[string][]float64{record.ActionID: {.4}}); err != nil {
		t.Fatal(err)
	}
	if err := l.Receive(context.Background(), feedback(t, record.ActionID, "teacher", 3, 3, 1)); err != nil {
		t.Fatal(err)
	}
	before := a.Snapshot()
	if report := update(t, l, 3); report.GradientSteps != 1 || report.PlasticSteps != 0 {
		t.Fatalf("gradient update reported %+v", report)
	}
	after := a.Snapshot()
	if reflect.DeepEqual(after.Parameters, before.Parameters) {
		t.Fatal("a gradient step left the base parameters unchanged")
	}
	if !reflect.DeepEqual(after.Plastic, before.Plastic) {
		t.Fatal("a gradient step changed the fast state")
	}
	if !reflect.DeepEqual(after.Neural, before.Neural) {
		t.Fatal("a gradient step changed the persistent neural state")
	}
}

func TestPlasticPathChangesOnlyTheFastState(t *testing.T) {
	a := onlinePlasticIndividual(t)
	p := learning.OnlinePolicy{AllowPlastic: true, ClockUnit: signal.TimeUnitModelStep}
	l := newLearner(t, a, p, rewardGate())
	record := act(t, l, onlineInput())
	if err := l.Receive(context.Background(), feedback(t, record.ActionID, "critic", 3, 3, 1)); err != nil {
		t.Fatal(err)
	}
	before := a.Snapshot()
	if report := update(t, l, 3); report.PlasticSteps != 1 || report.GradientSteps != 0 {
		t.Fatalf("plastic update reported %+v", report)
	}
	after := a.Snapshot()
	if !reflect.DeepEqual(after.Parameters, before.Parameters) {
		t.Fatal("a plastic step changed the base parameters")
	}
	if reflect.DeepEqual(after.Plastic, before.Plastic) {
		t.Fatal("a plastic step left the fast state unchanged")
	}
	if after.Optimizer.Updates != before.Optimizer.Updates {
		t.Fatal("a plastic step ran the optimizer")
	}
}

// TestLossWeightScalesTheTargetResidual is the declared mapping from a feedback
// score to a loss weight: w = clamp(score, 0, 1), applied by moving the target
// to y + w*(t - y), so the residual the trainer sees is exactly w times the
// declared one. The check is against a twin individual trained directly on the
// hand-computed effective target.
func TestLossWeightScalesTheTargetResidual(t *testing.T) {
	const declared = .4
	for name, score := range map[string]float64{
		"half":            .5,
		"clamped up":      2,
		"clamped to zero": -1,
		"whole":           1,
	} {
		t.Run(name, func(t *testing.T) {
			weight := math.Max(0, math.Min(1, score))
			a, twin := onlineIndividual(t), onlineIndividual(t)
			l := newLearner(t, a, onlinePolicy(), nil)
			record := act(t, l, onlineInput())
			// The twin follows the same persistent trajectory, so both start
			// the update from identical state.
			if _, err := twin.Advance(context.Background(), onlineInput()); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(twin.Snapshot(), a.Snapshot()) {
				t.Fatal("the twin did not follow the same trajectory")
			}
			if err := l.SetTargets(map[string][]float64{record.ActionID: {declared}}); err != nil {
				t.Fatal(err)
			}
			if err := l.Receive(context.Background(), feedback(t, record.ActionID, "teacher", 3, 3, score)); err != nil {
				t.Fatal(err)
			}
			// The episode prediction the effective target is built from.
			snapshot := a.Snapshot()
			n, err := learning.NewNetwork(snapshot.Config)
			if err != nil {
				t.Fatal(err)
			}
			prediction, err := n.Predict(context.Background(), snapshot.Parameters, onlineInput())
			if err != nil {
				t.Fatal(err)
			}
			effective := []float64{prediction[0] + weight*(declared-prediction[0])}
			if _, err = twin.TrainEpisode(context.Background(), onlineInput(), effective); err != nil {
				t.Fatal(err)
			}
			if report := update(t, l, 3); report.GradientSteps != 1 {
				t.Fatalf("the update did not run: %+v", report)
			}
			if !reflect.DeepEqual(a.Snapshot().Parameters, twin.Snapshot().Parameters) {
				t.Fatalf("the weighted update did not match the hand-computed effective target %v", effective)
			}
			if weight == 0 && !reflect.DeepEqual(a.Snapshot().Parameters, snapshot.Parameters) {
				t.Fatal("a zero loss weight still moved the parameters")
			}
			if weight > 0 && reflect.DeepEqual(a.Snapshot().Parameters, snapshot.Parameters) {
				t.Fatal("a positive loss weight left the parameters unchanged")
			}
		})
	}
}

// TestUpdateEveryCountsAvailableFeedback pins the counter: only every
// UpdateEvery gradient-eligible feedback runs a step, and the feedbacks in
// between are still consumed.
func TestUpdateEveryCountsAvailableFeedback(t *testing.T) {
	p := onlinePolicy()
	p.UpdateEvery = 2
	l := newLearner(t, onlineIndividual(t), p, nil)
	for i := range 3 {
		record := act(t, l, onlineInput())
		if err := l.SetTargets(map[string][]float64{record.ActionID: {.4}}); err != nil {
			t.Fatal(err)
		}
		if err := l.Receive(context.Background(), feedback(t, record.ActionID, "teacher", int64(3*(i+1)), int64(3*(i+1)), 1)); err != nil {
			t.Fatal(err)
		}
	}
	report := update(t, l, 9)
	if report.Consumed != 3 || report.GradientSteps != 1 {
		t.Fatalf("three feedbacks under UpdateEvery 2 reported %+v", report)
	}
	// An action without a target is consumed but is not gradient-eligible, so
	// it does not move the counter.
	record := act(t, l, onlineInput())
	if err := l.Receive(context.Background(), feedback(t, record.ActionID, "teacher", 12, 12, 1)); err != nil {
		t.Fatal(err)
	}
	if again := update(t, l, 12); again.Consumed != 1 || again.GradientSteps != 0 {
		t.Fatalf("an action without a target reported %+v", again)
	}
}

// TestActReflectsTheUpdate is the other half of the no-write-back rule: the
// answer already given does not change, and the next answer does.
func TestActReflectsTheUpdate(t *testing.T) {
	updated := newLearner(t, onlineIndividual(t), onlinePolicy(), nil)
	control := newLearner(t, onlineIndividual(t), onlinePolicy(), nil)
	first, controlFirst := act(t, updated, onlineInput()), act(t, control, onlineInput())
	if !reflect.DeepEqual(first.Output, controlFirst.Output) {
		t.Fatal("two identical learners answered differently")
	}
	if err := updated.SetTargets(map[string][]float64{first.ActionID: {.4}}); err != nil {
		t.Fatal(err)
	}
	if err := updated.Receive(context.Background(), feedback(t, first.ActionID, "teacher", 3, 3, 1)); err != nil {
		t.Fatal(err)
	}
	if report := update(t, updated, 3); report.GradientSteps != 1 {
		t.Fatalf("the update did not run: %+v", report)
	}
	second, controlSecond := act(t, updated, onlineInput()), act(t, control, onlineInput())
	if reflect.DeepEqual(second.Output, controlSecond.Output) {
		t.Fatal("the answer after an update is the one an unupdated learner gives")
	}
	if second.ModelVersion == first.ModelVersion {
		t.Fatal("the model version did not change with the parameters")
	}
	if controlSecond.ModelVersion != controlFirst.ModelVersion {
		t.Fatal("the model version changed without an update")
	}
	if second.ActionID != "act-1" || second.Step != 6 {
		t.Fatalf("second action = %q at step %d, want act-1 at step 6", second.ActionID, second.Step)
	}
}

// TestActionRecordNamesTheModelThatProducedIt pins the two fingerprints against
// their declared preimages.
func TestActionRecordNamesTheModelThatProducedIt(t *testing.T) {
	a := onlineIndividual(t)
	l := newLearner(t, a, onlinePolicy(), nil)
	record := act(t, l, onlineInput())
	parameters, err := json.Marshal(a.Snapshot().Parameters)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(parameters)
	if record.ModelVersion != hex.EncodeToString(sum[:]) {
		t.Fatalf("model version = %s, want the parameter fingerprint %s", record.ModelVersion, hex.EncodeToString(sum[:]))
	}
	if record.OutputHash != rowsHash(record.Output) {
		t.Fatalf("output hash = %s", record.OutputHash)
	}
	if len(record.Output) != len(onlineInput()) {
		t.Fatalf("the record holds %d output rows for %d input rows", len(record.Output), len(onlineInput()))
	}
}

// TestSetTargetsRefusesWhatTheTrainerCannotUse keeps the target path on the
// trainer side and checked: targets belong to actions that happened, and they
// have the model's own output shape.
func TestSetTargetsRefusesWhatTheTrainerCannotUse(t *testing.T) {
	l := newLearner(t, onlineIndividual(t), onlinePolicy(), nil)
	record := act(t, l, onlineInput())
	for name, targets := range map[string]map[string][]float64{
		"unknown action": {"act-9": {.4}},
		"wrong width":    {record.ActionID: {.4, .5}},
		"empty target":   {record.ActionID: {}},
		"non-finite":     {record.ActionID: {math.NaN()}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := l.SetTargets(targets); err == nil {
				t.Fatal("a target the trainer cannot use was accepted")
			}
		})
	}
	if err := l.SetTargets(map[string][]float64{record.ActionID: {.4}}); err != nil {
		t.Fatal(err)
	}
}

func rowsHash(rows [][]float64) string {
	digest := sha256.New()
	var buf [8]byte
	for _, row := range rows {
		for _, v := range row {
			for i := range 8 {
				buf[i] = byte(math.Float64bits(v) >> (8 * i))
			}
			digest.Write(buf[:])
		}
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func copyRowsForTest(rows [][]float64) [][]float64 {
	owned := make([][]float64, len(rows))
	for i, row := range rows {
		owned[i] = append([]float64(nil), row...)
	}
	return owned
}
