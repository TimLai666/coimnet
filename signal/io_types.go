package signal

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
)

// Observation contains only information available to a model at inference
// time. It cannot carry a Target value and has a distinct concrete type.
type Observation struct {
	schemaVersion Version
	experienceID  string
	streamID      string
	signals       []Signal
}

func NewObservation(version Version, experienceID, streamID string, signals []Signal) (Observation, error) {
	if err := validateSchemaVersion(version); err != nil {
		return Observation{}, err
	}
	if strings.TrimSpace(experienceID) == "" {
		return Observation{}, fmt.Errorf("experience_id must not be empty")
	}
	if strings.TrimSpace(streamID) == "" {
		return Observation{}, fmt.Errorf("stream_id must not be empty")
	}
	ordered, err := OrderSignals(signals)
	if err != nil {
		return Observation{}, err
	}
	for i, s := range ordered {
		if s.ExperienceID() != experienceID || s.StreamID() != streamID {
			return Observation{}, fmt.Errorf("signal %d does not belong to observation %q/%q", i, experienceID, streamID)
		}
		if s.Kind() == KindIntervention {
			return Observation{}, fmt.Errorf("intervention signal %d cannot enter an observation", i)
		}
	}
	owned := make([]Signal, len(ordered))
	copy(owned, ordered)
	return Observation{schemaVersion: version, experienceID: experienceID, streamID: streamID, signals: owned}, nil
}

func (o Observation) Validate() error {
	_, err := NewObservation(o.schemaVersion, o.experienceID, o.streamID, o.signals)
	return err
}

func (o Observation) SchemaVersion() Version { return o.schemaVersion }
func (o Observation) ExperienceID() string   { return o.experienceID }
func (o Observation) StreamID() string       { return o.streamID }
func (o Observation) Signals() []Signal {
	owned := make([]Signal, len(o.signals))
	copy(owned, o.signals)
	return owned
}

type observationJSON struct {
	SchemaVersion Version  `json:"schema_version"`
	ExperienceID  string   `json:"experience_id"`
	StreamID      string   `json:"stream_id"`
	Signals       []Signal `json:"signals"`
}

func (o Observation) MarshalJSON() ([]byte, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(observationJSON{SchemaVersion: o.schemaVersion, ExperienceID: o.experienceID, StreamID: o.streamID, Signals: o.Signals()})
}

func (o *Observation) UnmarshalJSON(data []byte) error {
	var raw observationJSON
	if err := decodeStrictBytes(data, &raw); err != nil {
		return err
	}
	if raw.Signals == nil {
		return fmt.Errorf("signals must be an array")
	}
	observation, err := NewObservation(raw.SchemaVersion, raw.ExperienceID, raw.StreamID, raw.Signals)
	if err != nil {
		return err
	}
	*o = observation
	return nil
}

func DecodeObservation(data io.Reader) (Observation, error) {
	var o Observation
	if err := decodeStrict(data, &o); err != nil {
		return Observation{}, err
	}
	return o, nil
}

// Target is a training-only expected result. It is deliberately separate from
// Observation so callers cannot accidentally pass an answer through the
// inference input path.
type Target struct {
	schemaVersion Version
	experienceID  string
	label         string
	signals       []Signal
}

func NewTarget(version Version, experienceID, label string, signals []Signal) (Target, error) {
	if err := validateSchemaVersion(version); err != nil {
		return Target{}, err
	}
	if strings.TrimSpace(experienceID) == "" {
		return Target{}, fmt.Errorf("experience_id must not be empty")
	}
	if strings.TrimSpace(label) == "" {
		return Target{}, fmt.Errorf("label must not be empty")
	}
	ordered, err := OrderSignals(signals)
	if err != nil {
		return Target{}, err
	}
	for i, s := range ordered {
		if s.ExperienceID() != experienceID {
			return Target{}, fmt.Errorf("signal %d does not belong to target experience %q", i, experienceID)
		}
	}
	owned := make([]Signal, len(ordered))
	copy(owned, ordered)
	return Target{schemaVersion: version, experienceID: experienceID, label: label, signals: owned}, nil
}

func (t Target) Validate() error {
	_, err := NewTarget(t.schemaVersion, t.experienceID, t.label, t.signals)
	return err
}

func (t Target) SchemaVersion() Version { return t.schemaVersion }
func (t Target) ExperienceID() string   { return t.experienceID }
func (t Target) Label() string          { return t.label }
func (t Target) Signals() []Signal {
	owned := make([]Signal, len(t.signals))
	copy(owned, t.signals)
	return owned
}

type targetJSON struct {
	SchemaVersion Version  `json:"schema_version"`
	ExperienceID  string   `json:"experience_id"`
	Label         string   `json:"label"`
	Signals       []Signal `json:"signals"`
}

func (t Target) MarshalJSON() ([]byte, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(targetJSON{SchemaVersion: t.schemaVersion, ExperienceID: t.experienceID, Label: t.label, Signals: t.Signals()})
}

func (t *Target) UnmarshalJSON(data []byte) error {
	var raw targetJSON
	if err := decodeStrictBytes(data, &raw); err != nil {
		return err
	}
	if raw.Signals == nil {
		return fmt.Errorf("signals must be an array")
	}
	target, err := NewTarget(raw.SchemaVersion, raw.ExperienceID, raw.Label, raw.Signals)
	if err != nil {
		return err
	}
	*t = target
	return nil
}

func DecodeTarget(data io.Reader) (Target, error) {
	var t Target
	if err := decodeStrict(data, &t); err != nil {
		return Target{}, err
	}
	return t, nil
}

// Feedback describes an outcome that arrives after an action. AvailableAt is
// the earliest timestamp at which a learner may consume it.
type FeedbackSpec struct {
	SchemaVersion Version   `json:"schema_version"`
	ExperienceID  string    `json:"experience_id"`
	ActionID      string    `json:"action_id"`
	ProducedAt    Timestamp `json:"produced_at"`
	AvailableAt   Timestamp `json:"available_at"`
	Source        string    `json:"source"`
	Score         float64   `json:"score"`
	ModelVersion  Version   `json:"model_version"`
}

type Feedback struct {
	schemaVersion Version
	experienceID  string
	actionID      string
	producedAt    Timestamp
	availableAt   Timestamp
	source        string
	score         float64
	modelVersion  Version
}

func NewFeedback(spec FeedbackSpec) (Feedback, error) {
	if err := spec.validate(); err != nil {
		return Feedback{}, err
	}
	return Feedback{
		schemaVersion: spec.SchemaVersion,
		experienceID:  spec.ExperienceID,
		actionID:      spec.ActionID,
		producedAt:    spec.ProducedAt,
		availableAt:   spec.AvailableAt,
		source:        spec.Source,
		score:         spec.Score,
		modelVersion:  spec.ModelVersion,
	}, nil
}

func (s FeedbackSpec) validate() error {
	if err := validateSchemaVersion(s.SchemaVersion); err != nil {
		return err
	}
	if strings.TrimSpace(s.ExperienceID) == "" {
		return fmt.Errorf("experience_id must not be empty")
	}
	if strings.TrimSpace(s.ActionID) == "" {
		return fmt.Errorf("action_id must not be empty")
	}
	if err := s.ProducedAt.validate("produced_at"); err != nil {
		return err
	}
	if err := s.AvailableAt.validate("available_at"); err != nil {
		return err
	}
	if s.ProducedAt.Unit != s.AvailableAt.Unit {
		return fmt.Errorf("produced_at and available_at use different units")
	}
	if s.AvailableAt.Value < s.ProducedAt.Value {
		return fmt.Errorf("available_at precedes produced_at")
	}
	if strings.TrimSpace(s.Source) == "" {
		return fmt.Errorf("source must not be empty")
	}
	if mathIsNotFinite(s.Score) {
		return fmt.Errorf("score must be finite")
	}
	if err := s.ModelVersion.validate("model_version"); err != nil {
		return err
	}
	return nil
}

func (s FeedbackSpec) Validate() error { return s.validate() }

func (f Feedback) Spec() FeedbackSpec {
	return FeedbackSpec{
		SchemaVersion: f.schemaVersion,
		ExperienceID:  f.experienceID,
		ActionID:      f.actionID,
		ProducedAt:    f.producedAt,
		AvailableAt:   f.availableAt,
		Source:        f.source,
		Score:         f.score,
		ModelVersion:  f.modelVersion,
	}
}

func mathIsNotFinite(value float64) bool { return math.IsNaN(value) || math.IsInf(value, 0) }

func (f Feedback) Validate() error {
	_, err := NewFeedback(FeedbackSpec{
		SchemaVersion: f.schemaVersion, ExperienceID: f.experienceID, ActionID: f.actionID,
		ProducedAt: f.producedAt, AvailableAt: f.availableAt, Source: f.source,
		Score: f.score, ModelVersion: f.modelVersion,
	})
	return err
}

// ValidateAvailableAt rejects consuming feedback before its declared arrival.
func (f Feedback) ValidateAvailableAt(now Timestamp) error {
	if err := f.Validate(); err != nil {
		return err
	}
	if err := now.validate("now"); err != nil {
		return err
	}
	if now.Unit != f.availableAt.Unit {
		return fmt.Errorf("now unit %q does not match feedback unit %q", now.Unit, f.availableAt.Unit)
	}
	if now.Value < f.availableAt.Value {
		return fmt.Errorf("feedback is not available at %d; available at %d", now.Value, f.availableAt.Value)
	}
	return nil
}

// ValidateForAction verifies that feedback was produced no earlier than the
// action it describes and has a causally valid availability time.
func (f Feedback) ValidateForAction(actionAt Timestamp) error {
	if err := f.Validate(); err != nil {
		return err
	}
	if err := actionAt.validate("action_at"); err != nil {
		return err
	}
	if actionAt.Unit != f.producedAt.Unit {
		return fmt.Errorf("action_at unit %q does not match feedback unit %q", actionAt.Unit, f.producedAt.Unit)
	}
	if f.producedAt.Value < actionAt.Value {
		return fmt.Errorf("feedback produced at %d before action at %d", f.producedAt.Value, actionAt.Value)
	}
	return nil
}

func (f Feedback) SchemaVersion() Version { return f.schemaVersion }
func (f Feedback) ExperienceID() string   { return f.experienceID }
func (f Feedback) ActionID() string       { return f.actionID }
func (f Feedback) ProducedAt() Timestamp  { return f.producedAt }
func (f Feedback) AvailableAt() Timestamp { return f.availableAt }
func (f Feedback) Source() string         { return f.source }
func (f Feedback) Score() float64         { return f.score }
func (f Feedback) ModelVersion() Version  { return f.modelVersion }

type feedbackJSON struct {
	SchemaVersion Version             `json:"schema_version"`
	ExperienceID  string              `json:"experience_id"`
	ActionID      string              `json:"action_id"`
	ProducedAt    Timestamp           `json:"produced_at"`
	AvailableAt   Timestamp           `json:"available_at"`
	Source        string              `json:"source"`
	Score         jsonNumber[float64] `json:"score"`
	ModelVersion  Version             `json:"model_version"`
}

func (f Feedback) MarshalJSON() ([]byte, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(feedbackJSON{SchemaVersion: f.schemaVersion, ExperienceID: f.experienceID, ActionID: f.actionID, ProducedAt: f.producedAt, AvailableAt: f.availableAt, Source: f.source, Score: jsonNumber[float64]{f.score}, ModelVersion: f.modelVersion})
}

func (f *Feedback) UnmarshalJSON(data []byte) error {
	var raw feedbackJSON
	if err := decodeStrictBytes(data, &raw); err != nil {
		return err
	}
	built, err := NewFeedback(FeedbackSpec{SchemaVersion: raw.SchemaVersion, ExperienceID: raw.ExperienceID, ActionID: raw.ActionID, ProducedAt: raw.ProducedAt, AvailableAt: raw.AvailableAt, Source: raw.Source, Score: raw.Score.value, ModelVersion: raw.ModelVersion})
	if err != nil {
		return err
	}
	*f = built
	return nil
}

func DecodeFeedback(data io.Reader) (Feedback, error) {
	var f Feedback
	if err := decodeStrict(data, &f); err != nil {
		return Feedback{}, err
	}
	return f, nil
}
