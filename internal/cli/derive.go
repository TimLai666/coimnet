package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/internal/fileio"
	"github.com/TimLai666/coimnet/params"
)

const (
	parameterDeriveSchemaVersion   = "coimnet-parameter-derive/v1"
	parameterValidateSchemaVersion = "coimnet-parameter-validate/v1"
)

// ParameterDeriveOutput is the data derive output.
type ParameterDeriveOutput struct {
	SchemaVersion string             `json:"schema_version"`
	Report        params.Report      `json:"report"`
	Set           params.SaveReceipt `json:"set"`
}

// ParameterValidateOutput is the data validate --params output.
type ParameterValidateOutput struct {
	SchemaVersion string             `json:"schema_version"`
	Path          string             `json:"path"`
	Bytes         int64              `json:"bytes"`
	SHA256        string             `json:"sha256"`
	Source        string             `json:"source"`
	RulesHash     string             `json:"rules_hash"`
	GraphHashes   params.GraphHashes `json:"graph_hashes"`
	Nodes         int                `json:"nodes"`
	Edges         int                `json:"edges"`
	GraphChecked  bool               `json:"graph_checked"`
	GraphPath     string             `json:"graph_path"`
	Verified      bool               `json:"verified"`
	Report        params.Report      `json:"report"`
}

func runDataDerive(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("data derive", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var storePath, rulesPath, outPath, tempDir string
	var limits params.Limits
	fs.StringVar(&storePath, "store", "", "graph store file written by data import --out-store")
	fs.StringVar(&rulesPath, "rules", "", "coimnet-derivation-rules/v1 JSON file; its directory resolves the four relative source paths")
	fs.StringVar(&outPath, "out", "", "new parameter set file; an existing path is never overwritten")
	fs.StringVar(&tempDir, "temp-dir", "", "existing directory for bounded sort runs and intermediate files (default: system temporary directory)")
	fs.Int64Var(&limits.MaxMemoryBytes, "max-memory-bytes", 4<<30, "accounted derivation memory limit; excludes process overhead")
	fs.Int64Var(&limits.MaxTempBytes, "max-temp-bytes", 64<<30, "total temporary bytes on disk, split across four sorts and two intermediate files")
	fs.IntVar(&limits.MaxRunFiles, "max-run-files", 512, "maximum run files per sort; also the merge fan-in")
	fs.Int64Var(&limits.MaxArrowBytes, "max-arrow-bytes", 512<<20, "maximum live Arrow allocator bytes per scan")
	fs.Int64Var(&limits.MaxRows, "max-rows", 400000000, "maximum rows per source file")
	w := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(w, "Usage: coimnet data derive --store FILE --rules FILE --out FILE [flags]\nDerive per edge signs, confidences and normalized strengths for a graph store from the four MaleCNS release files named by the rules document, write the parameter set to a new file and print the derivation report as JSON. Every source SHA-256 is verified before any sort starts. Signs come from the rule mapping applied to predicted transmitter probabilities; they are not measured, and an edge without a match, below a threshold or mapped to unknown stays unknown. The output file is never overwritten and every temporary file is removed before the command returns.\nExample: coimnet data derive --store graph.coimgraph --rules rules.json --out params.coimparams > derive.json\nErrors: missing or invalid options, unreadable or corrupt store, invalid rules, a source whose SHA-256 changed, exceeded memory/temp/run/arrow/row limits, an existing output path, cancellation or output failure. Limits fail the derivation; nothing is downsized.\nOptions:")
		fs.SetOutput(w)
		fs.PrintDefaults()
		fs.SetOutput(stderr)
	}
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return w.Err()
	} else if err != nil {
		return err
	}
	if fs.NArg() != 0 || storePath == "" || rulesPath == "" || outPath == "" {
		return fmt.Errorf("--store, --rules and --out are required; use data derive --help")
	}
	if limits.MaxMemoryBytes <= 0 {
		return fmt.Errorf("--max-memory-bytes must be positive")
	}
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	limits.TempDir = tempDir

	data, err := fileio.ReadRegular(ctx, rulesPath, params.MaxRulesBytes)
	if err != nil {
		return fmt.Errorf("read rules: %w", err)
	}
	rules, err := params.DecodeRules(bytes.NewReader(data))
	if err != nil {
		return err
	}
	graph, err := connectome.Load(ctx, storePath, connectome.StoreLimits{
		MaxFileBytes:   64 << 30,
		MaxFooterBytes: 16 << 20,
		MaxMemoryBytes: limits.MaxMemoryBytes,
	})
	if err != nil {
		return err
	}
	set, report, err := params.Derive(ctx, graph, rules, filepath.Dir(rulesPath), limits)
	if err != nil {
		return err
	}
	receipt, saveErr := params.Save(ctx, outPath, set)
	if saveErr != nil && receipt.Path == "" {
		return saveErr
	}
	// A receipt with a path means the set was published; report it even when
	// durability or cleanup failed afterwards, then return that error.
	return errors.Join(saveErr, writeJSON(stdout, ParameterDeriveOutput{
		SchemaVersion: parameterDeriveSchemaVersion,
		Report:        report,
		Set:           receipt,
	}))
}

// runParameterValidate reads a parameter set back, verifies it and optionally
// checks it against a graph store.
func runParameterValidate(ctx context.Context, path, storePath string, limits params.LoadLimits, storeLimits connectome.StoreLimits, stdout io.Writer) error {
	set, receipt, err := params.LoadWithReceipt(ctx, path, limits)
	if err != nil {
		return err
	}
	output := ParameterValidateOutput{
		SchemaVersion: parameterValidateSchemaVersion,
		Path:          path,
		Bytes:         receipt.Bytes,
		SHA256:        receipt.SHA256,
		Source:        set.Source,
		RulesHash:     set.RulesHash,
		GraphHashes:   set.GraphHashes,
		Nodes:         set.Nodes(),
		Edges:         set.Edges(),
		Verified:      true,
		Report:        set.Report,
	}
	if storePath != "" {
		graph, err := connectome.Load(ctx, storePath, storeLimits)
		if err != nil {
			return err
		}
		if err := set.CheckGraph(graph); err != nil {
			return err
		}
		output.GraphChecked = true
		output.GraphPath = storePath
	}
	return writeJSON(stdout, output)
}
