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
// back. Encoding and readout stay on Insyra on the host, as does AdamW.
//
// The batch gradient enters the same accumulation, clipping, schedule and
// AdamW path as StepBatch on a CPU trainer: the accumulator gains the sum of
// the chunk's gradients and the chunk's pair count, so a full window applies
// their mean. Two things differ. Each pair's StepResult.GradientNorm is the
// norm of its chunk's mean gradient, because single pairs' core gradients
// never leave the device; and a chunk that fails commits none of its pairs.
// Pairs within one chunk must have equal length.
//
// Step, StepFrom, Predict and the candidate check of every update keep the
// CPU core. The configuration must be a scalar continuous core without delays,
// fixed edge signs or parameter sharing, trained with full BPTT (Truncation
// zero), no recompute and no loss scale; anything else is refused with
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
	if c.Sharing != nil {
		return fmt.Errorf("%w: parameter sharing", ErrGPUTrainerUnsupported)
	}
	if hasFixedSigns(c.EdgeSigns) {
		return fmt.Errorf("%w: fixed edge signs", ErrGPUTrainerUnsupported)
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
	executions := make([]*execution, k)
	coreInputs := make([][][]float64, k)
	if err := forPairs(ctx, k, workers, func(i int) error {
		e, encoded, err := n.encode(p, inputs[i], true)
		executions[i], coreInputs[i] = e, encoded
		return err
	}); err != nil {
		return nil, err
	}
	readouts, err := tr.batch.Forward(ctx, core.Core, coreInputs)
	if err != nil {
		return nil, err
	}
	if len(readouts) != k {
		return nil, fmt.Errorf("batched core returned %d readouts for %d pairs", len(readouts), k)
	}
	losses := make([]float64, k)
	readoutGrads := make([][]float64, k)
	upstream := make([][][]float64, k)
	readoutCount := len(n.config.ReadoutNodes)
	if err := forPairs(ctx, k, workers, func(i int) error {
		var selected []float64
		shape := []int{readoutCount}
		for _, row := range readouts[i] {
			if len(row) != readoutCount {
				return fmt.Errorf("batched core returned a readout row of %d values, want %d", len(row), readoutCount)
			}
			selected = append(selected, row...)
		}
		if n.config.ReadoutEveryStep {
			if len(readouts[i]) != steps {
				return fmt.Errorf("batched core returned %d readout steps, want %d", len(readouts[i]), steps)
			}
			shape = []int{steps, readoutCount}
		} else if len(readouts[i]) != 1 {
			return fmt.Errorf("batched core returned %d readout rows for a last-step readout", len(readouts[i]))
		}
		e := executions[i]
		if err := n.readoutFrom(e, p, selected, shape, true); err != nil {
			return err
		}
		loss, up, err := n.lossUpstream(e, steps, targets[i], tr.options.Loss)
		if err != nil {
			return err
		}
		dh, dr, err := n.readoutReverse(e, up)
		if err != nil {
			return err
		}
		losses[i], readoutGrads[i] = loss, doubles(dr.Data())
		upstream[i] = rows(doubles(dh.Data()), readoutCount)
		return nil
	}); err != nil {
		return nil, err
	}
	g, err := tr.batch.Backward(ctx, upstream)
	if err != nil {
		return nil, err
	}
	if len(g.Inputs) != k {
		return nil, fmt.Errorf("batched core returned input gradients for %d pairs, want %d", len(g.Inputs), k)
	}
	encoderGrads := make([][]float64, k)
	width := inputWidth(n.config)
	if err := forPairs(ctx, k, workers, func(i int) error {
		flat := make([]float64, 0, steps*width)
		for _, row := range g.Inputs[i] {
			flat = append(flat, row...)
		}
		encoder, _, err := n.encoderReverse(executions[i], flat)
		encoderGrads[i] = encoder
		return err
	}); err != nil {
		return nil, err
	}
	sum := Gradient{Core: dynamics.Gradient{Weights: g.Weights, Bias: g.Bias, LogTau: g.LogTau, GateGain: g.GateGain, GateBias: g.GateBias}}
	sum.Encoder, sum.Readout = make([]float64, len(p.Encoder)), make([]float64, len(p.Readout))
	for i := range k {
		for j, v := range encoderGrads[i] {
			sum.Encoder[j] += v
		}
		for j, v := range readoutGrads[i] {
			sum.Readout[j] += v
		}
	}
	grad := flatGradient(sum)
	if len(grad) != len(flatParameters(p)) {
		return nil, fmt.Errorf("batched gradient has %d values, the model has %d parameters", len(grad), len(flatParameters(p)))
	}
	mask := parameterMask(p, tr.options, n.core.thetaNodes(), n.core)
	var norm float64
	for i, v := range grad {
		if !finite(v) {
			return nil, fmt.Errorf("non-finite batched gradient at parameter %d", i)
		}
		if mask[i] {
			norm = math.Hypot(norm, v/float64(k))
		}
	}
	if !finite(norm) {
		return nil, fmt.Errorf("non-finite gradient norm")
	}
	updates := tr.updates
	last, err := tr.applyGradientSum(ctx, inputs[k-1], losses[k-1], true, grad, k, norm, mask, nil, nil)
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
