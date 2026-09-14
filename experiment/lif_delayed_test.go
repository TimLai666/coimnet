package experiment_test

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/experiment"
	"github.com/TimLai666/coimnet/learning"
)

func TestDelayedLIFTrainerTopologyAndSteps(t *testing.T) {
	tr, err := experiment.NewDelayedLIFTrainer(7, .02, learning.Trainable{Theta: true})
	if err != nil {
		t.Fatal(err)
	}
	s := tr.Snapshot()
	if s.Config.LIF == nil {
		t.Fatal("trainer has no LIF core")
	}
	if !reflect.DeepEqual(s.Config.Dynamics, dynamics.Config{}) {
		t.Fatalf("continuous configuration is not zero: %+v", s.Config.Dynamics)
	}
	if s.Config.LIF.Nodes != 3 || len(s.Config.LIF.Sources) != len(s.Config.LIF.Targets) {
		t.Fatalf("unexpected topology: %+v", s.Config.LIF)
	}
	delayed := 0
	for _, d := range s.Config.LIF.Delays {
		if d > 0 {
			delayed++
		}
	}
	if delayed == 0 {
		t.Fatalf("no delayed edge: %v", s.Config.LIF.Delays)
	}
	if len(s.Parameters.ThetaRaw) != 3 || len(s.Config.ReadoutNodes) != 1 {
		t.Fatalf("unexpected parameters: %+v", s.Parameters)
	}
	if s.Config.LIF.VReset >= s.Config.LIF.ThetaMin || s.Config.LIF.ThetaMin >= s.Config.LIF.ThetaMax {
		t.Fatalf("threshold configuration is invalid: %+v", s.Config.LIF)
	}
	ctx := context.Background()
	for i := uint64(0); i < 5; i++ {
		ep := experiment.DelayedEpisode(1001, i)
		result, err := tr.Step(ctx, ep.Input, ep.Target)
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if math.IsNaN(result.Loss) || math.IsInf(result.Loss, 0) {
			t.Fatalf("step %d loss = %g", i, result.Loss)
		}
	}
	// The untrained model must spike sometimes and stay silent sometimes, so
	// the threshold gate is neither saturated nor dead before training.
	fresh, err := experiment.NewDelayedLIFTrainer(7, .02, learning.Trainable{})
	if err != nil {
		t.Fatal(err)
	}
	var events, total float64
	for i := uint64(0); i < 64; i++ {
		ep := experiment.DelayedEpisode(1003, i)
		spikes, err := fresh.Spikes(ctx, ep.Input)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range spikes {
			for _, v := range row {
				events += v
				total++
			}
		}
	}
	if events == 0 || events == total {
		t.Fatalf("untrained spike rate is %g/%g; the task must be neither silent nor saturated", events, total)
	}
	t.Logf("untrained holdout spike rate = %g", events/total)
}

func TestDelayedLIFTrainerIsDeterministicPerSeed(t *testing.T) {
	first, err := experiment.NewDelayedLIFTrainer(42, .02, learning.Trainable{Theta: true})
	if err != nil {
		t.Fatal(err)
	}
	second, err := experiment.NewDelayedLIFTrainer(42, .02, learning.Trainable{Theta: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.Snapshot(), second.Snapshot()) {
		t.Fatal("same seed produced different trainers")
	}
	other, err := experiment.NewDelayedLIFTrainer(123, .02, learning.Trainable{Theta: true})
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(first.Snapshot().Parameters, other.Snapshot().Parameters) {
		t.Fatal("different seeds produced identical parameters")
	}
	ctx := context.Background()
	for i := uint64(0); i < 3; i++ {
		ep := experiment.DelayedEpisode(1001, i)
		if _, err := first.Step(ctx, ep.Input, ep.Target); err != nil {
			t.Fatal(err)
		}
		if _, err := second.Step(ctx, ep.Input, ep.Target); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(first.Snapshot(), second.Snapshot()) {
		t.Fatal("identical training produced different state")
	}
}

func TestDelayedLIFTrainerRejectsInvalidRates(t *testing.T) {
	for _, rate := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := experiment.NewDelayedLIFTrainer(7, rate, learning.Trainable{Theta: true}); err == nil {
			t.Fatalf("accepted learning rate %g", rate)
		}
	}
	// Every group combination is legal on a LIF core, including the frozen
	// control used by the threshold example.
	for _, trainable := range []learning.Trainable{
		{},
		{Theta: true},
		{Encoder: true, Weights: true, Bias: true, Tau: true, Theta: true, Readout: true},
	} {
		if _, err := experiment.NewDelayedLIFTrainer(7, .02, trainable); err != nil {
			t.Fatalf("rejected trainable %+v: %v", trainable, err)
		}
	}
}
