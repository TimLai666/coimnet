package modulation

import (
	"math"
	"testing"
)

func controllerUpdateCacheController(parameters []float64) *Controller {
	return &Controller{
		Inputs:     ControllerInputs{SummaryWindow: 2},
		Nodes:      1,
		Hidden:     1,
		Channel:    0,
		Parameters: append([]float64(nil), parameters...),
	}
}

func controllerUpdateCacheMemory(parameters []float64) *MemoryController {
	return &MemoryController{
		Inputs:     ControllerInputs{SummaryWindow: 2},
		Nodes:      1,
		Hidden:     1,
		Parameters: append([]float64(nil), parameters...),
	}
}

func controllerUpdateCacheObjective() ControllerObjective {
	return ControllerObjective{Kind: ObjectiveActivityTarget, Target: 0}
}

func controllerUpdateCacheHistory() [][]float64 {
	return [][]float64{{1}, {-0.5}}
}

func controllerUpdateCacheReleaseHistory(t *testing.T, c *Controller, rows [][]float64) {
	t.Helper()
	for step, row := range rows {
		if _, err := c.Release(uint64(step), SourceContext{Activity: row}); err != nil {
			t.Fatalf("Release(%d) error = %v", step, err)
		}
	}
}

func controllerUpdateCacheOffsetHistory(t *testing.T, m *MemoryController, rows [][]float64) {
	t.Helper()
	for step, row := range rows {
		if _, err := m.Offset(uint64(step), SourceContext{Activity: row}); err != nil {
			t.Fatalf("Offset(%d) error = %v", step, err)
		}
	}
}

func controllerUpdateCacheFloatsBitsEqual(a, b []float64) bool {
	if (a == nil) != (b == nil) || len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Float64bits(a[i]) != math.Float64bits(b[i]) {
			return false
		}
	}
	return true
}

func controllerUpdateCacheRowsBitsEqual(a, b [][]float64) bool {
	if (a == nil) != (b == nil) || len(a) != len(b) {
		return false
	}
	for i := range a {
		if !controllerUpdateCacheFloatsBitsEqual(a[i], b[i]) {
			return false
		}
		if (a[i] == nil) != (b[i] == nil) {
			return false
		}
	}
	return true
}

func controllerUpdateCacheStatesBitsEqual(a, b ControllerState) bool {
	return a.Filled == b.Filled && controllerUpdateCacheRowsBitsEqual(a.Window, b.Window)
}

func controllerUpdateCacheAnyNonZero(values []float64) bool {
	for _, value := range values {
		if value != 0 {
			return true
		}
	}
	return false
}

func controllerUpdateCacheAssertGradientEqual(t *testing.T, name string, gotLoss float64, gotGradient []float64, wantLoss float64, wantGradient []float64) {
	t.Helper()
	if math.Float64bits(gotLoss) != math.Float64bits(wantLoss) {
		t.Fatalf("%s loss = %v, want %v", name, gotLoss, wantLoss)
	}
	if !controllerUpdateCacheFloatsBitsEqual(gotGradient, wantGradient) {
		t.Fatalf("%s gradient = %#v, want %#v", name, gotGradient, wantGradient)
	}
}

func controllerUpdateCacheAssertControllerFailure(t *testing.T, c *Controller, objective ControllerObjective, beforeParameters []float64, beforeState ControllerState, beforeLoss float64, beforeGradient []float64, beforeBackward [][]float64) {
	t.Helper()
	if !controllerUpdateCacheFloatsBitsEqual(c.Parameters, beforeParameters) {
		t.Fatalf("failed Update changed parameters to %#v, want %#v", c.Parameters, beforeParameters)
	}
	if !controllerUpdateCacheStatesBitsEqual(c.State(), beforeState) {
		t.Fatalf("failed Update changed the activity state")
	}
	loss, gradient, err := c.Gradient(objective)
	if err != nil {
		t.Fatalf("Gradient after failed Update error = %v", err)
	}
	controllerUpdateCacheAssertGradientEqual(t, "Gradient after failed Update", loss, gradient, beforeLoss, beforeGradient)
	backward, err := c.BackwardActivity(0.75)
	if err != nil {
		t.Fatalf("BackwardActivity after failed Update error = %v", err)
	}
	if !controllerUpdateCacheRowsBitsEqual(backward, beforeBackward) {
		t.Fatalf("BackwardActivity after failed Update = %#v, want %#v", backward, beforeBackward)
	}
}

// A successful parameter update invalidates the cache, but keeps the activity
// window. A fresh forward then has the same output and gradients as an
// independent controller with the updated parameters and the same history.
func TestControllerUpdateCache(t *testing.T) {
	parameters := []float64{0.5, -0.25, 0.1, 0.7, -0.2}
	objective := controllerUpdateCacheObjective()
	history := controllerUpdateCacheHistory()
	c := controllerUpdateCacheController(parameters)
	controllerUpdateCacheReleaseHistory(t, c, history)

	beforeState := c.State()
	_, gradient, err := c.Gradient(objective)
	if err != nil {
		t.Fatalf("baseline Gradient error = %v", err)
	}
	if !controllerUpdateCacheAnyNonZero(gradient) {
		t.Fatal("baseline Gradient is all zero")
	}
	if _, err := c.BackwardActivity(0.75); err != nil {
		t.Fatalf("baseline BackwardActivity error = %v", err)
	}
	beforeParameters := append([]float64(nil), c.Parameters...)
	updated, err := c.Update(gradient, 0.1)
	if err != nil {
		t.Fatalf("Update error = %v", err)
	}
	if controllerUpdateCacheFloatsBitsEqual(updated, beforeParameters) {
		t.Fatal("successful Update did not change any parameter")
	}
	if !controllerUpdateCacheFloatsBitsEqual(c.Parameters, updated) {
		t.Fatalf("Update parameters = %#v, returned %#v", c.Parameters, updated)
	}
	if !controllerUpdateCacheStatesBitsEqual(c.State(), beforeState) {
		t.Fatal("successful Update discarded the activity window")
	}

	if gotLoss, gotGradient, err := c.Gradient(objective); err == nil || gotLoss != 0 || gotGradient != nil {
		t.Errorf("Gradient after successful Update = (%v, %#v, %v), want zero,nil,error", gotLoss, gotGradient, err)
	}
	if got, err := c.BackwardActivity(0.75); err == nil || got != nil {
		t.Errorf("BackwardActivity after successful Update = (%#v, %v), want nil,error", got, err)
	}

	next := []float64{0.25}
	gotRates, err := c.Release(2, SourceContext{Activity: next})
	if err != nil {
		t.Fatalf("Release after successful Update error = %v", err)
	}
	twin := controllerUpdateCacheController(updated)
	controllerUpdateCacheReleaseHistory(t, twin, history)
	wantRates, err := twin.Release(2, SourceContext{Activity: next})
	if err != nil {
		t.Fatalf("twin Release after Update error = %v", err)
	}
	if !controllerUpdateCacheFloatsBitsEqual(gotRates, wantRates) {
		t.Fatalf("continued Release = %#v, want twin %#v", gotRates, wantRates)
	}
	if !controllerUpdateCacheStatesBitsEqual(c.State(), twin.State()) {
		t.Fatal("continued Release did not preserve the same activity history as the twin")
	}

	gotLoss, gotGradient, err := c.Gradient(objective)
	if err != nil {
		t.Fatalf("Gradient after new Release error = %v", err)
	}
	wantLoss, wantGradient, err := twin.Gradient(objective)
	if err != nil {
		t.Fatalf("twin Gradient after new Release error = %v", err)
	}
	controllerUpdateCacheAssertGradientEqual(t, "Gradient after new Release", gotLoss, gotGradient, wantLoss, wantGradient)
	gotBackward, err := c.BackwardActivity(0.75)
	if err != nil {
		t.Fatalf("BackwardActivity after new Release error = %v", err)
	}
	wantBackward, err := twin.BackwardActivity(0.75)
	if err != nil {
		t.Fatalf("twin BackwardActivity after new Release error = %v", err)
	}
	if !controllerUpdateCacheRowsBitsEqual(gotBackward, wantBackward) {
		t.Fatalf("BackwardActivity after new Release = %#v, want twin %#v", gotBackward, wantBackward)
	}
}

func TestControllerUpdateCacheFailures(t *testing.T) {
	objective := controllerUpdateCacheObjective()
	history := controllerUpdateCacheHistory()

	prepare := func(t *testing.T) (*Controller, []float64, ControllerState, float64, []float64, [][]float64) {
		t.Helper()
		c := controllerUpdateCacheController([]float64{0.5, -0.25, 0.1, 0.7, -0.2})
		controllerUpdateCacheReleaseHistory(t, c, history)
		loss, gradient, err := c.Gradient(objective)
		if err != nil {
			t.Fatalf("baseline Gradient error = %v", err)
		}
		backward, err := c.BackwardActivity(0.75)
		if err != nil {
			t.Fatalf("baseline BackwardActivity error = %v", err)
		}
		return c, append([]float64(nil), c.Parameters...), c.State(), loss, gradient, backward
	}

	t.Run("invalid rate", func(t *testing.T) {
		c, beforeParameters, beforeState, beforeLoss, beforeGradient, beforeBackward := prepare(t)
		if _, err := c.Update(beforeGradient, 0); err == nil {
			t.Fatal("Update with zero rate returned nil error")
		}
		controllerUpdateCacheAssertControllerFailure(t, c, objective, beforeParameters, beforeState, beforeLoss, beforeGradient, beforeBackward)
	})

	t.Run("invalid gradient", func(t *testing.T) {
		c, beforeParameters, beforeState, beforeLoss, beforeGradient, beforeBackward := prepare(t)
		invalid := append([]float64(nil), beforeGradient...)
		invalid[0] = math.NaN()
		if _, err := c.Update(invalid, 0.1); err == nil {
			t.Fatal("Update with a non-finite gradient returned nil error")
		}
		controllerUpdateCacheAssertControllerFailure(t, c, objective, beforeParameters, beforeState, beforeLoss, beforeGradient, beforeBackward)
	})

	t.Run("finite update overflow", func(t *testing.T) {
		c := controllerUpdateCacheController([]float64{math.MaxFloat64, 0, 0, 0, 0})
		if _, err := c.Release(0, SourceContext{Activity: []float64{0}}); err != nil {
			t.Fatalf("overflow fixture Release error = %v", err)
		}
		beforeState := c.State()
		beforeLoss, beforeGradient, err := c.Gradient(objective)
		if err != nil {
			t.Fatalf("overflow fixture Gradient error = %v", err)
		}
		beforeBackward, err := c.BackwardActivity(0.75)
		if err != nil {
			t.Fatalf("overflow fixture BackwardActivity error = %v", err)
		}
		beforeParameters := append([]float64(nil), c.Parameters...)
		overflowGradient := make([]float64, len(c.Parameters))
		overflowGradient[0] = -math.MaxFloat64
		if _, err := c.Update(overflowGradient, 2); err == nil {
			t.Fatal("Update with a finite overflowing parameter result returned nil error")
		}
		controllerUpdateCacheAssertControllerFailure(t, c, objective, beforeParameters, beforeState, beforeLoss, beforeGradient, beforeBackward)
	})
}

// MemoryController follows the same cache rule, while its offset has no
// activity reverse entry. The twin proves that the retained activity history,
// updated parameters and next forward are all aligned.
func TestMemoryControllerUpdateCache(t *testing.T) {
	parameters := []float64{0.5, -0.25, 0.1, 0.7, -0.2}
	history := controllerUpdateCacheHistory()
	const target = 0.0
	m := controllerUpdateCacheMemory(parameters)
	controllerUpdateCacheOffsetHistory(t, m, history)
	_, gradient, err := m.Gradient(target)
	if err != nil {
		t.Fatalf("baseline Gradient error = %v", err)
	}
	if !controllerUpdateCacheAnyNonZero(gradient) {
		t.Fatal("baseline MemoryController Gradient is all zero")
	}
	beforeParameters := append([]float64(nil), m.Parameters...)
	updated, err := m.Update(gradient, 0.1)
	if err != nil {
		t.Fatalf("Update error = %v", err)
	}
	if controllerUpdateCacheFloatsBitsEqual(updated, beforeParameters) {
		t.Fatal("successful Update did not change any memory-controller parameter")
	}
	if gotLoss, gotGradient, err := m.Gradient(target); err == nil || gotLoss != 0 || gotGradient != nil {
		t.Fatalf("Gradient after successful Update = (%v, %#v, %v), want zero,nil,error", gotLoss, gotGradient, err)
	}

	next := []float64{0.25}
	gotOffset, err := m.Offset(2, SourceContext{Activity: next})
	if err != nil {
		t.Fatalf("Offset after successful Update error = %v", err)
	}
	twin := controllerUpdateCacheMemory(updated)
	controllerUpdateCacheOffsetHistory(t, twin, history)
	wantOffset, err := twin.Offset(2, SourceContext{Activity: next})
	if err != nil {
		t.Fatalf("twin Offset after Update error = %v", err)
	}
	if math.Float64bits(gotOffset) != math.Float64bits(wantOffset) {
		t.Fatalf("continued Offset = %v, want twin %v", gotOffset, wantOffset)
	}
	gotLoss, gotGradient, err := m.Gradient(target)
	if err != nil {
		t.Fatalf("Gradient after new Offset error = %v", err)
	}
	wantLoss, wantGradient, err := twin.Gradient(target)
	if err != nil {
		t.Fatalf("twin Gradient after new Offset error = %v", err)
	}
	controllerUpdateCacheAssertGradientEqual(t, "MemoryController Gradient after new Offset", gotLoss, gotGradient, wantLoss, wantGradient)
}

func TestMemoryControllerUpdateCacheFailures(t *testing.T) {
	const target = 0.0
	type failureCase struct {
		name       string
		parameters []float64
		history    [][]float64
		next       []float64
		fail       func(*MemoryController, []float64) ([]float64, error)
	}
	cases := []failureCase{
		{
			name:       "invalid rate",
			parameters: []float64{0.5, -0.25, 0.1, 0.7, -0.2},
			history:    controllerUpdateCacheHistory(),
			next:       []float64{0.25},
			fail: func(m *MemoryController, gradient []float64) ([]float64, error) {
				return m.Update(gradient, 0)
			},
		},
		{
			name:       "invalid gradient",
			parameters: []float64{0.5, -0.25, 0.1, 0.7, -0.2},
			history:    controllerUpdateCacheHistory(),
			next:       []float64{0.25},
			fail: func(m *MemoryController, gradient []float64) ([]float64, error) {
				invalid := append([]float64(nil), gradient...)
				invalid[0] = math.NaN()
				return m.Update(invalid, 0.1)
			},
		},
		{
			name:       "finite update overflow",
			parameters: []float64{math.MaxFloat64, 0, 0, 1, 0.25},
			history:    [][]float64{{0}, {0}},
			next:       []float64{0.25},
			fail: func(m *MemoryController, _ []float64) ([]float64, error) {
				gradient := make([]float64, len(m.Parameters))
				gradient[0] = -math.MaxFloat64
				return m.Update(gradient, 2)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := controllerUpdateCacheMemory(tc.parameters)
			controllerUpdateCacheOffsetHistory(t, m, tc.history)
			beforeLoss, beforeGradient, err := m.Gradient(target)
			if err != nil {
				t.Fatalf("baseline Gradient error = %v", err)
			}
			if !controllerUpdateCacheAnyNonZero(beforeGradient) {
				t.Fatal("baseline MemoryController Gradient is all zero")
			}
			beforeParameters := append([]float64(nil), m.Parameters...)
			gradientForUpdate := append([]float64(nil), beforeGradient...)
			if _, err := tc.fail(m, gradientForUpdate); err == nil {
				t.Fatal("failed Update returned nil error")
			}
			if !controllerUpdateCacheFloatsBitsEqual(m.Parameters, beforeParameters) {
				t.Fatalf("failed Update changed parameters to %#v, want %#v", m.Parameters, beforeParameters)
			}
			gotLoss, gotGradient, err := m.Gradient(target)
			if err != nil {
				t.Fatalf("Gradient after failed Update error = %v", err)
			}
			controllerUpdateCacheAssertGradientEqual(t, "Gradient after failed Update", gotLoss, gotGradient, beforeLoss, beforeGradient)

			gotOffset, err := m.Offset(2, SourceContext{Activity: tc.next})
			if err != nil {
				t.Fatalf("continuation Offset error = %v", err)
			}
			twin := controllerUpdateCacheMemory(beforeParameters)
			controllerUpdateCacheOffsetHistory(t, twin, tc.history)
			wantOffset, err := twin.Offset(2, SourceContext{Activity: tc.next})
			if err != nil {
				t.Fatalf("twin continuation Offset error = %v", err)
			}
			if math.Float64bits(gotOffset) != math.Float64bits(wantOffset) {
				t.Fatalf("continuation Offset = %v, want twin %v", gotOffset, wantOffset)
			}
			gotLoss, gotGradient, err = m.Gradient(target)
			if err != nil {
				t.Fatalf("continuation Gradient error = %v", err)
			}
			wantLoss, wantGradient, err := twin.Gradient(target)
			if err != nil {
				t.Fatalf("twin continuation Gradient error = %v", err)
			}
			controllerUpdateCacheAssertGradientEqual(t, "continuation Gradient", gotLoss, gotGradient, wantLoss, wantGradient)
		})
	}
}

func TestControllerGradientNil(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("nil Controller Gradient panicked: %v", recovered)
		}
	}()
	var c *Controller
	loss, gradient, err := c.Gradient(controllerUpdateCacheObjective())
	if err == nil || loss != 0 || gradient != nil {
		t.Fatalf("nil Controller Gradient = (%v, %#v, %v), want zero,nil,error", loss, gradient, err)
	}
}
