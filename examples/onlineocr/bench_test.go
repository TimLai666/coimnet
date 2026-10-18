package main

import (
	"context"
	"testing"

	"github.com/TimLai666/coimnet/tasks/ocr"
	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

// benchSetup builds the showdown's recognizer from its seed alone; the
// timing of a call does not depend on trained values.
func benchSetup(b *testing.B) (*ocr.Recognizer, glyphs.Image) {
	b.Helper()
	alphabet := []rune("0123456789")
	r, err := ocr.NewRecognizer(ocr.RecognizerConfig{
		Height: 28, Hidden: 24, Alphabet: alphabet, LearningRate: 0.002, Seed: 1,
		TrainableEncoder: true, InputProfile: "single-character", CharacterWidth: 28,
	})
	if err != nil {
		b.Fatal(err)
	}
	pix := make([]float64, 28*28)
	for i := range pix {
		pix[i] = float64(i%7) / 7
	}
	return r, glyphs.Image{Width: 28, Height: 28, Pixels: pix}
}

func BenchmarkRecognizerPredict(b *testing.B) {
	r, img := benchSetup(b)
	l := &recognizerLearner{r: r}
	ctx := context.Background()
	b.ResetTimer()
	for b.Loop() {
		if _, err := l.predict(ctx, img); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPlasticPredict(b *testing.B) {
	r, img := benchSetup(b)
	l, err := newPlasticLearner(r, 0.05)
	if err != nil {
		b.Fatal(err)
	}
	if err := l.freeze(true); err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ResetTimer()
	for b.Loop() {
		if _, err := l.predict(ctx, img); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPlasticStep(b *testing.B) {
	r, img := benchSetup(b)
	l, err := newPlasticLearner(r, 0.05)
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ResetTimer()
	for b.Loop() {
		p, err := l.predict(ctx, img)
		if err != nil {
			b.Fatal(err)
		}
		if err := l.learn(ctx, img, p, 3); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGradientStep(b *testing.B) {
	r, img := benchSetup(b)
	l := &recognizerLearner{r: r, train: true}
	ctx := context.Background()
	b.ResetTimer()
	for b.Loop() {
		p, err := l.predict(ctx, img)
		if err != nil {
			b.Fatal(err)
		}
		if err := l.learn(ctx, img, p, 3); err != nil {
			b.Fatal(err)
		}
	}
}
