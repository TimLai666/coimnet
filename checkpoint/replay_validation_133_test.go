package checkpoint

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/replay"
)

// TestCheckpointRejectsImpossibleReplayStates exercises each public
// checkpoint boundary independently. The replay states are reached through
// replay.New and Store.Add, then one persisted counter or fingerprint is
// edited so the checkpoint has a valid envelope but an impossible store
// history.
func TestCheckpointRejectsImpossibleReplayStates(t *testing.T) {
	cases := []struct {
		name      string
		makeState func(*testing.T) State
	}{
		{name: "hashed-only FIFO with non-hex input fingerprint", makeState: invalidHashedReplayState},
		{name: "keep reservoir capacity two after three adds with zero draws", makeState: invalidReservoirDrawReplayState},
		{name: "empty FIFO with one draw", makeState: invalidEmptyFIFOReplayState},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Run("State.Validate", func(t *testing.T) {
				state := testCase.makeState(t)
				before := mustJSON(t, state)
				if err := state.Validate(); err == nil {
					t.Fatal("accepted an impossible replay state")
				}
				if after := mustJSON(t, state); !bytes.Equal(after, before) {
					t.Fatal("State.Validate changed the caller state")
				}
			})

			t.Run("Save", func(t *testing.T) {
				state := testCase.makeState(t)
				before := mustJSON(t, state)
				dir := t.TempDir()
				path := filepath.Join(dir, "rejected.json")
				if err := Save(context.Background(), path, state); err == nil {
					t.Fatal("saved an impossible replay state")
				}
				if after := mustJSON(t, state); !bytes.Equal(after, before) {
					t.Fatal("Save changed the caller state")
				}
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("Save left an output at %q, stat error %v", path, err)
				}
				assertNoTemps(t, dir, filepath.Base(path))
			})

			t.Run("Load", func(t *testing.T) {
				state := testCase.makeState(t)
				payload := mustJSON(t, state)
				document := envelopeJSON(SchemaVersion, payload, checksumHex(payload))
				path := filepath.Join(t.TempDir(), "impossible.json")
				writeRaw(t, path, document)
				before := append([]byte(nil), document...)
				got, err := Load(context.Background(), path)
				if err == nil {
					t.Fatal("loaded an impossible replay state")
				}
				if !reflect.DeepEqual(got, State{}) {
					t.Fatalf("Load returned a non-zero state after rejection: %#v", got)
				}
				if after, readErr := os.ReadFile(path); readErr != nil {
					t.Fatal(readErr)
				} else if !bytes.Equal(after, before) {
					t.Fatal("Load changed the rejected input file")
				}
				if afterState := mustJSON(t, state); !bytes.Equal(afterState, payload) {
					t.Fatal("Load changed the caller state")
				}
			})
		})
	}
}

// TestCheckpointReplayRawAndHashedContinuation is the lawful control for
// both privacy modes. Each restored store must produce the same sample,
// report, Add result and final state as its uninterrupted counterpart.
func TestCheckpointReplayRawAndHashedContinuation(t *testing.T) {
	for _, storeRaw := range []bool{true, false} {
		name := "hashed"
		if storeRaw {
			name = "raw"
		}
		t.Run(name, func(t *testing.T) {
			buffer := replayValidationBuffer(storeRaw, replay.EvictionReservoir)
			full := newReplayValidationStore(t, buffer)
			checkpointStore := newReplayValidationStore(t, buffer)
			for step := uint64(1); step <= 3; step++ {
				item := replayValidationExperience(step)
				if err := full.Add(item); err != nil {
					t.Fatal(err)
				}
				if err := checkpointStore.Add(item); err != nil {
					t.Fatal(err)
				}
			}

			trainer := newDelayedTrainer(t, 7)
			state, err := NewState(trainer.Snapshot(), 1001, 0)
			if err != nil {
				t.Fatal(err)
			}
			snapshot := checkpointStore.Snapshot()
			state.Replay = &snapshot
			path := filepath.Join(t.TempDir(), "replay.json")
			if err := Save(context.Background(), path, state); err != nil {
				t.Fatal(err)
			}
			loaded, err := Load(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Replay == nil {
				t.Fatal("lawful replay state disappeared during round trip")
			}
			if !reflect.DeepEqual(*loaded.Replay, snapshot) {
				t.Fatalf("round trip changed replay snapshot:\n got %#v\nwant %#v", *loaded.Replay, snapshot)
			}
			resumed, err := replay.RestoreSnapshot(*loaded.Replay)
			if err != nil {
				t.Fatal(err)
			}

			fullItems, fullReport, err := full.Sample(2)
			if err != nil {
				t.Fatal(err)
			}
			resumedItems, resumedReport, err := resumed.Sample(2)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(resumedItems, fullItems) {
				t.Fatalf("resumed sample = %#v, uninterrupted sample = %#v", resumedItems, fullItems)
			}
			if !reflect.DeepEqual(resumedReport, fullReport) {
				t.Fatalf("resumed sample report = %#v, uninterrupted report = %#v", resumedReport, fullReport)
			}

			item := replayValidationExperience(4)
			if err := full.Add(item); err != nil {
				t.Fatal(err)
			}
			if err := resumed.Add(item); err != nil {
				t.Fatal(err)
			}
			if got, want := resumed.Report(), full.Report(); !reflect.DeepEqual(got, want) {
				t.Fatalf("resumed report = %#v, uninterrupted report = %#v", got, want)
			}
			if got, want := resumed.State(), full.State(); !reflect.DeepEqual(got, want) {
				t.Fatalf("resumed state = %#v, uninterrupted state = %#v", got, want)
			}
		})
	}
}

func invalidHashedReplayState(t *testing.T) State {
	t.Helper()
	store := newReplayValidationStore(t, replayValidationBuffer(false, replay.EvictionFIFO))
	for step := uint64(1); step <= 2; step++ {
		if err := store.Add(replayValidationExperience(step)); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := store.Snapshot()
	snapshot.State.Items[0].InputHash = strings.Repeat("z", 64)
	return stateWithReplaySnapshot(t, snapshot)
}

func invalidReservoirDrawReplayState(t *testing.T) State {
	t.Helper()
	store := newReplayValidationStore(t, replayValidationBuffer(true, replay.EvictionReservoir))
	for step := uint64(1); step <= 3; step++ {
		if err := store.Add(replayValidationExperience(step)); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := store.Snapshot()
	snapshot.State.Draws = 0
	return stateWithReplaySnapshot(t, snapshot)
}

func invalidEmptyFIFOReplayState(t *testing.T) State {
	t.Helper()
	store := newReplayValidationStore(t, replayValidationBuffer(true, replay.EvictionFIFO))
	snapshot := store.Snapshot()
	snapshot.State.Draws = 1
	return stateWithReplaySnapshot(t, snapshot)
}

func stateWithReplaySnapshot(t *testing.T, snapshot replay.Snapshot) State {
	t.Helper()
	trainer := newDelayedTrainer(t, 7)
	state, err := NewState(trainer.Snapshot(), 1001, 0)
	if err != nil {
		t.Fatal(err)
	}
	state.Replay = &snapshot
	return state
}

func replayValidationBuffer(storeRaw bool, eviction string) replay.Buffer {
	return replay.Buffer{
		Capacity: 2,
		Sampling: replay.SamplingUniform,
		Eviction: eviction,
		Seed:     7,
		Privacy: replay.PrivacyPolicy{
			StoreRaw:  storeRaw,
			Retention: replay.RetentionKeepUntilEvicted,
		},
	}
}

func replayValidationExperience(step uint64) replay.Experience {
	value := float64(step)
	return replay.Experience{
		TaskID: "checkpoint-replay",
		Step:   step,
		Input:  [][]float64{{value}, {value + 0.5}},
		Target: [][]float64{{value + 1}},
		Weight: 1,
		Split:  replay.SplitTrain,
	}
}

func newReplayValidationStore(t *testing.T, buffer replay.Buffer) *replay.Store {
	t.Helper()
	store, err := replay.New(buffer)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
