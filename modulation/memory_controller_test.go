package modulation

import (
	"math"
	"math/rand/v2"
	"strings"
	"testing"
)

// One fixture shape serves the gradient tests below: the controller's own
// objective fixture context (one arrived feedback and the declared energy
// resource) with 3 hidden units, so x is 4 wide (activity mean, activity
// variance, feedback score, energy) and the parameter vector
// 3*4 + 3 + 3 + 1 = 19 long. The node count is the objective fixture's,
// because objectiveStep supplies an activity row of exactly that width.
const (
	memoryNodes  = objectiveNodes
	memoryHidden = 3
	memoryWidth  = 4
	memoryCount  = 19
	memoryTarget = 0.2
)

// memoryFixture draws the parameters and the two activity rows from one fixed
// generator, so every run reads the same numbers on every machine.
func memoryFixture() (parameters []float64, rows [][]float64) {
	prng := rand.New(rand.NewPCG(5, 0))
	parameters = make([]float64, memoryCount)
	for i := range parameters {
		parameters[i] = 2*prng.Float64() - 1
	}
	rows = make([][]float64, 2)
	for i := range rows {
		rows[i] = make([]float64, memoryNodes)
		for j := range rows[i] {
			rows[i][j] = prng.Float64()
		}
	}
	return parameters, rows
}

// memoryController is the fixture control carrying the given parameters. Every
// call builds a fresh one, so a perturbed replay starts its window from the
// same zeros as the replay it is compared against.
func memoryController(parameters []float64) *MemoryController {
	return &MemoryController{
		Inputs:     ControllerInputs{SummaryWindow: 2, UseFeedback: true, Resources: []string{"energy"}},
		Nodes:      memoryNodes,
		Hidden:     memoryHidden,
		Parameters: append([]float64(nil), parameters...),
	}
}

// replayOffsets runs the fixture sequence and returns the offset of the last
// step, which is the y the gradient is taken at.
func replayOffsets(t *testing.T, m *MemoryController, rows [][]float64) float64 {
	t.Helper()
	var offset float64
	for step, row := range rows {
		y, err := m.Offset(uint64(step), objectiveStep(t, row))
		if err != nil {
			t.Fatalf("Offset(%d) error = %v", step, err)
		}
		offset = y
	}
	return offset
}

// memoryLossAt replays the same sequence with one parameter moved by delta and
// returns the loss by hand, with no part of it coming from the gradient.
func memoryLossAt(t *testing.T, parameters []float64, index int, delta float64, rows [][]float64) float64 {
	t.Helper()
	moved := append([]float64(nil), parameters...)
	moved[index] += delta
	offset := replayOffsets(t, memoryController(moved), rows)
	return (offset - memoryTarget) * (offset - memoryTarget)
}

// The control is capacity-matched by construction: same inputs, same hidden
// layer, same parameter count and the same cost per step, with its own copy of
// everything it carries.
func TestCapacityMatchedHasEqualParameterCount(t *testing.T) {
	c := &Controller{
		Inputs:     ControllerInputs{SummaryWindow: 1, UseFeedback: true, Resources: []string{"energy"}},
		Nodes:      2,
		Hidden:     3,
		Channel:    0,
		Parameters: make([]float64, 19),
	}
	for i := range c.Parameters {
		c.Parameters[i] = 0.5 - float64(i)/19
	}
	if c.InputWidth() != 4 || c.ParameterCount() != 19 || c.MultAddsPerStep() != 15 {
		t.Fatalf("controller declares width %d, %d parameters and %d mult-adds, want 4, 19 and 15", c.InputWidth(), c.ParameterCount(), c.MultAddsPerStep())
	}

	m, err := NewCapacityMatched(c)
	if err != nil {
		t.Fatalf("NewCapacityMatched() error = %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if m.InputWidth() != c.InputWidth() {
		t.Errorf("InputWidth() = %d, want the controller's %d", m.InputWidth(), c.InputWidth())
	}
	if m.ParameterCount() != c.ParameterCount() || m.ParameterCount() != 19 {
		t.Errorf("ParameterCount() = %d, want the controller's %d and 19", m.ParameterCount(), c.ParameterCount())
	}
	if m.MultAddsPerStep() != c.MultAddsPerStep() || m.MultAddsPerStep() != 15 {
		t.Errorf("MultAddsPerStep() = %d, want the controller's %d and 15", m.MultAddsPerStep(), c.MultAddsPerStep())
	}
	if len(m.Parameters) != len(c.Parameters) {
		t.Fatalf("the control carries %d parameters, want the controller's %d", len(m.Parameters), len(c.Parameters))
	}

	// Each side owns what it carries: moving one moves nothing on the other.
	was := c.Parameters[0]
	m.Parameters[0] = was + 5
	if c.Parameters[0] != was {
		t.Errorf("controller parameter 0 = %v after the control moved its own, want %v", c.Parameters[0], was)
	}
	m.Inputs.Resources[0] = "fuel"
	if c.Inputs.Resources[0] != "energy" {
		t.Errorf("controller resource 0 = %q after the control renamed its own, want %q", c.Inputs.Resources[0], "energy")
	}
}

// The forward pass is small enough to write down, and it is the controller's
// with the softplus taken off: Nodes 2, Hidden 1, a window of one row, no
// feedback and no resources, so x is the mean and the population variance.
// W1 = {1, 0} reads the mean and ignores the variance, b1 = {0}, w2 = {2},
// b2 = {0.5}.
func TestMemoryControllerHandForward(t *testing.T) {
	parameters := []float64{1, 0, 0, 2, 0.5}
	m := &MemoryController{
		Inputs:     ControllerInputs{SummaryWindow: 1},
		Nodes:      2,
		Hidden:     1,
		Parameters: append([]float64(nil), parameters...),
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if m.InputWidth() != 2 || m.ParameterCount() != len(parameters) {
		t.Fatalf("the control declares width %d and %d parameters, want 2 and %d", m.InputWidth(), m.ParameterCount(), len(parameters))
	}

	// The window holds one row, {1, 1}: mean 1 and population variance 0.
	offset, err := m.Offset(0, SourceContext{Activity: []float64{1, 1}})
	if err != nil {
		t.Fatalf("Offset(0) error = %v", err)
	}
	want := 2*math.Tanh(1) + 0.5
	if math.Abs(offset-want) > 1e-12 {
		t.Fatalf("Offset(0) = %v, want %v", offset, want)
	}

	// The same parameters on the same input differ by the readout alone.
	c := &Controller{
		Inputs:     ControllerInputs{SummaryWindow: 1},
		Nodes:      2,
		Hidden:     1,
		Channel:    0,
		Parameters: append([]float64(nil), parameters...),
	}
	rates, err := c.Release(0, SourceContext{Activity: []float64{1, 1}})
	if err != nil {
		t.Fatalf("Release(0) error = %v", err)
	}
	if math.Abs(rates[0]-softplusByHand(want)) > 1e-12 {
		t.Fatalf("Release(0) released %v, want softplus of the control's %v = %v", rates[0], want, softplusByHand(want))
	}
}

// The analytic gradient is checked against a central difference of the loss
// itself, parameter by parameter, against the declared target.
func TestMemoryControllerGradientMatchesFiniteDifferences(t *testing.T) {
	parameters, rows := memoryFixture()
	const eps = 1e-6

	m := memoryController(parameters)
	if m.InputWidth() != memoryWidth || m.ParameterCount() != memoryCount {
		t.Fatalf("fixture declares width %d and %d parameters, want %d and %d", m.InputWidth(), m.ParameterCount(), memoryWidth, memoryCount)
	}
	offset := replayOffsets(t, m, rows)
	loss, grad, err := m.Gradient(memoryTarget)
	if err != nil {
		t.Fatalf("Gradient() error = %v", err)
	}
	if len(grad) != memoryCount {
		t.Fatalf("Gradient() returned %d values, want %d", len(grad), memoryCount)
	}
	byHand := (offset - memoryTarget) * (offset - memoryTarget)
	if math.Abs(loss-byHand) > 1e-12 {
		t.Fatalf("Gradient() loss = %v, want %v for offset %v and target %v", loss, byHand, offset, memoryTarget)
	}

	var worst float64
	for i := range parameters {
		plus := memoryLossAt(t, parameters, i, eps, rows)
		minus := memoryLossAt(t, parameters, i, -eps, rows)
		difference := (plus - minus) / (2 * eps)
		if math.Abs(grad[i]) < 1e-8 {
			if math.Abs(grad[i]-difference) > 1e-9 {
				t.Errorf("parameter %d: gradient %v, central difference %v", i, grad[i], difference)
			}
			continue
		}
		relative := math.Abs(grad[i]-difference) / math.Abs(grad[i])
		if relative > 1e-5 {
			t.Errorf("parameter %d: gradient %v, central difference %v, relative error %g", i, grad[i], difference, relative)
		}
		worst = math.Max(worst, relative)
	}
	t.Logf("worst relative error %g over %d parameters", worst, len(parameters))
}

// The control refuses a declaration it cannot run and a gradient it cannot
// take honestly.
func TestMemoryControllerValidateRejects(t *testing.T) {
	parameters, rows := memoryFixture()

	short := memoryController(parameters)
	short.Parameters = short.Parameters[:memoryCount-1]
	if err := short.Validate(); err == nil {
		t.Errorf("Validate() error = nil for %d of %d parameters", len(short.Parameters), memoryCount)
	}
	if _, err := short.Offset(0, objectiveStep(t, rows[0])); err == nil {
		t.Error("Offset() error = nil for a control carrying too few parameters")
	}

	flat := memoryController(parameters)
	flat.Hidden = 0
	if err := flat.Validate(); err == nil {
		t.Error("Validate() error = nil for 0 hidden units")
	}

	if _, err := NewCapacityMatched(nil); err == nil {
		t.Error("NewCapacityMatched(nil) error = nil, want one naming the missing controller")
	}

	// A control that has not offset anything has no inputs to take a gradient at.
	fresh := memoryController(parameters)
	if _, _, err := fresh.Gradient(memoryTarget); err == nil || !strings.Contains(err.Error(), "no offset") {
		t.Errorf("Gradient() before any offset error = %v, want one naming the missing offset", err)
	}
}
