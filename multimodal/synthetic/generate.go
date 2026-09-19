// Package synthetic draws controllable multimodal fixtures for the TSK-10
// paired-modality evaluation: image (geometric shape), text (shape and colour
// tokens), audio (shape-named frequency), and position, plus a
// splitter that holds out (shape, colour) combinations for unseen-combination
// evaluation. Labels are evaluator-only ground truth and never enter the
// modalities of a Sample.
package synthetic

import (
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/TimLai666/coimnet/multimodal"
	"github.com/TimLai666/coimnet/signal"
)

const (
	// Shapes is the number of shape classes {0, 1, 2}.
	Shapes = 3
	// Colours is the number of colour classes {0, 1, 2}.
	Colours = 3
)

// ShapeNames is the display name of each shape index.
var ShapeNames = [Shapes]string{"square", "triangle", "circle"}

const (
	modalityImage    = "image"
	modalityText     = "text"
	modalityAudio    = "audio"
	modalityPosition = "position"
)

const (
	defaultSamples      = 60
	defaultImageSize    = 8
	defaultAudioSamples = 16
)

// Config controls one draw. Every field has a default when zero: Samples 60,
// AudioDropRate 0 (fraction of samples whose audio is missing), ImageSize 8,
// AudioSamples 16.
type Config struct {
	Samples       int     `json:"samples"`
	AudioDropRate float64 `json:"audio_drop_rate"`
	ImageSize     int     `json:"image_size"`
	AudioSamples  int     `json:"audio_samples"`
}

// Label is the evaluator-only ground truth of one sample; it is never part of
// a Sample's modalities.
type Label struct {
	Shape  int `json:"shape"`
	Colour int `json:"colour"`
}

func (c Config) defaults() Config {
	if c.Samples == 0 {
		c.Samples = defaultSamples
	}
	if c.ImageSize == 0 {
		c.ImageSize = defaultImageSize
	}
	if c.AudioSamples == 0 {
		c.AudioSamples = defaultAudioSamples
	}
	return c
}

func (c Config) validate() error {
	if c.Samples < 0 {
		return fmt.Errorf("samples must not be negative")
	}
	if math.IsNaN(c.AudioDropRate) || c.AudioDropRate < 0 || c.AudioDropRate > 1 {
		return fmt.Errorf("audio drop rate must be in [0,1]")
	}
	if c.ImageSize < 1 {
		return fmt.Errorf("image size must be positive")
	}
	if c.AudioSamples < 1 {
		return fmt.Errorf("audio samples must be positive")
	}
	return nil
}

// Generate draws Samples samples from rand.New(rand.NewPCG(seed, 0)):
//
//	image    shape {ImageSize, ImageSize}: the shape drawn as intensity
//	         (colour+1)/Colours on a zero background (square = filled centre
//	         box, triangle = filled lower-left triangle, circle = filled disc
//	         of radius ImageSize/3)
//	text     shape {Shapes+Colours}: one-hot of the shape then one-hot of the
//	         colour (tokens describing the sample)
//	audio    shape {AudioSamples}: sin(2π f t/AudioSamples) with f = 2+shape
//	         (frequency names the shape); missing (Present false, Values nil)
//	         for an AudioDropRate fraction chosen by the same RNG
//	position shape {2}: uniform in [0,1)²
//
// EntityID = fmt.Sprintf("%s-%d-%03d", shape name, colour, k) and EventID =
// "ev-" + EntityID; every present modality is timestamped at step k in
// signal.TimeUnitModelStep.
func Generate(seed uint64, c Config) ([]multimodal.Sample, []Label, error) {
	c = c.defaults()
	if err := c.validate(); err != nil {
		return nil, nil, err
	}
	rng := rand.New(rand.NewPCG(seed, 0))
	samples := make([]multimodal.Sample, 0, c.Samples)
	labels := make([]Label, 0, c.Samples)
	for k := 0; k < c.Samples; k++ {
		shape := rng.IntN(Shapes)
		colour := rng.IntN(Colours)
		dropAudio := rng.Float64() < c.AudioDropRate
		px, py := rng.Float64(), rng.Float64()

		image := multimodal.Modality{
			Present: true,
			Values:  imageValues(shape, colour, c.ImageSize),
			Shape:   []int{c.ImageSize, c.ImageSize},
		}
		text := multimodal.Modality{
			Present: true,
			Values:  textValues(shape, colour),
			Shape:   []int{Shapes + Colours},
		}
		audio := multimodal.Modality{
			Present: true,
			Values:  audioValues(shape, c.AudioSamples),
			Shape:   []int{c.AudioSamples},
		}
		position := multimodal.Modality{
			Present: true,
			Values:  []float64{px, py},
			Shape:   []int{2},
		}
		if dropAudio {
			audio.Present = false
			audio.Values = nil
		}

		step := signal.Timestamp{Value: int64(k), Unit: signal.TimeUnitModelStep}
		timestamps := map[string]signal.Timestamp{
			modalityImage:    step,
			modalityText:     step,
			modalityPosition: step,
		}
		if audio.Present {
			timestamps[modalityAudio] = step
		}

		entityID := fmt.Sprintf("%s-%d-%03d", ShapeNames[shape], colour, k)
		samples = append(samples, multimodal.Sample{
			EntityID: entityID,
			EventID:  "ev-" + entityID,
			Modalities: map[string]multimodal.Modality{
				modalityImage:    image,
				modalityText:     text,
				modalityAudio:    audio,
				modalityPosition: position,
			},
			Timestamps: timestamps,
		})
		labels = append(labels, Label{Shape: shape, Colour: colour})
	}
	return samples, labels, nil
}

// DefaultLayout is the layout Generate's samples fit: order image, text,
// audio, position, with widths ImageSize×ImageSize, Shapes+Colours,
// AudioSamples and 2.
func DefaultLayout(c Config) multimodal.Layout {
	c = c.defaults()
	return multimodal.Layout{
		Order: []string{modalityImage, modalityText, modalityAudio, modalityPosition},
		Widths: map[string]int{
			modalityImage:    c.ImageSize * c.ImageSize,
			modalityText:     Shapes + Colours,
			modalityAudio:    c.AudioSamples,
			modalityPosition: 2,
		},
	}
}

// imageValues draws the shape as intensity (colour+1)/Colours on a zero
// background, flattened row-major.
func imageValues(shape, colour, size int) []float64 {
	intensity := (float64(colour) + 1) / Colours
	out := make([]float64, size*size)
	for r := 0; r < size; r++ {
		for c := 0; c < size; c++ {
			if inShape(shape, r, c, size) {
				out[r*size+c] = intensity
			}
		}
	}
	return out
}

// inShape reports whether cell (r, c) of a size×size image is inside the
// shape: square = filled centre box, triangle = filled lower-left triangle,
// circle = filled disc of radius size/3 centred at cell (size/2, size/2).
func inShape(shape, r, c, size int) bool {
	switch shape {
	case 0: // square: filled centre box
		lo, hi := size/4, size-size/4
		return r >= lo && r < hi && c >= lo && c < hi
	case 1: // triangle: filled lower-left triangle
		return r+c >= size-1
	default: // circle: filled disc of radius size/3
		radius := size / 3
		dx, dy := r-size/2, c-size/2
		return dx*dx+dy*dy <= radius*radius
	}
}

// textValues is a one-hot of the shape followed by a one-hot of the colour.
func textValues(shape, colour int) []float64 {
	out := make([]float64, Shapes+Colours)
	out[shape] = 1
	out[Shapes+colour] = 1
	return out
}

// audioValues is sin(2π f t/AudioSamples) for t in [0, AudioSamples) with
// f = 2+shape, so the frequency names the shape.
func audioValues(shape, samples int) []float64 {
	freq := 2 + shape
	out := make([]float64, samples)
	for t := 0; t < samples; t++ {
		out[t] = math.Sin(2 * math.Pi * float64(freq) * float64(t) / float64(samples))
	}
	return out
}
