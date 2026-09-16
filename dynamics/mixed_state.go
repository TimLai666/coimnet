package dynamics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
)

// MixedStateVersion identifies the persistent state of a core whose nodes
// follow two rules under one clock. It is independent of the two single-rule
// schemas, and a document written under it is not one of them: the two halves
// carry only their own nodes and the whole document is bound to the mixed
// configuration fingerprint.
const MixedStateVersion = "coimnet-mixed-state/v1"

// MixedIndex is the node map of a mixed state. ContinuousNodes and LIFNodes are
// the ascending node indices each half covers, so row k of a half belongs to
// node ContinuousNodes[k] or LIFNodes[k]. Together they are a partition of
// [0, Nodes).
type MixedIndex struct {
	ContinuousNodes []int `json:"continuous_nodes"`
	LIFNodes        []int `json:"lif_nodes"`
}

// MixedState owns the minimal history needed to continue a mixed model. Steps
// is the one clock both halves advance on, and each half is the state its own
// rule owns, restricted to the nodes that follow that rule and kept in
// ascending node order. Both histories cover the same rows, from
// max(0, Steps-maxDelay) through Steps, because the delays are one shared edge
// property. ConfigHash binds the canonical MixedConfig JSON including edge
// order, delays, DT, the rule assignment and both rule blocks; the halves
// repeat it, so neither half can be detached and read as a single-rule state by
// accident. It contains no optimizer, parameters or backward tape.
type MixedState struct {
	SchemaVersion string     `json:"schema_version"`
	ConfigHash    string     `json:"config_hash"`
	Steps         uint64     `json:"steps"`
	Continuous    State      `json:"continuous"`
	LIF           LIFState   `json:"lif"`
	Index         MixedIndex `json:"index"`
}

// NewState starts a trajectory without allocating a delay-sized history. The
// initial voltage is copied and split between the halves; a LIF node's synaptic
// trace, adaptation and refractory counter start at zero exactly as Forward
// starts them. A nil or zero model returns an error.
func (m *Mixed) NewState(initial []float64) (MixedState, error) {
	if err := m.mixedStateModelValid(); err != nil {
		return MixedState{}, err
	}
	if err := vector(initial, m.config.Nodes, "initial voltage"); err != nil {
		return MixedState{}, err
	}
	hash, err := m.mixedStateConfigHash()
	if err != nil {
		return MixedState{}, err
	}
	continuous := State{
		SchemaVersion: ContinuousStateVersion, ConfigHash: hash, Steps: 0,
		Voltage: make([]float64, len(m.continuousNodes)),
		History: [][]float64{make([]float64, len(m.continuousNodes))},
	}
	for k, i := range m.continuousNodes {
		continuous.Voltage[k] = initial[i]
		continuous.History[0][k] = m.act.activate(initial[i])
	}
	spiking := LIFState{
		SchemaVersion: LIFStateVersion, ConfigHash: hash, Steps: 0,
		Voltage:    make([]float64, len(m.lifNodes)),
		History:    [][]float64{make([]float64, len(m.lifNodes))},
		Adaptation: make([]float64, len(m.lifNodes)),
		Refractory: make([]int, len(m.lifNodes)),
	}
	for k, i := range m.lifNodes {
		spiking.Voltage[k] = initial[i]
	}
	if m.stabilising() {
		spiking.Rate, spiking.Homeostasis = make([]float64, len(m.lifNodes)), make([]float64, len(m.lifNodes))
	}
	return MixedState{
		SchemaVersion: MixedStateVersion, ConfigHash: hash, Steps: 0,
		Continuous: continuous, LIF: spiking,
		Index: MixedIndex{ContinuousNodes: m.ContinuousNodes(), LIFNodes: m.LIFNodes()},
	}, nil
}

// ValidateState rejects an incompatible configuration, a node map that is not
// this model's own, halves that disagree with the shared clock, missing or
// malformed history, non-finite values, and every defect the two single-rule
// validators reject on their own nodes. It never recomputes or repairs the
// stored state, and it neither retains nor modifies the supplied buffers.
func (m *Mixed) ValidateState(s MixedState) error {
	if err := m.mixedStateModelValid(); err != nil {
		return err
	}
	if s.SchemaVersion != MixedStateVersion {
		return fmt.Errorf("unsupported mixed state schema %q", s.SchemaVersion)
	}
	hash, err := m.mixedStateConfigHash()
	if err != nil {
		return err
	}
	if s.ConfigHash != hash {
		return fmt.Errorf("mixed state configuration fingerprint mismatch")
	}
	if !sameInts(s.Index.ContinuousNodes, m.continuousNodes) {
		return fmt.Errorf("mixed state continuous node map does not match the rule assignment")
	}
	if !sameInts(s.Index.LIFNodes, m.lifNodes) {
		return fmt.Errorf("mixed state lif node map does not match the rule assignment")
	}
	for _, half := range []struct {
		name    string
		version string
		got     string
		steps   uint64
	}{
		{"continuous", ContinuousStateVersion, s.Continuous.SchemaVersion, s.Continuous.Steps},
		{"lif", LIFStateVersion, s.LIF.SchemaVersion, s.LIF.Steps},
	} {
		if half.got != half.version {
			return fmt.Errorf("mixed state %s half has schema %q, want %q", half.name, half.got, half.version)
		}
		if half.steps != s.Steps {
			return fmt.Errorf("mixed state %s half is at step %d, the shared clock is at %d", half.name, half.steps, s.Steps)
		}
	}
	if s.Continuous.ConfigHash != hash || s.LIF.ConfigHash != hash {
		return fmt.Errorf("mixed state half carries a different configuration fingerprint")
	}
	rows, err := m.mixedStateHistoryRows(s.Steps)
	if err != nil {
		return err
	}
	cont, spiking := len(m.continuousNodes), len(m.lifNodes)
	if err = vector(s.Continuous.Voltage, cont, "state continuous voltage"); err != nil {
		return err
	}
	if len(s.Continuous.History) != rows {
		return fmt.Errorf("continuous history has %d rows, want %d", len(s.Continuous.History), rows)
	}
	for t, row := range s.Continuous.History {
		if err = vector(row, cont, "state continuous history row"); err != nil {
			return fmt.Errorf("continuous history[%d]: %w", t, err)
		}
		for k, y := range row {
			if (m.config.Continuous.Activation == "tanh" && (y < -1 || y > 1)) ||
				(m.config.Continuous.Activation == "softplus" && y < 0) {
				return fmt.Errorf("continuous history[%d][%d] is outside activation range", t, k)
			}
		}
	}
	for k := range s.Continuous.Voltage {
		if !activationMatches(s.Continuous.History[rows-1][k], m.act.activate(s.Continuous.Voltage[k])) {
			return fmt.Errorf("latest continuous history[%d] does not match voltage activation", k)
		}
	}
	if err = vector(s.LIF.Voltage, spiking, "state lif voltage"); err != nil {
		return err
	}
	if len(s.LIF.History) != rows {
		return fmt.Errorf("lif history has %d rows, want %d", len(s.LIF.History), rows)
	}
	limit := math.Inf(1)
	if m.spike != nil {
		limit = lifTraceLimit(m.spike.kappa)
	}
	for t, row := range s.LIF.History {
		if err = vector(row, spiking, "state lif history row"); err != nil {
			return fmt.Errorf("lif history[%d]: %w", t, err)
		}
		for k, x := range row {
			if x < 0 {
				return fmt.Errorf("lif history[%d][%d] is a negative synaptic trace", t, k)
			}
			if x > limit {
				return fmt.Errorf("lif history[%d][%d] exceeds the decayed unit event sum", t, k)
			}
		}
	}
	if err = vector(s.LIF.Adaptation, spiking, "state lif adaptation"); err != nil {
		return err
	}
	for k, a := range s.LIF.Adaptation {
		if a < 0 {
			return fmt.Errorf("lif adaptation[%d] is negative", k)
		}
		if a != 0 && !m.adapting() {
			return fmt.Errorf("lif adaptation[%d] is nonzero while the mechanism is disabled", k)
		}
	}
	if len(s.LIF.Refractory) != spiking {
		return fmt.Errorf("state lif refractory length %d, want %d", len(s.LIF.Refractory), spiking)
	}
	for k, r := range s.LIF.Refractory {
		if r < 0 || r > m.config.LIF.RefractorySteps {
			return fmt.Errorf("lif refractory[%d] is %d, outside [0, %d]", k, r, m.config.LIF.RefractorySteps)
		}
	}
	if !m.stabilising() {
		// Absent rather than zero filled, exactly as on the spiking core.
		if s.LIF.Rate != nil {
			return fmt.Errorf("state lif rate is present while homeostasis is disabled")
		}
		if s.LIF.Homeostasis != nil {
			return fmt.Errorf("state lif homeostasis is present while the mechanism is disabled")
		}
		return nil
	}
	if err = vector(s.LIF.Rate, spiking, "state lif rate"); err != nil {
		return err
	}
	if err = vector(s.LIF.Homeostasis, spiking, "state lif homeostasis"); err != nil {
		return err
	}
	for k, h := range s.LIF.Homeostasis {
		if h < 0 || h > m.config.LIF.Homeostasis.HMax {
			return fmt.Errorf("lif homeostasis[%d] is %g, outside [0, %g]", k, h, m.config.LIF.Homeostasis.HMax)
		}
	}
	return nil
}

// Advance evolves a nonempty input sequence and returns an owned continuation,
// the output series of every node after each step and the 0/1 events after each
// step. A continuous node owns no event, so its entry of the event rows is
// always zero. Continuing a saved state reproduces a single Forward over the
// whole sequence exactly. Parameters can change between calls; historical
// outputs retain the values computed when they occurred and no gradients cross
// calls. Errors or cancellation return zero results and leave arguments intact.
// The caller must not mutate arguments concurrently with this call.
//
// It is AdvanceModulated without a modulation.
func (m *Mixed) Advance(ctx context.Context, p MixedParameters, s MixedState, inputs [][]float64) (MixedState, [][]float64, [][]float64, error) {
	return m.AdvanceModulated(ctx, p, s, inputs, nil)
}

// AdvanceModulated is Advance with the per-step, per-node modulation a chemical
// layer produced for exactly these steps. Gain and Offset change the input
// current of any node, of either rule, before the bias is added and before the
// membrane update; the bias is a parameter of the neuron, not an input current,
// so it stays outside the gain. Threshold moves the effective threshold of a
// LIF node, floored at theta_min, and a nonzero threshold entry on a continuous
// node is refused rather than ignored, because a continuous node has no
// threshold to move.
//
// A nil modulation is the unmodulated path, with no extra arithmetic at all,
// and a neutral one (gain 1, offset 0, threshold 0) produces bit-identical
// results. A modulation whose shape does not match the call, or whose entries
// are not finite, is refused before any state is computed. The modulation lasts
// exactly this call and is never written back into p.
func (m *Mixed) AdvanceModulated(ctx context.Context, p MixedParameters, s MixedState, inputs [][]float64, mod *Modulation) (MixedState, [][]float64, [][]float64, error) {
	if ctx == nil {
		return MixedState{}, nil, nil, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return MixedState{}, nil, nil, err
	}
	if err := m.ValidateState(s); err != nil {
		return MixedState{}, nil, nil, err
	}
	if m.smooth {
		// The smooth spike is a package-private finite difference reference, not
		// a product mode, and it disables the refractory mechanism it would have
		// to persist.
		return MixedState{}, nil, nil, fmt.Errorf("smooth reference mode has no persistent state")
	}
	if len(inputs) == 0 {
		return MixedState{}, nil, nil, fmt.Errorf("empty sequence")
	}
	n := m.config.Nodes
	if len(inputs) > MaxStateValues/n {
		return MixedState{}, nil, nil, fmt.Errorf("mixed output exceeds %d values", MaxStateValues)
	}
	if uint64(len(inputs)) > math.MaxUint64-s.Steps {
		return MixedState{}, nil, nil, fmt.Errorf("mixed step counter overflow")
	}
	finalSteps := s.Steps + uint64(len(inputs))
	capacity, err := m.mixedStateHistoryRows(finalSteps)
	if err != nil {
		return MixedState{}, nil, nil, err
	}
	if err = m.validateParameters(p); err != nil {
		return MixedState{}, nil, nil, err
	}
	lambda, alpha, err := m.leak(p.LogTau)
	if err != nil {
		return MixedState{}, nil, nil, err
	}
	base, _, err := m.baseThreshold(p.ThetaRaw, false)
	if err != nil {
		return MixedState{}, nil, nil, err
	}
	for t, row := range inputs {
		if err = ctx.Err(); err != nil {
			return MixedState{}, nil, nil, err
		}
		if err = vector(row, n, "input"); err != nil {
			return MixedState{}, nil, nil, fmt.Errorf("input[%d]: %w", t, err)
		}
	}
	if err = mod.validate(len(inputs), n, true); err != nil {
		return MixedState{}, nil, nil, err
	}
	if err = m.refuseContinuousThreshold(mod); err != nil {
		return MixedState{}, nil, nil, err
	}
	driven, shifted := mod.drives(), mod.shifts()
	// Ring slots refer only to our copies or newly computed outputs. A slot is
	// replaced, never modified, so earlier returned outputs remain unchanged.
	length := len(s.Continuous.History)
	ring := make([][]float64, capacity)
	for t := range length {
		row := make([]float64, n)
		for k, i := range m.continuousNodes {
			row[i] = s.Continuous.History[t][k]
		}
		for k, i := range m.lifNodes {
			row[i] = s.LIF.History[t][k]
		}
		ring[t] = row
	}
	head := 0
	voltage := make([]float64, n)
	adaptation, rate, homeostasis := make([]float64, n), make([]float64, n), make([]float64, n)
	refractory := make([]int, n)
	for k, i := range m.continuousNodes {
		voltage[i] = s.Continuous.Voltage[k]
	}
	for k, i := range m.lifNodes {
		voltage[i] = s.LIF.Voltage[k]
		adaptation[i] = s.LIF.Adaptation[k]
		refractory[i] = s.LIF.Refractory[k]
		if len(s.LIF.Rate) == len(m.lifNodes) {
			rate[i], homeostasis[i] = s.LIF.Rate[k], s.LIF.Homeostasis[k]
		}
	}
	adapting, stabilising := m.adapting(), m.stabilising()
	outputs := make([][]float64, len(inputs))
	spikes := make([][]float64, len(inputs))
	for t, input := range inputs {
		if err = ctx.Err(); err != nil {
			return MixedState{}, nil, nil, err
		}
		step := s.Steps + uint64(t)
		first := step - uint64(length-1)
		drive := append([]float64(nil), input...)
		for e, source := range m.config.Sources {
			if e%4096 == 0 {
				if err = ctx.Err(); err != nil {
					return MixedState{}, nil, nil, err
				}
			}
			past := uint64(0)
			if uint64(m.config.Delays[e]) < step {
				past = step - uint64(m.config.Delays[e])
			}
			offset := int(past - first)
			drive[m.config.Targets[e]] += p.Weights[e] * ring[(head+offset)%capacity][source]
		}
		trace := ring[(head+length-1)%capacity]
		nextV, out := make([]float64, n), make([]float64, n)
		spike, nextA := make([]float64, n), make([]float64, n)
		nextRate, nextH := make([]float64, n), make([]float64, n)
		nextR := make([]int, n)
		for i := range drive {
			if driven {
				drive[i] = modulatedDrive(drive[i], mod, t, i)
			}
			drive[i] += p.Bias[i]
			// Both rules leak the membrane with the same coefficients, so the
			// candidate is computed once for either of them.
			cand := membrane(lambda[i], voltage[i], alpha[i], drive[i])
			if !m.spiking(i) {
				nextV[i] = cand
				out[i] = m.act.activate(nextV[i])
				if !finite(drive[i]) || !finite(nextV[i]) || !finite(out[i]) {
					return MixedState{}, nil, nil, fmt.Errorf("non-finite state at step %d neuron %d", step, i)
				}
				continue
			}
			k := m.position[i]
			theta := base[k]
			if adapting {
				theta += adaptation[i]
			}
			if stabilising {
				theta += homeostasis[i]
			}
			if shifted {
				theta = modulatedThreshold(theta, mod, t, i, m.config.LIF.ThetaMin)
			}
			if refractory[i] > 0 {
				// Hold and ignore: the drive of this step never reaches the
				// membrane and emits no event.
				nextV[i] = m.config.LIF.VReset
				nextR[i] = refractory[i] - 1
			} else {
				if cand-theta >= 0 {
					spike[i] = 1
				}
				nextV[i] = (1-spike[i])*cand + spike[i]*m.config.LIF.VReset
				if spike[i] != 0 {
					nextR[i] = m.config.LIF.RefractorySteps
				}
			}
			out[i] = m.spike.kappa*trace[i] + spike[i]
			if adapting {
				nextA[i] = m.spike.rho*adaptation[i] + m.config.LIF.Adaptation.Beta*spike[i]
			}
			if stabilising {
				nextRate[i], nextH[i] = m.spike.stabilise(rate[i], homeostasis[i], spike[i])
				if !finite(nextRate[i]) || !finite(nextH[i]) {
					return MixedState{}, nil, nil, fmt.Errorf("non-finite state at step %d neuron %d", step, i)
				}
			}
			if !finite(drive[i]) || !finite(cand) || !finite(nextV[i]) || !finite(out[i]) || !finite(nextA[i]) {
				return MixedState{}, nil, nil, fmt.Errorf("non-finite state at step %d neuron %d", step, i)
			}
		}
		outputs[t], spikes[t] = out, spike
		voltage, adaptation, refractory = nextV, nextA, nextR
		rate, homeostasis = nextRate, nextH
		if length < capacity {
			ring[(head+length)%capacity] = out
			length++
		} else {
			ring[head] = out
			head = (head + 1) % capacity
		}
	}
	if err = ctx.Err(); err != nil {
		return MixedState{}, nil, nil, err
	}
	return m.splitState(s.ConfigHash, finalSteps, ring, capacity, head, length, voltage, adaptation, refractory, rate, homeostasis), outputs, spikes, nil
}

// splitState turns the run's node-wide arrays back into the two halves. It runs
// only after every step has succeeded, so nothing here can publish a partial
// advance.
func (m *Mixed) splitState(hash string, steps uint64, ring [][]float64, capacity, head, length int, voltage, adaptation []float64, refractory []int, rate, homeostasis []float64) MixedState {
	cont, spiking := len(m.continuousNodes), len(m.lifNodes)
	continuous := State{
		SchemaVersion: ContinuousStateVersion, ConfigHash: hash, Steps: steps,
		Voltage: make([]float64, cont), History: make([][]float64, length),
	}
	lif := LIFState{
		SchemaVersion: LIFStateVersion, ConfigHash: hash, Steps: steps,
		Voltage: make([]float64, spiking), History: make([][]float64, length),
		Adaptation: make([]float64, spiking), Refractory: make([]int, spiking),
	}
	for t := range length {
		row := ring[(head+t)%capacity]
		continuous.History[t] = make([]float64, cont)
		lif.History[t] = make([]float64, spiking)
		for k, i := range m.continuousNodes {
			continuous.History[t][k] = row[i]
		}
		for k, i := range m.lifNodes {
			lif.History[t][k] = row[i]
		}
	}
	for k, i := range m.continuousNodes {
		continuous.Voltage[k] = voltage[i]
	}
	stabilising := m.stabilising()
	if stabilising {
		lif.Rate, lif.Homeostasis = make([]float64, spiking), make([]float64, spiking)
	}
	for k, i := range m.lifNodes {
		lif.Voltage[k] = voltage[i]
		lif.Adaptation[k] = adaptation[i]
		lif.Refractory[k] = refractory[i]
		if stabilising {
			lif.Rate[k], lif.Homeostasis[k] = rate[i], homeostasis[i]
		}
	}
	return MixedState{
		SchemaVersion: MixedStateVersion, ConfigHash: hash, Steps: steps,
		Continuous: continuous, LIF: lif,
		Index: MixedIndex{ContinuousNodes: m.ContinuousNodes(), LIFNodes: m.LIFNodes()},
	}
}

// refuseContinuousThreshold rejects a threshold entry on a node that owns no
// threshold. The shape and finiteness are already checked by Modulation.validate.
func (m *Mixed) refuseContinuousThreshold(mod *Modulation) error {
	if mod == nil || mod.Threshold == nil {
		return nil
	}
	for t, row := range mod.Threshold {
		for i, v := range row {
			if v != 0 && !m.spiking(i) {
				return fmt.Errorf("modulation threshold[%d][%d] is %v, and continuous node %d has no threshold to move", t, i, v, i)
			}
		}
	}
	return nil
}

func (m *Mixed) mixedStateModelValid() error {
	if m == nil || m.config.Nodes <= 0 {
		return fmt.Errorf("uninitialized mixed model")
	}
	if m.config.Nodes > MaxStateValues {
		return fmt.Errorf("state voltage exceeds %d values", MaxStateValues)
	}
	return nil
}

// mixedStateHistoryRows counts the retained output rows. Both halves keep the
// same rows because the delays are a shared edge property, and the element
// limit is charged against the whole node width, not one half.
func (m *Mixed) mixedStateHistoryRows(steps uint64) (int, error) {
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
	maxRows := MaxStateValues / m.config.Nodes
	if age >= uint64(maxRows) {
		return 0, fmt.Errorf("mixed history exceeds %d values", MaxStateValues)
	}
	return int(age) + 1, nil
}

func (m *Mixed) mixedStateConfigHash() (string, error) {
	data, err := json.Marshal(m.Config())
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func sameInts(got, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
