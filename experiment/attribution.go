package experiment

import (
	"context"
	"fmt"
	"math"
	"reflect"

	"github.com/TimLai666/coimnet/experiment/nav2d"
	"github.com/TimLai666/coimnet/learning"
)

// AttributionConclusion is the only conclusion sentence the report may carry
// (master plan 14.4): a drop after freezing or removing the core shows
// dependence on it, not that the original wiring is superior.
const AttributionConclusion = "A drop after freezing or removing the core shows dependence on it, not that the original wiring is superior."

// AttributionConfig is one complete seven-group attribution protocol: the
// environment, the shared seeds and schedule, the policy sizes, the learning
// rate, the shared evaluation batch, the groups to run and the pre-registered
// comparison.
type AttributionConfig struct {
	Env          nav2d.Config  `json:"env"`           // Task must be set
	Seeds        []uint64      `json:"seeds"`         // >= 3, distinct
	Episodes     int           `json:"episodes"`      // 1..10000, the same schedule for every group
	Hidden       int           `json:"hidden"`        // 4..128
	Recurrent    int           `json:"recurrent"`     // 1..Hidden
	LearningRate float64       `json:"learning_rate"` // > 0
	EvalEpisodes int           `json:"eval_episodes"` // 1..1000
	Groups       []string      `json:"groups"`        // subset of AttributionGroups(), no duplicates, must contain normal
	Comparison   PreRegistered `json:"comparison"`    // Method paired_bootstrap, Baseline normal, Interval in (0,1), Resamples >= 100
}

// Validate checks the protocol without running anything: the environment task
// must be declared and accepted by nav2d.New, seeds must be at least three
// and distinct, every budget and size must sit inside its declared range, the
// group list must be a duplicate-free subset of AttributionGroups() that
// contains normal, and the comparison must be the pre-registered paired
// bootstrap against the normal baseline.
func (c AttributionConfig) Validate() error {
	if c.Env.Task == "" {
		return fmt.Errorf("attribution config env: task must be set")
	}
	if _, err := nav2d.New(c.Env); err != nil {
		return fmt.Errorf("attribution config env: %w", err)
	}
	if len(c.Seeds) < 3 {
		return fmt.Errorf("attribution config declares %d seeds, want at least 3", len(c.Seeds))
	}
	seenSeed := map[uint64]bool{}
	for _, seed := range c.Seeds {
		if seenSeed[seed] {
			return fmt.Errorf("attribution config declares duplicate seed %d", seed)
		}
		seenSeed[seed] = true
	}
	if c.Episodes < 1 || c.Episodes > 10000 {
		return fmt.Errorf("attribution config episodes %d, want a value in [1, 10000]", c.Episodes)
	}
	if c.Hidden < 4 || c.Hidden > 128 {
		return fmt.Errorf("attribution config hidden %d, want a value in [4, 128]", c.Hidden)
	}
	if c.Recurrent < 1 || c.Recurrent > c.Hidden {
		return fmt.Errorf("attribution config recurrent %d, want a value in [1, %d]", c.Recurrent, c.Hidden)
	}
	if !finite(c.LearningRate) || c.LearningRate <= 0 {
		return fmt.Errorf("attribution config learning rate %v must be finite and positive", c.LearningRate)
	}
	if c.EvalEpisodes < 1 || c.EvalEpisodes > 1000 {
		return fmt.Errorf("attribution config eval episodes %d, want a value in [1, 1000]", c.EvalEpisodes)
	}
	known := map[string]bool{}
	for _, group := range AttributionGroups() {
		known[group] = true
	}
	used := map[string]bool{}
	normal := false
	for _, group := range c.Groups {
		if !known[group] {
			return fmt.Errorf("attribution config declares unknown group %q", group)
		}
		if used[group] {
			return fmt.Errorf("attribution config declares duplicate group %q", group)
		}
		used[group] = true
		if group == AttributionNormal {
			normal = true
		}
	}
	if !normal {
		return fmt.Errorf("attribution config declares no %q group", AttributionNormal)
	}
	if c.Comparison.Method != ComparisonPairedBootstrap {
		return fmt.Errorf("attribution comparison: unsupported method %q", c.Comparison.Method)
	}
	if c.Comparison.Baseline != AttributionNormal {
		return fmt.Errorf("attribution comparison: unsupported baseline %q", c.Comparison.Baseline)
	}
	if c.Comparison.Interval <= 0 || c.Comparison.Interval >= 1 {
		return fmt.Errorf("attribution comparison: interval must be strictly between 0 and 1, got %g", c.Comparison.Interval)
	}
	if c.Comparison.Resamples < 100 {
		return fmt.Errorf("attribution comparison: resamples %d, want at least 100", c.Comparison.Resamples)
	}
	return nil
}

// DefaultAttributionConfig returns the declared default protocol: the
// avoid_obstacles task, seeds 1, 2 and 3, 60 training episodes, a 16-node
// hidden block with 4 recurrent sources, learning rate 0.05, 20 evaluation
// maps, all seven groups and a 1000-resample 95% paired bootstrap against
// normal.
func DefaultAttributionConfig() AttributionConfig {
	return AttributionConfig{
		Env:          nav2d.Config{Task: nav2d.TaskAvoidObstacles},
		Seeds:        []uint64{1, 2, 3},
		Episodes:     60,
		Hidden:       16,
		Recurrent:    4,
		LearningRate: 0.05,
		EvalEpisodes: 20,
		Groups:       AttributionGroups(),
		Comparison: PreRegistered{
			Method:    ComparisonPairedBootstrap,
			Interval:  0.95,
			Baseline:  AttributionNormal,
			Resamples: 1000,
		},
	}
}

// CoreOnlyChecks is the troubleshooting list attached to core_only when it
// did not learn (Seen.ExpertAgreement <= Before.ExpertAgreement). It never
// changes the model; the periphery is never enlarged.
type CoreOnlyChecks struct {
	InputReachable  bool    `json:"input_reachable"`  // every readout node reachable from the input nodes over the core edges (BFS)
	CoreUpdateNorm  float64 `json:"core_update_norm"` // L2 norm of (weights, bias) after - before training: did any gradient reach the core
	MeanAbsInput    float64 `json:"mean_abs_input"`   // mean |value| of the probe observation rows
	MeanAbsReadout  float64 `json:"mean_abs_readout"` // mean |tanh(v)| of the action readout nodes (the last nav2d.Actions of ReadoutNodes) over the probe (same probe and voltages as the activity)
	Saturated       bool    `json:"saturated"`        // MeanAbsReadout > 0.99
	Vanishing       bool    `json:"vanishing"`        // MeanAbsReadout < 1e-6
	StateReset      bool    `json:"state_reset"`      // two PredictAll calls on the probe are bit-identical
	ObservationOnly bool    `json:"observation_only"` // act's parameter types are exactly (*nav2dPolicy, context.Context, [][]float64)
}

// AttributionRun is one group on one seed: the model it built, the schedule it
// took, its metrics before training, on the seen training maps and on the
// unseen maps, the probe activity around training, the core_only
// troubleshooting list when that group did not learn, and the failure slot a
// broken run keeps while the rest continue.
type AttributionRun struct {
	Group          string           `json:"group"`
	Seed           uint64           `json:"seed"`
	Model          AttributionModel `json:"model"`
	Updates        uint64           `json:"updates"`
	Before         Nav2DMetrics     `json:"before"`          // untrained, on nav2dTrainMap(seed, 0..EvalEpisodes-1)
	Seen           Nav2DMetrics     `json:"seen"`            // trained, same maps
	Unseen         Nav2DMetrics     `json:"unseen"`          // trained, nav2dUnseenMap(0..EvalEpisodes-1)
	ActivityBefore float64          `json:"activity_before"` // mean |output| over the non-input nodes on the probe (all nodes for ablated_retrained)
	ActivityAfter  float64          `json:"activity_after"`
	Checks         *CoreOnlyChecks  `json:"checks,omitempty"`
	Failed         bool             `json:"failed"`
	Error          string           `json:"error,omitempty"`
}

// GroupComparison is one pre-registered comparison of a group against the
// baseline on one metric ("unseen_success_rate" or
// "unseen_expert_agreement"): Differences[i] = group - baseline for Seeds[i]
// (paired by seed), and Mean, Lower, Upper from
// pairedBootstrap(Differences, Interval, Resamples, Seeds[0]). A group or
// baseline run that failed for a seed drops that seed from both and is
// counted in Dropped.
type GroupComparison struct {
	Group       string    `json:"group"`
	Metric      string    `json:"metric"`
	Differences []float64 `json:"differences"`
	Mean        float64   `json:"mean"`
	Lower       float64   `json:"lower"`
	Upper       float64   `json:"upper"`
	Dropped     int       `json:"dropped"`
}

// AttributionReport is one complete seven-group matrix: the protocol, its
// fingerprint, one run per group and seed in group-then-seed order, every
// pre-registered comparison, the single fixed conclusion sentence and the
// four fixed assumption sentences.
type AttributionReport struct {
	SchemaVersion string            `json:"schema_version"` // "coimnet-attribution/v1"
	Config        AttributionConfig `json:"config"`
	ConfigHash    string            `json:"config_hash"` // hashed like RunNav2D
	Runs          []AttributionRun  `json:"runs"`        // Groups order, then Seeds order
	Comparisons   []GroupComparison `json:"comparisons"` // every non-baseline group in Groups order x the two metrics in the order above
	Conclusion    string            `json:"conclusion"`  // AttributionConclusion
	Assumptions   []string          `json:"assumptions"` // four fixed English sentences
}

// attributionAssumptions returns the four fixed assumption sentences every
// attribution report carries.
func attributionAssumptions() []string {
	return []string{
		"The core is the navigation policy's random sparse recurrent block, not the fly connectome; the matrix shows that every control runs under one schedule, not which wiring is better.",
		"Every group trains with trainNav2D on the same maps for the same number of episodes; update counts are reported per run.",
		"ablated_retrained has no core and capacity_matched_modulator adds a readout-side block sized to the MOD-10 controller; both parameter counts are reported, not matched.",
		"core_only keeps a fixed identity periphery; when it does not learn, the report lists the checks instead of enlarging the periphery.",
	}
}

// attributionProbe is the shared activity probe: the expert episode
// observations of the first unseen map, every row repeated three times, so
// before/after activity and the core_only checks all read one fixed input
// sequence.
func attributionProbe(env nav2d.Config) ([][]float64, error) {
	obs, _, err := nav2dExpertEpisode(env, nav2dUnseenMap(0))
	if err != nil {
		return nil, fmt.Errorf("attribution probe: %w", err)
	}
	if len(obs) == 0 {
		return nil, fmt.Errorf("attribution probe: expert episode on unseen map 0 produced no observations")
	}
	return nav2dRepeat(obs, 3), nil
}

// attributionActivityNodes lists the nodes the activity metric averages over:
// every node that is not an input node, falling back to all nodes when the
// inputs cover the whole model (ablated_retrained, whose action nodes are
// both inputs and readouts).
func attributionActivityNodes(cfg learning.Config) []int {
	isInput := make(map[int]bool, len(cfg.InputNodes))
	for _, n := range cfg.InputNodes {
		isInput[n] = true
	}
	var nodes []int
	for n := 0; n < cfg.Dynamics.Nodes; n++ {
		if !isInput[n] {
			nodes = append(nodes, n)
		}
	}
	if len(nodes) == 0 {
		for n := 0; n < cfg.Dynamics.Nodes; n++ {
			nodes = append(nodes, n)
		}
	}
	return nodes
}

// attributionActivity builds a fresh individual from the trainer config, the
// given parameters and options at zero voltage, advances it one probe row per
// Advance call (Advance returns readouts, so the metric reads voltages from
// the snapshot), and returns the mean |tanh(voltage)| over nodes across every
// probe row.
func attributionActivity(ctx context.Context, cfg learning.Config, params learning.Parameters, opts learning.Options, probe [][]float64, nodes []int) (float64, error) {
	if len(probe) == 0 || len(nodes) == 0 {
		return 0, fmt.Errorf("attribution activity needs a non-empty probe and node list")
	}
	zero := make([]float64, cfg.Dynamics.Nodes)
	ind, err := learning.NewIndividual(cfg, params, opts, zero)
	if err != nil {
		return 0, fmt.Errorf("attribution activity individual: %w", err)
	}
	var sum float64
	for t, row := range probe {
		if _, err := ind.Advance(ctx, [][]float64{row}); err != nil {
			return 0, fmt.Errorf("attribution activity row %d: %w", t, err)
		}
		voltage := ind.Snapshot().Neural.Continuous.Voltage
		if len(voltage) != cfg.Dynamics.Nodes {
			return 0, fmt.Errorf("attribution activity: voltage has %d values, want %d", len(voltage), cfg.Dynamics.Nodes)
		}
		for _, n := range nodes {
			sum += math.Abs(math.Tanh(voltage[n]))
		}
	}
	return sum / float64(len(probe)*len(nodes)), nil
}

// attributionInputReachable breadth-first searches the trainer's core edges
// from its input nodes and reports whether every readout node was reached.
// An empty edge list leaves the readout nodes unseen and reports false.
func attributionInputReachable(cfg learning.Config) bool {
	nodes := cfg.Dynamics.Nodes
	if nodes <= 0 {
		return false
	}
	sources, targets := cfg.Dynamics.Sources, cfg.Dynamics.Targets
	if len(sources) != len(targets) {
		return false
	}
	adj := make([][]int, nodes)
	for i := range sources {
		s, tgt := sources[i], targets[i]
		if s < 0 || s >= nodes || tgt < 0 || tgt >= nodes {
			return false
		}
		adj[s] = append(adj[s], tgt)
	}
	seen := make([]bool, nodes)
	queue := make([]int, 0, nodes)
	for _, v := range cfg.InputNodes {
		if v < 0 || v >= nodes {
			return false
		}
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
	for _, v := range cfg.ReadoutNodes {
		if v < 0 || v >= nodes || !seen[v] {
			return false
		}
	}
	return true
}

// attributionCoreUpdateNorm is the L2 norm of (after - before) over the core
// weights and bias: zero means no gradient reached the core.
func attributionCoreUpdateNorm(before, after learning.Parameters) float64 {
	var sum float64
	diff := func(oldV, newV []float64) {
		n := len(oldV)
		if len(newV) < n {
			n = len(newV)
		}
		for i := 0; i < n; i++ {
			d := newV[i] - oldV[i]
			sum += d * d
		}
	}
	diff(before.Core.Weights, after.Core.Weights)
	diff(before.Core.Bias, after.Core.Bias)
	return math.Sqrt(sum)
}

// attributionObservationOnly reports whether act's method expression has
// exactly the three parameter types (*nav2dPolicy, context.Context,
// [][]float64): the policy can only ever receive the observation prefix.
func attributionObservationOnly() bool {
	m := reflect.TypeOf((*nav2dPolicy).act)
	return m.NumIn() == 3 &&
		m.In(0) == reflect.TypeOf((*nav2dPolicy)(nil)) &&
		m.In(1) == reflect.TypeOf((*context.Context)(nil)).Elem() &&
		m.In(2) == reflect.TypeOf([][]float64(nil))
}

// coreOnlyChecks measures the five troubleshooting checks of an unlearned
// core_only run without touching the model: input reachability over the core
// edges, the core parameter displacement between before and after training,
// the probe's input and readout signal scales, bit-identical repeated
// inference and the act parameter-type test. The probe is the shared unseen-
// map expert sequence, rebuilt from env.
func coreOnlyChecks(ctx context.Context, p *nav2dPolicy, env nav2d.Config, before, after learning.Parameters) (CoreOnlyChecks, error) {
	var checks CoreOnlyChecks
	if p == nil || p.trainer == nil {
		return checks, fmt.Errorf("core_only checks need a policy with a trainer")
	}
	snap := p.trainer.Snapshot()
	cfg := snap.Config
	checks.InputReachable = attributionInputReachable(cfg)
	checks.CoreUpdateNorm = attributionCoreUpdateNorm(before, after)
	checks.ObservationOnly = attributionObservationOnly()
	probe, err := attributionProbe(env)
	if err != nil {
		return checks, err
	}
	var inputSum float64
	var inputCount int
	for _, row := range probe {
		for _, v := range row {
			inputSum += math.Abs(v)
			inputCount++
		}
	}
	if inputCount > 0 {
		checks.MeanAbsInput = inputSum / float64(inputCount)
	}
	actionReadouts := cfg.ReadoutNodes
	if len(actionReadouts) >= nav2d.Actions {
		actionReadouts = actionReadouts[len(actionReadouts)-nav2d.Actions:]
	}
	if checks.MeanAbsReadout, err = attributionActivity(ctx, cfg, after, snap.Options, probe, actionReadouts); err != nil {
		return checks, fmt.Errorf("core_only checks readout scale: %w", err)
	}
	checks.Saturated = checks.MeanAbsReadout > 0.99
	checks.Vanishing = checks.MeanAbsReadout < 1e-6
	first, err := p.trainer.PredictAll(ctx, probe)
	if err != nil {
		return checks, fmt.Errorf("core_only checks first pass: %w", err)
	}
	second, err := p.trainer.PredictAll(ctx, probe)
	if err != nil {
		return checks, fmt.Errorf("core_only checks second pass: %w", err)
	}
	checks.StateReset = reflect.DeepEqual(first, second)
	return checks, nil
}

// attributionRunOne builds one fresh group policy for one seed, scores it
// before training on the seen maps and on the probe, trains it with
// trainNav2D, scores it again on the seen and unseen maps, measures the
// after-training probe activity, and attaches the troubleshooting checks to
// core_only when its expert agreement did not rise above the untrained run.
func attributionRunOne(ctx context.Context, c AttributionConfig, group string, seed uint64, inputs int, unseen []uint64, probe [][]float64, run *AttributionRun) error {
	p, model, err := newAttributionPolicy(group, seed, inputs, c.Hidden, c.Recurrent, c.LearningRate)
	if err != nil {
		return err
	}
	run.Model = model
	snap := p.trainer.Snapshot()
	beforeParams, cfg, opts := snap.Parameters, snap.Config, snap.Options
	actNodes := attributionActivityNodes(cfg)
	seen := make([]uint64, c.EvalEpisodes)
	for i := range seen {
		seen[i] = nav2dTrainMap(seed, i)
	}
	if run.Before, err = evaluateNav2D(ctx, p, c.Env, seen); err != nil {
		return err
	}
	if run.ActivityBefore, err = attributionActivity(ctx, cfg, beforeParams, opts, probe, actNodes); err != nil {
		return err
	}
	if run.Updates, err = trainNav2D(ctx, p, c.Env, seed, c.Episodes); err != nil {
		return err
	}
	afterParams := p.trainer.Snapshot().Parameters
	if run.Seen, err = evaluateNav2D(ctx, p, c.Env, seen); err != nil {
		return err
	}
	if run.Unseen, err = evaluateNav2D(ctx, p, c.Env, unseen); err != nil {
		return err
	}
	if run.ActivityAfter, err = attributionActivity(ctx, cfg, afterParams, opts, probe, actNodes); err != nil {
		return err
	}
	if group == AttributionCoreOnly && run.Seen.ExpertAgreement <= run.Before.ExpertAgreement {
		checks, err := coreOnlyChecks(ctx, p, c.Env, beforeParams, afterParams)
		if err != nil {
			return err
		}
		run.Checks = &checks
	}
	return nil
}

// attributionComparisons computes every pre-registered group-vs-baseline
// comparison on the two unseen metrics: paired by seed, failed runs dropped
// from both sides and counted, then one paired bootstrap per surviving
// difference list seeded with the first seed.
func attributionComparisons(report *AttributionReport, c AttributionConfig) {
	type key struct {
		group string
		seed  uint64
	}
	index := make(map[key]int, len(report.Runs))
	for i, run := range report.Runs {
		index[key{run.Group, run.Seed}] = i
	}
	metrics := []struct {
		name string
		get  func(Nav2DMetrics) float64
	}{
		{"unseen_success_rate", func(m Nav2DMetrics) float64 { return m.SuccessRate }},
		{"unseen_expert_agreement", func(m Nav2DMetrics) float64 { return m.ExpertAgreement }},
	}
	for _, group := range c.Groups {
		if group == c.Comparison.Baseline {
			continue
		}
		for _, metric := range metrics {
			gc := GroupComparison{Group: group, Metric: metric.name}
			for _, seed := range c.Seeds {
				gi, gok := index[key{group, seed}]
				bi, bok := index[key{c.Comparison.Baseline, seed}]
				if !gok || !bok {
					gc.Dropped++
					continue
				}
				g, b := report.Runs[gi], report.Runs[bi]
				if g.Failed || b.Failed {
					gc.Dropped++
					continue
				}
				gc.Differences = append(gc.Differences, metric.get(g.Unseen)-metric.get(b.Unseen))
			}
			if len(gc.Differences) > 0 {
				gc.Mean, gc.Lower, gc.Upper = pairedBootstrap(gc.Differences, c.Comparison.Interval, c.Comparison.Resamples, c.Seeds[0])
			}
			report.Comparisons = append(report.Comparisons, gc)
		}
	}
}

// RunAttribution builds each group for each seed with newAttributionPolicy,
// evaluates Before, trains with trainNav2D, evaluates Seen and Unseen,
// measures the probe activity before and after, attaches CoreOnlyChecks to
// core_only when it did not learn, then computes the pre-registered
// comparisons. A failing run keeps its error and the rest continue; ctx
// cancellation aborts with the error.
func RunAttribution(ctx context.Context, c AttributionConfig) (AttributionReport, error) {
	var report AttributionReport
	if ctx == nil {
		return report, fmt.Errorf("attribution run needs a context")
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
	c.Groups = append([]string(nil), c.Groups...)
	report = AttributionReport{
		SchemaVersion: "coimnet-attribution/v1",
		Config:        c,
		ConfigHash:    hash(c),
		Conclusion:    AttributionConclusion,
		Assumptions:   attributionAssumptions(),
	}
	probe, err := attributionProbe(c.Env)
	if err != nil {
		return report, err
	}
	unseen := make([]uint64, c.EvalEpisodes)
	for i := range unseen {
		unseen[i] = nav2dUnseenMap(i)
	}
	for _, group := range c.Groups {
		for _, seed := range c.Seeds {
			if err := ctx.Err(); err != nil {
				return report, err
			}
			run := AttributionRun{Group: group, Seed: seed}
			if err := attributionRunOne(ctx, c, group, seed, inputs, unseen, probe, &run); err != nil {
				if cerr := ctx.Err(); cerr != nil {
					return report, cerr
				}
				run.Failed = true
				run.Error = err.Error()
			}
			report.Runs = append(report.Runs, run)
		}
	}
	attributionComparisons(&report, c)
	return report, nil
}
