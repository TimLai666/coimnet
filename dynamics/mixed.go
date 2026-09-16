package dynamics

import (
	"context"
	"fmt"
	"math"
)

// Node rule codes of MixedConfig.NodeRule. Every node follows exactly one of
// them: the specification's mixing is by neuron type, not a neuron that emits
// a continuous output and an event at the same time.
const (
	// RuleContinuous integrates the continuous membrane and outputs phi(v).
	RuleContinuous uint8 = 0
	// RuleLIF integrates the spiking membrane and outputs the synaptic trace x.
	RuleLIF uint8 = 1
)

// ContinuousRule is the shared setting of every continuous node in a mixed
// core. The membrane time constant stays a per-node trainable parameter, so
// only the activation lives here. An assignment without a continuous node must
// leave this block empty, so a mixed configuration never carries a knob that
// nothing reads.
type ContinuousRule struct {
	Activation string `json:"activation"`
}

// LIFRule is the shared setting of every LIF node in a mixed core. The fields
// repeat LIFConfig's event parameters and carry the same meaning and the same
// validation; the per-node trainable quantities (log_tau and theta_raw) stay in
// MixedParameters. An assignment without a LIF node must leave this block at
// its zero value, for the reason ContinuousRule documents.
type LIFRule struct {
	TauSyn          float64         `json:"tau_syn"`
	ThetaMin        float64         `json:"theta_min"`
	ThetaMax        float64         `json:"theta_max"`
	VReset          float64         `json:"v_reset"`
	RefractorySteps int             `json:"refractory_steps"`
	Adaptation      LIFAdaptation   `json:"adaptation"`
	Homeostasis     *LIFHomeostasis `json:"homeostasis,omitempty"`
	Surrogate       LIFSurrogate    `json:"surrogate"`
}

// MixedConfig assigns every node to one rule and keeps one clock, one edge
// order and one delay semantics for all of them. Delay zero reads the output at
// the beginning of the step, positive delays read earlier outputs, and before
// the first step each node holds its own prehistory: the activated initial
// voltage for a continuous node and the zero synaptic trace for a LIF node.
//
// Every node has exactly one output series out_j(t): phi(v_j) for a continuous
// node and the decaying synaptic trace x_j for a LIF node. The input current of
// any node, of either rule, is
//
//	I_i(t) = external_i(t) + sum_j w_ji * out_j(t - delay_ji)
//
// so a LIF node's event reaches every downstream neighbour exactly once,
// through x, and never a second time as a raw event.
//
// NodeRule has one entry per node: 0 selects the continuous rule and 1 selects
// the LIF rule. An all-zero assignment reproduces Continuous bit for bit and an
// all-one assignment reproduces LIF bit for bit, on the same parameters.
//
// NodeRule is a []uint8, so encoding/json writes it as a base64 string rather
// than an array of numbers. That is deliberate: the array has one entry per
// neuron, and a whole-brain configuration stores four bytes of JSON per three
// neurons instead of two or more characters each.
type MixedConfig struct {
	Nodes      int            `json:"nodes"`
	Sources    []int          `json:"sources"`
	Targets    []int          `json:"targets"`
	Delays     []int          `json:"delays,omitempty"`
	DT         float64        `json:"dt"`
	NodeRule   []uint8        `json:"node_rule"`
	Continuous ContinuousRule `json:"continuous"`
	LIF        LIFRule        `json:"lif"`
}

// MixedParameters separates learnable quantities from immutable anatomy.
// Weights follows the declared edge order, Bias and LogTau have one entry per
// node of either rule, and ThetaRaw has one entry per LIF node in ascending
// node order. A continuous node owns no threshold, so an assignment with no LIF
// node carries no ThetaRaw at all.
type MixedParameters struct {
	Weights  []float64 `json:"weights"`
	Bias     []float64 `json:"bias"`
	LogTau   []float64 `json:"log_tau"`
	ThetaRaw []float64 `json:"theta_raw"`
}

// MixedGradient contains summed parameter derivatives and per-step input
// gradients, following MixedParameters. ThetaRaw has one entry per LIF node in
// ascending node order. Initial is the gradient of the initial voltage; the
// synaptic trace, the adaptation and the refractory counters always start at
// zero and are not caller supplied state.
type MixedGradient struct {
	Weights, Bias, LogTau, ThetaRaw []float64
	Inputs                          [][]float64
	Initial                         []float64
}

// Mixed is immutable and safe for concurrent independent trajectories.
//
// The rule blocks are validated and their coefficients computed by the two
// single-rule cores themselves: act is a one-node Continuous that owns the
// activation and its derivative, and spike is a one-node LIF that owns kappa,
// rho, the slow stabiliser coefficients and the stabiliser step. Reusing them
// is what keeps a single-rule assignment bit-identical to the core it names,
// permanently, rather than by a copy that could drift.
type Mixed struct {
	config MixedConfig
	act    *Continuous // nil when the assignment has no continuous node
	spike  *LIF        // nil when the assignment has no LIF node

	continuousNodes []int // ascending continuous node indices
	lifNodes        []int // ascending LIF node indices
	// position[i] is the index of node i inside its own rule's list, so a
	// continuous node indexes the continuous half and a LIF node the LIF half.
	position []int

	// smooth replaces the hard event with a differentiable spike so that the
	// declared reverse pass can be checked with central finite differences. It
	// is a test reference inside this package, never a product mode, and it
	// disables the refractory mechanism because that mechanism is an event.
	smooth bool
}

// MixedTrace owns the parameter snapshot and neural history of one forward
// pass. out is the single output series of the contract: phi(v) on a continuous
// node and x on a LIF node, in one array indexed by node. It contains no
// optimizer, mutable anatomy or external side effects.
type MixedTrace struct {
	model      *Mixed
	parameters MixedParameters
	out        [][]float64 // steps+1 rows; row 0 is the prehistory of each rule
	voltage    [][]float64 // steps+1 rows, after any reset; row 0 is the initial voltage
	drive      [][]float64 // steps rows, input + bias + edge contributions
	cand       [][]float64 // steps rows, LIF membrane candidate before the event
	spike      [][]float64 // steps+1 rows, zero on every continuous node
	adapt      [][]float64 // steps+1 rows, zero while the mechanism is disabled
	rate       [][]float64 // steps+1 rows, slow stabiliser activity estimate
	homeo      [][]float64 // steps+1 rows, slow stabiliser threshold offset
	refract    [][]int     // steps+1 rows, counters after each step
	reset      [][]float64 // steps rows, d v(t+1)/d v_cand(t+1) on a LIF node
	dspike     [][]float64 // steps rows, d spike(t+1)/d u; zero while refractory
	thetaSlope []float64   // one entry per LIF node, ascending
	lambda     []float64
	alpha      []float64
}

// NewMixed validates the assignment, both rule blocks and the topology, and
// copies everything it keeps. Duplicate edges are separate additive connections
// in the declared order, exactly as in NewContinuous and NewLIF.
func NewMixed(c MixedConfig) (*Mixed, error) {
	if c.Nodes <= 0 || !finite(c.DT) || c.DT <= 0 {
		return nil, fmt.Errorf("nodes and finite dt must be positive")
	}
	if len(c.NodeRule) != c.Nodes {
		return nil, fmt.Errorf("node_rule has %d entries, the model has %d nodes", len(c.NodeRule), c.Nodes)
	}
	c.NodeRule = append([]uint8(nil), c.NodeRule...)
	var continuousNodes, lifNodes []int
	position := make([]int, c.Nodes)
	for i, rule := range c.NodeRule {
		switch rule {
		case RuleContinuous:
			position[i] = len(continuousNodes)
			continuousNodes = append(continuousNodes, i)
		case RuleLIF:
			position[i] = len(lifNodes)
			lifNodes = append(lifNodes, i)
		default:
			return nil, fmt.Errorf("node_rule[%d] is %d, want %d (continuous) or %d (lif)", i, rule, RuleContinuous, RuleLIF)
		}
	}
	m := &Mixed{continuousNodes: continuousNodes, lifNodes: lifNodes, position: position}
	// A declared block that no node reads would be a second, silently ignored
	// knob, so an unused rule has to be left empty rather than merely unused.
	if len(continuousNodes) == 0 {
		if c.Continuous != (ContinuousRule{}) {
			return nil, fmt.Errorf("the continuous rule is declared but no node follows it")
		}
	} else {
		act, err := NewContinuous(Config{Nodes: 1, DT: c.DT, Activation: c.Continuous.Activation})
		if err != nil {
			return nil, err
		}
		m.act = act
	}
	if len(lifNodes) == 0 {
		if c.LIF != (LIFRule{}) {
			return nil, fmt.Errorf("the lif rule is declared but no node follows it")
		}
	} else {
		spike, err := NewLIF(LIFConfig{
			Nodes: 1, DT: c.DT, TauSyn: c.LIF.TauSyn,
			ThetaMin: c.LIF.ThetaMin, ThetaMax: c.LIF.ThetaMax, VReset: c.LIF.VReset,
			RefractorySteps: c.LIF.RefractorySteps, Adaptation: c.LIF.Adaptation,
			Homeostasis: c.LIF.Homeostasis, Surrogate: c.LIF.Surrogate,
		})
		if err != nil {
			return nil, err
		}
		m.spike = spike
		// The block is owned from here on, exactly like the edge arrays.
		c.LIF.Homeostasis = spike.config.Homeostasis
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
	m.config = c
	return m, nil
}

// Config returns an independent topology/configuration copy.
// A nil or zero-value Mixed returns MixedConfig{}.
func (m *Mixed) Config() MixedConfig {
	if m == nil {
		return MixedConfig{}
	}
	c := m.config
	c.Sources = append([]int(nil), c.Sources...)
	c.Targets = append([]int(nil), c.Targets...)
	c.Delays = append([]int(nil), c.Delays...)
	c.NodeRule = append([]uint8(nil), c.NodeRule...)
	if c.LIF.Homeostasis != nil {
		owned := *c.LIF.Homeostasis
		c.LIF.Homeostasis = &owned
	}
	return c
}

// ContinuousNodes returns the ascending continuous node indices.
func (m *Mixed) ContinuousNodes() []int {
	if m == nil {
		return nil
	}
	return append([]int(nil), m.continuousNodes...)
}

// LIFNodes returns the ascending LIF node indices. Entry k of MixedParameters
// .ThetaRaw and of MixedGradient.ThetaRaw belongs to node LIFNodes()[k].
func (m *Mixed) LIFNodes() []int {
	if m == nil {
		return nil
	}
	return append([]int(nil), m.lifNodes...)
}

// spiking reports whether node i follows the LIF rule.
func (m *Mixed) spiking(i int) bool { return m.config.NodeRule[i] == RuleLIF }

// adapting and stabilising answer once for the whole model, so an absent block
// and a disabled block behave identically, exactly as they do on the LIF core.
func (m *Mixed) adapting() bool    { return m.spike != nil && m.config.LIF.Adaptation.Enabled }
func (m *Mixed) stabilising() bool { return m.spike != nil && m.spike.homeostatic() }

// event repeats LIF.event for the mixed core. It is the declared fast_sigmoid
// surrogate psi(u) = 1/(1+scale*|u|)^2 on a hard 0/1 event, and the test-only
// smooth mode emits sigma(u) = 0.5*(1+scale*u/(1+scale*|u|)) with that
// function's exact derivative. The all-one bit identity test pins this against
// LIF.event over the whole pipeline.
func (m *Mixed) event(u float64) (spike, derivative float64) {
	scale := m.config.LIF.Surrogate.Scale
	den := 1 + scale*math.Abs(u)
	psi := 1 / (den * den)
	if m.smooth {
		return .5 * (1 + scale*u/den), .5 * scale * psi
	}
	if u >= 0 {
		return 1, psi
	}
	return 0, psi
}

// membrane is the one leaky-integration step both rules share:
//
//	v_cand = lambda*v + alpha*drive
//
// It lives in its own function, and is deliberately not inlined, so that its
// floating-point contraction is decided once and never by the code around it.
// Go may fuse a multiply and an add into a single rounded operation, and which
// of the two products it fuses depends on the surrounding basic block, so the
// same expression written inline in this file and in continuous.go can differ
// in the last bit under some build configurations (the race instrumented build
// is one). Keeping it here is what makes an all-continuous or an all-LIF
// assignment bit-identical to the core it names under every build.
//
//go:noinline
func membrane(lambda, voltage, alpha, drive float64) float64 {
	return lambda*voltage + alpha*drive
}

// leak repeats the membrane coefficients of both cores. -Expm1 avoids
// cancellation in 1-exp(-dt/tau) for a small time step.
func (m *Mixed) leak(logTau []float64) (lambda, alpha []float64, err error) {
	lambda, alpha = make([]float64, len(logTau)), make([]float64, len(logTau))
	for i, raw := range logTau {
		tau := math.Exp(raw)
		if !finite(tau) || tau <= 0 {
			return nil, nil, fmt.Errorf("tau[%d] is not representable and positive", i)
		}
		lambda[i] = math.Exp(-m.config.DT / tau)
		alpha[i] = -math.Expm1(-m.config.DT / tau)
	}
	return lambda, alpha, nil
}

// baseThreshold repeats the bounded transform of the LIF core, one entry per
// LIF node in ascending order. slope is nil when the caller does not need it.
func (m *Mixed) baseThreshold(thetaRaw []float64, withSlope bool) (base, slope []float64, err error) {
	span := m.config.LIF.ThetaMax - m.config.LIF.ThetaMin
	base = make([]float64, len(thetaRaw))
	if withSlope {
		slope = make([]float64, len(thetaRaw))
	}
	for k, raw := range thetaRaw {
		s := logistic(raw)
		base[k] = m.config.LIF.ThetaMin + span*s
		if !finite(base[k]) {
			return nil, nil, fmt.Errorf("theta_base[%d] is not representable", k)
		}
		if withSlope {
			slope[k] = span * s * (1 - s)
			if !finite(slope[k]) {
				return nil, nil, fmt.Errorf("theta_base[%d] is not representable", k)
			}
		}
	}
	return base, slope, nil
}

// validateParameters checks the four arrays against the assignment.
func (m *Mixed) validateParameters(p MixedParameters) error {
	if err := vector(p.Weights, len(m.config.Sources), "weights"); err != nil {
		return err
	}
	if err := vector(p.Bias, m.config.Nodes, "bias"); err != nil {
		return err
	}
	if err := vector(p.LogTau, m.config.Nodes, "log_tau"); err != nil {
		return err
	}
	return vector(p.ThetaRaw, len(m.lifNodes), "theta_raw")
}

// Forward evolves a nonempty sequence without changing its arguments. Each node
// follows its own rule under one clock and one edge order. The returned trace
// owns its buffers; cancellation or errors publish no state.
func (m *Mixed) Forward(ctx context.Context, p MixedParameters, initial []float64, inputs [][]float64) (*MixedTrace, error) {
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
	if err := m.validateParameters(p); err != nil {
		return nil, err
	}
	if len(inputs) > int(^uint(0)>>1)/n/8-1 {
		return nil, fmt.Errorf("sequence shape overflows")
	}
	lambda, alpha, err := m.leak(p.LogTau)
	if err != nil {
		return nil, err
	}
	base, slope, err := m.baseThreshold(p.ThetaRaw, true)
	if err != nil {
		return nil, err
	}
	for t, in := range inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := vector(in, n, fmt.Sprintf("input[%d]", t)); err != nil {
			return nil, err
		}
	}
	steps := len(inputs)
	tr := &MixedTrace{
		model: m, parameters: cloneMixedParameters(p),
		out: make([][]float64, steps+1), voltage: make([][]float64, steps+1),
		drive: make([][]float64, steps), cand: make([][]float64, steps),
		spike: make([][]float64, steps+1), adapt: make([][]float64, steps+1),
		rate: make([][]float64, steps+1), homeo: make([][]float64, steps+1),
		refract: make([][]int, steps+1), reset: make([][]float64, steps),
		dspike: make([][]float64, steps), thetaSlope: slope,
		lambda: lambda, alpha: alpha,
	}
	tr.voltage[0] = append([]float64(nil), initial...)
	tr.out[0] = make([]float64, n)
	for _, i := range m.continuousNodes {
		tr.out[0][i] = m.act.activate(initial[i])
	}
	tr.spike[0], tr.adapt[0] = make([]float64, n), make([]float64, n)
	tr.rate[0], tr.homeo[0] = make([]float64, n), make([]float64, n)
	tr.refract[0] = make([]int, n)
	adapting, stabilising := m.adapting(), m.stabilising()
	for t, in := range inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		drive := append([]float64(nil), in...)
		for e, s := range m.config.Sources {
			if e%4096 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			past := 0
			if m.config.Delays[e] < t {
				past = t - m.config.Delays[e]
			} // avoid subtraction overflow
			drive[m.config.Targets[e]] += p.Weights[e] * tr.out[past][s]
		}
		voltage, out := make([]float64, n), make([]float64, n)
		cand, spike := make([]float64, n), make([]float64, n)
		adapt, reset := make([]float64, n), make([]float64, n)
		rate, homeo := make([]float64, n), make([]float64, n)
		derivative := make([]float64, n)
		refract := make([]int, n)
		for i := range drive {
			drive[i] += p.Bias[i]
			// Both rules leak the membrane with the same coefficients, so the
			// candidate is computed once for either of them.
			next := membrane(lambda[i], tr.voltage[t][i], alpha[i], drive[i])
			if !m.spiking(i) {
				voltage[i] = next
				out[i] = m.act.activate(voltage[i])
				if !finite(drive[i]) || !finite(voltage[i]) || !finite(out[i]) {
					return nil, fmt.Errorf("non-finite state at step %d neuron %d", t, i)
				}
				continue
			}
			k := m.position[i]
			cand[i] = next
			theta := base[k]
			if adapting {
				theta += tr.adapt[t][i]
			}
			if stabilising {
				theta += tr.homeo[t][i]
			}
			switch {
			case !m.smooth && tr.refract[t][i] > 0:
				// Hold and ignore: the drive of this step never reaches the
				// membrane, so it carries no gradient either.
				voltage[i] = m.config.LIF.VReset
				refract[i] = tr.refract[t][i] - 1
			default:
				spike[i], derivative[i] = m.event(cand[i] - theta)
				reset[i] = 1 - spike[i]
				voltage[i] = reset[i]*cand[i] + spike[i]*m.config.LIF.VReset
				if !m.smooth && spike[i] != 0 {
					refract[i] = m.config.LIF.RefractorySteps
				}
			}
			out[i] = m.spike.kappa*tr.out[t][i] + spike[i]
			if adapting {
				adapt[i] = m.spike.rho*tr.adapt[t][i] + m.config.LIF.Adaptation.Beta*spike[i]
			}
			if stabilising {
				rate[i], homeo[i] = m.spike.stabilise(tr.rate[t][i], tr.homeo[t][i], spike[i])
			}
			if !finite(drive[i]) || !finite(cand[i]) || !finite(voltage[i]) || !finite(out[i]) || !finite(adapt[i]) || !finite(rate[i]) || !finite(homeo[i]) {
				return nil, fmt.Errorf("non-finite state at step %d neuron %d", t, i)
			}
		}
		tr.drive[t], tr.cand[t], tr.reset[t], tr.dspike[t] = drive, cand, reset, derivative
		tr.voltage[t+1], tr.spike[t+1], tr.out[t+1] = voltage, spike, out
		tr.adapt[t+1], tr.refract[t+1] = adapt, refract
		tr.rate[t+1], tr.homeo[t+1] = rate, homeo
	}
	return tr, nil
}

// Outputs returns the single output series of every node after each input step,
// excluding prehistory: the activated output of a continuous node and the
// synaptic trace of a LIF node. This is the value edges and readouts observe.
// A nil or zero-value MixedTrace returns a nil slice.
func (tr *MixedTrace) Outputs() [][]float64 {
	if tr == nil || len(tr.out) <= 1 {
		return nil
	}
	return cloneRows(tr.out[1:])
}

// Spikes returns the 0/1 events after each input step, excluding prehistory.
// A continuous node owns no event, so its entry is always zero; the rows keep
// the full node width so that a caller indexes them by node.
// A nil or zero-value MixedTrace returns a nil slice.
func (tr *MixedTrace) Spikes() [][]float64 {
	if tr == nil || len(tr.spike) <= 1 {
		return nil
	}
	return cloneRows(tr.spike[1:])
}

// Voltages returns the membrane voltage of every node after each input step,
// after any reset. A nil or zero-value MixedTrace returns a nil slice.
func (tr *MixedTrace) Voltages() [][]float64 {
	if tr == nil || len(tr.voltage) <= 1 {
		return nil
	}
	return cloneRows(tr.voltage[1:])
}

// FinalVoltage returns the last membrane voltage, not a complete delayed
// snapshot. A nil or zero-value MixedTrace returns a nil slice.
func (tr *MixedTrace) FinalVoltage() []float64 {
	if tr == nil || len(tr.voltage) == 0 {
		return nil
	}
	return append([]float64(nil), tr.voltage[len(tr.voltage)-1]...)
}

// Backward applies each node's own reverse rule to every step: a continuous
// node follows the continuous reverse pass and a LIF node the declared
// fast_sigmoid surrogate, with the reset and the refractory step detached
// exactly as they are on the LIF core. A cross-type edge is an ordinary w*out
// product, so its upstream gradient is routed into the source node's own rule.
//
// upstream[t] is the loss gradient of the output series after step t. Window
// zero propagates through the complete sequence; a positive window detaches at
// fixed boundaries (0, window, 2*window, ...), including delayed edges, exactly
// like Continuous.Backward and LIF.Backward. Trace and upstream are unchanged
// and no parameter is updated.
func (m *Mixed) Backward(ctx context.Context, tr *MixedTrace, upstream [][]float64, window int) (MixedGradient, error) {
	var empty MixedGradient
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
	gout := make([][]float64, steps+1)
	for t := range gout {
		gout[t] = make([]float64, n)
	}
	for t, row := range upstream {
		if err := vector(row, n, "upstream"); err != nil {
			return empty, err
		}
		copy(gout[t+1], row)
	}
	g := MixedGradient{
		Weights: make([]float64, len(tr.parameters.Weights)), Bias: make([]float64, n),
		LogTau: make([]float64, n), ThetaRaw: make([]float64, len(m.lifNodes)),
		Inputs: make([][]float64, steps), Initial: make([]float64, n),
	}
	adapting := m.adapting()
	gv, ga := make([]float64, n), make([]float64, n)
	for t := steps - 1; t >= 0; t-- {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		start := 0
		if window > 0 {
			start = t / window * window
		}
		keep := t > start || start == 0
		prevV, prevA := make([]float64, n), make([]float64, n)
		g.Inputs[t] = make([]float64, n)
		for i := 0; i < n; i++ {
			// dcand is the gradient of the membrane candidate of this step. Both
			// rules reach it differently and then share the same membrane
			// arithmetic, exactly as the forward pass does.
			var dcand float64
			if !m.spiking(i) {
				dcand = gv[i] + gout[t+1][i]*m.act.derivative(tr.voltage[t+1][i])
			} else {
				k := m.position[i]
				// The event feeds the synaptic trace and the adaptation.
				dspike := gout[t+1][i]
				if adapting {
					dspike += ga[i] * m.config.LIF.Adaptation.Beta
				}
				if m.smooth {
					// The smooth reference mixes v_cand and v_reset continuously,
					// so its reset coefficient is a real dependency. The product
					// model keeps the declared detached reset instead.
					dspike += gv[i] * (m.config.LIF.VReset - tr.cand[t][i])
				}
				if keep {
					gout[t][i] += m.spike.kappa * gout[t+1][i]
					if adapting {
						prevA[i] = m.spike.rho * ga[i]
					}
				}
				du := dspike * tr.dspike[t][i]
				dcand = gv[i]*tr.reset[t][i] + du
				// theta_effective = theta_base + a(t) + h(t), so the threshold
				// gradient is -du on both the bounded transform and the
				// adaptation state. The slow stabiliser offset is a declared
				// constant of the reverse pass, exactly as on the LIF core.
				if adapting && keep {
					prevA[i] -= du
				}
				g.ThetaRaw[k] -= du * tr.thetaSlope[k]
			}
			g.Inputs[t][i] = tr.alpha[i] * dcand
			g.Bias[i] += g.Inputs[t][i]
			// d exp(-dt/exp(log_tau)) / d log_tau = lambda*dt/tau.
			dlambda := tr.lambda[i] * (m.config.DT / math.Exp(tr.parameters.LogTau[i]))
			// Underflowed lambda has zero derivative, even if dt/tau overflowed.
			if tr.lambda[i] == 0 {
				dlambda = 0
			}
			g.LogTau[i] += dcand * (tr.voltage[t][i] - tr.drive[t][i]) * dlambda
			if keep {
				prevV[i] = tr.lambda[i] * dcand
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
			g.Weights[e] += driveGrad * tr.out[past][s]
			if past > start || start == 0 {
				gout[past][s] += driveGrad * tr.parameters.Weights[e]
			}
		}
		gv, ga = prevV, prevA
	}
	// A continuous node's initial voltage also reaches the loss through its own
	// prehistory output; a LIF node's prehistory trace is the declared zero and
	// is not caller supplied, so only the voltage carries a gradient there.
	for i := range gv {
		g.Initial[i] = gv[i]
		if !m.spiking(i) {
			g.Initial[i] += gout[0][i] * m.act.derivative(tr.voltage[0][i])
		}
	}
	for _, v := range [][]float64{g.Weights, g.Bias, g.LogTau, g.ThetaRaw, g.Initial} {
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

func cloneMixedParameters(p MixedParameters) MixedParameters {
	return MixedParameters{
		append([]float64(nil), p.Weights...), append([]float64(nil), p.Bias...),
		append([]float64(nil), p.LogTau...), append([]float64(nil), p.ThetaRaw...),
	}
}
