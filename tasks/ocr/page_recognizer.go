package ocr

import (
	"context"
	"errors"
	"fmt"

	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

const defaultPageMaxLinePixels = 1 << 20

// PageOptions controls projection segmentation and the source line crop budget.
// Zero MaxLinePixels uses 1,048,576 pixels. Threshold must be in (0, 1).
type PageOptions struct {
	Threshold     float64
	MaxLinePixels int
}

// InferPage segments horizontal blocks and recognizes each normalized line.
// It supports column-oriented profiles; single-character is reserved for
// complete-image classification and is rejected. Bounds remain in the
// caller's image coordinates, and errors return no partial page.
func (r *Recognizer) InferPage(ctx context.Context, image glyphs.Image, options PageOptions) (Page, error) {
	var zero Page
	if r == nil || r.trainer == nil {
		return zero, errors.New("ocr: nil recognizer")
	}
	if r.config.InputProfile == inputProfileSingleCharacter {
		return zero, errors.New("ocr: single-character profile does not support page inference")
	}
	if ctx == nil {
		return zero, errors.New("ocr: context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	maxLinePixels := options.MaxLinePixels
	if maxLinePixels == 0 {
		maxLinePixels = defaultPageMaxLinePixels
	}
	if maxLinePixels < 0 {
		return zero, errors.New("ocr: max line pixels cannot be negative")
	}
	if err := validatePageImage(image); err != nil {
		return zero, err
	}
	blocks, err := SegmentLines(image, options.Threshold)
	if err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}

	page := Page{
		Image:  glyphs.Image{Width: image.Width, Height: image.Height, Pixels: append([]float64(nil), image.Pixels...)},
		Blocks: make([]Block, len(blocks)),
	}
	for i, block := range blocks {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		lineWidth := block.Bounds.X1 - block.Bounds.X0
		lineHeight := block.Bounds.Y1 - block.Bounds.Y0
		linePixels, err := rasterImageArea(lineWidth, lineHeight)
		if err != nil {
			return zero, fmt.Errorf("ocr: line %d bounds: %w", i, err)
		}
		if linePixels > maxLinePixels {
			return zero, fmt.Errorf("ocr: line %d has %d pixels, exceeds limit %d", i, linePixels, maxLinePixels)
		}
		crop, err := Crop(image, block.Bounds)
		if err != nil {
			return zero, fmt.Errorf("ocr: crop line %d: %w", i, err)
		}
		_, normalizedPixels, err := resizeOCRShape(crop.Width, crop.Height, r.config.Height)
		if err != nil {
			return zero, fmt.Errorf("ocr: normalize line %d: %w", i, err)
		}
		normalizationLimit := maxLinePixels
		if normalizationLimit < defaultPageMaxLinePixels {
			normalizationLimit = defaultPageMaxLinePixels
		}
		if normalizedPixels > normalizationLimit {
			return zero, fmt.Errorf("ocr: normalized line %d has %d pixels, exceeds limit %d", i, normalizedPixels, normalizationLimit)
		}
		normalized, err := resizeOCRNearest(crop, r.config.Height)
		if err != nil {
			return zero, fmt.Errorf("ocr: normalize line %d: %w", i, err)
		}
		text, err := r.Infer(ctx, normalized)
		if err != nil {
			return zero, fmt.Errorf("ocr: infer line %d: %w", i, err)
		}
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		page.Blocks[i] = Block{Bounds: block.Bounds, ReadingOrder: block.ReadingOrder, Text: text}
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return page, nil
}

func resizeOCRNearest(src glyphs.Image, height int) (glyphs.Image, error) {
	if height <= 0 {
		return glyphs.Image{}, errors.New("ocr: target height must be positive")
	}
	if err := validatePageImage(src); err != nil {
		return glyphs.Image{}, err
	}
	if src.Height == height {
		return glyphs.Image{Width: src.Width, Height: src.Height, Pixels: append([]float64(nil), src.Pixels...)}, nil
	}
	width, area, err := resizeOCRShape(src.Width, src.Height, height)
	if err != nil {
		return glyphs.Image{}, err
	}
	out := glyphs.Image{Width: width, Height: height, Pixels: make([]float64, area)}
	for y := 0; y < height; y++ {
		sy, err := rasterNearestIndex(y, src.Height, height)
		if err != nil {
			return glyphs.Image{}, err
		}
		for x := 0; x < width; x++ {
			sx, err := rasterNearestIndex(x, src.Width, width)
			if err != nil {
				return glyphs.Image{}, err
			}
			out.Pixels[y*width+x] = src.Pixels[sy*src.Width+sx]
		}
	}
	return out, nil
}

func resizeOCRShape(srcWidth, srcHeight, height int) (int, int, error) {
	if srcWidth <= 0 || srcHeight <= 0 || height <= 0 {
		return 0, 0, errors.New("ocr: resize dimensions must be positive")
	}
	maxInt := int(^uint(0) >> 1)
	if srcWidth > maxInt/height {
		return 0, 0, errors.New("ocr: normalized dimensions overflow")
	}
	width := srcWidth * height / srcHeight
	if width < 1 {
		width = 1
	}
	area, err := rasterImageArea(width, height)
	if err != nil {
		return 0, 0, err
	}
	return width, area, nil
}
