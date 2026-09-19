package experiment

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/experiment/gridnav"
	"github.com/TimLai666/coimnet/learning"
)

// ImitationSchemaVersion identifies the LRN-09 expert-imitation document.
const ImitationSchemaVersion = "coimnet-imitation/v1"

const (
	// imitationInputs is the width of gridnav.Observation.Vector and
	// imitationOutputs the corridor's action count. One model therefore owns
	// imitationInputs + Hidden + imitationOutputs nodes.
	imitationInputs  = 4
	imitationOutputs = gridnav.Actions
	// imitationEvalBase is the seed of the first evaluation episode. The
	// evaluation batch does not depend on the run seed, so every seed and the
	// random-policy baseline are scored on the same episodes.
	imitationEvalBase uint64 = 1000
	// imitationMaxHidden bounds the hidden layer, whose recurrent block costs
	// Hidden squared edges.
	imitationMaxHidden = 512
)

// ImitationConfig is one supervised expert-imitation protocol: the corridor,
// the seeds, the per-seed training budget, the recurrent model size, the
// learning rate and the shared evaluation batch.
type ImitationConfig struct {
	Corridor     gridnav.Config `json:"corridor"`
	Seeds        []uint64       `json:"seeds"`
	Episodes     int            `json:"episodes"`
	Hidden       int            `json:"hidden"`
	LearningRate float64        `json:"learning_rate"`
	EvalEpisodes int            `json:"eval_episodes"`
}

// Validate checks the protocol without running anything.
func (c ImitationConfig) Validate() error {
	if _, err := gridnav.New(c.Corridor); err != nil {
		return fmt.Errorf("imitation corridor: %w", err)
	}
	if len(c.Seeds) == 0 {
		return fmt.Errorf("imitation declares no seed")
	}
	seen := map[uint64]bool{}
	for _, seed := range c.Seeds {
		if seen[seed] {
			return fmt.Errorf("imitation declares duplicate seed %d", seed)
		}
		seen[seed] = true
	}
	if c.Episodes < 1 || c.Episodes > 100000 {
		return fmt.Errorf("imitation episodes %d, want a value in [1, 100000]", c.Episodes)
	}
	if c.Hidden < 4 || c.Hidden > imitationMaxHidden {
		return fmt.Errorf("imitation hidden %d, want a value in [4, %d]", c.Hidden, imitationMaxHidden)
	}
	if !finite(c.LearningRate) || c.LearningRate <= 0 {
		return fmt.Errorf("imitation learning rate %v must be finite and positive", c.LearningRate)
	}
	if c.EvalEpisodes < 1 || c.EvalEpisodes > 10000 {
		return fmt.Errorf("imitation eval episodes %d, want a value in [1, 10000]", c.EvalEpisodes)
	}
	return nil
}

// ImitationSeedResult is one seed's before-and-after comparison on the shared
// evaluation batch. A failed seed keeps its seed and reason.
type ImitationSeedResult struct {
	Seed            uint64  `json:"seed"`
	ExpertAgreement float64 `json:"expert_agreement"`
	MeanReturn      float64 `json:"mean_return"`
	AgreementBefore float64 `json:"agreement_before"`
	Updates         uint64  `json:"updates"`
	Failed          bool    `json:"failed"`
	Error           string  `json:"error,omitempty"`
}

// ImitationReport separates the protocol from the per-seed results and the
// random-policy reference they are read against.
type ImitationReport struct {
	SchemaVersion string                `json:"schema_version"`
	Config        ImitationConfig       `json:"config"`
	ConfigHash    string                `json:"config_hash"`
	Results       []ImitationSeedResult `json:"results"`
	Baseline      float64               `json:"random_policy_mean_return"`
	Assumptions   []string              `json:"assumptions"`
}

// RunImitation trains one recurrent policy per seed on the expert's own
// actions and scores every seed on the same evaluation batch. A canceled
// context aborts the run; a failed seed keeps its error and the other seeds
// continue.
func RunImitation(ctx context.Context, c ImitationConfig) (ImitationReport, error) {
	var report ImitationReport
	if ctx == nil {
		return report, fmt.Errorf("imitation run needs a context")
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if err := c.Validate(); err != nil {
		return report, err
	}
	c.Seeds = append([]uint64(nil), c.Seeds...)
	report = ImitationReport{
		SchemaVersion: ImitationSchemaVersion,
		Config:        c,
		ConfigHash:    hash(c),
		Assumptions: []string{
			"Imitation targets are the BFS expert's actions on the corridor fixture; the evaluator never reads the expert.",
			"Recurrent inference re-runs the observation prefix at every step; efficiency is not a claim.",
		},
	}
	for _, seed := range c.Seeds {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		baseline, err := imitationRandomReturn(c, seed)
		if err != nil {
			return report, err
		}
		report.Baseline += baseline / float64(len(c.Seeds))
		out, err := runImitationSeed(ctx, c, seed)
		if err != nil {
			if cerr := ctx.Err(); cerr != nil {
				return report, cerr
			}
			out = ImitationSeedResult{Seed: seed, Failed: true, Error: err.Error()}
		}
		report.Results = append(report.Results, out)
	}
	return report, nil
}

// runImitationSeed scores one fresh policy, trains it on Episodes expert
// trajectories and scores it again on the same batch.
func runImitationSeed(ctx context.Context, c ImitationConfig, seed uint64) (ImitationSeedResult, error) {
	out := ImitationSeedResult{Seed: seed}
	tr, err := newImitationTrainer(seed, c.Hidden, c.LearningRate)
	if err != nil {
		return out, err
	}
	if out.AgreementBefore, _, err = evaluateImitation(ctx, tr, c); err != nil {
		return out, err
	}
	for e := 0; e < c.Episodes; e++ {
		input, actions, err := imitationExpertEpisode(c.Corridor, imitationTrainSeed(seed, e))
		if err != nil {
			return out, err
		}
		if out.Updates, err = imitationStep(ctx, tr, input, actions); err != nil {
			return out, err
		}
	}
	out.ExpertAgreement, out.MeanReturn, err = evaluateImitation(ctx, tr, c)
	return out, err
}

// newImitationTrainer fixes the recurrent policy: four input neurons carrying
// the observation unchanged, a fully recurrent hidden block and three readout
// neurons whose per-step activity is the action logits. Only the core weights,
// the biases and the readout train; the identity encoder and the time
// constants are fixed, so the observation reaches the core untransformed.
func newImitationTrainer(seed uint64, hidden int, rate float64) (*learning.Trainer, error) {
	nodes := imitationInputs + hidden + imitationOutputs
	hiddenFirst, readoutFirst := imitationInputs, imitationInputs+hidden
	var sources, targets []int
	edge := func(from, to int) {
		sources, targets = append(sources, from), append(targets, to)
	}
	for i := 0; i < imitationInputs; i++ {
		for h := 0; h < hidden; h++ {
			edge(i, hiddenFirst+h)
		}
	}
	for a := 0; a < hidden; a++ {
		for b := 0; b < hidden; b++ {
			edge(hiddenFirst+a, hiddenFirst+b)
		}
	}
	for h := 0; h < hidden; h++ {
		for o := 0; o < imitationOutputs; o++ {
			edge(hiddenFirst+h, readoutFirst+o)
		}
	}
	inputNodes := make([]int, imitationInputs)
	for i := range inputNodes {
		inputNodes[i] = i
	}
	readoutNodes := make([]int, imitationOutputs)
	for i := range readoutNodes {
		readoutNodes[i] = readoutFirst + i
	}
	config := learning.Config{
		Dynamics:         dynamics.Config{Nodes: nodes, Sources: sources, Targets: targets, DT: 1, Activation: "tanh"},
		InputSize:        imitationInputs,
		OutputSize:       imitationOutputs,
		ReadoutNodes:     readoutNodes,
		InputNodes:       inputNodes,
		ReadoutEveryStep: true,
	}
	logTau := make([]float64, nodes)
	for i := range logTau {
		logTau[i] = math.Log(2)
	}
	encoder := make([]float64, imitationInputs*imitationInputs)
	for i := 0; i < imitationInputs; i++ {
		encoder[i*imitationInputs+i] = 1
	}
	p := learning.Parameters{
		Core:    dynamics.Parameters{Weights: imitationUniform(seed, 0, len(sources)), Bias: make([]float64, nodes), LogTau: logTau},
		Encoder: encoder,
		Readout: imitationUniform(seed, 2, len(readoutNodes)*imitationOutputs),
	}
	o := learning.DefaultOptions()
	o.LearningRate = rate
	o.Trainable = learning.Trainable{Weights: true, Bias: true, Readout: true}
	return learning.NewTrainer(config, p, o)
}

// imitationStep is one supervised update: softmax cross entropy against the
// expert's action at every step of the episode, averaged over the steps, fed
// to the shared gradient entry point as dL/dlogits = (p - onehot)/steps.
func imitationStep(ctx context.Context, tr *learning.Trainer, input [][]float64, actions []int) (uint64, error) {
	logits, err := tr.PredictAll(ctx, input)
	if err != nil {
		return 0, err
	}
	upstream := make([][]float64, len(logits))
	scale := 1 / float64(len(logits))
	for t, row := range logits {
		g := imitationSoftmax(row)
		for j := range g {
			g[j] *= scale
		}
		g[actions[t]] -= scale
		upstream[t] = g
	}
	result, err := tr.StepFrom(ctx, input, upstream)
	if err != nil {
		return 0, err
	}
	return result.Updates, nil
}

// evaluateImitation replays the shared evaluation batch under the policy's own
// actions: the agreement is the fraction of steps whose action equals the
// expert's action in the state the agent actually reached, and the return is
// the mean reward per episode. The expert is read only to score, never to
// choose: imitationAction is the single decision point and it receives the
// observation prefix alone.
func evaluateImitation(ctx context.Context, tr *learning.Trainer, c ImitationConfig) (float64, float64, error) {
	env, err := gridnav.New(c.Corridor)
	if err != nil {
		return 0, 0, err
	}
	var matched, steps int
	var total float64
	for i := 0; i < c.EvalEpisodes; i++ {
		obs, _ := env.Reset(imitationEvalSeed(i))
		prefix := [][]float64{obs.Vector()}
		for {
			action, err := imitationAction(ctx, tr, prefix)
			if err != nil {
				return 0, 0, err
			}
			if action == env.Expert() {
				matched++
			}
			steps++
			next, reward, done, _, err := env.Step(action)
			if err != nil {
				return 0, 0, err
			}
			total += reward
			if done {
				break
			}
			prefix = append(prefix, next.Vector())
		}
	}
	return float64(matched) / float64(steps), total / float64(c.EvalEpisodes), nil
}

// imitationAction is the evaluator's only decision point: recurrent inference
// re-runs the whole observation prefix and takes the argmax of the last
// per-step readout. It receives no environment, so it cannot reach
// gridnav.Env.Expert.
func imitationAction(ctx context.Context, tr *learning.Trainer, prefix [][]float64) (int, error) {
	logits, err := tr.PredictAll(ctx, prefix)
	if err != nil {
		return 0, err
	}
	last := logits[len(logits)-1]
	best := 0
	for i, v := range last {
		if v > last[best] {
			best = i
		}
	}
	return best, nil
}

// imitationExpertEpisode walks the expert from Reset(seed) to done and returns
// the observations it saw and the action it took in each of them.
func imitationExpertEpisode(c gridnav.Config, seed uint64) ([][]float64, []int, error) {
	env, err := gridnav.New(c)
	if err != nil {
		return nil, nil, err
	}
	obs, _ := env.Reset(seed)
	var input [][]float64
	var actions []int
	for {
		action := env.Expert()
		input = append(input, obs.Vector())
		actions = append(actions, action)
		next, _, done, _, err := env.Step(action)
		if err != nil {
			return nil, nil, err
		}
		obs = next
		if done {
			return input, actions, nil
		}
	}
}

// imitationRandomReturn is the reference the learned return is read against: a
// uniform action draw from PCG(seed, 1) over the same evaluation batch.
func imitationRandomReturn(c ImitationConfig, seed uint64) (float64, error) {
	env, err := gridnav.New(c.Corridor)
	if err != nil {
		return 0, err
	}
	r := rand.New(rand.NewPCG(seed, 1))
	var total float64
	for i := 0; i < c.EvalEpisodes; i++ {
		env.Reset(imitationEvalSeed(i))
		for {
			_, reward, done, _, err := env.Step(r.IntN(gridnav.Actions))
			if err != nil {
				return 0, err
			}
			total += reward
			if done {
				break
			}
		}
	}
	return total / float64(c.EvalEpisodes), nil
}

// imitationUniform draws n values uniformly from [-0.5, 0.5) with PCG(seed,
// stream), so every parameter group of a seed has its own reproducible stream.
func imitationUniform(seed, stream uint64, n int) []float64 {
	r := rand.New(rand.NewPCG(seed, stream))
	out := make([]float64, n)
	for i := range out {
		out[i] = r.Float64() - .5
	}
	return out
}

// imitationSoftmax is the numerically shifted softmax of one logit row.
func imitationSoftmax(v []float64) []float64 {
	high := v[0]
	for _, x := range v {
		if x > high {
			high = x
		}
	}
	out := make([]float64, len(v))
	var sum float64
	for i, x := range v {
		out[i] = math.Exp(x - high)
		sum += out[i]
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

// imitationTrainSeed derives one training episode's corridor seed from the run
// seed, keeping the training episodes clear of the fixed evaluation batch.
func imitationTrainSeed(seed uint64, episode int) uint64 {
	return mix(seed ^ (uint64(episode)+1)*0x9e3779b97f4a7c15)
}

// imitationEvalSeed is the corridor seed of evaluation episode index.
func imitationEvalSeed(index int) uint64 { return imitationEvalBase + uint64(index) }
