// Package glyphs renders deterministic pseudo-font fixtures for OCR tasks: a
// Family turns each rune into a fixed 8x8 bitmap drawn from a seeded random
// generator, with no font files anywhere. Render places the bitmaps on a
// grayscale raster with optional noise and inversion, and Generate builds
// labeled samples for training and evaluation.
//
// The bitmaps are fixtures, not characters: they exist to exercise the
// pipeline and to support the ticket's train/test split of one family per
// split. Nothing here claims to recognize or reproduce real glyphs.
package glyphs

import (
	"math/rand/v2"
)

// Family is a deterministic pseudo-font: every rune renders to an 8x8 bitmap drawn from rand.NewPCG(Seed, uint64(r)).
// Shapes are stable per (Seed, rune), differ between families and carry no font file. The bitmaps do not look like the
// characters; they are fixtures that prove the pipeline, and the ticket's train/test split is "one family per split".
type Family struct {
	Seed uint64 `json:"seed"`
}

// Glyph returns the 8x8 bitmap (row-major, 1 = ink) of r. It is rejection-sampled until the ink count is in [12, 40], so
// no glyph is empty or full; each pixel is ink with probability 0.4 before rejection.
func (f Family) Glyph(r rune) [8][8]uint8 {
	rng := rand.New(rand.NewPCG(f.Seed, uint64(r)))
	for {
		var g [8][8]uint8
		ink := 0
		for y := range g {
			for x := range g[y] {
				if rng.Float64() < 0.4 {
					g[y][x] = 1
					ink++
				}
			}
		}
		if ink >= 12 && ink <= 40 {
			return g
		}
	}
}

// Image is a grayscale raster in [0, 1], row-major, 1 = ink.
type Image struct {
	Width  int       `json:"width"`
	Height int       `json:"height"`
	Pixels []float64 `json:"pixels"`
}

// Box is the horizontal span of one rune in a rendered line, inclusive-exclusive.
type Box struct {
	Rune rune `json:"rune"`
	X0   int  `json:"x0"`
	X1   int  `json:"x1"`
}

// Options controls the rendering: Background in [0, 0.5] is the value of non-ink pixels, Noise in [0, 0.5] is the amplitude of
// uniform noise added to every pixel from rand.NewPCG(Seed, 0) then clamped to [0, 1], Invert swaps ink and background.
type Options struct {
	Background float64 `json:"background"`
	Noise      float64 `json:"noise"`
	Seed       uint64  `json:"seed"`
	Invert     bool    `json:"invert"`
}

// Sample is one rendered line with its label.
type Sample struct {
	Text  string `json:"text"`
	Image Image  `json:"image"`
	Boxes []Box  `json:"boxes"`
}
