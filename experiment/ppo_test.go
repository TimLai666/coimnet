package experiment

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/experiment/gridnav"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/learning/rl"
)

func smallPPOExperimentConfig() PPOExperimentConfig {
	c := DefaultPPOExperimentConfig()
	c.Updates = 4
	c.EvalEpisodes = 3
	return c
}

func TestPPODefaultConfig(t *testing.T) {
	c := DefaultPPOExperimentConfig()
	if err := c.Validate(); err != nil {
		t.Fatalf("default config rejected: %v", err)
	}
	if !reflect.DeepEqual(c.Seeds, []uint64{1, 2, 3}) {
		t.Fatalf("default seeds = %v, want [1 2 3]", c.Seeds)
	}
	if c.Updates != 200 || c.Hidden != 8 || c.EvalEpisodes != 40 || c.LearningRate != .01 {
		t.Fatalf("default budget/model = updates=%d hidden=%d eval=%d rate=%g", c.Updates, c.Hidden, c.EvalEpisodes, c.LearningRate)
	}
	if c.Corridor != (gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: .01, GoalReward: 1}) {
		t.Fatalf("default corridor = %+v", c.Corridor)
	}
	wantPPO := rl.PPOConfig{Gamma: .99, Lambda: .95, ClipEpsilon: .2, ValueCoef: .5, EntropyCoef: .01, BurnIn: 0, TimeLimit: 20, Epochs: 1, MiniBatch: 1}
	if c.PPO != wantPPO {
		t.Fatalf("default PPO = %+v, want %+v", c.PPO, wantPPO)
	}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal default config: %v", err)
	}
	var round PPOExperimentConfig
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("unmarshal default config: %v", err)
	}
	if !reflect.DeepEqual(c, round) {
		t.Fatalf("default config JSON round trip changed it:\n%+v\n%+v", c, round)
	}
}

func TestPPOExperimentRNGStreamsAreSeparated(t *testing.T) {
	if ppoBaselineRNGStream == ppoTrainRNGStream || ppoBaselineRNGStream == ppoEvalRNGStream || ppoTrainRNGStream == ppoEvalRNGStream {
		t.Fatalf("PPO action RNG streams overlap: baseline=%#x train=%#x eval=%#x", ppoBaselineRNGStream, ppoTrainRNGStream, ppoEvalRNGStream)
	}
	if ppoBaselineRNGStream == 0 || ppoTrainRNGStream == 0 || ppoEvalRNGStream == 0 || ppoBaselineRNGStream == 2 || ppoTrainRNGStream == 2 || ppoEvalRNGStream == 2 {
		t.Fatalf("PPO action RNG stream overlaps model initialization stream: baseline=%#x train=%#x eval=%#x", ppoBaselineRNGStream, ppoTrainRNGStream, ppoEvalRNGStream)
	}
	report, err := RunPPO(context.Background(), smallPPOExperimentConfig())
	if err != nil {
		t.Fatalf("RunPPO: %v", err)
	}
	assumptions := strings.Join(report.Assumptions, "\n")
	for _, stream := range []string{"0x1001", "0x1002", "0x1003"} {
		if !strings.Contains(assumptions, stream) {
			t.Fatalf("report assumptions omit RNG stream %s: %q", stream, assumptions)
		}
	}
}

func TestPPOExperimentRejectsInvalid(t *testing.T) {
	cases := []struct {
		name  string
		patch func(*PPOExperimentConfig)
	}{
		{"no seeds", func(c *PPOExperimentConfig) { c.Seeds = nil }},
		{"duplicate seeds", func(c *PPOExperimentConfig) { c.Seeds = []uint64{1, 1} }},
		{"too many seeds", func(c *PPOExperimentConfig) { c.Seeds = make([]uint64, 101) }},
		{"updates below one", func(c *PPOExperimentConfig) { c.Updates = 0 }},
		{"updates above limit", func(c *PPOExperimentConfig) { c.Updates = 100001 }},
		{"hidden below four", func(c *PPOExperimentConfig) { c.Hidden = 3 }},
		{"hidden above limit", func(c *PPOExperimentConfig) { c.Hidden = 513 }},
		{"eval episodes below one", func(c *PPOExperimentConfig) { c.EvalEpisodes = 0 }},
		{"eval episodes above limit", func(c *PPOExperimentConfig) { c.EvalEpisodes = 10001 }},
		{"nonfinite learning rate", func(c *PPOExperimentConfig) { c.LearningRate = math.NaN() }},
		{"corridor length above limit", func(c *PPOExperimentConfig) { c.Corridor.Length = 1003 }},
		{"time limit above limit", func(c *PPOExperimentConfig) { c.Corridor.TimeLimit = 10001; c.PPO.TimeLimit = 10001 }},
		{"PPO time limit mismatch", func(c *PPOExperimentConfig) { c.PPO.TimeLimit++ }},
		{"PPO minibatch", func(c *PPOExperimentConfig) { c.PPO.MiniBatch = 2 }},
		{"burn in reaches minimum episode", func(c *PPOExperimentConfig) { c.PPO.BurnIn = c.Corridor.Length / 2 }},
		{"nonfinite PPO coefficient", func(c *PPOExperimentConfig) { c.PPO.Gamma = math.Inf(1) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := DefaultPPOExperimentConfig()
			tc.patch(&c)
			if err := c.Validate(); err == nil {
				t.Fatalf("Validate accepted %s", tc.name)
			}
		})
	}
}

func TestPPOExperimentModelLayout(t *testing.T) {
	ind, err := newPPOIndividual(7, 8, .01)
	if err != nil {
		t.Fatalf("newPPOIndividual: %v", err)
	}
	s := ind.Snapshot()
	if s.Config.Dynamics.Nodes != 12 || len(s.Config.Dynamics.Sources) != 96 || len(s.Config.Dynamics.Targets) != 96 {
		t.Fatalf("model topology = nodes %d edges %d, want nodes 12 edges 96", s.Config.Dynamics.Nodes, len(s.Config.Dynamics.Sources))
	}
	if s.Config.InputSize != 4 || s.Config.OutputSize != 4 || !s.Config.ReadoutEveryStep {
		t.Fatalf("model IO = input %d output %d every_step %v", s.Config.InputSize, s.Config.OutputSize, s.Config.ReadoutEveryStep)
	}
	if !reflect.DeepEqual(s.Config.InputNodes, []int{0, 1, 2, 3}) || len(s.Config.ReadoutNodes) != 12 {
		t.Fatalf("model input/readout nodes = %v/%v", s.Config.InputNodes, s.Config.ReadoutNodes)
	}
	for i, v := range s.Parameters.Encoder {
		want := 0.0
		if i/4 == i%4 {
			want = 1
		}
		if v != want {
			t.Fatalf("encoder[%d] = %g, want %g", i, v, want)
		}
	}
	for i, v := range s.Parameters.Core.LogTau {
		if v != math.Log(2) {
			t.Fatalf("log_tau[%d] = %g, want log(2)", i, v)
		}
	}
	if s.Optimizer.Options.Trainable != (learning.Trainable{Weights: true, Bias: true, Readout: true}) {
		t.Fatalf("trainable groups = %+v", s.Optimizer.Options.Trainable)
	}
}

func TestPPOExperimentCollector(t *testing.T) {
	ind, err := newPPOIndividual(7, 8, .01)
	if err != nil {
		t.Fatalf("newPPOIndividual: %v", err)
	}
	before := ind.Snapshot()
	corridor := gridnav.Config{Length: 7, TimeLimit: 1, StepPenalty: .01, GoalReward: 1}
	roll, total, err := collectPPOEpisode(context.Background(), ind, corridor, 1000, rand.New(rand.NewPCG(7, 1)))
	if err != nil {
		t.Fatalf("collectPPOEpisode: %v", err)
	}
	if len(roll.Steps) != 1 || !roll.Steps[0].Timeout || roll.Steps[0].Done {
		t.Fatalf("timeout rollout = %+v, want one timeout step", roll.Steps)
	}
	if !finite(roll.Steps[0].BootstrapValue) {
		t.Fatalf("timeout bootstrap = %v, want finite V(next)", roll.Steps[0].BootstrapValue)
	}
	if total != roll.Steps[0].Reward {
		t.Fatalf("episode return = %g, transition reward = %g", total, roll.Steps[0].Reward)
	}
	if roll.PolicyVersion != rl.PolicyVersion(ind) || !reflect.DeepEqual(roll.InitialNeural, before.Neural) {
		t.Fatalf("rollout policy/state does not describe the original zero-state policy")
	}
	if after := ind.Snapshot(); !reflect.DeepEqual(before, after) {
		t.Fatalf("collector mutated the supplied individual")
	}
}

func TestPPORunDefault(t *testing.T) {
	c := DefaultPPOExperimentConfig()
	report, err := RunPPO(context.Background(), c)
	if err != nil {
		t.Fatalf("RunPPO: %v", err)
	}
	if report.SchemaVersion != PPOExperimentSchemaVersion || len(report.Results) != len(c.Seeds) {
		t.Fatalf("report schema/results = %q/%d", report.SchemaVersion, len(report.Results))
	}
	t.Logf("baseline=%g before=%g mean=%g std=%g successful=%d passed=%v", report.Baseline, report.MeanReturnBefore, report.MeanReturn, report.StdReturn, report.SuccessfulSeeds, report.Passed)
	if !report.Passed {
		t.Fatalf("default PPO did not pass: %+v", report)
	}
	for _, result := range report.Results {
		t.Logf("seed=%d before=%g after=%g random=%g updates=%d terminal=%d timeout=%d snapshot=%s", result.Seed, result.ReturnBefore, result.MeanReturn, result.RandomReturn, result.Updates, result.TerminalEpisodes, result.TimeoutEpisodes, result.FinalSnapshotHash)
		if result.Failed || result.Updates != uint64(c.Updates) || len(result.Curve) != c.Updates {
			t.Fatalf("default seed result incomplete: %+v", result)
		}
		if result.TerminalEpisodes+result.TimeoutEpisodes != uint64(c.Updates) || len(result.FinalSnapshotHash) != 64 {
			t.Fatalf("default seed terminal/timeout/hash = %d/%d/%q", result.TerminalEpisodes, result.TimeoutEpisodes, result.FinalSnapshotHash)
		}
		for i, point := range result.Curve {
			if point.Update != i+1 || !finite(point.Return) || !finite(point.Loss) {
				t.Fatalf("seed %d curve[%d] = %+v", result.Seed, i, point)
			}
		}
	}
}

func TestPPORunDefaultReportDeterministic(t *testing.T) {
	c := DefaultPPOExperimentConfig()
	a, err := RunPPO(context.Background(), c)
	if err != nil {
		t.Fatalf("first default RunPPO: %v", err)
	}
	b, err := RunPPO(context.Background(), c)
	if err != nil {
		t.Fatalf("second default RunPPO: %v", err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("same default PPO config is not reproducible:\n%+v\n%+v", a, b)
	}
}

func TestPPORunDeterministic(t *testing.T) {
	c := smallPPOExperimentConfig()
	a, err := RunPPO(context.Background(), c)
	if err != nil {
		t.Fatalf("first RunPPO: %v", err)
	}
	b, err := RunPPO(context.Background(), c)
	if err != nil {
		t.Fatalf("second RunPPO: %v", err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("same PPO config is not reproducible:\n%+v\n%+v", a, b)
	}
}

func TestPPOExperimentJSONRoundTripAndFailedSeed(t *testing.T) {
	report := PPOExperimentReport{
		SchemaVersion: PPOExperimentSchemaVersion,
		Results:       []PPOSeedResult{{Seed: 9, Failed: true, Error: "fixture failure", Curve: []PPOCurvePoint{{Update: 1, Return: .1, Loss: .2}}}},
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	var round PPOExperimentReport
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if !reflect.DeepEqual(report, round) || len(round.Results) != 1 || !round.Results[0].Failed {
		t.Fatalf("failed seed was not retained by JSON round trip: %+v", round)
	}
}

func TestPPORunRetainsActualFailedSeeds(t *testing.T) {
	c := smallPPOExperimentConfig()
	c.Seeds = []uint64{1, 2}
	c.LearningRate = math.MaxFloat64
	report, err := RunPPO(context.Background(), c)
	if err != nil {
		t.Fatalf("RunPPO with finite extreme learning rate: %v", err)
	}
	if len(report.Results) != len(c.Seeds) || report.Passed || report.SuccessfulSeeds != 0 {
		t.Fatalf("failed run aggregate = results:%d passed:%v successful:%d, want two retained failures", len(report.Results), report.Passed, report.SuccessfulSeeds)
	}
	for i, result := range report.Results {
		t.Logf("failed seed=%d error=%q updates=%d", result.Seed, result.Error, result.Updates)
		if result.Seed != c.Seeds[i] || !result.Failed || result.Error == "" {
			t.Fatalf("result[%d] = %+v, want retained failure with reason", i, result)
		}
	}
}

func TestPPOExperimentFinalizeExcludesFailedSeedFromAggregate(t *testing.T) {
	report := PPOExperimentReport{
		Config: PPOExperimentConfig{Seeds: []uint64{1, 2, 3}},
		Results: []PPOSeedResult{
			{Seed: 1, ReturnBefore: 1, MeanReturn: 3, RandomReturn: .5},
			{Seed: 2, Failed: true, Error: "failed", ReturnBefore: 100, MeanReturn: 100, RandomReturn: 100},
			{Seed: 3, ReturnBefore: 1, MeanReturn: 2, RandomReturn: .5},
		},
	}
	finalizePPOReport(&report)
	if report.SuccessfulSeeds != 2 || report.MeanReturnBefore != 1 || report.MeanReturn != 2.5 || report.Baseline != .5 || report.Passed {
		t.Fatalf("mixed aggregate = %+v, want failed seed excluded and gate false", report)
	}
}

func TestPPOExperimentExtremeReturnsHaveFiniteJSON(t *testing.T) {
	c := DefaultPPOExperimentConfig()
	c.Corridor = gridnav.Config{Length: 3, TimeLimit: 1, StepPenalty: 0, GoalReward: 1e160}
	c.Seeds = []uint64{1, 2, 3}
	c.Updates = 1
	c.EvalEpisodes = 1
	c.PPO.TimeLimit = 1
	c.PPO.ValueCoef = 0
	c.PPO.EntropyCoef = 0
	report, err := RunPPO(context.Background(), c)
	if err != nil {
		t.Fatalf("RunPPO with finite extreme reward: %v", err)
	}
	if report.SuccessfulSeeds < 2 || len(report.Results) != len(c.Seeds) || !finite(report.MeanReturn) || !finite(report.MeanReturnBefore) || !finite(report.Baseline) || !finite(report.StdReturn) {
		t.Fatalf("extreme report has non-finite aggregate: %+v", report)
	}
	if _, err := json.Marshal(report); err != nil {
		t.Fatalf("marshal finite extreme report: %v", err)
	}
}

func TestPPOExperimentExtremeStatisticsMatchScaledHandCheck(t *testing.T) {
	report := PPOExperimentReport{
		Config: PPOExperimentConfig{Seeds: []uint64{1, 2}},
		Results: []PPOSeedResult{
			{Seed: 1, ReturnBefore: 0, MeanReturn: math.MaxFloat64, RandomReturn: 0},
			{Seed: 2, ReturnBefore: 0, MeanReturn: -math.MaxFloat64, RandomReturn: 0},
		},
	}
	finalizePPOReport(&report)
	if report.MeanReturn != 0 || report.StdReturn != math.MaxFloat64 || !finite(report.StdReturn) {
		t.Fatalf("extreme statistics = mean:%g std:%g, want 0/%g", report.MeanReturn, report.StdReturn, math.MaxFloat64)
	}

	sameSign := PPOExperimentReport{
		Config: PPOExperimentConfig{Seeds: []uint64{1, 2}},
		Results: []PPOSeedResult{
			{Seed: 1, ReturnBefore: 0, MeanReturn: math.MaxFloat64, RandomReturn: 0},
			{Seed: 2, ReturnBefore: 0, MeanReturn: math.MaxFloat64, RandomReturn: 0},
		},
	}
	finalizePPOReport(&sameSign)
	if sameSign.MeanReturn != math.MaxFloat64 || sameSign.StdReturn != 0 || !finite(sameSign.MeanReturn) || !finite(sameSign.StdReturn) {
		t.Fatalf("same-sign extreme statistics = mean:%g std:%g, want %g/0", sameSign.MeanReturn, sameSign.StdReturn, math.MaxFloat64)
	}
}

func TestPPORunHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report, err := RunPPO(ctx, smallPPOExperimentConfig())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunPPO error = %v, want context.Canceled", err)
	}
	if len(report.Results) != 0 {
		t.Fatalf("canceled run recorded seed results: %+v", report.Results)
	}
}
