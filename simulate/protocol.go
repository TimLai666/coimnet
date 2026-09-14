// Package simulate runs a stored connectome graph through one of the dynamics
// cores without any learning machinery: no trainable encoder, no readout, no
// optimizer and no Insyra tensors. Stimulus reaches the graph through fixed
// linear injections and activity leaves it through named probes, so a run is
// "this wiring plus these declared rules produced this", never a trained model.
//
// Every parameter of a run is an explicit assumption. In this ticket the only
// available parameter source is engineering_uniform_positive, which makes every
// edge excitatory with weight gain*raw_weight and gives every neuron the same
// bias, log_tau and theta_raw. That is an engineering placeholder, not a
// biological parameter set; deriving parameters from the release belongs to
// ticket 13.
package simulate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/internal/strictjson"
)

const (
	// ProtocolSchemaVersion identifies the run protocol JSON contract.
	ProtocolSchemaVersion = "coimnet-simulate-protocol/v1"
	// RunReportSchemaVersion identifies the RunReport JSON contract.
	RunReportSchemaVersion = "coimnet-simulate-run/v1"
	// StateSchemaVersion identifies the runner state snapshot union.
	StateSchemaVersion = "coimnet-simulate-state/v1"
	// MaxProtocolBytes bounds protocol JSON accepted by DecodeProtocol.
	MaxProtocolBytes int64 = 1 << 20
	// MaxStateBytes bounds state snapshot JSON accepted by DecodeState. A
	// whole-brain LIF snapshot is four arrays of one value per neuron.
	MaxStateBytes int64 = 64 << 20

	// CoreContinuous selects dynamics.Continuous; CoreLIF selects dynamics.LIF.
	CoreContinuous = "continuous"
	CoreLIF        = "lif"

	// ParameterSourceUniform is the only parameter source this ticket accepts.
	ParameterSourceUniform = "engineering_uniform_positive"

	// Probe reductions. The continuous core has no events, so it accepts only
	// the first two.
	ReduceMeanOutput    = "mean_output"
	ReduceSumOutput     = "sum_output"
	ReduceSpikeCount    = "spike_count"
	ReduceSpikeFraction = "spike_fraction"
)

// ErrCapacity wraps every accounted memory refusal. The runner never shrinks
// the graph, lowers the step count or drops probes to fit a limit.
var ErrCapacity = errors.New("simulate: capacity limit exceeded")

// Selector resolves a node set from the annotated node records of a graph.
// Field is one of class, type, superclass, subclass, instance or soma_side and
// a node matches when that field is present and equal to Equals. A selector
// that matches nothing is an error unless AllowEmpty is set, so a typo cannot
// quietly produce an empty probe.
type Selector struct {
	Field      string `json:"field"`
	Equals     string `json:"equals"`
	AllowEmpty bool   `json:"allow_empty,omitempty"`
}

// Injection adds Gain times the stimulus of one channel to the input of one
// node at every step. It is a fixed linear map, never a trainable matrix.
type Injection struct {
	Channel int     `json:"channel"`
	Node    int     `json:"node"`
	Gain    float64 `json:"gain"`
}

// InjectionGroup expands to one Injection per node the selector resolves, all
// with the same channel and gain.
type InjectionGroup struct {
	Channel  int       `json:"channel"`
	Selector *Selector `json:"selector"`
	Gain     float64   `json:"gain"`
}

// Probe reduces the activity of a node set to one value per step. Exactly one
// of Nodes and Selector declares the set.
type Probe struct {
	Name     string    `json:"name"`
	Nodes    []int     `json:"nodes,omitempty"`
	Selector *Selector `json:"selector,omitempty"`
	Reduce   string    `json:"reduce"`
}

// Pulse generates a Steps by Channels stimulus matrix that is zero everywhere
// except Amplitude on Channel during the steps [Onset, Onset+Duration).
type Pulse struct {
	Channels  int     `json:"channels"`
	Steps     int     `json:"steps"`
	Channel   int     `json:"channel"`
	Onset     int     `json:"onset"`
	Duration  int     `json:"duration"`
	Amplitude float64 `json:"amplitude"`
}

// StimulusSpec carries the stimulus as a literal matrix or as a pulse
// generator. Exactly one must be set.
type StimulusSpec struct {
	Inline [][]float64 `json:"inline,omitempty"`
	Pulse  *Pulse      `json:"pulse,omitempty"`
}

// Thresholds are optional activity bounds. Zero means unset. Exceeding one
// raises a stability flag in the report and changes nothing about the run.
type Thresholds struct {
	MaxPopulationRate float64 `json:"max_population_rate,omitempty"`
	MinActiveFraction float64 `json:"min_active_fraction,omitempty"`
}

// UniformParameters are the scalars of the engineering_uniform_positive source.
// Every edge weight becomes Gain times its raw source weight and every neuron
// receives the same Bias, LogTau and ThetaRaw.
type UniformParameters struct {
	Gain     float64 `json:"gain"`
	Bias     float64 `json:"bias"`
	LogTau   float64 `json:"log_tau"`
	ThetaRaw float64 `json:"theta_raw"`
}

// Protocol is the complete, serializable description of one run. The core
// configuration carries only the scalar model settings: nodes, sources, targets
// and delays come from the graph, so declaring them here is an error rather
// than a second source of truth.
type Protocol struct {
	SchemaVersion   string              `json:"schema_version"`
	Core            string              `json:"core"`
	Continuous      *dynamics.Config    `json:"continuous,omitempty"`
	LIF             *dynamics.LIFConfig `json:"lif,omitempty"`
	Injections      []Injection         `json:"injections,omitempty"`
	InjectionGroups []InjectionGroup    `json:"injection_groups,omitempty"`
	Probes          []Probe             `json:"probes"`
	Stimulus        StimulusSpec        `json:"stimulus"`
	Thresholds      Thresholds          `json:"thresholds"`
	ParameterSource string              `json:"parameter_source"`
	Uniform         *UniformParameters  `json:"uniform,omitempty"`
}

// DecodeProtocol reads exactly one strict JSON protocol and validates
// everything that does not depend on a graph.
func DecodeProtocol(r io.Reader) (Protocol, error) {
	var protocol Protocol
	if err := strictjson.Decode(r, MaxProtocolBytes, &protocol); err != nil {
		return Protocol{}, fmt.Errorf("simulate: invalid protocol: %w", err)
	}
	if err := protocol.Validate(); err != nil {
		return Protocol{}, err
	}
	return protocol, nil
}

// Hash returns the SHA-256 of the canonical protocol JSON as lowercase hex.
// Struct field order fixes the encoding, so identical protocols hash equally.
func (p Protocol) Hash() (string, error) {
	encoded, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("simulate: encode protocol: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// Validate checks everything that does not need a graph: schema, core
// selection, empty topology, parameter source, probes and the stimulus. Node
// indices, channels and selectors are checked against the graph in Build.
func (p Protocol) Validate() error {
	if p.SchemaVersion != ProtocolSchemaVersion {
		return fmt.Errorf("simulate: unsupported protocol schema %q, want %q", p.SchemaVersion, ProtocolSchemaVersion)
	}
	switch p.Core {
	case CoreLIF:
		if p.LIF == nil || p.Continuous != nil {
			return fmt.Errorf("simulate: core %q needs exactly the lif configuration", p.Core)
		}
		if err := emptyTopology(p.LIF.Nodes, p.LIF.Sources, p.LIF.Targets, p.LIF.Delays); err != nil {
			return err
		}
	case CoreContinuous:
		if p.Continuous == nil || p.LIF != nil {
			return fmt.Errorf("simulate: core %q needs exactly the continuous configuration", p.Core)
		}
		if err := emptyTopology(p.Continuous.Nodes, p.Continuous.Sources, p.Continuous.Targets, p.Continuous.Delays); err != nil {
			return err
		}
	default:
		return fmt.Errorf("simulate: unsupported core %q; choose %q or %q", p.Core, CoreContinuous, CoreLIF)
	}
	if p.ParameterSource != ParameterSourceUniform {
		return fmt.Errorf("simulate: parameter source %q is not available until ticket 13; this ticket only accepts %q", p.ParameterSource, ParameterSourceUniform)
	}
	if p.Uniform == nil {
		return fmt.Errorf("simulate: parameter source %q requires the uniform scalars", ParameterSourceUniform)
	}
	for name, value := range map[string]float64{
		"uniform.gain": p.Uniform.Gain, "uniform.bias": p.Uniform.Bias,
		"uniform.log_tau": p.Uniform.LogTau, "uniform.theta_raw": p.Uniform.ThetaRaw,
	} {
		if !finite(value) {
			return fmt.Errorf("simulate: %s is not finite", name)
		}
	}
	if len(p.Probes) == 0 {
		return errors.New("simulate: the protocol must declare at least one probe")
	}
	seen := map[string]struct{}{}
	for i, probe := range p.Probes {
		if probe.Name == "" {
			return fmt.Errorf("simulate: probe %d has no name", i)
		}
		if _, exists := seen[probe.Name]; exists {
			return fmt.Errorf("simulate: duplicate probe name %q", probe.Name)
		}
		seen[probe.Name] = struct{}{}
		switch probe.Reduce {
		case ReduceMeanOutput, ReduceSumOutput:
		case ReduceSpikeCount, ReduceSpikeFraction:
			if p.Core != CoreLIF {
				return fmt.Errorf("simulate: probe %q reduction %q needs a spiking core; the %q core emits no events", probe.Name, probe.Reduce, p.Core)
			}
		default:
			return fmt.Errorf("simulate: probe %q has unsupported reduction %q", probe.Name, probe.Reduce)
		}
		if (len(probe.Nodes) == 0) == (probe.Selector == nil) {
			return fmt.Errorf("simulate: probe %q must declare exactly one of nodes and selector", probe.Name)
		}
	}
	for i, group := range p.InjectionGroups {
		if group.Selector == nil {
			return fmt.Errorf("simulate: injection group %d has no selector", i)
		}
		if !finite(group.Gain) {
			return fmt.Errorf("simulate: injection group %d has a non-finite gain", i)
		}
	}
	for i, injection := range p.Injections {
		if !finite(injection.Gain) {
			return fmt.Errorf("simulate: injection %d has a non-finite gain", i)
		}
	}
	if len(p.Injections) == 0 && len(p.InjectionGroups) == 0 {
		return errors.New("simulate: the protocol must declare at least one injection or injection group")
	}
	if !finite(p.Thresholds.MaxPopulationRate) || p.Thresholds.MaxPopulationRate < 0 ||
		!finite(p.Thresholds.MinActiveFraction) || p.Thresholds.MinActiveFraction < 0 {
		return errors.New("simulate: thresholds must be finite and not negative")
	}
	_, err := p.Stimulus.matrix()
	return err
}

func emptyTopology(nodes int, sources, targets, delays []int) error {
	if nodes != 0 || len(sources) != 0 || len(targets) != 0 || len(delays) != 0 {
		return errors.New("simulate: the protocol core configuration must leave nodes, sources, targets and delays empty; the graph supplies the topology")
	}
	return nil
}

// matrix materializes the declared stimulus as steps by channels.
func (s StimulusSpec) matrix() ([][]float64, error) {
	if (s.Pulse == nil) == (len(s.Inline) == 0) {
		return nil, errors.New("simulate: the stimulus must declare exactly one of inline and pulse")
	}
	if s.Pulse != nil {
		p := *s.Pulse
		if p.Channels <= 0 || p.Steps <= 0 {
			return nil, errors.New("simulate: pulse channels and steps must be positive")
		}
		if p.Channel < 0 || p.Channel >= p.Channels {
			return nil, fmt.Errorf("simulate: pulse channel %d is outside [0,%d)", p.Channel, p.Channels)
		}
		if p.Onset < 0 || p.Duration < 0 {
			return nil, errors.New("simulate: pulse onset and duration must not be negative")
		}
		if p.Duration > p.Steps-p.Onset {
			return nil, fmt.Errorf("simulate: pulse [%d,%d) does not fit in %d steps", p.Onset, p.Onset+p.Duration, p.Steps)
		}
		if !finite(p.Amplitude) {
			return nil, errors.New("simulate: pulse amplitude is not finite")
		}
		if p.Steps > maxStimulusValues/p.Channels {
			return nil, fmt.Errorf("%w: pulse of %d steps by %d channels exceeds %d values", ErrCapacity, p.Steps, p.Channels, maxStimulusValues)
		}
		rows := make([][]float64, p.Steps)
		for t := range rows {
			rows[t] = make([]float64, p.Channels)
			if t >= p.Onset && t < p.Onset+p.Duration {
				rows[t][p.Channel] = p.Amplitude
			}
		}
		return rows, nil
	}
	channels := len(s.Inline[0])
	if channels <= 0 {
		return nil, errors.New("simulate: the inline stimulus must have at least one channel")
	}
	rows := make([][]float64, len(s.Inline))
	for t, row := range s.Inline {
		if len(row) != channels {
			return nil, fmt.Errorf("simulate: inline stimulus row %d has %d channels, want %d", t, len(row), channels)
		}
		for c, v := range row {
			if !finite(v) {
				return nil, fmt.Errorf("simulate: inline stimulus[%d][%d] is not finite", t, c)
			}
		}
		rows[t] = append([]float64(nil), row...)
	}
	return rows, nil
}

// maxStimulusValues bounds a generated stimulus matrix; it is an element limit,
// not a process memory bound.
const maxStimulusValues = 1 << 24

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
