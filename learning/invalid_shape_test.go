package learning_test

import (
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"testing"
)

func TestTrainerRejectsHugeDeclaredShapeBeforeAllocation(t *testing.T) {
	// A compact malformed snapshot must fail on missing parameter arrays before
	// trying to allocate an input or initial-state buffer of the declared size.
	for _, c := range []learning.Config{
		{Dynamics: dynamics.Config{Nodes: 1, DT: 1, Activation: "tanh"}, InputSize: 1 << 40, OutputSize: 1, ReadoutNodes: []int{0}},
		{Dynamics: dynamics.Config{Nodes: 1 << 40, DT: 1, Activation: "tanh"}, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{0}},
	} {
		if _, err := learning.NewTrainer(c, learning.Parameters{}, learning.DefaultOptions()); err == nil {
			t.Fatal("accepted missing parameters")
		}
	}
}
