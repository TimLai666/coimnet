//go:build aix || darwin || dragonfly || freebsd || hurd || illumos || linux || netbsd || openbsd || solaris

package fileio

import (
	"fmt"
	"os"
)

func finishUnixOpen(path string, fd int, closeFD func(int) error) (*os.File, error) {
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		if closeErr := closeFD(fd); closeErr != nil {
			return nil, fmt.Errorf("create opened file: %w; close file: %v", errInvalidFile, closeErr)
		}
		return nil, errInvalidFile
	}
	if err := postflightRegular(path, file); err != nil {
		return nil, closeAfterOpenError(file, err)
	}
	return file, nil
}
