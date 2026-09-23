package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PackageReport records one attempt to wrap a natively written clip into a single container file. Absence of the tool
// and failure of the tool are both recorded, never reported as success; the native frames, audio and timeline stay
// complete either way.
type PackageReport struct {
	Tool         string   `json:"tool"`              // the executable name looked up, e.g. "ffmpeg"
	Status       string   `json:"status"`            // "packaged", "tool_absent" or "tool_failed"
	Path         string   `json:"path,omitempty"`    // resolved executable path
	Version      string   `json:"version,omitempty"` // first line of `<tool> -version`
	Args         []string `json:"args,omitempty"`    // exactly the argument array passed to exec, never a shell string
	Output       string   `json:"output,omitempty"`  // the container path when packaged
	OutputSHA256 string   `json:"output_sha256,omitempty"`
	Error        string   `json:"error,omitempty"` // exit error plus the last 2 KiB of stderr when tool_failed
}

// PackageVideo optionally wraps a complete native video output using an external video tool.
func PackageVideo(ctx context.Context, dir, tool string) (PackageReport, error) {
	report := PackageReport{Tool: tool}
	if ctx == nil {
		return report, fmt.Errorf("media: package video context is nil")
	}
	if err := ctx.Err(); err != nil {
		return report, fmt.Errorf("media: package video canceled: %w", err)
	}
	if err := validateNativeVideo(dir); err != nil {
		return report, err
	}

	path, err := exec.LookPath(tool)
	if err != nil {
		report.Status = "tool_absent"
		return report, nil
	}
	report.Path = path
	version, versionStderr, err := runToolCommand(ctx, path, "-version")
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return report, fmt.Errorf("media: package video canceled: %w", ctxErr)
		}
		report.Status = "tool_failed"
		report.Error = formatToolError(err, versionStderr)
		return report, nil
	}
	if len(version) == 0 {
		version = versionStderr
	}
	report.Version = firstLine(version)

	output := filepath.Join(dir, "clip.mkv")
	if _, err := os.Lstat(output); err == nil {
		return report, fmt.Errorf("media: package output already exists: %s", output)
	} else if !os.IsNotExist(err) {
		return report, fmt.Errorf("media: inspect package output: %w", err)
	}
	report.Args = []string{
		"-n", "-framerate", "31.25", "-i", filepath.Join(dir, "frame_%02d.png"),
		"-i", filepath.Join(dir, "audio.wav"), "-c:v", "png", "-c:a", "pcm_s16le",
		"-shortest", output,
	}

	_, stderr, err := runToolCommand(ctx, path, report.Args...)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			_ = os.Remove(output)
			return report, fmt.Errorf("media: package video canceled: %w", ctxErr)
		}
		_ = os.Remove(output)
		report.Status = "tool_failed"
		report.Error = formatToolError(err, stderr)
		return report, nil
	}
	data, err := os.ReadFile(output)
	if err != nil {
		_ = os.Remove(output)
		report.Status = "tool_failed"
		report.Error = fmt.Sprintf("tool exited successfully but did not produce a readable clip: %v", err)
		return report, nil
	}
	digest := sha256.Sum256(data)
	report.Status = "packaged"
	report.Output = output
	report.OutputSHA256 = hex.EncodeToString(digest[:])
	return report, nil
}

func validateNativeVideo(dir string) error {
	data, err := os.ReadFile(filepath.Join(dir, "timeline.json"))
	if err != nil {
		return fmt.Errorf("media: read native video timeline: %w", err)
	}
	var timeline Timeline
	if err := json.Unmarshal(data, &timeline); err != nil {
		return fmt.Errorf("media: decode native video timeline: %w", err)
	}
	if timeline.SchemaVersion != "coimnet-video-timeline/v1" || timeline.FrameRate != float64(AudioSampleRate)/AudioBlock || timeline.SampleRate != AudioSampleRate || timeline.Audio != "audio.wav" || len(timeline.Frames) != VideoFrames || len(timeline.FileSHA256) != VideoFrames+1 {
		return fmt.Errorf("media: incomplete native video timeline")
	}
	expectedFiles := make([]string, 0, VideoFrames+1)
	for i, frame := range timeline.Frames {
		name := fmt.Sprintf("frame_%02d.png", i)
		start := i * AudioBlock
		if frame.Frame != i || frame.TimeSeconds != float64(start)/AudioSampleRate || frame.PNG != name || frame.AudioStart != start || frame.AudioEnd != start+AudioBlock {
			return fmt.Errorf("media: incomplete native video timeline at frame %d", i)
		}
		expectedFiles = append(expectedFiles, name)
	}
	expectedFiles = append(expectedFiles, timeline.Audio)
	for _, name := range expectedFiles {
		expected, ok := timeline.FileSHA256[name]
		if !ok {
			return fmt.Errorf("media: native video timeline is missing hash for %s", name)
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("media: read native video file %s: %w", name, err)
		}
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) != expected {
			return fmt.Errorf("media: native video file %s does not match timeline SHA-256", name)
		}
	}
	return nil
}

func runToolCommand(ctx context.Context, path string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func formatToolError(err error, stderr []byte) string {
	message := err.Error()
	if len(stderr) > 2048 {
		stderr = stderr[len(stderr)-2048:]
	}
	if len(stderr) != 0 {
		message += ": " + string(stderr)
	}
	return message
}

func firstLine(output []byte) string {
	line := strings.SplitN(strings.TrimSpace(string(output)), "\n", 2)[0]
	return strings.TrimSuffix(line, "\r")
}
