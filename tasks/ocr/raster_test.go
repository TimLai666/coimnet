package ocr_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/tasks/ocr"
)

func TestOCRRasterPNGDecodeMetadataPixelsAndOwnership(t *testing.T) {
	gray := image.NewGray(image.Rect(0, 0, 3, 2))
	values := []uint8{0, 64, 128, 255, 200, 32}
	for i, value := range values {
		gray.Pix[i] = value
	}
	data := encodeOCRPNG(t, gray)
	path := filepath.Join(t.TempDir(), "sample.png")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ocr.LoadImage(context.Background(), path, ocr.ImageOptions{})
	if err != nil {
		t.Fatalf("LoadImage PNG: %v", err)
	}
	if got.Image.Width != 3 || got.Image.Height != 2 || len(got.Image.Pixels) != 6 {
		t.Fatalf("PNG image shape = %dx%d/%d, want 3x2/6", got.Image.Width, got.Image.Height, len(got.Image.Pixels))
	}
	wantPixels := decodedOCRGrayPixels(t, data, false)
	assertOCRRasterPixels(t, got.Image.Pixels, wantPixels, 0)
	meta := got.Metadata
	if meta.SchemaVersion == "" || meta.SourceColorSpace == "" || meta.CoordinateSpace == "" {
		t.Fatalf("PNG metadata omitted schema/color/coordinate fields: %+v", meta)
	}
	if meta.SourceSHA256 != sha256OCR(data) {
		t.Fatalf("SourceSHA256 = %q, want hash of the decoded source %q", meta.SourceSHA256, sha256OCR(data))
	}
	if meta.Format != "png" || meta.SourceWidth != 3 || meta.SourceHeight != 2 || meta.Width != 3 || meta.Height != 2 {
		t.Fatalf("PNG metadata format/dimensions = %+v", meta)
	}
	if meta.SourceColorSpace == "" || meta.Orientation != "stored-pixel-order" || meta.TextDirection != "horizontal-left-to-right" {
		t.Fatalf("PNG metadata does not identify the declared source orientation/direction: %+v", meta)
	}
	if meta.ScalingMethod == "" || meta.ScaleX != 1 || meta.ScaleY != 1 {
		t.Fatalf("PNG identity scaling metadata = %+v", meta)
	}
	if meta.Region != (ocr.Rect{X0: 0, Y0: 0, X1: 3, Y1: 2}) {
		t.Fatalf("PNG source region = %+v, want full source rectangle", meta.Region)
	}

	second, err := ocr.LoadImage(context.Background(), path, ocr.ImageOptions{})
	if err != nil {
		t.Fatalf("second LoadImage PNG: %v", err)
	}
	secondBefore := append([]float64(nil), second.Image.Pixels...)
	got.Image.Pixels[0] = 0.123
	if !reflect.DeepEqual(second.Image.Pixels, secondBefore) {
		t.Fatal("separate LoadImage results share pixel ownership")
	}
	fileAfter, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fileAfter, data) {
		t.Fatal("LoadImage changed the source file")
	}

	light, err := ocr.LoadImage(context.Background(), path, ocr.ImageOptions{LightInk: true})
	if err != nil {
		t.Fatalf("LoadImage PNG LightInk: %v", err)
	}
	darkPixels := decodedOCRGrayPixels(t, data, false)
	lightWant := decodedOCRGrayPixels(t, data, true)
	assertOCRRasterPixels(t, light.Image.Pixels, lightWant, 0)
	for i := range darkPixels {
		if math.Abs(light.Image.Pixels[i]+darkPixels[i]-1) > 1e-12 {
			t.Fatalf("LightInk pixel %d = %g is not the inverse of dark-ink pixel %g", i, light.Image.Pixels[i], darkPixels[i])
		}
	}
}

func TestOCRRasterPNGAlphaCompositesWhiteAndSupportsRGB(t *testing.T) {
	nrgba := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	// Transparent black must become white before dark-ink conversion. The
	// other pixels exercise opaque black and an opaque non-gray RGB value.
	nrgba.SetNRGBA(0, 0, color.NRGBA{R: 0, G: 0, B: 0, A: 0})
	nrgba.SetNRGBA(1, 0, color.NRGBA{R: 0, G: 0, B: 0, A: 255})
	nrgba.SetNRGBA(2, 0, color.NRGBA{R: 255, G: 0, B: 0, A: 255})
	data := encodeOCRPNG(t, nrgba)
	path := filepath.Join(t.TempDir(), "alpha-rgb.png")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ocr.LoadImage(context.Background(), path, ocr.ImageOptions{})
	if err != nil {
		t.Fatalf("LoadImage alpha/RGB PNG: %v", err)
	}
	if len(got.Image.Pixels) != 3 {
		t.Fatalf("alpha/RGB pixel count = %d, want 3", len(got.Image.Pixels))
	}
	if math.Abs(got.Image.Pixels[0]) > 1e-12 {
		t.Fatalf("transparent black became ink %g, want transparent white background 0", got.Image.Pixels[0])
	}
	if math.Abs(got.Image.Pixels[1]-1) > 1e-12 {
		t.Fatalf("opaque black became %g, want dark ink 1", got.Image.Pixels[1])
	}
	if !(got.Image.Pixels[2] > 0 && got.Image.Pixels[2] < 1) {
		t.Fatalf("opaque red was not converted to an intermediate grayscale ink value: %g", got.Image.Pixels[2])
	}
}

func TestOCRRasterJPEGUsesIndependentDecodedPixels(t *testing.T) {
	gray := image.NewGray(image.Rect(0, 0, 4, 3))
	values := []uint8{0, 32, 96, 255, 255, 128, 64, 16, 200, 80, 40, 5}
	for i, value := range values {
		gray.Pix[i] = value
	}
	data := encodeOCRJPEG(t, gray)
	path := filepath.Join(t.TempDir(), "sample.jpeg")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ocr.LoadImage(context.Background(), path, ocr.ImageOptions{})
	if err != nil {
		t.Fatalf("LoadImage JPEG: %v", err)
	}
	if got.Metadata.Format != "jpeg" || got.Metadata.SourceSHA256 != sha256OCR(data) {
		t.Fatalf("JPEG metadata = %+v, source hash = %q", got.Metadata, got.Metadata.SourceSHA256)
	}
	want := decodedOCRGrayPixels(t, data, false)
	assertOCRRasterPixels(t, got.Image.Pixels, want, 1.0/255.0+1e-12)
}

func TestOCRRasterNearestScalingAndLimits(t *testing.T) {
	gray := image.NewGray(image.Rect(0, 0, 2, 2))
	gray.Pix = []uint8{0, 255, 64, 128}
	data := encodeOCRPNG(t, gray)
	path := filepath.Join(t.TempDir(), "scale.png")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ocr.LoadImage(context.Background(), path, ocr.ImageOptions{Height: 4, MaxPixels: 16})
	if err != nil {
		t.Fatalf("LoadImage scaled PNG: %v", err)
	}
	if got.Image.Width != 4 || got.Image.Height != 4 || len(got.Image.Pixels) != 16 {
		t.Fatalf("scaled shape = %dx%d/%d, want 4x4/16", got.Image.Width, got.Image.Height, len(got.Image.Pixels))
	}
	wantSource := decodedOCRGrayPixels(t, data, false)
	want := make([]float64, 16)
	for y := range 4 {
		for x := range 4 {
			want[y*4+x] = wantSource[(y/2)*2+(x/2)]
		}
	}
	assertOCRRasterPixels(t, got.Image.Pixels, want, 0)
	if got.Metadata.SourceWidth != 2 || got.Metadata.SourceHeight != 2 || got.Metadata.Width != 4 || got.Metadata.Height != 4 || got.Metadata.ScaleX != 2 || got.Metadata.ScaleY != 2 {
		t.Fatalf("scaled metadata = %+v", got.Metadata)
	}
	if got.Metadata.Region != (ocr.Rect{X0: 0, Y0: 0, X1: 2, Y1: 2}) || got.Metadata.ScalingMethod == "" {
		t.Fatalf("scaled metadata lost source region or method: %+v", got.Metadata)
	}

	cases := []struct {
		name string
		opt  ocr.ImageOptions
	}{
		{name: "encoded limit", opt: ocr.ImageOptions{MaxEncodedBytes: int64(len(data) - 1)}},
		{name: "source pixel limit", opt: ocr.ImageOptions{MaxPixels: 3}},
		{name: "scaled pixel limit", opt: ocr.ImageOptions{Height: 4, MaxPixels: 15}},
		{name: "negative encoded limit", opt: ocr.ImageOptions{MaxEncodedBytes: -1}},
		{name: "negative pixel limit", opt: ocr.ImageOptions{MaxPixels: -1}},
		{name: "negative height", opt: ocr.ImageOptions{Height: -1}},
		{name: "height overflow", opt: ocr.ImageOptions{Height: int(^uint(0) >> 1)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err, panicValue := loadOCRImageNoPanic(context.Background(), path, tc.opt)
			if panicValue != nil {
				t.Fatalf("LoadImage panicked: %v", panicValue)
			}
			if err == nil {
				t.Fatalf("LoadImage accepted invalid limit/options and returned %+v", got)
			}
			if !reflect.DeepEqual(got, ocr.Raster{}) {
				t.Fatalf("invalid LoadImage returned partial raster: %+v", got)
			}
		})
	}
}

func TestOCRRasterRejectsMissingBrokenUnsupportedNilAndCanceled(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.png")
	bad := filepath.Join(dir, "bad.png")
	if err := os.WriteFile(bad, []byte("not a PNG"), 0o600); err != nil {
		t.Fatal(err)
	}
	unsupported := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(unsupported, []byte("text"), 0o600); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(dir, "empty.png")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	cases := []struct {
		name string
		ctx  context.Context
		path string
	}{
		{name: "missing", ctx: context.Background(), path: missing},
		{name: "broken", ctx: context.Background(), path: bad},
		{name: "unsupported", ctx: context.Background(), path: unsupported},
		{name: "empty", ctx: context.Background(), path: empty},
		{name: "nil context", ctx: nil, path: missing},
		{name: "canceled", ctx: canceled, path: missing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err, panicValue := loadOCRImageNoPanic(tc.ctx, tc.path, ocr.ImageOptions{})
			if panicValue != nil {
				t.Fatalf("LoadImage panicked: %v", panicValue)
			}
			if err == nil {
				t.Fatalf("LoadImage accepted %s", tc.name)
			}
			if !reflect.DeepEqual(got, ocr.Raster{}) {
				t.Fatalf("%s returned partial raster: %+v", tc.name, got)
			}
			if tc.name == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled LoadImage = %v, want context.Canceled", err)
			}
		})
	}
}

func encodeOCRPNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func encodeOCRJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}
	return buf.Bytes()
}

func sha256OCR(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func decodedOCRGrayPixels(t *testing.T, data []byte, lightInk bool) []float64 {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("independent image.Decode: %v", err)
	}
	bounds := img.Bounds()
	out := make([]float64, 0, bounds.Dx()*bounds.Dy())
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gray := color.GrayModel.Convert(img.At(x, y)).(color.Gray)
			value := float64(gray.Y) / 255
			if !lightInk {
				value = 1 - value
			}
			out = append(out, value)
		}
	}
	return out
}

func assertOCRRasterPixels(t *testing.T, got, want []float64, tolerance float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("pixel length = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if math.IsNaN(got[i]) || math.IsInf(got[i], 0) || got[i] < 0 || got[i] > 1 {
			t.Fatalf("pixel %d = %g is outside finite [0,1]", i, got[i])
		}
		if math.Abs(got[i]-want[i]) > tolerance {
			t.Fatalf("pixel %d = %.17g, want %.17g (tolerance %.3g)", i, got[i], want[i], tolerance)
		}
	}
}

func loadOCRImageNoPanic(ctx context.Context, path string, options ocr.ImageOptions) (raster ocr.Raster, err error, panicValue any) {
	defer func() {
		panicValue = recover()
	}()
	raster, err = ocr.LoadImage(ctx, path, options)
	return raster, err, nil
}
