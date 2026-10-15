package dynamics

import (
	"context"
	"math"
	"reflect"
	"slices"
	"testing"
)

// effectiveThresholdExtremeConfig uses a symmetric interval whose endpoints
// are finite but whose span is not.  Let A = 2^1023.  The threshold transform
// can still produce the finite base A at raw=1000; the cases below then make
// only the effective threshold, base+adaptation or base+homeostasis, overflow.
func effectiveThresholdExtremeConfig(dt float64) LIFConfig {
	a := math.Ldexp(1, 1023)
	return LIFConfig{
		Nodes: 1, DT: dt, TauSyn: 1,
		ThetaMin: -a, ThetaMax: a, VReset: -math.MaxFloat64,
		Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
	}
}

func effectiveThresholdAdaptationLIFFixture(t *testing.T) (*LIF, LIFParameters, []float64, [][]float64) {
	t.Helper()
	config := effectiveThresholdExtremeConfig(math.Ln2)
	config.Adaptation = LIFAdaptation{Enabled: true, TauAdapt: 1, Beta: math.MaxFloat64}
	model, err := NewLIF(config)
	if err != nil {
		t.Fatal(err)
	}
	return model,
		LIFParameters{Bias: []float64{math.MaxFloat64}, LogTau: []float64{0}, ThetaRaw: []float64{1000}},
		[]float64{math.MaxFloat64}, effectiveThresholdZeroInputs(2)
}

func effectiveThresholdHomeostasisLIFFixture(t *testing.T) (*LIF, LIFParameters, []float64, [][]float64) {
	t.Helper()
	hMax := math.Ldexp(3, 1022) // 1.5*A, finite and exactly representable.
	config := effectiveThresholdExtremeConfig(1)
	config.Homeostasis = &LIFHomeostasis{
		Enabled: true, TauRate: 1, TargetRate: 0, Eta: math.MaxFloat64, HMax: hMax,
	}
	model, err := NewLIF(config)
	if err != nil {
		t.Fatal(err)
	}
	return model,
		LIFParameters{Bias: []float64{hMax}, LogTau: []float64{0}, ThetaRaw: []float64{1000}},
		[]float64{hMax}, effectiveThresholdZeroInputs(2)
}

func effectiveThresholdAdaptationMixedFixture(t *testing.T) (*Mixed, MixedParameters, []float64, [][]float64) {
	t.Helper()
	a := math.Ldexp(1, 1023)
	config := MixedConfig{
		Nodes: 1, DT: math.Ln2, NodeRule: []uint8{RuleLIF},
		LIF: LIFRule{
			TauSyn: 1, ThetaMin: -a, ThetaMax: a, VReset: -math.MaxFloat64,
			Adaptation: LIFAdaptation{Enabled: true, TauAdapt: 1, Beta: math.MaxFloat64},
			Surrogate:  LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
		},
	}
	model, err := NewMixed(config)
	if err != nil {
		t.Fatal(err)
	}
	return model,
		MixedParameters{Bias: []float64{math.MaxFloat64}, LogTau: []float64{0}, ThetaRaw: []float64{1000}},
		[]float64{math.MaxFloat64}, effectiveThresholdZeroInputs(2)
}

func effectiveThresholdHomeostasisMixedFixture(t *testing.T) (*Mixed, MixedParameters, []float64, [][]float64) {
	t.Helper()
	a := math.Ldexp(1, 1023)
	hMax := math.Ldexp(3, 1022) // 1.5*A, finite and exactly representable.
	config := MixedConfig{
		Nodes: 1, DT: 1, NodeRule: []uint8{RuleLIF},
		LIF: LIFRule{
			TauSyn: 1, ThetaMin: -a, ThetaMax: a, VReset: -math.MaxFloat64,
			Homeostasis: &LIFHomeostasis{
				Enabled: true, TauRate: 1, TargetRate: 0, Eta: math.MaxFloat64, HMax: hMax,
			},
			Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
		},
	}
	model, err := NewMixed(config)
	if err != nil {
		t.Fatal(err)
	}
	return model,
		MixedParameters{Bias: []float64{hMax}, LogTau: []float64{0}, ThetaRaw: []float64{1000}},
		[]float64{hMax}, effectiveThresholdZeroInputs(2)
}

func effectiveThresholdZeroInputs(steps int) [][]float64 {
	inputs := make([][]float64, steps)
	for step := range inputs {
		inputs[step] = []float64{0}
	}
	return inputs
}

func effectiveThresholdCloneRows(rows [][]float64) [][]float64 {
	if rows == nil {
		return nil
	}
	cloned := make([][]float64, len(rows))
	for i := range rows {
		cloned[i] = slices.Clone(rows[i])
	}
	return cloned
}

func effectiveThresholdCloneLIFParameters(p LIFParameters) LIFParameters {
	p.Weights = slices.Clone(p.Weights)
	p.Bias = slices.Clone(p.Bias)
	p.LogTau = slices.Clone(p.LogTau)
	p.ThetaRaw = slices.Clone(p.ThetaRaw)
	return p
}

func effectiveThresholdCloneMixedParameters(p MixedParameters) MixedParameters {
	p.Weights = slices.Clone(p.Weights)
	p.Bias = slices.Clone(p.Bias)
	p.LogTau = slices.Clone(p.LogTau)
	p.ThetaRaw = slices.Clone(p.ThetaRaw)
	return p
}

func effectiveThresholdCloneLIFState(s LIFState) LIFState {
	s.Voltage = slices.Clone(s.Voltage)
	s.History = effectiveThresholdCloneRows(s.History)
	s.Adaptation = slices.Clone(s.Adaptation)
	s.Refractory = slices.Clone(s.Refractory)
	s.Rate = slices.Clone(s.Rate)
	s.Homeostasis = slices.Clone(s.Homeostasis)
	return s
}

func effectiveThresholdCloneMixedState(s MixedState) MixedState {
	s.Continuous = effectiveThresholdCloneState(s.Continuous)
	s.LIF = effectiveThresholdCloneLIFState(s.LIF)
	s.Index.ContinuousNodes = slices.Clone(s.Index.ContinuousNodes)
	s.Index.LIFNodes = slices.Clone(s.Index.LIFNodes)
	return s
}

func effectiveThresholdCloneState(s State) State {
	s.Voltage = slices.Clone(s.Voltage)
	s.History = effectiveThresholdCloneRows(s.History)
	return s
}

func effectiveThresholdRequireFirstStep(t *testing.T, name string, outputs, spikes [][]float64) {
	t.Helper()
	if len(outputs) != 1 || len(outputs[0]) != 1 || math.Float64bits(outputs[0][0]) != math.Float64bits(1) {
		t.Fatalf("%s outputs = %#v, want [[1]]", name, outputs)
	}
	if len(spikes) != 1 || len(spikes[0]) != 1 || math.Float64bits(spikes[0][0]) != math.Float64bits(1) {
		t.Fatalf("%s spikes = %#v, want [[1]]", name, spikes)
	}
}

func effectiveThresholdRequireLIFForwardFailure(t *testing.T, name string, model *LIF, p LIFParameters, initial []float64, inputs [][]float64) {
	t.Helper()
	callP := effectiveThresholdCloneLIFParameters(p)
	callInitial := append([]float64(nil), initial...)
	callInputs := effectiveThresholdCloneRows(inputs)
	wantP, wantInitial, wantInputs := effectiveThresholdCloneLIFParameters(callP), append([]float64(nil), callInitial...), effectiveThresholdCloneRows(callInputs)
	trace, err := model.Forward(context.Background(), callP, callInitial, callInputs)
	if err == nil {
		t.Fatalf("%s unexpectedly accepted an infinite effective threshold", name)
	}
	if trace != nil {
		t.Fatalf("%s returned a partial trace after error", name)
	}
	if !reflect.DeepEqual(callP, wantP) || !reflect.DeepEqual(callInitial, wantInitial) || !reflect.DeepEqual(callInputs, wantInputs) {
		t.Fatalf("%s mutated caller arguments after error", name)
	}
}

func effectiveThresholdRequireLIFRecomputeFailure(t *testing.T, name string, model *LIF, p LIFParameters, initial []float64, inputs, upstream [][]float64) {
	t.Helper()
	t.Run("ForwardRecompute", func(t *testing.T) {
		callP := effectiveThresholdCloneLIFParameters(p)
		callInitial := append([]float64(nil), initial...)
		callInputs := effectiveThresholdCloneRows(inputs)
		wantP, wantInitial, wantInputs := effectiveThresholdCloneLIFParameters(callP), append([]float64(nil), callInitial...), effectiveThresholdCloneRows(callInputs)
		outputs, err := model.ForwardRecompute(context.Background(), callP, callInitial, callInputs, 1)
		if err == nil {
			t.Fatalf("%s ForwardRecompute unexpectedly accepted an infinite effective threshold", name)
		}
		if outputs != nil {
			t.Fatalf("%s ForwardRecompute returned partial outputs after error", name)
		}
		if !reflect.DeepEqual(callP, wantP) || !reflect.DeepEqual(callInitial, wantInitial) || !reflect.DeepEqual(callInputs, wantInputs) {
			t.Fatalf("%s ForwardRecompute mutated caller arguments after error", name)
		}
	})
	t.Run("BackwardRecompute", func(t *testing.T) {
		callP := effectiveThresholdCloneLIFParameters(p)
		callInitial := append([]float64(nil), initial...)
		callInputs := effectiveThresholdCloneRows(inputs)
		callUpstream := effectiveThresholdCloneRows(upstream)
		wantP, wantInitial, wantInputs, wantUpstream := effectiveThresholdCloneLIFParameters(callP), append([]float64(nil), callInitial...), effectiveThresholdCloneRows(callInputs), effectiveThresholdCloneRows(callUpstream)
		gradient, err := model.BackwardRecompute(context.Background(), callP, callInitial, callInputs, callUpstream, 0, 1)
		if err == nil {
			t.Fatalf("%s BackwardRecompute unexpectedly accepted an infinite effective threshold", name)
		}
		if !reflect.DeepEqual(gradient, LIFGradient{}) {
			t.Fatalf("%s BackwardRecompute returned partial gradient %#v", name, gradient)
		}
		if !reflect.DeepEqual(callP, wantP) || !reflect.DeepEqual(callInitial, wantInitial) || !reflect.DeepEqual(callInputs, wantInputs) || !reflect.DeepEqual(callUpstream, wantUpstream) {
			t.Fatalf("%s BackwardRecompute mutated caller arguments after error", name)
		}
	})
}

func effectiveThresholdExerciseLIF(t *testing.T, name string, model *LIF, p LIFParameters, initial []float64, inputs [][]float64) {
	t.Helper()
	oneInputs := effectiveThresholdCloneRows(inputs[:1])
	t.Run("Forward", func(t *testing.T) {
		trace, err := model.Forward(context.Background(), effectiveThresholdCloneLIFParameters(p), append([]float64(nil), initial...), effectiveThresholdCloneRows(oneInputs))
		if err != nil {
			t.Fatalf("%s first-step Forward: %v", name, err)
		}
		effectiveThresholdRequireFirstStep(t, name+" first-step Forward", trace.Outputs(), trace.Spikes())
		if _, err := model.Backward(context.Background(), trace, [][]float64{{1}}, 0); err != nil {
			t.Fatalf("%s first-step Backward: %v", name, err)
		}
		effectiveThresholdRequireLIFForwardFailure(t, name+" Forward", model, p, initial, inputs)
	})
	t.Run("Recompute", func(t *testing.T) {
		oneOutputs, err := model.ForwardRecompute(context.Background(), effectiveThresholdCloneLIFParameters(p), append([]float64(nil), initial...), effectiveThresholdCloneRows(oneInputs), 1)
		if err != nil {
			t.Fatalf("%s first-step ForwardRecompute: %v", name, err)
		}
		if len(oneOutputs) != 1 || len(oneOutputs[0]) != 1 || math.Float64bits(oneOutputs[0][0]) != math.Float64bits(1) {
			t.Fatalf("%s first-step ForwardRecompute outputs = %#v, want [[1]]", name, oneOutputs)
		}
		if _, err := model.BackwardRecompute(context.Background(), effectiveThresholdCloneLIFParameters(p), append([]float64(nil), initial...), effectiveThresholdCloneRows(oneInputs), [][]float64{{1}}, 0, 1); err != nil {
			t.Fatalf("%s first-step BackwardRecompute: %v", name, err)
		}
		effectiveThresholdRequireLIFRecomputeFailure(t, name, model, p, initial, inputs, [][]float64{{1}, {0}})
	})
	t.Run("AdvanceBatch", func(t *testing.T) {
		state, err := model.NewState(slices.Clone(initial))
		if err != nil {
			t.Fatal(err)
		}
		beforeState := effectiveThresholdCloneLIFState(state)
		if !reflect.DeepEqual(state, beforeState) {
			t.Fatal("state copy changed nil or empty fields")
		}
		callP, callInputs := effectiveThresholdCloneLIFParameters(p), effectiveThresholdCloneRows(inputs)
		wantP, wantInputs := effectiveThresholdCloneLIFParameters(callP), effectiveThresholdCloneRows(callInputs)
		next, outputs, spikes, err := model.Advance(context.Background(), callP, state, callInputs)
		if err == nil || !reflect.DeepEqual(next, LIFState{}) || outputs != nil || spikes != nil {
			t.Fatalf("%s batch Advance accepted overflow or returned partial state: %#v %v %v %v", name, next, outputs, spikes, err)
		}
		if !reflect.DeepEqual(state, beforeState) || !reflect.DeepEqual(callP, wantP) || !reflect.DeepEqual(callInputs, wantInputs) {
			t.Fatalf("%s batch Advance changed caller state or arguments", name)
		}
	})
	t.Run("Advance", func(t *testing.T) {
		state, err := model.NewState(append([]float64(nil), initial...))
		if err != nil {
			t.Fatalf("%s NewState: %v", name, err)
		}
		first, outputs, spikes, err := model.Advance(context.Background(), effectiveThresholdCloneLIFParameters(p), state, effectiveThresholdCloneRows(oneInputs))
		if err != nil {
			t.Fatalf("%s first-step Advance: %v", name, err)
		}
		effectiveThresholdRequireFirstStep(t, name+" first-step Advance", outputs, spikes)
		if err := model.ValidateState(first); err != nil {
			t.Fatalf("%s first-step state validation: %v", name, err)
		}
		beforeState := effectiveThresholdCloneLIFState(first)
		callP := effectiveThresholdCloneLIFParameters(p)
		callInputs := effectiveThresholdCloneRows(inputs[1:])
		wantP, wantInputs := effectiveThresholdCloneLIFParameters(callP), effectiveThresholdCloneRows(callInputs)
		gotState, outputs, spikes, err := model.Advance(context.Background(), callP, first, callInputs)
		if err == nil {
			t.Fatalf("%s continuation Advance unexpectedly accepted an infinite effective threshold", name)
		}
		if !reflect.DeepEqual(gotState, LIFState{}) || outputs != nil || spikes != nil {
			t.Fatalf("%s continuation Advance returned partial result: state=%#v outputs=%#v spikes=%#v", name, gotState, outputs, spikes)
		}
		if !reflect.DeepEqual(first, beforeState) || !reflect.DeepEqual(callP, wantP) || !reflect.DeepEqual(callInputs, wantInputs) {
			t.Fatalf("%s continuation Advance mutated caller state or arguments", name)
		}
	})
}

func effectiveThresholdRequireMixedForwardFailure(t *testing.T, name string, model *Mixed, p MixedParameters, initial []float64, inputs [][]float64) {
	t.Helper()
	callP := effectiveThresholdCloneMixedParameters(p)
	callInitial := append([]float64(nil), initial...)
	callInputs := effectiveThresholdCloneRows(inputs)
	wantP, wantInitial, wantInputs := effectiveThresholdCloneMixedParameters(callP), append([]float64(nil), callInitial...), effectiveThresholdCloneRows(callInputs)
	trace, err := model.Forward(context.Background(), callP, callInitial, callInputs)
	if err == nil {
		t.Fatalf("%s unexpectedly accepted an infinite effective threshold", name)
	}
	if trace != nil {
		t.Fatalf("%s returned a partial trace after error", name)
	}
	if !reflect.DeepEqual(callP, wantP) || !reflect.DeepEqual(callInitial, wantInitial) || !reflect.DeepEqual(callInputs, wantInputs) {
		t.Fatalf("%s mutated caller arguments after error", name)
	}
}

func effectiveThresholdExerciseMixed(t *testing.T, name string, model *Mixed, p MixedParameters, initial []float64, inputs [][]float64) {
	t.Helper()
	oneInputs := effectiveThresholdCloneRows(inputs[:1])
	t.Run("Forward", func(t *testing.T) {
		trace, err := model.Forward(context.Background(), effectiveThresholdCloneMixedParameters(p), append([]float64(nil), initial...), effectiveThresholdCloneRows(oneInputs))
		if err != nil {
			t.Fatalf("%s first-step Forward: %v", name, err)
		}
		effectiveThresholdRequireFirstStep(t, name+" first-step Forward", trace.Outputs(), trace.Spikes())
		if _, err := model.Backward(context.Background(), trace, [][]float64{{1}}, 0); err != nil {
			t.Fatalf("%s first-step Backward: %v", name, err)
		}
		effectiveThresholdRequireMixedForwardFailure(t, name+" Forward", model, p, initial, inputs)
	})
	t.Run("AdvanceBatch", func(t *testing.T) {
		state, err := model.NewState(slices.Clone(initial))
		if err != nil {
			t.Fatal(err)
		}
		beforeState := effectiveThresholdCloneMixedState(state)
		if !reflect.DeepEqual(state, beforeState) {
			t.Fatal("state copy changed nil or empty fields")
		}
		callP, callInputs := effectiveThresholdCloneMixedParameters(p), effectiveThresholdCloneRows(inputs)
		wantP, wantInputs := effectiveThresholdCloneMixedParameters(callP), effectiveThresholdCloneRows(callInputs)
		next, outputs, spikes, err := model.Advance(context.Background(), callP, state, callInputs)
		if err == nil || !reflect.DeepEqual(next, MixedState{}) || outputs != nil || spikes != nil {
			t.Fatalf("%s batch Advance accepted overflow or returned partial state: %#v %v %v %v", name, next, outputs, spikes, err)
		}
		if !reflect.DeepEqual(state, beforeState) || !reflect.DeepEqual(callP, wantP) || !reflect.DeepEqual(callInputs, wantInputs) {
			t.Fatalf("%s batch Advance changed caller state or arguments", name)
		}
	})
	t.Run("Advance", func(t *testing.T) {
		state, err := model.NewState(append([]float64(nil), initial...))
		if err != nil {
			t.Fatalf("%s NewState: %v", name, err)
		}
		first, outputs, spikes, err := model.Advance(context.Background(), effectiveThresholdCloneMixedParameters(p), state, effectiveThresholdCloneRows(oneInputs))
		if err != nil {
			t.Fatalf("%s first-step Advance: %v", name, err)
		}
		effectiveThresholdRequireFirstStep(t, name+" first-step Advance", outputs, spikes)
		if err := model.ValidateState(first); err != nil {
			t.Fatalf("%s first-step state validation: %v", name, err)
		}
		beforeState := effectiveThresholdCloneMixedState(first)
		callP := effectiveThresholdCloneMixedParameters(p)
		callInputs := effectiveThresholdCloneRows(inputs[1:])
		wantP, wantInputs := effectiveThresholdCloneMixedParameters(callP), effectiveThresholdCloneRows(callInputs)
		gotState, outputs, spikes, err := model.Advance(context.Background(), callP, first, callInputs)
		if err == nil {
			t.Fatalf("%s continuation Advance unexpectedly accepted an infinite effective threshold", name)
		}
		if !reflect.DeepEqual(gotState, MixedState{}) || outputs != nil || spikes != nil {
			t.Fatalf("%s continuation Advance returned partial result: state=%#v outputs=%#v spikes=%#v", name, gotState, outputs, spikes)
		}
		if !reflect.DeepEqual(first, beforeState) || !reflect.DeepEqual(callP, wantP) || !reflect.DeepEqual(callInputs, wantInputs) {
			t.Fatalf("%s continuation Advance mutated caller state or arguments", name)
		}
	})
}

func TestEffectiveThresholdRangeLIFAdaptationRejectsOverflow(t *testing.T) {
	a := math.Ldexp(1, 1023)
	if !math.IsInf(a+math.MaxFloat64, 1) {
		t.Fatal("fixture arithmetic did not make base plus adaptation unrepresentable")
	}
	model, parameters, initial, inputs := effectiveThresholdAdaptationLIFFixture(t)
	effectiveThresholdExerciseLIF(t, "LIF adaptation", model, parameters, initial, inputs)
}

func TestEffectiveThresholdRangeLIFHomeostasisRejectsOverflow(t *testing.T) {
	a := math.Ldexp(1, 1023)
	hMax := math.Ldexp(3, 1022)
	if !math.IsInf(a+hMax, 1) {
		t.Fatal("fixture arithmetic did not make base plus homeostasis unrepresentable")
	}
	model, parameters, initial, inputs := effectiveThresholdHomeostasisLIFFixture(t)
	effectiveThresholdExerciseLIF(t, "LIF homeostasis", model, parameters, initial, inputs)
}

func TestEffectiveThresholdRangeMixedAdaptationRejectsOverflow(t *testing.T) {
	a := math.Ldexp(1, 1023)
	if !math.IsInf(a+math.MaxFloat64, 1) {
		t.Fatal("fixture arithmetic did not make base plus adaptation unrepresentable")
	}
	model, parameters, initial, inputs := effectiveThresholdAdaptationMixedFixture(t)
	effectiveThresholdExerciseMixed(t, "all-LIF Mixed adaptation", model, parameters, initial, inputs)
}

func TestEffectiveThresholdRangeMixedHomeostasisRejectsOverflow(t *testing.T) {
	a := math.Ldexp(1, 1023)
	hMax := math.Ldexp(3, 1022)
	if !math.IsInf(a+hMax, 1) {
		t.Fatal("fixture arithmetic did not make base plus homeostasis unrepresentable")
	}
	model, parameters, initial, inputs := effectiveThresholdHomeostasisMixedFixture(t)
	effectiveThresholdExerciseMixed(t, "all-LIF Mixed homeostasis", model, parameters, initial, inputs)
}
