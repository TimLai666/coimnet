package asr_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/tasks/asr"
)

func TestMeasureStreaming(t *testing.T) {
	ctx := context.Background()
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}

	base := make([]float64, 256+128*2)
	for i := range base {
		base[i] = 0.2 * math.Sin(float64(i)*0.07)
	}
	tests := []struct {
		name    string
		samples []float64
		chunk   int
	}{
		{name: "short tail", samples: append(append([]float64(nil), base...), 0.1), chunk: 64},
		{name: "single chunk", samples: base, chunk: len(base)},
		{name: "multiple chunks", samples: base, chunk: 73},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			want, err := r.TranscribeStreaming(ctx, tc.samples, tc.chunk)
			if err != nil {
				t.Fatalf("TranscribeStreaming: %v", err)
			}
			got, err := r.MeasureStreaming(ctx, tc.samples, tc.chunk)
			if err != nil {
				t.Fatalf("MeasureStreaming: %v", err)
			}
			wantFirstOutput, err := firstStreamingOutputSamples(ctx, r, tc.samples, tc.chunk)
			if err != nil {
				t.Fatalf("manual first-output check: %v", err)
			}
			if got.Text != want {
				t.Fatalf("measured text %q, TranscribeStreaming text %q", got.Text, want)
			}
			if got.SampleRate != recognizerConfig().SampleRate {
				t.Fatalf("sample rate %d, want %d", got.SampleRate, recognizerConfig().SampleRate)
			}
			if got.AudioSampleCount != len(tc.samples) {
				t.Fatalf("audio sample count %d, want %d", got.AudioSampleCount, len(tc.samples))
			}
			wantSeconds := float64(len(tc.samples)) / float64(got.SampleRate)
			if got.AudioSeconds != wantSeconds {
				t.Fatalf("audio seconds %.12g, want %.12g", got.AudioSeconds, wantSeconds)
			}
			if got.ChunkSize != tc.chunk {
				t.Fatalf("chunk size %d, want %d", got.ChunkSize, tc.chunk)
			}
			wantChunks := (len(tc.samples) + tc.chunk - 1) / tc.chunk
			if got.ChunkCount != wantChunks {
				t.Fatalf("chunk count %d, want %d", got.ChunkCount, wantChunks)
			}
			if got.WallClockDurationNS < 0 || got.MaxFeedDurationNS < 0 || got.FlushDurationNS < 0 {
				t.Fatalf("negative duration in %+v", got)
			}
			if got.FirstOutputSamples != wantFirstOutput {
				t.Fatalf("first output sample count %d, want %d", got.FirstOutputSamples, wantFirstOutput)
			}

			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("Marshal measurement: %v", err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatalf("Unmarshal measurement: %v", err)
			}
			for _, name := range []string{
				"text", "sample_rate", "audio_sample_count", "audio_seconds",
				"chunk_size", "chunk_count", "wall_clock_duration_ns",
				"max_feed_duration_ns", "flush_duration_ns", "first_output_samples",
			} {
				if _, ok := fields[name]; !ok {
					t.Errorf("JSON field %q is missing from %s", name, encoded)
				}
			}
		})
	}
}

func firstStreamingOutputSamples(ctx context.Context, r *asr.Recognizer, samples []float64, chunk int) (int, error) {
	s, err := r.NewStream()
	if err != nil {
		return 0, err
	}
	for start := 0; start < len(samples); start += chunk {
		end := start + chunk
		if end > len(samples) {
			end = len(samples)
		}
		text, err := s.Feed(ctx, samples[start:end])
		if err != nil {
			return 0, err
		}
		if text != "" {
			return end, nil
		}
	}
	text, err := s.Flush(ctx)
	if err != nil {
		return 0, err
	}
	if text != "" {
		return len(samples), nil
	}
	return 0, nil
}

func TestMeasureStreamingErrors(t *testing.T) {
	ctx := context.Background()
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}

	tests := []struct {
		name  string
		r     *asr.Recognizer
		x     []float64
		chunk int
	}{
		{name: "nil recognizer", r: nil, x: []float64{0}, chunk: 1},
		{name: "empty samples", r: r, x: nil, chunk: 1},
		{name: "zero chunk", r: r, x: []float64{0}, chunk: 0},
		{name: "negative chunk", r: r, x: []float64{0}, chunk: -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.r.MeasureStreaming(ctx, tc.x, tc.chunk)
			if err == nil {
				t.Fatal("MeasureStreaming accepted invalid input")
			}
			if got != (asr.StreamMeasurement{}) {
				t.Fatalf("error returned partial report %+v", got)
			}
		})
	}

	bad := make([]float64, 256+128)
	bad[128] = math.NaN()
	got, err := r.MeasureStreaming(ctx, bad, 128)
	if err == nil {
		t.Fatal("MeasureStreaming accepted a non-finite sample")
	}
	if !strings.Contains(err.Error(), "chunk 1") {
		t.Fatalf("non-finite sample error %q does not identify chunk 1", err)
	}
	if got != (asr.StreamMeasurement{}) {
		t.Fatalf("non-finite sample returned partial report %+v", got)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	got, err = r.MeasureStreaming(cancelled, make([]float64, 256), 128)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled measurement error %v, want context.Canceled", err)
	}
	if !strings.Contains(err.Error(), "chunk 0") {
		t.Fatalf("cancelled measurement error %q does not identify chunk 0", err)
	}
	if got != (asr.StreamMeasurement{}) {
		t.Fatalf("cancelled measurement returned partial report %+v", got)
	}
}
