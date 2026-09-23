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

func runMedia(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	modality, seedsList, outDir := "", "1,2,3", ""
	epochs := 60
	var c media.RunConfig
	fs := flag.NewFlagSet("examples run media", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&modality, "modality", modality, "generation modality: image or audio (required)")
	fs.StringVar(&seedsList, "seeds", seedsList, "comma-separated distinct seeds")
	fs.IntVar(&epochs, "epochs", epochs, "training epochs per seed")
	fs.StringVar(&outDir, "out-dir", outDir, "new output directory for report and samples (required)")
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: coimnet examples run media --modality image|audio [--seeds 1,2,3] [--epochs N] --out-dir DIR")
		fmt.Fprintln(usageOutput, "Runs the finite synthetic image or audio generation fixture. The core generates a representation that a fixed decoder clamps into pixels or samples; the decoder never sees the prompt. Held-out prompts never train. frozen_core shows what the periphery alone achieves. Pixel or sample MSE says nothing about perceptual quality.")
		fmt.Fprintln(usageOutput, "Writes an indented JSON report to DIR/report.json and generated PNG or WAV samples to DIR/samples/. The output directory must not exist.")
		fmt.Fprintln(usageOutput, "Example: coimnet examples run media --modality image --seeds 1,2,3 --epochs 60 --out-dir media-output")
		fmt.Fprintln(usageOutput, "Errors: missing or unknown modality, missing or existing --out-dir, invalid --seeds or --epochs, positional arguments, cancellation or output failure. Usage errors exit with status 1. Failed seeds still write the report and return a nonzero status.")
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
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("examples run media takes no positional arguments; use examples run media --help")}
	}
	if modality != media.ModalityImage && modality != media.ModalityAudio {
		if modality == "" {
			return &ExitError{Code: exitUsage, Err: fmt.Errorf("--modality is required")}
		}
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("unknown --modality %q; use image or audio", modality)}
	}
	if outDir == "" {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--out-dir is required")}
	}
	if err := refuseMediaOutputDir(outDir); err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	c = media.DefaultRunConfig(modality)
	c.Epochs = epochs
	seeds, err := parseExampleSeeds(seedsList)
	if err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	c.Seeds = seeds
	if err := c.Validate(); err != nil {
		flagName := "--epochs"
		if strings.Contains(err.Error(), "seeds") {
			flagName = "--seeds"
		}
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("%s: %w", flagName, err)}
	}
	if err := os.Mkdir(outDir, 0o755); err != nil {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("create --out-dir %q: %w", outDir, err)}
	}

	report, err := media.RunMedia(ctx, c)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode media report: %w", err)
	}
	if err := writeNewTextFile(filepath.Join(outDir, "report.json"), string(data)+"\n"); err != nil {
		return fmt.Errorf("write media report: %w", err)
	}
	failed := 0
	for _, run := range report.Runs {
		if run.Failed {
			failed++
		}
	}
	if _, err := media.WriteSamples(ctx, filepath.Join(outDir, "samples"), c, c.Seeds[0]); err != nil && failed == 0 {
		return fmt.Errorf("write media samples: %w", err)
	}

	coreSeen, frozenSeen, coreHeldOut, disconnected := 0, 0, 0, 0
	for _, run := range report.Runs {
		coreSeen += run.Core.SeenCorrect
		frozenSeen += run.FrozenCore.SeenCorrect
		coreHeldOut += run.Core.HeldOutCorrect
		if run.CoreDisconnect.OutputChanged {
			disconnected++
		}
	}
	seedCount := float64(len(report.Runs))
	seenCount := 9 - len(c.Holdout)
	heldOutCount := len(c.Holdout)
	if _, err := fmt.Fprintf(stdout, "media: %s, %d seeds, seen correct %.2f of %d (frozen core %.2f), held-out correct %.2f of %d, core disconnect changed %d/%d\n", modality, len(report.Runs), float64(coreSeen)/seedCount, seenCount, float64(frozenSeen)/seedCount, float64(coreHeldOut)/seedCount, heldOutCount, disconnected, len(report.Runs)); err != nil {
		return err
	}
	if failed > 0 {
		return fmt.Errorf("%d media runs failed; see report", failed)
	}
	return nil
}

func refuseMediaOutputDir(path string) error {
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("--out-dir %q already exists; choose a new path", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("--out-dir path: %w", err)
	}
	parent := filepath.Dir(path)
	info, err := os.Stat(parent)
	if err != nil {
		return fmt.Errorf("--out-dir parent: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("--out-dir parent %q is not a directory", parent)
	}
	return nil
}
