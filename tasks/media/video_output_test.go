package media

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

func TestVideoWriteNative(t *testing.T) {
	frames, samples, err := RenderVideo(VideoCondition{Direction: "down", Sound: "high"})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "clip")
	timeline, err := WriteVideo(dir, frames, samples)
	if err != nil {
		t.Fatalf("WriteVideo: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != VideoFrames+2 {
		t.Fatalf("output directory has %d files, want %d", len(entries), VideoFrames+2)
	}
	wantFiles := map[string]bool{"audio.wav": true, "timeline.json": true}
	for frame := 0; frame < VideoFrames; frame++ {
		wantFiles[fmt.Sprintf("frame_%02d.png", frame)] = true
	}
	for _, entry := range entries {
		if !wantFiles[entry.Name()] {
			t.Errorf("unexpected output file %q", entry.Name())
		}
		delete(wantFiles, entry.Name())
	}
	if len(wantFiles) != 0 {
		t.Errorf("missing output files: %v", wantFiles)
	}
	if timeline.SchemaVersion != "coimnet-video-timeline/v1" || timeline.FrameRate != 31.25 || timeline.SampleRate != AudioSampleRate || timeline.Audio != "audio.wav" {
		t.Errorf("timeline header = %+v", timeline)
	}
	if len(timeline.Frames) != VideoFrames || len(timeline.FileSHA256) != VideoFrames+1 {
		t.Fatalf("timeline has %d frames and %d file hashes", len(timeline.Frames), len(timeline.FileSHA256))
	}
	for frame, entry := range timeline.Frames {
		wantStart, wantEnd := frame*AudioBlock, (frame+1)*AudioBlock
		if entry.Frame != frame || entry.TimeSeconds != float64(frame*AudioBlock)/AudioSampleRate || entry.PNG != fmt.Sprintf("frame_%02d.png", frame) || entry.AudioStart != wantStart || entry.AudioEnd != wantEnd {
			t.Errorf("timeline frame %d = %+v", frame, entry)
		}
		if frame > 0 && timeline.Frames[frame-1].AudioEnd != entry.AudioStart {
			t.Errorf("audio ranges do not meet at frame %d", frame)
		}
	}
	if got := timeline.Frames[VideoFrames-1].AudioEnd; got != 2048 {
		t.Errorf("last AudioEnd = %d, want 2048", got)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("unexpected directory %q", entry.Name())
		}
		if entry.Name() == "timeline.json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(raw)
		if got, want := timeline.FileSHA256[entry.Name()], hex.EncodeToString(digest[:]); got != want {
			t.Errorf("hash for %s = %q, want %q", entry.Name(), got, want)
		}
	}
	rawTimeline, err := os.ReadFile(filepath.Join(dir, "timeline.json"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded Timeline
	if err := json.Unmarshal(rawTimeline, &decoded); err != nil {
		t.Fatalf("decode timeline.json: %v", err)
	}
	if len(decoded.Frames) != VideoFrames {
		t.Fatalf("decoded timeline has %d frames, want %d", len(decoded.Frames), VideoFrames)
	}
	if decoded.Frames[VideoFrames-1].AudioEnd != 2048 {
		t.Errorf("decoded timeline last audio end = %d, want 2048", decoded.Frames[VideoFrames-1].AudioEnd)
	}
	wav, err := os.ReadFile(filepath.Join(dir, "audio.wav"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := audio.InspectWAV(wav)
	if err != nil {
		t.Fatal(err)
	}
	signal, err := audio.DecodeWAV(wav)
	if err != nil {
		t.Fatal(err)
	}
	if info.Frames != 2048 || info.SampleRate != AudioSampleRate || signal.SampleRate != AudioSampleRate || len(signal.Samples) != 1 || len(signal.Samples[0]) != 2048 {
		t.Errorf("WAV reports %+v and decoded %d channels", info, len(signal.Samples))
	}

	existing := filepath.Join(t.TempDir(), "existing")
	if err := os.Mkdir(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(existing, "keep.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteVideo(existing, frames, samples); err == nil {
		t.Fatal("WriteVideo on an existing directory returned nil error")
	}
	if content, err := os.ReadFile(sentinel); err != nil || string(content) != "keep" {
		t.Errorf("existing content after WriteVideo = %q, %v; want keep", content, err)
	}
	if got, err := os.ReadDir(existing); err != nil || len(got) != 1 || got[0].Name() != "keep.txt" {
		t.Errorf("existing directory entries after WriteVideo = %v, %v; want only keep.txt", got, err)
	}
}
