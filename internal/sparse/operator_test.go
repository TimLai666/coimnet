package sparse

import (
	"math"
	"math/rand"
	"reflect"
	"testing"
)

func TestForwardAndBackwardHandCalculation(t *testing.T) {
	op, err := New(3, []int{0, 2, 1}, []int{1, 1, 2})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	weights := []float64{0.5, -0.25, 0.75}
	input := []float64{2, 4, 8}
	got, err := op.Forward(weights, input, 1, 1)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	want := []float64{0, -1, 3}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Forward = %v, want %v", got, want)
	}

	gradWeight, gradInput, err := op.Backward(weights, input, []float64{1, 2, 3}, 1, 1)
	if err != nil {
		t.Fatalf("Backward: %v", err)
	}
	if want := []float64{4, 16, 12}; !reflect.DeepEqual(gradWeight, want) {
		t.Fatalf("gradWeight = %v, want %v", gradWeight, want)
	}
	if want := []float64{1, 2.25, -0.5}; !reflect.DeepEqual(gradInput, want) {
		t.Fatalf("gradInput = %v, want %v", gradInput, want)
	}
}

func TestForwardAndBackwardMatchIndependentReferenceForBatchVector(t *testing.T) {
	sources := []int{0, 2, 2, 3, 1, 3}
	targets := []int{1, 1, 1, 3, 3, 0}
	parameterIndex := []int{0, 1, 1, 2, 3, 0}
	op, err := New(4, sources, targets)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	op, err = op.WithParameterIndex(parameterIndex)
	if err != nil {
		t.Fatalf("WithParameterIndex: %v", err)
	}

	const batch = 2
	const channels = 3
	weights := []float64{0.5, -0.25, 0.75, 1.25}
	input := []float64{
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12,
		-1, 0.5, 2, -2, 3, -4, 4.5, -5, 6, -6.5, 7, -8,
	}
	gradOutput := []float64{
		0.25, -1, 2, 0.5, 1.5, -0.75, -2, 0.2, 0.4, 1, -1.5, 0.75,
		-0.5, 1, -1.25, 0.75, -2, 1.5, 0.25, 0.5, -0.25, 2, -1, 0.125,
	}

	wantOutput, wantWeight, wantInput := denseReference(sources, targets, parameterIndex, weights, input, gradOutput, 4, batch, channels)
	gotOutput, err := op.Forward(weights, input, batch, channels)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if !equalFloatSlices(gotOutput, wantOutput, 1e-12) {
		t.Fatalf("Forward = %v, want %v", gotOutput, wantOutput)
	}
	gotWeight, gotInput, err := op.Backward(weights, input, gradOutput, batch, channels)
	if err != nil {
		t.Fatalf("Backward: %v", err)
	}
	if !equalFloatSlices(gotWeight, wantWeight, 1e-12) {
		t.Fatalf("gradWeight = %v, want %v", gotWeight, wantWeight)
	}
	if !equalFloatSlices(gotInput, wantInput, 1e-12) {
		t.Fatalf("gradInput = %v, want %v", gotInput, wantInput)
	}
}

func TestRandomSmallGraphsMatchDenseReference(t *testing.T) {
	random := rand.New(rand.NewSource(20260913))
	for graph := 0; graph < 32; graph++ {
		nodes := 1 + random.Intn(6)
		edges := random.Intn(12)
		parameterSlots := 0
		if edges > 0 {
			parameterSlots = 1 + random.Intn(edges)
		}
		sources := make([]int, edges)
		targets := make([]int, edges)
		parameterIndex := make([]int, edges)
		for edge := 0; edge < edges; edge++ {
			sources[edge] = random.Intn(nodes)
			targets[edge] = random.Intn(nodes)
			if edge < parameterSlots {
				parameterIndex[edge] = edge
			} else {
				parameterIndex[edge] = random.Intn(parameterSlots)
			}
		}
		weights := make([]float64, parameterSlots)
		for index := range weights {
			weights[index] = 2*random.Float64() - 1
		}
		batch := 1 + random.Intn(3)
		channels := 1 + random.Intn(4)
		input := make([]float64, batch*nodes*channels)
		gradOutput := make([]float64, len(input))
		for index := range input {
			input[index] = 2*random.Float64() - 1
			gradOutput[index] = 2*random.Float64() - 1
		}

		op, err := New(nodes, sources, targets)
		if err != nil {
			t.Fatalf("graph %d New: %v", graph, err)
		}
		op, err = op.WithParameterIndex(parameterIndex)
		if err != nil {
			t.Fatalf("graph %d WithParameterIndex: %v", graph, err)
		}
		wantOutput, wantWeight, wantInput := denseReference(sources, targets, parameterIndex, weights, input, gradOutput, nodes, batch, channels)
		gotOutput, err := op.Forward(weights, input, batch, channels)
		if err != nil {
			t.Fatalf("graph %d Forward: %v", graph, err)
		}
		if !equalFloatSlices(gotOutput, wantOutput, 1e-12) {
			t.Fatalf("graph %d Forward = %v, want %v", graph, gotOutput, wantOutput)
		}
		gotWeight, gotInput, err := op.Backward(weights, input, gradOutput, batch, channels)
		if err != nil {
			t.Fatalf("graph %d Backward: %v", graph, err)
		}
		if !equalFloatSlices(gotWeight, wantWeight, 1e-12) {
			t.Fatalf("graph %d gradWeight = %v, want %v", graph, gotWeight, wantWeight)
		}
		if !equalFloatSlices(gotInput, wantInput, 1e-12) {
			t.Fatalf("graph %d gradInput = %v, want %v", graph, gotInput, wantInput)
		}
	}
}

func TestBackwardMatchesFiniteDifference(t *testing.T) {
	op, err := New(3, []int{0, 1, 2, 0}, []int{1, 2, 0, 2})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	const batch = 2
	const channels = 2
	weights := []float64{0.35, -0.8, 0.2, 1.1}
	input := []float64{1.2, -0.5, 0.7, 2.1, -1.4, 0.25, 1.7, -0.9, 0.3, 1.1, -2.2, 0.6}
	gradOutput := []float64{0.4, -0.8, 1.1, 0.2, -1.3, 0.5, 0.7, -0.6, 1.4, 0.9, -0.3, 0.8}

	gradWeight, gradInput, err := op.Backward(weights, input, gradOutput, batch, channels)
	if err != nil {
		t.Fatalf("Backward: %v", err)
	}
	const epsilon = 1e-6
	for i := range weights {
		plus := append([]float64(nil), weights...)
		minus := append([]float64(nil), weights...)
		plus[i] += epsilon
		minus[i] -= epsilon
		plusLoss := dot(mustForward(t, op, plus, input, batch, channels), gradOutput)
		minusLoss := dot(mustForward(t, op, minus, input, batch, channels), gradOutput)
		finiteDifference := (plusLoss - minusLoss) / (2 * epsilon)
		if !closeFloat(gradWeight[i], finiteDifference, 1e-8, 1e-8) {
			t.Errorf("gradWeight[%d] = %.12g, finite difference %.12g", i, gradWeight[i], finiteDifference)
		}
	}
	for i := range input {
		plus := append([]float64(nil), input...)
		minus := append([]float64(nil), input...)
		plus[i] += epsilon
		minus[i] -= epsilon
		plusLoss := dot(mustForward(t, op, weights, plus, batch, channels), gradOutput)
		minusLoss := dot(mustForward(t, op, weights, minus, batch, channels), gradOutput)
		finiteDifference := (plusLoss - minusLoss) / (2 * epsilon)
		if !closeFloat(gradInput[i], finiteDifference, 1e-8, 1e-8) {
			t.Errorf("gradInput[%d] = %.12g, finite difference %.12g", i, gradInput[i], finiteDifference)
		}
	}
}

func TestZeroEdgesAndIsolatedNodes(t *testing.T) {
	op, err := New(4, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	input := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	got, err := op.Forward(nil, input, 2, 1)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if want := make([]float64, len(input)); !reflect.DeepEqual(got, want) {
		t.Fatalf("Forward = %v, want %v", got, want)
	}
	gradWeight, gradInput, err := op.Backward(nil, input, input, 2, 1)
	if err != nil {
		t.Fatalf("Backward: %v", err)
	}
	if len(gradWeight) != 0 || !reflect.DeepEqual(gradInput, make([]float64, len(input))) {
		t.Fatalf("Backward = (%v, %v), want zero gradients", gradWeight, gradInput)
	}
}

func TestSelfAndRepeatedEdgesPreserveOrder(t *testing.T) {
	op, err := New(2, []int{0, 0, 0, 1}, []int{0, 0, 0, 0})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := op.Forward([]float64{1, 2, 3, 4}, []float64{5, 7}, 1, 1)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if want := []float64{58, 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Forward = %v, want %v", got, want)
	}
	gradWeight, gradInput, err := op.Backward([]float64{1, 2, 3, 4}, []float64{5, 7}, []float64{1, 0}, 1, 1)
	if err != nil {
		t.Fatalf("Backward: %v", err)
	}
	if want := []float64{5, 5, 5, 7}; !reflect.DeepEqual(gradWeight, want) {
		t.Fatalf("gradWeight = %v, want %v", gradWeight, want)
	}
	if want := []float64{6, 4}; !reflect.DeepEqual(gradInput, want) {
		t.Fatalf("gradInput = %v, want %v", gradInput, want)
	}
}

func TestConstructorCopiesMutableInputs(t *testing.T) {
	sources := []int{0, 1}
	targets := []int{1, 0}
	op, err := New(2, sources, targets)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sources[0] = 1
	targets[0] = 0
	got, err := op.Forward([]float64{2, 3}, []float64{5, 7}, 1, 1)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if want := []float64{21, 10}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Forward after source mutation = %v, want %v", got, want)
	}
}

func TestNewRejectsInvalidTopology(t *testing.T) {
	cases := []struct {
		name            string
		nodes           int
		sources, target []int
	}{
		{name: "negative nodes", nodes: -1},
		{name: "mismatched edge slices", nodes: 2, sources: []int{0}, target: nil},
		{name: "negative source", nodes: 2, sources: []int{-1}, target: []int{1}},
		{name: "source out of range", nodes: 2, sources: []int{2}, target: []int{1}},
		{name: "negative target", nodes: 2, sources: []int{0}, target: []int{-1}},
		{name: "target out of range", nodes: 2, sources: []int{0}, target: []int{2}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := New(testCase.nodes, testCase.sources, testCase.target); err == nil {
				t.Fatal("New unexpectedly succeeded")
			}
		})
	}
}

func TestWithParameterIndexCopiesAndValidates(t *testing.T) {
	op, err := New(2, []int{0, 1}, []int{1, 0})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	indices := []int{0, 0}
	shared, err := op.WithParameterIndex(indices)
	if err != nil {
		t.Fatalf("WithParameterIndex: %v", err)
	}
	indices[0] = 1
	got, err := shared.Forward([]float64{2}, []float64{3, 5}, 1, 1)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if want := []float64{10, 6}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Forward after parameter index mutation = %v, want %v", got, want)
	}
	if _, err := op.WithParameterIndex([]int{0}); err == nil {
		t.Fatal("WithParameterIndex accepted the wrong edge count")
	}
	if _, err := op.WithParameterIndex([]int{0, -1}); err == nil {
		t.Fatal("WithParameterIndex accepted a negative parameter index")
	}
}

func TestInvalidInputsAndArithmeticErrorsDoNotMutateInputs(t *testing.T) {
	op, err := New(2, []int{0}, []int{1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	weights := []float64{2}
	input := []float64{3, 5}
	gradOutput := []float64{7, 11}
	weightsBefore := append([]float64(nil), weights...)
	inputBefore := append([]float64(nil), input...)
	gradOutputBefore := append([]float64(nil), gradOutput...)

	for name, run := range map[string]func() error{
		"wrong input shape": func() error {
			_, err := op.Forward(weights, input[:1], 1, 1)
			return err
		},
		"wrong weight shape": func() error {
			_, err := op.Forward(nil, input, 1, 1)
			return err
		},
		"nonfinite weight": func() error {
			bad := []float64{math.NaN()}
			_, err := op.Forward(bad, input, 1, 1)
			return err
		},
		"nonfinite input": func() error {
			bad := []float64{math.Inf(1), 5}
			_, err := op.Forward(weights, bad, 1, 1)
			return err
		},
		"nonfinite gradient": func() error {
			bad := []float64{math.Inf(-1), 11}
			_, _, err := op.Backward(weights, input, bad, 1, 1)
			return err
		},
		"wrong gradient shape": func() error {
			_, _, err := op.Backward(weights, input, gradOutput[:1], 1, 1)
			return err
		},
		"zero batch": func() error {
			_, err := op.Forward(weights, nil, 0, 1)
			return err
		},
		"zero channels": func() error {
			_, err := op.Forward(weights, nil, 1, 0)
			return err
		},
		"integer shape overflow": func() error {
			maxInt := int(^uint(0) >> 1)
			_, err := op.Forward(weights, nil, maxInt, 2)
			return err
		},
		"forward arithmetic overflow": func() error {
			_, err := op.Forward([]float64{math.MaxFloat64}, []float64{2, 1}, 1, 1)
			return err
		},
		"backward arithmetic overflow": func() error {
			_, _, err := op.Backward([]float64{math.MaxFloat64}, []float64{1, 1}, []float64{0, math.MaxFloat64}, 1, 1)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := run(); err == nil {
				t.Fatal("operation unexpectedly succeeded")
			}
		})
	}
	if !reflect.DeepEqual(weights, weightsBefore) || !reflect.DeepEqual(input, inputBefore) || !reflect.DeepEqual(gradOutput, gradOutputBefore) {
		t.Fatalf("operation mutated inputs: weights=%v input=%v gradOutput=%v", weights, input, gradOutput)
	}
}

func denseReference(sources, targets, parameterIndex []int, weights, input, gradOutput []float64, nodes, batch, channels int) ([]float64, []float64, []float64) {
	// This deliberately uses an N*N matrix only for the tiny test fixture; the
	// implementation under test must remain edge-sparse.
	matrix := make([]float64, nodes*nodes)
	for edge := range sources {
		matrix[targets[edge]*nodes+sources[edge]] += weights[parameterIndex[edge]]
	}
	output := make([]float64, len(input))
	gradWeight := make([]float64, len(weights))
	gradInput := make([]float64, len(input))
	stride := nodes * channels
	for b := 0; b < batch; b++ {
		base := b * stride
		for targetNode := 0; targetNode < nodes; targetNode++ {
			for c := 0; c < channels; c++ {
				target := base + targetNode*channels + c
				for sourceNode := 0; sourceNode < nodes; sourceNode++ {
					coefficient := matrix[targetNode*nodes+sourceNode]
					output[target] += coefficient * input[base+sourceNode*channels+c]
					gradInput[base+sourceNode*channels+c] += gradOutput[target] * coefficient
				}
			}
		}
	}
	for b := 0; b < batch; b++ {
		base := b * stride
		for edge := range sources {
			parameter := parameterIndex[edge]
			for c := 0; c < channels; c++ {
				source := base + sources[edge]*channels + c
				target := base + targets[edge]*channels + c
				gradWeight[parameter] += gradOutput[target] * input[source]
			}
		}
	}
	return output, gradWeight, gradInput
}

func dot(a, b []float64) float64 {
	var result float64
	for i := range a {
		result += a[i] * b[i]
	}
	return result
}

func mustForward(t *testing.T, op *Operator, weights, input []float64, batch, channels int) []float64 {
	t.Helper()
	output, err := op.Forward(weights, input, batch, channels)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	return output
}

func equalFloatSlices(a, b []float64, tolerance float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Abs(a[i]-b[i]) > tolerance {
			return false
		}
	}
	return true
}

func closeFloat(a, b, absolute, relative float64) bool {
	difference := math.Abs(a - b)
	limit := absolute + relative*math.Max(math.Abs(a), math.Abs(b))
	return difference <= limit
}
