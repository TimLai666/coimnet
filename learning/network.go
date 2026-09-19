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

// Config selects exactly one CPU core, the neurons receiving encoded input,
// and the neurons readable by its output. Dynamics declares the continuous
// core, LIF the spiking core and Mixed the core whose nodes follow either rule
// under one clock; setting more than one, or none, is an error. The readout
// receives only current core activity, never raw observations.
type Config struct {
	Dynamics dynamics.Config     `json:"dynamics"`
	LIF      *dynamics.LIFConfig `json:"lif,omitempty"`
	// Mixed declares the by-type mixed core. Whole-brain uniform mixing is not
	// a default and is not supported by any experiment here; it is an optional
	// mechanism a caller selects deliberately.
	Mixed *dynamics.MixedConfig `json:"mixed,omitempty"`
	// LIFIndex maps each theta_raw entry to its node, in ascending node order.
	// Only the mixed core owns one, because only there does the threshold group
	// cover a subset of the nodes; the other two cores leave it absent, so every
	// configuration written before this field existed keeps its exact canonical
	// JSON and therefore its recorded fingerprint. Network.Config fills it in,
	// and a declared value that contradicts the rule assignment is refused
	// rather than replaced.
	LIFIndex     []int `json:"lif_index,omitempty"`
	InputSize    int   `json:"input_size"`
	OutputSize   int   `json:"output_size"`
	ReadoutNodes []int `json:"readout_nodes"`
	InputNodes   []int `json:"input_nodes,omitempty"`
	// EdgeSigns fixes the sign of individual edges. +1 or -1 switches that
	// edge to the log-magnitude parametrization, 0 leaves it free, and a nil
	// or empty array leaves every edge free. Its length is the edge count.
	EdgeSigns []int8 `json:"edge_signs,omitempty"`
	// MinLogMagnitude is the floor a fixed-sign edge's log magnitude is
	// projected to after each update. Zero selects the declared default -20.
	MinLogMagnitude float64 `json:"min_log_magnitude,omitempty"`
}

// Parameters uses a row-major encoder [input,input-node] (or [input,node]
// when InputNodes is nil) and readout [selected,output]. The float64 storage is
// converted with validation at Insyra's float32 boundary. ThetaRaw is the
// bounded base-threshold parameter of a LIF core, one value per neuron; a
// continuous core must leave it empty.
type Parameters struct {
	Core     dynamics.Parameters `json:"core"`
	ThetaRaw []float64           `json:"theta_raw,omitempty"`
	Encoder  []float64           `json:"encoder"`
	Readout  []float64           `json:"readout"`
}

// Gradient follows Parameters and includes gradients with respect to observations.
type Gradient struct {
	Core             dynamics.Gradient
	ThetaRaw         []float64
	Encoder, Readout []float64
	Inputs           [][]float64
}

// Network is immutable; callers own parameter versions and independent episodes.
// fixed caches whether any edge left the free parametrization, so a model
// without fixed signs never pays for the conversion.
type Network struct {
	config Config
	core   coreModel
	fixed  bool
}

// NewNetwork validates dimensions and copies connectivity and readout selection.
func NewNetwork(c Config) (*Network, error) {
	if c.InputSize <= 0 || c.OutputSize <= 0 || len(c.ReadoutNodes) == 0 {
		return nil, fmt.Errorf("input, output and readout dimensions must be positive")
	}
	core, err := newCore(&c)
	if err != nil {
		return nil, err
	}
	nodes := core.nodes()
	if c.InputNodes == nil {
		if _, err := size(c.InputSize, nodes); err != nil {
			return nil, err
		}
	} else {
		if len(c.InputNodes) == 0 {
			return nil, fmt.Errorf("input_nodes must be non-empty when specified")
		}
		seenInput := make(map[int]bool, len(c.InputNodes))
		for _, id := range c.InputNodes {
			if id < 0 || id >= nodes || seenInput[id] {
				return nil, fmt.Errorf("invalid or duplicate input neuron %d", id)
			}
			seenInput[id] = true
		}
		if _, err := size(c.InputSize, len(c.InputNodes)); err != nil {
			return nil, err
		}
	}
	if _, err := size(len(c.ReadoutNodes), c.OutputSize); err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	for _, id := range c.ReadoutNodes {
		if id < 0 || id >= nodes || seen[id] {
			return nil, fmt.Errorf("invalid or duplicate readout neuron %d", id)
		}
		seen[id] = true
	}
	if err := validateSignConfig(c, core.edges()); err != nil {
		return nil, err
	}
	c.ReadoutNodes = append([]int(nil), c.ReadoutNodes...)
	c.InputNodes = append([]int(nil), c.InputNodes...)
	c.EdgeSigns = append([]int8(nil), c.EdgeSigns...)
	c.LIFIndex = append([]int(nil), c.LIFIndex...)
	return &Network{c, core, hasFixedSigns(c.EdgeSigns)}, nil
}

// Config returns an independent copy of the complete model configuration.
// A nil or zero-value Network returns Config{}.
func (n *Network) Config() Config {
	if n == nil {
		return Config{}
	}
	c := n.config
	if n.core != nil {
		n.core.fill(&c)
	} else {
		c.Dynamics.Sources = append([]int(nil), c.Dynamics.Sources...)
		c.Dynamics.Targets = append([]int(nil), c.Dynamics.Targets...)
		c.Dynamics.Delays = append([]int(nil), c.Dynamics.Delays...)
		c.LIF = copyLIF(c.LIF)
		c.Mixed = copyMixed(c.Mixed)
	}
	c.ReadoutNodes = append([]int(nil), c.ReadoutNodes...)
	c.InputNodes = append([]int(nil), c.InputNodes...)
	c.EdgeSigns = append([]int8(nil), c.EdgeSigns...)
	c.LIFIndex = append([]int(nil), c.LIFIndex...)
	return c
}

// coreParameters converts stored raw parameters into the values the core
// integrates. A model that declares no fixed sign is returned unchanged, so it
// allocates nothing and stays bit-identical to the behaviour before ticket 17.
func (n *Network) coreParameters(p Parameters) (Parameters, error) {
	if !n.fixed {
		return p, nil
	}
	weights, err := EffectiveWeights(n.config, p)
	if err != nil {
		return Parameters{}, err
	}
	p.Core.Weights = weights
	return p, nil
}

type execution struct {
	encoderTape, readoutTape                    *nn.Tape
	x, encoder, encoded, h, readout, prediction *nn.Tensor
	trace                                       coreTrace
	// weights are the effective weights this pass integrated, kept so the
	// reverse pass can apply the log-magnitude chain rule without recomputing
	// them. It is nil when no edge carries a fixed sign.
	weights []float64
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

// spikeEvents runs one independent frozen episode and returns the 0/1 events of
// every neuron after each step. Only a LIF core produces events.
func (n *Network) spikeEvents(ctx context.Context, p Parameters, input [][]float64) ([][]float64, error) {
	e, err := n.forward(ctx, p, input)
	if err != nil {
		return nil, err
	}
	return n.core.events(e.trace)
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
	// Seed the shared reverse pass with the MSE VJP dL/dy from Insyra's own
	// tape, so the refactored path carries the same bits Backward(loss) used to
	// seed the readout with. Only the last row matters: the readout predicts
	// the final step alone.
	up := make([][]float64, len(input))
	for i := range up {
		up[i] = make([]float64, n.config.OutputSize)
	}
	mse := nn.NewTape()
	param, err := mse.Param(e.prediction)
	if err != nil {
		return 0, empty, err
	}
	upLoss, err := mse.MSELoss(param.Value(), targetTensor)
	if err != nil {
		return 0, empty, err
	}
	if err = mse.Backward(upLoss); err != nil {
		return 0, empty, err
	}
	up[len(up)-1] = doubles(param.Grad().Data())
	g, err := n.lossGradientReverse(ctx, input, e, up, window)
	if err != nil {
		return 0, empty, err
	}
	l := float64(loss.Data()[0])
	if !finite(l) {
		return 0, empty, fmt.Errorf("non-finite loss")
	}
	return l, g, nil
}

// LossGradientFrom runs one forward pass and the reverse pass with a
// caller-supplied upstream gradient dL/dy of the same shape as the readout
// output ([]step][output]). It returns the same Gradient shape as
// LossGradient. The loss value itself is unknown here, so it is not returned.
func (n *Network) LossGradientFrom(ctx context.Context, p Parameters, input, upstream [][]float64) (Gradient, error) {
	var empty Gradient
	if n == nil {
		return empty, fmt.Errorf("nil network")
	}
	e, err := n.forward(ctx, p, input)
	if err != nil {
		return empty, err
	}
	return n.lossGradientReverse(ctx, input, e, upstream, 0)
}

// lossGradientReverse is the shared reverse pass both gradient entry points
// drive: seed the readout VJP with the last upstream row, backpropagate through
// the readout, the recurrent core and the encoder. The readout reads only the
// final step, so upstream rows before the last are shape-checked, validated as
// finite and otherwise unused.
func (n *Network) lossGradientReverse(ctx context.Context, input [][]float64, e *execution, upstream [][]float64, window int) (Gradient, error) {
	var empty Gradient
	if len(upstream) != len(input) {
		return empty, fmt.Errorf("upstream has %d rows, want %d", len(upstream), len(input))
	}
	for i, row := range upstream {
		if len(row) != n.config.OutputSize {
			return empty, fmt.Errorf("upstream[%d] width %d, want %d", i, len(row), n.config.OutputSize)
		}
		for j, v := range row {
			if !finite(v) {
				return empty, fmt.Errorf("upstream[%d][%d] is not finite", i, j)
			}
		}
	}
	seed, err := tensor([]int{n.config.OutputSize}, upstream[len(upstream)-1])
	if err != nil {
		return empty, err
	}
	dot, err := e.readoutTape.MatMul(e.prediction, seed)
	if err != nil {
		return empty, err
	}
	if err = e.readoutTape.Backward(dot); err != nil {
		return empty, err
	}
	dh, err := e.readoutTape.Grad(e.h)
	if err != nil {
		return empty, err
	}
	dr, err := e.readoutTape.Grad(e.readout)
	if err != nil {
		return empty, err
	}
	nodes := configNodes(n.config)
	up := make([][]float64, len(input))
	for i := range up {
		up[i] = make([]float64, nodes)
	}
	hGradient := dh.Data()
	for j, id := range n.config.ReadoutNodes {
		up[len(up)-1][id] = float64(hGradient[j])
	}
	cg, err := n.core.backward(ctx, e.trace, up, window)
	if err != nil {
		return empty, err
	}
	// A fixed-sign edge stores rho, not w. The core differentiated with respect
	// to w = sign * exp(rho), so the chain rule gives
	// d loss/d rho = d loss/d w * d w/d rho = d loss/d w * w. A free edge stores
	// its weight directly and needs no conversion.
	if n.fixed {
		if len(cg.core.Weights) != len(n.config.EdgeSigns) || len(e.weights) != len(n.config.EdgeSigns) {
			return empty, fmt.Errorf("weight gradient has %d values, the core declares %d signs", len(cg.core.Weights), len(n.config.EdgeSigns))
		}
		for i, sign := range n.config.EdgeSigns {
			if sign != 0 {
				cg.core.Weights[i] *= e.weights[i]
			}
		}
	}
	// Seed the encoder VJP with <encoded, stop_gradient(core_input_gradient)>.
	// This scalar is a reverse-pass device, not the optimization objective.
	encoderWidth := inputWidth(n.config)
	flat := make([]float64, 0, len(input)*encoderWidth)
	for _, row := range cg.core.Inputs {
		if n.config.InputNodes == nil {
			flat = append(flat, row...)
			continue
		}
		for _, id := range n.config.InputNodes {
			flat = append(flat, row[id])
		}
	}
	seed, err = tensor([]int{len(flat)}, flat)
	if err != nil {
		return empty, err
	}
	encodedFlat, err := e.encoderTape.Reshape(e.encoded, []int{len(flat)})
	if err != nil {
		return empty, err
	}
	seedLoss, err := e.encoderTape.MatMul(encodedFlat, seed)
	if err != nil {
		return empty, err
	}
	if err = e.encoderTape.Backward(seedLoss); err != nil {
		return empty, err
	}
	de, err := e.encoderTape.Grad(e.encoder)
	if err != nil {
		return empty, err
	}
	dx, err := e.encoderTape.Grad(e.x)
	if err != nil {
		return empty, err
	}
	g := Gradient{Core: cg.core, ThetaRaw: cg.thetaRaw, Encoder: doubles(de.Data()), Readout: doubles(dr.Data()), Inputs: rows(doubles(dx.Data()), n.config.InputSize)}
	for _, v := range [][]float64{g.Encoder, g.Readout} {
		for _, x := range v {
			if !finite(x) {
				return empty, fmt.Errorf("non-finite boundary gradient")
			}
		}
	}
	for _, row := range g.Inputs {
		for _, x := range row {
			if !finite(x) {
				return empty, fmt.Errorf("non-finite input gradient")
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	return g, nil
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
	nodes := configNodes(n.config)
	if _, err := size(len(input), nodes); err != nil {
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
	encoderWidth := inputWidth(n.config)
	encoder, err := tensor([]int{n.config.InputSize, encoderWidth}, p.Encoder)
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
	encodedRows := rows(doubles(z.Data()), encoderWidth)
	coreInputs := encodedRows
	if n.config.InputNodes != nil {
		coreInputs = make([][]float64, len(encodedRows))
		for t, row := range encodedRows {
			coreInputs[t] = make([]float64, nodes)
			for i, id := range n.config.InputNodes {
				coreInputs[t][id] = row[i]
			}
		}
	}
	core, err := n.coreParameters(p)
	if err != nil {
		return nil, err
	}
	tr, y, err := n.core.forward(ctx, core, make([]float64, nodes), coreInputs)
	if err != nil {
		return nil, err
	}
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
	effective := []float64(nil)
	if n.fixed {
		effective = core.Core.Weights
	}
	return &execution{et, rt, x, encoder, z, h, r, pred, tr, effective}, nil
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

func inputWidth(c Config) int {
	if c.InputNodes != nil {
		return len(c.InputNodes)
	}
	return configNodes(c)
}

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
