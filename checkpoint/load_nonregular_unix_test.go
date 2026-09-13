//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package checkpoint

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestLoadRejectsFIFOPromptly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checkpoint.pipe")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	defer os.Remove(path)

	result := make(chan error, 1)
	go func() {
		_, err := Load(context.Background(), path)
		result <- err
	}()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("accepted a FIFO as a checkpoint")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("FIFO checkpoint load did not return promptly")
	}
}
