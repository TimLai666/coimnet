package learning_test

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
)

// chemicalSourceStateControllerConfig is a deliberately small continuous
// individual whose controller reads the two-node activity mean. There are no
// receptor effects, so the activity window can be checked against the core's
// independently stored output history.
func chemicalSourceStateControllerConfig(readoutFailure bool) (learning.Config, learning.Parameters) {
	config := learning.Config{
		Dynamics: dynamics.Config{
			Nodes: 2, Sources: []int{}, Targets: []int{}, Delays: []int{}, DT: 1, Activation: "tanh",
		},
		InputSize: 2, OutputSize: 1, ReadoutNodes: []int{0}, InputNodes: []int{0, 1},
	}
	parameters := learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}},
		Encoder: []float64{1, 0, 0, 1},
		Readout: []float64{1},
	}
	if readoutFailure {
		config.ReadoutNodes = []int{0, 1}
		parameters.Readout = []float64{math.MaxFloat32, math.MaxFloat32}
	}
	return config, parameters
}

func chemicalSourceStateControllerDeclaration() modulation.ChemistryConfig {
	return modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{2}},
		Sources: []modulation.SourceSpec{{
			Kind: modulation.SourceController, Channel: 0,
			Controller: &modulation.Controller{
				Inputs: modulation.ControllerInputs{SummaryWindow: 3, Resources: []string{"energy"}},
				Nodes:  2, Hidden: 1, Channel: 0,
				// InputWidth is mean, variance and energy. The controller
				// intentionally reads only the mean.
				Parameters: []float64{1, 0, 0, 0, 1, 0},
			},
		}},
		Receptors: modulation.Receptors{},
		Regions:   modulation.RegionAssignment{NodeRegion: []int{0, 0}},
	}
}

func newChemicalSourceStateControllerIndividual(t *testing.T, readoutFailure bool) *learning.Individual {
	t.Helper()
	config, parameters := chemicalSourceStateControllerConfig(readoutFailure)
	individual, err := learning.NewIndividual(config, parameters, learning.DefaultOptions(), []float64{0, 0})
	if err != nil {
		t.Fatal(err)
	}
	if err := individual.EnableChemistry(chemicalSourceStateControllerDeclaration()); err != nil {
		t.Fatal(err)
	}
	return individual
}

func newChemicalSourceStateMixedSourcesIndividual(t *testing.T) *learning.Individual {
	t.Helper()
	config, parameters := chemicalSourceStateControllerConfig(false)
	declaration := chemicalSourceStateControllerDeclaration()
	declaration.Chemistry.Channels = 2
	declaration.Chemistry.Tau = []float64{2, 2}
	declaration.Sources = append(declaration.Sources, modulation.SourceSpec{
		Kind: modulation.SourceExternalTimeline, Channel: 1,
		Timeline: &modulation.ExternalTimeline{ChannelCount: 2, Entries: []modulation.TimelineEntry{}},
	})
	individual, err := learning.NewIndividual(config, parameters, learning.DefaultOptions(), []float64{0, 0})
	if err != nil {
		t.Fatal(err)
	}
	if err := individual.EnableChemistry(declaration); err != nil {
		t.Fatal(err)
	}
	return individual
}

func chemicalSourceStateCloneSnapshot(t *testing.T, snapshot learning.IndividualSnapshot) learning.IndividualSnapshot {
	t.Helper()
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var copy learning.IndividualSnapshot
	if err := json.Unmarshal(data, &copy); err != nil {
		t.Fatal(err)
	}
	return copy
}

func chemicalSourceStateCloneRows(rows [][]float64) [][]float64 {
	if rows == nil {
		return nil
	}
	copy := make([][]float64, len(rows))
	for i, row := range rows {
		copy[i] = append([]float64(nil), row...)
	}
	return copy
}

func chemicalSourceStateCloneState(state modulation.ControllerState) modulation.ControllerState {
	return modulation.ControllerState{Window: chemicalSourceStateCloneRows(state.Window), Filled: state.Filled}
}

func chemicalSourceStateRowsEqualBits(got, want [][]float64) bool {
	if (got == nil) != (want == nil) || len(got) != len(want) {
		return false
	}
	for i := range got {
		if (got[i] == nil) != (want[i] == nil) || len(got[i]) != len(want[i]) {
			return false
		}
		for j := range got[i] {
			if math.Float64bits(got[i][j]) != math.Float64bits(want[i][j]) {
				return false
			}
		}
	}
	return true
}

func chemicalSourceStateAssertRowsBits(t *testing.T, name string, got, want [][]float64) {
	t.Helper()
	if !chemicalSourceStateRowsEqualBits(got, want) {
		t.Fatalf("%s = %#v, want %#v", name, got, want)
	}
}

func chemicalSourceStateAssertControllerWindow(t *testing.T, snapshot learning.IndividualSnapshot, wantFilled int, wantRows [][]float64) {
	t.Helper()
	if snapshot.Chemical == nil {
		t.Fatal("snapshot has no chemical part")
	}
	if len(snapshot.Chemical.Config.Sources) != len(snapshot.Chemical.SourceStates) {
		t.Fatalf("source state count = %d, want source count %d", len(snapshot.Chemical.SourceStates), len(snapshot.Chemical.Config.Sources))
	}
	if len(snapshot.Chemical.SourceStates) != 1 || snapshot.Chemical.SourceStates[0].Controller == nil {
		t.Fatalf("controller source state = %#v, want one controller entry", snapshot.Chemical.SourceStates)
	}
	controller := snapshot.Chemical.Config.Sources[0].Controller
	if controller == nil {
		t.Fatal("controller source declaration is missing")
	}
	if controller.Nodes != len(wantRows[0]) {
		t.Fatalf("controller.Nodes = %d, want %d", controller.Nodes, len(wantRows[0]))
	}
	state := snapshot.Chemical.SourceStates[0].Controller
	if state.Filled != wantFilled {
		t.Fatalf("controller state Filled = %d, want %d", state.Filled, wantFilled)
	}
	chemicalSourceStateAssertRowsBits(t, "controller state Window", state.Window, wantRows)
	for row, values := range state.Window {
		if len(values) != controller.Nodes {
			t.Fatalf("controller state Window[%d] width = %d, want %d", row, len(values), controller.Nodes)
		}
	}
}

func chemicalSourceStateWindowAfter(observed [][]float64, size, nodes int) [][]float64 {
	window := make([][]float64, size)
	for i := range window {
		window[i] = make([]float64, nodes)
	}
	start := len(observed) - size
	if start < 0 {
		start = 0
	}
	copyFrom := observed[start:]
	for i, row := range copyFrom {
		copy(window[size-len(copyFrom)+i], row)
	}
	return window
}

func chemicalSourceStateLatestActivity(t *testing.T, snapshot learning.IndividualSnapshot) []float64 {
	t.Helper()
	if snapshot.Neural.Continuous == nil || len(snapshot.Neural.Continuous.History) == 0 {
		t.Fatal("snapshot has no continuous activity history")
	}
	return append([]float64(nil), snapshot.Neural.Continuous.History[len(snapshot.Neural.Continuous.History)-1]...)
}

func TestChemicalSourceStateSnapshotShapeAndWire(t *testing.T) {
	controller := newChemicalSourceStateControllerIndividual(t, false)
	fresh := controller.Snapshot()
	chemicalSourceStateAssertControllerWindow(t, fresh, 0, [][]float64{{0, 0}, {0, 0}, {0, 0}})
	wire, err := json.Marshal(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(wire, []byte(`"window":null`)) || bytes.Contains(wire, []byte(`"controller":null`)) {
		t.Fatalf("fresh controller source state contains null required arrays: %s", wire)
	}

	mixed := newChemicalSourceStateMixedSourcesIndividual(t)
	mixedSnapshot := mixed.Snapshot()
	if mixedSnapshot.Chemical == nil || len(mixedSnapshot.Chemical.SourceStates) != len(mixedSnapshot.Chemical.Config.Sources) || len(mixedSnapshot.Chemical.SourceStates) != 2 {
		t.Fatalf("multi-source state alignment = %#v, want two entries", mixedSnapshot.Chemical)
	}
	if mixedSnapshot.Chemical.SourceStates[0].Controller == nil || mixedSnapshot.Chemical.SourceStates[1] != (learning.ChemicalSourceState{}) {
		t.Fatalf("multi-source controller/non-controller entries = %#v, want controller plus empty entry", mixedSnapshot.Chemical.SourceStates)
	}

	plain := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	enableChemistry(t, plain, chemistryDeclaration())
	plainSnapshot := plain.Snapshot()
	if plainSnapshot.Chemical == nil {
		t.Fatal("plain chemistry snapshot is missing")
	}
	if plainSnapshot.Chemical.SourceStates != nil {
		t.Fatalf("non-controller chemistry emitted SourceStates = %#v", plainSnapshot.Chemical.SourceStates)
	}
	plainWire, err := json.Marshal(plainSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(plainWire, []byte(`"source_states"`)) {
		t.Fatalf("non-controller chemistry changed the legacy wire: %s", plainWire)
	}
}

func TestChemicalSourceStateWindowAndResumeAreBitIdentical(t *testing.T) {
	individual := newChemicalSourceStateControllerIndividual(t, false)
	inputs := [][][]float64{{{1, 0}}, {{0, 1}}, {{1, 1}}}
	observed := make([][]float64, 0, len(inputs))
	activities := make([][]float64, 0, len(inputs))
	voltage := []float64{0, 0}
	lambda, alpha := math.Exp(-1), -math.Expm1(-1)
	for row, input := range inputs {
		before := []float64{0, 0}
		if row > 0 {
			before = append([]float64(nil), activities[row-1]...)
		}
		observed = append(observed, before)
		if _, err := individual.Advance(context.Background(), input); err != nil {
			t.Fatalf("Advance row %d: %v", row, err)
		}
		snapshot := individual.Snapshot()
		wantActivity := make([]float64, len(input[0]))
		for node, drive := range input[0] {
			voltage[node] = lambda*voltage[node] + alpha*drive
			wantActivity[node] = math.Tanh(voltage[node])
		}
		activity := chemicalSourceStateLatestActivity(t, snapshot)
		for node := range activity {
			if math.Abs(activity[node]-wantActivity[node]) > 1e-12 {
				t.Fatalf("row %d activity[%d] = %.17g, want hand-calculated %.17g", row, node, activity[node], wantActivity[node])
			}
		}
		// The source observes the output from the step before this call. The
		// saved history is the public record of that output, so build the
		// oldest-to-newest expected ring independently from the source state.
		wantFilled := row + 1
		if wantFilled > 3 {
			wantFilled = 3
		}
		wantRows := chemicalSourceStateWindowAfter(observed[:row+1], 3, 2)
		chemicalSourceStateAssertControllerWindow(t, snapshot, wantFilled, wantRows)
		activities = append(activities, wantActivity)
	}

	// Snapshot ownership includes the controller's nested rows and the other
	// mutable chemical inputs that are commonly carried beside them.
	if err := individual.SetResource("energy", 1.25); err != nil {
		t.Fatal(err)
	}
	if err := individual.OfferFeedback(stepFeedbackAt(t, 0, 10)); err != nil {
		t.Fatal(err)
	}
	first := individual.Snapshot()
	second := individual.Snapshot()
	first.Chemical.SourceStates[0].Controller.Window[0][0] = 99
	first.Chemical.Config.Sources[0].Controller.Parameters[0] = 99
	first.Chemical.Resources["energy"] = 99
	first.Chemical.PendingFeedback[0].Score = 99
	if second.Chemical.SourceStates[0].Controller.Window[0][0] == 99 || second.Chemical.Config.Sources[0].Controller.Parameters[0] == 99 || second.Chemical.Resources["energy"] == 99 || second.Chemical.PendingFeedback[0].Score == 99 {
		t.Fatal("repeated snapshots share nested chemical ownership")
	}
	live := individual.Snapshot()
	if live.Chemical.SourceStates[0].Controller.Window[0][0] == 99 || live.Chemical.Config.Sources[0].Controller.Parameters[0] == 99 || live.Chemical.Resources["energy"] == 99 || live.Chemical.PendingFeedback[0].Score == 99 {
		t.Fatal("mutating a snapshot changed the running individual")
	}

	prefix := [][]float64{{1, 0}, {0, 1}, {1, 1}}
	suffix := [][]float64{{0.5, 0}, {0, 0.5}}
	whole := newChemicalSourceStateControllerIndividual(t, false)
	if _, err := whole.Advance(context.Background(), prefix); err != nil {
		t.Fatal(err)
	}
	want, err := whole.Advance(context.Background(), suffix)
	if err != nil {
		t.Fatal(err)
	}
	split := newChemicalSourceStateControllerIndividual(t, false)
	if _, err := split.Advance(context.Background(), prefix); err != nil {
		t.Fatal(err)
	}
	mid := split.Snapshot()
	resumed, err := learning.RestoreIndividual(mid)
	if err != nil {
		t.Fatalf("RestoreIndividual: %v", err)
	}
	got, err := resumed.Advance(context.Background(), suffix)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resumed outputs differ:\n got %v\nwant %v", got, want)
	}
	if !reflect.DeepEqual(resumed.Snapshot(), whole.Snapshot()) {
		t.Fatal("resumed controller source state differs from uninterrupted state")
	}
}

func TestChemicalSourceStateLegacyResetsOnlySourceMemory(t *testing.T) {
	individual := newChemicalSourceStateControllerIndividual(t, false)
	if _, err := individual.Advance(context.Background(), [][]float64{{1, 0}, {0, 1}, {1, 1}}); err != nil {
		t.Fatal(err)
	}
	want := individual.Snapshot()
	legacy := chemicalSourceStateCloneSnapshot(t, want)
	legacy.Chemical.SourceStates = nil
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	var decoded learning.IndividualSnapshot
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	resumed, err := learning.RestoreIndividual(decoded)
	if err != nil {
		t.Fatalf("RestoreIndividual(legacy): %v", err)
	}
	got := resumed.Snapshot()
	if got.Chemical.State.Steps != want.Chemical.State.Steps || !reflect.DeepEqual(got.Chemical.State.Concentration, want.Chemical.State.Concentration) {
		t.Fatal("legacy SourceStates reset the chemical concentration or its clock")
	}
	if got.Neural.Continuous.Steps != want.Neural.Continuous.Steps {
		t.Fatalf("legacy SourceStates reset neural clock to %d, want %d", got.Neural.Continuous.Steps, want.Neural.Continuous.Steps)
	}
	chemicalSourceStateAssertControllerWindow(t, got, 0, [][]float64{{0, 0}, {0, 0}, {0, 0}})
}

func TestChemicalSourceStateRestoreRejectsMalformedBoundariesWithoutPanic(t *testing.T) {
	controller := newChemicalSourceStateControllerIndividual(t, false)
	if _, err := controller.Advance(context.Background(), [][]float64{{1, 0}}); err != nil {
		t.Fatal(err)
	}
	base := controller.Snapshot()
	if base.Chemical == nil || len(base.Chemical.SourceStates) != 1 || base.Chemical.SourceStates[0].Controller == nil {
		t.Fatalf("controller source state missing: %#v", base.Chemical)
	}
	valid := chemicalSourceStateCloneState(*base.Chemical.SourceStates[0].Controller)

	noController := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	enableChemistry(t, noController, chemistryDeclaration())
	noControllerSnapshot := noController.Snapshot()
	noControllerSnapshot.Chemical.SourceStates = []learning.ChemicalSourceState{{Controller: &valid}}
	cases := map[string]func(*learning.IndividualSnapshot){
		"wrong list length": func(s *learning.IndividualSnapshot) {
			s.Chemical.SourceStates = []learning.ChemicalSourceState{{Controller: &valid}, {Controller: &valid}}
		},
		"controller entry missing": func(s *learning.IndividualSnapshot) {
			s.Chemical.SourceStates = []learning.ChemicalSourceState{{}}
		},
		"bad filled negative": func(s *learning.IndividualSnapshot) {
			s.Chemical.SourceStates[0].Controller.Filled = -1
		},
		"bad filled over window": func(s *learning.IndividualSnapshot) {
			s.Chemical.SourceStates[0].Controller.Filled = len(s.Chemical.SourceStates[0].Controller.Window) + 1
		},
		"bad row width": func(s *learning.IndividualSnapshot) {
			s.Chemical.SourceStates[0].Controller.Window[0] = []float64{0}
		},
		"nonfinite row": func(s *learning.IndividualSnapshot) {
			s.Chemical.SourceStates[0].Controller.Window[0][0] = math.NaN()
		},
		"nonzero unfilled row": func(s *learning.IndividualSnapshot) {
			s.Chemical.SourceStates[0].Controller.Filled = 1
			s.Chemical.SourceStates[0].Controller.Window[0][0] = .25
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			broken := chemicalSourceStateCloneSnapshot(t, base)
			mutate(&broken)
			chemicalSourceStateAssertRestoreRejects(t, broken)
		})
	}
	chemicalSourceStateAssertRestoreRejects(t, noControllerSnapshot)
}

func chemicalSourceStateAssertRestoreRejects(t *testing.T, snapshot learning.IndividualSnapshot) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("RestoreIndividual panicked: %v", recovered)
		}
	}()
	if _, err := learning.RestoreIndividual(snapshot); err == nil {
		t.Fatal("RestoreIndividual accepted a malformed SourceStates boundary")
	}
}

func TestChemicalControllerTransactionAdvanceFailureKeepsSourceAndWholeBoundaryAtomic(t *testing.T) {
	newOverflowIndividual := func() *learning.Individual {
		config, parameters := chemicalSourceStateControllerConfig(false)
		config.Dynamics.Sources = []int{0, 1}
		config.Dynamics.Targets = []int{0, 0}
		config.Dynamics.Delays = []int{0, 0}
		parameters.Core.Weights = []float64{math.MaxFloat64, math.MaxFloat64}
		individual, err := learning.NewIndividual(config, parameters, learning.DefaultOptions(), []float64{0, 0})
		if err != nil {
			t.Fatal(err)
		}
		if err := individual.EnableChemistry(chemicalSourceStateControllerDeclaration()); err != nil {
			t.Fatal(err)
		}
		return individual
	}
	individual := newOverflowIndividual()
	if _, err := individual.Advance(context.Background(), [][]float64{{0, 0}}); err != nil {
		t.Fatal(err)
	}
	before := individual.Snapshot()
	report := individual.ChemistryReport()
	// Finite encoded inputs pass whole-input validation. Both previous-output terms
	// overflow only on the second row, after a controller release has occurred.
	if output, err := individual.Advance(context.Background(), [][]float64{{1, 1}, {0, 0}}); err == nil || output != nil {
		t.Fatalf("late core overflow = output %v, err %v; want refusal", output, err)
	}
	if !reflect.DeepEqual(individual.Snapshot(), before) {
		t.Fatal("late core failure committed neural, chemistry or controller source state")
	}
	if !reflect.DeepEqual(individual.ChemistryReport(), report) {
		t.Fatal("late core failure changed the previous chemistry report")
	}

	reference := newOverflowIndividual()
	if _, err := reference.Advance(context.Background(), [][]float64{{0, 0}}); err != nil {
		t.Fatal(err)
	}
	retryInput := [][]float64{{0.1, 0}, {0, 0}}
	want, err := reference.Advance(context.Background(), retryInput)
	if err != nil {
		t.Fatal(err)
	}
	got, err := individual.Advance(context.Background(), retryInput)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(individual.Snapshot(), reference.Snapshot()) {
		t.Fatal("a legal retry diverged from a fresh reference after a failed call")
	}

	readoutFailure := newChemicalSourceStateControllerIndividual(t, true)
	if _, err := readoutFailure.Advance(context.Background(), [][]float64{{0, 0}}); err != nil {
		t.Fatal(err)
	}
	readoutBefore := readoutFailure.Snapshot()
	readoutReport := readoutFailure.ChemistryReport()
	if output, err := readoutFailure.Advance(context.Background(), [][]float64{{0, 0}, {1, 1}}); err == nil || output != nil {
		t.Fatalf("final readout overflow = output %v, err %v; want refusal", output, err)
	}
	if !reflect.DeepEqual(readoutFailure.Snapshot(), readoutBefore) || !reflect.DeepEqual(readoutFailure.ChemistryReport(), readoutReport) {
		t.Fatal("final readout failure committed a partial controller or chemistry boundary")
	}
}
