package simulate

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/TimLai666/coimnet/connectome"
)

// Metric kinds. Every definition is fixed here and enters the compare protocol
// hash, so a threshold can never be read against a redefined metric. A metric
// that has no value is undefined and says so; it is never reported as zero.
const (
	// MetricSpikeFraction is the number of nodes of the set that spiked at
	// least once inside the window, over the size of the set.
	MetricSpikeFraction = "spike_fraction"
	// MetricMeanRate is the spike events of the set inside the window over the
	// set size times the window length: the mean spikes per neuron per step.
	MetricMeanRate = "mean_rate"
	// MetricLatencyToFirstSpike is the first step inside the window at which
	// any node of the set spiked, minus the window start. It is undefined when
	// no node of the set spiked in the window.
	MetricLatencyToFirstSpike = "latency_to_first_spike"
	// MetricActivityRatioVsBaseline is the mean_rate of the window over the
	// mean_rate of the baseline window. It is undefined when the baseline rate
	// is zero.
	MetricActivityRatioVsBaseline = "activity_ratio_vs_baseline"
	// MetricMeanOutput is the mean core output over the set and the window. It
	// is the only metric the continuous core can produce, because that core
	// emits no events; on the spiking core it reads the synaptic trace.
	MetricMeanOutput = "mean_output"

	// Threshold operators. There is no equality operator: a floating point
	// metric is not compared for exact equality in a declared threshold.
	OpAtLeast = ">="
	OpAbove   = ">"
	OpAtMost  = "<="
	OpBelow   = "<"

	// ReasonUndefined is the recorded reason an undefined metric fails its
	// threshold. An undefined metric never passes.
	ReasonUndefined = "undefined"
)

// NamedSet is a neuron set the user names. The framework attaches no meaning
// to the name: it is the intersection of the declared selectors and nothing
// else. An empty intersection is an error unless every selector allows it.
type NamedSet struct {
	Name      string     `json:"name"`
	Selectors []Selector `json:"selectors"`
}

// ResolvedSet is what a NamedSet actually matched. NodeHash is the SHA-256 of
// the ascending node indices as little-endian uint32, so two reports can be
// compared without the graph. The node indices themselves stay private: a set
// is obtained from ResolveSets and is not a value a caller can forge.
type ResolvedSet struct {
	Name      string               `json:"name"`
	Selectors []SelectorResolution `json:"selectors"`
	Count     int                  `json:"count"`
	NodeHash  string               `json:"node_hash"`

	nodes []int
}

// Nodes returns the ascending node indices the set resolved to, as a copy the
// caller owns. It is the read path for a consumer outside this package, such as
// a modulation source that averages the activity of a named set: the set still
// has to come from ResolveSets, so the caller reads a resolution instead of
// declaring one. A set that resolved to no node returns an empty slice.
func (s ResolvedSet) Nodes() []int {
	owned := make([]int, len(s.nodes))
	copy(owned, s.nodes)
	return owned
}

// ResolveSets resolves every named set against the graph in declared order.
func ResolveSets(ctx context.Context, g *connectome.Graph, sets []NamedSet) ([]ResolvedSet, error) {
	if ctx == nil {
		return nil, errors.New("simulate: nil context")
	}
	if g == nil || g.NodeCount() == 0 {
		return nil, errors.New("simulate: nil or empty graph")
	}
	if len(sets) == 0 {
		return nil, errors.New("simulate: no named set was declared")
	}
	seen := map[string]struct{}{}
	resolved := make([]ResolvedSet, 0, len(sets))
	for i, set := range sets {
		if set.Name == "" {
			return nil, fmt.Errorf("simulate: named set %d has no name", i)
		}
		if _, exists := seen[set.Name]; exists {
			return nil, fmt.Errorf("simulate: duplicate named set %q", set.Name)
		}
		seen[set.Name] = struct{}{}
		if len(set.Selectors) == 0 {
			return nil, fmt.Errorf("simulate: named set %q declares no selector", set.Name)
		}
		var nodes []int
		allowEmpty := true
		record := ResolvedSet{Name: set.Name}
		for s, selector := range set.Selectors {
			matched, resolution, err := resolveSelector(ctx, g, selector)
			if err != nil {
				return nil, fmt.Errorf("simulate: named set %q: %w", set.Name, err)
			}
			record.Selectors = append(record.Selectors, resolution)
			allowEmpty = allowEmpty && selector.AllowEmpty
			if s == 0 {
				nodes = matched
				continue
			}
			nodes = intersect(nodes, matched)
		}
		if len(nodes) == 0 && !allowEmpty {
			return nil, fmt.Errorf("simulate: named set %q resolved to no node; set allow_empty on every selector to accept that", set.Name)
		}
		record.nodes = nodes
		record.Count = len(nodes)
		record.NodeHash = nodeHash(nodes)
		resolved = append(resolved, record)
	}
	return resolved, nil
}

// intersect merges two ascending, distinct index lists.
func intersect(a, b []int) []int {
	out := make([]int, 0, min(len(a), len(b)))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			out = append(out, a[i])
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	return out
}

// nodeHash fingerprints a resolved set: the ascending node indices as
// little-endian uint32, hashed with SHA-256.
func nodeHash(nodes []int) string {
	digest := sha256.New()
	buffer := make([]byte, 4)
	for _, node := range nodes {
		binary.LittleEndian.PutUint32(buffer, uint32(node))
		digest.Write(buffer)
	}
	return hex.EncodeToString(digest.Sum(nil))
}

// Metric is one declared measurement. Window and Baseline are step ranges
// [start, end) inside one run. Baseline is used by
// activity_ratio_vs_baseline only and must stay at zero for every other kind,
// so a protocol never carries a silently ignored range.
type Metric struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Set      string `json:"set"`
	Window   [2]int `json:"window"`
	Baseline [2]int `json:"baseline"`
}

// MetricResult is one metric evaluated over one run. Numerator and Denominator
// are the two quantities the definition divides, so a reader can audit the
// value without the run; for latency_to_first_spike they are the first spike
// step and the window start and the value is their difference. A result with
// Defined false carries no value: its Value is zero because there is none.
type MetricResult struct {
	Name        string  `json:"name"`
	Kind        string  `json:"kind"`
	Set         string  `json:"set"`
	Value       float64 `json:"value"`
	Defined     bool    `json:"defined"`
	Numerator   float64 `json:"numerator"`
	Denominator float64 `json:"denominator"`
	Basis       string  `json:"basis"`
}

// Threshold is a pass or fail rule written before the run.
type Threshold struct {
	Metric string  `json:"metric"`
	Op     string  `json:"op"`
	Value  float64 `json:"value"`
}

// ThresholdResult repeats the declaration next to the observation, so a report
// cannot be read as if the rule had been chosen afterwards. An undefined
// metric always fails and records ReasonUndefined.
type ThresholdResult struct {
	Metric   string  `json:"metric"`
	Op       string  `json:"op"`
	Value    float64 `json:"value"`
	Observed float64 `json:"observed"`
	Passed   bool    `json:"passed"`
	Reason   string  `json:"reason"`
}

// WindowMeasurement is what one Run recorded for one tracked set over one
// window. FirstSpikeStep is -1 when no node of the set spiked in the window.
type WindowMeasurement struct {
	Set            string  `json:"set"`
	Window         [2]int  `json:"window"`
	Nodes          int     `json:"nodes"`
	Steps          int     `json:"steps"`
	Spiking        bool    `json:"spiking"`
	SpikedNodes    int     `json:"spiked_nodes"`
	Spikes         uint64  `json:"spikes"`
	FirstSpikeStep int     `json:"first_spike_step"`
	OutputSum      float64 `json:"output_sum"`
}

// Measurements are the set tracking results of the most recent Run.
type Measurements struct {
	Windows []WindowMeasurement `json:"windows"`
}

func (m Measurements) lookup(set string, window [2]int) (WindowMeasurement, bool) {
	for _, measurement := range m.Windows {
		if measurement.Set == set && measurement.Window == window {
			return measurement, true
		}
	}
	return WindowMeasurement{}, false
}

// MetricWindows returns every step range the metrics need, in first seen
// order and without repeats, which is what Runner.TrackSets expects.
func MetricWindows(metrics []Metric) [][2]int {
	var windows [][2]int
	add := func(window [2]int) {
		for _, existing := range windows {
			if existing == window {
				return
			}
		}
		windows = append(windows, window)
	}
	for _, metric := range metrics {
		add(metric.Window)
		if metric.Kind == MetricActivityRatioVsBaseline {
			add(metric.Baseline)
		}
	}
	return windows
}

// validate checks one metric against the core that will run and the number of
// steps the protocol declares.
func (m Metric) validate(core string, steps int, sets map[string]struct{}) error {
	if m.Name == "" {
		return errors.New("simulate: a metric has no name")
	}
	switch m.Kind {
	case MetricSpikeFraction, MetricMeanRate, MetricLatencyToFirstSpike, MetricActivityRatioVsBaseline:
		if core != CoreLIF {
			return fmt.Errorf("simulate: metric %q of kind %q needs a spiking core; the %q core emits no events, so only %q is available", m.Name, m.Kind, core, MetricMeanOutput)
		}
	case MetricMeanOutput:
	default:
		return fmt.Errorf("simulate: metric %q has unsupported kind %q", m.Name, m.Kind)
	}
	if _, known := sets[m.Set]; !known {
		return fmt.Errorf("simulate: metric %q names the undeclared set %q", m.Name, m.Set)
	}
	if err := checkWindow(m.Window, steps, "metric "+m.Name+" window"); err != nil {
		return err
	}
	if m.Kind == MetricActivityRatioVsBaseline {
		return checkWindow(m.Baseline, steps, "metric "+m.Name+" baseline")
	}
	if m.Baseline != [2]int{} {
		return fmt.Errorf("simulate: metric %q of kind %q uses no baseline, so it must be absent, got %v", m.Name, m.Kind, m.Baseline)
	}
	return nil
}

func checkWindow(window [2]int, steps int, what string) error {
	if window[0] < 0 || window[1] <= window[0] {
		return fmt.Errorf("simulate: %s %v must be a step range [start,end) with end above start", what, window)
	}
	if window[1] > steps {
		return fmt.Errorf("simulate: %s %v leaves the %d steps of the run", what, window, steps)
	}
	return nil
}

// validate checks one threshold against the metrics that will be evaluated.
func (t Threshold) validate(metrics map[string]struct{}) error {
	if _, known := metrics[t.Metric]; !known {
		return fmt.Errorf("simulate: threshold names the undeclared metric %q", t.Metric)
	}
	switch t.Op {
	case OpAtLeast, OpAbove, OpAtMost, OpBelow:
	default:
		return fmt.Errorf("simulate: threshold on %q has unsupported operator %q; choose %q, %q, %q or %q", t.Metric, t.Op, OpAtLeast, OpAbove, OpAtMost, OpBelow)
	}
	if !finite(t.Value) {
		return fmt.Errorf("simulate: threshold on %q has a non-finite value", t.Metric)
	}
	return nil
}

// EvaluateMetrics applies the declared definitions to one run's measurements.
func EvaluateMetrics(metrics []Metric, m Measurements) ([]MetricResult, error) {
	results := make([]MetricResult, 0, len(metrics))
	for _, metric := range metrics {
		window, known := m.lookup(metric.Set, metric.Window)
		if !known {
			return nil, fmt.Errorf("simulate: metric %q needs set %q tracked over %v, which this run did not measure", metric.Name, metric.Set, metric.Window)
		}
		result := MetricResult{Name: metric.Name, Kind: metric.Kind, Set: metric.Set}
		switch metric.Kind {
		case MetricSpikeFraction:
			if !window.Spiking {
				return nil, spikingMetricError(metric)
			}
			result.Numerator, result.Denominator = float64(window.SpikedNodes), float64(window.Nodes)
			result.Basis = fmt.Sprintf("nodes of %q that spiked at least once in steps [%d,%d) over the %d nodes the set resolved to", metric.Set, metric.Window[0], metric.Window[1], window.Nodes)
		case MetricMeanRate:
			if !window.Spiking {
				return nil, spikingMetricError(metric)
			}
			result.Numerator, result.Denominator = float64(window.Spikes), float64(window.Nodes)*float64(window.Steps)
			result.Basis = fmt.Sprintf("spike events of %q in steps [%d,%d) over %d nodes times %d steps", metric.Set, metric.Window[0], metric.Window[1], window.Nodes, window.Steps)
		case MetricMeanOutput:
			result.Numerator, result.Denominator = window.OutputSum, float64(window.Nodes)*float64(window.Steps)
			result.Basis = fmt.Sprintf("summed core output of %q in steps [%d,%d) over %d nodes times %d steps", metric.Set, metric.Window[0], metric.Window[1], window.Nodes, window.Steps)
		case MetricLatencyToFirstSpike:
			if !window.Spiking {
				return nil, spikingMetricError(metric)
			}
			result.Basis = fmt.Sprintf("first step in [%d,%d) at which any node of %q spiked, minus the window start; undefined when the set stayed silent", metric.Window[0], metric.Window[1], metric.Set)
			if window.FirstSpikeStep >= 0 {
				result.Numerator, result.Denominator = float64(window.FirstSpikeStep), float64(metric.Window[0])
				result.Value, result.Defined = result.Numerator-result.Denominator, true
			}
			results = append(results, result)
			continue
		case MetricActivityRatioVsBaseline:
			if !window.Spiking {
				return nil, spikingMetricError(metric)
			}
			baseline, known := m.lookup(metric.Set, metric.Baseline)
			if !known {
				return nil, fmt.Errorf("simulate: metric %q needs set %q tracked over its baseline %v, which this run did not measure", metric.Name, metric.Set, metric.Baseline)
			}
			result.Basis = fmt.Sprintf("mean_rate of %q in steps [%d,%d) over its mean_rate in the baseline steps [%d,%d); undefined when the baseline rate is zero",
				metric.Set, metric.Window[0], metric.Window[1], metric.Baseline[0], metric.Baseline[1])
			windowRate, windowKnown := rate(window)
			baselineRate, baselineKnown := rate(baseline)
			if !windowKnown || !baselineKnown {
				results = append(results, result)
				continue
			}
			result.Numerator, result.Denominator = windowRate, baselineRate
		default:
			return nil, fmt.Errorf("simulate: metric %q has unsupported kind %q", metric.Name, metric.Kind)
		}
		if result.Denominator != 0 {
			result.Value, result.Defined = result.Numerator/result.Denominator, true
			if !finite(result.Value) {
				return nil, fmt.Errorf("simulate: metric %q produced a non-finite value", metric.Name)
			}
		}
		results = append(results, result)
	}
	return results, nil
}

func spikingMetricError(metric Metric) error {
	return fmt.Errorf("simulate: metric %q of kind %q needs events, and the core of this run produced none", metric.Name, metric.Kind)
}

// rate is the mean_rate of one measurement, or false when the set or the
// window is empty and there is nothing to average.
func rate(m WindowMeasurement) (float64, bool) {
	denominator := float64(m.Nodes) * float64(m.Steps)
	if denominator == 0 {
		return 0, false
	}
	return float64(m.Spikes) / denominator, true
}

// EvaluateThresholds compares each declared threshold against the metric it
// names. An undefined metric fails, whatever the operator would have said.
func EvaluateThresholds(thresholds []Threshold, results []MetricResult) ([]ThresholdResult, error) {
	byName := make(map[string]MetricResult, len(results))
	names := make(map[string]struct{}, len(results))
	for _, result := range results {
		byName[result.Name] = result
		names[result.Name] = struct{}{}
	}
	evaluated := make([]ThresholdResult, 0, len(thresholds))
	for _, threshold := range thresholds {
		if err := threshold.validate(names); err != nil {
			return nil, err
		}
		metric := byName[threshold.Metric]
		result := ThresholdResult{Metric: threshold.Metric, Op: threshold.Op, Value: threshold.Value}
		if !metric.Defined {
			result.Reason = ReasonUndefined
			evaluated = append(evaluated, result)
			continue
		}
		result.Observed = metric.Value
		switch threshold.Op {
		case OpAtLeast:
			result.Passed = metric.Value >= threshold.Value
		case OpAbove:
			result.Passed = metric.Value > threshold.Value
		case OpAtMost:
			result.Passed = metric.Value <= threshold.Value
		default:
			result.Passed = metric.Value < threshold.Value
		}
		evaluated = append(evaluated, result)
	}
	return evaluated, nil
}

// quantilesNearestRank returns p0, p25, p50, p75 and p100 by the nearest rank
// rule the params package uses for its derivation report: for a fraction q the
// rank is ceil(q*n) clamped into [1,n] and the result is the value at that
// rank of the ascending values. It never interpolates, so every reported
// quantile is a value a seed actually produced. The input is not modified.
func quantilesNearestRank(values []float64) [5]float64 {
	var result [5]float64
	if len(values) == 0 {
		return result
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	for i, q := range [5]float64{0, .25, .5, .75, 1} {
		rank := int(math.Ceil(q * float64(len(sorted))))
		if rank < 1 {
			rank = 1
		}
		if rank > len(sorted) {
			rank = len(sorted)
		}
		result[i] = sorted[rank-1]
	}
	return result
}
