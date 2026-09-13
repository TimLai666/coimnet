package signal

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// NeuronID is an opaque, lossless identifier from an external graph source.
// CoImNet does not interpret or normalize either component.
type NeuronID struct {
	Namespace  string `json:"namespace"`
	ExternalID string `json:"external_id"`
}

func (id NeuronID) validate() error {
	if strings.TrimSpace(id.Namespace) == "" {
		return fmt.Errorf("neuron namespace must not be empty")
	}
	if strings.TrimSpace(id.ExternalID) == "" {
		return fmt.Errorf("neuron external_id must not be empty")
	}
	return nil
}

func (id NeuronID) Validate() error { return id.validate() }

func (id NeuronID) MarshalJSON() ([]byte, error) {
	if err := id.validate(); err != nil {
		return nil, err
	}
	type neuronIDAlias NeuronID
	return json.Marshal(neuronIDAlias(id))
}

func (id *NeuronID) UnmarshalJSON(data []byte) error {
	type neuronIDAlias NeuronID
	var decoded neuronIDAlias
	if err := decodeStrictBytes(data, &decoded); err != nil {
		return err
	}
	value := NeuronID(decoded)
	if err := value.validate(); err != nil {
		return err
	}
	*id = value
	return nil
}

// MappingSpec describes a channel-to-neuron mapping independently of any
// connectome representation. Neurons receive indices in the supplied order.
type MappingSpec struct {
	SchemaVersion Version    `json:"schema_version"`
	Source        string     `json:"source"`
	InputShape    []int      `json:"input_shape"`
	Neurons       []NeuronID `json:"neurons"`
}

func (s MappingSpec) Validate() error {
	_, err := NewMapping(s)
	return err
}

// MappingEntry exposes the persisted continuous index assigned to one opaque
// neuron identifier.
type MappingEntry struct {
	Namespace  string `json:"namespace"`
	ExternalID string `json:"external_id"`
	Index      int    `json:"index"`
}

type Mapping struct {
	schemaVersion Version
	source        string
	inputShape    []int
	neurons       []NeuronID
}

func NewMapping(spec MappingSpec) (Mapping, error) {
	if err := validateSchemaVersion(spec.SchemaVersion); err != nil {
		return Mapping{}, err
	}
	if strings.TrimSpace(spec.Source) == "" {
		return Mapping{}, fmt.Errorf("mapping source must not be empty")
	}
	inputSize, err := shapeSize(spec.InputShape)
	if err != nil {
		return Mapping{}, fmt.Errorf("input_shape: %w", err)
	}
	if inputSize != len(spec.Neurons) {
		return Mapping{}, fmt.Errorf("input_shape describes %d neurons, got %d", inputSize, len(spec.Neurons))
	}
	neurons := append([]NeuronID(nil), spec.Neurons...)
	seen := make(map[NeuronID]struct{}, len(neurons))
	for i, id := range neurons {
		if err := id.validate(); err != nil {
			return Mapping{}, fmt.Errorf("neurons[%d]: %w", i, err)
		}
		if _, exists := seen[id]; exists {
			return Mapping{}, fmt.Errorf("duplicate neuron %q/%q", id.Namespace, id.ExternalID)
		}
		seen[id] = struct{}{}
	}
	return Mapping{schemaVersion: spec.SchemaVersion, source: spec.Source, inputShape: append([]int(nil), spec.InputShape...), neurons: neurons}, nil
}

func (m Mapping) Validate() error {
	_, err := NewMapping(m.spec())
	return err
}

func (m Mapping) spec() MappingSpec {
	return MappingSpec{SchemaVersion: m.schemaVersion, Source: m.source, InputShape: append([]int(nil), m.inputShape...), Neurons: append([]NeuronID(nil), m.neurons...)}
}

func (m Mapping) SchemaVersion() Version { return m.schemaVersion }
func (m Mapping) Source() string         { return m.source }
func (m Mapping) InputShape() []int      { return append([]int(nil), m.inputShape...) }
func (m Mapping) Neurons() []NeuronID    { return append([]NeuronID(nil), m.neurons...) }

func (m Mapping) Entries() []MappingEntry {
	entries := make([]MappingEntry, len(m.neurons))
	for i, id := range m.neurons {
		entries[i] = MappingEntry{Namespace: id.Namespace, ExternalID: id.ExternalID, Index: i}
	}
	return entries
}

func (m Mapping) IndexOf(id NeuronID) (int, error) {
	if err := id.validate(); err != nil {
		return 0, err
	}
	for i, candidate := range m.neurons {
		if candidate == id {
			return i, nil
		}
	}
	return 0, fmt.Errorf("unknown neuron %q/%q", id.Namespace, id.ExternalID)
}

func (m Mapping) NeuronAt(index int) (NeuronID, error) {
	if index < 0 || index >= len(m.neurons) {
		return NeuronID{}, fmt.Errorf("neuron index %d out of range [0,%d)", index, len(m.neurons))
	}
	return m.neurons[index], nil
}

func (m Mapping) ValidateShape(shape []int) error {
	if err := m.Validate(); err != nil {
		return err
	}
	size, err := shapeSize(shape)
	if err != nil {
		return err
	}
	if size != len(m.neurons) || len(shape) != len(m.inputShape) {
		return fmt.Errorf("shape %v does not match mapping shape %v", shape, m.inputShape)
	}
	for i := range shape {
		if shape[i] != m.inputShape[i] {
			return fmt.Errorf("shape %v does not match mapping shape %v", shape, m.inputShape)
		}
	}
	return nil
}

// ValidateAgainst checks only the external graph boundary. The graph itself
// is intentionally represented as opaque IDs so this package has no connectome
// dependency.
func (m Mapping) ValidateAgainst(available []NeuronID) error {
	if err := m.Validate(); err != nil {
		return err
	}
	seen := make(map[NeuronID]struct{}, len(available))
	for i, id := range available {
		if err := id.validate(); err != nil {
			return fmt.Errorf("available[%d]: %w", i, err)
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("duplicate available neuron %q/%q", id.Namespace, id.ExternalID)
		}
		seen[id] = struct{}{}
	}
	for i, id := range m.neurons {
		if _, exists := seen[id]; !exists {
			return fmt.Errorf("mapping neuron %d %q/%q is absent from graph", i, id.Namespace, id.ExternalID)
		}
	}
	return nil
}

type mappingJSON struct {
	SchemaVersion Version        `json:"schema_version"`
	Source        string         `json:"source"`
	InputShape    []int          `json:"input_shape"`
	Entries       []MappingEntry `json:"entries"`
}

func (m Mapping) MarshalJSON() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(mappingJSON{SchemaVersion: m.schemaVersion, Source: m.source, InputShape: append([]int(nil), m.inputShape...), Entries: m.Entries()})
}

func (m *Mapping) UnmarshalJSON(data []byte) error {
	var raw mappingJSON
	if err := decodeStrictBytes(data, &raw); err != nil {
		return err
	}
	if raw.Entries == nil {
		return fmt.Errorf("entries must be an array")
	}
	neurons := make([]NeuronID, len(raw.Entries))
	for i, entry := range raw.Entries {
		if entry.Index != i {
			return fmt.Errorf("entries[%d] has index %d; want contiguous index %d", i, entry.Index, i)
		}
		neurons[i] = NeuronID{Namespace: entry.Namespace, ExternalID: entry.ExternalID}
	}
	built, err := NewMapping(MappingSpec{SchemaVersion: raw.SchemaVersion, Source: raw.Source, InputShape: raw.InputShape, Neurons: neurons})
	if err != nil {
		return err
	}
	*m = built
	return nil
}

func DecodeMapping(data io.Reader) (Mapping, error) {
	var m Mapping
	if err := decodeStrict(data, &m); err != nil {
		return Mapping{}, err
	}
	return m, nil
}
