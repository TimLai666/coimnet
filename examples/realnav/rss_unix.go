//go:build !windows

package main

import (
	"runtime"
	"syscall"
)

func maxRSSBytes() uint64 {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return 0
	}
	max := uint64(usage.Maxrss)
	if runtime.GOOS != "darwin" {
		max *= 1024
	}
	return max
}
