package experiment_test

import (
	"context"
	"testing"

	"github.com/TimLai666/coimnet/experiment"
)

func TestShuffledControlUsesTrainingLabelsOnly(t *testing.T) {
	c := experiment.DefaultDelayedConfig()
	c.Seeds, c.Updates, c.TestCount = []uint64{42}, 8, 16
	// This holdout seed collided with the previous independent-label generator.
	c.TestSeed = c.TrainSeed ^ 0xd1b54a32d192ed03
	r, err := experiment.RunDelayed(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	control, err := experiment.NewDelayedTrainer(42, c.LearningRate, false)
	if err != nil {
		t.Fatal(err)
	}
	// Frozen protocol vector, a permutation of exactly the eight training labels.
	indices := []int{4, 6, 2, 7, 3, 1, 5, 0}
	for i, j := range indices {
		input := experiment.DelayedEpisode(c.TrainSeed, uint64(i))
		label := experiment.DelayedEpisode(c.TrainSeed, uint64(j))
		if _, err := control.Step(context.Background(), input.Input, label.Target); err != nil {
			t.Fatal(err)
		}
	}
	want, err := experiment.EvaluateDelayed(context.Background(), control, c.TestSeed, c.TestCount)
	if err != nil {
		t.Fatal(err)
	}
	if r.Runs[0].ShuffledMSE != want {
		t.Fatalf("shuffled control MSE=%g, training-label permutation gives %g", r.Runs[0].ShuffledMSE, want)
	}
}

func TestEvaluationRejectsUnboundedWork(t *testing.T) {
	tr, err := experiment.NewDelayedTrainer(42, .02, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := experiment.EvaluateDelayed(context.Background(), tr, 1003, 100001); err == nil {
		t.Fatal("accepted count above fixture limit")
	}
}

func TestProtocolRejectsOverlappingCounterStreams(t *testing.T) {
	c := experiment.DefaultDelayedConfig()
	c.TestSeed = c.TrainSeed + uint64(0x9e3779b97f4a7c15)
	if _, err := experiment.RunDelayed(context.Background(), c); err == nil {
		t.Fatal("accepted holdout stream starting at training sample one")
	}
}
