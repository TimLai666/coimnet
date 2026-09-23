package media

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// TimelineEntry is one frame's place in time and its covered audio sample range.
type TimelineEntry struct {
	Frame       int     `json:"frame"`
	TimeSeconds float64 `json:"time_seconds"`
	PNG         string  `json:"png"`
	AudioStart  int     `json:"audio_start"`
	AudioEnd    int     `json:"audio_end"`
}

// Timeline records the native video files and frame-to-audio alignment.
type Timeline struct {
	SchemaVersion string            `json:"schema_version"`
	FrameRate     float64           `json:"frame_rate"`
	SampleRate    int               `json:"sample_rate"`
	Audio         string            `json:"audio"`
	Frames        []TimelineEntry   `json:"frames"`
	FileSHA256    map[string]string `json:"file_sha256"`
}

// WriteVideo writes PNG frames, a WAV track and an indented timeline into a new directory.
func WriteVideo(dir string, frames [][]float64, samples []float64) (Timeline, error) {
	if len(frames) != VideoFrames {
		return Timeline{}, fmt.Errorf("media: video has %d frames, want %d", len(frames), VideoFrames)
	}
	if len(samples) != VideoFrames*AudioBlock {
		return Timeline{}, fmt.Errorf("media: video audio has %d samples, want %d", len(samples), VideoFrames*AudioBlock)
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		return Timeline{}, fmt.Errorf("media: create video directory: %w", err)
	}
	created := make([]string, 0, VideoFrames+2)
	cleanup := func() {
		for _, name := range created {
			_ = os.Remove(filepath.Join(dir, name))
		}
		_ = os.Remove(dir)
	}

	timeline := Timeline{
		SchemaVersion: "coimnet-video-timeline/v1",
		FrameRate:     float64(AudioSampleRate) / AudioBlock,
		SampleRate:    AudioSampleRate,
		Audio:         "audio.wav",
		Frames:        make([]TimelineEntry, VideoFrames),
		FileSHA256:    make(map[string]string, VideoFrames+1),
	}
	for frame, pixels := range frames {
		name := fmt.Sprintf("frame_%02d.png", frame)
		digest, err := WritePNG(filepath.Join(dir, name), pixels)
		if err != nil {
			cleanup()
			return Timeline{}, fmt.Errorf("media: write %s: %w", name, err)
		}
		created = append(created, name)
		timeline.FileSHA256[name] = digest
		start := frame * AudioBlock
		timeline.Frames[frame] = TimelineEntry{
			Frame:       frame,
			TimeSeconds: float64(start) / AudioSampleRate,
			PNG:         name,
			AudioStart:  start,
			AudioEnd:    (frame + 1) * AudioBlock,
		}
	}
	audioName := "audio.wav"
	digest, err := WriteBlockWAV(filepath.Join(dir, audioName), samples)
	if err != nil {
		cleanup()
		return Timeline{}, fmt.Errorf("media: write %s: %w", audioName, err)
	}
	created = append(created, audioName)
	timeline.FileSHA256[audioName] = digest

	data, err := json.MarshalIndent(timeline, "", "  ")
	if err != nil {
		cleanup()
		return Timeline{}, fmt.Errorf("media: encode timeline: %w", err)
	}
	data = append(data, '\n')
	timelineFile, err := os.OpenFile(filepath.Join(dir, "timeline.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		cleanup()
		return Timeline{}, fmt.Errorf("media: create timeline: %w", err)
	}
	if _, err = timelineFile.Write(data); err != nil {
		_ = timelineFile.Close()
		created = append(created, "timeline.json")
		cleanup()
		return Timeline{}, fmt.Errorf("media: write timeline: %w", err)
	}
	if err := timelineFile.Close(); err != nil {
		created = append(created, "timeline.json")
		cleanup()
		return Timeline{}, fmt.Errorf("media: close timeline: %w", err)
	}
	return timeline, nil
}
