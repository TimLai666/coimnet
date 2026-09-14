package signal

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestJSONRejectsNullNumbersWithoutChangingReceiver(t *testing.T) {
	s, err := NewSignal(testSignalSpec(KindContinuous, 7, 3))
	if err != nil {
		t.Fatal(err)
	}
	signalData, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, base, old, replacement string
		receiver                     func() any
	}{
		{"signal value", string(signalData), `"values":[0.25,0.75]`, `"values":[null,0.75]`, func() any { return new(Signal) }},
		{"duration", string(signalData), `"duration":1`, `"duration":null`, func() any { return new(Signal) }},
		{"sequence", string(signalData), `"source_sequence":7`, `"source_sequence":null`, func() any { return new(Signal) }},
		{"timestamp", `{"value":3,"unit":"ms"}`, `"value":3`, `"value":null`, func() any { return new(Timestamp) }},
		{"version", `{"major":1,"minor":2}`, `"minor":2`, `"minor":null`, func() any { return new(Version) }},
		{"range min", `{"min":-1,"max":1}`, `"min":-1`, `"min":null`, func() any { return new(ValueRange) }},
		{"range max", `{"min":-1,"max":1}`, `"max":1`, `"max":null`, func() any { return new(ValueRange) }},
		{"clock", `{"schema_version":{"major":1,"minor":0},"unit":"ms","step_size":1,"current_step":3}`, `"current_step":3`, `"current_step":null`, func() any { return new(Clock) }},
		{"mapping index", `{"schema_version":{"major":1,"minor":0},"source":"fixture","input_shape":[1],"entries":[{"namespace":"fixture","external_id":"cell-1","index":0}]}`, `"index":0`, `"index":null`, func() any { return new(Mapping) }},
		{"feedback score", `{"schema_version":{"major":1,"minor":0},"experience_id":"exp-1","action_id":"act-1","produced_at":{"value":0,"unit":"ms"},"available_at":{"value":1,"unit":"ms"},"source":"fixture","score":0.5,"model_version":{"major":1,"minor":0}}`, `"score":0.5`, `"score":null`, func() any { return new(Feedback) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dst := tc.receiver()
			if err := json.Unmarshal([]byte(tc.base), dst); err != nil {
				t.Fatalf("valid fixture: %v", err)
			}
			before, err := json.Marshal(dst)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(tc.base, tc.old) != 1 {
				t.Fatal("mutation must match exactly once")
			}
			bad := strings.Replace(tc.base, tc.old, tc.replacement, 1)
			if err := json.Unmarshal([]byte(bad), dst); err == nil {
				t.Errorf("accepted numeric null: %s", bad)
			}
			after, err := json.Marshal(dst)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Errorf("failed decode changed receiver: %s -> %s", before, after)
			}
		})
	}
}

func TestSignalJSONNumericValuesAndNullableMetadata(t *testing.T) {
	for _, kind := range []SignalKind{KindContinuous, KindActivity, KindPulse, KindModulation} {
		t.Run(string(kind), func(t *testing.T) {
			spec := testSignalSpec(kind, 0, 0)
			spec.Duration = 0
			spec.Values = []float64{0, 0}
			s, err := NewSignal(spec)
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Count(data, []byte(`"quality":{"label":""}`)) != 1 {
				t.Fatal("nullable metadata mutation must match exactly once")
			}
			data = bytes.Replace(data, []byte(`"quality":{"label":""}`), []byte(`"quality":{"label":"","score":null},"valid_range":null`), 1)
			restored, err := DecodeSignal(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(s.Spec(), restored.Spec()) {
				t.Fatal("zero or nullable metadata changed")
			}
			for _, invalid := range []string{`null`, `"0"`, `true`, `[]`, `{}`, `1e400`} {
				bad := bytes.Replace(data, []byte(`"values":[0,0]`), []byte(`"values":[`+invalid+`,0]`), 1)
				if bytes.Equal(bad, data) {
					t.Fatal("missing fixture values")
				}
				if _, err := DecodeSignal(bytes.NewReader(bad)); err == nil {
					t.Errorf("accepted nonnumeric value %s", invalid)
				}
			}
		})
	}
}

func TestNestedSignalJSONRejectsNullAndPreservesOmittedDefaults(t *testing.T) {
	s, err := NewSignal(testSignalSpec(KindContinuous, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	observation, err := NewObservation(testVersion(), s.ExperienceID(), s.StreamID(), []Signal{s})
	if err != nil {
		t.Fatal(err)
	}
	target, err := NewTarget(testVersion(), s.ExperienceID(), "answer", []Signal{s})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		value    any
		receiver func() any
	}{
		{"observation", observation, func() any { return new(Observation) }},
		{"target", target, func() any { return new(Target) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatal(err)
			}
			dst := tc.receiver()
			if err := json.Unmarshal(data, dst); err != nil {
				t.Fatal(err)
			}
			for _, mutation := range [][2]string{
				{`"values":[0.25,0.75]`, `"values":[null,0.75]`},
				{`"start":{"value":0,"unit":"ms"}`, `"start":{"value":null,"unit":"ms"}`},
				{`"source_sequence":0`, `"source_sequence":null`},
			} {
				bad := bytes.Replace(data, []byte(mutation[0]), []byte(mutation[1]), 1)
				if bytes.Equal(data, bad) {
					t.Fatal("mutation did not change fixture")
				}
				if err := json.Unmarshal(bad, dst); err == nil {
					t.Errorf("accepted nested null: %s", bad)
				}
				after, err := json.Marshal(dst)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(data, after) {
					t.Error("failed nested decode changed receiver")
				}
			}
		})
	}
	// This fix rejects explicit null, without making omitted zero-default fields required.
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	omitted := bytes.Replace(data, []byte(`"duration":1,`), nil, 1)
	omitted = bytes.Replace(omitted, []byte(`,"source_sequence":0`), nil, 1)
	decoded, err := DecodeSignal(bytes.NewReader(omitted))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Duration() != 0 || decoded.SourceSequence() != 0 {
		t.Fatal("omitted defaults changed")
	}
}

func TestNamedSourceContracts(t *testing.T) {
	for _, kind := range []SignalKind{KindContinuous, KindActivity, KindPulse, KindModulation} {
		t.Run(string(kind), func(t *testing.T) {
			spec := testSignalSpec(kind, 3, 2)
			spec.Channel = string(kind) + "-source"
			spec.EncoderVersion = Version{Major: 2, Minor: 3}
			spec.ValidRange = &ValueRange{Min: 0, Max: 1}
			score := 0.75
			spec.Quality = Quality{Label: "artificial", Score: &score}
			good, err := NewSignal(spec)
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(good)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := DecodeSignal(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(restored.Spec(), spec) {
				t.Fatal("source metadata or values did not round trip")
			}
			for _, tc := range []struct {
				name   string
				mutate func(*SignalSpec)
			}{
				{"kind", func(s *SignalSpec) { s.Kind = "unknown" }},
				{"unit", func(s *SignalSpec) { s.Unit = "unknown" }},
				{"channel", func(s *SignalSpec) { s.Channel = " " }},
				{"shape", func(s *SignalSpec) { s.Shape = []int{3} }},
				{"range", func(s *SignalSpec) { s.Values[0] = 2 }},
				{"encoder", func(s *SignalSpec) { s.EncoderVersion.Major = 0 }},
				{"quality", func(s *SignalSpec) { *s.Quality.Score = 2 }},
			} {
				t.Run(tc.name, func(t *testing.T) {
					invalid := good.Spec()
					tc.mutate(&invalid)
					if _, err := NewSignal(invalid); err == nil {
						t.Fatal("accepted invalid source contract")
					}
				})
			}
			// Constructor and accessors must isolate caller-owned metadata too.
			spec.ValidRange.Min = 99
			*spec.Quality.Score = 99
			exported := restored.Spec()
			exported.ValidRange.Max = -99
			*exported.Quality.Score = -99
			if good.ValidRange().Min != 0 || *good.Quality().Score != 0.75 || restored.ValidRange().Max != 1 || *restored.Quality().Score != 0.75 {
				t.Fatal("metadata storage is shared")
			}
		})
	}
}

func TestSignalJSONPreservesNumericExtremes(t *testing.T) {
	spec := testSignalSpec(KindContinuous, math.MaxUint64, math.MaxInt64)
	spec.Duration = 0
	spec.Shape = []int{3}
	spec.Values = []float64{-math.MaxFloat64, math.Copysign(0, -1), math.SmallestNonzeroFloat64}
	s, err := NewSignal(spec)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := DecodeSignal(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Spec(), spec) || !math.Signbit(restored.Values()[1]) {
		t.Fatal("numeric boundary value changed")
	}
}
