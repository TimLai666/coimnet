package learning

import (
	"errors"
	"fmt"
	"strings"

	"github.com/TimLai666/coimnet/dynamics"
)

// Intervention kinds (主規格 11.8 的清單).
const (
	InterventionClampVoltage     = "clamp_voltage"
	InterventionForceSpike       = "force_spike"
	InterventionSilence          = "silence"
	InterventionBlockChannel     = "block_channel"
	InterventionFixConcentration = "fix_concentration"
	InterventionShuffleDelays    = "shuffle_delays"
	InterventionSwapRegions      = "swap_regions"
	InterventionRemoveChannel    = "remove_channel"
)

// Intervention is one authorised override applied on the rows [Start, End).
// Targets are node indices for the node kinds, Channel a chemistry channel
// for the channel kinds, Regions the two regions swap_regions exchanges.
type Intervention struct {
	Kind    string  `json:"kind"`
	Targets []int   `json:"targets,omitempty"`
	Channel int     `json:"channel,omitempty"`
	Regions [2]int  `json:"regions,omitempty"`
	Value   float64 `json:"value,omitempty"`
	Start   uint64  `json:"start"`
	End     uint64  `json:"end"`
	Restore bool    `json:"restore"`
	Seed    uint64  `json:"seed,omitempty"`
}

// InterventionPlan is only accepted when Authorized is true and Reason is
// non-blank: ordinary inference never clamps state, an experiment must say so.
type InterventionPlan struct {
	Authorized bool           `json:"authorized"`
	Reason     string         `json:"reason"`
	Items      []Intervention `json:"items"`
}

// InterventionShape is what Validate checks a plan against.
type InterventionShape struct {
	Nodes    int  // node count of the core
	Spiking  bool // force_spike needs a spiking core
	Channels int  // chemistry channels (0 = chemistry disabled)
	Regions  int  // chemistry regions (0 = chemistry disabled)
	MaxDelay int  // largest declared edge delay (shuffle_delays needs at least one edge with delay > 0)
	Edges    int
}

// ErrInterventionNotAuthorized is the refusal every plan that does not say it
// experiments gets. It is returned unwrapped where Validate can produce no
// detail, so errors.Is recognises it on any caller path.
var ErrInterventionNotAuthorized = errors.New("learning: interventions need an authorized plan with a reason")

// Validate checks one plan against the shape of the core it is about to run
// on. The checks follow the eight rules of COR-11: an experiment that says so,
// declared items of one of the eight kinds, a nonempty interval with a finite
// value, then the per-kind bounds, then that no two items of the same kind
// overwrite the same target at the same time. A refused plan is an error that
// names the item index and the field it failed.
func (p InterventionPlan) Validate(shape InterventionShape) error {
	if !p.Authorized || strings.TrimSpace(p.Reason) == "" {
		return ErrInterventionNotAuthorized
	}
	if len(p.Items) == 0 {
		return fmt.Errorf("learning: intervention plan items must not be empty")
	}
	for i, item := range p.Items {
		switch item.Kind {
		case InterventionClampVoltage, InterventionForceSpike, InterventionSilence:
			if err := validateNodeTargets(i, item, shape); err != nil {
				return err
			}
			if item.Kind == InterventionForceSpike && !shape.Spiking {
				return fmt.Errorf("learning: items[%d].targets: kind %q needs a spiking core", i, item.Kind)
			}
		case InterventionBlockChannel, InterventionFixConcentration, InterventionRemoveChannel:
			if shape.Channels < 1 {
				return fmt.Errorf("learning: items[%d].channel: kind %q needs enabled chemistry", i, item.Kind)
			}
			if item.Channel < 0 || item.Channel >= shape.Channels {
				return fmt.Errorf("learning: items[%d].channel: channel %d outside [0, %d)", i, item.Channel, shape.Channels)
			}
			if item.Kind == InterventionFixConcentration && item.Value < 0 {
				return fmt.Errorf("learning: items[%d].value: kind %q value must be non-negative", i, item.Kind)
			}
		case InterventionSwapRegions:
			if shape.Regions < 2 {
				return fmt.Errorf("learning: items[%d].regions: kind %q needs at least two regions", i, item.Kind)
			}
			if item.Regions[0] == item.Regions[1] {
				return fmt.Errorf("learning: items[%d].regions: kind %q needs two distinct regions", i, item.Kind)
			}
			for _, region := range item.Regions {
				if region < 0 || region >= shape.Regions {
					return fmt.Errorf("learning: items[%d].regions: region %d outside [0, %d)", i, region, shape.Regions)
				}
			}
		case InterventionShuffleDelays:
			if shape.MaxDelay < 1 {
				return fmt.Errorf("learning: items[%d].targets: kind %q needs a positive max_delay", i, item.Kind)
			}
			if len(item.Targets) != 0 {
				if err := validateAscendingInRange(i, "targets", item.Targets, shape.Edges); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("learning: items[%d].kind: unknown intervention kind %q", i, item.Kind)
		}
		if item.Start >= item.End {
			return fmt.Errorf("learning: items[%d].start: interval [%d, %d) is empty", i, item.Start, item.End)
		}
		if !finite(item.Value) {
			return fmt.Errorf("learning: items[%d].value: value must be finite", i)
		}
	}
	return p.checkConflicts()
}

// validateNodeTargets checks the shared shape of a node kind: a nonempty list
// of strictly increasing node indices inside [0, Nodes).
func validateNodeTargets(i int, item Intervention, shape InterventionShape) error {
	if len(item.Targets) == 0 {
		return fmt.Errorf("learning: items[%d].targets: kind %q needs at least one node", i, item.Kind)
	}
	return validateAscendingInRange(i, "targets", item.Targets, shape.Nodes)
}

func validateAscendingInRange(i int, field string, values []int, limit int) error {
	for k, value := range values {
		if value < 0 || value >= limit {
			return fmt.Errorf("learning: items[%d].%s: index %d outside [0, %d)", i, field, value, limit)
		}
		if k > 0 && value <= values[k-1] {
			return fmt.Errorf("learning: items[%d].%s: indices must be strictly increasing", i, field)
		}
	}
	return nil
}

// checkConflicts enforces rule 8. Two items conflict when they may actually
// overwrite the same target at the same time: the same node kind on a shared
// node, any two channel kinds on a shared channel (one channel cannot be
// zeroed, fixed and removed at once), swap_regions on a shared region, or
// shuffle_delays on a shared edge, where a nil target list means every edge.
func (p InterventionPlan) checkConflicts() error {
	for i, a := range p.Items {
		for j := i + 1; j < len(p.Items); j++ {
			b := p.Items[j]
			if !sameConflictGroup(a, b) {
				continue
			}
			if !conflictingTargets(a, b) {
				continue
			}
			if !intervalsOverlap(a.Start, a.End, b.Start, b.End) {
				continue
			}
			return fmt.Errorf("learning: items[%d] and items[%d] both %q overlap in time on the same target", i, j, a.Kind)
		}
	}
	return nil
}

func intervalsOverlap(aStart, aEnd, bStart, bEnd uint64) bool {
	return aStart < bEnd && bStart < aEnd
}

// sameConflictGroup reports whether two items act on the same target space.
// The same kind always does; the three channel kinds also conflict across
// themselves because they all override one channel.
func sameConflictGroup(a, b Intervention) bool {
	if a.Kind == b.Kind {
		return true
	}
	switch a.Kind {
	case InterventionBlockChannel, InterventionFixConcentration, InterventionRemoveChannel:
		return b.Kind == InterventionBlockChannel || b.Kind == InterventionFixConcentration || b.Kind == InterventionRemoveChannel
	default:
		return false
	}
}

// conflictingTargets reports whether two items of the same group name an
// overlapping target.
func conflictingTargets(a, b Intervention) bool {
	switch a.Kind {
	case InterventionClampVoltage, InterventionForceSpike, InterventionSilence:
		for _, ta := range a.Targets {
			for _, tb := range b.Targets {
				if ta == tb {
					return true
				}
			}
		}
		return false
	case InterventionBlockChannel, InterventionFixConcentration, InterventionRemoveChannel:
		return a.Channel == b.Channel
	case InterventionSwapRegions:
		return a.Regions[0] == b.Regions[0] || a.Regions[0] == b.Regions[1] ||
			a.Regions[1] == b.Regions[0] || a.Regions[1] == b.Regions[1]
	case InterventionShuffleDelays:
		// A nil list means every edge, which overlaps any other list. With two
		// declared lists the conflict is the shared edge.
		if len(a.Targets) == 0 || len(b.Targets) == 0 {
			return true
		}
		for _, ta := range a.Targets {
			for _, tb := range b.Targets {
				if ta == tb {
					return true
				}
			}
		}
		return false
	default:
		return false
	}
}

// InterventionShape derives the shape of an individual from its own settings:
// the node and edge counts of whatever core it drives, whether that core can
// produce spikes, and the chemistry channel and region counts when the layer
// is enabled (zero when it is not). MaxDelay is the largest declared edge
// delay, the answer a shuffle_delays plan is checked against.
func (i *Individual) InterventionShape() InterventionShape {
	if i == nil || i.trainer == nil {
		return InterventionShape{}
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	c := i.trainer.network.Config()
	shape := InterventionShape{
		Nodes: configNodes(c),
		Edges: configEdges(c),
	}
	shape.Spiking = c.LIF != nil
	if c.Mixed != nil {
		for _, rule := range c.Mixed.NodeRule {
			if rule == dynamics.RuleLIF {
				shape.Spiking = true
				break
			}
		}
	}
	if i.chemical != nil {
		shape.Channels = i.chemical.config.Chemistry.Channels
		shape.Regions = i.chemical.config.Chemistry.Regions
	}
	switch {
	case c.Mixed != nil:
		for _, delay := range c.Mixed.Delays {
			if delay > shape.MaxDelay {
				shape.MaxDelay = delay
			}
		}
	case c.LIF != nil:
		for _, delay := range c.LIF.Delays {
			if delay > shape.MaxDelay {
				shape.MaxDelay = delay
			}
		}
	default:
		for _, delay := range c.Dynamics.Delays {
			if delay > shape.MaxDelay {
				shape.MaxDelay = delay
			}
		}
	}
	return shape
}
