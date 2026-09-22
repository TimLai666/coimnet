package experiment

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/TimLai666/coimnet/experiment/gridnav"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/learning/rl"
)

// PPOExperimentSchemaVersion identifies the reproducible gridnav PPO report.
const PPOExperimentSchemaVersion = "coimnet-ppo-experiment/v1"

const (
	ppoMaxCorridorLength = 1001
	ppoMaxTimeLimit      = 10000
	ppoMaxSeeds          = 100
	ppoMaxUpdates        = 100000
	ppoMaxEvalEpisodes   = 10000

	ppoBaselineRNGStream uint64 = 0x1001
	ppoTrainRNGStream    uint64 = 0x1002
	ppoEvalRNGStream     uint64 = 0x1003
)

// PPOExperimentConfig fixes one reproducible recurrent PPO protocol.
type PPOExperimentConfig struct {
	Corridor     gridnav.Config `json:"corridor"`
	Seeds        []uint64       `json:"seeds"`
	Updates      int            `json:"updates"`
	Hidden       int            `json:"hidden"`
	EvalEpisodes int            `json:"eval_episodes"`
	LearningRate float64        `json:"learning_rate"`
	PPO          rl.PPOConfig   `json:"ppo"`
}

// DefaultPPOExperimentConfig returns the fixed small-corridor PPO protocol.
func DefaultPPOExperimentConfig() PPOExperimentConfig {
	return PPOExperimentConfig{
		Corridor:     gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: .01, GoalReward: 1},
		Seeds:        []uint64{1, 2, 3},
		Updates:      200,
		Hidden:       8,
		EvalEpisodes: 40,
		LearningRate: .01,
		PPO: rl.PPOConfig{
			Gamma:       .99,
			Lambda:      .95,
			ClipEpsilon: .2,
			ValueCoef:   .5,
			EntropyCoef: .01,
			TimeLimit:   20,
			Epochs:      1,
			MiniBatch:   1,
		},
	}
}

// Validate checks protocol, resource and PPO shape limits before any model is
// constructed or environment episode is started.
func (c PPOExperimentConfig) Validate() error {
	if !finite(c.Corridor.StepPenalty) || !finite(c.Corridor.GoalReward) {
		return fmt.Errorf("ppo corridor rewards must be finite")
	}
	if c.Corridor.Length > ppoMaxCorridorLength {
		return fmt.Errorf("ppo corridor length %d exceeds limit %d", c.Corridor.Length, ppoMaxCorridorLength)
	}
	if c.Corridor.TimeLimit > ppoMaxTimeLimit {
		return fmt.Errorf("ppo corridor time limit %d exceeds limit %d", c.Corridor.TimeLimit, ppoMaxTimeLimit)
	}
	if _, err := gridnav.New(c.Corridor); err != nil {
		return fmt.Errorf("ppo corridor: %w", err)
	}
	if len(c.Seeds) == 0 || len(c.Seeds) > ppoMaxSeeds {
		return fmt.Errorf("ppo seeds count %d, want a value in [1, %d]", len(c.Seeds), ppoMaxSeeds)
	}
	seen := make(map[uint64]bool, len(c.Seeds))
	for _, seed := range c.Seeds {
		if seen[seed] {
			return fmt.Errorf("ppo declares duplicate seed %d", seed)
		}
		seen[seed] = true
	}
	if c.Updates < 1 || c.Updates > ppoMaxUpdates {
		return fmt.Errorf("ppo updates %d, want a value in [1, %d]", c.Updates, ppoMaxUpdates)
	}
	if c.Hidden < 4 || c.Hidden > imitationMaxHidden {
		return fmt.Errorf("ppo hidden %d, want a value in [4, %d]", c.Hidden, imitationMaxHidden)
	}
	if c.EvalEpisodes < 1 || c.EvalEpisodes > ppoMaxEvalEpisodes {
		return fmt.Errorf("ppo eval episodes %d, want a value in [1, %d]", c.EvalEpisodes, ppoMaxEvalEpisodes)
	}
	if !finite(c.LearningRate) || c.LearningRate <= 0 {
		return fmt.Errorf("ppo learning rate %v must be finite and positive", c.LearningRate)
	}
	if err := c.PPO.Validate(); err != nil {
		return fmt.Errorf("ppo objective: %w", err)
	}
	if c.PPO.TimeLimit != c.Corridor.TimeLimit {
		return fmt.Errorf("ppo time limit %d must equal corridor time limit %d", c.PPO.TimeLimit, c.Corridor.TimeLimit)
	}
	if c.PPO.MiniBatch != 1 {
		return fmt.Errorf("ppo mini batch %d, want exactly 1", c.PPO.MiniBatch)
	}
	shortest := c.Corridor.Length / 2
	if c.Corridor.TimeLimit < shortest {
		shortest = c.Corridor.TimeLimit
	}
	if c.PPO.BurnIn >= shortest {
		return fmt.Errorf("ppo burn in %d must be smaller than shortest episode bound %d", c.PPO.BurnIn, shortest)
	}
	return nil
}

// PPOCurvePoint records the return and update loss of one training iteration.
type PPOCurvePoint struct {
	Update int     `json:"update"`
	Return float64 `json:"return"`
	Loss   float64 `json:"loss"`
}

// PPOSeedResult retains the complete outcome of one configured seed,
// including a failed seed's reason and any progress made before failure.
type PPOSeedResult struct {
	Seed                uint64          `json:"seed"`
	Failed              bool            `json:"failed"`
	Error               string          `json:"error,omitempty"`
	Updates             uint64          `json:"updates"`
	ReturnBefore        float64         `json:"return_before"`
	MeanReturn          float64         `json:"mean_return"`
	RandomReturn        float64         `json:"random_return"`
	Curve               []PPOCurvePoint `json:"curve"`
	PolicyVersionBefore string          `json:"policy_version_before"`
	PolicyVersionAfter  string          `json:"policy_version_after"`
	TerminalEpisodes    uint64          `json:"terminal_episodes"`
	TimeoutEpisodes     uint64          `json:"timeout_episodes"`
	FinalSnapshotHash   string          `json:"final_snapshot_hash"`
}

// PPOExperimentReport is the portable, JSON-serializable result of RunPPO.
type PPOExperimentReport struct {
	SchemaVersion    string              `json:"schema_version"`
	Config           PPOExperimentConfig `json:"config"`
	ConfigHash       string              `json:"config_hash"`
	Results          []PPOSeedResult     `json:"results"`
	MeanReturnBefore float64             `json:"mean_return_before"`
	MeanReturn       float64             `json:"mean_return"`
	StdReturn        float64             `json:"std_return"`
	Baseline         float64             `json:"random_policy_mean_return"`
	SuccessfulSeeds  int                 `json:"successful_seeds"`
	Passed           bool                `json:"passed"`
	Assumptions      []string            `json:"assumptions"`
}

// RunPPO trains and evaluates one independent recurrent policy per seed. A
// seed-level error is retained in Results while the run continues; context
// cancellation aborts the run and is returned to the caller.
func RunPPO(ctx context.Context, c PPOExperimentConfig) (PPOExperimentReport, error) {
	var report PPOExperimentReport
	if ctx == nil {
		return report, fmt.Errorf("ppo run needs a context")
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if err := c.Validate(); err != nil {
		return report, err
	}
	c.Seeds = append([]uint64(nil), c.Seeds...)
	report = PPOExperimentReport{
		SchemaVersion: PPOExperimentSchemaVersion,
		Config:        c,
		ConfigHash:    hash(c),
		Assumptions: []string{
			"Training actions use PCG(seed, 0x1002); evaluation actions use PCG(seed, 0x1003); random baseline actions use PCG(seed, 0x1001).",
			"Evaluation episodes use imitationEvalSeed(index) and do not read Expert, goal or position state.",
			"Each training iteration collects one zero-state rollout before one rl.Update call.",
		},
	}
	for _, seed := range c.Seeds {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		result, err := runPPOSeed(ctx, c, seed)
		if err != nil {
			if cerr := ctx.Err(); cerr != nil {
				return report, cerr
			}
			result.Failed = true
			result.Error = err.Error()
		}
		report.Results = append(report.Results, result)
	}
	finalizePPOReport(&report)
	return report, nil
}

func runPPOSeed(ctx context.Context, c PPOExperimentConfig, seed uint64) (PPOSeedResult, error) {
	result := PPOSeedResult{Seed: seed}
	baseline, err := ppoRandomReturn(ctx, c, seed)
	if err != nil {
		return result, err
	}
	result.RandomReturn = baseline
	ind, err := newPPOIndividual(seed, c.Hidden, c.LearningRate)
	if err != nil {
		return result, err
	}
	result.PolicyVersionBefore = rl.PolicyVersion(ind)
	result.PolicyVersionAfter = result.PolicyVersionBefore
	if result.ReturnBefore, err = evaluatePPOReturn(ctx, ind, c, seed, ppoEvalRNGStream); err != nil {
		return result, err
	}

	trainRNG := rand.New(rand.NewPCG(seed, ppoTrainRNGStream))
	for update := 0; update < c.Updates; update++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		envSeed := imitationTrainSeed(seed, update)
		rollout, episodeReturn, err := collectPPOEpisode(ctx, ind, c.Corridor, envSeed, trainRNG)
		if err != nil {
			return result, err
		}
		if len(rollout.Steps) == 0 || (!rollout.Steps[len(rollout.Steps)-1].Done && !rollout.Steps[len(rollout.Steps)-1].Timeout) {
			return result, fmt.Errorf("ppo update %d collector returned an incomplete episode", update+1)
		}
		last := rollout.Steps[len(rollout.Steps)-1]
		if last.Done {
			result.TerminalEpisodes++
		}
		if last.Timeout {
			result.TimeoutEpisodes++
		}
		updated, ppoReport, err := rl.Update(ctx, ind, []rl.Rollout{rollout}, gridnav.Actions, c.PPO)
		if err != nil {
			return result, fmt.Errorf("ppo update %d: %w", update+1, err)
		}
		if updated == nil {
			return result, fmt.Errorf("ppo update %d returned a nil individual", update+1)
		}
		if !finite(episodeReturn) || !finite(ppoReport.MeanLoss) {
			return result, fmt.Errorf("ppo update %d produced non-finite return or loss", update+1)
		}
		ind = updated
		result.Updates = ind.Snapshot().Optimizer.Updates
		result.PolicyVersionAfter = rl.PolicyVersion(ind)
		result.Curve = append(result.Curve, PPOCurvePoint{Update: update + 1, Return: episodeReturn, Loss: ppoReport.MeanLoss})
	}
	if result.MeanReturn, err = evaluatePPOReturn(ctx, ind, c, seed, ppoEvalRNGStream); err != nil {
		return result, err
	}
	if !finite(result.ReturnBefore) || !finite(result.MeanReturn) || !finite(result.RandomReturn) {
		return result, fmt.Errorf("ppo seed %d produced non-finite evaluation", seed)
	}
	result.Updates = ind.Snapshot().Optimizer.Updates
	result.FinalSnapshotHash = hash(ind.Snapshot())
	return result, nil
}

func evaluatePPOReturn(ctx context.Context, ind *learning.Individual, c PPOExperimentConfig, seed, stream uint64) (float64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	rng := rand.New(rand.NewPCG(seed, stream))
	var total float64
	for episode := 0; episode < c.EvalEpisodes; episode++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		_, episodeReturn, err := collectPPOEpisode(ctx, ind, c.Corridor, imitationEvalSeed(episode), rng)
		if err != nil {
			return 0, fmt.Errorf("ppo evaluation episode %d: %w", episode, err)
		}
		if !finite(episodeReturn) {
			return 0, fmt.Errorf("ppo evaluation episode %d produced a non-finite return", episode)
		}
		total += episodeReturn
		if !finite(total) {
			return 0, fmt.Errorf("ppo evaluation return overflowed at episode %d", episode)
		}
	}
	return total / float64(c.EvalEpisodes), nil
}

func ppoRandomReturn(ctx context.Context, c PPOExperimentConfig, seed uint64) (float64, error) {
	env, err := gridnav.New(c.Corridor)
	if err != nil {
		return 0, err
	}
	rng := rand.New(rand.NewPCG(seed, ppoBaselineRNGStream))
	var total float64
	for episode := 0; episode < c.EvalEpisodes; episode++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		env.Reset(imitationEvalSeed(episode))
		for {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			_, reward, done, _, err := env.Step(rng.IntN(gridnav.Actions))
			if err != nil {
				return 0, err
			}
			if !finite(reward) {
				return 0, fmt.Errorf("ppo random baseline produced a non-finite reward")
			}
			total += reward
			if !finite(total) {
				return 0, fmt.Errorf("ppo random baseline return overflowed at episode %d", episode)
			}
			if done {
				break
			}
		}
	}
	return total / float64(c.EvalEpisodes), nil
}

func finalizePPOReport(report *PPOExperimentReport) {
	beforeValues := make([]float64, 0, len(report.Results))
	returnValues := make([]float64, 0, len(report.Results))
	baselineValues := make([]float64, 0, len(report.Results))
	for _, result := range report.Results {
		if result.Failed || !finite(result.ReturnBefore) || !finite(result.MeanReturn) || !finite(result.RandomReturn) {
			continue
		}
		beforeValues = append(beforeValues, result.ReturnBefore)
		returnValues = append(returnValues, result.MeanReturn)
		baselineValues = append(baselineValues, result.RandomReturn)
	}
	report.SuccessfulSeeds = len(beforeValues)
	report.MeanReturnBefore, _ = stableMeanStd(beforeValues)
	report.MeanReturn, report.StdReturn = stableMeanStd(returnValues)
	report.Baseline, _ = stableMeanStd(baselineValues)
	report.Passed = len(report.Results) == len(report.Config.Seeds) &&
		report.SuccessfulSeeds == len(report.Config.Seeds) &&
		finite(report.MeanReturn) && finite(report.MeanReturnBefore) && finite(report.Baseline) && finite(report.StdReturn) &&
		report.MeanReturn > report.Baseline && report.MeanReturn > report.MeanReturnBefore
}

// stableMeanStd computes population statistics after scaling all values by
// their largest magnitude. The normalized sum and squared deviations stay
// bounded even when the original finite values are close to MaxFloat64.
func stableMeanStd(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	var scale float64
	for _, value := range values {
		if magnitude := math.Abs(value); magnitude > scale {
			scale = magnitude
		}
	}
	if scale == 0 {
		return 0, 0
	}
	normalized := make([]float64, len(values))
	var sum float64
	for i, value := range values {
		normalized[i] = value / scale
		sum += normalized[i]
	}
	meanNormalized := sum / float64(len(normalized))
	if meanNormalized > 1 {
		meanNormalized = 1
	} else if meanNormalized < -1 {
		meanNormalized = -1
	}
	var squared float64
	for _, value := range normalized {
		delta := value - meanNormalized
		squared += delta * delta
	}
	stdNormalized := math.Sqrt(squared / float64(len(normalized)))
	if stdNormalized > 1 {
		stdNormalized = 1
	}
	return scale * meanNormalized, scale * stdNormalized
}
