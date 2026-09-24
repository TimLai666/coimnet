package realdata

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseManifestRejectsUnknownAndTrailingJSON(t *testing.T) {
	valid := fixtureManifest()
	raw, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}

	unknown := strings.TrimSuffix(string(raw), "}") + `,"unexpected":true}`
	if _, err := ParseManifest([]byte(unknown)); err == nil {
		t.Fatal("ParseManifest accepted an unknown top-level field")
	}
	if _, err := ParseManifest(append(raw, []byte(` {}`)...)); err == nil {
		t.Fatal("ParseManifest accepted a second JSON value")
	}
	if _, err := ParseManifest([]byte("null")); err == nil {
		t.Fatal("ParseManifest accepted null as a manifest")
	}
}

func TestParseManifestRejectsDuplicateJSONKeysIncludingEscapedNames(t *testing.T) {
	valid, err := json.Marshal(fixtureManifest())
	if err != nil {
		t.Fatal(err)
	}

	for name, replacement := range map[string]string{
		"duplicate key":             `"dataset":"test-realmedia","dataset":"overridden"`,
		"unicode-escaped duplicate": `"dataset":"test-realmedia","da\u0074aset":"overridden"`,
	} {
		t.Run(name, func(t *testing.T) {
			raw := strings.Replace(string(valid), `"dataset":"test-realmedia"`, replacement, 1)
			if raw == string(valid) {
				t.Fatal("test setup did not find dataset field")
			}
			if _, err := ParseManifest([]byte(raw)); err == nil || !strings.Contains(err.Error(), "duplicate") {
				t.Fatalf("ParseManifest duplicate-key error = %v, want duplicate-key rejection", err)
			}
		})
	}
}

func TestManifestRejectsDurationClockMultiplicationOverflow(t *testing.T) {
	m := fixtureManifest()
	m.Output.FrameRate = 4
	m.Output.AudioSampleRate = 48000
	m.Sources[0].Media.DurationMillis = math.MaxInt64
	m.Sources[0].Segments[0].DurationMillis = math.MaxInt64/48000 + 1

	limits := DefaultLimits()
	limits.MaxDurationMillis = math.MaxInt64
	limits.MaxSegmentDurationMS = math.MaxInt64
	if err := m.Validate(limits); err == nil || !strings.Contains(err.Error(), "overflow") {
		t.Fatalf("overflowing output clock error = %v, want explicit overflow rejection", err)
	}
}

func TestManifestValidatesFrameAndAudioClocksWithoutTruncating(t *testing.T) {
	m := fixtureManifest()
	m.Output.FrameRate = 60
	m.Output.AudioSampleRate = 48000
	m.Sources[0].Segments[0].DurationMillis = 50
	if err := m.Validate(DefaultLimits()); err != nil {
		t.Fatalf("50ms at 60fps/48kHz is an exact 3-frame/2400-sample interval: %v", err)
	}

	m.Sources[0].Segments = append(m.Sources[0].Segments, Segment{
		ID: "overlap", Split: "validation", StartMillis: 25, DurationMillis: 50,
		Prompt: "A second interval.", PromptSource: "human_annotation", AnnotationNote: "Human review.",
	})
	if err := m.Validate(DefaultLimits()); err == nil || !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("overlapping intervals error = %v, want overlap rejection", err)
	}
}

func TestParseStreamStartRoundsToManifestMillisecondAndRejectsInvalidTimes(t *testing.T) {
	if got, err := parseStreamStart("0.066667"); err != nil || got != 67 {
		t.Fatalf("parseStreamStart(0.066667) = %d, %v; want 67ms", got, err)
	}
	for _, invalid := range []string{"", "N/A", "-0.001", "NaN", "+Inf"} {
		if _, err := parseStreamStart(invalid); err == nil {
			t.Errorf("parseStreamStart(%q) accepted an invalid media timestamp", invalid)
		}
	}
}

func TestManifestRejectsSegmentBeforeLaterAudioVideoStreamStart(t *testing.T) {
	m := fixtureManifest()
	m.Sources[0].Media.VideoStartMillis = 67
	if err := m.Validate(DefaultLimits()); err == nil || !strings.Contains(err.Error(), "stream start time") {
		t.Fatalf("segment before stream PTS start error = %v, want start-time rejection", err)
	}
}

func TestManifestRejectsSameMediaUnderDifferentSourceIDs(t *testing.T) {
	m := fixtureManifest()
	duplicate := m.Sources[0]
	duplicate.ID = "film-copy"
	duplicate.Path = "film-copy.mp4"
	duplicate.Segments = []Segment{{
		ID: "held-out", Split: "test", StartMillis: 0, DurationMillis: 2000,
		Prompt: "A blue-gray machine fills the dark room.", PromptSource: "human_annotation",
		AnnotationNote: "The same original film was renamed for a false held-out split.",
	}}
	m.Sources = append(m.Sources, duplicate)
	if err := m.Validate(DefaultLimits()); err == nil || !strings.Contains(err.Error(), "duplicate source content") {
		t.Fatalf("renamed duplicate source error = %v, want content-identity rejection", err)
	}
}

func TestImportRequiresLicenseAndHumanPromptProvenance(t *testing.T) {
	m := fixtureManifest()
	m.Sources[0].License = License{}
	if _, err := Import(context.Background(), t.TempDir(), m, DefaultLimits()); err == nil || !strings.Contains(err.Error(), "license") {
		t.Fatalf("Import with missing license error = %v, want license validation error", err)
	}

	m = fixtureManifest()
	m.Sources[0].Segments[0].PromptSource = ""
	if _, err := Import(context.Background(), t.TempDir(), m, DefaultLimits()); err == nil || !strings.Contains(err.Error(), "prompt_source") {
		t.Fatalf("Import with missing prompt provenance error = %v, want prompt_source validation error", err)
	}
}

func TestImportRejectsUnsafePathsAndSourceSymlinks(t *testing.T) {
	root := t.TempDir()
	m := fixtureManifest()
	m.Sources[0].Path = "../outside.mp4"
	if _, err := Import(context.Background(), root, m, DefaultLimits()); err == nil {
		t.Fatal("Import accepted a parent-directory traversal")
	}

	outside := filepath.Join(t.TempDir(), "outside.mp4")
	if err := os.WriteFile(outside, []byte("not a video"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked.mp4")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	m = fixtureManifest()
	m.Sources[0].Path = "linked.mp4"
	if _, err := Import(context.Background(), root, m, DefaultLimits()); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("Import symlink error = %v, want symlink rejection", err)
	}
}

func TestImportRejectsFingerprintMismatchAndResourceLimits(t *testing.T) {
	root, source := makeAVFixture(t)
	m := fixtureManifest()
	m.Sources[0].Path = filepath.Base(source)
	m.Sources[0].SHA256 = strings.Repeat("0", 64)
	if _, err := Import(context.Background(), root, m, DefaultLimits()); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("Import mismatched hash error = %v, want fingerprint rejection", err)
	}

	m = fixtureManifest()
	m.Sources[0].Path = filepath.Base(source)
	m.Sources[0].SHA256 = sha256File(t, source)
	limits := DefaultLimits()
	limits.MaxWidth = 16
	if _, err := Import(context.Background(), root, m, limits); err == nil || !strings.Contains(err.Error(), "width") {
		t.Fatalf("Import over-width error = %v, want dimension rejection", err)
	}

	limits = DefaultLimits()
	limits.MaxSourceBytes = 1
	if _, err := Import(context.Background(), root, m, limits); err == nil || !strings.Contains(err.Error(), "size") {
		t.Fatalf("Import over-size error = %v, want source-size rejection", err)
	}
}

func TestImportExtractsRealAVWithExplicitFrameAudioAlignment(t *testing.T) {
	root, source := makeAVFixture(t)
	m := fixtureManifest()
	m.Sources[0].Path = filepath.Base(source)
	m.Sources[0].SHA256 = sha256File(t, source)
	m.Sources[0].Media = MediaInfo{
		Width: 32, Height: 24, FrameRate: "4/1", DurationMillis: 2000,
		VideoCodec: "mpeg4", AudioCodec: "aac", AudioSampleRate: 8000, AudioChannels: 1,
	}
	dataset, err := Import(context.Background(), root, m, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(dataset.Sources) != 1 || len(dataset.Samples) != 1 {
		t.Fatalf("imported %d sources and %d samples, want one each", len(dataset.Sources), len(dataset.Samples))
	}
	sample := dataset.Samples[0]
	if len(sample.Frames) != 8 || len(sample.Audio) != 16000 {
		t.Fatalf("decoded %d frames and %d audio samples, want 8 and 16000", len(sample.Frames), len(sample.Audio))
	}
	for i, frame := range sample.Frames {
		if len(frame.Pixels) != 8*8*3 {
			t.Fatalf("frame %d has %d pixels, want %d", i, len(frame.Pixels), 8*8*3)
		}
		if frame.AudioStart != i*2000 || frame.AudioEnd != (i+1)*2000 {
			t.Fatalf("frame %d covers audio [%d,%d), want [%d,%d)", i, frame.AudioStart, frame.AudioEnd, i*2000, (i+1)*2000)
		}
		for j, value := range frame.Pixels {
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
				t.Fatalf("frame %d pixel %d is outside [0,1]: %g", i, j, value)
			}
		}
	}
	for i, value := range sample.Audio {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < -1 || value > 1 {
			t.Fatalf("audio sample %d is outside [-1,1]: %g", i, value)
		}
	}
	if dataset.Sources[0].SHA256 != m.Sources[0].SHA256 {
		t.Fatalf("source fingerprint = %s, want %s", dataset.Sources[0].SHA256, m.Sources[0].SHA256)
	}
}

func TestImportSelectsDeclaredAudioStreamFromMultipleTracks(t *testing.T) {
	root, source := makeAVFixture(t)
	m := fixtureManifest()
	m.Sources[0].Path = filepath.Base(source)
	m.Sources[0].SHA256 = sha256File(t, source)
	m.Sources[0].Media = MediaInfo{
		Width: 32, Height: 24, FrameRate: "4/1", DurationMillis: 2000,
		VideoCodec: "mpeg4", AudioStream: 1, AudioCodec: "aac", AudioSampleRate: 8000, AudioChannels: 1,
	}
	dataset, err := Import(context.Background(), root, m, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	audio := dataset.Samples[0].Audio
	positiveCrossings := 0
	for i := 4000; i < 12000; i++ {
		if audio[i-1] <= 0 && audio[i] > 0 {
			positiveCrossings++
		}
	}
	if positiveCrossings < 820 || positiveCrossings > 940 {
		t.Fatalf("selected second audio track had %d positive crossings over one second, want about 880 Hz", positiveCrossings)
	}
}

func fixtureManifest() Manifest {
	return Manifest{
		SchemaVersion: SchemaVersion,
		Dataset:       "test-realmedia",
		Output:        OutputSpec{FrameWidth: 8, FrameHeight: 8, FrameRate: 4, AudioSampleRate: 8000},
		Sources: []Source{{
			ID: "film", Path: "film.mp4", SHA256: strings.Repeat("a", 64), Origin: "https://example.org/video",
			License: License{
				Holder: "Example Holder", Terms: "CC BY 4.0", Source: "https://example.org/license",
				Attribution: "Example Holder", AttributionSource: "https://example.org/attribution",
			},
			Media: MediaInfo{
				Width: 32, Height: 24, FrameRate: "4/1", DurationMillis: 2000,
				VideoCodec: "mpeg4", AudioCodec: "aac", AudioSampleRate: 8000, AudioChannels: 1,
			},
			Segments: []Segment{{
				ID: "clip-1", Split: "train", StartMillis: 0, DurationMillis: 2000,
				Prompt: "A blue-gray machine fills the dark room.", PromptSource: "human_annotation",
				AnnotationNote: "Human visual review of this exact two-second interval.",
			}},
		}},
	}
}

func makeAVFixture(t *testing.T) (string, string) {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	root := t.TempDir()
	path := filepath.Join(root, "fixture.mp4")
	cmd := exec.Command(ffmpeg,
		"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=size=32x24:rate=4:duration=2",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=8000:duration=2",
		"-f", "lavfi", "-i", "sine=frequency=880:sample_rate=8000:duration=2",
		"-map", "0:v:0", "-map", "1:a:0", "-map", "2:a:0", "-t", "2", "-c:v", "mpeg4", "-q:v", "4", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-ar", "8000", "-ac", "1", "-shortest", path,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg cannot create integration fixture: %v: %s", err, output)
	}
	return root, path
}

func sha256File(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
