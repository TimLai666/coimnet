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
	"sort"
	"strconv"
	"strings"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/internal/fileio"
)

const (
	// The two shapes export publishes. json is the archival form: the same
	// values the input declared, indented and with object keys sorted at every
	// level, so two exports of one document compare byte for byte. markdown is
	// the reading form and is deliberately lossy.
	exportFormatJSON     = "json"
	exportFormatMarkdown = "markdown"

	// exportSupportedFormats is printed by every format refusal, so a caller
	// who named a format this build does not have learns the whole set at once.
	exportSupportedFormats = "supported: " + exportFormatJSON + ", " + exportFormatMarkdown

	// maxExportBytes bounds the document export reads. It is the artefact limit
	// the checkpoint reader uses, not a promise about report sizes.
	maxExportBytes = 64 << 20
)

// runExport converts one model package or one of this project's JSON reports
// into a new file. The input is never modified and an existing --out is never
// replaced; every mistake a caller can make is detected before anything is
// written.
func runExport(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	var in, out string
	format := exportFormatJSON
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&in, "in", "", "path to the model package or JSON report to export; never modified (required)")
	fs.StringVar(&out, "out", "", "path of the new exported document; an existing path is refused (required)")
	fs.StringVar(&format, "format", exportFormatJSON, "output format, "+exportSupportedFormats+" (default "+exportFormatJSON+")")
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintf(usageOutput, "Usage: coimnet export --in FILE --out FILE [--format %s|%s]\n", exportFormatJSON, exportFormatMarkdown)
		fmt.Fprintln(usageOutput, "Reads one model package or any of this project's JSON reports, identified by its top-level schema_version, and writes it to a new --out file. A document without a top-level schema_version string is refused rather than exported as an unknown shape.")
		fmt.Fprintln(usageOutput, "--format "+exportFormatJSON+" writes the same values indented with object keys sorted at every level, so the export is stable whatever order the source file used. --format "+exportFormatMarkdown+" writes a reading copy and is lossy: a model package becomes its schema, topology fingerprint, units, compatible versions, evidence registry and a parameter statistics section (count, min, max, mean), and any other report becomes one key and value table whose nested values are printed as single-line JSON.")
		fmt.Fprintln(usageOutput, "Example: coimnet export --in model.coimpkg --out model.md --format "+exportFormatMarkdown)
		fmt.Fprintln(usageOutput, "Errors: missing --in or --out, the same path for both, an existing --out, an unreadable or invalid input, a document with no schema_version, an unsupported --format ("+exportSupportedFormats+"), cancellation or output failure. Usage errors exit with status 1 and name the flag.")
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
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("export takes no positional arguments; use export --help")}
	}
	if in == "" {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--in is required; use export --help")}
	}
	if out == "" {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--out is required; use export --help")}
	}
	if in == out {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--in and --out must be different paths; export never rewrites its input")}
	}
	if format != exportFormatJSON && format != exportFormatMarkdown {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("unsupported --format %q; %s", format, exportSupportedFormats)}
	}
	if err := refuseExistingOutput("export", "--out", out); err != nil {
		return err
	}
	document, err := loadExportDocument(ctx, in)
	if err != nil {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("export --in %s: %w", in, err)}
	}
	if format == exportFormatJSON {
		if err := writeNewJSON(out, document.fields); err != nil {
			return fmt.Errorf("write export %s: %w", out, err)
		}
	} else {
		text, err := exportMarkdown(ctx, in, document)
		if err != nil {
			return &ExitError{Code: exitUsage, Err: fmt.Errorf("export --in %s: %w", in, err)}
		}
		if err := writeNewTextFile(out, text); err != nil {
			return fmt.Errorf("write export %s: %w", out, err)
		}
	}
	_, err = fmt.Fprintf(stdout, "export %s: wrote %s from %s (%s)\n", format, out, in, document.schemaVersion)
	return err
}

// exportDocument is one decoded input: the schema version that identifies it
// and its whole top-level object. Numbers keep their source literal, so an
// export never rewrites 1.0 as 1 or loses digits from a large integer.
type exportDocument struct {
	schemaVersion string
	fields        map[string]any
}

// loadExportDocument reads and decodes one input. It only identifies the
// document; a format that needs the strict decoder re-reads the same file
// through the loader that owns it.
func loadExportDocument(ctx context.Context, path string) (exportDocument, error) {
	var empty exportDocument
	data, err := fileio.ReadRegular(ctx, path, maxExportBytes)
	if err != nil {
		return empty, fmt.Errorf("read: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return empty, fmt.Errorf("decode: %w", err)
	}
	if decoder.More() {
		return empty, fmt.Errorf("decode: the file carries more than one JSON document")
	}
	fields, ok := value.(map[string]any)
	if !ok {
		return empty, fmt.Errorf(`the file is not a JSON object, so it carries no top-level "schema_version"; export reads this project's model packages and JSON reports`)
	}
	version, ok := fields["schema_version"].(string)
	if !ok || version == "" {
		return empty, fmt.Errorf(`the file has no top-level "schema_version" string; export reads this project's model packages and JSON reports`)
	}
	return exportDocument{schemaVersion: version, fields: fields}, nil
}

// exportMarkdown renders the reading copy. A model package is re-read through
// checkpoint.LoadModelPackage, so the rendered fingerprint, units and evidence
// are the ones the strict decoder verified rather than whatever the file
// claimed; every other report is rendered from its decoded top-level fields.
func exportMarkdown(ctx context.Context, path string, document exportDocument) (string, error) {
	if document.schemaVersion != checkpoint.ModelPackageSchemaVersion {
		return exportReportMarkdown(document), nil
	}
	pkg, err := checkpoint.LoadModelPackage(ctx, path)
	if err != nil {
		return "", err
	}
	return exportPackageMarkdown(pkg), nil
}

// exportPackageMarkdown renders one verified model package.
func exportPackageMarkdown(pkg checkpoint.ModelPackage) string {
	var b strings.Builder
	b.WriteString("# Model package\n\n")
	fmt.Fprintf(&b, "- Schema: %s\n", pkg.SchemaVersion)
	fmt.Fprintf(&b, "- Topology: %d nodes, %d edges, fingerprint %s\n", pkg.Topology.Nodes, pkg.Topology.Edges, pkg.Topology.SHA256)
	fmt.Fprintf(&b, "- Units: one time step is counted in %s, one time constant in %s\n", pkg.Units.TimeStep, pkg.Units.TimeConstant)
	fmt.Fprintf(&b, "- Can seed individual snapshots: %s\n", exportVersionList(pkg.CompatibleVersions.Individual))
	fmt.Fprintf(&b, "- Can seed training snapshots: %s\n", exportVersionList(pkg.CompatibleVersions.Training))
	b.WriteString("\n## Evidence\n\n")
	if len(pkg.EvidenceRegistry) == 0 {
		b.WriteString("The publisher declared no evidence path.\n")
	}
	for _, record := range pkg.EvidenceRegistry {
		fmt.Fprintf(&b, "- %s\n", record)
	}
	stats := modelParameterStats(pkg.Parameters)
	b.WriteString("\n## Parameters\n\nThe statistics cover the base core weights; the count is every learnable value the package carries.\n\n")
	b.WriteString("| Statistic | Value |\n| --- | --- |\n")
	fmt.Fprintf(&b, "| count | %d |\n", stats.Count)
	fmt.Fprintf(&b, "| min | %s |\n", exportFloat(stats.Min))
	fmt.Fprintf(&b, "| max | %s |\n", exportFloat(stats.Max))
	fmt.Fprintf(&b, "| mean | %s |\n", exportFloat(stats.Mean))
	return b.String()
}

// exportReportMarkdown renders any report that declares a schema version as one
// table of its top-level keys. Nested values are printed as single-line JSON,
// because a table cannot carry their structure and inventing one would misread
// the report.
func exportReportMarkdown(document exportDocument) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", document.schemaVersion)
	b.WriteString("Every top-level key of the report. Nested values are printed as single-line JSON.\n\n")
	b.WriteString("| Key | Value |\n| --- | --- |\n")
	keys := make([]string, 0, len(document.fields))
	for key := range document.fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&b, "| %s | %s |\n", exportCell(key), exportCell(exportValueText(document.fields[key])))
	}
	return b.String()
}

// exportValueText prints one decoded value for a table cell: a string as
// itself, anything else as the single-line JSON it came from.
func exportValueText(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}

// exportCell keeps one value inside its table cell: a pipe would end the cell
// early and a line break would end the row.
func exportCell(text string) string {
	return strings.NewReplacer("|", "\\|", "\r\n", " ", "\n", " ", "\r", " ").Replace(text)
}

// exportFloat prints one statistic with the shortest representation that reads
// back as the same float64.
func exportFloat(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}

// exportVersionList prints a declared version list, naming an empty one rather
// than leaving the line blank.
func exportVersionList(versions []string) string {
	if len(versions) == 0 {
		return "none declared"
	}
	return strings.Join(versions, ", ")
}

// refuseExistingOutput refuses an output path that is already taken, before the
// command does any work. The exclusive create in the writer remains the
// authority; this only reports the conflict as the usage mistake it is.
func refuseExistingOutput(command, flagName, path string) error {
	if _, err := os.Lstat(path); err == nil {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("%s %s %q already exists; pick a new path", command, flagName, path)}
	} else if !errors.Is(err, os.ErrNotExist) {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("stat %s output: %w", command, err)}
	}
	return nil
}

// writeNewTextFile publishes one new text document. An existing path is never
// replaced, which is the exclusive create, and the bytes are flushed before the
// command reports success.
func writeNewTextFile(path, contents string) (retErr error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			retErr = errors.Join(retErr, closeErr)
		}
	}()
	if _, err := io.WriteString(file, contents); err != nil {
		return err
	}
	return file.Sync()
}
