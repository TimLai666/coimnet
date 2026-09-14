package checkpoint

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/experiment"
	"github.com/TimLai666/coimnet/learning"
)

func newLIFTrainer(t *testing.T, seed uint64) *learning.Trainer {
	t.Helper()
	trainer, err := experiment.NewDelayedLIFTrainer(seed, .02, learning.Trainable{Theta: true})
	if err != nil {
		t.Fatal(err)
	}
	return trainer
}

// lifPayload saves a valid LIF episode checkpoint and returns its payload as
// nested generic JSON so tests can remove or null individual fields.
func lifPayload(t *testing.T) (envelope, map[string]any, string) {
	t.Helper()
	trainer := newLIFTrainer(t, 7)
	train(t, trainer, 1001, 0, 2)
	state, err := NewState(trainer.Snapshot(), 1001, 2)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "lif.json")
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
	return raw, payload, path
}

func TestLIFEpisodeCheckpointRoundTrip(t *testing.T) {
	trainer := newLIFTrainer(t, 7)
	train(t, trainer, 1001, 0, 3)
	snapshot := trainer.Snapshot()
	state, err := NewState(snapshot, 1001, 3)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "lif-round-trip.json")
	if err := Save(context.Background(), path, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(context.Background(), path)
	if err != nil {
		t.Fatalf("load LIF checkpoint: %v", err)
	}
	if !reflect.DeepEqual(loaded, state) {
		t.Fatalf("round trip changed state:\n%+v\n%+v", state, loaded)
	}
	if loaded.Training.Config.LIF == nil {
		t.Fatal("loaded checkpoint lost the LIF configuration")
	}
	if !reflect.DeepEqual(loaded.Training.Config.LIF, snapshot.Config.LIF) {
		t.Fatalf("LIF configuration changed: %+v", loaded.Training.Config.LIF)
	}
	if !reflect.DeepEqual(loaded.Training.Parameters.ThetaRaw, snapshot.Parameters.ThetaRaw) || len(loaded.Training.Parameters.ThetaRaw) == 0 {
		t.Fatalf("theta_raw changed: %v", loaded.Training.Parameters.ThetaRaw)
	}
	if !loaded.Training.Options.Trainable.Theta {
		t.Fatal("trainable theta flag was lost")
	}
	restored, err := learning.RestoreTrainer(loaded.Training)
	if err != nil {
		t.Fatalf("restore LIF trainer: %v", err)
	}
	if !reflect.DeepEqual(restored.Snapshot(), snapshot) {
		t.Fatal("restored trainer differs from the saved snapshot")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, key := range []string{`"lif"`, `"theta_raw"`, `"theta":true`, `"theta_min"`, `"surrogate"`} {
		if !strings.Contains(text, key) {
			t.Fatalf("checkpoint does not contain %s: %s", key, text)
		}
	}
	// The continuous core is absent, so its optional arrays stay omitted and
	// the required-field check still accepts the document.
	if strings.Contains(text, `"input_nodes"`) {
		t.Fatalf("optional input_nodes was written: %s", text)
	}
}

func TestLIFCheckpointRejectsNullThetaRaw(t *testing.T) {
	raw, payload, _ := lifPayload(t)
	parameters := nested(payload, "training", "parameters")
	if _, ok := parameters["theta_raw"].([]any); !ok {
		t.Fatalf("fixture has no theta_raw array: %v", parameters["theta_raw"])
	}
	parameters["theta_raw"] = nil
	if loaded, err := loadMutated(t, raw, payload); err == nil {
		t.Fatalf("accepted a null theta_raw: %+v", loaded.Training.Parameters)
	}
	raw, payload, _ = lifPayload(t)
	values := nested(payload, "training", "parameters")["theta_raw"].([]any)
	values[0] = nil
	if _, err := loadMutated(t, raw, payload); err == nil {
		t.Fatal("accepted a null theta_raw element")
	}
	raw, payload, _ = lifPayload(t)
	delete(nested(payload, "training", "parameters"), "theta_raw")
	if _, err := loadMutated(t, raw, payload); err == nil {
		t.Fatal("accepted a LIF checkpoint without theta_raw")
	}
	raw, payload, _ = lifPayload(t)
	nested(payload, "training", "config")["lif"] = nil
	if _, err := loadMutated(t, raw, payload); err == nil {
		t.Fatal("accepted a LIF checkpoint whose core is null")
	}
	raw, payload, _ = lifPayload(t)
	nested(payload, "training", "config", "lif")["theta_min"] = nil
	if _, err := loadMutated(t, raw, payload); err == nil {
		t.Fatal("accepted a null theta_min")
	}
}
