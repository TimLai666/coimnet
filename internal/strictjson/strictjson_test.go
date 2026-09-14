package strictjson

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type sample struct {
	SchemaVersion string   `json:"schema_version"`
	Count         int      `json:"count"`
	Names         []string `json:"names,omitempty"`
	Nested        *sample  `json:"nested,omitempty"`
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("reader failed") }

func TestDecodeAcceptsOneValueAndRejectsTheRest(t *testing.T) {
	var got sample
	if err := Decode(strings.NewReader(`{"schema_version":"v1","count":2,"names":["a","b"]}`), 1<<20, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.SchemaVersion != "v1" || got.Count != 2 || len(got.Names) != 2 || got.Names[1] != "b" {
		t.Fatalf("decoded = %#v", got)
	}

	var discard sample
	for name, input := range map[string]string{
		"unknown field": `{"schema_version":"v1","unexpected":true}`,
		"trailing data": `{"schema_version":"v1"} {}`,
		"trailing junk": `{"schema_version":"v1"} nope`,
		"empty input":   ``,
		"broken syntax": `{"schema_version":`,
	} {
		if err := Decode(strings.NewReader(input), 1<<20, &discard); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if err := Decode(nil, 1<<20, &discard); err == nil {
		t.Fatal("nil reader accepted")
	}
	var typedNil *bytes.Reader
	if err := Decode(typedNil, 1<<20, &discard); err == nil {
		t.Fatal("typed nil reader accepted")
	}
	if err := Decode(strings.NewReader(`{}`), 0, &discard); err == nil {
		t.Fatal("non-positive byte limit accepted")
	}
	if err := Decode(failingReader{}, 1<<20, &discard); err == nil || !strings.Contains(err.Error(), "reader failed") {
		t.Fatalf("reader failure = %v", err)
	}
}

func TestDecodeEnforcesTheByteLimitBeforeDecoding(t *testing.T) {
	payload := `{"schema_version":"v1","count":1}`
	var got sample
	if err := Decode(strings.NewReader(payload), int64(len(payload)), &got); err != nil {
		t.Fatalf("input of exactly the limit rejected: %v", err)
	}
	err := Decode(strings.NewReader(payload), int64(len(payload))-1, &got)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized input = %v", err)
	}
	// The limit must be applied to the stream, not only to a complete read.
	endless := io.MultiReader(strings.NewReader(`{"names":[`), strings.NewReader(strings.Repeat(`"x",`, 1000)))
	if err := Decode(endless, 64, &got); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("streamed oversized input = %v", err)
	}
}

func TestDecodeBoundsNestingDepthWithoutQuadraticWork(t *testing.T) {
	var discard any
	nested := strings.Repeat("[", 200000) + strings.Repeat("]", 200000)
	start := time.Now()
	err := Decode(strings.NewReader(nested), 1<<20, &discard)
	if err == nil || !strings.Contains(err.Error(), "nesting") {
		t.Fatalf("deeply nested JSON: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("deep nesting took %v", elapsed)
	}
	object := strings.Repeat(`{"k":`, 100) + "1" + strings.Repeat("}", 100)
	if err := Decode(strings.NewReader(object), 1<<20, &discard); err == nil || !strings.Contains(err.Error(), "nesting") {
		t.Fatalf("nested objects beyond the limit: %v", err)
	}
	// MaxDepth levels must still be accepted; the guard is off by nothing.
	atLimit := strings.Repeat(`{"k":`, MaxDepth) + "1" + strings.Repeat("}", MaxDepth)
	if err := Decode(strings.NewReader(atLimit), 1<<20, &discard); err != nil {
		t.Fatalf("nesting at exactly %d levels rejected: %v", MaxDepth, err)
	}
	tooDeep := strings.Repeat(`{"k":`, MaxDepth+2) + "1" + strings.Repeat("}", MaxDepth+2)
	if err := Decode(strings.NewReader(tooDeep), 1<<20, &discard); err == nil || !strings.Contains(err.Error(), "nesting") {
		t.Fatalf("nesting beyond %d levels: %v", MaxDepth, err)
	}
}

func TestDecodeRejectsDuplicateKeysIncludingUnicodeCaseAliases(t *testing.T) {
	var discard sample
	for name, input := range map[string]string{
		"exact duplicate":     `{"schema_version":"a","schema_version":"b"}`,
		"ascii case alias":    `{"schema_version":"a","Schema_Version":"b"}`,
		"unicode case alias":  `{"schema_version":"a","ſchema_version":"b"}`,
		"duplicate in nested": `{"nested":{"count":1,"COUNT":2}}`,
		"duplicate in array":  `{"names":[],"nested":{"schema_version":"a","SCHEMA_VERSION":"b"}}`,
	} {
		err := Decode(strings.NewReader(input), 1<<20, &discard)
		if err == nil || !strings.Contains(err.Error(), "duplicate") {
			t.Fatalf("%s = %v", name, err)
		}
	}
	// Distinct keys at the same depth and repeated keys in sibling objects stay legal.
	if err := Decode(strings.NewReader(`{"schema_version":"a","nested":{"schema_version":"b"}}`), 1<<20, &discard); err != nil {
		t.Fatalf("sibling reuse rejected: %v", err)
	}
}

func TestRejectDuplicateKeysReportsThePath(t *testing.T) {
	err := RejectDuplicateKeys([]byte(`{"nested":{"names":[{"count":1,"Count":2}]}}`))
	if err == nil {
		t.Fatal("duplicate accepted")
	}
	if !strings.Contains(err.Error(), "$.nested.names.0") {
		t.Fatalf("error %q does not name the duplicate path", err)
	}
}
