package learning_test

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/signal"
)

// chemicalStatefulFixture deliberately creates the saved neural boundary
// before enabling chemistry. Its neural clock is therefore five while the
// kinetics clock is three. The delayed edge is one step and the controller
// remembers four rows, so the source state reaches past the saved neural
// boundary without being confused with a fresh zero trajectory.
type chemicalStatefulFixture struct {
	config     learning.Config
	parameters learning.Parameters
	options    learning.Options
	snapshot   learning.IndividualSnapshot
	input      [][]float64
	upstream   [][]float64
}

func chemicalStatefulChemistry() modulation.ChemistryConfig {
	controller := &modulation.Controller{
		Inputs: modulation.ControllerInputs{
			SummaryWindow: 4,
			UseFeedback:   true,
			Resources:     []string{"energy"},
		},
		Nodes:      2,
		Hidden:     1,
		Channel:    0,
		Parameters: []float64{0.8, -0.3, 0.45, 0.35, 0.1, 0.6, 0.2},
	}
	return modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{
			Regions:  2,
			Channels: 1,
			DT:       1,
			Tau:      []float64{2},
			Transport: &modulation.Transport{Fraction: [][]float64{
				{0, 0.15},
				{0.2, 0},
			}},
		},
		Sources: []modulation.SourceSpec{{
			Kind:       modulation.SourceController,
			Channel:    0,
			Controller: controller,
		}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{{
			Cells: []int{1}, Signal: "fixture-signal", Channel: 0,
			Status: modulation.StatusHypothesized, Kd: 0.2, N: 1.3,
			Evidence: "chemical-stateful-test", MeasurementKind: "declared", MappingVersion: "test/v1",
			Location: &modulation.ReceptorLocation{
				Region: 1, Compartment: "postsynaptic", AnatomicalLocation: "fixture-region-1",
				Evidence: "chemical-stateful-test", MeasurementKind: "declared", MappingVersion: "test/location-v1",
			},
		}}},
		Effects: []modulation.Effect{{
			Kind: modulation.EffectSensitivity, Receptor: 0, GammaScale: 0.8, BetaScale: 0.12,
		}},
		Regions: modulation.RegionAssignment{NodeRegion: []int{0, 1}},
	}
}

func chemicalStatefulFeedbackSpec(produced, available int64, score float64) signal.FeedbackSpec {
	return signal.FeedbackSpec{
		SchemaVersion: signal.CurrentSchemaVersion(), ExperienceID: "chemical-stateful", ActionID: "action",
		ProducedAt:  signal.Timestamp{Value: produced, Unit: signal.TimeUnitModelStep},
		AvailableAt: signal.Timestamp{Value: available, Unit: signal.TimeUnitModelStep},
		Source:      "chemical-stateful-test", Score: score, ModelVersion: signal.CurrentSchemaVersion(),
	}
}

func newChemicalStatefulFixture(t *testing.T) chemicalStatefulFixture {
	t.Helper()
	config := learning.Config{
		Dynamics: dynamics.Config{
			Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{1}, DT: 0.7, Activation: "tanh",
		},
		InputSize: 1, OutputSize: 1, ReadoutNodes: []int{1}, InputNodes: []int{0}, ReadoutEveryStep: true,
	}
	parameters := learning.Parameters{
		Core: dynamics.Parameters{
			Weights: []float64{0.3}, Bias: []float64{0.02, -0.08}, LogTau: []float64{0.1, -0.2},
		},
		Encoder: []float64{0.6}, Readout: []float64{0.9},
	}
	options := learning.DefaultOptions()
	individual, err := learning.NewIndividual(config, parameters, options, []float64{0.11, -0.14})
	if err != nil {
		t.Fatalf("NewIndividual: %v", err)
	}
	prefix := [][]float64{{0.4}, {-0.2}}
	if _, err := individual.Advance(context.Background(), prefix); err != nil {
		t.Fatalf("neural prefix Advance: %v", err)
	}
	if err := individual.EnableChemistry(chemicalStatefulChemistry()); err != nil {
		t.Fatalf("EnableChemistry: %v", err)
	}
	if err := individual.SetResource("energy", 1.4); err != nil {
		t.Fatalf("SetResource: %v", err)
	}
	for _, spec := range []signal.FeedbackSpec{
		chemicalStatefulFeedbackSpec(0, 1, 0.5),
		chemicalStatefulFeedbackSpec(2, 4, -0.25),
	} {
		feedback, err := signal.NewFeedback(spec)
		if err != nil {
			t.Fatalf("NewFeedback: %v", err)
		}
		if err := individual.OfferFeedback(feedback); err != nil {
			t.Fatalf("OfferFeedback: %v", err)
		}
	}
	if err := individual.SetExpressionGain(learning.ExpressionGain{
		Nodes: []int{1}, Receptor: 0, Scale: 0.55, Min: 0.5, Max: 2,
	}); err != nil {
		t.Fatalf("SetExpressionGain: %v", err)
	}
	warmup := [][]float64{{0.8}, {-0.4}, {0.6}}
	if _, err := individual.Advance(context.Background(), warmup); err != nil {
		t.Fatalf("chemical warmup Advance: %v", err)
	}
	snapshot := individual.Snapshot()
	if snapshot.Chemical == nil || snapshot.Neural.Continuous == nil {
		t.Fatalf("fixture did not produce continuous chemical snapshot: %+v", snapshot)
	}
	if snapshot.Config.Chemistry != nil {
		t.Fatal("persistent individual unexpectedly retained Config.Chemistry")
	}
	if snapshot.Neural.Continuous.Steps != uint64(len(prefix)+len(warmup)) {
		t.Fatalf("neural steps = %d, want %d", snapshot.Neural.Continuous.Steps, len(prefix)+len(warmup))
	}
	if snapshot.Chemical.State.Steps != uint64(len(warmup)) {
		t.Fatalf("kinetics steps = %d, want %d", snapshot.Chemical.State.Steps, len(warmup))
	}
	if snapshot.Neural.Continuous.Steps == snapshot.Chemical.State.Steps {
		t.Fatal("fixture clocks accidentally agree")
	}
	if len(snapshot.Chemical.Resources) == 0 || snapshot.Chemical.Resources["energy"] != 1.4 {
		t.Fatalf("resources were not retained: %+v", snapshot.Chemical.Resources)
	}
	if len(snapshot.Chemical.PendingFeedback) != 2 || snapshot.Chemical.PendingFeedback[0].AvailableAt.Value == snapshot.Chemical.PendingFeedback[1].AvailableAt.Value {
		t.Fatalf("feedback availability boundary is not representative: %+v", snapshot.Chemical.PendingFeedback)
	}
	if snapshot.Chemical.Expression == nil || snapshot.Chemical.Expression.Nodes[0] != 1 {
		t.Fatalf("expression boundary was not retained: %+v", snapshot.Chemical.Expression)
	}
	if location := snapshot.Chemical.Config.Receptors.Records[0].Location; location == nil || location.Region != 1 || location.Compartment != "postsynaptic" {
		t.Fatalf("receptor location boundary was not retained: %+v", location)
	}
	if len(snapshot.Chemical.SourceStates) != 1 || snapshot.Chemical.SourceStates[0].Controller == nil {
		t.Fatalf("controller source state was not retained: %+v", snapshot.Chemical.SourceStates)
	}
	controllerState := snapshot.Chemical.SourceStates[0].Controller
	if len(controllerState.Window) != 4 || controllerState.Filled != 3 {
		t.Fatalf("controller state = %+v, want four-row window filled by three rows", controllerState)
	}
	positiveConcentration := false
	for _, row := range snapshot.Chemical.State.Concentration {
		for _, value := range row {
			positiveConcentration = positiveConcentration || value > 0
		}
	}
	if !positiveConcentration {
		t.Fatal("chemical warmup produced no concentration, so modulation could be a false zero control")
	}
	report := individual.ChemistryReport()
	if report.Steps != len(warmup) || len(report.ReleaseTotal) != 1 || report.ReleaseTotal[0] == 0 || report.ExpressionGainApplied == 0 {
		t.Fatalf("chemical warmup report is not active: %+v", report)
	}

	return chemicalStatefulFixture{
		config: config, parameters: parameters, options: options, snapshot: snapshot,
		input:    [][]float64{{0.2}, {0.5}, {-0.3}, {0.7}},
		upstream: [][]float64{{0.3}, {-0.4}, {0.25}, {0.5}},
	}
}

func chemicalStatefulEpisode(snapshot learning.IndividualSnapshot) *learning.ChemicalEpisode {
	part := *snapshot.Chemical
	part.Config = part.Config.Clone()
	part.State.Concentration = chemicalStatefulCopyRows(part.State.Concentration)
	part.Resources = make(map[string]float64, len(snapshot.Chemical.Resources))
	for name, amount := range snapshot.Chemical.Resources {
		part.Resources[name] = amount
	}
	part.PendingFeedback = append([]signal.FeedbackSpec(nil), snapshot.Chemical.PendingFeedback...)
	part.SourceStates = make([]learning.ChemicalSourceState, len(snapshot.Chemical.SourceStates))
	for i, source := range snapshot.Chemical.SourceStates {
		part.SourceStates[i] = source
		if source.Controller != nil {
			state := *source.Controller
			state.Window = chemicalStatefulCopyRows(source.Controller.Window)
			part.SourceStates[i].Controller = &state
		}
	}
	if snapshot.Chemical.Expression != nil {
		expression := *snapshot.Chemical.Expression
		expression.Nodes = append([]int(nil), snapshot.Chemical.Expression.Nodes...)
		part.Expression = &expression
	}
	neural := snapshot.Neural.Continuous
	return &learning.ChemicalEpisode{
		Part: part, SourceStep: neural.Steps,
		InitialActivity: append([]float64(nil), neural.History[len(neural.History)-1]...),
	}
}

func chemicalStatefulConfig(snapshot learning.IndividualSnapshot) learning.Config {
	config := snapshot.Config
	config.Chemistry = chemicalStatefulEpisode(snapshot)
	return config
}

func chemicalStatefulCopyRows(rows [][]float64) [][]float64 {
	owned := make([][]float64, len(rows))
	for i, row := range rows {
		owned[i] = append([]float64(nil), row...)
	}
	return owned
}

func chemicalStatefulAdvance(t *testing.T, snapshot learning.IndividualSnapshot, parameters learning.Parameters, input [][]float64) [][]float64 {
	t.Helper()
	candidate := snapshot
	candidate.Parameters = cloneLearningParametersStateful(parameters)
	individual, err := learning.RestoreIndividual(candidate)
	if err != nil {
		t.Fatalf("RestoreIndividual: %v", err)
	}
	outputs, err := individual.Advance(context.Background(), chemicalStatefulCopyRows(input))
	if err != nil {
		t.Fatalf("restored Advance: %v", err)
	}
	return outputs
}

func chemicalStatefulObjective(t *testing.T, snapshot learning.IndividualSnapshot, parameters learning.Parameters, input, upstream [][]float64) float64 {
	t.Helper()
	return weightedLearningOutputs(chemicalStatefulAdvance(t, snapshot, parameters, input), upstream)
}

func chemicalStatefulGradientHasFiniteNonzero(g learning.Gradient) bool {
	seen := false
	for _, group := range [][]float64{g.Core.Weights, g.Core.Bias, g.Core.LogTau, g.Encoder, g.Readout} {
		for _, value := range group {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return false
			}
			seen = seen || value != 0
		}
	}
	return seen
}

func TestChemicalStatefulSavedBoundaryMatchesRestoredIndividual(t *testing.T) {
	fixture := newChemicalStatefulFixture(t)
	config := chemicalStatefulConfig(fixture.snapshot)
	network, err := learning.NewNetwork(config)
	if err != nil {
		t.Fatalf("NewNetwork with saved chemistry boundary: %v", err)
	}
	// This is the legal stateful chemical forward/reverse boundary. The
	// currently stubbed coupled state entry point must make this call RED.
	gradient, err := network.LossGradientFromState(context.Background(), fixture.snapshot.Parameters, fixture.snapshot.Neural, fixture.input, fixture.upstream, 0)
	if err != nil {
		t.Fatalf("legal chemical LossGradientFromState: %v", err)
	}
	if !chemicalStatefulGradientHasFiniteNonzero(gradient) {
		t.Fatalf("legal chemical stateful gradient is empty, non-finite, or all zero: %+v", gradient)
	}
	// NewNetwork must own the episode boundary. Mutating the caller's source
	// map and explicit first activity after construction cannot alter the
	// already validated forward/reverse reference.
	ownedConfig := chemicalStatefulConfig(fixture.snapshot)
	ownedNetwork, err := learning.NewNetwork(ownedConfig)
	if err != nil {
		t.Fatalf("NewNetwork ownership copy: %v", err)
	}
	ownedConfig.Chemistry.Part.Resources["energy"] = 99
	ownedConfig.Chemistry.InitialActivity[0] = -99
	ownedGradient, err := ownedNetwork.LossGradientFromState(context.Background(), fixture.snapshot.Parameters, fixture.snapshot.Neural, fixture.input, fixture.upstream, 0)
	if err != nil {
		t.Fatalf("owned chemistry boundary after caller mutation: %v", err)
	}
	if !reflect.DeepEqual(gradient, ownedGradient) {
		t.Fatal("network chemistry boundary changed after caller source mutation")
	}

	before := fixture.snapshot
	on := chemicalStatefulAdvance(t, fixture.snapshot, fixture.parameters, fixture.input)
	offSnapshot := fixture.snapshot
	offSnapshot.Chemical = nil
	off := chemicalStatefulAdvance(t, offSnapshot, fixture.parameters, fixture.input)
	if len(on) != len(off) {
		t.Fatalf("chemical on rows = %d, off rows = %d", len(on), len(off))
	}
	var difference float64
	for row := range on {
		for column := range on[row] {
			difference = math.Max(difference, math.Abs(on[row][column]-off[row][column]))
		}
	}
	if difference <= 1e-8 {
		t.Fatalf("chemical on/off saved-start outputs are indistinguishable: on=%v off=%v", on, off)
	}
	second := chemicalStatefulAdvance(t, fixture.snapshot, fixture.parameters, fixture.input)
	if !reflect.DeepEqual(on, second) {
		t.Fatalf("restoring the same saved chemistry/neural boundary was not deterministic:\nfirst=%v\nsecond=%v", on, second)
	}
	if !reflect.DeepEqual(before, fixture.snapshot) {
		t.Fatal("building and advancing from the saved boundary mutated the fixture snapshot")
	}
}

func TestChemicalStatefulLossGradientMatchesSavedStartFiniteDifferences(t *testing.T) {
	fixture := newChemicalStatefulFixture(t)
	savedBefore := fixture.snapshot
	config := chemicalStatefulConfig(fixture.snapshot)
	network, err := learning.NewNetwork(config)
	if err != nil {
		t.Fatalf("NewNetwork: %v", err)
	}
	gradient, err := network.LossGradientFromState(context.Background(), fixture.parameters, fixture.snapshot.Neural, fixture.input, fixture.upstream, 0)
	if err != nil {
		t.Fatalf("legal chemical LossGradientFromState: %v", err)
	}
	objective := func(parameters learning.Parameters) float64 {
		return chemicalStatefulObjective(t, fixture.snapshot, parameters, fixture.input, fixture.upstream)
	}
	const epsilon = 2e-3
	check := func(label string, values, derivatives []float64, perturb func(int, float64) float64) {
		if len(values) != len(derivatives) {
			t.Fatalf("%s gradient length = %d, want %d", label, len(derivatives), len(values))
		}
		for index, value := range values {
			finiteDifference := (perturb(index, value+epsilon) - perturb(index, value-epsilon)) / (2 * epsilon)
			if math.Abs(derivatives[index]-finiteDifference) > 2e-3+2e-3*math.Abs(finiteDifference) {
				t.Fatalf("%s[%d] = %.17g, saved-start finite difference %.17g", label, index, derivatives[index], finiteDifference)
			}
		}
	}
	check("core.weights", fixture.parameters.Core.Weights, gradient.Core.Weights, func(index int, value float64) float64 {
		parameters := cloneLearningParametersStateful(fixture.parameters)
		parameters.Core.Weights[index] = value
		return objective(parameters)
	})
	check("core.bias", fixture.parameters.Core.Bias, gradient.Core.Bias, func(index int, value float64) float64 {
		parameters := cloneLearningParametersStateful(fixture.parameters)
		parameters.Core.Bias[index] = value
		return objective(parameters)
	})
	check("core.log_tau", fixture.parameters.Core.LogTau, gradient.Core.LogTau, func(index int, value float64) float64 {
		parameters := cloneLearningParametersStateful(fixture.parameters)
		parameters.Core.LogTau[index] = value
		return objective(parameters)
	})
	check("encoder", fixture.parameters.Encoder, gradient.Encoder, func(index int, value float64) float64 {
		parameters := cloneLearningParametersStateful(fixture.parameters)
		parameters.Encoder[index] = value
		return objective(parameters)
	})
	check("readout", fixture.parameters.Readout, gradient.Readout, func(index int, value float64) float64 {
		parameters := cloneLearningParametersStateful(fixture.parameters)
		parameters.Readout[index] = value
		return objective(parameters)
	})
	if !chemicalStatefulGradientHasFiniteNonzero(gradient) {
		t.Fatal("chemical controller boundary did not yield a finite nonzero parameter gradient")
	}
	if !reflect.DeepEqual(savedBefore, fixture.snapshot) {
		t.Fatal("finite-difference restores mutated the saved chemistry/neural boundary")
	}
}

func chemicalStatefulApplyMaskedFirstAdam(parameters *learning.Parameters, gradient learning.Gradient, options learning.Options) {
	parameterGroups := [][]float64{parameters.Core.Weights, parameters.Core.Bias, parameters.Core.LogTau, parameters.ThetaRaw, parameters.Encoder, parameters.Readout}
	gradientGroups := [][]float64{gradient.Core.Weights, gradient.Core.Bias, gradient.Core.LogTau, gradient.ThetaRaw, gradient.Encoder, gradient.Readout}
	trainable := []bool{options.Trainable.Weights, options.Trainable.Bias, options.Trainable.Tau, options.Trainable.Theta, options.Trainable.Encoder, options.Trainable.Readout}
	for group := range parameterGroups {
		if !trainable[group] {
			continue
		}
		for index, value := range gradientGroups[group] {
			parameterGroups[group][index] = parameterGroups[group][index]*(1-options.LearningRate*options.WeightDecay) - options.LearningRate*value/(math.Abs(value)+options.Epsilon)
		}
	}
}

func TestChemicalStatefulStepFromStateMatchesAdamWithMaskAndAccumulation(t *testing.T) {
	fixture := newChemicalStatefulFixture(t)
	savedBefore := fixture.snapshot
	config := chemicalStatefulConfig(fixture.snapshot)
	network, err := learning.NewNetwork(config)
	if err != nil {
		t.Fatalf("NewNetwork: %v", err)
	}
	gradient, err := network.LossGradientFromState(context.Background(), fixture.parameters, fixture.snapshot.Neural, fixture.input, fixture.upstream, 0)
	if err != nil {
		t.Fatalf("legal chemical LossGradientFromState: %v", err)
	}
	options := fixture.options
	options.ClipNorm = 0
	options.AccumulateSteps = 2
	options.Trainable.Readout = false
	trainer, err := learning.NewTrainer(config, fixture.parameters, options)
	if err != nil {
		t.Fatalf("NewTrainer: %v", err)
	}
	input := chemicalStatefulCopyRows(fixture.input)
	upstream := chemicalStatefulCopyRows(fixture.upstream)
	inputBefore := chemicalStatefulCopyRows(input)
	upstreamBefore := chemicalStatefulCopyRows(upstream)
	first, err := trainer.StepFromState(context.Background(), fixture.snapshot.Neural, input, upstream)
	if err != nil {
		t.Fatalf("first partial chemical StepFromState: %v", err)
	}
	if first.Applied || first.Accumulated != 1 || first.LossKnown || first.Updates != 0 {
		t.Fatalf("first partial step = %+v, want an open accumulation window", first)
	}
	middle := trainer.Snapshot()
	if middle.Accumulator == nil || middle.Accumulator.Count != 1 {
		t.Fatalf("partial chemical accumulator = %+v, want one representative gradient", middle.Accumulator)
	}
	if !reflect.DeepEqual(middle.Parameters, fixture.parameters) || middle.Updates != 0 {
		t.Fatal("partial chemical accumulation changed parameters or update count")
	}
	second, err := trainer.StepFromState(context.Background(), fixture.snapshot.Neural, input, upstream)
	if err != nil {
		t.Fatalf("second chemical StepFromState: %v", err)
	}
	if !second.Applied || second.Accumulated != 0 || second.LossKnown || second.Updates != 1 {
		t.Fatalf("second chemical step = %+v, want one applied external-upstream update", second)
	}
	wantParameters := cloneLearningParametersStateful(fixture.parameters)
	chemicalStatefulApplyMaskedFirstAdam(&wantParameters, gradient, options)
	got := trainer.Snapshot()
	if !learningParametersCloseStateful(got.Parameters, wantParameters, 2e-12) {
		t.Fatalf("chemical StepFromState parameters differ from masked first-Adam reference:\n got %#v\nwant %#v", got.Parameters, wantParameters)
	}
	if got.Parameters.Readout[0] != fixture.parameters.Readout[0] {
		t.Fatal("frozen readout changed during masked chemical update")
	}
	wantOptimizer := firstAdamStateful(gradient, options)
	last := len(wantOptimizer.Steps) - 1
	wantOptimizer.First[last], wantOptimizer.Second[last], wantOptimizer.Steps[last] = 0, 0, 0
	if !adamStateEqualStateful(got.Optimizer, wantOptimizer) {
		t.Fatalf("chemical StepFromState optimizer differs from masked Adam reference:\n got %#v\nwant %#v", got.Optimizer, wantOptimizer)
	}
	if !reflect.DeepEqual(input, inputBefore) || !reflect.DeepEqual(upstream, upstreamBefore) {
		t.Fatal("chemical StepFromState mutated caller input or upstream")
	}
	if !reflect.DeepEqual(savedBefore, fixture.snapshot) {
		t.Fatal("chemical StepFromState mutated the saved chemistry/neural boundary")
	}

	recomputeOptions := fixture.options
	recomputeOptions.Recompute = &learning.Recompute{SegmentSteps: 1}
	recomputeTrainer, err := learning.NewTrainer(config, fixture.parameters, recomputeOptions)
	if err != nil {
		t.Fatalf("NewTrainer with ordinary recompute option: %v", err)
	}
	recomputeBefore := recomputeTrainer.Snapshot()
	if result, err := recomputeTrainer.StepFromState(context.Background(), fixture.snapshot.Neural, fixture.input, fixture.upstream); err == nil || !reflect.DeepEqual(result, learning.StepResult{}) {
		t.Fatalf("stateful chemical StepFromState accepted Recompute: result=%+v err=%v", result, err)
	}
	if !reflect.DeepEqual(recomputeBefore, recomputeTrainer.Snapshot()) {
		t.Fatal("Recompute refusal changed the chemical stateful trainer")
	}
}

func chemicalStatefulExpectGradientError(t *testing.T, name string, network *learning.Network, ctx context.Context, parameters learning.Parameters, state learning.NeuralState, input, upstream [][]float64) {
	t.Helper()
	stateBefore := cloneNeuralStateful(state)
	parametersBefore := cloneLearningParametersStateful(parameters)
	inputBefore := chemicalStatefulCopyRows(input)
	upstreamBefore := chemicalStatefulCopyRows(upstream)
	got, err := network.LossGradientFromState(ctx, parameters, state, input, upstream, 0)
	if err == nil || !reflect.DeepEqual(got, learning.Gradient{}) {
		t.Fatalf("%s: result=%+v err=%v, want zero gradient and error", name, got, err)
	}
	if !neuralStateBitsEqualStateful(state, stateBefore) || !learningParametersBitsEqualStateful(parameters, parametersBefore) || !rowsBitsEqualStateful(input, inputBefore) || !rowsBitsEqualStateful(upstream, upstreamBefore) {
		t.Fatalf("%s: failed chemical stateful call mutated a caller boundary", name)
	}
}

func TestChemicalStatefulRejectsInvalidBoundariesAtomically(t *testing.T) {
	fixture := newChemicalStatefulFixture(t)
	config := chemicalStatefulConfig(fixture.snapshot)
	network, err := learning.NewNetwork(config)
	if err != nil {
		t.Fatalf("NewNetwork: %v", err)
	}
	// Establish a legal chemical stateful forward before each negative family.
	if _, err := network.LossGradientFromState(context.Background(), fixture.parameters, fixture.snapshot.Neural, fixture.input, fixture.upstream, 0); err != nil {
		t.Fatalf("legal chemical LossGradientFromState: %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	chemicalStatefulExpectGradientError(t, "canceled context", network, canceled, fixture.parameters, fixture.snapshot.Neural, fixture.input, fixture.upstream)

	badHash := cloneNeuralStateful(fixture.snapshot.Neural)
	badHash.Continuous.ConfigHash = "bad-chemical-stateful-hash"
	chemicalStatefulExpectGradientError(t, "invalid neural config hash", network, context.Background(), fixture.parameters, badHash, fixture.input, fixture.upstream)

	badHistory := cloneNeuralStateful(fixture.snapshot.Neural)
	badHistory.Continuous.History = nil
	chemicalStatefulExpectGradientError(t, "missing saved history", network, context.Background(), fixture.parameters, badHistory, fixture.input, fixture.upstream)

	badCapacity := cloneNeuralStateful(fixture.snapshot.Neural)
	badCapacity.Continuous.Steps = math.MaxUint64
	chemicalStatefulExpectGradientError(t, "neural clock capacity overflow", network, context.Background(), fixture.parameters, badCapacity, [][]float64{{0.2}}, [][]float64{{0.3}})

	badInput := chemicalStatefulCopyRows(fixture.input)
	badInput[1][0] = math.NaN()
	chemicalStatefulExpectGradientError(t, "NaN input", network, context.Background(), fixture.parameters, fixture.snapshot.Neural, badInput, fixture.upstream)

	badUpstream := chemicalStatefulCopyRows(fixture.upstream)
	badUpstream[2][0] = math.Inf(1)
	chemicalStatefulExpectGradientError(t, "non-finite upstream", network, context.Background(), fixture.parameters, fixture.snapshot.Neural, fixture.input, badUpstream)

	clockConfig := chemicalStatefulConfig(fixture.snapshot)
	clockConfig.Chemistry.SourceStep = uint64(math.MaxInt64)
	clockNetwork, err := learning.NewNetwork(clockConfig)
	if err != nil {
		t.Fatalf("NewNetwork at maximum valid source step: %v", err)
	}
	chemicalStatefulExpectGradientError(t, "chemical source clock overflow", clockNetwork, context.Background(), fixture.parameters, fixture.snapshot.Neural, [][]float64{{0.2}, {0.4}}, [][]float64{{0.3}, {0.2}})

	kineticsConfig := chemicalStatefulConfig(fixture.snapshot)
	kineticsConfig.Chemistry.Part.State.Steps = math.MaxUint64
	kineticsNetwork, err := learning.NewNetwork(kineticsConfig)
	if err != nil {
		t.Fatalf("NewNetwork at maximum valid kinetics step: %v", err)
	}
	chemicalStatefulExpectGradientError(t, "chemical kinetics clock overflow", kineticsNetwork, context.Background(), fixture.parameters, fixture.snapshot.Neural, [][]float64{{0.2}}, [][]float64{{0.3}})

	badUpstreamForCandidate := make([][]float64, len(fixture.input))
	for row := range badUpstreamForCandidate {
		badUpstreamForCandidate[row] = []float64{-1}
	}
	baseGradient, err := network.LossGradientFromState(context.Background(), fixture.parameters, fixture.snapshot.Neural, fixture.input, badUpstreamForCandidate, 0)
	if err != nil || len(baseGradient.Readout) != 1 || baseGradient.Readout[0] == 0 {
		t.Fatalf("candidate failure fixture has no usable readout gradient: gradient=%+v err=%v", baseGradient, err)
	}
	badParameters := cloneLearningParametersStateful(fixture.parameters)
	// Put the readout at the opposite side of the float32 range from its finite
	// gradient. A maximal finite learning rate then produces an infinite
	// candidate while the expression gain can still keep the pre-update tensor
	// inside the representable range.
	badParameters.Readout[0] = -math.Copysign(float64(math.MaxFloat32)/4, baseGradient.Readout[0])
	badOptions := fixture.options
	badOptions.LearningRate = float64(math.MaxFloat32)
	badOptions.ClipNorm = 0
	badTrainer, err := learning.NewTrainer(config, badParameters, badOptions)
	if err != nil {
		t.Fatalf("NewTrainer for candidate atomicity fixture: %v", err)
	}
	badTrainerBefore := badTrainer.Snapshot()
	result, err := badTrainer.StepFromState(context.Background(), fixture.snapshot.Neural, fixture.input, badUpstreamForCandidate)
	if err == nil || !reflect.DeepEqual(result, learning.StepResult{}) {
		t.Fatalf("non-finite candidate was accepted or published: result=%+v err=%v", result, err)
	}
	if !reflect.DeepEqual(badTrainerBefore, badTrainer.Snapshot()) {
		t.Fatal("non-finite candidate failure changed parameters, optimizer, or accumulator")
	}
}
