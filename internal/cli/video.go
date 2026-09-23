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
	"sort"
	"strings"

	"github.com/TimLai666/coimnet/tasks/media"
)

func runVideo(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	config := media.DefaultVideoRunConfig()
	seedsList, outDir, packager := "1,2,3", "", "ffmpeg"
	epochs := config.Epochs
	fs := flag.NewFlagSet("examples run video", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&seedsList, "seeds", seedsList, "comma-separated distinct seeds")
	fs.IntVar(&epochs, "epochs", epochs, "training epochs per seed")
	fs.StringVar(&packager, "packager", packager, "optional video packager executable, or none to skip packaging")
	fs.StringVar(&outDir, "out-dir", outDir, "new output directory for report, clips and packaging (required)")
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: coimnet examples run video [--seeds 1,2,3] [--epochs N] [--packager ffmpeg] --out-dir DIR")
		fmt.Fprintln(usageOutput, "The core generates every frame and its audio block from the prompt through a fixed clamp decoder. Held-out prompts never train. Sync error is the frame distance between the dot reaching the edge and the beep. Packaging is optional; its absence or failure is recorded and never reported as success. The native frames, audio and timeline are always complete.")
		fmt.Fprintln(usageOutput, "Writes an indented JSON report to DIR/report.json, native clips to DIR/clips/, and packaging results to DIR/packaging.json when packaging is enabled. The output directory must not exist.")
		fmt.Fprintln(usageOutput, "Example: coimnet examples run video --seeds 1,2,3 --epochs 80 --packager ffmpeg --out-dir video-output")
		fmt.Fprintln(usageOutput, "Errors: missing or existing --out-dir, invalid --seeds or --epochs, positional arguments, cancellation or output failure. Usage errors exit with status 1. Failed seeds still write the report and return a nonzero status.")
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
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("examples run video takes no positional arguments; use examples run video --help")}
	}
	if outDir == "" {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("--out-dir is required")}
	}
	if err := refuseMediaOutputDir(outDir); err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	seeds, err := parseExampleSeeds(seedsList)
	if err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	config.Epochs = epochs
	config.Seeds = seeds
	if err := config.Validate(); err != nil {
		flagName := "--epochs"
		if strings.Contains(err.Error(), "seeds") {
			flagName = "--seeds"
		}
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("%s: %w", flagName, err)}
	}
	if err := os.Mkdir(outDir, 0o755); err != nil {
		return fmt.Errorf("create --out-dir %q: %w", outDir, err)
	}

	report, err := media.RunVideo(ctx, config)
	if err != nil {
		return err
	}
	reportJSON, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode video report: %w", err)
	}
	if err := writeNewTextFile(filepath.Join(outDir, "report.json"), string(reportJSON)+"\n"); err != nil {
		return fmt.Errorf("write video report: %w", err)
	}

	timelines, err := media.WriteVideoSamples(ctx, filepath.Join(outDir, "clips"), config, config.Seeds[0])
	if err != nil {
		return fmt.Errorf("write video samples: %w", err)
	}
	packaged, absent, failed := 0, 0, 0
	if packager != "none" {
		clipNames := make([]string, 0, len(timelines))
		for name := range timelines {
			clipNames = append(clipNames, name)
		}
		sort.Strings(clipNames)
		packaging := make(map[string]media.PackageReport, len(clipNames))
		for _, name := range clipNames {
			result, err := media.PackageVideo(ctx, filepath.Join(outDir, "clips", name), packager)
			if err != nil {
				return fmt.Errorf("package video %s: %w", name, err)
			}
			packaging[name] = result
			switch result.Status {
			case "packaged":
				packaged++
			case "tool_absent":
				absent++
			case "tool_failed":
				failed++
			}
		}
		packagingJSON, err := json.MarshalIndent(struct {
			Packager map[string]media.PackageReport `json:"packager"`
		}{Packager: packaging}, "", "  ")
		if err != nil {
			return fmt.Errorf("encode video packaging report: %w", err)
		}
		if err := writeNewTextFile(filepath.Join(outDir, "packaging.json"), string(packagingJSON)+"\n"); err != nil {
			return fmt.Errorf("write video packaging report: %w", err)
		}
	}

	seenCount, heldOutCount := 0, 0
	for _, score := range report.Runs[0].Core.Before {
		if score.Seen {
			seenCount++
		} else {
			heldOutCount++
		}
	}
	coreSeen, frozenSeen, coreHeldOut, coreInSync, disconnected := 0, 0, 0, 0, 0
	for _, run := range report.Runs {
		coreSeen += videoSeenCorrect(run.Core.After)
		frozenSeen += videoSeenCorrect(run.FrozenCore.After)
		coreHeldOut += videoHeldOutCorrect(run.Core.After)
		coreInSync += videoSeenInSync(run.Core.After)
		if run.CoreDisconnect.OutputChanged {
			disconnected++
		}
	}
	seedCount := float64(len(report.Runs))
	var summaryErr error
	if packager == "none" {
		_, summaryErr = fmt.Fprintf(stdout, "video: %d seeds, seen correct %.2f of %d (frozen core %.2f), held-out correct %.2f of %d, seen in sync %.2f of %d, core disconnect changed %d/%d, packaging skipped\n",
			len(report.Runs), float64(coreSeen)/seedCount, seenCount, float64(frozenSeen)/seedCount,
			float64(coreHeldOut)/seedCount, heldOutCount, float64(coreInSync)/seedCount, seenCount,
			disconnected, len(report.Runs))
	} else {
		_, summaryErr = fmt.Fprintf(stdout, "video: %d seeds, seen correct %.2f of %d (frozen core %.2f), held-out correct %.2f of %d, seen in sync %.2f of %d, core disconnect changed %d/%d, packaged %d/%d (%d absent, %d failed)\n",
			len(report.Runs), float64(coreSeen)/seedCount, seenCount, float64(frozenSeen)/seedCount,
			float64(coreHeldOut)/seedCount, heldOutCount, float64(coreInSync)/seedCount, seenCount,
			disconnected, len(report.Runs), packaged, len(timelines), absent, failed)
	}
	if summaryErr != nil {
		return summaryErr
	}
	for _, run := range report.Runs {
		if run.Failed {
			return fmt.Errorf("%d video runs failed; see report", countFailedVideoRuns(report.Runs))
		}
	}
	return nil
}

func videoSeenCorrect(scores []media.VideoScore) int {
	count := 0
	for _, score := range scores {
		if score.Seen && score.Correct {
			count++
		}
	}
	return count
}

func videoHeldOutCorrect(scores []media.VideoScore) int {
	count := 0
	for _, score := range scores {
		if !score.Seen && score.Correct {
			count++
		}
	}
	return count
}

func videoSeenInSync(scores []media.VideoScore) int {
	count := 0
	for _, score := range scores {
		if score.Seen && score.SyncError == 0 {
			count++
		}
	}
	return count
}

func countFailedVideoRuns(runs []media.VideoRunSeed) int {
	count := 0
	for _, run := range runs {
		if run.Failed {
			count++
		}
	}
	return count
}
