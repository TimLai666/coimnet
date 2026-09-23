package asr

import (
	"context"
	"errors"
	"fmt"

	"github.com/TimLai666/coimnet/tasks/asr/audio"
	"github.com/TimLai666/coimnet/tasks/ocr"
)

// StreamEvalReport aggregates independent MeasureStreaming runs. Inference
// duration is the sum of each measurement's offline wall-clock duration;
// MaxFeedDurationNS is the slowest Feed call across all utterances. The front
// end version, sample rate, and configuration are reported explicitly;
// MeasureStreaming does not expose the stream's internal frame counter, so
// this report intentionally has no frame-count field.
type StreamEvalReport struct {
	CER                 ocr.EditReport       `json:"cer"`
	WER                 ocr.EditReport       `json:"wer"`
	ExactUtterances     float64              `json:"exact_utterances"`
	Samples             int                  `json:"samples"`
	AudioSeconds        float64              `json:"audio_seconds"`
	InferenceDurationNS int64                `json:"inference_duration_ns"`
	MaxFeedDurationNS   int64                `json:"max_feed_duration_ns"`
	Measurements        []StreamMeasurement  `json:"measurements"`
	FrontEndVersion     string               `json:"front_end_version"`
	FrontEndConfig      audio.FrontEndConfig `json:"front_end_config"`
	SampleRate          int                  `json:"sample_rate"`
}

// EvaluateStreaming runs every utterance through an independent streaming
// decoder, aggregates CER/WER over the total edit counts, and keeps each
// per-utterance timing measurement. A failed utterance discards the complete
// report and names its zero-based index in the returned error.
func (r *Recognizer) EvaluateStreaming(ctx context.Context, utterances []Utterance, chunk int) (StreamEvalReport, error) {
	var zero StreamEvalReport
	if r == nil || r.trainer == nil {
		return zero, errors.New("asr: nil recognizer")
	}
	if chunk <= 0 {
		return zero, fmt.Errorf("asr: streaming chunk %d must be at least one sample", chunk)
	}

	report := StreamEvalReport{
		Samples:         len(utterances),
		Measurements:    make([]StreamMeasurement, len(utterances)),
		FrontEndVersion: audio.FrontEndVersion,
		FrontEndConfig:  r.config.FrontEnd,
		SampleRate:      r.config.SampleRate,
	}
	var charSubs, charDels, charIns, charRef int
	var wordSubs, wordDels, wordIns, wordRef int
	exact := 0
	for i, utterance := range utterances {
		measurement, err := r.MeasureStreaming(ctx, utterance.Samples, chunk)
		if err != nil {
			return zero, fmt.Errorf("asr: utterance %d: %w", i, err)
		}
		report.Measurements[i] = measurement
		report.AudioSeconds += measurement.AudioSeconds
		report.InferenceDurationNS += measurement.WallClockDurationNS
		if measurement.MaxFeedDurationNS > report.MaxFeedDurationNS {
			report.MaxFeedDurationNS = measurement.MaxFeedDurationNS
		}

		chars := ocr.CER(utterance.Text, measurement.Text)
		charSubs += chars.Substitutions
		charDels += chars.Deletions
		charIns += chars.Insertions
		charRef += chars.ReferenceLength
		words := ocr.WER(utterance.Text, measurement.Text)
		wordSubs += words.Substitutions
		wordDels += words.Deletions
		wordIns += words.Insertions
		wordRef += words.ReferenceLength
		if utterance.Text == measurement.Text {
			exact++
		}
	}

	report.CER = editTotals(charRef, charSubs, charDels, charIns)
	report.WER = editTotals(wordRef, wordSubs, wordDels, wordIns)
	if report.Samples > 0 {
		report.ExactUtterances = float64(exact) / float64(report.Samples)
	}
	return report, nil
}
