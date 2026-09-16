package learning_test

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/signal"
)

// The chemical fixture is two neurons in two regions on one channel, with one
// hypothesized receptor on node 0 and one unresponsive receptor on node 1, and
// an external timeline that releases a single pulse of 1 at step 1.
//
// dt = 1 and tau = 2, so lambda = exp(-0.5) = 0.6065306597126334 and the
// declared update c' = lambda*c + tau*(1-lambda)*q gives, with q = 0, 1, 0, 0:
//
//	row 0  c = 0
//	row 1  c = 2*(1-lambda)   = 0.7869386805747332
//	row 2  c = lambda*c1      = 0.4773024370823822
//	row 3  c = lambda*c2      = 0.289498562046025
//
// The hypothesized receptor has Kd = 0.5 and n = 1, so occupancy = c/(0.5+c):
//
//	row 0  0                    row 1  0.611481100422978
//	row 2  0.48838764641507565  row 3  0.3666866235902634
//
// The unresponsive receptor sits on node 1 in region 1, which holds the same
// concentration, and still reports exactly 0 on every row.
const (
	chemC0, chemC1 = 0.0, 0.7869386805747332
	chemC2, chemC3 = 0.4773024370823822, 0.289498562046025

	chemOcc0, chemOcc1 = 0.0, 0.611481100422978
	chemOcc2, chemOcc3 = 0.48838764641507565, 0.3666866235902634
)

func chemistryDeclaration() modulation.ChemistryConfig {
	return modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 2, Channels: 1, DT: 1, Tau: []float64{2}},
		Sources: []modulation.SourceSpec{{
			Kind: modulation.SourceExternalTimeline, Channel: 0,
			Timeline: &modulation.ExternalTimeline{ChannelCount: 1, Entries: []modulation.TimelineEntry{{Step: 1, Channel: 0, Rate: 1}}},
		}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{
			{Cells: []int{0}, Signal: "octopamine", Channel: 0, Status: modulation.StatusHypothesized, Kd: 0.5, N: 1,
				Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chem-fixture/v1"},
			{Cells: []int{1}, Signal: "octopamine", Channel: 0, Status: modulation.StatusUnresponsive,
				Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chem-fixture/v1"},
		}},
		Regions: modulation.RegionAssignment{NodeRegion: []int{0, 1}},
	}
}

// sensitivityDeclaration drives node 0's input current: gamma = 1 + occ, beta = 0.
func sensitivityDeclaration() modulation.ChemistryConfig {
	c := chemistryDeclaration()
	c.Effects = []modulation.Effect{{Kind: modulation.EffectSensitivity, Receptor: 0, GammaScale: 1}}
	return c
}

// thresholdDeclaration raises node 0's effective threshold by 0.5*occ.
func thresholdDeclaration() modulation.ChemistryConfig {
	c := chemistryDeclaration()
	c.Effects = []modulation.Effect{{Kind: modulation.EffectThreshold, Receptor: 0, ThetaScale: 0.5, ThetaAbsMax: 1}}
	return c
}

// chemContinuous is two isolated tanh neurons, dt = 1 and tau = 1, so the
// membrane update is v' = exp(-1)*v + (1-exp(-1))*drive with no synaptic term
// and no bias. Node 0 is driven, node 1 is not.
func chemContinuousConfig() learning.Config {
	return learning.Config{
		Dynamics:     dynamics.Config{Nodes: 2, Sources: []int{}, Targets: []int{}, Delays: []int{}, DT: 1, Activation: "tanh"},
		InputSize:    2,
		OutputSize:   1,
		ReadoutNodes: []int{0},
		InputNodes:   []int{0, 1},
	}
}

func chemContinuousParameters() learning.Parameters {
	return learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}},
		Encoder: []float64{1, 0, 0, 1},
		Readout: []float64{1},
	}
}

// chemLIF is the same two isolated neurons on the spiking core. theta_raw 0
// with theta_min 0.1 and theta_max 2 puts the base threshold at exactly 1.05.
func chemLIFConfig() learning.Config {
	core := dynamics.LIFConfig{
		Nodes: 2, Sources: []int{}, Targets: []int{}, Delays: []int{},
		DT: 1, TauSyn: 1, ThetaMin: 0.1, ThetaMax: 2, VReset: -0.5, RefractorySteps: 0,
		Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
	}
	return learning.Config{LIF: &core, InputSize: 2, OutputSize: 1, ReadoutNodes: []int{0}, InputNodes: []int{0, 1}}
}

func chemLIFParameters() learning.Parameters {
	return learning.Parameters{
		Core:     dynamics.Parameters{Weights: []float64{}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}},
		ThetaRaw: []float64{0, 0},
		Encoder:  []float64{1, 0, 0, 1},
		Readout:  []float64{1},
	}
}

func enableChemistry(t *testing.T, a *learning.Individual, c modulation.ChemistryConfig) {
	t.Helper()
	if err := a.EnableChemistry(c); err != nil {
		t.Fatalf("EnableChemistry() error = %v", err)
	}
}

func voltageOf(t *testing.T, a *learning.Individual) []float64 {
	t.Helper()
	s := a.Snapshot()
	if s.Neural.Continuous != nil {
		return s.Neural.Continuous.Voltage
	}
	if s.Neural.LIF != nil {
		return s.Neural.LIF.Voltage
	}
	t.Fatal("the individual has neither a continuous nor a LIF state")
	return nil
}

// Hand table on the continuous core. The row's concentration and occupancy come
// from the table at the top of this file; gamma = 1 + occupancy; the membrane
// coefficients are lambda = exp(-1) = 0.36787944117144233 and
// alpha = 1-exp(-1) = 0.6321205588285577, and the input of node 0 is 1 on every
// row, so v' = lambda*v + alpha*gamma:
//
//	row 0  gamma 1                   v 0.6321205588285577
//	row 1  gamma 1.6114811004229779  v 1.2511944916758613
//	row 2  gamma 1.4883876464150756  v 1.401129161199922
//	row 3  gamma 1.3666866235902635  v 1.379357325078631
//
// Node 1 is reached by the unresponsive receptor only, receives no input and
// keeps a voltage of exactly zero on every row.
func TestChemistryHandTableOnTheContinuousCore(t *testing.T) {
	a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	enableChemistry(t, a, sensitivityDeclaration())
	rows := []struct {
		concentration, occupancy, gamma, voltage float64
	}{
		{chemC0, chemOcc0, 1, 0.6321205588285577},
		{chemC1, chemOcc1, 1.6114811004229779, 1.2511944916758613},
		{chemC2, chemOcc2, 1.4883876464150756, 1.401129161199922},
		{chemC3, chemOcc3, 1.3666866235902635, 1.379357325078631},
	}
	for row, want := range rows {
		advance(t, a, [][]float64{{1, 0}})
		report := a.ChemistryReport()
		if report.Steps != 1 {
			t.Fatalf("row %d: report.Steps = %d, want 1", row, report.Steps)
		}
		for region := range 2 {
			if !closeEnough(report.Concentration[region][0], want.concentration) {
				t.Fatalf("row %d region %d: concentration = %v, want %v", row, region, report.Concentration[region][0], want.concentration)
			}
		}
		if len(report.Occupancy) != 2 {
			t.Fatalf("row %d: %d occupancy records, want 2", row, len(report.Occupancy))
		}
		if !closeEnough(report.Occupancy[0].Occupancy, want.occupancy) {
			t.Fatalf("row %d: occupancy = %v, want %v", row, report.Occupancy[0].Occupancy, want.occupancy)
		}
		// The unresponsive receptor never reports a number that could be read
		// as a measurement of zero response: it is a declared zero and it is
		// counted separately.
		if report.Occupancy[1].Occupancy != 0 || report.Occupancy[1].Status != modulation.StatusUnresponsive {
			t.Fatalf("row %d: unresponsive record = %+v", row, report.Occupancy[1])
		}
		if report.Unresponsive != 1 || report.Assumed != 0 || report.UnknownSkipped != 0 {
			t.Fatalf("row %d: counts = {assumed %d, unknown_skipped %d, unresponsive %d}", row, report.Assumed, report.UnknownSkipped, report.Unresponsive)
		}
		if report.ClampedGamma != 0 || report.ClampedBeta != 0 || report.ClampedTheta != 0 {
			t.Fatalf("row %d: clamp counts = {%d %d %d}, want none", row, report.ClampedGamma, report.ClampedBeta, report.ClampedTheta)
		}
		if gamma := 1 + report.Occupancy[0].Occupancy; !closeEnough(gamma, want.gamma) {
			t.Fatalf("row %d: gamma = %v, want %v", row, gamma, want.gamma)
		}
		v := voltageOf(t, a)
		if !closeEnough(v[0], want.voltage) {
			t.Fatalf("row %d: node 0 voltage = %v, want %v", row, v[0], want.voltage)
		}
		if v[1] != 0 {
			t.Fatalf("row %d: node 1 voltage = %v, want exactly 0", row, v[1])
		}
		t.Logf("row %d: concentration want %v got %v | occupancy want %v got %v | gamma want %v got %v | node-0 voltage want %v got %v | node-1 voltage %v",
			row, want.concentration, report.Concentration[0][0], want.occupancy, report.Occupancy[0].Occupancy,
			want.gamma, 1+report.Occupancy[0].Occupancy, want.voltage, v[0], v[1])
	}
	// The single pulse of 1 is the whole release of the run: the last row's
	// report covers that row only, so its release total is zero.
	if total := a.ChemistryReport().ReleaseTotal; len(total) != 1 || total[0] != 0 {
		t.Fatalf("release total of the last row = %v, want [0]", total)
	}
}

// One four-row call must produce exactly what the four one-row calls above
// produced, and report the pulse once in its release total.
func TestChemistryOneCallEqualsFourCalls(t *testing.T) {
	a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	enableChemistry(t, a, sensitivityDeclaration())
	advance(t, a, [][]float64{{1, 0}, {1, 0}, {1, 0}, {1, 0}})
	report := a.ChemistryReport()
	if report.Steps != 4 {
		t.Fatalf("report.Steps = %d, want 4", report.Steps)
	}
	if len(report.ReleaseTotal) != 1 || report.ReleaseTotal[0] != 1 {
		t.Fatalf("release total = %v, want [1]", report.ReleaseTotal)
	}
	if !closeEnough(report.Concentration[0][0], chemC3) {
		t.Fatalf("final concentration = %v, want %v", report.Concentration[0][0], chemC3)
	}
	if !closeEnough(report.Occupancy[0].Occupancy, chemOcc3) {
		t.Fatalf("final occupancy = %v, want %v", report.Occupancy[0].Occupancy, chemOcc3)
	}
	if v := voltageOf(t, a); !closeEnough(v[0], 1.379357325078631) || v[1] != 0 {
		t.Fatalf("final voltage = %v, want [1.379357325078631 0]", v)
	}
}

// Hand table on the spiking core. The base threshold is
// theta_min + (theta_max-theta_min)*sigmoid(0) = 0.1 + 1.9*0.5 = 1.05, the
// threshold effect adds m = 0.5*occupancy, and the candidate membrane is
// lambda*v + alpha*2 with the same lambda and alpha as above:
//
//	row 0  theta 1.05                m 0                    cand 1.2642411176571153  spike   v -0.5
//	row 1  theta 1.355740550211489   m 0.305740550211489    cand 1.0803013970713942  silent  v 1.0803013970713942
//	row 2  theta 1.2941938232075378  m 0.24419382320753782  cand 1.6616617919084682  spike   v -0.5
//	row 3  theta 1.2333433117951318  m 0.1833433117951317   cand 1.0803013970713942  silent  v 1.0803013970713942
//
// Without the effect every one of those four candidates clears 1.05, so the run
// spikes on all four rows. The declared threshold effect suppresses the event of
// row 1, which is what leaves the membrane high enough for row 2 to clear a
// threshold that is itself raised.
func TestChemistryHandTableOnTheLIFCore(t *testing.T) {
	input := [][]float64{{2, 0}, {2, 0}, {2, 0}, {2, 0}}

	plain := newIndividual(t, chemLIFConfig(), chemLIFParameters())
	unmodulated := make([]float64, 0, 4)
	for range input {
		advance(t, plain, [][]float64{{2, 0}})
		unmodulated = append(unmodulated, voltageOf(t, plain)[0])
	}
	for row, v := range unmodulated {
		if v != -0.5 {
			t.Fatalf("unmodulated row %d: voltage = %v, want the reset -0.5 because every row spikes", row, v)
		}
	}

	a := newIndividual(t, chemLIFConfig(), chemLIFParameters())
	enableChemistry(t, a, thresholdDeclaration())
	silent := 1.0803013970713942
	rows := []struct {
		occupancy, shift, voltage float64
		spiked                    bool
	}{
		{chemOcc0, 0, -0.5, true},
		{chemOcc1, 0.305740550211489, silent, false},
		{chemOcc2, 0.24419382320753782, -0.5, true},
		{chemOcc3, 0.1833433117951317, silent, false},
	}
	for row, want := range rows {
		advance(t, a, [][]float64{input[row]})
		report := a.ChemistryReport()
		if !closeEnough(report.Occupancy[0].Occupancy, want.occupancy) {
			t.Fatalf("row %d: occupancy = %v, want %v", row, report.Occupancy[0].Occupancy, want.occupancy)
		}
		if shift := 0.5 * report.Occupancy[0].Occupancy; !closeEnough(shift, want.shift) {
			t.Fatalf("row %d: threshold shift = %v, want %v", row, shift, want.shift)
		}
		if report.ClampedTheta != 0 {
			t.Fatalf("row %d: clamped theta = %d, want 0", row, report.ClampedTheta)
		}
		v := voltageOf(t, a)
		if !closeEnough(v[0], want.voltage) {
			t.Fatalf("row %d: node 0 voltage = %v, want %v (spiked = %v)", row, v[0], want.voltage, want.spiked)
		}
		if want.spiked != (v[0] == -0.5) {
			t.Fatalf("row %d: voltage %v does not agree with the expected event %v", row, v[0], want.spiked)
		}
		t.Logf("row %d: occupancy want %v got %v | threshold shift want %v got %v | effective threshold %v | voltage want %v got %v | spiked %v (unmodulated voltage %v)",
			row, want.occupancy, report.Occupancy[0].Occupancy, want.shift, 0.5*report.Occupancy[0].Occupancy,
			1.05+0.5*report.Occupancy[0].Occupancy, want.voltage, v[0], want.spiked, unmodulated[row])
	}
}

// With chemistry disabled the whole path is the one that existed before it, bit
// for bit, on both cores: enabling and disabling again leaves nothing behind.
func TestChemistryDisabledIsBitIdentical(t *testing.T) {
	for name, fixture := range map[string]struct {
		config learning.Config
		params learning.Parameters
		input  [][]float64
		decl   modulation.ChemistryConfig
	}{
		"continuous": {chemContinuousConfig(), chemContinuousParameters(), [][]float64{{1, 0}, {1, 0}, {1, 0}}, sensitivityDeclaration()},
		"lif":        {chemLIFConfig(), chemLIFParameters(), [][]float64{{2, 0}, {2, 0}, {2, 0}}, thresholdDeclaration()},
	} {
		never := newIndividual(t, fixture.config, fixture.params)
		want := advance(t, never, fixture.input)

		toggled := newIndividual(t, fixture.config, fixture.params)
		enableChemistry(t, toggled, fixture.decl)
		toggled.DisableChemistry()
		got := advance(t, toggled, fixture.input)
		if !sameRows(got, want) {
			t.Fatalf("%s: outputs after disabling = %v, want %v", name, got, want)
		}
		if !sameBits(voltageOf(t, toggled), voltageOf(t, never)) {
			t.Fatalf("%s: voltage after disabling = %v, want %v", name, voltageOf(t, toggled), voltageOf(t, never))
		}
		if report := toggled.ChemistryReport(); report.Steps != 0 || report.Concentration != nil || report.Occupancy != nil {
			t.Fatalf("%s: a disabled individual reported %+v, want the zero value", name, report)
		}
	}
}

// A declared chemistry whose concentration never leaves zero is bit for bit the
// same run as no chemistry at all, on both cores. This is the neutral-equals-off
// property at the level of the individual rather than of one ApplyEffects call.
func TestChemistryAtZeroConcentrationIsBitIdentical(t *testing.T) {
	for name, fixture := range map[string]struct {
		config learning.Config
		params learning.Parameters
		input  [][]float64
		decl   modulation.ChemistryConfig
	}{
		"continuous": {chemContinuousConfig(), chemContinuousParameters(), [][]float64{{1, 0}, {1, 0}, {1, 0}}, sensitivityDeclaration()},
		"lif":        {chemLIFConfig(), chemLIFParameters(), [][]float64{{2, 0}, {2, 0}, {2, 0}}, thresholdDeclaration()},
	} {
		silent := fixture.decl
		silent.Sources = []modulation.SourceSpec{{
			Kind: modulation.SourceExternalTimeline, Channel: 0,
			Timeline: &modulation.ExternalTimeline{ChannelCount: 1},
		}}
		never := newIndividual(t, fixture.config, fixture.params)
		want := advance(t, never, fixture.input)

		declared := newIndividual(t, fixture.config, fixture.params)
		enableChemistry(t, declared, silent)
		got := advance(t, declared, fixture.input)
		if !sameRows(got, want) {
			t.Fatalf("%s: outputs at zero concentration = %v, want %v", name, got, want)
		}
		if !sameBits(voltageOf(t, declared), voltageOf(t, never)) {
			t.Fatalf("%s: voltage at zero concentration = %v, want %v", name, voltageOf(t, declared), voltageOf(t, never))
		}
		report := declared.ChemistryReport()
		if report.Occupancy[0].Occupancy != 0 || report.ClampedGamma != 0 || report.ClampedBeta != 0 || report.ClampedTheta != 0 {
			t.Fatalf("%s: report at zero concentration = %+v", name, report)
		}
	}
}

// A temporary effect is an argument of one step and never a parameter. A
// hundred rows under an active receptor leave every parameter, theta_raw
// included, bit for bit unchanged.
func TestChemistryNeverWritesParameters(t *testing.T) {
	a := newIndividual(t, chemLIFConfig(), chemLIFParameters())
	declaration := thresholdDeclaration()
	// A pulse every third step keeps the concentration, and therefore the
	// threshold shift, non-zero for the whole run.
	entries := make([]modulation.TimelineEntry, 0, 34)
	for step := uint64(0); step < 100; step += 3 {
		entries = append(entries, modulation.TimelineEntry{Step: step, Channel: 0, Rate: 1})
	}
	declaration.Sources[0].Timeline = &modulation.ExternalTimeline{ChannelCount: 1, Entries: entries}
	enableChemistry(t, a, declaration)
	before := a.Snapshot().Parameters

	input := make([][]float64, 100)
	for i := range input {
		input[i] = []float64{2, 0}
	}
	advance(t, a, input)

	report := a.ChemistryReport()
	if report.Steps != 100 {
		t.Fatalf("report.Steps = %d, want 100", report.Steps)
	}
	if report.Occupancy[0].Occupancy <= 0 {
		t.Fatalf("occupancy after 100 rows = %v, want a live receptor", report.Occupancy[0].Occupancy)
	}
	after := a.Snapshot().Parameters
	for name, pair := range map[string][2][]float64{
		"weights":   {before.Core.Weights, after.Core.Weights},
		"bias":      {before.Core.Bias, after.Core.Bias},
		"log_tau":   {before.Core.LogTau, after.Core.LogTau},
		"theta_raw": {before.ThetaRaw, after.ThetaRaw},
		"encoder":   {before.Encoder, after.Encoder},
		"readout":   {before.Readout, after.Readout},
	} {
		if !sameBits(pair[1], pair[0]) {
			t.Fatalf("%s changed from %v to %v after 100 modulated rows", name, pair[0], pair[1])
		}
	}
	if len(after.ThetaRaw) != 2 {
		t.Fatalf("theta_raw has %d values, want 2", len(after.ThetaRaw))
	}
}

// Feedback that has not arrived is never handed to a source. Every source
// re-checks the arrival itself and fails the call if it sees an early one, so
// an advance that succeeds while such feedback is queued is the evidence: the
// same source, handed the same unfiltered queue, refuses the same step.
func TestChemistryNeverHandsAnEarlyFeedbackToASource(t *testing.T) {
	a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	enableChemistry(t, a, sensitivityDeclaration())
	fb := stepFeedbackAt(t, 0, 3)
	if err := a.OfferFeedback(fb); err != nil {
		t.Fatalf("OfferFeedback() error = %v", err)
	}
	// Steps 0, 1 and 2 are all before the declared arrival at step 3.
	advance(t, a, [][]float64{{1, 0}, {1, 0}, {1, 0}})
	// Step 3 is the arrival itself, which is available and accepted.
	advance(t, a, [][]float64{{1, 0}})

	unfiltered := modulation.ExternalTimeline{ChannelCount: 1, Entries: []modulation.TimelineEntry{{Step: 1, Channel: 0, Rate: 1}}}
	if _, err := unfiltered.Release(0, modulation.SourceContext{Feedback: []signal.Feedback{fb}}); err == nil {
		t.Fatal("the same source accepted the unfiltered queue at step 0, so the run above proves nothing")
	}
	if err := a.OfferFeedback(wrongClockFeedback(t)); err == nil {
		t.Fatal("OfferFeedback() accepted feedback that is not on the model-step clock")
	}
}

// A resource is a declared input of the task. It reaches the chemistry through
// the declared rule coefficient * max(amount - threshold, 0) and nowhere else.
func TestChemistryReadsDeclaredResources(t *testing.T) {
	a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	declaration := sensitivityDeclaration()
	declaration.Sources = []modulation.SourceSpec{{
		Kind: modulation.SourceInternalResource, Channel: 0,
		Resource: &modulation.InternalResource{Resource: "energy", Coefficient: 2, Threshold: 0.25, Channel: 0},
	}}
	enableChemistry(t, a, declaration)

	// No resource is declared yet, so the source refuses and the call commits
	// nothing: the individual has taken no step.
	if _, err := a.Advance(context.Background(), [][]float64{{1, 0}}); err == nil {
		t.Fatal("Advance() error = nil with no resource declared")
	}
	if steps := a.Snapshot().Neural.Continuous.Steps; steps != 0 {
		t.Fatalf("a refused advance committed %d steps", steps)
	}
	if err := a.SetResource("energy", 0.75); err != nil {
		t.Fatalf("SetResource() error = %v", err)
	}
	advance(t, a, [][]float64{{1, 0}})
	// rate = 2 * max(0.75-0.25, 0) = 1, so one step of the declared kinetics
	// gives the same 2*(1-lambda) as the pulse of the table above.
	report := a.ChemistryReport()
	if !closeEnough(report.Concentration[0][0], chemC1) {
		t.Fatalf("concentration = %v, want %v", report.Concentration[0][0], chemC1)
	}
	if len(report.ReleaseTotal) != 1 || report.ReleaseTotal[0] != 1 {
		t.Fatalf("release total = %v, want [1]", report.ReleaseTotal)
	}
	for name, value := range map[string]float64{"nan": math.NaN(), "inf": math.Inf(1)} {
		if err := a.SetResource(name, value); err == nil {
			t.Errorf("SetResource(%q, %v) error = nil", name, value)
		}
	}
	if err := a.SetResource("   ", 1); err == nil {
		t.Error("SetResource() accepted a blank resource name")
	}
}

// EnableChemistry validates against this individual's own topology, and a
// second call replaces the declaration and restarts the chemical state.
func TestEnableChemistryValidatesAndRestarts(t *testing.T) {
	a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	wrongNodes := sensitivityDeclaration()
	wrongNodes.Regions.NodeRegion = []int{0, 1, 0}
	if err := a.EnableChemistry(wrongNodes); err == nil {
		t.Fatal("EnableChemistry() accepted a region map for three nodes on a two-node individual")
	}
	if report := a.ChemistryReport(); report.Steps != 0 || report.Occupancy != nil {
		t.Fatalf("a refused EnableChemistry left %+v behind", report)
	}
	enableChemistry(t, a, sensitivityDeclaration())
	advance(t, a, [][]float64{{1, 0}, {1, 0}})
	if c := a.ChemistryReport().Concentration[0][0]; !closeEnough(c, chemC1) {
		t.Fatalf("concentration after two rows = %v, want %v", c, chemC1)
	}
	// The second call restarts the chemical state: the concentration the first
	// declaration accumulated does not carry into the new one.
	enableChemistry(t, a, sensitivityDeclaration())
	advance(t, a, [][]float64{{1, 0}})
	if c := a.ChemistryReport().Concentration[0][0]; c != 0 {
		t.Fatalf("concentration after a restart = %v, want exactly 0 because step 2 releases nothing", c)
	}
}

// The resource map and the feedback queue belong to the enabled chemistry.
func TestResourceAndFeedbackNeedAnEnabledChemistry(t *testing.T) {
	a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	if err := a.SetResource("energy", 1); err == nil {
		t.Error("SetResource() error = nil while chemistry is disabled")
	}
	if err := a.OfferFeedback(stepFeedbackAt(t, 0, 0)); err == nil {
		t.Error("OfferFeedback() error = nil while chemistry is disabled")
	}
}

func stepFeedbackAt(t *testing.T, producedAt, availableAt int64) signal.Feedback {
	t.Helper()
	f, err := signal.NewFeedback(signal.FeedbackSpec{
		SchemaVersion: signal.CurrentSchemaVersion(),
		ExperienceID:  "exp-chem",
		ActionID:      "act-chem",
		ProducedAt:    signal.Timestamp{Value: producedAt, Unit: signal.TimeUnitModelStep},
		AvailableAt:   signal.Timestamp{Value: availableAt, Unit: signal.TimeUnitModelStep},
		Source:        "teacher",
		Score:         0.5,
		ModelVersion:  signal.CurrentSchemaVersion(),
	})
	if err != nil {
		t.Fatalf("build feedback: %v", err)
	}
	return f
}

func wrongClockFeedback(t *testing.T) signal.Feedback {
	t.Helper()
	f, err := signal.NewFeedback(signal.FeedbackSpec{
		SchemaVersion: signal.CurrentSchemaVersion(),
		ExperienceID:  "exp-chem",
		ActionID:      "act-chem",
		ProducedAt:    signal.Timestamp{Value: 0, Unit: signal.TimeUnitMilliseconds},
		AvailableAt:   signal.Timestamp{Value: 0, Unit: signal.TimeUnitMilliseconds},
		Source:        "teacher",
		Score:         0.5,
		ModelVersion:  signal.CurrentSchemaVersion(),
	})
	if err != nil {
		t.Fatalf("build feedback: %v", err)
	}
	return f
}

// sameBits compares two float64 slices by bit pattern, so a positive and a
// negative zero are different values.
func sameBits(got, want []float64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if math.Float64bits(got[i]) != math.Float64bits(want[i]) {
			return false
		}
	}
	return true
}

// neuralDeclaration feeds the channel from the mean output of node 0, so the
// release of every row is the activity of the row before it.
func neuralDeclaration() modulation.ChemistryConfig {
	c := sensitivityDeclaration()
	c.Sources = []modulation.SourceSpec{{
		Kind: modulation.SourceNeuralActivity, Channel: 0,
		Neural: &modulation.NeuralActivity{Nodes: []int{0}, SetName: "driven", Gain: 1, Channel: 0},
	}}
	return c
}

// A source that averages the previous step's activity has nothing to average on
// the very first row of a fresh individual, so it refuses rather than inventing
// a zero. After one step it reads exactly that step's node outputs:
// tanh(1-exp(-1)) = 0.5595106570525964, which one step of the declared kinetics
// turns into 2*(1-exp(-0.5))*0.5595106570525964 = 0.44030057822847224.
func TestNeuralSourceReadsThePreviousRowOutputs(t *testing.T) {
	fresh := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	enableChemistry(t, fresh, neuralDeclaration())
	if _, err := fresh.Advance(context.Background(), [][]float64{{1, 0}}); err == nil {
		t.Fatal("Advance() error = nil on the first row of a fresh individual with a neural source")
	}
	if steps := fresh.Snapshot().Neural.Continuous.Steps; steps != 0 {
		t.Fatalf("the refused advance committed %d steps", steps)
	}

	a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	// The readout crosses Insyra's float32 boundary, so it is compared at
	// float32 resolution. The source reads the float64 node output the neural
	// history holds, which is why the concentration below is exact.
	warm := advance(t, a, [][]float64{{1, 0}})
	if math.Abs(warm[0][0]-0.5595106570525964) > 1e-7 {
		t.Fatalf("warm-up output = %v, want 0.5595106570525964", warm[0][0])
	}
	enableChemistry(t, a, neuralDeclaration())
	advance(t, a, [][]float64{{1, 0}})
	report := a.ChemistryReport()
	if !closeEnough(report.Concentration[0][0], 0.44030057822847224) {
		t.Fatalf("concentration = %v, want 0.44030057822847224", report.Concentration[0][0])
	}
	if !closeEnough(report.ReleaseTotal[0], 0.5595106570525964) {
		t.Fatalf("release total = %v, want 0.5595106570525964", report.ReleaseTotal[0])
	}
}

// The chemical part survives a snapshot, a JSON document and a restore
// unchanged, declaration, concentration, resources and queued feedback alike.
func TestChemicalSnapshotRoundTripsBitIdentically(t *testing.T) {
	a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	declaration := sensitivityDeclaration()
	declaration.Chemistry.Transport = &modulation.Transport{Fraction: [][]float64{{0, 0.25}, {0.5, 0}}}
	enableChemistry(t, a, declaration)
	if err := a.SetResource("energy", 1.25); err != nil {
		t.Fatalf("SetResource() error = %v", err)
	}
	if err := a.OfferFeedback(stepFeedbackAt(t, 1, 4)); err != nil {
		t.Fatalf("OfferFeedback() error = %v", err)
	}
	advance(t, a, [][]float64{{1, 0}, {1, 0}})

	before := a.Snapshot()
	if before.Chemical == nil {
		t.Fatal("the snapshot has no chemical part")
	}
	data, err := json.Marshal(before)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded learning.IndividualSnapshot
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	restored, err := learning.RestoreIndividual(decoded)
	if err != nil {
		t.Fatalf("RestoreIndividual() error = %v", err)
	}
	after := restored.Snapshot()
	if !reflect.DeepEqual(before.Chemical, after.Chemical) {
		t.Fatalf("the chemical part changed:\n got %+v\nwant %+v", after.Chemical, before.Chemical)
	}
	for region := range before.Chemical.State.Concentration {
		if !sameBits(after.Chemical.State.Concentration[region], before.Chemical.State.Concentration[region]) {
			t.Fatalf("region %d concentration changed: %v against %v", region,
				after.Chemical.State.Concentration[region], before.Chemical.State.Concentration[region])
		}
	}
	// The running declaration is owned: writing into the snapshot changes
	// nothing the individual will run next.
	before.Chemical.Config.Chemistry.Tau[0] = 99
	before.Chemical.State.Concentration[0][0] = 99
	if a.Snapshot().Chemical.Config.Chemistry.Tau[0] != 2 || a.Snapshot().Chemical.State.Concentration[0][0] == 99 {
		t.Fatal("the snapshot shares a buffer with the running individual")
	}
}

// A snapshot taken in the middle of a run continues exactly where it stopped.
// The source here averages the previous step's activity, which no field of the
// chemical part carries: it is read back from the persistent neural history, so
// the restored individual sees the same row the uninterrupted one saw.
func TestChemicalMidRunSnapshotContinues(t *testing.T) {
	warm := [][]float64{{1, 0}}
	rows := [][]float64{{1, 0}, {0.5, 0}, {1, 0}}

	whole := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	advance(t, whole, warm)
	enableChemistry(t, whole, neuralDeclaration())
	first := advance(t, whole, rows)
	second := advance(t, whole, rows)
	wantReport := whole.ChemistryReport()
	wantVoltage := voltageOf(t, whole)

	split := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	advance(t, split, warm)
	enableChemistry(t, split, neuralDeclaration())
	if got := advance(t, split, rows); !sameRows(got, first) {
		t.Fatalf("the first half diverged: %v against %v", got, first)
	}
	data, err := json.Marshal(split.Snapshot())
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded learning.IndividualSnapshot
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	resumed, err := learning.RestoreIndividual(decoded)
	if err != nil {
		t.Fatalf("RestoreIndividual() error = %v", err)
	}
	if got := advance(t, resumed, rows); !sameRows(got, second) {
		t.Fatalf("the resumed half diverged: %v against %v", got, second)
	}
	report := resumed.ChemistryReport()
	for region := range wantReport.Concentration {
		if !sameBits(report.Concentration[region], wantReport.Concentration[region]) {
			t.Fatalf("region %d concentration after resuming = %v, want %v", region,
				report.Concentration[region], wantReport.Concentration[region])
		}
	}
	if !reflect.DeepEqual(report.Occupancy, wantReport.Occupancy) {
		t.Fatalf("occupancy after resuming = %+v, want %+v", report.Occupancy, wantReport.Occupancy)
	}
	if !sameBits(report.ReleaseTotal, wantReport.ReleaseTotal) {
		t.Fatalf("release total after resuming = %v, want %v", report.ReleaseTotal, wantReport.ReleaseTotal)
	}
	if !sameBits(voltageOf(t, resumed), wantVoltage) {
		t.Fatalf("voltage after resuming = %v, want %v", voltageOf(t, resumed), wantVoltage)
	}
}

// A chemical part that does not describe this individual is refused, never
// repaired and never zero filled.
func TestRestoreIndividualRefusesABrokenChemicalPart(t *testing.T) {
	a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	enableChemistry(t, a, sensitivityDeclaration())
	if err := a.SetResource("energy", 1); err != nil {
		t.Fatalf("SetResource() error = %v", err)
	}
	if err := a.OfferFeedback(stepFeedbackAt(t, 0, 2)); err != nil {
		t.Fatalf("OfferFeedback() error = %v", err)
	}
	advance(t, a, [][]float64{{1, 0}})
	if _, err := learning.RestoreIndividual(a.Snapshot()); err != nil {
		t.Fatalf("the intact snapshot was refused: %v", err)
	}
	for name, breakIt := range map[string]func(s *learning.IndividualSnapshot){
		"concentration has one region": func(s *learning.IndividualSnapshot) {
			s.Chemical.State.Concentration = [][]float64{{0}}
		},
		"concentration is negative": func(s *learning.IndividualSnapshot) {
			s.Chemical.State.Concentration[0][0] = -1
		},
		"concentration is not finite": func(s *learning.IndividualSnapshot) {
			s.Chemical.State.Concentration[0][0] = math.Inf(1)
		},
		"region map is for three nodes": func(s *learning.IndividualSnapshot) {
			s.Chemical.Config.Regions.NodeRegion = []int{0, 1, 0}
		},
		"a source is missing": func(s *learning.IndividualSnapshot) {
			s.Chemical.Config.Sources = nil
		},
		"a receptor lost its provenance": func(s *learning.IndividualSnapshot) {
			s.Chemical.Config.Receptors.Records[0].MappingVersion = ""
		},
		"an effect names a receptor that does not exist": func(s *learning.IndividualSnapshot) {
			s.Chemical.Config.Effects[0].Receptor = 5
		},
		"a resource is not finite": func(s *learning.IndividualSnapshot) {
			s.Chemical.Resources["energy"] = math.NaN()
		},
		"a resource has no name": func(s *learning.IndividualSnapshot) {
			s.Chemical.Resources["  "] = 1
		},
		"queued feedback is on another clock": func(s *learning.IndividualSnapshot) {
			spec := s.Chemical.PendingFeedback[0]
			spec.ProducedAt.Unit, spec.AvailableAt.Unit = signal.TimeUnitMilliseconds, signal.TimeUnitMilliseconds
			s.Chemical.PendingFeedback[0] = spec
		},
		"queued feedback is invalid": func(s *learning.IndividualSnapshot) {
			spec := s.Chemical.PendingFeedback[0]
			spec.Source = ""
			s.Chemical.PendingFeedback[0] = spec
		},
	} {
		s := a.Snapshot()
		breakIt(&s)
		if _, err := learning.RestoreIndividual(s); err == nil {
			t.Errorf("%s: RestoreIndividual() error = nil", name)
		}
	}
}

// The two mechanisms compose in one advance, in the declared order: the
// chemical layer produces the modulation of a row, the core takes that row, and
// the local plastic update follows it. With the concentration at zero the run
// is bit for bit the plastic-only one; with the declared pulse it is not, so
// the composition is doing something.
func TestPlasticityAndChemistryRunInOneAdvance(t *testing.T) {
	input, gate := plasticLIFInput(), []float64{1, 1, 1, 1, 1}

	plasticOnly := newIndividual(t, plasticLIFConfig(), plasticLIFParameters())
	if err := plasticOnly.EnablePlasticity(plasticConfig(0)); err != nil {
		t.Fatal(err)
	}
	want, wantReport := advanceGated(t, plasticOnly, input, gate)

	// The effect is an offset rather than a gain, because node 0 of this
	// fixture receives no input on most rows and a gain on zero is zero.
	declaration := chemistryDeclaration()
	declaration.Effects = []modulation.Effect{{Kind: modulation.EffectSensitivity, Receptor: 0, BetaScale: 1}}
	silent := declaration
	silent.Sources = []modulation.SourceSpec{{
		Kind: modulation.SourceExternalTimeline, Channel: 0,
		Timeline: &modulation.ExternalTimeline{ChannelCount: 1},
	}}
	both := newIndividual(t, plasticLIFConfig(), plasticLIFParameters())
	if err := both.EnablePlasticity(plasticConfig(0)); err != nil {
		t.Fatal(err)
	}
	enableChemistry(t, both, silent)
	got, report := advanceGated(t, both, input, gate)
	if !sameRows(got, want) {
		t.Fatalf("outputs with a silent chemistry = %v, want %v", got, want)
	}
	if report != wantReport {
		t.Fatalf("plastic report with a silent chemistry = %+v, want %+v", report, wantReport)
	}
	chemistry := both.ChemistryReport()
	if chemistry.Steps != len(input) || report.Steps != len(input) {
		t.Fatalf("reports cover %d chemical and %d plastic rows, want %d of each", chemistry.Steps, report.Steps, len(input))
	}
	if !sameBits(plasticOnly.Snapshot().Plastic.State.Plastic, both.Snapshot().Plastic.State.Plastic) {
		t.Fatalf("the fast state diverged under a silent chemistry: %v against %v",
			both.Snapshot().Plastic.State.Plastic, plasticOnly.Snapshot().Plastic.State.Plastic)
	}

	live := newIndividual(t, plasticLIFConfig(), plasticLIFParameters())
	if err := live.EnablePlasticity(plasticConfig(0)); err != nil {
		t.Fatal(err)
	}
	enableChemistry(t, live, declaration)
	changed, _ := advanceGated(t, live, input, gate)
	if sameRows(changed, want) {
		t.Fatal("the declared pulse changed nothing, so this fixture proves no composition")
	}
}
