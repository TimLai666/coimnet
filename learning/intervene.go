package learning

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"

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
// without a record. The deltas are the whole-call magnitudes, repeated on every
// entry so a record of one item never hides the movement the run caused.
//
//	ActivityDelta        L2 of the readout series against an in-silico twin
//	PlasticDelta         L2 of the eligibility/plastic/traces of the fast state
//	BaseParameterDelta   L2 of the flat learnable parameters (always zero)
//	ConcentrationDelta   L2 of the per-row concentration trajectory: zero while
//	                     the chemistry is disabled or the trajectories agree
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
	ConcentrationDelta float64 `json:"concentration_delta,omitempty"`
}

// rowOverride is the per-row hook the stepwise advance runs after each core
// step, so an override lands exactly on the step whose output and continuation
// it changes, and before the plastic update reads the event and the trace. Row
// is the zero-based index within the submitted input sequence. Owned by the
// advance loop: a nil hook is the unchanged path.
type rowOverride func(row uint64, next NeuralState, values, spikes []float64) error

// Intervene runs one authorized plan over the submitted input sequence and
// returns the readout plus a record of what each item applied and moved. The
// node kinds clamp_voltage, force_spike and silence act on the per-row output;
// the channel kinds block_channel, fix_concentration and remove_channel and
// the swap_regions land on the row of the chemical layer when it is enabled;
// and shuffle_delays permutes the edge delays for the whole call. The run is
// compared against an in-silico twin restored from the same starting snapshot
// and advanced without the plan, and the deltas of every entry report that
// difference.
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
	// A shuffle interchange the whole call must hold under the permuted delays,
	// so the window check and the network swap sit before anything advances and
	// the snapshot is taken before the swap, keeping the twin on the original
	// delays.
	shuffled, permuted, err := shufflePlanConfig(plan, c, len(input))
	if err != nil {
		return nil, empty, err
	}
	callOK := false
	if permuted {
		// The permuted core owns a different configuration fingerprint, so the
		// pre-run state cannot be carried into it as it is: the state is
		// re-labelled with the permuted hash, the history and the step count
		// staying exactly where they were, and the call runs under the swapped
		// network. Once the call committed, the delays return and the run state
		// is re-labelled back to the original hash, so the next ordinary call
		// continues from the same voltage, history and step count it inherited.
		// A failed call restores the initial state untouched.
		held := i.trainer.network
		heldState := copyNeural(i.neural)
		permutedNetwork, err := NewNetwork(shuffled)
		if err != nil {
			return nil, empty, err
		}
		fresh, err := permutedNetwork.core.newState(stateVoltage(heldState))
		if err != nil {
			return nil, empty, err
		}
		i.trainer.network = permutedNetwork
		i.neural = rehashState(heldState, stateHash(fresh))
		defer func() {
			i.trainer.network = held
			if callOK {
				i.neural = rehashState(i.neural, stateHash(heldState))
			} else {
				i.neural = heldState
			}
		}()
	}
	applied := make([]uint64, len(plan.Items))
	runtime := &rowOverrideRuntime{items: plan.Items, config: c, activation: continuousActivation(c.Dynamics.Activation), applied: applied, base: neuralSteps(snap.Neural)}
	i.rowOverride = runtime.apply
	defer func() { i.rowOverride = nil }()
	var runConc [][]float64
	if i.chemical != nil {
		runtime.record = &runConc
		i.chemical.override = runtime.applyChemical
		defer func() { i.chemical.override = nil }()
	}
	out, _, err := i.advanceRows(ctx, input, nil)
	if err != nil {
		return nil, empty, err
	}
	if permuted {
		callOK = true
	}
	twin, err := RestoreIndividual(snap)
	if err != nil {
		return nil, empty, err
	}
	var twinOut [][]float64
	var twinConc [][]float64
	if i.chemical != nil && twin.chemical != nil {
		// The twin records its own concentration trajectory under a hook that
		// applies nothing, so the two trajectories share one shape.
		twin.chemical.override = recordConcentration(&twinConc)
		twinOut, _, err = twin.advanceRows(ctx, input, nil)
		twin.chemical.override = nil
	} else {
		twinOut, err = twin.Advance(ctx, input)
	}
	if err != nil {
		return nil, empty, err
	}
	var plasticA, plasticB []float64
	if i.plastic != nil {
		plasticA = flattenPlastic(i.plastic.state)
		plasticB = flattenPlastic(twin.plastic.state)
	}
	var concentrationDelta float64
	if len(runConc) > 0 && len(twinConc) > 0 {
		concentrationDelta = l2Diff(flatten(runConc), flatten(twinConc))
	}
	for k, item := range plan.Items {
		if item.Kind == InterventionShuffleDelays {
			applied[k] = uint64(len(input))
		}
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
			ConcentrationDelta: concentrationDelta,
		})
	}
	return out, log, nil
}

// checkNodeInterventions rejects, before the run, the configurations the node
// hooks cannot answer for: a mixed core whose per-node rules the row hook would
// have to scatter back into, and a continuous silence on an activation that
// never reaches zero. The channel and structure kinds go through their own
// hooks and are checked here only for their declared bounds.
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
// was built from, the activation of its continuous core, the per-item count of
// applied rows its hooks are filling in, the model step of the first row of the
// call and the concentration trajectory it records when the chemistry is on.
type rowOverrideRuntime struct {
	items      []Intervention
	config     Config
	activation func(float64) float64
	applied    []uint64
	base       uint64
	record     *[][]float64
}

func (r *rowOverrideRuntime) apply(row uint64, next NeuralState, values, spikes []float64) error {
	for k, item := range r.items {
		// The chemistry and structure kinds act through their own hooks; only
		// the node kinds land on the core step row.
		switch item.Kind {
		case InterventionClampVoltage, InterventionForceSpike, InterventionSilence:
		default:
			continue
		}
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

// applyChemical is the per-row chemistry hook of one Intervene call: it lands
// the four chemistry kinds on the phase of the row they change and appends the
// concentration of every row of the call to the recording, so the report can
// state the L2 trajectory movement against the twin. Rows are indexed by the
// model step, which includes this individual's own step count before the call.
func (r *rowOverrideRuntime) applyChemical(row *chemicalRow, phase chemicalPhase, release [][]float64, step uint64) error {
	rowNo := int64(step) - int64(r.base)
	switch phase {
	case chemicalPreKinetics:
		for k, item := range r.items {
			if rowNo < int64(item.Start) || rowNo >= int64(item.End) {
				continue
			}
			switch item.Kind {
			case InterventionRemoveChannel:
				// The channel releases nothing on this row: the grid the
				// kinetics integrates and the row's own release total both say
				// zero, so the report never hides a removed release.
				for region := range release {
					release[region][item.Channel] = 0
				}
				row.release[item.Channel] = 0
				r.applied[k]++
			}
		}
	case chemicalPostKinetics:
		for k, item := range r.items {
			if rowNo < int64(item.Start) || rowNo >= int64(item.End) {
				continue
			}
			switch item.Kind {
			case InterventionFixConcentration:
				// The fixed value replaces the whole channel on every region.
				// The copy keeps the edit off the row's input state and the
				// variables the caller owns.
				row.state.Concentration = copyRows(row.state.Concentration)
				for region := range row.state.Concentration {
					row.state.Concentration[region][item.Channel] = item.Value
				}
				r.applied[k]++
			case InterventionSwapRegions:
				row.state.Concentration = copyRows(row.state.Concentration)
				a, b := item.Regions[0], item.Regions[1]
				row.state.Concentration[a], row.state.Concentration[b] = row.state.Concentration[b], row.state.Concentration[a]
				r.applied[k]++
			}
		}
		if r.record != nil {
			// The recording reads the row's final values, so the fixed and the
			// swapped rows report what the twin could not reach.
			*r.record = append(*r.record, concentrationFlat(row.state.Concentration))
		}
	case chemicalPostOccupancy:
		for k, item := range r.items {
			if rowNo < int64(item.Start) || rowNo >= int64(item.End) {
				continue
			}
			switch item.Kind {
			case InterventionBlockChannel:
				// A blocked channel reads occupancy zero on every cell of every
				// region, so its contribution to the modulation is the neutral
				// one while the record itself stays un-counted.
				for rec := range row.occupancy {
					if row.occupancy[rec].Channel == item.Channel {
						row.occupancy[rec].Occupancy = 0
					}
				}
				r.applied[k]++
			}
		}
	}
	return nil
}

// recordConcentration is the recording-only chemistry hook of an in-silico
// twin, which applies nothing and appends the concentration of every row.
func recordConcentration(mat *[][]float64) chemicalOverride {
	return func(row *chemicalRow, phase chemicalPhase, _ [][]float64, _ uint64) error {
		if phase != chemicalPostKinetics {
			return nil
		}
		*mat = append(*mat, concentrationFlat(row.state.Concentration))
		return nil
	}
}

// concentrationFlat turns one row of the concentration matrix into the flat
// series the section delta is the L2 of.
func concentrationFlat(c [][]float64) []float64 {
	count := 0
	for _, region := range c {
		count += len(region)
	}
	out := make([]float64, 0, count)
	for _, region := range c {
		out = append(out, region...)
	}
	return out
}

// shufflePlanConfig applies every shuffle_delays item of a plan to an
// independent copy of the core configuration. A shuffle interchanges the whole
// call: the delays it permutes must hold for every row, so a window smaller
// than the submitted sequence is refused before anything moves. permuted
// reports whether any item actually permuted a delay.
func shufflePlanConfig(plan InterventionPlan, c Config, rows int) (Config, bool, error) {
	var out Config
	permuted := false
	for _, item := range plan.Items {
		if item.Kind != InterventionShuffleDelays {
			continue
		}
		if item.Start != 0 || item.End != uint64(rows) {
			return c, permuted, fmt.Errorf("learning: shuffle_delays needs the whole call, window [%d, %d) on %d rows", item.Start, item.End, rows)
		}
		if !permuted {
			out = cloneConfig(c)
			permuted = true
		}
		delays := append([]int(nil), configDelays(out)...)
		positions := item.Targets
		if len(positions) == 0 {
			// A nil target list shuffles every edge whose delay is positive,
			// the only values a permutation can actually move.
			positions = make([]int, 0, len(delays))
			for k, delay := range delays {
				if delay > 0 {
					positions = append(positions, k)
				}
			}
			if len(positions) == 0 {
				return c, permuted, fmt.Errorf("learning: shuffle_delays has no delayed edge to shuffle")
			}
		}
		values := make([]int, len(positions))
		for p, pos := range positions {
			values[p] = delays[pos]
		}
		rand.New(rand.NewPCG(item.Seed, 0)).Shuffle(len(values), func(a, b int) { values[a], values[b] = values[b], values[a] })
		for p, pos := range positions {
			delays[pos] = values[p]
		}
		configSetDelays(&out, delays)
	}
	return out, permuted, nil
}

// cloneConfig deep copies the core configuration and its edge-delay list, so a
// shuffle never mutates the individual's own configuration.
func cloneConfig(c Config) Config {
	out := c
	delays := append([]int(nil), configDelays(c)...)
	switch {
	case c.Mixed != nil:
		out.Mixed = copyMixed(c.Mixed)
	case c.LIF != nil:
		out.LIF = copyLIF(c.LIF)
	default:
		out.Dynamics = c.Dynamics
	}
	configSetDelays(&out, delays)
	return out
}

// configDelays reads the edge-delay list of whichever core a configuration
// declares, and configSetDelays writes one back.
func configDelays(c Config) []int {
	if c.Mixed != nil {
		return c.Mixed.Delays
	}
	if c.LIF != nil {
		return c.LIF.Delays
	}
	return c.Dynamics.Delays
}

func configSetDelays(c *Config, delays []int) {
	if c.Mixed != nil {
		c.Mixed.Delays = delays
		return
	}
	if c.LIF != nil {
		c.LIF.Delays = delays
		return
	}
	c.Dynamics.Delays = delays
}

// stateVoltage extracts the full node-width voltage of a neural state, the
// only input a fresh state of another configuration needs.
func stateVoltage(s NeuralState) []float64 {
	switch {
	case s.Continuous != nil:
		return s.Continuous.Voltage
	case s.LIF != nil:
		return s.LIF.Voltage
	default:
		nodes := len(s.Mixed.Index.ContinuousNodes) + len(s.Mixed.Index.LIFNodes)
		full := make([]float64, nodes)
		for k, node := range s.Mixed.Index.ContinuousNodes {
			full[node] = s.Mixed.Continuous.Voltage[k]
		}
		for k, node := range s.Mixed.Index.LIFNodes {
			full[node] = s.Mixed.LIF.Voltage[k]
		}
		return full
	}
}

// stateHash is the configuration fingerprint a neural state claims to belong
// to, the top-level hash of whichever half the state carries.
func stateHash(s NeuralState) string {
	switch {
	case s.Continuous != nil:
		return s.Continuous.ConfigHash
	case s.LIF != nil:
		return s.LIF.ConfigHash
	default:
		return s.Mixed.ConfigHash
	}
}

// rehashState re-labels an owned copy of a state to another configuration's
// fingerprint without recomputing its trajectory: a delay permutation changes
// nothing a state carries but the fingerprint, because the voltage, the step
// count and the history rows are values rather than configuration. The mixed
// state repeats the shared hash on both halves, so all three are relabelled.
func rehashState(s NeuralState, hash string) NeuralState {
	owned := copyNeural(s)
	switch {
	case owned.Continuous != nil:
		owned.Continuous.ConfigHash = hash
	case owned.LIF != nil:
		owned.LIF.ConfigHash = hash
	default:
		owned.Mixed.ConfigHash = hash
		owned.Mixed.Continuous.ConfigHash = hash
		owned.Mixed.LIF.ConfigHash = hash
	}
	return owned
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
