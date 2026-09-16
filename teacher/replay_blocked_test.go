package teacher_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/TimLai666/coimnet/signal"
	"github.com/TimLai666/coimnet/teacher"
)

// testRecordByRequestID builds a Record with an explicit RequestID and InputHash.
func testRecordByRequestID(requestID, inputHash string, ts int64) teacher.Record {
	return teacher.Record{
		SchemaVersion: teacher.RecordSchemaVersion,
		Request: teacher.Request{
			RequestID:    requestID,
			InputHash:    inputHash,
			Kind:         teacher.KindLabel,
			Fields:       map[string]string{"cls": "cat"},
			ModelVersion: "v1",
		},
		Response: teacher.Response{
			TeacherID:      "recorder",
			TeacherVersion: "v1",
			RequestID:      requestID,
			InputHash:      inputHash,
			AnswerKind:     teacher.KindLabel,
			Answer:         json.RawMessage(`"ok"`),
			Time:           signal.Timestamp{Value: ts, Unit: signal.TimeUnitModelStep},
		},
	}
}

func writeRecords(t *testing.T, records []teacher.Record) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "records.jsonl")
	for i, rec := range records {
		line, err := json.Marshal(rec)
		if err != nil {
			t.Fatalf("marshal record %d: %v", i, err)
		}
		line = append(line, '\n')
		if err := appendFile(path, line); err != nil {
			t.Fatalf("append record %d: %v", i, err)
		}
	}
	return path
}

func appendFile(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

func TestReplayAnswersSameRequestIDIdentically(t *testing.T) {
	recA := testRecordByRequestID("req-aaa", strings.Repeat("a", 64), 1)
	recB := testRecordByRequestID("req-bbb", strings.Repeat("b", 64), 2)
	path := writeRecords(t, []teacher.Record{recA, recB})

	r, err := teacher.NewReplay(path, "replay-test", "v1")
	if err != nil {
		t.Fatalf("NewReplay = %v", err)
	}

	// Ask with RequestID that matches recA → should return recA's response.
	incomingA := teacher.Request{
		RequestID:    "req-aaa",
		InputHash:    strings.Repeat("a", 64),
		Kind:         teacher.KindLabel,
		ModelVersion: "v1",
	}
	gotA, err := r.Ask(context.Background(), incomingA)
	if err != nil {
		t.Fatalf("Ask(req-aaa) = %v", err)
	}
	wantA := recA.Response
	if !reflect.DeepEqual(gotA, wantA) {
		t.Fatalf("Ask(req-aaa) = %+v, want %+v", gotA, wantA)
	}

	// Ask again with same RequestID → identical response.
	gotA2, err := r.Ask(context.Background(), incomingA)
	if err != nil {
		t.Fatalf("Ask(req-aaa) second = %v", err)
	}
	if !reflect.DeepEqual(gotA, gotA2) {
		t.Fatalf("Ask(req-aaa) = %+v on second call, want identical", gotA2)
	}

	// Unknown RequestID → ErrNoAnswer.
	unknown := teacher.Request{
		RequestID:    "req-zzz",
		InputHash:    strings.Repeat("c", 64),
		Kind:         teacher.KindLabel,
		ModelVersion: "v1",
	}
	_, err = r.Ask(context.Background(), unknown)
	if !errors.Is(err, teacher.ErrNoAnswer) {
		t.Fatalf("Ask(unknown) = %v, want ErrNoAnswer", err)
	}
}

func TestReplayNeverFetches(t *testing.T) {
	old := http.DefaultTransport
	http.DefaultTransport = errorTransport{}
	defer func() { http.DefaultTransport = old }()

	rec := testRecordByRequestID("req-net", strings.Repeat("e", 64), 4)
	path := writeRecords(t, []teacher.Record{rec})

	r, err := teacher.NewReplay(path, "replay-net", "v1")
	if err != nil {
		t.Fatalf("NewReplay = %v", err)
	}

	incoming := teacher.Request{
		RequestID:    "req-net",
		InputHash:    strings.Repeat("e", 64),
		Kind:         teacher.KindLabel,
		ModelVersion: "v1",
	}
	got, err := r.Ask(context.Background(), incoming)
	if err != nil {
		t.Fatalf("Ask(hit) = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, rec.Response) {
		t.Fatalf("Ask(hit) = %+v, want %+v", got, rec.Response)
	}
}

func TestReplayDescribe(t *testing.T) {
	rec := testRecordByRequestID("req-d", strings.Repeat("d", 64), 5)
	path := writeRecords(t, []teacher.Record{rec})

	r, err := teacher.NewReplay(path, "my-replay", "v2")
	if err != nil {
		t.Fatalf("NewReplay = %v", err)
	}
	d := r.Describe()
	if d.ID != "my-replay" || d.Version != "v2" || d.Kind != teacher.DescriptorReplay {
		t.Fatalf("Describe() = %+v, want {my-replay v2 replay}", d)
	}
}

func TestBlockedRefusesAndCounts(t *testing.T) {
	b := teacher.Blocked{ID: "blk", Version: "v1"}

	req := teacher.Request{
		RequestID:    "req-x",
		InputHash:    strings.Repeat("a", 64),
		Kind:         teacher.KindLabel,
		ModelVersion: "v1",
	}

	for i := 0; i < 3; i++ {
		_, err := b.Ask(context.Background(), req)
		if !errors.Is(err, teacher.ErrTeacherBlocked) {
			t.Fatalf("Ask #%d = %v, want ErrTeacherBlocked", i+1, err)
		}
	}
	if got := b.Calls(); got != 3 {
		t.Fatalf("Calls() = %d, want 3", got)
	}

	// Concurrent calls.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := b.Ask(context.Background(), req)
			if !errors.Is(err, teacher.ErrTeacherBlocked) {
				t.Errorf("Ask(concurrent) = %v, want ErrTeacherBlocked", err)
			}
		}()
	}
	wg.Wait()
	if got := b.Calls(); got != 13 {
		t.Fatalf("Calls() after concurrent = %d, want 13", got)
	}
}

func TestBlockedDescribe(t *testing.T) {
	b := teacher.Blocked{ID: "blk-d", Version: "v3"}
	d := b.Describe()
	if d.ID != "blk-d" || d.Version != "v3" || d.Kind != teacher.DescriptorBlocked {
		t.Fatalf("Describe() = %+v, want {blk-d v3 blocked}", d)
	}
}
