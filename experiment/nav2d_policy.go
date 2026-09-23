package experiment

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/experiment/nav2d"
	"github.com/TimLai666/coimnet/learning"
)

// Policy kinds for newNav2DPolicy.
const (
	Nav2DRecurrent   = "recurrent"   // recurrent block, acts on the whole observation prefix
	Nav2DFeedforward = "feedforward" // no hidden to hidden edges, acts on the current observation only
	Nav2DRandom      = "random"      // uniform random actions, no model
	Nav2DRewired     = "rewired"     // recurrent policy whose hidden→hidden edges were degree-preservingly rewired
)

// Nav2DMetrics summarises one evaluation map set under one policy. Every
// episode ends either reached or timed out, so SuccessRate + TimeoutRate == 1.
// ExpertAgreement is the fraction of steps whose action equals env.Expert() in
// the state the agent actually reached; the expert is read only to score,
// never to choose.
type Nav2DMetrics struct {
	Episodes        int     `json:"episodes"`
	SuccessRate     float64 `json:"success_rate"`
	TimeoutRate     float64 `json:"timeout_rate"`
	MeanSteps       float64 `json:"mean_steps"`
	MeanCollisions  float64 `json:"mean_collisions"`
	MeanReturn      float64 `json:"mean_return"`
	ExpertAgreement float64 `json:"expert_agreement"`
}

// nav2dPolicy is one policy under evaluation. trainer is nil and rng is set
// only for the random policy.
//
// settle is how many consecutive rows each observation is presented for.
// dynamics.Continuous drives row t from the outputs of the previous step, so
// the readout of the row that carries observation t acts on a one-step-old
// view (two steps through the hidden layer). Both learned kinds therefore
// hold every observation for settle rows and score only the last copy, which
// is the first row whose readout has caught up with the observation.
type nav2dPolicy struct {
	kind    string
	trainer *learning.Trainer
	rng     *rand.Rand
	settle  int
	edges   int
	// rewire holds the degree-preserving rewire checklist when kind is
	// Nav2DRewired, and is nil for every other kind.
	rewire *RewireReport
}

// nav2dEdges lists the core edges in this fixed order: every input node to
// every hidden node; then, when withRecurrent, one r :=
// rand.New(rand.NewPCG(seed, 5)) is created and, for each hidden target in
// order, r.Perm(hidden) is called and its first `recurrent` entries become
// that target's sources (self loops allowed, sources distinct per target);
// then every hidden node to every readout node; then every input node to every
// readout node (the direct path keeps the first steps' logits from being
// zero). Node order: inputs 0..inputs-1, hidden, readout (Actions nodes).
func nav2dEdges(seed uint64, inputs, hidden, recurrent int, withRecurrent bool) (sources, targets []int) {
	var src, tgt []int
	edge := func(from, to int) {
		src = append(src, from)
		tgt = append(tgt, to)
	}
	hiddenFirst := inputs
	readoutFirst := inputs + hidden
	for i := 0; i < inputs; i++ {
		for h := 0; h < hidden; h++ {
			edge(i, hiddenFirst+h)
		}
	}
	if withRecurrent && recurrent > 0 {
		r := rand.New(rand.NewPCG(seed, 5))
		take := recurrent
		if take > hidden {
			take = hidden
		}
		for h := 0; h < hidden; h++ {
			perm := r.Perm(hidden)
			for k := 0; k < take; k++ {
				edge(hiddenFirst+perm[k], hiddenFirst+h)
			}
		}
	}
	for h := 0; h < hidden; h++ {
		for o := 0; o < nav2d.Actions; o++ {
			edge(hiddenFirst+h, readoutFirst+o)
		}
	}
	for i := 0; i < inputs; i++ {
		for o := 0; o < nav2d.Actions; o++ {
			edge(i, readoutFirst+o)
		}
	}
	return src, tgt
}

// newNav2DPolicy builds a policy. recurrent/feedforward: continuous core
// (DT 1, tanh), InputSize = InputNodes = inputs with an identity encoder,
// OutputSize Actions, ReadoutEveryStep true, weights uniform [-0.3, 0.3] from
// rand.NewPCG(seed, 0), Bias 0, LogTau log(2); DefaultOptions with LearningRate
// rate and Trainable Weights+Readout (Bias stays 0 so Adam cannot give "always
// forward" a free per-step push from the biases while the input-dependent
// weights stall). The readout reads both the hidden nodes and the Actions
// readout nodes: ReadoutNodes = the hidden nodes in order followed by the
// readout nodes, so its (hidden + Actions) x Actions matrix starts from all
// zeros except the readout rows, which are uniform [-0.3, 0.3] from
// rand.NewPCG(seed, 2). The zero hidden rows give the readout a fully
// trainable direct view of the hidden states, so the imitation gradient can
// reach the hidden layer even when the tanh readout nodes have saturated;
// feedforward uses withRecurrent false. Both learned
// kinds hold each observation for settle = 3 consecutive rows, because the
// continuous core computes row t from the previous step's output and scoring
// the last copy is what lets the readout reflect the observation the policy
// reacts to (see nav2dPolicy.settle). random: rng =
// rand.New(rand.NewPCG(seed, 0x1001)).
func newNav2DPolicy(kind string, seed uint64, inputs, hidden, recurrent int, rate float64) (*nav2dPolicy, error) {
	if kind == Nav2DRandom {
		return &nav2dPolicy{kind: kind, rng: rand.New(rand.NewPCG(seed, 0x1001))}, nil
	}
	if kind != Nav2DRecurrent && kind != Nav2DFeedforward && kind != Nav2DRewired {
		return nil, fmt.Errorf("unknown nav2d policy kind %q", kind)
	}
	if inputs <= 0 || hidden <= 0 {
		return nil, fmt.Errorf("nav2d policy needs positive inputs and hidden, got %d and %d", inputs, hidden)
	}
	withRecurrent := kind == Nav2DRecurrent || kind == Nav2DRewired
	sources, targets := nav2dEdges(seed, inputs, hidden, recurrent, withRecurrent)
	var rewire *RewireReport
	if kind == Nav2DRewired {
		rewiredTargets, report, rewireErr := nav2dRewire(sources, targets, inputs, hidden, seed)
		if rewireErr != nil {
			return nil, fmt.Errorf("nav2d rewired policy: %w", rewireErr)
		}
		targets = rewiredTargets
		rewire = &report
	}
	nodes := inputs + hidden + nav2d.Actions
	readoutFirst := inputs + hidden
	inputNodes := make([]int, inputs)
	for i := range inputNodes {
		inputNodes[i] = i
	}
	readoutNodes := make([]int, 0, hidden+nav2d.Actions)
	for h := 0; h < hidden; h++ {
		readoutNodes = append(readoutNodes, inputs+h)
	}
	for o := 0; o < nav2d.Actions; o++ {
		readoutNodes = append(readoutNodes, readoutFirst+o)
	}
	config := learning.Config{
		Dynamics:         dynamics.Config{Nodes: nodes, Sources: sources, Targets: targets, DT: 1, Activation: "tanh"},
		InputSize:        inputs,
		OutputSize:       nav2d.Actions,
		ReadoutNodes:     readoutNodes,
		InputNodes:       inputNodes,
		ReadoutEveryStep: true,
	}
	logTau := make([]float64, nodes)
	for i := range logTau {
		logTau[i] = math.Log(2)
	}
	encoder := make([]float64, inputs*inputs)
	for i := 0; i < inputs; i++ {
		encoder[i*inputs+i] = 1
	}
	readout := make([]float64, (hidden+nav2d.Actions)*nav2d.Actions)
	actRows := nav2dUniform(seed, 2, nav2d.Actions*nav2d.Actions)
	for o := 0; o < nav2d.Actions; o++ {
		copy(readout[(hidden+o)*nav2d.Actions:], actRows[o*nav2d.Actions:(o+1)*nav2d.Actions])
	}
	p := learning.Parameters{
		Core:    dynamics.Parameters{Weights: nav2dUniform(seed, 0, len(sources)), Bias: make([]float64, nodes), LogTau: logTau},
		Encoder: encoder,
		Readout: readout,
	}
	o := learning.DefaultOptions()
	o.LearningRate = rate
	o.Trainable = learning.Trainable{Weights: true, Bias: false, Readout: true}
	tr, err := learning.NewTrainer(config, p, o)
	if err != nil {
		return nil, err
	}
	return &nav2dPolicy{kind: kind, trainer: tr, settle: 3, edges: len(sources), rewire: rewire}, nil
}

// nav2dTrainMap and nav2dUnseenMap are the map seeds: training maps are odd
// (mix(seed ^ uint64(e+1)*0x9e3779b97f4a7c15) | 1), unseen maps are even
// ((uint64(i)+1) << 1), so the two sets can never overlap.
func nav2dTrainMap(seed uint64, e int) uint64 {
	return mix(seed^uint64(e+1)*0x9e3779b97f4a7c15) | 1
}

// nav2dUnseenMap returns the even map seed of an untouched evaluation map.
func nav2dUnseenMap(i int) uint64 {
	return (uint64(i) + 1) << 1
}

// nav2dExpertEpisode walks env.Expert() from Reset(mapSeed) until done and
// returns the observation vectors and actions.
func nav2dExpertEpisode(c nav2d.Config, mapSeed uint64) ([][]float64, []int, error) {
	env, err := nav2d.New(c)
	if err != nil {
		return nil, nil, err
	}
	obs, _ := env.Reset(mapSeed)
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

// nav2dDAggerEpisode rolls the policy's own actions out on one training map
// and asks the expert what it would have done in every state the policy
// reached (DAgger with beta = 0): from Reset(mapSeed), at every step it
// appends obs.Vector() to the observations and env.Expert() to the labels,
// then executes p.act(ctx, observations so far) — never the label — until the
// episode is done. The expert only labels here; it never chooses an executed
// action.
func nav2dDAggerEpisode(ctx context.Context, p *nav2dPolicy, c nav2d.Config, mapSeed uint64) ([][]float64, []int, error) {
	env, err := nav2d.New(c)
	if err != nil {
		return nil, nil, err
	}
	obs, _ := env.Reset(mapSeed)
	prefix := [][]float64{obs.Vector()}
	var input [][]float64
	var labels []int
	for {
		input = append(input, obs.Vector())
		labels = append(labels, env.Expert())
		action, err := p.act(ctx, prefix)
		if err != nil {
			return nil, nil, err
		}
		next, _, done, _, err := env.Step(action)
		if err != nil {
			return nil, nil, err
		}
		obs = next
		if done {
			return input, labels, nil
		}
		prefix = append(prefix, next.Vector())
	}
}

// trainNav2D is the one imitation schedule every learned policy follows, so
// policies of one comparison share the same data schedule: for e =
// 0..episodes-1 on map nav2dTrainMap(seed, e), episodes with e < warm =
// (episodes+3)/4 are expert demonstrations (nav2dExpertEpisode); after that,
// even e is a DAgger rollout (nav2dDAggerEpisode) and odd e a demonstration.
// Each episode is exactly one p.train call on its observations and labels. The
// random policy is never trained and returns 0 without rolling anything out.
// It returns the policy's update count after the last call. episodes < 1 is an
// error.
func trainNav2D(ctx context.Context, p *nav2dPolicy, c nav2d.Config, seed uint64, episodes int) (uint64, error) {
	if episodes < 1 {
		return 0, fmt.Errorf("trainNav2D needs at least one episode, got %d", episodes)
	}
	if p.kind == Nav2DRandom {
		return 0, nil
	}
	warm := (episodes + 3) / 4
	var updates uint64
	for e := 0; e < episodes; e++ {
		mapSeed := nav2dTrainMap(seed, e)
		var obs [][]float64
		var actions []int
		var err error
		if e < warm || e%2 == 1 {
			obs, actions, err = nav2dExpertEpisode(c, mapSeed)
		} else {
			obs, actions, err = nav2dDAggerEpisode(ctx, p, c, mapSeed)
		}
		if err != nil {
			return 0, fmt.Errorf("trainNav2D episode %d: %w", e, err)
		}
		if updates, err = p.train(ctx, obs, actions); err != nil {
			return 0, fmt.Errorf("trainNav2D train episode %d: %w", e, err)
		}
	}
	return updates, nil
}

// train runs one imitation update on one demonstration and returns the
// trainer's update count. Because the continuous core computes row t from the
// previous step's output, both learned kinds first stretch the episode: each
// observation is repeated for p.settle rows and only the last copy is scored,
// so the gradient sees the observation its logits have caught up with.
// recurrent: one StepFrom over the stretched episode with upstream
// (softmax(logits) - onehot(action)) * w(action) / len(obs) on the last copy
// of every observation and zeros elsewhere. feedforward: for each step, one
// StepFrom on p.settle copies of that observation with the same weighted
// softmax cross-entropy gradient on the last row only (other rows zero).
// The weight w(a) = n / (m * count(a)) is the inverse label frequency of this
// demonstration (n labelled steps, m distinct labels, count(a) occurrences of
// a), so the weights average to 1 and the rare turn labels pull as hard as the
// dominant forward label instead of being drowned by it. random: no-op,
// returns 0.
func (p *nav2dPolicy) train(ctx context.Context, obs [][]float64, actions []int) (uint64, error) {
	weights := nav2dLabelWeights(actions)
	switch p.kind {
	case Nav2DRandom:
		return 0, nil
	case Nav2DRecurrent, Nav2DRewired:
		in := nav2dRepeat(obs, p.settle)
		logits, err := p.trainer.PredictAll(ctx, in)
		if err != nil {
			return 0, err
		}
		upstream := make([][]float64, len(logits))
		scale := 1 / float64(len(obs))
		for t := range upstream {
			upstream[t] = make([]float64, nav2d.Actions)
		}
		for t := p.settle - 1; t < len(logits); t += p.settle {
			g := imitationSoftmax(logits[t])
			w := weights[actions[t/p.settle]]
			for j := range g {
				g[j] *= scale * w
			}
			g[actions[t/p.settle]] -= scale * w
			upstream[t] = g
		}
		result, err := p.trainer.StepFrom(ctx, in, upstream)
		if err != nil {
			return 0, err
		}
		return result.Updates, nil
	case Nav2DFeedforward:
		var updates uint64
		for t, o := range obs {
			in := make([][]float64, p.settle)
			for i := range in {
				in[i] = o
			}
			logits, err := p.trainer.PredictAll(ctx, in)
			if err != nil {
				return 0, err
			}
			upstream := make([][]float64, p.settle)
			for i := range upstream {
				upstream[i] = make([]float64, nav2d.Actions)
			}
			w := weights[actions[t]]
			g := imitationSoftmax(logits[p.settle-1])
			for j := range g {
				g[j] *= w
			}
			g[actions[t]] -= w
			upstream[p.settle-1] = g
			result, err := p.trainer.StepFrom(ctx, in, upstream)
			if err != nil {
				return 0, err
			}
			updates = result.Updates
		}
		return updates, nil
	}
	return 0, fmt.Errorf("unknown nav2d policy kind %q", p.kind)
}

// nav2dLabelWeights returns the inverse-frequency imitation weight of each
// label for one demonstration: w(a) = n / (m * count(a)), where n is the
// number of labelled steps, m the number of distinct labels and count(a) how
// often a occurs, so the average weight over the steps is 1. An empty
// demonstration yields all-zero weights (no step ever reads them).
func nav2dLabelWeights(actions []int) []float64 {
	var counts [nav2d.Actions]int
	distinct := 0
	for _, a := range actions {
		if counts[a] == 0 {
			distinct++
		}
		counts[a]++
	}
	w := make([]float64, nav2d.Actions)
	if len(actions) == 0 || distinct == 0 {
		return w
	}
	n := float64(len(actions))
	for a, c := range counts {
		if c > 0 {
			w[a] = n / (float64(distinct) * float64(c))
		}
	}
	return w
}

// act is the only decision point and receives the observation prefix alone. It
// stretches the prefix like train does: the readout of the row carrying an
// observation is one step old, so the decision is taken from the last copy of
// the current observation, where the readout has caught up with it.
// recurrent: argmax of the last row of PredictAll with every prefix
// observation repeated p.settle times. feedforward: argmax of the last row of
// PredictAll(p.settle copies of the last observation). random:
// rng.IntN(nav2d.Actions). Ties go to the lower action index.
func (p *nav2dPolicy) act(ctx context.Context, prefix [][]float64) (int, error) {
	switch p.kind {
	case Nav2DRandom:
		return p.rng.IntN(nav2d.Actions), nil
	case Nav2DRecurrent, Nav2DRewired:
		in := nav2dRepeat(prefix, p.settle)
		logits, err := p.trainer.PredictAll(ctx, in)
		if err != nil {
			return 0, err
		}
		return nav2dArgmax(logits[len(logits)-1]), nil
	case Nav2DFeedforward:
		last := prefix[len(prefix)-1]
		in := make([][]float64, p.settle)
		for i := range in {
			in[i] = last
		}
		logits, err := p.trainer.PredictAll(ctx, in)
		if err != nil {
			return 0, err
		}
		return nav2dArgmax(logits[len(logits)-1]), nil
	}
	return 0, fmt.Errorf("unknown nav2d policy kind %q", p.kind)
}

// evaluateNav2D runs one episode per map seed under the policy's own actions
// and returns the metrics.
func evaluateNav2D(ctx context.Context, p *nav2dPolicy, c nav2d.Config, maps []uint64) (Nav2DMetrics, error) {
	var out Nav2DMetrics
	if len(maps) == 0 {
		return out, fmt.Errorf("nav2d evaluation needs at least one map")
	}
	env, err := nav2d.New(c)
	if err != nil {
		return out, err
	}
	var matched, steps, collisions, reached, timedOut int
	var totalReturn float64
	for _, m := range maps {
		obs, _ := env.Reset(m)
		prefix := [][]float64{obs.Vector()}
		for {
			action, err := p.act(ctx, prefix)
			if err != nil {
				return out, err
			}
			if action == env.Expert() {
				matched++
			}
			steps++
			next, reward, done, info, err := env.Step(action)
			if err != nil {
				return out, err
			}
			if done {
				collisions += info.Collisions
				if info.Reached {
					reached++
				} else {
					timedOut++
				}
				totalReturn += reward
				break
			}
			totalReturn += reward
			prefix = append(prefix, next.Vector())
		}
	}
	return Nav2DMetrics{
		Episodes:        len(maps),
		SuccessRate:     float64(reached) / float64(len(maps)),
		TimeoutRate:     float64(timedOut) / float64(len(maps)),
		MeanSteps:       float64(steps) / float64(len(maps)),
		MeanCollisions:  float64(collisions) / float64(len(maps)),
		MeanReturn:      totalReturn / float64(len(maps)),
		ExpertAgreement: float64(matched) / float64(steps),
	}, nil
}

// parameters reports the learnable parameter count (len Weights + Bias +
// LogTau + Encoder + Readout of the trainer's snapshot) and the edge count;
// both 0 for random.
func (p *nav2dPolicy) parameters() (params, edges int) {
	if p == nil || p.trainer == nil {
		return 0, 0
	}
	s := p.trainer.Snapshot()
	params = len(s.Parameters.Core.Weights) + len(s.Parameters.Core.Bias) + len(s.Parameters.Core.LogTau) + len(s.Parameters.Encoder) + len(s.Parameters.Readout)
	return params, p.edges
}

// nav2dRepeat returns rows with every observation repeated settle times in
// order, sharing the observation slices (they are never mutated). It is the
// common shape behind both training and decisions: scoring the last copy is
// what lets the readout react to the current observation, because the
// continuous core drives row t from the previous step's outputs.
func nav2dRepeat(rows [][]float64, settle int) [][]float64 {
	out := make([][]float64, 0, len(rows)*settle)
	for _, r := range rows {
		for i := 0; i < settle; i++ {
			out = append(out, r)
		}
	}
	return out
}

// nav2dUniform draws n values uniformly from [-0.3, 0.3) with PCG(seed,
// stream), so every parameter group of a policy has its own reproducible
// stream.
func nav2dUniform(seed, stream uint64, n int) []float64 {
	r := rand.New(rand.NewPCG(seed, stream))
	out := make([]float64, n)
	for i := range out {
		out[i] = r.Float64()*0.6 - 0.3
	}
	return out
}

// nav2dArgmax returns the index of the highest row value, breaking ties to the
// lower index.
func nav2dArgmax(row []float64) int {
	best := 0
	for i, v := range row {
		if v > row[best] {
			best = i
		}
	}
	return best
}
