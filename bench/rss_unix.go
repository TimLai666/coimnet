//go:build !windows

package bench

import (
	"runtime"
	"syscall"
)

// PeakRSS is this process's resident-set peak so far, in bytes. Linux reports
// ru_maxrss in KiB and macOS in bytes.
func PeakRSS() uint64 {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return 0
	}
	peak := uint64(usage.Maxrss)
	if runtime.GOOS != "darwin" {
		peak *= 1024
	}
	return peak
}
