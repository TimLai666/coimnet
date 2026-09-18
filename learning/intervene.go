package learning

import (
	"context"
	"fmt"
	"math"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/plasticity"
)

// InterventionLog records what one authorized Intervene call did, so an
// experiment can be read from its own return value: the declared plan, how many
// rows each item actually covered, and the effect on activity, fast plastic
// state and base parameters at the end of the run.
type InterventionLog struct {
	Reason  string                 `json:"reason"`
	Entries []InterventionLogEntry `json:"entries"`
}

// InterventionLogEntry is one declared item as one Intervene call applied it.
// Start and End repeat the declared row window; AppliedRows is the overlap of
// that window with the rows of the submitted input, so a window that ends
// before the call's sequence does silently nothing instead of being truncated
// without a record. The three deltas are the whole-call magnitudes, repeated on
// every entry so a record of one item never hides the movement the run caused.
//
//	ActivityDelta        L2 of the readout series against an in-silico twin
//	PlasticDelta         L2 of the eligibility/plastic/traces of the fast state
//	BaseParameterDelta   L2 of the flat learnable parameters (always zero)
type InterventionLogEntry struct {
	Kind        string `json:"kind"`
	Targets     []int  `json:"targets,omitempty"`
	Channel     int    `json:"channel,omitempty"`
	Start       uint64 `json:"start"`
	End         uint64 `json:"end"`
	AppliedRows uint64 `json:"applied_rows"`

	ActivityDelta      float64 `json:"activity_delta"`
	PlasticDelta       float64 `json:"plastic_delta"`
	BaseParameterDelta float64 `json:"base_parameter_delta"`
}

// rowOverride is the per-row hook the stepwise advance runs after each core
// step, so an override lands exactly on the step whose output and continuation
// it changes, and before the plastic update reads the event and the trace. Row
// is the zero-based index within the submitted input sequence. Owned by the
// advance loop: a nil hook is the unchanged path.
type rowOverride func(row uint64, next NeuralState, values, spikes []float64) error

// Intervene runs one authorized plan over the submitted input sequence and
// returns the readout plus a record of what each item applied and moved. The
// only implemented kinds are the node interventions clamp_voltage, force_spike
// and silence; every other declared kind is refused before any state moves. The
// run is compared against an in-silico twin restored from the same starting
// snapshot and advanced without the plan, and the three deltas of every entry
// report that difference.
//
// The rows of a plan index the submitted sequence of this call, so a fresh
// individual and a continuing one both read "clamp rows 10..20" as rows 10..20
// of what this call was given. Intervene spans several complete operations
// (validate, snapshot, advance, restore) and is not safe to run concurrently
// with other mutating calls on the same individual.
func (i *Individual) Intervene(ctx context.Context, plan InterventionPlan, input [][]float64) ([][]float64, InterventionLog, error) {
	var empty InterventionLog
	if i == nil || i.trainer == nil {
		return nil, empty, fmt.Errorf("uninitialized individual")
	}
	if err := plan.Validate(i.InterventionShape()); err != nil {
		return nil, empty, err
	}
	c := i.trainer.network.Config()
	if err := checkNodeInterventions(plan, c); err != nil {
		return nil, empty, err
	}
	if len(input) == 0 {
		return nil, empty, fmt.Errorf("empty input sequence")
	}
	snap := i.Snapshot()
	applied := make([]uint64, len(plan.Items))
	runtime := &rowOverrideRuntime{items: plan.Items, config: c, activation: continuousActivation(c.Dynamics.Activation), applied: applied}
	i.rowOverride = runtime.apply
	defer func() { i.rowOverride = nil }()
	out, _, err := i.advanceRows(ctx, input, nil)
	if err != nil {
		return nil, empty, err
	}
	twin, err := RestoreIndividual(snap)
	if err != nil {
		return nil, empty, err
	}
	twinOut, err := twin.Advance(ctx, input)
	if err != nil {
		return nil, empty, err
	}
	var plasticA, plasticB []float64
	if i.plastic != nil {
		plasticA = flattenPlastic(i.plastic.state)
		plasticB = flattenPlastic(twin.plastic.state)
	}
	log := InterventionLog{Reason: plan.Reason, Entries: make([]InterventionLogEntry, 0, len(plan.Items))}
	activityDelta := l2Diff(flatten(out), flatten(twinOut))
	plasticDelta := l2Diff(plasticA, plasticB)
	baseParameterDelta := l2Diff(flatParameters(i.trainer.parameters), flatParameters(twin.trainer.parameters))
	for k, item := range plan.Items {
		log.Entries = append(log.Entries, InterventionLogEntry{
			Kind: item.Kind, Targets: append([]int(nil), item.Targets...), Channel: item.Channel,
			Start: item.Start, End: item.End, AppliedRows: applied[k],
			ActivityDelta: activityDelta, PlasticDelta: plasticDelta, BaseParameterDelta: baseParameterDelta,
		})
	}
	return out, log, nil
}

// checkNodeInterventions rejects, before the run, the declared kinds the
// learning path does not implement and the configurations it cannot answer for:
// a mixed core whose per-node rules the row hook would have to scatter back
// into, and a continuous silence on an activation that never reaches zero.
func checkNodeInterventions(plan InterventionPlan, c Config) error {
	if c.Mixed != nil {
		for _, item := range plan.Items {
			switch item.Kind {
			case InterventionClampVoltage, InterventionForceSpike, InterventionSilence:
				return fmt.Errorf("learning: node interventions on a mixed core are not implemented yet")
			}
		}
	}
	for _, item := range plan.Items {
		switch item.Kind {
		case InterventionBlockChannel, InterventionFixConcentration, InterventionRemoveChannel, InterventionSwapRegions, InterventionShuffleDelays:
			return fmt.Errorf("learning: intervention kind %q is not implemented yet", item.Kind)
		case InterventionSilence:
			// The continuous half silences by writing the voltage whose
			// activation is zero; tanh has one at 0, softplus has none.
			if c.Dynamics.Activation != "" && c.Dynamics.Activation != "tanh" {
				return fmt.Errorf("learning: silence needs an activation with a zero, activation %q has none", c.Dynamics.Activation)
			}
		}
	}
	return nil
}

// rowOverrideRuntime is the mutable state one Intervene call owns: the plan it
// was built from, the activation of its continuous core and the per-item count
// of applied rows its hook is filling in.
type rowOverrideRuntime struct {
	items      []Intervention
	config     Config
	activation func(float64) float64
	applied    []uint64
}

func (r *rowOverrideRuntime) apply(row uint64, next NeuralState, values, spikes []float64) error {
	for k, item := range r.items {
		if row < item.Start || row >= item.End {
			continue
		}
		var err error
		switch {
		case next.LIF != nil:
			err = applyLIFIntervention(r.config, item, next.LIF, values, spikes)
		case next.Continuous != nil:
			err = applyContinuousIntervention(item, r.activation, next.Continuous, values)
		default:
			err = fmt.Errorf("learning: intervention kind %q on a core with no node state", item.Kind)
		}
		if err != nil {
			return err
		}
		r.applied[k]++
	}
	return nil
}

// applyLIFIntervention lands one item on the spiking half. The persisted trace
// and the event row are both updated because the next step reads the trace from
// the continuation, and a forced event a plastic rule was about to see must say
// so. Silence leaves the membrane where the core put it, exactly as the native
// runner does.
func applyLIFIntervention(c Config, item Intervention, state *dynamics.LIFState, values, spikes []float64) error {
	trace := state.History[len(state.History)-1]
	for _, node := range item.Targets {
		switch item.Kind {
		case InterventionClampVoltage:
			// The clamp forces the membrane only; the event and the trace keep
			// the values the core computed, so probes still read the activity.
			state.Voltage[node] = item.Value
		case InterventionForceSpike:
			// A step that already fired carries its event in the trace; only a
			// silent step is forced, and forcing it means the event, one more
			// unit in the trace and the reset the event would have produced.
			if spikes[node] != 0 {
				continue
			}
			values[node]++
			spikes[node] = 1
			state.Voltage[node] = c.LIF.VReset
			state.Refractory[node] = c.LIF.RefractorySteps
			trace[node] = values[node]
		case InterventionSilence:
			// The neuron contributes nothing this step: no trace, no event.
			values[node] = 0
			spikes[node] = 0
			trace[node] = 0
		default:
			return fmt.Errorf("learning: intervention kind %q is not implemented yet", item.Kind)
		}
	}
	return nil
}

// applyContinuousIntervention lands one item on the continuous half. The
// clamped or silenced voltage, its activation and the persisted history row are
// all replaced together, because the core validates that the latest history is
// the activation of the latest voltage.
func applyContinuousIntervention(item Intervention, activation func(float64) float64, state *dynamics.State, values []float64) error {
	history := state.History[len(state.History)-1]
	for _, node := range item.Targets {
		switch item.Kind {
		case InterventionClampVoltage:
			values[node] = activation(item.Value)
			state.Voltage[node] = item.Value
			history[node] = values[node]
		case InterventionSilence:
			// tanh is the only activation the planning path lets through here,
			// and tanh(0) is the zero the silence needs.
			values[node] = 0
			state.Voltage[node] = 0
			history[node] = 0
		default:
			return fmt.Errorf("learning: intervention kind %q is not implemented yet", item.Kind)
		}
	}
	return nil
}

// continuousActivation is the dynamics activation of a continuous core,
// replicated here so a clamp writes the same output the core would have.
func continuousActivation(name string) func(float64) float64 {
	if name == "softplus" {
		return softplus
	}
	return math.Tanh
}

// softplus is the stable form of log(1+exp(v)) the dynamics core activates with.
func softplus(v float64) float64 {
	return math.Max(v, 0) + math.Log1p(math.Exp(-math.Abs(v)))
}

// flattenPlastic turns the fast state into the flat series the plastic delta is
// the L2 of. PreTrace and PostTrace exist only under stdp_pair; the other rules
// carry nil slices, which flatten as nothing.
func flattenPlastic(s plasticity.State) []float64 {
	out := make([]float64, 0, len(s.Eligibility)+len(s.Plastic)+len(s.PreTrace)+len(s.PostTrace))
	out = append(out, s.Eligibility...)
	out = append(out, s.Plastic...)
	out = append(out, s.PreTrace...)
	out = append(out, s.PostTrace...)
	return out
}

// flatten turns the readout rows into one flat series.
func flatten(rows [][]float64) []float64 {
	count := 0
	for _, row := range rows {
		count += len(row)
	}
	out := make([]float64, 0, count)
	for _, row := range rows {
		out = append(out, row...)
	}
	return out
}

// l2Diff is the Euclidean distance of two equally shaped series.
func l2Diff(a, b []float64) float64 {
	var sum float64
	for k := range a {
		d := a[k] - b[k]
		sum += d * d
	}
	return math.Sqrt(sum)
}
