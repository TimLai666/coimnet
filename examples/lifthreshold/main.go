// Command lifthreshold trains the base firing threshold of a spiking CoImNet
// core on the synthetic delayed-pulse fixture and reports preregistered gates.
// The protocol itself lives in experiment.RunLIFThreshold, which the CLI
// command coimnet examples run lif-threshold also calls.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	osSignal "os/signal"
	"syscall"

	"github.com/TimLai666/coimnet/experiment"
)

func main() {
	ctx, stop := osSignal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run parses the command line and prints one JSON report to stdout. It writes
// no files and keeps no model parameters.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if ctx == nil || stdout == nil || stderr == nil {
		return errors.New("context and output writers are required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	fs := flag.NewFlagSet("lifthreshold", flag.ContinueOnError)
	diagnosticOutput := &errorTrackingWriter{Writer: stderr}
	fs.SetOutput(diagnosticOutput)
	usageOutput := &errorTrackingWriter{Writer: stdout}
	var updates int
	fs.IntVar(&updates, "updates", experiment.LIFThresholdDefaultUpdates, fmt.Sprintf("training updates per seed and condition (%d..%d)", experiment.LIFThresholdMinUpdates, experiment.LIFThresholdMaxUpdates))
	fs.Usage = func() {
		fmt.Fprintf(usageOutput, "Usage: go run ./examples/lifthreshold [--updates %d]\n", experiment.LIFThresholdDefaultUpdates)
		fmt.Fprintln(usageOutput, "Trains the base firing threshold of a three-neuron leaky integrate-and-fire core on the synthetic five-step delayed-pulse fixture and prints one JSON report. Three preregistered seeds run three matched controls: every group trainable, only the threshold group trainable, and every group frozen. This is a numerical learnability check on generated data, not a biological behavior claim.")
		fmt.Fprintln(usageOutput, "Gates: "+experiment.LIFThresholdGateDescription+". A failing gate prints the full report and exits non-zero.")
		fmt.Fprintln(usageOutput, "Limitations: fixed LIF settings (dt=1, tau=1, tau_syn=1, theta in [0.05,1], v_reset=-0.5, one refractory step, fast_sigmoid surrogate with scale 2, adaptation disabled); fixed seeds, splits and learning rate; 1..100000 updates; holdout counters never overlap training counters; no files or model parameters are written.")
		fmt.Fprintln(usageOutput, "Errors: unknown flags, extra arguments, an update budget outside the documented range, overlapping sample counters, cancellation, numerical failure, failing gates or output failure.")
		fmt.Fprintln(usageOutput, "Options:")
		fs.SetOutput(usageOutput)
		fs.PrintDefaults()
		fs.SetOutput(diagnosticOutput)
	}
	if err := fs.Parse(args); err != nil {
		if diagnosticErr := diagnosticOutput.Err(); diagnosticErr != nil {
			return errors.Join(err, fmt.Errorf("write flag diagnostic: %w", diagnosticErr))
		}
		if usageErr := usageOutput.Err(); usageErr != nil {
			return fmt.Errorf("write usage: %w", usageErr)
		}
		if errors.Is(err, flag.ErrHelp) {
			if fs.NArg() != 0 {
				return errors.New("unexpected positional arguments")
			}
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if updates < experiment.LIFThresholdMinUpdates || updates > experiment.LIFThresholdMaxUpdates {
		return fmt.Errorf("--updates must be between %d and %d", experiment.LIFThresholdMinUpdates, experiment.LIFThresholdMaxUpdates)
	}
	return execute(ctx, updates, stdout)
}

// execute prints the report before deciding the exit status, so a failing gate
// is always visible with its evidence.
func execute(ctx context.Context, updates int, stdout io.Writer) error {
	report, err := experiment.RunLIFThreshold(ctx, updates)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	if !report.Passed {
		return errors.New("preregistered threshold gates failed")
	}
	return nil
}

type errorTrackingWriter struct {
	io.Writer
	err error
}

func (w *errorTrackingWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.Writer.Write(p)
	if err != nil {
		w.err = err
	}
	return n, err
}

func (w *errorTrackingWriter) Err() error { return w.err }
