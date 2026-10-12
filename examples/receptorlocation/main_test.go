package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunBareInvocationAndHelpExposeReadOnlyStoreUsage(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "bare invocation", args: nil},
		{name: "help", args: []string{"--help"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := run(context.Background(), tc.args, &out); err != nil {
				t.Fatalf("run(%v) error = %v", tc.args, err)
			}
			text := out.String()
			for _, want := range []string{"--store", "MaleCNS v1.0", "No model training or files are written"} {
				if !strings.Contains(text, want) {
					t.Errorf("run(%v) output does not contain %q:\n%s", tc.args, want, text)
				}
			}
		})
	}
}
func TestRunBareInvocationDoesNotCreateFiles(t *testing.T) {
	workingDir := t.TempDir()
	entriesBefore, err := os.ReadDir(workingDir)
	if err != nil {
		t.Fatalf("ReadDir(before) error = %v", err)
	}
	oldWorkingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(workingDir); err != nil {
		t.Fatalf("Chdir(%q) error = %v", workingDir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWorkingDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	var out bytes.Buffer
	if err := run(context.Background(), nil, &out); err != nil {
		t.Fatalf("bare run error = %v", err)
	}
	entriesAfter, err := os.ReadDir(workingDir)
	if err != nil {
		t.Fatalf("ReadDir(after) error = %v", err)
	}
	if len(entriesAfter) != len(entriesBefore) {
		t.Fatalf("bare invocation created files: before=%d after=%d", len(entriesBefore), len(entriesAfter))
	}
}

func TestRunRejectsNilContextOrOutput(t *testing.T) {
	if err := run(nil, nil, &bytes.Buffer{}); err == nil {
		t.Fatal("run(nil context) returned nil error")
	}
	if err := run(context.Background(), nil, nil); err == nil {
		t.Fatal("run(nil output) returned nil error")
	}
}

func TestRunRejectsCancellationBeforeReadingSourceOrStore(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	err := run(ctx, []string{"--store", filepath.Join(t.TempDir(), "missing.coimgraph")}, &out)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run(canceled context) error = %v, want context.Canceled", err)
	}
}

func TestRunRejectsMissingStoreUnexpectedArgumentsAndUnknownFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "missing store value", args: []string{"--store"}},
		{name: "empty store value", args: []string{"--store", ""}},
		{name: "unexpected positional argument", args: []string{"--store", "path", "extra"}},
		{name: "unknown flag", args: []string{"--unsupported"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := run(context.Background(), tc.args, &out); err == nil {
				t.Fatalf("run(%v) accepted invalid arguments and wrote:\n%s", tc.args, out.String())
			}
		})
	}
}

func TestRunRejectsNonexistentStoreWithoutCreatingIt(t *testing.T) {
	store := filepath.Join(t.TempDir(), "does-not-exist.coimgraph")
	var out bytes.Buffer
	if err := run(context.Background(), []string{"--store", store}, &out); err == nil {
		t.Fatalf("run() accepted nonexistent store %q", store)
	}
	if _, err := os.Stat(store); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("run() changed nonexistent store path %q: stat error = %v", store, err)
	}
}
