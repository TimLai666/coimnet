package rl_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/experiment/gridnav"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/learning/rl"
)

// The PPO fixture is the corridor of ticket 26 driven by one persistent
// individual whose readout is [logits(3), value]: four input neurons carrying
// gridnav.Observation.Vector unchanged, a fully recurrent hidden block and four
// readout neurons read every step.
const (
	ppoInputs  = 4
	ppoActions = gridnav.Actions
	ppoOutputs = ppoActions + 1
	ppoHidden  = 6
	ppoNodes   = ppoInputs + ppoHidden + ppoOutputs
)

// ppoCorridor is the configuration the gridnav tests use.
func ppoCorridor() gridnav.Config {
	return gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: 0.01, GoalReward: 1}
}

// ppoUpdateConfig is the shared PPO protocol; epochs, burn-in and the entropy
// coefficient are what the individual tests vary.
func ppoUpdateConfig(epochs, burnIn int, entropy float64) rl.PPOConfig {
	return rl.PPOConfig{
		Gamma: .99, Lambda: .95, ClipEpsilon: .2, ValueCoef: .5, EntropyCoef: entropy,
		BurnIn: burnIn, TimeLimit: ppoCorridor().TimeLimit, Epochs: epochs, MiniBatch: 1,
	}
}

// newPPOIndividual builds the fixture policy. Only the core weights, the biases
// and the readout train; the identity encoder and the time constants are fixed,
// so the observation reaches the core untransformed.
func newPPOIndividual(t *testing.T, seed uint64, rate float64) *learning.Individual {
	t.Helper()
	hiddenFirst, readoutFirst := ppoInputs, ppoInputs+ppoHidden
	var sources, targets []int
	edge := func(from, to int) {
		sources, targets = append(sources, from), append(targets, to)
	}
	for i := 0; i < ppoInputs; i++ {
		for h := 0; h < ppoHidden; h++ {
			edge(i, hiddenFirst+h)
		}
	}
	for a := 0; a < ppoHidden; a++ {
		for b := 0; b < ppoHidden; b++ {
			edge(hiddenFirst+a, hiddenFirst+b)
		}
	}
	for h := 0; h < ppoHidden; h++ {
		for o := 0; o < ppoOutputs; o++ {
			edge(hiddenFirst+h, readoutFirst+o)
		}
	}
	inputNodes := make([]int, ppoInputs)
	for i := range inputNodes {
		inputNodes[i] = i
	}
	readoutNodes := make([]int, ppoOutputs)
	for i := range readoutNodes {
		readoutNodes[i] = readoutFirst + i
	}
	config := learning.Config{
		Dynamics:         dynamics.Config{Nodes: ppoNodes, Sources: sources, Targets: targets, DT: 1, Activation: "tanh"},
		InputSize:        ppoInputs,
		OutputSize:       ppoOutputs,
		ReadoutNodes:     readoutNodes,
		InputNodes:       inputNodes,
		ReadoutEveryStep: true,
	}
	logTau := make([]float64, ppoNodes)
	for i := range logTau {
		logTau[i] = math.Log(2)
	}
	encoder := make([]float64, ppoInputs*ppoInputs)
	for i := 0; i < ppoInputs; i++ {
		encoder[i*ppoInputs+i] = 1
	}
	p := learning.Parameters{
		Core:    dynamics.Parameters{Weights: ppoUniform(seed, 0, len(sources)), Bias: make([]float64, ppoNodes), LogTau: logTau},
		Encoder: encoder,
		Readout: ppoUniform(seed, 2, len(readoutNodes)*ppoOutputs),
	}
	o := learning.DefaultOptions()
	o.LearningRate = rate
	o.Trainable = learning.Trainable{Weights: true, Bias: true, Readout: true}
	ind, err := learning.NewIndividual(config, p, o, make([]float64, ppoNodes))
	if err != nil {
		t.Fatalf("NewIndividual: %v", err)
	}
	return ind
}

// ppoUniform draws n values uniformly from [-0.5, 0.5) with PCG(seed, stream).
func ppoUniform(seed, stream uint64, n int) []float64 {
	r := rand.New(rand.NewPCG(seed, stream))
	out := make([]float64, n)
	for i := range out {
		out[i] = r.Float64() - .5
	}
	return out
}

// collectRollout walks one corridor episode under the individual's own greedy
// action and writes down what the update will need: the policy version and the
// initial recurrent and fast/chemical parts before the first observation, then
// one Transition per step. A timed-out episode bootstraps from zero, which is
// this fixture's declared choice, not a value the collector measured.
func collectRollout(ctx context.Context, t *testing.T, ind *learning.Individual, seed uint64) rl.Rollout {
	t.Helper()
	env, err := gridnav.New(ppoCorridor())
	if err != nil {
		t.Fatalf("gridnav.New: %v", err)
	}
	if err := ind.ResetNeural(ctx, make([]float64, ppoNodes)); err != nil {
		t.Fatalf("ResetNeural: %v", err)
	}
	snap := ind.Snapshot()
	roll := rl.Rollout{
		PolicyVersion:   rl.PolicyVersion(ind),
		InitialNeural:   snap.Neural,
		InitialPlastic:  snap.Plastic,
		InitialChemical: snap.Chemical,
	}
	obs, _ := env.Reset(seed)
	for {
		y, err := ind.Advance(ctx, [][]float64{obs.Vector()})
		if err != nil {
			t.Fatalf("Advance: %v", err)
		}
		logits := append([]float64(nil), y[0][:ppoActions]...)
		action := 0
		for i, v := range logits {
			if v > logits[action] {
				action = i
			}
		}
		logProb, err := rl.LogProb(logits, action)
		if err != nil {
			t.Fatalf("LogProb: %v", err)
		}
		step := rl.Transition{Obs: obs.Vector(), Action: action, LogProb: logProb, Value: y[0][ppoActions]}
		next, reward, done, info, err := env.Step(action)
		if err != nil {
			t.Fatalf("env.Step: %v", err)
		}
		step.Reward = reward
		if done {
			step.Done, step.Timeout = info.Reached, !info.Reached
			roll.Steps = append(roll.Steps, step)
			return roll
		}
		roll.Steps = append(roll.Steps, step)
		obs = next
	}
}

// ratiosOn re-runs the rollout's observations from its own initial recurrent
// state under ind's current parameters and returns r_t = exp(logp_t - LogProb_t).
func ratiosOn(ctx context.Context, t *testing.T, ind *learning.Individual, roll rl.Rollout) []float64 {
	t.Helper()
	s := ind.Snapshot()
	s.Neural = roll.InitialNeural
	at, err := learning.RestoreIndividual(s)
	if err != nil {
		t.Fatalf("RestoreIndividual: %v", err)
	}
	input := make([][]float64, len(roll.Steps))
	for i, step := range roll.Steps {
		input[i] = step.Obs
	}
	out, err := at.Advance(ctx, input)
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	ratios := make([]float64, len(roll.Steps))
	for i, step := range roll.Steps {
		logp, err := rl.LogProb(out[i][:ppoActions], step.Action)
		if err != nil {
			t.Fatalf("LogProb: %v", err)
		}
		ratios[i] = math.Exp(logp - step.LogProb)
	}
	return ratios
}

// snapshotJSON is the byte-for-byte comparison the "unchanged individual"
// checks read: every part of the snapshot, not just the parameters.
func snapshotJSON(t *testing.T, ind *learning.Individual) []byte {
	t.Helper()
	data, err := json.Marshal(ind.Snapshot())
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	return data
}

// TestUpdateRefusesStaleRollouts pins the version and initial-state check of
// ticket 26 root decision 7: a rollout collected under a different policy
// version, under a different fast/chemical part, or before a parameter update,
// is refused with ErrStalePolicy and never used, so the individual is unchanged.
func TestUpdateRefusesStaleRollouts(t *testing.T) {
	ctx := context.Background()
	c := ppoUpdateConfig(2, 0, .01)

	t.Run("version differs by one character", func(t *testing.T) {
		ind := newPPOIndividual(t, 3, .02)
		roll := collectRollout(ctx, t, ind, 0)
		version := []byte(roll.PolicyVersion)
		if version[0] == 'a' {
			version[0] = 'b'
		} else {
			version[0] = 'a'
		}
		roll.PolicyVersion = string(version)
		before := snapshotJSON(t, ind)
		_, report, err := rl.Update(ctx, ind, []rl.Rollout{roll}, ppoActions, c)
		if !errors.Is(err, rl.ErrStalePolicy) {
			t.Fatalf("Update error = %v, want ErrStalePolicy", err)
		}
		t.Logf("refused: %v (version_checks=%d)", err, report.VersionChecks)
		if got := snapshotJSON(t, ind); string(got) != string(before) {
			t.Fatalf("individual changed by a refused update")
		}
	})

	t.Run("plastic part present but plasticity is off", func(t *testing.T) {
		ind := newPPOIndividual(t, 3, .02)
		roll := collectRollout(ctx, t, ind, 0)
		if roll.InitialPlastic != nil {
			t.Fatalf("fixture individual reports a plastic part: %+v", roll.InitialPlastic)
		}
		roll.InitialPlastic = &learning.PlasticPart{}
		before := snapshotJSON(t, ind)
		if _, _, err := rl.Update(ctx, ind, []rl.Rollout{roll}, ppoActions, c); !errors.Is(err, rl.ErrStalePolicy) {
			t.Fatalf("Update error = %v, want ErrStalePolicy", err)
		}
		if got := snapshotJSON(t, ind); string(got) != string(before) {
			t.Fatalf("individual changed by a refused update")
		}
	})

	t.Run("parameters moved after collection", func(t *testing.T) {
		ind := newPPOIndividual(t, 3, .02)
		roll := collectRollout(ctx, t, ind, 0)
		input := make([][]float64, len(roll.Steps))
		for i, step := range roll.Steps {
			input[i] = step.Obs
		}
		if _, err := ind.TrainEpisode(ctx, input, make([]float64, ppoOutputs)); err != nil {
			t.Fatalf("TrainEpisode: %v", err)
		}
		if rl.PolicyVersion(ind) == roll.PolicyVersion {
			t.Fatalf("TrainEpisode left the policy version unchanged")
		}
		before := snapshotJSON(t, ind)
		if _, _, err := rl.Update(ctx, ind, []rl.Rollout{roll}, ppoActions, c); !errors.Is(err, rl.ErrStalePolicy) {
			t.Fatalf("Update error = %v, want ErrStalePolicy", err)
		}
		if got := snapshotJSON(t, ind); string(got) != string(before) {
			t.Fatalf("individual changed by a refused update")
		}
	})
}

// TestUpdateRatioIsOneBeforeAnyStep is the first of the three probability-ratio
// checks: with one epoch the forward pass that scores the rollout runs before
// any parameter has moved, so every transition's r is 1 and nothing clips.
func TestUpdateRatioIsOneBeforeAnyStep(t *testing.T) {
	ctx := context.Background()
	ind := newPPOIndividual(t, 5, 1e-12)
	roll := collectRollout(ctx, t, ind, 0)
	ratios := ratiosOn(ctx, t, ind, roll)
	maxDeviation := 0.0
	for i, ratio := range ratios {
		deviation := math.Abs(ratio - 1)
		if deviation > maxDeviation {
			maxDeviation = deviation
		}
		if deviation > 1e-12 {
			t.Fatalf("transition %d ratio = %.17g, want 1 within 1e-12", i, ratio)
		}
	}
	t.Logf("max transition ratio deviation = %.17g across %d transitions", maxDeviation, len(ratios))
	_, report, err := rl.Update(ctx, ind, []rl.Rollout{roll}, ppoActions, ppoUpdateConfig(1, 0, .01))
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	t.Logf("mean_ratio=%.17g clipped_fraction=%v transitions=%d", report.MeanRatio, report.ClippedFraction, report.Transitions)
	if math.Abs(report.MeanRatio-1) > 1e-12 {
		t.Fatalf("mean ratio = %.17g, want 1 within 1e-12", report.MeanRatio)
	}
	if report.ClippedFraction != 0 {
		t.Fatalf("clipped fraction = %v, want 0", report.ClippedFraction)
	}
}

// TestUpdateMovesRatioWithAdvantage is the second ratio check: after real
// updates the transitions the advantage estimate rewarded are more likely than
// they were, so most positive-advantage steps re-score with r > 1.
func TestUpdateMovesRatioWithAdvantage(t *testing.T) {
	ctx := context.Background()
	ind := newPPOIndividual(t, 5, .02)
	c := ppoUpdateConfig(3, 0, 0)
	var rollouts []rl.Rollout
	steps, reached := 0, 0
	for _, seed := range []uint64{0, 1, 2, 3, 4, 5, 6, 7} {
		roll := collectRollout(ctx, t, ind, seed)
		steps += len(roll.Steps)
		if roll.Steps[len(roll.Steps)-1].Done {
			reached++
		}
		rollouts = append(rollouts, roll)
	}
	t.Logf("collected %d rollouts, %d steps, %d ended at the goal and %d timed out", len(rollouts), steps, reached, len(rollouts)-reached)
	before := rl.PolicyVersion(ind)
	updated, report, err := rl.Update(ctx, ind, rollouts, ppoActions, c)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if report.Transitions != steps {
		t.Fatalf("report transitions = %d, want %d", report.Transitions, steps)
	}
	if report.Rollouts != len(rollouts) || report.VersionChecks != len(rollouts) {
		t.Fatalf("report rollouts=%d version_checks=%d, want %d", report.Rollouts, report.VersionChecks, len(rollouts))
	}
	if report.PolicyVersionBefore != before {
		t.Fatalf("report before-version %q, want %q", report.PolicyVersionBefore, before)
	}
	if report.PolicyVersionAfter == report.PolicyVersionBefore {
		t.Fatalf("policy version did not move: %q", report.PolicyVersionAfter)
	}
	var positive, up int
	for _, roll := range rollouts {
		advantages, _, err := rl.Advantages(roll.Steps, rl.GAEConfig{Gamma: c.Gamma, Lambda: c.Lambda})
		if err != nil {
			t.Fatalf("Advantages: %v", err)
		}
		ratios := ratiosOn(ctx, t, updated, roll)
		for i, a := range advantages {
			if a <= 0 {
				continue
			}
			positive++
			if ratios[i] > 1 {
				up++
			}
		}
	}
	t.Logf("positive-advantage transitions: %d, of which r>1 after the update: %d (mean_ratio last epoch %.6f)", positive, up, report.MeanRatio)
	if positive == 0 {
		t.Fatalf("no positive-advantage transition in %d steps", steps)
	}
	if up*2 <= positive {
		t.Fatalf("only %d of %d positive-advantage transitions have r > 1", up, positive)
	}
}

// TestUpdateRejectsFullBurnIn protects the StepFrom contract: a burn-in that
// covers the whole rollout has no scored rows and must be refused before an
// optimizer step can apply weight decay without a gradient.
func TestUpdateRejectsFullBurnIn(t *testing.T) {
	ctx := context.Background()
	ind := newPPOIndividual(t, 5, .05)
	roll := collectRollout(ctx, t, ind, 0)
	before := rl.PolicyVersion(ind)
	beforeSnapshot := snapshotJSON(t, ind)
	_, _, err := rl.Update(ctx, ind, []rl.Rollout{roll}, ppoActions, ppoUpdateConfig(2, len(roll.Steps), .01))
	if err == nil {
		t.Fatal("Update accepted a full burn-in rollout")
	}
	t.Logf("burn_in=%d transitions=%d version=%s error=%v", len(roll.Steps), len(roll.Steps), before[:12], err)
	if rl.PolicyVersion(ind) != before {
		t.Fatalf("policy version moved after full-burn-in refusal")
	}
	if got := snapshotJSON(t, ind); string(got) != string(beforeSnapshot) {
		t.Fatal("individual changed after full-burn-in refusal")
	}
}

// TestUpdateIsDeterministic pins the reproducibility requirement: the same seed
// and the same rollouts give the same report and the same individual, bit for
// bit.
func TestUpdateIsDeterministic(t *testing.T) {
	ctx := context.Background()
	c := ppoUpdateConfig(3, 1, .01)
	run := func() (rl.PPOReport, []byte) {
		ind := newPPOIndividual(t, 11, .02)
		rollouts := []rl.Rollout{collectRollout(ctx, t, ind, 0), collectRollout(ctx, t, ind, 1)}
		updated, report, err := rl.Update(ctx, ind, rollouts, ppoActions, c)
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		return report, snapshotJSON(t, updated)
	}
	firstReport, firstSnapshot := run()
	secondReport, secondSnapshot := run()
	t.Logf("report: %+v", firstReport)
	if firstReport != secondReport {
		t.Fatalf("reports differ:\n%+v\n%+v", firstReport, secondReport)
	}
	if string(firstSnapshot) != string(secondSnapshot) {
		t.Fatalf("individual snapshots differ between two identical runs")
	}
}
