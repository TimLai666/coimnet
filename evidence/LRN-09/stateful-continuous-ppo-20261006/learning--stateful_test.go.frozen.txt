package learning_test

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

func TestLossGradientFromStateMatchesFiniteDifferences(t *testing.T) {
	c, p, options, prefix, input, upstream := statefulLearningFixture(t)
	ind, err := learning.NewIndividual(c, p, options, []float64{.2, -.3})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ind.Advance(context.Background(), prefix); err != nil {
		t.Fatalf("prefix Advance: %v", err)
	}
	snapshot := ind.Snapshot()
	n, err := learning.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	got, err := n.LossGradientFromState(context.Background(), snapshot.Parameters, snapshot.Neural, input, upstream, 0)
	if err != nil {
		t.Fatalf("LossGradientFromState: %v", err)
	}

	objective := func(parameters learning.Parameters, inputs [][]float64) float64 {
		candidate := snapshot
		candidate.Parameters = cloneLearningParametersStateful(parameters)
		at, err := learning.RestoreIndividual(candidate)
		if err != nil {
			t.Fatalf("RestoreIndividual for finite difference: %v", err)
		}
		outputs, err := at.Advance(context.Background(), inputs)
		if err != nil {
			t.Fatalf("Advance for finite difference: %v", err)
		}
		return weightedLearningOutputs(outputs, upstream)
	}

	const eps = 2e-3
	check := func(label string, values, gradients []float64, perturb func(int, float64) float64) {
		for i, old := range values {
			plus := perturb(i, old+eps)
			minus := perturb(i, old-eps)
			fd := (plus - minus) / (2 * eps)
			if math.Abs(gradients[i]-fd) > 1e-5+1e-4*math.Abs(fd) {
				t.Fatalf("%s[%d] = %.17g, finite difference %.17g", label, i, gradients[i], fd)
			}
		}
	}
	check("core.weights", p.Core.Weights, got.Core.Weights, func(i int, value float64) float64 {
		q := cloneLearningParametersStateful(p)
		q.Core.Weights[i] = value
		return objective(q, input)
	})
	check("core.bias", p.Core.Bias, got.Core.Bias, func(i int, value float64) float64 {
		q := cloneLearningParametersStateful(p)
		q.Core.Bias[i] = value
		return objective(q, input)
	})
	check("core.log_tau", p.Core.LogTau, got.Core.LogTau, func(i int, value float64) float64 {
		q := cloneLearningParametersStateful(p)
		q.Core.LogTau[i] = value
		return objective(q, input)
	})
	check("encoder", p.Encoder, got.Encoder, func(i int, value float64) float64 {
		q := cloneLearningParametersStateful(p)
		q.Encoder[i] = value
		return objective(q, input)
	})
	check("readout", p.Readout, got.Readout, func(i int, value float64) float64 {
		q := cloneLearningParametersStateful(p)
		q.Readout[i] = value
		return objective(q, input)
	})
	for step := range input {
		for i, old := range input[step] {
			plus := cloneRowsLearningStateful(input)
			minus := cloneRowsLearningStateful(input)
			plus[step][i], minus[step][i] = old+eps, old-eps
			fd := (objective(p, plus) - objective(p, minus)) / (2 * eps)
			if math.Abs(got.Inputs[step][i]-fd) > 1e-5+1e-4*math.Abs(fd) {
				t.Fatalf("inputs[%d][%d] = %.17g, finite difference %.17g", step, i, got.Inputs[step][i], fd)
			}
		}
	}

	zero, err := learning.NewIndividual(c, p, options, make([]float64, c.Dynamics.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	zeroState := zero.Snapshot().Neural
	fromZero, err := n.LossGradientFromState(context.Background(), p, zeroState, input, upstream, 0)
	if err != nil {
		t.Fatalf("zero LossGradientFromState: %v", err)
	}
	fromLegacy, err := n.LossGradientFrom(context.Background(), p, input, upstream, 0)
	if err != nil {
		t.Fatalf("LossGradientFrom: %v", err)
	}
	if !gradientEqualExceptInitialStateful(fromZero, fromLegacy) {
		t.Fatalf("fresh zero state diverged from legacy gradient outside Core.Initial:\nstateful=%#v\nlegacy=%#v", fromZero, fromLegacy)
	}
}

func TestLossGradientFromStateRejectsInvalidStateAndUnsupportedModes(t *testing.T) {
	c, p, options, prefix, input, upstream := statefulLearningFixture(t)
	ind, err := learning.NewIndividual(c, p, options, []float64{.2, -.3})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ind.Advance(context.Background(), prefix); err != nil {
		t.Fatal(err)
	}
	base := ind.Snapshot()
	n, err := learning.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		ctx      context.Context
		state    func(*learning.NeuralState)
		params   func(*learning.Parameters)
		inputs   func(*[][]float64)
		upstream func(*[][]float64)
		window   int
		wantErr  error
	}{
		{name: "nil context", state: func(*learning.NeuralState) {}, params: func(*learning.Parameters) {}, inputs: func(*[][]float64) {}, upstream: func(*[][]float64) {}},
		{name: "canceled context", ctx: canceledLearningContext(), state: func(*learning.NeuralState) {}, params: func(*learning.Parameters) {}, inputs: func(*[][]float64) {}, upstream: func(*[][]float64) {}, wantErr: context.Canceled},
		{name: "empty input", ctx: context.Background(), state: func(*learning.NeuralState) {}, params: func(*learning.Parameters) {}, inputs: func(in *[][]float64) { *in = nil }, upstream: func(*[][]float64) {}},
		{name: "bad core", ctx: context.Background(), state: func(s *learning.NeuralState) { s.Core = learning.NeuralCoreLIF }, params: func(*learning.Parameters) {}, inputs: func(*[][]float64) {}, upstream: func(*[][]float64) {}},
		{name: "missing history", ctx: context.Background(), state: func(s *learning.NeuralState) { s.Continuous.History = nil }, params: func(*learning.Parameters) {}, inputs: func(*[][]float64) {}, upstream: func(*[][]float64) {}},
		{name: "state NaN", ctx: context.Background(), state: func(s *learning.NeuralState) { s.Continuous.Voltage[0] = math.NaN() }, params: func(*learning.Parameters) {}, inputs: func(*[][]float64) {}, upstream: func(*[][]float64) {}},
		{name: "overflow steps", ctx: context.Background(), state: func(s *learning.NeuralState) { s.Continuous.Steps = math.MaxUint64 }, params: func(*learning.Parameters) {}, inputs: func(*[][]float64) {}, upstream: func(*[][]float64) {}},
		{name: "parameter NaN", ctx: context.Background(), state: func(*learning.NeuralState) {}, params: func(p *learning.Parameters) { p.Core.Bias[0] = math.NaN() }, inputs: func(*[][]float64) {}, upstream: func(*[][]float64) {}},
		{name: "input NaN", ctx: context.Background(), state: func(*learning.NeuralState) {}, params: func(*learning.Parameters) {}, inputs: func(in *[][]float64) { (*in)[1][0] = math.Inf(1) }, upstream: func(*[][]float64) {}},
		{name: "upstream shape", ctx: context.Background(), state: func(*learning.NeuralState) {}, params: func(*learning.Parameters) {}, inputs: func(*[][]float64) {}, upstream: func(up *[][]float64) { *up = (*up)[:2] }},
		{name: "upstream NaN", ctx: context.Background(), state: func(*learning.NeuralState) {}, params: func(*learning.Parameters) {}, inputs: func(*[][]float64) {}, upstream: func(up *[][]float64) { (*up)[1][0] = math.NaN() }},
		{name: "negative window", ctx: context.Background(), state: func(*learning.NeuralState) {}, params: func(*learning.Parameters) {}, inputs: func(*[][]float64) {}, upstream: func(*[][]float64) {}, window: -1},
		{name: "theta on continuous", ctx: context.Background(), state: func(*learning.NeuralState) {}, params: func(p *learning.Parameters) { p.ThetaRaw = []float64{.1, .2} }, inputs: func(*[][]float64) {}, upstream: func(*[][]float64) {}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.ctx
			if tc.name == "nil context" {
				ctx = nil
			}
			state := cloneNeuralStateful(base.Neural)
			params := cloneLearningParametersStateful(base.Parameters)
			in := cloneRowsLearningStateful(input)
			up := cloneRowsLearningStateful(upstream)
			tc.state(&state)
			tc.params(&params)
			tc.inputs(&in)
			tc.upstream(&up)
			beforeState := cloneNeuralStateful(state)
			beforeParams := cloneLearningParametersStateful(params)
			beforeInput := cloneRowsLearningStateful(in)
			beforeUpstream := cloneRowsLearningStateful(up)
			got, err := n.LossGradientFromState(ctx, params, state, in, up, tc.window)
			if err == nil || !reflect.DeepEqual(got, learning.Gradient{}) {
				t.Fatalf("gradient=%#v err=%v, want empty error result", got, err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("error=%v, want errors.Is(..., %v)", err, tc.wantErr)
			}
			if !neuralStateBitsEqualStateful(state, beforeState) || !learningParametersBitsEqualStateful(params, beforeParams) || !rowsBitsEqualStateful(in, beforeInput) || !rowsBitsEqualStateful(up, beforeUpstream) {
				t.Fatal("invalid LossGradientFromState call mutated caller state, parameters, input, or upstream")
			}
		})
	}

	lf, lp, lo, lprefix, _, _ := lifStatefulFixture(t)
	lifInd, err := learning.NewIndividual(lf, lp, lo, make([]float64, lf.LIF.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lifInd.Advance(context.Background(), lprefix); err != nil {
		t.Fatal(err)
	}
	lifSnapshot := lifInd.Snapshot()
	lifNetwork, err := learning.NewNetwork(lf)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := lifNetwork.LossGradientFromState(context.Background(), lifSnapshot.Parameters, lifSnapshot.Neural, [][]float64{{.2}}, [][]float64{{.1}}, 0); err == nil || !reflect.DeepEqual(got, learning.Gradient{}) {
		t.Fatalf("LIF result=%#v err=%v, want explicit unsupported error", got, err)
	}
}

func TestStepFromStateMatchesIndependentFirstAdamUpdate(t *testing.T) {
	c, p, options, prefix, input, upstream := statefulLearningFixture(t)
	options.LearningRate = .037
	options.Epsilon = .011
	options.WeightDecay = .013
	options.ClipNorm = 0
	ind, err := learning.NewIndividual(c, p, options, []float64{.2, -.3})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ind.Advance(context.Background(), prefix); err != nil {
		t.Fatal(err)
	}
	base := ind.Snapshot()
	n, err := learning.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	g, err := n.LossGradientFromState(context.Background(), base.Parameters, base.Neural, input, upstream, 0)
	if err != nil {
		t.Fatal(err)
	}
	tr, err := learning.NewTrainer(c, base.Parameters, options)
	if err != nil {
		t.Fatal(err)
	}
	beforeInput := cloneRowsLearningStateful(input)
	beforeUpstream := cloneRowsLearningStateful(upstream)
	result, err := tr.StepFromState(context.Background(), base.Neural, input, upstream)
	if err != nil {
		t.Fatalf("StepFromState: %v", err)
	}
	if result.Loss != 0 || result.LossKnown || !result.Applied || result.Updates != 1 || result.Accumulated != 0 {
		t.Fatalf("unexpected StepResult: %+v", result)
	}
	if !reflect.DeepEqual(input, beforeInput) || !reflect.DeepEqual(upstream, beforeUpstream) {
		t.Fatal("StepFromState mutated caller inputs")
	}

	want := cloneLearningParametersStateful(base.Parameters)
	applyFirstAdamWStateful(&want, g, options)
	got := tr.Snapshot().Parameters
	if !learningParametersCloseStateful(got, want, 2e-12) {
		t.Fatalf("StepFromState parameters differ from independent AdamW reference:\n got %#v\nwant %#v", got, want)
	}
	wantOptimizer := firstAdamStateful(g, options)
	if !adamStateEqualStateful(tr.Snapshot().Optimizer, wantOptimizer) {
		t.Fatalf("StepFromState optimizer differs from independent first AdamW reference:\n got %#v\nwant %#v", tr.Snapshot().Optimizer, wantOptimizer)
	}
}

func TestStepFromStateRejectsRecomputeWithoutChangingTrainer(t *testing.T) {
	c, p, options, prefix, input, upstream := statefulLearningFixture(t)
	options.Recompute = &learning.Recompute{SegmentSteps: 1}
	ind, err := learning.NewIndividual(c, p, options, []float64{.2, -.3})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ind.Advance(context.Background(), prefix); err != nil {
		t.Fatal(err)
	}
	state := ind.Snapshot().Neural
	tr, err := learning.NewTrainer(c, p, options)
	if err != nil {
		t.Fatal(err)
	}
	before := tr.Snapshot()
	if _, err := tr.StepFromState(context.Background(), state, input, upstream); err == nil {
		t.Fatal("StepFromState accepted Recompute")
	}
	if !reflect.DeepEqual(tr.Snapshot(), before) {
		t.Fatal("recompute refusal changed trainer")
	}
}

func statefulLearningFixture(t *testing.T) (learning.Config, learning.Parameters, learning.Options, [][]float64, [][]float64, [][]float64) {
	t.Helper()
	c := learning.Config{
		Dynamics:  dynamics.Config{Nodes: 2, Sources: []int{0, 1, 1}, Targets: []int{1, 0, 1}, Delays: []int{0, 2, 0}, DT: .3, Activation: "tanh"},
		InputSize: 1, OutputSize: 1, ReadoutNodes: []int{1}, ReadoutEveryStep: true,
	}
	p := learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{.2, -.1, .15}, Bias: []float64{.1, -.2}, LogTau: []float64{.2, -.1}},
		Encoder: []float64{.5, .1}, Readout: []float64{.8},
	}
	o := learning.DefaultOptions()
	o.Trainable = learning.Trainable{Encoder: true, Weights: true, Bias: true, Tau: true, Readout: true}
	prefix := [][]float64{{.1}, {-.2}, {.3}}
	input := [][]float64{{-.3}, {.1}, {.4}, {-.2}}
	upstream := [][]float64{{.3}, {-.4}, {.2}, {.6}}
	return c, p, o, prefix, input, upstream
}

func lifStatefulFixture(t *testing.T) (learning.Config, learning.Parameters, learning.Options, [][]float64, [][]float64, [][]float64) {
	t.Helper()
	c := lifConfig()
	o := learning.DefaultOptions()
	o.Trainable = learning.Trainable{Weights: true, Bias: true, Tau: true, Theta: true, Encoder: true, Readout: true}
	return c, lifParameters(), o, [][]float64{{1}, {0}}, [][]float64{{.2}}, [][]float64{{.1}}
}

func cloneLearningParametersStateful(p learning.Parameters) learning.Parameters {
	return learning.Parameters{
		Core:     dynamics.Parameters{Weights: append([]float64(nil), p.Core.Weights...), Bias: append([]float64(nil), p.Core.Bias...), LogTau: append([]float64(nil), p.Core.LogTau...)},
		ThetaRaw: append([]float64(nil), p.ThetaRaw...), Encoder: append([]float64(nil), p.Encoder...), Readout: append([]float64(nil), p.Readout...),
	}
}

func cloneRowsLearningStateful(rows [][]float64) [][]float64 {
	out := make([][]float64, len(rows))
	for i := range rows {
		out[i] = append([]float64(nil), rows[i]...)
	}
	return out
}

func cloneNeuralStateful(s learning.NeuralState) learning.NeuralState {
	out := s
	if s.Continuous != nil {
		state := *s.Continuous
		state.Voltage = append([]float64(nil), state.Voltage...)
		state.History = cloneRowsLearningStateful(state.History)
		out.Continuous = &state
	}
	if s.LIF != nil {
		state := *s.LIF
		state.Voltage = append([]float64(nil), state.Voltage...)
		state.History = cloneRowsLearningStateful(state.History)
		state.Adaptation = append([]float64(nil), state.Adaptation...)
		state.Refractory = append([]int(nil), state.Refractory...)
		state.Rate = append([]float64(nil), state.Rate...)
		state.Homeostasis = append([]float64(nil), state.Homeostasis...)
		out.LIF = &state
	}
	return out
}

func weightedLearningOutputs(outputs, upstream [][]float64) float64 {
	var sum float64
	for t := range outputs {
		for i := range outputs[t] {
			sum += outputs[t][i] * upstream[t][i]
		}
	}
	return sum
}

func applyFirstAdamWStateful(p *learning.Parameters, g learning.Gradient, o learning.Options) {
	parameterSlices := [][]float64{p.Core.Weights, p.Core.Bias, p.Core.LogTau, p.ThetaRaw, p.Encoder, p.Readout}
	gradientSlices := [][]float64{g.Core.Weights, g.Core.Bias, g.Core.LogTau, g.ThetaRaw, g.Encoder, g.Readout}
	for group := range parameterSlices {
		for i, d := range gradientSlices[group] {
			parameterSlices[group][i] = parameterSlices[group][i]*(1-o.LearningRate*o.WeightDecay) - o.LearningRate*d/(math.Abs(d)+o.Epsilon)
		}
	}
}

func firstAdamStateful(g learning.Gradient, o learning.Options) learning.AdamState {
	groups := [][]float64{g.Core.Weights, g.Core.Bias, g.Core.LogTau, g.ThetaRaw, g.Encoder, g.Readout}
	count := 0
	for _, group := range groups {
		count += len(group)
	}
	state := learning.AdamState{First: make([]float64, count), Second: make([]float64, count), Steps: make([]uint64, count)}
	index := 0
	for _, group := range groups {
		for _, d := range group {
			state.First[index] = (1 - o.Beta1) * d
			state.Second[index] = (1 - o.Beta2) * d * d
			state.Steps[index] = 1
			index++
		}
	}
	return state
}

func adamStateEqualStateful(a, b learning.AdamState) bool {
	return floatBitsEqualStateful(a.First, b.First) && floatBitsEqualStateful(a.Second, b.Second) && reflect.DeepEqual(a.Steps, b.Steps)
}

func learningParametersCloseStateful(a, b learning.Parameters, tolerance float64) bool {
	groupsA := [][]float64{a.Core.Weights, a.Core.Bias, a.Core.LogTau, a.ThetaRaw, a.Encoder, a.Readout}
	groupsB := [][]float64{b.Core.Weights, b.Core.Bias, b.Core.LogTau, b.ThetaRaw, b.Encoder, b.Readout}
	if len(groupsA) != len(groupsB) {
		return false
	}
	for group := range groupsA {
		if len(groupsA[group]) != len(groupsB[group]) {
			return false
		}
		for i := range groupsA[group] {
			if math.Abs(groupsA[group][i]-groupsB[group][i]) > tolerance {
				return false
			}
		}
	}
	return true
}

func gradientEqualExceptInitialStateful(a, b learning.Gradient) bool {
	return floatBitsEqualStateful(a.Core.Weights, b.Core.Weights) &&
		floatBitsEqualStateful(a.Core.Bias, b.Core.Bias) &&
		floatBitsEqualStateful(a.Core.LogTau, b.Core.LogTau) &&
		rowsBitsEqualStateful(a.Core.Inputs, b.Core.Inputs) &&
		floatBitsEqualStateful(a.ThetaRaw, b.ThetaRaw) &&
		floatBitsEqualStateful(a.Encoder, b.Encoder) &&
		floatBitsEqualStateful(a.Readout, b.Readout) &&
		rowsBitsEqualStateful(a.Inputs, b.Inputs)
}

func learningParametersBitsEqualStateful(a, b learning.Parameters) bool {
	return floatBitsEqualStateful(a.Core.Weights, b.Core.Weights) &&
		floatBitsEqualStateful(a.Core.Bias, b.Core.Bias) &&
		floatBitsEqualStateful(a.Core.LogTau, b.Core.LogTau) &&
		floatBitsEqualStateful(a.ThetaRaw, b.ThetaRaw) &&
		floatBitsEqualStateful(a.Encoder, b.Encoder) &&
		floatBitsEqualStateful(a.Readout, b.Readout)
}

func neuralStateBitsEqualStateful(a, b learning.NeuralState) bool {
	if a.Core != b.Core || (a.Continuous == nil) != (b.Continuous == nil) || (a.LIF == nil) != (b.LIF == nil) || (a.Mixed == nil) != (b.Mixed == nil) {
		return false
	}
	if a.Continuous != nil {
		if a.Continuous.SchemaVersion != b.Continuous.SchemaVersion || a.Continuous.ConfigHash != b.Continuous.ConfigHash || a.Continuous.Steps != b.Continuous.Steps {
			return false
		}
		if !floatBitsEqualStateful(a.Continuous.Voltage, b.Continuous.Voltage) || !rowsBitsEqualStateful(a.Continuous.History, b.Continuous.History) {
			return false
		}
	}
	return reflect.DeepEqual(a.LIF, b.LIF) && reflect.DeepEqual(a.Mixed, b.Mixed)
}

func floatBitsEqualStateful(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Float64bits(a[i]) != math.Float64bits(b[i]) {
			return false
		}
	}
	return true
}

func rowsBitsEqualStateful(a, b [][]float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !floatBitsEqualStateful(a[i], b[i]) {
			return false
		}
	}
	return true
}

func canceledLearningContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
