package modulation

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/signal"
)

// softplusByHand is the reference the tests compare against. It is written out
// here, rather than called from the source, so a change in the implementation
// cannot quietly move the expected value with it.
func softplusByHand(z float64) float64 { return math.Log1p(math.Exp(z)) }

// The forward pass is small enough to write down. Nodes 2, Hidden 1, a window
// of two rows, no feedback and no resources, so the input is exactly the mean
// and the population variance of the window. W1 = {1, 0} reads the mean and
// ignores the variance, b1 = {0}, w2 = {1}, b2 = {0}.
func TestControllerHandForward(t *testing.T) {
	c := &Controller{
		Inputs:     ControllerInputs{SummaryWindow: 2},
		Nodes:      2,
		Hidden:     1,
		Channel:    1,
		Parameters: []float64{1, 0, 0, 1, 0},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	// The rows before the first call are zeros, so the window holds
	// {0, 0} and {1, 1}: mean 0.5, population variance 0.25 over 4 values.
	first, err := c.Release(0, SourceContext{Activity: []float64{1, 1}})
	if err != nil {
		t.Fatalf("Release(0) error = %v", err)
	}
	if len(first) != c.Channels() || len(first) != 2 {
		t.Fatalf("Release(0) returned %d rates, want %d", len(first), c.Channels())
	}
	if first[0] != 0 {
		t.Fatalf("Release(0) released %v on channel 0, want 0", first[0])
	}
	wantFirst := softplusByHand(math.Tanh(1*0.5 + 0*0.25))
	if math.Abs(first[1]-wantFirst) > 1e-12 {
		t.Fatalf("Release(0) released %v on channel 1, want %v", first[1], wantFirst)
	}

	// The second row pushes the zero row out: the window is {1, 1} twice,
	// mean 1 and variance 0.
	second, err := c.Release(1, SourceContext{Activity: []float64{1, 1}})
	if err != nil {
		t.Fatalf("Release(1) error = %v", err)
	}
	wantSecond := softplusByHand(math.Tanh(1))
	if math.Abs(second[1]-wantSecond) > 1e-12 {
		t.Fatalf("Release(1) released %v on channel 1, want %v", second[1], wantSecond)
	}
	if second[0] != 0 {
		t.Fatalf("Release(1) released %v on channel 0, want 0", second[0])
	}

	// An activity row of the wrong width is an error, and a nil row is the
	// declared Nodes zeros rather than a refusal.
	if _, err := c.Release(2, SourceContext{Activity: []float64{1}}); err == nil {
		t.Fatal("Release() error = nil for an activity row of 1 value and 2 nodes")
	}
	if _, err := c.Release(2, SourceContext{}); err != nil {
		t.Fatalf("Release() with no activity error = %v, want the declared zeros", err)
	}
}

// Feedback and resources enter as one number each, in the declared order. W1
// zeroes the two summary inputs so the hand value depends only on them.
func TestControllerReadsFeedbackAndResources(t *testing.T) {
	c := &Controller{
		Inputs:     ControllerInputs{SummaryWindow: 1, UseFeedback: true, Resources: []string{"energy"}},
		Nodes:      1,
		Hidden:     1,
		Channel:    0,
		Parameters: []float64{0, 0, 1, 1, 0, 1, 0},
	}
	if c.InputWidth() != 4 {
		t.Fatalf("InputWidth() = %d, want 4", c.InputWidth())
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	// x = {4, 0, 0.5, 2}: the window of one row is exactly {4}, the feedback
	// helper scores 0.5, and energy is 2. W1 reads only the last two.
	arrived := SourceContext{
		Activity:  []float64{4},
		Feedback:  []signal.Feedback{stepFeedback(t, 0, 1)},
		Resources: map[string]float64{"energy": 2},
	}
	got, err := c.Release(2, arrived)
	if err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	want := softplusByHand(math.Tanh(0.5 + 2))
	if len(got) != 1 || math.Abs(got[0]-want) > 1e-12 {
		t.Fatalf("Release() = %v, want [%v]", got, want)
	}

	// A resource the task did not declare reads as zero, not as a refusal:
	// the controller declares which resources it reads, and a missing one is
	// an absent amount.
	withoutEnergy := arrived
	withoutEnergy.Resources = nil
	got, err = c.Release(2, withoutEnergy)
	if err != nil {
		t.Fatalf("Release() without the resource error = %v", err)
	}
	want = softplusByHand(math.Tanh(0.5))
	if math.Abs(got[0]-want) > 1e-12 {
		t.Fatalf("Release() without the resource = %v, want [%v]", got, want)
	}

	// With no feedback at all the feedback input is zero.
	withoutFeedback := arrived
	withoutFeedback.Feedback = nil
	got, err = c.Release(2, withoutFeedback)
	if err != nil {
		t.Fatalf("Release() without feedback error = %v", err)
	}
	want = softplusByHand(math.Tanh(2))
	if math.Abs(got[0]-want) > 1e-12 {
		t.Fatalf("Release() without feedback = %v, want [%v]", got, want)
	}

	// Feedback that has not arrived is refused, exactly like every other
	// source.
	early := arrived
	early.Feedback = []signal.Feedback{stepFeedback(t, 0, 3)}
	if _, err := c.Release(2, early); err == nil {
		t.Fatal("Release() error = nil for feedback that arrives at step 3")
	}
}

// The counts are fixed arithmetic on the declared shape, and Validate refuses
// a declaration that cannot produce them.
func TestControllerValidateAndCounts(t *testing.T) {
	valid := Controller{
		Inputs:     ControllerInputs{SummaryWindow: 4, UseFeedback: true, Resources: []string{"energy"}},
		Nodes:      2,
		Hidden:     3,
		Channel:    0,
		Parameters: make([]float64, 19),
	}
	if valid.InputWidth() != 4 {
		t.Fatalf("InputWidth() = %d, want 4", valid.InputWidth())
	}
	if valid.ParameterCount() != 19 {
		t.Fatalf("ParameterCount() = %d, want 3*4+3+3+1 = 19", valid.ParameterCount())
	}
	if valid.MultAddsPerStep() != 15 {
		t.Fatalf("MultAddsPerStep() = %d, want 3*4+3 = 15", valid.MultAddsPerStep())
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if valid.Channels() != 1 {
		t.Fatalf("Channels() = %d, want 1", valid.Channels())
	}

	broken := map[string]func(c *Controller){
		"too few parameters":   func(c *Controller) { c.Parameters = make([]float64, 18) },
		"too many parameters":  func(c *Controller) { c.Parameters = make([]float64, 20) },
		"no window":            func(c *Controller) { c.Inputs.SummaryWindow = 0 },
		"window too long":      func(c *Controller) { c.Inputs.SummaryWindow = 65 },
		"duplicate resource":   func(c *Controller) { c.Inputs.Resources = []string{"energy", "energy"} },
		"blank resource":       func(c *Controller) { c.Inputs.Resources = []string{" "} },
		"no node":              func(c *Controller) { c.Nodes = 0 },
		"no hidden unit":       func(c *Controller) { c.Hidden = 0 },
		"too many hidden":      func(c *Controller) { c.Hidden = 257 },
		"negative channel":     func(c *Controller) { c.Channel = -1 },
		"parameter not finite": func(c *Controller) { c.Parameters[0] = math.NaN() },
	}
	for name, break_ := range broken {
		c := valid
		c.Parameters = append([]float64(nil), valid.Parameters...)
		c.Inputs.Resources = append([]string(nil), valid.Inputs.Resources...)
		break_(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: Validate() error = nil", name)
		}
		if _, err := c.Release(0, SourceContext{Activity: []float64{0, 0}}); err == nil {
			t.Errorf("%s: Release() error = nil", name)
		}
	}
}

// The controller is a declared source kind like the other four, and building
// one copies the body so the built source cannot be reached through the spec.
func TestSourceSpecBuildsAController(t *testing.T) {
	body := Controller{
		Inputs:     ControllerInputs{SummaryWindow: 2},
		Nodes:      2,
		Hidden:     1,
		Channel:    1,
		Parameters: []float64{1, 0, 0, 1, 0},
	}
	spec := SourceSpec{Kind: SourceController, Channel: 1, Controller: &body}
	built, err := spec.Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if built.Channels() != 2 {
		t.Fatalf("built source declares %d channels, want 2", built.Channels())
	}
	if _, ok := built.(*Controller); !ok {
		t.Fatalf("Build() returned %T, want *Controller: the controller carries a window across steps", built)
	}

	// Writing into the spec after the build changes nothing the caller holds.
	body.Parameters[0] = 99
	body.Inputs.Resources = []string{"energy"}
	got, err := built.Release(0, SourceContext{Activity: []float64{1, 1}})
	if err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	want := softplusByHand(math.Tanh(0.5))
	if math.Abs(got[1]-want) > 1e-12 {
		t.Fatalf("Release() released %v on channel 1, want %v: the built source shares the spec's buffers", got[1], want)
	}

	for name, bad := range map[string]SourceSpec{
		"channel differs": {Kind: SourceController, Channel: 0, Controller: &body},
		"no body":         {Kind: SourceController, Channel: 1},
		"body of another kind": {Kind: SourceController, Channel: 0,
			Replay: &Replay{Trace: [][]float64{{1}}}},
		"two bodies": {Kind: SourceController, Channel: 1, Controller: &body,
			Replay: &Replay{Trace: [][]float64{{1}}}},
		"invalid body": {Kind: SourceController, Channel: 0,
			Controller: &Controller{Inputs: ControllerInputs{SummaryWindow: 1}, Nodes: 1, Hidden: 1}},
	} {
		if _, err := bad.Build(); err == nil {
			t.Errorf("%s: Build() error = nil", name)
		}
	}
}

// The controller is trainable, which is exactly why it must not be able to
// name the answer: no field of the controller, of its input declaration or of
// the context it reads is a target.
func TestControllerHasNoTargetField(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(Controller{}),
		reflect.TypeOf(ControllerInputs{}),
		reflect.TypeOf(SourceContext{}),
	} {
		if typ.NumField() == 0 {
			t.Fatalf("%s has no field at all", typ)
		}
		for i := 0; i < typ.NumField(); i++ {
			name := typ.Field(i).Name
			if strings.Contains(strings.ToLower(name), "target") {
				t.Errorf("%s field %d is named %q", typ, i, name)
			}
		}
	}
}
