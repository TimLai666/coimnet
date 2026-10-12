package ocr_test

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/tasks/ocr"
	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

func TestOCRPageInferUsesTheRecognizerOnOwnedNormalizedCrops(t *testing.T) {
	ctx := context.Background()
	models := []struct {
		name  string
		model *ocr.Recognizer
	}{
		{
			name: "zero-output",
			model: newOCRZeroOutputRecognizer(t, ocr.RecognizerConfig{
				Height: 8, Hidden: 4, Alphabet: []rune{'a'}, LearningRate: 0.02, Seed: 41,
			}),
		},
		{
			name: "always-a",
			model: newOCRAlwaysARecognizer(t, ocr.RecognizerConfig{
				Height: 8, Hidden: 4, Alphabet: []rune{'a'}, LearningRate: 0.02, Seed: 41,
			}),
		},
	}
	image := ocrPageFixtureImage()
	sourceBefore := append([]float64(nil), image.Pixels...)
	sawNonEmpty := false
	for _, tc := range models {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.model
			beforeSnapshot := r.Snapshot()
			page, err := r.InferPage(ctx, image, ocr.PageOptions{Threshold: 0.5, MaxLinePixels: 128})
			if err != nil {
				t.Fatalf("InferPage: %v", err)
			}
			if len(page.Blocks) != 2 {
				t.Fatalf("InferPage returned %d blocks, want 2", len(page.Blocks))
			}
			wantBounds := []ocr.Rect{
				{X0: 1, Y0: 1, X1: 3, Y1: 3},
				{X0: 0, Y0: 5, X1: 4, Y1: 7},
			}
			if page.Blocks[0].Bounds != wantBounds[0] || page.Blocks[1].Bounds != wantBounds[1] {
				t.Fatalf("block bounds = %#v, want %#v", []ocr.Rect{page.Blocks[0].Bounds, page.Blocks[1].Bounds}, wantBounds)
			}
			for i, block := range page.Blocks {
				if block.ReadingOrder != i {
					t.Fatalf("block %d ReadingOrder = %d, want %d", i, block.ReadingOrder, i)
				}
				crop, err := ocr.Crop(image, block.Bounds)
				if err != nil {
					t.Fatalf("Crop block %d: %v", i, err)
				}
				normalized, err := nearestOCRResize(crop, r.Config().Height)
				if err != nil {
					t.Fatalf("normalize block %d: %v", i, err)
				}
				want, err := r.Infer(ctx, normalized)
				if err != nil {
					t.Fatalf("direct Infer block %d: %v", i, err)
				}
				if block.Text != want {
					t.Fatalf("block %d text = %q, want direct same-model inference %q", i, block.Text, want)
				}
				if block.Text != "" {
					sawNonEmpty = true
				}
			}
			if !reflect.DeepEqual(beforeSnapshot, r.Snapshot()) {
				t.Fatal("InferPage or direct block inference changed the recognizer snapshot")
			}
			if !reflect.DeepEqual(image.Pixels, sourceBefore) {
				t.Fatal("InferPage modified the caller's source pixels")
			}
			if len(page.Image.Pixels) != len(image.Pixels) || !reflect.DeepEqual(page.Image.Pixels, image.Pixels) {
				t.Fatal("InferPage did not return the source image contents")
			}
			page.Image.Pixels[0] = 0.987
			if image.Pixels[0] == page.Image.Pixels[0] {
				t.Fatal("Page.Image aliases the caller's pixel slice")
			}
		})
	}
	if !sawNonEmpty {
		t.Fatal("control recognizers produced only empty page text; InferPage could have skipped inference")
	}
}

func TestOCRPageScalesDifferentLineHeightAndPreservesReadingOrder(t *testing.T) {
	ctx := context.Background()
	r := newOCRZeroOutputRecognizer(t, ocr.RecognizerConfig{
		Height: 8, Hidden: 4, Alphabet: []rune{'a'}, LearningRate: 0.02, Seed: 42,
	})
	// Each source line is only two pixels high. InferPage must crop in source
	// coordinates, resize the crop to the recognizer height, and then call the
	// same public Infer path.
	image := glyphs.Image{Width: 5, Height: 8, Pixels: make([]float64, 5*8)}
	for i := range image.Pixels {
		image.Pixels[i] = 0.1
	}
	for y := 1; y <= 2; y++ {
		for x := 1; x <= 3; x++ {
			image.Pixels[y*image.Width+x] = 1
		}
	}
	for y := 5; y <= 6; y++ {
		for x := 0; x <= 4; x++ {
			image.Pixels[y*image.Width+x] = 1
		}
	}
	page, err := r.InferPage(ctx, image, ocr.PageOptions{Threshold: 0.5, MaxLinePixels: 128})
	if err != nil {
		t.Fatalf("InferPage with short lines: %v", err)
	}
	if len(page.Blocks) != 2 {
		t.Fatalf("short-line page returned %d blocks, want 2", len(page.Blocks))
	}
	if page.Blocks[0].Bounds != (ocr.Rect{X0: 1, Y0: 1, X1: 4, Y1: 3}) || page.Blocks[1].Bounds != (ocr.Rect{X0: 0, Y0: 5, X1: 5, Y1: 7}) {
		t.Fatalf("short-line bounds = %#v, want source-coordinate rectangles", page.Blocks)
	}
	for i, block := range page.Blocks {
		if block.ReadingOrder != i {
			t.Fatalf("short-line block %d ReadingOrder = %d", i, block.ReadingOrder)
		}
		crop, err := ocr.Crop(image, block.Bounds)
		if err != nil {
			t.Fatal(err)
		}
		resized, err := nearestOCRResize(crop, 8)
		if err != nil {
			t.Fatal(err)
		}
		want, err := r.Infer(ctx, resized)
		if err != nil {
			t.Fatalf("direct short-line Infer %d: %v", i, err)
		}
		if block.Text != want {
			t.Fatalf("short-line block %d text = %q, want %q", i, block.Text, want)
		}
	}
}

func TestOCRPageBlankIsSuccessfulWithNonNilEmptyBlocks(t *testing.T) {
	r := newOCRZeroOutputRecognizer(t, ocr.RecognizerConfig{
		Height: 8, Hidden: 4, Alphabet: []rune{'a'}, LearningRate: 0.02, Seed: 43,
	})
	blank := glyphs.Image{Width: 4, Height: 5, Pixels: make([]float64, 4*5)}
	for i := range blank.Pixels {
		blank.Pixels[i] = 0.1
	}
	page, err := r.InferPage(context.Background(), blank, ocr.PageOptions{Threshold: 0.5})
	if err != nil {
		t.Fatalf("InferPage blank: %v", err)
	}
	if page.Blocks == nil {
		t.Fatal("blank page returned nil Blocks; empty page must be successful and non-nil")
	}
	if len(page.Blocks) != 0 {
		t.Fatalf("blank page returned %d blocks, want 0", len(page.Blocks))
	}
}

func TestOCRPageRejectsInvalidNilCanceledAndLineLimitsWithoutPartialPage(t *testing.T) {
	image := ocrPageFixtureImage()
	r := newOCRZeroOutputRecognizer(t, ocr.RecognizerConfig{
		Height: 8, Hidden: 4, Alphabet: []rune{'a'}, LearningRate: 0.02, Seed: 44,
	})
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	cases := []struct {
		name string
		ctx  context.Context
		img  glyphs.Image
		opt  ocr.PageOptions
		want error
	}{
		{name: "nil recognizer", ctx: context.Background(), img: image, opt: ocr.PageOptions{Threshold: 0.5}},
		{name: "nil context", ctx: nil, img: image, opt: ocr.PageOptions{Threshold: 0.5}},
		{name: "canceled context", ctx: canceled, img: image, opt: ocr.PageOptions{Threshold: 0.5}, want: context.Canceled},
		{name: "empty image", ctx: context.Background(), img: glyphs.Image{}, opt: ocr.PageOptions{Threshold: 0.5}},
		{name: "missing pixels", ctx: context.Background(), img: glyphs.Image{Width: 4, Height: 8}, opt: ocr.PageOptions{Threshold: 0.5}},
		{name: "threshold zero", ctx: context.Background(), img: image, opt: ocr.PageOptions{Threshold: 0}},
		{name: "threshold one", ctx: context.Background(), img: image, opt: ocr.PageOptions{Threshold: 1}},
		{name: "threshold NaN", ctx: context.Background(), img: image, opt: ocr.PageOptions{Threshold: math.NaN()}},
		{name: "negative line limit", ctx: context.Background(), img: image, opt: ocr.PageOptions{Threshold: 0.5, MaxLinePixels: -1}},
		{name: "line pixel limit", ctx: context.Background(), img: image, opt: ocr.PageOptions{Threshold: 0.5, MaxLinePixels: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var receiver *ocr.Recognizer = r
			if tc.name == "nil recognizer" {
				receiver = nil
			}
			got, err, panicValue := inferOCRPageNoPanic(receiver, tc.ctx, tc.img, tc.opt)
			if panicValue != nil {
				t.Fatalf("InferPage panicked: %v", panicValue)
			}
			if err == nil {
				t.Fatalf("InferPage accepted %s and returned %+v", tc.name, got)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("InferPage %s = %v, want %v", tc.name, err, tc.want)
			}
			if got.Image.Width != 0 || got.Image.Height != 0 || got.Image.Pixels != nil || got.Blocks != nil {
				t.Fatalf("%s returned partial page: %+v", tc.name, got)
			}
		})
	}
}

func TestOCRPagePublicProjectionRejectsNaNShapeAndOverflowInputs(t *testing.T) {
	valid := ocrPageFixtureImage()
	missing := glyphs.Image{Width: 2, Height: 2, Pixels: []float64{1, 0, 0}}
	maxInt := int(^uint(0) >> 1)
	overflow := glyphs.Image{Width: maxInt, Height: 2}
	cases := []struct {
		name  string
		check func(t *testing.T)
	}{
		{name: "SegmentLines NaN threshold", check: func(t *testing.T) {
			blocks, err := ocr.SegmentLines(valid, math.NaN())
			if err == nil || blocks != nil {
				t.Fatalf("SegmentLines accepted NaN threshold with blocks=%v err=%v", blocks, err)
			}
		}},
		{name: "Stack NaN background", check: func(t *testing.T) {
			stacked, err := ocr.Stack([]glyphs.Image{valid}, 0, math.NaN())
			if err == nil || !reflect.DeepEqual(stacked, glyphs.Image{}) {
				t.Fatalf("Stack accepted NaN background with image=%+v err=%v", stacked, err)
			}
		}},
		{name: "SegmentLines missing pixels", check: func(t *testing.T) {
			blocks, err := ocr.SegmentLines(missing, 0.5)
			if err == nil || blocks != nil {
				t.Fatalf("SegmentLines accepted missing pixels with blocks=%v err=%v", blocks, err)
			}
		}},
		{name: "Crop missing pixels", check: func(t *testing.T) {
			cropped, err, panicValue := cropOCRNoPanic(missing, ocr.Rect{X0: 0, Y0: 0, X1: 1, Y1: 1})
			if panicValue != nil || err == nil || !reflect.DeepEqual(cropped, glyphs.Image{}) {
				t.Fatalf("Crop missing pixels result=%+v err=%v panic=%v, want error without panic", cropped, err, panicValue)
			}
		}},
		{name: "Crop overflow", check: func(t *testing.T) {
			cropped, err, panicValue := cropOCRNoPanic(overflow, ocr.Rect{X0: 0, Y0: 0, X1: maxInt, Y1: 2})
			if panicValue != nil || err == nil || !reflect.DeepEqual(cropped, glyphs.Image{}) {
				t.Fatalf("Crop overflow result=%+v err=%v panic=%v, want bounded error", cropped, err, panicValue)
			}
		}},
		{name: "Stack overflow", check: func(t *testing.T) {
			stacked, err, panicValue := stackOCRNoPanic([]glyphs.Image{{Width: maxInt, Height: 2}}, 0, 0.1)
			if panicValue != nil || err == nil || !reflect.DeepEqual(stacked, glyphs.Image{}) {
				t.Fatalf("Stack overflow result=%+v err=%v panic=%v, want bounded error", stacked, err, panicValue)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, tc.check)
	}
}

func TestOCRPagePublicProjectionRejectsNonFinitePixels(t *testing.T) {
	r := newOCRZeroOutputRecognizer(t, ocr.RecognizerConfig{
		Height: 8, Hidden: 4, Alphabet: []rune{'a'}, LearningRate: 0.02, Seed: 45,
	})
	for _, tc := range []struct {
		name  string
		value float64
	}{
		{name: "NaN", value: math.NaN()},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "negative infinity", value: math.Inf(-1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := glyphs.Image{Width: 1, Height: 1, Pixels: []float64{tc.value}}
			t.Run("SegmentLines", func(t *testing.T) {
				blocks, err := ocr.SegmentLines(invalid, 0.5)
				if err == nil || blocks != nil {
					t.Fatalf("SegmentLines accepted non-finite pixel with blocks=%v err=%v", blocks, err)
				}
			})
			t.Run("Stack", func(t *testing.T) {
				stacked, err := ocr.Stack([]glyphs.Image{invalid}, 0, 0.1)
				if err == nil || !reflect.DeepEqual(stacked, glyphs.Image{}) {
					t.Fatalf("Stack accepted non-finite pixel with image=%+v err=%v", stacked, err)
				}
			})
			t.Run("Crop", func(t *testing.T) {
				cropped, err, panicValue := cropOCRNoPanic(invalid, ocr.Rect{X0: 0, Y0: 0, X1: 1, Y1: 1})
				if panicValue != nil || err == nil || !reflect.DeepEqual(cropped, glyphs.Image{}) {
					t.Fatalf("Crop accepted non-finite pixel with image=%+v err=%v panic=%v", cropped, err, panicValue)
				}
			})
			t.Run("InferPage", func(t *testing.T) {
				pageImage := glyphs.Image{Width: 1, Height: 8, Pixels: make([]float64, 8)}
				for i := range pageImage.Pixels {
					pageImage.Pixels[i] = 0.1
				}
				pageImage.Pixels[0] = tc.value
				page, err, panicValue := inferOCRPageNoPanic(r, context.Background(), pageImage, ocr.PageOptions{Threshold: 0.5})
				if panicValue != nil || err == nil {
					t.Fatalf("InferPage accepted non-finite pixel with page=%+v err=%v panic=%v", page, err, panicValue)
				}
				if page.Image.Width != 0 || page.Image.Height != 0 || page.Image.Pixels != nil || page.Blocks != nil {
					t.Fatalf("InferPage returned partial page for non-finite pixel: %+v", page)
				}
			})
		})
	}
}

func ocrPageFixtureImage() glyphs.Image {
	image := glyphs.Image{Width: 4, Height: 8, Pixels: make([]float64, 4*8)}
	for i := range image.Pixels {
		image.Pixels[i] = 0.1
	}
	for y := 1; y <= 2; y++ {
		for x := 1; x <= 2; x++ {
			image.Pixels[y*image.Width+x] = 1
		}
	}
	for y := 5; y <= 6; y++ {
		for x := 0; x < image.Width; x++ {
			image.Pixels[y*image.Width+x] = 1
		}
	}
	return image
}

func nearestOCRResize(src glyphs.Image, height int) (glyphs.Image, error) {
	if src.Width <= 0 || src.Height <= 0 || len(src.Pixels) != src.Width*src.Height || height <= 0 {
		return glyphs.Image{}, errors.New("invalid test resize input")
	}
	if src.Height == height {
		return glyphs.Image{Width: src.Width, Height: src.Height, Pixels: append([]float64(nil), src.Pixels...)}, nil
	}
	if src.Width > math.MaxInt/height {
		return glyphs.Image{}, errors.New("test resize overflow")
	}
	width := (src.Width * height) / src.Height
	if width < 1 || width > math.MaxInt/height {
		return glyphs.Image{}, errors.New("test resize dimensions invalid")
	}
	out := glyphs.Image{Width: width, Height: height, Pixels: make([]float64, width*height)}
	for y := 0; y < height; y++ {
		sy := y * src.Height / height
		for x := 0; x < width; x++ {
			sx := x * src.Width / width
			out.Pixels[y*width+x] = src.Pixels[sy*src.Width+sx]
		}
	}
	return out, nil
}

func inferOCRPageNoPanic(r *ocr.Recognizer, ctx context.Context, image glyphs.Image, options ocr.PageOptions) (page ocr.Page, err error, panicValue any) {
	defer func() { panicValue = recover() }()
	page, err = r.InferPage(ctx, image, options)
	return page, err, nil
}

func cropOCRNoPanic(image glyphs.Image, rect ocr.Rect) (cropped glyphs.Image, err error, panicValue any) {
	defer func() { panicValue = recover() }()
	cropped, err = ocr.Crop(image, rect)
	return cropped, err, nil
}

func stackOCRNoPanic(lines []glyphs.Image, gap int, background float64) (image glyphs.Image, err error, panicValue any) {
	defer func() { panicValue = recover() }()
	image, err = ocr.Stack(lines, gap, background)
	return image, err, nil
}
