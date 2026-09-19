// Package tokenizer turns text into the integer ids the text tasks encode
// through. The built-in vocabulary is byte based, so any Unicode text round
// trips without an out-of-vocabulary case, and special tokens are never
// triggered by the text itself: they exist only where a caller puts their ids.
// The declaration that produced a vocabulary — format, special tokens and
// their roles — is saved as JSON and summarised by a VocabHash, so a run can
// be reproduced and a perplexity can say which vocabulary it was measured
// under.
package tokenizer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/TimLai666/coimnet/internal/strictjson"
)

// FormatByte is the only vocabulary format this package builds: ids 0..255 are
// the raw bytes and the declared specials follow them.
const FormatByte = "coimnet-byte-vocab/v1"

// byteCount is how many ids the raw bytes reserve before the first special
// token, so special ids start at 256.
const byteCount = 256

// MaxConfigBytes bounds a declaration read from disk. A byte vocabulary
// declares a format and a handful of specials, so a megabyte is far more than
// a real file needs and still refuses a runaway one before decoding it.
const MaxConfigBytes = 1 << 20

// roleOrder lists the roles a special token may declare, in the order an error
// message names them.
var roleOrder = []string{"bos", "eos", "pad", "sep"}

// Vocabulary is the replaceable interface every text task encodes through.
type Vocabulary interface {
	// Encode turns text into ids. Special tokens are never produced from
	// text: the characters "<eos>" encode as the five bytes they are.
	Encode(text string) ([]int, error)
	// Decode turns ids back into text. A special token decodes to the empty
	// string; an id outside the vocabulary is an error.
	Decode(ids []int) (string, error)
	// Size is the number of ids, raw bytes and specials together.
	Size() int
	// Special reports the id of the special token declared under this name.
	Special(name string) (int, bool)
	// Hash is the VocabHash: the hexadecimal SHA-256 of the canonical JSON of
	// the declaration behind this vocabulary.
	Hash() string
}

// Config is the saved, reproducible declaration of a byte vocabulary: format
// name, the special tokens in order and their roles.
type Config struct {
	Format   string         `json:"format"`
	Specials []SpecialToken `json:"specials"`
}

// SpecialToken is one special token: the name it is addressed by and the role
// it plays, which must be one of bos, eos, pad or sep.
type SpecialToken struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

// ByteVocab maps ids 0..255 to bytes and appends the specials after them in
// Config order, so the first declared special is id 256.
type ByteVocab struct {
	config Config
	byName map[string]int
	hash   string
}

// ByteVocab is the reference implementation of the replaceable interface.
var _ Vocabulary = (*ByteVocab)(nil)

// New builds a vocabulary from a declaration. The format must be FormatByte,
// every special must carry a non-empty name and a known role, and two specials
// may not share a name. The declaration is copied, so later changes to the
// caller's slice cannot move an id or invalidate the hash.
func New(c Config) (*ByteVocab, error) {
	if err := validate(c); err != nil {
		return nil, fmt.Errorf("tokenizer: %w", err)
	}
	declared := normalise(c)
	hash, err := hashConfig(declared)
	if err != nil {
		return nil, err
	}
	v := &ByteVocab{
		config: declared,
		byName: make(map[string]int, len(declared.Specials)),
		hash:   hash,
	}
	for i, s := range declared.Specials {
		v.byName[s.Name] = byteCount + i
	}
	return v, nil
}

// Config returns the declaration this vocabulary was built from, with its own
// copy of the special list.
func (v *ByteVocab) Config() Config {
	return normalise(v.config)
}

// Encode returns one id per byte of text. It never fails; the error is part of
// the interface for vocabularies whose encoding can reject input.
func (v *ByteVocab) Encode(text string) ([]int, error) {
	ids := make([]int, len(text))
	for i := 0; i < len(text); i++ {
		ids[i] = int(text[i])
	}
	return ids, nil
}

// Decode concatenates the bytes behind the ids. Special ids contribute
// nothing, and an id below zero or beyond the vocabulary is an error naming
// its position, because silently dropping it would corrupt the text.
func (v *ByteVocab) Decode(ids []int) (string, error) {
	out := make([]byte, 0, len(ids))
	for i, id := range ids {
		switch {
		case id >= 0 && id < byteCount:
			out = append(out, byte(id))
		case id >= byteCount && id < v.Size():
			// A special token carries no text of its own.
		default:
			return "", fmt.Errorf("tokenizer: id %d at position %d is outside the vocabulary of %d ids", id, i, v.Size())
		}
	}
	return string(out), nil
}

// Size is 256 raw byte ids plus one id per declared special.
func (v *ByteVocab) Size() int {
	return byteCount + len(v.config.Specials)
}

// Special reports the id declared under this name, and whether the name was
// declared at all.
func (v *ByteVocab) Special(name string) (int, bool) {
	id, ok := v.byName[name]
	return id, ok
}

// Hash is the VocabHash of the declaration: 64 lowercase hexadecimal digits.
func (v *ByteVocab) Hash() string {
	return v.hash
}

// SaveConfig writes a declaration as indented JSON to a path that must not
// exist yet, so a saved vocabulary is never silently replaced by a different
// one. A declaration New would reject is refused instead of written, because a
// file no vocabulary can be built from is not a record of anything.
func SaveConfig(path string, c Config) error {
	if err := validate(c); err != nil {
		return fmt.Errorf("tokenizer: %s: %w", path, err)
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(normalise(c)); err != nil {
		return fmt.Errorf("tokenizer: %s: encoding the declaration failed: %w", path, err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("tokenizer: %s: %w", path, err)
	}
	if _, err := file.Write(buf.Bytes()); err != nil {
		file.Close()
		return fmt.Errorf("tokenizer: %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("tokenizer: %s: %w", path, err)
	}
	return nil
}

// LoadConfig reads a declaration back. Decoding is strict: an unknown field, a
// duplicate key, trailing data or a file over MaxConfigBytes is an error, and
// so is a declaration New would reject, so a bad file is named at its source
// rather than at the first use of the vocabulary.
func LoadConfig(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("tokenizer: %w", err)
	}
	defer file.Close()

	var c Config
	if err := strictjson.Decode(file, MaxConfigBytes, &c); err != nil {
		return Config{}, fmt.Errorf("tokenizer: %s: %w", path, err)
	}
	if err := validate(c); err != nil {
		return Config{}, fmt.Errorf("tokenizer: %s: %w", path, err)
	}
	return normalise(c), nil
}

// validate reports the first reason a declaration cannot become a vocabulary.
// Its messages carry no package prefix; each caller adds its own context.
func validate(c Config) error {
	if c.Format != FormatByte {
		return fmt.Errorf("vocabulary format %q is not %q", c.Format, FormatByte)
	}
	seen := make(map[string]struct{}, len(c.Specials))
	for i, s := range c.Specials {
		if s.Name == "" {
			return fmt.Errorf("special token %d has an empty name", i)
		}
		if !knownRole(s.Role) {
			return fmt.Errorf("special token %q declares role %q, want one of %s", s.Name, s.Role, strings.Join(roleOrder, ", "))
		}
		if _, duplicate := seen[s.Name]; duplicate {
			return fmt.Errorf("special token %q is declared twice", s.Name)
		}
		seen[s.Name] = struct{}{}
	}
	return nil
}

// knownRole reports whether role is one of the declared roles.
func knownRole(role string) bool {
	for _, known := range roleOrder {
		if role == known {
			return true
		}
	}
	return false
}

// normalise copies a declaration into a shape that hashes the same way however
// it reached us: the special list gets a slice of its own, and an absent list
// and an empty list both become an empty slice rather than JSON null.
func normalise(c Config) Config {
	out := Config{Format: c.Format, Specials: make([]SpecialToken, len(c.Specials))}
	copy(out.Specials, c.Specials)
	return out
}

// canonicalJSON renders the one form the VocabHash is taken over: compact
// JSON, struct fields in declaration order, specials in declared order and no
// HTML escaping, so a name holding '<' hashes as the name it is.
func canonicalJSON(c Config) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(c); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// hashConfig is the VocabHash of a normalised declaration.
func hashConfig(c Config) (string, error) {
	data, err := canonicalJSON(c)
	if err != nil {
		return "", fmt.Errorf("tokenizer: hashing the vocabulary declaration failed: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
