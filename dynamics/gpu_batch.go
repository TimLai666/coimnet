package dynamics

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/TimLai666/coimnet/backend/webgpu"
)

// BatchLayout selects which nodes a batched episode feeds and reads.
// InputNodes names the node of each input column; nil means every node, in
// node order. ReadoutNodes names the nodes Forward returns, after every step
// with ReadoutEveryStep and after the last step otherwise.
type BatchLayout struct {
	InputNodes       []int
	ReadoutNodes     []int
	ReadoutEveryStep bool
}

// BatchGradient is the reverse pass of one batched episode. The parameter
// groups are summed over every sequence of the batch, exactly the sum of the
// per-sequence Backward gradients; GateGain and GateBias are nil without a
// gate. Inputs[b][t] is sequence b's gradient of its input columns at step t,
// in BatchLayout.InputNodes order.
type BatchGradient struct {
	Weights, Bias, LogTau []float64
	GateGain, GateBias    []float64
	Inputs                [][][]float64
}

// GPUContinuousBatch runs whole minibatches of fresh, independent episodes of
// a scalar, undelayed Continuous core on the device: Forward integrates every
// step of every sequence in one submission and keeps the history resident,
// and Backward runs full BPTT (window zero) over that history and reduces the
// parameter gradients on the device. Values cross the boundary as float32;
// lambda, alpha and the log_tau slope are computed in float64 on the host.
// The tanh, softsign and softplus activations and every gate are supported.
// Positive delays, vector state, matrix edges, truncated windows, persistent
// state and nonzero initial voltage are refused. There is no CPU fallback.
type GPUContinuousBatch struct {
	mu     sync.Mutex
	model  *Continuous
	layout BatchLayout
	width  int
	exec   *webgpu.ContinuousBatch
	closed bool
	// batch, steps describe the resident episode Backward reads.
	batch, steps int
}

// NewGPUContinuousBatch validates cfg and layout and opens the batched device
// executor. Unsupported configurations are refused before a device is opened.
func NewGPUContinuousBatch(ctx context.Context, cfg Config, layout BatchLayout) (*GPUContinuousBatch, error) {
	if ctx == nil {
		return nil, errors.New("dynamics gpu continuous batch: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateGPUContinuousConfig(cfg); err != nil {
		return nil, err
	}
	model, err := NewContinuous(cfg)
	if err != nil {
		return nil, err
	}
	c := model.Config()
	width := len(layout.InputNodes)
	if layout.InputNodes == nil {
		width = c.Nodes
	}
	exec, err := webgpu.NewContinuousBatch(ctx, webgpu.ContinuousBatchConfig{
		Nodes:            c.Nodes,
		Sources:          c.Sources,
		Targets:          c.Targets,
		Activation:       c.Activation,
		Gate:             c.Gate,
		InputNodes:       append([]int(nil), layout.InputNodes...),
		ReadoutNodes:     append([]int(nil), layout.ReadoutNodes...),
		ReadoutEveryStep: layout.ReadoutEveryStep,
	})
	if err != nil {
		return nil, err
	}
	owned := BatchLayout{ReadoutNodes: append([]int(nil), layout.ReadoutNodes...), ReadoutEveryStep: layout.ReadoutEveryStep}
	if layout.InputNodes != nil {
		owned.InputNodes = append([]int(nil), layout.InputNodes...)
	}
	return &GPUContinuousBatch{model: model, layout: owned, width: width, exec: exec}, nil
}

// batchCoefficients converts the integrator parameters into the float32
// values the device integrates: lambda, alpha and dlambda/dlog_tau.
func batchCoefficients(m *Continuous, p Parameters) (lambda, alpha, slope []float32, err error) {
	l64, a64, err := continuousCoefficients(m, p)
	if err != nil {
		return nil, nil, nil, err
	}
	s64 := make([]float64, len(l64))
	for i, l := range l64 {
		// d exp(-dt/exp(log_tau)) / d log_tau = lambda*dt/tau; an underflowed
		// lambda has zero slope.
		if l != 0 {
			s64[i] = l * (m.config.DT / math.Exp(p.LogTau[i]))
		}
	}
	if lambda, err = gpuFloat32Values(l64, "lambda"); err != nil {
		return nil, nil, nil, err
	}
	if alpha, err = gpuFloat32Values(a64, "alpha"); err != nil {
		return nil, nil, nil, err
	}
	if slope, err = gpuFloat32Values(s64, "log_tau slope"); err != nil {
		return nil, nil, nil, err
	}
	return lambda, alpha, slope, nil
}

// Forward runs one fresh episode per sequence from zero voltage. inputs[b][t]
// holds sequence b's input columns at step t; every sequence must have the
// same nonzero length. It returns readout[b][s][r], with one row per step
// when ReadoutEveryStep is set and only the last step otherwise.
func (g *GPUContinuousBatch) Forward(ctx context.Context, p Parameters, inputs [][][]float64) ([][][]float64, error) {
	if ctx == nil {
		return nil, errors.New("dynamics gpu continuous batch: nil context")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed || g.exec == nil {
		return nil, ErrGPUContinuousClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	g.batch, g.steps = 0, 0
	m := g.model
	n := m.config.Nodes
	if err := vector(p.Weights, len(m.config.Sources), "weights"); err != nil {
		return nil, err
	}
	if err := vector(p.Bias, n, "bias"); err != nil {
		return nil, err
	}
	if err := vector(p.LogTau, n, "log_tau"); err != nil {
		return nil, err
	}
	if err := m.checkGateParameters(p); err != nil {
		return nil, err
	}
	batch := len(inputs)
	if batch == 0 || len(inputs[0]) == 0 {
		return nil, fmt.Errorf("empty batch or sequence")
	}
	steps := len(inputs[0])
	flat := make([]float64, 0, batch*steps*g.width)
	for b, seq := range inputs {
		if len(seq) != steps {
			return nil, fmt.Errorf("sequence %d has %d steps, want %d", b, len(seq), steps)
		}
		for t, row := range seq {
			if len(row) != g.width {
				return nil, fmt.Errorf("input[%d][%d] width %d, want %d", b, t, len(row), g.width)
			}
			flat = append(flat, row...)
		}
	}
	inputs32, err := gpuFloat32Values(flat, "inputs")
	if err != nil {
		return nil, err
	}
	lambda, alpha, slope, err := batchCoefficients(m, p)
	if err != nil {
		return nil, err
	}
	params := webgpu.ContinuousBatchParameters{Lambda: lambda, Alpha: alpha, TauSlope: slope}
	if params.Weights, err = gpuFloat32Values(p.Weights, "weights"); err != nil {
		return nil, err
	}
	if params.Bias, err = gpuFloat32Values(p.Bias, "bias"); err != nil {
		return nil, err
	}
	if m.config.Gate != "" {
		if params.GateGain, err = gpuFloat32Values(p.GateGain, "gate_gain"); err != nil {
			return nil, err
		}
		if params.GateBias, err = gpuFloat32Values(p.GateBias, "gate_bias"); err != nil {
			return nil, err
		}
	}
	values, err := g.exec.Forward(ctx, params, batch, steps, inputs32)
	if err != nil {
		return nil, err
	}
	rowsPer, r := g.readSteps(steps), len(g.layout.ReadoutNodes)
	if len(values) != batch*rowsPer*r {
		return nil, fmt.Errorf("device returned %d readout values, want %d", len(values), batch*rowsPer*r)
	}
	out := make([][][]float64, batch)
	for b := range out {
		out[b] = make([][]float64, rowsPer)
		for s := range out[b] {
			row := make([]float64, r)
			for j := range row {
				row[j] = float64(values[(b*rowsPer+s)*r+j])
			}
			out[b][s] = row
		}
	}
	g.batch, g.steps = batch, steps
	return out, nil
}

func (g *GPUContinuousBatch) readSteps(steps int) int {
	if g.layout.ReadoutEveryStep {
		return steps
	}
	return 1
}

// Backward runs full BPTT over the episode of the last successful Forward.
// upstream[b][s][r] is the loss gradient of Forward's readout[b][s][r].
func (g *GPUContinuousBatch) Backward(ctx context.Context, upstream [][][]float64) (BatchGradient, error) {
	var empty BatchGradient
	if ctx == nil {
		return empty, errors.New("dynamics gpu continuous batch: nil context")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed || g.exec == nil {
		return empty, ErrGPUContinuousClosed
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if g.batch == 0 {
		return empty, fmt.Errorf("backward needs a successful forward first")
	}
	batch, steps, rowsPer, r := g.batch, g.steps, g.readSteps(g.steps), len(g.layout.ReadoutNodes)
	if len(upstream) != batch {
		return empty, fmt.Errorf("upstream has %d sequences, want %d", len(upstream), batch)
	}
	flat := make([]float64, 0, batch*rowsPer*r)
	for b, seq := range upstream {
		if len(seq) != rowsPer {
			return empty, fmt.Errorf("upstream[%d] has %d rows, want %d", b, len(seq), rowsPer)
		}
		for s, row := range seq {
			if len(row) != r {
				return empty, fmt.Errorf("upstream[%d][%d] width %d, want %d", b, s, len(row), r)
			}
			flat = append(flat, row...)
		}
	}
	upstream32, err := gpuFloat32Values(flat, "upstream")
	if err != nil {
		return empty, err
	}
	got, err := g.exec.Backward(ctx, upstream32)
	if err != nil {
		return empty, err
	}
	if len(got.Inputs) != batch*steps*g.width {
		return empty, fmt.Errorf("device returned %d input gradients, want %d", len(got.Inputs), batch*steps*g.width)
	}
	grad := BatchGradient{Weights: doubles32(got.Weights), Bias: doubles32(got.Bias), LogTau: doubles32(got.LogTau), Inputs: make([][][]float64, batch)}
	if g.model.config.Gate != "" {
		grad.GateGain, grad.GateBias = doubles32(got.GateGain), doubles32(got.GateBias)
	}
	for b := range grad.Inputs {
		grad.Inputs[b] = make([][]float64, steps)
		for t := range grad.Inputs[b] {
			base := (b*steps + t) * g.width
			grad.Inputs[b][t] = doubles32(got.Inputs[base : base+g.width])
		}
	}
	return grad, nil
}

func doubles32(v []float32) []float64 {
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = float64(x)
	}
	return out
}

// Info identifies the adapter the executor runs on.
func (g *GPUContinuousBatch) Info() webgpu.DeviceInfo {
	if g == nil || g.exec == nil {
		return webgpu.DeviceInfo{}
	}
	return g.exec.Info()
}

// Metrics returns the executor's construction and latest call measurements.
func (g *GPUContinuousBatch) Metrics() webgpu.ContinuousBatchMetrics {
	if g == nil || g.exec == nil {
		return webgpu.ContinuousBatchMetrics{}
	}
	return g.exec.Metrics()
}

// Close releases the device executor; it is idempotent.
func (g *GPUContinuousBatch) Close() error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil
	}
	g.closed = true
	if g.exec == nil {
		return nil
	}
	return g.exec.Close()
}
