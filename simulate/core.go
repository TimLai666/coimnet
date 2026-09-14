package simulate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/internal/strictjson"
)

// StateSnapshot is the JSON-serializable union of the two core states. Exactly
// one payload is set and Core says which. It carries no parameters, no
// optimizer and no probe history: it is only what the core needs to continue.
type StateSnapshot struct {
	SchemaVersion string             `json:"schema_version"`
	Core          string             `json:"core"`
	LIF           *dynamics.LIFState `json:"lif,omitempty"`
	Continuous    *dynamics.State    `json:"continuous,omitempty"`
}

// core hides the two dynamics models behind the few operations a run needs. It
// is deliberately private: the runner is core-agnostic, but this package does
// not publish a third model abstraction for callers to implement.
type core interface {
	// name returns CoreLIF or CoreContinuous.
	name() string
	// configHash fingerprints the canonical core configuration, including the
	// edge order the graph supplied.
	configHash() (string, error)
	// initial returns a fresh zero-voltage state.
	initial() (StateSnapshot, error)
	// validate rejects a snapshot that does not belong to this core.
	validate(StateSnapshot) error
	// advance evolves one bounded chunk and returns the continuation, the
	// per-step outputs and, for a spiking core, the per-step events.
	advance(ctx context.Context, p ParameterSet, s StateSnapshot, inputs [][]float64) (StateSnapshot, [][]float64, [][]float64, error)
	// maxStepsPerCall is the largest chunk advance accepts, set by the
	// element limit dynamics applies to one output matrix.
	maxStepsPerCall() int
	// spiking reports whether advance returns events.
	spiking() bool
	// baseline returns the output the graph presented before the next step.
	baseline(StateSnapshot) []float64
}

// lifCore adapts dynamics.LIF.
type lifCore struct {
	model *dynamics.LIF
	nodes int
}

func (c lifCore) name() string { return CoreLIF }

func (c lifCore) configHash() (string, error) { return configHash(c.model.Config()) }

func (c lifCore) initial() (StateSnapshot, error) {
	state, err := c.model.NewState(make([]float64, c.nodes))
	if err != nil {
		return StateSnapshot{}, fmt.Errorf("simulate: initial LIF state: %w", err)
	}
	return StateSnapshot{SchemaVersion: StateSchemaVersion, Core: CoreLIF, LIF: &state}, nil
}

func (c lifCore) validate(s StateSnapshot) error {
	if s.Core != CoreLIF || s.LIF == nil || s.Continuous != nil {
		return fmt.Errorf("simulate: state snapshot does not carry exactly one %s state", CoreLIF)
	}
	if err := c.model.ValidateState(*s.LIF); err != nil {
		return fmt.Errorf("simulate: %w", err)
	}
	return nil
}

func (c lifCore) advance(ctx context.Context, p ParameterSet, s StateSnapshot, inputs [][]float64) (StateSnapshot, [][]float64, [][]float64, error) {
	next, outputs, spikes, err := c.model.Advance(ctx, dynamics.LIFParameters{
		Weights: p.Weights, Bias: p.Bias, LogTau: p.LogTau, ThetaRaw: p.ThetaRaw,
	}, *s.LIF, inputs)
	if err != nil {
		return StateSnapshot{}, nil, nil, err
	}
	return StateSnapshot{SchemaVersion: StateSchemaVersion, Core: CoreLIF, LIF: &next}, outputs, spikes, nil
}

func (c lifCore) maxStepsPerCall() int { return maxSteps(c.nodes) }
func (c lifCore) spiking() bool        { return true }

func (c lifCore) baseline(s StateSnapshot) []float64 {
	return s.LIF.History[len(s.LIF.History)-1]
}

// continuousCore adapts dynamics.Continuous.
type continuousCore struct {
	model *dynamics.Continuous
	nodes int
}

func (c continuousCore) name() string { return CoreContinuous }

func (c continuousCore) configHash() (string, error) { return configHash(c.model.Config()) }

func (c continuousCore) initial() (StateSnapshot, error) {
	state, err := c.model.NewState(make([]float64, c.nodes))
	if err != nil {
		return StateSnapshot{}, fmt.Errorf("simulate: initial continuous state: %w", err)
	}
	return StateSnapshot{SchemaVersion: StateSchemaVersion, Core: CoreContinuous, Continuous: &state}, nil
}

func (c continuousCore) validate(s StateSnapshot) error {
	if s.Core != CoreContinuous || s.Continuous == nil || s.LIF != nil {
		return fmt.Errorf("simulate: state snapshot does not carry exactly one %s state", CoreContinuous)
	}
	if err := c.model.ValidateState(*s.Continuous); err != nil {
		return fmt.Errorf("simulate: %w", err)
	}
	return nil
}

func (c continuousCore) advance(ctx context.Context, p ParameterSet, s StateSnapshot, inputs [][]float64) (StateSnapshot, [][]float64, [][]float64, error) {
	next, outputs, err := c.model.Advance(ctx, dynamics.Parameters{
		Weights: p.Weights, Bias: p.Bias, LogTau: p.LogTau,
	}, *s.Continuous, inputs)
	if err != nil {
		return StateSnapshot{}, nil, nil, err
	}
	return StateSnapshot{SchemaVersion: StateSchemaVersion, Core: CoreContinuous, Continuous: &next}, outputs, nil, nil
}

func (c continuousCore) maxStepsPerCall() int { return maxSteps(c.nodes) }
func (c continuousCore) spiking() bool        { return false }

func (c continuousCore) baseline(s StateSnapshot) []float64 {
	return s.Continuous.History[len(s.Continuous.History)-1]
}

// maxSteps mirrors the element limit dynamics applies to one Advance output
// matrix. A whole-brain graph therefore advances in small chunks instead of
// being refused, and the chunk boundary changes no value.
func maxSteps(nodes int) int {
	steps := dynamics.MaxStateValues / nodes
	if steps < 1 {
		return 1
	}
	return steps
}

// DecodeState reads exactly one strict JSON state snapshot. It checks the
// union shape only; whether the state fits a particular core is decided by
// Runner.RestoreState.
func DecodeState(r io.Reader) (StateSnapshot, error) {
	var snapshot StateSnapshot
	if err := strictjson.Decode(r, MaxStateBytes, &snapshot); err != nil {
		return StateSnapshot{}, fmt.Errorf("simulate: invalid state snapshot: %w", err)
	}
	if snapshot.SchemaVersion != StateSchemaVersion {
		return StateSnapshot{}, fmt.Errorf("simulate: unsupported state schema %q, want %q", snapshot.SchemaVersion, StateSchemaVersion)
	}
	if (snapshot.LIF == nil) == (snapshot.Continuous == nil) {
		return StateSnapshot{}, errors.New("simulate: the state snapshot must carry exactly one core state")
	}
	return snapshot, nil
}

func configHash(config any) (string, error) {
	encoded, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("simulate: encode core configuration: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}
