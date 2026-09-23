package textgen

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/tasks/textgen/tokenizer"
)

func TestExamples(t *testing.T) {
	vocab := testVocabulary(t)
	short, err := Examples(vocab, Document{Text: "貓"}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(short) != 1 {
		t.Fatalf("short document produced %d windows, want 1", len(short))
	}
	assertWindowScores(t, short)

	doc := Document{Text: "abcdefgh"}
	windows, err := Examples(vocab, doc, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(windows) < 2 {
		t.Fatalf("long document produced %d windows, want multiple", len(windows))
	}
	var joined []int
	for _, window := range windows {
		if len(window.IDs) > 5 {
			t.Fatalf("window has %d ids, maximum is 5", len(window.IDs))
		}
		joined = append(joined, window.IDs...)
	}
	textIDs, err := vocab.Encode(doc.Text)
	if err != nil {
		t.Fatal(err)
	}
	bos, _ := vocab.Special("bos")
	eos, _ := vocab.Special("eos")
	want := append([]int{bos}, textIDs...)
	want = append(want, eos)
	if !reflect.DeepEqual(joined, want) {
		t.Fatalf("joined windows = %v, want %v", joined, want)
	}
	assertWindowScores(t, windows)

	withoutBOS, err := tokenizer.New(tokenizer.Config{Format: tokenizer.FormatByte, Specials: []tokenizer.SpecialToken{{Name: "eos", Role: "eos"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Examples(withoutBOS, Document{Text: "x"}, 4); err == nil {
		t.Fatal("Examples accepted a vocabulary without bos")
	}
	withoutEOS, err := tokenizer.New(tokenizer.Config{Format: tokenizer.FormatByte, Specials: []tokenizer.SpecialToken{{Name: "bos", Role: "bos"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Examples(withoutEOS, Document{Text: "x"}, 4); err == nil {
		t.Fatal("Examples accepted a vocabulary without eos")
	}
	if _, err := Examples(vocab, Document{Text: "x"}, 1); err == nil {
		t.Fatal("Examples accepted maxTokens 1")
	}
}

func assertWindowScores(t *testing.T, examples []Example) {
	t.Helper()
	for i, ex := range examples {
		if len(ex.IDs) != len(ex.Scored) || len(ex.IDs) == 0 {
			t.Fatalf("window %d has mismatched or empty IDs and scores: %+v", i, ex)
		}
		if ex.Scored[0] {
			t.Fatalf("window %d scores its first position", i)
		}
		for position := 1; position < len(ex.Scored); position++ {
			if !ex.Scored[position] {
				t.Fatalf("window %d does not score position %d", i, position)
			}
		}
	}
}

func TestEvaluateHoldout(t *testing.T) {
	vocab := testVocabulary(t)
	m, err := NewModel(vocab, DefaultModelConfig())
	if err != nil {
		t.Fatal(err)
	}
	docs := []Document{{Text: "貓吃米。"}, {Text: "狗看水。"}}
	before := m.Snapshot()
	result, err := EvaluateHoldout(context.Background(), m, docs, 7)
	if err != nil {
		t.Fatal(err)
	}
	if result.Tokens == 0 || !closeEnough(result.Perplexity, math.Exp(result.Mean)) {
		t.Fatalf("holdout result has invalid token count or perplexity: %+v", result)
	}
	if result.VocabHash != vocab.Hash() {
		t.Fatalf("vocab hash = %q, want %q", result.VocabHash, vocab.Hash())
	}
	wantTokens := 0
	for _, doc := range docs {
		examples, err := Examples(vocab, doc, 7)
		if err != nil {
			t.Fatal(err)
		}
		for _, ex := range examples {
			for _, scored := range ex.Scored {
				if scored {
					wantTokens++
				}
			}
		}
	}
	if result.Tokens != wantTokens {
		t.Fatalf("scored tokens = %d, want %d", result.Tokens, wantTokens)
	}
	if !reflect.DeepEqual(before, m.Snapshot()) {
		t.Fatal("EvaluateHoldout changed the model snapshot")
	}
}

func TestEvaluateTaskAndTraining(t *testing.T) {
	ctx := context.Background()
	vocab := testVocabulary(t)
	corpus, err := FixtureCorpus(1, 12, 4)
	if err != nil {
		t.Fatal(err)
	}
	trainDocs, testDocs := corpus.Documents[:9], corpus.Documents[9:]
	m, err := NewModel(vocab, DefaultModelConfig())
	if err != nil {
		t.Fatal(err)
	}
	beforeTask, err := EvaluateTask(ctx, m, testDocs)
	if err != nil {
		t.Fatal(err)
	}
	beforeNLL, err := EvaluateHoldout(ctx, m, testDocs, 64)
	if err != nil {
		t.Fatal(err)
	}
	for epoch := 0; epoch < 3; epoch++ {
		for _, doc := range trainDocs {
			examples, err := Examples(vocab, doc, 64)
			if err != nil {
				t.Fatal(err)
			}
			for _, ex := range examples {
				if _, _, err := m.Step(ctx, ex); err != nil {
					t.Fatalf("epoch %d step: %v", epoch, err)
				}
			}
		}
	}
	afterTask, err := EvaluateTask(ctx, m, testDocs)
	if err != nil {
		t.Fatal(err)
	}
	afterNLL, err := EvaluateHoldout(ctx, m, testDocs, 64)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("before perplexity=%.6f task_accuracy=%.6f validity=%.6f", beforeNLL.Perplexity, beforeTask.Accuracy, beforeTask.Validity)
	t.Logf("after perplexity=%.6f task_accuracy=%.6f validity=%.6f", afterNLL.Perplexity, afterTask.Accuracy, afterTask.Validity)
	if !(afterNLL.Perplexity < beforeNLL.Perplexity) {
		t.Fatalf("perplexity did not decrease: before=%g after=%g", beforeNLL.Perplexity, afterNLL.Perplexity)
	}
	for _, result := range []TaskResult{beforeTask, afterTask} {
		if result.Prompts != 9 || result.Valid > result.Prompts || result.Correct > result.Prompts {
			t.Fatalf("invalid task result: %+v", result)
		}
		if !closeEnough(result.Accuracy, float64(result.Correct)/float64(result.Prompts)) || !closeEnough(result.Validity, float64(result.Valid)/float64(result.Prompts)) {
			t.Fatalf("task rates do not match counts: %+v", result)
		}
		if len(result.Samples) > 3 {
			t.Fatalf("got %d samples, want at most 3", len(result.Samples))
		}
	}
}

func TestCheckCoreDisconnect(t *testing.T) {
	ctx := context.Background()
	vocab := testVocabulary(t)
	m, err := NewModel(vocab, DefaultModelConfig())
	if err != nil {
		t.Fatal(err)
	}
	ids, err := Examples(vocab, Document{Text: "貓吃米。"}, 64)
	if err != nil {
		t.Fatal(err)
	}
	for _, ex := range ids {
		if _, _, err := m.Step(ctx, ex); err != nil {
			t.Fatal(err)
		}
	}
	before := m.Snapshot()
	bos, _ := vocab.Special("bos")
	textIDs, err := vocab.Encode("貓吃")
	if err != nil {
		t.Fatal(err)
	}
	probe := append([]int{bos}, textIDs...)
	result, err := CheckCoreDisconnect(ctx, m, probe)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OutputChanged || !(result.MaxAbsDelta > 0) {
		t.Fatalf("core disconnect result = %+v, want changed outputs and positive delta", result)
	}
	if !reflect.DeepEqual(before, m.Snapshot()) {
		t.Fatal("CheckCoreDisconnect changed the original model snapshot")
	}
}

func closeEnough(a, b float64) bool {
	return math.Abs(a-b) <= 1e-12*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}

func TestValidGeneration(t *testing.T) {
	tests := []struct {
		name string
		gen  Generation
		want bool
	}{
		{name: "valid UTF-8", gen: Generation{Text: "米。"}, want: true},
		{name: "invalid UTF-8 byte", gen: Generation{Text: "米\xff。"}, want: false},
		{name: "incomplete UTF-8 bytes", gen: Generation{Text: "米", IncompleteUTF8Bytes: 2}, want: false},
		{name: "empty text", gen: Generation{}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validGeneration(test.gen); got != test.want {
				t.Errorf("validGeneration(%+v) = %t, want %t", test.gen, got, test.want)
			}
		})
	}
}
