package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/TimLai666/coimnet/tasks/media"
)

func runRoles(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	c := media.DefaultRolesConfig()
	seedsList, outPath := formatSeeds(c.Seeds), ""
	fs := flag.NewFlagSet("examples run roles", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&seedsList, "seeds", seedsList, "comma-separated distinct seeds")
	fs.IntVar(&c.Epochs, "epochs", c.Epochs, "training epochs per seed")
	fs.IntVar(&c.MaxToolCalls, "max-tool-calls", c.MaxToolCalls, "maximum external tool calls per seed")
	fs.StringVar(&outPath, "out", outPath, "new report JSON file (required)")
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: coimnet examples run roles [--seeds 1,2,3] [--epochs N] [--max-tool-calls N] --out FILE")
		fmt.Fprintln(usageOutput, "Compares three ways of producing the nine fixture images from the same prompt: core_generated uses the core's own pixels through a fixed clamp; fixed_decoder has the core produce a latent that a frozen, never-trained autoencoder decodes, named by its hash; external_tool has the core emit a structured request that an allow-listed local stand-in tool renders under a call budget. Only what the core itself produced counts as core capability. Tool pixels are reported separately and never added to it. No network or third-party model is called.")
		fmt.Fprintln(usageOutput, "Writes an indented JSON report to the new --out file and prints one summary line.")
		fmt.Fprintln(usageOutput, "Example: coimnet examples run roles --seeds 1,2,3 --epochs 60 --max-tool-calls 9 --out roles.json")
		fmt.Fprintln(usageOutput, "Errors: missing or existing --out, invalid --seeds, --epochs or --max-tool-calls, positional arguments, cancellation or output failure. Usage errors exit with status 1. Failed seeds still write the report and return a nonzero status.")
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
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("examples run roles takes no positional arguments; use examples run roles --help")}
	}
	if outPath == "" {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--out is required")}
	}
	if err := refuseRolesOutput(outPath); err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	seeds, err := parseExampleSeeds(seedsList)
	if err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	c.Seeds = seeds
	if err := c.Validate(); err != nil {
		flagName := "--epochs"
		switch {
		case strings.Contains(err.Error(), "seeds"):
			flagName = "--seeds"
		case strings.Contains(err.Error(), "tool") || strings.Contains(err.Error(), "calls"):
			flagName = "--max-tool-calls"
		}
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("%s: %w", flagName, err)}
	}

	report, err := media.RunRoles(ctx, c)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode roles report: %w", err)
	}
	if err := writeNewTextFile(outPath, string(data)+"\n"); err != nil {
		return fmt.Errorf("write roles report: %w", err)
	}
	if err := writeRolesSummary(stdout, report); err != nil {
		return err
	}
	failed := 0
	for _, run := range report.Runs {
		if run.Failed {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d roles runs failed; see report", failed)
	}
	return nil
}

func refuseRolesOutput(path string) error {
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("--out %q already exists; choose a new path", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("--out path: %w", err)
	}
	parent := filepath.Dir(path)
	info, err := os.Stat(parent)
	if err != nil {
		return fmt.Errorf("--out parent: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("--out parent %q is not a directory", parent)
	}
	return nil
}

func writeRolesSummary(w io.Writer, report media.RolesReport) error {
	successful := 0
	for _, run := range report.Runs {
		if !run.Failed {
			successful++
		}
	}
	seen, heldOut := 0, 0
	if len(report.Runs) > 0 {
		for _, score := range report.Runs[0].CoreGenerated.Scores {
			if score.Seen {
				seen++
			} else {
				heldOut++
			}
		}
	}
	coreSeen, coreHeldOut := 0, 0
	decoderSeen, decoderHeldOut := 0, 0
	requestSeen, requestHeldOut := 0, 0
	pixelsSeen, pixelsHeldOut := 0, 0
	calls, budgetRefused := 0, 0
	decoderHash := ""
	for _, run := range report.Runs {
		if !run.Failed {
			coreSeen += run.CoreGenerated.CoreCapability.SeenCorrect
			coreHeldOut += run.CoreGenerated.CoreCapability.HeldOutCorrect
			decoderSeen += run.FixedDecoder.CoreCapability.SeenCorrect
			decoderHeldOut += run.FixedDecoder.CoreCapability.HeldOutCorrect
			requestSeen += run.ExternalTool.CoreCapability.SeenCorrect
			requestHeldOut += run.ExternalTool.CoreCapability.HeldOutCorrect
			if run.ExternalTool.ToolResult != nil {
				pixelsSeen += run.ExternalTool.ToolResult.SeenCorrect
				pixelsHeldOut += run.ExternalTool.ToolResult.HeldOutCorrect
			}
			if run.ExternalTool.ToolCounts != nil {
				calls += run.ExternalTool.ToolCounts.Calls
				budgetRefused += run.ExternalTool.ToolCounts.BudgetRefused
			}
		}
		if decoderHash == "" {
			decoderHash = run.FixedDecoder.DecoderHash
		}
	}
	if len(decoderHash) > 12 {
		decoderHash = decoderHash[:12]
	}
	avg := func(total int) float64 {
		if successful == 0 {
			return 0
		}
		return float64(total) / float64(successful)
	}
	_, err := fmt.Fprintf(w, "roles: %d seeds; core_generated seen %.2f of %d, held-out %.2f of %d; fixed_decoder seen %.2f of %d, held-out %.2f of %d (decoder %s); external_tool requests seen %.2f of %d, held-out %.2f of %d; tool pixels seen %.2f, held-out %.2f (not core capability); tool calls %d, budget refused %d\n", len(report.Runs), avg(coreSeen), seen, avg(coreHeldOut), heldOut, avg(decoderSeen), seen, avg(decoderHeldOut), heldOut, decoderHash, avg(requestSeen), seen, avg(requestHeldOut), heldOut, avg(pixelsSeen), avg(pixelsHeldOut), calls, budgetRefused)
	return err
}
