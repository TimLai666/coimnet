//go:build darwin || linux

package download

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func diskFreeBytes(path string) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("stat filesystem: %w", err)
	}
	if stat.Bsize <= 0 {
		return 0, fmt.Errorf("filesystem reports invalid block size %d", stat.Bsize)
	}
	blockSize := uint64(stat.Bsize)
	blocks := uint64(stat.Bavail)
	if blocks > ^uint64(0)/blockSize {
		return ^uint64(0), nil
	}
	return blocks * blockSize, nil
}
