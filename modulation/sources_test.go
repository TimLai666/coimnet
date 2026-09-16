package modulation

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/signal"
	"github.com/TimLai666/coimnet/simulate"
)

// stepFeedback builds valid feedback on the model-step clock a release step is
// counted in. ProducedAt is never after AvailableAt.
func stepFeedback(t *testing.T, producedAt, availableAt int64) signal.Feedback {
	t.Helper()
	f, err := signal.NewFeedback(signal.FeedbackSpec{
		SchemaVersion: signal.CurrentSchemaVersion(),
		ExperienceID:  "exp-1",
		ActionID:      "action-1",
		ProducedAt:    signal.Timestamp{Value: producedAt, Unit: signal.TimeUnitModelStep},
		AvailableAt:   signal.Timestamp{Value: availableAt, Unit: signal.TimeUnitModelStep},
		Source:        "teacher",
		Score:         0.5,
		ModelVersion:  signal.CurrentSchemaVersion(),
	})
	if err != nil {
		t.Fatalf("build feedback: %v", err)
	}
	return f
}

// fixtureObservation is a valid observation. No source reads a target: there is
// no target type in this package, and an observation carries only signals.
func fixtureObservation(t *testing.T) *signal.Observation {
	t.Helper()
	s, err := signal.NewSignal(signal.SignalSpec{
		SchemaVersion:  signal.CurrentSchemaVersion(),
		ExperienceID:   "exp-1",
		StreamID:       "stream-1",
		Channel:        "vision",
		Kind:           signal.KindContinuous,
		Start:          signal.Timestamp{Value: 0, Unit: signal.TimeUnitModelStep},
		Duration:       1,
		Shape:          []int{2},
		Values:         []float64{0.25, 0.75},
		Unit:           signal.UnitNormalized,
		EncoderVersion: signal.CurrentSchemaVersion(),
		SourceSequence: 1,
	})
	if err != nil {
		t.Fatalf("build signal: %v", err)
	}
	o, err := signal.NewObservation(signal.CurrentSchemaVersion(), "exp-1", "stream-1", []signal.Signal{s})
	if err != nil {
		t.Fatalf("build observation: %v", err)
	}
	return &o
}

// fixtureSources returns one valid instance of every implemented source,
// together with a context each of them accepts at step 2.
func fixtureSources(t *testing.T) (map[string]Source, SourceContext) {
	t.Helper()
	sources := map[string]Source{
		"external_timeline": ExternalTimeline{ChannelCount: 2, Entries: []TimelineEntry{{Step: 0, Channel: 0, Rate: 0.5}, {Step: 2, Channel: 1, Rate: 2}}},
		"neural_activity":   NeuralActivity{Set: fixtureSet(t, "alpn"), Gain: 0.5, Channel: 1},
		"internal_resource": InternalResource{Resource: "energy", Coefficient: 2, Threshold: 0.25, Channel: 0},
		"replay":            Replay{Trace: [][]float64{{0, 1.5}, {2, 0}, {0.25, 0.25}}},
	}
	c := SourceContext{
		Observation: fixtureObservation(t),
		Activity:    []float64{10, 0.25, 0.75},
		Feedback:    []signal.Feedback{stepFeedback(t, 0, 2)},
		Resources:   map[string]float64{"energy": 1.25},
	}
	return sources, c
}

// Hand table, ExternalTimeline{ChannelCount: 2} with the entries below:
//
//	entry (step 0, channel 0, 0.5), (step 0, channel 1, 1.25),
//	      (step 2, channel 1, 2), (step 3, channel 0, 0)
//
//	step 0 -> [0.5, 1.25]   both entries of the step
//	step 1 -> [0, 0]        no entry at all
//	step 2 -> [0, 2]        one entry, the other channel stays 0
//	step 3 -> [0, 0]        a declared zero is still zero
//	step 9 -> [0, 0]        past the last entry, not an error
func TestExternalTimelineReleasesTheDeclaredRateAndZeroElsewhere(t *testing.T) {
	timeline := ExternalTimeline{ChannelCount: 2, Entries: []TimelineEntry{
		{Step: 0, Channel: 0, Rate: 0.5},
		{Step: 0, Channel: 1, Rate: 1.25},
		{Step: 2, Channel: 1, Rate: 2},
		{Step: 3, Channel: 0, Rate: 0},
	}}
	if timeline.Channels() != 2 {
		t.Fatalf("Channels() = %d, want 2", timeline.Channels())
	}
	want := map[uint64][]float64{
		0: {0.5, 1.25},
		1: {0, 0},
		2: {0, 2},
		3: {0, 0},
		9: {0, 0},
	}
	for step, expected := range want {
		got, err := timeline.Release(step, SourceContext{})
		if err != nil {
			t.Fatalf("Release(%d) error = %v", step, err)
		}
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("Release(%d) = %v, want %v", step, got, expected)
		}
	}
	// The declaration is read, never consumed: releasing again reproduces the
	// same row and the caller's entries are untouched.
	entries := append([]TimelineEntry(nil), timeline.Entries...)
	if _, err := timeline.Release(0, SourceContext{}); err != nil {
		t.Fatalf("second Release(0) error = %v", err)
	}
	if !reflect.DeepEqual(timeline.Entries, entries) {
		t.Fatal("Release mutated the declared timeline")
	}
}

func TestExternalTimelineRejectsAnInvalidDeclaration(t *testing.T) {
	for name, timeline := range map[string]ExternalTimeline{
		"no channel":        {ChannelCount: 0, Entries: []TimelineEntry{{Step: 0, Channel: 0, Rate: 1}}},
		"step regression":   {ChannelCount: 1, Entries: []TimelineEntry{{Step: 2, Channel: 0, Rate: 1}, {Step: 1, Channel: 0, Rate: 1}}},
		"channel too large": {ChannelCount: 1, Entries: []TimelineEntry{{Step: 0, Channel: 1, Rate: 1}}},
		"channel negative":  {ChannelCount: 1, Entries: []TimelineEntry{{Step: 0, Channel: -1, Rate: 1}}},
		"negative rate":     {ChannelCount: 1, Entries: []TimelineEntry{{Step: 0, Channel: 0, Rate: -1}}},
		"not finite":        {ChannelCount: 1, Entries: []TimelineEntry{{Step: 0, Channel: 0, Rate: math.Inf(1)}}},
		"nan":               {ChannelCount: 1, Entries: []TimelineEntry{{Step: 0, Channel: 0, Rate: math.NaN()}}},
		"duplicate entry":   {ChannelCount: 1, Entries: []TimelineEntry{{Step: 1, Channel: 0, Rate: 1}, {Step: 1, Channel: 0, Rate: 2}}},
	} {
		if _, err := timeline.Release(1, SourceContext{}); err == nil {
			t.Errorf("%s: Release() error = nil", name)
		}
	}
}

// Hand table, NeuralActivity{Set: alpn (nodes 1 and 2), Gain: 0.5, Channel: 1}:
//
//	activity [10, 0.25, 0.75] -> mean (0.25+0.75)/2 = 0.5, x 0.5 = 0.25 -> [0, 0.25]
//	activity [10, -1, -3]     -> mean (-1-3)/2 = -2,  x 0.5 = -1 -> clamped [0, 0]
//	activity [10, 0, 4]       -> mean (0+4)/2 = 2,    x 0.5 = 1    -> [0, 1]
//
// Node 0 is outside the set and is never read, which is what "the set must be
// explicit" buys: activity[0] is large and changes nothing.
func TestNeuralActivityAveragesTheResolvedSetTimesGain(t *testing.T) {
	source := NeuralActivity{Set: fixtureSet(t, "alpn"), Gain: 0.5, Channel: 1}
	if source.Channels() != 2 {
		t.Fatalf("Channels() = %d, want 2 (channel 1 plus the zero channels below it)", source.Channels())
	}
	for _, row := range []struct {
		activity []float64
		want     []float64
	}{
		{[]float64{10, 0.25, 0.75}, []float64{0, 0.25}},
		{[]float64{10, -1, -3}, []float64{0, 0}},
		{[]float64{10, 0, 4}, []float64{0, 1}},
	} {
		got, err := source.Release(7, SourceContext{Activity: row.activity})
		if err != nil {
			t.Fatalf("Release(activity %v) error = %v", row.activity, err)
		}
		if !reflect.DeepEqual(got, row.want) {
			t.Fatalf("Release(activity %v) = %v, want %v", row.activity, got, row.want)
		}
	}
}

func TestNeuralActivityRequiresAnExplicitSetAndUsableActivity(t *testing.T) {
	set := fixtureSet(t, "alpn")
	for name, c := range map[string]struct {
		source   NeuralActivity
		activity []float64
	}{
		"no activity":         {NeuralActivity{Set: set, Gain: 1, Channel: 0}, nil},
		"activity too short":  {NeuralActivity{Set: set, Gain: 1, Channel: 0}, []float64{1, 2}},
		"activity not finite": {NeuralActivity{Set: set, Gain: 1, Channel: 0}, []float64{1, math.NaN(), 3}},
		"unresolved set":      {NeuralActivity{Gain: 1, Channel: 0}, []float64{1, 2, 3}},
		"declared empty set":  {NeuralActivity{Set: fixtureSet(t, "none"), Gain: 1, Channel: 0}, []float64{1, 2, 3}},
		"gain not finite":     {NeuralActivity{Set: set, Gain: math.Inf(1), Channel: 0}, []float64{1, 2, 3}},
		"negative channel":    {NeuralActivity{Set: set, Gain: 1, Channel: -1}, []float64{1, 2, 3}},
	} {
		if _, err := c.source.Release(0, SourceContext{Activity: c.activity}); err == nil {
			t.Errorf("%s: Release() error = nil", name)
		}
	}
}

// Hand table, InternalResource{Resource: "energy", Coefficient: 2,
// Threshold: 0.25, Channel: 0}: rate = 2 * max(energy - 0.25, 0).
//
//	energy 1.25 -> 2 * 1    = 2     -> [2]
//	energy 0.75 -> 2 * 0.5  = 1     -> [1]
//	energy 0.25 -> 2 * 0    = 0     -> [0]
//	energy 0.1  -> 2 * 0    = 0     -> [0]  (below threshold, never negative)
func TestInternalResourceReleasesTheDeclaredExcessRule(t *testing.T) {
	source := InternalResource{Resource: "energy", Coefficient: 2, Threshold: 0.25, Channel: 0}
	if source.Channels() != 1 {
		t.Fatalf("Channels() = %d, want 1", source.Channels())
	}
	for energy, want := range map[float64]float64{1.25: 2, 0.75: 1, 0.25: 0, 0.1: 0} {
		got, err := source.Release(0, SourceContext{Resources: map[string]float64{"energy": energy}})
		if err != nil {
			t.Fatalf("Release(energy %v) error = %v", energy, err)
		}
		if !reflect.DeepEqual(got, []float64{want}) {
			t.Fatalf("Release(energy %v) = %v, want [%v]", energy, got, want)
		}
	}
}

func TestInternalResourceRequiresTheDeclaredResource(t *testing.T) {
	source := InternalResource{Resource: "energy", Coefficient: 2, Threshold: 0.25}
	for name, resources := range map[string]map[string]float64{
		"no resources":   nil,
		"other resource": {"dopamine": 1},
		"not finite":     {"energy": math.Inf(1)},
		"nan":            {"energy": math.NaN()},
	} {
		if _, err := source.Release(0, SourceContext{Resources: resources}); err == nil {
			t.Errorf("%s: Release() error = nil", name)
		}
	}
	for name, source := range map[string]InternalResource{
		"no resource name":       {Coefficient: 1},
		"negative coefficient":   {Resource: "energy", Coefficient: -1},
		"coefficient not finite": {Resource: "energy", Coefficient: math.NaN()},
		"threshold not finite":   {Resource: "energy", Coefficient: 1, Threshold: math.Inf(-1)},
		"negative channel":       {Resource: "energy", Coefficient: 1, Channel: -1},
	} {
		if _, err := source.Release(0, SourceContext{Resources: map[string]float64{"energy": 1}}); err == nil {
			t.Errorf("%s: Release() error = nil", name)
		}
	}
}

// Hand table, Replay{Trace: [[0, 1.5], [2, 0], [0.25, 0.25]]}:
//
//	step 0 -> [0, 1.5]
//	step 1 -> [2, 0]
//	step 2 -> [0.25, 0.25]
//	step 3 -> error: the recording ends
func TestReplayReturnsARecordedRowAndStopsAtTheEnd(t *testing.T) {
	source := Replay{Trace: [][]float64{{0, 1.5}, {2, 0}, {0.25, 0.25}}}
	if source.Channels() != 2 {
		t.Fatalf("Channels() = %d, want 2", source.Channels())
	}
	for step, want := range [][]float64{{0, 1.5}, {2, 0}, {0.25, 0.25}} {
		got, err := source.Release(uint64(step), SourceContext{})
		if err != nil {
			t.Fatalf("Release(%d) error = %v", step, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Release(%d) = %v, want %v", step, got, want)
		}
		got[0] = 99
	}
	if _, err := source.Release(3, SourceContext{}); err == nil {
		t.Fatal("Release(3) error = nil past the end of the trace")
	}
	// The rows handed out above were overwritten; the recording is unchanged.
	if !reflect.DeepEqual(source.Trace, [][]float64{{0, 1.5}, {2, 0}, {0.25, 0.25}}) {
		t.Fatalf("Release handed out the stored row: trace is now %v", source.Trace)
	}
	for name, source := range map[string]Replay{
		"empty trace":    {},
		"empty row":      {Trace: [][]float64{{}}},
		"ragged trace":   {Trace: [][]float64{{0, 1}, {0}}},
		"negative value": {Trace: [][]float64{{0, -1}}},
		"not finite":     {Trace: [][]float64{{0, math.Inf(1)}}},
	} {
		if _, err := source.Release(0, SourceContext{}); err == nil {
			t.Errorf("%s: Release() error = nil", name)
		}
	}
}

// Replay must not be able to reach a network, a file or a clock: it has exactly
// one field, the recorded trace it replays.
func TestReplayHasNoFieldThatCanReachOutsideTheTrace(t *testing.T) {
	replay := reflect.TypeOf(Replay{})
	if replay.NumField() != 1 {
		t.Fatalf("Replay has %d fields, want 1", replay.NumField())
	}
	field := replay.Field(0)
	if field.Name != "Trace" || field.Type.String() != "[][]float64" {
		t.Fatalf("Replay field 0 is %s %s, want Trace [][]float64", field.Name, field.Type)
	}
	if methods := reflect.PointerTo(replay).NumMethod(); methods != 2 {
		t.Fatalf("Replay has %d methods, want only Channels and Release", methods)
	}
}

// Feedback is filtered by signal.AvailableFeedback before it reaches a source,
// but a source is not allowed to trust that: it re-checks every feedback
// against its own step and fails on one that has not arrived.
func TestEverySourceRejectsFeedbackThatHasNotArrived(t *testing.T) {
	sources, c := fixtureSources(t)
	for name, source := range sources {
		early := c
		early.Feedback = []signal.Feedback{stepFeedback(t, 0, 2), stepFeedback(t, 1, 3)}
		if _, err := source.Release(2, early); err == nil {
			t.Errorf("%s: Release() error = nil with feedback available only at step 3", name)
		}
		wrongUnit, err := signal.NewFeedback(signal.FeedbackSpec{
			SchemaVersion: signal.CurrentSchemaVersion(), ExperienceID: "exp-1", ActionID: "action-2",
			ProducedAt:  signal.Timestamp{Value: 0, Unit: signal.TimeUnitMilliseconds},
			AvailableAt: signal.Timestamp{Value: 0, Unit: signal.TimeUnitMilliseconds},
			Source:      "teacher", Score: 1, ModelVersion: signal.CurrentSchemaVersion(),
		})
		if err != nil {
			t.Fatalf("build feedback: %v", err)
		}
		other := c
		other.Feedback = []signal.Feedback{wrongUnit}
		if _, err := source.Release(2, other); err == nil {
			t.Errorf("%s: Release() error = nil with feedback on another clock", name)
		}
		invalid := c
		invalid.Feedback = []signal.Feedback{{}}
		if _, err := source.Release(2, invalid); err == nil {
			t.Errorf("%s: Release() error = nil with invalid feedback", name)
		}
	}
}

// Every release is a non-negative, finite rate per declared channel, with or
// without an observation in the context.
func TestEverySourceReleasesNonNegativeFiniteChannelWideRates(t *testing.T) {
	sources, c := fixtureSources(t)
	withoutObservation := c
	withoutObservation.Observation = nil
	for name, source := range sources {
		for label, context := range map[string]SourceContext{"with observation": c, "without observation": withoutObservation} {
			got, err := source.Release(2, context)
			if err != nil {
				t.Fatalf("%s (%s): Release() error = %v", name, label, err)
			}
			if len(got) != source.Channels() {
				t.Fatalf("%s (%s): Release() returned %d values, want %d channels", name, label, len(got), source.Channels())
			}
			for i, v := range got {
				if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || math.Signbit(v) {
					t.Fatalf("%s (%s): channel %d released %v", name, label, i, v)
				}
			}
		}
	}
}

func TestControllerSourceIsReservedButNotImplemented(t *testing.T) {
	controller, err := NewController()
	if !errors.Is(err, ErrControllerNotImplemented) {
		t.Fatalf("NewController() error = %v, want ErrControllerNotImplemented", err)
	}
	if controller != nil {
		t.Fatalf("NewController() returned %v, want nil", controller)
	}
}

// The four implemented sources satisfy the interface; a target type appears in
// none of their signatures, because this package has no access to one.
var (
	_ Source = ExternalTimeline{}
	_ Source = NeuralActivity{Set: simulate.ResolvedSet{}}
	_ Source = InternalResource{}
	_ Source = Replay{}
)
