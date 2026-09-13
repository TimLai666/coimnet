package connectome

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// EdgeViewMode selects how annotated edges are materialized.
type EdgeViewMode string

const (
	// EdgeViewRows keeps every source row as its own edge.
	EdgeViewRows EdgeViewMode = "rows"
	// EdgeViewAggregatedPairs sums repeated pairs. It requires additive
	// duplicate semantics in the manifest.
	EdgeViewAggregatedPairs EdgeViewMode = "aggregated_pairs"
)

// ResourceLimits bounds one build. All values must be positive. The builder
// returns ErrCapacity instead of shrinking the graph when a limit is hit.
// MaxMemoryBytes bounds accounted working memory (annotation rows, ID
// catalogs, sort buffers and final edge arrays), not process RSS.
// MaxTempBytes is split evenly between the three sort passes.
// SortBufferBytes optionally fixes the in-memory buffer of each sort pass;
// zero derives it from the memory left after the annotation catalogs. It is
// still accounted against MaxMemoryBytes.
type ResourceLimits struct {
	MaxMemoryBytes  int64 `json:"max_memory_bytes"`
	MaxTempBytes    int64 `json:"max_temp_bytes"`
	MaxRunFiles     int   `json:"max_run_files"`
	MaxArrowBytes   int64 `json:"max_arrow_bytes"`
	MaxFooterBytes  int64 `json:"max_footer_bytes"`
	MaxRows         int64 `json:"max_rows"`
	SortBufferBytes int64 `json:"sort_buffer_bytes,omitempty"`
}

func (l ResourceLimits) validate() error {
	if l.MaxMemoryBytes <= 0 || l.MaxTempBytes <= 0 || l.MaxRunFiles <= 0 || l.MaxArrowBytes <= 0 || l.MaxFooterBytes <= 0 || l.MaxRows <= 0 {
		return fmt.Errorf("connectome: every resource limit must be positive: %+v", l)
	}
	if l.SortBufferBytes < 0 {
		return fmt.Errorf("connectome: sort_buffer_bytes must not be negative")
	}
	return nil
}

// EndpointStats counts null, negative and zero values of an endpoint column.
type EndpointStats struct {
	NullRows     uint64 `json:"null_rows"`
	NegativeRows uint64 `json:"negative_rows"`
	ZeroRows     uint64 `json:"zero_rows"`
}

// ValueStats counts null, negative and zero values of the weight column.
type ValueStats struct {
	NullRows     uint64 `json:"null_rows"`
	NegativeRows uint64 `json:"negative_rows"`
	ZeroRows     uint64 `json:"zero_rows"`
}

// ValueSum is the checked exact sum of non-null, non-negative source values.
// Rows counts the values included; NullRows the values excluded as null.
// Valid is false when a negative value made the sum meaningless.
type ValueSum struct {
	Value    uint64 `json:"value"`
	Rows     uint64 `json:"rows"`
	NullRows uint64 `json:"null_rows"`
	Valid    bool   `json:"valid"`
	Basis    string `json:"basis"`
}

// Ratio keeps numerator, denominator and the field the ratio is based on so
// nobody can reinterpret it with another denominator. Defined is false when
// the denominator is zero.
type Ratio struct {
	Numerator   uint64  `json:"numerator"`
	Denominator uint64  `json:"denominator"`
	Value       float64 `json:"value"`
	Defined     bool    `json:"defined"`
	Basis       string  `json:"basis"`
}

func newRatio(numerator, denominator uint64, basis string) Ratio {
	r := Ratio{Numerator: numerator, Denominator: denominator, Basis: basis}
	if denominator != 0 {
		r.Value = float64(numerator) / float64(denominator)
		r.Defined = true
	}
	return r
}

// AnnotationExclusions counts annotation rows excluded from the node set.
// NullID, InvalidID, DuplicateID and PredicateFalse are disjoint reasons in
// order of precedence and sum with Nodes to the annotation row count.
// PredicateFieldNull is a sub-count of PredicateFalse: rows whose predicate
// column is null.
type AnnotationExclusions struct {
	NullID             uint64 `json:"null_id"`
	InvalidID          uint64 `json:"invalid_id"`
	DuplicateID        uint64 `json:"duplicate_id"`
	PredicateFalse     uint64 `json:"predicate_false"`
	PredicateFieldNull uint64 `json:"predicate_field_null"`
}

// WeightExclusions counts weights rows excluded from the annotated view, one
// reason per row in this order of precedence.
type WeightExclusions struct {
	NullEndpoint      uint64 `json:"null_endpoint"`
	InvalidID         uint64 `json:"invalid_id"`
	MissingAnnotation uint64 `json:"missing_annotation"`
	PredicateFalse    uint64 `json:"predicate_false"`
}

// Column mapping status values for report sections that depend on an
// optional manifest column.
const (
	ColumnMapped    = "mapped"
	ColumnNotMapped = "not_mapped"
)

// TransmitterSummary reports prediction coverage of the selected nodes.
// When the manifest maps no consensus column, Status is not_mapped, the
// counts stay zero and the ratio is undefined instead of reporting every
// node as unknown.
type TransmitterSummary struct {
	Status            string `json:"status"`
	Predicted         uint64 `json:"predicted"`
	Unknown           uint64 `json:"unknown"`
	UnknownRatio      Ratio  `json:"unknown_ratio"`
	UnknownDefinition string `json:"unknown_definition"`
	Measured          string `json:"measured"`
}

// ReceptorSummary states that receptor evidence is not derived and reports
// only the annotation label null ratio separately. LabelStatus is
// not_mapped and the ratio undefined when no receptor label column is mapped.
type ReceptorSummary struct {
	Status         string `json:"status"`
	LabelStatus    string `json:"label_status"`
	LabelNullRatio Ratio  `json:"label_null_ratio"`
}

// RawTransmitterStats describes the prediction file itself. ConsensusStatus
// is not_mapped, with zero unknown rows and an undefined ratio, when the
// manifest maps no consensus column.
type RawTransmitterStats struct {
	Rows              uint64 `json:"rows"`
	UniqueIDs         uint64 `json:"unique_ids"`
	DuplicateRows     uint64 `json:"duplicate_rows"`
	NullIDs           uint64 `json:"null_ids"`
	InvalidIDs        uint64 `json:"invalid_ids"`
	ConsensusStatus   string `json:"consensus_status"`
	UnknownRows       uint64 `json:"unknown_rows"`
	UnknownRatio      Ratio  `json:"unknown_ratio"`
	UnknownDefinition string `json:"unknown_definition"`
}

// RawView reports the raw_segments view: every weights row as stored.
type RawView struct {
	Name                           string              `json:"name"`
	Rows                           uint64              `json:"rows"`
	Source                         EndpointStats       `json:"source"`
	Target                         EndpointStats       `json:"target"`
	Weight                         ValueStats          `json:"weight"`
	SelfLoopRows                   uint64              `json:"self_loop_rows"`
	UniqueEndpoints                uint64              `json:"unique_endpoints"`
	UnannotatedUniqueEndpoints     uint64              `json:"unannotated_unique_endpoints"`
	UnannotatedEndpointOccurrences uint64              `json:"unannotated_endpoint_occurrences"`
	ValidPairRows                  uint64              `json:"valid_pair_rows"`
	UniquePairs                    uint64              `json:"unique_pairs"`
	DuplicatePairs                 uint64              `json:"duplicate_pairs"`
	DuplicateRows                  uint64              `json:"duplicate_rows"`
	WeightSum                      ValueSum            `json:"weight_sum"`
	DuplicateSemantics             DuplicateSemantics  `json:"duplicate_semantics"`
	Aggregation                    string              `json:"aggregation"`
	Transmitter                    RawTransmitterStats `json:"transmitter"`
}

// AnnotatedView reports the annotated_neurons view.
type AnnotatedView struct {
	Name                 string               `json:"name"`
	AnnotationRows       uint64               `json:"annotation_rows"`
	Nodes                uint64               `json:"nodes"`
	Edges                uint64               `json:"edges"`
	AnnotationExclusions AnnotationExclusions `json:"annotation_exclusions"`
	WeightExclusions     WeightExclusions     `json:"weight_exclusions"`
	UniquePairs          uint64               `json:"unique_pairs"`
	DuplicatePairs       uint64               `json:"duplicate_pairs"`
	DuplicateRows        uint64               `json:"duplicate_rows"`
	SelfLoops            uint64               `json:"self_loops"`
	IsolatedNodes        uint64               `json:"isolated_nodes"`
	WeightSum            ValueSum             `json:"weight_sum"`
	Transmitter          TransmitterSummary   `json:"transmitter"`
	Receptor             ReceptorSummary      `json:"receptor"`
}

// SourceReport records one source before and after the build.
type SourceReport struct {
	Role              FileRole `json:"role"`
	Path              string   `json:"path"`
	URL               string   `json:"url,omitempty"`
	ETag              string   `json:"etag,omitempty"`
	Bytes             int64    `json:"bytes"`
	SHA256            string   `json:"sha256"`
	SHA256After       string   `json:"sha256_after"`
	HashStatus        string   `json:"hash_status"`
	FingerprintStable bool     `json:"fingerprint_stable"`
	RecordBatches     int64    `json:"record_batches"`
	Rows              int64    `json:"rows"`
}

// PredicateReport records the applied selection rule.
type PredicateReport struct {
	Canonical string `json:"canonical"`
	Hash      string `json:"hash"`
	Label     string `json:"label"`
}

// ResultHashes identify the produced views. NodeIndex covers the ordered
// node IDs, EdgeOrder the ordered edge stream, Report the whole report with
// this field blank.
type ResultHashes struct {
	NodeIndex string `json:"node_index"`
	EdgeOrder string `json:"edge_order"`
	Report    string `json:"report"`
}

// SortStats summarizes the external sort passes and memory accounting.
// ReservedPeakBytes is the peak of the accounted reservations; with
// SortBufferBytes zero the sort passes reserve everything left, so it equals
// the limit by construction. BufferedPeakBytes is the high-water mark of
// bytes actually held: annotation rows, ID catalogs, buffered sort records
// with their index, and the final edge arrays.
type SortStats struct {
	Runs              int64 `json:"runs"`
	TempBytes         int64 `json:"temp_bytes"`
	ReservedPeakBytes int64 `json:"reserved_peak_bytes"`
	BufferedPeakBytes int64 `json:"buffered_peak_bytes"`
}

// GraphReport is the complete, JSON-serializable build report.
type GraphReport struct {
	SchemaVersion    string          `json:"schema_version"`
	ConverterVersion string          `json:"converter_version"`
	Dataset          string          `json:"dataset"`
	Namespace        string          `json:"namespace"`
	SourceVersion    string          `json:"source_version"`
	License          License         `json:"license"`
	AcquiredAt       string          `json:"acquired_at"`
	ManifestHash     string          `json:"manifest_hash"`
	EdgeView         EdgeViewMode    `json:"edge_view"`
	Sources          []SourceReport  `json:"sources"`
	Predicate        PredicateReport `json:"predicate"`
	Identity         IdentityMapping `json:"identity"`
	Raw              RawView         `json:"raw_segments"`
	Annotated        AnnotatedView   `json:"annotated_neurons"`
	Hashes           ResultHashes    `json:"hashes"`
	Limits           ResourceLimits  `json:"limits"`
	Sort             SortStats       `json:"sort"`
	Limitations      []string        `json:"limitations"`
}

func (r GraphReport) clone() GraphReport {
	r.Sources = append([]SourceReport(nil), r.Sources...)
	r.Limitations = append([]string(nil), r.Limitations...)
	return r
}

// hash computes the report hash over the JSON encoding with Hashes.Report
// blank.
func (r GraphReport) hash() (string, error) {
	r = r.clone()
	r.Hashes.Report = ""
	encoded, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("connectome: encode report: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}
