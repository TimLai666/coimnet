package simulate

import (
	"fmt"
	"math"
	"sort"
)

// GraphHashes identify the wiring a run was executed on. They are the node
// index and edge order hashes of the graph report, so a run can be tied back to
// the exact store it came from.
type GraphHashes struct {
	NodeIndex string `json:"node_index"`
	EdgeOrder string `json:"edge_order"`
}

// ResolvedInjection is one injection as actually applied. A protocol entry that
// named a single node reports NodeCount 1 and no selector; an injection group
// reports the selector resolution and the size of the matched set.
type ResolvedInjection struct {
	Channel   int                 `json:"channel"`
	Gain      float64             `json:"gain"`
	NodeCount int                 `json:"node_count"`
	FirstNode int                 `json:"first_node"`
	LastNode  int                 `json:"last_node"`
	Selector  *SelectorResolution `json:"selector,omitempty"`
}

// ProbeResult is one probe's declared reduction over the run.
type ProbeResult struct {
	Name      string              `json:"name"`
	Reduce    string              `json:"reduce"`
	NodeCount int                 `json:"node_count"`
	Selector  *SelectorResolution `json:"selector,omitempty"`
	Series    []float64           `json:"series"`
}

// Monitors are the stability observations of main specification 8.5.
//
// SilentFraction is the fraction of neurons that never spiked during the run;
// for the continuous core, which has no events, it is the fraction whose output
// never moved from the value it held before the run.
//
// PopulationRatePerStep is the fraction of neurons that spiked at each step.
// RateQuantiles are p0, p25, p50, p75 and p100 of the per-neuron spike rate
// (spikes divided by steps), interpolated linearly between order statistics.
// The continuous core emits no events, so both stay empty and zero instead of
// reporting an invented rate.
//
// NonFinite is always false in a report that exists: a non-finite value aborts
// the run with an error and publishes nothing. The field keeps the schema
// explicit about what was checked.
type Monitors struct {
	SilentFraction        float64    `json:"silent_fraction"`
	PopulationRatePerStep []float64  `json:"population_rate_per_step"`
	RateQuantiles         [5]float64 `json:"rate_quantiles"`
	NonFinite             bool       `json:"non_finite"`
}

// RunReport is the complete, JSON-serializable result of one Run call. Two runs
// with identical graph, parameters, protocol and stimulus encode to identical
// bytes. StepsBefore and StepsAfter are the core step counter around this call,
// so a run continued from a saved state can be audited.
type RunReport struct {
	SchemaVersion  string      `json:"schema_version"`
	Core           string      `json:"core"`
	CoreConfigHash string      `json:"core_config_hash"`
	GraphHashes    GraphHashes `json:"graph_hashes"`
	// TopologyHash fingerprints the edge arrays the run was executed on: the
	// graph's own wiring for an original run, the derivative's for a null
	// model. NullModel is present only for a null model and records the kind,
	// the seed and the counts a reader needs to rebuild the same derivative.
	TopologyHash    string           `json:"topology_hash"`
	NullModel       *NullModelReport `json:"null_model,omitempty"`
	ParameterSource string           `json:"parameter_source"`
	ParameterHash   string           `json:"parameter_hash"`
	// The five fields below describe the derived parameter source and are
	// absent from a report of the uniform source. UnknownSignEdges is a
	// pointer so that a derived run with no unknown edge still reports the
	// zero instead of hiding the field.
	ParameterSetSHA256 string              `json:"parameter_set_sha256,omitempty"`
	RulesHash          string              `json:"rules_hash,omitempty"`
	UnknownSignPolicy  string              `json:"unknown_sign_policy,omitempty"`
	UnknownSignEdges   *uint64             `json:"unknown_sign_edges,omitempty"`
	WeightScale        float64             `json:"weight_scale,omitempty"`
	ProtocolHash       string              `json:"protocol_hash"`
	Steps              int                 `json:"steps"`
	StepsBefore        uint64              `json:"steps_before"`
	StepsAfter         uint64              `json:"steps_after"`
	Nodes              int                 `json:"nodes"`
	Edges              int                 `json:"edges"`
	Injections         []ResolvedInjection `json:"injections"`
	Probes             []ProbeResult       `json:"probes"`
	Monitors           Monitors            `json:"monitors"`
	StabilityFlags     []string            `json:"stability_flags"`
	Assumptions        []string            `json:"assumptions"`
	// Plasticity is present only when the protocol declared the block, so a
	// report of a run without local fast changes encodes exactly as it did
	// before the field existed.
	Plasticity *PlasticityReport `json:"plasticity,omitempty"`
}

// Stability flag names. They report an observation and never change a run.
const (
	FlagMaxPopulationRateExceeded = "max_population_rate_exceeded"
	FlagMinActiveFractionBelow    = "min_active_fraction_below_threshold"
)

// quantiles returns p0, p25, p50, p75 and p100 of values using linear
// interpolation between order statistics: for q the position is q*(n-1) and the
// result interpolates between the two neighbouring sorted values. The input is
// copied before sorting.
func quantiles(values []float64) [5]float64 {
	var result [5]float64
	if len(values) == 0 {
		return result
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	for i, q := range [5]float64{0, .25, .5, .75, 1} {
		position := q * float64(len(sorted)-1)
		low := int(math.Floor(position))
		high := int(math.Ceil(position))
		if low == high {
			result[i] = sorted[low]
			continue
		}
		result[i] = sorted[low] + (position-float64(low))*(sorted[high]-sorted[low])
	}
	return result
}

// checkFinite rejects a series that left the finite range, naming where.
func checkFinite(values []float64, what string) error {
	for i, v := range values {
		if !finite(v) {
			return fmt.Errorf("simulate: %s[%d] is non-finite", what, i)
		}
	}
	return nil
}
