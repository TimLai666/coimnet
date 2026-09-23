package textgen

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/tasks/textgen/tokenizer"
)

func smallRunConfig() RunConfig {
	return RunConfig{
		Model:        DefaultModelConfig(),
		Documents:    8,
		Sources:      4,
		TestFraction: 0.25,
		TeacherTexts: 2,
		MaxTokens:    32,
		Epochs:       1,
		Seeds:        []uint64{1},
	}
}

func TestRunTextgenReport(t *testing.T) {
	config := smallRunConfig()
	report, err := RunTextgen(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != ReportSchemaVersion {
		t.Fatalf("schema_version = %q, want %q", report.SchemaVersion, ReportSchemaVersion)
	}
	if report.DataScope.Kind != "fixture" {
		t.Fatalf("data scope kind = %q, want fixture", report.DataScope.Kind)
	}
	trainSources := stringSet(report.Split.TrainSources)
	for _, source := range report.Split.TestSources {
		if trainSources[source] {
			t.Errorf("source %q appears in both split sides", source)
		}
	}
	if report.Teacher.Removed < 1 || report.Teacher.Kept+report.Teacher.Removed != report.Teacher.Offered {
		t.Errorf("teacher counts are inconsistent: %+v", report.Teacher)
	}
	if len(report.Runs) != 1 {
		t.Fatalf("got %d seed results, want 1", len(report.Runs))
	}
	if report.VocabHash == "" || report.VocabHash != report.Runs[0].HoldoutBefore.VocabHash {
		t.Fatalf("vocab hashes are missing or differ: report=%q before=%q", report.VocabHash, report.Runs[0].HoldoutBefore.VocabHash)
	}
	logRunMetrics(t, report)
	seed := report.Runs[0]
	if seed.TeacherCalls != 0 {
		t.Errorf("teacher_calls = %d, want 0", seed.TeacherCalls)
	}
	if !seed.CoreDisconnect.OutputChanged {
		t.Errorf("core disconnect did not change output: %+v", seed.CoreDisconnect)
	}
	corpus, err := FixtureCorpus(config.CorpusSeed, config.Documents, config.Sources)
	if err != nil {
		t.Fatal(err)
	}
	train, test, err := SplitBySource(corpus.Documents, config.TestFraction, config.SplitSeed)
	if err != nil {
		t.Fatal(err)
	}
	vocab, err := tokenizer.New(tokenizer.Config{Format: tokenizer.FormatByte, Specials: []tokenizer.SpecialToken{{Name: "bos", Role: "bos"}, {Name: "eos", Role: "eos"}, {Name: "pad", Role: "pad"}}})
	if err != nil {
		t.Fatal(err)
	}
	teacherDocs, _, err := teacherFixtureDocs(config, test)
	if err != nil {
		t.Fatal(err)
	}
	updates := 0
	for _, doc := range append(train, teacherDocs...) {
		examples, err := Examples(vocab, doc, config.MaxTokens)
		if err != nil {
			t.Fatal(err)
		}
		updates += len(examples) * config.Epochs
	}
	if seed.Updates != updates {
		t.Errorf("updates = %d, want %d", seed.Updates, updates)
	}
	wantAssumptions := []string{
		"The fixture corpus is a three-word synthetic grammar; this proves the text pipeline runs, not Chinese dialogue, reasoning or knowledge.",
		"Perplexities are comparable only between runs with the same vocab_hash.",
		"Evaluation runs in student mode: the teacher is blocked and every attempted call is counted.",
		"Teacher texts keep their source and anything overlapping a test document is removed before training.",
	}
	if !reflect.DeepEqual(report.Assumptions, wantAssumptions) {
		t.Errorf("assumptions = %#v, want %#v", report.Assumptions, wantAssumptions)
	}
}

func TestRunTextgenLowersPerplexity(t *testing.T) {
	config := DefaultRunConfig()
	config.Seeds = []uint64{1}
	report, err := RunTextgen(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Runs) != 1 || report.Runs[0].Failed {
		t.Fatalf("seed run failed: %+v", report.Runs)
	}
	seed := report.Runs[0]
	t.Logf("epochs=%d perplexity_before=%.12g perplexity_after=%.12g task_accuracy_after=%.12g", config.Epochs, seed.HoldoutBefore.Perplexity, seed.HoldoutAfter.Perplexity, seed.TaskAfter.Accuracy)
	if !(seed.HoldoutAfter.Perplexity < seed.HoldoutBefore.Perplexity) {
		t.Fatalf("perplexity did not decrease: before=%g after=%g", seed.HoldoutBefore.Perplexity, seed.HoldoutAfter.Perplexity)
	}
}

func TestRunTextgenIsDeterministic(t *testing.T) {
	config := smallRunConfig()
	config.Seeds = []uint64{1, 2}
	first, err := RunTextgen(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RunTextgen(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	logRunMetrics(t, first)
	a, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatal("identical run configurations produced different JSON reports")
	}
}

func TestRunTextgenReadsAManifest(t *testing.T) {
	manifest := `{"schema":"coimnet-textgen-corpus/v1","scope":{"kind":"fixture","language":"zh-Hant","note":"manifest fixture"},"documents":[` +
		manifestDocument("one", "source-a", "貓吃米。") + "," +
		manifestDocument("two", "source-a", "狗看水。") + "," +
		manifestDocument("three", "source-b", "鳥吃肉。") + "," +
		manifestDocument("four", "source-b", "貓看米。") + "]}"
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	config := smallRunConfig()
	config.Corpus = path
	report, err := RunTextgen(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	want := DataScope{Kind: "fixture", Language: "zh-Hant", Note: "manifest fixture"}
	if report.DataScope != want {
		t.Fatalf("data scope = %+v, want %+v", report.DataScope, want)
	}
	if report.Split.TrainDocuments+report.Split.TestDocuments != 4 {
		t.Fatalf("split document counts = %d + %d, want 4", report.Split.TrainDocuments, report.Split.TestDocuments)
	}
}

func TestRunConfigValidate(t *testing.T) {
	tests := []struct {
		name  string
		edit  func(*RunConfig)
		field string
	}{
		{name: "model", edit: func(c *RunConfig) { c.Model.Hidden = 0 }, field: "hidden"},
		{name: "documents", edit: func(c *RunConfig) { c.Documents = 3 }, field: "documents"},
		{name: "sources", edit: func(c *RunConfig) { c.Sources = 1 }, field: "sources"},
		{name: "test_fraction", edit: func(c *RunConfig) { c.TestFraction = 1 }, field: "test_fraction"},
		{name: "teacher_texts", edit: func(c *RunConfig) { c.TeacherTexts = 1001 }, field: "teacher_texts"},
		{name: "max_tokens", edit: func(c *RunConfig) { c.MaxTokens = 1 }, field: "max_tokens"},
		{name: "epochs", edit: func(c *RunConfig) { c.Epochs = 0 }, field: "epochs"},
		{name: "seeds_empty", edit: func(c *RunConfig) { c.Seeds = nil }, field: "seeds"},
		{name: "seeds_duplicate", edit: func(c *RunConfig) { c.Seeds = []uint64{1, 1} }, field: "seeds"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := smallRunConfig()
			test.edit(&config)
			if err := config.Validate(); err == nil || !strings.Contains(err.Error(), test.field) {
				t.Fatalf("Validate() error = %v, want field %q", err, test.field)
			}
		})
	}
	manifestConfig := smallRunConfig()
	manifestConfig.Corpus = "corpus.json"
	manifestConfig.Documents, manifestConfig.Sources = 0, 0
	if err := manifestConfig.Validate(); err != nil {
		t.Fatalf("manifest config should ignore fixture dimensions: %v", err)
	}
}

func TestDefaultRunConfig(t *testing.T) {
	first, second := DefaultRunConfig(), DefaultRunConfig()
	if first.Model != DefaultModelConfig() || first.Corpus != "" || first.CorpusSeed != 1 || first.Documents != 24 || first.Sources != 6 || first.TestFraction != 0.25 || first.SplitSeed != 1 || first.TeacherTexts != 8 || first.MaxTokens != 64 || len(first.Seeds) != 3 || first.Seeds[0] != 1 || first.Seeds[1] != 2 || first.Seeds[2] != 3 {
		t.Fatalf("unexpected default run config: %+v", first)
	}
	if err := first.Validate(); err != nil {
		t.Fatalf("default config is invalid: %v", err)
	}
	first.Seeds[0] = 99
	if second.Seeds[0] != 1 {
		t.Fatal("DefaultRunConfig calls share their Seeds backing array")
	}
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func teacherFixtureDocs(config RunConfig, test []Document) ([]Document, int, error) {
	texts := make([]TeacherText, 0, config.TeacherTexts+1)
	if config.TeacherTexts > 0 {
		corpus, err := FixtureCorpus(config.CorpusSeed+1000, config.TeacherTexts, 2)
		if err != nil {
			return nil, 0, err
		}
		for _, doc := range corpus.Documents {
			texts = append(texts, TeacherText{Text: doc.Text, Source: "offline-teacher"})
		}
		texts = append(texts, TeacherText{Text: test[0].Text, Source: "offline-teacher-duplicate"})
	}
	kept, removed, err := DedupTeacher(texts, test)
	if err != nil {
		return nil, 0, err
	}
	docs := make([]Document, 0, len(kept))
	for i, text := range kept {
		docs = append(docs, Document{ID: "teacher-text-" + string(rune('a'+i)), Source: text.Source, Text: text.Text, License: License{Holder: "CoImNet fixture teacher", Terms: "offline fixture answers", Source: text.Source}})
	}
	return docs, removed, nil
}

func manifestDocument(id, source, text string) string {
	return `{"id":"` + id + `","source":"` + source + `","text":"` + text + `","license":{"holder":"fixture","terms":"offline","source":"test"}}`
}

// logRunMetrics writes the required per-seed before-and-after scores to the test log.
func logRunMetrics(t *testing.T, report RunReport) {
	t.Helper()
	for _, seed := range report.Runs {
		t.Logf("seed=%d perplexity_before=%.12g perplexity_after=%.12g task_accuracy_before=%.12g task_accuracy_after=%.12g teacher_calls=%d", seed.Seed, seed.HoldoutBefore.Perplexity, seed.HoldoutAfter.Perplexity, seed.TaskBefore.Accuracy, seed.TaskAfter.Accuracy, seed.TeacherCalls)
	}
}
