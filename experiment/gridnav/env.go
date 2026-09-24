package gridnav

import (
	"fmt"
	"math"
	"math/rand/v2"
)

// Actions.
const (
	ActionLeft  = 0
	ActionRight = 1
	ActionStay  = 2
	Actions     = 3
)

// Config declares the corridor. Length must be odd and >= 3 so the start is
// the centre cell; TimeLimit >= 1; StepPenalty >= 0; GoalReward > 0.
type Config struct {
	Length      int     `json:"length"`
	TimeLimit   int     `json:"time_limit"`
	StepPenalty float64 `json:"step_penalty"`
	GoalReward  float64 `json:"goal_reward"`
}

// Observation is everything the agent may see: the cue is +1 (goal on the
// right) or -1 (goal on the left) only on the first step of an episode and 0
// afterwards; Position is NOT included, only the two wall indicators, so the
// agent has to remember the cue. Fields are exported floats so they can be
// fed to an encoder directly.
type Observation struct {
	Cue     float64 `json:"cue"`
	AtLeft  float64 `json:"at_left"`  // 1 when the agent is in cell 0
	AtRight float64 `json:"at_right"` // 1 when the agent is in cell Length-1
	Elapsed float64 `json:"elapsed"`  // steps taken / TimeLimit, in [0, 1]
}

// Info is evaluator-only state that must never enter an encoder.
type Info struct {
	Position int  `json:"position"`
	Goal     int  `json:"goal"`
	Steps    int  `json:"steps"`
	Reached  bool `json:"reached"`
	TimedOut bool `json:"timed_out"`
}

// Env is a single corridor episode generator. It is not safe for concurrent
// use; one goroutine runs one agent through it at a time.
type Env struct {
	length      int
	timeLimit   int
	stepPenalty float64
	goalReward  float64

	position int
	goal     int
	steps    int
	done     bool
}

// New validates the configuration and returns a ready-to-reset environment.
// StepPenalty must be a finite value >= 0 and GoalReward a finite value > 0;
// NaN and the infinities are rejected.
func New(c Config) (*Env, error) {
	if c.Length < 3 {
		return nil, fmt.Errorf("gridnav: length %d < 3", c.Length)
	}
	if c.Length%2 == 0 {
		return nil, fmt.Errorf("gridnav: length %d is even, want odd", c.Length)
	}
	if c.TimeLimit < 1 {
		return nil, fmt.Errorf("gridnav: time limit %d < 1", c.TimeLimit)
	}
	if math.IsNaN(c.StepPenalty) || math.IsInf(c.StepPenalty, 0) || c.StepPenalty < 0 {
		return nil, fmt.Errorf("gridnav: step penalty %v must be a finite value >= 0", c.StepPenalty)
	}
	if math.IsNaN(c.GoalReward) || math.IsInf(c.GoalReward, 0) || c.GoalReward <= 0 {
		return nil, fmt.Errorf("gridnav: goal reward %v must be a finite value > 0", c.GoalReward)
	}
	return &Env{
		length:      c.Length,
		timeLimit:   c.TimeLimit,
		stepPenalty: c.StepPenalty,
		goalReward:  c.GoalReward,
	}, nil
}

// Reset starts an episode: goal side from the seed (PCG(seed, 0) first draw,
// even -> right, odd -> left), agent at the centre, returns the first
// observation (with the cue) and the info.
func (e *Env) Reset(seed uint64) (Observation, Info) {
	e.position = e.length / 2
	e.steps = 0
	e.done = false
	d := rand.New(rand.NewPCG(seed, 0)).Uint64()
	if d%2 == 0 {
		e.goal = e.length - 1
		return Observation{Cue: 1}, Info{Position: e.position, Goal: e.goal}
	}
	e.goal = 0
	return Observation{Cue: -1}, Info{Position: e.position, Goal: e.goal}
}

// Step applies an action, returns the next observation, the reward, whether
// the episode is done (goal reached or time limit) and the info. A step after
// done returns an error. Moving into a wall keeps the position (no extra
// penalty). Reward = -StepPenalty every step, +GoalReward on the step that
// reaches the goal cell.
func (e *Env) Step(action int) (Observation, float64, bool, Info, error) {
	if e.done {
		return Observation{}, 0, true, Info{}, fmt.Errorf("gridnav: step after done")
	}
	if action < 0 || action >= Actions {
		return Observation{}, 0, false, Info{}, fmt.Errorf("gridnav: action %d out of [0, %d)", action, Actions)
	}
	switch action {
	case ActionLeft:
		if e.position > 0 {
			e.position--
		}
	case ActionRight:
		if e.position < e.length-1 {
			e.position++
		}
	}
	e.steps++
	reached := e.position == e.goal
	reward := -e.stepPenalty
	if reached {
		reward += e.goalReward
	}
	e.done = reached || e.steps >= e.timeLimit
	info := Info{
		Position: e.position,
		Goal:     e.goal,
		Steps:    e.steps,
		Reached:  reached,
		TimedOut: !reached && e.steps >= e.timeLimit,
	}
	obs := Observation{
		AtLeft:  boolToFloat(e.position == 0),
		AtRight: boolToFloat(e.position == e.length-1),
		Elapsed: float64(e.steps) / float64(e.timeLimit),
	}
	return obs, reward, e.done, info, nil
}

// Expert returns the action that moves toward the goal (Stay when already
// there). It reads the hidden goal, so it is training-label material only.
func (e *Env) Expert() int {
	switch {
	case e.position < e.goal:
		return ActionRight
	case e.position > e.goal:
		return ActionLeft
	default:
		return ActionStay
	}
}

// Vector returns the observation as [cue, at_left, at_right, elapsed].
func (o Observation) Vector() []float64 {
	return []float64{o.Cue, o.AtLeft, o.AtRight, o.Elapsed}
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
