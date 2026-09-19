package experiment

import (
	"context"
	"fmt"
	"math"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/plasticity"
)

const (
	// AblationSchemaVersion identifies the MOD-10 comparison document.
	AblationSchemaVersion = "coimnet-ablation/v1"
	// The five MOD-10 control groups. The last two are the trainable
	// controller and the capacity-matched control of ablation_controllers.go.
	GroupNoModulation        = "no_modulation"
	GroupDirectReward        = "direct_reward"
	GroupFixedDecay          = "fixed_decay"
	GroupTrainableController = "trainable_controller"
	GroupCapacityMatched     = "capacity_matched"

	// ablationTrainSeed and ablationTestSeed are the fixed delayed-pulse
	// splits every group trains and evaluates on.
	ablationTrainSeed uint64 = 1001
	ablationTestSeed  uint64 = 1003
)

// AblationConfig is one comparison protocol: the seeds, the training budget,
// the evaluation budget and the groups that share them all.
type AblationConfig struct {
	Seeds        []uint64 `json:"seeds"`
	Episodes     int      `json:"episodes"`
	EvalEpisodes int      `json:"eval_episodes"`
	Groups       []string `json:"groups"`
	LearningRate float64  `json:"learning_rate"`
	// ControllerHidden and ControllerRate belong to the two controller groups
	// alone. Both are allowed to be zero here because RunAblation normalizes
	// them to the defaults 4 and 0.05, exactly like LearningRate.
	ControllerHidden int     `json:"controller_hidden"`
	ControllerRate   float64 `json:"controller_rate"`
}

// Validate checks the protocol without running anything. LearningRate is
// allowed to be zero here because RunAblation normalizes it to the default.
func (c AblationConfig) Validate() error {
	if len(c.Seeds) < 3 {
		return fmt.Errorf("ablation needs at least three seeds, got %d", len(c.Seeds))
	}
	seen := map[uint64]bool{}
	for _, seed := range c.Seeds {
		if seen[seed] {
			return fmt.Errorf("ablation declares duplicate seed %d", seed)
		}
		seen[seed] = true
	}
	if c.Episodes < 1 || c.Episodes > 100000 {
		return fmt.Errorf("ablation episodes %d, want a value in [1, 100000]", c.Episodes)
	}
	if c.EvalEpisodes < 1 || c.EvalEpisodes > 10000 {
		return fmt.Errorf("ablation eval episodes %d, want a value in [1, 10000]", c.EvalEpisodes)
	}
	if !finite(c.LearningRate) || c.LearningRate < 0 {
		return fmt.Errorf("ablation learning rate %v must be finite and non-negative", c.LearningRate)
	}
	if len(c.Groups) == 0 {
		return fmt.Errorf("ablation declares no group")
	}
	known := map[string]bool{GroupNoModulation: true, GroupDirectReward: true, GroupFixedDecay: true, GroupTrainableController: true, GroupCapacityMatched: true}
	declared := map[string]bool{}
	for _, group := range c.Groups {
		if !known[group] {
			return fmt.Errorf("ablation declares unknown group %q", group)
		}
		if declared[group] {
			return fmt.Errorf("ablation declares duplicate group %q", group)
		}
		declared[group] = true
	}
	if !declared[GroupNoModulation] {
		return fmt.Errorf("ablation needs %q as the activity reference", GroupNoModulation)
	}
	return nil
}

// GroupRun is one seed under one group: the task metric, the parameter count,
// the activity change against the same-seed no_modulation run, and the failure
// state. A failed run keeps its seed and reason, because a missing cell is a
// result too.
type GroupRun struct {
	Group         string  `json:"group"`
	Seed          uint64  `json:"seed"`
	Score         float64 `json:"score"`
	Parameters    int     `json:"parameters"`
	ActivityDelta float64 `json:"activity_delta"`
	Failed        bool    `json:"failed"`
	Error         string  `json:"error,omitempty"`
}

// GroupSummary aggregates one group across its seeds.
type GroupSummary struct {
	Group      string     `json:"group"`
	Runs       []GroupRun `json:"runs"`
	Mean       float64    `json:"mean"`
	Std        float64    `json:"std"`
	Succeeded  int        `json:"succeeded"`
	Total      int        `json:"total"`
	Parameters int        `json:"parameters"`
}

// AblationReport separates the shared protocol from the per-group runs.
type AblationReport struct {
	SchemaVersion string         `json:"schema_version"`
	Profile       string         `json:"profile"`
	Config        AblationConfig `json:"config"`
	ConfigHash    string         `json:"config_hash"`
	DataHash      string         `json:"data_hash"`
	TrainSeed     uint64         `json:"train_seed"`
	TestSeed      uint64         `json:"test_seed"`
	Groups        []GroupSummary `json:"groups"`
	Assumptions   []string       `json:"assumptions"`
}

// RunAblation runs every group and seed on the same split, budget and metric.
// A canceled context aborts immediately; a failed seed is recorded in its
// group and the other seeds and groups continue.
func RunAblation(ctx context.Context, c AblationConfig) (AblationReport, error) {
	var report AblationReport
	if ctx == nil {
		return report, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if err := c.Validate(); err != nil {
		return report, err
	}
	if c.LearningRate == 0 {
		c.LearningRate = 0.02
	}
	if c.ControllerHidden == 0 {
		c.ControllerHidden = 4
	}
	if c.ControllerRate == 0 {
		c.ControllerRate = 0.05
	}
	c.Seeds = append([]uint64(nil), c.Seeds...)
	c.Groups = append([]string(nil), c.Groups...)

	training := make([]Episode, 0, c.Episodes)
	for e := 0; e < c.Episodes; e++ {
		training = append(training, DelayedEpisode(ablationTrainSeed, uint64(e)))
	}
	holdout := make([]Episode, 0, c.EvalEpisodes)
	for i := 0; i < c.EvalEpisodes; i++ {
		holdout = append(holdout, DelayedEpisode(ablationTestSeed, uint64(i)))
	}
	data := append(append([]Episode(nil), training...), holdout...)
	report = AblationReport{SchemaVersion: AblationSchemaVersion, Profile: "fixture", Config: c, ConfigHash: hash(c), DataHash: hash(data), TrainSeed: ablationTrainSeed, TestSeed: ablationTestSeed, Assumptions: ablationAssumptions()}

	predsByGroup := make([][][]float64, len(c.Groups))
	noModulation := -1
	for gi, group := range c.Groups {
		summary := GroupSummary{Group: group, Total: len(c.Seeds)}
		for _, seed := range c.Seeds {
			var run GroupRun
			var preds []float64
			var err error
			switch group {
			case GroupTrainableController:
				run, preds, err = runTrainableController(ctx, c, seed, training, holdout)
			case GroupCapacityMatched:
				run, preds, err = runCapacityMatched(ctx, c, seed, training, holdout)
			default:
				run, preds, err = runAblationSeed(ctx, seed, group, c)
			}
			if err != nil {
				return report, err
			}
			predsByGroup[gi] = append(predsByGroup[gi], preds)
			summary.Runs = append(summary.Runs, run)
		}
		if len(summary.Runs) > 0 {
			summary.Parameters = summary.Runs[0].Parameters
		}
		summarizeAblation(&summary)
		if group == GroupNoModulation {
			noModulation = gi
		}
		report.Groups = append(report.Groups, summary)
	}
	for gi, group := range c.Groups {
		if group == GroupNoModulation || noModulation < 0 {
			continue
		}
		for i := range report.Groups[gi].Runs {
			a, b := predsByGroup[noModulation][i], predsByGroup[gi][i]
			if a == nil || b == nil {
				continue
			}
			report.Groups[gi].Runs[i].ActivityDelta = l2(a, b)
		}
	}
	return report, nil
}

// runAblationSeed runs one group and seed and reports it. A non-context error
// becomes a failed run so the rest of the protocol continues; only a canceled
// context returns an error.
func runAblationSeed(ctx context.Context, seed uint64, group string, c AblationConfig) (GroupRun, []float64, error) {
	var run GroupRun
	run.Group, run.Seed = group, seed
	ind, count, err := ablationIndividual(seed, group, c)
	run.Parameters = count
	if err != nil {
		if ctx.Err() != nil {
			return run, nil, ctx.Err()
		}
		run.Failed, run.Error = true, err.Error()
		return run, nil, nil
	}
	if err := trainAblationIndividual(ctx, ind, group, c); err != nil {
		if ctx.Err() != nil {
			return run, nil, ctx.Err()
		}
		run.Failed, run.Error = true, err.Error()
		return run, nil, nil
	}
	score, preds, err := scoreAblationIndividual(ctx, ind, c)
	if err != nil {
		if ctx.Err() != nil {
			return run, nil, ctx.Err()
		}
		run.Failed, run.Error = true, err.Error()
		return run, nil, nil
	}
	run.Score = score
	return run, preds, nil
}

// ablationIndividual builds the three-neuron delayed-pulse chain as a
// persistent individual with hebbian_rate local plasticity on its 1->1 self
// edge. Edge 0 starts at .15 + (mix(seed)%100)/1000, exactly like the delayed
// runner, so the reference delayed behavior is reproducible here. Only the
// weights train; the encoder, the readout and the time constants stay fixed.
func ablationIndividual(seed uint64, group string, c AblationConfig) (*learning.Individual, int, error) {
	config := learning.Config{
		Dynamics: dynamics.Config{
			Nodes: 3, Sources: []int{0, 1, 1}, Targets: []int{1, 1, 2},
			DT: 1, Activation: "tanh",
		},
		InputSize: 1, OutputSize: 1, ReadoutNodes: []int{2},
	}
	params := learning.Parameters{
		Core: dynamics.Parameters{
			Weights: []float64{.15 + float64(mix(seed)%100)/1000, .2, .2},
			Bias:    []float64{0, 0, 0},
			LogTau:  []float64{math.Log(2), math.Log(2), math.Log(2)},
		},
		Encoder: []float64{1, 0, 0},
		Readout: []float64{1},
	}
	count := ablationParameterCount(params)
	o := learning.DefaultOptions()
	o.LearningRate = c.LearningRate
	o.Trainable = learning.Trainable{Weights: true}
	ind, err := learning.NewIndividual(config, params, o, make([]float64, 3))
	if err != nil {
		return nil, count, err
	}
	if group == GroupFixedDecay {
		if err := ind.EnableChemistry(ablationChemistry(c.Episodes)); err != nil {
			return nil, count, err
		}
	}
	rule := plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 1, WMin: .01}
	if group == GroupFixedDecay {
		receptor := 0
		rule.GateReceptor, rule.GateScale = &receptor, 1
	}
	if err := ind.EnablePlasticity(plasticity.Config{Rule: rule, Edges: []int{1}}); err != nil {
		return nil, count, err
	}
	return ind, count, nil
}

// trainAblationIndividual replays every training episode on the persistent
// individual: the walk over the input first, then the isolated gradient update
// of the same episode. no_modulation and fixed_decay walk with the unmodulated
// advance; direct_reward walks with every row gated by the previous episode's
// reward, which is zero before the first episode has produced an output.
func trainAblationIndividual(ctx context.Context, ind *learning.Individual, group string, c AblationConfig) error {
	reward := 0.0
	for e := 0; e < c.Episodes; e++ {
		ep := DelayedEpisode(ablationTrainSeed, uint64(e))
		if group == GroupDirectReward {
			gate := make([]float64, len(ep.Input))
			for t := range gate {
				gate[t] = reward
			}
			out, _, err := ind.AdvanceGated(ctx, ep.Input, gate)
			if err != nil {
				return err
			}
			d := out[len(out)-1][0] - ep.Target[0]
			reward = math.Max(0, 1-d*d)
		} else if _, err := ind.Advance(ctx, ep.Input); err != nil {
			return err
		}
		if _, err := ind.TrainEpisode(ctx, ep.Input, ep.Target); err != nil {
			return err
		}
	}
	return nil
}

// scoreAblationIndividual restores a twin of the trained individual and
// evaluates it on the held-out split: a fresh zero-voltage episode per holdout
// sample, a zero-gate walk, and the negative mean squared error of the last
// output row. The returned predictions are the per-sample readouts in order,
// which the activity delta against no_modulation is computed from.
func scoreAblationIndividual(ctx context.Context, ind *learning.Individual, c AblationConfig) (float64, []float64, error) {
	twin, err := learning.RestoreIndividual(ind.Snapshot())
	if err != nil {
		return 0, nil, err
	}
	preds := make([]float64, c.EvalEpisodes)
	var sumSq float64
	for i := 0; i < c.EvalEpisodes; i++ {
		ep := DelayedEpisode(ablationTestSeed, uint64(i))
		if err := twin.ResetNeural(ctx, make([]float64, 3)); err != nil {
			return 0, nil, err
		}
		out, err := twin.Advance(ctx, ep.Input)
		if err != nil {
			return 0, nil, err
		}
		pred := out[len(out)-1][0]
		preds[i] = pred
		d := pred - ep.Target[0]
		sumSq += d * d
	}
	return -sumSq / float64(c.EvalEpisodes), preds, nil
}

// ablationChemistry is the fixed_decay declaration: one region holding all
// three nodes, one channel cleared with a fixed time constant, an external
// timeline releasing one unit at the first step (global step 5*e) of every
// training episode, a hypothesized receptor on neuron 1 and one sensitivity
// effect driven by it. The gate scale turns the occupancy into the local
// learning gate.
func ablationChemistry(episodes int) modulation.ChemistryConfig {
	entries := make([]modulation.TimelineEntry, episodes)
	for e := range entries {
		entries[e] = modulation.TimelineEntry{Step: uint64(5 * e), Channel: 0, Rate: 1}
	}
	return modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{2}},
		Sources: []modulation.SourceSpec{{
			Kind: modulation.SourceExternalTimeline, Channel: 0,
			Timeline: &modulation.ExternalTimeline{ChannelCount: 1, Entries: entries},
		}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{{
			Cells: []int{1}, Signal: "octopamine", Channel: 0,
			Status: modulation.StatusHypothesized, Kd: .5, N: 1,
			Evidence: "ablation fixed_decay", MeasurementKind: "declared", MappingVersion: "coimnet-ablation/v1",
		}}},
		Effects: []modulation.Effect{{Kind: modulation.EffectSensitivity, Receptor: 0, GammaScale: 1}},
		Regions: modulation.RegionAssignment{NodeRegion: []int{0, 0, 0}},
	}
}

func ablationParameterCount(p learning.Parameters) int {
	return len(p.Core.Weights) + len(p.Core.Bias) + len(p.Core.LogTau) + len(p.Encoder) + len(p.Readout)
}

// summarizeAblation fills the mean and standard deviation over the succeeded
// runs only. A group with no succeeded run keeps both at zero, because no
// number would describe it.
func summarizeAblation(summary *GroupSummary) {
	var sum float64
	for _, run := range summary.Runs {
		if run.Failed {
			continue
		}
		summary.Succeeded++
		sum += run.Score
	}
	if summary.Succeeded == 0 {
		return
	}
	summary.Mean = sum / float64(summary.Succeeded)
	var sq float64
	for _, run := range summary.Runs {
		if run.Failed {
			continue
		}
		d := run.Score - summary.Mean
		sq += d * d
	}
	summary.Std = math.Sqrt(sq / float64(summary.Succeeded))
}

func l2(a, b []float64) float64 {
	var d float64
	for i := range a {
		d = math.Hypot(d, a[i]-b[i])
	}
	return d
}

func ablationAssumptions() []string {
	return []string{
		"Every group trains on the same training split, is scored on the same held-out split with the same seeds, budget and metric; the report records the numbers and makes no verbal judgement.",
		"direct_reward feeds the previous episode's reward (max(0, 1 - MSE) of its last output) straight in as the local learning gate, bypassing any concentration or receptor: it is the control for merely renaming a reward as a hormone.",
		"fixed_decay releases one unit into its single channel at the first step of every training episode from a fixed external timeline, so the concentration is set by the declaration and never by learning; the hypothesized receptor's occupancy is the learning gate.",
		"trainable_controller only sees an activity summary and already-arrived feedback, never a target; its release enters the chemistry through an internal resource.",
		"capacity_matched has the controller's parameter count and adds its output to the readout without any chemistry; its base parameters keep training against the unchanged loss, so the offset never enters their gradient.",
	}
}
