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
	if err == nil || !strings.Contains(err.Error(), "duplicate JSON key") {
		t.Fatalf("DecodeSignal() error = %v, want case-folded duplicate error", err)
	}
}

func TestDecodeSignalRejectsUnicodeCaseFoldedDuplicateKeys(t *testing.T) {
	input := `{"schema_version":{"major":1,"minor":0},"ſchema_version":{"major":1,"minor":0}}`
	_, err := DecodeSignal(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "duplicate JSON key") {
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

// TestDecodeSignalBoundsNestingAtSixtyFourLevels pins both sides of the bound:
// 65 levels must be refused by name, and 64 levels must reach the schema so the
// depth guard cannot quietly become the reason every deep document is refused.
func TestDecodeSignalBoundsNestingAtSixtyFourLevels(t *testing.T) {
	overDepth := nestedJSONWithLongKeys(65, 8)
	_, err := DecodeSignal(strings.NewReader(overDepth))
	if err == nil || !strings.Contains(err.Error(), "JSON nesting exceeds 64 levels") {
		t.Fatalf("65-level DecodeSignal() error = %v, want a nesting error naming the 64 level bound", err)
	}

	// 64 levels is inside the bound, so the walk accepts it and the Signal
	// schema is what refuses it: the outermost key is not a Signal field.
	atDepth := nestedJSONWithLongKeys(64, 8)
	_, err = DecodeSignal(strings.NewReader(atDepth))
	if err == nil || strings.Contains(err.Error(), "JSON nesting") || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("64-level DecodeSignal() error = %v, want an unknown-field error and no nesting error", err)
	}
}
