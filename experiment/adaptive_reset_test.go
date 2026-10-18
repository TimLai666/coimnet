package experiment

import (
	"context"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/plasticity"
)

// receptorGatedIndividual has chemistry enabled, a resource stored on it and a
// plasticity rule whose gate reads receptor 0, so DisableChemistry refuses
// while the plasticity stays on.
func receptorGatedIndividual(t *testing.T) *learning.Individual {
	t.Helper()
	config := learning.Config{
		Dynamics:     dynamics.Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{0}, DT: 1, Activation: "tanh"},
		InputSize:    2,
		OutputSize:   1,
		ReadoutNodes: []int{0},
		InputNodes:   []int{0, 1},
	}
	params := learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{.9}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}},
		Encoder: []float64{1, 0, 0, 1},
		Readout: []float64{1},
	}
	ind, err := learning.NewIndividual(config, params, learning.DefaultOptions(), make([]float64, 2))
	if err != nil {
		t.Fatal(err)
	}
	chemistry := modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 2, Channels: 1, DT: 1, Tau: []float64{2}},
		Sources: []modulation.SourceSpec{{
			Kind: modulation.SourceExternalTimeline, Channel: 0,
			Timeline: &modulation.ExternalTimeline{ChannelCount: 1, Entries: []modulation.TimelineEntry{{Step: 1, Channel: 0, Rate: 1}}},
		}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{
			{Cells: []int{0}, Signal: "octopamine", Channel: 0, Status: modulation.StatusHypothesized, Kd: 0.5, N: 1,
				Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chem-fixture/v1"},
			{Cells: []int{1}, Signal: "octopamine", Channel: 0, Status: modulation.StatusUnresponsive,
				Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chem-fixture/v1"},
		}},
		Regions: modulation.RegionAssignment{NodeRegion: []int{0, 1}},
	}
	if err := ind.EnableChemistry(chemistry); err != nil {
		t.Fatal(err)
	}
	gate := 0
	rule := plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 8, WMin: .0625, GateReceptor: &gate, GateScale: 1}
	if err := ind.EnablePlasticity(plasticity.Config{Rule: rule, Edges: []int{0}}); err != nil {
		t.Fatal(err)
	}
	if err := ind.SetResource("food", 1); err != nil {
		t.Fatal(err)
	}
	return ind
}

// TestResetForItemClearsChemistryUnderReceptorGatedPlasticity checks that a
// chemical reset really drops the stored resources when the plasticity rule
// reads a receptor, instead of keeping them because DisableChemistry refused.
func TestResetForItemClearsChemistryUnderReceptorGatedPlasticity(t *testing.T) {
	ind := receptorGatedIndividual(t)
	before := ind.Snapshot()
	reset := ResetPolicy{PlasticAtItemStart: true, ChemicalAtItemStart: true}
	if err := resetForItem(context.Background(), ind, reset, "item"); err != nil {
		t.Fatalf("resetForItem: %v", err)
	}
	after := ind.Snapshot()
	if after.Chemical == nil || len(after.Chemical.Resources) != 0 {
		t.Fatalf("chemical reset kept resources: %+v", after.Chemical)
	}
	if after.Plastic == nil || after.Plastic.Config.Rule.GateReceptor == nil || *after.Plastic.Config.Rule.GateReceptor != *before.Plastic.Config.Rule.GateReceptor {
		t.Fatalf("plasticity was not restored with its receptor gate: %+v", after.Plastic)
	}
}

// TestResetForItemRejectsChemicalOnlyResetUnderReceptorGatedPlasticity checks
// that a chemical-only reset fails loudly when it cannot run without also
// resetting the plasticity that reads a receptor.
func TestResetForItemRejectsChemicalOnlyResetUnderReceptorGatedPlasticity(t *testing.T) {
	ind := receptorGatedIndividual(t)
	err := resetForItem(context.Background(), ind, ResetPolicy{ChemicalAtItemStart: true}, "item")
	if err == nil || !strings.Contains(err.Error(), "receptor") {
		t.Fatalf("resetForItem error = %v, want the receptor refusal", err)
	}
	if got := ind.Snapshot().Chemical.Resources["food"]; got != 1 {
		t.Fatalf("refused reset changed the resource to %v", got)
	}
}
