//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package fileio

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestReadRegularRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.pipe")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	defer os.Remove(path)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := ReadRegular(ctx, path, 16)
		result <- err
	}()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("accepted a FIFO as a regular file")
		}
		if errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("FIFO read unexpectedly waited for context: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		cancel()
		t.Fatal("FIFO open did not return promptly")
	}
}
