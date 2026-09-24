package textgen

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/TimLai666/coimnet/internal/strictjson"
)

const (
	textCorpusSchema = "coimnet-textgen-corpus/v1"
	maxCorpusBytes   = int64(16 << 20)
)

// License records who holds a document and on what terms it may be used; every field is required.
type License struct {
	Holder string `json:"holder"`
	Terms  string `json:"terms"`
	Source string `json:"source"`
}

// Document is one unit of text; splits never cut a document, and its Source decides its side of a split.
type Document struct {
	ID      string  `json:"id"`
	Source  string  `json:"source"`
	Text    string  `json:"text"`
	License License `json:"license"`
}

// DataScope states what a corpus is: Kind "fixture" or "real", the language(s), and a free note; reports copy it verbatim.
type DataScope struct {
	Kind     string `json:"kind"`
	Language string `json:"language"`
	Note     string `json:"note"`
}

// Corpus is the documents plus their scope and the digest of the manifest bytes parsed by ReadCorpus.
type Corpus struct {
	Scope          DataScope  `json:"scope"`
	Documents      []Document `json:"documents"`
	ManifestSHA256 string     `json:"manifest_sha256,omitempty"`
}

// corpusManifest is the strict JSON representation read from disk.
type corpusManifest struct {
	Schema    *string                   `json:"schema"`
	Scope     *DataScope                `json:"scope"`
	Documents *[]corpusManifestDocument `json:"documents"`
}

// corpusManifestDocument keeps text and path pointers distinct so missing and null values are rejected.
type corpusManifestDocument struct {
	ID      string                 `json:"id"`
	Source  string                 `json:"source"`
	Text    *string                `json:"text"`
	Path    *string                `json:"path"`
	License *corpusManifestLicense `json:"license"`
}

// corpusManifestLicense preserves missing or null required license fields.
type corpusManifestLicense struct {
	Holder *string `json:"holder"`
	Terms  *string `json:"terms"`
	Source *string `json:"source"`
}

// ReadCorpus reads a strict coimnet-textgen-corpus/v1 manifest and its relative text files.
func ReadCorpus(ctx context.Context, manifestPath string) (corpus Corpus, retErr error) {
	if ctx == nil {
		return Corpus{}, errors.New("textgen: context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return Corpus{}, fmt.Errorf("textgen: read corpus: %w", err)
	}
	if manifestPath == "" {
		return Corpus{}, errors.New("textgen: manifest path is empty")
	}
	manifestAbs, err := filepath.Abs(manifestPath)
	if err != nil {
		return Corpus{}, fmt.Errorf("textgen: resolve manifest path: %w", err)
	}
	manifestDir := filepath.Dir(manifestAbs)
	root, err := os.OpenRoot(manifestDir)
	if err != nil {
		return Corpus{}, fmt.Errorf("textgen: open manifest directory %q: %w", manifestDir, err)
	}
	defer func() {
		if closeErr := root.Close(); closeErr != nil {
			closeErr = fmt.Errorf("textgen: close manifest directory %q: %w", manifestDir, closeErr)
			if retErr == nil {
				corpus = Corpus{}
				retErr = closeErr
			} else {
				retErr = errors.Join(retErr, closeErr)
			}
		}
	}()

	manifestFile, err := root.Open(filepath.Base(manifestAbs))
	if err != nil {
		return Corpus{}, fmt.Errorf("textgen: open manifest %q: %w", manifestPath, err)
	}
	raw, readErr := readCorpusFile(manifestFile, maxCorpusBytes)
	closeErr := manifestFile.Close()
	if readErr != nil {
		return Corpus{}, fmt.Errorf("textgen: read manifest %q: %w", manifestPath, readErr)
	}
	if closeErr != nil {
		return Corpus{}, fmt.Errorf("textgen: close manifest %q: %w", manifestPath, closeErr)
	}
	if !utf8.Valid(raw) {
		return Corpus{}, errors.New("textgen: manifest is not valid UTF-8")
	}
	if err := validateCorpusJSONUnicode(raw); err != nil {
		return Corpus{}, err
	}
	var manifest corpusManifest
	if err := strictjson.Decode(bytes.NewReader(raw), maxCorpusBytes, &manifest); err != nil {
		return Corpus{}, fmt.Errorf("textgen: decode manifest %q: %w", manifestPath, err)
	}
	if err := validateCorpusManifestKeys(raw); err != nil {
		return Corpus{}, err
	}
	if manifest.Schema == nil {
		return Corpus{}, errors.New("textgen: manifest schema is required")
	}
	if *manifest.Schema != textCorpusSchema {
		return Corpus{}, fmt.Errorf("textgen: manifest schema %q is unsupported, want %q", *manifest.Schema, textCorpusSchema)
	}
	if manifest.Scope == nil {
		return Corpus{}, errors.New("textgen: scope is required")
	}
	if manifest.Scope.Kind != "fixture" && manifest.Scope.Kind != "real" {
		return Corpus{}, fmt.Errorf("textgen: scope.kind %q must be fixture or real", manifest.Scope.Kind)
	}
	if manifest.Documents == nil {
		return Corpus{}, errors.New("textgen: documents is required")
	}

	manifestSum := sha256.Sum256(raw)
	corpus = Corpus{
		Scope: *manifest.Scope, Documents: make([]Document, 0, len(*manifest.Documents)),
		ManifestSHA256: hex.EncodeToString(manifestSum[:]),
	}
	seenIDs := make(map[string]struct{}, len(*manifest.Documents))
	for i, rawDocument := range *manifest.Documents {
		if err := ctx.Err(); err != nil {
			return Corpus{}, fmt.Errorf("textgen: read document %d: %w", i, err)
		}
		id := strings.TrimSpace(rawDocument.ID)
		if id == "" {
			return Corpus{}, fmt.Errorf("textgen: document %d id is required", i)
		}
		if _, exists := seenIDs[rawDocument.ID]; exists {
			return Corpus{}, fmt.Errorf("textgen: document %q has a duplicate id", rawDocument.ID)
		}
		seenIDs[rawDocument.ID] = struct{}{}
		if strings.TrimSpace(rawDocument.Source) == "" {
			return Corpus{}, fmt.Errorf("textgen: document %q source is required", rawDocument.ID)
		}
		if (rawDocument.Text == nil) == (rawDocument.Path == nil) {
			return Corpus{}, fmt.Errorf("textgen: document %q must provide exactly one of text or path", rawDocument.ID)
		}
		license, err := readCorpusLicense(rawDocument.ID, rawDocument.License)
		if err != nil {
			return Corpus{}, err
		}
		text := ""
		if rawDocument.Text != nil {
			text = *rawDocument.Text
		} else {
			text, err = readCorpusDocument(root, rawDocument.ID, *rawDocument.Path)
			if err != nil {
				return Corpus{}, err
			}
		}
		if !utf8.ValidString(text) {
			return Corpus{}, fmt.Errorf("textgen: document %q text is not valid UTF-8", rawDocument.ID)
		}
		corpus.Documents = append(corpus.Documents, Document{
			ID: rawDocument.ID, Source: rawDocument.Source, Text: text, License: license,
		})
	}
	return corpus, nil
}

// SplitBySource assigns complete sources to train or test and preserves input order within each result.
func SplitBySource(docs []Document, testFraction float64, seed uint64) (train, test []Document, err error) {
	if !(testFraction > 0 && testFraction < 1) {
		return nil, nil, fmt.Errorf("textgen: test fraction %v must be strictly between 0 and 1", testFraction)
	}
	sourceSet := make(map[string]struct{}, len(docs))
	for i, doc := range docs {
		if strings.TrimSpace(doc.Source) == "" {
			return nil, nil, fmt.Errorf("textgen: document %q at index %d has an empty source", doc.ID, i)
		}
		sourceSet[doc.Source] = struct{}{}
	}
	sources := make([]string, 0, len(sourceSet))
	for source := range sourceSet {
		sources = append(sources, source)
	}
	if len(sources) < 2 {
		return nil, nil, fmt.Errorf("textgen: source split needs at least 2 distinct sources, got %d", len(sources))
	}
	sort.Strings(sources)
	rng := rand.New(rand.NewPCG(seed, 0))
	rng.Shuffle(len(sources), func(i, j int) { sources[i], sources[j] = sources[j], sources[i] })
	testSourceCount := int(math.Round(testFraction * float64(len(sources))))
	if testSourceCount < 1 {
		testSourceCount = 1
	}
	if testSourceCount >= len(sources) {
		testSourceCount = len(sources) - 1
	}
	inTest := make(map[string]struct{}, testSourceCount)
	for _, source := range sources[:testSourceCount] {
		inTest[source] = struct{}{}
	}
	train = make([]Document, 0, len(docs))
	test = make([]Document, 0, len(docs))
	for _, doc := range docs {
		if _, ok := inTest[doc.Source]; ok {
			test = append(test, doc)
		} else {
			train = append(train, doc)
		}
	}
	return train, test, nil
}

// TeacherText is text a teacher produced, with where it came from; Source is required.
type TeacherText struct {
	Text   string `json:"text"`
	Source string `json:"source"`
}

// DedupTeacher removes teacher texts that overlap after trimming with any test document text.
func DedupTeacher(texts []TeacherText, test []Document) (kept []TeacherText, removed int, err error) {
	for i, teacher := range texts {
		if strings.TrimSpace(teacher.Source) == "" {
			return nil, 0, fmt.Errorf("textgen: teacher text %d source is required", i)
		}
	}
	kept = make([]TeacherText, 0, len(texts))
	for _, teacher := range texts {
		candidate := strings.TrimSpace(teacher.Text)
		duplicate := false
		for _, doc := range test {
			target := strings.TrimSpace(doc.Text)
			if strings.Contains(candidate, target) || strings.Contains(target, candidate) {
				duplicate = true
				break
			}
		}
		if duplicate {
			removed++
		} else {
			kept = append(kept, teacher)
		}
	}
	return kept, removed, nil
}

// FixtureCorpus returns deterministic synthetic documents for offline pipeline checks.
func FixtureCorpus(seed uint64, documents, sources int) (Corpus, error) {
	if documents < 2 {
		return Corpus{}, fmt.Errorf("textgen: fixture needs at least 2 documents, got %d", documents)
	}
	if sources < 2 || sources > documents {
		return Corpus{}, fmt.Errorf("textgen: fixture sources must be between 2 and %d, got %d", documents, sources)
	}
	subjects := []rune("貓狗鳥")
	verbs := []rune("吃看")
	objects := []rune("米水肉")
	corpus := Corpus{
		Scope: DataScope{
			Kind: "fixture", Language: "zh-Hant (synthetic grammar)",
			Note: "3 subjects x 2 verbs x 3 objects; proves the pipeline runs, not language ability",
		},
		Documents: make([]Document, documents),
	}
	license := License{
		Holder: "CoImNet fixture", Terms: "generated in memory; no external rights",
		Source: "coimnet-textgen-fixture/v1",
	}
	for i := range corpus.Documents {
		rng := rand.New(rand.NewPCG(seed, uint64(i)))
		var text strings.Builder
		text.Grow(36)
		for sentence := 0; sentence < 3; sentence++ {
			text.WriteRune(subjects[rng.IntN(len(subjects))])
			text.WriteRune(verbs[rng.IntN(len(verbs))])
			text.WriteRune(objects[rng.IntN(len(objects))])
			text.WriteRune('。')
		}
		corpus.Documents[i] = Document{
			ID: fmt.Sprintf("fixture-doc-%d", i), Source: fmt.Sprintf("fixture-source-%d", i%sources),
			Text: text.String(), License: license,
		}
	}
	return corpus, nil
}

// readCorpusFile reads at most maxBytes plus one byte, distinguishing oversized files from read failures.
func readCorpusFile(file io.Reader, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("file exceeds %d bytes (16 MiB)", maxBytes)
	}
	return data, nil
}

// readCorpusLicense checks and copies each required license value for one document.
func readCorpusLicense(id string, raw *corpusManifestLicense) (License, error) {
	if raw == nil {
		return License{}, fmt.Errorf("textgen: document %q license is required", id)
	}
	fields := []struct {
		name  string
		value *string
	}{{"holder", raw.Holder}, {"terms", raw.Terms}, {"source", raw.Source}}
	for _, field := range fields {
		if field.value == nil || strings.TrimSpace(*field.value) == "" {
			return License{}, fmt.Errorf("textgen: document %q license.%s is required", id, field.name)
		}
	}
	return License{Holder: *raw.Holder, Terms: *raw.Terms, Source: *raw.Source}, nil
}

// readCorpusDocument opens a relative document path inside the manifest root and checks its bytes.
func readCorpusDocument(root *os.Root, id, name string) (string, error) {
	if name == "" || filepath.IsAbs(name) || filepath.VolumeName(name) != "" || hasWindowsVolume(name) || strings.HasPrefix(name, "\\") || hasParentPath(name) {
		return "", fmt.Errorf("textgen: document %q path %q must be relative and stay inside the manifest directory", id, name)
	}
	file, err := root.Open(name)
	if err != nil {
		return "", fmt.Errorf("textgen: document %q path %q: %w", id, name, err)
	}
	data, readErr := readCorpusFile(file, maxCorpusBytes)
	closeErr := file.Close()
	if readErr != nil {
		return "", fmt.Errorf("textgen: document %q path %q: %w", id, name, readErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("textgen: close document %q path %q: %w", id, name, closeErr)
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("textgen: document %q text is not valid UTF-8", id)
	}
	return string(data), nil
}

// hasWindowsVolume reports whether a path begins with a drive-letter volume prefix.
func hasWindowsVolume(name string) bool {
	return len(name) >= 2 && ((name[0] >= 'a' && name[0] <= 'z') || (name[0] >= 'A' && name[0] <= 'Z')) && name[1] == ':'
}

// hasParentPath reports whether a path contains a parent-directory component.
func hasParentPath(name string) bool {
	for _, part := range strings.FieldsFunc(name, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return true
		}
	}
	return false
}

// validateCorpusManifestKeys rejects case aliases so every accepted field uses the declared snake_case spelling.
func validateCorpusManifestKeys(raw []byte) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return fmt.Errorf("textgen: decode manifest keys: %w", err)
	}
	if err := checkCorpusObjectKeys("manifest", root, "schema", "scope", "documents"); err != nil {
		return err
	}
	var scope map[string]json.RawMessage
	if err := json.Unmarshal(root["scope"], &scope); err != nil {
		return fmt.Errorf("textgen: scope must be an object: %w", err)
	}
	if err := checkCorpusObjectKeys("scope", scope, "kind", "language", "note"); err != nil {
		return err
	}
	var documents []json.RawMessage
	if err := json.Unmarshal(root["documents"], &documents); err != nil {
		return fmt.Errorf("textgen: documents must be an array: %w", err)
	}
	for i, rawDocument := range documents {
		var document map[string]json.RawMessage
		if err := json.Unmarshal(rawDocument, &document); err != nil {
			return fmt.Errorf("textgen: document %d must be an object: %w", i, err)
		}
		if err := checkCorpusObjectKeys(fmt.Sprintf("document %d", i), document, "id", "source", "text", "path", "license"); err != nil {
			return err
		}
		var license map[string]json.RawMessage
		if err := json.Unmarshal(document["license"], &license); err != nil {
			return fmt.Errorf("textgen: document %d license must be an object: %w", i, err)
		}
		if err := checkCorpusObjectKeys(fmt.Sprintf("document %d license", i), license, "holder", "terms", "source"); err != nil {
			return err
		}
	}
	return nil
}

// checkCorpusObjectKeys verifies that an object contains only exact, supported JSON key spellings.
func checkCorpusObjectKeys(where string, object map[string]json.RawMessage, allowed ...string) error {
	for key := range object {
		found := false
		for _, candidate := range allowed {
			if key == candidate {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("textgen: %s has unknown field %q", where, key)
		}
	}
	return nil
}

// validateCorpusJSONUnicode rejects unpaired escaped UTF-16 surrogates before encoding/json replaces them.
func validateCorpusJSONUnicode(raw []byte) error {
	for i := 0; i < len(raw); i++ {
		if raw[i] != '"' {
			continue
		}
	scanString:
		for i++; i < len(raw); i++ {
			switch raw[i] {
			case '"':
				break scanString
			case '\\':
				if i+1 >= len(raw) || raw[i+1] != 'u' {
					i++
					continue
				}
				if i+6 > len(raw) {
					break
				}
				value, ok := corpusJSONHex4(raw[i+2 : i+6])
				if !ok {
					break
				}
				switch {
				case value >= 0xD800 && value <= 0xDBFF:
					if i+12 > len(raw) || raw[i+6] != '\\' || raw[i+7] != 'u' {
						return errors.New("textgen: manifest contains an unpaired Unicode surrogate")
					}
					low, valid := corpusJSONHex4(raw[i+8 : i+12])
					if !valid || low < 0xDC00 || low > 0xDFFF {
						return errors.New("textgen: manifest contains an unpaired Unicode surrogate")
					}
					i += 11
				case value >= 0xDC00 && value <= 0xDFFF:
					return errors.New("textgen: manifest contains an unpaired Unicode surrogate")
				default:
					i += 5
				}
			}
		}
	}
	return nil
}

// corpusJSONHex4 parses one four-digit hexadecimal JSON escape.
func corpusJSONHex4(raw []byte) (uint16, bool) {
	if len(raw) != 4 {
		return 0, false
	}
	var value uint16
	for _, digit := range raw {
		value <<= 4
		switch {
		case digit >= '0' && digit <= '9':
			value |= uint16(digit - '0')
		case digit >= 'a' && digit <= 'f':
			value |= uint16(digit-'a') + 10
		case digit >= 'A' && digit <= 'F':
			value |= uint16(digit-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}
