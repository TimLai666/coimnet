package modulation

import (
	"fmt"
	"math"
)

// The declared control objectives. Each one names a proxy the controller can
// compute from the inputs it already read, so training never needs a target:
// there is no target to hand in, and no field to hand one in through.
const (
	// ObjectiveRewardProxy trains the release towards the mean score of the
	// feedback that has arrived, which is the input UseFeedback adds.
	ObjectiveRewardProxy = "reward_proxy"
	// ObjectiveActivityTarget trains the release towards how far the mean
	// activity falls short of Target, and towards zero once it is reached.
	ObjectiveActivityTarget = "activity_target"
)

// The inputs an objective reads, at the positions Release builds them in: the
// activity mean first, then the variance, then the feedback score when the
// controller declares it. The resources follow and no objective reads them.
const (
	activityMeanInput = 0
	feedbackInput     = 2
)

// ControllerObjective declares the one scalar the controller's parameters are
// trained against: the squared distance between its release q and a proxy
// computed from the same inputs Release read. The gradient goes through the
// MLP only; it never crosses the concentration, the receptors or the core.
type ControllerObjective struct {
	Kind   string  `json:"kind"`
	Target float64 `json:"target,omitempty"`
}

// Validate checks the objective against the inputs a controller declares: an
// objective is only usable if the controller already reads what its proxy is
// made of, which is why this takes the declaration rather than a release.
func (o ControllerObjective) Validate(inputs ControllerInputs) error {
	switch o.Kind {
	case ObjectiveRewardProxy:
		if !inputs.UseFeedback {
			return fmt.Errorf("modulation: objective %q is the mean feedback score, which this controller does not read", o.Kind)
		}
	case ObjectiveActivityTarget:
		if !finite(o.Target) {
			return fmt.Errorf("modulation: objective %q declares a non-finite target", o.Kind)
		}
	default:
		return fmt.Errorf("modulation: unknown controller objective %q, want %q or %q", o.Kind, ObjectiveRewardProxy, ObjectiveActivityTarget)
	}
	return nil
}

// proxy is the value the release is trained towards, read from the inputs of
// one release. It depends on those inputs alone, never on the parameters,
// which is why its derivative with respect to every parameter is zero.
func (o ControllerObjective) proxy(x []float64) (float64, error) {
	read := func(i int) (float64, error) {
		if i >= len(x) {
			return 0, fmt.Errorf("modulation: objective %q reads input %d of the %d this release carries", o.Kind, i, len(x))
		}
		return x[i], nil
	}
	switch o.Kind {
	case ObjectiveRewardProxy:
		return read(feedbackInput)
	case ObjectiveActivityTarget:
		mean, err := read(activityMeanInput)
		if err != nil {
			return 0, err
		}
		return nonNegative(o.Target - mean), nil
	}
	return 0, fmt.Errorf("modulation: unknown controller objective %q", o.Kind)
}

// Gradient evaluates loss = (q − proxy)² at the inputs of the most recent
// Release and returns dloss/dParameters in the parameter order: row-major W1,
// then b1, then w2, then b2. It reads the cached x, h and q of that release and
// pushes nothing: no release happens here, and no state moves.
func (c *Controller) Gradient(o ControllerObjective) (float64, []float64, error) {
	if err := o.Validate(c.Inputs); err != nil {
		return 0, nil, err
	}
	if err := c.Validate(); err != nil {
		return 0, nil, err
	}
	if !c.released {
		return 0, nil, fmt.Errorf("modulation: controller has no release yet to take a gradient at")
	}
	width, hidden := c.InputWidth(), c.Hidden
	if len(c.lastX) != width || len(c.lastH) != hidden {
		return 0, nil, fmt.Errorf("modulation: the last release read %d inputs and %d hidden units, but the controller now declares %d and %d", len(c.lastX), len(c.lastH), width, hidden)
	}
	proxy, err := o.proxy(c.lastX)
	if err != nil {
		return 0, nil, err
	}

	// The forward pass, backwards. z2 is recomputed from the cached hidden
	// layer because the release keeps the rate it produced, not the sum
	// behind it; sigmoid(z2) is the derivative of the softplus readout.
	bias1, weights2 := hidden*width, hidden*width+hidden
	bias2 := weights2 + hidden
	z2 := c.Parameters[bias2]
	for j, h := range c.lastH {
		z2 += c.Parameters[weights2+j] * h
	}
	distance := c.lastQ - proxy
	loss := distance * distance
	dz2 := 2 * distance * sigmoid(z2)

	grad := make([]float64, len(c.Parameters))
	grad[bias2] = dz2
	for j, h := range c.lastH {
		grad[weights2+j] = dz2 * h
		// dh/dz1 is 1 − h², the derivative of the tanh that produced h.
		dz1 := dz2 * c.Parameters[weights2+j] * (1 - h*h)
		grad[bias1+j] = dz1
		for i, v := range c.lastX {
			grad[j*width+i] = dz1 * v
		}
	}
	if !finite(loss) {
		return 0, nil, fmt.Errorf("modulation: controller objective %q has a non-finite loss at release rate %v", o.Kind, c.lastQ)
	}
	for i, v := range grad {
		if !finite(v) {
			return 0, nil, fmt.Errorf("modulation: gradient of parameter %d is not finite", i)
		}
	}
	return loss, grad, nil
}

// Update applies parameters −= rate * grad and returns a copy of the new
// parameters. The whole step lands or none of it does: a gradient of another
// length, a rate that is not a finite number above zero, or a parameter that
// would leave the finite numbers is an error, and the controller keeps the
// parameters it had.
func (c *Controller) Update(grad []float64, rate float64) ([]float64, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if len(grad) != len(c.Parameters) {
		return nil, fmt.Errorf("modulation: update carries %d gradient values for %d parameters", len(grad), len(c.Parameters))
	}
	if !finite(rate) || rate <= 0 {
		return nil, fmt.Errorf("modulation: update declares a learning rate of %v, want a finite rate above zero", rate)
	}
	updated := make([]float64, len(c.Parameters))
	for i, v := range c.Parameters {
		if !finite(grad[i]) {
			return nil, fmt.Errorf("modulation: gradient of parameter %d is not finite", i)
		}
		updated[i] = v - rate*grad[i]
		if !finite(updated[i]) {
			return nil, fmt.Errorf("modulation: parameter %d becomes %v under this update", i, updated[i])
		}
	}
	copy(c.Parameters, updated)
	// The caller holds a slice of its own: writing into it moves no parameter.
	return updated, nil
}

// sigmoid is the derivative of softplus, written so neither branch exponentiates
// a large positive number: e^z overflows above 710, e^-z below -710.
func sigmoid(z float64) float64 {
	if z >= 0 {
		return 1 / (1 + math.Exp(-z))
	}
	e := math.Exp(z)
	return e / (1 + e)
}
