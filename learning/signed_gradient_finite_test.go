package learning_test

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

func signedGradientFixture(raw float64, sign int8, fixed bool) (learning.Config, learning.Parameters, dynamics.Config) {
	// Four zero-input steps and a reverse window of two keep the direct tanh
	// core derivatives finite while still carrying the fixed-sign edge product
	// through the reverse boundary where raw=370 overflows.
	core := dynamics.Config{
		Nodes:      3,
		Sources:    []int{0, 1},
		Targets:    []int{1, 2},
		Delays:     []int{0, 0},
		DT:         1,
		Activation: "tanh",
	}
	config := learning.Config{
		Dynamics:     core,
		InputSize:    1,
		OutputSize:   1,
		InputNodes:   []int{2},
		ReadoutNodes: []int{2},
	}
	weight := math.Exp(raw)
	parameters := learning.Parameters{
		Core: dynamics.Parameters{
			Weights: []float64{raw, raw},
			Bias:    []float64{1e160, -float64(sign) * weight, 0},
			LogTau:  []float64{math.Log(0.001), math.Log(0.001), math.Log(0.001)},
		},
		Encoder: []float64{1},
		Readout: []float64{1},
	}
	if fixed {
		config.EdgeSigns = []int8{sign, 1}
	} else {
		parameters.Core.Weights = []float64{float64(sign) * weight, weight}
	}
	return config, parameters, core
}

func signedGradientRows(rows [][]float64) [][]float64 {
	if rows == nil {
		return nil
	}
	out := make([][]float64, len(rows))
	for i, row := range rows {
		out[i] = append([]float64(nil), row...)
	}
	return out
}

func signedGradientInputRows() [][]float64 {
	return [][]float64{{0}, {0}, {0}, {0}}
}

func signedGradientUpstreamRows(value float64) [][]float64 {
	return [][]float64{{0}, {0}, {0}, {value}}
}

func signedGradientCoreInputRows() [][]float64 {
	return [][]float64{{0, 0, 0}, {0, 0, 0}, {0, 0, 0}, {0, 0, 0}}
}

func signedGradientCoreUpstreamRows(value float64) [][]float64 {
	return [][]float64{{0, 0, 0}, {0, 0, 0}, {0, 0, 0}, {0, 0, value}}
}

func signedGradientCloneParameters(parameters learning.Parameters) learning.Parameters {
	clone := parameters
	clone.Core.Weights = append([]float64(nil), parameters.Core.Weights...)
	clone.Core.Bias = append([]float64(nil), parameters.Core.Bias...)
	clone.Core.LogTau = append([]float64(nil), parameters.Core.LogTau...)
	clone.ThetaRaw = append([]float64(nil), parameters.ThetaRaw...)
	clone.Encoder = append([]float64(nil), parameters.Encoder...)
	clone.Readout = append([]float64(nil), parameters.Readout...)
	return clone
}

func signedGradientCloneState(state learning.NeuralState) learning.NeuralState {
	clone := state
	if state.Continuous != nil {
		continuous := *state.Continuous
		continuous.Voltage = append([]float64(nil), state.Continuous.Voltage...)
		continuous.History = signedGradientRows(state.Continuous.History)
		clone.Continuous = &continuous
	}
	return clone
}

func signedGradientInitialState(t *testing.T, core dynamics.Config) learning.NeuralState {
	t.Helper()
	model, err := dynamics.NewContinuous(core)
	if err != nil {
		t.Fatalf("new continuous model: %v", err)
	}
	state, err := model.NewState([]float64{0, 0, 0})
	if err != nil {
		t.Fatalf("new continuous state: %v", err)
	}
	return learning.NeuralState{Core: learning.NeuralCoreContinuous, Continuous: &state}
}

func signedGradientDirect(t *testing.T, parameters learning.Parameters, config learning.Config, core dynamics.Config, upstream float64) (dynamics.Gradient, []float64) {
	t.Helper()
	effective, err := learning.EffectiveWeights(config, parameters)
	if err != nil {
		t.Fatalf("effective weights: %v", err)
	}
	model, err := dynamics.NewContinuous(core)
	if err != nil {
		t.Fatalf("new continuous model: %v", err)
	}
	coreParameters := parameters.Core
	coreParameters.Weights = append([]float64(nil), effective...)
	trace, err := model.Forward(context.Background(), coreParameters, make([]float64, core.Nodes), signedGradientCoreInputRows())
	if err != nil {
		t.Fatalf("direct forward: %v", err)
	}
	gradient, err := model.Backward(context.Background(), trace, signedGradientCoreUpstreamRows(upstream), 2)
	if err != nil {
		t.Fatalf("direct backward: %v", err)
	}
	return gradient, effective
}

func signedGradientFinite(values []float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}
	return true
}

func signedGradientFiniteRows(rows [][]float64) bool {
	for _, row := range rows {
		if !signedGradientFinite(row) {
			return false
		}
	}
	return true
}

func signedGradientRequireFinite(t *testing.T, gradient learning.Gradient) {
	t.Helper()
	if !signedGradientFinite(gradient.Core.Weights) ||
		!signedGradientFinite(gradient.Core.Bias) ||
		!signedGradientFinite(gradient.Core.LogTau) ||
		!signedGradientFinite(gradient.Core.Initial) ||
		!signedGradientFiniteRows(gradient.Core.Inputs) ||
		!signedGradientFinite(gradient.ThetaRaw) ||
		!signedGradientFinite(gradient.Encoder) ||
		!signedGradientFinite(gradient.Readout) ||
		!signedGradientFiniteRows(gradient.Inputs) {
		t.Fatal("gradient contains a non-finite value")
	}
}

func signedGradientRequireFiniteCore(t *testing.T, gradient dynamics.Gradient) {
	t.Helper()
	if !signedGradientFinite(gradient.Weights) ||
		!signedGradientFinite(gradient.Bias) ||
		!signedGradientFinite(gradient.LogTau) ||
		!signedGradientFinite(gradient.Initial) ||
		!signedGradientFiniteRows(gradient.Inputs) {
		t.Fatal("direct core gradient contains a non-finite value")
	}
}

func signedGradientRequireShape(t *testing.T, gradient learning.Gradient, parameters learning.Parameters, inputRows int) {
	t.Helper()
	if len(gradient.Encoder) != len(parameters.Encoder) || len(gradient.Readout) != len(parameters.Readout) || len(gradient.ThetaRaw) != len(parameters.ThetaRaw) {
		t.Fatalf("parameter gradient shape mismatch: encoder=%d/%d readout=%d/%d theta=%d/%d", len(gradient.Encoder), len(parameters.Encoder), len(gradient.Readout), len(parameters.Readout), len(gradient.ThetaRaw), len(parameters.ThetaRaw))
	}
	if len(gradient.Inputs) != inputRows {
		t.Fatalf("input gradient rows: got %d, want %d", len(gradient.Inputs), inputRows)
	}
	for i, row := range gradient.Inputs {
		if len(row) != 1 {
			t.Fatalf("input gradient row %d width: got %d, want 1", i, len(row))
		}
	}
}

func signedGradientRequireCoreReference(t *testing.T, got learning.Gradient, parameters learning.Parameters, config learning.Config, direct dynamics.Gradient, effective []float64, fixed bool) {
	t.Helper()
	signedGradientRequireFinite(t, got)
	signedGradientRequireShape(t, got, parameters, len(signedGradientInputRows()))

	wantWeights := append([]float64(nil), direct.Weights...)
	if fixed {
		for i, edgeSign := range config.EdgeSigns {
			if edgeSign != 0 {
				wantWeights[i] = direct.Weights[i] * effective[i]
			}
		}
	}
	signedGradientRequireBits(t, "core weights", got.Core.Weights, wantWeights)
	signedGradientRequireBits(t, "core bias", got.Core.Bias, direct.Bias)
	signedGradientRequireBits(t, "core log tau", got.Core.LogTau, direct.LogTau)
	signedGradientRequireBits(t, "core initial", got.Core.Initial, direct.Initial)
	signedGradientRequireRowBits(t, "core inputs", got.Core.Inputs, direct.Inputs)
}

func signedGradientRequireBits(t *testing.T, name string, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s length: got %d, want %d", name, len(got), len(want))
	}
	for i := range got {
		if math.Float64bits(got[i]) != math.Float64bits(want[i]) {
			t.Fatalf("%s[%d]: got %v (%x), want %v (%x)", name, i, got[i], math.Float64bits(got[i]), want[i], math.Float64bits(want[i]))
		}
	}
}

func signedGradientRequireRowBits(t *testing.T, name string, got, want [][]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s rows: got %d, want %d", name, len(got), len(want))
	}
	for i := range got {
		signedGradientRequireBits(t, fmt.Sprintf("%s[%d]", name, i), got[i], want[i])
	}
}

func signedGradientRequireZero(t *testing.T, result learning.Gradient, loss float64, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected non-finite signed-gradient error")
	}
	if !reflect.DeepEqual(result, learning.Gradient{}) {
		t.Fatalf("failed gradient is not zero: %#v", result)
	}
	if loss != 0 {
		t.Fatalf("failed loss: got %v, want zero", loss)
	}
}

func signedGradientOptions(segment int) learning.Options {
	options := learning.DefaultOptions()
	options.AccumulateSteps = 2
	options.Truncation = 2
	if segment > 0 {
		options.Recompute = &learning.Recompute{SegmentSteps: segment}
	}
	return options
}

func TestFixedSignGradientNetworkRejectsOverflow(t *testing.T) {
	for _, sign := range []int8{1, -1} {
		for _, upstream := range []float64{1, -1} {
			t.Run(fmt.Sprintf("sign=%d/upstream=%+.0f", sign, upstream), func(t *testing.T) {
				config, parameters, core := signedGradientFixture(370, sign, true)
				direct, _ := signedGradientDirect(t, parameters, config, core, upstream)
				signedGradientRequireFiniteCore(t, direct)

				t.Run("LossGradientFrom", func(t *testing.T) {
					network, err := learning.NewNetwork(config)
					if err != nil {
						t.Fatalf("new network: %v", err)
					}
					input := signedGradientInputRows()
					inputBefore := signedGradientRows(input)
					upstreamRows := signedGradientUpstreamRows(upstream)
					upstreamBefore := signedGradientRows(upstreamRows)
					callParameters := signedGradientCloneParameters(parameters)
					parametersBefore := signedGradientCloneParameters(callParameters)
					gradient, err := network.LossGradientFrom(context.Background(), callParameters, input, upstreamRows, 2)
					signedGradientRequireZero(t, gradient, 0, err)
					if !reflect.DeepEqual(callParameters, parametersBefore) {
						t.Fatal("LossGradientFrom mutated parameters on failure")
					}
					if !reflect.DeepEqual(input, inputBefore) || !reflect.DeepEqual(upstreamRows, upstreamBefore) {
						t.Fatal("LossGradientFrom mutated caller-owned input or upstream")
					}
				})

				t.Run("LossGradient", func(t *testing.T) {
					network, err := learning.NewNetwork(config)
					if err != nil {
						t.Fatalf("new network: %v", err)
					}
					input := signedGradientInputRows()
					inputBefore := signedGradientRows(input)
					target := []float64{-upstream / 2}
					targetBefore := append([]float64(nil), target...)
					callParameters := signedGradientCloneParameters(parameters)
					parametersBefore := signedGradientCloneParameters(callParameters)
					loss, gradient, err := network.LossGradient(context.Background(), callParameters, input, target, 2)
					signedGradientRequireZero(t, gradient, loss, err)
					if !reflect.DeepEqual(callParameters, parametersBefore) || !reflect.DeepEqual(input, inputBefore) || !reflect.DeepEqual(target, targetBefore) {
						t.Fatal("LossGradient mutated caller-owned data on failure")
					}
				})

				t.Run("LossGradientFromState", func(t *testing.T) {
					network, err := learning.NewNetwork(config)
					if err != nil {
						t.Fatalf("new network: %v", err)
					}
					state := signedGradientInitialState(t, core)
					stateBefore := signedGradientCloneState(state)
					input := signedGradientInputRows()
					inputBefore := signedGradientRows(input)
					upstreamRows := signedGradientUpstreamRows(upstream)
					upstreamBefore := signedGradientRows(upstreamRows)
					callParameters := signedGradientCloneParameters(parameters)
					parametersBefore := signedGradientCloneParameters(callParameters)
					gradient, err := network.LossGradientFromState(context.Background(), callParameters, state, input, upstreamRows, 2)
					signedGradientRequireZero(t, gradient, 0, err)
					if !reflect.DeepEqual(callParameters, parametersBefore) || !reflect.DeepEqual(state, stateBefore) || !reflect.DeepEqual(input, inputBefore) || !reflect.DeepEqual(upstreamRows, upstreamBefore) {
						t.Fatal("LossGradientFromState mutated caller-owned data on failure")
					}
				})
			})
		}
	}
}

func TestFixedSignGradientFiniteControls(t *testing.T) {
	cases := []struct {
		name  string
		raw   float64
		fixed bool
	}{
		{name: "fixed-raw300", raw: 300, fixed: true},
		{name: "free-raw370", raw: 370, fixed: false},
	}
	for _, testCase := range cases {
		for _, sign := range []int8{1, -1} {
			for _, upstream := range []float64{1, -1} {
				t.Run(fmt.Sprintf("%s/sign=%d/upstream=%+.0f", testCase.name, sign, upstream), func(t *testing.T) {
					config, parameters, core := signedGradientFixture(testCase.raw, sign, testCase.fixed)
					direct, effective := signedGradientDirect(t, parameters, config, core, upstream)
					signedGradientRequireFiniteCore(t, direct)
					network, err := learning.NewNetwork(config)
					if err != nil {
						t.Fatalf("new network: %v", err)
					}

					t.Run("LossGradientFrom", func(t *testing.T) {
						callParameters := signedGradientCloneParameters(parameters)
						got, err := network.LossGradientFrom(context.Background(), callParameters, signedGradientInputRows(), signedGradientUpstreamRows(upstream), 2)
						if err != nil {
							t.Fatalf("finite control rejected: %v", err)
						}
						signedGradientRequireCoreReference(t, got, callParameters, config, direct, effective, testCase.fixed)
					})

					t.Run("LossGradient", func(t *testing.T) {
						callParameters := signedGradientCloneParameters(parameters)
						loss, got, err := network.LossGradient(context.Background(), callParameters, signedGradientInputRows(), []float64{-upstream / 2}, 2)
						if err != nil {
							t.Fatalf("finite control rejected: %v", err)
						}
						if math.IsNaN(loss) || math.IsInf(loss, 0) {
							t.Fatalf("finite control returned non-finite loss: %v", loss)
						}
						signedGradientRequireCoreReference(t, got, callParameters, config, direct, effective, testCase.fixed)
					})

					t.Run("LossGradientFromState", func(t *testing.T) {
						state := signedGradientInitialState(t, core)
						callParameters := signedGradientCloneParameters(parameters)
						got, err := network.LossGradientFromState(context.Background(), callParameters, state, signedGradientInputRows(), signedGradientUpstreamRows(upstream), 2)
						if err != nil {
							t.Fatalf("finite state control rejected: %v", err)
						}
						signedGradientRequireCoreReference(t, got, callParameters, config, direct, effective, testCase.fixed)
					})
				})
			}
		}
	}
}

func TestFixedSignGradientTrainerRejectsFailureAtomically(t *testing.T) {
	for _, sign := range []int8{1, -1} {
		for _, segment := range []int{0, 1, 3, 7} {
			for _, from := range []bool{false, true} {
				name := fmt.Sprintf("sign=%d/segment=%d", sign, segment)
				if from {
					name += "/from"
				} else {
					name += "/target"
				}
				t.Run(name, func(t *testing.T) {
					config, parameters, _ := signedGradientFixture(370, sign, true)
					trainer, err := learning.NewTrainer(config, parameters, signedGradientOptions(segment))
					if err != nil {
						t.Fatalf("new trainer: %v", err)
					}
					defer func() {
						if err := trainer.Close(); err != nil {
							t.Errorf("close trainer: %v", err)
						}
					}()
					input := signedGradientInputRows()
					if from {
						_, err = trainer.StepFrom(context.Background(), input, signedGradientUpstreamRows(0))
					} else {
						_, err = trainer.Step(context.Background(), input, []float64{0})
					}
					if err != nil {
						t.Fatalf("zero-loss accumulation step: %v", err)
					}
					middle := trainer.Snapshot()
					if middle.Accumulator == nil || middle.Accumulator.Count != 1 || middle.Updates != 0 {
						count := 0
						if middle.Accumulator != nil {
							count = middle.Accumulator.Count
						}
						t.Fatalf("unexpected partial accumulation: count=%d updates=%d", count, middle.Updates)
					}

					before := trainer.Snapshot()
					badInput := signedGradientInputRows()
					badInputBefore := signedGradientRows(badInput)
					var result learning.StepResult
					if from {
						badUpstream := signedGradientUpstreamRows(1)
						badUpstreamBefore := signedGradientRows(badUpstream)
						result, err = trainer.StepFrom(context.Background(), badInput, badUpstream)
						if !reflect.DeepEqual(badUpstream, badUpstreamBefore) {
							t.Fatal("StepFrom mutated caller-owned upstream on failure")
						}
					} else {
						badTarget := []float64{-0.5}
						badTargetBefore := append([]float64(nil), badTarget...)
						result, err = trainer.Step(context.Background(), badInput, badTarget)
						if !reflect.DeepEqual(badTarget, badTargetBefore) {
							t.Fatal("Step mutated caller-owned target on failure")
						}
					}
					if err == nil {
						t.Fatal("expected Step numerical failure")
					}
					if !reflect.DeepEqual(result, learning.StepResult{}) {
						t.Fatalf("failed StepResult is not zero: %#v", result)
					}
					if !reflect.DeepEqual(before, trainer.Snapshot()) {
						t.Fatal("trainer snapshot changed after rejected numerical gradient")
					}
					if !reflect.DeepEqual(badInput, badInputBefore) {
						t.Fatal("trainer mutated caller-owned input on failure")
					}
				})
			}
		}
	}
}

func TestFixedSignGradientStateStepRejectsFailureAtomically(t *testing.T) {
	for _, sign := range []int8{1, -1} {
		for _, upstream := range []float64{1, -1} {
			t.Run(fmt.Sprintf("sign=%d/upstream=%+.0f", sign, upstream), func(t *testing.T) {
				config, parameters, core := signedGradientFixture(370, sign, true)
				trainer, err := learning.NewTrainer(config, parameters, signedGradientOptions(0))
				if err != nil {
					t.Fatalf("new trainer: %v", err)
				}
				defer func() {
					if err := trainer.Close(); err != nil {
						t.Errorf("close trainer: %v", err)
					}
				}()
				state := signedGradientInitialState(t, core)
				input := signedGradientInputRows()
				if _, err = trainer.StepFromState(context.Background(), state, input, signedGradientUpstreamRows(0)); err != nil {
					t.Fatalf("zero-loss state accumulation step: %v", err)
				}
				middle := trainer.Snapshot()
				if middle.Accumulator == nil || middle.Accumulator.Count != 1 || middle.Updates != 0 {
					count := 0
					if middle.Accumulator != nil {
						count = middle.Accumulator.Count
					}
					t.Fatalf("unexpected state partial accumulation: count=%d updates=%d", count, middle.Updates)
				}
				before := middle
				stateBefore := signedGradientCloneState(state)
				badInput := signedGradientInputRows()
				badInputBefore := signedGradientRows(badInput)
				badUpstream := signedGradientUpstreamRows(upstream)
				badUpstreamBefore := signedGradientRows(badUpstream)
				result, err := trainer.StepFromState(context.Background(), state, badInput, badUpstream)
				if err == nil {
					t.Fatal("expected StepFromState numerical failure")
				}
				if !reflect.DeepEqual(result, learning.StepResult{}) {
					t.Fatalf("failed StepFromState result is not zero: %#v", result)
				}
				if !reflect.DeepEqual(before, trainer.Snapshot()) {
					t.Fatal("trainer snapshot changed after rejected state gradient")
				}
				if !reflect.DeepEqual(state, stateBefore) || !reflect.DeepEqual(badInput, badInputBefore) || !reflect.DeepEqual(badUpstream, badUpstreamBefore) {
					t.Fatal("StepFromState mutated caller-owned state or inputs on failure")
				}
			})
		}
	}
}
