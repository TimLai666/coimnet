package experiment

import (
	"math"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

// ContinualFixture is the individual every continual fixture protocol trains:
// the three-neuron chain of NewDelayedTrainer with two input channels that
// both inject into neuron 0 with weight 1, the readout on neuron 2, weights
// trainable at rate 0.02, the initial weight of edge 0 seeded exactly like
// NewDelayedTrainer. Neural state starts at zeros.
func ContinualFixture(seed uint64) (*learning.Individual, error) {
	c := learning.Config{Dynamics: dynamics.Config{Nodes: 3, Sources: []int{0, 1, 1}, Targets: []int{1, 1, 2}, DT: 1, Activation: "tanh"}, InputSize: 2, OutputSize: 1, ReadoutNodes: []int{2}}
	p := learning.Parameters{Core: dynamics.Parameters{Weights: []float64{.15 + float64(mix(seed)%100)/1000, .2, .2}, Bias: []float64{0, 0, 0}, LogTau: []float64{math.Log(2), math.Log(2), math.Log(2)}}, Encoder: []float64{1, 0, 0, 1, 0, 0}, Readout: []float64{1}}
	o := learning.DefaultOptions()
	o.LearningRate = 0.02
	o.Trainable = learning.Trainable{Weights: true}
	return learning.NewIndividual(c, p, o, make([]float64, 3))
}
