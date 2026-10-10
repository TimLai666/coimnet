package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func localRunsRepository(t *testing.T) (root, dataPath, runsPath string) {
	t.Helper()
	root = t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatalf("create fake repository marker: %v", err)
	}
	dataDir := filepath.Join(root, "data")
	if err := os.Mkdir(dataDir, 0o700); err != nil {
		t.Fatalf("create data directory: %v", err)
	}
	dataPath = filepath.Join(dataDir, "source.csv")
	if err := os.WriteFile(dataPath, []byte("source\n"), 0o600); err != nil {
		t.Fatalf("write source data: %v", err)
	}
	return root, dataPath, filepath.Join(root, "runs")
}

func localRunsAssertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected path changed filesystem: %s (err=%v)", path, err)
	}
}

func localRunsAssertCreatedDirectory(t *testing.T, path string) string {
	t.Helper()
	got, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve expected output path: %v", err)
	}
	info, err := os.Stat(got)
	if err != nil {
		t.Fatalf("created output directory %q is missing: %v", got, err)
	}
	if !info.IsDir() {
		t.Fatalf("created output path %q is not a directory", got)
	}
	return got
}

func TestLocalRunsAcceptsNewProjectAndExternalOutputs(t *testing.T) {
	root, dataPath, _ := localRunsRepository(t)
	projectOutput := filepath.Join(root, "runs", "nested", "result")
	got, err := prepareOutputDirectory(projectOutput, dataPath)
	if err != nil {
		t.Fatalf("new output under repository runs was rejected: %v", err)
	}
	want := localRunsAssertCreatedDirectory(t, projectOutput)
	if got != want {
		t.Fatalf("project output = %q, want %q", got, want)
	}
	entries, err := os.ReadDir(filepath.Dir(got))
	if err != nil {
		t.Fatalf("read new output parent: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(got) {
		t.Fatalf("new output parent entries = %v, want only %q", entries, filepath.Base(got))
	}
	if _, err := prepareOutputDirectory(projectOutput, dataPath); err == nil {
		t.Fatal("existing project output was accepted")
	}

	externalOutput := filepath.Join(t.TempDir(), "result")
	got, err = prepareOutputDirectory(externalOutput, dataPath)
	if err != nil {
		t.Fatalf("existing external output support was lost: %v", err)
	}
	want = localRunsAssertCreatedDirectory(t, externalOutput)
	if got != want {
		t.Fatalf("external output = %q, want %q", got, want)
	}
}

func TestLocalRunsRejectsRepositoryBoundariesWithoutCreatingOutput(t *testing.T) {
	root, dataPath, runsPath := localRunsRepository(t)
	trackedPath := filepath.Join(root, "examples", "realnav")
	if err := os.MkdirAll(trackedPath, 0o700); err != nil {
		t.Fatalf("create tracked path fixture: %v", err)
	}
	cases := []struct {
		name       string
		output     string
		mustAbsent []string
	}{
		{
			name:       "tracked repository path",
			output:     filepath.Join(trackedPath, "result"),
			mustAbsent: []string{filepath.Join(trackedPath, "result")},
		},
		{
			name:   "repository root",
			output: root,
		},
		{
			name:       "runs root",
			output:     runsPath,
			mustAbsent: []string{runsPath},
		},
		{
			name:       "runs prefix sibling",
			output:     filepath.Join(root, "runs-extra", "result"),
			mustAbsent: []string{filepath.Join(root, "runs-extra"), filepath.Join(root, "runs-extra", "result")},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := prepareOutputDirectory(test.output, dataPath); err == nil {
				t.Fatalf("accepted repository boundary output %q", test.output)
			}
			for _, path := range test.mustAbsent {
				localRunsAssertAbsent(t, path)
			}
			if test.name == "repository root" {
				if info, err := os.Stat(root); err != nil || !info.IsDir() {
					t.Fatalf("repository root changed: info=%v err=%v", info, err)
				}
			}
		})
	}
}

func TestLocalRunsRejectsResolvedTrackedAndSourceOverlaps(t *testing.T) {
	root, dataPath, runsPath := localRunsRepository(t)
	if err := os.Mkdir(runsPath, 0o700); err != nil {
		t.Fatalf("create runs directory: %v", err)
	}
	trackedPath := filepath.Join(root, "examples")
	if err := os.MkdirAll(filepath.Join(trackedPath, "realnav"), 0o700); err != nil {
		t.Fatalf("create tracked source fixture: %v", err)
	}
	trackedAlias := filepath.Join(runsPath, "alias")
	if err := os.Symlink(filepath.Join("..", "examples"), trackedAlias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	trackedOutput := filepath.Join(trackedAlias, "result")
	if _, err := prepareOutputDirectory(trackedOutput, dataPath); err == nil {
		t.Fatal("accepted runs symlink resolving into tracked repository content")
	}
	localRunsAssertAbsent(t, trackedOutput)
	localRunsAssertAbsent(t, filepath.Join(trackedPath, "result"))
	if target, err := os.Readlink(trackedAlias); err != nil || target != filepath.Join("..", "examples") {
		t.Fatalf("tracked alias changed: target=%q err=%v", target, err)
	}

	sourceAlias := filepath.Join(runsPath, "data-alias")
	if err := os.Symlink(filepath.Join("..", "data"), sourceAlias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	sourceOutput := filepath.Join(sourceAlias, "derived", "result")
	if _, err := prepareOutputDirectory(sourceOutput, dataPath); err == nil {
		t.Fatal("accepted output overlapping the source data directory through runs")
	}
	localRunsAssertAbsent(t, sourceOutput)
	localRunsAssertAbsent(t, filepath.Join(root, "data", "derived", "result"))
	if target, err := os.Readlink(sourceAlias); err != nil || target != filepath.Join("..", "data") {
		t.Fatalf("source alias changed: target=%q err=%v", target, err)
	}
}

func TestLocalRunsRejectsExistingOutputsWithoutMutation(t *testing.T) {
	root, dataPath, runsPath := localRunsRepository(t)
	if err := os.Mkdir(runsPath, 0o700); err != nil {
		t.Fatalf("create runs directory: %v", err)
	}

	existingFile := filepath.Join(runsPath, "existing-file")
	fileContents := []byte("file sentinel\n")
	if err := os.WriteFile(existingFile, fileContents, 0o600); err != nil {
		t.Fatalf("create existing file: %v", err)
	}
	existingDirectory := filepath.Join(runsPath, "existing-directory")
	if err := os.Mkdir(existingDirectory, 0o700); err != nil {
		t.Fatalf("create existing directory: %v", err)
	}
	directoryMarker := filepath.Join(existingDirectory, "marker")
	directoryContents := []byte("directory sentinel\n")
	if err := os.WriteFile(directoryMarker, directoryContents, 0o600); err != nil {
		t.Fatalf("create existing directory marker: %v", err)
	}
	existingTarget := filepath.Join(runsPath, "existing-target")
	linkContents := []byte("link target sentinel\n")
	if err := os.WriteFile(existingTarget, linkContents, 0o600); err != nil {
		t.Fatalf("create existing symlink target: %v", err)
	}
	existingLink := filepath.Join(runsPath, "existing-link")
	if err := os.Symlink(filepath.Base(existingTarget), existingLink); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	cases := []struct {
		name  string
		path  string
		check func(t *testing.T)
	}{
		{
			name: "file",
			path: existingFile,
			check: func(t *testing.T) {
				got, err := os.ReadFile(existingFile)
				if err != nil || string(got) != string(fileContents) {
					t.Fatalf("existing file changed: got=%q err=%v", got, err)
				}
			},
		},
		{
			name: "directory",
			path: existingDirectory,
			check: func(t *testing.T) {
				got, err := os.ReadFile(directoryMarker)
				if err != nil || string(got) != string(directoryContents) {
					t.Fatalf("existing directory contents changed: got=%q err=%v", got, err)
				}
			},
		},
		{
			name: "symlink",
			path: existingLink,
			check: func(t *testing.T) {
				target, err := os.Readlink(existingLink)
				if err != nil || target != filepath.Base(existingTarget) {
					t.Fatalf("existing symlink changed: target=%q err=%v", target, err)
				}
				got, err := os.ReadFile(existingTarget)
				if err != nil || string(got) != string(linkContents) {
					t.Fatalf("existing symlink target changed: got=%q err=%v", got, err)
				}
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := prepareOutputDirectory(test.path, dataPath); err == nil {
				t.Fatalf("accepted existing output %q", test.path)
			}
			test.check(t)
		})
	}

	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("repository root changed while checking existing outputs: info=%v err=%v", info, err)
	}
}
