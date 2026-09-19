package experiment

import (
	"context"
	"fmt"
	"math"
	"reflect"

	"github.com/TimLai666/coimnet/learning"
)

const (
	// npfPulseStep is the row the external timeline releases on during
	// training, so the receptor occupancy that gates the local rule is non-zero
	// while the individual learns. The evaluations replace the concentration
	// outright, so this timing only shapes the learning, never the readout.
	npfPulseStep = 1
	// npfEvaluationEpisodes is how many fresh episodes each of the three
	// evaluations scores; the same count for all three, so the scores are
	// comparable.
	npfEvaluationEpisodes = 8
)

// npfExpressionGain is the readout-only suppression this protocol declares:
// on the readout node, driven by the single receptor, the gain falls from the
// neutral 1 towards 0.1 as occupancy rises. It touches the readout alone, so
// nothing it does can be mistaken for a change of what was learned.
var npfExpressionGain = learning.ExpressionGain{Nodes: []int{2}, Receptor: 0, Scale: -1, Min: 0.1, Max: 1}

// trainNPFSeed replays the training episodes of the state protocol on one
// individual: for every episode the walk over the input first, so the pulse
// opens the receptor-gated local rule, then the isolated gradient update. The
// protocol never consolidates, because what the suppression must leave
// untouched is the fast plastic state and the base parameters themselves.
func trainNPFSeed(ctx context.Context, ind *learning.Individual, spec TaskSpec, width int, seed uint64, episodes int) error {
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
	}
	return nil
}

// npfStateUnchanged compares the consolidation layer and the fast plastic
// state of two snapshots of the same individual. A plastic part that is absent
// on both sides holds nothing that could have changed, so both checks read
// true; a part that appeared or vanished is a change.
func npfStateUnchanged(reference, final learning.IndividualSnapshot) (slowUnchanged, plasticUnchanged bool) {
	if reference.Plastic == nil || final.Plastic == nil {
		absent := reference.Plastic == nil && final.Plastic == nil
		return absent, absent
	}
	return reflect.DeepEqual(reference.Plastic.Slow, final.Plastic.Slow), reflect.DeepEqual(reference.Plastic.State, final.Plastic.State)
}

// runNPFSeed is the whole protocol on one seed: a fresh individual trained
// with the pulse on, the readout-only suppression declared afterwards, then
// three evaluations of the same task on switched chemical states — neutral,
// suppressed, neutral again. The scores come from twins, so the individual is
// never advanced by an evaluation and the closing snapshot can be compared to
// the one taken before the first score.
func runNPFSeed(ctx context.Context, c BioInspiredConfig, spec TaskSpec, seed uint64) (*ExpressionResult, error) {
	ind, err := bioInspiredIndividual(seed, npfPulseStep)
	if err != nil {
		return nil, err
	}
	if err := trainNPFSeed(ctx, ind, spec, 2, seed, c.Episodes); err != nil {
		return nil, err
	}
	if err := ind.SetExpressionGain(npfExpressionGain); err != nil {
		return nil, err
	}
	reference := ind.Snapshot()
	neutral := [][]float64{{0}}
	suppressed := [][]float64{{c.Suppressed}}
	score := func(conc [][]float64) (float64, error) {
		return evaluateSwitched(ctx, ind, spec, 2, false, npfEvaluationEpisodes, evalSeed(seed, 0), conc)
	}
	before, err := score(neutral)
	if err != nil {
		return nil, err
	}
	during, err := score(suppressed)
	if err != nil {
		return nil, err
	}
	after, err := score(neutral)
	if err != nil {
		return nil, err
	}
	final := ind.Snapshot()
	slowUnchanged, plasticUnchanged := npfStateUnchanged(reference, final)
	return &ExpressionResult{
		Before:              before,
		Suppressed:          during,
		After:               after,
		Recovered:           math.Abs(after-before) < c.Tolerance,
		ParametersUnchanged: reflect.DeepEqual(final.Parameters, reference.Parameters),
		SlowUnchanged:       slowUnchanged,
		PlasticUnchanged:    plasticUnchanged,
	}, nil
}

// runNPFMemoryExpression is the state protocol: for every seed a fresh
// individual trained on the fixed "a" task (delay 2, channel 0, width 2), then
// the suppressed and neutral evaluations that separate expression from
// forgetting. A failed seed keeps its error and the other seeds continue; a
// non-nil context error aborts the run.
func runNPFMemoryExpression(ctx context.Context, c BioInspiredConfig) (BioInspiredReport, error) {
	var report BioInspiredReport
	if ctx == nil {
		return report, fmt.Errorf("npf_memory_expression_hypothesis run needs a context")
	}
	if err := ctx.Err(); err != nil {
		return report, err
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
		if err := ctx.Err(); err != nil {
			return report, err
		}
		out := BioInspiredSeed{Seed: seed}
		result, err := runNPFSeed(ctx, c, spec, seed)
		if err != nil {
			if cerr := ctx.Err(); cerr != nil {
				return report, cerr
			}
			out.Failed, out.Error = true, err.Error()
		} else {
			out.Expression = result
		}
		report.Seeds = append(report.Seeds, out)
	}
	return report, nil
}
