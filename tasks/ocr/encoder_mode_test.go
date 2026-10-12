package ocr_test

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/ocr"
	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

func TestOCREncoderModesShareSeededParametersAndCapacity(t *testing.T) {
	config := ocr.RecognizerConfig{
		Height: 8, Hidden: 6, Alphabet: []rune{'a', 'b'}, LearningRate: 0.02, Seed: 101,
	}
	fixed := newOCRRecognizerWithMode(t, config, false)
	trainable := newOCRRecognizerWithMode(t, config, true)
	fixedSnapshot := fixed.Snapshot()
	trainableSnapshot := trainable.Snapshot()
	if !equalOCRParameterBits(fixedSnapshot.Parameters, trainableSnapshot.Parameters) {
		t.Fatal("fixed and trainable encoders with the same seed did not share initial parameter bits")
	}
	if !trainableSnapshot.Options.Trainable.Encoder {
		t.Fatal("the trainable encoder options did not mark Encoder trainable")
	}
	if fixedSnapshot.Options.Trainable.Encoder {
		t.Fatal("fixed encoder options mark Encoder trainable")
	}
	if len(fixedSnapshot.Parameters.Encoder) != config.Height*config.Height {
		t.Fatalf("encoder capacity = %d, want %d", len(fixedSnapshot.Parameters.Encoder), config.Height*config.Height)
	}
	if len(fixedSnapshot.Parameters.Encoder) != len(trainableSnapshot.Parameters.Encoder) || len(fixedSnapshot.Parameters.Readout) != len(trainableSnapshot.Parameters.Readout) || len(fixedSnapshot.Parameters.Core.Weights) != len(trainableSnapshot.Parameters.Core.Weights) {
		t.Fatalf("fixed/trainable capacity differs: fixed encoder/core/readout=%d/%d/%d trainable=%d/%d/%d", len(fixedSnapshot.Parameters.Encoder), len(fixedSnapshot.Parameters.Core.Weights), len(fixedSnapshot.Parameters.Readout), len(trainableSnapshot.Parameters.Encoder), len(trainableSnapshot.Parameters.Core.Weights), len(trainableSnapshot.Parameters.Readout))
	}
	if !reflect.DeepEqual(fixed.Config(), config) || !reflect.DeepEqual(trainable.Config(), func() ocr.RecognizerConfig {
		c := config
		c.TrainableEncoder = true
		return c
	}()) {
		t.Fatalf("recognizer configurations did not retain encoder mode: fixed=%+v trainable=%+v", fixed.Config(), trainable.Config())
	}
}

func TestOCREncoderTrainingFreezesFixedAndUpdatesTrainable(t *testing.T) {
	ctx := context.Background()
	config := ocr.RecognizerConfig{
		Height: 8, Hidden: 6, Alphabet: []rune{'a', 'b'}, LearningRate: 0.02, Seed: 102,
	}
	fixed := newOCRRecognizerWithMode(t, config, false)
	trainable := newOCRRecognizerWithMode(t, config, true)
	image, _, err := glyphs.Render("ab", glyphs.Family{Seed: 19}, glyphs.Options{Background: 0.1})
	if err != nil {
		t.Fatal(err)
	}
	fixedBefore := fixed.Snapshot()
	trainableBefore := trainable.Snapshot()
	fixedReport, err := fixed.TrainLine(ctx, image, "a")
	if err != nil {
		t.Fatalf("fixed TrainLine: %v", err)
	}
	trainableReport, err := trainable.TrainLine(ctx, image, "a")
	if err != nil {
		t.Fatalf("trainable TrainLine: %v", err)
	}
	if fixedReport.Impossible || trainableReport.Impossible {
		t.Fatalf("one-step fixture was impossible: fixed=%+v trainable=%+v", fixedReport, trainableReport)
	}
	fixedAfter := fixed.Snapshot()
	trainableAfter := trainable.Snapshot()
	if fixedAfter.Updates != fixedBefore.Updates+1 || trainableAfter.Updates != trainableBefore.Updates+1 {
		t.Fatalf("updates fixed=%d/%d trainable=%d/%d, want one applied update", fixedBefore.Updates, fixedAfter.Updates, trainableBefore.Updates, trainableAfter.Updates)
	}
	if !equalOCRFloatBits(fixedBefore.Parameters.Encoder, fixedAfter.Parameters.Encoder) {
		t.Fatal("fixed encoder changed during TrainLine")
	}
	if equalOCRFloatBits(trainableBefore.Parameters.Encoder, trainableAfter.Parameters.Encoder) {
		t.Fatal("trainable encoder did not change during TrainLine")
	}
	if !equalOCRParameterShape(fixedAfter.Parameters, trainableAfter.Parameters) {
		t.Fatal("fixed and trainable parameter shapes diverged after training")
	}
}

func TestOCREncoderSaveLoadPreservesModeAndNextUpdate(t *testing.T) {
	ctx := context.Background()
	config := ocr.RecognizerConfig{
		Height: 8, Hidden: 6, Alphabet: []rune{'a', 'b'}, LearningRate: 0.02, Seed: 103, TrainableEncoder: true,
	}
	image, _, err := glyphs.Render("ab", glyphs.Family{Seed: 20}, glyphs.Options{Background: 0.1})
	if err != nil {
		t.Fatal(err)
	}
	model := newOCRRecognizerWithMode(t, config, true)
	beforeTraining := model.Snapshot()
	if !beforeTraining.Options.Trainable.Encoder {
		t.Fatal("trainable recognizer snapshot did not retain Encoder=true before training")
	}
	if report, err := model.TrainLine(ctx, image, "a"); err != nil || report.Impossible {
		t.Fatalf("initial train = %+v, err=%v", report, err)
	}
	afterTraining := model.Snapshot()
	if !afterTraining.Options.Trainable.Encoder {
		t.Fatal("trainable recognizer snapshot did not retain Encoder=true after training")
	}
	if equalOCRFloatBits(beforeTraining.Parameters.Encoder, afterTraining.Parameters.Encoder) {
		t.Fatal("trainable encoder did not change during the pre-save training update")
	}
	wantInference, err := model.Infer(ctx, image)
	if err != nil {
		t.Fatal(err)
	}
	wantSnapshot := afterTraining
	path := t.TempDir() + "/trainable-model"
	if err := model.Save(ctx, path); err != nil {
		t.Fatalf("Save trainable recognizer: %v", err)
	}
	loaded, err := ocr.LoadRecognizer(ctx, path)
	if err != nil {
		t.Fatalf("Load trainable recognizer: %v", err)
	}
	if !loaded.Config().TrainableEncoder {
		t.Fatal("LoadRecognizer lost TrainableEncoder=true")
	}
	loadedSnapshot := loaded.Snapshot()
	if !loadedSnapshot.Options.Trainable.Encoder {
		t.Fatal("LoadRecognizer snapshot lost Options.Trainable.Encoder=true")
	}
	if !reflect.DeepEqual(loaded.Config(), model.Config()) || !reflect.DeepEqual(loadedSnapshot, wantSnapshot) {
		t.Fatal("loaded trainable recognizer does not match saved configuration/snapshot")
	}
	gotInference, err := loaded.Infer(ctx, image)
	if err != nil {
		t.Fatal(err)
	}
	if gotInference != wantInference {
		t.Fatalf("loaded inference = %q, want %q", gotInference, wantInference)
	}
	if _, err := model.TrainLine(ctx, image, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := loaded.TrainLine(ctx, image, "a"); err != nil {
		t.Fatal(err)
	}
	loadedAfterNext := loaded.Snapshot()
	if !loadedAfterNext.Options.Trainable.Encoder {
		t.Fatal("loaded snapshot lost Options.Trainable.Encoder=true after the next update")
	}
	if !reflect.DeepEqual(loadedAfterNext, model.Snapshot()) {
		t.Fatal("loaded trainable recognizer diverged from uninterrupted next update")
	}
}

func TestOCREmptyOutputsAndRatiosUseExactDecodedStrings(t *testing.T) {
	ctx := context.Background()
	config := ocr.RecognizerConfig{
		Height: 8, Hidden: 4, Alphabet: []rune{'a', ' '}, LearningRate: 0.02, Seed: 104,
	}
	image, _, err := glyphs.Render("a", glyphs.Family{Seed: 21}, glyphs.Options{Background: 0.1})
	if err != nil {
		t.Fatal(err)
	}
	emptyModel := newOCRZeroOutputRecognizer(t, config)
	emptyDecoded, err := emptyModel.Infer(ctx, image)
	if err != nil {
		t.Fatalf("Infer zero-output model: %v", err)
	}
	if emptyDecoded != "" {
		t.Fatalf("controllable zero-output model decoded %q, want exact empty string", emptyDecoded)
	}
	report, err := emptyModel.Evaluate(ctx, []glyphs.Sample{
		{Text: "a", Image: image},
		{Text: "", Image: image},
		{Text: " ", Image: image},
	})
	if err != nil {
		t.Fatalf("Evaluate empty-output model: %v", err)
	}
	if report.Samples != 3 || report.EmptyOutputs != 3 || report.EmptyOutputRatio != 1 {
		t.Fatalf("empty-output report = %+v, want 3 samples/3 empty/ratio 1", report)
	}
	if report.ExactLines != 1.0/3.0 {
		t.Fatalf("empty-output ExactLines = %g, want 1/3", report.ExactLines)
	}
	if report.CER.ReferenceLength != 2 || !report.CER.Defined || report.CER.Rate != 1 {
		t.Fatalf("empty-output CER = %+v, want two reference runes and rate 1", report.CER)
	}

	nonEmptyModel := newOCRAlwaysARecognizer(t, config)
	nonEmptyDecoded, err := nonEmptyModel.Infer(ctx, image)
	if err != nil {
		t.Fatalf("Infer always-a model: %v", err)
	}
	if nonEmptyDecoded != "a" {
		t.Fatalf("controllable non-empty model decoded %q, want %q", nonEmptyDecoded, "a")
	}
	nonEmpty, err := nonEmptyModel.Evaluate(ctx, []glyphs.Sample{{Text: "", Image: image}})
	if err != nil {
		t.Fatal(err)
	}
	if nonEmpty.EmptyOutputs != 0 || nonEmpty.EmptyOutputRatio != 0 {
		t.Fatalf("non-empty decoded output was counted as empty: %+v", nonEmpty)
	}
	if nonEmpty.ExactLines != 0 {
		t.Fatalf("non-empty decoded output unexpectedly matched empty reference: %+v", nonEmpty)
	}
	spaceModel := newOCRAlwaysSpaceRecognizer(t, config)
	spaceDecoded, err := spaceModel.Infer(ctx, image)
	if err != nil {
		t.Fatalf("Infer always-space model: %v", err)
	}
	if spaceDecoded != " " {
		t.Fatalf("controllable whitespace model decoded %q, want a single space", spaceDecoded)
	}
	spaceReport, err := spaceModel.Evaluate(ctx, []glyphs.Sample{{Text: "", Image: image}})
	if err != nil {
		t.Fatalf("Evaluate whitespace model: %v", err)
	}
	if spaceReport.EmptyOutputs != 0 || spaceReport.EmptyOutputRatio != 0 {
		t.Fatalf("whitespace decoded output was counted as empty: %+v", spaceReport)
	}
	if spaceReport.ExactLines != 0 {
		t.Fatalf("whitespace decoded output unexpectedly matched empty reference: %+v", spaceReport)
	}
	emptySet, err := emptyModel.Evaluate(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if emptySet.Samples != 0 || emptySet.EmptyOutputs != 0 || emptySet.EmptyOutputRatio != 0 || emptySet.CER.Defined {
		t.Fatalf("empty sample set report = %+v, want zero ratio and undefined CER", emptySet)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := emptyModel.Evaluate(canceled, []glyphs.Sample{{Text: "a", Image: image}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Evaluate = %v, want context.Canceled", err)
	}
}

func newOCRRecognizerWithMode(t *testing.T, config ocr.RecognizerConfig, trainable bool) *ocr.Recognizer {
	t.Helper()
	config.TrainableEncoder = trainable
	model, err := ocr.NewRecognizer(config)
	if err != nil {
		t.Fatalf("NewRecognizer(%v): %v", trainable, err)
	}
	return model
}

func newOCRZeroOutputRecognizer(t *testing.T, config ocr.RecognizerConfig) *ocr.Recognizer {
	t.Helper()
	parameters := parametersForOCRControlModel(t, config)
	for i := range parameters.Core.Weights {
		parameters.Core.Weights[i] = 0
	}
	for i := range parameters.Core.Bias {
		parameters.Core.Bias[i] = 0
	}
	for i := range parameters.Readout {
		parameters.Readout[i] = 0
	}
	model, err := ocr.NewRecognizerWith(config, parameters)
	if err != nil {
		t.Fatalf("NewRecognizerWith zero output: %v", err)
	}
	return model
}

func newOCRAlwaysARecognizer(t *testing.T, config ocr.RecognizerConfig) *ocr.Recognizer {
	t.Helper()
	parameters := parametersForOCRControlModel(t, config)
	for i := range parameters.Core.Weights {
		parameters.Core.Weights[i] = 0
	}
	for i := range parameters.Core.Bias {
		parameters.Core.Bias[i] = 0
	}
	for hidden := 0; hidden < config.Hidden; hidden++ {
		parameters.Core.Bias[config.Height+hidden] = 1
		parameters.Readout[hidden*(len(config.Alphabet)+1)+1] = 10
	}
	model, err := ocr.NewRecognizerWith(config, parameters)
	if err != nil {
		t.Fatalf("NewRecognizerWith always-a output: %v", err)
	}
	return model
}

func newOCRAlwaysSpaceRecognizer(t *testing.T, config ocr.RecognizerConfig) *ocr.Recognizer {
	t.Helper()
	spaceClass := -1
	for i, symbol := range config.Alphabet {
		if symbol == ' ' {
			spaceClass = i + 1
			break
		}
	}
	if spaceClass < 0 {
		t.Fatalf("control config alphabet %q has no space class", string(config.Alphabet))
	}
	parameters := parametersForOCRControlModel(t, config)
	for i := range parameters.Core.Weights {
		parameters.Core.Weights[i] = 0
	}
	for i := range parameters.Core.Bias {
		parameters.Core.Bias[i] = 0
	}
	for hidden := 0; hidden < config.Hidden; hidden++ {
		parameters.Core.Bias[config.Height+hidden] = 1
		parameters.Readout[hidden*(len(config.Alphabet)+1)+spaceClass] = 10
	}
	model, err := ocr.NewRecognizerWith(config, parameters)
	if err != nil {
		t.Fatalf("NewRecognizerWith always-space output: %v", err)
	}
	return model
}

func parametersForOCRControlModel(t *testing.T, config ocr.RecognizerConfig) learning.Parameters {
	t.Helper()
	model, err := ocr.NewRecognizer(config)
	if err != nil {
		t.Fatalf("NewRecognizer control model: %v", err)
	}
	return model.Parameters()
}

func equalOCRParameterBits(a, b learning.Parameters) bool {
	return equalOCRFloatBits(a.Core.Weights, b.Core.Weights) && equalOCRFloatBits(a.Core.Bias, b.Core.Bias) &&
		equalOCRFloatBits(a.Core.LogTau, b.Core.LogTau) && equalOCRFloatBits(a.ThetaRaw, b.ThetaRaw) &&
		equalOCRFloatBits(a.Encoder, b.Encoder) && equalOCRFloatBits(a.Readout, b.Readout)
}

func equalOCRParameterShape(a, b learning.Parameters) bool {
	return len(a.Core.Weights) == len(b.Core.Weights) && len(a.Core.Bias) == len(b.Core.Bias) &&
		len(a.Core.LogTau) == len(b.Core.LogTau) && len(a.ThetaRaw) == len(b.ThetaRaw) &&
		len(a.Encoder) == len(b.Encoder) && len(a.Readout) == len(b.Readout)
}

func equalOCRFloatBits(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Float64bits(a[i]) != math.Float64bits(b[i]) {
			return false
		}
	}
	return true
}
