package simulate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/internal/strictjson"
	"github.com/TimLai666/coimnet/params"
)

const (
	// CompareProtocolSchemaVersion identifies the compare protocol contract.
	CompareProtocolSchemaVersion = "coimnet-simulate-compare/v1"
	// CompareReportSchemaVersion identifies the compare report contract.
	CompareReportSchemaVersion = "coimnet-simulate-compare-report/v1"
	// MaxCompareProtocolBytes bounds compare protocol JSON. It carries one run
	// protocol plus the declared sets, metrics and thresholds.
	MaxCompareProtocolBytes int64 = 1 << 20
)

// reproduction is written into every report so the matrix is never read as an
// opaque result: each cell is an ordinary run that can be repeated.
const reproduction = "Every cell is one independent run of the same protocol. " +
	"Cell 0 is reproduced by simulate run with this store, the run protocol and, for the derived source, the same parameter set file. " +
	"A null model cell is reproduced by rebuilding its variant from the same store with the recorded kind, seed and " + NullPRNG +
	" generator and then running that protocol; the recorded topology_hash and parameter_hash must match before the numbers are compared. " +
	"The report states metric values, thresholds declared before the run and the position of the original inside the null model distribution. It makes no claim about behaviour."

// CompareProtocol is the complete, serializable description of one comparison
// matrix: one run protocol, the named neuron sets, the metrics and thresholds
// written before the run, the null models and the seeds they are run with.
// A NullModels entry declares the kind and, for a rewire, the swap factor; the
// seeds come from Seeds, so an entry must leave its own seed unset.
type CompareProtocol struct {
	SchemaVersion string          `json:"schema_version"`
	Run           Protocol        `json:"run"`
	Sets          []NamedSet      `json:"sets"`
	Metrics       []Metric        `json:"metrics"`
	Thresholds    []Threshold     `json:"thresholds"`
	NullModels    []NullModelSpec `json:"null_models,omitempty"`
	Seeds         []uint64        `json:"seeds,omitempty"`
}

// CompareCell is one variant run with its metrics and thresholds. WallSeconds
// is the only field that differs between two identical comparisons.
type CompareCell struct {
	Index       int               `json:"index"`
	Variant     string            `json:"variant"`
	Kind        string            `json:"kind,omitempty"`
	Seed        uint64            `json:"seed"`
	Null        *NullModelReport  `json:"null_model,omitempty"`
	Run         RunReport         `json:"run"`
	Metrics     []MetricResult    `json:"metrics"`
	Thresholds  []ThresholdResult `json:"thresholds"`
	WallSeconds float64           `json:"wall_seconds"`
}

// NullModelSummary places the original inside the distribution one null model
// kind produced over the declared seeds. Quantiles are p0, p25, p50, p75 and
// p100 of the defined values by the nearest rank rule documented on
// quantilesNearestRank, and are zero when no seed produced a defined value.
// OriginalPercentile is (values below + half the values equal) / defined, and
// is absent when the original or every seed is undefined.
type NullModelSummary struct {
	Kind                      string     `json:"kind"`
	Seeds                     int        `json:"seeds"`
	Defined                   int        `json:"defined"`
	Quantiles                 [5]float64 `json:"quantiles"`
	OriginalPercentile        float64    `json:"original_percentile"`
	OriginalPercentileDefined bool       `json:"original_percentile_defined"`
}

// MetricSummary is one metric across the whole matrix. PerKind follows the
// declared order of the null models rather than a map, so the report reads in
// the order the protocol wrote.
type MetricSummary struct {
	Metric   string             `json:"metric"`
	Kind     string             `json:"kind"`
	Set      string             `json:"set"`
	Original MetricResult       `json:"original"`
	PerKind  []NullModelSummary `json:"per_kind"`
}

// CompareReport is the complete result of one Compare call.
type CompareReport struct {
	SchemaVersion      string          `json:"schema_version"`
	ProtocolHash       string          `json:"protocol_hash"`
	GraphHashes        GraphHashes     `json:"graph_hashes"`
	ParameterSource    string          `json:"parameter_source"`
	ParameterSetSHA256 string          `json:"parameter_set_sha256,omitempty"`
	Sets               []ResolvedSet   `json:"sets"`
	Cells              []CompareCell   `json:"cells"`
	Summary            []MetricSummary `json:"summary"`
	Reproduction       string          `json:"reproduction"`
}

// DecodeCompareProtocol reads exactly one strict JSON compare protocol and
// validates everything that does not depend on a graph.
func DecodeCompareProtocol(r io.Reader) (CompareProtocol, error) {
	var protocol CompareProtocol
	if err := strictjson.Decode(r, MaxCompareProtocolBytes, &protocol); err != nil {
		return CompareProtocol{}, fmt.Errorf("simulate: invalid compare protocol: %w", err)
	}
	if err := protocol.Validate(); err != nil {
		return CompareProtocol{}, err
	}
	return protocol, nil
}

// Hash returns the SHA-256 of the canonical compare protocol JSON as lowercase
// hex, so a report names the exact declaration it was run from.
func (c CompareProtocol) Hash() (string, error) {
	encoded, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("simulate: encode compare protocol: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// Validate checks everything that does not need a graph: the run protocol, the
// declared names, the metric definitions against the core and the step count,
// the thresholds against the metrics, and the null models against the seeds
// and the parameter source.
func (c CompareProtocol) Validate() error {
	if c.SchemaVersion != CompareProtocolSchemaVersion {
		return fmt.Errorf("simulate: unsupported compare protocol schema %q, want %q", c.SchemaVersion, CompareProtocolSchemaVersion)
	}
	if err := c.Run.Validate(); err != nil {
		return err
	}
	stimulus, err := c.Run.Stimulus.matrix()
	if err != nil {
		return err
	}
	steps := len(stimulus)
	if len(c.Sets) == 0 {
		return errors.New("simulate: the compare protocol must declare at least one named set")
	}
	setNames := make(map[string]struct{}, len(c.Sets))
	for i, set := range c.Sets {
		if set.Name == "" {
			return fmt.Errorf("simulate: named set %d has no name", i)
		}
		if _, exists := setNames[set.Name]; exists {
			return fmt.Errorf("simulate: duplicate named set %q", set.Name)
		}
		setNames[set.Name] = struct{}{}
		if len(set.Selectors) == 0 {
			return fmt.Errorf("simulate: named set %q declares no selector", set.Name)
		}
	}
	if len(c.Metrics) == 0 {
		return errors.New("simulate: the compare protocol must declare at least one metric")
	}
	metricNames := make(map[string]struct{}, len(c.Metrics))
	for _, metric := range c.Metrics {
		if _, exists := metricNames[metric.Name]; exists {
			return fmt.Errorf("simulate: duplicate metric %q", metric.Name)
		}
		if err := metric.validate(c.Run.Core, steps, setNames); err != nil {
			return err
		}
		metricNames[metric.Name] = struct{}{}
	}
	for _, threshold := range c.Thresholds {
		if err := threshold.validate(metricNames); err != nil {
			return err
		}
	}
	kinds := make(map[string]struct{}, len(c.NullModels))
	for i, spec := range c.NullModels {
		if spec.Seed != 0 {
			return fmt.Errorf("simulate: null model %d carries its own seed %d; every kind is run with the declared seeds, so the spec must leave it unset", i, spec.Seed)
		}
		if _, exists := kinds[spec.Kind]; exists {
			return fmt.Errorf("simulate: duplicate null model kind %q", spec.Kind)
		}
		if err := spec.validate(c.Run.ParameterSource); err != nil {
			return err
		}
		kinds[spec.Kind] = struct{}{}
	}
	if len(c.NullModels) == 0 && len(c.Seeds) != 0 {
		return errors.New("simulate: seeds are declared but no null model uses them")
	}
	if len(c.NullModels) != 0 && len(c.Seeds) == 0 {
		return errors.New("simulate: every declared null model needs at least one seed")
	}
	seeds := make(map[uint64]struct{}, len(c.Seeds))
	for _, seed := range c.Seeds {
		if _, exists := seeds[seed]; exists {
			return fmt.Errorf("simulate: duplicate seed %d", seed)
		}
		seeds[seed] = struct{}{}
	}
	return nil
}

// Compare runs the original wiring and then every declared null model kind
// with every declared seed, in that order, over the same protocol and the same
// stimulus. Each cell is an independent Runner, so no cell can observe another
// one's state, and each cell holds the complete run report it produced.
//
// The graph and the parameter set are read and never modified: a null model is
// an in-memory derivative reproduced from the seed, which is why the report
// carries the hashes instead of a derived store.
func Compare(ctx context.Context, g *connectome.Graph, set *params.Set, setSHA256 string, cp CompareProtocol, limits Limits) (CompareReport, error) {
	var empty CompareReport
	if ctx == nil {
		return empty, errors.New("simulate: nil context")
	}
	if err := ctx.Err(); err != nil {
		return empty, fmt.Errorf("simulate: %w", err)
	}
	if g == nil || g.NodeCount() == 0 {
		return empty, errors.New("simulate: nil or empty graph")
	}
	if err := cp.Validate(); err != nil {
		return empty, err
	}
	protocolHash, err := cp.Hash()
	if err != nil {
		return empty, err
	}
	sets, err := ResolveSets(ctx, g, cp.Sets)
	if err != nil {
		return empty, err
	}
	windows := MetricWindows(cp.Metrics)
	report := CompareReport{
		SchemaVersion:   CompareReportSchemaVersion,
		ProtocolHash:    protocolHash,
		GraphHashes:     GraphHashes{NodeIndex: g.Report().Hashes.NodeIndex, EdgeOrder: g.Report().Hashes.EdgeOrder},
		ParameterSource: cp.Run.ParameterSource,
		Sets:            sets,
		Reproduction:    reproduction,
	}
	if cp.Run.ParameterSource == ParameterSourceDerived {
		report.ParameterSetSHA256 = setSHA256
	}

	original, err := OriginalVariant(ctx, g, set, setSHA256, cp.Run, limits)
	if err != nil {
		return empty, err
	}
	cell, err := runCell(ctx, g, original, cp, sets, windows, limits, 0, "", 0)
	if err != nil {
		return empty, err
	}
	report.Cells = append(report.Cells, cell)
	for _, spec := range cp.NullModels {
		for _, seed := range cp.Seeds {
			seeded := spec
			seeded.Seed = seed
			variant, _, err := DeriveNullModel(ctx, g, set, setSHA256, cp.Run, seeded, limits)
			if err != nil {
				return empty, err
			}
			cell, err := runCell(ctx, g, variant, cp, sets, windows, limits, len(report.Cells), spec.Kind, seed)
			if err != nil {
				return empty, err
			}
			report.Cells = append(report.Cells, cell)
		}
	}
	report.Summary = summarize(cp, report.Cells)
	return report, nil
}

// runCell builds, tracks, runs and evaluates one variant.
func runCell(ctx context.Context, g *connectome.Graph, v Variant, cp CompareProtocol, sets []ResolvedSet, windows [][2]int, limits Limits, index int, kind string, seed uint64) (CompareCell, error) {
	started := time.Now()
	fail := func(err error) (CompareCell, error) {
		return CompareCell{}, fmt.Errorf("simulate: compare cell %d (%s): %w", index, v.Name, err)
	}
	runner, err := BuildVariant(ctx, g, v, cp.Run, limits)
	if err != nil {
		return fail(err)
	}
	if err := runner.TrackSets(sets, windows); err != nil {
		return fail(err)
	}
	run, err := runner.Run(ctx, runner.Stimulus())
	if err != nil {
		return fail(err)
	}
	metrics, err := EvaluateMetrics(cp.Metrics, runner.Measurements())
	if err != nil {
		return fail(err)
	}
	thresholds, err := EvaluateThresholds(cp.Thresholds, metrics)
	if err != nil {
		return fail(err)
	}
	cell := CompareCell{
		Index: index, Variant: v.Name, Kind: kind, Seed: seed,
		Run: run, Metrics: metrics, Thresholds: thresholds,
		WallSeconds: time.Since(started).Seconds(),
	}
	if v.Null != nil {
		cell.Null = &NullModelReport{}
		*cell.Null = *v.Null
	}
	return cell, nil
}

// summarize places the original metric value inside the distribution each null
// model kind produced over the declared seeds. It reports quantiles and a
// percentile and performs no test that was not declared.
func summarize(cp CompareProtocol, cells []CompareCell) []MetricSummary {
	summaries := make([]MetricSummary, 0, len(cp.Metrics))
	for m, metric := range cp.Metrics {
		summary := MetricSummary{
			Metric: metric.Name, Kind: metric.Kind, Set: metric.Set,
			Original: cells[0].Metrics[m],
			PerKind:  make([]NullModelSummary, 0, len(cp.NullModels)),
		}
		for _, spec := range cp.NullModels {
			perKind := NullModelSummary{Kind: spec.Kind}
			var values []float64
			for _, cell := range cells[1:] {
				if cell.Kind != spec.Kind {
					continue
				}
				perKind.Seeds++
				if cell.Metrics[m].Defined {
					values = append(values, cell.Metrics[m].Value)
				}
			}
			perKind.Defined = len(values)
			perKind.Quantiles = quantilesNearestRank(values)
			if summary.Original.Defined && len(values) > 0 {
				below, equal := 0.0, 0.0
				for _, value := range values {
					switch {
					case value < summary.Original.Value:
						below++
					case value == summary.Original.Value:
						equal++
					}
				}
				perKind.OriginalPercentile = (below + 0.5*equal) / float64(len(values))
				perKind.OriginalPercentileDefined = true
			}
			summary.PerKind = append(summary.PerKind, perKind)
		}
		summaries = append(summaries, summary)
	}
	return summaries
}
