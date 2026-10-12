package dynamics_test

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
)

// coupledRowsClone is deliberately kept in this test package. The mocks own
// every row they observe so a production implementation cannot accidentally
// use a caller-owned slice as chemical history.
func coupledRowsClone(rows [][]float64) [][]float64 {
	result := make([][]float64, len(rows))
	for i, row := range rows {
		result[i] = append([]float64(nil), row...)
	}
	return result
}

func coupledVectorClone(values []float64) []float64 {
	return append([]float64(nil), values...)
}

func coupledParametersClone(p dynamics.Parameters) dynamics.Parameters {
	return dynamics.Parameters{
		Weights: coupledVectorClone(p.Weights),
		Bias:    coupledVectorClone(p.Bias),
		LogTau:  coupledVectorClone(p.LogTau),
	}
}

func coupledRowsBitsEqual(t *testing.T, name string, got, want [][]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s has %d rows, want %d", name, len(got), len(want))
	}
	for r := range want {
		if len(got[r]) != len(want[r]) {
			t.Fatalf("%s row %d has %d values, want %d", name, r, len(got[r]), len(want[r]))
		}
		for i := range want[r] {
			if math.Float64bits(got[r][i]) != math.Float64bits(want[r][i]) {
				t.Fatalf("%s[%d][%d] = %v (%#x), want %v (%#x)", name, r, i,
					got[r][i], math.Float64bits(got[r][i]), want[r][i], math.Float64bits(want[r][i]))
			}
		}
	}
}

func coupledVectorBitsEqual(t *testing.T, name string, got, want []float64) {
	t.Helper()
	coupledRowsBitsEqual(t, name, [][]float64{got}, [][]float64{want})
}

func coupledGradientsBitsEqual(t *testing.T, name string, got, want dynamics.Gradient) {
	t.Helper()
	coupledVectorBitsEqual(t, name+" weights", got.Weights, want.Weights)
	coupledVectorBitsEqual(t, name+" bias", got.Bias, want.Bias)
	coupledVectorBitsEqual(t, name+" log_tau", got.LogTau, want.LogTau)
	coupledRowsBitsEqual(t, name+" inputs", got.Inputs, want.Inputs)
	coupledVectorBitsEqual(t, name+" initial", got.Initial, want.Initial)
}

func coupledClose(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 3e-8*(1+math.Abs(want)) {
		t.Fatalf("%s = %.16g, want %.16g", name, got, want)
	}
}

func coupledFDClose(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 4e-5*(1+math.Abs(want)) {
		t.Fatalf("%s = %.16g, central difference %.16g", name, got, want)
	}
}

func coupledGradientIsZero(g dynamics.Gradient) bool {
	return g.Weights == nil && g.Bias == nil && g.LogTau == nil && g.Inputs == nil && g.Initial == nil
}

// recordingSequence supplies a fixed one-row modulation and records both the
// forward observations and every reverse modulation gradient. It is used for
// hand-calculated checks where no chemistry package should be involved.
type recordingSequence struct {
	nodes          int
	gain           float64
	offset         float64
	forwardRows    [][]float64
	forwardTs      []int
	reverses       []*recordingReverse
	forwardErr     error
	badMod         *dynamics.Modulation
	returnNil      bool
	zeroThreshold  bool
	reverseFactory func(totalsteps, window int) *recordingReverse
}

func (s *recordingSequence) Forward(ctx context.Context, step int, previousRawOutput []float64) (*dynamics.Modulation, error) {
	if ctx == nil {
		return nil, errors.New("recording sequence received nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.forwardTs = append(s.forwardTs, step)
	s.forwardRows = append(s.forwardRows, coupledVectorClone(previousRawOutput))
	if s.forwardErr != nil {
		return nil, s.forwardErr
	}
	if s.badMod != nil {
		return s.badMod, nil
	}
	if s.returnNil {
		return nil, nil
	}
	gain, offset := make([]float64, s.nodes), make([]float64, s.nodes)
	for i := range gain {
		gain[i], offset[i] = s.gain, s.offset
	}
	mod := &dynamics.Modulation{Gain: [][]float64{gain}, Offset: [][]float64{offset}}
	if s.zeroThreshold {
		mod.Threshold = [][]float64{make([]float64, s.nodes)}
	}
	return mod, nil
}

func (s *recordingSequence) NewReverse(ctx context.Context, totalsteps, window int) (dynamics.ModulationReverse, error) {
	if ctx == nil {
		return nil, errors.New("recording sequence received nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r := &recordingReverse{nodes: s.nodes, totalsteps: totalsteps, window: window, errAt: -1}
	if s.reverseFactory != nil {
		r = s.reverseFactory(totalsteps, window)
	}
	s.reverses = append(s.reverses, r)
	return r, nil
}

type recordingReverse struct {
	nodes      int
	totalsteps int
	window     int
	calls      []int
	gradients  []dynamics.ModulationGradient
	returnRows [][]dynamics.ActivityGradient
	responses  map[int][]dynamics.ActivityGradient
	errAt      int
}

func (r *recordingReverse) BackwardStep(ctx context.Context, step int, gradient dynamics.ModulationGradient) ([]dynamics.ActivityGradient, error) {
	if ctx == nil {
		return nil, errors.New("recording reverse received nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.errAt >= 0 && step == r.errAt {
		return nil, errors.New("recording reverse failure")
	}
	r.calls = append(r.calls, step)
	r.gradients = append(r.gradients, dynamics.ModulationGradient{
		Gain:      coupledVectorClone(gradient.Gain),
		Offset:    coupledVectorClone(gradient.Offset),
		Threshold: coupledVectorClone(gradient.Threshold),
	})
	rows := cloneActivityGradients(r.responses[step])
	r.returnRows = append(r.returnRows, cloneActivityGradients(rows))
	return rows, nil
}

func cloneActivityGradients(rows []dynamics.ActivityGradient) []dynamics.ActivityGradient {
	result := make([]dynamics.ActivityGradient, len(rows))
	for i, row := range rows {
		result[i] = dynamics.ActivityGradient{Step: row.Step, Values: coupledVectorClone(row.Values)}
	}
	return result
}

func newRecordingSequence(nodes int, gain, offset float64) *recordingSequence {
	return &recordingSequence{nodes: nodes, gain: gain, offset: offset}
}

func coupledOneNodeModel(t *testing.T) *dynamics.Continuous {
	t.Helper()
	m, err := dynamics.NewContinuous(dynamics.Config{Nodes: 1, DT: 1, Activation: "tanh"})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func coupledFixture(t *testing.T, activation string) (*dynamics.Continuous, dynamics.Parameters, []float64, [][]float64, [][]float64) {
	t.Helper()
	m, err := dynamics.NewContinuous(dynamics.Config{
		Nodes:      2,
		Sources:    []int{0},
		Targets:    []int{1},
		Delays:     []int{1},
		DT:         0.35,
		Activation: activation,
	})
	if err != nil {
		t.Fatal(err)
	}
	p := dynamics.Parameters{
		Weights: []float64{0.27},
		Bias:    []float64{0.11, -0.16},
		LogTau:  []float64{0.18, -0.12},
	}
	initial := []float64{0.21, -0.29}
	inputs := [][]float64{{0.4, -0.1}, {-0.2, 0.3}, {0.1, -0.25}, {0.2, 0.15}}
	upstream := [][]float64{{0.13, -0.09}, {-0.2, 0.11}, {0.17, 0.07}, {0.31, -0.27}}
	return m, p, initial, inputs, upstream
}

// historySequence makes every modulation row depend on the current and the
// preceding raw output rows. Its reverse computes the same derivatives from
// the recorded modulation adjoints and scatters both rows, which gives the
// finite-difference test an independent activity-feedback oracle.
type historySequence struct {
	forwardRows [][]float64
	forwardTs   []int
	reverses    []*historyReverse
}

func (s *historySequence) Forward(ctx context.Context, step int, previousRawOutput []float64) (*dynamics.Modulation, error) {
	if ctx == nil {
		return nil, errors.New("history sequence received nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if step != len(s.forwardRows) {
		return nil, errors.New("history sequence received a non-sequential step")
	}
	prev := coupledVectorClone(previousRawOutput)
	s.forwardRows = append(s.forwardRows, prev)
	s.forwardTs = append(s.forwardTs, step)
	var older []float64
	if step > 0 {
		older = s.forwardRows[step-1]
	}
	gain := []float64{
		1.12 + 0.06*prev[0] + valueAt(older, 0, 0.0)*0.02,
		0.91 + 0.06*prev[1] + valueAt(older, 1, 0.0)*0.02,
	}
	offset := []float64{
		0.08 + 0.015*prev[1] - valueAt(older, 1, 0.0)*0.01,
		-0.06 + 0.015*prev[0] - valueAt(older, 0, 0.0)*0.01,
	}
	return &dynamics.Modulation{Gain: [][]float64{gain}, Offset: [][]float64{offset}}, nil
}

func valueAt(row []float64, index int, fallback float64) float64 {
	if index < 0 || index >= len(row) {
		return fallback
	}
	return row[index]
}

func (s *historySequence) NewReverse(ctx context.Context, totalsteps, window int) (dynamics.ModulationReverse, error) {
	if ctx == nil {
		return nil, errors.New("history sequence received nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if totalsteps != len(s.forwardRows) {
		return nil, errors.New("history sequence step count mismatch")
	}
	r := &historyReverse{
		totalsteps:  totalsteps,
		window:      window,
		forwardRows: coupledRowsClone(s.forwardRows),
	}
	s.reverses = append(s.reverses, r)
	return r, nil
}

type historyReverse struct {
	totalsteps  int
	window      int
	forwardRows [][]float64
	calls       []int
	gradients   []dynamics.ModulationGradient
	returnRows  [][]dynamics.ActivityGradient
}

func (r *historyReverse) BackwardStep(ctx context.Context, step int, gradient dynamics.ModulationGradient) ([]dynamics.ActivityGradient, error) {
	if ctx == nil {
		return nil, errors.New("history reverse received nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if step < 0 || step >= r.totalsteps {
		return nil, errors.New("history reverse step out of range")
	}
	r.calls = append(r.calls, step)
	r.gradients = append(r.gradients, dynamics.ModulationGradient{
		Gain:      coupledVectorClone(gradient.Gain),
		Offset:    coupledVectorClone(gradient.Offset),
		Threshold: coupledVectorClone(gradient.Threshold),
	})
	start := 0
	if r.window > 0 {
		start = step / r.window * r.window
	}
	rows := make([]dynamics.ActivityGradient, 0, 2)
	appendRow := func(targetStep int, values []float64) {
		if targetStep < -1 || targetStep >= step {
			return
		}
		if targetStep == -1 {
			if start != 0 {
				return
			}
		} else if targetStep < start {
			return
		}
		rows = append(rows, dynamics.ActivityGradient{Step: targetStep, Values: values})
	}
	current := []float64{
		valueAt(gradient.Gain, 0, 0)*0.06 + valueAt(gradient.Offset, 1, 0)*0.015,
		valueAt(gradient.Gain, 1, 0)*0.06 + valueAt(gradient.Offset, 0, 0)*0.015,
	}
	appendRow(step-1, current)
	if step > 0 {
		older := []float64{
			valueAt(gradient.Gain, 0, 0)*0.02 - valueAt(gradient.Offset, 1, 0)*0.01,
			valueAt(gradient.Gain, 1, 0)*0.02 - valueAt(gradient.Offset, 0, 0)*0.01,
		}
		appendRow(step-2, older)
	}
	r.returnRows = append(r.returnRows, cloneActivityGradients(rows))
	return rows, nil
}

func TestContinuousCoupledHandCalculationAndModulationGradient(t *testing.T) {
	m := coupledOneNodeModel(t)
	p := dynamics.Parameters{Bias: []float64{0.5}, LogTau: []float64{0}}
	initial := []float64{0}
	inputs := [][]float64{{1}}
	seq := newRecordingSequence(1, 2, 0.25)
	tr, err := m.ForwardCoupled(context.Background(), p, initial, inputs, seq)
	if err != nil {
		t.Fatal(err)
	}
	alpha := 1 - math.Exp(-1)
	wantDrive := 2*1 + 0.25 + 0.5
	wantVoltage := alpha * wantDrive
	coupledClose(t, "final voltage", tr.FinalVoltage()[0], wantVoltage)
	coupledClose(t, "output", tr.Outputs()[0][0], math.Tanh(wantVoltage))
	if !reflect.DeepEqual(seq.forwardTs, []int{0}) {
		t.Fatalf("forward steps %v, want [0]", seq.forwardTs)
	}
	coupledVectorBitsEqual(t, "initial raw output", seq.forwardRows[0], []float64{0})

	upstream := [][]float64{{1}}
	g, err := m.Backward(context.Background(), tr, upstream, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(seq.reverses) != 1 || len(seq.reverses[0].gradients) != 1 {
		t.Fatalf("reverse calls = %d, gradients = %d; want one each", len(seq.reverses), len(seq.reverses[0].gradients))
	}
	modGradient := seq.reverses[0].gradients[0]
	doutput := 1 - math.Tanh(wantVoltage)*math.Tanh(wantVoltage)
	dv := doutput
	driveGradient := alpha * dv
	coupledClose(t, "input gradient", g.Inputs[0][0], 2*driveGradient)
	coupledClose(t, "bias gradient", g.Bias[0], driveGradient)
	coupledClose(t, "gain adjoint", modGradient.Gain[0], driveGradient)
	coupledClose(t, "offset adjoint", modGradient.Offset[0], driveGradient)
	dlambda := math.Exp(-1.0)
	coupledClose(t, "log_tau gradient", g.LogTau[0], dv*(0-wantDrive)*dlambda)
	coupledClose(t, "initial gradient", g.Initial[0], dlambda*dv)
}

func TestContinuousCoupledNilAndNeutralAreBitIdentical(t *testing.T) {
	m, p, initial, inputs, upstream := coupledFixture(t, "tanh")
	plain, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	nilSeq, err := m.ForwardCoupled(context.Background(), p, initial, inputs, nil)
	if err != nil {
		t.Fatal(err)
	}
	neutralSeq := newRecordingSequence(2, 1, 0)
	neutralSeq.zeroThreshold = true
	neutral, err := m.ForwardCoupled(context.Background(), p, initial, inputs, neutralSeq)
	if err != nil {
		t.Fatal(err)
	}
	coupledRowsBitsEqual(t, "nil outputs", nilSeq.Outputs(), plain.Outputs())
	coupledRowsBitsEqual(t, "neutral outputs", neutral.Outputs(), plain.Outputs())
	coupledVectorBitsEqual(t, "nil final voltage", nilSeq.FinalVoltage(), plain.FinalVoltage())
	coupledVectorBitsEqual(t, "neutral final voltage", neutral.FinalVoltage(), plain.FinalVoltage())

	plainGradient, err := m.Backward(context.Background(), plain, upstream, 0)
	if err != nil {
		t.Fatal(err)
	}
	nilGradient, err := m.Backward(context.Background(), nilSeq, upstream, 0)
	if err != nil {
		t.Fatal(err)
	}
	neutralGradient, err := m.Backward(context.Background(), neutral, upstream, 0)
	if err != nil {
		t.Fatal(err)
	}
	coupledGradientsBitsEqual(t, "nil gradient", nilGradient, plainGradient)
	coupledGradientsBitsEqual(t, "neutral gradient", neutralGradient, plainGradient)
	if len(neutralSeq.forwardRows) != len(inputs) || len(neutralSeq.reverses) != 1 {
		t.Fatalf("neutral sequence forward rows=%d reverse sessions=%d", len(neutralSeq.forwardRows), len(neutralSeq.reverses))
	}
}

func TestContinuousCoupledNilPathKeepsForwardAllocationProfile(t *testing.T) {
	m, p, initial, inputs, _ := coupledFixture(t, "tanh")
	plainAllocs := testing.AllocsPerRun(20, func() {
		_, _ = m.Forward(context.Background(), p, initial, inputs)
	})
	coupledAllocs := testing.AllocsPerRun(20, func() {
		_, _ = m.ForwardCoupled(context.Background(), p, initial, inputs, nil)
	})
	if coupledAllocs != plainAllocs {
		t.Fatalf("nil ForwardCoupled allocations = %v, plain Forward allocations = %v", coupledAllocs, plainAllocs)
	}
}

func TestContinuousCoupledGradientFiniteDifferenceAndOwnsForwardHistory(t *testing.T) {
	for _, activation := range []string{"tanh", "softplus"} {
		t.Run(activation, func(t *testing.T) {
			m, p, initial, inputs, upstream := coupledFixture(t, activation)
			beforeParameters := coupledParametersClone(p)
			beforeInitial := coupledVectorClone(initial)
			beforeInputs := coupledRowsClone(inputs)
			seq := &historySequence{}
			tr, err := m.ForwardCoupled(context.Background(), p, initial, inputs, seq)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(p, beforeParameters) || !reflect.DeepEqual(initial, beforeInitial) || !reflect.DeepEqual(inputs, beforeInputs) {
				t.Fatal("coupled forward changed caller-owned arrays")
			}
			outputs := tr.Outputs()
			if len(outputs) != len(inputs) {
				t.Fatalf("outputs rows=%d, want %d", len(outputs), len(inputs))
			}
			wantFirst := []float64{activationValue(activation, initial[0]), activationValue(activation, initial[1])}
			if len(seq.forwardRows) != len(inputs) {
				t.Fatalf("sequence observed %d rows, want %d", len(seq.forwardRows), len(inputs))
			}
			coupledVectorBitsEqual(t, "first forward raw output", seq.forwardRows[0], wantFirst)
			for step := 1; step < len(inputs); step++ {
				coupledVectorBitsEqual(t, "forward raw output", seq.forwardRows[step], outputs[step-1])
			}
			mutated := tr.Outputs()
			mutated[0][0] = 123
			if tr.Outputs()[0][0] == 123 {
				t.Fatal("Outputs returned an aliased row")
			}

			got, err := m.Backward(context.Background(), tr, upstream, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(seq.reverses) != 1 {
				t.Fatalf("NewReverse calls=%d, want 1", len(seq.reverses))
			}
			r := seq.reverses[0]
			wantCalls := []int{len(inputs) - 1, len(inputs) - 2, len(inputs) - 3, len(inputs) - 4}
			if !reflect.DeepEqual(r.calls, wantCalls) {
				t.Fatalf("BackwardStep order %v, want %v", r.calls, wantCalls)
			}
			// Reverse calls are [3, 2, 1, 0], so step 2 is at index 1.
			if len(r.returnRows) < 3 || len(r.returnRows[1]) != 2 {
				t.Fatalf("step-2 activity scatter = %#v, want two prior rows", r.returnRows)
			}
			if r.returnRows[1][0].Step != 1 || r.returnRows[1][1].Step != 0 {
				t.Fatalf("step 2 activity steps = %#v, want 1 and 0", r.returnRows[1])
			}

			coupledCheckFiniteDifferences(t, m, p, initial, inputs, upstream, activation, got)
		})
	}
}

func activationValue(kind string, v float64) float64 {
	if kind == "softplus" {
		return math.Max(v, 0) + math.Log1p(math.Exp(-math.Abs(v)))
	}
	return math.Tanh(v)
}

func coupledObjective(t *testing.T, m *dynamics.Continuous, p dynamics.Parameters, initial []float64, inputs, upstream [][]float64, activation string) float64 {
	t.Helper()
	tr, err := m.ForwardCoupled(context.Background(), p, initial, inputs, &historySequence{})
	if err != nil {
		t.Fatal(err)
	}
	var result float64
	for step, row := range tr.Outputs() {
		for i, value := range row {
			result += value * upstream[step][i]
		}
	}
	return result
}

func coupledCheckFiniteDifferences(t *testing.T, m *dynamics.Continuous, p dynamics.Parameters, initial []float64, inputs, upstream [][]float64, activation string, got dynamics.Gradient) {
	t.Helper()
	const epsilon = 1e-6
	base := coupledParametersClone(p)
	for i := range base.Weights {
		plus := coupledParametersClone(base)
		minus := coupledParametersClone(base)
		plus.Weights[i] += epsilon
		minus.Weights[i] -= epsilon
		fd := coupledObjective(t, m, plus, initial, inputs, upstream, activation)
		minusValue := coupledObjective(t, m, minus, initial, inputs, upstream, activation)
		fd = (fd - minusValue) / (2 * epsilon)
		coupledFDClose(t, "weight gradient", got.Weights[i], fd)
	}
	for i := range base.Bias {
		plus := coupledParametersClone(base)
		minus := coupledParametersClone(base)
		plus.Bias[i] += epsilon
		minus.Bias[i] -= epsilon
		fd := coupledObjective(t, m, plus, initial, inputs, upstream, activation)
		minusValue := coupledObjective(t, m, minus, initial, inputs, upstream, activation)
		fd = (fd - minusValue) / (2 * epsilon)
		coupledFDClose(t, "bias gradient", got.Bias[i], fd)
	}
	for i := range base.LogTau {
		plus := coupledParametersClone(base)
		minus := coupledParametersClone(base)
		plus.LogTau[i] += epsilon
		minus.LogTau[i] -= epsilon
		fd := coupledObjective(t, m, plus, initial, inputs, upstream, activation)
		minusValue := coupledObjective(t, m, minus, initial, inputs, upstream, activation)
		fd = (fd - minusValue) / (2 * epsilon)
		coupledFDClose(t, "log_tau gradient", got.LogTau[i], fd)
	}
	for i := range initial {
		plusInitial := coupledVectorClone(initial)
		minusInitial := coupledVectorClone(initial)
		plusInitial[i] += epsilon
		minusInitial[i] -= epsilon
		plusValue := coupledObjective(t, m, base, plusInitial, inputs, upstream, activation)
		minusValue := coupledObjective(t, m, base, minusInitial, inputs, upstream, activation)
		fd := (plusValue - minusValue) / (2 * epsilon)
		coupledFDClose(t, "initial gradient", got.Initial[i], fd)
	}
	for step := range inputs {
		for i := range inputs[step] {
			plusInputs := coupledRowsClone(inputs)
			minusInputs := coupledRowsClone(inputs)
			plusInputs[step][i] += epsilon
			minusInputs[step][i] -= epsilon
			plusValue := coupledObjective(t, m, base, initial, plusInputs, upstream, activation)
			minusValue := coupledObjective(t, m, base, initial, minusInputs, upstream, activation)
			fd := (plusValue - minusValue) / (2 * epsilon)
			coupledFDClose(t, "input gradient", got.Inputs[step][i], fd)
		}
	}
}

func TestContinuousCoupledTruncationCutsCoreAndActivityFeedback(t *testing.T) {
	m, p, initial, inputs, _ := coupledFixture(t, "tanh")
	upstream := make([][]float64, len(inputs))
	for i := range upstream {
		upstream[i] = []float64{0, 0}
	}
	upstream[len(upstream)-1] = []float64{0.37, -0.29}
	fullSequence := &historySequence{}
	fullTrace, err := m.ForwardCoupled(context.Background(), p, initial, inputs, fullSequence)
	if err != nil {
		t.Fatal(err)
	}
	cutSequence := &historySequence{}
	cutTrace, err := m.ForwardCoupled(context.Background(), p, initial, inputs, cutSequence)
	if err != nil {
		t.Fatal(err)
	}
	full, err := m.Backward(context.Background(), fullTrace, upstream, 0)
	if err != nil {
		t.Fatal(err)
	}
	cut, err := m.Backward(context.Background(), cutTrace, upstream, 2)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(full.Inputs[0][0])+math.Abs(full.Inputs[1][0]) == 0 {
		t.Fatal("full backward did not reach the first activity window")
	}
	for step := 0; step < 2; step++ {
		for node, value := range cut.Inputs[step] {
			if value != 0 {
				t.Fatalf("cut input gradient[%d][%d] = %v, crossed the window boundary", step, node, value)
			}
		}
	}
	for node, value := range cut.Initial {
		if value != 0 {
			t.Fatalf("cut initial gradient[%d] = %v, crossed the window boundary", node, value)
		}
	}
	if len(cutSequence.reverses) != 1 {
		t.Fatalf("cut NewReverse calls=%d, want 1", len(cutSequence.reverses))
	}
	cutReverse := cutSequence.reverses[0]
	if len(cutReverse.returnRows) < 4 {
		t.Fatalf("cut reverse rows=%d, want one row per step", len(cutReverse.returnRows))
	}
	if len(cutReverse.returnRows[1]) != 0 {
		t.Fatalf("cut step 2 returned %#v, crossed the window boundary", cutReverse.returnRows[1])
	}
	if len(cutReverse.returnRows[0]) != 1 || cutReverse.returnRows[0][0].Step != 2 {
		t.Fatalf("cut step 3 returned %#v, want only prior step 2", cutReverse.returnRows[0])
	}
}

func TestContinuousCoupledBackwardUsesFreshReverseAndAddsActivityRows(t *testing.T) {
	m := coupledOneNodeModel(t)
	p := dynamics.Parameters{Bias: []float64{0.1}, LogTau: []float64{0.2}}
	initial := []float64{0.2}
	inputs := [][]float64{{0.4}, {-0.1}, {0.25}}
	upstreamA := [][]float64{{0.1}, {0.2}, {0.7}}
	upstreamB := [][]float64{{-0.3}, {0.4}, {-0.2}}
	newTrace := func() (*dynamics.Trace, *recordingSequence) {
		seq := newRecordingSequence(1, 1.3, 0.08)
		tr, err := m.ForwardCoupled(context.Background(), p, initial, inputs, seq)
		if err != nil {
			t.Fatal(err)
		}
		return tr, seq
	}
	tr, seq := newTrace()
	first, err := m.Backward(context.Background(), tr, upstreamA, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Backward(context.Background(), tr, upstreamB, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(seq.reverses) != 2 {
		t.Fatalf("NewReverse calls=%d, want two independent sessions", len(seq.reverses))
	}
	if seq.reverses[0] == seq.reverses[1] {
		t.Fatal("NewReverse reused the same reverse session")
	}
	if !reflect.DeepEqual(seq.reverses[0].calls, []int{2, 1, 0}) || !reflect.DeepEqual(seq.reverses[1].calls, []int{2, 1, 0}) {
		t.Fatalf("reverse call histories = %v and %v", seq.reverses[0].calls, seq.reverses[1].calls)
	}
	trA, seqA := newTrace()
	wantFirst, err := m.Backward(context.Background(), trA, upstreamA, 0)
	if err != nil {
		t.Fatal(err)
	}
	trB, seqB := newTrace()
	wantSecond, err := m.Backward(context.Background(), trB, upstreamB, 0)
	if err != nil {
		t.Fatal(err)
	}
	coupledGradientsClose(t, "first repeat", first, wantFirst)
	coupledGradientsClose(t, "second repeat", second, wantSecond)
	if len(seqA.reverses) != 1 || len(seqB.reverses) != 1 {
		t.Fatalf("fresh sequence reverse counts = %d and %d", len(seqA.reverses), len(seqB.reverses))
	}

	// Two identical prior-step rows are intentionally returned by the reverse
	// mock. The result must be the sum of both rows, not the last row only.
	newResponseSequence := func(rows []dynamics.ActivityGradient) (*dynamics.Trace, *recordingSequence) {
		seq := newRecordingSequence(1, 1.3, 0.08)
		seq.reverseFactory = func(totalsteps, window int) *recordingReverse {
			return &recordingReverse{
				nodes:      1,
				totalsteps: totalsteps,
				window:     window,
				errAt:      -1,
				responses:  map[int][]dynamics.ActivityGradient{2: cloneActivityGradients(rows)},
			}
		}
		tr, err := m.ForwardCoupled(context.Background(), p, initial, inputs, seq)
		if err != nil {
			t.Fatal(err)
		}
		return tr, seq
	}
	baseTrace, _ := newResponseSequence(nil)
	oneTrace, _ := newResponseSequence([]dynamics.ActivityGradient{{Step: 1, Values: []float64{0.4}}})
	twoTrace, _ := newResponseSequence([]dynamics.ActivityGradient{
		{Step: 1, Values: []float64{0.4}},
		{Step: 1, Values: []float64{0.4}},
	})
	baseGradient, err := m.Backward(context.Background(), baseTrace, upstreamA, 0)
	if err != nil {
		t.Fatal(err)
	}
	oneGradient, err := m.Backward(context.Background(), oneTrace, upstreamA, 0)
	if err != nil {
		t.Fatal(err)
	}
	twoGradient, err := m.Backward(context.Background(), twoTrace, upstreamA, 0)
	if err != nil {
		t.Fatal(err)
	}
	coupledGradientLinear(t, "additive activity", baseGradient, oneGradient, twoGradient, 2)
}

func coupledGradientsClose(t *testing.T, name string, got, want dynamics.Gradient) {
	t.Helper()
	if len(got.Weights) != len(want.Weights) || len(got.Bias) != len(want.Bias) || len(got.LogTau) != len(want.LogTau) || len(got.Inputs) != len(want.Inputs) || len(got.Initial) != len(want.Initial) {
		t.Fatalf("%s has incompatible gradient shapes", name)
	}
	for i := range got.Weights {
		coupledFDClose(t, name+" weights", got.Weights[i], want.Weights[i])
	}
	for i := range got.Bias {
		coupledFDClose(t, name+" bias", got.Bias[i], want.Bias[i])
		coupledFDClose(t, name+" log_tau", got.LogTau[i], want.LogTau[i])
		coupledFDClose(t, name+" initial", got.Initial[i], want.Initial[i])
	}
	for step := range got.Inputs {
		for i := range got.Inputs[step] {
			coupledFDClose(t, name+" inputs", got.Inputs[step][i], want.Inputs[step][i])
		}
	}
}

func coupledGradientLinear(t *testing.T, name string, base, one, two dynamics.Gradient, scale float64) {
	t.Helper()
	check := func(label string, got, baseValue, oneValue float64) {
		want := baseValue + scale*(oneValue-baseValue)
		coupledFDClose(t, name+" "+label, got, want)
	}
	for i := range two.Weights {
		check("weights", two.Weights[i], base.Weights[i], one.Weights[i])
	}
	for i := range two.Bias {
		check("bias", two.Bias[i], base.Bias[i], one.Bias[i])
		check("log_tau", two.LogTau[i], base.LogTau[i], one.LogTau[i])
		check("initial", two.Initial[i], base.Initial[i], one.Initial[i])
	}
	for step := range two.Inputs {
		for i := range two.Inputs[step] {
			check("inputs", two.Inputs[step][i], base.Inputs[step][i], one.Inputs[step][i])
		}
	}
}

type invalidActivitySequence struct {
	nodes int
	kind  string
}

func (s invalidActivitySequence) Forward(ctx context.Context, step int, previousRawOutput []float64) (*dynamics.Modulation, error) {
	if ctx == nil {
		return nil, errors.New("invalid sequence received nil context")
	}
	gain, offset := make([]float64, s.nodes), make([]float64, s.nodes)
	for i := range gain {
		gain[i], offset[i] = 1, 0
	}
	return &dynamics.Modulation{Gain: [][]float64{gain}, Offset: [][]float64{offset}}, nil
}

func (s invalidActivitySequence) NewReverse(ctx context.Context, totalsteps, window int) (dynamics.ModulationReverse, error) {
	return invalidActivityReverse{nodes: s.nodes, kind: s.kind}, nil
}

type invalidActivityReverse struct {
	nodes int
	kind  string
}

func (r invalidActivityReverse) BackwardStep(context.Context, int, dynamics.ModulationGradient) ([]dynamics.ActivityGradient, error) {
	values := make([]float64, r.nodes)
	step := 0
	switch r.kind {
	case "future":
		step = 99
	case "below-initial":
		step = -2
	case "width":
		return []dynamics.ActivityGradient{{Step: -1, Values: []float64{0}}}, nil
	case "nonfinite":
		values[0] = math.NaN()
		step = -1
	default:
		return nil, errors.New("unknown invalid activity case")
	}
	return []dynamics.ActivityGradient{{Step: step, Values: values}}, nil
}

func TestContinuousCoupledForwardErrorsAreAtomic(t *testing.T) {
	m, p, initial, inputs, _ := coupledFixture(t, "tanh")
	cases := []struct {
		name string
		call func() (*dynamics.Trace, error)
	}{
		{
			name: "sequence error",
			call: func() (*dynamics.Trace, error) {
				seq := newRecordingSequence(2, 1, 0)
				seq.forwardErr = errors.New("forward sequence failure")
				return m.ForwardCoupled(context.Background(), p, initial, inputs, seq)
			},
		},
		{
			name: "malformed gain",
			call: func() (*dynamics.Trace, error) {
				seq := newRecordingSequence(2, 1, 0)
				seq.badMod = &dynamics.Modulation{Gain: [][]float64{{1}}}
				return m.ForwardCoupled(context.Background(), p, initial, inputs, seq)
			},
		},
		{
			name: "multiple modulation rows",
			call: func() (*dynamics.Trace, error) {
				seq := newRecordingSequence(2, 1, 0)
				seq.badMod = &dynamics.Modulation{Gain: [][]float64{{1, 1}, {1, 1}}}
				return m.ForwardCoupled(context.Background(), p, initial, inputs, seq)
			},
		},
		{
			name: "nonzero continuous threshold",
			call: func() (*dynamics.Trace, error) {
				seq := newRecordingSequence(2, 1, 0)
				seq.badMod = &dynamics.Modulation{Threshold: [][]float64{{0, 0.25}}}
				return m.ForwardCoupled(context.Background(), p, initial, inputs, seq)
			},
		},
		{
			name: "nil modulation is not one row",
			call: func() (*dynamics.Trace, error) {
				seq := newRecordingSequence(2, 1, 0)
				seq.returnNil = true
				return m.ForwardCoupled(context.Background(), p, initial, inputs, seq)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeParameters := coupledParametersClone(p)
			beforeInitial := coupledVectorClone(initial)
			beforeInputs := coupledRowsClone(inputs)
			tr, err := tc.call()
			if err == nil || tr != nil {
				t.Fatalf("trace=%v err=%v, want nil trace and error", tr, err)
			}
			if !reflect.DeepEqual(p, beforeParameters) || !reflect.DeepEqual(initial, beforeInitial) || !reflect.DeepEqual(inputs, beforeInputs) {
				t.Fatal("failed forward changed caller-owned arrays")
			}
		})
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	tr, err := m.ForwardCoupled(canceled, p, initial, inputs, newRecordingSequence(2, 1, 0))
	if err == nil || tr != nil {
		t.Fatalf("canceled forward trace=%v err=%v", tr, err)
	}
	var nilModel *dynamics.Continuous
	tr, err = nilModel.ForwardCoupled(context.Background(), p, []float64{0}, [][]float64{{0}}, newRecordingSequence(1, 1, 0))
	if err == nil || tr != nil {
		t.Fatalf("nil model trace=%v err=%v", tr, err)
	}
}

func TestContinuousCoupledBackwardRejectsInvalidActivityAtomically(t *testing.T) {
	m, p, initial, inputs, upstream := coupledFixture(t, "tanh")
	for _, kind := range []string{"future", "below-initial", "width", "nonfinite"} {
		t.Run(kind, func(t *testing.T) {
			tr, err := m.ForwardCoupled(context.Background(), p, initial, inputs, invalidActivitySequence{nodes: 2, kind: kind})
			if err != nil {
				t.Fatal(err)
			}
			beforeOutputs := tr.Outputs()
			beforeUpstream := coupledRowsClone(upstream)
			got, err := m.Backward(context.Background(), tr, upstream, 0)
			if err == nil || !coupledGradientIsZero(got) {
				t.Fatalf("gradient=%#v err=%v, want zero gradient and error", got, err)
			}
			coupledRowsBitsEqual(t, "trace after failed backward", tr.Outputs(), beforeOutputs)
			if !reflect.DeepEqual(upstream, beforeUpstream) {
				t.Fatal("failed backward changed upstream")
			}
		})
	}

}

func TestContinuousCoupledBackwardReverseErrorIsZero(t *testing.T) {
	m, p, initial, inputs, upstream := coupledFixture(t, "tanh")
	// Replace the default reverse session behavior for this case by using a
	// sequence whose NewReverse returns a reverse that fails at step 2.
	failing := &recordingSequence{nodes: 2, gain: 1.1, offset: 0.03, reverseFactory: func(totalsteps, window int) *recordingReverse {
		return &recordingReverse{nodes: 2, totalsteps: totalsteps, window: window, errAt: 2, responses: map[int][]dynamics.ActivityGradient{}}
	}}
	tr, err := m.ForwardCoupled(context.Background(), p, initial, inputs, failing)
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.Backward(context.Background(), tr, upstream, 0)
	if err == nil || !coupledGradientIsZero(got) {
		t.Fatalf("gradient=%#v err=%v, want zero gradient and error", got, err)
	}
}
