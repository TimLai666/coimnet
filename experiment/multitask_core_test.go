package experiment

import (
	"context"
	"math"
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/experiment/gridnav"
	"github.com/TimLai666/coimnet/learning"
)

// multiTaskTestCorridor is the length-7 corridor of the imitation fixture. The
// tests below spell the shared layout out as literals (5 input channels, 4
// outputs) so they check it rather than reuse the implementation's constants.
var multiTaskTestCorridor = gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: 0.01, GoalReward: 1}

const (
	multiTaskTestSeed   uint64 = 1
	multiTaskTestHidden        = 16
	multiTaskTestRate          = 0.05
	// multiTaskTestTrainSeed is the seed of the training test, the smallest
	// seed whose untrained corridor policy falls short of the expert. With
	// seed 1 the untrained core already agrees with the expert on every
	// evaluation step: bias 0 makes the first decision exactly antisymmetric
	// in the cue, seed 1 ranks right highest and left lowest for cue +1, and
	// the later steps keep that direction, so the agreement has no room to
	// rise (it stays at 1 after 200, 400 and 600 alternations).
	multiTaskTestTrainSeed uint64 = 2
	// multiTaskTestEval is the evaluation batch of both tasks.
	multiTaskTestEval = 20
	// multiTaskTestAlternations is how many corridor-then-delayed update pairs
	// the shared core receives.
	multiTaskTestAlternations = 200
	// multiTaskTestDelayedTrainSeed is the delayed task's training split, the
	// TrainSeed of DefaultDelayedConfig.
	multiTaskTestDelayedTrainSeed uint64 = 1001
)

// newMultiTaskTestModel builds the fixture model for seed or stops the test.
func newMultiTaskTestModel(t *testing.T, seed uint64) *multiTaskModel {
	t.Helper()
	m, err := newMultiTaskModel(seed, multiTaskTestHidden, multiTaskTestRate)
	if err != nil {
		t.Fatalf("newMultiTaskModel: %v", err)
	}
	return m
}

// multiTaskTestResult is what one task's result record carries: its score and
// the fingerprints read right after that task was scored.
type multiTaskTestResult struct {
	score                   float64
	topology, base, adapter string
}

// multiTaskTestEvaluate scores the corridor and then the delayed task, reading
// the fingerprints after each score as two separate result records would.
func multiTaskTestEvaluate(t *testing.T, m *multiTaskModel) (corridor, delayed multiTaskTestResult) {
	t.Helper()
	ctx := context.Background()
	score, err := m.corridorScore(ctx, multiTaskTestCorridor, multiTaskTestEval)
	if err != nil {
		t.Fatalf("corridorScore: %v", err)
	}
	corridor = multiTaskTestResult{score, m.topologyHash(), m.baseParameterHash(), m.adapterVersion(MultiTaskCorridor)}
	if score, err = m.delayedScore(ctx, multiTaskTestEval); err != nil {
		t.Fatalf("delayedScore: %v", err)
	}
	delayed = multiTaskTestResult{score, m.topologyHash(), m.baseParameterHash(), m.adapterVersion(MultiTaskDelayed)}
	return corridor, delayed
}

// multiTaskTestAdapterBits returns the bit patterns of the parameters a task
// owns, read without adapterVersion: encoder rows 0..3 and readout columns
// 0..2 for the corridor, encoder row 4 and readout column 3 for the delayed
// task, every readout row (hidden rows included) of each column.
func multiTaskTestAdapterBits(t *testing.T, p learning.Parameters, task string) (encoder, readout []uint64) {
	t.Helper()
	var rows, columns []int
	switch task {
	case MultiTaskCorridor:
		rows, columns = []int{0, 1, 2, 3}, []int{0, 1, 2}
	case MultiTaskDelayed:
		rows, columns = []int{4}, []int{3}
	default:
		t.Fatalf("unknown task %q", task)
	}
	for _, r := range rows {
		for k := 0; k < 5; k++ {
			encoder = append(encoder, math.Float64bits(p.Encoder[r*5+k]))
		}
	}
	for _, j := range columns {
		for r := 0; r < len(p.Readout)/4; r++ {
			readout = append(readout, math.Float64bits(p.Readout[r*4+j]))
		}
	}
	return encoder, readout
}

// TestMultiTaskCoreLayout pins the core the constructor declares: 5 input
// nodes, the hidden block and 4 readout nodes; an identity encoder; a readout
// over the hidden nodes and then the readout nodes whose hidden rows start at
// zero; edges from every input and hidden node to every hidden and readout
// node; the seeded initial values; and the trainable groups with the bias
// frozen at zero.
func TestMultiTaskCoreLayout(t *testing.T) {
	m := newMultiTaskTestModel(t, multiTaskTestSeed)
	s := m.trainer.Snapshot()
	c, p := s.Config, s.Parameters
	const inputs, hidden, outputs = 5, multiTaskTestHidden, 4
	nodes := inputs + hidden + outputs
	if c.InputSize != inputs || c.OutputSize != outputs || !c.ReadoutEveryStep || c.Dynamics.Nodes != nodes {
		t.Fatalf("input %d, output %d, every step %v, nodes %d; want %d, %d, true, %d", c.InputSize, c.OutputSize, c.ReadoutEveryStep, c.Dynamics.Nodes, inputs, outputs, nodes)
	}
	if want := []int{0, 1, 2, 3, 4}; !reflect.DeepEqual(c.InputNodes, want) {
		t.Errorf("input nodes %v, want %v", c.InputNodes, want)
	}
	var readoutNodes []int
	for n := inputs; n < nodes; n++ {
		readoutNodes = append(readoutNodes, n)
	}
	if !reflect.DeepEqual(c.ReadoutNodes, readoutNodes) {
		t.Errorf("readout nodes %v, want the hidden nodes then the readout nodes %v", c.ReadoutNodes, readoutNodes)
	}
	want, got := map[[2]int]bool{}, map[[2]int]bool{}
	for from := 0; from < inputs+hidden; from++ {
		for to := inputs; to < nodes; to++ {
			want[[2]int{from, to}] = true
		}
	}
	for e, from := range c.Dynamics.Sources {
		edge := [2]int{from, c.Dynamics.Targets[e]}
		if got[edge] {
			t.Errorf("edge %v declared twice", edge)
		}
		got[edge] = true
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%d distinct edges, want the %d from every input and hidden node to every hidden and readout node", len(got), len(want))
	}
	uniform := func(stream uint64, n int) []float64 {
		r := rand.New(rand.NewPCG(multiTaskTestSeed, stream))
		out := make([]float64, n)
		for i := range out {
			out[i] = r.Float64()*0.6 - 0.3
		}
		return out
	}
	if !reflect.DeepEqual(p.Core.Weights, uniform(0, len(c.Dynamics.Sources))) {
		t.Errorf("weights are not uniform [-0.3, 0.3] from PCG(seed, 0) in edge order")
	}
	for i := range p.Core.Bias {
		if p.Core.Bias[i] != 0 || p.Core.LogTau[i] != math.Log(2) {
			t.Errorf("node %d bias %v, log_tau %v; want 0 and log 2", i, p.Core.Bias[i], p.Core.LogTau[i])
		}
	}
	encoder := make([]float64, inputs*inputs)
	for i := 0; i < inputs; i++ {
		encoder[i*inputs+i] = 1
	}
	if !reflect.DeepEqual(p.Encoder, encoder) {
		t.Errorf("encoder %v, want the 5x5 identity", p.Encoder)
	}
	readout := make([]float64, (hidden+outputs)*outputs)
	copy(readout[hidden*outputs:], uniform(2, outputs*outputs))
	if !reflect.DeepEqual(p.Readout, readout) {
		t.Errorf("readout is not zero hidden rows followed by readout-node rows from PCG(seed, 2)")
	}
	if want := (learning.Trainable{Encoder: true, Weights: true, Tau: true, Readout: true}); s.Options.Trainable != want {
		t.Errorf("trainable %+v, want %+v", s.Options.Trainable, want)
	}
	if s.Options.LearningRate != multiTaskTestRate {
		t.Errorf("learning rate %v, want %v", s.Options.LearningRate, multiTaskTestRate)
	}
	if m.built != 1 {
		t.Errorf("built %d trainers, want 1", m.built)
	}
}

// TestMultiTaskCoreBothTasksTrainOneCore is the TSK-09 shared-core claim on the
// fixture: alternating corridor imitation and delayed-association updates on
// the one trainer raise both tasks' scores, no second trainer is built, both
// tasks' result records carry the same topology and base-parameter
// fingerprints, and their adapter versions differ.
func TestMultiTaskCoreBothTasksTrainOneCore(t *testing.T) {
	ctx := context.Background()
	m := newMultiTaskTestModel(t, multiTaskTestTrainSeed)
	corridorBefore, delayedBefore := multiTaskTestEvaluate(t, m)
	for e := 0; e < multiTaskTestAlternations; e++ {
		obs, actions, err := imitationExpertEpisode(multiTaskTestCorridor, imitationTrainSeed(multiTaskTestTrainSeed, e))
		if err != nil {
			t.Fatalf("corridor episode %d: %v", e, err)
		}
		if err := m.corridorStep(ctx, obs, actions, 1); err != nil {
			t.Fatalf("corridorStep %d: %v", e, err)
		}
		if err := m.delayedStep(ctx, DelayedEpisode(multiTaskTestDelayedTrainSeed, uint64(e)), 1); err != nil {
			t.Fatalf("delayedStep %d: %v", e, err)
		}
	}
	corridorAfter, delayedAfter := multiTaskTestEvaluate(t, m)
	t.Logf("corridor expert agreement %v -> %v; delayed -MSE %v -> %v", corridorBefore.score, corridorAfter.score, delayedBefore.score, delayedAfter.score)
	t.Logf("topology %s; base parameters %s -> %s", corridorBefore.topology, corridorBefore.base, corridorAfter.base)
	t.Logf("adapters after training: corridor %s, delayed %s", corridorAfter.adapter, delayedAfter.adapter)
	if corridorAfter.score <= corridorBefore.score {
		t.Errorf("corridor agreement %v did not rise above %v", corridorAfter.score, corridorBefore.score)
	}
	if delayedAfter.score <= delayedBefore.score {
		t.Errorf("delayed -MSE %v did not rise above %v", delayedAfter.score, delayedBefore.score)
	}
	if m.built != 1 {
		t.Errorf("built %d trainers, want 1", m.built)
	}
	if got := m.trainer.Snapshot().Updates; got != 2*multiTaskTestAlternations {
		t.Errorf("trainer applied %d updates, want %d", got, 2*multiTaskTestAlternations)
	}
	for _, phase := range []struct {
		name              string
		corridor, delayed multiTaskTestResult
	}{{"before training", corridorBefore, delayedBefore}, {"after training", corridorAfter, delayedAfter}} {
		if phase.corridor.topology != phase.delayed.topology || phase.corridor.base != phase.delayed.base {
			t.Errorf("%s the two tasks read different brains: topology %s vs %s, base %s vs %s", phase.name, phase.corridor.topology, phase.delayed.topology, phase.corridor.base, phase.delayed.base)
		}
		if phase.corridor.adapter == phase.delayed.adapter {
			t.Errorf("%s both tasks report adapter version %s", phase.name, phase.corridor.adapter)
		}
	}
	if corridorAfter.topology != corridorBefore.topology {
		t.Errorf("training changed the topology fingerprint %s -> %s", corridorBefore.topology, corridorAfter.topology)
	}
	if corridorAfter.base == corridorBefore.base {
		t.Errorf("training left the base parameter fingerprint at %s", corridorBefore.base)
	}
}

// TestMultiTaskCoreStepsTouchOnlyTheirAdapter checks the adapter split on a
// fresh model, whose optimizer moments are still zero: one step of a task
// moves the core and that task's adapter and leaves the other task's adapter
// bit for bit unchanged. The bias stays frozen at zero throughout.
func TestMultiTaskCoreStepsTouchOnlyTheirAdapter(t *testing.T) {
	ctx := context.Background()
	obs, actions, err := imitationExpertEpisode(multiTaskTestCorridor, imitationTrainSeed(multiTaskTestSeed, 0))
	if err != nil {
		t.Fatalf("corridor episode: %v", err)
	}
	for _, tc := range []struct {
		task, other string
		step        func(m *multiTaskModel) error
	}{
		{MultiTaskCorridor, MultiTaskDelayed, func(m *multiTaskModel) error { return m.corridorStep(ctx, obs, actions, 1) }},
		{MultiTaskDelayed, MultiTaskCorridor, func(m *multiTaskModel) error {
			return m.delayedStep(ctx, DelayedEpisode(multiTaskTestDelayedTrainSeed, 0), 1)
		}},
	} {
		t.Run(tc.task, func(t *testing.T) {
			m := newMultiTaskTestModel(t, multiTaskTestSeed)
			before := m.trainer.Snapshot().Parameters
			base, own, other := m.baseParameterHash(), m.adapterVersion(tc.task), m.adapterVersion(tc.other)
			if err := tc.step(m); err != nil {
				t.Fatalf("%s step: %v", tc.task, err)
			}
			after := m.trainer.Snapshot().Parameters
			otherEncoder, otherReadout := multiTaskTestAdapterBits(t, before, tc.other)
			gotEncoder, gotReadout := multiTaskTestAdapterBits(t, after, tc.other)
			if !reflect.DeepEqual(gotEncoder, otherEncoder) || !reflect.DeepEqual(gotReadout, otherReadout) {
				t.Errorf("a %s step changed the %s adapter", tc.task, tc.other)
			}
			if got := m.adapterVersion(tc.other); got != other {
				t.Errorf("a %s step changed the %s adapter version %s -> %s", tc.task, tc.other, other, got)
			}
			ownEncoder, ownReadout := multiTaskTestAdapterBits(t, before, tc.task)
			gotEncoder, gotReadout = multiTaskTestAdapterBits(t, after, tc.task)
			if reflect.DeepEqual(gotEncoder, ownEncoder) {
				t.Errorf("a %s step left its encoder rows unchanged", tc.task)
			}
			if reflect.DeepEqual(gotReadout, ownReadout) {
				t.Errorf("a %s step left its readout columns unchanged", tc.task)
			}
			if got := m.adapterVersion(tc.task); got == own {
				t.Errorf("a %s step left its adapter version at %s", tc.task, own)
			}
			if reflect.DeepEqual(after.Core.Weights, before.Core.Weights) || m.baseParameterHash() == base {
				t.Errorf("a %s step left the shared core weights unchanged", tc.task)
			}
			for i, v := range after.Core.Bias {
				if v != 0 {
					t.Errorf("a %s step moved the frozen bias of node %d to %v", tc.task, i, v)
				}
			}
		})
	}
}

// TestMultiTaskCoreEvaluationIndividualsAreIndependent builds two
// learning.Individual values from the one set of shared parameters and shows
// that each owns its neural state: advancing one leaves the other exactly
// where it was, in both directions, while both stay the same brain as the
// model.
func TestMultiTaskCoreEvaluationIndividualsAreIndependent(t *testing.T) {
	ctx := context.Background()
	m := newMultiTaskTestModel(t, multiTaskTestSeed)
	s := m.trainer.Snapshot()
	zeros := make([]float64, s.Config.Dynamics.Nodes)
	a, err := learning.NewIndividual(s.Config, s.Parameters, s.Options, zeros)
	if err != nil {
		t.Fatalf("first individual: %v", err)
	}
	b, err := learning.NewIndividual(s.Config, s.Parameters, s.Options, zeros)
	if err != nil {
		t.Fatalf("second individual: %v", err)
	}
	for name, ind := range map[string]*learning.Individual{"first": a, "second": b} {
		snap := ind.Snapshot()
		if hash(snap.Config.Dynamics) != m.topologyHash() || hash(snap.Parameters.Core) != m.baseParameterHash() {
			t.Errorf("the %s individual is not the model's brain", name)
		}
	}
	obs, _, err := imitationExpertEpisode(multiTaskTestCorridor, imitationEvalSeed(0))
	if err != nil {
		t.Fatalf("corridor episode: %v", err)
	}
	pulse, err := multiTaskDelayedRows(DelayedEpisode(multiTaskDelayedEvalSeed, 0))
	if err != nil {
		t.Fatalf("delayed rows: %v", err)
	}
	for _, move := range []struct {
		name         string
		moved, still *learning.Individual
		rows         [][]float64
	}{{"first", a, b, multiTaskCorridorRows(obs)}, {"second", b, a, pulse}} {
		movedBefore, stillBefore := move.moved.Snapshot().Neural, move.still.Snapshot().Neural
		if _, err := move.moved.Advance(ctx, move.rows); err != nil {
			t.Fatalf("advance the %s individual: %v", move.name, err)
		}
		if reflect.DeepEqual(move.moved.Snapshot().Neural, movedBefore) {
			t.Fatalf("advancing the %s individual left its neural state unchanged", move.name)
		}
		if !reflect.DeepEqual(move.still.Snapshot().Neural, stillBefore) {
			t.Errorf("advancing the %s individual changed the other individual's neural state", move.name)
		}
	}
	if m.built != 1 {
		t.Errorf("built %d trainers, want 1", m.built)
	}
}

// TestMultiTaskCoreRejectsBadInputs checks the constructor and every task
// entry point refuse malformed input, and that a refused step leaves the
// trainer exactly as it was.
func TestMultiTaskCoreRejectsBadInputs(t *testing.T) {
	ctx := context.Background()
	for _, bad := range []struct {
		name   string
		hidden int
		rate   float64
	}{{"hidden 0", 0, multiTaskTestRate}, {"hidden above the bound", imitationMaxHidden + 1, multiTaskTestRate}, {"rate 0", multiTaskTestHidden, 0}, {"rate NaN", multiTaskTestHidden, math.NaN()}} {
		if _, err := newMultiTaskModel(multiTaskTestSeed, bad.hidden, bad.rate); err == nil {
			t.Errorf("newMultiTaskModel accepted %s", bad.name)
		}
	}
	m := newMultiTaskTestModel(t, multiTaskTestSeed)
	obs, actions, err := imitationExpertEpisode(multiTaskTestCorridor, imitationTrainSeed(multiTaskTestSeed, 0))
	if err != nil {
		t.Fatalf("corridor episode: %v", err)
	}
	wide := append([][]float64{{1, 0, 0, 0, 1}}, obs[1:]...)
	outOfRange := append([]int{gridnav.Actions}, actions[1:]...)
	before := m.trainer.Snapshot()
	for name, call := range map[string]func() error{
		"fewer actions than observations": func() error { return m.corridorStep(ctx, obs, actions[:len(actions)-1], 1) },
		"no observation":                  func() error { return m.corridorStep(ctx, nil, nil, 1) },
		"a five-wide observation":         func() error { return m.corridorStep(ctx, wide, actions, 1) },
		"an action out of range":          func() error { return m.corridorStep(ctx, obs, outOfRange, 1) },
		"a delayed episode without input": func() error { return m.delayedStep(ctx, Episode{Target: []float64{0}}, 1) },
		"a two-wide delayed row": func() error {
			return m.delayedStep(ctx, Episode{Input: [][]float64{{1, 1}}, Target: []float64{0}}, 1)
		},
		"a delayed episode with two targets": func() error {
			return m.delayedStep(ctx, Episode{Input: [][]float64{{1}}, Target: []float64{0, 0}}, 1)
		},
		"a corridor score without episodes": func() error {
			_, err := m.corridorScore(ctx, multiTaskTestCorridor, 0)
			return err
		},
		"a delayed score without episodes": func() error {
			_, err := m.delayedScore(ctx, 0)
			return err
		},
	} {
		if err := call(); err == nil {
			t.Errorf("accepted %s", name)
		}
	}
	if !reflect.DeepEqual(m.trainer.Snapshot(), before) {
		t.Errorf("a refused call changed the trainer")
	}
	if got := m.adapterVersion("unknown"); got != "" {
		t.Errorf("unknown task adapter version %q, want empty", got)
	}
}
