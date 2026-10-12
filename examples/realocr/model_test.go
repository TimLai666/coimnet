package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/tasks/ocr"
)

func TestModelArtifactUsesTheManifestVerifiedAtLoad(t *testing.T) {
	ctx := context.Background()
	model, err := ocr.NewRecognizer(ocr.RecognizerConfig{
		Height: 8, Hidden: 6, Alphabet: []rune{'a', 'b'}, LearningRate: 0.02, Seed: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "model")
	if err := model.Save(ctx, path); err != nil {
		t.Fatal(err)
	}
	loaded, err := ocr.LoadRecognizer(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := checkpoint.ReadBundleManifest(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Document.SHA256 = strings.Repeat("0", 64)
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "manifest.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	artifact, err := realOCRArtifact(ctx, path, "restored", loaded)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.SHA256 != loaded.LoadedCheckpointSHA256() {
		t.Fatal("report identified a replacement manifest rather than the loaded model")
	}
}

func TestModelOutputAndInputPreserveEvaluationWithoutRetraining(t *testing.T) {
	dir := t.TempDir()
	writeMetadata(t, dir)
	writeOfficialFixture(t, dir)
	dataset, err := LoadDataset(dir, DatasetLimits{TrainCount: 1, TestCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	config := RunConfig{Seed: 7, TrainCount: 1, TestCount: 1, Epochs: 1, Hidden: 6, LearningRate: 0.02, Selection: "balanced_round_robin_by_label_v1"}
	path := filepath.Join(t.TempDir(), "trained-model")
	saved, err := RunWithModel(context.Background(), dataset, config, "", path)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(path, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RunWithModel(context.Background(), dataset, config, path, "")
	if err != nil {
		t.Fatal(err)
	}
	if saved.After != restored.After || restored.Train.Updates != 0 || restored.Train.Epochs != 0 {
		t.Fatalf("restored evaluation changed or trained: %+v / %+v", saved, restored)
	}
	if !saved.ModelPersisted || restored.ModelPersisted {
		t.Fatal("incorrect persistence report")
	}
	for _, report := range []Report{saved, restored} {
		encoded, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatal(err)
		}
		if len(fields["model"]) == 0 {
			t.Fatal("missing model provenance")
		}
	}
	after, err := os.ReadFile(filepath.Join(path, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("evaluation changed the saved model")
	}
}

func TestModelFlagsRejectConflictsAndInvalidModels(t *testing.T) {
	dir := t.TempDir()
	writeMetadata(t, dir)
	writeOfficialFixture(t, dir)
	dataset, err := LoadDataset(dir, DatasetLimits{TrainCount: 1, TestCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	config := RunConfig{Seed: 7, TrainCount: 1, TestCount: 1, Epochs: 1, Hidden: 6, LearningRate: 0.02, Selection: "balanced_round_robin_by_label_v1"}
	output := filepath.Join(t.TempDir(), "never-published")
	if _, err := RunWithModel(context.Background(), dataset, config, "missing", output); err == nil {
		t.Fatal("conflicting flags accepted")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("conflict published artifact: %v", err)
	}
	if _, err := RunWithModel(context.Background(), dataset, config, "missing", ""); err == nil {
		t.Fatal("missing model accepted")
	}
}
