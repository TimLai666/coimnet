package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/TimLai666/coimnet/experiment/multimodaleval"
)

func runMultimodal(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	c := multimodaleval.DefaultConfig()
	seedsDefault := formatSeeds(c.Seeds)
	seedsList, outPath := seedsDefault, ""
	fs := flag.NewFlagSet("examples run multimodal", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&seedsList, "seeds", seedsDefault, "comma-separated distinct model seeds")
	fs.IntVar(&c.Epochs, "epochs", c.Epochs, "training epochs")
	fs.IntVar(&c.Hidden, "hidden", c.Hidden, "hidden units")
	fs.StringVar(&outPath, "out", "", "new report JSON file; an existing path is never overwritten (default: stdout)")
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintf(usageOutput, "Usage: coimnet examples run multimodal [--seeds %s] [--epochs %d] [--hidden %d] [--out FILE]\n", seedsDefault, c.Epochs, c.Hidden)
		fmt.Fprintln(usageOutput, "Runs the synthetic multimodal paired-data fixture and reports seen and unseen image-to-text retrieval. The indented JSON report goes to --out, or stdout when omitted; the summary goes to stdout with --out and stderr otherwise.")
		fmt.Fprintln(usageOutput, "Example: coimnet examples run multimodal --seeds 1 --epochs 2 --out multimodal.json")
		fmt.Fprintln(usageOutput, "Errors: empty or duplicate seeds, epochs or hidden units outside the experiment limits, existing --out, missing or non-directory --out parent, positional arguments, cancellation or output failure. Usage errors exit with status 1. Failed runs still emit the full report and return a nonzero status.")
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
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("examples run multimodal takes no positional arguments; use examples run multimodal --help")}
	}
	if outPath != "" {
		if err := refuseExistingOut(outPath); err != nil {
			return &ExitError{Code: exitUsage, Err: err}
		}
	}
	seeds, err := parseExampleSeeds(seedsList)
	if err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	c.Seeds = seeds
	if err := c.Validate(); err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	report, err := multimodaleval.Run(ctx, c)
	if err != nil {
		return err
	}
	if outPath == "" {
		if err := writeJSON(stdout, report); err != nil {
			return err
		}
	} else {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("encode multimodal report: %w", err)
		}
		if err := writeNewTextFile(outPath, string(data)+"\n"); err != nil {
			return &ExitError{Code: exitUsage, Err: fmt.Errorf("publish --out %q: %w", outPath, err)}
		}
	}
	seen, unseen := meanMultimodalRetrieval(report)
	summaryOut := stdout
	if outPath == "" {
		summaryOut = stderr
	}
	if _, err := fmt.Fprintf(summaryOut, "multimodal: %d seeds, seen image->text %.6f, unseen image->text %.6f, chance %.6f\n", len(report.Runs), seen, unseen, report.Chance.Label); err != nil {
		return err
	}
	failed := 0
	for _, run := range report.Runs {
		if run.Failed {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d multimodal runs failed; see report", failed)
	}
	return nil
}

func meanMultimodalRetrieval(report multimodaleval.Report) (seen, unseen float64) {
	count := 0
	for _, run := range report.Runs {
		if run.Failed {
			continue
		}
		seen += run.Seen.ImageToText
		unseen += run.UnseenRetrieval.ImageToText
		count++
	}
	if count > 0 {
		seen /= float64(count)
		unseen /= float64(count)
	}
	return seen, unseen
}
