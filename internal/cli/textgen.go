package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/TimLai666/coimnet/tasks/textgen"
)

func runTextgen(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	c := textgen.DefaultRunConfig()
	seedsList := formatSeeds(c.Seeds)
	outPath := ""
	fs := flag.NewFlagSet("examples run textgen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&seedsList, "seeds", seedsList, "comma-separated distinct seeds")
	fs.IntVar(&c.Epochs, "epochs", c.Epochs, "training epochs per seed (1..1000)")
	fs.StringVar(&c.Corpus, "corpus", c.Corpus, "licensed text corpus manifest (default: synthetic fixture)")
	fs.StringVar(&outPath, "out", "", "new report JSON file; an existing path is never overwritten (default: stdout)")
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: coimnet examples run textgen [--seeds 1,2,3] [--epochs N] [--corpus MANIFEST] [--out FILE]")
		fmt.Fprintln(usageOutput, "Runs byte-level language model training and student-mode evaluation on the synthetic grammar fixture or a licensed manifest corpus.")
		fmt.Fprintln(usageOutput, "The fixture corpus is a synthetic grammar, so the run proves the pipeline, not Chinese dialogue, reasoning or knowledge.")
		fmt.Fprintln(usageOutput, "Perplexity is comparable only with the same vocab_hash. Evaluation runs in student mode with the teacher blocked.")
		fmt.Fprintln(usageOutput, "--corpus reads a licensed manifest (every document needs holder, terms and source). The indented JSON report goes to --out, or stdout when omitted; the summary goes to stdout with --out and stderr otherwise.")
		fmt.Fprintln(usageOutput, "Example: coimnet examples run textgen --seeds 1 --epochs 10 --out textgen.json")
		fmt.Fprintln(usageOutput, "Errors: invalid --seeds or --epochs, invalid manifest, existing or unwritable --out, positional arguments, cancellation or output failure. Failed seeds still emit the full report and return a nonzero status.")
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
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("examples run textgen takes no positional arguments; use examples run textgen --help")}
	}
	seeds, err := parseExampleSeeds(seedsList)
	if err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	c.Seeds = seeds
	if err := c.Validate(); err != nil {
		flagName := "--epochs"
		if strings.HasPrefix(err.Error(), "seeds ") {
			flagName = "--seeds"
		}
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("%s: %w", flagName, err)}
	}
	if outPath != "" {
		if err := refuseExistingOut(outPath); err != nil {
			return &ExitError{Code: exitUsage, Err: err}
		}
	}
	report, err := textgen.RunTextgen(ctx, c)
	if err != nil {
		if c.Corpus != "" {
			return fmt.Errorf("read --corpus %q: %w", c.Corpus, err)
		}
		return err
	}
	if outPath == "" {
		if err := writeJSON(stdout, report); err != nil {
			return err
		}
	} else {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("encode textgen report: %w", err)
		}
		if err := writeNewTextFile(outPath, string(data)+"\n"); err != nil {
			return &ExitError{Code: exitUsage, Err: fmt.Errorf("publish --out %q: %w", outPath, err)}
		}
	}
	var perplexityBefore, perplexityAfter, taskBefore, taskAfter float64
	teacherCalls, failed := 0, 0
	for _, run := range report.Runs {
		perplexityBefore += run.HoldoutBefore.Perplexity
		perplexityAfter += run.HoldoutAfter.Perplexity
		taskBefore += run.TaskBefore.Accuracy
		taskAfter += run.TaskAfter.Accuracy
		teacherCalls += run.TeacherCalls
		if run.Failed {
			failed++
		}
	}
	count := float64(len(report.Runs))
	if count > 0 {
		perplexityBefore /= count
		perplexityAfter /= count
		taskBefore /= count
		taskAfter /= count
	}
	summaryOut := stdout
	if outPath == "" {
		summaryOut = stderr
	}
	if _, err := fmt.Fprintf(summaryOut, "textgen: %d seeds, holdout perplexity %.4f -> %.4f, task accuracy %.4f -> %.4f, teacher calls %d\n",
		len(report.Runs), perplexityBefore, perplexityAfter, taskBefore, taskAfter, teacherCalls); err != nil {
		return err
	}
	if failed > 0 {
		return fmt.Errorf("%d textgen seeds failed; see report", failed)
	}
	return nil
}
