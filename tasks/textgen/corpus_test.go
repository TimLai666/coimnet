package textgen

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestReadCorpus(t *testing.T) {
	manifest := filepath.Join("testdata", "corpus", "manifest.json")
	corpus, err := ReadCorpus(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := corpus.ManifestSHA256, "75212a011009217b438d68b7de9ab377140d27dd54b1ad78ab9647ce8a978ea7"; got != want {
		t.Fatalf("parsed manifest SHA-256 = %q, want %q", got, want)
	}
	wantLicense := License{Holder: "Example holder", Terms: "Example terms", Source: "Example source"}
	if corpus.Scope != (DataScope{Kind: "fixture", Language: "zh-Hant", Note: "tiny test corpus"}) {
		t.Fatalf("scope = %#v", corpus.Scope)
	}
	if len(corpus.Documents) != 2 {
		t.Fatalf("documents = %d, want 2", len(corpus.Documents))
	}
	if got := corpus.Documents[0]; got.ID != "doc-text" || got.Source != "book-a" || got.Text != "這是文字文件。" || got.License != wantLicense {
		t.Fatalf("text document = %#v", got)
	}
	if got := corpus.Documents[1]; got.ID != "doc-path" || got.Source != "book-b" || got.Text != "從相對路徑讀取的文字。\n" || got.License != wantLicense {
		t.Fatalf("path document = %#v", got)
	}

	t.Run("invalid manifests", func(t *testing.T) {
		outside := filepath.Join(t.TempDir(), "outside.txt")
		if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
			t.Fatal(err)
		}
		base := map[string]any{
			"schema": "coimnet-textgen-corpus/v1",
			"scope":  map[string]any{"kind": "fixture", "language": "en", "note": "test"},
			"documents": []any{map[string]any{
				"id": "doc-case", "source": "source-a", "text": "text",
				"license": map[string]any{"holder": "holder", "terms": "terms", "source": "source"},
			}},
		}
		cases := []struct {
			name string
			body func() []byte
			want string
		}{
			{name: "missing license field", body: func() []byte {
				return []byte(`{"schema":"coimnet-textgen-corpus/v1","scope":{"kind":"fixture","language":"en","note":"test"},"documents":[{"id":"doc-case","source":"source-a","text":"text","license":{"holder":"holder","terms":"terms"}}]}`)
			}, want: "license.source"},
			{name: "both text and path", body: func() []byte {
				return []byte(`{"schema":"coimnet-textgen-corpus/v1","scope":{"kind":"fixture","language":"en","note":"test"},"documents":[{"id":"doc-case","source":"source-a","text":"text","path":"second.txt","license":{"holder":"holder","terms":"terms","source":"source"}}]}`)
			}, want: "doc-case"},
			{name: "neither text nor path", body: func() []byte {
				return []byte(`{"schema":"coimnet-textgen-corpus/v1","scope":{"kind":"fixture","language":"en","note":"test"},"documents":[{"id":"doc-case","source":"source-a","license":{"holder":"holder","terms":"terms","source":"source"}}]}`)
			}, want: "doc-case"},
			{name: "parent path", body: func() []byte {
				return marshalManifestWithPath(t, "../x.txt")
			}, want: "doc-case"},
			{name: "absolute path", body: func() []byte {
				return marshalManifestWithPath(t, outside)
			}, want: "doc-case"},
			{name: "drive path", body: func() []byte {
				return marshalManifestWithPath(t, `C:\outside.txt`)
			}, want: "doc-case"},
			{name: "duplicate id", body: func() []byte {
				m := cloneManifest(t, base)
				m["documents"] = []any{m["documents"].([]any)[0], m["documents"].([]any)[0]}
				return marshalJSON(t, m)
			}, want: "doc-case"},
			{name: "unknown field", body: func() []byte {
				m := cloneManifest(t, base)
				m["extra"] = true
				return marshalJSON(t, m)
			}, want: "extra"},
			{name: "non-snake-case field", body: func() []byte {
				return []byte(`{"schema":"coimnet-textgen-corpus/v1","scope":{"kind":"fixture","language":"en","note":"test"},"documents":[{"id":"doc-case","source":"source-a","text":"text","license":{"Holder":"holder","terms":"terms","source":"source"}}]}`)
			}, want: "Holder"},
			{name: "invalid scope kind", body: func() []byte {
				m := cloneManifest(t, base)
				m["scope"].(map[string]any)["kind"] = "unknown"
				return marshalJSON(t, m)
			}, want: "scope.kind"},
			{name: "wrong schema", body: func() []byte {
				m := cloneManifest(t, base)
				m["schema"] = "other/v1"
				return marshalJSON(t, m)
			}, want: "schema"},
			{name: "duplicate JSON key", body: func() []byte {
				return []byte(`{"schema":"coimnet-textgen-corpus/v1","schema":"coimnet-textgen-corpus/v1","scope":{"kind":"fixture","language":"en","note":"test"},"documents":[]}`)
			}, want: "duplicate"},
			{name: "oversized manifest", body: func() []byte {
				return []byte(strings.Repeat(" ", (16<<20)+1))
			}, want: "16 MiB"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				manifestPath := writeManifest(t, tc.body())
				_, err := ReadCorpus(context.Background(), manifestPath)
				if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.want)) {
					t.Fatalf("error = %v, want it to contain %q", err, tc.want)
				}
			})
		}
		t.Run("invalid UTF-8 document", func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "invalid.txt"), []byte{0xff, 0xfe}, 0o600); err != nil {
				t.Fatal(err)
			}
			manifestPath := writeManifestInDir(t, dir, marshalManifestWithPath(t, "invalid.txt"))
			_, err := ReadCorpus(context.Background(), manifestPath)
			if err == nil || !strings.Contains(err.Error(), "doc-case") {
				t.Fatalf("error = %v, want document id doc-case", err)
			}
		})
		t.Run("oversized document", func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "large.txt"), []byte(strings.Repeat("x", (16<<20)+1)), 0o600); err != nil {
				t.Fatal(err)
			}
			manifestPath := writeManifestInDir(t, dir, marshalManifestWithPath(t, "large.txt"))
			_, err := ReadCorpus(context.Background(), manifestPath)
			if err == nil || !strings.Contains(err.Error(), "doc-case") {
				t.Fatalf("error = %v, want document id doc-case", err)
			}
		})
	})
}

func TestSplitBySource(t *testing.T) {
	docs := []Document{
		{ID: "a1", Source: "a", Text: "a1"},
		{ID: "b1", Source: "b", Text: "b1"},
		{ID: "a2", Source: "a", Text: "a2"},
		{ID: "c1", Source: "c", Text: "c1"},
		{ID: "d1", Source: "d", Text: "d1"},
	}
	train, test, err := SplitBySource(docs, 0.5, 7)
	if err != nil {
		t.Fatal(err)
	}
	assertSourceSplit(t, docs, train, test)
	if got := countSources(test); got != 2 {
		t.Fatalf("test sources = %d, want round(0.5 x 4) = 2", got)
	}
	trainAgain, testAgain, err := SplitBySource(docs, 0.5, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(train, trainAgain) || !reflect.DeepEqual(test, testAgain) {
		t.Fatal("same seed produced different results")
	}
	trainOther, testOther, err := SplitBySource(docs, 0.5, 8)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(train, trainOther) && reflect.DeepEqual(test, testOther) {
		t.Fatal("different seeds produced the same split")
	}
	for _, tc := range []struct {
		name     string
		docs     []Document
		fraction float64
	}{
		{name: "one source", docs: []Document{{ID: "x", Source: "same"}, {ID: "y", Source: "same"}}, fraction: 0.5},
		{name: "zero fraction", docs: docs, fraction: 0},
		{name: "unit fraction", docs: docs, fraction: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := SplitBySource(tc.docs, tc.fraction, 1); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	if _, _, err := SplitBySource(nil, 0.5, 1); err == nil {
		t.Fatal("expected an error for no sources")
	}
	if _, _, err := SplitBySource([]Document{{ID: "x", Source: ""}, {ID: "y", Source: "b"}}, 0.5, 1); err == nil {
		t.Fatal("expected an error for an empty source")
	}
}

func TestDedupTeacher(t *testing.T) {
	testDocs := []Document{{Text: "完整保留的測試文字"}, {Text: "目標句子"}}
	texts := []TeacherText{
		{Text: "完整保留的測試文字", Source: "exact"},
		{Text: "前綴完整保留的測試文字後綴", Source: "contains"},
		{Text: "目標", Source: "contained"},
		{Text: "  目標句子  ", Source: "trimmed"},
		{Text: "與保留文件無關", Source: "keep-a"},
		{Text: "另一份無關文字", Source: "keep-b"},
	}
	kept, removed, err := DedupTeacher(texts, testDocs)
	if err != nil {
		t.Fatal(err)
	}
	want := []TeacherText{{Text: "與保留文件無關", Source: "keep-a"}, {Text: "另一份無關文字", Source: "keep-b"}}
	if removed != 4 || !reflect.DeepEqual(kept, want) {
		t.Fatalf("kept = %#v, removed = %d", kept, removed)
	}
	if _, _, err := DedupTeacher([]TeacherText{{Text: "x"}}, nil); err == nil {
		t.Fatal("expected error for empty source")
	}
}

func TestFixtureCorpus(t *testing.T) {
	first, err := FixtureCorpus(1, 12, 4)
	if err != nil {
		t.Fatal(err)
	}
	second, err := FixtureCorpus(1, 12, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("same seed produced different corpus")
	}
	t.Logf("first document text: %s", first.Documents[0].Text)
	if first.Scope != (DataScope{
		Kind: "fixture", Language: "zh-Hant (synthetic grammar)",
		Note: "3 subjects x 2 verbs x 3 objects; proves the pipeline runs, not language ability",
	}) {
		t.Fatalf("scope = %#v", first.Scope)
	}
	if len(first.Documents) != 12 {
		t.Fatalf("documents = %d, want 12", len(first.Documents))
	}
	seenSources := map[string]bool{}
	for i, doc := range first.Documents {
		if doc.ID != fmt.Sprintf("fixture-doc-%d", i) || doc.Source != fmt.Sprintf("fixture-source-%d", i%4) {
			t.Errorf("document %d identity = (%q, %q)", i, doc.ID, doc.Source)
		}
		seenSources[doc.Source] = true
		if !utf8.ValidString(doc.Text) {
			t.Errorf("document %q is invalid UTF-8", doc.ID)
		}
		if got := strings.Split(doc.Text, "。"); len(got) != 4 || got[3] != "" {
			t.Errorf("document %q has sentence form %q", doc.ID, doc.Text)
			continue
		}
		for _, sentence := range strings.Split(doc.Text, "。")[:3] {
			if utf8.RuneCountInString(sentence+"。") != 4 {
				t.Errorf("sentence %q has %d characters including punctuation", sentence+"。", utf8.RuneCountInString(sentence+"。"))
			}
			runes := []rune(sentence)
			if !oneOf(runes[0], "貓狗鳥") || !oneOf(runes[1], "吃看") || !oneOf(runes[2], "米水肉") {
				t.Errorf("sentence %q is outside the grammar", sentence)
			}
		}
	}
	if len(seenSources) != 4 {
		t.Fatalf("sources = %d, want 4", len(seenSources))
	}
	if first.Documents[0].License != (License{
		Holder: "CoImNet fixture", Terms: "generated in memory; no external rights", Source: "coimnet-textgen-fixture/v1",
	}) {
		t.Fatalf("license = %#v", first.Documents[0].License)
	}
	for _, tc := range []struct {
		seed               uint64
		documents, sources int
	}{{1, 1, 1}, {1, 2, 1}, {1, 2, 3}, {1, 0, 0}} {
		if _, err := FixtureCorpus(tc.seed, tc.documents, tc.sources); err == nil {
			t.Errorf("FixtureCorpus(%d, %d, %d) succeeded", tc.seed, tc.documents, tc.sources)
		}
	}
}

func assertSourceSplit(t *testing.T, docs, train, test []Document) {
	t.Helper()
	trainSources, testSources := map[string]bool{}, map[string]bool{}
	for _, doc := range train {
		trainSources[doc.Source] = true
	}
	for _, doc := range test {
		testSources[doc.Source] = true
	}
	if len(trainSources) == 0 || len(testSources) == 0 {
		t.Fatal("both sides must contain at least one source")
	}
	for source := range trainSources {
		if testSources[source] {
			t.Errorf("source %q appears in both sides", source)
		}
	}
	if len(train)+len(test) != len(docs) {
		t.Fatalf("split lost documents: train=%d test=%d input=%d", len(train), len(test), len(docs))
	}
	assertInputOrder(t, docs, train)
	assertInputOrder(t, docs, test)
}

func assertInputOrder(t *testing.T, input, side []Document) {
	t.Helper()
	last := -1
	for _, doc := range side {
		index := -1
		for i := last + 1; i < len(input); i++ {
			if input[i] == doc {
				index = i
				break
			}
		}
		if index < 0 {
			t.Fatalf("document order changed or document repeated: %#v", doc)
		}
		last = index
	}
}

func countSources(docs []Document) int {
	sources := make(map[string]struct{}, len(docs))
	for _, doc := range docs {
		sources[doc.Source] = struct{}{}
	}
	return len(sources)
}

func oneOf(value rune, choices string) bool {
	return strings.ContainsRune(choices, value)
}

func marshalManifestWithPath(t *testing.T, path string) []byte {
	t.Helper()
	return marshalJSON(t, map[string]any{
		"schema": "coimnet-textgen-corpus/v1",
		"scope":  map[string]any{"kind": "fixture", "language": "en", "note": "test"},
		"documents": []any{map[string]any{
			"id": "doc-case", "source": "source-a", "path": path,
			"license": map[string]any{"holder": "holder", "terms": "terms", "source": "source"},
		}},
	})
}

func cloneManifest(t *testing.T, manifest map[string]any) map[string]any {
	t.Helper()
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var clone map[string]any
	if err := json.Unmarshal(body, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func marshalJSON(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func writeManifest(t *testing.T, body []byte) string {
	t.Helper()
	dir := t.TempDir()
	return writeManifestInDir(t, dir, body)
}

func writeManifestInDir(t *testing.T, dir string, body []byte) string {
	t.Helper()
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
