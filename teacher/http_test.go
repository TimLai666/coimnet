package teacher_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TimLai666/coimnet/config"
	"github.com/TimLai666/coimnet/signal"
	"github.com/TimLai666/coimnet/teacher"
)

// replyServer is an httptest server that records the number of requests it
// receives, each request's raw body and Authorization header, and answers via
// respond(call, request) with a status and body.
type replyServer struct {
	srv    *httptest.Server
	mu     sync.Mutex
	calls  int
	bodies []string
	auths  []string
	last   teacher.Request
}

func newReplyServer(t *testing.T, respond func(call int, r teacher.Request) (int, []byte)) *replyServer {
	t.Helper()
	rs := &replyServer{}
	rs.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("server: read body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var req teacher.Request
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("server: decode request: %v; body %q", err, body)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		rs.mu.Lock()
		rs.calls++
		call := rs.calls
		rs.bodies = append(rs.bodies, string(body))
		rs.auths = append(rs.auths, r.Header.Get("Authorization"))
		rs.last = req
		rs.mu.Unlock()
		status, payload := respond(call, req)
		w.WriteHeader(status)
		if len(payload) > 0 {
			if _, err := w.Write(payload); err != nil {
				t.Errorf("server: write response: %v", err)
			}
		}
	}))
	t.Cleanup(rs.srv.Close)
	return rs
}

func (s *replyServer) requestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *replyServer) lastBody() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bodies[len(s.bodies)-1]
}

func (s *replyServer) lastAuth() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.auths[len(s.auths)-1]
}

func (s *replyServer) lastRequest() teacher.Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

// responseJSON builds a valid teacher.Response JSON that echoes the request.
func responseJSON(r teacher.Request) []byte {
	conf := 0.5
	resp := teacher.Response{
		TeacherID:      "http-test",
		TeacherVersion: "v1",
		RequestID:      r.RequestID,
		InputHash:      r.InputHash,
		AnswerKind:     teacher.KindLabel,
		Answer:         json.RawMessage(`"ok"`),
		Time:           signal.Timestamp{Value: 1, Unit: signal.TimeUnitSeconds},
		Confidence:     &conf,
		Usage:          &teacher.Usage{Requests: 1, Cost: 0.1},
	}
	b, err := json.Marshal(resp)
	if err != nil {
		panic(err)
	}
	return b
}

func validHTTPRequest(requestID, inputHash string) teacher.Request {
	return teacher.Request{
		RequestID:    requestID,
		InputHash:    inputHash,
		Kind:         teacher.KindLabel,
		Fields:       map[string]string{"prompt": "x"},
		ModelVersion: "v1",
	}
}

func okTeacher(srv *httptest.Server) *teacher.HTTP {
	return &teacher.HTTP{
		Endpoint:      srv.URL,
		Timeout:       time.Second,
		MaxRetries:    2,
		Backoff:       time.Millisecond,
		RateLimit:     0,
		Budget:        teacher.Budget{MaxRequests: 100, MaxCost: 100},
		AllowedFields: []string{"prompt", "raw_audio"},
		ID:            "http-t",
		Version:       "v1",
		Env:           func(string) string { return "" },
	}
}

func TestHTTPRetriesOnServerErrorsExactly(t *testing.T) {
	respond := func(call int, r teacher.Request) (int, []byte) {
		if call < 3 {
			return http.StatusServiceUnavailable, nil
		}
		return http.StatusOK, responseJSON(r)
	}

	srvA := newReplyServer(t, respond)
	hA := okTeacher(srvA.srv)
	hA.MaxRetries = 2
	if _, err := hA.Ask(context.Background(), validHTTPRequest("req-1", strings.Repeat("a", 64))); err != nil {
		t.Fatalf("Ask(MaxRetries 2) = %v, want nil", err)
	}
	if got := srvA.requestCount(); got != 3 {
		t.Fatalf("server calls with MaxRetries 2 = %d, want 3", got)
	}

	srvB := newReplyServer(t, respond)
	hB := okTeacher(srvB.srv)
	hB.MaxRetries = 1
	if _, err := hB.Ask(context.Background(), validHTTPRequest("req-2", strings.Repeat("b", 64))); err == nil {
		t.Fatal("Ask(MaxRetries 1, 5xx) = nil, want error")
	}
	if got := srvB.requestCount(); got != 2 {
		t.Fatalf("server calls with MaxRetries 1 = %d, want 2", got)
	}
}

func TestHTTPDoesNotRetryClientErrors(t *testing.T) {
	srv := newReplyServer(t, func(call int, r teacher.Request) (int, []byte) {
		return http.StatusBadRequest, nil
	})
	h := okTeacher(srv.srv)
	if _, err := h.Ask(context.Background(), validHTTPRequest("req-1", strings.Repeat("a", 64))); err == nil {
		t.Fatal("Ask(400) = nil, want error")
	}
	if got := srv.requestCount(); got != 1 {
		t.Fatalf("server calls on 400 = %d, want 1", got)
	}
}

func TestHTTPTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		if r.Context().Err() != nil {
			return
		}
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := okTeacher(srv)
	h.Timeout = 20 * time.Millisecond
	h.MaxRetries = 0
	_, err := h.Ask(context.Background(), validHTTPRequest("req-1", strings.Repeat("a", 64)))
	if err == nil {
		t.Fatal("Ask(timeout) = nil, want error")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("Ask(timeout) = %v, want a deadline exceeded error", err)
	}
}

func TestHTTPRateLimit(t *testing.T) {
	srv := newReplyServer(t, func(call int, r teacher.Request) (int, []byte) {
		return http.StatusOK, responseJSON(r)
	})
	h := okTeacher(srv.srv)
	h.RateLimit = 1
	if _, err := h.Ask(context.Background(), validHTTPRequest("req-1", strings.Repeat("a", 64))); err != nil {
		t.Fatalf("Ask(first) = %v, want nil", err)
	}
	_, err := h.Ask(context.Background(), validHTTPRequest("req-2", strings.Repeat("b", 64)))
	if !errors.Is(err, teacher.ErrRateLimited) {
		t.Fatalf("Ask(second, immediately) = %v, want ErrRateLimited", err)
	}
	if got := srv.requestCount(); got != 1 {
		t.Fatalf("server calls after rate limit = %d, want 1", got)
	}
}

func TestHTTPBudget(t *testing.T) {
	srv := newReplyServer(t, func(call int, r teacher.Request) (int, []byte) {
		return http.StatusOK, responseJSON(r)
	})
	h := okTeacher(srv.srv)
	h.Budget = teacher.Budget{MaxRequests: 1, MaxCost: 100}
	if _, err := h.Ask(context.Background(), validHTTPRequest("req-1", strings.Repeat("a", 64))); err != nil {
		t.Fatalf("Ask(first) = %v, want nil", err)
	}
	_, err := h.Ask(context.Background(), validHTTPRequest("req-2", strings.Repeat("b", 64)))
	if !errors.Is(err, teacher.ErrBudgetExhausted) {
		t.Fatalf("Ask(second) = %v, want ErrBudgetExhausted", err)
	}
	if got := srv.requestCount(); got != 1 {
		t.Fatalf("server calls after budget exhausted = %d, want 1", got)
	}

	srv0 := newReplyServer(t, func(call int, r teacher.Request) (int, []byte) {
		return http.StatusOK, responseJSON(r)
	})
	h0 := okTeacher(srv0.srv)
	h0.Budget = teacher.Budget{MaxRequests: 0, MaxCost: 0}
	_, err = h0.Ask(context.Background(), validHTTPRequest("req-3", strings.Repeat("c", 64)))
	if !errors.Is(err, teacher.ErrBudgetExhausted) {
		t.Fatalf("Ask(zero budget) = %v, want ErrBudgetExhausted", err)
	}
	if got := srv0.requestCount(); got != 0 {
		t.Fatalf("server calls with zero budget = %d, want 0", got)
	}
}

func TestHTTPDedupByRequestID(t *testing.T) {
	srv := newReplyServer(t, func(call int, r teacher.Request) (int, []byte) {
		return http.StatusOK, responseJSON(r)
	})
	h := okTeacher(srv.srv)
	r := validHTTPRequest("req-1", strings.Repeat("a", 64))
	first, err := h.Ask(context.Background(), r)
	if err != nil {
		t.Fatalf("Ask(call 1) = %v, want nil", err)
	}
	second, err := h.Ask(context.Background(), r)
	if err != nil {
		t.Fatalf("Ask(call 2) = %v, want nil", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("Ask(call 2) = %+v, want cached equal to %+v", second, first)
	}
	if got := srv.requestCount(); got != 1 {
		t.Fatalf("server calls for same request ID = %d, want 1", got)
	}
	if got := h.Used().MaxRequests; got != 1 {
		t.Fatalf("Used().Requests = %d, want 1", got)
	}
}

func TestHTTPSendsOnlyAllowedFields(t *testing.T) {
	srv := newReplyServer(t, func(call int, r teacher.Request) (int, []byte) {
		return http.StatusOK, responseJSON(r)
	})
	h := okTeacher(srv.srv)
	h.AllowedFields = []string{"prompt"}
	r := validHTTPRequest("req-1", strings.Repeat("a", 64))
	r.Fields = map[string]string{"prompt": "x", "raw_audio": "y"}
	if _, err := h.Ask(context.Background(), r); err != nil {
		t.Fatalf("Ask = %v, want nil", err)
	}
	seen := srv.lastRequest()
	if got := seen.Fields["prompt"]; got != "x" {
		t.Fatalf("server saw fields[prompt] = %q, want x", got)
	}
	if _, ok := seen.Fields["raw_audio"]; ok {
		t.Fatalf("server saw disallowed field raw_audio = %q", seen.Fields["raw_audio"])
	}
	if strings.Contains(srv.lastBody(), "raw_audio") {
		t.Fatalf("request body contains disallowed field: %s", srv.lastBody())
	}
}

func TestHTTPSecretIsBearerAndNeverInBody(t *testing.T) {
	srv := newReplyServer(t, func(call int, r teacher.Request) (int, []byte) {
		return http.StatusOK, responseJSON(r)
	})
	h := okTeacher(srv.srv)
	h.Secret = config.SecretRef{Env: "COIMNET_API_KEY"}
	h.Env = func(string) string { return "sekrit-123" }
	if _, err := h.Ask(context.Background(), validHTTPRequest("req-1", strings.Repeat("a", 64))); err != nil {
		t.Fatalf("Ask(with secret) = %v, want nil", err)
	}
	if got := srv.lastAuth(); got != "Bearer sekrit-123" {
		t.Fatalf("Authorization header = %q, want %q", got, "Bearer sekrit-123")
	}
	if strings.Contains(srv.lastBody(), "sekrit") {
		t.Fatalf("request body leaks secret value: %s", srv.lastBody())
	}

	srvEmpty := newReplyServer(t, func(call int, r teacher.Request) (int, []byte) {
		return http.StatusOK, responseJSON(r)
	})
	hEmpty := okTeacher(srvEmpty.srv)
	hEmpty.Secret = config.SecretRef{Env: "COIMNET_API_KEY"}
	hEmpty.Env = func(string) string { return "" }
	_, err := hEmpty.Ask(context.Background(), validHTTPRequest("req-2", strings.Repeat("b", 64)))
	if err == nil {
		t.Fatal("Ask(empty secret value) = nil, want error")
	}
	if !strings.Contains(err.Error(), "is not set") {
		t.Fatalf("Ask(empty secret value) error %q does not say the secret is not set", err)
	}
	if got := srvEmpty.requestCount(); got != 0 {
		t.Fatalf("server calls with empty secret value = %d, want 0", got)
	}
}

func TestHTTPDescribe(t *testing.T) {
	srv := newReplyServer(t, func(call int, r teacher.Request) (int, []byte) {
		return http.StatusOK, responseJSON(r)
	})
	h := okTeacher(srv.srv)
	d := h.Describe()
	if d.Kind != teacher.DescriptorHTTP {
		t.Fatalf("Describe().Kind = %q, want %q", d.Kind, teacher.DescriptorHTTP)
	}
	if d.ID != h.ID || d.Version != h.Version {
		t.Fatalf("Describe() = %+v, want ID/Version from the teacher", d)
	}
}
