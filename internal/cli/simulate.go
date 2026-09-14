package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/internal/fileio"
	"github.com/TimLai666/coimnet/simulate"
)

func runSimulateCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "--help" || args[0] == "-h")) {
		_, err := fmt.Fprintln(stdout, "Usage: coimnet simulate run --store FILE --protocol FILE [flags]\nRun a stored connectome graph through one dynamics core with fixed injections and named probes, without any training. Use 'coimnet simulate run --help' for options, limits and errors.")
		return err
	}
	if args[0] != "run" {
		return fmt.Errorf("unknown simulate command; use simulate --help")
	}
	return runSimulateRun(ctx, args[1:], stdout, stderr)
}

func runSimulateRun(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("simulate run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var storePath, protocolPath, stateIn, stateOut string
	var memoryBytes int64
	storeLimits := connectome.StoreLimits{}
	fs.StringVar(&storePath, "store", "", "graph store file written by data import --out-store (regular file, no symlink)")
	fs.StringVar(&protocolPath, "protocol", "", "run protocol JSON: core settings, injections, probes, stimulus, thresholds and the parameter source")
	fs.StringVar(&stateIn, "state-in", "", "optional state snapshot JSON to continue from; it must match this core and configuration")
	fs.StringVar(&stateOut, "state-out", "", "optional new file for the state snapshot after the run; never overwritten")
	fs.Int64Var(&memoryBytes, "max-memory-bytes", 8<<30, "accounted limit applied separately to loading the store and to the simulation arrays; excludes decoded JSON and runtime overhead")
	fs.Int64Var(&storeLimits.MaxFileBytes, "max-store-bytes", 8<<30, "maximum store file size")
	fs.Int64Var(&storeLimits.MaxFooterBytes, "max-footer-bytes", 16<<20, "maximum store footer bytes")
	w := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(w, "Usage: coimnet simulate run --store FILE --protocol FILE [flags]\nLoad a verified graph store, build the protocol's core from the stored topology, inject the declared stimulus into the declared neurons and print the run report as JSON. No encoder, readout, optimizer or training step is involved.\nThe only parameter source this command accepts is engineering_uniform_positive: every edge weight is gain times its raw source weight, every connection is therefore excitatory and every neuron shares one bias, log_tau and theta_raw. That is an explicit engineering assumption, stated in the report, and not a biological parameter set; parameters derived from the release belong to a later ticket.\nStability thresholds only add flags to the report; they never change the run and never change the exit status. Memory limits fail the run; the graph is never downsized and the step count is never lowered.\nExample: coimnet simulate run --store graph.coimgraph --protocol protocol.json --state-out state.json > run.json\nErrors: missing or invalid options, unreadable or corrupt store, invalid protocol JSON, an unavailable parameter source, a selector that matches nothing, a probe reduction the core cannot produce, exceeded file/footer/memory limits, a non-finite value, an existing state-out path, cancellation or output failure.\nOptions:")
		fs.SetOutput(w)
		fs.PrintDefaults()
		fs.SetOutput(stderr)
	}
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return w.Err()
	} else if err != nil {
		return err
	}
	if fs.NArg() != 0 || storePath == "" || protocolPath == "" {
		return fmt.Errorf("--store and --protocol are required; use simulate run --help")
	}
	if memoryBytes <= 0 {
		return fmt.Errorf("--max-memory-bytes must be positive")
	}
	storeLimits.MaxMemoryBytes = memoryBytes

	// Refuse an existing state-out path before doing any work, so a long run is
	// not thrown away by a conflict that was knowable up front. The exclusive
	// create below is still the authority.
	if stateOut != "" {
		if _, err := os.Lstat(stateOut); err == nil {
			return fmt.Errorf("state output %q already exists", stateOut)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat state output: %w", err)
		}
	}

	protocolBytes, err := fileio.ReadRegular(ctx, protocolPath, simulate.MaxProtocolBytes)
	if err != nil {
		return fmt.Errorf("read protocol: %w", err)
	}
	protocol, err := simulate.DecodeProtocol(bytes.NewReader(protocolBytes))
	if err != nil {
		return err
	}
	var restored simulate.StateSnapshot
	if stateIn != "" {
		stateBytes, err := fileio.ReadRegular(ctx, stateIn, simulate.MaxStateBytes)
		if err != nil {
			return fmt.Errorf("read state: %w", err)
		}
		if restored, err = simulate.DecodeState(bytes.NewReader(stateBytes)); err != nil {
			return err
		}
	}

	graph, err := connectome.Load(ctx, storePath, storeLimits)
	if err != nil {
		return err
	}
	params, err := simulate.UniformPositive(ctx, graph, *protocol.Uniform)
	if err != nil {
		return err
	}
	runner, err := simulate.Build(ctx, graph, params, protocol, simulate.Limits{MaxMemoryBytes: memoryBytes})
	if err != nil {
		return err
	}
	// The runner keeps no reference to the graph, so the store arrays become
	// collectable here while the run holds only the core's own topology copy.
	if stateIn != "" {
		if err := runner.RestoreState(restored); err != nil {
			return err
		}
	}
	report, err := runner.Run(ctx, runner.Stimulus())
	if err != nil {
		return err
	}
	if stateOut != "" {
		if err := writeNewJSON(stateOut, runner.State()); err != nil {
			return fmt.Errorf("write state: %w", err)
		}
	}
	return writeJSON(stdout, report)
}

// writeNewJSON writes one JSON document to a path that must not exist. O_EXCL
// makes the check and the create one operation, so a concurrent writer loses
// instead of being overwritten.
func writeNewJSON(path string, value any) (retErr error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			retErr = errors.Join(retErr, closeErr)
		}
	}()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return err
	}
	return file.Sync()
}
