package tokenizer_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/tasks/textgen/tokenizer"
)

// bosEos is the two-special declaration the specials, hash and stream tests
// share: ids 256 (bos) and 257 (eos) sit right after the 256 byte ids. The
// names are the strings Special looks up, which is why they are not decorated.
func bosEos() tokenizer.Config {
	return tokenizer.Config{
		Format: tokenizer.FormatByte,
		Specials: []tokenizer.SpecialToken{
			{Name: "bos", Role: "bos"},
			{Name: "eos", Role: "eos"},
		},
	}
}

// newVocab builds a vocabulary and fails the test if the declaration is
// rejected, so each test body can stay on its own subject.
func newVocab(t *testing.T, c tokenizer.Config) *tokenizer.ByteVocab {
	t.Helper()
	v, err := tokenizer.New(c)
	if err != nil {
		t.Fatalf("New(%+v) returned error: %v", c, err)
	}
	return v
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	v := newVocab(t, bosEos())
	const text = "台灣 test 🚀\n"

	ids, err := v.Encode(text)
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	if want := len([]byte(text)); len(ids) != want {
		t.Fatalf("Encode produced %d ids, want one per byte (%d)", len(ids), want)
	}
	for i, id := range ids {
		if id < 0 || id > 255 {
			t.Fatalf("id %d at position %d is not a byte id", id, i)
		}
		if byte(id) != text[i] {
			t.Fatalf("id %d at position %d encodes byte %#x, want %#x", id, i, byte(id), text[i])
		}
	}

	back, err := v.Decode(ids)
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}
	if back != text {
		t.Fatalf("round trip produced %q, want %q", back, text)
	}
}

func TestSpecialsAreExplicit(t *testing.T) {
	v := newVocab(t, bosEos())

	if got := v.Size(); got != 258 {
		t.Fatalf("Size() = %d, want 258", got)
	}
	id, ok := v.Special("eos")
	if !ok {
		t.Fatal(`Special("eos") reported not found`)
	}
	if id != 257 {
		t.Fatalf(`Special("eos") = %d, want 257`, id)
	}
	if _, ok := v.Special("nope"); ok {
		t.Fatal(`Special("nope") reported found`)
	}

	ids, err := v.Encode("<eos>")
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	if len(ids) != 5 {
		t.Fatalf(`Encode("<eos>") produced %d ids, want the 5 literal bytes`, len(ids))
	}
	for i, got := range ids {
		if want := int("<eos>"[i]); got != want {
			t.Fatalf("id %d at position %d, want byte id %d", got, i, want)
		}
	}

	plain, err := v.Encode("eos")
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	if len(plain) != 3 || plain[0] != 'e' || plain[1] != 'o' || plain[2] != 's' {
		t.Fatalf(`Encode("eos") produced %v, want the 3 literal bytes`, plain)
	}

	text, err := v.Decode([]int{257})
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}
	if text != "" {
		t.Fatalf("Decode([257]) = %q, want the empty string", text)
	}
}

// isLowerHex reports whether s is exactly n lowercase hexadecimal digits.
func isLowerHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func TestHashIsStableAndSensitive(t *testing.T) {
	first := newVocab(t, bosEos()).Hash()
	second := newVocab(t, bosEos()).Hash()
	if first != second {
		t.Fatalf("the same declaration hashed to %q then %q", first, second)
	}
	if !isLowerHex(first, 64) {
		t.Fatalf("Hash() = %q, want 64 lowercase hexadecimal digits", first)
	}

	swapped := bosEos()
	swapped.Specials[0], swapped.Specials[1] = swapped.Specials[1], swapped.Specials[0]
	if got := newVocab(t, swapped).Hash(); got == first {
		t.Fatalf("swapping the special order kept the hash %q", got)
	}

	path := filepath.Join(t.TempDir(), "vocab.json")
	if err := tokenizer.SaveConfig(path, bosEos()); err != nil {
		t.Fatalf("SaveConfig returned error: %v", err)
	}
	loaded, err := tokenizer.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if got := newVocab(t, loaded).Hash(); got != first {
		t.Fatalf("hash after save and load is %q, want %q", got, first)
	}
}

func TestNewRejects(t *testing.T) {
	cases := []struct {
		name   string
		config tokenizer.Config
	}{
		{"wrong format", tokenizer.Config{Format: "other/v1", Specials: []tokenizer.SpecialToken{{Name: "eos", Role: "eos"}}}},
		{"empty format", tokenizer.Config{Specials: []tokenizer.SpecialToken{{Name: "eos", Role: "eos"}}}},
		{"duplicate name", tokenizer.Config{Format: tokenizer.FormatByte, Specials: []tokenizer.SpecialToken{
			{Name: "eos", Role: "eos"},
			{Name: "eos", Role: "pad"},
		}}},
		{"unknown role", tokenizer.Config{Format: tokenizer.FormatByte, Specials: []tokenizer.SpecialToken{{Name: "start", Role: "start"}}}},
		{"empty role", tokenizer.Config{Format: tokenizer.FormatByte, Specials: []tokenizer.SpecialToken{{Name: "pad", Role: ""}}}},
		{"empty name", tokenizer.Config{Format: tokenizer.FormatByte, Specials: []tokenizer.SpecialToken{{Name: "", Role: "eos"}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := tokenizer.New(c.config); err == nil {
				t.Fatalf("New(%+v) returned no error", c.config)
			}
		})
	}
}

func TestStreamDecoderBuffersIncompleteUTF8(t *testing.T) {
	v := newVocab(t, bosEos())
	d := tokenizer.NewStreamDecoder(v)

	raw := []byte("台")
	if len(raw) != 3 {
		t.Fatalf(`"台" is %d bytes, want 3`, len(raw))
	}
	for i, b := range raw {
		out, err := d.Feed(int(b))
		if err != nil {
			t.Fatalf("Feed(%d) returned error: %v", b, err)
		}
		want := ""
		if i == 2 {
			want = "台"
		}
		if out != want {
			t.Fatalf("Feed of byte %d produced %q, want %q", i, out, want)
		}
	}

	out, err := d.Feed(257)
	if err != nil {
		t.Fatalf("Feed(257) returned error: %v", err)
	}
	if out != "" {
		t.Fatalf("Feed(257) produced %q, want the empty string", out)
	}

	if _, err := d.Feed(999); err == nil {
		t.Fatal("Feed(999) returned no error for an unknown id")
	}
	if _, err := d.Feed(-1); err == nil {
		t.Fatal("Feed(-1) returned no error for an unknown id")
	}
}

func TestStreamDecoderFlushRefusesIncomplete(t *testing.T) {
	v := newVocab(t, bosEos())
	d := tokenizer.NewStreamDecoder(v)

	raw := []byte("台")
	for _, b := range raw[:2] {
		out, err := d.Feed(int(b))
		if err != nil {
			t.Fatalf("Feed(%d) returned error: %v", b, err)
		}
		if out != "" {
			t.Fatalf("Feed produced %q before the sequence was complete", out)
		}
	}

	out, err := d.Flush()
	if err == nil {
		t.Fatal("Flush returned no error while an incomplete sequence was buffered")
	}
	if !strings.Contains(err.Error(), "incomplete UTF-8") {
		t.Fatalf("Flush error %q does not name the incomplete UTF-8 sequence", err)
	}
	if out != "" {
		t.Fatalf("Flush produced %q with an incomplete sequence buffered, want the empty string", out)
	}

	tail, err := d.Feed(int(raw[2]))
	if err != nil {
		t.Fatalf("Feed returned error: %v", err)
	}
	if tail != "台" {
		t.Fatalf("Feed of the final byte produced %q, want %q", tail, "台")
	}

	out, err = d.Flush()
	if err != nil {
		t.Fatalf("Flush returned error after the sequence completed: %v", err)
	}
	if out != "" {
		t.Fatalf("Flush produced %q, want the empty string", out)
	}
}

func TestLoadConfigRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown.json")
	raw := `{"format":"` + tokenizer.FormatByte + `","specials":[{"name":"eos","role":"eos"}],"vocab_size":258}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("writing the fixture failed: %v", err)
	}

	if _, err := tokenizer.LoadConfig(path); err == nil {
		t.Fatal("LoadConfig accepted an unknown field")
	}

	good := filepath.Join(dir, "good.json")
	if err := tokenizer.SaveConfig(good, bosEos()); err != nil {
		t.Fatalf("SaveConfig returned error: %v", err)
	}
	data, err := os.ReadFile(good)
	if err != nil {
		t.Fatalf("reading the saved config failed: %v", err)
	}
	if !strings.Contains(string(data), "\n  ") {
		t.Fatalf("SaveConfig wrote %q, want indented JSON", data)
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		t.Fatalf("the saved config is not a JSON object: %v", err)
	}
	for _, key := range []string{"format", "specials"} {
		if _, ok := probe[key]; !ok {
			t.Fatalf("the saved config has no %q field: %s", key, data)
		}
	}
}

func TestSaveConfigRefusesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vocab.json")
	if err := tokenizer.SaveConfig(path, bosEos()); err != nil {
		t.Fatalf("SaveConfig returned error: %v", err)
	}
	if err := tokenizer.SaveConfig(path, bosEos()); err == nil {
		t.Fatal("SaveConfig overwrote an existing file")
	}
}
