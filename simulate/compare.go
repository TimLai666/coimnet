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
	"github.com/TimLai666/coimnet/plasticity"
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

// learningReproduction is appended to reproduction when the comparison declares
// learning variants, so a comparison that declares none keeps the exact string
// earlier reports recorded.
const learningReproduction = " The learning variants run on the original wiring: original is the same protocol with the plasticity block removed, plastic runs the block as declared, and learned_then_frozen reruns the same stimulus with the weights fixed at base plus the fast changes the plastic cell ended with and no further update. Each cell's plasticity block reports the rule, the enabled edges, the gate channel, both clamp counts and the Euclidean norm of the fast changes before and after the run."

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
	// LearningVariants declares which of original, plastic and
	// learned_then_frozen run on the original wiring, in that order. It is
	// declared exactly when the run protocol declares a plasticity block:
	// original strips the block and is the ticket 14 cell, plastic runs it,
	// and learned_then_frozen reruns the same stimulus with w_eff fixed at
	// base plus the fast changes the plastic cell ended with. Without the
	// field the matrix is the single original cell it always was.
	LearningVariants []string `json:"learning_variants,omitempty"`
}

// MetricDelta is one cell's metric minus the original cell's value for the
// same metric. It is defined only when both sides have a value: a difference
// against something that was never measured is not zero, it does not exist.
// An undefined delta carries no value, exactly as an undefined metric does.
type MetricDelta struct {
	Metric  string  `json:"metric"`
	Value   float64 `json:"value"`
	Defined bool    `json:"defined"`
}

// CompareCell is one variant run with its metrics and thresholds. WallSeconds
// is the only field that differs between two identical comparisons.
//
// Deltas follows the declared metric order and holds this cell's difference
// from the original cell. The original's own deltas are therefore zero
// wherever its metrics are defined.
type CompareCell struct {
	Index       int               `json:"index"`
	Variant     string            `json:"variant"`
	Kind        string            `json:"kind,omitempty"`
	Seed        uint64            `json:"seed"`
	Null        *NullModelReport  `json:"null_model,omitempty"`
	Run         RunReport         `json:"run"`
	Metrics     []MetricResult    `json:"metrics"`
	Deltas      []MetricDelta     `json:"deltas_from_original"`
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

// learningVariants returns the cells the matrix runs on the original wiring, in
// declaration order. A comparison that declares none is the single original
// cell of ticket 14.
func (c CompareProtocol) learningVariants() []string {
	if len(c.LearningVariants) == 0 {
		return []string{VariantOriginal}
	}
	return c.LearningVariants
}

// validateLearningVariants checks the learning matrix on its own. The block and
// the variant list are declared together, original comes first because every
// delta is taken from it, and learned_then_frozen needs the plastic cell that
// produces the fast changes it freezes.
func (c CompareProtocol) validateLearningVariants() error {
	if c.Run.Plasticity == nil && len(c.LearningVariants) != 0 {
		return fmt.Errorf("simulate: learning_variants are declared but the run protocol carries no plasticity block, so %q and %q would have no rule to run", VariantPlastic, VariantLearnedThenFrozen)
	}
	if c.Run.Plasticity != nil && len(c.LearningVariants) == 0 {
		return fmt.Errorf("simulate: the run protocol declares a plasticity block, so the comparison must declare learning_variants and say which of %q, %q and %q to run", VariantOriginal, VariantPlastic, VariantLearnedThenFrozen)
	}
	seen := make(map[string]struct{}, len(c.LearningVariants))
	for i, name := range c.LearningVariants {
		switch name {
		case VariantOriginal, VariantPlastic, VariantLearnedThenFrozen:
		default:
			return fmt.Errorf("simulate: learning variant %q is not %q, %q or %q", name, VariantOriginal, VariantPlastic, VariantLearnedThenFrozen)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("simulate: duplicate learning variant %q", name)
		}
		if i == 0 && name != VariantOriginal {
			return fmt.Errorf("simulate: learning variant %q is declared first, but %q is the cell every delta is taken from and must come first", name, VariantOriginal)
		}
		if name == VariantLearnedThenFrozen {
			if _, exists := seen[VariantPlastic]; !exists {
				return fmt.Errorf("simulate: learning variant %q freezes the fast changes of %q, which must be declared before it", VariantLearnedThenFrozen, VariantPlastic)
			}
		}
		seen[name] = struct{}{}
	}
	return nil
}

// Validate checks everything that does not need a graph: the run protocol, the
// declared names, the metric definitions against the core and the step count,
// the thresholds against the metrics, the null models against the seeds and the
// parameter source, and the learning variants against the plasticity block.
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
	return c.validateLearningVariants()
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
	if len(cp.LearningVariants) != 0 {
		report.Reproduction += learningReproduction
	}
	if cp.Run.ParameterSource == ParameterSourceDerived {
		report.ParameterSetSHA256 = setSHA256
	}

	// The learning variants run first and on the untouched wiring, so cell 0
	// is the original every delta is taken from whether or not the comparison
	// declares any of them.
	var learned plasticity.State
	for _, name := range cp.learningVariants() {
		run, initial, err := cp.learningCell(name, learned)
		if err != nil {
			return empty, err
		}
		variant, err := OriginalVariant(ctx, g, set, setSHA256, run, limits)
		if err != nil {
			return empty, err
		}
		variant.Name = name
		cell, final, err := runCell(ctx, g, cellRequest{
			variant: variant, run: run, index: len(report.Cells), plastic: initial,
		}, cp, sets, windows, limits)
		if err != nil {
			return empty, err
		}
		if name == VariantPlastic {
			learned = final
		}
		if len(report.Cells) == 0 {
			cell.Deltas = deltasFromOriginal(cell.Metrics, cell.Metrics)
		} else {
			cell.Deltas = deltasFromOriginal(report.Cells[0].Metrics, cell.Metrics)
		}
		report.Cells = append(report.Cells, cell)
	}
	// A null model cell is the ticket 14 cell and carries no plasticity: this
	// matrix compares learning against the original wiring, and comparing it
	// against shuffled wiring is a different question with its own protocol.
	nullRun := cp.Run
	nullRun.Plasticity = nil
	for _, spec := range cp.NullModels {
		for _, seed := range cp.Seeds {
			seeded := spec
			seeded.Seed = seed
			variant, _, err := DeriveNullModel(ctx, g, set, setSHA256, nullRun, seeded, limits)
			if err != nil {
				return empty, err
			}
			cell, _, err := runCell(ctx, g, cellRequest{
				variant: variant, run: nullRun, index: len(report.Cells), kind: spec.Kind, seed: seed,
			}, cp, sets, windows, limits)
			if err != nil {
				return empty, err
			}
			cell.Deltas = deltasFromOriginal(report.Cells[0].Metrics, cell.Metrics)
			report.Cells = append(report.Cells, cell)
		}
	}
	report.Summary = summarize(cp, report.Cells)
	return report, nil
}

// learningCell returns the run protocol one learning variant uses and, for
// learned_then_frozen, the fast state it starts from. original strips the
// block, plastic runs it as declared, and learned_then_frozen declares the same
// block with updates frozen and is handed the fast changes the plastic cell
// ended with, so the weights it integrates are base plus exactly those.
func (c CompareProtocol) learningCell(name string, learned plasticity.State) (Protocol, *plasticity.State, error) {
	run := c.Run
	switch name {
	case VariantOriginal:
		run.Plasticity = nil
		return run, nil, nil
	case VariantPlastic:
		return run, nil, nil
	case VariantLearnedThenFrozen:
		frozen := *c.Run.Plasticity
		frozen.Frozen = true
		run.Plasticity = &frozen
		state := clonePlasticState(learned)
		return run, &state, nil
	}
	return Protocol{}, nil, fmt.Errorf("simulate: unsupported learning variant %q", name)
}

// cellRequest is one cell of the matrix: the variant to run, the run protocol
// that cell uses, which is the declared one with the plasticity block stripped,
// declared or frozen, and the fast state the cell starts from.
type cellRequest struct {
	variant Variant
	run     Protocol
	index   int
	kind    string
	seed    uint64
	plastic *plasticity.State
}

// runCell builds, tracks, runs and evaluates one cell and returns the fast
// state it ended with, which learned_then_frozen freezes.
func runCell(ctx context.Context, g *connectome.Graph, req cellRequest, cp CompareProtocol, sets []ResolvedSet, windows [][2]int, limits Limits) (CompareCell, plasticity.State, error) {
	started := time.Now()
	fail := func(err error) (CompareCell, plasticity.State, error) {
		return CompareCell{}, plasticity.State{}, fmt.Errorf("simulate: compare cell %d (%s): %w", req.index, req.variant.Name, err)
	}
	runner, err := BuildVariant(ctx, g, req.variant, req.run, limits)
	if err != nil {
		return fail(err)
	}
	if req.plastic != nil {
		if err := runner.RestorePlasticState(*req.plastic); err != nil {
			return fail(err)
		}
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
		Index: req.index, Variant: req.variant.Name, Kind: req.kind, Seed: req.seed,
		Run: run, Metrics: metrics, Thresholds: thresholds,
		WallSeconds: time.Since(started).Seconds(),
	}
	if req.variant.Null != nil {
		cell.Null = &NullModelReport{}
		*cell.Null = *req.variant.Null
	}
	return cell, runner.PlasticState(), nil
}

// deltasFromOriginal subtracts the original cell's metric values from one
// cell's, in the declared metric order. Both slices are the evaluation of the
// same declared metrics, so they are the same length and the same order; a
// metric that either side left undefined produces an undefined delta.
func deltasFromOriginal(original, cell []MetricResult) []MetricDelta {
	deltas := make([]MetricDelta, 0, len(cell))
	for i, result := range cell {
		delta := MetricDelta{Metric: result.Name}
		if result.Defined && original[i].Defined {
			delta.Value, delta.Defined = result.Value-original[i].Value, true
		}
		deltas = append(deltas, delta)
	}
	return deltas
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
