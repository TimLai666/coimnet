package modulation

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func controllerStateTestRows(rows ...[]float64) [][]float64 {
	out := make([][]float64, len(rows))
	for i, row := range rows {
		out[i] = append([]float64(nil), row...)
	}
	return out
}

func controllerStateTestCloneRows(rows [][]float64) [][]float64 {
	if rows == nil {
		return nil
	}
	out := make([][]float64, len(rows))
	for i, row := range rows {
		out[i] = append([]float64(nil), row...)
	}
	return out
}

func controllerStateTestClone(state ControllerState) ControllerState {
	return ControllerState{Window: controllerStateTestCloneRows(state.Window), Filled: state.Filled}
}

func controllerStateTestRowsBitsEqual(a, b [][]float64) bool {
	if (a == nil) != (b == nil) || len(a) != len(b) {
		return false
	}
	for i := range a {
		if (a[i] == nil) != (b[i] == nil) || len(a[i]) != len(b[i]) {
			return false
		}
		for j := range a[i] {
			if math.Float64bits(a[i][j]) != math.Float64bits(b[i][j]) {
				return false
			}
		}
	}
	return true
}

func controllerStateTestEqual(a, b ControllerState) bool {
	return a.Filled == b.Filled && controllerStateTestRowsBitsEqual(a.Window, b.Window)
}

func controllerStateTestAssertRowsBits(t *testing.T, name string, got, want [][]float64) {
	t.Helper()
	if !controllerStateTestRowsBitsEqual(got, want) {
		t.Fatalf("%s = %#v, want %#v", name, got, want)
	}
}

func controllerStateTestAssertSlicesBits(t *testing.T, name string, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s length = %d, want %d", name, len(got), len(want))
	}
	for i := range got {
		if math.Float64bits(got[i]) != math.Float64bits(want[i]) {
			t.Fatalf("%s[%d] = %v (%x), want %v (%x)", name, i, got[i], math.Float64bits(got[i]), want[i], math.Float64bits(want[i]))
		}
	}
}

func controllerStateTestController() *Controller {
	return &Controller{
		Inputs:  ControllerInputs{SummaryWindow: 3, Resources: []string{"energy", "oxygen"}},
		Nodes:   2,
		Hidden:  2,
		Channel: 1,
		// InputWidth is four: mean, variance, energy and oxygen. The
		// parameters are row-major W1, b1, w2 and b2.
		Parameters: []float64{
			0.7, -0.4, 0.2, -0.1,
			-0.3, 0.6, -0.15, 0.25,
			0.1, -0.2,
			1.1, -0.8,
			0.35,
		},
	}
}

func controllerStateTestContext(activity []float64, energy, oxygen float64) SourceContext {
	return SourceContext{
		Activity:  append([]float64(nil), activity...),
		Resources: map[string]float64{"energy": energy, "oxygen": oxygen},
	}
}

func controllerActivityTestController() *Controller {
	return &Controller{
		Inputs:  ControllerInputs{SummaryWindow: 3},
		Nodes:   2,
		Hidden:  2,
		Channel: 0,
		Parameters: []float64{
			0.7, -0.4,
			-0.3, 0.6,
			0.1, -0.2,
			1.1, -0.8,
			0.35,
		},
	}
}

func controllerActivityTestReference(c *Controller, state ControllerState, upstream float64, exactLargeSoftplusSlope bool) [][]float64 {
	width, hidden := c.InputWidth(), c.Hidden
	total := float64(c.Inputs.SummaryWindow * c.Nodes)
	start := len(state.Window) - state.Filled
	var sum, squares float64
	for _, row := range state.Window[start:] {
		for _, v := range row {
			sum += v
			squares += v * v
		}
	}
	mean := sum / total
	rawVariance := squares/total - mean*mean
	variance := rawVariance
	varianceDerivative := 1.0
	if variance <= 0 {
		variance = 0
		varianceDerivative = 0
	}
	x := make([]float64, width)
	x[0], x[1] = mean, variance
	h := make([]float64, hidden)
	bias1 := hidden * width
	for j := range h {
		z := c.Parameters[bias1+j]
		for i, v := range x {
			z += c.Parameters[j*width+i] * v
		}
		h[j] = math.Tanh(z)
	}
	weights2 := bias1 + hidden
	bias2 := weights2 + hidden
	z2 := c.Parameters[bias2]
	for j, v := range h {
		z2 += c.Parameters[weights2+j] * v
	}
	dSoftplus := 1 / (1 + math.Exp(-z2))
	if exactLargeSoftplusSlope && z2 > 30 {
		dSoftplus = 1
	}
	dMean, dVariance := 0.0, 0.0
	for j, hv := range h {
		dz1 := dSoftplus * c.Parameters[weights2+j] * (1 - hv*hv)
		dMean += dz1 * c.Parameters[j*width]
		dVariance += varianceDerivative * dz1 * c.Parameters[j*width+1]
	}
	gradient := make([][]float64, c.Inputs.SummaryWindow)
	for i := range gradient {
		gradient[i] = make([]float64, c.Nodes)
	}
	for i := start; i < len(state.Window); i++ {
		for j, v := range state.Window[i] {
			gradient[i][j] = upstream * (dMean/total + dVariance*2*(v-mean)/total)
		}
	}
	return gradient
}

func controllerActivityTestRowsClose(got, want [][]float64, absTol, relTol float64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if len(got[i]) != len(want[i]) {
			return false
		}
		for j := range got[i] {
			delta := math.Abs(got[i][j] - want[i][j])
			scale := math.Max(math.Abs(got[i][j]), math.Abs(want[i][j]))
			if delta > absTol+relTol*scale {
				return false
			}
		}
	}
	return true
}

func controllerActivityTestReleaseSequence(t *testing.T, activities [][]float64, row, node int, delta float64) float64 {
	t.Helper()
	c := controllerActivityTestController()
	var finalRate float64
	for step, original := range activities {
		activity := append([]float64(nil), original...)
		if step == row {
			activity[node] += delta
		}
		rates, err := c.Release(uint64(step), SourceContext{Activity: activity})
		if err != nil {
			t.Fatalf("Release(%d) error = %v", step, err)
		}
		finalRate = rates[0]
	}
	return finalRate
}

func controllerStateTestAssertGradientSame(t *testing.T, name string, gotLoss float64, gotGrad []float64, wantLoss float64, wantGrad []float64) {
	t.Helper()
	if math.Float64bits(gotLoss) != math.Float64bits(wantLoss) {
		t.Fatalf("%s loss = %v (%x), want %v (%x)", name, gotLoss, math.Float64bits(gotLoss), wantLoss, math.Float64bits(wantLoss))
	}
	controllerStateTestAssertSlicesBits(t, name+" gradient", gotGrad, wantGrad)
}

func TestControllerStateJSONResumeAndOwnership(t *testing.T) {
	activities := [][]float64{{0.25, -0.75}, {1.5, 0.5}, {-1.25, 2}}
	resources := [][2]float64{{1.25, -0.5}, {2.5, 0.75}, {-1, 3}}
	c := controllerStateTestController()

	for step, activity := range activities {
		if _, err := c.Release(uint64(step), controllerStateTestContext(activity, resources[step][0], resources[step][1])); err != nil {
			t.Fatalf("Release(%d) error = %v", step, err)
		}
		state := c.State()
		wantRows := make([][]float64, 3)
		for i := range wantRows {
			wantRows[i] = []float64{0, 0}
		}
		for i := 0; i <= step; i++ {
			wantRows[3-step-1+i] = append([]float64(nil), activities[i]...)
		}
		want := ControllerState{Window: wantRows, Filled: step + 1}
		if !controllerStateTestEqual(state, want) {
			t.Fatalf("State() after step %d = %#v, want %#v", step, state, want)
		}

		// State owns every row. Mutating the returned snapshot must not move
		// the controller's ring or change a future serialized boundary.
		ownedSnapshot := controllerStateTestClone(state)
		state.Window[len(state.Window)-1][0] += 100
		if !controllerStateTestEqual(c.State(), ownedSnapshot) {
			t.Fatalf("State() shares its returned window after step %d", step)
		}

		encoded, err := json.Marshal(ownedSnapshot)
		if err != nil {
			t.Fatalf("Marshal state at step %d: %v", step, err)
		}
		var decoded ControllerState
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("Unmarshal state at step %d: %v", step, err)
		}
		if !strings.Contains(string(encoded), `"window"`) || !strings.Contains(string(encoded), `"filled"`) || strings.Contains(string(encoded), "last") {
			t.Fatalf("State JSON at step %d has an unexpected schema: %s", step, encoded)
		}
		resumed := controllerStateTestController()
		if err := resumed.RestoreState(decoded); err != nil {
			t.Fatalf("RestoreState at step %d: %v", step, err)
		}
		decoded.Window[len(decoded.Window)-1][0] += 200
		if !controllerStateTestEqual(resumed.State(), ownedSnapshot) {
			t.Fatalf("RestoreState at step %d shares the JSON input window", step)
		}

		if step+1 < len(activities) {
			reference := controllerStateTestController()
			for prior, priorActivity := range activities[:step+1] {
				if _, err := reference.Release(uint64(prior), controllerStateTestContext(priorActivity, resources[prior][0], resources[prior][1])); err != nil {
					t.Fatalf("reference Release(%d) error = %v", prior, err)
				}
			}
			wantNext, err := reference.Release(uint64(step+1), controllerStateTestContext(activities[step+1], resources[step+1][0], resources[step+1][1]))
			if err != nil {
				t.Fatalf("reference continuation Release(%d) error = %v", step+1, err)
			}
			gotNext, err := resumed.Release(uint64(step+1), controllerStateTestContext(activities[step+1], resources[step+1][0], resources[step+1][1]))
			if err != nil {
				t.Fatalf("resumed continuation Release(%d) error = %v", step+1, err)
			}
			controllerStateTestAssertSlicesBits(t, "resumed next release", gotNext, wantNext)
		}
	}

	// A nil window is the legacy fresh boundary. It is valid only with Filled=0.
	if state := controllerStateTestController().State(); !controllerStateTestEqual(state, ControllerState{}) {
		t.Fatalf("fresh State() = %#v, want the empty zero boundary", state)
	}
	fresh := controllerStateTestController()
	if err := fresh.RestoreState(ControllerState{Window: nil, Filled: 0}); err != nil {
		t.Fatalf("RestoreState(nil, 0) error = %v", err)
	}
	wantFresh := controllerStateTestController()
	activity := []float64{0.4, -0.2}
	wantRates, err := wantFresh.Release(0, controllerStateTestContext(activity, 1, 2))
	if err != nil {
		t.Fatalf("fresh reference Release error = %v", err)
	}
	gotRates, err := fresh.Release(0, controllerStateTestContext(activity, 1, 2))
	if err != nil {
		t.Fatalf("nil-window Release error = %v", err)
	}
	controllerStateTestAssertSlicesBits(t, "nil-window fresh release", gotRates, wantRates)
}

func TestControllerStateRestoreValidationAtomic(t *testing.T) {
	c := controllerActivityTestController()
	if _, err := c.Release(0, SourceContext{Activity: []float64{0.4, -0.7}}); err != nil {
		t.Fatalf("Release error = %v", err)
	}
	if _, err := c.Release(1, SourceContext{Activity: []float64{1.1, 0.2}}); err != nil {
		t.Fatalf("Release second error = %v", err)
	}
	baselineState := controllerStateTestClone(c.State())
	if len(baselineState.Window) != c.Inputs.SummaryWindow {
		t.Fatalf("State() has %d rows, want %d", len(baselineState.Window), c.Inputs.SummaryWindow)
	}
	objective := ControllerObjective{Kind: ObjectiveActivityTarget, Target: 0.35}
	baselineLoss, baselineGradient, err := c.Gradient(objective)
	if err != nil {
		t.Fatalf("Gradient baseline error = %v", err)
	}
	baselineBackward, err := c.BackwardActivity(-0.8)
	if err != nil {
		t.Fatalf("BackwardActivity baseline error = %v", err)
	}

	invalid := []struct {
		name  string
		state ControllerState
	}{
		{"nil window with filled row", ControllerState{Window: nil, Filled: 1}},
		{"nil window with negative filled", ControllerState{Window: nil, Filled: -1}},
		{"nil window overfilled", ControllerState{Window: nil, Filled: 4}},
		{"non-nil empty window", ControllerState{Window: [][]float64{}, Filled: 0}},
		{"wrong row count", ControllerState{Window: controllerStateTestRows([]float64{0, 0}, []float64{0, 0}), Filled: 0}},
		{"wrong node count", ControllerState{Window: controllerStateTestRows([]float64{0, 0}, []float64{0}, []float64{0, 0}), Filled: 0}},
		{"negative filled", ControllerState{Window: controllerStateTestRows([]float64{0, 0}, []float64{0, 0}, []float64{0, 0}), Filled: -1}},
		{"overfilled", ControllerState{Window: controllerStateTestRows([]float64{0, 0}, []float64{0, 0}, []float64{0, 0}), Filled: 4}},
		{"non-finite NaN", ControllerState{Window: controllerStateTestRows([]float64{0, 0}, []float64{0, math.NaN()}, []float64{0, 0}), Filled: 2}},
		{"non-finite infinity", ControllerState{Window: controllerStateTestRows([]float64{0, 0}, []float64{0, math.Inf(1)}, []float64{0, 0}), Filled: 2}},
		{"unfilled oldest row is nonzero", ControllerState{Window: controllerStateTestRows([]float64{0.25, 0}, []float64{0, 0}, []float64{1, 2}), Filled: 1}},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if err := c.RestoreState(tc.state); err == nil {
				t.Fatalf("RestoreState(%s) error = nil", tc.name)
			}
			if !controllerStateTestEqual(c.State(), baselineState) {
				t.Fatalf("RestoreState(%s) changed the existing window", tc.name)
			}
			loss, gradient, err := c.Gradient(objective)
			if err != nil {
				t.Fatalf("Gradient after RestoreState(%s) error = %v", tc.name, err)
			}
			controllerStateTestAssertGradientSame(t, "Gradient after invalid restore", loss, gradient, baselineLoss, baselineGradient)
			backward, err := c.BackwardActivity(-0.8)
			if err != nil {
				t.Fatalf("BackwardActivity after RestoreState(%s) error = %v", tc.name, err)
			}
			controllerStateTestAssertRowsBits(t, "BackwardActivity after invalid restore", backward, baselineBackward)
		})
	}

	// A valid restore owns its input and deliberately clears the prior release cache.
	restoredInput := controllerStateTestClone(baselineState)
	if err := c.RestoreState(restoredInput); err != nil {
		t.Fatalf("RestoreState(valid) error = %v", err)
	}
	restoredInput.Window[len(restoredInput.Window)-1][0] += 99
	if !controllerStateTestEqual(c.State(), baselineState) {
		t.Fatalf("RestoreState(valid) shares its input window")
	}
	if _, _, err := c.Gradient(objective); err == nil {
		t.Fatal("Gradient() error = nil after a valid restore cleared the release cache")
	}
	if got, err := c.BackwardActivity(1); err == nil || got != nil {
		t.Fatalf("BackwardActivity() after restore = (%v, %v), want nil,error", got, err)
	}
	if _, err := c.Release(2, SourceContext{Activity: []float64{0.3, -0.4}}); err != nil {
		t.Fatalf("Release after valid restore error = %v", err)
	}
	if _, _, err := c.Gradient(objective); err != nil {
		t.Fatalf("Gradient after the next Release error = %v", err)
	}
	if _, err := c.BackwardActivity(1); err != nil {
		t.Fatalf("BackwardActivity after the next Release error = %v", err)
	}

	zeroWindow := controllerActivityTestController()
	if err := zeroWindow.RestoreState(ControllerState{Window: controllerStateTestRows([]float64{0, 0}, []float64{0, 0}, []float64{0, 0}), Filled: 0}); err != nil {
		t.Fatalf("RestoreState(full zero window, filled=0) error = %v", err)
	}

	var nilController *Controller
	if err := nilController.RestoreState(ControllerState{}); err == nil {
		t.Fatal("nil Controller.RestoreState() error = nil")
	}
	if err := (&Controller{}).RestoreState(ControllerState{}); err == nil {
		t.Fatal("zero Controller.RestoreState() error = nil")
	}
	invalidDeclaration := controllerActivityTestController()
	invalidDeclaration.Inputs.SummaryWindow = 0
	if err := invalidDeclaration.RestoreState(ControllerState{}); err == nil {
		t.Fatal("invalid declaration RestoreState() error = nil")
	}
}

func TestControllerStateCloneDeepOwnership(t *testing.T) {
	original := controllerStateTestController()
	if _, err := original.Release(0, controllerStateTestContext([]float64{0.25, -0.75}, 1, 2)); err != nil {
		t.Fatalf("original Release error = %v", err)
	}
	beforeState := controllerStateTestClone(original.State())
	objective := ControllerObjective{Kind: ObjectiveActivityTarget, Target: 0.35}
	originalLoss, originalGradient, err := original.Gradient(objective)
	if err != nil {
		t.Fatalf("original Gradient error = %v", err)
	}
	originalBackward, err := original.BackwardActivity(1.25)
	if err != nil {
		t.Fatalf("original BackwardActivity error = %v", err)
	}
	repeatedLoss, repeatedGradient, err := original.Gradient(objective)
	if err != nil {
		t.Fatalf("repeated original Gradient error = %v", err)
	}
	controllerStateTestAssertGradientSame(t, "repeated original Gradient", repeatedLoss, repeatedGradient, originalLoss, originalGradient)
	repeatedBackward, err := original.BackwardActivity(1.25)
	if err != nil {
		t.Fatalf("repeated original BackwardActivity error = %v", err)
	}
	controllerStateTestAssertRowsBits(t, "repeated original BackwardActivity", repeatedBackward, originalBackward)

	var nilController *Controller
	if nilController.Clone() != nil {
		t.Fatal("nil Controller.Clone() returned a value")
	}
	clone := original.Clone()
	if clone == nil {
		t.Fatal("Clone() returned nil for a live controller")
	}
	if !reflect.DeepEqual(clone.Inputs, original.Inputs) || clone.Nodes != original.Nodes || clone.Hidden != original.Hidden || clone.Channel != original.Channel {
		t.Fatalf("Clone() declaration = %#v, want %#v", clone, original)
	}
	controllerStateTestAssertSlicesBits(t, "clone parameters", clone.Parameters, original.Parameters)
	cloneLoss, cloneGradient, err := clone.Gradient(objective)
	if err != nil {
		t.Fatalf("clone Gradient error = %v", err)
	}
	controllerStateTestAssertGradientSame(t, "clone Gradient", cloneLoss, cloneGradient, originalLoss, originalGradient)
	cloneBackward, err := clone.BackwardActivity(1.25)
	if err != nil {
		t.Fatalf("clone BackwardActivity error = %v", err)
	}
	controllerStateTestAssertRowsBits(t, "clone BackwardActivity", cloneBackward, originalBackward)
	if !controllerStateTestEqual(clone.State(), beforeState) {
		t.Fatalf("Clone() did not copy the activity window")
	}

	// A clone and its source must make the same next release bit-for-bit.
	parityClone := original.Clone()
	nextActivity := []float64{1.5, 0.5}
	originalNext, err := original.Release(1, controllerStateTestContext(nextActivity, 2.5, 0.75))
	if err != nil {
		t.Fatalf("original next Release error = %v", err)
	}
	cloneNext, err := parityClone.Release(1, controllerStateTestContext(nextActivity, 2.5, 0.75))
	if err != nil {
		t.Fatalf("clone next Release error = %v", err)
	}
	controllerStateTestAssertSlicesBits(t, "clone next release", cloneNext, originalNext)

	// Mutating every owned public buffer and advancing a different clone must
	// leave the source's declaration, ring and last-release cache untouched.
	independentSource := controllerStateTestController()
	if _, err := independentSource.Release(0, controllerStateTestContext([]float64{0.25, -0.75}, 1, 2)); err != nil {
		t.Fatalf("independence source Release error = %v", err)
	}
	independentBeforeState := controllerStateTestClone(independentSource.State())
	independentBeforeLoss, independentBeforeGradient, err := independentSource.Gradient(objective)
	if err != nil {
		t.Fatalf("independence source Gradient error = %v", err)
	}
	independentBeforeBackward, err := independentSource.BackwardActivity(1.25)
	if err != nil {
		t.Fatalf("independence source BackwardActivity error = %v", err)
	}
	independentClone := independentSource.Clone()
	independentClone.Inputs.Resources[0] = "changed-energy"
	independentClone.Parameters[0] += 3
	if _, err := independentClone.Release(1, controllerStateTestContext([]float64{-4, 5}, 2.5, 0.75)); err != nil {
		t.Fatalf("independent clone Release error = %v", err)
	}
	if independentSource.Inputs.Resources[0] != "energy" || independentSource.Parameters[0] != 0.7 {
		t.Fatalf("mutating clone changed the source declaration or parameters")
	}
	if !controllerStateTestEqual(independentSource.State(), independentBeforeState) {
		t.Fatalf("advancing clone changed the source window")
	}
	afterLoss, afterGradient, err := independentSource.Gradient(objective)
	if err != nil {
		t.Fatalf("source Gradient after clone advance error = %v", err)
	}
	controllerStateTestAssertGradientSame(t, "source Gradient after clone advance", afterLoss, afterGradient, independentBeforeLoss, independentBeforeGradient)
	afterBackward, err := independentSource.BackwardActivity(1.25)
	if err != nil {
		t.Fatalf("source BackwardActivity after clone advance error = %v", err)
	}
	controllerStateTestAssertRowsBits(t, "source BackwardActivity after clone advance", afterBackward, independentBeforeBackward)
}

func TestControllerActivityGradientHandFormula(t *testing.T) {
	c := &Controller{
		Inputs:     ControllerInputs{SummaryWindow: 3},
		Nodes:      2,
		Hidden:     1,
		Channel:    0,
		Parameters: []float64{0.7, -0.4, 0.15, -1.3, 0.2},
	}
	if _, err := c.Release(0, SourceContext{Activity: []float64{1.2, -0.7}}); err != nil {
		t.Fatalf("Release error = %v", err)
	}
	state := c.State()
	total := 6.0
	mean := (1.2 - 0.7) / total
	variance := (1.2*1.2+0.7*0.7)/total - mean*mean
	h := math.Tanh(0.7*mean - 0.4*variance + 0.15)
	z2 := -1.3*h + 0.2
	dSoftplus := 1 / (1 + math.Exp(-z2))
	dMean := dSoftplus * -1.3 * (1 - h*h) * 0.7
	dVariance := dSoftplus * -1.3 * (1 - h*h) * -0.4

	for _, upstream := range []float64{-1.75, 0.5, 0} {
		got, err := c.BackwardActivity(upstream)
		if err != nil {
			t.Fatalf("BackwardActivity(%v) error = %v", upstream, err)
		}
		want := make([][]float64, 3)
		want[0], want[1] = []float64{0, 0}, []float64{0, 0}
		want[2] = []float64{
			upstream * (dMean/total + dVariance*2*(1.2-mean)/total),
			upstream * (dMean/total + dVariance*2*(-0.7-mean)/total),
		}
		if !controllerActivityTestRowsClose(got, want, 1e-14, 1e-12) {
			t.Fatalf("BackwardActivity(%v) = %#v, want %#v", upstream, got, want)
		}
		if got[0][0] != 0 || got[0][1] != 0 || got[1][0] != 0 || got[1][1] != 0 {
			t.Fatalf("BackwardActivity(%v) gave a gradient to an unfilled row: %#v", upstream, got)
		}
	}
	if !controllerStateTestEqual(c.State(), state) {
		t.Fatal("BackwardActivity changed the controller state")
	}
}

func TestControllerActivityGradientCentralDifferences(t *testing.T) {
	activities := [][]float64{{0.45, -0.8}, {-1.1, 0.7}, {0.9, 1.25}}
	c := controllerActivityTestController()
	for step, activity := range activities {
		if _, err := c.Release(uint64(step), SourceContext{Activity: activity}); err != nil {
			t.Fatalf("Release(%d) error = %v", step, err)
		}
	}
	upstream := -1.3
	got, err := c.BackwardActivity(upstream)
	if err != nil {
		t.Fatalf("BackwardActivity error = %v", err)
	}
	want := controllerActivityTestReference(c, c.State(), upstream, true)
	if !controllerActivityTestRowsClose(got, want, 1e-14, 1e-12) {
		t.Fatalf("BackwardActivity = %#v, hand reference = %#v", got, want)
	}

	const epsilon = 1e-6
	for row := range activities {
		for node := range activities[row] {
			plus := controllerActivityTestReleaseSequence(t, activities, row, node, epsilon)
			minus := controllerActivityTestReleaseSequence(t, activities, row, node, -epsilon)
			finiteDifference := upstream * (plus - minus) / (2 * epsilon)
			if !controllerActivityTestRowsClose([][]float64{{got[row][node]}}, [][]float64{{finiteDifference}}, 2e-7, 2e-6) {
				t.Fatalf("central difference row=%d node=%d = %v, BackwardActivity = %v", row, node, finiteDifference, got[row][node])
			}
		}
	}
	var summedGradient float64
	for _, row := range want {
		for _, value := range row {
			summedGradient += value
		}
	}
	if math.Abs(summedGradient) < 1e-8 {
		t.Fatalf("central-difference fixture did not exercise the mean path: summed gradient=%g", summedGradient)
	}
	if math.Abs(want[0][0]-want[0][1]) < 1e-8 {
		t.Fatalf("central-difference fixture did not exercise the variance path: first-row gradients=%v", want[0])
	}
}

func TestControllerActivityGradientBranchesAndErrors(t *testing.T) {
	t.Run("variance clamp has zero derivative", func(t *testing.T) {
		c := &Controller{
			Inputs:     ControllerInputs{SummaryWindow: 2},
			Nodes:      1,
			Hidden:     1,
			Channel:    0,
			Parameters: []float64{0, 1, 0, 1, 0},
		}
		for step, activity := range []float64{1, 1.0000000000000004} {
			if _, err := c.Release(uint64(step), SourceContext{Activity: []float64{activity}}); err != nil {
				t.Fatalf("Release(%d) error = %v", step, err)
			}
		}
		// The population-variance subtraction rounds to exactly zero for
		// these distinct values. A missing clamp derivative would be
		// amplified by this finite upstream and produce a visible gradient.
		got, err := c.BackwardActivity(1e16)
		if err != nil {
			t.Fatalf("BackwardActivity error = %v", err)
		}
		for i := range got {
			if len(got[i]) != 1 || got[i][0] != 0 {
				t.Fatalf("variance-clamped gradient = %#v, want all zeros", got)
			}
		}
	})

	t.Run("softplus large branch has unit derivative", func(t *testing.T) {
		c := &Controller{
			Inputs:     ControllerInputs{SummaryWindow: 2},
			Nodes:      1,
			Hidden:     1,
			Channel:    0,
			Parameters: []float64{0.8, -0.5, 0.2, 2, 31},
		}
		var finalRates []float64
		for step, activity := range [][]float64{{0.3}, {1.1}} {
			rates, err := c.Release(uint64(step), SourceContext{Activity: activity})
			if err != nil {
				t.Fatalf("Release(%d) error = %v", step, err)
			}
			finalRates = rates
		}
		forwardMean := (0.3 + 1.1) / 2
		forwardVariance := (0.3*0.3+1.1*1.1)/2 - forwardMean*forwardMean
		forwardHiddenInput := 0.2
		forwardHiddenInput += 0.8 * forwardMean
		forwardHiddenInput += -0.5 * forwardVariance
		forwardHidden := math.Tanh(forwardHiddenInput)
		forwardReadout := 31.0
		forwardReadout += 2 * forwardHidden
		if len(finalRates) != 1 || math.Float64bits(finalRates[0]) != math.Float64bits(forwardReadout) {
			t.Fatalf("large-softplus Release() = %v, want branch output %v", finalRates, forwardReadout)
		}
		state := c.State()
		got, err := c.BackwardActivity(1)
		if err != nil {
			t.Fatalf("BackwardActivity error = %v", err)
		}
		want := controllerActivityTestReference(c, state, 1, true)
		naive := controllerActivityTestReference(c, state, 1, false)
		if !controllerActivityTestRowsClose(got, want, 1e-14, 1e-12) {
			t.Fatalf("large-softplus BackwardActivity = %#v, want %#v", got, want)
		}
		maxNaiveDelta := 0.0
		for i := range want {
			for j := range want[i] {
				maxNaiveDelta = math.Max(maxNaiveDelta, math.Abs(want[i][j]-naive[i][j]))
			}
		}
		if maxNaiveDelta < 1e-15 {
			t.Fatalf("large-softplus fixture did not distinguish unit branch from logistic derivative: delta=%g", maxNaiveDelta)
		}
	})

	t.Run("nil zero no-release and invalid declaration reject", func(t *testing.T) {
		var nilController *Controller
		if got, err := nilController.BackwardActivity(1); err == nil || got != nil {
			t.Fatalf("nil controller BackwardActivity = (%v, %v), want nil,error", got, err)
		}
		if got, err := (&Controller{}).BackwardActivity(1); err == nil || got != nil {
			t.Fatalf("zero controller BackwardActivity = (%v, %v), want nil,error", got, err)
		}
		fresh := controllerActivityTestController()
		if got, err := fresh.BackwardActivity(1); err == nil || got != nil {
			t.Fatalf("no-release BackwardActivity = (%v, %v), want nil,error", got, err)
		}
		bad := controllerActivityTestController()
		if _, err := bad.Release(0, SourceContext{Activity: []float64{0.1, 0.2}}); err != nil {
			t.Fatalf("bad declaration setup Release error = %v", err)
		}
		before := controllerStateTestClone(bad.State())
		bad.Inputs.SummaryWindow = 0
		if got, err := bad.BackwardActivity(1); err == nil || got != nil {
			t.Fatalf("invalid declaration BackwardActivity = (%v, %v), want nil,error", got, err)
		}
		if !controllerStateTestEqual(bad.State(), before) {
			t.Fatal("invalid declaration BackwardActivity changed the window")
		}
	})

	t.Run("bad shape and non-finite cache reject", func(t *testing.T) {
		badShape := controllerActivityTestController()
		if _, err := badShape.Release(0, SourceContext{Activity: []float64{0.1, 0.2}}); err != nil {
			t.Fatalf("bad-shape setup Release error = %v", err)
		}
		before := controllerStateTestClone(badShape.State())
		badShape.Inputs.SummaryWindow = 4 // Validate still passes; the cached ring no longer matches.
		if got, err := badShape.BackwardActivity(1); err == nil || got != nil {
			t.Fatalf("bad-shape BackwardActivity = (%v, %v), want nil,error", got, err)
		}
		if !controllerStateTestEqual(badShape.State(), before) {
			t.Fatal("bad-shape BackwardActivity changed the window")
		}

		nonFiniteCache := &Controller{
			Inputs:     ControllerInputs{SummaryWindow: 1},
			Nodes:      2,
			Hidden:     1,
			Channel:    0,
			Parameters: []float64{1, 0, 0, 1, 0},
		}
		if _, err := nonFiniteCache.Release(0, SourceContext{Activity: []float64{0.1, 0.2}}); err != nil {
			t.Fatalf("non-finite-cache setup Release error = %v", err)
		}
		// Corrupt an otherwise valid private cache directly. A forward overflow
		// must be rejected rather than required to succeed as fixture setup.
		nonFiniteCache.lastX[0] = math.NaN()
		before = controllerStateTestClone(nonFiniteCache.State())
		if got, err := nonFiniteCache.BackwardActivity(1); err == nil || got != nil {
			t.Fatalf("non-finite-cache BackwardActivity = (%v, %v), want nil,error", got, err)
		}
		if !controllerStateTestEqual(nonFiniteCache.State(), before) {
			t.Fatal("non-finite-cache BackwardActivity changed the window")
		}
	})

	t.Run("non-finite upstream and overflow reject without mutation", func(t *testing.T) {
		c := controllerActivityTestController()
		if _, err := c.Release(0, SourceContext{Activity: []float64{0.1, 0.2}}); err != nil {
			t.Fatalf("upstream setup Release error = %v", err)
		}
		beforeState := controllerStateTestClone(c.State())
		objective := ControllerObjective{Kind: ObjectiveActivityTarget, Target: 0.35}
		beforeLoss, beforeGradient, err := c.Gradient(objective)
		if err != nil {
			t.Fatalf("upstream setup Gradient error = %v", err)
		}
		beforeBackward, err := c.BackwardActivity(1)
		if err != nil {
			t.Fatalf("upstream setup BackwardActivity error = %v", err)
		}
		for _, upstream := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			if got, err := c.BackwardActivity(upstream); err == nil || got != nil {
				t.Fatalf("BackwardActivity(%v) = (%v, %v), want nil,error", upstream, got, err)
			}
			if !controllerStateTestEqual(c.State(), beforeState) {
				t.Fatalf("BackwardActivity(%v) changed the state", upstream)
			}
			loss, gradient, err := c.Gradient(objective)
			if err != nil {
				t.Fatalf("Gradient after BackwardActivity(%v) error = %v", upstream, err)
			}
			controllerStateTestAssertGradientSame(t, "Gradient after invalid upstream", loss, gradient, beforeLoss, beforeGradient)
			backward, err := c.BackwardActivity(1)
			if err != nil {
				t.Fatalf("valid BackwardActivity after invalid upstream error = %v", err)
			}
			controllerStateTestAssertRowsBits(t, "BackwardActivity after invalid upstream", backward, beforeBackward)
		}

		overflow := &Controller{
			Inputs:     ControllerInputs{SummaryWindow: 1},
			Nodes:      1,
			Hidden:     1,
			Channel:    0,
			Parameters: []float64{0.5, 0, 0, math.MaxFloat64, 0},
		}
		if _, err := overflow.Release(0, SourceContext{Activity: []float64{1}}); err != nil {
			t.Fatalf("overflow setup Release error = %v", err)
		}
		beforeState = controllerStateTestClone(overflow.State())
		if got, err := overflow.BackwardActivity(math.MaxFloat64); err == nil || got != nil {
			t.Fatalf("overflow BackwardActivity = (%v, %v), want nil,error", got, err)
		}
		if !controllerStateTestEqual(overflow.State(), beforeState) {
			t.Fatal("overflow BackwardActivity changed the state")
		}
	})
}

func TestControllerReleaseAtomic(t *testing.T) {
	c := controllerStateTestController()
	first := []float64{0.5, -0.25}
	if _, err := c.Release(0, controllerStateTestContext(first, 1, 2)); err != nil {
		t.Fatalf("first Release error = %v", err)
	}
	beforeState := controllerStateTestClone(c.State())
	objective := ControllerObjective{Kind: ObjectiveActivityTarget, Target: 0.35}
	beforeLoss, beforeGradient, err := c.Gradient(objective)
	if err != nil {
		t.Fatalf("baseline Gradient error = %v", err)
	}
	beforeBackward, err := c.BackwardActivity(-0.75)
	if err != nil {
		t.Fatalf("baseline BackwardActivity error = %v", err)
	}
	if _, err := c.Release(1, SourceContext{
		Activity:  []float64{9, -8},
		Resources: map[string]float64{"energy": math.NaN(), "oxygen": 2},
	}); err == nil {
		t.Fatal("Release() error = nil for a non-finite declared resource")
	}
	if !controllerStateTestEqual(c.State(), beforeState) {
		t.Fatal("failed resource Release changed the activity window")
	}
	afterLoss, afterGradient, err := c.Gradient(objective)
	if err != nil {
		t.Fatalf("Gradient after failed Release error = %v", err)
	}
	controllerStateTestAssertGradientSame(t, "Gradient after failed Release", afterLoss, afterGradient, beforeLoss, beforeGradient)
	afterBackward, err := c.BackwardActivity(-0.75)
	if err != nil {
		t.Fatalf("BackwardActivity after failed Release error = %v", err)
	}
	controllerStateTestAssertRowsBits(t, "BackwardActivity after failed Release", afterBackward, beforeBackward)

	// The failed call must not consume the step. A valid continuation is the
	// same as a fresh controller that saw only the valid first boundary.
	continuation := controllerStateTestController()
	if _, err := continuation.Release(0, controllerStateTestContext(first, 1, 2)); err != nil {
		t.Fatalf("continuation first Release error = %v", err)
	}
	second := []float64{1.25, 0.75}
	want, err := continuation.Release(1, controllerStateTestContext(second, 2.5, 0.75))
	if err != nil {
		t.Fatalf("continuation second Release error = %v", err)
	}
	got, err := c.Release(1, controllerStateTestContext(second, 2.5, 0.75))
	if err != nil {
		t.Fatalf("post-error valid Release error = %v", err)
	}
	controllerStateTestAssertSlicesBits(t, "post-error release", got, want)

	// A malformed first observation must fail before allocating or consuming any
	// ring row, and it must not create a release cache.
	fresh := controllerStateTestController()
	freshBefore := controllerStateTestClone(fresh.State())
	if _, err := fresh.Release(0, SourceContext{Activity: []float64{1}}); err == nil {
		t.Fatal("first shape-invalid Release() error = nil")
	}
	if !controllerStateTestEqual(fresh.State(), freshBefore) {
		t.Fatal("shape-invalid first Release changed the activity window")
	}
	if _, err := fresh.BackwardActivity(1); err == nil {
		t.Fatal("BackwardActivity() error = nil after shape-invalid first Release")
	}
	if _, _, err := fresh.Gradient(objective); err == nil {
		t.Fatal("Gradient() error = nil after shape-invalid first Release")
	}

	// Finite observations can overflow the summary. They must not be silently
	// converted to a zero rate or replace the last successful boundary.
	overflowBefore := controllerStateTestClone(c.State())
	if got, err := c.Release(2, controllerStateTestContext([]float64{math.MaxFloat64, math.MaxFloat64}, 1, 2)); err == nil || got != nil {
		t.Fatalf("overflow Release = (%v, %v), want nil,error", got, err)
	}
	if !controllerStateTestEqual(c.State(), overflowBefore) {
		t.Fatal("overflow Release changed the activity window")
	}
}
