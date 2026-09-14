package learning_test

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strconv"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

func inputNodesConfig(nodes []int) learning.Config {
	return learning.Config{
		Dynamics: dynamics.Config{
			Nodes:      3,
			Sources:    []int{0, 1, 2},
			Targets:    []int{1, 2, 0},
			DT:         .4,
			Activation: "tanh",
		},
		InputSize:    2,
		OutputSize:   1,
		ReadoutNodes: []int{1},
		InputNodes:   nodes,
	}
}

func inputNodesParameters(encoder []float64) learning.Parameters {
	return learning.Parameters{
		Core: dynamics.Parameters{
			Weights: []float64{.3, -.2, .15},
			Bias:    []float64{.1, -.1, .05},
			LogTau:  []float64{.1, .2, -.05},
		},
		Encoder: encoder,
		Readout: []float64{.8},
	}
}

func expandInputEncoder(compact []float64, inputSize, nodes int, inputNodes []int) []float64 {
	width := len(inputNodes)
	dense := make([]float64, inputSize*nodes)
	for i := 0; i < inputSize; i++ {
		for j, node := range inputNodes {
			dense[i*nodes+node] = compact[i*width+j]
		}
	}
	return dense
}

func requireClose(t *testing.T, label string, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Fatalf("%s = %g, want %g (tolerance %g)", label, got, want, tolerance)
	}
}

func requireSlicesClose(t *testing.T, label string, got, want []float64, tolerance float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s length = %d, want %d", label, len(got), len(want))
	}
	for i := range got {
		requireClose(t, label+"["+strconv.Itoa(i)+"]", got[i], want[i], tolerance)
	}
}

func selectedInputFixture(t *testing.T) (*learning.Network, learning.Parameters, [][]float64, []float64) {
	t.Helper()
	compact := []float64{.7, -.2, .3, .6}
	network, err := learning.NewNetwork(inputNodesConfig([]int{2, 0}))
	if err != nil {
		t.Fatal(err)
	}
	return network, inputNodesParameters(compact), [][]float64{{.8, -.1}, {-.2, .4}, {.3, .5}}, []float64{.25}
}

func TestInputNodesMatchDenseEncoderExpansionAndFullGradient(t *testing.T) {
	selected, compact, input, target := selectedInputFixture(t)
	selectedConfig := selected.Config()
	denseConfig := inputNodesConfig(nil)
	dense, err := learning.NewNetwork(denseConfig)
	if err != nil {
		t.Fatal(err)
	}
	denseParameters := compact
	denseParameters.Encoder = expandInputEncoder(compact.Encoder, denseConfig.InputSize, denseConfig.Dynamics.Nodes, selectedConfig.InputNodes)

	selectedPrediction, err := selected.Predict(context.Background(), compact, input)
	if err != nil {
		t.Fatal(err)
	}
	densePrediction, err := dense.Predict(context.Background(), denseParameters, input)
	if err != nil {
		t.Fatal(err)
	}
	requireSlicesClose(t, "prediction", selectedPrediction, densePrediction, 1e-7)

	selectedLoss, selectedGradient, err := selected.LossGradient(context.Background(), compact, input, target, 0)
	if err != nil {
		t.Fatal(err)
	}
	denseLoss, denseGradient, err := dense.LossGradient(context.Background(), denseParameters, input, target, 0)
	if err != nil {
		t.Fatal(err)
	}
	requireClose(t, "loss", selectedLoss, denseLoss, 1e-7)
	requireSlicesClose(t, "core weights", selectedGradient.Core.Weights, denseGradient.Core.Weights, 1e-6)
	requireSlicesClose(t, "core bias", selectedGradient.Core.Bias, denseGradient.Core.Bias, 1e-6)
	requireSlicesClose(t, "core log tau", selectedGradient.Core.LogTau, denseGradient.Core.LogTau, 1e-6)
	requireSlicesClose(t, "readout", selectedGradient.Readout, denseGradient.Readout, 1e-6)
	requireSlicesClose(t, "inputs", flattenRows(selectedGradient.Inputs), flattenRows(denseGradient.Inputs), 1e-6)
	for i, node := range selectedConfig.InputNodes {
		for j := 0; j < selectedConfig.InputSize; j++ {
			selectedIndex := j*len(selectedConfig.InputNodes) + i
			denseIndex := j*denseConfig.Dynamics.Nodes + node
			requireClose(t, "encoder gradient", selectedGradient.Encoder[selectedIndex], denseGradient.Encoder[denseIndex], 1e-6)
		}
	}
}

func flattenRows(rows [][]float64) []float64 {
	var out []float64
	for _, row := range rows {
		out = append(out, row...)
	}
	return out
}

func selectedLoss(t *testing.T, network *learning.Network, parameters learning.Parameters, input [][]float64, target []float64) float64 {
	t.Helper()
	prediction, err := network.Predict(context.Background(), parameters, input)
	if err != nil {
		t.Fatal(err)
	}
	var loss float64
	for i := range target {
		residual := prediction[i] - target[i]
		loss += residual * residual
	}
	return loss / float64(len(target))
}

func TestInputNodesGradientsMatchFiniteDifferences(t *testing.T) {
	network, parameters, input, target := selectedInputFixture(t)
	_, gradient, err := network.LossGradient(context.Background(), parameters, input, target, 0)
	if err != nil {
		t.Fatal(err)
	}
	const epsilon = 2e-3
	const tolerance = 2e-5
	check := func(label string, value *float64, derivative float64) {
		t.Helper()
		original := *value
		*value = original + epsilon
		plus := selectedLoss(t, network, parameters, input, target)
		*value = original - epsilon
		minus := selectedLoss(t, network, parameters, input, target)
		*value = original
		finiteDifference := (plus - minus) / (2 * epsilon)
		requireClose(t, label, derivative, finiteDifference, tolerance+1e-3*math.Abs(finiteDifference))
	}
	for i := range parameters.Core.Weights {
		check("core weights finite difference", &parameters.Core.Weights[i], gradient.Core.Weights[i])
	}
	for i := range parameters.Core.Bias {
		check("core bias finite difference", &parameters.Core.Bias[i], gradient.Core.Bias[i])
	}
	for i := range parameters.Core.LogTau {
		check("core log tau finite difference", &parameters.Core.LogTau[i], gradient.Core.LogTau[i])
	}
	for i := range parameters.Encoder {
		check("encoder finite difference", &parameters.Encoder[i], gradient.Encoder[i])
	}
	for i := range parameters.Readout {
		check("readout finite difference", &parameters.Readout[i], gradient.Readout[i])
	}
	for i := range input {
		for j := range input[i] {
			check("input finite difference", &input[i][j], gradient.Inputs[i][j])
		}
	}
}

func TestInputNodesValidationAndConfigIsolation(t *testing.T) {
	for _, test := range []struct {
		name  string
		nodes []int
	}{
		{name: "empty", nodes: []int{}},
		{name: "duplicate", nodes: []int{1, 1}},
		{name: "negative", nodes: []int{-1}},
		{name: "out of range", nodes: []int{3}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := learning.NewNetwork(inputNodesConfig(test.nodes)); err == nil {
				t.Fatalf("accepted invalid input nodes %v", test.nodes)
			}
		})
	}

	inputNodes := []int{2, 0}
	network, err := learning.NewNetwork(inputNodesConfig(inputNodes))
	if err != nil {
		t.Fatal(err)
	}
	inputNodes[0] = 1
	got := network.Config().InputNodes
	if !reflect.DeepEqual(got, []int{2, 0}) {
		t.Fatalf("network retained caller input nodes: %v", got)
	}
	got[0] = 1
	if !reflect.DeepEqual(network.Config().InputNodes, []int{2, 0}) {
		t.Fatalf("Config() exposed input nodes: %v", network.Config().InputNodes)
	}
}

func TestInputNodesTrainerUsesCompactEncoderAndRestoresSelection(t *testing.T) {
	network, parameters, input, target := selectedInputFixture(t)
	trainer, err := learning.NewTrainer(network.Config(), parameters, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := learning.NewTrainer(network.Config(), inputNodesParameters(expandInputEncoder(parameters.Encoder, 2, 3, []int{2, 0})), learning.DefaultOptions()); err == nil {
		t.Fatal("accepted dense encoder for selected input nodes")
	}
	if _, err := trainer.Step(context.Background(), input, target); err != nil {
		t.Fatal(err)
	}
	snapshot := trainer.Snapshot()
	if !reflect.DeepEqual(snapshot.Config.InputNodes, []int{2, 0}) {
		t.Fatalf("training changed input nodes: %v", snapshot.Config.InputNodes)
	}
	if len(snapshot.Parameters.Encoder) != 4 {
		t.Fatalf("training expanded compact encoder to %d parameters", len(snapshot.Parameters.Encoder))
	}
	restored, err := learning.RestoreTrainer(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Snapshot().Config.InputNodes, []int{2, 0}) {
		t.Fatalf("restore lost input nodes: %v", restored.Snapshot().Config.InputNodes)
	}
	got, err := restored.Predict(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	want, err := trainer.Predict(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	requireSlicesClose(t, "restored prediction", got, want, 1e-7)
}

func TestNilInputNodesKeepsLegacyConfigJSON(t *testing.T) {
	config := inputNodesConfig(nil)
	network, err := learning.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	if network.Config().InputNodes != nil {
		t.Fatalf("nil input nodes became non-nil: %v", network.Config().InputNodes)
	}
	encoded, err := json.Marshal(network.Config())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(`"input_nodes"`)) {
		t.Fatalf("legacy nil input nodes changed JSON: %s", encoded)
	}
	var decoded learning.Config
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.InputNodes != nil {
		t.Fatalf("legacy JSON decoded input nodes as non-nil: %v", decoded.InputNodes)
	}
}
