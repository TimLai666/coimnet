//go:build darwin || linux

package coimnet_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const operationalGit = `#!/bin/sh
set -eu
case "${1-}" in
rev-parse)
  case "${2-}" in
  --show-toplevel) printf '%s\n' "${FAKE_GIT_ROOT:?}" ;;
  HEAD) printf '%s\n' "${FAKE_GIT_HEAD:?}" ;;
  *) exit 42 ;;
  esac
  ;;
-C)
  shift 2
  if [ "${1-}" != archive ]; then exit 42; fi
  printf '%s\n' 41 > "${FAKE_GIT_ARCHIVE_MARKER:?}"
  printf '%s\n' 'fake git archive failure' >&2
  exit 41
  ;;
*) exit 42 ;;
esac
`

const operationalMktemp = `#!/bin/sh
set -eu
: > "${FAKE_MKTEMP_ARGS:?}"
for arg in "$@"; do printf '%s\n' "$arg" >> "${FAKE_MKTEMP_ARGS:?}"; done
if [ "$#" -eq 1 ] && [ "$1" = -d ]; then
  exec /usr/bin/mktemp -d "${FAKE_MKTEMP_FALLBACK:?}/legacy.XXXXXX"
fi
exec /usr/bin/mktemp "$@"
`

const operationalGo = `#!/bin/sh
set -eu
: > "${FAKE_GO_MARKER:?}"
output=
want_output=0
for arg in "$@"; do
  if [ "$want_output" = 1 ]; then output=$arg; break; fi
  if [ "$arg" = -o ]; then want_output=1; fi
done
if [ -z "$output" ]; then exit 2; fi
cat > "$output" <<'FAKE_CLI'
#!/bin/sh
set -eu
if [ "${1-}" = doctor ]; then
  printf '%s\n' '{"status":"ok"}'
  exit 0
fi
if [ "${1-}" = data ] && [ "${2-}" = import ]; then
  temp=
  want_temp=0
  for arg in "$@"; do
    if [ "$want_temp" = 1 ]; then temp=$arg; want_temp=0; continue; fi
    if [ "$arg" = --temp-dir ]; then want_temp=1; fi
  done
  printf '%s\n' "$temp" > "${FAKE_GRAPH_TEMP_RECORD:?}"
  if [ "${FAKE_GRAPH_NONEMPTY:-0}" = 1 ]; then
    printf '%s\n' 'leftover sentinel' > "$temp/leftover"
  fi
  printf '%s\n' '{"status":"imported"}'
  exit "${FAKE_GRAPH_IMPORT_STATUS:-0}"
fi
if [ "${1-}" = data ] && [ "${2-}" = validate ]; then
  printf '%s\n' '{"status":"validated"}'
  exit 0
fi
exit 2
FAKE_CLI
chmod +x "$output"
`

type operationalFixture struct {
	root          string
	clean         string
	graph         string
	tempRoot      string
	mktempArgs    string
	archiveMarker string
	goMarker      string
	graphTemp     string
	baseEnv       []string
}

func newOperationalFixture(t *testing.T, withSpaces bool) *operationalFixture {
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
	probeRoot := repositoryRoot(t)
	for _, name := range []string{"clean-env-verify.sh", "graph-evidence.sh"} {
		source, err := os.ReadFile(filepath.Join(probeRoot, "scripts", name))
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(root, "scripts", name)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, source, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifests", "malecns-v1.0.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeBin := filepath.Join(root, "fake-bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"git":    operationalGit,
		"mktemp": operationalMktemp,
		"go":     operationalGo,
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(fakeBin, name), []byte(source), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	tempRoot := filepath.Join(root, "runs", ".tmp")
	markerRoot := filepath.Join(root, "fixture-markers")
	if err := os.MkdirAll(markerRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	mktempFallback := filepath.Join(markerRoot, "mktemp-fallback")
	if err := os.MkdirAll(mktempFallback, 0o755); err != nil {
		t.Fatal(err)
	}
	f := &operationalFixture{
		root:          root,
		clean:         filepath.Join(root, "scripts", "clean-env-verify.sh"),
		graph:         filepath.Join(root, "scripts", "graph-evidence.sh"),
		tempRoot:      tempRoot,
		mktempArgs:    filepath.Join(markerRoot, "mktemp.args"),
		archiveMarker: filepath.Join(markerRoot, "archive.status"),
		goMarker:      filepath.Join(markerRoot, "go.invoked"),
		graphTemp:     filepath.Join(markerRoot, "graph-temp-dir"),
	}
	env := append([]string(nil), os.Environ()...)
	env = setEnv(env, "PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	env = setEnv(env, "FAKE_GIT_ROOT", root)
	env = setEnv(env, "FAKE_GIT_HEAD", "0123456789abcdef0123456789abcdef01234567")
	env = setEnv(env, "FAKE_GIT_ARCHIVE_MARKER", f.archiveMarker)
	env = setEnv(env, "FAKE_MKTEMP_ARGS", f.mktempArgs)
	env = setEnv(env, "FAKE_MKTEMP_FALLBACK", mktempFallback)
	env = setEnv(env, "FAKE_GO_MARKER", f.goMarker)
	env = setEnv(env, "FAKE_GRAPH_TEMP_RECORD", f.graphTemp)
	f.baseEnv = env
	return f
}

func (f *operationalFixture) env(options map[string]string) []string {
	env := append([]string(nil), f.baseEnv...)
	for key, value := range options {
		env = setEnv(env, key, value)
	}
	return env
}

func (f *operationalFixture) cleanRun(t *testing.T, args []string, options map[string]string) scriptResult {
	t.Helper()
	command := append([]string{f.clean}, args...)
	return commandResult(t, f.root, "bash", command, f.env(options))
}

func (f *operationalFixture) graphRun(t *testing.T, output, store string, options map[string]string) scriptResult {
	t.Helper()
	args := []string{f.graph, output}
	if store != "" {
		args = append(args, store)
	}
	return commandResult(t, f.root, "bash", args, f.env(options))
}

func writeOperationalSiblings(t *testing.T, f *operationalFixture) {
	t.Helper()
	if err := os.MkdirAll(f.tempRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.tempRoot, "sibling.txt"), []byte("keep sibling\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(f.tempRoot, "sibling-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.tempRoot, "sibling-dir", "keep.txt"), []byte("keep nested\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertOperationalSiblingContents(t *testing.T, f *operationalFixture) {
	t.Helper()
	for path, want := range map[string]string{
		filepath.Join(f.tempRoot, "sibling.txt"):             "keep sibling\n",
		filepath.Join(f.tempRoot, "sibling-dir", "keep.txt"): "keep nested\n",
	} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("sibling %q changed: %q", path, got)
		}
	}
}

func readOperationalValue(t *testing.T, path string) string {
	t.Helper()
	value, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(value))
}

func readOperationalLines(t *testing.T, path string) []string {
	t.Helper()
	value, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimSuffix(string(value), "\n"), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func printedOperationalValue(output, prefix string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func assertOperationalWorkspace(t *testing.T, path, tempRoot, prefix string, wantPresent bool) {
	t.Helper()
	if !pathWithin(path, tempRoot) || filepath.Dir(path) != tempRoot || !strings.HasPrefix(filepath.Base(path), prefix) {
		t.Fatalf("workspace %q is not a direct %s child of %q", path, prefix, tempRoot)
	}
	_, err := os.Stat(path)
	if wantPresent && err != nil {
		t.Fatalf("workspace %q is not retained: %v", path, err)
	}
	if !wantPresent && !os.IsNotExist(err) {
		t.Fatalf("workspace %q was not removed: %v", path, err)
	}
}

func assertOperationalNoPath(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("unexpected path exists: %s", path)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func assertOperationalFailure(t *testing.T, result scriptResult) {
	t.Helper()
	if result.exit == 0 {
		t.Fatalf("expected nonzero exit, output=%q", result.output)
	}
}

func assertOperationalOutput(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("output %q changed: %q", path, got)
	}
}

func extractREADMESetupFrom(t *testing.T, relativePath, assignment string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), relativePath))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for offset := 0; offset < len(text); {
		start := strings.Index(text[offset:], "```sh")
		if start < 0 {
			break
		}
		start += offset
		body := start + len("```sh")
		if body < len(text) && text[body] == '\r' {
			body++
		}
		if body < len(text) && text[body] == '\n' {
			body++
		}
		end := strings.Index(text[body:], "```")
		if end < 0 {
			t.Fatalf("README shell block starting at byte %d is unterminated", start)
		}
		end += body
		block := text[body:end]
		if !strings.Contains(block, assignment+"=") {
			offset = end + len("```")
			continue
		}

		var setup []string
		seenAssignment := false
		for _, line := range strings.Split(block, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				if !seenAssignment {
					setup = append(setup, line)
				}
				continue
			}
			if strings.HasPrefix(trimmed, "mkdir ") && !seenAssignment {
				setup = append(setup, line)
				continue
			}
			if strings.HasPrefix(trimmed, assignment+"=") && !seenAssignment {
				setup = append(setup, line)
				seenAssignment = true
				break
			}
			// Keep scanning, but never execute unrelated README commands such as
			// training or data-source/download commands.
		}
		if seenAssignment {
			return strings.TrimSpace(strings.Join(setup, "\n"))
		}
		offset = end + len("```")
	}
	t.Fatalf("README has no sh block assigning %s", assignment)
	return ""
}

func extractREADMESetup(t *testing.T, assignment string) string {
	t.Helper()
	return extractREADMESetupFrom(t, "README.md", assignment)
}

func runREADMESetup(t *testing.T, f *operationalFixture, setup, variable string) scriptResult {
	t.Helper()
	command := setup + "\nprintf '%s\\n' \"$" + variable + "\"\n"
	env := f.env(nil)
	env = setEnv(env, "PWD", f.root)
	env = setEnv(env, "TMPDIR", t.TempDir())
	return commandResult(t, f.root, "bash", []string{"-c", command}, env)
}

func TestOperationalScriptsProjectTemp(t *testing.T) {
	t.Run("clean_default_spaces_archive_failure", func(t *testing.T) {
		f := newOperationalFixture(t, true)
		output := filepath.Join(f.root, "result.json")
		if err := os.WriteFile(output, []byte("preexisting result\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		result := f.cleanRun(t, []string{"--out", output}, nil)
		assertOperationalFailure(t, result)
		assertOperationalOutput(t, output, "preexisting result\n")
		workdir := printedOperationalValue(result.output, "workdir:")
		if workdir == "" {
			t.Fatalf("clean-env output did not print workdir: %q", result.output)
		}
		assertOperationalWorkspace(t, workdir, f.tempRoot, "clean-env.", true)
		if readOperationalValue(t, f.archiveMarker) != "41" {
			t.Fatal("fake git archive was not invoked with exit 41")
		}
		assertOperationalNoPath(t, f.goMarker)
		args := readOperationalLines(t, f.mktempArgs)
		if len(args) != 2 || args[0] != "-d" || !strings.HasPrefix(args[1], filepath.Join(f.tempRoot, "clean-env.")) {
			t.Fatalf("clean-env did not pass project temp template: %v", args)
		}
	})

	t.Run("clean_custom_workspace_preserved", func(t *testing.T) {
		f := newOperationalFixture(t, false)
		custom := filepath.Join(f.root, "custom workspace")
		if err := os.MkdirAll(custom, 0o755); err != nil {
			t.Fatal(err)
		}
		sentinel := filepath.Join(custom, "sentinel.txt")
		if err := os.WriteFile(sentinel, []byte("keep custom\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		output := filepath.Join(f.root, "result.json")
		if err := os.WriteFile(output, []byte("preexisting result\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		result := f.cleanRun(t, []string{"--workdir", custom, "--out", output}, nil)
		assertOperationalFailure(t, result)
		if got := printedOperationalValue(result.output, "workdir:"); got != custom {
			t.Fatalf("printed custom workdir = %q, want %q", got, custom)
		}
		assertOperationalOutput(t, sentinel, "keep custom\n")
		assertOperationalOutput(t, output, "preexisting result\n")
		assertOperationalNoPath(t, f.tempRoot)
		assertOperationalNoPath(t, f.mktempArgs)
		assertOperationalNoPath(t, f.goMarker)
	})

	t.Run("graph_success_empty_workspace", func(t *testing.T) {
		f := newOperationalFixture(t, false)
		writeOperationalSiblings(t, f)
		before := directoryEntries(t, f.tempRoot)
		output := filepath.Join(f.root, "graph-output")
		result := f.graphRun(t, output, "", nil)
		if result.exit != 0 {
			t.Fatalf("graph script failed: %q", result.output)
		}
		temp := readOperationalValue(t, filepath.Join(output, "temp-dir.txt"))
		if got := readOperationalValue(t, f.graphTemp); got != temp {
			t.Fatalf("fake CLI received temp dir %q, want %q", got, temp)
		}
		assertOperationalWorkspace(t, temp, f.tempRoot, "graph-evidence.", false)
		assertEntriesUnchanged(t, f.tempRoot, before)
		assertOperationalSiblingContents(t, f)
		for _, name := range []string{"binary.sha256", "manifest.sha256", "doctor.json", "temp-dir.txt", "command.txt", "report.json", "started-at.txt", "finished-at.txt", "run.log", "temp-dir-after.txt"} {
			if _, err := os.Stat(filepath.Join(output, name)); err != nil {
				t.Fatalf("missing graph artifact %s: %v", name, err)
			}
		}
		args := readOperationalLines(t, f.mktempArgs)
		if len(args) != 2 || args[0] != "-d" || !strings.HasPrefix(args[1], filepath.Join(f.tempRoot, "graph-evidence.")) {
			t.Fatalf("graph did not pass project temp template: %v", args)
		}
	})

	for _, tc := range []struct {
		name     string
		nonempty string
		wantKeep bool
	}{
		{name: "graph_failure_empty_workspace", nonempty: "0", wantKeep: false},
		{name: "graph_failure_nonempty_workspace", nonempty: "1", wantKeep: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newOperationalFixture(t, false)
			writeOperationalSiblings(t, f)
			before := directoryEntries(t, f.tempRoot)
			output := filepath.Join(f.root, "graph-output")
			result := f.graphRun(t, output, "", map[string]string{"FAKE_GRAPH_IMPORT_STATUS": "23", "FAKE_GRAPH_NONEMPTY": tc.nonempty})
			assertOperationalFailure(t, result)
			temp := readOperationalValue(t, filepath.Join(output, "temp-dir.txt"))
			if got := readOperationalValue(t, f.graphTemp); got != temp {
				t.Fatalf("fake CLI received temp dir %q, want %q", got, temp)
			}
			assertOperationalWorkspace(t, temp, f.tempRoot, "graph-evidence.", tc.wantKeep)
			if tc.wantKeep {
				assertOperationalOutput(t, filepath.Join(temp, "leftover"), "leftover sentinel\n")
				if !strings.Contains(readOperationalValue(t, filepath.Join(output, "temp-dir-after.txt")), "temporary directory not empty; kept for inspection") {
					t.Fatal("nonempty graph workspace did not record kept-for-inspection message")
				}
			} else {
				if strings.Contains(readOperationalValue(t, filepath.Join(output, "temp-dir-after.txt")), "not empty") {
					t.Fatal("empty graph workspace was reported nonempty")
				}
			}
			if !strings.Contains(readOperationalValue(t, filepath.Join(output, "run.log")), "exit=23") {
				t.Fatal("graph failure exit was not recorded")
			}
			if tc.wantKeep {
				expected := make(map[string]struct{}, len(before)+1)
				for name := range before {
					expected[name] = struct{}{}
				}
				expected[filepath.Base(temp)] = struct{}{}
				assertEntriesUnchanged(t, f.tempRoot, expected)
			} else {
				assertEntriesUnchanged(t, f.tempRoot, before)
			}
			assertOperationalSiblingContents(t, f)
		})
	}

	t.Run("both_help_describe_project_temp", func(t *testing.T) {
		f := newOperationalFixture(t, false)
		clean := commandResult(t, f.root, "bash", []string{f.clean, "--help"}, f.env(nil))
		graph := commandResult(t, f.root, "bash", []string{f.graph, "--help"}, f.env(nil))
		if clean.exit != 0 || graph.exit != 0 {
			t.Fatalf("help failed: clean=%d graph=%d", clean.exit, graph.exit)
		}
		for name, output := range map[string]string{"clean-env": clean.output, "graph-evidence": graph.output} {
			if !strings.Contains(output, "runs/.tmp") {
				t.Fatalf("%s help omits runs/.tmp: %q", name, output)
			}
		}
		assertOperationalNoPath(t, f.tempRoot)
		assertOperationalNoPath(t, f.goMarker)
		assertOperationalNoPath(t, f.mktempArgs)
	})

	t.Run("graph_existing_output_or_store_refused", func(t *testing.T) {
		t.Run("existing_output", func(t *testing.T) {
			f := newOperationalFixture(t, false)
			output := filepath.Join(f.root, "existing-output")
			if err := os.MkdirAll(output, 0o755); err != nil {
				t.Fatal(err)
			}
			sentinel := filepath.Join(output, "sentinel.txt")
			if err := os.WriteFile(sentinel, []byte("keep output\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			result := f.graphRun(t, output, "", nil)
			assertOperationalFailure(t, result)
			assertOperationalOutput(t, sentinel, "keep output\n")
			assertOperationalNoPath(t, f.goMarker)
			assertOperationalNoPath(t, f.tempRoot)
		})
		t.Run("existing_store", func(t *testing.T) {
			f := newOperationalFixture(t, false)
			output := filepath.Join(f.root, "new-output")
			store := filepath.Join(f.root, "existing.store")
			if err := os.WriteFile(store, []byte("keep store\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			result := f.graphRun(t, output, store, nil)
			assertOperationalFailure(t, result)
			assertOperationalOutput(t, store, "keep store\n")
			assertOperationalNoPath(t, output)
			assertOperationalNoPath(t, f.goMarker)
			assertOperationalNoPath(t, f.tempRoot)
		})
	})

	t.Run("readme_delayed_training_setup", func(t *testing.T) {
		f := newOperationalFixture(t, false)
		setup := extractREADMESetup(t, "run_dir")
		result := runREADMESetup(t, f, setup, "run_dir")
		if result.exit != 0 {
			t.Fatalf("README delayed setup failed: %q", result.output)
		}
		runDir := strings.TrimSpace(result.output)
		parent := filepath.Join(f.root, "runs")
		if _, err := os.Stat(parent); err != nil {
			t.Fatalf("README setup did not create %s: %v", parent, err)
		}
		assertOperationalWorkspace(t, runDir, parent, "delayed-example.", true)
		assertOperationalNoPath(t, f.goMarker)
		args := readOperationalLines(t, f.mktempArgs)
		if len(args) != 2 || args[0] != "-d" || !strings.HasPrefix(args[1], filepath.Join(parent, "delayed-example.")) {
			t.Fatalf("README delayed setup did not pass project template: %v", args)
		}
	})

	t.Run("readme_download_setup", func(t *testing.T) {
		f := newOperationalFixture(t, false)
		setup := extractREADMESetup(t, "data_dir")
		result := runREADMESetup(t, f, setup, "data_dir")
		if result.exit != 0 {
			t.Fatalf("README download setup failed: %q", result.output)
		}
		dataDir := strings.TrimSpace(result.output)
		parent := filepath.Join(f.root, "data", "downloads")
		if _, err := os.Stat(parent); err != nil {
			t.Fatalf("README setup did not create %s: %v", parent, err)
		}
		assertOperationalWorkspace(t, dataDir, parent, "malecns.", true)
		assertOperationalNoPath(t, f.goMarker)
		args := readOperationalLines(t, f.mktempArgs)
		if len(args) != 2 || args[0] != "-d" || !strings.HasPrefix(args[1], filepath.Join(parent, "malecns.")) {
			t.Fatalf("README download setup did not pass project template: %v", args)
		}
	})

	t.Run("readme_realsubgraph_setup", func(t *testing.T) {
		f := newOperationalFixture(t, false)
		setup := extractREADMESetupFrom(t, filepath.Join("examples", "realsubgraph", "README.md"), "run_dir")
		result := runREADMESetup(t, f, setup, "run_dir")
		if result.exit != 0 {
			t.Fatalf("real-subgraph README setup failed: %q", result.output)
		}
		runDir := strings.TrimSpace(result.output)
		parent := filepath.Join(f.root, "runs")
		if _, err := os.Stat(parent); err != nil {
			t.Fatalf("README setup did not create %s: %v", parent, err)
		}
		assertOperationalWorkspace(t, runDir, parent, "realsubgraph.", true)
		assertOperationalNoPath(t, f.goMarker)
		args := readOperationalLines(t, f.mktempArgs)
		if len(args) != 2 || args[0] != "-d" || !strings.HasPrefix(args[1], filepath.Join(parent, "realsubgraph.")) {
			t.Fatalf("real-subgraph README setup did not pass project template: %v", args)
		}
	})
}
