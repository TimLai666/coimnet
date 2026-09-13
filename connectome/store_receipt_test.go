package connectome

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type storeReadContext struct {
	context.Context
	calls  int
	onCall func(int)
}

func (c *storeReadContext) Err() error {
	c.calls++
	if c.onCall != nil {
		c.onCall(c.calls)
	}
	return c.Context.Err()
}

func TestLoadReceiptHashesValidatedBytesWhenPathChanges(t *testing.T) {
	manifest, _ := happyFixture(t, []int{11})
	g := build(t, manifest, fixtureLimits()).Graph
	dir := t.TempDir()
	path := filepath.Join(dir, "graph")
	written, err := Save(context.Background(), path, g)
	if err != nil {
		t.Fatal(err)
	}
	count := &storeReadContext{Context: context.Background()}
	loaded, receipt, err := LoadWithReceipt(count, path, storeLimits())
	if err != nil {
		t.Fatal(err)
	}
	assertSameGraph(t, g, loaded)
	want := written
	want.DurabilityConfirmed = false
	if !reflect.DeepEqual(receipt, want) || receipt.SHA256 != fileSHA256(t, path) {
		t.Fatalf("load receipt: %#v, want %#v", receipt, want)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(dir, "replacement")
	if err := os.WriteFile(replacement, rewriteStoreFooter(t, original, func(*storeFooter) {}, 1), 0600); err != nil {
		t.Fatal(err)
	}
	swapped := false
	ctx := &storeReadContext{Context: context.Background(), onCall: func(call int) {
		if call == count.calls {
			if err := os.Rename(replacement, path); err != nil {
				t.Fatal(err)
			}
			swapped = true
		}
	}}
	loaded, receipt, err = LoadWithReceipt(ctx, path, storeLimits())
	if err != nil {
		t.Fatal(err)
	}
	assertSameGraph(t, g, loaded)
	if !swapped || receipt.SHA256 != written.SHA256 || receipt.SHA256 == fileSHA256(t, path) {
		t.Fatalf("receipt mixed pathname bytes: swapped=%v receipt=%#v", swapped, receipt)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	loaded, receipt, err = LoadWithReceipt(cancelled, path, storeLimits())
	if !errors.Is(err, context.Canceled) || loaded != nil || !reflect.DeepEqual(receipt, StoreReceipt{}) {
		t.Fatalf("failure must return no graph or receipt: %v %#v %v", loaded, receipt, err)
	}
}

func TestLoadRejectsRehashedUnknownConverter(t *testing.T) {
	manifest, _ := happyFixture(t, []int{11})
	g := build(t, manifest, fixtureLimits()).Graph
	g.report.ConverterVersion = "coimnet-connectome-builder/v999"
	var err error
	g.report.Hashes.Report, err = g.report.hash()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "unknown")
	if _, err := Save(context.Background(), path, g); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(context.Background(), path, storeLimits()); !errors.Is(err, ErrStoreCorrupt) {
		t.Fatalf("rehashed unknown converter accepted: %v", err)
	}
}
