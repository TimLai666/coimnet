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
  train delayed [flags]         Train the fixture and save an episode checkpoint
  resume [flags]                Continue training into a new checkpoint
  predict [flags]               Predict from observation-only JSON
  data sources                   List audited official MaleCNS source metadata
  data download [flags]          Download and verify one bounded source file
  data inspect [flags]           Inspect every batch of a local Feather file
  data import [flags]            Build graph views from a manifest and report JSON
  data validate [flags]          Verify a graph store and print its report

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
	case "resume":
		return runTrain(ctx, args[1:], stdout, stderr, true)
	case "predict":
		return runPredict(ctx, args[1:], stdout, stderr)
	case "data":
		return runData(ctx, args[1:], stdout, stderr)
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
			_, err := fmt.Fprintln(stdout, "Usage: coimnet examples list | run delayed [flags]\nRun 'coimnet examples run delayed --help' for the fixed fixture protocol.")
			return err
		}
		if len(args) == 2 && args[1] == "list" {
			return writeJSON(stdout, []map[string]string{{"name": "delayed", "profile": "fixture", "description": "Five-step delayed pulse, three synthetic neurons, trainable core weights and fixed periphery"}})
		}
		if len(args) >= 3 && args[1] == "run" && args[2] == "delayed" {
			return runDelayed(ctx, args[3:], stdout, stderr)
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

func writeJSON(w io.Writer, v any) error {
	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	return e.Encode(v)
}
