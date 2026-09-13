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
)

func runDataImport(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("data import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var manifestPath, tempDir, edgeView string
	var limits connectome.ResourceLimits
	fs.StringVar(&manifestPath, "manifest", "", "dataset manifest JSON (regular file); relative source paths resolve against its directory")
	fs.StringVar(&tempDir, "temp-dir", "", "existing directory for bounded sort runs (default: system temporary directory)")
	fs.StringVar(&edgeView, "edge-view", string(connectome.EdgeViewRows), "rows keeps every source row; aggregated_pairs needs additive duplicate semantics in the manifest")
	fs.Int64Var(&limits.MaxMemoryBytes, "max-memory-bytes", 4<<30, "accounted builder memory limit; excludes process overhead")
	fs.Int64Var(&limits.MaxTempBytes, "max-temp-bytes", 16<<30, "total sort run bytes on disk, split across three passes")
	fs.IntVar(&limits.MaxRunFiles, "max-run-files", 256, "maximum run files per sort pass; also the merge fan-in")
	fs.Int64Var(&limits.MaxArrowBytes, "max-arrow-bytes", 128<<20, "maximum live Arrow allocator bytes per scan")
	fs.Int64Var(&limits.MaxFooterBytes, "max-footer-bytes", 16<<20, "maximum Feather footer bytes")
	fs.Int64Var(&limits.MaxRows, "max-rows", 250000000, "maximum rows per source file")
	fs.Int64Var(&limits.SortBufferBytes, "sort-buffer-bytes", 0, "fixed in-memory buffer per sort pass; 0 derives it from the remaining memory limit")
	w := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(w, "Usage: coimnet data import --manifest FILE [flags]\nBuild the raw_segments and annotated_neurons views from the manifest's three read-only sources and write the graph report as JSON. Source fingerprints are verified before and after; the graph itself stays in memory and is not persisted by this command.\nExample: coimnet data import --manifest data/malecns-v1.0/manifest.json > graph-report.json\nErrors: invalid manifest or selection, missing identity evidence, changed sources, schema mismatch, exceeded memory/temp/run/row limits, aggregation without evidence, cancellation or output failure. Limits fail the build; the graph is never downsized.\nOptions:")
		fs.SetOutput(w)
		fs.PrintDefaults()
		fs.SetOutput(stderr)
	}
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return w.Err()
	} else if err != nil {
		return err
	}
	if fs.NArg() != 0 || manifestPath == "" {
		return fmt.Errorf("--manifest is required; use data import --help")
	}
	data, err := fileio.ReadRegular(ctx, manifestPath, connectome.MaxManifestBytes)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	manifest, err := connectome.DecodeManifest(bytes.NewReader(data))
	if err != nil {
		return err
	}
	base := filepath.Dir(manifestPath)
	for i := range manifest.Files {
		if !filepath.IsAbs(manifest.Files[i].Path) {
			manifest.Files[i].Path = filepath.Join(base, manifest.Files[i].Path)
		}
	}
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	result, err := connectome.Build(ctx, connectome.BuildRequest{
		Manifest: manifest,
		Limits:   limits,
		TempDir:  tempDir,
		EdgeView: connectome.EdgeViewMode(edgeView),
	})
	if err != nil {
		return err
	}
	return writeJSON(stdout, result.Report)
}
