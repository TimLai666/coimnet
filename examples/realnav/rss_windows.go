//go:build windows

package main

// Windows has no syscall.Getrusage equivalent in the standard library. Keep
// the report field defined and use zero to mean that RSS was unavailable.
func maxRSSBytes() uint64 { return 0 }
