package experiment

import (
	"context"
	"fmt"
	"math"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/experiment/gridnav"
	"github.com/TimLai666/coimnet/learning"
)

// Two tasks share one core. Input channels 0..3 carry the corridor observation
// (gridnav Observation.Vector) and channel 4 the delayed pulse; outputs 0..2
// are the corridor action logits and output 3 the delayed prediction. A task's
// adapter is its rows of the encoder and its columns of the readout; the core
// (weights, bias, log_tau) is the same object for both.
const (
	// MultiTaskCorridor names the corridor imitation task.
	MultiTaskCorridor = "corridor"
	// MultiTaskDelayed names the delayed-association task.
	MultiTaskDelayed = "delayed"
)

const (
	// multiTaskInputs is the shared input width: the corridor observation in
	// channels 0..3 and the delayed pulse in multiTaskDelayedChannel.
	multiTaskInputs = imitationInputs + 1
	// multiTaskOutputs is the shared readout width: the corridor logits in
	// outputs 0..2 and the delayed prediction in multiTaskDelayedOutput.
	multiTaskOutputs = gridnav.Actions + 1
	// multiTaskDelayedChannel and multiTaskDelayedOutput are the delayed
	// task's only input channel and only output.
	multiTaskDelayedChannel = imitationInputs
	multiTaskDelayedOutput  = gridnav.Actions
	// multiTaskSettle is how many consecutive rows each corridor observation
	// is held for. The continuous core drives row t from the previous step's
	// outputs, so an observation reaches the readout one row after it arrives
	// through the direct input-to-readout edges and two rows after through the
	// hidden block; the corridor head acts on and is scored at the last copy.
	multiTaskSettle = 3
	// multiTaskDelayedEvalSeed is the delayed task's holdout split, the
	// TestSeed of DefaultDelayedConfig.
	multiTaskDelayedEvalSeed uint64 = 1003
)

// multiTaskModel is the single trainer every task updates. hidden is the size
// of the recurrent block, which fixes where the readout-node rows of the
// readout start. built counts the NewTrainer calls behind the model and must
// stay 1, so a task that grew a core of its own would show as a second build.
//
// Every step runs through the trainer's one AdamW state. A step's gradient
// reaches only the core and its own task's adapter, but once a task has
// stepped, its adapter keeps moving on its optimizer momentum during the other
// task's steps.
type multiTaskModel struct {
	trainer *learning.Trainer
	hidden  int
	built   int
}

// newMultiTaskModel builds the shared core: nodes = 5 input nodes + hidden + 4
// readout nodes; InputSize 5 with an identity encoder into the 5 input nodes;
// ReadoutNodes = the hidden nodes in order followed by the 4 readout nodes, so
// the readout is (hidden+4)×4 with the hidden rows at 0 and the readout-node
// rows uniform [-0.3, 0.3] from rand.NewPCG(seed, 2) (a readout limited to a
// few tanh readout nodes collapsed in the navigation policy; reading the
// hidden nodes directly fixed it); OutputSize 4; ReadoutEveryStep true; edges,
// in this order, from every input node to every hidden node, from every hidden
// node to every hidden node (self loops included), from every hidden node to
// every readout node and from every input node to every readout node; weights
// uniform [-0.3, 0.3] from rand.NewPCG(seed, 0) in edge order; Bias 0; LogTau
// log(2); DefaultOptions with LearningRate rate and the weights, time
// constants, encoder and readout trainable, so the bias stays 0. hidden must
// lie in [1, imitationMaxHidden], the bound of the imitation fixture's
// recurrent block of the same hidden-squared edge cost, and rate must be
// finite and positive.
func newMultiTaskModel(seed uint64, hidden int, rate float64) (*multiTaskModel, error) {
	if hidden < 1 || hidden > imitationMaxHidden {
		return nil, fmt.Errorf("multitask hidden %d, want a value in [1, %d]", hidden, imitationMaxHidden)
	}
	if !finite(rate) || rate <= 0 {
		return nil, fmt.Errorf("multitask learning rate %v must be finite and positive", rate)
	}
	hiddenFirst, readoutFirst := multiTaskInputs, multiTaskInputs+hidden
	nodes := readoutFirst + multiTaskOutputs
	var sources, targets []int
	block := func(fromFirst, fromCount, toFirst, toCount int) {
		for from := fromFirst; from < fromFirst+fromCount; from++ {
			for to := toFirst; to < toFirst+toCount; to++ {
				sources, targets = append(sources, from), append(targets, to)
			}
		}
	}
	block(0, multiTaskInputs, hiddenFirst, hidden)
	block(hiddenFirst, hidden, hiddenFirst, hidden)
	block(hiddenFirst, hidden, readoutFirst, multiTaskOutputs)
	block(0, multiTaskInputs, readoutFirst, multiTaskOutputs)
	inputNodes := make([]int, multiTaskInputs)
	encoder := make([]float64, multiTaskInputs*multiTaskInputs)
	for i := range inputNodes {
		inputNodes[i] = i
		encoder[i*multiTaskInputs+i] = 1
	}
	readoutNodes := make([]int, 0, hidden+multiTaskOutputs)
	for n := hiddenFirst; n < nodes; n++ {
		readoutNodes = append(readoutNodes, n)
	}
	readout := make([]float64, (hidden+multiTaskOutputs)*multiTaskOutputs)
	copy(readout[hidden*multiTaskOutputs:], nav2dUniform(seed, 2, multiTaskOutputs*multiTaskOutputs))
	logTau := make([]float64, nodes)
	for i := range logTau {
		logTau[i] = math.Log(2)
	}
	config := learning.Config{
		Dynamics:         dynamics.Config{Nodes: nodes, Sources: sources, Targets: targets, DT: 1, Activation: "tanh"},
		InputSize:        multiTaskInputs,
		OutputSize:       multiTaskOutputs,
		ReadoutNodes:     readoutNodes,
		InputNodes:       inputNodes,
		ReadoutEveryStep: true,
	}
	p := learning.Parameters{
		Core:    dynamics.Parameters{Weights: nav2dUniform(seed, 0, len(sources)), Bias: make([]float64, nodes), LogTau: logTau},
		Encoder: encoder,
		Readout: readout,
	}
	o := learning.DefaultOptions()
	o.LearningRate = rate
	o.Trainable = learning.Trainable{Encoder: true, Weights: true, Tau: true, Readout: true}
	tr, err := learning.NewTrainer(config, p, o)
	if err != nil {
		return nil, err
	}
	return &multiTaskModel{trainer: tr, hidden: hidden, built: 1}, nil
}

// corridorStep is one imitation update of the corridor head on one expert
// episode: rows = every observation repeated 3 times in channels 0..3
// (channel 4 zero); upstream only on outputs 0..2 of each observation's third
// row = scale × (softmax − onehot(expert))/observations, the softmax taken
// over outputs 0..2; output 3 upstream zero on every row. obs and actions must
// have the same positive length, every observation 4 values and every action
// a corridor action; a refused episode leaves the trainer untouched.
func (m *multiTaskModel) corridorStep(ctx context.Context, obs [][]float64, actions []int, scale float64) error {
	if len(obs) == 0 || len(actions) != len(obs) {
		return fmt.Errorf("corridor step has %d observations and %d actions, want the same positive count", len(obs), len(actions))
	}
	for k, o := range obs {
		if len(o) != imitationInputs {
			return fmt.Errorf("corridor observation %d has %d values, want %d", k, len(o), imitationInputs)
		}
		if actions[k] < 0 || actions[k] >= gridnav.Actions {
			return fmt.Errorf("corridor action %d at step %d is outside [0, %d)", actions[k], k, gridnav.Actions)
		}
	}
	input := multiTaskCorridorRows(obs)
	logits, err := m.trainer.PredictAll(ctx, input)
	if err != nil {
		return err
	}
	upstream := make([][]float64, len(input))
	for t := range upstream {
		upstream[t] = make([]float64, multiTaskOutputs)
	}
	per := scale / float64(len(obs))
	for k, action := range actions {
		t := (k+1)*multiTaskSettle - 1
		p := imitationSoftmax(logits[t][:gridnav.Actions])
		p[action]--
		for j, v := range p {
			upstream[t][j] = per * v
		}
	}
	_, err = m.trainer.StepFrom(ctx, input, upstream)
	return err
}

// delayedStep is one update of the delayed head on one DelayedEpisode: rows =
// the episode input in channel 4 (channels 0..3 zero); upstream only on output
// 3 of the last row = scale × 2(y − target), the derivative of that output's
// squared error; outputs 0..2 upstream zero on every row. The episode needs at
// least one one-value input row and exactly one target; a refused episode
// leaves the trainer untouched.
func (m *multiTaskModel) delayedStep(ctx context.Context, ep Episode, scale float64) error {
	input, err := multiTaskDelayedRows(ep)
	if err != nil {
		return err
	}
	outputs, err := m.trainer.PredictAll(ctx, input)
	if err != nil {
		return err
	}
	upstream := make([][]float64, len(input))
	for t := range upstream {
		upstream[t] = make([]float64, multiTaskOutputs)
	}
	last := len(input) - 1
	upstream[last][multiTaskDelayedOutput] = scale * 2 * (outputs[last][multiTaskDelayedOutput] - ep.Target[0])
	_, err = m.trainer.StepFrom(ctx, input, upstream)
	return err
}

// corridorScore is the expert agreement of the corridor head on evalEpisodes
// episodes (imitationEvalSeed(i)): the fraction of the steps taken whose
// action equals the expert's in the state the agent actually reached. The
// agent acts by recursive prefix inference with settle 3, every observation so
// far held for 3 rows and the action the argmax of outputs 0..2 of the last
// row, which is fixed before the expert is read. It uses PredictAll only, so
// the trainer is unchanged.
func (m *multiTaskModel) corridorScore(ctx context.Context, c gridnav.Config, evalEpisodes int) (float64, error) {
	if evalEpisodes < 1 {
		return 0, fmt.Errorf("corridor score needs at least one episode, got %d", evalEpisodes)
	}
	env, err := gridnav.New(c)
	if err != nil {
		return 0, err
	}
	var matched, steps int
	for i := 0; i < evalEpisodes; i++ {
		obs, _ := env.Reset(imitationEvalSeed(i))
		input := multiTaskCorridorRows([][]float64{obs.Vector()})
		for {
			logits, err := m.trainer.PredictAll(ctx, input)
			if err != nil {
				return 0, err
			}
			action := nav2dArgmax(logits[len(logits)-1][:gridnav.Actions])
			if action == env.Expert() {
				matched++
			}
			steps++
			next, _, done, _, err := env.Step(action)
			if err != nil {
				return 0, err
			}
			if done {
				break
			}
			input = append(input, multiTaskCorridorRows([][]float64{next.Vector()})...)
		}
	}
	return float64(matched) / float64(steps), nil
}

// delayedScore is −mean squared error of output 3 on the last row of
// evalEpisodes DelayedEpisode(1003, i), the delayed task's holdout split, so a
// higher score is better as with corridorScore. It uses PredictAll only, so
// the trainer is unchanged.
func (m *multiTaskModel) delayedScore(ctx context.Context, evalEpisodes int) (float64, error) {
	if evalEpisodes < 1 {
		return 0, fmt.Errorf("delayed score needs at least one episode, got %d", evalEpisodes)
	}
	var mse float64
	for i := 0; i < evalEpisodes; i++ {
		ep := DelayedEpisode(multiTaskDelayedEvalSeed, uint64(i))
		input, err := multiTaskDelayedRows(ep)
		if err != nil {
			return 0, err
		}
		outputs, err := m.trainer.PredictAll(ctx, input)
		if err != nil {
			return 0, err
		}
		d := outputs[len(outputs)-1][multiTaskDelayedOutput] - ep.Target[0]
		mse += d * d / float64(evalEpisodes)
	}
	return -mse, nil
}

// topologyHash is the SHA-256 hex of the JSON of the trainer's dynamics
// configuration: the anatomy of the one brain every task reads.
func (m *multiTaskModel) topologyHash() string {
	return hash(m.trainer.Snapshot().Config.Dynamics)
}

// baseParameterHash is the SHA-256 hex of the JSON of the core parameters
// (weights, bias, log_tau) every task shares.
func (m *multiTaskModel) baseParameterHash() string {
	return hash(m.trainer.Snapshot().Parameters.Core)
}

// adapterVersion is the SHA-256 hex of the JSON of one task's adapter: its
// encoder rows in row-major order (input channels 0..3 for the corridor, 4 for
// the delayed task) and its readout columns, each column read over every row
// with the hidden rows included (outputs 0..2 for the corridor, 3 for the
// delayed task). An unknown task has no adapter and returns "".
func (m *multiTaskModel) adapterVersion(task string) string {
	var channels, outputs []int
	switch task {
	case MultiTaskCorridor:
		channels, outputs = []int{0, 1, 2, 3}, []int{0, 1, 2}
	case MultiTaskDelayed:
		channels, outputs = []int{multiTaskDelayedChannel}, []int{multiTaskDelayedOutput}
	default:
		return ""
	}
	p := m.trainer.Snapshot().Parameters
	var encoder, readout []float64
	for _, i := range channels {
		encoder = append(encoder, p.Encoder[i*multiTaskInputs:(i+1)*multiTaskInputs]...)
	}
	for _, j := range outputs {
		for r := 0; r < m.hidden+multiTaskOutputs; r++ {
			readout = append(readout, p.Readout[r*multiTaskOutputs+j])
		}
	}
	return hash(struct {
		Encoder []float64 `json:"encoder"`
		Readout []float64 `json:"readout"`
	}{encoder, readout})
}

// multiTaskCorridorRows lays corridor observations out as core input rows:
// each observation fills channels 0..3, channel 4 stays zero, and every
// observation is held for multiTaskSettle consecutive rows.
func multiTaskCorridorRows(obs [][]float64) [][]float64 {
	wide := make([][]float64, len(obs))
	for k, o := range obs {
		wide[k] = make([]float64, multiTaskInputs)
		copy(wide[k][:imitationInputs], o)
	}
	return nav2dRepeat(wide, multiTaskSettle)
}

// multiTaskDelayedRows lays a delayed episode out as core input rows: each
// one-value input row goes to channel 4 and channels 0..3 stay zero. The
// episode needs at least one input row, one value per row and exactly one
// target.
func multiTaskDelayedRows(ep Episode) ([][]float64, error) {
	if len(ep.Input) == 0 || len(ep.Target) != 1 {
		return nil, fmt.Errorf("delayed episode has %d input rows and %d targets, want at least one row and exactly one target", len(ep.Input), len(ep.Target))
	}
	rows := make([][]float64, len(ep.Input))
	for t, in := range ep.Input {
		if len(in) != 1 {
			return nil, fmt.Errorf("delayed input row %d has %d values, want 1", t, len(in))
		}
		rows[t] = make([]float64, multiTaskInputs)
		rows[t][multiTaskDelayedChannel] = in[0]
	}
	return rows, nil
}
