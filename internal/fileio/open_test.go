package fileio

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenRegularStreaming(t *testing.T) {
	p := filepath.Join(t.TempDir(), "input")
	if err := os.WriteFile(p, []byte("stream"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := OpenRegular(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b := make([]byte, 3)
	if _, err := f.ReadAt(b, 3); err != nil || string(b) != "eam" {
		t.Fatalf("random access: %q, %v", b, err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("x")); err == nil {
		t.Fatal("opened file was writable")
	}
}

func TestOpenRegularRejectsInvalidInput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if f, err := OpenRegular(ctx, "missing"); f != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled open: %v %v", f, err)
	}
	for _, c := range []struct {
		ctx  context.Context
		path string
	}{{nil, "missing"}, {context.Background(), ""}, {context.Background(), t.TempDir()}} {
		if f, err := OpenRegular(c.ctx, c.path); err == nil || f != nil {
			t.Fatalf("invalid open: %v %v", f, err)
		}
	}
}
