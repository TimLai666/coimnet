package learning

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sync"
)

// StepBatch runs Step on each (inputs[i], targets[i]) pair in order and
// returns one StepResult per pair. Pairs whose gradients are taken from the
// same parameter version, that is the pairs inside one accumulation window,
// have their gradients computed concurrently on up to workers goroutines
// (zero or less means GOMAXPROCS). Each gradient is then fed through the
// same loss-scale, accumulation, clip, schedule and AdamW path in pair order,
// so the trainer ends in exactly the state the same sequence of Step calls
// leaves, bit for bit. With Options.AccumulateSteps of one every pair is its
// own update and nothing runs concurrently; minibatch training sets
// AccumulateSteps to the batch size.
//
// When a pair fails, the pairs before it stay committed exactly as if Step had
// been called for them, the returned slice holds their results, and the error
// names the failing pair.
//
// On a NewGPUBatchTrainer trainer each such window chunk runs as one batched
// device episode instead; see NewGPUBatchTrainer for what differs.
func (tr *Trainer) StepBatch(ctx context.Context, inputs [][][]float64, targets [][]float64, workers int) ([]StepResult, error) {
	if tr == nil || ctx == nil {
		return nil, fmt.Errorf("nil trainer or context")
	}
	if tr.network == nil {
		return nil, fmt.Errorf("nil trainer")
	}
	if len(inputs) != len(targets) {
		return nil, fmt.Errorf("%d inputs but %d targets", len(inputs), len(targets))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	if err := tr.mu.LockContext(ctx); err != nil {
		return nil, err
	}
	defer tr.mu.Unlock()
	if tr.batch != nil {
		return tr.stepBatchCore(ctx, inputs, targets, workers)
	}
	results := make([]StepResult, 0, len(inputs))
	window := accumulateSteps(tr.options)
	for start := 0; start < len(inputs); {
		// The pairs up to the end of the open accumulation window all read the
		// current parameters, so their gradients are independent.
		open := 0
		if a := tr.accumulator; window > 1 && a != nil && a.Count < window {
			open = a.Count
		}
		end := min(start+window-open, len(inputs))
		grads, err := tr.batchGradients(ctx, inputs[start:end], targets[start:end], workers)
		for i, g := range grads {
			pair := start + i
			if g.err != nil {
				return results, fmt.Errorf("pair %d: %w", pair, g.err)
			}
			if tr.updates == math.MaxUint64 {
				return results, fmt.Errorf("pair %d: update counter overflow", pair)
			}
			result, err := tr.stepWithGradient(ctx, inputs[pair], g.loss, true, g.gradient, nil)
			if err != nil {
				return results, fmt.Errorf("pair %d: %w", pair, err)
			}
			result.GradientHorizonSteps = gradientHorizon(len(inputs[pair])-1, tr.options.Truncation, truncationOffset(tr.options, len(inputs[pair])))
			results = append(results, result)
		}
		if err != nil {
			return results, err
		}
		start = end
	}
	return results, nil
}

type pairGradient struct {
	loss     float64
	gradient Gradient
	err      error
}

// batchGradients computes the loss gradient of every pair against the current
// parameters on up to workers goroutines. A cancelled context stops the pairs
// that have not started and is returned once every running pair is done.
func (tr *Trainer) batchGradients(ctx context.Context, inputs [][][]float64, targets [][]float64, workers int) ([]pairGradient, error) {
	out := make([]pairGradient, len(inputs))
	next := make(chan int)
	var wg sync.WaitGroup
	for range min(workers, len(inputs)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				o := tr.options
				out[i].loss, out[i].gradient, out[i].err = tr.network.lossGradient(ctx, tr.parameters, inputs[i], targets[i], o.Truncation, truncationOffset(o, len(inputs[i])), recomputeSegment(o), o.Loss)
			}
		}()
	}
	sent := 0
	for i := range inputs {
		if ctx.Err() != nil {
			break
		}
		next <- i
		sent++
	}
	close(next)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		for i := sent; i < len(out); i++ {
			out[i].err = err
		}
		return out[:sent], err
	}
	return out, nil
}
