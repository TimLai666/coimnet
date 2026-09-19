package experiment

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/experiment/gridnav"
)

// imitationTestConfig is the shared LRN-09 imitation fixture: the length-7
// corridor the gridnav tests pin, the three preregistered seeds, and a budget
// small enough to run under the race detector.
//
// The budget is 500 episodes rather than the 40 this ticket first named. The
// readout neurons sit two hops behind the input, so the expert's three-step
// trajectory only reaches them on its last step: two of every three training
// steps can move nothing but the readout bias, and 40 updates left all three
// seeds below where they started.
func imitationTestConfig() ImitationConfig {
	return ImitationConfig{
		Corridor:     gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: 0.01, GoalReward: 1},
		Seeds:        []uint64{1, 2, 3},
		Episodes:     500,
		Hidden:       16,
		LearningRate: 0.05,
		EvalEpisodes: 20,
	}
}

// TestImitationImprovesAgreement is the LRN-09 imitation claim: after training
// on the expert's own actions every seed matches the expert more often than it
// did before training, and the learned policy collects more reward than the
// uniform random policy on the same evaluation episodes.
func TestImitationImprovesAgreement(t *testing.T) {
	c := imitationTestConfig()
	report, err := RunImitation(context.Background(), c)
	if err != nil {
		t.Fatalf("RunImitation: %v", err)
	}
	if report.SchemaVersion != ImitationSchemaVersion {
		t.Fatalf("schema version %q, want %q", report.SchemaVersion, ImitationSchemaVersion)
	}
	if len(report.Results) != len(c.Seeds) {
		t.Fatalf("report holds %d seeds, want %d", len(report.Results), len(c.Seeds))
	}
	t.Logf("random policy baseline return = %v", report.Baseline)
	beat := 0
	for _, r := range report.Results {
		t.Logf("seed %d: agreement %v -> %v, mean return %v, updates %d", r.Seed, r.AgreementBefore, r.ExpertAgreement, r.MeanReturn, r.Updates)
		if r.Failed {
			t.Errorf("seed %d failed: %s", r.Seed, r.Error)
			continue
		}
		if r.ExpertAgreement <= r.AgreementBefore {
			t.Errorf("seed %d agreement %v did not improve on %v", r.Seed, r.ExpertAgreement, r.AgreementBefore)
		}
		if r.Updates != uint64(c.Episodes) {
			t.Errorf("seed %d applied %d updates, want %d", r.Seed, r.Updates, c.Episodes)
		}
		if r.MeanReturn > report.Baseline {
			beat++
		}
	}
	if beat < 2 {
		t.Errorf("%d of %d seeds beat the random baseline %v, want at least 2", beat, len(report.Results), report.Baseline)
	}
}

// TestImitationIsDeterministic pins reproducibility: the same protocol run
// twice produces bit-identical per-seed results.
func TestImitationIsDeterministic(t *testing.T) {
	c := imitationTestConfig()
	a, err := RunImitation(context.Background(), c)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	b, err := RunImitation(context.Background(), c)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if !reflect.DeepEqual(a.Results, b.Results) {
		t.Fatalf("two runs of the same protocol differ:\n%+v\n%+v", a.Results, b.Results)
	}
	if a.Baseline != b.Baseline {
		t.Fatalf("baseline %v then %v", a.Baseline, b.Baseline)
	}
	if a.ConfigHash != b.ConfigHash {
		t.Fatalf("config hash %q then %q", a.ConfigHash, b.ConfigHash)
	}
}

// TestImitationValidateRejects checks that an unusable protocol is refused
// before anything runs.
func TestImitationValidateRejects(t *testing.T) {
	cases := []struct {
		name  string
		patch func(*ImitationConfig)
	}{
		{"no seeds", func(c *ImitationConfig) { c.Seeds = nil }},
		{"zero episodes", func(c *ImitationConfig) { c.Episodes = 0 }},
		{"hidden below four", func(c *ImitationConfig) { c.Hidden = 3 }},
		{"zero learning rate", func(c *ImitationConfig) { c.LearningRate = 0 }},
		{"zero eval episodes", func(c *ImitationConfig) { c.EvalEpisodes = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := imitationTestConfig()
			tc.patch(&c)
			if err := c.Validate(); err == nil {
				t.Fatalf("Validate accepted %s", tc.name)
			}
			if _, err := RunImitation(context.Background(), c); err == nil {
				t.Fatalf("RunImitation accepted %s", tc.name)
			}
		})
	}
	if err := imitationTestConfig().Validate(); err != nil {
		t.Fatalf("valid protocol rejected: %v", err)
	}
}

// TestImitationHonoursCancellation checks that a canceled context aborts the
// whole run instead of being recorded as a failed seed.
func TestImitationHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report, err := RunImitation(ctx, imitationTestConfig())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunImitation error = %v, want context.Canceled", err)
	}
	for _, r := range report.Results {
		if r.Failed {
			t.Fatalf("cancellation recorded as a failed seed: %+v", r)
		}
	}
}

// TestImitationEvaluatorNeverReadsTheExpert backs the reported assumption.
// gridnav.Env has no call hook, so the proof is in two parts: the evaluator's
// only decision point cannot reach Expert because it never receives the
// environment, and the action it returns is reproduced here from the
// observation prefix alone.
func TestImitationEvaluatorNeverReadsTheExpert(t *testing.T) {
	fn := reflect.TypeOf(imitationAction)
	env := reflect.TypeOf((*gridnav.Env)(nil))
	for i := 0; i < fn.NumIn(); i++ {
		if fn.In(i) == env {
			t.Fatalf("imitationAction parameter %d is %v, so the evaluator could call Expert", i, env)
		}
	}
	c := imitationTestConfig()
	tr, err := newImitationTrainer(c.Seeds[0], c.Hidden, c.LearningRate)
	if err != nil {
		t.Fatalf("newImitationTrainer: %v", err)
	}
	e, err := gridnav.New(c.Corridor)
	if err != nil {
		t.Fatalf("gridnav.New: %v", err)
	}
	obs, _ := e.Reset(imitationEvalSeed(0))
	prefix := [][]float64{obs.Vector()}
	// The readout neurons are two hops behind the input, so a prefix shorter
	// than three steps reads out exact zeros and the argmax would be a tie.
	for i := 0; i < 3; i++ {
		next, _, _, _, err := e.Step(gridnav.ActionStay)
		if err != nil {
			t.Fatalf("Step %d: %v", i+1, err)
		}
		prefix = append(prefix, next.Vector())
	}
	got, err := imitationAction(context.Background(), tr, prefix)
	if err != nil {
		t.Fatalf("imitationAction: %v", err)
	}
	logits, err := tr.PredictAll(context.Background(), prefix)
	if err != nil {
		t.Fatalf("PredictAll: %v", err)
	}
	last := logits[len(logits)-1]
	want := 0
	for j, v := range last {
		if v > last[want] {
			want = j
		}
	}
	t.Logf("last-step logits %v, argmax %d, evaluator action %d", last, want, got)
	for j, v := range last {
		if j != want && v == last[want] {
			t.Fatalf("logits %v tie at %d and %d, so the argmax proves nothing", last, want, j)
		}
	}
	if got != want {
		t.Fatalf("evaluator action %d, want the observation-prefix argmax %d", got, want)
	}
}
