package learning_test

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

// lifCore is the fixture spiking topology: neuron 0 receives the encoded
// observation, neuron 1 integrates it through one instantaneous and one
// delayed self edge, and neuron 2 is the only readout neuron.
func lifCore() dynamics.LIFConfig {
	return dynamics.LIFConfig{
		Nodes: 3, Sources: []int{0, 1, 1}, Targets: []int{1, 1, 2}, Delays: []int{0, 1, 0},
		DT: 1, TauSyn: 1, ThetaMin: .05, ThetaMax: 1, VReset: -.5, RefractorySteps: 1,
		Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
	}
}

func lifConfig() learning.Config {
	core := lifCore()
	return learning.Config{LIF: &core, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{2}}
}

func lifParameters() learning.Parameters {
	return learning.Parameters{
		Core:     dynamics.Parameters{Weights: []float64{.65, .25, .7}, Bias: []float64{0, 0, 0}, LogTau: []float64{0, 0, 0}},
		ThetaRaw: []float64{-1, -1, -1},
		Encoder:  []float64{1, 0, 0},
		Readout:  []float64{1},
	}
}

// lifInput is a five-step pulse; only the first step carries the observation.
func lifInput() [][]float64 { return [][]float64{{1}, {0}, {0}, {0}, {0}} }

func continuousConfig() learning.Config {
	return learning.Config{
		Dynamics:  dynamics.Config{Nodes: 2, Sources: []int{0, 1}, Targets: []int{1, 0}, DT: .5, Activation: "tanh"},
		InputSize: 1, OutputSize: 1, ReadoutNodes: []int{1},
	}
}

func continuousParameters() learning.Parameters {
	return learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{.3, -.2}, Bias: []float64{.1, -.1}, LogTau: []float64{.1, .2}},
		Encoder: []float64{.5, .1},
		Readout: []float64{.8},
	}
}

func TestNewNetworkRequiresExactlyOneCore(t *testing.T) {
	if _, err := learning.NewNetwork(learning.Config{InputSize: 1, OutputSize: 1, ReadoutNodes: []int{0}}); err == nil {
		t.Fatal("accepted a configuration without any core")
	}
	both := lifConfig()
	both.Dynamics = dynamics.Config{Nodes: 3, DT: 1, Activation: "tanh"}
	if _, err := learning.NewNetwork(both); err == nil {
		t.Fatal("accepted both cores at once")
	}
	partial := lifConfig()
	partial.Dynamics = dynamics.Config{Sources: []int{0}}
	if _, err := learning.NewNetwork(partial); err == nil {
		t.Fatal("accepted a partially populated continuous configuration next to LIF")
	}
	empty := lifConfig()
	empty.Dynamics = dynamics.Config{Sources: []int{}, Targets: []int{}, Delays: []int{}}
	if _, err := learning.NewNetwork(empty); err != nil {
		t.Fatalf("rejected empty (non-nil) continuous slices next to LIF: %v", err)
	}
	bad := lifConfig()
	bad.ReadoutNodes = []int{3}
	if _, err := learning.NewNetwork(bad); err == nil {
		t.Fatal("accepted a readout neuron outside the LIF node count")
	}
	badInput := lifConfig()
	badInput.InputNodes = []int{3}
	if _, err := learning.NewNetwork(badInput); err == nil {
		t.Fatal("accepted an input neuron outside the LIF node count")
	}
	if _, err := learning.NewNetwork(lifConfig()); err != nil {
		t.Fatalf("rejected a valid LIF configuration: %v", err)
	}
}

func TestLIFConfigIsCopiedAndIndependent(t *testing.T) {
	source := lifCore()
	c := learning.Config{LIF: &source, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{2}}
	n, err := learning.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	source.Sources[0] = 2
	source.ThetaMin = .9
	got := n.Config()
	if got.LIF == &source || got.LIF.Sources[0] != 0 || got.LIF.ThetaMin != .05 {
		t.Fatalf("network aliases the caller configuration: %+v", got.LIF)
	}
	got.LIF.Sources[0] = 1
	got.LIF.ThetaMax = 7
	again := n.Config()
	if again.LIF.Sources[0] != 0 || again.LIF.ThetaMax != 1 {
		t.Fatalf("returned configuration aliases the network: %+v", again.LIF)
	}
	if !reflect.DeepEqual(again.Dynamics, dynamics.Config{}) {
		t.Fatalf("LIF network reported a continuous configuration: %+v", again.Dynamics)
	}
}

func TestThetaRawAndThetaMaskValidation(t *testing.T) {
	ctx := context.Background()
	continuousWithTheta := continuousParameters()
	continuousWithTheta.ThetaRaw = []float64{0, 0}
	if _, err := learning.NewTrainer(continuousConfig(), continuousWithTheta, learning.DefaultOptions()); err == nil {
		t.Fatal("continuous trainer accepted theta_raw")
	}
	cn, err := learning.NewNetwork(continuousConfig())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cn.Predict(ctx, continuousWithTheta, [][]float64{{1}}); err == nil {
		t.Fatal("continuous network accepted theta_raw")
	}
	for _, theta := range [][]float64{nil, {}, {0, 0}, {0, 0, 0, 0}} {
		p := lifParameters()
		p.ThetaRaw = theta
		if _, err := learning.NewTrainer(lifConfig(), p, learning.DefaultOptions()); err == nil {
			t.Fatalf("LIF trainer accepted theta_raw of length %d", len(theta))
		}
	}
	theta := learning.DefaultOptions()
	theta.Trainable.Theta = true
	if _, err := learning.NewTrainer(continuousConfig(), continuousParameters(), theta); err == nil {
		t.Fatal("continuous trainer accepted a trainable theta group")
	}
	if _, err := learning.NewTrainer(lifConfig(), lifParameters(), theta); err != nil {
		t.Fatalf("LIF trainer rejected a trainable theta group: %v", err)
	}
	if learning.DefaultOptions().Trainable.Theta {
		t.Fatal("DefaultOptions must keep the theta group frozen")
	}
}

// referenceTrace runs the fixture directly on the dynamics core so the
// learning bridge is compared against an independent composition.
func referenceTrace(t *testing.T, p learning.Parameters, input [][]float64) (*dynamics.LIFTrace, *dynamics.LIF) {
	t.Helper()
	core := lifCore()
	model, err := dynamics.NewLIF(core)
	if err != nil {
		t.Fatal(err)
	}
	coreInputs := make([][]float64, len(input))
	for i, row := range input {
		coreInputs[i] = []float64{row[0] * p.Encoder[0], row[0] * p.Encoder[1], row[0] * p.Encoder[2]}
	}
	trace, err := model.Forward(context.Background(), dynamics.LIFParameters{
		Weights: p.Core.Weights, Bias: p.Core.Bias, LogTau: p.Core.LogTau, ThetaRaw: p.ThetaRaw,
	}, make([]float64, core.Nodes), coreInputs)
	if err != nil {
		t.Fatal(err)
	}
	return trace, model
}

func TestLIFPredictMatchesSynapticTraceReadout(t *testing.T) {
	n, err := learning.NewNetwork(lifConfig())
	if err != nil {
		t.Fatal(err)
	}
	p, input := lifParameters(), lifInput()
	got, err := n.Predict(context.Background(), p, input)
	if err != nil {
		t.Fatal(err)
	}
	trace, _ := referenceTrace(t, p, input)
	outputs := trace.Outputs()
	want := outputs[len(outputs)-1][2] * p.Readout[0]
	if len(got) != 1 || math.Abs(got[0]-want) > 1e-6+1e-6*math.Abs(want) {
		t.Fatalf("readout = %v, want %g from the last-step synaptic trace", got, want)
	}
	if want == 0 {
		t.Fatal("fixture never spikes; the readout carries no event")
	}
	spikes := trace.Spikes()
	var events float64
	for _, row := range spikes {
		for _, v := range row {
			if v != 0 && v != 1 {
				t.Fatalf("non-binary event %g", v)
			}
			events += v
		}
	}
	if events == 0 {
		t.Fatal("fixture produced no spikes")
	}
}

func TestLIFLossGradientMatchesCoreBackward(t *testing.T) {
	ctx := context.Background()
	n, err := learning.NewNetwork(lifConfig())
	if err != nil {
		t.Fatal(err)
	}
	p, input := lifParameters(), lifInput()
	target := []float64{.4}
	loss, g, err := n.LossGradient(ctx, p, input, target, 0)
	if err != nil {
		t.Fatal(err)
	}
	trace, model := referenceTrace(t, p, input)
	outputs := trace.Outputs()
	prediction := outputs[len(outputs)-1][2] * p.Readout[0]
	if math.Abs(loss-(prediction-target[0])*(prediction-target[0])) > 1e-6 {
		t.Fatalf("loss = %g, want %g", loss, (prediction-target[0])*(prediction-target[0]))
	}
	upstream := make([][]float64, len(input))
	for i := range upstream {
		upstream[i] = make([]float64, 3)
	}
	upstream[len(upstream)-1][2] = 2 * (prediction - target[0]) * p.Readout[0]
	cg, err := model.Backward(ctx, trace, upstream, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range []struct {
		name      string
		got, want []float64
	}{
		{"weights", g.Core.Weights, cg.Weights},
		{"bias", g.Core.Bias, cg.Bias},
		{"log_tau", g.Core.LogTau, cg.LogTau},
		{"theta_raw", g.ThetaRaw, cg.ThetaRaw},
		{"initial", g.Core.Initial, cg.Initial},
	} {
		if len(pair.got) != len(pair.want) {
			t.Fatalf("%s gradient has %d values, want %d", pair.name, len(pair.got), len(pair.want))
		}
		for i, want := range pair.want {
			if math.Abs(pair.got[i]-want) > 1e-7+1e-4*math.Abs(want) {
				t.Fatalf("%s[%d] = %g, want %g", pair.name, i, pair.got[i], want)
			}
		}
	}
	var thetaMagnitude float64
	for _, v := range g.ThetaRaw {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("non-finite theta gradient %g", v)
		}
		thetaMagnitude += math.Abs(v)
	}
	if thetaMagnitude == 0 {
		t.Fatal("spiking episode produced a zero theta gradient")
	}
}

// A neuron with no path to the readout must receive exactly zero threshold
// gradient. The declared surrogate is positive everywhere, so a silent neuron
// that still reaches the readout keeps a nonzero gradient by design.
func TestLIFThetaGradientIsZeroWithoutAReadoutPath(t *testing.T) {
	c := lifConfig()
	core := lifCore()
	core.Nodes = 4
	c.LIF = &core
	n, err := learning.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	p := lifParameters()
	p.Core.Bias = []float64{0, 0, 0, 0}
	p.Core.LogTau = []float64{0, 0, 0, 0}
	p.ThetaRaw = []float64{-1, -1, -1, -1}
	p.Encoder = []float64{1, 0, 0, 0}
	_, g, err := n.LossGradient(context.Background(), p, lifInput(), []float64{.4}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if g.ThetaRaw[3] != 0 {
		t.Fatalf("disconnected neuron theta gradient = %g, want 0", g.ThetaRaw[3])
	}
	if g.ThetaRaw[0] == 0 || g.ThetaRaw[2] == 0 {
		t.Fatalf("connected neurons lost their threshold gradient: %v", g.ThetaRaw)
	}
}

func TestLIFThetaOnlyStepChangesOnlyTheta(t *testing.T) {
	o := learning.DefaultOptions()
	o.LearningRate = .05
	o.Trainable = learning.Trainable{Theta: true}
	tr, err := learning.NewTrainer(lifConfig(), lifParameters(), o)
	if err != nil {
		t.Fatal(err)
	}
	before := tr.Snapshot()
	if _, err := tr.Step(context.Background(), lifInput(), []float64{.4}); err != nil {
		t.Fatal(err)
	}
	after := tr.Snapshot()
	if reflect.DeepEqual(before.Parameters.ThetaRaw, after.Parameters.ThetaRaw) {
		t.Fatalf("theta_raw did not move: %v", after.Parameters.ThetaRaw)
	}
	for _, pair := range []struct {
		name      string
		got, want []float64
	}{
		{"weights", after.Parameters.Core.Weights, before.Parameters.Core.Weights},
		{"bias", after.Parameters.Core.Bias, before.Parameters.Core.Bias},
		{"log_tau", after.Parameters.Core.LogTau, before.Parameters.Core.LogTau},
		{"encoder", after.Parameters.Encoder, before.Parameters.Encoder},
		{"readout", after.Parameters.Readout, before.Parameters.Readout},
	} {
		if !reflect.DeepEqual(pair.got, pair.want) {
			t.Fatalf("frozen %s changed: %v -> %v", pair.name, pair.want, pair.got)
		}
	}
	// Flat layout: weights[0:3] bias[3:6] log_tau[6:9] theta_raw[9:12]
	// encoder[12:15] readout[15:16].
	if len(after.Optimizer.Steps) != 16 {
		t.Fatalf("optimizer covers %d parameters, want 16", len(after.Optimizer.Steps))
	}
	for i, steps := range after.Optimizer.Steps {
		want := uint64(0)
		if i >= 9 && i < 12 {
			want = 1
		}
		if steps != want {
			t.Fatalf("optimizer step count at %d = %d, want %d", i, steps, want)
		}
		if want == 0 && (after.Optimizer.First[i] != 0 || after.Optimizer.Second[i] != 0) {
			t.Fatalf("frozen moment at %d moved", i)
		}
	}
}

func TestLIFSnapshotRoundTrip(t *testing.T) {
	o := learning.DefaultOptions()
	o.Trainable = learning.Trainable{Theta: true, Weights: true}
	tr, err := learning.NewTrainer(lifConfig(), lifParameters(), o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Step(context.Background(), lifInput(), []float64{.4}); err != nil {
		t.Fatal(err)
	}
	snapshot := tr.Snapshot()
	if snapshot.SchemaVersion != "coimnet-episode-training/v1" {
		t.Fatalf("schema = %q", snapshot.SchemaVersion)
	}
	restored, err := learning.RestoreTrainer(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	again := restored.Snapshot()
	if !reflect.DeepEqual(snapshot, again) {
		t.Fatalf("snapshot round trip changed state:\n%+v\n%+v", snapshot, again)
	}
	if again.Config.LIF == nil || len(again.Parameters.ThetaRaw) != 3 || !again.Options.Trainable.Theta {
		t.Fatalf("restored snapshot lost LIF fields: %+v", again)
	}
	snapshot.Config.LIF.ThetaMin = .5
	snapshot.Parameters.ThetaRaw[0] = 42
	if restored.Snapshot().Config.LIF.ThetaMin != .05 || restored.Snapshot().Parameters.ThetaRaw[0] == 42 {
		t.Fatal("snapshot aliases trainer state")
	}
}

// lifHomeostasisConfig is the same spiking fixture with the slow stabiliser
// switched on, so the individual has to carry its rate estimate and threshold
// offset across calls as well.
func lifHomeostasisConfig() learning.Config {
	core := lifCore()
	core.Homeostasis = &dynamics.LIFHomeostasis{Enabled: true, TauRate: 2, TargetRate: .3, Eta: 1.5, HMax: .4}
	return learning.Config{LIF: &core, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{2}}
}

// lifReadoutReference runs dynamics.LIF.Forward over the whole sequence and
// returns what an individual's readout must produce for it. The encoder is the
// identity onto neuron 0 and the readout is 1 on neuron 2, so the only
// transformation left is Insyra's float32 boundary.
func lifReadoutReference(t *testing.T, c learning.Config, p learning.Parameters, input [][]float64) [][]float64 {
	t.Helper()
	m, err := dynamics.NewLIF(*c.LIF)
	if err != nil {
		t.Fatal(err)
	}
	coreInputs := make([][]float64, len(input))
	for step, row := range input {
		coreInputs[step] = []float64{row[0], 0, 0}
	}
	tr, err := m.Forward(context.Background(), dynamics.LIFParameters{
		Weights: p.Core.Weights, Bias: p.Core.Bias, LogTau: p.Core.LogTau, ThetaRaw: p.ThetaRaw,
	}, make([]float64, c.LIF.Nodes), coreInputs)
	if err != nil {
		t.Fatal(err)
	}
	want := make([][]float64, len(input))
	for step, row := range tr.Outputs() {
		want[step] = []float64{float64(float32(row[c.ReadoutNodes[0]]))}
	}
	return want
}

// TestLIFIndividualAdvanceMatchesForward is the contract of the new profile:
// continuing a persistent spiking individual in two chunks must equal one
// dynamics.LIF.Forward over the concatenated sequence, with the slow stabiliser
// both off and on.
func TestLIFIndividualAdvanceMatchesForward(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config learning.Config
	}{{"homeostasis_off", lifConfig()}, {"homeostasis_on", lifHomeostasisConfig()}} {
		t.Run(tc.name, func(t *testing.T) {
			p := lifParameters()
			input := [][]float64{{1}, {0}, {1}, {0}, {0}, {1}}
			want := lifReadoutReference(t, tc.config, p, input)
			a, err := learning.NewIndividual(tc.config, p, learning.DefaultOptions(), make([]float64, tc.config.LIF.Nodes))
			if err != nil {
				t.Fatal(err)
			}
			var got [][]float64
			for _, part := range [][][]float64{input[:2], input[2:]} {
				out, err := a.Advance(context.Background(), part)
				if err != nil {
					t.Fatal(err)
				}
				got = append(got, out...)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("persistent readout differs from Forward:\n got %v\nwant %v", got, want)
			}
			s := a.Snapshot()
			if s.Profile != learning.IndividualProfileLIF {
				t.Fatalf("profile = %q", s.Profile)
			}
			if s.Neural.Core != learning.NeuralCoreLIF || s.Neural.LIF == nil || s.Neural.Continuous != nil {
				t.Fatalf("neural union = %+v", s.Neural)
			}
			if s.Neural.LIF.Steps != uint64(len(input)) {
				t.Fatalf("steps = %d, want %d", s.Neural.LIF.Steps, len(input))
			}
			stabilising := tc.config.LIF.Homeostasis != nil && tc.config.LIF.Homeostasis.Enabled
			if stabilising != (s.Neural.LIF.Rate != nil) || stabilising != (s.Neural.LIF.Homeostasis != nil) {
				t.Fatalf("homeostasis arrays do not follow the declared mechanism: %+v", s.Neural.LIF)
			}
			if stabilising {
				var raised bool
				for _, h := range s.Neural.LIF.Homeostasis {
					if h > 0 {
						raised = true
					}
				}
				if !raised {
					t.Fatal("the stabilised fixture never raised a threshold offset")
				}
			}
			// A fresh restore of the same snapshot continues the same way.
			b, err := learning.RestoreIndividual(s)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(b.Snapshot(), s) {
				t.Fatal("snapshot round trip changed the individual")
			}
			extra := [][]float64{{0}, {1}}
			fromA, err := a.Advance(context.Background(), extra)
			if err != nil {
				t.Fatal(err)
			}
			fromB, err := b.Advance(context.Background(), extra)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(fromA, fromB) {
				t.Fatalf("restored individual diverged: %v vs %v", fromA, fromB)
			}
			longer := lifReadoutReference(t, tc.config, p, append(append([][]float64{}, input...), extra...))
			if !reflect.DeepEqual(append(got, fromA...), longer) {
				t.Fatal("continuing after a snapshot left the single Forward trajectory")
			}
		})
	}
}

// TestLIFIndividualSnapshotOwnsItsBuffersAndRejectsMismatches covers the union:
// a restored individual shares nothing with the snapshot, and a snapshot whose
// declared core does not match its configuration is refused.
func TestLIFIndividualSnapshotOwnsItsBuffersAndRejectsMismatches(t *testing.T) {
	c, p := lifHomeostasisConfig(), lifParameters()
	a, err := learning.NewIndividual(c, p, learning.DefaultOptions(), make([]float64, c.LIF.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Advance(context.Background(), lifInput()); err != nil {
		t.Fatal(err)
	}
	s := a.Snapshot()
	b, err := learning.RestoreIndividual(s)
	if err != nil {
		t.Fatal(err)
	}
	s.Neural.LIF.Voltage[0] = 99
	s.Neural.LIF.Homeostasis[0] = .1
	s.Parameters.ThetaRaw[0] = 99
	if !reflect.DeepEqual(a.Snapshot(), b.Snapshot()) {
		t.Fatal("restore retained aliases into the snapshot")
	}
	for name, mutate := range map[string]func(s *learning.IndividualSnapshot){
		"continuous profile": func(s *learning.IndividualSnapshot) { s.Profile = learning.IndividualProfile },
		"continuous core":    func(s *learning.IndividualSnapshot) { s.Neural.Core = learning.NeuralCoreContinuous },
		"missing lif state":  func(s *learning.IndividualSnapshot) { s.Neural.LIF = nil },
		"both cores": func(s *learning.IndividualSnapshot) {
			s.Neural.Continuous = &dynamics.State{SchemaVersion: dynamics.ContinuousStateVersion}
		},
		"foreign offsets": func(s *learning.IndividualSnapshot) {
			s.Neural.LIF.Homeostasis = make([]float64, len(s.Neural.LIF.Homeostasis)+1)
		},
		"offset above ceiling": func(s *learning.IndividualSnapshot) {
			s.Neural.LIF.Homeostasis[0] = s.Config.LIF.Homeostasis.HMax + 1
		},
	} {
		t.Run(name, func(t *testing.T) {
			bad := a.Snapshot()
			mutate(&bad)
			if _, err := learning.RestoreIndividual(bad); err == nil {
				t.Fatalf("accepted %s", name)
			}
		})
	}
}

// TestLIFIndividualTrainsThresholdAndResetsNeural checks that the episode
// trainer of a spiking individual moves theta_raw, that training leaves the
// persistent state alone, and that ResetNeural starts a new trajectory without
// touching parameters or the optimizer.
func TestLIFIndividualTrainsThresholdAndResetsNeural(t *testing.T) {
	c, p := lifConfig(), lifParameters()
	o := learning.DefaultOptions()
	o.Trainable = learning.Trainable{Theta: true}
	a, err := learning.NewIndividual(c, p, o, make([]float64, c.LIF.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Advance(context.Background(), lifInput()); err != nil {
		t.Fatal(err)
	}
	advanced := a.Snapshot()
	for range 20 {
		if _, err = a.TrainEpisode(context.Background(), lifInput(), []float64{.4}); err != nil {
			t.Fatal(err)
		}
	}
	trained := a.Snapshot()
	if reflect.DeepEqual(trained.Parameters.ThetaRaw, advanced.Parameters.ThetaRaw) {
		t.Fatal("episode training did not move theta_raw")
	}
	if !reflect.DeepEqual(trained.Parameters.Core, advanced.Parameters.Core) || !reflect.DeepEqual(trained.Parameters.Encoder, advanced.Parameters.Encoder) {
		t.Fatal("a theta-only optimizer changed another group")
	}
	if !reflect.DeepEqual(trained.Neural, advanced.Neural) {
		t.Fatal("episode training changed the persistent spiking state")
	}
	if trained.Optimizer.Updates != 20 {
		t.Fatalf("updates = %d", trained.Optimizer.Updates)
	}
	if err = a.ResetNeural(context.Background(), make([]float64, c.LIF.Nodes)); err != nil {
		t.Fatal(err)
	}
	reset := a.Snapshot()
	if reset.Neural.Core != learning.NeuralCoreLIF || reset.Neural.LIF == nil || reset.Neural.LIF.Steps != 0 {
		t.Fatalf("neural reset did not restart the spiking trajectory: %+v", reset.Neural)
	}
	if !reflect.DeepEqual(reset.Parameters, trained.Parameters) || !reflect.DeepEqual(reset.Optimizer, trained.Optimizer) {
		t.Fatal("neural reset crossed ownership")
	}
	if err = a.ResetNeural(context.Background(), make([]float64, c.LIF.Nodes+1)); err == nil {
		t.Fatal("accepted an initial voltage of the wrong width")
	}
}

func TestTrainerSpikesRequireALIFCore(t *testing.T) {
	ctx := context.Background()
	continuous, err := learning.NewTrainer(continuousConfig(), continuousParameters(), learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := continuous.Spikes(ctx, [][]float64{{1}}); err == nil {
		t.Fatal("continuous trainer reported spikes")
	}
	tr, err := learning.NewTrainer(lifConfig(), lifParameters(), learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	spikes, err := tr.Spikes(ctx, lifInput())
	if err != nil {
		t.Fatal(err)
	}
	if len(spikes) != len(lifInput()) {
		t.Fatalf("spikes have %d steps, want %d", len(spikes), len(lifInput()))
	}
	var events float64
	for _, row := range spikes {
		if len(row) != 3 {
			t.Fatalf("spike row width %d, want 3", len(row))
		}
		for _, v := range row {
			if v != 0 && v != 1 {
				t.Fatalf("non-binary spike %g", v)
			}
			events += v
		}
	}
	if events == 0 {
		t.Fatal("fixture episode produced no spikes")
	}
	if _, err := tr.Spikes(ctx, nil); err == nil {
		t.Fatal("accepted an empty episode")
	}
}
