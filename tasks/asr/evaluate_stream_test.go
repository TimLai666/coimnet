package asr_test

import (
	"context"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/tasks/asr"
	"github.com/TimLai666/coimnet/tasks/asr/audio"
	"github.com/TimLai666/coimnet/tasks/ocr"
)

func TestEvaluateStreaming(t *testing.T) {
	ctx := context.Background()
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}

	first, err := asr.Synthesize(synthConfig(), "a", 31)
	if err != nil {
		t.Fatalf("Synthesize first waveform: %v", err)
	}
	second, err := asr.Synthesize(synthConfig(), "bc", 32)
	if err != nil {
		t.Fatalf("Synthesize second waveform: %v", err)
	}
	utterances := []asr.Utterance{
		{Text: "a", Samples: first},
		{Text: "bc", Samples: second},
	}

	got, err := r.EvaluateStreaming(ctx, utterances, 173)
	if err != nil {
		t.Fatalf("EvaluateStreaming: %v", err)
	}
	if got.Samples != len(utterances) {
		t.Fatalf("samples %d, want %d", got.Samples, len(utterances))
	}
	if len(got.Measurements) != len(utterances) {
		t.Fatalf("measurements %d, want %d", len(got.Measurements), len(utterances))
	}

	wantCER := manualAggregate(t, utterances, got.Measurements, false)
	wantWER := manualAggregate(t, utterances, got.Measurements, true)
	if !reflect.DeepEqual(got.CER, wantCER) {
		t.Fatalf("CER %+v, hand-aggregated %+v", got.CER, wantCER)
	}
	if !reflect.DeepEqual(got.WER, wantWER) {
		t.Fatalf("WER %+v, hand-aggregated %+v", got.WER, wantWER)
	}
	exact := 0
	audioSeconds := 0.0
	for i, measurement := range got.Measurements {
		if measurement.Text == utterances[i].Text {
			exact++
		}
		audioSeconds += float64(len(utterances[i].Samples)) / float64(recognizerConfig().SampleRate)
		if measurement.AudioSampleCount != len(utterances[i].Samples) {
			t.Errorf("utterance %d measured %d samples, want %d", i, measurement.AudioSampleCount, len(utterances[i].Samples))
		}
		if measurement.AudioSeconds != float64(len(utterances[i].Samples))/float64(recognizerConfig().SampleRate) {
			t.Errorf("utterance %d measured %.12g seconds, want %.12g", i, measurement.AudioSeconds, float64(len(utterances[i].Samples))/float64(recognizerConfig().SampleRate))
		}
	}
	if got.ExactUtterances != float64(exact)/float64(len(utterances)) {
		t.Fatalf("exact utterance share %.12g, want %.12g", got.ExactUtterances, float64(exact)/float64(len(utterances)))
	}
	if got.AudioSeconds != audioSeconds {
		t.Fatalf("audio seconds %.12g, want %.12g", got.AudioSeconds, audioSeconds)
	}
	if got.InferenceDurationNS < 0 || got.MaxFeedDurationNS < 0 {
		t.Fatalf("negative aggregate durations: %+v", got)
	}
	for i, measurement := range got.Measurements {
		if measurement.WallClockDurationNS < 0 || measurement.MaxFeedDurationNS < 0 {
			t.Errorf("utterance %d has negative timing: %+v", i, measurement)
		}
	}

	front := recognizerConfig()
	if got.FrontEndVersion != audio.FrontEndVersion {
		t.Fatalf("front end version %q, want %q", got.FrontEndVersion, audio.FrontEndVersion)
	}
	if got.SampleRate != front.SampleRate {
		t.Fatalf("front end sample rate %d, want %d", got.SampleRate, front.SampleRate)
	}
	if got.FrontEndConfig != front.FrontEnd {
		t.Fatalf("front end config %+v, want %+v", got.FrontEndConfig, front.FrontEnd)
	}

	// Timing is deliberately observational: record it in the test log without
	// making a wall-clock threshold part of the reproducibility contract.
	t.Logf("stream fixture: audio_seconds=%.6f inference_ns=%d max_feed_ns=%d CER=%.6f WER=%.6f", got.AudioSeconds, got.InferenceDurationNS, got.MaxFeedDurationNS, got.CER.Rate, got.WER.Rate)
}

func TestEvaluateStreamingEmpty(t *testing.T) {
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}

	got, err := r.EvaluateStreaming(context.Background(), nil, 173)
	if err != nil {
		t.Fatalf("EvaluateStreaming empty: %v", err)
	}
	if got.Samples != 0 || got.ExactUtterances != 0 || got.AudioSeconds != 0 || got.InferenceDurationNS != 0 || got.MaxFeedDurationNS != 0 {
		t.Fatalf("empty report has nonzero aggregate fields: %+v", got)
	}
	if got.CER.Defined || got.WER.Defined || got.CER.Rate != 0 || got.WER.Rate != 0 {
		t.Fatalf("empty report rates should be undefined: CER %+v WER %+v", got.CER, got.WER)
	}
	if len(got.Measurements) != 0 {
		t.Fatalf("empty report has %d measurements", len(got.Measurements))
	}
}

func TestEvaluateStreamingErrors(t *testing.T) {
	ctx := context.Background()
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}
	x, err := asr.Synthesize(synthConfig(), "a", 41)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	for _, chunk := range []int{0, -1} {
		got, err := r.EvaluateStreaming(ctx, []asr.Utterance{{Text: "a", Samples: x}}, chunk)
		if err == nil || !strings.Contains(err.Error(), "chunk") {
			t.Fatalf("chunk %d error %v does not identify the invalid chunk", chunk, err)
		}
		if !reflect.DeepEqual(got, asr.StreamEvalReport{}) {
			t.Fatalf("chunk %d returned partial report %+v", chunk, got)
		}
	}

	bad := append([]float64(nil), x...)
	bad[len(bad)/2] = math.NaN()
	got, err := r.EvaluateStreaming(ctx, []asr.Utterance{
		{Text: "a", Samples: x},
		{Text: "bc", Samples: bad},
	}, 173)
	if err == nil || !strings.Contains(err.Error(), "utterance 1") {
		t.Fatalf("bad utterance error %v does not identify utterance 1", err)
	}
	if !reflect.DeepEqual(got, asr.StreamEvalReport{}) {
		t.Fatalf("failed evaluation returned partial report %+v", got)
	}

	got, err = r.EvaluateStreaming(ctx, []asr.Utterance{{Text: "a", Samples: x}}, 173)
	if err != nil {
		t.Fatalf("control EvaluateStreaming: %v", err)
	}
	if got.Samples != 1 {
		t.Fatalf("control report samples %d, want 1", got.Samples)
	}

	if _, err := (*asr.Recognizer)(nil).EvaluateStreaming(ctx, []asr.Utterance{{Text: "a", Samples: x}}, 173); err == nil || !strings.Contains(err.Error(), "nil recognizer") {
		t.Fatalf("nil recognizer error %v does not identify nil recognizer", err)
	}
}

func TestEvaluateStreamingReproducible(t *testing.T) {
	ctx := context.Background()
	first, err := asr.Synthesize(synthConfig(), "ab", 51)
	if err != nil {
		t.Fatalf("Synthesize first waveform: %v", err)
	}
	second, err := asr.Synthesize(synthConfig(), "c", 52)
	if err != nil {
		t.Fatalf("Synthesize second waveform: %v", err)
	}
	utterances := []asr.Utterance{{Text: "ab", Samples: first}, {Text: "c", Samples: second}}
	leftRecognizer, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatalf("NewRecognizer left: %v", err)
	}
	rightRecognizer, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatalf("NewRecognizer right: %v", err)
	}
	left, err := leftRecognizer.EvaluateStreaming(ctx, utterances, 173)
	if err != nil {
		t.Fatalf("left EvaluateStreaming: %v", err)
	}
	right, err := rightRecognizer.EvaluateStreaming(ctx, utterances, 173)
	if err != nil {
		t.Fatalf("right EvaluateStreaming: %v", err)
	}
	if left.CER != right.CER || left.WER != right.WER || left.ExactUtterances != right.ExactUtterances || left.Samples != right.Samples || left.AudioSeconds != right.AudioSeconds {
		t.Fatalf("deterministic report fields differ:\nleft %+v\nright %+v", left, right)
	}
	if len(left.Measurements) != len(right.Measurements) {
		t.Fatalf("measurement counts differ: %d and %d", len(left.Measurements), len(right.Measurements))
	}
	for i := range left.Measurements {
		l, r := left.Measurements[i], right.Measurements[i]
		l.WallClockDurationNS, l.MaxFeedDurationNS, l.FlushDurationNS = 0, 0, 0
		r.WallClockDurationNS, r.MaxFeedDurationNS, r.FlushDurationNS = 0, 0, 0
		if !reflect.DeepEqual(l, r) {
			t.Fatalf("measurement %d differs after removing timing:\nleft %+v\nright %+v", i, l, r)
		}
	}
}

func manualAggregate(t *testing.T, utterances []asr.Utterance, measurements []asr.StreamMeasurement, words bool) ocr.EditReport {
	t.Helper()
	var substitutions, deletions, insertions, referenceLength int
	for i, measurement := range measurements {
		var edit ocr.EditReport
		if words {
			edit = ocr.WER(utterances[i].Text, measurement.Text)
		} else {
			edit = ocr.CER(utterances[i].Text, measurement.Text)
		}
		substitutions += edit.Substitutions
		deletions += edit.Deletions
		insertions += edit.Insertions
		referenceLength += edit.ReferenceLength
	}
	report := ocr.EditReport{
		Substitutions:   substitutions,
		Deletions:       deletions,
		Insertions:      insertions,
		ReferenceLength: referenceLength,
	}
	if referenceLength > 0 {
		report.Rate = float64(substitutions+deletions+insertions) / float64(referenceLength)
		report.Defined = true
	}
	return report
}
