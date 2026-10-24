package learning

import (
	"context"
	"fmt"
)

// PredictBatch runs one frozen, independent episode per input, like Predict
// for each of them, and returns the last-step readouts in input order. On a
// NewGPUBatchTrainer trainer the core of every episode runs on the device in
// one batched episode, in float32, while encoding and readout stay on Insyra
// on the host; inputs must then have equal length. On every other trainer it
// is Predict for each input, on up to workers goroutines. Trainer state never
// changes.
func (tr *Trainer) PredictBatch(ctx context.Context, inputs [][][]float64, workers int) ([][]float64, error) {
	if tr == nil || tr.network == nil || ctx == nil {
		return nil, fmt.Errorf("nil trainer or context")
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("empty batch")
	}
	if err := tr.mu.LockContext(ctx); err != nil {
		return nil, err
	}
	defer tr.mu.Unlock()
	n, p := tr.network, tr.parameters
	out := make([][]float64, len(inputs))
	if tr.batch == nil {
		err := forPairs(ctx, len(inputs), workers, func(i int) error {
			var err error
			out[i], err = n.Predict(ctx, p, inputs[i])
			return err
		})
		return out, err
	}
	steps := len(inputs[0])
	for i, input := range inputs {
		if len(input) != steps {
			return nil, fmt.Errorf("input %d has %d steps, the batch has %d; the batched core needs equal lengths", i, len(input), steps)
		}
	}
	core, err := n.coreParameters(p)
	if err != nil {
		return nil, err
	}
	host, coreInputs, err := n.encodeBatch(p, inputs, false)
	if err != nil {
		return nil, err
	}
	readouts, err := tr.batch.Forward(ctx, core.Core, coreInputs)
	if err != nil {
		return nil, err
	}
	if len(readouts) != len(inputs) {
		return nil, fmt.Errorf("batched core returned %d readouts for %d inputs", len(readouts), len(inputs))
	}
	// Predict reads out the last step; an every-step model's last row is the
	// same readout.
	last := make([][][]float64, len(inputs))
	for i, rows := range readouts {
		if len(rows) == 0 {
			return nil, fmt.Errorf("batched core returned no readout rows")
		}
		last[i] = rows[len(rows)-1:]
	}
	executions, err := n.readoutBatch(host, p, last, 1, false)
	if err != nil {
		return nil, err
	}
	for i, e := range executions {
		if out[i], err = values(e.prediction); err != nil {
			return nil, err
		}
	}
	return out, nil
}
