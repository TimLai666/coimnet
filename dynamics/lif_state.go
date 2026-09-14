package dynamics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
)

// LIFStateVersion identifies the persistent state of the spiking core and its
// step rule. It is independent of the continuous schema.
const LIFStateVersion = "coimnet-lif-state/v1"

// LIFState owns the minimal history needed to continue a LIF model. History is
// the chronological synaptic trace x, from max(0,Steps-maxDelay) through Steps,
// and the row at time zero is the zero prehistory that Forward assumes. Voltage
// is the membrane after any reset, Adaptation stays at zero while the mechanism
// is disabled, and Refractory counts the steps each neuron still holds v_reset.
// ConfigHash binds the canonical LIFConfig JSON including edge order, delays,
// DT, the event parameters and the declared surrogate. It contains no
// optimizer, parameters, backward tape or disabled mechanisms.
type LIFState struct {
	SchemaVersion string      `json:"schema_version"`
	ConfigHash    string      `json:"config_hash"`
	Steps         uint64      `json:"steps"`
	Voltage       []float64   `json:"voltage"`
	History       [][]float64 `json:"history"`
	Adaptation    []float64   `json:"adaptation"`
	Refractory    []int       `json:"refractory"`
}

// NewState starts a trajectory without allocating a delay-sized history. The
// initial voltage is copied; the synaptic trace, the adaptation and the
// refractory counters start at zero exactly as Forward starts them. A nil or
// zero model returns an error.
func (m *LIF) NewState(initial []float64) (LIFState, error) {
	if err := m.lifStateModelValid(); err != nil {
		return LIFState{}, err
	}
	n := m.config.Nodes
	if err := vector(initial, n, "initial voltage"); err != nil {
		return LIFState{}, err
	}
	hash, err := m.lifStateConfigHash()
	if err != nil {
		return LIFState{}, err
	}
	return LIFState{
		SchemaVersion: LIFStateVersion,
		ConfigHash:    hash,
		Steps:         0,
		Voltage:       append([]float64(nil), initial...),
		History:       [][]float64{make([]float64, n)},
		Adaptation:    make([]float64, n),
		Refractory:    make([]int, n),
	}, nil
}

// ValidateState rejects an incompatible configuration, missing or malformed
// history, non-finite values, a synaptic trace outside the range a decayed sum
// of unit events can reach, an adaptation that contradicts the declared
// mechanism and refractory counters outside the declared length. The synaptic
// trace cannot be recomputed from the voltage, so the checks on it are
// structural. This method never recomputes or repairs the stored state, and it
// neither retains nor modifies the supplied buffers.
func (m *LIF) ValidateState(s LIFState) error {
	if err := m.lifStateModelValid(); err != nil {
		return err
	}
	if s.SchemaVersion != LIFStateVersion {
		return fmt.Errorf("unsupported LIF state schema %q", s.SchemaVersion)
	}
	hash, err := m.lifStateConfigHash()
	if err != nil {
		return err
	}
	if s.ConfigHash != hash {
		return fmt.Errorf("LIF state configuration fingerprint mismatch")
	}
	n := m.config.Nodes
	if err = vector(s.Voltage, n, "state voltage"); err != nil {
		return err
	}
	count, err := m.lifStateHistoryRows(s.Steps)
	if err != nil {
		return err
	}
	if len(s.History) != count {
		return fmt.Errorf("state history has %d rows, want %d", len(s.History), count)
	}
	limit := lifTraceLimit(m.kappa)
	for t, row := range s.History {
		if err = vector(row, n, "state history row"); err != nil {
			return fmt.Errorf("history[%d]: %w", t, err)
		}
		for i, x := range row {
			if x < 0 {
				return fmt.Errorf("history[%d][%d] is a negative synaptic trace", t, i)
			}
			if x > limit {
				return fmt.Errorf("history[%d][%d] exceeds the decayed unit event sum", t, i)
			}
		}
	}
	if err = vector(s.Adaptation, n, "state adaptation"); err != nil {
		return err
	}
	for i, a := range s.Adaptation {
		if a < 0 {
			return fmt.Errorf("adaptation[%d] is negative", i)
		}
		if a != 0 && !m.config.Adaptation.Enabled {
			return fmt.Errorf("adaptation[%d] is nonzero while the mechanism is disabled", i)
		}
	}
	if len(s.Refractory) != n {
		return fmt.Errorf("state refractory length %d, want %d", len(s.Refractory), n)
	}
	for i, r := range s.Refractory {
		if r < 0 || r > m.config.RefractorySteps {
			return fmt.Errorf("refractory[%d] is %d, outside [0, %d]", i, r, m.config.RefractorySteps)
		}
	}
	return nil
}

// Advance evolves a nonempty input sequence and returns an owned continuation,
// the synaptic trace after each step and the 0/1 events after each step. It
// applies the product hard event rule of Forward, so continuing a saved state
// reproduces a single Forward over the whole sequence exactly. Parameters can
// change between calls; historical traces retain the values computed when they
// occurred and no gradients cross calls. Errors or cancellation return zero
// results and leave arguments intact. The caller must not mutate arguments
// concurrently with this call.
func (m *LIF) Advance(ctx context.Context, p LIFParameters, s LIFState, inputs [][]float64) (LIFState, [][]float64, [][]float64, error) {
	if ctx == nil {
		return LIFState{}, nil, nil, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return LIFState{}, nil, nil, err
	}
	if err := m.ValidateState(s); err != nil {
		return LIFState{}, nil, nil, err
	}
	if m.smooth {
		// The smooth spike is a package-private finite difference reference, not
		// a product mode, and it disables the refractory mechanism it would have
		// to persist.
		return LIFState{}, nil, nil, fmt.Errorf("smooth reference mode has no persistent state")
	}
	if len(inputs) == 0 {
		return LIFState{}, nil, nil, fmt.Errorf("empty sequence")
	}
	n := m.config.Nodes
	if len(inputs) > MaxStateValues/n {
		return LIFState{}, nil, nil, fmt.Errorf("LIF output exceeds %d values", MaxStateValues)
	}
	if uint64(len(inputs)) > math.MaxUint64-s.Steps {
		return LIFState{}, nil, nil, fmt.Errorf("LIF step counter overflow")
	}
	finalSteps := s.Steps + uint64(len(inputs))
	capacity, err := m.lifStateHistoryRows(finalSteps)
	if err != nil {
		return LIFState{}, nil, nil, err
	}
	if err = vector(p.Weights, len(m.config.Sources), "weights"); err != nil {
		return LIFState{}, nil, nil, err
	}
	if err = vector(p.Bias, n, "bias"); err != nil {
		return LIFState{}, nil, nil, err
	}
	if err = vector(p.LogTau, n, "log_tau"); err != nil {
		return LIFState{}, nil, nil, err
	}
	if err = vector(p.ThetaRaw, n, "theta_raw"); err != nil {
		return LIFState{}, nil, nil, err
	}
	lambda, alpha, err := m.lifLeak(p.LogTau)
	if err != nil {
		return LIFState{}, nil, nil, err
	}
	base, err := m.lifBaseThreshold(p.ThetaRaw)
	if err != nil {
		return LIFState{}, nil, nil, err
	}
	for t, row := range inputs {
		if err = ctx.Err(); err != nil {
			return LIFState{}, nil, nil, err
		}
		if err = vector(row, n, "input"); err != nil {
			return LIFState{}, nil, nil, fmt.Errorf("input[%d]: %w", t, err)
		}
	}
	// Ring slots refer only to our copies or newly computed traces. A slot is
	// replaced, never modified, so earlier returned outputs remain unchanged.
	ring := make([][]float64, capacity)
	for t, row := range s.History {
		ring[t] = append([]float64(nil), row...)
	}
	head, length := 0, len(s.History)
	voltage := append([]float64(nil), s.Voltage...)
	adaptation := append([]float64(nil), s.Adaptation...)
	refractory := append([]int(nil), s.Refractory...)
	adapting := m.config.Adaptation.Enabled
	outputs := make([][]float64, len(inputs))
	spikes := make([][]float64, len(inputs))
	for t, input := range inputs {
		if err = ctx.Err(); err != nil {
			return LIFState{}, nil, nil, err
		}
		step := s.Steps + uint64(t)
		first := step - uint64(length-1)
		drive := append([]float64(nil), input...)
		for e, source := range m.config.Sources {
			if e%4096 == 0 {
				if err = ctx.Err(); err != nil {
					return LIFState{}, nil, nil, err
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
		nextV, spike := make([]float64, n), make([]float64, n)
		synapse, nextA := make([]float64, n), make([]float64, n)
		nextR := make([]int, n)
		for i := range drive {
			drive[i] += p.Bias[i]
			cand := lambda[i]*voltage[i] + alpha[i]*drive[i]
			theta := base[i]
			if adapting {
				theta += adaptation[i]
			}
			if refractory[i] > 0 {
				// Hold and ignore: the drive of this step never reaches the
				// membrane and emits no event.
				nextV[i] = m.config.VReset
				nextR[i] = refractory[i] - 1
			} else {
				if cand-theta >= 0 {
					spike[i] = 1
				}
				nextV[i] = (1-spike[i])*cand + spike[i]*m.config.VReset
				if spike[i] != 0 {
					nextR[i] = m.config.RefractorySteps
				}
			}
			synapse[i] = m.kappa*trace[i] + spike[i]
			if adapting {
				nextA[i] = m.rho*adaptation[i] + m.config.Adaptation.Beta*spike[i]
			}
			if !finite(drive[i]) || !finite(cand) || !finite(nextV[i]) || !finite(synapse[i]) || !finite(nextA[i]) {
				return LIFState{}, nil, nil, fmt.Errorf("non-finite state at step %d neuron %d", step, i)
			}
		}
		outputs[t], spikes[t] = synapse, spike
		voltage, adaptation, refractory = nextV, nextA, nextR
		if length < capacity {
			ring[(head+length)%capacity] = synapse
			length++
		} else {
			ring[head] = synapse
			head = (head + 1) % capacity
		}
	}
	history := make([][]float64, length)
	for t := range history {
		history[t] = append([]float64(nil), ring[(head+t)%capacity]...)
	}
	if err = ctx.Err(); err != nil {
		return LIFState{}, nil, nil, err
	}
	return LIFState{LIFStateVersion, s.ConfigHash, finalSteps, voltage, history, adaptation, refractory}, outputs, spikes, nil
}

// lifLeak repeats the Forward membrane coefficients. -Expm1 avoids cancellation
// in 1-exp(-dt/tau) for a small time step.
func (m *LIF) lifLeak(logTau []float64) ([]float64, []float64, error) {
	lambda, alpha := make([]float64, len(logTau)), make([]float64, len(logTau))
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

// lifBaseThreshold repeats the Forward bounded threshold transform. Only the
// value is needed here; the slope belongs to the reverse pass.
func (m *LIF) lifBaseThreshold(thetaRaw []float64) ([]float64, error) {
	span := m.config.ThetaMax - m.config.ThetaMin
	base := make([]float64, len(thetaRaw))
	for i, raw := range thetaRaw {
		base[i] = m.config.ThetaMin + span*logistic(raw)
		if !finite(base[i]) {
			return nil, fmt.Errorf("theta_base[%d] is not representable", i)
		}
	}
	return base, nil
}

// lifTraceLimit bounds a synaptic trace that started at the zero prehistory:
// x is a decayed sum of unit events, so it cannot pass the geometric limit
// 1/(1-kappa). The slack absorbs accumulated rounding, which leaves the check
// as a guard against grossly inconsistent traces rather than a tight bound.
// A kappa of one, reached when dt/tau_syn falls below float64 resolution,
// gives an infinite limit and accepts every finite trace.
func lifTraceLimit(kappa float64) float64 {
	bound := 1 / (1 - kappa)
	return bound + 1e-9*(1+bound)
}

func (m *LIF) lifStateModelValid() error {
	if m == nil || m.config.Nodes <= 0 {
		return fmt.Errorf("uninitialized LIF model")
	}
	if m.config.Nodes > MaxStateValues {
		return fmt.Errorf("state voltage exceeds %d values", MaxStateValues)
	}
	return nil
}

func (m *LIF) lifStateHistoryRows(steps uint64) (int, error) {
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
		return 0, fmt.Errorf("LIF history exceeds %d values", MaxStateValues)
	}
	return int(age) + 1, nil
}

func (m *LIF) lifStateConfigHash() (string, error) {
	data, err := json.Marshal(m.Config())
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
