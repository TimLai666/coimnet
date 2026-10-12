package learning_test

import (
	"context"
	"math"
	"reflect"
	"sync"
	"testing"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
)

const individualChemicalTrainingSnapshotVersion = "coimnet-episode-training/v1"

type individualChemicalTrainingProfile struct {
	name       string
	config     learning.Config
	parameters learning.Parameters
	warmup     [][]float64
	input      [][]float64
	target     []float64
}

func individualChemicalTrainingProfiles() []individualChemicalTrainingProfile {
	continuousConfig := chemContinuousConfig()
	continuousParameters := chemContinuousParameters()
	lifConfig := chemLIFConfig()
	lifParameters := chemLIFParameters()
	mixedConfig := mixedNodeConfig()
	mixedParameters := mixedNodeParameters()
	return []individualChemicalTrainingProfile{
		{
			name:       "continuous",
			config:     continuousConfig,
			parameters: continuousParameters,
			warmup:     [][]float64{{0.7, 0}},
			input:      [][]float64{{0.2, 0.1}, {0.3, 0.2}},
			target:     []float64{0.35},
		},
		{
			name:       "lif",
			config:     lifConfig,
			parameters: lifParameters,
			warmup:     [][]float64{{0.7, 0}},
			input:      [][]float64{{1.2, 0.1}, {1.3, 0.2}},
			target:     []float64{0.35},
		},
		{
			name:       "mixed",
			config:     mixedConfig,
			parameters: mixedParameters,
			warmup:     [][]float64{{0.7}},
			// The readout is two edges downstream, including a one-step
			// delay. Six rows let a chemical change reach that readout.
			input:  [][]float64{{0.7}, {0.8}, {0.7}, {0.8}, {0.7}, {0.8}},
			target: []float64{0.35},
		},
	}
}

// The controller window is three rows, longer than the mixed fixture's maximum
// delay. Its source state, together with a location and transport declaration,
// makes the persistent boundary materially different from a zero chemistry
// boundary.
func individualChemicalTrainingControllerChemistry(nodes int) modulation.ChemistryConfig {
	nodeRegion := make([]int, nodes)
	controller := &modulation.Controller{
		Inputs:  modulation.ControllerInputs{SummaryWindow: 3},
		Nodes:   nodes,
		Hidden:  1,
		Channel: 0,
		Parameters: []float64{
			0.6, 0.2, 0.05, 0.8, -0.1,
		},
	}
	return modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{
			Regions:  2,
			Channels: 1,
			DT:       1,
			Tau:      []float64{2},
			Transport: &modulation.Transport{Fraction: [][]float64{
				{0, 0.25},
				{0.1, 0},
			}},
		},
		Sources: []modulation.SourceSpec{{
			Kind:       modulation.SourceController,
			Channel:    0,
			Controller: controller,
		}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{{
			Cells: []int{0}, Signal: "fixture", Channel: 0,
			Status: modulation.StatusHypothesized, Kd: 0.5, N: 1,
			Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "test/v1",
			Location: &modulation.ReceptorLocation{
				Region: 0, Compartment: "soma", AnatomicalLocation: "fixture-soma",
				Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "test/location-v1",
			},
		}}},
		Effects: []modulation.Effect{{
			Kind: modulation.EffectSensitivity, Receptor: 0, GammaScale: 1.5,
		}},
		Regions: modulation.RegionAssignment{NodeRegion: nodeRegion},
	}
}

func individualChemicalTrainingPulseChemistry(nodes int, step uint64, rate float64) modulation.ChemistryConfig {
	nodeRegion := make([]int, nodes)
	return modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{2}},
		Sources: []modulation.SourceSpec{{
			Kind:    modulation.SourceExternalTimeline,
			Channel: 0,
			Timeline: &modulation.ExternalTimeline{
				ChannelCount: 1,
				Entries:      []modulation.TimelineEntry{{Step: step, Channel: 0, Rate: rate}},
			},
		}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{{
			Cells: []int{0}, Signal: "fixture", Channel: 0,
			Status: modulation.StatusHypothesized, Kd: 0.5, N: 1,
			Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "test/v1",
			Location: &modulation.ReceptorLocation{
				Region: 0, Compartment: "soma", AnatomicalLocation: "fixture-soma",
				Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "test/location-v1",
			},
		}}},
		Effects: []modulation.Effect{{
			Kind: modulation.EffectSensitivity, Receptor: 0, GammaScale: 1.5,
		}},
		Regions: modulation.RegionAssignment{NodeRegion: nodeRegion},
	}
}

func individualChemicalTrainingNodes(c learning.Config) int {
	if c.Mixed != nil {
		return c.Mixed.Nodes
	}
	if c.LIF != nil {
		return c.LIF.Nodes
	}
	return c.Dynamics.Nodes
}

func newIndividualChemicalTraining(t *testing.T, p individualChemicalTrainingProfile) *learning.Individual {
	t.Helper()
	individual, err := learning.NewIndividual(
		p.config,
		p.parameters,
		learning.DefaultOptions(),
		make([]float64, individualChemicalTrainingNodes(p.config)),
	)
	if err != nil {
		t.Fatal(err)
	}
	return individual
}

func prepareIndividualChemicalTrainingBoundary(t *testing.T, p individualChemicalTrainingProfile) (*learning.Individual, learning.IndividualSnapshot, learning.ChemistryReport) {
	t.Helper()
	individual := newIndividualChemicalTraining(t, p)
	if _, err := individual.Advance(context.Background(), p.warmup); err != nil {
		t.Fatalf("warmup Advance() error = %v", err)
	}
	if err := individual.EnableChemistry(individualChemicalTrainingControllerChemistry(individualChemicalTrainingNodes(p.config))); err != nil {
		t.Fatalf("EnableChemistry() error = %v", err)
	}
	if err := individual.SetResource("energy", 1.25); err != nil {
		t.Fatalf("SetResource() error = %v", err)
	}
	if err := individual.OfferFeedback(stepFeedbackAt(t, 0, 0)); err != nil {
		t.Fatalf("OfferFeedback() error = %v", err)
	}
	if err := individual.SetExpressionGain(learning.ExpressionGain{
		Nodes: p.config.ReadoutNodes, Receptor: 0, Scale: 1.2, Min: 0.5, Max: 3,
	}); err != nil {
		t.Fatalf("SetExpressionGain() error = %v", err)
	}
	if _, err := individual.Advance(context.Background(), p.input); err != nil {
		t.Fatalf("chemical Advance() error = %v", err)
	}
	snapshot := individual.Snapshot()
	report := individual.ChemistryReport()
	if snapshot.Chemical == nil {
		t.Fatal("chemical snapshot part is nil after EnableChemistry and Advance")
	}
	if snapshot.Chemical.State.Steps == 0 || len(snapshot.Chemical.SourceStates) != 1 || snapshot.Chemical.SourceStates[0].Controller == nil {
		t.Fatalf("persistent chemical boundary is incomplete: %+v", snapshot.Chemical)
	}
	if snapshot.Chemical.Resources["energy"] != 1.25 || len(snapshot.Chemical.PendingFeedback) != 1 || snapshot.Chemical.Expression == nil {
		t.Fatalf("persistent chemical inputs were not retained: %+v", snapshot.Chemical)
	}
	if report.Steps != len(p.input) || len(report.ReleaseTotal) != 1 || report.ReleaseTotal[0] == 0 {
		t.Fatalf("chemical Advance() did not produce the expected report: %+v", report)
	}
	return individual, snapshot, report
}

func individualChemicalTrainingLatestRaw(s learning.NeuralState) []float64 {
	switch {
	case s.Continuous != nil:
		return append([]float64(nil), s.Continuous.History[len(s.Continuous.History)-1]...)
	case s.LIF != nil:
		return append([]float64(nil), s.LIF.History[len(s.LIF.History)-1]...)
	case s.Mixed != nil:
		raw := make([]float64, len(s.Mixed.Index.ContinuousNodes)+len(s.Mixed.Index.LIFNodes))
		continuous := s.Mixed.Continuous.History[len(s.Mixed.Continuous.History)-1]
		for k, node := range s.Mixed.Index.ContinuousNodes {
			raw[node] = continuous[k]
		}
		lif := s.Mixed.LIF.History[len(s.Mixed.LIF.History)-1]
		for k, node := range s.Mixed.Index.LIFNodes {
			raw[node] = lif[k]
		}
		return raw
	default:
		return nil
	}
}

func individualChemicalTrainingSteps(s learning.NeuralState) uint64 {
	switch {
	case s.Continuous != nil:
		return s.Continuous.Steps
	case s.LIF != nil:
		return s.LIF.Steps
	case s.Mixed != nil:
		return s.Mixed.Steps
	default:
		return 0
	}
}

func individualChemicalTrainingReference(t *testing.T, snapshot learning.IndividualSnapshot) *learning.Trainer {
	t.Helper()
	if snapshot.Chemical == nil {
		t.Fatal("reference requires a persistent chemical snapshot")
	}
	episode := &learning.ChemicalEpisode{
		Part:            *snapshot.Chemical,
		SourceStep:      individualChemicalTrainingSteps(snapshot.Neural),
		InitialActivity: individualChemicalTrainingLatestRaw(snapshot.Neural),
	}
	config := snapshot.Config
	config.Chemistry = episode
	trainer, err := learning.RestoreTrainer(learning.TrainingSnapshot{
		SchemaVersion: individualChemicalTrainingSnapshotVersion,
		Config:        config,
		Parameters:    snapshot.Parameters,
		Options:       snapshot.Optimizer.Options,
		Optimizer:     snapshot.Optimizer.State,
		Updates:       snapshot.Optimizer.Updates,
		Accumulator:   snapshot.Optimizer.Accumulator,
	})
	if err != nil {
		t.Fatalf("RestoreTrainer() reference error = %v", err)
	}
	return trainer
}

func assertIndividualChemicalTrainingBoundary(t *testing.T, before, after learning.IndividualSnapshot, beforeReport, afterReport learning.ChemistryReport) {
	t.Helper()
	if !reflect.DeepEqual(before.Config, after.Config) || before.ConfigHash != after.ConfigHash || before.Profile != after.Profile {
		t.Fatal("chemical training changed immutable individual configuration")
	}
	if !reflect.DeepEqual(before.Neural, after.Neural) {
		t.Fatal("chemical training advanced persistent neural state")
	}
	if !reflect.DeepEqual(before.Plastic, after.Plastic) {
		t.Fatal("chemical training changed persistent plastic state")
	}
	if !reflect.DeepEqual(before.Chemical, after.Chemical) {
		t.Fatal("chemical training changed persistent chemical/controller state")
	}
	if !reflect.DeepEqual(beforeReport, afterReport) {
		t.Fatalf("chemical training changed ChemistryReport: before=%+v after=%+v", beforeReport, afterReport)
	}
}

func assertIndividualChemicalTrainingStateMatchesReference(t *testing.T, actual learning.IndividualSnapshot, reference learning.TrainingSnapshot) {
	t.Helper()
	if !reflect.DeepEqual(actual.Parameters, reference.Parameters) {
		t.Fatal("chemical training parameters differ from the public chemical reference")
	}
	if !reflect.DeepEqual(actual.Optimizer.Options, reference.Options) ||
		!reflect.DeepEqual(actual.Optimizer.State, reference.Optimizer) ||
		actual.Optimizer.Updates != reference.Updates ||
		!reflect.DeepEqual(actual.Optimizer.Accumulator, reference.Accumulator) {
		t.Fatal("chemical training optimizer state differs from the public chemical reference")
	}
}

func assertIndividualChemicalTrainingAffectsLoss(t *testing.T, snapshot learning.IndividualSnapshot, input [][]float64, target []float64) {
	t.Helper()
	chemical, err := learning.RestoreIndividual(snapshot)
	if err != nil {
		t.Fatalf("RestoreIndividual() chemical copy error = %v", err)
	}
	plainSnapshot := snapshot
	plainSnapshot.Chemical = nil
	plain, err := learning.RestoreIndividual(plainSnapshot)
	if err != nil {
		t.Fatalf("RestoreIndividual() plain copy error = %v", err)
	}
	chemicalResult, err := chemical.TrainEpisode(context.Background(), input, target)
	if err != nil {
		t.Fatalf("chemical TrainEpisode() error = %v", err)
	}
	plainResult, err := plain.TrainEpisode(context.Background(), input, target)
	if err != nil {
		t.Fatalf("plain TrainEpisode() error = %v", err)
	}
	if math.Abs(chemicalResult.Loss-plainResult.Loss) <= 1e-12 &&
		individualChemicalTrainingParametersClose(chemical.Snapshot().Parameters, plain.Snapshot().Parameters, 1e-10) {
		t.Fatal("persistent chemistry had no visible effect on the training loss or update")
	}
}

func assertIndividualChemicalTrainingFailurePreserves(t *testing.T, individual *learning.Individual, before learning.IndividualSnapshot, beforeReport learning.ChemistryReport, input [][]float64, target []float64) {
	t.Helper()
	if _, err := individual.TrainEpisode(context.Background(), input, target); err == nil {
		t.Fatal("invalid chemical training input unexpectedly succeeded")
	}
	if !reflect.DeepEqual(before, individual.Snapshot()) || !reflect.DeepEqual(beforeReport, individual.ChemistryReport()) {
		t.Fatal("rejected chemical training changed the individual snapshot or report")
	}
}

func TestIndividualChemicalTrainingMatchesPrivateReference(t *testing.T) {
	for _, profile := range individualChemicalTrainingProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			individual, before, beforeReport := prepareIndividualChemicalTrainingBoundary(t, profile)
			assertIndividualChemicalTrainingAffectsLoss(t, before, profile.input, profile.target)
			reference := individualChemicalTrainingReference(t, before)
			defer reference.Close()

			for episode := 0; episode < 3; episode++ {
				got, err := individual.TrainEpisode(context.Background(), profile.input, profile.target)
				if err != nil {
					t.Fatalf("TrainEpisode(%d) error = %v", episode, err)
				}
				want, err := reference.Step(context.Background(), profile.input, profile.target)
				if err != nil {
					t.Fatalf("reference Step(%d) error = %v", episode, err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("TrainEpisode(%d) result = %+v, reference = %+v", episode, got, want)
				}
				after := individual.Snapshot()
				assertIndividualChemicalTrainingBoundary(t, before, after, beforeReport, individual.ChemistryReport())
				if after.Optimizer.Episodes != before.Optimizer.Episodes+uint64(episode)+1 {
					t.Fatalf("episode counter = %d, want %d", after.Optimizer.Episodes, before.Optimizer.Episodes+uint64(episode)+1)
				}
				assertIndividualChemicalTrainingStateMatchesReference(t, after, reference.Snapshot())
			}
		})
	}
}

func TestIndividualChemicalTrainingAtomicityAndPartialAccumulation(t *testing.T) {
	profile := individualChemicalTrainingProfiles()[0]
	individual, before, beforeReport := prepareIndividualChemicalTrainingBoundary(t, profile)
	assertIndividualChemicalTrainingAffectsLoss(t, before, profile.input, profile.target)

	assertIndividualChemicalTrainingFailurePreserves(t, individual, before, beforeReport, [][]float64{{math.NaN(), 0}}, profile.target)
	assertIndividualChemicalTrainingFailurePreserves(t, individual, before, beforeReport, profile.input, []float64{0.1, 0.2})
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := individual.TrainEpisode(canceled, profile.input, profile.target); err == nil {
		t.Fatal("canceled chemical TrainEpisode() unexpectedly succeeded")
	}
	if !reflect.DeepEqual(before, individual.Snapshot()) || !reflect.DeepEqual(beforeReport, individual.ChemistryReport()) {
		t.Fatal("canceled chemical training changed the individual snapshot or report")
	}

	badSnapshot := before
	badOptions := badSnapshot.Optimizer.Options
	badOptions.LearningRate = 2
	badOptions.WeightDecay = math.MaxFloat64
	badSnapshot.Optimizer.Options = badOptions
	badOptimizer, err := learning.RestoreIndividual(badSnapshot)
	if err != nil {
		t.Fatalf("RestoreIndividual() candidate-invalid fixture error = %v", err)
	}
	badBefore, badReport := badOptimizer.Snapshot(), badOptimizer.ChemistryReport()
	if _, err := badOptimizer.TrainEpisode(context.Background(), profile.input, profile.target); err == nil {
		t.Fatal("candidate-invalid chemical update unexpectedly succeeded")
	}
	if !reflect.DeepEqual(badBefore, badOptimizer.Snapshot()) || !reflect.DeepEqual(badReport, badOptimizer.ChemistryReport()) {
		t.Fatal("candidate-invalid chemical training changed the individual snapshot or report")
	}

	overflowSnapshot := before
	state := *overflowSnapshot.Neural.Continuous
	state.Voltage = append([]float64(nil), state.Voltage...)
	state.History = copyIndividualChemicalTrainingRows(state.History)
	state.Steps = uint64(math.MaxInt64) + 1
	overflowSnapshot.Neural.Continuous = &state
	overflow, err := learning.RestoreIndividual(overflowSnapshot)
	if err != nil {
		t.Fatalf("RestoreIndividual() source-overflow fixture error = %v", err)
	}
	overflowBefore, overflowReport := overflow.Snapshot(), overflow.ChemistryReport()
	if _, err := overflow.TrainEpisode(context.Background(), profile.input, profile.target); err == nil {
		t.Fatal("late source-clock overflow unexpectedly succeeded")
	}
	if !reflect.DeepEqual(overflowBefore, overflow.Snapshot()) || !reflect.DeepEqual(overflowReport, overflow.ChemistryReport()) {
		t.Fatal("late source-clock overflow changed the individual snapshot or report")
	}

	accumulatingSnapshot := before
	accumulatingOptions := accumulatingSnapshot.Optimizer.Options
	accumulatingOptions.AccumulateSteps = 2
	accumulatingSnapshot.Optimizer.Options = accumulatingOptions
	accumulating, err := learning.RestoreIndividual(accumulatingSnapshot)
	if err != nil {
		t.Fatalf("RestoreIndividual() accumulation fixture error = %v", err)
	}
	accumulatingBefore, accumulatingReport := accumulating.Snapshot(), accumulating.ChemistryReport()
	result, err := accumulating.TrainEpisode(context.Background(), profile.input, profile.target)
	if err != nil {
		t.Fatalf("partial-accumulation TrainEpisode() error = %v", err)
	}
	if result.Applied || result.Accumulated != 1 {
		t.Fatalf("partial-accumulation result = %+v, want one unapplied gradient", result)
	}
	accumulatingAfter := accumulating.Snapshot()
	if !reflect.DeepEqual(accumulatingBefore.Parameters, accumulatingAfter.Parameters) ||
		!reflect.DeepEqual(accumulatingBefore.Optimizer.State, accumulatingAfter.Optimizer.State) ||
		accumulatingBefore.Optimizer.Updates != accumulatingAfter.Optimizer.Updates {
		t.Fatal("partial accumulation changed parameters, Adam state or update count")
	}
	if accumulatingAfter.Optimizer.Accumulator == nil || accumulatingAfter.Optimizer.Accumulator.Count != 1 {
		t.Fatalf("partial accumulator = %+v, want count 1", accumulatingAfter.Optimizer.Accumulator)
	}
	if accumulatingAfter.Optimizer.Episodes != accumulatingBefore.Optimizer.Episodes+1 {
		t.Fatalf("partial-accumulation episode counter = %d, want %d", accumulatingAfter.Optimizer.Episodes, accumulatingBefore.Optimizer.Episodes+1)
	}
	assertIndividualChemicalTrainingBoundary(t, accumulatingBefore, accumulatingAfter, accumulatingReport, accumulating.ChemistryReport())
}

func TestIndividualChemicalTrainingConcurrentCallsSerialize(t *testing.T) {
	profile := individualChemicalTrainingProfiles()[0]
	_, before, _ := prepareIndividualChemicalTrainingBoundary(t, profile)
	assertIndividualChemicalTrainingAffectsLoss(t, before, profile.input, profile.target)
	serial, err := learning.RestoreIndividual(before)
	if err != nil {
		t.Fatal(err)
	}
	concurrent, err := learning.RestoreIndividual(before)
	if err != nil {
		t.Fatal(err)
	}
	// Reports describe the latest operation and are not snapshot state.
	// Compare with the restored individual's report before training.
	beforeReport := concurrent.ChemistryReport()
	for range 2 {
		if _, err := serial.TrainEpisode(context.Background(), profile.input, profile.target); err != nil {
			t.Fatalf("serial TrainEpisode() error = %v", err)
		}
	}
	errors := make(chan error, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := concurrent.TrainEpisode(context.Background(), profile.input, profile.target)
			errors <- err
		}()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent TrainEpisode() error = %v", err)
		}
	}
	if !reflect.DeepEqual(serial.Snapshot(), concurrent.Snapshot()) {
		t.Fatal("concurrent chemical episodes do not match equivalent serial training")
	}
	assertIndividualChemicalTrainingBoundary(t, before, concurrent.Snapshot(), beforeReport, concurrent.ChemistryReport())
}

func TestIndividualChemicalTrainingFreezeUsesHeldConcentration(t *testing.T) {
	profile := individualChemicalTrainingProfiles()[0]
	declaration := individualChemicalTrainingPulseChemistry(2, 0, 64)
	plain := newIndividualChemicalTraining(t, profile)
	unfrozen := newIndividualChemicalTraining(t, profile)
	frozen := newIndividualChemicalTraining(t, profile)
	for name, individual := range map[string]*learning.Individual{"unfrozen": unfrozen, "frozen": frozen} {
		if err := individual.EnableChemistry(declaration); err != nil {
			t.Fatalf("%s EnableChemistry() error = %v", name, err)
		}
	}
	if err := frozen.FreezeChemistry(true); err != nil {
		t.Fatalf("FreezeChemistry(true) error = %v", err)
	}
	frozenBefore, frozenReport := frozen.Snapshot(), frozen.ChemistryReport()
	input, target := [][]float64{{1, 0}}, []float64{0.5}
	frozenResult, err := frozen.TrainEpisode(context.Background(), input, target)
	if err != nil {
		t.Fatalf("frozen TrainEpisode() error = %v", err)
	}
	plainResult, err := plain.TrainEpisode(context.Background(), input, target)
	if err != nil {
		t.Fatalf("plain TrainEpisode() error = %v", err)
	}
	unfrozenResult, err := unfrozen.TrainEpisode(context.Background(), input, target)
	if err != nil {
		t.Fatalf("unfrozen TrainEpisode() error = %v", err)
	}
	if math.Abs(frozenResult.Loss-plainResult.Loss) > 1e-9 ||
		!individualChemicalTrainingParametersClose(frozen.Snapshot().Parameters, plain.Snapshot().Parameters, 1e-8) {
		t.Fatal("held zero concentration did not match the neutral no-chemistry episode")
	}
	if math.Abs(frozenResult.Loss-unfrozenResult.Loss) <= 1e-9 &&
		individualChemicalTrainingParametersClose(frozen.Snapshot().Parameters, unfrozen.Snapshot().Parameters, 1e-8) {
		t.Fatal("FreezeChemistry did not suppress the pulse in the training episode")
	}
	assertIndividualChemicalTrainingBoundary(t, frozenBefore, frozen.Snapshot(), frozenReport, frozen.ChemistryReport())
}

func TestIndividualChemicalTrainingDisablePreservesLegacyBits(t *testing.T) {
	profile := individualChemicalTrainingProfiles()[0]
	declaration := individualChemicalTrainingPulseChemistry(2, 0, 64)
	enabled := newIndividualChemicalTraining(t, profile)
	plain := newIndividualChemicalTraining(t, profile)
	if err := enabled.EnableChemistry(declaration); err != nil {
		t.Fatalf("EnableChemistry() error = %v", err)
	}
	input, target := [][]float64{{1, 0}}, []float64{0.5}
	enabledResult, err := enabled.TrainEpisode(context.Background(), input, target)
	if err != nil {
		t.Fatalf("enabled TrainEpisode() error = %v", err)
	}
	plainResult, err := plain.TrainEpisode(context.Background(), input, target)
	if err != nil {
		t.Fatalf("plain TrainEpisode() error = %v", err)
	}
	if math.Abs(enabledResult.Loss-plainResult.Loss) <= 1e-12 &&
		individualChemicalTrainingParametersClose(enabled.Snapshot().Parameters, plain.Snapshot().Parameters, 1e-10) {
		t.Fatal("enabled chemistry did not affect the training path")
	}

	disabled := newIndividualChemicalTraining(t, profile)
	legacy := newIndividualChemicalTraining(t, profile)
	if err := disabled.EnableChemistry(declaration); err != nil {
		t.Fatalf("second EnableChemistry() error = %v", err)
	}
	if err := disabled.DisableChemistry(); err != nil {
		t.Fatalf("DisableChemistry() error = %v", err)
	}
	disabledResult, err := disabled.TrainEpisode(context.Background(), input, target)
	if err != nil {
		t.Fatalf("disabled TrainEpisode() error = %v", err)
	}
	legacyResult, err := legacy.TrainEpisode(context.Background(), input, target)
	if err != nil {
		t.Fatalf("legacy TrainEpisode() error = %v", err)
	}
	if !reflect.DeepEqual(disabledResult, legacyResult) || !reflect.DeepEqual(disabled.Snapshot(), legacy.Snapshot()) {
		t.Fatal("DisableChemistry did not restore the legacy training path")
	}
	if disabled.Snapshot().Chemical != nil {
		t.Fatal("DisableChemistry left a chemical snapshot part behind")
	}
}

func copyIndividualChemicalTrainingRows(rows [][]float64) [][]float64 {
	owned := make([][]float64, len(rows))
	for i, row := range rows {
		owned[i] = append([]float64(nil), row...)
	}
	return owned
}

func individualChemicalTrainingParametersClose(a, b learning.Parameters, tolerance float64) bool {
	return individualChemicalTrainingSlicesClose(a.Core.Weights, b.Core.Weights, tolerance) &&
		individualChemicalTrainingSlicesClose(a.Core.Bias, b.Core.Bias, tolerance) &&
		individualChemicalTrainingSlicesClose(a.Core.LogTau, b.Core.LogTau, tolerance) &&
		individualChemicalTrainingSlicesClose(a.ThetaRaw, b.ThetaRaw, tolerance) &&
		individualChemicalTrainingSlicesClose(a.Encoder, b.Encoder, tolerance) &&
		individualChemicalTrainingSlicesClose(a.Readout, b.Readout, tolerance)
}

func individualChemicalTrainingSlicesClose(a, b []float64, tolerance float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Abs(a[i]-b[i]) > tolerance {
			return false
		}
	}
	return true
}
