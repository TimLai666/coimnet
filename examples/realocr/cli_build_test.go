package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// The CLI tests build the command once and run the binary, instead of `go
// run` per test: under -race the parallel package builds made a cold `go run`
// exceed a fixed per-test timeout.
var cliBuild struct {
	once sync.Once
	dir  string
	path string
	err  error
	out  []byte
}

func TestMain(m *testing.M) {
	code := m.Run()
	if cliBuild.dir != "" {
		os.RemoveAll(cliBuild.dir)
	}
	os.Exit(code)
}

// cliCommand returns the built realocr command with args under a context that
// ends with the test's deadline, so only `go test -timeout` bounds the build.
func cliCommand(t *testing.T, args ...string) (*exec.Cmd, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.Background(), context.CancelFunc(func() {})
	if deadline, ok := t.Deadline(); ok {
		ctx, cancel = context.WithDeadline(ctx, deadline.Add(-5*time.Second))
	}
	cliBuild.once.Do(func() {
		cliBuild.dir, cliBuild.err = os.MkdirTemp("", "realocr-cli-")
		if cliBuild.err != nil {
			return
		}
		cliBuild.path = filepath.Join(cliBuild.dir, "realocr")
		cliBuild.out, cliBuild.err = exec.CommandContext(ctx, "go", "build", "-o", cliBuild.path, ".").CombinedOutput()
	})
	if cliBuild.err != nil {
		cancel()
		t.Fatalf("go build: %v\n%s", cliBuild.err, cliBuild.out)
	}
	return exec.CommandContext(ctx, cliBuild.path, args...), cancel
}
