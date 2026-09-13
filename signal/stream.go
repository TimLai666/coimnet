package signal

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// OrderSignals returns a deep-copied, deterministic ordering by timestamp and
// source sequence. Events sharing a timestamp may arrive in any order and are
// reordered by source sequence. A timestamp that regresses in the input is
// rejected because silently repairing it would hide a stream error.
func OrderSignals(input []Signal) ([]Signal, error) {
	ordered := append([]Signal(nil), input...)
	if len(input) == 0 {
		return make([]Signal, 0), nil
	}
	seen := make(map[sequenceKey]struct{}, len(input))
	var unit TimeUnit
	var previousTime int64
	for i := range ordered {
		if err := ordered[i].Validate(); err != nil {
			return nil, fmt.Errorf("signal %d: %w", i, err)
		}
		if i == 0 {
			unit = ordered[i].StartTime().Unit
			previousTime = ordered[i].StartTime().Value
		} else if ordered[i].StartTime().Unit != unit {
			return nil, fmt.Errorf("signal %d uses time unit %q; want %q", i, ordered[i].StartTime().Unit, unit)
		} else if ordered[i].StartTime().Value < previousTime {
			return nil, fmt.Errorf("signal %d moves time backwards from %d to %d", i, previousTime, ordered[i].StartTime().Value)
		} else {
			previousTime = ordered[i].StartTime().Value
		}
		key := sequenceKey{experienceID: ordered[i].ExperienceID(), streamID: ordered[i].StreamID(), sequence: ordered[i].SourceSequence()}
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate source sequence %d for %s/%s", key.sequence, key.experienceID, key.streamID)
		}
		seen[key] = struct{}{}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := ordered[i], ordered[j]
		if left.StartTime().Unit != right.StartTime().Unit {
			return string(left.StartTime().Unit) < string(right.StartTime().Unit)
		}
		if left.StartTime().Value != right.StartTime().Value {
			return left.StartTime().Value < right.StartTime().Value
		}
		if left.SourceSequence() != right.SourceSequence() {
			return left.SourceSequence() < right.SourceSequence()
		}
		if left.StreamID() != right.StreamID() {
			return left.StreamID() < right.StreamID()
		}
		return left.Channel() < right.Channel()
	})
	return ordered, nil
}

// SortSignals is an equivalent descriptive name for OrderSignals.
func SortSignals(input []Signal) ([]Signal, error) { return OrderSignals(input) }

type sequenceKey struct {
	experienceID string
	streamID     string
	sequence     uint64
}

// Stream is a single chronologically validated signal stream. Its storage is
// private and Signals returns a copy of the slice.
type Stream struct {
	signals []Signal
}

func NewStream(input []Signal) (Stream, error) {
	if len(input) == 0 {
		return Stream{signals: make([]Signal, 0)}, nil
	}
	signals, err := OrderSignals(input)
	if err != nil {
		return Stream{}, err
	}
	if err := ValidateChronological(signals); err != nil {
		return Stream{}, err
	}
	return Stream{signals: signals}, nil
}

// ValidateChronological verifies that an input slice is already in stream
// order. It rejects time regressions, repeated source sequences, and reversed
// simultaneous events rather than silently repairing them.
func ValidateChronological(signals []Signal) error {
	seen := make(map[sequenceKey]struct{}, len(signals))
	var previous Signal
	for i, current := range signals {
		if err := current.Validate(); err != nil {
			return fmt.Errorf("signal %d: %w", i, err)
		}
		key := sequenceKey{experienceID: current.ExperienceID(), streamID: current.StreamID(), sequence: current.SourceSequence()}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate source sequence %d for %s/%s", key.sequence, key.experienceID, key.streamID)
		}
		seen[key] = struct{}{}
		if i == 0 {
			previous = current
			continue
		}
		if current.ExperienceID() != previous.ExperienceID() || current.StreamID() != previous.StreamID() || current.Channel() != previous.Channel() {
			return fmt.Errorf("signal %d does not belong to the same experience, stream, and channel", i)
		}
		if current.StartTime().Unit != previous.StartTime().Unit {
			return fmt.Errorf("signal %d uses time unit %q; want %q", i, current.StartTime().Unit, previous.StartTime().Unit)
		}
		if current.StartTime().Value < previous.StartTime().Value {
			return fmt.Errorf("signal %d moves time backwards from %d to %d", i, previous.StartTime().Value, current.StartTime().Value)
		}
		if current.StartTime().Value == previous.StartTime().Value && current.SourceSequence() <= previous.SourceSequence() {
			return fmt.Errorf("signal %d has simultaneous source sequence %d after %d", i, current.SourceSequence(), previous.SourceSequence())
		}
		if current.StartTime().Value > previous.StartTime().Value && current.SourceSequence() <= previous.SourceSequence() {
			return fmt.Errorf("signal %d source sequence %d does not advance after %d", i, current.SourceSequence(), previous.SourceSequence())
		}
		previous = current
	}
	return nil
}

func (s Stream) Signals() []Signal {
	owned := make([]Signal, len(s.signals))
	copy(owned, s.signals)
	return owned
}

type streamJSON struct {
	Signals []Signal `json:"signals"`
}

func (s Stream) MarshalJSON() ([]byte, error) {
	if err := ValidateChronological(s.signals); err != nil {
		return nil, err
	}
	return json.Marshal(streamJSON{Signals: s.Signals()})
}

func (s *Stream) UnmarshalJSON(data []byte) error {
	var raw streamJSON
	if err := decodeStrictBytes(data, &raw); err != nil {
		return err
	}
	if raw.Signals == nil {
		return fmt.Errorf("signals must be an array")
	}
	stream, err := NewStream(raw.Signals)
	if err != nil {
		return err
	}
	*s = stream
	return nil
}

func DecodeStream(data io.Reader) (Stream, error) {
	var s Stream
	if err := decodeStrict(data, &s); err != nil {
		return Stream{}, err
	}
	return s, nil
}
