// Package ocr is the OCR task package: rule-based projection segmentation
// that handles only single-column, horizontal text lines. It makes no claim
// about arbitrary page layouts; metrics for OCR/ASR evaluation live here too.
package ocr

import (
	"errors"

	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

// Rect is a pixel rectangle, inclusive-exclusive on both axes.
type Rect struct {
	X0, Y0, X1, Y1 int
}

// Block is one text region a recogniser reads: its bounds, its reading order
// (0-based, top to bottom) and, once read, its text.
type Block struct {
	Bounds       Rect   `json:"bounds"`
	ReadingOrder int    `json:"reading_order"`
	Text         string `json:"text"`
}

// Page is an image with its blocks.
type Page struct {
	Image  glyphs.Image `json:"image"`
	Blocks []Block      `json:"blocks"`
}

// SegmentLines finds horizontal text lines by projection: a row is inked when
// any pixel > threshold; every maximal run of inked rows is one block, its X
// extent trimmed to the inked columns of that run. Blocks are ordered top to
// bottom.
// Errors: empty image, threshold outside (0, 1), Pixels length ≠ Width*Height.
// No inked row → empty non-nil slice, nil error.
func SegmentLines(img glyphs.Image, threshold float64) ([]Block, error) {
	if img.Width <= 0 || img.Height <= 0 {
		return nil, errors.New("ocr: empty image")
	}
	if len(img.Pixels) != img.Width*img.Height {
		return nil, errors.New("ocr: pixel count does not match width*height")
	}
	if threshold <= 0 || threshold >= 1 {
		return nil, errors.New("ocr: threshold outside (0, 1)")
	}
	blocks := []Block{}
	y := 0
	for y < img.Height {
		for y < img.Height && !rowInked(img, y, threshold) {
			y++
		}
		if y >= img.Height {
			break
		}
		y0 := y
		for y < img.Height && rowInked(img, y, threshold) {
			y++
		}
		y1 := y
		x0, x1 := inkColumnSpan(img, y0, y1, threshold)
		blocks = append(blocks, Block{
			Bounds:       Rect{X0: x0, Y0: y0, X1: x1, Y1: y1},
			ReadingOrder: len(blocks),
		})
	}
	return blocks, nil
}

// Stack composes line images vertically with gap background rows between them
// and pads narrower lines on the right with background, giving a page image
// for fixtures. Errors: no lines, gap < 0, background outside [0, 1].
func Stack(lines []glyphs.Image, gap int, background float64) (glyphs.Image, error) {
	if len(lines) == 0 {
		return glyphs.Image{}, errors.New("ocr: no lines")
	}
	if gap < 0 {
		return glyphs.Image{}, errors.New("ocr: gap cannot be negative")
	}
	if background < 0 || background > 1 {
		return glyphs.Image{}, errors.New("ocr: background outside [0, 1]")
	}
	width := 0
	height := 0
	for _, ln := range lines {
		if ln.Width > width {
			width = ln.Width
		}
		height += ln.Height
	}
	height += gap * (len(lines) - 1)
	img := glyphs.Image{Width: width, Height: height, Pixels: make([]float64, width*height)}
	for i := range img.Pixels {
		img.Pixels[i] = background
	}
	y := 0
	for _, ln := range lines {
		for ly := 0; ly < ln.Height; ly++ {
			src := ln.Pixels[ly*ln.Width : (ly+1)*ln.Width]
			dst := img.Pixels[(y+ly)*width : (y+ly)*width+ln.Width]
			copy(dst, src)
		}
		y += ln.Height + gap
	}
	return img, nil
}

// Crop returns a copy of the pixels inside r. Errors: r outside the image or
// empty.
func Crop(img glyphs.Image, r Rect) (glyphs.Image, error) {
	if r.X0 < 0 || r.Y0 < 0 || r.X1 > img.Width || r.Y1 > img.Height {
		return glyphs.Image{}, errors.New("ocr: rect outside image")
	}
	if r.X0 >= r.X1 || r.Y0 >= r.Y1 {
		return glyphs.Image{}, errors.New("ocr: empty rect")
	}
	out := glyphs.Image{
		Width:  r.X1 - r.X0,
		Height: r.Y1 - r.Y0,
		Pixels: make([]float64, (r.X1-r.X0)*(r.Y1-r.Y0)),
	}
	for y := r.Y0; y < r.Y1; y++ {
		for x := r.X0; x < r.X1; x++ {
			out.Pixels[(y-r.Y0)*out.Width+(x-r.X0)] = img.Pixels[y*img.Width+x]
		}
	}
	return out, nil
}

// rowInked reports whether row y has any pixel above the threshold.
func rowInked(img glyphs.Image, y int, threshold float64) bool {
	for x := 0; x < img.Width; x++ {
		if img.Pixels[y*img.Width+x] > threshold {
			return true
		}
	}
	return false
}

// inkColumnSpan returns the [x0, x1) span of columns that hold any pixel above
// the threshold inside rows [y0, y1). Rows of the span are known inked, so the
// span is non-empty.
func inkColumnSpan(img glyphs.Image, y0, y1 int, threshold float64) (int, int) {
	x0, x1 := img.Width, 0
	for y := y0; y < y1; y++ {
		for x := 0; x < img.Width; x++ {
			if img.Pixels[y*img.Width+x] > threshold {
				if x < x0 {
					x0 = x
				}
				if x+1 > x1 {
					x1 = x + 1
				}
			}
		}
	}
	return x0, x1
}
