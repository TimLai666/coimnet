package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/TimLai666/coimnet/experiment"
)

// continualMatrixSeeds is the deterministic seed triple every default run uses.
const continualMatrixSeeds = "7,42,123"

// runContinualMatrix builds the fixed two-task continual fixture, runs its
// declared stages and comparison over every seed and writes the report to a
// new file.
func runContinualMatrix(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	var outPath, seedsList string
	var budget uint64
	var episodes int
	var chemistry bool
	fs := flag.NewFlagSet("examples run continual-matrix", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&outPath, "out", "", "new continual matrix report JSON file; an existing path is never overwritten (required)")
	fs.StringVar(&seedsList, "seeds", continualMatrixSeeds, "comma-separated deterministic seeds, at least three, no duplicates")
	fs.Uint64Var(&budget, "budget", 20, "training episodes per train_task stage (0..100000)")
	fs.IntVar(&episodes, "episodes", 16, "evaluation episodes per cell (1..10000)")
	fs.BoolVar(&chemistry, "chemistry", false, "enable the chemistry layer and re-score every cell under a switched chemical state")
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: coimnet examples run continual-matrix --out FILE [--seeds 7,42,123] [--budget 20] [--episodes 16] [--chemistry]")
		fmt.Fprintln(usageOutput, "Builds the fixed two-task fixture, task A (delay 2, channel 0) and task B (delay 3, channel 1), on the three-neuron delayed-core individual and runs the continual stages train A, train B and rule-change A, where the rule change negates the gain of task A. Every seed trains its own individual; each stage is scored on both tasks with evaluation episodes the training never sees. A preregistered paired bootstrap compares every last-stage cell against the same seeds' independent re-initialized control. The report is written as indented JSON to a new --out file and a one-line summary goes to stdout.")
		fmt.Fprintln(usageOutput, "--chemistry adds the hypothesized octopamine layer, freezes it during evaluation and records every cell re-scored under a switched concentration.")
		fmt.Fprintln(usageOutput, "Example: coimnet examples run continual-matrix --chemistry --out continual-matrix.json")
		fmt.Fprintln(usageOutput, "Errors: missing --out, an existing --out path, fewer than three seeds or duplicate seeds, an unparsable seed, a budget outside 0..100000, episodes outside 1..10000, cancellation or output failure. Usage errors exit with status 1 and name the flag.")
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
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("examples run continual-matrix takes no positional arguments; use examples run continual-matrix --help")}
	}
	if outPath == "" {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--out is required; use examples run continual-matrix --help")}
	}
	if err := refuseExistingOut(outPath); err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	if budget > 100000 {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--budget must be between 0 and 100000")}
	}
	if episodes < 1 || episodes > 10000 {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--episodes must be between 1 and 10000")}
	}
	seeds, err := parseContinualSeeds(seedsList)
	if err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	p := continualMatrixProtocol(seeds, budget, episodes, chemistry)
	build := experiment.ContinualFixture
	if chemistry {
		build = experiment.ContinualFixtureWithChemistry
	}
	report, err := experiment.RunContinualMatrix(ctx, p, build)
	if err != nil {
		return err
	}
	if err := writeContinualReport(ctx, outPath, report); err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	failed := 0
	for _, rec := range report.Runs {
		if rec.Status == experiment.RunStatusFailed {
			failed++
		}
	}
	_, writeErr := fmt.Fprintf(stdout, "continual-matrix: %d seeds, %d stages x %d tasks, %d failed runs, report %s\n",
		len(seeds), len(p.Stages), len(p.Tasks), failed, outPath)
	return writeErr
}

// parseContinualSeeds parses the comma-separated --seeds list into unsigned
// integers, rejecting fewer than three seeds or a duplicate.
func parseContinualSeeds(list string) ([]uint64, error) {
	var seeds []uint64
	for _, tok := range strings.Split(list, ",") {
		v, err := strconv.ParseUint(tok, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid --seeds value %q", tok)
		}
		seeds = append(seeds, v)
	}
	if len(seeds) < 3 {
		return nil, fmt.Errorf("--seeds needs at least 3 seeds, got %d", len(seeds))
	}
	seen := make(map[uint64]bool, len(seeds))
	for _, s := range seeds {
		if seen[s] {
			return nil, fmt.Errorf("--seeds contains duplicate seed %d", s)
		}
		seen[s] = true
	}
	return seeds, nil
}

// continualMatrixProtocol is the fixed fixture protocol of this example: the
// two tasks and three stages every run shares, the evaluation fixed by the
// --chemistry flag and the preregistered paired bootstrap.
func continualMatrixProtocol(seeds []uint64, budget uint64, episodes int, chemistry bool) experiment.ContinualProtocol {
	var stateSwitch *experiment.StateSwitch
	if chemistry {
		stateSwitch = &experiment.StateSwitch{Concentration: [][]float64{{2}}}
	}
	return experiment.ContinualProtocol{
		Tasks: []experiment.TaskSpec{
			{Name: "A", Generator: experiment.GeneratorDelayedCorrelation, Params: map[string]float64{"delay": 2, "channel": 0}},
			{Name: "B", Generator: experiment.GeneratorDelayedCorrelation, Params: map[string]float64{"delay": 3, "channel": 1}},
		},
		Stages: []experiment.Stage{
			{Kind: experiment.StageTrainTask, Task: "A", Budget: budget},
			{Kind: experiment.StageTrainTask, Task: "B", Budget: budget},
			{Kind: experiment.StageRuleChange, Task: "A"},
		},
		Seeds: seeds,
		Evaluation: experiment.Evaluation{
			Episodes:       episodes,
			FixedChemistry: chemistry,
			StateSwitch:    stateSwitch,
			Metric:         experiment.MetricNegMSE,
		},
		Comparison: &experiment.PreRegistered{
			Method:    experiment.ComparisonPairedBootstrap,
			Interval:  0.9,
			Baseline:  experiment.BaselineIndependent,
			Resamples: 1000,
		},
	}
}

// writeContinualReport publishes one continual matrix report as indented JSON
// to a new file. The temporary file is created next to the destination and
// renamed into place, so a canceled or failed write leaves the --out path
// untouched.
func writeContinualReport(ctx context.Context, path string, report experiment.ContinualReport) (retErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode continual matrix report: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create --out temporary file: %w", err)
	}
	tempPath := temp.Name()
	removeTemp := true
	tempClosed := false
	defer func() {
		if !tempClosed {
			tempClosed = true
			if closeErr := temp.Close(); closeErr != nil {
				retErr = errors.Join(retErr, fmt.Errorf("close --out temporary file: %w", closeErr))
			}
		}
		if removeTemp {
			if removeErr := os.Remove(tempPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				retErr = errors.Join(retErr, fmt.Errorf("remove --out temporary file: %w", removeErr))
			}
		}
	}()
	if _, err := temp.Write(data); err != nil {
		return fmt.Errorf("write --out temporary file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync --out temporary file: %w", err)
	}
	if err := temp.Close(); err != nil {
		tempClosed = true
		return fmt.Errorf("close --out temporary file: %w", err)
	}
	tempClosed = true
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("publish --out %q: %w", path, err)
	}
	removeTemp = false
	return retErr
}
