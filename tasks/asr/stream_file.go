package asr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/internal/strictjson"
	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

const streamFileSchema = "coimnet-asr-stream/v1"

// streamFileState nests the complete individual checkpoint rather than
// decoding it directly into IndividualSnapshot: the individual checkpoint's
// raw-field, Unicode, schema, and checksum checks must run before Go's JSON
// decoder can turn missing or null numbers into valid zero values.
type streamFileState struct {
	Individual      json.RawMessage      `json:"individual"`
	Pending         []float64            `json:"pending"`
	PrevClass       int                  `json:"prev_class"`
	Emitted         string               `json:"emitted"`
	SampleRate      int                  `json:"sample_rate"`
	FrontEnd        audio.FrontEndConfig `json:"front_end"`
	FrontEndVersion string               `json:"front_end_version"`
	Alphabet        []rune               `json:"alphabet"`
	Flushed         bool                 `json:"flushed"`
}

// Save persists the whole resumable stream as one bounded, versioned file.
// The destination must not exist; a canceled or failed pre-publication save
// leaves no destination. The caller must not use the stream concurrently.
func (s *Stream) Save(ctx context.Context, path string) error {
	if s == nil || s.ind == nil {
		return errors.New("asr: nil stream")
	}
	if ctx == nil {
		return errors.New("asr: nil context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	state := s.Snapshot()
	individual, err := checkpoint.EncodeIndividual(state.Individual)
	if err != nil {
		return fmt.Errorf("asr: encode stream individual: %w", err)
	}
	payload, err := json.Marshal(streamFileState{
		Individual:      individual,
		Pending:         append(make([]float64, 0, len(state.Pending)), state.Pending...),
		PrevClass:       state.PrevClass,
		Emitted:         state.Emitted,
		SampleRate:      state.SampleRate,
		FrontEnd:        state.FrontEnd,
		FrontEndVersion: state.FrontEndVersion,
		Alphabet:        append(make([]rune, 0, len(state.Alphabet)), state.Alphabet...),
		Flushed:         state.Flushed,
	})
	if err != nil {
		return fmt.Errorf("asr: encode stream state: %w", err)
	}
	if err := checkpoint.SavePayload(ctx, path, streamFileSchema, payload); err != nil {
		return fmt.Errorf("asr: save stream: %w", err)
	}
	return nil
}

// LoadStream reads a bounded stream file and resumes it using this
// recognizer's sample rate, front end, alphabet, and core configuration.
// Parameters and neural state come from the saved stream, not the current
// trainer. The caller owns the file path and its access permissions.
func (r *Recognizer) LoadStream(ctx context.Context, path string) (*Stream, error) {
	if r == nil || r.trainer == nil {
		return nil, errors.New("asr: nil recognizer")
	}
	payload, err := checkpoint.LoadPayload(ctx, path, streamFileSchema)
	if err != nil {
		return nil, fmt.Errorf("asr: load stream: %w", err)
	}
	if err := requireStreamFileFields(payload, r.config.FrontEnd.Window-1, len(r.config.Alphabet)); err != nil {
		return nil, fmt.Errorf("asr: invalid stream file: %w", err)
	}
	var disk streamFileState
	if err := strictjson.Decode(bytes.NewReader(payload), int64(len(payload)), &disk); err != nil {
		return nil, fmt.Errorf("asr: decode stream file: %w", err)
	}
	individual, err := checkpoint.DecodeIndividual(disk.Individual)
	if err != nil {
		return nil, fmt.Errorf("asr: decode stream individual: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stream, err := r.RestoreStream(StreamState{
		Individual:      individual,
		Pending:         disk.Pending,
		PrevClass:       disk.PrevClass,
		Emitted:         disk.Emitted,
		SampleRate:      disk.SampleRate,
		FrontEnd:        disk.FrontEnd,
		FrontEndVersion: disk.FrontEndVersion,
		Alphabet:        disk.Alphabet,
		Flushed:         disk.Flushed,
	})
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return stream, nil
}

func requireStreamFileFields(payload []byte, maxPending, alphabetSize int) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil || fields == nil {
		return errors.New("payload must be an object")
	}
	for _, name := range []string{"individual", "pending", "prev_class", "emitted", "sample_rate", "front_end", "front_end_version", "alphabet", "flushed"} {
		value, ok := fields[name]
		if !ok {
			return fmt.Errorf("missing field %s", name)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("field %s is null", name)
		}
	}
	if _, err := boundedNumberArray(fields["pending"], "pending", maxPending); err != nil {
		return err
	}
	count, err := boundedNumberArray(fields["alphabet"], "alphabet", alphabetSize)
	if err != nil {
		return err
	}
	if count != alphabetSize {
		return fmt.Errorf("field alphabet has %d runes, want %d", count, alphabetSize)
	}
	var front map[string]json.RawMessage
	if err := json.Unmarshal(fields["front_end"], &front); err != nil || front == nil {
		return errors.New("field front_end must be an object")
	}
	for _, name := range []string{"window", "hop", "mels", "f_min", "f_max", "floor"} {
		value, ok := front[name]
		if !ok {
			return fmt.Errorf("missing field front_end.%s", name)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("field front_end.%s is null", name)
		}
	}
	return nil
}

// boundedNumberArray scans without materializing one RawMessage per item.
// In particular, a 64 MiB file with millions of one-byte pending values is
// rejected before it can expand into a much larger []float64 allocation.
func boundedNumberArray(raw []byte, name string, max int) (int, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	first, err := decoder.Token()
	if err != nil || first != json.Delim('[') {
		return 0, fmt.Errorf("field %s must be an array", name)
	}
	count := 0
	for decoder.More() {
		if count >= max {
			return 0, fmt.Errorf("field %s exceeds %d elements", name, max)
		}
		value, err := decoder.Token()
		if err != nil {
			return 0, fmt.Errorf("field %s[%d]: %w", name, count, err)
		}
		if _, ok := value.(json.Number); !ok {
			return 0, fmt.Errorf("field %s[%d] must be a number", name, count)
		}
		count++
	}
	if end, err := decoder.Token(); err != nil || end != json.Delim(']') {
		return 0, fmt.Errorf("field %s has no array end", name)
	}
	if _, err := decoder.Token(); err != io.EOF {
		return 0, fmt.Errorf("field %s has trailing data", name)
	}
	return count, nil
}
