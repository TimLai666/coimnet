package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	rand "math/rand/v2"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/nav2d/trajectory"
)

const (
	rolloutSchemaVersion = "coimnet-realnav-rollout/v1"
	defaultRolloutSteps  = 200
	maxRolloutSteps      = 4096
	arenaRadiusCM        = 30.0
	goalRadiusCM         = 2.0
	maxActionLengthCM    = 5.0
	rolloutDecisionDT    = 0.1
	randomDirectionSeed  = uint64(20261002)
)

type rolloutStrategy string

const (
	rolloutStrategyModel       rolloutStrategy = "model"
	rolloutStrategyPersistent  rolloutStrategy = "persistent_direction"
	rolloutStrategyZero        rolloutStrategy = "zero_output"
	rolloutStrategyRandom      rolloutStrategy = "random_direction"
	rolloutStrategyFixedAction rolloutStrategy = "fixed_action"
)

var rolloutStrategyNames = []rolloutStrategy{
	rolloutStrategyModel,
	rolloutStrategyPersistent,
	rolloutStrategyZero,
	rolloutStrategyRandom,
}

type rolloutTarget struct {
	TrialID string
	XCM     float64
	YCM     float64
}

type rolloutStart struct {
	TrialID      string
	Condition    string
	XCM          float64
	YCM          float64
	PreviousDXCM float64
	PreviousDYCM float64
	PreviousDT   float64
	Target       rolloutTarget
}

type rolloutExcluded struct {
	TrialID   string `json:"trial_id"`
	Condition string `json:"condition,omitempty"`
	Reason    string `json:"reason"`
}

type rolloutProtocol struct {
	SchemaVersion          string            `json:"schema_version"`
	DecisionSteps          int               `json:"decision_steps"`
	ArenaCenterCM          [2]float64        `json:"arena_center_cm"`
	ArenaRadiusCM          float64           `json:"arena_radius_cm"`
	GoalRadiusCM           float64           `json:"goal_radius_cm"`
	MaxActionLengthCM      float64           `json:"max_action_length_cm"`
	MaxDeltaTSeconds       float64           `json:"max_delta_t_seconds"`
	MaxStepDistanceCM      float64           `json:"max_step_distance_cm"`
	InitialPairRule        string            `json:"initial_pair_rule"`
	InitialPreviousDT      string            `json:"initial_previous_dt"`
	FixedPreviousDTSeconds float64           `json:"fixed_previous_dt_seconds"`
	ResetEveryRows         int               `json:"reset_every_rows"`
	InputFeatures          []string          `json:"input_features"`
	Strategies             []rolloutStrategy `json:"strategies"`
	RandomDirectionSeed    uint64            `json:"random_direction_seed"`
	RandomStreamDerivation string            `json:"random_stream_derivation"`
	TargetRole             string            `json:"target_role"`
	SuccessRule            string            `json:"success_rule"`
	ActionRule             string            `json:"action_rule"`
	ClippingRule           string            `json:"clipping_rule"`
	CollisionRule          string            `json:"collision_rule"`
}

type rolloutStrategyResult struct {
	Success           bool    `json:"success"`
	StartSuccess      bool    `json:"start_success"`
	HitStep           int     `json:"hit_step"`
	Steps             int     `json:"steps"`
	PathCM            float64 `json:"path_cm"`
	FinalDistanceCM   float64 `json:"final_distance_cm"`
	NearestDistanceCM float64 `json:"nearest_distance_cm"`
	Collisions        int     `json:"collisions"`
	ClippedActions    int     `json:"clipped_actions"`
}

type rolloutTrialReport struct {
	TrialID    string                                    `json:"trial_id"`
	Condition  string                                    `json:"condition"`
	Target     rolloutTargetReport                       `json:"target"`
	Strategies map[rolloutStrategy]rolloutStrategyResult `json:"strategies"`
}

type rolloutTargetReport struct {
	XCM            float64 `json:"x_cm"`
	YCM            float64 `json:"y_cm"`
	EvaluationOnly bool    `json:"evaluation_only"`
}

type rolloutAggregate struct {
	Samples          int     `json:"samples"`
	Successes        int     `json:"successes"`
	SuccessRate      float64 `json:"success_rate"`
	StartSuccesses   int     `json:"start_successes"`
	StartSuccessRate float64 `json:"start_success_rate"`
	Steps            int     `json:"steps"`
	MeanSteps        float64 `json:"mean_steps"`
	Collisions       int     `json:"collisions"`
	MeanCollisions   float64 `json:"mean_collisions"`
	ClippedActions   int     `json:"clipped_actions"`
}

type rolloutReport struct {
	SchemaVersion           string                                          `json:"schema_version"`
	Mode                    string                                          `json:"mode"`
	Protocol                rolloutProtocol                                 `json:"protocol"`
	ProtocolSHA256          string                                          `json:"protocol_sha256"`
	Source                  sourceReport                                    `json:"source"`
	SourceSHA256            string                                          `json:"source_sha256"`
	SnapshotSHA256          string                                          `json:"snapshot_sha256"`
	OriginalHoldoutTrialIDs []string                                        `json:"original_holdout_trial_ids"`
	Split                   splitReport                                     `json:"split"`
	Trials                  []rolloutTrialReport                            `json:"trials"`
	Excluded                []rolloutExcluded                               `json:"excluded"`
	Aggregates              map[string]map[rolloutStrategy]rolloutAggregate `json:"aggregates"`
	Runtime                 runtimeReport                                   `json:"runtime"`
	Limitations             []string                                        `json:"limitations"`
}

type rolloutTraceStep struct {
	Input        trajectory.Input
	ActionDXCM   float64
	ActionDYCM   float64
	RealizedDXCM float64
	RealizedDYCM float64
	Collision    int
	Clipped      int
	XCM          float64
	YCM          float64
}

func currentRolloutProtocol(steps int) rolloutProtocol {
	return rolloutProtocol{
		SchemaVersion:          rolloutSchemaVersion,
		DecisionSteps:          steps,
		ArenaCenterCM:          [2]float64{0, 0},
		ArenaRadiusCM:          arenaRadiusCM,
		GoalRadiusCM:           goalRadiusCM,
		MaxActionLengthCM:      maxActionLengthCM,
		MaxDeltaTSeconds:       currentPreprocessingContract().MaxDeltaTSeconds,
		MaxStepDistanceCM:      currentPreprocessingContract().MaxStepDistanceCM,
		InitialPairRule:        "earliest adjacent same-trial after_relocation pair with 0<gap<=max_delta_t and distance<=max_step_distance",
		InitialPreviousDT:      "observed_pair_gap",
		FixedPreviousDTSeconds: rolloutDecisionDT,
		ResetEveryRows:         trainingChunk,
		InputFeatures:          append([]string(nil), currentPreprocessingContract().InputFeatures...),
		Strategies:             append([]rolloutStrategy(nil), rolloutStrategyNames...),
		RandomDirectionSeed:    randomDirectionSeed,
		RandomStreamDerivation: "PCG(seed xor first_u64(sha256(trial_id)), second_u64(sha256(trial_id))) per trial",
		TargetRole:             "evaluation_only",
		SuccessRule:            "initial position or any realized action segment intersects the closed disk centered at that trial's evaluation-only target",
		ActionRule:             "model residual plus causal direction baseline; controls use the same start, arena, and step limit",
		ClippingRule:           "actions longer than 5 cm are scaled to exactly 5 cm and counted",
		CollisionRule:          "a candidate endpoint outside the 30 cm arena leaves position unchanged and realizes zero displacement",
	}
}

func rolloutProtocolHash(protocol rolloutProtocol) (string, error) {
	encoded, err := json.Marshal(protocol)
	if err != nil {
		return "", fmt.Errorf("realnav rollout: encode protocol: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func rolloutProtocolHashMust(protocol rolloutProtocol) string {
	hash, _ := rolloutProtocolHash(protocol)
	return hash
}

func validateRolloutSteps(steps int) error {
	if steps < 1 || steps > maxRolloutSteps {
		return fmt.Errorf("realnav rollout: --steps must be between 1 and %d", maxRolloutSteps)
	}
	return nil
}

func convertReturnTargets(targets []trajectory.ReturnTarget) []rolloutTarget {
	converted := make([]rolloutTarget, len(targets))
	for i, target := range targets {
		converted[i] = rolloutTarget{TrialID: target.TrialID, XCM: target.XCM, YCM: target.YCM}
	}
	return converted
}

func validateRolloutModel(model savedModel, sourceSHA string) error {
	if model.SchemaVersion != realnavSchemaVersion || model.FeatureRule != featureRuleVersion {
		return errors.New("realnav rollout: unsupported snapshot schema or feature rule")
	}
	if err := validateSnapshotPreprocessing(model); err != nil {
		return err
	}
	if model.SourceSHA256 == "" || (sourceSHA != "" && model.SourceSHA256 != sourceSHA) {
		return fmt.Errorf("realnav rollout: source SHA-256 %q differs from snapshot %q", sourceSHA, model.SourceSHA256)
	}
	if !isFinitePositive(model.TrainMeanDisplacement) {
		return fmt.Errorf("realnav rollout: snapshot has invalid training displacement scale %v", model.TrainMeanDisplacement)
	}
	if len(model.Split.Test) == 0 {
		return errors.New("realnav rollout: snapshot has no held-out trial IDs")
	}
	if len(model.Snapshot.Config.ReadoutNodes) == 0 || model.Snapshot.Config.OutputSize != 2 {
		return errors.New("realnav rollout: snapshot readout must have two outputs")
	}
	if _, err := restoreTrainer(model.Snapshot); err != nil {
		return fmt.Errorf("realnav rollout: invalid frozen snapshot: %w", err)
	}
	return nil
}

func validateRolloutSplit(dataset trajectory.Dataset, split splitTrials) error {
	all := trialIDs(dataset)
	if len(all) == 0 {
		return errors.New("realnav rollout: current dataset has no trials")
	}
	seen := make(map[string]string, len(all))
	for _, id := range split.Train {
		if id == "" {
			return errors.New("realnav rollout: training split contains an empty trial ID")
		}
		if previous, ok := seen[id]; ok {
			return fmt.Errorf("realnav rollout: trial %q appears in %s and training split", id, previous)
		}
		seen[id] = "training split"
	}
	for _, id := range split.Test {
		if id == "" {
			return errors.New("realnav rollout: held-out split contains an empty trial ID")
		}
		if previous, ok := seen[id]; ok {
			return fmt.Errorf("realnav rollout: trial %q appears in %s and held-out split", id, previous)
		}
		seen[id] = "held-out split"
	}
	if len(seen) != len(all) {
		return fmt.Errorf("realnav rollout: snapshot split covers %d trials, current source has %d", len(seen), len(all))
	}
	for _, id := range all {
		if _, ok := seen[id]; !ok {
			return fmt.Errorf("realnav rollout: current source trial %q is absent from snapshot split", id)
		}
	}
	return nil
}

func buildRolloutStarts(dataset trajectory.Dataset, testIDs []string, targets []rolloutTarget) ([]rolloutStart, []rolloutExcluded, error) {
	if len(testIDs) == 0 {
		return nil, nil, errors.New("realnav rollout: no held-out trial IDs")
	}
	wanted := make(map[string]struct{}, len(testIDs))
	for _, id := range testIDs {
		if id == "" {
			return nil, nil, errors.New("realnav rollout: held-out trial ID is empty")
		}
		if _, exists := wanted[id]; exists {
			return nil, nil, fmt.Errorf("realnav rollout: duplicate held-out trial ID %q", id)
		}
		wanted[id] = struct{}{}
	}
	targetByTrial := make(map[string]rolloutTarget, len(targets))
	for _, target := range targets {
		if target.TrialID == "" {
			return nil, nil, errors.New("realnav rollout: return target has empty trial ID")
		}
		if _, exists := targetByTrial[target.TrialID]; exists {
			return nil, nil, fmt.Errorf("realnav rollout: duplicate return target for trial %q", target.TrialID)
		}
		if !finitePoint(target.XCM, target.YCM) {
			return nil, nil, fmt.Errorf("realnav rollout: return target for trial %q is non-finite", target.TrialID)
		}
		targetByTrial[target.TrialID] = target
	}
	for id := range wanted {
		target, ok := targetByTrial[id]
		if !ok {
			return nil, nil, fmt.Errorf("realnav rollout: no return target for held-out trial %q", id)
		}
		if math.Hypot(target.XCM, target.YCM) > arenaRadiusCM {
			return nil, nil, fmt.Errorf("realnav rollout: return target for trial %q is outside arena", id)
		}
	}
	conditions, err := trialConditions(dataset)
	if err != nil {
		return nil, nil, err
	}
	starts := make(map[string]rolloutStart, len(wanted))
	config := sampleConfig()
	for i := 0; i+1 < len(dataset.Rows); i++ {
		previous, current := dataset.Rows[i], dataset.Rows[i+1]
		if previous.TrialID != current.TrialID {
			continue
		}
		if _, ok := wanted[current.TrialID]; !ok || previous.Segment != currentPreprocessingContract().Segment || current.Segment != currentPreprocessingContract().Segment {
			continue
		}
		if _, exists := starts[current.TrialID]; exists {
			continue
		}
		gap := current.T - previous.T
		dx, dy := current.XCM-previous.XCM, current.YCM-previous.YCM
		if !isFinitePositive(gap) || gap > config.MaxDeltaT || !finitePoint(dx, dy) || math.Hypot(dx, dy) > config.MaxStepDistanceCM {
			continue
		}
		start := rolloutStart{TrialID: current.TrialID, Condition: conditions[current.TrialID], XCM: current.XCM, YCM: current.YCM, PreviousDXCM: dx, PreviousDYCM: dy, PreviousDT: gap, Target: targetByTrial[current.TrialID]}
		starts[current.TrialID] = start
	}
	excluded := make([]rolloutExcluded, 0, len(wanted)-len(starts))
	for id := range wanted {
		start, ok := starts[id]
		if !ok {
			excluded = append(excluded, rolloutExcluded{TrialID: id, Condition: conditions[id], Reason: "no legal after_relocation pair"})
			continue
		}
		if !finitePoint(start.XCM, start.YCM) {
			delete(starts, id)
			excluded = append(excluded, rolloutExcluded{TrialID: id, Condition: conditions[id], Reason: "initial position is non-finite"})
			continue
		}
		if math.Hypot(start.XCM, start.YCM) > arenaRadiusCM {
			delete(starts, id)
			excluded = append(excluded, rolloutExcluded{TrialID: id, Condition: conditions[id], Reason: "initial position is outside arena"})
		}
	}
	ids := make([]string, 0, len(starts))
	for id := range starts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	ordered := make([]rolloutStart, 0, len(ids))
	for _, id := range ids {
		ordered = append(ordered, starts[id])
	}
	sort.Slice(excluded, func(i, j int) bool { return excluded[i].TrialID < excluded[j].TrialID })
	return ordered, excluded, nil
}

func runRolloutEngine(ctx context.Context, model savedModel, dataset trajectory.Dataset, targets []rolloutTarget, steps int) (rolloutReport, error) {
	if ctx == nil {
		return rolloutReport{}, errors.New("realnav rollout: nil context")
	}
	if err := ctx.Err(); err != nil {
		return rolloutReport{}, err
	}
	if err := validateRolloutSteps(steps); err != nil {
		return rolloutReport{}, err
	}
	if err := validateRolloutModel(model, dataset.Source.SHA256); err != nil {
		return rolloutReport{}, err
	}
	startTime := time.Now()
	filtered := onlyAfterRelocation(dataset)
	if err := validateRolloutSplit(filtered, model.Split); err != nil {
		return rolloutReport{}, err
	}
	conditions, err := trialConditions(dataset)
	if err != nil {
		return rolloutReport{}, err
	}
	starts, excluded, err := buildRolloutStarts(dataset, model.Split.Test, targets)
	if err != nil {
		return rolloutReport{}, err
	}
	if len(starts) == 0 {
		return rolloutReport{}, fmt.Errorf("realnav rollout: all held-out trials were excluded")
	}
	trainDataset, testDataset, trainSamples, testSamples, err := rebuildRolloutSplit(filtered, model.Split, conditions)
	if err != nil {
		return rolloutReport{}, err
	}
	protocol := currentRolloutProtocol(steps)
	protocolHash, err := rolloutProtocolHash(protocol)
	if err != nil {
		return rolloutReport{}, err
	}
	report := rolloutReport{
		SchemaVersion:           rolloutSchemaVersion,
		Mode:                    "rollout",
		Protocol:                protocol,
		ProtocolSHA256:          protocolHash,
		Source:                  sourceReport{ID: dataset.Source.ID, URL: dataset.Source.URL, DOI: dataset.Source.DOI, PinnedCommit: dataset.Source.PinnedCommit, SHA256: dataset.Source.SHA256, AuthorGitBlobSHA1: realGitBlobSHA1, AuthorReadmeBlobSHA1: realReadmeBlobSHA1, DryadByteEquivalence: "unverified", License: dataset.Source.License, Rows: len(dataset.Rows), Trials: len(trialIDs(dataset))},
		SourceSHA256:            dataset.Source.SHA256,
		SnapshotSHA256:          snapshotFingerprint(model.Snapshot),
		OriginalHoldoutTrialIDs: append([]string(nil), model.Split.Test...),
		Split:                   splitReport{Seed: splitSeed, TestFraction: testFraction, TrainTrialIDs: trialIDs(trainDataset), TestTrialIDs: trialIDs(testDataset), TrainTrialCounts: conditionCounts(trainDataset), TestTrialCounts: conditionCounts(testDataset), TrainSampleCounts: sampleConditionCounts(trainSamples), TestSampleCounts: sampleConditionCounts(testSamples)},
		Trials:                  make([]rolloutTrialReport, 0, len(starts)),
		Excluded:                excluded,
		Aggregates:              emptyRolloutAggregates(),
		Limitations: []string{
			"This is a frozen-model engineering rollout from two observed after_relocation rows, not a closed-loop animal dataset or connectome result.",
			"The source is downsampled by time and distance, so decision steps and the fixed 0.1 second feature are not animal seconds or a reconstructed velocity.",
			"The author's original preprint reports a 2.8 cm return-zone criterion, which is not reproduced here; this protocol uses a predeclared 2 cm engineering goal radius and does not claim an animal return result.",
			"The engineering arena has no obstacles, vision, olfaction, thermal wall, or animal control dynamics; target coordinates are evaluation-only metadata.",
		},
	}
	for _, trial := range starts {
		if err := ctx.Err(); err != nil {
			return rolloutReport{}, err
		}
		trialReport := rolloutTrialReport{TrialID: trial.TrialID, Condition: trial.Condition, Target: rolloutTargetReport{XCM: trial.Target.XCM, YCM: trial.Target.YCM, EvaluationOnly: true}, Strategies: make(map[rolloutStrategy]rolloutStrategyResult, len(rolloutStrategyNames))}
		for _, strategy := range rolloutStrategyNames {
			result, err := simulateRolloutTrial(ctx, model, trial, steps, strategy)
			if err != nil {
				return rolloutReport{}, fmt.Errorf("realnav rollout: trial %q strategy %q: %w", trial.TrialID, strategy, err)
			}
			trialReport.Strategies[strategy] = result
			aggregate := report.Aggregates[trial.Condition]
			if aggregate == nil {
				aggregate = make(map[rolloutStrategy]rolloutAggregate)
				report.Aggregates[trial.Condition] = aggregate
			}
			value := aggregate[strategy]
			value.Samples++
			if result.Success {
				value.Successes++
			}
			if result.StartSuccess {
				value.StartSuccesses++
			}
			value.Steps += result.Steps
			value.Collisions += result.Collisions
			value.ClippedActions += result.ClippedActions
			aggregate[strategy] = value
		}
		report.Trials = append(report.Trials, trialReport)
	}
	for _, byStrategy := range report.Aggregates {
		for strategy, value := range byStrategy {
			if value.Samples > 0 {
				value.SuccessRate = float64(value.Successes) / float64(value.Samples)
				value.StartSuccessRate = float64(value.StartSuccesses) / float64(value.Samples)
				value.MeanSteps = float64(value.Steps) / float64(value.Samples)
				value.MeanCollisions = float64(value.Collisions) / float64(value.Samples)
			}
			byStrategy[strategy] = value
		}
	}
	report.Runtime = runtimeReport{ElapsedMilliseconds: time.Since(startTime).Milliseconds(), MaxRSSBytes: maxRSSBytes(), GoVersion: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	return report, nil
}

// rebuildRolloutSplit records available causal samples without requiring a
// third observed row after the initial pair used by autonomous rollout.
func rebuildRolloutSplit(dataset trajectory.Dataset, split splitTrials, conditions map[string]string) (trajectory.Dataset, trajectory.Dataset, []causalSample, []causalSample, error) {

	trainDataset, err := selectTrials(dataset, split.Train)
	if err != nil {
		return trajectory.Dataset{}, trajectory.Dataset{}, nil, nil, fmt.Errorf("realnav rollout: select training trials: %w", err)
	}
	testDataset, err := selectTrials(dataset, split.Test)
	if err != nil {
		return trajectory.Dataset{}, trajectory.Dataset{}, nil, nil, fmt.Errorf("realnav rollout: select held-out trials: %w", err)
	}
	trainSamples, err := rebuildOptionalSamples(trainDataset, conditions)
	if err != nil {
		return trajectory.Dataset{}, trajectory.Dataset{}, nil, nil, fmt.Errorf("realnav rollout: rebuild optional training samples: %w", err)
	}
	testSamples, err := rebuildOptionalSamples(testDataset, conditions)
	if err != nil {
		return trajectory.Dataset{}, trajectory.Dataset{}, nil, nil, fmt.Errorf("realnav rollout: rebuild optional held-out samples: %w", err)
	}
	return trainDataset, testDataset, trainSamples, testSamples, nil
}

func rebuildOptionalSamples(dataset trajectory.Dataset, conditions map[string]string) ([]causalSample, error) {
	set, err := trajectory.BuildSamples(dataset, sampleConfig())
	if err != nil {
		return nil, err
	}
	if len(set.Samples) == 0 {
		return nil, nil
	}
	return convertSamples(set, conditions)
}

func emptyRolloutAggregates() map[string]map[rolloutStrategy]rolloutAggregate {
	aggregates := make(map[string]map[rolloutStrategy]rolloutAggregate, 2)
	for _, condition := range []string{"rewarded", "non-rewarded"} {
		byStrategy := make(map[rolloutStrategy]rolloutAggregate, len(rolloutStrategyNames))
		for _, strategy := range rolloutStrategyNames {
			byStrategy[strategy] = rolloutAggregate{}
		}
		aggregates[condition] = byStrategy
	}
	return aggregates
}

func validateRolloutStart(ctx context.Context, start rolloutStart, steps int, strategy rolloutStrategy) error {
	if ctx == nil {
		return errors.New("realnav rollout: nil context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateRolloutSteps(steps); err != nil {
		return err
	}
	if !finitePoint(start.XCM, start.YCM) || !finitePoint(start.PreviousDXCM, start.PreviousDYCM) || !isFinitePositive(start.PreviousDT) {
		return errors.New("realnav rollout: invalid initial observation")
	}
	if !finitePoint(start.Target.XCM, start.Target.YCM) || math.Hypot(start.Target.XCM, start.Target.YCM) > arenaRadiusCM {
		return errors.New("realnav rollout: invalid evaluation target")
	}
	if math.Hypot(start.XCM, start.YCM) > arenaRadiusCM {
		return errors.New("realnav rollout: initial observation is outside arena")
	}
	if strategy != rolloutStrategyModel && strategy != rolloutStrategyPersistent && strategy != rolloutStrategyZero && strategy != rolloutStrategyRandom && strategy != rolloutStrategyFixedAction {
		return fmt.Errorf("realnav rollout: unknown strategy %q", strategy)
	}
	return nil
}

func simulateRolloutTrial(ctx context.Context, model savedModel, start rolloutStart, steps int, strategy rolloutStrategy) (rolloutStrategyResult, error) {
	if err := validateRolloutStart(ctx, start, steps, strategy); err != nil {
		return rolloutStrategyResult{}, err
	}
	initialDistance := math.Hypot(start.XCM-start.Target.XCM, start.YCM-start.Target.YCM)
	result := rolloutStrategyResult{HitStep: -1, FinalDistanceCM: initialDistance, NearestDistanceCM: initialDistance}
	if initialDistance <= goalRadiusCM {
		result.Success = true
		result.StartSuccess = true
		result.HitStep = 0
		return result, nil
	}
	trace, err := simulateRolloutTraceInternal(ctx, model, start, steps, strategy, true)
	if err != nil {
		return rolloutStrategyResult{}, err
	}
	result.Steps = len(trace)
	for i, step := range trace {
		result.PathCM += math.Hypot(step.RealizedDXCM, step.RealizedDYCM)
		result.Collisions += step.Collision
		result.ClippedActions += step.Clipped
		result.FinalDistanceCM = math.Hypot(step.XCM-start.Target.XCM, step.YCM-start.Target.YCM)
		from := [2]float64{step.XCM - step.RealizedDXCM, step.YCM - step.RealizedDYCM}
		to := [2]float64{step.XCM, step.YCM}
		_, nearest := rolloutSegmentHitsTarget(start.Target, from, to)
		if nearest < result.NearestDistanceCM {
			result.NearestDistanceCM = nearest
		}
		if hit, _ := rolloutSegmentHitsTarget(start.Target, from, to); hit {
			result.Success = true
			result.HitStep = i + 1
			result.Steps = i + 1
			break
		}
	}
	return result, nil
}

func simulateRolloutTrace(ctx context.Context, model savedModel, start rolloutStart, steps int, strategy rolloutStrategy, fixed ...[2]float64) ([]rolloutTraceStep, error) {
	return simulateRolloutTraceInternal(ctx, model, start, steps, strategy, false, fixed...)
}

func simulateRolloutTraceInternal(ctx context.Context, model savedModel, start rolloutStart, steps int, strategy rolloutStrategy, stopOnHit bool, fixed ...[2]float64) ([]rolloutTraceStep, error) {
	if err := validateRolloutStart(ctx, start, steps, strategy); err != nil {
		return nil, err
	}
	if strategy == rolloutStrategyFixedAction && len(fixed) != 1 {
		return nil, errors.New("realnav rollout: fixed action requires one vector")
	}
	var individual *learning.Individual
	var err error
	if strategy == rolloutStrategyModel {
		// NewIndividual requires an explicit zero vector for the current
		// continuous core. It is the same zero initial state intended by the
		// rollout protocol.
		individual, err = learning.NewIndividual(model.Snapshot.Config, model.Snapshot.Parameters, model.Snapshot.Options, rolloutZeroNeuralState(model.Snapshot.Config))
		if err != nil {
			return nil, fmt.Errorf("create rollout individual: %w", err)
		}
	}
	rng := rolloutRandomForTrial(start.TrialID)
	trace := make([]rolloutTraceStep, 0, steps)
	x, y := start.XCM, start.YCM
	previousDX, previousDY, previousDT := start.PreviousDXCM, start.PreviousDYCM, start.PreviousDT
	for step := 0; step < steps; step++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		input := trajectory.Input{CurrentXCM: x, CurrentYCM: y, PreviousDXCM: previousDX, PreviousDYCM: previousDY, PreviousDT: previousDT}
		base := directionBaseline(causalSample{Input: sampleInput(input)}, model.TrainMeanDisplacement)
		action := base
		switch strategy {
		case rolloutStrategyModel:
			rows, advanceErr := individual.Advance(ctx, [][]float64{sampleInput(input)})
			if advanceErr != nil {
				return nil, fmt.Errorf("advance model: %w", advanceErr)
			}
			if len(rows) != 1 || len(rows[0]) != 2 || !finitePoint(rows[0][0], rows[0][1]) {
				return nil, errors.New("realnav rollout: model readout must be two finite values")
			}
			action = [2]float64{rows[0][0] + base[0], rows[0][1] + base[1]}
		case rolloutStrategyPersistent:
			action = base
		case rolloutStrategyZero:
			action = [2]float64{}
		case rolloutStrategyRandom:
			angle := rng.Float64() * 2 * math.Pi
			action = [2]float64{math.Cos(angle) * model.TrainMeanDisplacement, math.Sin(angle) * model.TrainMeanDisplacement}
		case rolloutStrategyFixedAction:
			if len(fixed) != 1 {
				return nil, errors.New("realnav rollout: fixed action requires one vector")
			}
			action = fixed[0]
		default:
			return nil, fmt.Errorf("realnav rollout: unknown strategy %q", strategy)
		}
		if !finitePoint(action[0], action[1]) {
			return nil, errors.New("realnav rollout: action is non-finite")
		}
		length := math.Hypot(action[0], action[1])
		clipped := 0
		if length > maxActionLengthCM {
			// Normalize before taking the norm so finite components whose
			// combined length overflows still preserve their direction.
			scale := math.Max(math.Abs(action[0]), math.Abs(action[1]))
			unitX, unitY := action[0]/scale, action[1]/scale
			factor := maxActionLengthCM / math.Hypot(unitX, unitY)
			action[0], action[1] = unitX*factor, unitY*factor
			clipped = 1
		}
		candidateX, candidateY := x+action[0], y+action[1]
		collision := 0
		realizedDX, realizedDY := action[0], action[1]
		if !finitePoint(candidateX, candidateY) {
			return nil, errors.New("realnav rollout: candidate position is non-finite")
		}
		if math.Hypot(candidateX, candidateY) > arenaRadiusCM {
			candidateX, candidateY = x, y
			realizedDX, realizedDY = 0, 0
			collision = 1
		}
		trace = append(trace, rolloutTraceStep{Input: input, ActionDXCM: action[0], ActionDYCM: action[1], RealizedDXCM: realizedDX, RealizedDYCM: realizedDY, Collision: collision, Clipped: clipped, XCM: candidateX, YCM: candidateY})
		if stopOnHit {
			from := [2]float64{x, y}
			to := [2]float64{candidateX, candidateY}
			if hit, _ := rolloutSegmentHitsTarget(start.Target, from, to); hit {
				break
			}
		}
		x, y = candidateX, candidateY
		previousDX, previousDY, previousDT = realizedDX, realizedDY, rolloutDecisionDT
		if individual != nil && (step+1)%trainingChunk == 0 && step+1 < steps {
			if err := individual.ResetNeural(ctx, rolloutZeroNeuralState(model.Snapshot.Config)); err != nil {
				return nil, fmt.Errorf("reset rollout neural state: %w", err)
			}
		}
	}
	return trace, nil
}

func rolloutZeroNeuralState(config learning.Config) []float64 {
	if config.LIF != nil {
		return make([]float64, config.LIF.Nodes)
	}
	if config.Mixed != nil {
		return make([]float64, config.Mixed.Nodes)
	}
	return make([]float64, config.Dynamics.Nodes)
}

func runRollout(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("realnav rollout", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: go run ./examples/realnav rollout --data PATH --snapshot PATH --out DIR [--steps N]")
		fmt.Fprintln(stderr, "Runs a frozen model from the earliest legal observed after_relocation pair in each held-out trial.")
		fmt.Fprintln(stderr, "Required: --data, --snapshot, --out. --steps defaults to 200 and must be 1..4096.")
		fmt.Fprintln(stderr, "Targets are evaluation-only virtual-zone metadata; decision steps are not animal seconds.")
		fmt.Fprintln(stderr, "The command refuses source or snapshot mismatches and an existing output directory.")
	}
	dataPath := fs.String("data", "", "external licensed trajectory CSV or CSV.gz with return-target metadata")
	snapshotPath := fs.String("snapshot", "", "saved model.json produced by train")
	outPath := fs.String("out", "", "new output directory outside the repository and data directory")
	steps := fs.Int("steps", defaultRolloutSteps, "maximum model decision steps per trial (1..4096)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("realnav rollout: unexpected positional arguments")
	}
	if *dataPath == "" || *snapshotPath == "" || *outPath == "" {
		return errors.New("realnav rollout: --data, --snapshot and --out are required")
	}
	if err := validateRolloutSteps(*steps); err != nil {
		return err
	}
	var model savedModel
	if err := readJSON(*snapshotPath, &model); err != nil {
		return fmt.Errorf("realnav rollout: read snapshot: %w", err)
	}
	if err := validateRolloutModel(model, ""); err != nil {
		return err
	}
	start := time.Now()
	dataset, returnTargets, err := trajectory.ReadWithReturnTargets(ctx, *dataPath, realSource(), trajectory.DefaultLimits())
	if err != nil {
		return err
	}
	report, err := runRolloutEngine(ctx, model, dataset, convertReturnTargets(returnTargets), *steps)
	if err != nil {
		return err
	}
	outputPath, err := prepareOutputDirectory(*outPath, *dataPath)
	if err != nil {
		return err
	}
	if report.Runtime.ElapsedMilliseconds == 0 {
		report.Runtime.ElapsedMilliseconds = time.Since(start).Milliseconds()
	}
	if err := writeJSONExclusive(filepath.Join(outputPath, "report.json"), report); err != nil {
		return err
	}
	return encodeRolloutReport(stdout, report)
}

func modelFirstAction(ctx context.Context, model savedModel, start rolloutStart) ([2]float64, error) {
	trace, err := simulateRolloutTrace(ctx, model, start, 1, rolloutStrategyModel)
	if err != nil {
		return [2]float64{}, err
	}
	return [2]float64{trace[0].ActionDXCM, trace[0].ActionDYCM}, nil
}

func rolloutSegmentHitsGoal(from, to [2]float64) (bool, float64) {
	return rolloutSegmentHitsTarget(rolloutTarget{}, from, to)
}

func rolloutSegmentHitsTarget(target rolloutTarget, from, to [2]float64) (bool, float64) {
	from[0] -= target.XCM
	from[1] -= target.YCM
	to[0] -= target.XCM
	to[1] -= target.YCM
	dx, dy := to[0]-from[0], to[1]-from[1]
	denom := dx*dx + dy*dy
	t := 0.0
	if denom > 0 {
		t = -(from[0]*dx + from[1]*dy) / denom
		if t < 0 {
			t = 0
		} else if t > 1 {
			t = 1
		}
	}
	closestX, closestY := from[0]+t*dx, from[1]+t*dy
	distance := math.Hypot(closestX, closestY)
	return distance <= goalRadiusCM, distance
}

func rolloutRandomForTrial(trialID string) *rand.Rand {
	digest := sha256.Sum256([]byte(trialID))
	var seed, stream uint64
	for i := 0; i < 8; i++ {
		seed = seed<<8 | uint64(digest[i])
		stream = stream<<8 | uint64(digest[8+i])
	}
	return rand.New(rand.NewPCG(randomDirectionSeed^seed, stream))
}

func finitePoint(x, y float64) bool {
	return !math.IsNaN(x) && !math.IsInf(x, 0) && !math.IsNaN(y) && !math.IsInf(y, 0)
}

func encodeRolloutReport(w io.Writer, value rolloutReport) error {
	if w == nil {
		return errors.New("realnav rollout: nil output writer")
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
