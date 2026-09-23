package experiment

import (
	"context"
	"fmt"
	"math/rand/v2"

	"github.com/TimLai666/coimnet/experiment/nav2d"
)

// Schema versions of the nav2d report documents.
const (
	// Nav2DSchemaVersion identifies one single-task nav2d report.
	Nav2DSchemaVersion = "coimnet-nav2d/v1"
	// Nav2DSuiteSchemaVersion identifies the four-task nav2d suite report.
	Nav2DSuiteSchemaVersion = "coimnet-nav2d-suite/v1"
)

// nav2dSuiteTasks is the fixed task order of RunNav2DSuite.
var nav2dSuiteTasks = []string{
	nav2d.TaskRememberGoal,
	nav2d.TaskAvoidObstacles,
	nav2d.TaskAdaptAfterChange,
	nav2d.TaskLanguageGoal,
}

// RewireReport is the check list the ticket requires for the rewired control.
type RewireReport struct {
	Edges            int  `json:"edges"`             // recurrent edges considered (hidden→hidden only)
	Attempts         int  `json:"attempts"`          // 10 x Edges double-edge swap attempts drawn
	Accepted         int  `json:"accepted"`          // swaps that were applied
	DegreesKept      bool `json:"degrees_kept"`      // every hidden node keeps its in- and out-degree within the block
	Duplicates       int  `json:"duplicates"`        // duplicate source-target pairs in the result; must be 0
	SelfLoopsBefore  int  `json:"self_loops_before"` // source == target edges before the rewire
	SelfLoopsAfter   int  `json:"self_loops_after"`  // never larger than before: a swap that creates a self loop is refused
	DirectionKept    bool `json:"direction_kept"`    // only hidden→hidden edges moved; input and readout edges untouched
	ReadoutReachable bool `json:"readout_reachable"` // every readout node reachable from the input nodes (BFS on the rewired edges)
}

// nav2dRewire applies attempts = 10 × (number of hidden→hidden edges) double-edge swaps drawn from
// rand.New(rand.NewPCG(seed, 6)) to the hidden→hidden edges of (sources, targets) in place of a copy; it returns the new
// targets and the report. Edge i keeps its position, weight and source; only targets move. Each attempt picks two
// distinct hidden-block edge indices i and j and proposes (a→b),(c→d) → (a→d),(c→b); the proposal is refused when
// it would create a self loop and then when either new pair already exists, so every hidden node keeps its in- and
// out-degree, pairs stay unique and no readout edge is touched.
func nav2dRewire(sources, targets []int, hiddenFirst, hidden int, seed uint64) ([]int, RewireReport, error) {
	var report RewireReport
	if len(sources) != len(targets) {
		return nil, report, fmt.Errorf("nav2d rewire: %d sources and %d targets", len(sources), len(targets))
	}
	if hiddenFirst < 0 || hidden <= 0 {
		return nil, report, fmt.Errorf("nav2d rewire: hidden block needs hiddenFirst >= 0 and hidden > 0, got %d and %d", hiddenFirst, hidden)
	}
	hiddenLast := hiddenFirst + hidden
	out := append([]int(nil), targets...)
	selfLoops := func(ts []int) int {
		n := 0
		for i := range ts {
			if sources[i] == ts[i] {
				n++
			}
		}
		return n
	}
	report.SelfLoopsBefore = selfLoops(targets)
	block := make([]int, 0, hidden*hidden)
	for i := range targets {
		if sources[i] >= hiddenFirst && sources[i] < hiddenLast &&
			targets[i] >= hiddenFirst && targets[i] < hiddenLast {
			block = append(block, i)
		}
	}
	report.Edges = len(block)
	report.Attempts = 10 * len(block)
	pairs := make(map[[2]int]bool, len(sources))
	for i := range sources {
		pairs[[2]int{sources[i], targets[i]}] = true
	}
	n := len(block)
	if n >= 2 {
		rng := rand.New(rand.NewPCG(seed, 6))
		for attempt := 0; attempt < report.Attempts; attempt++ {
			// Two distinct indices from exactly two draws, so the draw
			// sequence depends only on the seed and the attempt count.
			i := rng.IntN(n)
			j := rng.IntN(n - 1)
			if j >= i {
				j++
			}
			gi, gj := block[i], block[j]
			a, b := sources[gi], out[gi]
			c, d := sources[gj], out[gj]
			if a == d || c == b {
				continue // a swap that creates a self loop is refused
			}
			if pairs[[2]int{a, d}] || pairs[[2]int{c, b}] {
				continue // a swap that creates a duplicate pair is refused
			}
			delete(pairs, [2]int{a, b})
			delete(pairs, [2]int{c, d})
			pairs[[2]int{a, d}] = true
			pairs[[2]int{c, b}] = true
			out[gi], out[gj] = d, b
			report.Accepted++
		}
	}
	report.SelfLoopsAfter = selfLoops(out)
	report.DegreesKept = nav2dDegreesMatch(sources, targets, out, hiddenFirst, hidden)
	seen := make(map[[2]int]int, len(sources))
	for i := range sources {
		seen[[2]int{sources[i], out[i]}]++
	}
	for _, count := range seen {
		if count > 1 {
			report.Duplicates += count - 1
		}
	}
	report.DirectionKept = true
	for i := range out {
		inBlock := sources[i] >= hiddenFirst && sources[i] < hiddenLast &&
			targets[i] >= hiddenFirst && targets[i] < hiddenLast
		if !inBlock && out[i] != targets[i] {
			report.DirectionKept = false
			break
		}
	}
	report.ReadoutReachable = nav2dReadoutReachable(sources, out, hiddenFirst, hidden)
	return out, report, nil
}

// nav2dDegreesMatch reports whether every hidden node has the same in- and
// out-degree within the hidden block in the two target lists (edges whose
// source and original target are both hidden nodes; sources never change).
func nav2dDegreesMatch(sources, before, after []int, hiddenFirst, hidden int) bool {
	if len(before) != len(sources) || len(after) != len(sources) {
		return false
	}
	hiddenLast := hiddenFirst + hidden
	beforeIn := make([]int, hidden)
	beforeOut := make([]int, hidden)
	afterIn := make([]int, hidden)
	afterOut := make([]int, hidden)
	for i := range sources {
		s := sources[i]
		if s < hiddenFirst || s >= hiddenLast ||
			before[i] < hiddenFirst || before[i] >= hiddenLast {
			continue
		}
		beforeOut[s-hiddenFirst]++
		beforeIn[before[i]-hiddenFirst]++
		if after[i] < hiddenFirst || after[i] >= hiddenLast {
			continue
		}
		afterOut[s-hiddenFirst]++
		afterIn[after[i]-hiddenFirst]++
	}
	for h := 0; h < hidden; h++ {
		if beforeIn[h] != afterIn[h] || beforeOut[h] != afterOut[h] {
			return false
		}
	}
	return true
}

// nav2dReadoutReachable breadth-first searches the rewired graph from the
// input nodes (0..hiddenFirst-1) and reports whether every node at or above
// the readout block (hiddenFirst+hidden) is reached.
func nav2dReadoutReachable(sources, targets []int, hiddenFirst, hidden int) bool {
	nodes := 0
	for _, v := range sources {
		if v+1 > nodes {
			nodes = v + 1
		}
	}
	for _, v := range targets {
		if v+1 > nodes {
			nodes = v + 1
		}
	}
	adj := make([][]int, nodes)
	for i, s := range sources {
		adj[s] = append(adj[s], targets[i])
	}
	seen := make([]bool, nodes)
	queue := make([]int, 0, nodes)
	for v := 0; v < hiddenFirst && v < nodes; v++ {
		if !seen[v] {
			seen[v] = true
			queue = append(queue, v)
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, next := range adj[cur] {
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	for v := hiddenFirst + hidden; v < nodes; v++ {
		if !seen[v] {
			return false
		}
	}
	return true
}

// Nav2DConfig is one complete single-task nav2d protocol: the environment,
// the training seeds and budget, the policy sizes, the learning rate, the
// shared evaluation batch and the policies to compare.
type Nav2DConfig struct {
	Env          nav2d.Config `json:"env"`
	Seeds        []uint64     `json:"seeds"`         // ≥ 1, distinct
	Episodes     int          `json:"episodes"`      // 1..10000
	Hidden       int          `json:"hidden"`        // 4..128
	Recurrent    int          `json:"recurrent"`     // 1..Hidden
	LearningRate float64      `json:"learning_rate"` // > 0
	EvalEpisodes int          `json:"eval_episodes"` // 1..1000
	Policies     []string     `json:"policies"`      // non-empty subset of recurrent, feedforward, rewired, random; no duplicates
}

// Validate checks the protocol without running anything: nav2d.New must
// accept the environment, seeds must be present and distinct, and every
// budget, size and policy name must sit inside its declared range.
func (c Nav2DConfig) Validate() error {
	if _, err := nav2d.New(c.Env); err != nil {
		return fmt.Errorf("nav2d config env: %w", err)
	}
	if len(c.Seeds) == 0 {
		return fmt.Errorf("nav2d config declares no seed")
	}
	seenSeed := map[uint64]bool{}
	for _, seed := range c.Seeds {
		if seenSeed[seed] {
			return fmt.Errorf("nav2d config declares duplicate seed %d", seed)
		}
		seenSeed[seed] = true
	}
	if c.Episodes < 1 || c.Episodes > 10000 {
		return fmt.Errorf("nav2d config episodes %d, want a value in [1, 10000]", c.Episodes)
	}
	if c.Hidden < 4 || c.Hidden > 128 {
		return fmt.Errorf("nav2d config hidden %d, want a value in [4, 128]", c.Hidden)
	}
	if c.Recurrent < 1 || c.Recurrent > c.Hidden {
		return fmt.Errorf("nav2d config recurrent %d, want a value in [1, %d]", c.Recurrent, c.Hidden)
	}
	if !finite(c.LearningRate) || c.LearningRate <= 0 {
		return fmt.Errorf("nav2d config learning rate %v must be finite and positive", c.LearningRate)
	}
	if c.EvalEpisodes < 1 || c.EvalEpisodes > 1000 {
		return fmt.Errorf("nav2d config eval episodes %d, want a value in [1, 1000]", c.EvalEpisodes)
	}
	if len(c.Policies) == 0 {
		return fmt.Errorf("nav2d config declares no policy")
	}
	known := map[string]bool{Nav2DRecurrent: true, Nav2DFeedforward: true, Nav2DRewired: true, Nav2DRandom: true}
	used := map[string]bool{}
	for _, kind := range c.Policies {
		if !known[kind] {
			return fmt.Errorf("nav2d config declares unknown policy %q", kind)
		}
		if used[kind] {
			return fmt.Errorf("nav2d config declares duplicate policy %q", kind)
		}
		used[kind] = true
	}
	return nil
}

// Nav2DRun is one policy on one seed: the budget it took, its metrics before
// training, on the seen training maps and on the unseen maps, plus the
// rewiring checklist when the policy is the rewired control. A failed run
// keeps its error and the other runs continue.
type Nav2DRun struct {
	Policy     string        `json:"policy"`
	Seed       uint64        `json:"seed"`
	Parameters int           `json:"parameters"`
	Edges      int           `json:"edges"`
	Updates    uint64        `json:"updates"`
	Before     Nav2DMetrics  `json:"before"` // untrained, on EvalEpisodes training maps nav2dTrainMap(seed, 0..EvalEpisodes-1)
	Seen       Nav2DMetrics  `json:"seen"`   // after training, the same training maps
	Unseen     Nav2DMetrics  `json:"unseen"` // after training, nav2dUnseenMap(0..EvalEpisodes-1) (shared by every policy and seed)
	Rewire     *RewireReport `json:"rewire,omitempty"`
	Failed     bool          `json:"failed"`
	Error      string        `json:"error,omitempty"`
}

// Nav2DReport is one task's full comparison: the protocol, its fingerprint
// and one run per policy and seed in policy-then-seed order.
type Nav2DReport struct {
	SchemaVersion string      `json:"schema_version"` // "coimnet-nav2d/v1"
	Task          string      `json:"task"`
	Config        Nav2DConfig `json:"config"`
	ConfigHash    string      `json:"config_hash"`
	Runs          []Nav2DRun  `json:"runs"`        // Policies order, then Seeds order
	Assumptions   []string    `json:"assumptions"` // three fixed sentences
}

// nav2dAssumptions returns the three fixed assumption sentences every
// nav2d report carries.
func nav2dAssumptions() []string {
	return []string{
		"Imitation targets come from a BFS expert that sees the whole map; the policy sees only the view cone, heading, collision flag, elapsed time, token, rule-change flag and cue. After a warm start on demonstrations, half of the training episodes are the policy's own rollouts labelled by the expert (DAgger); at evaluation the expert only scores.",
		"Seen maps are training map seeds; unseen maps are even seeds that training never draws.",
		"The rewired control keeps parameter count, update budget and every node's recurrent in/out degree; it tests whether this particular wiring matters, not the fly connectome.",
	}
}

// nav2dInputWidth returns len(nav2d.Observation.Vector) for the config: the
// environment is built, reset with seed 1 and the first observation's vector
// length is measured, so whatever nav2d actually emits decides the size.
func nav2dInputWidth(c nav2d.Config) (int, error) {
	env, err := nav2d.New(c)
	if err != nil {
		return 0, fmt.Errorf("nav2d input width: %w", err)
	}
	obs, _ := env.Reset(1)
	return len(obs.Vector()), nil
}

// RunNav2D trains each policy for each seed with trainNav2D(ctx, p, c.Env,
// seed, c.Episodes) (demonstrations plus DAgger rollouts on
// nav2dTrainMap(seed, e)) and evaluates it; a failed run keeps its error and
// the others continue; ctx cancellation aborts.
func RunNav2D(ctx context.Context, c Nav2DConfig) (Nav2DReport, error) {
	var report Nav2DReport
	if ctx == nil {
		return report, fmt.Errorf("nav2d run needs a context")
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if err := c.Validate(); err != nil {
		return report, err
	}
	inputs, err := nav2dInputWidth(c.Env)
	if err != nil {
		return report, err
	}
	c.Seeds = append([]uint64(nil), c.Seeds...)
	task := c.Env.Task
	if task == "" {
		task = nav2d.TaskAvoidObstacles
	}
	report = Nav2DReport{
		SchemaVersion: Nav2DSchemaVersion,
		Task:          task,
		Config:        c,
		ConfigHash:    hash(c),
		Assumptions:   nav2dAssumptions(),
	}
	unseen := make([]uint64, c.EvalEpisodes)
	for i := range unseen {
		unseen[i] = nav2dUnseenMap(i)
	}
	for _, kind := range c.Policies {
		for _, seed := range c.Seeds {
			if err := ctx.Err(); err != nil {
				return report, err
			}
			run := Nav2DRun{Policy: kind, Seed: seed}
			if err := nav2dRunOne(ctx, c, kind, seed, inputs, unseen, &run); err != nil {
				if cerr := ctx.Err(); cerr != nil {
					return report, cerr
				}
				run.Failed = true
				run.Error = err.Error()
			}
			report.Runs = append(report.Runs, run)
		}
	}
	return report, nil
}

// nav2dRunOne builds one fresh policy, scores it before training on the
// seen training maps, trains it, then scores it again on the seen maps and
// on the shared unseen maps. The rewired policy publishes its rewire report
// on the run.
func nav2dRunOne(ctx context.Context, c Nav2DConfig, kind string, seed uint64, inputs int, unseen []uint64, run *Nav2DRun) error {
	p, err := newNav2DPolicy(kind, seed, inputs, c.Hidden, c.Recurrent, c.LearningRate)
	if err != nil {
		return err
	}
	run.Parameters, run.Edges = p.parameters()
	seen := make([]uint64, c.EvalEpisodes)
	for i := range seen {
		seen[i] = nav2dTrainMap(seed, i)
	}
	if run.Before, err = evaluateNav2D(ctx, p, c.Env, seen); err != nil {
		return err
	}
	if run.Updates, err = trainNav2D(ctx, p, c.Env, seed, c.Episodes); err != nil {
		return err
	}
	if run.Seen, err = evaluateNav2D(ctx, p, c.Env, seen); err != nil {
		return err
	}
	if run.Unseen, err = evaluateNav2D(ctx, p, c.Env, unseen); err != nil {
		return err
	}
	if kind == Nav2DRewired {
		run.Rewire = p.rewire
	}
	return nil
}

// Nav2DSuiteReport is one shared config run over the four navigation tasks.
type Nav2DSuiteReport struct {
	SchemaVersion string        `json:"schema_version"` // "coimnet-nav2d-suite/v1"
	Tasks         []Nav2DReport `json:"tasks"`
}

// RunNav2DSuite runs RunNav2D for the four tasks with one shared config
// (Env.Task replaced per task, in the order remember_goal, avoid_obstacles,
// adapt_after_change, language_goal).
func RunNav2DSuite(ctx context.Context, c Nav2DConfig) (Nav2DSuiteReport, error) {
	var report Nav2DSuiteReport
	if ctx == nil {
		return report, fmt.Errorf("nav2d suite needs a context")
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	report.SchemaVersion = Nav2DSuiteSchemaVersion
	for _, task := range nav2dSuiteTasks {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		perTask := c
		perTask.Env.Task = task
		one, err := RunNav2D(ctx, perTask)
		if err != nil {
			return report, err
		}
		report.Tasks = append(report.Tasks, one)
	}
	return report, nil
}
