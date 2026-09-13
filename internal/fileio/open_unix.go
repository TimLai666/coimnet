//go:build darwin || linux

package fileio

import (
	"os"

	"golang.org/x/sys/unix"
)

func openRegular(path string) (*os.File, error) {
	if err := preflightRegular(path); err != nil {
		return nil, err
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return finishUnixOpen(path, fd, unix.Close)
}
