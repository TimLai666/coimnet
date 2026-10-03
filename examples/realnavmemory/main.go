// Command realnavmemory is a source-pinned engineering example for measuring
// whether a causal trajectory model uses recorded stimulus history. It does
// not alter the existing examples/realnav snapshot contract.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"syscall"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/nav2d/trajectory"
)

const memoryInferenceSchema = "coimnet-realnav-memory-inference/v1"

type memoryMetricSet struct {
	Train      metric `json:"train"`
	Validation metric `json:"validation"`
	Test       metric `json:"test"`
}

type memoryInferenceRun struct {
	Seed    uint64          `json:"seed"`
	Control string          `json:"control"`
	Before  memoryMetricSet `json:"before"`
	After   memoryMetricSet `json:"after"`
}

type memoryInferenceReport struct {
	SchemaVersion   string               `json:"schema_version"`
	Mode            string               `json:"mode"`
	Source          trajectory.Source    `json:"source"`
	SourceSHA256    string               `json:"source_sha256"`
	PlanFingerprint string               `json:"plan_fingerprint"`
	Split           trialSplit           `json:"split"`
	Results         []memoryInferenceRun `json:"results"`
	Limitations     []string             `json:"limitations"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if ctx == nil || stdout == nil || stderr == nil {
		return errors.New("realnavmemory: context and output writers are required")
	}
	if len(args) == 0 {
		writeMemoryUsage(stdout)
		return nil
	}
	if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		if len(args) != 1 {
			return errors.New("realnavmemory: help does not accept positional arguments")
		}
		writeMemoryUsage(stdout)
		return nil
	}
	switch args[0] {
	case "prepare":
		return runMemoryPrepare(ctx, args[1:], stdout, stderr)
	case "train":
		return runMemoryTrain(ctx, args[1:], stdout, stderr)
	case "infer":
		return runMemoryInfer(ctx, args[1:], stdout, stderr)
	case "rollout":
		return runMemoryRollout(ctx, args[1:], stdout, stderr)
	case "counterfactual":
		return runMemoryCounterfactual(ctx, args[1:], stdout, stderr)
	default:
		return fmt.Errorf("realnavmemory: unknown command %q; use prepare, train, infer, rollout, or counterfactual", args[0])
	}
}

func writeMemoryUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  realnavmemory prepare --input CSV --base-model V1_JSON --out-dir NEW")
	fmt.Fprintln(w, "  realnavmemory train   --input CSV --plan PLAN_JSON --out-dir NEW")
	fmt.Fprintln(w, "  realnavmemory infer   --input CSV --model BUNDLE_JSON --out-dir NEW")
	fmt.Fprintln(w, "  realnavmemory rollout --input CSV --model BUNDLE_JSON --out-dir NEW")
	fmt.Fprintln(w, "  realnavmemory counterfactual --input CSV --model BUNDLE_JSON --out-dir NEW")
	fmt.Fprintln(w, "All inputs are validated before the new output directory is created.")
}

func runMemoryPrepare(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("realnavmemory prepare", flag.ContinueOnError)
	fs.SetOutput(stderr)
	inputPath := fs.String("input", "", "source-pinned trajectory CSV or CSV.gz")
	basePath := fs.String("base-model", "", "existing realnav v1 model JSON")
	outPath := fs.String("out-dir", "", "new output directory")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			writeMemoryUsage(stdout)
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("realnavmemory prepare: unexpected positional arguments")
	}
	if *inputPath == "" || *basePath == "" || *outPath == "" {
		return errors.New("realnavmemory prepare: --input, --base-model and --out-dir are required")
	}
	trials, err := importHistory(ctx, *inputPath)
	if err != nil {
		return err
	}
	base, baseSHA, err := readMemoryBaseModel(*basePath)
	if err != nil {
		return err
	}
	if len(trials) != 39 {
		return fmt.Errorf("realnavmemory: source must contain 39 trials, got %d", len(trials))
	}
	split, err := makeSplit(trials, base.Split.Train, base.Split.Test)
	if err != nil {
		return fmt.Errorf("realnavmemory: make split: %w", err)
	}
	plan := newMemoryPlan(realSource(), base, baseSHA, split)
	if err := validateMemoryPlan(plan, trials); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	output, err := createMemoryOutput(*outPath, *inputPath, *basePath)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			cleanupMemoryOutput(output)
		}
	}()
	if err := writeMemoryJSONExclusive(filepathJoin(output, "plan.json"), plan); err != nil {
		return err
	}
	committed = true
	return encodeMemoryJSON(stdout, plan)
}

func runMemoryTrain(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("realnavmemory train", flag.ContinueOnError)
	fs.SetOutput(stderr)
	inputPath := fs.String("input", "", "source-pinned trajectory CSV or CSV.gz")
	planPath := fs.String("plan", "", "plan.json produced by prepare")
	outPath := fs.String("out-dir", "", "new output directory")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			writeMemoryUsage(stdout)
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("realnavmemory train: unexpected positional arguments")
	}
	if *inputPath == "" || *planPath == "" || *outPath == "" {
		return errors.New("realnavmemory train: --input, --plan and --out-dir are required")
	}
	var plan memoryPlan
	if err := readMemoryJSON(*planPath, &plan); err != nil {
		return err
	}
	trials, err := importHistory(ctx, *inputPath)
	if err != nil {
		return err
	}
	if err := validateMemoryPlan(plan, trials); err != nil {
		return err
	}
	trainTrials, err := selectTrials(trials, plan.Split.Train)
	if err != nil {
		return err
	}
	meanStepLen, err := meanLegalStepLength(trainTrials)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	output, err := createMemoryOutput(*outPath, *inputPath, *planPath)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			cleanupMemoryOutput(output)
		}
	}()
	runs := make([]memoryRun, 0, len(memorySeeds)*len(memoryControls))
	for _, seed := range plan.Seeds {
		for _, control := range plan.Controls {
			if err := ctx.Err(); err != nil {
				return err
			}
			before, after, curve, err := fitModel(ctx, trainTrials, control, int64(seed), plan.Epochs)
			if err != nil {
				return fmt.Errorf("realnavmemory: fit seed %d control %s: %w", seed, control, err)
			}
			trainer, err := restoreMemoryTrainer(after)
			if err != nil {
				return fmt.Errorf("realnavmemory: seed %d control %s snapshot: %w", seed, control, err)
			}
			run := memoryRun{Seed: seed, Control: control, Before: before, After: after, Curve: append([]float64(nil), curve...), Capacity: trainer.Capacity(), MeanStepLen: meanStepLen, Updates: after.Updates}
			if err := validateMemoryRun(run, plan); err != nil {
				return fmt.Errorf("realnavmemory: seed %d control %s: %w", seed, control, err)
			}
			runs = append(runs, run)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	bundle := bundleFromPlan(plan, runs)
	if err := validateMemoryBundle(bundle, trials); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := writeMemoryJSONExclusive(filepathJoin(output, "bundle.json"), bundle); err != nil {
		return err
	}
	committed = true
	return encodeMemoryJSON(stdout, bundle)
}

func runMemoryInfer(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("realnavmemory infer", flag.ContinueOnError)
	fs.SetOutput(stderr)
	inputPath := fs.String("input", "", "source-pinned trajectory CSV or CSV.gz")
	modelPath := fs.String("model", "", "bundle.json produced by train")
	outPath := fs.String("out-dir", "", "new output directory")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			writeMemoryUsage(stdout)
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("realnavmemory infer: unexpected positional arguments")
	}
	if *inputPath == "" || *modelPath == "" || *outPath == "" {
		return errors.New("realnavmemory infer: --input, --model and --out-dir are required")
	}
	var bundle memoryBundle
	if err := readMemoryJSON(*modelPath, &bundle); err != nil {
		return err
	}
	trials, err := importHistory(ctx, *inputPath)
	if err != nil {
		return err
	}
	if err := validateMemoryBundle(bundle, trials); err != nil {
		return err
	}
	report, err := buildMemoryInferenceReport(ctx, bundle, trials)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	output, err := createMemoryOutput(*outPath, *inputPath, *modelPath)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			cleanupMemoryOutput(output)
		}
	}()
	if err := writeMemoryJSONExclusive(filepathJoin(output, "report.json"), report); err != nil {
		return err
	}
	committed = true
	return encodeMemoryJSON(stdout, report)
}

func runMemoryRollout(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("realnavmemory rollout", flag.ContinueOnError)
	fs.SetOutput(stderr)
	inputPath := fs.String("input", "", "source-pinned trajectory CSV or CSV.gz")
	modelPath := fs.String("model", "", "bundle.json produced by train")
	outPath := fs.String("out-dir", "", "new output directory")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			writeMemoryUsage(stdout)
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("realnavmemory rollout: unexpected positional arguments")
	}
	if *inputPath == "" || *modelPath == "" || *outPath == "" {
		return errors.New("realnavmemory rollout: --input, --model and --out-dir are required")
	}
	var bundle memoryBundle
	if err := readMemoryJSON(*modelPath, &bundle); err != nil {
		return err
	}
	trials, err := importHistory(ctx, *inputPath)
	if err != nil {
		return err
	}
	if err := validateMemoryBundle(bundle, trials); err != nil {
		return err
	}
	_, targets, err := trajectory.ReadWithReturnTargets(ctx, *inputPath, realSource(), trajectory.DefaultLimits())
	if err != nil {
		return fmt.Errorf("realnavmemory: read evaluation targets: %w", err)
	}
	report, err := buildMemoryRolloutReport(ctx, bundle, trials, targets)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	output, err := createMemoryOutput(*outPath, *inputPath, *modelPath)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			cleanupMemoryOutput(output)
		}
	}()
	if err := writeMemoryJSONExclusive(filepathJoin(output, "report.json"), report); err != nil {
		return err
	}
	committed = true
	return encodeMemoryJSON(stdout, report)
}

func buildMemoryInferenceReport(ctx context.Context, bundle memoryBundle, trials []historyTrial) (memoryInferenceReport, error) {
	trainTrials, err := selectTrials(trials, bundle.Split.Train)
	if err != nil {
		return memoryInferenceReport{}, err
	}
	validationTrials, err := selectTrials(trials, bundle.Split.Validation)
	if err != nil {
		return memoryInferenceReport{}, err
	}
	testTrials, err := selectTrials(trials, bundle.Split.Test)
	if err != nil {
		return memoryInferenceReport{}, err
	}
	results := make([]memoryInferenceRun, 0, len(bundle.Runs))
	for _, run := range bundle.Runs {
		if err := ctx.Err(); err != nil {
			return memoryInferenceReport{}, err
		}
		beforeTrain, err := evaluateModel(ctx, run.Before, trainTrials, run.Control, int64(run.Seed))
		if err != nil {
			return memoryInferenceReport{}, fmt.Errorf("evaluate before train %d/%s: %w", run.Seed, run.Control, err)
		}
		beforeValidation, err := evaluateModel(ctx, run.Before, validationTrials, run.Control, int64(run.Seed))
		if err != nil {
			return memoryInferenceReport{}, fmt.Errorf("evaluate before validation %d/%s: %w", run.Seed, run.Control, err)
		}
		beforeTest, err := evaluateModel(ctx, run.Before, testTrials, run.Control, int64(run.Seed))
		if err != nil {
			return memoryInferenceReport{}, fmt.Errorf("evaluate before test %d/%s: %w", run.Seed, run.Control, err)
		}
		afterTrain, err := evaluateModel(ctx, run.After, trainTrials, run.Control, int64(run.Seed))
		if err != nil {
			return memoryInferenceReport{}, fmt.Errorf("evaluate after train %d/%s: %w", run.Seed, run.Control, err)
		}
		afterValidation, err := evaluateModel(ctx, run.After, validationTrials, run.Control, int64(run.Seed))
		if err != nil {
			return memoryInferenceReport{}, fmt.Errorf("evaluate after validation %d/%s: %w", run.Seed, run.Control, err)
		}
		afterTest, err := evaluateModel(ctx, run.After, testTrials, run.Control, int64(run.Seed))
		if err != nil {
			return memoryInferenceReport{}, fmt.Errorf("evaluate after test %d/%s: %w", run.Seed, run.Control, err)
		}
		results = append(results, memoryInferenceRun{Seed: run.Seed, Control: run.Control, Before: memoryMetricSet{Train: beforeTrain, Validation: beforeValidation, Test: beforeTest}, After: memoryMetricSet{Train: afterTrain, Validation: afterValidation, Test: afterTest}})
	}
	return memoryInferenceReport{
		SchemaVersion:   memoryInferenceSchema,
		Mode:            "infer",
		Source:          bundle.Source,
		SourceSHA256:    bundle.SourceSHA256,
		PlanFingerprint: bundle.PlanFingerprint,
		Split:           copyTrialSplit(bundle.Split),
		Results:         results,
		Limitations: []string{
			"Metrics are teacher-forced engineering-model metrics; they do not establish a biological or connectome mechanism.",
			"For each current observation, rows after that observation are not used to build its input; the next displacement is used only as a loss label.",
		},
	}, nil
}

func meanLegalStepLength(trials []historyTrial) (float64, error) {
	var sum float64
	var count int
	for _, trial := range trials {
		for i, step := range trial.Steps {
			if !step.Scored {
				continue
			}
			if i <= 0 || i >= len(trial.Steps)-1 || !finite(step.Target[0]) || !finite(step.Target[1]) {
				return 0, fmt.Errorf("realnavmemory: illegal scored target in trial %q row %d", trial.ID, i)
			}
			length := math.Hypot(step.Target[0], step.Target[1])
			if !finite(length) {
				return 0, fmt.Errorf("realnavmemory: non-finite legal step length in trial %q row %d", trial.ID, i)
			}
			sum += length
			count++
		}
	}
	if count == 0 {
		return 0, errors.New("realnavmemory: training split has no legal scored labels")
	}
	mean := sum / float64(count)
	if !finite(mean) || mean < 0 {
		return 0, errors.New("realnavmemory: training mean step length is invalid")
	}
	return mean, nil
}

func restoreMemoryTrainer(snapshot learning.TrainingSnapshot) (*learning.Trainer, error) {
	return learning.RestoreTrainer(snapshot)
}

func encodeMemoryJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

// filepathJoin is kept as a tiny wrapper so all output paths in this command
// are constructed in one place and tests do not need platform-specific joins.
func filepathJoin(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	path := parts[0]
	for _, part := range parts[1:] {
		if path == "" {
			path = part
		} else {
			path += string(os.PathSeparator) + part
		}
	}
	return path
}
