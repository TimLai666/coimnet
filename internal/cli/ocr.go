package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/TimLai666/coimnet/tasks/ocr"
)

// runOCR runs the fixed synthetic OCR fixture over its three seeds and
// publishes the report as indented JSON, to --out when it is given and to
// stdout otherwise. The one-line summary always goes to stdout.
func runOCR(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	var outPath string
	fs := flag.NewFlagSet("examples run ocr", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&outPath, "out", "", "new OCR fixture report JSON file; an existing path is never overwritten (default: stdout)")
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: coimnet examples run ocr [--out FILE]")
		fmt.Fprintln(usageOutput, "Runs the fixed synthetic glyph fixture: a 3-rune alphabet drawn as 8x8 pseudo-glyphs, 40 training lines from glyph family 1 and 15 held-out lines from glyph family 2, so the shapes the run is scored on are never the shapes it trained on. Every one of the three seeds scores the held-out split before training, runs 3 epochs through a per-column readout trained with CTC and scores again, both on the whole held-out split and on the held-out lines whose text never appeared in training. One seed also re-reads an image with every core weight set to 0, which must change the logits. The indented JSON report goes to --out, or to stdout when --out is omitted, and a one-line summary always goes to stdout.")
		fmt.Fprintln(usageOutput, "Example: coimnet examples run ocr --out ocr-fixture.json")
		fmt.Fprintln(usageOutput, "Errors: an existing --out path, an --out parent that is missing or is not a directory, any positional argument, cancellation or output failure. Usage errors exit with status 1 and name the flag. The scores are fixture evidence, not an OCR capability claim; the report's data_scope and assumptions state what they were measured on.")
	}
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return usageOutput.Err()
	} else if err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	if fs.NArg() != 0 {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("examples run ocr takes no positional arguments; use examples run ocr --help")}
	}
	if outPath != "" {
		if err := refuseExistingOut(outPath); err != nil {
			return &ExitError{Code: exitUsage, Err: err}
		}
	}
	report, err := ocr.RunOCRFixture(ctx, ocrFixtureConfig())
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
			return fmt.Errorf("encode OCR fixture report: %w", err)
		}
		if err := writeNewTextFile(outPath, string(data)+"\n"); err != nil {
			return &ExitError{Code: exitUsage, Err: fmt.Errorf("publish --out %q: %w", outPath, err)}
		}
	}
	before, after := meanHeldOutCER(report)
	_, writeErr := fmt.Fprintf(stdout, "ocr: %d seeds, held-out CER %.4f -> %.4f, core disconnect %t\n",
		len(report.Seeds), before, after, report.CoreDisconnect.OutputChanged)
	return writeErr
}

// ocrFixtureConfig is the fixed fixture configuration of this example. It is a
// constant, not a flag surface: changing any field makes a different protocol,
// and the report carries the configuration and its hash so a reader can tell
// which one produced the numbers.
func ocrFixtureConfig() ocr.FixtureConfig {
	return ocr.FixtureConfig{
		Seeds:        []uint64{1, 2, 3},
		Alphabet:     []rune{'a', 'b', 'c'},
		TrainSamples: 40,
		TestSamples:  15,
		MinLen:       1,
		MaxLen:       3,
		Epochs:       3,
		Hidden:       24,
		LearningRate: 0.05,
		TrainFamily:  1,
		TestFamily:   2,
		Background:   0.1,
		Noise:        0.05,
	}
}

// meanHeldOutCER averages the held-out character error rate before and after
// training over the seeds that finished and whose rate is defined. A run whose
// every seed failed reports 0 for both, which the summary shows beside the
// failure count the report itself carries.
func meanHeldOutCER(report ocr.OCRReport) (before, after float64) {
	beforeN, afterN := 0, 0
	for _, s := range report.Seeds {
		if s.Failed {
			continue
		}
		if s.Before.CER.Defined {
			before += s.Before.CER.Rate
			beforeN++
		}
		if s.After.CER.Defined {
			after += s.After.CER.Rate
			afterN++
		}
	}
	if beforeN > 0 {
		before /= float64(beforeN)
	}
	if afterN > 0 {
		after /= float64(afterN)
	}
	return before, after
}
