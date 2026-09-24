// Package trajectory imports real two-dimensional trajectories and turns them
// into source-isolated, causal next-displacement samples. The importer only
// consumes the metadata needed to identify a trial and the observed time and
// position columns. Reward, fictive-target, condition and segment values are
// never part of a sample input.
package trajectory

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	// SchemaVersion identifies the imported data contract.
	SchemaVersion = "coimnet-nav2d-trajectory/v1"
	// SampleRuleVersion identifies the causal sample construction rule.
	SampleRuleVersion = "coimnet-nav2d-causal-next-displacement/v1"

	defaultMaxCompressedBytes   int64 = 64 << 20
	defaultMaxUncompressedBytes int64 = 512 << 20
	defaultMaxRows                    = 2_000_000
	defaultMaxFields                  = 128
	defaultMaxFieldBytes        int64 = 1 << 20
)

// License records the holder, terms and provenance URL for an imported source.
// All fields are required so a real-data dataset cannot silently lose its
// authorization context.
type License struct {
	Holder string `json:"holder"`
	Terms  string `json:"terms"`
	Source string `json:"source"`
}

// Source identifies the exact original file and its authorization metadata.
// SHA256 is the SHA-256 digest of the bytes at Path, including gzip bytes when
// Path is compressed. PinnedCommit is optional for non-repository sources; if
// supplied it must be a full 40-hex Git commit.
type Source struct {
	ID           string  `json:"id"`
	URL          string  `json:"url"`
	DOI          string  `json:"doi,omitempty"`
	PinnedCommit string  `json:"pinned_commit,omitempty"`
	SHA256       string  `json:"sha256"`
	License      License `json:"license"`
}

// Validate checks source identity, the original-file digest format and the
// required authorization fields. Read performs the same check before parsing.
func (s Source) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return errors.New("trajectory: source id is required")
	}
	if strings.TrimSpace(s.URL) == "" && strings.TrimSpace(s.DOI) == "" {
		return errors.New("trajectory: source url or doi is required")
	}
	if s.PinnedCommit != "" {
		if len(s.PinnedCommit) != 40 {
			return fmt.Errorf("trajectory: pinned commit %q must be 40 hex characters", s.PinnedCommit)
		}
		if _, err := hex.DecodeString(s.PinnedCommit); err != nil {
			return fmt.Errorf("trajectory: pinned commit %q is not hexadecimal: %w", s.PinnedCommit, err)
		}
	}
	if _, err := normalizeSHA256(s.SHA256); err != nil {
		return fmt.Errorf("trajectory: source sha256: %w", err)
	}
	if strings.TrimSpace(s.License.Holder) == "" {
		return errors.New("trajectory: license.holder is required")
	}
	if strings.TrimSpace(s.License.Terms) == "" {
		return errors.New("trajectory: license.terms is required")
	}
	if strings.TrimSpace(s.License.Source) == "" {
		return errors.New("trajectory: license.source is required")
	}
	return nil
}

// Limits bounds compressed bytes, decompressed bytes, rows, columns and one
// field. The defaults admit the first Titova et al. 2023 gzip source while
// preventing an accidental unbounded read.
type Limits struct {
	MaxCompressedBytes   int64
	MaxUncompressedBytes int64
	MaxRows              int
	MaxFields            int
	MaxFieldBytes        int64
}

// DefaultLimits returns the bounded import limits used for the real trajectory
// source.
func DefaultLimits() Limits {
	return Limits{
		MaxCompressedBytes:   defaultMaxCompressedBytes,
		MaxUncompressedBytes: defaultMaxUncompressedBytes,
		MaxRows:              defaultMaxRows,
		MaxFields:            defaultMaxFields,
		MaxFieldBytes:        defaultMaxFieldBytes,
	}
}

func normalizeLimits(limits Limits) (Limits, error) {
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	fields := []struct {
		name  string
		value int64
	}{
		{name: "max compressed bytes", value: limits.MaxCompressedBytes},
		{name: "max uncompressed bytes", value: limits.MaxUncompressedBytes},
		{name: "max rows", value: int64(limits.MaxRows)},
		{name: "max fields", value: int64(limits.MaxFields)},
		{name: "max field bytes", value: limits.MaxFieldBytes},
	}
	for _, field := range fields {
		if field.value <= 0 {
			return Limits{}, fmt.Errorf("trajectory: %s %d must be positive", field.name, field.value)
		}
	}
	return limits, nil
}

// Point is one observed trajectory position. FName and Fly define TrialID.
// Condition and Segment are kept solely for provenance and boundary checks;
// they are never copied to Input.
type Point struct {
	TrialID   string  `json:"trial_id"`
	FName     string  `json:"fname"`
	Fly       string  `json:"fly"`
	Condition string  `json:"condition"`
	Segment   string  `json:"segment"`
	T         float64 `json:"t"`
	XCM       float64 `json:"x_cm"`
	YCM       float64 `json:"y_cm"`
}

// Dataset is one imported source. Rows stay in source order. The importer
// rejects interleaved or non-increasing rows instead of sorting away source
// corruption.
type Dataset struct {
	Schema string  `json:"schema"`
	Source Source  `json:"source"`
	Rows   []Point `json:"rows"`
}

// Read imports a CSV or gzip-compressed CSV, verifies the exact original-file
// SHA-256 and required source/license metadata, and validates the required
// trajectory columns. Rows for a trial must be contiguous and strictly
// increasing in t. Unknown columns are accepted as source data but ignored by
// the model adapter.
func Read(ctx context.Context, path string, source Source, limits Limits) (dataset Dataset, retErr error) {
	if ctx == nil {
		return Dataset{}, errors.New("trajectory: context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return Dataset{}, fmt.Errorf("trajectory: read: %w", err)
	}
	if strings.TrimSpace(path) == "" {
		return Dataset{}, errors.New("trajectory: path is empty")
	}
	if err := source.Validate(); err != nil {
		return Dataset{}, err
	}
	limits, err := normalizeLimits(limits)
	if err != nil {
		return Dataset{}, err
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return Dataset{}, fmt.Errorf("trajectory: resolve path: %w", err)
	}
	f, err := os.Open(abs)
	if err != nil {
		return Dataset{}, fmt.Errorf("trajectory: open %q: %w", path, err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			if retErr == nil {
				dataset = Dataset{}
				retErr = fmt.Errorf("trajectory: close %q: %w", path, closeErr)
			} else {
				retErr = errors.Join(retErr, fmt.Errorf("trajectory: close %q: %w", path, closeErr))
			}
		}
	}()
	info, err := f.Stat()
	if err != nil {
		return Dataset{}, fmt.Errorf("trajectory: stat %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return Dataset{}, fmt.Errorf("trajectory: %q is not a regular file", path)
	}
	if info.Size() > limits.MaxCompressedBytes {
		return Dataset{}, fmt.Errorf("trajectory: compressed file size %d exceeds limit %d", info.Size(), limits.MaxCompressedBytes)
	}
	raw, err := readLimited(ctx, f, limits.MaxCompressedBytes)
	if err != nil {
		return Dataset{}, fmt.Errorf("trajectory: read %q: %w", path, err)
	}
	if int64(len(raw)) > limits.MaxCompressedBytes {
		return Dataset{}, fmt.Errorf("trajectory: compressed file exceeds limit %d", limits.MaxCompressedBytes)
	}
	actualSum := sha256.Sum256(raw)
	actualSHA := hex.EncodeToString(actualSum[:])
	expectedSHA, _ := normalizeSHA256(source.SHA256)
	if actualSHA != expectedSHA {
		return Dataset{}, fmt.Errorf("trajectory: source SHA-256 mismatch: got %s, want %s", actualSHA, expectedSHA)
	}

	reader, closeReader, err := trajectoryReader(raw, path)
	if err != nil {
		return Dataset{}, fmt.Errorf("trajectory: open CSV stream: %w", err)
	}
	defer func() {
		if closeErr := closeReader(); closeErr != nil {
			if retErr == nil {
				dataset = Dataset{}
				retErr = fmt.Errorf("trajectory: close CSV stream: %w", closeErr)
			} else {
				retErr = errors.Join(retErr, fmt.Errorf("trajectory: close CSV stream: %w", closeErr))
			}
		}
	}()

	limited := &io.LimitedReader{R: &contextReader{ctx: ctx, reader: reader}, N: limits.MaxUncompressedBytes + 1}
	csvReader := csv.NewReader(bufio.NewReader(limited))
	csvReader.FieldsPerRecord = -1
	csvReader.ReuseRecord = false
	csvReader.LazyQuotes = false
	header, err := csvReader.Read()
	if err != nil {
		if limited.N == 0 {
			return Dataset{}, fmt.Errorf("trajectory: uncompressed data exceeds limit %d", limits.MaxUncompressedBytes)
		}
		if errors.Is(err, io.EOF) {
			return Dataset{}, errors.New("trajectory: CSV header is missing")
		}
		return Dataset{}, fmt.Errorf("trajectory: read CSV header: %w", err)
	}
	indices, err := validateHeader(header, limits)
	if err != nil {
		return Dataset{}, err
	}
	headerFields := len(header)

	dataset = Dataset{Schema: SchemaVersion, Source: source, Rows: make([]Point, 0, minInt(limits.MaxRows, 4096))}
	closedTrials := make(map[string]struct{})
	activeTrial := ""
	var previousT float64
	rowNumber := 1
	for {
		if rowNumber%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return Dataset{}, fmt.Errorf("trajectory: read canceled: %w", err)
			}
		}
		record, readErr := csvReader.Read()
		if readErr != nil {
			if limited.N == 0 {
				return Dataset{}, fmt.Errorf("trajectory: uncompressed data exceeds limit %d", limits.MaxUncompressedBytes)
			}
			if errors.Is(readErr, io.EOF) {
				break
			}
			return Dataset{}, fmt.Errorf("trajectory: malformed CSV at row %d: %w", rowNumber+1, readErr)
		}
		rowNumber++
		if len(record) != headerFields {
			return Dataset{}, fmt.Errorf("trajectory: row %d has %d fields, want %d", rowNumber, len(record), headerFields)
		}
		if len(record) > limits.MaxFields {
			return Dataset{}, fmt.Errorf("trajectory: row %d has %d fields, exceeds limit %d", rowNumber, len(record), limits.MaxFields)
		}
		for fieldIndex, field := range record {
			if int64(len(field)) > limits.MaxFieldBytes {
				return Dataset{}, fmt.Errorf("trajectory: row %d field %d exceeds limit %d bytes", rowNumber, fieldIndex+1, limits.MaxFieldBytes)
			}
		}
		if len(dataset.Rows) >= limits.MaxRows {
			return Dataset{}, fmt.Errorf("trajectory: row count exceeds limit %d", limits.MaxRows)
		}
		point, err := parsePoint(record, indices, rowNumber)
		if err != nil {
			return Dataset{}, err
		}
		if point.TrialID != activeTrial {
			if activeTrial != "" {
				closedTrials[activeTrial] = struct{}{}
			}
			if _, closed := closedTrials[point.TrialID]; closed {
				return Dataset{}, fmt.Errorf("trajectory: trial %q rows are not contiguous at row %d", point.TrialID, rowNumber)
			}
			activeTrial = point.TrialID
			previousT = point.T
		} else if point.T <= previousT {
			if point.T == previousT {
				return Dataset{}, fmt.Errorf("trajectory: duplicate time %v in trial %q at row %d", point.T, point.TrialID, rowNumber)
			}
			return Dataset{}, fmt.Errorf("trajectory: time out of order in trial %q at row %d: %v after %v", point.TrialID, rowNumber, point.T, previousT)
		}
		previousT = point.T
		dataset.Rows = append(dataset.Rows, point)
	}
	if limited.N == 0 {
		return Dataset{}, fmt.Errorf("trajectory: uncompressed data exceeds limit %d", limits.MaxUncompressedBytes)
	}
	if len(dataset.Rows) == 0 {
		return Dataset{}, errors.New("trajectory: CSV has no trajectory rows")
	}
	return dataset, nil
}

type columnIndices struct {
	fname, fly, condition, segment, t, x, y int
}

func validateHeader(header []string, limits Limits) (columnIndices, error) {
	if len(header) == 0 {
		return columnIndices{}, errors.New("trajectory: CSV header is empty")
	}
	if len(header) > limits.MaxFields {
		return columnIndices{}, fmt.Errorf("trajectory: header has %d fields, exceeds limit %d", len(header), limits.MaxFields)
	}
	seen := make(map[string]struct{}, len(header))
	indices := columnIndices{fname: -1, fly: -1, condition: -1, segment: -1, t: -1, x: -1, y: -1}
	for i, rawName := range header {
		name := strings.TrimSpace(rawName)
		if i == 0 {
			name = strings.TrimPrefix(name, "\ufeff")
		}
		if name == "" {
			return columnIndices{}, fmt.Errorf("trajectory: header field %d is empty", i+1)
		}
		if int64(len(name)) > limits.MaxFieldBytes {
			return columnIndices{}, fmt.Errorf("trajectory: header field %d exceeds limit %d bytes", i+1, limits.MaxFieldBytes)
		}
		if _, exists := seen[name]; exists {
			return columnIndices{}, fmt.Errorf("trajectory: duplicate header %q", name)
		}
		seen[name] = struct{}{}
		switch name {
		case "fname":
			indices.fname = i
		case "fly":
			indices.fly = i
		case "condition":
			indices.condition = i
		case "segment":
			indices.segment = i
		case "t":
			indices.t = i
		case "x_cm":
			indices.x = i
		case "y_cm":
			indices.y = i
		}
	}
	missing := make([]string, 0, 7)
	for name, index := range map[string]int{
		"fname": indices.fname, "fly": indices.fly, "condition": indices.condition,
		"segment": indices.segment, "t": indices.t, "x_cm": indices.x, "y_cm": indices.y,
	} {
		if index < 0 {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return columnIndices{}, fmt.Errorf("trajectory: required columns missing: %s", strings.Join(missing, ", "))
	}
	return indices, nil
}

func parsePoint(record []string, indices columnIndices, rowNumber int) (Point, error) {
	get := func(index int) string {
		if index < 0 || index >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[index])
	}
	fname, fly, condition, segment := get(indices.fname), get(indices.fly), get(indices.condition), get(indices.segment)
	for name, value := range map[string]string{"fname": fname, "fly": fly, "condition": condition, "segment": segment} {
		if value == "" {
			return Point{}, fmt.Errorf("trajectory: row %d %s is required", rowNumber, name)
		}
		if strings.ContainsRune(value, trialSeparator) {
			return Point{}, fmt.Errorf("trajectory: row %d %s contains reserved trial separator", rowNumber, name)
		}
	}
	parseFinite := func(name string, index int) (float64, error) {
		value, err := strconv.ParseFloat(get(index), 64)
		if err != nil {
			return 0, fmt.Errorf("trajectory: row %d parse %s: %w", rowNumber, name, err)
		}
		if !isFinite(value) {
			return 0, fmt.Errorf("trajectory: row %d %s must be finite", rowNumber, name)
		}
		return value, nil
	}
	t, err := parseFinite("t", indices.t)
	if err != nil {
		return Point{}, err
	}
	if t < 0 {
		return Point{}, fmt.Errorf("trajectory: row %d t %v must be non-negative", rowNumber, t)
	}
	x, err := parseFinite("x_cm", indices.x)
	if err != nil {
		return Point{}, err
	}
	y, err := parseFinite("y_cm", indices.y)
	if err != nil {
		return Point{}, err
	}
	return Point{
		TrialID: makeTrialID(fname, fly), FName: fname, Fly: fly,
		Condition: condition, Segment: segment, T: t, XCM: x, YCM: y,
	}, nil
}

const trialSeparator = '\x1f'

func makeTrialID(fname, fly string) string {
	return strings.Join([]string{fname, fly}, string(trialSeparator))
}

func trajectoryReader(raw []byte, path string) (io.Reader, func() error, error) {
	if len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b {
		reader, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, func() error { return nil }, err
		}
		return reader, reader.Close, nil
	}
	if strings.HasSuffix(strings.ToLower(path), ".gz") {
		return nil, func() error { return nil }, errors.New("file has .gz suffix but is not gzip data")
	}
	return bytes.NewReader(raw), func() error { return nil }, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func readLimited(ctx context.Context, reader io.Reader, max int64) ([]byte, error) {
	limited := io.LimitReader(&contextReader{ctx: ctx, reader: reader}, max+1)
	return io.ReadAll(limited)
}

func normalizeSHA256(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != sha256.Size*2 {
		return "", fmt.Errorf("must be 64 hex characters")
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", fmt.Errorf("must be hexadecimal: %w", err)
	}
	return value, nil
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SplitByTrial assigns complete trial IDs to train or test. The fraction is
// applied to the number of trials, not rows, and input order within each side
// is preserved. The seeded shuffle makes the assignment reproducible.
func SplitByTrial(dataset Dataset, testFraction float64, seed uint64) (train, test Dataset, err error) {
	if !isFinite(testFraction) || !(testFraction > 0 && testFraction < 1) {
		return Dataset{}, Dataset{}, fmt.Errorf("trajectory: test fraction %v must be strictly between 0 and 1", testFraction)
	}
	if err := validateDatasetRows(dataset.Rows); err != nil {
		return Dataset{}, Dataset{}, err
	}
	trialIDs := make([]string, 0)
	seen := make(map[string]struct{})
	for _, row := range dataset.Rows {
		if _, ok := seen[row.TrialID]; !ok {
			seen[row.TrialID] = struct{}{}
			trialIDs = append(trialIDs, row.TrialID)
		}
	}
	if len(trialIDs) < 2 {
		return Dataset{}, Dataset{}, fmt.Errorf("trajectory: trial split needs at least 2 trials, got %d", len(trialIDs))
	}
	order := append([]string(nil), trialIDs...)
	rng := rand.New(rand.NewPCG(seed, 0))
	rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	testCount := int(math.Round(testFraction * float64(len(order))))
	if testCount < 1 {
		testCount = 1
	}
	if testCount >= len(order) {
		testCount = len(order) - 1
	}
	testIDs := make(map[string]struct{}, testCount)
	for _, id := range order[:testCount] {
		testIDs[id] = struct{}{}
	}
	train = Dataset{Schema: dataset.Schema, Source: dataset.Source, Rows: make([]Point, 0, len(dataset.Rows))}
	test = Dataset{Schema: dataset.Schema, Source: dataset.Source, Rows: make([]Point, 0, len(dataset.Rows))}
	for _, row := range dataset.Rows {
		if _, ok := testIDs[row.TrialID]; ok {
			test.Rows = append(test.Rows, row)
		} else {
			train.Rows = append(train.Rows, row)
		}
	}
	return train, test, nil
}

func validateDatasetRows(rows []Point) error {
	if len(rows) == 0 {
		return errors.New("trajectory: dataset has no rows")
	}
	closedTrials := make(map[string]struct{})
	activeTrial := ""
	var previousT float64
	for i, row := range rows {
		if row.TrialID == "" {
			return fmt.Errorf("trajectory: row %d has an empty trial id", i+1)
		}
		if !isFinite(row.T) || !isFinite(row.XCM) || !isFinite(row.YCM) {
			return fmt.Errorf("trajectory: row %d has a non-finite value", i+1)
		}
		if row.T < 0 {
			return fmt.Errorf("trajectory: row %d t %v must be non-negative", i+1, row.T)
		}
		if row.TrialID != activeTrial {
			if activeTrial != "" {
				closedTrials[activeTrial] = struct{}{}
			}
			if _, closed := closedTrials[row.TrialID]; closed {
				return fmt.Errorf("trajectory: trial %q rows are not contiguous at row %d", row.TrialID, i+1)
			}
			activeTrial = row.TrialID
			previousT = row.T
			continue
		}
		if row.T <= previousT {
			if row.T == previousT {
				return fmt.Errorf("trajectory: duplicate time %v in trial %q at row %d", row.T, row.TrialID, i+1)
			}
			return fmt.Errorf("trajectory: time out of order in trial %q at row %d", row.TrialID, i+1)
		}
		previousT = row.T
	}
	return nil
}
