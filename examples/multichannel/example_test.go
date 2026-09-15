package multichannel_test

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/examples/multichannel"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/signal"
)

// Fixed artificial projection: six features -> two synthetic neurons. Each
// encoder row is [feature->node0, feature->node1]; readout sees only node1.
// No anatomical IDs or biological sensory assignments are implied.
func syntheticModel() (learning.Config, learning.Parameters) {
	return learning.Config{Dynamics: dynamics.Config{Nodes: 2, Sources: []int{0, 1}, Targets: []int{1, 0}, DT: .5, Activation: "tanh"}, InputSize: 6, OutputSize: 1, ReadoutNodes: []int{1}},
		learning.Parameters{Core: dynamics.Parameters{Weights: []float64{.3, -.2}, Bias: []float64{.1, -.1}, LogTau: []float64{.1, .2}}, Encoder: []float64{.5, .1, .05, .02, .3, .2, .04, .01, .4, .3, .03, .02}, Readout: []float64{.8}}
}

func ExampleAdapt() {
	ctx := context.Background()
	input, err := multichannel.Adapt(ctx, fixture(), 1)
	if err != nil {
		panic(err)
	}
	config, parameters := syntheticModel()
	trainer, err := learning.NewTrainer(config, parameters, learning.DefaultOptions())
	if err != nil {
		panic(err)
	}
	before, err := trainer.Predict(ctx, input)
	if err != nil {
		panic(err)
	}
	// The synthetic answer belongs only to the training call, never to Adapt
	// or Predict. Each Step starts a fresh episode, as specified by learning.
	target := []float64{.7}
	for i := 0; i < 50; i++ {
		if _, err := trainer.Step(ctx, input, target); err != nil {
			panic(err)
		}
	}
	after, err := trainer.Predict(ctx, input)
	if err != nil {
		panic(err)
	}
	fmt.Println("steps:", len(input), "features:", len(input[0]))
	fmt.Println("synthetic training error decreased:", math.Abs(after[0]-target[0]) < math.Abs(before[0]-target[0]))
	// Output:
	// steps: 8 features: 6
	// synthetic training error decreased: true
}

func TestAdaptCoreTrainingCompatibility(t *testing.T) {
	ctx := context.Background()
	input, err := multichannel.Adapt(ctx, fixture(), 1)
	if err != nil {
		t.Fatal(err)
	}
	c, p := syntheticModel()
	n, err := learning.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	target := []float64{.7}
	loss, g, err := n.LossGradient(ctx, p, input, target, 0)
	if err != nil {
		t.Fatal(err)
	}
	wantLoss, wantG, err := n.LossGradient(ctx, p, reference(), target, 0)
	if err != nil {
		t.Fatal(err)
	}
	if loss != wantLoss || !reflect.DeepEqual(g, wantG) {
		t.Fatal("adapter gradient differs from native reference input")
	}
	for name, values := range map[string][]float64{"encoder": g.Encoder, "core": g.Core.Weights, "readout": g.Readout, "observations": g.Inputs[1]} {
		norm := 0.
		for _, v := range values {
			norm = math.Hypot(norm, v)
		}
		if norm == 0 || math.IsNaN(norm) || math.IsInf(norm, 0) {
			t.Fatalf("%s gradient invalid: %v", name, values)
		}
	}
	tr, err := learning.NewTrainer(c, p, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	control, err := learning.NewTrainer(c, p, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	before, err := tr.Predict(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		a, err := tr.Step(ctx, input, target)
		if err != nil {
			t.Fatal(err)
		}
		b, err := control.Step(ctx, reference(), target)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("update %d differs", i)
		}
	}
	if !reflect.DeepEqual(tr.Snapshot(), control.Snapshot()) {
		t.Fatal("optimizer/parameters differ")
	}
	if reflect.DeepEqual(p.Core.Weights, tr.Snapshot().Parameters.Core.Weights) {
		t.Fatal("core did not train")
	}
	after, err := tr.Predict(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(after[0]-.7) >= math.Abs(before[0]-.7) {
		t.Fatal("synthetic training error did not decrease")
	}
	if !reflect.DeepEqual(input, reference()) {
		t.Fatal("training mutated adapted input")
	}
	// Changing the answer changes the training objective, not inference inputs
	// or predictions made from the same parameter version.
	otherLoss, _, err := n.LossGradient(ctx, p, input, []float64{-.7}, 0)
	if err != nil {
		t.Fatal(err)
	}
	again, err := n.Predict(ctx, p, input)
	if err != nil {
		t.Fatal(err)
	}
	if otherLoss == loss || !reflect.DeepEqual(before, again) {
		t.Fatal("target separation failed")
	}
	t.Logf("synthetic probe: loss=%g prediction_before=%g prediction_after=%g updates=50", loss, before[0], after[0])
}

func TestAdaptMetadataAndAggregateErrors(t *testing.T) {
	changes := map[string]func(*signal.SignalSpec){
		"unit":                func(s *signal.SignalSpec) { s.Unit = signal.UnitVolt },
		"time unit":           func(s *signal.SignalSpec) { s.Start.Unit = signal.TimeUnitMilliseconds },
		"encoder":             func(s *signal.SignalSpec) { s.EncoderVersion = signal.Version{Major: 2} },
		"shape":               func(s *signal.SignalSpec) { s.Shape = []int{1, 1} },
		"channel":             func(s *signal.SignalSpec) { s.Channel = "other" },
		"sequence regression": func(s *signal.SignalSpec) { s.SourceSequence = 0 },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			input := fixture()
			samples := input.Level.Signals()
			spec := samples[1].Spec()
			change(&spec)
			changed, err := signal.NewSignal(spec)
			if err != nil {
				t.Fatal(err)
			}
			samples[1] = changed
			// A uniformly unsupported time unit must reach Adapt. Mixed units
			// are already rejected by the Observation constructor.
			if name == "time unit" {
				for i, source := range samples {
					spec := source.Spec()
					change(&spec)
					samples[i], err = signal.NewSignal(spec)
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			input.Level, err = signal.NewObservation(signal.CurrentSchemaVersion(), "synthetic", "level", samples)
			if err != nil {
				t.Fatal(err)
			}
			got, err := multichannel.Adapt(context.Background(), input, 1)
			if err == nil || got != nil {
				t.Fatal("incompatible metadata accepted")
			}
		})
	}
	input := fixture()
	var err error
	input.Gate, err = signal.NewObservation(signal.CurrentSchemaVersion(), "another", "gate", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := multichannel.Adapt(context.Background(), input, 1); err == nil || got != nil {
		t.Fatal("cross-experience input accepted")
	}
	input = fixture()
	input.Pulses = observation("pulses", signal.KindPulse, [][3]float64{{1, 0, math.MaxFloat64}, {2, 0, math.MaxFloat64}})
	if got, err := multichannel.Adapt(context.Background(), input, 1); err == nil || got != nil {
		t.Fatal("overflowing pulse sum accepted")
	}
}
