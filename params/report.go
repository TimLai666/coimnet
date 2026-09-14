package params

import (
	"math"
	"slices"
)

const (
	// ReportSchemaVersion identifies the derivation report contract.
	ReportSchemaVersion = "coimnet-derivation-report/v1"

	// StatusMeasured marks a value this run actually computed.
	StatusMeasured = "measured"
	// StatusNotMeasured marks a value this run could not compute, so the
	// zero fields next to it are not results.
	StatusNotMeasured = "not_measured"
)

// Ratio carries a proportion with the numbers it came from. Value is only
// meaningful when Defined is true; a zero denominator leaves it undefined
// instead of reporting zero.
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

// SourceFingerprint is one verified release file.
type SourceFingerprint struct {
	Role   string `json:"role"`
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
	Rows   int64  `json:"rows"`
	// Batches is the number of Feather record batches read; it stays 0 for
	// the CSV source, whose Format says csv.
	Batches int64  `json:"record_batches"`
	Format  string `json:"format"`
}

// GraphSummary ties a derivation to the exact store it ran on.
type GraphSummary struct {
	Namespace string `json:"namespace"`
	Nodes     uint64 `json:"nodes"`
	Edges     uint64 `json:"edges"`
	NodeIndex string `json:"node_index_hash"`
	EdgeOrder string `json:"edge_order_hash"`
	// EdgeOrderFinding records how the derivation joined the edge stream.
	EdgeOrderFinding string `json:"edge_order_finding"`
}

// MetaSummary is what this stage reads from Neuprint_Meta.csv: the ROI
// hierarchy shape and the release confidence thresholds. The thresholds are
// recorded, not applied; the sources are the already filtered minconf-0.5
// release files.
type MetaSummary struct {
	Status                    string  `json:"status"`
	Dataset                   string  `json:"dataset"`
	PostHighAccuracyThreshold float64 `json:"post_high_accuracy_threshold"`
	PreHPThreshold            float64 `json:"pre_hp_threshold"`
	PostHPThreshold           float64 `json:"post_hp_threshold"`
	ThresholdsApplied         bool    `json:"thresholds_applied"`
	ROIRoot                   string  `json:"roi_hierarchy_root"`
	ROILevels                 int     `json:"roi_hierarchy_levels"`
	ROILevelCounts            []int   `json:"roi_hierarchy_level_counts"`
	ROINodes                  int     `json:"roi_hierarchy_nodes"`
}

// BodyStatsSummary counts the body-stats pass. Rows is every source row;
// RowsSelected counts the first row of each selected body, and the disjoint
// reasons below account for the rest.
type BodyStatsSummary struct {
	Rows            uint64 `json:"rows"`
	RowsSelected    uint64 `json:"rows_selected"`
	RowsNotSelected uint64 `json:"rows_not_selected"`
	NullBodyRows    uint64 `json:"null_body_rows"`
	NegativeBodies  uint64 `json:"negative_body_rows"`
	DuplicateRows   uint64 `json:"duplicate_rows"`
	NullCountRows   uint64 `json:"null_count_rows"`
	BodiesMissing   uint64 `json:"bodies_missing_from_body_stats"`
	Coverage        Ratio  `json:"coverage"`
}

// TbarSummary counts the T-bar pass and the key groups it produced.
type TbarSummary struct {
	Rows                    uint64 `json:"rows"`
	RowsSkippedNullKey      uint64 `json:"rows_skipped_null_coordinate"`
	RowsSkippedNullBody     uint64 `json:"rows_skipped_null_body"`
	RowsSorted              uint64 `json:"rows_sorted"`
	RowsWithNullProbability uint64 `json:"rows_with_null_probability"`
	RowsInAmbiguousKeys     uint64 `json:"rows_in_ambiguous_keys"`
	Keys                    uint64 `json:"keys"`
	AmbiguousKeys           uint64 `json:"ambiguous_keys"`
	KeysWithNullProbability uint64 `json:"keys_with_null_probability"`
	UsableKeys              uint64 `json:"usable_keys"`
	KeysWithoutSynapse      uint64 `json:"keys_without_synapse"`
	UsableRatio             Ratio  `json:"usable_ratio"`
}

// SynPartnersSummary counts the synapse pass and the merge join against the
// T-bar keys.
type SynPartnersSummary struct {
	Rows                         uint64 `json:"rows"`
	RowsNullCoordinate           uint64 `json:"rows_null_coordinate"`
	RowsNullBody                 uint64 `json:"rows_null_body"`
	RowsBodyNotSelected          uint64 `json:"rows_body_not_selected"`
	RowsSorted                   uint64 `json:"rows_sorted"`
	MatchedSynapses              uint64 `json:"matched_synapses"`
	UnmatchedSynapses            uint64 `json:"unmatched_synapses"`
	SynapsesOnAmbiguousKey       uint64 `json:"synapses_on_ambiguous_key"`
	SynapsesOnNullProbabilityKey uint64 `json:"synapses_on_null_probability_key"`
	MatchedRatio                 Ratio  `json:"matched_ratio"`
}

// PairSummary counts the aggregated (source, target) pairs.
type PairSummary struct {
	Pairs      uint64 `json:"pairs"`
	InGraph    uint64 `json:"pairs_in_graph"`
	NotInGraph uint64 `json:"pairs_not_in_graph"`
}

// SignHistogram counts the derived edge signs.
type SignHistogram struct {
	Positive uint64 `json:"positive"`
	Negative uint64 `json:"negative"`
	Unknown  uint64 `json:"unknown"`
}

// Quantiles are the nearest-rank quantiles of the derived weights: with n
// sorted values, pQ is element ceil(Q*n)-1 and p0 is the first element. No
// interpolation is performed, so every reported number is a real weight.
type Quantiles struct {
	Status string  `json:"status"`
	Count  uint64  `json:"count"`
	P0     float64 `json:"p0"`
	P25    float64 `json:"p25"`
	P50    float64 `json:"p50"`
	P75    float64 `json:"p75"`
	P100   float64 `json:"p100"`
}

// quantiles sorts values in place and returns the nearest-rank quantiles.
func quantiles(values []float64) Quantiles {
	if len(values) == 0 {
		return Quantiles{Status: StatusNotMeasured}
	}
	slices.Sort(values)
	at := func(fraction float64) float64 {
		rank := int(math.Ceil(fraction * float64(len(values))))
		if rank < 1 {
			rank = 1
		}
		if rank > len(values) {
			rank = len(values)
		}
		return values[rank-1]
	}
	return Quantiles{
		Status: StatusMeasured,
		Count:  uint64(len(values)),
		P0:     values[0],
		P25:    at(0.25),
		P50:    at(0.50),
		P75:    at(0.75),
		P100:   values[len(values)-1],
	}
}

// EdgeSummary is the per-edge result of the whole derivation.
type EdgeSummary struct {
	Edges              uint64 `json:"edges"`
	WithMatch          uint64 `json:"edges_with_match"`
	WithoutMatch       uint64 `json:"edges_without_match"`
	DuplicatePairEdges uint64 `json:"duplicate_pair_edges"`
	NullRawWeightEdges uint64 `json:"null_raw_weight_edges"`

	TransmitterEdges     map[string]uint64 `json:"transmitter_edges"`
	TransmitterUnmatched uint64            `json:"transmitter_unmatched"`

	Sign         SignHistogram `json:"sign"`
	UnknownRatio Ratio         `json:"unknown_ratio"`
	// The four unknown reasons are disjoint and sum to Sign.Unknown, in the
	// order they are checked: no match, then the probability threshold, then
	// the matched fraction threshold, then the rule mapping itself.
	UnknownWithoutMatch         uint64 `json:"unknown_without_match"`
	UnknownBelowProbability     uint64 `json:"unknown_below_min_probability"`
	UnknownBelowMatchedFraction uint64 `json:"unknown_below_min_matched_fraction"`
	UnknownByMapping            uint64 `json:"unknown_by_mapping"`

	MatchedFraction     Ratio     `json:"matched_fraction"`
	NormalizerZeroEdges uint64    `json:"normalizer_zero_edges"`
	NormalizerBasis     string    `json:"normalizer_basis"`
	WeightQuantiles     Quantiles `json:"weight_quantiles"`
}

// PrimaryROISummary describes the per-node primary ROI mode.
type PrimaryROISummary struct {
	Status          string `json:"status"`
	Basis           string `json:"basis"`
	Rows            uint64 `json:"rows"`
	NullROIRows     uint64 `json:"null_roi_rows"`
	Names           int    `json:"distinct_roi_names"`
	NodesWithROI    uint64 `json:"nodes_with_roi"`
	NodesWithoutROI uint64 `json:"nodes_without_roi"`
}

// SortStats is one bounded external sort.
type SortStats struct {
	Name              string `json:"name"`
	RecordBytes       int    `json:"record_bytes"`
	Records           int64  `json:"records"`
	Emitted           int64  `json:"emitted"`
	Runs              int64  `json:"runs"`
	TempBytes         int64  `json:"temp_bytes"`
	PeakMemoryBytes   int64  `json:"peak_memory_bytes"`
	PeakBufferedBytes int64  `json:"peak_buffered_bytes"`
}

// SpillStats is one sorted intermediate file. The pipeline materializes the
// merged output of a sort so the next stage can read it sequentially while it
// streams the other side of a join.
type SpillStats struct {
	Name        string `json:"name"`
	RecordBytes int    `json:"record_bytes"`
	Records     int64  `json:"records"`
	Bytes       int64  `json:"bytes"`
}

// Report is the complete derivation report. It is embedded in the parameter
// set file, so two saves of the same set produce the same bytes.
type Report struct {
	SchemaVersion string              `json:"schema_version"`
	Source        string              `json:"source"`
	RulesHash     string              `json:"rules_hash"`
	Rules         Rules               `json:"rules"`
	Sources       []SourceFingerprint `json:"sources"`
	Graph         GraphSummary        `json:"graph"`
	Meta          MetaSummary         `json:"neuprint_meta"`
	BodyStats     BodyStatsSummary    `json:"body_stats"`
	Tbar          TbarSummary         `json:"tbar"`
	SynPartners   SynPartnersSummary  `json:"syn_partners"`
	Pairs         PairSummary         `json:"pairs"`
	Edges         EdgeSummary         `json:"edges"`
	PrimaryROI    PrimaryROISummary   `json:"primary_roi"`
	Sorts         []SortStats         `json:"sorts"`
	Spills        []SpillStats        `json:"spills"`
	Limits        Limits              `json:"limits"`
	// PeakAccountedBytes covers the arrays and buffers this package reserves.
	// It excludes Go allocation slack, Arrow buffers (bounded separately by
	// MaxArrowBytes) and the runtime, so it is not a process RSS bound.
	PeakAccountedBytes int64    `json:"peak_accounted_bytes"`
	WallTimeSeconds    float64  `json:"wall_time_seconds"`
	Limitations        []string `json:"limitations"`
}

// reportLimitations is copied into every report so a reader of the parameter
// set file alone still sees what the numbers do and do not claim.
var reportLimitations = []string{
	"edge signs are derived from predicted per-T-bar transmitter probabilities under the recorded rule; they are not measured synaptic actions",
	"inhibitory glutamate is an engineering assumption stated in the rule basis, not evidence in the release",
	"an edge whose winning transmitter maps to unknown, or that misses a threshold, or that no synapse matched, stays unknown; no default sign is substituted",
	"sign_confidence is the mean of the winning probability over the matched synapses of the pair; it is kept for unknown edges too",
	"matched_fraction uses the raw release weight of the pair as its denominator; the release does not promise that weight equals the number of T-bar to partner contacts",
	"neuprint confidence thresholds are recorded, not applied: the sources are the already filtered minconf-0.5 release files and the per-synapse conf columns are not used in this stage",
	"the primary ROI is the mode of primary_post over synapses onto the node; ties are broken by the ROI name so the result is reproducible, not biologically ranked",
	"memory accounting covers this package's arrays and buffers only, not process RSS; Arrow allocations are bounded separately by max_arrow_bytes",
}
