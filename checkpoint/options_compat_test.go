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
