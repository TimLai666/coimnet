package multimodaleval

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/multimodal"
	"github.com/TimLai666/coimnet/multimodal/synthetic"
	"github.com/TimLai666/coimnet/signal"
)

const (
	fixtureSettle = 3
	fixtureRate   = 0.05
	fixtureHidden = 16
	fixtureSeed   = uint64(1)
	fixtureEpochs = 6
)

// fixtureConfig is the shared data draw of the concept tests: 180 samples
// with 20% of the audio events missing.
var fixtureConfig = synthetic.Config{Samples: 180, AudioDropRate: 0.2}

// holdout is the only (shape, colour) combination kept out of training.
var holdout = []synthetic.Label{{Shape: 2, Colour: 1}}

// cloneSample returns a deep copy of s.
func cloneSample(s multimodal.Sample) multimodal.Sample {
	out := s
	out.Modalities = make(map[string]multimodal.Modality, len(s.Modalities))
	for name, m := range s.Modalities {
		out.Modalities[name] = multimodal.Modality{
			Present: m.Present,
			Values:  append([]float64(nil), m.Values...),
			Shape:   append([]int(nil), m.Shape...),
		}
	}
	out.Timestamps = make(map[string]signal.Timestamp, len(s.Timestamps))
	for name, ts := range s.Timestamps {
		out.Timestamps[name] = ts
	}
	return out
}

func TestOnlyModalityMarksOthersAbsent(t *testing.T) {
	samples, _, err := synthetic.Generate(1, fixtureConfig)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	base := cloneSample(samples[0])
	one, err := onlyModality(base, "image")
	if err != nil {
		t.Fatalf("onlyModality: %v", err)
	}
	for name, m := range one.Modalities {
		if name == "image" {
			if !m.Present || m.Values == nil {
				t.Fatalf("image should stay present with values, got %+v", m)
			}
			continue
		}
		if m.Present {
			t.Fatalf("modality %q should be absent, got present %+v", name, m)
		}
		if m.Values != nil {
			t.Fatalf("modality %q: absent values must be nil, got %v", name, m.Values)
		}
	}
	if !reflect.DeepEqual(base, samples[0]) {
		t.Fatalf("onlyModality modified the source sample")
	}

	missing := cloneSample(samples[0])
	missing.Modalities["audio"] = multimodal.Modality{
		Present: false,
		Values:  nil,
		Shape:   append([]int(nil), samples[0].Modalities["audio"].Shape...),
	}
	if _, err := onlyModality(missing, "audio"); err == nil {
		t.Fatalf("onlyModality on an absent modality should error")
	}
}

func TestInputHasNoIDs(t *testing.T) {
	samples, labels, err := synthetic.Generate(1, fixtureConfig)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	layout := synthetic.DefaultLayout(fixtureConfig)
	base := cloneSample(samples[0])
	other := base
	other.EntityID = "a-different-entity"
	other.EventID = "a-different-event"

	exBase, err := buildExamples([]multimodal.Sample{base}, labels, []int{0}, layout, fixtureSettle)
	if err != nil {
		t.Fatalf("buildExamples base: %v", err)
	}
	exOther, err := buildExamples([]multimodal.Sample{other}, labels, []int{0}, layout, fixtureSettle)
	if err != nil {
		t.Fatalf("buildExamples other: %v", err)
	}
	if len(exBase) != len(exOther) {
		t.Fatalf("example counts differ: %d vs %d", len(exBase), len(exOther))
	}
	for i := range exBase {
		if !reflect.DeepEqual(exBase[i].Input, exOther[i].Input) {
			t.Fatalf("example %d: inputs differ for samples that differ only in EntityID/EventID", i)
		}
	}
}

func TestMissingDiffersFromZero(t *testing.T) {
	samples, _, err := synthetic.Generate(1, fixtureConfig)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	layout := synthetic.DefaultLayout(fixtureConfig)
	base := cloneSample(samples[0])

	missing := cloneSample(base)
	missing.Modalities["audio"] = multimodal.Modality{
		Present: false,
		Values:  nil,
		Shape:   append([]int(nil), base.Modalities["audio"].Shape...),
	}
	zeroed := cloneSample(base)
	zeroed.Modalities["audio"] = multimodal.Modality{
		Present: true,
		Values:  make([]float64, layout.Widths["audio"]),
		Shape:   append([]int(nil), base.Modalities["audio"].Shape...),
	}

	vecMissing, err := multimodal.Vector(missing, layout)
	if err != nil {
		t.Fatalf("Vector(missing): %v", err)
	}
	vecZero, err := multimodal.Vector(zeroed, layout)
	if err != nil {
		t.Fatalf("Vector(zeroed): %v", err)
	}
	if reflect.DeepEqual(vecMissing, vecZero) {
		t.Fatalf("missing and zero audio encode to the same vector")
	}

	tr, err := newTrainer(fixtureSeed, layout, fixtureHidden, fixtureRate)
	if err != nil {
		t.Fatalf("newTrainer: %v", err)
	}
	settle := func(v []float64) [][]float64 {
		rows := make([][]float64, fixtureSettle)
		for i := range rows {
			rows[i] = append([]float64(nil), v...)
		}
		return rows
	}
	ctx := context.Background()
	pMissing, err := tr.Predict(ctx, settle(vecMissing))
	if err != nil {
		t.Fatalf("Predict(missing): %v", err)
	}
	pZero, err := tr.Predict(ctx, settle(vecZero))
	if err != nil {
		t.Fatalf("Predict(zeroed): %v", err)
	}
	if reflect.DeepEqual(pMissing, pZero) {
		t.Fatalf("untrained model predicts identically for missing and zero audio")
	}
}

func TestHoldoutNeverTrains(t *testing.T) {
	samples, labels, err := synthetic.Generate(1, fixtureConfig)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	layout := synthetic.DefaultLayout(fixtureConfig)
	train, eval, err := synthetic.SplitUnseenCombinations(labels, holdout)
	if err != nil {
		t.Fatalf("SplitUnseenCombinations: %v", err)
	}
	if len(train) == 0 || len(eval) == 0 {
		t.Fatalf("split produced train %d eval %d", len(train), len(eval))
	}
	examples, err := buildExamples(samples, labels, train, layout, fixtureSettle)
	if err != nil {
		t.Fatalf("buildExamples: %v", err)
	}
	for _, ex := range examples {
		if ex.Label == holdout[0] {
			t.Fatalf("training example carries held-out label %+v", ex.Label)
		}
	}
}

func TestConceptsTransferAcrossModalities(t *testing.T) {
	const chance = 1.0 / (synthetic.Shapes * synthetic.Colours)

	samples1, labels1, err := synthetic.Generate(1, fixtureConfig)
	if err != nil {
		t.Fatalf("Generate(1): %v", err)
	}
	samples2, labels2, err := synthetic.Generate(2, fixtureConfig)
	if err != nil {
		t.Fatalf("Generate(2): %v", err)
	}
	layout := synthetic.DefaultLayout(fixtureConfig)
	trainIdx, _, err := synthetic.SplitUnseenCombinations(labels1, holdout)
	if err != nil {
		t.Fatalf("SplitUnseenCombinations(1): %v", err)
	}
	seenIdx, holdoutIdx, err := synthetic.SplitUnseenCombinations(labels2, holdout)
	if err != nil {
		t.Fatalf("SplitUnseenCombinations(2): %v", err)
	}
	trainExamples, err := buildExamples(samples1, labels1, trainIdx, layout, fixtureSettle)
	if err != nil {
		t.Fatalf("buildExamples train: %v", err)
	}
	seenExamples, err := buildExamples(samples2, labels2, seenIdx, layout, fixtureSettle)
	if err != nil {
		t.Fatalf("buildExamples seen: %v", err)
	}
	holdoutExamples, err := buildExamples(samples2, labels2, holdoutIdx, layout, fixtureSettle)
	if err != nil {
		t.Fatalf("buildExamples holdout: %v", err)
	}

	ctx := context.Background()
	tr, err := newTrainer(fixtureSeed, layout, fixtureHidden, fixtureRate)
	if err != nil {
		t.Fatalf("newTrainer: %v", err)
	}
	beforeSeen, err := score(ctx, tr, seenExamples)
	if err != nil {
		t.Fatalf("score before: %v", err)
	}
	beforeHoldout, err := score(ctx, tr, holdoutExamples)
	if err != nil {
		t.Fatalf("score before holdout: %v", err)
	}
	updates, err := train(ctx, tr, trainExamples, fixtureEpochs)
	if err != nil {
		t.Fatalf("train: %v", err)
	}
	afterSeen, err := score(ctx, tr, seenExamples)
	if err != nil {
		t.Fatalf("score after: %v", err)
	}
	afterHoldout, err := score(ctx, tr, holdoutExamples)
	if err != nil {
		t.Fatalf("score after holdout: %v", err)
	}

	t.Logf("updates=%d\nseen before:  %+v\nseen after:   %+v\nholdout before: %+v\nholdout after:  %+v",
		updates, beforeSeen, afterSeen, beforeHoldout, afterHoldout)

	if afterSeen.ImageToText <= beforeSeen.ImageToText || afterSeen.ImageToText <= chance {
		t.Errorf("seen ImageToText after %v must beat before %v and chance %v", afterSeen.ImageToText, beforeSeen.ImageToText, chance)
	}
	if afterSeen.TextToImage <= beforeSeen.TextToImage || afterSeen.TextToImage <= chance {
		t.Errorf("seen TextToImage after %v must beat before %v and chance %v", afterSeen.TextToImage, beforeSeen.TextToImage, chance)
	}

	if afterHoldout.Inputs == 0 {
		t.Fatalf("holdout group scored no inputs")
	}
	for name, v := range afterHoldout.ShapeAccuracy {
		if !finiteUnit(v) {
			t.Errorf("holdout shape accuracy %q = %v, want finite in [0,1]", name, v)
		}
	}
	for name, v := range afterHoldout.ColourAccuracy {
		if !finiteUnit(v) {
			t.Errorf("holdout colour accuracy %q = %v, want finite in [0,1]", name, v)
		}
	}
	for _, v := range []float64{afterHoldout.ImageToText, afterHoldout.TextToImage, afterHoldout.AudioToImageShape} {
		if !finiteUnit(v) {
			t.Errorf("holdout retrieval score %v, want finite in [0,1]", v)
		}
	}
}

func TestTrainReturnsTheUpdateCount(t *testing.T) {
	samples, labels, err := synthetic.Generate(1, fixtureConfig)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	layout := synthetic.DefaultLayout(fixtureConfig)
	trainIdx, _, err := synthetic.SplitUnseenCombinations(labels, holdout)
	if err != nil {
		t.Fatalf("SplitUnseenCombinations: %v", err)
	}
	examples, err := buildExamples(samples, labels, trainIdx, layout, fixtureSettle)
	if err != nil {
		t.Fatalf("buildExamples: %v", err)
	}
	tr, err := newTrainer(fixtureSeed, layout, fixtureHidden, fixtureRate)
	if err != nil {
		t.Fatalf("newTrainer: %v", err)
	}
	const epochs = 2
	updates, err := train(context.Background(), tr, examples, epochs)
	if err != nil {
		t.Fatalf("train: %v", err)
	}
	want := uint64(epochs * len(examples))
	if updates != want {
		t.Errorf("train returned %d, want %d (= epochs × len(examples))", updates, want)
	}
	if snap := tr.Snapshot().Updates; updates != snap {
		t.Errorf("train returned %d, Snapshot().Updates = %d", updates, snap)
	}
}

func TestScoringIsDeterministic(t *testing.T) {
	samples, labels, err := synthetic.Generate(1, fixtureConfig)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	layout := synthetic.DefaultLayout(fixtureConfig)
	trainIdx, _, err := synthetic.SplitUnseenCombinations(labels, holdout)
	if err != nil {
		t.Fatalf("SplitUnseenCombinations: %v", err)
	}
	examples, err := buildExamples(samples, labels, trainIdx, layout, fixtureSettle)
	if err != nil {
		t.Fatalf("buildExamples: %v", err)
	}
	ctx := context.Background()

	run := func() (Scores, uint64, error) {
		tr, err := newTrainer(fixtureSeed, layout, fixtureHidden, fixtureRate)
		if err != nil {
			return Scores{}, 0, err
		}
		updates, err := train(ctx, tr, examples, fixtureEpochs)
		if err != nil {
			return Scores{}, 0, err
		}
		s, err := score(ctx, tr, examples)
		return s, updates, err
	}
	s1, u1, err := run()
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	s2, u2, err := run()
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if u1 != u2 {
		t.Fatalf("update counts differ across identical runs: %d vs %d", u1, u2)
	}
	if !reflect.DeepEqual(s1, s2) {
		t.Fatalf("scores differ across identical runs:\n%+v\n%+v", s1, s2)
	}
}

// finiteUnit reports whether v is finite and in [0, 1].
func finiteUnit(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1
}

func TestRetrieveHandlesEmptySides(t *testing.T) {
	match := func(query, found synthetic.Label) bool { return query == found }
	queries := []prediction{{Label: synthetic.Label{Shape: 0, Colour: 1}, Modality: "image", Vec: []float64{1, 0}}}
	gallery := []prediction{{Label: synthetic.Label{Shape: 0, Colour: 1}, Modality: "text", Vec: []float64{1, 0}}}
	if hits, n := retrieve(nil, gallery, match); hits != 0 || n != 0 {
		t.Errorf("retrieve with empty queries = (%d, %d), want (0, 0)", hits, n)
	}
	if hits, n := retrieve(queries, nil, match); hits != 0 || n != 0 {
		t.Errorf("retrieve with empty gallery = (%d, %d), want (0, 0)", hits, n)
	}

	samples, labels, err := synthetic.Generate(1, fixtureConfig)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	layout := synthetic.DefaultLayout(fixtureConfig)
	trainIdx, _, err := synthetic.SplitUnseenCombinations(labels, holdout)
	if err != nil {
		t.Fatalf("SplitUnseenCombinations: %v", err)
	}
	examples, err := buildExamples(samples, labels, trainIdx, layout, fixtureSettle)
	if err != nil {
		t.Fatalf("buildExamples: %v", err)
	}
	imageOnly := make([]example, 0, len(examples))
	for _, ex := range examples {
		if ex.Modality == "image" {
			imageOnly = append(imageOnly, ex)
		}
	}
	if len(imageOnly) == 0 {
		t.Fatal("fixture produced no image examples")
	}
	tr, err := newTrainer(fixtureSeed, layout, fixtureHidden, fixtureRate)
	if err != nil {
		t.Fatalf("newTrainer: %v", err)
	}
	s, err := score(context.Background(), tr, imageOnly)
	if err != nil {
		t.Fatalf("score: %v", err)
	}
	for name, v := range map[string]float64{
		"image_to_text":        s.ImageToText,
		"text_to_image":        s.TextToImage,
		"audio_to_image_shape": s.AudioToImageShape,
	} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v != 0 {
			t.Errorf("%s = %v, want 0 (not NaN) when only image inputs are scored", name, v)
		}
	}
}
