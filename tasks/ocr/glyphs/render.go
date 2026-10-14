package glyphs

import (
	"errors"
	"math/rand/v2"
	"unsafe"
)

// Render draws text left to right: glyph i occupies columns [9i, 9i+8) with one blank column between glyphs, height 8,
// width 9*len(runes)-1. The blank column carries Options.Background in every row, without noise, so the gap is the
// declared background rather than an unwritten zero. Empty text, an option outside its range, or image storage
// whose size cannot be represented by int is an error.
func Render(text string, f Family, o Options) (Image, []Box, error) {
	if text == "" {
		return Image{}, nil, errors.New("glyphs: empty text")
	}
	if err := validateOptions(o); err != nil {
		return Image{}, nil, err
	}
	runes := []rune(text)
	if err := ValidateGenerateDimensions(1, len(runes), len(runes)); err != nil {
		return Image{}, nil, err
	}
	width := 9*len(runes) - 1
	img := Image{Width: width, Height: 8, Pixels: make([]float64, width*8)}
	boxes := make([]Box, 0, len(runes))
	// The blank column after every glyph but the last is background, drawn
	// before the glyphs so it never consumes the noise generator.
	for i := 0; i < len(runes)-1; i++ {
		for y := 0; y < 8; y++ {
			img.Pixels[y*width+9*i+8] = o.Background
		}
	}
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
// ValidateGenerateDimensions rejects unrepresentable storage before sampling or allocating the output.
func Generate(alphabet []rune, n, minLen, maxLen int, f Family, o Options, seed uint64) ([]Sample, error) {
	if len(alphabet) == 0 {
		return nil, errors.New("glyphs: empty alphabet")
	}
	if err := ValidateGenerateDimensions(n, minLen, maxLen); err != nil {
		return nil, err
	}
	if err := validateOptions(o); err != nil {
		return nil, err
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

// ValidateGenerateDimensions checks that n sample records and each rendered
// line of minLen..maxLen glyphs have storage sizes representable by int. It
// allocates nothing and is shared with callers that must reject before training.
// A zero sample count preserves the existing empty-batch result, including
// unused wide length ranges; negative or inverted declarations still fail.
// These are arithmetic bounds, not a limit on available physical memory.
func ValidateGenerateDimensions(n, minLen, maxLen int) error {
	if n < 0 || minLen < 0 || maxLen < minLen {
		return errors.New("glyphs: invalid generate parameters")
	}
	if n == 0 {
		return nil
	}
	maxInt := int(^uint(0) >> 1)
	if n > maxInt/int(unsafe.Sizeof(Sample{})) {
		return errors.New("glyphs: sample storage size overflows int")
	}
	// A line has width 9L-1, eight rows, and eight bytes per float64 pixel.
	// This also bounds the smaller rune, UTF-8 text and Box buffers and keeps
	// the inclusive random length range positive without evaluating overflow.
	maxWidth := maxInt / (8 * int(unsafe.Sizeof(float64(0))))
	if maxLen > (maxWidth+1)/9 {
		return errors.New("glyphs: image storage size overflows int")
	}
	return nil
}

func validateOptions(o Options) error {
	if !(o.Background >= 0 && o.Background <= 0.5) {
		return errors.New("glyphs: Background outside finite [0, 0.5]")
	}
	if !(o.Noise >= 0 && o.Noise <= 0.5) {
		return errors.New("glyphs: Noise outside finite [0, 0.5]")
	}
	return nil
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
