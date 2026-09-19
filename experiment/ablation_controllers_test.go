package experiment

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// ablationControllerConfig is the five-group MOD-10 fixture: the same three
// seeds, training budget and held-out budget the three earlier groups run on,
// with the two controller groups appended. ControllerHidden and ControllerRate
// stay zero so RunAblation's normalization to 4 and 0.05 is exercised.
func ablationControllerConfig() AblationConfig {
	return AblationConfig{
		Seeds:        []uint64{7, 42, 123},
		Episodes:     6,
		EvalEpisodes: 8,
		Groups: []string{
			GroupNoModulation, GroupDirectReward, GroupFixedDecay,
			GroupTrainableController, GroupCapacityMatched,
		},
	}
}

// ablationTrainingEpisodes is the training split every group shares.
func ablationTrainingEpisodes(c AblationConfig) []Episode {
	episodes := make([]Episode, 0, c.Episodes)
	for e := 0; e < c.Episodes; e++ {
		episodes = append(episodes, DelayedEpisode(ablationTrainSeed, uint64(e)))
	}
	return episodes
}

func ablationGroup(t *testing.T, report AblationReport, group string) GroupSummary {
	t.Helper()
	for _, g := range report.Groups {
		if g.Group == group {
			return g
		}
	}
	t.Fatalf("report has no %q group", group)
	return GroupSummary{}
}

// TestAblationControllerParametersMatch is the capacity claim of MOD-10: the
// trainable controller and its capacity-matched control carry the same number
// of parameters, and that number is the individual's own count plus the
// controller's, so the comparison is not a comparison of model sizes.
func TestAblationControllerParametersMatch(t *testing.T) {
	report, err := RunAblation(context.Background(), ablationControllerConfig())
	if err != nil {
		t.Fatal(err)
	}
	controller, err := ablationController(7, 4)
	if err != nil {
		t.Fatalf("reference controller: %v", err)
	}
	base := ablationGroup(t, report, GroupNoModulation)
	want := base.Parameters + controller.ParameterCount()
	for _, group := range []string{GroupTrainableController, GroupCapacityMatched} {
		summary := ablationGroup(t, report, group)
		if summary.Parameters != want {
			t.Fatalf("group %q reports %d parameters, want %d (%d + %d)", group, summary.Parameters, want, base.Parameters, controller.ParameterCount())
		}
		for _, run := range summary.Runs {
			if run.Parameters != want {
				t.Fatalf("group %q seed %d reports %d parameters, want %d", group, run.Seed, run.Parameters, want)
			}
		}
	}
}

// TestAblationControllerContextHasNoTarget is the separation claim: what the
// controller reads after an episode carries no target field at all, and the
// only score in it is the previous episode's, which does not exist yet before
// the first episode has run.
func TestAblationControllerContextHasNoTarget(t *testing.T) {
	c := ablationControllerConfig()
	c.LearningRate = .02
	ind, _, err := ablationIndividual(7, GroupNoModulation, c)
	if err != nil {
		t.Fatalf("individual: %v", err)
	}
	first, err := controllerContext(ind, 0, nil)
	if err != nil {
		t.Fatalf("episode 0 context: %v", err)
	}
	shape := reflect.TypeOf(first)
	for i := 0; i < shape.NumField(); i++ {
		if strings.Contains(shape.Field(i).Name, "Target") {
			t.Fatalf("the controller context carries field %q", shape.Field(i).Name)
		}
	}
	if len(first.Feedback) != 0 {
		t.Fatalf("episode 0 carries %d feedback, want none before the first score exists", len(first.Feedback))
	}
	score := .25
	second, err := controllerContext(ind, 1, &score)
	if err != nil {
		t.Fatalf("episode 1 context: %v", err)
	}
	if len(second.Feedback) != 1 {
		t.Fatalf("episode 1 carries %d feedback, want the previous episode's score", len(second.Feedback))
	}
	if got := second.Feedback[0].Score(); got != score {
		t.Fatalf("episode 1 feedback score %v, want %v", got, score)
	}
}

// TestAblationTrainableControllerChangesRelease is the training claim: six
// episodes of the declared objective move the controller's own parameters, and
// the group they drive is not the unmodulated run under another name.
func TestAblationTrainableControllerChangesRelease(t *testing.T) {
	c := ablationControllerConfig()
	c.LearningRate, c.ControllerHidden, c.ControllerRate = .02, 4, .05
	initial, err := ablationController(7, c.ControllerHidden)
	if err != nil {
		t.Fatalf("initial controller: %v", err)
	}
	_, trained, _, err := trainAblationController(context.Background(), c, 7, ablationTrainingEpisodes(c))
	if err != nil {
		t.Fatalf("trainable controller seed 7: %v", err)
	}
	if slices.Equal(initial.Parameters, trained.Parameters) {
		t.Fatal("six episodes left the controller's parameters untouched; the update did nothing")
	}

	report, err := RunAblation(context.Background(), ablationControllerConfig())
	if err != nil {
		t.Fatal(err)
	}
	base, group := ablationGroup(t, report, GroupNoModulation), ablationGroup(t, report, GroupTrainableController)
	if group.Runs[0].Failed {
		t.Fatalf("trainable_controller seed %d failed: %s", group.Runs[0].Seed, group.Runs[0].Error)
	}
	if group.Runs[0].ActivityDelta <= 0 && group.Runs[0].Score == base.Runs[0].Score {
		t.Fatalf("trainable_controller seed %d is the no_modulation run: delta %v, score %v", group.Runs[0].Seed, group.Runs[0].ActivityDelta, group.Runs[0].Score)
	}
}

// TestAblationFiveGroupsDeterministic runs the whole five-group protocol twice
// and requires the same document, with every seed of every group succeeded.
func TestAblationFiveGroupsDeterministic(t *testing.T) {
	c := ablationControllerConfig()
	first, err := RunAblation(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RunAblation(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	a, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatal("two identical five-group runs produced different JSON")
	}
	if len(first.Groups) != len(c.Groups) {
		t.Fatalf("got %d groups, want %d", len(first.Groups), len(c.Groups))
	}
	for _, g := range first.Groups {
		if g.Succeeded != len(c.Seeds) {
			t.Fatalf("group %q succeeded on %d of %d seeds", g.Group, g.Succeeded, g.Total)
		}
		t.Logf("group %-22s mean %+.6f std %.6f parameters %d", g.Group, g.Mean, g.Std, g.Parameters)
	}
}
