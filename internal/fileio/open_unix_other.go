//go:build aix || dragonfly || freebsd || hurd || illumos || netbsd || openbsd || solaris

package fileio

import (
	"os"
	"syscall"
)

// These targets use the portable syscall O_NONBLOCK flag. Their syscall
// packages do not share Linux/Darwin's O_NOFOLLOW and O_CLOEXEC contract, so
// the preflight and post-open checks are the supported protection there.
func openRegular(path string) (*os.File, error) {
	if err := preflightRegular(path); err != nil {
		return nil, err
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	return finishUnixOpen(path, fd, syscall.Close)
}
