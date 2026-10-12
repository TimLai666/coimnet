package learning

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/TimLai666/coimnet/backend/webgpu"
)

const deviceAdamSnapshotSchema = "coimnet-device-adamw/v1"

// NewGPUDeviceTrainer creates the opt-in float32 hardware AdamW episode mode.
// Initial parameters and kernel options are explicitly rounded to float32;
// unsupported constraints are rejected before opening the device.
func NewGPUDeviceTrainer(ctx context.Context, c Config, p Parameters, o Options, maxOptimizerBytes uint64) (*Trainer, error) {
	if err := validateGPUDeviceTrainerSupport(c, o); err != nil {
		return nil, err
	}
	quantizedOptions, kernel, err := quantizeGPUOptions(o, maxOptimizerBytes)
	if err != nil {
		return nil, err
	}
	quantizedParameters, err := quantizeGPUParameters(p)
	if err != nil {
		return nil, err
	}
	trainer, err := NewTrainer(c, quantizedParameters, quantizedOptions)
	if err != nil {
		return nil, err
	}
	if maxOptimizerBytes == 0 {
		return nil, fmt.Errorf("%w: optimizer budget must be positive", webgpu.ErrCapacity)
	}

	// Open the optimizer first. Its budget preflight runs before any sparse GPU
	// core is attached, so a capacity error cannot leave a half-open trainer.
	deviceAdam, err := webgpu.NewDeviceAdamW(ctx, float32Parameters(flatParameters(quantizedParameters)), kernel)
	if err != nil {
		return nil, err
	}
	trainer, err = attachGPUTrainer(ctx, trainer)
	if err != nil {
		return nil, errors.Join(err, deviceAdam.Close())
	}
	trainer.deviceAdam = deviceAdam
	return trainer, nil
}

// RestoreGPUDeviceTrainer restores an exactly representable device snapshot.
// It never silently quantizes trained parameters, moments, or counters.
func RestoreGPUDeviceTrainer(ctx context.Context, s TrainingSnapshot, maxOptimizerBytes uint64) (*Trainer, error) {
	if err := validateGPUDeviceTrainerSupport(s.Config, s.Options); err != nil {
		return nil, err
	}
	// Classify device representability before RestoreTrainer can collapse a
	// corrupt float64 value into its generic model/optimizer error. No device
	// resource is opened by these checks.
	if err := validateGPUExactState(s); err != nil {
		return nil, err
	}
	quantizedOptions, kernel, err := exactGPUOptions(s.Options, maxOptimizerBytes)
	if err != nil {
		return nil, err
	}
	// RestoreTrainer performs the complete model, options, optimizer and
	// accumulator validation without opening either GPU resource.
	trainer, err := RestoreTrainer(s)
	if err != nil {
		return nil, err
	}
	if err := validateGPUExactSnapshot(s, trainer); err != nil {
		return nil, err
	}
	if maxOptimizerBytes == 0 {
		return nil, fmt.Errorf("%w: optimizer budget must be positive", webgpu.ErrCapacity)
	}
	deviceSnapshot := webgpu.AdamWSnapshot{
		SchemaVersion: deviceAdamSnapshotSchema,
		Config:        kernel,
		Parameters:    float32Parameters(flatParameters(s.Parameters)),
		First:         float32Values(s.Optimizer.First),
		Second:        float32Values(s.Optimizer.Second),
		Steps:         make([]uint32, len(s.Optimizer.Steps)),
	}
	for i, step := range s.Optimizer.Steps {
		deviceSnapshot.Steps[i] = uint32(step)
	}

	// As with construction, restore the resident optimizer before attaching
	// the sparse core. Any failure after this point releases both resources.
	deviceAdam, err := webgpu.RestoreDeviceAdamW(ctx, deviceSnapshot)
	if err != nil {
		return nil, err
	}
	trainer, err = attachGPUTrainer(ctx, trainer)
	if err != nil {
		return nil, errors.Join(err, deviceAdam.Close())
	}
	// exactGPUOptions returns the same float32 values represented in s.Options.
	// Keep the original validated snapshot values on the host; they are exact,
	// and this avoids changing any unrelated JSON fields during restore.
	trainer.options = copyOptions(quantizedOptions)
	trainer.deviceAdam = deviceAdam
	return trainer, nil
}

// GPUAdamWMetrics reports device optimizer measurements; CPU and legacy GPU
// trainers return false. The optimizer budget covers its logical buffer sizes.
func (tr *Trainer) GPUAdamWMetrics() (webgpu.AdamWMetrics, bool) {
	if tr == nil || tr.network == nil {
		return webgpu.AdamWMetrics{}, false
	}
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if tr.deviceAdam == nil {
		return webgpu.AdamWMetrics{}, false
	}
	return tr.deviceAdam.Metrics(), true
}

// applyDeviceAdamUpdate receives the host-validated averaged gradient and mask.
// It validates the full GPU candidate before committing device and host state.
// The caller holds tr.mu; this helper must not lock the trainer recursively.
func (tr *Trainer) applyDeviceAdamUpdate(ctx context.Context, input [][]float64, loss float64, known bool, gradient []float64, mask []bool, gradientNorm, clipScale, rate float64) (StepResult, error) {
	var zero StepResult
	if tr == nil || tr.deviceAdam == nil {
		return zero, fmt.Errorf("GPU device AdamW is not attached")
	}
	if len(gradient) != len(mask) || len(gradient) != len(flatParameters(tr.parameters)) {
		return zero, fmt.Errorf("device Adam gradient shape mismatch")
	}
	if !finite(gradientNorm) || !finite(clipScale) || clipScale < 0 {
		return zero, fmt.Errorf("invalid device Adam gradient norm or clip scale")
	}
	if tr.updates >= uint64(math.MaxUint32) {
		return zero, fmt.Errorf("%w: device Adam updates counter %d exceeds uint32 continuation range", webgpu.ErrCapacity, tr.updates)
	}
	actualRate := float32(rate)
	if !finite(rate) || rate < 0 || !finite(float64(actualRate)) {
		return zero, fmt.Errorf("%w: scheduled learning rate %g is not representable as float32", webgpu.ErrInvalidConfig, rate)
	}
	if rate > 0 && actualRate == 0 {
		return zero, fmt.Errorf("%w: scheduled learning rate %g rounds to zero as float32", webgpu.ErrInvalidConfig, rate)
	}

	// The device backend validates every uploaded float32. Frozen values are
	// deliberately left at zero before conversion, so an irrelevant huge or
	// non-finite frozen gradient cannot reject a valid update.
	deviceGradient := make([]float32, len(gradient))
	for i, value := range gradient {
		if !mask[i] {
			continue
		}
		scaled := value * clipScale
		if !finite(scaled) {
			return zero, fmt.Errorf("gradient at parameter %d is not finite after clipping", i)
		}
		converted := float32(scaled)
		if !finite(float64(converted)) {
			return zero, fmt.Errorf("%w: gradient at parameter %d is not representable as float32", webgpu.ErrNonFinite, i)
		}
		deviceGradient[i] = converted
	}

	originalParameters := flatParameters(tr.parameters)
	originalOptimizer := copyAdam(tr.optimizer)
	var candidateParameters Parameters
	var candidateOptimizer AdamState
	var updateNorm float64
	var stepErr error
	_, stepErr = tr.deviceAdam.StepChecked(ctx, deviceGradient, mask, actualRate, func(candidate webgpu.AdamWSnapshot) error {
		var candidateFlat []float64
		var candidateErr error
		candidateParameters, candidateOptimizer, candidateFlat, candidateErr = validateDeviceAdamCandidate(candidate, tr.parameters, originalOptimizer, originalParameters, mask, tr.updates)
		if candidateErr != nil {
			return candidateErr
		}
		for i, value := range candidateFlat {
			if !mask[i] {
				continue
			}
			updateNorm = math.Hypot(updateNorm, value-originalParameters[i])
		}
		if !finite(updateNorm) {
			return fmt.Errorf("non-finite device Adam update norm")
		}
		// The candidate is checked on the same GPU-backed network that produced
		// this step's gradient. This catches invalid tau and all forward casts
		// before StepChecked swaps its resident bank.
		if _, err := tr.network.Predict(ctx, candidateParameters, input); err != nil {
			return fmt.Errorf("candidate update rejected: %w", err)
		}
		return nil
	})
	if stepErr != nil {
		return zero, stepErr
	}

	// StepChecked has now passed its final context check and committed the
	// device bank. The assignments below are non-failable host mirror writes.
	tr.parameters = candidateParameters
	tr.optimizer = candidateOptimizer
	tr.updates++
	tr.accumulator = nil
	return StepResult{
		Loss:         loss,
		LossKnown:    known,
		GradientNorm: gradientNorm,
		UpdateNorm:   updateNorm,
		Updates:      tr.updates,
		LearningRate: float64(actualRate),
		Applied:      true,
	}, nil
}

func validateGPUDeviceTrainerSupport(c Config, o Options) error {
	if err := validateGPUTrainerSupport(c, o); err != nil {
		return err
	}
	if o.Ranges != nil {
		return fmt.Errorf("%w: parameter ranges and projection", ErrGPUTrainerUnsupported)
	}
	if hasFixedSigns(c.EdgeSigns) {
		return fmt.Errorf("%w: fixed edge signs", ErrGPUTrainerUnsupported)
	}
	if c.Sharing != nil {
		return fmt.Errorf("%w: parameter sharing", ErrGPUTrainerUnsupported)
	}
	return nil
}

func quantizeGPUOptions(o Options, maxBytes uint64) (Options, webgpu.AdamWConfig, error) {
	q := o
	beta1, err := quantizeGPUKernelOption("beta1", o.Beta1, func(v float32) bool { return v >= 0 && v < 1 })
	if err != nil {
		return Options{}, webgpu.AdamWConfig{}, err
	}
	beta2, err := quantizeGPUKernelOption("beta2", o.Beta2, func(v float32) bool { return v >= 0 && v < 1 })
	if err != nil {
		return Options{}, webgpu.AdamWConfig{}, err
	}
	epsilon, err := quantizeGPUKernelOption("epsilon", o.Epsilon, func(v float32) bool { return v > 0 })
	if err != nil {
		return Options{}, webgpu.AdamWConfig{}, err
	}
	weightDecay, err := quantizeGPUKernelOption("weight_decay", o.WeightDecay, func(v float32) bool { return v >= 0 })
	if err != nil {
		return Options{}, webgpu.AdamWConfig{}, err
	}
	learningRate, err := quantizeGPUKernelOption("learning_rate", o.LearningRate, func(v float32) bool { return v > 0 })
	if err != nil {
		return Options{}, webgpu.AdamWConfig{}, err
	}
	q.Beta1, q.Beta2, q.Epsilon, q.WeightDecay = float64(beta1), float64(beta2), float64(epsilon), float64(weightDecay)
	if learningRate == 0 {
		return Options{}, webgpu.AdamWConfig{}, fmt.Errorf("%w: learning_rate rounds to zero as float32", webgpu.ErrInvalidConfig)
	}
	if err := validateOptions(q); err != nil {
		return Options{}, webgpu.AdamWConfig{}, err
	}
	return q, webgpu.AdamWConfig{Beta1: beta1, Beta2: beta2, Epsilon: epsilon, WeightDecay: weightDecay, MaxBytes: maxBytes}, nil
}

func quantizeGPUKernelOption(field string, value float64, valid func(float32) bool) (float32, error) {
	if !finite(value) {
		return 0, fmt.Errorf("%w: %s is non-finite", webgpu.ErrInvalidConfig, field)
	}
	if value < 0 {
		return 0, fmt.Errorf("%w: %s cannot be negative", webgpu.ErrInvalidConfig, field)
	}
	converted := float32(value)
	if !finite(float64(converted)) {
		return 0, fmt.Errorf("%w: %s is not representable as finite float32", webgpu.ErrInvalidConfig, field)
	}
	if converted == 0 && value > 0 && (field == "epsilon" || field == "learning_rate") {
		return 0, fmt.Errorf("%w: %s rounds to zero as float32", webgpu.ErrInvalidConfig, field)
	}
	if !valid(converted) {
		if field == "epsilon" && converted == 0 {
			return 0, fmt.Errorf("%w: epsilon is zero after float32 conversion", webgpu.ErrInvalidConfig)
		}
		return 0, fmt.Errorf("%w: %s is invalid after float32 conversion (%v)", webgpu.ErrInvalidConfig, field, converted)
	}
	return converted, nil
}

func quantizeGPUParameters(p Parameters) (Parameters, error) {
	var err error
	q := Parameters{}
	if q.Core.Weights, err = quantizeGPUParameterSlice("core.weights", p.Core.Weights); err != nil {
		return Parameters{}, err
	}
	if q.Core.Bias, err = quantizeGPUParameterSlice("core.bias", p.Core.Bias); err != nil {
		return Parameters{}, err
	}
	if q.Core.LogTau, err = quantizeGPUParameterSlice("core.log_tau", p.Core.LogTau); err != nil {
		return Parameters{}, err
	}
	if q.ThetaRaw, err = quantizeGPUParameterSlice("theta_raw", p.ThetaRaw); err != nil {
		return Parameters{}, err
	}
	if q.Encoder, err = quantizeGPUParameterSlice("encoder", p.Encoder); err != nil {
		return Parameters{}, err
	}
	if q.Readout, err = quantizeGPUParameterSlice("readout", p.Readout); err != nil {
		return Parameters{}, err
	}
	return q, nil
}

func quantizeGPUParameterSlice(field string, values []float64) ([]float64, error) {
	if values == nil {
		return nil, nil
	}
	converted := make([]float64, len(values))
	for i, value := range values {
		if !finite(value) {
			return nil, fmt.Errorf("%w: parameter %s[%d] is non-finite", webgpu.ErrInvalidConfig, field, i)
		}
		f32 := float32(value)
		if !finite(float64(f32)) {
			return nil, fmt.Errorf("%w: parameter %s[%d] is not representable as finite float32", webgpu.ErrInvalidConfig, field, i)
		}
		converted[i] = float64(f32)
	}
	return converted, nil
}

func exactGPUOptions(o Options, maxBytes uint64) (Options, webgpu.AdamWConfig, error) {
	q := o
	beta1, err := exactGPUKernelOption("beta1", o.Beta1, func(v float32) bool { return v >= 0 && v < 1 })
	if err != nil {
		return Options{}, webgpu.AdamWConfig{}, err
	}
	beta2, err := exactGPUKernelOption("beta2", o.Beta2, func(v float32) bool { return v >= 0 && v < 1 })
	if err != nil {
		return Options{}, webgpu.AdamWConfig{}, err
	}
	epsilon, err := exactGPUKernelOption("epsilon", o.Epsilon, func(v float32) bool { return v > 0 })
	if err != nil {
		return Options{}, webgpu.AdamWConfig{}, err
	}
	weightDecay, err := exactGPUKernelOption("weight_decay", o.WeightDecay, func(v float32) bool { return v >= 0 })
	if err != nil {
		return Options{}, webgpu.AdamWConfig{}, err
	}
	if _, err := quantizeGPUKernelOption("learning_rate", o.LearningRate, func(v float32) bool { return v > 0 }); err != nil {
		return Options{}, webgpu.AdamWConfig{}, err
	}
	q.Beta1, q.Beta2, q.Epsilon, q.WeightDecay = float64(beta1), float64(beta2), float64(epsilon), float64(weightDecay)
	if err := validateOptions(q); err != nil {
		return Options{}, webgpu.AdamWConfig{}, err
	}
	return q, webgpu.AdamWConfig{Beta1: beta1, Beta2: beta2, Epsilon: epsilon, WeightDecay: weightDecay, MaxBytes: maxBytes}, nil
}

func exactGPUKernelOption(field string, value float64, valid func(float32) bool) (float32, error) {
	if !finite(value) {
		return 0, fmt.Errorf("%w: %s is non-finite and cannot be represented as float32", webgpu.ErrInvalidConfig, field)
	}
	converted := float32(value)
	if converted == 0 && value != 0 {
		return 0, fmt.Errorf("%w: %s rounds to zero and is not exactly representable as float32", webgpu.ErrInvalidConfig, field)
	}
	if !finite(float64(converted)) || float64(converted) != value {
		return 0, fmt.Errorf("%w: %s value %.17g is not exactly representable as float32", webgpu.ErrInvalidConfig, field, value)
	}
	if !valid(converted) {
		if field == "epsilon" && converted == 0 {
			return 0, fmt.Errorf("%w: epsilon is zero after float32 conversion", webgpu.ErrInvalidConfig)
		}
		return 0, fmt.Errorf("%w: %s is invalid after float32 conversion (%v)", webgpu.ErrInvalidConfig, field, converted)
	}
	return converted, nil
}

func validateGPUExactState(s TrainingSnapshot) error {
	if s.Updates > uint64(math.MaxUint32) {
		return fmt.Errorf("%w: updates counter %d exceeds uint32 overflow boundary", webgpu.ErrInvalidConfig, s.Updates)
	}
	for field, values := range map[string][]float64{
		"parameter": flatParameters(s.Parameters),
		"first":     s.Optimizer.First,
		"second":    s.Optimizer.Second,
	} {
		for i, value := range values {
			if !finite(value) {
				return fmt.Errorf("%w: %s[%d] is non-finite and cannot be represented as float32", webgpu.ErrInvalidConfig, field, i)
			}
			converted := float32(value)
			if converted == 0 && value != 0 {
				return fmt.Errorf("%w: %s[%d] rounds to zero and is not exactly representable as float32", webgpu.ErrInvalidConfig, field, i)
			}
			if !finite(float64(converted)) || float64(converted) != value {
				return fmt.Errorf("%w: %s[%d] value %.17g is not exactly representable as float32", webgpu.ErrInvalidConfig, field, i, value)
			}
		}
	}
	for i, step := range s.Optimizer.Steps {
		if step > uint64(math.MaxUint32) {
			return fmt.Errorf("%w: optimizer counter[%d]=%d exceeds uint32 overflow boundary", webgpu.ErrInvalidConfig, i, step)
		}
		if step > s.Updates {
			return fmt.Errorf("%w: optimizer counter[%d]=%d exceeds updates %d", webgpu.ErrInvalidConfig, i, step, s.Updates)
		}
	}
	return nil
}

func validateGPUExactSnapshot(s TrainingSnapshot, trainer *Trainer) error {
	if trainer == nil {
		return fmt.Errorf("nil trainer while validating GPU snapshot")
	}
	if len(s.Optimizer.Steps) != len(trainer.optimizer.Steps) {
		return fmt.Errorf("%w: optimizer counter shape mismatch", webgpu.ErrInvalidConfig)
	}
	return nil
}

func float32Parameters(values []float64) []float32 {
	converted := make([]float32, len(values))
	for i, value := range values {
		converted[i] = float32(value)
	}
	return converted
}

func float32Values(values []float64) []float32 {
	converted := make([]float32, len(values))
	for i, value := range values {
		converted[i] = float32(value)
	}
	return converted
}

func validateDeviceAdamCandidate(candidate webgpu.AdamWSnapshot, shape Parameters, original AdamState, originalParameters []float64, mask []bool, updates uint64) (Parameters, AdamState, []float64, error) {
	count := len(originalParameters)
	if len(candidate.Parameters) != count || len(candidate.First) != count || len(candidate.Second) != count || len(candidate.Steps) != count || len(mask) != count {
		return Parameters{}, AdamState{}, nil, fmt.Errorf("device Adam candidate shape mismatch")
	}
	candidateParameters := make([]float64, count)
	candidateFirst := make([]float64, count)
	candidateSecond := make([]float64, count)
	candidateSteps := make([]uint64, count)
	newUpdates := updates + 1
	for i := 0; i < count; i++ {
		parameter := float64(candidate.Parameters[i])
		first := float64(candidate.First[i])
		second := float64(candidate.Second[i])
		if !finite(parameter) || !finite(first) || !finite(second) || second < 0 {
			return Parameters{}, AdamState{}, nil, fmt.Errorf("%w: device Adam candidate at parameter %d is non-finite", webgpu.ErrNonFinite, i)
		}
		candidateParameters[i], candidateFirst[i], candidateSecond[i] = parameter, first, second
		candidateSteps[i] = uint64(candidate.Steps[i])
		if candidateSteps[i] > newUpdates {
			return Parameters{}, AdamState{}, nil, fmt.Errorf("%w: device Adam candidate counter[%d]=%d exceeds update %d", webgpu.ErrInvalidConfig, i, candidateSteps[i], newUpdates)
		}
		if !mask[i] {
			if parameter != originalParameters[i] || first != original.First[i] || second != original.Second[i] || candidateSteps[i] != original.Steps[i] {
				return Parameters{}, AdamState{}, nil, fmt.Errorf("device Adam candidate changed frozen parameter %d", i)
			}
			continue
		}
		if original.Steps[i] >= uint64(math.MaxUint32) || candidateSteps[i] != original.Steps[i]+1 {
			return Parameters{}, AdamState{}, nil, fmt.Errorf("%w: device Adam counter[%d] did not advance within uint32", webgpu.ErrCapacity, i)
		}
	}
	return unflatten(candidateParameters, shape), AdamState{First: candidateFirst, Second: candidateSecond, Steps: candidateSteps}, candidateParameters, nil
}
