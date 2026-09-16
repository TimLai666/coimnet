// Package plasticity holds the local fast changes a running individual may
// apply to selected connections: a recent-participation record (the
// eligibility trace), a bounded fast change per enabled edge, and the two
// update rules the specification names.
//
// Everything here is a pure function over caller-owned slices, evaluated in one
// goroutine. Step never mutates its arguments; it returns a new state.
//
// # Update order inside one step
//
// The rules are evaluated in exactly one order, and both orders below are part
// of the contract because they change the numbers:
//
//  1. Each pair trace decays first and adds its own event afterwards, so a
//     neuron that fires in two consecutive steps holds decay*1 + 1, not
//     decay*(1+1).
//  2. The eligibility reads both pair traces before either trace has added the
//     event of this step. A pre and a post event in the same step therefore
//     pair with the residual traces only and never with each other.
//  3. The eligibility is updated before the gate is applied, so a gate arriving
//     later still acts on whatever eligibility has survived its own decay.
//
// The caller owns the surrounding order (core forward, then Step, then the next
// forward with the new effective weights); see learning.Individual.AdvanceGated.
//
// # Rules
//
// hebbian_rate, for edge e = (j -> i):
//
//	elig(t+1)    = decay_e * elig(t) + pre_j * post_i
//	plastic(t+1) = clamp(decay_p * plastic(t) + gate * elig(t+1), -plastic_max, +plastic_max)
//
// stdp_pair, which needs 0/1 events and therefore a spiking core:
//
//	pre_trace  = decay_pre  * pre_trace   (then + 1 on a pre event)
//	post_trace = decay_post * post_trace  (then + 1 on a post event)
//	elig(t+1)  = decay_e * elig(t) + a_plus * pre_trace   [on a post event]
//	                                - a_minus * post_trace [on a pre event]
//	plastic(t+1) as above.
//
// # Effective weights
//
// A free edge integrates w_base + plastic. A fixed-sign edge integrates
// s * max(|w_base| + plastic, w_min), so a fast change can shrink it to w_min
// but never past zero. Fast changes are never written back into the base
// parameters.
package plasticity

import (
	"fmt"
	"math"
)

// The two rule names the specification defines. hebbian_rate reads node
// activity values and runs on either core; stdp_pair reads 0/1 events and
// therefore needs a spiking core.
const (
	RuleHebbianRate = "hebbian_rate"
	RuleSTDPPair    = "stdp_pair"
)

// Rule is one complete update law. Kind selects which fields are read: every
// rule uses DecayE, DecayP, PlasticMax and WMin, and only stdp_pair uses
// DecayPre, DecayPost, APlus and AMinus. A rate rule that sets a pair field is
// rejected rather than silently ignoring it.
type Rule struct {
	Kind           string  `json:"kind"`
	DecayE         float64 `json:"decay_e"`
	DecayP         float64 `json:"decay_p"`
	PlasticMax     float64 `json:"plastic_max"`
	WMin           float64 `json:"w_min"`
	DecayPre       float64 `json:"decay_pre,omitempty"`
	DecayPost      float64 `json:"decay_post,omitempty"`
	APlus          float64 `json:"a_plus,omitempty"`
	AMinus         float64 `json:"a_minus,omitempty"`
	GateReceptor   *int    `json:"gate_receptor,omitempty"`
	GateScale      float64 `json:"gate_scale,omitempty"`
	DecayEReceptor *int    `json:"decay_e_receptor,omitempty"`
	DecayEBase     float64 `json:"decay_e_base,omitempty"`
	DecayESpan     float64 `json:"decay_e_span,omitempty"`
	DecayEMin      float64 `json:"decay_e_min,omitempty"`
	DecayEMax      float64 `json:"decay_e_max,omitempty"`
}

// Config declares one rule and the edges that follow it. Edges holds edge
// indices of the core, strictly increasing and below its edge count; every
// other edge keeps its base weight. It is not empty, because a model that
// changes nothing is a configuration error rather than a mechanism.
type Config struct {
	Rule  Rule  `json:"rule"`
	Edges []int `json:"edges"`
}

// State is the per-edge fast state, one entry per enabled edge in the order
// Config.Edges declares. PreTrace and PostTrace exist only under stdp_pair:
// under hebbian_rate they are absent rather than zero filled, because carrying
// them would be state the declared rule does not own.
type State struct {
	Eligibility []float64 `json:"eligibility"`
	Plastic     []float64 `json:"plastic"`
	PreTrace    []float64 `json:"pre_trace,omitempty"`
	PostTrace   []float64 `json:"post_trace,omitempty"`
}

// Report counts what one Step had to bound: entries the plastic_max clamp held
// this step, counted once per entry and per step.
type Report struct {
	Clamped int `json:"clamped"`
}

// ClampReport counts what one weight conversion had to bound: fixed-sign edges
// whose magnitude the w_min floor raised, counted once per edge and per call.
type ClampReport struct {
	HeldAtWMin int `json:"held_at_w_min"`
}

// Model is an immutable rule plus its owned edge selection.
type Model struct {
	rule  Rule
	edges []int
	total int
}

// New validates the whole declaration against a core with the given edge count
// and takes an independent copy of the edge selection.
func New(c Config, edges int) (*Model, error) {
	if edges < 0 {
		return nil, fmt.Errorf("the core declares %d edges", edges)
	}
	if err := validateRule(c.Rule); err != nil {
		return nil, err
	}
	if len(c.Edges) == 0 {
		return nil, fmt.Errorf("plasticity must enable at least one edge")
	}
	for k, e := range c.Edges {
		if e < 0 || e >= edges {
			return nil, fmt.Errorf("enabled edge %d is %d, outside [0, %d)", k, e, edges)
		}
		if k > 0 && e <= c.Edges[k-1] {
			return nil, fmt.Errorf("enabled edges must be strictly increasing: %d follows %d", e, c.Edges[k-1])
		}
	}
	return &Model{rule: c.Rule, edges: append([]int(nil), c.Edges...), total: edges}, nil
}

func validateRule(r Rule) error {
	switch r.Kind {
	case RuleHebbianRate, RuleSTDPPair:
	default:
		return fmt.Errorf("plasticity rule %q, want %q or %q", r.Kind, RuleHebbianRate, RuleSTDPPair)
	}
	for _, d := range []struct {
		name  string
		value float64
	}{{"decay_e", r.DecayE}, {"decay_p", r.DecayP}} {
		if !finite(d.value) || d.value < 0 || d.value >= 1 {
			return fmt.Errorf("%s is %g, want a value in [0, 1)", d.name, d.value)
		}
	}
	if !finite(r.PlasticMax) || r.PlasticMax <= 0 {
		return fmt.Errorf("plastic_max is %g, want a finite positive bound", r.PlasticMax)
	}
	if !finite(r.WMin) || r.WMin <= 0 {
		return fmt.Errorf("w_min is %g, want a finite positive floor", r.WMin)
	}
	if r.GateReceptor != nil {
		if *r.GateReceptor < 0 {
			return fmt.Errorf("gate_receptor is %d, want a value >= 0", *r.GateReceptor)
		}
		if !finite(r.GateScale) {
			return fmt.Errorf("gate_scale is %g, want a finite value", r.GateScale)
		}
	} else if r.GateScale != 0 {
		return fmt.Errorf("gate_scale is %g but rule %q declares no gate_receptor", r.GateScale, r.Kind)
	}
	if r.DecayEReceptor != nil {
		if *r.DecayEReceptor < 0 {
			return fmt.Errorf("decay_e_receptor is %d, want a value >= 0", *r.DecayEReceptor)
		}
		if !finite(r.DecayESpan) {
			return fmt.Errorf("decay_e_span is %g, want a finite value", r.DecayESpan)
		}
		if !(r.DecayEMin > 0 && r.DecayEMin <= r.DecayEBase && r.DecayEBase <= r.DecayEMax && r.DecayEMax < 1) {
			return fmt.Errorf("decay_e_min/base/max are %g/%g/%g, want 0 < min <= base <= max < 1", r.DecayEMin, r.DecayEBase, r.DecayEMax)
		}
	} else {
		for _, f := range []struct {
			name  string
			value float64
		}{{"decay_e_base", r.DecayEBase}, {"decay_e_span", r.DecayESpan}, {"decay_e_min", r.DecayEMin}, {"decay_e_max", r.DecayEMax}} {
			if f.value != 0 {
				return fmt.Errorf("%s is %g but rule %q declares no decay_e_receptor", f.name, f.value, r.Kind)
			}
		}
	}
	if r.Kind == RuleHebbianRate {
		for _, f := range []struct {
			name  string
			value float64
		}{{"decay_pre", r.DecayPre}, {"decay_post", r.DecayPost}, {"a_plus", r.APlus}, {"a_minus", r.AMinus}} {
			if f.value != 0 {
				return fmt.Errorf("%s is %g but rule %q never reads it", f.name, f.value, RuleHebbianRate)
			}
		}
		return nil
	}
	for _, d := range []struct {
		name  string
		value float64
	}{{"decay_pre", r.DecayPre}, {"decay_post", r.DecayPost}} {
		if !finite(d.value) || d.value < 0 || d.value >= 1 {
			return fmt.Errorf("%s is %g, want a value in [0, 1)", d.name, d.value)
		}
	}
	for _, a := range []struct {
		name  string
		value float64
	}{{"a_plus", r.APlus}, {"a_minus", r.AMinus}} {
		if !finite(a.value) || a.value < 0 {
			return fmt.Errorf("%s is %g, want a finite non-negative amplitude", a.name, a.value)
		}
	}
	return nil
}

// Config returns an independent copy of the declaration this model enforces.
func (m *Model) Config() Config {
	if m == nil {
		return Config{}
	}
	return Config{Rule: m.rule, Edges: append([]int(nil), m.edges...)}
}

// Edges reports how many edges carry fast changes.
func (m *Model) Edges() int {
	if m == nil {
		return 0
	}
	return len(m.edges)
}

// NewState starts every enabled edge at zero eligibility and zero fast change.
func (m *Model) NewState() State {
	if m == nil {
		return State{}
	}
	s := State{Eligibility: make([]float64, len(m.edges)), Plastic: make([]float64, len(m.edges))}
	if m.rule.Kind == RuleSTDPPair {
		s.PreTrace, s.PostTrace = make([]float64, len(m.edges)), make([]float64, len(m.edges))
	}
	return s
}

// ValidateState rejects a state that does not belong to this rule and edge
// selection: wrong lengths, the pair traces missing under stdp_pair or present
// under hebbian_rate, and non-finite values.
func (m *Model) ValidateState(s State) error {
	if m == nil {
		return fmt.Errorf("uninitialized plasticity model")
	}
	n := len(m.edges)
	for _, v := range []struct {
		name   string
		values []float64
	}{{"eligibility", s.Eligibility}, {"plastic", s.Plastic}} {
		if len(v.values) != n {
			return fmt.Errorf("plastic state %s has %d values, want %d", v.name, len(v.values), n)
		}
		for i, x := range v.values {
			if !finite(x) {
				return fmt.Errorf("plastic state %s[%d] is not finite", v.name, i)
			}
		}
	}
	for i, p := range s.Plastic {
		if math.Abs(p) > m.rule.PlasticMax {
			return fmt.Errorf("plastic state plastic[%d] is %g, outside the declared bound %g", i, p, m.rule.PlasticMax)
		}
	}
	if m.rule.Kind != RuleSTDPPair {
		if s.PreTrace != nil || s.PostTrace != nil {
			return fmt.Errorf("plastic state carries pair traces while rule %q never reads them", m.rule.Kind)
		}
		return nil
	}
	for _, v := range []struct {
		name   string
		values []float64
	}{{"pre_trace", s.PreTrace}, {"post_trace", s.PostTrace}} {
		if len(v.values) != n {
			return fmt.Errorf("plastic state %s has %d values, want %d", v.name, len(v.values), n)
		}
		for i, x := range v.values {
			if !finite(x) || x < 0 {
				return fmt.Errorf("plastic state %s[%d] is %g, want a finite non-negative trace", v.name, i, x)
			}
		}
	}
	return nil
}

// StepInput carries one step's signals. DecayE, when non-nil, overrides the
// rule's DecayE for this step only (a receptor-driven eligibility window);
// it must lie in [0, 1).
type StepInput struct {
	Pre, Post, SpikesPre, SpikesPost []float64
	Gate                             float64
	DecayE                           *float64
}

// StepWith advances the fast state by one core step. It is Step with the
// rule's DecayE optionally replaced for this step by in.DecayE, which must lie
// in [0, 1) when present. pre and post are node-level activity values,
// spikesPre and spikesPost node-level 0/1 events (nil on a core that produces
// none, which stdp_pair therefore refuses). sources and targets are the full
// edge arrays of the core, so an enabled edge index reads its own endpoints.
// The returned state is new; the arguments are untouched.
func (m *Model) StepWith(s State, in StepInput, sources, targets []int) (State, Report, error) {
	var report Report
	if m == nil {
		return State{}, report, fmt.Errorf("uninitialized plasticity model")
	}
	if err := m.ValidateState(s); err != nil {
		return State{}, report, err
	}
	if !finite(in.Gate) {
		return State{}, report, fmt.Errorf("gate is not finite")
	}
	decayE := m.rule.DecayE
	if in.DecayE != nil {
		if !finite(*in.DecayE) || *in.DecayE < 0 || *in.DecayE >= 1 {
			return State{}, report, fmt.Errorf("decay_e override is %g, want a value in [0, 1)", *in.DecayE)
		}
		decayE = *in.DecayE
	}
	if len(sources) != m.total || len(targets) != m.total {
		return State{}, report, fmt.Errorf("topology has %d sources and %d targets, the model declares %d edges", len(sources), len(targets), m.total)
	}
	pair := m.rule.Kind == RuleSTDPPair
	nodes, err := stepSignals(in.Pre, in.Post, in.SpikesPre, in.SpikesPost, pair)
	if err != nil {
		return State{}, report, err
	}
	for e := range m.total {
		if sources[e] < 0 || sources[e] >= nodes || targets[e] < 0 || targets[e] >= nodes {
			return State{}, report, fmt.Errorf("edge %d connects nodes %d and %d, outside [0, %d)", e, sources[e], targets[e], nodes)
		}
	}
	out := State{Eligibility: make([]float64, len(m.edges)), Plastic: make([]float64, len(m.edges))}
	if pair {
		out.PreTrace, out.PostTrace = make([]float64, len(m.edges)), make([]float64, len(m.edges))
	}
	for k, e := range m.edges {
		j, i := sources[e], targets[e]
		elig := decayE * s.Eligibility[k]
		if pair {
			// Decay first, read the decayed traces, add this step's own events
			// last: the two orders the package documentation pins.
			preTrace := m.rule.DecayPre * s.PreTrace[k]
			postTrace := m.rule.DecayPost * s.PostTrace[k]
			if in.SpikesPost[i] != 0 {
				elig += m.rule.APlus * preTrace
			}
			if in.SpikesPre[j] != 0 {
				elig -= m.rule.AMinus * postTrace
			}
			if in.SpikesPre[j] != 0 {
				preTrace++
			}
			if in.SpikesPost[i] != 0 {
				postTrace++
			}
			if !finite(preTrace) || !finite(postTrace) {
				return State{}, Report{}, fmt.Errorf("edge %d produced a non-finite pair trace", e)
			}
			out.PreTrace[k], out.PostTrace[k] = preTrace, postTrace
		} else {
			elig += in.Pre[j] * in.Post[i]
		}
		plastic := m.rule.DecayP*s.Plastic[k] + in.Gate*elig
		if !finite(elig) || !finite(plastic) {
			return State{}, Report{}, fmt.Errorf("edge %d produced a non-finite fast change", e)
		}
		if plastic > m.rule.PlasticMax {
			plastic, report.Clamped = m.rule.PlasticMax, report.Clamped+1
		} else if plastic < -m.rule.PlasticMax {
			plastic, report.Clamped = -m.rule.PlasticMax, report.Clamped+1
		}
		out.Eligibility[k], out.Plastic[k] = elig, plastic
	}
	return out, report, nil
}

// Step advances the fast state by one core step and is exactly StepWith with
// no DecayE override. pre and post are node-level activity values, spikesPre
// and spikesPost node-level 0/1 events (nil on a core that produces none,
// which stdp_pair therefore refuses). sources and targets are the full edge
// arrays of the core, so an enabled edge index reads its own endpoints. The
// returned state is new; the arguments are untouched.
func (m *Model) Step(s State, pre, post, spikesPre, spikesPost []float64, gate float64, sources, targets []int) (State, Report, error) {
	return m.StepWith(s, StepInput{Pre: pre, Post: post, SpikesPre: spikesPre, SpikesPost: spikesPost, Gate: gate}, sources, targets)
}

// stepSignals checks the node-level arguments of one step and returns the node
// count they agree on.
func stepSignals(pre, post, spikesPre, spikesPost []float64, pair bool) (int, error) {
	if pair {
		if spikesPre == nil || spikesPost == nil {
			return 0, fmt.Errorf("rule %q needs 0/1 events from a spiking core", RuleSTDPPair)
		}
	}
	nodes := len(spikesPre)
	if !pair {
		nodes = len(pre)
	}
	if nodes == 0 {
		return 0, fmt.Errorf("a step needs node-level signals")
	}
	for _, v := range []struct {
		name   string
		values []float64
		needed bool
	}{
		{"pre", pre, !pair}, {"post", post, !pair},
		{"spikes_pre", spikesPre, pair}, {"spikes_post", spikesPost, pair},
	} {
		if !v.needed {
			continue
		}
		if len(v.values) != nodes {
			return 0, fmt.Errorf("%s has %d values, want %d", v.name, len(v.values), nodes)
		}
		for i, x := range v.values {
			if !finite(x) {
				return 0, fmt.Errorf("%s[%d] is not finite", v.name, i)
			}
			if v.name == "spikes_pre" || v.name == "spikes_post" {
				if x != 0 && x != 1 {
					return 0, fmt.Errorf("%s[%d] is %g, want a 0/1 event", v.name, i, x)
				}
			}
		}
	}
	return nodes, nil
}

// WindowFor returns clamp(DecayEBase + DecayESpan*occupancy, DecayEMin,
// DecayEMax) or false when the rule declares no DecayEReceptor. A non-finite
// occupancy is treated as 0.
func (m *Model) WindowFor(occupancy float64) (float64, bool) {
	if m == nil || m.rule.DecayEReceptor == nil {
		return 0, false
	}
	if !finite(occupancy) {
		occupancy = 0
	}
	window := m.rule.DecayEBase + m.rule.DecayESpan*occupancy
	if window < m.rule.DecayEMin {
		window = m.rule.DecayEMin
	}
	if window > m.rule.DecayEMax {
		window = m.rule.DecayEMax
	}
	return window, true
}

// GateFor returns GateScale*occupancy or false when the rule declares no
// GateReceptor. A non-finite occupancy is treated as 0.
func (m *Model) GateFor(occupancy float64) (float64, bool) {
	if m == nil || m.rule.GateReceptor == nil {
		return 0, false
	}
	if !finite(occupancy) {
		occupancy = 0
	}
	return m.rule.GateScale * occupancy, true
}

// Effective returns the weights the core integrates, given the effective base
// weights it would integrate without plasticity (already through the
// log-magnitude parametrization of a fixed-sign edge) and the declared signs.
// signs may be nil or empty, which means every edge is free. The returned
// slice is owned by the caller and base is never modified.
func (m *Model) Effective(base []float64, signs []int8, s State) ([]float64, ClampReport, error) {
	var report ClampReport
	if m == nil {
		return nil, report, fmt.Errorf("uninitialized plasticity model")
	}
	if err := m.ValidateState(s); err != nil {
		return nil, report, err
	}
	if len(base) != m.total {
		return nil, report, fmt.Errorf("base weights have %d values, the model declares %d edges", len(base), m.total)
	}
	if len(signs) != 0 && len(signs) != m.total {
		return nil, report, fmt.Errorf("edge signs have %d values, the model declares %d edges", len(signs), m.total)
	}
	for i, sign := range signs {
		if sign < -1 || sign > 1 {
			return nil, report, fmt.Errorf("edge sign %d is %d, want -1, 0 or +1", i, sign)
		}
	}
	for i, w := range base {
		if !finite(w) {
			return nil, report, fmt.Errorf("base weight %d is not finite", i)
		}
	}
	out := append([]float64(nil), base...)
	for k, e := range m.edges {
		sign := int8(0)
		if len(signs) != 0 {
			sign = signs[e]
		}
		if sign == 0 {
			out[e] = base[e] + s.Plastic[k]
		} else {
			magnitude := math.Abs(base[e]) + s.Plastic[k]
			if magnitude < m.rule.WMin {
				magnitude, report.HeldAtWMin = m.rule.WMin, report.HeldAtWMin+1
			}
			out[e] = float64(sign) * magnitude
		}
		if !finite(out[e]) {
			return nil, ClampReport{}, fmt.Errorf("edge %d produced a non-finite effective weight", e)
		}
	}
	return out, report, nil
}

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
