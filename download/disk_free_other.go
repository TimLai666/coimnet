//go:build !darwin && !linux && !windows

package download

import (
	"fmt"
	"runtime"
)

func diskFreeBytes(path string) (uint64, error) {
	return 0, fmt.Errorf("free disk space is unsupported on %s", runtime.GOOS)
}
