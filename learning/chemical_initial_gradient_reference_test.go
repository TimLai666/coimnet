package learning_test

import (
	"context"
	"math"
	"strconv"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
)

const chemicalInitialReferenceTolerance = 1e-5

// TestChemicalInitialGradientReference keeps the first source row's implicit
// initial activity on the same numerical boundary as the core.  The mixed
// case uses the frozen two-node reference; the scalar cases below use a
// separate hand-derived softplus reference so the source and initial-state
// path are independently checked.
func TestChemicalInitialGradientReference(t *testing.T) {
	t.Run("mixed_controller_nil_initial_full_history", func(t *testing.T) {
		fixture := mixedReferenceFixture("controller", "normal")
		fixture.name = "controller-nil-initial"
		fixture.config.Chemistry.InitialActivity = nil
		runMixedReference(t, fixture)
	})

	for _, source := range []string{"controller", "neural"} {
		for _, window := range []int{0, 1} {
			source, window := source, window
			t.Run(source+"_softplus_window_"+chemicalInitialReferenceInt(window), func(t *testing.T) {
				runSoftplusInitialReference(t, source, window)
			})
		}
	}
}

type chemicalInitialReferenceFixture struct {
	config     learning.Config
	parameters learning.Parameters
	input      [][]float64
	upstream   [][]float64
	a0         float64
}

func runSoftplusInitialReference(t *testing.T, source string, window int) {
	t.Helper()
	nilFixture := chemicalInitialReferenceFixtureFor(source, false)
	explicitFixture := chemicalInitialReferenceFixtureFor(source, true)
	nilNetwork, err := learning.NewNetwork(nilFixture.config)
	if err != nil {
		t.Fatalf("nil InitialActivity NewNetwork: %v", err)
	}
	explicitNetwork, err := learning.NewNetwork(explicitFixture.config)
	if err != nil {
		t.Fatalf("explicit InitialActivity NewNetwork: %v", err)
	}
	nilGradient, err := nilNetwork.LossGradientFrom(context.Background(), nilFixture.parameters, nilFixture.input, nilFixture.upstream, window)
	if err != nil {
		t.Fatalf("nil InitialActivity LossGradientFrom window=%d: %v", window, err)
	}
	explicitGradient, err := explicitNetwork.LossGradientFrom(context.Background(), explicitFixture.parameters, explicitFixture.input, explicitFixture.upstream, window)
	if err != nil {
		t.Fatalf("explicit InitialActivity LossGradientFrom window=%d: %v", window, err)
	}
	nilPrediction, err := nilNetwork.PredictAll(context.Background(), nilFixture.parameters, nilFixture.input)
	if err != nil {
		t.Fatalf("nil InitialActivity PredictAll: %v", err)
	}
	explicitPrediction, err := explicitNetwork.PredictAll(context.Background(), explicitFixture.parameters, explicitFixture.input)
	if err != nil {
		t.Fatalf("explicit InitialActivity PredictAll: %v", err)
	}

	assertChemicalInitialGradientShape(t, "nil", nilGradient)
	assertChemicalInitialGradientShape(t, "explicit", explicitGradient)
	assertChemicalInitialPredictionShape(t, "nil", nilPrediction)
	assertChemicalInitialPredictionShape(t, "explicit", explicitPrediction)

	expected := chemicalInitialReferenceValues(source, nilFixture.a0)
	assertChemicalInitialClose(t, "nil prediction", nilPrediction[0][0], expected.output)
	assertChemicalInitialClose(t, "explicit prediction", explicitPrediction[0][0], expected.output)
	assertChemicalInitialClose(t, "explicit Core.Initial", explicitGradient.Core.Initial[0], expected.explicitInitial)

	// An explicit activity equal to softplus(0) has the same forward value as
	// the implicit core output.  It therefore must leave every trainable group
	// and input gradient unchanged; only the initial-state boundary is a
	// constant in the explicit case.
	assertChemicalInitialGradientEqualExceptInitial(t, nilGradient, explicitGradient)
	assertChemicalInitialBitEqual(t, "implicit/explicit output", nilPrediction[0][0], explicitPrediction[0][0])
	t.Logf("source=%s window=%d explicit_control_output=%.17g explicit_initial=%.17g", source, window, explicitPrediction[0][0], expected.explicitInitial)
	assertChemicalInitialClose(t, "nil Core.Initial", nilGradient.Core.Initial[0], expected.implicitInitial)
	if math.Abs(nilGradient.Core.Initial[0]-explicitGradient.Core.Initial[0]) <= 1e-9 {
		t.Fatalf("implicit and explicit Core.Initial gradients did not differ: implicit=%.17g explicit=%.17g", nilGradient.Core.Initial[0], explicitGradient.Core.Initial[0])
	}
	t.Logf("source=%s window=%d output=%.17g implicit_initial=%.17g explicit_initial=%.17g", source, window, expected.output, expected.implicitInitial, expected.explicitInitial)
}

func chemicalInitialReferenceFixtureFor(source string, explicit bool) chemicalInitialReferenceFixture {
	a0 := math.Log1p(math.Exp(0))
	controller := &modulation.Controller{
		Inputs: modulation.ControllerInputs{SummaryWindow: 1},
		Nodes:  1, Hidden: 1, Channel: 0,
		// W1=[1,0], b1=0, w2=[1], b2=0.  The first source
		// release is softplus(tanh(activity)) and variance is zero.
		Parameters: []float64{1, 0, 0, 1, 0},
	}
	sourceSpec := modulation.SourceSpec{
		Kind: modulation.SourceController, Channel: 0, Controller: controller,
	}
	if source == "neural" {
		sourceSpec = modulation.SourceSpec{
			Kind: modulation.SourceNeuralActivity, Channel: 0,
			Neural: &modulation.NeuralActivity{Nodes: []int{0}, SetName: "initial-reference", Gain: 1, Channel: 0},
		}
	}
	episode := &learning.ChemicalEpisode{
		Part: learning.ChemicalPart{
			Config: modulation.ChemistryConfig{
				Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{1}},
				Sources:   []modulation.SourceSpec{sourceSpec},
				Receptors: modulation.Receptors{Records: []modulation.Receptor{{
					Cells: []int{0}, Signal: "initial-reference", Channel: 0,
					Status: modulation.StatusHypothesized, Kd: 1, N: 1,
					Evidence: "bounded-test-fixture", MeasurementKind: "declared-engineering",
					MappingVersion: "initial-reference/v1",
				}}},
				Effects: []modulation.Effect{{Kind: modulation.EffectSensitivity, Receptor: 0, GammaScale: 1}},
				Regions: modulation.RegionAssignment{NodeRegion: []int{0}},
			},
			State:     modulation.ChemistryState{Concentration: [][]float64{{0}}},
			Resources: map[string]float64{},
		},
		InitialActivity: nil,
	}
	if explicit {
		episode.InitialActivity = []float64{a0}
	}
	config := learning.Config{
		Dynamics:  dynamics.Config{Nodes: 1, Sources: []int{}, Targets: []int{}, Delays: []int{}, DT: 1, Activation: "softplus"},
		InputSize: 1, OutputSize: 1, ReadoutNodes: []int{0}, InputNodes: []int{0},
		ReadoutEveryStep: true, Chemistry: episode,
	}
	parameters := learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{}, Bias: []float64{0}, LogTau: []float64{0}},
		Encoder: []float64{1}, Readout: []float64{1},
	}
	return chemicalInitialReferenceFixture{
		config: config, parameters: parameters,
		input: [][]float64{{1}}, upstream: [][]float64{{1}}, a0: a0,
	}
}

type chemicalInitialReferenceExpected struct {
	output          float64
	implicitInitial float64
	explicitInitial float64
}

func chemicalInitialReferenceValues(source string, a0 float64) chemicalInitialReferenceExpected {
	e := math.Exp(-1)
	a := 1 - e
	q := a0
	dqDa0 := 1.0
	if source == "controller" {
		tanhA0 := math.Tanh(a0)
		q = math.Log1p(math.Exp(tanhA0))
		dqDa0 = (1 / (1 + math.Exp(-tanhA0))) * (1 - tanhA0*tanhA0)
	}
	c := a * q
	occupancy := c / (1 + c)
	v1 := a * (1 + occupancy)
	output := math.Log1p(math.Exp(v1))
	dOutputDv1 := 1 / (1 + math.Exp(-v1))
	implicit := dOutputDv1 * (e + a*a/((1+c)*(1+c))*dqDa0*0.5)
	explicit := dOutputDv1 * e
	return chemicalInitialReferenceExpected{output: float64(float32(output)), implicitInitial: implicit, explicitInitial: explicit}
}

func assertChemicalInitialGradientShape(t *testing.T, label string, gradient learning.Gradient) {
	t.Helper()
	if len(gradient.Core.Weights) != 0 || len(gradient.Core.Bias) != 1 || len(gradient.Core.LogTau) != 1 || len(gradient.ThetaRaw) != 0 || len(gradient.Core.Initial) != 1 || len(gradient.Encoder) != 1 || len(gradient.Readout) != 1 {
		t.Fatalf("%s gradient shape mismatch: weights=%d bias=%d log_tau=%d theta=%d initial=%d encoder=%d readout=%d", label, len(gradient.Core.Weights), len(gradient.Core.Bias), len(gradient.Core.LogTau), len(gradient.ThetaRaw), len(gradient.Core.Initial), len(gradient.Encoder), len(gradient.Readout))
	}
	if len(gradient.Core.Inputs) != 1 || len(gradient.Core.Inputs[0]) != 1 || len(gradient.Inputs) != 1 || len(gradient.Inputs[0]) != 1 {
		t.Fatalf("%s input gradient shape mismatch: core_inputs=%#v inputs=%#v", label, gradient.Core.Inputs, gradient.Inputs)
	}
	for name, values := range map[string][]float64{
		label + ".bias": gradient.Core.Bias, label + ".log_tau": gradient.Core.LogTau,
		label + ".initial": gradient.Core.Initial, label + ".encoder": gradient.Encoder,
		label + ".readout": gradient.Readout,
	} {
		for index, value := range values {
			assertChemicalInitialFinite(t, name+"["+chemicalInitialReferenceInt(index)+"]", value)
		}
	}
	assertChemicalInitialFinite(t, label+".core_inputs[0][0]", gradient.Core.Inputs[0][0])
	assertChemicalInitialFinite(t, label+".inputs[0][0]", gradient.Inputs[0][0])
}

func assertChemicalInitialPredictionShape(t *testing.T, label string, prediction [][]float64) {
	t.Helper()
	if len(prediction) != 1 || len(prediction[0]) != 1 {
		t.Fatalf("%s prediction shape mismatch: %#v", label, prediction)
	}
	assertChemicalInitialFinite(t, label+" prediction", prediction[0][0])
}

func assertChemicalInitialGradientEqualExceptInitial(t *testing.T, implicit, explicit learning.Gradient) {
	t.Helper()
	assertChemicalInitialBitEqual(t, "bias equality", implicit.Core.Bias[0], explicit.Core.Bias[0])
	assertChemicalInitialBitEqual(t, "log_tau equality", implicit.Core.LogTau[0], explicit.Core.LogTau[0])
	assertChemicalInitialBitEqual(t, "encoder equality", implicit.Encoder[0], explicit.Encoder[0])
	assertChemicalInitialBitEqual(t, "readout equality", implicit.Readout[0], explicit.Readout[0])
	assertChemicalInitialBitEqual(t, "core input equality", implicit.Core.Inputs[0][0], explicit.Core.Inputs[0][0])
	assertChemicalInitialBitEqual(t, "input equality", implicit.Inputs[0][0], explicit.Inputs[0][0])
}

func assertChemicalInitialBitEqual(t *testing.T, label string, got, want float64) {
	t.Helper()
	assertChemicalInitialFinite(t, label+" got", got)
	assertChemicalInitialFinite(t, label+" want", want)
	if math.Float64bits(got) != math.Float64bits(want) {
		t.Fatalf("%s: got=%.17g want=%.17g got_bits=%#x want_bits=%#x", label, got, want, math.Float64bits(got), math.Float64bits(want))
	}
}

func assertChemicalInitialClose(t *testing.T, label string, got, want float64) {
	t.Helper()
	assertChemicalInitialFinite(t, label+" got", got)
	assertChemicalInitialFinite(t, label+" want", want)
	if math.Abs(got-want) > chemicalInitialReferenceTolerance+1e-4*math.Abs(want) {
		t.Fatalf("%s: got=%.17g want=%.17g abs=%.17g", label, got, want, math.Abs(got-want))
	}
}

func assertChemicalInitialFinite(t *testing.T, label string, value float64) {
	t.Helper()
	if math.IsNaN(value) || math.IsInf(value, 0) {
		t.Fatalf("%s is non-finite: %v", label, value)
	}
}

func chemicalInitialReferenceInt(value int) string {
	return strconv.Itoa(value)
}
