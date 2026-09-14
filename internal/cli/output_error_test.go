package cli

import (
	"context"
	"errors"
	"io"
	"testing"
)

func TestHelpReturnsUsageWriteError(t *testing.T) {
	writeErr := errors.New("usage output failed")
	for _, args := range [][]string{
		{"doctor", "--help"},
		{"examples", "run", "delayed", "--help"},
		{"examples", "run", "lif-threshold", "--help"},
		{"train", "delayed", "--help"},
		{"resume", "--help"},
		{"predict", "--help"},
	} {
		t.Run(args[0], func(t *testing.T) {
			err := Run(context.Background(), args, failingWriter{err: writeErr}, io.Discard)
			if !errors.Is(err, writeErr) {
				t.Fatalf("Run(%v) error = %v, want usage write error", args, err)
			}
		})
	}
}

func TestHelpReturnsUsageShortWrite(t *testing.T) {
	for _, args := range [][]string{nil, {"doctor", "--help"}, {"train"}, {"examples"}, {"examples", "list"}, {"data"}, {"data", "sources"}, {"data", "sources", "--help"}, {"data", "download", "--help"}, {"data", "inspect", "--help"}} {
		err := Run(context.Background(), args, shortWriter{}, io.Discard)
		if !errors.Is(err, io.ErrShortWrite) {
			t.Errorf("Run(%v) error = %v, want io.ErrShortWrite", args, err)
		}
	}
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}
