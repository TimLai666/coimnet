package learning_test

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/signal"
)

const chemicalEpisodeEpsilon = 1e-3

func chemicalEpisodeScalarConfig(episode *learning.ChemicalEpisode, everyStep bool) learning.Config {
	return learning.Config{
		Dynamics: dynamics.Config{
			Nodes: 1, Sources: []int{}, Targets: []int{}, Delays: []int{}, DT: 1, Activation: "tanh",
		},
		InputSize: 1, OutputSize: 1, ReadoutNodes: []int{0}, InputNodes: []int{0},
		ReadoutEveryStep: everyStep, Chemistry: episode,
	}
}

func chemicalEpisodeScalarParameters() learning.Parameters {
	return learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{}, Bias: []float64{0}, LogTau: []float64{0}},
		Encoder: []float64{1}, Readout: []float64{1},
	}
}

func chemicalEpisodePart(config modulation.ChemistryConfig, concentration [][]float64, expression *learning.ExpressionGain) learning.ChemicalPart {
	return learning.ChemicalPart{
		Config:     config,
		State:      modulation.ChemistryState{Concentration: concentration},
		Resources:  map[string]float64{},
		Expression: expression,
	}
}

func chemicalEpisodePulse(sourceStep uint64, location *modulation.ReceptorLocation, transport *modulation.Transport, expression *learning.ExpressionGain) *learning.ChemicalEpisode {
	regions := 1
	if location != nil && location.Region+1 > regions {
		regions = location.Region + 1
	}
	if transport != nil {
		regions = len(transport.Fraction)
	}
	concentration := make([][]float64, regions)
	for region := range concentration {
		concentration[region] = []float64{0}
	}
	config := modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{
			Regions: regions, Channels: 1, DT: 1, Tau: []float64{2}, Transport: transport,
		},
		Sources: []modulation.SourceSpec{{
			Kind: modulation.SourceExternalTimeline, Channel: 0,
			Timeline: &modulation.ExternalTimeline{
				ChannelCount: 1,
				Entries:      []modulation.TimelineEntry{{Step: sourceStep, Channel: 0, Rate: 1}},
			},
		}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{{
			Cells: []int{0}, Signal: "fixture", Channel: 0, Status: modulation.StatusHypothesized,
			Kd: 0.5, N: 1, Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "test/v1",
			Location: location,
		}}},
		Effects: []modulation.Effect{{Kind: modulation.EffectSensitivity, Receptor: 0, GammaScale: 1}},
		Regions: modulation.RegionAssignment{NodeRegion: []int{0}},
	}
	return &learning.ChemicalEpisode{
		Part:       chemicalEpisodePart(config, concentration, expression),
		SourceStep: sourceStep,
	}
}

func chemicalEpisodeReceptorOnly(concentration float64, expression *learning.ExpressionGain) *learning.ChemicalEpisode {
	config := modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{2}},
		Sources: []modulation.SourceSpec{{
			Kind: modulation.SourceExternalTimeline, Channel: 0,
			Timeline: &modulation.ExternalTimeline{ChannelCount: 1, Entries: []modulation.TimelineEntry{}},
		}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{{
			Cells: []int{0}, Signal: "fixture", Channel: 0, Status: modulation.StatusHypothesized,
			Kd: 0.5, N: 1, Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "test/v1",
		}}},
		Regions: modulation.RegionAssignment{NodeRegion: []int{0}},
	}
	return &learning.ChemicalEpisode{
		Part: chemicalEpisodePart(config, [][]float64{{concentration}}, expression),
	}
}

func chemicalEpisodeControllerConfig(nodes int, useFeedback bool, expression *learning.ExpressionGain) (learning.Config, learning.Parameters, *learning.ChemicalEpisode) {
	controllerParameters := []float64{0.6, 0.2, 0.05, 0.8, -0.1}
	if useFeedback {
		controllerParameters = []float64{0, 0, 1, 0, 1, 0}
	}
	controller := &modulation.Controller{
		Inputs: modulation.ControllerInputs{SummaryWindow: 3, UseFeedback: useFeedback},
		Nodes:  nodes, Hidden: 1, Channel: 0, Parameters: controllerParameters,
	}
	regions := 1
	nodeRegion := make([]int, nodes)
	for i := range nodeRegion {
		nodeRegion[i] = 0
	}
	config := modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: regions, Channels: 1, DT: 1, Tau: []float64{2}},
		Sources:   []modulation.SourceSpec{{Kind: modulation.SourceController, Channel: 0, Controller: controller}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{{
			Cells: []int{0}, Signal: "fixture", Channel: 0, Status: modulation.StatusHypothesized,
			Kd: 0.5, N: 1, Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "test/v1",
		}}},
		Effects: []modulation.Effect{{Kind: modulation.EffectSensitivity, Receptor: 0, GammaScale: 0.8, BetaScale: 0.1}},
		Regions: modulation.RegionAssignment{NodeRegion: nodeRegion},
	}
	episode := &learning.ChemicalEpisode{
		Part:       chemicalEpisodePart(config, [][]float64{{0}}, expression),
		SourceStep: 7,
		InitialActivity: func() []float64 {
			activity := make([]float64, nodes)
			for i := range activity {
				activity[i] = 0.2 - 0.1*float64(i)
			}
			return activity
		}(),
	}
	if nodes == 2 && !useFeedback {
		episode.Part.Config.Chemistry.Regions = 2
		episode.Part.Config.Chemistry.Transport = &modulation.Transport{Fraction: [][]float64{{0, 0.2}, {0.3, 0}}}
		episode.Part.Config.Regions.NodeRegion = []int{0, 1}
		episode.Part.State.Concentration = [][]float64{{0}, {0}}
		episode.Part.Config.Receptors.Records[0].Location = &modulation.ReceptorLocation{
			Region: 1, Compartment: "postsynaptic", AnatomicalLocation: "fixture-region-1",
			Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "test/location-v1",
		}
	}
	core := dynamics.Config{
		Nodes: nodes, Sources: []int{0, 1}, Targets: []int{0, 1}, Delays: []int{1, 1}, DT: 0.7, Activation: "tanh",
	}
	if nodes == 1 {
		core.Sources = []int{}
		core.Targets = []int{}
		core.Delays = []int{}
	}
	configOut := learning.Config{
		Dynamics: core, InputSize: 1, OutputSize: 1,
		ReadoutNodes: func() []int {
			if nodes == 1 {
				return []int{0}
			}
			return []int{0, 1}
		}(),
		InputNodes: func() []int {
			if nodes == 1 {
				return []int{0}
			}
			return []int{0, 1}
		}(),
		ReadoutEveryStep: true, Chemistry: episode,
	}
	parameters := learning.Parameters{
		Core: dynamics.Parameters{Weights: func() []float64 {
			if nodes == 1 {
				return []float64{}
			}
			return []float64{0.25, -0.18}
		}(), Bias: func() []float64 {
			if nodes == 1 {
				return []float64{0}
			}
			return []float64{0.05, -0.03}
		}(), LogTau: func() []float64 {
			if nodes == 1 {
				return []float64{0}
			}
			return []float64{0.1, -0.05}
		}()},
		Encoder: func() []float64 {
			if nodes == 1 {
				return []float64{0.7}
			}
			return []float64{0.45, -0.35}
		}(),
		Readout: func() []float64 {
			if nodes == 1 {
				return []float64{1}
			}
			return []float64{0.6, -0.4}
		}(),
	}
	return configOut, parameters, episode
}

func chemicalEpisodeFeedbackSpec(t *testing.T, produced, available int64, score float64) signal.FeedbackSpec {
	t.Helper()
	return signal.FeedbackSpec{
		SchemaVersion: signal.CurrentSchemaVersion(), ExperienceID: "chemical-episode", ActionID: "action",
		ProducedAt:  signal.Timestamp{Value: produced, Unit: signal.TimeUnitModelStep},
		AvailableAt: signal.Timestamp{Value: available, Unit: signal.TimeUnitModelStep},
		Source:      "fixture", Score: score, ModelVersion: signal.CurrentSchemaVersion(),
	}
}

func chemicalEpisodeCopyParameters(p learning.Parameters) learning.Parameters {
	return learning.Parameters{
		Core: dynamics.Parameters{
			Weights: append([]float64(nil), p.Core.Weights...), Bias: append([]float64(nil), p.Core.Bias...), LogTau: append([]float64(nil), p.Core.LogTau...),
		},
		ThetaRaw: append([]float64(nil), p.ThetaRaw...), Encoder: append([]float64(nil), p.Encoder...), Readout: append([]float64(nil), p.Readout...),
	}
}

func chemicalEpisodeCopyRows(rows [][]float64) [][]float64 {
	copyRows := make([][]float64, len(rows))
	for i, row := range rows {
		copyRows[i] = append([]float64(nil), row...)
	}
	return copyRows
}

func chemicalEpisodeClose(got, want float64) bool {
	return math.Abs(got-want) <= 2e-5+2e-5*math.Abs(want)
}

func chemicalEpisodeHill(concentration, kd, n float64) float64 {
	if concentration == 0 {
		return 0
	}
	return 1 / (1 + math.Exp(n*(math.Log(kd)-math.Log(concentration))))
}

func chemicalEpisodeHandRows(inputs, releases []float64, receptorRegion int, transport [][]float64, effectScale float64, expression *learning.ExpressionGain) [][]float64 {
	const dt = 1.0
	const chemicalTau = 2.0
	const coreTau = 1.0
	lambdaChemical := math.Exp(-dt / chemicalTau)
	alphaChemical := 1 - lambdaChemical
	lambdaCore := math.Exp(-dt / coreTau)
	alphaCore := 1 - lambdaCore
	regions := 1
	if transport != nil {
		regions = len(transport)
	}
	concentration := make([]float64, regions)
	voltage := 0.0
	rows := make([][]float64, len(inputs))
	for step, input := range inputs {
		local := make([]float64, regions)
		for region := range local {
			rate := 0.0
			if step < len(releases) {
				rate = releases[step]
			}
			local[region] = lambdaChemical*concentration[region] + chemicalTau*alphaChemical*rate
		}
		if transport != nil {
			next := make([]float64, regions)
			for destination := range next {
				outflow := 0.0
				for target, fraction := range transport[destination] {
					if target != destination {
						outflow += fraction
					}
				}
				next[destination] = local[destination] * (1 - outflow)
				for source := range local {
					if source != destination {
						next[destination] += local[source] * transport[source][destination]
					}
				}
			}
			local = next
		}
		concentration = local
		occupancy := chemicalEpisodeHill(concentration[receptorRegion], 0.5, 1)
		gain := 1 + effectScale*occupancy
		voltage = lambdaCore*voltage + alphaCore*(gain*input)
		raw := math.Tanh(voltage)
		readoutGain := 1.0
		if expression != nil {
			readoutGain = expression.Scale*occupancy + 1
			if readoutGain < expression.Min {
				readoutGain = expression.Min
			}
			if readoutGain > expression.Max {
				readoutGain = expression.Max
			}
		}
		rows[step] = []float64{raw * readoutGain}
	}
	return rows
}

// The source step is deliberately 4 while the episode has no prior neural
// state. The hand table therefore has one release on the first row, followed
// by two zero-release kinetic rows. It is calculated from the declared
// equations above, independently of learning.Network.
func TestChemicalEpisodePredictAllMatchesIndependentHandTable(t *testing.T) {
	expression := &learning.ExpressionGain{Nodes: []int{0}, Receptor: 0, Scale: 1, Min: 0.5, Max: 2}
	episode := chemicalEpisodePulse(4, nil, nil, expression)
	config := chemicalEpisodeScalarConfig(episode, true)
	parameters := chemicalEpisodeScalarParameters()
	network, err := learning.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	input := [][]float64{{1}, {1}, {1}}
	got, err := network.PredictAll(context.Background(), parameters, input)
	if err != nil {
		t.Fatal(err)
	}
	want := chemicalEpisodeHandRows([]float64{1, 1, 1}, []float64{1, 0, 0}, 0, nil, 1, expression)
	if len(got) != len(want) {
		t.Fatalf("PredictAll returned %d rows, want %d", len(got), len(want))
	}
	for row := range want {
		if len(got[row]) != 1 || !chemicalEpisodeClose(got[row][0], want[row][0]) {
			t.Fatalf("row %d = %v, want the independent hand result %v", row, got[row], want[row])
		}
	}
	last, err := network.Predict(context.Background(), parameters, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(last) != 1 || !chemicalEpisodeClose(last[0], want[len(want)-1][0]) {
		t.Fatalf("Predict = %v, want the last hand-table row %v", last, want[len(want)-1])
	}
	plain, err := learning.NewNetwork(chemicalEpisodeScalarConfig(nil, true))
	if err != nil {
		t.Fatal(err)
	}
	plainRows, err := plain.PredictAll(context.Background(), parameters, input)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(plainRows, got) {
		t.Fatalf("a non-nil chemical episode produced the same rows as nil chemistry: %v", got)
	}
}

// This two-node fixture keeps one delayed synapse, nonzero bias and an
// expression gain on only the first readout node. The expected rows are built
// from the declared source, kinetics, Hill occupancy, modulated drive and
// readout equations, so omitting synaptic drive or applying expression before
// the core changes the independently calculated result.
func TestChemicalEpisodeHandSequenceIncludesSynapseBiasAndReadoutExpression(t *testing.T) {
	expression := &learning.ExpressionGain{Nodes: []int{0}, Receptor: 0, Scale: 0.4, Min: 0.5, Max: 2}
	episode := chemicalEpisodePulse(0, nil, nil, expression)
	episode.Part.Config.Regions.NodeRegion = []int{0, 0}
	episode.Part.Config.Receptors.Records[0].Cells = []int{0, 1}
	episode.Part.Config.Effects = []modulation.Effect{{Kind: modulation.EffectSensitivity, Receptor: 0, GammaScale: 0.8, BetaScale: 0.15}}
	config := learning.Config{
		Dynamics: dynamics.Config{
			Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{1}, DT: 1, Activation: "tanh",
		},
		InputSize: 1, OutputSize: 1, ReadoutNodes: []int{0, 1}, InputNodes: []int{0, 1}, ReadoutEveryStep: true, Chemistry: episode,
	}
	parameters := learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{0.45}, Bias: []float64{0.1, -0.2}, LogTau: []float64{0, 0}},
		Encoder: []float64{1, 1}, Readout: []float64{0.7, -0.3},
	}
	network, err := learning.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	input := [][]float64{{1}, {0.5}, {-0.2}}
	got, err := network.PredictAll(context.Background(), parameters, input)
	if err != nil {
		t.Fatal(err)
	}

	chemicalLambda := math.Exp(-0.5)
	chemicalAlpha := 1 - chemicalLambda
	coreLambda := math.Exp(-1)
	coreAlpha := 1 - coreLambda
	concentration, voltage := 0.0, []float64{0, 0}
	rawHistory := make([][]float64, len(input))
	want := make([][]float64, len(input))
	for step, row := range input {
		rate := 0.0
		if step == 0 {
			rate = 1
		}
		concentration = chemicalLambda*concentration + 2*chemicalAlpha*rate
		occupancy := chemicalEpisodeHill(concentration, 0.5, 1)
		gain := 1 + 0.8*occupancy
		offset := 0.15 * occupancy
		pastRaw := 0.0
		if step >= 2 {
			pastRaw = rawHistory[step-2][0]
		}
		current := []float64{row[0], row[0] + 0.45*pastRaw}
		raw := make([]float64, 2)
		for node := range raw {
			drive := gain*current[node] + offset + []float64{0.1, -0.2}[node]
			voltage[node] = coreLambda*voltage[node] + coreAlpha*drive
			raw[node] = math.Tanh(voltage[node])
		}
		readoutGain := 1 + 0.4*occupancy
		want[step] = []float64{0.7*readoutGain*raw[0] - 0.3*raw[1]}
		rawHistory[step] = raw
	}
	if len(got) != len(want) {
		t.Fatalf("PredictAll returned %d rows, want %d", len(got), len(want))
	}
	for row := range want {
		if len(got[row]) != 1 || !chemicalEpisodeClose(got[row][0], want[row][0]) {
			t.Fatalf("row %d = %v, want independent synapse hand row %v", row, got[row], want[row])
		}
	}
}

func TestChemicalEpisodeLocationAndTransportUseTheLocalConcentration(t *testing.T) {
	transport := &modulation.Transport{Fraction: [][]float64{{0, 0.25}, {0.5, 0}}}
	noLocation := chemicalEpisodePulse(0, nil, transport, nil)
	located := chemicalEpisodePulse(0, &modulation.ReceptorLocation{
		Region: 1, Compartment: "postsynaptic", AnatomicalLocation: "fixture-region-1",
		Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "test/location-v1",
	}, transport, nil)
	parameters := chemicalEpisodeScalarParameters()
	input := [][]float64{{1}}
	first, err := learning.NewNetwork(chemicalEpisodeScalarConfig(noLocation, false))
	if err != nil {
		t.Fatal(err)
	}
	second, err := learning.NewNetwork(chemicalEpisodeScalarConfig(located, false))
	if err != nil {
		t.Fatal(err)
	}
	gotNoLocation, err := first.Predict(context.Background(), parameters, input)
	if err != nil {
		t.Fatal(err)
	}
	gotLocated, err := second.Predict(context.Background(), parameters, input)
	if err != nil {
		t.Fatal(err)
	}
	wantNoLocation := chemicalEpisodeHandRows([]float64{1}, []float64{1}, 0, transport.Fraction, 1, nil)[0][0]
	wantLocated := chemicalEpisodeHandRows([]float64{1}, []float64{1}, 1, transport.Fraction, 1, nil)[0][0]
	if len(gotNoLocation) != 1 || !chemicalEpisodeClose(gotNoLocation[0], wantNoLocation) {
		t.Fatalf("nil location output = %v, want region-0 hand value %v", gotNoLocation, wantNoLocation)
	}
	if len(gotLocated) != 1 || !chemicalEpisodeClose(gotLocated[0], wantLocated) {
		t.Fatalf("located output = %v, want region-1 hand value %v", gotLocated, wantLocated)
	}
	if chemicalEpisodeClose(gotNoLocation[0], gotLocated[0]) {
		t.Fatalf("moving the receptor to the transported region did not change output: %v and %v", gotNoLocation, gotLocated)
	}
}

func TestChemicalEpisodeTrainerStepUsesTheSameChemicalPath(t *testing.T) {
	episode := chemicalEpisodePulse(0, nil, nil, nil)
	config := chemicalEpisodeScalarConfig(episode, false)
	parameters := chemicalEpisodeScalarParameters()
	input := [][]float64{{1}, {1}, {1}}
	target := []float64{0.2}
	chemicalNetwork, err := learning.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	plainNetwork, err := learning.NewNetwork(chemicalEpisodeScalarConfig(nil, false))
	if err != nil {
		t.Fatal(err)
	}
	chemicalLoss, chemicalGradient, err := chemicalNetwork.LossGradient(context.Background(), parameters, input, target, 0)
	if err != nil {
		t.Fatal(err)
	}
	plainLoss, _, err := plainNetwork.LossGradient(context.Background(), parameters, input, target, 0)
	if err != nil {
		t.Fatal(err)
	}
	if chemicalEpisodeClose(chemicalLoss, plainLoss) {
		t.Fatalf("chemical and nil-network losses are indistinguishable: %g and %g", chemicalLoss, plainLoss)
	}
	if math.Abs(chemicalGradient.Encoder[0])+math.Abs(chemicalGradient.Readout[0]) == 0 {
		t.Fatalf("chemical gradient has no encoder/readout path: %+v", chemicalGradient)
	}
	chemicalTrainer, err := learning.NewTrainer(config, parameters, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	plainTrainer, err := learning.NewTrainer(chemicalEpisodeScalarConfig(nil, false), parameters, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chemicalTrainer.Step(context.Background(), input, target); err != nil {
		t.Fatal(err)
	}
	if _, err := plainTrainer.Step(context.Background(), input, target); err != nil {
		t.Fatal(err)
	}
	chemicalSnapshot := chemicalTrainer.Snapshot()
	plainSnapshot := plainTrainer.Snapshot()
	if reflect.DeepEqual(chemicalSnapshot.Parameters, plainSnapshot.Parameters) {
		t.Fatalf("Trainer.Step ignored chemistry and committed the nil-path parameters: %+v", chemicalSnapshot.Parameters)
	}
}

func chemicalEpisodeSoftplus(v float64) float64 {
	if v > 30 {
		return v
	}
	return math.Log1p(math.Exp(v))
}

// This reference intentionally feeds the previous raw tanh output back into
// the controller. The expression gain is applied only after that raw value is
// produced, so replacing raw with the already-scaled readout changes this
// table on the second row.
func chemicalEpisodeControllerExpressionHandRows(inputs []float64, initial float64, expression *learning.ExpressionGain) [][]float64 {
	chemicalLambda := math.Exp(-0.5)
	chemicalAlpha := 1 - chemicalLambda
	coreLambda := math.Exp(-0.7)
	coreAlpha := 1 - coreLambda
	window := []float64{0, 0, 0}
	concentration, voltage, previousRaw := 0.0, 0.0, initial
	rows := make([][]float64, len(inputs))
	for step, input := range inputs {
		copy(window, window[1:])
		window[len(window)-1] = previousRaw
		mean := (window[0] + window[1] + window[2]) / 3
		variance := (window[0]*window[0]+window[1]*window[1]+window[2]*window[2])/3 - mean*mean
		if variance < 0 {
			variance = 0
		}
		hidden := math.Tanh(0.6*mean + 0.2*variance + 0.05)
		rate := chemicalEpisodeSoftplus(0.8*hidden - 0.1)
		concentration = chemicalLambda*concentration + 2*chemicalAlpha*rate
		occupancy := chemicalEpisodeHill(concentration, 0.5, 1)
		gain := 1 + 0.8*occupancy
		offset := 0.1 * occupancy
		voltage = coreLambda*voltage + coreAlpha*(gain*input+offset)
		raw := math.Tanh(voltage)
		readoutGain := 1.0
		if expression != nil {
			readoutGain = expression.Scale*occupancy + 1
			if readoutGain < expression.Min {
				readoutGain = expression.Min
			}
			if readoutGain > expression.Max {
				readoutGain = expression.Max
			}
		}
		rows[step] = []float64{raw * readoutGain}
		previousRaw = raw
	}
	return rows
}

func TestChemicalEpisodeControllerUsesRawActivityBeforeExpression(t *testing.T) {
	expression := &learning.ExpressionGain{Nodes: []int{0}, Receptor: 0, Scale: 0.7, Min: 0.5, Max: 2}
	config, parameters, episode := chemicalEpisodeControllerConfig(1, false, expression)
	network, err := learning.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	input := [][]float64{{0.7}, {0.7}, {0.7}}
	got, err := network.PredictAll(context.Background(), parameters, input)
	if err != nil {
		t.Fatal(err)
	}
	want := chemicalEpisodeControllerExpressionHandRows([]float64{0.49, 0.49, 0.49}, episode.InitialActivity[0], expression)
	for row := range want {
		if len(got[row]) != 1 || !chemicalEpisodeClose(got[row][0], want[row][0]) {
			t.Fatalf("row %d = %v, want raw-activity hand row %v", row, got[row], want[row])
		}
	}
	withoutExpressionConfig, withoutExpressionParameters, _ := chemicalEpisodeControllerConfig(1, false, nil)
	withoutExpression, err := learning.NewNetwork(withoutExpressionConfig)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := withoutExpression.PredictAll(context.Background(), withoutExpressionParameters, input)
	if err != nil {
		t.Fatal(err)
	}
	if chemicalEpisodeClose(raw[1][0], got[1][0]) {
		t.Fatalf("expression gain did not affect the readout: raw=%v expression=%v", raw, got)
	}
	plusConfig, plusParameters, plusEpisode := chemicalEpisodeControllerConfig(1, false, expression)
	minusConfig, minusParameters, minusEpisode := chemicalEpisodeControllerConfig(1, false, expression)
	plusEpisode.InitialActivity[0] += chemicalEpisodeEpsilon
	minusEpisode.InitialActivity[0] -= chemicalEpisodeEpsilon
	plusNetwork, err := learning.NewNetwork(plusConfig)
	if err != nil {
		t.Fatal(err)
	}
	minusNetwork, err := learning.NewNetwork(minusConfig)
	if err != nil {
		t.Fatal(err)
	}
	plusOutput, err := plusNetwork.Predict(context.Background(), plusParameters, input[:1])
	if err != nil {
		t.Fatal(err)
	}
	minusOutput, err := minusNetwork.Predict(context.Background(), minusParameters, input[:1])
	if err != nil {
		t.Fatal(err)
	}
	initialActivityDerivative := (plusOutput[0] - minusOutput[0]) / (2 * chemicalEpisodeEpsilon)
	if math.Abs(initialActivityDerivative) < 1e-3 {
		t.Fatalf("expression/source path is insensitive to InitialActivity, finite difference=%g", initialActivityDerivative)
	}
}

func TestChemicalEpisodeExpressionFiniteDifferenceAndClamp(t *testing.T) {
	active := &learning.ExpressionGain{Nodes: []int{0}, Receptor: 0, Scale: 1, Min: 0.5, Max: 2}
	clamped := &learning.ExpressionGain{Nodes: []int{0}, Receptor: 0, Scale: 1, Min: 1, Max: 1}
	base := chemicalEpisodeReceptorOnly(0.8, active)
	baseConfig := chemicalEpisodeScalarConfig(base, false)
	parameters := chemicalEpisodeScalarParameters()
	input := [][]float64{{0.8}}
	baseNetwork, err := learning.NewNetwork(baseConfig)
	if err != nil {
		t.Fatal(err)
	}
	baseOutput, err := baseNetwork.Predict(context.Background(), parameters, input)
	if err != nil {
		t.Fatal(err)
	}
	plus := chemicalEpisodeReceptorOnly(0.8+chemicalEpisodeEpsilon, active)
	minus := chemicalEpisodeReceptorOnly(0.8-chemicalEpisodeEpsilon, active)
	plusNetwork, err := learning.NewNetwork(chemicalEpisodeScalarConfig(plus, false))
	if err != nil {
		t.Fatal(err)
	}
	minusNetwork, err := learning.NewNetwork(chemicalEpisodeScalarConfig(minus, false))
	if err != nil {
		t.Fatal(err)
	}
	plusOutput, err := plusNetwork.Predict(context.Background(), parameters, input)
	if err != nil {
		t.Fatal(err)
	}
	minusOutput, err := minusNetwork.Predict(context.Background(), parameters, input)
	if err != nil {
		t.Fatal(err)
	}
	derivative := (plusOutput[0] - minusOutput[0]) / (2 * chemicalEpisodeEpsilon)
	if math.Abs(derivative) < 1e-3 {
		t.Fatalf("expression output is insensitive to receptor concentration, finite difference=%g base=%v", derivative, baseOutput)
	}
	clampedPlus, err := learning.NewNetwork(chemicalEpisodeScalarConfig(chemicalEpisodeReceptorOnly(0.8+chemicalEpisodeEpsilon, clamped), false))
	if err != nil {
		t.Fatal(err)
	}
	clampedMinus, err := learning.NewNetwork(chemicalEpisodeScalarConfig(chemicalEpisodeReceptorOnly(0.8-chemicalEpisodeEpsilon, clamped), false))
	if err != nil {
		t.Fatal(err)
	}
	clampedPlusOutput, err := clampedPlus.Predict(context.Background(), parameters, input)
	if err != nil {
		t.Fatal(err)
	}
	clampedMinusOutput, err := clampedMinus.Predict(context.Background(), parameters, input)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs((clampedPlusOutput[0]-clampedMinusOutput[0])/(2*chemicalEpisodeEpsilon)) > 2e-4 {
		t.Fatalf("clamped expression still has a concentration derivative: plus=%v minus=%v", clampedPlusOutput, clampedMinusOutput)
	}
}

func chemicalEpisodeSquaredError(network *learning.Network, parameters learning.Parameters, input [][]float64, target float64) (float64, error) {
	output, err := network.Predict(context.Background(), parameters, input)
	if err != nil {
		return 0, err
	}
	delta := output[0] - target
	return delta * delta, nil
}

func TestChemicalEpisodeControllerGradientMatchesFullPathFiniteDifference(t *testing.T) {
	expression := &learning.ExpressionGain{Nodes: []int{0}, Receptor: 0, Scale: 0.7, Min: 0.5, Max: 2}
	config, parameters, _ := chemicalEpisodeControllerConfig(2, false, expression)
	network, err := learning.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	input := [][]float64{{0.2}, {0.4}, {-0.3}, {0.5}, {0.1}}
	target := 0.17
	chemicalRows, err := network.PredictAll(context.Background(), parameters, input)
	if err != nil {
		t.Fatal(err)
	}
	plainConfig := config
	plainConfig.Chemistry = nil
	plainNetwork, err := learning.NewNetwork(plainConfig)
	if err != nil {
		t.Fatal(err)
	}
	plainRows, err := plainNetwork.PredictAll(context.Background(), parameters, input)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(chemicalRows, plainRows) {
		t.Fatalf("controller chemistry did not change the recurrent full-trace outputs: %v", chemicalRows)
	}
	_, gradient, err := network.LossGradient(context.Background(), parameters, input, []float64{target}, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := range parameters.Core.Weights {
		index := i
		plusParameters := chemicalEpisodeCopyParameters(parameters)
		minusParameters := chemicalEpisodeCopyParameters(parameters)
		plusParameters.Core.Weights[index] += chemicalEpisodeEpsilon
		minusParameters.Core.Weights[index] -= chemicalEpisodeEpsilon
		plusLoss, err := chemicalEpisodeSquaredError(network, plusParameters, input, target)
		if err != nil {
			t.Fatal(err)
		}
		minusLoss, err := chemicalEpisodeSquaredError(network, minusParameters, input, target)
		if err != nil {
			t.Fatal(err)
		}
		fd := (plusLoss - minusLoss) / (2 * chemicalEpisodeEpsilon)
		if math.Abs(fd-gradient.Core.Weights[index]) > 1e-4+0.003*math.Abs(fd) {
			t.Fatalf("core weight[%d] gradient=%g finite_difference=%g", index, gradient.Core.Weights[index], fd)
		}
	}
	for i := range parameters.Core.Bias {
		index := i
		plus := chemicalEpisodeCopyParameters(parameters)
		minus := chemicalEpisodeCopyParameters(parameters)
		plus.Core.Bias[index] += chemicalEpisodeEpsilon
		minus.Core.Bias[index] -= chemicalEpisodeEpsilon
		plusLoss, err := chemicalEpisodeSquaredError(network, plus, input, target)
		if err != nil {
			t.Fatal(err)
		}
		minusLoss, err := chemicalEpisodeSquaredError(network, minus, input, target)
		if err != nil {
			t.Fatal(err)
		}
		fd := (plusLoss - minusLoss) / (2 * chemicalEpisodeEpsilon)
		if math.Abs(fd-gradient.Core.Bias[index]) > 1e-4+0.003*math.Abs(fd) || math.Abs(fd) < 1e-4 {
			t.Fatalf("core bias[%d] gradient=%g finite_difference=%g", index, gradient.Core.Bias[index], fd)
		}
	}
	for i := range parameters.Core.LogTau {
		index := i
		plus := chemicalEpisodeCopyParameters(parameters)
		minus := chemicalEpisodeCopyParameters(parameters)
		plus.Core.LogTau[index] += chemicalEpisodeEpsilon
		minus.Core.LogTau[index] -= chemicalEpisodeEpsilon
		plusLoss, err := chemicalEpisodeSquaredError(network, plus, input, target)
		if err != nil {
			t.Fatal(err)
		}
		minusLoss, err := chemicalEpisodeSquaredError(network, minus, input, target)
		if err != nil {
			t.Fatal(err)
		}
		fd := (plusLoss - minusLoss) / (2 * chemicalEpisodeEpsilon)
		if math.Abs(fd-gradient.Core.LogTau[index]) > 1e-4+0.003*math.Abs(fd) || math.Abs(fd) < 1e-4 {
			t.Fatalf("core log_tau[%d] gradient=%g finite_difference=%g", index, gradient.Core.LogTau[index], fd)
		}
	}
	for i := range parameters.Encoder {
		index := i
		plus := chemicalEpisodeCopyParameters(parameters)
		minus := chemicalEpisodeCopyParameters(parameters)
		plus.Encoder[index] += chemicalEpisodeEpsilon
		minus.Encoder[index] -= chemicalEpisodeEpsilon
		plusLoss, err := chemicalEpisodeSquaredError(network, plus, input, target)
		if err != nil {
			t.Fatal(err)
		}
		minusLoss, err := chemicalEpisodeSquaredError(network, minus, input, target)
		if err != nil {
			t.Fatal(err)
		}
		fd := (plusLoss - minusLoss) / (2 * chemicalEpisodeEpsilon)
		if math.Abs(fd-gradient.Encoder[index]) > 1e-4+0.003*math.Abs(fd) || math.Abs(fd) < 1e-4 {
			t.Fatalf("encoder[%d] gradient=%g finite_difference=%g", index, gradient.Encoder[index], fd)
		}
	}
	for i := range parameters.Readout {
		index := i
		plus := chemicalEpisodeCopyParameters(parameters)
		minus := chemicalEpisodeCopyParameters(parameters)
		plus.Readout[index] += chemicalEpisodeEpsilon
		minus.Readout[index] -= chemicalEpisodeEpsilon
		plusLoss, err := chemicalEpisodeSquaredError(network, plus, input, target)
		if err != nil {
			t.Fatal(err)
		}
		minusLoss, err := chemicalEpisodeSquaredError(network, minus, input, target)
		if err != nil {
			t.Fatal(err)
		}
		fd := (plusLoss - minusLoss) / (2 * chemicalEpisodeEpsilon)
		if math.Abs(fd-gradient.Readout[index]) > 1e-4+0.003*math.Abs(fd) || math.Abs(fd) < 1e-4 {
			t.Fatalf("readout[%d] gradient=%g finite_difference=%g", index, gradient.Readout[index], fd)
		}
	}
	for step := range input {
		row := step
		plus := chemicalEpisodeCopyRows(input)
		minus := chemicalEpisodeCopyRows(input)
		plus[row][0] += chemicalEpisodeEpsilon
		minus[row][0] -= chemicalEpisodeEpsilon
		plusLoss, err := chemicalEpisodeSquaredError(network, parameters, plus, target)
		if err != nil {
			t.Fatal(err)
		}
		minusLoss, err := chemicalEpisodeSquaredError(network, parameters, minus, target)
		if err != nil {
			t.Fatal(err)
		}
		fd := (plusLoss - minusLoss) / (2 * chemicalEpisodeEpsilon)
		if math.Abs(fd-gradient.Inputs[row][0]) > 1e-4+0.003*math.Abs(fd) || math.Abs(fd) < 1e-4 {
			t.Fatalf("input[%d] gradient=%g finite_difference=%g", row, gradient.Inputs[row][0], fd)
		}
	}
}

func TestChemicalEpisodeNeuralActivityFeedbackGradient(t *testing.T) {
	expression := &learning.ExpressionGain{Nodes: []int{0}, Receptor: 0, Scale: 0.7, Min: 0.5, Max: 2}
	config, parameters, episode := chemicalEpisodeControllerConfig(2, false, expression)
	episode.Part.Config.Sources = []modulation.SourceSpec{{Kind: modulation.SourceNeuralActivity, Channel: 0,
		Neural: &modulation.NeuralActivity{Nodes: []int{0}, Gain: 1.3, Channel: 0}}}
	input := [][]float64{{0.2}, {0.4}, {0.3}, {0.5}, {0.1}}
	network, err := learning.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	plainConfig := config
	plainConfig.Chemistry = nil
	plain, err := learning.NewNetwork(plainConfig)
	if err != nil {
		t.Fatal(err)
	}
	got, err := network.PredictAll(context.Background(), parameters, input)
	if err != nil {
		t.Fatal(err)
	}
	without, err := plain.PredictAll(context.Background(), parameters, input)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(got, without) {
		t.Fatal("neural activity source was ignored")
	}
	_, gradient, err := network.LossGradient(context.Background(), parameters, input, []float64{0.17}, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range []string{"bias", "weight", "encoder", "input"} {
		plus, minus := chemicalEpisodeCopyParameters(parameters), chemicalEpisodeCopyParameters(parameters)
		plusInput, minusInput := chemicalEpisodeCopyRows(input), chemicalEpisodeCopyRows(input)
		var analytical float64
		switch group {
		case "bias":
			plus.Core.Bias[0] += chemicalEpisodeEpsilon
			minus.Core.Bias[0] -= chemicalEpisodeEpsilon
			analytical = gradient.Core.Bias[0]
		case "weight":
			plus.Core.Weights[0] += chemicalEpisodeEpsilon
			minus.Core.Weights[0] -= chemicalEpisodeEpsilon
			analytical = gradient.Core.Weights[0]
		case "encoder":
			plus.Encoder[0] += chemicalEpisodeEpsilon
			minus.Encoder[0] -= chemicalEpisodeEpsilon
			analytical = gradient.Encoder[0]
		case "input":
			plusInput[0][0] += chemicalEpisodeEpsilon
			minusInput[0][0] -= chemicalEpisodeEpsilon
			analytical = gradient.Inputs[0][0]
		}
		a, err := chemicalEpisodeSquaredError(network, plus, plusInput, 0.17)
		if err != nil {
			t.Fatal(err)
		}
		b, err := chemicalEpisodeSquaredError(network, minus, minusInput, 0.17)
		if err != nil {
			t.Fatal(err)
		}
		fd := (a - b) / (2 * chemicalEpisodeEpsilon)
		if math.Abs(fd-analytical) > 1e-4+0.003*math.Abs(fd) {
			t.Fatalf("%s gradient=%g finite_difference=%g", group, analytical, fd)
		}
	}
}

func TestChemicalEpisodeTimelineClockFeedbackAndInitialActivity(t *testing.T) {
	input := [][]float64{{0.6}, {0.6}}
	noFeedbackConfig, noFeedbackParameters, _ := chemicalEpisodeControllerConfig(1, true, nil)
	noFeedbackNetwork, err := learning.NewNetwork(noFeedbackConfig)
	if err != nil {
		t.Fatal(err)
	}
	noFeedback, err := noFeedbackNetwork.PredictAll(context.Background(), noFeedbackParameters, input)
	if err != nil {
		t.Fatal(err)
	}
	lateConfig, lateParameters, lateEpisode := chemicalEpisodeControllerConfig(1, true, nil)
	lateEpisode.Part.PendingFeedback = []signal.FeedbackSpec{chemicalEpisodeFeedbackSpec(t, 7, 8, 3)}
	lateNetwork, err := learning.NewNetwork(lateConfig)
	if err != nil {
		t.Fatal(err)
	}
	late, err := lateNetwork.PredictAll(context.Background(), lateParameters, input)
	if err != nil {
		t.Fatal(err)
	}
	if !chemicalEpisodeClose(late[0][0], noFeedback[0][0]) {
		t.Fatalf("feedback available at step 8 changed the step-7 release: late=%v no_feedback=%v", late, noFeedback)
	}
	if chemicalEpisodeClose(late[1][0], noFeedback[1][0]) {
		t.Fatalf("feedback available at step 8 never reached the step-8 controller: late=%v no_feedback=%v", late, noFeedback)
	}
	stateClockConfig, stateClockParameters, stateClockEpisode := chemicalEpisodeControllerConfig(1, true, nil)
	stateClockEpisode.Part.PendingFeedback = []signal.FeedbackSpec{chemicalEpisodeFeedbackSpec(t, 7, 8, 3)}
	stateClockEpisode.Part.State.Steps = 99
	stateClockNetwork, err := learning.NewNetwork(stateClockConfig)
	if err != nil {
		t.Fatal(err)
	}
	stateClock, err := stateClockNetwork.PredictAll(context.Background(), stateClockParameters, input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(late, stateClock) {
		t.Fatalf("changing the kinetics step count changed the source clock: late=%v state_clock=%v", late, stateClock)
	}
	shiftedConfig, shiftedParameters, shiftedEpisode := chemicalEpisodeControllerConfig(1, true, nil)
	shiftedEpisode.Part.PendingFeedback = []signal.FeedbackSpec{chemicalEpisodeFeedbackSpec(t, 7, 8, 3)}
	shiftedEpisode.SourceStep = 8
	shiftedNetwork, err := learning.NewNetwork(shiftedConfig)
	if err != nil {
		t.Fatal(err)
	}
	shifted, err := shiftedNetwork.PredictAll(context.Background(), shiftedParameters, input)
	if err != nil {
		t.Fatal(err)
	}
	if chemicalEpisodeClose(shifted[0][0], late[0][0]) {
		t.Fatalf("changing SourceStep did not move the feedback boundary: shifted=%v late=%v", shifted, late)
	}
	initialConfig, initialParameters, initialEpisode := chemicalEpisodeControllerConfig(1, false, nil)
	initialEpisode.InitialActivity[0] = 0.8
	initialNetwork, err := learning.NewNetwork(initialConfig)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := initialNetwork.Predict(context.Background(), initialParameters, [][]float64{{0.6}})
	if err != nil {
		t.Fatal(err)
	}
	defaultConfig, defaultParameters, _ := chemicalEpisodeControllerConfig(1, false, nil)
	defaultNetwork, err := learning.NewNetwork(defaultConfig)
	if err != nil {
		t.Fatal(err)
	}
	defaultOutput, err := defaultNetwork.Predict(context.Background(), defaultParameters, [][]float64{{0.6}})
	if err != nil {
		t.Fatal(err)
	}
	if chemicalEpisodeClose(initial[0], defaultOutput[0]) {
		t.Fatalf("supplied InitialActivity did not affect the first controller release: initial=%v default=%v", initial, defaultOutput)
	}
}

func TestChemicalEpisodeLIFThresholdEffectUsesTheChemicalPath(t *testing.T) {
	lif := dynamics.LIFConfig{
		Nodes: 1, Sources: []int{}, Targets: []int{}, Delays: []int{}, DT: 1, TauSyn: 1,
		ThetaMin: 0.1, ThetaMax: 2, VReset: -0.5, RefractorySteps: 0,
		Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
	}
	episode := chemicalEpisodePulse(0, nil, nil, nil)
	episode.Part.Config.Effects = []modulation.Effect{{Kind: modulation.EffectThreshold, Receptor: 0, ThetaScale: 0.5, ThetaAbsMax: 1}}
	config := learning.Config{LIF: &lif, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{0}, InputNodes: []int{0}, ReadoutEveryStep: true, Chemistry: episode}
	parameters := learning.Parameters{
		Core:     dynamics.Parameters{Weights: []float64{}, Bias: []float64{0}, LogTau: []float64{0}},
		ThetaRaw: []float64{0}, Encoder: []float64{2}, Readout: []float64{1},
	}
	network, err := learning.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	got, err := network.PredictAll(context.Background(), parameters, [][]float64{{1}, {1}})
	if err != nil {
		t.Fatal(err)
	}
	// The first row's occupancy is 0.611481..., moving the base threshold
	// 1.05 to 1.3557 and suppressing the event. The silent membrane then
	// carries into row 2, whose candidate clears the lower decayed threshold,
	// so its synaptic readout is exactly one event.
	want := [][]float64{{0}, {1}}
	if len(got) != len(want) {
		t.Fatalf("PredictAll returned %d rows, want %d", len(got), len(want))
	}
	for row := range want {
		if len(got[row]) != 1 || math.Abs(got[row][0]-want[row][0]) > 1e-6 {
			t.Fatalf("LIF row %d = %v, want the surrogate hand path %v", row, got[row], want[row])
		}
	}
}

func TestChemicalEpisodeRejectsInvalidBoundariesAndIndividualConflict(t *testing.T) {
	valid := chemicalEpisodePulse(0, nil, nil, nil)
	cases := []struct {
		name   string
		mutate func(*learning.ChemicalEpisode)
	}{
		{name: "negative concentration", mutate: func(e *learning.ChemicalEpisode) {
			e.Part.State.Concentration[0][0] = -1
		}},
		{name: "nonfinite concentration", mutate: func(e *learning.ChemicalEpisode) {
			e.Part.State.Concentration[0][0] = math.NaN()
		}},
		{name: "state shape", mutate: func(e *learning.ChemicalEpisode) {
			e.Part.State.Concentration = [][]float64{{0, 0}}
		}},
		{name: "source count", mutate: func(e *learning.ChemicalEpisode) {
			e.Part.Config.Sources = nil
		}},
		{name: "initial activity width", mutate: func(e *learning.ChemicalEpisode) {
			e.InitialActivity = []float64{0, 0}
		}},
		{name: "nonfinite initial activity", mutate: func(e *learning.ChemicalEpisode) {
			e.InitialActivity = []float64{math.NaN()}
		}},
		{name: "source clock overflow", mutate: func(e *learning.ChemicalEpisode) {
			e.SourceStep = uint64(math.MaxInt64) + 1
		}},
		{name: "expression node", mutate: func(e *learning.ChemicalEpisode) {
			e.Part.Expression = &learning.ExpressionGain{Nodes: []int{1}, Receptor: 0, Scale: 1, Min: 0.5, Max: 2}
		}},
		{name: "nonfinite resource", mutate: func(e *learning.ChemicalEpisode) {
			e.Part.Resources = map[string]float64{"energy": math.Inf(1)}
		}},
		{name: "nonfinite feedback", mutate: func(e *learning.ChemicalEpisode) {
			feedback := chemicalEpisodeFeedbackSpec(t, 0, 0, 0.5)
			feedback.Score = math.NaN()
			e.Part.PendingFeedback = []signal.FeedbackSpec{feedback}
		}},
		{name: "controller core node mismatch", mutate: func(e *learning.ChemicalEpisode) {
			e.Part.Config.Sources = []modulation.SourceSpec{{
				Kind: modulation.SourceController, Channel: 0,
				Controller: &modulation.Controller{
					Inputs: modulation.ControllerInputs{SummaryWindow: 3}, Nodes: 2, Hidden: 1, Channel: 0,
					Parameters: []float64{0, 0, 0, 1, 0},
				},
			}}
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			episode := chemicalEpisodePulse(0, nil, nil, nil)
			testCase.mutate(episode)
			if _, err := learning.NewNetwork(chemicalEpisodeScalarConfig(episode, false)); err == nil {
				t.Fatalf("accepted invalid ChemicalEpisode boundary %q", testCase.name)
			}
		})
	}
	vectorEpisode := chemicalEpisodePulse(0, nil, nil, nil)
	vectorCore := dynamics.Config{Nodes: 1, Sources: []int{}, Targets: []int{}, Delays: []int{}, DT: 1, Activation: "tanh", StateDimension: 2}
	vectorConfig := learning.Config{Dynamics: vectorCore, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{0}, InputNodes: []int{0}, Chemistry: vectorEpisode}
	if _, err := learning.NewNetwork(vectorConfig); err == nil {
		t.Fatal("accepted vector core with a scalar ChemicalEpisode path")
	}
	if _, err := learning.NewGPUTrainer(context.Background(), vectorConfig, learning.Parameters{}, learning.DefaultOptions()); err == nil {
		t.Fatal("accepted vector/GPU ChemicalEpisode construction before computation")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, gpuErr := learning.NewGPUTrainer(ctx, chemicalEpisodeScalarConfig(valid, false), chemicalEpisodeScalarParameters(), learning.DefaultOptions())
	if !errors.Is(gpuErr, learning.ErrGPUTrainerUnsupported) || !strings.Contains(strings.ToLower(gpuErr.Error()), "chem") {
		t.Fatalf("GPU chemistry must be refused before adapter execution, got %v", gpuErr)
	}
	individualConfig := chemicalEpisodeScalarConfig(valid, false)
	if _, err := learning.NewIndividual(individualConfig, chemicalEpisodeScalarParameters(), learning.DefaultOptions(), []float64{0}); err == nil {
		t.Fatal("NewIndividual accepted a separate ChemicalEpisode entry point")
	}
	plainConfig := chemicalEpisodeScalarConfig(nil, false)
	plain, err := learning.NewIndividual(plainConfig, chemicalEpisodeScalarParameters(), learning.DefaultOptions(), []float64{0})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := plain.Snapshot()
	snapshot.Config.Chemistry = valid
	if _, err := learning.RestoreIndividual(snapshot); err == nil || !strings.Contains(strings.ToLower(err.Error()), "chem") {
		t.Fatalf("RestoreIndividual did not explicitly reject ChemicalEpisode coexistence: %v", err)
	}
}

func TestChemicalEpisodeOwnsBoundariesAndRunsPrivateEpisodes(t *testing.T) {
	expression := &learning.ExpressionGain{Nodes: []int{0}, Receptor: 0, Scale: 1, Min: 0.5, Max: 2}
	episode := chemicalEpisodePulse(0, nil, nil, expression)
	config := chemicalEpisodeScalarConfig(episode, true)
	parameters := chemicalEpisodeScalarParameters()
	network, err := learning.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	input := [][]float64{{1}, {1}}
	inputBefore := chemicalEpisodeCopyRows(input)
	configBefore := network.Config()
	episode.Part.Config.Sources[0].Timeline.Entries[0].Rate = 99
	episode.Part.State.Concentration[0][0] = 99
	episode.Part.Expression.Nodes[0] = 99
	owned := network.Config()
	if owned.Chemistry == nil || owned.Chemistry.Part.Config.Sources[0].Timeline.Entries[0].Rate != 1 || owned.Chemistry.Part.State.Concentration[0][0] != 0 || owned.Chemistry.Part.Expression.Nodes[0] != 0 {
		t.Fatalf("NewNetwork did not deep-copy the chemical boundary: %+v", owned.Chemistry)
	}
	owned.Chemistry.Part.Config.Sources[0].Timeline.Entries[0].Rate = 77
	owned.Chemistry.Part.State.Concentration[0][0] = 77
	owned.Chemistry.Part.Expression.Nodes[0] = 77
	again := network.Config()
	if again.Chemistry.Part.Config.Sources[0].Timeline.Entries[0].Rate != 1 || again.Chemistry.Part.State.Concentration[0][0] != 0 || again.Chemistry.Part.Expression.Nodes[0] != 0 {
		t.Fatalf("Network.Config returned aliased chemical buffers: %+v", again.Chemistry)
	}
	if !reflect.DeepEqual(input, inputBefore) {
		t.Fatalf("Predict input was modified before execution: got %v want %v", input, inputBefore)
	}
	first, err := network.PredictAll(context.Background(), parameters, input)
	if err != nil {
		t.Fatal(err)
	}
	const episodes = 6
	results := make(chan [][]float64, episodes)
	var group sync.WaitGroup
	for range episodes {
		group.Add(1)
		go func() {
			defer group.Done()
			rows, err := network.PredictAll(context.Background(), parameters, input)
			if err != nil {
				results <- [][]float64{{math.NaN()}}
				return
			}
			results <- rows
		}()
	}
	group.Wait()
	close(results)
	for rows := range results {
		if len(rows) != len(first) || !chemicalEpisodeClose(rows[0][0], first[0][0]) || !chemicalEpisodeClose(rows[1][0], first[1][0]) {
			t.Fatalf("independent concurrent episodes diverged: got %v want %v", rows, first)
		}
	}
	if !reflect.DeepEqual(network.Config(), configBefore) {
		t.Fatal("PredictAll changed the immutable network chemical boundary")
	}
	trainer, err := learning.NewTrainer(configBefore, parameters, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	before := trainer.Snapshot()
	if _, err := trainer.Step(context.Background(), input, []float64{0.1}); err != nil {
		t.Fatal(err)
	}
	after := trainer.Snapshot()
	if reflect.DeepEqual(before.Parameters, after.Parameters) {
		t.Fatal("a successful chemical Trainer.Step did not update parameters")
	}
	if !reflect.DeepEqual(before.Config.Chemistry, after.Config.Chemistry) {
		t.Fatalf("Trainer.Step changed the episode start boundary: before=%+v after=%+v", before.Config.Chemistry, after.Config.Chemistry)
	}
	failedBefore := trainer.Snapshot()
	if _, err := trainer.Step(context.Background(), [][]float64{{math.NaN()}}, []float64{0.1}); err == nil {
		t.Fatal("accepted a non-finite input in a chemical episode")
	}
	if !reflect.DeepEqual(failedBefore, trainer.Snapshot()) {
		t.Fatal("a rejected chemical step changed parameters, optimizer or episode boundary")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := trainer.Step(ctx, input, []float64{0.1}); err == nil {
		t.Fatal("accepted a canceled chemical step")
	}
	if !reflect.DeepEqual(failedBefore, trainer.Snapshot()) {
		t.Fatal("a canceled chemical step changed parameters, optimizer or episode boundary")
	}
	lateConfig, lateParameters, lateEpisode := chemicalEpisodeControllerConfig(1, false, nil)
	lateEpisode.InitialActivity = nil
	lateEpisode.Part.Config.Sources[0].Controller.Parameters = []float64{
		math.MaxFloat64, 0, 0, math.MaxFloat64, math.MaxFloat64,
	}
	lateTrainer, err := learning.NewTrainer(lateConfig, lateParameters, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	lateBefore := lateTrainer.Snapshot()
	if _, err := lateTrainer.Step(context.Background(), [][]float64{{1}, {1}}, []float64{0.1}); err == nil {
		t.Fatal("accepted a late non-finite controller source release")
	}
	if !reflect.DeepEqual(lateBefore, lateTrainer.Snapshot()) {
		t.Fatal("a late controller source failure changed parameters, optimizer or episode boundary")
	}
	plain, err := learning.NewNetwork(chemicalEpisodeScalarConfig(nil, false))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plain.Predict(context.Background(), parameters, [][]float64{{1}}); err != nil {
		t.Fatalf("nil chemistry did not preserve the legacy network path: %v", err)
	}
}
