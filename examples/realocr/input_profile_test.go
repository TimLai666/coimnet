package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/TimLai666/coimnet/tasks/ocr"
	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

func TestRealOCRInputProfileInputsPreserveLegacyAndDeclareNewShapes(t *testing.T) {
	dataset := realOCRInputProfileDataset(t)
	image := dataset.Train[0].Image
	for _, test := range []struct {
		profile string
		rows    int
		width   int
	}{
		{profile: "", rows: image.Width, width: image.Height},
		{profile: "columns", rows: image.Width, width: image.Height},
		{profile: "aligned-columns", rows: image.Width + 1, width: image.Height},
		{profile: "single-character", rows: 2, width: image.Width * image.Height},
	} {
		t.Run(inputProfileName(test.profile), func(t *testing.T) {
			model, err := ocr.NewRecognizer(realOCRInputProfileRecognizerConfig(test.profile))
			if err != nil {
				t.Fatalf("NewRecognizer(%q): %v", test.profile, err)
			}
			got, err := model.Inputs(image)
			if err != nil {
				t.Fatalf("Inputs(%q): %v", test.profile, err)
			}
			if len(got) != test.rows {
				t.Fatalf("Inputs(%q) rows = %d, want %d", test.profile, len(got), test.rows)
			}
			for i, row := range got {
				if len(row) != test.width {
					t.Fatalf("Inputs(%q) row %d width = %d, want %d", test.profile, i, len(row), test.width)
				}
			}
			switch test.profile {
			case "", "columns":
				want, err := model.Columns(image)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("Inputs(%q) changed legacy columns", test.profile)
				}
			case "aligned-columns":
				columns, err := model.Columns(image)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got[:len(columns)], columns) {
					t.Fatal("aligned-columns changed the legacy column prefix")
				}
				if !allZero(got[len(got)-1]) {
					t.Fatal("aligned-columns final propagation row is not zero")
				}
			case "single-character":
				if !reflect.DeepEqual(got[0], image.Pixels) {
					t.Fatal("single-character first input does not copy the complete image")
				}
				if !allZero(got[1]) {
					t.Fatal("single-character second input is not the zero propagation row")
				}
			}
		})
	}
}

func TestRealOCRInputProfileLegacyEmptyPreservesBaselineConfigHash(t *testing.T) {
	legacy := RunConfig{
		Seed: 20260924, TrainCount: 1000, TestCount: 200, Epochs: 1, Hidden: 24, LearningRate: 0.002,
		MaxCompressedBytes: defaultMaxCompressedBytes, MaxUncompressedBytes: defaultMaxUncompressedBytes,
		Selection: "balanced_round_robin_by_label_v1",
	}
	got, err := digestJSON(legacy)
	if err != nil {
		t.Fatal(err)
	}
	const want = "1f4f007101f9aab66292d1ef099a434964112d7936c3ae66704f520ed6098e1a"
	if got != want {
		t.Fatalf("legacy baseline config hash = %q, want preserved %q", got, want)
	}

	dataset := realOCRInputProfileDataset(t)
	config := realOCRInputProfileRunConfig("")
	report, err := Run(context.Background(), dataset, config)
	if err != nil {
		t.Fatalf("Run with empty input profile: %v", err)
	}
	if report.Config.InputProfile != "" {
		t.Fatalf("empty profile was rewritten in the report config: %q", report.Config.InputProfile)
	}
	wantReportHash, err := digestJSON(config)
	if err != nil {
		t.Fatal(err)
	}
	if report.ConfigHash != wantReportHash {
		t.Fatalf("empty profile config hash = %q, want %q", report.ConfigHash, wantReportHash)
	}
}

func TestRealOCRInputProfileColumnsMatchesLegacy(t *testing.T) {
	dataset := realOCRInputProfileDataset(t)
	emptyConfig := realOCRInputProfileRunConfig("")
	emptyReport, err := Run(context.Background(), dataset, emptyConfig)
	if err != nil {
		t.Fatalf("Run with empty input profile: %v", err)
	}
	columnsConfig := emptyConfig
	columnsConfig.InputProfile = "columns"
	columnsReport, err := Run(context.Background(), dataset, columnsConfig)
	if err != nil {
		t.Fatalf("Run with explicit columns profile: %v", err)
	}
	if columnsReport.Config.InputProfile != "columns" {
		t.Fatalf("explicit columns profile was lost in the report config: %q", columnsReport.Config.InputProfile)
	}
	if columnsReport.Before != emptyReport.Before || columnsReport.After != emptyReport.After || columnsReport.Train != emptyReport.Train || columnsReport.EncoderProfile != emptyReport.EncoderProfile {
		t.Fatalf("explicit columns profile changed legacy behavior: empty=%+v/%+v/%+v/%+v columns=%+v/%+v/%+v/%+v", emptyReport.Before, emptyReport.After, emptyReport.Train, emptyReport.EncoderProfile, columnsReport.Before, columnsReport.After, columnsReport.Train, columnsReport.EncoderProfile)
	}
	if columnsReport.ConfigHash == emptyReport.ConfigHash {
		t.Fatalf("explicit columns profile did not bind the config hash: %q", columnsReport.ConfigHash)
	}
}

func TestRealOCRInputProfileAlignedDataScopeDescribesPropagationAndAlignment(t *testing.T) {
	dataset := realOCRInputProfileDataset(t)
	defaultReport, err := Run(context.Background(), dataset, realOCRInputProfileRunConfig(""))
	if err != nil {
		t.Fatalf("Run with empty input profile: %v", err)
	}
	wantDefaultScope := DataScope{
		Data:           "official ROIS-CODH Kuzushiji-MNIST handwritten/cursive character images, one character per 28x28 sample",
		Language:       "Japanese hiragana label subset: 10 official KMNIST classes",
		Resolution:     "28x28 grayscale, IDX unsigned bytes normalized to [0,1]",
		SequenceLength: "one Unicode character per sample; 28 image columns are read as the CTC time sequence",
		Periphery:      "identity row encoder, recurrent continuous core, per-column linear readout, greedy CTC decoder",
	}
	if defaultReport.DataScope != wantDefaultScope {
		t.Fatalf("empty input profile changed the default data scope: got=%+v want=%+v", defaultReport.DataScope, wantDefaultScope)
	}

	report, err := Run(context.Background(), dataset, realOCRInputProfileRunConfig("aligned-columns"))
	if err != nil {
		t.Fatalf("Run(aligned-columns): %v", err)
	}
	scope := strings.ToLower(strings.Join([]string{report.DataScope.SequenceLength, report.DataScope.Periphery}, " "))
	if !(strings.Contains(scope, "29") || strings.Contains(scope, "twenty-nine")) || !strings.Contains(scope, "neural") || !strings.Contains(scope, "step") {
		t.Fatalf("aligned data scope does not describe 29 neural steps: %q", scope)
	}
	if !strings.Contains(scope, "zero") || !strings.Contains(scope, "propagation") || !strings.Contains(scope, "step") {
		t.Fatalf("aligned data scope does not describe a zero propagation step: %q", scope)
	}
	firstDiscarded := strings.Contains(scope, "first") && (strings.Contains(scope, "discard") || strings.Contains(scope, "skip"))
	if !firstDiscarded && !strings.Contains(scope, "alignment") {
		t.Fatalf("aligned data scope does not describe first-row discard/skip or alignment: %q", scope)
	}
	if !strings.Contains(scope, "28") || !strings.Contains(scope, "ctc") || !strings.Contains(scope, "visible") || !strings.Contains(scope, "frame") {
		t.Fatalf("aligned data scope does not preserve 28 visible CTC frames: %q", scope)
	}
}

func TestRealOCRInputProfileAlignedAndSingleCharacterTrainInferSaveLoad(t *testing.T) {
	dataset := realOCRInputProfileDataset(t)
	ctx := context.Background()
	hashes := make(map[string]string)
	for _, profile := range []string{"aligned-columns", "single-character"} {
		t.Run(profile, func(t *testing.T) {
			config := realOCRInputProfileRunConfig(profile)
			modelPath := filepath.Join(realOCRInputProfileTempDir(t), profile+"-model")
			saved, err := RunWithModel(ctx, dataset, config, "", modelPath)
			if err != nil {
				t.Fatalf("RunWithModel(%q) save: %v", profile, err)
			}
			if !saved.ModelPersisted || saved.Model == nil {
				t.Fatalf("%q did not report a persisted model: %+v", profile, saved)
			}
			if saved.Config.InputProfile != profile {
				t.Fatalf("saved report input profile = %q, want %q", saved.Config.InputProfile, profile)
			}
			if saved.Train.Updates == 0 {
				t.Fatalf("%q performed no training updates", profile)
			}
			wantHash, err := digestJSON(config)
			if err != nil {
				t.Fatal(err)
			}
			if saved.ConfigHash != wantHash {
				t.Fatalf("%q config hash = %q, want profile-bound %q", profile, saved.ConfigHash, wantHash)
			}
			if saved.Model.SHA256 == "" {
				t.Fatalf("%q saved model has no manifest hash", profile)
			}
			hashes[profile] = saved.ConfigHash

			loaded, err := ocr.LoadRecognizer(ctx, modelPath)
			if err != nil {
				t.Fatalf("LoadRecognizer(%q): %v", profile, err)
			}
			if got := loaded.Config().InputProfile; got != profile {
				t.Fatalf("loaded task config input profile = %q, want %q", got, profile)
			}
			if _, err := loaded.Infer(ctx, dataset.Test[0].Image); err != nil {
				t.Fatalf("loaded %q model inference: %v", profile, err)
			}
			if profile == "single-character" {
				if got := len(loaded.Snapshot().Parameters.Encoder); got != 784*28 {
					t.Fatalf("single-character encoder parameters = %d, want %d", got, 784*28)
				}
				assertSingleCharacterDataScope(t, saved)
			}

			manifestBefore, err := os.ReadFile(filepath.Join(modelPath, "manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			restored, err := RunWithModel(ctx, dataset, config, modelPath, "")
			if err != nil {
				t.Fatalf("RunWithModel(%q) restore: %v", profile, err)
			}
			if restored.Train.Updates != 0 || restored.Train.Epochs != 0 {
				t.Fatalf("%q restore performed updates: epochs=%d updates=%d", profile, restored.Train.Epochs, restored.Train.Updates)
			}
			if restored.After != saved.After || restored.ConfigHash != saved.ConfigHash {
				t.Fatalf("%q restore changed evaluation/hash: saved=%+v restored=%+v", profile, saved.After, restored.After)
			}
			manifestAfter, err := os.ReadFile(filepath.Join(modelPath, "manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(manifestBefore, manifestAfter) {
				t.Fatalf("%q restore changed the saved bundle manifest", profile)
			}

			mismatch := config
			if profile == "aligned-columns" {
				mismatch.InputProfile = "single-character"
			} else {
				mismatch.InputProfile = "aligned-columns"
			}
			if _, err := RunWithModel(ctx, dataset, mismatch, modelPath, ""); err == nil {
				t.Fatalf("%q saved model accepted mismatched profile %q", profile, mismatch.InputProfile)
			}
			manifestAfterMismatch, err := os.ReadFile(filepath.Join(modelPath, "manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(manifestBefore, manifestAfterMismatch) {
				t.Fatalf("%q profile mismatch changed the original bundle", profile)
			}
		})
	}
	if len(hashes) == 2 && hashes["aligned-columns"] == hashes["single-character"] {
		t.Fatalf("different input profiles share config hash %q", hashes["aligned-columns"])
	}
}

func TestRealOCRInputProfileSingleCharacterDataScopeDescribesWholeImagePath(t *testing.T) {
	dataset := realOCRInputProfileDataset(t)
	config := realOCRInputProfileRunConfig("single-character")
	report, err := Run(context.Background(), dataset, config)
	if err != nil {
		t.Fatalf("Run(single-character): %v", err)
	}
	assertSingleCharacterDataScope(t, report)
	if report.EncoderProfile.EncoderParameters != 784*28 {
		t.Fatalf("encoder profile parameters = %d, want %d", report.EncoderProfile.EncoderParameters, 784*28)
	}
}

func TestRealOCRInputProfileTestOnlyChangesDoNotChangeTrainedModel(t *testing.T) {
	dataset := realOCRInputProfileDataset(t)
	if reflect.DeepEqual(dataset.Train, dataset.Test) || reflect.DeepEqual(dataset.TrainIndices, dataset.TestIndices) {
		t.Fatal("fixture train/test slices are not distinct")
	}
	changed := cloneRealOCRInputProfileDataset(dataset)
	changed.Test[0].Text = dataset.Metadata.Mapping[1].Char
	changed.Test[0].Image.Pixels[0] = 1 - changed.Test[0].Image.Pixels[0]
	if reflect.DeepEqual(dataset.Test, changed.Test) {
		t.Fatal("test fixture mutation did not change the test slice")
	}
	if !reflect.DeepEqual(dataset.Train, changed.Train) {
		t.Fatal("test-only mutation changed the training slice")
	}

	config := realOCRInputProfileRunConfig("single-character")
	ctx := context.Background()
	basePath := filepath.Join(realOCRInputProfileTempDir(t), "base-model")
	changedPath := filepath.Join(realOCRInputProfileTempDir(t), "changed-test-model")
	base, err := RunWithModel(ctx, dataset, config, "", basePath)
	if err != nil {
		t.Fatalf("base RunWithModel: %v", err)
	}
	mutated, err := RunWithModel(ctx, changed, config, "", changedPath)
	if err != nil {
		t.Fatalf("test-mutated RunWithModel: %v", err)
	}
	if base.Model == nil || mutated.Model == nil || base.Model.SHA256 != mutated.Model.SHA256 {
		t.Fatalf("test-only data changed trained model hash: base=%+v mutated=%+v", base.Model, mutated.Model)
	}
	baseModel, err := ocr.LoadRecognizer(ctx, basePath)
	if err != nil {
		t.Fatal(err)
	}
	mutatedModel, err := ocr.LoadRecognizer(ctx, changedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(baseModel.Snapshot(), mutatedModel.Snapshot()) {
		t.Fatal("test-only label/pixel changes changed the trained model snapshot")
	}
	if base.ConfigHash != mutated.ConfigHash || base.Train != mutated.Train {
		t.Fatalf("test-only data changed config/train provenance: base=%+v mutated=%+v", base.Train, mutated.Train)
	}
}

func TestRealOCRInputProfilePositiveCharacterWidthRejectsWrongImageShape(t *testing.T) {
	config := realOCRInputProfileRecognizerConfig("single-character")
	config.CharacterWidth = 27
	model, err := ocr.NewRecognizer(config)
	if err != nil {
		t.Fatalf("NewRecognizer accepted positive CharacterWidth=27: %v", err)
	}

	correctWidthImage := glyphs.Image{Width: 27, Height: 28, Pixels: make([]float64, 27*28)}
	if inputs, err := model.Inputs(correctWidthImage); err != nil {
		t.Fatalf("Inputs rejected image matching CharacterWidth=27: %v", err)
	} else if len(inputs) == 0 {
		t.Fatal("Inputs returned no steps for image matching CharacterWidth=27")
	}

	wrongWidthImage := glyphs.Image{Width: 28, Height: 28, Pixels: make([]float64, 28*28)}
	beforeInputs := model.Snapshot()
	if _, err := model.Inputs(wrongWidthImage); err == nil {
		t.Fatal("Inputs accepted an image wider than CharacterWidth=27")
	}
	if !reflect.DeepEqual(beforeInputs, model.Snapshot()) {
		t.Fatal("invalid Inputs shape changed the model snapshot")
	}

	text := string(realOCRInputProfileDataset(t).Metadata.Mapping[0].Char)
	beforeTrain := model.Snapshot()
	if _, err := model.TrainLine(context.Background(), wrongWidthImage, text); err == nil {
		t.Fatal("TrainLine accepted an image wider than CharacterWidth=27")
	}
	if !reflect.DeepEqual(beforeTrain, model.Snapshot()) {
		t.Fatal("invalid TrainLine shape changed the model snapshot")
	}
}

func TestRealOCRInputProfileRejectsUnknownAndInvalidShapeBeforeUpdates(t *testing.T) {
	for _, test := range []struct {
		name   string
		config ocr.RecognizerConfig
	}{
		{name: "unknown", config: realOCRInputProfileRecognizerConfig("unknown")},
		{name: "empty width is irrelevant", config: func() ocr.RecognizerConfig {
			c := realOCRInputProfileRecognizerConfig("")
			c.CharacterWidth = 28
			return c
		}()},
		{name: "aligned width is irrelevant", config: func() ocr.RecognizerConfig {
			c := realOCRInputProfileRecognizerConfig("aligned-columns")
			c.CharacterWidth = 28
			return c
		}()},
		{name: "single width zero", config: func() ocr.RecognizerConfig {
			c := realOCRInputProfileRecognizerConfig("single-character")
			c.CharacterWidth = 0
			return c
		}()},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ocr.NewRecognizer(test.config); err == nil {
				t.Fatalf("NewRecognizer accepted invalid input profile config %+v", test.config)
			}
		})
	}

	model, err := ocr.NewRecognizer(realOCRInputProfileRecognizerConfig("single-character"))
	if err != nil {
		t.Fatalf("valid single-character recognizer: %v", err)
	}
	before := model.Snapshot()
	badImage := glyphs.Image{Width: 27, Height: 28, Pixels: make([]float64, 27*28)}
	if _, err := model.TrainLine(context.Background(), badImage, string(realOCRInputProfileDataset(t).Metadata.Mapping[0].Char)); err == nil {
		t.Fatal("single-character TrainLine accepted an image with the wrong width")
	}
	if !reflect.DeepEqual(before, model.Snapshot()) {
		t.Fatal("invalid single-character shape changed the model before rejection")
	}

	dataset := realOCRInputProfileDataset(t)
	dataset.Train[0].Image = badImage
	output := filepath.Join(realOCRInputProfileTempDir(t), "rejected-model")
	config := realOCRInputProfileRunConfig("single-character")
	if _, err := RunWithModel(context.Background(), dataset, config, "", output); err == nil {
		t.Fatal("RunWithModel accepted an invalid single-character train shape")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("invalid shape published a model bundle: %v", err)
	}
}

func TestRealOCRInputProfileCLIHelpIsEnglishAndListsProfiles(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", ".", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run --help: %v\n%s", err, output)
	}
	help := string(output)
	if !utf8.ValidString(help) {
		t.Fatal("CLI help is not valid UTF-8")
	}
	for _, want := range []string{"input-profile", "columns", "aligned-columns", "single-character"} {
		if !strings.Contains(help, want) {
			t.Fatalf("CLI help omitted %q:\n%s", want, help)
		}
	}
	for _, ru := range help {
		if ru > 127 {
			t.Fatalf("CLI help contains non-English character %q", ru)
		}
	}
}

func realOCRInputProfileRecognizerConfig(profile string) ocr.RecognizerConfig {
	return ocr.RecognizerConfig{
		Height: 28, Hidden: 4, Alphabet: realOCRInputProfileAlphabet(), LearningRate: 0.01, Seed: 77,
		InputProfile: profile, CharacterWidth: func() int {
			if profile == "single-character" {
				return 28
			}
			return 0
		}(),
	}
}

func realOCRInputProfileRunConfig(profile string) RunConfig {
	return RunConfig{
		Seed: 77, TrainCount: 10, TestCount: 10, Epochs: 1, Hidden: 4, LearningRate: 0.01,
		MaxCompressedBytes: defaultMaxCompressedBytes, MaxUncompressedBytes: defaultMaxUncompressedBytes,
		Selection: "balanced_round_robin_by_label_v1", InputProfile: profile,
	}
}

func realOCRInputProfileDataset(t *testing.T) Dataset {
	t.Helper()
	meta := validMetadata()
	if len(meta.Mapping) != 10 {
		t.Fatalf("fixture mapping has %d classes, want 10", len(meta.Mapping))
	}
	train := make([]glyphs.Sample, 10)
	test := make([]glyphs.Sample, 10)
	for class := 0; class < 10; class++ {
		train[class] = glyphs.Sample{Text: meta.Mapping[class].Char, Image: realOCRInputProfileImage(class, 0)}
		test[class] = glyphs.Sample{Text: meta.Mapping[class].Char, Image: realOCRInputProfileImage(class, 1)}
	}
	return Dataset{
		Metadata: meta, MetadataSHA256: "input-profile-metadata", ClassMapSHA256: "input-profile-classmap",
		TrainTotal: 10, TestTotal: 10, Train: train, Test: test,
		TrainIndices: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, TestIndices: []int{10, 11, 12, 13, 14, 15, 16, 17, 18, 19},
	}
}

func cloneRealOCRInputProfileDataset(in Dataset) Dataset {
	out := in
	out.Train = cloneRealOCRInputProfileSamples(in.Train)
	out.Test = cloneRealOCRInputProfileSamples(in.Test)
	out.TrainIndices = append([]int(nil), in.TrainIndices...)
	out.TestIndices = append([]int(nil), in.TestIndices...)
	return out
}

func cloneRealOCRInputProfileSamples(in []glyphs.Sample) []glyphs.Sample {
	out := make([]glyphs.Sample, len(in))
	for i, sample := range in {
		out[i] = sample
		out[i].Image.Pixels = append([]float64(nil), sample.Image.Pixels...)
	}
	return out
}

func realOCRInputProfileImage(class, split int) glyphs.Image {
	image := glyphs.Image{Width: 28, Height: 28, Pixels: make([]float64, 28*28)}
	for i := range image.Pixels {
		image.Pixels[i] = float64((i+class*7+split*3)%19) / 19
	}
	return image
}

func realOCRInputProfileAlphabet() []rune {
	meta := validMetadata()
	alphabet := make([]rune, len(meta.Mapping))
	for i, mapping := range meta.Mapping {
		alphabet[i] = []rune(mapping.Char)[0]
	}
	return alphabet
}

func assertSingleCharacterDataScope(t *testing.T, report Report) {
	t.Helper()
	scope := strings.ToLower(strings.Join([]string{
		report.DataScope.Data, report.DataScope.Language, report.DataScope.Resolution,
		report.DataScope.SequenceLength, report.DataScope.Periphery,
	}, " "))
	for _, requirement := range []struct {
		name string
		ok   bool
	}{
		{name: "whole image input", ok: (strings.Contains(scope, "whole") || strings.Contains(scope, "complete")) && strings.Contains(scope, "28x28") && strings.Contains(scope, "image") && strings.Contains(scope, "input")},
		{name: "zero propagation step", ok: strings.Contains(scope, "zero") && strings.Contains(scope, "propagation") && strings.Contains(scope, "step")},
		{name: "two neural steps", ok: strings.Contains(scope, "two") && strings.Contains(scope, "neural") && strings.Contains(scope, "step")},
		{name: "final row classification", ok: strings.Contains(scope, "final") && strings.Contains(scope, "row") && strings.Contains(scope, "class")},
	} {
		if !requirement.ok {
			t.Errorf("single-character data_scope lacks %s: %q", requirement.name, scope)
		}
	}
	for _, forbidden := range []string{"identity", "per-column", "ctc"} {
		if strings.Contains(scope, forbidden) {
			t.Errorf("single-character data_scope still contains legacy term %q: %q", forbidden, scope)
		}
	}
}

func allZero(row []float64) bool {
	for _, value := range row {
		if value != 0 {
			return false
		}
	}
	return true
}

func inputProfileName(profile string) string {
	if profile == "" {
		return "empty"
	}
	return profile
}

func realOCRInputProfileTempDir(t *testing.T) string {
	t.Helper()
	base, err := filepath.Abs(filepath.Join("..", "..", "runs", ".tmp", "real-task-quality-20261012", "input-profile-tests"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(base, "case-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
