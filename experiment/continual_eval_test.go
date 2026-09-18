package experiment

import (
	"context"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
)

// The four mission-critical seed streams of seed 7, pinned after they were
// printed: training stages 0 and 1 and evaluation tasks 0 and 1 are pairwise
// distinct, so a run's training episodes and an evaluation never share a
// counter stream.
const (
	continualTestTrainSeed0 uint64 = 9648886400068060533
	continualTestTrainSeed1 uint64 = 13633754720362554755
	continualTestEvalSeed0  uint64 = 1043032536603952313
	continualTestEvalSeed1  uint64 = 12115091614907694051
)

func continualEvalFixture(t *testing.T, seed uint64) *learning.Individual {
	t.Helper()
	ind, err := ContinualFixture(seed)
	if err != nil {
		t.Fatalf("ContinualFixture(%d) error = %v", seed, err)
	}
	return ind
}

// continualEvalTaskA is the two-channel task the fixture tests train and score:
// a pulse on channel 0, target at gain 0.4.
func continualEvalTaskA() TaskSpec {
	return TaskSpec{Name: "A", Generator: GeneratorDelayedCorrelation, Params: map[string]float64{"delay": 2, "channel": 0}}
}

// continualEvalTrain runs n training episodes of task A on the individual.
func continualEvalTrain(t *testing.T, ctx context.Context, ind *learning.Individual, n int) {
	t.Helper()
	for e := 0; e < n; e++ {
		ep, err := ContinualEpisode(continualEvalTaskA(), 2, false, trainSeed(7, 0), uint64(e))
		if err != nil {
			t.Fatalf("training episode %d: %v", e, err)
		}
		if _, err := ind.TrainEpisode(ctx, ep.Input, ep.Target); err != nil {
			t.Fatalf("training episode %d: %v", e, err)
		}
	}
}

// continualEvalChemistry is the 3-node chemical declaration of the continued
// fixture, in the style of the learning package's chemistry fixture: one region,
// one channel and an external timeline that pulses once on the second row.
func continualEvalChemistry() modulation.ChemistryConfig {
	return modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{2}},
		Sources: []modulation.SourceSpec{{
			Kind: modulation.SourceExternalTimeline, Channel: 0,
			Timeline: &modulation.ExternalTimeline{ChannelCount: 1, Entries: []modulation.TimelineEntry{{Step: 1, Channel: 0, Rate: 1}}},
		}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{
			{Cells: []int{0}, Signal: "octopamine", Channel: 0, Status: modulation.StatusHypothesized, Kd: 0.5, N: 1,
				Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "continual-chem-fixture/v1"},
		}},
		Regions: modulation.RegionAssignment{NodeRegion: []int{0, 0, 0}},
		Effects: []modulation.Effect{{Kind: modulation.EffectSensitivity, Receptor: 0, GammaScale: 1}},
	}
}

func enableContinualChemistry(t *testing.T, ind *learning.Individual) {
	t.Helper()
	if err := ind.EnableChemistry(continualEvalChemistry()); err != nil {
		t.Fatalf("EnableChemistry() error = %v", err)
	}
}

func TestContinualFixtureShape(t *testing.T) {
	a := continualEvalFixture(t, 7)
	cfg := a.Snapshot().Config
	if cfg.InputSize != 2 {
		t.Fatalf("InputSize = %d, want 2", cfg.InputSize)
	}
	if cfg.OutputSize != 1 {
		t.Fatalf("OutputSize = %d, want 1", cfg.OutputSize)
	}
	if !reflect.DeepEqual(cfg.ReadoutNodes, []int{2}) {
		t.Fatalf("ReadoutNodes = %v, want [2]", cfg.ReadoutNodes)
	}
	if cfg.Dynamics.Nodes != 3 {
		t.Fatalf("nodes = %d, want 3", cfg.Dynamics.Nodes)
	}

	w7 := a.Snapshot().Parameters.Core.Weights[0]
	w42 := continualEvalFixture(t, 42).Snapshot().Parameters.Core.Weights[0]
	if w7 == w42 {
		t.Fatal("seeds 7 and 42 produced the same edge 0 weight")
	}
	for _, pair := range []struct {
		seed uint64
		got  float64
	}{{7, w7}, {42, w42}} {
		want := .15 + float64(mix(pair.seed)%100)/1000
		if pair.got != want {
			t.Fatalf("seed %d: edge 0 weight = %v, want %v", pair.seed, pair.got, want)
		}
	}

	out, err := a.Advance(context.Background(), [][]float64{{0, 1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("Advance returned %d output rows, want 1", len(out))
	}
	history := a.Snapshot().Neural.Continuous.History
	if len(history) == 0 {
		t.Fatal("the advanced individual has no output history")
	}
	if history[len(history)-1][0] == 0 {
		t.Fatalf("a channel 1 pulse did not drive node 0: last history row %v", history[len(history)-1])
	}
}

func TestEvaluateTaskLeavesTheIndividualUntouched(t *testing.T) {
	ctx := context.Background()
	ind := continualEvalFixture(t, 7)
	continualEvalTrain(t, ctx, ind, 3)
	before := ind.Snapshot()
	score, err := evaluateTask(ctx, ind, continualEvalTaskA(), 2, false, 8, evalSeed(7, 0), false)
	if err != nil {
		t.Fatal(err)
	}
	after := ind.Snapshot()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("evaluateTask changed the individual, including its neural state")
	}
	if !finite(score) || score > 0 {
		t.Fatalf("score = %v, want finite and <= 0", score)
	}
	t.Logf("score after 3 training episodes: %g", score)
}

// TestEvaluateTaskMatchesDirectComputation builds the score by hand from the
// definition and requires the implemented path to be bit-identical, and the
// flipped rule change to actually change the score.
func TestEvaluateTaskMatchesDirectComputation(t *testing.T) {
	ctx := context.Background()
	ind := continualEvalFixture(t, 7)
	continualEvalTrain(t, ctx, ind, 3)
	const width, episodes = 2, 8
	seed := evalSeed(7, 0)

	direct := func(flipped bool) float64 {
		reference := ind.Snapshot()
		twin, err := learning.RestoreIndividual(reference)
		if err != nil {
			t.Fatal(err)
		}
		var score float64
		for e := 0; e < episodes; e++ {
			ep, err := ContinualEpisode(continualEvalTaskA(), width, flipped, seed, uint64(e))
			if err != nil {
				t.Fatalf("episode %d: %v", e, err)
			}
			if err := twin.ResetNeural(ctx, make([]float64, len(reference.Parameters.Core.Bias))); err != nil {
				t.Fatalf("episode %d: %v", e, err)
			}
			out, err := twin.Advance(ctx, ep.Input)
			if err != nil {
				t.Fatalf("episode %d: %v", e, err)
			}
			mse, err := itemMSE(out, EvaluationItem{Target: ep.Target})
			if err != nil {
				t.Fatalf("episode %d: %v", e, err)
			}
			score += -mse / float64(episodes)
		}
		return score
	}

	viaEval, err := evaluateTask(ctx, ind, continualEvalTaskA(), width, false, episodes, seed, false)
	if err != nil {
		t.Fatal(err)
	}
	directScore := direct(false)
	if directScore != viaEval {
		t.Fatalf("direct computation %v differs from evaluateTask %v", directScore, viaEval)
	}

	viaFlipped, err := evaluateTask(ctx, ind, continualEvalTaskA(), width, true, episodes, seed, false)
	if err != nil {
		t.Fatal(err)
	}
	directFlipped := direct(true)
	if directFlipped != viaFlipped {
		t.Fatalf("direct flipped %v differs from evaluateTask flipped %v", directFlipped, viaFlipped)
	}
	if viaEval == viaFlipped {
		t.Fatalf("flipping the target changed nothing: %v both ways", viaEval)
	}
	t.Logf("plain score %g, flipped score %g", viaEval, viaFlipped)
}

func TestEvaluateTaskFixedChemistryIsBitIdentical(t *testing.T) {
	ctx := context.Background()
	ind := continualEvalFixture(t, 7)
	enableContinualChemistry(t, ind)
	if _, err := ind.Advance(ctx, [][]float64{{1, 0}, {1, 0}, {1, 0}}); err != nil {
		t.Fatal(err)
	}
	const width, episodes = 2, 8
	seed := evalSeed(7, 0)

	frozen1, err := evaluateTask(ctx, ind, continualEvalTaskA(), width, false, episodes, seed, true)
	if err != nil {
		t.Fatal(err)
	}
	frozen2, err := evaluateTask(ctx, ind, continualEvalTaskA(), width, false, episodes, seed, true)
	if err != nil {
		t.Fatal(err)
	}
	live1, err := evaluateTask(ctx, ind, continualEvalTaskA(), width, false, episodes, seed, false)
	if err != nil {
		t.Fatal(err)
	}
	live2, err := evaluateTask(ctx, ind, continualEvalTaskA(), width, false, episodes, seed, false)
	if err != nil {
		t.Fatal(err)
	}
	if frozen1 != frozen2 {
		t.Fatalf("two frozen evaluations diverged: %v against %v", frozen1, frozen2)
	}
	if live1 != live2 {
		t.Fatalf("two live evaluations diverged: %v against %v", live1, live2)
	}
	if frozen1 == live1 {
		t.Fatalf("freezing the chemistry did not change the score: %v", frozen1)
	}
	t.Logf("frozen score %g, live score %g", frozen1, live1)
}

func TestSeedsDiffer(t *testing.T) {
	got := []uint64{trainSeed(7, 0), trainSeed(7, 1), evalSeed(7, 0), evalSeed(7, 1)}
	t.Logf("trainSeed(7,0)=%d trainSeed(7,1)=%d evalSeed(7,0)=%d evalSeed(7,1)=%d", got[0], got[1], got[2], got[3])
	want := []uint64{continualTestTrainSeed0, continualTestTrainSeed1, continualTestEvalSeed0, continualTestEvalSeed1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("seed streams = %v, want %v", got, want)
	}
	for i := 1; i < len(got); i++ {
		for j := 0; j < i; j++ {
			if got[i] == got[j] {
				t.Fatalf("seed %d reuses the counter stream of seed %d: %d", i, j, got[i])
			}
		}
	}
}
