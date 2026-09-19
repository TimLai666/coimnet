// Package audio is the ASR data layer: it reads and writes uncompressed WAV
// PCM and applies the fixed preprocessing steps — mixdown, resampling and peak
// normalisation — that the recogniser expects. Compressed containers are out
// of scope: they only ever reach a Signal through an explicit decoder.
package audio

import (
	"errors"
	"fmt"
	"math"
)

// Signal is PCM audio decoded to float64 samples in [-1, 1], one slice per
// channel, all the same length.
type Signal struct {
	SampleRate int         `json:"sample_rate"`
	Channels   int         `json:"channels"`
	Samples    [][]float64 `json:"-"`
}

// ResampleReport records how one rate conversion was performed, so a run
// report can declare the method instead of leaving it implied.
type ResampleReport struct {
	Method       string `json:"method"`
	From         int    `json:"from"`
	To           int    `json:"to"`
	InputLength  int    `json:"input_length"`
	OutputLength int    `json:"output_length"`
}

// NormalizeReport records one peak normalisation. Peak is the largest absolute
// value found in the input, before any scaling; Gain is the factor applied, so
// the requested target peak is Peak*Gain. Scaled is false when the input held
// no non-zero sample and was left alone.
type NormalizeReport struct {
	Peak   float64 `json:"peak"`
	Gain   float64 `json:"gain"`
	Scaled bool    `json:"scaled"`
}

// Mixdown averages the channels into one. A signal with no channels yields an
// empty slice; should the channels be ragged despite the Signal invariant, the
// shortest one sets the length rather than panicking.
func Mixdown(s Signal) []float64 {
	if len(s.Samples) == 0 {
		return []float64{}
	}
	frames := len(s.Samples[0])
	for _, ch := range s.Samples[1:] {
		if len(ch) < frames {
			frames = len(ch)
		}
	}
	out := make([]float64, frames)
	for i := range out {
		sum := 0.0
		for _, ch := range s.Samples {
			sum += ch[i]
		}
		out[i] = sum / float64(len(s.Samples))
	}
	return out
}

// Resample converts x from `from` Hz to `to` Hz by linear interpolation and
// reports the method; the output length is round(len(x)*to/from) and output
// sample i reads input position i*from/to, clamped to the last input sample.
// Errors: non-positive rates, empty x.
func Resample(x []float64, from, to int) ([]float64, ResampleReport, error) {
	if from <= 0 || to <= 0 {
		return nil, ResampleReport{}, fmt.Errorf("audio: resample rates must be positive, got %d Hz to %d Hz", from, to)
	}
	if len(x) == 0 {
		return nil, ResampleReport{}, errors.New("audio: resample input is empty")
	}
	n := int(math.Round(float64(len(x)) * float64(to) / float64(from)))
	out := make([]float64, n)
	step := float64(from) / float64(to)
	last := len(x) - 1
	for i := range out {
		pos := float64(i) * step
		i0 := int(pos)
		if i0 >= last {
			out[i] = x[last]
			continue
		}
		// x[i0] + frac*(x[i0+1]-x[i0]) keeps a constant input exactly
		// constant, which x[i0]*(1-frac) + x[i0+1]*frac does not.
		out[i] = x[i0] + (pos-float64(i0))*(x[i0+1]-x[i0])
	}
	report := ResampleReport{
		Method:       "linear",
		From:         from,
		To:           to,
		InputLength:  len(x),
		OutputLength: n,
	}
	return out, report, nil
}

// Normalize scales x so its peak absolute value equals peak (0 < peak <= 1);
// an all-zero x is returned unchanged with Scaled false. The result is always
// a fresh slice, so x is never modified.
func Normalize(x []float64, peak float64) ([]float64, NormalizeReport, error) {
	if !(peak > 0 && peak <= 1) {
		return nil, NormalizeReport{}, fmt.Errorf("audio: normalise peak %v outside (0, 1]", peak)
	}
	measured := 0.0
	for _, v := range x {
		if a := math.Abs(v); a > measured {
			measured = a
		}
	}
	out := make([]float64, len(x))
	copy(out, x)
	report := NormalizeReport{Peak: measured, Gain: 1}
	if measured == 0 {
		return out, report, nil
	}
	gain := peak / measured
	for i := range out {
		out[i] *= gain
	}
	report.Gain = gain
	report.Scaled = true
	return out, report, nil
}
