package nav2d

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func resetSnapshot(e *Env, seed uint64) ([][]int, Info) {
	_, info := e.Reset(seed)
	return e.Cells(), info
}

func TestResetIsDeterministicAndConnected(t *testing.T) {
	env, err := New(Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for seed := uint64(0); seed < 20; seed++ {
		cellsA, infoA := resetSnapshot(env, seed)
		cellsB, infoB := resetSnapshot(env, seed)
		if !reflect.DeepEqual(cellsA, cellsB) {
			t.Fatalf("seed %d: same-seed reset differs in cells", seed)
		}
		if infoA != infoB {
			t.Fatalf("seed %d: same-seed reset differs in info: %+v vs %+v", seed, infoA, infoB)
		}
		if infoA.ShortestPath < 1 {
			t.Fatalf("seed %d: shortest path %d < 1", seed, infoA.ShortestPath)
		}
		if infoA.X == infoA.GoalX && infoA.Y == infoA.GoalY {
			t.Fatalf("seed %d: start == goal at (%d,%d)", seed, infoA.X, infoA.Y)
		}
		if cellsA[infoA.Y][infoA.X] != CellFree {
			t.Fatalf("seed %d: start (%d,%d) is kind %d, want CellFree", seed, infoA.X, infoA.Y, cellsA[infoA.Y][infoA.X])
		}
		if cellsA[infoA.GoalY][infoA.GoalX] != CellGoal {
			t.Fatalf("seed %d: goal (%d,%d) is kind %d, want CellGoal", seed, infoA.GoalX, infoA.GoalY, cellsA[infoA.GoalY][infoA.GoalX])
		}
	}
}

func TestObservationHasNoCoordinates(t *testing.T) {
	typ := reflect.TypeOf(Observation{})
	if n := typ.NumField(); n != 7 {
		t.Fatalf("Observation has %d fields, want 7", n)
	}
	want := []string{"View", "Heading", "Collided", "Elapsed", "Token", "RuleChanged", "Cue"}
	for _, name := range want {
		if _, ok := typ.FieldByName(name); !ok {
			t.Fatalf("Observation missing field %s", name)
		}
	}
	forbidden := []string{"x", "y", "goal", "path", "position"}
	for i := 0; i < typ.NumField(); i++ {
		lower := strings.ToLower(typ.Field(i).Name)
		for _, f := range forbidden {
			if strings.Contains(lower, f) {
				t.Fatalf("Observation field %s leaks forbidden name %q", typ.Field(i).Name, f)
			}
		}
	}
	env, err := New(Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	obs, _ := env.Reset(0)
	if n := len(obs.Vector()); n != 40 {
		t.Fatalf("Vector length = %d, want 40 (3*3*3 + 4 + 1 + 1 + 4 + 1 + 2)", n)
	}
}

func viewIndex(k, lat int) int { return (k-1)*(k-1) + lat + (k - 1) }

func assertView(t *testing.T, view []float64, k, lat, kind int, where string) {
	t.Helper()
	idx := viewIndex(k, lat) * 3
	for i := 0; i < 3; i++ {
		var want float64
		if i == kind {
			want = 1
		}
		if view[idx+i] != want {
			t.Fatalf("%s: view[%d:%d] = %v, want one-hot cell kind %d", where, idx, idx+3, view[idx:idx+3], kind)
		}
	}
}

func TestViewConeHandMap(t *testing.T) {
	free, wall, goal := CellFree, CellWall, CellGoal
	cells := [][]int{
		{free, free, free, free, free},
		{goal, free, free, free, free},
		{free, free, wall, free, free},
		{free, free, free, free, free},
		{free, free, free, free, free},
	}
	north, err := newFromMap(cells, 2, 4, 90, 0, 1, Config{})
	if err != nil {
		t.Fatalf("newFromMap north: %v", err)
	}
	obs := north.observation()
	if len(obs.View) != 27 {
		t.Fatalf("view length = %d, want 27", len(obs.View))
	}
	if obs.Heading != [4]float64{0, 1, 0, 0} {
		t.Fatalf("heading north = %v, want [0 1 0 0]", obs.Heading)
	}
	assertView(t, obs.View, 1, 0, CellFree, "north k1 (2,3)")
	assertView(t, obs.View, 2, -1, CellFree, "north k2 (1,2)")
	assertView(t, obs.View, 2, 0, CellWall, "north k2 (2,2)")
	assertView(t, obs.View, 2, 1, CellFree, "north k2 (3,2)")
	assertView(t, obs.View, 3, -2, CellGoal, "north k3 (0,1)")
	assertView(t, obs.View, 3, -1, CellFree, "north k3 (1,1)")
	assertView(t, obs.View, 3, 2, CellFree, "north k3 (4,1)")

	east, err := newFromMap(cells, 2, 4, 0, 0, 1, Config{})
	if err != nil {
		t.Fatalf("newFromMap east: %v", err)
	}
	obsE := east.observation()
	if obsE.Heading != [4]float64{1, 0, 0, 0} {
		t.Fatalf("heading east = %v, want [1 0 0 0]", obsE.Heading)
	}
	assertView(t, obsE.View, 1, 0, CellFree, "east k1 (3,4)")
	assertView(t, obsE.View, 2, -1, CellFree, "east k2 (4,3)")
	assertView(t, obsE.View, 2, 0, CellFree, "east k2 (4,4)")
	assertView(t, obsE.View, 2, 1, CellWall, "east k2 (4,5) out of map")
}

func TestCollisionAndRewards(t *testing.T) {
	free, wall, goal := CellFree, CellWall, CellGoal
	cells := [][]int{
		{free, wall, free, free, free},
		{free, free, free, free, free},
		{free, free, free, free, free},
		{free, free, free, free, free},
		{goal, free, free, free, free},
	}
	env, err := newFromMap(cells, 0, 0, 0, 0, 4, Config{})
	if err != nil {
		t.Fatalf("newFromMap: %v", err)
	}
	obs, rew, done, info, err := env.Step(ActionForward)
	if err != nil {
		t.Fatalf("forward into wall: %v", err)
	}
	if info.X != 0 || info.Y != 0 {
		t.Fatalf("position after collision = (%d,%d), want (0,0)", info.X, info.Y)
	}
	if obs.Collided != 1 {
		t.Fatalf("Collided = %v, want 1", obs.Collided)
	}
	if !near(rew, -0.01-0.05) {
		t.Fatalf("collision reward = %v, want -0.06", rew)
	}
	if info.Collisions != 1 {
		t.Fatalf("Collisions = %d, want 1", info.Collisions)
	}
	if done {
		t.Fatalf("collision ended the episode")
	}
	obs, rew, _, info, err = env.Step(ActionStay)
	if err != nil {
		t.Fatalf("stay: %v", err)
	}
	if obs.Collided != 0 {
		t.Fatalf("stay Collided = %v, want 0", obs.Collided)
	}
	if !near(rew, -0.01) {
		t.Fatalf("stay reward = %v, want -0.01", rew)
	}
	if info.X != 0 || info.Y != 0 {
		t.Fatalf("position after stay = (%d,%d), want (0,0)", info.X, info.Y)
	}
	if info.Collisions != 1 {
		t.Fatalf("Collisions after stay = %d, want 1", info.Collisions)
	}
}

func TestTerminationAndTimeoutAreSeparate(t *testing.T) {
	free, goal := CellFree, CellGoal
	cells := [][]int{
		{free, goal, free, free, free},
		{free, free, free, free, free},
		{free, free, free, free, free},
		{free, free, free, free, free},
		{free, free, free, free, free},
	}
	env, err := newFromMap(cells, 0, 0, 0, 1, 0, Config{})
	if err != nil {
		t.Fatalf("newFromMap: %v", err)
	}
	obs, rew, done, info, err := env.Step(ActionForward)
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	if !done {
		t.Fatalf("reaching the goal did not end the episode")
	}
	if !info.Reached || info.TimedOut {
		t.Fatalf("goal step info = %+v, want Reached and not TimedOut", info)
	}
	if obs.Collided != 0 {
		t.Fatalf("goal step Collided = %v, want 0", obs.Collided)
	}
	if !near(rew, -0.01+1) {
		t.Fatalf("goal reward = %v, want 0.99", rew)
	}
	if _, _, _, _, err = env.Step(ActionStay); err == nil {
		t.Fatalf("step after reaching the goal succeeded, want error")
	}

	timeout, err := New(Config{TimeLimit: 2})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	timeout.Reset(7)
	var infoT Info
	var doneT bool
	for i := 0; i < 2; i++ {
		_, _, doneT, infoT, err = timeout.Step(ActionStay)
		if err != nil {
			t.Fatalf("stay %d: %v", i+1, err)
		}
		if i == 0 && doneT {
			t.Fatalf("done at stay 1, want at stay 2")
		}
	}
	if !doneT || !infoT.TimedOut || infoT.Reached {
		t.Fatalf("timeout info = %+v, want TimedOut and not Reached", infoT)
	}
	if _, _, _, _, err = timeout.Step(ActionStay); err == nil {
		t.Fatalf("step after timeout succeeded, want error")
	}
}

func TestExpertReachesGoalWithinBudget(t *testing.T) {
	env, err := New(Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for seed := uint64(0); seed < 20; seed++ {
		_, info := env.Reset(seed)
		budget := 3*info.ShortestPath + 2
		reached := false
		steps := 0
		for steps = 0; steps < budget; steps++ {
			var done bool
			var infoS Info
			_, _, done, infoS, err = env.Step(env.Expert())
			if err != nil {
				t.Fatalf("seed %d step %d: %v", seed, steps+1, err)
			}
			if done {
				if !infoS.Reached {
					t.Fatalf("seed %d: done without reaching the goal: %+v", seed, infoS)
				}
				reached = true
				steps++
				break
			}
		}
		if !reached {
			t.Fatalf("seed %d: expert did not reach the goal within %d steps", seed, budget)
		}
		if steps > budget {
			t.Fatalf("seed %d: expert took %d steps, want <= %d", seed, steps, budget)
		}
	}
}
