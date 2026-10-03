package experiment

import (
	"context"
	"math"
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/experiment/gridnav"
)

func TestPPOShortGoalConfig(t *testing.T) {
	want := DefaultPPOExperimentConfig()
	want.Corridor.TimeLimit = 6
	want.PPO.TimeLimit = 6

	got := shortGoalConfig()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("short-goal config = %+v, want defaults with only both time limits set to 6: %+v", got, want)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("short-goal config rejected: %v", err)
	}
}

func TestPPOShortGoalPairs(t *testing.T) {
	got, err := shortGoalPairs()
	if err != nil {
		t.Fatalf("shortGoalPairs: %v", err)
	}
	again, err := shortGoalPairs()
	if err != nil {
		t.Fatalf("second shortGoalPairs: %v", err)
	}
	if !reflect.DeepEqual(got, again) {
		t.Fatalf("shortGoalPairs is not stable:\nfirst=%v\nsecond=%v", got, again)
	}
	if len(got) != 20 {
		t.Fatalf("shortGoalPairs returned %d pairs, want 20", len(got))
	}

	env, err := gridnav.New(DefaultPPOExperimentConfig().Corridor)
	if err != nil {
		t.Fatal(err)
	}
	var left, right []uint64
	for seed := uint64(1000); seed < 1040; seed++ {
		obs, _ := env.Reset(seed)
		if obs.Cue < 0 {
			left = append(left, seed)
		} else {
			right = append(right, seed)
		}
	}
	if len(left) != 20 || len(right) != 20 {
		t.Fatalf("seed sides = left %d, right %d, want 20 each", len(left), len(right))
	}

	seen := make(map[uint64]bool, 40)
	for i, pair := range got {
		if pair[0] != left[i] || pair[1] != right[i] {
			t.Fatalf("pair %d = %v, want left/right ascending zip [%d %d]", i, pair, left[i], right[i])
		}
		for j, seed := range pair {
			if seed < 1000 || seed >= 1040 {
				t.Fatalf("pair %d side %d has out-of-range seed %d", i, j, seed)
			}
			if seen[seed] {
				t.Fatalf("seed %d appears more than once", seed)
			}
			seen[seed] = true
			obs, _ := env.Reset(seed)
			wantCue := -1.0
			if j == 1 {
				wantCue = 1
			}
			if obs.Cue != wantCue {
				t.Fatalf("pair %d side %d seed %d cue=%v, want %v", i, j, seed, obs.Cue, wantCue)
			}
		}
	}
	if len(seen) != 40 {
		t.Fatalf("unique paired seeds = %d, want all 40 seeds", len(seen))
	}
}

func TestPPOShortGoalWitnessAndPairProtocol(t *testing.T) {
	c := shortGoalConfig()
	pairs, err := shortGoalPairs()
	if err != nil {
		t.Fatalf("shortGoalPairs: %v", err)
	}
	ind := goalCueWitness(t)
	ctx := context.Background()
	erasedReached := 0

	for pairIndex, pair := range pairs {
		for side, seed := range pair {
			original, err := ppoGoalEpisodeRun(ctx, ind, c.Corridor, seed, "greedy", "original", nil)
			if err != nil {
				t.Fatalf("pair %d side %d original: %v", pairIndex, side, err)
			}
			if !original.Reached || len(original.Trace) != 3 || math.Abs(original.Return-.97) > 1e-14 {
				t.Fatalf("pair %d side %d original = %+v, want 3-step success with return .97", pairIndex, side, original)
			}
			wantAction := gridnav.ActionLeft
			if original.Cue > 0 {
				wantAction = gridnav.ActionRight
			}
			for step, trace := range original.Trace {
				if trace.Action != wantAction {
					t.Fatalf("pair %d side %d original step %d action=%d, want %d", pairIndex, side, step, trace.Action, wantAction)
				}
			}

			flipped, err := ppoGoalEpisodeRun(ctx, ind, c.Corridor, seed, "greedy", "flipped", nil)
			if err != nil {
				t.Fatalf("pair %d side %d flipped: %v", pairIndex, side, err)
			}
			if flipped.Reached || len(flipped.Trace) != 6 || math.Abs(flipped.Return+.06) > 1e-14 {
				t.Fatalf("pair %d side %d flipped = %+v, want 6-step timeout with return -.06", pairIndex, side, flipped)
			}

			erased, err := ppoGoalEpisodeRun(ctx, ind, c.Corridor, seed, "greedy", "erased", nil)
			if err != nil {
				t.Fatalf("pair %d side %d erased: %v", pairIndex, side, err)
			}
			if erased.Reached {
				erasedReached++
			}
			wantErasedSteps := 6
			if side == 0 {
				wantErasedSteps = 3
			}
			if len(erased.Trace) != wantErasedSteps {
				t.Fatalf("pair %d side %d erased trace length=%d, want %d", pairIndex, side, len(erased.Trace), wantErasedSteps)
			}
		}

		streamSeed := uint64(1000 + pairIndex)
		left, err := ppoGoalEpisodeRun(ctx, ind, c.Corridor, pair[0], "sampled", "erased", rand.New(rand.NewPCG(streamSeed, 0x1005)))
		if err != nil {
			t.Fatalf("pair %d left sampled erased: %v", pairIndex, err)
		}
		right, err := ppoGoalEpisodeRun(ctx, ind, c.Corridor, pair[1], "sampled", "erased", rand.New(rand.NewPCG(streamSeed, 0x1005)))
		if err != nil {
			t.Fatalf("pair %d right sampled erased: %v", pairIndex, err)
		}
		common := len(left.Trace)
		if len(right.Trace) < common {
			common = len(right.Trace)
		}
		if common == 0 {
			t.Fatalf("pair %d has no paired trace prefix", pairIndex)
		}
		for step := 0; step < common; step++ {
			l, r := left.Trace[step], right.Trace[step]
			if l.Action != r.Action {
				t.Fatalf("pair %d step %d paired action differs: %d vs %d", pairIndex, step, l.Action, r.Action)
			}
			if !reflect.DeepEqual(l.Output, r.Output) {
				t.Fatalf("pair %d step %d paired logits differ: %v vs %v", pairIndex, step, l.Output, r.Output)
			}
			if !reflect.DeepEqual(l.Observation, r.Observation) {
				t.Fatalf("pair %d step %d paired observation differs: %v vs %v", pairIndex, step, l.Observation, r.Observation)
			}
		}
	}
	if erasedReached != 20 {
		t.Fatalf("greedy erased reached %d/40, want exactly 20/40", erasedReached)
	}
}

func TestPPOShortGoalSixStepActionEnumeration(t *testing.T) {
	c := shortGoalConfig()
	pairs, err := shortGoalPairs()
	if err != nil {
		t.Fatalf("shortGoalPairs: %v", err)
	}
	if len(pairs) == 0 {
		t.Fatal("shortGoalPairs returned no pair")
	}

	leftSeed, rightSeed := pairs[0][0], pairs[0][1]
	play := func(seed uint64, actions [6]int) bool {
		t.Helper()
		env, err := gridnav.New(c.Corridor)
		if err != nil {
			t.Fatal(err)
		}
		env.Reset(seed)
		for step, action := range actions {
			_, _, done, info, err := env.Step(action)
			if err != nil {
				t.Fatalf("seed %d action %d (%d): %v", seed, step, action, err)
			}
			if done {
				return info.Reached
			}
		}
		t.Fatal("six actions did not finish the short-goal episode")
		return false
	}

	const sequenceCount = 3 * 3 * 3 * 3 * 3 * 3
	checked, leftSuccess, rightSuccess := 0, 0, 0
	for encoded := 0; encoded < sequenceCount; encoded++ {
		value := encoded
		var actions [6]int
		for i := range actions {
			actions[i] = value % gridnav.Actions
			value /= gridnav.Actions
		}
		leftReached := play(leftSeed, actions)
		rightReached := play(rightSeed, actions)
		if leftReached && rightReached {
			t.Fatalf("action sequence %v reaches both paired goals", actions)
		}
		if leftReached {
			leftSuccess++
		}
		if rightReached {
			rightSuccess++
		}
		checked++
	}
	if checked != 729 {
		t.Fatalf("enumerated %d action sequences, want 729", checked)
	}
	if leftSuccess == 0 || rightSuccess == 0 {
		t.Fatalf("enumeration did not exercise both sides: left=%d right=%d", leftSuccess, rightSuccess)
	}
}

func ppoShortGoalTestMetrics(leftReached, rightReached int) ppoGoalMetrics {
	return ppoGoalMetrics{
		Episodes:      40,
		Reached:       leftReached + rightReached,
		LeftEpisodes:  20,
		LeftReached:   leftReached,
		RightEpisodes: 20,
		RightReached:  rightReached,
	}
}

func ppoShortGoalTestFixture() []ppoGoalRun {
	keys := []struct {
		stage string
		mode  string
		cue   string
	}{
		{"before", "sampled", "original"},
		{"before", "sampled", "erased"},
		{"before", "sampled", "flipped"},
		{"before", "greedy", "original"},
		{"before", "greedy", "erased"},
		{"before", "greedy", "flipped"},
		{"after", "sampled", "original"},
		{"after", "sampled", "erased"},
		{"after", "sampled", "flipped"},
		{"after", "greedy", "original"},
		{"after", "greedy", "erased"},
		{"after", "greedy", "flipped"},
		{"random", "random", "original"},
	}
	runs := make([]ppoGoalRun, 0, len(keys))
	for _, key := range keys {
		metrics := ppoShortGoalTestMetrics(20, 20)
		switch key.stage + "/" + key.mode + "/" + key.cue {
		case "before/sampled/original", "random/random/original":
			metrics = ppoShortGoalTestMetrics(5, 5)
		case "after/sampled/original":
			metrics = ppoShortGoalTestMetrics(16, 16)
		case "after/sampled/erased":
			metrics = ppoShortGoalTestMetrics(4, 4)
		case "after/sampled/flipped":
			metrics = ppoShortGoalTestMetrics(0, 0)
		case "after/greedy/original":
			metrics = ppoShortGoalTestMetrics(20, 20)
		case "after/greedy/erased":
			metrics = ppoShortGoalTestMetrics(10, 10)
		case "after/greedy/flipped":
			metrics = ppoShortGoalTestMetrics(0, 0)
		}
		runs = append(runs, ppoGoalRun{Seed: 7, Stage: key.stage, Mode: key.mode, Cue: key.cue, Metrics: metrics})
	}
	return runs
}

func ppoShortGoalTestRunIndex(runs []ppoGoalRun, stage, mode, cue string) int {
	for i, run := range runs {
		if run.Stage == stage && run.Mode == mode && run.Cue == cue {
			return i
		}
	}
	return -1
}

func TestPPOShortGoalGate(t *testing.T) {
	runs := ppoShortGoalTestFixture()
	if !shortGoalGate(runs) {
		t.Fatal("complete short-goal gate witness rejected")
	}
	if shortGoalGate(nil) {
		t.Fatal("empty short-goal gate accepted")
	}

	keys := []struct {
		stage string
		mode  string
		cue   string
	}{
		{"before", "sampled", "original"},
		{"before", "sampled", "erased"},
		{"before", "sampled", "flipped"},
		{"before", "greedy", "original"},
		{"before", "greedy", "erased"},
		{"before", "greedy", "flipped"},
		{"after", "sampled", "original"},
		{"after", "sampled", "erased"},
		{"after", "sampled", "flipped"},
		{"after", "greedy", "original"},
		{"after", "greedy", "erased"},
		{"after", "greedy", "flipped"},
		{"random", "random", "original"},
	}
	for _, key := range keys {
		missing := make([]ppoGoalRun, 0, len(runs)-1)
		for _, run := range runs {
			if run.Stage == key.stage && run.Mode == key.mode && run.Cue == key.cue {
				continue
			}
			missing = append(missing, run)
		}
		if shortGoalGate(missing) {
			t.Fatalf("missing %s/%s/%s passed gate", key.stage, key.mode, key.cue)
		}
	}

	duplicate := append([]ppoGoalRun(nil), runs...)
	duplicateIndex := ppoShortGoalTestRunIndex(duplicate, "before", "greedy", "flipped")
	sourceIndex := ppoShortGoalTestRunIndex(duplicate, "before", "greedy", "erased")
	if duplicateIndex < 0 || sourceIndex < 0 {
		t.Fatal("fixture is missing optional matrix conditions")
	}
	duplicate[duplicateIndex] = duplicate[sourceIndex]
	if shortGoalGate(duplicate) {
		t.Fatal("duplicate matrix condition passed gate")
	}

	mixedSeed := append([]ppoGoalRun(nil), runs...)
	mixedSeed[0].Seed = 99
	if shortGoalGate(mixedSeed) {
		t.Fatal("mixed seeds passed gate")
	}

	cueInsensitive := append([]ppoGoalRun(nil), runs...)
	originalIndex := ppoShortGoalTestRunIndex(cueInsensitive, "after", "sampled", "original")
	flippedIndex := ppoShortGoalTestRunIndex(cueInsensitive, "after", "sampled", "flipped")
	cueInsensitive[flippedIndex].Metrics = cueInsensitive[originalIndex].Metrics
	if shortGoalGate(cueInsensitive) {
		t.Fatal("cue-insensitive trained policy passed gate")
	}

	boundary := append([]ppoGoalRun(nil), runs...)
	erasedIndex := ppoShortGoalTestRunIndex(boundary, "after", "sampled", "erased")
	boundary[erasedIndex].Metrics = ppoShortGoalTestMetrics(12, 12)
	if !shortGoalGate(boundary) {
		t.Fatal("sampled erasure drop of exactly .2 rejected")
	}

	badErasure := append([]ppoGoalRun(nil), runs...)
	badErasure[erasedIndex].Metrics = ppoShortGoalTestMetrics(13, 12)
	if shortGoalGate(badErasure) {
		t.Fatal("sampled erasure drop below .2 passed gate")
	}

	badGreedyErasure := append([]ppoGoalRun(nil), runs...)
	greedyErasedIndex := ppoShortGoalTestRunIndex(badGreedyErasure, "after", "greedy", "erased")
	badGreedyErasure[greedyErasedIndex].Metrics = ppoShortGoalTestMetrics(10, 11)
	if shortGoalGate(badGreedyErasure) {
		t.Fatal("greedy erasure above 20/40 passed gate")
	}

	badGreedyFlip := append([]ppoGoalRun(nil), runs...)
	greedyFlippedIndex := ppoShortGoalTestRunIndex(badGreedyFlip, "after", "greedy", "flipped")
	badGreedyFlip[greedyFlippedIndex].Metrics = ppoShortGoalTestMetrics(1, 0)
	if shortGoalGate(badGreedyFlip) {
		t.Fatal("greedy flipped success above 0/40 passed gate")
	}
}
