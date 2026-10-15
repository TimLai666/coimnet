//go:build darwin || linux

package coimnet_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

const fakeGoSource = `#!/bin/sh
set -eu
record=${FAKE_GO_RECORD:?}
case "${1-}" in
env)
  case "${2-}" in
  GOVERSION) printf '%s\n' "${FAKE_GO_VERSION:-go1.25.12}" ;;
  GOOS) printf '%s\n' "${FAKE_GO_GOOS:-darwin}" ;;
  GOARCH) printf '%s\n' "${FAKE_GO_GOARCH:-arm64}" ;;
  *) exit 2 ;;
  esac
  ;;
test)
  output=
  expect_output=0
  for arg in "$@"; do
    if [ "$expect_output" = 1 ]; then
      output=$arg
      break
    fi
    if [ "$arg" = "-o" ]; then
      expect_output=1
    fi
  done
  if [ -z "$output" ]; then
    printf '%s\n' 'fake go: missing -o output' >&2
    exit 2
  fi
  printf '%s\n' "$output" > "$record"
  if [ "${FAKE_GO_BUILD_FAIL:-0}" = 1 ]; then
    exit 17
  fi
  if [ "${FAKE_GO_BUILD_WAIT:-0}" = 1 ]; then
    : > "${FAKE_GO_STARTED:?}"
    while [ ! -e "${FAKE_GO_RELEASE:?}" ]; do
      sleep 0.01
    done
  fi
  cat > "$output" <<'FAKE_BINARY'
#!/bin/sh
set -eu
value=${COIMNET_RECOMPUTE_RSS:?}
if [ "${FAKE_GO_HELPER_FAIL:-}" = "$value" ]; then
  printf '%s\n' 'forced fake helper failure' >&2
  exit 19
fi
core=${value%:*}
mode=${value#*:}
case "$core:$mode" in
continuous:full|continuous:recompute|lif:full|lif:recompute) ;;
*) exit 20 ;;
esac
segment=0
if [ "$mode" = recompute ]; then
  segment=16
fi
printf '{"core":"%s","mode":"%s","nodes":512,"edges":4096,"steps":256,"segment":%s,"gradient_norm":1,"update_norm":1,"parameters_sha256":"@@FAKE_SHA@@"}\n' "$core" "$mode" "$segment"
FAKE_BINARY
  chmod +x "$output"
  if [ -n "${FAKE_GO_FINISHED:-}" ]; then
    : > "$FAKE_GO_FINISHED"
  fi
  ;;
*) exit 2 ;;
esac
`

type scriptFixture struct {
	root    string
	script  string
	record  string
	tmpRoot string
	baseEnv []string
}

type scriptResult struct {
	exit   int
	output string
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(env)+1)
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			continue
		}
		result = append(result, item)
	}
	return append(result, prefix+value)
}

func (f *scriptFixture) env(options map[string]string) []string {
	env := append([]string(nil), f.baseEnv...)
	for key, value := range options {
		env = setEnv(env, key, value)
	}
	return env
}

func newScriptFixture(t *testing.T, withSpaces, precreateSharedTmp bool) *scriptFixture {
	t.Helper()
	container := t.TempDir()
	name := "fixture-repo"
	if withSpaces {
		name = "fixture repo with spaces"
	}
	root := filepath.Join(container, name)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	source, err := os.ReadFile(filepath.Join(repositoryRoot(t), "scripts", "recompute-evidence.sh"))
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "scripts", "recompute-evidence.sh")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, source, 0o755); err != nil {
		t.Fatal(err)
	}

	fakeBin := filepath.Join(root, "fake-bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeGo := strings.ReplaceAll(fakeGoSource, "@@FAKE_SHA@@", strings.Repeat("a", 64))
	if err := os.WriteFile(filepath.Join(fakeBin, "go"), []byte(fakeGo), 0o755); err != nil {
		t.Fatal(err)
	}

	tmpRoot := filepath.Join(root, "runs", ".tmp")
	if precreateSharedTmp {
		if err := os.MkdirAll(tmpRoot, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	record := filepath.Join(root, "fake-go-record.txt")
	env := append([]string(nil), os.Environ()...)
	env = setEnv(env, "PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	env = setEnv(env, "FAKE_GO_RECORD", record)
	env = setEnv(env, "FAKE_GO_GOOS", runtime.GOOS)
	env = setEnv(env, "FAKE_GO_GOARCH", runtime.GOARCH)

	return &scriptFixture{
		root:    root,
		script:  script,
		record:  record,
		tmpRoot: tmpRoot,
		baseEnv: env,
	}
}

func commandResult(t *testing.T, dir, program string, args []string, env []string) scriptResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Dir = dir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("%s timed out: %s", program, ctx.Err())
	}
	return scriptResult{exit: exitCode(err), output: string(output)}
}

func (f *scriptFixture) scriptArgs(output string) []string {
	args := []string{f.script, "--scale", "small"}
	if output != "" {
		args = append(args, "--out", output)
	}
	return args
}

func (f *scriptFixture) run(t *testing.T, output string, options map[string]string) scriptResult {
	t.Helper()
	return commandResult(t, f.root, "bash", f.scriptArgs(output), f.env(options))
}

func (f *scriptFixture) startForSignal(t *testing.T, output string, options map[string]string) (*exec.Cmd, <-chan scriptResult, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	cmd := exec.CommandContext(ctx, "bash", f.scriptArgs(output)...)
	cmd.Dir = f.root
	cmd.Env = f.env(options)
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start script: %v", err)
		return nil, nil, cancel
	}
	results := make(chan scriptResult, 1)
	go func() {
		err := cmd.Wait()
		results <- scriptResult{exit: exitCode(err), output: combined.String()}
	}()
	return cmd, results, cancel
}

func waitForResult(results <-chan scriptResult, timeout time.Duration) (scriptResult, error) {
	select {
	case result := <-results:
		return result, nil
	case <-time.After(timeout):
		return scriptResult{}, fmt.Errorf("script did not finish within %s", timeout)
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func directoryEntries(t *testing.T, path string) map[string]struct{} {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	result := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		result[entry.Name()] = struct{}{}
	}
	return result
}

func assertEntriesUnchanged(t *testing.T, path string, before map[string]struct{}) {
	t.Helper()
	after := directoryEntries(t, path)
	if len(after) != len(before) {
		t.Fatalf("runs/.tmp entries changed: before=%v after=%v", before, after)
	}
	for name := range before {
		if _, ok := after[name]; !ok {
			t.Fatalf("runs/.tmp entry %q was removed", name)
		}
	}
}

func pathWithin(child, parent string) bool {
	childAbs, err := filepath.Abs(child)
	if err != nil {
		return false
	}
	parentAbs, err := filepath.Abs(parent)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(parentAbs, childAbs)
	if err != nil || filepath.IsAbs(rel) || rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func assertObservedBinary(t *testing.T, f *scriptFixture) string {
	t.Helper()
	record, err := os.ReadFile(f.record)
	if err != nil {
		t.Fatal(err)
	}
	observed := strings.TrimSpace(string(record))
	if observed == "" {
		t.Fatal("fake go did not record a binary path")
	}
	if !pathWithin(observed, f.tmpRoot) {
		t.Fatalf("fake go binary path %q is outside %q", observed, f.tmpRoot)
	}
	workDir := filepath.Dir(observed)
	if filepath.Dir(workDir) != f.tmpRoot {
		t.Fatalf("temporary work directory %q is not a direct child of %q", workDir, f.tmpRoot)
	}
	if !strings.HasPrefix(filepath.Base(workDir), "recompute-rss.") {
		t.Fatalf("temporary work directory %q has no recompute-rss. prefix", workDir)
	}
	return observed
}

func assertOutput(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.IsDir() {
		t.Fatalf("output %q is a directory", path)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) == 0 {
		t.Fatalf("output %q is empty", path)
	}
}

func assertNonzero(t *testing.T, result scriptResult) {
	t.Helper()
	if result.exit == 0 {
		t.Fatalf("expected script failure, output=%q", result.output)
	}
}

func writeSiblingFixture(t *testing.T, f *scriptFixture) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.tmpRoot, "sibling.txt"), []byte("keep sibling\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(f.tmpRoot, "sibling-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.tmpRoot, "sibling-dir", "keep.txt"), []byte("keep nested\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertSiblingContents(t *testing.T, f *scriptFixture) {
	t.Helper()
	sibling, err := os.ReadFile(filepath.Join(f.tmpRoot, "sibling.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(sibling) != "keep sibling\n" {
		t.Fatalf("sibling file changed: %q", sibling)
	}
	nested, err := os.ReadFile(filepath.Join(f.tmpRoot, "sibling-dir", "keep.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(nested) != "keep nested\n" {
		t.Fatalf("nested sibling file changed: %q", nested)
	}
}

func waitForFile(path string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, err := os.Stat(path)
	return err == nil
}

func TestRecomputeEvidenceTempPaths(t *testing.T) {
	t.Run("success_empty_shared_tmp", func(t *testing.T) {
		f := newScriptFixture(t, false, true)
		before := directoryEntries(t, f.tmpRoot)
		result := f.run(t, "", nil)
		if result.exit != 0 {
			t.Fatalf("script failed: output=%q", result.output)
		}
		assertObservedBinary(t, f)
		output := filepath.Join(f.root, "evidence", "LRN-02", "rss.json")
		assertOutput(t, output)
		evidence, err := os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(evidence), `"schema_version": "coimnet-recompute-rss/v1"`) {
			t.Fatalf("default evidence output has no recompute RSS schema: %q", evidence)
		}
		assertEntriesUnchanged(t, f.tmpRoot, before)
	})

	t.Run("success_spaces_preserves_siblings_and_legacy", func(t *testing.T) {
		f := newScriptFixture(t, true, true)
		writeSiblingFixture(t, f)
		legacy := filepath.Join(f.root, ".recompute-work")
		if err := os.MkdirAll(legacy, 0o755); err != nil {
			t.Fatal(err)
		}
		legacySentinel := filepath.Join(legacy, "legacy-sentinel")
		if err := os.WriteFile(legacySentinel, []byte("preserve legacy\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		before := directoryEntries(t, f.tmpRoot)
		output := filepath.Join(f.root, "result.json")
		result := f.run(t, output, nil)
		if result.exit != 0 {
			t.Fatalf("script failed: output=%q", result.output)
		}
		assertObservedBinary(t, f)
		assertOutput(t, output)
		assertEntriesUnchanged(t, f.tmpRoot, before)
		assertSiblingContents(t, f)
		got, err := os.ReadFile(legacySentinel)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "preserve legacy\n" {
			t.Fatalf("legacy sentinel changed: %q", got)
		}
	})

	for _, tc := range []struct {
		name    string
		options map[string]string
	}{
		{name: "build_failure", options: map[string]string{"FAKE_GO_BUILD_FAIL": "1"}},
		{name: "helper_failure", options: map[string]string{"FAKE_GO_HELPER_FAIL": "lif:recompute"}},
	} {
		t.Run(tc.name+"_preserves_output_and_siblings", func(t *testing.T) {
			f := newScriptFixture(t, false, true)
			writeSiblingFixture(t, f)
			before := directoryEntries(t, f.tmpRoot)
			output := filepath.Join(f.root, "result.json")
			if err := os.WriteFile(output, []byte("preexisting output\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			result := f.run(t, output, tc.options)
			assertNonzero(t, result)
			assertObservedBinary(t, f)
			got, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != "preexisting output\n" {
				t.Fatalf("preexisting output changed: %q", got)
			}
			assertEntriesUnchanged(t, f.tmpRoot, before)
			assertSiblingContents(t, f)
		})
	}

	t.Run("term_during_build_releases_fixture_child", func(t *testing.T) {
		f := newScriptFixture(t, false, true)
		writeSiblingFixture(t, f)
		before := directoryEntries(t, f.tmpRoot)
		output := filepath.Join(f.root, "result.json")
		if err := os.WriteFile(output, []byte("preexisting output\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		started := filepath.Join(f.root, "fake-build-started")
		release := filepath.Join(f.root, "fake-build-release")
		finished := filepath.Join(f.root, "fake-build-finished")
		cmd, results, cancel := f.startForSignal(t, output, map[string]string{
			"FAKE_GO_BUILD_WAIT": "1",
			"FAKE_GO_STARTED":    started,
			"FAKE_GO_RELEASE":    release,
			"FAKE_GO_FINISHED":   finished,
		})
		defer cancel()
		if !waitForFile(started, 5*time.Second) {
			_ = os.WriteFile(release, nil, 0o644)
			cancel()
			_, _ = waitForResult(results, 2*time.Second)
			t.Fatal("fake go build did not reach its controlled wait")
		}
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			_ = os.WriteFile(release, nil, 0o644)
			cancel()
			_, _ = waitForResult(results, 2*time.Second)
			t.Fatal(err)
		}
		if err := os.WriteFile(release, nil, 0o644); err != nil {
			cancel()
			_, _ = waitForResult(results, 2*time.Second)
			t.Fatal(err)
		}
		result, err := waitForResult(results, 5*time.Second)
		if err != nil {
			cancel()
			_, _ = waitForResult(results, 2*time.Second)
			t.Fatal(err)
		}
		cancel()
		if result.exit != 143 {
			t.Fatalf("TERM exit code = %d, want 143; output=%q", result.exit, result.output)
		}
		if !waitForFile(finished, 2*time.Second) {
			t.Fatal("controlled fake go build did not finish after release")
		}
		got, err := os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "preexisting output\n" {
			t.Fatalf("preexisting output changed: %q", got)
		}
		assertObservedBinary(t, f)
		assertEntriesUnchanged(t, f.tmpRoot, before)
		assertSiblingContents(t, f)
	})

	t.Run("help_mentions_shared_tmp", func(t *testing.T) {
		f := newScriptFixture(t, false, true)
		result := commandResult(t, f.root, "bash", []string{f.script, "--help"}, f.env(nil))
		if result.exit != 0 {
			t.Fatalf("--help failed: output=%q", result.output)
		}
		help := result.output
		if !strings.Contains(help, "runs/.tmp") {
			t.Fatalf("--help does not describe runs/.tmp: %q", help)
		}
		if strings.Contains(help, ".recompute-work") {
			t.Fatalf("--help retains stale .recompute-work description: %q", help)
		}
	})
}

func extractResourceSetup(t *testing.T) string {
	t.Helper()
	doc, err := os.ReadFile(filepath.Join(repositoryRoot(t), "docs", "resources.md"))
	if err != nil {
		t.Fatal(err)
	}
	sectionStart := strings.Index(string(doc), "## 重現局部量測")
	if sectionStart < 0 {
		t.Fatal("resource measurement section not found")
	}
	section := string(doc)[sectionStart:]
	fenceStart := strings.Index(section, "```sh")
	if fenceStart < 0 {
		t.Fatal("resource measurement shell block not found")
	}
	blockStart := fenceStart + len("```sh")
	fenceEnd := strings.Index(section[blockStart:], "```")
	if fenceEnd < 0 {
		t.Fatal("resource measurement shell block is unterminated")
	}
	block := section[blockStart : blockStart+fenceEnd]
	var setup []string
	seenRunDir := false
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "/usr/bin/time") {
			break
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			setup = append(setup, line)
			continue
		}
		if strings.HasPrefix(trimmed, "mkdir ") || strings.HasPrefix(trimmed, "run_dir=") {
			setup = append(setup, line)
			if strings.HasPrefix(trimmed, "run_dir=") {
				seenRunDir = true
			}
			continue
		}
		t.Fatalf("unsupported command before resource measurement: %q", trimmed)
	}
	if !seenRunDir {
		t.Fatal("resource measurement setup has no run_dir assignment")
	}
	return strings.TrimSpace(strings.Join(setup, "\n"))
}

func TestResourceMeasurementExampleTempPath(t *testing.T) {
	f := newScriptFixture(t, false, false)
	setup := extractResourceSetup(t)
	command := setup + "\nprintf '%s\\n' \"$run_dir\"\n"
	privateTmp := filepath.Join(t.TempDir(), "docs-tmp")
	if err := os.MkdirAll(privateTmp, 0o755); err != nil {
		t.Fatal(err)
	}
	env := setEnv(append([]string(nil), os.Environ()...), "PWD", f.root)
	env = setEnv(env, "TMPDIR", privateTmp)
	result := commandResult(t, f.root, "sh", []string{"-c", command}, env)
	if result.exit != 0 {
		t.Fatalf("document setup failed: output=%q", result.output)
	}
	runDir := strings.TrimSpace(result.output)
	if runDir == "" {
		t.Fatal("document setup did not return run_dir")
	}
	if !pathWithin(runDir, f.tmpRoot) {
		t.Fatalf("document run_dir %q is outside %q", runDir, f.tmpRoot)
	}
	info, err := os.Stat(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("document run_dir %q is not a directory", runDir)
	}
	tmpInfo, err := os.Stat(f.tmpRoot)
	if err != nil {
		t.Fatalf("document setup did not create runs/.tmp: %v", err)
	}
	if !tmpInfo.IsDir() {
		t.Fatalf("document runs/.tmp %q is not a directory", f.tmpRoot)
	}
	entries := directoryEntries(t, f.tmpRoot)
	if len(entries) != 1 {
		t.Fatalf("document setup created unexpected runs/.tmp entries: %v", entries)
	}
	if _, ok := entries[filepath.Base(runDir)]; !ok {
		t.Fatalf("document run_dir %q is not the only runs/.tmp entry: %v", runDir, entries)
	}
}
