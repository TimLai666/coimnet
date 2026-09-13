package signal

import (
	"strings"
	"testing"
)

func TestDecodeSignalRejectsExcessiveJSONNestingWithLongKeys(t *testing.T) {
	data := nestedJSONWithLongKeys(65, 512)
	_, err := DecodeSignal(strings.NewReader(data))
	if err == nil || !strings.Contains(err.Error(), "JSON nesting exceeds 64 levels") {
		t.Fatalf("DecodeSignal() error = %v, want bounded nesting error", err)
	}
}

func TestDecodeSignalRejectsCaseFoldedDuplicateKeys(t *testing.T) {
	input := `{"schema_version":{"major":1,"minor":0},"SCHEMA_VERSION":{"major":1,"minor":0}}`
	_, err := DecodeSignal(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "duplicate JSON field") {
		t.Fatalf("DecodeSignal() error = %v, want case-folded duplicate error", err)
	}
}

func TestDecodeSignalRejectsUnicodeCaseFoldedDuplicateKeys(t *testing.T) {
	input := `{"schema_version":{"major":1,"minor":0},"ſchema_version":{"major":1,"minor":0}}`
	_, err := DecodeSignal(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "duplicate JSON field") {
		t.Fatalf("DecodeSignal() error = %v, want Unicode case-folded duplicate error", err)
	}
}

func nestedJSONWithLongKeys(depth, keyLength int) string {
	key := strings.Repeat("k", keyLength)
	var builder strings.Builder
	for range depth {
		builder.WriteString(`{"`)
		builder.WriteString(key)
		builder.WriteString(`":`)
	}
	builder.WriteByte('0')
	for range depth {
		builder.WriteByte('}')
	}
	return builder.String()
}
