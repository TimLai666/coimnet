package dynamics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
)

const (
	// ContinuousStateVersion identifies the scalar continuous state and solver.
	ContinuousStateVersion = "coimnet-continuous-state/v1"
	// MaxStateValues limits voltage, retained history and each Advance output
	// matrix separately. It is an element limit, not a process memory bound.
	MaxStateValues = 1 << 20
)

// State owns the minimal history needed to continue a scalar continuous model.
// History is chronological: outputs at max(0,Steps-maxDelay) through Steps.
// Before time zero, the initial output is held constant. ConfigHash binds the
// canonical Config JSON including edge order, delays, DT and activation.
// It contains no optimizer, parameters, backward tape or disabled mechanisms.
type State struct {
	SchemaVersion string      `json:"schema_version"`
	ConfigHash    string      `json:"config_hash"`
	Steps         uint64      `json:"steps"`
	Voltage       []float64   `json:"voltage"`
	History       [][]float64 `json:"history"`
}

// NewState starts a trajectory without allocating a delay-sized history.
// The initial voltage is copied. A nil or zero model returns an error.
func (m *Continuous) NewState(initial []float64) (State, error) {
	if err := m.stateModelValid(); err != nil {
		return State{}, err
	}
	if err := vector(initial, m.config.Nodes, "initial voltage"); err != nil {
		return State{}, err
	}
	hash, err := m.stateConfigHash()
	if err != nil {
		return State{}, err
	}
	output := make([]float64, len(initial))
	for i, v := range initial {
		output[i] = m.activate(v)
	}
	return State{ContinuousStateVersion, hash, 0, append([]float64(nil), initial...), [][]float64{output}}, nil
}

// ValidateState rejects incompatible topology, missing or malformed history,
// non-finite values and outputs inconsistent with the activation. It does not
// retain or modify the supplied buffers. Latest output allows four adjacent
// float64 values on either side for CPU math-kernel rounding differences.
// This method never recomputes or repairs the stored history.
func (m *Continuous) ValidateState(s State) error {
	if err := m.stateModelValid(); err != nil {
		return err
	}
	if s.SchemaVersion != ContinuousStateVersion {
		return fmt.Errorf("unsupported continuous state schema %q", s.SchemaVersion)
	}
	hash, err := m.stateConfigHash()
	if err != nil {
		return err
	}
	if s.ConfigHash != hash {
		return fmt.Errorf("continuous state configuration fingerprint mismatch")
	}
	n := m.config.Nodes
	if err := vector(s.Voltage, n, "state voltage"); err != nil {
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
		if err := vector(row, n, "state history row"); err != nil {
			return fmt.Errorf("history[%d]: %w", t, err)
		}
		for i, y := range row {
			if (m.config.Activation == "tanh" && (y < -1 || y > 1)) || (m.config.Activation == "softplus" && y < 0) {
				return fmt.Errorf("history[%d][%d] is outside activation range", t, i)
			}
		}
	}
	for i, v := range s.Voltage {
		if !activationMatches(s.History[count-1][i], m.activate(v)) {
			return fmt.Errorf("latest history[%d] does not match voltage activation", i)
		}
	}
	return nil
}

// activationMatches permits only bounded representation-level rounding.
func activationMatches(saved, expected float64) bool {
	lower, upper := expected, expected
	for range 4 {
		lower = math.Nextafter(lower, math.Inf(-1))
		upper = math.Nextafter(upper, math.Inf(1))
	}
	return saved >= lower && saved <= upper
}

// Advance evolves a nonempty input sequence and returns an owned continuation
// and all post-step outputs. Parameters can change between calls; historical
// outputs retain the values computed when they occurred. No gradients cross
// calls. Errors or cancellation return zero results and leave arguments intact.
// The caller must not mutate arguments concurrently with this call.
//
// It is AdvanceModulated without a modulation.
func (m *Continuous) Advance(ctx context.Context, p Parameters, s State, inputs [][]float64) (State, [][]float64, error) {
	return m.AdvanceModulated(ctx, p, s, inputs, nil)
}

// AdvanceModulated is Advance with a per-step, per-node modulation of the input
// current: the external input plus the delayed synaptic contribution of node i
// at step t becomes gain[t][i]*I + offset[t][i] before the bias is added and
// before the membrane update. The bias is a parameter of the neuron, not an
// input current, so it is outside the gain.
//
// A nil modulation is the unmodulated path, with no extra arithmetic at all,
// and a neutral one (gain 1, offset 0) produces bit-identical results. The
// continuous core has no threshold, so a Threshold array is accepted only while
// every entry is zero. A modulation whose shape does not match the call, or
// whose entries are not finite, is refused before any state is computed.
//
// The modulation lasts exactly this call. Nothing here writes into p.
func (m *Continuous) AdvanceModulated(ctx context.Context, p Parameters, s State, inputs [][]float64, mod *Modulation) (State, [][]float64, error) {
	if ctx == nil {
		return State{}, nil, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return State{}, nil, err
	}
	if err := m.ValidateState(s); err != nil {
		return State{}, nil, err
	}
	if len(inputs) == 0 {
		return State{}, nil, fmt.Errorf("empty sequence")
	}
	n := m.config.Nodes
	if len(inputs) > MaxStateValues/n {
		return State{}, nil, fmt.Errorf("continuous output exceeds %d values", MaxStateValues)
	}
	if uint64(len(inputs)) > math.MaxUint64-s.Steps {
		return State{}, nil, fmt.Errorf("continuous step counter overflow")
	}
	finalSteps := s.Steps + uint64(len(inputs))
	capacity, err := m.stateHistoryRows(finalSteps)
	if err != nil {
		return State{}, nil, err
	}
	if err = vector(p.Weights, len(m.config.Sources), "weights"); err != nil {
		return State{}, nil, err
	}
	if err = vector(p.Bias, n, "bias"); err != nil {
		return State{}, nil, err
	}
	if err = vector(p.LogTau, n, "log_tau"); err != nil {
		return State{}, nil, err
	}
	lambda, alpha := make([]float64, n), make([]float64, n)
	for i, raw := range p.LogTau {
		tau := math.Exp(raw)
		if !finite(tau) || tau <= 0 {
			return State{}, nil, fmt.Errorf("tau[%d] is not representable and positive", i)
		}
		lambda[i] = math.Exp(-m.config.DT / tau)
		alpha[i] = -math.Expm1(-m.config.DT / tau)
	}
	for t, row := range inputs {
		if err = ctx.Err(); err != nil {
			return State{}, nil, err
		}
		if err = vector(row, n, "input"); err != nil {
			return State{}, nil, fmt.Errorf("input[%d]: %w", t, err)
		}
	}
	if err = mod.validate(len(inputs), n, false); err != nil {
		return State{}, nil, err
	}
	driven := mod.drives()
	// Ring slots refer only to our copies or newly computed outputs. A slot is
	// replaced, never modified, so earlier returned outputs remain unchanged.
	ring := make([][]float64, capacity)
	for t, row := range s.History {
		ring[t] = append([]float64(nil), row...)
	}
	head, length := 0, len(s.History)
	voltage := append([]float64(nil), s.Voltage...)
	outputs := make([][]float64, len(inputs))
	for t, input := range inputs {
		if err = ctx.Err(); err != nil {
			return State{}, nil, err
		}
		step := s.Steps + uint64(t)
		first := step - uint64(length-1)
		drive := append([]float64(nil), input...)
		for e, source := range m.config.Sources {
			if e%4096 == 0 {
				if err = ctx.Err(); err != nil {
					return State{}, nil, err
				}
			}
			past := uint64(0)
			if uint64(m.config.Delays[e]) < step {
				past = step - uint64(m.config.Delays[e])
			}
			offset := int(past - first)
			drive[m.config.Targets[e]] += p.Weights[e] * ring[(head+offset)%capacity][source]
		}
		next, output := make([]float64, n), make([]float64, n)
		for i := range drive {
			if driven {
				drive[i] = modulatedDrive(drive[i], mod, t, i)
			}
			drive[i] += p.Bias[i]
			next[i] = lambda[i]*voltage[i] + alpha[i]*drive[i]
			output[i] = m.activate(next[i])
			if !finite(drive[i]) || !finite(next[i]) || !finite(output[i]) {
				return State{}, nil, fmt.Errorf("non-finite state at step %d neuron %d", step, i)
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
		return State{}, nil, err
	}
	return State{ContinuousStateVersion, s.ConfigHash, finalSteps, voltage, history}, outputs, nil
}

func (m *Continuous) stateModelValid() error {
	if m == nil || m.config.Nodes <= 0 {
		return fmt.Errorf("uninitialized continuous model")
	}
	if m.config.Nodes > MaxStateValues {
		return fmt.Errorf("state voltage exceeds %d values", MaxStateValues)
	}
	return nil
}
func (m *Continuous) stateHistoryRows(steps uint64) (int, error) {
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
		return 0, fmt.Errorf("continuous history exceeds %d values", MaxStateValues)
	}
	return int(age) + 1, nil
}
func (m *Continuous) stateConfigHash() (string, error) {
	data, err := json.Marshal(m.Config())
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
