// Command realnav imports a licensed fruit-fly trajectory dataset, trains a
// small continuous sparse displacement predictor, and evaluates a frozen
// snapshot on trial-held-out trajectories. It never downloads data.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	osSignal "os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/TimLai666/coimnet/tasks/nav2d/trajectory"
)

const (
	realnavSchemaVersion = "coimnet-realnav-example/v1"
	testFraction         = 1.0 / 3.0
	splitSeed            = uint64(20260925)
	defaultEpochs        = 2
)

type report struct {
	SchemaVersion string         `json:"schema_version"`
	Mode          string         `json:"mode"`
	Source        sourceReport   `json:"source"`
	Split         splitReport    `json:"split"`
	Filtering     filterReport   `json:"filtering"`
	Training      trainingReport `json:"training"`
	Metrics       metricReport   `json:"metrics"`
	Runtime       runtimeReport  `json:"runtime"`
	Limitations   []string       `json:"limitations"`
}

type sourceReport struct {
	ID                   string             `json:"id"`
	URL                  string             `json:"url"`
	DOI                  string             `json:"doi"`
	PinnedCommit         string             `json:"pinned_commit"`
	SHA256               string             `json:"sha256"`
	AuthorGitBlobSHA1    string             `json:"author_git_blob_sha1"`
	AuthorReadmeBlobSHA1 string             `json:"author_readme_blob_sha1"`
	DryadByteEquivalence string             `json:"dryad_byte_equivalence"`
	License              trajectory.License `json:"license"`
	Rows                 int                `json:"rows"`
	Trials               int                `json:"trials"`
}

type splitReport struct {
	Seed              uint64         `json:"seed"`
	TestFraction      float64        `json:"test_fraction"`
	TrainTrialIDs     []string       `json:"train_trial_ids"`
	TestTrialIDs      []string       `json:"test_trial_ids"`
	TrainTrialCounts  map[string]int `json:"train_trial_counts"`
	TestTrialCounts   map[string]int `json:"test_trial_counts"`
	TrainSampleCounts map[string]int `json:"train_sample_counts"`
	TestSampleCounts  map[string]int `json:"test_sample_counts"`
}

type filterReport struct {
	RuleVersion       string   `json:"rule_version"`
	Segment           string   `json:"segment"`
	MaxDeltaTSeconds  float64  `json:"max_delta_t_seconds"`
	MaxStepDistanceCM float64  `json:"max_step_distance_cm"`
	InputFeatures     []string `json:"input_features"`
	Target            []string `json:"target"`
	ForbiddenFeatures []string `json:"forbidden_features"`
}

type trainingReport struct {
	Epochs                   int     `json:"epochs"`
	Updates                  uint64  `json:"updates"`
	RecurrentChunkRows       int     `json:"recurrent_chunk_rows"`
	Optimizer                string  `json:"optimizer"`
	LearningRate             float64 `json:"learning_rate"`
	InitialSnapshotSHA       *string `json:"initial_snapshot_sha256"`
	FinalSnapshotSHA         *string `json:"final_snapshot_sha256"`
	SnapshotPath             string  `json:"snapshot_path,omitempty"`
	MeanTargetDisplacementCM float64 `json:"mean_target_displacement_cm_per_sample"`
}

type metricReport struct {
	Before   *metricReportSet `json:"before"`
	After    *metricReportSet `json:"after"`
	Baseline baselineMetrics  `json:"baseline"`
}

type metricReportSet struct {
	Train *metric `json:"train,omitempty"`
	Test  *metric `json:"test,omitempty"`
}

type runtimeReport struct {
	ElapsedMilliseconds int64  `json:"elapsed_milliseconds"`
	MaxRSSBytes         uint64 `json:"max_rss_bytes"`
	GoVersion           string `json:"go_version"`
	GOOS                string `json:"goos"`
	GOARCH              string `json:"goarch"`
}

func main() {
	ctx, stop := osSignal.NotifyContext(context.Background(), shutdownSignals()...)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if ctx == nil || stdout == nil || stderr == nil {
		return errors.New("realnav: context and output writers are required")
	}
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		writeUsage(stdout)
		return nil
	}
	switch args[0] {
	case "train":
		return runTrain(ctx, args[1:], stdout, stderr)
	case "infer":
		return runInfer(ctx, args[1:], stdout, stderr)
	default:
		return fmt.Errorf("realnav: unknown mode %q; use train or infer", args[0])
	}
}

func writeUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: go run ./examples/realnav train --data PATH --out DIR [--epochs N]")
	fmt.Fprintln(w, "       go run ./examples/realnav infer --data PATH --snapshot PATH --out DIR")
	fmt.Fprintln(w, "Imports the verified Dryad trajectory, trains a causal next-displacement regressor, and evaluates a frozen snapshot on trial-held-out data.")
	fmt.Fprintln(w, "Only after_relocation rows and bounded same-trial time windows are used. Reward/fictive distances, condition, segment, and future pose are evaluation metadata, never input features.")
	fmt.Fprintln(w, "The report is an observer-position behaviour prediction; it is not a connectome model or closed-loop navigation result.")
}

func runTrain(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("realnav train", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dataPath := fs.String("data", "", "external licensed trajectory CSV or CSV.gz")
	outPath := fs.String("out", "", "new output directory outside the repository and data directory")
	epochs := fs.Int("epochs", defaultEpochs, "training epochs over trial-held-out training samples")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("realnav train: unexpected positional arguments")
	}
	if *dataPath == "" || *outPath == "" {
		return errors.New("realnav train: --data and --out are required")
	}
	if *epochs <= 0 {
		return errors.New("realnav train: --epochs must be positive")
	}
	start := time.Now()
	dataset, err := importTrajectory(ctx, *dataPath)
	if err != nil {
		return err
	}
	filteredDataset := onlyAfterRelocation(dataset)
	trainDataset, testDataset, err := trajectory.SplitByTrial(filteredDataset, testFraction, splitSeed)
	if err != nil {
		return fmt.Errorf("realnav: split by trial: %w", err)
	}
	conditions, err := trialConditions(dataset)
	if err != nil {
		return err
	}
	config := sampleConfig()
	trainSet, err := trajectory.BuildSamples(trainDataset, config)
	if err != nil {
		return fmt.Errorf("realnav: build training samples: %w", err)
	}
	testSet, err := trajectory.BuildSamples(testDataset, config)
	if err != nil {
		return fmt.Errorf("realnav: build held-out samples: %w", err)
	}
	trainSamples, err := convertSamples(trainSet, conditions)
	if err != nil {
		return err
	}
	testSamples, err := convertSamples(testSet, conditions)
	if err != nil {
		return err
	}
	runResult, err := fitSamples(ctx, trainSamples, testSamples, *epochs)
	if err != nil {
		return err
	}
	outputPath, err := prepareOutputDirectory(*outPath, *dataPath)
	if err != nil {
		return err
	}
	model := savedModel{
		SchemaVersion:         realnavSchemaVersion,
		SourceSHA256:          dataset.Source.SHA256,
		FeatureRule:           featureRuleVersion,
		Preprocessing:         currentPreprocessingContract(),
		TrainMeanDisplacement: runResult.MeanTargetDisplacement,
		Split:                 splitTrials{Train: trialIDs(trainDataset), Test: trialIDs(testDataset)},
		Snapshot:              runResult.After,
	}
	modelPath := filepath.Join(outputPath, "model.json")
	if err := writeJSONExclusive(modelPath, model); err != nil {
		return err
	}
	value := buildReport("train", dataset, trainDataset, testDataset, trainSamples, testSamples, runResult, *epochs, modelPath, start)
	reportPath := filepath.Join(outputPath, "report.json")
	if err := writeJSONExclusive(reportPath, value); err != nil {
		return err
	}
	return encodeReport(stdout, value)
}

func runInfer(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("realnav infer", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dataPath := fs.String("data", "", "external licensed trajectory CSV or CSV.gz")
	snapshotPath := fs.String("snapshot", "", "saved model.json produced by train")
	outPath := fs.String("out", "", "new output directory outside the repository and data directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("realnav infer: unexpected positional arguments")
	}
	if *dataPath == "" || *snapshotPath == "" || *outPath == "" {
		return errors.New("realnav infer: --data, --snapshot and --out are required")
	}
	start := time.Now()
	var model savedModel
	if err := readJSON(*snapshotPath, &model); err != nil {
		return fmt.Errorf("realnav: read snapshot: %w", err)
	}
	if model.SchemaVersion != realnavSchemaVersion || model.FeatureRule != featureRuleVersion {
		return fmt.Errorf("realnav: unsupported snapshot schema or feature rule")
	}
	if err := validateSnapshotPreprocessing(model); err != nil {
		return err
	}
	dataset, err := importTrajectory(ctx, *dataPath)
	if err != nil {
		return err
	}
	if dataset.Source.SHA256 != model.SourceSHA256 {
		return fmt.Errorf("realnav: source SHA-256 %q differs from snapshot %q", dataset.Source.SHA256, model.SourceSHA256)
	}
	filteredDataset := onlyAfterRelocation(dataset)
	conditions, err := trialConditions(dataset)
	if err != nil {
		return err
	}
	trainDataset, testDataset, trainSamples, testSamples, err := rebuildInferenceSplit(filteredDataset, model.Split, conditions)
	if err != nil {
		return err
	}
	trainer, err := restoreTrainer(model.Snapshot)
	if err != nil {
		return fmt.Errorf("realnav: restore frozen snapshot: %w", err)
	}
	if !isFinitePositive(model.TrainMeanDisplacement) {
		return fmt.Errorf("realnav: snapshot has invalid training displacement scale %v", model.TrainMeanDisplacement)
	}
	testMetric, err := evaluateTrainer(ctx, trainer, testSamples, model.TrainMeanDisplacement)
	if err != nil {
		return fmt.Errorf("realnav: independent inference: %w", err)
	}
	runResult := modelRun{Config: model.Snapshot.Config, Before: model.Snapshot, After: model.Snapshot, BeforeMSE: metricSet{Test: testMetric}, AfterMSE: metricSet{Test: testMetric}, Baselines: baselineMetricsFor(testSamples, model.TrainMeanDisplacement), Updates: model.Snapshot.Updates, MeanTargetDisplacement: model.TrainMeanDisplacement}
	outputPath, err := prepareOutputDirectory(*outPath, *dataPath)
	if err != nil {
		return err
	}
	value := buildReport("infer", dataset, trainDataset, testDataset, trainSamples, testSamples, runResult, 0, *snapshotPath, start)
	if err := writeJSONExclusive(filepath.Join(outputPath, "report.json"), value); err != nil {
		return err
	}
	return encodeReport(stdout, value)
}

func buildReport(mode string, dataset, trainDataset, testDataset trajectory.Dataset, trainSamples, testSamples []causalSample, result modelRun, epochs int, snapshotPath string, start time.Time) report {
	contract := currentPreprocessingContract()
	var initialSnapshotSHA *string
	if mode == "train" {
		value := snapshotFingerprint(result.Before)
		initialSnapshotSHA = &value
	}
	finalSnapshotSHAValue := snapshotFingerprint(result.After)
	finalSnapshotSHA := &finalSnapshotSHAValue
	var beforeReport *metricReportSet
	var afterReport = &metricReportSet{Test: metricPointer(result.AfterMSE.Test)}
	if mode == "train" {
		beforeReport = &metricReportSet{Train: metricPointer(result.BeforeMSE.Train), Test: metricPointer(result.BeforeMSE.Test)}
		afterReport = &metricReportSet{Train: metricPointer(result.AfterMSE.Train), Test: metricPointer(result.AfterMSE.Test)}
	}
	return report{
		SchemaVersion: realnavSchemaVersion,
		Mode:          mode,
		Source:        sourceReport{ID: dataset.Source.ID, URL: dataset.Source.URL, DOI: dataset.Source.DOI, PinnedCommit: dataset.Source.PinnedCommit, SHA256: dataset.Source.SHA256, AuthorGitBlobSHA1: realGitBlobSHA1, AuthorReadmeBlobSHA1: realReadmeBlobSHA1, DryadByteEquivalence: "unverified", License: dataset.Source.License, Rows: len(dataset.Rows), Trials: len(trialIDs(dataset))},
		Split:         splitReport{Seed: splitSeed, TestFraction: testFraction, TrainTrialIDs: trialIDs(trainDataset), TestTrialIDs: trialIDs(testDataset), TrainTrialCounts: conditionCounts(trainDataset), TestTrialCounts: conditionCounts(testDataset), TrainSampleCounts: sampleConditionCounts(trainSamples), TestSampleCounts: sampleConditionCounts(testSamples)},
		Filtering:     filterReport{RuleVersion: contract.FeatureRule, Segment: contract.Segment, MaxDeltaTSeconds: contract.MaxDeltaTSeconds, MaxStepDistanceCM: contract.MaxStepDistanceCM, InputFeatures: append([]string(nil), contract.InputFeatures...), Target: append([]string(nil), contract.TargetFeatures...), ForbiddenFeatures: append([]string(nil), contract.ForbiddenFeatures...)},
		Training:      trainingReport{Epochs: epochs, Updates: result.Updates, RecurrentChunkRows: trainingChunk, Optimizer: "learning.Trainer StepFrom with AdamW", LearningRate: result.After.Options.LearningRate, InitialSnapshotSHA: initialSnapshotSHA, FinalSnapshotSHA: finalSnapshotSHA, SnapshotPath: snapshotPath, MeanTargetDisplacementCM: result.MeanTargetDisplacement},
		Metrics:       metricReport{Before: beforeReport, After: afterReport, Baseline: result.Baselines},
		Runtime:       runtimeReport{ElapsedMilliseconds: time.Since(start).Milliseconds(), MaxRSSBytes: maxRSSBytes(), GoVersion: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH},
		Limitations:   []string{"This is a teacher-forced next-displacement behaviour-prediction example for an observer position, not a fruit-fly connectome or a closed-loop navigation policy.", "The model receives current pose and prior displacement/time only. Reward, fictive target, condition, segment, and future pose fields are excluded from the model input.", "The trajectory adapter exposes no reward or fictive-zone coordinates, so this report makes no predicted-distance, distance-change, or return-success claim.", "The local author-repository copy matches the pinned Git blob and its README blob. Byte-equivalence to the Dryad ZIP was not verified because direct Dryad retrieval returned 403/401.", "The Dryad source is one experiment with 39 trials; held-out trial metrics are evidence of this run's pipeline and not a general behavioural or biological result."},
	}
}

func metricPointer(value metric) *metric {
	return &value
}

func encodeReport(w io.Writer, value report) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func readJSON(path string, value any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("realnav: JSON path %q is not a regular file", path)
	}
	data, err := io.ReadAll(io.LimitReader(f, 32<<20+1))
	if err != nil {
		return err
	}
	if len(data) > 32<<20 {
		return fmt.Errorf("realnav: JSON file %q exceeds 32 MiB", path)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("realnav: JSON file %q contains trailing value", path)
		}
		return fmt.Errorf("realnav: JSON file %q contains trailing data: %w", path, err)
	}
	return nil
}

func writeJSONExclusive(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("realnav: encode %s: %w", path, err)
	}
	data = append(data, '\n')
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("realnav: create %s: %w", path, err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("realnav: write %s: %w", path, err)
	}
	return f.Sync()
}

func prepareOutputDirectory(outPath, dataPath string) (string, error) {
	output, err := filepath.Abs(outPath)
	if err != nil {
		return "", err
	}
	data, err := filepath.Abs(dataPath)
	if err != nil {
		return "", err
	}
	resolvedOutput, err := resolveOutputPath(output)
	if err != nil {
		return "", err
	}
	resolvedData, err := filepath.EvalSymlinks(data)
	if err != nil {
		return "", fmt.Errorf("realnav: resolve source data path: %w", err)
	}
	resolvedData, err = filepath.Abs(resolvedData)
	if err != nil {
		return "", err
	}
	dataDirectory := filepath.Dir(resolvedData)
	if pathOverlaps(resolvedOutput, dataDirectory) || pathOverlaps(dataDirectory, resolvedOutput) {
		return "", errors.New("realnav: output directory must be separate from the source data directory")
	}
	if _, err := os.Stat(output); err == nil {
		return "", fmt.Errorf("realnav: output directory %q already exists", output)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(output, 0o700); err != nil {
		return "", err
	}
	return output, nil
}

func pathWithin(path, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func pathOverlaps(path, other string) bool {
	return filepath.Clean(path) == filepath.Clean(other) || pathWithin(path, other)
}

func resolveOutputPath(path string) (string, error) {
	current := filepath.Clean(path)
	var missing []string
	for {
		_, err := os.Lstat(current)
		if err == nil {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", fmt.Errorf("realnav: resolve output path %q: %w", current, err)
			}
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Abs(resolved)
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("realnav: inspect output path %q: %w", current, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("realnav: no existing parent for output path %q", path)
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func selectTrials(dataset trajectory.Dataset, ids []string) (trajectory.Dataset, error) {
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" || wanted[id] {
			return trajectory.Dataset{}, errors.New("realnav: snapshot has duplicate or empty held-out trial id")
		}
		wanted[id] = true
	}
	rows := make([]trajectory.Point, 0, len(dataset.Rows))
	seen := make(map[string]bool)
	for _, row := range dataset.Rows {
		if wanted[row.TrialID] {
			rows = append(rows, row)
			seen[row.TrialID] = true
		}
	}
	for id := range wanted {
		if !seen[id] {
			return trajectory.Dataset{}, fmt.Errorf("realnav: snapshot trial %q is absent from current dataset", id)
		}
	}
	return trajectory.Dataset{Schema: dataset.Schema, Source: dataset.Source, Rows: rows}, nil
}

func rebuildInferenceSplit(dataset trajectory.Dataset, split splitTrials, conditions map[string]string) (trajectory.Dataset, trajectory.Dataset, []causalSample, []causalSample, error) {
	trainDataset, err := selectTrials(dataset, split.Train)
	if err != nil {
		return trajectory.Dataset{}, trajectory.Dataset{}, nil, nil, fmt.Errorf("realnav: select training trials: %w", err)
	}
	testDataset, err := selectTrials(dataset, split.Test)
	if err != nil {
		return trajectory.Dataset{}, trajectory.Dataset{}, nil, nil, fmt.Errorf("realnav: select held-out trials: %w", err)
	}
	config := sampleConfig()
	trainSet, err := trajectory.BuildSamples(trainDataset, config)
	if err != nil {
		return trajectory.Dataset{}, trajectory.Dataset{}, nil, nil, fmt.Errorf("realnav: rebuild training samples: %w", err)
	}
	testSet, err := trajectory.BuildSamples(testDataset, config)
	if err != nil {
		return trajectory.Dataset{}, trajectory.Dataset{}, nil, nil, fmt.Errorf("realnav: rebuild held-out samples: %w", err)
	}
	trainSamples, err := convertSamples(trainSet, conditions)
	if err != nil {
		return trajectory.Dataset{}, trajectory.Dataset{}, nil, nil, fmt.Errorf("realnav: convert training samples: %w", err)
	}
	testSamples, err := convertSamples(testSet, conditions)
	if err != nil {
		return trajectory.Dataset{}, trajectory.Dataset{}, nil, nil, fmt.Errorf("realnav: convert held-out samples: %w", err)
	}
	return trainDataset, testDataset, trainSamples, testSamples, nil
}

func trialIDs(dataset trajectory.Dataset) []string {
	seen := make(map[string]bool)
	for _, row := range dataset.Rows {
		if row.TrialID != "" {
			seen[row.TrialID] = true
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func conditionCounts(dataset trajectory.Dataset) map[string]int {
	counts := make(map[string]int)
	seen := make(map[string]bool)
	for _, row := range dataset.Rows {
		if row.TrialID == "" || seen[row.TrialID] {
			continue
		}
		seen[row.TrialID] = true
		counts[row.Condition]++
	}
	return counts
}

func sampleConditionCounts(samples []causalSample) map[string]int {
	counts := make(map[string]int)
	for _, sample := range samples {
		counts[sample.Condition]++
	}
	return counts
}
