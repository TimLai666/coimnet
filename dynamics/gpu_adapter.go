package dynamics

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/TimLai666/coimnet/backend/webgpu"
)

// Errors returned by the scalar zero-delay GPU adapter.
var (
	ErrGPUContinuousUnsupported         = errors.New("dynamics gpu continuous: unsupported configuration")
	ErrGPUContinuousPrecision           = errors.New("dynamics gpu continuous: value cannot be represented as finite float32")
	ErrGPUContinuousBackwardUnavailable = errors.New("dynamics gpu continuous: backward adapter unavailable")
	ErrGPUContinuousClosed              = errors.New("dynamics gpu continuous: adapter is closed")
)

// GPUContinuous is a scalar, zero-delay Continuous executor backed by real
// WebGPU sparse primitives. It keeps the continuous state and the activation
// derivative on the host in float64, while each sparse drive or sparse
// derivative operation is dispatched through backend/webgpu in float32.
//
// NewGPUContinuous constructs both the forward and backward primitives. The
// Forward-only constructor is provided so a caller can measure the independent
// forward path when a backward shader is unavailable. Neither constructor has
// a CPU fallback.
type GPUContinuous struct {
	mu       sync.Mutex
	model    *Continuous
	drive    *webgpu.SparseDrive
	backward *webgpu.SparseBackward
	closed   bool
}

// NewGPUContinuous constructs a scalar zero-delay adapter with forward and
// reverse primitives. Positive delays, vector state, matrix edges, and other
// unsupported configurations are rejected before any GPU resource is opened.
func NewGPUContinuous(ctx context.Context, cfg Config) (*GPUContinuous, error) {
	return newGPUContinuous(ctx, cfg, true)
}

// NewGPUContinuousForward constructs the independently testable forward-only
// adapter. Backward on its result returns ErrGPUContinuousBackwardUnavailable.
func NewGPUContinuousForward(ctx context.Context, cfg Config) (*GPUContinuous, error) {
	return newGPUContinuous(ctx, cfg, false)
}

func newGPUContinuous(ctx context.Context, cfg Config, withBackward bool) (*GPUContinuous, error) {
	if ctx == nil {
		return nil, errors.New("dynamics gpu continuous: nil context")
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
	modelConfig := model.Config()
	topology := webgpu.Config{
		Nodes:   modelConfig.Nodes,
		Sources: append([]int(nil), modelConfig.Sources...),
		Targets: append([]int(nil), modelConfig.Targets...),
	}
	drive, err := webgpu.NewSparseDrive(ctx, topology)
	if err != nil {
		return nil, err
	}
	gpu := &GPUContinuous{model: model, drive: drive}
	if !withBackward {
		return gpu, nil
	}
	backward, err := webgpu.NewSparseBackward(ctx, topology)
	if err != nil {
		_ = drive.Close()
		return nil, fmt.Errorf("%w: %w", ErrGPUContinuousBackwardUnavailable, err)
	}
	if err := verifyGPUBackwardPrimitive(ctx, backward, topology); err != nil {
		_ = backward.Close()
		_ = drive.Close()
		return nil, fmt.Errorf("%w: %v", ErrGPUContinuousBackwardUnavailable, err)
	}
	gpu.backward = backward
	return gpu, nil
}

// verifyGPUBackwardPrimitive is a fixed, non-model probe. It prevents the
// adapter from exposing a backward path when the backend has successfully
// compiled and dispatched a shader but returned an incomplete gradient buffer.
// The expected values are computed in float32 on the host only as a capability
// check; they are never used as a fallback for a production Backward call.
func verifyGPUBackwardPrimitive(ctx context.Context, backward *webgpu.SparseBackward, cfg webgpu.Config) error {
	upstream := make([]float32, cfg.Nodes)
	sourceOutput := make([]float32, cfg.Nodes)
	weights := make([]float32, len(cfg.Sources))
	for i := range upstream {
		upstream[i] = float32((i%7)-3) / 5
		sourceOutput[i] = float32((i%11)-5) / 7
	}
	for edge := range weights {
		weights[edge] = float32((edge%13)-6) / 9
	}
	wantSource := make([]float32, cfg.Nodes)
	wantWeights := make([]float32, len(cfg.Sources))
	for edge, source := range cfg.Sources {
		target := cfg.Targets[edge]
		wantSource[source] = wantSource[source] + weights[edge]*upstream[target]
		wantWeights[edge] = sourceOutput[source] * upstream[target]
	}
	got, err := backward.Backward(ctx, upstream, sourceOutput, weights)
	if err != nil {
		return err
	}
	if len(got.Input) != len(upstream) || len(got.Source) != len(wantSource) || len(got.Weights) != len(wantWeights) {
		return fmt.Errorf("probe result shape input=%d source=%d weights=%d, want %d/%d/%d", len(got.Input), len(got.Source), len(got.Weights), len(upstream), len(wantSource), len(wantWeights))
	}
	for i := range upstream {
		if got.Input[i] != upstream[i] {
			return fmt.Errorf("probe input gradient[%d] = %g, want %g", i, got.Input[i], upstream[i])
		}
		if got.Source[i] != wantSource[i] {
			return fmt.Errorf("probe source gradient[%d] = %g, want %g", i, got.Source[i], wantSource[i])
		}
	}
	for edge := range wantWeights {
		if got.Weights[edge] != wantWeights[edge] {
			return fmt.Errorf("probe weight gradient[%d] = %g, want %g", edge, got.Weights[edge], wantWeights[edge])
		}
	}
	return nil
}

func validateGPUContinuousConfig(cfg Config) error {
	if cfg.StateDimension < 0 || cfg.StateDimension > 1 {
		return fmt.Errorf("%w: only scalar state is supported, state dimension %d", ErrGPUContinuousUnsupported, cfg.StateDimension)
	}
	if cfg.EdgeShape != "" && cfg.EdgeShape != "scalar" {
		return fmt.Errorf("%w: edge shape %q is not scalar", ErrGPUContinuousUnsupported, cfg.EdgeShape)
	}
	if len(cfg.Delays) == len(cfg.Sources) {
		for edge, delay := range cfg.Delays {
			if delay != 0 {
				return fmt.Errorf("%w: edge %d has delay %d; only zero delay is supported", ErrGPUContinuousUnsupported, edge, delay)
			}
		}
	}
	return nil
}

// Forward evolves inputs using the GPU sparse drive for every recurrent step.
// The state transition, bias, activation, and time constants are evaluated in
// float64 on the host to retain the Continuous state contract. No CPU sparse
// drive is called when the GPU operation fails.
func (g *GPUContinuous) Forward(ctx context.Context, p Parameters, initial []float64, inputs [][]float64) (*Trace, error) {
	var empty *Trace
	if ctx == nil {
		return empty, errors.New("dynamics gpu continuous: nil context")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return empty, ErrGPUContinuousClosed
	}
	if g.model == nil || g.drive == nil {
		return empty, ErrGPUContinuousClosed
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	m := g.model
	n := m.config.Nodes
	if len(inputs) == 0 {
		return empty, fmt.Errorf("empty sequence")
	}
	if err := vector(initial, n, "initial voltage"); err != nil {
		return empty, err
	}
	if err := vector(p.Weights, len(m.config.Sources), "weights"); err != nil {
		return empty, err
	}
	if err := vector(p.Bias, n, "bias"); err != nil {
		return empty, err
	}
	if err := vector(p.LogTau, n, "log_tau"); err != nil {
		return empty, err
	}
	if len(inputs) > int(^uint(0)>>1)/n/3-1 {
		return empty, fmt.Errorf("sequence shape overflows")
	}
	lambda, alpha, err := continuousCoefficients(m, p)
	if err != nil {
		return empty, err
	}
	weights32, err := gpuFloat32Values(p.Weights, "weights")
	if err != nil {
		return empty, err
	}
	for t, in := range inputs {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		if len(in) != n {
			return empty, fmt.Errorf("input[%d] length %d, want %d", t, len(in), n)
		}
		if _, err := gpuFloat32Values(in, fmt.Sprintf("input[%d]", t)); err != nil {
			return empty, err
		}
	}

	tr := &Trace{
		model:      m,
		parameters: cloneParameters(p),
		voltage:    make([][]float64, len(inputs)+1),
		output:     make([][]float64, len(inputs)+1),
		drive:      make([][]float64, len(inputs)),
		lambda:     lambda,
		alpha:      alpha,
	}
	tr.voltage[0] = append([]float64(nil), initial...)
	tr.output[0] = make([]float64, n)
	for i, v := range initial {
		tr.output[0][i] = m.activate(v)
	}

	for t, in := range inputs {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		input32, err := gpuFloat32Values(in, fmt.Sprintf("input[%d]", t))
		if err != nil {
			return empty, err
		}
		source32, err := gpuFloat32Values(tr.output[t], fmt.Sprintf("output[%d]", t))
		if err != nil {
			return empty, err
		}
		result, err := g.drive.Execute(ctx, input32, source32, weights32)
		if err != nil {
			return empty, err
		}
		if len(result.Output) != n {
			return empty, fmt.Errorf("webgpu sparse drive returned %d values, want %d", len(result.Output), n)
		}
		drive := make([]float64, n)
		vnext := make([]float64, n)
		ynext := make([]float64, n)
		for i, value := range result.Output {
			drive[i] = float64(value) + p.Bias[i]
			vnext[i] = lambda[i]*tr.voltage[t][i] + alpha[i]*drive[i]
			ynext[i] = m.activate(vnext[i])
			if !finite(drive[i]) || !finite(vnext[i]) || !finite(ynext[i]) {
				return empty, fmt.Errorf("non-finite state at step %d neuron %d", t, i)
			}
		}
		tr.drive[t] = drive
		tr.voltage[t+1] = vnext
		tr.output[t+1] = ynext
	}
	return tr, nil
}

func continuousCoefficients(m *Continuous, p Parameters) ([]float64, []float64, error) {
	n := m.config.Nodes
	lambda := make([]float64, n)
	alpha := make([]float64, n)
	for i, raw := range p.LogTau {
		tau := math.Exp(raw)
		if !finite(tau) || tau <= 0 {
			return nil, nil, fmt.Errorf("tau[%d] is not representable and positive", i)
		}
		lambda[i] = math.Exp(-m.config.DT / tau)
		alpha[i] = -math.Expm1(-m.config.DT / tau)
	}
	return lambda, alpha, nil
}

func gpuFloat32Values(values []float64, name string) ([]float32, error) {
	converted := make([]float32, len(values))
	for i, value := range values {
		converted[i] = float32(value)
		if (value != 0 && converted[i] == 0) || math.IsInf(float64(converted[i]), 0) || math.IsNaN(float64(converted[i])) {
			return nil, fmt.Errorf("%w: %s[%d]=%g", ErrGPUContinuousPrecision, name, i, value)
		}
	}
	return converted, nil
}

// Backward computes the complete or fixed-window BPTT pass. The host applies
// activation and recurrent chain-rule terms; the GPU primitive supplies sparse
// source and per-edge gradients for each zero-delay step.
func (g *GPUContinuous) Backward(ctx context.Context, tr *Trace, upstream [][]float64, window int) (Gradient, error) {
	var empty Gradient
	if ctx == nil {
		return empty, errors.New("dynamics gpu continuous: nil context")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return empty, ErrGPUContinuousClosed
	}
	if g.model == nil || tr == nil || tr.model != g.model {
		return empty, fmt.Errorf("trace belongs to a different model")
	}
	if g.backward == nil {
		return empty, ErrGPUContinuousBackwardUnavailable
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	m := g.model
	steps, n := len(tr.drive), m.config.Nodes
	if window < 0 || len(upstream) != steps {
		return empty, fmt.Errorf("invalid backward window or sequence length")
	}
	weights32, err := gpuFloat32Values(tr.parameters.Weights, "weights")
	if err != nil {
		return empty, err
	}
	gy := make([][]float64, steps+1)
	for t := range gy {
		gy[t] = make([]float64, n)
	}
	for t, row := range upstream {
		if err := vector(row, n, "upstream"); err != nil {
			return empty, err
		}
		copy(gy[t+1], row)
	}
	gradient := Gradient{
		Weights: make([]float64, len(tr.parameters.Weights)),
		Bias:    make([]float64, n),
		LogTau:  make([]float64, n),
		Inputs:  make([][]float64, steps),
		Initial: make([]float64, n),
	}
	gV := make([]float64, n)
	for t := steps - 1; t >= 0; t-- {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		start := 0
		if window > 0 {
			start = t / window * window
		}
		prev := make([]float64, n)
		gradient.Inputs[t] = make([]float64, n)
		for i := 0; i < n; i++ {
			dv := gV[i] + gy[t+1][i]*m.derivative(tr.voltage[t+1][i])
			gradient.Inputs[t][i] = tr.alpha[i] * dv
			gradient.Bias[i] += gradient.Inputs[t][i]
			dlambda := tr.lambda[i] * (m.config.DT / math.Exp(tr.parameters.LogTau[i]))
			if tr.lambda[i] == 0 {
				dlambda = 0
			}
			gradient.LogTau[i] += dv * (tr.voltage[t][i] - tr.drive[t][i]) * dlambda
			if t > start || start == 0 {
				prev[i] = tr.lambda[i] * dv
			}
		}
		upstream32, err := gpuFloat32Values(gradient.Inputs[t], fmt.Sprintf("input gradient[%d]", t))
		if err != nil {
			return empty, err
		}
		source32, err := gpuFloat32Values(tr.output[t], fmt.Sprintf("output[%d]", t))
		if err != nil {
			return empty, err
		}
		result, err := g.backward.Backward(ctx, upstream32, source32, weights32)
		if err != nil {
			return empty, err
		}
		if len(result.Input) != n || len(result.Source) != n || len(result.Weights) != len(gradient.Weights) {
			return empty, fmt.Errorf("webgpu sparse backward returned input=%d source=%d weights=%d, want %d/%d/%d", len(result.Input), len(result.Source), len(result.Weights), n, n, len(gradient.Weights))
		}
		for i, value := range result.Input {
			if value != upstream32[i] {
				return empty, fmt.Errorf("webgpu sparse backward identity gradient[%d] = %g, want %g", i, value, upstream32[i])
			}
		}
		if t > start || start == 0 {
			for i, value := range result.Source {
				gy[t][i] += float64(value)
			}
		}
		for edge, value := range result.Weights {
			gradient.Weights[edge] += float64(value)
		}
		gV = prev
	}
	for i := range gV {
		gradient.Initial[i] = gV[i] + gy[0][i]*m.derivative(tr.voltage[0][i])
	}
	for _, values := range [][]float64{gradient.Weights, gradient.Bias, gradient.LogTau, gradient.Initial} {
		if err := vector(values, len(values), "gradient"); err != nil {
			return empty, err
		}
	}
	for _, values := range gradient.Inputs {
		if err := vector(values, len(values), "input gradient"); err != nil {
			return empty, err
		}
	}
	return gradient, nil
}

// DriveInfo identifies the adapter used by the forward primitive.
func (g *GPUContinuous) DriveInfo() webgpu.DeviceInfo {
	if g == nil || g.drive == nil {
		return webgpu.DeviceInfo{}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.drive.Info()
}

// BackwardInfo identifies the adapter used by the backward primitive. The
// boolean is false for a forward-only adapter.
func (g *GPUContinuous) BackwardInfo() (webgpu.DeviceInfo, bool) {
	if g == nil || g.backward == nil {
		return webgpu.DeviceInfo{}, false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.backward.Info(), true
}

// Close waits for queued GPU work and releases both primitives. It is
// idempotent and prevents any later Forward or Backward call.
func (g *GPUContinuous) Close() error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil
	}
	g.closed = true
	var closeErr error
	if g.backward != nil {
		closeErr = g.backward.Close()
	}
	if err := g.drive.Close(); err != nil {
		closeErr = errors.Join(closeErr, err)
	}
	return closeErr
}
