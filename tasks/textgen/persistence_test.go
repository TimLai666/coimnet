package textgen_test

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
	"github.com/TimLai666/coimnet/tasks/textgen"
	"github.com/TimLai666/coimnet/tasks/textgen/tokenizer"
)

const textPersistenceHelperEnv = "COIMNET_TEXTGEN_PERSISTENCE_HELPER"

func textPersistenceVocab(t *testing.T) *tokenizer.ByteVocab {
	t.Helper()
	vocab, err := tokenizer.New(tokenizer.Config{
		Format: tokenizer.FormatByte,
		Specials: []tokenizer.SpecialToken{
			{Name: "bos", Role: "bos"},
			{Name: "eos", Role: "eos"},
			{Name: "pad", Role: "pad"},
		},
	})
	if err != nil {
		t.Fatalf("tokenizer.New: %v", err)
	}
	return vocab
}

func textPersistenceConfig() textgen.ModelConfig {
	return textgen.ModelConfig{Embed: 4, Hidden: 4, Settle: 1, LearningRate: 0.02, Seed: 29}
}

func textPersistenceExample(t *testing.T, vocab tokenizer.Vocabulary) textgen.Example {
	t.Helper()
	ids, err := vocab.Encode("abc")
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	bos, ok := vocab.Special("bos")
	if !ok {
		t.Fatal("fixture vocabulary has no bos token")
	}
	ids = append([]int{bos}, ids...)
	scored := make([]bool, len(ids))
	for i := 1; i < len(scored); i++ {
		scored[i] = true
	}
	return textgen.Example{IDs: ids, Scored: scored}
}

func newTextPersistenceModel(t *testing.T) (*textgen.Model, *tokenizer.ByteVocab) {
	t.Helper()
	vocab := textPersistenceVocab(t)
	model, err := textgen.NewModel(vocab, textPersistenceConfig())
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}
	return model, vocab
}

func trainTextPersistenceStep(t *testing.T, model *textgen.Model, example textgen.Example) {
	t.Helper()
	result, _, err := model.Step(context.Background(), example)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if !result.Applied {
		t.Fatalf("Step did not apply an update: %+v", result)
	}
}

func textBundleFiles(t *testing.T, dir string) map[string][]byte {
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

func assertTextBundleFilesEqual(t *testing.T, want, got map[string][]byte) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bundle files changed:\n got=%v\nwant=%v", got, want)
	}
}

func mutateTextDocument(t *testing.T, dir string, updateManifest bool, mutate func(map[string]json.RawMessage)) {
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
	mutateTextManifest(t, dir, func(manifest *checkpoint.BundleManifest) {
		manifest.Document.Bytes = int64(len(document))
		manifest.Document.SHA256 = digestText(document)
	})
}

func mutateTextManifest(t *testing.T, dir string, mutate func(*checkpoint.BundleManifest)) {
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

func digestText(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func deleteTextField(field string) func(map[string]json.RawMessage) {
	return func(document map[string]json.RawMessage) { delete(document, field) }
}

func addTextUnknownField(document map[string]json.RawMessage) {
	document["unexpected"] = json.RawMessage(`true`)
}

func mutateTextConfigLearningRate(t *testing.T, document map[string]json.RawMessage) {
	t.Helper()
	var config struct {
		Model      textgen.ModelConfig `json:"model"`
		Vocabulary tokenizer.Config    `json:"vocabulary"`
		VocabHash  string              `json:"vocab_hash"`
	}
	if err := json.Unmarshal(document["config"], &config); err != nil {
		t.Fatal(err)
	}
	config.Model.LearningRate = 0.123456789
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	document["config"] = encoded
}

func mutateTextVocabHash(document map[string]json.RawMessage) {
	var walk func(map[string]json.RawMessage) bool
	walk = func(object map[string]json.RawMessage) bool {
		if _, ok := object["vocab_hash"]; ok {
			object["vocab_hash"] = json.RawMessage(`"0000000000000000000000000000000000000000000000000000000000000000"`)
			return true
		}
		for key, raw := range object {
			var nested map[string]json.RawMessage
			if err := json.Unmarshal(raw, &nested); err != nil {
				continue
			}
			if walk(nested) {
				encoded, err := json.Marshal(nested)
				if err == nil {
					object[key] = encoded
				}
				return true
			}
		}
		return false
	}
	if !walk(document) {
		document["vocab_hash"] = json.RawMessage(`"0000000000000000000000000000000000000000000000000000000000000000"`)
	}
}

func mutateTextNonFiniteParameter(t *testing.T, document map[string]json.RawMessage) {
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
		t.Fatal("saved text fixture has no core weights")
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

type textCustomVocabulary struct{ tokenizer.Vocabulary }

func TestModelPersistenceRoundTripAndSubprocessResume(t *testing.T) {
	ctx := context.Background()
	model, vocab := newTextPersistenceModel(t)
	full, _ := newTextPersistenceModel(t)
	part, _ := newTextPersistenceModel(t)
	example := textPersistenceExample(t, vocab)
	fullExample := textPersistenceExample(t, full.Vocabulary())
	partExample := textPersistenceExample(t, part.Vocabulary())
	trainTextPersistenceStep(t, full, fullExample)
	trainTextPersistenceStep(t, part, partExample)
	midpoint := part.Snapshot()
	wantLogits, err := part.Logits(ctx, example.IDs)
	if err != nil {
		t.Fatalf("Logits before Save: %v", err)
	}

	path := filepath.Join(t.TempDir(), "text-model")
	if err := part.Save(ctx, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	childOutput := filepath.Join(t.TempDir(), "child.json")
	command := exec.Command(os.Args[0], "-test.run=^TestTextModelPersistenceHelperProcess$", "--", path, childOutput)
	command.Env = append(os.Environ(), textPersistenceHelperEnv+"=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("child load failed: %v\n%s", err, output)
	}
	childData, err := os.ReadFile(childOutput)
	if err != nil {
		t.Fatalf("ReadFile(child output): %v", err)
	}
	var childLogits [][]float64
	if err := json.Unmarshal(childData, &childLogits); err != nil {
		t.Fatalf("decode child logits: %v", err)
	}
	if !reflect.DeepEqual(childLogits, wantLogits) {
		t.Fatal("child logits differ from the pre-save logits")
	}

	loaded, err := textgen.LoadModel(ctx, path)
	if err != nil {
		t.Fatalf("LoadModel: %v", err)
	}
	gotLogits, err := loaded.Logits(ctx, example.IDs)
	if err != nil {
		t.Fatalf("Logits after Load: %v", err)
	}
	if !reflect.DeepEqual(gotLogits, wantLogits) {
		t.Fatal("loaded logits differ from the pre-save logits")
	}
	if !reflect.DeepEqual(loaded.Snapshot(), midpoint) {
		t.Fatal("loaded snapshot differs from the saved midpoint")
	}
	receipt, err := checkpoint.LoadTaskTrainingBundle(ctx, path, "coimnet-textgen-checkpoint/v1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LoadedCheckpointSHA256() == "" || loaded.LoadedCheckpointSHA256() != receipt.ManifestSHA256 {
		t.Fatal("loaded model does not retain its verified manifest receipt")
	}
	loadedVocab, ok := loaded.Vocabulary().(*tokenizer.ByteVocab)
	if !ok {
		t.Fatalf("loaded vocabulary type = %T, want *tokenizer.ByteVocab", loaded.Vocabulary())
	}
	if loadedVocab.Hash() != vocab.Hash() || !reflect.DeepEqual(loadedVocab.Config(), vocab.Config()) {
		t.Fatal("loaded vocabulary config/hash differs from the saved ByteVocab")
	}
	if model.Config() != textPersistenceConfig() {
		t.Fatalf("model config changed before persistence: %+v", model.Config())
	}

	trainTextPersistenceStep(t, full, fullExample)
	trainTextPersistenceStep(t, loaded, textPersistenceExample(t, loaded.Vocabulary()))
	if !reflect.DeepEqual(loaded.Snapshot(), full.Snapshot()) {
		t.Fatal("loaded model diverged from uninterrupted next update")
	}
	if loaded.LoadedCheckpointSHA256() != receipt.ManifestSHA256 || part.LoadedCheckpointSHA256() != "" {
		t.Fatal("training changed checkpoint origin or a new model claimed a checkpoint")
	}

	firstFiles := textBundleFiles(t, path)
	trainTextPersistenceStep(t, part, partExample)
	fresh, err := textgen.LoadModel(ctx, path)
	if err != nil {
		t.Fatalf("LoadModel after caller mutation: %v", err)
	}
	if !reflect.DeepEqual(fresh.Snapshot(), midpoint) {
		t.Fatal("Save retained mutable state from the caller after publication")
	}
	trainTextPersistenceStep(t, loaded, textPersistenceExample(t, loaded.Vocabulary()))
	fresh, err = textgen.LoadModel(ctx, path)
	if err != nil {
		t.Fatalf("LoadModel after loaded mutation: %v", err)
	}
	if !reflect.DeepEqual(fresh.Snapshot(), midpoint) {
		t.Fatal("Load returned state aliased with the checkpoint payload")
	}
	assertTextBundleFilesEqual(t, firstFiles, textBundleFiles(t, path))
}

func TestModelPersistenceRejectsInvalidInputsAndCustomVocabulary(t *testing.T) {
	ctx := context.Background()
	model, vocab := newTextPersistenceModel(t)
	dir := t.TempDir()

	var nilModel *textgen.Model
	nilPath := filepath.Join(dir, "nil-model")
	if err := nilModel.Save(ctx, nilPath); err == nil {
		t.Fatal("nil receiver Save succeeded")
	}
	if _, err := os.Stat(nilPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("nil receiver Save left a path: %v", err)
	}
	if err := model.Save(nil, filepath.Join(dir, "nil-context")); err == nil {
		t.Fatal("Save accepted a nil context")
	}
	if err := model.Save(ctx, ""); err == nil {
		t.Fatal("Save accepted an empty path")
	}
	if _, err := textgen.LoadModel(nil, filepath.Join(dir, "missing-model")); err == nil {
		t.Fatal("LoadModel accepted a nil context")
	}
	if _, err := textgen.LoadModel(ctx, ""); err == nil {
		t.Fatal("LoadModel accepted an empty path")
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	canceledPath := filepath.Join(dir, "canceled-model")
	if err := model.Save(canceled, canceledPath); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Save = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(canceledPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled Save left a path: %v", err)
	}

	custom, err := textgen.NewModel(&textCustomVocabulary{Vocabulary: vocab}, textPersistenceConfig())
	if err != nil {
		t.Fatalf("NewModel(custom vocabulary): %v", err)
	}
	customPath := filepath.Join(dir, "custom-vocab-model")
	if err := custom.Save(ctx, customPath); err == nil {
		t.Fatal("Save accepted a vocabulary without a reproducible ByteVocab config")
	}
	if _, err := os.Stat(customPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("custom-vocabulary Save left a path: %v", err)
	}
}

func TestModelPersistenceDoesNotOverwriteExistingDestination(t *testing.T) {
	model, _ := newTextPersistenceModel(t)
	path := filepath.Join(t.TempDir(), "text-model")
	if err := model.Save(context.Background(), path); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	original := textBundleFiles(t, path)
	if err := model.Save(context.Background(), path); err == nil {
		t.Fatal("second Save overwrote an existing bundle")
	}
	assertTextBundleFilesEqual(t, original, textBundleFiles(t, path))
}

func TestModelPersistenceRejectsMalformedAndSemanticVariants(t *testing.T) {
	model, _ := newTextPersistenceModel(t)
	source := filepath.Join(t.TempDir(), "valid-model")
	if err := model.Save(context.Background(), source); err != nil {
		t.Fatalf("Save: %v", err)
	}
	cases := []struct {
		name           string
		mutate         func(map[string]json.RawMessage)
		updateManifest bool
	}{
		{name: "unknown document field", mutate: addTextUnknownField, updateManifest: true},
		{name: "missing config", mutate: deleteTextField("config"), updateManifest: true},
		{name: "missing training", mutate: deleteTextField("training"), updateManifest: true},
		{name: "config and training mismatch", mutate: func(p map[string]json.RawMessage) { mutateTextConfigLearningRate(t, p) }, updateManifest: true},
		{name: "vocabulary hash mismatch", mutate: mutateTextVocabHash, updateManifest: true},
		{name: "non-finite parameter", mutate: func(p map[string]json.RawMessage) {
			mutateTextNonFiniteParameter(t, p)
		}, updateManifest: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			variant := filepath.Join(t.TempDir(), "variant-model")
			if err := os.CopyFS(variant, os.DirFS(source)); err != nil {
				t.Fatalf("copy source bundle: %v", err)
			}
			mutateTextDocument(t, variant, tc.updateManifest, tc.mutate)
			if _, err := textgen.LoadModel(context.Background(), variant); err == nil {
				t.Fatal("LoadModel accepted an invalid semantic payload")
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
	if _, err := textgen.LoadModel(context.Background(), checksumVariant); err == nil {
		t.Fatal("LoadModel accepted a corrupt document checksum")
	}

	schemaVariant := filepath.Join(t.TempDir(), "schema-model")
	if err := os.CopyFS(schemaVariant, os.DirFS(source)); err != nil {
		t.Fatalf("copy source bundle: %v", err)
	}
	mutateTextManifest(t, schemaVariant, func(manifest *checkpoint.BundleManifest) {
		manifest.SchemaVersion = "coimnet-wrong-bundle/v1"
	})
	if _, err := textgen.LoadModel(context.Background(), schemaVariant); err == nil {
		t.Fatal("LoadModel accepted an unsupported bundle schema")
	}

	malformedVariant := filepath.Join(t.TempDir(), "malformed-model")
	if err := os.CopyFS(malformedVariant, os.DirFS(source)); err != nil {
		t.Fatalf("copy source bundle: %v", err)
	}
	malformedDocument := filepath.Join(malformedVariant, "document.json")
	if err := os.WriteFile(malformedDocument, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	mutateTextManifest(t, malformedVariant, func(manifest *checkpoint.BundleManifest) {
		manifest.Document.Bytes = 1
		manifest.Document.SHA256 = digestText([]byte("{"))
	})
	if _, err := textgen.LoadModel(context.Background(), malformedVariant); err == nil {
		t.Fatal("LoadModel accepted malformed document JSON")
	}
}

func TestTextModelPersistenceHelperProcess(t *testing.T) {
	if os.Getenv(textPersistenceHelperEnv) != "1" {
		return
	}
	if len(os.Args) < 5 || os.Args[len(os.Args)-3] != "--" {
		t.Fatalf("helper arguments = %#v", os.Args)
	}
	separator := len(os.Args) - 3
	path, outputPath := os.Args[separator+1], os.Args[separator+2]
	restored, err := textgen.LoadModel(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := restored.Vocabulary().Encode("abc")
	if err != nil {
		t.Fatal(err)
	}
	if bos, ok := restored.Vocabulary().Special("bos"); ok {
		ids = append([]int{bos}, ids...)
	}
	logits, err := restored.Logits(context.Background(), ids)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(logits)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outputPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}
