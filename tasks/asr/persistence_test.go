package asr_test

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
	"github.com/TimLai666/coimnet/tasks/asr"
	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

const (
	asrPersistenceHelperEnv = "COIMNET_ASR_PERSISTENCE_HELPER"
	asrPersistenceText      = "ab"
)

func asrPersistenceSynthConfig() asr.SynthConfig {
	return asr.SynthConfig{
		SampleRate: 16000,
		DurationMS: 40,
		GapMS:      20,
		Noise:      0,
		Alphabet:   []rune{'a', 'b'},
	}
}

func asrPersistenceConfig() asr.RecognizerConfig {
	return asr.RecognizerConfig{
		SampleRate: 16000,
		FrontEnd: audio.FrontEndConfig{
			Window: 256,
			Hop:    128,
			Mels:   12,
			FMin:   0,
			FMax:   8000,
			Floor:  1e-10,
		},
		Hidden:       6,
		Alphabet:     []rune{'a', 'b'},
		LearningRate: 0.02,
		Seed:         19,
	}
}

func asrPersistenceSignal(t *testing.T) []float64 {
	t.Helper()
	signal, err := asr.Synthesize(asrPersistenceSynthConfig(), asrPersistenceText, 31)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	return signal
}

func newASRPersistenceRecognizer(t *testing.T) *asr.Recognizer {
	t.Helper()
	recognizer, err := asr.NewRecognizer(asrPersistenceConfig())
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}
	return recognizer
}

func TestRecognizerConfigReturnsOwnedCopy(t *testing.T) {
	m := newASRPersistenceRecognizer(t)
	config := m.Config()
	if !reflect.DeepEqual(config, asrPersistenceConfig()) {
		t.Fatalf("configuration differs: %+v", config)
	}
	config.Alphabet[0] = 'z'
	if !reflect.DeepEqual(m.Config(), asrPersistenceConfig()) {
		t.Fatal("returned configuration aliases model")
	}
	var empty *asr.Recognizer
	if !reflect.DeepEqual(empty.Config(), asr.RecognizerConfig{}) {
		t.Fatal("nil receiver configuration is not zero")
	}
}

func TestRecognizerPersistenceOwnsConstructorAlphabet(t *testing.T) {
	config := asrPersistenceConfig()
	model, err := asr.NewRecognizer(config)
	if err != nil {
		t.Fatal(err)
	}
	config.Alphabet[0] = 'z'
	path := filepath.Join(t.TempDir(), "owned-model")
	if err := model.Save(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	loaded, err := asr.LoadRecognizer(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Config(), asrPersistenceConfig()) {
		t.Fatal("checkpoint aliases constructor alphabet")
	}
}

func trainASRPersistenceStep(t *testing.T, recognizer *asr.Recognizer, signal []float64) {
	t.Helper()
	report, err := recognizer.TrainUtterance(context.Background(), signal, asrPersistenceText)
	if err != nil {
		t.Fatalf("TrainUtterance: %v", err)
	}
	if report.Impossible {
		t.Fatalf("TrainUtterance reported an impossible fixture: %+v", report)
	}
}

func asrBundleFiles(t *testing.T, dir string) map[string][]byte {
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

func assertASRBundleFilesEqual(t *testing.T, want, got map[string][]byte) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bundle files changed:\n got=%v\nwant=%v", got, want)
	}
}

func mutateASRDocument(t *testing.T, dir string, updateManifest bool, mutate func(map[string]json.RawMessage)) {
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
	mutateASRManifest(t, dir, func(manifest *checkpoint.BundleManifest) {
		manifest.Document.Bytes = int64(len(document))
		manifest.Document.SHA256 = digestASR(document)
	})
}

func mutateASRManifest(t *testing.T, dir string, mutate func(*checkpoint.BundleManifest)) {
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

func digestASR(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func deleteASRField(field string) func(map[string]json.RawMessage) {
	return func(document map[string]json.RawMessage) { delete(document, field) }
}

func addASRUnknownField(document map[string]json.RawMessage) {
	document["unexpected"] = json.RawMessage(`true`)
}

func mutateASRConfigLearningRate(t *testing.T, document map[string]json.RawMessage) {
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

func mutateASRNonFiniteParameter(t *testing.T, document map[string]json.RawMessage) {
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
		t.Fatal("saved ASR fixture has no core weights")
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
	signal := asrPersistenceSignal(t)
	full := newASRPersistenceRecognizer(t)
	part := newASRPersistenceRecognizer(t)
	trainASRPersistenceStep(t, full, signal)
	trainASRPersistenceStep(t, part, signal)
	midpoint := part.Snapshot()
	wantInference, err := part.Transcribe(ctx, signal)
	if err != nil {
		t.Fatalf("Transcribe before Save: %v", err)
	}

	path := filepath.Join(t.TempDir(), "asr-model")
	if err := part.Save(ctx, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	childOutput := filepath.Join(t.TempDir(), "child.json")
	command := exec.Command(os.Args[0], "-test.run=^TestASRPersistenceHelperProcess$", "--", path, childOutput)
	command.Env = append(os.Environ(), asrPersistenceHelperEnv+"=1")
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

	loaded, err := asr.LoadRecognizer(ctx, path)
	if err != nil {
		t.Fatalf("LoadRecognizer: %v", err)
	}
	gotInference, err := loaded.Transcribe(ctx, signal)
	if err != nil {
		t.Fatalf("Transcribe after Load: %v", err)
	}
	if gotInference != wantInference {
		t.Fatalf("loaded inference = %q, want %q", gotInference, wantInference)
	}
	if !reflect.DeepEqual(loaded.Snapshot(), midpoint) {
		t.Fatal("loaded snapshot differs from the saved midpoint")
	}
	receipt, err := checkpoint.LoadTaskTrainingBundle(ctx, path, "coimnet-asr-checkpoint/v1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LoadedCheckpointSHA256() == "" || loaded.LoadedCheckpointSHA256() != receipt.ManifestSHA256 {
		t.Fatal("loaded model does not retain its verified manifest receipt")
	}

	trainASRPersistenceStep(t, full, signal)
	trainASRPersistenceStep(t, loaded, signal)
	if !reflect.DeepEqual(loaded.Snapshot(), full.Snapshot()) {
		t.Fatal("loaded recognizer diverged from uninterrupted next update")
	}
	if loaded.LoadedCheckpointSHA256() != receipt.ManifestSHA256 || part.LoadedCheckpointSHA256() != "" {
		t.Fatal("training changed checkpoint origin or a new model claimed a checkpoint")
	}

	firstFiles := asrBundleFiles(t, path)
	trainASRPersistenceStep(t, part, signal)
	fresh, err := asr.LoadRecognizer(ctx, path)
	if err != nil {
		t.Fatalf("LoadRecognizer after caller mutation: %v", err)
	}
	if !reflect.DeepEqual(fresh.Snapshot(), midpoint) {
		t.Fatal("Save retained mutable state from the caller after publication")
	}
	trainASRPersistenceStep(t, loaded, signal)
	fresh, err = asr.LoadRecognizer(ctx, path)
	if err != nil {
		t.Fatalf("LoadRecognizer after loaded mutation: %v", err)
	}
	if !reflect.DeepEqual(fresh.Snapshot(), midpoint) {
		t.Fatal("Load returned state aliased with the checkpoint payload")
	}
	assertASRBundleFilesEqual(t, firstFiles, asrBundleFiles(t, path))
}

func TestRecognizerPersistenceRejectsInvalidInputsAndDoesNotPublish(t *testing.T) {
	ctx := context.Background()
	recognizer := newASRPersistenceRecognizer(t)
	dir := t.TempDir()

	var nilRecognizer *asr.Recognizer
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
	if _, err := asr.LoadRecognizer(nil, filepath.Join(dir, "missing-model")); err == nil {
		t.Fatal("LoadRecognizer accepted a nil context")
	}
	if _, err := asr.LoadRecognizer(ctx, ""); err == nil {
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
	recognizer := newASRPersistenceRecognizer(t)
	path := filepath.Join(t.TempDir(), "asr-model")
	if err := recognizer.Save(context.Background(), path); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	original := asrBundleFiles(t, path)
	if err := recognizer.Save(context.Background(), path); err == nil {
		t.Fatal("second Save overwrote an existing bundle")
	}
	assertASRBundleFilesEqual(t, original, asrBundleFiles(t, path))
}

func TestRecognizerPersistenceRejectsMalformedAndSemanticVariants(t *testing.T) {
	recognizer := newASRPersistenceRecognizer(t)
	source := filepath.Join(t.TempDir(), "valid-model")
	if err := recognizer.Save(context.Background(), source); err != nil {
		t.Fatalf("Save: %v", err)
	}
	cases := []struct {
		name           string
		mutate         func(map[string]json.RawMessage)
		updateManifest bool
	}{
		{name: "unknown document field", mutate: addASRUnknownField, updateManifest: true},
		{name: "missing config", mutate: deleteASRField("config"), updateManifest: true},
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
		{name: "missing training", mutate: deleteASRField("training"), updateManifest: true},
		{name: "overflowing hidden count", mutate: func(p map[string]json.RawMessage) {
			var config asr.RecognizerConfig
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
			mutateASRConfigLearningRate(t, p)
		}, updateManifest: true},
		{name: "non-finite parameter", mutate: func(p map[string]json.RawMessage) {
			mutateASRNonFiniteParameter(t, p)
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
			mutateASRDocument(t, variant, tc.updateManifest, tc.mutate)
			if _, err := asr.LoadRecognizer(context.Background(), variant); err == nil {
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
	if _, err := asr.LoadRecognizer(context.Background(), checksumVariant); err == nil {
		t.Fatal("LoadRecognizer accepted a corrupt document checksum")
	}

	schemaVariant := filepath.Join(t.TempDir(), "schema-model")
	if err := os.CopyFS(schemaVariant, os.DirFS(source)); err != nil {
		t.Fatalf("copy source bundle: %v", err)
	}
	mutateASRManifest(t, schemaVariant, func(manifest *checkpoint.BundleManifest) {
		manifest.SchemaVersion = "coimnet-wrong-bundle/v1"
	})
	if _, err := asr.LoadRecognizer(context.Background(), schemaVariant); err == nil {
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
	mutateASRManifest(t, malformedVariant, func(manifest *checkpoint.BundleManifest) {
		manifest.Document.Bytes = 1
		manifest.Document.SHA256 = digestASR([]byte("{"))
	})
	if _, err := asr.LoadRecognizer(context.Background(), malformedVariant); err == nil {
		t.Fatal("LoadRecognizer accepted malformed document JSON")
	}
}

func TestASRPersistenceHelperProcess(t *testing.T) {
	if os.Getenv(asrPersistenceHelperEnv) != "1" {
		return
	}
	if len(os.Args) < 5 || os.Args[len(os.Args)-3] != "--" {
		t.Fatalf("helper arguments = %#v", os.Args)
	}
	separator := len(os.Args) - 3
	path, outputPath := os.Args[separator+1], os.Args[separator+2]
	restored, err := asr.LoadRecognizer(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	inference, err := restored.Transcribe(context.Background(), asrPersistenceSignal(t))
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
