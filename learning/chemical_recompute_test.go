package learning_test

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/signal"
)

// chemicalRecomputeFixture deliberately keeps every mutable boundary visible:
// the source clock and kinetics clock differ, a controller starts with a full
// four-row window, transport spans two regions, and all three channels have a
// distinct nonzero source and receptor. This is the public training path a
// segmented replay must preserve.
func chemicalRecomputeFixture(t *testing.T) (learning.Config, learning.Parameters, [][]float64, []float64, [][]float64) {
	t.Helper()
	expression := &learning.ExpressionGain{Nodes: []int{0}, Receptor: 0, Scale: 0.7, Min: 0.5, Max: 2}
	config, parameters, episode := chemicalEpisodeControllerConfig(2, false, expression)
	declaration := &episode.Part.Config
	declaration.Chemistry.Channels = 3
	declaration.Chemistry.Tau = []float64{2, 2.5, 3}
	declaration.Chemistry.Transport = &modulation.Transport{Fraction: [][]float64{{0, 0.2}, {0.3, 0}}}
	declaration.Sources = []modulation.SourceSpec{
		{
			Kind: modulation.SourceController, Channel: 0,
			Controller: &modulation.Controller{
				Inputs: modulation.ControllerInputs{SummaryWindow: 4},
				Nodes:  2, Hidden: 1, Channel: 0,
				Parameters: []float64{0.6, 0.2, 0.05, 0.8, -0.1},
			},
		},
		{
			Kind: modulation.SourceExternalTimeline, Channel: 1,
			Timeline: &modulation.ExternalTimeline{
				ChannelCount: 2,
				Entries: []modulation.TimelineEntry{
					{Step: 8, Channel: 1, Rate: 0.75},
					{Step: 11, Channel: 1, Rate: 0.35},
				},
			},
		},
		{
			Kind: modulation.SourceNeuralActivity, Channel: 2,
			Neural: &modulation.NeuralActivity{Nodes: []int{0, 1}, SetName: "both", Gain: 1.2, Channel: 2},
		},
	}
	declaration.Receptors.Records = []modulation.Receptor{
		{
			Cells: []int{0}, Signal: "channel-0", Channel: 0, Status: modulation.StatusHypothesized,
			Kd: 0.45, N: 1.1, Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chemical-recompute/v1",
			Location: &modulation.ReceptorLocation{Region: 1, Compartment: "postsynaptic", AnatomicalLocation: "region-1", Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chemical-recompute-location/v1"},
		},
		{
			Cells: []int{1}, Signal: "channel-1", Channel: 1, Status: modulation.StatusHypothesized,
			Kd: 0.6, N: 1.3, Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chemical-recompute/v1",
			Location: &modulation.ReceptorLocation{Region: 0, Compartment: "postsynaptic", AnatomicalLocation: "region-0", Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chemical-recompute-location/v1"},
		},
		{
			Cells: []int{0, 1}, Signal: "channel-2", Channel: 2, Status: modulation.StatusHypothesized,
			Kd: 0.7, N: 1.5, Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chemical-recompute/v1",
			Location: &modulation.ReceptorLocation{Region: 1, Compartment: "postsynaptic", AnatomicalLocation: "region-1", Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chemical-recompute-location/v1"},
		},
	}
	declaration.Effects = []modulation.Effect{
		{Kind: modulation.EffectSensitivity, Receptor: 0, GammaScale: 0.8, BetaScale: 0.1},
		{Kind: modulation.EffectSensitivity, Receptor: 1, GammaScale: 0.8, BetaScale: 0.1},
		{Kind: modulation.EffectSensitivity, Receptor: 2, GammaScale: 0.8, BetaScale: 0.1},
	}
	declaration.Regions.NodeRegion = []int{0, 1}
	episode.Part.State = modulation.ChemistryState{
		Concentration: [][]float64{{0.21, 0.13, 0.07}, {0.16, 0.24, 0.11}},
		Steps:         3,
	}
	episode.Part.SourceStates = []learning.ChemicalSourceState{
		{Controller: &modulation.ControllerState{
			Window: [][]float64{{0.08, -0.02}, {0.12, 0.03}, {0.18, 0.06}, {0.22, 0.1}}, Filled: 4,
		}},
		{},
		{},
	}
	episode.Part.Resources = map[string]float64{"energy": 0.8}
	episode.Part.PendingFeedback = []signal.FeedbackSpec{
		chemicalEpisodeFeedbackSpec(t, 8, 9, 0.35),
	}
	episode.InitialActivity = []float64{0.2, -0.1}
	config.Chemistry = episode

	inputs := make([][]float64, 8)
	upstream := make([][]float64, 8)
	for step := range inputs {
		f := float64(step)
		inputs[step] = []float64{0.35 + 0.06*f}
		upstream[step] = []float64{0.08 - 0.006*f}
	}
	target := []float64{0.24}
	return config, parameters, inputs, target, upstream
}

func chemicalRecomputeOptions(segment, window, accumulate int) learning.Options {
	o := learning.DefaultOptions()
	o.LearningRate = 0.013
	o.Beta1 = 0.83
	o.Beta2 = 0.991
	o.Epsilon = 1e-8
	o.WeightDecay = 0.001
	o.ClipNorm = 0.7
	o.Truncation = window
	o.AccumulateSteps = accumulate
	o.Masks = &learning.UpdateMasks{Edges: []bool{true, false}, Nodes: []bool{true, false}}
	o.Recompute = &learning.Recompute{SegmentSteps: segment}
	return o
}

func assertChemicalRecomputeSnapshot(t *testing.T, label string, got, want learning.TrainingSnapshot) {
	t.Helper()
	gotOptions, wantOptions := got.Options, want.Options
	gotOptions.Recompute, wantOptions.Recompute = nil, nil
	if !reflect.DeepEqual(got.Config, want.Config) || !reflect.DeepEqual(gotOptions, wantOptions) {
		t.Fatalf("%s config/options differ", label)
	}
	assertRecomputeBits(t, label+" weights", got.Parameters.Core.Weights, want.Parameters.Core.Weights)
	assertRecomputeBits(t, label+" bias", got.Parameters.Core.Bias, want.Parameters.Core.Bias)
	assertRecomputeBits(t, label+" log tau", got.Parameters.Core.LogTau, want.Parameters.Core.LogTau)
	assertRecomputeBits(t, label+" theta", got.Parameters.ThetaRaw, want.Parameters.ThetaRaw)
	assertRecomputeBits(t, label+" encoder", got.Parameters.Encoder, want.Parameters.Encoder)
	assertRecomputeBits(t, label+" readout", got.Parameters.Readout, want.Parameters.Readout)
	assertRecomputeBits(t, label+" optimizer first", got.Optimizer.First, want.Optimizer.First)
	assertRecomputeBits(t, label+" optimizer second", got.Optimizer.Second, want.Optimizer.Second)
	if !reflect.DeepEqual(got.Optimizer.Steps, want.Optimizer.Steps) || got.Updates != want.Updates || !reflect.DeepEqual(got.Accumulator, want.Accumulator) {
		t.Fatalf("%s optimizer/update state differs: got updates=%d accumulator=%+v want updates=%d accumulator=%+v", label, got.Updates, got.Accumulator, want.Updates, want.Accumulator)
	}
}

func chemicalRecomputeRowsCopy(rows [][]float64) [][]float64 {
	owned := make([][]float64, len(rows))
	for row, values := range rows {
		owned[row] = append([]float64(nil), values...)
	}
	return owned
}

func TestChemicalRecomputeFixtureUsesAllDeclaredBoundaries(t *testing.T) {
	config, parameters, inputs, _, _ := chemicalRecomputeFixture(t)
	if config.Chemistry == nil {
		t.Fatal("fixture has no chemical episode")
	}
	episode := config.Chemistry
	if episode.SourceStep == episode.Part.State.Steps {
		t.Fatalf("source clock=%d and kinetics clock=%d accidentally agree", episode.SourceStep, episode.Part.State.Steps)
	}
	if episode.Part.Config.Chemistry.Regions != 2 || episode.Part.Config.Chemistry.Channels != 3 {
		t.Fatalf("chemistry dimensions=%+v, want two regions and three channels", episode.Part.Config.Chemistry)
	}
	if len(episode.Part.SourceStates) != 3 || episode.Part.SourceStates[0].Controller == nil || len(episode.Part.SourceStates[0].Controller.Window) != 4 || episode.Part.SourceStates[0].Controller.Filled != 4 {
		t.Fatalf("controller source state=%+v, want a filled four-row boundary", episode.Part.SourceStates)
	}
	for row, values := range episode.Part.SourceStates[0].Controller.Window {
		if len(values) != 2 {
			t.Fatalf("source state row %d width=%d, want 2", row, len(values))
		}
	}
	if location := episode.Part.Config.Receptors.Records[0].Location; location == nil || location.Region != 1 {
		t.Fatalf("receptor location=%+v, want region 1", location)
	}
	for channel, row := range episode.Part.State.Concentration {
		for column, value := range row {
			if value <= 0 {
				t.Fatalf("concentration[%d][%d]=%g, want nonzero fixture state", channel, column, value)
			}
		}
	}
	network, err := learning.NewNetwork(config)
	if err != nil {
		t.Fatalf("NewNetwork: %v", err)
	}
	rows, err := network.PredictAll(context.Background(), parameters, inputs)
	if err != nil {
		t.Fatalf("chemical PredictAll: %v", err)
	}
	nonzero := false
	for _, row := range rows {
		for _, value := range row {
			nonzero = nonzero || value != 0
		}
	}
	if !nonzero {
		t.Fatal("chemical fixture prediction is all zero")
	}
	plain := config
	plain.Chemistry = nil
	plainNetwork, err := learning.NewNetwork(plain)
	if err != nil {
		t.Fatalf("plain NewNetwork: %v", err)
	}
	plainRows, err := plainNetwork.PredictAll(context.Background(), parameters, inputs)
	if err != nil {
		t.Fatalf("plain PredictAll: %v", err)
	}
	if reflect.DeepEqual(rows, plainRows) {
		t.Fatal("declared chemistry and nil chemistry produced identical rows")
	}
}

func TestChemicalRecomputeTrainerMatchesFullHistory(t *testing.T) {
	ctx := context.Background()
	for _, segment := range []int{1, 4, 7, 12} {
		for _, window := range []int{0, 3, 5} {
			for _, accumulate := range []int{0, 2} {
				t.Run(fmt.Sprintf("segment=%d/window=%d/accumulate=%d", segment, window, accumulate), func(t *testing.T) {
					config, parameters, inputs, target, upstream := chemicalRecomputeFixture(t)
					fullOptions := chemicalRecomputeOptions(segment, window, accumulate)
					fullOptions.Recompute = nil
					recomputeOptions := chemicalRecomputeOptions(segment, window, accumulate)
					full, err := learning.NewTrainer(config, parameters, fullOptions)
					if err != nil {
						t.Fatalf("full NewTrainer: %v", err)
					}
					defer full.Close()
					recompute, err := learning.NewTrainer(config, parameters, recomputeOptions)
					if err != nil {
						t.Fatalf("recompute NewTrainer: %v", err)
					}
					defer recompute.Close()
					want, err := full.Step(ctx, inputs, target)
					if err != nil {
						t.Fatalf("full Step: %v", err)
					}
					got, err := recompute.Step(ctx, inputs, target)
					if err != nil {
						t.Fatalf("recompute Step: %v", err)
					}
					assertRecomputeResult(t, "Step", got, want)
					wantFrom, err := full.StepFrom(ctx, inputs, upstream)
					if err != nil {
						t.Fatalf("full StepFrom: %v", err)
					}
					gotFrom, err := recompute.StepFrom(ctx, inputs, upstream)
					if err != nil {
						t.Fatalf("recompute StepFrom: %v", err)
					}
					assertRecomputeResult(t, "StepFrom", gotFrom, wantFrom)
					assertChemicalRecomputeSnapshot(t, "final", recompute.Snapshot(), full.Snapshot())
				})
			}
		}
	}
}

func TestChemicalRecomputeKeepsNilChemistryLegacyPath(t *testing.T) {
	fx := recomputeTrainerFixtures()[0]
	fullOptions := fx.options
	recomputeOptions := fullOptions
	recomputeOptions.Recompute = &learning.Recompute{SegmentSteps: 4}
	full, err := learning.NewTrainer(fx.config, fx.params, fullOptions)
	if err != nil {
		t.Fatalf("full NewTrainer: %v", err)
	}
	defer full.Close()
	recompute, err := learning.NewTrainer(fx.config, fx.params, recomputeOptions)
	if err != nil {
		t.Fatalf("recompute NewTrainer: %v", err)
	}
	defer recompute.Close()
	want, err := full.Step(context.Background(), fx.input, fx.target)
	if err != nil {
		t.Fatalf("full Step: %v", err)
	}
	got, err := recompute.Step(context.Background(), fx.input, fx.target)
	if err != nil {
		t.Fatalf("legacy recompute Step: %v", err)
	}
	assertRecomputeResult(t, "legacy Step", got, want)
	wantFrom, err := full.StepFrom(context.Background(), fx.input, fx.upstream)
	if err != nil {
		t.Fatalf("full StepFrom: %v", err)
	}
	gotFrom, err := recompute.StepFrom(context.Background(), fx.input, fx.upstream)
	if err != nil {
		t.Fatalf("legacy recompute StepFrom: %v", err)
	}
	assertRecomputeResult(t, "legacy StepFrom", gotFrom, wantFrom)
	assertRecomputeCapacitySnapshot(t, "legacy final", recompute.Snapshot(), full.Snapshot())
}

func TestChemicalRecomputeFailuresPreserveTrainerState(t *testing.T) {
	config, parameters, inputs, target, upstream := chemicalRecomputeFixture(t)
	o := chemicalRecomputeOptions(4, 3, 2)
	trainer, err := learning.NewTrainer(config, parameters, o)
	if err != nil {
		t.Fatalf("NewTrainer: %v", err)
	}
	defer trainer.Close()
	checkUnchanged := func(name string, call func() (learning.StepResult, error)) {
		t.Helper()
		before := trainer.Snapshot()
		result, err := call()
		if err == nil || !reflect.DeepEqual(result, learning.StepResult{}) {
			t.Fatalf("%s result=%+v err=%v, want zero result and error", name, result, err)
		}
		if !reflect.DeepEqual(before, trainer.Snapshot()) {
			t.Fatalf("%s changed parameters, optimizer, accumulator or episode boundary", name)
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	checkUnchanged("canceled Step", func() (learning.StepResult, error) {
		return trainer.Step(canceled, inputs, target)
	})
	badInput := chemicalRecomputeRowsCopy(inputs)
	badInput[2][0] = math.NaN()
	checkUnchanged("NaN Step input", func() (learning.StepResult, error) {
		return trainer.Step(context.Background(), badInput, target)
	})
	badTarget := []float64{math.Inf(1)}
	checkUnchanged("nonfinite target", func() (learning.StepResult, error) {
		return trainer.Step(context.Background(), inputs, badTarget)
	})
	badUpstream := chemicalRecomputeRowsCopy(upstream)
	badUpstream[4][0] = math.NaN()
	checkUnchanged("NaN StepFrom upstream", func() (learning.StepResult, error) {
		return trainer.StepFrom(context.Background(), inputs, badUpstream)
	})
}

func TestChemicalRecomputeKeepsStatefulAndMixedUnsupported(t *testing.T) {
	fixture := newChemicalStatefulFixture(t)
	config := chemicalStatefulConfig(fixture.snapshot)
	o := fixture.options
	o.Recompute = &learning.Recompute{SegmentSteps: 1}
	trainer, err := learning.NewTrainer(config, fixture.parameters, o)
	if err != nil {
		t.Fatalf("stateful chemical NewTrainer: %v", err)
	}
	defer trainer.Close()
	before := trainer.Snapshot()
	result, err := trainer.StepFromState(context.Background(), fixture.snapshot.Neural, fixture.input, fixture.upstream)
	if err == nil || !reflect.DeepEqual(result, learning.StepResult{}) {
		t.Fatalf("stateful StepFromState result=%+v err=%v, want explicit unsupported error", result, err)
	}
	if !reflect.DeepEqual(before, trainer.Snapshot()) {
		t.Fatal("unsupported stateful chemical recompute changed trainer state")
	}
	mixedOptions := learning.DefaultOptions()
	mixedOptions.Recompute = &learning.Recompute{SegmentSteps: 2}
	if _, err := learning.NewTrainer(mixedNodeConfig(), mixedNodeParameters(), mixedOptions); err == nil {
		t.Fatal("mixed recompute was accepted; existing unsupported boundary changed")
	}
}

type chemicalRecomputeLIFFixture struct {
	config     learning.Config
	parameters learning.Parameters
	inputs     [][]float64
	target     []float64
	upstream   [][]float64
}

// chemicalRecomputeLIFFixture uses the public LIF configuration path while
// keeping every chemistry boundary observable. The controller reads a filled
// four-row window, arrived feedback and an energy resource; the other sources
// exercise a global timeline and previous raw neural activity.
func newChemicalRecomputeLIFFixture(t *testing.T, feedbackAvailable int64, energy float64) chemicalRecomputeLIFFixture {
	t.Helper()
	lif := dynamics.LIFConfig{
		Nodes: 2, Sources: []int{0, 1}, Targets: []int{1, 0}, Delays: []int{1, 1},
		DT: 0.5, TauSyn: 0.8, ThetaMin: 0.1, ThetaMax: 1.7, VReset: -0.5,
		RefractorySteps: 2,
		Adaptation:      dynamics.LIFAdaptation{Enabled: true, TauAdapt: 1.1, Beta: 0.18},
		Homeostasis:     &dynamics.LIFHomeostasis{Enabled: true, TauRate: 1.7, TargetRate: 0.2, Eta: 0.2, HMax: 0.4},
		Surrogate:       dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 1.7},
	}
	controller := &modulation.Controller{
		Inputs: modulation.ControllerInputs{SummaryWindow: 4, UseFeedback: true, Resources: []string{"energy"}},
		Nodes:  2, Hidden: 1, Channel: 0,
		// InputWidth is mean, variance, feedback and energy. The remaining
		// three values are b1, w2 and b2 for one hidden unit.
		Parameters: []float64{0.35, 0.1, 0.4, 0.2, 0.05, 0.7, -0.1},
	}
	chemistry := modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{
			Regions: 2, Channels: 3, DT: 1, Tau: []float64{2, 2.5, 3},
			Transport: &modulation.Transport{Fraction: [][]float64{{0, 0.2}, {0.3, 0}}},
		},
		Sources: []modulation.SourceSpec{
			{Kind: modulation.SourceController, Channel: 0, Controller: controller},
			{Kind: modulation.SourceExternalTimeline, Channel: 1, Timeline: &modulation.ExternalTimeline{
				ChannelCount: 2,
				Entries: []modulation.TimelineEntry{
					{Step: 8, Channel: 1, Rate: 0.75},
					{Step: 11, Channel: 1, Rate: 0.35},
				},
			}},
			{Kind: modulation.SourceNeuralActivity, Channel: 2, Neural: &modulation.NeuralActivity{
				Nodes: []int{0, 1}, SetName: "both", Gain: 1.2, Channel: 2,
			}},
		},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{
			{
				Cells: []int{0}, Signal: "channel-0", Channel: 0, Status: modulation.StatusHypothesized,
				Kd: 0.45, N: 1.2, Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chemical-recompute-lif/v1",
				Location: &modulation.ReceptorLocation{Region: 1, Compartment: "postsynaptic", AnatomicalLocation: "region-1", Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chemical-recompute-lif-location/v1"},
			},
			{
				Cells: []int{1}, Signal: "channel-1", Channel: 1, Status: modulation.StatusHypothesized,
				Kd: 0.6, N: 1.1, Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chemical-recompute-lif/v1",
				Location: &modulation.ReceptorLocation{Region: 0, Compartment: "postsynaptic", AnatomicalLocation: "region-0", Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chemical-recompute-lif-location/v1"},
			},
			{
				Cells: []int{0, 1}, Signal: "channel-2", Channel: 2, Status: modulation.StatusHypothesized,
				Kd: 0.7, N: 1.4, Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chemical-recompute-lif/v1",
				Location: &modulation.ReceptorLocation{Region: 1, Compartment: "postsynaptic", AnatomicalLocation: "region-1", Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chemical-recompute-lif-location/v1"},
			},
		}},
		Effects: []modulation.Effect{
			{Kind: modulation.EffectThreshold, Receptor: 0, ThetaScale: 0.35, ThetaAbsMax: 0.8},
			{Kind: modulation.EffectSensitivity, Receptor: 1, GammaScale: 0.65, BetaScale: 0.08},
			{Kind: modulation.EffectSensitivity, Receptor: 2, GammaScale: 0.65, BetaScale: 0.08},
		},
		Regions: modulation.RegionAssignment{NodeRegion: []int{0, 1}},
	}
	expression := &learning.ExpressionGain{Nodes: []int{0}, Receptor: 0, Scale: 0.7, Min: 0.5, Max: 2}
	episode := &learning.ChemicalEpisode{
		Part: learning.ChemicalPart{
			Config: chemistry,
			State: modulation.ChemistryState{
				Concentration: [][]float64{{0.21, 0.13, 0.07}, {0.16, 0.24, 0.11}},
				Steps:         3,
			},
			Resources: map[string]float64{"energy": energy},
			PendingFeedback: []signal.FeedbackSpec{
				chemicalEpisodeFeedbackSpec(t, 8, feedbackAvailable, 0.35),
			},
			SourceStates: []learning.ChemicalSourceState{
				{Controller: &modulation.ControllerState{
					Window: [][]float64{{0.08, -0.02}, {0.12, 0.03}, {0.18, 0.06}, {0.22, 0.1}}, Filled: 4,
				}},
				{},
				{},
			},
			Expression: expression,
		},
		SourceStep:      7,
		InitialActivity: []float64{0.15, -0.05},
	}
	config := learning.Config{
		LIF: &lif, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{0, 1}, InputNodes: []int{0, 1},
		ReadoutEveryStep: true, Chemistry: episode,
	}
	parameters := learning.Parameters{
		Core: dynamics.Parameters{
			Weights: []float64{0.22, -0.18}, Bias: []float64{0.1, 0.08}, LogTau: []float64{-0.15, -0.1},
		},
		ThetaRaw: []float64{-0.2, -0.1}, Encoder: []float64{1.2, 0.95}, Readout: []float64{0.7, -0.45},
	}
	inputs := make([][]float64, 8)
	upstream := make([][]float64, 8)
	for step := range inputs {
		inputs[step] = []float64{1.35 + 0.05*float64(step%3)}
		upstream[step] = []float64{0.04 - 0.003*float64(step)}
	}
	return chemicalRecomputeLIFFixture{config: config, parameters: parameters, inputs: inputs, target: []float64{0.18}, upstream: upstream}
}

func chemicalRecomputeLIFOptions(segment, window int) learning.Options {
	o := chemicalRecomputeOptions(segment, window, 2)
	o.Trainable.Theta = true
	return o
}

func assertChemicalRecomputeLIFAdamGroups(t *testing.T, snapshot learning.TrainingSnapshot) {
	t.Helper()
	steps := snapshot.Optimizer.Steps
	if len(steps) != 12 {
		t.Fatalf("optimizer step layout has %d entries, want weights/bias/log_tau/theta/encoder/readout = 12", len(steps))
	}
	groups := []struct {
		name string
		from int
		mask []bool
	}{
		{name: "weights", from: 0, mask: []bool{true, false}},
		{name: "bias", from: 2, mask: []bool{true, false}},
		{name: "log_tau", from: 4, mask: []bool{true, false}},
		{name: "theta_raw", from: 6, mask: []bool{true, false}},
		{name: "encoder", from: 8, mask: []bool{true, true}},
		{name: "readout", from: 10, mask: []bool{true, true}},
	}
	for _, group := range groups {
		for item, masked := range group.mask {
			got := steps[group.from+item]
			if masked && got == 0 {
				t.Fatalf("optimizer group %s item %d never committed", group.name, item)
			}
			if !masked && got != 0 {
				t.Fatalf("masked optimizer group %s item %d committed %d times", group.name, item, got)
			}
		}
	}
}

func TestChemicalRecomputeLIFFixtureUsesFeedbackResourceAndThreshold(t *testing.T) {
	fixture := newChemicalRecomputeLIFFixture(t, 9, 0.8)
	if fixture.config.LIF == nil {
		t.Fatal("fixture did not select the public LIF config path")
	}
	if _, err := dynamics.NewLIF(*fixture.config.LIF); err != nil {
		t.Fatalf("public NewLIF rejected fixture: %v", err)
	}
	episode := fixture.config.Chemistry
	if episode == nil {
		t.Fatal("LIF fixture has no chemical episode")
	}
	if episode.SourceStep == episode.Part.State.Steps {
		t.Fatalf("source clock=%d and kinetics clock=%d accidentally agree", episode.SourceStep, episode.Part.State.Steps)
	}
	if episode.Part.Config.Chemistry.Regions != 2 || episode.Part.Config.Chemistry.Channels != 3 {
		t.Fatalf("chemistry dimensions=%+v, want two regions and three channels", episode.Part.Config.Chemistry)
	}
	if maxDelay := fixture.config.LIF.Delays[0]; episode.Part.Config.Sources[0].Controller.Inputs.SummaryWindow <= maxDelay {
		t.Fatalf("controller SummaryWindow=%d is not above max delay %d", episode.Part.Config.Sources[0].Controller.Inputs.SummaryWindow, maxDelay)
	}
	controller := episode.Part.Config.Sources[0].Controller
	if controller == nil || !controller.Inputs.UseFeedback || len(controller.Inputs.Resources) != 1 || controller.Inputs.Resources[0] != "energy" || len(controller.Parameters) != 7 {
		t.Fatalf("controller boundary=%+v, want feedback/resource inputs and width-4 parameters", controller)
	}
	if len(episode.Part.SourceStates) != 3 || episode.Part.SourceStates[0].Controller == nil || episode.Part.SourceStates[0].Controller.Filled != 4 {
		t.Fatalf("source states=%+v, want filled controller plus two stateless entries", episode.Part.SourceStates)
	}
	if episode.Part.Config.Receptors.Records[0].Location == nil || episode.Part.Config.Receptors.Records[0].Location.Region != 1 {
		t.Fatalf("receptor location=%+v, want explicit region 1", episode.Part.Config.Receptors.Records[0].Location)
	}
	if episode.Part.Expression == nil || len(episode.Part.Config.Effects) != 3 || episode.Part.Config.Effects[0].Kind != modulation.EffectThreshold {
		t.Fatalf("LIF chemical effects/expression=%+v/%+v, want threshold plus sensitivity and expression", episode.Part.Config.Effects, episode.Part.Expression)
	}

	network, err := learning.NewNetwork(fixture.config)
	if err != nil {
		t.Fatalf("NewNetwork: %v", err)
	}
	rows, err := network.PredictAll(context.Background(), fixture.parameters, fixture.inputs)
	if err != nil {
		t.Fatalf("chemical LIF PredictAll: %v", err)
	}
	spikeTrainer, err := learning.NewTrainer(fixture.config, fixture.parameters, learning.DefaultOptions())
	if err != nil {
		t.Fatalf("LIF spike trainer: %v", err)
	}
	defer spikeTrainer.Close()
	spikes, err := spikeTrainer.Spikes(context.Background(), fixture.inputs)
	if err != nil {
		t.Fatalf("chemical LIF Spikes: %v", err)
	}
	spikeCount := 0
	for _, row := range spikes {
		for _, value := range row {
			if value != 0 {
				spikeCount++
			}
		}
	}
	if spikeCount == 0 {
		t.Fatal("LIF fixture produced no hard spikes; threshold, adaptation and refractory paths were not exercised")
	}
	plainConfig := fixture.config
	plainConfig.Chemistry = nil
	plainNetwork, err := learning.NewNetwork(plainConfig)
	if err != nil {
		t.Fatalf("plain NewNetwork: %v", err)
	}
	plainRows, err := plainNetwork.PredictAll(context.Background(), fixture.parameters, fixture.inputs)
	if err != nil {
		t.Fatalf("plain LIF PredictAll: %v", err)
	}
	if reflect.DeepEqual(rows, plainRows) {
		t.Fatal("LIF chemistry, threshold effect or expression did not change observable readout")
	}

	late := newChemicalRecomputeLIFFixture(t, 100, 0.8)
	lateNetwork, err := learning.NewNetwork(late.config)
	if err != nil {
		t.Fatalf("late-feedback NewNetwork: %v", err)
	}
	lateRows, err := lateNetwork.PredictAll(context.Background(), late.parameters, late.inputs)
	if err != nil {
		t.Fatalf("late-feedback PredictAll: %v", err)
	}
	if reflect.DeepEqual(rows, lateRows) {
		t.Fatal("moving feedback availability did not change the controller source output")
	}

	richer := newChemicalRecomputeLIFFixture(t, 9, 1.6)
	richerNetwork, err := learning.NewNetwork(richer.config)
	if err != nil {
		t.Fatalf("richer-resource NewNetwork: %v", err)
	}
	richerRows, err := richerNetwork.PredictAll(context.Background(), richer.parameters, richer.inputs)
	if err != nil {
		t.Fatalf("richer-resource PredictAll: %v", err)
	}
	if reflect.DeepEqual(rows, richerRows) {
		t.Fatal("changing energy resource did not change the controller source output")
	}

	withoutExpression := newChemicalRecomputeLIFFixture(t, 9, 0.8)
	withoutExpression.config.Chemistry.Part.Expression = nil
	withoutExpressionNetwork, err := learning.NewNetwork(withoutExpression.config)
	if err != nil {
		t.Fatalf("without-expression NewNetwork: %v", err)
	}
	withoutExpressionRows, err := withoutExpressionNetwork.PredictAll(context.Background(), withoutExpression.parameters, withoutExpression.inputs)
	if err != nil {
		t.Fatalf("without-expression PredictAll: %v", err)
	}
	if reflect.DeepEqual(rows, withoutExpressionRows) {
		t.Fatal("expression gain did not change observable LIF readout")
	}
}

func TestChemicalRecomputeLIFTrainerMatchesFullHistory(t *testing.T) {
	ctx := context.Background()
	for _, segment := range []int{1, 4, 7} {
		for _, window := range []int{0, 3} {
			t.Run(fmt.Sprintf("segment=%d/window=%d", segment, window), func(t *testing.T) {
				fixture := newChemicalRecomputeLIFFixture(t, 9, 0.8)
				fullOptions := chemicalRecomputeLIFOptions(segment, window)
				fullOptions.Recompute = nil
				recomputeOptions := chemicalRecomputeLIFOptions(segment, window)
				full, err := learning.NewTrainer(fixture.config, fixture.parameters, fullOptions)
				if err != nil {
					t.Fatalf("full NewTrainer: %v", err)
				}
				defer full.Close()
				recompute, err := learning.NewTrainer(fixture.config, fixture.parameters, recomputeOptions)
				if err != nil {
					t.Fatalf("recompute NewTrainer: %v", err)
				}
				defer recompute.Close()
				initial := full.Snapshot()
				for operation := 0; operation < 4; operation++ {
					wantRows, err := full.PredictAll(ctx, fixture.inputs)
					if err != nil {
						t.Fatalf("full PredictAll %d: %v", operation, err)
					}
					gotRows, err := recompute.PredictAll(ctx, fixture.inputs)
					if err != nil {
						t.Fatalf("recompute PredictAll %d: %v", operation, err)
					}
					assertRecomputeRows(t, fmt.Sprintf("prediction %d", operation), gotRows, wantRows)

					var want, got learning.StepResult
					if operation%2 == 0 {
						want, err = full.Step(ctx, fixture.inputs, fixture.target)
						if err != nil {
							t.Fatalf("full Step %d: %v", operation, err)
						}
						got, err = recompute.Step(ctx, fixture.inputs, fixture.target)
						if err != nil {
							t.Fatalf("recompute Step %d: %v", operation, err)
						}
						if got.Applied || got.Accumulated != 1 {
							t.Fatalf("partial Step %d result=%+v, want one pending accumulation", operation, got)
						}
					} else {
						want, err = full.StepFrom(ctx, fixture.inputs, fixture.upstream)
						if err != nil {
							t.Fatalf("full StepFrom %d: %v", operation, err)
						}
						got, err = recompute.StepFrom(ctx, fixture.inputs, fixture.upstream)
						if err != nil {
							t.Fatalf("recompute StepFrom %d: %v", operation, err)
						}
						if !got.Applied || got.Accumulated != 0 {
							t.Fatalf("committing StepFrom %d result=%+v, want an applied update", operation, got)
						}
					}
					assertRecomputeResult(t, fmt.Sprintf("operation %d", operation), got, want)
					assertChemicalRecomputeSnapshot(t, fmt.Sprintf("snapshot %d", operation), recompute.Snapshot(), full.Snapshot())
				}
				final := recompute.Snapshot()
				if final.Updates != 2 || full.Snapshot().Updates != 2 {
					t.Fatalf("updates full=%d recompute=%d, want two committed updates", full.Snapshot().Updates, final.Updates)
				}
				if reflect.DeepEqual(final.Parameters, initial.Parameters) {
					t.Fatal("two committed LIF chemistry updates left every parameter unchanged")
				}
				assertChemicalRecomputeLIFAdamGroups(t, final)
			})
		}
	}
}
