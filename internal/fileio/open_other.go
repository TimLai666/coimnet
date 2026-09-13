//go:build !aix && !darwin && !dragonfly && !freebsd && !hurd && !illumos && !linux && !netbsd && !openbsd && !solaris && !windows

package fileio

import "os"

func openRegular(path string) (*os.File, error) {
	if err := preflightRegular(path); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if err := postflightRegular(path, file); err != nil {
		return nil, closeAfterOpenError(file, err)
	}
	return file, nil
}
