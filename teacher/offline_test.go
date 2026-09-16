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
	"testing"

	"github.com/TimLai666/coimnet/signal"
	"github.com/TimLai666/coimnet/teacher"
)

func testRecord(requestID, inputHash string, ts int64) teacher.Record {
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

func TestRecordStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.jsonl")
	s, err := teacher.OpenRecordStore(path, nil)
	if err != nil {
		t.Fatalf("OpenRecordStore(missing file) = %v, want nil", err)
	}
	if got := s.Len(); got != 0 {
		t.Fatalf("Len() of empty store = %d, want 0", got)
	}
	ra := testRecord("req-a", strings.Repeat("a", 64), 1)
	rb := testRecord("req-b", strings.Repeat("b", 64), 2)
	if err := s.Append(ra); err != nil {
		t.Fatalf("Append(ra) = %v, want nil", err)
	}
	if err := s.Append(rb); err != nil {
		t.Fatalf("Append(rb) = %v, want nil", err)
	}
	s2, err := teacher.OpenRecordStore(path, nil)
	if err != nil {
		t.Fatalf("OpenRecordStore(reopen) = %v, want nil", err)
	}
	if got := s2.Len(); got != 2 {
		t.Fatalf("Len() after reopen = %d, want 2", got)
	}
	got, ok := s2.Lookup(strings.Repeat("b", 64))
	if !ok {
		t.Fatalf("Lookup(b...) = ok=false, want true")
	}
	if !reflect.DeepEqual(got, rb) {
		t.Fatalf("Lookup(b...) = %+v, want %+v", got, rb)
	}
	if _, ok := s2.Lookup(strings.Repeat("c", 64)); ok {
		t.Fatalf("Lookup(missing hash) = ok=true, want false")
	}
}

func TestRecordStoreRefusesTestInputs(t *testing.T) {
	holdout := strings.Repeat("c", 64)
	path := filepath.Join(t.TempDir(), "records.jsonl")
	s, err := teacher.OpenRecordStore(path, nil)
	if err != nil {
		t.Fatalf("OpenRecordStore = %v, want nil", err)
	}
	if err := s.Append(testRecord("req-held", holdout, 1)); err != nil {
		t.Fatalf("Append(held-out) before refuse = %v, want nil", err)
	}
	_, err = teacher.OpenRecordStore(path, []string{holdout})
	if err == nil {
		t.Fatalf("OpenRecordStore with held-out hash = nil, want error")
	}
	if !strings.Contains(err.Error(), holdout) {
		t.Fatalf("OpenRecordStore error %q does not name hash %s", err, holdout)
	}

	fresh := filepath.Join(t.TempDir(), "records2.jsonl")
	s2, err := teacher.OpenRecordStore(fresh, []string{holdout})
	if err != nil {
		t.Fatalf("OpenRecordStore(fresh, hashes) = %v, want nil", err)
	}
	err = s2.Append(testRecord("req-held2", holdout, 2))
	if err == nil {
		t.Fatalf("Append(held-out hash) = nil, want error")
	}
	if !strings.Contains(err.Error(), holdout) {
		t.Fatalf("Append error %q does not name hash %s", err, holdout)
	}
}

func TestRecordStoreRejectsBadLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.jsonl")
	good, err := json.Marshal(testRecord("req-good", strings.Repeat("d", 64), 3))
	if err != nil {
		t.Fatalf("json.Marshal(good record) = %v, want nil", err)
	}
	content := string(good) + "\n" + `{"schema_version":"wrong"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile = %v, want nil", err)
	}
	_, err = teacher.OpenRecordStore(path, nil)
	if err == nil {
		t.Fatalf("OpenRecordStore(bad line) = nil, want error")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("OpenRecordStore error %q does not name line 2", err)
	}
}

type errorTransport struct{}

func (errorTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("network must not be touched")
}

func TestOfflineJSONLAnswersWithoutNetwork(t *testing.T) {
	old := http.DefaultTransport
	http.DefaultTransport = errorTransport{}
	defer func() { http.DefaultTransport = old }()

	path := filepath.Join(t.TempDir(), "records.jsonl")
	s, err := teacher.OpenRecordStore(path, nil)
	if err != nil {
		t.Fatalf("OpenRecordStore = %v, want nil", err)
	}
	src := testRecord("req-1", strings.Repeat("e", 64), 4)
	if err := s.Append(src); err != nil {
		t.Fatalf("Append = %v, want nil", err)
	}
	o := teacher.OfflineJSONL{Store: s, ID: "offline", Version: "v1"}

	incoming := teacher.Request{
		RequestID:    "req-2",
		InputHash:    strings.Repeat("e", 64),
		Kind:         teacher.KindLabel,
		ModelVersion: "v1",
	}
	got, err := o.Ask(context.Background(), incoming)
	if err != nil {
		t.Fatalf("Ask(hit) = %v, want nil", err)
	}
	want := src.Response
	want.RequestID = "req-2"
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Ask(hit) = %+v, want %+v", got, want)
	}

	miss := teacher.Request{
		RequestID:    "req-3",
		InputHash:    strings.Repeat("f", 64),
		Kind:         teacher.KindLabel,
		ModelVersion: "v1",
	}
	if _, err := o.Ask(context.Background(), miss); !errors.Is(err, teacher.ErrNoAnswer) {
		t.Fatalf("Ask(miss) = %v, want ErrNoAnswer", err)
	}

	d := o.Describe()
	if d.ID != "offline" || d.Version != "v1" || d.Kind != teacher.DescriptorOffline {
		t.Fatalf("Describe() = %+v, want {offline v1 offline}", d)
	}
}

func TestOfflineJSONLRejectsInvalidRequest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.jsonl")
	s, err := teacher.OpenRecordStore(path, nil)
	if err != nil {
		t.Fatalf("OpenRecordStore = %v, want nil", err)
	}
	o := teacher.OfflineJSONL{Store: s, ID: "offline", Version: "v1"}
	bad := teacher.Request{
		RequestID:    "req-x",
		InputHash:    "not-64-hex",
		Kind:         teacher.KindLabel,
		ModelVersion: "v1",
	}
	_, err = o.Ask(context.Background(), bad)
	if err == nil {
		t.Fatalf("Ask(invalid) = nil, want error")
	}
	if errors.Is(err, teacher.ErrNoAnswer) {
		t.Fatalf("Ask(invalid) = ErrNoAnswer, want request validation error before lookup")
	}
	if !strings.Contains(err.Error(), "input_hash") {
		t.Fatalf("Ask(invalid) error %q does not name input_hash", err)
	}
}

func TestOfflineJSONLAskErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.jsonl")
	s, err := teacher.OpenRecordStore(path, nil)
	if err != nil {
		t.Fatalf("OpenRecordStore = %v, want nil", err)
	}
	valid := teacher.Request{
		RequestID:    "req-y",
		InputHash:    strings.Repeat("a", 64),
		Kind:         teacher.KindLabel,
		ModelVersion: "v1",
	}
	if _, err := (teacher.OfflineJSONL{Store: s, ID: "offline", Version: "v1"}).Ask(nil, valid); err == nil {
		t.Fatalf("Ask(nil ctx) = nil, want error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (teacher.OfflineJSONL{Store: s, ID: "offline", Version: "v1"}).Ask(ctx, valid); !errors.Is(err, context.Canceled) {
		t.Fatalf("Ask(cancelled ctx) = %v, want context.Canceled", err)
	}
	if _, err := (teacher.OfflineJSONL{Store: s, ID: "", Version: "v1"}).Ask(context.Background(), valid); err == nil {
		t.Fatalf("Ask(empty ID) = nil, want error")
	}
	if _, err := (teacher.OfflineJSONL{Store: s, ID: "offline", Version: ""}).Ask(context.Background(), valid); err == nil {
		t.Fatalf("Ask(empty Version) = nil, want error")
	}
	if _, err := (teacher.OfflineJSONL{Store: nil, ID: "offline", Version: "v1"}).Ask(context.Background(), valid); err == nil {
		t.Fatalf("Ask(nil store) = nil, want error")
	}
}
