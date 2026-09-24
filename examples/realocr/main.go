// realocr is an explicitly labelled real-data example. It imports a bounded
// sample of the official KMNIST IDX gzip files, trains tasks/ocr.Recognizer on
// one-character labels, and emits a deterministic JSON report. KMNIST is a
// single-character dataset; this example does not implement page OCR.
package main

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/TimLai666/coimnet/tasks/ocr"
	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

const (
	dataSchemaVersion           = "coimnet-realocr-data/v1"
	reportSchemaVersion         = "coimnet-realocr/v1"
	metadataName                = "metadata.json"
	classMapName                = "kmnist_classmap.csv"
	trainImagesName             = "train-images-idx3-ubyte.gz"
	trainLabelsName             = "train-labels-idx1-ubyte.gz"
	testImagesName              = "t10k-images-idx3-ubyte.gz"
	testLabelsName              = "t10k-labels-idx1-ubyte.gz"
	officialBaseURL             = "https://codh.rois.ac.jp/kmnist/dataset/kmnist/"
	officialRepositoryURL       = "https://github.com/rois-codh/kmnist"
	officialDatasetURL          = "https://codh.rois.ac.jp/kmnist/"
	officialLicenseName         = "CC BY-SA 4.0"
	officialLicenseURL          = "https://creativecommons.org/licenses/by-sa/4.0/"
	imageMagic                  = uint32(2051)
	labelMagic                  = uint32(2049)
	imageRows                   = 28
	imageCols                   = 28
	classCount                  = 10
	defaultMaxCompressedBytes   = int64(24 << 20)
	defaultMaxUncompressedBytes = int64(64 << 20)
	defaultTrainCount           = 1000
	defaultTestCount            = 200
	defaultEpochs               = 1
	defaultHidden               = 24
	defaultLearningRate         = 0.002
	maxClassMapBytes            = int64(1 << 20)
)

// LicenseMetadata records the license that must accompany the imported data.
type LicenseMetadata struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Attribution string `json:"attribution"`
}

// FileMetadata records the expected official URL and, when available, the
// fingerprint captured at download time. Bytes is the compressed file size.
type FileMetadata struct {
	Name            string `json:"name"`
	URL             string `json:"url"`
	SHA256          string `json:"sha256,omitempty"`
	CompressedBytes int64  `json:"compressed_bytes,omitempty"`
}

// ClassMapping is the official KMNIST label-to-modern-hiragana mapping.
type ClassMapping struct {
	Index     int    `json:"index"`
	Codepoint string `json:"codepoint"`
	Char      string `json:"char"`
}

// Metadata is the required sidecar for a real-data run. It is intentionally
// strict so a local file cannot silently be treated as official data.
type Metadata struct {
	SchemaVersion string          `json:"schema_version"`
	Dataset       string          `json:"dataset"`
	RepositoryURL string          `json:"repository_url"`
	DatasetURL    string          `json:"dataset_url"`
	License       LicenseMetadata `json:"license"`
	Files         []FileMetadata  `json:"files"`
	Mapping       []ClassMapping  `json:"mapping"`
}

// DatasetLimits bounds both selected examples and the complete compressed and
// decompressed files that are parsed. The complete IDX stream is consumed so
// truncation and trailing corruption cannot be hidden by a small sample.
type DatasetLimits struct {
	TrainCount           int
	TestCount            int
	MaxCompressedBytes   int64
	MaxUncompressedBytes int64
}

// Dataset is the selected real-data split plus the verified source metadata.
type Dataset struct {
	Metadata         Metadata
	MetadataSHA256   string
	ClassMapSHA256   string
	TrainTotal       int
	TestTotal        int
	Train            []glyphs.Sample
	Test             []glyphs.Sample
	TrainIndices     []int
	TestIndices      []int
	FileFingerprints []SourceFile
}

// RunConfig fixes the experiment. The selection policy is part of the config
// hash and keeps train/test selection deterministic without touching the test
// split during training.
type RunConfig struct {
	Seed                 uint64  `json:"seed"`
	TrainCount           int     `json:"train_count"`
	TestCount            int     `json:"test_count"`
	Epochs               int     `json:"epochs"`
	Hidden               int     `json:"hidden"`
	LearningRate         float64 `json:"learning_rate"`
	MaxCompressedBytes   int64   `json:"max_compressed_bytes"`
	MaxUncompressedBytes int64   `json:"max_uncompressed_bytes"`
	Selection            string  `json:"selection"`
}

// EvalResult keeps the two requested before/after metrics compact and
// deterministic. CER is over Unicode code points, as implemented by tasks/ocr.
type EvalResult struct {
	CER     float64 `json:"cer"`
	Exact   float64 `json:"exact_lines"`
	Samples int     `json:"samples"`
}

// SourceFile is a verified source fingerprint included in every report.
type SourceFile struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// SourceReport records the source, license, mapping, and actual file hashes.
type SourceReport struct {
	Dataset        string          `json:"dataset"`
	RepositoryURL  string          `json:"repository_url"`
	DatasetURL     string          `json:"dataset_url"`
	License        LicenseMetadata `json:"license"`
	Files          []SourceFile    `json:"files"`
	ClassMapSHA256 string          `json:"classmap_sha256"`
	MetadataSHA256 string          `json:"metadata_sha256"`
	TrainTotal     int             `json:"train_total"`
	TestTotal      int             `json:"test_total"`
	ImageFormat    string          `json:"image_format"`
	LabelFormat    string          `json:"label_format"`
	ImageHeight    int             `json:"image_height"`
	ImageWidth     int             `json:"image_width"`
	Mapping        []ClassMapping  `json:"mapping"`
}

// ProgramFingerprint identifies both the source file used and the executable
// that ran the example. The binary hash is useful when go run is used.
type ProgramFingerprint struct {
	GoVersion    string `json:"go_version"`
	GOOS         string `json:"goos"`
	GOARCH       string `json:"goarch"`
	SourceSHA256 string `json:"source_sha256"`
	BinarySHA256 string `json:"binary_sha256"`
	VCSRevision  string `json:"vcs_revision,omitempty"`
}

// TrainSummary records the actual isolated train split and skipped CTC lines.
type TrainSummary struct {
	Samples              int    `json:"samples"`
	Epochs               int    `json:"epochs"`
	Updates              int    `json:"updates"`
	ImpossibleAlignments int    `json:"impossible_alignments"`
	IndicesSHA256        string `json:"indices_sha256"`
	TestIndicesSHA256    string `json:"test_indices_sha256"`
}

// Report is deterministic JSON. Runtime timing is printed separately to
// stderr, so two identical runs can be compared byte-for-byte.
type Report struct {
	SchemaVersion  string             `json:"schema_version"`
	Example        string             `json:"example"`
	DataScope      DataScope          `json:"data_scope"`
	Source         SourceReport       `json:"source"`
	Program        ProgramFingerprint `json:"program"`
	Config         RunConfig          `json:"config"`
	ConfigHash     string             `json:"config_hash"`
	Before         EvalResult         `json:"before"`
	After          EvalResult         `json:"after"`
	Train          TrainSummary       `json:"train"`
	ModelPersisted bool               `json:"model_persisted"`
	Limitations    []string           `json:"limitations"`
}

// DataScope prevents a one-character benchmark from being mistaken for page
// OCR or natural Japanese text recognition.
type DataScope struct {
	Data           string `json:"data"`
	Language       string `json:"language"`
	Resolution     string `json:"resolution"`
	SequenceLength string `json:"sequence_length"`
	Periphery      string `json:"periphery"`
}

type splitLabels struct {
	Count  int
	Labels []int
}

func main() {
	var (
		dataDir         = flag.String("data-dir", "", "directory containing official KMNIST IDX gzip files and metadata.json")
		trainCount      = flag.Int("train-count", defaultTrainCount, "number of balanced train examples to select")
		testCount       = flag.Int("test-count", defaultTestCount, "number of balanced test examples to select")
		epochs          = flag.Int("epochs", defaultEpochs, "training epochs over the selected train split")
		hidden          = flag.Int("hidden", defaultHidden, "recognizer recurrent hidden nodes")
		learningRate    = flag.Float64("learning-rate", defaultLearningRate, "positive finite AdamW learning rate")
		seed            = flag.Uint64("seed", 20260924, "deterministic recognizer initialization seed")
		maxCompressed   = flag.Int64("max-compressed-bytes", defaultMaxCompressedBytes, "maximum compressed bytes per IDX gzip")
		maxUncompressed = flag.Int64("max-uncompressed-bytes", defaultMaxUncompressedBytes, "maximum uncompressed bytes per IDX gzip")
		reportPath      = flag.String("report", "", "optional path for the deterministic JSON report")
	)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "realocr: EXAMPLE ONLY; run tasks/ocr.Recognizer on single-character KMNIST data, not page OCR\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *dataDir == "" {
		flag.Usage()
		os.Exit(2)
	}
	ctx := context.Background()
	start := time.Now()
	dataset, err := LoadDataset(*dataDir, DatasetLimits{
		TrainCount: *trainCount, TestCount: *testCount,
		MaxCompressedBytes: *maxCompressed, MaxUncompressedBytes: *maxUncompressed,
	})
	if err != nil {
		fatal(err)
	}
	report, err := Run(ctx, dataset, RunConfig{
		Seed: *seed, TrainCount: *trainCount, TestCount: *testCount, Epochs: *epochs,
		Hidden: *hidden, LearningRate: *learningRate,
		MaxCompressedBytes: *maxCompressed, MaxUncompressedBytes: *maxUncompressed,
		Selection: "balanced_round_robin_by_label_v1",
	})
	if err != nil {
		fatal(err)
	}
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fatal(err)
	}
	b = append(b, '\n')
	if *reportPath != "" {
		if err := os.WriteFile(*reportPath, b, 0o644); err != nil {
			fatal(fmt.Errorf("write report: %w", err))
		}
	}
	if _, err := os.Stdout.Write(b); err != nil {
		fatal(fmt.Errorf("write report to stdout: %w", err))
	}
	fmt.Fprintf(os.Stderr, "elapsed_ms=%d\n", time.Since(start).Milliseconds())
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "realocr: %v\n", err)
	os.Exit(1)
}

// LoadDataset strictly validates metadata, the official classmap, and all four
// IDX streams before selecting a balanced bounded sample.
func LoadDataset(dir string, limits DatasetLimits) (Dataset, error) {
	var zero Dataset
	if dir == "" {
		return zero, errors.New("realocr: data directory is empty")
	}
	if limits.TrainCount < 1 || limits.TestCount < 1 {
		return zero, errors.New("realocr: train and test counts must be positive")
	}
	if limits.MaxCompressedBytes == 0 {
		limits.MaxCompressedBytes = defaultMaxCompressedBytes
	}
	if limits.MaxCompressedBytes < 0 {
		return zero, errors.New("realocr: max compressed bytes must be positive")
	}
	if limits.MaxUncompressedBytes == 0 {
		limits.MaxUncompressedBytes = defaultMaxUncompressedBytes
	}
	if limits.MaxUncompressedBytes < 0 {
		return zero, errors.New("realocr: max uncompressed bytes must be positive")
	}
	metaPath := filepath.Join(dir, metadataName)
	meta, metaHash, err := readMetadata(metaPath)
	if err != nil {
		return zero, err
	}
	mapping, classHash, err := readClassMap(filepath.Join(dir, classMapName), meta.Mapping)
	if err != nil {
		return zero, err
	}
	if expected := metadataFile(meta, classMapName); expected.SHA256 != "" && !strings.EqualFold(expected.SHA256, classHash) {
		return zero, fmt.Errorf("realocr: source hash mismatch for %s", classMapName)
	}
	if len(mapping) != classCount {
		return zero, fmt.Errorf("realocr: classmap has %d classes, want %d", len(mapping), classCount)
	}
	trainLabels, err := readIDXLabels(filepath.Join(dir, trainLabelsName), limits)
	if err != nil {
		return zero, fmt.Errorf("realocr: read training labels: %w", err)
	}
	testLabels, err := readIDXLabels(filepath.Join(dir, testLabelsName), limits)
	if err != nil {
		return zero, fmt.Errorf("realocr: read test labels: %w", err)
	}
	if limits.TrainCount > trainLabels.Count || limits.TestCount > testLabels.Count {
		return zero, fmt.Errorf("realocr: requested sample count exceeds IDX count (train=%d/%d test=%d/%d)", limits.TrainCount, trainLabels.Count, limits.TestCount, testLabels.Count)
	}
	trainIndices, err := balancedIndices(trainLabels.Labels, limits.TrainCount, classCount)
	if err != nil {
		return zero, fmt.Errorf("realocr: training selection: %w", err)
	}
	testIndices, err := balancedIndices(testLabels.Labels, limits.TestCount, classCount)
	if err != nil {
		return zero, fmt.Errorf("realocr: test selection: %w", err)
	}
	trainImages, err := readIDXImages(filepath.Join(dir, trainImagesName), trainLabels.Count, trainIndices, limits)
	if err != nil {
		return zero, fmt.Errorf("realocr: read training images: %w", err)
	}
	testImages, err := readIDXImages(filepath.Join(dir, testImagesName), testLabels.Count, testIndices, limits)
	if err != nil {
		return zero, fmt.Errorf("realocr: read test images: %w", err)
	}
	train, err := makeSamples(trainImages, trainIndices, trainLabels.Labels, mapping)
	if err != nil {
		return zero, fmt.Errorf("realocr: training samples: %w", err)
	}
	test, err := makeSamples(testImages, testIndices, testLabels.Labels, mapping)
	if err != nil {
		return zero, fmt.Errorf("realocr: test samples: %w", err)
	}
	fingerprints, err := sourceFingerprints(dir, meta)
	if err != nil {
		return zero, err
	}
	return Dataset{
		Metadata: meta, MetadataSHA256: metaHash, ClassMapSHA256: classHash,
		TrainTotal: trainLabels.Count, TestTotal: testLabels.Count,
		Train: train, Test: test, TrainIndices: trainIndices, TestIndices: testIndices,
		FileFingerprints: fingerprints,
	}, nil
}

func readMetadata(path string) (Metadata, string, error) {
	var meta Metadata
	b, err := os.ReadFile(path)
	if err != nil {
		return meta, "", fmt.Errorf("realocr: read %s: %w", path, err)
	}
	if !utf8.Valid(b) {
		return meta, "", errors.New("realocr: metadata is not valid UTF-8")
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&meta); err != nil {
		return meta, "", fmt.Errorf("realocr: decode metadata: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return meta, "", errors.New("realocr: metadata has trailing JSON")
	}
	if err := validateMetadata(meta); err != nil {
		return meta, "", err
	}
	return meta, digestBytes(b), nil
}

func validateMetadata(meta Metadata) error {
	if meta.SchemaVersion != dataSchemaVersion {
		return fmt.Errorf("realocr: metadata schema %q, want %q", meta.SchemaVersion, dataSchemaVersion)
	}
	if meta.Dataset != "Kuzushiji-MNIST" {
		return fmt.Errorf("realocr: metadata dataset %q is not Kuzushiji-MNIST", meta.Dataset)
	}
	if meta.RepositoryURL != officialRepositoryURL || meta.DatasetURL != officialDatasetURL {
		return errors.New("realocr: metadata does not identify the official ROIS-CODH source")
	}
	if meta.License.Name != officialLicenseName || meta.License.URL != officialLicenseURL ||
		!strings.Contains(meta.License.Attribution, "KMNIST Dataset") || !strings.Contains(meta.License.Attribution, "CODH") {
		return errors.New("realocr: metadata must declare CC BY-SA 4.0 and the KMNIST CODH attribution")
	}
	want := []string{trainImagesName, trainLabelsName, testImagesName, testLabelsName, classMapName}
	if len(meta.Files) != len(want) {
		return fmt.Errorf("realocr: metadata lists %d files, want %d", len(meta.Files), len(want))
	}
	seen := make(map[string]bool, len(meta.Files))
	for _, file := range meta.Files {
		if seen[file.Name] {
			return fmt.Errorf("realocr: metadata repeats file %q", file.Name)
		}
		seen[file.Name] = true
		if !contains(want, file.Name) || file.URL != officialBaseURL+file.Name {
			return fmt.Errorf("realocr: metadata file %q is not the official KMNIST URL", file.Name)
		}
		if file.SHA256 != "" {
			if len(file.SHA256) != sha256.Size*2 {
				return fmt.Errorf("realocr: metadata hash for %q is not SHA-256", file.Name)
			}
			if _, err := hex.DecodeString(file.SHA256); err != nil {
				return fmt.Errorf("realocr: metadata hash for %q is invalid: %w", file.Name, err)
			}
		}
		if file.CompressedBytes < 0 {
			return fmt.Errorf("realocr: metadata compressed size for %q is negative", file.Name)
		}
	}
	if len(meta.Mapping) != classCount {
		return fmt.Errorf("realocr: metadata mapping has %d classes, want %d", len(meta.Mapping), classCount)
	}
	for i, mapping := range meta.Mapping {
		if mapping.Index != i {
			return fmt.Errorf("realocr: metadata mapping index %d at row %d", mapping.Index, i)
		}
		if err := validateMapping(mapping); err != nil {
			return fmt.Errorf("realocr: metadata mapping row %d: %w", i, err)
		}
	}
	return nil
}

func validateMapping(mapping ClassMapping) error {
	if !strings.HasPrefix(mapping.Codepoint, "U+") {
		return errors.New("codepoint must start with U+")
	}
	n, err := strconv.ParseUint(mapping.Codepoint[2:], 16, 21)
	if err != nil || n > 0x10ffff || (n >= 0xd800 && n <= 0xdfff) {
		return fmt.Errorf("invalid codepoint %q", mapping.Codepoint)
	}
	rs := []rune(mapping.Char)
	if len(rs) != 1 || uint64(rs[0]) != n {
		return fmt.Errorf("character %q does not match %q", mapping.Char, mapping.Codepoint)
	}
	return nil
}

func readClassMap(path string, expected []ClassMapping) ([]ClassMapping, string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", fmt.Errorf("realocr: stat classmap: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, "", errors.New("realocr: classmap is not a regular file")
	}
	if info.Size() > maxClassMapBytes {
		return nil, "", fmt.Errorf("realocr: classmap is %d bytes, exceeds limit %d", info.Size(), maxClassMapBytes)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("realocr: read classmap: %w", err)
	}
	if !utf8.Valid(b) {
		return nil, "", errors.New("realocr: classmap is not valid UTF-8")
	}
	r := csv.NewReader(strings.NewReader(string(b)))
	r.FieldsPerRecord = 3
	header, err := r.Read()
	if err != nil {
		return nil, "", fmt.Errorf("realocr: read classmap header: %w", err)
	}
	if strings.Join(header, ",") != "index,codepoint,char" {
		return nil, "", fmt.Errorf("realocr: classmap header %q is invalid", strings.Join(header, ","))
	}
	got := make([]ClassMapping, 0, classCount)
	for {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("realocr: read classmap row: %w", err)
		}
		index, err := strconv.Atoi(record[0])
		if err != nil {
			return nil, "", fmt.Errorf("realocr: classmap index %q: %w", record[0], err)
		}
		mapping := ClassMapping{Index: index, Codepoint: record[1], Char: record[2]}
		if err := validateMapping(mapping); err != nil {
			return nil, "", fmt.Errorf("realocr: classmap row %d: %w", len(got), err)
		}
		got = append(got, mapping)
	}
	if len(got) != len(expected) {
		return nil, "", fmt.Errorf("realocr: classmap has %d rows, metadata has %d", len(got), len(expected))
	}
	for i := range got {
		if got[i] != expected[i] {
			return nil, "", fmt.Errorf("realocr: classmap row %d does not match metadata", i)
		}
	}
	return got, digestBytes(b), nil
}

func readIDXLabels(path string, limits DatasetLimits) (splitLabels, error) {
	var zero splitLabels
	b, err := readGzipPayload(path, 8, limits)
	if err != nil {
		return zero, err
	}
	if len(b) < 8 || binary.BigEndian.Uint32(b[0:4]) != labelMagic {
		return zero, errors.New("invalid IDX label magic")
	}
	count64 := uint64(binary.BigEndian.Uint32(b[4:8]))
	if count64 == 0 || count64 > uint64(math.MaxInt) || 8+count64 > uint64(len(b)) {
		return zero, errors.New("IDX label payload is truncated or has an invalid count")
	}
	if uint64(len(b)) != 8+count64 {
		return zero, fmt.Errorf("IDX label payload has %d bytes, want %d", len(b), 8+count64)
	}
	labels := make([]int, int(count64))
	for i, label := range b[8:] {
		if label >= classCount {
			return zero, fmt.Errorf("label %d at index %d is outside [0,%d)", label, i, classCount)
		}
		labels[i] = int(label)
	}
	return splitLabels{Count: int(count64), Labels: labels}, nil
}

func readIDXImages(path string, expectedCount int, indices []int, limits DatasetLimits) ([]glyphs.Image, error) {
	minimum := uint64(16)
	b, err := readGzipPayload(path, minimum, limits)
	if err != nil {
		return nil, err
	}
	if len(b) < 16 || binary.BigEndian.Uint32(b[0:4]) != imageMagic {
		return nil, errors.New("invalid IDX image magic")
	}
	count64 := uint64(binary.BigEndian.Uint32(b[4:8]))
	rows := binary.BigEndian.Uint32(b[8:12])
	cols := binary.BigEndian.Uint32(b[12:16])
	if count64 != uint64(expectedCount) {
		return nil, fmt.Errorf("IDX image count %d does not match labels %d", count64, expectedCount)
	}
	if rows != imageRows || cols != imageCols {
		return nil, fmt.Errorf("IDX image shape %dx%d, want %dx%d", rows, cols, imageRows, imageCols)
	}
	pixelsPerImage := uint64(rows) * uint64(cols)
	want := uint64(16) + count64*pixelsPerImage
	if want > uint64(len(b)) {
		return nil, errors.New("IDX image payload is truncated")
	}
	if uint64(len(b)) != want {
		return nil, fmt.Errorf("IDX image payload has %d bytes, want %d", len(b), want)
	}
	position := make(map[int]int, len(indices))
	for i, index := range indices {
		if index < 0 || index >= expectedCount {
			return nil, fmt.Errorf("selected image index %d outside [0,%d)", index, expectedCount)
		}
		if _, exists := position[index]; exists {
			return nil, fmt.Errorf("selected image index %d is repeated", index)
		}
		position[index] = i
	}
	images := make([]glyphs.Image, len(indices))
	for index, out := range position {
		start64 := uint64(16) + uint64(index)*pixelsPerImage
		start := int(start64)
		pixels := make([]float64, int(pixelsPerImage))
		for i, pixel := range b[start : start+int(pixelsPerImage)] {
			pixels[i] = float64(pixel) / 255
		}
		images[out] = glyphs.Image{Width: imageCols, Height: imageRows, Pixels: pixels}
	}
	return images, nil
}

func readGzipPayload(path string, headerBytes uint64, limits DatasetLimits) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	if info.Size() > limits.MaxCompressedBytes {
		return nil, fmt.Errorf("compressed file %s is %d bytes, exceeds limit %d", path, info.Size(), limits.MaxCompressedBytes)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(io.LimitReader(f, limits.MaxCompressedBytes+1))
	if err != nil {
		return nil, fmt.Errorf("open gzip: %w", err)
	}
	// The header is read before any payload allocation. A first bounded read
	// gives the caller a useful malformed/truncated error without trusting the
	// compressed stream's declared dimensions.
	header := make([]byte, headerBytes)
	if _, err := io.ReadFull(gz, header); err != nil {
		gz.Close()
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			return nil, errors.New("truncated gzip payload header")
		}
		return nil, fmt.Errorf("read gzip payload header: %w", err)
	}
	remaining, err := io.ReadAll(io.LimitReader(gz, limits.MaxUncompressedBytes-int64(headerBytes)+1))
	if err != nil {
		gz.Close()
		return nil, fmt.Errorf("read gzip payload: %w", err)
	}
	if int64(headerBytes)+int64(len(remaining)) > limits.MaxUncompressedBytes {
		gz.Close()
		return nil, fmt.Errorf("uncompressed file %s exceeds limit %d", path, limits.MaxUncompressedBytes)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("close gzip payload: %w", err)
	}
	return append(header, remaining...), nil
}

func balancedIndices(labels []int, limit, classes int) ([]int, error) {
	if limit < 1 || classes < 1 {
		return nil, errors.New("selection limit and class count must be positive")
	}
	byClass := make([][]int, classes)
	for index, label := range labels {
		if label < 0 || label >= classes {
			return nil, fmt.Errorf("label %d at index %d is outside class count", label, index)
		}
		byClass[label] = append(byClass[label], index)
	}
	out := make([]int, 0, limit)
	for round := 0; len(out) < limit; round++ {
		progress := false
		for class := 0; class < classes && len(out) < limit; class++ {
			if round < len(byClass[class]) {
				out = append(out, byClass[class][round])
				progress = true
			}
		}
		if !progress {
			return nil, fmt.Errorf("only %d balanced examples available for requested %d", len(out), limit)
		}
	}
	return out, nil
}

func makeSamples(images []glyphs.Image, indices []int, labels []int, mapping []ClassMapping) ([]glyphs.Sample, error) {
	if len(images) != len(indices) {
		return nil, errors.New("image and selection lengths differ")
	}
	out := make([]glyphs.Sample, len(images))
	for i, index := range indices {
		if index < 0 || index >= len(labels) || labels[index] < 0 || labels[index] >= len(mapping) {
			return nil, fmt.Errorf("sample index %d has invalid label", index)
		}
		out[i] = glyphs.Sample{Text: mapping[labels[index]].Char, Image: images[i]}
	}
	return out, nil
}

// Run uses only dataset.Train for updates and evaluates only dataset.Test.
// It intentionally never snapshots or writes the recognizer model.
func Run(ctx context.Context, dataset Dataset, config RunConfig) (Report, error) {
	var zero Report
	if err := validateRunConfig(config, dataset); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	alphabet := make([]rune, len(dataset.Metadata.Mapping))
	for i, mapping := range dataset.Metadata.Mapping {
		alphabet[i] = []rune(mapping.Char)[0]
	}
	recognizer, err := ocr.NewRecognizer(ocr.RecognizerConfig{
		Height: imageRows, Hidden: config.Hidden, Alphabet: alphabet,
		LearningRate: config.LearningRate, Seed: config.Seed,
	})
	if err != nil {
		return zero, fmt.Errorf("realocr: create recognizer: %w", err)
	}
	beforeReport, err := recognizer.Evaluate(ctx, dataset.Test)
	if err != nil {
		return zero, fmt.Errorf("realocr: evaluate before training: %w", err)
	}
	updates, impossible := 0, 0
	for epoch := 0; epoch < config.Epochs; epoch++ {
		for i, sample := range dataset.Train {
			if err := ctx.Err(); err != nil {
				return zero, err
			}
			line, err := recognizer.TrainLine(ctx, sample.Image, sample.Text)
			if err != nil {
				return zero, fmt.Errorf("realocr: train epoch %d sample %d: %w", epoch, i, err)
			}
			if line.Impossible {
				impossible++
			} else {
				updates++
			}
		}
	}
	afterReport, err := recognizer.Evaluate(ctx, dataset.Test)
	if err != nil {
		return zero, fmt.Errorf("realocr: evaluate after training: %w", err)
	}
	configHash, err := digestJSON(config)
	if err != nil {
		return zero, err
	}
	return Report{
		SchemaVersion: reportSchemaVersion,
		Example:       "realocr: official KMNIST single-character OCR example",
		DataScope: DataScope{
			Data:           "official ROIS-CODH Kuzushiji-MNIST handwritten/cursive character images, one character per 28x28 sample",
			Language:       "Japanese hiragana label subset: 10 official KMNIST classes",
			Resolution:     "28x28 grayscale, IDX unsigned bytes normalized to [0,1]",
			SequenceLength: "one Unicode character per sample; 28 image columns are read as the CTC time sequence",
			Periphery:      "identity row encoder, recurrent continuous core, per-column linear readout, greedy CTC decoder",
		},
		Source: SourceReport{
			Dataset: dataset.Metadata.Dataset, RepositoryURL: dataset.Metadata.RepositoryURL,
			DatasetURL: dataset.Metadata.DatasetURL, License: dataset.Metadata.License,
			Files:          append([]SourceFile(nil), dataset.FileFingerprints...),
			ClassMapSHA256: dataset.ClassMapSHA256, MetadataSHA256: dataset.MetadataSHA256,
			TrainTotal: dataset.TrainTotal, TestTotal: dataset.TestTotal,
			ImageFormat: "IDX-3 unsigned-byte payload inside gzip",
			LabelFormat: "IDX-1 unsigned-byte payload inside gzip",
			ImageHeight: imageRows, ImageWidth: imageCols,
			Mapping: append([]ClassMapping(nil), dataset.Metadata.Mapping...),
		},
		Program: programFingerprint(), Config: config, ConfigHash: configHash,
		Before: evalResult(beforeReport), After: evalResult(afterReport),
		Train: TrainSummary{
			Samples: len(dataset.Train), Epochs: config.Epochs, Updates: updates,
			ImpossibleAlignments: impossible,
			IndicesSHA256:        digestIntSlice(dataset.TrainIndices),
			TestIndicesSHA256:    digestIntSlice(dataset.TestIndices),
		},
		ModelPersisted: false,
		Limitations: []string{
			"This is a single-character handwritten/cursive image task, not natural page OCR.",
			"It has no page layout, line segmentation, reading order, multi-character text, or manuscript context.",
			"The report uses bounded balanced subsets rather than the full official train/test sets.",
			"No trained model or checkpoint is saved by this example; only the deterministic report is emitted.",
		},
	}, nil
}

func validateRunConfig(config RunConfig, dataset Dataset) error {
	if config.TrainCount != len(dataset.Train) || config.TestCount != len(dataset.Test) {
		return errors.New("realocr: run counts do not match loaded dataset")
	}
	if config.Epochs < 1 || config.Hidden < 2 || !(config.LearningRate > 0) || math.IsInf(config.LearningRate, 0) || math.IsNaN(config.LearningRate) {
		return errors.New("realocr: invalid training configuration")
	}
	if config.Selection != "balanced_round_robin_by_label_v1" {
		return fmt.Errorf("realocr: unknown selection policy %q", config.Selection)
	}
	return nil
}

func evalResult(report ocr.EvalReport) EvalResult {
	return EvalResult{CER: report.CER.Rate, Exact: report.ExactLines, Samples: report.Samples}
}

func sourceFingerprints(dir string, meta Metadata) ([]SourceFile, error) {
	want := []string{trainImagesName, trainLabelsName, testImagesName, testLabelsName, classMapName}
	out := make([]SourceFile, 0, len(want))
	byName := make(map[string]FileMetadata, len(meta.Files))
	for _, file := range meta.Files {
		byName[file.Name] = file
	}
	for _, name := range want {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("realocr: stat source %s: %w", name, err)
		}
		hash, err := sha256File(path)
		if err != nil {
			return nil, fmt.Errorf("realocr: hash source %s: %w", name, err)
		}
		expected := byName[name]
		if expected.SHA256 != "" && !strings.EqualFold(expected.SHA256, hash) {
			return nil, fmt.Errorf("realocr: source hash mismatch for %s", name)
		}
		if expected.CompressedBytes != 0 && expected.CompressedBytes != info.Size() {
			return nil, fmt.Errorf("realocr: source size mismatch for %s", name)
		}
		out = append(out, SourceFile{Name: name, URL: expected.URL, SHA256: hash, Bytes: info.Size()})
	}
	return out, nil
}

func metadataFile(meta Metadata, name string) FileMetadata {
	for _, file := range meta.Files {
		if file.Name == name {
			return file
		}
	}
	return FileMetadata{}
}

func programFingerprint() ProgramFingerprint {
	fingerprint := ProgramFingerprint{GoVersion: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	if _, path, _, ok := runtime.Caller(0); ok {
		if hash, err := sha256File(path); err == nil {
			fingerprint.SourceSHA256 = hash
		}
	}
	if path, err := os.Executable(); err == nil {
		if hash, err := sha256File(path); err == nil {
			fingerprint.BinarySHA256 = hash
		}
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				fingerprint.VCSRevision = setting.Value
			}
		}
	}
	return fingerprint
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func digestBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func digestJSON(value any) (string, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("realocr: hash JSON: %w", err)
	}
	return digestBytes(b), nil
}

func digestIntSlice(values []int) string {
	h := sha256.New()
	for _, value := range values {
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(value))
		_, _ = h.Write(b[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
