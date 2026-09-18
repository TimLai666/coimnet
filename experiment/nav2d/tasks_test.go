package nav2d

import "testing"

func TestRememberGoalCueOnlyAtStepZero(t *testing.T) {
	for seed := uint64(0); seed < 20; seed++ {
		env, err := New(Config{Task: TaskRememberGoal})
		if err != nil {
			t.Fatalf("seed %d: New: %v", seed, err)
		}
		obs, info := env.Reset(seed)
		wantX := signum(float64(info.GoalX - info.X))
		wantY := signum(float64(info.GoalY - info.Y))
		if obs.Cue[0] != wantX || obs.Cue[1] != wantY {
			t.Fatalf("seed %d: Reset Cue = %v, want (%g, %g)", seed, obs.Cue, wantX, wantY)
		}
		obs1, _, _, _, err := env.Step(ActionStay)
		if err != nil {
			t.Fatalf("seed %d: step: %v", seed, err)
		}
		if obs1.Cue[0] != 0 || obs1.Cue[1] != 0 {
			t.Fatalf("seed %d: step-1 Cue = %v, want (0,0)", seed, obs1.Cue)
		}
		for !env.done {
			if _, _, _, _, err := env.Step(env.Expert()); err != nil {
				t.Fatalf("seed %d: expert step: %v", seed, err)
			}
			assertNoGoalInView(t, env.observation().View, seed)
		}
	}
}

// assertNoGoalInView fails when any one-hot triple in view is CellGoal ((0,0,1)).
func assertNoGoalInView(t *testing.T, view []float64, seed uint64) {
	t.Helper()
	for i := 0; i+2 < len(view); i += 3 {
		if view[i] == 0 && view[i+1] == 0 && view[i+2] == 1 {
			t.Fatalf("seed %d: CellGoal one-hot at view index %d", seed, i/3)
		}
	}
}

func TestAvoidObstaclesShowsGoalWhenInCone(t *testing.T) {
	cells := [][]int{
		{CellFree, CellFree, CellFree, CellFree, CellFree},
		{CellFree, CellFree, CellFree, CellFree, CellFree},
		{CellFree, CellFree, CellFree, CellGoal, CellFree},
		{CellFree, CellFree, CellFree, CellFree, CellFree},
		{CellFree, CellFree, CellFree, CellFree, CellFree},
	}
	env, err := newFromMap(cells, 1, 2, 0, 3, 2, Config{})
	if err != nil {
		t.Fatalf("newFromMap: %v", err)
	}
	env.task = TaskAvoidObstacles
	assertView(t, env.observation().View, 2, 0, CellGoal, "avoid_obstacles goal in cone")
}

func TestAdaptAfterChangeRegeneratesAndFlags(t *testing.T) {
	// Seed 2: the redraw leaves the agent within the remaining half-timeslot
	// budget (seeds like 42 leave only 5 steps while the path still needs a
	// turn, and would end in a timeout rather than honest exhaustion).
	const seed = 2
	env, err := New(Config{TimeLimit: 10, Task: TaskAdaptAfterChange})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, info := env.Reset(seed)
	cellsBefore := env.Cells()
	obs := tipToStep5(t, env, seed)
	if obs.RuleChanged != 1 {
		t.Fatalf("step 5 RuleChanged = %v, want 1", obs.RuleChanged)
	}
	cellsAfter := env.Cells()
	diff := 0
	for y := range cellsBefore {
		for x := range cellsBefore[y] {
			if cellsBefore[y][x] != cellsAfter[y][x] {
				diff++
			}
		}
	}
	if diff == 0 {
		t.Fatalf("cells unchanged after adapt")
	}
	if cellsAfter[info.Y][info.X] != CellFree {
		t.Fatalf("agent pos (%d,%d) kind %d, want CellFree", info.X, info.Y, cellsAfter[info.Y][info.X])
	}
	if cellsAfter[info.GoalY][info.GoalX] != CellGoal {
		t.Fatalf("goal (%d,%d) kind %d, want CellGoal", info.GoalX, info.GoalY, cellsAfter[info.GoalY][info.GoalX])
	}
	if info.ShortestPath < 1 {
		t.Fatalf("ShortestPath %d < 1", info.ShortestPath)
	}
	if obs4 := tipToStep4(t, seed); obs4.RuleChanged != 0 {
		t.Fatalf("step 4 RuleChanged = %v, want 0", obs4.RuleChanged)
	}
	if obs6 := tipToStep6(t, seed); obs6.RuleChanged != 0 {
		t.Fatalf("step 6 RuleChanged = %v, want 0", obs6.RuleChanged)
	}
	// Continue with Expert after the change; goal must be reached.
	reached := false
	for !env.done {
		_, _, done, infoS, err := env.Step(env.Expert())
		if err != nil {
			t.Fatalf("expert step: %v", err)
		}
		if done {
			reached = infoS.Reached
			break
		}
	}
	if !reached {
		t.Fatalf("expert did not reach the goal after the map change")
	}
}

func tipToStep5(t *testing.T, env *Env, seed uint64) Observation {
	t.Helper()
	var obs Observation
	for i := 0; i < 5; i++ {
		var err error
		if obs, _, _, _, err = env.Step(ActionStay); err != nil {
			t.Fatalf("seed %d stay %d: %v", seed, i+1, err)
		}
	}
	return obs
}

func tipToStep4(t *testing.T, seed uint64) Observation {
	t.Helper()
	env, err := New(Config{TimeLimit: 10, Task: TaskAdaptAfterChange})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	env.Reset(seed)
	var obs Observation
	for i := 0; i < 4; i++ {
		if obs, _, _, _, err = env.Step(ActionStay); err != nil {
			t.Fatalf("stay %d: %v", i+1, err)
		}
	}
	return obs
}

func tipToStep6(t *testing.T, seed uint64) Observation {
	t.Helper()
	env, err := New(Config{TimeLimit: 10, Task: TaskAdaptAfterChange})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	env.Reset(seed)
	var obs Observation
	for i := 0; i < 6; i++ {
		if obs, _, _, _, err = env.Step(ActionStay); err != nil {
			t.Fatalf("stay %d: %v", i+1, err)
		}
	}
	return obs
}

func TestLanguageGoalTokenNamesTheCorner(t *testing.T) {
	corners := [4][2]int{{0, 0}, {8, 0}, {0, 8}, {8, 8}} // default 9x9
	seen := [4]bool{}
	for seed := uint64(0); seed < 40; seed++ {
		env, err := New(Config{Task: TaskLanguageGoal})
		if err != nil {
			t.Fatalf("seed %d: New: %v", seed, err)
		}
		_, info := env.Reset(seed)
		token := env.observation().Token
		ones, idx := 0, -1
		for i, v := range token {
			if v == 1 {
				ones++
				idx = i
			}
		}
		if ones != 1 {
			t.Fatalf("seed %d: Token has %d ones, want 1 (%v)", seed, ones, token)
		}
		if info.GoalX != corners[idx][0] || info.GoalY != corners[idx][1] {
			t.Fatalf("seed %d: Token one at %d but goal=(%d,%d), want corner %v",
				seed, idx, info.GoalX, info.GoalY, corners[idx])
		}
		assertNoGoalInView(t, env.observation().View, seed)
		seen[idx] = true
	}
	for i, s := range seen {
		if !s {
			t.Fatalf("corner %d (%v) never appeared in seeds 0..39", i, corners[i])
		}
	}
}

func TestObservationDoesNotLeakTheGoal(t *testing.T) {
	base := make([][]int, 7)
	for y := range base {
		base[y] = make([]int, 7)
	}
	cellsA := copyCells(base)
	cellsA[2][5] = CellGoal
	envA, err := newFromMap(cellsA, 1, 2, 0, 5, 2, Config{})
	if err != nil {
		t.Fatalf("newFromMap A: %v", err)
	}
	envA.task = TaskAvoidObstacles

	cellsB := copyCells(base)
	cellsB[4][5] = CellGoal
	envB, err := newFromMap(cellsB, 1, 2, 0, 5, 4, Config{})
	if err != nil {
		t.Fatalf("newFromMap B: %v", err)
	}
	envB.task = TaskAvoidObstacles

	vecA := envA.observation().Vector()
	vecB := envB.observation().Vector()
	if len(vecA) != len(vecB) {
		t.Fatalf("vector lengths differ: %d vs %d", len(vecA), len(vecB))
	}
	for i := range vecA {
		if vecA[i] != vecB[i] {
			t.Fatalf("vectors differ at index %d: %v vs %v", i, vecA[i], vecB[i])
		}
	}
	// Info differs: different goal coordinates.
	if envA.goalX == envB.goalX && envA.goalY == envB.goalY {
		t.Fatalf("test maps must place the goals at different coordinates")
	}
}

func TestVectorLengthWithCue(t *testing.T) {
	env, err := New(Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	obs, _ := env.Reset(0)
	// 3*3*3 (view) + 4 (heading) + 1 (collided) + 1 (elapsed) + 4 (token)
	// + 1 (rule_changed) + 2 (cue) = 40.
	if n := len(obs.Vector()); n != 40 {
		t.Fatalf("Vector length = %d, want 40", n)
	}
}

func signum(v float64) float64 {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	default:
		return 0
	}
}
