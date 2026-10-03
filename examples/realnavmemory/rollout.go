package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/rand"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/nav2d/trajectory"
)

const memoryRolloutSchema = "coimnet-realnav-memory-rollout/v1"

type memoryRolloutProtocol struct {
	SchemaVersion     string   `json:"schema_version"`
	ArenaRadiusCM     float64  `json:"arena_radius_cm"`
	GoalRadiusCM      float64  `json:"goal_radius_cm"`
	DecisionSteps     int      `json:"decision_steps"`
	DecisionDTSeconds float64  `json:"decision_dt_seconds"`
	GeneratedStimulus float64  `json:"generated_stimulus"`
	InputFeatures     []string `json:"input_features"`
	PrefixRule        string   `json:"prefix_rule"`
	TargetRole        string   `json:"target_role"`
	OutsideArenaRule  string   `json:"outside_arena_rule"`
	PersistenceRule   string   `json:"persistence_rule"`
	RandomRule        string   `json:"random_rule"`
	ParameterRule     string   `json:"parameter_rule"`
}

type memoryRolloutTarget struct {
	TrialID        string  `json:"trial_id"`
	XCM            float64 `json:"x_cm"`
	YCM            float64 `json:"y_cm"`
	EvaluationOnly bool    `json:"evaluation_only"`
}

type memoryRolloutStep struct {
	Index     int        `json:"index"`
	Input     [6]float64 `json:"input"`
	Action    [2]float64 `json:"action_cm"`
	Realized  [2]float64 `json:"realized_cm"`
	Position  [2]float64 `json:"position_cm"`
	Collision bool       `json:"collision"`
	Hit       bool       `json:"hit"`
}

type memoryRolloutTrial struct {
	TrialID     string              `json:"trial_id"`
	Condition   string              `json:"condition"`
	Target      memoryRolloutTarget `json:"target"`
	StartHit    bool                `json:"start_hit"`
	Success     bool                `json:"success"`
	HitStep     int                 `json:"hit_step"`
	Steps       int                 `json:"steps"`
	PathCM      float64             `json:"path_cm"`
	FinalDistCM float64             `json:"final_distance_cm"`
	NearestCM   float64             `json:"nearest_distance_cm"`
	Collisions  int                 `json:"collisions"`
	Trace       []memoryRolloutStep `json:"trace"`
}

type memoryRolloutAggregate struct {
	Samples        int     `json:"samples"`
	Successes      int     `json:"successes"`
	SuccessRate    float64 `json:"success_rate"`
	StartSuccesses int     `json:"start_successes"`
	Steps          int     `json:"steps"`
	Collisions     int     `json:"collisions"`
	MeanPathCM     float64 `json:"mean_path_cm"`
}

type memoryRolloutGroup struct {
	Seed                  uint64                 `json:"seed"`
	Control               string                 `json:"control"`
	Phase                 string                 `json:"phase"`
	Strategy              string                 `json:"strategy"`
	MeanStepLengthCM      float64                `json:"mean_step_length_cm"`
	ParameterSHA256       string                 `json:"parameter_sha256"`
	OptimizerSHA256       string                 `json:"optimizer_sha256"`
	ParameterBeforeSHA256 string                 `json:"parameter_before_sha256,omitempty"`
	ParameterAfterSHA256  string                 `json:"parameter_after_sha256,omitempty"`
	OptimizerBeforeSHA256 string                 `json:"optimizer_before_sha256,omitempty"`
	OptimizerAfterSHA256  string                 `json:"optimizer_after_sha256,omitempty"`
	ParameterFrozen       bool                   `json:"parameter_frozen"`
	OptimizerFrozen       bool                   `json:"optimizer_frozen"`
	Trials                []memoryRolloutTrial   `json:"trials"`
	Summary               memoryRolloutAggregate `json:"summary"`
}

type memoryRolloutReport struct {
	SchemaVersion   string                `json:"schema_version"`
	Mode            string                `json:"mode"`
	Protocol        memoryRolloutProtocol `json:"protocol"`
	Source          trajectory.Source     `json:"source"`
	SourceSHA256    string                `json:"source_sha256"`
	PlanFingerprint string                `json:"plan_fingerprint"`
	Split           trialSplit            `json:"split"`
	Groups          []memoryRolloutGroup  `json:"groups"`
	Limitations     []string              `json:"limitations"`
}

func memoryRolloutProtocolContract() memoryRolloutProtocol {
	return memoryRolloutProtocol{
		SchemaVersion:     memoryRolloutSchema,
		ArenaRadiusCM:     30,
		GoalRadiusCM:      2,
		DecisionSteps:     memoryDecisionRows,
		DecisionDTSeconds: 0.1,
		GeneratedStimulus: 0,
		InputFeatures:     append([]string(nil), memoryConfigContract().InputFeatures...),
		PrefixRule:        "replay every row before rollout_index once, then advance one current row and only generated rows",
		TargetRole:        "evaluation_only",
		OutsideArenaRule:  "reject the candidate endpoint, keep the position, realize zero displacement, and record collision",
		PersistenceRule:   "use the starting legal previous displacement direction at the training mean step length",
		RandomRule:        "math/rand.NewSource derived from seed and trial ID; target is never read for action generation",
		ParameterRule:     "rollout creates a fresh individual and cannot modify parameters or optimizer state",
	}
}

func buildMemoryRolloutReport(ctx context.Context, bundle memoryBundle, trials []historyTrial, targets []trajectory.ReturnTarget) (memoryRolloutReport, error) {
	if err := ctx.Err(); err != nil {
		return memoryRolloutReport{}, err
	}
	testTrials, err := selectTrials(trials, bundle.Split.Test)
	if err != nil {
		return memoryRolloutReport{}, err
	}
	targetByID, err := memoryRolloutTargets(targets, bundle.Split.Test)
	if err != nil {
		return memoryRolloutReport{}, err
	}
	groups := make([]memoryRolloutGroup, 0, len(bundle.Runs)*2+len(bundle.Seeds)*2)
	for _, run := range bundle.Runs {
		if err := ctx.Err(); err != nil {
			return memoryRolloutReport{}, err
		}
		before, err := memoryRunRollout(ctx, run.Before, testTrials, targetByID, run.Control, run.Seed, run.MeanStepLen)
		if err != nil {
			return memoryRolloutReport{}, fmt.Errorf("rollout before %d/%s: %w", run.Seed, run.Control, err)
		}
		before.Seed, before.Control, before.Phase = run.Seed, run.Control, "before"
		after, err := memoryRunRollout(ctx, run.After, testTrials, targetByID, run.Control, run.Seed, run.MeanStepLen)
		if err != nil {
			return memoryRolloutReport{}, fmt.Errorf("rollout after %d/%s: %w", run.Seed, run.Control, err)
		}
		after.Seed, after.Control, after.Phase = run.Seed, run.Control, "after"
		groups = append(groups, before, after)
	}
	for _, seed := range bundle.Seeds {
		run, err := memoryBundleRun(bundle, seed, "delivered")
		if err != nil {
			return memoryRolloutReport{}, err
		}
		for _, strategy := range []string{"persistence", "random"} {
			group, err := memoryBaselineRollout(ctx, testTrials, targetByID, strategy, seed, run.MeanStepLen)
			if err != nil {
				return memoryRolloutReport{}, fmt.Errorf("rollout %s %d: %w", strategy, seed, err)
			}
			group.Seed, group.Control, group.Phase = seed, "baseline", "baseline"
			groups = append(groups, group)
		}
	}
	return memoryRolloutReport{
		SchemaVersion:   memoryRolloutSchema,
		Mode:            "rollout",
		Protocol:        memoryRolloutProtocolContract(),
		Source:          bundle.Source,
		SourceSHA256:    bundle.SourceSHA256,
		PlanFingerprint: bundle.PlanFingerprint,
		Split:           copyTrialSplit(bundle.Split),
		Groups:          groups,
		Limitations: []string{
			"The arena, action, and target rules are an engineering evaluator and do not claim animal navigation or a connectome mechanism.",
			"Targets are read separately from the history model input and are used only for scoring.",
			"Before/after labels refer to frozen training snapshots; persistence and random baselines are one baseline phase per seed.",
		},
	}, nil
}

func memoryRolloutTargets(targets []trajectory.ReturnTarget, testIDs []string) (map[string]memoryRolloutTarget, error) {
	wanted := make(map[string]struct{}, len(testIDs))
	for _, id := range testIDs {
		if id == "" {
			return nil, errors.New("realnavmemory: test trial ID is empty")
		}
		if _, exists := wanted[id]; exists {
			return nil, fmt.Errorf("realnavmemory: duplicate test trial ID %q", id)
		}
		wanted[id] = struct{}{}
	}
	result := make(map[string]memoryRolloutTarget, len(testIDs))
	for _, target := range targets {
		if target.TrialID == "" {
			return nil, errors.New("realnavmemory: return target has empty trial ID")
		}
		if !finite(target.XCM) || !finite(target.YCM) {
			return nil, fmt.Errorf("realnavmemory: return target for %q is non-finite", target.TrialID)
		}
		if _, ok := wanted[target.TrialID]; !ok {
			continue
		}
		if _, duplicate := result[target.TrialID]; duplicate {
			return nil, fmt.Errorf("realnavmemory: duplicate return target for %q", target.TrialID)
		}
		if math.Hypot(target.XCM, target.YCM) > 30 {
			return nil, fmt.Errorf("realnavmemory: return target for %q is outside the arena", target.TrialID)
		}
		result[target.TrialID] = memoryRolloutTarget{TrialID: target.TrialID, XCM: target.XCM, YCM: target.YCM, EvaluationOnly: true}
	}
	if len(result) != len(wanted) {
		for id := range wanted {
			if _, ok := result[id]; !ok {
				return nil, fmt.Errorf("realnavmemory: no return target for test trial %q", id)
			}
		}
	}
	return result, nil
}

func memoryRunRollout(ctx context.Context, snapshot learning.TrainingSnapshot, trials []historyTrial, targets map[string]memoryRolloutTarget, control string, seed uint64, meanStepLength float64) (memoryRolloutGroup, error) {
	if control != "delivered" && control != "no_stimulus" && control != "shuffled_stimulus" {
		return memoryRolloutGroup{}, fmt.Errorf("unknown rollout control %q", control)
	}
	if !finite(meanStepLength) || meanStepLength < 0 {
		return memoryRolloutGroup{}, errors.New("invalid rollout mean step length")
	}
	parametersHash := memoryJSONHash(snapshot.Parameters)
	optimizerHash := memoryJSONHash(snapshot.Optimizer)
	results := make([]memoryRolloutTrial, 0, len(trials))
	for _, trial := range trials {
		if err := ctx.Err(); err != nil {
			return memoryRolloutGroup{}, err
		}
		target, ok := targets[trial.ID]
		if !ok {
			return memoryRolloutGroup{}, fmt.Errorf("no evaluation target for test trial %q", trial.ID)
		}
		// Each trial is a fresh zero-state individual. Reusing one individual
		// would leak the previous trial's neural state into this trial.
		individual, err := learning.NewIndividual(snapshot.Config, snapshot.Parameters, snapshot.Options, make([]float64, snapshot.Config.Dynamics.Nodes))
		if err != nil {
			return memoryRolloutGroup{}, fmt.Errorf("new individual for trial %q: %w", trial.ID, err)
		}
		result, err := memoryModelTrial(ctx, individual, trial, target, control, seed)
		if err != nil {
			return memoryRolloutGroup{}, err
		}
		final := individual.Snapshot()
		if memoryJSONHash(final.Parameters) != parametersHash {
			return memoryRolloutGroup{}, errors.New("rollout mutated parameters")
		}
		results = append(results, result)
	}
	return memoryRolloutGroup{Strategy: "model", MeanStepLengthCM: meanStepLength, ParameterSHA256: parametersHash, OptimizerSHA256: optimizerHash, ParameterBeforeSHA256: parametersHash, ParameterAfterSHA256: parametersHash, OptimizerBeforeSHA256: optimizerHash, OptimizerAfterSHA256: optimizerHash, ParameterFrozen: true, OptimizerFrozen: true, Trials: results, Summary: memoryRolloutSummary(results)}, nil
}

func memoryModelTrial(ctx context.Context, individual *learning.Individual, trial historyTrial, target memoryRolloutTarget, control string, seed uint64) (memoryRolloutTrial, error) {
	if individual == nil {
		return memoryRolloutTrial{}, errors.New("nil rollout individual")
	}
	if trial.RolloutIndex < 0 || trial.RolloutIndex >= len(trial.Steps) {
		return memoryRolloutTrial{}, fmt.Errorf("trial %q has invalid rollout index %d", trial.ID, trial.RolloutIndex)
	}
	// The truncated copy is intentional. It prevents a future row from being
	// consulted while trialInputs constructs the shuffled prefix.
	prefixTrial := trial
	prefixTrial.Steps = append([]historyStep(nil), trial.Steps[:trial.RolloutIndex+1]...)
	inputs, err := trialInputs(prefixTrial, control, int64(seed))
	if err != nil {
		return memoryRolloutTrial{}, err
	}
	if trial.RolloutIndex > 0 {
		if _, err := individual.Advance(ctx, inputs[:trial.RolloutIndex]); err != nil {
			return memoryRolloutTrial{}, fmt.Errorf("replay prefix: %w", err)
		}
	}
	start := trial.Steps[trial.RolloutIndex]
	x, y := start.Input[0]*30, start.Input[1]*30
	previousDX, previousDY := start.Input[2]*2, start.Input[3]*2
	if !finite(x) || !finite(y) || !finite(previousDX) || !finite(previousDY) {
		return memoryRolloutTrial{}, fmt.Errorf("trial %q has non-finite rollout start", trial.ID)
	}
	if math.Hypot(x, y) > 30 {
		return memoryRolloutTrial{}, fmt.Errorf("trial %q starts outside arena", trial.ID)
	}
	// Keep raw delivered-stimulus blocks separately from the transformed input
	// blocks that the model saw. Shuffling a transformed block would apply the
	// delay repeatedly and could echo a positive stimulus forever. Generated
	// rows carry raw zero stimulus, so later blocks remain causal zeros.
	rawStimulusBlocks := make(map[int][]float64)
	for i := 0; i <= trial.RolloutIndex; i++ {
		block := i / memoryBlockRows
		values := rawStimulusBlocks[block]
		if values == nil {
			values = make([]float64, memoryBlockRows)
			rawStimulusBlocks[block] = values
		}
		values[i%memoryBlockRows] = prefixTrial.Steps[i].Input[5]
	}
	currentInput := append([]float64(nil), inputs[trial.RolloutIndex]...)
	if len(currentInput) != 6 {
		return memoryRolloutTrial{}, fmt.Errorf("trial %q current input has width %d", trial.ID, len(currentInput))
	}
	trace := make([]memoryRolloutStep, 0, memoryDecisionRows)
	result := memoryRolloutTrial{TrialID: trial.ID, Condition: trial.Condition, Target: target, HitStep: -1}
	nearest := math.Hypot(x-target.XCM, y-target.YCM)
	if nearest <= 2 {
		result.StartHit = true
		result.HitStep = 0
	}
	if control == "" {
		return memoryRolloutTrial{}, errors.New("rollout control is empty")
	}
	for stepIndex := 0; stepIndex < memoryDecisionRows; stepIndex++ {
		if err := ctx.Err(); err != nil {
			return memoryRolloutTrial{}, err
		}
		var action [2]float64
		var input [6]float64
		if stepIndex == 0 {
			copy(input[:], currentInput)
		} else {
			input = [6]float64{x / 30, y / 30, previousDX / 2, previousDY / 2, 0.1, memoryGeneratedStimulus(rawStimulusBlocks, trial.RolloutIndex+stepIndex, control, seed)}
		}
		output, err := individual.Advance(ctx, [][]float64{{input[0], input[1], input[2], input[3], input[4], input[5]}})
		if err != nil {
			return memoryRolloutTrial{}, fmt.Errorf("generated row %d: %w", stepIndex, err)
		}
		if len(output) != 1 || len(output[0]) != 2 || !finite(output[0][0]) || !finite(output[0][1]) {
			return memoryRolloutTrial{}, fmt.Errorf("generated row %d has invalid model output", stepIndex)
		}
		action = [2]float64{output[0][0], output[0][1]}
		candidate, realized, collision := memoryArenaStep(x, y, action)
		x, y = candidate[0], candidate[1]
		previousDX, previousDY = realized[0], realized[1]
		if collision {
			result.Collisions++
		}
		result.PathCM += math.Hypot(realized[0], realized[1])
		distance := math.Hypot(x-target.XCM, y-target.YCM)
		if distance < nearest {
			nearest = distance
		}
		if result.HitStep < 0 && memorySegmentHits(target.XCM, target.YCM, x-realized[0], y-realized[1], x, y) {
			result.HitStep = stepIndex + 1
		}
		trace = append(trace, memoryRolloutStep{Index: stepIndex, Input: input, Action: action, Realized: realized, Position: [2]float64{x, y}, Collision: collision, Hit: result.HitStep >= 0 && result.HitStep == stepIndex+1})
		absoluteIndex := trial.RolloutIndex + stepIndex
		block := absoluteIndex / memoryBlockRows
		values := rawStimulusBlocks[block]
		if values == nil {
			values = make([]float64, memoryBlockRows)
			rawStimulusBlocks[block] = values
		}
		if stepIndex > 0 {
			values[absoluteIndex%memoryBlockRows] = 0
		}
	}
	result.Success = result.HitStep >= 0
	result.Steps = len(trace)
	result.FinalDistCM = math.Hypot(x-target.XCM, y-target.YCM)
	result.NearestCM = nearest
	result.Trace = trace
	return result, nil
}

func memoryGeneratedStimulus(blocks map[int][]float64, index int, control string, seed uint64) float64 {
	if control != "shuffled_stimulus" {
		return 0
	}
	block := index / memoryBlockRows
	if block == 0 {
		return 0
	}
	previous := blocks[block-1]
	if len(previous) != memoryBlockRows {
		return 0
	}
	key := block
	shuffled := blocks[-key]
	if shuffled == nil {
		shuffled = append([]float64(nil), previous...)
		permuteStimulusBlock(shuffled, int64(seed)+int64(block))
		blocks[-key] = shuffled
	}
	return shuffled[index%memoryBlockRows]
}

func memoryBaselineRollout(ctx context.Context, trials []historyTrial, targets map[string]memoryRolloutTarget, strategy string, seed uint64, meanStepLength float64) (memoryRolloutGroup, error) {
	if strategy != "persistence" && strategy != "random" {
		return memoryRolloutGroup{}, fmt.Errorf("unknown baseline strategy %q", strategy)
	}
	if !finite(meanStepLength) || meanStepLength < 0 {
		return memoryRolloutGroup{}, errors.New("invalid baseline mean step length")
	}
	results := make([]memoryRolloutTrial, 0, len(trials))
	for _, trial := range trials {
		if err := ctx.Err(); err != nil {
			return memoryRolloutGroup{}, err
		}
		result, err := memoryBaselineTrial(ctx, trial, targets[trial.ID], strategy, seed, meanStepLength)
		if err != nil {
			return memoryRolloutGroup{}, err
		}
		results = append(results, result)
	}
	return memoryRolloutGroup{Strategy: strategy, MeanStepLengthCM: meanStepLength, ParameterFrozen: true, OptimizerFrozen: true, Trials: results, Summary: memoryRolloutSummary(results)}, nil
}

func memoryBaselineTrial(ctx context.Context, trial historyTrial, target memoryRolloutTarget, strategy string, seed uint64, meanStepLength float64) (memoryRolloutTrial, error) {
	if trial.RolloutIndex < 0 || trial.RolloutIndex >= len(trial.Steps) {
		return memoryRolloutTrial{}, fmt.Errorf("trial %q has invalid rollout index %d", trial.ID, trial.RolloutIndex)
	}
	start := trial.Steps[trial.RolloutIndex]
	x, y := start.Input[0]*30, start.Input[1]*30
	previousDX, previousDY := start.Input[2]*2, start.Input[3]*2
	if !finite(x) || !finite(y) || !finite(previousDX) || !finite(previousDY) || math.Hypot(x, y) > 30 {
		return memoryRolloutTrial{}, fmt.Errorf("trial %q has invalid rollout start", trial.ID)
	}
	result := memoryRolloutTrial{TrialID: trial.ID, Condition: trial.Condition, Target: target, HitStep: -1}
	nearest := math.Hypot(x-target.XCM, y-target.YCM)
	if nearest <= 2 {
		result.StartHit = true
		result.HitStep = 0
	}
	rng := memoryRandomStream(seed, trial.ID)
	direction := [2]float64{previousDX, previousDY}
	if norm := math.Hypot(direction[0], direction[1]); norm > 0 {
		direction[0] /= norm
		direction[1] /= norm
	} else {
		direction = [2]float64{}
	}
	trace := make([]memoryRolloutStep, 0, memoryDecisionRows)
	for stepIndex := 0; stepIndex < memoryDecisionRows; stepIndex++ {
		if err := ctx.Err(); err != nil {
			return memoryRolloutTrial{}, err
		}
		input := [6]float64{x / 30, y / 30, previousDX / 2, previousDY / 2, 0.1, 0}
		var action [2]float64
		if strategy == "persistence" {
			action = [2]float64{direction[0] * meanStepLength, direction[1] * meanStepLength}
		} else {
			angle := rng.Float64() * 2 * math.Pi
			action = [2]float64{math.Cos(angle) * meanStepLength, math.Sin(angle) * meanStepLength}
		}
		candidate, realized, collision := memoryArenaStep(x, y, action)
		x, y = candidate[0], candidate[1]
		previousDX, previousDY = realized[0], realized[1]
		if collision {
			result.Collisions++
		}
		result.PathCM += math.Hypot(realized[0], realized[1])
		distance := math.Hypot(x-target.XCM, y-target.YCM)
		if distance < nearest {
			nearest = distance
		}
		if result.HitStep < 0 && memorySegmentHits(target.XCM, target.YCM, x-realized[0], y-realized[1], x, y) {
			result.HitStep = stepIndex + 1
		}
		trace = append(trace, memoryRolloutStep{Index: stepIndex, Input: input, Action: action, Realized: realized, Position: [2]float64{x, y}, Collision: collision, Hit: result.HitStep >= 0 && result.HitStep == stepIndex+1})
	}
	result.Success = result.HitStep >= 0
	result.Steps = len(trace)
	result.FinalDistCM = math.Hypot(x-target.XCM, y-target.YCM)
	result.NearestCM = nearest
	result.Trace = trace
	return result, nil
}

func memoryArenaStep(x, y float64, action [2]float64) ([2]float64, [2]float64, bool) {
	if !finite(action[0]) || !finite(action[1]) {
		return [2]float64{x, y}, [2]float64{}, true
	}
	candidate := [2]float64{x + action[0], y + action[1]}
	if math.Hypot(candidate[0], candidate[1]) > 30 {
		return [2]float64{x, y}, [2]float64{}, true
	}
	return candidate, action, false
}

func memorySegmentHits(targetX, targetY, x0, y0, x1, y1 float64) bool {
	dx, dy := x1-x0, y1-y0
	denom := dx*dx + dy*dy
	parameter := 0.0
	if denom > 0 {
		parameter = ((targetX-x0)*dx + (targetY-y0)*dy) / denom
		if parameter < 0 {
			parameter = 0
		} else if parameter > 1 {
			parameter = 1
		}
	}
	closestX := x0 + parameter*dx
	closestY := y0 + parameter*dy
	return math.Hypot(targetX-closestX, targetY-closestY) <= 2
}

func memoryRandomStream(seed uint64, trialID string) *rand.Rand {
	digest := sha256.Sum256(append([]byte(fmt.Sprintf("%d\x1f", seed)), []byte(trialID)...))
	streamSeed := int64(binary.LittleEndian.Uint64(digest[:8]) ^ seed)
	return rand.New(rand.NewSource(streamSeed))
}

func memoryRolloutSummary(results []memoryRolloutTrial) memoryRolloutAggregate {
	var summary memoryRolloutAggregate
	summary.Samples = len(results)
	for _, result := range results {
		if result.Success {
			summary.Successes++
		}
		if result.StartHit {
			summary.StartSuccesses++
		}
		summary.Steps += result.Steps
		summary.Collisions += result.Collisions
		summary.MeanPathCM += result.PathCM
	}
	if summary.Samples > 0 {
		summary.SuccessRate = float64(summary.Successes) / float64(summary.Samples)
		summary.MeanPathCM /= float64(summary.Samples)
	}
	return summary
}

func memoryBundleRun(bundle memoryBundle, seed uint64, control string) (memoryRun, error) {
	for _, run := range bundle.Runs {
		if run.Seed == seed && run.Control == control {
			return run, nil
		}
	}
	return memoryRun{}, fmt.Errorf("realnavmemory: missing bundle run %d/%s", seed, control)
}
