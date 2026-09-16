package dynamics

import "fmt"

// VectorLayout describes how per-node and per-edge arrays are laid out
// for StateDimension C: node arrays hold N*C values (node-major), edge
// weights hold E values (scalar) or E*C*C values (matrix, row-major per edge).
type VectorLayout struct {
	Nodes, Edges, C int
	Matrix          bool
}

// NewVectorLayout returns the layout for the given anatomy. The state
// dimension C must be at least 1; an empty edge shape means scalar.
func NewVectorLayout(nodes, edges, stateDimension int, edgeShape string) (VectorLayout, error) {
	if nodes < 0 {
		return VectorLayout{}, fmt.Errorf("node count %d must be non-negative", nodes)
	}
	if edges < 0 {
		return VectorLayout{}, fmt.Errorf("edge count %d must be non-negative", edges)
	}
	if stateDimension < 1 {
		return VectorLayout{}, fmt.Errorf("state dimension %d must be at least 1", stateDimension)
	}
	switch edgeShape {
	case "", "scalar", "matrix":
	default:
		return VectorLayout{}, fmt.Errorf("unsupported edge shape %q", edgeShape)
	}
	return VectorLayout{Nodes: nodes, Edges: edges, C: stateDimension, Matrix: edgeShape == "matrix"}, nil
}

// NodeValues reports the number of node-major state values, N*C.
func (l VectorLayout) NodeValues() int { return l.Nodes * l.C }

// WeightValues reports the number of edge weight values: E for scalar, or
// E*C*C for a row-major per-edge matrix.
func (l VectorLayout) WeightValues() int {
	if l.Matrix {
		return l.Edges * l.C * l.C
	}
	return l.Edges
}

// NodeSlice returns the C values of one node inside the node-major array.
func (l VectorLayout) NodeSlice(values []float64, node int) []float64 {
	return values[node*l.C : (node+1)*l.C]
}

// EdgeMatrix returns the weights of one edge. For scalar edges the single
// broadcast value is returned as a one-element slice and the diagonal is not
// materialised; for matrix edges the row-major C*C block is returned.
func (l VectorLayout) EdgeMatrix(weights []float64, edge int) []float64 {
	if !l.Matrix {
		return weights[edge : edge+1]
	}
	return weights[edge*l.C*l.C : (edge+1)*l.C*l.C]
}

// ApplyEdge adds the contribution of edge e with source output source (length
// C) into drive (length C). Scalar edges broadcast the single weight to every
// component: drive += w*y; matrix edges apply the row-major C*C block:
// drive[r] += sum_c W[r*C+c]*y[c]. For C == 1 scalar edges the multiply-add is
// bit-identical to the eager forward loop.
func (l VectorLayout) ApplyEdge(weights []float64, edge int, source, drive []float64) error {
	if l.C <= 0 {
		return fmt.Errorf("layout has state dimension 0")
	}
	if edge < 0 || edge >= l.Edges {
		return fmt.Errorf("edge %d out of range [0, %d)", edge, l.Edges)
	}
	if len(source) < l.C {
		return fmt.Errorf("source length %d, want at least %d", len(source), l.C)
	}
	if len(drive) < l.C {
		return fmt.Errorf("drive length %d, want at least %d", len(drive), l.C)
	}
	if l.Matrix {
		start := edge * l.C * l.C
		if len(weights) < start+l.C*l.C {
			return fmt.Errorf("weights length %d, want at least %d", len(weights), start+l.C*l.C)
		}
		for r := 0; r < l.C; r++ {
			row := start + r*l.C
			var sum float64
			for c := 0; c < l.C; c++ {
				sum += weights[row+c] * source[c]
			}
			drive[r] += sum
		}
		return nil
	}
	if len(weights) < edge+1 {
		return fmt.Errorf("weights length %d, want at least %d", len(weights), edge+1)
	}
	w := weights[edge]
	for i := 0; i < l.C; i++ {
		drive[i] += w * source[i]
	}
	return nil
}

// ParameterCount reports weights + biases + log_tau capacities only: weights
// follow the layout (E or E*C*C), bias is N*C and log_tau is N.
func (l VectorLayout) ParameterCount() (weights, bias, logTau int) {
	return l.WeightValues(), l.Nodes * l.C, l.Nodes
}
