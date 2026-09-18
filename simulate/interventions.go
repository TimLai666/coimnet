package simulate

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/TimLai666/coimnet/dynamics"
)

// Intervention kinds the native runner accepts. They are the node kinds of
// learning/intervention.go (主規格 11.8); the channel, region and delay kinds
// there are refused here because the native runner has no chemistry, no
// regions and no delayed edges.
const (
	InterventionKindClampVoltage = "clamp_voltage"
	InterventionKindForceSpike   = "force_spike"
	InterventionKindSilence      = "silence"
)

// Intervention is one authorised override applied on the rows [Start, End). It
// repeats the declaration record of learning.Intervention with the same JSON
// tags and field order, so a plan a notebook writes for the learning path is
// byte-identical here; of the fields, only Kind, Targets, Value, Start and End
// have an effect on the native runner. Channel, Regions, Restore and Seed are
// carried only so the record does not change shape between the two paths.
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
// non-blank: ordinary inference never clamps state, an experiment must say so,
// exactly as on the learning path.
type InterventionPlan struct {
	Authorized bool           `json:"authorized"`
	Reason     string         `json:"reason"`
	Items      []Intervention `json:"items"`
}

// validateInterventions checks everything about the block that does not need a
// graph: the authorisation, a nonempty item list, the supported kinds, node
// targets that are non-negative and strictly increasing, nonempty intervals
// with finite values, and that no two items of the same kind overwrite the same
// node at the same time. The upper bound on the node indices is checked at
// Build, where alone the node count is known.
func (p Protocol) validateInterventions() error {
	plan := p.Interventions
	if plan == nil {
		return nil
	}
	if !plan.Authorized || strings.TrimSpace(plan.Reason) == "" {
		return errors.New("simulate: interventions need an authorized plan with a reason")
	}
	if len(plan.Items) == 0 {
		return errors.New("simulate: an intervention plan must declare at least one item")
	}
	for i, item := range plan.Items {
		switch item.Kind {
		case InterventionKindClampVoltage, InterventionKindForceSpike, InterventionKindSilence:
			if len(item.Targets) == 0 {
				return fmt.Errorf("simulate: interventions.items[%d] kind %q needs at least one target node", i, item.Kind)
			}
			for k, node := range item.Targets {
				if node < 0 {
					return fmt.Errorf("simulate: interventions.items[%d] target %d is negative", i, k)
				}
				if k > 0 && node <= item.Targets[k-1] {
					return fmt.Errorf("simulate: interventions.items[%d] targets must be strictly increasing", i)
				}
			}
		default:
			return fmt.Errorf("simulate: interventions.items[%d] kind %q is not supported on the native runner; only clamp_voltage, force_spike and silence are", i, item.Kind)
		}
		if item.Start >= item.End {
			return fmt.Errorf("simulate: interventions.items[%d] interval [%d, %d) is empty", i, item.Start, item.End)
		}
		if !finite(item.Value) {
			return fmt.Errorf("simulate: interventions.items[%d] value %v is not finite", i, item.Value)
		}
	}
	return p.checkInterventionConflicts()
}

// checkInterventionConflicts enforces that two items of the same kind never
// overwrite the same node at the same time. Rows overlap when a.Start <
// b.End and b.Start < a.End; two intervals next to each other never do.
func (p Protocol) checkInterventionConflicts() error {
	items := p.Interventions.Items
	for i, a := range items {
		for j := i + 1; j < len(items); j++ {
			b := items[j]
			if a.Kind != b.Kind || !intervalsOverlap(a.Start, a.End, b.Start, b.End) {
				continue
			}
			shared := false
			for _, na := range a.Targets {
				for _, nb := range b.Targets {
					if na == nb {
						shared = true
						break
					}
				}
				if shared {
					break
				}
			}
			if shared {
				return fmt.Errorf("simulate: interventions.items[%d] and items[%d] both %q overlap in time on the same target", i, j, a.Kind)
			}
		}
	}
	return nil
}

func intervalsOverlap(aStart, aEnd, bStart, bEnd uint64) bool {
	return aStart < bEnd && bStart < aEnd
}

// interventionConfig is the resolved interventions block of one runner. The
// activation and the silence voltage belong to the continuous core and are set
// only there; the reset and the refractory length belong to the spiking core.
// The pointer is nil on a run without the block, so an ordinary run takes no
// intervention path at all.
type interventionConfig struct {
	reason          string
	items           []Intervention
	activation      func(float64) float64
	silenceVoltage  float64
	vReset          float64
	refractorySteps int
}

// InterventionEntry is one declared item as one Run applied it. Start and End
// repeat the declared row window, and AppliedSteps is the overlap of that
// window with the steps of this Run call, so a declaration that starts after
// the call ends visibly applied nothing instead of being silently truncated.
type InterventionEntry struct {
	Kind         string `json:"kind"`
	Targets      []int  `json:"targets"`
	Start        uint64 `json:"start"`
	End          uint64 `json:"end"`
	AppliedSteps uint64 `json:"applied_steps"`
}

// InterventionReport is what one Run did with the declared plan. Reason repeats
// the authorisation, and Entries carries one record per declared item so an
// experiment can be read from the report alone. The field is absent from a run
// without the block, keeping ordinary reports byte identical to what they were
// before the field existed.
type InterventionReport struct {
	Reason  string              `json:"reason"`
	Entries []InterventionEntry `json:"entries"`
}

// enableInterventions resolves the declared plan against this runner's core.
// It fixes the chunk size at one step, exactly as plasticity does: an override
// must apply to the step whose probes and continuation it changes, so the core
// can no longer be handed a whole chunk. It is the last construction step, so
// a refused plan leaves nothing behind.
func (r *Runner) enableInterventions(protocol Protocol) error {
	plan := protocol.Interventions
	if plan == nil {
		return nil
	}
	info := &interventionConfig{reason: plan.Reason}
	silence := false
	for _, item := range plan.Items {
		for _, node := range item.Targets {
			if node < 0 || node >= r.nodes {
				return fmt.Errorf("simulate: interventions kind %q targets node %d outside [0,%d)", item.Kind, node, r.nodes)
			}
		}
		switch item.Kind {
		case InterventionKindForceSpike:
			if !r.core.spiking() {
				return fmt.Errorf("simulate: interventions kind %q needs a spiking core; the %q core emits no events", item.Kind, r.core.name())
			}
		case InterventionKindSilence:
			silence = true
		}
		info.items = append(info.items, item)
	}
	if r.core.spiking() {
		config := protocol.LIF
		info.vReset = config.VReset
		info.refractorySteps = config.RefractorySteps
	} else {
		// Silencing an output means forcing it to the voltage whose activation
		// is zero; tanh has one at 0, the softplus activation has none.
		switch protocol.Continuous.Activation {
		case "tanh":
			info.activation = math.Tanh
			info.silenceVoltage = 0
		default:
			info.activation = continuousSoftplus
			if silence {
				return fmt.Errorf("simulate: interventions kind %q: silence needs an activation with a zero; the activation %q has none", InterventionKindSilence, protocol.Continuous.Activation)
			}
		}
	}
	r.interventions = info
	r.maxChunk = 1
	return nil
}

// continuousSoftplus is the stable form of log(1+exp(v)) the dynamics core
// activates with, replicated here so a clamp writes the same output the core
// would have.
func continuousSoftplus(v float64) float64 {
	return math.Max(v, 0) + math.Log1p(math.Exp(-math.Abs(v)))
}

// applyInterventions forces the declared overrides onto one chunk of a run.
// The chunk is one step, so start indexes the step its single row records.
// Every override lands in three places at once: the output and the event rows
// the probes and the monitors read, and the continuation state, which is what
// the next step computes from. The thresholds and the stability flags are
// observations of the run and are left alone.
func (r *Runner) applyInterventions(outputs, spikes [][]float64, next StateSnapshot, start int) error {
	info := r.interventions
	if info == nil {
		return nil
	}
	for t := range outputs {
		step := uint64(start + t)
		for _, item := range info.items {
			if !intervalsOverlap(step, step+1, item.Start, item.End) {
				continue
			}
			switch {
			case next.LIF != nil:
				if err := r.applyInterventionLIF(info, item, outputs[t], spikes[t], next.LIF); err != nil {
					return err
				}
			case next.Continuous != nil:
				if err := r.applyInterventionContinuous(info, item, outputs[t], next.Continuous); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (r *Runner) applyInterventionLIF(info *interventionConfig, item Intervention, output, spikes []float64, state *dynamics.LIFState) error {
	trace := state.History[len(state.History)-1]
	for _, node := range item.Targets {
		switch item.Kind {
		case InterventionKindClampVoltage:
			// The clamp forces the membrane. The event and the trace keep the
			// values the core computed, so the probes still read the activity.
			state.Voltage[node] = item.Value
		case InterventionKindForceSpike:
			// A step that already fired carries its event in the trace; only a
			// silent step is forced, and forcing it means the event, one more in
			// the trace and the reset the event would have produced.
			if spikes[node] != 0 {
				continue
			}
			output[node]++
			spikes[node] = 1
			state.Voltage[node] = info.vReset
			state.Refractory[node] = info.refractorySteps
			trace[node] = output[node]
		case InterventionKindSilence:
			// The neuron contributes nothing this step: no trace, no event. The
			// membrane is left where the core put it.
			output[node] = 0
			spikes[node] = 0
			trace[node] = 0
		default:
			return fmt.Errorf("simulate: interventions kind %q is not supported on the native runner", item.Kind)
		}
	}
	return nil
}

func (r *Runner) applyInterventionContinuous(info *interventionConfig, item Intervention, output []float64, state *dynamics.State) error {
	history := state.History[len(state.History)-1]
	for _, node := range item.Targets {
		switch item.Kind {
		case InterventionKindClampVoltage:
			// The continuous output is the activation of the voltage, so the
			// clamp changes both, and the persisted output must stay the
			// activation of the persisted voltage for the state to validate.
			output[node] = info.activation(item.Value)
			state.Voltage[node] = item.Value
			history[node] = output[node]
		case InterventionKindSilence:
			output[node] = 0
			state.Voltage[node] = info.silenceVoltage
			history[node] = 0
		default:
			return fmt.Errorf("simulate: interventions kind %q is not supported on the native runner", item.Kind)
		}
	}
	return nil
}

// interventionReport records what one Run applied. AppliedSteps counts the
// steps of this call, not of the whole trajectory, because the runner advances
// per call and the declaration is per Run.
func (r *Runner) interventionReport(steps int) *InterventionReport {
	info := r.interventions
	entries := make([]InterventionEntry, 0, len(info.items))
	for _, item := range info.items {
		entries = append(entries, InterventionEntry{
			Kind:         item.Kind,
			Targets:      append([]int(nil), item.Targets...),
			Start:        item.Start,
			End:          item.End,
			AppliedSteps: overlapSteps(item.Start, item.End, uint64(steps)),
		})
	}
	return &InterventionReport{Reason: info.reason, Entries: entries}
}

// overlapSteps is the length of [start, end) inside [0, limit): the steps of a
// Run call a declared row window actually covers.
func overlapSteps(start, end, limit uint64) uint64 {
	if start >= end || start >= limit {
		return 0
	}
	if end > limit {
		end = limit
	}
	return end - start
}
