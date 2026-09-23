package experiment_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/experiment"
	"github.com/TimLai666/coimnet/internal/delayedfixture"
	"github.com/TimLai666/coimnet/learning"
)

func TestDelayedFixtureWrappers(t *testing.T) {
	for _, pair := range [][2]uint64{{1001, 0}, {1003, 7}} {
		if got, want := experiment.DelayedEpisode(pair[0], pair[1]), delayedfixture.DelayedEpisode(pair[0], pair[1]); !reflect.DeepEqual(got, want) {
			t.Fatalf("episode wrapper mismatch for seed=%d index=%d: got %+v, want %+v", pair[0], pair[1], got, want)
		}
	}
	continuous, err := experiment.NewDelayedTrainer(1, .1, false)
	if err != nil {
		t.Fatal(err)
	}
	continuousFixture, err := delayedfixture.NewDelayedTrainer(1, .1, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(continuous.Snapshot().Parameters, continuousFixture.Snapshot().Parameters) {
		t.Fatal("continuous trainer wrapper mismatch")
	}
	lif, err := experiment.NewDelayedLIFTrainer(1, .1, learning.Trainable{Weights: true})
	if err != nil {
		t.Fatal(err)
	}
	lifFixture, err := delayedfixture.NewDelayedLIFTrainer(1, .1, learning.Trainable{Weights: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lif.Snapshot().Parameters, lifFixture.Snapshot().Parameters) {
		t.Fatal("LIF trainer wrapper mismatch")
	}
}

func TestDelayedBenchmarkLearnsWithFixedPeriphery(t *testing.T) {
	config := experiment.DefaultDelayedConfig()
	report, err := experiment.RunDelayed(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Runs) != 3 {
		t.Fatal("expected all three preregistered seeds")
	}
	for _, r := range report.Runs {
		if !r.Passed {
			t.Fatalf("learning gate failed: %+v", r)
		}
		if r.CoreUpdateNorm == 0 || r.PeripheryUpdateNorm != 0 {
			t.Fatalf("incorrect trainable groups: %+v", r)
		}
		if r.FrozenMSE != r.BeforeMSE {
			t.Fatalf("frozen baseline changed: %+v", r)
		}
		t.Logf("seed=%d mse=%g -> %g frozen=%g shuffled=%g", r.Seed, r.BeforeMSE, r.AfterMSE, r.FrozenMSE, r.ShuffledMSE)
	}
}

func TestDelayedGeneratorAndCancellation(t *testing.T) {
	a := experiment.DelayedEpisode(10, 20)
	b := experiment.DelayedEpisode(10, 20)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("generator is not deterministic")
	}
	if reflect.DeepEqual(a, experiment.DelayedEpisode(11, 20)) {
		t.Fatal("different split seeds have same sample")
	}
	for _, row := range a.Input[1:] {
		if row[0] != 0 {
			t.Fatal("later observations leak target")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := experiment.RunDelayed(ctx, experiment.DefaultDelayedConfig()); err == nil {
		t.Fatal("accepted canceled benchmark")
	}
}
