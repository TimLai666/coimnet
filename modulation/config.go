package modulation

import (
	"fmt"
	"strings"
)

// The four source kinds a SourceSpec may declare. The trainable controller of
// MOD-07 is deliberately absent: NewController still refuses it, so a
// declaration cannot reach a source that does not exist.
const (
	SourceExternalTimeline = "external_timeline"
	SourceNeuralActivity   = "neural_activity"
	SourceInternalResource = "internal_resource"
	SourceReplay           = "replay"
)

// SourceSpec is one serializable source declaration. Kind names which body is
// present and exactly one body must be, so a document cannot describe two
// sources at once or none at all.
//
// Channel is the chemistry channel this source feeds. The four kinds all index
// their release vector by channel and declare a zero on every channel below the
// one they release on, so a spec's source must declare exactly Channel+1
// channels: a narrower one could not reach the channel, and a wider one would
// carry rates this layer never reads.
type SourceSpec struct {
	Kind     string            `json:"kind"`
	Channel  int               `json:"channel"`
	Timeline *ExternalTimeline `json:"timeline,omitempty"`
	Neural   *NeuralActivity   `json:"neural,omitempty"`
	Resource *InternalResource `json:"resource,omitempty"`
	Replay   *Replay           `json:"replay,omitempty"`
}

// Build validates the declaration and returns the source it names. The result
// owns its own copy of the body, so writing into the spec afterwards changes
// nothing the caller already built.
func (s SourceSpec) Build() (Source, error) {
	if s.Channel < 0 {
		return nil, fmt.Errorf("modulation: source spec declares channel %d", s.Channel)
	}
	bodies := 0
	for _, present := range []bool{s.Timeline != nil, s.Neural != nil, s.Resource != nil, s.Replay != nil} {
		if present {
			bodies++
		}
	}
	if bodies != 1 {
		return nil, fmt.Errorf("modulation: a source spec carries exactly one body, got %d", bodies)
	}
	var built Source
	switch s.Kind {
	case SourceExternalTimeline:
		if s.Timeline == nil {
			return nil, fmt.Errorf("modulation: source spec of kind %q carries no timeline", s.Kind)
		}
		timeline := ExternalTimeline{ChannelCount: s.Timeline.ChannelCount, Entries: append([]TimelineEntry(nil), s.Timeline.Entries...)}
		if err := timeline.Validate(); err != nil {
			return nil, err
		}
		built = timeline
	case SourceNeuralActivity:
		if s.Neural == nil {
			return nil, fmt.Errorf("modulation: source spec of kind %q carries no neural body", s.Kind)
		}
		neural := *s.Neural
		neural.Nodes = append([]int(nil), s.Neural.Nodes...)
		if err := neural.Validate(); err != nil {
			return nil, err
		}
		if neural.Channel != s.Channel {
			return nil, fmt.Errorf("modulation: source spec feeds channel %d but its neural body releases on channel %d", s.Channel, neural.Channel)
		}
		built = neural
	case SourceInternalResource:
		if s.Resource == nil {
			return nil, fmt.Errorf("modulation: source spec of kind %q carries no resource body", s.Kind)
		}
		resource := *s.Resource
		if strings.TrimSpace(resource.Resource) == "" {
			return nil, fmt.Errorf("modulation: internal resource source names no resource")
		}
		if resource.Channel != s.Channel {
			return nil, fmt.Errorf("modulation: source spec feeds channel %d but its resource body releases on channel %d", s.Channel, resource.Channel)
		}
		if !finite(resource.Coefficient) || resource.Coefficient < 0 {
			return nil, fmt.Errorf("modulation: internal resource source declares coefficient %v", resource.Coefficient)
		}
		if !finite(resource.Threshold) {
			return nil, fmt.Errorf("modulation: internal resource source declares a non-finite threshold")
		}
		built = resource
	case SourceReplay:
		if s.Replay == nil {
			return nil, fmt.Errorf("modulation: source spec of kind %q carries no replay body", s.Kind)
		}
		trace := make([][]float64, len(s.Replay.Trace))
		for i, row := range s.Replay.Trace {
			trace[i] = append([]float64(nil), row...)
		}
		replay := Replay{Trace: trace}
		if err := replay.validate(); err != nil {
			return nil, err
		}
		built = replay
	default:
		return nil, fmt.Errorf("modulation: unsupported source kind %q", s.Kind)
	}
	if built.Channels() != s.Channel+1 {
		return nil, fmt.Errorf("modulation: source spec feeds channel %d, so its body must declare %d channels, got %d", s.Channel, s.Channel+1, built.Channels())
	}
	return built, nil
}

// clone returns a spec that shares no buffer with the receiver.
func (s SourceSpec) clone() SourceSpec {
	owned := SourceSpec{Kind: s.Kind, Channel: s.Channel}
	if s.Timeline != nil {
		owned.Timeline = &ExternalTimeline{ChannelCount: s.Timeline.ChannelCount, Entries: append([]TimelineEntry(nil), s.Timeline.Entries...)}
	}
	if s.Neural != nil {
		neural := *s.Neural
		neural.Nodes = append([]int(nil), s.Neural.Nodes...)
		owned.Neural = &neural
	}
	if s.Resource != nil {
		resource := *s.Resource
		owned.Resource = &resource
	}
	if s.Replay != nil {
		trace := make([][]float64, len(s.Replay.Trace))
		for i, row := range s.Replay.Trace {
			trace[i] = append([]float64(nil), row...)
		}
		owned.Replay = &Replay{Trace: trace}
	}
	return owned
}

// RegionAssignment names the region every node sits in, one entry per node in
// node order. It is chemical data declared by the researcher, exactly like the
// transport matrix, and is never derived from the wiring.
type RegionAssignment struct {
	NodeRegion []int `json:"node_region"`
}

// ChemistryConfig is the whole declaration a persistent individual runs its
// chemical layer from: the concentration kinetics, one source per channel in
// channel order, the receptor set, the effects those receptors drive and the
// region every node belongs to. Every field is serializable, because the
// individual snapshot carries the declaration next to the state it produced.
type ChemistryConfig struct {
	Chemistry Chemistry        `json:"chemistry"`
	Sources   []SourceSpec     `json:"sources"`
	Receptors Receptors        `json:"receptors"`
	Effects   []Effect         `json:"effects"`
	Regions   RegionAssignment `json:"regions"`
}

// Validate checks the whole declaration against the node count of the model it
// is about to run on: the kinetics build, there is exactly one source per
// channel and it sits at its own index, every node has a region inside the
// declared region count, the receptor set is valid for these nodes and
// channels, and every effect names a receptor that exists and declares only the
// knobs its kind reads.
func (c ChemistryConfig) Validate(nodes int) error {
	if nodes < 1 {
		return fmt.Errorf("modulation: a chemistry needs at least one node, got %d", nodes)
	}
	if _, err := NewChemistry(c.Chemistry); err != nil {
		return err
	}
	if len(c.Sources) != c.Chemistry.Channels {
		return fmt.Errorf("modulation: %d sources for %d channels, want exactly one per channel", len(c.Sources), c.Chemistry.Channels)
	}
	for k, spec := range c.Sources {
		if spec.Channel != k {
			return fmt.Errorf("modulation: source %d feeds channel %d, want the sources in channel order", k, spec.Channel)
		}
		if _, err := spec.Build(); err != nil {
			return fmt.Errorf("modulation: source %d: %w", k, err)
		}
		if spec.Neural != nil {
			for _, node := range spec.Neural.Nodes {
				if node >= nodes {
					return fmt.Errorf("modulation: source %d names node %d, outside the %d nodes of this model", k, node, nodes)
				}
			}
		}
	}
	if len(c.Regions.NodeRegion) != nodes {
		return fmt.Errorf("modulation: the region map names %d nodes, want %d", len(c.Regions.NodeRegion), nodes)
	}
	for node, region := range c.Regions.NodeRegion {
		if region < 0 || region >= c.Chemistry.Regions {
			return fmt.Errorf("modulation: node %d is mapped to region %d, outside [0, %d)", node, region, c.Chemistry.Regions)
		}
	}
	receptors := c.Receptors
	if err := receptors.Validate(nodes, c.Chemistry.Channels); err != nil {
		return err
	}
	for i, effect := range c.Effects {
		if effect.Receptor < 0 || effect.Receptor >= len(c.Receptors.Records) {
			return fmt.Errorf("modulation: effect %d names receptor %d, outside the %d declared receptors", i, effect.Receptor, len(c.Receptors.Records))
		}
	}
	// ApplyEffects owns the per-declaration rules (known kind, declared bounds,
	// no knob the kind never reads, a supported mix). Calling it with no
	// occupancy record validates the declarations without computing anything.
	if _, _, _, _, err := ApplyEffects(c.Effects, nil, c.Receptors.Mix, nodes); err != nil {
		return err
	}
	return nil
}

// ValidateState checks a chemical state against this declaration: one row per
// declared region, one column per declared channel, every entry finite and
// non-negative. It is the gate a restored snapshot passes before it becomes a
// running state.
func (c ChemistryConfig) ValidateState(s ChemistryState) error {
	regions, channels, err := concentrationShape(s.Concentration)
	if err != nil {
		return err
	}
	if regions != c.Chemistry.Regions {
		return fmt.Errorf("modulation: the state has %d regions, the declaration has %d", regions, c.Chemistry.Regions)
	}
	if channels != c.Chemistry.Channels {
		return fmt.Errorf("modulation: the state has %d channels, the declaration has %d", channels, c.Chemistry.Channels)
	}
	return nil
}

// Clone returns a declaration that shares no buffer with the receiver.
func (c ChemistryConfig) Clone() ChemistryConfig {
	owned := ChemistryConfig{
		Chemistry: c.Chemistry,
		Receptors: Receptors{Mix: c.Receptors.Mix, AllowAssumedCoefficients: c.Receptors.AllowAssumedCoefficients},
		Effects:   append([]Effect(nil), c.Effects...),
		Regions:   RegionAssignment{NodeRegion: append([]int(nil), c.Regions.NodeRegion...)},
	}
	owned.Chemistry.Tau = append([]float64(nil), c.Chemistry.Tau...)
	if c.Chemistry.Transport != nil {
		fraction := make([][]float64, len(c.Chemistry.Transport.Fraction))
		for r, row := range c.Chemistry.Transport.Fraction {
			fraction[r] = append([]float64(nil), row...)
		}
		owned.Chemistry.Transport = &Transport{Fraction: fraction}
	}
	if c.Sources != nil {
		owned.Sources = make([]SourceSpec, len(c.Sources))
		for i, spec := range c.Sources {
			owned.Sources[i] = spec.clone()
		}
	}
	if c.Receptors.Records != nil {
		owned.Receptors.Records = make([]Receptor, len(c.Receptors.Records))
		for i, record := range c.Receptors.Records {
			owned.Receptors.Records[i] = record
			owned.Receptors.Records[i].Cells = append([]int(nil), record.Cells...)
		}
	}
	return owned
}
