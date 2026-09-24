// Package realdata imports license-declared real video clips into bounded,
// time-aligned low-resolution RGB frames and mono audio samples.
package realdata

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/TimLai666/coimnet/internal/strictjson"
)

// SchemaVersion identifies the accepted real-media manifest shape.
const SchemaVersion = "coimnet-realmedia/v1"

const maxManifestBytes = 1 << 20

var identifierPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Manifest describes licensed sources and manually annotated clip intervals.
type Manifest struct {
	SchemaVersion string     `json:"schema_version"`
	Dataset       string     `json:"dataset"`
	Output        OutputSpec `json:"output"`
	Sources       []Source   `json:"sources"`
}

// LoadedManifest retains the exact manifest-file fingerprint used by an
// example report while exposing the validated data declaration.
type LoadedManifest struct {
	Manifest Manifest
	SHA256   string
}

// OutputSpec declares the exact resolution and synchronized sample clocks
// produced by the importer.
type OutputSpec struct {
	FrameWidth      int `json:"frame_width"`
	FrameHeight     int `json:"frame_height"`
	FrameRate       int `json:"frame_rate"`
	AudioSampleRate int `json:"audio_sample_rate"`
}

// License records the attribution terms and the page that states them.
type License struct {
	Holder            string `json:"holder"`
	Terms             string `json:"terms"`
	Source            string `json:"source"`
	Attribution       string `json:"attribution"`
	AttributionSource string `json:"attribution_source"`
}

// Source describes one source video file and its expected stream metadata.
type Source struct {
	ID       string    `json:"id"`
	Path     string    `json:"path"`
	SHA256   string    `json:"sha256"`
	Origin   string    `json:"origin"`
	License  License   `json:"license"`
	Media    MediaInfo `json:"media"`
	Segments []Segment `json:"segments"`
}

// MediaInfo pins the source media properties checked against ffprobe.
type MediaInfo struct {
	Width            int    `json:"width"`
	Height           int    `json:"height"`
	FrameRate        string `json:"frame_rate"`
	DurationMillis   int64  `json:"duration_millis"`
	VideoCodec       string `json:"video_codec"`
	VideoStartMillis int64  `json:"video_start_millis"`
	AudioStream      int    `json:"audio_stream"`
	AudioCodec       string `json:"audio_codec"`
	AudioSampleRate  int    `json:"audio_sample_rate"`
	AudioChannels    int    `json:"audio_channels"`
	AudioStartMillis int64  `json:"audio_start_millis"`
}

// Segment is a non-overlapping temporal clip with its own prompt and split.
type Segment struct {
	ID             string `json:"id"`
	Split          string `json:"split"`
	StartMillis    int64  `json:"start_millis"`
	DurationMillis int64  `json:"duration_millis"`
	Prompt         string `json:"prompt"`
	PromptSource   string `json:"prompt_source"`
	AnnotationNote string `json:"annotation_note"`
}

// Limits bounds source files, metadata, decoded shapes and total in-memory
// samples. Zero fields select the documented defaults; negative limits fail.
type Limits struct {
	MaxSourceBytes        int64
	MaxWidth              int
	MaxHeight             int
	MaxDurationMillis     int64
	MaxSegments           int
	MaxSegmentDurationMS  int64
	MaxOutputFrames       int
	MaxOutputAudioSamples int64
	MaxDecodedValues      int64
}

// DefaultLimits returns caps that admit the selected Blender open movies while
// keeping decoded clips small. Source MP4s are copied to a private temp file
// and never decoded at their original pixel resolution.
func DefaultLimits() Limits {
	return Limits{
		MaxSourceBytes:        1 << 30,
		MaxWidth:              3840,
		MaxHeight:             2160,
		MaxDurationMillis:     15 * 60 * 1000,
		MaxSegments:           64,
		MaxSegmentDurationMS:  10 * 1000,
		MaxOutputFrames:       40,
		MaxOutputAudioSamples: 80000,
		MaxDecodedValues:      8_000_000,
	}
}

func (l Limits) withDefaults() (Limits, error) {
	d := DefaultLimits()
	fields := []struct {
		name string
		got  int64
		def  int64
		set  func(int64)
	}{
		{"max_source_bytes", l.MaxSourceBytes, d.MaxSourceBytes, func(v int64) { l.MaxSourceBytes = v }},
		{"max_width", int64(l.MaxWidth), int64(d.MaxWidth), func(v int64) { l.MaxWidth = int(v) }},
		{"max_height", int64(l.MaxHeight), int64(d.MaxHeight), func(v int64) { l.MaxHeight = int(v) }},
		{"max_duration_millis", l.MaxDurationMillis, d.MaxDurationMillis, func(v int64) { l.MaxDurationMillis = v }},
		{"max_segments", int64(l.MaxSegments), int64(d.MaxSegments), func(v int64) { l.MaxSegments = int(v) }},
		{"max_segment_duration_millis", l.MaxSegmentDurationMS, d.MaxSegmentDurationMS, func(v int64) { l.MaxSegmentDurationMS = v }},
		{"max_output_frames", int64(l.MaxOutputFrames), int64(d.MaxOutputFrames), func(v int64) { l.MaxOutputFrames = int(v) }},
		{"max_output_audio_samples", l.MaxOutputAudioSamples, d.MaxOutputAudioSamples, func(v int64) { l.MaxOutputAudioSamples = v }},
		{"max_decoded_values", l.MaxDecodedValues, d.MaxDecodedValues, func(v int64) { l.MaxDecodedValues = v }},
	}
	for _, field := range fields {
		if field.got < 0 {
			return Limits{}, fmt.Errorf("realdata: %s must not be negative", field.name)
		}
		if field.got == 0 {
			field.set(field.def)
		}
	}
	return l, nil
}

// ParseManifest strictly decodes one bounded JSON manifest and validates its
// declarations before any source path is opened.
func ParseManifest(raw []byte) (LoadedManifest, error) {
	if len(raw) == 0 || len(raw) > maxManifestBytes {
		return LoadedManifest{}, fmt.Errorf("realdata: manifest size %d is outside [1,%d] bytes", len(raw), maxManifestBytes)
	}
	if err := strictjson.RejectDuplicateKeys(raw); err != nil {
		return LoadedManifest{}, fmt.Errorf("realdata: reject duplicate manifest keys: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return LoadedManifest{}, fmt.Errorf("realdata: decode manifest: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return LoadedManifest{}, err
	}
	limits, err := Limits{}.withDefaults()
	if err != nil {
		return LoadedManifest{}, err
	}
	if err := manifest.Validate(limits); err != nil {
		return LoadedManifest{}, err
	}
	digest := sha256.Sum256(raw)
	return LoadedManifest{Manifest: manifest, SHA256: hex.EncodeToString(digest[:])}, nil
}

// ReadManifest reads at most one MiB and returns its strictly validated value.
func ReadManifest(r io.Reader) (LoadedManifest, error) {
	if r == nil {
		return LoadedManifest{}, fmt.Errorf("realdata: nil manifest reader")
	}
	raw, err := io.ReadAll(io.LimitReader(r, maxManifestBytes+1))
	if err != nil {
		return LoadedManifest{}, fmt.Errorf("realdata: read manifest: %w", err)
	}
	return ParseManifest(raw)
}

// Validate checks license, prompt provenance, metadata, output shape and
// temporal intervals before any input is read or decoder process is started.
func (m Manifest) Validate(limits Limits) error {
	l, err := limits.withDefaults()
	if err != nil {
		return err
	}
	if m.SchemaVersion != SchemaVersion {
		return fmt.Errorf("realdata: unsupported schema_version %q", m.SchemaVersion)
	}
	if !validID(m.Dataset) {
		return fmt.Errorf("realdata: dataset must be a lowercase identifier")
	}
	if m.Output.FrameWidth <= 0 || m.Output.FrameWidth > 256 || m.Output.FrameWidth > l.MaxWidth {
		return fmt.Errorf("realdata: output frame_width %d is outside [1,%d]", m.Output.FrameWidth, min(256, l.MaxWidth))
	}
	if m.Output.FrameHeight <= 0 || m.Output.FrameHeight > 256 || m.Output.FrameHeight > l.MaxHeight {
		return fmt.Errorf("realdata: output frame_height %d is outside [1,%d]", m.Output.FrameHeight, min(256, l.MaxHeight))
	}
	if m.Output.FrameRate <= 0 || m.Output.FrameRate > 60 {
		return fmt.Errorf("realdata: output frame_rate %d is outside [1,60]", m.Output.FrameRate)
	}
	if m.Output.AudioSampleRate < 8000 || m.Output.AudioSampleRate > 48000 {
		return fmt.Errorf("realdata: output audio_sample_rate %d is outside [8000,48000]", m.Output.AudioSampleRate)
	}
	if m.Output.AudioSampleRate%m.Output.FrameRate != 0 {
		return fmt.Errorf("realdata: audio_sample_rate %d must be divisible by frame_rate %d for exact synchronization", m.Output.AudioSampleRate, m.Output.FrameRate)
	}
	if len(m.Sources) == 0 {
		return fmt.Errorf("realdata: sources must not be empty")
	}
	seenSources := make(map[string]struct{}, len(m.Sources))
	seenContent := make(map[string]string, len(m.Sources))
	segmentCount := 0
	decodedValues := int64(0)
	for sourceIndex, source := range m.Sources {
		if !validID(source.ID) {
			return fmt.Errorf("realdata: source %d id must be a lowercase identifier", sourceIndex)
		}
		if _, ok := seenSources[source.ID]; ok {
			return fmt.Errorf("realdata: duplicate source id %q", source.ID)
		}
		seenSources[source.ID] = struct{}{}
		if !validRelativeMP4(source.Path) {
			return fmt.Errorf("realdata: source %q path must be a local relative .mp4 path", source.ID)
		}
		if !digestPattern.MatchString(source.SHA256) {
			return fmt.Errorf("realdata: source %q sha256 must be 64 lowercase hexadecimal characters", source.ID)
		}
		if firstID, exists := seenContent[source.SHA256]; exists {
			return fmt.Errorf("realdata: duplicate source content for %q and %q; declare clips under one source", firstID, source.ID)
		}
		seenContent[source.SHA256] = source.ID
		if err := validateHTTPURL(source.Origin, "origin"); err != nil {
			return fmt.Errorf("realdata: source %q: %w", source.ID, err)
		}
		if strings.TrimSpace(source.License.Holder) == "" || strings.TrimSpace(source.License.Terms) == "" || strings.TrimSpace(source.License.Attribution) == "" {
			return fmt.Errorf("realdata: source %q license holder, terms and required attribution are required", source.ID)
		}
		if err := validateHTTPURL(source.License.Source, "license source"); err != nil {
			return fmt.Errorf("realdata: source %q: %w", source.ID, err)
		}
		if err := validateHTTPURL(source.License.AttributionSource, "attribution source"); err != nil {
			return fmt.Errorf("realdata: source %q: %w", source.ID, err)
		}
		if err := validateMediaInfo(source.ID, source.Media, l); err != nil {
			return err
		}
		if len(source.Segments) == 0 {
			return fmt.Errorf("realdata: source %q must contain at least one segment", source.ID)
		}
		segmentCount += len(source.Segments)
		if segmentCount > l.MaxSegments {
			return fmt.Errorf("realdata: segment count exceeds limit %d", l.MaxSegments)
		}
		seenSegments := make(map[string]struct{}, len(source.Segments))
		ordered := append([]Segment(nil), source.Segments...)
		for segmentIndex, segment := range ordered {
			if !validID(segment.ID) {
				return fmt.Errorf("realdata: source %q segment %d id must be a lowercase identifier", source.ID, segmentIndex)
			}
			if _, ok := seenSegments[segment.ID]; ok {
				return fmt.Errorf("realdata: source %q has duplicate segment id %q", source.ID, segment.ID)
			}
			seenSegments[segment.ID] = struct{}{}
			if segment.Split != "train" && segment.Split != "validation" && segment.Split != "test" {
				return fmt.Errorf("realdata: source %q segment %q split must be train, validation or test", source.ID, segment.ID)
			}
			if segment.StartMillis < 0 || segment.DurationMillis <= 0 || segment.DurationMillis > l.MaxSegmentDurationMS {
				return fmt.Errorf("realdata: source %q segment %q has invalid or over-limit interval", source.ID, segment.ID)
			}
			if segment.StartMillis < max(source.Media.VideoStartMillis, source.Media.AudioStartMillis) {
				return fmt.Errorf("realdata: source %q segment %q starts before a declared audio/video stream start time", source.ID, segment.ID)
			}
			if segment.StartMillis > math.MaxInt64-segment.DurationMillis || segment.StartMillis+segment.DurationMillis > source.Media.DurationMillis {
				return fmt.Errorf("realdata: source %q segment %q extends past media duration", source.ID, segment.ID)
			}
			frameCount, audioCount, err := outputCounts(segment.DurationMillis, m.Output)
			if err != nil {
				return fmt.Errorf("realdata: source %q segment %q %w", source.ID, segment.ID, err)
			}
			if !utf8.ValidString(segment.Prompt) || strings.TrimSpace(segment.Prompt) == "" || len(segment.Prompt) > 4096 {
				return fmt.Errorf("realdata: source %q segment %q prompt must be non-empty UTF-8 up to 4096 bytes", source.ID, segment.ID)
			}
			if segment.PromptSource != "human_annotation" || strings.TrimSpace(segment.AnnotationNote) == "" || len(segment.AnnotationNote) > 2048 {
				return fmt.Errorf("realdata: source %q segment %q requires prompt_source=human_annotation and an annotation_note", source.ID, segment.ID)
			}
			if frameCount <= 0 || frameCount > int64(l.MaxOutputFrames) || audioCount <= 0 || audioCount > l.MaxOutputAudioSamples {
				return fmt.Errorf("realdata: source %q segment %q decoded shape exceeds frame/audio limit", source.ID, segment.ID)
			}
			maxInt := int64(int(^uint(0) >> 1))
			frameValues := int64(m.Output.FrameWidth) * int64(m.Output.FrameHeight) * 3
			if frameCount > maxInt || audioCount > maxInt/4 || frameCount > maxInt/frameValues {
				return fmt.Errorf("realdata: source %q segment %q decoded buffer size overflows", source.ID, segment.ID)
			}
			if frameValues > math.MaxInt64/frameCount || frameCount*frameValues > math.MaxInt64-audioCount {
				return fmt.Errorf("realdata: source %q segment %q decoded value count overflows", source.ID, segment.ID)
			}
			values := frameCount*frameValues + audioCount
			if decodedValues > math.MaxInt64-values {
				return fmt.Errorf("realdata: decoded value count overflows")
			}
			decodedValues += values
			if decodedValues > l.MaxDecodedValues {
				return fmt.Errorf("realdata: total decoded values exceed limit %d", l.MaxDecodedValues)
			}
		}
		sortSegments(ordered)
		for i := 1; i < len(ordered); i++ {
			previousEnd := ordered[i-1].StartMillis + ordered[i-1].DurationMillis
			if ordered[i].StartMillis < previousEnd {
				return fmt.Errorf("realdata: source %q segments %q and %q overlap", source.ID, ordered[i-1].ID, ordered[i].ID)
			}
		}
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("realdata: manifest contains more than one JSON value")
		}
		return fmt.Errorf("realdata: trailing manifest data: %w", err)
	}
	return nil
}

// outputCounts derives decoder allocation sizes only after proving that both
// millisecond-to-clock multiplications fit in int64 and land on whole units.
func outputCounts(durationMillis int64, output OutputSpec) (int64, int64, error) {
	if durationMillis <= 0 || output.FrameRate <= 0 || output.AudioSampleRate <= 0 {
		return 0, 0, fmt.Errorf("duration and output clocks must be positive")
	}
	frameRate := int64(output.FrameRate)
	audioRate := int64(output.AudioSampleRate)
	if durationMillis > math.MaxInt64/frameRate || durationMillis > math.MaxInt64/audioRate {
		return 0, 0, fmt.Errorf("duration/output clock multiplication overflows int64")
	}
	frameTicks := durationMillis * frameRate
	audioTicks := durationMillis * audioRate
	if frameTicks%1000 != 0 {
		return 0, 0, fmt.Errorf("duration must contain a whole number of output frames")
	}
	if audioTicks%1000 != 0 {
		return 0, 0, fmt.Errorf("duration must contain a whole number of output audio samples")
	}
	return frameTicks / 1000, audioTicks / 1000, nil
}

func validID(value string) bool { return identifierPattern.MatchString(value) }

func validRelativeMP4(value string) bool {
	return value != "" && !strings.Contains(value, `\`) && filepath.IsLocal(value) && filepath.Clean(value) == value && strings.EqualFold(filepath.Ext(value), ".mp4")
}

func sortSegments(segments []Segment) {
	sort.Slice(segments, func(i, j int) bool {
		if segments[i].StartMillis == segments[j].StartMillis {
			return segments[i].ID < segments[j].ID
		}
		return segments[i].StartMillis < segments[j].StartMillis
	})
}

func validateHTTPURL(value, field string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return fmt.Errorf("%s must be an absolute HTTP(S) URL without credentials", field)
	}
	return nil
}

func validateMediaInfo(sourceID string, media MediaInfo, limits Limits) error {
	if media.Width <= 0 || media.Width > limits.MaxWidth || media.Height <= 0 || media.Height > limits.MaxHeight {
		return fmt.Errorf("realdata: source %q dimensions %dx%d exceed configured width/height limits", sourceID, media.Width, media.Height)
	}
	if media.Width > math.MaxInt/media.Height || int64(media.Width)*int64(media.Height) > 3840*2160 {
		return fmt.Errorf("realdata: source %q pixel dimensions exceed the absolute limit", sourceID)
	}
	if _, _, err := parseRate(media.FrameRate); err != nil {
		return fmt.Errorf("realdata: source %q frame_rate: %w", sourceID, err)
	}
	if media.DurationMillis <= 0 || media.DurationMillis > limits.MaxDurationMillis {
		return fmt.Errorf("realdata: source %q duration %dms exceeds configured limits", sourceID, media.DurationMillis)
	}
	if strings.TrimSpace(media.VideoCodec) == "" || strings.TrimSpace(media.AudioCodec) == "" {
		return fmt.Errorf("realdata: source %q video and audio codecs are required", sourceID)
	}
	if media.VideoStartMillis < 0 || media.VideoStartMillis >= media.DurationMillis || media.AudioStartMillis < 0 || media.AudioStartMillis >= media.DurationMillis {
		return fmt.Errorf("realdata: source %q audio/video stream start times must fall within the media duration", sourceID)
	}
	if media.AudioStream < 0 || media.AudioStream > 7 || media.AudioSampleRate < 8000 || media.AudioSampleRate > 192000 || media.AudioChannels < 1 || media.AudioChannels > 8 {
		return fmt.Errorf("realdata: source %q audio sample rate or channel count is outside limits", sourceID)
	}
	return nil
}
