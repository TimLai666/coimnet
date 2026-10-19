package main

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func writeGzip(t *testing.T, path string, data []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := gzip.NewWriter(f)
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestReadSplitGroupsColumns(t *testing.T) {
	dir := t.TempDir()
	// Pixel (row, col) holds row + 2*col, so every position is distinct.
	img := make([]byte, side*side)
	for row := 0; row < side; row++ {
		for col := 0; col < side; col++ {
			img[row*side+col] = byte(row + 2*col)
		}
	}
	// IDX magic: two zero bytes, type 0x08 (unsigned byte), dimension count.
	imageHeader := []byte{0, 0, 8, 3, 0, 0, 0, 1, 0, 0, 0, side, 0, 0, 0, side}
	labelHeader := []byte{0, 0, 8, 1, 0, 0, 0, 1}
	writeGzip(t, filepath.Join(dir, "x-images-idx3-ubyte.gz"), append(imageHeader, img...))
	writeGzip(t, filepath.Join(dir, "x-labels-idx1-ubyte.gz"), append(labelHeader, 7))
	for _, cols := range []int{1, 2, 4} {
		xs, ys, err := readSplit(dir, "x", 1, cols)
		if err != nil {
			t.Fatal(err)
		}
		if ys[0] != 7 || len(xs[0]) != side/cols {
			t.Fatalf("cols %d: label %d, %d steps", cols, ys[0], len(xs[0]))
		}
		for step, v := range xs[0] {
			if len(v) != side*cols {
				t.Fatalf("cols %d: step %d has %d values", cols, step, len(v))
			}
			for i, got := range v {
				col, row := step*cols+i/side, i%side
				if want := float64(row+2*col) / 255; got != want {
					t.Fatalf("cols %d: step %d value %d = %v, want pixel (%d, %d) = %v", cols, step, i, got, row, col, want)
				}
			}
		}
	}
}
