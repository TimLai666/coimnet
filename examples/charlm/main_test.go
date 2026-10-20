package main

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadTextSplitsJSONLByDocumentIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "docs.jsonl")
	var body []byte
	for i := range 203 {
		body = append(body, []byte(`{"text": " d`+string(rune('a'+i%26))+` "}`+"\n")...)
	}
	if err := os.WriteFile(path, append(body, `{"text": "cut`...), 0o644); err != nil {
		t.Fatal(err)
	}
	s, sum, err := readText(path, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	// Documents 0, 100 and 200 are test; 1, 101 and 201 validation.
	if string(s.test) != "da\ndw\nds\n" || string(s.val) != "db\ndx\ndt\n" || len(s.train) != 197*3 || len(sum) != 64 {
		t.Fatalf("test %q val %q train %d bytes", s.test, s.val, len(s.train))
	}
}

func TestReadTextCutsPlainText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.txt")
	text := make([]byte, 200)
	for i := range text {
		text[i] = byte('a' + i%26)
	}
	if err := os.WriteFile(path, text, 0o644); err != nil {
		t.Fatal(err)
	}
	s, _, err := readText(path, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.train, text[:180]) || !reflect.DeepEqual(s.val, text[180:190]) || !reflect.DeepEqual(s.test, text[190:]) {
		t.Fatalf("split %d/%d/%d", len(s.train), len(s.val), len(s.test))
	}
}

func TestVocabularyWindowTargetsTheNextByte(t *testing.T) {
	v := newVocabulary([]byte("cab"))
	if v.size() != 4 {
		t.Fatalf("size %d, want 3 bytes plus unknown", v.size())
	}
	ids := v.encode([]byte("abz"))
	if !reflect.DeepEqual(ids, []int{0, 1, 3}) || v.decode(ids) != "ab?" {
		t.Fatalf("ids %v decode %q", ids, v.decode(ids))
	}
	inputs, target := v.window(ids)
	wantIn := [][]float64{{1, 0, 0, 0}, {0, 1, 0, 0}}
	wantTarget := []float64{0, 1, 0, 0, 0, 0, 0, 1}
	if !reflect.DeepEqual(inputs, wantIn) || !reflect.DeepEqual(target, wantTarget) {
		t.Fatalf("inputs %v target %v", inputs, target)
	}
}

func TestEvalWindowsAreNonOverlappingAndFit(t *testing.T) {
	if got := evalWindows(make([]int, 10), 3, 5); !reflect.DeepEqual(got, []int{0, 3, 6}) {
		t.Fatalf("starts %v", got)
	}
	if got := evalWindows(make([]int, 10), 3, 2); !reflect.DeepEqual(got, []int{0, 3}) {
		t.Fatalf("capped starts %v", got)
	}
}

func TestLogSumExpIsStable(t *testing.T) {
	got := logSumExp([]float64{1000, 1000})
	if math.Abs(got-(1000+math.Ln2)) > 1e-9 {
		t.Fatalf("logSumExp %v", got)
	}
}
