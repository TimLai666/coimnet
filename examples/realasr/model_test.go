package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestModelOutputAndInputPreserveEvaluationWithoutRetraining(t *testing.T) {
	manifest, _ := writeExampleDataset(t, t.TempDir())
	model := filepath.Join(t.TempDir(), "trained-model")
	var saved, restored, stderr bytes.Buffer
	if err := run(context.Background(), []string{"--manifest", manifest, "--model-output", model}, &saved, &stderr); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(model, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := run(context.Background(), []string{"--manifest", manifest, "--model-input", model}, &restored, &stderr); err != nil {
		t.Fatal(err)
	}
	var a, b map[string]json.RawMessage
	if err := json.Unmarshal(saved.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(restored.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	var am, bm map[string]any
	if err := json.Unmarshal(a["model"], &am); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b["model"], &bm); err != nil {
		t.Fatal(err)
	}
	if am["mode"] != "saved" || bm["mode"] != "restored" || am["updates"] != bm["updates"] || am["sha256"] != bm["sha256"] {
		t.Fatalf("model provenance changed: %v / %v", am, bm)
	}
	if !bytes.Equal(a["after"], b["after"]) {
		t.Fatal("restored held-out evaluation differs")
	}
	after, err := os.ReadFile(filepath.Join(model, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("evaluation rewrote saved model")
	}
}

func TestModelFlagsRejectConflictAndMissingModelWithoutPartialReport(t *testing.T) {
	manifest, _ := writeExampleDataset(t, t.TempDir())
	for _, args := range [][]string{
		{"--manifest", manifest, "--model-input", "missing-model"},
		{"--manifest", manifest, "--model-input", "missing-model", "--model-output", "new-model"},
	} {
		var stdout, stderr bytes.Buffer
		if err := run(context.Background(), args, &stdout, &stderr); err == nil {
			t.Fatal("invalid model arguments accepted")
		}
		if stdout.Len() != 0 {
			t.Fatal("partial report on failure")
		}
	}
}
