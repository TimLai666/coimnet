package checkpoint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

type bundleErrorMode uint8

const (
	bundleManifestCleanup bundleErrorMode = iota + 1
	bundleDirectorySync
)

type bundleErrorContext struct {
	context.Context
	dir, heldPath, tempPath string
	mode                    bundleErrorMode
	cancel                  context.CancelFunc
	injected                bool
	injectErr               error
}

func (c *bundleErrorContext) Err() error {
	if err := c.Context.Err(); err != nil {
		return err
	}
	if !c.injected {
		switch c.mode {
		case bundleManifestCleanup:
			c.injectManifestCleanup()
		case bundleDirectorySync:
			c.injectDirectorySync()
		}
	}
	return c.Context.Err()
}

func (c *bundleErrorContext) fail(err error) {
	c.injected = true
	c.injectErr = err
}

func (c *bundleErrorContext) injectManifestCleanup() {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if !os.IsNotExist(err) {
			c.fail(err)
		}
		return
	}
	prefix := "." + bundleManifestFile + ".tmp-"
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		path := filepath.Join(c.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			c.fail(err)
			return
		}
		if !json.Valid(data) {
			continue
		}
		c.tempPath = path
		c.heldPath = filepath.Join(c.dir, "."+bundleManifestFile+".held")
		if err := os.Rename(path, c.heldPath); err != nil {
			c.fail(err)
			return
		}
		if err := os.Mkdir(path, 0o700); err != nil {
			c.fail(err)
			return
		}
		if err := os.WriteFile(filepath.Join(path, "child"), []byte("held"), 0o600); err != nil {
			c.fail(err)
			return
		}
		c.injected = true
		c.cancel()
		return
	}
}

func (c *bundleErrorContext) injectDirectorySync() {
	data, err := os.ReadFile(filepath.Join(c.dir, bundleManifestFile))
	if err != nil {
		if !os.IsNotExist(err) {
			c.fail(err)
		}
		return
	}
	if !json.Valid(data) {
		return
	}
	c.heldPath = c.dir + ".held"
	if err := os.Rename(c.dir, c.heldPath); err != nil {
		c.fail(err)
		return
	}
	c.injected = true
}

func TestNetworkBundleCancellationWithCleanupFailureRetainsOutput(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "bundle")
	want := newTestModelPackage(t, false)
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &bundleErrorContext{Context: base, dir: dir, mode: bundleManifestCleanup, cancel: cancel}
	err := SaveModelPackageBundle(ctx, dir, want)
	if ctx.injectErr != nil {
		t.Fatalf("filesystem fixture injection failed: %v", ctx.injectErr)
	}
	if !ctx.injected {
		t.Fatal("did not observe a complete manifest temporary file")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SaveModelPackageBundle error = %v, want context.Canceled", err)
	}
	if !errors.Is(err, syscall.ENOTEMPTY) || !strings.Contains(err.Error(), "remove checkpoint temporary file") {
		t.Fatalf("SaveModelPackageBundle error = %v, want temporary cleanup failure", err)
	}
	if !strings.Contains(err.Error(), "publication unconfirmed") {
		t.Fatalf("SaveModelPackageBundle error = %v, want publication unconfirmed", err)
	}
	for _, path := range []string{filepath.Join(dir, bundleDocumentFile), filepath.Join(dir, bundleArraysFile), ctx.heldPath, filepath.Join(ctx.tempPath, "child")} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("owned bundle output %q was not retained: %v", path, statErr)
		}
	}
	if _, statErr := os.Stat(filepath.Join(dir, bundleManifestFile)); !os.IsNotExist(statErr) {
		t.Fatalf("manifest unexpectedly exists after unconfirmed publication: %v", statErr)
	}
	if _, loadErr := LoadModelPackageBundle(context.Background(), dir); loadErr == nil || !strings.Contains(loadErr.Error(), "manifest is missing") {
		t.Fatalf("LoadModelPackageBundle error = %v, want missing manifest", loadErr)
	}
}

func TestNetworkBundlePublishedDirectorySyncFailureRetainsModel(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "bundle")
	want := newTestModelPackage(t, false)
	before, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &bundleErrorContext{Context: base, dir: dir, mode: bundleDirectorySync, cancel: cancel}
	err = SaveModelPackageBundle(ctx, dir, want)
	if ctx.injectErr != nil {
		t.Fatalf("filesystem fixture injection failed: %v", ctx.injectErr)
	}
	if !ctx.injected {
		t.Fatal("did not observe a complete published manifest")
	}
	if err == nil || !strings.Contains(err.Error(), "bundle published") || !strings.Contains(err.Error(), "durability unconfirmed") || !errors.Is(err, syscall.ENOENT) {
		t.Fatalf("SaveModelPackageBundle error = %v, want published directory sync failure", err)
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Fatalf("original bundle directory still exists after injected move: %v", statErr)
	}
	got, err := LoadModelPackageBundle(context.Background(), ctx.heldPath)
	if err != nil {
		t.Fatalf("moved published bundle is not loadable: %v", err)
	}
	assertBundleJSONEqual(t, got, mustCanonicalModelPackage(t, want))
	after, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("SaveModelPackageBundle changed the caller's model package")
	}
}
