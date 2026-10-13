package learning_test

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/modulation"
)

func TestChemistryReleaseTotalOverflowIsAtomic(t *testing.T) {
	for _, core := range []string{"continuous", "lif"} {
		for _, source := range []string{"timeline", "controller"} {
			for _, gated := range []bool{false, true} {
				name := core + "/" + source
				if gated {
					name += "/gated"
				}
				t.Run(name, func(t *testing.T) {
					c, p := consolidationConfig(), consolidationParameters()
					if core == "lif" {
						c, p = chemLIFConfig(), chemLIFParameters()
						c.LIF.Sources, c.LIF.Targets, c.LIF.Delays = []int{0}, []int{1}, []int{0}
						p.Core.Weights = []float64{0.9}
					}
					a := newIndividual(t, c, p)
					if err := a.EnablePlasticity(plasticConfig(0)); err != nil {
						t.Fatal(err)
					}
					chem := modulation.ChemistryConfig{
						Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{1e-308}},
						Regions:   modulation.RegionAssignment{NodeRegion: []int{0, 0}},
					}
					if source == "timeline" {
						chem.Sources = []modulation.SourceSpec{{Kind: modulation.SourceExternalTimeline, Channel: 0, Timeline: &modulation.ExternalTimeline{ChannelCount: 1, Entries: []modulation.TimelineEntry{{Step: 1, Channel: 0, Rate: 1e308}, {Step: 2, Channel: 0, Rate: 1e308}}}}}
					} else {
						chem.Sources = []modulation.SourceSpec{{Kind: modulation.SourceController, Channel: 0, Controller: &modulation.Controller{Inputs: modulation.ControllerInputs{SummaryWindow: 3}, Nodes: 2, Hidden: 1, Channel: 0, Parameters: []float64{0, 0, 0, 0, 1e308}}}}
					}
					enableChemistry(t, a, chem)
					row := make([]float64, c.InputSize)
					row[0] = 2
					advance(t, a, [][]float64{row}) // establish a report and source memory
					before, reportBefore := a.Snapshot(), a.ChemistryReport()
					var out [][]float64
					var err error
					if gated {
						out, _, err = a.AdvanceGated(context.Background(), [][]float64{row, row}, []float64{1, 1})
					} else {
						out, err = a.Advance(context.Background(), [][]float64{row, row})
					}
					if err == nil || !strings.Contains(err.Error(), "release total") || out != nil {
						t.Fatalf("overflow Advance: outputs=%v error=%v, want nil outputs and release total error", out, err)
					}
					if !reflect.DeepEqual(before, a.Snapshot()) || !reflect.DeepEqual(reportBefore, a.ChemistryReport()) {
						t.Fatal("overflow committed neural, chemical, source, plastic, or report state")
					}
					if _, err := json.Marshal(a.ChemistryReport()); err != nil {
						t.Fatal(err)
					}
					advance(t, a, [][]float64{row}) // retry only the first row successfully
					if got := a.ChemistryReport().ReleaseTotal[0]; got != 1e308 {
						t.Fatalf("retry release total=%g, want 1e308", got)
					}
				})
			}
		}
	}
}

func TestChemistryReleaseTotalFiniteBoundaryAndFrozen(t *testing.T) {
	a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	chem := chemistryDeclaration()
	chem.Chemistry.Tau[0] = 1e-308
	rate := math.MaxFloat64 / 2
	chem.Sources[0].Timeline.Entries = []modulation.TimelineEntry{{Step: 0, Channel: 0, Rate: rate}, {Step: 1, Channel: 0, Rate: rate}, {Step: 2, Channel: 0, Rate: rate}}
	enableChemistry(t, a, chem)
	advance(t, a, [][]float64{{1, 0}, {1, 0}})
	if got := a.ChemistryReport().ReleaseTotal[0]; got != math.MaxFloat64 {
		t.Fatalf("finite boundary total=%g", got)
	}
	if _, err := json.Marshal(a.ChemistryReport()); err != nil {
		t.Fatal(err)
	}
	advance(t, a, [][]float64{{1, 0}})
	if got := a.ChemistryReport().ReleaseTotal[0]; got != rate {
		t.Fatalf("per-call total=%g, want %g", got, rate)
	}
	if err := a.FreezeChemistry(true); err != nil {
		t.Fatal(err)
	}
	advance(t, a, [][]float64{{1, 0}, {1, 0}})
	if got := a.ChemistryReport().ReleaseTotal[0]; got != 0 {
		t.Fatalf("frozen release=%g", got)
	}
}
