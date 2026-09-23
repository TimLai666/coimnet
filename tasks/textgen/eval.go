package textgen

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/textgen/tokenizer"
)

// Examples turns a document into training examples: ids = [bos] + the document's bytes + [eos] (the vocabulary must
// declare specials named "bos" and "eos"), cut into consecutive windows of at most maxTokens ids (maxTokens 2..4096);
// in every window position 0 is unscored and every other position is scored.
func Examples(v tokenizer.Vocabulary, doc Document, maxTokens int) ([]Example, error) {
	if v == nil {
		return nil, fmt.Errorf("vocabulary must not be nil")
	}
	if maxTokens < 2 || maxTokens > 4096 {
		return nil, fmt.Errorf("maxTokens must be in [2, 4096]")
	}
	if !utf8.ValidString(doc.Text) {
		return nil, fmt.Errorf("document %q text is not valid UTF-8", doc.ID)
	}
	bos, hasBOS := v.Special("bos")
	if !hasBOS {
		return nil, fmt.Errorf("vocabulary must declare special token \"bos\"")
	}
	eos, hasEOS := v.Special("eos")
	if !hasEOS {
		return nil, fmt.Errorf("vocabulary must declare special token \"eos\"")
	}
	if bos < 0 || bos >= v.Size() || eos < 0 || eos >= v.Size() {
		return nil, fmt.Errorf("vocabulary special token id is outside vocabulary size %d", v.Size())
	}
	encoded, err := v.Encode(doc.Text)
	if err != nil {
		return nil, fmt.Errorf("encode document %q: %w", doc.ID, err)
	}
	ids := make([]int, 0, len(encoded)+2)
	ids = append(ids, bos)
	for i, id := range encoded {
		if id < 0 || id >= v.Size() {
			return nil, fmt.Errorf("vocabulary encoded document %q byte %d as out-of-range id %d", doc.ID, i, id)
		}
		ids = append(ids, id)
	}
	ids = append(ids, eos)

	count := (len(ids) + maxTokens - 1) / maxTokens
	examples := make([]Example, 0, count)
	for start := 0; start < len(ids); start += maxTokens {
		end := start + maxTokens
		if end > len(ids) {
			end = len(ids)
		}
		windowIDs := append([]int(nil), ids[start:end]...)
		scored := make([]bool, len(windowIDs))
		for i := 1; i < len(scored); i++ {
			scored[i] = true
		}
		examples = append(examples, Example{IDs: windowIDs, Scored: scored})
	}
	return examples, nil
}

// HoldoutNLL is the NLL of the test documents: the scored-position sum of m.NLL over Examples(doc, maxTokens) for every
// doc, the count, the mean per token and Perplexity = exp(mean); VocabHash names the tokenizer so perplexities from
// different tokenizers are never ranked against each other.
type HoldoutNLL struct {
	Sum        float64 `json:"sum_nats"`
	Tokens     int     `json:"tokens"`
	Mean       float64 `json:"mean_nats_per_token"`
	Perplexity float64 `json:"perplexity"`
	VocabHash  string  `json:"vocab_hash"`
}

// EvaluateHoldout measures the scored-position NLL and perplexity of held-out documents without changing the model.
func EvaluateHoldout(ctx context.Context, m *Model, docs []Document, maxTokens int) (HoldoutNLL, error) {
	if ctx == nil {
		return HoldoutNLL{}, fmt.Errorf("context must not be nil")
	}
	if m == nil || m.vocab == nil {
		return HoldoutNLL{}, fmt.Errorf("model and vocabulary must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return HoldoutNLL{}, err
	}
	result := HoldoutNLL{VocabHash: m.vocab.Hash()}
	for i, doc := range docs {
		if err := ctx.Err(); err != nil {
			return HoldoutNLL{}, err
		}
		examples, err := Examples(m.vocab, doc, maxTokens)
		if err != nil {
			return HoldoutNLL{}, fmt.Errorf("build holdout examples for document %d: %w", i, err)
		}
		for _, ex := range examples {
			tokens := 0
			for _, scored := range ex.Scored {
				if scored {
					tokens++
				}
			}
			if tokens == 0 {
				continue
			}
			sum, count, err := m.NLL(ctx, ex)
			if err != nil {
				return HoldoutNLL{}, fmt.Errorf("score holdout document %q: %w", doc.ID, err)
			}
			if count != tokens {
				return HoldoutNLL{}, fmt.Errorf("model scored %d positions, want %d", count, tokens)
			}
			if math.IsNaN(sum) || math.IsInf(sum, 0) || math.IsInf(result.Sum+sum, 0) {
				return HoldoutNLL{}, fmt.Errorf("holdout NLL is not finite")
			}
			if result.Tokens > int(^uint(0)>>1)-count {
				return HoldoutNLL{}, fmt.Errorf("holdout token count overflows int")
			}
			result.Sum += sum
			result.Tokens += count
		}
	}
	if result.Tokens == 0 {
		return HoldoutNLL{}, fmt.Errorf("holdout documents contain no scored tokens")
	}
	result.Mean = result.Sum / float64(result.Tokens)
	result.Perplexity = math.Exp(result.Mean)
	if math.IsNaN(result.Mean) || math.IsInf(result.Mean, 0) || math.IsNaN(result.Perplexity) || math.IsInf(result.Perplexity, 0) {
		return HoldoutNLL{}, fmt.Errorf("holdout mean NLL or perplexity is not finite")
	}
	return result, nil
}

// TaskResult scores grammar completion: for every sentence of every test document (split on "。"), the prompt is [bos] +
// the bytes of subject + verb; Generate runs greedily (Temperature 0, MaxTokens 12, StopAtEOS true, Seed 0) and the
// completion is correct when the decoded text starts with one of the three objects followed by "。". Valid counts the
// generations whose decoded text is complete, valid UTF-8 (validGeneration).
type TaskResult struct {
	Prompts  int      `json:"prompts"`
	Correct  int      `json:"correct"`
	Accuracy float64  `json:"accuracy"`
	Valid    int      `json:"valid"`
	Validity float64  `json:"validity"`
	Samples  []string `json:"samples"`
}

// validGeneration reports whether g decoded to complete, valid UTF-8: the stream decoder kept no bytes back
// (IncompleteUTF8Bytes is 0) and the text holds no byte that cannot start or continue a UTF-8 sequence.
func validGeneration(g Generation) bool {
	return g.IncompleteUTF8Bytes == 0 && utf8.ValidString(g.Text)
}

// EvaluateTask scores greedy completions for subject-verb grammar prompts in test documents.
func EvaluateTask(ctx context.Context, m *Model, docs []Document) (TaskResult, error) {
	if ctx == nil {
		return TaskResult{}, fmt.Errorf("context must not be nil")
	}
	if m == nil || m.vocab == nil {
		return TaskResult{}, fmt.Errorf("model and vocabulary must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return TaskResult{}, err
	}
	bos, hasBOS := m.vocab.Special("bos")
	if !hasBOS {
		return TaskResult{}, fmt.Errorf("vocabulary must declare special token \"bos\"")
	}
	if bos < 0 || bos >= m.vocab.Size() {
		return TaskResult{}, fmt.Errorf("bos id %d is outside vocabulary size %d", bos, m.vocab.Size())
	}
	result := TaskResult{Samples: make([]string, 0, 3)}
	objects := []string{"米", "水", "肉"}
	subjects := []string{"貓", "狗", "鳥"}
	verbs := []string{"吃", "看"}
	for _, doc := range docs {
		if !utf8.ValidString(doc.Text) {
			return TaskResult{}, fmt.Errorf("document %q text is not valid UTF-8", doc.ID)
		}
		for sentenceIndex, sentence := range strings.Split(doc.Text, "。") {
			if sentence == "" {
				continue
			}
			if err := ctx.Err(); err != nil {
				return TaskResult{}, err
			}
			runes := []rune(sentence)
			if len(runes) < 2 {
				return TaskResult{}, fmt.Errorf("document %q sentence %d must contain a subject and verb", doc.ID, sentenceIndex)
			}
			subject, verb := string(runes[0]), string(runes[1])
			if !containsText(subjects, subject) || !containsText(verbs, verb) {
				return TaskResult{}, fmt.Errorf("document %q sentence %d has an unsupported subject or verb", doc.ID, sentenceIndex)
			}
			promptBytes, err := m.vocab.Encode(subject + verb)
			if err != nil {
				return TaskResult{}, fmt.Errorf("encode document %q sentence %d prompt: %w", doc.ID, sentenceIndex, err)
			}
			prompt := make([]int, 1, len(promptBytes)+1)
			prompt[0] = bos
			for _, id := range promptBytes {
				if id < 0 || id >= m.vocab.Size() {
					return TaskResult{}, fmt.Errorf("vocabulary encoded prompt as out-of-range id %d", id)
				}
				prompt = append(prompt, id)
			}
			generation, err := Generate(ctx, m, prompt, GenerateConfig{MaxTokens: 12, Temperature: 0, TopP: 1, StopAtEOS: true, Seed: 0})
			if err != nil {
				return TaskResult{}, fmt.Errorf("generate document %q sentence %d: %w", doc.ID, sentenceIndex, err)
			}
			result.Prompts++
			if validGeneration(generation) {
				result.Valid++
			}
			if len(result.Samples) < 3 {
				result.Samples = append(result.Samples, generation.Text)
			}
			for _, object := range objects {
				if strings.HasPrefix(generation.Text, object+"。") {
					result.Correct++
					break
				}
			}
		}
	}
	if result.Prompts > 0 {
		result.Accuracy = float64(result.Correct) / float64(result.Prompts)
		result.Validity = float64(result.Valid) / float64(result.Prompts)
	}
	return result, nil
}

// CoreDisconnect checks that the output depends on the core: it rebuilds the model from its snapshot with every core
// weight set to 0 (learning.RestoreTrainer on a modified TrainingSnapshot), runs Logits on probe with both, and reports
// whether any logit changed and the largest absolute change.
type CoreDisconnect struct {
	OutputChanged bool    `json:"output_changed"`
	MaxAbsDelta   float64 `json:"max_abs_delta"`
}

// CheckCoreDisconnect checks the logits from m against a restored snapshot whose core weights are all zero.
func CheckCoreDisconnect(ctx context.Context, m *Model, probe []int) (CoreDisconnect, error) {
	if ctx == nil {
		return CoreDisconnect{}, fmt.Errorf("context must not be nil")
	}
	if m == nil || m.trainer == nil || m.vocab == nil {
		return CoreDisconnect{}, fmt.Errorf("model must not be nil")
	}
	if len(probe) == 0 {
		return CoreDisconnect{}, fmt.Errorf("probe must not be empty")
	}
	if err := ctx.Err(); err != nil {
		return CoreDisconnect{}, err
	}
	original, err := m.Logits(ctx, probe)
	if err != nil {
		return CoreDisconnect{}, fmt.Errorf("get connected model logits: %w", err)
	}
	snapshot := m.Snapshot()
	for i := range snapshot.Parameters.Core.Weights {
		snapshot.Parameters.Core.Weights[i] = 0
	}
	trainer, err := learning.RestoreTrainer(snapshot)
	if err != nil {
		return CoreDisconnect{}, fmt.Errorf("restore model with disconnected core: %w", err)
	}
	modified := &Model{vocab: m.vocab, vocabSize: m.vocabSize, config: m.config, trainer: trainer}
	disconnected, err := modified.Logits(ctx, probe)
	if err != nil {
		return CoreDisconnect{}, fmt.Errorf("get disconnected model logits: %w", err)
	}
	if len(original) != len(disconnected) {
		return CoreDisconnect{}, fmt.Errorf("logit row count changed from %d to %d", len(original), len(disconnected))
	}
	result := CoreDisconnect{}
	for row := range original {
		if len(original[row]) != len(disconnected[row]) {
			return CoreDisconnect{}, fmt.Errorf("logit row %d width changed from %d to %d", row, len(original[row]), len(disconnected[row]))
		}
		for column := range original[row] {
			delta := math.Abs(original[row][column] - disconnected[row][column])
			if math.IsNaN(delta) || math.IsInf(delta, 0) {
				return CoreDisconnect{}, fmt.Errorf("logit delta at row %d column %d is not finite", row, column)
			}
			if delta != 0 {
				result.OutputChanged = true
			}
			if delta > result.MaxAbsDelta {
				result.MaxAbsDelta = delta
			}
		}
	}
	return result, nil
}

// containsText reports whether value exactly matches one of the allowed grammar strings.
func containsText(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
