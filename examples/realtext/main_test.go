package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRunHelpDescribesRealTextContract(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("help: %v", err)
	}
	for _, want := range []string{
		"Usage: go run ./examples/realtext --manifest PATH",
		"source split",
		"holdout",
		"independent inference",
		"No data is downloaded",
		"Limitations:",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("help does not contain %q:\n%s", want, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("help wrote stderr: %s", stderr.String())
	}
}

func TestRunImportsSplitsTrainsAndReportsRealText(t *testing.T) {
	manifestPath, manifestBytes := writeTestCorpus(t)
	args := []string{"--manifest", manifestPath}
	var firstOut, firstErr bytes.Buffer
	if err := run(context.Background(), args, &firstOut, &firstErr); err != nil {
		t.Fatalf("run: %v\nstderr=%s", err, firstErr.String())
	}
	if firstErr.Len() != 0 {
		t.Fatalf("run wrote stderr: %s", firstErr.String())
	}

	var report realTextReport
	if err := json.Unmarshal(firstOut.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, firstOut.String())
	}
	wantManifestHash := sha256.Sum256(manifestBytes)
	if report.SchemaVersion != realTextSchemaVersion {
		t.Fatalf("schema_version = %q", report.SchemaVersion)
	}
	if report.Input.ManifestSHA256 != hex.EncodeToString(wantManifestHash[:]) {
		t.Fatalf("manifest_sha256 = %q, want %x", report.Input.ManifestSHA256, wantManifestHash)
	}
	if report.Split.TrainDocuments != 2 || report.Split.TestDocuments != 1 {
		t.Fatalf("split counts = train %d test %d", report.Split.TrainDocuments, report.Split.TestDocuments)
	}
	if len(report.Split.TrainSources) != 2 || len(report.Split.TestSources) != 1 {
		t.Fatalf("split sources = train %#v test %#v", report.Split.TrainSources, report.Split.TestSources)
	}
	if report.Training.TrainingDocuments != 2 || report.Training.Updates == 0 || report.Training.VocabHash == "" {
		t.Fatalf("training = %#v", report.Training)
	}
	if report.Metrics.Before.Holdout.Tokens == 0 || report.Metrics.After.Holdout.Tokens == 0 {
		t.Fatalf("holdout metrics = before %#v after %#v", report.Metrics.Before, report.Metrics.After)
	}
	for name, value := range map[string]float64{
		"before perplexity": report.Metrics.Before.Holdout.Perplexity,
		"after perplexity":  report.Metrics.After.Holdout.Perplexity,
		"before accuracy":   report.Metrics.Before.NextTokenAccuracy,
		"after accuracy":    report.Metrics.After.NextTokenAccuracy,
	} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			t.Fatalf("%s is not finite: %v", name, value)
		}
	}
	if report.IndependentInference.Prompt == "" || report.IndependentInference.After.VocabHash == "" {
		t.Fatalf("independent inference = %#v", report.IndependentInference)
	}
	if len(report.Documents) != 3 || report.Documents[0].RawSHA256 == "" || report.Documents[0].SelectedBytes == 0 {
		t.Fatalf("document fingerprints = %#v", report.Documents)
	}
	wantDocumentHashes := map[string]string{
		"book-alice": sha256Hex([]byte("Alice was beginning to get very tired. She looked around.\n")),
		"book-time":  sha256Hex([]byte("The Time Traveller (for so it will be convenient to speak of him) waited.\n")),
		"book-pride": sha256Hex([]byte("It is a truth universally acknowledged that a single man must be in want of a wife.\n")),
	}
	for _, document := range report.Documents {
		if document.RawSHA256 != wantDocumentHashes[document.ID] {
			t.Fatalf("document %q raw SHA-256 = %q, want %q", document.ID, document.RawSHA256, wantDocumentHashes[document.ID])
		}
	}
	if strings.Contains(firstOut.String(), "\"accelerator\"") {
		t.Fatalf("report contains an accelerator field: %s", firstOut.String())
	}
	if len(report.ScopeLimitations) == 0 {
		t.Fatal("scope limitations are missing")
	}

	var secondOut, secondErr bytes.Buffer
	if err := run(context.Background(), args, &secondOut, &secondErr); err != nil {
		t.Fatalf("second run: %v\nstderr=%s", err, secondErr.String())
	}
	if !bytes.Equal(firstOut.Bytes(), secondOut.Bytes()) {
		t.Fatalf("identical runs produced different reports:\nfirst=%s\nsecond=%s", firstOut.String(), secondOut.String())
	}
}

func TestRunRejectsInvalidArgumentsWithoutPartialJSON(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"--manifest", filepath.Join(t.TempDir(), "missing.json")},
		{"--manifest", filepath.Join(t.TempDir(), "missing.json"), "extra"},
	} {
		var stdout, stderr bytes.Buffer
		if err := run(context.Background(), args, &stdout, &stderr); err == nil {
			t.Fatalf("accepted invalid args %v", args)
		}
		if stdout.Len() != 0 {
			t.Fatalf("args %v wrote partial stdout: %s", args, stdout.String())
		}
	}
}

func TestSelectBodyWindowUsesGutenbergAnchorAndValidUTF8(t *testing.T) {
	body := "prefix\nIt is a truth universally acknowledged\n" + strings.Repeat("é", 64)
	selected, anchor, offset := selectBodyWindow(body, 64)
	if anchor != "It is a truth universally acknowledged" || offset != strings.Index(body, anchor) {
		t.Fatalf("anchor = %q offset = %d", anchor, offset)
	}
	if len(selected) > 64 || !utf8.ValidString(selected) {
		t.Fatalf("selected window is invalid: len=%d valid=%v", len(selected), utf8.ValidString(selected))
	}
}

func TestDocumentBodyExtractsOfficialGutenbergSpan(t *testing.T) {
	text := "header\n*** START OF THE PROJECT GUTENBERG EBOOK TEST ***\nbody\n*** END OF THE PROJECT GUTENBERG EBOOK TEST ***\nfooter\n"
	body, start, end, err := documentBody(text)
	if err != nil {
		t.Fatalf("documentBody: %v", err)
	}
	if body != "body\n" || start == "" || end == "" {
		t.Fatalf("body=%q start=%q end=%q", body, start, end)
	}
	if _, _, _, err := documentBody("*** START OF THE PROJECT GUTENBERG EBOOK TEST ***\nbody\n"); err == nil {
		t.Fatal("accepted a Gutenberg document without an END marker")
	}
}

type testCorpusManifest struct {
	Schema    string                  `json:"schema"`
	Scope     testCorpusScope         `json:"scope"`
	Documents []testCorpusManifestDoc `json:"documents"`
}

type testCorpusScope struct {
	Kind     string `json:"kind"`
	Language string `json:"language"`
	Note     string `json:"note"`
}

type testCorpusManifestDoc struct {
	ID      string            `json:"id"`
	Source  string            `json:"source"`
	Path    string            `json:"path"`
	License testCorpusLicense `json:"license"`
}

type testCorpusLicense struct {
	Holder string `json:"holder"`
	Terms  string `json:"terms"`
	Source string `json:"source"`
}

func writeTestCorpus(t *testing.T) (string, []byte) {
	t.Helper()
	root := t.TempDir()
	texts := []struct {
		name string
		text string
	}{
		{name: "alice.txt", text: "Alice was beginning to get very tired. She looked around.\n"},
		{name: "time.txt", text: "The Time Traveller (for so it will be convenient to speak of him) waited.\n"},
		{name: "pride.txt", text: "It is a truth universally acknowledged that a single man must be in want of a wife.\n"},
	}
	docs := make([]testCorpusManifestDoc, 0, len(texts))
	for i, item := range texts {
		if err := os.WriteFile(filepath.Join(root, item.name), []byte(item.text), 0o644); err != nil {
			t.Fatal(err)
		}
		docs = append(docs, testCorpusManifestDoc{
			ID:     "book-" + item.name[:len(item.name)-len(filepath.Ext(item.name))],
			Source: "source-" + string(rune('a'+i)),
			Path:   item.name,
			License: testCorpusLicense{
				Holder: "Test holder",
				Terms:  "test-only",
				Source: "https://example.test/license",
			},
		})
	}
	manifest := testCorpusManifest{
		Schema:    "coimnet-textgen-corpus/v1",
		Scope:     testCorpusScope{Kind: "real", Language: "en", Note: "test corpus"},
		Documents: docs,
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	return manifestPath, manifestBytes
}
