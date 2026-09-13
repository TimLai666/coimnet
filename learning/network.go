// Package learning joins Insyra input/readout tapes with the sparse recurrent
// core through explicit vector-Jacobian products, without a gradient bypass.
package learning

import (
	"context"
	"fmt"
	"math"

	"github.com/HazelnutParadise/insyra/nn"
	"github.com/TimLai666/coimnet/dynamics"
)

// Config selects a continuous CPU model and the neurons readable by its output.
// The readout receives only current core activity, never raw observations.
type Config struct {
	Dynamics     dynamics.Config `json:"dynamics"`
	InputSize    int             `json:"input_size"`
	OutputSize   int             `json:"output_size"`
	ReadoutNodes []int           `json:"readout_nodes"`
}

// Parameters uses row-major encoder [input,node] and readout [selected,output].
// The float64 storage is converted with validation at Insyra's float32 boundary.
type Parameters struct {
	Core    dynamics.Parameters `json:"core"`
	Encoder []float64           `json:"encoder"`
	Readout []float64           `json:"readout"`
}

// Gradient follows Parameters and includes gradients with respect to observations.
type Gradient struct {
	Core             dynamics.Gradient
	Encoder, Readout []float64
	Inputs           [][]float64
}

// Network is immutable; callers own parameter versions and independent episodes.
type Network struct {
	config Config
	core   *dynamics.Continuous
}

// NewNetwork validates dimensions and copies connectivity and readout selection.
func NewNetwork(c Config) (*Network, error) {
	if c.InputSize <= 0 || c.OutputSize <= 0 || len(c.ReadoutNodes) == 0 {
		return nil, fmt.Errorf("input, output and readout dimensions must be positive")
	}
	core, err := dynamics.NewContinuous(c.Dynamics)
	if err != nil {
		return nil, err
	}
	if _, err := size(c.InputSize, c.Dynamics.Nodes); err != nil {
		return nil, err
	}
	if _, err := size(len(c.ReadoutNodes), c.OutputSize); err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	for _, id := range c.ReadoutNodes {
		if id < 0 || id >= c.Dynamics.Nodes || seen[id] {
			return nil, fmt.Errorf("invalid or duplicate readout neuron %d", id)
		}
		seen[id] = true
	}
	c.Dynamics = core.Config()
	c.ReadoutNodes = append([]int(nil), c.ReadoutNodes...)
	return &Network{c, core}, nil
}

// Config returns an independent copy of the complete model configuration.
// A nil or zero-value Network returns Config{}.
func (n *Network) Config() Config {
	if n == nil {
		return Config{}
	}
	c := n.config
	if n.core != nil {
		c.Dynamics = n.core.Config()
	} else {
		c.Dynamics.Sources = append([]int(nil), c.Dynamics.Sources...)
		c.Dynamics.Targets = append([]int(nil), c.Dynamics.Targets...)
		c.Dynamics.Delays = append([]int(nil), c.Dynamics.Delays...)
	}
	c.ReadoutNodes = append([]int(nil), c.ReadoutNodes...)
	return c
}

type execution struct {
	encoderTape, readoutTape                    *nn.Tape
	x, encoder, encoded, h, readout, prediction *nn.Tensor
	trace                                       *dynamics.Trace
}

// Predict starts an independent episode at zero voltage and returns only the
// last-step readout. Episode lifecycle and float32 boundaries are explicit.
func (n *Network) Predict(ctx context.Context, p Parameters, input [][]float64) ([]float64, error) {
	e, err := n.forward(ctx, p, input)
	if err != nil {
		return nil, err
	}
	return doubles(e.prediction.Data()), nil
}

// LossGradient computes mean squared error and derivatives through Insyra's
// readout, the recurrent core and Insyra's input encoder. No update occurs.
// The core uses float64; Insyra encoders/loss/readout use float32.
func (n *Network) LossGradient(ctx context.Context, p Parameters, input [][]float64, target []float64, window int) (float64, Gradient, error) {
	var empty Gradient
	if n == nil {
		return 0, empty, fmt.Errorf("nil network")
	}
	if window < 0 {
		return 0, empty, fmt.Errorf("negative truncation window")
	}
	targetTensor, err := tensor([]int{n.config.OutputSize}, target)
	if err != nil {
		return 0, empty, err
	}
	e, err := n.forward(ctx, p, input)
	if err != nil {
		return 0, empty, err
	}
	loss, err := e.readoutTape.MSELoss(e.prediction, targetTensor)
	if err != nil {
		return 0, empty, err
	}
	if err = e.readoutTape.Backward(loss); err != nil {
		return 0, empty, err
	}
	dh, err := e.readoutTape.Grad(e.h)
	if err != nil {
		return 0, empty, err
	}
	dr, err := e.readoutTape.Grad(e.readout)
	if err != nil {
		return 0, empty, err
	}
	up := make([][]float64, len(input))
	for i := range up {
		up[i] = make([]float64, n.config.Dynamics.Nodes)
	}
	hGradient := dh.Data()
	for j, id := range n.config.ReadoutNodes {
		up[len(up)-1][id] = float64(hGradient[j])
	}
	cg, err := n.core.Backward(ctx, e.trace, up, window)
	if err != nil {
		return 0, empty, err
	}
	// Seed the encoder VJP with <encoded, stop_gradient(core_input_gradient)>.
	// This scalar is a reverse-pass device, not the optimization objective.
	flat := make([]float64, 0, len(input)*n.config.Dynamics.Nodes)
	for _, row := range cg.Inputs {
		flat = append(flat, row...)
	}
	seed, err := tensor([]int{len(flat)}, flat)
	if err != nil {
		return 0, empty, err
	}
	encodedFlat, err := e.encoderTape.Reshape(e.encoded, []int{len(flat)})
	if err != nil {
		return 0, empty, err
	}
	seedLoss, err := e.encoderTape.MatMul(encodedFlat, seed)
	if err != nil {
		return 0, empty, err
	}
	if err = e.encoderTape.Backward(seedLoss); err != nil {
		return 0, empty, err
	}
	de, err := e.encoderTape.Grad(e.encoder)
	if err != nil {
		return 0, empty, err
	}
	dx, err := e.encoderTape.Grad(e.x)
	if err != nil {
		return 0, empty, err
	}
	g := Gradient{Core: cg, Encoder: doubles(de.Data()), Readout: doubles(dr.Data()), Inputs: rows(doubles(dx.Data()), n.config.InputSize)}
	l := float64(loss.Data()[0])
	if !finite(l) {
		return 0, empty, fmt.Errorf("non-finite loss")
	}
	for _, v := range [][]float64{g.Encoder, g.Readout} {
		for _, x := range v {
			if !finite(x) {
				return 0, empty, fmt.Errorf("non-finite boundary gradient")
			}
		}
	}
	for _, row := range g.Inputs {
		for _, x := range row {
			if !finite(x) {
				return 0, empty, fmt.Errorf("non-finite input gradient")
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, empty, err
	}
	return l, g, nil
}

func (n *Network) forward(ctx context.Context, p Parameters, input [][]float64) (*execution, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if n == nil {
		return nil, fmt.Errorf("nil network")
	}
	if len(input) == 0 {
		return nil, fmt.Errorf("empty input sequence")
	}
	count, err := size(len(input), n.config.InputSize)
	if err != nil {
		return nil, err
	}
	if _, err := size(len(input), n.config.Dynamics.Nodes); err != nil {
		return nil, err
	}
	flat := make([]float64, 0, count)
	for i, row := range input {
		if len(row) != n.config.InputSize {
			return nil, fmt.Errorf("input[%d] width %d, want %d", i, len(row), n.config.InputSize)
		}
		flat = append(flat, row...)
	}
	x, err := tensor([]int{len(input), n.config.InputSize}, flat)
	if err != nil {
		return nil, err
	}
	encoder, err := tensor([]int{n.config.InputSize, n.config.Dynamics.Nodes}, p.Encoder)
	if err != nil {
		return nil, err
	}
	et := nn.NewTape()
	if _, err = et.Param(x); err != nil {
		return nil, err
	}
	if _, err = et.Param(encoder); err != nil {
		return nil, err
	}
	z, err := et.MatMul(x, encoder)
	if err != nil {
		return nil, err
	}
	tr, err := n.core.Forward(ctx, p.Core, make([]float64, n.config.Dynamics.Nodes), rows(doubles(z.Data()), n.config.Dynamics.Nodes))
	if err != nil {
		return nil, err
	}
	y := tr.Outputs()
	selected := make([]float64, len(n.config.ReadoutNodes))
	for i, id := range n.config.ReadoutNodes {
		selected[i] = y[len(y)-1][id]
	}
	h, err := tensor([]int{len(selected)}, selected)
	if err != nil {
		return nil, err
	}
	r, err := tensor([]int{len(selected), n.config.OutputSize}, p.Readout)
	if err != nil {
		return nil, err
	}
	rt := nn.NewTape()
	if _, err = rt.Param(h); err != nil {
		return nil, err
	}
	if _, err = rt.Param(r); err != nil {
		return nil, err
	}
	pred, err := rt.MatMul(h, r)
	if err != nil {
		return nil, err
	}
	for _, v := range pred.Data() {
		if !finite(float64(v)) {
			return nil, fmt.Errorf("non-finite prediction")
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &execution{et, rt, x, encoder, z, h, r, pred, tr}, nil
}

func tensor(shape []int, values []float64) (*nn.Tensor, error) {
	count := 1
	for _, dim := range shape {
		var err error
		count, err = size(count, dim)
		if err != nil {
			return nil, err
		}
	}
	if len(values) != count {
		return nil, fmt.Errorf("tensor has %d values, want %d for %v", len(values), count, shape)
	}
	f := make([]float32, len(values))
	for i, v := range values {
		if !finite(v) || math.Abs(v) > math.MaxFloat32 {
			return nil, fmt.Errorf("tensor[%d] cannot be represented as finite float32", i)
		}
		f[i] = float32(v)
	}
	return nn.NewTensor(shape, f)
}
func doubles(v []float32) []float64 {
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = float64(x)
	}
	return out
}
func rows(v []float64, width int) [][]float64 {
	out := make([][]float64, len(v)/width)
	for i := range out {
		out[i] = v[i*width : (i+1)*width]
	}
	return out
}
func size(a, b int) (int, error) {
	if a <= 0 || b <= 0 || a > int(^uint(0)>>1)/b {
		return 0, fmt.Errorf("invalid or overflowing shape %d x %d", a, b)
	}
	return a * b, nil
}
func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
