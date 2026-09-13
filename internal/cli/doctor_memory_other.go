//go:build !darwin && !linux && !windows

package cli

import "fmt"

func physicalMemoryBytes() (uint64, error) {
	return 0, fmt.Errorf("physical memory probe is unavailable on this platform")
}
