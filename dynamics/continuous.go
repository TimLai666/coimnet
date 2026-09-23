// Package dynamics implements synchronous neural state evolution and its
// explicitly defined reverse pass. Values use normalized model units.
package dynamics

import (
	"context"
	"fmt"
	"math"
)

// Config defines immutable connectivity. Delay zero reads the state at the
// beginning of the step; positive delays read earlier states. Before the first
// step, the initial voltage and its activated output are held constant.
type Config struct {
	Nodes      int     `json:"nodes"`
	Sources    []int   `json:"sources"`
	Targets    []int   `json:"targets"`
	Delays     []int   `json:"delays,omitempty"`
	DT         float64 `json:"dt"`
	Activation string  `json:"activation"`
	// Workers selects the forward execution mode: 0 or 1 is the single
	// threaded reference mode; more than 1 enables controlled parallelism
	// that partitions work by target node across a fixed number of workers,
	// summing each target's inputs in declared edge order so the result is
	// bit-identical to the single threaded pass.
	Workers int `json:"workers,omitempty"`
	// StateDimension sets the per-node state width: 0 or 1 keep the scalar
	// model (the pre-existing layout); C > 1 declares vector nodes whose
	// arrays are laid out by VectorLayout.
	StateDimension int `json:"state_dimension,omitempty"`
	// EdgeShape selects the per-edge weight layout, used only when
	// StateDimension > 1: "" or "scalar" broadcast one weight per edge to
	// all C components, "matrix" stores one C*C row-major matrix per edge.
	EdgeShape string `json:"edge_shape,omitempty"`
}

// Parameters separates learnable quantities from immutable anatomy. Tau is
// exp(LogTau), so finite, representable positive time constants are required.
type Parameters struct {
	Weights []float64 `json:"weights"`
	Bias    []float64 `json:"bias"`
	LogTau  []float64 `json:"log_tau"`
}

// Continuous is immutable and safe for concurrent independent trajectories.
type Continuous struct {
	config    Config
	partition *targetPartition
}

// Trace owns the parameter snapshot and neural history for one forward pass.
// It contains no optimizer, mutable anatomy, or external side effects.
type Trace struct {
	model                  *Continuous
	parameters             Parameters
	voltage, output, drive [][]float64
	lambda                 []float64
	alpha                  []float64
}

// Gradient contains summed parameter derivatives and per-step input gradients.
type Gradient struct {
	Weights, Bias, LogTau []float64
	Inputs                [][]float64
	Initial               []float64
}

// NewContinuous validates and copies the topology. Duplicate edges are separate
// additive connections in the declared order; source importers must resolve
// whether upstream duplicates are legitimate before constructing this model.
func NewContinuous(c Config) (*Continuous, error) {
	if c.Workers < 0 || c.Workers > 1024 {
		return nil, fmt.Errorf("workers must be in [0, 1024]")
	}
	if c.StateDimension < 0 || c.StateDimension > 64 {
		return nil, fmt.Errorf("state dimension must be in [0, 64]")
	}
	switch c.EdgeShape {
	case "", "scalar", "matrix":
	default:
		return nil, fmt.Errorf("unsupported edge shape %q", c.EdgeShape)
	}
	if c.StateDimension <= 1 && c.EdgeShape == "matrix" {
		return nil, fmt.Errorf("edge shape matrix requires a vector state dimension")
	}
	if c.StateDimension > 1 {
		// The scalar core never hosts vector nodes; NewVectorContinuous runs
		// them, exactly as the message below says.
		return nil, fmt.Errorf("use NewVectorContinuous for a vector state")
	}
	if c.Nodes <= 0 || !finite(c.DT) || c.DT <= 0 {
		return nil, fmt.Errorf("nodes and finite dt must be positive")
	}
	if c.Activation != "tanh" && c.Activation != "softplus" {
		return nil, fmt.Errorf("unsupported activation %q", c.Activation)
	}
	if len(c.Sources) != len(c.Targets) || (len(c.Delays) != 0 && len(c.Delays) != len(c.Sources)) {
		return nil, fmt.Errorf("edge arrays have different lengths")
	}
	c.Sources = append([]int(nil), c.Sources...)
	c.Targets = append([]int(nil), c.Targets...)
	if len(c.Delays) == 0 {
		c.Delays = make([]int, len(c.Sources))
	} else {
		c.Delays = append([]int(nil), c.Delays...)
	}
	for e, s := range c.Sources {
		if s < 0 || s >= c.Nodes || c.Targets[e] < 0 || c.Targets[e] >= c.Nodes || c.Delays[e] < 0 {
			return nil, fmt.Errorf("edge %d has invalid endpoint or delay", e)
		}
	}
	// The partition is built only for parallel runs. Workers 0/1 is the single
	// threaded reference mode with partition == nil and the original eager
	// loop; for Workers > 1 it groups edges by target so each target's inputs
	// are summed in declared order. Building it here, after every validation,
	// keeps a huge declared node count from allocating before an invalid
	// configuration is rejected.
	var partition *targetPartition
	if c.Workers > 1 {
		var err error
		partition, err = newTargetPartition(c.Nodes, c.Workers, c.Sources, c.Targets)
		if err != nil {
			return nil, err
		}
	}
	return &Continuous{config: c, partition: partition}, nil
}

// Config returns an independent topology/configuration copy.
// A nil or zero-value Continuous returns Config{}.
func (m *Continuous) Config() Config {
	if m == nil {
		return Config{}
	}
	c := m.config
	c.Sources = append([]int(nil), c.Sources...)
	c.Targets = append([]int(nil), c.Targets...)
	c.Delays = append([]int(nil), c.Delays...)
	return c
}

// reader returns the delayed input of edge e for one synaptic drive pass. It
// is used only by the parallel path, where extra allocations are allowed; the
// reference path uses the concrete accumulateSerial methods below so its
// allocations stay within the original eager loop's per-step budget.
type reader interface {
	at(e int) float64
}

// forwardReader reads delayed outputs of a forward Trace.
type forwardReader struct {
	output  [][]float64
	sources []int
	delays  []int
	t       int
}

func (r forwardReader) at(e int) float64 {
	past := 0
	if r.delays[e] < r.t {
		past = r.t - r.delays[e]
	} // avoid subtraction overflow
	return r.output[past][r.sources[e]]
}

// ringReader reads delayed outputs from an Advance ring slot.
type ringReader struct {
	ring     [][]float64
	sources  []int
	delays   []int
	head     int
	capacity int
	step     uint64
	first    uint64
}

func (r ringReader) at(e int) float64 {
	past := uint64(0)
	if uint64(r.delays[e]) < r.step {
		past = r.step - uint64(r.delays[e])
	}
	offset := int(past - r.first)
	return r.ring[(r.head+offset)%r.capacity][r.sources[e]]
}

// accumulateSerial adds weights[e]*r.at(e) to drive[targets[e]] in declared
// edge order, bit-identical to the original eager single threaded loop.
func (r forwardReader) accumulateSerial(drive, weights []float64, targets []int) {
	for e := range weights {
		drive[targets[e]] += weights[e] * r.at(e)
	}
}

// accumulateSerial adds weights[e]*r.at(e) to drive[targets[e]] in declared
// edge order, bit-identical to the original eager single threaded loop.
func (r ringReader) accumulateSerial(drive, weights []float64, targets []int) {
	for e := range weights {
		drive[targets[e]] += weights[e] * r.at(e)
	}
}

// synapticDriveParallel accumulates weights[e]*r.at(e) into drive[target] on
// the owning worker. Each target's addition order stays the declared edge
// order, so results are bit-identical to the single threaded pass.
func (m *Continuous) synapticDriveParallel(drive, weights []float64, r reader) error {
	return m.partition.Run(func(target int, edges []int) error {
		for _, e := range edges {
			drive[target] += weights[e] * r.at(e)
		}
		return nil
	})
}

// Forward evolves a nonempty sequence without changing its arguments. The
// returned trace owns its buffers. Cancellation or errors publish no state.
func (m *Continuous) Forward(ctx context.Context, p Parameters, initial []float64, inputs [][]float64) (*Trace, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("nil model")
	}
	n := m.config.Nodes
	if n <= 0 {
		return nil, fmt.Errorf("uninitialized model")
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("empty sequence")
	}
	if err := vector(initial, n, "initial voltage"); err != nil {
		return nil, err
	}
	if err := vector(p.Weights, len(m.config.Sources), "weights"); err != nil {
		return nil, err
	}
	if err := vector(p.Bias, n, "bias"); err != nil {
		return nil, err
	}
	if err := vector(p.LogTau, n, "log_tau"); err != nil {
		return nil, err
	}
	if len(inputs) > int(^uint(0)>>1)/n/3-1 {
		return nil, fmt.Errorf("sequence shape overflows")
	}
	lambda := make([]float64, n)
	alpha := make([]float64, n)
	for i, raw := range p.LogTau {
		tau := math.Exp(raw)
		if !finite(tau) || tau <= 0 {
			return nil, fmt.Errorf("tau[%d] is not representable and positive", i)
		}
		lambda[i] = math.Exp(-m.config.DT / tau)
		// Avoid cancellation in 1-exp(-dt/tau) for small positive dt/tau.
		alpha[i] = -math.Expm1(-m.config.DT / tau)
	}
	for t, in := range inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Validate inline without building an input[%d] name string on the
		// success path: the reference forward must keep the eager loop's
		// per-step allocation budget, and per-step formatting strings escape
		// to the heap under the race detector. Error text is unchanged.
		if len(in) != n {
			return nil, fmt.Errorf("input[%d] length %d, want %d", t, len(in), n)
		}
		for i, x := range in {
			if !finite(x) {
				return nil, fmt.Errorf("input[%d][%d] is non-finite", t, i)
			}
		}
	}
	tr := &Trace{model: m, parameters: cloneParameters(p), voltage: make([][]float64, len(inputs)+1), output: make([][]float64, len(inputs)+1), drive: make([][]float64, len(inputs)), lambda: lambda, alpha: alpha}
	tr.voltage[0] = append([]float64(nil), initial...)
	tr.output[0] = make([]float64, n)
	for i, v := range initial {
		tr.output[0][i] = m.activate(v)
	}
	for t, in := range inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		drive := append([]float64(nil), in...)
		if m.partition != nil {
			if err := m.synapticDriveParallel(drive, p.Weights, &forwardReader{output: tr.output, sources: m.config.Sources, delays: m.config.Delays, t: t}); err != nil {
				return nil, err
			}
		} else {
			r := forwardReader{output: tr.output, sources: m.config.Sources, delays: m.config.Delays, t: t}
			r.accumulateSerial(drive, p.Weights, m.config.Targets)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		vnext, ynext := make([]float64, n), make([]float64, n)
		for i := range drive {
			drive[i] += p.Bias[i]
			vnext[i] = lambda[i]*tr.voltage[t][i] + alpha[i]*drive[i]
			ynext[i] = m.activate(vnext[i])
			if !finite(drive[i]) || !finite(vnext[i]) || !finite(ynext[i]) {
				return nil, fmt.Errorf("non-finite state at step %d neuron %d", t, i)
			}
		}
		tr.drive[t] = drive
		tr.voltage[t+1] = vnext
		tr.output[t+1] = ynext
	}
	return tr, nil
}

// Outputs returns activated outputs after each input step, excluding prehistory.
// A nil or zero-value Trace returns a nil slice.
func (tr *Trace) Outputs() [][]float64 {
	if tr == nil || len(tr.output) <= 1 {
		return nil
	}
	return cloneRows(tr.output[1:])
}

// FinalVoltage returns the last neural voltage, not a complete delayed snapshot.
// A nil or zero-value Trace returns a nil slice.
func (tr *Trace) FinalVoltage() []float64 {
	if tr == nil || len(tr.voltage) == 0 {
		return nil
	}
	return append([]float64(nil), tr.voltage[len(tr.voltage)-1]...)
}

// Backward applies the chain rule to all outputs. Window zero propagates through
// the complete sequence. Positive window detaches at fixed window boundaries
// (0, window, 2*window, ...), including delayed edges, without resetting voltage.
// Trace and upstream are unchanged. This function does not update parameters.
func (m *Continuous) Backward(ctx context.Context, tr *Trace, upstream [][]float64, window int) (Gradient, error) {
	var empty Gradient
	if ctx == nil {
		return empty, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if m == nil || tr == nil || tr.model != m {
		return empty, fmt.Errorf("trace belongs to a different model")
	}
	steps, n := len(tr.drive), m.config.Nodes
	if window < 0 || len(upstream) != steps {
		return empty, fmt.Errorf("invalid backward window or sequence length")
	}
	gy := make([][]float64, steps+1)
	for t := range gy {
		gy[t] = make([]float64, n)
	}
	for t, row := range upstream {
		if err := vector(row, n, "upstream"); err != nil {
			return empty, err
		}
		copy(gy[t+1], row)
	}
	g := Gradient{Weights: make([]float64, len(tr.parameters.Weights)), Bias: make([]float64, n), LogTau: make([]float64, n), Inputs: make([][]float64, steps), Initial: make([]float64, n)}
	gv := make([]float64, n)
	for t := steps - 1; t >= 0; t-- {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		start := 0
		if window > 0 {
			start = t / window * window
		}
		prev := make([]float64, n)
		g.Inputs[t] = make([]float64, n)
		for i := 0; i < n; i++ {
			dv := gv[i] + gy[t+1][i]*m.derivative(tr.voltage[t+1][i])
			g.Inputs[t][i] = tr.alpha[i] * dv
			g.Bias[i] += g.Inputs[t][i]
			// d exp(-dt/exp(log_tau)) / d log_tau = lambda*dt/tau.
			dlambda := tr.lambda[i] * (m.config.DT / math.Exp(tr.parameters.LogTau[i]))
			// Underflowed lambda has zero derivative, even if dt/tau overflowed.
			if tr.lambda[i] == 0 {
				dlambda = 0
			}
			g.LogTau[i] += dv * (tr.voltage[t][i] - tr.drive[t][i]) * dlambda
			if t > start || start == 0 {
				prev[i] = tr.lambda[i] * dv
			}
		}
		for e, s := range m.config.Sources {
			if e%4096 == 0 {
				if err := ctx.Err(); err != nil {
					return empty, err
				}
			}
			past := 0
			if m.config.Delays[e] < t {
				past = t - m.config.Delays[e]
			}
			driveGrad := g.Inputs[t][m.config.Targets[e]]
			g.Weights[e] += driveGrad * tr.output[past][s]
			if past > start || start == 0 {
				gy[past][s] += driveGrad * tr.parameters.Weights[e]
			}
		}
		gv = prev
	}
	for i := range gv {
		g.Initial[i] = gv[i] + gy[0][i]*m.derivative(tr.voltage[0][i])
	}
	for _, v := range [][]float64{g.Weights, g.Bias, g.LogTau, g.Initial} {
		if err := vector(v, len(v), "gradient"); err != nil {
			return empty, err
		}
	}
	for _, v := range g.Inputs {
		if err := vector(v, len(v), "input gradient"); err != nil {
			return empty, err
		}
	}
	return g, nil
}

func (m *Continuous) activate(v float64) float64 {
	if m.config.Activation == "tanh" {
		return math.Tanh(v)
	}
	return math.Max(v, 0) + math.Log1p(math.Exp(-math.Abs(v)))
}
func (m *Continuous) derivative(v float64) float64 {
	if m.config.Activation == "tanh" {
		y := math.Tanh(v)
		return 1 - y*y
	}
	if v >= 0 {
		return 1 / (1 + math.Exp(-v))
	}
	e := math.Exp(v)
	return e / (1 + e)
}
func vector(v []float64, n int, name string) error {
	if len(v) != n {
		return fmt.Errorf("%s length %d, want %d", name, len(v), n)
	}
	for i, x := range v {
		if !finite(x) {
			return fmt.Errorf("%s[%d] is non-finite", name, i)
		}
	}
	return nil
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func cloneParameters(p Parameters) Parameters {
	return Parameters{append([]float64(nil), p.Weights...), append([]float64(nil), p.Bias...), append([]float64(nil), p.LogTau...)}
}
func cloneRows(rows [][]float64) [][]float64 {
	c := make([][]float64, len(rows))
	for i, row := range rows {
		c[i] = append([]float64(nil), row...)
	}
	return c
}
