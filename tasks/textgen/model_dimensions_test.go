package textgen

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/tasks/textgen/tokenizer"
)

const dimensionTestFloat64Bytes = 8

// dimensionTestVocabulary has the same lazy byte/special declaration as the
// public reproduction. It does not allocate storage proportional to Size.
type dimensionTestVocabulary struct {
	size int
}

var _ tokenizer.Vocabulary = dimensionTestVocabulary{}

func (v dimensionTestVocabulary) Encode(text string) ([]int, error) {
	ids := make([]int, len(text))
	for i := range ids {
		ids[i] = int(text[i])
	}
	return ids, nil
}

func (v dimensionTestVocabulary) Decode(ids []int) (string, error) {
	var out []byte
	for _, id := range ids {
		if id < 0 || id >= v.size {
			return "", fmt.Errorf("ID %d outside [0, %d)", id, v.size)
		}
		if id < 256 {
			out = append(out, byte(id))
		}
	}
	return string(out), nil
}

func (v dimensionTestVocabulary) Size() int {
	return v.size
}

func (v dimensionTestVocabulary) Special(name string) (int, bool) {
	value, ok := strings.CutPrefix(name, "extra:")
	if !ok {
		return 0, false
	}
	index, err := strconv.Atoi(value)
	if err != nil || index < 0 || index >= v.size-256 || strconv.Itoa(index) != value {
		return 0, false
	}
	return index + 256, true
}

func (v dimensionTestVocabulary) Hash() string {
	declaration := map[string]any{
		"schema": "test-lazy-byte-specials-v1", "size": v.size,
		"byte_ids": 256, "special_name": "extra:{decimal-index}",
	}
	raw, err := json.Marshal(declaration)
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func dimensionTestCanonicalHash(size int) string {
	// encoding/json sorts map keys, so this is the canonical JSON emitted by
	// dimensionTestVocabulary.Hash, expressed independently as a byte string.
	raw := []byte(fmt.Sprintf(`{"byte_ids":256,"schema":"test-lazy-byte-specials-v1","size":%d,"special_name":"extra:{decimal-index}"}`, size))
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func checkDimensionTestVocabulary(v dimensionTestVocabulary) error {
	if v.Size() < 256 {
		return fmt.Errorf("test vocabulary size %d is below its 256 byte ids", v.Size())
	}
	const text = "A貓<eos>🙂"
	ids, err := v.Encode(text)
	if err != nil {
		return fmt.Errorf("Encode: %w", err)
	}
	if len(ids) != len(text) {
		return fmt.Errorf("Encode returned %d ids for %d bytes", len(ids), len(text))
	}
	for i, id := range ids {
		if id != int(text[i]) || id < 0 || id >= 256 {
			return fmt.Errorf("byte id at offset %d = %d, want %d", i, id, text[i])
		}
	}
	decoded, err := v.Decode(ids)
	if err != nil {
		return fmt.Errorf("Decode encoded bytes: %w", err)
	}
	if decoded != text {
		return fmt.Errorf("Decode(Encode(%q)) = %q", text, decoded)
	}
	for _, id := range []int{-1, v.Size()} {
		if _, err := v.Decode([]int{id}); err == nil {
			return fmt.Errorf("Decode accepted invalid ID %d", id)
		}
	}
	if _, ok := v.Special("bos"); ok {
		return fmt.Errorf("Special(\"bos\") unexpectedly found")
	}
	if _, ok := v.Special("extra:-1"); ok {
		return fmt.Errorf("Special accepted a negative index")
	}
	if _, ok := v.Special("extra:00"); ok {
		return fmt.Errorf("Special accepted a non-canonical index")
	}
	if v.Size() == 256 {
		if _, ok := v.Special("extra:0"); ok {
			return fmt.Errorf("Special accepted an undeclared extra")
		}
	} else {
		for _, id := range []int{256, v.Size() - 1} {
			name := "extra:" + strconv.Itoa(id-256)
			got, ok := v.Special(name)
			if !ok || got != id {
				return fmt.Errorf("Special(%q) = (%d, %t), want (%d, true)", name, got, ok, id)
			}
			out, err := v.Decode([]int{id, 'A', id})
			if err != nil || out != "A" {
				return fmt.Errorf("Decode special %d = (%q, %v), want (A, nil)", id, out, err)
			}
		}
		if _, ok := v.Special("extra:" + strconv.Itoa(v.Size()-256)); ok {
			return fmt.Errorf("Special accepted an undeclared final index")
		}
	}
	if got, want := v.Hash(), dimensionTestCanonicalHash(v.Size()); got != want {
		return fmt.Errorf("Hash() = %s, want canonical declaration hash %s", got, want)
	}
	if len(v.Hash()) != sha256.Size*2 {
		return fmt.Errorf("Hash() length = %d, want %d", len(v.Hash()), sha256.Size*2)
	}
	return nil
}

func callDimensionTestNewModel(v tokenizer.Vocabulary, c ModelConfig) (model *Model, err error, recovered any) {
	defer func() {
		if value := recover(); value != nil {
			recovered = value
		}
	}()
	model, err = NewModel(v, c)
	return
}

func dimensionTestMaxInt() int {
	return int(^uint(0) >> 1)
}

func TestModelRejectsUnrepresentableVocabularyBeforeAllocation(t *testing.T) {
	maxInt := dimensionTestMaxInt()
	invalid := []struct {
		name  string
		size  int
		embed int
	}{
		{name: "max vocabulary embed 4", size: maxInt, embed: 4},
		{name: "max vocabulary embed 8", size: maxInt, embed: 8},
		{name: "max vocabulary embed 256", size: maxInt, embed: 256},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			vocab := dimensionTestVocabulary{size: tc.size}
			if err := checkDimensionTestVocabulary(vocab); err != nil {
				t.Fatal(err)
			}
			config := ModelConfig{Embed: tc.embed, Hidden: 4, Settle: 1, LearningRate: 0.01, Seed: 7}
			if err := config.Validate(); err != nil {
				t.Fatalf("test configuration is invalid: %v", err)
			}
			wrappedEncoderLength := tc.size * tc.embed
			if wrappedEncoderLength >= 0 {
				t.Fatalf("unsafe test case: wrapped encoder length = %d, want negative", wrappedEncoderLength)
			}

			model, err, recovered := callDimensionTestNewModel(vocab, config)
			if recovered != nil {
				t.Fatalf("NewModel panicked for unrepresentable encoder length: %v", recovered)
			}
			if model != nil {
				t.Fatal("NewModel returned a model for an unrepresentable vocabulary")
			}
			if err == nil {
				t.Fatal("NewModel returned nil error for an unrepresentable vocabulary")
			}
		})
	}

	for _, tc := range []struct {
		name string
		size int
	}{
		{name: "ordinary bytes", size: 256},
		{name: "ordinary specials", size: 260},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vocab := dimensionTestVocabulary{size: tc.size}
			if err := checkDimensionTestVocabulary(vocab); err != nil {
				t.Fatal(err)
			}
			config := ModelConfig{Embed: 4, Hidden: 4, Settle: 1, LearningRate: 0.01, Seed: 7}
			model, err, recovered := callDimensionTestNewModel(vocab, config)
			if recovered != nil {
				t.Fatalf("NewModel panicked for legal size %d: %v", tc.size, recovered)
			}
			if err != nil {
				t.Fatalf("NewModel returned error for legal size %d: %v", tc.size, err)
			}
			if model == nil {
				t.Fatal("NewModel returned nil model for a legal vocabulary")
			}
			if got := model.Config(); got != config {
				t.Fatalf("Config() = %+v, want %+v", got, config)
			}
			if got := model.Vocabulary().Size(); got != tc.size {
				t.Fatalf("model vocabulary size = %d, want %d", got, tc.size)
			}
			snapshot := model.Snapshot()
			if got, want := len(snapshot.Parameters.Encoder), tc.size*config.Embed; got != want {
				t.Fatalf("snapshot encoder length = %d, want %d", got, want)
			}
			if got, want := len(snapshot.Parameters.Readout), config.Hidden*tc.size; got != want {
				t.Fatalf("snapshot readout length = %d, want %d", got, want)
			}
			if got := model.Config(); got != config {
				t.Fatalf("Config() changed after Snapshot(): %+v, want %+v", got, config)
			}
		})
	}
}

func dimensionTestReferenceLength(left, right, maxInt int) (int, error) {
	product := new(big.Int).Mul(big.NewInt(int64(left)), big.NewInt(int64(right)))
	max := big.NewInt(int64(maxInt))
	if product.Sign() < 0 || product.Cmp(max) > 0 {
		return 0, fmt.Errorf("element count is not representable")
	}
	bytes := new(big.Int).Mul(product, big.NewInt(dimensionTestFloat64Bytes))
	if bytes.Cmp(max) > 0 {
		return 0, fmt.Errorf("float64 byte count is not representable")
	}
	return int(product.Int64()), nil
}

func dimensionTestReferenceParameterLengths(vocabSize, embed, hidden int) (encoder, readout int, err error) {
	if vocabSize < 2 {
		return 0, 0, fmt.Errorf("vocabulary size must be at least 2")
	}
	if embed <= 0 || hidden <= 0 {
		return 0, 0, fmt.Errorf("embedding and hidden widths must be positive")
	}
	maxInt := dimensionTestMaxInt()
	encoder, err = dimensionTestReferenceLength(vocabSize, embed, maxInt)
	if err != nil {
		return 0, 0, fmt.Errorf("encoder: %w", err)
	}
	readout, err = dimensionTestReferenceLength(hidden, vocabSize, maxInt)
	if err != nil {
		return 0, 0, fmt.Errorf("readout: %w", err)
	}
	return encoder, readout, nil
}

func TestModelParameterLengths(t *testing.T) {
	maxInt := dimensionTestMaxInt()
	maxFloat64Elements := maxInt / dimensionTestFloat64Bytes
	cases := []struct {
		name             string
		vocabSize, embed int
		hidden           int
	}{
		{name: "vocabulary zero", vocabSize: 0, embed: 4, hidden: 4},
		{name: "vocabulary one", vocabSize: 1, embed: 4, hidden: 4},
		{name: "vocabulary negative", vocabSize: -1, embed: 4, hidden: 4},
		{name: "embed zero", vocabSize: 2, embed: 0, hidden: 4},
		{name: "embed negative", vocabSize: 2, embed: -1, hidden: 4},
		{name: "hidden zero", vocabSize: 2, embed: 4, hidden: 0},
		{name: "hidden negative", vocabSize: 2, embed: 4, hidden: -1},
		{name: "minimum legal vocabulary", vocabSize: 2, embed: 1, hidden: 1},
		{name: "small byte vocabulary", vocabSize: 256, embed: 4, hidden: 4},
		{name: "small special vocabulary", vocabSize: 260, embed: 4, hidden: 4},
		{name: "maximum legal float64 byte boundary", vocabSize: maxFloat64Elements, embed: 1, hidden: 1},
		{name: "encoder just over float64 byte boundary", vocabSize: maxFloat64Elements + 1, embed: 1, hidden: 1},
		{name: "readout alone over float64 byte boundary", vocabSize: maxFloat64Elements, embed: 1, hidden: 2},
		{name: "maximum element count but byte count over", vocabSize: maxInt, embed: 1, hidden: 1},
		{name: "encoder element multiplication overflows", vocabSize: maxInt, embed: 4, hidden: 4},
		{name: "readout element multiplication overflows", vocabSize: 2, embed: 1, hidden: maxInt},
		{name: "readout element multiplication overflows with max vocabulary", vocabSize: maxInt, embed: 1, hidden: 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantEncoder, wantReadout, wantErr := dimensionTestReferenceParameterLengths(tc.vocabSize, tc.embed, tc.hidden)
			gotEncoder, gotReadout, gotErr := modelParameterLengths(tc.vocabSize, tc.embed, tc.hidden)
			if (gotErr == nil) != (wantErr == nil) {
				t.Fatalf("modelParameterLengths(%d, %d, %d) error = %v, want error presence %t; reference error = %v", tc.vocabSize, tc.embed, tc.hidden, gotErr, wantErr != nil, wantErr)
			}
			if wantErr != nil {
				return
			}
			if gotEncoder != wantEncoder || gotReadout != wantReadout {
				t.Fatalf("modelParameterLengths(%d, %d, %d) = (%d, %d), want (%d, %d)", tc.vocabSize, tc.embed, tc.hidden, gotEncoder, gotReadout, wantEncoder, wantReadout)
			}
		})
	}
}
