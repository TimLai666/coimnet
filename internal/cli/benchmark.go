package cli

import (
	"context"
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
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/plasticity"
)

// The fixed draft of the report this command publishes. The schema version and
// the assumption list are part of OPS-02's output contract; they pin what stage
// this benchmark actually measured so a reader does not mistake it for a run on
// the MaleCNS graph.
const (
	benchmarkSchemaVersion = "coimnet-benchmark/v1"
	benchmarkSeed1         = 1
	benchmarkSeed2         = 0
)

type benchmarkConfig struct {
	Nodes  int `json:"nodes"`
	Edges  int `json:"edges"`
	Steps  int `json:"steps"`
	Repeat int `json:"repeat"`
}

type stageResult struct {
	Name     string  `json:"name"`
	MedianMS float64 `json:"median_ms"`
	MinMS    float64 `json:"min_ms"`
	MaxMS    float64 `json:"max_ms"`
	Repeat   int     `json:"repeat"`
	RSSMiB   float64 `json:"rss_mib_after"`
}

type benchmarkReport struct {
	SchemaVersion string          `json:"schema_version"`
	GoVersion     string          `json:"go_version"`
	GOOS          string          `json:"goos"`
	GOARCH        string          `json:"goarch"`
	NumCPU        int             `json:"num_cpu"`
	Uptime        string          `json:"uptime"`
	Config        benchmarkConfig `json:"config"`
	Stages        []stageResult   `json:"stages"`
	Assumptions   []string        `json:"assumptions"`
}

// runBenchmark measures import, forward, backward, local plasticity, modulation
// and snapshot on a synthetic continuous topology and writes one benchmark
// report to a new --out file. The report goes only to the file; stdout carries a
// one-line summary.
func runBenchmark(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	var out string
	nodes, edges, steps, repeat := 64, 256, 200, 3
	fs := flag.NewFlagSet("benchmark", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&out, "out", "", "path of the new benchmark report JSON; an existing path is refused (required)")
	fs.IntVar(&nodes, "nodes", 64, "synthetic node count")
	fs.IntVar(&edges, "edges", 256, "synthetic edge count")
	fs.IntVar(&steps, "steps", 200, "rows per stage")
	fs.IntVar(&repeat, "repeat", 3, "runs per stage; the median is reported")
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: coimnet benchmark --out FILE [--nodes 64] [--edges 256] [--steps 200] [--repeat 3]")
		fmt.Fprintln(usageOutput, "Builds one synthetic continuous topology (seed 1, self loops allowed, weights in [-0.1, 0.1], bias 0, log tau log(2), dt 1, tanh) and times six stages on it: import builds the dynamics model from the config, forward runs the model over --steps rows of 0.1 input, backward takes one learning step on a trainer whose encoder reaches node 0 and whose readout reads node nodes-1, both Insyra-driven, local_plasticity walks a persistent individual of that same declaration over --steps rows of 0.1 input with hebbian_rate plasticity on the first 64 edges and the learning gate held open, modulation walks a persistent individual whose single chemical channel receives one unit every five steps and whose hypothesized receptor sits on node nodes-1, and snapshot takes that individual's snapshot, saves it to <--out>.snapshot.tmp.json, reads it back and removes the file. Each stage runs --repeat times and reports the median, min and max wall-clock milliseconds plus the RSS after it, from runtime.ReadMemStats.Sys.")
		fmt.Fprintln(usageOutput, "Writes the coimnet-benchmark/v1 report to --out, which must not already exist, and prints one benchmark summary line to stdout.")
		fmt.Fprintln(usageOutput, "Example: coimnet benchmark --out benchmark.json")
		fmt.Fprintln(usageOutput, "Errors: a missing or existing --out, non-positive --nodes, --edges, --steps or --repeat, cancellation, a failing stage or an output failure. Usage errors exit with status 1 and name the flag.")
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
	if out == "" {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--out is required; use benchmark --help")}
	}
	if nodes <= 0 || edges <= 0 || steps <= 0 || repeat <= 0 {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--nodes, --edges, --steps and --repeat must be positive")}
	}
	// Refuse an existing --out before running anything, so the measurements are
	// not wasted on a conflict that was knowable up front; the exclusive create
	// in writeNewJSON remains the authority.
	if _, err := os.Lstat(out); err == nil {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("benchmark --out %q already exists; pick a new path", out)}
	} else if !errors.Is(err, os.ErrNotExist) {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("stat benchmark output: %w", err)}
	}

	cfg := syntheticTopology(nodes, edges)
	params := syntheticParameters(cfg)
	initial := make([]float64, nodes)
	forwardInput := make([][]float64, steps)
	for i := range forwardInput {
		forwardInput[i] = make([]float64, nodes)
		for j := range forwardInput[i] {
			forwardInput[i][j] = 0.1
		}
	}
	stepInput := make([][]float64, steps)
	for i := range stepInput {
		stepInput[i] = []float64{0.1}
	}
	target := []float64{0.5}

	stages := make([]stageResult, 0, 6)
	importResult, err := benchmarkStage("import", repeat, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := dynamics.NewContinuous(cfg)
		return err
	})
	if err != nil {
		return err
	}
	stages = append(stages, importResult)
	// Building the model is what the import stage measures, so the forward
	// stage times Forward alone. Forward leaves the model unchanged, so one
	// model serves every repetition and each repetition does the same work.
	model, err := dynamics.NewContinuous(cfg)
	if err != nil {
		return err
	}
	forwardResult, err := benchmarkStage("forward", repeat, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := model.Forward(ctx, params, initial, forwardInput)
		return err
	})
	if err != nil {
		return err
	}
	stages = append(stages, forwardResult)
	backwardResult, err := benchmarkStage("backward", repeat, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		tr, err := learning.NewTrainer(learning.Config{
			Dynamics:     cfg,
			InputSize:    1,
			OutputSize:   1,
			ReadoutNodes: []int{nodes - 1},
			InputNodes:   []int{0},
		}, learning.Parameters{
			Core:     params,
			ThetaRaw: nil,
			Encoder:  []float64{0.1},
			Readout:  []float64{0.1},
		}, learning.DefaultOptions())
		if err != nil {
			return err
		}
		_, err = tr.Step(ctx, stepInput, target)
		return err
	})
	if err != nil {
		return err
	}
	stages = append(stages, backwardResult)

	// The persistent individual of the last three stages repeats the backward
	// stage's declaration, so every stage of the report walks the same synthetic
	// topology through the same encoder and readout.
	individualConfig := learning.Config{
		Dynamics:     cfg,
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: []int{nodes - 1},
		InputNodes:   []int{0},
	}
	individualParameters := learning.Parameters{
		Core:     params,
		ThetaRaw: nil,
		Encoder:  []float64{0.1},
		Readout:  []float64{0.1},
	}
	plasticEdges := make([]int, min(edges, 64))
	for i := range plasticEdges {
		plasticEdges[i] = i
	}
	openGate := make([]float64, steps)
	for i := range openGate {
		openGate[i] = 1
	}
	// A walk leaves persistent voltage and fast state behind, so each repetition
	// needs its own individual. Building them all before the stage keeps
	// construction out of the timed function, the same split the import and
	// forward stages use, so the stage times AdvanceGated alone.
	plasticIndividuals := make([]*learning.Individual, 0, repeat)
	for range repeat {
		individual, err := learning.NewIndividual(individualConfig, individualParameters, learning.DefaultOptions(), make([]float64, nodes))
		if err != nil {
			return err
		}
		if err := individual.EnablePlasticity(plasticity.Config{
			Rule:  plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: 0.5, DecayP: 0.5, PlasticMax: 1, WMin: 0.01},
			Edges: plasticEdges,
		}); err != nil {
			return err
		}
		plasticIndividuals = append(plasticIndividuals, individual)
	}
	plasticRepetition := 0
	plasticResult, err := benchmarkStage("local_plasticity", repeat, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		individual := plasticIndividuals[plasticRepetition]
		plasticRepetition++
		_, _, err := individual.AdvanceGated(ctx, stepInput, openGate)
		return err
	})
	if err != nil {
		return err
	}
	stages = append(stages, plasticResult)

	// The modulation stage is the same walk under a declared chemistry instead
	// of local plasticity, so the difference between the two rows is the
	// mechanism and not the topology or the input.
	chemicalIndividuals := make([]*learning.Individual, 0, repeat)
	for range repeat {
		individual, err := learning.NewIndividual(individualConfig, individualParameters, learning.DefaultOptions(), make([]float64, nodes))
		if err != nil {
			return err
		}
		if err := individual.EnableChemistry(benchmarkChemistry(nodes, steps)); err != nil {
			return err
		}
		chemicalIndividuals = append(chemicalIndividuals, individual)
	}
	chemicalRepetition := 0
	modulationResult, err := benchmarkStage("modulation", repeat, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		individual := chemicalIndividuals[chemicalRepetition]
		chemicalRepetition++
		_, err := individual.Advance(ctx, stepInput)
		return err
	})
	if err != nil {
		return err
	}
	stages = append(stages, modulationResult)

	// The snapshot stage round-trips the individual the modulation stage walked,
	// so it measures a state that carries chemistry rather than a fresh one. The
	// temporary file sits next to --out and every repetition removes it, failed
	// or not, so the run leaves only the report behind. The removal runs inside
	// the timed function because the next repetition needs the path free.
	snapshotPath := out + ".snapshot.tmp.json"
	snapshotSubject := chemicalIndividuals[len(chemicalIndividuals)-1]
	snapshotResult, err := benchmarkStage("snapshot", repeat, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		defer os.Remove(snapshotPath)
		if err := checkpoint.SaveIndividual(ctx, snapshotPath, snapshotSubject.Snapshot()); err != nil {
			return err
		}
		_, err := checkpoint.LoadIndividual(ctx, snapshotPath)
		return err
	})
	if err != nil {
		return err
	}
	stages = append(stages, snapshotResult)

	report := benchmarkReport{
		SchemaVersion: benchmarkSchemaVersion,
		GoVersion:     runtime.Version(),
		GOOS:          runtime.GOOS,
		GOARCH:        runtime.GOARCH,
		NumCPU:        runtime.NumCPU(),
		Uptime:        readUptime(),
		Config:        benchmarkConfig{Nodes: nodes, Edges: edges, Steps: steps, Repeat: repeat},
		Stages:        stages,
		Assumptions: []string{
			"synthetic topology, not the MaleCNS graph",
			"wall-clock medians on a shared machine; see uptime",
		},
	}
	if err := writeNewJSON(out, report); err != nil {
		return fmt.Errorf("benchmark --out %s: %w", out, err)
	}
	_, err = fmt.Fprintf(stdout, "benchmark: %d stages, nodes %d, edges %d, steps %d, repeat %d\n", len(stages), nodes, edges, steps, repeat)
	return err
}

// benchmarkStage times fn repeat times and summarizes the wall-clock durations
// with the median, min and max in milliseconds, then records the RSS after the
// stage as runtime.ReadMemStats.Sys rounded to 0.1 MiB. One shared timing path
// keeps later stages of the OPS-02 report comparable.
func benchmarkStage(name string, repeat int, fn func() error) (stageResult, error) {
	if repeat <= 0 {
		return stageResult{}, fmt.Errorf("repeat must be positive")
	}
	durations := make([]time.Duration, 0, repeat)
	for i := 0; i < repeat; i++ {
		start := time.Now()
		if err := fn(); err != nil {
			return stageResult{}, fmt.Errorf("benchmark stage %q repetition %d: %w", name, i+1, err)
		}
		durations = append(durations, time.Since(start))
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	min, max := durations[0], durations[0]
	for _, d := range durations {
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
	}
	return stageResult{
		Name:     name,
		MedianMS: medianMS(durations),
		MinMS:    ms(min),
		MaxMS:    ms(max),
		Repeat:   repeat,
		RSSMiB:   math.Round(float64(m.Sys)/1048576*10) / 10,
	}, nil
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
