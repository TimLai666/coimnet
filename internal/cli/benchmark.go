package cli

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"time"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/experiment/fullgraph"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/plasticity"
	"github.com/TimLai666/coimnet/resources"
)

// The fixed draft of the report this command publishes. The schema version and
// the assumption list are part of OPS-08's output contract; they pin what stage
// this benchmark actually measured so a reader does not mistake it for a run on
// the MaleCNS graph.
const (
	benchmarkSchemaVersion = "coimnet-benchmark/v2"
	benchmarkSeed1         = 1
	benchmarkSeed2         = 0
)

type benchmarkConfig struct {
	Source         string            `json:"source"`
	Nodes          int               `json:"nodes"`
	Edges          int               `json:"edges"`
	Steps          int               `json:"steps"`
	Repeat         int               `json:"repeat"`
	Files          map[string]string `json:"files,omitempty"`
	InputSet       string            `json:"input_set,omitempty"`
	ReadoutSet     string            `json:"readout_set,omitempty"`
	MaxMemoryMiB   int               `json:"max_memory_mib,omitempty"`
	SnapshotFormat string            `json:"snapshot_format"`
}

type stageResult struct {
	Name           string             `json:"name"`
	InitMS         float64            `json:"init_ms"`
	TransferMS     float64            `json:"transfer_ms"`
	WarmupMS       float64            `json:"warmup_ms"`
	SteadyMedianMS float64            `json:"steady_median_ms"`
	SteadyMinMS    float64            `json:"steady_min_ms"`
	SteadyMaxMS    float64            `json:"steady_max_ms"`
	SteadyRepeat   int                `json:"steady_repeat"`
	RSSMiB         float64            `json:"rss_mib_after"`
	Activity       *benchmarkActivity `json:"activity,omitempty"`
}

type benchmarkActivity struct {
	Rows            int      `json:"rows"`
	Nodes           int      `json:"nodes"`
	NonzeroFraction float64  `json:"nonzero_fraction"`
	MeanAbsOutput   float64  `json:"mean_abs_output"`
	SpikesPerStep   *float64 `json:"spikes_per_step"`
	MeanRate        *float64 `json:"mean_rate"`
	Digest          string   `json:"digest"`
}

type benchmarkReproducibility struct {
	ForwardDigestFirst  string  `json:"forward_digest_first"`
	ForwardDigestSecond string  `json:"forward_digest_second"`
	Identical           bool    `json:"identical"`
	SteadyRatio         float64 `json:"steady_ratio"`
}

type benchmarkEnergy struct {
	Measured bool   `json:"measured"`
	Note     string `json:"note"`
}

type benchmarkReport struct {
	SchemaVersion   string                   `json:"schema_version"`
	GoVersion       string                   `json:"go_version"`
	GOOS            string                   `json:"goos"`
	GOARCH          string                   `json:"goarch"`
	NumCPU          int                      `json:"num_cpu"`
	Uptime          string                   `json:"uptime"`
	Config          benchmarkConfig          `json:"config"`
	Stages          []stageResult            `json:"stages"`
	Assumptions     []string                 `json:"assumptions"`
	Reproducibility benchmarkReproducibility `json:"reproducibility"`
	Energy          benchmarkEnergy          `json:"energy"`
	Memory          *resources.Report        `json:"memory,omitempty"`
}

// runBenchmark measures import, forward, backward, local plasticity, modulation
// and snapshot on a synthetic continuous topology and writes one benchmark
// report to a new --out file. The report goes only to the file; stdout carries a
// one-line summary.
func runBenchmark(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	var out string
	defaults := fullgraph.DefaultOptions()
	nodes, edges, steps, repeat := 64, 256, 200, 3
	var store, paramsPath, protocolPath string
	inputSet, readoutSet := defaults.InputSet, defaults.ReadoutSet
	maxMemoryMiB := defaults.MaxMemoryMiB
	fs := flag.NewFlagSet("benchmark", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&out, "out", "", "path of the new benchmark report JSON; an existing path is refused (required)")
	fs.IntVar(&nodes, "nodes", 64, "synthetic node count")
	fs.IntVar(&edges, "edges", 256, "synthetic edge count")
	fs.IntVar(&steps, "steps", 200, "rows per stage")
	fs.IntVar(&repeat, "repeat", 3, "runs per stage including warmup; at least 2")
	fs.StringVar(&store, "store", "", "path of the connectome graph store; use with --params and --protocol")
	fs.StringVar(&paramsPath, "params", "", "path of the derived parameter set for --store")
	fs.StringVar(&protocolPath, "protocol", "", "path of the compare protocol for --store")
	fs.StringVar(&inputSet, "input-set", defaults.InputSet, "protocol input set name in store mode")
	fs.StringVar(&readoutSet, "readout-set", defaults.ReadoutSet, "protocol readout set name in store mode")
	fs.IntVar(&maxMemoryMiB, "max-memory-mib", defaults.MaxMemoryMiB, "store-mode memory estimate limit in MiB")
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: coimnet benchmark --out FILE [--nodes 64] [--edges 256] [--steps 200] [--repeat 3] [--store FILE --params FILE --protocol FILE]")
		fmt.Fprintln(usageOutput, "Builds and times six continuous-core stages on either a synthetic topology or a graph loaded from --store. Store mode requires --store, --params and --protocol together, resolves --input-set and --readout-set, checks the memory estimate before measuring, and snapshots to a temporary bundle. The backward encoder still reaches node 0 and the readout reads the last node; it does not use the protocol's named sets.")
		fmt.Fprintln(usageOutput, "Synthetic mode uses seed 1 with self loops allowed, weights in [-0.1, 0.1], bias 0, log tau log(2), dt 1 and tanh. Forward runs --steps rows of 0.1 input. Backward takes one Insyra-driven learning step; local_plasticity uses hebbian_rate on the first 64 edges with its gate open; modulation uses one chemical channel and a hypothesized receptor on the last node. Snapshot round-trips through JSON (synthetic) or a bundle (store). Each stage separates setup from work, with the first run as warmup and later runs summarized by steady median, min and max milliseconds. CPU transfer time is 0 ms. RSS uses runtime.ReadMemStats.Sys.")
		fmt.Fprintln(usageOutput, "Writes the coimnet-benchmark/v2 report to --out, which must not already exist, and prints one benchmark summary line to stdout. Forward activity and same-seed reproducibility are reported; energy is not measured.")
		fmt.Fprintln(usageOutput, "Example: coimnet benchmark --out benchmark.json")
		fmt.Fprintln(usageOutput, "Example: coimnet benchmark --store data/malecns-v1.0/graph-v1.coimgraph --params data/malecns-v1.0/params-derive-v1.coimparams --protocol evidence/NAT-05/compare-fullgraph-derived-continuous.json --steps 8 --repeat 2 --out benchmark-full.json")
		fmt.Fprintln(usageOutput, "Errors: a missing or existing --out, non-positive --nodes, --edges or --steps, --repeat below 2, partial --store/--params/--protocol, --nodes or --edges with --store, a memory estimate above --max-memory-mib, cancellation, a failing stage or an output failure. Usage errors exit with status 1 and name the flag.")
		fmt.Fprintln(usageOutput, "Options and their defaults:")
		fs.SetOutput(usageOutput)
		fs.PrintDefaults()
		fs.SetOutput(stderr)
	}
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return usageOutput.Err()
	} else if err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	if fs.NArg() != 0 {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("benchmark takes no positional arguments; use benchmark --help")}
	}
	provided := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) { provided[f.Name] = true })
	if out == "" {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--out is required; use benchmark --help")}
	}
	storeCount := 0
	for _, name := range []string{"store", "params", "protocol"} {
		if provided[name] {
			storeCount++
		}
	}
	if storeCount != 0 && storeCount != 3 {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--store, --params and --protocol must be provided together")}
	}
	storeMode := storeCount == 3
	if storeMode && (provided["nodes"] || provided["edges"]) {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--nodes and --edges come from --store")}
	}
	if steps <= 0 || (!storeMode && (nodes <= 0 || edges <= 0)) {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--nodes, --edges and --steps must be positive")}
	}
	if repeat < 2 {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--repeat must be at least 2 so warmup and steady each run at least once")}
	}
	// Refuse an existing --out before running anything, so the measurements are
	// not wasted on a conflict that was knowable up front; the exclusive create
	// in writeNewJSON remains the authority.
	if _, err := os.Lstat(out); err == nil {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("benchmark --out %q already exists; pick a new path", out)}
	} else if !errors.Is(err, os.ErrNotExist) {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("stat benchmark output: %w", err)}
	}

	source, snapshotFormat := "synthetic", "json"
	var loaded fullgraph.LoadedModel
	var memory *resources.Report
	var cfg dynamics.Config
	var params dynamics.Parameters
	var err error
	if storeMode {
		source, snapshotFormat = "store", "bundle"
		loaded, err = fullgraph.LoadModel(ctx, fullgraph.Options{
			Store: store, Params: paramsPath, Protocol: protocolPath, InputSet: inputSet, ReadoutSet: readoutSet,
			Truncation: defaults.Truncation, LearningRate: defaults.LearningRate, MaxMemoryMiB: maxMemoryMiB,
		})
		if err != nil {
			return fmt.Errorf("benchmark: load model with --max-memory-mib %d MiB: %w", maxMemoryMiB, err)
		}
		memReport, err := fullgraph.CheckModelMemory(loaded, steps, min(loaded.Edges, 64), 1, 1, maxMemoryMiB)
		if err != nil {
			return err
		}
		memory = &memReport
		cfg, params = loaded.Config.Dynamics, loaded.Parameters.Core
		nodes, edges = cfg.Nodes, len(cfg.Sources)
	} else {
		cfg = syntheticTopology(nodes, edges)
		params = syntheticParameters(cfg)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	stages := make([]stageResult, 0, 6)
	importResult, err := benchmarkStage("import", repeat, func() (func() error, error) {
		if storeMode {
			return func() error {
				if err := ctx.Err(); err != nil {
					return err
				}
				_, err := fullgraph.LoadModel(ctx, fullgraph.Options{
					Store: store, Params: paramsPath, Protocol: protocolPath, InputSet: inputSet, ReadoutSet: readoutSet,
					Truncation: defaults.Truncation, LearningRate: defaults.LearningRate, MaxMemoryMiB: maxMemoryMiB,
				})
				return err
			}, nil
		}
		synthetic := syntheticTopology(nodes, edges)
		return func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			_, err := dynamics.NewContinuous(synthetic)
			return err
		}, nil
	})
	if err != nil {
		return err
	}
	stages = append(stages, importResult)
	forwardResult, firstActivity, err := benchmarkForward(ctx, cfg, params, nodes, steps, repeat)
	if err != nil {
		return err
	}
	stages = append(stages, forwardResult)
	forwardSecond, secondActivity, err := benchmarkForward(ctx, cfg, params, nodes, steps, repeat)
	if err != nil {
		return err
	}
	identical := firstActivity.Digest == secondActivity.Digest
	reproducibility := benchmarkReproducibility{
		ForwardDigestFirst:  firstActivity.Digest,
		ForwardDigestSecond: secondActivity.Digest,
		Identical:           identical,
	}
	if forwardResult.SteadyMedianMS > 0 {
		reproducibility.SteadyRatio = forwardSecond.SteadyMedianMS / forwardResult.SteadyMedianMS
	}

	stepInput := benchmarkStepInput(steps)
	target := []float64{0.5}
	trainerConfig := learning.Config{
		Dynamics:     cfg,
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: []int{nodes - 1},
		InputNodes:   []int{0},
	}
	trainerParameters := learning.Parameters{Core: params, Encoder: []float64{0.1}, Readout: []float64{0.1}}
	backwardResult, err := benchmarkStage("backward", repeat, func() (func() error, error) {
		tr, err := learning.NewTrainer(trainerConfig, trainerParameters, learning.DefaultOptions())
		if err != nil {
			return nil, err
		}
		return func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			_, err := tr.Step(ctx, stepInput, target)
			return err
		}, nil
	})
	if err != nil {
		return err
	}
	stages = append(stages, backwardResult)

	// Each measured repetition gets a fresh individual during setup.
	plasticEdges := make([]int, min(edges, 64))
	for i := range plasticEdges {
		plasticEdges[i] = i
	}
	openGate := make([]float64, steps)
	for i := range openGate {
		openGate[i] = 1
	}
	plasticResult, err := benchmarkStage("local_plasticity", repeat, func() (func() error, error) {
		individual, err := newBenchmarkIndividual(cfg, params, nodes)
		if err != nil {
			return nil, err
		}
		if err := individual.EnablePlasticity(plasticity.Config{
			Rule:  plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: 0.5, DecayP: 0.5, PlasticMax: 1, WMin: 0.01},
			Edges: plasticEdges,
		}); err != nil {
			return nil, err
		}
		return func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			_, _, err := individual.AdvanceGated(ctx, stepInput, openGate)
			return err
		}, nil
	})
	if err != nil {
		return err
	}
	stages = append(stages, plasticResult)

	// The modulation stage is the same walk under a declared chemistry instead
	// of local plasticity, so the difference between the two rows is the
	// mechanism and not the topology or the input.
	modulationResult, err := benchmarkStage("modulation", repeat, func() (func() error, error) {
		individual, err := newBenchmarkIndividual(cfg, params, nodes)
		if err != nil {
			return nil, err
		}
		if err := individual.EnableChemistry(benchmarkChemistry(nodes, steps)); err != nil {
			return nil, err
		}
		return func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			_, err := individual.Advance(ctx, stepInput)
			return err
		}, nil
	})
	if err != nil {
		return err
	}
	stages = append(stages, modulationResult)

	// Each snapshot repetition prepares an individual with the modulation walk
	// during setup; only the snapshot round trip and temp-file cleanup are timed.
	snapshotPath := out + ".snapshot.tmp." + map[string]string{"json": "json", "bundle": "coimbundle"}[snapshotFormat]
	snapshotResult, err := benchmarkStage("snapshot", repeat, func() (func() error, error) {
		individual, err := newBenchmarkIndividual(cfg, params, nodes)
		if err != nil {
			return nil, err
		}
		if err := individual.EnableChemistry(benchmarkChemistry(nodes, steps)); err != nil {
			return nil, err
		}
		if _, err := individual.Advance(ctx, stepInput); err != nil {
			return nil, err
		}
		return func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if snapshotFormat == "json" {
				defer os.Remove(snapshotPath)
				if err := checkpoint.SaveIndividual(ctx, snapshotPath, individual.Snapshot()); err != nil {
					return err
				}
				_, err := checkpoint.LoadIndividual(ctx, snapshotPath)
				return err
			}
			return saveLoadBenchmarkBundle(ctx, snapshotPath, individual.Snapshot())
		}, nil
	})
	if err != nil {
		return err
	}
	stages = append(stages, snapshotResult)

	config := benchmarkConfig{Source: source, Nodes: nodes, Edges: edges, Steps: steps, Repeat: repeat, SnapshotFormat: snapshotFormat}
	if storeMode {
		config.Files = loaded.Files
		config.InputSet = inputSet
		config.ReadoutSet = readoutSet
		config.MaxMemoryMiB = maxMemoryMiB
	}
	report := benchmarkReport{
		SchemaVersion: benchmarkSchemaVersion,
		GoVersion:     runtime.Version(),
		GOOS:          runtime.GOOS,
		GOARCH:        runtime.GOARCH,
		NumCPU:        runtime.NumCPU(),
		Uptime:        readUptime(),
		Config:        config,
		Stages:        stages,
		Assumptions: []string{
			"synthetic topology, not the MaleCNS graph",
			"wall-clock medians on a shared machine; see uptime",
			"continuous core: spike counts and firing rates do not apply",
		},
		Reproducibility: reproducibility,
		Energy:          benchmarkEnergy{Measured: false, Note: "no power measurement"},
		Memory:          memory,
	}
	if storeMode {
		report.Assumptions = []string{
			"MaleCNS graph from --store with derived parameters from --params; the backward encoder reaches node 0 and the readout reads the last node, not the protocol's named sets",
			report.Assumptions[1],
			report.Assumptions[2],
		}
	}
	if err := writeNewJSON(out, report); err != nil {
		return fmt.Errorf("benchmark --out %s: %w", out, err)
	}
	_, err = fmt.Fprintf(stdout, "benchmark: %d stages, nodes %d, edges %d, steps %d, repeat %d, forward reproducible %t, source %s\n", len(stages), nodes, edges, steps, repeat, identical, source)
	return err
}

func saveLoadBenchmarkBundle(ctx context.Context, path string, snapshot learning.IndividualSnapshot) (result error) {
	defer func() {
		result = errors.Join(result, os.RemoveAll(path))
	}()
	if err := checkpoint.SaveIndividualBundle(ctx, path, snapshot); err != nil {
		return err
	}
	_, err := checkpoint.LoadIndividualBundle(ctx, path)
	return err
}

// benchmarkStage measures setup separately, treats the first work run as warmup,
// summarizes later work runs as steady state, and records RSS after the stage.
func benchmarkStage(name string, repeat int, setup func() (func() error, error)) (stageResult, error) {
	if repeat < 2 {
		return stageResult{}, fmt.Errorf("repeat must be at least 2 so warmup and steady each run at least once")
	}
	initDurations := make([]time.Duration, 0, repeat)
	workDurations := make([]time.Duration, 0, repeat)
	for i := 0; i < repeat; i++ {
		initStart := time.Now()
		work, err := setup()
		initDurations = append(initDurations, time.Since(initStart))
		if err != nil {
			return stageResult{}, fmt.Errorf("benchmark stage %q setup repetition %d: %w", name, i+1, err)
		}
		if work == nil {
			return stageResult{}, fmt.Errorf("benchmark stage %q setup repetition %d returned no work", name, i+1)
		}
		start := time.Now()
		if err := work(); err != nil {
			return stageResult{}, fmt.Errorf("benchmark stage %q repetition %d: %w", name, i+1, err)
		}
		workDurations = append(workDurations, time.Since(start))
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	steady := workDurations[1:]
	min, max := steady[0], steady[0]
	for _, d := range steady {
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
	}
	return stageResult{
		Name:           name,
		InitMS:         medianMS(initDurations),
		TransferMS:     0,
		WarmupMS:       ms(workDurations[0]),
		SteadyMedianMS: medianMS(steady),
		SteadyMinMS:    ms(min),
		SteadyMaxMS:    ms(max),
		SteadyRepeat:   repeat - 1,
		RSSMiB:         math.Round(float64(m.Sys)/1048576*10) / 10,
	}, nil
}

func benchmarkForward(ctx context.Context, cfg dynamics.Config, params dynamics.Parameters, nodes, steps, repeat int) (stageResult, benchmarkActivity, error) {
	var output [][]float64
	result, err := benchmarkStage("forward", repeat, func() (func() error, error) {
		model, err := dynamics.NewContinuous(cfg)
		if err != nil {
			return nil, err
		}
		initial := make([]float64, nodes)
		input := make([][]float64, steps)
		for i := range input {
			input[i] = make([]float64, nodes)
			for j := range input[i] {
				input[i][j] = 0.1
			}
		}
		return func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			trace, err := model.Forward(ctx, params, initial, input)
			if err != nil {
				return err
			}
			output = trace.Outputs()
			return nil
		}, nil
	})
	if err != nil {
		return stageResult{}, benchmarkActivity{}, err
	}
	activity := summarizeBenchmarkActivity(output, steps, nodes)
	result.Activity = &activity
	return result, activity, nil
}

func summarizeBenchmarkActivity(output [][]float64, rows, nodes int) benchmarkActivity {
	hash := sha256.New()
	var total, nonzero int
	var sumAbs float64
	var bits [8]byte
	for _, row := range output {
		for _, value := range row {
			binary.LittleEndian.PutUint64(bits[:], math.Float64bits(value))
			_, _ = hash.Write(bits[:])
			total++
			if math.Abs(value) > 1e-12 {
				nonzero++
			}
			sumAbs += math.Abs(value)
		}
	}
	return benchmarkActivity{
		Rows: rows, Nodes: nodes,
		NonzeroFraction: float64(nonzero) / float64(total),
		MeanAbsOutput:   sumAbs / float64(total),
		Digest:          hex.EncodeToString(hash.Sum(nil)),
	}
}

func benchmarkStepInput(steps int) [][]float64 {
	input := make([][]float64, steps)
	for i := range input {
		input[i] = []float64{0.1}
	}
	return input
}

func newBenchmarkIndividual(cfg dynamics.Config, params dynamics.Parameters, nodes int) (*learning.Individual, error) {
	return learning.NewIndividual(learning.Config{
		Dynamics: cfg, InputSize: 1, OutputSize: 1,
		ReadoutNodes: []int{nodes - 1}, InputNodes: []int{0},
	}, learning.Parameters{
		Core: params, Encoder: []float64{0.1}, Readout: []float64{0.1},
	}, learning.DefaultOptions(), make([]float64, nodes))
}

// syntheticTopology draws edge endpoints from rand.NewPCG(benchmarkSeed1,
// benchmarkSeed2), allowing self loops and duplicate pairs.
func syntheticTopology(nodes, edges int) dynamics.Config {
	rng := rand.New(rand.NewPCG(benchmarkSeed1, benchmarkSeed2))
	sources := make([]int, edges)
	targets := make([]int, edges)
	for e := range edges {
		sources[e] = int(rng.Uint64N(uint64(nodes)))
		targets[e] = int(rng.Uint64N(uint64(nodes)))
	}
	return dynamics.Config{Nodes: nodes, Sources: sources, Targets: targets, DT: 1, Activation: "tanh"}
}

// syntheticParameters fixes every weight in [-0.1, 0.1] from the same seeded
// generator, zero bias and log tau log(2), which keeps each column of the stage
// grid reproducible. It is a uniform engineering parameter set, not a
// biological one; the report's assumptions say so.
func syntheticParameters(cfg dynamics.Config) dynamics.Parameters {
	rng := rand.New(rand.NewPCG(benchmarkSeed1, benchmarkSeed2))
	weights := make([]float64, len(cfg.Sources))
	for i := range weights {
		weights[i] = -0.1 + rng.Float64()*0.2
	}
	bias := make([]float64, cfg.Nodes)
	logTau := make([]float64, cfg.Nodes)
	for i := range logTau {
		logTau[i] = math.Ln2
	}
	return dynamics.Parameters{Weights: weights, Bias: bias, LogTau: logTau}
}

// benchmarkChemistry declares the modulation stage's chemistry on the synthetic
// topology: one region holding every node, one channel cleared with a fixed time
// constant, an external timeline releasing one unit every five steps, and a
// hypothesized receptor on the readout node driving one sensitivity effect. It
// is a declared engineering stimulus, not measured biology, exactly like the
// uniform parameter set above.
func benchmarkChemistry(nodes, steps int) modulation.ChemistryConfig {
	entries := make([]modulation.TimelineEntry, steps)
	for e := range entries {
		entries[e] = modulation.TimelineEntry{Step: uint64(5 * e), Channel: 0, Rate: 1}
	}
	return modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{2}},
		Sources: []modulation.SourceSpec{{
			Kind: modulation.SourceExternalTimeline, Channel: 0,
			Timeline: &modulation.ExternalTimeline{ChannelCount: 1, Entries: entries},
		}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{{
			Cells: []int{nodes - 1}, Signal: "octopamine", Channel: 0,
			Status: modulation.StatusHypothesized, Kd: 0.5, N: 1,
			Evidence: "coimnet benchmark modulation stage", MeasurementKind: "declared", MappingVersion: benchmarkSchemaVersion,
		}}},
		Effects: []modulation.Effect{{Kind: modulation.EffectSensitivity, Receptor: 0, GammaScale: 1}},
		Regions: modulation.RegionAssignment{NodeRegion: make([]int, nodes)},
	}
}

func medianMS(durations []time.Duration) float64 {
	sorted := append([]time.Duration(nil), durations...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return ms(sorted[mid])
	}
	return (ms(sorted[mid-1]) + ms(sorted[mid])) / 2
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

func readUptime() string {
	out, err := exec.Command("uptime").Output()
	if err != nil {
		return "unavailable"
	}
	return string(out)
}
