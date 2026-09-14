package signal

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
)

// Version identifies the schema or encoder contract used by a value.
type Version struct {
	Major int `json:"major"`
	Minor int `json:"minor"`
}

const (
	supportedSchemaMajor = 1
	supportedSchemaMinor = 0
)

// CurrentSchemaVersion is the only schema version this package can safely
// decode. Encoder and model versions remain user-defined Version values.
func CurrentSchemaVersion() Version {
	return Version{Major: supportedSchemaMajor, Minor: supportedSchemaMinor}
}

func validateSchemaVersion(v Version) error {
	if err := v.validate("schema_version"); err != nil {
		return err
	}
	if v != CurrentSchemaVersion() {
		return fmt.Errorf("unsupported schema version %d.%d; want %d.%d", v.Major, v.Minor, supportedSchemaMajor, supportedSchemaMinor)
	}
	return nil
}

func NewVersion(major, minor int) (Version, error) {
	v := Version{Major: major, Minor: minor}
	if err := v.validate("version"); err != nil {
		return Version{}, err
	}
	return v, nil
}

func (v Version) Validate() error { return v.validate("version") }

func (v Version) validate(name string) error {
	if v.Major < 1 || v.Minor < 0 {
		return fmt.Errorf("%s must have major >= 1 and minor >= 0", name)
	}
	return nil
}

// Timestamp is an integer timestamp in a declared time unit.
type Timestamp struct {
	Value int64    `json:"value"`
	Unit  TimeUnit `json:"unit"`
}

func NewTimestamp(value int64, unit TimeUnit) (Timestamp, error) {
	t := Timestamp{Value: value, Unit: unit}
	if err := t.validate("timestamp"); err != nil {
		return Timestamp{}, err
	}
	return t, nil
}

func (t Timestamp) Validate() error { return t.validate("timestamp") }

func (t Timestamp) validate(name string) error {
	if t.Value < 0 {
		return fmt.Errorf("%s value must be non-negative", name)
	}
	if err := t.Unit.validate(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// TimeUnit is the unit of the integer simulation clock or timestamp.
type TimeUnit string

const (
	TimeUnitModelStep      TimeUnit = "model_step"
	TimeUnitNormalizedTime TimeUnit = "normalized_time"
	TimeUnitNanoseconds    TimeUnit = "ns"
	TimeUnitMicroseconds   TimeUnit = "us"
	TimeUnitMilliseconds   TimeUnit = "ms"
	TimeUnitSeconds        TimeUnit = "s"
)

func (u TimeUnit) validate() error {
	switch u {
	case TimeUnitModelStep, TimeUnitNormalizedTime, TimeUnitNanoseconds,
		TimeUnitMicroseconds, TimeUnitMilliseconds, TimeUnitSeconds:
		return nil
	default:
		return fmt.Errorf("unsupported time unit %q", u)
	}
}

func (u TimeUnit) Validate() error { return u.validate() }

// SignalUnit is the declared unit for signal values.
type SignalUnit string

const (
	UnitNormalized     SignalUnit = "normalized"
	UnitDimensionless  SignalUnit = "1"
	UnitAmpere         SignalUnit = "A"
	UnitMilliampere    SignalUnit = "mA"
	UnitMicroampere    SignalUnit = "uA"
	UnitVolt           SignalUnit = "V"
	UnitMillivolt      SignalUnit = "mV"
	UnitHertz          SignalUnit = "Hz"
	UnitModelStep      SignalUnit = "model_step"
	UnitNormalizedTime SignalUnit = "normalized_time"
)

func (u SignalUnit) validate() error {
	switch u {
	case UnitNormalized, UnitDimensionless, UnitAmpere, UnitMilliampere,
		UnitMicroampere, UnitVolt, UnitMillivolt, UnitHertz, UnitModelStep,
		UnitNormalizedTime:
		return nil
	default:
		return fmt.Errorf("unsupported signal unit %q", u)
	}
}

func (u SignalUnit) Validate() error { return u.validate() }

// SignalKind describes the semantics of the values in a Signal.
type SignalKind string

const (
	KindContinuous   SignalKind = "continuous"
	KindActivity     SignalKind = "activity"
	KindPulse        SignalKind = "pulse"
	KindModulation   SignalKind = "modulation"
	KindIntervention SignalKind = "intervention"
)

func (k SignalKind) validate() error {
	switch k {
	case KindContinuous, KindActivity, KindPulse, KindModulation, KindIntervention:
		return nil
	default:
		return fmt.Errorf("unsupported signal kind %q", k)
	}
}

func (k SignalKind) Validate() error { return k.validate() }

// ValueRange optionally constrains every value in a signal.
type ValueRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

func (r *ValueRange) validate() error {
	if r == nil {
		return nil
	}
	if math.IsNaN(r.Min) || math.IsInf(r.Min, 0) || math.IsNaN(r.Max) || math.IsInf(r.Max, 0) {
		return fmt.Errorf("valid range must be finite")
	}
	if r.Min > r.Max {
		return fmt.Errorf("valid range min %v exceeds max %v", r.Min, r.Max)
	}
	return nil
}

// Quality describes optional quality metadata for a signal.
type Quality struct {
	Label string   `json:"label"`
	Score *float64 `json:"score,omitempty"`
}

func (q Quality) validate() error {
	if q.Score == nil {
		return nil
	}
	if math.IsNaN(*q.Score) || math.IsInf(*q.Score, 0) {
		return fmt.Errorf("quality score must be finite")
	}
	if *q.Score < 0 || *q.Score > 1 {
		return fmt.Errorf("quality score must be between 0 and 1")
	}
	return nil
}

// SignalSpec is the input to NewSignal. Its slices are copied by the
// constructor. Decode untrusted JSON with DecodeSignal, then call Spec.
type SignalSpec struct {
	SchemaVersion      Version     `json:"schema_version"`
	ExperienceID       string      `json:"experience_id"`
	StreamID           string      `json:"stream_id"`
	Channel            string      `json:"channel"`
	Kind               SignalKind  `json:"kind"`
	Start              Timestamp   `json:"start"`
	Duration           int64       `json:"duration"`
	Shape              []int       `json:"shape"`
	Values             []float64   `json:"values"`
	Unit               SignalUnit  `json:"unit"`
	ValidRange         *ValueRange `json:"valid_range,omitempty"`
	Quality            Quality     `json:"quality"`
	EncoderVersion     Version     `json:"encoder_version"`
	SourceSequence     uint64      `json:"source_sequence"`
	InterventionTarget string      `json:"intervention_target,omitempty"`
}

// Signal is a validated, versioned named signal. Its mutable storage is
// private and is copied on construction and access.
type Signal struct {
	schemaVersion      Version
	experienceID       string
	streamID           string
	channel            string
	kind               SignalKind
	start              Timestamp
	duration           int64
	shape              []int
	values             []float64
	unit               SignalUnit
	validRange         *ValueRange
	quality            Quality
	encoderVersion     Version
	sourceSequence     uint64
	interventionTarget string
}

func NewSignal(spec SignalSpec) (Signal, error) {
	if err := spec.validate(); err != nil {
		return Signal{}, err
	}
	shape := append([]int(nil), spec.Shape...)
	values := append([]float64(nil), spec.Values...)
	var validRange *ValueRange
	if spec.ValidRange != nil {
		copyRange := *spec.ValidRange
		validRange = &copyRange
	}
	quality := spec.Quality
	if quality.Score != nil {
		score := *quality.Score
		quality.Score = &score
	}
	return Signal{
		schemaVersion:      spec.SchemaVersion,
		experienceID:       spec.ExperienceID,
		streamID:           spec.StreamID,
		channel:            spec.Channel,
		kind:               spec.Kind,
		start:              spec.Start,
		duration:           spec.Duration,
		shape:              shape,
		values:             values,
		unit:               spec.Unit,
		validRange:         validRange,
		quality:            quality,
		encoderVersion:     spec.EncoderVersion,
		sourceSequence:     spec.SourceSequence,
		interventionTarget: spec.InterventionTarget,
	}, nil
}

func (s SignalSpec) validate() error {
	if err := validateSchemaVersion(s.SchemaVersion); err != nil {
		return err
	}
	if strings.TrimSpace(s.ExperienceID) == "" {
		return fmt.Errorf("experience_id must not be empty")
	}
	if strings.TrimSpace(s.StreamID) == "" {
		return fmt.Errorf("stream_id must not be empty")
	}
	if strings.TrimSpace(s.Channel) == "" {
		return fmt.Errorf("channel must not be empty")
	}
	if err := s.Kind.validate(); err != nil {
		return err
	}
	if err := s.Start.validate("start"); err != nil {
		return err
	}
	if s.Duration < 0 {
		return fmt.Errorf("duration must be non-negative")
	}
	if s.Duration > math.MaxInt64-s.Start.Value {
		return fmt.Errorf("start plus duration overflows timestamp")
	}
	if err := s.Unit.validate(); err != nil {
		return err
	}
	if err := s.EncoderVersion.validate("encoder_version"); err != nil {
		return err
	}
	if err := s.Quality.validate(); err != nil {
		return err
	}
	if err := s.ValidRange.validate(); err != nil {
		return err
	}
	size, err := shapeSize(s.Shape)
	if err != nil {
		return err
	}
	if size != len(s.Values) {
		return fmt.Errorf("shape describes %d values, got %d", size, len(s.Values))
	}
	for i, value := range s.Values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("values[%d] must be finite", i)
		}
		if s.ValidRange != nil && (value < s.ValidRange.Min || value > s.ValidRange.Max) {
			return fmt.Errorf("values[%d]=%v is outside valid range [%v,%v]", i, value, s.ValidRange.Min, s.ValidRange.Max)
		}
	}
	if s.Kind == KindIntervention && strings.TrimSpace(s.InterventionTarget) == "" {
		return fmt.Errorf("intervention_target is required for intervention signals")
	}
	if s.Kind != KindIntervention && s.InterventionTarget != "" {
		return fmt.Errorf("intervention_target is only valid for intervention signals")
	}
	return nil
}

func (s SignalSpec) Validate() error { return s.validate() }

func shapeSize(shape []int) (int, error) {
	if len(shape) == 0 {
		return 0, fmt.Errorf("shape must contain at least one dimension")
	}
	maxInt := int(^uint(0) >> 1)
	size := 1
	for i, dimension := range shape {
		if dimension <= 0 {
			return 0, fmt.Errorf("shape[%d] must be positive", i)
		}
		if size > maxInt/dimension {
			return 0, fmt.Errorf("shape capacity overflows int")
		}
		size *= dimension
	}
	return size, nil
}

func (s Signal) Validate() error {
	_, err := NewSignal(s.spec())
	return err
}

func (s Signal) spec() SignalSpec {
	var validRange *ValueRange
	if s.validRange != nil {
		copyRange := *s.validRange
		validRange = &copyRange
	}
	quality := s.quality
	if quality.Score != nil {
		score := *quality.Score
		quality.Score = &score
	}
	return SignalSpec{
		SchemaVersion:      s.schemaVersion,
		ExperienceID:       s.experienceID,
		StreamID:           s.streamID,
		Channel:            s.channel,
		Kind:               s.kind,
		Start:              s.start,
		Duration:           s.duration,
		Shape:              append([]int(nil), s.shape...),
		Values:             append([]float64(nil), s.values...),
		Unit:               s.unit,
		ValidRange:         validRange,
		Quality:            quality,
		EncoderVersion:     s.encoderVersion,
		SourceSequence:     s.sourceSequence,
		InterventionTarget: s.interventionTarget,
	}
}

func (s Signal) Clone() Signal { return mustSignal(s.spec()) }

// Spec returns an owned builder value whose slices can be changed without
// changing the Signal.
func (s Signal) Spec() SignalSpec { return s.spec() }

func mustSignal(spec SignalSpec) Signal {
	s, err := NewSignal(spec)
	if err != nil {
		return Signal{}
	}
	return s
}

func (s Signal) SchemaVersion() Version { return s.schemaVersion }
func (s Signal) ExperienceID() string   { return s.experienceID }
func (s Signal) StreamID() string       { return s.streamID }
func (s Signal) Channel() string        { return s.channel }
func (s Signal) Kind() SignalKind       { return s.kind }
func (s Signal) StartTime() Timestamp   { return s.start }
func (s Signal) Duration() int64        { return s.duration }
func (s Signal) Shape() []int           { return append([]int(nil), s.shape...) }
func (s Signal) Values() []float64      { return append([]float64(nil), s.values...) }
func (s Signal) Unit() SignalUnit       { return s.unit }
func (s Signal) ValidRange() *ValueRange {
	if s.validRange == nil {
		return nil
	}
	copyRange := *s.validRange
	return &copyRange
}
func (s Signal) Quality() Quality {
	q := s.quality
	if q.Score != nil {
		score := *q.Score
		q.Score = &score
	}
	return q
}
func (s Signal) EncoderVersion() Version    { return s.encoderVersion }
func (s Signal) SourceSequence() uint64     { return s.sourceSequence }
func (s Signal) InterventionTarget() string { return s.interventionTarget }

type signalJSON struct {
	SchemaVersion      Version            `json:"schema_version"`
	ExperienceID       string             `json:"experience_id"`
	StreamID           string             `json:"stream_id"`
	Channel            string             `json:"channel"`
	Kind               SignalKind         `json:"kind"`
	Start              Timestamp          `json:"start"`
	Duration           jsonNumber[int64]  `json:"duration"`
	Shape              []int              `json:"shape"`
	Values             jsonValues         `json:"values"`
	Unit               SignalUnit         `json:"unit"`
	ValidRange         *ValueRange        `json:"valid_range,omitempty"`
	Quality            *Quality           `json:"quality"`
	EncoderVersion     Version            `json:"encoder_version"`
	SourceSequence     jsonNumber[uint64] `json:"source_sequence"`
	InterventionTarget string             `json:"intervention_target,omitempty"`
}

func (s Signal) MarshalJSON() ([]byte, error) {
	quality := s.Quality()
	return json.Marshal(signalJSON{
		SchemaVersion: s.schemaVersion, ExperienceID: s.experienceID,
		StreamID: s.streamID, Channel: s.channel, Kind: s.kind, Start: s.start,
		Duration: jsonNumber[int64]{s.duration}, Shape: append([]int(nil), s.shape...),
		Values: append([]float64(nil), s.values...), Unit: s.unit,
		ValidRange: s.ValidRange(), Quality: &quality,
		EncoderVersion: s.encoderVersion, SourceSequence: jsonNumber[uint64]{s.sourceSequence},
		InterventionTarget: s.interventionTarget,
	})
}

func (s *Signal) UnmarshalJSON(data []byte) error {
	var raw signalJSON
	if err := decodeStrictBytes(data, &raw); err != nil {
		return err
	}
	if raw.Quality == nil {
		return fmt.Errorf("quality must be present")
	}
	built, err := NewSignal(SignalSpec{
		SchemaVersion: raw.SchemaVersion, ExperienceID: raw.ExperienceID,
		StreamID: raw.StreamID, Channel: raw.Channel, Kind: raw.Kind,
		Start: raw.Start, Duration: raw.Duration.value, Shape: raw.Shape,
		Values: raw.Values, Unit: raw.Unit, ValidRange: raw.ValidRange,
		Quality: *raw.Quality, EncoderVersion: raw.EncoderVersion,
		SourceSequence: raw.SourceSequence.value, InterventionTarget: raw.InterventionTarget,
	})
	if err != nil {
		return err
	}
	*s = built
	return nil
}

func DecodeSignal(data io.Reader) (Signal, error) {
	var s Signal
	if err := decodeStrict(data, &s); err != nil {
		return Signal{}, err
	}
	return s, nil
}

// UnmarshalVersion is a strict helper for callers that store a version alone.
func UnmarshalVersion(data []byte) (Version, error) {
	var v Version
	if err := decodeStrictBytes(data, &v); err != nil {
		return Version{}, err
	}
	return v, v.validate("version")
}

func (v Version) MarshalJSON() ([]byte, error) {
	if err := v.validate("version"); err != nil {
		return nil, err
	}
	type versionAlias Version
	return json.Marshal(versionAlias(v))
}

func (v *Version) UnmarshalJSON(data []byte) error {
	var decoded struct {
		Major jsonNumber[int] `json:"major"`
		Minor jsonNumber[int] `json:"minor"`
	}
	if err := decodeStrictBytes(data, &decoded); err != nil {
		return err
	}
	value := Version{Major: decoded.Major.value, Minor: decoded.Minor.value}
	if err := value.validate("version"); err != nil {
		return err
	}
	*v = value
	return nil
}

func (t Timestamp) MarshalJSON() ([]byte, error) {
	if err := t.validate("timestamp"); err != nil {
		return nil, err
	}
	type timestampAlias Timestamp
	return json.Marshal(timestampAlias(t))
}

func (t *Timestamp) UnmarshalJSON(data []byte) error {
	var decoded struct {
		Value jsonNumber[int64] `json:"value"`
		Unit  TimeUnit          `json:"unit"`
	}
	if err := decodeStrictBytes(data, &decoded); err != nil {
		return err
	}
	value := Timestamp{Value: decoded.Value.value, Unit: decoded.Unit}
	if err := value.validate("timestamp"); err != nil {
		return err
	}
	*t = value
	return nil
}

func (r ValueRange) MarshalJSON() ([]byte, error) {
	if err := (&r).validate(); err != nil {
		return nil, err
	}
	type rangeAlias ValueRange
	return json.Marshal(rangeAlias(r))
}

func (r *ValueRange) UnmarshalJSON(data []byte) error {
	var decoded struct {
		Min jsonNumber[float64] `json:"min"`
		Max jsonNumber[float64] `json:"max"`
	}
	if err := decodeStrictBytes(data, &decoded); err != nil {
		return err
	}
	value := ValueRange{Min: decoded.Min.value, Max: decoded.Max.value}
	if err := (&value).validate(); err != nil {
		return err
	}
	*r = value
	return nil
}

func (q Quality) MarshalJSON() ([]byte, error) {
	if err := q.validate(); err != nil {
		return nil, err
	}
	type qualityAlias Quality
	return json.Marshal(qualityAlias(q))
}

func (q *Quality) UnmarshalJSON(data []byte) error {
	type qualityAlias Quality
	var decoded qualityAlias
	if err := decodeStrictBytes(data, &decoded); err != nil {
		return err
	}
	value := Quality(decoded)
	if err := value.validate(); err != nil {
		return err
	}
	*q = value
	return nil
}
