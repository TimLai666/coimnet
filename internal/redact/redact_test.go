package redact_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"math/rand"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/internal/redact"
)

func TestLineMasksSecrets(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"key sk-abcdefgh12345678 end", "key sk-[redacted] end"},
		{"Authorization: Bearer eyJhbGciOi.xx", "Authorization: Bearer [redacted]"},
		{"GET /x?token=abc123&y=1", "GET /x?token=[redacted]&y=1"},
		{"API_KEY=Q1w2E3", "API_KEY=[redacted]"},
		{`{"api_key": "abc", "n": 1}`, `{"api_key": "[redacted]", "n": 1}`},
		{`{"password":"p\"q"}`, `{"password":"[redacted]"}`},
		{"sk-short", "sk-short"},
	}
	for _, tc := range cases {
		if got := redact.Line(tc.in); got != tc.want {
			t.Errorf("Line(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLineHashesRawPayloads(t *testing.T) {
	sumHello := sha256.Sum256([]byte("hello"))
	prefixHello := hex.EncodeToString(sumHello[:4])
	sumEmpty := sha256.Sum256([]byte(""))
	prefixEmpty := hex.EncodeToString(sumEmpty[:4])
	cases := []struct {
		in   string
		want string
	}{
		{
			`{"raw_text": "hello", "k": 2}`,
			`{"raw_text": "[len=5 sha256=` + prefixHello + `]", "k": 2}`,
		},
		{
			`{"audio":""}`,
			`{"audio":"[len=0 sha256=` + prefixEmpty + `]"}`,
		},
	}
	for _, tc := range cases {
		if got := redact.Line(tc.in); got != tc.want {
			t.Errorf("Line(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLineLeavesCleanLinesUntouched(t *testing.T) {
	lines := []string{
		`{"name": "hello world", "count": 3, "nested": {"a": [1, 2]}}`,
		`{"raw_text": null}`,
		`{"token_reference": "sk-abc", "other": true}`,
		"just a plain log message",
		"",
	}
	for _, s := range lines {
		if got := redact.Line(s); got != s {
			t.Errorf("Line(%q) = %q, want byte-for-byte unchanged", s, got)
		}
	}
}

func TestWriterMasksAcrossWriteBoundaries(t *testing.T) {
	var out bytes.Buffer
	w := redact.NewWriter(&out)

	parts := []string{"a sk-abc", "defgh123", "45678 b\n"}
	for _, p := range parts {
		n, err := w.Write([]byte(p))
		if err != nil {
			t.Fatalf("Write(%q): %v", p, err)
		}
		if n != len(p) {
			t.Fatalf("Write(%q) = %d, want %d", p, n, len(p))
		}
	}
	if got := out.String(); got != "a sk-[redacted] b\n" {
		t.Fatalf("output = %q, want %q", got, "a sk-[redacted] b\n")
	}

	if _, err := w.Write([]byte("trailing sk-abcdefgh12345678 tail")); err != nil {
		t.Fatalf("trailing Write: %v", err)
	}
	if got := out.String(); strings.Contains(got, "sk-abcdefgh12345678") {
		t.Fatalf("secret emitted before Flush: %q", got)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	want := "a sk-[redacted] b\ntrailing sk-[redacted] tail"
	if got := out.String(); got != want {
		t.Fatalf("after Flush = %q, want %q", got, want)
	}
}

func TestWriterNeverEmitsTheSecret(t *testing.T) {
	const secret = "sk-abcdefgh12345678"
	const line = "keep " + secret + " masked " + secret + "\n"
	const masked = "keep sk-[redacted] masked sk-[redacted]\n"

	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 50; i++ {
		var out bytes.Buffer
		w := redact.NewWriter(&out)
		rest := line
		for rest != "" {
			cut := 1 + rng.Intn(len(rest))
			if _, err := w.Write([]byte(rest[:cut])); err != nil {
				t.Fatalf("iteration %d: Write: %v", i, err)
			}
			if strings.Contains(out.String(), secret) {
				t.Fatalf("iteration %d: secret emitted: %q", i, out.String())
			}
			rest = rest[cut:]
		}
		if err := w.Flush(); err != nil {
			t.Fatalf("iteration %d: Flush: %v", i, err)
		}
		if got := out.String(); got != masked {
			t.Fatalf("iteration %d: got %q, want %q", i, got, masked)
		}
	}
}
