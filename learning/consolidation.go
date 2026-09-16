package learning

import (
	"context"
	"errors"
	"fmt"
	"math"
)

// The three refusals a consolidation write can hit. Each is the absent state of
// one report flag: ErrConsolidationGateClosed means the receptor occupancy did
// not reach the threshold, ErrAlreadyConsolidated means the episode clock has
// already claimed this episode, and ErrBudgetExhausted means the layer has no
// budget slot left. Callers that only care whether a write happened can check
// errors.Is; callers that want the counters read the returned report either way.
var (
	ErrConsolidationGateClosed = errors.New("consolidation gate closed")
	ErrAlreadyConsolidated     = errors.New("already consolidated this episode")
	ErrBudgetExhausted         = errors.New("consolidation budget exhausted")
)

// SlowState is one consolidation layer, holding one value per enabled edge, in
// the order the layer's model was enabled with. LastEpisode is the episode
// clock of the write, Budget the total number of writes the layer allows and
// Used how many it has taken, so the layer serializes like the other optional
// parts a checkpoint must carry: an absent layer is "off", a zero-filled one is
// "on with nothing written yet".
type SlowState struct {
	Values      []float64 `json:"values"`
	LastEpisode uint64    `json:"last_episode,omitempty"`
	Budget      uint64    `json:"budget,omitempty"`
	Used        uint64    `json:"used,omitempty"`
}

// ConsolidationTrigger declares the conditions of one consolidation write:
// Receptor is the occupancy the write gates on, Threshold the occupancy below
// which the gate stays closed, Rate the fraction of the fast change that moves
// into the slow layer and Retain the fraction of the fast change that survives.
type ConsolidationTrigger struct {
	Receptor  int
	Threshold float64
	Rate      float64
	Retain    float64
}

// ConsolidationReport counts every outcome of one Consolidate call. On a
// refused write the counters that led to the refusal are still reported: the
// occupancy and the episode clock are what the caller needs to diagnose a
// closed gate, and Used is what remains of the budget.
type ConsolidationReport struct {
	Applied         bool    `json:"applied,omitempty"`
	GateClosed      bool    `json:"gate_closed,omitempty"`
	Already         bool    `json:"already,omitempty"`
	BudgetExhausted bool    `json:"budget_exhausted,omitempty"`
	Occupancy       float64 `json:"occupancy,omitempty"`
	Episode         uint64  `json:"episode,omitempty"`
	LastEpisode     uint64  `json:"last_episode,omitempty"`
	Used            uint64  `json:"used,omitempty"`
	BaseL2          float64 `json:"base_l2,omitempty"`
	SlowL2          float64 `json:"slow_l2,omitempty"`
	PlasticL2       float64 `json:"plastic_l2,omitempty"`
}

// EnableConsolidation turns the persistent slow consolidation layer on for this
// individual. It needs local plasticity: a consolidation layer without a place
// to write from would be a silent no-op. The budget is the total number of
// writes the layer is allowed across its life, and a second call replaces the
// declaration and resets the layer, exactly like EnablePlasticity does for the
// fast state.
func (i *Individual) EnableConsolidation(budget uint64) error {
	if i == nil || i.trainer == nil {
		return fmt.Errorf("uninitialized individual")
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.plastic == nil {
		return fmt.Errorf("consolidation needs an enabled local plasticity")
	}
	i.plastic.slow = &SlowState{Values: make([]float64, i.plastic.model.Edges()), Budget: budget}
	return nil
}

// Consolidate runs one consolidation write under the trigger, under the lock,
// in the one fixed order:
//
//	trigger validation -> the declared chain of refusals
//	-> the write: slow += Rate*plastic, then plastic *= Retain
//	-> the counters: LastEpisode, Used
//
// The reported L2 norms are measured after the write, so they reflect the
// state the run leaves behind: BaseL2 over every core weight, SlowL2 over the
// slow layer and PlasticL2 over the surviving fast change.
func (i *Individual) Consolidate(ctx context.Context, trigger ConsolidationTrigger) (ConsolidationReport, error) {
	var report ConsolidationReport
	if i == nil || i.trainer == nil {
		return report, fmt.Errorf("uninitialized individual")
	}
	if err := i.mu.LockContext(ctx); err != nil {
		return report, err
	}
	defer i.mu.Unlock()
	if trigger.Receptor < 0 {
		return report, fmt.Errorf("consolidation trigger receptor index %d is negative", trigger.Receptor)
	}
	if i.chemical == nil {
		return report, fmt.Errorf("consolidation needs an enabled chemistry")
	}
	if trigger.Receptor >= len(i.chemical.config.Receptors.Records) {
		return report, fmt.Errorf("consolidation trigger receptor index %d is out of range (0..%d)", trigger.Receptor, len(i.chemical.config.Receptors.Records)-1)
	}
	if !finite(trigger.Threshold) || trigger.Threshold < 0 {
		return report, fmt.Errorf("consolidation trigger threshold is %v, want a finite value >= 0", trigger.Threshold)
	}
	if !finite(trigger.Rate) || trigger.Rate <= 0 || trigger.Rate > 1 {
		return report, fmt.Errorf("consolidation trigger rate is %v, want a value in (0, 1]", trigger.Rate)
	}
	if !finite(trigger.Retain) || trigger.Retain < 0 || trigger.Retain > 1 {
		return report, fmt.Errorf("consolidation trigger retain is %v, want a value in [0, 1]", trigger.Retain)
	}
	if i.plastic == nil {
		return report, fmt.Errorf("consolidation needs an enabled local plasticity")
	}
	if i.plastic.slow == nil {
		return report, fmt.Errorf("consolidation layer is not enabled")
	}
	slow := i.plastic.slow
	report.Episode = i.episodes
	report.Occupancy = meanOccupancy(i.chemical.report.Occupancy, trigger.Receptor)
	if report.Occupancy < trigger.Threshold {
		report.GateClosed = true
		return report, ErrConsolidationGateClosed
	}
	if slow.LastEpisode != 0 && slow.LastEpisode >= i.episodes+1 {
		report.Already = true
		return report, ErrAlreadyConsolidated
	}
	if slow.Used >= slow.Budget {
		report.BudgetExhausted = true
		return report, ErrBudgetExhausted
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	plastic := i.plastic.state.Plastic
	for k := range slow.Values {
		slow.Values[k] += trigger.Rate * plastic[k]
	}
	for k := range plastic {
		plastic[k] *= trigger.Retain
	}
	slow.LastEpisode = i.episodes + 1
	slow.Used++
	report.Applied = true
	report.LastEpisode = slow.LastEpisode
	report.Used = slow.Used
	report.BaseL2 = l2(i.trainer.parameters.Core.Weights)
	report.SlowL2 = l2(slow.Values)
	report.PlasticL2 = l2(plastic)
	return report, nil
}

// copySlowState deep copies one consolidation layer, keeping it absent rather
// than turning it into a zero-filled one.
func copySlowState(s *SlowState) *SlowState {
	if s == nil {
		return nil
	}
	owned := *s
	owned.Values = append([]float64(nil), s.Values...)
	return &owned
}

// validateSlowState refuses a layer that does not match the enabled edges or a
// budget that the used counter has already overshot. A nil layer is the
// declared "never enabled" state and passes.
func validateSlowState(s *SlowState, edges int) error {
	if s == nil {
		return nil
	}
	if len(s.Values) != edges {
		return fmt.Errorf("slow state has %d values, the model declares %d enabled edges", len(s.Values), edges)
	}
	for i, v := range s.Values {
		if !finite(v) {
			return fmt.Errorf("slow state value %d is not finite", i)
		}
	}
	if s.Used > s.Budget {
		return fmt.Errorf("slow state has used %d of its %d budget", s.Used, s.Budget)
	}
	return nil
}

// l2 is the Euclidean norm of one vector, the size each consolidation layer
// reports independently so a checkpoint shows how much of the weight magnitude
// sits in which layer. It is package-private because it belongs to the size
// accounting, not to a public API.
func l2(x []float64) float64 {
	var sum float64
	for _, v := range x {
		sum += v * v
	}
	return math.Sqrt(sum)
}
