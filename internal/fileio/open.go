package fileio

import (
	"context"
	"fmt"
	"os"
)

// OpenRegular opens an ordinary file read-only for streaming or random access.
// The caller owns the returned file and must close it. Platform-specific
// symlink protection follows the package contract. Context is checked around
// opening; subsequent reads require the caller's own limits and cancellation.
func OpenRegular(ctx context.Context, path string) (*os.File, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if path == "" {
		return nil, fmt.Errorf("file path must not be empty")
	}
	f, err := openRegular(path)
	if err != nil {
		return nil, fmt.Errorf("open regular file %q: %w", path, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, closeAfterOpenError(f, err)
	}
	return f, nil
}
