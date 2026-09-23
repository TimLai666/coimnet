package textgen

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"

	"github.com/TimLai666/coimnet/tasks/textgen/tokenizer"
)

// GenerateConfig controls one generation.
type GenerateConfig struct {
	MaxTokens   int     `json:"max_tokens"`  // 1..4096 new tokens at most.
	Temperature float64 `json:"temperature"` // Finite and non-negative; zero means greedy argmax.
	TopK        int     `json:"top_k"`       // Zero disables the filter; otherwise at least one.
	TopP        float64 `json:"top_p"`       // (0, 1]; one disables the filter.
	Seed        uint64  `json:"seed"`        // PCG seed used for every sample.
	StopAtEOS   bool    `json:"stop_at_eos"` // Stop after emitting the special named "eos".
}

// Validate checks generation limits and reports the invalid field name.
func (c GenerateConfig) Validate() error {
	if c.MaxTokens < 1 || c.MaxTokens > 4096 {
		return fmt.Errorf("max_tokens must be in [1, 4096]")
	}
	if math.IsNaN(c.Temperature) || math.IsInf(c.Temperature, 0) || c.Temperature < 0 {
		return fmt.Errorf("temperature must be finite and non-negative")
	}
	if c.TopK < 0 {
		return fmt.Errorf("top_k must be non-negative")
	}
	if math.IsNaN(c.TopP) || math.IsInf(c.TopP, 0) || c.TopP <= 0 || c.TopP > 1 {
		return fmt.Errorf("top_p must be in (0, 1]")
	}
	return nil
}

// Generation is one result. Text contains everything the stream decoder
// emitted; bytes still buffered at the end are counted in IncompleteUTF8Bytes.
type Generation struct {
	PromptIDs           []int  `json:"prompt_ids"`
	IDs                 []int  `json:"ids"`
	Text                string `json:"text"`
	Stop                string `json:"stop"`
	IncompleteUTF8Bytes int    `json:"incomplete_utf8_bytes"`
	VocabHash           string `json:"vocab_hash"`
}

// Generate produces up to MaxTokens ids from m without changing its state.
func Generate(ctx context.Context, m *Model, prompt []int, c GenerateConfig) (Generation, error) {
	if m == nil {
		return Generation{}, fmt.Errorf("model must not be nil")
	}
	return generateWith(ctx, m.vocab, m.Logits, prompt, c)
}

// generateWith produces tokens using logits, which makes the generation loop
// independently testable from a trained model.
func generateWith(ctx context.Context, vocab tokenizer.Vocabulary, logits func(context.Context, []int) ([][]float64, error), prompt []int, c GenerateConfig) (Generation, error) {
	if err := c.Validate(); err != nil {
		return Generation{}, err
	}
	if ctx == nil {
		return Generation{}, fmt.Errorf("context must not be nil")
	}
	if vocab == nil {
		return Generation{}, fmt.Errorf("vocabulary must not be nil")
	}
	if logits == nil {
		return Generation{}, fmt.Errorf("logits function must not be nil")
	}
	if len(prompt) == 0 {
		return Generation{}, fmt.Errorf("prompt must contain at least one id")
	}
	for i, id := range prompt {
		if id < 0 || id >= vocab.Size() {
			return Generation{}, fmt.Errorf("prompt id %d at position %d is outside vocabulary size %d", id, i, vocab.Size())
		}
	}

	state := append([]int(nil), prompt...)
	result := Generation{
		PromptIDs: append([]int(nil), prompt...),
		IDs:       make([]int, 0, c.MaxTokens),
		Stop:      "max_tokens",
		VocabHash: vocab.Hash(),
	}
	decoder := tokenizer.NewStreamDecoder(vocab)
	eosID, hasEOS := vocab.Special("eos")
	rng := rand.New(rand.NewPCG(c.Seed, 0))

	for len(result.IDs) < c.MaxTokens {
		if err := ctx.Err(); err != nil {
			return Generation{}, err
		}
		rows, err := logits(ctx, state)
		if err != nil {
			return Generation{}, fmt.Errorf("get generation logits: %w", err)
		}
		if len(rows) == 0 {
			return Generation{}, fmt.Errorf("logits function returned no rows")
		}
		row := rows[len(rows)-1]
		if len(row) != vocab.Size() {
			return Generation{}, fmt.Errorf("logits row has width %d, want vocabulary size %d", len(row), vocab.Size())
		}
		next, err := sampleNext(row, c, rng)
		if err != nil {
			return Generation{}, err
		}
		result.IDs = append(result.IDs, next)
		state = append(state, next)
		text, err := decoder.Feed(next)
		if err != nil {
			return Generation{}, fmt.Errorf("decode generated id %d: %w", next, err)
		}
		result.Text += text
		if c.StopAtEOS && hasEOS && next == eosID {
			result.Stop = "eos"
			break
		}
	}

	if _, err := decoder.Flush(); err != nil {
		const prefix = "incomplete UTF-8 sequence ("
		const suffix = " bytes)"
		message := err.Error()
		if !strings.HasPrefix(message, prefix) || !strings.HasSuffix(message, suffix) {
			return Generation{}, fmt.Errorf("flush text decoder: %w", err)
		}
		count, parseErr := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(message, prefix), suffix))
		if parseErr != nil || count < 0 {
			return Generation{}, fmt.Errorf("flush text decoder: %w", err)
		}
		result.IncompleteUTF8Bytes = count
	}
	return result, nil
}

// sampleNext chooses one id with temperature, top-k, and top-p filtering.
func sampleNext(logits []float64, c GenerateConfig, rng *rand.Rand) (int, error) {
	if math.IsNaN(c.Temperature) || math.IsInf(c.Temperature, 0) || c.Temperature < 0 {
		return 0, fmt.Errorf("temperature must be finite and non-negative")
	}
	if c.TopK < 0 {
		return 0, fmt.Errorf("top_k must be non-negative")
	}
	if math.IsNaN(c.TopP) || math.IsInf(c.TopP, 0) || c.TopP <= 0 || c.TopP > 1 {
		return 0, fmt.Errorf("top_p must be in (0, 1]")
	}
	if len(logits) == 0 {
		return 0, fmt.Errorf("logits must not be empty")
	}
	best := 0
	for id, value := range logits {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, fmt.Errorf("logit at id %d is not finite", id)
		}
		if value > logits[best] {
			best = id
		}
	}
	if c.Temperature == 0 {
		return best, nil
	}
	if rng == nil {
		return 0, fmt.Errorf("random source must not be nil")
	}

	maxLogit := logits[best]
	probabilities := make([]float64, len(logits))
	var total float64
	for id, value := range logits {
		probabilities[id] = math.Exp((value - maxLogit) / c.Temperature)
		total += probabilities[id]
	}
	if !finite(total) || total <= 0 {
		return 0, fmt.Errorf("invalid probability normalization")
	}
	for id := range probabilities {
		probabilities[id] /= total
	}

	order := make([]int, len(logits))
	for id := range order {
		order[id] = id
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := order[i], order[j]
		if logits[a] == logits[b] {
			return a < b
		}
		return logits[a] > logits[b]
	})
	if c.TopK > 0 && c.TopK < len(order) {
		order = order[:c.TopK]
	}
	if c.TopP < 1 {
		var mass float64
		prefix := 0
		for _, id := range order {
			mass += probabilities[id]
			prefix++
			if mass >= c.TopP {
				break
			}
		}
		order = order[:prefix]
	}

	kept := make([]bool, len(logits))
	var keptMass float64
	for _, id := range order {
		kept[id] = true
		keptMass += probabilities[id]
	}
	if !finite(keptMass) || keptMass <= 0 {
		return 0, fmt.Errorf("invalid filtered probability normalization")
	}
	draw := rng.Float64()
	var cumulative float64
	last := order[0]
	for id, probability := range probabilities {
		if kept[id] {
			last = id
			cumulative += probability / keptMass
			if draw < cumulative {
				return id, nil
			}
		}
	}
	return last, nil
}
