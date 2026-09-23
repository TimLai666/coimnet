package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/TimLai666/coimnet/experiment"
	"github.com/TimLai666/coimnet/experiment/nav2d"
)

func runNav2D(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	c := experiment.DefaultNav2DConfig()
	seedsDefault := formatSeeds(c.Seeds)
	policiesDefault := strings.Join(c.Policies, ",")
	task, seedsList, policiesList, outPath := "all", seedsDefault, policiesDefault, ""
	var episodes, hidden, recurrent, evalEpisodes int
	var learningRate float64
	fs := flag.NewFlagSet("examples run nav2d", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&task, "task", task, "task to run: all, remember_goal, avoid_obstacles, adapt_after_change or language_goal")
	fs.StringVar(&seedsList, "seeds", seedsDefault, "comma-separated distinct model seeds")
	fs.IntVar(&episodes, "episodes", c.Episodes, "training episodes per seed")
	fs.IntVar(&hidden, "hidden", c.Hidden, "hidden units")
	fs.IntVar(&recurrent, "recurrent", c.Recurrent, "recurrent hidden units")
	fs.IntVar(&evalEpisodes, "eval", c.EvalEpisodes, "evaluation episodes per seed")
	fs.StringVar(&policiesList, "policies", policiesDefault, "comma-separated policies: recurrent,feedforward,rewired,random")
	fs.Float64Var(&learningRate, "learning-rate", c.LearningRate, "learning rate")
	fs.StringVar(&outPath, "out", "", "new report JSON file; an existing path is never overwritten (default: stdout)")
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintf(usageOutput, "Usage: coimnet examples run nav2d [--task NAME|all] [--seeds %s] [--episodes %d] [--hidden %d] [--recurrent %d] [--eval %d] [--policies %s] [--learning-rate %g] [--out FILE]\n", seedsDefault, c.Episodes, c.Hidden, c.Recurrent, c.EvalEpisodes, policiesDefault, c.LearningRate)
		fmt.Fprintln(usageOutput, "Runs the synthetic two-dimensional navigation fixture across four tasks by default, comparing the selected policies and seeds. The indented JSON report goes to --out, or stdout when omitted; the summary goes to stdout with --out and stderr otherwise.")
		fmt.Fprintln(usageOutput, "Example: coimnet examples run nav2d --task avoid_obstacles --seeds 1 --episodes 10 --policies recurrent,random --out nav2d.json")
		fmt.Fprintln(usageOutput, "Errors: unknown task or policy, empty or duplicate seeds, invalid experiment settings, existing --out, missing or non-directory --out parent, positional arguments, cancellation or output failure. Usage errors exit with status 1. Failed runs still emit the full report and return a nonzero status.")
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
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("examples run nav2d takes no positional arguments; use examples run nav2d --help")}
	}
	if task != "all" && task != nav2d.TaskRememberGoal && task != nav2d.TaskAvoidObstacles && task != nav2d.TaskAdaptAfterChange && task != nav2d.TaskLanguageGoal {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("unknown --task %q", task)}
	}
	if task != "all" {
		c.Env.Task = task
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
	policies := strings.Split(policiesList, ",")
	c.Seeds = seeds
	c.Episodes = episodes
	c.Hidden = hidden
	c.Recurrent = recurrent
	c.EvalEpisodes = evalEpisodes
	c.Policies = policies
	c.LearningRate = learningRate
	if err := c.Validate(); err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}

	var report any
	tasks, failed := 1, 0
	if task == "all" {
		suite, err := experiment.RunNav2DSuite(ctx, c)
		if err != nil {
			return err
		}
		report = suite
		tasks = len(suite.Tasks)
		for _, result := range suite.Tasks {
			for _, run := range result.Runs {
				if run.Failed {
					failed++
				}
			}
		}
	} else {
		result, err := experiment.RunNav2D(ctx, c)
		if err != nil {
			return err
		}
		report = result
		for _, run := range result.Runs {
			if run.Failed {
				failed++
			}
		}
	}
	if outPath == "" {
		if err := writeJSON(stdout, report); err != nil {
			return err
		}
	} else {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("encode nav2d report: %w", err)
		}
		if err := writeNewTextFile(outPath, string(data)+"\n"); err != nil {
			return &ExitError{Code: exitUsage, Err: fmt.Errorf("publish --out %q: %w", outPath, err)}
		}
	}
	summaryOut := stdout
	if outPath == "" {
		summaryOut = stderr
	}
	if _, err := fmt.Fprintf(summaryOut, "nav2d: %d tasks x %d policies x %d seeds, %d failed runs\n", tasks, len(policies), len(seeds), failed); err != nil {
		return err
	}
	if failed > 0 {
		return fmt.Errorf("%d nav2d runs failed; see report", failed)
	}
	return nil
}

func parseExampleSeeds(list string) ([]uint64, error) {
	parts := strings.Split(list, ",")
	seeds := make([]uint64, 0, len(parts))
	seen := make(map[uint64]bool, len(parts))
	for _, part := range parts {
		seed, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid --seeds value %q", part)
		}
		if seen[seed] {
			return nil, fmt.Errorf("--seeds contains duplicate seed %d", seed)
		}
		seen[seed] = true
		seeds = append(seeds, seed)
	}
	return seeds, nil
}

func formatSeeds(seeds []uint64) string {
	parts := make([]string, len(seeds))
	for i, seed := range seeds {
		parts[i] = strconv.FormatUint(seed, 10)
	}
	return strings.Join(parts, ",")
}
