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

func TestLIFIndividualsAreRejected(t *testing.T) {
	const want = "LIF individuals are not supported yet"
	_, err := learning.NewIndividual(lifConfig(), lifParameters(), learning.DefaultOptions(), make([]float64, 3))
	if err == nil || err.Error() != want {
		t.Fatalf("NewIndividual error = %v, want %q", err, want)
	}
	tr, err := learning.NewTrainer(lifConfig(), lifParameters(), learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	s := tr.Snapshot()
	_, err = learning.RestoreIndividual(learning.IndividualSnapshot{
		SchemaVersion: learning.IndividualVersion, Profile: learning.IndividualProfile,
		Config: s.Config, Parameters: s.Parameters,
		Optimizer: learning.OptimizerSnapshot{Options: s.Options, State: s.Optimizer, Updates: s.Updates},
	})
	if err == nil || err.Error() != want {
		t.Fatalf("RestoreIndividual error = %v, want %q", err, want)
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
