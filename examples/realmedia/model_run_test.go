package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

const (
	realMediaChildEnv        = "COIMNET_REALMEDIA_MODEL_CHILD"
	realMediaChildBundleEnv  = "COIMNET_REALMEDIA_MODEL_BUNDLE"
	realMediaChildResultEnv  = "COIMNET_REALMEDIA_MODEL_RESULT"
	realMediaChildPreviewEnv = "COIMNET_REALMEDIA_MODEL_PREVIEW"
)

type realMediaPredictionWire struct {
	SourceID  string      `json:"source_id"`
	SegmentID string      `json:"segment_id"`
	Split     string      `json:"split"`
	Rows      [][]float64 `json:"rows"`
}

type realMediaFitWire struct {
	Config       learning.Config           `json:"config"`
	Before       learning.TrainingSnapshot `json:"before"`
	Snapshot     learning.TrainingSnapshot `json:"snapshot"`
	BeforeScores map[string]score          `json:"before_scores"`
	AfterScores  map[string]score          `json:"after_scores"`
	Predictions  []realMediaPredictionWire `json:"predictions"`
}

type realMediaChildResult struct {
	VerifiedManifestSHA string           `json:"verified_manifest_sha"`
	Fit                 realMediaFitWire `json:"fit"`
}

func TestRealMediaModelChildLoad(t *testing.T) {
	if os.Getenv(realMediaChildEnv) != "1" {
		t.Skip("helper test only; parent test enables the real child process")
	}
	bundle := os.Getenv(realMediaChildBundleEnv)
	resultPath := os.Getenv(realMediaChildResultEnv)
	previewPath := os.Getenv(realMediaChildPreviewEnv)
	if bundle == "" || resultPath == "" || previewPath == "" {
		t.Fatal("real child process is missing its bundle, result, or preview path")
	}
	loaded := realMediaManifest(t)
	expected := newMediaModelConfig(loaded.SHA256, loaded.Manifest.Output)
	snapshot, verifiedSHA, err := loadMediaModel(context.Background(), bundle, expected)
	if err != nil {
		t.Fatalf("child loadMediaModel: %v", err)
	}
	fit, err := fitFromSnapshot(context.Background(), realMediaSamples(), snapshot)
	if err != nil {
		t.Fatalf("child fitFromSnapshot: %v", err)
	}
	if fit.Before.Updates != fit.Snapshot.Updates || fit.Snapshot.Updates == 0 {
		t.Fatalf("child restored inference did not preserve trained update count: before=%d snapshot=%d", fit.Before.Updates, fit.Snapshot.Updates)
	}
	if err := os.Setenv("PATH", ""); err != nil {
		t.Fatal(err)
	}
	testSample := realMediaSamples()[1]
	prediction, ok := findPrediction(fit.Predictions, testSample.SourceID, testSample.SegmentID)
	if !ok {
		t.Fatal("child restored inference omitted the held-out test prediction")
	}
	if _, err := writePreview(context.Background(), previewPath, expected.Output, testSample, prediction); err != nil {
		t.Fatalf("child writePreview: %v", err)
	}
	wire := realMediaChildResult{VerifiedManifestSHA: verifiedSHA, Fit: toRealMediaFitWire(fit)}
	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRealMediaModelChildProcessLoadsBundlePredictsScoresAndWritesPreview(t *testing.T) {
	fit := realMediaFit(t)
	config := realMediaConfig(t)
	bundle := filepath.Join(t.TempDir(), "model")
	if err := saveMediaModel(context.Background(), bundle, config, fit.Snapshot); err != nil {
		t.Fatal(err)
	}
	parentLoaded, parentSHA, err := loadMediaModel(context.Background(), bundle, config)
	if err != nil {
		t.Fatal(err)
	}
	parentFit, err := fitFromSnapshot(context.Background(), realMediaSamples(), parentLoaded)
	if err != nil {
		t.Fatal(err)
	}
	parentPreview := filepath.Join(t.TempDir(), "parent-preview")
	childPreview := filepath.Join(t.TempDir(), "child-preview")
	if err := os.MkdirAll(parentPreview, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(childPreview, 0o700); err != nil {
		t.Fatal(err)
	}
	// Native PNG/WAV/timeline output is deterministic and does not require the
	// optional ffmpeg packaging step. The real runWithModel ffmpeg path remains
	// a Root-owned check against the licensed source clips.
	t.Setenv("PATH", "")
	testSample := realMediaSamples()[1]
	parentPrediction, ok := findPrediction(parentFit.Predictions, testSample.SourceID, testSample.SegmentID)
	if !ok {
		t.Fatal("parent restored inference omitted the held-out test prediction")
	}
	if _, err := writePreview(context.Background(), parentPreview, config.Output, testSample, parentPrediction); err != nil {
		t.Fatalf("parent writePreview: %v", err)
	}
	resultPath := filepath.Join(t.TempDir(), "child-result.json")
	cmd := exec.Command(os.Args[0], "-test.run", "^TestRealMediaModelChildLoad$")
	cmd.Env = append(os.Environ(),
		realMediaChildEnv+"=1",
		realMediaChildBundleEnv+"="+bundle,
		realMediaChildResultEnv+"="+resultPath,
		realMediaChildPreviewEnv+"="+childPreview,
		"PATH=",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("real child process failed: %v\n%s", err, output)
	}
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var child realMediaChildResult
	if err := json.Unmarshal(data, &child); err != nil {
		t.Fatalf("decode child result: %v", err)
	}
	if child.VerifiedManifestSHA != parentSHA || child.VerifiedManifestSHA == config.ManifestSHA256 {
		t.Fatalf("child verified SHA = %q, parent = %q, source = %q", child.VerifiedManifestSHA, parentSHA, config.ManifestSHA256)
	}
	wantFit := toRealMediaFitWire(parentFit)
	if !reflect.DeepEqual(child.Fit, wantFit) {
		t.Fatalf("child restored fit/prediction/score differs from parent\nchild=%s\nwant=%s", mustJSON(t, child.Fit), mustJSON(t, wantFit))
	}
	assertRealMediaPreviewEqual(t, parentPreview, childPreview, len(testSample.Frames))
	if _, err := os.Stat(filepath.Join(bundle, "manifest.json")); err != nil {
		t.Fatalf("child inference removed or changed the persisted bundle: %v", err)
	}
}

func TestRealMediaModelRunFlagsAreExclusiveAndDoNotPublishPartialOutput(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "run-output")
	err := runWithModel(context.Background(), filepath.Join(t.TempDir(), "raw-data"), outDir, "model-input", "model-output")
	if err == nil {
		t.Fatal("runWithModel accepted simultaneous model input and output flags")
	}
	if strings.Contains(err.Error(), "skeleton is unimplemented") {
		t.Fatalf("runWithModel returned the placeholder error: %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "exclusive") {
		t.Fatalf("runWithModel conflict error = %v, want an explicit mutually-exclusive flag error", err)
	}
	if _, statErr := os.Stat(outDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("conflicting model flags published partial output: %v", statErr)
	}
}

func TestRealMediaModelRunCanceledDoesNotPublishOutput(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "canceled-output")
	err := runWithModel(canceledContext(), "", outDir, "", "")
	if err == nil {
		t.Fatal("runWithModel accepted a canceled context")
	}
	if strings.Contains(err.Error(), "skeleton is unimplemented") {
		t.Fatalf("runWithModel returned the placeholder error: %v", err)
	}
	if _, statErr := os.Stat(outDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("canceled run published partial output: %v", statErr)
	}
}

func toRealMediaFitWire(fit fitResult) realMediaFitWire {
	predictions := make([]realMediaPredictionWire, len(fit.Predictions))
	for i, prediction := range fit.Predictions {
		predictions[i] = realMediaPredictionWire{
			SourceID: prediction.SourceID, SegmentID: prediction.SegmentID, Split: prediction.Split,
			Rows: prediction.Rows,
		}
	}
	return realMediaFitWire{
		Config: fit.Config, Before: fit.Before, Snapshot: fit.Snapshot,
		BeforeScores: fit.BeforeScores, AfterScores: fit.AfterScores, Predictions: predictions,
	}
}

func findPrediction(predictions []sequencePrediction, sourceID, segmentID string) (sequencePrediction, bool) {
	for _, prediction := range predictions {
		if prediction.SourceID == sourceID && prediction.SegmentID == segmentID {
			return prediction, true
		}
	}
	return sequencePrediction{}, false
}

func assertRealMediaPreviewEqual(t *testing.T, parent, child string, frames int) {
	t.Helper()
	paths := make([]string, 0, frames+2)
	for i := 0; i < frames; i++ {
		paths = append(paths, filepath.Join("frames", fmt.Sprintf("frame-%02d.png", i)))
	}
	paths = append(paths, "predicted.wav", "timeline.json")
	for _, relative := range paths {
		left, err := os.ReadFile(filepath.Join(parent, relative))
		if err != nil {
			t.Fatalf("read parent preview %s: %v", relative, err)
		}
		right, err := os.ReadFile(filepath.Join(child, relative))
		if err != nil {
			t.Fatalf("read child preview %s: %v", relative, err)
		}
		if !bytes.Equal(left, right) {
			t.Fatalf("child preview %s differs from uninterrupted preview", relative)
		}
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		return "<marshal error>"
	}
	return string(data)
}
