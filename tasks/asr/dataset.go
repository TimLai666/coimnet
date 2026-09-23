package asr

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/TimLai666/coimnet/internal/fileio"
	"github.com/TimLai666/coimnet/internal/strictjson"
	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

const (
	asrDatasetSchema                      = "coimnet-asr-dataset/v1"
	maxDatasetManifestBytes         int64 = 16 << 20
	defaultMaxDatasetRawBytes       int64 = 64 << 20
	defaultMaxDatasetFileProcessed  int64 = 512 << 20
	defaultMaxDatasetTotalProcessed int64 = 2 << 30
)

// DatasetLimits bounds the memory accounted for one dataset import. Raw WAV
// bytes are limited per recording. Processed bytes conservatively count the
// decoded, mixed, resampled and normalised float64 buffers that can coexist for
// one recording; the total limit sums that estimate across all recordings.
type DatasetLimits struct {
	MaxRawBytes            int64
	MaxFileProcessedBytes  int64
	MaxTotalProcessedBytes int64
}

// DefaultDatasetLimits returns the safe limits used by ReadDataset. Callers
// importing a larger licensed corpus can opt into larger, explicit limits with
// ReadDatasetWithLimits after estimating their available memory.
func DefaultDatasetLimits() DatasetLimits {
	return DatasetLimits{
		MaxRawBytes:            defaultMaxDatasetRawBytes,
		MaxFileProcessedBytes:  defaultMaxDatasetFileProcessed,
		MaxTotalProcessedBytes: defaultMaxDatasetTotalProcessed,
	}
}

// License identifies the terms under which a dataset may be used. All three
// fields are required when a manifest is imported.
type License struct {
	Holder string `json:"holder"`
	Terms  string `json:"terms"`
	Source string `json:"source"`
}

// UtteranceMetadata records the source and deterministic preprocessing facts
// for one imported utterance. Metadata[i] belongs to Utterances[i].
type UtteranceMetadata struct {
	OriginalPath       string                `json:"original_path"`
	OriginalSHA256     string                `json:"original_sha256"`
	OriginalSampleRate int                   `json:"original_sample_rate"`
	OriginalChannels   int                   `json:"original_channels"`
	Resample           audio.ResampleReport  `json:"resample"`
	Normalize          audio.NormalizeReport `json:"normalize"`
}

// Dataset is an imported ASR corpus. Utterances contain mono, resampled and
// peak-normalised samples; Metadata preserves the source facts needed to
// audit each corresponding utterance.
type Dataset struct {
	License    License             `json:"license"`
	Utterances []Utterance         `json:"utterances"`
	Metadata   []UtteranceMetadata `json:"metadata"`
}

type datasetManifest struct {
	Schema     *string                 `json:"schema"`
	License    *datasetManifestLicense `json:"license"`
	Recordings []datasetManifestRecord `json:"recordings"`
}

type datasetManifestLicense struct {
	Holder *string `json:"holder"`
	Terms  *string `json:"terms"`
	Source *string `json:"source"`
}

type datasetManifestRecord struct {
	Path       *string `json:"path"`
	Text       *string `json:"text"`
	Speaker    *string `json:"speaker"`
	Session    *string `json:"session"`
	SampleRate *int    `json:"sample_rate"`
	Channels   *int    `json:"channels"`
}

type datasetRecording struct {
	Path       string
	Text       string
	Speaker    string
	Session    string
	SampleRate int
	Channels   int
}

// ReadDataset reads a coimnet-asr-dataset/v1 manifest. Every recording is
// decoded as supported PCM16 WAV, mixed to mono, linearly resampled to
// targetRate and peak-normalised to peak, in that order.
func ReadDataset(manifestPath string, targetRate int, peak float64) (Dataset, error) {
	return ReadDatasetWithLimits(manifestPath, targetRate, peak, DefaultDatasetLimits())
}

// ReadDatasetWithLimits is ReadDataset with explicit per-file and aggregate
// capacity limits. It reads each WAV once, hashes that byte slice, and decodes
// the same byte slice after its shape and processed allocation have passed the
// limits.
func ReadDatasetWithLimits(manifestPath string, targetRate int, peak float64, limits DatasetLimits) (dataset Dataset, retErr error) {
	if targetRate <= 0 {
		return Dataset{}, fmt.Errorf("asr: target rate %d must be positive", targetRate)
	}
	if !(peak > 0 && peak <= 1) {
		return Dataset{}, fmt.Errorf("asr: peak %v must be in (0, 1]", peak)
	}
	if manifestPath == "" {
		return Dataset{}, errors.New("asr: manifest path is empty")
	}
	if err := validateDatasetLimits(limits); err != nil {
		return Dataset{}, err
	}

	manifestAbs, err := filepath.Abs(manifestPath)
	if err != nil {
		return Dataset{}, fmt.Errorf("asr: resolve manifest path: %w", err)
	}
	manifestDir := filepath.Dir(manifestAbs)
	datasetRoot, err := os.OpenRoot(manifestDir)
	if err != nil {
		return Dataset{}, fmt.Errorf("asr: open manifest root %q: %w", manifestDir, err)
	}
	defer func() {
		if closeErr := datasetRoot.Close(); closeErr != nil {
			closeErr = fmt.Errorf("asr: close manifest root %q: %w", manifestDir, closeErr)
			if retErr == nil {
				dataset = Dataset{}
				retErr = closeErr
				return
			}
			retErr = errors.Join(retErr, closeErr)
		}
	}()
	rootPath, err := filepath.EvalSymlinks(manifestDir)
	if err != nil {
		return Dataset{}, fmt.Errorf("asr: resolve manifest root %q: %w", manifestDir, err)
	}
	raw, err := readDatasetManifestFile(datasetRoot, filepath.Base(manifestAbs))
	if err != nil {
		return Dataset{}, fmt.Errorf("asr: read manifest %q: %w", manifestPath, err)
	}
	if !utf8.Valid(raw) {
		return Dataset{}, errors.New("asr: manifest is not valid UTF-8")
	}
	if err := validateJSONUnicode(raw); err != nil {
		return Dataset{}, err
	}

	manifest, err := decodeDatasetManifest(raw)
	if err != nil {
		return Dataset{}, fmt.Errorf("asr: decode manifest %q: %w", manifestPath, err)
	}
	if manifest.Schema == nil {
		return Dataset{}, errors.New("asr: manifest schema must be present and not null")
	}
	if *manifest.Schema != asrDatasetSchema {
		return Dataset{}, fmt.Errorf("asr: manifest schema %q is unsupported, want %q", *manifest.Schema, asrDatasetSchema)
	}
	license, err := materializeLicense(manifest.License)
	if err != nil {
		return Dataset{}, err
	}
	if len(manifest.Recordings) == 0 {
		return Dataset{}, errors.New("asr: manifest recordings must not be empty")
	}

	dataset = Dataset{
		License:    license,
		Utterances: make([]Utterance, 0, len(manifest.Recordings)),
		Metadata:   make([]UtteranceMetadata, 0, len(manifest.Recordings)),
	}
	seen := make(map[string]int, len(manifest.Recordings))
	var totalProcessedBytes int64
	for i, rawRecording := range manifest.Recordings {
		recording, err := materializeRecording(rawRecording, i)
		if err != nil {
			return Dataset{}, err
		}
		utterance, metadata, err := readDatasetRecording(datasetRoot, manifestDir, rootPath, recording, i, targetRate, peak, limits, seen, &totalProcessedBytes)
		if err != nil {
			return Dataset{}, err
		}
		dataset.Utterances = append(dataset.Utterances, utterance)
		dataset.Metadata = append(dataset.Metadata, metadata)
	}
	return dataset, nil
}

func readDatasetManifestFile(root *os.Root, name string) ([]byte, error) {
	raw, _, err := fileio.ReadRootRegular(context.Background(), root, name, maxDatasetManifestBytes)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func validateDatasetLimits(limits DatasetLimits) error {
	for _, field := range []struct {
		name  string
		value int64
	}{
		{name: "max raw bytes", value: limits.MaxRawBytes},
		{name: "max file processed bytes", value: limits.MaxFileProcessedBytes},
		{name: "max total processed bytes", value: limits.MaxTotalProcessedBytes},
	} {
		if field.value <= 0 {
			return fmt.Errorf("asr: %s %d must be positive", field.name, field.value)
		}
	}
	return nil
}

func decodeDatasetManifest(raw []byte) (datasetManifest, error) {
	var manifest datasetManifest
	if err := strictjson.Decode(bytes.NewReader(raw), maxDatasetManifestBytes, &manifest); err != nil {
		return datasetManifest{}, err
	}
	return manifest, nil
}

func materializeLicense(raw *datasetManifestLicense) (License, error) {
	if raw == nil {
		return License{}, errors.New("asr: license must be present and not null")
	}
	fields := []struct {
		name  string
		value *string
	}{
		{name: "holder", value: raw.Holder},
		{name: "terms", value: raw.Terms},
		{name: "source", value: raw.Source},
	}
	var license License
	for _, field := range fields {
		if field.value == nil {
			return License{}, fmt.Errorf("asr: license.%s must be present and not null", field.name)
		}
		switch field.name {
		case "holder":
			license.Holder = *field.value
		case "terms":
			license.Terms = *field.value
		case "source":
			license.Source = *field.value
		}
	}
	if err := validateLicense(license); err != nil {
		return License{}, err
	}
	return license, nil
}

func materializeRecording(raw datasetManifestRecord, index int) (datasetRecording, error) {
	stringFields := []struct {
		name  string
		value *string
	}{
		{name: "path", value: raw.Path},
		{name: "text", value: raw.Text},
		{name: "speaker", value: raw.Speaker},
		{name: "session", value: raw.Session},
	}
	for _, field := range stringFields {
		if field.value == nil {
			return datasetRecording{}, fmt.Errorf("asr: recording %d %s must be present and not null", index, field.name)
		}
	}
	intFields := []struct {
		name  string
		value *int
	}{
		{name: "sample rate", value: raw.SampleRate},
		{name: "channels", value: raw.Channels},
	}
	for _, field := range intFields {
		if field.value == nil {
			return datasetRecording{}, fmt.Errorf("asr: recording %d %s must be present and not null", index, field.name)
		}
	}
	return datasetRecording{
		Path:       *raw.Path,
		Text:       *raw.Text,
		Speaker:    *raw.Speaker,
		Session:    *raw.Session,
		SampleRate: *raw.SampleRate,
		Channels:   *raw.Channels,
	}, nil
}

func validateLicense(license License) error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "holder", value: license.Holder},
		{name: "terms", value: license.Terms},
		{name: "source", value: license.Source},
	} {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("asr: license.%s must be non-empty", field.name)
		}
		if !utf8.ValidString(field.value) {
			return fmt.Errorf("asr: license.%s is not valid UTF-8", field.name)
		}
	}
	return nil
}

// validateJSONUnicode rejects escaped UTF-16 surrogate code points before
// encoding/json can silently replace an unmatched pair with U+FFFD.
func validateJSONUnicode(raw []byte) error {
	for i := 0; i < len(raw); i++ {
		if raw[i] != '"' {
			continue
		}
	scanString:
		for i++; i < len(raw); i++ {
			switch raw[i] {
			case '"':
				break scanString
			case '\\':
				if i+1 >= len(raw) {
					return nil // encoding/json reports the truncated escape.
				}
				if raw[i+1] != 'u' {
					i++
					continue
				}
				if i+6 > len(raw) {
					return nil // encoding/json reports the truncated escape.
				}
				value, ok := jsonHex4(raw[i+2 : i+6])
				if !ok {
					return nil // encoding/json reports the malformed escape.
				}
				switch {
				case value >= 0xD800 && value <= 0xDBFF:
					if i+12 > len(raw) || raw[i+6] != '\\' || raw[i+7] != 'u' {
						return errors.New("asr: manifest contains an unpaired Unicode surrogate")
					}
					low, ok := jsonHex4(raw[i+8 : i+12])
					if !ok || low < 0xDC00 || low > 0xDFFF {
						return errors.New("asr: manifest contains an unpaired Unicode surrogate")
					}
					i += 11
				case value >= 0xDC00 && value <= 0xDFFF:
					return errors.New("asr: manifest contains an unpaired Unicode surrogate")
				default:
					i += 5
				}
			}
		}
	}
	return nil
}

func jsonHex4(raw []byte) (uint16, bool) {
	if len(raw) != 4 {
		return 0, false
	}
	var value uint16
	for _, b := range raw {
		value <<= 4
		switch {
		case b >= '0' && b <= '9':
			value += uint16(b - '0')
		case b >= 'a' && b <= 'f':
			value += uint16(b-'a') + 10
		case b >= 'A' && b <= 'F':
			value += uint16(b-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}

func readDatasetRecording(root *os.Root, manifestDir, rootPath string, recording datasetRecording, index, targetRate int, peak float64, limits DatasetLimits, seen map[string]int, totalProcessedBytes *int64) (Utterance, UtteranceMetadata, error) {
	if !utf8.ValidString(recording.Path) {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: recording %d path is not valid UTF-8", index)
	}
	if strings.TrimSpace(recording.Path) == "" {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: recording %d path must be non-empty", index)
	}
	if filepath.IsAbs(recording.Path) {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: recording %d path %q must be relative to the manifest", index, recording.Path)
	}
	if !utf8.ValidString(recording.Text) {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: recording %d text is not valid UTF-8", index)
	}
	if strings.TrimSpace(recording.Speaker) == "" {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: recording %d speaker must be non-empty", index)
	}
	if strings.TrimSpace(recording.Session) == "" {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: recording %d session must be non-empty", index)
	}
	if recording.SampleRate <= 0 {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: recording %d sample rate %d must be positive", index, recording.SampleRate)
	}
	if recording.Channels <= 0 {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: recording %d channel count %d must be positive", index, recording.Channels)
	}

	relativePath := filepath.Clean(recording.Path)
	path := filepath.Clean(filepath.Join(manifestDir, relativePath))
	if !pathWithin(manifestDir, path) {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: recording %d path %q leaves the manifest directory", index, recording.Path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: recording %d resolve path %q: %w", index, recording.Path, err)
	}
	if !pathWithin(rootPath, resolved) {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: recording %d path %q leaves the manifest directory through a symlink", index, recording.Path)
	}
	if previous, ok := seen[resolved]; ok {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: recording %d path %q duplicates recording %d", index, recording.Path, previous)
	}
	seen[resolved] = index
	raw, _, err := fileio.ReadRootRegular(context.Background(), root, relativePath, limits.MaxRawBytes)
	if err != nil {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: recording %d read raw WAV %q: %w", index, recording.Path, err)
	}
	shaBytes := sha256.Sum256(raw)
	sha := hex.EncodeToString(shaBytes[:])
	info, err := audio.InspectWAV(raw)
	if err != nil {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: recording %d read WAV %q: %w", index, recording.Path, err)
	}
	if info.SampleRate != recording.SampleRate {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: recording %d sample rate declares %d Hz but WAV contains %d Hz", index, recording.SampleRate, info.SampleRate)
	}
	if info.Channels != recording.Channels {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: recording %d channels declares %d but WAV contains %d", index, recording.Channels, info.Channels)
	}
	outputLength, err := audio.ResampleOutputLength(info.Frames, info.SampleRate, targetRate)
	if err != nil {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: recording %d preprocess resample: %w", index, err)
	}
	processedBytes, err := datasetProcessedBytes(info, outputLength)
	if err != nil {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: recording %d processed allocation estimate: %w", index, err)
	}
	if processedBytes > limits.MaxFileProcessedBytes {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: recording %d processed allocation %d bytes exceeds per-file limit %d", index, processedBytes, limits.MaxFileProcessedBytes)
	}
	if *totalProcessedBytes > limits.MaxTotalProcessedBytes-processedBytes {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: total processed allocation would exceed limit %d bytes", limits.MaxTotalProcessedBytes)
	}
	*totalProcessedBytes += processedBytes

	signal, err := audio.DecodeWAV(raw)
	if err != nil {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: recording %d decode WAV %q: %w", index, recording.Path, err)
	}

	mixed := audio.Mixdown(signal)
	resampled, resampleReport, err := audio.Resample(mixed, signal.SampleRate, targetRate)
	if err != nil {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: recording %d preprocess resample: %w", index, err)
	}
	if len(resampled) == 0 {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: recording %d preprocess resample produced zero samples", index)
	}
	normalized, normalizeReport, err := audio.Normalize(resampled, peak)
	if err != nil {
		return Utterance{}, UtteranceMetadata{}, fmt.Errorf("asr: recording %d preprocess normalise: %w", index, err)
	}

	return Utterance{
			Text:    recording.Text,
			Samples: normalized,
			Speaker: recording.Speaker,
			Session: recording.Session,
		}, UtteranceMetadata{
			OriginalPath:       relativePath,
			OriginalSHA256:     sha,
			OriginalSampleRate: signal.SampleRate,
			OriginalChannels:   signal.Channels,
			Resample:           resampleReport,
			Normalize:          normalizeReport,
		}, nil
}

func datasetProcessedBytes(info audio.WAVInfo, outputLength int) (int64, error) {
	decodedSamples, ok := checkedProduct(int64(info.Channels), int64(info.Frames))
	if !ok {
		return 0, errors.New("decoded sample count overflows int64")
	}
	resampledSamples := int64(outputLength)
	if outputLength <= 0 {
		return 0, errors.New("resampled output length must be positive")
	}
	outputBuffers, ok := checkedProduct(resampledSamples, 2)
	if !ok {
		return 0, errors.New("resampled sample buffers overflow int64")
	}
	sampleBuffers, ok := checkedAdd(decodedSamples, int64(info.Frames))
	if !ok {
		return 0, errors.New("decoded and mixed sample buffers overflow int64")
	}
	sampleBuffers, ok = checkedAdd(sampleBuffers, outputBuffers)
	if !ok {
		return 0, errors.New("processed sample buffers overflow int64")
	}
	bytes, ok := checkedProduct(sampleBuffers, 8)
	if !ok {
		return 0, errors.New("processed byte estimate overflows int64")
	}
	return bytes, nil
}

func checkedProduct(a, b int64) (int64, bool) {
	if a < 0 || b < 0 {
		return 0, false
	}
	if a != 0 && b > (int64(^uint64(0)>>1))/a {
		return 0, false
	}
	return a * b, true
}

func checkedAdd(a, b int64) (int64, bool) {
	maxInt64 := int64(^uint64(0) >> 1)
	if a < 0 || b < 0 || a > maxInt64-b {
		return 0, false
	}
	return a + b, true
}

func pathWithin(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
