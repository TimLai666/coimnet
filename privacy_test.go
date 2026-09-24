package coimnet

// STA-06 (ticket 24, stage 3): by default the framework sends nothing
// anywhere.
//
//   - TestOnlyNetworkPackagesImportHTTP proves that only the network-facing
//     packages (download, teacher) may import net/http directly.
//   - TestNoTelemetry proves that no non-test Go file mentions telemetry or
//     analytics vendors at all (comments included).
//   - TestScanCommitScriptFlagsSecrets drives scripts/scan-commit.sh against
//     temporary git repositories and checks its exit codes and output.

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot walks up from the test working directory to the module root, the
// nearest directory containing go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		info, err := os.Stat(filepath.Join(dir, "go.mod"))
		if err == nil && !info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod above the test working directory")
		}
		dir = parent
	}
}

// runGit runs git in dir and fails the test with the combined output on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// newTempRepo creates a throwaway git repository in t.TempDir with a local
// identity, so staging and scanning never touch the real repository.
func newTempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.name", "CoImNet Privacy Test")
	runGit(t, dir, "config", "user.email", "privacy-test@coimnet.invalid")
	return dir
}

type scanResult struct {
	output string
	code   int
}

// runScan invokes bash scripts/scan-commit.sh with its working directory set
// to a temporary repository. SCAN_COMMIT_ROOT is passed as an environment
// override; the script falls back to the working directory.
func runScan(t *testing.T, script, dir string) scanResult {
	t.Helper()
	cmd := exec.Command("bash", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "SCAN_COMMIT_ROOT="+dir)
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run %s: %v", script, err)
		}
		code = ee.ExitCode()
	}
	return scanResult{output: string(out), code: code}
}

func TestOnlyNetworkPackagesImportHTTP(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "-f", `{{.ImportPath}} {{join .Imports " "}}`, "./...")
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps failed: %v\n%s", err, out)
	}
	// download fetches releases and teacher talks to remote teachers; internal/cli
	// is allowed only for the HEAD check of `data sources` (internal/cli/data.go),
	// a pre-existing deviation from the "download and teacher only" rule that
	// the STA-06 evidence records. No other package may reach the network.
	allowedHTTP := map[string]bool{
		"github.com/TimLai666/coimnet/download":     true,
		"github.com/TimLai666/coimnet/teacher":      true,
		"github.com/TimLai666/coimnet/internal/cli": true,
	}
	module := "github.com/TimLai666/coimnet"
	var offenders []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		pkg := fields[0]
		if pkg != module && !strings.HasPrefix(pkg, module+"/") {
			continue
		}
		for _, imp := range fields[1:] {
			if imp == "net/http" && !allowedHTTP[pkg] {
				offenders = append(offenders, pkg)
			}
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("packages outside the allow-list (download, teacher, internal/cli) import net/http directly:\n%s", strings.Join(offenders, "\n"))
	}
}

func TestNoTelemetry(t *testing.T) {
	root := repoRoot(t)
	patterns := []string{"analytics", "telemetry", "segment.io", "sentry", "mixpanel", "posthog"}
	var hits []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if d.Name() == "privacy_test.go" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lower := strings.ToLower(string(data))
		for _, line := range strings.Split(lower, "\n") {
			for _, pat := range patterns {
				if strings.Contains(line, pat) {
					rel, err := filepath.Rel(root, path)
					if err != nil {
						return err
					}
					hits = append(hits, rel+": "+pat)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk failed: %v", err)
	}
	if len(hits) > 0 {
		t.Fatalf("telemetry/analytics string in non-test Go file:\n%s", strings.Join(hits, "\n"))
	}
}

func TestScanCommitScriptFlagsSecrets(t *testing.T) {
	script := filepath.Join(repoRoot(t), "scripts", "scan-commit.sh")

	t.Run("secret-in-added-line", func(t *testing.T) {
		dir := newTempRepo(t)
		path := filepath.Join(dir, "example-config.txt")
		if err := os.WriteFile(path, []byte("sk-abcdefgh12345678\n"), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		runGit(t, dir, "add", "example-config.txt")
		res := runScan(t, script, dir)
		if res.code != 1 {
			t.Fatalf("exit code = %d, want 1\noutput:\n%s", res.code, res.output)
		}
		if !strings.Contains(res.output, "example-config.txt") || !strings.Contains(res.output, "secret") {
			t.Fatalf("output missing filename and rule name %q:\n%s", "secret", res.output)
		}
	})

	t.Run("clean-hyphenated-word", func(t *testing.T) {
		dir := newTempRepo(t)
		path := filepath.Join(dir, "evidence.json")
		if err := os.WriteFile(path, []byte(`{"schema_version":"coimnet-real-task-evidence/v1"}`+"\n"), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		runGit(t, dir, "add", "evidence.json")
		res := runScan(t, script, dir)
		if res.code != 0 {
			t.Fatalf("exit code = %d, want 0\noutput:\n%s", res.code, res.output)
		}
	})

	t.Run("clean-staged-diff", func(t *testing.T) {
		dir := newTempRepo(t)
		path := filepath.Join(dir, "clean.txt")
		if err := os.WriteFile(path, []byte("plan: simulate the connectome\n"), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		runGit(t, dir, "add", "clean.txt")
		res := runScan(t, script, dir)
		if res.code != 0 {
			t.Fatalf("exit code = %d, want 0\noutput:\n%s", res.code, res.output)
		}
		if !strings.Contains(res.output, "PASS") {
			t.Fatalf("output missing PASS:\n%s", res.output)
		}
	})

	t.Run("oversized-file", func(t *testing.T) {
		dir := newTempRepo(t)
		path := filepath.Join(dir, "big.bin")
		if err := os.WriteFile(path, make([]byte, 6*1024*1024), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		runGit(t, dir, "add", "big.bin")
		res := runScan(t, script, dir)
		if res.code != 1 {
			t.Fatalf("exit code = %d, want 1\noutput:\n%s", res.code, res.output)
		}
		if !strings.Contains(res.output, "big.bin") || !strings.Contains(res.output, "size") {
			t.Fatalf("output missing filename and rule name %q:\n%s", "size", res.output)
		}
	})
}
