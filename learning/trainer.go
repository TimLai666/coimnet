package learning

import (
	"context"
	"fmt"
	"math"
	"sync"

	"github.com/TimLai666/coimnet/dynamics"
)

// Trainable selects whole parameter groups. A frozen group retains parameters,
// optimizer moments and per-parameter step counts, including with weight decay.
// Theta is the LIF base-threshold group and is rejected on a continuous core.
type Trainable struct {
	Encoder bool `json:"encoder"`
	Weights bool `json:"weights"`
	Bias    bool `json:"bias"`
	Tau     bool `json:"tau"`
	Theta   bool `json:"theta,omitempty"`
	Readout bool `json:"readout"`
}

// Options specifies a local, serializable AdamW update and BPTT window.
type Options struct {
	LearningRate float64   `json:"learning_rate"`
	Beta1        float64   `json:"beta1"`
	Beta2        float64   `json:"beta2"`
	Epsilon      float64   `json:"epsilon"`
	WeightDecay  float64   `json:"weight_decay"`
	ClipNorm     float64   `json:"clip_norm"`
	Truncation   int       `json:"truncation"`
	Trainable    Trainable `json:"trainable"`
}

// DefaultOptions returns every group of the continuous core trainable and a
// declared AdamW baseline. The LIF threshold group stays frozen; a spiking
// model must enable it explicitly.
func DefaultOptions() Options {
	return Options{.01, .9, .999, 1e-8, 0, 1, 0, Trainable{Encoder: true, Weights: true, Bias: true, Tau: true, Readout: true}}
}

// AdamState is ordered weights, bias, log_tau, theta_raw, encoder, readout.
// A continuous model has no theta_raw values, so its layout and every existing
// snapshot keep their positions. Each parameter counts its own unfrozen
// updates, preserving bias correction across freezes.
type AdamState struct {
	First  []float64 `json:"first"`
	Second []float64 `json:"second"`
	Steps  []uint64  `json:"steps"`
}

// TrainingSnapshot covers the implemented independent-episode training mode.
// Neural state resets per Step; continuous individuals require a different
// snapshot profile and must not be represented by this schema.
type TrainingSnapshot struct {
	SchemaVersion string     `json:"schema_version"`
	Config        Config     `json:"config"`
	Parameters    Parameters `json:"parameters"`
	Options       Options    `json:"options"`
	Optimizer     AdamState  `json:"optimizer"`
	Updates       uint64     `json:"updates"`
}

// StepResult records the pre-update loss and parameter update size.
type StepResult struct {
	Loss         float64 `json:"loss"`
	GradientNorm float64 `json:"gradient_norm"`
	UpdateNorm   float64 `json:"update_norm"`
	Updates      uint64  `json:"updates"`
}

// Trainer serializes updates. Context-aware operations can stop while waiting
// for the serialized state. Snapshots and parameters do not expose its state.
type Trainer struct {
	mu         cancellableMutex
	network    *Network
	parameters Parameters
	options    Options
	optimizer  AdamState
	updates    uint64
}

// NewTrainer validates the entire model before making an owned parameter copy.
func NewTrainer(c Config, p Parameters, o Options) (*Trainer, error) {
	if err := validateOptions(o); err != nil {
		return nil, err
	}
	n, err := NewNetwork(c)
	if err != nil {
		return nil, err
	}
	if err := validateTrainable(o.Trainable, n.core.theta()); err != nil {
		return nil, err
	}
	// Validate supplied arrays before allocating buffers from untrusted declared
	// dimensions (for example, a compact but malformed checkpoint).
	encoderSize, sizeErr := size(c.InputSize, inputWidth(n.config))
	if sizeErr != nil {
		return nil, sizeErr
	}
	nodes, theta := n.core.nodes(), 0
	if n.core.theta() {
		theta = nodes
	}
	if len(p.Core.Weights) != n.core.edges() || len(p.Core.Bias) != nodes || len(p.Core.LogTau) != nodes || len(p.ThetaRaw) != theta || len(p.Encoder) != encoderSize || len(p.Readout) != len(c.ReadoutNodes)*c.OutputSize {
		return nil, fmt.Errorf("parameter shape mismatch")
	}
	if _, err = n.Predict(context.Background(), p, [][]float64{make([]float64, c.InputSize)}); err != nil {
		return nil, fmt.Errorf("invalid model: %w", err)
	}
	count := len(flatParameters(p))
	return &Trainer{network: n, parameters: copyParameters(p), options: o, optimizer: AdamState{make([]float64, count), make([]float64, count), make([]uint64, count)}}, nil
}

// RestoreTrainer rejects unsupported snapshots and invalid optimizer state.
func RestoreTrainer(s TrainingSnapshot) (*Trainer, error) {
	if s.SchemaVersion != "coimnet-episode-training/v1" {
		return nil, fmt.Errorf("unsupported training snapshot %q", s.SchemaVersion)
	}
	tr, err := NewTrainer(s.Config, s.Parameters, s.Options)
	if err != nil {
		return nil, err
	}
	count := len(tr.optimizer.First)
	if len(s.Optimizer.First) != count || len(s.Optimizer.Second) != count || len(s.Optimizer.Steps) != count {
		return nil, fmt.Errorf("optimizer shape mismatch")
	}
	for i, v := range s.Optimizer.First {
		if !finite(v) || !finite(s.Optimizer.Second[i]) || s.Optimizer.Second[i] < 0 || s.Optimizer.Steps[i] > s.Updates {
			return nil, fmt.Errorf("invalid optimizer state at parameter %d", i)
		}
		if s.Optimizer.Steps[i] == 0 && (v != 0 || s.Optimizer.Second[i] != 0) {
			return nil, fmt.Errorf("nonzero moments without updates at parameter %d", i)
		}
	}
	tr.optimizer = copyAdam(s.Optimizer)
	tr.updates = s.Updates
	return tr, nil
}

// Snapshot returns a fully independent snapshot at an update boundary.
// A nil or zero-value Trainer returns TrainingSnapshot{}.
func (tr *Trainer) Snapshot() TrainingSnapshot {
	if tr == nil || tr.network == nil {
		return TrainingSnapshot{}
	}
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return TrainingSnapshot{"coimnet-episode-training/v1", tr.network.Config(), copyParameters(tr.parameters), tr.options, copyAdam(tr.optimizer), tr.updates}
}

// Predict runs a frozen, independent episode without changing trainer state.
func (tr *Trainer) Predict(ctx context.Context, input [][]float64) ([]float64, error) {
	if tr == nil || tr.network == nil {
		return nil, fmt.Errorf("nil trainer")
	}
	if err := tr.mu.LockContext(ctx); err != nil {
		return nil, err
	}
	defer tr.mu.Unlock()
	return tr.network.Predict(ctx, tr.parameters, input)
}

// Spikes runs a frozen, independent episode and returns the 0/1 event of every
// neuron after each step. Only a LIF core produces events; a continuous network
// returns an error. Trainer state is unchanged.
func (tr *Trainer) Spikes(ctx context.Context, input [][]float64) ([][]float64, error) {
	if tr == nil || tr.network == nil {
		return nil, fmt.Errorf("nil trainer")
	}
	if err := tr.mu.LockContext(ctx); err != nil {
		return nil, err
	}
	defer tr.mu.Unlock()
	return tr.network.spikeEvents(ctx, tr.parameters, input)
}

// Step computes and validates a complete update before committing parameters,
// optimizer state and the transaction count together. It never changes inputs.
func (tr *Trainer) Step(ctx context.Context, input [][]float64, target []float64) (StepResult, error) {
	var zero StepResult
	if tr == nil || ctx == nil {
		return zero, fmt.Errorf("nil trainer or context")
	}
	if tr.network == nil {
		return zero, fmt.Errorf("nil trainer")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if err := tr.mu.LockContext(ctx); err != nil {
		return zero, err
	}
	defer tr.mu.Unlock()
	if tr.updates == math.MaxUint64 {
		return zero, fmt.Errorf("update counter overflow")
	}
	loss, g, err := tr.network.LossGradient(ctx, tr.parameters, input, target, tr.options.Truncation)
	if err != nil {
		return zero, err
	}
	p := flatParameters(tr.parameters)
	grad := flatGradient(g)
	mask := parameterMask(tr.parameters, tr.options.Trainable)
	state := copyAdam(tr.optimizer)
	var norm float64
	for i, v := range grad {
		if mask[i] {
			norm = math.Hypot(norm, v)
		}
	}
	if !finite(norm) {
		return zero, fmt.Errorf("non-finite gradient norm")
	}
	scale := 1.0
	if tr.options.ClipNorm > 0 && norm > tr.options.ClipNorm {
		scale = tr.options.ClipNorm / norm
	}
	var updateNorm float64
	for i, v := range p {
		if !mask[i] {
			continue
		}
		if state.Steps[i] == math.MaxUint64 {
			return zero, fmt.Errorf("optimizer counter overflow")
		}
		state.Steps[i]++
		d := grad[i] * scale
		state.First[i] = tr.options.Beta1*state.First[i] + (1-tr.options.Beta1)*d
		state.Second[i] = tr.options.Beta2*state.Second[i] + (1-tr.options.Beta2)*d*d
		m := state.First[i] / (1 - math.Pow(tr.options.Beta1, float64(state.Steps[i])))
		vhat := state.Second[i] / (1 - math.Pow(tr.options.Beta2, float64(state.Steps[i])))
		p[i] = v*(1-tr.options.LearningRate*tr.options.WeightDecay) - tr.options.LearningRate*m/(math.Sqrt(vhat)+tr.options.Epsilon)
		if !finite(p[i]) || !finite(state.First[i]) || !finite(state.Second[i]) {
			return zero, fmt.Errorf("non-finite optimizer result at parameter %d", i)
		}
		updateNorm = math.Hypot(updateNorm, p[i]-v)
	}
	candidate := unflatten(p, tr.parameters)
	// Includes positive representable tau, tensor casts and finite forward state.
	if _, err := tr.network.Predict(ctx, candidate, input); err != nil {
		return zero, fmt.Errorf("candidate update rejected: %w", err)
	}
	if !finite(updateNorm) {
		return zero, fmt.Errorf("non-finite update norm")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	tr.parameters = candidate
	tr.optimizer = state
	tr.updates++
	return StepResult{loss, norm, updateNorm, tr.updates}, nil
}

type cancellableMutex struct {
	once  sync.Once
	token chan struct{}
}

func (m *cancellableMutex) init() {
	m.once.Do(func() {
		m.token = make(chan struct{}, 1)
		m.token <- struct{}{}
	})
}

func (m *cancellableMutex) Lock() {
	m.init()
	<-m.token
}

func (m *cancellableMutex) LockContext(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.init()
	select {
	case <-ctx.Done():
		if err := ctx.Err(); err != nil {
			return err
		}
		return context.Canceled
	case <-m.token:
		if err := ctx.Err(); err != nil {
			m.Unlock()
			return err
		}
		return nil
	}
}

func (m *cancellableMutex) Unlock() {
	m.init()
	m.token <- struct{}{}
}

func validateOptions(o Options) error {
	for _, x := range []float64{o.LearningRate, o.Beta1, o.Beta2, o.Epsilon, o.WeightDecay, o.ClipNorm} {
		if !finite(x) {
			return fmt.Errorf("non-finite optimizer option")
		}
	}
	if o.LearningRate <= 0 || o.Beta1 < 0 || o.Beta1 >= 1 || o.Beta2 < 0 || o.Beta2 >= 1 || o.Epsilon <= 0 || o.WeightDecay < 0 || o.ClipNorm < 0 || o.Truncation < 0 {
		return fmt.Errorf("invalid optimizer option range")
	}
	return nil
}

// validateTrainable rejects a threshold group the configured core does not own,
// so a continuous model can never silently ignore Trainable.Theta.
func validateTrainable(m Trainable, theta bool) error {
	if m.Theta && !theta {
		return fmt.Errorf("trainable theta requires a LIF core")
	}
	return nil
}
func copyParameters(p Parameters) Parameters {
	return Parameters{Core: dynamics.Parameters{Weights: append([]float64(nil), p.Core.Weights...), Bias: append([]float64(nil), p.Core.Bias...), LogTau: append([]float64(nil), p.Core.LogTau...)}, ThetaRaw: append([]float64(nil), p.ThetaRaw...), Encoder: append([]float64(nil), p.Encoder...), Readout: append([]float64(nil), p.Readout...)}
}
func copyAdam(s AdamState) AdamState {
	return AdamState{append([]float64(nil), s.First...), append([]float64(nil), s.Second...), append([]uint64(nil), s.Steps...)}
}
func flatParameters(p Parameters) []float64 {
	out := []float64{}
	for _, v := range [][]float64{p.Core.Weights, p.Core.Bias, p.Core.LogTau, p.ThetaRaw, p.Encoder, p.Readout} {
		out = append(out, v...)
	}
	return out
}
func flatGradient(g Gradient) []float64 {
	out := []float64{}
	for _, v := range [][]float64{g.Core.Weights, g.Core.Bias, g.Core.LogTau, g.ThetaRaw, g.Encoder, g.Readout} {
		out = append(out, v...)
	}
	return out
}
func unflatten(v []float64, shape Parameters) Parameters {
	p := copyParameters(shape)
	offset := 0
	for _, dst := range [][]float64{p.Core.Weights, p.Core.Bias, p.Core.LogTau, p.ThetaRaw, p.Encoder, p.Readout} {
		copy(dst, v[offset:offset+len(dst)])
		offset += len(dst)
	}
	return p
}
func parameterMask(p Parameters, m Trainable) []bool {
	var out []bool
	for _, g := range []struct {
		size    int
		enabled bool
	}{{len(p.Core.Weights), m.Weights}, {len(p.Core.Bias), m.Bias}, {len(p.Core.LogTau), m.Tau}, {len(p.ThetaRaw), m.Theta}, {len(p.Encoder), m.Encoder}, {len(p.Readout), m.Readout}} {
		for range g.size {
			out = append(out, g.enabled)
		}
	}
	return out
}
