package modulation

import (
	"math"
	"reflect"
	"strconv"
	"testing"
)

func chemicalCapacityMaxInt() int { return int(^uint(0) >> 1) }

func chemicalCapacityExpectError(t *testing.T, name string, call func() error) {
	t.Helper()
	var (
		err      error
		panicked any
	)
	func() {
		defer func() { panicked = recover() }()
		err = call()
	}()
	if panicked != nil {
		t.Errorf("%s panicked: %v", name, panicked)
		return
	}
	if err == nil {
		t.Errorf("%s returned nil error", name)
	}
}

func chemicalCapacityExpectReleaseError(t *testing.T, name string, call func() ([]float64, error)) {
	t.Helper()
	var (
		rates    []float64
		err      error
		panicked any
	)
	func() {
		defer func() { panicked = recover() }()
		rates, err = call()
	}()
	if panicked != nil {
		t.Errorf("%s panicked: %v", name, panicked)
		return
	}
	if err == nil {
		t.Errorf("%s returned nil error", name)
	}
	if rates != nil {
		t.Errorf("%s returned rates with an error: %v", name, rates)
	}
}

func chemicalCapacityExpectBuildError(t *testing.T, name string, spec SourceSpec) {
	t.Helper()
	var (
		built    Source
		err      error
		panicked any
	)
	func() {
		defer func() { panicked = recover() }()
		built, err = spec.Build()
	}()
	if panicked != nil {
		t.Errorf("%s panicked: %v", name, panicked)
		return
	}
	if err == nil {
		t.Errorf("%s returned nil error", name)
	}
	if built != nil {
		t.Errorf("%s returned a source with an error: %T", name, built)
	}
}

func chemicalCapacityExpectGradientError(t *testing.T, name string, call func() ([][]float64, error)) {
	t.Helper()
	var (
		gradient [][]float64
		err      error
		panicked any
	)
	func() {
		defer func() { panicked = recover() }()
		gradient, err = call()
	}()
	if panicked != nil {
		t.Errorf("%s panicked: %v", name, panicked)
		return
	}
	if err == nil {
		t.Errorf("%s returned nil error", name)
	}
	if gradient != nil {
		t.Errorf("%s returned a gradient with an error: %v", name, gradient)
	}
}

func chemicalCapacityExpectEffectsError(t *testing.T, name string, effects []Effect, nodes int) {
	t.Helper()
	var (
		gain, offset, threshold []float64
		clamped                 ClampReport
		err                     error
		panicked                any
	)
	func() {
		defer func() { panicked = recover() }()
		gain, offset, threshold, clamped, err = ApplyEffects(effects, nil, MixSum, nodes)
	}()
	if panicked != nil {
		t.Errorf("%s panicked: %v", name, panicked)
		return
	}
	if err == nil {
		t.Errorf("%s returned nil error", name)
	}
	if gain != nil || offset != nil || threshold != nil {
		t.Errorf("%s returned arrays with an error: gain=%v offset=%v threshold=%v", name, gain, offset, threshold)
	}
	if clamped != (ClampReport{}) {
		t.Errorf("%s returned a clamp report with an error: %+v", name, clamped)
	}
}

func chemicalCapacityController(nodes, window int) *Controller {
	return &Controller{
		Inputs:     ControllerInputs{SummaryWindow: window},
		Nodes:      nodes,
		Hidden:     1,
		Channel:    0,
		Parameters: []float64{1, 0, 0, 1, 0},
	}
}

func chemicalCapacityMemoryController(nodes, window int) *MemoryController {
	return &MemoryController{
		Inputs:     ControllerInputs{SummaryWindow: window},
		Nodes:      nodes,
		Hidden:     1,
		Parameters: []float64{1, 0, 0, 1, 0},
	}
}

func chemicalCapacityChemistryConfig(regions, channels int) ChemistryConfig {
	tau := make([]float64, channels)
	sources := make([]SourceSpec, channels)
	for channel := range tau {
		tau[channel] = 1
		sources[channel] = SourceSpec{
			Kind:    SourceExternalTimeline,
			Channel: channel,
			Timeline: &ExternalTimeline{
				ChannelCount: channel + 1,
			},
		}
	}
	return ChemistryConfig{
		Chemistry: Chemistry{Regions: regions, Channels: channels, DT: 1, Tau: tau},
		Sources:   sources,
		Regions:   RegionAssignment{NodeRegion: []int{0}},
	}
}

func TestChemicalCapacitySourceReleaseAndBuild(t *testing.T) {
	maxInt := chemicalCapacityMaxInt()
	portableCount := maxInt/8 + 1
	portableChannel := portableCount - 1
	for _, tc := range []struct {
		name    string
		channel int
	}{
		{name: "portable-channel-byte-boundary", channel: portableChannel},
		{name: "maxint-minus-one", channel: maxInt - 1},
	} {
		t.Run("NeuralActivity.Release/"+tc.name, func(t *testing.T) {
			neural := NeuralActivity{Nodes: []int{0}, Gain: 1, Channel: tc.channel}
			chemicalCapacityExpectReleaseError(t, "NeuralActivity.Release", func() ([]float64, error) {
				return neural.Release(0, SourceContext{Activity: []float64{1}})
			})
		})
		t.Run("InternalResource.Release/"+tc.name, func(t *testing.T) {
			resource := InternalResource{Resource: "energy", Coefficient: 1, Channel: tc.channel}
			chemicalCapacityExpectReleaseError(t, "InternalResource.Release", func() ([]float64, error) {
				return resource.Release(0, SourceContext{Resources: map[string]float64{"energy": 1}})
			})
		})
		t.Run("SourceSpec.Build/ExternalTimeline/"+tc.name, func(t *testing.T) {
			chemicalCapacityExpectBuildError(t, "SourceSpec.Build external timeline", SourceSpec{
				Kind:    SourceExternalTimeline,
				Channel: tc.channel,
				Timeline: &ExternalTimeline{
					ChannelCount: tc.channel + 1,
				},
			})
		})
		t.Run("SourceSpec.Build/NeuralActivity/"+tc.name, func(t *testing.T) {
			chemicalCapacityExpectBuildError(t, "SourceSpec.Build neural", SourceSpec{
				Kind:    SourceNeuralActivity,
				Channel: tc.channel,
				Neural:  &NeuralActivity{Nodes: []int{0}, Gain: 1, Channel: tc.channel},
			})
		})
		t.Run("SourceSpec.Build/InternalResource/"+tc.name, func(t *testing.T) {
			chemicalCapacityExpectBuildError(t, "SourceSpec.Build resource", SourceSpec{
				Kind:     SourceInternalResource,
				Channel:  tc.channel,
				Resource: &InternalResource{Resource: "energy", Coefficient: 1, Channel: tc.channel},
			})
		})
		t.Run("Controller.Release/"+tc.name, func(t *testing.T) {
			controller := &Controller{
				Inputs:     ControllerInputs{SummaryWindow: 1},
				Nodes:      1,
				Hidden:     1,
				Channel:    tc.channel,
				Parameters: []float64{1, 0, 0, 1, 0},
			}
			chemicalCapacityExpectReleaseError(t, "Controller.Release", func() ([]float64, error) {
				return controller.Release(0, SourceContext{Activity: []float64{1}})
			})
		})
		t.Run("SourceSpec.Build/Controller/"+tc.name, func(t *testing.T) {
			chemicalCapacityExpectBuildError(t, "SourceSpec.Build controller", SourceSpec{
				Kind:    SourceController,
				Channel: tc.channel,
				Controller: &Controller{
					Inputs:     ControllerInputs{SummaryWindow: 1},
					Nodes:      1,
					Hidden:     1,
					Channel:    tc.channel,
					Parameters: []float64{1, 0, 0, 1, 0},
				},
			})
		})
		t.Run("ExternalTimeline.Release/"+tc.name, func(t *testing.T) {
			timeline := ExternalTimeline{ChannelCount: tc.channel + 1}
			chemicalCapacityExpectReleaseError(t, "ExternalTimeline.Release", func() ([]float64, error) {
				return timeline.Release(0, SourceContext{})
			})
		})
	}
}

func TestChemicalCapacityControllerAndMemoryRejectUnrepresentableNodes(t *testing.T) {
	maxInt := chemicalCapacityMaxInt()

	t.Run("controller-validate-release-backward-and-restore", func(t *testing.T) {
		controller := chemicalCapacityController(2, 2)
		rates, err := controller.Release(0, SourceContext{Activity: []float64{1, 0}})
		if err != nil {
			t.Fatalf("small Controller.Release() error = %v", err)
		}
		if len(rates) != 1 || !(rates[0] > 0) || math.IsNaN(rates[0]) || math.IsInf(rates[0], 0) {
			t.Fatalf("small Controller.Release() = %v, want one positive rate", rates)
		}
		beforeState := controller.State()
		beforeGradient, err := controller.BackwardActivity(1)
		if err != nil {
			t.Fatalf("small Controller.BackwardActivity() error = %v", err)
		}

		controller.Nodes = maxInt
		chemicalCapacityExpectError(t, "Controller.Validate Nodes=MaxInt", controller.Validate)
		chemicalCapacityExpectReleaseError(t, "Controller.Release Nodes=MaxInt", func() ([]float64, error) {
			return controller.Release(1, SourceContext{})
		})
		freshController := chemicalCapacityController(2, 2)
		freshController.Nodes = maxInt
		chemicalCapacityExpectReleaseError(t, "fresh Controller.Release Nodes=MaxInt", func() ([]float64, error) {
			return freshController.Release(0, SourceContext{})
		})
		chemicalCapacityExpectGradientError(t, "Controller.BackwardActivity Nodes=MaxInt", func() ([][]float64, error) {
			return controller.BackwardActivity(1)
		})

		controller.Nodes = 2
		if got := controller.State(); !reflect.DeepEqual(got, beforeState) {
			t.Fatalf("Controller.State changed after rejected declaration: got=%+v want=%+v", got, beforeState)
		}
		afterGradient, err := controller.BackwardActivity(1)
		if err != nil {
			t.Fatalf("Controller.BackwardActivity() after restore error = %v", err)
		}
		if !reflect.DeepEqual(afterGradient, beforeGradient) {
			t.Fatalf("Controller.BackwardActivity() changed after rejected declaration: got=%v want=%v", afterGradient, beforeGradient)
		}
	})

	t.Run("memory-validate-offset-gradient-and-restore", func(t *testing.T) {
		memory := chemicalCapacityMemoryController(2, 2)
		control := chemicalCapacityMemoryController(2, 2)
		firstContext := SourceContext{Activity: []float64{1, 0}}
		beforeOffset, err := memory.Offset(0, firstContext)
		if err != nil {
			t.Fatalf("small MemoryController.Offset() error = %v", err)
		}
		if !(beforeOffset > 0) || math.IsNaN(beforeOffset) || math.IsInf(beforeOffset, 0) {
			t.Fatalf("small MemoryController.Offset() = %v, want a finite positive offset", beforeOffset)
		}
		controlOffset, err := control.Offset(0, firstContext)
		if err != nil {
			t.Fatalf("small control MemoryController.Offset() error = %v", err)
		}
		if controlOffset != beforeOffset {
			t.Fatalf("first MemoryController.Offset() = %v, control = %v", beforeOffset, controlOffset)
		}
		beforeLoss, beforeGradient, err := memory.Gradient(0.25)
		if err != nil {
			t.Fatalf("small MemoryController.Gradient() error = %v", err)
		}
		controlLoss, controlGradient, err := control.Gradient(0.25)
		if err != nil {
			t.Fatalf("small control MemoryController.Gradient() error = %v", err)
		}
		if beforeLoss != controlLoss || !reflect.DeepEqual(beforeGradient, controlGradient) {
			t.Fatalf("first MemoryController.Gradient() = (%v, %v), control = (%v, %v)", beforeLoss, beforeGradient, controlLoss, controlGradient)
		}

		memory.Nodes = maxInt
		chemicalCapacityExpectError(t, "MemoryController.Validate Nodes=MaxInt", memory.Validate)
		var panicked any
		var offset float64
		var offsetErr error
		func() {
			defer func() { panicked = recover() }()
			offset, offsetErr = memory.Offset(1, SourceContext{})
		}()
		if panicked != nil {
			t.Errorf("MemoryController.Offset Nodes=MaxInt panicked: %v", panicked)
		} else {
			if offsetErr == nil {
				t.Errorf("MemoryController.Offset Nodes=MaxInt returned nil error and offset %v", offset)
			}
			if offset != 0 {
				t.Errorf("MemoryController.Offset Nodes=MaxInt returned offset %v with an error", offset)
			}
		}
		var (
			loss        float64
			gradient    []float64
			gradientErr error
		)
		chemicalCapacityCallMemoryGradient := func() {
			defer func() { panicked = recover() }()
			loss, gradient, gradientErr = memory.Gradient(0.25)
		}
		panicked = nil
		chemicalCapacityCallMemoryGradient()
		if panicked != nil {
			t.Errorf("MemoryController.Gradient Nodes=MaxInt panicked: %v", panicked)
		} else {
			if gradientErr == nil {
				t.Errorf("MemoryController.Gradient Nodes=MaxInt returned nil error and loss %v", loss)
			}
			if loss != 0 {
				t.Errorf("MemoryController.Gradient Nodes=MaxInt returned loss %v with an error, want zero", loss)
			}
			if gradient != nil {
				t.Errorf("MemoryController.Gradient Nodes=MaxInt returned gradient with an error: %v", gradient)
			}
		}

		memory.Nodes = 2
		nextContext := SourceContext{Activity: []float64{0, 1}}
		afterOffset, err := memory.Offset(1, nextContext)
		if err != nil {
			t.Fatalf("MemoryController.Offset() after restore error = %v", err)
		}
		controlOffset, err = control.Offset(1, nextContext)
		if err != nil {
			t.Fatalf("control MemoryController.Offset() after restore error = %v", err)
		}
		if afterOffset != controlOffset {
			t.Fatalf("MemoryController.Offset() after rejected declaration = %v, control = %v", afterOffset, controlOffset)
		}
		afterLoss, afterGradient, err := memory.Gradient(0.25)
		if err != nil {
			t.Fatalf("MemoryController.Gradient() after restore error = %v", err)
		}
		controlLoss, controlGradient, err = control.Gradient(0.25)
		if err != nil {
			t.Fatalf("control MemoryController.Gradient() after restore error = %v", err)
		}
		if afterLoss != controlLoss {
			t.Fatalf("MemoryController loss after rejected declaration = %v, control = %v", afterLoss, controlLoss)
		}
		if !reflect.DeepEqual(afterGradient, controlGradient) {
			t.Fatalf("MemoryController gradient after rejected declaration = %v, control = %v", afterGradient, controlGradient)
		}
	})

	t.Run("summary-window-grid-controller-and-memory", func(t *testing.T) {
		nodes := maxInt/(8*16) + 1
		controller := chemicalCapacityController(nodes, 16)
		chemicalCapacityExpectError(t, "Controller.Validate SummaryWindow=16 grid", controller.Validate)
		memory := chemicalCapacityMemoryController(nodes, 16)
		chemicalCapacityExpectError(t, "MemoryController.Validate SummaryWindow=16 grid", memory.Validate)
	})
}

func TestChemicalCapacityChemistryAndEffectsRejectUnrepresentableShapes(t *testing.T) {
	maxInt := chemicalCapacityMaxInt()
	portableRegions := maxInt/(3*(strconv.IntSize/8)) + 1
	gridRegions := maxInt/(8*4) + 1

	for _, tc := range []struct {
		name     string
		regions  int
		channels int
	}{
		{name: "regions-maxint", regions: maxInt, channels: 1},
		{name: "regions-portable-header", regions: portableRegions, channels: 1},
		{name: "region-channel-grid", regions: gridRegions, channels: 4},
	} {
		t.Run(tc.name+"/NewChemistry", func(t *testing.T) {
			var (
				kinetics *Kinetics
				err      error
				panicked any
			)
			func() {
				defer func() { panicked = recover() }()
				tau := make([]float64, tc.channels)
				for i := range tau {
					tau[i] = 1
				}
				kinetics, err = NewChemistry(Chemistry{Regions: tc.regions, Channels: tc.channels, DT: 1, Tau: tau})
			}()
			if panicked != nil {
				t.Errorf("NewChemistry() panicked: %v", panicked)
				return
			}
			if err == nil {
				t.Errorf("NewChemistry() returned nil error and kinetics=%v", kinetics)
			}
			if kinetics != nil {
				t.Errorf("NewChemistry() returned kinetics with an error: %+v", kinetics)
			}
		})
		t.Run(tc.name+"/ChemistryConfig.Validate", func(t *testing.T) {
			config := chemicalCapacityChemistryConfig(tc.regions, tc.channels)
			chemicalCapacityExpectError(t, "ChemistryConfig.Validate", func() error {
				return config.Validate(1)
			})
		})
	}

	t.Run("ApplyEffects-nil-occupancy", func(t *testing.T) {
		chemicalCapacityExpectEffectsError(t, "ApplyEffects nil effects", nil, maxInt)
	})
	t.Run("ApplyEffects-valid-effect-nil-occupancy", func(t *testing.T) {
		chemicalCapacityExpectEffectsError(t, "ApplyEffects valid effect", []Effect{{Kind: EffectSensitivity, Receptor: 0, GammaScale: 1}}, maxInt)
	})
}

func TestChemicalCapacitySmallControls(t *testing.T) {
	t.Run("nonzero-sources", func(t *testing.T) {
		timeline := ExternalTimeline{ChannelCount: 1, Entries: []TimelineEntry{{Step: 0, Channel: 0, Rate: 2}}}
		got, err := timeline.Release(0, SourceContext{})
		if err != nil || !reflect.DeepEqual(got, []float64{2}) {
			t.Fatalf("ExternalTimeline.Release() = %v, %v, want [2], nil", got, err)
		}
		neural := NeuralActivity{Nodes: []int{0}, Gain: 2, Channel: 0}
		got, err = neural.Release(0, SourceContext{Activity: []float64{1}})
		if err != nil || !reflect.DeepEqual(got, []float64{2}) {
			t.Fatalf("NeuralActivity.Release() = %v, %v, want [2], nil", got, err)
		}
		resource := InternalResource{Resource: "energy", Coefficient: 2, Threshold: 1, Channel: 0}
		got, err = resource.Release(0, SourceContext{Resources: map[string]float64{"energy": 3}})
		if err != nil || !reflect.DeepEqual(got, []float64{4}) {
			t.Fatalf("InternalResource.Release() = %v, %v, want [4], nil", got, err)
		}
	})

	t.Run("controller-and-kinetics", func(t *testing.T) {
		controller := chemicalCapacityController(2, 2)
		rates, err := controller.Release(0, SourceContext{Activity: []float64{1, 0}})
		if err != nil {
			t.Fatalf("Controller.Release() error = %v", err)
		}
		if len(rates) != 1 || !(rates[0] > 0) || math.IsNaN(rates[0]) || math.IsInf(rates[0], 0) {
			t.Fatalf("Controller.Release() = %v, want one positive rate", rates)
		}
		kinetics, err := NewChemistry(Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{1}})
		if err != nil {
			t.Fatalf("NewChemistry() small control error = %v", err)
		}
		state, err := kinetics.Step(kinetics.NewState(), [][]float64{{rates[0]}}, nil)
		if err != nil {
			t.Fatalf("Kinetics.Step() small control error = %v", err)
		}
		if len(state.Concentration) != 1 || len(state.Concentration[0]) != 1 || !(state.Concentration[0][0] > 0) || state.Steps != 1 {
			t.Fatalf("Kinetics.Step() = %+v, want one positive concentration and one step", state)
		}
	})
}
