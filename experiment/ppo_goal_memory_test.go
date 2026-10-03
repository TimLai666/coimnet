package experiment

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func goalCueWitness(t *testing.T) *learning.Individual {
	t.Helper()
	ind, err := newPPOIndividual(7, 8, .01)
	if err != nil {
		t.Fatal(err)
	}
	s := ind.Snapshot()
	clear(s.Parameters.Core.Weights)
	clear(s.Parameters.Core.Bias)
	clear(s.Parameters.Readout)
	// Node 0 holds cue/2 on row 0, then decays by half each row.
	// The fixed left/right readouts retain the cue sign for all three moves.
	s.Parameters.Readout[0] = -100
	s.Parameters.Readout[1] = 100
	ind, err = learning.NewIndividual(s.Config, s.Parameters, learning.DefaultOptions(), make([]float64, s.Config.Dynamics.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	return ind
}

func TestPPOGoalCueHandWitness(t *testing.T) {
	ctx := context.Background()
	ind := goalCueWitness(t)
	before := hash(ind.Snapshot())
	c := DefaultPPOExperimentConfig().Corridor
	sides := map[float64]bool{}
	for seed := uint64(1000); seed < 1040; seed++ {
		original, err := ppoGoalEpisodeRun(ctx, ind, c, seed, "greedy", "original", nil)
		if err != nil {
			t.Fatal(err)
		}
		sides[original.Cue] = true
		if !original.Reached || len(original.Trace) != 3 || math.Abs(original.Return-.97) > 1e-14 {
			t.Fatalf("hand witness: %+v", original)
		}
		for _, step := range original.Trace {
			want := 0
			if original.Cue > 0 {
				want = 1
			}
			if step.Action != want {
				t.Fatalf("action=%d cue=%g", step.Action, original.Cue)
			}
		}
		flipped, err := ppoGoalEpisodeRun(ctx, ind, c, seed, "greedy", "flipped", nil)
		if err != nil {
			t.Fatal(err)
		}
		if flipped.Reached || len(flipped.Trace) != c.TimeLimit || math.Abs(flipped.Return+.2) > 1e-14 {
			t.Fatalf("flipped witness: %+v", flipped)
		}
		erased, err := ppoGoalEpisodeRun(ctx, ind, c, seed, "greedy", "erased", nil)
		if err != nil {
			t.Fatal(err)
		}
		if erased.Reached != (original.Cue < 0) {
			t.Fatalf("erased cue unexpectedly reached: %+v", erased)
		}
		if original.Trace[0].Observation[0] != -flipped.Trace[0].Observation[0] || erased.Trace[0].Observation[0] != 0 {
			t.Fatal("cue intervention missing")
		}
		for _, ep := range []ppoGoalEpisode{original, flipped, erased} {
			for i, step := range ep.Trace {
				if i > 0 && step.Observation[0] != 0 {
					t.Fatal("cue leaked after first row")
				}
			}
		}
	}
	if len(sides) != 2 {
		t.Fatal("both goal sides were not checked")
	}
	if hash(ind.Snapshot()) != before {
		t.Fatal("evaluation mutated caller")
	}
}

func TestPPOGoalCueOriginalMatchesCollector(t *testing.T) {
	ctx := context.Background()
	ind, err := newPPOIndividual(3, 8, .01)
	if err != nil {
		t.Fatal(err)
	}
	c := DefaultPPOExperimentConfig().Corridor
	for seed := uint64(1000); seed < 1005; seed++ {
		rng := func() *rand.Rand { return rand.New(rand.NewPCG(seed, 0x1004)) }
		got, err := ppoGoalEpisodeRun(ctx, ind, c, seed, "sampled", "original", rng())
		if err != nil {
			t.Fatal(err)
		}
		want, ret, err := collectPPOEpisode(ctx, ind, c, seed, rng())
		if err != nil {
			t.Fatal(err)
		}
		if got.Return != ret || len(got.Trace) != len(want.Steps) {
			t.Fatal("collector return/length differs")
		}
		for i, step := range got.Trace {
			w := want.Steps[i]
			if !reflect.DeepEqual(step.Observation, w.Obs) || step.Action != w.Action || step.Reward != w.Reward || step.Info.Reached != w.Done || step.Info.TimedOut != w.Timeout {
				t.Fatalf("collector trace differs at row %d", i)
			}
		}
	}
}

func TestPPOGoalCueRejectsAndCancels(t *testing.T) {
	ind := goalCueWitness(t)
	c := DefaultPPOExperimentConfig().Corridor
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ppoGoalEpisodeRun(ctx, ind, c, 1, "greedy", "original", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	for _, tc := range []struct {
		ctx       context.Context
		ind       *learning.Individual
		mode, cue string
	}{
		{nil, ind, "greedy", "original"}, {context.Background(), nil, "greedy", "original"},
		{context.Background(), ind, "other", "original"}, {context.Background(), ind, "greedy", "other"},
		{context.Background(), ind, "sampled", "original"}, {context.Background(), nil, "random", "original"},
	} {
		if _, err := ppoGoalEpisodeRun(tc.ctx, tc.ind, c, 1, tc.mode, tc.cue, nil); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
	c.Length = 0
	if _, err := ppoGoalEpisodeRun(context.Background(), ind, c, 1, "greedy", "original", nil); err == nil {
		t.Fatal("invalid corridor accepted")
	}
}

func TestPPOGoalCueTrainingMatchesRunner(t *testing.T) {
	c := smallPPOExperimentConfig()
	c.Seeds = []uint64{3}
	ind, err := trainPPOGoalFixture(context.Background(), c, 3)
	if err != nil {
		t.Fatal(err)
	}
	want, err := RunPPO(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if hash(ind.Snapshot()) != want.Results[0].FinalSnapshotHash {
		t.Fatal("training changed from RunPPO")
	}
}

func TestPPOGoalCueSummaryAndGate(t *testing.T) {
	ep := []ppoGoalEpisode{
		{Cue: -1, Reached: true, Return: .97, Trace: make([]ppoGoalStep, 3)},
		{Cue: 1, Return: -.2, Trace: make([]ppoGoalStep, 20)},
	}
	got, err := ppoGoalSummarize(ep)
	if err != nil {
		t.Fatal(err)
	}
	if got.Episodes != 2 || got.Reached != 1 || got.LeftReached != 1 || got.RightReached != 0 || got.LeftEpisodes != 1 || got.RightEpisodes != 1 || math.Abs(got.MeanReturn-.385) > 1e-14 || got.MeanSteps != 11.5 {
		t.Fatalf("summary=%+v", got)
	}
	if _, err := ppoGoalSummarize(nil); err == nil {
		t.Fatal("empty summary accepted")
	}
	bad := []ppoGoalEpisode{{Cue: 0, Return: 0}}
	if _, err := ppoGoalSummarize(bad); err == nil {
		t.Fatal("invalid episode accepted")
	}
	perfect := ppoGoalMetrics{Episodes: 40, Reached: 40, LeftEpisodes: 20, LeftReached: 20, RightEpisodes: 20, RightReached: 20}
	half := ppoGoalMetrics{Episodes: 40, Reached: 20, LeftEpisodes: 20, LeftReached: 10, RightEpisodes: 20, RightReached: 10}
	runs := []ppoGoalRun{
		{Stage: "before", Mode: "sampled", Cue: "original", Metrics: half},
		{Stage: "random", Mode: "random", Cue: "original", Metrics: half},
		{Stage: "after", Mode: "sampled", Cue: "original", Metrics: perfect},
		{Stage: "after", Mode: "sampled", Cue: "flipped", Metrics: half},
		{Stage: "after", Mode: "greedy", Cue: "original", Metrics: perfect},
	}
	// The evidence matrix is part of acceptance, even though the descriptive
	// rate comparisons only use five of these thirteen conditions.
	for _, stage := range []string{"before", "after"} {
		for _, mode := range []string{"sampled", "greedy"} {
			for _, cue := range []string{"original", "erased", "flipped"} {
				exists := false
				for _, r := range runs {
					if r.Stage == stage && r.Mode == mode && r.Cue == cue {
						exists = true
					}
				}
				if !exists {
					runs = append(runs, ppoGoalRun{Stage: stage, Mode: mode, Cue: cue, Metrics: half})
				}
			}
		}
	}
	if !ppoGoalGate(runs) {
		t.Fatal("complete goal/cue witness rejected")
	}
	if ppoGoalGate(runs[:5]) {
		t.Fatal("missing cue-control matrix passes gate")
	}
	duplicate := append(append([]ppoGoalRun{}, runs...), runs[0])
	if ppoGoalGate(duplicate) {
		t.Fatal("duplicate condition passes gate")
	}
	mixed := append([]ppoGoalRun{}, runs...)
	mixed[len(mixed)-1].Seed = 99
	if ppoGoalGate(mixed) {
		t.Fatal("mixed seeds pass per-seed gate")
	}

	if ppoGoalGate(nil) || ppoGoalGate(runs[:4]) {
		t.Fatal("missing results pass gate")
	}
	runs[2].Metrics.LeftReached = 0
	runs[2].Metrics.Reached = 20
	if ppoGoalGate(runs) {
		t.Fatal("one-sided strategy passes gate")
	}
	runs[2].Metrics = perfect
	runs[3].Metrics = perfect
	if ppoGoalGate(runs) {
		t.Fatal("cue-insensitive strategy passes gate")
	}
}
