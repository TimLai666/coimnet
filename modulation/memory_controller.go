package modulation

import (
	"fmt"
	"math"
	"strings"
)

// MemoryController is the capacity-matched control of the trainable
// controller: the same MLP shape over the same inputs, but its scalar output
// is a linear offset the caller adds to the model's readout instead of a
// release rate. It has no softplus, so the only difference between the two
// is where the output goes. Same parameter layout as Controller.
type MemoryController struct {
	Inputs     ControllerInputs `json:"inputs"`
	Nodes      int              `json:"nodes"`
	Hidden     int              `json:"hidden"`
	Parameters []float64        `json:"parameters"`

	// State, not declaration: unexported, and not serialized with the body.
	// summary owns the activity ring: it is a Controller kept in step with
	// this declaration and used for its observe and inputs alone, so both
	// controls summarise a window by the very same rule rather than by two
	// copies of it. None of its parameters or channels is ever read.
	summary      Controller
	lastX, lastH []float64
	lastY        float64
	offset       bool
}

// NewCapacityMatched builds a memory controller with the shape of c and its
// own copy of the parameters, so ParameterCount is equal by construction.
func NewCapacityMatched(c *Controller) (*MemoryController, error) {
	if c == nil {
		return nil, fmt.Errorf("modulation: a capacity-matched control needs a controller to match, got none")
	}
	inputs := c.Inputs
	inputs.Resources = append([]string(nil), c.Inputs.Resources...)
	return &MemoryController{Inputs: inputs, Nodes: c.Nodes, Hidden: c.Hidden, Parameters: append([]float64(nil), c.Parameters...)}, nil
}

// InputWidth is the two summary values, the feedback score when it is read, and one value per declared resource.
func (m *MemoryController) InputWidth() int {
	width := 2 + len(m.Inputs.Resources)
	if m.Inputs.UseFeedback {
		width++
	}
	return width
}

// ParameterCount is the length Parameters must have: W1, b1, w2 and b2.
func (m *MemoryController) ParameterCount() int { return m.Hidden*m.InputWidth() + 2*m.Hidden + 1 }

// MultAddsPerStep is what one Offset costs: the hidden layer and the readout.
func (m *MemoryController) MultAddsPerStep() int { return m.Hidden*m.InputWidth() + m.Hidden }

// Validate checks the declaration itself: the node count, hidden layer and
// window, resource names neither blank nor repeated, ParameterCount finite
// parameters. There is no channel, because an offset is not released.
func (m *MemoryController) Validate() error {
	switch {
	case m.Nodes < 1:
		return fmt.Errorf("modulation: memory controller summarises %d nodes, want at least 1", m.Nodes)
	case m.Hidden < 1 || m.Hidden > maxControllerHidden:
		return fmt.Errorf("modulation: memory controller declares %d hidden units, outside [1,%d]", m.Hidden, maxControllerHidden)
	case m.Inputs.SummaryWindow < 1 || m.Inputs.SummaryWindow > maxSummaryWindow:
		return fmt.Errorf("modulation: memory controller summarises %d rows, outside [1,%d]", m.Inputs.SummaryWindow, maxSummaryWindow)
	}
	named := make(map[string]bool, len(m.Inputs.Resources))
	for i, name := range m.Inputs.Resources {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("modulation: memory controller resource %d is blank", i)
		}
		if named[name] {
			return fmt.Errorf("modulation: memory controller names resource %q twice", name)
		}
		named[name] = true
	}
	if len(m.Parameters) != m.ParameterCount() {
		return fmt.Errorf("modulation: memory controller carries %d parameters, want %d for %d hidden units and %d inputs", len(m.Parameters), m.ParameterCount(), m.Hidden, m.InputWidth())
	}
	for i, v := range m.Parameters {
		if !finite(v) {
			return fmt.Errorf("modulation: memory controller parameter %d is not finite", i)
		}
	}
	return nil
}

// Offset pushes the activity row and returns y = w2·tanh(W1 x + b1) + b2.
func (m *MemoryController) Offset(step uint64, ctx SourceContext) (float64, error) {
	if err := m.Validate(); err != nil {
		return 0, err
	}
	if err := ctx.checkFeedback(step); err != nil {
		return 0, err
	}
	// The ring is the controller's own: the same window, the same rule for
	// the rows no activity has reached yet, and the same order in x.
	m.summary.Inputs, m.summary.Nodes = m.Inputs, m.Nodes
	if err := m.summary.observe(step, ctx.Activity); err != nil {
		return 0, err
	}
	x, err := m.summary.inputs(step, ctx)
	if err != nil {
		return 0, err
	}
	width, bias1 := m.InputWidth(), m.Hidden*m.InputWidth()
	h := make([]float64, m.Hidden)
	for j := range h {
		z := m.Parameters[bias1+j]
		for i, v := range x {
			z += m.Parameters[j*width+i] * v
		}
		h[j] = math.Tanh(z)
	}
	y := m.Parameters[bias1+2*m.Hidden]
	for j, v := range h {
		y += m.Parameters[bias1+m.Hidden+j] * v
	}
	// An offset has no checkedRelease behind it: it may be negative, but a
	// number that left the finite ones is still a failure, not a value.
	if !finite(y) {
		return 0, fmt.Errorf("modulation: memory controller offset is not finite at step %d", step)
	}
	m.lastX, m.lastH, m.lastY, m.offset = x, h, y, true
	return y, nil
}

// Gradient evaluates loss = (y − target)² at the inputs of the most recent
// Offset and returns dloss/dParameters in the parameter order: row-major W1,
// then b1, then w2, then b2. It reads the cached x, h and y of that offset and
// pushes nothing: no row moves here, and no state with it. The caller supplies
// the target it wants the offset to track, so the control trains on the same
// budget as the controller without a proxy of its own.
func (m *MemoryController) Gradient(target float64) (float64, []float64, error) {
	if !finite(target) {
		return 0, nil, fmt.Errorf("modulation: memory controller declares a target of %v, want a finite number", target)
	}
	if err := m.Validate(); err != nil {
		return 0, nil, err
	}
	if !m.offset {
		return 0, nil, fmt.Errorf("modulation: memory controller has no offset yet to take a gradient at")
	}
	width, hidden := m.InputWidth(), m.Hidden
	if len(m.lastX) != width || len(m.lastH) != hidden {
		return 0, nil, fmt.Errorf("modulation: the last offset read %d inputs and %d hidden units, but the memory controller now declares %d and %d", len(m.lastX), len(m.lastH), width, hidden)
	}

	// The forward pass, backwards. The readout is linear, so dy/dz2 is 1 and
	// the sum behind y never has to be recomputed: that missing sigmoid is
	// the whole difference from the controller's gradient.
	bias1, weights2 := hidden*width, hidden*width+hidden
	bias2 := weights2 + hidden
	distance := m.lastY - target
	loss := distance * distance
	dz2 := 2 * distance

	grad := make([]float64, len(m.Parameters))
	grad[bias2] = dz2
	for j, h := range m.lastH {
		grad[weights2+j] = dz2 * h
		// dh/dz1 is 1 − h², the derivative of the tanh that produced h.
		dz1 := dz2 * m.Parameters[weights2+j] * (1 - h*h)
		grad[bias1+j] = dz1
		for i, v := range m.lastX {
			grad[j*width+i] = dz1 * v
		}
	}
	if !finite(loss) {
		return 0, nil, fmt.Errorf("modulation: memory controller has a non-finite loss at offset %v and target %v", m.lastY, target)
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
// would leave the finite numbers is an error, and the control keeps the
// parameters it had.
func (m *MemoryController) Update(grad []float64, rate float64) ([]float64, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	if len(grad) != len(m.Parameters) {
		return nil, fmt.Errorf("modulation: update carries %d gradient values for %d parameters", len(grad), len(m.Parameters))
	}
	if !finite(rate) || rate <= 0 {
		return nil, fmt.Errorf("modulation: update declares a learning rate of %v, want a finite rate above zero", rate)
	}
	updated := make([]float64, len(m.Parameters))
	for i, v := range m.Parameters {
		if !finite(grad[i]) {
			return nil, fmt.Errorf("modulation: gradient of parameter %d is not finite", i)
		}
		updated[i] = v - rate*grad[i]
		if !finite(updated[i]) {
			return nil, fmt.Errorf("modulation: parameter %d becomes %v under this update", i, updated[i])
		}
	}
	copy(m.Parameters, updated)
	// The caller holds a slice of its own: writing into it moves no parameter.
	return updated, nil
}
