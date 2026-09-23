package media

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPackageVideoToolAbsent(t *testing.T) {
	dir := writeNativeVideo(t)
	report, err := PackageVideo(t.Context(), dir, "coimnet-missing-video-packager-938475")
	if err != nil {
		t.Fatalf("PackageVideo() error = %v", err)
	}
	if report.Tool != "coimnet-missing-video-packager-938475" || report.Status != "tool_absent" {
		t.Fatalf("PackageVideo() report = %+v", report)
	}
	if report.Path != "" || report.Output != "" {
		t.Fatalf("absent tool report has path or output: %+v", report)
	}
}

func TestPackageVideoToolFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake packager is a POSIX shell script")
	}
	dir := writeNativeVideo(t)
	tool := writePackager(t, "printf 'fake ffmpeg version 1\\n'\nexit 0", "printf 'packager failed on purpose\\n' >&2\nexit 3")
	report, err := PackageVideo(t.Context(), dir, tool)
	if err != nil {
		t.Fatalf("PackageVideo() error = %v", err)
	}
	if report.Status != "tool_failed" || !strings.Contains(report.Error, "exit status 3") || !strings.Contains(report.Error, "packager failed on purpose") {
		t.Fatalf("PackageVideo() report = %+v", report)
	}
	if _, err := os.Stat(filepath.Join(dir, "clip.mkv")); !os.IsNotExist(err) {
		t.Fatalf("clip.mkv stat error = %v, want not-exist", err)
	}
	assertNativeFilesAndTimelineHashes(t, dir)
}

func TestPackageVideoPackages(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake packager is a POSIX shell script")
	}
	dir := writeNativeVideo(t)
	tool := writePackager(t, "printf 'fake ffmpeg version 1\\n'\nexit 0", "for arg do last=$arg; done\nprintf 'packaged video' > \"$last\"\nexit 0")
	report, err := PackageVideo(t.Context(), dir, tool)
	if err != nil {
		t.Fatalf("PackageVideo() error = %v", err)
	}
	if report.Status != "packaged" || report.Path != tool || report.Version != "fake ffmpeg version 1" {
		t.Fatalf("PackageVideo() report = %+v", report)
	}
	wantArgs := []string{"-n", "-framerate", "31.25", "-i", filepath.Join(dir, "frame_%02d.png"), "-i", filepath.Join(dir, "audio.wav"), "-c:v", "png", "-c:a", "pcm_s16le", "-shortest", filepath.Join(dir, "clip.mkv")}
	if len(report.Args) != len(wantArgs) {
		t.Fatalf("Args = %q, want %q", report.Args, wantArgs)
	}
	for i := range wantArgs {
		if report.Args[i] != wantArgs[i] {
			t.Fatalf("Args = %q, want %q", report.Args, wantArgs)
		}
	}
	if report.Output != filepath.Join(dir, "clip.mkv") {
		t.Fatalf("Output = %q", report.Output)
	}
	data, err := os.ReadFile(report.Output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	digest := sha256.Sum256(data)
	if report.OutputSHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("OutputSHA256 = %q, want %x", report.OutputSHA256, digest)
	}
}

func TestPackageVideoRequiresNativeOutput(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "invoked")
	tool := writePackager(t, "printf 'fake version\\n'\nprintf invoked > \"$MARKER\"\nexit 0", "printf invoked > \"$MARKER\"\nexit 0")
	contents, err := os.ReadFile(tool)
	if err != nil {
		t.Fatalf("read fake tool: %v", err)
	}
	contents = []byte(strings.ReplaceAll(string(contents), "$MARKER", marker))
	if err := os.WriteFile(tool, contents, 0o700); err != nil {
		t.Fatalf("update fake tool: %v", err)
	}
	if _, err := PackageVideo(t.Context(), dir, tool); err == nil {
		t.Fatal("PackageVideo() on an empty directory returned nil error")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("fake tool marker stat error = %v, want not-exist", err)
	}
}

func writeNativeVideo(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "native")
	frames := make([][]float64, VideoFrames)
	for i := range frames {
		frames[i] = make([]float64, 8*8*3)
	}
	if _, err := WriteVideo(dir, frames, make([]float64, VideoFrames*AudioBlock)); err != nil {
		t.Fatalf("WriteVideo() error = %v", err)
	}
	return dir
}

func writePackager(t *testing.T, versionBody, packageBody string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-ffmpeg")
	script := "#!/bin/sh\nif [ \"$1\" = \"-version\" ]; then\n" + versionBody + "\nfi\n" + packageBody + "\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake packager: %v", err)
	}
	return path
}

func assertNativeFilesAndTimelineHashes(t *testing.T, dir string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "timeline.json"))
	if err != nil {
		t.Fatalf("read timeline: %v", err)
	}
	var timeline Timeline
	if err := json.Unmarshal(data, &timeline); err != nil {
		t.Fatalf("decode timeline: %v", err)
	}
	for name, expected := range timeline.FileSHA256 {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("native file %q was not retained: %v", name, err)
		}
		digest := sha256.Sum256(data)
		if got := hex.EncodeToString(digest[:]); got != expected {
			t.Errorf("native file %q SHA-256 = %s, timeline records %s", name, got, expected)
		}
	}
}
