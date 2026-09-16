package learning

import (
	"fmt"

	"github.com/TimLai666/coimnet/modulation"
)

// ExpressionGain scales the outputs of the named readout nodes by
// clamp(1 + Scale*occupancy, Min, Max) before the readout, where occupancy
// is the mean occupancy of Receptor on the current row. It touches only the
// readout path: core state, plastic state, chemistry state and parameters are
// unchanged by it, so a gain back at 1 restores the exact outputs. It is
// suppressed expression, not forgetting.
type ExpressionGain struct {
	Nodes    []int   `json:"nodes"`
	Receptor int     `json:"receptor"`
	Scale    float64 `json:"scale"`
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
}

// Validate checks one expression gain against the individual it is about to
// run on: the nodes must be non-empty, strictly increasing and inside the
// readout set, the receptor must be inside the declared receptor count, every
// coefficient must be finite, and the bounds must hold the neutral gain 1 so a
// zero occupancy leaves the readout unchanged.
func (g ExpressionGain) Validate(readoutNodes []int, receptors int) error {
	if len(g.Nodes) == 0 {
		return fmt.Errorf("expression gain needs at least one node")
	}
	if g.Receptor < 0 || g.Receptor >= receptors {
		return fmt.Errorf("expression gain receptor %d is out of range [0, %d)", g.Receptor, receptors)
	}
	if !finite(g.Scale) || !finite(g.Min) || !finite(g.Max) {
		return fmt.Errorf("expression gain scale, min and max must be finite")
	}
	if g.Min > 1 || g.Max < 1 {
		return fmt.Errorf("expression gain bounds must satisfy min <= 1 <= max")
	}
	inReadout := make(map[int]bool, len(readoutNodes))
	for _, id := range readoutNodes {
		inReadout[id] = true
	}
	prev := -1
	for _, id := range g.Nodes {
		if !inReadout[id] {
			return fmt.Errorf("expression gain node %d is not a readout node", id)
		}
		if id <= prev {
			return fmt.Errorf("expression gain nodes must be strictly increasing")
		}
		prev = id
	}
	return nil
}

// SetExpressionGain declares the readout-only gain and stores a copy. It needs
// an enabled chemistry, because that is the layer occupancy comes from; the
// declaration is validated against this individual's readout set and receptor
// count. A second call replaces the declaration.
func (i *Individual) SetExpressionGain(g ExpressionGain) error {
	if i == nil || i.trainer == nil {
		return fmt.Errorf("uninitialized individual")
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.chemical == nil {
		return fmt.Errorf("an expression gain needs an enabled chemistry")
	}
	readout := i.trainer.network.config.ReadoutNodes
	receptors := len(i.chemical.config.Receptors.Records)
	if err := g.Validate(readout, receptors); err != nil {
		return err
	}
	i.expression = copyExpression(&g)
	return nil
}

// ClearExpressionGain drops the declaration, so the readout returns to the
// untouched one. State accumulated while the gain was on is kept: suppressed
// expression, not forgetting.
func (i *Individual) ClearExpressionGain() {
	if i == nil || i.trainer == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.expression = nil
}

// copyExpression deep copies an expression gain, keeping an absent one absent.
func copyExpression(g *ExpressionGain) *ExpressionGain {
	if g == nil {
		return nil
	}
	owned := *g
	owned.Nodes = append([]int(nil), g.Nodes...)
	return &owned
}

// meanOccupancy is the mean of the occupancy records one receptor produced on
// one row. A row with no record for the receptor has no mean and reads 0,
// which keeps the gain at exactly 1.
func meanOccupancy(records []modulation.OccupancyRecord, receptor int) float64 {
	var total float64
	var count int
	for _, record := range records {
		if record.Receptor == receptor {
			total += record.Occupancy
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
