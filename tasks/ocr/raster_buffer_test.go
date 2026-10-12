package ocr_test

import (
	"context"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/tasks/ocr"
	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

// TestLoadImageBufferBoundaryRejectsFloat64Allocation keeps the decoded
// source tiny while making the aspect-preserving output area exceed the
// platform's float64 backing-buffer capacity.
func TestLoadImageBufferBoundaryRejectsFloat64Allocation(t *testing.T) {
	data := encodeOCRPNG(t, image.NewGray(image.Rect(0, 0, 1, 1)))
	path := writeOCRBufferScratch(t, "load-image-buffer.png", data)
	height := 1 << ((strconv.IntSize - 2) / 2)
	maxInt := int(^uint(0) >> 1)

	got, err, panicValue := loadOCRImageNoPanic(context.Background(), path, ocr.ImageOptions{
		Height:    height,
		MaxPixels: maxInt,
	})
	if panicValue != nil {
		t.Fatalf("LoadImage panicked at float64 buffer boundary: %v", panicValue)
	}
	if err == nil {
		t.Fatalf("LoadImage accepted output height %d with MaxPixels=%d and returned %+v", height, maxInt, got)
	}
	if !reflect.DeepEqual(got, ocr.Raster{}) {
		t.Fatalf("LoadImage error returned partial raster: %+v", got)
	}
	requireOCRBufferBoundaryError(t, "LoadImage", err)
}

// TestStackBufferBoundaryRejectsFloat64Allocation keeps both source images
// at one pixel while making the output float64 backing buffer too large.
func TestStackBufferBoundaryRejectsFloat64Allocation(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	line := glyphs.Image{Width: 1, Height: 1, Pixels: []float64{0}}

	got, err, panicValue := stackOCRNoPanic([]glyphs.Image{line, line}, maxInt/8, 0)
	if panicValue != nil {
		t.Fatalf("Stack panicked at float64 buffer boundary: %v", panicValue)
	}
	if err == nil {
		t.Fatalf("Stack accepted gap %d and returned %+v", maxInt/8, got)
	}
	if !reflect.DeepEqual(got, glyphs.Image{}) {
		t.Fatalf("Stack error returned partial image: %+v", got)
	}
	requireOCRBufferBoundaryError(t, "Stack", err)
}

func requireOCRBufferBoundaryError(t *testing.T, operation string, err error) {
	t.Helper()
	message := strings.ToLower(err.Error())
	for _, term := range []string{"overflow", "size", "pixel", "buffer"} {
		if strings.Contains(message, term) {
			return
		}
	}
	t.Fatalf("%s error = %q, want a descriptive overflow/size error", operation, err)
}

func writeOCRBufferScratch(t *testing.T, name string, data []byte) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("locate current working directory for OCR buffer scratch: %v", err)
	}
	for {
		parent := filepath.Dir(root)
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		if parent == root {
			t.Fatal("could not locate repository root for OCR buffer scratch")
		}
		root = parent
	}

	dir := filepath.Join(root, "runs", ".tmp", "62", "buffer-tests")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create OCR buffer scratch directory: %v", err)
	}
	path := filepath.Join(dir, name)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatalf("create OCR buffer scratch file %q: %v", path, err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		t.Fatalf("write OCR buffer scratch file %q: %v", path, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		t.Fatalf("close OCR buffer scratch file %q: %v", path, err)
	}
	t.Cleanup(func() {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			t.Errorf("remove OCR buffer scratch file %q: %v", path, err)
		}
	})
	return path
}
