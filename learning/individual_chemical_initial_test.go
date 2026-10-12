package learning_test

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
)

type individualChemicalInitialCase struct {
	name   string
	warmup bool
}

// TestIndividualChemicalInitialActivityMatchesStandaloneZeroBoundary checks
// the public bridge at both boundaries where previousActivity is nil: a fresh
// individual and an individual whose persistent neural trajectory was reset.
// The standalone trainer receives the explicit zero activity that the
// persistent controller observes when no prior neural row exists.
func TestIndividualChemicalInitialActivityMatchesStandaloneZeroBoundary(t *testing.T) {
	for _, tc := range []individualChemicalInitialCase{
		{name: "fresh", warmup: false},
		{name: "warmup_then_reset", warmup: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			individual, snapshot := newIndividualChemicalInitialFixture(t, tc.warmup)
			if snapshot.Chemical == nil {
				t.Fatal("fixture snapshot has no persistent chemical boundary")
			}
			if snapshot.Optimizer.Updates != 0 {
				t.Fatalf("fixture optimizer updates = %d, want zero before comparison", snapshot.Optimizer.Updates)
			}

			before := individual.Snapshot()
			beforeReport := individual.ChemistryReport()
			input := [][]float64{{0.4}}
			target := []float64{0.25}

			// A nil InitialActivity is a control for standalone semantics. The
			// core starts at voltage zero, so its initial activated output is
			// softplus(0). Keeping this case equal protects that public default
			// while the Individual bridge uses an explicit zero boundary.
			if snapshot.Neural.Continuous == nil || len(snapshot.Neural.Continuous.History) == 0 {
				t.Fatalf("fixture continuous history = %#v, want an initial activated row", snapshot.Neural.Continuous)
			}
			coreInitial := append([]float64(nil), snapshot.Neural.Continuous.History[len(snapshot.Neural.Continuous.History)-1]...)
			nilTrainer := newIndividualChemicalInitialStandalone(t, snapshot, nil)
			defer nilTrainer.Close()
			coreInitialTrainer := newIndividualChemicalInitialStandalone(t, snapshot, coreInitial)
			defer coreInitialTrainer.Close()
			nilResult, err := nilTrainer.Step(context.Background(), input, target)
			if err != nil {
				t.Fatalf("standalone nil InitialActivity Step: %v", err)
			}
			coreInitialResult, err := coreInitialTrainer.Step(context.Background(), input, target)
			if err != nil {
				t.Fatalf("standalone core-initial InitialActivity Step: %v", err)
			}
			if !reflect.DeepEqual(nilResult, coreInitialResult) {
				t.Fatalf("standalone nil/default result differs from explicit core initial activity:\n nil=%+v\n explicit=%+v", nilResult, coreInitialResult)
			}
			assertIndividualChemicalInitialTrainerState(t, nilTrainer.Snapshot(), coreInitialTrainer.Snapshot())

			// The fixture must expose the bridge bug independently of the
			// Individual assertion: implicit softplus(0) and explicit zero
			// activity must produce a visible loss difference.
			zeroTrainer := newIndividualChemicalInitialStandalone(t, snapshot, []float64{0})
			defer zeroTrainer.Close()
			zeroResult, err := zeroTrainer.Step(context.Background(), input, target)
			if err != nil {
				t.Fatalf("standalone zero InitialActivity Step: %v", err)
			}
			if math.Abs(nilResult.Loss-zeroResult.Loss) <= 1e-9 {
				t.Fatalf("fixture did not make nil-versus-zero activity visible: nil loss %.17g, zero loss %.17g", nilResult.Loss, zeroResult.Loss)
			}

			reference := newIndividualChemicalInitialStandalone(t, snapshot, []float64{0})
			defer reference.Close()
			want, err := reference.Step(context.Background(), input, target)
			if err != nil {
				t.Fatalf("standalone zero-boundary Step: %v", err)
			}
			got, err := individual.TrainEpisode(context.Background(), input, target)
			if err != nil {
				t.Fatalf("Individual.TrainEpisode: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Individual.TrainEpisode result differs from explicit-zero standalone reference:\n got=%+v\nwant=%+v", got, want)
			}
			after := individual.Snapshot()
			assertIndividualChemicalInitialTrainerState(t, individualChemicalInitialTrainingSnapshot(after), reference.Snapshot())
			assertIndividualChemicalInitialPersistentBoundary(t, before, after, beforeReport, individual.ChemistryReport())
			if after.Optimizer.Episodes != before.Optimizer.Episodes+1 {
				t.Fatalf("episode counter = %d, want %d", after.Optimizer.Episodes, before.Optimizer.Episodes+1)
			}
		})
	}
}

func newIndividualChemicalInitialFixture(t *testing.T, warmup bool) (*learning.Individual, learning.IndividualSnapshot) {
	t.Helper()
	config := individualChemicalInitialConfig()
	parameters := individualChemicalInitialParameters()
	options := learning.DefaultOptions()
	chemistry := individualChemicalInitialChemistry()
	if err := chemistry.Validate(1); err != nil {
		t.Fatalf("chemical fixture Validate: %v", err)
	}
	individualChemicalInitialAssertControllerActivity(t, chemistry)
	individualChemicalInitialAssertReceptorEffect(t, chemistry)

	individual, err := learning.NewIndividual(config, parameters, options, []float64{0})
	if err != nil {
		t.Fatalf("NewIndividual: %v", err)
	}
	if err := individual.EnableChemistry(chemistry); err != nil {
		t.Fatalf("EnableChemistry: %v", err)
	}
	if warmup {
		if _, err := individual.Advance(context.Background(), [][]float64{{0.4}}); err != nil {
			t.Fatalf("warmup Advance: %v", err)
		}
		if err := individual.ResetNeural(context.Background(), []float64{0}); err != nil {
			t.Fatalf("ResetNeural: %v", err)
		}
	}
	snapshot := individual.Snapshot()
	if snapshot.Chemical == nil {
		t.Fatal("EnableChemistry snapshot has no Chemical part")
	}
	if snapshot.Neural.Continuous == nil || snapshot.Neural.Continuous.Steps != 0 {
		t.Fatalf("fixture neural boundary = %#v, want continuous Steps=0", snapshot.Neural)
	}
	if len(snapshot.Chemical.Config.Sources) != 1 || snapshot.Chemical.Config.Sources[0].Controller == nil {
		t.Fatalf("fixture chemical sources = %#v, want one controller", snapshot.Chemical.Config.Sources)
	}
	controllerState := snapshot.Chemical.SourceStates
	if len(controllerState) != 1 || controllerState[0].Controller == nil || len(controllerState[0].Controller.Window) != 2 {
		t.Fatalf("fixture source state = %#v, want a two-row controller window", controllerState)
	}
	return individual, snapshot
}

func individualChemicalInitialConfig() learning.Config {
	return learning.Config{
		Dynamics: dynamics.Config{
			Nodes: 1, Sources: []int{}, Targets: []int{}, Delays: []int{}, DT: 1, Activation: "softplus",
		},
		InputSize: 1, OutputSize: 1, ReadoutNodes: []int{0}, InputNodes: []int{0},
	}
}

func individualChemicalInitialParameters() learning.Parameters {
	return learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{}, Bias: []float64{0.1}, LogTau: []float64{0}},
		Encoder: []float64{1}, Readout: []float64{1},
	}
}

func individualChemicalInitialChemistry() modulation.ChemistryConfig {
	return modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{2}},
		Sources: []modulation.SourceSpec{{
			Kind: modulation.SourceController, Channel: 0,
			Controller: &modulation.Controller{
				Inputs: modulation.ControllerInputs{SummaryWindow: 2},
				Nodes:  1, Hidden: 1, Channel: 0,
				// InputWidth is mean plus variance. The mean weight and
				// readout weight make activity change the release rate.
				Parameters: []float64{2, 0, 0, 1, 0},
			},
		}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{{
			Cells: []int{0}, Signal: "fixture", Channel: 0,
			Status: modulation.StatusHypothesized, Kd: 0.5, N: 1,
			Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "initial-activity/v1",
			Location: &modulation.ReceptorLocation{
				Region: 0, Compartment: "soma", AnatomicalLocation: "fixture-soma",
				Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "initial-activity/location-v1",
			},
		}}},
		Effects: []modulation.Effect{{
			Kind: modulation.EffectSensitivity, Receptor: 0, GammaScale: 0.5, BetaScale: 0.8,
		}},
		Regions: modulation.RegionAssignment{NodeRegion: []int{0}},
	}
}

func individualChemicalInitialAssertControllerActivity(t *testing.T, chemistry modulation.ChemistryConfig) {
	t.Helper()
	controller := chemistry.Sources[0].Controller
	if controller == nil {
		t.Fatal("controller fixture is nil")
	}
	zero := controller.Clone()
	if err := zero.RestoreState(modulation.ControllerState{Window: [][]float64{{0}, {0}}, Filled: 0}); err != nil {
		t.Fatalf("restore zero controller state: %v", err)
	}
	active := controller.Clone()
	if err := active.RestoreState(modulation.ControllerState{Window: [][]float64{{0}, {0}}, Filled: 0}); err != nil {
		t.Fatalf("restore active controller state: %v", err)
	}
	zeroRelease, err := zero.Release(0, modulation.SourceContext{Activity: []float64{0}})
	if err != nil {
		t.Fatalf("zero controller release: %v", err)
	}
	nilActivity := controller.Clone()
	if err := nilActivity.RestoreState(modulation.ControllerState{Window: [][]float64{{0}, {0}}, Filled: 0}); err != nil {
		t.Fatalf("restore nil-activity controller state: %v", err)
	}
	nilRelease, err := nilActivity.Release(0, modulation.SourceContext{})
	if err != nil {
		t.Fatalf("nil-activity controller release: %v", err)
	}
	if !reflect.DeepEqual(nilRelease, zeroRelease) {
		t.Fatalf("controller nil activity did not use the zero row: nil=%v zero=%v", nilRelease, zeroRelease)
	}
	activeRelease, err := active.Release(0, modulation.SourceContext{Activity: []float64{math.Log1p(math.Exp(0))}})
	if err != nil {
		t.Fatalf("active controller release: %v", err)
	}
	if len(zeroRelease) != 1 || len(activeRelease) != 1 || math.Abs(zeroRelease[0]-activeRelease[0]) <= 1e-9 {
		t.Fatalf("controller activity did not change release: zero=%v active=%v", zeroRelease, activeRelease)
	}
}

func individualChemicalInitialAssertReceptorEffect(t *testing.T, chemistry modulation.ChemistryConfig) {
	t.Helper()
	gain, offset, _, _, err := modulation.ApplyEffects(
		chemistry.Effects,
		[]modulation.OccupancyRecord{{Receptor: 0, Cell: 0, Region: 0, Channel: 0, Status: modulation.StatusHypothesized, Occupancy: 0.5}},
		chemistry.Receptors.Mix,
		1,
	)
	if err != nil {
		t.Fatalf("receptor effect fixture ApplyEffects: %v", err)
	}
	if len(gain) != 1 || len(offset) != 1 || (gain[0] == 1 && offset[0] == 0) {
		t.Fatalf("receptor effect fixture is neutral: gain=%v offset=%v", gain, offset)
	}
}

func newIndividualChemicalInitialStandalone(t *testing.T, snapshot learning.IndividualSnapshot, initial []float64) *learning.Trainer {
	t.Helper()
	if snapshot.Chemical == nil {
		t.Fatal("standalone fixture has no Chemical part")
	}
	episode := &learning.ChemicalEpisode{
		Part:            *snapshot.Chemical,
		SourceStep:      0,
		InitialActivity: append([]float64(nil), initial...),
	}
	config := snapshot.Config
	config.Chemistry = episode
	trainer, err := learning.NewTrainer(config, snapshot.Parameters, snapshot.Optimizer.Options)
	if err != nil {
		t.Fatalf("NewTrainer standalone chemistry: %v", err)
	}
	return trainer
}

func assertIndividualChemicalInitialTrainerState(t *testing.T, actual learning.TrainingSnapshot, reference learning.TrainingSnapshot) {
	t.Helper()
	if !reflect.DeepEqual(actual.Parameters, reference.Parameters) {
		t.Fatal("parameters differ from standalone trainer")
	}
	if !reflect.DeepEqual(actual.Optimizer, reference.Optimizer) {
		t.Fatalf("Adam state differs from standalone trainer:\n actual=%+v\nreference=%+v", actual.Optimizer, reference.Optimizer)
	}
	if actual.Updates != reference.Updates {
		t.Fatalf("updates = %d, standalone = %d", actual.Updates, reference.Updates)
	}
	if !reflect.DeepEqual(actual.Options, reference.Options) {
		t.Fatal("trainer options differ from standalone trainer")
	}
}

func individualChemicalInitialTrainingSnapshot(snapshot learning.IndividualSnapshot) learning.TrainingSnapshot {
	return learning.TrainingSnapshot{
		SchemaVersion: "coimnet-episode-training/v1",
		Config:        snapshot.Config,
		Parameters:    snapshot.Parameters,
		Options:       snapshot.Optimizer.Options,
		Optimizer:     snapshot.Optimizer.State,
		Updates:       snapshot.Optimizer.Updates,
		Accumulator:   snapshot.Optimizer.Accumulator,
	}
}

func assertIndividualChemicalInitialPersistentBoundary(t *testing.T, before, after learning.IndividualSnapshot, beforeReport, afterReport learning.ChemistryReport) {
	t.Helper()
	if !reflect.DeepEqual(before.Config, after.Config) || before.ConfigHash != after.ConfigHash || before.Profile != after.Profile {
		t.Fatal("TrainEpisode changed the persistent model boundary")
	}
	if !reflect.DeepEqual(before.Neural, after.Neural) {
		t.Fatal("TrainEpisode changed persistent neural state")
	}
	if !reflect.DeepEqual(before.Chemical, after.Chemical) {
		t.Fatal("TrainEpisode changed persistent chemical state or controller window")
	}
	if !reflect.DeepEqual(before.Plastic, after.Plastic) {
		t.Fatal("TrainEpisode changed persistent plastic state")
	}
	if !reflect.DeepEqual(beforeReport, afterReport) {
		t.Fatalf("TrainEpisode changed ChemistryReport: before=%+v after=%+v", beforeReport, afterReport)
	}
}
