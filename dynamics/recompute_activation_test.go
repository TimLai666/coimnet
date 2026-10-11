package dynamics_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
)

// recomputeActivationFixture deliberately combines repeated edges, self
// edges, isolated nodes, and both short and cross-segment delays. The inputs
// and upstream rows are generated from fixed arithmetic so the comparison is
// reproducible without depending on a random source.
func recomputeActivationFixture(t *testing.T, activation, delayMode string) (*dynamics.Continuous, dynamics.Parameters, []float64, [][]float64, [][]float64) {
	t.Helper()

	var sources, targets, delays []int
	addEdges := func(target int, sourceRows ...int) {
		for _, source := range sourceRows {
			sources = append(sources, source)
			targets = append(targets, target)
			delay := 0
			switch delayMode {
			case "zero":
			case "three":
				delay = 3
			case "mixed":
				if len(delays)%2 == 0 {
					delay = 3
				}
			default:
				t.Fatalf("unknown delay mode %q", delayMode)
			}
			delays = append(delays, delay)
		}
	}
	// Node 7 is isolated. Targets 0 through 4 have repeated and self edges;
	// node 6 is an input source without an incoming edge.
	addEdges(0, 0, 0, 1, 1, 2)
	addEdges(1, 1, 1, 2, 2, 3)
	addEdges(2, 2, 2, 3, 3, 4, 5)
	addEdges(3, 0, 1, 2, 3, 4, 5, 6, 0, 1, 2, 3)
	addEdges(4, 4, 4, 5, 6, 4)

	m, err := dynamics.NewContinuous(dynamics.Config{
		Nodes:      8,
		Sources:    sources,
		Targets:    targets,
		Delays:     delays,
		DT:         0.37,
		Activation: activation,
	})
	if err != nil {
		t.Fatal(err)
	}

	p := dynamics.Parameters{
		Weights: make([]float64, len(sources)),
		Bias:    make([]float64, 8),
		LogTau:  make([]float64, 8),
	}
	for i := range p.Weights {
		p.Weights[i] = 0.012 * float64((i%7)-3)
	}
	for i := range p.Bias {
		p.Bias[i] = 0.017 * float64((i%5)-2)
		p.LogTau[i] = 0.13 * float64((i%4)-1)
	}
	initial := []float64{0.21, -0.16, 0.08, -0.11, 0.14, -0.07, 0.04, math.Copysign(0, -1)}
	const steps = 13
	inputs := make([][]float64, steps)
	upstream := make([][]float64, steps)
	for step := 0; step < steps; step++ {
		inputs[step] = make([]float64, 8)
		upstream[step] = make([]float64, 8)
		for node := 0; node < 8; node++ {
			inputs[step][node] = 0.021 * float64((step+2*node)%9-4)
			upstream[step][node] = 0.031 * float64((3*step+node)%11-5)
		}
	}
	// Keep signed zero in the public inputs and upstream rows. This exercises
	// bitwise equality without changing the finite dynamics of the fixture.
	inputs[0][7] = math.Copysign(0, -1)
	upstream[1][6] = math.Copysign(0, -1)
	return m, p, initial, inputs, upstream
}

func recomputeActivationCloneRows(rows [][]float64) [][]float64 {
	clone := make([][]float64, len(rows))
	for i, row := range rows {
		clone[i] = append([]float64(nil), row...)
	}
	return clone
}

func recomputeActivationCloneParameters(p dynamics.Parameters) dynamics.Parameters {
	return dynamics.Parameters{
		Weights: append([]float64(nil), p.Weights...),
		Bias:    append([]float64(nil), p.Bias...),
		LogTau:  append([]float64(nil), p.LogTau...),
	}
}

func recomputeActivationBitsEqual(t *testing.T, label string, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s length %d, want %d", label, len(got), len(want))
	}
	for i := range want {
		gotBits, wantBits := math.Float64bits(got[i]), math.Float64bits(want[i])
		if gotBits != wantBits {
			t.Fatalf("%s[%d] got %.17g (%#x), want %.17g (%#x)", label, i, got[i], gotBits, want[i], wantBits)
		}
	}
}

func recomputeActivationRowsEqual(t *testing.T, label string, got, want [][]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s row count %d, want %d", label, len(got), len(want))
	}
	for row := range want {
		recomputeActivationBitsEqual(t, fmt.Sprintf("%s[%d]", label, row), got[row], want[row])
	}
}

func recomputeActivationGradientEqual(t *testing.T, got, want dynamics.Gradient) {
	t.Helper()
	recomputeActivationBitsEqual(t, "weights", got.Weights, want.Weights)
	recomputeActivationBitsEqual(t, "bias", got.Bias, want.Bias)
	recomputeActivationBitsEqual(t, "log_tau", got.LogTau, want.LogTau)
	recomputeActivationBitsEqual(t, "initial", got.Initial, want.Initial)
	recomputeActivationRowsEqual(t, "inputs", got.Inputs, want.Inputs)
}

type recomputeActivationCancelContext struct {
	context.Context
	cancel      context.CancelFunc
	cancelAfter int
	mu          sync.Mutex
	checks      int
	cancelled   bool
}

func newRecomputeActivationCancelContext(cancelAfter int) *recomputeActivationCancelContext {
	base, cancel := context.WithCancel(context.Background())
	return &recomputeActivationCancelContext{
		Context:     base,
		cancel:      cancel,
		cancelAfter: cancelAfter,
	}
}

func (c *recomputeActivationCancelContext) Err() error {
	c.mu.Lock()
	c.checks++
	shouldCancel := c.checks > c.cancelAfter && !c.cancelled
	if shouldCancel {
		c.cancelled = true
	}
	c.mu.Unlock()
	if shouldCancel {
		c.cancel()
	}
	return c.Context.Err()
}

func recomputeActivationGradientEmpty(g dynamics.Gradient) bool {
	return g.Weights == nil && g.Bias == nil && g.LogTau == nil && g.Inputs == nil && g.Initial == nil
}

// TestRecomputeActivationMatchesPublicReference compares the public
// ForwardRecompute and BackwardRecompute entrances with the independent
// Forward and Backward reference entrances. It covers both activations,
// delay-zero and delay-three graphs, delays crossing segment boundaries,
// repeated/self edges, an isolated node, nonzero initial state, signed zero,
// all requested segment sizes, and all requested truncation windows.
func TestRecomputeActivationMatchesPublicReference(t *testing.T) {
	for _, activation := range []string{"tanh", "softplus"} {
		for _, delayMode := range []string{"zero", "three", "mixed"} {
			t.Run(activation+"/"+delayMode, func(t *testing.T) {
				m, p, initial, inputs, upstream := recomputeActivationFixture(t, activation, delayMode)
				originalParameters := recomputeActivationCloneParameters(p)
				originalInitial := append([]float64(nil), initial...)
				originalInputs := recomputeActivationCloneRows(inputs)
				originalUpstream := recomputeActivationCloneRows(upstream)

				tr, err := m.Forward(context.Background(), p, initial, inputs)
				if err != nil {
					t.Fatalf("Forward: %v", err)
				}
				wantOutputs := tr.Outputs()
				wantGradients := make(map[int]dynamics.Gradient)
				for _, window := range []int{0, 3, 5} {
					want, err := m.Backward(context.Background(), tr, upstream, window)
					if err != nil {
						t.Fatalf("Backward window=%d: %v", window, err)
					}
					wantGradients[window] = want
				}

				for _, segment := range []int{1, 4, 7, 32} {
					gotOutputs, err := m.ForwardRecompute(context.Background(), p, initial, inputs, segment)
					if err != nil {
						t.Fatalf("ForwardRecompute segment=%d: %v", segment, err)
					}
					recomputeActivationRowsEqual(t, fmt.Sprintf("outputs segment=%d", segment), gotOutputs, wantOutputs)
					for _, window := range []int{0, 3, 5} {
						got, err := m.BackwardRecompute(context.Background(), p, initial, inputs, upstream, window, segment)
						if err != nil {
							t.Fatalf("BackwardRecompute segment=%d window=%d: %v", segment, window, err)
						}
						recomputeActivationGradientEqual(t, got, wantGradients[window])
					}
				}

				recomputeActivationBitsEqual(t, "parameters.weights", p.Weights, originalParameters.Weights)
				recomputeActivationBitsEqual(t, "parameters.bias", p.Bias, originalParameters.Bias)
				recomputeActivationBitsEqual(t, "parameters.log_tau", p.LogTau, originalParameters.LogTau)
				recomputeActivationBitsEqual(t, "initial", initial, originalInitial)
				recomputeActivationRowsEqual(t, "inputs", inputs, originalInputs)
				recomputeActivationRowsEqual(t, "upstream", upstream, originalUpstream)
			})
		}
	}
}

// TestRecomputeActivationZeroEdges exercises the no-edge and all-isolated
// boundary with signed zero, including the empty weight gradient.
func TestRecomputeActivationZeroEdges(t *testing.T) {
	for _, activation := range []string{"tanh", "softplus"} {
		t.Run(activation, func(t *testing.T) {
			m, err := dynamics.NewContinuous(dynamics.Config{Nodes: 4, DT: 0.4, Activation: activation})
			if err != nil {
				t.Fatal(err)
			}
			p := dynamics.Parameters{
				Bias:   []float64{0.03, -0.02, 0.01, -0.04},
				LogTau: []float64{0.1, -0.2, 0.3, -0.1},
			}
			initial := []float64{math.Copysign(0, -1), 0.2, -0.15, 0.07}
			inputs := [][]float64{
				{math.Copysign(0, -1), 0.1, -0.2, 0.3},
				{0.2, math.Copysign(0, -1), 0.1, -0.1},
				{-0.3, 0.2, math.Copysign(0, -1), 0.4},
				{0.1, -0.2, 0.3, math.Copysign(0, -1)},
				{0.0, 0.1, -0.1, 0.2},
			}
			upstream := [][]float64{
				{0.2, -0.1, 0.3, -0.2},
				{math.Copysign(0, -1), 0.4, -0.2, 0.1},
				{-0.3, 0.2, 0.1, -0.4},
				{0.1, -0.2, math.Copysign(0, -1), 0.3},
				{-0.2, 0.1, -0.3, 0.2},
			}
			tr, err := m.Forward(context.Background(), p, initial, inputs)
			if err != nil {
				t.Fatalf("Forward: %v", err)
			}
			wantOutputs := tr.Outputs()
			wantGradients := make(map[int]dynamics.Gradient)
			for _, window := range []int{0, 3, 5} {
				want, err := m.Backward(context.Background(), tr, upstream, window)
				if err != nil {
					t.Fatalf("Backward window=%d: %v", window, err)
				}
				wantGradients[window] = want
			}
			for _, segment := range []int{1, 4, 7} {
				gotOutputs, err := m.ForwardRecompute(context.Background(), p, initial, inputs, segment)
				if err != nil {
					t.Fatalf("ForwardRecompute segment=%d: %v", segment, err)
				}
				recomputeActivationRowsEqual(t, fmt.Sprintf("outputs segment=%d", segment), gotOutputs, wantOutputs)
				for _, window := range []int{0, 3, 5} {
					got, err := m.BackwardRecompute(context.Background(), p, initial, inputs, upstream, window, segment)
					if err != nil {
						t.Fatalf("BackwardRecompute segment=%d window=%d: %v", segment, window, err)
					}
					recomputeActivationGradientEqual(t, got, wantGradients[window])
				}
			}
			if len(wantGradients[0].Weights) != 0 {
				t.Fatalf("reference weight gradient length %d, want zero", len(wantGradients[0].Weights))
			}
		})
	}
}

// TestRecomputeActivationDeclaredTargetOrder pins the declared edge order
// when one target's edges are split by other targets. The large cancelling
// pair and smaller finite terms make a target-local accumulation order
// observable while keeping the complete reference trajectory finite.
func TestRecomputeActivationDeclaredTargetOrder(t *testing.T) {
	sources := []int{0, 1, 1, 2, 2, 0, 4, 3}
	targets := []int{2, 2, 1, 1, 2, 0, 1, 2}
	delays := []int{0, 3, 0, 3, 0, 3, 0, 3}
	parameters := dynamics.Parameters{
		Weights: []float64{1e16, -1e16, 0.25, -0.25, 0.75, 0.5, -0.125, -0.5},
		Bias:    []float64{0.01, -0.02, 0, 0.015, -0.01},
		LogTau:  []float64{0.1, -0.15, 0.2, -0.05, 0.12},
	}
	initial := []float64{0.125, 0.125, 0, -0.075, 0.2}
	inputs := [][]float64{
		{0.02, 0.02, 0, 0.01, -0.02},
		{-0.03, -0.03, 0.01, -0.02, 0.03},
		{0.04, 0.04, -0.015, 0.03, -0.01},
		{-0.01, -0.01, 0.02, -0.04, 0.02},
		{0.03, 0.03, -0.01, 0.02, -0.03},
		{-0.02, -0.02, 0.015, -0.01, 0.01},
		{0.01, 0.01, -0.02, 0.03, -0.02},
		{-0.04, -0.04, 0.01, -0.03, 0.04},
		{0.025, 0.025, -0.005, 0.01, -0.015},
		{-0.015, -0.015, 0.005, -0.02, 0.02},
		{0.035, 0.035, -0.01, 0.02, -0.025},
	}
	upstream := [][]float64{
		{0.11, -0.07, 0.13, -0.05, 0.09},
		{-0.08, 0.12, -0.04, 0.1, -0.06},
		{0.07, -0.1, 0.05, -0.09, 0.08},
		{-0.12, 0.06, -0.08, 0.04, -0.1},
		{0.09, -0.05, 0.11, -0.07, 0.03},
		{-0.06, 0.1, -0.09, 0.08, -0.04},
		{0.05, -0.11, 0.07, -0.03, 0.12},
		{-0.1, 0.04, -0.06, 0.09, -0.08},
		{0.08, -0.02, 0.1, -0.12, 0.06},
		{-0.04, 0.09, -0.05, 0.07, -0.11},
		{0.1, -0.08, 0.04, -0.06, 0.05},
	}

	for _, activation := range []string{"tanh", "softplus"} {
		t.Run(activation, func(t *testing.T) {
			m, err := dynamics.NewContinuous(dynamics.Config{
				Nodes:      5,
				Sources:    append([]int(nil), sources...),
				Targets:    append([]int(nil), targets...),
				Delays:     append([]int(nil), delays...),
				DT:         0.4,
				Activation: activation,
			})
			if err != nil {
				t.Fatal(err)
			}
			p := recomputeActivationCloneParameters(parameters)
			initialCopy := append([]float64(nil), initial...)
			inputsCopy := recomputeActivationCloneRows(inputs)
			upstreamCopy := recomputeActivationCloneRows(upstream)
			tr, err := m.Forward(context.Background(), p, initialCopy, inputsCopy)
			if err != nil {
				t.Fatalf("Forward: %v", err)
			}
			wantOutputs := tr.Outputs()
			wantGradients := make(map[int]dynamics.Gradient)
			for _, window := range []int{0, 3, 5} {
				want, err := m.Backward(context.Background(), tr, upstreamCopy, window)
				if err != nil {
					t.Fatalf("Backward window=%d: %v", window, err)
				}
				wantGradients[window] = want
			}

			for row, values := range wantOutputs {
				for column, value := range values {
					if math.IsNaN(value) || math.IsInf(value, 0) {
						t.Fatalf("reference output[%d][%d] is non-finite: %g", row, column, value)
					}
				}
			}
			for window, gradient := range wantGradients {
				for name, values := range map[string][]float64{
					"weights": gradient.Weights,
					"bias":    gradient.Bias,
					"log_tau": gradient.LogTau,
					"initial": gradient.Initial,
				} {
					for index, value := range values {
						if math.IsNaN(value) || math.IsInf(value, 0) {
							t.Fatalf("reference window=%d %s[%d] is non-finite: %g", window, name, index, value)
						}
					}
				}
				for row, values := range gradient.Inputs {
					for column, value := range values {
						if math.IsNaN(value) || math.IsInf(value, 0) {
							t.Fatalf("reference window=%d inputs[%d][%d] is non-finite: %g", window, row, column, value)
						}
					}
				}
			}

			for _, segment := range []int{1, 4, 7, 32} {
				gotOutputs, err := m.ForwardRecompute(context.Background(), p, initialCopy, inputsCopy, segment)
				if err != nil {
					t.Fatalf("ForwardRecompute segment=%d: %v", segment, err)
				}
				recomputeActivationRowsEqual(t, fmt.Sprintf("outputs segment=%d", segment), gotOutputs, wantOutputs)
				for _, window := range []int{0, 3, 5} {
					got, err := m.BackwardRecompute(context.Background(), p, initialCopy, inputsCopy, upstreamCopy, window, segment)
					if err != nil {
						t.Fatalf("BackwardRecompute segment=%d window=%d: %v", segment, window, err)
					}
					recomputeActivationGradientEqual(t, got, wantGradients[window])
				}
			}

			recomputeActivationBitsEqual(t, "weights", p.Weights, parameters.Weights)
			recomputeActivationBitsEqual(t, "bias", p.Bias, parameters.Bias)
			recomputeActivationBitsEqual(t, "log_tau", p.LogTau, parameters.LogTau)
			recomputeActivationBitsEqual(t, "initial", initialCopy, initial)
			recomputeActivationRowsEqual(t, "inputs", inputsCopy, inputs)
			recomputeActivationRowsEqual(t, "upstream", upstreamCopy, upstream)
		})
	}
}

// TestRecomputeActivationMaxIntBoundaries covers the integer limits that
// should still describe a finite small sequence. The delay test uses a
// declared delay larger than the whole sequence, so every edge must safely
// read the initial output prefix.
func TestRecomputeActivationMaxIntBoundaries(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	m, p, initial, inputs, upstream := recomputeActivationFixture(t, "softplus", "mixed")
	tr, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	wantOutputs := tr.Outputs()
	want, err := m.Backward(context.Background(), tr, upstream, 0)
	if err != nil {
		t.Fatalf("Backward: %v", err)
	}
	got, err := m.BackwardRecompute(context.Background(), p, initial, inputs, upstream, 0, maxInt)
	if err != nil {
		t.Fatalf("BackwardRecompute max segment: %v", err)
	}
	recomputeActivationGradientEqual(t, got, want)
	gotOutputs, err := m.ForwardRecompute(context.Background(), p, initial, inputs, maxInt)
	if err != nil {
		t.Fatalf("ForwardRecompute max segment: %v", err)
	}
	recomputeActivationRowsEqual(t, "max segment outputs", gotOutputs, wantOutputs)

	largeDelayModel, err := dynamics.NewContinuous(dynamics.Config{
		Nodes:      2,
		Sources:    []int{0, 1, 1},
		Targets:    []int{1, 0, 1},
		Delays:     []int{maxInt, maxInt, maxInt},
		DT:         0.5,
		Activation: "tanh",
	})
	if err != nil {
		t.Fatal(err)
	}
	largeDelayParameters := dynamics.Parameters{
		Weights: []float64{0.2, -0.15, 0.1},
		Bias:    []float64{0.04, -0.03},
		LogTau:  []float64{0.1, -0.2},
	}
	largeDelayInitial := []float64{0.25, -0.2}
	largeDelayInputs := [][]float64{{0.1, -0.1}, {-0.2, 0.2}, {0.3, -0.3}, {-0.1, 0.1}, {0.2, -0.2}}
	largeDelayUpstream := [][]float64{{0.2, -0.1}, {-0.3, 0.4}, {0.1, 0.2}, {-0.2, 0.3}, {0.4, -0.2}}
	largeDelayTrace, err := largeDelayModel.Forward(context.Background(), largeDelayParameters, largeDelayInitial, largeDelayInputs)
	if err != nil {
		t.Fatalf("large-delay Forward: %v", err)
	}
	largeDelayWantOutputs := largeDelayTrace.Outputs()
	largeDelayWant, err := largeDelayModel.Backward(context.Background(), largeDelayTrace, largeDelayUpstream, 0)
	if err != nil {
		t.Fatalf("large-delay Backward: %v", err)
	}
	for _, segment := range []int{1, maxInt} {
		outputs, err := largeDelayModel.ForwardRecompute(context.Background(), largeDelayParameters, largeDelayInitial, largeDelayInputs, segment)
		if err != nil {
			t.Fatalf("large-delay ForwardRecompute segment=%d: %v", segment, err)
		}
		recomputeActivationRowsEqual(t, fmt.Sprintf("large-delay outputs segment=%d", segment), outputs, largeDelayWantOutputs)
		gradient, err := largeDelayModel.BackwardRecompute(context.Background(), largeDelayParameters, largeDelayInitial, largeDelayInputs, largeDelayUpstream, 0, segment)
		if err != nil {
			t.Fatalf("large-delay BackwardRecompute segment=%d: %v", segment, err)
		}
		recomputeActivationGradientEqual(t, gradient, largeDelayWant)
	}
}

// TestRecomputeActivationCancellationAndLateNonFinite verifies that a
// cancellation observed inside a segment and a finite-input overflow on a
// later step return no partial public result and preserve caller-owned data.
func TestRecomputeActivationCancellationAndLateNonFinite(t *testing.T) {
	m, p, initial, inputs, upstream := recomputeActivationFixture(t, "tanh", "mixed")
	parametersBefore := recomputeActivationCloneParameters(p)
	initialBefore := append([]float64(nil), initial...)
	inputsBefore := recomputeActivationCloneRows(inputs)
	upstreamBefore := recomputeActivationCloneRows(upstream)
	forwardCtx := newRecomputeActivationCancelContext(16)
	outputs, err := m.ForwardRecompute(forwardCtx, p, initial, inputs, 4)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ForwardRecompute cancellation error=%v, want context.Canceled", err)
	}
	if outputs != nil {
		t.Fatalf("cancelled ForwardRecompute returned %d rows", len(outputs))
	}
	backwardCtx := newRecomputeActivationCancelContext(16)
	gradient, err := m.BackwardRecompute(backwardCtx, p, initial, inputs, upstream, 0, 4)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("BackwardRecompute cancellation error=%v, want context.Canceled", err)
	}
	if !recomputeActivationGradientEmpty(gradient) {
		t.Fatalf("cancelled BackwardRecompute returned partial gradient: %+v", gradient)
	}
	recomputeActivationBitsEqual(t, "cancel parameters.weights", p.Weights, parametersBefore.Weights)
	recomputeActivationBitsEqual(t, "cancel parameters.bias", p.Bias, parametersBefore.Bias)
	recomputeActivationBitsEqual(t, "cancel parameters.log_tau", p.LogTau, parametersBefore.LogTau)
	recomputeActivationBitsEqual(t, "cancel initial", initial, initialBefore)
	recomputeActivationRowsEqual(t, "cancel inputs", inputs, inputsBefore)
	recomputeActivationRowsEqual(t, "cancel upstream", upstream, upstreamBefore)

	overflowModel, err := dynamics.NewContinuous(dynamics.Config{
		Nodes:      1,
		Sources:    []int{0, 0},
		Targets:    []int{0, 0},
		Delays:     []int{0, 0},
		DT:         1,
		Activation: "tanh",
	})
	if err != nil {
		t.Fatal(err)
	}
	overflowParameters := dynamics.Parameters{
		Weights: []float64{math.MaxFloat64, math.MaxFloat64},
		Bias:    []float64{0},
		LogTau:  []float64{0},
	}
	overflowInitial := []float64{0}
	overflowInputs := [][]float64{{1}, {0}}
	overflowUpstream := [][]float64{{0}, {1}}
	overflowParametersBefore := recomputeActivationCloneParameters(overflowParameters)
	overflowInitialBefore := append([]float64(nil), overflowInitial...)
	overflowInputsBefore := recomputeActivationCloneRows(overflowInputs)
	overflowUpstreamBefore := recomputeActivationCloneRows(overflowUpstream)
	overflowOutputs, err := overflowModel.ForwardRecompute(context.Background(), overflowParameters, overflowInitial, overflowInputs, 1)
	if err == nil {
		t.Fatal("late overflow ForwardRecompute unexpectedly succeeded")
	}
	if overflowOutputs != nil {
		t.Fatalf("late overflow ForwardRecompute returned %d rows", len(overflowOutputs))
	}
	overflowGradient, err := overflowModel.BackwardRecompute(context.Background(), overflowParameters, overflowInitial, overflowInputs, overflowUpstream, 0, 1)
	if err == nil {
		t.Fatal("late overflow BackwardRecompute unexpectedly succeeded")
	}
	if !recomputeActivationGradientEmpty(overflowGradient) {
		t.Fatalf("late overflow BackwardRecompute returned partial gradient: %+v", overflowGradient)
	}
	recomputeActivationBitsEqual(t, "overflow parameters.weights", overflowParameters.Weights, overflowParametersBefore.Weights)
	recomputeActivationBitsEqual(t, "overflow parameters.bias", overflowParameters.Bias, overflowParametersBefore.Bias)
	recomputeActivationBitsEqual(t, "overflow parameters.log_tau", overflowParameters.LogTau, overflowParametersBefore.LogTau)
	recomputeActivationBitsEqual(t, "overflow initial", overflowInitial, overflowInitialBefore)
	recomputeActivationRowsEqual(t, "overflow inputs", overflowInputs, overflowInputsBefore)
	recomputeActivationRowsEqual(t, "overflow upstream", overflowUpstream, overflowUpstreamBefore)
}

// TestRecomputeActivationOutputOwnershipAndConcurrency ensures one successful
// result can be changed by its caller, and that repeated or concurrent calls
// remain independent and bit-identical to the public reference.
func TestRecomputeActivationOutputOwnershipAndConcurrency(t *testing.T) {
	m, p, initial, inputs, upstream := recomputeActivationFixture(t, "tanh", "mixed")
	tr, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	wantOutputs := tr.Outputs()
	want, err := m.Backward(context.Background(), tr, upstream, 0)
	if err != nil {
		t.Fatalf("Backward: %v", err)
	}
	upstreamBefore := recomputeActivationCloneRows(upstream)

	owned, err := m.ForwardRecompute(context.Background(), p, initial, inputs, 4)
	if err != nil {
		t.Fatalf("ForwardRecompute: %v", err)
	}
	owned[0][0] = math.NaN()
	owned[1] = nil
	owned[len(owned)-1][len(owned[len(owned)-1])-1] = -999
	repeated, err := m.ForwardRecompute(context.Background(), p, initial, inputs, 4)
	if err != nil {
		t.Fatalf("repeated ForwardRecompute: %v", err)
	}
	recomputeActivationRowsEqual(t, "repeated outputs", repeated, wantOutputs)

	ownedGradient, err := m.BackwardRecompute(context.Background(), p, initial, inputs, upstream, 0, 4)
	if err != nil {
		t.Fatalf("owned BackwardRecompute: %v", err)
	}
	ownedGradient.Weights[0] = math.NaN()
	ownedGradient.Bias[0] = math.NaN()
	ownedGradient.LogTau[0] = math.NaN()
	ownedGradient.Initial[0] = math.NaN()
	ownedGradient.Inputs[0][0] = math.NaN()
	ownedGradient.Inputs[1] = nil
	repeatedGradient, err := m.BackwardRecompute(context.Background(), p, initial, inputs, upstream, 0, 4)
	if err != nil {
		t.Fatalf("repeated BackwardRecompute: %v", err)
	}
	recomputeActivationGradientEqual(t, repeatedGradient, want)
	recomputeActivationRowsEqual(t, "repeated upstream", upstream, upstreamBefore)

	const callers = 4
	type concurrentResult struct {
		outputs  [][]float64
		gradient dynamics.Gradient
		err      error
	}
	ready := make(chan struct{}, callers)
	barrier := make(chan struct{})
	results := make(chan concurrentResult, callers)
	for i := 0; i < callers; i++ {
		go func() {
			ready <- struct{}{}
			<-barrier
			outputs, err := m.ForwardRecompute(context.Background(), p, initial, inputs, 1)
			if err != nil {
				results <- concurrentResult{err: fmt.Errorf("concurrent ForwardRecompute: %w", err)}
				return
			}
			gradient, err := m.BackwardRecompute(context.Background(), p, initial, inputs, upstream, 0, 4)
			if err != nil {
				results <- concurrentResult{outputs: outputs, err: fmt.Errorf("concurrent BackwardRecompute: %w", err)}
				return
			}
			results <- concurrentResult{outputs: outputs, gradient: gradient}
		}()
	}
	for i := 0; i < callers; i++ {
		<-ready
	}
	for i := 0; i < callers; i++ {
		barrier <- struct{}{}
	}
	for i := 0; i < callers; i++ {
		result := <-results
		if result.err != nil {
			t.Error(result.err)
			continue
		}
		recomputeActivationRowsEqual(t, "concurrent outputs", result.outputs, wantOutputs)
		recomputeActivationGradientEqual(t, result.gradient, want)
	}
	recomputeActivationRowsEqual(t, "concurrent upstream", upstream, upstreamBefore)
}
