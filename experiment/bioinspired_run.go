package experiment

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/plasticity"
)

// ecdysoneEpisodeRows is how many rows one episode has: delay 2 plus the two
// rows the delayed-correlation episode always carries. A pulse must fall on
// one of those rows, because the timeline releases on the individual's global
// step clock and a step beyond the row span of one episode lands in a later
// episode, which would make the "timing within one learning episode" claim
// meaningless.
const ecdysoneEpisodeRows = 4

// bioInspiredIndividual is ContinualFixtureWithChemistry's declaration with the
// external timeline replaced by the single pulse of this run: one region, one
// channel, a release of 1 at pulseStep on the step clock, the hypothesized
// receptor on node 2 (Kd 0.5, N 1). The hebbian_rate local plasticity on edge
// 1 is gated by that receptor (the pulse opens the learning gate) and the slow
// consolidation layer is declared with the training budget.
func bioInspiredIndividual(seed uint64, pulseStep int) (*learning.Individual, error) {
	ind, err := ContinualFixture(seed)
	if err != nil {
		return nil, err
	}
	declaration := modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{2}},
		Sources: []modulation.SourceSpec{{
			Kind: modulation.SourceExternalTimeline, Channel: 0,
			Timeline: &modulation.ExternalTimeline{ChannelCount: 1, Entries: []modulation.TimelineEntry{{Step: uint64(pulseStep), Channel: 0, Rate: 1}}},
		}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{
			{Cells: []int{2}, Signal: "octopamine", Channel: 0, Status: modulation.StatusHypothesized, Kd: 0.5, N: 1,
				Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chem-fixture/v1"},
		}},
		Regions: modulation.RegionAssignment{NodeRegion: []int{0, 0, 0}},
	}
	if err := ind.EnableChemistry(declaration); err != nil {
		return nil, err
	}
	zero := 0
	rule := plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 1, WMin: .01, GateReceptor: &zero, GateScale: 1}
	if err := ind.EnablePlasticity(plasticity.Config{Rule: rule, Edges: []int{1}}); err != nil {
		return nil, err
	}
	return ind, nil
}

// trainEcdysoneSeed replays the training episodes of one pulse timing on one
// individual: for every episode the walk over the input first (the receptor
// gate drives the fast change), then the isolated gradient update, then one
// consolidation attempt against the receptor occupancy. The three refusals a
// write can hit are ignored exactly like the consolidation write never
// happened; anything else fails the seed. The budget is the episode count, so
// the layer can take as many writes as the run has consolidation attempts.
func trainEcdysoneSeed(ctx context.Context, ind *learning.Individual, spec TaskSpec, width int, seed uint64, episodes int) error {
	for e := 0; e < episodes; e++ {
		ep, err := ContinualEpisode(spec, width, false, trainSeed(seed, 0), uint64(e))
		if err != nil {
			return err
		}
		if _, err := ind.Advance(ctx, ep.Input); err != nil {
			return err
		}
		if _, err := ind.TrainEpisode(ctx, ep.Input, ep.Target); err != nil {
			return err
		}
		_, err = ind.Consolidate(ctx, learning.ConsolidationTrigger{Receptor: 0, Threshold: 0.1, Rate: 0.5, Retain: 0.5})
		if err == nil {
			continue
		}
		if errors.Is(err, learning.ErrConsolidationGateClosed) || errors.Is(err, learning.ErrAlreadyConsolidated) || errors.Is(err, learning.ErrBudgetExhausted) {
			continue
		}
		return err
	}
	return nil
}

// runEcdysoneInspired is the timing protocol: for every seed and every pulse
// step a fresh individual, Episodes training episodes of the fixed "a" task
// (delay 2, channel 0, width 2), then the slow-layer L2 magnitude and the
// retest score as the curve point. A pulse step outside the 4 rows of one
// episode refuses the whole run before any seed starts; a failed seed keeps its
// error and the other seeds continue; a non-nil context error aborts the run.
func runEcdysoneInspired(ctx context.Context, c BioInspiredConfig) (BioInspiredReport, error) {
	var report BioInspiredReport
	if ctx == nil {
		return report, fmt.Errorf("ecdysone_inspired run needs a context")
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	for _, step := range c.PulseSteps {
		if step >= ecdysoneEpisodeRows {
			return report, fmt.Errorf("pulse step %d is outside the episode of %d rows", step, ecdysoneEpisodeRows)
		}
	}
	spec := TaskSpec{Name: "a", Generator: GeneratorDelayedCorrelation, Params: map[string]float64{"delay": 2, "channel": 0}}
	report = BioInspiredReport{
		SchemaVersion: BioInspiredSchemaVersion,
		Protocol:      c.Protocol,
		Config:        c,
		ConfigHash:    hash(c),
		Registry:      c.RegistryIDs(),
		Assumptions:   c.assumptions(),
	}
	for _, seed := range c.Seeds {
		out := BioInspiredSeed{Seed: seed}
		for _, step := range c.PulseSteps {
			if err := ctx.Err(); err != nil {
				return report, err
			}
			ind, err := bioInspiredIndividual(seed, step)
			if err == nil {
				err = ind.EnableConsolidation(uint64(c.Episodes))
			}
			if err == nil {
				err = trainEcdysoneSeed(ctx, ind, spec, 2, seed, c.Episodes)
			}
			if err == nil {
				err = ctx.Err()
			}
			if err != nil {
				if cerr := ctx.Err(); cerr != nil {
					return report, cerr
				}
				out.Failed, out.Error = true, err.Error()
				break
			}
			magnitude := 0.0
			if slow := ind.Snapshot().Plastic.Slow; slow != nil {
				magnitude = euclidean(slow.Values)
			}
			// evaluateTask scores a twin restored from the individual's snapshot and
			// refuses any drift of the parameters, plastic or slow layers during the
			// evaluation. A local fast state would keep moving under its own
			// eligibility dynamics on the twin, so the retest measures the trained
			// base parameters only: the slow-layer magnitude was captured above from
			// the pre-drop snapshot, and this drop makes the fixed evaluateTask
			// invariant hold.
			ind.DisablePlasticity()
			score, err := evaluateTask(ctx, ind, spec, 2, false, 8, evalSeed(seed, 0), true)
			if err != nil {
				if cerr := ctx.Err(); cerr != nil {
					return report, cerr
				}
				out.Failed, out.Error = true, err.Error()
				break
			}
			out.Curve = append(out.Curve, BioInspiredPoint{PulseStep: step, SlowMagnitude: magnitude, RetestScore: score})
		}
		report.Seeds = append(report.Seeds, out)
	}
	return report, nil
}

// euclidean is the L2 norm of one vector, the magnitude each slow layer point
// reports so the report shows how much of the weight change the consolidation
// wrote at each pulse timing.
func euclidean(v []float64) float64 {
	var sum float64
	for _, x := range v {
		sum += x * x
	}
	return math.Sqrt(sum)
}
