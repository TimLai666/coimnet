package experiment_test

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/experiment"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/signal"
)

// TestAdaptiveChemicalReset135 checks the public evaluation boundary. Chemical
// reset starts a fresh concentration/source state for every item while task
// inputs (resources and queued feedback) survive the declaration replacement.
func TestAdaptiveChemicalReset135(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []string{experiment.EvaluationModeFixed, experiment.EvaluationModeAdaptive} {
		for _, source := range []string{modulation.SourceInternalResource, modulation.SourceExternalTimeline} {
			for _, chemicalReset := range []bool{false, true} {
				for _, gated := range []bool{false, true} {
					name := fmt.Sprintf("%s/%s/reset=%t/gated=%t", mode, source, chemicalReset, gated)
					t.Run(name, func(t *testing.T) {
						reset := experiment.ResetPolicy{NeuralAtItemStart: true, PlasticAtItemStart: true, ChemicalAtItemStart: chemicalReset}
						ind, evaluation, err := experiment.AdaptiveFixture(7, mode, reset)
						if err != nil {
							t.Fatalf("AdaptiveFixture: %v", err)
						}
						if err := ind.EnableChemistry(adaptiveChemicalReset135Config(source)); err != nil {
							t.Fatalf("EnableChemistry: %v", err)
						}
						if gated {
							pc := ind.Snapshot().Plastic.Config
							index := 0
							pc.Rule.GateReceptor = &index
							if err := ind.EnablePlasticity(pc); err != nil {
								t.Fatalf("EnablePlasticity(gate receptor): %v", err)
							}
						}
						if got := ind.Snapshot().Plastic.Config.Rule.GateReceptor != nil; got != gated {
							t.Fatalf("gate receptor present = %t, want %t", got, gated)
						}
						if err := ind.SetResource("energy", .75); err != nil {
							t.Fatalf("SetResource(energy): %v", err)
						}
						if err := ind.SetResource("water", 1.25); err != nil {
							t.Fatalf("SetResource(water): %v", err)
						}
						current := adaptiveChemicalReset135Feedback(t, "current", 0, .25)
						future := adaptiveChemicalReset135Feedback(t, "future", 100, .75)
						if err := ind.OfferFeedback(current); err != nil {
							t.Fatalf("OfferFeedback(current): %v", err)
						}
						if err := ind.OfferFeedback(future); err != nil {
							t.Fatalf("OfferFeedback(future): %v", err)
						}

						if _, err := ind.Advance(ctx, [][]float64{{.25}, {-.25}}); err != nil {
							t.Fatalf("warmup Advance: %v", err)
						}
						before := ind.Snapshot()
						if before.Chemical == nil || before.Chemical.State.Concentration[0][0] <= 0 {
							t.Fatalf("warmup concentration = %#v, want nonzero", before.Chemical)
						}
						if len(before.Chemical.PendingFeedback) != 2 {
							t.Fatalf("warmup feedback queue = %d, want 2", len(before.Chemical.PendingFeedback))
						}
						evaluation.Scoring = []experiment.EvaluationItem{
							{ID: "score-a", Input: [][]float64{{.5}}, Target: []float64{.2}},
							{ID: "score-b", Input: [][]float64{{-.5}}, Target: []float64{-.1}},
						}
						if mode == experiment.EvaluationModeAdaptive {
							evaluation.Adaptation = []experiment.EvaluationItem{{ID: "adapt", Input: [][]float64{{.25}}, Target: []float64{.1}, AllowedFeedback: []string{"score"}}}
						}
						report, err := experiment.RunAdaptiveEvaluation(ctx, ind, evaluation)
						if err != nil {
							t.Fatalf("RunAdaptiveEvaluation: %v", err)
						}
						if !report.Contamination.ParametersUnchanged {
							t.Fatal("evaluation changed base parameters")
						}
						after := ind.Snapshot()
						if after.Chemical == nil {
							t.Fatal("evaluation disabled the chemical layer")
						}
						if !reflect.DeepEqual(after.Chemical.Resources, before.Chemical.Resources) {
							t.Fatalf("resources changed across evaluation: before=%v after=%v", before.Chemical.Resources, after.Chemical.Resources)
						}
						if !reflect.DeepEqual(after.Chemical.PendingFeedback, before.Chemical.PendingFeedback) {
							t.Fatalf("pending feedback changed across evaluation: before=%v after=%v", before.Chemical.PendingFeedback, after.Chemical.PendingFeedback)
						}
						chemReport := ind.ChemistryReport()
						if chemReport.Steps != 1 || len(chemReport.ReleaseTotal) != 1 {
							t.Fatalf("last item chemistry report = %+v, want one step and one channel", chemReport)
						}
						if chemicalReset {
							want := 2 * (1 - math.Exp(-.25))
							got := after.Chemical.State.Concentration[0][0]
							if math.Abs(got-want) > 1e-15 || after.Chemical.State.Steps != 1 {
								t.Fatalf("reset first row concentration/steps = %.17g/%d, want %.17g/1", got, after.Chemical.State.Steps, want)
							}
							if report.Contamination.ShuffleInvariant == nil || !*report.Contamination.ShuffleInvariant {
								t.Fatalf("shuffle_invariant = %v, want true when all resets are enabled", report.Contamination.ShuffleInvariant)
							}
						} else if after.Chemical.State.Steps <= 1 {
							t.Fatalf("non-reset chemical steps = %d, want accumulated state", after.Chemical.State.Steps)
						}
						wantRelease := 1.0
						if math.Abs(chemReport.ReleaseTotal[0]-wantRelease) > 1e-15 {
							t.Fatalf("last release = %.17g, want %.17g for %s", chemReport.ReleaseTotal[0], wantRelease, source)
						}
					})
				}
			}
		}
	}

	t.Run("disabled control", func(t *testing.T) {
		reset := experiment.ResetPolicy{NeuralAtItemStart: true, PlasticAtItemStart: true, ChemicalAtItemStart: true}
		ind, evaluation, err := experiment.AdaptiveFixture(7, experiment.EvaluationModeFixed, reset)
		if err != nil {
			t.Fatalf("AdaptiveFixture: %v", err)
		}
		evaluation.Scoring = []experiment.EvaluationItem{{ID: "score", Input: [][]float64{{.5}}, Target: []float64{.2}}}
		if _, err := experiment.RunAdaptiveEvaluation(ctx, ind, evaluation); err != nil {
			t.Fatalf("disabled chemistry evaluation: %v", err)
		}
		if got := ind.Snapshot().Chemical; got != nil {
			t.Fatalf("disabled control Chemical = %#v, want nil", got)
		}
	})

	t.Run("missing resource is a source error", func(t *testing.T) {
		reset := experiment.ResetPolicy{ChemicalAtItemStart: true}
		ind, evaluation, err := experiment.AdaptiveFixture(7, experiment.EvaluationModeFixed, reset)
		if err != nil {
			t.Fatalf("AdaptiveFixture: %v", err)
		}
		if err := ind.EnableChemistry(adaptiveChemicalReset135Config(modulation.SourceInternalResource)); err != nil {
			t.Fatalf("EnableChemistry: %v", err)
		}
		evaluation.Scoring = []experiment.EvaluationItem{{ID: "missing", Input: [][]float64{{.5}}, Target: []float64{.2}}}
		if _, err := experiment.RunAdaptiveEvaluation(ctx, ind, evaluation); err == nil || !strings.Contains(err.Error(), "resource \"energy\" is not declared") {
			t.Fatalf("missing-resource evaluation error = %v, want a declared-resource error", err)
		}
	})
}

func adaptiveChemicalReset135Config(source string) modulation.ChemistryConfig {
	spec := modulation.SourceSpec{Kind: source, Channel: 0}
	if source == modulation.SourceInternalResource {
		spec.Resource = &modulation.InternalResource{Resource: "energy", Coefficient: 2, Threshold: .25, Channel: 0}
	} else {
		spec.Timeline = &modulation.ExternalTimeline{ChannelCount: 1, Entries: []modulation.TimelineEntry{{Step: 0, Channel: 0, Rate: 1}}}
	}
	return modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: .5, Tau: []float64{2}},
		Sources:   []modulation.SourceSpec{spec},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{{Cells: []int{0}, Signal: "octopamine", Channel: 0, Status: modulation.StatusHypothesized, Kd: .5, N: 1, Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chemical-reset-135/v1"}}},
		Regions:   modulation.RegionAssignment{NodeRegion: []int{0, 0}},
	}
}

func adaptiveChemicalReset135Feedback(t *testing.T, id string, available int64, score float64) signal.Feedback {
	t.Helper()
	f, err := signal.NewFeedback(signal.FeedbackSpec{
		SchemaVersion: signal.CurrentSchemaVersion(), ExperienceID: id, ActionID: id + "-action",
		ProducedAt:  signal.Timestamp{Value: 0, Unit: signal.TimeUnitModelStep},
		AvailableAt: signal.Timestamp{Value: available, Unit: signal.TimeUnitModelStep},
		Source:      "chemical-reset-135", Score: score, ModelVersion: signal.CurrentSchemaVersion(),
	})
	if err != nil {
		t.Fatalf("NewFeedback(%q): %v", id, err)
	}
	return f
}
