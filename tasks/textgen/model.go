// Package textgen provides byte-level text generation models on a CoImNet core.
package textgen

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/textgen/tokenizer"
)

// ModelConfig declares one byte-level language model on a CoImNet core.
type ModelConfig struct {
	Embed        int     `json:"embed"`         // 4..256 input nodes for encoded one-hot tokens.
	Hidden       int     `json:"hidden"`        // 4..512 recurrent core nodes read by the output layer.
	Settle       int     `json:"settle"`        // 1..8 rows each token is held; the last predicts the next token.
	LearningRate float64 `json:"learning_rate"` // Finite and positive.
	Seed         uint64  `json:"seed"`
}

// Validate reports a field-specific error when a model setting is invalid.
func (c ModelConfig) Validate() error {
	if c.Embed < 4 || c.Embed > 256 {
		return fmt.Errorf("embed must be in [4, 256]")
	}
	if c.Hidden < 4 || c.Hidden > 512 {
		return fmt.Errorf("hidden must be in [4, 512]")
	}
	if c.Settle < 1 || c.Settle > 8 {
		return fmt.Errorf("settle must be in [1, 8]")
	}
	if math.IsNaN(c.LearningRate) || math.IsInf(c.LearningRate, 0) || c.LearningRate <= 0 {
		return fmt.Errorf("learning_rate must be finite and positive")
	}
	return nil
}

// DefaultModelConfig returns the declared default byte language model settings.
func DefaultModelConfig() ModelConfig {
	return ModelConfig{Embed: 16, Hidden: 32, Settle: 3, LearningRate: 0.01, Seed: 1}
}

// Example is one training or scoring sequence. IDs[t] is the token at position t;
// Scored[t] says whether predicting IDs[t] from IDs[:t] counts in the loss.
// Scored has the length of IDs, Scored[0] must be false, and at least one
// position must be scored. Padding and an unscored prompt prefix are marked false.
type Example struct {
	IDs    []int  `json:"ids"`
	Scored []bool `json:"scored"`
}

// Model is a byte-level language model: the one-hot of each token (width
// Vocabulary().Size()) is encoded into Embed input nodes by a trainable
// encoder, held for Settle rows, and the readout of the Hidden nodes on the
// last row of that hold predicts the next token.
type Model struct {
	vocab     tokenizer.Vocabulary
	vocabSize int
	config    ModelConfig
	trainer   *learning.Trainer
}

// NewModel builds a core with Embed input nodes followed by Hidden hidden
// nodes. Edges are every input-to-hidden edge followed by every hidden-to-hidden
// edge, including self-loops. Weights use [-1/sqrt(Embed+Hidden),
// 1/sqrt(Embed+Hidden)) from PCG(Seed, 0); the encoder uses [-0.5, 0.5) from
// PCG(Seed, 1), and the readout uses [-0.1, 0.1) from PCG(Seed, 2). Biases
// start at zero, time constants at log(2), DT is 1, activation is tanh, and
// the readout runs on every row. DefaultOptions is used with LearningRate and
// the encoder, weights, time constants and readout trainable; bias is frozen.
func NewModel(vocab tokenizer.Vocabulary, c ModelConfig) (*Model, error) {
	if vocab == nil {
		return nil, fmt.Errorf("vocabulary must not be nil")
	}
	vocabSize := vocab.Size()
	if vocabSize < 2 {
		return nil, fmt.Errorf("vocabulary size must be at least 2")
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if c.Embed > int(^uint(0)>>1)-c.Hidden {
		return nil, fmt.Errorf("embed plus hidden overflows node count")
	}
	inputs, hidden := c.Embed, c.Hidden
	nodes := inputs + hidden
	sources := make([]int, 0, inputs*hidden+hidden*hidden)
	targets := make([]int, 0, cap(sources))
	appendBlock := func(fromStart, fromCount, toStart, toCount int) {
		for from := fromStart; from < fromStart+fromCount; from++ {
			for to := toStart; to < toStart+toCount; to++ {
				sources = append(sources, from)
				targets = append(targets, to)
			}
		}
	}
	appendBlock(0, inputs, inputs, hidden)
	appendBlock(inputs, hidden, inputs, hidden)

	inputNodes := make([]int, inputs)
	for i := range inputNodes {
		inputNodes[i] = i
	}
	readoutNodes := make([]int, hidden)
	for i := range readoutNodes {
		readoutNodes[i] = inputs + i
	}
	config := learning.Config{
		Dynamics: dynamics.Config{
			Nodes: nodes, Sources: sources, Targets: targets, DT: 1, Activation: "tanh",
		},
		InputSize:        vocabSize,
		OutputSize:       vocabSize,
		ReadoutNodes:     readoutNodes,
		InputNodes:       inputNodes,
		ReadoutEveryStep: true,
	}
	weights := uniform(rand.NewPCG(c.Seed, 0), len(sources), 1/math.Sqrt(float64(nodes)))
	encoder := uniform(rand.NewPCG(c.Seed, 1), vocabSize*inputs, 0.5)
	readout := uniform(rand.NewPCG(c.Seed, 2), hidden*vocabSize, 0.1)
	logTau := make([]float64, nodes)
	for i := range logTau {
		logTau[i] = math.Log(2)
	}
	parameters := learning.Parameters{
		Core:    dynamics.Parameters{Weights: weights, Bias: make([]float64, nodes), LogTau: logTau},
		Encoder: encoder,
		Readout: readout,
	}
	options := learning.DefaultOptions()
	options.LearningRate = c.LearningRate
	options.Trainable = learning.Trainable{Encoder: true, Weights: true, Tau: true, Readout: true}
	trainer, err := learning.NewTrainer(config, parameters, options)
	if err != nil {
		return nil, fmt.Errorf("create text generation trainer: %w", err)
	}
	return &Model{vocab: vocab, vocabSize: vocabSize, config: c, trainer: trainer}, nil
}

// Vocabulary returns the vocabulary this model was built with.
func (m *Model) Vocabulary() tokenizer.Vocabulary {
	if m == nil {
		return nil
	}
	return m.vocab
}

// Config returns the model configuration this model was built with.
func (m *Model) Config() ModelConfig {
	if m == nil {
		return ModelConfig{}
	}
	return m.config
}

// Snapshot returns an independent snapshot of the trainer state.
func (m *Model) Snapshot() learning.TrainingSnapshot {
	if m == nil || m.trainer == nil {
		return learning.TrainingSnapshot{}
	}
	return m.trainer.Snapshot()
}

// rows lays IDs out as core input rows, holding each one-hot token for Settle
// consecutive rows.
func (m *Model) rows(ids []int) ([][]float64, error) {
	if m == nil || m.vocab == nil {
		return nil, fmt.Errorf("nil model or vocabulary")
	}
	if len(ids) > int(^uint(0)>>1)/m.config.Settle {
		return nil, fmt.Errorf("input sequence is too long")
	}
	rows := make([][]float64, len(ids)*m.config.Settle)
	for position, id := range ids {
		if id < 0 || id >= m.vocabSize {
			return nil, fmt.Errorf("id %d at position %d is outside vocabulary size %d", id, position, m.vocabSize)
		}
		for hold := 0; hold < m.config.Settle; hold++ {
			row := make([]float64, m.vocabSize)
			row[id] = 1
			rows[position*m.config.Settle+hold] = row
		}
	}
	return rows, nil
}

// Logits returns the next-token logits after the final row of each token's
// hold. Each row predicts the token immediately after the corresponding ID.
func (m *Model) Logits(ctx context.Context, ids []int) ([][]float64, error) {
	if m == nil || m.trainer == nil || m.vocab == nil {
		return nil, fmt.Errorf("nil model")
	}
	if ctx == nil {
		return nil, fmt.Errorf("context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	input, err := m.rows(ids)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return make([][]float64, 0), nil
	}
	all, err := m.trainer.PredictAll(ctx, input)
	if err != nil {
		return nil, err
	}
	if len(all) != len(input) {
		return nil, fmt.Errorf("trainer returned %d logits rows, want %d", len(all), len(input))
	}
	result := make([][]float64, len(ids))
	for position := range result {
		row := all[(position+1)*m.config.Settle-1]
		if len(row) != m.vocabSize {
			return nil, fmt.Errorf("trainer returned %d logits at position %d, want %d", len(row), position, m.vocabSize)
		}
		result[position] = append([]float64(nil), row...)
	}
	return result, nil
}

// NLL returns the summed negative log-likelihood in nats and the number of
// scored positions without changing model state.
func (m *Model) NLL(ctx context.Context, ex Example) (sum float64, count int, err error) {
	if err := m.validateExample(ex); err != nil {
		return 0, 0, err
	}
	logits, err := m.Logits(ctx, ex.IDs)
	if err != nil {
		return 0, 0, err
	}
	sum, count, _, err = m.lossAndUpstream(ex, logits)
	return sum, count, err
}

// Step performs one gradient update using the scored next-token positions and
// returns their mean negative log-likelihood before the update.
func (m *Model) Step(ctx context.Context, ex Example) (learning.StepResult, float64, error) {
	var zero learning.StepResult
	if m == nil || m.trainer == nil || m.vocab == nil {
		return zero, 0, fmt.Errorf("nil model")
	}
	if ctx == nil {
		return zero, 0, fmt.Errorf("context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return zero, 0, err
	}
	if err := m.validateExample(ex); err != nil {
		return zero, 0, err
	}
	input, err := m.rows(ex.IDs)
	if err != nil {
		return zero, 0, err
	}
	all, err := m.trainer.PredictAll(ctx, input)
	if err != nil {
		return zero, 0, err
	}
	if len(all) != len(input) {
		return zero, 0, fmt.Errorf("trainer returned %d logits rows, want %d", len(all), len(input))
	}
	logits := make([][]float64, len(ex.IDs))
	for position := range logits {
		row := all[(position+1)*m.config.Settle-1]
		if len(row) != m.vocabSize {
			return zero, 0, fmt.Errorf("trainer returned %d logits at position %d, want %d", len(row), position, m.vocabSize)
		}
		logits[position] = row
	}
	sum, count, upstream, err := m.lossAndUpstream(ex, logits)
	if err != nil {
		return zero, 0, err
	}
	result, err := m.trainer.StepFrom(ctx, input, upstream)
	if err != nil {
		return zero, 0, err
	}
	return result, sum / float64(count), nil
}

// upstream constructs the row-level logit gradient for scored next-token
// targets. Loss positions map to the final row of the preceding token hold.
func (m *Model) upstream(ex Example, logits [][]float64) ([][]float64, int, error) {
	if err := m.validateExample(ex); err != nil {
		return nil, 0, err
	}
	_, count, upstream, err := m.lossAndUpstream(ex, logits)
	return upstream, count, err
}

// validateExample checks sequence lengths, score-mask semantics and token IDs.
func (m *Model) validateExample(ex Example) error {
	if m == nil || m.vocab == nil {
		return fmt.Errorf("nil model or vocabulary")
	}
	if len(ex.IDs) != len(ex.Scored) {
		return fmt.Errorf("example IDs and Scored lengths differ")
	}
	if len(ex.IDs) == 0 {
		return fmt.Errorf("example must not be empty")
	}
	if ex.Scored[0] {
		return fmt.Errorf("scored[0] must be false")
	}
	scored := 0
	for i, id := range ex.IDs {
		if id < 0 || id >= m.vocabSize {
			return fmt.Errorf("id %d at position %d is outside vocabulary size %d", id, i, m.vocabSize)
		}
		if ex.Scored[i] {
			scored++
		}
	}
	if scored == 0 {
		return fmt.Errorf("example must have at least one scored position")
	}
	return nil
}

// lossAndUpstream computes stable summed NLL and its normalized logit gradient.
func (m *Model) lossAndUpstream(ex Example, logits [][]float64) (float64, int, [][]float64, error) {
	if len(logits) != len(ex.IDs) {
		return 0, 0, nil, fmt.Errorf("logits rows %d do not match IDs length %d", len(logits), len(ex.IDs))
	}
	count := 0
	for _, score := range ex.Scored {
		if score {
			count++
		}
	}
	if count == 0 {
		return 0, 0, nil, fmt.Errorf("example must have at least one scored position")
	}
	if len(ex.IDs) > int(^uint(0)>>1)/m.config.Settle {
		return 0, 0, nil, fmt.Errorf("input sequence is too long")
	}
	upstream := make([][]float64, len(ex.IDs)*m.config.Settle)
	for i := range upstream {
		upstream[i] = make([]float64, m.vocabSize)
	}
	var sum float64
	for target := 1; target < len(ex.IDs); target++ {
		if !ex.Scored[target] {
			continue
		}
		row := logits[target-1]
		if len(row) != m.vocabSize {
			return 0, 0, nil, fmt.Errorf("logits at position %d have width %d, want %d", target-1, len(row), m.vocabSize)
		}
		maxLogit := math.Inf(-1)
		for _, value := range row {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return 0, 0, nil, fmt.Errorf("logits at position %d contain a non-finite value", target-1)
			}
			if value > maxLogit {
				maxLogit = value
			}
		}
		expSum := 0.0
		for _, value := range row {
			expSum += math.Exp(value - maxLogit)
		}
		if !finite(expSum) || expSum <= 0 {
			return 0, 0, nil, fmt.Errorf("invalid logit normalization at position %d", target-1)
		}
		logNormalizer := maxLogit + math.Log(expSum)
		loss := logNormalizer - row[ex.IDs[target]]
		if !finite(loss) {
			return 0, 0, nil, fmt.Errorf("non-finite loss at position %d", target)
		}
		sum += loss
		if !finite(sum) {
			return 0, 0, nil, fmt.Errorf("non-finite summed loss")
		}
		gradient := upstream[target*m.config.Settle-1]
		for id, value := range row {
			gradient[id] = math.Exp(value-maxLogit) / expSum / float64(count)
		}
		gradient[ex.IDs[target]] -= 1 / float64(count)
	}
	return sum, count, upstream, nil
}

// uniform returns n deterministic values in [-scale, scale) from source.
func uniform(source rand.Source, n int, scale float64) []float64 {
	rng := rand.New(source)
	values := make([]float64, n)
	for i := range values {
		values[i] = (2*rng.Float64() - 1) * scale
	}
	return values
}

// finite reports whether v is neither NaN nor infinite.
func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
