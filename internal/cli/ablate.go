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

// ablationGroupOrder is the fixed default order of the five MOD-10 control
// groups when --groups is all.
var ablationGroupOrder = []string{
	experiment.GroupNoModulation,
	experiment.GroupDirectReward,
	experiment.GroupFixedDecay,
	experiment.GroupTrainableController,
	experiment.GroupCapacityMatched,
}

// runAblate runs the MOD-10 ablation over the declared seeds, groups, training
// and evaluation budgets and writes one indented JSON report per group plus a
// summary under DIR/ablation.
func runAblate(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	var outDir, seedsList, groupsList string
	var episodes, evalEpisodes int
	fs := flag.NewFlagSet("examples run ablate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&outDir, "out-dir", "", "directory to hold the ablation reports under its ablation/ subdirectory; an existing ablation/ directory is never reused (required)")
	fs.StringVar(&seedsList, "seeds", continualMatrixSeeds, "comma-separated deterministic seeds, at least three, no duplicates")
	fs.IntVar(&episodes, "episodes", 40, "training episodes per seed (1..100000)")
	fs.IntVar(&evalEpisodes, "eval", 32, "evaluation episodes per seed (1..10000)")
	fs.StringVar(&groupsList, "groups", "all", "groups to run: all or a comma-separated subset that includes no_modulation")
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: coimnet examples run ablate --out-dir DIR [--seeds 7,42,123] [--episodes 40] [--eval 32] [--groups all|no_modulation,direct_reward,...]")
		fmt.Fprintln(usageOutput, "Runs the MOD-10 modulation ablation: every declared control group trains on the shared delayed-pulse split over every seed and is scored on the same held-out split. Reports are written as indented JSON under DIR/ablation: one <group>.json per group carrying its per-seed runs and aggregate, plus summary.json carrying the whole report, the cross-group activity deltas and the protocol metadata. A one-line summary goes to stdout.")
		fmt.Fprintln(usageOutput, "The default five groups run in the fixed order no_modulation, direct_reward, fixed_decay, trainable_controller, capacity_matched; a custom --groups list must include no_modulation as the activity reference.")
		fmt.Fprintln(usageOutput, "Example: coimnet examples run ablate --out-dir results")
		fmt.Fprintln(usageOutput, "Errors: a missing --out-dir, an existing DIR/ablation directory, a missing or non-directory --out-dir parent, fewer than three seeds or duplicate seeds, an unparsable seed, episodes outside 1..100000, eval outside 1..10000, an unknown --groups name or a list without no_modulation, any positional argument, cancellation or output failure. Usage errors exit with status 1 and name the flag.")
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
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("examples run ablate takes no positional arguments; use examples run ablate --help")}
	}
	if outDir == "" {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--out-dir is required; use examples run ablate --help")}
	}
	if err := refuseAblationDir(outDir); err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	if episodes < 1 || episodes > 100000 {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--episodes must be between 1 and 100000")}
	}
	if evalEpisodes < 1 || evalEpisodes > 10000 {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--eval must be between 1 and 10000")}
	}
	seeds, err := parseContinualSeeds(seedsList)
	if err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	groups, err := parseAblationGroups(groupsList)
	if err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	report, err := experiment.RunAblation(ctx, experiment.AblationConfig{
		Seeds:        seeds,
		Episodes:     episodes,
		EvalEpisodes: evalEpisodes,
		Groups:       groups,
	})
	if err != nil {
		return err
	}
	ablationDir := filepath.Join(outDir, "ablation")
	if err := os.Mkdir(ablationDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", ablationDir, err)
	}
	for _, group := range report.Groups {
		if err := writeAblationJSON(ctx, filepath.Join(ablationDir, group.Group+".json"), group); err != nil {
			return err
		}
	}
	if err := writeAblationJSON(ctx, filepath.Join(ablationDir, "summary.json"), report); err != nil {
		return err
	}
	failed := 0
	for _, group := range report.Groups {
		for _, run := range group.Runs {
			if run.Failed {
				failed++
			}
		}
	}
	_, writeErr := fmt.Fprintf(stdout, "ablate: %d groups x %d seeds, %d failed runs, reports in %s\n",
		len(report.Groups), len(seeds), failed, ablationDir)
	return writeErr
}

// refuseAblationDir stops an ablation before any work starts when DIR already
// contains an ablation/ directory and confirms the out-dir parent exists and is
// a directory.
func refuseAblationDir(dir string) error {
	ablationDir := filepath.Join(dir, "ablation")
	if _, err := os.Lstat(ablationDir); err == nil {
		return fmt.Errorf("--out-dir %q ablation directory already exists; choose a new --out-dir", dir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("--out-dir path: %w", err)
	}
	if info, err := os.Stat(dir); err != nil {
		return fmt.Errorf("--out-dir: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("--out-dir %q is not a directory", dir)
	}
	return nil
}

// parseAblationGroups expands --groups: the literal value "all" is replaced by
// the five control groups in their fixed order; any other value is a comma-
// separated subset of known groups that must still include no_modulation as the
// activity reference.
func parseAblationGroups(list string) ([]string, error) {
	if list == "all" {
		return append([]string(nil), ablationGroupOrder...), nil
	}
	known := map[string]bool{}
	for _, group := range ablationGroupOrder {
		known[group] = true
	}
	var groups []string
	declared := map[string]bool{}
	for _, name := range strings.Split(list, ",") {
		if !known[name] {
			return nil, fmt.Errorf("unknown --groups name %q: use all or one of no_modulation,direct_reward,fixed_decay,trainable_controller,capacity_matched", name)
		}
		if declared[name] {
			return nil, fmt.Errorf("--groups contains duplicate %q", name)
		}
		declared[name] = true
		groups = append(groups, name)
	}
	if !declared[experiment.GroupNoModulation] {
		return nil, fmt.Errorf("--groups must include %q as the activity reference", experiment.GroupNoModulation)
	}
	return groups, nil
}

// writeAblationJSON publishes one ablation report as indented JSON to a new
// file. The temporary file is created next to the destination and renamed into
// place, so a canceled or failed write leaves the destination untouched.
func writeAblationJSON(ctx context.Context, path string, v any) (retErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode ablation report: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create ablation report temporary file: %w", err)
	}
	tempPath := temp.Name()
	removeTemp := true
	tempClosed := false
	defer func() {
		if !tempClosed {
			tempClosed = true
			if closeErr := temp.Close(); closeErr != nil {
				retErr = errors.Join(retErr, fmt.Errorf("close ablation report temporary file: %w", closeErr))
			}
		}
		if removeTemp {
			if removeErr := os.Remove(tempPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				retErr = errors.Join(retErr, fmt.Errorf("remove ablation report temporary file: %w", removeErr))
			}
		}
	}()
	if _, err := temp.Write(data); err != nil {
		return fmt.Errorf("write ablation report temporary file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync ablation report temporary file: %w", err)
	}
	if err := temp.Close(); err != nil {
		tempClosed = true
		return fmt.Errorf("close ablation report temporary file: %w", err)
	}
	tempClosed = true
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("publish ablation report %q: %w", path, err)
	}
	removeTemp = false
	return retErr
}
