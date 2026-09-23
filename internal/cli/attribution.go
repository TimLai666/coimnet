package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/TimLai666/coimnet/experiment"
)

func runAttribution(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	c := experiment.DefaultAttributionConfig()
	task := c.Env.Task
	seedsList := formatSeeds(c.Seeds)
	groupsList := "all"
	outPath := ""
	fs := flag.NewFlagSet("examples run attribution", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&task, "task", task, "navigation task: avoid_obstacles, remember_goal, adapt_after_change or language_goal")
	fs.StringVar(&seedsList, "seeds", seedsList, "comma-separated distinct seeds, at least three")
	fs.IntVar(&c.Episodes, "episodes", c.Episodes, "training episodes per seed (1..10000)")
	fs.IntVar(&c.Hidden, "hidden", c.Hidden, "hidden units (4..128)")
	fs.IntVar(&c.Recurrent, "recurrent", c.Recurrent, "recurrent hidden units (1..hidden)")
	fs.IntVar(&c.EvalEpisodes, "eval", c.EvalEpisodes, "evaluation episodes per seed (1..1000)")
	fs.StringVar(&groupsList, "groups", groupsList, "groups to run: all or a comma-separated subset including normal")
	fs.Float64Var(&c.LearningRate, "learning-rate", c.LearningRate, "learning rate")
	fs.IntVar(&c.Comparison.Resamples, "resamples", c.Comparison.Resamples, "paired bootstrap resamples (at least 100)")
	fs.Float64Var(&c.Comparison.Interval, "interval", c.Comparison.Interval, "paired bootstrap confidence interval (0..1)")
	fs.StringVar(&outPath, "out", outPath, "new report JSON file; an existing path is never overwritten (default: stdout)")
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintf(usageOutput, "Usage: coimnet examples run attribution [--task %s] [--seeds %s] [--episodes %d] [--hidden %d] [--recurrent %d] [--eval %d] [--groups all|normal,frozen_core,...] [--learning-rate %g] [--resamples %d] [--interval %g] [--out FILE]\n", task, seedsList, c.Episodes, c.Hidden, c.Recurrent, c.EvalEpisodes, c.LearningRate, c.Comparison.Resamples, c.Comparison.Interval)
		fmt.Fprintln(usageOutput, "Runs the synthetic two-dimensional navigation attribution fixture. Reports contain every selected group and seed, paired bootstrap comparisons, and the fixed conclusion. Indented JSON goes to --out, or stdout when omitted; the summary goes to stdout with --out and stderr otherwise.")
		fmt.Fprintln(usageOutput, "Groups:")
		fmt.Fprintln(usageOutput, "  normal: trains core weights and both encoder and readout.")
		fmt.Fprintln(usageOutput, "  frozen_core: fixes core weights at initialization and trains encoder and readout.")
		fmt.Fprintln(usageOutput, "  core_only: fixes an identity encoder and an identity readout that reads only action nodes, and trains only core weights.")
		fmt.Fprintln(usageOutput, "  generic_matched: uniformly samples unique edges between the same node groups with the same edge count, then trains like normal.")
		fmt.Fprintln(usageOutput, "  rewired: performs degree-preserving double-edge swaps on the hidden recurrent block, then trains like normal.")
		fmt.Fprintln(usageOutput, "  ablated_retrained: removes the core and retrains encoder and readout from scratch.")
		fmt.Fprintln(usageOutput, "  capacity_matched_modulator: adds readout capacity sized to the MOD-10 controller and otherwise trains like normal.")
		fmt.Fprintln(usageOutput, "Comparison: paired bootstrap against normal on the unseen-map success rate and expert agreement, using same-seed differences. When core_only does not learn, the report attaches the troubleshooting checks; the periphery is never enlarged. The only conclusion is the fixed sentence: A drop after freezing or removing the core shows dependence on it, not that the original wiring is superior.")
		fmt.Fprintln(usageOutput, "Example: coimnet examples run attribution --task avoid_obstacles --seeds 1,2,3 --out attribution.json")
		fmt.Fprintln(usageOutput, "Errors: invalid --task, --seeds, --episodes, --hidden, --recurrent, --eval, --groups, --learning-rate, --resamples or --interval; an existing --out, positional arguments, cancellation or output failure. Usage errors exit with status 1. Failed runs still emit the full report and return a nonzero status.")
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
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("examples run attribution takes no positional arguments; use examples run attribution --help")}
	}
	if outPath != "" {
		if err := refuseExistingOut(outPath); err != nil {
			return &ExitError{Code: exitUsage, Err: err}
		}
	}
	seeds, err := parseExampleSeeds(seedsList)
	if err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	groups, err := parseAttributionGroups(groupsList)
	if err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	c.Env.Task = task
	c.Seeds = seeds
	c.Groups = groups
	c.Comparison.Method = "paired_bootstrap"
	c.Comparison.Baseline = experiment.AttributionNormal
	if err := c.Validate(); err != nil {
		flagName := "--task"
		switch {
		case strings.Contains(err.Error(), "seeds"):
			flagName = "--seeds"
		case strings.Contains(err.Error(), "eval episodes"):
			flagName = "--eval"
		case strings.Contains(err.Error(), "episodes"):
			flagName = "--episodes"
		case strings.Contains(err.Error(), "hidden"):
			flagName = "--hidden"
		case strings.Contains(err.Error(), "recurrent"):
			flagName = "--recurrent"
		case strings.Contains(err.Error(), "learning rate"):
			flagName = "--learning-rate"
		case strings.Contains(err.Error(), "group"):
			flagName = "--groups"
		case strings.Contains(err.Error(), "interval"):
			flagName = "--interval"
		case strings.Contains(err.Error(), "resamples"):
			flagName = "--resamples"
		}
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("%s: %w", flagName, err)}
	}
	report, err := experiment.RunAttribution(ctx, c)
	if err != nil {
		return err
	}
	if outPath == "" {
		if err := writeJSON(stdout, report); err != nil {
			return err
		}
	} else {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("encode attribution report: %w", err)
		}
		if err := writeNewTextFile(outPath, string(data)+"\n"); err != nil {
			return &ExitError{Code: exitUsage, Err: fmt.Errorf("publish --out %q: %w", outPath, err)}
		}
	}
	failed := 0
	for _, run := range report.Runs {
		if run.Failed {
			failed++
		}
	}
	summaryOut := stdout
	if outPath == "" {
		summaryOut = stderr
	}
	if _, err := fmt.Fprintf(summaryOut, "attribution: %d groups x %d seeds, %d failed runs; conclusion: %s\n", len(report.Config.Groups), len(report.Config.Seeds), failed, report.Conclusion); err != nil {
		return err
	}
	if failed > 0 {
		return fmt.Errorf("%d attribution runs failed; see report", failed)
	}
	return nil
}

func parseAttributionGroups(list string) ([]string, error) {
	all := experiment.AttributionGroups()
	if list == "all" {
		return all, nil
	}
	known := make(map[string]bool, len(all))
	for _, group := range all {
		known[group] = true
	}
	groups := strings.Split(list, ",")
	for _, group := range groups {
		if !known[group] {
			return nil, fmt.Errorf("unknown --groups name %q", group)
		}
	}
	return groups, nil
}
