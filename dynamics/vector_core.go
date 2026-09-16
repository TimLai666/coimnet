package dynamics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
)

// VectorStateVersion identifies the vector continuous state and solver.
const VectorStateVersion = "coimnet-vector-state/v1"

// activation applies the configured output nonlinearity to one component.
type activation func(float64) float64

// VectorParameters separates learnable quantities from immutable anatomy for
// a vector continuous model. Weights has layout.WeightValues() entries,
// Bias holds N*C node-major values and LogTau holds one value per node.
type VectorParameters struct {
	Weights []float64 `json:"weights"`
	Bias    []float64 `json:"bias"`
	LogTau  []float64 `json:"log_tau"`
}

// VectorTrace owns the parameter snapshot, per-step drive and neural history
// of one forward pass, mirroring Trace. Voltage and output are
// [step][node*C+component] rows; drive holds each step's external plus
// synaptic input after bias is added; lambda and alpha derive from the
// snapshot's log_tau. It contains no optimizer, mutable anatomy or external
// side effects.
type VectorTrace struct {
	model      *VectorContinuous
	parameters VectorParameters
	voltage    [][]float64
	output     [][]float64
	drive      [][]float64
	lambda     []float64
	alpha      []float64
}

// VectorContinuous is the continuous core with a C-dimensional state per
// node: v_i, b_i in R^C, tau_i scalar, and each edge either a scalar weight
// broadcast over the C components or a C×C matrix applied to the source
// output vector. With C = 1 and scalar edges it computes exactly what
// Continuous computes.
type VectorContinuous struct {
	config Config
	layout VectorLayout
	act    activation
}

// VectorState mirrors State with N*C-wide arrays and the same delay history
// rule. History is chronological: outputs at max(0,Steps-maxDelay) through
// Steps. Before time zero, the initial output is held constant. ConfigHash
// binds the canonical Config JSON. It contains no optimizer, parameters,
// backward tape or disabled mechanisms.
type VectorState struct {
	SchemaVersion string      `json:"schema_version"`
	ConfigHash    string      `json:"config_hash"`
	Steps         uint64      `json:"steps"`
	Voltage       []float64   `json:"voltage"`
	History       [][]float64 `json:"history"`
}

// NewVectorContinuous accepts the same Config as NewContinuous but requires
// StateDimension >= 1 (1 is allowed and is the scalar case) and applies every
// other validation NewContinuous applies, copying its checks instead of
// calling it, because it refuses StateDimension > 1.
func NewVectorContinuous(c Config) (*VectorContinuous, error) {
	if c.Workers < 0 || c.Workers > 1024 {
		return nil, fmt.Errorf("workers must be in [0, 1024]")
	}
	if c.StateDimension < 1 || c.StateDimension > 64 {
		return nil, fmt.Errorf("state dimension must be in [1, 64]")
	}
	switch c.EdgeShape {
	case "", "scalar", "matrix":
	default:
		return nil, fmt.Errorf("unsupported edge shape %q", c.EdgeShape)
	}
	if c.StateDimension == 1 && c.EdgeShape == "matrix" {
		return nil, fmt.Errorf("edge shape matrix requires a vector state dimension")
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
	layout, err := NewVectorLayout(c.Nodes, len(c.Sources), c.StateDimension, c.EdgeShape)
	if err != nil {
		return nil, err
	}
	var act activation
	if c.Activation == "tanh" {
		act = math.Tanh
	} else {
		act = activation(vectorSoftplus)
	}
	return &VectorContinuous{config: c, layout: layout, act: act}, nil
}

func vectorSoftplus(v float64) float64 {
	return math.Max(v, 0) + math.Log1p(math.Exp(-math.Abs(v)))
}

// forwardReader reads delayed source output vectors of a forward Trace.
type vectorForwardReader struct {
	output  [][]float64
	sources []int
	delays  []int
	layout  VectorLayout
	t       int
}

func (r vectorForwardReader) at(e int) []float64 {
	past := 0
	if r.delays[e] < r.t {
		past = r.t - r.delays[e]
	} // avoid subtraction overflow
	return r.layout.NodeSlice(r.output[past], r.sources[e])
}

// ringReader reads delayed source output vectors from an Advance ring slot.
type vectorRingReader struct {
	ring     [][]float64
	sources  []int
	delays   []int
	layout   VectorLayout
	head     int
	capacity int
	step     uint64
	first    uint64
}

func (r vectorRingReader) at(e int) []float64 {
	past := uint64(0)
	if uint64(r.delays[e]) < r.step {
		past = r.step - uint64(r.delays[e])
	}
	offset := int(past - r.first)
	return r.layout.NodeSlice(r.ring[(r.head+offset)%r.capacity], r.sources[e])
}

// Forward evolves a nonempty sequence without changing its arguments. initial
// holds N*C node-major values and each input row holds one external drive
// value per component. The returned trace owns its buffers. Cancellation or
// errors publish no state.
func (m *VectorContinuous) Forward(ctx context.Context, p VectorParameters, initial []float64, inputs [][]float64) (*VectorTrace, error) {
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
	nv := m.layout.NodeValues()
	if err := vector(initial, nv, "initial voltage"); err != nil {
		return nil, err
	}
	if err := vector(p.Weights, m.layout.WeightValues(), "weights"); err != nil {
		return nil, err
	}
	if err := vector(p.Bias, nv, "bias"); err != nil {
		return nil, err
	}
	if err := vector(p.LogTau, n, "log_tau"); err != nil {
		return nil, err
	}
	if len(inputs) > int(^uint(0)>>1)/nv/3-1 {
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
		if len(in) != nv {
			return nil, fmt.Errorf("input[%d] length %d, want %d", t, len(in), nv)
		}
		for i, x := range in {
			if !finite(x) {
				return nil, fmt.Errorf("input[%d][%d] is non-finite", t, i)
			}
		}
	}
	tr := &VectorTrace{
		model:      m,
		parameters: VectorParameters{Weights: append([]float64(nil), p.Weights...), Bias: append([]float64(nil), p.Bias...), LogTau: append([]float64(nil), p.LogTau...)},
		voltage:    make([][]float64, len(inputs)+1),
		output:     make([][]float64, len(inputs)+1),
		drive:      make([][]float64, len(inputs)),
		lambda:     lambda,
		alpha:      alpha,
	}
	tr.voltage[0] = append([]float64(nil), initial...)
	tr.output[0] = make([]float64, nv)
	for i, v := range initial {
		tr.output[0][i] = m.act(v)
	}
	for t, in := range inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		drive := append([]float64(nil), in...)
		r := vectorForwardReader{output: tr.output, sources: m.config.Sources, delays: m.config.Delays, layout: m.layout, t: t}
		for e := range m.config.Sources {
			if err := m.layout.ApplyEdge(p.Weights, e, r.at(e), m.layout.NodeSlice(drive, m.config.Targets[e])); err != nil {
				return nil, err
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		vnext := make([]float64, nv)
		ynext := make([]float64, nv)
		for i := range lambda {
			prev := m.layout.NodeSlice(tr.voltage[t], i)
			d := m.layout.NodeSlice(drive, i)
			vn := m.layout.NodeSlice(vnext, i)
			yn := m.layout.NodeSlice(ynext, i)
			for r := 0; r < m.layout.C; r++ {
				d[r] += p.Bias[i*m.layout.C+r]
				vn[r] = lambda[i]*prev[r] + alpha[i]*d[r]
				yn[r] = m.act(vn[r])
				if !finite(d[r]) || !finite(vn[r]) || !finite(yn[r]) {
					return nil, fmt.Errorf("non-finite state at step %d neuron %d component %d", t, i, r)
				}
			}
		}
		tr.drive[t] = drive
		tr.voltage[t+1] = vnext
		tr.output[t+1] = ynext
	}
	return tr, nil
}

// Outputs returns each step's activated outputs as steps+1 rows of N*C values:
// row 0 is the initial output and row t+1 is the output after input step t. A
// nil or zero-value VectorTrace returns a nil slice.
func (t *VectorTrace) Outputs() [][]float64 {
	if t == nil || len(t.output) == 0 {
		return nil
	}
	return cloneRows(t.output)
}

// Voltages returns each step's voltages as steps+1 rows of N*C values. Row 0
// is the initial voltage. A nil or zero-value VectorTrace returns a nil slice.
func (t *VectorTrace) Voltages() [][]float64 {
	if t == nil || len(t.voltage) == 0 {
		return nil
	}
	return cloneRows(t.voltage)
}

// FinalVoltage returns the last neural voltage, not a complete delayed
// snapshot. A nil or zero-value VectorTrace returns a nil slice.
func (t *VectorTrace) FinalVoltage() []float64 {
	if t == nil || len(t.voltage) == 0 {
		return nil
	}
	return append([]float64(nil), t.voltage[len(t.voltage)-1]...)
}

// NewState starts a trajectory without allocating a delay-sized history. The
// initial voltage is copied. A nil or zero model returns an error.
func (m *VectorContinuous) NewState(initial []float64) (VectorState, error) {
	if err := m.stateModelValid(); err != nil {
		return VectorState{}, err
	}
	nv := m.layout.NodeValues()
	if err := vector(initial, nv, "initial voltage"); err != nil {
		return VectorState{}, err
	}
	hash, err := m.stateConfigHash()
	if err != nil {
		return VectorState{}, err
	}
	output := make([]float64, nv)
	for i, v := range initial {
		output[i] = m.act(v)
	}
	return VectorState{VectorStateVersion, hash, 0, append([]float64(nil), initial...), [][]float64{output}}, nil
}

// ValidateState rejects incompatible topology, missing or malformed history,
// non-finite values and outputs inconsistent with the activation. It does not
// retain or modify the supplied buffers. This method never recomputes or
// repairs the stored history.
func (m *VectorContinuous) ValidateState(s VectorState) error {
	if err := m.stateModelValid(); err != nil {
		return err
	}
	if s.SchemaVersion != VectorStateVersion {
		return fmt.Errorf("unsupported vector state schema %q", s.SchemaVersion)
	}
	hash, err := m.stateConfigHash()
	if err != nil {
		return err
	}
	if s.ConfigHash != hash {
		return fmt.Errorf("vector state configuration fingerprint mismatch")
	}
	nv := m.layout.NodeValues()
	if err := vector(s.Voltage, nv, "state voltage"); err != nil {
		return err
	}
	count, err := m.stateHistoryRows(s.Steps)
	if err != nil {
		return err
	}
	if len(s.History) != count {
		return fmt.Errorf("state history has %d rows, want %d", len(s.History), count)
	}
	for t, row := range s.History {
		if err := vector(row, nv, "state history row"); err != nil {
			return fmt.Errorf("history[%d]: %w", t, err)
		}
		for i, y := range row {
			if (m.config.Activation == "tanh" && (y < -1 || y > 1)) || (m.config.Activation == "softplus" && y < 0) {
				return fmt.Errorf("history[%d][%d] is outside activation range", t, i)
			}
		}
	}
	for i, v := range s.Voltage {
		if !activationMatches(s.History[count-1][i], m.act(v)) {
			return fmt.Errorf("latest history[%d] does not match voltage activation", i)
		}
	}
	return nil
}

// Advance evolves a nonempty input sequence and returns an owned continuation
// and all post-step outputs. Parameters can change between calls; historical
// outputs retain the values computed when they occurred. No gradients cross
// calls. Errors or cancellation return zero results and leave arguments intact.
func (m *VectorContinuous) Advance(ctx context.Context, p VectorParameters, s VectorState, inputs [][]float64) (VectorState, [][]float64, error) {
	if ctx == nil {
		return VectorState{}, nil, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return VectorState{}, nil, err
	}
	if err := m.ValidateState(s); err != nil {
		return VectorState{}, nil, err
	}
	if len(inputs) == 0 {
		return VectorState{}, nil, fmt.Errorf("empty sequence")
	}
	n := m.config.Nodes
	nv := m.layout.NodeValues()
	if len(inputs) > MaxStateValues/nv {
		return VectorState{}, nil, fmt.Errorf("vector output exceeds %d values", MaxStateValues)
	}
	if uint64(len(inputs)) > math.MaxUint64-s.Steps {
		return VectorState{}, nil, fmt.Errorf("vector step counter overflow")
	}
	finalSteps := s.Steps + uint64(len(inputs))
	capacity, err := m.stateHistoryRows(finalSteps)
	if err != nil {
		return VectorState{}, nil, err
	}
	if err = vector(p.Weights, m.layout.WeightValues(), "weights"); err != nil {
		return VectorState{}, nil, err
	}
	if err = vector(p.Bias, nv, "bias"); err != nil {
		return VectorState{}, nil, err
	}
	if err = vector(p.LogTau, n, "log_tau"); err != nil {
		return VectorState{}, nil, err
	}
	lambda, alpha := make([]float64, n), make([]float64, n)
	for i, raw := range p.LogTau {
		tau := math.Exp(raw)
		if !finite(tau) || tau <= 0 {
			return VectorState{}, nil, fmt.Errorf("tau[%d] is not representable and positive", i)
		}
		lambda[i] = math.Exp(-m.config.DT / tau)
		alpha[i] = -math.Expm1(-m.config.DT / tau)
	}
	for t, row := range inputs {
		if err = ctx.Err(); err != nil {
			return VectorState{}, nil, err
		}
		if err = vector(row, nv, "input"); err != nil {
			return VectorState{}, nil, fmt.Errorf("input[%d]: %w", t, err)
		}
	}
	// Ring slots refer only to our copies or newly computed outputs. A slot
	// is replaced, never modified, so earlier returned outputs stay unchanged.
	ring := make([][]float64, capacity)
	for t, row := range s.History {
		ring[t] = append([]float64(nil), row...)
	}
	head, length := 0, len(s.History)
	voltage := append([]float64(nil), s.Voltage...)
	outputs := make([][]float64, len(inputs))
	for t, input := range inputs {
		if err = ctx.Err(); err != nil {
			return VectorState{}, nil, err
		}
		step := s.Steps + uint64(t)
		first := step - uint64(length-1)
		drive := append([]float64(nil), input...)
		r := vectorRingReader{ring: ring, sources: m.config.Sources, delays: m.config.Delays, layout: m.layout, head: head, capacity: capacity, step: step, first: first}
		for e := range m.config.Sources {
			if err = m.layout.ApplyEdge(p.Weights, e, r.at(e), m.layout.NodeSlice(drive, m.config.Targets[e])); err != nil {
				return VectorState{}, nil, err
			}
		}
		if err = ctx.Err(); err != nil {
			return VectorState{}, nil, err
		}
		next, output := make([]float64, nv), make([]float64, nv)
		for i := range lambda {
			prev := m.layout.NodeSlice(voltage, i)
			d := m.layout.NodeSlice(drive, i)
			vn := m.layout.NodeSlice(next, i)
			yn := m.layout.NodeSlice(output, i)
			for r := 0; r < m.layout.C; r++ {
				d[r] += p.Bias[i*m.layout.C+r]
				vn[r] = lambda[i]*prev[r] + alpha[i]*d[r]
				yn[r] = m.act(vn[r])
				if !finite(d[r]) || !finite(vn[r]) || !finite(yn[r]) {
					return VectorState{}, nil, fmt.Errorf("non-finite state at step %d neuron %d component %d", step, i, r)
				}
			}
		}
		outputs[t] = output
		voltage = next
		if length < capacity {
			ring[(head+length)%capacity] = output
			length++
		} else {
			ring[head] = output
			head = (head + 1) % capacity
		}
	}
	history := make([][]float64, length)
	for t := range history {
		history[t] = append([]float64(nil), ring[(head+t)%capacity]...)
	}
	if err = ctx.Err(); err != nil {
		return VectorState{}, nil, err
	}
	return VectorState{VectorStateVersion, s.ConfigHash, finalSteps, voltage, history}, outputs, nil
}

func (m *VectorContinuous) stateModelValid() error {
	if m == nil || m.config.Nodes <= 0 {
		return fmt.Errorf("uninitialized vector model")
	}
	if m.layout.NodeValues() > MaxStateValues {
		return fmt.Errorf("vector state voltage exceeds %d values", MaxStateValues)
	}
	return nil
}

func (m *VectorContinuous) stateHistoryRows(steps uint64) (int, error) {
	maxDelay := 0
	for _, d := range m.config.Delays {
		if d > maxDelay {
			maxDelay = d
		}
	}
	age := steps
	if age > uint64(maxDelay) {
		age = uint64(maxDelay)
	}
	// Check before adding one or converting to int (maxDelay may be MaxInt).
	maxRows := MaxStateValues / m.layout.NodeValues()
	if age >= uint64(maxRows) {
		return 0, fmt.Errorf("vector history exceeds %d values", MaxStateValues)
	}
	return int(age) + 1, nil
}

func (m *VectorContinuous) stateConfigHash() (string, error) {
	data, err := json.Marshal(m.config)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// VectorGradient mirrors Gradient with the vector layouts: Weights has
// layout.WeightValues() entries (matrix edges: dL/dW_ji[r][c] at
// edge*C*C + r*C + c), Bias N*C, LogTau N (summed over the C components),
// Inputs [step][N*C], Initial N*C.
type VectorGradient struct {
	Weights, Bias, LogTau []float64
	Inputs                [][]float64
	Initial               []float64
}

// Backward mirrors Continuous.Backward for the vector core: upstream is
// [step][N*C] (dL/d out at each step), window is the truncation window with
// the same meaning as the scalar core.
func (m *VectorContinuous) Backward(ctx context.Context, tr *VectorTrace, upstream [][]float64, window int) (VectorGradient, error) {
	var empty VectorGradient
	if ctx == nil {
		return empty, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if m == nil || tr == nil || tr.model != m {
		return empty, fmt.Errorf("trace belongs to a different model")
	}
	steps, nv := len(tr.drive), m.layout.NodeValues()
	n := m.config.Nodes
	if window < 0 || len(upstream) != steps {
		return empty, fmt.Errorf("invalid backward window or sequence length")
	}
	if m.layout.C <= 0 || m.layout.Edges != len(m.config.Sources) {
		return empty, fmt.Errorf("layout does not match configuration")
	}
	deriv := func(v float64) float64 {
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
	gy := make([][]float64, steps+1)
	for t := range gy {
		gy[t] = make([]float64, nv)
	}
	for t, row := range upstream {
		if err := vector(row, nv, "upstream"); err != nil {
			return empty, err
		}
		copy(gy[t+1], row)
	}
	g := VectorGradient{Weights: make([]float64, m.layout.WeightValues()), Bias: make([]float64, nv), LogTau: make([]float64, n), Inputs: make([][]float64, steps), Initial: make([]float64, nv)}
	gv := make([]float64, nv)
	c := m.layout.C
	for t := steps - 1; t >= 0; t-- {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		start := 0
		if window > 0 {
			start = t / window * window
		}
		prev := make([]float64, nv)
		g.Inputs[t] = make([]float64, nv)
		for i := 0; i < n; i++ {
			base := i * c
			// d exp(-dt/exp(log_tau)) / d log_tau = lambda*dt/tau.
			dlambda := tr.lambda[i] * (m.config.DT / math.Exp(tr.parameters.LogTau[i]))
			// Underflowed lambda has zero derivative, even if dt/tau overflowed.
			if tr.lambda[i] == 0 {
				dlambda = 0
			}
			for r := 0; r < c; r++ {
				k := base + r
				dv := gv[k] + gy[t+1][k]*deriv(tr.voltage[t+1][k])
				g.Inputs[t][k] = tr.alpha[i] * dv
				g.Bias[k] += g.Inputs[t][k]
				g.LogTau[i] += dv * (tr.voltage[t][k] - tr.drive[t][k]) * dlambda
				if t > start || start == 0 {
					prev[k] = tr.lambda[i] * dv
				}
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
			ti := m.config.Targets[e]
			outJ := m.layout.NodeSlice(tr.output[past], s)
			if m.layout.Matrix {
				w := m.layout.EdgeMatrix(tr.parameters.Weights, e)
				for r := 0; r < c; r++ {
					driveGrad := g.Inputs[t][ti*c+r]
					rowBase := r * c
					for cc := 0; cc < c; cc++ {
						g.Weights[e*c*c+rowBase+cc] += driveGrad * outJ[cc]
					}
					if past > start || start == 0 {
						for cc := 0; cc < c; cc++ {
							gy[past][s*c+cc] += w[rowBase+cc] * driveGrad
						}
					}
				}
			} else {
				w := tr.parameters.Weights[e]
				for r := 0; r < c; r++ {
					driveGrad := g.Inputs[t][ti*c+r]
					g.Weights[e] += driveGrad * outJ[r]
					if past > start || start == 0 {
						gy[past][s*c+r] += w * driveGrad
					}
				}
			}
		}
		gv = prev
	}
	for i := range gv {
		g.Initial[i] = gv[i] + gy[0][i]*deriv(tr.voltage[0][i])
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
