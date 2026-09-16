package checkpoint

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/replay"
)

// TestEpisodeCheckpointOmitsUnusedConstraintFields keeps every checkpoint
// written before ticket 17 loadable: the masks, the ranges, the edge signs and
// the log-magnitude floor are optional and are not written when unused.
func TestEpisodeCheckpointOmitsUnusedConstraintFields(t *testing.T) {
	trainer := newDelayedTrainer(t, 7)
	train(t, trainer, 1001, 0, 2)
	state, err := NewState(trainer.Snapshot(), 1001, 2)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "plain.json")
	if err := Save(context.Background(), path, state); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"masks", "ranges", "edge_signs", "min_log_magnitude"} {
		if strings.Contains(string(data), field) {
			t.Fatalf("an unused %s field was written: %s", field, data)
		}
	}
	loaded, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Training.Options.Masks != nil || loaded.Training.Options.Ranges != nil ||
		loaded.Training.Config.EdgeSigns != nil || loaded.Training.Config.MinLogMagnitude != 0 {
		t.Fatalf("an old-shaped checkpoint did not load as unconstrained: %+v", loaded.Training.Options)
	}
}

// TestEpisodeCheckpointCarriesMasksRangesAndEdgeSigns is the round trip of the
// stage-one fields through the whole envelope, including the required-field
// walk that runs before the strict decoder.
func TestEpisodeCheckpointCarriesMasksRangesAndEdgeSigns(t *testing.T) {
	trainer := newDelayedTrainer(t, 7)
	snapshot := trainer.Snapshot()
	snapshot.Options.Masks = &learning.UpdateMasks{
		Edges: make([]bool, len(snapshot.Parameters.Core.Weights)),
		Nodes: make([]bool, len(snapshot.Parameters.Core.Bias)),
	}
	snapshot.Options.Masks.Edges[0] = true
	snapshot.Options.Masks.Nodes[0] = true
	snapshot.Options.Ranges = &learning.ParameterRanges{WeightMagnitudeMax: 2, BiasAbsMax: 1, LogTauMin: -3, LogTauMax: 3}
	snapshot.Config.EdgeSigns = make([]int8, len(snapshot.Parameters.Core.Weights))
	snapshot.Config.MinLogMagnitude = -12
	state, err := NewState(snapshot, 1001, 0)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "constrained.json")
	if err := Save(context.Background(), path, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Training.Options.Masks, snapshot.Options.Masks) {
		t.Fatalf("masks did not round trip: %+v", loaded.Training.Options.Masks)
	}
	if !reflect.DeepEqual(loaded.Training.Options.Ranges, snapshot.Options.Ranges) {
		t.Fatalf("ranges did not round trip: %+v", loaded.Training.Options.Ranges)
	}
	if !reflect.DeepEqual(loaded.Training.Config.EdgeSigns, snapshot.Config.EdgeSigns) ||
		loaded.Training.Config.MinLogMagnitude != -12 {
		t.Fatalf("edge signs did not round trip: %+v", loaded.Training.Config.EdgeSigns)
	}
}

// TestEpisodeCheckpointRejectsAMaskThatDoesNotMatchTheTopology proves the
// length check runs on load, not only in memory.
func TestEpisodeCheckpointRejectsAMaskThatDoesNotMatchTheTopology(t *testing.T) {
	raw, payload := episodePayload(t)
	options := nested(payload, "training", "options")
	options["masks"] = map[string]any{"edges": []any{true}}
	if _, err := loadMutated(t, raw, payload); err == nil {
		t.Fatal("accepted a mask shorter than the topology")
	}
}

// TestEpisodeCheckpointRejectsNullInsideTheNewOptionalStructs keeps the
// required-field rule of this package applied to the new fields.
func TestEpisodeCheckpointRejectsNullInsideTheNewOptionalStructs(t *testing.T) {
	for name, mutate := range map[string]func(payload map[string]any){
		"null mask element": func(p map[string]any) {
			nested(p, "training", "options")["masks"] = map[string]any{"edges": []any{nil, nil}}
		},
		"null range bound": func(p map[string]any) {
			nested(p, "training", "options")["ranges"] = map[string]any{"bias_abs_max": nil}
		},
		"null edge sign": func(p map[string]any) {
			nested(p, "training", "config")["edge_signs"] = []any{nil, nil}
		},
		"null min log magnitude": func(p map[string]any) {
			nested(p, "training", "config")["min_log_magnitude"] = nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			raw, payload := episodePayload(t)
			mutate(payload)
			if _, err := loadMutated(t, raw, payload); err == nil {
				t.Fatal("accepted a null inside an optional constraint field")
			}
		})
	}
	// A JSON null for the whole optional pointer stays legal: it is the
	// declared "not configured" value, exactly like the LIF core.
	raw, payload := episodePayload(t)
	nested(payload, "training", "options")["masks"] = nil
	nested(payload, "training", "options")["ranges"] = nil
	nested(payload, "training", "config")["edge_signs"] = nil
	if _, err := loadMutated(t, raw, payload); err != nil {
		t.Fatalf("null optional constraint fields rejected: %v", err)
	}
}

// TestEpisodeCheckpointStillLoadsAHandWrittenPreTicketPayload uses a literal
// document rather than a freshly written one, so a future field cannot make
// this test pass by accident.
func TestEpisodeCheckpointStillLoadsAHandWrittenPreTicketPayload(t *testing.T) {
	trainer := newDelayedTrainer(t, 7)
	state, err := NewState(trainer.Snapshot(), 1001, 0)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]any
	if err := json.Unmarshal(payload, &generic); err != nil {
		t.Fatal(err)
	}
	// Strip every stage-one field a pre-ticket writer could not have produced.
	delete(nested(generic, "training", "options"), "masks")
	delete(nested(generic, "training", "options"), "ranges")
	delete(nested(generic, "training", "config"), "edge_signs")
	delete(nested(generic, "training", "config"), "min_log_magnitude")
	stripped, err := json.Marshal(generic)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "pre-ticket.json")
	writeRaw(t, path, envelopeJSON(SchemaVersion, stripped, checksumHex(stripped)))
	loaded, err := Load(context.Background(), path)
	if err != nil {
		t.Fatalf("a pre-ticket payload no longer loads: %v", err)
	}
	if loaded.Training.Options.Masks != nil || loaded.Training.Options.Ranges != nil || loaded.Training.Config.EdgeSigns != nil {
		t.Fatal("a pre-ticket payload loaded with constraints")
	}
}

// TestEpisodeCheckpointOmitsUnusedStageTwoFields keeps every checkpoint written
// before the second stage of ticket 17 loadable: the loss scale, the
// accumulation window, the schedule and the accumulator are optional and are
// not written when unused.
func TestEpisodeCheckpointOmitsUnusedStageTwoFields(t *testing.T) {
	trainer := newDelayedTrainer(t, 7)
	train(t, trainer, 1001, 0, 2)
	state, err := NewState(trainer.Snapshot(), 1001, 2)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "stage-two-plain.json")
	if err := Save(context.Background(), path, state); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"loss_scale", "accumulate_steps", "schedule", "accumulator"} {
		if strings.Contains(string(data), field) {
			t.Fatalf("an unused %s field was written: %s", field, data)
		}
	}
	loaded, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	o := loaded.Training.Options
	if o.LossScale != 0 || o.AccumulateSteps != 0 || o.Schedule != nil || loaded.Training.Accumulator != nil {
		t.Fatalf("an old-shaped checkpoint did not load unscaled and unaccumulated: %+v", o)
	}
}

// accumulatingState is a valid checkpoint stopped one gradient into a
// three-gradient accumulation window, with a schedule and a loss scale.
func accumulatingState(t *testing.T) State {
	t.Helper()
	trainer := newDelayedTrainer(t, 7)
	train(t, trainer, 1001, 0, 2)
	snapshot := trainer.Snapshot()
	snapshot.Options.LossScale = 1024
	snapshot.Options.AccumulateSteps = 3
	snapshot.Options.Schedule = &learning.Schedule{
		Kind: learning.ScheduleCosine, WarmupUpdates: 2, DecayUpdates: 8, FinalFactor: .25,
	}
	sum := make([]float64, len(snapshot.Optimizer.First))
	for i := range sum {
		sum[i] = float64(i+1) * .5
	}
	snapshot.Accumulator = &learning.GradientAccumulator{Sum: sum, Count: 1}
	state, err := NewState(snapshot, 1001, snapshot.Updates)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(state.Training.Accumulator, snapshot.Accumulator) {
		t.Fatalf("NewState did not keep the open accumulation window: %+v", state.Training.Accumulator)
	}
	return state
}

// TestEpisodeCheckpointCarriesTheStageTwoFields round trips the loss scale, the
// accumulation window, the schedule and a partly filled accumulator through the
// whole envelope, including the required-field walk.
func TestEpisodeCheckpointCarriesTheStageTwoFields(t *testing.T) {
	state := accumulatingState(t)
	path := filepath.Join(t.TempDir(), "accumulating.json")
	if err := Save(context.Background(), path, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Training.Options.Schedule, state.Training.Options.Schedule) {
		t.Fatalf("the schedule did not round trip: %+v", loaded.Training.Options.Schedule)
	}
	if loaded.Training.Options.LossScale != 1024 || loaded.Training.Options.AccumulateSteps != 3 {
		t.Fatalf("the loss scale or the window did not round trip: %+v", loaded.Training.Options)
	}
	if loaded.Training.Accumulator == nil || loaded.Training.Accumulator.Count != 1 ||
		!reflect.DeepEqual(loaded.Training.Accumulator, state.Training.Accumulator) {
		t.Fatalf("the accumulator did not round trip: %+v", loaded.Training.Accumulator)
	}
}

// TestEpisodeCheckpointRejectsAnAccumulatorThatDoesNotFitTheModel proves the
// accumulator rules run on load, not only in memory: the sum has to cover the
// parameter vector and the count has to be inside the declared window.
func TestEpisodeCheckpointRejectsAnAccumulatorThatDoesNotFitTheModel(t *testing.T) {
	for name, mutate := range map[string]func(payload map[string]any){
		"short sum": func(p map[string]any) {
			nested(p, "training")["accumulator"] = map[string]any{"sum": []any{1.0, 2.0}, "count": 1}
		},
		"count at the window": func(p map[string]any) {
			nested(p, "training", "accumulator")["count"] = 3
		},
		"count above the window": func(p map[string]any) {
			nested(p, "training", "accumulator")["count"] = 9
		},
		"negative count": func(p map[string]any) {
			nested(p, "training", "accumulator")["count"] = -1
		},
		"no window declared": func(p map[string]any) {
			delete(nested(p, "training", "options"), "accumulate_steps")
		},
		"missing sum": func(p map[string]any) {
			delete(nested(p, "training", "accumulator"), "sum")
		},
		"missing count": func(p map[string]any) {
			delete(nested(p, "training", "accumulator"), "count")
		},
		"null sum element": func(p map[string]any) {
			sum := nested(p, "training", "accumulator")["sum"].([]any)
			sum[0] = nil
			nested(p, "training", "accumulator")["sum"] = sum
		},
		"null count": func(p map[string]any) {
			nested(p, "training", "accumulator")["count"] = nil
		},
		"unknown schedule kind": func(p map[string]any) {
			nested(p, "training", "options", "schedule")["kind"] = "linear"
		},
		"null schedule kind": func(p map[string]any) {
			nested(p, "training", "options", "schedule")["kind"] = nil
		},
		"negative loss scale": func(p map[string]any) {
			nested(p, "training", "options")["loss_scale"] = -1
		},
	} {
		t.Run(name, func(t *testing.T) {
			raw, payload := accumulatingPayload(t)
			mutate(payload)
			if _, err := loadMutated(t, raw, payload); err == nil {
				t.Fatalf("accepted %s", name)
			}
		})
	}
	// The whole optional accumulator as a JSON null stays legal, exactly like
	// the other optional pointers.
	raw, payload := accumulatingPayload(t)
	nested(payload, "training")["accumulator"] = nil
	if _, err := loadMutated(t, raw, payload); err != nil {
		t.Fatalf("a null accumulator was rejected: %v", err)
	}
}

// accumulatingPayload saves the accumulating checkpoint and returns its payload
// as generic JSON so a test can remove or corrupt individual fields.
func accumulatingPayload(t *testing.T) (envelope, map[string]any) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "accumulating.json")
	if err := Save(context.Background(), path, accumulatingState(t)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw envelope
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	return raw, payload
}

// replayBuffer is the declaration the checkpoint tests below persist: a small
// capacity with every policy named, so the saved document has to carry them.
func replayBuffer() replay.Buffer {
	return replay.Buffer{
		Capacity: 4, Sampling: replay.SamplingTaskBalanced, Eviction: replay.EvictionReservoir,
		Seed: 7, Privacy: replay.PrivacyPolicy{StoreRaw: true, Retention: replay.RetentionKeepUntilEvicted},
	}
}

func replaySnapshot(t *testing.T) replay.Snapshot {
	t.Helper()
	store, err := replay.New(replayBuffer())
	if err != nil {
		t.Fatal(err)
	}
	for step := uint64(1); step <= 6; step++ {
		item := replay.Experience{
			TaskID: "delayed", Step: step,
			Input:  [][]float64{{float64(step)}, {0}},
			Target: [][]float64{{1}},
			Weight: 1, Split: replay.SplitTrain,
		}
		if err := store.Add(item); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := store.Sample(2); err != nil {
		t.Fatal(err)
	}
	return store.Snapshot()
}

// TestEpisodeCheckpointOmitsAnAbsentReplayPart keeps every checkpoint written
// before the replay store existed loadable, and makes "no replay" a readable
// answer rather than an empty store.
func TestEpisodeCheckpointOmitsAnAbsentReplayPart(t *testing.T) {
	trainer := newDelayedTrainer(t, 7)
	train(t, trainer, 1001, 0, 2)
	state, err := NewState(trainer.Snapshot(), 1001, 2)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "no-replay.json")
	if err := Save(context.Background(), path, state); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "replay") {
		t.Fatalf("a run without a replay store wrote a replay part: %s", data)
	}
	loaded, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Replay != nil {
		t.Fatalf("an old-shaped checkpoint loaded with a replay store: %+v", loaded.Replay)
	}
}

// TestEpisodeCheckpointCarriesTheReplayPart is the round trip of the declared
// buffer and its reached state, including the draw count a resumed store needs
// to continue the same sample sequence.
func TestEpisodeCheckpointCarriesTheReplayPart(t *testing.T) {
	trainer := newDelayedTrainer(t, 7)
	train(t, trainer, 1001, 0, 2)
	state, err := NewState(trainer.Snapshot(), 1001, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := replaySnapshot(t)
	state.Replay = &want
	path := filepath.Join(t.TempDir(), "replay.json")
	if err := Save(context.Background(), path, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Replay == nil {
		t.Fatal("the replay part did not survive the round trip")
	}
	if !reflect.DeepEqual(*loaded.Replay, want) {
		t.Fatalf("replay part changed:\n got %+v\nwant %+v", *loaded.Replay, want)
	}
	// The loaded part is a usable store, and continuing it draws what the
	// original store would have drawn next.
	resumed, err := replay.RestoreSnapshot(*loaded.Replay)
	if err != nil {
		t.Fatal(err)
	}
	original, err := replay.RestoreSnapshot(want)
	if err != nil {
		t.Fatal(err)
	}
	got, gotReport, err := resumed.Sample(2)
	if err != nil {
		t.Fatal(err)
	}
	expected, expectedReport, err := original.Sample(2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, expected) || gotReport.Draws != expectedReport.Draws {
		t.Fatalf("a resumed store sampled %+v, the original %+v", got, expected)
	}
}

// TestEpisodeCheckpointRejectsMalformedReplayParts keeps a hand-edited replay
// block from decoding as a valid store. Every case replaces the whole "replay"
// value, so the rejected shapes are written out in full.
func TestEpisodeCheckpointRejectsMalformedReplayParts(t *testing.T) {
	trainer := newDelayedTrainer(t, 7)
	train(t, trainer, 1001, 0, 2)
	state, err := NewState(trainer.Snapshot(), 1001, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := replaySnapshot(t)
	state.Replay = &want
	path := filepath.Join(t.TempDir(), "replay.json")
	if err := Save(context.Background(), path, state); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw envelope
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	full := payload["replay"]
	var part map[string]json.RawMessage
	if err := json.Unmarshal(full, &part); err != nil {
		t.Fatal(err)
	}
	for name, damaged := range map[string]string{
		"missing buffer":    `{"state":` + string(part["state"]) + `}`,
		"missing state":     `{"buffer":` + string(part["buffer"]) + `}`,
		"empty object":      `{}`,
		"unknown sampling":  strings.Replace(string(full), `"task_balanced"`, `"priority"`, 1),
		"unknown eviction":  strings.Replace(string(full), `"reservoir"`, `"lru"`, 1),
		"unknown retention": strings.Replace(string(full), `"keep_until_evicted"`, `"forever"`, 1),
		"unknown schema":    strings.Replace(string(full), `"coimnet-replay/v1"`, `"coimnet-replay/v0"`, 1),
		"test split inside": strings.Replace(string(full), `"split":"train"`, `"split":"test"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			object := map[string]json.RawMessage{}
			for key, value := range payload {
				object[key] = value
			}
			object["replay"] = json.RawMessage(damaged)
			changed, err := json.Marshal(object)
			if err != nil {
				t.Fatal(err)
			}
			bad := filepath.Join(t.TempDir(), "bad-replay.json")
			writeRaw(t, bad, envelopeJSON(SchemaVersion, changed, checksumHex(changed)))
			if _, err := Load(context.Background(), bad); err == nil {
				t.Fatal("accepted a malformed replay part")
			}
		})
	}
}
