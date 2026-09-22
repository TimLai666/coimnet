package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/TimLai666/coimnet/experiment"
	"github.com/TimLai666/coimnet/experiment/gridnav"
)

const (
	gridnavUpdatesDefault = 200
	gridnavUpdatesMin     = 1
	gridnavUpdatesMax     = 100000
)

// runGridnav runs the fixed synthetic gridnav fixture with one of two learning
// protocols: supervised imitation of the corridor expert, or recurrent PPO
// trained from sampled action feedback. The protocol report is published as
// indented JSON on stdout, and a run whose declared gate fails still prints
// the complete report before returning an error.
func runGridnav(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	method := "ppo"
	updates := gridnavUpdatesDefault
	fs := flag.NewFlagSet("examples run gridnav", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&method, "method", method, "learning method: imitation or ppo")
	fs.IntVar(&updates, "updates", updates, "updates per seed (1..100000)")
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: coimnet examples run gridnav [--method ppo] [--updates 200]")
		fmt.Fprintln(usageOutput, "Runs the fixed synthetic one-dimensional corridor fixture: remember the initial goal-side cue and move left, right or stay. Imitation learns the corridor expert's actions; ppo learns from sampled action feedback. This is a numerical learnability check, not a claim about navigation in any biological brain.")
		fmt.Fprintln(usageOutput, "Outputs JSON including every seed, the protocol and the acceptance gates. Changing the update budget or the method creates a different protocol.")
		fmt.Fprintln(usageOutput, "Gates: ppo fails when any seed fails or the mean return is not above both the random baseline and the untrained policy; imitation fails when any seed fails. A failing run still prints the full report and returns a nonzero exit status.")
		fmt.Fprintln(usageOutput, "Example: coimnet examples run gridnav --method imitation > gridnav.json")
		fmt.Fprintln(usageOutput, "Errors: unknown method, invalid budget, positional arguments, cancellation, numerical failure, failed learning gate or output failure.")
		fmt.Fprintf(usageOutput, "Options:\n  --method string  Learning method, imitation or ppo (default %q)\n  --updates int    Updates per seed, default %d (%d..%d)\n", method, updates, gridnavUpdatesMin, gridnavUpdatesMax)
	}
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return usageOutput.Err()
	} else if err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if method != "imitation" && method != "ppo" {
		return fmt.Errorf("unknown --method %q: use imitation or ppo", method)
	}
	if updates < gridnavUpdatesMin || updates > gridnavUpdatesMax {
		return fmt.Errorf("--updates must be between %d and %d", gridnavUpdatesMin, gridnavUpdatesMax)
	}
	switch method {
	case "ppo":
		c := experiment.DefaultPPOExperimentConfig()
		c.Updates = updates
		report, err := experiment.RunPPO(ctx, c)
		if err != nil {
			return err
		}
		if err = writeJSON(stdout, report); err != nil {
			return err
		}
		if !report.Passed {
			return fmt.Errorf("gridnav ppo failed its declared learning gate; see JSON report")
		}
		return nil
	case "imitation":
		report, err := experiment.RunImitation(ctx, experiment.ImitationConfig{
			Corridor:     gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: .01, GoalReward: 1},
			Seeds:        []uint64{1, 2, 3},
			Episodes:     updates,
			Hidden:       16,
			LearningRate: .05,
			EvalEpisodes: 20,
		})
		if err != nil {
			return err
		}
		if err = writeJSON(stdout, report); err != nil {
			return err
		}
		for _, r := range report.Results {
			if r.Failed {
				return fmt.Errorf("gridnav imitation failed a seed; see JSON report")
			}
		}
		return nil
	}
	return fmt.Errorf("unknown --method %q: use imitation or ppo", method)
}
