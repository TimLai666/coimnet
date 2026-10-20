package learning

import (
	"context"
	"fmt"
	"math"
	"sync"

	"github.com/TimLai666/coimnet/backend/webgpu"
	"github.com/TimLai666/coimnet/dynamics"
)

// batchCore runs the continuous core of a whole minibatch of fresh episodes:
// Forward returns the readout nodes of every sequence and keeps the episode,
// Backward returns the batch-summed parameter gradients and each sequence's
// input-column gradients. *dynamics.GPUContinuousBatch implements it.
type batchCore interface {
	Forward(ctx context.Context, p dynamics.Parameters, inputs [][][]float64) ([][][]float64, error)
	Backward(ctx context.Context, upstream [][][]float64) (dynamics.BatchGradient, error)
	Info() webgpu.DeviceInfo
	Close() error
}

// NewGPUBatchTrainer constructs a trainer whose StepBatch runs the continuous
// core of each minibatch on the device in one batched episode: the core
// forward and full reverse pass of every pair stay on the GPU and only the
// readout activity, the summed core gradients and the input gradients come
// back. Encoding and readout stay on Insyra on the host, as does AdamW: the
// chunk's observation rows are stacked and run through one encoder product
// (a one-hot chunk through Tape.Embedding) and one readout product.
//
// The batch gradient enters the same accumulation, clipping, schedule and
// AdamW path as StepBatch on a CPU trainer: the accumulator gains the sum of
// the chunk's gradients and the chunk's pair count, so a full window applies
// their mean. Three things differ. The encoder and readout gradients are
// summed over the chunk in float32 inside one product, not per pair and then
// in float64, so they match StepBatch to float32 rounding. Each pair's
// StepResult.GradientNorm is the norm of its chunk's mean gradient, because
// single pairs' core gradients never leave the device. And a chunk that fails
// commits none of its pairs.
// Pairs within one chunk must have equal length.
//
// Step, StepFrom, Predict and the candidate check of every update keep the
// CPU core. The configuration must be a scalar continuous core without
// delays, trained with full BPTT (Truncation zero), no recompute and no loss
// scale. Fixed edge signs and parameter sharing are supported by converting
// and reducing the summed gradient on the host. Anything else is refused with
// ErrGPUTrainerUnsupported before a device is opened. GPU errors are returned
// directly: there is no CPU fallback for the batched core.
func NewGPUBatchTrainer(ctx context.Context, c Config, p Parameters, o Options) (*Trainer, error) {
	if err := validateGPUBatchTrainerSupport(c, o); err != nil {
		return nil, err
	}
	trainer, err := NewTrainer(c, p, o)
	if err != nil {
		return nil, err
	}
	return attachGPUBatch(ctx, trainer)
}

// RestoreGPUBatchTrainer restores an episode snapshot into the batched GPU
// trainer. The snapshot schema is unchanged, and an unsupported configuration
// is refused rather than restored as a CPU trainer.
func RestoreGPUBatchTrainer(ctx context.Context, s TrainingSnapshot) (*Trainer, error) {
	if err := validateGPUBatchTrainerSupport(s.Config, s.Options); err != nil {
		return nil, err
	}
	trainer, err := RestoreTrainer(s)
	if err != nil {
		return nil, err
	}
	return attachGPUBatch(ctx, trainer)
}

func validateGPUBatchTrainerSupport(c Config, o Options) error {
	if err := validateGPUTrainerSupport(c, o); err != nil {
		return err
	}
	if o.Truncation != 0 {
		return fmt.Errorf("%w: truncated BPTT window %d; the batched core runs full BPTT only", ErrGPUTrainerUnsupported, o.Truncation)
	}
	if o.LossScale != 0 && o.LossScale != 1 {
		return fmt.Errorf("%w: loss scale %g", ErrGPUTrainerUnsupported, o.LossScale)
	}
	return nil
}

func attachGPUBatch(ctx context.Context, trainer *Trainer) (*Trainer, error) {
	if trainer == nil || trainer.network == nil {
		return nil, fmt.Errorf("nil trainer")
	}
	core, ok := trainer.network.core.(continuousCore)
	if !ok {
		return nil, fmt.Errorf("%w: batched GPU training requires a continuous core", ErrGPUTrainerUnsupported)
	}
	c := trainer.network.config
	gpu, err := dynamics.NewGPUContinuousBatch(ctx, core.model.Config(), dynamics.BatchLayout{InputNodes: c.InputNodes, ReadoutNodes: c.ReadoutNodes, ReadoutEveryStep: c.ReadoutEveryStep})
	if err != nil {
		return nil, fmt.Errorf("construct batched GPU core: %w", err)
	}
	trainer.batch = gpu
	return trainer, nil
}

// stepBatchCore is StepBatch on a trainer with a batched core. Its chunks are
// the ones StepBatch forms: the pairs up to the end of the open accumulation
// window, which all read the current parameters.
func (tr *Trainer) stepBatchCore(ctx context.Context, inputs [][][]float64, targets [][]float64, workers int) ([]StepResult, error) {
	results := make([]StepResult, 0, len(inputs))
	window := accumulateSteps(tr.options)
	for start := 0; start < len(inputs); {
		open := 0
		if a := tr.accumulator; window > 1 && a != nil && a.Count < window {
			open = a.Count
		}
		end := min(start+window-open, len(inputs))
		if tr.updates == math.MaxUint64 {
			return results, fmt.Errorf("pair %d: update counter overflow", start)
		}
		chunk, err := tr.batchChunk(ctx, inputs[start:end], targets[start:end], workers, open)
		if err != nil {
			return results, fmt.Errorf("pairs %d to %d: %w", start, end-1, err)
		}
		results = append(results, chunk...)
		start = end
	}
	return results, nil
}

// batchChunk computes the summed gradient of one chunk with the core on the
// device and commits it as len(inputs) accumulated gradients.
func (tr *Trainer) batchChunk(ctx context.Context, inputs [][][]float64, targets [][]float64, workers, open int) ([]StepResult, error) {
	n := tr.network
	k := len(inputs)
	steps := len(inputs[0])
	for i, input := range inputs {
		if len(input) != steps {
			return nil, fmt.Errorf("pair %d has %d steps, the chunk has %d; the batched core needs equal lengths", i, len(input), steps)
		}
	}
	p := tr.parameters
	core, err := n.coreParameters(p)
	if err != nil {
		return nil, err
	}
	host, coreInputs, err := n.encodeBatch(p, inputs, true)
	if err != nil {
		return nil, err
	}
	readouts, err := tr.batch.Forward(ctx, core.Core, coreInputs)
	if err != nil {
		return nil, err
	}
	if len(readouts) != k {
		return nil, fmt.Errorf("batched core returned %d readouts for %d pairs", len(readouts), k)
	}
	readRows := 1
	if n.config.ReadoutEveryStep {
		readRows = steps
	}
	executions, err := n.readoutBatch(host, p, readouts, readRows, true)
	if err != nil {
		return nil, err
	}
	losses := make([]float64, k)
	lossUp := make([][][]float64, k)
	if err := forPairs(ctx, k, workers, func(i int) error {
		loss, up, err := n.lossUpstream(executions[i], steps, targets[i], tr.options.Loss)
		losses[i], lossUp[i] = loss, up
		return err
	}); err != nil {
		return nil, err
	}
	upstream, readoutGrad, err := n.readoutBatchReverse(host, lossUp)
	if err != nil {
		return nil, err
	}
	g, err := tr.batch.Backward(ctx, upstream)
	if err != nil {
		return nil, err
	}
	// The device integrated the effective weights, so a fixed-sign edge's
	// summed gradient is with respect to w = sign * exp(rho); the chain rule
	// d loss/d rho = d loss/d w * w applies to the sum because w is the same
	// for every pair of the chunk.
	if n.fixed {
		if len(g.Weights) != len(n.config.EdgeSigns) {
			return nil, fmt.Errorf("batched weight gradient has %d values, the core declares %d signs", len(g.Weights), len(n.config.EdgeSigns))
		}
		for i, sign := range n.config.EdgeSigns {
			if sign != 0 {
				g.Weights[i] *= core.Core.Weights[i]
			}
		}
	}
	if len(g.Inputs) != k {
		return nil, fmt.Errorf("batched core returned input gradients for %d pairs, want %d", len(g.Inputs), k)
	}
	encoderGrad, err := n.encoderBatchReverse(host, g.Inputs)
	if err != nil {
		return nil, err
	}
	sum := Gradient{Core: dynamics.Gradient{Weights: g.Weights, Bias: g.Bias, LogTau: g.LogTau, GateGain: g.GateGain, GateBias: g.GateBias}}
	sum.Encoder, sum.Readout = encoderGrad, readoutGrad
	grad := flatGradient(sum)
	if len(grad) != len(flatParameters(p)) {
		return nil, fmt.Errorf("batched gradient has %d values, the model has %d parameters", len(grad), len(flatParameters(p)))
	}
	mask := parameterMask(p, tr.options, n.core.thetaNodes(), n.core)
	for i, v := range grad {
		if !finite(v) {
			return nil, fmt.Errorf("non-finite batched gradient at parameter %d", i)
		}
	}
	// Sharing sums each group's member gradients, a linear map, so reducing
	// the chunk sum equals summing the per-pair reductions StepBatch forms.
	var plan *sharePlan
	var norm float64
	if n.config.Sharing != nil {
		plan = newSharePlan(n.config.Sharing, len(p.Core.Weights), len(p.Core.Bias), len(p.Core.LogTau))
		reduceSharedGradients(grad, plan)
		mask, _ = sharedMaskFreeze(mask, plan)
		norm = reducedNorm(grad, mask, plan) / float64(k)
	} else {
		for i, v := range grad {
			if mask[i] {
				norm = math.Hypot(norm, v/float64(k))
			}
		}
	}
	if !finite(norm) {
		return nil, fmt.Errorf("non-finite gradient norm")
	}
	updates := tr.updates
	last, err := tr.applyGradientSum(ctx, inputs[k-1], losses[k-1], true, grad, k, norm, mask, plan, nil)
	if err != nil {
		return nil, err
	}
	horizon := gradientHorizon(steps-1, 0, 0)
	results := make([]StepResult, k)
	for i := 0; i < k-1; i++ {
		results[i] = StepResult{Loss: losses[i], LossKnown: true, GradientNorm: norm, Updates: updates, Accumulated: open + i + 1, GradientHorizonSteps: horizon}
	}
	last.GradientHorizonSteps = horizon
	results[k-1] = last
	return results, nil
}

// forPairs runs fn for every pair index on up to workers goroutines and
// returns the error of the lowest failing index. A cancelled context stops
// the pairs that have not started.
func forPairs(ctx context.Context, count, workers int, fn func(int) error) error {
	errs := make([]error, count)
	next := make(chan int)
	var wg sync.WaitGroup
	for range min(max(workers, 1), count) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				errs[i] = fn(i)
			}
		}()
	}
	for i := 0; i < count; i++ {
		if ctx.Err() != nil {
			break
		}
		next <- i
	}
	close(next)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}
	for i, err := range errs {
		if err != nil {
			return fmt.Errorf("pair %d: %w", i, err)
		}
	}
	return nil
}
