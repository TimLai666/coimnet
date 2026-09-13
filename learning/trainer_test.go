package learning_test

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func TestTrainerLearnsAndMaskFreezesMomentum(t *testing.T) {
	n, p := network(t)
	options := learning.DefaultOptions()
	tr, err := learning.NewTrainer(n.Config(), p, options)
	if err != nil {
		t.Fatal(err)
	}
	x := [][]float64{{.7}, {0}, {0}}
	target := []float64{.4}
	before, _, err := n.LossGradient(context.Background(), p, x, target, 0)
	if err != nil {
		t.Fatal(err)
	}
	for range 150 {
		if _, err := tr.Step(context.Background(), x, target); err != nil {
			t.Fatal(err)
		}
	}
	s := tr.Snapshot()
	after, _, err := n.LossGradient(context.Background(), s.Parameters, x, target, 0)
	if err != nil {
		t.Fatal(err)
	}
	if after >= before*.01 {
		t.Fatalf("loss did not improve enough: %g -> %g", before, after)
	}
	if reflect.DeepEqual(p.Core.Weights, s.Parameters.Core.Weights) {
		t.Fatal("core did not update")
	}
	// Freeze after nonzero momentum has accumulated, with nonzero weight decay.
	s.Options.Trainable = learning.Trainable{}
	s.Options.WeightDecay = .5
	frozen, err := learning.RestoreTrainer(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = frozen.Step(context.Background(), x, target); err != nil {
		t.Fatal(err)
	}
	fs := frozen.Snapshot()
	if !reflect.DeepEqual(fs.Parameters, s.Parameters) || !reflect.DeepEqual(fs.Optimizer, s.Optimizer) {
		t.Fatal("freeze changed parameters or optimizer moments")
	}
}

func TestTrainerFailureAndSnapshotIsolation(t *testing.T) {
	n, p := network(t)
	tr, err := learning.NewTrainer(n.Config(), p, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	x := [][]float64{{1}, {0}}
	target := []float64{.2}
	before := tr.Snapshot()
	if _, err := tr.Step(context.Background(), x, []float64{math.NaN()}); err == nil {
		t.Fatal("accepted NaN target")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := tr.Step(ctx, x, target); err == nil {
		t.Fatal("accepted cancellation")
	}
	if !reflect.DeepEqual(before, tr.Snapshot()) {
		t.Fatal("failed transaction changed state")
	}
	before.Parameters.Encoder[0] = 9
	before.Config.ReadoutNodes[0] = 0
	if tr.Snapshot().Parameters.Encoder[0] == 9 || tr.Snapshot().Config.ReadoutNodes[0] == 0 {
		t.Fatal("snapshot aliases trainer")
	}
	for range 5 {
		if _, err := tr.Step(context.Background(), x, target); err != nil {
			t.Fatal(err)
		}
	}
	restored, err := learning.RestoreTrainer(tr.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if _, err := tr.Step(context.Background(), x, target); err != nil {
			t.Fatal(err)
		}
		if _, err := restored.Step(context.Background(), x, target); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(tr.Snapshot(), restored.Snapshot()) {
		t.Fatal("restored optimizer diverged")
	}
	s := tr.Snapshot()
	s.Optimizer.Second[0] = -1
	if _, err := learning.RestoreTrainer(s); err == nil {
		t.Fatal("accepted invalid moment")
	}
}
