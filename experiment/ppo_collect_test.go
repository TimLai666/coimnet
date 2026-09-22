package experiment

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/experiment/gridnav"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/learning/rl"
	"github.com/TimLai666/coimnet/plasticity"
)

func ppoCollectTestConfig(timeLimit int) gridnav.Config {
	return gridnav.Config{Length: 9, TimeLimit: timeLimit, StepPenalty: 0.01, GoalReward: 1}
}

func ppoCollectTestIndividual(t *testing.T) *learning.Individual {
	t.Helper()
	const nodes = 4
	config := learning.Config{
		Dynamics: dynamics.Config{
			Nodes:      nodes,
			Sources:    []int{0},
			Targets:    []int{0},
			DT:         1,
			Activation: "tanh",
		},
		InputSize:        4,
		OutputSize:       gridnav.Actions + 1,
		ReadoutNodes:     []int{0, 1, 2, 3},
		InputNodes:       []int{0, 1, 2, 3},
		ReadoutEveryStep: true,
	}
	logTau := make([]float64, nodes)
	for i := range logTau {
		logTau[i] = math.Log(2)
	}
	encoder := make([]float64, 4*4)
	for i := 0; i < 4; i++ {
		encoder[i*4+i] = 1
	}
	readout := make([]float64, nodes*(gridnav.Actions+1))
	// The cue makes left/right distinct while retaining a nonzero stay chance.
	readout[0*(gridnav.Actions+1)+gridnav.ActionLeft] = -2
	readout[0*(gridnav.Actions+1)+gridnav.ActionRight] = 2
	readout[0*(gridnav.Actions+1)+gridnav.Actions] = 0.5
	// The elapsed input gives the timeout bootstrap a value that differs from
	// the first observation's value.
	readout[3*(gridnav.Actions+1)+gridnav.Actions] = 1
	parameters := learning.Parameters{
		Core: dynamics.Parameters{
			Weights: []float64{0.1},
			Bias:    make([]float64, nodes),
			LogTau:  logTau,
		},
		Encoder: encoder,
		Readout: readout,
	}
	options := learning.DefaultOptions()
	options.Trainable = learning.Trainable{Weights: true, Bias: true, Readout: true}
	ind, err := learning.NewIndividual(config, parameters, options, make([]float64, nodes))
	if err != nil {
		t.Fatalf("NewIndividual: %v", err)
	}
	return ind
}

func ppoCollectTestRNG(seed uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, 17))
}

func TestPPOCollectRecordsReplayAndTimeoutBootstrap(t *testing.T) {
	ctx := context.Background()
	ind := ppoCollectTestIndividual(t)
	before := ind.Snapshot()
	c := ppoCollectTestConfig(1)
	roll, total, err := collectPPOEpisode(ctx, ind, c, 0, ppoCollectTestRNG(1))
	if err != nil {
		t.Fatalf("collectPPOEpisode: %v", err)
	}
	if len(roll.Steps) != 1 {
		t.Fatalf("rollout has %d steps, want one timeout step", len(roll.Steps))
	}
	step := roll.Steps[0]
	if step.Done || !step.Timeout {
		t.Fatalf("terminal flags = done:%v timeout:%v, want timeout only", step.Done, step.Timeout)
	}
	if got, want := total, -c.StepPenalty; got != want {
		t.Fatalf("total reward = %v, want %v", got, want)
	}
	if !isFinitePPOCollect(step.BootstrapValue) {
		t.Fatalf("timeout bootstrap value is non-finite: %v", step.BootstrapValue)
	}
	if step.BootstrapValue == 0 {
		t.Fatal("timeout bootstrap value is zero; expected the next observation's value")
	}
	if step.BootstrapValue == step.Value {
		t.Fatalf("timeout bootstrap %.17g equals the current value %.17g", step.BootstrapValue, step.Value)
	}
	_, targetWithBootstrap, err := rl.Advantages([]rl.Transition{step}, rl.GAEConfig{Gamma: .99, Lambda: .95})
	if err != nil {
		t.Fatalf("Advantages with timeout bootstrap: %v", err)
	}
	zeroBootstrap := step
	zeroBootstrap.BootstrapValue = 0
	_, targetWithZero, err := rl.Advantages([]rl.Transition{zeroBootstrap}, rl.GAEConfig{Gamma: .99, Lambda: .95})
	if err != nil {
		t.Fatalf("Advantages with zero timeout bootstrap: %v", err)
	}
	if targetWithBootstrap[0] == targetWithZero[0] {
		t.Fatalf("GAE target did not change when timeout bootstrap was cleared: %v", targetWithBootstrap[0])
	}
	if roll.PolicyVersion != rl.PolicyVersion(ind) {
		t.Fatalf("rollout policy version %q, want current version %q", roll.PolicyVersion, rl.PolicyVersion(ind))
	}
	if !reflect.DeepEqual(roll.InitialPlastic, before.Plastic) || !reflect.DeepEqual(roll.InitialChemical, before.Chemical) {
		t.Fatal("rollout initial mechanism snapshot does not match the caller")
	}

	snap := ind.Snapshot()
	replay, err := learning.RestoreIndividual(snap)
	if err != nil {
		t.Fatalf("RestoreIndividual: %v", err)
	}
	if err := replay.ResetNeural(ctx, make([]float64, len(snap.Parameters.Core.Bias))); err != nil {
		t.Fatalf("ResetNeural: %v", err)
	}
	out, err := replay.Advance(ctx, [][]float64{append([]float64(nil), step.Obs...)})
	if err != nil {
		t.Fatalf("replay first Advance: %v", err)
	}
	if len(out) != 1 || len(out[0]) != gridnav.Actions+1 {
		t.Fatalf("replay output shape = %#v", out)
	}
	logProb, err := rl.LogProb(out[0][:gridnav.Actions], step.Action)
	if err != nil {
		t.Fatalf("LogProb: %v", err)
	}
	if step.LogProb != logProb {
		t.Fatalf("recorded log probability %.17g, replay %.17g", step.LogProb, logProb)
	}
	if ratio := math.Exp(logProb - step.LogProb); math.Abs(ratio-1) > 1e-12 {
		t.Fatalf("same-policy replay ratio = %.17g, want 1", ratio)
	}
	if step.Value != out[0][gridnav.Actions] {
		t.Fatalf("recorded value %.17g, replay %.17g", step.Value, out[0][gridnav.Actions])
	}

	env, err := gridnav.New(c)
	if err != nil {
		t.Fatalf("gridnav.New: %v", err)
	}
	obs, _ := env.Reset(0)
	next, _, _, _, err := env.Step(step.Action)
	if err != nil {
		t.Fatalf("replay env Step: %v", err)
	}
	if got, want := obs.Vector(), step.Obs; !reflect.DeepEqual(got, want) {
		t.Fatalf("recorded observation = %v, replay observation = %v", want, got)
	}
	bootOut, err := replay.Advance(ctx, [][]float64{next.Vector()})
	if err != nil {
		t.Fatalf("replay bootstrap Advance: %v", err)
	}
	if step.BootstrapValue != bootOut[0][gridnav.Actions] {
		t.Fatalf("recorded bootstrap %.17g, replay %.17g", step.BootstrapValue, bootOut[0][gridnav.Actions])
	}
	if !reflect.DeepEqual(before, ind.Snapshot()) {
		t.Fatal("collector changed the caller's individual")
	}
}

func TestPPOCollectDistinguishesTrueTerminationFromTimeout(t *testing.T) {
	ctx := context.Background()
	ind := ppoCollectTestIndividual(t)
	c := gridnav.Config{Length: 3, TimeLimit: 20, StepPenalty: 0.01, GoalReward: 1}
	var terminal rl.Rollout
	found := false
	for seed := uint64(0); seed < 256; seed++ {
		roll, _, err := collectPPOEpisode(ctx, ind, c, seed, ppoCollectTestRNG(seed))
		if err != nil {
			t.Fatalf("seed %d: collectPPOEpisode: %v", seed, err)
		}
		last := roll.Steps[len(roll.Steps)-1]
		if last.Done {
			terminal = roll
			found = true
			break
		}
		if !last.Timeout {
			t.Fatalf("seed %d ended without done or timeout", seed)
		}
	}
	if !found {
		t.Fatal("fixed RNG seed set produced no true terminal episode")
	}
	last := terminal.Steps[len(terminal.Steps)-1]
	if last.Timeout {
		t.Fatal("true terminal transition also marked timeout")
	}
	if last.BootstrapValue != 0 {
		t.Fatalf("true terminal bootstrap = %v, want zero", last.BootstrapValue)
	}
}

func TestPPOCollectSamplesPolicyAndRNGChangesAction(t *testing.T) {
	ctx := context.Background()
	ind := ppoCollectTestIndividual(t)
	c := ppoCollectTestConfig(1)
	replay, err := learning.RestoreIndividual(ind.Snapshot())
	if err != nil {
		t.Fatalf("RestoreIndividual: %v", err)
	}
	obs, _ := func() (gridnav.Observation, gridnav.Info) {
		env, err := gridnav.New(c)
		if err != nil {
			t.Fatalf("gridnav.New: %v", err)
		}
		return env.Reset(0)
	}()
	if err := replay.ResetNeural(ctx, make([]float64, len(ind.Snapshot().Parameters.Core.Bias))); err != nil {
		t.Fatalf("ResetNeural: %v", err)
	}
	out, err := replay.Advance(ctx, [][]float64{obs.Vector()})
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	argmax := 0
	for i, value := range out[0][:gridnav.Actions] {
		if value > out[0][argmax] {
			argmax = i
		}
	}

	seen := map[int]bool{}
	nonArgmax := false
	for seed := uint64(0); seed < 32; seed++ {
		roll, _, err := collectPPOEpisode(ctx, ind, c, 0, ppoCollectTestRNG(seed))
		if err != nil {
			t.Fatalf("seed %d: collectPPOEpisode: %v", seed, err)
		}
		action := roll.Steps[0].Action
		seen[action] = true
		if action != argmax {
			nonArgmax = true
		}
	}
	if len(seen) < 2 {
		t.Fatalf("actions from different RNG seeds = %v, want sampling variation", seen)
	}
	if !nonArgmax {
		t.Fatalf("all sampled actions were argmax %d; collector appears greedy", argmax)
	}
}

func TestPPOCollectRejectsInvalidInputAndUnsupportedMechanism(t *testing.T) {
	validConfig := ppoCollectTestConfig(1)
	validRNG := func() *rand.Rand { return ppoCollectTestRNG(4) }
	cases := []struct {
		name string
		ctx  context.Context
		ind  *learning.Individual
		c    gridnav.Config
		rng  *rand.Rand
		want string
	}{
		{name: "nil context", ctx: nil, ind: ppoCollectTestIndividual(t), c: validConfig, rng: validRNG(), want: "context"},
		{name: "nil individual", ctx: context.Background(), ind: nil, c: validConfig, rng: validRNG(), want: "individual"},
		{name: "nil rng", ctx: context.Background(), ind: ppoCollectTestIndividual(t), c: validConfig, rng: nil, want: "rng"},
		{name: "nonfinite step penalty", ctx: context.Background(), ind: ppoCollectTestIndividual(t), c: gridnav.Config{Length: 9, TimeLimit: 1, StepPenalty: math.NaN(), GoalReward: 1}, rng: validRNG(), want: "finite"},
		{name: "nonfinite goal reward", ctx: context.Background(), ind: ppoCollectTestIndividual(t), c: gridnav.Config{Length: 9, TimeLimit: 1, StepPenalty: .01, GoalReward: math.Inf(1)}, rng: validRNG(), want: "finite"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := collectPPOEpisode(tc.ctx, tc.ind, tc.c, 0, tc.rng)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := collectPPOEpisode(ctx, ppoCollectTestIndividual(t), validConfig, 0, validRNG()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context error = %v, want context.Canceled", err)
	}

	plastic := ppoCollectTestIndividual(t)
	if err := plastic.EnablePlasticity(plasticity.Config{
		Rule:  plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: .1, WMin: .0001},
		Edges: []int{0},
	}); err != nil {
		t.Fatalf("EnablePlasticity: %v", err)
	}
	if _, _, err := collectPPOEpisode(context.Background(), plastic, validConfig, 0, validRNG()); err == nil || !strings.Contains(strings.ToLower(err.Error()), "plastic") {
		t.Fatalf("unsupported plasticity error = %v, want plasticity refusal", err)
	}
}

func TestPPOCollectResetsOnlyTheEpisodeCopy(t *testing.T) {
	ctx := context.Background()
	ind := ppoCollectTestIndividual(t)
	if _, err := ind.Advance(ctx, [][]float64{{1, 0, 0, 0}}); err != nil {
		t.Fatalf("Advance before collection: %v", err)
	}
	before := ind.Snapshot()
	if before.Neural.Continuous == nil || before.Neural.Continuous.Steps == 0 {
		t.Fatalf("fixture did not create a nonzero caller state: %+v", before.Neural)
	}
	roll, _, err := collectPPOEpisode(ctx, ind, ppoCollectTestConfig(1), 0, ppoCollectTestRNG(5))
	if err != nil {
		t.Fatalf("collectPPOEpisode: %v", err)
	}
	if roll.InitialNeural.Continuous == nil {
		t.Fatalf("rollout initial state is not continuous: %+v", roll.InitialNeural)
	}
	if roll.InitialNeural.Continuous.Steps != 0 {
		t.Fatalf("rollout initial neural steps = %d, want zero", roll.InitialNeural.Continuous.Steps)
	}
	for i, value := range roll.InitialNeural.Continuous.Voltage {
		if value != 0 {
			t.Fatalf("rollout initial voltage[%d] = %v, want zero", i, value)
		}
	}
	if !reflect.DeepEqual(before, ind.Snapshot()) {
		t.Fatal("collector changed the caller's individual while resetting the copy")
	}
}

func isFinitePPOCollect(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
