package simulate

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/plasticity"
)

// Limits bounds the accounted arrays of one runner. MaxMemoryBytes covers the
// edge arrays this package holds, the copy the core makes, the per-neuron
// parameter and state vectors, the stimulus, the per-chunk input, output and
// event buffers and the probe series. It excludes Go allocator slack, the
// loaded graph and process overhead, so it is an accounting limit, not an RSS
// bound. A run that does not fit is refused with ErrCapacity; the graph is
// never shrunk and the step count is never lowered.
type Limits struct {
	MaxMemoryBytes int64 `json:"max_memory_bytes"`
}

// Accounted bytes per element. The edge arrays exist twice: once here as
// sources, targets and weights, and once inside the core, which copies sources,
// targets and delays in NewLIF/NewContinuous. The per-neuron figure covers
// voltage, one history row, adaptation, refractory counters, bias, log_tau,
// theta_raw, the two membrane coefficients and the base threshold.
const (
	bytesPerEdgeHere = 8 + 8 + 8
	bytesPerEdgeCore = 8 + 8 + 8
	bytesPerNode     = 8 * 10
)

// resolvedProbe is a probe with its node set and reduction fixed.
type resolvedProbe struct {
	name       string
	reduce     string
	nodes      []int
	selector   *SelectorResolution
	scaleByOne bool // spike_fraction and mean_output divide by the node count
}

// Runner owns one core, one parameter set and the persistent neural state of
// one trajectory. It holds no graph, no trainer and no Insyra tensor. Calling
// Run repeatedly continues the same trajectory; a failed call leaves the state
// exactly as it was.
type Runner struct {
	core            core
	params          ParameterSet
	protocolHash    string
	coreConfigHash  string
	graphHashes     GraphHashes
	stimulus        [][]float64
	channels        int
	nodes           int
	edges           int
	injections      []ResolvedInjection
	injectionNodes  [][]int
	probes          []resolvedProbe
	state           StateSnapshot
	maxChunk        int
	stabilityBounds Thresholds
	topologyHash    string
	null            *NullModelReport
	limits          Limits
	accounted       int64
	tracked         []trackedSet
	trackWindows    [][2]int
	measurements    Measurements
	// The plasticity fields are all zero on a run without the block. The
	// runner keeps its own copy of the edge endpoints because
	// plasticity.Step reads them for every enabled edge, and it holds the
	// fast state exactly as it holds the neural one: a failed Run leaves it
	// where it was.
	plastic        *plasticity.Model
	plasticState   plasticity.State
	plasticRule    plasticity.Rule
	plasticGate    int
	plasticScale   float64
	plasticFrozen  bool
	plasticSources []int
	plasticTargets []int
	// interventions is nil on a run without the block. When present it forces
	// the chunk size to one, exactly like plasticity, and the per-step hook
	// applies the declared overrides to the output, the event and the
	// continuation state.
	interventions *interventionConfig
}

// trackedSet is one resolved node set the run measures alongside the probes.
// Tracking never changes a probe, a parameter or the trajectory.
type trackedSet struct {
	name  string
	nodes []int
}

// Build runs the protocol on the wiring the graph itself carries. It is
// BuildVariant applied to the original variant: the topology is streamed from
// the graph here, and every later step, including the accounting, the core and
// the report, is the one code path BuildVariant owns. The graph is read here
// and never retained, so the caller can release it once Build returns.
func Build(ctx context.Context, g *connectome.Graph, params ParameterSet, protocol Protocol, limits Limits) (*Runner, error) {
	nodes, edges, err := buildPreflight(ctx, g, protocol, limits)
	if err != nil {
		return nil, err
	}
	sources, targets, err := streamTopology(ctx, g, nodes, edges)
	if err != nil {
		return nil, err
	}
	return BuildVariant(ctx, g, Variant{Name: VariantOriginal, Sources: sources, Targets: targets, Params: params}, protocol, limits)
}

// buildPreflight repeats the cheap checks and the memory accounting before the
// edge arrays are read, so an oversized graph is refused instead of allocated.
// BuildVariant checks everything again on the variant it is handed.
func buildPreflight(ctx context.Context, g *connectome.Graph, protocol Protocol, limits Limits) (int, int, error) {
	if ctx == nil {
		return 0, 0, errors.New("simulate: nil context")
	}
	if err := ctx.Err(); err != nil {
		return 0, 0, fmt.Errorf("simulate: %w", err)
	}
	if g == nil || g.NodeCount() == 0 {
		return 0, 0, errors.New("simulate: nil or empty graph")
	}
	if limits.MaxMemoryBytes <= 0 {
		return 0, 0, fmt.Errorf("%w: the memory limit must be positive, got %d", ErrCapacity, limits.MaxMemoryBytes)
	}
	if err := protocol.Validate(); err != nil {
		return 0, 0, err
	}
	nodes, edges, err := graphShape(g)
	if err != nil {
		return 0, 0, err
	}
	stimulus, err := protocol.Stimulus.matrix()
	if err != nil {
		return 0, 0, err
	}
	if _, err := account(limits, nodes, edges, len(stimulus), len(stimulus[0]), len(protocol.Probes)); err != nil {
		return 0, 0, err
	}
	return nodes, edges, nil
}

// BuildVariant validates the protocol against the graph, takes the topology
// from the variant instead of the graph, resolves every selector, accounts the
// memory the run needs and constructs the core. The variant's edge arrays are
// handed to the core, which copies them, so the runner retains neither the
// graph nor the caller's slices.
func BuildVariant(ctx context.Context, g *connectome.Graph, v Variant, protocol Protocol, limits Limits) (*Runner, error) {
	if ctx == nil {
		return nil, errors.New("simulate: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("simulate: %w", err)
	}
	if g == nil || g.NodeCount() == 0 {
		return nil, errors.New("simulate: nil or empty graph")
	}
	if limits.MaxMemoryBytes <= 0 {
		return nil, fmt.Errorf("%w: the memory limit must be positive, got %d", ErrCapacity, limits.MaxMemoryBytes)
	}
	if err := protocol.Validate(); err != nil {
		return nil, err
	}
	nodes, edges, err := graphShape(g)
	if err != nil {
		return nil, err
	}
	if v.Name == "" {
		return nil, errors.New("simulate: the variant has no name")
	}
	if len(v.Sources) != edges || len(v.Targets) != edges {
		return nil, fmt.Errorf("simulate: variant %q carries %d sources and %d targets, the graph declares %d edges", v.Name, len(v.Sources), len(v.Targets), edges)
	}
	for i, source := range v.Sources {
		if source < 0 || source >= nodes || v.Targets[i] < 0 || v.Targets[i] >= nodes {
			return nil, fmt.Errorf("simulate: variant %q edge %d (%d->%d) leaves the node range [0,%d)", v.Name, i, source, v.Targets[i], nodes)
		}
	}
	if err := v.Params.validate(nodes, edges, protocol.Core); err != nil {
		return nil, err
	}
	if v.Params.Source != protocol.ParameterSource {
		return nil, fmt.Errorf("simulate: parameter set source %q does not match the protocol source %q", v.Params.Source, protocol.ParameterSource)
	}
	stimulus, err := protocol.Stimulus.matrix()
	if err != nil {
		return nil, err
	}
	steps, channels := len(stimulus), len(stimulus[0])

	accounted, err := account(limits, nodes, edges, steps, channels, len(protocol.Probes))
	if err != nil {
		return nil, err
	}

	built, coreHash, err := newCore(protocol, nodes, v.Sources, v.Targets)
	if err != nil {
		return nil, err
	}
	protocolHash, err := protocol.Hash()
	if err != nil {
		return nil, err
	}
	topology, err := topologyHash(v.Sources, v.Targets)
	if err != nil {
		return nil, err
	}
	runner := &Runner{
		core:            built,
		params:          v.Params,
		protocolHash:    protocolHash,
		coreConfigHash:  coreHash,
		graphHashes:     GraphHashes{NodeIndex: g.Report().Hashes.NodeIndex, EdgeOrder: g.Report().Hashes.EdgeOrder},
		stimulus:        stimulus,
		channels:        channels,
		nodes:           nodes,
		edges:           edges,
		maxChunk:        built.maxStepsPerCall(),
		stabilityBounds: protocol.Thresholds,
		topologyHash:    topology,
		limits:          limits,
		accounted:       accounted,
	}
	if v.Null != nil {
		runner.null = &NullModelReport{}
		*runner.null = *v.Null
	}
	if err := runner.resolveInjections(ctx, g, protocol, channels); err != nil {
		return nil, err
	}
	if err := runner.resolveProbes(ctx, g, protocol); err != nil {
		return nil, err
	}
	if runner.state, err = built.initial(); err != nil {
		return nil, err
	}
	if err := runner.enablePlasticity(ctx, g, v, protocol); err != nil {
		return nil, err
	}
	if err := runner.enableInterventions(protocol); err != nil {
		return nil, err
	}
	return runner, nil
}

// enablePlasticity resolves the declared block against this variant's topology
// and fixes the chunk size at one step. It is the last construction step, so a
// refused block leaves nothing behind.
func (r *Runner) enablePlasticity(ctx context.Context, g *connectome.Graph, v Variant, protocol Protocol) error {
	block := protocol.Plasticity
	if block == nil {
		return nil
	}
	enabled, err := plasticEdges(ctx, g, *block, v.Sources, v.Targets)
	if err != nil {
		return err
	}
	if err := r.accountPlasticity(len(enabled), block.Rule.Kind == plasticity.RuleSTDPPair); err != nil {
		return err
	}
	model, err := plasticity.New(plasticity.Config{Rule: block.Rule, Edges: enabled}, r.edges)
	if err != nil {
		return fmt.Errorf("simulate: %w", err)
	}
	r.plastic = model
	r.plasticState = model.NewState()
	r.plasticRule = block.Rule
	r.plasticGate, r.plasticScale, r.plasticFrozen = block.GateChannel, block.GateScale, block.Frozen
	r.plasticSources = append([]int(nil), v.Sources...)
	r.plasticTargets = append([]int(nil), v.Targets...)
	// The fast changes of one step decide the weights of the next one, so the
	// core can no longer be handed a whole chunk.
	r.maxChunk = 1
	return nil
}

// graphShape returns the node and edge counts as platform ints.
func graphShape(g *connectome.Graph) (int, int, error) {
	nodes, edges := int(g.NodeCount()), int(g.EdgeCount())
	if uint64(nodes) != g.NodeCount() || uint64(edges) != g.EdgeCount() {
		return 0, 0, fmt.Errorf("%w: graph size does not fit this platform's int", ErrCapacity)
	}
	return nodes, edges, nil
}

// streamTopology reads the canonical edge stream once. Delays are zero for
// every edge: the release carries no conduction delay and inventing one would
// be an undeclared assumption.
func streamTopology(ctx context.Context, g *connectome.Graph, nodes, edges int) ([]int, []int, error) {
	sources := make([]int, 0, edges)
	targets := make([]int, 0, edges)
	err := g.StreamAnnotatedEdges(ctx, func(edge connectome.EdgeRecord) error {
		if edge.Source >= uint64(nodes) || edge.Target >= uint64(nodes) {
			return fmt.Errorf("edge %d->%d leaves the node range [0,%d)", edge.Source, edge.Target, nodes)
		}
		sources = append(sources, int(edge.Source))
		targets = append(targets, int(edge.Target))
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if len(sources) != edges {
		return nil, nil, fmt.Errorf("simulate: streamed %d edges, the graph declares %d", len(sources), edges)
	}
	return sources, targets, nil
}

func newCore(protocol Protocol, nodes int, sources, targets []int) (core, string, error) {
	delays := make([]int, len(sources))
	switch protocol.Core {
	case CoreLIF:
		config := *protocol.LIF
		config.Nodes, config.Sources, config.Targets, config.Delays = nodes, sources, targets, delays
		model, err := dynamics.NewLIF(config)
		if err != nil {
			return nil, "", fmt.Errorf("simulate: %w", err)
		}
		built := lifCore{model: model, nodes: nodes}
		hash, err := built.configHash()
		return built, hash, err
	default:
		config := *protocol.Continuous
		config.Nodes, config.Sources, config.Targets, config.Delays = nodes, sources, targets, delays
		model, err := dynamics.NewContinuous(config)
		if err != nil {
			return nil, "", fmt.Errorf("simulate: %w", err)
		}
		built := continuousCore{model: model, nodes: nodes}
		hash, err := built.configHash()
		return built, hash, err
	}
}

// resolveInjections and resolveProbes take the protocol as an argument so the
// runner never retains the caller's protocol slices after Build.
func (r *Runner) resolveInjections(ctx context.Context, g *connectome.Graph, protocol Protocol, channels int) error {
	for i, injection := range protocol.Injections {
		if injection.Channel < 0 || injection.Channel >= channels {
			return fmt.Errorf("simulate: injection %d uses channel %d, the stimulus has %d", i, injection.Channel, channels)
		}
		if injection.Node < 0 || injection.Node >= r.nodes {
			return fmt.Errorf("simulate: injection %d targets node %d, outside [0,%d)", i, injection.Node, r.nodes)
		}
		r.injections = append(r.injections, ResolvedInjection{
			Channel: injection.Channel, Gain: injection.Gain, NodeCount: 1,
			FirstNode: injection.Node, LastNode: injection.Node,
		})
		r.injectionNodes = append(r.injectionNodes, []int{injection.Node})
	}
	for i, group := range protocol.InjectionGroups {
		if group.Channel < 0 || group.Channel >= channels {
			return fmt.Errorf("simulate: injection group %d uses channel %d, the stimulus has %d", i, group.Channel, channels)
		}
		nodes, resolution, err := resolveSelector(ctx, g, *group.Selector)
		if err != nil {
			return err
		}
		record := ResolvedInjection{
			Channel: group.Channel, Gain: group.Gain, NodeCount: len(nodes),
			FirstNode: resolution.FirstIndex, LastNode: resolution.LastIndex,
			Selector: &SelectorResolution{},
		}
		*record.Selector = resolution
		r.injections = append(r.injections, record)
		r.injectionNodes = append(r.injectionNodes, nodes)
	}
	return nil
}

func (r *Runner) resolveProbes(ctx context.Context, g *connectome.Graph, protocol Protocol) error {
	for _, probe := range protocol.Probes {
		resolved := resolvedProbe{name: probe.Name, reduce: probe.Reduce}
		if probe.Selector != nil {
			nodes, resolution, err := resolveSelector(ctx, g, *probe.Selector)
			if err != nil {
				return err
			}
			resolved.nodes = nodes
			resolved.selector = &SelectorResolution{}
			*resolved.selector = resolution
		} else {
			nodes, err := checkNodes(probe.Nodes, r.nodes, "probe "+probe.Name)
			if err != nil {
				return err
			}
			resolved.nodes = nodes
		}
		resolved.scaleByOne = probe.Reduce == ReduceMeanOutput || probe.Reduce == ReduceSpikeFraction
		r.probes = append(r.probes, resolved)
	}
	return nil
}

// Stimulus returns an independent copy of the stimulus the protocol declared.
func (r *Runner) Stimulus() [][]float64 {
	rows := make([][]float64, len(r.stimulus))
	for t, row := range r.stimulus {
		rows[t] = append([]float64(nil), row...)
	}
	return rows
}

// State returns an independent copy of the persistent neural state.
func (r *Runner) State() StateSnapshot {
	snapshot := StateSnapshot{SchemaVersion: StateSchemaVersion, Core: r.state.Core}
	if r.state.LIF != nil {
		state := *r.state.LIF
		state.Voltage = append([]float64(nil), state.Voltage...)
		state.Adaptation = append([]float64(nil), state.Adaptation...)
		state.Refractory = append([]int(nil), state.Refractory...)
		state.History = cloneRows(state.History)
		snapshot.LIF = &state
	}
	if r.state.Continuous != nil {
		state := *r.state.Continuous
		state.Voltage = append([]float64(nil), state.Voltage...)
		state.History = cloneRows(state.History)
		snapshot.Continuous = &state
	}
	return snapshot
}

// RestoreState replaces the persistent state after validating it through the
// core, which includes the configuration fingerprint. A rejected snapshot
// leaves the existing state untouched.
func (r *Runner) RestoreState(s StateSnapshot) error {
	if s.SchemaVersion != "" && s.SchemaVersion != StateSchemaVersion {
		return fmt.Errorf("simulate: unsupported state schema %q, want %q", s.SchemaVersion, StateSchemaVersion)
	}
	if err := r.core.validate(s); err != nil {
		return err
	}
	candidate := StateSnapshot{SchemaVersion: StateSchemaVersion, Core: s.Core}
	if s.LIF != nil {
		state := *s.LIF
		state.Voltage = append([]float64(nil), state.Voltage...)
		state.Adaptation = append([]float64(nil), state.Adaptation...)
		state.Refractory = append([]int(nil), state.Refractory...)
		state.History = cloneRows(state.History)
		candidate.LIF = &state
	}
	if s.Continuous != nil {
		state := *s.Continuous
		state.Voltage = append([]float64(nil), state.Voltage...)
		state.History = cloneRows(state.History)
		candidate.Continuous = &state
	}
	r.state = candidate
	return nil
}

// TrackSets fixes the node sets Run measures and the step windows it measures
// them over. It changes nothing about the probes, the parameters or the
// trajectory: tracking only counts, per tracked node, the spikes inside each
// window and the first step of the window at which the set spiked, plus the
// summed core output the continuous core needs. Windows are step ranges
// [start, end) inside one Run call and are validated against the step count
// the protocol declares. Calling it again replaces the previous request and
// discards the measurements of an earlier run.
func (r *Runner) TrackSets(sets []ResolvedSet, windows [][2]int) error {
	steps := len(r.stimulus)
	tracked := make([]trackedSet, 0, len(sets))
	seenSet := map[string]struct{}{}
	for i, set := range sets {
		if set.Name == "" {
			return fmt.Errorf("simulate: tracked set %d has no name", i)
		}
		if _, exists := seenSet[set.Name]; exists {
			return fmt.Errorf("simulate: duplicate tracked set %q", set.Name)
		}
		seenSet[set.Name] = struct{}{}
		nodes, err := checkNodes(set.nodes, r.nodes, "tracked set "+set.Name)
		if err != nil {
			return err
		}
		tracked = append(tracked, trackedSet{name: set.Name, nodes: nodes})
	}
	seenWindow := map[[2]int]struct{}{}
	for _, window := range windows {
		if err := checkWindow(window, steps, "tracked"); err != nil {
			return err
		}
		if _, exists := seenWindow[window]; exists {
			return fmt.Errorf("simulate: duplicate tracked window %v", window)
		}
		seenWindow[window] = struct{}{}
	}
	if err := r.accountTracking(tracked, len(windows)); err != nil {
		return err
	}
	r.tracked = tracked
	r.trackWindows = append([][2]int(nil), windows...)
	r.measurements = Measurements{}
	return nil
}

// accountTracking adds the per set, per window counters to the bytes the run
// already accounts for and refuses the request if the sum leaves the limit.
func (r *Runner) accountTracking(sets []trackedSet, windows int) error {
	total := r.accounted
	for _, set := range sets {
		// One spike counter per node and window, plus the fixed per window
		// record with its first spike step and output sum.
		counters, err := checkedProduct(int64(len(set.nodes)), 4, int64(windows))
		if err != nil {
			return err
		}
		fixed, err := checkedProduct(int64(windows), bytesPerTrackedWindow, 1)
		if err != nil {
			return err
		}
		if total > math.MaxInt64-counters || total+counters > math.MaxInt64-fixed {
			return fmt.Errorf("%w: accounted bytes overflow", ErrCapacity)
		}
		total += counters + fixed
	}
	if total > r.limits.MaxMemoryBytes {
		return fmt.Errorf("%w: tracking %d sets over %d windows raises the accounted bytes to %d, over the %d byte limit", ErrCapacity, len(sets), windows, total, r.limits.MaxMemoryBytes)
	}
	return nil
}

// bytesPerTrackedWindow is the fixed record one tracked set keeps per window.
const bytesPerTrackedWindow = 64

// Measurements returns an independent copy of what the most recent successful
// Run recorded for the tracked sets. It is empty before the first run and
// before any set is tracked. A failed Run leaves the previous measurements in
// place, exactly as it leaves the trajectory.
func (r *Runner) Measurements() Measurements {
	return Measurements{Windows: append([]WindowMeasurement(nil), r.measurements.Windows...)}
}

// tracker holds the counters of one Run while it is in progress.
type tracker struct {
	counts [][][]uint32
	first  [][]int
	sums   [][]float64
}

func (r *Runner) newTracker(steps int) (*tracker, error) {
	if len(r.tracked) == 0 {
		return nil, nil
	}
	for _, window := range r.trackWindows {
		if window[1] > steps {
			return nil, fmt.Errorf("simulate: tracked window %v leaves the %d steps of this run", window, steps)
		}
	}
	t := &tracker{
		counts: make([][][]uint32, len(r.tracked)),
		first:  make([][]int, len(r.tracked)),
		sums:   make([][]float64, len(r.tracked)),
	}
	for s, set := range r.tracked {
		t.counts[s] = make([][]uint32, len(r.trackWindows))
		t.first[s] = make([]int, len(r.trackWindows))
		t.sums[s] = make([]float64, len(r.trackWindows))
		for w := range r.trackWindows {
			t.counts[s][w] = make([]uint32, len(set.nodes))
			t.first[s][w] = -1
		}
	}
	return t, nil
}

// observe records one step of one chunk. step is the absolute step index
// inside this Run call, which is what a window range refers to.
func (r *Runner) observe(t *tracker, step int, outputs, spikes []float64) {
	for w, window := range r.trackWindows {
		if step < window[0] || step >= window[1] {
			continue
		}
		for s, set := range r.tracked {
			for i, node := range set.nodes {
				t.sums[s][w] += outputs[node]
				if spikes == nil || spikes[node] == 0 {
					continue
				}
				t.counts[s][w][i]++
				if t.first[s][w] < 0 {
					t.first[s][w] = step
				}
			}
		}
	}
}

// measurements turns the counters into the per set, per window records a
// metric is evaluated from.
func (r *Runner) trackedMeasurements(t *tracker) Measurements {
	if t == nil {
		return Measurements{}
	}
	windows := make([]WindowMeasurement, 0, len(r.tracked)*len(r.trackWindows))
	for s, set := range r.tracked {
		for w, window := range r.trackWindows {
			measurement := WindowMeasurement{
				Set: set.name, Window: window, Nodes: len(set.nodes),
				Steps: window[1] - window[0], Spiking: r.core.spiking(),
				FirstSpikeStep: t.first[s][w], OutputSum: t.sums[s][w],
			}
			for _, count := range t.counts[s][w] {
				if count > 0 {
					measurement.SpikedNodes++
				}
				measurement.Spikes += uint64(count)
			}
			windows = append(windows, measurement)
		}
	}
	return Measurements{Windows: windows}
}

// Run advances the persistent state over the supplied stimulus and returns the
// report of exactly those steps. The state is committed only after the whole
// sequence and the whole report succeed, so cancellation, a non-finite value or
// any other failure leaves the trajectory where it was.
func (r *Runner) Run(ctx context.Context, stimulus [][]float64) (RunReport, error) {
	var empty RunReport
	if ctx == nil {
		return empty, errors.New("simulate: nil context")
	}
	if err := ctx.Err(); err != nil {
		return empty, fmt.Errorf("simulate: %w", err)
	}
	if len(stimulus) == 0 {
		return empty, errors.New("simulate: the stimulus has no step")
	}
	for t, row := range stimulus {
		if len(row) != r.channels {
			return empty, fmt.Errorf("simulate: stimulus[%d] has %d channels, the protocol declares %d", t, len(row), r.channels)
		}
		if err := checkFinite(row, fmt.Sprintf("stimulus[%d]", t)); err != nil {
			return empty, err
		}
	}
	steps := len(stimulus)
	series := make([][]float64, len(r.probes))
	for i := range series {
		series[i] = make([]float64, 0, steps)
	}
	spikeCounts := make([]uint64, r.nodes)
	populationRate := make([]float64, 0, steps)
	tracking, err := r.newTracker(steps)
	if err != nil {
		return empty, err
	}
	baseline := append([]float64(nil), r.core.baseline(r.state)...)
	changed := make([]bool, r.nodes)

	state := r.state
	fast := clonePlasticState(r.plasticState)
	var plastic *PlasticityReport
	if r.plastic != nil {
		plastic = &PlasticityReport{
			Rule: r.plasticRule.Kind, EnabledEdges: r.plastic.Edges(), GateChannel: r.plasticGate,
			Frozen: r.plasticFrozen, PlasticL2Before: l2Norm(fast.Plastic),
			ChunkSize: r.maxChunk, WallClockPenaltyNote: wallClockPenaltyNote,
		}
	}
	for start := 0; start < steps; start += r.maxChunk {
		if err := ctx.Err(); err != nil {
			return empty, fmt.Errorf("simulate: %w", err)
		}
		end := min(start+r.maxChunk, steps)
		inputs, err := r.inputs(stimulus[start:end])
		if err != nil {
			return empty, err
		}
		parameters := r.params
		if r.plastic != nil {
			weights, held, err := r.plastic.Effective(r.params.Weights, r.params.Signs, fast)
			if err != nil {
				return empty, fmt.Errorf("simulate: effective weights at step %d: %w", start, err)
			}
			plastic.ClampedByWMin += uint64(held.HeldAtWMin)
			parameters.Weights = weights
		}
		next, outputs, spikes, err := r.core.advance(ctx, parameters, state, inputs)
		if err != nil {
			return empty, fmt.Errorf("simulate: advance steps %d..%d: %w", start, end, err)
		}
		if r.interventions != nil {
			if err := r.applyInterventions(outputs, spikes, next, start); err != nil {
				return empty, fmt.Errorf("simulate: interventions at step %d: %w", start, err)
			}
		}
		if r.plastic != nil && !r.plasticFrozen {
			updated, stepReport, err := r.plasticStep(fast, outputs[0], spikeRow(spikes, 0), stimulus[start])
			if err != nil {
				return empty, fmt.Errorf("simulate: plasticity at step %d: %w", start, err)
			}
			fast = updated
			plastic.ClampedByPlasticMax += uint64(stepReport.Clamped)
		}
		for t := range outputs {
			if tracking != nil {
				r.observe(tracking, start+t, outputs[t], spikeRow(spikes, t))
			}
			for i, probe := range r.probes {
				value, err := reduce(probe, outputs[t], spikeRow(spikes, t))
				if err != nil {
					return empty, err
				}
				series[i] = append(series[i], value)
			}
			if r.core.spiking() {
				count := 0
				for node, spike := range spikes[t] {
					if spike != 0 {
						spikeCounts[node]++
						count++
					}
				}
				populationRate = append(populationRate, float64(count)/float64(r.nodes))
			} else {
				for node, value := range outputs[t] {
					if value != baseline[node] {
						changed[node] = true
					}
				}
			}
		}
		state = next
	}
	if err := ctx.Err(); err != nil {
		return empty, fmt.Errorf("simulate: %w", err)
	}
	report, err := r.report(state, steps, series, spikeCounts, populationRate, changed)
	if err != nil {
		return empty, err
	}
	if plastic != nil {
		plastic.PlasticL2After = l2Norm(fast.Plastic)
		// A norm that left the finite range would make the report
		// unencodable; refuse the run instead, exactly as a non-finite probe
		// value does. It needs a plastic_max above 1e150 on a whole-brain
		// graph, but the report must never be the place that finds out.
		if err := checkFinite([]float64{plastic.PlasticL2Before, plastic.PlasticL2After}, "plastic l2"); err != nil {
			return empty, err
		}
		report.Plasticity = plastic
	}
	if r.interventions != nil {
		report.Interventions = r.interventionReport(steps)
	}
	r.state = state
	r.plasticState = fast
	r.measurements = r.trackedMeasurements(tracking)
	return report, nil
}

// plasticStep updates the fast state from one core step, in the order both the
// plasticity package and learning.Individual.AdvanceGated fix: the core has
// already integrated the weights this step was given, the eligibility and the
// pair traces then take the pre and post signals of that step, and the gate
// turns the eligibility into a bounded fast change the next step integrates.
//
// The pre signal is the value the edge carries after this step: the activated
// output on the continuous core and the synaptic trace on the spiking one. The
// post signal is the 0/1 event where the core produces one and the same output
// where it does not.
func (r *Runner) plasticStep(fast plasticity.State, outputs, spikes, stimulus []float64) (plasticity.State, plasticity.Report, error) {
	pre, post := outputs, outputs
	var eventsPre, eventsPost []float64
	if spikes != nil {
		post, eventsPre, eventsPost = spikes, spikes, spikes
	}
	gate := r.plasticScale * stimulus[r.plasticGate]
	return r.plastic.Step(fast, pre, post, eventsPre, eventsPost, gate, r.plasticSources, r.plasticTargets)
}

// inputs turns one stimulus chunk into the per-step node input matrix.
func (r *Runner) inputs(stimulus [][]float64) ([][]float64, error) {
	rows := make([][]float64, len(stimulus))
	for t, row := range stimulus {
		input := make([]float64, r.nodes)
		for i, injection := range r.injections {
			drive := injection.Gain * row[injection.Channel]
			for _, node := range r.injectionNodes[i] {
				input[node] += drive
			}
		}
		if err := checkFinite(input, fmt.Sprintf("injected input at step %d", t)); err != nil {
			return nil, err
		}
		rows[t] = input
	}
	return rows, nil
}

// spikeRow returns the events of one step, or nil for a core without events.
func spikeRow(spikes [][]float64, t int) []float64 {
	if spikes == nil {
		return nil
	}
	return spikes[t]
}

// reduce applies one probe's declared reduction to a single step.
func reduce(probe resolvedProbe, outputs, spikes []float64) (float64, error) {
	if len(probe.nodes) == 0 {
		// An allow_empty probe reports zero rather than an undefined mean.
		return 0, nil
	}
	sum := 0.0
	switch probe.reduce {
	case ReduceMeanOutput, ReduceSumOutput:
		for _, node := range probe.nodes {
			sum += outputs[node]
		}
	case ReduceSpikeCount, ReduceSpikeFraction:
		if spikes == nil {
			return 0, fmt.Errorf("simulate: probe %q needs events from a spiking core", probe.name)
		}
		for _, node := range probe.nodes {
			if spikes[node] != 0 {
				sum++
			}
		}
	default:
		return 0, fmt.Errorf("simulate: probe %q has unsupported reduction %q", probe.name, probe.reduce)
	}
	if probe.scaleByOne {
		sum /= float64(len(probe.nodes))
	}
	if !finite(sum) {
		return 0, fmt.Errorf("simulate: probe %q produced a non-finite value", probe.name)
	}
	return sum, nil
}

func (r *Runner) report(state StateSnapshot, steps int, series [][]float64, spikeCounts []uint64, populationRate []float64, changed []bool) (RunReport, error) {
	var empty RunReport
	monitors := Monitors{PopulationRatePerStep: populationRate}
	if monitors.PopulationRatePerStep == nil {
		monitors.PopulationRatePerStep = []float64{}
	}
	silent := 0
	if r.core.spiking() {
		rates := make([]float64, r.nodes)
		for node, count := range spikeCounts {
			if count == 0 {
				silent++
			}
			rates[node] = float64(count) / float64(steps)
		}
		monitors.RateQuantiles = quantiles(rates)
	} else {
		for _, moved := range changed {
			if !moved {
				silent++
			}
		}
		monitors.PopulationRatePerStep = []float64{}
	}
	monitors.SilentFraction = float64(silent) / float64(r.nodes)
	if err := checkFinite(monitors.PopulationRatePerStep, "population rate"); err != nil {
		return empty, err
	}
	probes := make([]ProbeResult, len(r.probes))
	for i, probe := range r.probes {
		if err := checkFinite(series[i], "probe "+probe.name); err != nil {
			return empty, err
		}
		probes[i] = ProbeResult{
			Name: probe.name, Reduce: probe.reduce, NodeCount: len(probe.nodes),
			Selector: cloneResolution(probe.selector), Series: series[i],
		}
	}
	// A report is an independent value: mutating it must not reach the runner.
	injections := make([]ResolvedInjection, len(r.injections))
	for i, injection := range r.injections {
		injections[i] = injection
		injections[i].Selector = cloneResolution(injection.Selector)
	}
	flags := []string{}
	if r.stabilityBounds.MaxPopulationRate > 0 {
		for _, rate := range monitors.PopulationRatePerStep {
			if rate > r.stabilityBounds.MaxPopulationRate {
				flags = append(flags, FlagMaxPopulationRateExceeded)
				break
			}
		}
	}
	if r.stabilityBounds.MinActiveFraction > 0 && 1-monitors.SilentFraction < r.stabilityBounds.MinActiveFraction {
		flags = append(flags, FlagMinActiveFractionBelow)
	}
	report := RunReport{
		SchemaVersion:   RunReportSchemaVersion,
		Core:            r.core.name(),
		CoreConfigHash:  r.coreConfigHash,
		GraphHashes:     r.graphHashes,
		TopologyHash:    r.topologyHash,
		ParameterSource: r.params.Source,
		ParameterHash:   r.params.Hash,
		ProtocolHash:    r.protocolHash,
		Steps:           steps,
		StepsBefore:     stepCount(r.state),
		StepsAfter:      stepCount(state),
		Nodes:           r.nodes,
		Edges:           r.edges,
		Injections:      injections,
		Probes:          probes,
		Monitors:        monitors,
		StabilityFlags:  flags,
		Assumptions:     r.assumptions(),
	}
	if r.null != nil {
		report.NullModel = &NullModelReport{}
		*report.NullModel = *r.null
	}
	if derived := r.params.Derived; derived != nil {
		unknown := derived.UnknownSignEdges
		report.ParameterSetSHA256 = derived.ParameterSetSHA256
		report.RulesHash = derived.RulesHash
		report.UnknownSignPolicy = derived.UnknownSignPolicy
		report.UnknownSignEdges = &unknown
		report.WeightScale = derived.WeightScale
	}
	return report, nil
}

func (r *Runner) assumptions() []string {
	var lines []string
	if derived := r.params.Derived; derived != nil {
		lines = append(lines,
			"The parameter source "+ParameterSourceDerived+" takes every edge sign and strength from a parameter set derived from the official release under the rules with hash "+derived.RulesHash+", read from the file with SHA-256 "+derived.ParameterSetSHA256+". The signs are rule-derived from predicted per-T-bar transmitter probabilities, not measured synaptic actions, and the rule that maps a transmitter to a sign is an explicit assumption recorded in that file's derivation report.",
			fmt.Sprintf("Edges whose sign the rules left unknown were handled by the declared policy %q: %d of %d edges were unknown (%d positive, %d negative), and under this policy an unknown edge contributes %s. No sign was guessed and no default was substituted.",
				derived.UnknownSignPolicy, derived.UnknownSignEdges, r.edges, derived.PositiveEdges, derived.NegativeEdges, unknownSignEffect(derived.UnknownSignPolicy)),
			fmt.Sprintf("Every derived strength was multiplied by the declared weight_scale %v, which is an engineering choice of units and not a measured synaptic conductance.", derived.WeightScale),
			"bias, log_tau and theta_raw are still uniform engineering values from the protocol: the release carries no per-neuron time constant or threshold and none was derived here.",
		)
	} else {
		lines = append(lines, "The parameter source "+ParameterSourceUniform+" is an explicit engineering assumption and is not a biological parameter set: every edge weight is gain times its raw source weight, every connection is therefore excitatory, and every neuron shares one bias, log_tau and theta_raw. No sign, transmitter, time constant or threshold was derived from the release.")
	}
	lines = append(lines,
		"Every edge delay is zero. The release carries no conduction delay and none was invented.",
		"Node indices, edge order and raw weights come from the graph store; the run changes nothing about the wiring.",
	)
	if r.plastic != nil {
		lines = append(lines, r.plasticAssumption())
	}
	if r.interventions != nil {
		lines = append(lines, "Interventions were applied as declared; ordinary runs never clamp state.")
	}
	if !r.core.spiking() {
		lines = append(lines, "The continuous core emits no events, so population_rate_per_step is empty, rate_quantiles are zero and silent_fraction counts neurons whose output never left the value held before the run.")
	}
	return lines
}

// unknownSignEffect states in words what one unknown edge contributed.
func unknownSignEffect(policy string) string {
	switch policy {
	case UnknownSignExcitatory:
		return "the positive magnitude of its derived strength"
	case UnknownSignInhibitory:
		return "the negative magnitude of its derived strength"
	default:
		return "weight zero, so it was excluded from the run"
	}
}

func stepCount(s StateSnapshot) uint64 {
	if s.LIF != nil {
		return s.LIF.Steps
	}
	if s.Continuous != nil {
		return s.Continuous.Steps
	}
	return 0
}

// account refuses a run whose declared arrays exceed the limit. Every product
// is checked for overflow before it is added.
func account(limits Limits, nodes, edges, steps, channels, probes int) (int64, error) {
	chunk := min(maxSteps(nodes), steps)
	terms := [][3]int64{
		{int64(edges), bytesPerEdgeHere, 1},
		{int64(edges), bytesPerEdgeCore, 1},
		{int64(nodes), bytesPerNode, 1},
		{int64(steps), int64(channels) * 8, 1},
		{int64(chunk), int64(nodes) * 8, 3},
		{int64(steps), 8, int64(probes) + 1},
		{int64(nodes), 8, 1},
	}
	total := int64(0)
	for _, term := range terms {
		product, err := checkedProduct(term[0], term[1], term[2])
		if err != nil {
			return 0, err
		}
		if total > math.MaxInt64-product {
			return 0, fmt.Errorf("%w: accounted bytes overflow", ErrCapacity)
		}
		total += product
	}
	if total > limits.MaxMemoryBytes {
		return 0, fmt.Errorf("%w: the run accounts %d bytes for %d nodes, %d edges and %d steps, over the %d byte limit", ErrCapacity, total, nodes, edges, steps, limits.MaxMemoryBytes)
	}
	return total, nil
}

func checkedProduct(values ...int64) (int64, error) {
	product := int64(1)
	for _, value := range values {
		if value < 0 {
			return 0, fmt.Errorf("%w: negative accounted factor %d", ErrCapacity, value)
		}
		if value != 0 && product > math.MaxInt64/value {
			return 0, fmt.Errorf("%w: accounted bytes overflow", ErrCapacity)
		}
		product *= value
	}
	return product, nil
}

func cloneResolution(r *SelectorResolution) *SelectorResolution {
	if r == nil {
		return nil
	}
	clone := *r
	return &clone
}

func cloneRows(rows [][]float64) [][]float64 {
	clone := make([][]float64, len(rows))
	for i, row := range rows {
		clone[i] = append([]float64(nil), row...)
	}
	return clone
}
