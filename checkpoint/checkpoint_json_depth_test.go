package checkpoint

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRejectsExcessiveJSONNestingWithLongKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deep.json")
	if err := os.WriteFile(path, nestedCheckpointJSON(65, 512), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(context.Background(), path)
	if err == nil || !strings.Contains(err.Error(), "JSON nesting exceeds 64 levels") {
		t.Fatalf("Load() error = %v, want bounded nesting error", err)
	}
}

func TestLoadRejectsCaseFoldedDuplicateKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "duplicate.json")
	input := `{"schema_version":"coimnet-episode-checkpoint/v1","Schema_Version":"coimnet-episode-checkpoint/v1"}`
	if err := os.WriteFile(path, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(context.Background(), path)
	if err == nil || !strings.Contains(err.Error(), "duplicate JSON key") {
		t.Fatalf("Load() error = %v, want case-folded duplicate error", err)
	}
}

func TestLoadRejectsUnicodeCaseFoldedDuplicateKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unicode-duplicate.json")
	input := `{"schema_version":"coimnet-episode-checkpoint/v1","ſchema_version":"coimnet-episode-checkpoint/v1"}`
	if err := os.WriteFile(path, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(context.Background(), path)
	if err == nil || !strings.Contains(err.Error(), "duplicate JSON key") {
		t.Fatalf("Load() error = %v, want Unicode case-folded duplicate error", err)
	}
}

func nestedCheckpointJSON(depth, keyLength int) []byte {
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
	return []byte(builder.String())
}
