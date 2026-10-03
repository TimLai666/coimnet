package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/nav2d/trajectory"
)

const memoryCounterfactualSchema = "coimnet-realnav-memory-counterfactual/v1"
const counterfactualResolutionCM = 1e-6

// counterfactualInputs changes the already-transformed model-visible history.
// Transforming erased raw stimulus would also change later shuffled inputs.
func counterfactualInputs(trial historyTrial, cutoff int, control string, seed int64) ([][]float64, [][]float64, int, error) {
	if cutoff <= 0 || cutoff > trial.RolloutIndex || trial.RolloutIndex >= len(trial.Steps) {
		return nil, nil, 0, fmt.Errorf("realnavmemory: invalid relocation cutoff %d for trial %q", cutoff, trial.ID)
	}
	original, err := trialInputs(trial, control, seed)
	if err != nil {
		return nil, nil, 0, err
	}
	erased := make([][]float64, len(original))
	changes := 0
	for i, row := range original {
		erased[i] = append([]float64(nil), row...)
		if i < cutoff {
			if row[5] != 0 {
				changes++
			}
			erased[i][5] = 0
		}
	}
	return original, erased, changes, nil
}

type counterfactualRecorded struct {
	Samples              int     `json:"samples"`
	OriginalMSE          float64 `json:"original_mse_cm2"`
	ErasedMSE            float64 `json:"erased_mse_cm2"`
	DeltaMSE             float64 `json:"erased_minus_original_mse_cm2"`
	ChangedRows          int     `json:"changed_rows"`
	AboveResolutionRows  int     `json:"above_resolution_rows"`
	MaxDeltaCM           float64 `json:"max_delta_cm"`
	MeanDeltaCM          float64 `json:"mean_delta_cm"`
	FirstChangedRow      int     `json:"first_changed_row"`
	LastScoredDeltaCM    float64 `json:"last_scored_delta_cm"`
	OriginalOutputSHA256 string  `json:"original_output_sha256"`
	ErasedOutputSHA256   string  `json:"erased_output_sha256"`
}

type counterfactualRollout struct {
	Original                 memoryRolloutTrial `json:"original"`
	Erased                   memoryRolloutTrial `json:"erased"`
	OriginalTraceSHA256      string             `json:"original_trace_sha256"`
	ErasedTraceSHA256        string             `json:"erased_trace_sha256"`
	MaxActionDeltaCM         float64            `json:"max_action_delta_cm"`
	MaxPositionSeparationCM  float64            `json:"max_position_separation_cm"`
	ChangedDecisions         int                `json:"changed_decisions"`
	AboveResolutionDecisions int                `json:"above_resolution_decisions"`
	FirstActionDeltaCM       float64            `json:"first_action_delta_cm"`
}

type memoryCounterfactualTrial struct {
	TrialID                     string                 `json:"trial_id"`
	Condition                   string                 `json:"condition"`
	RelocationIndex             int                    `json:"relocation_index"`
	RolloutIndex                int                    `json:"rollout_index"`
	HistoryRows                 int                    `json:"history_rows"`
	ErasedRows                  int                    `json:"erased_stimulus_rows"`
	OriginalNonStimulusSHA256   string                 `json:"original_non_stimulus_sha256"`
	ErasedNonStimulusSHA256     string                 `json:"erased_non_stimulus_sha256"`
	OriginalLaterStimulusSHA256 string                 `json:"original_later_stimulus_sha256"`
	ErasedLaterStimulusSHA256   string                 `json:"erased_later_stimulus_sha256"`
	Recorded                    counterfactualRecorded `json:"recorded"`
	Rollout                     counterfactualRollout  `json:"rollout"`
	Frozen                      bool                   `json:"snapshot_frozen"`
}

type memoryCounterfactualGroup struct {
	Seed            uint64                      `json:"seed"`
	Control         string                      `json:"control"`
	Phase           string                      `json:"phase"`
	ParameterSHA256 string                      `json:"parameter_sha256"`
	OptimizerSHA256 string                      `json:"optimizer_sha256"`
	Trials          []memoryCounterfactualTrial `json:"trials"`
}

type memoryCounterfactualReport struct {
	SchemaVersion     string                      `json:"schema_version"`
	Mode              string                      `json:"mode"`
	Source            trajectory.Source           `json:"source"`
	SourceSHA256      string                      `json:"source_sha256"`
	BundleFingerprint string                      `json:"bundle_fingerprint"`
	Split             trialSplit                  `json:"split"`
	ResolutionCM      float64                     `json:"resolution_cm"`
	Intervention      string                      `json:"intervention"`
	Continuation      string                      `json:"continuation"`
	RolloutProtocol   memoryRolloutProtocol       `json:"rollout_protocol"`
	Groups            []memoryCounterfactualGroup `json:"groups"`
	Limitations       []string                    `json:"limitations"`
}

func compareMemoryTrial(ctx context.Context, snapshot learning.TrainingSnapshot, trial historyTrial, cutoff int, target memoryRolloutTarget, control string, seed uint64) (memoryCounterfactualTrial, error) {
	if ctx == nil {
		return memoryCounterfactualTrial{}, errors.New("realnavmemory: nil counterfactual context")
	}
	if err := ctx.Err(); err != nil {
		return memoryCounterfactualTrial{}, err
	}
	frozenHash := memoryJSONHash(snapshot)
	original, erased, changes, err := counterfactualInputs(trial, cutoff, control, int64(seed))
	if err != nil {
		return memoryCounterfactualTrial{}, err
	}
	network, err := learning.NewNetwork(snapshot.Config)
	if err != nil {
		return memoryCounterfactualTrial{}, err
	}
	originalPrediction, err := network.PredictAll(ctx, snapshot.Parameters, original)
	if err != nil {
		return memoryCounterfactualTrial{}, err
	}
	erasedPrediction, err := network.PredictAll(ctx, snapshot.Parameters, erased)
	if err != nil {
		return memoryCounterfactualTrial{}, err
	}
	originalLoss, _, samples, err := maskedMSE(originalPrediction, trial)
	if err != nil {
		return memoryCounterfactualTrial{}, err
	}
	erasedLoss, _, _, err := maskedMSE(erasedPrediction, trial)
	if err != nil {
		return memoryCounterfactualTrial{}, err
	}
	recorded := counterfactualRecorded{Samples: samples, OriginalMSE: originalLoss, ErasedMSE: erasedLoss, DeltaMSE: erasedLoss - originalLoss, FirstChangedRow: -1, OriginalOutputSHA256: memoryJSONHash(originalPrediction), ErasedOutputSHA256: memoryJSONHash(erasedPrediction)}
	if !finite(recorded.DeltaMSE) {
		return memoryCounterfactualTrial{}, errors.New("non-finite counterfactual MSE difference")
	}
	nonStimOriginal, nonStimErased := make([][5]float64, len(original)), make([][5]float64, len(original))
	laterOriginal, laterErased := make([]float64, len(original)-cutoff), make([]float64, len(original)-cutoff)
	for i, step := range trial.Steps {
		copy(nonStimOriginal[i][:], original[i][:5])
		copy(nonStimErased[i][:], erased[i][:5])
		if i >= cutoff {
			laterOriginal[i-cutoff], laterErased[i-cutoff] = original[i][5], erased[i][5]
		}
		if !step.Scored {
			continue
		}
		if i < cutoff {
			return memoryCounterfactualTrial{}, errors.New("scored row precedes relocation cutoff")
		}
		delta := math.Hypot(originalPrediction[i][0]-erasedPrediction[i][0], originalPrediction[i][1]-erasedPrediction[i][1])
		if !finite(delta) {
			return memoryCounterfactualTrial{}, errors.New("non-finite prediction difference")
		}
		if delta > 0 {
			recorded.ChangedRows++
			if recorded.FirstChangedRow < 0 {
				recorded.FirstChangedRow = i
			}
		}
		if delta > counterfactualResolutionCM {
			recorded.AboveResolutionRows++
		}
		recorded.MaxDeltaCM = math.Max(recorded.MaxDeltaCM, delta)
		recorded.MeanDeltaCM += delta / float64(samples)
		recorded.LastScoredDeltaCM = delta
	}
	rollout := counterfactualRollout{}
	for _, eraseUntil := range []int{0, cutoff} {
		individual, err := learning.NewIndividual(snapshot.Config, snapshot.Parameters, snapshot.Options, make([]float64, snapshot.Config.Dynamics.Nodes))
		if err != nil {
			return memoryCounterfactualTrial{}, err
		}
		result, err := memoryModelTrialErase(ctx, individual, trial, target, control, seed, eraseUntil)
		if err != nil {
			return memoryCounterfactualTrial{}, err
		}
		if memoryJSONHash(individual.Snapshot().Parameters) != memoryJSONHash(snapshot.Parameters) {
			return memoryCounterfactualTrial{}, errors.New("counterfactual rollout changed parameters")
		}
		if eraseUntil == 0 {
			rollout.Original = result
		} else {
			rollout.Erased = result
		}
	}
	if len(rollout.Original.Trace) != memoryDecisionRows || len(rollout.Erased.Trace) != memoryDecisionRows {
		return memoryCounterfactualTrial{}, errors.New("counterfactual rollout lost decisions")
	}
	for i, left := range rollout.Original.Trace {
		right := rollout.Erased.Trace[i]
		if left.Input[5] != right.Input[5] {
			return memoryCounterfactualTrial{}, errors.New("counterfactual changed later rollout stimulus")
		}
		delta := math.Hypot(left.Action[0]-right.Action[0], left.Action[1]-right.Action[1])
		separation := math.Hypot(left.Position[0]-right.Position[0], left.Position[1]-right.Position[1])
		if !finite(delta) || !finite(separation) {
			return memoryCounterfactualTrial{}, errors.New("non-finite counterfactual rollout difference")
		}
		if i == 0 {
			rollout.FirstActionDeltaCM = delta
		}
		if delta > 0 {
			rollout.ChangedDecisions++
		}
		if delta > counterfactualResolutionCM {
			rollout.AboveResolutionDecisions++
		}
		rollout.MaxActionDeltaCM = math.Max(rollout.MaxActionDeltaCM, delta)
		rollout.MaxPositionSeparationCM = math.Max(rollout.MaxPositionSeparationCM, separation)
	}
	rollout.OriginalTraceSHA256, rollout.ErasedTraceSHA256 = memoryJSONHash(rollout.Original.Trace), memoryJSONHash(rollout.Erased.Trace)
	// Compact reports keep every trial and complete trace fingerprints; no model
	// or large generated-position trace is added to Git.
	rollout.Original.Trace, rollout.Erased.Trace = nil, nil
	result := memoryCounterfactualTrial{TrialID: trial.ID, Condition: trial.Condition, RelocationIndex: cutoff, RolloutIndex: trial.RolloutIndex, HistoryRows: len(trial.Steps), ErasedRows: changes, OriginalNonStimulusSHA256: memoryJSONHash(nonStimOriginal), ErasedNonStimulusSHA256: memoryJSONHash(nonStimErased), OriginalLaterStimulusSHA256: memoryJSONHash(laterOriginal), ErasedLaterStimulusSHA256: memoryJSONHash(laterErased), Recorded: recorded, Rollout: rollout, Frozen: memoryJSONHash(snapshot) == frozenHash}
	if !result.Frozen || result.OriginalNonStimulusSHA256 != result.ErasedNonStimulusSHA256 || result.OriginalLaterStimulusSHA256 != result.ErasedLaterStimulusSHA256 {
		return memoryCounterfactualTrial{}, errors.New("counterfactual violated frozen input contract")
	}
	return result, nil
}

// Cutoffs are evaluation metadata. They never become model input features.
func memoryRelocationCutoffs(rows []trajectory.Point, trials []historyTrial) (map[string]int, error) {
	counts := make(map[string]int)
	cutoffs := make(map[string]int)
	for _, row := range rows {
		index := counts[row.TrialID]
		if row.Segment == "relocation" {
			if _, ok := cutoffs[row.TrialID]; !ok {
				cutoffs[row.TrialID] = index
			}
		}
		counts[row.TrialID]++
	}
	if len(counts) != len(trials) {
		return nil, errors.New("relocation trial coverage mismatch")
	}
	for _, trial := range trials {
		cutoff, ok := cutoffs[trial.ID]
		if !ok || cutoff <= 0 || cutoff > trial.RolloutIndex || counts[trial.ID] != len(trial.Steps) {
			return nil, fmt.Errorf("invalid or missing relocation boundary for %q", trial.ID)
		}
	}
	return cutoffs, nil
}

func buildMemoryCounterfactualReport(ctx context.Context, bundle memoryBundle, trials []historyTrial, cutoffs map[string]int, targets []trajectory.ReturnTarget) (memoryCounterfactualReport, error) {
	selected, err := selectTrials(trials, bundle.Split.Test)
	if err != nil {
		return memoryCounterfactualReport{}, err
	}
	targetByID, err := memoryRolloutTargets(targets, bundle.Split.Test)
	if err != nil {
		return memoryCounterfactualReport{}, err
	}
	frozenHash := memoryJSONHash(bundle)
	groups := make([]memoryCounterfactualGroup, 0, len(bundle.Runs)*2)
	for _, run := range bundle.Runs {
		for phaseIndex, snapshot := range []learning.TrainingSnapshot{run.Before, run.After} {
			phase := "before"
			if phaseIndex == 1 {
				phase = "after"
			}
			group := memoryCounterfactualGroup{Seed: run.Seed, Control: run.Control, Phase: phase, ParameterSHA256: memoryJSONHash(snapshot.Parameters), OptimizerSHA256: memoryJSONHash(snapshot.Optimizer), Trials: make([]memoryCounterfactualTrial, 0, len(selected))}
			for _, trial := range selected {
				result, err := compareMemoryTrial(ctx, snapshot, trial, cutoffs[trial.ID], targetByID[trial.ID], run.Control, run.Seed)
				if err != nil {
					return memoryCounterfactualReport{}, fmt.Errorf("counterfactual %d/%s/%s/%s: %w", run.Seed, run.Control, phase, trial.ID, err)
				}
				group.Trials = append(group.Trials, result)
			}
			groups = append(groups, group)
		}
	}
	if memoryJSONHash(bundle) != frozenHash {
		return memoryCounterfactualReport{}, errors.New("counterfactual changed bundle")
	}
	return memoryCounterfactualReport{SchemaVersion: memoryCounterfactualSchema, Mode: "counterfactual", Source: bundle.Source, SourceSHA256: bundle.SourceSHA256, BundleFingerprint: frozenHash, Split: bundle.Split, ResolutionCM: counterfactualResolutionCM, Intervention: "zero model-visible stimulus before first relocation source row, after the original control transform", Continuation: "keep all non-stimulus recorded inputs and all later stimulus inputs unchanged; retain original raw blocks for autonomous shuffled continuation", RolloutProtocol: memoryRolloutProtocolContract(), Groups: groups, Limitations: []string{"artificial eight-node graph, not MaleCNS", "input sensitivity is not evidence of useful memory, navigation improvement or a biological mechanism", "removal is a model-input intervention, not an observed animal counterfactual", "recorded paths stay fixed; autonomous paths may diverge through model actions", "test trials were previously evaluated; no retuning or training in this command", "resolution is an engineering reporting scale, not a statistical or biological threshold", "whole-file bundle fingerprint hashes decoded JSON; raw file hash is recorded by the evidence runner"}}, nil
}

func runMemoryCounterfactual(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("realnavmemory counterfactual", flag.ContinueOnError)
	fs.SetOutput(stderr)
	inputPath := fs.String("input", "", "required source-pinned trajectory CSV or CSV.gz")
	modelPath := fs.String("model", "", "required existing bundle.json; no training or retuning")
	outPath := fs.String("out-dir", "", "required new output directory; never overwritten")
	fs.Usage = func() {
		fmt.Fprintln(stdout, "realnavmemory counterfactual --input CSV --model BUNDLE_JSON --out-dir NEW\nErases only pre-relocation model-visible stimulus; reports every test pair.\nFixed reporting scale: 1e-6 cm. Errors: invalid source, model, boundary, cancellation or existing output.\nExample: realnavmemory counterfactual --input /data/trials.csv.gz --model /data/bundle.json --out-dir /data/counterfactual")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("realnavmemory counterfactual: unexpected positional arguments")
	}
	if *inputPath == "" || *modelPath == "" || *outPath == "" {
		return errors.New("realnavmemory counterfactual: --input, --model and --out-dir are required")
	}
	var bundle memoryBundle
	if err := readMemoryJSON(*modelPath, &bundle); err != nil {
		return err
	}
	dataset, stimuli, err := trajectory.ReadWithStimulusHistory(ctx, *inputPath, realSource(), trajectory.DefaultLimits())
	if err != nil {
		return err
	}
	trials, err := buildHistory(dataset, stimuli)
	if err != nil {
		return err
	}
	if err := validateMemoryBundle(bundle, trials); err != nil {
		return err
	}
	cutoffs, err := memoryRelocationCutoffs(dataset.Rows, trials)
	if err != nil {
		return err
	}
	_, targets, err := trajectory.ReadWithReturnTargets(ctx, *inputPath, realSource(), trajectory.DefaultLimits())
	if err != nil {
		return err
	}
	report, err := buildMemoryCounterfactualReport(ctx, bundle, trials, cutoffs, targets)
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
