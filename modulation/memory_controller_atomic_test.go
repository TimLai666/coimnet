package modulation_test

import (
	"math"
	"strconv"
	"testing"

	"github.com/TimLai666/coimnet/modulation"
)

const memoryAtomicTarget = 0.2

func newAtomicMemoryController() *modulation.MemoryController {
	return &modulation.MemoryController{
		Inputs:     modulation.ControllerInputs{SummaryWindow: 2},
		Nodes:      1,
		Hidden:     1,
		Parameters: []float64{1, 0.5, 0, 1, 0.2},
	}
}

func newAtomicResourceMemoryController() *modulation.MemoryController {
	return &modulation.MemoryController{
		Inputs: modulation.ControllerInputs{SummaryWindow: 2, Resources: []string{"energy"}},
		Nodes:  1,
		Hidden: 1,
		// W1 = {1, 0.5, 0}, b1 = {0}, w2 = {1}, b2 = {0.2}.
		Parameters: []float64{1, 0.5, 0, 0, 1, 0.2},
	}
}

func memoryActivityContext(activity float64) modulation.SourceContext {
	return modulation.SourceContext{Activity: []float64{activity}}
}

func memoryResourceContext(activity, amount float64) modulation.SourceContext {
	return modulation.SourceContext{
		Activity:  []float64{activity},
		Resources: map[string]float64{"energy": amount},
	}
}

func assertMemoryScalarBits(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Float64bits(got) != math.Float64bits(want) {
		t.Fatalf("%s = %v (%x), want %v (%x)", name, got, math.Float64bits(got), want, math.Float64bits(want))
	}
}

func assertMemoryGradientBits(t *testing.T, name string, gotLoss float64, gotGrad []float64, wantLoss float64, wantGrad []float64) {
	t.Helper()
	assertMemoryScalarBits(t, name+" loss", gotLoss, wantLoss)
	if len(gotGrad) != len(wantGrad) {
		t.Fatalf("%s gradient length = %d, want %d", name, len(gotGrad), len(wantGrad))
	}
	for i := range gotGrad {
		assertMemoryScalarBits(t, name+" gradient["+strconv.Itoa(i)+"]", gotGrad[i], wantGrad[i])
	}
}

type memoryAtomicCase struct {
	name    string
	newPair func() (*modulation.MemoryController, *modulation.MemoryController)
	first   func() modulation.SourceContext
	fail    func(*modulation.MemoryController) (float64, error)
	next    func() modulation.SourceContext
	prepare func(*modulation.MemoryController) func()
}

func TestMemoryControllerAtomicFailureDoesNotPollute(t *testing.T) {
	cases := []memoryAtomicCase{
		{
			name: "activity-summary-overflow",
			newPair: func() (*modulation.MemoryController, *modulation.MemoryController) {
				return newAtomicMemoryController(), newAtomicMemoryController()
			},
			first: func() modulation.SourceContext { return memoryActivityContext(0.25) },
			fail: func(m *modulation.MemoryController) (float64, error) {
				return m.Offset(1, memoryActivityContext(math.MaxFloat64))
			},
			next: func() modulation.SourceContext { return memoryActivityContext(0.5) },
		},
		{
			name: "declared-resource-NaN",
			newPair: func() (*modulation.MemoryController, *modulation.MemoryController) {
				return newAtomicResourceMemoryController(), newAtomicResourceMemoryController()
			},
			first: func() modulation.SourceContext { return memoryResourceContext(0.25, 0.4) },
			fail: func(m *modulation.MemoryController) (float64, error) {
				return m.Offset(1, memoryResourceContext(0.75, math.NaN()))
			},
			next: func() modulation.SourceContext { return memoryResourceContext(0.5, 0.4) },
		},
		{
			name: "readout-overflow",
			newPair: func() (*modulation.MemoryController, *modulation.MemoryController) {
				return newAtomicMemoryController(), newAtomicMemoryController()
			},
			first: func() modulation.SourceContext { return memoryActivityContext(0.25) },
			prepare: func(m *modulation.MemoryController) func() {
				original := append([]float64(nil), m.Parameters...)
				// W1[0] makes the hidden unit saturate while remaining finite;
				// w2 + b2 then overflows in the linear readout.
				m.Parameters[0] = math.MaxFloat64
				m.Parameters[3] = math.MaxFloat64
				m.Parameters[4] = math.MaxFloat64
				return func() { copy(m.Parameters, original) }
			},
			fail: func(m *modulation.MemoryController) (float64, error) {
				return m.Offset(1, memoryActivityContext(0.75))
			},
			next: func() modulation.SourceContext { return memoryActivityContext(0.5) },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			good, bad := tc.newPair()
			if good == bad {
				t.Fatal("twin controllers share the same object")
			}
			if &good.Parameters[0] == &bad.Parameters[0] {
				t.Fatal("twin controllers share the parameter backing array")
			}
			if err := good.Validate(); err != nil {
				t.Fatalf("good Validate: %v", err)
			}
			if err := bad.Validate(); err != nil {
				t.Fatalf("bad Validate: %v", err)
			}

			goodFirst, err := good.Offset(0, tc.first())
			if err != nil {
				t.Fatalf("good first Offset: %v", err)
			}
			badFirst, err := bad.Offset(0, tc.first())
			if err != nil {
				t.Fatalf("bad first Offset: %v", err)
			}
			assertMemoryScalarBits(t, "first offset", badFirst, goodFirst)

			goodLoss, goodGrad, err := good.Gradient(memoryAtomicTarget)
			if err != nil {
				t.Fatalf("good first Gradient: %v", err)
			}
			badLoss, badGrad, err := bad.Gradient(memoryAtomicTarget)
			if err != nil {
				t.Fatalf("bad first Gradient: %v", err)
			}
			assertMemoryGradientBits(t, "first gradient", badLoss, badGrad, goodLoss, goodGrad)

			var restore func()
			if tc.prepare != nil {
				restore = tc.prepare(bad)
			}
			failedOffset, failErr := tc.fail(bad)
			if restore != nil {
				restore()
			}
			if failErr == nil {
				t.Fatal("failed Offset returned nil error")
			}
			assertMemoryScalarBits(t, "failed offset", failedOffset, 0)

			preservedLoss, preservedGrad, err := bad.Gradient(memoryAtomicTarget)
			if err != nil {
				t.Fatalf("Gradient after failed Offset: %v", err)
			}
			assertMemoryGradientBits(t, "preserved gradient", preservedLoss, preservedGrad, badLoss, badGrad)

			goodNext, err := good.Offset(2, tc.next())
			if err != nil {
				t.Fatalf("good continuation Offset: %v", err)
			}
			badNext, err := bad.Offset(2, tc.next())
			if err != nil {
				t.Fatalf("bad continuation Offset: %v", err)
			}
			assertMemoryScalarBits(t, "continuation offset", badNext, goodNext)

			goodLoss, goodGrad, err = good.Gradient(memoryAtomicTarget)
			if err != nil {
				t.Fatalf("good continuation Gradient: %v", err)
			}
			badLoss, badGrad, err = bad.Gradient(memoryAtomicTarget)
			if err != nil {
				t.Fatalf("bad continuation Gradient: %v", err)
			}
			assertMemoryGradientBits(t, "continuation gradient", badLoss, badGrad, goodLoss, goodGrad)
		})
	}
}

func TestMemoryControllerAtomicNilReceiver(t *testing.T) {
	var m *modulation.MemoryController
	if err := m.Validate(); err == nil {
		t.Fatal("nil Validate returned nil error")
	}
	if offset, err := m.Offset(0, modulation.SourceContext{}); err == nil || math.Float64bits(offset) != 0 {
		t.Fatalf("nil Offset = %v, error %v, want zero and error", offset, err)
	}
	if loss, grad, err := m.Gradient(memoryAtomicTarget); err == nil || math.Float64bits(loss) != 0 || grad != nil {
		t.Fatalf("nil Gradient = loss %v, gradient %v, error %v, want zero, nil and error", loss, grad, err)
	}
	if updated, err := m.Update(nil, 1); err == nil || updated != nil {
		t.Fatalf("nil Update = parameters %v, error %v, want nil and error", updated, err)
	}
}

func TestMemoryControllerRejectsHiddenOverflow(t *testing.T) {
	cases := []struct {
		name       string
		controller *modulation.MemoryController
		context    modulation.SourceContext
		width      int
		count      int
	}{
		{
			name: "hidden-input-product",
			controller: &modulation.MemoryController{
				Inputs: modulation.ControllerInputs{SummaryWindow: 2, Resources: []string{"energy"}},
				Nodes:  1,
				Hidden: 1,
				// W1 = {0, 0, 2}, b1 = {0}, w2 = {1}, b2 = {0.2}.
				Parameters: []float64{0, 0, 2, 0, 1, 0.2},
			},
			context: memoryResourceContext(0.25, math.MaxFloat64),
			width:   3,
			count:   6,
		},
		{
			name: "hidden-sum",
			controller: &modulation.MemoryController{
				Inputs: modulation.ControllerInputs{SummaryWindow: 2, Resources: []string{"energy", "oxygen"}},
				Nodes:  1,
				Hidden: 1,
				// W1 = {0, 0, MaxFloat64, MaxFloat64}, b1 = {0},
				// w2 = {1}, b2 = {0}; each product is finite, their sum is not.
				Parameters: []float64{0, 0, math.MaxFloat64, math.MaxFloat64, 0, 1, 0},
			},
			context: modulation.SourceContext{
				Activity:  []float64{0.25},
				Resources: map[string]float64{"energy": 1, "oxygen": 1},
			},
			width: 4,
			count: 7,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.controller.InputWidth() != tc.width {
				t.Fatalf("InputWidth() = %d, want %d", tc.controller.InputWidth(), tc.width)
			}
			if tc.controller.ParameterCount() != tc.count || len(tc.controller.Parameters) != tc.count {
				t.Fatalf("ParameterCount() = %d with %d values, want %d", tc.controller.ParameterCount(), len(tc.controller.Parameters), tc.count)
			}
			if err := tc.controller.Validate(); err != nil {
				t.Fatalf("Validate() rejected finite overflow fixture: %v", err)
			}
			offset, err := tc.controller.Offset(0, tc.context)
			if err == nil {
				t.Fatal("overflowing hidden computation returned nil error")
			}
			assertMemoryScalarBits(t, "overflow offset", offset, 0)
		})
	}
}
