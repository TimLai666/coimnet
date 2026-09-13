package download

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchRejectsExcessiveMetadataJSONNestingWithLongKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("request reached before malformed metadata was rejected: %s", r.Method)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "deep.bin")
	if err := os.WriteFile(path+partSuffix, []byte(""), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+metadataSuffix, nestedMetadataJSON(65, 512), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := Fetch(context.Background(), server.Client(), server.URL, path, Options{MaxBytes: 1})
	if err == nil || !strings.Contains(err.Error(), "JSON nesting exceeds 64 levels") {
		t.Fatalf("Fetch() error = %v, want bounded nesting error", err)
	}
}

func TestFetchRejectsCaseFoldedDuplicateMetadataKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("request reached before malformed metadata was rejected: %s", r.Method)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "duplicate.bin")
	if err := os.WriteFile(path+partSuffix, nil, 0600); err != nil {
		t.Fatal(err)
	}
	input := `{"schema_version":"coimnet-download-part/v1","SCHEMA_VERSION":"coimnet-download-part/v1","url":"` + server.URL + `","etag":"\"v1\"","length":0,"bytes":0}`
	if err := os.WriteFile(path+metadataSuffix, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := Fetch(context.Background(), server.Client(), server.URL, path, Options{MaxBytes: 1})
	if err == nil || !strings.Contains(err.Error(), "duplicate JSON key") {
		t.Fatalf("Fetch() error = %v, want case-folded duplicate error", err)
	}
}

func TestFetchRejectsUnicodeCaseFoldedDuplicateMetadataKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("request reached before malformed metadata was rejected: %s", r.Method)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "unicode-duplicate.bin")
	if err := os.WriteFile(path+partSuffix, nil, 0600); err != nil {
		t.Fatal(err)
	}
	input := `{"schema_version":"coimnet-download-part/v1","ſchema_version":"coimnet-download-part/v1","url":"` + server.URL + `","etag":"\"v1\"","length":0,"bytes":0}`
	if err := os.WriteFile(path+metadataSuffix, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := Fetch(context.Background(), server.Client(), server.URL, path, Options{MaxBytes: 1})
	if err == nil || !strings.Contains(err.Error(), "duplicate JSON key") {
		t.Fatalf("Fetch() error = %v, want Unicode case-folded duplicate error", err)
	}
}

func nestedMetadataJSON(depth, keyLength int) []byte {
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
