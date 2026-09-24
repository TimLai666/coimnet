// Command realtext imports one user-provided real textgen manifest, performs a
// bounded source-held-out training run, and prints one auditable JSON report.
// It does not download data or persist model parameters.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	osSignal "os/signal"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/TimLai666/coimnet/tasks/textgen"
	"github.com/TimLai666/coimnet/tasks/textgen/tokenizer"
)

const (
	realTextSchemaVersion = "coimnet-real-text-example/v1"
	realTextTestFraction  = 0.34
	realTextSplitSeed     = uint64(7)
	realTextModelSeed     = uint64(7)
	realTextWindowBytes   = 8192
	realTextAccuracyBytes = 1024
	realTextMaxTokens     = 128
	realTextEpochs        = 3
	realTextGeneration    = 96
)

type realTextReport struct {
	SchemaVersion        string                `json:"schema_version"`
	Input                realTextInput         `json:"input"`
	Runtime              realTextRuntime       `json:"runtime"`
	Scope                textgen.DataScope     `json:"scope"`
	Documents            []realTextDocument    `json:"documents"`
	Split                realTextSplit         `json:"split"`
	Training             realTextTraining      `json:"training"`
	Metrics              realTextMetricSummary `json:"metrics"`
	IndependentInference realTextInference     `json:"independent_inference"`
	ScopeLimitations     []string              `json:"scope_limitations"`
}

type realTextInput struct {
	ManifestPath   string `json:"manifest_path"`
	ManifestSHA256 string `json:"manifest_sha256"`
}

type realTextRuntime struct {
	GoVersion string `json:"go_version"`
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
	NumCPU    int    `json:"num_cpu"`
}

type realTextDocument struct {
	ID              string `json:"id"`
	Source          string `json:"source"`
	RawBytes        int    `json:"raw_bytes"`
	RawSHA256       string `json:"raw_sha256"`
	BodyBytes       int    `json:"body_bytes"`
	SelectedBytes   int    `json:"selected_bytes"`
	SelectionAnchor string `json:"selection_anchor"`
	SelectionOffset int    `json:"selection_offset_bytes"`
	BodyStartMarker string `json:"body_start_marker,omitempty"`
	BodyEndMarker   string `json:"body_end_marker,omitempty"`
}

type realTextSplit struct {
	TrainDocuments int      `json:"train_documents"`
	TestDocuments  int      `json:"test_documents"`
	TestFraction   float64  `json:"test_fraction"`
	Seed           uint64   `json:"seed"`
	TrainSources   []string `json:"train_sources"`
	TestSources    []string `json:"test_sources"`
	TrainIDs       []string `json:"train_ids"`
	TestIDs        []string `json:"test_ids"`
}

type realTextTraining struct {
	ModelConfig                   textgen.ModelConfig `json:"model_config"`
	ModelSeed                     uint64              `json:"model_seed"`
	Epochs                        int                 `json:"epochs"`
	MaxTokens                     int                 `json:"max_tokens"`
	SelectedBytesPerTrainDocument int                 `json:"selected_bytes_per_train_document"`
	TrainingDocuments             int                 `json:"training_documents"`
	ExamplesPerDocument           []int               `json:"examples_per_document"`
	Updates                       int                 `json:"updates"`
	VocabHash                     string              `json:"vocab_hash"`
}

type realTextMetricSummary struct {
	HoldoutBytes  int             `json:"holdout_bytes"`
	AccuracyBytes int             `json:"next_token_accuracy_bytes"`
	Before        realTextMetrics `json:"before"`
	After         realTextMetrics `json:"after"`
}

type realTextMetrics struct {
	Holdout           textgen.HoldoutNLL `json:"holdout"`
	NextTokenAccuracy float64            `json:"next_token_accuracy"`
	NextTokenCorrect  int                `json:"next_token_correct"`
	NextTokenTotal    int                `json:"next_token_total"`
}

type realTextInference struct {
	Prompt string                   `json:"prompt"`
	Config textgen.GenerateConfig   `json:"config"`
	Before realTextGenerationResult `json:"before"`
	After  realTextGenerationResult `json:"after"`
}

type realTextGenerationResult struct {
	Text                string `json:"text"`
	Stop                string `json:"stop"`
	IncompleteUTF8Bytes int    `json:"incomplete_utf8_bytes"`
	VocabHash           string `json:"vocab_hash"`
}

func main() {
	ctx, stop := osSignal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run parses the standalone example's arguments and writes one JSON report to
// stdout. Errors are returned to main and never produce a partial report.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if ctx == nil || stdout == nil || stderr == nil {
		return errors.New("context and output writers are required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	fs := flag.NewFlagSet("realtext", flag.ContinueOnError)
	diagnosticOutput := &errorTrackingWriter{Writer: stderr}
	usageOutput := &errorTrackingWriter{Writer: stdout}
	fs.SetOutput(diagnosticOutput)
	var manifestPath string
	fs.StringVar(&manifestPath, "manifest", "", "strict coimnet-textgen-corpus/v1 manifest to import")
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: go run ./examples/realtext --manifest PATH")
		fmt.Fprintln(usageOutput, "Imports the supplied real English text manifest, performs a source split, runs one bounded training pass, and prints before/after holdout metrics plus independent inference. No data is downloaded and no model is saved.")
		fmt.Fprintln(usageOutput, "Fixed settings: source split fraction 0.34 with seed 7; 8 KiB selected window per document; byte vocabulary; model embed/hidden/settle 16/32/3; learning rate 0.01; model seed 7; 3 epochs; max tokens 128; inference max tokens 96 with seed 19.")
		fmt.Fprintln(usageOutput, "Report: ReadCorpus manifest SHA-256, parsed document SHA-256 values, source split, holdout NLL/perplexity, next-token accuracy, vocabulary hash, and an independent generation result.")
		fmt.Fprintln(usageOutput, "Limitations: this is a bounded acceptance run, not full-corpus training or a language-quality evaluation. No GPU or accelerator claim is made from backend logs.")
		fmt.Fprintln(usageOutput, "Errors: missing or malformed manifest, non-real scope, invalid source split, cancellation, numerical failure, or output failure.")
		fmt.Fprintln(usageOutput, "Options:")
		fs.SetOutput(usageOutput)
		fs.PrintDefaults()
		fs.SetOutput(diagnosticOutput)
	}

	if err := fs.Parse(args); err != nil {
		if diagnosticErr := diagnosticOutput.Err(); diagnosticErr != nil {
			return errors.Join(err, fmt.Errorf("write flag diagnostic: %w", diagnosticErr))
		}
		if usageErr := usageOutput.Err(); usageErr != nil {
			return fmt.Errorf("write usage: %w", usageErr)
		}
		if errors.Is(err, flag.ErrHelp) && fs.NArg() == 0 {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if manifestPath == "" {
		return errors.New("--manifest is required; use --help")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return execute(ctx, manifestPath, stdout)
}

func execute(ctx context.Context, manifestPath string, stdout io.Writer) error {
	corpus, err := textgen.ReadCorpus(ctx, manifestPath)
	if err != nil {
		return fmt.Errorf("import corpus: %w", err)
	}
	if corpus.Scope.Kind != "real" {
		return fmt.Errorf("manifest scope.kind must be real, got %q", corpus.Scope.Kind)
	}
	if !strings.EqualFold(strings.TrimSpace(corpus.Scope.Language), "en") {
		return fmt.Errorf("manifest scope.language must be en, got %q", corpus.Scope.Language)
	}
	train, test, err := textgen.SplitBySource(corpus.Documents, realTextTestFraction, realTextSplitSeed)
	if err != nil {
		return fmt.Errorf("split corpus by source: %w", err)
	}
	if len(train) == 0 || len(test) == 0 {
		return fmt.Errorf("source split produced %d training and %d held-out documents", len(train), len(test))
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
		return fmt.Errorf("create vocabulary: %w", err)
	}
	preparedTrain, trainSummaries, err := prepareDocuments(train, realTextWindowBytes)
	if err != nil {
		return fmt.Errorf("prepare training documents: %w", err)
	}
	preparedTest, testSummaries, err := prepareDocuments(test, realTextWindowBytes)
	if err != nil {
		return fmt.Errorf("prepare held-out documents: %w", err)
	}
	allSummaries := append(append([]realTextDocument(nil), trainSummaries...), testSummaries...)

	modelConfig := textgen.DefaultModelConfig()
	modelConfig.Seed = realTextModelSeed
	model, err := textgen.NewModel(vocab, modelConfig)
	if err != nil {
		return fmt.Errorf("create model: %w", err)
	}
	before, err := measure(ctx, model, vocab, preparedTest, realTextAccuracyBytes)
	if err != nil {
		return fmt.Errorf("measure before training: %w", err)
	}
	prompt := promptText(preparedTest[0].Text, 64)
	generationConfig := textgen.GenerateConfig{
		MaxTokens: realTextGeneration, Temperature: 0.8, TopK: 20, TopP: 0.95, Seed: 19, StopAtEOS: true,
	}
	generatedBefore, err := generate(ctx, model, vocab, prompt, generationConfig)
	if err != nil {
		return fmt.Errorf("generate before training: %w", err)
	}

	examples := make([][]textgen.Example, len(preparedTrain))
	exampleCounts := make([]int, len(preparedTrain))
	updates := 0
	for i, doc := range preparedTrain {
		examples[i], err = textgen.Examples(vocab, doc, realTextMaxTokens)
		if err != nil {
			return fmt.Errorf("build examples for %q: %w", doc.ID, err)
		}
		exampleCounts[i] = len(examples[i])
	}
	for epoch := 0; epoch < realTextEpochs; epoch++ {
		for documentIndex, documentExamples := range examples {
			for exampleIndex, example := range documentExamples {
				if err := ctx.Err(); err != nil {
					return err
				}
				if _, _, err := model.Step(ctx, example); err != nil {
					return fmt.Errorf("train epoch %d document %q example %d: %w", epoch, preparedTrain[documentIndex].ID, exampleIndex, err)
				}
				updates++
			}
		}
	}
	after, err := measure(ctx, model, vocab, preparedTest, realTextAccuracyBytes)
	if err != nil {
		return fmt.Errorf("measure after training: %w", err)
	}
	generatedAfter, err := generate(ctx, model, vocab, prompt, generationConfig)
	if err != nil {
		return fmt.Errorf("generate after training: %w", err)
	}

	report := realTextReport{
		SchemaVersion: realTextSchemaVersion,
		Input:         realTextInput{ManifestPath: manifestPath, ManifestSHA256: corpus.ManifestSHA256},
		Runtime:       realTextRuntime{GoVersion: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, NumCPU: runtime.NumCPU()},
		Scope:         corpus.Scope,
		Documents:     allSummaries,
		Split: realTextSplit{
			TrainDocuments: len(train),
			TestDocuments:  len(test),
			TestFraction:   realTextTestFraction,
			Seed:           realTextSplitSeed,
			TrainSources:   documentSources(train),
			TestSources:    documentSources(test),
			TrainIDs:       documentIDs(train),
			TestIDs:        documentIDs(test),
		},
		Training: realTextTraining{
			ModelConfig:                   modelConfig,
			ModelSeed:                     realTextModelSeed,
			Epochs:                        realTextEpochs,
			MaxTokens:                     realTextMaxTokens,
			SelectedBytesPerTrainDocument: realTextWindowBytes,
			TrainingDocuments:             len(preparedTrain),
			ExamplesPerDocument:           exampleCounts,
			Updates:                       updates,
			VocabHash:                     vocab.Hash(),
		},
		Metrics: realTextMetricSummary{
			HoldoutBytes:  totalSelectedBytes(testSummaries),
			AccuracyBytes: before.NextTokenTotal,
			Before:        before,
			After:         after,
		},
		IndependentInference: realTextInference{
			Prompt: prompt,
			Config: generationConfig,
			Before: generatedBefore,
			After:  generatedAfter,
		},
		ScopeLimitations: []string{
			"This run uses the supplied real corpus and a deterministic 8 KiB window from each document; it is a bounded acceptance run, not full-corpus training.",
			"Perplexity and next-token accuracy are measured on the source-held-out window with the same byte vocabulary hash; they do not establish English fluency, knowledge, convergence, or generalisation.",
			"The generated continuation is an observable sample, not a human or task-quality judgment.",
			"The report does not infer GPU or accelerator training from any backend log; no accelerator claim is made.",
		},
	}

	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

func totalSelectedBytes(documents []realTextDocument) int {
	total := 0
	for _, document := range documents {
		total += document.SelectedBytes
	}
	return total
}

func prepareDocuments(docs []textgen.Document, limit int) ([]textgen.Document, []realTextDocument, error) {
	prepared := make([]textgen.Document, len(docs))
	summaries := make([]realTextDocument, len(docs))
	for i, doc := range docs {
		body, startMarker, endMarker, err := documentBody(doc.Text)
		if err != nil {
			return nil, nil, fmt.Errorf("document %q: %w", doc.ID, err)
		}
		selected, anchor, offset := selectBodyWindow(body, limit)
		prepared[i] = doc
		prepared[i].Text = selected
		summaries[i] = realTextDocument{
			ID:              doc.ID,
			Source:          doc.Source,
			RawBytes:        len(doc.Text),
			RawSHA256:       sha256Hex([]byte(doc.Text)),
			BodyBytes:       len(body),
			SelectedBytes:   len(selected),
			SelectionAnchor: anchor,
			SelectionOffset: offset,
			BodyStartMarker: startMarker,
			BodyEndMarker:   endMarker,
		}
	}
	return prepared, summaries, nil
}

func documentBody(text string) (body, startMarker, endMarker string, err error) {
	start := strings.Index(text, "*** START OF THE PROJECT GUTENBERG EBOOK")
	if start < 0 {
		return text, "", "", nil
	}
	startLineEnd := strings.IndexByte(text[start:], '\n')
	if startLineEnd < 0 {
		return "", "", "", errors.New("Gutenberg START marker has no line ending")
	}
	startLineEnd += start
	end := strings.Index(text[startLineEnd+1:], "*** END OF THE PROJECT GUTENBERG EBOOK")
	if end < 0 {
		return "", "", "", errors.New("Gutenberg END marker not found")
	}
	end += startLineEnd + 1
	if end <= startLineEnd+1 {
		return "", "", "", errors.New("Gutenberg body is empty")
	}
	startMarker = strings.TrimRight(text[start:startLineEnd], "\r\n")
	endLineEnd := strings.IndexByte(text[end:], '\n')
	if endLineEnd < 0 {
		endLineEnd = len(text) - end
	}
	endMarker = strings.TrimRight(text[end:end+endLineEnd], "\r\n")
	return text[startLineEnd+1 : end], startMarker, endMarker, nil
}

func selectBodyWindow(body string, limit int) (string, string, int) {
	if limit <= 0 {
		return "", "body_start_fallback", 0
	}
	anchors := []string{
		"Alice was beginning to get very tired",
		"The Time Traveller (for so it will be convenient to speak of him)",
		"It is a truth universally acknowledged",
	}
	for _, anchor := range anchors {
		if offset := strings.Index(body, anchor); offset >= 0 {
			return truncateUTF8(body[offset:], limit), anchor, offset
		}
	}
	return truncateUTF8(body, limit), "body_start_fallback", 0
}

func truncateUTF8(text string, limit int) string {
	if limit <= 0 || text == "" {
		return ""
	}
	if len(text) <= limit {
		return text
	}
	cut := text[:limit]
	for !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut
}

func measure(ctx context.Context, model *textgen.Model, vocab tokenizer.Vocabulary, docs []textgen.Document, accuracyLimit int) (realTextMetrics, error) {
	holdout, err := textgen.EvaluateHoldout(ctx, model, docs, realTextMaxTokens)
	if err != nil {
		return realTextMetrics{}, err
	}
	if len(docs) == 0 {
		return realTextMetrics{}, errors.New("accuracy documents are empty")
	}
	correct, total, err := nextTokenAccuracy(ctx, model, vocab, docs[0].Text, accuracyLimit)
	if err != nil {
		return realTextMetrics{}, err
	}
	return realTextMetrics{
		Holdout:           holdout,
		NextTokenAccuracy: float64(correct) / float64(total),
		NextTokenCorrect:  correct,
		NextTokenTotal:    total,
	}, nil
}

func nextTokenAccuracy(ctx context.Context, model *textgen.Model, vocab tokenizer.Vocabulary, text string, limit int) (int, int, error) {
	if limit < 1 {
		return 0, 0, errors.New("accuracy limit must be positive")
	}
	encoded, err := vocab.Encode(text)
	if err != nil {
		return 0, 0, err
	}
	if len(encoded) > limit {
		encoded = encoded[:limit]
	}
	if len(encoded) == 0 {
		return 0, 0, errors.New("accuracy window is empty")
	}
	bos, ok := vocab.Special("bos")
	if !ok {
		return 0, 0, errors.New("vocabulary is missing bos")
	}
	input := make([]int, 1, len(encoded))
	input[0] = bos
	input = append(input, encoded[:len(encoded)-1]...)
	rows, err := model.Logits(ctx, input)
	if err != nil {
		return 0, 0, err
	}
	if len(rows) != len(encoded) {
		return 0, 0, fmt.Errorf("got %d logits rows for %d targets", len(rows), len(encoded))
	}
	correct := 0
	for i, row := range rows {
		best := 0
		for id := 1; id < len(row); id++ {
			if row[id] > row[best] {
				best = id
			}
		}
		if best == encoded[i] {
			correct++
		}
	}
	return correct, len(encoded), nil
}

func promptText(body string, limit int) string {
	const knownHoldoutOpening = "It is a truth universally acknowledged"
	if index := strings.Index(body, knownHoldoutOpening); index >= 0 {
		return knownHoldoutOpening
	}
	prompt := strings.TrimSpace(truncateUTF8(body, limit))
	if prompt == "" {
		return "text"
	}
	return prompt
}

func generate(ctx context.Context, model *textgen.Model, vocab tokenizer.Vocabulary, prompt string, config textgen.GenerateConfig) (realTextGenerationResult, error) {
	ids, err := vocab.Encode(prompt)
	if err != nil {
		return realTextGenerationResult{}, err
	}
	bos, ok := vocab.Special("bos")
	if !ok {
		return realTextGenerationResult{}, errors.New("vocabulary is missing bos")
	}
	input := make([]int, 1, len(ids)+1)
	input[0] = bos
	input = append(input, ids...)
	generated, err := textgen.Generate(ctx, model, input, config)
	if err != nil {
		return realTextGenerationResult{}, err
	}
	return realTextGenerationResult{
		Text:                generated.Text,
		Stop:                generated.Stop,
		IncompleteUTF8Bytes: generated.IncompleteUTF8Bytes,
		VocabHash:           generated.VocabHash,
	}, nil
}

func documentSources(docs []textgen.Document) []string {
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

func documentIDs(docs []textgen.Document) []string {
	ids := make([]string, len(docs))
	for i, doc := range docs {
		ids[i] = doc.ID
	}
	return ids
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type errorTrackingWriter struct {
	io.Writer
	err error
}

func (w *errorTrackingWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.Writer.Write(p)
	if err != nil {
		w.err = err
	}
	return n, err
}

func (w *errorTrackingWriter) Err() error {
	return w.err
}
