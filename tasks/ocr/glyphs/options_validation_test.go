package glyphs_test

import (
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

func TestRenderOptionsRejectInvalidValues(t *testing.T) {
	fields := []struct {
		name string
		set  func(*glyphs.Options, float64)
	}{
		{name: "Background", set: func(o *glyphs.Options, value float64) { o.Background = value }},
		{name: "Noise", set: func(o *glyphs.Options, value float64) { o.Noise = value }},
	}
	values := []struct {
		name  string
		value float64
	}{
		{name: "negative", value: -0.01},
		{name: "above-half", value: 0.5001},
		{name: "NaN", value: math.NaN()},
		{name: "positive-infinity", value: math.Inf(1)},
		{name: "negative-infinity", value: math.Inf(-1)},
	}

	for _, field := range fields {
		for _, value := range values {
			t.Run(field.name+"/"+value.name, func(t *testing.T) {
				o := glyphs.Options{}
				field.set(&o, value.value)
				img, boxes, err := glyphs.Render("a", glyphs.Family{Seed: 1}, o)
				if err == nil {
					t.Fatalf("Render accepted %s=%v", field.name, value.value)
				}
				if !reflect.DeepEqual(img, (glyphs.Image{})) {
					t.Fatalf("Render returned non-zero image for %s=%v: %+v", field.name, value.value, img)
				}
				if boxes != nil {
					t.Fatalf("Render returned boxes for %s=%v: %+v", field.name, value.value, boxes)
				}
			})
		}
	}
}

func TestRenderOptionsAcceptBoundaries(t *testing.T) {
	for _, background := range []float64{0, 0.5} {
		for _, noise := range []float64{0, 0.5} {
			t.Run(fmt.Sprintf("background=%g/noise=%g", background, noise), func(t *testing.T) {
				o := glyphs.Options{Background: background, Noise: noise, Seed: 3}
				img, boxes, err := glyphs.Render("ab", glyphs.Family{Seed: 1}, o)
				if err != nil {
					t.Fatalf("Render rejected boundary options: %v", err)
				}
				if len(boxes) != 2 {
					t.Fatalf("Render returned %d boxes, want 2", len(boxes))
				}
				assertFinitePixels(t, img)
			})
		}
	}
}

func TestGenerateOptionsRejectInvalidValues(t *testing.T) {
	fields := []struct {
		name string
		set  func(*glyphs.Options, float64)
	}{
		{name: "Background", set: func(o *glyphs.Options, value float64) { o.Background = value }},
		{name: "Noise", set: func(o *glyphs.Options, value float64) { o.Noise = value }},
	}
	values := []struct {
		name  string
		value float64
	}{
		{name: "negative", value: -0.01},
		{name: "above-half", value: 0.5001},
		{name: "NaN", value: math.NaN()},
		{name: "positive-infinity", value: math.Inf(1)},
		{name: "negative-infinity", value: math.Inf(-1)},
	}

	for _, field := range fields {
		for _, value := range values {
			t.Run(field.name+"/"+value.name, func(t *testing.T) {
				o := glyphs.Options{}
				field.set(&o, value.value)
				samples, err := glyphs.Generate([]rune{'a', 'b'}, 1, 1, 1, glyphs.Family{Seed: 1}, o, 9)
				if err == nil {
					t.Fatalf("Generate accepted %s=%v", field.name, value.value)
				}
				if samples != nil {
					t.Fatalf("Generate returned %d samples for %s=%v", len(samples), field.name, value.value)
				}
			})
		}
	}
}

func TestGenerateOptionsAcceptBoundaries(t *testing.T) {
	for _, background := range []float64{0, 0.5} {
		for _, noise := range []float64{0, 0.5} {
			t.Run(fmt.Sprintf("background=%g/noise=%g", background, noise), func(t *testing.T) {
				o := glyphs.Options{Background: background, Noise: noise, Seed: 3}
				samples, err := glyphs.Generate([]rune{'a', 'b'}, 1, 1, 1, glyphs.Family{Seed: 1}, o, 9)
				if err != nil {
					t.Fatalf("Generate rejected boundary options: %v", err)
				}
				if len(samples) != 1 {
					t.Fatalf("Generate returned %d samples, want 1", len(samples))
				}
				if len([]rune(samples[0].Text)) != 1 {
					t.Fatalf("generated text %q has length %d, want 1", samples[0].Text, len([]rune(samples[0].Text)))
				}
				if len(samples[0].Boxes) != 1 {
					t.Fatalf("generated sample has %d boxes, want 1", len(samples[0].Boxes))
				}
				assertFinitePixels(t, samples[0].Image)
			})
		}
	}
}

func TestGenerateOptionsRejectInvalidEmptyBatch(t *testing.T) {
	for name, options := range map[string]glyphs.Options{
		"background negative": {Background: -0.01},
		"background NaN":      {Background: math.NaN()},
		"noise above half":    {Noise: 0.5001},
		"noise NaN":           {Noise: math.NaN()},
	} {
		t.Run(name, func(t *testing.T) {
			samples, err := glyphs.Generate([]rune{'a'}, 0, 1, 1, glyphs.Family{}, options, 1)
			if err == nil || samples != nil {
				t.Fatalf("invalid empty batch returned samples=%v err=%v", samples, err)
			}
		})
	}
}

func assertFinitePixels(t *testing.T, img glyphs.Image) {
	t.Helper()
	if img.Width <= 0 || img.Height <= 0 {
		t.Fatalf("image size %dx%d, want positive dimensions", img.Width, img.Height)
	}
	if len(img.Pixels) != img.Width*img.Height {
		t.Fatalf("image has %d pixels for %dx%d", len(img.Pixels), img.Width, img.Height)
	}
	for i, pixel := range img.Pixels {
		if math.IsNaN(pixel) || math.IsInf(pixel, 0) || pixel < 0 || pixel > 1 {
			t.Fatalf("pixel %d = %v outside finite [0,1]", i, pixel)
		}
	}
}
