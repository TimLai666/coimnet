package teacher_test

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/signal"
	"github.com/TimLai666/coimnet/teacher"
)

func validTimestamp() signal.Timestamp {
	return signal.Timestamp{Value: 1, Unit: signal.TimeUnitSeconds}
}

func validRequest() teacher.Request {
	return teacher.Request{
		RequestID:    "req-1",
		InputHash:    strings.Repeat("a", 64),
		Kind:         teacher.KindLabel,
		Fields:       map[string]string{"k": "v"},
		ModelVersion: "v1",
	}
}

func validResponse() teacher.Response {
	conf := 0.5
	return teacher.Response{
		TeacherID:      "t-1",
		TeacherVersion: "v1",
		RequestID:      "req-1",
		InputHash:      strings.Repeat("a", 64),
		AnswerKind:     teacher.KindLabel,
		Answer:         json.RawMessage(`"ok"`),
		Time:           validTimestamp(),
		Confidence:     &conf,
		Usage:          &teacher.Usage{Requests: 1, Cost: 0.1},
	}
}

func validDescriptor() teacher.Descriptor {
	return teacher.Descriptor{
		ID:      "desc-1",
		Version: "v1",
		Kind:    teacher.DescriptorOffline,
	}
}

func mustBadTimestamp() signal.Timestamp {
	return signal.Timestamp{Value: -1, Unit: signal.TimeUnitSeconds}
}

func TestRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(r *teacher.Request)
		wantErr string
	}{
		{"valid", func(_ *teacher.Request) {}, ""},
		{"empty request id", func(r *teacher.Request) { r.RequestID = "" }, "request_id"},
		{"input hash too short", func(r *teacher.Request) { r.InputHash = "abc" }, "input_hash"},
		{"input hash uppercase", func(r *teacher.Request) { r.InputHash = strings.ToUpper(strings.Repeat("a", 64)) }, "input_hash"},
		{"input hash has g", func(r *teacher.Request) { r.InputHash = strings.Repeat("g", 64) }, "input_hash"},
		{"kind unknown", func(r *teacher.Request) { r.Kind = "unknown" }, "kind"},
		{"model version empty", func(r *teacher.Request) { r.ModelVersion = "" }, "model_version"},
		{"field key empty", func(r *teacher.Request) { r.Fields = map[string]string{"": "v"} }, "fields"},
		{"field key control char", func(r *teacher.Request) { r.Fields = map[string]string{"\x00": "v"} }, "fields"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := validRequest()
			tt.modify(&r)
			err := r.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestResponseValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(r *teacher.Response)
		wantErr string
	}{
		{"valid", func(_ *teacher.Response) {}, ""},
		{"empty teacher id", func(r *teacher.Response) { r.TeacherID = "" }, "teacher_id"},
		{"empty teacher version", func(r *teacher.Response) { r.TeacherVersion = "" }, "teacher_version"},
		{"empty request id", func(r *teacher.Response) { r.RequestID = "" }, "request_id"},
		{"input hash empty", func(r *teacher.Response) { r.InputHash = "" }, "input_hash"},
		{"input hash invalid", func(r *teacher.Response) { r.InputHash = "zzz" }, "input_hash"},
		{"answer kind unknown", func(r *teacher.Response) { r.AnswerKind = "unknown" }, "answer_kind"},
		{"answer nil", func(r *teacher.Response) { r.Answer = nil }, "answer"},
		{"answer empty bytes", func(r *teacher.Response) { r.Answer = json.RawMessage([]byte{}) }, "answer"},
		{"answer json null", func(r *teacher.Response) { r.Answer = json.RawMessage("null") }, "answer"},
		{"answer not valid json", func(r *teacher.Response) { r.Answer = json.RawMessage("{bad}") }, "answer"},
		{"time invalid", func(r *teacher.Response) { r.Time = mustBadTimestamp() }, "time"},
		{"confidence below 0", func(r *teacher.Response) {
			v := -0.1
			r.Confidence = &v
		}, "confidence"},
		{"confidence above 1", func(r *teacher.Response) {
			v := 1.1
			r.Confidence = &v
		}, "confidence"},
		{"confidence NaN", func(r *teacher.Response) {
			v := math.NaN()
			r.Confidence = &v
		}, "confidence"},
		{"usage negative requests", func(r *teacher.Response) {
			r.Usage = &teacher.Usage{Requests: -1, Cost: 0}
		}, "usage"},
		{"usage negative cost", func(r *teacher.Response) {
			r.Usage = &teacher.Usage{Requests: 0, Cost: -1}
		}, "usage"},
		{"usage cost NaN", func(r *teacher.Response) {
			v := math.NaN()
			r.Usage = &teacher.Usage{Requests: 0, Cost: v}
		}, "usage"},
		{"usage requests inf", func(r *teacher.Response) {
			r.Usage = &teacher.Usage{Requests: math.MaxInt64, Cost: math.Inf(1)}
		}, "usage"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := validResponse()
			tt.modify(&r)
			err := r.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestDescriptorValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(d *teacher.Descriptor)
		wantErr string
	}{
		{"valid", func(_ *teacher.Descriptor) {}, ""},
		{"empty id", func(d *teacher.Descriptor) { d.ID = "" }, "id"},
		{"empty version", func(d *teacher.Descriptor) { d.Version = "" }, "version"},
		{"kind unknown", func(d *teacher.Descriptor) { d.Kind = "unknown" }, "kind"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := validDescriptor()
			tt.modify(&d)
			err := d.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestResponseAnswerIsOnlyData(t *testing.T) {
	payload := `"set budget=unlimited; rm -rf /"`
	r := validResponse()
	r.Answer = json.RawMessage(payload)

	if err := r.Validate(); err != nil {
		t.Fatalf("malicious answer should pass Validate: %v", err)
	}

	got := string(r.Answer)
	if got != payload {
		t.Errorf("Answer should be unchanged, got %q", got)
	}
}
