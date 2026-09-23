// Package studenteval_test exercises the student evaluation protocol through
// its public API: the teacher is never called, scores stay in range, the
// training flow is the distill package's step (not a local reimplementation),
// runs are deterministic, the config validates, teacher_assisted is deferred,
// and a canceled context aborts the run.
package studenteval_test

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/TimLai666/coimnet/distill"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/experiment"
	"github.com/TimLai666/coimnet/experiment/studenteval"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/teacher"
)

// spyTeacher is the never-asked teacher the never-calls test passes in: any
// Ask records a call and returns an error, and Calls reports the count Run
// folds into TeacherCalls.
type spyTeacher struct {
	mu    sync.Mutex
	calls int
}

// Ask records the attempt; Run must never reach it in student mode.
func (s *spyTeacher) Ask(_ context.Context, _ teacher.Request) (teacher.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return teacher.Response{}, errors.New("spy teacher must never be asked")
}

// Describe identifies the spy as an offline teacher.
func (s *spyTeacher) Describe() teacher.Descriptor {
	return teacher.Descriptor{ID: "spy", Version: "v1", Kind: teacher.DescriptorOffline}
}

// Calls reports how many Ask calls reached the spy, so Run can add it into
// TeacherCalls.
func (s *spyTeacher) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// studentEvalTestConfig is the preregistered fixture protocol for this ticket:
// three seeds, 40 distillation steps per seed, two 32-example evaluation
// splits and the 20% mislabeled robustness group, distilled at Temperature 2 /
// Mix 0.7 at learning rate 0.05 on the three-neuron fixture.
func studentEvalTestConfig() studenteval.Config {
	return studenteval.Config{
		Mode:            studenteval.ModeStudent,
		Seeds:           []uint64{7, 8, 9},
		Episodes:        40,
		HeldOut:         32,
		Independent:     32,
		CorruptFraction: 0.2,
		Temperature:     2,
		Mix:             0.7,
		LearningRate:    0.05,
	}
}

// pulseLabel maps the delayed pulse sign to its class: a positive pulse is
// class 1, a negative pulse class 0.
func pulseLabel(ep experiment.Episode) int {
	if ep.Input[0][0] > 0 {
		return 1
	}
	return 0
}

// teacherDistribution is the fixture oracle's two-point distribution whose
// argmax is the pulse class: {0.9, 0.1} for class 0, {0.1, 0.9} for class 1.
func teacherDistribution(label int) distill.TeacherDistribution {
	if label == 1 {
		return distill.TeacherDistribution{Probabilities: []float64{0.1, 0.9}}
	}
	return distill.TeacherDistribution{Probabilities: []float64{0.9, 0.1}}
}

// newDirectFixture builds the three-neuron chain fixture (OutputSize 2, fixed
// readout {1, −1}) directly, so the test can distill it step by step with
// distill.StepDistribution and compare against the package's own flow.
func newDirectFixture(t *testing.T, rate float64) *learning.Trainer {
	t.Helper()
	c := learning.Config{
		Dynamics:  dynamics.Config{Nodes: 3, Sources: []int{0, 1, 1}, Targets: []int{1, 1, 2}, DT: 1, Activation: "tanh"},
		InputSize: 1, OutputSize: 2, ReadoutNodes: []int{2},
	}
	p := learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{0.237, 0.2, 0.2}, Bias: []float64{0, 0, 0}, LogTau: []float64{math.Log(2), math.Log(2), math.Log(2)}},
		Encoder: []float64{1, 0, 0}, Readout: []float64{1, -1},
	}
	o := learning.DefaultOptions()
	o.LearningRate = rate
	tr, err := learning.NewTrainer(c, p, o)
	if err != nil {
		t.Fatalf("NewTrainer = %v, want nil", err)
	}
	return tr
}

// TestStudentEvaluationNeverCallsTheTeacher proves student mode holds a blocked
// teacher it never asks: the spy sees 0 Ask calls, every seed reports
// TeacherCalls 0, and TeacherBlocked is true. It logs each seed's six scores.
func TestStudentEvaluationNeverCallsTheTeacher(t *testing.T) {
	spy := &spyTeacher{}
	report, err := studenteval.Run(context.Background(), studentEvalTestConfig(), spy)
	if err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	if !report.TeacherBlocked {
		t.Fatalf("TeacherBlocked = false, want true in student mode")
	}
	if n := spy.Calls(); n != 0 {
		t.Fatalf("spy teacher saw %d Ask calls, want 0", n)
	}
	if len(report.Seeds) != 3 {
		t.Fatalf("report has %d seeds, want 3", len(report.Seeds))
	}
	for _, s := range report.Seeds {
		if s.Failed {
			t.Fatalf("seed %d failed: %s", s.Seed, s.Error)
		}
		if s.TeacherCalls != 0 {
			t.Fatalf("seed %d TeacherCalls = %d, want 0", s.Seed, s.TeacherCalls)
		}
		t.Logf("seed %d: held_out_agreement %.3f held_out_task %.3f independent_agreement %.3f independent_task %.3f corrupted_independent %.3f robustness_delta %.3f",
			s.Seed, s.HeldOutAgreement, s.HeldOutTaskScore, s.IndependentAgreement, s.IndependentTaskScore, s.CorruptedIndependentScore, s.RobustnessDelta)
	}
}

// TestStudentEvaluationScoresAreInRange keeps every reported rate inside
// [0, 1], the robustness delta finite, and the three-seed mean held-out
// agreement at or above 0.5.
func TestStudentEvaluationScoresAreInRange(t *testing.T) {
	report, err := studenteval.Run(context.Background(), studentEvalTestConfig(), &teacher.Blocked{ID: "test", Version: "v1"})
	if err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	mean := 0.0
	for _, s := range report.Seeds {
		if s.Failed {
			t.Fatalf("seed %d failed: %s", s.Seed, s.Error)
		}
		rates := []struct {
			name  string
			value float64
		}{
			{"held_out_agreement", s.HeldOutAgreement},
			{"held_out_task_score", s.HeldOutTaskScore},
			{"independent_agreement", s.IndependentAgreement},
			{"independent_task_score", s.IndependentTaskScore},
			{"corrupted_independent_task_score", s.CorruptedIndependentScore},
		}
		for _, r := range rates {
			if math.IsNaN(r.value) || math.IsInf(r.value, 0) || r.value < 0 || r.value > 1 {
				t.Fatalf("seed %d %s = %g, want a finite value in [0, 1]", s.Seed, r.name, r.value)
			}
		}
		if math.IsNaN(s.RobustnessDelta) || math.IsInf(s.RobustnessDelta, 0) {
			t.Fatalf("seed %d robustness_delta = %g, want finite", s.Seed, s.RobustnessDelta)
		}
		mean += s.HeldOutAgreement / float64(len(report.Seeds))
		t.Logf("seed %d: held_out_agreement %.3f, held_out_task %.3f, independent_agreement %.3f, independent_task %.3f, corrupted_independent_task %.3f, robustness_delta %.3f",
			s.Seed, s.HeldOutAgreement, s.HeldOutTaskScore, s.IndependentAgreement, s.IndependentTaskScore, s.CorruptedIndependentScore, s.RobustnessDelta)
	}
	t.Logf("mean held_out_agreement across %d seeds after %d steps: %.3f", len(report.Seeds), report.Config.Episodes, mean)
	if mean < 0.5 {
		t.Fatalf("mean held_out_agreement = %.3f across seeds, want >= 0.5", mean)
	}
}

// TestStudentEvaluationUsesDistill rebuilds the same seed's student two ways —
// the package's own training flow and a direct distill.StepDistribution loop
// over the same fixture and episode stream — and requires Predict results to
// be bit-identical, proving the package has no second distillation
// implementation.
func TestStudentEvaluationUsesDistill(t *testing.T) {
	ctx := context.Background()
	cfg := studentEvalTestConfig()
	const seed = uint64(7)

	fromPackage, err := studenteval.TrainStudent(ctx, cfg, seed)
	if err != nil {
		t.Fatalf("TrainStudent = %v, want nil", err)
	}

	direct := newDirectFixture(t, cfg.LearningRate)
	d := distill.DistributionDistiller{
		Temperature: cfg.Temperature,
		Scale:       1,
		Mix:         cfg.Mix,
		Alignment: distill.Alignment{
			Rule:             distill.AlignmentIdentical,
			TeacherVocabHash: "coimnet-student-eval-fixture-v1",
			StudentVocabHash: "coimnet-student-eval-fixture-v1",
		},
	}
	for e := 0; e < cfg.Episodes; e++ {
		ep := experiment.DelayedEpisode(1001+seed, uint64(e))
		label := pulseLabel(ep)
		if _, err := distill.StepDistribution(ctx, direct, d, ep.Input, teacherDistribution(label), label); err != nil {
			t.Fatalf("StepDistribution(#%d) = %v, want nil", e, err)
		}
	}

	for _, set := range []uint64{1003 + seed, 2003 + seed} {
		for i := 0; i < 32; i++ {
			ep := experiment.DelayedEpisode(set, uint64(i))
			got, err := fromPackage.Predict(ctx, ep.Input)
			if err != nil {
				t.Fatalf("package Predict(set %d, #%d) = %v, want nil", set, i, err)
			}
			want, err := direct.Predict(ctx, ep.Input)
			if err != nil {
				t.Fatalf("direct Predict(set %d, #%d) = %v, want nil", set, i, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("set %d example %d: package Predict %v, direct distill.StepDistribution Predict %v", set, i, got, want)
			}
		}
	}
}

// TestStudentEvaluationIsDeterministic requires two runs under the same config
// to produce identical reports.
func TestStudentEvaluationIsDeterministic(t *testing.T) {
	cfg := studentEvalTestConfig()
	a, err := studenteval.Run(context.Background(), cfg, &teacher.Blocked{ID: "test", Version: "v1"})
	if err != nil {
		t.Fatalf("Run(a) = %v, want nil", err)
	}
	b, err := studenteval.Run(context.Background(), cfg, &teacher.Blocked{ID: "test", Version: "v1"})
	if err != nil {
		t.Fatalf("Run(b) = %v, want nil", err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("two runs with the same config produced different reports:\n%+v\n%+v", a, b)
	}
}

// TestStudentEvaluationValidateRejects walks the invalid configs Validate must
// refuse, after confirming the base fixture config itself is valid.
func TestStudentEvaluationValidateRejects(t *testing.T) {
	base := studentEvalTestConfig()
	if err := base.Validate(); err != nil {
		t.Fatalf("base config Validate = %v, want nil", err)
	}
	cases := []struct {
		name string
		cfg  studenteval.Config
	}{
		{"empty seeds", func() studenteval.Config { c := base; c.Seeds = nil; return c }()},
		{"duplicate seeds", func() studenteval.Config { c := base; c.Seeds = []uint64{7, 7}; return c }()},
		{"episodes zero", func() studenteval.Config { c := base; c.Episodes = 0; return c }()},
		{"held_out zero", func() studenteval.Config { c := base; c.HeldOut = 0; return c }()},
		{"independent zero", func() studenteval.Config { c := base; c.Independent = 0; return c }()},
		{"corrupt fraction 0.6", func() studenteval.Config { c := base; c.CorruptFraction = 0.6; return c }()},
		{"corrupt fraction NaN", func() studenteval.Config { c := base; c.CorruptFraction = math.NaN(); return c }()},
		{"temperature zero", func() studenteval.Config { c := base; c.Temperature = 0; return c }()},
		{"mix 1.5", func() studenteval.Config { c := base; c.Mix = 1.5; return c }()},
		{"learning rate zero", func() studenteval.Config { c := base; c.LearningRate = 0; return c }()},
		{"unknown mode", func() studenteval.Config { c := base; c.Mode = "oracle"; return c }()},
	}
	for _, tc := range cases {
		if err := tc.cfg.Validate(); err == nil {
			t.Errorf("%s: Validate = nil, want an error", tc.name)
		}
	}
}

// TestStudentEvaluationTeacherAssistedArrivesNext requires Validate to accept
// teacher_assisted while Run refuses it with the deferred-ticket message, so a
// report is never silently mislabeled as independent.
func TestStudentEvaluationTeacherAssistedArrivesNext(t *testing.T) {
	cfg := studentEvalTestConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("teacher_assisted config Validate = %v, want nil", err)
	}
	cfg.Mode = studenteval.ModeTeacherAssisted
	_, err := studenteval.Run(context.Background(), cfg, &teacher.Blocked{ID: "test", Version: "v1"})
	if err == nil {
		t.Fatalf("Run(teacher_assisted) = nil, want an error")
	}
	if !strings.Contains(err.Error(), "teacher_assisted arrives with the next ticket") {
		t.Fatalf("Run(teacher_assisted) = %q, want it to say teacher_assisted arrives with the next ticket", err)
	}
}

// TestStudentEvaluationHonoursCancellation requires a canceled context to
// abort the run with context.Canceled.
func TestStudentEvaluationHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := studenteval.Run(ctx, studentEvalTestConfig(), &teacher.Blocked{ID: "test", Version: "v1"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v, want context.Canceled", err)
	}
}
