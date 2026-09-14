package learning_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/signal"
)

func ExampleBindProjections() {
	// Artificial two-neuron graph, listed in the core's actual node-index order.
	candidates := []signal.NeuronCandidate{
		{ID: signal.NeuronID{Namespace: "example", ExternalID: "a"}, Region: "input"},
		{ID: signal.NeuronID{Namespace: "example", ExternalID: "b"}, Region: "output"},
	}
	input, err := signal.NewProjection(signal.ProjectionSpec{
		SchemaVersion: signal.CurrentSchemaVersion(), Direction: signal.ProjectionInput,
		Source: "artificial-example/v1", Artificial: true, ChannelShape: []int{1},
		Selection: signal.NeuronSelection{Mode: signal.SelectRegion, Value: "input"},
		Random:    &signal.ProjectionRandom{Seed: 7, Scale: .5},
	}, candidates)
	if err != nil {
		panic(err)
	}
	saved, err := json.Marshal(input)
	if err != nil {
		panic(err)
	}
	loaded, err := signal.DecodeProjection(bytes.NewReader(saved))
	if err != nil {
		panic(err)
	}
	if err := loaded.ValidateChannelShape([]int{1}); err != nil {
		panic(err)
	}
	output, err := signal.NewProjection(signal.ProjectionSpec{
		SchemaVersion: signal.CurrentSchemaVersion(), Direction: signal.ProjectionOutput,
		Source: "artificial-example/v1", Artificial: true, ChannelShape: []int{1},
		Selection: signal.NeuronSelection{Mode: signal.SelectRegion, Value: "output"},
		Weights:   []float64{.8},
	}, candidates)
	if err != nil {
		panic(err)
	}
	config := learning.Config{Dynamics: dynamics.Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, DT: .5, Activation: "tanh"}, InputSize: 1, OutputSize: 1}
	parameters := learning.Parameters{Core: dynamics.Parameters{Weights: []float64{.3}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}}}
	config, parameters, err = learning.BindProjections(config, parameters, candidates, loaded, output)
	if err != nil {
		panic(err)
	}
	options := learning.DefaultOptions()
	options.Trainable.Encoder = false // Keep the saved random input projection fixed.
	trainer, err := learning.NewTrainer(config, parameters, options)
	if err != nil {
		panic(err)
	}
	if _, err := trainer.Step(context.Background(), [][]float64{{1}, {0}}, []float64{.2}); err != nil {
		panic(err)
	}
	before := trainer.Snapshot()
	replacement := output.Spec()
	replacement.Weights = []float64{.4}
	output, err = signal.NewProjection(replacement, candidates)
	if err != nil {
		panic(err)
	}
	config, parameters, err = learning.BindProjections(before.Config, before.Parameters, candidates, loaded, output)
	if err != nil {
		panic(err)
	}
	next, err := learning.NewTrainer(config, parameters, options)
	if err != nil {
		panic(err)
	}
	fmt.Println("input nodes:", config.InputNodes, "readout nodes:", config.ReadoutNodes)
	fmt.Println("old updates:", trainer.Snapshot().Updates, "replacement updates:", next.Snapshot().Updates)
	// Output:
	// input nodes: [0] readout nodes: [1]
	// old updates: 1 replacement updates: 0
}
