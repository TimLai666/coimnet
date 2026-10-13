// Public error, cancellation and retry controls for online individual operations.
package learning

import (
	"context"
	"errors"
	"math"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/signal"
)

const onlineBoundaryControlTimeout = 5 * time.Second

var onlineBoundaryControlInput = [][]float64{{.7}, {0}, {.2}}

func onlineBoundaryControlConfig() Config {
	return Config{
		Dynamics: dynamics.Config{
			Nodes:      2,
			Sources:    []int{0, 1},
			Targets:    []int{1, 0},
			DT:         .5,
			Activation: "tanh",
		},
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: []int{1},
	}
}

func onlineBoundaryControlParameters() Parameters {
	return Parameters{
		Core: dynamics.Parameters{
			Weights: []float64{.3, -.2},
			Bias:    []float64{.1, -.1},
			LogTau:  []float64{.1, .2},
		},
		Encoder: []float64{.5, .1},
		Readout: []float64{.8},
	}
}

func onlineBoundaryControlIndividual(t *testing.T) *Individual {
	t.Helper()
	individual, err := NewIndividual(
		onlineBoundaryControlConfig(), onlineBoundaryControlParameters(), DefaultOptions(), []float64{0, 0},
	)
	if err != nil {
		t.Fatal(err)
	}
	// Snapshot initializes the existing cancellable mutex token. The private
	// token is used below only to synchronize a public operation boundary.
	individual.Snapshot()
	return individual
}

func onlineBoundaryControlLearner(t *testing.T, individual *Individual, every uint64) *OnlineLearner {
	t.Helper()
	learner, err := NewOnlineLearner(individual, OnlinePolicy{
		AllowGradient: true,
		UpdateEvery:   every,
		ClockUnit:     signal.TimeUnitModelStep,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return learner
}

func onlineBoundaryControlObservation(t *testing.T) signal.Observation {
	t.Helper()
	observation, err := signal.NewObservation(signal.CurrentSchemaVersion(), "exp-1", "stream-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func onlineBoundaryControlFeedback(t *testing.T, actionID string, step int64, source string) signal.Feedback {
	t.Helper()
	version, err := signal.NewVersion(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	feedback, err := signal.NewFeedback(signal.FeedbackSpec{
		SchemaVersion: signal.CurrentSchemaVersion(),
		ExperienceID:  "exp-1",
		ActionID:      actionID,
		ProducedAt:    signal.Timestamp{Value: step, Unit: signal.TimeUnitModelStep},
		AvailableAt:   signal.Timestamp{Value: step, Unit: signal.TimeUnitModelStep},
		Source:        source,
		Score:         1,
		ModelVersion:  version,
	})
	if err != nil {
		t.Fatal(err)
	}
	return feedback
}

func onlineBoundaryControlAt(step uint64) signal.Timestamp {
	return signal.Timestamp{Value: int64(step), Unit: signal.TimeUnitModelStep}
}

func onlineBoundaryControlWait(t *testing.T, name string, ready <-chan struct{}) {
	t.Helper()
	timer := time.NewTimer(onlineBoundaryControlTimeout)
	defer timer.Stop()
	select {
	case <-ready:
	case <-timer.C:
		t.Fatalf("timeout waiting for %s", name)
	}
}

type onlineBoundaryControlUpdateOutcome struct {
	report UpdateReport
	err    error
}

type onlineBoundaryControlStepOutcome struct {
	result StepResult
	err    error
}

func onlineBoundaryControlStartUpdate(learner *OnlineLearner, ctx context.Context, now signal.Timestamp) (<-chan onlineBoundaryControlUpdateOutcome, <-chan struct{}) {
	result := make(chan onlineBoundaryControlUpdateOutcome, 1)
	done := make(chan struct{})
	go func() {
		report, err := learner.Update(ctx, now)
		result <- onlineBoundaryControlUpdateOutcome{report: report, err: err}
		close(done)
	}()
	return result, done
}

func onlineBoundaryControlStartTrainEpisode(individual *Individual, ctx context.Context) (<-chan onlineBoundaryControlStepOutcome, <-chan struct{}) {
	result := make(chan onlineBoundaryControlStepOutcome, 1)
	done := make(chan struct{})
	go func() {
		step, err := individual.TrainEpisode(ctx, onlineBoundaryControlInput, []float64{.4})
		result <- onlineBoundaryControlStepOutcome{result: step, err: err}
		close(done)
	}()
	return result, done
}

func onlineBoundaryControlReadUpdate(t *testing.T, result <-chan onlineBoundaryControlUpdateOutcome) onlineBoundaryControlUpdateOutcome {
	t.Helper()
	timer := time.NewTimer(onlineBoundaryControlTimeout)
	defer timer.Stop()
	select {
	case outcome := <-result:
		return outcome
	case <-timer.C:
		t.Fatalf("online Update did not return")
		return onlineBoundaryControlUpdateOutcome{}
	}
}

func onlineBoundaryControlReadStep(t *testing.T, result <-chan onlineBoundaryControlStepOutcome) onlineBoundaryControlStepOutcome {
	t.Helper()
	timer := time.NewTimer(onlineBoundaryControlTimeout)
	defer timer.Stop()
	select {
	case outcome := <-result:
		return outcome
	case <-timer.C:
		t.Fatalf("external TrainEpisode did not return")
		return onlineBoundaryControlStepOutcome{}
	}
}

// onlineBoundaryControlHoldContext pauses the first context check after the
// public online operation has acquired Individual.mu. It does not count Err
// calls. The existing private token is synchronization only; all assertions
// use public operation results, logs and snapshots.
type onlineBoundaryControlHoldContext struct {
	context.Context
	individual  *Individual
	entered     chan struct{}
	release     chan struct{}
	enterOnce   sync.Once
	releaseOnce sync.Once
}

func onlineBoundaryControlNewHoldContext(individual *Individual) *onlineBoundaryControlHoldContext {
	return &onlineBoundaryControlHoldContext{
		Context:    context.Background(),
		individual: individual,
		entered:    make(chan struct{}),
		release:    make(chan struct{}),
	}
}

func (c *onlineBoundaryControlHoldContext) Done() <-chan struct{} { return nil }

func (c *onlineBoundaryControlHoldContext) Err() error {
	if c.individual != nil && len(c.individual.mu.token) == 0 {
		c.enterOnce.Do(func() {
			close(c.entered)
			<-c.release
		})
	}
	return nil
}

func (c *onlineBoundaryControlHoldContext) open() {
	c.releaseOnce.Do(func() { close(c.release) })
}

// onlineBoundaryControlQueueContext acknowledges evaluation of Done, which
// proves the external operation reached cancellableMutex's select while the
// online operation still owns the token. Done acknowledgement is not treated
// as FIFO evidence.
type onlineBoundaryControlQueueContext struct {
	context.Context
	evaluated chan struct{}
	once      sync.Once
}

func onlineBoundaryControlNewQueueContext(ctx context.Context) *onlineBoundaryControlQueueContext {
	return &onlineBoundaryControlQueueContext{Context: ctx, evaluated: make(chan struct{})}
}

func (c *onlineBoundaryControlQueueContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.evaluated) })
	return c.Context.Done()
}

// TestOnlineIndividualBoundaryControlInvalidInputTarget proves that rejected
// input and target values do not append an action or mutate the individual,
// and that a corrected public target can complete the ordinary update path.
func TestOnlineIndividualBoundaryControlInvalidInputTarget(t *testing.T) {
	individual := onlineBoundaryControlIndividual(t)
	learner := onlineBoundaryControlLearner(t, individual, 1)
	observation := onlineBoundaryControlObservation(t)
	initial := individual.Snapshot()

	for name, input := range map[string][][]float64{
		"empty":       nil,
		"wrong_width": {{}},
	} {
		t.Run("input/"+name, func(t *testing.T) {
			if _, err := learner.Act(context.Background(), observation, input); err == nil {
				t.Fatal("invalid input was accepted")
			}
			if got := learner.Log(); len(got) != 0 {
				t.Fatalf("invalid input appended log entries: %+v", got)
			}
			if got := individual.Snapshot(); !reflect.DeepEqual(got, initial) {
				t.Fatalf("invalid input changed snapshot: got=%+v want=%+v", got, initial)
			}
		})
	}

	record, err := learner.Act(context.Background(), observation, onlineBoundaryControlInput)
	if err != nil {
		t.Fatalf("valid Act: %v", err)
	}
	beforeTarget := individual.Snapshot()
	for name, target := range map[string][]float64{
		"wrong_width": nil,
		"non_finite":  {math.NaN()},
	} {
		t.Run("target/"+name, func(t *testing.T) {
			if err := learner.SetTargets(map[string][]float64{record.ActionID: target}); err == nil {
				t.Fatal("invalid target was accepted")
			}
			if got := learner.Log(); len(got) != 1 || got[0].ActionID != record.ActionID {
				t.Fatalf("invalid target changed log: %+v", got)
			}
			if got := individual.Snapshot(); !reflect.DeepEqual(got, beforeTarget) {
				t.Fatalf("invalid target changed snapshot: got=%+v want=%+v", got, beforeTarget)
			}
		})
	}

	if err := learner.SetTargets(map[string][]float64{record.ActionID: {.4}}); err != nil {
		t.Fatalf("corrected target: %v", err)
	}
	if err := learner.Receive(context.Background(), onlineBoundaryControlFeedback(t, record.ActionID, int64(record.Step), "target-control")); err != nil {
		t.Fatalf("Receive after corrected target: %v", err)
	}
	report, err := learner.Update(context.Background(), onlineBoundaryControlAt(record.Step))
	if err != nil {
		t.Fatalf("Update after corrected target: %v", err)
	}
	if report.Consumed != 1 || report.GradientSteps != 1 || report.Deferred != 0 || report.PlasticSteps != 0 {
		t.Fatalf("corrected target report=%+v, want one consumed gradient update", report)
	}
	t.Logf("invalid input/target preserved snapshot and log; corrected target report=%+v log=%+v final_snapshot=%+v", report, learner.Log(), individual.Snapshot())
}

// TestOnlineIndividualBoundaryControlCanceledContextAndRetry covers nil and
// canceled public contexts. UpdateEvery=2 makes the first feedback an
// interval skip; a canceled second Update must preserve both its pending
// feedback and the interval position so one corrected-context retry applies
// exactly one gradient step.
func TestOnlineIndividualBoundaryControlCanceledContextAndRetry(t *testing.T) {
	individual := onlineBoundaryControlIndividual(t)
	learner := onlineBoundaryControlLearner(t, individual, 2)
	observation := onlineBoundaryControlObservation(t)

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	initial := individual.Snapshot()
	if _, err := learner.Act(canceled, observation, onlineBoundaryControlInput); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Act error=%v, want context.Canceled", err)
	}
	if got := learner.Log(); len(got) != 0 {
		t.Fatalf("canceled Act appended log entries: %+v", got)
	}
	if got := individual.Snapshot(); !reflect.DeepEqual(got, initial) {
		t.Fatalf("canceled Act changed snapshot: got=%+v want=%+v", got, initial)
	}

	first, err := learner.Act(context.Background(), observation, onlineBoundaryControlInput)
	if err != nil {
		t.Fatalf("first Act: %v", err)
	}
	if err := learner.SetTargets(map[string][]float64{first.ActionID: {.4}}); err != nil {
		t.Fatalf("first SetTargets: %v", err)
	}
	if err := learner.Receive(context.Background(), onlineBoundaryControlFeedback(t, first.ActionID, int64(first.Step), "first")); err != nil {
		t.Fatalf("first Receive: %v", err)
	}
	beforeSkip := individual.Snapshot()
	skip, err := learner.Update(context.Background(), onlineBoundaryControlAt(first.Step))
	if err != nil {
		t.Fatalf("UpdateEvery skip: %v", err)
	}
	if skip.Consumed != 1 || skip.Deferred != 0 || skip.GradientSteps != 0 || skip.PlasticSteps != 0 {
		t.Fatalf("UpdateEvery skip report=%+v", skip)
	}
	if got := individual.Snapshot(); !reflect.DeepEqual(got, beforeSkip) {
		t.Fatalf("interval skip changed snapshot: got=%+v want=%+v", got, beforeSkip)
	}

	second, err := learner.Act(context.Background(), observation, onlineBoundaryControlInput)
	if err != nil {
		t.Fatalf("second Act: %v", err)
	}
	if err := learner.SetTargets(map[string][]float64{second.ActionID: {.4}}); err != nil {
		t.Fatalf("second SetTargets: %v", err)
	}
	secondFeedback := onlineBoundaryControlFeedback(t, second.ActionID, int64(second.Step), "second")
	if err := learner.Receive(nil, secondFeedback); err == nil {
		t.Fatal("nil Receive context was accepted")
	}
	if err := learner.Receive(canceled, secondFeedback); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Receive error=%v, want context.Canceled", err)
	}
	if err := learner.Receive(context.Background(), secondFeedback); err != nil {
		t.Fatalf("second Receive: %v", err)
	}

	beforeFailure := individual.Snapshot()
	if report, err := learner.Update(nil, onlineBoundaryControlAt(second.Step)); err == nil {
		t.Fatalf("nil Update succeeded with report=%+v", report)
	} else if report != (UpdateReport{}) {
		t.Fatalf("nil Update report=%+v, want zero report", report)
	} else if got := individual.Snapshot(); !reflect.DeepEqual(got, beforeFailure) {
		t.Fatalf("nil Update changed snapshot: got=%+v want=%+v", got, beforeFailure)
	}
	canceledReport, err := learner.Update(canceled, onlineBoundaryControlAt(second.Step))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Update report=%+v error=%v, want context.Canceled", canceledReport, err)
	}
	if canceledReport != (UpdateReport{}) {
		t.Fatalf("canceled Update report=%+v, want zero report", canceledReport)
	}
	if got := individual.Snapshot(); !reflect.DeepEqual(got, beforeFailure) {
		t.Fatalf("canceled Update changed snapshot: got=%+v want=%+v", got, beforeFailure)
	}

	if _, err := individual.TrainEpisode(nil, onlineBoundaryControlInput, []float64{.4}); err == nil {
		t.Fatal("nil TrainEpisode context was accepted")
	}
	if _, err := individual.TrainEpisode(canceled, onlineBoundaryControlInput, []float64{.4}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled TrainEpisode error=%v, want context.Canceled", err)
	}
	if got := individual.Snapshot(); !reflect.DeepEqual(got, beforeFailure) {
		t.Fatalf("canceled TrainEpisode changed snapshot: got=%+v want=%+v", got, beforeFailure)
	}

	retry, err := learner.Update(context.Background(), onlineBoundaryControlAt(second.Step))
	if err != nil {
		t.Fatalf("corrected-context retry: %v", err)
	}
	if retry.Consumed != 1 || retry.Deferred != 0 || retry.GradientSteps != 1 || retry.PlasticSteps != 0 {
		t.Fatalf("corrected-context retry report=%+v, want one gradient update", retry)
	}
	afterRetry := individual.Snapshot()
	if afterRetry.Optimizer.Updates != beforeFailure.Optimizer.Updates+1 {
		t.Fatalf("corrected-context retry Updates=%d, want %d", afterRetry.Optimizer.Updates, beforeFailure.Optimizer.Updates+1)
	}
	t.Logf("skip=%+v canceled_report=%+v retry=%+v log=%+v snapshot_before_failure=%+v snapshot_after_retry=%+v pending_consumed_on_retry=true interval_preserved=true", skip, canceledReport, retry, learner.Log(), beforeFailure, afterRetry)
}

// onlineBoundaryControlQueueContext waits for the external public operation
// to reach the mutex select while Update owns Individual.mu. Cancellation is
// observed before that operation receives the token, so its zero result and
// final serial snapshot prove that it committed nothing.
func TestOnlineIndividualBoundaryControlWaitingTrainEpisodeCancellation(t *testing.T) {
	seed := onlineBoundaryControlIndividual(t)
	initial := seed.Snapshot()

	serialIndividual, err := RestoreIndividual(initial)
	if err != nil {
		t.Fatal(err)
	}
	serialLearner := onlineBoundaryControlLearner(t, serialIndividual, 1)
	serialAction, err := serialLearner.Act(context.Background(), onlineBoundaryControlObservation(t), onlineBoundaryControlInput)
	if err != nil {
		t.Fatalf("serial Act: %v", err)
	}
	if err := serialLearner.SetTargets(map[string][]float64{serialAction.ActionID: {.4}}); err != nil {
		t.Fatalf("serial SetTargets: %v", err)
	}
	if err := serialLearner.Receive(context.Background(), onlineBoundaryControlFeedback(t, serialAction.ActionID, int64(serialAction.Step), "serial")); err != nil {
		t.Fatalf("serial Receive: %v", err)
	}
	serialReport, err := serialLearner.Update(context.Background(), onlineBoundaryControlAt(serialAction.Step))
	if err != nil || serialReport.Consumed != 1 || serialReport.GradientSteps != 1 {
		t.Fatalf("serial Update report=%+v error=%v, want one gradient update", serialReport, err)
	}
	serialSnapshot := serialIndividual.Snapshot()

	victim, err := RestoreIndividual(initial)
	if err != nil {
		t.Fatal(err)
	}
	victimLearner := onlineBoundaryControlLearner(t, victim, 1)
	victimAction, err := victimLearner.Act(context.Background(), onlineBoundaryControlObservation(t), onlineBoundaryControlInput)
	if err != nil {
		t.Fatalf("victim Act: %v", err)
	}
	if err := victimLearner.SetTargets(map[string][]float64{victimAction.ActionID: {.4}}); err != nil {
		t.Fatalf("victim SetTargets: %v", err)
	}
	if err := victimLearner.Receive(context.Background(), onlineBoundaryControlFeedback(t, victimAction.ActionID, int64(victimAction.Step), "victim")); err != nil {
		t.Fatalf("victim Receive: %v", err)
	}

	hold := onlineBoundaryControlNewHoldContext(victim)
	var updateDone <-chan struct{}
	var externalDone <-chan struct{}
	var cancel context.CancelFunc
	defer func() {
		// Release every pause before joining either started goroutine. Completion
		// channels are independent of result channels, so cleanup never rereads
		// an already-consumed result after a Fatal assertion.
		hold.open()
		if cancel != nil {
			cancel()
		}
		for name, done := range map[string]<-chan struct{}{"Update": updateDone, "TrainEpisode": externalDone} {
			if done == nil {
				continue
			}
			timer := time.NewTimer(onlineBoundaryControlTimeout)
			select {
			case <-done:
				timer.Stop()
			case <-timer.C:
				t.Errorf("timeout joining started %s goroutine", name)
			}
		}
	}()

	updateResult, updateDone := onlineBoundaryControlStartUpdate(victimLearner, hold, onlineBoundaryControlAt(victimAction.Step))
	onlineBoundaryControlWait(t, "online Update to own Individual.mu", hold.entered)

	externalBase, externalCancel := context.WithCancel(context.Background())
	cancel = externalCancel
	queue := onlineBoundaryControlNewQueueContext(externalBase)
	externalResult, externalDone := onlineBoundaryControlStartTrainEpisode(victim, queue)
	onlineBoundaryControlWait(t, "external TrainEpisode mutex select", queue.evaluated)
	cancel()
	external := onlineBoundaryControlReadStep(t, externalResult)
	if !errors.Is(external.err, context.Canceled) {
		t.Fatalf("waiting external TrainEpisode error=%v, want context.Canceled", external.err)
	}
	if !reflect.DeepEqual(external.result, StepResult{}) {
		t.Fatalf("canceled waiting external TrainEpisode result=%+v, want zero result", external.result)
	}

	hold.open()
	update := onlineBoundaryControlReadUpdate(t, updateResult)
	if update.err != nil || update.report.Consumed != 1 || update.report.GradientSteps != 1 {
		t.Fatalf("online Update report=%+v error=%v, want one gradient update", update.report, update.err)
	}
	final := victim.Snapshot()
	if !reflect.DeepEqual(final, serialSnapshot) {
		t.Fatalf("canceled waiting TrainEpisode changed final snapshot: got=%+v want=%+v", final, serialSnapshot)
	}
	if got := victimLearner.Log(); len(got) != 1 || got[0].ActionID != victimAction.ActionID {
		t.Fatalf("final online log=%+v, want the single victim action", got)
	}
	t.Logf("external_wait_cancel_error=%v external_result=%+v update=%+v victim_log=%+v serial_snapshot=%+v final_snapshot=%+v snapshot_equal=true external_pending_commit=false", external.err, external.result, update.report, victimLearner.Log(), serialSnapshot, final)
}
