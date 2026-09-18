package experiment_test

import (
	"math"
	"testing"

	"github.com/TimLai666/coimnet/experiment"
)

func TestContinualEpisodeMatchesDelayedEpisodeAtDelayThree(t *testing.T) {
	spec := experiment.TaskSpec{
		Name:      "A",
		Generator: experiment.GeneratorDelayedCorrelation,
		Params:    map[string]float64{"delay": 3, "channel": 0},
	}
	for i := uint64(0); i < 8; i++ {
		got, err := experiment.ContinualEpisode(spec, 1, false, 1003, i)
		if err != nil {
			t.Fatalf("ContinualEpisode(seed 1003, index %d): %v", i, err)
		}
		want := experiment.DelayedEpisode(1003, i)
		if len(got.Input) != len(want.Input) || len(got.Target) != len(want.Target) {
			t.Fatalf("index %d: got input rows %d target %d, want input rows %d target %d",
				i, len(got.Input), len(got.Target), len(want.Input), len(want.Target))
		}
		for r := range want.Input {
			for c := range want.Input[r] {
				if got.Input[r][c] != want.Input[r][c] {
					t.Fatalf("index %d: Input[%d][%d] = %v, want %v", i, r, c, got.Input[r][c], want.Input[r][c])
				}
			}
		}
		for c := range want.Target {
			if got.Target[c] != want.Target[c] {
				t.Fatalf("index %d: Target[%d] = %v, want %v", i, c, got.Target[c], want.Target[c])
			}
		}
	}
}

func TestContinualEpisodeChannelAndFlip(t *testing.T) {
	base := experiment.TaskSpec{
		Name:      "A",
		Generator: experiment.GeneratorDelayedCorrelation,
		Params:    map[string]float64{"delay": 2, "channel": 1},
	}
	ep, err := experiment.ContinualEpisode(base, 2, false, 1003, 0)
	if err != nil {
		t.Fatalf("ContinualEpisode: %v", err)
	}
	if len(ep.Input) != 4 {
		t.Fatalf("Input has %d rows, want 4 (delay+2)", len(ep.Input))
	}
	for r, row := range ep.Input {
		if len(row) != 2 {
			t.Fatalf("row %d has width %d, want 2", r, len(row))
		}
	}
	if ep.Input[0][0] != 0 || ep.Input[0][1] == 0 {
		t.Fatalf("row 0 = %v, want {0, a} with a != 0", ep.Input[0])
	}
	a := ep.Input[0][1]
	for r := 1; r < len(ep.Input); r++ {
		for c := range ep.Input[r] {
			if ep.Input[r][c] != 0 {
				t.Fatalf("Input[%d][%d] = %v, want 0", r, c, ep.Input[r][c])
			}
		}
	}
	if len(ep.Target) != 1 || math.Abs(ep.Target[0]-0.4*a) > 1e-15 {
		t.Fatalf("Target = %v, want {0.4*a} with a = %v", ep.Target, a)
	}

	flipped, err := experiment.ContinualEpisode(base, 2, true, 1003, 0)
	if err != nil {
		t.Fatalf("ContinualEpisode flipped: %v", err)
	}
	if len(flipped.Target) != 1 || math.Abs(flipped.Target[0]+0.4*a) > 1e-15 {
		t.Fatalf("flipped Target = %v, want {-0.4*a} with a = %v", flipped.Target, a)
	}

	gain := experiment.TaskSpec{
		Name:      "A",
		Generator: experiment.GeneratorDelayedCorrelation,
		Params:    map[string]float64{"delay": 2, "channel": 1, "gain": 0.25},
	}
	g, err := experiment.ContinualEpisode(gain, 2, false, 1003, 0)
	if err != nil {
		t.Fatalf("ContinualEpisode gain: %v", err)
	}
	if g.Input[0][1] != a {
		t.Fatalf("gain must not change the pulse, got %v want %v", g.Input[0][1], a)
	}
	if len(g.Target) != 1 || math.Abs(g.Target[0]-0.25*a) > 1e-15 {
		t.Fatalf("gain Target = %v, want {0.25*a} with a = %v", g.Target, a)
	}

	outOfRange := experiment.TaskSpec{
		Name:      "A",
		Generator: experiment.GeneratorDelayedCorrelation,
		Params:    map[string]float64{"delay": 2, "channel": 2},
	}
	if _, err := experiment.ContinualEpisode(outOfRange, 2, false, 1003, 0); err == nil {
		t.Fatal("expected error for channel 2 with width 2, got nil")
	}
}

func TestForgettingHandTable(t *testing.T) {
	scores := [][]float64{{-1, -2}, {-0.5, -3}, {-0.8, -1}}
	failed := [][]bool{{false, false}, {false, false}, {false, false}}
	F, err := experiment.Forgetting(scores, failed)
	if err != nil {
		t.Fatalf("Forgetting: %v", err)
	}
	want := [][]float64{{0, 0}, {0, 1}, {0.3, 0}}
	for i := range want {
		for j := range want[i] {
			if math.Abs(F[i][j]-want[i][j]) > 1e-12 {
				t.Fatalf("F[%d][%d] = %v, want %v", i, j, F[i][j], want[i][j])
			}
		}
	}

	failed = [][]bool{{false, false}, {false, true}, {false, false}}
	F, err = experiment.Forgetting(scores, failed)
	if err != nil {
		t.Fatalf("Forgetting with a failure: %v", err)
	}
	if F[1][1] != 0 {
		t.Fatalf("F[1][1] = %v, want 0 for a failed cell", F[1][1])
	}
	if !failed[1][1] {
		t.Fatal("failed[1][1] must remain set in the returned mask")
	}
	// Contract is max over non-failed k<=i: candidates -2 (row 0) and -1
	// (row 2) give max -1, so F[2][1] = -1 - (-1) = 0.
	if math.Abs(F[2][1]) > 1e-12 {
		t.Fatalf("F[2][1] = %v, want 0 (max of {-2, -1} minus -1)", F[2][1])
	}

	ragged := [][]float64{{-1, -2}, {-0.5}, {-0.8, -1}}
	if _, err := experiment.Forgetting(ragged, failed); err == nil {
		t.Fatal("expected error for ragged scores, got nil")
	}
	mismatch := [][]bool{{false}, {false}, {false}}
	if _, err := experiment.Forgetting(scores, mismatch); err == nil {
		t.Fatal("expected error for mismatched mask, got nil")
	}
}

func TestAggregateCellsSkipsFailedSeeds(t *testing.T) {
	perSeed := [][][]float64{{{-1}}, {{-3}}, {{-2}}}
	failed := [][][]bool{{{false}}, {{false}}, {{true}}}
	cells, err := experiment.AggregateCells(perSeed, failed)
	if err != nil {
		t.Fatalf("AggregateCells: %v", err)
	}
	if len(cells) != 1 || len(cells[0]) != 1 {
		t.Fatalf("cells shape = %d x %d, want 1 x 1", len(cells), len(cells[0]))
	}
	cs := cells[0][0]
	if cs.Succeeded != 2 || cs.Total != 3 {
		t.Fatalf("Succeeded=%d Total=%d, want 2 and 3", cs.Succeeded, cs.Total)
	}
	if cs.Mean != -2 || cs.Std != 1 || cs.Min != -3 || cs.Max != -1 {
		t.Fatalf("cell = %+v, want Mean -2 Std 1 Min -3 Max -1", cs)
	}

	allFailed := [][][]bool{{{true}}, {{true}}, {{true}}}
	cells, err = experiment.AggregateCells(perSeed, allFailed)
	if err != nil {
		t.Fatalf("AggregateCells all failed: %v", err)
	}
	cs = cells[0][0]
	if cs.Succeeded != 0 || cs.Total != 3 || cs.Mean != 0 || cs.Std != 0 || cs.Min != 0 || cs.Max != 0 {
		t.Fatalf("all-failed cell = %+v, want zeros with Succeeded 0", cs)
	}

	ragged := [][][]float64{{{1}}, {{1, 2}}}
	raggedFail := [][][]bool{{{false}}, {{false, false}}}
	if _, err := experiment.AggregateCells(ragged, raggedFail); err == nil {
		t.Fatal("expected error for mismatched shapes, got nil")
	}
}
