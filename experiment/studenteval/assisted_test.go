package studenteval_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TimLai666/coimnet/experiment"
	"github.com/TimLai666/coimnet/experiment/studenteval"
	"github.com/TimLai666/coimnet/signal"
	"github.com/TimLai666/coimnet/teacher"
)

// assistedTestConfig is the fixture protocol under teacher_assisted: the same
// seeds, budgets and splits as studentEvalTestConfig with only the mode moved.
func assistedTestConfig() studenteval.Config {
	c := studentEvalTestConfig()
	c.Mode = studenteval.ModeTeacherAssisted
	return c
}

// inputHashOf mirrors the package's request hash rule: the sha256 hex of the
// input row's JSON encoding.
func inputHashOf(input [][]float64) string {
	b, err := json.Marshal(input)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

// oddTrailingIndex reports whether the request id's trailing number is odd.
func oddTrailingIndex(requestID string) bool {
	i := strings.LastIndex(requestID, "-")
	if i < 0 {
		return false
	}
	n, err := strconv.Atoi(requestID[i+1:])
	return err == nil && n%2 != 0
}

// oracleTeacher is a table teacher: it is built from the same DelayedEpisode
// stream and hash rule as the package, answers each InputHash with the pulse
// label, records every request it sees, and can be told to fail odd trailing
// request numbers with teacher.ErrNoAnswer.
type oracleTeacher struct {
	mu       sync.Mutex
	labels   map[string]int
	requests []teacher.Request
	failOdd  bool
}

func newOracleTeacher(cfg studenteval.Config, failOdd bool) *oracleTeacher {
	o := &oracleTeacher{labels: make(map[string]int), failOdd: failOdd}
	for _, seed := range cfg.Seeds {
		sets := []struct {
			base  uint64
			count int
		}{{1003 + seed, cfg.HeldOut}, {2003 + seed, cfg.Independent}}
		for _, set := range sets {
			for i := 0; i < set.count; i++ {
				ep := experiment.DelayedEpisode(set.base, uint64(i))
				o.labels[inputHashOf(ep.Input)] = pulseLabel(ep)
			}
		}
	}
	return o
}

// Ask records the request, fails odd trailing numbers when configured, and
// otherwise answers the recorded label for the request's InputHash.
func (o *oracleTeacher) Ask(_ context.Context, r teacher.Request) (teacher.Response, error) {
	o.mu.Lock()
	o.requests = append(o.requests, r)
	failOdd := o.failOdd
	label, ok := o.labels[r.InputHash]
	o.mu.Unlock()
	if failOdd && oddTrailingIndex(r.RequestID) {
		return teacher.Response{}, teacher.ErrNoAnswer
	}
	if !ok {
		return teacher.Response{}, teacher.ErrNoAnswer
	}
	answer, err := json.Marshal(strconv.Itoa(label))
	if err != nil {
		return teacher.Response{}, err
	}
	return teacher.Response{
		TeacherID:      "oracle",
		TeacherVersion: "v1",
		RequestID:      r.RequestID,
		InputHash:      r.InputHash,
		AnswerKind:     teacher.KindLabel,
		Answer:         answer,
		Time:           signal.Timestamp{Value: 1, Unit: signal.TimeUnitSeconds},
	}, nil
}

// Describe identifies the oracle as an offline teacher.
func (o *oracleTeacher) Describe() teacher.Descriptor {
	return teacher.Descriptor{ID: "oracle", Version: "v1", Kind: teacher.DescriptorOffline}
}

// Calls reports how many Ask calls reached the oracle.
func (o *oracleTeacher) Calls() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.requests)
}

// requestSnapshot copies the recorded requests for assertions.
func (o *oracleTeacher) requestSnapshot() []teacher.Request {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]teacher.Request(nil), o.requests...)
}

// TestTeacherAssistedCallsTheTeacherAndKeepsTheStudentScores proves the
// assisted protocol: one Ask per held-out and independent input, a perfect
// oracle makes both assisted scores 1 with zero fallbacks, the student's own
// six scores stay bit-identical to a student-mode run of the same config, and
// the report is labeled teacher_assisted with TeacherBlocked false.
func TestTeacherAssistedCallsTheTeacherAndKeepsTheStudentScores(t *testing.T) {
	cfg := assistedTestConfig()
	oracle := newOracleTeacher(cfg, false)
	report, err := studenteval.Run(context.Background(), cfg, oracle)
	if err != nil {
		t.Fatalf("Run(teacher_assisted) = %v, want nil", err)
	}
	if report.TeacherBlocked {
		t.Fatalf("TeacherBlocked = true, want false in teacher_assisted mode")
	}
	if report.Mode != studenteval.ModeTeacherAssisted {
		t.Fatalf("Mode = %q, want %q", report.Mode, studenteval.ModeTeacherAssisted)
	}
	wantCaveat := "teacher_assisted scores include the teacher's answers; they are never reported as the student's own capability."
	if n := len(report.Assumptions); n != 3 || report.Assumptions[n-1] != wantCaveat {
		t.Fatalf("Assumptions = %q, want two original sentences plus %q", report.Assumptions, wantCaveat)
	}

	studentReport, err := studenteval.Run(context.Background(), studentEvalTestConfig(), &teacher.Blocked{ID: "test", Version: "v1"})
	if err != nil {
		t.Fatalf("Run(student) = %v, want nil", err)
	}
	if len(report.Seeds) != len(studentReport.Seeds) {
		t.Fatalf("assisted report has %d seeds, student report has %d", len(report.Seeds), len(studentReport.Seeds))
	}

	wantCalls := cfg.HeldOut + cfg.Independent
	for i, s := range report.Seeds {
		if s.Failed {
			t.Fatalf("seed %d failed: %s", s.Seed, s.Error)
		}
		if s.TeacherCalls != wantCalls {
			t.Fatalf("seed %d TeacherCalls = %d, want %d", s.Seed, s.TeacherCalls, wantCalls)
		}
		if s.Fallbacks == nil {
			t.Fatalf("seed %d Fallbacks = nil, want 0", s.Seed)
		}
		if *s.Fallbacks != 0 {
			t.Fatalf("seed %d Fallbacks = %d, want 0", s.Seed, *s.Fallbacks)
		}
		if s.AssistedHeldOutTaskScore == nil || *s.AssistedHeldOutTaskScore != 1 {
			t.Fatalf("seed %d AssistedHeldOutTaskScore = %v, want 1", s.Seed, s.AssistedHeldOutTaskScore)
		}
		if s.AssistedIndependentTaskScore == nil || *s.AssistedIndependentTaskScore != 1 {
			t.Fatalf("seed %d AssistedIndependentTaskScore = %v, want 1", s.Seed, s.AssistedIndependentTaskScore)
		}
		p := studentReport.Seeds[i]
		if p.Seed != s.Seed {
			t.Fatalf("seed order differs: assisted %d vs student %d", s.Seed, p.Seed)
		}
		six := []struct {
			name string
			a, b float64
		}{
			{"held_out_agreement", s.HeldOutAgreement, p.HeldOutAgreement},
			{"held_out_task_score", s.HeldOutTaskScore, p.HeldOutTaskScore},
			{"independent_agreement", s.IndependentAgreement, p.IndependentAgreement},
			{"independent_task_score", s.IndependentTaskScore, p.IndependentTaskScore},
			{"corrupted_independent_task_score", s.CorruptedIndependentScore, p.CorruptedIndependentScore},
			{"robustness_delta", s.RobustnessDelta, p.RobustnessDelta},
		}
		for _, cmp := range six {
			if math.Float64bits(cmp.a) != math.Float64bits(cmp.b) {
				t.Fatalf("seed %d %s: assisted %v != student-mode %v", s.Seed, cmp.name, cmp.a, cmp.b)
			}
		}
		t.Logf("seed %d student: held_out_agreement %.3f held_out_task %.3f independent_agreement %.3f independent_task %.3f corrupted_independent %.3f robustness_delta %.3f",
			s.Seed, s.HeldOutAgreement, s.HeldOutTaskScore, s.IndependentAgreement, s.IndependentTaskScore, s.CorruptedIndependentScore, s.RobustnessDelta)
		t.Logf("seed %d assisted: held_out_task %.3f independent_task %.3f fallbacks %d teacher_calls %d",
			s.Seed, *s.AssistedHeldOutTaskScore, *s.AssistedIndependentTaskScore, *s.Fallbacks, s.TeacherCalls)
	}

	if got, want := oracle.Calls(), len(cfg.Seeds)*wantCalls; got != want {
		t.Fatalf("oracle saw %d Ask calls, want %d", got, want)
	}
	for _, r := range oracle.requestSnapshot() {
		if r.Kind != teacher.KindLabel {
			t.Fatalf("request kind = %q, want %q", r.Kind, teacher.KindLabel)
		}
		if r.ModelVersion != "studenteval/v1" {
			t.Fatalf("request model_version = %q, want studenteval/v1", r.ModelVersion)
		}
		if !strings.HasPrefix(r.RequestID, "assist-") {
			t.Fatalf("request_id = %q, want an assist- prefix", r.RequestID)
		}
	}
}

// TestTeacherAssistedFallsBackWhenTheTeacherCannotAnswer proves that every
// odd-numbered request the teacher refuses falls back to the student's own
// argmax: Fallbacks equals the number of refused requests and each assisted
// score lands between the student's own score and 1, endpoints included.
func TestTeacherAssistedFallsBackWhenTheTeacherCannotAnswer(t *testing.T) {
	cfg := assistedTestConfig()
	oracle := newOracleTeacher(cfg, true)
	report, err := studenteval.Run(context.Background(), cfg, oracle)
	if err != nil {
		t.Fatalf("Run(teacher_assisted) = %v, want nil", err)
	}
	if report.TeacherBlocked {
		t.Fatalf("TeacherBlocked = true, want false in teacher_assisted mode")
	}
	requests := oracle.requestSnapshot()
	for _, s := range report.Seeds {
		if s.Failed {
			t.Fatalf("seed %d failed: %s", s.Seed, s.Error)
		}
		prefix := fmt.Sprintf("assist-%d-", s.Seed)
		odd := 0
		for _, r := range requests {
			if strings.HasPrefix(r.RequestID, prefix) && oddTrailingIndex(r.RequestID) {
				odd++
			}
		}
		if odd == 0 {
			t.Fatalf("seed %d: fixture produced no odd requests to refuse", s.Seed)
		}
		if s.Fallbacks == nil {
			t.Fatalf("seed %d Fallbacks = nil, want %d", s.Seed, odd)
		}
		if *s.Fallbacks != odd {
			t.Fatalf("seed %d Fallbacks = %d, want %d (the refused requests)", s.Seed, *s.Fallbacks, odd)
		}
		if s.AssistedHeldOutTaskScore == nil {
			t.Fatalf("seed %d AssistedHeldOutTaskScore = nil", s.Seed)
		}
		if s.AssistedIndependentTaskScore == nil {
			t.Fatalf("seed %d AssistedIndependentTaskScore = nil", s.Seed)
		}
		if *s.AssistedHeldOutTaskScore < s.HeldOutTaskScore || *s.AssistedHeldOutTaskScore > 1 {
			t.Fatalf("seed %d assisted held_out %.6f, want within [student %.6f, 1]", s.Seed, *s.AssistedHeldOutTaskScore, s.HeldOutTaskScore)
		}
		if *s.AssistedIndependentTaskScore < s.IndependentTaskScore || *s.AssistedIndependentTaskScore > 1 {
			t.Fatalf("seed %d assisted independent %.6f, want within [student %.6f, 1]", s.Seed, *s.AssistedIndependentTaskScore, s.IndependentTaskScore)
		}
		t.Logf("seed %d: refused %d requests; student held_out_task %.3f independent_task %.3f | assisted held_out %.3f independent %.3f",
			s.Seed, odd, s.HeldOutTaskScore, s.IndependentTaskScore, *s.AssistedHeldOutTaskScore, *s.AssistedIndependentTaskScore)
	}
}

// TestStudentModeNeverReachesTheNetwork points a real teacher.HTTP at a
// counting httptest server and runs student mode: the server must receive zero
// requests, every seed must report TeacherCalls 0, and TeacherBlocked true.
func TestStudentModeNeverReachesTheNetwork(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	h := &teacher.HTTP{
		Endpoint:      srv.URL,
		Timeout:       time.Second,
		MaxRetries:    0,
		Budget:        teacher.Budget{MaxRequests: 100},
		AllowedFields: []string{"prompt"},
		ID:            "counting-http",
		Version:       "v1",
		Client:        srv.Client(),
	}
	report, err := studenteval.Run(context.Background(), studentEvalTestConfig(), h)
	if err != nil {
		t.Fatalf("Run(student) = %v, want nil", err)
	}
	mu.Lock()
	received := calls
	mu.Unlock()
	if received != 0 {
		t.Fatalf("httptest server received %d requests, want 0", received)
	}
	if !report.TeacherBlocked {
		t.Fatalf("TeacherBlocked = false, want true in student mode")
	}
	if len(report.Seeds) == 0 {
		t.Fatal("report has no seeds")
	}
	for _, s := range report.Seeds {
		if s.Failed {
			t.Fatalf("seed %d failed: %s", s.Seed, s.Error)
		}
		if s.TeacherCalls != 0 {
			t.Fatalf("seed %d TeacherCalls = %d, want 0", s.Seed, s.TeacherCalls)
		}
	}
	t.Logf("server requests %d, TeacherCalls 0, TeacherBlocked %v", received, report.TeacherBlocked)
}

// TestStudentModeJSONIsUnchanged requires the student-mode report's JSON to
// carry none of the teacher_assisted indicator fields: no assisted_ keys and
// no fallbacks key.
func TestStudentModeJSONIsUnchanged(t *testing.T) {
	report, err := studenteval.Run(context.Background(), studentEvalTestConfig(), &teacher.Blocked{ID: "test", Version: "v1"})
	if err != nil {
		t.Fatalf("Run(student) = %v, want nil", err)
	}
	b, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal(report) = %v, want nil", err)
	}
	if bytes.Contains(b, []byte("assisted_")) {
		t.Fatalf("student-mode JSON contains an assisted_ field: %s", b)
	}
	if bytes.Contains(b, []byte("fallbacks")) {
		t.Fatalf("student-mode JSON contains a fallbacks field: %s", b)
	}
}
