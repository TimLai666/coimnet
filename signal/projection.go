package signal

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"
)

// ProjectionDirection identifies the neural side of a row-major projection.
type ProjectionDirection string

const (
	ProjectionInput  ProjectionDirection = "input"
	ProjectionOutput ProjectionDirection = "output"
	// Projection limits bound logical storage, not process RSS.
	MaxProjectionNeurons = 1 << 20
	MaxProjectionWeights = 1 << 20
)

type SelectionMode string

const (
	SelectExplicit SelectionMode = "explicit"
	SelectCellType SelectionMode = "cell_type"
	SelectRegion   SelectionMode = "region"
)

// NeuronCandidate describes a neuron at its actual index in the caller's graph.
// Empty CellType or Region means unknown; it never matches a selector.
type NeuronCandidate struct {
	ID       NeuronID
	CellType string
	Region   string
}

// NeuronSelection preserves explicit ID order, or selects an exact metadata
// value and sorts by opaque namespace/external ID strings.
type NeuronSelection struct {
	Mode    SelectionMode
	Value   string
	Neurons []NeuronID
}

// ProjectionRandom uses splitmix64-sign/v1: one signed Scale per matrix entry.
// It does not consult a global generator. Seed zero is valid.
type ProjectionRandom struct {
	Seed  uint64
	Scale float64
}

// ProjectionSpec describes a versioned numeric projection. Explicit Weights
// are row-major [channel,selected] for input and [selected,channel] for output.
// With Random, nil Weights generates coefficients; supplied weights must match
// the seed exactly. Artificial=false requires caller-provided Evidence.
type ProjectionSpec struct {
	SchemaVersion Version
	Direction     ProjectionDirection
	Source        string
	Artificial    bool
	Evidence      string
	ChannelShape  []int
	Selection     NeuronSelection
	Weights       []float64
	Random        *ProjectionRandom
}

// Projection owns its source metadata, resolved neuron order and coefficients.
// It is independent of graph indices; Bind resolves them against an actual graph.
type Projection struct {
	spec       ProjectionSpec
	neurons    []NeuronID
	neuronHash string
}

func NewProjection(spec ProjectionSpec, candidates []NeuronCandidate) (Projection, error) {
	ids, _, err := selectProjectionNeurons(spec.Selection, candidates)
	if err != nil {
		return Projection{}, err
	}
	count, err := projectionSize(spec, len(ids))
	if err != nil {
		return Projection{}, err
	}
	if spec.Random != nil && spec.Weights == nil {
		if err := validateProjectionRandom(spec.Random); err != nil {
			return Projection{}, err
		}
		spec.Weights = make([]float64, count)
		state := spec.Random.Seed
		for i := range spec.Weights {
			spec.Weights[i] = projectionRandomValue(&state, spec.Random.Scale)
		}
	}
	p := Projection{spec: spec, neurons: ids}
	if err := p.Validate(); err != nil {
		return Projection{}, err
	}
	p.spec = copyProjectionSpec(spec)
	p.neurons = append([]NeuronID(nil), ids...)
	p.neuronHash = projectionNeuronHash(ids)
	return p, nil
}

func projectionSize(spec ProjectionSpec, neurons int) (int, error) {
	if err := validateSchemaVersion(spec.SchemaVersion); err != nil {
		return 0, err
	}
	if spec.Direction != ProjectionInput && spec.Direction != ProjectionOutput {
		return 0, fmt.Errorf("unsupported projection direction %q", spec.Direction)
	}
	if strings.TrimSpace(spec.Source) == "" || !utf8.ValidString(spec.Source) || !utf8.ValidString(spec.Evidence) {
		return 0, fmt.Errorf("projection source/evidence must be valid UTF-8 and source must be nonempty")
	}
	if !spec.Artificial && strings.TrimSpace(spec.Evidence) == "" {
		return 0, fmt.Errorf("non-artificial projection requires evidence")
	}
	channels, err := shapeSize(spec.ChannelShape)
	if err != nil {
		return 0, err
	}
	if neurons <= 0 || neurons > MaxProjectionNeurons || channels > MaxProjectionWeights/neurons {
		return 0, fmt.Errorf("projection exceeds neuron/weight limits or has no selected neurons")
	}
	return channels * neurons, nil
}

func (p Projection) Validate() error {
	count, err := projectionSize(p.spec, len(p.neurons))
	if err != nil {
		return err
	}
	if len(p.spec.Weights) != count {
		return fmt.Errorf("projection has %d coefficients, want %d", len(p.spec.Weights), count)
	}
	if err := validateProjectionSelection(p.spec.Selection); err != nil {
		return err
	}
	if p.spec.Selection.Mode == SelectExplicit && !reflect.DeepEqual(p.spec.Selection.Neurons, p.neurons) {
		return fmt.Errorf("explicit selection differs from resolved neuron order")
	}
	if p.spec.Selection.Mode != SelectExplicit && !sort.SliceIsSorted(p.neurons, func(i, j int) bool { return projectionNeuronLess(p.neurons[i], p.neurons[j]) }) {
		return fmt.Errorf("metadata projection neurons must use canonical order")
	}
	seen := make(map[NeuronID]bool, len(p.neurons))
	for _, id := range p.neurons {
		if err := id.validate(); err != nil {
			return err
		}
		if !utf8.ValidString(id.Namespace) || !utf8.ValidString(id.ExternalID) || seen[id] {
			return fmt.Errorf("projection neurons must be unique valid UTF-8 IDs")
		}
		seen[id] = true
	}
	if p.neuronHash != "" && p.neuronHash != projectionNeuronHash(p.neurons) {
		return fmt.Errorf("projection neuron hash mismatch")
	}
	var state uint64
	if p.spec.Random != nil {
		if err := validateProjectionRandom(p.spec.Random); err != nil {
			return err
		}
		state = p.spec.Random.Seed
	}
	for i, w := range p.spec.Weights {
		if math.IsNaN(w) || math.IsInf(w, 0) {
			return fmt.Errorf("projection weight %d must be finite", i)
		}
		if p.spec.Random != nil && w != projectionRandomValue(&state, p.spec.Random.Scale) {
			return fmt.Errorf("projection weight %d differs from seed reconstruction", i)
		}
	}
	return nil
}

func validateProjectionSelection(s NeuronSelection) error {
	switch s.Mode {
	case SelectExplicit:
		if len(s.Neurons) == 0 || len(s.Neurons) > MaxProjectionNeurons || s.Value != "" {
			return fmt.Errorf("explicit selection requires neurons and no metadata value")
		}
	case SelectCellType, SelectRegion:
		if strings.TrimSpace(s.Value) == "" || !utf8.ValidString(s.Value) || len(s.Neurons) != 0 {
			return fmt.Errorf("metadata selection requires an exact known value and no explicit neurons")
		}
	default:
		return fmt.Errorf("unsupported neuron selection %q", s.Mode)
	}
	return nil
}

func selectProjectionNeurons(s NeuronSelection, candidates []NeuronCandidate) ([]NeuronID, []int, error) {
	if err := validateProjectionSelection(s); err != nil {
		return nil, nil, err
	}
	if len(candidates) > MaxProjectionNeurons {
		return nil, nil, fmt.Errorf("candidate graph exceeds %d neurons", MaxProjectionNeurons)
	}
	positions := make(map[NeuronID]int, len(candidates))
	for i, c := range candidates {
		if err := c.ID.validate(); err != nil {
			return nil, nil, fmt.Errorf("candidate %d: %w", i, err)
		}
		if !utf8.ValidString(c.ID.Namespace) || !utf8.ValidString(c.ID.ExternalID) || !utf8.ValidString(c.CellType) || !utf8.ValidString(c.Region) {
			return nil, nil, fmt.Errorf("candidate %d must contain valid UTF-8", i)
		}
		if _, ok := positions[c.ID]; ok {
			return nil, nil, fmt.Errorf("duplicate candidate neuron %v", c.ID)
		}
		positions[c.ID] = i
	}
	var ids []NeuronID
	if s.Mode == SelectExplicit {
		ids = append([]NeuronID(nil), s.Neurons...)
	} else {
		for _, c := range candidates {
			if (s.Mode == SelectCellType && c.CellType == s.Value) || (s.Mode == SelectRegion && c.Region == s.Value) {
				ids = append(ids, c.ID)
			}
		}
		sort.Slice(ids, func(i, j int) bool { return projectionNeuronLess(ids[i], ids[j]) })
	}
	if len(ids) == 0 {
		return nil, nil, fmt.Errorf("selection matches no neurons")
	}
	indices := make([]int, len(ids))
	seen := make(map[NeuronID]bool, len(ids))
	for i, id := range ids {
		index, ok := positions[id]
		if !ok || seen[id] {
			return nil, nil, fmt.Errorf("selected neuron %v is absent or duplicated", id)
		}
		seen[id] = true
		indices[i] = index
	}
	return ids, indices, nil
}

// Bind validates graph membership and exact metadata selection, returning
// indices in projection order. Changing graph order is safe; changing the
// selected set fails and requires an explicitly rebuilt projection.
func (p Projection) Bind(candidates []NeuronCandidate) ([]int, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	ids, indices, err := selectProjectionNeurons(p.spec.Selection, candidates)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(ids, p.neurons) {
		return nil, fmt.Errorf("graph selection differs from persisted projection")
	}
	return indices, nil
}

func (p Projection) Spec() ProjectionSpec           { return copyProjectionSpec(p.spec) }
func (p Projection) Direction() ProjectionDirection { return p.spec.Direction }
func (p Projection) ChannelShape() []int            { return append([]int(nil), p.spec.ChannelShape...) }
func (p Projection) Weights() []float64             { return append([]float64(nil), p.spec.Weights...) }
func (p Projection) Neurons() []NeuronID            { return append([]NeuronID(nil), p.neurons...) }

// NeuronHash covers the ordered ID list, so coefficient columns/rows cannot
// silently acquire different neuron identities when persisted order changes.
func (p Projection) NeuronHash() string { return p.neuronHash }
func (p Projection) InputSize() int {
	n, _ := shapeSize(p.spec.ChannelShape)
	if p.spec.Direction == ProjectionInput {
		return n
	}
	return len(p.neurons)
}
func (p Projection) OutputSize() int {
	n, _ := shapeSize(p.spec.ChannelShape)
	if p.spec.Direction == ProjectionOutput {
		return n
	}
	return len(p.neurons)
}

func copyProjectionSpec(s ProjectionSpec) ProjectionSpec {
	s.ChannelShape = append([]int(nil), s.ChannelShape...)
	s.Selection.Neurons = append([]NeuronID(nil), s.Selection.Neurons...)
	s.Weights = append([]float64(nil), s.Weights...)
	if s.Random != nil {
		r := *s.Random
		s.Random = &r
	}
	return s
}
func projectionNeuronHash(ids []NeuronID) string {
	data, _ := json.Marshal(ids)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
func validateProjectionRandom(r *ProjectionRandom) error {
	if !(r.Scale > 0) || math.IsNaN(r.Scale) || math.IsInf(r.Scale, 0) {
		return fmt.Errorf("random projection scale must be positive and finite")
	}
	return nil
}
func projectionRandomValue(state *uint64, scale float64) float64 {
	// Unsigned wrapping is part of the persisted splitmix64-sign/v1 algorithm.
	*state += 0x9e3779b97f4a7c15
	z := *state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	z ^= z >> 31
	if z&1 == 0 {
		return -scale
	}
	return scale
}

type projectionRandomJSON struct {
	Algorithm string              `json:"algorithm"`
	Seed      *jsonNumber[uint64] `json:"seed"`
	Scale     jsonNumber[float64] `json:"scale"`
}
type projectionJSON struct {
	SchemaVersion  Version               `json:"schema_version"`
	Direction      ProjectionDirection   `json:"direction"`
	Source         string                `json:"source"`
	Artificial     *bool                 `json:"artificial"`
	Evidence       string                `json:"evidence,omitempty"`
	ChannelShape   []int                 `json:"channel_shape"`
	SelectionMode  SelectionMode         `json:"selection_mode"`
	SelectionValue string                `json:"selection_value,omitempty"`
	Neurons        []NeuronID            `json:"neurons"`
	NeuronHash     string                `json:"neuron_hash"`
	InputSize      jsonNumber[int]       `json:"input_size"`
	OutputSize     jsonNumber[int]       `json:"output_size"`
	Weights        jsonValues            `json:"weights"`
	Random         *projectionRandomJSON `json:"random,omitempty"`
}

func (p Projection) MarshalJSON() ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	artificial := p.spec.Artificial
	raw := projectionJSON{SchemaVersion: p.spec.SchemaVersion, Direction: p.spec.Direction, Source: p.spec.Source, Artificial: &artificial, Evidence: p.spec.Evidence, ChannelShape: p.ChannelShape(), SelectionMode: p.spec.Selection.Mode, SelectionValue: p.spec.Selection.Value, Neurons: p.Neurons(), NeuronHash: p.neuronHash, InputSize: jsonNumber[int]{p.InputSize()}, OutputSize: jsonNumber[int]{p.OutputSize()}, Weights: p.Weights()}
	if p.spec.Random != nil {
		raw.Random = &projectionRandomJSON{Algorithm: "splitmix64-sign/v1", Seed: &jsonNumber[uint64]{p.spec.Random.Seed}, Scale: jsonNumber[float64]{p.spec.Random.Scale}}
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > MaxJSONBytes {
		return nil, fmt.Errorf("projection JSON exceeds %d bytes", MaxJSONBytes)
	}
	return data, nil
}
func (p *Projection) UnmarshalJSON(data []byte) error {
	var raw projectionJSON
	if err := decodeStrictBytes(data, &raw); err != nil {
		return err
	}
	if raw.Artificial == nil || raw.NeuronHash == "" {
		return fmt.Errorf("projection requires artificial flag and neuron hash")
	}
	s := ProjectionSpec{SchemaVersion: raw.SchemaVersion, Direction: raw.Direction, Source: raw.Source, Artificial: *raw.Artificial, Evidence: raw.Evidence, ChannelShape: raw.ChannelShape, Selection: NeuronSelection{Mode: raw.SelectionMode, Value: raw.SelectionValue}, Weights: raw.Weights}
	if s.Selection.Mode == SelectExplicit {
		s.Selection.Neurons = raw.Neurons
	}
	if raw.Random != nil {
		if raw.Random.Algorithm != "splitmix64-sign/v1" || raw.Random.Seed == nil {
			return fmt.Errorf("random projection requires supported algorithm and seed")
		}
		s.Random = &ProjectionRandom{Seed: raw.Random.Seed.value, Scale: raw.Random.Scale.value}
	}
	candidate := Projection{spec: s, neurons: raw.Neurons, neuronHash: raw.NeuronHash}
	if err := candidate.Validate(); err != nil {
		return err
	}
	if raw.InputSize.value != candidate.InputSize() || raw.OutputSize.value != candidate.OutputSize() {
		return fmt.Errorf("persisted projection dimensions do not match channel/neuron shape")
	}
	candidate.spec = copyProjectionSpec(s)
	candidate.neurons = append([]NeuronID(nil), raw.Neurons...)
	*p = candidate
	return nil
}
func DecodeProjection(r io.Reader) (Projection, error) {
	var p Projection
	if err := decodeStrict(r, &p); err != nil {
		return Projection{}, err
	}
	return p, nil
}

// ValidateChannelShape checks the original channel rank and dimensions, before
// callers flatten channel values into a learning input or output vector.
func (p Projection) ValidateChannelShape(shape []int) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if !reflect.DeepEqual(shape, p.spec.ChannelShape) {
		return fmt.Errorf("channel shape %v does not match projection shape %v", shape, p.spec.ChannelShape)
	}
	return nil
}

func projectionNeuronLess(a, b NeuronID) bool {
	if a.Namespace != b.Namespace {
		return a.Namespace < b.Namespace
	}
	return a.ExternalID < b.ExternalID
}
