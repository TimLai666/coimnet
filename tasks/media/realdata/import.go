package realdata

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Transform records the fixed decoder settings used for every source clip.
type Transform struct {
	FrameFilter        string   `json:"frame_filter"`
	FramePixelFormat   string   `json:"frame_pixel_format"`
	FrameNormalization string   `json:"frame_normalization"`
	AudioFilter        []string `json:"audio_filter"`
	AudioFormat        string   `json:"audio_format"`
	AudioNormalization string   `json:"audio_normalization"`
}

// Toolchain records the decoder versions actually used for import.
type Toolchain struct {
	FFmpeg  string `json:"ffmpeg"`
	FFprobe string `json:"ffprobe"`
}

// SourceRecord records the verified source path, fingerprint, license and
// media metadata without copying any source media into the repository.
type SourceRecord struct {
	ID      string    `json:"id"`
	Path    string    `json:"path"`
	Bytes   int64     `json:"bytes"`
	SHA256  string    `json:"sha256"`
	Origin  string    `json:"origin"`
	License License   `json:"license"`
	Media   MediaInfo `json:"media"`
}

// Frame is one 8-bit RGB image converted to normalized float64 values. The
// associated audio interval uses sample indices relative to Sample.Audio.
type Frame struct {
	Index      int       `json:"index"`
	TimeMillis int64     `json:"time_millis"`
	Pixels     []float64 `json:"pixels"`
	AudioStart int       `json:"audio_start"`
	AudioEnd   int       `json:"audio_end"`
}

// Sample is one manually annotated, real temporal segment.
type Sample struct {
	SourceID        string    `json:"source_id"`
	SegmentID       string    `json:"segment_id"`
	Split           string    `json:"split"`
	Prompt          string    `json:"prompt"`
	PromptSource    string    `json:"prompt_source"`
	AnnotationNote  string    `json:"annotation_note"`
	StartMillis     int64     `json:"start_millis"`
	DurationMillis  int64     `json:"duration_millis"`
	FrameRate       int       `json:"frame_rate"`
	AudioSampleRate int       `json:"audio_sample_rate"`
	Frames          []Frame   `json:"frames"`
	Audio           []float64 `json:"audio"`
	ClippedSamples  int       `json:"clipped_audio_samples"`
}

// Dataset is the decoded in-memory result of importing a manifest.
type Dataset struct {
	SchemaVersion string         `json:"schema_version"`
	Dataset       string         `json:"dataset"`
	Output        OutputSpec     `json:"output"`
	Sources       []SourceRecord `json:"sources"`
	Samples       []Sample       `json:"samples"`
	Transform     Transform      `json:"transform"`
	Toolchain     Toolchain      `json:"toolchain"`
}

// DefaultTransform describes the versioned, fixed FFmpeg conversion. The
// source aspect ratio is retained and letterboxed into the output frame.
func DefaultTransform() Transform {
	return Transform{
		FrameFilter:        "fps={frame_rate},scale={width}:{height}:force_original_aspect_ratio=decrease:flags=area,pad={width}:{height}:(ow-iw)/2:(oh-ih)/2:black,format=rgb24",
		FramePixelFormat:   "rgb24",
		FrameNormalization: "uint8 RGB divided by 255 into [0,1] float64",
		AudioFilter:        []string{"mono downmix", "resample to manifest output.audio_sample_rate"},
		AudioFormat:        "f32le",
		AudioNormalization: "float32 converted to float64 and clamped to [-1,1]",
	}
}

// Import verifies sources and extracts only declared intervals. FFmpeg and
// ffprobe are explicit external dependencies; no shell is used.
func Import(ctx context.Context, root string, manifest Manifest, requested Limits) (Dataset, error) {
	if ctx == nil {
		return Dataset{}, fmt.Errorf("realdata: context is nil")
	}
	limits, err := requested.withDefaults()
	if err != nil {
		return Dataset{}, err
	}
	if err := manifest.Validate(limits); err != nil {
		return Dataset{}, err
	}
	if err := ctx.Err(); err != nil {
		return Dataset{}, fmt.Errorf("realdata: import canceled: %w", err)
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return Dataset{}, fmt.Errorf("realdata: ffmpeg is required to import MP4: %w", err)
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		return Dataset{}, fmt.Errorf("realdata: ffprobe is required to validate MP4: %w", err)
	}
	toolchain, err := readToolchain(ctx, ffmpeg, ffprobe)
	if err != nil {
		return Dataset{}, err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return Dataset{}, fmt.Errorf("realdata: resolve data root: %w", err)
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return Dataset{}, fmt.Errorf("realdata: resolve data root symlinks: %w", err)
	}
	rootInfo, err := os.Stat(rootReal)
	if err != nil || !rootInfo.IsDir() {
		return Dataset{}, fmt.Errorf("realdata: data root is not a directory")
	}
	dataset := Dataset{
		SchemaVersion: SchemaVersion,
		Dataset:       manifest.Dataset,
		Output:        manifest.Output,
		Sources:       make([]SourceRecord, 0, len(manifest.Sources)),
		Transform:     DefaultTransform(),
		Toolchain:     toolchain,
	}
	for _, source := range manifest.Sources {
		if err := ctx.Err(); err != nil {
			return Dataset{}, fmt.Errorf("realdata: import canceled: %w", err)
		}
		tempPath, size, err := stageSource(rootReal, source, limits.MaxSourceBytes)
		if err != nil {
			return Dataset{}, err
		}
		probe, probeErr := probeMedia(ctx, ffprobe, tempPath, limits, source.Media.AudioStream)
		if probeErr == nil {
			probeErr = compareMediaInfo(source.ID, source.Media, probe)
		}
		if probeErr == nil {
			for _, segment := range source.Segments {
				if err := ctx.Err(); err != nil {
					probeErr = fmt.Errorf("realdata: import canceled: %w", err)
					break
				}
				sample, decodeErr := decodeSegment(ctx, ffmpeg, tempPath, manifest.Output, source.ID, source.Media.AudioStream, segment, limits)
				if decodeErr != nil {
					probeErr = decodeErr
					break
				}
				dataset.Samples = append(dataset.Samples, sample)
			}
		}
		removeErr := os.Remove(tempPath)
		if probeErr != nil {
			return Dataset{}, probeErr
		}
		if removeErr != nil {
			return Dataset{}, fmt.Errorf("realdata: remove verified temporary source: %w", removeErr)
		}
		dataset.Sources = append(dataset.Sources, SourceRecord{
			ID: source.ID, Path: source.Path, Bytes: size, SHA256: source.SHA256,
			Origin: source.Origin, License: source.License, Media: probe,
		})
	}
	return dataset, nil
}

type probeResult struct {
	Streams []struct {
		CodecName  string `json:"codec_name"`
		CodecType  string `json:"codec_type"`
		StartTime  string `json:"start_time"`
		Width      int    `json:"width"`
		Height     int    `json:"height"`
		AvgRate    string `json:"avg_frame_rate"`
		Rate       string `json:"r_frame_rate"`
		SampleRate string `json:"sample_rate"`
		Channels   int    `json:"channels"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func stageSource(root string, source Source, maxBytes int64) (string, int64, error) {
	path, err := safeSourcePath(root, source.Path)
	if err != nil {
		return "", 0, fmt.Errorf("realdata: source %q path: %w", source.ID, err)
	}
	input, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("realdata: open source %q: %w", source.ID, err)
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return "", 0, fmt.Errorf("realdata: stat source %q: %w", source.ID, err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return "", 0, fmt.Errorf("realdata: source %q must be a non-empty regular file", source.ID)
	}
	if info.Size() > maxBytes {
		return "", 0, fmt.Errorf("realdata: source %q size %d bytes exceeds limit %d", source.ID, info.Size(), maxBytes)
	}
	temp, err := os.CreateTemp("", "coimnet-realmedia-*.mp4")
	if err != nil {
		return "", 0, fmt.Errorf("realdata: create temporary source: %w", err)
	}
	tempPath := temp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = temp.Close()
			_ = os.Remove(tempPath)
		}
	}()
	digest := sha256.New()
	copyLimit := maxBytes
	if copyLimit < math.MaxInt64 {
		copyLimit++
	}
	limitReader := io.LimitReader(input, copyLimit)
	copyBytes, err := io.Copy(io.MultiWriter(temp, digest), limitReader)
	if err != nil {
		return "", 0, fmt.Errorf("realdata: stage source %q: %w", source.ID, err)
	}
	if copyBytes > maxBytes || copyBytes != info.Size() {
		return "", 0, fmt.Errorf("realdata: source %q changed size while being staged or exceeds limit", source.ID)
	}
	got := hex.EncodeToString(digest.Sum(nil))
	if got != source.SHA256 {
		return "", 0, fmt.Errorf("realdata: source %q SHA-256 mismatch: got %s, want %s", source.ID, got, source.SHA256)
	}
	if err := temp.Sync(); err != nil {
		return "", 0, fmt.Errorf("realdata: sync temporary source: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", 0, fmt.Errorf("realdata: close temporary source: %w", err)
	}
	ok = true
	return tempPath, copyBytes, nil
}

func safeSourcePath(root, relative string) (string, error) {
	if !validRelativeMP4(relative) {
		return "", fmt.Errorf("must be a local relative .mp4 path")
	}
	path := root
	for _, part := range strings.Split(filepath.FromSlash(relative), string(os.PathSeparator)) {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("symlink path component %q is not allowed", part)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("source is not a regular file")
	}
	return path, nil
}

func probeMedia(ctx context.Context, ffprobe, path string, limits Limits, audioOrdinal int) (MediaInfo, error) {
	args := []string{"-v", "error", "-show_streams", "-show_format", "-of", "json", path}
	raw, err := runLimited(ctx, ffprobe, args, 1<<20)
	if err != nil {
		return MediaInfo{}, fmt.Errorf("realdata: ffprobe source: %w", err)
	}
	var probe probeResult
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	if err := decoder.Decode(&probe); err != nil {
		return MediaInfo{}, fmt.Errorf("realdata: decode ffprobe result: %w", err)
	}
	if len(probe.Streams) == 0 {
		return MediaInfo{}, fmt.Errorf("realdata: ffprobe found no media streams")
	}
	var video *probeResultStream
	audios := make([]probeResultStream, 0, 2)
	for i := range probe.Streams {
		stream := probe.Streams[i]
		candidate := probeResultStream{CodecName: stream.CodecName, CodecType: stream.CodecType, Width: stream.Width,
			Height: stream.Height, AvgRate: stream.AvgRate, Rate: stream.Rate, SampleRate: stream.SampleRate, Channels: stream.Channels}
		switch stream.CodecType {
		case "video":
			if video != nil {
				return MediaInfo{}, fmt.Errorf("realdata: source has multiple video streams; manifest requires one")
			}
			candidate.StartMillis, err = parseStreamStart(stream.StartTime)
			if err != nil {
				return MediaInfo{}, fmt.Errorf("realdata: video stream %d start time: %w", i, err)
			}
			video = &candidate
		case "audio":
			candidate.StartMillis, err = parseStreamStart(stream.StartTime)
			if err != nil {
				return MediaInfo{}, fmt.Errorf("realdata: audio stream %d start time: %w", i, err)
			}
			audios = append(audios, candidate)
		}
	}
	if video == nil {
		return MediaInfo{}, fmt.Errorf("realdata: source must contain one video stream")
	}
	if audioOrdinal < 0 || audioOrdinal >= len(audios) {
		return MediaInfo{}, fmt.Errorf("realdata: source audio stream %d is unavailable; source has %d audio streams", audioOrdinal, len(audios))
	}
	audio := &audios[audioOrdinal]
	if video.Width <= 0 || video.Width > limits.MaxWidth || video.Height <= 0 || video.Height > limits.MaxHeight {
		return MediaInfo{}, fmt.Errorf("realdata: source dimensions %dx%d exceed configured width/height limits", video.Width, video.Height)
	}
	if int64(video.Width)*int64(video.Height) > 3840*2160 {
		return MediaInfo{}, fmt.Errorf("realdata: source pixel dimensions exceed the absolute limit")
	}
	durationSeconds, err := strconv.ParseFloat(probe.Format.Duration, 64)
	if err != nil || math.IsNaN(durationSeconds) || math.IsInf(durationSeconds, 0) || durationSeconds <= 0 || durationSeconds > float64(limits.MaxDurationMillis)/1000 {
		return MediaInfo{}, fmt.Errorf("realdata: source duration is missing, invalid or over limit")
	}
	frameRate := video.AvgRate
	if frameRate == "0/0" || frameRate == "" {
		frameRate = video.Rate
	}
	if _, _, err := parseRate(frameRate); err != nil {
		return MediaInfo{}, fmt.Errorf("realdata: source video frame rate: %w", err)
	}
	sampleRate, err := strconv.Atoi(audio.SampleRate)
	if err != nil || sampleRate < 8000 || sampleRate > 192000 || audio.Channels < 1 || audio.Channels > 8 {
		return MediaInfo{}, fmt.Errorf("realdata: source audio sample rate or channel count is outside limits")
	}
	return MediaInfo{
		Width: video.Width, Height: video.Height, FrameRate: frameRate,
		DurationMillis: int64(math.Round(durationSeconds * 1000)), VideoCodec: video.CodecName, VideoStartMillis: video.StartMillis,
		AudioStream: audioOrdinal, AudioCodec: audio.CodecName, AudioSampleRate: sampleRate, AudioChannels: audio.Channels,
		AudioStartMillis: audio.StartMillis,
	}, nil
}

func parseStreamStart(value string) (int64, error) {
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 || seconds > float64(math.MaxInt64)/1000 {
		return 0, fmt.Errorf("missing or invalid non-negative start_time %q", value)
	}
	return int64(math.Round(seconds * 1000)), nil
}

type probeResultStream struct {
	CodecName   string
	CodecType   string
	StartMillis int64
	Width       int
	Height      int
	AvgRate     string
	Rate        string
	SampleRate  string
	Channels    int
}

func compareMediaInfo(sourceID string, expected, actual MediaInfo) error {
	expectedNumerator, expectedDenominator, err := parseRate(expected.FrameRate)
	if err != nil {
		return fmt.Errorf("realdata: invalid expected frame rate for source %q: %w", sourceID, err)
	}
	actualNumerator, actualDenominator, err := parseRate(actual.FrameRate)
	if err != nil {
		return fmt.Errorf("realdata: invalid observed frame rate for source %q: %w", sourceID, err)
	}
	if expected.Width != actual.Width || expected.Height != actual.Height ||
		expectedNumerator*actualDenominator != actualNumerator*expectedDenominator ||
		abs64(expected.DurationMillis-actual.DurationMillis) > 1 || expected.VideoCodec != actual.VideoCodec || expected.VideoStartMillis != actual.VideoStartMillis ||
		expected.AudioStream != actual.AudioStream || expected.AudioCodec != actual.AudioCodec || expected.AudioSampleRate != actual.AudioSampleRate ||
		expected.AudioChannels != actual.AudioChannels || expected.AudioStartMillis != actual.AudioStartMillis {
		return fmt.Errorf("realdata: source %q probed metadata does not match manifest: got %+v, want %+v", sourceID, actual, expected)
	}
	return nil
}

func decodeSegment(ctx context.Context, ffmpeg, path string, output OutputSpec, sourceID string, audioOrdinal int, segment Segment, limits Limits) (Sample, error) {
	frameCount64, audioCount64, err := outputCounts(segment.DurationMillis, output)
	if err != nil {
		return Sample{}, fmt.Errorf("realdata: source %q segment %q: %w", sourceID, segment.ID, err)
	}
	maxInt := int64(int(^uint(0) >> 1))
	if frameCount64 > int64(limits.MaxOutputFrames) || audioCount64 > limits.MaxOutputAudioSamples ||
		frameCount64 > maxInt || audioCount64 > maxInt/4 {
		return Sample{}, fmt.Errorf("realdata: source %q segment %q decoded shape exceeds frame/audio limit", sourceID, segment.ID)
	}
	frameBytes := output.FrameWidth * output.FrameHeight * 3
	if frameCount64 > maxInt/int64(frameBytes) {
		return Sample{}, fmt.Errorf("realdata: source %q segment %q decoded frame buffer size overflows", sourceID, segment.ID)
	}
	frameCount, audioCount := int(frameCount64), int(audioCount64)
	maxFrameBytes := int(frameCount64 * int64(frameBytes))
	maxAudioBytes := audioCount * 4
	start, duration := formatSeconds(segment.StartMillis), formatSeconds(segment.DurationMillis)
	filter := fmt.Sprintf("fps=%d,scale=%d:%d:force_original_aspect_ratio=decrease:flags=area,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black,format=rgb24",
		output.FrameRate, output.FrameWidth, output.FrameHeight, output.FrameWidth, output.FrameHeight)
	frameArgs := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-ss", start, "-i", path, "-t", duration,
		"-map", "0:v:0", "-vf", filter, "-frames:v", strconv.Itoa(frameCount), "-an", "-f", "rawvideo", "-pix_fmt", "rgb24", "pipe:1"}
	rawFrames, err := runLimited(ctx, ffmpeg, frameArgs, maxFrameBytes)
	if err != nil {
		return Sample{}, fmt.Errorf("realdata: decode source %q segment %q frames: %w", sourceID, segment.ID, err)
	}
	if len(rawFrames) != maxFrameBytes {
		return Sample{}, fmt.Errorf("realdata: source %q segment %q decoded %d frame bytes, want %d", sourceID, segment.ID, len(rawFrames), maxFrameBytes)
	}
	audioArgs := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-ss", start, "-i", path, "-t", duration,
		"-map", fmt.Sprintf("0:a:%d", audioOrdinal), "-vn", "-ac", "1", "-ar", strconv.Itoa(output.AudioSampleRate), "-f", "f32le", "pipe:1"}
	rawAudio, err := runLimited(ctx, ffmpeg, audioArgs, maxAudioBytes)
	if err != nil {
		return Sample{}, fmt.Errorf("realdata: decode source %q segment %q audio: %w", sourceID, segment.ID, err)
	}
	if len(rawAudio) != maxAudioBytes {
		return Sample{}, fmt.Errorf("realdata: source %q segment %q decoded %d audio bytes, want %d", sourceID, segment.ID, len(rawAudio), maxAudioBytes)
	}
	samplesPerFrame := output.AudioSampleRate / output.FrameRate
	sample := Sample{
		SourceID: sourceID, SegmentID: segment.ID, Split: segment.Split, Prompt: segment.Prompt,
		PromptSource: segment.PromptSource, AnnotationNote: segment.AnnotationNote,
		StartMillis: segment.StartMillis, DurationMillis: segment.DurationMillis,
		FrameRate: output.FrameRate, AudioSampleRate: output.AudioSampleRate,
		Frames: make([]Frame, frameCount), Audio: make([]float64, audioCount),
	}
	for i := 0; i < audioCount; i++ {
		value := math.Float32frombits(binary.LittleEndian.Uint32(rawAudio[i*4 : i*4+4]))
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return Sample{}, fmt.Errorf("realdata: source %q segment %q audio sample %d is not finite", sourceID, segment.ID, i)
		}
		converted := float64(value)
		if converted < -1 {
			converted = -1
			sample.ClippedSamples++
		} else if converted > 1 {
			converted = 1
			sample.ClippedSamples++
		}
		sample.Audio[i] = converted
	}
	for frameIndex := 0; frameIndex < frameCount; frameIndex++ {
		pixels := make([]float64, frameBytes)
		for pixelIndex, value := range rawFrames[frameIndex*frameBytes : (frameIndex+1)*frameBytes] {
			pixels[pixelIndex] = float64(value) / 255
		}
		audioStart := frameIndex * samplesPerFrame
		sample.Frames[frameIndex] = Frame{
			Index: frameIndex, TimeMillis: segment.StartMillis + int64(frameIndex)*1000/int64(output.FrameRate),
			Pixels: pixels, AudioStart: audioStart, AudioEnd: audioStart + samplesPerFrame,
		}
	}
	return sample, nil
}

func readToolchain(ctx context.Context, ffmpeg, ffprobe string) (Toolchain, error) {
	version := func(path string) (string, error) {
		output, err := runLimited(ctx, path, []string{"-version"}, 4096)
		if err != nil {
			return "", err
		}
		line := strings.SplitN(strings.TrimSpace(string(output)), "\n", 2)[0]
		if line == "" {
			return "", fmt.Errorf("realdata: %s returned an empty version", filepath.Base(path))
		}
		return line, nil
	}
	ffmpegVersion, err := version(ffmpeg)
	if err != nil {
		return Toolchain{}, fmt.Errorf("realdata: read ffmpeg version: %w", err)
	}
	ffprobeVersion, err := version(ffprobe)
	if err != nil {
		return Toolchain{}, fmt.Errorf("realdata: read ffprobe version: %w", err)
	}
	return Toolchain{FFmpeg: ffmpegVersion, FFprobe: ffprobeVersion}, nil
}

type limitedBuffer struct {
	data []byte
	max  int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	room := b.max - len(b.data)
	if room > 0 {
		if len(p) > room {
			b.data = append(b.data, p[:room]...)
		} else {
			b.data = append(b.data, p...)
		}
	}
	return len(p), nil
}

func runLimited(ctx context.Context, name string, args []string, maxOutput int) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}
	stderr := &limitedBuffer{max: 16 << 10}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", filepath.Base(name), err)
	}
	raw, readErr := io.ReadAll(io.LimitReader(stdout, int64(maxOutput)+1))
	if readErr != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("read %s output: %w", filepath.Base(name), readErr)
	}
	if len(raw) > maxOutput {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("%s output exceeds %d byte limit", filepath.Base(name), maxOutput)
	}
	if err := cmd.Wait(); err != nil {
		message := strings.TrimSpace(string(stderr.data))
		if message != "" {
			return nil, fmt.Errorf("%s failed: %w: %s", filepath.Base(name), err, message)
		}
		return nil, fmt.Errorf("%s failed: %w", filepath.Base(name), err)
	}
	return raw, nil
}

func formatSeconds(milliseconds int64) string {
	return strconv.FormatFloat(float64(milliseconds)/1000, 'f', 3, 64)
}

func parseRate(raw string) (int64, int64, error) {
	parts := strings.Split(raw, "/")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid rational frame rate %q", raw)
	}
	numerator, err1 := strconv.ParseInt(parts[0], 10, 64)
	denominator, err2 := strconv.ParseInt(parts[1], 10, 64)
	if err1 != nil || err2 != nil || numerator <= 0 || denominator <= 0 || numerator > math.MaxInt32 || denominator > math.MaxInt32 {
		return 0, 0, fmt.Errorf("invalid rational frame rate %q", raw)
	}
	divisor := gcd(numerator, denominator)
	return numerator / divisor, denominator / divisor, nil
}

func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
