package dynamics_test

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
)

func TestContinuousForwardFromStateMatchesAdvance(t *testing.T) {
	for _, activation := range []string{"tanh", "softplus"} {
		for _, workers := range []int{1, 2} {
			for _, prefixSteps := range []int{0, 1, 3} {
				name := fmt.Sprintf("%s/workers=%d/prefix=%d", activation, workers, prefixSteps)
				t.Run(name, func(t *testing.T) {
					m, p, initial, prefix, suffix := stateForwardFixture(t, activation, workers)
					state, err := m.NewState(initial)
					if err != nil {
						t.Fatal(err)
					}
					if prefixSteps > 0 {
						state, _, err = m.Advance(context.Background(), p, state, prefix[:prefixSteps])
						if err != nil {
							t.Fatalf("prefix Advance: %v", err)
						}
					}
					before := cloneStatefulState(state)

					trace, err := m.ForwardFromState(context.Background(), p, state, suffix)
					if err != nil {
						t.Fatalf("ForwardFromState: %v", err)
					}
					next, wantOutputs, err := m.Advance(context.Background(), p, state, suffix)
					if err != nil {
						t.Fatalf("Advance: %v", err)
					}
					if got := trace.Outputs(); !reflect.DeepEqual(got, wantOutputs) {
						t.Fatalf("outputs differ from Advance:\n got %#v\nwant %#v", got, wantOutputs)
					}
					if !reflect.DeepEqual(trace.FinalVoltage(), next.Voltage) {
						t.Fatalf("final voltage differs from Advance: got %#v want %#v", trace.FinalVoltage(), next.Voltage)
					}
					if !reflect.DeepEqual(state, before) {
						t.Fatal("ForwardFromState mutated the caller's state")
					}

					// A trace owns the state, parameters, and input values used by its
					// forward pass. Mutating the caller after the call must not alter
					// either its outputs or its reverse pass.
					wantTraceOutputs := trace.Outputs()
					wantGradient, err := m.Backward(context.Background(), trace, constantUpstream(len(suffix), m.Config().Nodes), 0)
					if err != nil {
						t.Fatalf("Backward before mutation: %v", err)
					}
					state.Voltage[0] = 99
					state.History[0][0] = -99
					p.Weights[0] = 99
					suffix[0][0] = 99
					if !reflect.DeepEqual(trace.Outputs(), wantTraceOutputs) {
						t.Fatal("trace outputs alias caller buffers")
					}
					gotGradient, err := m.Backward(context.Background(), trace, constantUpstream(len(suffix), m.Config().Nodes), 0)
					if err != nil {
						t.Fatalf("Backward after mutation: %v", err)
					}
					if !reflect.DeepEqual(gotGradient, wantGradient) {
						t.Fatalf("trace reverse pass changed after caller mutation:\n got %#v\nwant %#v", gotGradient, wantGradient)
					}
				})
			}
		}
	}
}

func TestContinuousForwardFromStateUsesSavedHistoryWithinFourULP(t *testing.T) {
	m, err := dynamics.NewContinuous(dynamics.Config{
		Nodes: 1, Sources: []int{0}, Targets: []int{0}, Delays: []int{0}, DT: .5, Activation: "softplus",
	})
	if err != nil {
		t.Fatal(err)
	}
	p := dynamics.Parameters{Weights: []float64{1e12}, Bias: []float64{0}, LogTau: []float64{0}}
	state, err := m.NewState([]float64{.7})
	if err != nil {
		t.Fatal(err)
	}
	for range 4 {
		state.History[0][0] = math.Nextafter(state.History[0][0], math.Inf(1))
	}
	if err := m.ValidateState(state); err != nil {
		t.Fatalf("four-ULP saved history rejected: %v", err)
	}
	saved := cloneStatefulState(state)
	trace, err := m.ForwardFromState(context.Background(), p, state, [][]float64{{0}})
	if err != nil {
		t.Fatalf("ForwardFromState: %v", err)
	}
	_, want, err := m.Advance(context.Background(), p, state, [][]float64{{0}})
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if !reflect.DeepEqual(trace.Outputs(), want) {
		t.Fatalf("ForwardFromState did not use the saved history:\n got %#v\nwant %#v", trace.Outputs(), want)
	}
	if !sameStatefulState(state, saved) {
		t.Fatal("ForwardFromState rewrote the saved history")
	}
}

func TestContinuousForwardFromStateGradientMatchesFixedHistoryFiniteDifferences(t *testing.T) {
	for _, activation := range []string{"tanh", "softplus"} {
		t.Run(activation, func(t *testing.T) {
			m, p, initial, prefix, suffix := stateForwardFixture(t, activation, 1)
			state, err := m.NewState(initial)
			if err != nil {
				t.Fatal(err)
			}
			state, _, err = m.Advance(context.Background(), p, state, prefix[:1])
			if err != nil {
				t.Fatal(err)
			}
			upstream := [][]float64{{.3, -.2}, {-.4, .5}, {.2, .6}, {-.1, .7}}
			trace, err := m.ForwardFromState(context.Background(), p, state, suffix)
			if err != nil {
				t.Fatalf("ForwardFromState: %v", err)
			}
			got, err := m.Backward(context.Background(), trace, upstream, 0)
			if err != nil {
				t.Fatalf("Backward: %v", err)
			}

			objective := func(parameters dynamics.Parameters, inputs [][]float64) float64 {
				tr, err := m.ForwardFromState(context.Background(), parameters, state, inputs)
				if err != nil {
					t.Fatalf("finite-difference ForwardFromState: %v", err)
				}
				return weightedOutputs(tr.Outputs(), upstream)
			}
			const eps = 1e-6
			check := func(label string, values, gradient []float64, perturb func(int, float64) float64) {
				for i, old := range values {
					plus := perturb(i, old+eps)
					minus := perturb(i, old-eps)
					fd := (plus - minus) / (2 * eps)
					if math.Abs(gradient[i]-fd) > 2e-6+3e-4*math.Abs(fd) {
						t.Fatalf("%s[%d] = %.17g, finite difference %.17g", label, i, gradient[i], fd)
					}
				}
			}
			check("weights", p.Weights, got.Weights, func(i int, value float64) float64 {
				q := cloneDynamicsParameters(p)
				q.Weights[i] = value
				return objective(q, suffix)
			})
			check("bias", p.Bias, got.Bias, func(i int, value float64) float64 {
				q := cloneDynamicsParameters(p)
				q.Bias[i] = value
				return objective(q, suffix)
			})
			check("log_tau", p.LogTau, got.LogTau, func(i int, value float64) float64 {
				q := cloneDynamicsParameters(p)
				q.LogTau[i] = value
				return objective(q, suffix)
			})
			for step := range suffix {
				for i, old := range suffix[step] {
					plusInputs := cloneRowsStateful(suffix)
					minusInputs := cloneRowsStateful(suffix)
					plusInputs[step][i], minusInputs[step][i] = old+eps, old-eps
					fd := (objective(p, plusInputs) - objective(p, minusInputs)) / (2 * eps)
					if math.Abs(got.Inputs[step][i]-fd) > 2e-6+3e-4*math.Abs(fd) {
						t.Fatalf("inputs[%d][%d] = %.17g, finite difference %.17g", step, i, got.Inputs[step][i], fd)
					}
				}
			}

			// Public State validation intentionally ties the latest saved history
			// row to Voltage. To test the stateful Initial derivative without
			// changing the saved history, use the independent fixed-history
			// reference below and perturb only the segment-start voltage.
			fixedObjective := func(voltage []float64) float64 {
				return fixedHistoryObjective(m.Config(), p, state, voltage, suffix, upstream)
			}
			for i, old := range state.Voltage {
				plus := append([]float64(nil), state.Voltage...)
				minus := append([]float64(nil), state.Voltage...)
				plus[i], minus[i] = old+eps, old-eps
				fd := (fixedObjective(plus) - fixedObjective(minus)) / (2 * eps)
				if math.Abs(got.Initial[i]-fd) > 2e-6+3e-4*math.Abs(fd) {
					t.Fatalf("initial[%d] = %.17g, fixed-history finite difference %.17g", i, got.Initial[i], fd)
				}
			}
		})
	}
}

func TestContinuousForwardFromStateTruncationDoesNotCrossSegmentStart(t *testing.T) {
	m, err := dynamics.NewContinuous(dynamics.Config{
		Nodes: 1, Sources: []int{0}, Targets: []int{0}, Delays: []int{2}, DT: .4, Activation: "tanh",
	})
	if err != nil {
		t.Fatal(err)
	}
	p := dynamics.Parameters{Weights: []float64{.7}, Bias: []float64{.1}, LogTau: []float64{.2}}
	state, err := m.NewState([]float64{.2})
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = m.Advance(context.Background(), p, state, [][]float64{{.5}, {-.2}, {.1}})
	if err != nil {
		t.Fatal(err)
	}
	inputs := [][]float64{{.3}, {-.1}, {.2}, {0}}
	upstream := [][]float64{{0}, {0}, {0}, {1}}
	trace, err := m.ForwardFromState(context.Background(), p, state, inputs)
	if err != nil {
		t.Fatal(err)
	}
	full, err := m.Backward(context.Background(), trace, upstream, 0)
	if err != nil {
		t.Fatal(err)
	}
	cut, err := m.Backward(context.Background(), trace, upstream, 2)
	if err != nil {
		t.Fatal(err)
	}
	if full.Inputs[1][0] == 0 {
		t.Fatal("fixture has no delayed path across the truncation boundary")
	}
	if cut.Inputs[1][0] != 0 || cut.Initial[0] != 0 {
		t.Fatalf("truncated reverse crossed segment start: inputs=%v initial=%v", cut.Inputs, cut.Initial)
	}
	if cut.Weights[0] == 0 {
		t.Fatal("truncation removed the direct in-segment delayed-edge gradient")
	}
}

func TestContinuousForwardFromStateRejectsInvalidInputAtomically(t *testing.T) {
	m, p, initial, prefix, suffix := stateForwardFixture(t, "tanh", 1)
	state, err := m.NewState(initial)
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = m.Advance(context.Background(), p, state, prefix[:3])
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		ctx         context.Context
		mutateState func(*dynamics.State)
		mutateP     func(*dynamics.Parameters)
		mutateInput func([][]float64)
	}{
		{name: "nil context", mutateState: func(*dynamics.State) {}, mutateP: func(*dynamics.Parameters) {}, mutateInput: func([][]float64) {}},
		{name: "canceled context", ctx: canceledContext(), mutateState: func(*dynamics.State) {}, mutateP: func(*dynamics.Parameters) {}, mutateInput: func([][]float64) {}},
		{name: "empty sequence", ctx: context.Background(), mutateState: func(*dynamics.State) {}, mutateP: func(*dynamics.Parameters) {}},
		{name: "schema", ctx: context.Background(), mutateState: func(s *dynamics.State) { s.SchemaVersion = "wrong" }, mutateP: func(*dynamics.Parameters) {}, mutateInput: func([][]float64) {}},
		{name: "history", ctx: context.Background(), mutateState: func(s *dynamics.State) { s.History = nil }, mutateP: func(*dynamics.Parameters) {}, mutateInput: func([][]float64) {}},
		{name: "nonfinite state", ctx: context.Background(), mutateState: func(s *dynamics.State) { s.Voltage[0] = math.NaN() }, mutateP: func(*dynamics.Parameters) {}, mutateInput: func([][]float64) {}},
		{name: "overflow steps", ctx: context.Background(), mutateState: func(s *dynamics.State) { s.Steps = math.MaxUint64 }, mutateP: func(*dynamics.Parameters) {}, mutateInput: func([][]float64) {}},
		{name: "parameter NaN", ctx: context.Background(), mutateState: func(*dynamics.State) {}, mutateP: func(p *dynamics.Parameters) { p.LogTau[0] = math.NaN() }, mutateInput: func([][]float64) {}},
		{name: "input NaN", ctx: context.Background(), mutateState: func(*dynamics.State) {}, mutateP: func(*dynamics.Parameters) {}, mutateInput: func(in [][]float64) { in[1][0] = math.Inf(1) }},
		{name: "input shape", ctx: context.Background(), mutateState: func(*dynamics.State) {}, mutateP: func(*dynamics.Parameters) {}, mutateInput: func(in [][]float64) { in[0] = in[0][:1] }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.ctx
			if ctx == nil && tc.name != "nil context" {
				ctx = context.Background()
			}
			badState := cloneStatefulState(state)
			badP := cloneDynamicsParameters(p)
			badInputs := cloneRowsStateful(suffix)
			tc.mutateState(&badState)
			tc.mutateP(&badP)
			if tc.mutateInput != nil {
				tc.mutateInput(badInputs)
			}
			beforeState := cloneStatefulState(badState)
			beforeP := cloneDynamicsParameters(badP)
			beforeInputs := cloneRowsStateful(badInputs)
			var input [][]float64 = badInputs
			if tc.name == "empty sequence" {
				input = nil
			}
			trace, err := m.ForwardFromState(ctx, badP, badState, input)
			if err == nil || trace != nil {
				t.Fatalf("result = %#v, err = %v, want empty error result", trace, err)
			}
			if !sameStatefulState(badState, beforeState) || !sameDynamicsParameters(badP, beforeP) || !sameRowsStateful(badInputs, beforeInputs) {
				t.Fatal("failed ForwardFromState changed caller inputs")
			}
		})
	}
}

func stateForwardFixture(t *testing.T, activation string, workers int) (*dynamics.Continuous, dynamics.Parameters, []float64, [][]float64, [][]float64) {
	t.Helper()
	m, err := dynamics.NewContinuous(dynamics.Config{
		Nodes: 2, Sources: []int{0, 1, 1}, Targets: []int{1, 0, 1}, Delays: []int{0, 2, 0}, DT: .3, Activation: activation, Workers: workers,
	})
	if err != nil {
		t.Fatal(err)
	}
	p := dynamics.Parameters{Weights: []float64{.2, -.1, .15}, Bias: []float64{.1, -.2}, LogTau: []float64{.2, -.1}}
	initial := []float64{.2, -.3}
	prefix := [][]float64{{.1, .3}, {-.2, .4}, {.5, -.1}, {.2, .2}}
	suffix := [][]float64{{-.3, .1}, {.1, -.5}, {.4, .2}, {-.2, .3}}
	return m, p, initial, prefix, suffix
}

func cloneStatefulState(s dynamics.State) dynamics.State {
	out := s
	out.Voltage = append([]float64(nil), s.Voltage...)
	out.History = cloneRowsStateful(s.History)
	return out
}

func cloneDynamicsParameters(p dynamics.Parameters) dynamics.Parameters {
	return dynamics.Parameters{Weights: append([]float64(nil), p.Weights...), Bias: append([]float64(nil), p.Bias...), LogTau: append([]float64(nil), p.LogTau...)}
}

func cloneRowsStateful(rows [][]float64) [][]float64 {
	out := make([][]float64, len(rows))
	for i := range rows {
		out[i] = append([]float64(nil), rows[i]...)
	}
	return out
}

func sameStatefulState(a, b dynamics.State) bool {
	if a.SchemaVersion != b.SchemaVersion || a.ConfigHash != b.ConfigHash || a.Steps != b.Steps || !sameFloatSlice(a.Voltage, b.Voltage) {
		return false
	}
	return sameRowsStateful(a.History, b.History)
}

func sameDynamicsParameters(a, b dynamics.Parameters) bool {
	return sameFloatSlice(a.Weights, b.Weights) && sameFloatSlice(a.Bias, b.Bias) && sameFloatSlice(a.LogTau, b.LogTau)
}

func sameRowsStateful(a, b [][]float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !sameFloatSlice(a[i], b[i]) {
			return false
		}
	}
	return true
}

func sameFloatSlice(a, b []float64) bool {
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

func constantUpstream(steps, nodes int) [][]float64 {
	out := make([][]float64, steps)
	for i := range out {
		out[i] = make([]float64, nodes)
		for j := range out[i] {
			out[i][j] = float64(i+1) * float64(j+1) * .1
		}
	}
	return out
}

func weightedOutputs(outputs, upstream [][]float64) float64 {
	var sum float64
	for t := range outputs {
		for i := range outputs[t] {
			sum += outputs[t][i] * upstream[t][i]
		}
	}
	return sum
}

func fixedHistoryObjective(c dynamics.Config, p dynamics.Parameters, state dynamics.State, voltage []float64, inputs, upstream [][]float64) float64 {
	maxDelay := 0
	for _, delay := range c.Delays {
		if delay > maxDelay {
			maxDelay = delay
		}
	}
	start := uint64(0)
	if state.Steps > uint64(maxDelay) {
		start = state.Steps - uint64(maxDelay)
	}
	history := cloneRowsStateful(state.History)
	outputs := make([][]float64, len(inputs))
	lambda := make([]float64, c.Nodes)
	alpha := make([]float64, c.Nodes)
	for i, raw := range p.LogTau {
		tau := math.Exp(raw)
		lambda[i] = math.Exp(-c.DT / tau)
		alpha[i] = -math.Expm1(-c.DT / tau)
	}
	for t, in := range inputs {
		step := state.Steps + uint64(t)
		drive := append([]float64(nil), in...)
		for e, source := range c.Sources {
			past := uint64(0)
			if uint64(c.Delays[e]) < step {
				past = step - uint64(c.Delays[e])
			}
			var sourceOutput float64
			if past <= state.Steps {
				idx := int(past - start)
				if idx < 0 {
					idx = 0
				}
				if idx >= len(history) {
					idx = len(history) - 1
				}
				sourceOutput = history[idx][source]
			} else {
				sourceOutput = outputs[int(past-state.Steps)-1][source]
			}
			drive[c.Targets[e]] += p.Weights[e] * sourceOutput
		}
		next := make([]float64, c.Nodes)
		out := make([]float64, c.Nodes)
		for i := range next {
			drive[i] += p.Bias[i]
			next[i] = lambda[i]*voltage[i] + alpha[i]*drive[i]
			out[i] = statefulActivation(c.Activation, next[i])
		}
		outputs[t] = out
		voltage = next
	}
	return weightedOutputs(outputs, upstream)
}

func statefulActivation(name string, value float64) float64 {
	if name == "tanh" {
		return math.Tanh(value)
	}
	return math.Max(value, 0) + math.Log1p(math.Exp(-math.Abs(value)))
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
