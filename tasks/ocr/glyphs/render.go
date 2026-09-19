package glyphs

import (
	"errors"
	"math/rand/v2"
)

// Render draws text left to right: glyph i occupies columns [9i, 9i+8) with one blank column between glyphs, height 8,
// width 9*len(runes)-1. Empty text or an option outside its range is an error.
func Render(text string, f Family, o Options) (Image, []Box, error) {
	if text == "" {
		return Image{}, nil, errors.New("glyphs: empty text")
	}
	if o.Background < 0 || o.Background > 0.5 {
		return Image{}, nil, errors.New("glyphs: Background outside [0, 0.5]")
	}
	if o.Noise < 0 || o.Noise > 0.5 {
		return Image{}, nil, errors.New("glyphs: Noise outside [0, 0.5]")
	}
	runes := []rune(text)
	width := 9*len(runes) - 1
	img := Image{Width: width, Height: 8, Pixels: make([]float64, width*8)}
	boxes := make([]Box, 0, len(runes))
	rng := rand.New(rand.NewPCG(o.Seed, 0))
	for i, r := range runes {
		x0 := 9 * i
		g := f.Glyph(r)
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				v := o.Background
				if g[y][x] == 1 {
					if !o.Invert {
						v = 1
					}
				} else if o.Invert {
					v = 1
				}
				if o.Noise != 0 {
					v += (rng.Float64()*2 - 1) * o.Noise
					if v < 0 {
						v = 0
					} else if v > 1 {
						v = 1
					}
				}
				img.Pixels[y*width+x0+x] = v
			}
		}
		boxes = append(boxes, Box{Rune: r, X0: x0, X1: x0 + 8})
	}
	return img, boxes, nil
}

// Generate draws n samples of length in [minLen, maxLen] from alphabet with rand.NewPCG(seed, 1) for the text and
// Options.Seed = seed + index for the noise, all rendered in family f. Deterministic for equal arguments.
func Generate(alphabet []rune, n, minLen, maxLen int, f Family, o Options, seed uint64) ([]Sample, error) {
	if len(alphabet) == 0 {
		return nil, errors.New("glyphs: empty alphabet")
	}
	if n < 0 || minLen < 0 || maxLen < minLen {
		return nil, errors.New("glyphs: invalid generate parameters")
	}
	textRng := rand.New(rand.NewPCG(seed, 1))
	samples := make([]Sample, 0, n)
	for i := 0; i < n; i++ {
		length := minLen + textRng.IntN(maxLen-minLen+1)
		rs := make([]rune, length)
		for j := range rs {
			rs[j] = alphabet[textRng.IntN(len(alphabet))]
		}
		oi := o
		oi.Seed = seed + uint64(i)
		img, boxes, err := Render(string(rs), f, oi)
		if err != nil {
			return nil, err
		}
		samples = append(samples, Sample{Text: string(rs), Image: img, Boxes: boxes})
	}
	return samples, nil
}

// UnseenCombinations returns the samples of candidates whose text does not appear in seen.
func UnseenCombinations(seen, candidates []Sample) []Sample {
	seenTexts := make(map[string]bool, len(seen))
	for _, s := range seen {
		seenTexts[s.Text] = true
	}
	out := make([]Sample, 0, len(candidates))
	for _, c := range candidates {
		if !seenTexts[c.Text] {
			out = append(out, c)
		}
	}
	return out
}
