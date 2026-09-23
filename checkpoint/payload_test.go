package checkpoint_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/checkpoint"
)

func TestSaveLoadPayloadCanonicalizesBeforeChecksum(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pretty.json")
	input := []byte("{\n  \"message\": \"<tag>&字\",\n  \"count\": 1\n}\n")
	if err := checkpoint.SavePayload(context.Background(), path, "test-payload/v1", input); err != nil {
		t.Fatalf("SavePayload: %v", err)
	}
	got, err := checkpoint.LoadPayload(context.Background(), path, "test-payload/v1")
	if err != nil {
		t.Fatalf("LoadPayload: %v", err)
	}
	var wantValue, gotValue map[string]any
	if err := json.Unmarshal(input, &wantValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("payload changed: got %#v, want %#v", gotValue, wantValue)
	}
}
