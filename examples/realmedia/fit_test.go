package main

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/tasks/media/realdata"
)

func TestFitUsesInsyraCoreAndIndependentSequenceInference(t *testing.T) {
	train := sampleForTest("train-source", "train-clip", "train", "a machine in a dark corridor", .7, .12)
	testSample := sampleForTest("heldout-source", "test-clip", "test", "a rabbit in a sunny clearing", .25, -.08)
	result, err := fitSamples(context.Background(), []realdata.Sample{train, testSample}, 32)
	if err != nil {
		t.Fatal(err)
	}
	if result.Snapshot.Updates != 32 {
		t.Fatalf("learning updates = %d, want 32", result.Snapshot.Updates)
	}
	if reflect.DeepEqual(result.Before.Parameters.Core.Weights, result.Snapshot.Parameters.Core.Weights) {
		t.Fatal("learning core weights did not change during supervised training")
	}
	before := result.BeforeScores["train"].BalancedMSE
	after := result.AfterScores["train"].BalancedMSE
	if !(after < before*.9) {
		t.Fatalf("training balanced MSE = %g before, %g after, want at least 10%% reduction", before, after)
	}
	if len(result.Predictions) != 2 || len(result.Predictions[0].Rows) != len(train.Frames) {
		t.Fatalf("independent inference produced %d predictions; want both sequences with %d rows", len(result.Predictions), len(train.Frames))
	}
	for i, row := range result.Predictions[0].Rows {
		if len(row) != frameOutputSize {
			t.Fatalf("prediction row %d has %d values, want %d image/audio values", i, len(row), frameOutputSize)
		}
		for j, value := range row {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				t.Fatalf("prediction row %d value %d is non-finite: %g", i, j, value)
			}
		}
	}
	if result.AfterScores["test"].Samples != 1 || result.AfterScores["test"].AudioSamples != len(testSample.Audio) {
		t.Fatalf("test metrics do not cover the held-out audio target: %+v", result.AfterScores["test"])
	}
}

func TestPromptFeaturesAreDeterministicAndContentDependent(t *testing.T) {
	first := makeInputRows("A white rabbit leaves a dark tree hollow.", 8)
	repeat := makeInputRows("A white rabbit leaves a dark tree hollow.", 8)
	other := makeInputRows("A pale face peers through orange machinery.", 8)
	if !reflect.DeepEqual(first, repeat) {
		t.Fatal("identical annotations produced different model inputs")
	}
	if reflect.DeepEqual(first, other) {
		t.Fatal("different manually annotated clip descriptions produced identical model inputs")
	}
}

func TestScorePredictionsRejectsIncompleteOrMisalignedCoverage(t *testing.T) {
	samples := []realdata.Sample{
		sampleForTest("source-a", "clip-a", "train", "a machine", .5, .1),
		sampleForTest("source-b", "clip-b", "test", "a rabbit", .4, -.1),
	}
	good := []sequencePrediction{predictionForTest(samples[0]), predictionForTest(samples[1])}
	if _, err := scorePredictions(samples, good); err != nil {
		t.Fatalf("complete prediction set rejected: %v", err)
	}
	cases := map[string][]sequencePrediction{
		"missing sample":   {good[0]},
		"swapped identity": {good[1], good[0]},
		"wrong frame count": {
			{SourceID: samples[0].SourceID, SegmentID: samples[0].SegmentID, Split: samples[0].Split, Rows: good[0].Rows[:1]},
			good[1],
		},
		"wrong value count": {
			{SourceID: samples[0].SourceID, SegmentID: samples[0].SegmentID, Split: samples[0].Split, Rows: [][]float64{make([]float64, frameOutputSize-1)}},
			good[1],
		},
	}
	for name, predictions := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := scorePredictions(samples, predictions); err == nil {
				t.Fatalf("scorePredictions error = %v, want a coverage or shape error", err)
			}
		})
	}
}

func TestScorePredictionsMeasuresFrameToFrameError(t *testing.T) {
	sample := sampleForTest("source", "clip", "test", "one bright pixel moves", 0, 0)
	sample.Frames[1].Pixels[0] = 1
	prediction := predictionForTest(sample)
	scores, err := scorePredictions([]realdata.Sample{sample}, []sequencePrediction{prediction})
	if err != nil {
		t.Fatal(err)
	}
	got := scores["test"]
	// The lone target pixel changes at frames 1 and 2. Both predicted
	// transitions are zero; the other six transitions and 191 pixels match.
	want := 2.0 / float64(7*framePixels)
	if got.TemporalPairs != 7 || math.Abs(got.TemporalDeltaMSE-want) > 1e-15 {
		t.Fatalf("temporal score = %d pairs, %g MSE; want 7 pairs, %g MSE", got.TemporalPairs, got.TemporalDeltaMSE, want)
	}
	prediction.Rows[1][0] = imageOutputScale()
	scores, err = scorePredictions([]realdata.Sample{sample}, []sequencePrediction{prediction})
	if err != nil {
		t.Fatal(err)
	}
	if scores["test"].TemporalDeltaMSE != 0 {
		t.Fatalf("matching frame transitions have MSE %g, want zero", scores["test"].TemporalDeltaMSE)
	}
}

func predictionForTest(sample realdata.Sample) sequencePrediction {
	rows := make([][]float64, len(sample.Frames))
	for i := range rows {
		rows[i] = make([]float64, frameOutputSize)
	}
	return sequencePrediction{SourceID: sample.SourceID, SegmentID: sample.SegmentID, Split: sample.Split, Rows: rows}
}

func sampleForTest(sourceID, segmentID, split, prompt string, pixel, audio float64) realdata.Sample {
	const frames = 8
	const samplesPerFrame = 2000
	sample := realdata.Sample{
		SourceID: sourceID, SegmentID: segmentID, Split: split, Prompt: prompt,
		PromptSource: "human_annotation", AnnotationNote: "Unit test annotation for training-path verification.",
		StartMillis: 0, DurationMillis: 2000, FrameRate: 4, AudioSampleRate: 8000,
		Frames: make([]realdata.Frame, frames), Audio: make([]float64, frames*samplesPerFrame),
	}
	for i := range sample.Audio {
		sample.Audio[i] = audio
	}
	for frame := range sample.Frames {
		pixels := make([]float64, 8*8*3)
		for i := range pixels {
			pixels[i] = pixel
		}
		sample.Frames[frame] = realdata.Frame{
			Index: frame, TimeMillis: int64(frame * 250), Pixels: pixels,
			AudioStart: frame * samplesPerFrame, AudioEnd: (frame + 1) * samplesPerFrame,
		}
	}
	return sample
}
