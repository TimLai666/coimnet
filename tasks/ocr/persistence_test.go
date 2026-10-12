package ocr_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/tasks/ocr"
	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

const (
	ocrPersistenceHelperEnv = "COIMNET_OCR_PERSISTENCE_HELPER"
	ocrPersistenceText      = "ab"
)

func ocrPersistenceConfig() ocr.RecognizerConfig {
	return ocr.RecognizerConfig{
		Height:       8,
		Hidden:       6,
		Alphabet:     []rune{'a', 'b'},
		LearningRate: 0.02,
		Seed:         17,
	}
}

func ocrPersistenceImage(t *testing.T) glyphs.Image {
	t.Helper()
	image, _, err := glyphs.Render(ocrPersistenceText, glyphs.Family{Seed: 23}, glyphs.Options{Background: 0.1})
	if err != nil {
		t.Fatalf("glyphs.Render: %v", err)
	}
	return image
}

func newOCRPersistenceRecognizer(t *testing.T) *ocr.Recognizer {
	t.Helper()
	recognizer, err := ocr.NewRecognizer(ocrPersistenceConfig())
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}
	return recognizer
}

func TestRecognizerConfigReturnsOwnedCopy(t *testing.T) {
	m := newOCRPersistenceRecognizer(t)
	config := m.Config()
	if !reflect.DeepEqual(config, ocrPersistenceConfig()) {
		t.Fatalf("configuration differs: %+v", config)
	}
	config.Alphabet[0] = 'z'
	if !reflect.DeepEqual(m.Config(), ocrPersistenceConfig()) {
		t.Fatal("returned configuration aliases model")
	}
	var empty *ocr.Recognizer
	if !reflect.DeepEqual(empty.Config(), ocr.RecognizerConfig{}) {
		t.Fatal("nil receiver configuration is not zero")
	}
}

func TestRecognizerPersistenceOwnsConstructorAlphabet(t *testing.T) {
	config := ocrPersistenceConfig()
	model, err := ocr.NewRecognizer(config)
	if err != nil {
		t.Fatal(err)
	}
	config.Alphabet[0] = 'z'
	path := filepath.Join(t.TempDir(), "owned-model")
	if err := model.Save(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	loaded, err := ocr.LoadRecognizer(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Config(), ocrPersistenceConfig()) {
		t.Fatal("checkpoint aliases constructor alphabet")
	}
}

func trainOCRPersistenceStep(t *testing.T, recognizer *ocr.Recognizer, image glyphs.Image) {
	t.Helper()
	report, err := recognizer.TrainLine(context.Background(), image, ocrPersistenceText)
	if err != nil {
		t.Fatalf("TrainLine: %v", err)
	}
	if report.Impossible {
		t.Fatalf("TrainLine reported an impossible fixture: %+v", report)
	}
}

func ocrBundleFiles(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}
	files := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("bundle contains nested directory %q", entry.Name())
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", entry.Name(), err)
		}
		files[entry.Name()] = data
	}
	return files
}

func assertOCRBundleFilesEqual(t *testing.T, want, got map[string][]byte) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bundle files changed:\n got=%v\nwant=%v", got, want)
	}
}

func mutateOCRDocument(t *testing.T, dir string, updateManifest bool, mutate func(map[string]json.RawMessage)) {
	t.Helper()
	documentPath := filepath.Join(dir, "document.json")
	document, err := os.ReadFile(documentPath)
	if err != nil {
		t.Fatalf("ReadFile(document.json): %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(document, &fields); err != nil {
		t.Fatalf("decode document.json: %v", err)
	}
	mutate(fields)
	document, err = json.Marshal(fields)
	if err != nil {
		t.Fatalf("encode document.json: %v", err)
	}
	if err := os.WriteFile(documentPath, document, 0o600); err != nil {
		t.Fatalf("write document.json: %v", err)
	}
	if !updateManifest {
		return
	}
	mutateOCRManifest(t, dir, func(manifest *checkpoint.BundleManifest) {
		manifest.Document.Bytes = int64(len(document))
		manifest.Document.SHA256 = digestOCR(document)
	})
}

func mutateOCRManifest(t *testing.T, dir string, mutate func(*checkpoint.BundleManifest)) {
	t.Helper()
	path := filepath.Join(dir, "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(manifest.json): %v", err)
	}
	var manifest checkpoint.BundleManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode manifest.json: %v", err)
	}
	mutate(&manifest)
	data, err = json.Marshal(manifest)
	if err != nil {
		t.Fatalf("encode manifest.json: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}
}

func digestOCR(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func deleteOCRField(field string) func(map[string]json.RawMessage) {
	return func(document map[string]json.RawMessage) { delete(document, field) }
}

func addOCRUnknownField(document map[string]json.RawMessage) {
	document["unexpected"] = json.RawMessage(`true`)
}

func mutateOCRConfigLearningRate(t *testing.T, document map[string]json.RawMessage) {
	t.Helper()
	var config map[string]json.RawMessage
	if err := json.Unmarshal(document["config"], &config); err != nil {
		t.Fatalf("decode task config: %v", err)
	}
	config["learning_rate"] = json.RawMessage(`0.123456789`)
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("encode task config: %v", err)
	}
	document["config"] = encoded
}

func mutateOCRNonFiniteParameter(t *testing.T, document map[string]json.RawMessage) {
	t.Helper()
	var training map[string]json.RawMessage
	if err := json.Unmarshal(document["training"], &training); err != nil {
		t.Fatalf("decode training: %v", err)
	}
	var parameters map[string]json.RawMessage
	if err := json.Unmarshal(training["parameters"], &parameters); err != nil {
		t.Fatalf("decode parameters: %v", err)
	}
	var core map[string]json.RawMessage
	if err := json.Unmarshal(parameters["core"], &core); err != nil {
		t.Fatalf("decode core parameters: %v", err)
	}
	var weights []json.RawMessage
	if err := json.Unmarshal(core["weights"], &weights); err != nil {
		t.Fatalf("decode weights: %v", err)
	}
	if len(weights) == 0 {
		t.Fatal("saved OCR fixture has no core weights")
	}
	weights[0] = json.RawMessage(`1e9999`)
	encodedWeights, err := json.Marshal(weights)
	if err != nil {
		t.Fatalf("encode weights: %v", err)
	}
	core["weights"] = encodedWeights
	encodedCore, err := json.Marshal(core)
	if err != nil {
		t.Fatalf("encode core parameters: %v", err)
	}
	parameters["core"] = encodedCore
	encodedParameters, err := json.Marshal(parameters)
	if err != nil {
		t.Fatalf("encode parameters: %v", err)
	}
	training["parameters"] = encodedParameters
	encodedTraining, err := json.Marshal(training)
	if err != nil {
		t.Fatalf("encode training: %v", err)
	}
	document["training"] = encodedTraining
}

func TestRecognizerPersistenceRoundTripAndSubprocessResume(t *testing.T) {
	ctx := context.Background()
	image := ocrPersistenceImage(t)
	full := newOCRPersistenceRecognizer(t)
	part := newOCRPersistenceRecognizer(t)
	trainOCRPersistenceStep(t, full, image)
	trainOCRPersistenceStep(t, part, image)
	midpoint := part.Snapshot()
	wantInference, err := part.Infer(ctx, image)
	if err != nil {
		t.Fatalf("Infer before Save: %v", err)
	}

	path := filepath.Join(t.TempDir(), "ocr-model")
	if err := part.Save(ctx, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	childOutput := filepath.Join(t.TempDir(), "child.json")
	command := exec.Command(os.Args[0], "-test.run=^TestOCRPersistenceHelperProcess$", "--", path, childOutput)
	command.Env = append(os.Environ(), ocrPersistenceHelperEnv+"=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("child load failed: %v\n%s", err, output)
	}
	childData, err := os.ReadFile(childOutput)
	if err != nil {
		t.Fatalf("ReadFile(child output): %v", err)
	}
	var childInference string
	if err := json.Unmarshal(childData, &childInference); err != nil {
		t.Fatalf("decode child inference: %v", err)
	}
	if childInference != wantInference {
		t.Fatalf("child inference = %q, want %q", childInference, wantInference)
	}

	loaded, err := ocr.LoadRecognizer(ctx, path)
	if err != nil {
		t.Fatalf("LoadRecognizer: %v", err)
	}
	gotInference, err := loaded.Infer(ctx, image)
	if err != nil {
		t.Fatalf("Infer after Load: %v", err)
	}
	if gotInference != wantInference {
		t.Fatalf("loaded inference = %q, want %q", gotInference, wantInference)
	}
	if !reflect.DeepEqual(loaded.Snapshot(), midpoint) {
		t.Fatal("loaded snapshot differs from the saved midpoint")
	}
	receipt, err := checkpoint.LoadTaskTrainingBundle(ctx, path, "coimnet-ocr-checkpoint/v1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LoadedCheckpointSHA256() == "" || loaded.LoadedCheckpointSHA256() != receipt.ManifestSHA256 {
		t.Fatal("loaded model does not retain its verified manifest receipt")
	}

	trainOCRPersistenceStep(t, full, image)
	trainOCRPersistenceStep(t, loaded, image)
	if !reflect.DeepEqual(loaded.Snapshot(), full.Snapshot()) {
		t.Fatal("loaded recognizer diverged from uninterrupted next update")
	}
	if loaded.LoadedCheckpointSHA256() != receipt.ManifestSHA256 || part.LoadedCheckpointSHA256() != "" {
		t.Fatal("training changed checkpoint origin or a new model claimed a checkpoint")
	}

	firstFiles := ocrBundleFiles(t, path)
	trainOCRPersistenceStep(t, part, image)
	fresh, err := ocr.LoadRecognizer(ctx, path)
	if err != nil {
		t.Fatalf("LoadRecognizer after caller mutation: %v", err)
	}
	if !reflect.DeepEqual(fresh.Snapshot(), midpoint) {
		t.Fatal("Save retained mutable state from the caller after publication")
	}
	trainOCRPersistenceStep(t, loaded, image)
	fresh, err = ocr.LoadRecognizer(ctx, path)
	if err != nil {
		t.Fatalf("LoadRecognizer after loaded mutation: %v", err)
	}
	if !reflect.DeepEqual(fresh.Snapshot(), midpoint) {
		t.Fatal("Load returned state aliased with the checkpoint payload")
	}
	assertOCRBundleFilesEqual(t, firstFiles, ocrBundleFiles(t, path))
}

func TestRecognizerPersistenceRejectsInvalidInputsAndDoesNotPublish(t *testing.T) {
	ctx := context.Background()
	recognizer := newOCRPersistenceRecognizer(t)
	dir := t.TempDir()

	var nilRecognizer *ocr.Recognizer
	nilPath := filepath.Join(dir, "nil-model")
	if err := nilRecognizer.Save(ctx, nilPath); err == nil {
		t.Fatal("nil receiver Save succeeded")
	}
	if _, err := os.Stat(nilPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("nil receiver Save left a path: %v", err)
	}
	if err := recognizer.Save(nil, filepath.Join(dir, "nil-context")); err == nil {
		t.Fatal("Save accepted a nil context")
	}
	if err := recognizer.Save(ctx, ""); err == nil {
		t.Fatal("Save accepted an empty path")
	}
	if _, err := ocr.LoadRecognizer(nil, filepath.Join(dir, "missing-model")); err == nil {
		t.Fatal("LoadRecognizer accepted a nil context")
	}
	if _, err := ocr.LoadRecognizer(ctx, ""); err == nil {
		t.Fatal("LoadRecognizer accepted an empty path")
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	canceledPath := filepath.Join(dir, "canceled-model")
	if err := recognizer.Save(canceled, canceledPath); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Save = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(canceledPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled Save left a path: %v", err)
	}
}

func TestRecognizerPersistenceDoesNotOverwriteExistingDestination(t *testing.T) {
	recognizer := newOCRPersistenceRecognizer(t)
	path := filepath.Join(t.TempDir(), "ocr-model")
	if err := recognizer.Save(context.Background(), path); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	original := ocrBundleFiles(t, path)
	if err := recognizer.Save(context.Background(), path); err == nil {
		t.Fatal("second Save overwrote an existing bundle")
	}
	assertOCRBundleFilesEqual(t, original, ocrBundleFiles(t, path))
}

func TestRecognizerPersistenceRejectsMalformedAndSemanticVariants(t *testing.T) {
	recognizer := newOCRPersistenceRecognizer(t)
	source := filepath.Join(t.TempDir(), "valid-model")
	if err := recognizer.Save(context.Background(), source); err != nil {
		t.Fatalf("Save: %v", err)
	}
	cases := []struct {
		name           string
		mutate         func(map[string]json.RawMessage)
		updateManifest bool
	}{
		{name: "unknown document field", mutate: addOCRUnknownField, updateManifest: true},
		{name: "missing config", mutate: deleteOCRField("config"), updateManifest: true},
		{name: "missing config seed", mutate: func(p map[string]json.RawMessage) {
			var config map[string]json.RawMessage
			if err := json.Unmarshal(p["config"], &config); err != nil {
				t.Fatal(err)
			}
			delete(config, "seed")
			data, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			p["config"] = data
		}, updateManifest: true},
		{name: "missing training", mutate: deleteOCRField("training"), updateManifest: true},
		{name: "overflowing hidden count", mutate: func(p map[string]json.RawMessage) {
			var config ocr.RecognizerConfig
			if err := json.Unmarshal(p["config"], &config); err != nil {
				t.Fatal(err)
			}
			config.Hidden = int(^uint(0) >> 1)
			data, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			p["config"] = data
		}, updateManifest: true},
		{name: "config and training mismatch", mutate: func(p map[string]json.RawMessage) {
			mutateOCRConfigLearningRate(t, p)
		}, updateManifest: true},
		{name: "non-finite parameter", mutate: func(p map[string]json.RawMessage) {
			mutateOCRNonFiniteParameter(t, p)
		}, updateManifest: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("LoadRecognizer panicked on invalid config: %v", recovered)
				}
			}()
			variant := filepath.Join(t.TempDir(), "variant-model")
			if err := os.CopyFS(variant, os.DirFS(source)); err != nil {
				t.Fatalf("copy source bundle: %v", err)
			}
			mutateOCRDocument(t, variant, tc.updateManifest, tc.mutate)
			if _, err := ocr.LoadRecognizer(context.Background(), variant); err == nil {
				t.Fatal("LoadRecognizer accepted an invalid semantic payload")
			}
		})
	}

	checksumVariant := filepath.Join(t.TempDir(), "checksum-model")
	if err := os.CopyFS(checksumVariant, os.DirFS(source)); err != nil {
		t.Fatalf("copy source bundle: %v", err)
	}
	documentPath := filepath.Join(checksumVariant, "document.json")
	document, err := os.ReadFile(documentPath)
	if err != nil {
		t.Fatal(err)
	}
	document[0] ^= 1
	if err := os.WriteFile(documentPath, document, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ocr.LoadRecognizer(context.Background(), checksumVariant); err == nil {
		t.Fatal("LoadRecognizer accepted a corrupt document checksum")
	}

	schemaVariant := filepath.Join(t.TempDir(), "schema-model")
	if err := os.CopyFS(schemaVariant, os.DirFS(source)); err != nil {
		t.Fatalf("copy source bundle: %v", err)
	}
	mutateOCRManifest(t, schemaVariant, func(manifest *checkpoint.BundleManifest) {
		manifest.SchemaVersion = "coimnet-wrong-bundle/v1"
	})
	if _, err := ocr.LoadRecognizer(context.Background(), schemaVariant); err == nil {
		t.Fatal("LoadRecognizer accepted an unsupported bundle schema")
	}

	malformedVariant := filepath.Join(t.TempDir(), "malformed-model")
	if err := os.CopyFS(malformedVariant, os.DirFS(source)); err != nil {
		t.Fatalf("copy source bundle: %v", err)
	}
	malformedDocument := filepath.Join(malformedVariant, "document.json")
	if err := os.WriteFile(malformedDocument, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	mutateOCRManifest(t, malformedVariant, func(manifest *checkpoint.BundleManifest) {
		manifest.Document.Bytes = 1
		manifest.Document.SHA256 = digestOCR([]byte("{"))
	})
	if _, err := ocr.LoadRecognizer(context.Background(), malformedVariant); err == nil {
		t.Fatal("LoadRecognizer accepted malformed document JSON")
	}
}

func TestOCRPersistenceHelperProcess(t *testing.T) {
	if os.Getenv(ocrPersistenceHelperEnv) != "1" {
		return
	}
	if len(os.Args) < 5 || os.Args[len(os.Args)-3] != "--" {
		t.Fatalf("helper arguments = %#v", os.Args)
	}
	separator := len(os.Args) - 3
	path, outputPath := os.Args[separator+1], os.Args[separator+2]
	restored, err := ocr.LoadRecognizer(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	inference, err := restored.Infer(context.Background(), ocrPersistenceImage(t))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(inference)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outputPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}
