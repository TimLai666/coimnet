package media

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"

	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

// WritePNG writes 8-bit RGB pixels to a new PNG path and returns the file's
// SHA-256 digest. Existing paths are refused.
func WritePNG(path string, pixels []float64) (string, error) {
	if len(pixels) != ImageSize*ImageSize*3 {
		return "", fmt.Errorf("media: image has %d values, want %d", len(pixels), ImageSize*ImageSize*3)
	}
	img := image.NewRGBA(image.Rect(0, 0, ImageSize, ImageSize))
	for pixel := 0; pixel < ImageSize*ImageSize; pixel++ {
		var rgb [3]uint8
		for channel := range rgb {
			i := pixel*3 + channel
			value := pixels[i]
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return "", fmt.Errorf("media: image value %d is non-finite", i)
			}
			value = math.Max(0, math.Min(1, value))
			rgb[channel] = uint8(math.Round(value * 255))
		}
		img.SetRGBA(pixel%ImageSize, pixel/ImageSize, color.RGBA{R: rgb[0], G: rgb[1], B: rgb[2], A: 255})
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	encodeErr := png.Encode(io.MultiWriter(f, h), img)
	closeErr := f.Close()
	if encodeErr != nil || closeErr != nil {
		_ = os.Remove(path)
		if encodeErr != nil {
			return "", fmt.Errorf("media: encode PNG: %w", encodeErr)
		}
		return "", fmt.Errorf("media: close PNG: %w", closeErr)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ReadPNG decodes an image file to row-major RGB values in [0,1].
func ReadPNG(path string) ([]float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("media: decode PNG: %w", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != ImageSize || bounds.Dy() != ImageSize {
		return nil, fmt.Errorf("media: PNG dimensions are %dx%d, want %dx%d", bounds.Dx(), bounds.Dy(), ImageSize, ImageSize)
	}
	pixels := make([]float64, ImageSize*ImageSize*3)
	for y := 0; y < ImageSize; y++ {
		for x := 0; x < ImageSize; x++ {
			r, g, b, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			base := (y*ImageSize + x) * 3
			pixels[base] = float64(r) / 65535
			pixels[base+1] = float64(g) / 65535
			pixels[base+2] = float64(b) / 65535
		}
	}
	return pixels, nil
}

// WriteBlockWAV writes mono PCM16 blocks at 8000 Hz to a new WAV path and
// returns the file's SHA-256 digest. The sample count must be block-aligned.
func WriteBlockWAV(path string, samples []float64) (string, error) {
	if len(samples)%AudioBlock != 0 {
		return "", fmt.Errorf("media: audio has %d samples, not a multiple of block size %d", len(samples), AudioBlock)
	}
	if uint64(len(samples))*2 > uint64(^uint32(0))-36 {
		return "", fmt.Errorf("media: audio has %d samples, exceeding the WAV size limit", len(samples))
	}
	for i, sample := range samples {
		if math.IsNaN(sample) || math.IsInf(sample, 0) {
			return "", fmt.Errorf("media: audio sample %d is non-finite", i)
		}
	}
	if err := audio.WriteWAV(path, audio.Signal{SampleRate: AudioSampleRate, Channels: 1, Samples: [][]float64{samples}}); err != nil {
		return "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("media: read written WAV: %w", err)
	}
	info, err := audio.InspectWAV(raw)
	if err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("media: inspect written WAV: %w", err)
	}
	if info.Frames != len(samples) || info.SampleRate != AudioSampleRate || info.Channels != 1 {
		_ = os.Remove(path)
		return "", fmt.Errorf("media: written WAV has %d frames at %d Hz with %d channels", info.Frames, info.SampleRate, info.Channels)
	}
	if _, err := audio.DecodeWAV(raw); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("media: decode written WAV: %w", err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}
