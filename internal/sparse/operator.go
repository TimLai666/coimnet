// Package sparse provides deterministic sparse graph operators.
package sparse

import (
	"fmt"
	"math"
)

// Operator applies a directed weighted graph independently to each batch
// element and channel. Edges are evaluated in the order supplied to New.
//
// The input and output layout is [batch][node][channel]. Each edge has one
// scalar parameter that is broadcast over all channels. Use
// WithParameterIndex to make multiple edges share a parameter.
type Operator struct {
	nodes          int
	sources        []int
	targets        []int
	parameterIndex []int
	parameterCount int
}

// New constructs an Operator from directed edges. The source and target
// slices are copied, and their order is retained exactly. A zero-edge graph
// is valid. nodes may be zero when no edges are supplied.
func New(nodes int, sources, targets []int) (*Operator, error) {
	if nodes < 0 {
		return nil, fmt.Errorf("sparse: nodes must be non-negative, got %d", nodes)
	}
	if len(sources) != len(targets) {
		return nil, fmt.Errorf("sparse: sources and targets must have equal lengths, got %d and %d", len(sources), len(targets))
	}
	for edge := range sources {
		if sources[edge] < 0 || sources[edge] >= nodes {
			return nil, fmt.Errorf("sparse: source index at edge %d is out of range: %d for %d nodes", edge, sources[edge], nodes)
		}
		if targets[edge] < 0 || targets[edge] >= nodes {
			return nil, fmt.Errorf("sparse: target index at edge %d is out of range: %d for %d nodes", edge, targets[edge], nodes)
		}
	}

	parameterIndex := make([]int, len(sources))
	for edge := range parameterIndex {
		parameterIndex[edge] = edge
	}
	return &Operator{
		nodes:          nodes,
		sources:        cloneInts(sources),
		targets:        cloneInts(targets),
		parameterIndex: parameterIndex,
		parameterCount: len(parameterIndex),
	}, nil
}

// WithParameterIndex returns a copy of the operator whose edge at position e
// reads weights[parameterIndex[e]]. Parameter indices must be non-negative;
// the supplied weight slice must have length max(index)+1. Gaps below the
// largest index are allowed, while trailing unused slots cannot be inferred
// from this mapping and therefore are not part of the operator shape.
func (o *Operator) WithParameterIndex(parameterIndex []int) (*Operator, error) {
	if o == nil {
		return nil, fmt.Errorf("sparse: cannot configure a nil operator")
	}
	if len(parameterIndex) != len(o.sources) {
		return nil, fmt.Errorf("sparse: parameter index length must equal edge count %d, got %d", len(o.sources), len(parameterIndex))
	}
	parameterCount, err := parameterCount(parameterIndex)
	if err != nil {
		return nil, err
	}
	return &Operator{
		nodes:          o.nodes,
		sources:        cloneInts(o.sources),
		targets:        cloneInts(o.targets),
		parameterIndex: cloneInts(parameterIndex),
		parameterCount: parameterCount,
	}, nil
}

// ParameterIndex returns a copy of the edge-to-parameter mapping.
func (o *Operator) ParameterIndex() []int {
	if o == nil {
		return nil
	}
	return cloneInts(o.parameterIndex)
}

// ParameterCount returns the number of values expected in weights.
func (o *Operator) ParameterCount() int {
	if o == nil {
		return 0
	}
	return o.parameterCount
}

// Forward computes weighted sparse aggregation. The returned slice has the
// same [batch][node][channel] shape as input. batch and channels must both be
// positive; a graph with no edges still returns a zero-valued output.
func (o *Operator) Forward(weights, input []float64, batch, channels int) ([]float64, error) {
	total, stride, err := o.shape(batch, channels)
	if err != nil {
		return nil, err
	}
	if err := o.validateWeights(weights); err != nil {
		return nil, err
	}
	if len(input) != total {
		return nil, fmt.Errorf("sparse: input length must be %d for batch=%d nodes=%d channels=%d, got %d", total, batch, o.nodes, channels, len(input))
	}
	if err := validateFinite("input", input); err != nil {
		return nil, err
	}
	if err := validateFinite("weight", weights); err != nil {
		return nil, err
	}

	output := make([]float64, total)
	for b := 0; b < batch; b++ {
		base := b * stride
		for edge := range o.sources {
			sourceBase := base + o.sources[edge]*channels
			targetBase := base + o.targets[edge]*channels
			weight := weights[o.parameterIndex[edge]]
			for channel := 0; channel < channels; channel++ {
				contribution := weight * input[sourceBase+channel]
				if !isFinite(contribution) {
					return nil, fmt.Errorf("sparse: forward multiplication overflow at batch %d edge %d channel %d", b, edge, channel)
				}
				index := targetBase + channel
				sum := output[index] + contribution
				if !isFinite(sum) {
					return nil, fmt.Errorf("sparse: forward accumulation overflow at batch %d edge %d channel %d", b, edge, channel)
				}
				output[index] = sum
			}
		}
	}
	return output, nil
}

// Backward computes gradients for weights and input using the same fixed edge
// order as Forward. For shared parameters, contributions from every edge are
// accumulated into the corresponding gradWeight entry.
func (o *Operator) Backward(weights, input, gradOutput []float64, batch, channels int) (gradWeight, gradInput []float64, err error) {
	total, stride, err := o.shape(batch, channels)
	if err != nil {
		return nil, nil, err
	}
	if err := o.validateWeights(weights); err != nil {
		return nil, nil, err
	}
	if len(input) != total {
		return nil, nil, fmt.Errorf("sparse: input length must be %d for batch=%d nodes=%d channels=%d, got %d", total, batch, o.nodes, channels, len(input))
	}
	if len(gradOutput) != total {
		return nil, nil, fmt.Errorf("sparse: gradOutput length must be %d for batch=%d nodes=%d channels=%d, got %d", total, batch, o.nodes, channels, len(gradOutput))
	}
	if err := validateFinite("input", input); err != nil {
		return nil, nil, err
	}
	if err := validateFinite("weight", weights); err != nil {
		return nil, nil, err
	}
	if err := validateFinite("gradOutput", gradOutput); err != nil {
		return nil, nil, err
	}

	gradWeight = make([]float64, len(weights))
	gradInput = make([]float64, total)
	for b := 0; b < batch; b++ {
		base := b * stride
		for edge := range o.sources {
			sourceBase := base + o.sources[edge]*channels
			targetBase := base + o.targets[edge]*channels
			parameter := o.parameterIndex[edge]
			weight := weights[parameter]
			for channel := 0; channel < channels; channel++ {
				source := sourceBase + channel
				target := targetBase + channel
				upstream := gradOutput[target]

				weightContribution := upstream * input[source]
				if !isFinite(weightContribution) {
					return nil, nil, fmt.Errorf("sparse: weight gradient multiplication overflow at batch %d edge %d channel %d", b, edge, channel)
				}
				weightSum := gradWeight[parameter] + weightContribution
				if !isFinite(weightSum) {
					return nil, nil, fmt.Errorf("sparse: weight gradient accumulation overflow at parameter %d", parameter)
				}
				gradWeight[parameter] = weightSum

				inputContribution := upstream * weight
				if !isFinite(inputContribution) {
					return nil, nil, fmt.Errorf("sparse: input gradient multiplication overflow at batch %d edge %d channel %d", b, edge, channel)
				}
				inputSum := gradInput[source] + inputContribution
				if !isFinite(inputSum) {
					return nil, nil, fmt.Errorf("sparse: input gradient accumulation overflow at batch %d edge %d channel %d", b, edge, channel)
				}
				gradInput[source] = inputSum
			}
		}
	}
	return gradWeight, gradInput, nil
}

func (o *Operator) shape(batch, channels int) (total, stride int, err error) {
	if o == nil {
		return 0, 0, fmt.Errorf("sparse: nil operator")
	}
	if batch <= 0 {
		return 0, 0, fmt.Errorf("sparse: batch must be positive, got %d", batch)
	}
	if channels <= 0 {
		return 0, 0, fmt.Errorf("sparse: channels must be positive, got %d", channels)
	}
	stride, err = checkedMul(o.nodes, channels)
	if err != nil {
		return 0, 0, fmt.Errorf("sparse: node/channel shape overflow: %w", err)
	}
	total, err = checkedMul(batch, stride)
	if err != nil {
		return 0, 0, fmt.Errorf("sparse: batch shape overflow: %w", err)
	}
	return total, stride, nil
}

func (o *Operator) validateWeights(weights []float64) error {
	if o == nil {
		return fmt.Errorf("sparse: nil operator")
	}
	if len(weights) != o.parameterCount {
		return fmt.Errorf("sparse: weight length must be %d, got %d", o.parameterCount, len(weights))
	}
	return nil
}

func parameterCount(indices []int) (int, error) {
	if len(indices) == 0 {
		return 0, nil
	}
	maxInt := int(^uint(0) >> 1)
	maximum := 0
	for edge, index := range indices {
		if index < 0 {
			return 0, fmt.Errorf("sparse: parameter index at edge %d must be non-negative, got %d", edge, index)
		}
		if index == maxInt {
			return 0, fmt.Errorf("sparse: parameter index at edge %d overflows parameter count", edge)
		}
		if index > maximum {
			maximum = index
		}
	}
	return maximum + 1, nil
}

func checkedMul(a, b int) (int, error) {
	if a < 0 || b < 0 {
		return 0, fmt.Errorf("negative dimension %d*%d", a, b)
	}
	if a == 0 || b == 0 {
		return 0, nil
	}
	maxInt := int(^uint(0) >> 1)
	if a > maxInt/b {
		return 0, fmt.Errorf("dimension multiplication overflows int: %d*%d", a, b)
	}
	return a * b, nil
}

func validateFinite(name string, values []float64) error {
	for index, value := range values {
		if !isFinite(value) {
			return fmt.Errorf("sparse: %s[%d] must be finite, got %v", name, index, value)
		}
	}
	return nil
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func cloneInts(values []int) []int {
	if values == nil {
		return nil
	}
	copyOfValues := make([]int, len(values))
	copy(copyOfValues, values)
	return copyOfValues
}
