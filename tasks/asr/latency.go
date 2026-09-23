package asr

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// StreamMeasurement reports one successful offline, sequential streaming
// inference. The duration fields measure local chunk computation with Go's
// monotonic clock; they are not microphone-to-text transport latency and do
// not measure word-timing alignment precision.
type StreamMeasurement struct {
	Text                string  `json:"text"`
	SampleRate          int     `json:"sample_rate"`
	AudioSampleCount    int     `json:"audio_sample_count"`
	AudioSeconds        float64 `json:"audio_seconds"`
	ChunkSize           int     `json:"chunk_size"`
	ChunkCount          int     `json:"chunk_count"`
	WallClockDurationNS int64   `json:"wall_clock_duration_ns"`
	MaxFeedDurationNS   int64   `json:"max_feed_duration_ns"`
	FlushDurationNS     int64   `json:"flush_duration_ns"`
	FirstOutputSamples  int     `json:"first_output_samples"`
}

// MeasureStreaming runs samples through a fresh stream in sequential chunks,
// then flushes it, and returns the complete text and offline timing report.
// Feed and Flush are timed with time.Now/time.Since's monotonic clock. An
// error returns a zero report, even if earlier chunks completed successfully.
func (r *Recognizer) MeasureStreaming(ctx context.Context, samples []float64, chunk int) (StreamMeasurement, error) {
	var zero StreamMeasurement
	if r == nil || r.trainer == nil {
		return zero, errors.New("asr: nil recognizer")
	}
	if len(samples) == 0 {
		return zero, errors.New("asr: streaming samples are empty")
	}
	if chunk <= 0 {
		return zero, fmt.Errorf("asr: streaming chunk %d must be at least one sample", chunk)
	}

	started := time.Now()
	s, err := r.NewStream()
	if err != nil {
		return zero, err
	}

	report := StreamMeasurement{
		SampleRate:       r.config.SampleRate,
		AudioSampleCount: len(samples),
		AudioSeconds:     float64(len(samples)) / float64(r.config.SampleRate),
		ChunkSize:        chunk,
	}
	firstOutputRecorded := false
	for chunkIndex, start := 0, 0; start < len(samples); chunkIndex, start = chunkIndex+1, start+chunk {
		end := start + chunk
		if end > len(samples) {
			end = len(samples)
		}

		feedStarted := time.Now()
		text, err := s.Feed(ctx, samples[start:end])
		feedNS := elapsedNanoseconds(feedStarted)
		if feedNS > report.MaxFeedDurationNS {
			report.MaxFeedDurationNS = feedNS
		}
		if err != nil {
			return zero, fmt.Errorf("asr: streaming chunk %d: %w", chunkIndex, err)
		}
		report.ChunkCount++
		if text != "" && !firstOutputRecorded {
			report.FirstOutputSamples = end
			firstOutputRecorded = true
		}
	}

	flushStarted := time.Now()
	text, err := s.Flush(ctx)
	report.FlushDurationNS = elapsedNanoseconds(flushStarted)
	if err != nil {
		return zero, fmt.Errorf("asr: streaming flush after chunk %d: %w", report.ChunkCount-1, err)
	}
	report.WallClockDurationNS = elapsedNanoseconds(started)
	if text != "" && !firstOutputRecorded {
		report.FirstOutputSamples = len(samples)
	}
	report.Text = s.Text()
	return report, nil
}

func elapsedNanoseconds(start time.Time) int64 {
	d := time.Since(start)
	if d < 0 {
		return 0
	}
	return d.Nanoseconds()
}
