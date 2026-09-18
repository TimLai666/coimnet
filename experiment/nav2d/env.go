package nav2d

import (
	"fmt"
	"math/rand/v2"
)

// Actions.
const (
	ActionForward   = 0
	ActionTurnLeft  = 1
	ActionTurnRight = 2
	ActionStay      = 3
	Actions         = 4
)

// Cell kinds.
const (
	CellFree = 0
	CellWall = 1
	CellGoal = 2
)

// Config declares the map, episode rules and task. Width and Height must be odd
// and >= 5; WallDensity in [0, 0.4]; ViewDepth in [1, 5]; TimeLimit >= 1;
// StepPenalty >= 0; CollisionPenalty >= 0; GoalReward > 0; Task is one of
// remember_goal, avoid_obstacles, adapt_after_change, language_goal ("" maps to
// avoid_obstacles). Zero values take the defaults (9x9, 0.2, 3, 60, 0.01, 0.05,
// 1).
type Config struct {
	Width            int     `json:"width"`
	Height           int     `json:"height"`
	WallDensity      float64 `json:"wall_density"`
	ViewDepth        int     `json:"view_depth"`
	TimeLimit        int     `json:"time_limit"`
	StepPenalty      float64 `json:"step_penalty"`
	CollisionPenalty float64 `json:"collision_penalty"`
	GoalReward       float64 `json:"goal_reward"`
	Task             string  `json:"task"`
}

// withDefaults fills zero fields with their defaults.
func (c Config) withDefaults() Config {
	if c.Width == 0 {
		c.Width = 9
	}
	if c.Height == 0 {
		c.Height = 9
	}
	if c.WallDensity == 0 {
		c.WallDensity = 0.2
	}
	if c.ViewDepth == 0 {
		c.ViewDepth = 3
	}
	if c.TimeLimit == 0 {
		c.TimeLimit = 60
	}
	if c.StepPenalty == 0 {
		c.StepPenalty = 0.01
	}
	if c.CollisionPenalty == 0 {
		c.CollisionPenalty = 0.05
	}
	if c.GoalReward == 0 {
		c.GoalReward = 1
	}
	return c
}

// validate rejects a defaulted configuration. withDefaults must run first.
func validate(c Config) error {
	if c.Width < 5 || c.Width%2 == 0 {
		return fmt.Errorf("nav2d: width %d must be odd and >= 5", c.Width)
	}
	if c.Height < 5 || c.Height%2 == 0 {
		return fmt.Errorf("nav2d: height %d must be odd and >= 5", c.Height)
	}
	if c.WallDensity < 0 || c.WallDensity > 0.4 {
		return fmt.Errorf("nav2d: wall density %v outside [0, 0.4]", c.WallDensity)
	}
	if c.ViewDepth < 1 || c.ViewDepth > 5 {
		return fmt.Errorf("nav2d: view depth %d outside [1, 5]", c.ViewDepth)
	}
	if c.TimeLimit < 1 {
		return fmt.Errorf("nav2d: time limit %d < 1", c.TimeLimit)
	}
	if c.StepPenalty < 0 {
		return fmt.Errorf("nav2d: step penalty %v < 0", c.StepPenalty)
	}
	if c.CollisionPenalty < 0 {
		return fmt.Errorf("nav2d: collision penalty %v < 0", c.CollisionPenalty)
	}
	if c.GoalReward <= 0 {
		return fmt.Errorf("nav2d: goal reward %v <= 0", c.GoalReward)
	}
	switch resolveTask(c.Task) {
	case TaskRememberGoal, TaskAvoidObstacles, TaskAdaptAfterChange, TaskLanguageGoal:
	default:
		return fmt.Errorf("nav2d: task %q is not a known task identifier", c.Task)
	}
	return nil
}

// Observation is everything the agent may see. View is the cone in front of
// the agent: for distance k = 1..ViewDepth the 2k-1 cells at lateral offsets
// -(k-1)..(k-1), in that order, each as a one-hot of the three cell kinds
// (ViewDepth^2 cells x 3 floats); cells outside the map read as walls. For
// remember_goal and language_goal the goal cell reads as free. Heading is a
// one-hot of {0, 90, 180, 270} degrees. Token is a one-hot naming the goal
// corner for language_goal and all zeros otherwise. RuleChanged is 1 only on
// the adapt_after_change step that redraws the walls. Cue is the sign of the
// goal vector for remember_goal on its first observation and zeros otherwise.
// There is no position, no goal coordinate and no path field, and there never
// will be.
type Observation struct {
	View        []float64  `json:"view"`
	Heading     [4]float64 `json:"heading"`
	Collided    float64    `json:"collided"`
	Elapsed     float64    `json:"elapsed"` // steps taken / TimeLimit, in [0, 1]
	Token       [4]float64 `json:"token"`
	RuleChanged float64    `json:"rule_changed"`
	Cue         [2]float64 `json:"cue"`
}

// Vector returns View, Heading, Collided, Elapsed, Token, RuleChanged and Cue
// concatenated.
func (o Observation) Vector() []float64 {
	v := make([]float64, 0, len(o.View)+4+1+1+4+1+2)
	v = append(v, o.View...)
	v = append(v, o.Heading[:]...)
	v = append(v, o.Collided, o.Elapsed)
	v = append(v, o.Token[:]...)
	v = append(v, o.RuleChanged)
	v = append(v, o.Cue[0], o.Cue[1])
	return v
}

// Info is evaluator-only state that must never enter an encoder.
type Info struct {
	X            int  `json:"x"`
	Y            int  `json:"y"`
	Heading      int  `json:"heading"`
	GoalX        int  `json:"goal_x"`
	GoalY        int  `json:"goal_y"`
	Steps        int  `json:"steps"`
	Collisions   int  `json:"collisions"`
	Reached      bool `json:"reached"`
	TimedOut     bool `json:"timed_out"`
	ShortestPath int  `json:"shortest_path"`
}

// Env is a single 2D navigation episode generator. It is not safe for
// concurrent use; one goroutine runs one agent through it at a time.
type Env struct {
	width, height    int
	wallDensity      float64
	viewDepth        int
	timeLimit        int
	stepPenalty      float64
	collisionPenalty float64
	goalReward       float64
	task             string // resolved task for the current episode

	cells        [][]int // rows y, columns x
	x, y         int
	heading      int
	goalX, goalY int
	steps        int
	collisions   int
	collided     bool
	done         bool
	dist         [][]int // BFS distance to the goal, -1 when blocked

	carrySeed   uint64     // episode seed, kept for adapt_after_change redraws
	cue         [2]float64 // remember_goal direction shown on the first observation
	cueSeen     bool       // remember_goal cue already emitted
	token       [4]float64 // language_goal one-hot corner token
	ruleChanged float64    // 1 during the step that redraws the map
	mapChanged  bool       // adapt_after_change already redrawn this episode
}

// New validates the configuration and returns a ready-to-reset environment.
func New(c Config) (*Env, error) {
	c = c.withDefaults()
	if err := validate(c); err != nil {
		return nil, err
	}
	return &Env{
		width:            c.Width,
		height:           c.Height,
		wallDensity:      c.WallDensity,
		viewDepth:        c.ViewDepth,
		timeLimit:        c.TimeLimit,
		stepPenalty:      c.StepPenalty,
		collisionPenalty: c.CollisionPenalty,
		goalReward:       c.GoalReward,
		task:             resolveTask(c.Task),
	}, nil
}

// Reset starts an episode: the map is drawn from rand.New(rand.NewPCG(seed,
// 0)) with every cell a wall with probability WallDensity, the start and the
// goal are two distinct free cells chosen with the same RNG, and the draw
// repeats (at most 100 times, then with the density halved on each further
// try) until BFS connects start to goal. Heading starts at 0. For
// language_goal the goal is a random free corner (its token records the
// index) and the start is drawn separately; the draw also repeats until
// connected. For remember_goal the goal direction is put into Cue, shown on
// the first observation only. Returns the first observation and the info with
// ShortestPath from BFS.
func (e *Env) Reset(seed uint64) (Observation, Info) {
	e.carrySeed = seed
	e.cue = [2]float64{}
	e.cueSeen = false
	e.token = [4]float64{}
	e.ruleChanged = 0
	e.mapChanged = false
	rng := rand.New(rand.NewPCG(seed, 0))
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
		var g [2]int
		goalIdx := -1
		if e.task == TaskLanguageGoal {
			g, goalIdx = pickLanguageGoal(rng, e.width, e.height, cells)
		}
		free := make([][2]int, 0, e.width*e.height)
		for y := 0; y < e.height; y++ {
			for x := 0; x < e.width; x++ {
				if cells[y][x] == CellFree {
					free = append(free, [2]int{x, y})
				}
			}
		}
		if len(free) < 2 {
			continue
		}
		s := free[rng.IntN(len(free))]
		if goalIdx < 0 {
			for {
				g = free[rng.IntN(len(free))]
				if g != s {
					break
				}
			}
		} else {
			for g == s {
				s = free[rng.IntN(len(free))]
			}
		}
		dist := bfsDist(cells, e.width, e.height, g[0], g[1])
		if dist[s[1]][s[0]] < 0 {
			continue
		}
		cells[g[1]][g[0]] = CellGoal
		e.cells = cells
		e.x, e.y = s[0], s[1]
		e.heading = 0
		e.goalX, e.goalY = g[0], g[1]
		e.steps = 0
		e.collisions = 0
		e.collided = false
		e.done = false
		e.dist = dist
		if goalIdx >= 0 {
			e.token[goalIdx] = 1
		}
		if e.task == TaskRememberGoal {
			e.cue = [2]float64{signInt(e.goalX - e.x), signInt(e.goalY - e.y)}
		}
		return e.observation(), Info{
			X: s[0], Y: s[1], Heading: 0,
			GoalX: g[0], GoalY: g[1],
			Steps:        0,
			Collisions:   0,
			ShortestPath: dist[s[1]][s[0]],
		}
	}
}

// Step applies an action and returns the next observation, the reward,
// whether the episode is done (goal reached or time limit, reported
// separately and never both true) and the info. A step after done returns an
// error. Forward moves one cell in the heading unless the cell is a wall or
// outside the map (then the position stays, Collided is 1 and the collision
// penalty is charged); turns change the heading by +-90 degrees; stay does
// nothing. Reward = -StepPenalty - (collision penalty) + GoalReward on the
// step that reaches the goal.
func (e *Env) Step(action int) (Observation, float64, bool, Info, error) {
	if e.done {
		return Observation{}, 0, true, Info{}, fmt.Errorf("nav2d: step after done")
	}
	if action < 0 || action >= Actions {
		return Observation{}, 0, false, Info{}, fmt.Errorf("nav2d: action %d out of [0, %d)", action, Actions)
	}
	e.collided = false
	e.ruleChanged = 0
	switch action {
	case ActionForward:
		fx, fy := forward(e.heading)
		nx, ny := e.x+fx, e.y+fy
		if nx < 0 || ny < 0 || nx >= e.width || ny >= e.height || e.cells[ny][nx] == CellWall {
			e.collided = true
			e.collisions++
		} else {
			e.x, e.y = nx, ny
		}
	case ActionTurnLeft:
		e.heading = (e.heading + 90) % 360
	case ActionTurnRight:
		e.heading = (e.heading + 270) % 360
	}
	e.steps++
	if e.task == TaskAdaptAfterChange && e.steps == e.timeLimit/2 && !e.mapChanged {
		e.redrawWalls(e.carrySeed)
		e.ruleChanged = 1
		e.mapChanged = true
	}
	reached := e.x == e.goalX && e.y == e.goalY
	reward := -e.stepPenalty
	if e.collided {
		reward -= e.collisionPenalty
	}
	if reached {
		reward += e.goalReward
	}
	e.done = reached || e.steps >= e.timeLimit
	info := Info{
		X: e.x, Y: e.y, Heading: e.heading,
		GoalX: e.goalX, GoalY: e.goalY,
		Steps:        e.steps,
		Collisions:   e.collisions,
		Reached:      reached,
		TimedOut:     !reached && e.steps >= e.timeLimit,
		ShortestPath: e.dist[e.y][e.x],
	}
	return e.observation(), reward, e.done, info, nil
}

// Expert returns the action that follows a shortest path to the goal: forward
// when the next path cell is straight ahead, otherwise the turn that faces it
// (left when it is 90 degrees to the left or behind, right otherwise). Only
// for imitation labels; evaluators never call it.
func (e *Env) Expert() int {
	if e.done || e.dist[e.y][e.x] < 0 || e.dist[e.y][e.x] == 0 {
		return ActionStay
	}
	nx, ny, ok := e.nextPathCell()
	if !ok {
		return ActionStay
	}
	fx, fy := forward(e.heading)
	if nx-e.x == fx && ny-e.y == fy {
		return ActionForward
	}
	ang := 0
	switch {
	case nx > e.x:
		ang = 0
	case ny < e.y:
		ang = 90
	case nx < e.x:
		ang = 180
	default:
		ang = 270
	}
	switch (ang - e.heading + 360) % 360 {
	case 90, 180:
		return ActionTurnLeft
	default:
		return ActionTurnRight
	}
}

// Cells returns a copy of the map (rows y, columns x).
func (e *Env) Cells() [][]int {
	out := make([][]int, len(e.cells))
	for i := range e.cells {
		out[i] = append([]int(nil), e.cells[i]...)
	}
	return out
}

// newFromMap builds an environment from a literal map for tests. The goal
// cell must be readable, start and goal must be in bounds, the heading must
// be a multiple of 90 degrees, and BFS must connect start to goal.
func newFromMap(cells [][]int, x, y, heading, goalX, goalY int, c Config) (*Env, error) {
	c = c.withDefaults()
	if err := validate(c); err != nil {
		return nil, err
	}
	if len(cells) == 0 {
		return nil, fmt.Errorf("nav2d: empty map")
	}
	width := len(cells[0])
	if width == 0 {
		return nil, fmt.Errorf("nav2d: empty map rows")
	}
	for i := range cells {
		if len(cells[i]) != width {
			return nil, fmt.Errorf("nav2d: ragged map row %d (%d != %d)", i, len(cells[i]), width)
		}
	}
	height := len(cells)
	if x < 0 || y < 0 || x >= width || y >= height {
		return nil, fmt.Errorf("nav2d: start (%d,%d) outside %dx%d map", x, y, width, height)
	}
	if goalX < 0 || goalY < 0 || goalX >= width || goalY >= height {
		return nil, fmt.Errorf("nav2d: goal (%d,%d) outside %dx%d map", goalX, goalY, width, height)
	}
	if cells[goalY][goalX] != CellGoal {
		return nil, fmt.Errorf("nav2d: goal cell (%d,%d) is kind %d, want CellGoal", goalX, goalY, cells[goalY][goalX])
	}
	heading = (heading%360 + 360) % 360
	if heading%90 != 0 {
		return nil, fmt.Errorf("nav2d: heading %d not a multiple of 90", heading)
	}
	e := &Env{
		width:            width,
		height:           height,
		wallDensity:      c.WallDensity,
		viewDepth:        c.ViewDepth,
		timeLimit:        c.TimeLimit,
		stepPenalty:      c.StepPenalty,
		collisionPenalty: c.CollisionPenalty,
		goalReward:       c.GoalReward,
		cells:            copyCells(cells),
		x:                x,
		y:                y,
		heading:          heading,
		goalX:            goalX,
		goalY:            goalY,
	}
	e.dist = bfsDist(e.cells, e.width, e.height, goalX, goalY)
	if e.dist[e.y][e.x] < 0 {
		return nil, fmt.Errorf("nav2d: goal unreachable from (%d,%d)", x, y)
	}
	return e, nil
}

func copyCells(cells [][]int) [][]int {
	out := make([][]int, len(cells))
	for i := range cells {
		out[i] = append([]int(nil), cells[i]...)
	}
	return out
}

func bfsDist(cells [][]int, width, height, gx, gy int) [][]int {
	dist := make([][]int, height)
	for y := range dist {
		dist[y] = make([]int, width)
		for x := range dist[y] {
			dist[y][x] = -1
		}
	}
	dist[gy][gx] = 0
	q := make([][2]int, 1, width*height)
	q[0] = [2]int{gx, gy}
	for len(q) > 0 {
		cur := q[0]
		q = q[1:]
		for _, d := range [][2]int{{1, 0}, {0, 1}, {-1, 0}, {0, -1}} {
			nx, ny := cur[0]+d[0], cur[1]+d[1]
			if nx < 0 || ny < 0 || nx >= width || ny >= height {
				continue
			}
			if dist[ny][nx] != -1 || cells[ny][nx] == CellWall {
				continue
			}
			dist[ny][nx] = dist[cur[1]][cur[0]] + 1
			q = append(q, [2]int{nx, ny})
		}
	}
	return dist
}

func (e *Env) nextPathCell() (int, int, bool) {
	want := e.dist[e.y][e.x] - 1
	for _, d := range [][2]int{{1, 0}, {0, 1}, {-1, 0}, {0, -1}} {
		nx, ny := e.x+d[0], e.y+d[1]
		if nx < 0 || ny < 0 || nx >= e.width || ny >= e.height {
			continue
		}
		if e.dist[ny][nx] == want {
			return nx, ny, true
		}
	}
	return 0, 0, false
}

// forward returns the movement vector for a heading in degrees.
func forward(heading int) (int, int) {
	switch heading {
	case 90:
		return 0, -1
	case 180:
		return -1, 0
	case 270:
		return 0, 1
	default:
		return 1, 0
	}
}

// right returns the vector pointing to the right of a heading in degrees.
func right(heading int) (int, int) {
	switch heading {
	case 90:
		return 1, 0
	case 180:
		return 0, -1
	case 270:
		return -1, 0
	default:
		return 0, 1
	}
}

// observation builds the current observation from the agent state. The view
// cone is relative to the agent: for distance k the 2k-1 cells at lateral
// offsets -(k-1)..(k-1), where a positive lateral offset is to the right of
// the heading. remember_goal and language_goal hide the goal cell in the view
// (it reads as free), remember_goal emits its Cue on the first observation
// only, and RuleChanged reflects the adapt_after_change redraw step.
func (e *Env) observation() Observation {
	view := make([]float64, e.viewDepth*e.viewDepth*3)
	fx, fy := forward(e.heading)
	rx, ry := right(e.heading)
	hideGoal := e.task == TaskRememberGoal || e.task == TaskLanguageGoal
	i := 0
	for k := 1; k <= e.viewDepth; k++ {
		for lat := -(k - 1); lat <= k-1; lat++ {
			cx := e.x + k*fx + lat*rx
			cy := e.y + k*fy + lat*ry
			callView := CellWall
			if cx >= 0 && cy >= 0 && cx < e.width && cy < e.height {
				callView = e.cells[cy][cx]
				if hideGoal && callView == CellGoal {
					callView = CellFree
				}
			}
			view[i*3+callView] = 1
			i++
		}
	}
	heading := [4]float64{}
	heading[e.heading/90] = 1
	var cue [2]float64
	if e.task == TaskRememberGoal && !e.cueSeen {
		cue = e.cue
		e.cueSeen = true
	}
	return Observation{
		View:        view,
		Heading:     heading,
		Collided:    boolToFloat(e.collided),
		Elapsed:     float64(e.steps) / float64(e.timeLimit),
		Token:       e.token,
		RuleChanged: e.ruleChanged,
		Cue:         cue,
	}
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
