package ocr_test

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/tasks/ocr"
	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

func inputRangeRecognizer(t *testing.T, profile string) *ocr.Recognizer {
	t.Helper()
	config := ocr.RecognizerConfig{
		Height:       2,
		Hidden:       4,
		Alphabet:     []rune{'a', 'b'},
		LearningRate: 0.03,
		Seed:         91,
		InputProfile: profile,
	}
	if profile == "single-character" {
		config.CharacterWidth = 3
	}
	r, err := ocr.NewRecognizer(config)
	if err != nil {
		t.Fatalf("NewRecognizer(%q): %v", profile, err)
	}
	return r
}

func inputRangeImage(replacement float64) glyphs.Image {
	pixels := []float64{
		0, 1, 0,
		0, 1, 0,
	}
	pixels[0] = replacement
	return glyphs.Image{Width: 3, Height: 2, Pixels: pixels}
}

func TestOCRImagePixelRangeAcceptsZeroAndOne(t *testing.T) {
	ctx := context.Background()
	image := inputRangeImage(0)

	blocks, err := ocr.SegmentLines(image, 0.5)
	if err != nil {
		t.Fatalf("SegmentLines([0,1] image): %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("SegmentLines([0,1] image) returned %d blocks, want 1", len(blocks))
	}

	for _, profile := range []string{"columns", "aligned-columns", "single-character"} {
		profile := profile
		t.Run(profile, func(t *testing.T) {
			r := inputRangeRecognizer(t, profile)
			if _, err := r.Inputs(image); err != nil {
				t.Fatalf("Inputs([0,1] image): %v", err)
			}
			if _, err := r.Columns(image); err != nil {
				t.Fatalf("Columns([0,1] image): %v", err)
			}
			report, err := r.TrainLine(ctx, image, "a")
			if err != nil {
				t.Fatalf("TrainLine([0,1] image): %v", err)
			}
			if report.Impossible {
				t.Fatalf("TrainLine([0,1] image) reported an impossible target: %+v", report)
			}
		})
	}
}

func TestOCRImagePixelRangeRejectsInvalidPixelsAtAllPublicEntries(t *testing.T) {
	invalid := []struct {
		name  string
		value float64
	}{
		{name: "below zero", value: -1},
		{name: "above one", value: 2},
		{name: "NaN", value: math.NaN()},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "negative infinity", value: math.Inf(-1)},
	}

	for _, tc := range invalid {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			image := inputRangeImage(tc.value)
			if _, err := ocr.SegmentLines(image, 0.5); err == nil {
				t.Error("SegmentLines accepted an invalid image pixel")
			}

			for _, profile := range []string{"columns", "aligned-columns", "single-character"} {
				profile := profile
				t.Run(profile, func(t *testing.T) {
					r := inputRangeRecognizer(t, profile)
					if _, err := r.Inputs(image); err == nil {
						t.Error("Inputs accepted an invalid image pixel")
					}
					if _, err := r.Columns(image); err == nil {
						t.Error("Columns accepted an invalid image pixel")
					}

					before := r.Snapshot()
					if _, err := r.TrainLine(context.Background(), image, "a"); err == nil {
						t.Error("TrainLine accepted an invalid image pixel")
					}
					if after := r.Snapshot(); !reflect.DeepEqual(before, after) {
						t.Error("TrainLine changed the snapshot after rejecting an invalid image pixel")
					}
				})
			}
		})
	}
}
