package textgen

import (
	"context"
	"math"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/tasks/textgen/tokenizer"
)

func generationTestVocabulary(t *testing.T) tokenizer.Vocabulary {
	t.Helper()
	vocab, err := tokenizer.New(tokenizer.Config{
		Format:   tokenizer.FormatByte,
		Specials: []tokenizer.SpecialToken{{Name: "eos", Role: "eos"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return vocab
}

func generationTestConfig() GenerateConfig {
	return GenerateConfig{MaxTokens: 4, Temperature: 0, TopP: 1, Seed: 7, StopAtEOS: true}
}

func TestSampleNext(t *testing.T) {
	t.Run("greedy and ties", func(t *testing.T) {
		got, err := sampleNext([]float64{3, 3, 1}, GenerateConfig{Temperature: 0, TopP: 1}, rand.New(rand.NewPCG(1, 0)))
		if err != nil || got != 0 {
			t.Fatalf("sampleNext() = %d, %v; want 0, nil", got, err)
		}
	})
	t.Run("top k one", func(t *testing.T) {
		for _, temperature := range []float64{0, 0.7, 100} {
			got, err := sampleNext([]float64{-1, 4, 3}, GenerateConfig{Temperature: temperature, TopK: 1, TopP: 1}, rand.New(rand.NewPCG(1, 0)))
			if err != nil || got != 1 {
				t.Fatalf("temperature %g: sampleNext() = %d, %v; want 1, nil", temperature, got, err)
			}
		}
	})
	t.Run("tiny top p", func(t *testing.T) {
		got, err := sampleNext([]float64{10, 9, 8}, GenerateConfig{Temperature: 1, TopP: 1e-9}, rand.New(rand.NewPCG(1, 0)))
		if err != nil || got != 0 {
			t.Fatalf("sampleNext() = %d, %v; want 0, nil", got, err)
		}
	})
	t.Run("filters exclude removed ids", func(t *testing.T) {
		for name, config := range map[string]GenerateConfig{
			"top_k": {Temperature: 1, TopK: 2, TopP: 1},
			"top_p": {Temperature: 1, TopP: 0.5},
		} {
			rng := rand.New(rand.NewPCG(7, 0))
			for i := 0; i < 10000; i++ {
				got, err := sampleNext([]float64{3, 2, 1, 0}, config, rng)
				if err != nil {
					t.Fatal(err)
				}
				if (name == "top_k" && got >= 2) || (name == "top_p" && got != 0) {
					t.Fatalf("%s returned filtered id %d", name, got)
				}
			}
		}
	})
	t.Run("reproducible rng", func(t *testing.T) {
		config := GenerateConfig{Temperature: 0.9, TopK: 3, TopP: 0.8}
		first := rand.New(rand.NewPCG(7, 0))
		second := rand.New(rand.NewPCG(7, 0))
		for i := 0; i < 20; i++ {
			a, errA := sampleNext([]float64{0.2, 1.1, -0.4, 0.7}, config, first)
			b, errB := sampleNext([]float64{0.2, 1.1, -0.4, 0.7}, config, second)
			if errA != nil || errB != nil || a != b {
				t.Fatalf("draw %d = (%d, %v), (%d, %v)", i, a, errA, b, errB)
			}
		}
	})
	t.Run("rejects non-finite logits", func(t *testing.T) {
		for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			if _, err := sampleNext([]float64{0, value}, GenerateConfig{Temperature: 1, TopP: 1}, rand.New(rand.NewPCG(1, 0))); err == nil {
				t.Fatalf("sampleNext accepted logit %v", value)
			}
		}
	})
}

func TestGenerateWithScriptedLogits(t *testing.T) {
	vocab := generationTestVocabulary(t)
	eos, ok := vocab.Special("eos")
	if !ok {
		t.Fatal("missing eos special")
	}
	cat := []int{0xe8, 0xb2, 0x93}

	t.Run("emits complete utf8 and stops at eos", func(t *testing.T) {
		result, err := generateWithScript(vocab, []int{1}, append(append([]int(nil), cat...), eos), generationTestConfig())
		if err != nil {
			t.Fatal(err)
		}
		if result.Text != "貓" || result.Stop != "eos" || !reflect.DeepEqual(result.IDs, append(append([]int(nil), cat...), eos)) || result.IncompleteUTF8Bytes != 0 {
			t.Fatalf("unexpected generation: %+v", result)
		}
	})
	t.Run("counts buffered utf8 at token limit", func(t *testing.T) {
		config := generationTestConfig()
		config.MaxTokens = 2
		result, err := generateWithScript(vocab, []int{1}, cat, config)
		if err != nil {
			t.Fatal(err)
		}
		if result.Text != "" || result.Stop != "max_tokens" || result.IncompleteUTF8Bytes != 2 {
			t.Fatalf("unexpected generation: %+v", result)
		}
	})
	t.Run("continues after eos when disabled", func(t *testing.T) {
		config := generationTestConfig()
		config.StopAtEOS = false
		config.MaxTokens = 3
		result, err := generateWithScript(vocab, []int{1}, []int{eos, 'x', 'y'}, config)
		if err != nil {
			t.Fatal(err)
		}
		if result.Text != "xy" || result.Stop != "max_tokens" || !reflect.DeepEqual(result.IDs, []int{eos, 'x', 'y'}) {
			t.Fatalf("unexpected generation: %+v", result)
		}
	})
}

func generateWithScript(vocab tokenizer.Vocabulary, prompt, script []int, config GenerateConfig) (Generation, error) {
	step := 0
	return generateWith(context.Background(), vocab, func(_ context.Context, ids []int) ([][]float64, error) {
		rows := make([][]float64, len(ids))
		for i := range rows {
			rows[i] = make([]float64, vocab.Size())
		}
		if step >= len(script) {
			return nil, context.DeadlineExceeded
		}
		rows[len(rows)-1][script[step]] = 10
		step++
		return rows, nil
	}, prompt, config)
}

func TestGenerateIsReproducible(t *testing.T) {
	vocab := generationTestVocabulary(t)
	model, err := NewModel(vocab, DefaultModelConfig())
	if err != nil {
		t.Fatal(err)
	}
	config := GenerateConfig{MaxTokens: 6, Temperature: 1, TopK: 5, TopP: 1, Seed: 77, StopAtEOS: true}
	before := model.Snapshot()
	first, err := Generate(context.Background(), model, []int{'h', 'i'}, config)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Generate(context.Background(), model, []int{'h', 'i'}, config)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("generation differs for same seed: first=%+v second=%+v", first, second)
	}
	if after := model.Snapshot(); !reflect.DeepEqual(before, after) {
		t.Fatal("Generate changed model snapshot")
	}
}

func TestGenerateConfigValidate(t *testing.T) {
	base := generationTestConfig()
	tests := []struct {
		name  string
		field string
		edit  func(*GenerateConfig)
	}{
		{name: "max_tokens low", field: "max_tokens", edit: func(c *GenerateConfig) { c.MaxTokens = 0 }},
		{name: "max_tokens high", field: "max_tokens", edit: func(c *GenerateConfig) { c.MaxTokens = 4097 }},
		{name: "temperature negative", field: "temperature", edit: func(c *GenerateConfig) { c.Temperature = -1 }},
		{name: "temperature nan", field: "temperature", edit: func(c *GenerateConfig) { c.Temperature = math.NaN() }},
		{name: "temperature infinite", field: "temperature", edit: func(c *GenerateConfig) { c.Temperature = math.Inf(1) }},
		{name: "top_k negative", field: "top_k", edit: func(c *GenerateConfig) { c.TopK = -1 }},
		{name: "top_p zero", field: "top_p", edit: func(c *GenerateConfig) { c.TopP = 0 }},
		{name: "top_p high", field: "top_p", edit: func(c *GenerateConfig) { c.TopP = 1.01 }},
		{name: "top_p nan", field: "top_p", edit: func(c *GenerateConfig) { c.TopP = math.NaN() }},
		{name: "top_p infinite", field: "top_p", edit: func(c *GenerateConfig) { c.TopP = math.Inf(1) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := base
			tt.edit(&config)
			err := config.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.field) {
				t.Fatalf("Validate() error = %v, want field %q", err, tt.field)
			}
		})
	}
}

func TestGenerateRejectsBadPrompt(t *testing.T) {
	vocab := generationTestVocabulary(t)
	logits := func(context.Context, []int) ([][]float64, error) {
		t.Fatal("logits called for invalid prompt")
		return nil, nil
	}
	for _, prompt := range [][]int{nil, {}, {-1}, {vocab.Size()}} {
		if _, err := generateWith(context.Background(), vocab, logits, prompt, generationTestConfig()); err == nil {
			t.Fatalf("generateWith accepted prompt %v", prompt)
		}
	}
}
