package ocr_test

import (
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/tasks/ocr"
	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

const (
	testThreshold  = 0.5
	testBackground = 0.1
)

func mustRender(t *testing.T, text string, o glyphs.Options) glyphs.Image {
	t.Helper()
	img, _, err := glyphs.Render(text, glyphs.Family{Seed: 1}, o)
	if err != nil {
		t.Fatalf("Render(%q): %v", text, err)
	}
	return img
}

// rowInkedOne reports whether row y of img has any pixel above the threshold.
func rowInkedOne(img glyphs.Image, y int, threshold float64) bool {
	for x := 0; x < img.Width; x++ {
		if img.Pixels[y*img.Width+x] > threshold {
			return true
		}
	}
	return false
}

// rowRuns returns the maximal runs of inked rows in img, in [start, end) rows.
func rowRuns(img glyphs.Image, threshold float64) [][2]int {
	var runs [][2]int
	for y := 0; y < img.Height; {
		for y < img.Height && !rowInkedOne(img, y, threshold) {
			y++
		}
		if y >= img.Height {
			break
		}
		start := y
		for y < img.Height && rowInkedOne(img, y, threshold) {
			y++
		}
		runs = append(runs, [2]int{start, y})
	}
	return runs
}

// inkedColumns returns the [x0, x1) span of columns holding any pixel above
// the threshold inside rows [y0, y1). A line without ink yields x0 == x1.
func inkedColumns(img glyphs.Image, y0, y1 int, threshold float64) (int, int) {
	x0, last := img.Width, -1
	for y := y0; y < y1; y++ {
		for x := 0; x < img.Width; x++ {
			if img.Pixels[y*img.Width+x] > threshold {
				if x < x0 {
					x0 = x
				}
				if x > last {
					last = x
				}
			}
		}
	}
	return x0, last + 1
}

// expectedBlocks is an independent oracle for SegmentLines on a Stack of
// lines with the given gap: one block per maximal row run, order top to
// bottom, X trimmed to the inked columns of the run.
func expectedBlocks(lines []glyphs.Image, gap int, threshold float64) []ocr.Block {
	var want []ocr.Block
	yOff := 0
	order := 0
	for _, ln := range lines {
		for _, r := range rowRuns(ln, threshold) {
			x0, x1 := inkedColumns(ln, r[0], r[1], threshold)
			want = append(want, ocr.Block{
				Bounds:       ocr.Rect{X0: x0, Y0: yOff + r[0], X1: x1, Y1: yOff + r[1]},
				ReadingOrder: order,
			})
			order++
		}
		yOff += ln.Height + gap
	}
	return want
}

// TestSegmentLinesFindsStackedLines pins the projection contract: each line
// becomes one block with exact Y bounds and reading order, the X extent is
// the line's inked columns, and cropping the block from the page equals the
// original line without its all-background columns.
func TestSegmentLinesFindsStackedLines(t *testing.T) {
	line1 := mustRender(t, "台灣", glyphs.Options{Background: testBackground})
	line2 := mustRender(t, "AB", glyphs.Options{Background: testBackground})
	if line1.Width != 17 || line2.Width != 17 || line1.Height != 8 {
		t.Fatalf("fixture sizes: line1 %dx%d, line2 %dx%d, want 17x8 rows", line1.Width, line1.Height, line2.Width, line2.Height)
	}
	page, err := ocr.Stack([]glyphs.Image{line1, line2}, 3, testBackground)
	if err != nil {
		t.Fatalf("Stack: %v", err)
	}
	blocks, err := ocr.SegmentLines(page, testThreshold)
	if err != nil {
		t.Fatalf("SegmentLines: %v", err)
	}
	want := expectedBlocks([]glyphs.Image{line1, line2}, 3, testThreshold)
	if len(want) != 2 {
		t.Fatalf("fixture yields %d row runs, want 2 (rows must be continuous)", len(want))
	}
	if !reflect.DeepEqual(blocks, want) {
		t.Fatalf("SegmentLines = %+v, want %+v", blocks, want)
	}
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}
	if blocks[0].ReadingOrder != 0 || blocks[1].ReadingOrder != 1 {
		t.Fatalf("reading order = %d, %d, want 0, 1", blocks[0].ReadingOrder, blocks[1].ReadingOrder)
	}
	if b := blocks[0].Bounds; b.Y0 != 0 || b.Y1 != line1.Height {
		t.Fatalf("block 0 Y range %d..%d, want 0..%d", b.Y0, b.Y1, line1.Height)
	}
	if b := blocks[1].Bounds; b.Y0 != line1.Height+3 || b.Y1 != line1.Height+3+line2.Height {
		t.Fatalf("block 1 Y range %d..%d, want %d..%d", b.Y0, b.Y1, line1.Height+3, line1.Height+4+line2.Height)
	}
	for i, ln := range []glyphs.Image{line1, line2} {
		b := blocks[i].Bounds
		if b.X0 < 0 || b.X1 > ln.Width || b.X0 >= b.X1 {
			t.Fatalf("block %d X range %d..%d outside [0, %d]", i, b.X0, b.X1, ln.Width)
		}
		// Every inked column of the line lies inside the block extent.
		for x := 0; x < ln.Width; x++ {
			for y := 0; y < ln.Height; y++ {
				if ln.Pixels[y*ln.Width+x] > testThreshold && (x < b.X0 || x >= b.X1) {
					t.Fatalf("block %d X range %d..%d misses inked column %d", i, b.X0, b.X1, x)
				}
			}
		}
		// Cropping the block from the page equals the line with all-background
		// columns outside [X0, X1) removed.
		cropped, err := ocr.Crop(page, b)
		if err != nil {
			t.Fatalf("Crop(page, block %d): %v", i, err)
		}
		ref, err := ocr.Crop(ln, ocr.Rect{X0: b.X0, Y0: 0, X1: b.X1, Y1: ln.Height})
		if err != nil {
			t.Fatalf("Crop(line %d): %v", i, err)
		}
		if !reflect.DeepEqual(cropped.Pixels, ref.Pixels) {
			t.Fatalf("block %d cropped pixels differ from the trimmed original line", i)
		}
	}
}

// TestSegmentLinesIgnoresNoiseBelowThreshold pins that noise below the
// threshold never invents rows: the same two lines are still found.
func TestSegmentLinesIgnoresNoiseBelowThreshold(t *testing.T) {
	o := glyphs.Options{Background: testBackground, Noise: 0.3, Seed: 7}
	line1 := mustRender(t, "台灣", o)
	o.Seed = 8
	line2 := mustRender(t, "AB", o)
	page, err := ocr.Stack([]glyphs.Image{line1, line2}, 3, testBackground)
	if err != nil {
		t.Fatalf("Stack: %v", err)
	}
	blocks, err := ocr.SegmentLines(page, testThreshold)
	if err != nil {
		t.Fatalf("SegmentLines: %v", err)
	}
	want := expectedBlocks([]glyphs.Image{line1, line2}, 3, testThreshold)
	if len(want) != 2 {
		t.Fatalf("fixture yields %d row runs, want 2", len(want))
	}
	if !reflect.DeepEqual(blocks, want) {
		t.Fatalf("SegmentLines = %+v, want %+v", blocks, want)
	}
}

// TestSegmentLinesEmpty pins the empty and error cases: an all-background
// image has no blocks with no error, while zero width, threshold at the
// closed ends of (0, 1) and a pixel-length mismatch all error.
func TestSegmentLinesEmpty(t *testing.T) {
	blank := glyphs.Image{Width: 17, Height: 8, Pixels: make([]float64, 17*8)}
	for i := range blank.Pixels {
		blank.Pixels[i] = testBackground
	}
	blocks, err := ocr.SegmentLines(blank, testThreshold)
	if err != nil {
		t.Fatalf("all-background image: %v", err)
	}
	if len(blocks) != 0 {
		t.Fatalf("got %d blocks, want 0", len(blocks))
	}
	if blocks == nil {
		t.Fatal("expected empty non-nil slice, got nil")
	}

	if _, err := ocr.SegmentLines(glyphs.Image{Width: 0, Height: 8, Pixels: nil}, testThreshold); err == nil {
		t.Fatal("want error for Width 0")
	}
	sized := glyphs.Image{Width: 8, Height: 8, Pixels: make([]float64, 8*8)}
	if _, err := ocr.SegmentLines(sized, 1); err == nil {
		t.Fatal("want error for threshold 1")
	}
	if _, err := ocr.SegmentLines(sized, 0); err == nil {
		t.Fatal("want error for threshold 0")
	}
	if _, err := ocr.SegmentLines(glyphs.Image{Width: 8, Height: 8, Pixels: make([]float64, 10)}, testThreshold); err == nil {
		t.Fatal("want error for Pixels length mismatch")
	}
}

// TestStackPadsAndGaps pins the vertical composition: gap rows are all
// background, the output height is the sum plus the gaps, and a narrower line
// is padded on the right with background.
func TestStackPadsAndGaps(t *testing.T) {
	lineA := mustRender(t, "台灣", glyphs.Options{Background: testBackground})
	lineB := mustRender(t, "AB", glyphs.Options{Background: testBackground})
	page, err := ocr.Stack([]glyphs.Image{lineA, lineB}, 2, testBackground)
	if err != nil {
		t.Fatalf("Stack: %v", err)
	}
	if page.Width != 17 || page.Height != 18 {
		t.Fatalf("size %dx%d, want 17x18", page.Width, page.Height)
	}
	for y := 8; y < 10; y++ {
		for x := 0; x < page.Width; x++ {
			if page.Pixels[y*page.Width+x] != testBackground {
				t.Fatalf("gap pixel (%d,%d) = %v, want %v", y, x, page.Pixels[y*page.Width+x], testBackground)
			}
		}
	}
	for y := 0; y < 8; y++ {
		for x := 0; x < 17; x++ {
			if page.Pixels[y*17+x] != lineA.Pixels[y*17+x] {
				t.Fatalf("line A pixel (%d,%d) changed by Stack", y, x)
			}
		}
	}
	for y := 0; y < 8; y++ {
		for x := 0; x < 17; x++ {
			if page.Pixels[(10+y)*17+x] != lineB.Pixels[y*17+x] {
				t.Fatalf("line B pixel (%d,%d) changed by Stack", y, x)
			}
		}
	}

	narrow := mustRender(t, "A", glyphs.Options{Background: testBackground})
	if narrow.Width != 8 {
		t.Fatalf("narrow line width = %d, want 8", narrow.Width)
	}
	page2, err := ocr.Stack([]glyphs.Image{lineA, narrow}, 2, testBackground)
	if err != nil {
		t.Fatalf("Stack: %v", err)
	}
	if page2.Width != 17 {
		t.Fatalf("stacked width %d, want 17", page2.Width)
	}
	for y := 0; y < narrow.Height; y++ {
		for x := 8; x < 17; x++ {
			if page2.Pixels[(10+y)*17+x] != testBackground {
				t.Fatalf("narrow-line pad pixel (%d,%d) = %v, want %v", y, x, page2.Pixels[(10+y)*17+x], testBackground)
			}
		}
		for x := 0; x < 8; x++ {
			if page2.Pixels[(10+y)*17+x] != narrow.Pixels[y*8+x] {
				t.Fatalf("narrow-line pixel (%d,%d) changed by Stack", y, x)
			}
		}
	}

	if _, err := ocr.Stack(nil, 0, testBackground); err == nil {
		t.Fatal("want error for no lines")
	}
	if _, err := ocr.Stack([]glyphs.Image{lineA}, -1, testBackground); err == nil {
		t.Fatal("want error for gap < 0")
	}
	if _, err := ocr.Stack([]glyphs.Image{lineA}, 0, -0.1); err == nil {
		t.Fatal("want error for background < 0")
	}
	if _, err := ocr.Stack([]glyphs.Image{lineA}, 0, 1.1); err == nil {
		t.Fatal("want error for background > 1")
	}
}

// TestCropRoundTrip pins Crop: cropped pixels match the source region,
// a whole-image crop returns the same pixels, and out-of-bounds or empty
// rectangles error.
func TestCropRoundTrip(t *testing.T) {
	line := mustRender(t, "台灣", glyphs.Options{Background: testBackground, Noise: 0.2, Seed: 3})
	full, err := ocr.Crop(line, ocr.Rect{X0: 0, Y0: 0, X1: line.Width, Y1: line.Height})
	if err != nil {
		t.Fatalf("Crop whole: %v", err)
	}
	if !reflect.DeepEqual(full.Pixels, line.Pixels) {
		t.Fatal("crop of the whole image differs from the source")
	}

	r := ocr.Rect{X0: 3, Y0: 2, X1: 10, Y1: 7}
	sub, err := ocr.Crop(line, r)
	if err != nil {
		t.Fatalf("Crop sub: %v", err)
	}
	if sub.Width != 7 || sub.Height != 5 {
		t.Fatalf("sub size %dx%d, want 7x5", sub.Width, sub.Height)
	}
	for y := 0; y < sub.Height; y++ {
		for x := 0; x < sub.Width; x++ {
			got, want := sub.Pixels[y*sub.Width+x], line.Pixels[(y+2)*line.Width+(x+3)]
			if got != want {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}

	cases := []ocr.Rect{
		{X0: -1, Y0: 0, X1: 3, Y1: 3},
		{X0: 0, Y0: 0, X1: line.Width + 1, Y1: 3},
		{X0: 0, Y0: 0, X1: 3, Y1: line.Height + 1},
		{X0: 3, Y0: 2, X1: 3, Y1: 5},
		{X0: 3, Y0: 2, X1: 5, Y1: 2},
	}
	for _, c := range cases {
		if _, err := ocr.Crop(line, c); err == nil {
			t.Fatalf("Crop(%+v): want error", c)
		}
	}
}
