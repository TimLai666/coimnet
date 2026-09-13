package jsonkey

import (
	"strings"
	"testing"
)

func TestFoldMatchesUnicodeCaseAliases(t *testing.T) {
	for _, pair := range [][2]string{{"schema_version", "ſCHEMA_VERSION"}, {"key", "Key"}, {"Σ", "ς"}, {"Σ", "σ"}, {"Name", "name"}, {"field", "fields"}, {"é", "é"}, {"輸入", "輸出"}} {
		want := strings.EqualFold(pair[0], pair[1])
		if got := Fold(pair[0]) == Fold(pair[1]); got != want {
			t.Fatalf("fold equality for %q: got %v want %v", pair, got, want)
		}
	}
}
