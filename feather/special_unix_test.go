//go:build darwin || linux

package feather

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestScanRejectsFIFO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.feather")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Skipf("FIFO unavailable: %v", err)
	}
	defer os.Remove(path)

	report, err := Scan(context.Background(), path, Options{
		MaxFileBytes:   1,
		MaxFooterBytes: 1,
		MaxArrowBytes:  1,
		MaxRows:        1,
	}, nil)
	if err == nil || report.Complete {
		t.Fatalf("accepted FIFO: report=%#v err=%v", report, err)
	}
}
