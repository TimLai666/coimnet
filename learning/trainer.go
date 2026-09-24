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
	// Masks narrows a trainable group to individual edges and nodes. The
	// effective mask is the group flag AND the per-item mask; nil means every
	// item of every group follows its group flag alone.
	Masks *UpdateMasks `json:"masks,omitempty"`
	// Ranges bounds parameter values by projection after each update. nil, and
	// every zero field inside it, mean unlimited.
	Ranges *ParameterRanges `json:"ranges,omitempty"`
	// LossScale multiplies the loss gradient before it is divided back, so a
	// gradient that would underflow survives the round trip. A scaled value
	// that is not finite rejects the whole step. Zero means one.
	LossScale float64 `json:"loss_scale,omitempty"`
	// AccumulateSteps is how many Step gradients one update averages over.
	// Zero and one apply every step. A larger value makes Step accumulate into
	// Accumulator and update only when the window is full.
	AccumulateSteps int `json:"accumulate_steps,omitempty"`
	// Schedule shapes the learning rate as a function of the update count.
	// nil holds LearningRate constant.
	Schedule *Schedule `json:"schedule,omitempty"`
	// Recompute makes Step and StepFrom keep only segment checkpoints of the
	// core's forward history and recompute each segment in the reverse pass;
	// see Recompute. nil keeps the complete reverse history. The field is
	// omitted when nil, so every existing configuration and snapshot keeps its
	// exact JSON and its recorded hash.
	Recompute *Recompute `json:"recompute,omitempty"`
}

// DefaultOptions returns every group of the continuous core trainable and a
// declared AdamW baseline. The LIF threshold group stays frozen; a spiking
// model must enable it explicitly.
func DefaultOptions() Options {
	return Options{
		LearningRate: .01, Beta1: .9, Beta2: .999, Epsilon: 1e-8, WeightDecay: 0, ClipNorm: 1, Truncation: 0,
		Trainable: Trainable{Encoder: true, Weights: true, Bias: true, Tau: true, Readout: true},
	}
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
	// Accumulator is the open gradient-accumulation window, present only when
	// Options.AccumulateSteps is above one and the window is partly filled.
	Accumulator *GradientAccumulator `json:"accumulator,omitempty"`
}

// StepResult records the pre-update loss, this step's own gradient norm, the
// parameter update size, how many parameters each projection moved and the
// learning rate the update used. Projected is nil when no projection changed
// anything; its keys are the parameter groups "weights", "bias", "log_tau" and
// "theta_raw" plus "min_log_magnitude" for the fixed-sign underflow floor.
// GradientNorm is always the norm of the gradient this step computed, before
// clipping and before any accumulation average.
type StepResult struct {
	Loss float64 `json:"loss"`
	// LossKnown reports whether Loss is a computed value. Step sets it true;
	// StepFrom leaves it false and Loss at zero, because the caller-supplied
	// upstream gradient never reveals the objective value.
	LossKnown    bool           `json:"loss_known"`
	GradientNorm float64        `json:"gradient_norm"`
	UpdateNorm   float64        `json:"update_norm"`
	Updates      uint64         `json:"updates"`
	Projected    map[string]int `json:"projected,omitempty"`
	LearningRate float64        `json:"learning_rate"`
	// Applied reports whether this step ran the optimizer. A step that only
	// filled part of an accumulation window reports false, leaves Updates,
	// UpdateNorm, Projected and LearningRate at their no-update values, and
	// changes nothing but the accumulator.
	Applied bool `json:"applied"`
	// Accumulated is how many gradients the accumulator holds after this step,
	// which is zero on an applied step.
	Accumulated int `json:"accumulated"`
	// GradientHorizonSteps is the furthest number of steps back the gradient
	// of any row's loss reached in this step: Options.Truncation when it is
	// positive and below the number of input rows, the number of input rows
	// otherwise. Earlier rows still shaped the prediction and the neural state
	// but received no gradient, so the loss cannot be claimed to reach back
	// indefinitely.
	GradientHorizonSteps int `json:"gradient_horizon_steps,omitempty"`
}

// Trainer serializes updates. Context-aware operations can stop while waiting
// for the serialized state. Snapshots and parameters do not expose its state.
type Trainer struct {
	mu          cancellableMutex
	network     *Network
	parameters  Parameters
	options     Options
	optimizer   AdamState
	updates     uint64
	accumulator *GradientAccumulator
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
	// Root decision 6 of ticket 25 defines a fixed sign per scalar edge only;
	// a C*C matrix weight has no declared sign, so a nonzero declaration on a
	// vector-state model is refused rather than silently ignored. An all-zero
	// declaration is the same model as no declaration at all and stays legal.
	if n.core.stateDim() > 1 && hasFixedSigns(c.EdgeSigns) {
		return nil, fmt.Errorf("fixed edge signs are not defined for vector state edges")
	}
	if err := validateTrainable(o.Trainable, n.core.theta()); err != nil {
		return nil, err
	}
	if o.Recompute != nil {
		if err := recomputeSupported(n.core); err != nil {
			return nil, err
		}
	}
	if err := validateMasks(o.Masks, n.core.nodes(), n.core.edges()); err != nil {
		return nil, err
	}
	if err := validateRangesAgainstSigns(n.config, o.Ranges); err != nil {
		return nil, err
	}
	// Validate supplied arrays before allocating buffers from untrusted declared
	// dimensions (for example, a compact but malformed checkpoint).
	encoderSize, sizeErr := size(c.InputSize, inputWidth(n.config))
	if sizeErr != nil {
		return nil, sizeErr
	}
	nodeCount, theta := n.core.nodes(), n.core.thetaCount()
	stateDim := n.core.stateDim()
	if len(p.Core.Weights) != n.core.weightCount() || len(p.Core.Bias) != n.core.biasCount() || len(p.Core.LogTau) != nodeCount || len(p.ThetaRaw) != theta || len(p.Encoder) != encoderSize || len(p.Readout) != len(c.ReadoutNodes)*stateDim*c.OutputSize {
		return nil, fmt.Errorf("parameter shape mismatch")
	}
	if c.Sharing != nil {
		if err := validateSharingParameters(c.Sharing, p, c, n.core); err != nil {
			return nil, err
		}
	}
	if _, err = n.Predict(context.Background(), p, [][]float64{make([]float64, c.InputSize)}); err != nil {
		return nil, fmt.Errorf("invalid model: %w", err)
	}
	count := len(flatParameters(p))
	return &Trainer{network: n, parameters: copyParameters(p), options: copyOptions(o), optimizer: AdamState{make([]float64, count), make([]float64, count), make([]uint64, count)}}, nil
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
	if err := validateAccumulator(s.Accumulator, count, s.Options); err != nil {
		return nil, err
	}
	tr.optimizer = copyAdam(s.Optimizer)
	tr.updates = s.Updates
	tr.accumulator = copyAccumulator(s.Accumulator)
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
	return TrainingSnapshot{"coimnet-episode-training/v1", tr.network.Config(), copyParameters(tr.parameters), copyOptions(tr.options), copyAdam(tr.optimizer), tr.updates, copyAccumulator(tr.accumulator)}
}

// Close releases resources owned by the trainer. It is a no-op for CPU
// trainers and safe to call more than once. GPU execution after Close returns
// the adapter's closed error.
func (tr *Trainer) Close() error {
	if tr == nil || tr.network == nil {
		return nil
	}
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if core, ok := tr.network.core.(gpuContinuousCore); ok && core.gpu != nil {
		return core.gpu.Close()
	}
	return nil
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

// PredictAll runs a frozen, independent episode and returns the readout of
// every step. It requires Config.ReadoutEveryStep; a last-step model is
// refused. Trainer state is unchanged.
func (tr *Trainer) PredictAll(ctx context.Context, input [][]float64) ([][]float64, error) {
	if tr == nil || tr.network == nil {
		return nil, fmt.Errorf("nil trainer")
	}
	if err := tr.mu.LockContext(ctx); err != nil {
		return nil, err
	}
	defer tr.mu.Unlock()
	return tr.network.PredictAll(ctx, tr.parameters, input)
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
//
// The order inside one step is fixed: the loss gradient is multiplied by
// Options.LossScale and divided back, the result is added to the accumulator,
// and only a full accumulation window is averaged, clipped against
// Options.ClipNorm and handed to AdamW at the scheduled learning rate. A step
// that only fills part of the window changes nothing but the accumulator and
// reports Applied false.
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
	loss, g, err := tr.network.lossGradient(ctx, tr.parameters, input, target, tr.options.Truncation, recomputeSegment(tr.options))
	if err != nil {
		return zero, err
	}
	result, err := tr.stepWithGradient(ctx, input, loss, true, g)
	if err != nil {
		return zero, err
	}
	result.GradientHorizonSteps = gradientHorizon(len(input), tr.options.Truncation)
	return result, nil
}

// StepFrom is Step with a caller-supplied upstream gradient: the same mask,
// loss-scale, accumulation, clipping, schedule, AdamW and projection path,
// with StepResult.Loss reported as NaN-free zero and documented as
// "not computed" (Loss = 0, LossKnown = false).
func (tr *Trainer) StepFrom(ctx context.Context, input, upstream [][]float64) (StepResult, error) {
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
	g, err := tr.network.lossGradientFrom(ctx, tr.parameters, input, upstream, tr.options.Truncation, recomputeSegment(tr.options))
	if err != nil {
		return zero, err
	}
	result, err := tr.stepWithGradient(ctx, input, 0, false, g)
	if err != nil {
		return zero, err
	}
	result.GradientHorizonSteps = gradientHorizon(len(input), tr.options.Truncation)
	return result, nil
}

// recomputeSegment resolves Options.Recompute to the segment length the
// network reverse pass takes: zero, the full history, when it is nil.
func recomputeSegment(o Options) int {
	if o.Recompute == nil {
		return 0
	}
	return o.Recompute.SegmentSteps
}

// gradientHorizon is StepResult.GradientHorizonSteps for an episode of rows
// input rows under the truncation window.
func gradientHorizon(rows, truncation int) int {
	if truncation > 0 && truncation < rows {
		return truncation
	}
	return rows
}

// stepWithGradient is the mask, loss-scale, accumulation, clipping, scheduled
// AdamW and projection path shared by Step and StepFrom. Step reports the loss
// it computed with LossKnown true; StepFrom reports zero with LossKnown false.
func (tr *Trainer) stepWithGradient(ctx context.Context, input [][]float64, loss float64, known bool, g Gradient) (StepResult, error) {
	var zero StepResult
	var err error
	p := flatParameters(tr.parameters)
	grad := flatGradient(g)
	mask := parameterMask(tr.parameters, tr.options, tr.network.core.thetaNodes(), tr.network.core)
	// A sharing declaration reduces each group's gradient to its members' sum
	// before loss scaling, accumulation, the norm and AdamW (root decision 7 of
	// ticket 25; main spec 9.2 sums, it does not average), and freezes a whole
	// group whenever its per-item mask froze any one member.
	var plan *sharePlan
	if tr.network.config.Sharing != nil {
		plan = newSharePlan(tr.network.config.Sharing, len(tr.parameters.Core.Weights), len(tr.parameters.Core.Bias), len(tr.parameters.Core.LogTau))
		reduceSharedGradients(grad, plan)
		mask, _ = sharedMaskFreeze(mask, plan)
	}
	state := copyAdam(tr.optimizer)
	// Loss scaling multiplies and divides back here, before the gradient
	// reaches the accumulator or the clip: a product that leaves the
	// representable range rejects the whole step rather than saturating it.
	if factor := lossScale(tr.options); factor != 1 {
		for i, v := range grad {
			if !mask[i] {
				continue
			}
			scaled := v * factor
			if !finite(scaled) {
				return zero, fmt.Errorf("gradient at parameter %d is not finite under loss scale %g", i, factor)
			}
			grad[i] = scaled / factor
		}
	}
	var norm float64
	if plan == nil {
		for i, v := range grad {
			if mask[i] {
				norm = math.Hypot(norm, v)
			}
		}
	} else {
		// The reduced vector counts each group once, so shared members cannot
		// inflate the norm by repeating themselves.
		norm = reducedNorm(grad, mask, plan)
	}
	if !finite(norm) {
		return zero, fmt.Errorf("non-finite gradient norm")
	}
	// Accumulation. A window of one uses this step's gradient directly, which
	// allocates nothing extra and stays bit-identical to the behaviour before
	// this ticket. A larger window sums first and only a full window continues.
	window, mean := accumulateSteps(tr.options), grad
	var accumulator *GradientAccumulator
	if window > 1 {
		accumulator = copyAccumulator(tr.accumulator)
		// An accumulator that no longer fits the declared window starts over.
		// RestoreTrainer refuses such a snapshot, so this can only come from an
		// options change on a live trainer.
		if accumulator != nil && (len(accumulator.Sum) != len(grad) || accumulator.Count >= window) {
			accumulator = nil
		}
		if accumulator == nil {
			accumulator = &GradientAccumulator{Sum: make([]float64, len(grad))}
		}
		for i, v := range grad {
			if mask[i] {
				accumulator.Sum[i] += v
			}
		}
		accumulator.Count++
		if accumulator.Count < window {
			if err := ctx.Err(); err != nil {
				return zero, err
			}
			tr.accumulator = accumulator
			return StepResult{Loss: loss, LossKnown: known, GradientNorm: norm, Updates: tr.updates, Accumulated: accumulator.Count}, nil
		}
		mean = make([]float64, len(grad))
		for i := range mean {
			if mask[i] {
				mean[i] = accumulator.Sum[i] / float64(accumulator.Count)
			}
		}
	}
	// Clipping consumes the window average, before AdamW consumes the clip. A
	// window of one averages nothing, so its norm is the one already measured.
	clipNorm := norm
	if window > 1 {
		clipNorm = 0
		if plan == nil {
			for i, v := range mean {
				if mask[i] {
					clipNorm = math.Hypot(clipNorm, v)
				}
			}
		} else {
			clipNorm = reducedNorm(mean, mask, plan)
		}
		if !finite(clipNorm) {
			return zero, fmt.Errorf("non-finite accumulated gradient norm")
		}
	}
	scale := 1.0
	if tr.options.ClipNorm > 0 && clipNorm > tr.options.ClipNorm {
		scale = tr.options.ClipNorm / clipNorm
	}
	rate := LearningRateAt(tr.options, tr.updates)
	if !finite(rate) || rate < 0 {
		return zero, fmt.Errorf("scheduled learning rate %g at update %d is not usable", rate, tr.updates)
	}
	// Only a constrained model pays for the pre-update copy the projection
	// needs to report an update norm that includes the projection.
	project := constrained(tr.network.fixed, tr.options.Ranges)
	var before []float64
	if project {
		before = append([]float64(nil), p...)
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
		d := mean[i] * scale
		state.First[i] = tr.options.Beta1*state.First[i] + (1-tr.options.Beta1)*d
		state.Second[i] = tr.options.Beta2*state.Second[i] + (1-tr.options.Beta2)*d*d
		m := state.First[i] / (1 - math.Pow(tr.options.Beta1, float64(state.Steps[i])))
		vhat := state.Second[i] / (1 - math.Pow(tr.options.Beta2, float64(state.Steps[i])))
		p[i] = v*(1-rate*tr.options.WeightDecay) - rate*m/(math.Sqrt(vhat)+tr.options.Epsilon)
		if !finite(p[i]) || !finite(state.First[i]) || !finite(state.Second[i]) {
			return zero, fmt.Errorf("non-finite optimizer result at parameter %d", i)
		}
		if !project {
			updateNorm = math.Hypot(updateNorm, p[i]-v)
		}
	}
	var projected map[string]int
	if project {
		if projected, err = doProject(tr.network.config, tr.options.Ranges, p, mask, tr.parameters); err != nil {
			return zero, err
		}
		for i := range p {
			if !mask[i] {
				continue
			}
			updateNorm = math.Hypot(updateNorm, p[i]-before[i])
		}
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
	tr.accumulator = nil
	return StepResult{Loss: loss, LossKnown: known, GradientNorm: norm, UpdateNorm: updateNorm, Updates: tr.updates, Projected: projected, LearningRate: rate, Applied: true}, nil
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
	for _, x := range []float64{o.LearningRate, o.Beta1, o.Beta2, o.Epsilon, o.WeightDecay, o.ClipNorm, o.LossScale} {
		if !finite(x) {
			return fmt.Errorf("non-finite optimizer option")
		}
	}
	if o.LearningRate <= 0 || o.Beta1 < 0 || o.Beta1 >= 1 || o.Beta2 < 0 || o.Beta2 >= 1 || o.Epsilon <= 0 || o.WeightDecay < 0 || o.ClipNorm < 0 || o.Truncation < 0 {
		return fmt.Errorf("invalid optimizer option range")
	}
	if o.LossScale < 0 {
		return fmt.Errorf("loss scale %g cannot be negative", o.LossScale)
	}
	if o.AccumulateSteps < 0 {
		return fmt.Errorf("accumulate steps %d cannot be negative", o.AccumulateSteps)
	}
	if o.Recompute != nil && o.Recompute.SegmentSteps < 1 {
		return fmt.Errorf("recompute segment_steps %d must be at least 1", o.Recompute.SegmentSteps)
	}
	if err := validateSchedule(o.Schedule); err != nil {
		return err
	}
	return validateRanges(o.Ranges)
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

// parameterMask is the effective mask: the group flag of Options.Trainable AND
// the per-item entry of Options.Masks. The edge half applies to the weight
// group and the node half to bias, log_tau and theta_raw; the encoder and the
// readout have no per-item mask and follow their group flag alone.
//
// thetaNodes maps each theta_raw entry to its node, which the node half of the
// mask is indexed by. It is nil where that map is the identity, which is every
// core whose threshold group covers all of its nodes; a mixed core's group
// covers only its LIF nodes and needs the map, or the node mask would land on
// the wrong thresholds.
//
// The optional core supplies the vector layout of the weight and bias groups.
// On a matrix-edge vector core the i-th weight value belongs to edge
// i/(C*C) and the i-th bias value to node i/C, so the per-item halves are
// indexed through those spans; a scalar-broadcast vector core stores one
// weight per edge and maps weights by identity. Without a core every span is
// one and a scalar model's mask is bit-identical to the mapping before the
// vector core existed.
func parameterMask(p Parameters, o Options, thetaNodes []int, core ...coreModel) []bool {
	var edges, nodes []bool
	if o.Masks != nil {
		edges, nodes = o.Masks.Edges, o.Masks.Nodes
	}
	weightSpan, biasSpan := 1, 1
	if len(core) != 0 && core[0] != nil {
		dim := core[0].stateDim()
		biasSpan = dim
		if core[0].matrixEdges() {
			weightSpan = dim * dim
		}
	}
	theta := nodes
	if len(thetaNodes) != 0 && len(nodes) != 0 {
		theta = make([]bool, len(thetaNodes))
		for k, node := range thetaNodes {
			theta[k] = node < len(nodes) && nodes[node]
		}
	}
	m := o.Trainable
	var out []bool
	for _, g := range []struct {
		size    int
		enabled bool
		item    []bool
		span    int
	}{
		{len(p.Core.Weights), m.Weights, edges, weightSpan}, {len(p.Core.Bias), m.Bias, nodes, biasSpan},
		{len(p.Core.LogTau), m.Tau, nodes, 1}, {len(p.ThetaRaw), m.Theta, theta, 1},
		{len(p.Encoder), m.Encoder, nil, 1}, {len(p.Readout), m.Readout, nil, 1},
	} {
		for i := range g.size {
			enabled := g.enabled
			if enabled && i < len(g.item)*g.span {
				enabled = g.item[i/g.span]
			}
			out = append(out, enabled)
		}
	}
	return out
}
