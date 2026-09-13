package checkpoint

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRejectsUnknownAndDirectoryPaths(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(context.Background(), filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("accepted an unknown checkpoint path")
	}
	if _, err := Load(context.Background(), dir); err == nil {
		t.Fatal("accepted a directory as a checkpoint")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
}
