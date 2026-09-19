package modulation

import (
	"fmt"
	"math"
	"strings"
)

// SourceController is the kind name of the trainable controller source. The
// shape is bounded at both ends so a controller stays a small policy: a longer
// window would be a recording, and a wider hidden layer a second model.
const (
	SourceController    = "controller"
	maxSummaryWindow    = 64
	maxControllerHidden = 256
)

// ControllerInputs declares what the controller may read. There is no target
// field and never will be: the controller sees a summary of activity, the
// feedback that has already arrived and the declared resources, nothing else.
type ControllerInputs struct {
	SummaryWindow int      `json:"summary_window"`      // 1..64 rows of activity summarised as one mean and one variance
	UseFeedback   bool     `json:"use_feedback"`        // adds the mean score of the available feedback (0 when none)
	Resources     []string `json:"resources,omitempty"` // declared resource names in this fixed order (missing → 0)
}

// Controller is a small MLP source: x (InputWidth) → h = tanh(W1 x + b1)
// (Hidden) → q = softplus(w2·h + b2), released on Channel. Parameters is
// row-major W1 (Hidden×InputWidth), then b1 (Hidden), then w2 (Hidden), then
// b2 (1). The activity window is a ring the source itself keeps across Release
// calls (rows before the first call are zeros); it is not part of any snapshot
// in this stage, so a restored controller starts its window from zeros again.
// Channels is Channel+1, like the other per-channel sources.
type Controller struct {
	Inputs     ControllerInputs `json:"inputs"`
	Nodes      int              `json:"nodes"`
	Hidden     int              `json:"hidden"`
	Channel    int              `json:"channel"`
	Parameters []float64        `json:"parameters"`

	// State, not declaration: unexported, and not serialized with the body.
	window       [][]float64
	filled       int
	lastX, lastH []float64
	lastQ        float64
	released     bool
}

// InputWidth is the two summary values, the feedback score when it is read, and one value per declared resource.
func (c *Controller) InputWidth() int {
	width := 2 + len(c.Inputs.Resources)
	if c.Inputs.UseFeedback {
		width++
	}
	return width
}

// ParameterCount is the length Parameters must have: W1, b1, w2 and b2.
func (c *Controller) ParameterCount() int { return c.Hidden*c.InputWidth() + 2*c.Hidden + 1 }

// MultAddsPerStep is what one Release costs: the hidden layer and the readout.
func (c *Controller) MultAddsPerStep() int { return c.Hidden*c.InputWidth() + c.Hidden }

func (c *Controller) Channels() int { return c.Channel + 1 }

// declaration copies the declared fields alone, leaving the window and the
// cached forward pass behind: a built or cloned controller owns its own ring.
func (c *Controller) declaration() *Controller {
	inputs := c.Inputs
	inputs.Resources = append([]string(nil), c.Inputs.Resources...)
	return &Controller{Inputs: inputs, Nodes: c.Nodes, Hidden: c.Hidden, Channel: c.Channel, Parameters: append([]float64(nil), c.Parameters...)}
}

// Validate checks the declaration itself: the node count, hidden layer, channel
// and window, resource names neither blank nor repeated, ParameterCount finite parameters.
func (c *Controller) Validate() error {
	switch {
	case c.Nodes < 1:
		return fmt.Errorf("modulation: controller source summarises %d nodes, want at least 1", c.Nodes)
	case c.Hidden < 1 || c.Hidden > maxControllerHidden:
		return fmt.Errorf("modulation: controller source declares %d hidden units, outside [1,%d]", c.Hidden, maxControllerHidden)
	case c.Channel < 0:
		return fmt.Errorf("modulation: controller source declares channel %d", c.Channel)
	case c.Inputs.SummaryWindow < 1 || c.Inputs.SummaryWindow > maxSummaryWindow:
		return fmt.Errorf("modulation: controller source summarises %d rows, outside [1,%d]", c.Inputs.SummaryWindow, maxSummaryWindow)
	}
	named := make(map[string]bool, len(c.Inputs.Resources))
	for i, name := range c.Inputs.Resources {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("modulation: controller source resource %d is blank", i)
		}
		if named[name] {
			return fmt.Errorf("modulation: controller source names resource %q twice", name)
		}
		named[name] = true
	}
	if len(c.Parameters) != c.ParameterCount() {
		return fmt.Errorf("modulation: controller source carries %d parameters, want %d for %d hidden units and %d inputs", len(c.Parameters), c.ParameterCount(), c.Hidden, c.InputWidth())
	}
	for i, v := range c.Parameters {
		if !finite(v) {
			return fmt.Errorf("modulation: controller source parameter %d is not finite", i)
		}
	}
	return nil
}

// Release summarises this step's activity with the window behind it, reads the
// feedback that has arrived and the declared resources, and releases one rate.
func (c *Controller) Release(step uint64, ctx SourceContext) ([]float64, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.checkFeedback(step); err != nil {
		return nil, err
	}
	if err := c.observe(step, ctx.Activity); err != nil {
		return nil, err
	}
	x, err := c.inputs(step, ctx)
	if err != nil {
		return nil, err
	}
	width, bias1 := c.InputWidth(), c.Hidden*c.InputWidth()
	h := make([]float64, c.Hidden)
	for j := range h {
		z := c.Parameters[bias1+j]
		for i, v := range x {
			z += c.Parameters[j*width+i] * v
		}
		h[j] = math.Tanh(z)
	}
	z := c.Parameters[bias1+2*c.Hidden]
	for j, v := range h {
		z += c.Parameters[bias1+c.Hidden+j] * v
	}
	q := softplus(z)
	// Kept so the gradient of the next ticket reads the step that produced this rate.
	c.lastX, c.lastH, c.lastQ, c.released = x, h, q, true
	rates := make([]float64, c.Channels())
	rates[c.Channel] = nonNegative(q)
	return checkedRelease(rates, c.Channels())
}

// observe writes this step's activity into the ring as its newest row and drops
// the oldest. A caller with no activity is the declared node count as zeros: a
// step the controller did not see is no activity, not a gap. Another width is an error.
func (c *Controller) observe(step uint64, activity []float64) error {
	if activity != nil {
		if len(activity) != c.Nodes {
			return fmt.Errorf("modulation: controller source summarises %d nodes but step %d has %d activity values", c.Nodes, step, len(activity))
		}
		for i, v := range activity {
			if !finite(v) {
				return fmt.Errorf("modulation: activity of node %d is not finite at step %d", i, step)
			}
		}
	}
	if len(c.window) != c.Inputs.SummaryWindow || len(c.window[0]) != c.Nodes {
		c.window = make([][]float64, c.Inputs.SummaryWindow)
		for i := range c.window {
			c.window[i] = make([]float64, c.Nodes)
		}
		c.filled = 0
	}
	// The oldest row becomes the newest: a step allocates nothing, and the caller's slice is copied rather than held.
	oldest := c.window[0]
	copy(c.window, c.window[1:])
	c.window[len(c.window)-1] = oldest
	clear(oldest)
	copy(oldest, activity)
	c.filled = min(c.filled+1, len(c.window))
	return nil
}

// inputs builds x in the declared order: one mean and one population variance
// over the whole ring, divided by the full window times the node count so the
// rows no activity has reached yet count as the zeros they hold; the mean score
// of the arrived feedback; one value per declared resource, zero when absent.
func (c *Controller) inputs(step uint64, ctx SourceContext) ([]float64, error) {
	var sum, squares float64
	for _, row := range c.window[len(c.window)-c.filled:] {
		for _, v := range row {
			sum, squares = sum+v, squares+v*v
		}
	}
	total := float64(len(c.window) * c.Nodes)
	mean := sum / total
	// Rounding can push a variance of zero just below it; it is never negative.
	x := append(make([]float64, 0, c.InputWidth()), mean, nonNegative(squares/total-mean*mean))
	if c.Inputs.UseFeedback {
		var score float64
		for _, f := range ctx.Feedback {
			score += f.Score()
		}
		if len(ctx.Feedback) > 0 {
			score /= float64(len(ctx.Feedback))
		}
		x = append(x, score)
	}
	for _, name := range c.Inputs.Resources {
		amount, declared := ctx.Resources[name]
		if declared && !finite(amount) {
			return nil, fmt.Errorf("modulation: resource %q is not finite at step %d", name, step)
		}
		x = append(x, amount)
	}
	return x, nil
}

// softplus is log(1 + e^z), the readout that makes a release rate non-negative
// without a clamp. Above 30 it and z differ by under 1e-13, so z avoids overflow.
func softplus(z float64) float64 {
	if z > 30 {
		return z
	}
	return math.Log1p(math.Exp(z))
}
