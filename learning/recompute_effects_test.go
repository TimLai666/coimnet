package learning_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

// recomputeEffectsOptions is DefaultOptions with the two-step recompute
// segment every side-effect test declares.
func recomputeEffectsOptions() learning.Options {
	o := learning.DefaultOptions()
	o.Recompute = &learning.Recompute{SegmentSteps: 2}
	return o
}

// assertRecomputeSnapshot requires the recompute individual to still declare
// segment 2 and, with that declaration cleared, to serialize to exactly the
// bytes of the full-history individual.
func assertRecomputeSnapshot(t *testing.T, got, want learning.IndividualSnapshot) {
	t.Helper()
	if r := got.Optimizer.Options.Recompute; r == nil || r.SegmentSteps != 2 {
		t.Fatalf("the recompute individual declares recompute %+v, want segment 2", r)
	}
	got.Optimizer.Options.Recompute = nil
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("recompute snapshot %s, want the full-history snapshot %s", gotJSON, wantJSON)
	}
}

// TestRecomputeResetOptimizerRefusesUnsupportedCore proves ResetOptimizer refuses recompute on a core that cannot run it, without writing it, and accepts it on a continuous core.
func TestRecomputeResetOptimizerRefusesUnsupportedCore(t *testing.T) {
	ctx := context.Background()
	o := recomputeEffectsOptions()
	mixed, err := learning.NewIndividual(mixedNodeConfig(), mixedNodeParameters(), learning.DefaultOptions(), []float64{0, 0, 0})
	if err != nil {
		t.Fatal(err)
	}
	if err := mixed.ResetOptimizer(ctx, o); err == nil || !strings.Contains(err.Error(), "recompute supports the continuous and LIF cores") {
		t.Fatalf("mixed ResetOptimizer error %v, want the recompute support error", err)
	}
	if got := mixed.Snapshot().Optimizer.Options.Recompute; got != nil {
		t.Fatalf("the refused ResetOptimizer left recompute %+v on the mixed individual", got)
	}
	continuous := onlineIndividual(t)
	if err := continuous.ResetOptimizer(ctx, o); err != nil {
		t.Fatalf("continuous ResetOptimizer: %v", err)
	}
	if got := continuous.Snapshot().Optimizer.Options.Recompute; got == nil || got.SegmentSteps != 2 {
		t.Fatalf("the continuous individual declares recompute %+v, want segment 2", got)
	}
}

// TestRecomputeOnlineLearnerMakesNoExtraCalls proves an online learner on a recompute individual takes no extra gradient step, plastic step or reward-gate call and ends bit-identical to the full-history learner.
func TestRecomputeOnlineLearnerMakesNoExtraCalls(t *testing.T) {
	ctx := context.Background()
	policy := onlinePolicy()
	policy.AllowPlastic = true
	type outcome struct {
		report   learning.UpdateReport
		calls    int
		log      []learning.ActionRecord
		snapshot learning.IndividualSnapshot
	}
	run := func(name string, o learning.Options) outcome {
		t.Helper()
		a := onlinePlasticIndividual(t)
		if err := a.ResetOptimizer(ctx, o); err != nil {
			t.Fatalf("%s: ResetOptimizer: %v", name, err)
		}
		mapper, calls := rewardGate(), 0
		l := newLearner(t, a, policy, learning.RewardGateFunc(func(score float64) ([]float64, error) {
			calls++
			return mapper.Applied(score)
		}))
		// NewOnlineLearner validates the gate with one call of its own; only the
		// calls of the loop below are counted.
		calls = 0
		records := make([]learning.ActionRecord, 3)
		for i := range records {
			records[i] = act(t, l, onlineInput())
		}
		if err := l.SetTargets(map[string][]float64{records[0].ActionID: {.4}, records[2].ActionID: {.4}}); err != nil {
			t.Fatalf("%s: SetTargets: %v", name, err)
		}
		for i, record := range records {
			step := int64(3 * (i + 1))
			if err := l.Receive(ctx, feedback(t, record.ActionID, "teacher", step, step, 1)); err != nil {
				t.Fatalf("%s: Receive %s: %v", name, record.ActionID, err)
			}
		}
		report := update(t, l, 9)
		return outcome{report: report, calls: calls, log: l.Log(), snapshot: a.Snapshot()}
	}
	full, recompute := run("full", learning.DefaultOptions()), run("recompute", recomputeEffectsOptions())
	if !reflect.DeepEqual(recompute.report, full.report) || full.report.GradientSteps != 2 || full.report.PlasticSteps != 3 {
		t.Fatalf("recompute update %+v, full update %+v, want the same report with 2 gradient and 3 plastic steps", recompute.report, full.report)
	}
	if recompute.calls != 3 || full.calls != 3 {
		t.Fatalf("reward gate calls: recompute %d, full %d, want 3 on both", recompute.calls, full.calls)
	}
	if len(recompute.log) != len(full.log) {
		t.Fatalf("the recompute log holds %d answers, the full log %d", len(recompute.log), len(full.log))
	}
	for i := range full.log {
		assertRecomputeRows(t, fmt.Sprintf("log[%d] output", i), recompute.log[i].Output, full.log[i].Output)
	}
	if !reflect.DeepEqual(recompute.log, full.log) {
		t.Fatalf("recompute log %+v, want %+v", recompute.log, full.log)
	}
	assertRecomputeSnapshot(t, recompute.snapshot, full.snapshot)
}

// TestRecomputeChemistryUnchanged proves a recompute TrainEpisode between two chemical advances leaves the readouts, the step result, the chemistry report and the whole individual bit-identical to a full-history episode.
func TestRecomputeChemistryUnchanged(t *testing.T) {
	ctx := context.Background()
	c := chemContinuousConfig()
	rows := recomputeTrainerRows(31, 4, c.InputSize, -1, 1)
	target := recomputeTrainerVector(32, c.OutputSize, -1, 1)
	type outcome struct {
		first, second [][]float64
		step          learning.StepResult
		report        learning.ChemistryReport
		snapshot      learning.IndividualSnapshot
	}
	run := func(name string, o learning.Options) outcome {
		t.Helper()
		a := newIndividual(t, c, chemContinuousParameters())
		enableChemistry(t, a, chemistryDeclaration())
		if err := a.ResetOptimizer(ctx, o); err != nil {
			t.Fatalf("%s: ResetOptimizer: %v", name, err)
		}
		var out outcome
		var err error
		if out.first, err = a.Advance(ctx, rows); err != nil {
			t.Fatalf("%s: first Advance: %v", name, err)
		}
		if out.step, err = a.TrainEpisode(ctx, rows, target); err != nil {
			t.Fatalf("%s: TrainEpisode: %v", name, err)
		}
		if out.second, err = a.Advance(ctx, rows); err != nil {
			t.Fatalf("%s: second Advance: %v", name, err)
		}
		out.report, out.snapshot = a.ChemistryReport(), a.Snapshot()
		return out
	}
	full, recompute := run("full", learning.DefaultOptions()), run("recompute", recomputeEffectsOptions())
	assertRecomputeRows(t, "first Advance", recompute.first, full.first)
	assertRecomputeResult(t, "TrainEpisode", recompute.step, full.step)
	assertRecomputeRows(t, "second Advance", recompute.second, full.second)
	if !reflect.DeepEqual(recompute.report, full.report) {
		t.Fatalf("recompute chemistry report %+v, want %+v", recompute.report, full.report)
	}
	assertRecomputeSnapshot(t, recompute.snapshot, full.snapshot)
}
