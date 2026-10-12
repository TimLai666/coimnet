package modulation

import "fmt"

// ControllerState stores the oldest-to-newest activity window at a completed
// release boundary. Unfilled rows are zeros. It carries no declaration or
// training target; a zero state is the legacy fresh-window boundary.
type ControllerState struct {
	Window [][]float64 `json:"window"`
	Filled int         `json:"filled"`
}

// State returns an owned window snapshot, without the last gradient cache.
func (c *Controller) State() ControllerState {
	if c == nil {
		return ControllerState{}
	}
	return ControllerState{Window: cloneControllerRows(c.window), Filled: c.filled}
}

// RestoreState validates the whole window against the declaration before
// replacing it. It clears the previous release cache; Gradient requires a new
// successful Release. Errors leave every existing state and cache unchanged.
func (c *Controller) RestoreState(state ControllerState) error {
	if c == nil {
		return fmt.Errorf("modulation: cannot restore state into a nil controller")
	}
	if err := c.Validate(); err != nil {
		return err
	}
	if err := validateControllerRows(state.Window, state.Filled, c.Inputs.SummaryWindow, c.Nodes); err != nil {
		return err
	}

	// Validation above is deliberately complete before any receiver field is
	// touched. The clone also keeps the caller's decoded rows independent from
	// the controller after a successful restore.
	window := cloneControllerRows(state.Window)
	c.window = window
	c.filled = state.Filled
	c.lastX = nil
	c.lastH = nil
	c.lastQ = 0
	c.released = false
	return nil
}

// Clone copies declaration, window and last-release cache for a private
// candidate. A nil receiver returns nil. No array is shared with its caller.
func (c *Controller) Clone() *Controller {
	if c == nil {
		return nil
	}
	clone := c.declaration()
	clone.window = cloneControllerRows(c.window)
	clone.filled = c.filled
	clone.lastX = cloneControllerValues(c.lastX)
	clone.lastH = cloneControllerValues(c.lastH)
	clone.lastQ = c.lastQ
	clone.released = c.released
	return clone
}

// BackwardActivity returns d(upstream*q)/d(window activity), oldest-to-newest,
// for the most recent successful Release with fixed controller parameters.
// It differentiates both mean and population variance and the softplus/tanh
// network. Unfilled rows are constant zeros and receive zero gradients.
// Invalid declaration/cache/upstream or overflow returns nil and an error.
func (c *Controller) BackwardActivity(upstream float64) ([][]float64, error) {
	if c == nil {
		return nil, fmt.Errorf("modulation: cannot backpropagate activity through a nil controller")
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if !finite(upstream) {
		return nil, fmt.Errorf("modulation: activity upstream %v is not finite", upstream)
	}
	if !c.released {
		return nil, fmt.Errorf("modulation: controller has no release yet to backpropagate activity through")
	}
	if err := c.validateWindow(); err != nil {
		return nil, err
	}
	width, hidden := c.InputWidth(), c.Hidden
	if len(c.lastX) != width || len(c.lastH) != hidden {
		return nil, fmt.Errorf("modulation: the last release read %d inputs and %d hidden units, but the controller now declares %d and %d", len(c.lastX), len(c.lastH), width, hidden)
	}
	for i, value := range c.lastX {
		if !finite(value) {
			return nil, fmt.Errorf("modulation: cached release input %d is not finite", i)
		}
	}
	for i, value := range c.lastH {
		if !finite(value) {
			return nil, fmt.Errorf("modulation: cached hidden unit %d is not finite", i)
		}
	}
	if !finite(c.lastQ) || c.lastQ < 0 {
		return nil, fmt.Errorf("modulation: cached release rate %v is invalid", c.lastQ)
	}

	mean, variance, rawVariance, err := controllerSummary(c.window, c.filled, c.Nodes)
	if err != nil {
		return nil, err
	}
	if c.lastX[0] != mean || c.lastX[1] != variance {
		return nil, fmt.Errorf("modulation: cached release summary no longer matches the activity window")
	}
	total := float64(c.Inputs.SummaryWindow) * float64(c.Nodes)
	if !finite(total) || total <= 0 {
		return nil, fmt.Errorf("modulation: controller activity summary size is not finite")
	}
	varianceDerivative := 1.0
	if rawVariance <= 0 {
		varianceDerivative = 0
	}

	bias1 := hidden * width
	weights2 := bias1 + hidden
	bias2 := weights2 + hidden
	z2 := c.Parameters[bias2]
	for j, value := range c.lastH {
		product := c.Parameters[weights2+j] * value
		if !finite(product) {
			return nil, fmt.Errorf("modulation: controller readout product %d is not finite", j)
		}
		z2 += product
		if !finite(z2) {
			return nil, fmt.Errorf("modulation: controller readout is not finite")
		}
	}
	dSoftplus := 1.0
	if z2 <= 30 {
		dSoftplus = sigmoid(z2)
	}
	if !finite(dSoftplus) {
		return nil, fmt.Errorf("modulation: controller softplus derivative is not finite")
	}

	var dMean, dVariance float64
	for j, value := range c.lastH {
		squared := value * value
		if !finite(squared) {
			return nil, fmt.Errorf("modulation: hidden unit %d square is not finite", j)
		}
		tanhDerivative := 1 - squared
		if !finite(tanhDerivative) {
			return nil, fmt.Errorf("modulation: hidden unit %d tanh derivative is not finite", j)
		}
		dz1 := dSoftplus * c.Parameters[weights2+j]
		if !finite(dz1) {
			return nil, fmt.Errorf("modulation: hidden unit %d readout gradient is not finite", j)
		}
		dz1 *= tanhDerivative
		if !finite(dz1) {
			return nil, fmt.Errorf("modulation: hidden unit %d gradient is not finite", j)
		}
		meanTerm := dz1 * c.Parameters[j*width]
		if !finite(meanTerm) {
			return nil, fmt.Errorf("modulation: mean gradient contribution %d is not finite", j)
		}
		dMean += meanTerm
		if !finite(dMean) {
			return nil, fmt.Errorf("modulation: activity mean gradient is not finite")
		}
		varianceTerm := varianceDerivative * dz1
		if !finite(varianceTerm) {
			return nil, fmt.Errorf("modulation: variance gradient contribution %d is not finite", j)
		}
		varianceTerm *= c.Parameters[j*width+1]
		if !finite(varianceTerm) {
			return nil, fmt.Errorf("modulation: variance gradient contribution %d is not finite", j)
		}
		dVariance += varianceTerm
		if !finite(dVariance) {
			return nil, fmt.Errorf("modulation: activity variance gradient is not finite")
		}
	}

	meanTerm := dMean / total
	if !finite(meanTerm) {
		return nil, fmt.Errorf("modulation: activity mean gradient per value is not finite")
	}
	varianceScale := dVariance * 2
	if !finite(varianceScale) {
		return nil, fmt.Errorf("modulation: activity variance gradient scale is not finite")
	}
	gradient := make([][]float64, c.Inputs.SummaryWindow)
	for i := range gradient {
		gradient[i] = make([]float64, c.Nodes)
	}
	start := len(c.window) - c.filled
	for i := start; i < len(c.window); i++ {
		for j, value := range c.window[i] {
			delta := value - mean
			if !finite(delta) {
				return nil, fmt.Errorf("modulation: activity delta at row %d node %d is not finite", i, j)
			}
			varianceTerm := varianceScale * delta
			if !finite(varianceTerm) {
				return nil, fmt.Errorf("modulation: activity variance gradient at row %d node %d is not finite", i, j)
			}
			varianceTerm /= total
			if !finite(varianceTerm) {
				return nil, fmt.Errorf("modulation: activity variance gradient at row %d node %d is not finite", i, j)
			}
			valueGradient := meanTerm + varianceTerm
			if !finite(valueGradient) {
				return nil, fmt.Errorf("modulation: activity gradient at row %d node %d is not finite", i, j)
			}
			valueGradient *= upstream
			if !finite(valueGradient) {
				return nil, fmt.Errorf("modulation: activity gradient at row %d node %d is not finite", i, j)
			}
			gradient[i][j] = valueGradient
		}
	}
	return gradient, nil
}

// cloneControllerValues and cloneControllerRows preserve nil versus allocated
// empty slices while ensuring every returned buffer is owned by its caller.
func cloneControllerValues(values []float64) []float64 {
	if values == nil {
		return nil
	}
	copyOf := make([]float64, len(values))
	copy(copyOf, values)
	return copyOf
}

func cloneControllerStrings(values []string) []string {
	if values == nil {
		return nil
	}
	copyOf := make([]string, len(values))
	copy(copyOf, values)
	return copyOf
}

func cloneControllerRows(rows [][]float64) [][]float64 {
	if rows == nil {
		return nil
	}
	copyOf := make([][]float64, len(rows))
	for i, row := range rows {
		copyOf[i] = cloneControllerValues(row)
	}
	return copyOf
}

// validateControllerRows validates a serialized or internal activity ring.
// A nil window is the legacy fresh boundary and is valid only with no filled
// rows. For an allocated ring, the oldest unfilled rows must remain exact
// numeric zeros so the boundary has one unambiguous representation.
func validateControllerRows(window [][]float64, filled, summaryWindow, nodes int) error {
	if window == nil {
		if filled != 0 {
			return fmt.Errorf("modulation: controller state has a nil window with filled=%d", filled)
		}
		return nil
	}
	if summaryWindow < 1 || nodes < 1 {
		return fmt.Errorf("modulation: controller state cannot be checked against an invalid declaration")
	}
	if len(window) != summaryWindow {
		return fmt.Errorf("modulation: controller state has %d rows, want %d", len(window), summaryWindow)
	}
	if filled < 0 || filled > summaryWindow {
		return fmt.Errorf("modulation: controller state has filled=%d, want a value in [0,%d]", filled, summaryWindow)
	}
	for i, row := range window {
		if len(row) != nodes {
			return fmt.Errorf("modulation: controller state row %d has %d values, want %d", i, len(row), nodes)
		}
		for j, value := range row {
			if !finite(value) {
				return fmt.Errorf("modulation: controller state row %d node %d is not finite", i, j)
			}
			if i < summaryWindow-filled && value != 0 {
				return fmt.Errorf("modulation: controller state unfilled row %d node %d is %v, want zero", i, j, value)
			}
		}
	}
	return nil
}

func (c *Controller) validateWindow() error {
	if c == nil {
		return fmt.Errorf("modulation: cannot validate the window of a nil controller")
	}
	return validateControllerRows(c.window, c.filled, c.Inputs.SummaryWindow, c.Nodes)
}
