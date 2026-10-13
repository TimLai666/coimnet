package ocr

import (
	"fmt"
	"math"

	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

const (
	inputProfileColumns         = "columns"
	inputProfileAlignedColumns  = "aligned-columns"
	inputProfileSingleCharacter = "single-character"
)

// validateInputProfile validates the declared input representation and all
// derived array sizes before a recognizer allocates its network.
func (c RecognizerConfig) validateInputProfile() error {
	switch c.InputProfile {
	case "", inputProfileColumns, inputProfileAlignedColumns:
		if c.CharacterWidth != 0 {
			return fmt.Errorf("ocr: character width is only valid for %s", inputProfileSingleCharacter)
		}
	case inputProfileSingleCharacter:
		if c.CharacterWidth <= 0 {
			return fmt.Errorf("ocr: character width %d must be positive", c.CharacterWidth)
		}
	default:
		return fmt.Errorf("ocr: unknown input profile %q", c.InputProfile)
	}
	inputSize, err := recognizerInputSize(c)
	if err != nil {
		return err
	}
	nodes, err := profileSum(c.Height, c.Hidden, "network nodes")
	if err != nil {
		return err
	}
	if _, err := profileProduct(nodes, c.Hidden, "dense edge"); err != nil {
		return err
	}
	if _, err := profileProduct(inputSize, c.Height, "encoder"); err != nil {
		return err
	}
	if _, err := profileProduct(c.Hidden, len(c.Alphabet)+1, "readout"); err != nil {
		return err
	}
	return nil
}

func recognizerInputSize(c RecognizerConfig) (int, error) {
	if c.InputProfile != inputProfileSingleCharacter {
		return c.Height, nil
	}
	return profileProduct(c.Height, c.CharacterWidth, "character input")
}

func profileSum(a, b int, name string) (int, error) {
	if a < 0 || b < 0 {
		return 0, fmt.Errorf("ocr: %s dimensions cannot be negative", name)
	}
	maxInt := int(^uint(0) >> 1)
	if b > maxInt-a {
		return 0, fmt.Errorf("ocr: %s dimensions overflow", name)
	}
	sum := a + b
	if sum > maxInt/8 {
		return 0, fmt.Errorf("ocr: %s allocation size overflow", name)
	}
	return sum, nil
}

func profileProduct(a, b int, name string) (int, error) {
	if a < 0 || b < 0 {
		return 0, fmt.Errorf("ocr: %s dimensions cannot be negative", name)
	}
	maxInt := int(^uint(0) >> 1)
	if a != 0 && b > maxInt/a {
		return 0, fmt.Errorf("ocr: %s dimensions overflow", name)
	}
	product := a * b
	// All these values become float64 slices. Reject before allocation if the
	// byte size itself cannot be represented by an int.
	if product > maxInt/8 {
		return 0, fmt.Errorf("ocr: %s allocation size overflow", name)
	}
	return product, nil
}

func profileImage(img glyphs.Image, height, width int) error {
	if img.Height != height {
		return fmt.Errorf("ocr: image height %d, want %d", img.Height, height)
	}
	if img.Width != width {
		return fmt.Errorf("ocr: image width %d, want %d", img.Width, width)
	}
	area, err := rasterImageArea(img.Width, img.Height)
	if err != nil {
		return err
	}
	if len(img.Pixels) != area {
		return fmt.Errorf("ocr: %d pixels for a %dx%d image", len(img.Pixels), img.Width, img.Height)
	}
	for i, pixel := range img.Pixels {
		if math.IsNaN(pixel) || math.IsInf(pixel, 0) || pixel < 0 || pixel > 1 {
			return fmt.Errorf("ocr: image pixel %d must be finite and in [0,1]", i)
		}
	}
	return nil
}

// Inputs returns independent neural-step inputs for the configured profile.
// Columns remains the raw column converter. Empty and columns preserve the
// legacy sequence, aligned-columns appends an independent zero propagation
// row, and single-character supplies a copied whole-image row followed by a
// zero propagation row.
func (r *Recognizer) Inputs(img glyphs.Image) ([][]float64, error) {
	if r == nil {
		return nil, fmt.Errorf("ocr: nil recognizer")
	}
	switch r.config.InputProfile {
	case "", inputProfileColumns:
		return r.Columns(img)
	case inputProfileAlignedColumns:
		columns, err := r.Columns(img)
		if err != nil {
			return nil, err
		}
		zero := make([]float64, r.config.Height)
		return append(columns, zero), nil
	case inputProfileSingleCharacter:
		if err := profileImage(img, r.config.Height, r.config.CharacterWidth); err != nil {
			return nil, err
		}
		inputSize, err := recognizerInputSize(r.config)
		if err != nil {
			return nil, err
		}
		row := append([]float64(nil), img.Pixels...)
		return [][]float64{row, make([]float64, inputSize)}, nil
	default:
		return nil, fmt.Errorf("ocr: unknown input profile %q", r.config.InputProfile)
	}
}
