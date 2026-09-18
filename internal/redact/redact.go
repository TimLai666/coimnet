// Package redact masks secrets and raw modality payloads in log lines.
package redact

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"regexp"
	"strconv"
)

var (
	reSecret   = regexp.MustCompile(`sk-[A-Za-z0-9_-]{8,}`)
	reBearer   = regexp.MustCompile(`Bearer \S+`)
	reKeyVal   = regexp.MustCompile(`(?i)(token|api_key|apikey)=([^&\s"]+)`)
	reJSON     = regexp.MustCompile(`("(?:token|api_key|secret|password|raw_text|audio|image)"\s*:\s*)"((?:[^"\\]|\\.)*)"`)
	reModality = regexp.MustCompile(`"(?:raw_text|audio|image)"\s*:\s*`)
)

// Line returns s with secrets and raw modality payloads masked, in this order:
//  1. `sk-` followed by 8 or more of [A-Za-z0-9_-]          → `sk-[redacted]`
//  2. `Bearer ` followed by one non-space token             → `Bearer [redacted]`
//  3. `token=` / `api_key=` / `apikey=` (case-insensitive key) followed by a non-space, non-`&`, non-`"` run → `<key>=[redacted]`
//  4. JSON string members "token", "api_key", "secret", "password" → the value replaced by "[redacted]"
//  5. JSON string members "raw_text", "audio", "image" → the value replaced by "[len=N sha256=XXXXXXXX]" where N is the byte length of the decoded string and XXXXXXXX the first 8 lowercase hex characters of its SHA-256 (the doc says the length and the hash prefix are all that is kept)
//
// The output never contains the original secret. Lines without a match are
// returned unchanged, byte for byte.
func Line(s string) string {
	s = reSecret.ReplaceAllString(s, "sk-[redacted]")
	s = reBearer.ReplaceAllString(s, "Bearer [redacted]")
	s = reKeyVal.ReplaceAllString(s, `${1}=[redacted]`)
	return reJSON.ReplaceAllStringFunc(s, func(m string) string {
		groups := reJSON.FindStringSubmatch(m)
		prefix, value := groups[1], groups[2]
		if reModality.MatchString(prefix) {
			return prefix + maskModality(value)
		}
		return prefix + `"[redacted]"`
	})
}

// maskModality returns the masked replacement for the decoded JSON string
// value caught in capture group 2.
func maskModality(value string) string {
	decoded, err := strconv.Unquote(`"` + value + `"`)
	if err != nil {
		decoded = value
	}
	sum := sha256.Sum256([]byte(decoded))
	return `"[len=` + strconv.Itoa(len(decoded)) +
		` sha256=` + hex.EncodeToString(sum[:4]) + `]"`
}

// Writer masks every complete line written through it and forwards it to w.
// A partial trailing line is held until the next newline or Flush, so a
// secret split across two Write calls is still masked. Write returns len(p)
// on success.
type Writer struct {
	w   io.Writer
	buf []byte
}

// NewWriter returns a Writer that masks lines and forwards them to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// Write accepts p, buffers it, and forwards every complete line (masked) to
// the underlying writer. It returns len(p) after accepting all input, or a
// non-nil error if forwarding a complete line fails.
func (wr *Writer) Write(p []byte) (int, error) {
	wr.buf = append(wr.buf, p...)
	for {
		idx := bytes.IndexByte(wr.buf, '\n')
		if idx < 0 {
			break
		}
		if _, err := io.WriteString(wr.w, Line(string(wr.buf[:idx+1]))); err != nil {
			return 0, err
		}
		wr.buf = wr.buf[idx+1:]
	}
	return len(p), nil
}

// Flush masks and forwards any partial trailing line held in the buffer.
func (wr *Writer) Flush() error {
	if len(wr.buf) == 0 {
		return nil
	}
	if _, err := io.WriteString(wr.w, Line(string(wr.buf))); err != nil {
		return err
	}
	wr.buf = wr.buf[:0]
	return nil
}
