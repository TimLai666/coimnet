//go:build windows

package bench

// PeakRSS is not measured on Windows; Validate then refuses the Result.
func PeakRSS() uint64 { return 0 }
