package checkpoint

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

// episodePayload saves a valid episode checkpoint and returns its payload as
// nested generic JSON so tests can remove or null individual fields.
func episodePayload(t *testing.T) (envelope, map[string]any) {
	t.Helper()
	trainer := newDelayedTrainer(t, 7)
	train(t, trainer, 1001, 0, 2)
	state, err := NewState(trainer.Snapshot(), 1001, 2)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "valid.json")
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
	var payload map[string]any
	if err := json.Unmarshal(raw.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	return raw, payload
}

func loadMutated(t *testing.T, raw envelope, payload map[string]any) (State, error) {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "mutated.json")
	writeRaw(t, path, envelopeJSON(raw.SchemaVersion, encoded, checksumHex(encoded)))
	return Load(context.Background(), path)
}

func nested(payload map[string]any, keys ...string) map[string]any {
	current := payload
	for _, key := range keys {
		current = current[key].(map[string]any)
	}
	return current
}

func TestEpisodeLoadRejectsMissingOrNullRequiredScalars(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(payload map[string]any)
		want   string
	}{
		{"missing data_seed", func(p map[string]any) { delete(p, "data_seed") }, "data_seed"},
		{"null data_seed", func(p map[string]any) { p["data_seed"] = nil }, "data_seed"},
		{"missing next_sample", func(p map[string]any) { delete(p, "next_sample") }, "next_sample"},
		{"null next_sample", func(p map[string]any) { p["next_sample"] = nil }, "next_sample"},
		{"null generator", func(p map[string]any) { p["generator"] = nil }, "generator"},
		{"missing training", func(p map[string]any) { delete(p, "training") }, "training"},
		{"null training", func(p map[string]any) { p["training"] = nil }, "training"},
		{"missing training.updates", func(p map[string]any) { delete(nested(p, "training"), "updates") }, "training.updates"},
		{"null training.updates", func(p map[string]any) { nested(p, "training")["updates"] = nil }, "training.updates"},
		{"missing options.weight_decay", func(p map[string]any) { delete(nested(p, "training", "options"), "weight_decay") }, "weight_decay"},
		{"null options.weight_decay", func(p map[string]any) { nested(p, "training", "options")["weight_decay"] = nil }, "weight_decay"},
		{"null options.learning_rate", func(p map[string]any) { nested(p, "training", "options")["learning_rate"] = nil }, "learning_rate"},
		{"missing options.truncation", func(p map[string]any) { delete(nested(p, "training", "options"), "truncation") }, "truncation"},
		{"null trainable.weights", func(p map[string]any) { nested(p, "training", "options", "trainable")["weights"] = nil }, "trainable.weights"},
		{"missing trainable", func(p map[string]any) { delete(nested(p, "training", "options"), "trainable") }, "trainable"},
		{"missing config.input_size", func(p map[string]any) { delete(nested(p, "training", "config"), "input_size") }, "input_size"},
		{"null config.dynamics.dt", func(p map[string]any) { nested(p, "training", "config", "dynamics")["dt"] = nil }, "dynamics.dt"},
		{"missing config.dynamics.nodes", func(p map[string]any) { delete(nested(p, "training", "config", "dynamics"), "nodes") }, "dynamics.nodes"},
		{"null config.dynamics.activation", func(p map[string]any) { nested(p, "training", "config", "dynamics")["activation"] = nil }, "activation"},
		{"missing parameters", func(p map[string]any) { delete(nested(p, "training"), "parameters") }, "parameters"},
		{"null parameters.core", func(p map[string]any) { nested(p, "training", "parameters")["core"] = nil }, "parameters.core"},
		{"missing optimizer", func(p map[string]any) { delete(nested(p, "training"), "optimizer") }, "optimizer"},
		{"null element in core.bias", func(p map[string]any) {
			bias := nested(p, "training", "parameters", "core")["bias"].([]any)
			bias[0] = nil
		}, "core.bias[0]"},
		{"null element in optimizer.steps", func(p map[string]any) {
			steps := nested(p, "training", "optimizer")["steps"].([]any)
			steps[1] = nil
		}, "optimizer.steps[1]"},
		{"null element in readout_nodes", func(p map[string]any) {
			nodes := nested(p, "training", "config")["readout_nodes"].([]any)
			nodes[0] = nil
		}, "readout_nodes[0]"},
		{"missing training.schema_version", func(p map[string]any) { delete(nested(p, "training"), "schema_version") }, "training.schema_version"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, payload := episodePayload(t)
			tc.mutate(payload)
			loaded, err := loadMutated(t, raw, payload)
			if err == nil {
				t.Fatalf("accepted checkpoint; data_seed=%d updates=%d", loaded.DataSeed, loaded.Training.Updates)
			}
			if !strings.Contains(err.Error(), tc.want) || !(strings.Contains(err.Error(), "missing") || strings.Contains(err.Error(), "null")) {
				t.Fatalf("error %q does not name %s as missing or null", err, tc.want)
			}
		})
	}
}

func TestEpisodeLoadKeepsLegalZeroValuesNullableArraysAndOmittedOptionalFields(t *testing.T) {
	// Explicit legal zeros: data seed 0, cursor 0 and update count 0.
	raw, payload := episodePayload(t)
	trainer := newDelayedTrainer(t, 7)
	fresh, err := NewState(trainer.Snapshot(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	freshPath := filepath.Join(t.TempDir(), "fresh.json")
	if err := Save(context.Background(), freshPath, fresh); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(context.Background(), freshPath)
	if err != nil || loaded.DataSeed != 0 || loaded.NextSample != 0 || loaded.Training.Updates != 0 {
		t.Fatalf("legal zero values: %+v err=%v", loaded, err)
	}
	// Explicit zero weight decay and truncation written as 0 stay accepted.
	nested(payload, "training", "options")["weight_decay"] = 0
	nested(payload, "training", "options")["truncation"] = 0
	if _, err := loadMutated(t, raw, payload); err != nil {
		t.Fatalf("explicit zero options rejected: %v", err)
	}

	// Zero-edge graph: Save writes core.weights as JSON null and Load accepts it.
	zero, err := learning.NewTrainer(learning.Config{
		Dynamics:     dynamics.Config{Nodes: 2, DT: .5, Activation: "tanh"},
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: []int{1},
	}, learning.Parameters{
		Core:    dynamics.Parameters{Bias: []float64{.1, -.1}, LogTau: []float64{0, 0}},
		Encoder: []float64{.5, .2},
		Readout: []float64{.8},
	}, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	zeroState, err := NewState(zero.Snapshot(), 1001, 0)
	if err != nil {
		t.Fatal(err)
	}
	zeroPath := filepath.Join(t.TempDir(), "zero-edge.json")
	if err := Save(context.Background(), zeroPath, zeroState); err != nil {
		t.Fatal(err)
	}
	zeroBytes, err := os.ReadFile(zeroPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(zeroBytes), `"weights":null`) {
		t.Fatalf("zero-edge fixture no longer writes a null weights array: %s", zeroBytes)
	}
	zeroLoaded, err := Load(context.Background(), zeroPath)
	if err != nil || zeroLoaded.Training.Parameters.Core.Weights != nil || len(zeroLoaded.Training.Config.Dynamics.Sources) != 0 {
		t.Fatalf("zero-edge checkpoint: %+v err=%v", zeroLoaded.Training.Parameters.Core, err)
	}
	// Optional input_nodes and delays are omitted by Save and may also be
	// present as JSON null.
	if strings.Contains(string(zeroBytes), "input_nodes") || strings.Contains(string(zeroBytes), "delays") {
		t.Fatalf("optional fields were written: %s", zeroBytes)
	}
	nested(payload, "training", "config")["input_nodes"] = nil
	nested(payload, "training", "config", "dynamics")["delays"] = nil
	if _, err := loadMutated(t, raw, payload); err != nil {
		t.Fatalf("null optional arrays rejected: %v", err)
	}
}
