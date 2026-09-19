// Package flywire converts the three CSV tables of one FlyWire public release
// (optionally gzip-compressed) into the connectome intermediate Feather files
// and a DatasetManifest, so the release can be built as an independent
// namespace. The conversion is the DAT-06 adapter; the real release files are
// blocked_data, so the package is verified against fixtures.
package flywire

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/TimLai666/coimnet/connectome"
)

const (
	// releasedLabel is the constant value of the included column: the release
	// table itself is the selection set, so every row is marked released.
	releasedLabel = "released"
)

// Inputs names the three tables of one FlyWire public release (CSV, optionally
// gzip-compressed when the path ends in .gz) and the release metadata the
// manifest records.
type Inputs struct {
	Connections    string // columns pre_root_id, post_root_id, neuropil, syn_count, nt_type
	Classification string // columns root_id, flow, super_class, class, sub_class, cell_type, hemibrain_type, hemilineage, side, nerve
	Neurons        string // columns root_id, group, nt_type, nt_type_score (other columns ignored)
	Version        string // release number, e.g. "783"; must be non-empty and contain no '/' or whitespace
	AcquiredAt     string // RFC3339
	LicenseURL     string // the release terms page
}

// RowCounts reports how many data rows Convert read from each table.
type RowCounts struct {
	Connections    int64 // rows of the connections table
	Classification int64 // rows of the classification table
	Neurons        int64 // rows of the neurons table
}

// Result is what Convert wrote.
type Result struct {
	Manifest     connectome.DatasetManifest
	ManifestPath string
	Rows         RowCounts // Connections, Classification, Neurons int64
}

// Convert reads the three tables and writes weights.feather, annotations.feather,
// nt.feather and manifest.json into outDir (created; must not already contain
// files). Root ids are parsed as uint64 and stored in int64 Feather columns; a
// value above math.MaxInt64 is the error "root id %s overflows int64" (never a
// silent truncation). Missing columns are errors naming the column and the
// table. The neuropil partitions of one (pre, post) pair are kept as separate
// rows and declared additive_partitions.
func Convert(ctx context.Context, in Inputs, outDir string) (Result, error) {
	if err := validateInputs(in); err != nil {
		return Result{}, err
	}
	if err := requireEmptyDir(outDir); err != nil {
		return Result{}, err
	}
	connections, err := openCSV(in.Connections, "connections")
	if err != nil {
		return Result{}, err
	}
	defer connections.close()
	classification, err := openCSV(in.Classification, "classification")
	if err != nil {
		return Result{}, err
	}
	defer classification.close()
	neurons, err := openCSV(in.Neurons, "neurons")
	if err != nil {
		return Result{}, err
	}
	defer neurons.close()
	for _, col := range []string{"pre_root_id", "post_root_id", "neuropil", "syn_count"} {
		if err := connections.require(col); err != nil {
			return Result{}, err
		}
	}
	for _, col := range []string{"root_id", "flow", "super_class", "class", "sub_class", "cell_type", "side"} {
		if err := classification.require(col); err != nil {
			return Result{}, err
		}
	}
	for _, col := range []string{"root_id", "nt_type", "nt_type_score"} {
		if err := neurons.require(col); err != nil {
			return Result{}, err
		}
	}

	classificationRows, err := writeAnnotations(ctx, classification, filepath.Join(outDir, "annotations.feather"))
	if err != nil {
		return Result{}, err
	}
	connectionRows, err := writeWeights(ctx, connections, filepath.Join(outDir, "weights.feather"))
	if err != nil {
		return Result{}, err
	}
	neuronRows, err := writeNeurotransmitters(ctx, neurons, filepath.Join(outDir, "nt.feather"))
	if err != nil {
		return Result{}, err
	}

	files := []connectome.SourceFile{
		{Role: connectome.RoleWeights, Path: filepath.Join(outDir, "weights.feather")},
		{Role: connectome.RoleAnnotations, Path: filepath.Join(outDir, "annotations.feather")},
		{Role: connectome.RoleNeurotransmitters, Path: filepath.Join(outDir, "nt.feather")},
	}
	for i := range files {
		if err := fingerprintSource(&files[i]); err != nil {
			return Result{}, err
		}
	}
	manifest := connectome.DatasetManifest{
		SchemaVersion: connectome.ManifestSchemaVersion,
		Dataset:       "flywire",
		Namespace:     "flywire-" + in.Version,
		SourceVersion: in.Version,
		License:       connectome.License{Name: "FlyWire public release terms", URL: in.LicenseURL},
		AcquiredAt:    in.AcquiredAt,
		Files:         files,
		FieldMapping: connectome.FieldMapping{
			Weights:           connectome.WeightsFields{Source: "pre_root_id", Target: "post_root_id", Value: "syn_count"},
			Annotations:       connectome.AnnotationFields{ID: "root_id", Status: "included", Class: "class", Superclass: "super_class", Subclass: "sub_class", Type: "cell_type", SomaSide: "side"},
			Neurotransmitters: connectome.NeurotransmitterFields{ID: "root_id", Predicted: "nt_type", Confidence: "nt_type_score"},
		},
		Identity: connectome.IdentityMapping{
			WeightsEndpointsAreAnnotationIDs:    true,
			NeurotransmitterIDsAreAnnotationIDs: true,
			Evidence:                            "root_id is the identifier shared by every table of one FlyWire release; no cross-release mapping is implied",
		},
		Selection: connectome.SelectionPredicate{
			Source: connectome.RoleAnnotations,
			Field:  "included",
			Equals: releasedLabel,
			Label:  "every neuron in the classification table of the release",
		},
		DuplicateSemantics: connectome.DuplicateSemanticsAdditivePartitions,
		CoordinateUnit:     "not_exported",
		TransformHistory: []connectome.TransformStep{{
			Step:        "flywire-csv-to-feather",
			Description: "converts the release CSV tables into Feather; included is a constant column added by the adapter marking every classification row as released because the release table itself is the selection set",
			Version:     "flywire-adapter/v1",
		}},
	}
	if err := manifest.Validate(); err != nil {
		return Result{}, err
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Result{}, fmt.Errorf("flywire: encode manifest: %w", err)
	}
	manifestPath := filepath.Join(outDir, "manifest.json")
	if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
		return Result{}, fmt.Errorf("flywire: write manifest: %w", err)
	}
	return Result{
		Manifest:     manifest,
		ManifestPath: manifestPath,
		Rows:         RowCounts{Connections: connectionRows, Classification: classificationRows, Neurons: neuronRows},
	}, nil
}

func validateInputs(in Inputs) error {
	for _, field := range [][2]string{
		{"connections", in.Connections},
		{"classification", in.Classification},
		{"neurons", in.Neurons},
		{"version", in.Version},
		{"acquired_at", in.AcquiredAt},
		{"license_url", in.LicenseURL},
	} {
		if strings.TrimSpace(field[1]) == "" {
			return fmt.Errorf("flywire: %s must not be empty", field[0])
		}
	}
	if strings.ContainsAny(in.Version, "/ \t\r\n") {
		return fmt.Errorf("flywire: release version %q must not contain '/' or whitespace", in.Version)
	}
	return nil
}

func requireEmptyDir(outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("flywire: create output directory: %w", err)
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return fmt.Errorf("flywire: read output directory: %w", err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("flywire: output directory %q already contains files", outDir)
	}
	return nil
}

// csvTable is one open CSV table with its trimmed header index.
type csvTable struct {
	name   string
	file   *os.File
	reader *csv.Reader
	cols   map[string]int
	rows   int64
}

func openCSV(path, name string) (*csvTable, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("flywire: open %s table: %w", name, err)
	}
	var reader io.Reader = file
	if strings.HasSuffix(strings.ToLower(path), ".gz") {
		gzipReader, err := gzip.NewReader(file)
		if err != nil {
			file.Close()
			return nil, fmt.Errorf("flywire: %s table is not valid gzip: %w", name, err)
		}
		reader = gzipReader
	}
	csvReader := csv.NewReader(reader)
	header, err := csvReader.Read()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("flywire: read %s table header: %w", name, err)
	}
	cols := make(map[string]int, len(header))
	for i, column := range header {
		if i == 0 {
			column = strings.TrimPrefix(column, "\ufeff")
		}
		cols[strings.TrimSpace(column)] = i
	}
	return &csvTable{name: name, file: file, reader: csvReader, cols: cols}, nil
}

func (t *csvTable) close() { t.file.Close() }

func (t *csvTable) require(col string) error {
	if _, ok := t.cols[col]; !ok {
		return fmt.Errorf("flywire: %s table is missing column %q", t.name, col)
	}
	return nil
}

// next returns the next data row or ok=false at EOF. The returned row is only
// valid until the following call.
func (t *csvTable) next() ([]string, bool, error) {
	record, err := t.reader.Read()
	if err == io.EOF {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("flywire: read %s table: %w", t.name, err)
	}
	t.rows++
	return record, true, nil
}

func (t *csvTable) cell(record []string, col string) (string, error) {
	i, ok := t.cols[col]
	if !ok {
		return "", fmt.Errorf("flywire: %s table is missing column %q", t.name, col)
	}
	if i >= len(record) {
		return "", fmt.Errorf("flywire: %s table row %d has fewer fields than the header", t.name, t.rows)
	}
	return strings.TrimSpace(record[i]), nil
}

// cells reads one row of the named columns in order.
func cells(table *csvTable, record []string, cols ...string) ([]string, error) {
	values := make([]string, len(cols))
	for i, col := range cols {
		value, err := table.cell(record, col)
		if err != nil {
			return nil, err
		}
		values[i] = value
	}
	return values, nil
}

// fingerprintSource fills the bytes and SHA256 of the file Path and marks the
// hash as locally recorded.
func fingerprintSource(file *connectome.SourceFile) error {
	f, err := os.Open(file.Path)
	if err != nil {
		return fmt.Errorf("flywire: open %s for hashing: %w", file.Path, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("flywire: stat %s: %w", file.Path, err)
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, f); err != nil {
		return fmt.Errorf("flywire: hash %s: %w", file.Path, err)
	}
	file.Bytes = info.Size()
	file.SHA256 = hex.EncodeToString(digest.Sum(nil))
	file.HashStatus = connectome.HashLocallyRecorded
	return nil
}
