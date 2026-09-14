// Package checkpoint persists versioned independent-episode and continuous-
// individual snapshots. The two formats intentionally remain separate.
package checkpoint

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/TimLai666/coimnet/internal/fileio"
	"github.com/TimLai666/coimnet/internal/jsonkey"
	"github.com/TimLai666/coimnet/learning"
)

const (
	// SchemaVersion identifies the complete checkpoint envelope and payload.
	SchemaVersion = "coimnet-episode-checkpoint/v1"
	// Generator identifies the counter-based delayed-pulse sample sequence.
	Generator = "delayed-pulse-splitmix64-v1"

	maxCheckpointBytes = 64 << 20
	writeChunkBytes    = 32 << 10
)

// State is the complete state for the independent-episode trainer and its
// counter-based sample generator. It intentionally excludes continuous
// individual state, which needs a different snapshot profile.
type State struct {
	SchemaVersion string                    `json:"schema_version"`
	Training      learning.TrainingSnapshot `json:"training"`
	Generator     string                    `json:"generator"`
	DataSeed      uint64                    `json:"data_seed"`
	NextSample    uint64                    `json:"next_sample"`
}

type envelope struct {
	SchemaVersion string          `json:"schema_version"`
	Payload       json.RawMessage `json:"payload"`
	Checksum      string          `json:"checksum"`
}

// NewState validates and owns a training snapshot at an episode boundary.
// The delayed-pulse generator has one sample per trainer update, so its cursor
// must equal the snapshot update count.
func NewState(snapshot learning.TrainingSnapshot, dataSeed, next uint64) (State, error) {
	trainer, err := learning.RestoreTrainer(snapshot)
	if err != nil {
		return State{}, fmt.Errorf("invalid training snapshot: %w", err)
	}
	owned := trainer.Snapshot()
	if next != owned.Updates {
		return State{}, fmt.Errorf("next sample %d does not equal training updates %d", next, owned.Updates)
	}
	return State{SchemaVersion: SchemaVersion, Training: owned, Generator: Generator, DataSeed: dataSeed, NextSample: next}, nil
}

// Validate checks the versioned generator contract and the complete trainer
// snapshot without exposing or retaining a mutable trainer.
func (s State) Validate() error {
	_, err := canonicalTraining(s)
	return err
}

func canonicalTraining(s State) (learning.TrainingSnapshot, error) {
	if s.SchemaVersion != SchemaVersion {
		return learning.TrainingSnapshot{}, fmt.Errorf("unsupported checkpoint schema %q", s.SchemaVersion)
	}
	if s.Generator != Generator {
		return learning.TrainingSnapshot{}, fmt.Errorf("unsupported checkpoint generator %q", s.Generator)
	}
	if s.NextSample != s.Training.Updates {
		return learning.TrainingSnapshot{}, fmt.Errorf("next sample %d does not equal training updates %d", s.NextSample, s.Training.Updates)
	}
	trainer, err := learning.RestoreTrainer(s.Training)
	if err != nil {
		return learning.TrainingSnapshot{}, fmt.Errorf("invalid training snapshot: %w", err)
	}
	return trainer.Snapshot(), nil
}

func canonicalState(s State) (State, error) {
	training, err := canonicalTraining(s)
	if err != nil {
		return State{}, err
	}
	s.Training = training
	return s, nil
}

// Save publishes one complete checkpoint without replacing an existing path.
// The temporary file is created in the destination directory, synced, and
// published by an exclusive hardlink. A directory sync makes the link
// durable where the filesystem supports directory synchronization.
func Save(ctx context.Context, path string, state State) (retErr error) {
	if err := contextError(ctx); err != nil {
		return err
	}
	if path == "" {
		return fmt.Errorf("checkpoint path must not be empty")
	}
	owned, err := canonicalState(state)
	if err != nil {
		return err
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	payload, err := json.Marshal(owned)
	if err != nil {
		return fmt.Errorf("marshal checkpoint payload: %w", err)
	}
	document, err := marshalEnvelope(payload)
	if err != nil {
		return fmt.Errorf("marshal checkpoint envelope: %w", err)
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	return publishDocument(ctx, path, document)
}

// Load reads and validates one complete checkpoint using a fixed size bound.
func Load(ctx context.Context, path string) (state State, retErr error) {
	if err := contextError(ctx); err != nil {
		return State{}, err
	}
	if path == "" {
		return State{}, fmt.Errorf("checkpoint path must not be empty")
	}
	data, err := fileio.ReadRegular(ctx, path, maxCheckpointBytes)
	if err != nil {
		return State{}, fmt.Errorf("read checkpoint: %w", err)
	}
	if err := contextError(ctx); err != nil {
		return State{}, err
	}
	state, err = decodeDocument(data)
	if err != nil {
		return State{}, err
	}
	if err := contextError(ctx); err != nil {
		return State{}, err
	}
	return state, nil
}

func marshalEnvelope(payload []byte) ([]byte, error) {
	return marshalEnvelopeFor(SchemaVersion, payload)
}

func marshalEnvelopeFor(schema string, payload []byte) ([]byte, error) {
	if schema == "" {
		return nil, fmt.Errorf("checkpoint schema must not be empty")
	}
	if len(payload) > maxCheckpointBytes {
		return nil, fmt.Errorf("checkpoint exceeds %d byte limit", maxCheckpointBytes)
	}
	sum := sha256.Sum256(payload)
	document, err := json.Marshal(envelope{SchemaVersion: schema, Payload: json.RawMessage(payload), Checksum: hex.EncodeToString(sum[:])})
	if err != nil {
		return nil, err
	}
	if len(document) > maxCheckpointBytes {
		return nil, fmt.Errorf("checkpoint exceeds %d byte limit", maxCheckpointBytes)
	}
	return document, nil
}

// publishDocument writes and publishes one already validated JSON document.
// The temporary file is created next to the destination, synced, and linked
// with exclusive-create semantics. Existing callers depend on cancellation
// before publication cleaning up the temporary file, while cancellation after
// publication still completes the durability step.
func publishDocument(ctx context.Context, path string, document []byte) (retErr error) {
	if path == "" {
		return fmt.Errorf("checkpoint path must not be empty")
	}
	if len(document) > maxCheckpointBytes {
		return fmt.Errorf("checkpoint exceeds %d byte limit", maxCheckpointBytes)
	}

	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create checkpoint temporary file: %w", err)
	}
	tempPath := temp.Name()
	removeTemp := true
	tempClosed := false
	defer func() {
		if !tempClosed {
			tempClosed = true
			if closeErr := temp.Close(); closeErr != nil {
				retErr = errors.Join(retErr, fmt.Errorf("close checkpoint temporary file: %w", closeErr))
			}
		}
		if removeTemp {
			if removeErr := os.Remove(tempPath); removeErr != nil && !os.IsNotExist(removeErr) {
				retErr = errors.Join(retErr, fmt.Errorf("remove checkpoint temporary file: %w", removeErr))
			}
		}
	}()

	if err := writeContext(ctx, temp, document); err != nil {
		return err
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync checkpoint temporary file: %w", err)
	}
	if err := temp.Close(); err != nil {
		tempClosed = true
		return fmt.Errorf("close checkpoint temporary file: %w", err)
	}
	tempClosed = true
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := os.Link(tempPath, path); err != nil {
		return fmt.Errorf("publish checkpoint without overwrite: %w", err)
	}
	removeErr := os.Remove(tempPath)
	if removeErr == nil {
		removeTemp = false
	}
	// The destination is visible once Link succeeds. Finish the directory
	// sync even if the caller cancels after publication, so cancellation never
	// skips the durability step or removes the newly published checkpoint.
	syncErr := syncDirectory(context.Background(), dir)
	contextErr := contextError(ctx)
	if removeErr != nil {
		retErr = errors.Join(retErr, fmt.Errorf("checkpoint published but temporary cleanup failed: %w", removeErr))
	}
	if syncErr != nil {
		retErr = errors.Join(retErr, fmt.Errorf("checkpoint published but durability unconfirmed: %w", syncErr))
	}
	if contextErr != nil {
		retErr = errors.Join(retErr, fmt.Errorf("checkpoint published; context ended after publication: %w", contextErr))
	}
	return retErr
}

func decodeDocument(data []byte) (State, error) {
	if err := checkUniqueJSON(data); err != nil {
		return State{}, err
	}
	var raw envelope
	if err := decodeStrict(data, &raw); err != nil {
		return State{}, fmt.Errorf("decode checkpoint envelope: %w", err)
	}
	if raw.SchemaVersion != SchemaVersion {
		return State{}, fmt.Errorf("unsupported checkpoint schema %q", raw.SchemaVersion)
	}
	if len(bytes.TrimSpace(raw.Payload)) == 0 || bytes.Equal(bytes.TrimSpace(raw.Payload), []byte("null")) {
		return State{}, fmt.Errorf("checkpoint payload must be an object")
	}
	if len(raw.Checksum) != sha256.Size*2 {
		return State{}, fmt.Errorf("invalid checkpoint checksum encoding")
	}
	got, err := hex.DecodeString(raw.Checksum)
	if err != nil {
		return State{}, fmt.Errorf("invalid checkpoint checksum encoding: %w", err)
	}
	want := sha256.Sum256(raw.Payload)
	if !bytes.Equal(got, want[:]) {
		return State{}, fmt.Errorf("checkpoint payload checksum mismatch")
	}
	// Missing or null required values must fail before encoding/json would
	// silently decode them as zero.
	if err := checkRequiredFields(raw.Payload, reflect.TypeOf(State{}), "payload"); err != nil {
		return State{}, fmt.Errorf("decode checkpoint payload: %w", err)
	}
	var state State
	if err := decodeStrict(raw.Payload, &state); err != nil {
		return State{}, fmt.Errorf("decode checkpoint payload: %w", err)
	}
	owned, err := canonicalState(state)
	if err != nil {
		return State{}, err
	}
	return owned, nil
}

func decodeStrict(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return fmt.Errorf("trailing JSON: %w", err)
	}
	return nil
}

// maxJSONDepth bounds nesting during the duplicate-key walk; json.Decoder.Token
// itself has no nesting limit.
const maxJSONDepth = 64

type jsonPathPart struct {
	key   string
	index int
	array bool
}

// checkUniqueJSON is a small token scanner because encoding/json's strict
// decoder rejects unknown fields but accepts duplicate object keys.
func checkUniqueJSON(data []byte) error {
	return checkUniqueJSONWithOptions(data, false)
}

func checkUniqueJSONRejectNull(data []byte) error {
	return checkUniqueJSONWithOptions(data, true)
}

func checkUniqueJSONWithOptions(data []byte, rejectNull bool) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var path []jsonPathPart
	if err := scanJSONValueWithOptions(decoder, &path, rejectNull); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value %v", token)
		}
		return fmt.Errorf("trailing JSON: %w", err)
	}
	return nil
}

// scanJSONValueWithOptions walks one JSON value. The path is kept as a stack and only
// formatted when an error is reported, so the walk allocates per key, not per
// key times depth.
func scanJSONValueWithOptions(decoder *json.Decoder, path *[]jsonPathPart, rejectNull bool) error {
	if len(*path) > maxJSONDepth {
		return fmt.Errorf("JSON nesting exceeds %d levels at %s", maxJSONDepth, formatJSONPath(*path))
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		switch token.(type) {
		case nil:
			if rejectNull {
				return fmt.Errorf("JSON null is not permitted at %s", formatJSONPath(*path))
			}
			return nil
		case bool, string, json.Number:
			return nil
		default:
			return fmt.Errorf("unexpected token %T at %s", token, formatJSONPath(*path))
		}
	}
	switch delim {
	case '{':
		seen := make(map[string]string)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key is %T at %s", keyToken, formatJSONPath(*path))
			}
			folded := jsonkey.Fold(key)
			if previous, exists := seen[folded]; exists {
				return fmt.Errorf("duplicate JSON key %q conflicts with %q at %s", key, previous, formatJSONPath(*path))
			}
			seen[folded] = key
			*path = append(*path, jsonPathPart{key: key})
			err = scanJSONValueWithOptions(decoder, path, rejectNull)
			*path = (*path)[:len(*path)-1]
			if err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return fmt.Errorf("object ended with %v at %s", end, formatJSONPath(*path))
		}
		return nil
	case '[':
		index := 0
		for decoder.More() {
			*path = append(*path, jsonPathPart{index: index, array: true})
			err = scanJSONValueWithOptions(decoder, path, rejectNull)
			*path = (*path)[:len(*path)-1]
			if err != nil {
				return err
			}
			index++
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return fmt.Errorf("array ended with %v at %s", end, formatJSONPath(*path))
		}
		return nil
	default:
		return fmt.Errorf("unexpected delimiter %q at %s", delim, formatJSONPath(*path))
	}
}

func formatJSONPath(path []jsonPathPart) string {
	var builder strings.Builder
	builder.WriteByte('$')
	for _, part := range path {
		if part.array {
			builder.WriteByte('[')
			builder.WriteString(strconv.Itoa(part.index))
			builder.WriteByte(']')
			continue
		}
		builder.WriteByte('.')
		builder.WriteString(part.key)
	}
	return builder.String()
}

func writeContext(ctx context.Context, writer io.Writer, data []byte) error {
	for len(data) > 0 {
		if err := contextError(ctx); err != nil {
			return err
		}
		count := len(data)
		if count > writeChunkBytes {
			count = writeChunkBytes
		}
		written, err := writer.Write(data[:count])
		if written < 0 || written > count {
			return fmt.Errorf("invalid checkpoint write count %d", written)
		}
		if written == 0 && err == nil {
			return io.ErrShortWrite
		}
		data = data[written:]
		if err != nil {
			return fmt.Errorf("write checkpoint: %w", err)
		}
	}
	return contextError(ctx)
}

func readBounded(ctx context.Context, reader io.Reader, limit int) ([]byte, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(reader, int64(limit)+1))
	if err != nil {
		return nil, fmt.Errorf("read checkpoint: %w", err)
	}
	if len(data) > limit {
		return nil, fmt.Errorf("checkpoint exceeds %d byte limit", limit)
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	return data, nil
}

func syncDirectory(ctx context.Context, path string) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open checkpoint directory for sync: %w", err)
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil || closeErr != nil {
		return errors.Join(wrapError("sync checkpoint directory", syncErr), wrapError("close checkpoint directory", closeErr))
	}
	return contextError(ctx)
}

func wrapError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("nil context")
	}
	return ctx.Err()
}
