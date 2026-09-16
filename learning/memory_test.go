package learning_test

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
)

// The memory fixture is the chemistry fixture's two isolated tanh neurons on
// one region and one channel, with one hypothesized receptor on node 0
// (Kd = 0.5, n = 1) and an external timeline that releases a single pulse of 1
// at step 1. Node 1 is the readout and receives the second encoder column, so
// its output is non-zero on every row. The occupancy table is the chemistry
// fixture's own: row 1 is 0.611481100422978, row 2 is 0.48838764641507565,
// row 3 is 0.3666866235902634.
const memoryOcc1 = 0.611481100422978

func memoryContinuousConfig() learning.Config {
	return learning.Config{
		Dynamics:     dynamics.Config{Nodes: 2, Sources: []int{}, Targets: []int{}, Delays: []int{}, DT: 1, Activation: "tanh"},
		InputSize:    2,
		OutputSize:   1,
		ReadoutNodes: []int{1},
		InputNodes:   []int{0, 1},
	}
}

func memoryContinuousParameters() learning.Parameters {
	return learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}},
		Encoder: []float64{1, 0, 0, 1},
		Readout: []float64{1},
	}
}

func memoryDeclaration() modulation.ChemistryConfig {
	return modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{2}},
		Sources: []modulation.SourceSpec{{
			Kind: modulation.SourceExternalTimeline, Channel: 0,
			Timeline: &modulation.ExternalTimeline{ChannelCount: 1, Entries: []modulation.TimelineEntry{{Step: 1, Channel: 0, Rate: 1}}},
		}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{
			{Cells: []int{0}, Signal: "octopamine", Channel: 0, Status: modulation.StatusHypothesized, Kd: 0.5, N: 1,
				Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chem-fixture/v1"},
		}},
		Regions: modulation.RegionAssignment{NodeRegion: []int{0, 0}},
	}
}

// memoryInput drives node 1 through the second encoder column on every row, so
// the readout is non-zero and the gain is observable.
func memoryInput() [][]float64 {
	return [][]float64{{0, 1}, {0, 1}, {0, 1}, {0, 1}}
}

func memoryExpression() learning.ExpressionGain {
	return learning.ExpressionGain{Nodes: []int{1}, Receptor: 0, Scale: -0.5, Min: 0.5, Max: 1}
}

// TestExpressionGainScalesOnlyTheReadout runs one individual with the gain and
// its restored twin without it, and compares the two readouts row by row. Row 0
// has zero occupancy, so the gain is exactly 1 and the outputs are bit for bit
// the twin's. Later rows carry the occupancy of their own row: the selected
// value of node 1 is multiplied by clamp(1 - 0.5*occ, 0.5, 1) before the
// readout matmul, and nothing else moves. The readout crosses Insyra's float32
// boundary, so the scaled outputs are compared at float32 resolution; the
// occupancy constant is float64 and is checked at 1e-12.
func TestExpressionGainScalesOnlyTheReadout(t *testing.T) {
	a := newIndividual(t, memoryContinuousConfig(), memoryContinuousParameters())
	enableChemistry(t, a, memoryDeclaration())
	twin, err := learning.RestoreIndividual(a.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetExpressionGain(memoryExpression()); err != nil {
		t.Fatal(err)
	}

	got := advance(t, a, memoryInput())
	want := advance(t, twin, memoryInput())

	// Row 0: occupancy is zero, so the gain stays exactly 1 and the readout is
	// bit for bit the twin's.
	if !sameRows(got[:1], want[:1]) {
		t.Fatalf("row 0 outputs at gain 1 = %v, want %v", got[0], want[0])
	}
	// The per-row occupancy is the fixture's own row-1 table value.
	probe := newIndividual(t, memoryContinuousConfig(), memoryContinuousParameters())
	enableChemistry(t, probe, memoryDeclaration())
	advance(t, probe, memoryInput()[:2])
	if !closeEnough(probe.ChemistryReport().Occupancy[0].Occupancy, memoryOcc1) {
		t.Fatalf("row-1 occupancy = %.17g, want %.17g", probe.ChemistryReport().Occupancy[0].Occupancy, memoryOcc1)
	}
	// Row 1: each readout value is the twin's value scaled by the row's gain.
	gain := 1 - 0.5*memoryOcc1
	for i := range got[1] {
		wantScaled := want[1][i] * gain
		if math.Abs(got[1][i]-wantScaled) > 1e-7 {
			t.Fatalf("row 1 output %d = %.17g, want the twin's %.17g times %.17g = %.17g at float32 resolution", i, got[1][i], want[1][i], gain, wantScaled)
		}
	}
	if report := a.ChemistryReport(); report.ExpressionGainApplied < 1 {
		t.Fatalf("expression_gain_applied = %d, want at least 1", report.ExpressionGainApplied)
	}
	// The gain must leave every part of the snapshot alone: the two individuals
	// differ only in the declared expression.
	gotSnap, wantSnap := a.Snapshot(), twin.Snapshot()
	if gotSnap.Chemical == nil || wantSnap.Chemical == nil {
		t.Fatal("both individuals should carry a chemical part")
	}
	gotSnap.Chemical.Expression, wantSnap.Chemical.Expression = nil, nil
	if !reflect.DeepEqual(gotSnap, wantSnap) {
		t.Fatalf("the gain changed the snapshot:\n got  %+v\n want %+v", gotSnap, wantSnap)
	}
}

// TestExpressionGainStateSwitchIsNotForgetting runs the gain in a high
// occupancy stretch so the readout is suppressed, clears it, and runs one more
// row: the forward returns bit for bit to the individual that never had it,
// because the gain is suppressed expression, not a learned change of state.
func TestExpressionGainStateSwitchIsNotForgetting(t *testing.T) {
	a := newIndividual(t, memoryContinuousConfig(), memoryContinuousParameters())
	enableChemistry(t, a, memoryDeclaration())
	twin, err := learning.RestoreIndividual(a.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetExpressionGain(memoryExpression()); err != nil {
		t.Fatal(err)
	}
	suppressed := advance(t, a, memoryInput())
	baseline := advance(t, twin, memoryInput())
	if sameRows(suppressed, baseline) {
		t.Fatal("the declared gain changed nothing, so this fixture proves no suppression")
	}
	a.ClearExpressionGain()
	next := advance(t, a, [][]float64{{0, 1}})
	reference := advance(t, twin, [][]float64{{0, 1}})
	if !sameRows(next, reference) {
		t.Fatalf("the row after clearing the gain = %v, want the twin's %v bit for bit", next, reference)
	}
	if s := a.Snapshot(); s.Chemical.Expression != nil {
		t.Fatalf("ClearExpressionGain left %+v behind", s.Chemical.Expression)
	}
}

// TestExpressionGainValidation refuses each invalid declaration and a gain
// without an enabled chemistry.
func TestExpressionGainValidation(t *testing.T) {
	valid := memoryExpression()
	for _, tc := range []struct {
		name string
		g    learning.ExpressionGain
		want string
	}{
		{"node not a readout", learning.ExpressionGain{Nodes: []int{0}, Receptor: valid.Receptor, Scale: valid.Scale, Min: valid.Min, Max: valid.Max}, "readout"},
		{"receptor out of range", learning.ExpressionGain{Nodes: valid.Nodes, Receptor: 5, Scale: valid.Scale, Min: valid.Min, Max: valid.Max}, "receptor"},
		{"min above one", learning.ExpressionGain{Nodes: valid.Nodes, Receptor: valid.Receptor, Scale: valid.Scale, Min: 1.5, Max: 2}, "min"},
	} {
		a := newIndividual(t, memoryContinuousConfig(), memoryContinuousParameters())
		enableChemistry(t, a, memoryDeclaration())
		err := a.SetExpressionGain(tc.g)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: SetExpressionGain() error = %v, want one mentioning %q", tc.name, err, tc.want)
		}
	}

	noChem := newIndividual(t, memoryContinuousConfig(), memoryContinuousParameters())
	if err := noChem.SetExpressionGain(valid); err == nil || !strings.Contains(err.Error(), "chemistry") {
		t.Fatalf("SetExpressionGain() without chemistry, error = %v", err)
	}
}

// TestExpressionGainSurvivesSnapshot carries the declaration through a
// snapshot, a restore and another snapshot unchanged.
func TestExpressionGainSurvivesSnapshot(t *testing.T) {
	a := newIndividual(t, memoryContinuousConfig(), memoryContinuousParameters())
	enableChemistry(t, a, memoryDeclaration())
	g := memoryExpression()
	if err := a.SetExpressionGain(g); err != nil {
		t.Fatal(err)
	}
	restored, err := learning.RestoreIndividual(a.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	got := restored.Snapshot().Chemical.Expression
	if got == nil || !reflect.DeepEqual(*got, g) {
		t.Fatalf("restored expression = %+v, want %+v", got, g)
	}
}
