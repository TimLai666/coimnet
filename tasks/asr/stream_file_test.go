package asr_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/tasks/asr"
)

func TestStreamFileResumesExactlyAndDoesNotOverwrite(t *testing.T) {
	ctx := context.Background()
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatal(err)
	}
	x, err := asr.Synthesize(synthConfig(), "ab", 17)
	if err != nil {
		t.Fatal(err)
	}
	split := recognizerConfig().FrontEnd.Window + 23
	continuous, err := r.NewStream()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := continuous.Feed(ctx, x[:split]); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "stream.json")
	if err := continuous.Save(ctx, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := continuous.Save(ctx, path); !errors.Is(err, os.ErrExist) {
		t.Fatalf("second Save = %v, want os.ErrExist", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("existing file changed after failed Save: %v", err)
	}

	restored, err := r.LoadStream(ctx, path)
	if err != nil {
		t.Fatalf("LoadStream: %v", err)
	}
	if !reflect.DeepEqual(restored.Snapshot(), continuous.Snapshot()) {
		t.Fatal("loaded snapshot differs before continuation")
	}
	wrongConfig := recognizerConfig()
	wrongConfig.SampleRate = 18000
	wrong, err := asr.NewRecognizer(wrongConfig)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrong.LoadStream(ctx, path); err == nil {
		t.Fatal("LoadStream accepted a different sample rate")
	}
	for _, stream := range []*asr.Stream{continuous, restored} {
		if _, err := stream.Feed(ctx, x[split:]); err != nil {
			t.Fatalf("continued Feed: %v", err)
		}
		if _, err := stream.Flush(ctx); err != nil {
			t.Fatalf("continued Flush: %v", err)
		}
	}
	if !reflect.DeepEqual(restored.Snapshot(), continuous.Snapshot()) {
		t.Fatal("loaded stream diverged after continuation")
	}
}

func TestStreamFileRejectsTamperingEvenWithFreshChecksum(t *testing.T) {
	ctx := context.Background()
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatal(err)
	}
	s, err := r.NewStream()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Feed(ctx, []float64{0.125, -0.25}); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(t.TempDir(), "valid.json")
	if err := s.Save(ctx, base); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(base)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		mutate      func(map[string]json.RawMessage)
		wantMessage string
	}{
		{"missing sample rate", func(p map[string]json.RawMessage) { delete(p, "sample_rate") }, ""},
		{"null sample rate", func(p map[string]json.RawMessage) { p["sample_rate"] = json.RawMessage("null") }, ""},
		{"null pending element", func(p map[string]json.RawMessage) { p["pending"] = json.RawMessage("[null, -0.25]") }, ""},
		{"missing flushed flag", func(p map[string]json.RawMessage) { delete(p, "flushed") }, ""},
		{"null alphabet element", func(p map[string]json.RawMessage) { p["alphabet"] = json.RawMessage("[null, 98, 99]") }, ""},
		{"unknown field", func(p map[string]json.RawMessage) { p["unexpected"] = json.RawMessage("1") }, ""},
		{"pending count checked before decode", func(p map[string]json.RawMessage) {
			p["pending"], _ = json.Marshal(make([]float64, recognizerConfig().FrontEnd.Window))
		}, "pending exceeds"},
		{"missing front end field", func(p map[string]json.RawMessage) {
			var front map[string]json.RawMessage
			if err := json.Unmarshal(p["front_end"], &front); err != nil {
				panic(err)
			}
			delete(front, "window")
			p["front_end"], _ = json.Marshal(front)
		}, ""},
		{"null front end field", func(p map[string]json.RawMessage) {
			var front map[string]json.RawMessage
			if err := json.Unmarshal(p["front_end"], &front); err != nil {
				panic(err)
			}
			front["window"] = json.RawMessage("null")
			p["front_end"], _ = json.Marshal(front)
		}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var outer struct {
				Schema   string          `json:"schema_version"`
				Payload  json.RawMessage `json:"payload"`
				Checksum string          `json:"checksum"`
			}
			if err := json.Unmarshal(data, &outer); err != nil {
				t.Fatal(err)
			}
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(outer.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			tc.mutate(payload)
			outer.Payload, err = json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(outer.Payload)
			outer.Checksum = hex.EncodeToString(sum[:])
			bad, err := json.Marshal(outer)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "tampered.json")
			if err := os.WriteFile(path, bad, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := r.LoadStream(ctx, path); err == nil {
				t.Fatal("LoadStream accepted tampered state")
			} else if tc.wantMessage != "" && !strings.Contains(err.Error(), tc.wantMessage) {
				t.Fatalf("LoadStream error %q does not contain %q", err, tc.wantMessage)
			}
		})
	}
}

func TestStreamFileRejectsCanceledAndOversizeRead(t *testing.T) {
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatal(err)
	}
	s, err := r.NewStream()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "canceled.json")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Save(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("Save canceled = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled Save left file: %v", err)
	}
	if _, err := r.LoadStream(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("Load canceled = %v", err)
	}

	oversize := filepath.Join(dir, "oversize.json")
	f, err := os.Create(oversize)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate((64 << 20) + 1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.LoadStream(context.Background(), oversize); err == nil {
		t.Fatal("LoadStream accepted oversized file")
	}
}

func TestStreamFileRejectsWrongSchemaAndChecksum(t *testing.T) {
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatal(err)
	}
	s, err := r.NewStream()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "good.json")
	if err := s.Save(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"schema_version", "checksum"} {
		t.Run(name, func(t *testing.T) {
			original := document[name]
			document[name] = json.RawMessage(`"wrong"`)
			bad, err := json.Marshal(document)
			document[name] = original
			if err != nil {
				t.Fatal(err)
			}
			badPath := filepath.Join(t.TempDir(), "bad.json")
			if err := os.WriteFile(badPath, bad, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := r.LoadStream(context.Background(), badPath); err == nil {
				t.Fatal("LoadStream accepted wrong envelope identity")
			}
		})
	}
}

func TestStreamFileAcrossProcess(t *testing.T) {
	ctx := context.Background()
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatal(err)
	}
	x, err := asr.Synthesize(synthConfig(), "abc", 31)
	if err != nil {
		t.Fatal(err)
	}
	split := recognizerConfig().FrontEnd.Window + 23
	s, err := r.NewStream()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Feed(ctx, x[:split]); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path, result := filepath.Join(dir, "stream.json"), filepath.Join(dir, "result.sha256")
	if err := s.Save(ctx, path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Feed(ctx, x[split:]); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(s.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(wantJSON)
	command := exec.Command(os.Args[0], "-test.run=^TestStreamFileChild$")
	command.Env = append(os.Environ(), "COIMNET_ASR_STREAM_TEST_PATH="+path, "COIMNET_ASR_STREAM_TEST_RESULT="+result)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("child resume: %v\n%s", err, output)
	}
	got, err := os.ReadFile(result)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != hex.EncodeToString(want[:]) {
		t.Fatalf("cross-process state hash %s, want %x", got, want)
	}
}

func TestStreamFileChild(t *testing.T) {
	path := os.Getenv("COIMNET_ASR_STREAM_TEST_PATH")
	if path == "" {
		return
	}
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatal(err)
	}
	s, err := r.LoadStream(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	x, err := asr.Synthesize(synthConfig(), "abc", 31)
	if err != nil {
		t.Fatal(err)
	}
	split := recognizerConfig().FrontEnd.Window + 23
	if _, err := s.Feed(context.Background(), x[split:]); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(s.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(encoded)
	result := os.Getenv("COIMNET_ASR_STREAM_TEST_RESULT")
	if err := os.WriteFile(result, []byte(fmt.Sprintf("%x", sum)), 0o600); err != nil {
		t.Fatal(err)
	}
}
