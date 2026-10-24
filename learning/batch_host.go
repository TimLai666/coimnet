package learning

import (
	"fmt"

	"github.com/HazelnutParadise/insyra/nn"
)

// batchHost holds the encoder and readout of a whole batched chunk on one
// Insyra tape each: the pairs' observation rows are stacked into one
// [pairs*steps, input] matrix, so the chunk runs one encoder product and one
// readout product instead of one small product per pair.
//
// Forward values match the per-pair path bit for bit: every output row is the
// same dot products in the same order. The encoder and readout gradients are
// the batch sums, accumulated in float32 inside one product rather than per
// pair and then in float64, so they can differ from the per-pair sum by
// float32 rounding.
type batchHost struct {
	encoderTape, readoutTape *nn.Tape
	encoder, encoded         *nn.Tensor
	h, readout, prediction   *nn.Tensor
	pairs, steps             int
}

// encodeBatch validates the observations of a chunk of equal-length episodes
// and encodes them on one tape (or none when record is unset). It returns the
// encoded rows of every pair, [pair][step][encoder width].
//
// When every observation row is one-hot, the product of the row with the
// encoder is exactly the encoder row it selects, so the rows are looked up
// with Tape.Embedding instead of multiplied: the same values, and a gradient
// that scatter-adds into the selected rows instead of a dense product.
func (n *Network) encodeBatch(p Parameters, inputs [][][]float64, record bool) (*batchHost, [][][]float64, error) {
	pairs := len(inputs)
	if pairs == 0 {
		return nil, nil, fmt.Errorf("empty batch")
	}
	steps := len(inputs[0])
	if steps == 0 {
		return nil, nil, fmt.Errorf("empty input sequence")
	}
	total, err := size(pairs, steps)
	if err != nil {
		return nil, nil, err
	}
	count, err := size(total, n.config.InputSize)
	if err != nil {
		return nil, nil, err
	}
	coreWidth := configNodes(n.config) * n.core.stateDim()
	if _, err := size(steps, coreWidth); err != nil {
		return nil, nil, err
	}
	flat := make([]float64, 0, count)
	labels := make([]int64, 0, total)
	oneHot := true
	for i, input := range inputs {
		if len(input) != steps {
			return nil, nil, fmt.Errorf("pair %d has %d steps, the chunk has %d", i, len(input), steps)
		}
		for s, row := range input {
			if len(row) != n.config.InputSize {
				return nil, nil, fmt.Errorf("pair %d: input[%d] width %d, want %d", i, s, len(row), n.config.InputSize)
			}
			flat = append(flat, row...)
			if oneHot {
				label, ok := oneHotIndex(row)
				oneHot = ok
				labels = append(labels, int64(label))
			}
		}
	}
	encoderWidth := inputWidth(n.config)
	encoder, err := n.encoderCache.get(n.config.InputSize, encoderWidth, p.Encoder)
	if err != nil {
		return nil, nil, err
	}
	b := &batchHost{encoder: encoder, pairs: pairs, steps: steps}
	if record {
		b.encoderTape = nn.NewTape()
		if _, err = b.encoderTape.Param(encoder); err != nil {
			return nil, nil, err
		}
	}
	if oneHot {
		var indices *nn.Tensor
		if indices, err = nn.NewInt64Tensor([]int{total}, labels); err != nil {
			return nil, nil, err
		}
		tape := b.encoderTape
		if tape == nil {
			tape = nn.NewTape()
		}
		b.encoded, err = tape.Embedding(encoder, indices)
	} else {
		var x *nn.Tensor
		if x, err = tensor([]int{total, n.config.InputSize}, flat); err != nil {
			return nil, nil, err
		}
		if record {
			b.encoded, err = b.encoderTape.MatMul(x, encoder)
		} else {
			b.encoded, err = nn.MatMul(x, encoder)
		}
	}
	if err != nil {
		return nil, nil, err
	}
	encoded, err := values(b.encoded)
	if err != nil {
		return nil, nil, err
	}
	all := rows(encoded, encoderWidth)
	out := make([][][]float64, pairs)
	for i := range out {
		out[i] = all[i*steps : (i+1)*steps]
	}
	return b, out, nil
}

// oneHotIndex reports the position of the single 1 of a row whose other
// values are all exactly zero.
func oneHotIndex(row []float64) (int, bool) {
	index := -1
	for i, v := range row {
		switch v {
		case 0:
		case 1:
			if index >= 0 {
				return 0, false
			}
			index = i
		default:
			return 0, false
		}
	}
	return index, index >= 0
}

// readoutBatch runs the readout on the selected core activity of every pair,
// [pair][row][readout node], with readRows rows per pair (1 for a last-step
// model, steps for a per-step one), as one product on one tape. It returns
// one execution per pair whose prediction is that pair's rows, shaped as
// readoutFrom shapes it, so the loss code reads it unchanged.
func (n *Network) readoutBatch(b *batchHost, p Parameters, selected [][][]float64, readRows int, record bool) ([]*execution, error) {
	readoutCount := len(n.config.ReadoutNodes)
	total, err := size(b.pairs, readRows)
	if err != nil {
		return nil, err
	}
	flat := make([]float64, 0, total*readoutCount)
	for i, pair := range selected {
		if len(pair) != readRows {
			return nil, fmt.Errorf("pair %d has %d readout rows, want %d", i, len(pair), readRows)
		}
		for _, row := range pair {
			if len(row) != readoutCount {
				return nil, fmt.Errorf("batched core returned a readout row of %d values, want %d", len(row), readoutCount)
			}
			flat = append(flat, row...)
		}
	}
	if b.h, err = tensor([]int{total, readoutCount}, flat); err != nil {
		return nil, err
	}
	if b.readout, err = n.readoutCache.get(readoutCount, n.config.OutputSize, p.Readout); err != nil {
		return nil, err
	}
	if record {
		b.readoutTape = nn.NewTape()
		if _, err = b.readoutTape.Param(b.h); err != nil {
			return nil, err
		}
		if _, err = b.readoutTape.Param(b.readout); err != nil {
			return nil, err
		}
		b.prediction, err = b.readoutTape.MatMul(b.h, b.readout)
	} else {
		b.prediction, err = nn.MatMul(b.h, b.readout)
	}
	if err != nil {
		return nil, err
	}
	predictions, err := b.prediction.Float32Data()
	if err != nil {
		return nil, err
	}
	for _, v := range predictions {
		if !finite(float64(v)) {
			return nil, fmt.Errorf("non-finite prediction")
		}
	}
	shape := []int{n.config.OutputSize}
	if n.config.ReadoutEveryStep {
		shape = []int{readRows, n.config.OutputSize}
	}
	width := readRows * n.config.OutputSize
	executions := make([]*execution, b.pairs)
	for i := range executions {
		pred, err := nn.NewTensor(shape, predictions[i*width:(i+1)*width])
		if err != nil {
			return nil, err
		}
		executions[i] = &execution{prediction: pred}
	}
	return executions, nil
}

// readoutBatchReverse seeds the batched readout with every pair's readout
// upstream, [pair][step][output] as lossUpstream returns it, and returns each
// pair's selected-activity gradient rows and the batch-summed readout
// gradient.
func (n *Network) readoutBatchReverse(b *batchHost, upstream [][][]float64) ([][][]float64, []float64, error) {
	readRows := b.prediction.Shape()[0] / b.pairs
	seed := make([]float64, 0, b.prediction.Shape()[0]*n.config.OutputSize)
	for _, up := range upstream {
		if n.config.ReadoutEveryStep {
			for _, row := range up {
				seed = append(seed, row...)
			}
		} else {
			seed = append(seed, up[len(up)-1]...)
		}
	}
	seedTensor, err := tensor(b.prediction.Shape(), seed)
	if err != nil {
		return nil, nil, err
	}
	if err = b.readoutTape.BackwardFrom(b.prediction, seedTensor); err != nil {
		return nil, nil, err
	}
	dh, err := b.readoutTape.Grad(b.h)
	if err != nil {
		return nil, nil, err
	}
	dr, err := b.readoutTape.Grad(b.readout)
	if err != nil {
		return nil, nil, err
	}
	dhValues, err := values(dh)
	if err != nil {
		return nil, nil, err
	}
	all := rows(dhValues, len(n.config.ReadoutNodes))
	out := make([][][]float64, b.pairs)
	for i := range out {
		out[i] = all[i*readRows : (i+1)*readRows]
	}
	readout, err := values(dr)
	if err != nil {
		return nil, nil, err
	}
	return out, readout, nil
}

// encoderBatchReverse seeds the batched encoder with every pair's core input
// gradient, [pair][step][encoder width], and returns the batch-summed encoder
// gradient.
func (n *Network) encoderBatchReverse(b *batchHost, inputs [][][]float64) ([]float64, error) {
	width := inputWidth(n.config)
	seed := make([]float64, 0, b.pairs*b.steps*width)
	for i, pair := range inputs {
		if len(pair) != b.steps {
			return nil, fmt.Errorf("pair %d has %d input gradient rows, want %d", i, len(pair), b.steps)
		}
		for _, row := range pair {
			if len(row) != width {
				return nil, fmt.Errorf("pair %d has an input gradient row of %d values, want %d", i, len(row), width)
			}
			seed = append(seed, row...)
		}
	}
	seedTensor, err := tensor(b.encoded.Shape(), seed)
	if err != nil {
		return nil, err
	}
	if err = b.encoderTape.BackwardFrom(b.encoded, seedTensor); err != nil {
		return nil, err
	}
	de, err := b.encoderTape.Grad(b.encoder)
	if err != nil {
		return nil, err
	}
	encoder, err := values(de)
	if err != nil {
		return nil, err
	}
	for _, x := range encoder {
		if !finite(x) {
			return nil, fmt.Errorf("non-finite boundary gradient")
		}
	}
	return encoder, nil
}
