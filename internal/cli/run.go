package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/TimLai666/coimnet/experiment"
)

const overview = `CoImNet: connectome training research framework

Commands:
  doctor                        Report local runtime and device capabilities as JSON
  examples list                 List executable reference tasks
  examples run delayed [flags]  Run the synthetic delayed-pulse learning protocol
  examples run lif-threshold [flags]
                                Train the spiking fixture's base firing threshold
  examples run evaluate [flags]
                                Score the delayed-correlation fixture under a
                                fixed or adaptive protocol and write the report
  examples run continual-matrix [flags]
                                Run the fixed two-task continual matrix with a
                                rule-change stage and a paired comparison report
  examples run ocr [flags]      Run the synthetic glyph OCR fixture: CTC-trained
                                line recognition with a train/test glyph family
                                split and a core-disconnect check
  examples run gridnav [flags]  Run the synthetic one-dimensional corridor:
                                expert imitation or recurrent PPO training
  examples run ablate [flags]   Run the MOD-10 modulation ablation: every control
                                group scored per seed, one report per group plus
                                a summary under DIR/ablation
  run --config FILE --dry-run   Expand a strict configuration, report where every
                                value came from, estimate the memory one run needs
                                and refuse instead of shrinking anything
  train delayed [flags]         Train the fixture and save an episode checkpoint
  resume [flags]                Continue training into a new checkpoint
  predict [flags]               Predict from observation-only JSON
  data sources                   List audited official MaleCNS source metadata
  data download [flags]          Download and verify one bounded source file
  data inspect [flags]           Inspect every batch of a local Feather file
  data import [flags]            Build graph views from a manifest and report JSON
  data derive [flags]            Derive edge signs and strengths from the release
                                 files under an explicit rules document
  data validate [flags]          Verify a graph store or parameter set and print
                                 its report
  simulate run [flags]           Run a graph store through one dynamics core
                                 with fixed injections and probes, no training
  simulate compare [flags]       Run one protocol on the original wiring and on
                                 seeded null models and report the declared
                                 metrics and thresholds
  checkpoint migrate [flags]     Migrate a snapshot into a new file under a
                                 target schema, never touch the source, and
                                 print the migration report as JSON
  benchmark [flags]              Time import, forward, backward, local
                                 plasticity, modulation and snapshot on a
                                 synthetic topology and write the report JSON
  model inspect [flags]          Inspect one model package or individual
                                 snapshot: topology, parameters, modes,
                                 evidence, mapping and a memory estimate
  model validate [flags]         Recompute and verify one model package or
                                 individual snapshot and report valid,
                                 schema and checksum
  export [flags]                 Export one model package or JSON report to a
                                 new file as sorted JSON or as Markdown
  report [flags]                 Summarise the requirements status document
                                 and its evidence records into a JSON and a
                                 Markdown completion report

Use COMMAND --help for options and examples. Unsupported commands and invalid
arguments return a nonzero exit status. Data transfers require an explicit
capacity limit and report their receipt as JSON.
`

// outputCapture preserves the usage writer's error because flag.Usage cannot
// return one. The caller checks the captured error when Parse reports help.
type outputCapture struct {
	writer io.Writer
	err    error
}

func (w *outputCapture) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if err == nil && n < len(p) {
		err = io.ErrShortWrite
	}
	if err != nil && w.err == nil {
		w.err = err
	}
	return n, err
}

func (w *outputCapture) Err() error {
	return w.err
}

// Run executes one command. Reports go to stdout and argument diagnostics to
// stderr; callers map errors to exit status. Learning-gate failures still emit
// their complete report before returning an error.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if ctx == nil || stdout == nil || stderr == nil {
		return fmt.Errorf("nil context or output writer")
	}
	// Normalize short writes for every command, including direct help and JSON.
	stdout = &outputCapture{writer: stdout}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(args) == 0 || (len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help")) {
		_, err := io.WriteString(stdout, overview)
		return err
	}
	switch args[0] {
	case "train":
		if len(args) >= 2 && args[1] == "delayed" {
			return runTrain(ctx, args[2:], stdout, stderr, false)
		}
		if len(args) == 1 || (len(args) == 2 && (args[1] == "--help" || args[1] == "-h")) {
			_, err := fmt.Fprintln(stdout, "Usage: coimnet train delayed [flags]\nUse coimnet train delayed --help for options.")
			return err
		}
	case "run":
		return runConfigRun(ctx, args[1:], stdout, stderr)
	case "resume":
		return runTrain(ctx, args[1:], stdout, stderr, true)
	case "predict":
		return runPredict(ctx, args[1:], stdout, stderr)
	case "data":
		return runData(ctx, args[1:], stdout, stderr)
	case "simulate":
		return runSimulateCommand(ctx, args[1:], stdout, stderr)
	case "checkpoint":
		return runCheckpoint(ctx, args[1:], stdout, stderr)
	case "benchmark":
		return runBenchmark(ctx, args[1:], stdout, stderr)
	case "model":
		return runModel(ctx, args[1:], stdout, stderr)
	case "export":
		return runExport(ctx, args[1:], stdout, stderr)
	case "report":
		return runReport(ctx, args[1:], stdout, stderr)
	case "doctor":
		fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
		fs.SetOutput(stderr)
		usageOutput := &outputCapture{writer: stdout}
		fs.Usage = func() {
			fmt.Fprintln(usageOutput, "Usage: coimnet doctor\nReport actual runtime, CPU, memory and detected devices as JSON.\nMissing optional device tools are reported as unknown. GPU presence does not imply sparse training support.\nExample: coimnet doctor\nErrors: invalid arguments, canceled execution or output failure.")
		}
		if err := fs.Parse(args[1:]); errors.Is(err, flag.ErrHelp) {
			return usageOutput.Err()
		} else if err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return fmt.Errorf("doctor takes no positional arguments")
		}
		report, err := Doctor(ctx)
		if err != nil {
			return err
		}
		return writeJSON(stdout, report)
	case "examples":
		if len(args) == 1 || (len(args) == 2 && (args[1] == "--help" || args[1] == "-h")) {
			_, err := fmt.Fprintln(stdout, "Usage: coimnet examples list | run delayed [flags] | run lif-threshold [flags] | run evaluate [flags] | run continual-matrix [flags] | run ocr [flags] | run gridnav [flags] | run ablate [flags]\nRun 'coimnet examples run NAME --help' for each fixed fixture protocol.")
			return err
		}
		if len(args) == 2 && args[1] == "list" {
			return writeJSON(stdout, []map[string]string{
				{"name": "delayed", "profile": "fixture", "description": "Five-step delayed pulse, three synthetic neurons, trainable core weights and fixed periphery"},
				{"name": "lif-threshold", "profile": "fixture", "description": "Five-step delayed pulse, three leaky integrate-and-fire neurons, trainable base firing threshold against frozen and fully trainable controls"},
				{"name": "evaluate", "profile": "fixture", "description": "Two-node delayed-correlation core with local plasticity, eight adaptation and eight scoring items under a fixed or adaptive protocol"},
				{"name": "continual-matrix", "profile": "fixture", "description": "Two-task delayed-core continual matrix with train stages, a rule change and a preregistered paired bootstrap comparison"},
				{"name": "ocr", "profile": "fixture", "description": "Synthetic glyph line recognition: CTC-trained column reader with family split, unseen combinations and a core-disconnect check"},
				{"name": "gridnav", "profile": "fixture", "description": "Synthetic one-dimensional corridor with expert imitation and recurrent PPO action-feedback learning"},
				{"name": "ablate", "profile": "fixture", "description": "MOD-10 modulation ablation: five control groups scored per seed on one delayed-pulse split, written as one report per group plus a summary"},
			})
		}
		if len(args) >= 3 && args[1] == "run" && args[2] == "delayed" {
			return runDelayed(ctx, args[3:], stdout, stderr)
		}
		if len(args) >= 3 && args[1] == "run" && args[2] == "lif-threshold" {
			return runLIFThreshold(ctx, args[3:], stdout, stderr)
		}
		if len(args) >= 3 && args[1] == "run" && args[2] == "evaluate" {
			return runEvaluate(ctx, args[3:], stdout, stderr)
		}
		if len(args) >= 3 && args[1] == "run" && args[2] == "continual-matrix" {
			return runContinualMatrix(ctx, args[3:], stdout, stderr)
		}
		if len(args) >= 3 && args[1] == "run" && args[2] == "ocr" {
			return runOCR(ctx, args[3:], stdout, stderr)
		}
		if len(args) >= 3 && args[1] == "run" && args[2] == "gridnav" {
			return runGridnav(ctx, args[3:], stdout, stderr)
		}
		if len(args) >= 3 && args[1] == "run" && args[2] == "ablate" {
			return runAblate(ctx, args[3:], stdout, stderr)
		}
	}
	return fmt.Errorf("unknown command; use coimnet --help")
}

func runDelayed(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	c := experiment.DefaultDelayedConfig()
	fs := flag.NewFlagSet("examples run delayed", flag.ContinueOnError)
	fs.SetOutput(stderr)
	usageOutput := &outputCapture{writer: stdout}
	fs.IntVar(&c.Updates, "updates", c.Updates, "updates per seed and condition (1..100000)")
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: coimnet examples run delayed [--updates 600]\nRuns all three fixed seeds with trained, frozen and shuffled-target controls.\nOutputs JSON including every seed, hashes and acceptance gates. Changing the update budget creates a different protocol.\nExample: coimnet examples run delayed > delayed.json\nErrors: invalid budget, cancellation, numerical failure, failed learning gate or output failure.\nOptions:\n  --updates int  Updates per seed and condition, default 600 (1..100000)")
	}
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return usageOutput.Err()
	} else if err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	report, err := experiment.RunDelayed(ctx, c)
	if err != nil {
		return err
	}
	if err = writeJSON(stdout, report); err != nil {
		return err
	}
	if !report.Passed {
		return fmt.Errorf("delayed fixture failed its declared learning gate; see JSON report")
	}
	return nil
}

func runLIFThreshold(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	updates := experiment.LIFThresholdDefaultUpdates
	fs := flag.NewFlagSet("examples run lif-threshold", flag.ContinueOnError)
	fs.SetOutput(stderr)
	usageOutput := &outputCapture{writer: stdout}
	fs.IntVar(&updates, "updates", updates, fmt.Sprintf("updates per seed and condition (%d..%d)", experiment.LIFThresholdMinUpdates, experiment.LIFThresholdMaxUpdates))
	fs.Usage = func() {
		fmt.Fprintf(usageOutput, "Usage: coimnet examples run lif-threshold [--updates %d]\n", experiment.LIFThresholdDefaultUpdates)
		fmt.Fprintln(usageOutput, "Trains the base firing threshold of a three-neuron leaky integrate-and-fire core on the synthetic five-step delayed-pulse fixture. Three fixed seeds run three matched controls: every group trainable, only the threshold group trainable, and every group frozen. This is a numerical learnability check on generated data, not a biological firing claim.")
		fmt.Fprintln(usageOutput, "Outputs JSON including every seed, the fixed core settings and the acceptance gates. Changing the update budget creates a different protocol.")
		fmt.Fprintln(usageOutput, "Gates: "+experiment.LIFThresholdGateDescription+". A failing gate prints the full report and returns a nonzero exit status.")
		fmt.Fprintln(usageOutput, "Example: coimnet examples run lif-threshold > lif-threshold.json")
		fmt.Fprintln(usageOutput, "Errors: invalid budget, cancellation, numerical failure, failed learning gate or output failure.")
		fmt.Fprintf(usageOutput, "Options:\n  --updates int  Updates per seed and condition, default %d (%d..%d)\n", experiment.LIFThresholdDefaultUpdates, experiment.LIFThresholdMinUpdates, experiment.LIFThresholdMaxUpdates)
	}
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return usageOutput.Err()
	} else if err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if updates < experiment.LIFThresholdMinUpdates || updates > experiment.LIFThresholdMaxUpdates {
		return fmt.Errorf("--updates must be between %d and %d", experiment.LIFThresholdMinUpdates, experiment.LIFThresholdMaxUpdates)
	}
	report, err := experiment.RunLIFThreshold(ctx, updates)
	if err != nil {
		return err
	}
	if err = writeJSON(stdout, report); err != nil {
		return err
	}
	if !report.Passed {
		return fmt.Errorf("lif-threshold fixture failed its declared learning gate; see JSON report")
	}
	return nil
}

func writeJSON(w io.Writer, v any) error {
	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	return e.Encode(v)
}
