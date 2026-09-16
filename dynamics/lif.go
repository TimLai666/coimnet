package dynamics

import (
	"context"
	"fmt"
	"math"
)

// LIFConfig defines immutable connectivity and the fixed event parameters of a
// leaky integrate-and-fire core. Delay semantics match Config: delay zero reads
// the synaptic trace at the beginning of the step, positive delays read earlier
// traces, and before the first step the trace is held at its zero prehistory.
// TauSyn, the thresholds, the refractory length and the surrogate are model
// settings, not learnable parameters.
type LIFConfig struct {
	Nodes           int           `json:"nodes"`
	Sources         []int         `json:"sources"`
	Targets         []int         `json:"targets"`
	Delays          []int         `json:"delays,omitempty"`
	DT              float64       `json:"dt"`
	TauSyn          float64       `json:"tau_syn"`
	ThetaMin        float64       `json:"theta_min"`
	ThetaMax        float64       `json:"theta_max"`
	VReset          float64       `json:"v_reset"`
	RefractorySteps int           `json:"refractory_steps"`
	Adaptation      LIFAdaptation `json:"adaptation"`
	// Homeostasis is the optional slow activity stabiliser. The pointer is
	// omitted when no block is declared, so every configuration written before
	// the mechanism existed keeps its exact canonical JSON and therefore its
	// recorded fingerprint.
	Homeostasis *LIFHomeostasis `json:"homeostasis,omitempty"`
	Surrogate   LIFSurrogate    `json:"surrogate"`
	// Workers selects the forward and persistent-state execution mode: 0 or 1
	// is the single threaded reference mode; more than 1 enables controlled
	// parallelism that partitions work by target node across a fixed number
	// of workers, summing each target's inputs in declared edge order so the
	// result is bit-identical to the single threaded pass.
	Workers int `json:"workers,omitempty"`
}

// LIFHomeostasis is the slow activity stabiliser of the spiking core. Each
// neuron keeps one activity estimate r and one threshold offset h:
//
//	r(t+1) = r(t) + (dt/TauRate) * (spike(t+1) - r(t))
//	h(t+1) = clamp(h(t) + Eta*dt*(r(t+1) - TargetRate), 0, HMax)
//
// and the event of a step compares against theta_base + adaptation + h. The
// offset only raises the threshold or falls back to zero, which is the bounded
// adjustment the specification asks for, never an unbounded gain. Both values
// are state, not learnable parameters: the reverse pass treats h as a constant
// exactly as it treats the adaptation. Disabled leaves r and h at zero and
// restores the behaviour of a model that never had the mechanism.
type LIFHomeostasis struct {
	Enabled    bool    `json:"enabled"`
	TauRate    float64 `json:"tau_rate"`
	TargetRate float64 `json:"target_rate"`
	Eta        float64 `json:"eta"`
	HMax       float64 `json:"h_max"`
}

// LIFAdaptation is the short term threshold adaptation
// a(t+1) = exp(-dt/tau_adapt)*a(t) + beta*spike(t+1). Disabled leaves a at zero
// and restores the behaviour of a model that never had the mechanism.
type LIFAdaptation struct {
	Enabled  bool    `json:"enabled"`
	TauAdapt float64 `json:"tau_adapt"`
	Beta     float64 `json:"beta"`
}

// LIFSurrogate declares the reverse pass approximation of the hard event.
// Only "fast_sigmoid", psi(u) = 1/(1+scale*|u|)^2, is supported.
type LIFSurrogate struct {
	Kind  string  `json:"kind"`
	Scale float64 `json:"scale"`
}

// LIFParameters separates learnable quantities from immutable anatomy. Tau is
// exp(LogTau) and the base threshold is a bounded transform of ThetaRaw, so
// every finite ThetaRaw is accepted.
type LIFParameters struct {
	Weights  []float64 `json:"weights"`
	Bias     []float64 `json:"bias"`
	LogTau   []float64 `json:"log_tau"`
	ThetaRaw []float64 `json:"theta_raw"`
}

// LIFGradient contains summed parameter derivatives and per-step input
// gradients. Initial is the gradient of the initial voltage; the synaptic
// trace, the adaptation and the refractory counters always start at zero and
// are not caller supplied state.
type LIFGradient struct {
	Weights, Bias, LogTau, ThetaRaw []float64
	Inputs                          [][]float64
	Initial                         []float64
}

// LIF is immutable and safe for concurrent independent trajectories.
type LIF struct {
	config LIFConfig
	// partition groups edges by target for Workers > 1. The reference mode
	// keeps it nil and uses the original eager accumulation.
	partition *targetPartition
	kappa     float64
	rho       float64
	// rateStep is dt/tau_rate and offsetStep is eta*dt; both are zero while the
	// slow stabiliser is disabled.
	rateStep   float64
	offsetStep float64
	// smooth replaces the hard event with a differentiable spike so that the
	// declared reverse pass can be checked with central finite differences.
	// It is a test reference inside this package, never a product mode, and
	// disables the refractory mechanism because that mechanism is an event.
	smooth bool
}

// LIFTrace owns the parameter snapshot and neural history of one forward pass.
// It contains no optimizer, mutable anatomy or external side effects.
type LIFTrace struct {
	model      *LIF
	parameters LIFParameters
	rate       [][]float64 // steps+1 rows, activity estimate r; row 0 is zero
	homeo      [][]float64 // steps+1 rows, threshold offset h; row 0 is zero
	voltage    [][]float64 // steps+1 rows, after reset; row 0 is the initial voltage
	cand       [][]float64 // steps rows, membrane candidate before the event
	drive      [][]float64 // steps rows, input + bias + edge contributions
	syn        [][]float64 // steps+1 rows, row 0 is the zero prehistory trace
	spike      [][]float64 // steps+1 rows, row 0 is the zero prehistory event
	adapt      [][]float64 // steps+1 rows, row 0 is the zero initial adaptation
	refract    [][]int     // steps+1 rows, counters after each step
	reset      [][]float64 // steps rows, d v(t+1)/d v_cand(t+1): 1-spike, or 0 while refractory
	dspike     [][]float64 // steps rows, d spike(t+1)/d u; zero while refractory
	thetaSlope []float64   // d theta_base/d theta_raw of the bounded transform
	lambda     []float64
	alpha      []float64
}

// NewLIF validates and copies the topology. Duplicate edges are separate
// additive connections in the declared order, exactly as in NewContinuous.
func NewLIF(c LIFConfig) (*LIF, error) {
	if c.Workers < 0 || c.Workers > 1024 {
		return nil, fmt.Errorf("workers must be in [0, 1024]")
	}
	if c.Nodes <= 0 || !finite(c.DT) || c.DT <= 0 {
		return nil, fmt.Errorf("nodes and finite dt must be positive")
	}
	if !finite(c.TauSyn) || c.TauSyn <= 0 {
		return nil, fmt.Errorf("finite tau_syn must be positive")
	}
	if !finite(c.VReset) || !finite(c.ThetaMin) || !finite(c.ThetaMax) || c.VReset >= c.ThetaMin || c.ThetaMin >= c.ThetaMax {
		return nil, fmt.Errorf("finite thresholds must satisfy v_reset < theta_min < theta_max")
	}
	if c.RefractorySteps < 0 {
		return nil, fmt.Errorf("refractory steps must not be negative")
	}
	if c.Surrogate.Kind != "fast_sigmoid" {
		return nil, fmt.Errorf("unsupported surrogate %q", c.Surrogate.Kind)
	}
	if !finite(c.Surrogate.Scale) || c.Surrogate.Scale <= 0 {
		return nil, fmt.Errorf("finite surrogate scale must be positive")
	}
	rho := 0.0
	if c.Adaptation.Enabled {
		if !finite(c.Adaptation.TauAdapt) || c.Adaptation.TauAdapt <= 0 {
			return nil, fmt.Errorf("finite tau_adapt must be positive")
		}
		if !finite(c.Adaptation.Beta) || c.Adaptation.Beta < 0 {
			return nil, fmt.Errorf("finite beta must not be negative")
		}
		rho = math.Exp(-c.DT / c.Adaptation.TauAdapt)
	}
	rateStep, offsetStep := 0.0, 0.0
	if c.Homeostasis != nil {
		// The block is owned from here on, exactly like the edge arrays.
		owned := *c.Homeostasis
		c.Homeostasis = &owned
		if owned.Enabled {
			if !finite(owned.TauRate) || owned.TauRate <= 0 {
				return nil, fmt.Errorf("finite tau_rate must be positive")
			}
			if !finite(owned.TargetRate) || owned.TargetRate < 0 || owned.TargetRate > 1 {
				return nil, fmt.Errorf("target_rate must lie in [0, 1]")
			}
			if !finite(owned.Eta) || owned.Eta < 0 {
				return nil, fmt.Errorf("finite eta must not be negative")
			}
			if !finite(owned.HMax) || owned.HMax < 0 {
				return nil, fmt.Errorf("finite h_max must not be negative")
			}
			// The offset only raises the threshold, so the highest effective
			// threshold the mechanism can reach is theta_min + h_max plus the
			// adaptation. Requiring that ceiling to stay finite and above
			// v_reset keeps v_reset < theta_effective true at every step.
			ceiling := c.ThetaMin + owned.HMax
			if !finite(ceiling) || c.VReset >= ceiling {
				return nil, fmt.Errorf("theta_min + h_max must be finite and above v_reset")
			}
			rateStep, offsetStep = c.DT/owned.TauRate, owned.Eta*c.DT
			if !finite(rateStep) || !finite(offsetStep) {
				return nil, fmt.Errorf("dt/tau_rate and eta*dt must be representable")
			}
		}
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
	// The partition is built only for parallel runs, after every validation,
	// so a huge declared node count never allocates before an invalid
	// configuration is rejected.
	var partition *targetPartition
	if c.Workers > 1 {
		var err error
		partition, err = newTargetPartition(c.Nodes, c.Workers, c.Sources, c.Targets)
		if err != nil {
			return nil, err
		}
	}
	return &LIF{config: c, partition: partition, kappa: math.Exp(-c.DT / c.TauSyn), rho: rho, rateStep: rateStep, offsetStep: offsetStep}, nil
}

// homeostatic reports whether a declared and enabled slow stabiliser exists.
// Everything downstream branches on this one answer, so an absent block and a
// disabled block behave identically.
func (m *LIF) homeostatic() bool {
	return m.config.Homeostasis != nil && m.config.Homeostasis.Enabled
}

// stabilise applies the two declared rules of one neuron for one step and
// returns the new activity estimate and the new bounded threshold offset.
func (m *LIF) stabilise(rate, offset, spike float64) (float64, float64) {
	h := m.config.Homeostasis
	next := rate + m.rateStep*(spike-rate)
	shifted := offset + m.offsetStep*(next-h.TargetRate)
	// Plain comparisons so a non-finite value survives the clamp and is
	// rejected by the caller's finite check instead of being hidden as zero.
	if shifted < 0 {
		shifted = 0
	} else if shifted > h.HMax {
		shifted = h.HMax
	}
	return next, shifted
}

// Config returns an independent topology/configuration copy.
// A nil or zero-value LIF returns LIFConfig{}.
func (m *LIF) Config() LIFConfig {
	if m == nil {
		return LIFConfig{}
	}
	c := m.config
	c.Sources = append([]int(nil), c.Sources...)
	c.Targets = append([]int(nil), c.Targets...)
	c.Delays = append([]int(nil), c.Delays...)
	if c.Homeostasis != nil {
		owned := *c.Homeostasis
		c.Homeostasis = &owned
	}
	return c
}

// Forward evolves a nonempty sequence without changing its arguments. Each step
// leaks the membrane, adds the drive, compares against the effective threshold,
// emits at most one event, resets and decays the synaptic trace. A refractory
// neuron holds v_reset and ignores that step's drive entirely. The returned
// trace owns its buffers; cancellation or errors publish no state.
func (m *LIF) Forward(ctx context.Context, p LIFParameters, initial []float64, inputs [][]float64) (*LIFTrace, error) {
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
	if err := vector(p.ThetaRaw, n, "theta_raw"); err != nil {
		return nil, err
	}
	if len(inputs) > int(^uint(0)>>1)/n/8-1 {
		return nil, fmt.Errorf("sequence shape overflows")
	}
	lambda, alpha := make([]float64, n), make([]float64, n)
	for i, raw := range p.LogTau {
		tau := math.Exp(raw)
		if !finite(tau) || tau <= 0 {
			return nil, fmt.Errorf("tau[%d] is not representable and positive", i)
		}
		lambda[i] = math.Exp(-m.config.DT / tau)
		// Avoid cancellation in 1-exp(-dt/tau) for small positive dt/tau.
		alpha[i] = -math.Expm1(-m.config.DT / tau)
	}
	span := m.config.ThetaMax - m.config.ThetaMin
	base, slope := make([]float64, n), make([]float64, n)
	for i, raw := range p.ThetaRaw {
		s := logistic(raw)
		base[i] = m.config.ThetaMin + span*s
		slope[i] = span * s * (1 - s)
		if !finite(base[i]) || !finite(slope[i]) {
			return nil, fmt.Errorf("theta_base[%d] is not representable", i)
		}
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
	tr := &LIFTrace{
		model: m, parameters: cloneLIFParameters(p),
		voltage: make([][]float64, steps+1), cand: make([][]float64, steps),
		drive: make([][]float64, steps), syn: make([][]float64, steps+1),
		spike: make([][]float64, steps+1), adapt: make([][]float64, steps+1),
		rate: make([][]float64, steps+1), homeo: make([][]float64, steps+1),
		refract: make([][]int, steps+1), reset: make([][]float64, steps),
		dspike: make([][]float64, steps), thetaSlope: slope,
		lambda: lambda, alpha: alpha,
	}
	tr.voltage[0] = append([]float64(nil), initial...)
	tr.syn[0], tr.spike[0], tr.adapt[0] = make([]float64, n), make([]float64, n), make([]float64, n)
	tr.rate[0], tr.homeo[0] = make([]float64, n), make([]float64, n)
	tr.refract[0] = make([]int, n)
	stabilising := m.homeostatic()
	for t, in := range inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		drive := append([]float64(nil), in...)
		r := forwardReader{output: tr.syn, sources: m.config.Sources, delays: m.config.Delays, t: t}
		if m.partition != nil {
			if err := m.lifSynapticDriveParallel(drive, p.Weights, &r); err != nil {
				return nil, err
			}
		} else {
			r.accumulateSerial(drive, p.Weights, m.config.Targets)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cand, voltage := make([]float64, n), make([]float64, n)
		spike, synapse := make([]float64, n), make([]float64, n)
		adapt, reset := make([]float64, n), make([]float64, n)
		rate, homeo := make([]float64, n), make([]float64, n)
		derivative := make([]float64, n)
		refract := make([]int, n)
		for i := range drive {
			drive[i] += p.Bias[i]
			cand[i] = lambda[i]*tr.voltage[t][i] + alpha[i]*drive[i]
			theta := base[i]
			if m.config.Adaptation.Enabled {
				theta += tr.adapt[t][i]
			}
			if stabilising {
				theta += tr.homeo[t][i]
			}
			switch {
			case !m.smooth && tr.refract[t][i] > 0:
				// Hold and ignore: the drive of this step never reaches the
				// membrane, so it carries no gradient either.
				voltage[i] = m.config.VReset
				refract[i] = tr.refract[t][i] - 1
			default:
				spike[i], derivative[i] = m.event(cand[i] - theta)
				reset[i] = 1 - spike[i]
				voltage[i] = reset[i]*cand[i] + spike[i]*m.config.VReset
				if !m.smooth && spike[i] != 0 {
					refract[i] = m.config.RefractorySteps
				}
			}
			synapse[i] = m.kappa*tr.syn[t][i] + spike[i]
			if m.config.Adaptation.Enabled {
				adapt[i] = m.rho*tr.adapt[t][i] + m.config.Adaptation.Beta*spike[i]
			}
			if stabilising {
				rate[i], homeo[i] = m.stabilise(tr.rate[t][i], tr.homeo[t][i], spike[i])
			}
			if !finite(drive[i]) || !finite(cand[i]) || !finite(voltage[i]) || !finite(synapse[i]) || !finite(adapt[i]) || !finite(rate[i]) || !finite(homeo[i]) {
				return nil, fmt.Errorf("non-finite state at step %d neuron %d", t, i)
			}
		}
		tr.drive[t], tr.cand[t], tr.reset[t], tr.dspike[t] = drive, cand, reset, derivative
		tr.voltage[t+1], tr.spike[t+1], tr.syn[t+1] = voltage, spike, synapse
		tr.adapt[t+1], tr.refract[t+1] = adapt, refract
		tr.rate[t+1], tr.homeo[t+1] = rate, homeo
	}
	return tr, nil
}

// lifSynapticDriveParallel accumulates weights[e]*r.at(e) into drive[target]
// on the owning worker. Each target's addition order stays the declared edge
// order, so results are bit-identical to the single threaded pass.
func (m *LIF) lifSynapticDriveParallel(drive, weights []float64, r reader) error {
	return m.partition.Run(func(target int, edges []int) error {
		for _, e := range edges {
			drive[target] += weights[e] * r.at(e)
		}
		return nil
	})
}

// event returns the emitted spike and the declared derivative of that spike
// with respect to u = v_cand - theta_effective. The product model emits a hard
// 0/1 event and reports the declared surrogate psi(u) = 1/(1+scale*|u|)^2. The
// test-only smooth mode emits sigma(u) = 0.5*(1+scale*u/(1+scale*|u|)) and
// reports that function's exact derivative 0.5*scale*psi(u), which coincides
// with psi at scale = 2.
func (m *LIF) event(u float64) (spike, derivative float64) {
	scale := m.config.Surrogate.Scale
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

// Outputs returns the synaptic trace after each input step, excluding
// prehistory. This is the value edges and readouts observe.
// A nil or zero-value LIFTrace returns a nil slice.
func (tr *LIFTrace) Outputs() [][]float64 {
	if tr == nil || len(tr.syn) <= 1 {
		return nil
	}
	return cloneRows(tr.syn[1:])
}

// Spikes returns the 0/1 events after each input step, excluding prehistory.
// A nil or zero-value LIFTrace returns a nil slice.
func (tr *LIFTrace) Spikes() [][]float64 {
	if tr == nil || len(tr.spike) <= 1 {
		return nil
	}
	return cloneRows(tr.spike[1:])
}

// Voltages returns the membrane voltage after each input step, after any reset.
// A nil or zero-value LIFTrace returns a nil slice.
func (tr *LIFTrace) Voltages() [][]float64 {
	if tr == nil || len(tr.voltage) <= 1 {
		return nil
	}
	return cloneRows(tr.voltage[1:])
}

// FinalVoltage returns the last membrane voltage, not a complete delayed
// snapshot. A nil or zero-value LIFTrace returns a nil slice.
func (tr *LIFTrace) FinalVoltage() []float64 {
	if tr == nil || len(tr.voltage) == 0 {
		return nil
	}
	return append([]float64(nil), tr.voltage[len(tr.voltage)-1]...)
}

// Backward applies the declared surrogate rules to every step. upstream[t] is
// the loss gradient of the synaptic trace after step t. Window zero propagates
// through the complete sequence; a positive window detaches the synaptic,
// adaptation and voltage state at fixed boundaries (0, window, 2*window, ...),
// including delayed edges, exactly like Continuous.Backward. The reset keeps
// spike constant, so d v(t+1)/d v_cand = 1-spike and a refractory step passes
// nothing to its drive, input or previous voltage. Trace and upstream are
// unchanged and no parameter is updated.
func (m *LIF) Backward(ctx context.Context, tr *LIFTrace, upstream [][]float64, window int) (LIFGradient, error) {
	var empty LIFGradient
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
	gx := make([][]float64, steps+1)
	for t := range gx {
		gx[t] = make([]float64, n)
	}
	for t, row := range upstream {
		if err := vector(row, n, "upstream"); err != nil {
			return empty, err
		}
		copy(gx[t+1], row)
	}
	g := LIFGradient{
		Weights: make([]float64, len(tr.parameters.Weights)), Bias: make([]float64, n),
		LogTau: make([]float64, n), ThetaRaw: make([]float64, n),
		Inputs: make([][]float64, steps), Initial: make([]float64, n),
	}
	adapting := m.config.Adaptation.Enabled
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
			// The event feeds the synaptic trace and the adaptation.
			dspike := gx[t+1][i]
			if adapting {
				dspike += ga[i] * m.config.Adaptation.Beta
			}
			if m.smooth {
				// The smooth reference mixes v_cand and v_reset continuously,
				// so its reset coefficient is a real dependency. The product
				// model keeps the declared detached reset instead.
				dspike += gv[i] * (m.config.VReset - tr.cand[t][i])
			}
			if keep {
				gx[t][i] += m.kappa * gx[t+1][i]
				if adapting {
					prevA[i] = m.rho * ga[i]
				}
			}
			du := dspike * tr.dspike[t][i]
			dcand := gv[i]*tr.reset[t][i] + du
			// theta_effective = theta_base + a(t) + h(t), so the threshold
			// gradient is -du on both the bounded transform and the adaptation
			// state. The slow stabiliser offset h is a declared constant of the
			// reverse pass, exactly like the adaptation inside one step: it
			// carries no gradient and opens no path back to the events that
			// raised it.
			if adapting && keep {
				prevA[i] -= du
			}
			g.ThetaRaw[i] -= du * tr.thetaSlope[i]
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
			g.Weights[e] += driveGrad * tr.syn[past][s]
			if past > start || start == 0 {
				gx[past][s] += driveGrad * tr.parameters.Weights[e]
			}
		}
		gv, ga = prevV, prevA
	}
	// The synaptic trace, adaptation and refractory counters start at zero and
	// are not caller supplied, so only the voltage carries an initial gradient.
	copy(g.Initial, gv)
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

func logistic(v float64) float64 {
	if v >= 0 {
		return 1 / (1 + math.Exp(-v))
	}
	e := math.Exp(v)
	return e / (1 + e)
}

func cloneLIFParameters(p LIFParameters) LIFParameters {
	return LIFParameters{
		append([]float64(nil), p.Weights...), append([]float64(nil), p.Bias...),
		append([]float64(nil), p.LogTau...), append([]float64(nil), p.ThetaRaw...),
	}
}
