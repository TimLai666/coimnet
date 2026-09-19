package experiment

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/plasticity"
)

// ablationTestConfig is the shared MOD-10 fixture the tests run: three seeds
// from the delayed benchmark, a pickable training budget and the three groups
// currently implemented. LearningRate stays zero (the default) so the
// normalization path is exercised.
func ablationTestConfig(episodes, evalEpisodes int) AblationConfig {
	return AblationConfig{
		Seeds:        []uint64{7, 42, 123},
		Episodes:     episodes,
		EvalEpisodes: evalEpisodes,
		Groups:       []string{GroupNoModulation, GroupDirectReward, GroupFixedDecay},
	}
}

func TestAblationValidate(t *testing.T) {
	t.Run("fewer than three seeds", func(t *testing.T) {
		c := ablationTestConfig(6, 8)
		c.Seeds = []uint64{7, 42}
		if err := c.Validate(); err == nil {
			t.Fatal("Validate accepted two seeds")
		}
	})
	t.Run("duplicate seed", func(t *testing.T) {
		c := ablationTestConfig(6, 8)
		c.Seeds = []uint64{7, 7, 123}
		if err := c.Validate(); err == nil {
			t.Fatal("Validate accepted a duplicate seed")
		}
	})
	t.Run("zero episodes", func(t *testing.T) {
		c := ablationTestConfig(0, 8)
		if err := c.Validate(); err == nil {
			t.Fatal("Validate accepted zero episodes")
		}
	})
	t.Run("unknown group", func(t *testing.T) {
		c := ablationTestConfig(6, 8)
		c.Groups = []string{GroupNoModulation, "not_a_group"}
		if err := c.Validate(); err == nil {
			t.Fatal("Validate accepted an unknown group")
		}
	})
	t.Run("missing activity reference", func(t *testing.T) {
		c := ablationTestConfig(6, 8)
		c.Groups = []string{GroupDirectReward, GroupFixedDecay}
		if err := c.Validate(); err == nil {
			t.Fatal("Validate accepted a config without no_modulation")
		}
	})
	t.Run("duplicate group", func(t *testing.T) {
		c := ablationTestConfig(6, 8)
		c.Groups = []string{GroupNoModulation, GroupNoModulation}
		if err := c.Validate(); err == nil {
			t.Fatal("Validate accepted a duplicate group")
		}
	})
	t.Run("trainable controller runs", func(t *testing.T) {
		c := ablationTestConfig(6, 8)
		c.Groups = []string{GroupNoModulation, GroupTrainableController}
		if _, err := RunAblation(context.Background(), c); err != nil {
			t.Fatalf("RunAblation refused trainable_controller: %v", err)
		}
	})
	t.Run("capacity matched runs", func(t *testing.T) {
		c := ablationTestConfig(6, 8)
		c.Groups = []string{GroupNoModulation, GroupCapacityMatched}
		if _, err := RunAblation(context.Background(), c); err != nil {
			t.Fatalf("RunAblation refused capacity_matched: %v", err)
		}
	})
}

func TestAblationSharesDataAcrossGroups(t *testing.T) {
	c := ablationTestConfig(6, 8)
	report, err := RunAblation(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Groups) != 3 {
		t.Fatalf("got %d groups, want 3", len(report.Groups))
	}
	for _, g := range report.Groups {
		if len(g.Runs) != len(c.Seeds) {
			t.Fatalf("group %q has %d runs, want %d", g.Group, len(g.Runs), len(c.Seeds))
		}
		for i, r := range g.Runs {
			if r.Seed != c.Seeds[i] {
				t.Fatalf("group %q run %d seed %d, want %d", g.Group, i, r.Seed, c.Seeds[i])
			}
		}
	}
	if report.TrainSeed != ablationTrainSeed || report.TestSeed != ablationTestSeed {
		t.Fatalf("split seeds %d/%d, want %d/%d", report.TrainSeed, report.TestSeed, ablationTrainSeed, ablationTestSeed)
	}
	all := make([]Episode, 0, c.Episodes+c.EvalEpisodes)
	for e := 0; e < c.Episodes; e++ {
		all = append(all, DelayedEpisode(ablationTrainSeed, uint64(e)))
	}
	for i := 0; i < c.EvalEpisodes; i++ {
		all = append(all, DelayedEpisode(ablationTestSeed, uint64(i)))
	}
	if report.DataHash != hash(all) {
		t.Fatalf("data hash %q, want the hash of the shared episodes %q", report.DataHash, hash(all))
	}
}

// TestAblationNoModulationMatchesDirectTraining replays the no_modulation loop
// by hand: the same individual, six Advance + TrainEpisode pairs and the
// restored-twin evaluation. The group's own score must match that independent
// run exactly, and its activity delta against itself is zero by definition.
func TestAblationNoModulationMatchesDirectTraining(t *testing.T) {
	c := ablationTestConfig(6, 8)
	report, err := RunAblation(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	noMod := report.Groups[0]
	if noMod.Group != GroupNoModulation {
		t.Fatalf("group 0 is %q, want no_modulation", noMod.Group)
	}
	run := noMod.Runs[0]
	if run.Seed != 7 {
		t.Fatalf("run 0 seed %d, want 7", run.Seed)
	}
	if run.Failed {
		t.Fatalf("no_modulation seed 7 failed: %s", run.Error)
	}
	rate := c.LearningRate
	if rate == 0 {
		rate = 0.02
	}
	ind := ablationReferenceIndividual(t, 7, rate)
	for e := 0; e < c.Episodes; e++ {
		ep := DelayedEpisode(ablationTrainSeed, uint64(e))
		if _, err := ind.Advance(context.Background(), ep.Input); err != nil {
			t.Fatalf("reference advance: %v", err)
		}
		if _, err := ind.TrainEpisode(context.Background(), ep.Input, ep.Target); err != nil {
			t.Fatalf("reference training: %v", err)
		}
	}
	want := ablationReferenceScore(t, ind, c)
	if run.Score != want {
		t.Fatalf("no_modulation seed 7 score %v, reference %v", run.Score, want)
	}
	if run.ActivityDelta != 0 {
		t.Fatalf("no_modulation activity delta %v, want 0", run.ActivityDelta)
	}
}

func TestAblationDirectRewardDiffersAfterReward(t *testing.T) {
	ctx := context.Background()
	one := ablationTestConfig(1, 8)
	one.Groups = []string{GroupNoModulation, GroupDirectReward}
	repOne, err := RunAblation(ctx, one)
	if err != nil {
		t.Fatal(err)
	}
	no, dr := repOne.Groups[0], repOne.Groups[1]
	if no.Group != GroupNoModulation || dr.Group != GroupDirectReward {
		t.Fatalf("unexpected group order %q %q", no.Group, dr.Group)
	}
	for i := range no.Runs {
		if no.Runs[i].Score != dr.Runs[i].Score {
			t.Fatalf("episodes=1 seed %d: direct_reward %v != no_modulation %v, r0 must be 0", no.Runs[i].Seed, dr.Runs[i].Score, no.Runs[i].Score)
		}
	}

	six := ablationTestConfig(6, 8)
	six.Groups = []string{GroupNoModulation, GroupDirectReward}
	repSix, err := RunAblation(ctx, six)
	if err != nil {
		t.Fatal(err)
	}
	no6, dr6 := repSix.Groups[0], repSix.Groups[1]
	differs, positiveDelta := false, false
	for i := range no6.Runs {
		if no6.Runs[i].Seed != dr6.Runs[i].Seed {
			t.Fatalf("seed order diverged: %d vs %d", no6.Runs[i].Seed, dr6.Runs[i].Seed)
		}
		if dr6.Runs[i].Failed {
			t.Fatalf("direct_reward seed %d failed: %s", dr6.Runs[i].Seed, dr6.Runs[i].Error)
		}
		if no6.Runs[i].Score != dr6.Runs[i].Score {
			differs = true
		}
		if dr6.Runs[i].ActivityDelta > 0 {
			positiveDelta = true
		}
	}
	if !differs {
		t.Fatal("episodes=6: direct_reward matches no_modulation on every seed; reward had no effect")
	}
	if !positiveDelta {
		t.Fatal("episodes=6: no direct_reward run reports a positive activity delta")
	}
}

func TestAblationFixedDecayRuns(t *testing.T) {
	c := ablationTestConfig(6, 8)
	report, err := RunAblation(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	var fixed *GroupSummary
	for i := range report.Groups {
		if report.Groups[i].Group == GroupFixedDecay {
			fixed = &report.Groups[i]
		}
	}
	if fixed == nil {
		t.Fatal("report has no fixed_decay group")
	}
	for _, r := range fixed.Runs {
		if r.Failed {
			t.Fatalf("fixed_decay seed %d failed: %s", r.Seed, r.Error)
		}
		if math.IsNaN(r.Score) || math.IsInf(r.Score, 0) {
			t.Fatalf("fixed_decay seed %d score %v is not finite", r.Seed, r.Score)
		}
		if r.ActivityDelta < 0 {
			t.Fatalf("fixed_decay seed %d activity delta %v is negative", r.Seed, r.ActivityDelta)
		}
	}
	ref := report.Groups[0].Parameters
	for _, g := range report.Groups {
		if g.Parameters != ref {
			t.Fatalf("group %q has %d parameters, want %d", g.Group, g.Parameters, ref)
		}
	}
}

func TestAblationIsDeterministic(t *testing.T) {
	c := ablationTestConfig(6, 8)
	first, err := RunAblation(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RunAblation(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	a, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatal("two identical runs produced different JSON")
	}
}

// ablationReferenceIndividual rebuilds the three-neuron chain of
// NewDelayedTrainer as a persistent individual with hebbian_rate plasticity on
// its 1->1 self edge, independent of the implementation helper.
func ablationReferenceIndividual(t *testing.T, seed uint64, learningRate float64) *learning.Individual {
	t.Helper()
	config := learning.Config{
		Dynamics: dynamics.Config{
			Nodes: 3, Sources: []int{0, 1, 1}, Targets: []int{1, 1, 2},
			DT: 1, Activation: "tanh",
		},
		InputSize: 1, OutputSize: 1, ReadoutNodes: []int{2},
	}
	params := learning.Parameters{
		Core: dynamics.Parameters{
			Weights: []float64{.15 + float64(mix(seed)%100)/1000, .2, .2},
			Bias:    []float64{0, 0, 0},
			LogTau:  []float64{math.Log(2), math.Log(2), math.Log(2)},
		},
		Encoder: []float64{1, 0, 0},
		Readout: []float64{1},
	}
	o := learning.DefaultOptions()
	o.LearningRate = learningRate
	o.Trainable = learning.Trainable{Weights: true}
	ind, err := learning.NewIndividual(config, params, o, make([]float64, 3))
	if err != nil {
		t.Fatalf("reference individual: %v", err)
	}
	rule := plasticity.Rule{
		Kind:   plasticity.RuleHebbianRate,
		DecayE: .5, DecayP: .5, PlasticMax: 1, WMin: .01,
	}
	if err := ind.EnablePlasticity(plasticity.Config{Rule: rule, Edges: []int{1}}); err != nil {
		t.Fatalf("reference plasticity: %v", err)
	}
	return ind
}

// ablationReferenceScore evaluates a trained individual on the held-out episodes
// the same way the flow does: restore a twin, reset neural state per episode,
// advance with a zero gate and average the negative MSE of the last output row.
func ablationReferenceScore(t *testing.T, ind *learning.Individual, c AblationConfig) float64 {
	t.Helper()
	twin, err := learning.RestoreIndividual(ind.Snapshot())
	if err != nil {
		t.Fatalf("reference restore: %v", err)
	}
	ctx := context.Background()
	var sumSq float64
	for i := 0; i < c.EvalEpisodes; i++ {
		ep := DelayedEpisode(ablationTestSeed, uint64(i))
		if err := twin.ResetNeural(ctx, make([]float64, 3)); err != nil {
			t.Fatalf("reference reset: %v", err)
		}
		out, err := twin.Advance(ctx, ep.Input)
		if err != nil {
			t.Fatalf("reference advance: %v", err)
		}
		d := out[len(out)-1][0] - ep.Target[0]
		sumSq += d * d
	}
	return -sumSq / float64(c.EvalEpisodes)
}
