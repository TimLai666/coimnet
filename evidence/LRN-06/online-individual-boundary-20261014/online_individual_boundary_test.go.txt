// Online individual consistency regressions use private mutex state only for
// synchronization. Result oracles are public action records and complete snapshots.
package learning

import (
	"context"
	"math"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/signal"
)

const (
	onlineBoundaryExperience = "exp-1"
	onlineBoundaryRows       = 3
	onlineBoundaryTimeout    = 5 * time.Second
)

var onlineBoundaryInput = [][]float64{{.7}, {0}, {.2}}

// onlineBoundaryHoldContext pauses the first context.Err observed after the
// named Individual has consumed its private mutex token. The test initializes
// that token with Snapshot before starting the online operation. The context
// does not count Err calls and does not inspect core implementation frames.
type onlineBoundaryHoldContext struct {
	context.Context
	individual *Individual
	entered    chan struct{}
	release    chan struct{}
	enterOnce  sync.Once
	openOnce   sync.Once
}

func newOnlineBoundaryHoldContext(individual *Individual) *onlineBoundaryHoldContext {
	return &onlineBoundaryHoldContext{
		Context:    context.Background(),
		individual: individual,
		entered:    make(chan struct{}),
		release:    make(chan struct{}),
	}
}

func (c *onlineBoundaryHoldContext) Done() <-chan struct{} { return nil }

func (c *onlineBoundaryHoldContext) Err() error {
	if c.individual != nil && len(c.individual.mu.token) == 0 {
		c.enterOnce.Do(func() {
			close(c.entered)
			<-c.release
		})
	}
	return nil
}

func (c *onlineBoundaryHoldContext) open() {
	c.openOnce.Do(func() { close(c.release) })
}

// onlineBoundaryQueueContext records that its Done expression was evaluated,
// then records the first Err after the mutex token was received. The latter
// callback is after the receive in cancellableMutex.LockContext and blocks the
// external public mutator before it can change state. Done alone is never used
// as proof that the goroutine is parked.
type onlineBoundaryQueueContext struct {
	context.Context
	doneEvaluated atomic.Bool
	acquired      chan struct{}
	release       chan struct{}
	acquireOnce   sync.Once
	openOnce      sync.Once
}

func newOnlineBoundaryQueueContext() *onlineBoundaryQueueContext {
	return &onlineBoundaryQueueContext{
		Context:  context.Background(),
		acquired: make(chan struct{}),
		release:  make(chan struct{}),
	}
}

func (c *onlineBoundaryQueueContext) Done() <-chan struct{} {
	c.doneEvaluated.Store(true)
	return nil
}

func (c *onlineBoundaryQueueContext) Err() error {
	if c.doneEvaluated.Load() {
		c.acquireOnce.Do(func() {
			close(c.acquired)
			<-c.release
		})
	}
	return nil
}

func (c *onlineBoundaryQueueContext) open() {
	c.openOnce.Do(func() { close(c.release) })
}

func onlineBoundaryWait(t *testing.T, name string, done <-chan struct{}) {
	t.Helper()
	timer := time.NewTimer(onlineBoundaryTimeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		t.Fatalf("timeout waiting for %s", name)
	}
}

func onlineBoundaryWaitQueueDone(t *testing.T, c *onlineBoundaryQueueContext) {
	t.Helper()
	deadline := time.Now().Add(onlineBoundaryTimeout)
	for !c.doneEvaluated.Load() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for queue context Done evaluation")
		}
		runtime.Gosched()
	}
}

func onlineBoundaryWaitParkedTrainEpisode(t *testing.T, wrapper string) {
	t.Helper()
	deadline := time.Now().Add(onlineBoundaryTimeout)
	for {
		buffer := make([]byte, 1<<20)
		n := runtime.Stack(buffer, true)
		if onlineBoundaryParkedStack(string(buffer[:n]), wrapper) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s to park in cancellable mutex select", wrapper)
		}
		runtime.Gosched()
	}
}

func onlineBoundaryParkedStack(dump, wrapper string) bool {
	for _, block := range strings.Split(dump, "\n\n") {
		lines := strings.SplitN(block, "\n", 2)
		if len(lines) < 2 || !strings.HasPrefix(lines[0], "goroutine ") || !strings.Contains(lines[0], " [select]") {
			continue
		}
		if strings.Contains(block, wrapper) &&
			strings.Contains(block, "(*cancellableMutex).LockContext") &&
			strings.Contains(block, "(*Individual).TrainEpisode") {
			return true
		}
	}
	return false
}

type onlineBoundaryActionOutcome struct {
	record ActionRecord
	err    error
}

type onlineBoundaryStepOutcome struct {
	result StepResult
	err    error
}

type onlineBoundaryUpdateOutcome struct {
	report UpdateReport
	err    error
}

// onlineBoundaryGoroutines owns every pause context and started goroutine in
// one interleaving. Cleanup always opens all pause contexts before it waits for
// any completion channel, so a Fatal path cannot strand a goroutine behind a
// still-closed test gate. Completion is deliberately separate from the result
// channel: a result may already have been consumed when cleanup runs.
type onlineBoundaryGoroutines struct {
	releases    []func()
	completions []struct {
		name string
		done <-chan struct{}
	}
}

func onlineBoundaryOwnGoroutines(t *testing.T) *onlineBoundaryGoroutines {
	t.Helper()
	owned := &onlineBoundaryGoroutines{}
	t.Cleanup(func() {
		for _, release := range owned.releases {
			release()
		}
		for _, completion := range owned.completions {
			timer := time.NewTimer(onlineBoundaryTimeout)
			select {
			case <-completion.done:
				timer.Stop()
			case <-timer.C:
				t.Errorf("timeout joining started %s goroutine", completion.name)
			}
		}
	})
	return owned
}

func (g *onlineBoundaryGoroutines) release(open func()) {
	g.releases = append(g.releases, open)
}

func (g *onlineBoundaryGoroutines) join(name string, done <-chan struct{}) {
	g.completions = append(g.completions, struct {
		name string
		done <-chan struct{}
	}{name: name, done: done})
}

//go:noinline
func onlineBoundaryRunAct(learner *OnlineLearner, ctx context.Context, obs signal.Observation, out chan<- onlineBoundaryActionOutcome, complete chan<- struct{}) {
	defer close(complete)
	record, err := learner.Act(ctx, obs, onlineBoundaryInput)
	out <- onlineBoundaryActionOutcome{record: record, err: err}
}

//go:noinline
func onlineBoundaryRunUpdate(learner *OnlineLearner, ctx context.Context, now signal.Timestamp, out chan<- onlineBoundaryUpdateOutcome, complete chan<- struct{}) {
	defer close(complete)
	report, err := learner.Update(ctx, now)
	out <- onlineBoundaryUpdateOutcome{report: report, err: err}
}

//go:noinline
func onlineBoundaryRunExternal(individual *Individual, ctx context.Context, out chan<- onlineBoundaryStepOutcome, complete chan<- struct{}) {
	defer close(complete)
	result, err := individual.TrainEpisode(ctx, onlineBoundaryInput, []float64{.4})
	out <- onlineBoundaryStepOutcome{result: result, err: err}
}

func onlineBoundaryConfig() Config {
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

func onlineBoundaryParameters() Parameters {
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

func onlineBoundaryChemistry() modulation.ChemistryConfig {
	return modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{2}},
		Sources: []modulation.SourceSpec{{
			Kind: modulation.SourceExternalTimeline, Channel: 0,
			Timeline: &modulation.ExternalTimeline{ChannelCount: 1, Entries: []modulation.TimelineEntry{{Step: 1, Channel: 0, Rate: 1}}},
		}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{{
			Cells: []int{0}, Signal: "fixture", Channel: 0,
			Status: modulation.StatusHypothesized, Kd: .5, N: 1,
			Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "online-boundary/v1",
		}}},
		Effects: []modulation.Effect{{Kind: modulation.EffectSensitivity, Receptor: 0, GammaScale: 1}},
		Regions: modulation.RegionAssignment{NodeRegion: []int{0, 0}},
	}
}

func onlineBoundaryIndividualWith(t *testing.T, options Options, chemistry bool) *Individual {
	t.Helper()
	individual, err := NewIndividual(onlineBoundaryConfig(), onlineBoundaryParameters(), options, []float64{0, 0})
	if err != nil {
		t.Fatal(err)
	}
	if chemistry {
		if err := individual.EnableChemistry(onlineBoundaryChemistry()); err != nil {
			t.Fatalf("EnableChemistry: %v", err)
		}
	}
	// Snapshot initializes the private cancellable mutex token. The test only
	// reads its length in onlineBoundaryHoldContext.Err after this point.
	individual.Snapshot()
	return individual
}

func onlineBoundaryLearner(t *testing.T, individual *Individual) *OnlineLearner {
	t.Helper()
	learner, err := NewOnlineLearner(individual, OnlinePolicy{
		AllowGradient: true,
		ClockUnit:     signal.TimeUnitModelStep,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return learner
}

func onlineBoundaryObservation(t *testing.T) signal.Observation {
	t.Helper()
	observation, err := signal.NewObservation(signal.CurrentSchemaVersion(), onlineBoundaryExperience, "stream-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func onlineBoundaryFeedback(t *testing.T, actionID string, score float64) signal.Feedback {
	t.Helper()
	version, err := signal.NewVersion(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	feedback, err := signal.NewFeedback(signal.FeedbackSpec{
		SchemaVersion: signal.CurrentSchemaVersion(),
		ExperienceID:  onlineBoundaryExperience,
		ActionID:      actionID,
		ProducedAt:    signal.Timestamp{Value: onlineBoundaryRows, Unit: signal.TimeUnitModelStep},
		AvailableAt:   signal.Timestamp{Value: onlineBoundaryRows, Unit: signal.TimeUnitModelStep},
		Source:        "boundary-diagnostic",
		Score:         score,
		ModelVersion:  version,
	})
	if err != nil {
		t.Fatal(err)
	}
	return feedback
}

type onlineBoundaryMatrixCase struct {
	name       string
	chemistry  bool
	masks      bool
	accumulate int
	warmed     bool
}

func onlineBoundaryMatrix() []onlineBoundaryMatrixCase {
	return []onlineBoundaryMatrixCase{
		{name: "ordinary"},
		{name: "enabled_chemistry", chemistry: true},
		{name: "frozen_masks", masks: true},
		{name: "accumulate_2", accumulate: 2},
		{name: "warmed_adam_momentum", warmed: true},
	}
}

func onlineBoundaryMatrixOptions(testCase onlineBoundaryMatrixCase) Options {
	options := DefaultOptions()
	if testCase.masks {
		options.Masks = &UpdateMasks{Edges: []bool{true, false}, Nodes: []bool{false, true}}
	}
	if testCase.accumulate != 0 {
		options.AccumulateSteps = testCase.accumulate
	}
	return options
}

func onlineBoundaryAnyNonzero(values []float64) bool {
	for _, value := range values {
		if value != 0 {
			return true
		}
	}
	return false
}

func onlineBoundaryMatrixInitial(t *testing.T, testCase onlineBoundaryMatrixCase) IndividualSnapshot {
	t.Helper()
	individual := onlineBoundaryIndividualWith(t, onlineBoundaryMatrixOptions(testCase), testCase.chemistry)
	if testCase.warmed {
		result, err := individual.TrainEpisode(context.Background(), onlineBoundaryInput, []float64{.9})
		if err != nil {
			t.Fatalf("warm Adam episode: %v", err)
		}
		if !result.Applied || result.UpdateNorm <= 0 {
			t.Fatalf("warm Adam episode did not apply a parameter update: %+v", result)
		}
		snapshot := individual.Snapshot()
		if snapshot.Optimizer.Updates == 0 || !onlineBoundaryAnyNonzero(snapshot.Optimizer.State.First) || !onlineBoundaryAnyNonzero(snapshot.Optimizer.State.Second) {
			t.Fatalf("warm Adam episode did not leave momentum state: %+v", snapshot.Optimizer)
		}
	}
	return individual.Snapshot()
}

type onlineBoundaryScenario struct {
	individual *Individual
	learner    *OnlineLearner
	record     ActionRecord
	before     IndividualSnapshot
}

func onlineBoundaryPrepare(t *testing.T, initial IndividualSnapshot, score float64) onlineBoundaryScenario {
	t.Helper()
	individual, err := RestoreIndividual(initial)
	if err != nil {
		t.Fatal(err)
	}
	learner := onlineBoundaryLearner(t, individual)
	record, err := learner.Act(context.Background(), onlineBoundaryObservation(t), onlineBoundaryInput)
	if err != nil {
		t.Fatal(err)
	}
	if err := learner.SetTargets(map[string][]float64{record.ActionID: {.4}}); err != nil {
		t.Fatal(err)
	}
	if err := learner.Receive(context.Background(), onlineBoundaryFeedback(t, record.ActionID, score)); err != nil {
		t.Fatal(err)
	}
	return onlineBoundaryScenario{
		individual: individual,
		learner:    learner,
		record:     record,
		before:     individual.Snapshot(),
	}
}

func onlineBoundaryPrediction(t *testing.T, snapshot IndividualSnapshot) []float64 {
	t.Helper()
	network, err := NewNetwork(snapshot.Config)
	if err != nil {
		t.Fatal(err)
	}
	prediction, err := network.Predict(context.Background(), snapshot.Parameters, onlineBoundaryInput)
	if err != nil {
		t.Fatal(err)
	}
	return prediction
}

func onlineBoundaryWeightedTarget(prediction, target []float64, score float64) []float64 {
	weight := score
	if !math.IsNaN(weight) && weight >= 0 {
		if weight > 1 {
			weight = 1
		}
	} else {
		weight = 0
	}
	effective := make([]float64, len(target))
	for i := range target {
		effective[i] = prediction[i] + weight*(target[i]-prediction[i])
	}
	return effective
}

func onlineBoundaryAwaitAction(t *testing.T, done <-chan onlineBoundaryActionOutcome) onlineBoundaryActionOutcome {
	t.Helper()
	timer := time.NewTimer(onlineBoundaryTimeout)
	defer timer.Stop()
	select {
	case outcome := <-done:
		return outcome
	case <-timer.C:
		t.Fatalf("online Act did not return before external mutex release")
		return onlineBoundaryActionOutcome{}
	}
}

func onlineBoundaryAwaitUpdate(t *testing.T, done <-chan onlineBoundaryUpdateOutcome) onlineBoundaryUpdateOutcome {
	t.Helper()
	timer := time.NewTimer(onlineBoundaryTimeout)
	defer timer.Stop()
	select {
	case outcome := <-done:
		return outcome
	case <-timer.C:
		t.Fatalf("online Update did not return before external mutex release")
		return onlineBoundaryUpdateOutcome{}
	}
}

func onlineBoundaryAwaitStep(t *testing.T, done <-chan onlineBoundaryStepOutcome) onlineBoundaryStepOutcome {
	t.Helper()
	timer := time.NewTimer(onlineBoundaryTimeout)
	defer timer.Stop()
	select {
	case outcome := <-done:
		return outcome
	case <-timer.C:
		t.Fatalf("external TrainEpisode did not return after mutex release")
		return onlineBoundaryStepOutcome{}
	}
}

func onlineBoundaryActionFieldsEqual(a, b ActionRecord) bool {
	return reflect.DeepEqual(a, b)
}

// TestOnlineIndividualBoundaryAct holds the private Individual token while a
// public Act reaches its first lock-held context check. Only then does the test
// start the public external TrainEpisode. The queue stack probe proves the
// unique external wrapper is waiting in LockContext; it does not claim FIFO.
// The online Act must return before the queue context is released. On old code,
// Advance unlocks before Snapshot and the blocked external operation prevents
// Act from returning. On repaired code, the output and ModelVersion are both
// produced under the same Individual operation boundary.
func onlineBoundaryActCase(t *testing.T, initial IndividualSnapshot) {
	ctx := context.Background()

	control, err := RestoreIndividual(initial)
	if err != nil {
		t.Fatal(err)
	}
	controlLearner := onlineBoundaryLearner(t, control)
	controlRecord, err := controlLearner.Act(ctx, onlineBoundaryObservation(t), onlineBoundaryInput)
	if err != nil {
		t.Fatal(err)
	}
	controlAfterAdvance := control.Snapshot()
	controlExternal, err := control.TrainEpisode(ctx, onlineBoundaryInput, []float64{.4})
	if err != nil {
		t.Fatal(err)
	}
	controlAfter := control.Snapshot()
	if !controlExternal.Applied || controlExternal.UpdateNorm <= 0 {
		t.Fatalf("serial external control did not apply: %+v", controlExternal)
	}

	victim, err := RestoreIndividual(initial)
	if err != nil {
		t.Fatal(err)
	}
	if before := victim.Snapshot(); !reflect.DeepEqual(before, initial) {
		t.Fatal("victim fixture did not match serial control initial state")
	}
	learner := onlineBoundaryLearner(t, victim)
	hold := newOnlineBoundaryHoldContext(victim)
	queue := newOnlineBoundaryQueueContext()
	owned := onlineBoundaryOwnGoroutines(t)
	owned.release(hold.open)
	owned.release(queue.open)
	actDone := make(chan onlineBoundaryActionOutcome, 1)
	externalDone := make(chan onlineBoundaryStepOutcome, 1)
	actComplete := make(chan struct{})
	owned.join("Act", actComplete)

	go onlineBoundaryRunAct(learner, hold, onlineBoundaryObservation(t), actDone, actComplete)
	onlineBoundaryWait(t, "Act to hold Individual.mu", hold.entered)

	externalComplete := make(chan struct{})
	owned.join("external TrainEpisode", externalComplete)
	go onlineBoundaryRunExternal(victim, queue, externalDone, externalComplete)
	onlineBoundaryWaitQueueDone(t, queue)
	onlineBoundaryWaitParkedTrainEpisode(t, "onlineBoundaryRunExternal")
	t.Logf("act queue sync doneEvaluated=true parked=true wrapper=onlineBoundaryRunExternal")

	hold.open()
	act := onlineBoundaryAwaitAction(t, actDone)
	if act.err != nil {
		t.Fatalf("Act: %v", act.err)
	}
	if !onlineBoundaryActionFieldsEqual(act.record, controlRecord) {
		t.Fatalf("Act fields changed across serialized control: actual={output:%v hash:%s version:%s} control={output:%v hash:%s version:%s}", act.record.Output, act.record.OutputHash, act.record.ModelVersion, controlRecord.Output, controlRecord.OutputHash, controlRecord.ModelVersion)
	}

	// Act has now returned and released its Individual lock. The queue context
	// owns the external operation only until its post-receive Err returns.
	onlineBoundaryWait(t, "external TrainEpisode token receive", queue.acquired)
	queue.open()
	external := onlineBoundaryAwaitStep(t, externalDone)
	if external.err != nil {
		t.Fatalf("external TrainEpisode: %v", external.err)
	}
	if !external.result.Applied || external.result.UpdateNorm <= 0 {
		t.Fatalf("external TrainEpisode did not apply: %+v", external.result)
	}
	actualAfter := victim.Snapshot()
	if !reflect.DeepEqual(actualAfter, controlAfter) {
		t.Fatalf("final IndividualSnapshot differs from Advance-then-external control: actual={parameters:%v optimizer:%+v neural:%+v} control={parameters:%v optimizer:%+v neural:%+v}", actualAfter.Parameters, actualAfter.Optimizer, actualAfter.Neural, controlAfter.Parameters, controlAfter.Optimizer, controlAfter.Neural)
	}
	if !reflect.DeepEqual(controlAfterAdvance.Neural, actualAfter.Neural) {
		t.Fatalf("serial control lost neural state: afterAdvance=%+v final=%+v", controlAfterAdvance.Neural, actualAfter.Neural)
	}
	t.Logf("Act record={output:%v hash:%s version:%s} external={applied:%t gradient_norm:%.17g update_norm:%.17g updates:%d} finalSnapshotEqual=true neuralIncluded=true", act.record.Output, act.record.OutputHash, act.record.ModelVersion, external.result.Applied, external.result.GradientNorm, external.result.UpdateNorm, external.result.Updates)
}

func TestOnlineIndividualBoundaryAct(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		chemistry bool
	}{
		{name: "ordinary"},
		{name: "enabled_chemistry", chemistry: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			initial := onlineBoundaryIndividualWith(t, DefaultOptions(), testCase.chemistry).Snapshot()
			onlineBoundaryActCase(t, initial)
		})
	}
}

// TestOnlineIndividualBoundaryWeighted covers the public Act -> SetTargets ->
// Receive -> Update path for score zero, half and one. The serial oracle is
// always weighted Update first and external TrainEpisode second. A stale-P0
// external-first control is retained only as a negative control: it is required
// to differ only when the complete snapshots make that distinction observable.
// The online operation must return before the queued external operation is
// released, so old code fails by blocking at its second Individual lock.
func onlineBoundaryWeightedCase(t *testing.T, testCase onlineBoundaryMatrixCase, initial IndividualSnapshot, score float64) {
	ctx := context.Background()
	fresh := onlineBoundaryPrepare(t, initial, score)
	freshReport, err := fresh.learner.Update(ctx, signal.Timestamp{Value: onlineBoundaryRows, Unit: signal.TimeUnitModelStep})
	if err != nil {
		t.Fatalf("fresh weighted Update: %v", err)
	}
	expectedGradientSteps := 1
	if testCase.accumulate > 1 {
		expectedGradientSteps = 0
	}
	if freshReport.Consumed != 1 || freshReport.GradientSteps != expectedGradientSteps {
		t.Fatalf("fresh weighted report=%+v, want one consumed feedback and %d gradient steps", freshReport, expectedGradientSteps)
	}
	freshAfterWeighted := fresh.individual.Snapshot()
	if !reflect.DeepEqual(freshAfterWeighted.Neural, fresh.before.Neural) || !reflect.DeepEqual(freshAfterWeighted.Chemical, fresh.before.Chemical) {
		t.Fatal("weighted training changed the persistent neural or chemical boundary")
	}
	freshExternal, err := fresh.individual.TrainEpisode(ctx, onlineBoundaryInput, []float64{.4})
	if err != nil {
		t.Fatalf("fresh serial external TrainEpisode: %v", err)
	}
	if !freshExternal.Applied || freshExternal.UpdateNorm <= 0 {
		t.Fatalf("fresh serial external update did not apply: %+v", freshExternal)
	}
	freshAfter := fresh.individual.Snapshot()

	actual := onlineBoundaryPrepare(t, initial, score)
	if !reflect.DeepEqual(actual.before, fresh.before) {
		t.Fatal("actual and fresh weighted controls did not start from the same complete snapshot")
	}
	if !onlineBoundaryActionFieldsEqual(actual.record, fresh.record) {
		t.Fatalf("actual and fresh Act fields differ: actual=%+v fresh=%+v", actual.record, fresh.record)
	}

	var zeroResult StepResult
	var zeroAfter IndividualSnapshot
	var zeroParametersEqual, zeroAdamStateEqual bool
	if score == 0 && !testCase.warmed && !testCase.chemistry {
		// This is the only direct zero-norm oracle. It is valid because the
		// ordinary episode prediction is public-network equivalent; chemistry
		// uses the serial Update snapshot oracle below instead.
		prediction := onlineBoundaryPrediction(t, actual.before)
		zeroTwin, err := RestoreIndividual(actual.before)
		if err != nil {
			t.Fatal(err)
		}
		zeroResult, err = zeroTwin.TrainEpisode(ctx, onlineBoundaryInput, prediction)
		if err != nil {
			t.Fatalf("fresh score-zero basis: %v", err)
		}
		zeroAfter = zeroTwin.Snapshot()
		zeroParametersEqual = reflect.DeepEqual(zeroAfter.Parameters, actual.before.Parameters)
		zeroAdamStateEqual = reflect.DeepEqual(zeroAfter.Optimizer.State, actual.before.Optimizer.State)
		if zeroResult.GradientNorm != 0 || zeroResult.UpdateNorm != 0 || !zeroParametersEqual {
			t.Fatalf("fresh score-zero result=%+v parameters_equal=%t adam_state_equal=%t", zeroResult, zeroParametersEqual, zeroAdamStateEqual)
		}
		if !reflect.DeepEqual(zeroAfter.Neural, actual.before.Neural) {
			t.Fatal("fresh score-zero basis changed persistent neural state")
		}
		wantUpdates := actual.before.Optimizer.Updates
		wantApplied := testCase.accumulate <= 1
		if wantApplied {
			wantUpdates++
		}
		if zeroResult.Applied != wantApplied || zeroAfter.Optimizer.Updates != wantUpdates || zeroAfter.Optimizer.Episodes != actual.before.Optimizer.Episodes+1 {
			t.Fatalf("fresh score-zero counters/result=%+v, want applied=%t updates=%d episodes=%d", zeroAfter.Optimizer, wantApplied, wantUpdates, actual.before.Optimizer.Episodes+1)
		}
	}

	// Negative control: external first, followed by a stale target made from
	// P0. It is retained for ordinary cases only, because a chemical episode
	// prediction must never be replaced with an ordinary Network prediction.
	var staleAfter IndividualSnapshot
	var staleExternal, staleExtra StepResult
	staleDistinguishable := false
	if !testCase.chemistry {
		prediction := onlineBoundaryPrediction(t, actual.before)
		stale, err := RestoreIndividual(actual.before)
		if err != nil {
			t.Fatal(err)
		}
		staleExternal, err = stale.TrainEpisode(ctx, onlineBoundaryInput, []float64{.4})
		if err != nil {
			t.Fatalf("stale control external TrainEpisode: %v", err)
		}
		staleExtra, err = stale.TrainEpisode(ctx, onlineBoundaryInput, onlineBoundaryWeightedTarget(prediction, []float64{.4}, score))
		if err != nil {
			t.Fatalf("stale control weighted TrainEpisode: %v", err)
		}
		staleAfter = stale.Snapshot()
		staleDistinguishable = !reflect.DeepEqual(staleAfter, freshAfter)
	}

	hold := newOnlineBoundaryHoldContext(actual.individual)
	queue := newOnlineBoundaryQueueContext()
	owned := onlineBoundaryOwnGoroutines(t)
	owned.release(hold.open)
	owned.release(queue.open)
	updateDone := make(chan onlineBoundaryUpdateOutcome, 1)
	externalDone := make(chan onlineBoundaryStepOutcome, 1)
	updateComplete := make(chan struct{})
	owned.join("Update", updateComplete)
	go onlineBoundaryRunUpdate(actual.learner, hold, signal.Timestamp{Value: onlineBoundaryRows, Unit: signal.TimeUnitModelStep}, updateDone, updateComplete)
	onlineBoundaryWait(t, "Update to hold Individual.mu", hold.entered)
	externalComplete := make(chan struct{})
	owned.join("external TrainEpisode", externalComplete)
	go onlineBoundaryRunExternal(actual.individual, queue, externalDone, externalComplete)
	onlineBoundaryWaitQueueDone(t, queue)
	onlineBoundaryWaitParkedTrainEpisode(t, "onlineBoundaryRunExternal")
	t.Logf("weighted queue sync case=%s score=%g doneEvaluated=true parked=true wrapper=onlineBoundaryRunExternal", testCase.name, score)

	hold.open()
	actualUpdate := onlineBoundaryAwaitUpdate(t, updateDone)
	if actualUpdate.err != nil {
		t.Fatalf("interleaved weighted Update: %v", actualUpdate.err)
	}
	if actualUpdate.report.Consumed != 1 || actualUpdate.report.GradientSteps != expectedGradientSteps {
		t.Fatalf("interleaved weighted report=%+v, want one consumed feedback and %d gradient steps", actualUpdate.report, expectedGradientSteps)
	}

	onlineBoundaryWait(t, "external TrainEpisode token receive", queue.acquired)
	queue.open()
	external := onlineBoundaryAwaitStep(t, externalDone)
	if external.err != nil {
		t.Fatalf("interleaved external TrainEpisode: %v", external.err)
	}
	if !external.result.Applied || external.result.UpdateNorm <= 0 {
		t.Fatalf("interleaved external update did not apply: %+v", external.result)
	}
	actualAfter := actual.individual.Snapshot()
	actualFreshEqual := reflect.DeepEqual(actualAfter, freshAfter)
	actualStaleEqual := !testCase.chemistry && reflect.DeepEqual(actualAfter, staleAfter)
	if !actualFreshEqual {
		t.Fatalf("case=%s score=%g complete snapshot differs from weighted-first control: actual={parameters:%v optimizer:%+v neural:%+v chemical:%v} fresh={parameters:%v optimizer:%+v neural:%+v chemical:%v}", testCase.name, score, actualAfter.Parameters, actualAfter.Optimizer, actualAfter.Neural, actualAfter.Chemical, freshAfter.Parameters, freshAfter.Optimizer, freshAfter.Neural, freshAfter.Chemical)
	}
	if staleDistinguishable && actualStaleEqual {
		t.Fatalf("case=%s score=%g actual snapshot matched distinguishable stale-P0 negative control", testCase.name, score)
	}
	t.Logf("weighted case=%s score=%g chemistry=%t masks=%t accumulate=%d warmed=%t freshReport={consumed:%d gradient_steps:%d} actualReport={consumed:%d gradient_steps:%d} zero={result:%+v parameters_equal:%t adam_state_equal:%t} stale={external:%+v extra:%+v distinguishable:%t} actual={external:%+v fresh_equal:%t stale_equal:%t}", testCase.name, score, testCase.chemistry, testCase.masks, testCase.accumulate, testCase.warmed, freshReport.Consumed, freshReport.GradientSteps, actualUpdate.report.Consumed, actualUpdate.report.GradientSteps, zeroResult, zeroParametersEqual, zeroAdamStateEqual, staleExternal, staleExtra, staleDistinguishable, external.result, actualFreshEqual, actualStaleEqual)
}

func TestOnlineIndividualBoundaryWeighted(t *testing.T) {
	for _, testCase := range onlineBoundaryMatrix() {
		t.Run(testCase.name, func(t *testing.T) {
			initial := onlineBoundaryMatrixInitial(t, testCase)
			for _, scoreCase := range []struct {
				name  string
				score float64
			}{
				{name: "score_0", score: 0},
				{name: "score_half", score: .5},
				{name: "score_1", score: 1},
			} {
				t.Run(scoreCase.name, func(t *testing.T) {
					onlineBoundaryWeightedCase(t, testCase, initial, scoreCase.score)
				})
			}
		})
	}
}
