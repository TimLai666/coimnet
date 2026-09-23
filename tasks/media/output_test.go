package media

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

func TestPNGRoundTrip(t *testing.T) {
	pixels, err := RenderImage(ImageCondition{Colour: "blue", Shape: "cross"})
	if err != nil {
		t.Fatal(err)
	}
	pixels[0], pixels[1], pixels[2] = 0.5, -0.2, 1.2
	path := filepath.Join(t.TempDir(), "fixture.png")
	hash, err := WritePNG(path, pixels)
	if err != nil {
		t.Fatalf("WritePNG: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 26 {
		t.Fatalf("PNG has %d bytes, too short for IHDR", len(raw))
	}
	if raw[24] != 8 || raw[25] != 2 {
		t.Fatalf("PNG IHDR bit depth/type = %v, want 8-bit RGB", raw[24:26])
	}
	digest := sha256.Sum256(raw)
	if hash != hex.EncodeToString(digest[:]) {
		t.Errorf("WritePNG hash = %q, want hash of output bytes %x", hash, digest)
	}
	got, err := ReadPNG(path)
	if err != nil {
		t.Fatalf("ReadPNG: %v", err)
	}
	for i, value := range pixels {
		value = math.Max(0, math.Min(1, value))
		want := math.Round(255*value) / 255
		if got[i] != want {
			t.Errorf("ReadPNG()[%d] = %.17g, want %.17g", i, got[i], want)
		}
	}
	if _, err := WritePNG(path, pixels); err == nil {
		t.Error("WritePNG over an existing path = nil, want error")
	}
	if _, err := WritePNG(filepath.Join(t.TempDir(), "short.png"), pixels[:len(pixels)-1]); err == nil {
		t.Error("WritePNG with 191 values = nil, want error")
	}
	secondPath := filepath.Join(t.TempDir(), "same.png")
	secondHash, err := WritePNG(secondPath, pixels)
	if err != nil || secondHash != hash {
		t.Errorf("same PNG hash = %q, %v; want %q", secondHash, err, hash)
	}
}

func TestWAVExactLength(t *testing.T) {
	samples := make([]float64, 3*AudioBlock)
	for n := range samples {
		samples[n] = 0.7 * math.Sin(2*math.Pi*1000*float64(n)/AudioSampleRate)
	}
	path := filepath.Join(t.TempDir(), "fixture.wav")
	hash, err := WriteBlockWAV(path, samples)
	if err != nil {
		t.Fatalf("WriteBlockWAV: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if hash != hex.EncodeToString(digest[:]) {
		t.Errorf("WriteBlockWAV hash = %q, want hash of output bytes %x", hash, digest)
	}
	info, err := audio.InspectWAV(raw)
	if err != nil {
		t.Fatalf("InspectWAV: %v", err)
	}
	decoded, err := audio.DecodeWAV(raw)
	if err != nil {
		t.Fatalf("DecodeWAV: %v", err)
	}
	if info.Frames != 3*AudioBlock || info.SampleRate != AudioSampleRate || info.Channels != 1 {
		t.Errorf("InspectWAV = %+v, want 768 frames at 8000 Hz mono", info)
	}
	if decoded.SampleRate != AudioSampleRate || decoded.Channels != 1 || len(decoded.Samples) != 1 || len(decoded.Samples[0]) != len(samples) {
		t.Fatalf("DecodeWAV shape = rate %d, channels %d, samples %d; want 8000 Hz, mono, 768", decoded.SampleRate, decoded.Channels, len(decoded.Samples[0]))
	}
	for i, value := range samples {
		want := math.Round(value*32767) / 32767
		if math.Abs(decoded.Samples[0][i]-want) > 1.0/32767 {
			t.Errorf("decoded sample %d = %.17g, expected PCM16 within tolerance of %.17g", i, decoded.Samples[0][i], want)
		}
	}
	if _, err := WriteBlockWAV(path, samples); err == nil {
		t.Error("WriteBlockWAV over an existing path = nil, want error")
	}
	if _, err := WriteBlockWAV(filepath.Join(t.TempDir(), "partial.wav"), samples[:len(samples)-1]); err == nil {
		t.Error("WriteBlockWAV with a partial block = nil, want error")
	}
}
