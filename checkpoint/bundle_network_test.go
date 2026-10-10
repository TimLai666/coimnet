package checkpoint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type networkBundleCancelPhase uint8

const (
	networkBundleCancelBeforeSave networkBundleCancelPhase = iota + 1
	networkBundleCancelAfterDocument
	networkBundleCancelAfterManifest
)

// networkBundleStateContext turns cancellation into a filesystem state
// transition. It avoids timing assumptions while keeping the Save API public.
type networkBundleStateContext struct {
	context.Context
	dir   string
	phase networkBundleCancelPhase
}

func (c networkBundleStateContext) Err() error {
	if err := c.Context.Err(); err != nil {
		return err
	}
	switch c.phase {
	case networkBundleCancelBeforeSave:
		return context.Canceled
	case networkBundleCancelAfterDocument:
		if !networkBundleJSONFileValid(filepath.Join(c.dir, bundleDocumentFile)) {
			return nil
		}
		if networkBundleJSONFileValid(filepath.Join(c.dir, bundleManifestFile)) {
			return nil
		}
		return context.Canceled
	case networkBundleCancelAfterManifest:
		if networkBundleJSONFileValid(filepath.Join(c.dir, bundleManifestFile)) {
			return context.Canceled
		}
	}
	return nil
}

func networkBundleJSONFileValid(path string) bool {
	data, err := os.ReadFile(path)
	return err == nil && json.Valid(data)
}

func TestNetworkBundleConcurrentModelWritersHaveOneWinner(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "shared")
	continuous := newTestModelPackage(t, false)
	lif := newTestModelPackage(t, true)

	type result struct {
		want ModelPackage
		err  error
	}
	results := make(chan result, 2)
	save := func(source ModelPackage) {
		want, err := cloneNetworkModelPackage(source)
		if err == nil {
			owned, cloneErr := cloneNetworkModelPackage(want)
			if cloneErr != nil {
				err = cloneErr
			} else {
				err = SaveModelPackageBundle(context.Background(), dir, owned)
			}
		}
		results <- result{want: want, err: err}
	}
	go save(continuous)
	go save(lif)

	var winner *result
	losers := 0
	for range 2 {
		got := <-results
		if got.err == nil {
			if winner != nil {
				t.Fatal("both concurrent model bundle saves succeeded")
			}
			copy := got
			winner = &copy
			continue
		}
		losers++
	}
	if winner == nil {
		t.Fatal("neither concurrent model bundle save succeeded")
	}
	if losers != 1 {
		t.Fatalf("concurrent model bundle saves had %d losers, want 1", losers)
	}

	got, err := LoadModelPackageBundle(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	assertBundleJSONEqual(t, got, mustCanonicalModelPackage(t, winner.want))
}

func TestNetworkBundleCancellationBeforePublicationCleansOwnedOutput(t *testing.T) {
	t.Run("before save", func(t *testing.T) {
		parent := t.TempDir()
		dir := filepath.Join(parent, "bundle")
		keepPath := filepath.Join(parent, "keep.txt")
		if err := os.WriteFile(keepPath, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}

		ctx := networkBundleStateContext{
			Context: context.Background(),
			dir:     dir,
			phase:   networkBundleCancelBeforeSave,
		}
		err := SaveModelPackageBundle(ctx, dir, newTestModelPackage(t, false))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("SaveModelPackageBundle error = %v, want context.Canceled", err)
		}
		assertNetworkBundleAbsent(t, dir)
		assertNetworkBundleBytes(t, keepPath, []byte("keep"))
	})

	t.Run("after document before manifest", func(t *testing.T) {
		parent := t.TempDir()
		dir := filepath.Join(parent, "bundle")
		keepPath := filepath.Join(parent, "keep.txt")
		if err := os.WriteFile(keepPath, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}

		ctx := networkBundleStateContext{
			Context: context.Background(),
			dir:     dir,
			phase:   networkBundleCancelAfterDocument,
		}
		err := SaveModelPackageBundle(ctx, dir, newTestModelPackage(t, false))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("SaveModelPackageBundle error = %v, want context.Canceled", err)
		}
		assertNetworkBundleAbsent(t, dir)
		assertNetworkBundleBytes(t, keepPath, []byte("keep"))
	})
}

func TestNetworkBundleCancellationAfterManifestPreservesLoadableBundle(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "bundle")
	want := newTestModelPackage(t, false)
	ctx := networkBundleStateContext{
		Context: context.Background(),
		dir:     dir,
		phase:   networkBundleCancelAfterManifest,
	}

	err := SaveModelPackageBundle(ctx, dir, want)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SaveModelPackageBundle error = %v, want context.Canceled", err)
	}
	manifest, readErr := os.ReadFile(filepath.Join(dir, bundleManifestFile))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !json.Valid(manifest) {
		t.Fatal("manifest is not complete JSON after published cancellation")
	}

	got, err := LoadModelPackageBundle(context.Background(), dir)
	if err != nil {
		t.Fatalf("published bundle is not loadable: %v", err)
	}
	assertBundleJSONEqual(t, got, mustCanonicalModelPackage(t, want))
}

func TestNetworkBundleTruncatedManifestRejectedAndFullBytesRestore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bundle")
	want := newTestModelPackage(t, false)
	if err := SaveModelPackageBundle(context.Background(), dir, want); err != nil {
		t.Fatal(err)
	}

	manifestPath := filepath.Join(dir, bundleManifestFile)
	full, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(full) {
		t.Fatal("baseline manifest is not complete JSON")
	}
	defer func() {
		if err := os.WriteFile(manifestPath, full, 0o600); err != nil {
			t.Errorf("restore full manifest: %v", err)
		}
	}()

	for _, length := range []int{0, 1, len(full) / 2, len(full) - 1} {
		t.Run(fmt.Sprintf("manifest-bytes-%d", length), func(t *testing.T) {
			truncated := full[:length]
			if json.Valid(truncated) {
				t.Fatalf("truncated manifest of %d bytes is unexpectedly valid JSON", length)
			}
			if err := os.WriteFile(manifestPath, truncated, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadModelPackageBundle(context.Background(), dir); err == nil {
				t.Fatalf("LoadModelPackageBundle accepted manifest truncated to %d bytes", length)
			}
		})
	}

	if err := os.WriteFile(manifestPath, full, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadModelPackageBundle(context.Background(), dir)
	if err != nil {
		t.Fatalf("LoadModelPackageBundle rejected restored full manifest: %v", err)
	}
	assertBundleJSONEqual(t, got, mustCanonicalModelPackage(t, want))
}

func TestNetworkBundleExistingTargetsRemainUnchanged(t *testing.T) {
	t.Run("existing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "model")
		before := []byte("keep")
		if err := os.WriteFile(path, before, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := SaveModelPackageBundle(context.Background(), path, newTestModelPackage(t, false)); err == nil {
			t.Fatal("SaveModelPackageBundle overwrote an existing file")
		}
		assertNetworkBundleBytes(t, path, before)
	})

	t.Run("empty directory", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "model")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := SaveModelPackageBundle(context.Background(), dir, newTestModelPackage(t, false)); err == nil {
			t.Fatal("SaveModelPackageBundle wrote into an existing empty directory")
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Fatalf("existing empty directory changed: %v", entries)
		}
	})

	t.Run("complete model bundle", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "model")
		want := newTestModelPackage(t, false)
		inputBefore, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		if err := SaveModelPackageBundle(context.Background(), dir, want); err != nil {
			t.Fatal(err)
		}
		inputAfter, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(inputAfter, inputBefore) {
			t.Fatal("SaveModelPackageBundle changed the caller's model package")
		}
		before := readNetworkBundleFiles(t, dir)
		if err := SaveModelPackageBundle(context.Background(), dir, newTestModelPackage(t, true)); err == nil {
			t.Fatal("SaveModelPackageBundle overwrote an existing complete bundle")
		}
		after := readNetworkBundleFiles(t, dir)
		if !reflect.DeepEqual(after, before) {
			t.Fatalf("existing complete bundle changed:\nbefore=%v\nafter=%v", before, after)
		}
		got, err := LoadModelPackageBundle(context.Background(), dir)
		if err != nil {
			t.Fatal(err)
		}
		assertBundleJSONEqual(t, got, mustCanonicalModelPackage(t, want))
	})
}

func cloneNetworkModelPackage(pkg ModelPackage) (ModelPackage, error) {
	data, err := json.Marshal(pkg)
	if err != nil {
		return ModelPackage{}, err
	}
	var clone ModelPackage
	if err := json.Unmarshal(data, &clone); err != nil {
		return ModelPackage{}, err
	}
	return clone, nil
}

func assertNetworkBundleAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("bundle output %q exists after pre-publication cancellation: %v", path, err)
	}
}

func assertNetworkBundleBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%q changed: got %q, want %q", path, got, want)
	}
}

func readNetworkBundleFiles(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("bundle contains unexpected directory %q", entry.Name())
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[entry.Name()] = data
	}
	return files
}
