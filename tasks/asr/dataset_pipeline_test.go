package asr_test

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"github.com/TimLai666/coimnet/tasks/asr"
	"github.com/TimLai666/coimnet/tasks/asr/audio"
	"github.com/TimLai666/coimnet/tasks/ocr"
)

func TestImportedDatasetPipeline(t *testing.T) {
	root := t.TempDir()
	synth := synthConfig()
	synth.SampleRate = 8000
	synth.DurationMS = 80
	synth.GapMS = 20

	records := []datasetManifestRecord{
		{Path: "record-00.wav", Text: "a", Speaker: "speaker-00", Session: "session-00", SampleRate: synth.SampleRate, Channels: 1},
		{Path: "record-01.wav", Text: "bc", Speaker: "speaker-01", Session: "session-01", SampleRate: synth.SampleRate, Channels: 1},
		{Path: "record-02.wav", Text: "ca", Speaker: "speaker-02", Session: "session-02", SampleRate: synth.SampleRate, Channels: 1},
		{Path: "record-03.wav", Text: "abc", Speaker: "speaker-03", Session: "session-03", SampleRate: synth.SampleRate, Channels: 1},
	}
	for i, record := range records {
		samples, err := asr.Synthesize(synth, record.Text, uint64(100+i))
		if err != nil {
			t.Fatalf("Synthesize(%q): %v", record.Text, err)
		}
		if err := audio.WriteWAV(filepath.Join(root, record.Path), audio.Signal{
			SampleRate: synth.SampleRate,
			Samples:    [][]float64{samples},
		}); err != nil {
			t.Fatalf("WriteWAV(%q): %v", record.Path, err)
		}
	}

	manifestPath := filepath.Join(root, "manifest.json")
	writeDatasetManifest(t, manifestPath, datasetManifest{
		Schema: "coimnet-asr-dataset/v1",
		License: datasetLicense{
			Holder: "CoImNet integration fixture",
			Terms:  "fixture-only test terms",
			Source: "https://example.test/coimnet/asr-import-fixture",
		},
		Records: records,
	})

	dataset, err := asr.ReadDataset(manifestPath, 16000, 0.8)
	if err != nil {
		t.Fatalf("ReadDataset: %v", err)
	}
	if dataset.License.Holder != "CoImNet integration fixture" || dataset.License.Terms != "fixture-only test terms" || dataset.License.Source != "https://example.test/coimnet/asr-import-fixture" {
		t.Fatalf("License = %+v, want manifest license", dataset.License)
	}
	if len(dataset.Utterances) != len(records) || len(dataset.Metadata) != len(records) {
		t.Fatalf("imported utterances/metadata lengths = %d/%d, want %d/%d", len(dataset.Utterances), len(dataset.Metadata), len(records), len(records))
	}
	for i, record := range records {
		utterance := dataset.Utterances[i]
		metadata := dataset.Metadata[i]
		if utterance.Text != record.Text || utterance.Speaker != record.Speaker || utterance.Session != record.Session {
			t.Errorf("utterance %d = %+v, want text/speaker/session from manifest %+v", i, utterance, record)
		}
		if metadata.OriginalPath != filepath.Clean(record.Path) || metadata.OriginalSampleRate != record.SampleRate || metadata.OriginalChannels != record.Channels {
			t.Errorf("metadata %d = %+v, want original path/rate/channels from %q", i, metadata, record.Path)
		}
		assertDatasetSHA(t, filepath.Join(root, record.Path), metadata.OriginalSHA256)
		if metadata.Resample.Method != "linear" || metadata.Resample.From != record.SampleRate || metadata.Resample.To != 16000 {
			t.Errorf("metadata %d resample = %+v, want linear %d->16000", i, metadata.Resample, record.SampleRate)
		}
		if metadata.Resample.OutputLength != len(utterance.Samples) || metadata.Resample.InputLength <= 0 {
			t.Errorf("metadata %d resample lengths = %+v, imported samples = %d", i, metadata.Resample, len(utterance.Samples))
		}
		if !metadata.Normalize.Scaled || metadata.Normalize.Peak <= 0 || metadata.Normalize.Gain <= 0 || math.IsNaN(metadata.Normalize.Gain) || math.IsInf(metadata.Normalize.Gain, 0) {
			t.Errorf("metadata %d normalize = %+v, want finite positive scaling to peak 0.8", i, metadata.Normalize)
		}
		if peak := importedPeak(utterance.Samples); math.Abs(peak-0.8) > 1e-12 {
			t.Errorf("utterance %d normalized peak = %.17g, want 0.8", i, peak)
		}
	}

	train, test, err := asr.SplitBySpeakerSession(dataset.Utterances, 0.25, 42)
	if err != nil {
		t.Fatalf("SplitBySpeakerSession: %v", err)
	}
	if len(train) == 0 || len(test) == 0 {
		t.Fatalf("split lengths = (%d, %d), want both partitions non-empty", len(train), len(test))
	}
	trainSpeakers, trainSessions := identitySets(train)
	testSpeakers, testSessions := identitySets(test)
	for speaker := range trainSpeakers {
		if testSpeakers[speaker] {
			t.Fatalf("speaker %q appears in both train and test", speaker)
		}
	}
	for session := range trainSessions {
		if testSessions[session] {
			t.Fatalf("session %q appears in both train and test", session)
		}
	}

	ctx := context.Background()
	recognizer, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}
	before, err := recognizer.Evaluate(ctx, test)
	if err != nil {
		t.Fatalf("Evaluate before training: %v", err)
	}
	if before.Samples != len(test) {
		t.Fatalf("Evaluate before samples = %d, want %d", before.Samples, len(test))
	}
	assertEvalRates(t, "Evaluate before CER", before.CER, totalRuneLength(test))
	assertEvalRates(t, "Evaluate before WER", before.WER, len(test))

	for i, utterance := range train {
		report, err := recognizer.TrainUtterance(ctx, utterance.Samples, utterance.Text)
		if err != nil {
			t.Fatalf("TrainUtterance train[%d]: %v", i, err)
		}
		if report.Impossible {
			t.Fatalf("TrainUtterance train[%d] reported impossible alignment: %+v", i, report)
		}
		if math.IsNaN(report.Loss) || math.IsInf(report.Loss, 0) {
			t.Fatalf("TrainUtterance train[%d] loss = %v, want finite", i, report.Loss)
		}
		if report.Frames <= 0 || report.Labels != utf8.RuneCountInString(utterance.Text) {
			t.Fatalf("TrainUtterance train[%d] report = %+v, want positive frames and %d labels", i, report, utf8.RuneCountInString(utterance.Text))
		}
	}

	after, err := recognizer.Evaluate(ctx, test)
	if err != nil {
		t.Fatalf("Evaluate after training: %v", err)
	}
	if after.Samples != len(test) {
		t.Fatalf("Evaluate after samples = %d, want %d", after.Samples, len(test))
	}
	assertEvalRates(t, "Evaluate after CER", after.CER, totalRuneLength(test))
	assertEvalRates(t, "Evaluate after WER", after.WER, len(test))

	stream, err := recognizer.EvaluateStreaming(ctx, test, 173)
	if err != nil {
		t.Fatalf("EvaluateStreaming: %v", err)
	}
	if stream.Samples != len(test) || len(stream.Measurements) != len(test) {
		t.Fatalf("stream samples/measurements = %d/%d, want %d/%d", stream.Samples, len(stream.Measurements), len(test), len(test))
	}
	assertEvalRates(t, "EvaluateStreaming CER", stream.CER, totalRuneLength(test))
	assertEvalRates(t, "EvaluateStreaming WER", stream.WER, len(test))
	for i, measurement := range stream.Measurements {
		if measurement.AudioSampleCount != len(test[i].Samples) {
			t.Errorf("stream measurement %d sample count = %d, want %d", i, measurement.AudioSampleCount, len(test[i].Samples))
		}
	}
}

func importedPeak(samples []float64) float64 {
	peak := 0.0
	for _, sample := range samples {
		if abs := math.Abs(sample); abs > peak {
			peak = abs
		}
	}
	return peak
}

func totalRuneLength(utterances []asr.Utterance) int {
	total := 0
	for _, utterance := range utterances {
		total += utf8.RuneCountInString(utterance.Text)
	}
	return total
}

func assertEvalRates(t *testing.T, name string, report ocr.EditReport, referenceLength int) {
	t.Helper()
	if !report.Defined || report.ReferenceLength != referenceLength {
		t.Fatalf("%s = %+v, want defined reference length %d", name, report, referenceLength)
	}
	want := float64(report.Substitutions+report.Deletions+report.Insertions) / float64(report.ReferenceLength)
	if math.IsNaN(report.Rate) || math.IsInf(report.Rate, 0) || report.Rate != want {
		t.Fatalf("%s rate = %.17g, want finite edit total / reference length = %.17g", name, report.Rate, want)
	}
}
