package experiment_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/experiment"
)

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
