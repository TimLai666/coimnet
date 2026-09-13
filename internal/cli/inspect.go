package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/TimLai666/coimnet/feather"
)

func runDataInspect(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("data inspect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var path string
	var options feather.Options
	fs.StringVar(&path, "input", "", "Feather V2 / Arrow IPC file path (regular file, no symlink)")
	fs.Int64Var(&options.MaxFileBytes, "max-bytes", 0, "required positive file size limit")
	fs.Int64Var(&options.MaxFooterBytes, "max-footer-bytes", 8<<20, "maximum footer bytes")
	fs.Int64Var(&options.MaxArrowBytes, "max-arrow-bytes", 128<<20, "maximum live Arrow allocator bytes; excludes other process memory")
	fs.Int64Var(&options.MaxRows, "max-rows", 200000000, "maximum total rows")
	w := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(w, "Usage: coimnet data inspect --input FILE --max-bytes N [flags]\nRead every record batch and report the source schema and row count as JSON. Preserves the source file. A partial scan has complete=false and returns an error.\nExample: coimnet data inspect --input annotations.feather --max-bytes 20000000\nErrors: invalid options, unsupported format or type, malformed file, exceeded limits, cancellation or output failure.\nOptions:")
		fs.SetOutput(w)
		fs.PrintDefaults()
		fs.SetOutput(stderr)
	}
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return w.Err()
	} else if err != nil {
		return err
	}
	if fs.NArg() != 0 || path == "" || options.MaxFileBytes <= 0 || options.MaxFooterBytes <= 0 || options.MaxArrowBytes <= 0 || options.MaxRows <= 0 {
		return fmt.Errorf("input and positive limits are required; use data inspect --help")
	}
	report, scanErr := feather.Scan(ctx, path, options, nil)
	return errors.Join(scanErr, writeJSON(stdout, report))
}
