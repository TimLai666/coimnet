package simulate

import (
	"context"
	"fmt"
	"math"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/plasticity"
)

// Variant names of the learning matrix. They are cell names, not mechanisms:
// every one of them is an ordinary run of the same stimulus.
const (
	// VariantPlastic runs the protocol with the declared plasticity block.
	VariantPlastic = "plastic"
	// VariantLearnedThenFrozen reruns the same stimulus with w_eff fixed at
	// base plus the fast changes the plastic cell ended with, and no update.
	VariantLearnedThenFrozen = "learned_then_frozen"
)

// wallClockPenaltyNote is written into every plasticity report. The fast
// changes of one step decide the weights of the next one, so the runner cannot
// hand several steps to the core at once; the note states that cost where the
// numbers are read rather than leaving it to a document.
const wallClockPenaltyNote = "Local plasticity forces chunk_size 1: the fast changes of a step decide the weights of the next one, so the run advances one step per core call instead of the chunk the core would accept. The step count, the graph and the parameters are unchanged; only the wall clock is, and the cost grows with the step count, not with the enabled edge count."

// Plasticity declares local fast changes on top of a run. It is the protocol
// form of the plasticity package: Rule is the complete update law, exactly one
// of Edges and All selects the connections that follow it, and the learning
// gate is read from the stimulus itself, so a run needs no second input.
//
//	gate(t) = GateScale * stimulus[t][GateChannel]
//
// GateChannel must be a channel of the declared stimulus and must not be a
// channel any injection reads, because a channel that both drives neurons and
// opens the gate would make the two impossible to separate afterwards.
//
// Edges is a node selector and enables every edge whose two endpoints are both
// in the resolved set, which is the connections inside that population. An
// empty result is refused by plasticity.New rather than quietly enabling
// nothing.
//
// Frozen holds the fast changes where they are: the weights of every step are
// base plus the current fast changes and no update is applied. It is what
// learned_then_frozen runs, and it is also the form of "disabled" that leaves
// an all-zero declaration bit identical to a run without the block.
type Plasticity struct {
	Rule        plasticity.Rule `json:"rule"`
	Edges       *Selector       `json:"edges,omitempty"`
	All         bool            `json:"all,omitempty"`
	GateChannel int             `json:"gate_channel"`
	GateScale   float64         `json:"gate_scale"`
	Frozen      bool            `json:"frozen,omitempty"`
}

// PlasticityReport is what one Run did with the declared block. The two clamp
// counters are cumulative over the call and count one entry per step:
// ClampedByWMin counts fixed sign edges the w_min floor raised when the weights
// of a step were built, ClampedByPlasticMax counts fast changes the declared
// bound held. PlasticL2Before and PlasticL2After are the Euclidean norms of the
// fast change array around this call, so a reader sees how far the run moved
// without the array itself entering the report.
//
// ChunkSize is 1 whenever the block is present and WallClockPenaltyNote states
// why. Frozen repeats the declaration, because the same rule and the same edge
// selection mean something different when no update is applied.
type PlasticityReport struct {
	Rule                 string  `json:"rule"`
	EnabledEdges         int     `json:"enabled_edges"`
	GateChannel          int     `json:"gate_channel"`
	Frozen               bool    `json:"frozen,omitempty"`
	ClampedByWMin        uint64  `json:"clamped_by_w_min"`
	ClampedByPlasticMax  uint64  `json:"clamped_by_plastic_max"`
	PlasticL2Before      float64 `json:"plastic_l2_before"`
	PlasticL2After       float64 `json:"plastic_l2_after"`
	ChunkSize            int     `json:"chunk_size"`
	WallClockPenaltyNote string  `json:"wall_clock_penalty_note"`
}

// validate checks everything about the block that does not need a graph:
// exactly one edge declaration, the gate channel against the stimulus and the
// injections, a finite gate scale and the rule itself. The rule is validated by
// plasticity.New on a one-edge core, which is the package's own validator and
// therefore cannot drift from what Build will accept.
func (p Protocol) validatePlasticity(channels int) error {
	block := p.Plasticity
	if block == nil {
		return nil
	}
	if (block.Edges == nil) == !block.All {
		return fmt.Errorf("simulate: plasticity must declare exactly one of edges and all")
	}
	if block.GateChannel < 0 || block.GateChannel >= channels {
		return fmt.Errorf("simulate: plasticity gate_channel %d is outside [0,%d), the channels the stimulus declares", block.GateChannel, channels)
	}
	for i, injection := range p.Injections {
		if injection.Channel == block.GateChannel {
			return fmt.Errorf("simulate: plasticity gate_channel %d is also the channel of injection %d; the learning gate must be a channel no injection drives", block.GateChannel, i)
		}
	}
	for i, group := range p.InjectionGroups {
		if group.Channel == block.GateChannel {
			return fmt.Errorf("simulate: plasticity gate_channel %d is also the channel of injection group %d; the learning gate must be a channel no injection drives", block.GateChannel, i)
		}
	}
	if !finite(block.GateScale) {
		return fmt.Errorf("simulate: plasticity gate_scale is %v, want a finite value", block.GateScale)
	}
	if block.Rule.Kind == plasticity.RuleSTDPPair && p.Core != CoreLIF {
		return fmt.Errorf("simulate: plasticity rule %q needs 0/1 events from a spiking core; the %q core emits none", plasticity.RuleSTDPPair, p.Core)
	}
	if _, err := plasticity.New(plasticity.Config{Rule: block.Rule, Edges: []int{0}}, 1); err != nil {
		return fmt.Errorf("simulate: plasticity rule: %w", err)
	}
	return nil
}

// plasticEdges resolves the enabled edge indices in increasing order. all takes
// every edge of the run; a selector takes every edge whose source and target
// are both inside the resolved node set.
func plasticEdges(ctx context.Context, g *connectome.Graph, block Plasticity, sources, targets []int) ([]int, error) {
	if block.All {
		edges := make([]int, len(sources))
		for e := range edges {
			edges[e] = e
		}
		return edges, nil
	}
	nodes, _, err := resolveSelector(ctx, g, *block.Edges)
	if err != nil {
		return nil, err
	}
	inside := make(map[int]struct{}, len(nodes))
	for _, node := range nodes {
		inside[node] = struct{}{}
	}
	var edges []int
	for e, source := range sources {
		if _, ok := inside[source]; !ok {
			continue
		}
		if _, ok := inside[targets[e]]; !ok {
			continue
		}
		edges = append(edges, e)
	}
	if len(edges) == 0 {
		return nil, fmt.Errorf("simulate: plasticity selector %s == %q resolved %d nodes but no edge runs between them", block.Edges.Field, block.Edges.Equals, len(nodes))
	}
	return edges, nil
}

// Accounted bytes the plasticity block adds. The runner keeps its own copy of
// the edge endpoints because plasticity.Step reads them, the model keeps the
// enabled edge indices, and one step holds the current fast state, the state it
// produces and the effective weight array it hands to the core.
const (
	bytesPerEdgePlasticTopology = 8 + 8 // the retained sources and targets
	bytesPerEdgePlasticWeights  = 8     // the per-step effective weight array
	bytesPerEnabledEdgeIndex    = 8     // the model's own edge selection
	bytesPerEnabledEdgeValue    = 8     // one float64 of one fast state array
)

// accountPlasticity adds the arrays the block needs to the bytes the run
// already accounts for and refuses the request if the sum leaves the limit.
// arrays is the number of per-edge float64 arrays one step holds live: two for
// hebbian_rate (eligibility and fast change) and four for stdp_pair, counted
// twice because Step returns a new state while the old one is still reachable.
func (r *Runner) accountPlasticity(enabled int, pair bool) error {
	arrays := int64(4)
	if pair {
		arrays = 8
	}
	terms := [][3]int64{
		{int64(r.edges), bytesPerEdgePlasticTopology, 1},
		{int64(r.edges), bytesPerEdgePlasticWeights, 1},
		{int64(enabled), bytesPerEnabledEdgeIndex, 1},
		{int64(enabled), bytesPerEnabledEdgeValue, arrays},
	}
	total := r.accounted
	for _, term := range terms {
		product, err := checkedProduct(term[0], term[1], term[2])
		if err != nil {
			return err
		}
		if total > math.MaxInt64-product {
			return fmt.Errorf("%w: accounted bytes overflow", ErrCapacity)
		}
		total += product
	}
	if total > r.limits.MaxMemoryBytes {
		return fmt.Errorf("%w: plasticity on %d of %d edges raises the accounted bytes to %d, over the %d byte limit", ErrCapacity, enabled, r.edges, total, r.limits.MaxMemoryBytes)
	}
	r.accounted = total
	return nil
}

// PlasticState returns an independent copy of the fast state. It is the zero
// value on a runner whose protocol declared no plasticity block.
func (r *Runner) PlasticState() plasticity.State {
	if r.plastic == nil {
		return plasticity.State{}
	}
	return clonePlasticState(r.plasticState)
}

// RestorePlasticState replaces the fast state after validating it against the
// declared rule and edge selection, which is what learned_then_frozen hands the
// frozen cell. A rejected state leaves the existing one untouched.
func (r *Runner) RestorePlasticState(s plasticity.State) error {
	if r.plastic == nil {
		return fmt.Errorf("simulate: this run declares no plasticity block, so it holds no fast state")
	}
	if err := r.plastic.ValidateState(s); err != nil {
		return fmt.Errorf("simulate: %w", err)
	}
	r.plasticState = clonePlasticState(s)
	return nil
}

func clonePlasticState(s plasticity.State) plasticity.State {
	clone := plasticity.State{
		Eligibility: append([]float64(nil), s.Eligibility...),
		Plastic:     append([]float64(nil), s.Plastic...),
	}
	if s.PreTrace != nil {
		clone.PreTrace = append([]float64(nil), s.PreTrace...)
	}
	if s.PostTrace != nil {
		clone.PostTrace = append([]float64(nil), s.PostTrace...)
	}
	return clone
}

// plasticAssumption is the sentence the report adds to its assumptions when
// the block is present. It names the rule, its constants, the gate and what a
// fixed sign edge does, because the numbers in the plasticity block mean
// nothing without them. A run without the block adds nothing, so its
// assumptions are byte identical to what they were before this field existed.
func (r *Runner) plasticAssumption() string {
	rule := r.plasticRule
	line := fmt.Sprintf("Local plasticity was enabled on %d of %d edges under the rule %q with decay_e %v, decay_p %v, plastic_max %v and w_min %v; the learning gate of each step is gate_scale %v times channel %d of the declared stimulus. The weights the core integrated were recomputed before every step as the parameter set's weight plus that edge's bounded fast change, and a fixed sign edge was held at w_min instead of being allowed to cross zero. Fast changes are never written back into the parameter set. The rule, its four constants and the gate are declared engineering assumptions, not measured plasticity of the animal.",
		r.plastic.Edges(), r.edges, rule.Kind, rule.DecayE, rule.DecayP, rule.PlasticMax, rule.WMin, r.plasticScale, r.plasticGate)
	if rule.Kind == plasticity.RuleSTDPPair {
		line += fmt.Sprintf(" The pair rule additionally used decay_pre %v, decay_post %v, a_plus %v and a_minus %v.", rule.DecayPre, rule.DecayPost, rule.APlus, rule.AMinus)
	}
	if r.plasticFrozen {
		line += " Updates were frozen for this run: the fast changes the run started with were applied to every step and none was changed, which is what the learned_then_frozen cell of a comparison runs."
	}
	return line
}

// l2Norm is the Euclidean norm of one fast change array, summed in index order
// so two identical runs report the same bits.
func l2Norm(values []float64) float64 {
	sum := 0.0
	for _, v := range values {
		sum += v * v
	}
	return math.Sqrt(sum)
}
