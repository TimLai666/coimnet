package gridnav_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/experiment/gridnav"
)

// TestResetSeedDecidesGoalSide pins the seed-to-goal rule of ticket 26 root
// decision 5: the goal side comes from the first PCG(seed, 0) draw, even ->
// right, odd -> left. The two first draws are
// seed 0: 4107282207882862730 (even)  -> Cue +1
// seed 1: 11035966612244867331 (odd)  -> Cue -1
// printed by a scratch run over math/rand/v2 before these constants were
// written, matching the pinned PCG convention the replay package uses.
func TestResetSeedDecidesGoalSide(t *testing.T) {
	c := gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: 0.01, GoalReward: 1}
	env, err := gridnav.New(c)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	obs, _ := env.Reset(0)
	t.Logf("seed 0: cue=%v (first draw even, want 1)", obs.Cue)
	if obs.Cue != 1 {
		t.Fatalf("seed 0 cue = %v, want 1 (right)", obs.Cue)
	}
	obs, _ = env.Reset(1)
	t.Logf("seed 1: cue=%v (first draw odd, want -1)", obs.Cue)
	if obs.Cue != -1 {
		t.Fatalf("seed 1 cue = %v, want -1 (left)", obs.Cue)
	}
	first, infoA := env.Reset(0)
	second, infoB := env.Reset(0)
	if first.Cue != second.Cue || first.AtLeft != second.AtLeft || first.AtRight != second.AtRight || first.Elapsed != second.Elapsed {
		t.Fatalf("same seed reset differs: %+v vs %+v", first, second)
	}
	if infoA != infoB {
		t.Fatalf("same seed reset info differs: %+v vs %+v", infoA, infoB)
	}
}

// TestExpertReachesGoalWithinLength drives the expert from the centre of a
// length-7 corridor: each goal side is 3 cells away, so the episode ends in
// exactly 3 steps and the reward total is GoalReward - 3*StepPenalty
// = 1 - 3*0.01 = 0.97 by hand.
func TestExpertReachesGoalWithinLength(t *testing.T) {
	env, err := gridnav.New(gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: 0.01, GoalReward: 1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	env.Reset(0)
	var total float64
	steps := 0
	for i := 0; i < 20; i++ {
		_, rew, done, info, err := env.Step(env.Expert())
		if err != nil {
			t.Fatalf("step %d: %v", i+1, err)
		}
		total += rew
		steps++
		if i < 2 {
			if done {
				t.Fatalf("done at step %d, want not before step 3", i+1)
			}
			continue
		}
		if !done {
			continue
		}
		if i != 2 {
			t.Fatalf("done at step %d, want exactly 3", i+1)
		}
		if !info.Reached {
			t.Fatalf("done without reaching goal: %+v", info)
		}
		break
	}
	if steps != 3 {
		t.Fatalf("expert took %d steps, want exactly 3", steps)
	}
	if total != 0.97 {
		t.Fatalf("reward total = %v, want 0.97 (1 - 3*0.01)", total)
	}
}

// TestCueOnlyOnFirstStep checks the partial observability of root decision 5:
// the cue appears only at t = 0 and disappears afterwards, so the agent must
// remember it internally.
func TestCueOnlyOnFirstStep(t *testing.T) {
	env, err := gridnav.New(gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: 0.01, GoalReward: 1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	obs, _ := env.Reset(0)
	if obs.Cue == 0 {
		t.Fatalf("first observation cue = 0, want nonzero")
	}
	for i := 0; i < 3; i++ {
		obs, _, _, _, err = env.Step(gridnav.ActionStay)
		if err != nil {
			t.Fatalf("step %d: %v", i+1, err)
		}
		if obs.Cue != 0 {
			t.Fatalf("cue at step %d = %v, want 0", i+1, obs.Cue)
		}
	}
}

// TestWallKeepsPosition walks past the left wall with the goal on the right
// (seed 0), so no step ever reaches it: from the centre of length 7, 3 steps
// reach cell 0 and the remaining Length/2+2-3 = 2 steps stay in place, so the
// position is 0 and AtLeft is 1 at the end.
func TestWallKeepsPosition(t *testing.T) {
	env, err := gridnav.New(gridnav.Config{Length: 7, TimeLimit: 10, StepPenalty: 0.01, GoalReward: 1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	env.Reset(0)
	for i := 0; i < 7/2+2; i++ {
		obs, _, done, info, err := env.Step(gridnav.ActionLeft)
		if err != nil {
			t.Fatalf("step %d: %v", i+1, err)
		}
		if done {
			t.Fatalf("done at step %d, want to keep walking into the wall", i+1)
		}
		if i < 3 {
			continue
		}
		if info.Position != 0 || obs.AtLeft != 1 {
			t.Fatalf("after step %d: position=%d AtLeft=%v, want 0 and 1", i+1, info.Position, obs.AtLeft)
		}
	}
}

// TestTimeLimitEndsEpisode pins the time-limit branch: with TimeLimit 4, the
// fourth stay step ends the episode (TimedOut, not Reached) and any further
// Step is an error.
func TestTimeLimitEndsEpisode(t *testing.T) {
	env, err := gridnav.New(gridnav.Config{Length: 7, TimeLimit: 4, StepPenalty: 0.01, GoalReward: 1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	env.Reset(0)
	var info gridnav.Info
	var done bool
	for i := 0; i < 4; i++ {
		_, _, done, info, err = env.Step(gridnav.ActionStay)
		if err != nil {
			t.Fatalf("step %d: %v", i+1, err)
		}
		if i < 3 && done {
			t.Fatalf("done at step %d, want only at step 4", i+1)
		}
	}
	if !info.TimedOut {
		t.Fatalf("timed-out episode ended without TimedOut: %+v", info)
	}
	if info.Reached {
		t.Fatalf("stay episode reached the goal: %+v", info)
	}
	if _, _, _, _, err = env.Step(gridnav.ActionStay); err == nil {
		t.Fatalf("step after done succeeded, want error")
	}
}

// TestObservationHidesEvaluatorState checks root decision 5 and main spec
// 13.6: the observation carries the cue and the wall indicators but never the
// position or the goal coordinates.
func TestObservationHidesEvaluatorState(t *testing.T) {
	if n := reflect.TypeOf(gridnav.Observation{}).NumField(); n != 4 {
		t.Fatalf("Observation has %d fields, want 4", n)
	}
	want := []string{"Cue", "AtLeft", "AtRight", "Elapsed"}
	for i, name := range want {
		if got := reflect.TypeOf(gridnav.Observation{}).Field(i).Name; got != name {
			t.Fatalf("field %d = %s, want %s", i, got, name)
		}
	}
	for _, forbidden := range []string{"Position", "Goal"} {
		if _, ok := reflect.TypeOf(gridnav.Observation{}).FieldByName(forbidden); ok {
			t.Fatalf("Observation leaks %s", forbidden)
		}
	}
}

// TestConfigValidation rejects each invalid configuration with an error.
func TestConfigValidation(t *testing.T) {
	valid := gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: 0.01, GoalReward: 1}
	cases := []struct {
		name string
		c    gridnav.Config
	}{
		{"even length", gridnav.Config{Length: 4, TimeLimit: 20, StepPenalty: 0.01, GoalReward: 1}},
		{"length < 3", gridnav.Config{Length: 1, TimeLimit: 20, StepPenalty: 0.01, GoalReward: 1}},
		{"zero time limit", gridnav.Config{Length: 7, TimeLimit: 0, StepPenalty: 0.01, GoalReward: 1}},
		{"negative step penalty", gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: -0.01, GoalReward: 1}},
		{"non-positive goal reward", gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: 0.01, GoalReward: 0}},
		{"NaN step penalty", gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: math.NaN(), GoalReward: 1}},
		{"+Inf step penalty", gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: math.Inf(1), GoalReward: 1}},
		{"-Inf step penalty", gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: math.Inf(-1), GoalReward: 1}},
		{"NaN goal reward", gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: 0.01, GoalReward: math.NaN()}},
		{"+Inf goal reward", gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: 0.01, GoalReward: math.Inf(1)}},
		{"-Inf goal reward", gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: 0.01, GoalReward: math.Inf(-1)}},
	}
	for _, tc := range cases {
		if _, err := gridnav.New(tc.c); err == nil {
			t.Errorf("%s: New succeeded, want error", tc.name)
		}
	}
	if _, err := gridnav.New(valid); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	zeroPenalty := gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: 0, GoalReward: 1}
	if _, err := gridnav.New(zeroPenalty); err != nil {
		t.Fatalf("zero step penalty rejected: %v", err)
	}
}
