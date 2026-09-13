package checkpoint

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/TimLai666/coimnet/experiment"
	"github.com/TimLai666/coimnet/learning"
)

const helperEnv = "COIMNET_CHECKPOINT_HELPER"

func TestNewStateCopiesAndValidates(t *testing.T) {
	trainer := newDelayedTrainer(t, 7)
	snapshot := trainer.Snapshot()
	state, err := NewState(snapshot, 1001, 0)
	if err != nil {
		t.Fatal(err)
	}

	snapshot.Parameters.Core.Weights[0] = 99
	if state.Training.Parameters.Core.Weights[0] == 99 {
		t.Fatal("NewState retained caller-owned parameter slice")
	}
	if _, err := NewState(trainer.Snapshot(), 1001, 1); err == nil {
		t.Fatal("accepted a cursor different from the training update count")
	}

	bad := state
	bad.Training.Parameters.Core.Weights = []float64{1}
	if err := bad.Validate(); err == nil {
		t.Fatal("accepted a malformed training snapshot")
	}
}

func TestSaveLoadRoundTripAndAliasIsolation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "episode.json")
	trainer := newDelayedTrainer(t, 7)
	train(t, trainer, 1001, 0, 3)
	state, err := NewState(trainer.Snapshot(), 1001, 3)
	if err != nil {
		t.Fatal(err)
	}
	want, err := NewState(state.Training, state.DataSeed, state.NextSample)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(context.Background(), path, state); err != nil {
		t.Fatal(err)
	}

	state.Training.Parameters.Core.Weights[0] = 88
	state.Training.Optimizer.First[0] = 77
	got, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip changed state:\n got %#v\nwant %#v", got, want)
	}

	got.Training.Parameters.Core.Bias[0] = 66
	got.Training.Optimizer.Second[0] = 55
	again, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again, want) {
		t.Fatal("Load returned buffers aliased with a later caller mutation")
	}
}

func TestSaveDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "episode.json")
	state := testState(t)
	if err := Save(context.Background(), path, state); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	state.DataSeed++
	if err := Save(context.Background(), path, state); err == nil {
		t.Fatal("overwrote an existing checkpoint")
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(current, original) {
		t.Fatal("failed overwrite changed the original checkpoint")
	}
	assertNoTemps(t, dir, filepath.Base(path))
}

func TestConcurrentSavesHaveOneWinner(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "episode.json")
	state := testState(t)
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- Save(context.Background(), path, state)
		}()
	}
	wg.Wait()
	close(results)
	var successes int
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent saves had %d successful publishers, want 1", successes)
	}
	if _, err := Load(context.Background(), path); err != nil {
		t.Fatalf("winner is not loadable: %v", err)
	}
	assertNoTemps(t, dir, filepath.Base(path))
}

func TestLoadRejectsCorruptUnknownDuplicateAndTrailingJSON(t *testing.T) {
	dir := t.TempDir()
	state := testState(t)
	validPayload := mustJSON(t, state)
	checksum := checksumHex(validPayload)

	writeRaw(t, filepath.Join(dir, "corrupt.json"), append([]byte(`{"`), []byte("not a checkpoint")...))
	if _, err := Load(context.Background(), filepath.Join(dir, "corrupt.json")); err == nil {
		t.Fatal("accepted corrupt JSON")
	}

	unknownVersion := envelopeJSON("unknown-envelope-v1", validPayload, checksum)
	writeRaw(t, filepath.Join(dir, "unknown-version.json"), unknownVersion)
	if _, err := Load(context.Background(), filepath.Join(dir, "unknown-version.json")); err == nil {
		t.Fatal("accepted an unknown envelope version")
	}

	duplicate := []byte(fmt.Sprintf(`{"schema_version":%q,"schema_version":%q,"payload":%s,"checksum":%q}`, SchemaVersion, SchemaVersion, validPayload, checksum))
	writeRaw(t, filepath.Join(dir, "duplicate.json"), duplicate)
	if _, err := Load(context.Background(), filepath.Join(dir, "duplicate.json")); err == nil {
		t.Fatal("accepted duplicate JSON keys")
	}

	duplicateFolded := []byte(fmt.Sprintf(`{"schema_version":%q,"Schema_Version":%q,"payload":%s,"checksum":%q}`, SchemaVersion, SchemaVersion, validPayload, checksum))
	writeRaw(t, filepath.Join(dir, "duplicate-folded.json"), duplicateFolded)
	if _, err := Load(context.Background(), filepath.Join(dir, "duplicate-folded.json")); err == nil {
		t.Fatal("accepted duplicate JSON keys differing only by case")
	}

	unknownField := []byte(fmt.Sprintf(`{"schema_version":%q,"payload":%s,"checksum":%q,"extra":true}`, SchemaVersion, validPayload, checksum))
	writeRaw(t, filepath.Join(dir, "unknown-field.json"), unknownField)
	if _, err := Load(context.Background(), filepath.Join(dir, "unknown-field.json")); err == nil {
		t.Fatal("accepted an unknown envelope field")
	}

	trailing := append(envelopeJSON(SchemaVersion, validPayload, checksum), []byte("\n{}")...)
	writeRaw(t, filepath.Join(dir, "trailing.json"), trailing)
	if _, err := Load(context.Background(), filepath.Join(dir, "trailing.json")); err == nil {
		t.Fatal("accepted trailing JSON")
	}
}

func TestLoadRejectsPayloadVersionShapeAndNonFiniteValues(t *testing.T) {
	dir := t.TempDir()
	state := testState(t)

	state.SchemaVersion = "unknown-training-envelope"
	payload := mustJSON(t, state)
	writeRaw(t, filepath.Join(dir, "payload-version.json"), envelopeJSON(SchemaVersion, payload, checksumHex(payload)))
	if _, err := Load(context.Background(), filepath.Join(dir, "payload-version.json")); err == nil {
		t.Fatal("accepted an unknown payload version")
	}

	state = testState(t)
	state.Training.Parameters.Core.Weights = []float64{1}
	payload = mustJSON(t, state)
	writeRaw(t, filepath.Join(dir, "shape.json"), envelopeJSON(SchemaVersion, payload, checksumHex(payload)))
	if _, err := Load(context.Background(), filepath.Join(dir, "shape.json")); err == nil {
		t.Fatal("accepted a malformed parameter shape")
	}

	state = testState(t)
	payload = mustJSON(t, state)
	marker := []byte(`"weights":[`)
	start := bytes.Index(payload, marker)
	if start < 0 {
		t.Fatal("test payload has no weights")
	}
	start += len(marker)
	end := bytes.IndexByte(payload[start:], ',')
	if end < 0 {
		t.Fatal("test payload has one weight only")
	}
	end += start
	mutated := append([]byte(nil), payload[:start]...)
	mutated = append(mutated, []byte("1e1000")...)
	mutated = append(mutated, payload[end:]...)
	writeRaw(t, filepath.Join(dir, "nonfinite.json"), envelopeJSON(SchemaVersion, mutated, checksumHex(mutated)))
	if _, err := Load(context.Background(), filepath.Join(dir, "nonfinite.json")); err == nil {
		t.Fatal("accepted an out-of-range non-finite JSON number")
	}
}

func TestLoadIsBoundedAndContextAware(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "oversized.json")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(int64(maxCheckpointBytes) + 1); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(context.Background(), path); err == nil {
		t.Fatal("accepted an oversized checkpoint")
	}

	state := testState(t)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Save(canceled, filepath.Join(dir, "canceled.json"), state); !errors.Is(err, context.Canceled) {
		t.Fatalf("Save error = %v, want context.Canceled", err)
	}
	if _, err := Load(canceled, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("Load error = %v, want context.Canceled", err)
	}
	assertNoTemps(t, dir, "canceled.json")
}

func TestEnvelopeSizeBoundAppliesBeforeSave(t *testing.T) {
	if _, err := marshalEnvelope(make([]byte, maxCheckpointBytes+1)); err == nil {
		t.Fatal("accepted a payload larger than the Load limit")
	}
}

func TestCanceledAfterPublicationLeavesLoadableCheckpoint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "published.json")
	ctx := &cancelOnErrCall{Context: context.Background(), cancelAt: 8}
	if err := Save(ctx, path, testState(t)); !errors.Is(err, context.Canceled) {
		t.Fatalf("Save error = %v, want context.Canceled", err)
	} else if !strings.Contains(err.Error(), "checkpoint published") {
		t.Fatalf("post-publication error = %v, want publication status", err)
	}
	if _, err := Load(context.Background(), path); err != nil {
		t.Fatalf("published checkpoint is not loadable: %v", err)
	}
	assertNoTemps(t, dir, filepath.Base(path))
}

func TestLoadChecksContextAfterValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "valid.json")
	if err := Save(context.Background(), path, testState(t)); err != nil {
		t.Fatal(err)
	}
	ctx := &cancelOnErrCall{Context: context.Background(), cancelAt: 5}
	if _, err := Load(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("Load error = %v, want context.Canceled from final check", err)
	}
}

func TestSubprocessResumeMatchesUninterruptedTraining(t *testing.T) {
	if os.Getenv(helperEnv) == "1" {
		t.Skip("helper is tested separately")
	}
	dir := t.TempDir()
	checkpointPath := filepath.Join(dir, "midpoint.json")
	resumedPath := filepath.Join(dir, "resumed.json")
	const total = 9
	const split = 4
	const dataSeed = uint64(1001)

	full := newDelayedTrainer(t, 7)
	train(t, full, dataSeed, 0, total)
	fullSnapshot := full.Snapshot()
	fullPredictions := predictions(t, full, 1003, 5)

	part := newDelayedTrainer(t, 7)
	train(t, part, dataSeed, 0, split)
	midpoint, err := NewState(part.Snapshot(), dataSeed, split)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(context.Background(), checkpointPath, midpoint); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestCheckpointHelperProcess$", "--", checkpointPath, resumedPath, strconv.Itoa(total))
	cmd.Env = append(os.Environ(), helperEnv+"=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("helper failed: %v\n%s", err, output)
	}
	resumedState, err := Load(context.Background(), resumedPath)
	if err != nil {
		t.Fatal(err)
	}
	if resumedState.NextSample != total {
		t.Fatalf("resumed cursor = %d, want %d", resumedState.NextSample, total)
	}
	resumed, err := learning.RestoreTrainer(resumedState.Training)
	if err != nil {
		t.Fatal(err)
	}
	if got := resumed.Snapshot(); !reflect.DeepEqual(got, fullSnapshot) {
		t.Fatalf("resumed snapshot differs from uninterrupted snapshot:\n got %#v\nwant %#v", got, fullSnapshot)
	}
	if got := predictions(t, resumed, 1003, 5); !reflect.DeepEqual(got, fullPredictions) {
		t.Fatalf("resumed predictions differ:\n got %#v\nwant %#v", got, fullPredictions)
	}
}

func TestCheckpointHelperProcess(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		return
	}
	args := os.Args
	separator := -1
	for i, arg := range args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator < 0 || len(args)-separator != 4 {
		t.Fatalf("helper arguments = %#v", args)
	}
	inputPath, outputPath := args[separator+1], args[separator+2]
	total, err := strconv.ParseUint(args[separator+3], 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	state, err := Load(context.Background(), inputPath)
	if err != nil {
		t.Fatal(err)
	}
	trainer, err := learning.RestoreTrainer(state.Training)
	if err != nil {
		t.Fatal(err)
	}
	if state.NextSample > total {
		t.Fatalf("checkpoint cursor %d exceeds target %d", state.NextSample, total)
	}
	train(t, trainer, state.DataSeed, state.NextSample, total)
	final, err := NewState(trainer.Snapshot(), state.DataSeed, total)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(context.Background(), outputPath, final); err != nil {
		t.Fatal(err)
	}
}

func newDelayedTrainer(t *testing.T, seed uint64) *learning.Trainer {
	t.Helper()
	trainer, err := experiment.NewDelayedTrainer(seed, .02, false)
	if err != nil {
		t.Fatal(err)
	}
	return trainer
}

func testState(t *testing.T) State {
	t.Helper()
	trainer := newDelayedTrainer(t, 7)
	state, err := NewState(trainer.Snapshot(), 1001, 0)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func train(t *testing.T, trainer *learning.Trainer, seed, start, end uint64) {
	t.Helper()
	for i := start; i < end; i++ {
		ep := experiment.DelayedEpisode(seed, i)
		if _, err := trainer.Step(context.Background(), ep.Input, ep.Target); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
}

func predictions(t *testing.T, trainer *learning.Trainer, seed uint64, count int) [][]float64 {
	t.Helper()
	out := make([][]float64, count)
	for i := range out {
		ep := experiment.DelayedEpisode(seed, uint64(i))
		prediction, err := trainer.Predict(context.Background(), ep.Input)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = prediction
	}
	return out
}

type testEnvelope struct {
	SchemaVersion string          `json:"schema_version"`
	Payload       json.RawMessage `json:"payload"`
	Checksum      string          `json:"checksum"`
}

func envelopeJSON(schema string, payload []byte, checksum string) []byte {
	return mustJSONValue(testEnvelope{SchemaVersion: schema, Payload: payload, Checksum: checksum})
}

func checksumHex(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func mustJSONValue(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func writeRaw(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func assertNoTemps(t *testing.T, dir, base string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "."+base+".tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Fatalf("temporary checkpoint files remain: %v", paths)
	}
}

type cancelOnErrCall struct {
	context.Context
	cancelAt int
	calls    int
}

func (c *cancelOnErrCall) Err() error {
	c.calls++
	if c.calls >= c.cancelAt {
		return context.Canceled
	}
	return nil
}
