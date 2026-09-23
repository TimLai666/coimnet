// Package media provides deterministic image and audio fixtures for text-conditioned generation.
package media

import (
	"fmt"
	"math"
	"strings"
)

const (
	// ImageSize is the number of pixels on each side of a fixture image.
	ImageSize = 8
	// AudioSampleRate is the fixture audio sample rate in hertz.
	AudioSampleRate = 8000
	// AudioBlock is the number of samples in one fixture audio block.
	AudioBlock = 256
)

// ImageColours lists the supported image colour conditions.
var ImageColours = []string{"red", "green", "blue"}

// ImageShapes lists the supported image shape conditions.
var ImageShapes = []string{"square", "cross", "diagonal"}

// AudioPitches lists the supported audio pitch conditions.
var AudioPitches = []string{"low", "mid", "high"}

// AudioPatterns lists the supported audio envelope conditions.
var AudioPatterns = []string{"steady", "fade", "pulse"}

// ImageCondition names one fixture image condition.
type ImageCondition struct {
	Colour string `json:"colour"`
	Shape  string `json:"shape"`
}

// AudioCondition names one fixture audio condition.
type AudioCondition struct {
	Pitch   string `json:"pitch"`
	Pattern string `json:"pattern"`
}

// Prompt returns the image condition as "<colour> <shape>".
func (c ImageCondition) Prompt() string {
	return c.Colour + " " + c.Shape
}

// Prompt returns the audio condition as "<pitch> <pattern>".
func (c AudioCondition) Prompt() string {
	return c.Pitch + " " + c.Pattern
}

// ParseImagePrompt parses an exact supported "<colour> <shape>" prompt.
func ParseImagePrompt(p string) (ImageCondition, error) {
	parts := strings.Split(p, " ")
	if len(parts) != 2 {
		return ImageCondition{}, fmt.Errorf("media: invalid image prompt %q", p)
	}
	c := ImageCondition{Colour: parts[0], Shape: parts[1]}
	if !contains(ImageColours, c.Colour) || !contains(ImageShapes, c.Shape) {
		return ImageCondition{}, fmt.Errorf("media: unsupported image prompt %q", p)
	}
	return c, nil
}

// ParseAudioPrompt parses an exact supported "<pitch> <pattern>" prompt.
func ParseAudioPrompt(p string) (AudioCondition, error) {
	parts := strings.Split(p, " ")
	if len(parts) != 2 {
		return AudioCondition{}, fmt.Errorf("media: invalid audio prompt %q", p)
	}
	c := AudioCondition{Pitch: parts[0], Pattern: parts[1]}
	if !contains(AudioPitches, c.Pitch) || !contains(AudioPatterns, c.Pattern) {
		return AudioCondition{}, fmt.Errorf("media: unsupported audio prompt %q", p)
	}
	return c, nil
}

// RenderImage returns a row-major 8×8 RGB image with channel-last values.
// A square occupies rows and columns 2 through 5; a cross uses rows and
// columns 3 and 4; a diagonal uses the main diagonal and the diagonal above it.
func RenderImage(c ImageCondition) ([]float64, error) {
	if !contains(ImageColours, c.Colour) || !contains(ImageShapes, c.Shape) {
		return nil, fmt.Errorf("media: unsupported image condition %q", c.Prompt())
	}
	channel := 0
	switch c.Colour {
	case "green":
		channel = 1
	case "blue":
		channel = 2
	}
	pixels := make([]float64, ImageSize*ImageSize*3)
	for r := 0; r < ImageSize; r++ {
		for col := 0; col < ImageSize; col++ {
			if imageMask(c.Shape, r, col) {
				pixels[(r*ImageSize+col)*3+channel] = 1
			}
		}
	}
	return pixels, nil
}

// RenderAudio returns one 256-sample sine block at 8000 Hz with the requested
// pitch and steady, fade, or pulse envelope.
func RenderAudio(c AudioCondition) ([]float64, error) {
	if !contains(AudioPitches, c.Pitch) || !contains(AudioPatterns, c.Pattern) {
		return nil, fmt.Errorf("media: unsupported audio condition %q", c.Prompt())
	}
	frequency := map[string]float64{"low": 500, "mid": 1000, "high": 2000}[c.Pitch]
	samples := make([]float64, AudioBlock)
	for n := range samples {
		envelope := audioEnvelope(c.Pattern, n)
		samples[n] = 0.8 * math.Sin(2*math.Pi*frequency*float64(n)/AudioSampleRate) * envelope
	}
	return samples, nil
}

// ClassifyImage independently estimates the image colour and shape. Confidence
// is the winning cosine correlation between the per-pixel maximum and a shape mask.
func ClassifyImage(pixels []float64) (ImageCondition, float64, error) {
	if len(pixels) != ImageSize*ImageSize*3 {
		return ImageCondition{}, 0, fmt.Errorf("media: image has %d values, want %d", len(pixels), ImageSize*ImageSize*3)
	}
	var totals [3]float64
	intensity := make([]float64, ImageSize*ImageSize)
	for i := range intensity {
		base := i * 3
		for channel := 0; channel < 3; channel++ {
			v := pixels[base+channel]
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return ImageCondition{}, 0, fmt.Errorf("media: image value %d is non-finite", base+channel)
			}
			totals[channel] += v
			if v > intensity[i] || channel == 0 {
				intensity[i] = v
			}
		}
	}
	channel := argMax3(totals)
	shape, confidence := bestImageShape(intensity)
	if confidence == 0 {
		return ImageCondition{}, 0, nil
	}
	return ImageCondition{Colour: ImageColours[channel], Shape: shape}, confidence, nil
}

// ClassifyAudio independently estimates the pitch by DFT magnitude and the
// envelope by cosine correlation with the 32-sample RMS profile.
func ClassifyAudio(samples []float64) (AudioCondition, float64, error) {
	if len(samples) != AudioBlock {
		return AudioCondition{}, 0, fmt.Errorf("media: audio has %d samples, want %d", len(samples), AudioBlock)
	}
	for i, v := range samples {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return AudioCondition{}, 0, fmt.Errorf("media: audio sample %d is non-finite", i)
		}
	}
	frequencies := []float64{500, 1000, 2000}
	magnitudes := [3]float64{}
	energy := 0.0
	for _, sample := range samples {
		energy += sample * sample
	}
	if energy == 0 {
		return AudioCondition{}, 0, nil
	}
	for i, frequency := range frequencies {
		real, imag := 0.0, 0.0
		for n, sample := range samples {
			angle := 2 * math.Pi * frequency * float64(n) / AudioSampleRate
			real += sample * math.Cos(angle)
			imag -= sample * math.Sin(angle)
		}
		magnitudes[i] = math.Hypot(real, imag)
	}
	pitch := AudioPitches[argMax3(magnitudes)]
	profile := make([]float64, AudioBlock/32)
	for window := range profile {
		energy := 0.0
		for _, sample := range samples[window*32 : (window+1)*32] {
			energy += sample * sample
		}
		profile[window] = math.Sqrt(energy / 32)
	}
	pattern, confidence := bestAudioPattern(profile)
	return AudioCondition{Pitch: pitch, Pattern: pattern}, confidence, nil
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func imageMask(shape string, r, c int) bool {
	switch shape {
	case "square":
		return r >= 2 && r <= 5 && c >= 2 && c <= 5
	case "cross":
		return r == 3 || r == 4 || c == 3 || c == 4
	case "diagonal":
		return r == c || c == r+1
	default:
		return false
	}
}

func audioEnvelope(pattern string, n int) float64 {
	switch pattern {
	case "fade":
		return 1 - float64(n)/AudioBlock
	case "pulse":
		if n%64 >= 32 {
			return 0
		}
	}
	return 1
}

func argMax3(values [3]float64) int {
	best := 0
	for i := 1; i < len(values); i++ {
		if values[i] > values[best] {
			best = i
		}
	}
	return best
}

func bestImageShape(intensity []float64) (string, float64) {
	bestShape, bestScore := "", 0.0
	for _, shape := range ImageShapes {
		dot, norm := 0.0, 0.0
		for i, value := range intensity {
			mask := 0.0
			if imageMask(shape, i/ImageSize, i%ImageSize) {
				mask = 1
			}
			dot += value * mask
			norm += value * value
		}
		if norm > 0 {
			score := dot / math.Sqrt(norm*float64(maskArea(shape)))
			if score > bestScore {
				bestShape, bestScore = shape, score
			}
		}
	}
	return bestShape, bestScore
}

func maskArea(shape string) int {
	area := 0
	for r := 0; r < ImageSize; r++ {
		for c := 0; c < ImageSize; c++ {
			if imageMask(shape, r, c) {
				area++
			}
		}
	}
	return area
}

func bestAudioPattern(profile []float64) (string, float64) {
	bestPattern, bestScore := "", 0.0
	for _, pattern := range AudioPatterns {
		dot, inputNorm, templateNorm := 0.0, 0.0, 0.0
		for i, value := range profile {
			windowEnergy := 0.0
			for n := i * 32; n < (i+1)*32; n++ {
				envelope := audioEnvelope(pattern, n)
				windowEnergy += envelope * envelope
			}
			template := math.Sqrt(windowEnergy / 32)
			dot += value * template
			inputNorm += value * value
			templateNorm += template * template
		}
		if inputNorm > 0 && templateNorm > 0 {
			score := dot / math.Sqrt(inputNorm*templateNorm)
			if score > bestScore {
				bestPattern, bestScore = pattern, score
			}
		}
	}
	return bestPattern, bestScore
}
