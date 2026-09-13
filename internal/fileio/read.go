// Package fileio contains bounded readers for inputs that must be ordinary
// files. A path that names a symlink is rejected; callers that need symlink
// resolution must resolve and validate the destination before calling here.
// Linux and Darwin also prevent a symlink from being followed between the
// preflight and open operations. Other Unix targets retain the preflight and
// post-open checks but do not promise atomic race protection.
package fileio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
)

const readChunkBytes = 32 << 10

var errInvalidFile = errors.New("opened file is unavailable")

// ReadRegular reads path in bounded chunks after checking that it is a
// regular, non-symlink file. maxBytes is an inclusive limit; zero permits an
// empty file and a negative limit is invalid. Context cancellation is checked
// between filesystem operations and reads. Filesystem reads themselves are
// not made interruptible by context.
func ReadRegular(ctx context.Context, path string, maxBytes int64) (data []byte, retErr error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if maxBytes < 0 {
		return nil, fmt.Errorf("file byte limit must not be negative")
	}
	if path == "" {
		return nil, fmt.Errorf("file path must not be empty")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	file, err := openRegular(path)
	if err != nil {
		return nil, fmt.Errorf("open regular file %q: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close regular file %q: %w", path, closeErr))
			data = nil
		}
	}()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat regular file %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("path %q is not a regular file", path)
	}
	if info.Size() < 0 {
		return nil, fmt.Errorf("regular file %q has a negative size", path)
	}
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("file %q exceeds %d byte limit", path, maxBytes)
	}

	data = nil
	chunk := make([]byte, readChunkBytes)
	zeroReads := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		count, readErr := file.Read(chunk)
		if count < 0 || count > len(chunk) {
			return nil, fmt.Errorf("read regular file %q returned invalid count %d", path, count)
		}
		if count > 0 {
			zeroReads = 0
			if int64(len(data)) > maxBytes-int64(count) {
				return nil, fmt.Errorf("file %q exceeds %d byte limit", path, maxBytes)
			}
			need := len(data) + count
			if cap(data) < need {
				grown := make([]byte, len(data), nextCapacity(cap(data), need, maxBytes))
				copy(grown, data)
				data = grown
			}
			data = append(data, chunk[:count]...)
		} else if readErr == nil {
			zeroReads++
			if zeroReads >= 100 {
				return nil, io.ErrNoProgress
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("read regular file %q: %w", path, readErr)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}

func nextCapacity(current, need int, maxBytes int64) int {
	maxInt := int64(^uint(0) >> 1)
	limit := maxBytes
	if limit > maxInt {
		limit = maxInt
	}
	capacity := int64(current)
	if capacity == 0 {
		capacity = readChunkBytes
	}
	if capacity > limit {
		capacity = limit
	}
	for capacity < int64(need) {
		if capacity > limit/2 {
			capacity = limit
			break
		}
		capacity *= 2
	}
	return int(capacity)
}

func preflightRegular(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("path %q is not a regular file", path)
	}
	return nil
}

func postflightRegular(path string, file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat opened file %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("path %q is not a regular file", path)
	}
	return nil
}

func closeAfterOpenError(file *os.File, err error) error {
	if closeErr := file.Close(); closeErr != nil {
		return errors.Join(err, fmt.Errorf("close opened file: %w", closeErr))
	}
	return err
}
