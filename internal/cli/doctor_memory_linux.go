//go:build linux

package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func physicalMemoryBytes() (uint64, error) {
	contents, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, fmt.Errorf("read /proc/meminfo: %w", err)
	}
	for _, line := range strings.Split(string(contents), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "MemTotal:" {
			continue
		}
		kilobytes, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse MemTotal: %w", err)
		}
		if kilobytes > ^uint64(0)/1024 {
			return 0, fmt.Errorf("MemTotal overflows bytes")
		}
		return kilobytes * 1024, nil
	}
	return 0, fmt.Errorf("MemTotal is missing")
}
