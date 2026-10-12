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
// calls (rows before the first call are zeros). State exposes that runtime ring
// through ControllerState while declaration fields remain separate. Restore
// clears the release cache; Clone gives the candidate its own cache. Channels
// is Channel+1, like the other per-channel sources.
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
	if c == nil {
		return nil
	}
	inputs := c.Inputs
	inputs.Resources = cloneControllerStrings(c.Inputs.Resources)
	return &Controller{Inputs: inputs, Nodes: c.Nodes, Hidden: c.Hidden, Channel: c.Channel, Parameters: cloneControllerValues(c.Parameters)}
}

// Validate checks the declaration itself: the node count, hidden layer, channel
// and window, resource names neither blank nor repeated, ParameterCount finite parameters.
func (c *Controller) Validate() error {
	if c == nil {
		return fmt.Errorf("modulation: cannot validate a nil controller")
	}
	switch {
	case c.Nodes < 1:
		return fmt.Errorf("modulation: controller source summarises %d nodes, want at least 1", c.Nodes)
	case c.Hidden < 1 || c.Hidden > maxControllerHidden:
		return fmt.Errorf("modulation: controller source declares %d hidden units, outside [1,%d]", c.Hidden, maxControllerHidden)
	case c.Channel < 0:
		return fmt.Errorf("modulation: controller source declares channel %d", c.Channel)
	case c.Channel == int(^uint(0)>>1):
		return fmt.Errorf("modulation: controller source channel %d cannot form a channel count", c.Channel)
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
	if c == nil {
		return nil, fmt.Errorf("modulation: cannot release from a nil controller")
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.checkFeedback(step); err != nil {
		return nil, err
	}
	if err := c.validateWindow(); err != nil {
		return nil, err
	}

	// All mutable state lives on a private candidate until the complete summary,
	// MLP and checked release have succeeded. A malformed activity or a numeric
	// overflow therefore cannot consume a ring row or replace the last cache.
	candidate := c.Clone()
	if err := candidate.observe(step, ctx.Activity); err != nil {
		return nil, err
	}
	x, err := candidate.inputs(step, ctx)
	if err != nil {
		return nil, err
	}
	width, bias1 := c.InputWidth(), c.Hidden*c.InputWidth()
	h := make([]float64, c.Hidden)
	for j := range h {
		z := c.Parameters[bias1+j]
		for i, v := range x {
			product := c.Parameters[j*width+i] * v
			if !finite(product) {
				return nil, fmt.Errorf("modulation: controller hidden product %d,%d is not finite at step %d", j, i, step)
			}
			z += product
			if !finite(z) {
				return nil, fmt.Errorf("modulation: controller hidden unit %d is not finite at step %d", j, step)
			}
		}
		h[j] = math.Tanh(z)
		if !finite(h[j]) {
			return nil, fmt.Errorf("modulation: controller hidden unit %d is not finite at step %d", j, step)
		}
	}
	z := c.Parameters[bias1+2*c.Hidden]
	for j, v := range h {
		product := c.Parameters[bias1+c.Hidden+j] * v
		if !finite(product) {
			return nil, fmt.Errorf("modulation: controller readout product %d is not finite at step %d", j, step)
		}
		z += product
		if !finite(z) {
			return nil, fmt.Errorf("modulation: controller readout is not finite at step %d", step)
		}
	}
	q := softplus(z)
	if !finite(q) {
		return nil, fmt.Errorf("modulation: controller release rate is not finite at step %d", step)
	}
	rates := make([]float64, c.Channels())
	rates[c.Channel] = nonNegative(q)
	checked, err := checkedRelease(rates, c.Channels())
	if err != nil {
		return nil, err
	}

	// Commit the candidate ring and the cache together, after every finite and
	// channel check has passed.
	c.window = candidate.window
	c.filled = candidate.filled
	c.lastX = x
	c.lastH = h
	c.lastQ = q
	c.released = true
	return checked, nil
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
	if len(c.window) != c.Inputs.SummaryWindow || !controllerRowsShape(c.window, c.Nodes) {
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
	mean, variance, _, err := controllerSummary(c.window, c.filled, c.Nodes)
	if err != nil {
		return nil, fmt.Errorf("modulation: controller activity summary failed at step %d: %w", step, err)
	}
	x := append(make([]float64, 0, c.InputWidth()), mean, variance)
	if c.Inputs.UseFeedback {
		var score float64
		for i, f := range ctx.Feedback {
			score += f.Score()
			if !finite(score) {
				return nil, fmt.Errorf("modulation: feedback score summary is not finite at step %d after item %d", step, i)
			}
		}
		if len(ctx.Feedback) > 0 {
			score /= float64(len(ctx.Feedback))
			if !finite(score) {
				return nil, fmt.Errorf("modulation: feedback score summary is not finite at step %d", step)
			}
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
	for i, value := range x {
		if !finite(value) {
			return nil, fmt.Errorf("modulation: controller input %d is not finite at step %d", i, step)
		}
	}
	return x, nil
}

func controllerRowsShape(window [][]float64, nodes int) bool {
	if window == nil {
		return false
	}
	for _, row := range window {
		if len(row) != nodes {
			return false
		}
	}
	return true
}

// controllerSummary computes the population mean and variance over the
// complete declared window denominator. Rows before the first observation are
// already zero and are omitted from the summation. The raw variance is returned
// separately so BackwardActivity can apply the clamp's zero derivative.
func controllerSummary(window [][]float64, filled, nodes int) (float64, float64, float64, error) {
	if len(window) == 0 || nodes < 1 || filled < 0 || filled > len(window) {
		return 0, 0, 0, fmt.Errorf("modulation: controller activity summary has invalid window shape")
	}
	total := float64(len(window)) * float64(nodes)
	if !finite(total) || total <= 0 {
		return 0, 0, 0, fmt.Errorf("modulation: controller activity summary size is not finite")
	}
	var sum, squares float64
	start := len(window) - filled
	for i := start; i < len(window); i++ {
		row := window[i]
		if len(row) != nodes {
			return 0, 0, 0, fmt.Errorf("modulation: controller activity summary row %d has %d values, want %d", i, len(row), nodes)
		}
		for j, value := range row {
			if !finite(value) {
				return 0, 0, 0, fmt.Errorf("modulation: controller activity summary row %d node %d is not finite", i, j)
			}
			square := value * value
			if !finite(square) {
				return 0, 0, 0, fmt.Errorf("modulation: controller activity summary square at row %d node %d is not finite", i, j)
			}
			sum += value
			if !finite(sum) {
				return 0, 0, 0, fmt.Errorf("modulation: controller activity summary sum is not finite")
			}
			squares += square
			if !finite(squares) {
				return 0, 0, 0, fmt.Errorf("modulation: controller activity summary squares are not finite")
			}
		}
	}
	mean := sum / total
	if !finite(mean) {
		return 0, 0, 0, fmt.Errorf("modulation: controller activity mean is not finite")
	}
	meanSquare := mean * mean
	if !finite(meanSquare) {
		return 0, 0, 0, fmt.Errorf("modulation: controller activity mean square is not finite")
	}
	rawVariance := squares/total - meanSquare
	if !finite(rawVariance) {
		return 0, 0, 0, fmt.Errorf("modulation: controller activity variance is not finite")
	}
	variance := rawVariance
	if variance <= 0 {
		variance = 0
	}
	return mean, variance, rawVariance, nil
}

// softplus is log(1 + e^z), the readout that makes a release rate non-negative
// without a clamp. Above 30 it and z differ by under 1e-13, so z avoids overflow.
func softplus(z float64) float64 {
	if z > 30 {
		return z
	}
	return math.Log1p(math.Exp(z))
}
