package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/TimLai666/coimnet/checkpoint"
)

// runCheckpoint dispatches the checkpoint command group. Only one subcommand
// exists: migrate rewrites a snapshot document into a new file under a target
// schema and prints the migration report.
func runCheckpoint(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "--help" || args[0] == "-h")) {
		_, err := fmt.Fprintln(stdout, "Usage: coimnet checkpoint migrate [flags]\nMigrate a snapshot document into a new file under a target schema without touching the source.\nRun 'coimnet checkpoint migrate --help' for options, an example and the error list.")
		return err
	}
	if args[0] != "migrate" {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("unknown checkpoint command %q: use coimnet checkpoint migrate", args[0])}
	}
	return runCheckpointMigrate(ctx, args[1:], stdout, stderr)
}

// runCheckpointMigrate wraps checkpoint.Migrate for the command line: the
// migration itself is thin the way every CLI command is thin, and the report is
// printed as indented JSON to stdout. A usage mistake exits with status 1 and
// names the flag that caused it.
func runCheckpointMigrate(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	var src, dst, target string
	fs := flag.NewFlagSet("checkpoint migrate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&src, "src", "", "path to the source snapshot document; never modified (required)")
	fs.StringVar(&dst, "dst", "", "path to the new migrated document; an existing path is refused (required)")
	fs.StringVar(&target, "target", checkpoint.IndividualSchemaVersion, "target schema version (default "+checkpoint.IndividualSchemaVersion+")")
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintf(usageOutput, "Usage: coimnet checkpoint migrate --src FILE --dst FILE [--target %s]\n", checkpoint.IndividualSchemaVersion)
		fmt.Fprintln(usageOutput, "Reads the snapshot document at --src with the schema's strict decoder, rewrites it under the target schema and publishes --dst atomically. The source bytes are never changed and an existing --dst path is never overwritten.")
		fmt.Fprintln(usageOutput, "Supported migrations: an individual document written before the neural union (payload.neural is the continuous state itself) to "+checkpoint.IndividualSchemaVersion+", and any document whose schema already equals the target as a byte-for-byte copy. A document read under a schema the target does not accept is refused with the schema pair.")
		fmt.Fprintln(usageOutput, "Prints one indented JSON migration report: source_path, target_path, source_sha256, target_sha256, source_schema, target_schema, field_changes, precision_mapping, information_loss and no_information_loss.")
		fmt.Fprintln(usageOutput, "Example: coimnet checkpoint migrate --src old-individual.json --dst migrated.json")
		fmt.Fprintln(usageOutput, "Errors: missing --src or --dst, the same path for both, an existing --dst, an unreadable or invalid source, an unsupported migration, cancellation or output failure. Usage errors exit with status 1 and name the flag.")
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
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("checkpoint migrate takes no positional arguments; use checkpoint migrate --help")}
	}
	if src == "" {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--src is required; use checkpoint migrate --help")}
	}
	if dst == "" {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--dst is required; use checkpoint migrate --help")}
	}
	if target == "" {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--target must not be empty; use checkpoint migrate --help")}
	}
	report, err := checkpoint.Migrate(ctx, src, dst, target)
	if err != nil {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("checkpoint migrate --src %s --dst %s: %w", src, dst, err)}
	}
	if err := writeJSON(stdout, report); err != nil {
		return err
	}
	return nil
}
