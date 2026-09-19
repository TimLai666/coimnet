package learning_test

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
)

// continuousDelayConfig is three tanh neurons on three edges of delays 0, 1
// and 2, node 2 reading the output. Without input_nodes the encoder spans
// every node, so the single input drives node 0.
func continuousDelayConfig() learning.Config {
	return learning.Config{
		Dynamics:     dynamics.Config{Nodes: 3, Sources: []int{0, 1, 0}, Targets: []int{1, 2, 2}, Delays: []int{0, 1, 2}, DT: 1, Activation: "tanh"},
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: []int{2},
	}
}

func continuousDelayParameters() learning.Parameters {
	return learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{1, 1, 1}, Bias: []float64{0, 0, 0}, LogTau: []float64{0, 0, 0}},
		Encoder: []float64{1, 0, 0},
		Readout: []float64{1},
	}
}

// mixedNodeConfig is the three node mixed fixture of ticket 25, where the node
// interventions are still refused: node 0 continuous, node 1 LIF, node 2
// continuous, one edge in each direction.
func mixedNodeConfig() learning.Config {
	core := dynamics.MixedConfig{
		Nodes:      3,
		Sources:    []int{0, 1},
		Targets:    []int{1, 2},
		Delays:     []int{0, 1},
		DT:         math.Ln2,
		NodeRule:   []uint8{0, 1, 0},
		Continuous: dynamics.ContinuousRule{Activation: "tanh"},
		LIF: dynamics.LIFRule{
			TauSyn: 1, ThetaMin: 0, ThetaMax: 2, VReset: -1, RefractorySteps: 1,
			Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 1},
		},
	}
	return learning.Config{Mixed: &core, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{2}}
}

func mixedNodeParameters() learning.Parameters {
	return learning.Parameters{
		Core:     dynamics.Parameters{Weights: []float64{4, 2}, Bias: []float64{0, 0, 0}, LogTau: []float64{0, 0, 0}},
		ThetaRaw: []float64{0},
		Encoder:  []float64{4, 0, 0},
		Readout:  []float64{1},
	}
}

// removeDeclaration is the sensitivity fixture with a second pulse at step 4,
// so the call after a removal window sees the channel release again.
func removeDeclaration() modulation.ChemistryConfig {
	c := sensitivityDeclaration()
	c.Sources[0].Timeline = &modulation.ExternalTimeline{ChannelCount: 1, Entries: []modulation.TimelineEntry{
		{Step: 1, Channel: 0, Rate: 1},
		{Step: 4, Channel: 0, Rate: 1},
	}}
	return c
}

// TestInterveneRemoveChannelZeroesTheChannelForTheWindow removes channel 0 for
// the whole four row call, on the row the fixture's single pulse would land.
// The concentration stays exactly zero on every row and the release total
// reports nothing, so the removed channel is a silent one for the whole window;
// the constant after it releases into both regions again on a later call.
func TestInterveneRemoveChannelZeroesTheChannelForTheWindow(t *testing.T) {
	a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	enableChemistry(t, a, removeDeclaration())
	plan := authorizedPlan(channelIntervention(learning.InterventionRemoveChannel, 0, 0, 0, 4))
	_, log, err := a.Intervene(context.Background(), plan, [][]float64{{1, 0}, {1, 0}, {1, 0}, {1, 0}})
	if err != nil {
		t.Fatalf("Intervene() error = %v", err)
	}
	report := a.ChemistryReport()
	if report.Steps != 4 {
		t.Fatalf("report.Steps = %d, want 4", report.Steps)
	}
	for region := range 2 {
		if report.Concentration[region][0] != 0 {
			t.Fatalf("region %d concentration after the removal = %v, want exactly 0", region, report.Concentration[region][0])
		}
	}
	if report.Occupancy[0].Occupancy != 0 {
		t.Fatalf("occupancy after the removal = %v, want exactly 0", report.Occupancy[0].Occupancy)
	}
	if len(report.ReleaseTotal) != 1 || report.ReleaseTotal[0] != 0 {
		t.Fatalf("release total = %v, want [0] because the removal consumed the pulse", report.ReleaseTotal)
	}
	e := log.Entries[0]
	if e.Kind != learning.InterventionRemoveChannel || e.Channel != 0 || e.Start != 0 || e.End != 4 || e.AppliedRows != 4 {
		t.Fatalf("entry = %+v, want remove_channel on rows [0,4)", e)
	}
	if math.Abs(e.ConcentrationDelta-1.3644775709818022) > stateTol {
		t.Fatalf("ConcentrationDelta = %.17g, want 1.3644775709818022", e.ConcentrationDelta)
	}
	if e.ActivityDelta <= 0 {
		t.Fatalf("ActivityDelta = %g, want movement against the natural twin", e.ActivityDelta)
	}
	if e.PlasticDelta != 0 || e.BaseParameterDelta != 0 {
		t.Fatalf("a removal cannot move plastic or base state: %+v", e)
	}
	// The channel is back on the call after the window: the pulse at step 4
	// lands and the concentration restarts its ladder.
	advance(t, a, [][]float64{{1, 0}})
	if c := a.ChemistryReport().Concentration[0][0]; math.Abs(c-chemC1) > stateTol {
		t.Fatalf("concentration after the window = %v, want %v", c, chemC1)
	}
}

// TestInterveneFixConcentrationHoldsThroughTheWindowThenDecays fixes channel 0
// at 2 on rows 0 and 1: row 0 jumps straight to 2, row 1 already integrates the
// new pulse at 2 and the fix is a no-op, and the free row 2 decays one step to
// 2*exp(-0.5) while the following ordinary call keeps the free decay at
// 2*exp(-1). The declaration is constant, never a stored parameter.
func TestInterveneFixConcentrationHoldsThroughTheWindowThenDecays(t *testing.T) {
	a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	enableChemistry(t, a, sensitivityDeclaration())
	plan := authorizedPlan(channelIntervention(learning.InterventionFixConcentration, 0, 2, 0, 2))
	_, log, err := a.Intervene(context.Background(), plan, [][]float64{{1, 0}, {1, 0}, {1, 0}})
	if err != nil {
		t.Fatalf("Intervene() error = %v", err)
	}
	report := a.ChemistryReport()
	if report.Steps != 3 {
		t.Fatalf("report.Steps = %d, want 3", report.Steps)
	}
	if !closeEnough(report.Concentration[0][0], 1.2130613194252668) {
		t.Fatalf("concentration after the window = %.17g, want 2*exp(-0.5)", report.Concentration[0][0])
	}
	if !closeEnough(report.Occupancy[0].Occupancy, 0.7081248672594217) {
		t.Fatalf("occupancy after the window = %.17g, want %.17g", report.Occupancy[0].Occupancy, 0.7081248672594217)
	}
	e := log.Entries[0]
	if e.Kind != learning.InterventionFixConcentration || e.Channel != 0 || e.Start != 0 || e.End != 2 || e.AppliedRows != 2 {
		t.Fatalf("entry = %+v, want fix_concentration on rows [0,2)", e)
	}
	if math.Abs(e.ConcentrationDelta-3.4678116724044341) > stateTol {
		t.Fatalf("ConcentrationDelta = %.17g, want 3.4678116724044341", e.ConcentrationDelta)
	}
	if e.ActivityDelta <= 0 || e.PlasticDelta != 0 || e.BaseParameterDelta != 0 {
		t.Fatalf("entry movement = %+v", e)
	}
	advance(t, a, [][]float64{{1, 0}})
	if !closeEnough(a.ChemistryReport().Concentration[0][0], 0.7357588823428847) {
		t.Fatalf("continuation concentration = %.17g, want 2*exp(-1)", a.ChemistryReport().Concentration[0][0])
	}
}

// TestInterveneBlockChannelNeutralizesTheWindow blocks channel 0 on rows 1 and
// 2 of a three row call. The occupied receptor is the only input the fixture
// gains, so a blocked row is the neutral modulation, and the run is bit for bit
// the no-chemistry one while the natural chemistry demonstrably moves the same
// input. The concentration itself is untouched and the follow-up row returns to
// the natural occupancy ladder.
func TestInterveneBlockChannelNeutralizesTheWindow(t *testing.T) {
	a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	enableChemistry(t, a, sensitivityDeclaration())
	plan := authorizedPlan(channelIntervention(learning.InterventionBlockChannel, 0, 0, 1, 3))
	input := [][]float64{{1, 0}, {1, 0}, {1, 0}}

	silent := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	wantOut := advance(t, silent, input)
	wantVoltage := voltageOf(t, silent)

	natural := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	enableChemistry(t, natural, sensitivityDeclaration())
	naturalOut := advance(t, natural, input)
	if sameRows(naturalOut, wantOut) {
		t.Fatal("the natural chemistry did not move the fixture, the block proves nothing")
	}

	out, log, err := a.Intervene(context.Background(), plan, input)
	if err != nil {
		t.Fatalf("Intervene() error = %v", err)
	}
	if !sameRows(out, wantOut) {
		t.Fatalf("blocked output = %v, want the neutral run %v", out, wantOut)
	}
	if !sameBits(voltageOf(t, a), wantVoltage) {
		t.Fatalf("blocked voltage = %v, want the neutral run %v", voltageOf(t, a), wantVoltage)
	}
	report := a.ChemistryReport()
	if len(report.Occupancy) != 2 {
		t.Fatalf("occupancy records = %d, want 2", len(report.Occupancy))
	}
	if report.Occupancy[0].Occupancy != 0 {
		t.Fatalf("last-row occupancy = %v, want exactly 0 because row 2 was blocked", report.Occupancy[0].Occupancy)
	}
	if !closeEnough(report.Concentration[0][0], chemC2) {
		t.Fatalf("concentration = %v, want %v because the block does not change the concentration", report.Concentration[0][0], chemC2)
	}
	e := log.Entries[0]
	if e.Kind != learning.InterventionBlockChannel || e.Channel != 0 || e.Start != 1 || e.End != 3 || e.AppliedRows != 2 {
		t.Fatalf("entry = %+v, want block_channel on rows [1,3)", e)
	}
	if e.ConcentrationDelta != 0 || e.PlasticDelta != 0 || e.BaseParameterDelta != 0 {
		t.Fatalf("entry movement = %+v, want none of the state deltas", e)
	}
	if e.ActivityDelta <= 0 {
		t.Fatalf("ActivityDelta = %g, want movement against the un-blocked twin", e.ActivityDelta)
	}
	// The follow-up row decays from the natural c2 the block left behind, so
	// the occupancy returns to the natural ladder.
	advance(t, a, [][]float64{{1, 0}})
	if !closeEnough(a.ChemistryReport().Occupancy[0].Occupancy, chemOcc3) {
		t.Fatalf("continuation occupancy = %v, want %v", a.ChemistryReport().Occupancy[0].Occupancy, chemOcc3)
	}
}

// TestInterveneSwapRegionsExchangesTheRegions swaps regions 0 and 1 on the
// last row of a two row call. One channel releases into every region with the
// same rate in this stage, so the swap is value-neutral today: the test pins
// the mechanics, the identical post-swap values and the zero concentration
// delta, and the comment names the per-region sources decision that would make
// the swap observable.
func TestInterveneSwapRegionsExchangesTheRegions(t *testing.T) {
	a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	enableChemistry(t, a, sensitivityDeclaration())
	plan := authorizedPlan(swapIntervention(0, 1, 1, 2))
	_, log, err := a.Intervene(context.Background(), plan, [][]float64{{1, 0}, {1, 0}})
	if err != nil {
		t.Fatalf("Intervene() error = %v", err)
	}
	report := a.ChemistryReport()
	if !closeEnough(report.Concentration[0][0], chemC1) || !closeEnough(report.Concentration[1][0], chemC1) {
		t.Fatalf("concentration = %v, want both regions holding %v after the swap", report.Concentration, chemC1)
	}
	e := log.Entries[0]
	if e.Kind != learning.InterventionSwapRegions || e.Start != 1 || e.End != 2 || e.AppliedRows != 1 {
		t.Fatalf("entry = %+v, want swap_regions on rows [1,2)", e)
	}
	if e.ConcentrationDelta != 0 || e.ActivityDelta != 0 {
		t.Fatalf("entry deltas = %+v, want both zero because the identical regions swap to the same values", e)
	}
	if e.PlasticDelta != 0 || e.BaseParameterDelta != 0 {
		t.Fatalf("a swap cannot move plastic or base state: %+v", e)
	}
}

// TestInterveneShuffleDelaysPermutesEdgesForTheWholeCall shuffles the whole
// call of five rows on the three edge continuous fixture. Seed 7 on the two
// delayed edges permutes [1 2] into [2 1], so the run must equal an
// independent core declared with delays [0 2 1], differ from the twin running
// the original [0 1 2], and hand the original delays back before the next
// ordinary call.
func TestInterveneShuffleDelaysPermutesEdgesForTheWholeCall(t *testing.T) {
	core := continuousDelayConfig()
	params := continuousDelayParameters()
	shuffled := core
	shuffled.Dynamics.Delays = []int{0, 2, 1}

	a := newIndividual(t, core, params)
	ref := newIndividual(t, shuffled, params)
	plan := authorizedPlan(shuffleIntervention(nil, 0, 5))
	plan.Items[0].Seed = 7
	input := [][]float64{{1}, {1}, {1}, {1}, {1}}

	refOut := advance(t, ref, input)
	out, log, err := a.Intervene(context.Background(), plan, input)
	if err != nil {
		t.Fatalf("Intervene() error = %v", err)
	}
	if !sameRows(out, refOut) {
		t.Fatalf("shuffled output = %v, want the [0 2 1] reference %v", out, refOut)
	}
	twin := newIndividual(t, core, params)
	if twinOut := advance(t, twin, input); sameRows(out, twinOut) {
		t.Fatal("the shuffle moved nothing against the un-shuffled run")
	}
	e := log.Entries[0]
	if e.Kind != learning.InterventionShuffleDelays || e.Start != 0 || e.End != 5 || e.AppliedRows != 5 {
		t.Fatalf("entry = %+v, want shuffle_delays over the whole call", e)
	}
	if e.ActivityDelta <= 0 || e.PlasticDelta != 0 || e.BaseParameterDelta != 0 || e.ConcentrationDelta != 0 {
		t.Fatalf("entry movement = %+v, want activity only", e)
	}
	t.Logf("seed 7 over the delayed edges permutes [1 2] to [2 1]; output %v", out)

	if got := a.Snapshot().Config.Dynamics.Delays; !equalInts(got, []int{0, 1, 2}) {
		t.Fatalf("delays after the call = %v, want the original [0 1 2]", got)
	}
	next := advance(t, a, [][]float64{{1}})
	refNext := advance(t, ref, [][]float64{{1}})
	if sameRows(next, refNext) {
		t.Fatalf("the continuation still followed the shuffled delays: %v", next)
	}
}

// TestInterveneChannelKindsKeepBaseParameters runs every channel and structure
// kind once and pins that Intervene disturbs the activity only: the base
// parameters of the snapshot stay byte identical and every state delta but the
// activity one is zero.
func TestInterveneChannelKindsKeepBaseParameters(t *testing.T) {
	base := chemContinuousParameters()
	cases := []struct {
		name  string
		decl  modulation.ChemistryConfig
		plan  learning.InterventionPlan
		input [][]float64
	}{
		{"block_channel", sensitivityDeclaration(), authorizedPlan(channelIntervention(learning.InterventionBlockChannel, 0, 0, 1, 3)), [][]float64{{1, 0}, {1, 0}, {1, 0}}},
		{"fix_concentration", sensitivityDeclaration(), authorizedPlan(channelIntervention(learning.InterventionFixConcentration, 0, 2, 0, 2)), [][]float64{{1, 0}, {1, 0}, {1, 0}}},
		{"remove_channel", removeDeclaration(), authorizedPlan(channelIntervention(learning.InterventionRemoveChannel, 0, 0, 0, 4)), [][]float64{{1, 0}, {1, 0}, {1, 0}, {1, 0}}},
		{"swap_regions", sensitivityDeclaration(), authorizedPlan(swapIntervention(0, 1, 1, 2)), [][]float64{{1, 0}, {1, 0}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
			enableChemistry(t, a, tc.decl)
			_, log, err := a.Intervene(context.Background(), tc.plan, tc.input)
			if err != nil {
				t.Fatalf("Intervene() error = %v", err)
			}
			e := log.Entries[0]
			if e.PlasticDelta != 0 || e.BaseParameterDelta != 0 {
				t.Fatalf("entry deltas = %+v, want plastic and base state untouched", e)
			}
			if !equalParameters(a.Snapshot().Parameters, base) {
				t.Fatalf("base parameters moved:\n got %+v\nwant %+v", a.Snapshot().Parameters, base)
			}
		})
	}

	shuffle := newIndividual(t, continuousDelayConfig(), continuousDelayParameters())
	plan := authorizedPlan(shuffleIntervention(nil, 0, 5))
	plan.Items[0].Seed = 7
	if _, log, err := shuffle.Intervene(context.Background(), plan, [][]float64{{1}, {1}, {1}, {1}, {1}}); err != nil {
		t.Fatalf("shuffle Intervene() error = %v", err)
	} else if log.Entries[0].BaseParameterDelta != 0 {
		t.Fatalf("shuffle BaseParameterDelta = %g, want 0", log.Entries[0].BaseParameterDelta)
	}
	if !equalParameters(shuffle.Snapshot().Parameters, continuousDelayParameters()) {
		t.Fatalf("shuffle moved base parameters: %+v", shuffle.Snapshot().Parameters)
	}
}

// equalParameters compares two parameter sets element by element, treating a
// nil slice and an empty one as the same value: a fresh model and a snapshot
// disagree on that representation, not on any number.
func equalParameters(a, b learning.Parameters) bool {
	normalize := func(p learning.Parameters) learning.Parameters {
		fill := func(s []float64) []float64 {
			if len(s) == 0 {
				return nil
			}
			return append([]float64(nil), s...)
		}
		p.Core.Weights = fill(p.Core.Weights)
		p.Core.Bias = fill(p.Core.Bias)
		p.Core.LogTau = fill(p.Core.LogTau)
		p.ThetaRaw = fill(p.ThetaRaw)
		p.Encoder = fill(p.Encoder)
		p.Readout = fill(p.Readout)
		return p
	}
	return reflect.DeepEqual(normalize(a), normalize(b))
}
