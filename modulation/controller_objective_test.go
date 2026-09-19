package modulation

import (
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/signal"
)

// One fixture shape serves every test below: 3 nodes, 4 hidden units, a window
// of two rows, the feedback input and one declared resource. x is then 4 wide
// (activity mean, activity variance, feedback score, energy) and the parameter
// vector 4*4 + 4 + 4 + 1 = 25 long.
const (
	objectiveNodes  = 3
	objectiveHidden = 4
	objectiveWidth  = 4
	objectiveCount  = 25
	objectiveScore  = 0.3
	objectiveEnergy = 1.5
)

// objectiveFixture draws the parameters and the two activity rows from one
// fixed generator, so every run reads the same numbers on every machine.
func objectiveFixture() (parameters []float64, rows [][]float64) {
	prng := rand.New(rand.NewPCG(11, 0))
	parameters = make([]float64, objectiveCount)
	for i := range parameters {
		parameters[i] = 2*prng.Float64() - 1
	}
	rows = make([][]float64, 2)
	for i := range rows {
		rows[i] = make([]float64, objectiveNodes)
		for j := range rows[i] {
			rows[i][j] = prng.Float64()
		}
	}
	return parameters, rows
}

// objectiveController is the fixture controller carrying the given parameters.
// Every call builds a fresh one, so a perturbed replay starts its window from
// the same zeros as the replay it is compared against.
func objectiveController(parameters []float64) *Controller {
	return &Controller{
		Inputs:     ControllerInputs{SummaryWindow: 2, UseFeedback: true, Resources: []string{"energy"}},
		Nodes:      objectiveNodes,
		Hidden:     objectiveHidden,
		Channel:    0,
		Parameters: append([]float64(nil), parameters...),
	}
}

// objectiveStep is the context of one release: one activity row, one feedback
// that has already arrived and scores 0.3, and the declared resource.
func objectiveStep(t *testing.T, row []float64) SourceContext {
	t.Helper()
	f, err := signal.NewFeedback(signal.FeedbackSpec{
		SchemaVersion: signal.CurrentSchemaVersion(),
		ExperienceID:  "exp-objective",
		ActionID:      "action-objective",
		ProducedAt:    signal.Timestamp{Value: 0, Unit: signal.TimeUnitModelStep},
		AvailableAt:   signal.Timestamp{Value: 0, Unit: signal.TimeUnitModelStep},
		Source:        "teacher",
		Score:         objectiveScore,
		ModelVersion:  signal.CurrentSchemaVersion(),
	})
	if err != nil {
		t.Fatalf("build feedback: %v", err)
	}
	return SourceContext{
		Activity:  row,
		Feedback:  []signal.Feedback{f},
		Resources: map[string]float64{"energy": objectiveEnergy},
	}
}

// replayReleases runs the fixture release sequence and returns the rate of the
// last step, which is the q the gradient is taken at.
func replayReleases(t *testing.T, c *Controller, rows [][]float64) float64 {
	t.Helper()
	var released float64
	for step, row := range rows {
		rates, err := c.Release(uint64(step), objectiveStep(t, row))
		if err != nil {
			t.Fatalf("Release(%d) error = %v", step, err)
		}
		released = rates[c.Channel]
	}
	return released
}

// meanActivity is the input the activity objective reads, written out here
// rather than taken from the controller: the window holds exactly the two
// released rows, so it is their mean over the window times the node count.
func meanActivity(rows [][]float64) float64 {
	var sum float64
	for _, row := range rows {
		for _, v := range row {
			sum += v
		}
	}
	return sum / float64(len(rows)*objectiveNodes)
}

// lossAt replays the same release sequence with one parameter moved by delta
// and returns the loss by hand. The proxy depends on the inputs alone, so it is
// the same number for the moved parameters, and no part of this comes from the
// gradient being checked.
func lossAt(t *testing.T, parameters []float64, index int, delta float64, rows [][]float64, proxy float64) float64 {
	t.Helper()
	moved := append([]float64(nil), parameters...)
	moved[index] += delta
	released := replayReleases(t, objectiveController(moved), rows)
	return (released - proxy) * (released - proxy)
}

// The analytic gradient is checked against a central difference of the loss
// itself, parameter by parameter, for both declared objectives.
func TestControllerGradientMatchesFiniteDifferences(t *testing.T) {
	parameters, rows := objectiveFixture()
	const eps = 1e-6
	for _, c := range []struct {
		name      string
		objective ControllerObjective
		proxy     float64
	}{
		{"reward proxy", ControllerObjective{Kind: ObjectiveRewardProxy}, objectiveScore},
		{"activity target", ControllerObjective{Kind: ObjectiveActivityTarget, Target: 0.7}, math.Max(0, 0.7-meanActivity(rows))},
	} {
		t.Run(c.name, func(t *testing.T) {
			controller := objectiveController(parameters)
			if controller.InputWidth() != objectiveWidth || controller.ParameterCount() != objectiveCount {
				t.Fatalf("fixture declares width %d and %d parameters, want %d and %d", controller.InputWidth(), controller.ParameterCount(), objectiveWidth, objectiveCount)
			}
			released := replayReleases(t, controller, rows)
			loss, grad, err := controller.Gradient(c.objective)
			if err != nil {
				t.Fatalf("Gradient() error = %v", err)
			}
			if len(grad) != objectiveCount {
				t.Fatalf("Gradient() returned %d values, want %d", len(grad), objectiveCount)
			}
			byHand := (released - c.proxy) * (released - c.proxy)
			if math.Abs(loss-byHand) > 1e-12 {
				t.Fatalf("Gradient() loss = %v, want %v for release %v and proxy %v", loss, byHand, released, c.proxy)
			}

			var worst float64
			for i := range parameters {
				plus := lossAt(t, parameters, i, eps, rows, c.proxy)
				minus := lossAt(t, parameters, i, -eps, rows, c.proxy)
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
		})
	}
}

// The gradient refuses what it cannot compute honestly, and a refused update
// leaves the parameters where they were.
func TestControllerGradientRejects(t *testing.T) {
	parameters, rows := objectiveFixture()

	// A controller that has not released has no inputs to take a gradient at.
	fresh := objectiveController(parameters)
	_, _, err := fresh.Gradient(ControllerObjective{Kind: ObjectiveRewardProxy})
	if err == nil || !strings.Contains(err.Error(), "no release") {
		t.Fatalf("Gradient() before any release error = %v, want one naming the missing release", err)
	}

	// The reward proxy is the feedback input, so a controller that does not
	// read feedback cannot be trained against it, released or not.
	deaf := &Controller{
		Inputs:  ControllerInputs{SummaryWindow: 2, Resources: []string{"energy"}},
		Nodes:   objectiveNodes,
		Hidden:  objectiveHidden,
		Channel: 0,
	}
	deaf.Parameters = make([]float64, deaf.ParameterCount())
	if _, err := deaf.Release(0, objectiveStep(t, rows[0])); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	reward := ControllerObjective{Kind: ObjectiveRewardProxy}
	if err := reward.Validate(deaf.Inputs); err == nil {
		t.Error("Validate() error = nil for the reward proxy without the feedback input")
	}
	if _, _, err := deaf.Gradient(reward); err == nil {
		t.Error("Gradient() error = nil for the reward proxy without the feedback input")
	}

	controller := objectiveController(parameters)
	replayReleases(t, controller, rows)
	for name, bad := range map[string]ControllerObjective{
		"no kind":             {},
		"unknown kind":        {Kind: "release_more"},
		"target not a number": {Kind: ObjectiveActivityTarget, Target: math.NaN()},
		"target infinite":     {Kind: ObjectiveActivityTarget, Target: math.Inf(1)},
	} {
		if err := bad.Validate(controller.Inputs); err == nil {
			t.Errorf("%s: Validate() error = nil", name)
		}
		if _, _, err := controller.Gradient(bad); err == nil {
			t.Errorf("%s: Gradient() error = nil", name)
		}
	}

	_, grad, err := controller.Gradient(reward)
	if err != nil {
		t.Fatalf("Gradient() error = %v", err)
	}
	before := append([]float64(nil), controller.Parameters...)
	for name, bad := range map[string]struct {
		grad []float64
		rate float64
	}{
		"gradient too short":   {grad[:len(grad)-1], 0.05},
		"gradient too long":    {append(append([]float64(nil), grad...), 0), 0.05},
		"rate is zero":         {grad, 0},
		"rate is negative":     {grad, -0.05},
		"rate is not a number": {grad, math.NaN()},
	} {
		if _, err := controller.Update(bad.grad, bad.rate); err == nil {
			t.Errorf("%s: Update() error = nil", name)
		}
	}
	if !slices.Equal(controller.Parameters, before) {
		t.Errorf("a refused Update() moved the parameters to %v, want %v", controller.Parameters, before)
	}
}

// Descending the declared objective lowers its loss step after step, which is
// what a gradient through the controller's own output is for.
func TestControllerUpdateReducesLoss(t *testing.T) {
	parameters, rows := objectiveFixture()
	controller := objectiveController(parameters)
	objective := ControllerObjective{Kind: ObjectiveRewardProxy}
	row := rows[0]

	// Fill the window with the row the loop repeats, so each pass reads the
	// same inputs and only the parameters move.
	for step := range 2 {
		if _, err := controller.Release(uint64(step), objectiveStep(t, row)); err != nil {
			t.Fatalf("warm-up Release(%d) error = %v", step, err)
		}
	}

	var first, previous float64
	for pass := range 20 {
		if _, err := controller.Release(uint64(2+pass), objectiveStep(t, row)); err != nil {
			t.Fatalf("pass %d: Release() error = %v", pass, err)
		}
		loss, grad, err := controller.Gradient(objective)
		if err != nil {
			t.Fatalf("pass %d: Gradient() error = %v", pass, err)
		}
		switch {
		case pass == 0:
			first = loss
		case loss > previous+1e-12:
			t.Fatalf("pass %d: loss rose from %v to %v", pass, previous, loss)
		}
		previous = loss

		updated, err := controller.Update(grad, 0.05)
		if err != nil {
			t.Fatalf("pass %d: Update() error = %v", pass, err)
		}
		if !slices.Equal(updated, controller.Parameters) {
			t.Fatalf("pass %d: Update() returned %v, want the new parameters %v", pass, updated, controller.Parameters)
		}
		if pass == 0 {
			updated[0] = math.NaN()
			if !finite(controller.Parameters[0]) {
				t.Fatal("Update() returned the controller's own parameter slice")
			}
		}
	}
	if previous >= first {
		t.Fatalf("loss %v after the updates, want below the first loss %v", previous, first)
	}
	t.Logf("loss %g after the first pass, %g after 19 updates", first, previous)
}
