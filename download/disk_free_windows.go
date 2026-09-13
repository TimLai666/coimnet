//go:build windows

package download

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func diskFreeBytes(path string) (uint64, error) {
	widePath, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, fmt.Errorf("encode filesystem path: %w", err)
	}
	var available, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(widePath, &available, &total, &free); err != nil {
		return 0, fmt.Errorf("get disk free space: %w", err)
	}
	return available, nil
}
