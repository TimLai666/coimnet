package checkpoint

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/TimLai666/coimnet/internal/fileio"
	"github.com/TimLai666/coimnet/learning"
)

// IndividualSchemaVersion identifies the envelope used for a continuous
// individual snapshot. It is deliberately distinct from SchemaVersion, which
// is the independent-episode checkpoint format.
const IndividualSchemaVersion = "coimnet-individual-checkpoint/v1"

// SaveIndividual validates and publishes a complete continuous individual
// snapshot. The caller's snapshot is never retained, and an existing path is
// never replaced.
func SaveIndividual(ctx context.Context, path string, snapshot learning.IndividualSnapshot) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if path == "" {
		return fmt.Errorf("checkpoint path must not be empty")
	}
	individual, err := learning.RestoreIndividual(snapshot)
	if err != nil {
		return fmt.Errorf("invalid individual snapshot: %w", err)
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	owned := normalizeIndividualSnapshot(individual.Snapshot())
	payload, err := json.Marshal(owned)
	if err != nil {
		return fmt.Errorf("marshal individual checkpoint payload: %w", err)
	}
	// A new snapshot has no nullable fields. Check the marshaled representation
	// as well as the incoming document so valid zero-length arrays are emitted as
	// [] and never become an unreadable null value.
	if err := checkUniqueJSONRejectNull(payload); err != nil {
		return fmt.Errorf("marshal individual checkpoint payload: %w", err)
	}
	document, err := marshalEnvelopeFor(IndividualSchemaVersion, payload)
	if err != nil {
		return fmt.Errorf("marshal individual checkpoint envelope: %w", err)
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	return publishDocument(ctx, path, document)
}

// LoadIndividual reads one complete continuous individual snapshot. It
// validates raw JSON before encoding/json can replace invalid Unicode or turn
// null numeric values into Go zero values, then validates the semantic snapshot
// through learning.RestoreIndividual.
func LoadIndividual(ctx context.Context, path string) (learning.IndividualSnapshot, error) {
	if err := contextError(ctx); err != nil {
		return learning.IndividualSnapshot{}, err
	}
	if path == "" {
		return learning.IndividualSnapshot{}, fmt.Errorf("checkpoint path must not be empty")
	}
	data, err := fileio.ReadRegular(ctx, path, maxCheckpointBytes)
	if err != nil {
		return learning.IndividualSnapshot{}, fmt.Errorf("read individual checkpoint: %w", err)
	}
	if err := contextError(ctx); err != nil {
		return learning.IndividualSnapshot{}, err
	}
	snapshot, err := decodeIndividualDocument(data)
	if err != nil {
		return learning.IndividualSnapshot{}, err
	}
	if err := contextError(ctx); err != nil {
		return learning.IndividualSnapshot{}, err
	}
	return snapshot, nil
}

func decodeIndividualDocument(data []byte) (learning.IndividualSnapshot, error) {
	var empty learning.IndividualSnapshot
	if err := validateJSONUnicode(data); err != nil {
		return empty, fmt.Errorf("decode individual checkpoint: %w", err)
	}
	if err := checkUniqueJSONRejectNull(data); err != nil {
		return empty, err
	}
	if err := requireIndividualFields(data); err != nil {
		return empty, err
	}
	var raw envelope
	if err := decodeStrict(data, &raw); err != nil {
		return empty, fmt.Errorf("decode individual checkpoint envelope: %w", err)
	}
	if raw.SchemaVersion != IndividualSchemaVersion {
		return empty, fmt.Errorf("unsupported individual checkpoint schema %q", raw.SchemaVersion)
	}
	payload := bytes.TrimSpace(raw.Payload)
	if len(payload) == 0 || bytes.Equal(payload, []byte("null")) {
		return empty, fmt.Errorf("individual checkpoint payload must be an object")
	}
	if len(raw.Checksum) != sha256.Size*2 {
		return empty, fmt.Errorf("invalid individual checkpoint checksum encoding")
	}
	got, err := hex.DecodeString(raw.Checksum)
	if err != nil {
		return empty, fmt.Errorf("invalid individual checkpoint checksum encoding: %w", err)
	}
	want := sha256.Sum256(raw.Payload)
	if !bytes.Equal(got, want[:]) {
		return empty, fmt.Errorf("individual checkpoint payload checksum mismatch")
	}
	var snapshot learning.IndividualSnapshot
	if err := decodeStrict(raw.Payload, &snapshot); err != nil {
		return empty, fmt.Errorf("decode individual checkpoint payload: %w", err)
	}
	individual, err := learning.RestoreIndividual(snapshot)
	if err != nil {
		return empty, fmt.Errorf("invalid individual snapshot: %w", err)
	}
	// Normalization is a wire-format concern only. Return the runtime's
	// canonical snapshot so nil-versus-empty optional slices retain their
	// documented semantics for callers.
	return individual.Snapshot(), nil
}

// requireIndividualFields closes the gap between strict decoding and presence
// validation. encoding/json cannot distinguish an omitted scalar from a
// present zero, so every required field is checked in the raw object first.
// InputNodes and Delays retain their documented legacy omission semantics.
func requireIndividualFields(data []byte) error {
	outer, err := requiredObject(data, "$", "schema_version", "payload", "checksum")
	if err != nil {
		return err
	}
	payload := outer["payload"]
	payloadObject, err := requiredObject(payload, "$.payload", "schema_version", "profile", "config_hash", "config", "parameters", "neural", "optimizer")
	if err != nil {
		return err
	}
	config, err := requiredObject(payloadObject["config"], "$.payload.config", "dynamics", "input_size", "output_size", "readout_nodes")
	if err != nil {
		return err
	}
	dynamics, err := requiredObject(config["dynamics"], "$.payload.config.dynamics", "nodes", "sources", "targets", "dt", "activation")
	if err != nil {
		return err
	}
	_ = dynamics
	parameters, err := requiredObject(payloadObject["parameters"], "$.payload.parameters", "core", "encoder", "readout")
	if err != nil {
		return err
	}
	if _, err = requiredObject(parameters["core"], "$.payload.parameters.core", "weights", "bias", "log_tau"); err != nil {
		return err
	}
	optimizer, err := requiredObject(payloadObject["optimizer"], "$.payload.optimizer", "options", "state", "updates")
	if err != nil {
		return err
	}
	options, err := requiredObject(optimizer["options"], "$.payload.optimizer.options", "learning_rate", "beta1", "beta2", "epsilon", "weight_decay", "clip_norm", "truncation", "trainable")
	if err != nil {
		return err
	}
	if _, err = requiredObject(options["trainable"], "$.payload.optimizer.options.trainable", "encoder", "weights", "bias", "tau", "readout"); err != nil {
		return err
	}
	if _, err = requiredObject(optimizer["state"], "$.payload.optimizer.state", "first", "second", "steps"); err != nil {
		return err
	}
	if _, err = requiredObject(payloadObject["neural"], "$.payload.neural", "schema_version", "config_hash", "steps", "voltage", "history"); err != nil {
		return err
	}
	return nil
}

func requiredObject(data []byte, path string, fields ...string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, fmt.Errorf("required object %s: %w", path, err)
	}
	if object == nil {
		return nil, fmt.Errorf("required object %s must not be null", path)
	}
	for _, field := range fields {
		if _, ok := object[field]; !ok {
			return nil, fmt.Errorf("missing required field %s.%s", path, field)
		}
	}
	return object, nil
}

// normalizeIndividualSnapshot changes nil required arrays to empty arrays so
// their JSON representation is [] rather than null. InputNodes is intentionally
// preserved because nil means the full core receives encoded observations.
func normalizeIndividualSnapshot(s learning.IndividualSnapshot) learning.IndividualSnapshot {
	s.Config.Dynamics.Sources = nonNilInts(s.Config.Dynamics.Sources)
	s.Config.Dynamics.Targets = nonNilInts(s.Config.Dynamics.Targets)
	s.Config.Dynamics.Delays = nonNilInts(s.Config.Dynamics.Delays)
	s.Parameters.Core.Weights = nonNilFloats(s.Parameters.Core.Weights)
	s.Parameters.Core.Bias = nonNilFloats(s.Parameters.Core.Bias)
	s.Parameters.Core.LogTau = nonNilFloats(s.Parameters.Core.LogTau)
	s.Parameters.Encoder = nonNilFloats(s.Parameters.Encoder)
	s.Parameters.Readout = nonNilFloats(s.Parameters.Readout)
	s.Optimizer.State.First = nonNilFloats(s.Optimizer.State.First)
	s.Optimizer.State.Second = nonNilFloats(s.Optimizer.State.Second)
	s.Optimizer.State.Steps = nonNilUint64(s.Optimizer.State.Steps)
	return s
}

func nonNilInts(values []int) []int {
	if values == nil {
		return []int{}
	}
	return values
}
func nonNilFloats(values []float64) []float64 {
	if values == nil {
		return []float64{}
	}
	return values
}
func nonNilUint64(values []uint64) []uint64 {
	if values == nil {
		return []uint64{}
	}
	return values
}

// validateJSONUnicode runs before encoding/json can replace invalid UTF-8 or
// unpaired UTF-16 escapes with U+FFFD. Syntax is validated separately by the
// duplicate/depth scanner and strict decoder.
func validateJSONUnicode(data []byte) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("JSON text must be valid UTF-8")
	}
	for i := 0; i < len(data); i++ {
		if data[i] != '\\' {
			continue
		}
		i++
		if i >= len(data) {
			return fmt.Errorf("incomplete JSON escape")
		}
		if data[i] != 'u' {
			continue
		}
		if i+4 >= len(data) {
			return fmt.Errorf("incomplete JSON Unicode escape")
		}
		code, err := strconv.ParseUint(string(data[i+1:i+5]), 16, 16)
		if err != nil {
			return fmt.Errorf("invalid JSON Unicode escape")
		}
		i += 4
		if code >= 0xdc00 && code <= 0xdfff {
			return fmt.Errorf("unpaired low surrogate in JSON text")
		}
		if code < 0xd800 || code > 0xdbff {
			continue
		}
		if i+6 >= len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
			return fmt.Errorf("unpaired high surrogate in JSON text")
		}
		low, err := strconv.ParseUint(string(data[i+3:i+7]), 16, 16)
		if err != nil || low < 0xdc00 || low > 0xdfff {
			return fmt.Errorf("invalid low surrogate in JSON text")
		}
		i += 6
	}
	return nil
}
