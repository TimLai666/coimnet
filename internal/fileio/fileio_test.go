package fileio

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadRegularReadsBoundedRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.json")
	want := []byte("{\"value\":42}\n")
	if err := os.WriteFile(path, want, 0600); err != nil {
		t.Fatal(err)
	}

	got, err := ReadRegular(context.Background(), path, int64(len(want)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("ReadRegular() = %q, want %q", got, want)
	}
}

func TestReadRegularRejectsUnknownNonregularAndOversizedPaths(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadRegular(context.Background(), filepath.Join(dir, "missing"), 4); err == nil {
		t.Fatal("accepted an unknown path")
	}
	if _, err := ReadRegular(context.Background(), dir, 4); err == nil || !strings.Contains(err.Error(), "regular") {
		t.Fatalf("directory error = %v, want regular-file error", err)
	}
	over := filepath.Join(dir, "oversized")
	if err := os.WriteFile(over, []byte("12345"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRegular(context.Background(), over, 4); err == nil || !strings.Contains(err.Error(), "4") {
		t.Fatalf("oversized error = %v, want byte-limit error", err)
	}
}

func TestReadRegularHonorsCanceledContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input")
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadRegular(ctx, path, 4); !errors.Is(err, context.Canceled) {
		t.Fatalf("ReadRegular() error = %v, want context.Canceled", err)
	}
}

func TestReadRegularRejectsNilContextAndNegativeLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input")
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRegular(nil, path, 4); err == nil || !strings.Contains(err.Error(), "nil context") {
		t.Fatalf("nil context error = %v", err)
	}
	if _, err := ReadRegular(context.Background(), path, -1); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("negative limit error = %v", err)
	}
}

func TestReadRegularRejectsSymlinkPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	if err := os.WriteFile(target, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := ReadRegular(context.Background(), link, 4); err == nil || !strings.Contains(err.Error(), "regular") {
		t.Fatalf("symlink error = %v, want regular-file error", err)
	}
}
