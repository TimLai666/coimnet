package nav2d

import "math/rand/v2"

// Task identifiers, one per navigation variation. The empty Config.Task
// resolves to avoid_obstacles.
const (
	TaskRememberGoal     = "remember_goal"      // the goal never appears in the view; Cue gives its direction on step 0 only
	TaskAvoidObstacles   = "avoid_obstacles"    // the goal is visible whenever it is in the cone
	TaskAdaptAfterChange = "adapt_after_change" // at step TimeLimit/2 the walls are redrawn and RuleChanged is 1 on that step only
	TaskLanguageGoal     = "language_goal"      // the goal is one of the four corners named by Token; the goal is hidden in the view
)

// resolveTask maps the zero task to avoid_obstacles.
func resolveTask(task string) string {
	if task == "" {
		return TaskAvoidObstacles
	}
	return task
}

// corner returns the position of corner i on a width x height map, in order
// (0,0), (width-1,0), (0,height-1), (width-1,height-1).
func corner(width, height, i int) [2]int {
	switch i {
	case 0:
		return [2]int{0, 0}
	case 1:
		return [2]int{width - 1, 0}
	case 2:
		return [2]int{0, height - 1}
	default:
		return [2]int{width - 1, height - 1}
	}
}

func signInt(v int) float64 {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	default:
		return 0
	}
}

// pickLanguageGoal draws the language_goal goal from the same RNG stream as
// the walls, forces all four corners free and returns the chosen corner and
// its token index (0..3 in corner order).
func pickLanguageGoal(rng *rand.Rand, width, height int, cells [][]int) ([2]int, int) {
	idx := rng.IntN(4)
	for i := 0; i < 4; i++ {
		c := corner(width, height, i)
		cells[c[1]][c[0]] = CellFree
	}
	return corner(width, height, idx), idx
}

// redrawWalls re-generates the maze for adapt_after_change on the step that
// crosses steps == TimeLimit/2. The walls come from a new PCG stream seeded
// from the episode seed with counter 1 at the same wall density; the agent
// cell stays free, the goal cell stays the goal, and the draw retries (halving
// the density after 100 tries, as Reset does) until BFS connects the agent to
// the goal.
func (e *Env) redrawWalls(seed uint64) {
	rng := rand.New(rand.NewPCG(seed, 1))
	density := e.wallDensity
	for attempt := 1; ; attempt++ {
		if attempt > 100 {
			density /= 2
		}
		cells := make([][]int, e.height)
		for y := range cells {
			cells[y] = make([]int, e.width)
			for x := range cells[y] {
				if rng.Float64() < density {
					cells[y][x] = CellWall
				}
			}
		}
		cells[e.y][e.x] = CellFree
		cells[e.goalY][e.goalX] = CellGoal
		dist := bfsDist(cells, e.width, e.height, e.goalX, e.goalY)
		if dist[e.y][e.x] < 0 {
			continue
		}
		e.cells = cells
		e.dist = dist
		return
	}
}
