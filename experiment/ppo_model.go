package experiment

import (
	"fmt"
	"math"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/experiment/gridnav"
	"github.com/TimLai666/coimnet/learning"
)

const (
	ppoModelInputs  = 4
	ppoModelOutputs = gridnav.Actions + 1
)

// newPPOIndividual builds the recurrent policy used by the gridnav protocol.
// The four observation nodes are followed by a fully recurrent hidden block;
// every core node is readable as either an action logit or the value output.
// The encoder and time constants are fixed, while the core weights, biases and
// readout are the only trainable groups.
func newPPOIndividual(seed uint64, hidden int, rate float64) (*learning.Individual, error) {
	if hidden < 4 || hidden > imitationMaxHidden {
		return nil, fmt.Errorf("ppo hidden %d, want a value in [4, %d]", hidden, imitationMaxHidden)
	}
	if !finite(rate) || rate <= 0 {
		return nil, fmt.Errorf("ppo learning rate %v must be finite and positive", rate)
	}

	nodes := ppoModelInputs + hidden
	var sources, targets []int
	edge := func(from, to int) {
		sources = append(sources, from)
		targets = append(targets, to)
	}
	for input := 0; input < ppoModelInputs; input++ {
		for h := 0; h < hidden; h++ {
			edge(input, ppoModelInputs+h)
		}
	}
	for from := 0; from < hidden; from++ {
		for to := 0; to < hidden; to++ {
			edge(ppoModelInputs+from, ppoModelInputs+to)
		}
	}

	inputNodes := make([]int, ppoModelInputs)
	for i := range inputNodes {
		inputNodes[i] = i
	}
	readoutNodes := make([]int, nodes)
	for i := range readoutNodes {
		readoutNodes[i] = i
	}
	config := learning.Config{
		Dynamics: dynamics.Config{
			Nodes:      nodes,
			Sources:    sources,
			Targets:    targets,
			DT:         1,
			Activation: "tanh",
		},
		InputSize:        ppoModelInputs,
		OutputSize:       ppoModelOutputs,
		ReadoutNodes:     readoutNodes,
		InputNodes:       inputNodes,
		ReadoutEveryStep: true,
	}
	logTau := make([]float64, nodes)
	for i := range logTau {
		logTau[i] = math.Log(2)
	}
	encoder := make([]float64, ppoModelInputs*ppoModelInputs)
	for i := 0; i < ppoModelInputs; i++ {
		encoder[i*ppoModelInputs+i] = 1
	}
	parameters := learning.Parameters{
		Core: dynamics.Parameters{
			Weights: imitationUniform(seed, 0, len(sources)),
			Bias:    make([]float64, nodes),
			LogTau:  logTau,
		},
		Encoder: encoder,
		Readout: imitationUniform(seed, 2, len(readoutNodes)*ppoModelOutputs),
	}
	options := learning.DefaultOptions()
	options.LearningRate = rate
	options.Trainable = learning.Trainable{Weights: true, Bias: true, Readout: true}
	return learning.NewIndividual(config, parameters, options, make([]float64, nodes))
}
