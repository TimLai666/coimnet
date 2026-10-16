package experiment_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/experiment"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/plasticity"
)

func feedbackPermissionIndividual(t *testing.T, fast float64) *learning.Individual {
	t.Helper()
	ind := newAdaptiveIndividual(t)
	rule := plasticity.Rule{
		Kind:       plasticity.RuleHebbianRate,
		DecayE:     0.5,
		DecayP:     0.5,
		PlasticMax: 1,
		WMin:       0.01,
	}
	if err := ind.EnablePlasticity(plasticity.Config{Rule: rule, Edges: []int{0}}); err != nil {
		t.Fatal(err)
	}
	if fast == 0 {
		return ind
	}
	snapshot := ind.Snapshot()
	if snapshot.Plastic == nil || len(snapshot.Plastic.State.Plastic) != 1 {
		t.Fatalf("seed fixture plastic state = %+v, want one fast edge", snapshot.Plastic)
	}
	snapshot.Plastic.State.Plastic[0] = fast
	ind, err := learning.RestoreIndividual(snapshot)
	if err != nil {
		t.Fatalf("restore seeded fixture: %v", err)
	}
	return ind
}

func feedbackPermissionInput() [][]float64 {
	return [][]float64{{.5}, {0}, {.2}}
}

func feedbackPermissionItem(id string, target float64, allowed ...string) experiment.EvaluationItem {
	return experiment.EvaluationItem{
		ID:              id,
		Input:           feedbackPermissionInput(),
		Target:          []float64{target},
		AllowedFeedback: append([]string(nil), allowed...),
	}
}

func feedbackPermissionEvaluation(available []string, adaptation ...experiment.EvaluationItem) experiment.AdaptiveEvaluation {
	return experiment.AdaptiveEvaluation{
		Mode:              experiment.EvaluationModeAdaptive,
		FeedbackAvailable: append([]string(nil), available...),
		Adaptation:        adaptation,
		Scoring: []experiment.EvaluationItem{{
			ID:     "score",
			Input:  feedbackPermissionInput(),
			Target: []float64{0},
		}},
		Reset: experiment.ResetPolicy{},
	}
}

func runFeedbackPermission(t *testing.T, initial learning.IndividualSnapshot, e experiment.AdaptiveEvaluation) (experiment.EvaluationReport, *learning.Individual) {
	t.Helper()
	ind, err := learning.RestoreIndividual(initial)
	if err != nil {
		t.Fatalf("restore feedback-permission fixture: %v", err)
	}
	report, err := experiment.RunAdaptiveEvaluation(context.Background(), ind, e)
	if err != nil {
		t.Fatalf("run feedback-permission fixture: %v", err)
	}
	return report, ind
}

func assertFeedbackPermissionEquivalent(t *testing.T, firstReport, secondReport experiment.EvaluationReport, first, second *learning.Individual) {
	t.Helper()
	if !reflect.DeepEqual(firstReport, secondReport) {
		t.Fatalf("target-only declaration change altered report:\nfirst=%+v\nsecond=%+v", firstReport, secondReport)
	}
	if !reflect.DeepEqual(first.Snapshot(), second.Snapshot()) {
		t.Fatalf("target-only declaration change altered individual state:\nfirst=%+v\nsecond=%+v", first.Snapshot(), second.Snapshot())
	}
	if !firstReport.Contamination.ParametersUnchanged || !secondReport.Contamination.ParametersUnchanged {
		t.Fatal("plastic-only adaptation changed base parameters")
	}
}

func feedbackPermissionMSE(output [][]float64, target []float64) float64 {
	last := output[len(output)-1]
	var sum float64
	for i, value := range last {
		delta := value - target[i]
		sum += delta * delta
	}
	return sum / float64(len(last))
}

func feedbackPermissionClosedGateOracle(t *testing.T, initial learning.IndividualSnapshot, e experiment.AdaptiveEvaluation) (*learning.Individual, []experiment.ItemScore) {
	t.Helper()
	if e.Reset != (experiment.ResetPolicy{}) {
		t.Fatalf("oracle fixture uses reset policy %+v, want zero reset", e.Reset)
	}
	ind, err := learning.RestoreIndividual(initial)
	if err != nil {
		t.Fatalf("restore closed-gate oracle: %v", err)
	}
	ctx := context.Background()
	for _, item := range e.Adaptation {
		if _, err := ind.Advance(ctx, item.Input); err != nil {
			t.Fatalf("closed-gate adaptation %q: %v", item.ID, err)
		}
	}
	scores := make([]experiment.ItemScore, 0, len(e.Scoring))
	for _, item := range e.Scoring {
		output, err := ind.AdvanceFixed(ctx, item.Input)
		if err != nil {
			t.Fatalf("closed-gate scoring %q: %v", item.ID, err)
		}
		scores = append(scores, experiment.ItemScore{ID: item.ID, MSE: feedbackPermissionMSE(output, item.Target), Output: output})
	}
	return ind, scores
}

func TestAdaptiveFeedbackPermissionEmptyDeclarations(t *testing.T) {
	initial := feedbackPermissionIndividual(t, .25).Snapshot()
	withoutFeedback := feedbackPermissionEvaluation(nil,
		feedbackPermissionItem("adapt-a", 0),
		feedbackPermissionItem("adapt-b", 0),
	)
	changedTargets := feedbackPermissionEvaluation(nil,
		feedbackPermissionItem("adapt-a", 100),
		feedbackPermissionItem("adapt-b", 100),
	)

	firstReport, first := runFeedbackPermission(t, initial, withoutFeedback)
	secondReport, second := runFeedbackPermission(t, initial, changedTargets)
	assertFeedbackPermissionEquivalent(t, firstReport, secondReport, first, second)

	oracle, wantScores := feedbackPermissionClosedGateOracle(t, initial, withoutFeedback)
	if !reflect.DeepEqual(first.Snapshot(), oracle.Snapshot()) {
		t.Fatalf("empty feedback skipped ordinary closed-gate Act/decay:\n got=%+v\nwant=%+v", first.Snapshot(), oracle.Snapshot())
	}
	if !reflect.DeepEqual(firstReport.Scores, wantScores) {
		t.Fatalf("empty feedback changed scoring outputs:\n got=%+v\nwant=%+v", firstReport.Scores, wantScores)
	}
	if reflect.DeepEqual(first.Snapshot().Plastic.State.Plastic, initial.Plastic.State.Plastic) {
		t.Fatal("nonzero seeded fast state did not undergo ordinary Act decay")
	}
}

func TestAdaptiveFeedbackPermissionUnavailableOrOtherKind(t *testing.T) {
	cases := []struct {
		name      string
		available []string
		allowed   []string
	}{
		{name: "score available but item disallows", available: []string{"score"}},
		{name: "correctness only", available: []string{"correctness"}, allowed: []string{"correctness"}},
		{name: "score available but item allows correctness only", available: []string{"score", "correctness"}, allowed: []string{"correctness"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			initial := feedbackPermissionIndividual(t, .25).Snapshot()
			withoutUsableScore := feedbackPermissionEvaluation(tc.available,
				feedbackPermissionItem("adapt-a", 0, tc.allowed...),
				feedbackPermissionItem("adapt-b", 0, tc.allowed...),
			)
			changedTargets := feedbackPermissionEvaluation(tc.available,
				feedbackPermissionItem("adapt-a", 100, tc.allowed...),
				feedbackPermissionItem("adapt-b", 100, tc.allowed...),
			)

			firstReport, first := runFeedbackPermission(t, initial, withoutUsableScore)
			secondReport, second := runFeedbackPermission(t, initial, changedTargets)
			assertFeedbackPermissionEquivalent(t, firstReport, secondReport, first, second)
			if !reflect.DeepEqual(firstReport.FeedbackAvailable, tc.available) {
				t.Fatalf("feedback metadata = %v, want %v", firstReport.FeedbackAvailable, tc.available)
			}

			oracle, wantScores := feedbackPermissionClosedGateOracle(t, initial, withoutUsableScore)
			if !reflect.DeepEqual(first.Snapshot(), oracle.Snapshot()) {
				t.Fatalf("%s triggered a score update despite no usable score permission:\n got=%+v\nwant=%+v", tc.name, first.Snapshot(), oracle.Snapshot())
			}
			if !reflect.DeepEqual(firstReport.Scores, wantScores) {
				t.Fatalf("%s changed scoring outputs:\n got=%+v\nwant=%+v", tc.name, firstReport.Scores, wantScores)
			}
		})
	}
}

func TestAdaptiveFeedbackPermissionMixedItems(t *testing.T) {
	initial := feedbackPermissionIndividual(t, 0).Snapshot()
	mixed := feedbackPermissionEvaluation([]string{"score"},
		feedbackPermissionItem("allowed-first", 0, "score"),
		feedbackPermissionItem("forbidden-second", 0),
	)
	changedForbiddenTarget := feedbackPermissionEvaluation([]string{"score"},
		feedbackPermissionItem("allowed-first", 0, "score"),
		feedbackPermissionItem("forbidden-second", 100),
	)

	firstReport, first := runFeedbackPermission(t, initial, mixed)
	secondReport, second := runFeedbackPermission(t, initial, changedForbiddenTarget)
	assertFeedbackPermissionEquivalent(t, firstReport, secondReport, first, second)

	allForbidden := feedbackPermissionEvaluation([]string{"score"},
		feedbackPermissionItem("allowed-first", 0),
		feedbackPermissionItem("forbidden-second", 0),
	)
	controlReport, control := runFeedbackPermission(t, initial, allForbidden)
	if reflect.DeepEqual(first.Snapshot().Plastic.State, control.Snapshot().Plastic.State) {
		t.Fatalf("allowed first item had no effect; mixed permissions may have disabled all adaptation:\n mixed=%+v\ncontrol=%+v", first.Snapshot().Plastic.State, control.Snapshot().Plastic.State)
	}
	if !firstReport.Contamination.ParametersUnchanged || !controlReport.Contamination.ParametersUnchanged {
		t.Fatal("mixed permission adaptation changed base parameters")
	}
}

func TestAdaptiveFeedbackPermissionAllowedScoreEffective(t *testing.T) {
	initial := feedbackPermissionIndividual(t, 0).Snapshot()
	usefulScore := feedbackPermissionEvaluation([]string{"score"}, feedbackPermissionItem("adapt", 0, "score"))
	zeroScore := feedbackPermissionEvaluation([]string{"score"}, feedbackPermissionItem("adapt", 100, "score"))

	usefulReport, useful := runFeedbackPermission(t, initial, usefulScore)
	zeroReport, zero := runFeedbackPermission(t, initial, zeroScore)
	if !usefulReport.Contamination.ParametersUnchanged || !zeroReport.Contamination.ParametersUnchanged {
		t.Fatal("allowed score adaptation changed base parameters")
	}
	if reflect.DeepEqual(useful.Snapshot().Plastic.State, zero.Snapshot().Plastic.State) {
		t.Fatalf("useful and zero allowed scores produced the same fast state: %+v", useful.Snapshot().Plastic.State)
	}
	if reflect.DeepEqual(usefulReport.Scores, zeroReport.Scores) {
		t.Fatalf("useful and zero allowed scores produced the same later output: %+v", usefulReport.Scores)
	}
	if !reflect.DeepEqual(useful.Snapshot().Parameters, initial.Parameters) || !reflect.DeepEqual(zero.Snapshot().Parameters, initial.Parameters) {
		t.Fatal("plastic adaptation changed base parameters")
	}
}
