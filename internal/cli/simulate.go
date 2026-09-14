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
	"github.com/TimLai666/coimnet/params"
	"github.com/TimLai666/coimnet/simulate"
)

func runSimulateCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "--help" || args[0] == "-h")) {
		_, err := fmt.Fprintln(stdout, "Usage: coimnet simulate run --store FILE --protocol FILE [--params FILE] [flags]\nRun a stored connectome graph through one dynamics core with fixed injections and named probes, without any training. Parameters come from the protocol's uniform engineering scalars or, with --params, from a derived parameter set. Use 'coimnet simulate run --help' for options, limits and errors.")
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
	var storePath, protocolPath, parameterPath, stateIn, stateOut string
	var memoryBytes int64
	storeLimits := connectome.StoreLimits{}
	fs.StringVar(&storePath, "store", "", "graph store file written by data import --out-store (regular file, no symlink)")
	fs.StringVar(&protocolPath, "protocol", "", "run protocol JSON: core settings, injections, probes, stimulus, thresholds and the parameter source")
	fs.StringVar(&parameterPath, "params", "", "parameter set file written by data derive --out; required by the derived_release/v1 source and refused by engineering_uniform_positive")
	fs.StringVar(&stateIn, "state-in", "", "optional state snapshot JSON to continue from; it must match this core and configuration")
	fs.StringVar(&stateOut, "state-out", "", "optional new file for the state snapshot after the run; never overwritten")
	fs.Int64Var(&memoryBytes, "max-memory-bytes", 8<<30, "accounted limit applied separately to loading the store and to the simulation arrays; excludes decoded JSON and runtime overhead")
	fs.Int64Var(&storeLimits.MaxFileBytes, "max-store-bytes", 8<<30, "maximum store file size")
	fs.Int64Var(&storeLimits.MaxFooterBytes, "max-footer-bytes", 16<<20, "maximum store footer bytes")
	w := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(w, "Usage: coimnet simulate run --store FILE --protocol FILE [--params FILE] [flags]\nLoad a verified graph store, build the protocol's core from the stored topology, inject the declared stimulus into the declared neurons and print the run report as JSON. No encoder, readout, optimizer or training step is involved.\nThe protocol must name one of two parameter sources. engineering_uniform_positive needs no --params: every edge weight is gain times its raw source weight, every connection is therefore excitatory and every neuron shares one bias, log_tau and theta_raw. derived_release/v1 requires --params with a parameter set written by data derive, plus a derived block declaring unknown_sign (exclude, excitatory or inhibitory) and a positive weight_scale; each edge weight becomes weight_scale times the derived sign times the derived strength, and an edge whose sign the rules left unknown follows the declared policy and is counted in the report. Its signs are rule-derived from predicted transmitter probabilities, not measured, and bias, log_tau and theta_raw stay uniform engineering values, so neither source is a biological parameter set. The report states the assumptions either way.\nStability thresholds only add flags to the report; they never change the run and never change the exit status. Memory limits fail the run; the graph is never downsized and the step count is never lowered.\nExample: coimnet simulate run --store graph.coimgraph --protocol protocol.json --state-out state.json > run.json\nExample: coimnet simulate run --store graph.coimgraph --protocol derived.json --params params.coimparams > run.json\nErrors: missing or invalid options, unreadable or corrupt store, invalid protocol JSON, a parameter source that does not match the --params flag, a corrupt parameter set or one derived for different wiring, a selector that matches nothing, a probe reduction the core cannot produce, exceeded file/footer/memory limits, a non-finite value, an existing state-out path, cancellation or output failure.\nOptions:")
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
	parameters, err := simulateParameters(ctx, graph, protocol, parameterPath, params.LoadLimits{
		MaxFileBytes:   storeLimits.MaxFileBytes,
		MaxFooterBytes: storeLimits.MaxFooterBytes,
		MaxMemoryBytes: memoryBytes,
	})
	if err != nil {
		return err
	}
	runner, err := simulate.Build(ctx, graph, parameters, protocol, simulate.Limits{MaxMemoryBytes: memoryBytes})
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

// simulateParameters builds the parameter set the protocol declares. The
// derived source reads a parameter set file, checks it against this exact
// graph and applies the declared unknown sign policy and weight scale; the
// uniform source derives its weights from the raw edge stream and refuses a
// parameter set file, so a run can never silently ignore one. The loaded set
// is released when this function returns: only the runner's own arrays stay
// live for the run itself.
func simulateParameters(ctx context.Context, graph *connectome.Graph, protocol simulate.Protocol, path string, limits params.LoadLimits) (simulate.ParameterSet, error) {
	if protocol.ParameterSource != simulate.ParameterSourceDerived {
		if path != "" {
			return simulate.ParameterSet{}, fmt.Errorf("--params is only used by the %s parameter source; this protocol declares %s, which derives its weights from the store", simulate.ParameterSourceDerived, protocol.ParameterSource)
		}
		return simulate.UniformPositive(ctx, graph, *protocol.Uniform)
	}
	if path == "" {
		return simulate.ParameterSet{}, fmt.Errorf("--params is required: the protocol declares the %s parameter source, whose edge signs and strengths come from a parameter set file written by data derive", simulate.ParameterSourceDerived)
	}
	set, receipt, err := params.LoadWithReceipt(ctx, path, limits)
	if err != nil {
		return simulate.ParameterSet{}, err
	}
	if err := set.CheckGraph(graph); err != nil {
		return simulate.ParameterSet{}, err
	}
	parameters, _, err := simulate.FromDerived(set, receipt.SHA256, protocol)
	return parameters, err
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
