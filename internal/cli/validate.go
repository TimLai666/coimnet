package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/params"
)

const (
	graphImportSchemaVersion   = "coimnet-graph-import/v1"
	graphValidateSchemaVersion = "coimnet-graph-validate/v1"
)

// GraphImportOutput is the data import output when a store is written.
type GraphImportOutput struct {
	SchemaVersion string                  `json:"schema_version"`
	Report        connectome.GraphReport  `json:"report"`
	Store         connectome.StoreReceipt `json:"store"`
}

// GraphValidateOutput is the data validate output for a verified store.
type GraphValidateOutput struct {
	SchemaVersion string                  `json:"schema_version"`
	Path          string                  `json:"path"`
	Bytes         int64                   `json:"bytes"`
	SHA256        string                  `json:"sha256"`
	Namespace     string                  `json:"namespace"`
	EdgeView      connectome.EdgeViewMode `json:"edge_view"`
	Nodes         uint64                  `json:"nodes"`
	Edges         uint64                  `json:"edges"`
	Hashes        connectome.ResultHashes `json:"hashes"`
	Verified      bool                    `json:"verified"`
	Report        connectome.GraphReport  `json:"report"`
}

func runDataValidate(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("data validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var path, paramsPath string
	var limits connectome.StoreLimits
	fs.StringVar(&path, "store", "", "graph store file written by data import --out-store (regular file, no symlink)")
	fs.StringVar(&paramsPath, "params", "", "parameter set file written by data derive --out; with --store the graph hashes are checked too")
	fs.Int64Var(&limits.MaxFileBytes, "max-bytes", 8<<30, "maximum store or parameter set file size")
	fs.Int64Var(&limits.MaxFooterBytes, "max-footer-bytes", 16<<20, "maximum footer bytes")
	fs.Int64Var(&limits.MaxMemoryBytes, "max-memory-bytes", 4<<30, "accounted graph and input buffer limit; excludes decoded JSON and runtime overhead")
	w := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(w, "Usage: coimnet data validate --store FILE | --params FILE [--store FILE] [flags]\nWith --store alone, read a graph store back, verify every section hash, the footer, the structural invariants and the node index, edge order and report hashes, then print the embedded report as JSON. With --params, read a parameter set back, verify every section hash, the footer, the declared counts and the value ranges, print the embedded derivation report, and when --store is also given check that the set was derived for exactly that graph. Nothing is written.\nExample: coimnet data validate --store graph.coimgraph > validate.json\nExample: coimnet data validate --params params.coimparams --store graph.coimgraph > params-validate.json\nErrors: missing or invalid options, unreadable file, corrupt or mismatching store or parameter set, a parameter set derived for different wiring, exceeded file/footer/memory limits, cancellation or output failure.\nOptions:")
		fs.SetOutput(w)
		fs.PrintDefaults()
		fs.SetOutput(stderr)
	}
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return w.Err()
	} else if err != nil {
		return err
	}
	if fs.NArg() != 0 || (path == "" && paramsPath == "") {
		return fmt.Errorf("--store or --params is required; use data validate --help")
	}
	if paramsPath != "" {
		return runParameterValidate(ctx, paramsPath, path, params.LoadLimits{
			MaxFileBytes:   limits.MaxFileBytes,
			MaxFooterBytes: limits.MaxFooterBytes,
			MaxMemoryBytes: limits.MaxMemoryBytes,
		}, limits, stdout)
	}
	graph, receipt, err := connectome.LoadWithReceipt(ctx, path, limits)
	if err != nil {
		return err
	}
	report := graph.Report()
	return writeJSON(stdout, GraphValidateOutput{
		SchemaVersion: graphValidateSchemaVersion,
		Path:          path,
		Bytes:         receipt.Bytes,
		SHA256:        receipt.SHA256,
		Namespace:     graph.Namespace(),
		EdgeView:      report.EdgeView,
		Nodes:         graph.NodeCount(),
		Edges:         graph.EdgeCount(),
		Hashes:        report.Hashes,
		Verified:      true,
		Report:        report,
	})
}
