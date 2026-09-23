package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/TimLai666/coimnet/experiment/fullgraph"
)

func runFullGraph(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	o := fullgraph.DefaultOptions()
	var resumeDir string
	fs := flag.NewFlagSet("examples run full-graph-short-training", flag.ContinueOnError)
	fs.SetOutput(stderr)
	usageOutput := &outputCapture{writer: stdout}
	fs.StringVar(&o.Store, "store", o.Store, "graph store file")
	fs.StringVar(&o.Params, "params", o.Params, "derived parameter set file")
	fs.StringVar(&o.Protocol, "protocol", o.Protocol, "simulate compare protocol file")
	fs.StringVar(&o.OutDir, "out-dir", o.OutDir, "new output directory")
	fs.StringVar(&o.InputSet, "input-set", o.InputSet, "named input set")
	fs.StringVar(&o.ReadoutSet, "readout-set", o.ReadoutSet, "named readout set")
	fs.IntVar(&o.Steps, "steps", o.Steps, "training steps")
	fs.IntVar(&o.Truncation, "truncation", o.Truncation, "truncated backpropagation rows")
	fs.IntVar(&o.ContinueRows, "continue-rows", o.ContinueRows, "rows to run after the update")
	fs.IntVar(&o.PlasticEdges, "plastic-edges", o.PlasticEdges, "plastic edges (0 disables plasticity)")
	fs.BoolVar(&o.Chemistry, "chemistry", o.Chemistry, "enable chemical modulation")
	fs.Float64Var(&o.LearningRate, "learning-rate", o.LearningRate, "optimizer learning rate")
	fs.IntVar(&o.MaxMemoryMiB, "max-memory-mib", o.MaxMemoryMiB, "memory estimate limit in MiB")
	fs.IntVar(&o.MaxCells, "max-cells", o.MaxCells, "maximum model cells (0 uses the library limit)")
	fs.StringVar(&resumeDir, "resume", "", "resume and compare both digests from an output directory")
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: coimnet examples run full-graph-short-training --store FILE --params FILE --protocol FILE --out-dir DIR [flags]")
		fmt.Fprintln(usageOutput, "Runs OPS-07 full-graph short training. Every node and edge participates; the graph is never cropped. A memory estimate is checked first and the run is refused when it exceeds the limit.")
		fmt.Fprintln(usageOutput, "The run saves model, individual and training state as three directory bundles: model.coimbundle, individual.coimbundle and training.coimbundle. --resume loads them in a new process and compares both digests.")
		fmt.Fprintln(usageOutput, "For real data, first use data import to create the store and data derive to create the parameter set. Recommended protocol: evidence/NAT-05/compare-fullgraph-derived-continuous.json.")
		fmt.Fprintln(usageOutput, "Example: coimnet examples run full-graph-short-training --store graph.coimgraph --params params.coimparams --protocol evidence/NAT-05/compare-fullgraph-derived-continuous.json --out-dir fullgraph-run")
		fmt.Fprintln(usageOutput, "Example: coimnet examples run full-graph-short-training --resume fullgraph-run")
		fmt.Fprintln(usageOutput, "Errors: invalid flags or options, missing required paths, an existing --out-dir, memory estimate above the limit, load or training errors, digest mismatch, cancellation or output failure. Usage errors exit with status 1.")
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
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("examples run full-graph-short-training takes no positional arguments")}
	}
	resumeSet := false
	resumeOnly := true
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "resume" {
			resumeSet = true
		} else {
			resumeOnly = false
		}
	})
	if resumeSet {
		if !resumeOnly {
			return &ExitError{Code: exitUsage, Err: fmt.Errorf("--resume cannot be combined with other flags")}
		}
		if resumeDir == "" {
			return &ExitError{Code: exitUsage, Err: fmt.Errorf("--resume requires a directory")}
		}
		report, err := fullgraph.Resume(ctx, resumeDir)
		if err != nil {
			return err
		}
		if err := writeJSON(stdout, report); err != nil {
			return err
		}
		if !report.Matches {
			return fmt.Errorf("full-graph-short-training: resumed digests do not match the report")
		}
		return nil
	}
	for _, required := range []struct{ flag, value string }{
		{"--store", o.Store}, {"--params", o.Params}, {"--protocol", o.Protocol}, {"--out-dir", o.OutDir},
	} {
		if required.value == "" {
			return &ExitError{Code: exitUsage, Err: fmt.Errorf("%s is required", required.flag)}
		}
	}
	if err := o.Validate(); err != nil {
		flagName := "--input-set"
		for _, field := range []struct{ text, name string }{
			{"steps", "--steps"}, {"truncation", "--truncation"}, {"continue_rows", "--continue-rows"},
			{"plastic_edges", "--plastic-edges"}, {"learning_rate", "--learning-rate"},
			{"max_memory_mib", "--max-memory-mib"}, {"max_cells", "--max-cells"},
			{"input_set", "--input-set"}, {"readout_set", "--readout-set"},
			{"input set and readout set", "--input-set/--readout-set"},
		} {
			if strings.Contains(err.Error(), field.text) {
				flagName = field.name
				break
			}
		}
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("%s: %w", flagName, err)}
	}
	if _, err := os.Lstat(o.OutDir); err == nil {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--out-dir %q already exists; choose a new directory", o.OutDir)}
	} else if !errors.Is(err, os.ErrNotExist) {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--out-dir %q: %w", o.OutDir, err)}
	}
	report, err := fullgraph.Run(ctx, o)
	if err != nil {
		return err
	}
	const mib = uint64(1 << 20)
	estimateMiB := report.Memory.TotalBytes / mib
	if report.Memory.TotalBytes%mib != 0 {
		estimateMiB++
	}
	_, err = fmt.Fprintf(stdout, "full-graph-short-training: %d nodes, %d edges, updates %d, estimate %d MiB, report %s/report.json, continuation %s\n",
		report.Nodes, report.Edges, report.Run.Updates, estimateMiB, o.OutDir, report.ContinuationDigest[:12])
	return err
}
