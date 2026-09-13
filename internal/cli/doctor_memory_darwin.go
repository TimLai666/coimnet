//go:build darwin

package cli

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func physicalMemoryBytes() (uint64, error) {
	bytes, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0, fmt.Errorf("hw.memsize: %w", err)
	}
	if bytes == 0 {
		return 0, fmt.Errorf("hw.memsize returned zero")
	}
	return bytes, nil
}
