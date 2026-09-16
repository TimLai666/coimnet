package learning

import (
	"fmt"
	"math"
	"strings"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/signal"
)

// ChemistryReport is what one advance did to the chemical layer. Steps counts
// the rows that ran under it, Concentration is the state after the last of them
// and Occupancy holds that last row's records, one per receptor per cell.
//
// Assumed, UnknownSkipped and Unresponsive describe those same last-row
// records, because they count the records in Occupancy and not the rows that
// produced them. ClampedGamma, ClampedBeta and ClampedTheta are the opposite:
// a bound holds a value on the row it holds it, so they are summed over every
// row of the call, the way PlasticReport counts its own bounds.
//
// ReleaseTotal is the release each channel received, summed over the rows of
// this call. Sources are global to every region in this stage, so a channel's
// rate is counted once per row rather than once per region.
//
// The zero value is what a disabled individual reports.
type ChemistryReport struct {
	Steps          int                          `json:"steps"`
	Concentration  [][]float64                  `json:"concentration,omitempty"`
	Occupancy      []modulation.OccupancyRecord `json:"occupancy,omitempty"`
	Assumed        int                          `json:"assumed"`
	UnknownSkipped int                          `json:"unknown_skipped"`
	Unresponsive   int                          `json:"unresponsive"`
	ClampedGamma   int                          `json:"clamped_gamma"`
	ClampedBeta    int                          `json:"clamped_beta"`
	ClampedTheta   int                          `json:"clamped_theta"`
	ReleaseTotal   []float64                    `json:"release_total,omitempty"`
}

// ChemicalPart is the chemical layer of one individual at a completed
// operation boundary: the declaration it was enabled with, the concentration
// that has accumulated since, the resources the task declared and the feedback
// that is still queued. It is absent, not zero filled, while the layer was
// never enabled, so a document written before it existed stays readable and a
// reader can tell "off" from "on with nothing released yet".
//
// The activity a source averages over is not stored here: it is the node
// outputs of the previous step, which the persistent neural history already
// holds, so a restored individual reads the same row the uninterrupted one
// would have read.
type ChemicalPart struct {
	Config          modulation.ChemistryConfig `json:"config"`
	State           modulation.ChemistryState  `json:"state"`
	Resources       map[string]float64         `json:"resources"`
	PendingFeedback []signal.FeedbackSpec      `json:"pending_feedback"`
}

// chemicalRuntime is the enabled chemical layer of one individual: nil means
// the layer is off and the forward path is the one that existed before it.
type chemicalRuntime struct {
	config    modulation.ChemistryConfig
	kinetics  *modulation.Kinetics
	sources   []modulation.Source
	state     modulation.ChemistryState
	resources map[string]float64
	pending   []signal.Feedback
	// report describes the most recent successful advance. A refused advance
	// commits nothing, so it leaves the previous report in place.
	report ChemistryReport
}

// newChemicalRuntime validates one declaration against a node count and builds
// the kinetics and the sources it names. Every refusal happens here rather than
// at the first step.
func newChemicalRuntime(c modulation.ChemistryConfig, nodes int) (*chemicalRuntime, error) {
	if err := c.Validate(nodes); err != nil {
		return nil, err
	}
	owned := c.Clone()
	kinetics, err := modulation.NewChemistry(owned.Chemistry)
	if err != nil {
		return nil, err
	}
	sources := make([]modulation.Source, len(owned.Sources))
	for k, spec := range owned.Sources {
		if sources[k], err = spec.Build(); err != nil {
			return nil, fmt.Errorf("source %d: %w", k, err)
		}
	}
	return &chemicalRuntime{
		config:    owned,
		kinetics:  kinetics,
		sources:   sources,
		state:     kinetics.NewState(),
		resources: map[string]float64{},
	}, nil
}

// chemicalRow is one row of the chemical layer, produced before the core step
// of that same row and discarded after it.
type chemicalRow struct {
	state     modulation.ChemistryState
	occupancy []modulation.OccupancyRecord
	summary   modulation.OccupancySummary
	clamped   modulation.ClampReport
	release   []float64
	modulated *dynamics.Modulation
}

// advanceOne runs the fixed order of one row: every source releases, the
// concentration takes one step, the receptors report their occupancy against
// the new concentration, and the effects turn that occupancy into the per-node
// arrays the core reads for this one row.
//
// activity is the node outputs of the previous step and is nil on the very
// first row of a fresh individual. step is that row's position on the
// model-step clock, which is the individual's own step count; it is both the
// step a source releases for and the "now" that feedback is filtered against.
//
// Nothing here is committed: the caller owns both the state it passes in and
// the one it gets back until the whole advance has succeeded.
func (r *chemicalRuntime) advanceOne(state modulation.ChemistryState, step uint64, activity []float64, nodes int) (chemicalRow, error) {
	var row chemicalRow
	if step > math.MaxInt64 {
		return row, fmt.Errorf("step %d does not fit an int64 timestamp", step)
	}
	now, err := signal.NewTimestamp(int64(step), modulation.ReleaseTimeUnit)
	if err != nil {
		return row, err
	}
	available, err := signal.AvailableFeedback(r.pending, now)
	if err != nil {
		return row, err
	}
	context := modulation.SourceContext{Activity: activity, Feedback: available, Resources: r.resources}
	regions, channels := r.config.Chemistry.Regions, r.config.Chemistry.Channels
	release := make([][]float64, regions)
	for region := range release {
		release[region] = make([]float64, channels)
	}
	row.release = make([]float64, channels)
	for k, source := range r.sources {
		rates, err := source.Release(step, context)
		if err != nil {
			return chemicalRow{}, fmt.Errorf("source %d at step %d: %w", k, step, err)
		}
		for j, rate := range rates {
			if j != k && rate != 0 {
				return chemicalRow{}, fmt.Errorf("the source of channel %d released %v on channel %d at step %d", k, rate, j, step)
			}
		}
		// Sources are global to every region in this stage: the rate a channel
		// receives arrives in all of them. Per-region sources are a later
		// decision and would change this loop and nothing else.
		for region := range release {
			release[region][k] = rates[k]
		}
		row.release[k] = rates[k]
	}
	if row.state, err = r.kinetics.Step(state, release, nil); err != nil {
		return chemicalRow{}, err
	}
	receptors := r.config.Receptors
	if row.occupancy, row.summary, err = receptors.Occupancies(row.state, r.config.Regions.NodeRegion); err != nil {
		return chemicalRow{}, err
	}
	gain, offset, threshold, clamped, err := modulation.ApplyEffects(r.config.Effects, row.occupancy, receptors.Mix, nodes)
	if err != nil {
		return chemicalRow{}, err
	}
	row.clamped = clamped
	row.modulated = &dynamics.Modulation{
		Gain:      [][]float64{gain},
		Offset:    [][]float64{offset},
		Threshold: [][]float64{threshold},
	}
	return row, nil
}

// EnableChemistry turns the chemical layer on for this individual. The whole
// declaration is validated against this individual's own node count, and a
// second call replaces it and restarts the concentration, because a
// concentration accumulated under different regions, channels or time constants
// does not describe the new declaration. The resources the task declared and
// the feedback still queued are kept: they are inputs of the task, not chemical
// state.
//
// The layer lives on the persistent individual only. An independent training
// episode has no access to it, so there is no way to enable it on a
// non-persistent path.
func (i *Individual) EnableChemistry(c modulation.ChemistryConfig) error {
	if i == nil || i.trainer == nil {
		return fmt.Errorf("uninitialized individual")
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	runtime, err := newChemicalRuntime(c, configNodes(i.trainer.network.config))
	if err != nil {
		return err
	}
	if i.chemical != nil {
		runtime.resources = i.chemical.resources
		runtime.pending = i.chemical.pending
	}
	i.chemical = runtime
	return nil
}

// DisableChemistry turns the layer off and drops the concentration, the
// declared resources and the queued feedback, so the forward path returns bit
// for bit to the one an individual that never enabled it produces. Base
// parameters were never changed by it and are unaffected.
func (i *Individual) DisableChemistry() {
	if i == nil || i.trainer == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.chemical = nil
}

// OfferFeedback queues one feedback for the sources of the chemical layer. It
// is filtered with signal.AvailableFeedback at every step, so a feedback is
// offered to a source only from the step its own AvailableAt names onwards.
// The clock is the model step, which is the clock a release step is counted in;
// feedback on any other clock is refused rather than converted.
//
// The queue is a declaration, not a consumable: a feedback stays available once
// it has arrived, and the snapshot carries it. None of the four implemented
// sources reads a score, so nothing is lost by keeping it.
func (i *Individual) OfferFeedback(f signal.Feedback) error {
	if i == nil || i.trainer == nil {
		return fmt.Errorf("uninitialized individual")
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.chemical == nil {
		return fmt.Errorf("feedback needs an enabled chemistry")
	}
	if err := f.Validate(); err != nil {
		return fmt.Errorf("individual feedback: %w", err)
	}
	if unit := f.Spec().AvailableAt.Unit; unit != modulation.ReleaseTimeUnit {
		return fmt.Errorf("individual feedback uses time unit %q, want %q", unit, modulation.ReleaseTimeUnit)
	}
	i.chemical.pending = append(append([]signal.Feedback(nil), i.chemical.pending...), f)
	return nil
}

// SetResource declares how much of one named resource the task currently holds.
// An internal-resource source reads it by name and by the declared rule; a
// source whose resource was never declared is an error rather than a zero.
func (i *Individual) SetResource(name string, value float64) error {
	if i == nil || i.trainer == nil {
		return fmt.Errorf("uninitialized individual")
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.chemical == nil {
		return fmt.Errorf("a resource needs an enabled chemistry")
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("a resource needs a name")
	}
	if !finite(value) {
		return fmt.Errorf("resource %q must be finite, got %v", name, value)
	}
	resources := make(map[string]float64, len(i.chemical.resources)+1)
	for key, amount := range i.chemical.resources {
		resources[key] = amount
	}
	resources[name] = value
	i.chemical.resources = resources
	return nil
}

// ChemistryReport returns what the most recent advance did to the chemical
// layer, as an independent copy. A refused advance commits nothing and leaves
// the previous report in place; a disabled individual returns the zero value.
func (i *Individual) ChemistryReport() ChemistryReport {
	if i == nil || i.trainer == nil {
		return ChemistryReport{}
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.chemical == nil {
		return ChemistryReport{}
	}
	return copyChemistryReport(i.chemical.report)
}

func copyChemistryReport(r ChemistryReport) ChemistryReport {
	owned := r
	owned.Concentration = copyRows(r.Concentration)
	owned.Occupancy = append([]modulation.OccupancyRecord(nil), r.Occupancy...)
	owned.ReleaseTotal = append([]float64(nil), r.ReleaseTotal...)
	return owned
}

// copyChemicalState deep copies a concentration so a snapshot never shares a
// buffer with the running individual.
func copyChemicalState(s modulation.ChemistryState) modulation.ChemistryState {
	return modulation.ChemistryState{Concentration: copyRows(s.Concentration), Steps: s.Steps}
}

// neuralSteps reads the model-step count of whichever core an individual drives.
// It is the clock the chemical layer releases and filters feedback on.
func neuralSteps(s NeuralState) uint64 {
	if s.Continuous != nil {
		return s.Continuous.Steps
	}
	if s.LIF != nil {
		return s.LIF.Steps
	}
	return 0
}

// previousActivity is the node outputs of the step before the next one: the
// activated outputs of the continuous core and the synaptic trace of the
// spiking one, which is exactly what each of them stores as the newest row of
// its delay history. It is nil before the first step has run, because the
// prehistory row is the initial state and not an observed step.
func previousActivity(s NeuralState) []float64 {
	var steps uint64
	var history [][]float64
	switch {
	case s.Continuous != nil:
		steps, history = s.Continuous.Steps, s.Continuous.History
	case s.LIF != nil:
		steps, history = s.LIF.Steps, s.LIF.History
	default:
		return nil
	}
	if steps == 0 || len(history) == 0 {
		return nil
	}
	return append([]float64(nil), history[len(history)-1]...)
}
