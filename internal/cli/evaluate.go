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
	"strings"

	"github.com/TimLai666/coimnet/experiment"
)

// runEvaluate builds the deterministic adaptive fixture and runs one declared
// evaluation over it, writing the evaluation report to a new file.
func runEvaluate(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	var mode, outPath, resetList string
	var seed uint64
	fs := flag.NewFlagSet("examples run evaluate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&mode, "mode", "", "evaluation mode, fixed or adaptive (required)")
	fs.StringVar(&outPath, "out", "", "new evaluation report JSON file; an existing path is never overwritten (required)")
	fs.Uint64Var(&seed, "seed", 1, "deterministic item generator seed")
	fs.StringVar(&resetList, "reset", "", "comma-separated state reset before every item: neural,plastic,chemical (default none)")
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: coimnet examples run evaluate --mode fixed|adaptive --out FILE [--seed 1] [--reset neural,plastic,chemical]")
		fmt.Fprintln(usageOutput, "Builds the small delayed-correlation fixture: a 2-node continuous core with local plasticity on its edge and 8+8 deterministic items derived from --seed. Fixed mode scores the eight items without any feedback; adaptive mode first replays eight adaptation items with score feedback through the online learner, then scores the same eight items closed-gate. The evaluation report is written as indented JSON to a new --out file and a one-line summary goes to stdout.")
		fmt.Fprintln(usageOutput, "Example: coimnet examples run evaluate --mode adaptive --reset neural,plastic,chemical --out evaluation.json")
		fmt.Fprintln(usageOutput, "Errors: missing --mode or --out, unknown --mode, an existing --out path, an unknown --reset name, cancellation or output failure. Usage errors exit with status 1 and name the flag.")
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
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("examples run evaluate takes no positional arguments; use examples run evaluate --help")}
	}
	if mode != experiment.EvaluationModeFixed && mode != experiment.EvaluationModeAdaptive {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("unknown --mode %q: use fixed or adaptive", mode)}
	}
	if outPath == "" {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--out is required; use examples run evaluate --help")}
	}
	if err := refuseExistingOut(outPath); err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	reset, err := parseReset(resetList)
	if err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	ind, eval, err := experiment.AdaptiveFixture(seed, mode, reset)
	if err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	report, err := experiment.RunAdaptiveEvaluation(ctx, ind, eval)
	if err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	if err := writeEvaluateReport(ctx, outPath, report); err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	shuffle := "n/a"
	if report.Contamination.ShuffleInvariant != nil {
		if *report.Contamination.ShuffleInvariant {
			shuffle = "true"
		} else {
			shuffle = "false"
		}
	}
	_, writeErr := fmt.Fprintf(stdout, "evaluate: mode=%s scoring=%d parameters_unchanged=%t shuffle_invariant=%s out=%s\n",
		report.Mode, report.ScoringItems, report.Contamination.ParametersUnchanged, shuffle, outPath)
	return writeErr
}

// parseReset maps the comma-separated --reset names onto the reset policy.
// An empty value leaves every reset off; an unknown name is a usage error.
func parseReset(list string) (experiment.ResetPolicy, error) {
	var r experiment.ResetPolicy
	if list == "" {
		return r, nil
	}
	for _, name := range strings.Split(list, ",") {
		switch name {
		case "neural":
			r.NeuralAtItemStart = true
		case "plastic":
			r.PlasticAtItemStart = true
		case "chemical":
			r.ChemicalAtItemStart = true
		default:
			return r, fmt.Errorf("unknown --reset name %q: use one or more of neural,plastic,chemical", name)
		}
	}
	return r, nil
}

// refuseExistingOut stops an evaluation before any work starts when the
// declared --out path already exists.
func refuseExistingOut(path string) error {
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("--out %q already exists; choose a new path", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("--out path: %w", err)
	}
	dir := filepath.Dir(path)
	if info, err := os.Stat(dir); err != nil {
		return fmt.Errorf("--out directory: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("--out parent %q is not a directory", dir)
	}
	return nil
}

// writeEvaluateReport publishes one report as indented JSON to a new file. The
// temporary file is created next to the destination and renamed into place, so
// a canceled or failed write leaves the --out path untouched.
func writeEvaluateReport(ctx context.Context, path string, report experiment.EvaluationReport) (retErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode evaluation report: %w", err)
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
