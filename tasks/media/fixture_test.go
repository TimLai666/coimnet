package media

import (
	"math"
	"math/rand"
	"testing"
)

func TestConditionsAndPrompts(t *testing.T) {
	for _, colour := range ImageColours {
		for _, shape := range ImageShapes {
			condition := ImageCondition{Colour: colour, Shape: shape}
			parsed, err := ParseImagePrompt(condition.Prompt())
			if err != nil || parsed != condition {
				t.Errorf("ParseImagePrompt(%q) = %+v, %v; want %+v", condition.Prompt(), parsed, err, condition)
			}
		}
	}
	for _, pitch := range AudioPitches {
		for _, pattern := range AudioPatterns {
			condition := AudioCondition{Pitch: pitch, Pattern: pattern}
			parsed, err := ParseAudioPrompt(condition.Prompt())
			if err != nil || parsed != condition {
				t.Errorf("ParseAudioPrompt(%q) = %+v, %v; want %+v", condition.Prompt(), parsed, err, condition)
			}
		}
	}
	for _, prompt := range []string{"", "red  square", " red square", "red square ", "orange square", "red triangle"} {
		if _, err := ParseImagePrompt(prompt); err == nil {
			t.Errorf("ParseImagePrompt(%q) = nil error, want error", prompt)
		}
	}
	for _, prompt := range []string{"", "low  fade", " low fade", "low fade ", "ultra fade", "low echo"} {
		if _, err := ParseAudioPrompt(prompt); err == nil {
			t.Errorf("ParseAudioPrompt(%q) = nil error, want error", prompt)
		}
	}
}

func TestRenderAndClassifyAgree(t *testing.T) {
	for _, colour := range ImageColours {
		for _, shape := range ImageShapes {
			want := ImageCondition{Colour: colour, Shape: shape}
			pixels, err := RenderImage(want)
			if err != nil {
				t.Fatalf("RenderImage(%+v): %v", want, err)
			}
			got, confidence, err := ClassifyImage(pixels)
			if err != nil || got != want || confidence <= 0 {
				t.Errorf("ClassifyImage(%+v) = %+v, %v, %v", want, got, confidence, err)
			}
		}
	}
	for _, pitch := range AudioPitches {
		for _, pattern := range AudioPatterns {
			want := AudioCondition{Pitch: pitch, Pattern: pattern}
			samples, err := RenderAudio(want)
			if err != nil {
				t.Fatalf("RenderAudio(%+v): %v", want, err)
			}
			got, confidence, err := ClassifyAudio(samples)
			if err != nil || got != want || confidence <= 0 {
				t.Errorf("ClassifyAudio(%+v) = %+v, %v, %v", want, got, confidence, err)
			}
		}
	}
}

func TestClassifierIsIndependent(t *testing.T) {
	rng := rand.New(rand.NewSource(29))
	for _, colour := range ImageColours {
		for _, shape := range ImageShapes {
			want := ImageCondition{Colour: colour, Shape: shape}
			pixels, err := RenderImage(want)
			if err != nil {
				t.Fatal(err)
			}
			for i := range pixels {
				pixels[i] += (rng.Float64()*2 - 1) * 0.1
			}
			got, _, err := ClassifyImage(pixels)
			if err != nil || got != want {
				t.Errorf("noisy ClassifyImage(%+v) = %+v, %v", want, got, err)
			}
		}
	}
	for _, pitch := range AudioPitches {
		for _, pattern := range AudioPatterns {
			want := AudioCondition{Pitch: pitch, Pattern: pattern}
			samples, err := RenderAudio(want)
			if err != nil {
				t.Fatal(err)
			}
			for i := range samples {
				samples[i] += (rng.Float64()*2 - 1) * 0.1
			}
			got, _, err := ClassifyAudio(samples)
			if err != nil || got != want {
				t.Errorf("noisy ClassifyAudio(%+v) = %+v, %v", want, got, err)
			}
		}
	}
	_, imageConfidence, imageErr := ClassifyImage(make([]float64, ImageSize*ImageSize*3))
	if imageErr != nil || imageConfidence != 0 {
		t.Errorf("zero image confidence/error = %v/%v, want 0/nil", imageConfidence, imageErr)
	}
	_, audioConfidence, audioErr := ClassifyAudio(make([]float64, AudioBlock))
	if audioErr != nil || audioConfidence != 0 {
		t.Errorf("zero audio confidence/error = %v/%v, want 0/nil", audioConfidence, audioErr)
	}
}

func TestRenderFixtureValues(t *testing.T) {
	pixels, err := RenderImage(ImageCondition{Colour: "red", Shape: "diagonal"})
	if err != nil {
		t.Fatal(err)
	}
	for r := 0; r < ImageSize; r++ {
		for c := 0; c < ImageSize; c++ {
			want := 0.0
			if r == c || c == r+1 {
				want = 1
			}
			if got := pixels[(r*ImageSize+c)*3]; got != want {
				t.Errorf("red pixel (%d,%d) = %v, want %v", r, c, got, want)
			}
		}
	}
	for _, condition := range []AudioCondition{{Pitch: "low", Pattern: "fade"}, {Pitch: "mid", Pattern: "pulse"}} {
		samples, err := RenderAudio(condition)
		if err != nil {
			t.Fatal(err)
		}
		for n, got := range samples {
			hz := map[string]float64{"low": 500, "mid": 1000}[condition.Pitch]
			envelope := 1.0
			if condition.Pattern == "fade" {
				envelope = 1 - float64(n)/AudioBlock
			} else if condition.Pattern == "pulse" && n%64 >= 32 {
				envelope = 0
			}
			want := 0.8 * math.Sin(2*math.Pi*hz*float64(n)/AudioSampleRate) * envelope
			if math.Abs(got-want) > 1e-14 {
				t.Errorf("sample %d for %+v = %.17g, want %.17g", n, condition, got, want)
				break
			}
		}
	}
}
