package textgen

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/TimLai666/coimnet/tasks/textgen/tokenizer"
	"github.com/TimLai666/coimnet/teacher"
)

// ReportSchemaVersion identifies the complete text-generation run report format.
const ReportSchemaVersion = "coimnet-textgen/v1"

// RunConfig is one complete text-generation run on the fixture corpus or a user corpus.
type RunConfig struct {
	Model        ModelConfig `json:"model"`
	Corpus       string      `json:"corpus"`
	CorpusSeed   uint64      `json:"corpus_seed"`
	Documents    int         `json:"documents"`
	Sources      int         `json:"sources"`
	TestFraction float64     `json:"test_fraction"`
	SplitSeed    uint64      `json:"split_seed"`
	TeacherTexts int         `json:"teacher_texts"`
	MaxTokens    int         `json:"max_tokens"`
	Epochs       int         `json:"epochs"`
	Seeds        []uint64    `json:"seeds"`
}

// Validate reports the name of each invalid configuration field.
func (c RunConfig) Validate() error {
	if err := c.Model.Validate(); err != nil {
		return fmt.Errorf("model: %w", err)
	}
	if c.Corpus == "" {
		if c.Documents < 4 {
			return fmt.Errorf("documents must be at least 4 for a fixture corpus")
		}
		if c.Sources < 2 || c.Sources > c.Documents {
			return fmt.Errorf("sources must be in [2, documents] for a fixture corpus")
		}
	}
	if !(c.TestFraction > 0 && c.TestFraction < 1) {
		return fmt.Errorf("test_fraction must be strictly between 0 and 1")
	}
	if c.TeacherTexts < 0 || c.TeacherTexts > 1000 {
		return fmt.Errorf("teacher_texts must be in [0, 1000]")
	}
	if c.MaxTokens < 2 || c.MaxTokens > 4096 {
		return fmt.Errorf("max_tokens must be in [2, 4096]")
	}
	if c.Epochs < 1 || c.Epochs > 1000 {
		return fmt.Errorf("epochs must be in [1, 1000]")
	}
	if len(c.Seeds) == 0 {
		return fmt.Errorf("seeds must contain at least one value")
	}
	seen := make(map[uint64]struct{}, len(c.Seeds))
	for _, seed := range c.Seeds {
		if _, exists := seen[seed]; exists {
			return fmt.Errorf("seeds must be distinct; seed %d is repeated", seed)
		}
		seen[seed] = struct{}{}
	}
	return nil
}

// DefaultRunConfig returns the standard fixture run settings. The default Epochs value is the smallest tested count
// whose seed 1 task accuracy after training is above zero. The measured (epochs, perplexity after, task accuracy after)
// triples were (10, 1.45176465376, 1), (20, 1.37366418024, 1), and (40, 1.36535091257, 1).
func DefaultRunConfig() RunConfig {
	return RunConfig{
		Model:        DefaultModelConfig(),
		CorpusSeed:   1,
		Documents:    24,
		Sources:      6,
		TestFraction: 0.25,
		SplitSeed:    1,
		TeacherTexts: 8,
		MaxTokens:    64,
		Epochs:       10,
		Seeds:        []uint64{1, 2, 3},
	}
}

// RunSeed is one seed's result; a failed seed keeps its seed and failure reason.
type RunSeed struct {
	Seed           uint64         `json:"seed"`
	Updates        int            `json:"updates"`
	HoldoutBefore  HoldoutNLL     `json:"holdout_before"`
	HoldoutAfter   HoldoutNLL     `json:"holdout_after"`
	TaskBefore     TaskResult     `json:"task_before"`
	TaskAfter      TaskResult     `json:"task_after"`
	CoreDisconnect CoreDisconnect `json:"core_disconnect"`
	TeacherCalls   int            `json:"teacher_calls"`
	Failed         bool           `json:"failed"`
	Error          string         `json:"error,omitempty"`
}

// SplitReport records document counts and sorted source names on each side of the split.
type SplitReport struct {
	TrainDocuments int      `json:"train_documents"`
	TestDocuments  int      `json:"test_documents"`
	TrainSources   []string `json:"train_sources"`
	TestSources    []string `json:"test_sources"`
}

// TeacherReport records the offered offline texts, removed test overlaps, and kept-text sources.
type TeacherReport struct {
	Offered int      `json:"offered"`
	Removed int      `json:"removed"`
	Kept    int      `json:"kept"`
	Sources []string `json:"sources"`
}

// RunReport contains the reproducible corpus split, teacher-removal result, and per-seed student evaluations.
type RunReport struct {
	SchemaVersion string        `json:"schema_version"`
	Config        RunConfig     `json:"config"`
	ConfigHash    string        `json:"config_hash"`
	VocabHash     string        `json:"vocab_hash"`
	DataScope     DataScope     `json:"data_scope"`
	Split         SplitReport   `json:"split"`
	Teacher       TeacherReport `json:"teacher"`
	Runs          []RunSeed     `json:"runs"`
	Assumptions   []string      `json:"assumptions"`
}

// RunTextgen executes a complete fixture or manifest-backed text-generation experiment and student-mode evaluation.
func RunTextgen(ctx context.Context, c RunConfig) (RunReport, error) {
	if ctx == nil {
		return RunReport{}, fmt.Errorf("context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return RunReport{}, err
	}
	if err := c.Validate(); err != nil {
		return RunReport{}, err
	}

	var corpus Corpus
	var err error
	if c.Corpus == "" {
		corpus, err = FixtureCorpus(c.CorpusSeed, c.Documents, c.Sources)
	} else {
		corpus, err = ReadCorpus(ctx, c.Corpus)
	}
	if err != nil {
		return RunReport{}, fmt.Errorf("load corpus: %w", err)
	}
	train, test, err := SplitBySource(corpus.Documents, c.TestFraction, c.SplitSeed)
	if err != nil {
		return RunReport{}, fmt.Errorf("split corpus by source: %w", err)
	}
	vocab, err := tokenizer.New(tokenizer.Config{
		Format: tokenizer.FormatByte,
		Specials: []tokenizer.SpecialToken{
			{Name: "bos", Role: "bos"},
			{Name: "eos", Role: "eos"},
			{Name: "pad", Role: "pad"},
		},
	})
	if err != nil {
		return RunReport{}, fmt.Errorf("create vocabulary: %w", err)
	}
	teacherTexts, err := makeTeacherTexts(c, test)
	if err != nil {
		return RunReport{}, fmt.Errorf("build offline teacher texts: %w", err)
	}
	keptTeacherTexts, removed, err := DedupTeacher(teacherTexts, test)
	if err != nil {
		return RunReport{}, fmt.Errorf("deduplicate teacher texts: %w", err)
	}
	teacherDocs := teacherDocuments(keptTeacherTexts)
	config := c
	config.Seeds = append([]uint64(nil), c.Seeds...)
	configHash, err := hashRunConfig(config)
	if err != nil {
		return RunReport{}, fmt.Errorf("hash run config: %w", err)
	}
	report := RunReport{
		SchemaVersion: ReportSchemaVersion,
		Config:        config,
		ConfigHash:    configHash,
		VocabHash:     vocab.Hash(),
		DataScope:     corpus.Scope,
		Split: SplitReport{
			TrainDocuments: len(train),
			TestDocuments:  len(test),
			TrainSources:   documentSources(train),
			TestSources:    documentSources(test),
		},
		Teacher: TeacherReport{
			Offered: len(teacherTexts),
			Removed: removed,
			Kept:    len(keptTeacherTexts),
			Sources: teacherSources(keptTeacherTexts),
		},
		Runs:        make([]RunSeed, 0, len(c.Seeds)),
		Assumptions: assumptions(),
	}
	trainingDocs := make([]Document, 0, len(train)+len(teacherDocs))
	trainingDocs = append(trainingDocs, train...)
	trainingDocs = append(trainingDocs, teacherDocs...)
	for _, seed := range c.Seeds {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		result := runTextgenSeed(ctx, vocab, test, trainingDocs, c, seed)
		report.Runs = append(report.Runs, result)
	}
	return report, nil
}

// makeTeacherTexts builds the requested deterministic offline texts and a guaranteed test duplicate.
func makeTeacherTexts(c RunConfig, test []Document) ([]TeacherText, error) {
	texts := make([]TeacherText, 0, c.TeacherTexts+1)
	if c.TeacherTexts == 0 {
		return texts, nil
	}
	corpus, err := FixtureCorpus(c.CorpusSeed+1000, c.TeacherTexts, 2)
	if err != nil {
		return nil, err
	}
	for _, doc := range corpus.Documents {
		texts = append(texts, TeacherText{Text: doc.Text, Source: "offline-teacher"})
	}
	texts = append(texts, TeacherText{Text: test[0].Text, Source: "offline-teacher-duplicate"})
	return texts, nil
}

// teacherDocuments converts kept texts to licensed training documents without losing their sources.
func teacherDocuments(texts []TeacherText) []Document {
	docs := make([]Document, len(texts))
	for i, text := range texts {
		docs[i] = Document{
			ID:     fmt.Sprintf("offline-teacher-%d", i),
			Source: text.Source,
			Text:   text.Text,
			License: License{
				Holder: "CoImNet fixture teacher",
				Terms:  "offline fixture answers",
				Source: text.Source,
			},
		}
	}
	return docs
}

// runTextgenSeed trains and evaluates one independent model, retaining errors on that seed only.
func runTextgenSeed(ctx context.Context, vocab tokenizer.Vocabulary, test, trainingDocs []Document, c RunConfig, seed uint64) (result RunSeed) {
	result.Seed = seed
	blockedTeacher := &teacher.Blocked{ID: "coimnet-textgen-student", Version: ReportSchemaVersion}
	defer func() { result.TeacherCalls = blockedTeacher.Calls() }()
	modelConfig := c.Model
	modelConfig.Seed = seed
	model, err := NewModel(vocab, modelConfig)
	if err != nil {
		return failTextgenSeed(result, err)
	}
	result.HoldoutBefore, err = EvaluateHoldout(ctx, model, test, c.MaxTokens)
	if err != nil {
		return failTextgenSeed(result, fmt.Errorf("evaluate holdout before training: %w", err))
	}
	result.TaskBefore, err = EvaluateTask(ctx, model, test)
	if err != nil {
		return failTextgenSeed(result, fmt.Errorf("evaluate task before training: %w", err))
	}
	examples := make([][]Example, len(trainingDocs))
	for i, doc := range trainingDocs {
		if err := ctx.Err(); err != nil {
			return failTextgenSeed(result, err)
		}
		examples[i], err = Examples(vocab, doc, c.MaxTokens)
		if err != nil {
			return failTextgenSeed(result, fmt.Errorf("build training examples for document %q: %w", doc.ID, err))
		}
	}
	for epoch := 0; epoch < c.Epochs; epoch++ {
		for docIndex, documentExamples := range examples {
			for exampleIndex, example := range documentExamples {
				if err := ctx.Err(); err != nil {
					return failTextgenSeed(result, err)
				}
				if _, _, err := model.Step(ctx, example); err != nil {
					return failTextgenSeed(result, fmt.Errorf("train epoch %d document %q example %d: %w", epoch, trainingDocs[docIndex].ID, exampleIndex, err))
				}
				if result.Updates == int(^uint(0)>>1) {
					return failTextgenSeed(result, fmt.Errorf("updates count overflows int"))
				}
				result.Updates++
			}
		}
	}
	result.HoldoutAfter, err = EvaluateHoldout(ctx, model, test, c.MaxTokens)
	if err != nil {
		return failTextgenSeed(result, fmt.Errorf("evaluate holdout after training: %w", err))
	}
	result.TaskAfter, err = EvaluateTask(ctx, model, test)
	if err != nil {
		return failTextgenSeed(result, fmt.Errorf("evaluate task after training: %w", err))
	}
	probe, err := vocab.Encode(test[0].Text)
	if err != nil {
		return failTextgenSeed(result, fmt.Errorf("encode core-disconnect probe: %w", err))
	}
	if len(probe) == 0 {
		return failTextgenSeed(result, fmt.Errorf("core-disconnect probe is empty"))
	}
	result.CoreDisconnect, err = CheckCoreDisconnect(ctx, model, probe)
	if err != nil {
		return failTextgenSeed(result, fmt.Errorf("check core disconnect: %w", err))
	}
	return result
}

// failTextgenSeed marks one seed failed and preserves the reason in the report.
func failTextgenSeed(result RunSeed, err error) RunSeed {
	result.Failed = true
	result.Error = err.Error()
	return result
}

// documentSources returns distinct source names in lexical order.
func documentSources(docs []Document) []string {
	set := make(map[string]struct{}, len(docs))
	for _, doc := range docs {
		set[doc.Source] = struct{}{}
	}
	sources := make([]string, 0, len(set))
	for source := range set {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	return sources
}

// teacherSources returns distinct teacher sources in lexical order.
func teacherSources(texts []TeacherText) []string {
	set := make(map[string]struct{}, len(texts))
	for _, text := range texts {
		set[text.Source] = struct{}{}
	}
	sources := make([]string, 0, len(set))
	for source := range set {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	return sources
}

// hashRunConfig returns the hex SHA-256 of the JSON-encoded run configuration.
func hashRunConfig(c RunConfig) (string, error) {
	encoded, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(encoded)), nil
}

// assumptions describes the fixture and student-mode limits attached to every report.
func assumptions() []string {
	return []string{
		"The fixture corpus is a three-word synthetic grammar; this proves the text pipeline runs, not Chinese dialogue, reasoning or knowledge.",
		"Perplexities are comparable only between runs with the same vocab_hash.",
		"Evaluation runs in student mode: the teacher is blocked and every attempted call is counted.",
		"Teacher texts keep their source and anything overlapping a test document is removed before training.",
	}
}
