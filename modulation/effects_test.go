package modulation

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
)

// twoOnOneCell is the mixing fixture: receptor 0 and receptor 1 both act on
// cell 0, with occupancies 0.75 and 0.5, and receptor 0 also acts on cell 2
// with occupancy 0.25. Every number here is exact in binary, so the expected
// values below are compared exactly.
func twoOnOneCell() []OccupancyRecord {
	return []OccupancyRecord{
		{Receptor: 0, Cell: 0, Status: StatusHypothesized, Occupancy: 0.75},
		{Receptor: 1, Cell: 0, Status: StatusHypothesized, Occupancy: 0.5},
		{Receptor: 0, Cell: 2, Status: StatusHypothesized, Occupancy: 0.25},
	}
}

func sensitivityPair() []Effect {
	return []Effect{
		{Kind: EffectSensitivity, Receptor: 0, GammaScale: 0.5, BetaScale: 0.25},
		{Kind: EffectSensitivity, Receptor: 1, GammaScale: 0.5, BetaScale: 0.25},
	}
}

func assertExact(t *testing.T, name string, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s has %d entries, want %d", name, len(got), len(want))
	}
	for i := range want {
		if math.Float64bits(got[i]) != math.Float64bits(want[i]) {
			t.Fatalf("%s[%d] = %v (bits %#x), want %v (bits %#x)", name, i, got[i], math.Float64bits(got[i]), want[i], math.Float64bits(want[i]))
		}
	}
}

// Declaring an effect while nothing is bound must be the same as declaring no
// effect at all: gain exactly 1, offset and threshold exactly positive zero.
func TestApplyEffectsIsNeutralWithoutOccupancy(t *testing.T) {
	silent := twoOnOneCell()
	for i := range silent {
		silent[i].Occupancy = 0
	}
	effects := append(sensitivityPair(), Effect{Kind: EffectThreshold, Receptor: 0, ThetaScale: -3, ThetaAbsMax: 0.5})
	for _, mix := range []string{"", MixSum, MixMax} {
		gain, offset, threshold, clamped, err := ApplyEffects(effects, silent, mix, 3)
		if err != nil {
			t.Fatal(err)
		}
		assertExact(t, "gain", gain, []float64{1, 1, 1})
		assertExact(t, "offset", offset, []float64{0, 0, 0})
		assertExact(t, "threshold", threshold, []float64{0, 0, 0})
		if (clamped != ClampReport{}) {
			t.Fatalf("mix %q clamped %+v with no occupancy", mix, clamped)
		}

		// And bit for bit the same as declaring nothing.
		bareGain, bareOffset, bareThreshold, bareClamped, err := ApplyEffects(nil, nil, mix, 3)
		if err != nil {
			t.Fatal(err)
		}
		assertExact(t, "gain without effects", bareGain, gain)
		assertExact(t, "offset without effects", bareOffset, offset)
		assertExact(t, "threshold without effects", bareThreshold, threshold)
		if bareClamped != clamped {
			t.Fatalf("mix %q: %+v against %+v", mix, bareClamped, clamped)
		}
	}
}

// Two receptors on one cell, hand computed. Cell 0 carries 0.75 and 0.5:
//
//	sum: occ = 1.25, gamma = 1 + 0.5*1.25 = 1.625, beta = 0.25*1.25 = 0.3125
//	max: occ = 0.75, gamma = 1 + 0.5*0.75 = 1.375, beta = 0.25*0.75 = 0.1875
//
// Cell 2 carries only 0.25 under both rules: gamma = 1.125, beta = 0.0625.
// Cell 1 carries no receptor and stays neutral.
func TestApplyEffectsMixesTwoReceptorsOnOneCell(t *testing.T) {
	cases := []struct {
		mix          string
		gain, offset []float64
	}{
		{MixSum, []float64{1.625, 1, 1.125}, []float64{0.3125, 0, 0.0625}},
		{"", []float64{1.625, 1, 1.125}, []float64{0.3125, 0, 0.0625}},
		{MixMax, []float64{1.375, 1, 1.125}, []float64{0.1875, 0, 0.0625}},
	}
	for _, tc := range cases {
		t.Run("mix "+tc.mix, func(t *testing.T) {
			gain, offset, threshold, clamped, err := ApplyEffects(sensitivityPair(), twoOnOneCell(), tc.mix, 3)
			if err != nil {
				t.Fatal(err)
			}
			assertExact(t, "gain", gain, tc.gain)
			assertExact(t, "offset", offset, tc.offset)
			assertExact(t, "threshold", threshold, []float64{0, 0, 0})
			if (clamped != ClampReport{}) {
				t.Fatalf("clamped %+v inside the declared bounds", clamped)
			}
		})
	}

	// A threshold effect reads the same occupancies through its own group:
	// receptor 0 only, so cell 0 gets 0.5*0.75 = 0.375 under both rules and
	// cell 2 gets 0.5*0.25 = 0.125.
	effects := []Effect{{Kind: EffectThreshold, Receptor: 0, ThetaScale: 0.5, ThetaAbsMax: 1}}
	for _, mix := range []string{MixSum, MixMax} {
		gain, offset, threshold, _, err := ApplyEffects(effects, twoOnOneCell(), mix, 3)
		if err != nil {
			t.Fatal(err)
		}
		assertExact(t, "gain", gain, []float64{1, 1, 1})
		assertExact(t, "offset", offset, []float64{0, 0, 0})
		assertExact(t, "threshold", threshold, []float64{0.375, 0, 0.125})
	}
}

// Every bound is a clamp that is counted, not a silent saturation.
func TestApplyEffectsClampsAndCountsEachBound(t *testing.T) {
	occ := []OccupancyRecord{{Receptor: 0, Cell: 0, Status: StatusHypothesized, Occupancy: 0.75}}
	cases := []struct {
		name                    string
		effects                 []Effect
		gain, offset, threshold float64
		clamped                 ClampReport
	}{
		{"gamma at the upper default", []Effect{{Kind: EffectSensitivity, Receptor: 0, GammaScale: 4}}, 2, 0, 0, ClampReport{Gamma: 1}},
		{"gamma at the lower default", []Effect{{Kind: EffectSensitivity, Receptor: 0, GammaScale: -4}}, 0.5, 0, 0, ClampReport{Gamma: 1}},
		{"beta at the upper default", []Effect{{Kind: EffectSensitivity, Receptor: 0, BetaScale: 4}}, 1, 1, 0, ClampReport{Beta: 1}},
		{"beta at the lower default", []Effect{{Kind: EffectSensitivity, Receptor: 0, BetaScale: -4}}, 1, -1, 0, ClampReport{Beta: 1}},
		{"gamma and beta together", []Effect{{Kind: EffectSensitivity, Receptor: 0, GammaScale: 4, BetaScale: 4}}, 2, 1, 0, ClampReport{Gamma: 1, Beta: 1}},
		{"declared bounds beat the defaults", []Effect{{Kind: EffectSensitivity, Receptor: 0, GammaScale: 4, GammaMin: 0.25, GammaMax: 1.5, BetaScale: 4, BetaAbsMax: 0.25}}, 1.5, 0.25, 0, ClampReport{Gamma: 1, Beta: 1}},
		{"theta above its bound", []Effect{{Kind: EffectThreshold, Receptor: 0, ThetaScale: 4, ThetaAbsMax: 0.5}}, 1, 0, 0.5, ClampReport{Theta: 1}},
		{"theta below its bound", []Effect{{Kind: EffectThreshold, Receptor: 0, ThetaScale: -4, ThetaAbsMax: 0.5}}, 1, 0, -0.5, ClampReport{Theta: 1}},
		{"inside every bound", []Effect{{Kind: EffectSensitivity, Receptor: 0, GammaScale: 0.5, BetaScale: 0.25}}, 1.375, 0.1875, 0, ClampReport{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gain, offset, threshold, clamped, err := ApplyEffects(tc.effects, occ, MixSum, 2)
			if err != nil {
				t.Fatal(err)
			}
			assertExact(t, "gain", gain, []float64{tc.gain, 1})
			assertExact(t, "offset", offset, []float64{tc.offset, 0})
			assertExact(t, "threshold", threshold, []float64{tc.threshold, 0})
			if clamped != tc.clamped {
				t.Fatalf("clamped %+v, want %+v", clamped, tc.clamped)
			}
		})
	}
}

// An effect that names no receptor record changes nothing, and a record whose
// receptor no effect names changes nothing either.
func TestApplyEffectsIgnoresUnboundReceptorsAndRecords(t *testing.T) {
	occ := twoOnOneCell()
	gain, offset, threshold, clamped, err := ApplyEffects([]Effect{{Kind: EffectSensitivity, Receptor: 7, GammaScale: 1}}, occ, MixSum, 3)
	if err != nil {
		t.Fatal(err)
	}
	assertExact(t, "gain", gain, []float64{1, 1, 1})
	assertExact(t, "offset", offset, []float64{0, 0, 0})
	assertExact(t, "threshold", threshold, []float64{0, 0, 0})
	if (clamped != ClampReport{}) {
		t.Fatalf("clamped %+v", clamped)
	}
	// The call does not write into the records it was given.
	if occ[0].Occupancy != 0.75 {
		t.Fatalf("ApplyEffects rewrote the occupancy records: %+v", occ)
	}
}

func TestApplyEffectsRejectsAnInvalidDeclaration(t *testing.T) {
	occ := twoOnOneCell()
	cases := []struct {
		name    string
		effects []Effect
		occ     []OccupancyRecord
		mix     string
		nodes   int
		want    string
	}{
		{"unknown kind", []Effect{{Kind: "gain", Receptor: 0, GammaScale: 1}}, occ, MixSum, 3, "kind"},
		{"empty kind", []Effect{{Receptor: 0, GammaScale: 1}}, occ, MixSum, 3, "kind"},
		{"negative receptor", []Effect{{Kind: EffectSensitivity, Receptor: -1, GammaScale: 1}}, occ, MixSum, 3, "receptor"},
		{"threshold without a bound", []Effect{{Kind: EffectThreshold, Receptor: 0, ThetaScale: 1}}, occ, MixSum, 3, "theta_abs_max"},
		{"negative theta bound", []Effect{{Kind: EffectThreshold, Receptor: 0, ThetaScale: 1, ThetaAbsMax: -1}}, occ, MixSum, 3, "theta_abs_max"},
		{"negative beta bound", []Effect{{Kind: EffectSensitivity, Receptor: 0, BetaScale: 1, BetaAbsMax: -1}}, occ, MixSum, 3, "beta_abs_max"},
		{"gamma bounds crossed", []Effect{{Kind: EffectSensitivity, Receptor: 0, GammaScale: 1, GammaMin: 2, GammaMax: 1}}, occ, MixSum, 3, "gamma_min"},
		{"negative gamma minimum", []Effect{{Kind: EffectSensitivity, Receptor: 0, GammaScale: 1, GammaMin: -1, GammaMax: 2}}, occ, MixSum, 3, "gamma_min"},
		{"non-finite scale", []Effect{{Kind: EffectSensitivity, Receptor: 0, GammaScale: math.Inf(1)}}, occ, MixSum, 3, "finite"},
		{"sensitivity with a threshold knob", []Effect{{Kind: EffectSensitivity, Receptor: 0, GammaScale: 1, ThetaScale: 1}}, occ, MixSum, 3, "never reads"},
		{"threshold with a sensitivity knob", []Effect{{Kind: EffectThreshold, Receptor: 0, ThetaScale: 1, ThetaAbsMax: 1, GammaScale: 1}}, occ, MixSum, 3, "never reads"},
		{"unknown mix", sensitivityPair(), occ, "mean", 3, "mix"},
		{"no node", sensitivityPair(), occ, MixSum, 0, "nodes"},
		{"cell past the node count", sensitivityPair(), occ, MixSum, 2, "cell"},
		{"negative cell", sensitivityPair(), []OccupancyRecord{{Receptor: 0, Cell: -1}}, MixSum, 3, "cell"},
		{"occupancy above one", sensitivityPair(), []OccupancyRecord{{Receptor: 0, Cell: 0, Occupancy: 1.5}}, MixSum, 3, "occupancy"},
		{"negative occupancy", sensitivityPair(), []OccupancyRecord{{Receptor: 0, Cell: 0, Occupancy: -0.5}}, MixSum, 3, "occupancy"},
		{"non-finite occupancy", sensitivityPair(), []OccupancyRecord{{Receptor: 0, Cell: 0, Occupancy: math.NaN()}}, MixSum, 3, "occupancy"},
		{"two scales on one cell", []Effect{
			{Kind: EffectSensitivity, Receptor: 0, GammaScale: 0.5},
			{Kind: EffectSensitivity, Receptor: 1, GammaScale: 0.25},
		}, occ, MixSum, 3, "same coefficients"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gain, offset, threshold, clamped, err := ApplyEffects(tc.effects, tc.occ, tc.mix, tc.nodes)
			if err == nil {
				t.Fatalf("accepted %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
			if gain != nil || offset != nil || threshold != nil || (clamped != ClampReport{}) {
				t.Fatalf("a refused call returned %v %v %v %+v", gain, offset, threshold, clamped)
			}
		})
	}

	// Two effects of the same kind on the same cell are fine while they
	// declare the same coefficients: that is the mixing case above.
	if _, _, _, _, err := ApplyEffects(sensitivityPair(), occ, MixSum, 3); err != nil {
		t.Fatalf("the mixing fixture was refused: %v", err)
	}
	// Different coefficients on different cells never meet, so they are fine.
	apart := []Effect{
		{Kind: EffectSensitivity, Receptor: 0, GammaScale: 0.5},
		{Kind: EffectSensitivity, Receptor: 1, GammaScale: 0.25},
	}
	separate := []OccupancyRecord{
		{Receptor: 0, Cell: 0, Occupancy: 0.5},
		{Receptor: 1, Cell: 1, Occupancy: 0.5},
	}
	gain, _, _, _, err := ApplyEffects(apart, separate, MixSum, 3)
	if err != nil {
		t.Fatalf("effects on different cells were refused: %v", err)
	}
	assertExact(t, "gain", gain, []float64{1.25, 1.125, 1})
}

// The whole stage-one chain with nothing released: a declared chemistry,
// declared receptors and declared effects, all at zero concentration, must
// leave both cores bit for bit where an undeclared chemistry leaves them.
func TestDeclaredEffectsAtZeroConcentrationChangeNeitherCore(t *testing.T) {
	kinetics, err := NewChemistry(Chemistry{Regions: 2, Channels: 1, DT: 1, Tau: []float64{4},
		Transport: &Transport{Fraction: [][]float64{{0, 0.25}, {0.5, 0}}}})
	if err != nil {
		t.Fatal(err)
	}
	state := kinetics.NewState()
	for range 10 {
		if state, err = kinetics.Step(state, [][]float64{{0}, {0}}, nil); err != nil {
			t.Fatal(err)
		}
	}
	receptors := &Receptors{Mix: MixSum, AllowAssumedCoefficients: true, Records: []Receptor{
		{Cells: []int{0, 2}, Signal: "octopamine", Channel: 0, Status: StatusHypothesized, Kd: 1, N: 2,
			Evidence: "fixture", MeasurementKind: "engineering_assumption", MappingVersion: "fixture/v1"},
		{Cells: []int{1}, Signal: "octopamine", Channel: 0, Status: StatusUnknown, Kd: 0.5, N: 1,
			Evidence: "fixture", MeasurementKind: "engineering_assumption", MappingVersion: "fixture/v1"},
	}}
	occ, summary, err := receptors.Occupancies(state, []int{0, 1, 1})
	if err != nil {
		t.Fatal(err)
	}
	for i, record := range occ {
		if record.Occupancy != 0 {
			t.Fatalf("occupancy record %d is %v with nothing released: %+v", i, record.Occupancy, record)
		}
	}
	if summary.Assumed != 1 {
		t.Fatalf("summary %+v", summary)
	}
	effects := []Effect{
		{Kind: EffectSensitivity, Receptor: 0, GammaScale: 0.75, BetaScale: -0.5},
		{Kind: EffectThreshold, Receptor: 1, ThetaScale: 0.25, ThetaAbsMax: 0.5},
	}
	gain, offset, threshold, clamped, err := ApplyEffects(effects, occ, MixSum, 3)
	if err != nil {
		t.Fatal(err)
	}
	if (clamped != ClampReport{}) {
		t.Fatalf("clamped %+v with nothing released", clamped)
	}
	assertExact(t, "gain", gain, []float64{1, 1, 1})
	assertExact(t, "offset", offset, []float64{0, 0, 0})
	assertExact(t, "threshold", threshold, []float64{0, 0, 0})

	steps := 4
	mod := &dynamics.Modulation{Gain: make([][]float64, steps), Offset: make([][]float64, steps), Threshold: make([][]float64, steps)}
	for step := range steps {
		mod.Gain[step] = append([]float64(nil), gain...)
		mod.Offset[step] = append([]float64(nil), offset...)
		mod.Threshold[step] = append([]float64(nil), threshold...)
	}
	inputs := [][]float64{{0.5, -0.25, 1}, {-1, 0.75, 0}, {0.25, 0.25, -0.5}, {2, 0, 0.5}}
	initial := []float64{0.3, -0.4, 0.2}

	continuous, err := dynamics.NewContinuous(dynamics.Config{Nodes: 3, Sources: []int{0, 1}, Targets: []int{1, 2},
		Delays: []int{0, 2}, DT: 0.4, Activation: "tanh"})
	if err != nil {
		t.Fatal(err)
	}
	cp := dynamics.Parameters{Weights: []float64{0.5, -0.25}, Bias: []float64{0.1, -0.2, 0.05}, LogTau: []float64{0, 0.2, -0.1}}
	cs, err := continuous.NewState(initial)
	if err != nil {
		t.Fatal(err)
	}
	plainState, plainOut, err := continuous.Advance(context.Background(), cp, cs, inputs)
	if err != nil {
		t.Fatal(err)
	}
	// The continuous core takes no threshold, so the chain must hand it the
	// all-zero array ApplyEffects produced and be accepted.
	modState, modOut, err := continuous.AdvanceModulated(context.Background(), cp, cs, inputs, mod)
	if err != nil {
		t.Fatal(err)
	}
	assertExactRows(t, "continuous outputs", modOut, plainOut)
	assertExact(t, "continuous voltage", modState.Voltage, plainState.Voltage)

	spiking, err := dynamics.NewLIF(dynamics.LIFConfig{Nodes: 3, Sources: []int{0, 1}, Targets: []int{1, 2},
		Delays: []int{0, 2}, DT: 0.4, TauSyn: 0.9, ThetaMin: 0.3, ThetaMax: 1.6, VReset: -0.5,
		Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2}})
	if err != nil {
		t.Fatal(err)
	}
	lp := dynamics.LIFParameters{Weights: []float64{0.5, -0.25}, Bias: []float64{0.1, -0.2, 0.05},
		LogTau: []float64{0, 0.2, -0.1}, ThetaRaw: []float64{0, 0.3, -0.3}}
	ls, err := spiking.NewState(initial)
	if err != nil {
		t.Fatal(err)
	}
	loud := [][]float64{{2, 2, 2}, {2, 2, 2}, {2, 2, 2}, {2, 2, 2}}
	plainLIF, plainTrace, plainSpikes, err := spiking.Advance(context.Background(), lp, ls, loud)
	if err != nil {
		t.Fatal(err)
	}
	events := 0
	for _, row := range plainSpikes {
		for _, v := range row {
			events += int(v)
		}
	}
	if events == 0 {
		t.Fatal("the spiking fixture never spiked, so the comparison proves nothing")
	}
	modLIF, modTrace, modSpikes, err := spiking.AdvanceModulated(context.Background(), lp, ls, loud, mod)
	if err != nil {
		t.Fatal(err)
	}
	assertExactRows(t, "lif traces", modTrace, plainTrace)
	assertExactRows(t, "lif spikes", modSpikes, plainSpikes)
	assertExact(t, "lif voltage", modLIF.Voltage, plainLIF.Voltage)

	// And the parameters the cores were given are untouched by the effects.
	if cp.Bias[0] != 0.1 || lp.ThetaRaw[1] != 0.3 {
		t.Fatalf("a temporary effect reached the parameters: %v %v", cp.Bias, lp.ThetaRaw)
	}
}

func assertExactRows(t *testing.T, name string, got, want [][]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s has %d rows, want %d", name, len(got), len(want))
	}
	for r := range want {
		assertExact(t, name, got[r], want[r])
	}
}
