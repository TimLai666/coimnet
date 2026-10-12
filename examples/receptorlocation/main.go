// This example maps measured Rdl localization onto explicitly inferred
// MaleCNS cell types. It demonstrates location effects, not a calibrated
// receptor model or a measured behavioral improvement.
package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/modulation"
)

//go:embed testdata/fendl-rdl.csv
var measuredCSV []byte

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if ctx == nil || out == nil {
		return fmt.Errorf("nil context or output")
	}
	flags := flag.NewFlagSet("receptorlocation", flag.ContinueOnError)
	flags.SetOutput(out)
	store := flags.String("store", "", "required MaleCNS v1.0 annotated GraphStore; loaded read-only (maximum 1 GiB)")
	flags.Usage = func() {
		fmt.Fprintln(out, "Map Fendl 2020 measured dendritic Rdl localization to MaleCNS T4a-d/T5a-d types.")
		fmt.Fprintln(out, "Usage: go run ./examples/receptorlocation --store data/malecns-v1.0/graph-v1.coimgraph")
		fmt.Fprintln(out, "Writes JSON to stdout. No model training or files are written. Invalid stores/mappings return an error.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if len(args) == 0 {
		flags.Usage()
		return nil
	}
	if flags.NArg() != 0 || *store == "" {
		return fmt.Errorf("--store is required; positional arguments are unsupported")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	observations, err := observationsFromCSV(bytes.NewReader(measuredCSV))
	if err != nil {
		return err
	}
	graph, receipt, err := connectome.LoadWithReceipt(ctx, *store, connectome.StoreLimits{
		MaxFileBytes: 1 << 30, MaxFooterBytes: 1 << 20, MaxMemoryBytes: 2 << 30,
	})
	if err != nil {
		return err
	}
	if graph.Namespace() != "malecns-v1.0" {
		return fmt.Errorf("expected malecns-v1.0 namespace, got %q", graph.Namespace())
	}
	var selected []connectome.NodeRecord
	if err := graph.StreamAnnotatedNodes(ctx, func(node connectome.NodeRecord) error {
		if node.Type.Valid && len(node.Type.Value) >= 2 && (node.Type.Value[:2] == "T4" || node.Type.Value[:2] == "T5") {
			selected = append(selected, node)
		}
		return nil
	}); err != nil {
		return err
	}
	receptors, mapping, err := mapObservations(observations, selected, int(graph.NodeCount()))
	if err != nil {
		return err
	}
	control, err := locationControl(receptors, int(graph.NodeCount()))
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	report := struct {
		SourceURL    string                  `json:"source_url"`
		SourceSHA256 string                  `json:"source_sha256"`
		Article      string                  `json:"article"`
		License      string                  `json:"license"`
		Store        connectome.StoreReceipt `json:"store"`
		Observations []observation           `json:"observations"`
		Mapping      mappingReport           `json:"mapping"`
		Receptors    modulation.Receptors    `json:"receptors"`
		Control      controlReport           `json:"engineering_control"`
		Assumptions  []string                `json:"assumptions"`
	}{sourceURL, sourceSHA256, articleURL, "CC BY", receipt, observations, mapping, receptors, control, []string{
		"UAS-Rdl::GFP localization is measured in T4/T5 dendrites; it may reflect overexpression bias.",
		"MaleCNS body mapping is inferred from cell type; the study did not measure these individual bodies.",
		"Region 1 is an engineering chemical-region mapping, not a measured anatomical coordinate.",
		"Puncta counts are retained as observations and never determine Kd, Hill exponent or effect strength.",
		"The control alone uses declared Kd=1, N=1, beta=-0.2 and concentrations 0/1; it is not a quantitative biological result.",
	}}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}
