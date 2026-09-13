package signal

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"
)

func testVersion() Version { return Version{Major: 1, Minor: 0} }

func testTimestamp(value int64) Timestamp {
	return Timestamp{Value: value, Unit: TimeUnitMilliseconds}
}

func testSignalSpec(kind SignalKind, sequence uint64, start int64) SignalSpec {
	return SignalSpec{
		SchemaVersion:  testVersion(),
		ExperienceID:   "exp-1",
		StreamID:       "stream-1",
		Channel:        "vision",
		Kind:           kind,
		Start:          testTimestamp(start),
		Duration:       1,
		Shape:          []int{2},
		Values:         []float64{0.25, 0.75},
		Unit:           UnitNormalized,
		EncoderVersion: testVersion(),
		SourceSequence: sequence,
	}
}

func TestNewSignalValidatesKindsShapeUnitsAndCopiesInputs(t *testing.T) {
	kinds := []SignalKind{KindContinuous, KindActivity, KindPulse, KindModulation, KindIntervention}
	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			spec := testSignalSpec(kind, 1, 0)
			if kind == KindIntervention {
				spec.InterventionTarget = "male-cns:cell-7"
			}
			s, err := NewSignal(spec)
			if err != nil {
				t.Fatalf("NewSignal() error = %v", err)
			}
			spec.Shape[0] = 99
			spec.Values[0] = 99
			if got := s.Shape(); !reflect.DeepEqual(got, []int{2}) {
				t.Fatalf("Shape() = %v, want [2]", got)
			}
			if got := s.Values(); !reflect.DeepEqual(got, []float64{0.25, 0.75}) {
				t.Fatalf("Values() = %v, want original values", got)
			}
			gotValues := s.Values()
			gotValues[0] = -1
			if got := s.Values()[0]; got != 0.25 {
				t.Fatalf("Values() exposed mutable storage: got %v", got)
			}
		})
	}

	bad := []struct {
		name string
		spec SignalSpec
	}{
		{"unsupported unit", func() SignalSpec {
			s := testSignalSpec(KindContinuous, 1, 0)
			s.Unit = SignalUnit("furlong")
			return s
		}()},
		{"shape mismatch", func() SignalSpec {
			s := testSignalSpec(KindContinuous, 1, 0)
			s.Shape = []int{3}
			return s
		}()},
		{"negative duration", func() SignalSpec {
			s := testSignalSpec(KindContinuous, 1, 0)
			s.Duration = -1
			return s
		}()},
		{"non-finite value", func() SignalSpec {
			s := testSignalSpec(KindContinuous, 1, 0)
			s.Values[0] = math.NaN()
			return s
		}()},
		{"intervention target missing", testSignalSpec(KindIntervention, 1, 0)},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewSignal(tc.spec); err == nil {
				t.Fatal("NewSignal() error = nil, want validation error")
			}
		})
	}
}

func TestSignalJSONIsStrictAndRoundTrips(t *testing.T) {
	s, err := NewSignal(testSignalSpec(KindContinuous, 4, 10))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeSignal(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("DecodeSignal() error = %v", err)
	}
	if !reflect.DeepEqual(decoded, s) {
		t.Fatalf("decoded signal differs: %#v != %#v", decoded, s)
	}
	for _, input := range []string{
		`{"schema_version":{"major":1,"minor":0},"experience_id":"e","stream_id":"s","channel":"c","kind":"continuous","start":{"value":0,"unit":"ms"},"duration":1,"shape":[1],"values":[0],"unit":"normalized","encoder_version":{"major":1,"minor":0},"source_sequence":1,"unknown":true}`,
		string(data) + ` {"extra":true}`,
	} {
		if _, err := DecodeSignal(bytes.NewBufferString(input)); err == nil {
			t.Fatalf("DecodeSignal(%q) error = nil, want strict JSON error", input)
		}
	}
	duplicate := strings.Replace(string(data), `"schema_version":{"major":1,"minor":0},`, `"schema_version":{"major":1,"minor":0},"SCHEMA_VERSION":{"major":1,"minor":0},`, 1)
	if _, err := DecodeSignal(strings.NewReader(duplicate)); err == nil {
		t.Fatal("DecodeSignal() error = nil for case-insensitive duplicate field")
	}
	if _, err := DecodeSignal(strings.NewReader(strings.Repeat(" ", int(MaxJSONBytes+1)))); err == nil {
		t.Fatal("DecodeSignal() error = nil for oversized JSON input")
	}
	var nilReader io.Reader
	if _, err := DecodeSignal(nilReader); err == nil {
		t.Fatal("DecodeSignal() error = nil for nil reader")
	}
}

func TestSchemaVersionIsSupportedSeparatelyFromUserVersions(t *testing.T) {
	spec := testSignalSpec(KindContinuous, 1, 0)
	spec.SchemaVersion = Version{Major: 2, Minor: 0}
	if _, err := NewSignal(spec); err == nil {
		t.Fatal("NewSignal() error = nil for unsupported schema major")
	}
	spec.SchemaVersion = testVersion()
	spec.EncoderVersion = Version{Major: 27, Minor: 3}
	if _, err := NewSignal(spec); err != nil {
		t.Fatalf("NewSignal() rejected user encoder version: %v", err)
	}
}

func TestSignalOrderingAndStreamValidation(t *testing.T) {
	a, err := NewSignal(testSignalSpec(KindContinuous, 10, 2))
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewSignal(testSignalSpec(KindContinuous, 11, 2))
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := OrderSignals([]Signal{b, a})
	if err != nil {
		t.Fatalf("OrderSignals() error = %v", err)
	}
	if got := ordered[0].SourceSequence(); got != 10 {
		t.Fatalf("ordered first sequence = %d, want 10", got)
	}
	if ordered[0].SourceSequence() == b.SourceSequence() {
		t.Fatal("OrderSignals() did not order simultaneous events")
	}
	if _, err := OrderSignals([]Signal{a, a}); err == nil {
		t.Fatal("OrderSignals() error = nil for duplicate sequence")
	}
	stream, err := NewStream([]Signal{b, a})
	if err != nil {
		t.Fatalf("NewStream() error = %v for simultaneous events", err)
	}
	if stream.Signals()[0].SourceSequence() != 10 {
		t.Fatal("NewStream() did not order simultaneous events")
	}
	backward := testSignalSpec(KindContinuous, 12, 1)
	backwardSignal, err := NewSignal(backward)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewStream([]Signal{a, b, backwardSignal}); err == nil {
		t.Fatal("NewStream() error = nil for timestamp regression")
	}
	stream, err = NewStream([]Signal{a, b})
	if err != nil {
		t.Fatalf("NewStream() error = %v", err)
	}
	streamSignals := stream.Signals()
	streamSignals[0] = b
	if stream.Signals()[0].SourceSequence() != 10 {
		t.Fatal("Stream.Signals() exposed mutable storage")
	}
}

func TestClockUsesIntegerTimeAndRejectsOverflowOrMisalignment(t *testing.T) {
	clock, err := NewClock(testVersion(), TimeUnitMilliseconds, 5)
	if err != nil {
		t.Fatal(err)
	}
	at, err := clock.TimestampAt(3)
	if err != nil {
		t.Fatal(err)
	}
	if at != (Timestamp{Value: 15, Unit: TimeUnitMilliseconds}) {
		t.Fatalf("TimestampAt() = %#v, want 15ms", at)
	}
	if _, err := clock.TimestampAt(math.MaxInt64); err == nil {
		t.Fatal("TimestampAt() error = nil on overflow")
	}
	if _, err := clock.StepFor(Timestamp{Value: 16, Unit: TimeUnitMilliseconds}); err == nil {
		t.Fatal("StepFor() error = nil for misaligned timestamp")
	}
	next, nextTime, err := clock.Advance(2)
	if err != nil {
		t.Fatal(err)
	}
	if next.CurrentStep() != 2 || nextTime.Value != 10 {
		t.Fatalf("Advance() = step %d time %#v, want step 2 time 10", next.CurrentStep(), nextTime)
	}
	if _, err := NewClock(testVersion(), TimeUnitMilliseconds, 0); err == nil {
		t.Fatal("NewClock() error = nil for zero step size")
	}
}

func TestObservationTargetFeedbackAreSeparateAndCausal(t *testing.T) {
	s, err := NewSignal(testSignalSpec(KindContinuous, 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	observation, err := NewObservation(testVersion(), "exp-1", "stream-1", []Signal{s})
	if err != nil {
		t.Fatal(err)
	}
	target, err := NewTarget(testVersion(), "exp-1", "label", []Signal{s})
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(observation, target) {
		t.Fatal("Observation and Target unexpectedly have the same concrete type/value")
	}
	feedback, err := NewFeedback(FeedbackSpec{
		SchemaVersion: testVersion(),
		ExperienceID:  "exp-1",
		ActionID:      "action-1",
		ProducedAt:    testTimestamp(10),
		AvailableAt:   testTimestamp(15),
		Source:        "teacher",
		Score:         0.5,
		ModelVersion:  testVersion(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := feedback.ValidateAvailableAt(testTimestamp(14)); err == nil {
		t.Fatal("ValidateAvailableAt() error = nil before feedback is available")
	}
	if err := feedback.ValidateAvailableAt(testTimestamp(16)); err != nil {
		t.Fatalf("ValidateAvailableAt() unexpectedly failed after availability: %v", err)
	}
	if _, err := NewFeedback(FeedbackSpec{
		SchemaVersion: testVersion(), ExperienceID: "exp-1", ActionID: "action-1",
		ProducedAt: testTimestamp(10), AvailableAt: testTimestamp(9), Source: "teacher",
		Score: 0, ModelVersion: testVersion(),
	}); err == nil {
		t.Fatal("NewFeedback() error = nil when available time precedes produced time")
	}
	if err := feedback.ValidateForAction(testTimestamp(11)); err == nil {
		t.Fatal("ValidateForAction() error = nil when feedback predates action")
	}
	if err := feedback.ValidateForAction(testTimestamp(9)); err != nil {
		t.Fatalf("ValidateForAction() unexpectedly failed: %v", err)
	}

	empty, err := NewObservation(testVersion(), "exp-1", "stream-1", nil)
	if err != nil {
		t.Fatalf("NewObservation(empty) error = %v", err)
	}
	encoded, err := json.Marshal(empty)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "null" || bytes.Contains(encoded, []byte(`"signals":null`)) {
		t.Fatalf("empty observation lost its explicit empty-array meaning: %s", encoded)
	}
	if _, err := DecodeObservation(bytes.NewReader(encoded)); err != nil {
		t.Fatalf("DecodeObservation(empty) error = %v", err)
	}
}

func TestMappingPersistsLosslessIDsAndValidatesGraphBoundary(t *testing.T) {
	ids := []NeuronID{
		{Namespace: "male:cns", ExternalID: "cell/α"},
		{Namespace: "flywire", ExternalID: "00042"},
	}
	mapping, err := NewMapping(MappingSpec{
		SchemaVersion: testVersion(),
		Source:        "fixture",
		InputShape:    []int{2},
		Neurons:       ids,
	})
	if err != nil {
		t.Fatal(err)
	}
	ids[0].ExternalID = "mutated-after-construction"
	shape := mapping.InputShape()
	shape[0] = 99
	if got := mapping.Neurons()[0].ExternalID; got != "cell/α" {
		t.Fatalf("mapping shared mutable ID storage: got %q", got)
	}
	if got := mapping.InputShape()[0]; got != 2 {
		t.Fatalf("mapping shared mutable shape storage: got %d", got)
	}
	if got, err := mapping.IndexOf(ids[1]); err != nil || got != 1 {
		t.Fatalf("IndexOf() = %d, %v; want 1", got, err)
	}
	if err := mapping.ValidateShape([]int{2}); err != nil {
		t.Fatal(err)
	}
	if err := mapping.ValidateShape([]int{1, 2}); err == nil {
		t.Fatal("ValidateShape() error = nil for shape with matching product but different dimensions")
	}
	if err := mapping.ValidateShape([]int{1, 1}); err == nil {
		t.Fatal("ValidateShape() error = nil for dimension mismatch")
	}
	if err := mapping.ValidateAgainst([]NeuronID{ids[0]}); err == nil {
		t.Fatal("ValidateAgainst() error = nil for unknown neuron")
	}
	data, err := json.Marshal(mapping)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, err := DecodeMapping(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("DecodeMapping() error = %v", err)
	}
	if !reflect.DeepEqual(rebuilt, mapping) {
		t.Fatalf("rebuilt mapping differs: %#v != %#v", rebuilt, mapping)
	}
	if got := rebuilt.Neurons()[0].ExternalID; got != "cell/α" {
		t.Fatalf("rebuilt ID = %q, want lossless external ID", got)
	}
	if _, err := NewMapping(MappingSpec{
		SchemaVersion: testVersion(), Source: "fixture", InputShape: []int{2},
		Neurons: []NeuronID{ids[0], ids[0]},
	}); err == nil {
		t.Fatal("NewMapping() error = nil for duplicate neuron ID")
	}
	if _, err := NewMapping(MappingSpec{
		SchemaVersion: testVersion(), Source: "fixture", InputShape: []int{int(^uint(0) >> 1), 2},
		Neurons: []NeuronID{{Namespace: "fixture", ExternalID: "one"}},
	}); err == nil {
		t.Fatal("NewMapping() error = nil for overflowing input shape")
	}
}
