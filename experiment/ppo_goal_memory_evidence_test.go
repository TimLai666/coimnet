package experiment

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/TimLai666/coimnet/experiment/gridnav"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/learning/rl"
)

type ppoGoalStep struct {
	Observation []float64    `json:"observation"`
	Output      []float64    `json:"output,omitempty"`
	Action      int          `json:"action"`
	Reward      float64      `json:"reward"`
	Info        gridnav.Info `json:"evaluator_info"`
}

type ppoGoalEpisode struct {
	EnvSeed uint64        `json:"env_seed"`
	Cue     float64       `json:"original_cue"`
	Reached bool          `json:"reached"`
	Return  float64       `json:"return"`
	Trace   []ppoGoalStep `json:"trace"`
}

type ppoGoalMetrics struct {
	Episodes      int     `json:"episodes"`
	Reached       int     `json:"reached"`
	LeftEpisodes  int     `json:"left_episodes"`
	LeftReached   int     `json:"left_reached"`
	RightEpisodes int     `json:"right_episodes"`
	RightReached  int     `json:"right_reached"`
	MeanReturn    float64 `json:"mean_return"`
	MeanSteps     float64 `json:"mean_steps"`
}

type ppoGoalRun struct {
	Seed         uint64           `json:"seed"`
	Stage        string           `json:"stage"`
	Mode         string           `json:"mode"`
	Cue          string           `json:"cue"`
	SnapshotHash string           `json:"snapshot_hash,omitempty"`
	Metrics      ppoGoalMetrics   `json:"metrics"`
	Episodes     []ppoGoalEpisode `json:"episodes"`
}

func ppoGoalSummarize(episodes []ppoGoalEpisode) (ppoGoalMetrics, error) {
	var m ppoGoalMetrics
	if len(episodes) == 0 {
		return m, fmt.Errorf("empty goal evaluation")
	}
	returns := make([]float64, len(episodes))
	for i, ep := range episodes {
		if (ep.Cue != -1 && ep.Cue != 1) || len(ep.Trace) == 0 || !finite(ep.Return) {
			return m, fmt.Errorf("invalid goal episode %d", i)
		}
		m.Episodes++
		if ep.Reached {
			m.Reached++
		}
		if ep.Cue < 0 {
			m.LeftEpisodes++
			if ep.Reached {
				m.LeftReached++
			}
		} else {
			m.RightEpisodes++
			if ep.Reached {
				m.RightReached++
			}
		}
		returns[i] = ep.Return
		m.MeanSteps += float64(len(ep.Trace))
	}
	m.MeanReturn, _ = stableMeanStd(returns)
	m.MeanSteps /= float64(m.Episodes)
	return m, nil
}

// Descriptive criteria only: a long corridor may also be solved by searching
// both ends. A true gate is not proof that goal-cue memory is necessary.
func ppoGoalGate(runs []ppoGoalRun) bool {
	if len(runs) != 13 {
		return false
	}
	metrics := map[string]ppoGoalMetrics{}
	for _, run := range runs {
		if run.Seed != runs[0].Seed {
			return false
		}
	}
	for _, run := range runs {
		key := run.Stage + "/" + run.Mode + "/" + run.Cue
		if _, exists := metrics[key]; exists {
			return false
		}
		metrics[key] = run.Metrics
	}
	required := []string{"before/sampled/original", "random/random/original", "after/sampled/original", "after/sampled/flipped", "after/greedy/original"}
	for _, key := range required {
		m, ok := metrics[key]
		if !ok || m.Episodes <= 0 || m.LeftEpisodes <= 0 || m.RightEpisodes <= 0 || m.LeftEpisodes+m.RightEpisodes != m.Episodes || m.LeftReached < 0 || m.LeftReached > m.LeftEpisodes || m.RightReached < 0 || m.RightReached > m.RightEpisodes || m.LeftReached+m.RightReached != m.Reached {
			return false
		}
	}
	// Every before/after action mode must include both cue controls.
	for _, stage := range []string{"before", "after"} {
		for _, mode := range []string{"sampled", "greedy"} {
			for _, cue := range []string{"original", "erased", "flipped"} {
				m, ok := metrics[stage+"/"+mode+"/"+cue]
				if !ok || m.Episodes != DefaultPPOExperimentConfig().EvalEpisodes || m.LeftEpisodes != 20 || m.RightEpisodes != 20 || m.LeftReached < 0 || m.LeftReached > 20 || m.RightReached < 0 || m.RightReached > 20 || m.Reached != m.LeftReached+m.RightReached {
					return false
				}
			}
		}
	}
	trained := metrics[required[2]]
	rate := func(m ppoGoalMetrics) float64 { return float64(m.Reached) / float64(m.Episodes) }
	for _, key := range required {
		m := metrics[key]
		if m.LeftEpisodes != trained.LeftEpisodes || m.RightEpisodes != trained.RightEpisodes {
			return false
		}
	}
	const tol = 1e-12
	greedy := metrics[required[4]]
	return float64(trained.LeftReached)/float64(trained.LeftEpisodes)+tol >= .8 &&
		float64(trained.RightReached)/float64(trained.RightEpisodes)+tol >= .8 &&
		rate(trained)-rate(metrics[required[0]])+tol >= .15 &&
		rate(trained)-rate(metrics[required[1]])+tol >= .15 &&
		rate(trained)-rate(metrics[required[3]])+tol >= .2 &&
		greedy.Reached == greedy.Episodes
}

// This test-only evaluator owns its recurrent state. Evaluator Info never enters
// the encoder; only the first observation's cue may be changed.
func ppoGoalEpisodeRun(ctx context.Context, ind *learning.Individual, c gridnav.Config, envSeed uint64, mode, cue string, rng *rand.Rand) (ppoGoalEpisode, error) {
	var result ppoGoalEpisode
	if ctx == nil {
		return result, fmt.Errorf("goal evaluator needs context")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if mode != "sampled" && mode != "greedy" && mode != "random" {
		return result, fmt.Errorf("unknown action mode %q", mode)
	}
	if cue != "original" && cue != "erased" && cue != "flipped" {
		return result, fmt.Errorf("unknown cue control %q", cue)
	}
	if (mode == "sampled" || mode == "random") && rng == nil {
		return result, fmt.Errorf("action sampling needs RNG")
	}
	if mode != "random" && ind == nil {
		return result, fmt.Errorf("goal evaluator needs individual")
	}
	env, err := gridnav.New(c)
	if err != nil {
		return result, err
	}
	var episode *learning.Individual
	if mode != "random" {
		base := ind.Snapshot()
		if base.Plastic != nil || base.Chemical != nil || base.Config.InputSize != 4 || base.Config.OutputSize != gridnav.Actions+1 || !base.Config.ReadoutEveryStep {
			return result, fmt.Errorf("goal evaluator needs plain 4-input PPO model")
		}
		episode, err = learning.RestoreIndividual(base)
		if err != nil {
			return result, err
		}
		if err = episode.ResetNeural(ctx, make([]float64, len(base.Parameters.Core.Bias))); err != nil {
			return result, err
		}
	}
	obs, _ := env.Reset(envSeed)
	result.EnvSeed, result.Cue = envSeed, obs.Cue
	for i := 0; i < c.TimeLimit; i++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		vector := obs.Vector()
		if i == 0 {
			if cue == "erased" {
				vector[0] = 0
			}
			if cue == "flipped" {
				vector[0] = -vector[0]
			}
		}
		step := ppoGoalStep{Observation: vector}
		if mode == "random" {
			step.Action = int(rng.Float64() * float64(gridnav.Actions))
		} else {
			out, err := episode.Advance(ctx, [][]float64{vector})
			if err != nil {
				return result, err
			}
			if len(out) != 1 || len(out[0]) != gridnav.Actions+1 {
				return result, fmt.Errorf("invalid goal policy output shape")
			}
			for _, v := range out[0] {
				if !finite(v) {
					return result, fmt.Errorf("nonfinite goal policy output")
				}
			}
			step.Output = out[0]
			if mode == "sampled" {
				step.Action, err = samplePPOCollectAction(out[0][:gridnav.Actions], rng)
				if err != nil {
					return result, err
				}
			} else {
				for a := 1; a < gridnav.Actions; a++ {
					if out[0][a] > out[0][step.Action] {
						step.Action = a
					}
				}
			}
		}
		next, reward, done, info, err := env.Step(step.Action)
		if err != nil {
			return result, err
		}
		step.Reward, step.Info = reward, info
		result.Return += reward
		if !finite(result.Return) {
			return result, fmt.Errorf("goal return overflow")
		}
		result.Trace = append(result.Trace, step)
		if done {
			result.Reached = info.Reached
			return result, nil
		}
		obs = next
	}
	return result, fmt.Errorf("goal episode did not finish")
}

// Same initialization, environment seeds, action RNG and update path as RunPPO.
// The evidence entry checks its final full snapshot against RunPPO on every seed.
func trainPPOGoalFixture(ctx context.Context, c PPOExperimentConfig, seed uint64) (*learning.Individual, error) {
	if ctx == nil {
		return nil, fmt.Errorf("goal training needs context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	ind, err := newPPOIndividual(seed, c.Hidden, c.LearningRate)
	if err != nil {
		return nil, err
	}
	rng := rand.New(rand.NewPCG(seed, ppoTrainRNGStream))
	for update := 0; update < c.Updates; update++ {
		rollout, _, err := collectPPOEpisode(ctx, ind, c.Corridor, imitationTrainSeed(seed, update), rng)
		if err != nil {
			return nil, err
		}
		ind, _, err = rl.Update(ctx, ind, []rl.Rollout{rollout}, gridnav.Actions, c.PPO)
		if err != nil {
			return nil, err
		}
	}
	return ind, nil
}

// TestPPOGoalCueEvidence is an opt-in synthetic example; gate failure is a
// recorded research result, not a software-test failure. Never tunes a model.
// COIMNET_GOAL_CUE_EVIDENCE=/new/path go test -count=1 -run '^TestPPOGoalCueEvidence$' ./experiment
func TestPPOGoalCueEvidence(t *testing.T) {
	dir := os.Getenv("COIMNET_GOAL_CUE_EVIDENCE")
	if dir == "" {
		t.Skip("COIMNET_GOAL_CUE_EVIDENCE is not set")
	}
	path := filepath.Join(dir, "report.json")
	if _, err := os.Lstat(path); err == nil || !os.IsNotExist(err) {
		t.Fatalf("output must be absent: %s (%v)", path, err)
	}
	ctx := context.Background()
	c := DefaultPPOExperimentConfig()
	legacy, err := RunPPO(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	report := struct {
		SchemaVersion string              `json:"schema_version"`
		Config        PPOExperimentConfig `json:"config"`
		ConfigHash    string              `json:"config_hash"`
		EvalStream    uint64              `json:"eval_stream"`
		GateCriteria  string              `json:"gate_criteria"`
		GateBySeed    map[uint64]bool     `json:"goal_cue_gate_by_seed"`
		Gate          bool                `json:"goal_cue_gate"`
		Legacy        PPOExperimentReport `json:"legacy_report"`
		Runs          []ppoGoalRun        `json:"runs"`
	}{"coimnet-synthetic-goal-cue-audit/v1", c, hash(c), 0x1004,
		"each seed: sampled left/right >=0.8, success gain over before and random >=0.15, flipped-cue success drop >=0.2, greedy both goal sides succeed; 1e-12 comparison tolerance", map[uint64]bool{}, true, legacy, nil}
	for i, seed := range c.Seeds {
		before, err := newPPOIndividual(seed, c.Hidden, c.LearningRate)
		if err != nil {
			t.Fatal(err)
		}
		after, err := trainPPOGoalFixture(ctx, c, seed)
		if err != nil {
			t.Fatal(err)
		}
		if legacy.Results[i].Failed || hash(after.Snapshot()) != legacy.Results[i].FinalSnapshotHash || after.Snapshot().Optimizer.Updates != uint64(c.Updates) {
			t.Fatalf("seed %d training differs from RunPPO", seed)
		}
		start := len(report.Runs)
		for _, stage := range []string{"before", "after", "random"} {
			ind := before
			if stage == "after" {
				ind = after
			}
			modes := []string{"sampled", "greedy"}
			cues := []string{"original", "erased", "flipped"}
			if stage == "random" {
				modes = []string{"random"}
				cues = []string{"original"}
			}
			for _, mode := range modes {
				for _, cue := range cues {
					modelHash := hash(ind.Snapshot())
					run := ppoGoalRun{Seed: seed, Stage: stage, Mode: mode, Cue: cue}
					if stage != "random" {
						run.SnapshotHash = modelHash
					}
					for ep := 0; ep < c.EvalEpisodes; ep++ {
						envSeed := imitationEvalSeed(ep)
						rng := rand.New(rand.NewPCG(envSeed, 0x1004))
						result, err := ppoGoalEpisodeRun(ctx, ind, c.Corridor, envSeed, mode, cue, rng)
						if err != nil {
							t.Fatalf("seed %d %s/%s/%s episode %d: %v", seed, stage, mode, cue, ep, err)
						}
						run.Episodes = append(run.Episodes, result)
					}
					if hash(ind.Snapshot()) != modelHash {
						t.Fatal("evaluation changed full caller snapshot")
					}
					run.Metrics, err = ppoGoalSummarize(run.Episodes)
					if err != nil {
						t.Fatal(err)
					}
					report.Runs = append(report.Runs, run)
				}
			}
		}
		report.GateBySeed[seed] = ppoGoalGate(report.Runs[start:])
		report.Gate = report.Gate && report.GateBySeed[seed]
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.Write(append(data, '\n'))
	closeErr := file.Close()
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	t.Logf("39 runs, 1560 episodes; goal_cue_gate=%v, per seed=%v", report.Gate, report.GateBySeed)
}
