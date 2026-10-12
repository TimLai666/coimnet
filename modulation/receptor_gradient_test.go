package modulation

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func gradientReceptor(cells []int, channel int, status string, kd, n float64, location *ReceptorLocation) Receptor {
	return Receptor{
		Cells:           append([]int(nil), cells...),
		Signal:          "gradient-signal",
		Channel:         channel,
		Status:          status,
		Kd:              kd,
		N:               n,
		Evidence:        "gradient fixture",
		MeasurementKind: "engineering_assumption",
		MappingVersion:  "gradient/v1",
		Location:        location,
	}
}

func gradientLocation(region int) *ReceptorLocation {
	return &ReceptorLocation{
		Region:             region,
		Compartment:        "dendrite",
		AnatomicalLocation: "gradient compartment",
		Evidence:           "gradient fixture",
		MeasurementKind:    "receptor imaging",
		MappingVersion:     "gradient-location/v1",
	}
}

func gradientCloneState(state ChemistryState) ChemistryState {
	clone := state
	clone.Concentration = make([][]float64, len(state.Concentration))
	for region, row := range state.Concentration {
		clone.Concentration[region] = append([]float64(nil), row...)
	}
	return clone
}

func gradientAssertClose(t *testing.T, name string, got, want, absolute, relative float64) {
	t.Helper()
	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Fatalf("%s = %v, want finite %v", name, got, want)
	}
	if math.Abs(got-want) > absolute+relative*math.Max(1, math.Abs(want)) {
		t.Fatalf("%s = %.17g, want %.17g", name, got, want)
	}
}

func gradientAssertZero(t *testing.T, name string, got []float64) {
	t.Helper()
	for i, value := range got {
		if math.Float64bits(value) != 0 {
			t.Fatalf("%s[%d] = %v (bits %#x), want positive zero", name, i, value, math.Float64bits(value))
		}
	}
}

func gradientAssertMatrixShape(t *testing.T, got [][]float64, regions, channels int) {
	t.Helper()
	if got == nil {
		t.Fatal("gradient matrix is nil")
	}
	if len(got) != regions {
		t.Fatalf("gradient matrix has %d regions, want %d", len(got), regions)
	}
	for region, row := range got {
		if len(row) != channels {
			t.Fatalf("gradient matrix row %d has %d channels, want %d", region, len(row), channels)
		}
	}
}

func gradientOccupancyObjective(t *testing.T, receptors *Receptors, state ChemistryState, regionOf []int, upstream []float64) float64 {
	t.Helper()
	records, _, err := receptors.Occupancies(state, regionOf)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != len(upstream) {
		t.Fatalf("objective has %d occupancy records, want %d upstream values", len(records), len(upstream))
	}
	var value float64
	for i, record := range records {
		value += upstream[i] * record.Occupancy
	}
	return value
}

func gradientEffectObjective(t *testing.T, effects []Effect, occ []OccupancyRecord, mix string, nodes int, gain, offset, threshold []float64) float64 {
	t.Helper()
	gotGain, gotOffset, gotThreshold, _, err := ApplyEffects(effects, occ, mix, nodes)
	if err != nil {
		t.Fatal(err)
	}
	var value float64
	for node := range gotGain {
		value += gain[node]*gotGain[node] + offset[node]*gotOffset[node] + threshold[node]*gotThreshold[node]
	}
	return value
}

func gradientPerturbOccupancy(occ []OccupancyRecord, index int, delta float64) []OccupancyRecord {
	perturbed := append([]OccupancyRecord(nil), occ...)
	perturbed[index].Occupancy += delta
	return perturbed
}

func TestReceptorGradientHandComputedLocationAndAccumulation(t *testing.T) {
	state := ChemistryState{Concentration: [][]float64{{1, 9}, {7, 2}}, Steps: 11}
	regionOf := []int{0, 1}
	receptors := &Receptors{Records: []Receptor{
		// Both cells use region 1, channel 1 explicitly, even though their
		// neuron-to-region assignments differ.
		gradientReceptor([]int{0, 1}, 1, StatusHypothesized, 2, 1, gradientLocation(1)),
		// This is the same cell as the first record, but it uses another
		// explicitly declared chemical location.
		gradientReceptor([]int{0}, 0, StatusHypothesized, 1, 1, gradientLocation(0)),
	}, Mix: MixSum}
	upstream := []float64{2, -3, 5}
	wantState := gradientCloneState(state)
	wantRegionOf := append([]int(nil), regionOf...)

	got, err := receptors.BackwardOccupancies(state, regionOf, upstream)
	if err != nil {
		t.Fatalf("BackwardOccupancies() error = %v", err)
	}
	gradientAssertMatrixShape(t, got, 2, 2)
	// At c = Kd and N = 1, occupancy is 0.5 and the slope is
	// N*o*(1-o)/c = 1/(4*Kd). The first receptor therefore contributes
	// (2-3)/(4*2) to region 1/channel 1, while the second contributes
	// 5/(4*1) to region 0/channel 0.
	gradientAssertClose(t, "region 0 channel 0", got[0][0], 1.25, 0, 0)
	gradientAssertClose(t, "region 0 channel 1", got[0][1], 0, 0, 0)
	gradientAssertClose(t, "region 1 channel 0", got[1][0], 0, 0, 0)
	gradientAssertClose(t, "region 1 channel 1", got[1][1], -0.125, 0, 0)
	if !reflect.DeepEqual(state, wantState) {
		t.Fatalf("BackwardOccupancies rewrote state: got %+v, want %+v", state, wantState)
	}
	if !reflect.DeepEqual(regionOf, wantRegionOf) {
		t.Fatalf("BackwardOccupancies rewrote region map: got %v, want %v", regionOf, wantRegionOf)
	}

	// Results are owned by each call. Mutating one result cannot alter a later
	// call, and a different upstream gets a fresh zero result.
	got[0][0] = 99
	got[1][1] = 99
	again, err := receptors.BackwardOccupancies(state, regionOf, upstream)
	if err != nil {
		t.Fatalf("second BackwardOccupancies() error = %v", err)
	}
	if again[0][0] != 1.25 || again[1][1] != -0.125 {
		t.Fatalf("later gradient shares storage with the first: %v", again)
	}
	zero, err := receptors.BackwardOccupancies(state, regionOf, []float64{0, 0, 0})
	if err != nil {
		t.Fatalf("zero-upstream BackwardOccupancies() error = %v", err)
	}
	gradientAssertMatrixShape(t, zero, 2, 2)
	for region, row := range zero {
		gradientAssertZero(t, "zero occupancy gradient row "+string(rune('0'+region)), row)
	}
}

func TestReceptorGradientEmptyRecordsReturnOwnedZeroGrid(t *testing.T) {
	receptors := &Receptors{}
	state := ChemistryState{Concentration: [][]float64{{1, 2}, {3, 4}}}
	got, err := receptors.BackwardOccupancies(state, []int{1, 0}, nil)
	if err != nil {
		t.Fatalf("empty-record BackwardOccupancies() error = %v", err)
	}
	gradientAssertMatrixShape(t, got, 2, 2)
	for region, row := range got {
		gradientAssertZero(t, "empty-record row "+string(rune('0'+region)), row)
	}
	got[0][0] = 42
	again, err := receptors.BackwardOccupancies(state, []int{1, 0}, nil)
	if err != nil {
		t.Fatalf("second empty-record BackwardOccupancies() error = %v", err)
	}
	gradientAssertZero(t, "fresh empty-record row", again[0])
}

func TestReceptorGradientStatusAndZeroConcentration(t *testing.T) {
	// Unknown and unresponsive records have zero slope when they are skipped.
	skipped := &Receptors{Records: []Receptor{
		gradientReceptor([]int{0}, 0, StatusUnknown, 2, 1, nil),
		gradientReceptor([]int{0}, 0, StatusUnresponsive, 2, 1, nil),
	}}
	state := ChemistryState{Concentration: [][]float64{{1}}}
	got, err := skipped.BackwardOccupancies(state, []int{0}, []float64{7, -11})
	if err != nil {
		t.Fatalf("skipped status BackwardOccupancies() error = %v", err)
	}
	gradientAssertMatrixShape(t, got, 1, 1)
	gradientAssertZero(t, "skipped status gradient", got[0])

	// An allowed unknown with legal assumed coefficients follows the same curve
	// as a hypothesized receptor, and signed upstream contributions add.
	assumed := &Receptors{AllowAssumedCoefficients: true, Records: []Receptor{
		gradientReceptor([]int{0}, 0, StatusUnknown, 2, 1, nil),
		gradientReceptor([]int{0}, 0, StatusHypothesized, 2, 1, nil),
	}}
	got, err = assumed.BackwardOccupancies(state, []int{0}, []float64{2, -3})
	if err != nil {
		t.Fatalf("assumed status BackwardOccupancies() error = %v", err)
	}
	gradientAssertClose(t, "assumed plus hypothesized", got[0][0], -2.0/9.0, 1e-15, 1e-12)

	// At c = 0, N = 1 has the declared one-sided slope 1/Kd while N > 1
	// has slope zero. This also checks that negative upstream is accepted.
	zeroState := ChemistryState{Concentration: [][]float64{{0}}}
	zeroReceptors := &Receptors{Records: []Receptor{
		gradientReceptor([]int{0}, 0, StatusHypothesized, 2, 1, nil),
		gradientReceptor([]int{0}, 0, StatusHypothesized, 3, 2, nil),
	}}
	got, err = zeroReceptors.BackwardOccupancies(zeroState, []int{0}, []float64{4, -9})
	if err != nil {
		t.Fatalf("zero concentration BackwardOccupancies() error = %v", err)
	}
	gradientAssertClose(t, "zero concentration N=1", got[0][0], 2, 0, 0)
}

func TestReceptorGradientFiniteDifferenceForNonlinearHillCurve(t *testing.T) {
	state := ChemistryState{Concentration: [][]float64{{1.7}}}
	regionOf := []int{0}
	receptors := &Receptors{Records: []Receptor{
		gradientReceptor([]int{0}, 0, StatusHypothesized, 1.3, 2.4, nil),
	}}
	upstream := []float64{-2.3}
	got, err := receptors.BackwardOccupancies(state, regionOf, upstream)
	if err != nil {
		t.Fatalf("BackwardOccupancies() error = %v", err)
	}
	o := Occupancy(1.7, 1.3, 2.4)
	gradientAssertMatrixShape(t, got, 1, 1)
	want := upstream[0] * 2.4 * o * (1 - o) / 1.7
	gradientAssertClose(t, "hand Hill VJP", got[0][0], want, 1e-14, 1e-12)

	// This objective is evaluated only through the forward Occupancies call;
	// the reverse expectation above comes from the written Hill derivative.
	h := 1e-6
	plus := gradientCloneState(state)
	plus.Concentration[0][0] += h
	minus := gradientCloneState(state)
	minus.Concentration[0][0] -= h
	fd := (gradientOccupancyObjective(t, receptors, plus, regionOf, upstream) -
		gradientOccupancyObjective(t, receptors, minus, regionOf, upstream)) / (2 * h)
	gradientAssertClose(t, "central difference Hill VJP", got[0][0], fd, 1e-9, 1e-8)
}

func TestReceptorGradientRejectsInvalidInputsAndOverflow(t *testing.T) {
	valid := func() *Receptors {
		return &Receptors{Records: []Receptor{gradientReceptor([]int{0}, 0, StatusHypothesized, 1, 1, nil)}}
	}
	validState := ChemistryState{Concentration: [][]float64{{1}}}
	cases := []struct {
		name      string
		receptors *Receptors
		state     ChemistryState
		regionOf  []int
		upstream  []float64
		want      string
	}{
		{"nil receptors", nil, validState, []int{0}, []float64{1}, "nil receptor"},
		{"missing concentration", valid(), ChemistryState{}, []int{0}, []float64{1}, "concentration"},
		{"ragged concentration", valid(), ChemistryState{Concentration: [][]float64{{1}, {}}}, []int{0}, []float64{1}, "concentration"},
		{"negative concentration", valid(), ChemistryState{Concentration: [][]float64{{-1}}}, []int{0}, []float64{1}, "negative"},
		{"NaN concentration", valid(), ChemistryState{Concentration: [][]float64{{math.NaN()}}}, []int{0}, []float64{1}, "finite"},
		{"infinite concentration", valid(), ChemistryState{Concentration: [][]float64{{math.Inf(1)}}}, []int{0}, []float64{1}, "finite"},
		{"nil region map", valid(), validState, nil, []float64{1}, "region"},
		{"region outside", valid(), validState, []int{1}, []float64{1}, "region"},
		{"cell outside region map", &Receptors{Records: []Receptor{gradientReceptor([]int{1}, 0, StatusHypothesized, 1, 1, nil)}}, validState, []int{0}, []float64{1}, "cell"},
		{"explicit location outside state", &Receptors{Records: []Receptor{gradientReceptor([]int{0}, 0, StatusHypothesized, 1, 1, gradientLocation(1))}}, validState, []int{0}, []float64{1}, "location"},
		{"unsupported receptor mix", &Receptors{Mix: "mean", Records: []Receptor{gradientReceptor([]int{0}, 0, StatusHypothesized, 1, 1, nil)}}, validState, []int{0}, []float64{1}, "mix"},
		{"receptor channel outside state", &Receptors{Records: []Receptor{gradientReceptor([]int{0}, 1, StatusHypothesized, 1, 1, nil)}}, validState, []int{0}, []float64{1}, "channel"},
		{"upstream too short", valid(), validState, []int{0}, nil, "upstream"},
		{"upstream NaN", valid(), validState, []int{0}, []float64{math.NaN()}, "finite"},
		{"upstream infinity", valid(), validState, []int{0}, []float64{math.Inf(-1)}, "finite"},
		{"bad receptor status", &Receptors{Records: []Receptor{gradientReceptor([]int{0}, 0, "not-a-status", 1, 1, nil)}}, validState, []int{0}, []float64{1}, "status"},
		{"invalid receptor Kd", &Receptors{Records: []Receptor{gradientReceptor([]int{0}, 0, StatusHypothesized, 0, 1, nil)}}, validState, []int{0}, []float64{1}, "engineering_kd"},
		{"invalid receptor N", &Receptors{Records: []Receptor{gradientReceptor([]int{0}, 0, StatusHypothesized, 1, 0.5, nil)}}, validState, []int{0}, []float64{1}, "engineering_n"},
		{"gradient overflow", &Receptors{Records: []Receptor{gradientReceptor([]int{0}, 0, StatusHypothesized, 1, math.MaxFloat64, nil)}}, validState, []int{0}, []float64{4}, "finite"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.receptors.BackwardOccupancies(tc.state, tc.regionOf, tc.upstream)
			if err == nil {
				t.Fatalf("accepted invalid %s", tc.name)
			}
			if got != nil {
				t.Fatalf("refused %s with partial gradient: %v", tc.name, got)
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func gradientRecord(t *testing.T, values []float64, index int) float64 {
	t.Helper()
	if index < 0 || index >= len(values) {
		t.Fatalf("gradient record %d missing from length-%d result", index, len(values))
	}
	return values[index]
}

func TestEffectGradientHandComputedSumAndDuplicateEffects(t *testing.T) {
	occ := []OccupancyRecord{
		// Status, Assumed and Skipped are metadata for the receptor report. The
		// effect stage follows ApplyEffects and reads the occupancy value.
		{Receptor: 0, Cell: 0, Status: "metadata-only", Occupancy: 0.25, Assumed: true, Skipped: true},
		{Receptor: 0, Cell: 1, Status: StatusUnknown, Occupancy: 0.5, Skipped: true},
		{Receptor: 1, Cell: 0, Status: StatusUnresponsive, Occupancy: 0.75},
	}
	effects := []Effect{
		{Kind: EffectSensitivity, Receptor: 0, GammaScale: 0.5, BetaScale: -0.25},
		{Kind: EffectSensitivity, Receptor: 1, GammaScale: 0.5, BetaScale: -0.25},
		{Kind: EffectThreshold, Receptor: 0, ThetaScale: 0.4, ThetaAbsMax: 1},
	}
	gainUpstream := []float64{2, -3}
	offsetUpstream := []float64{5, 7}
	thresholdUpstream := []float64{-11, 13}
	wantEffects := append([]Effect(nil), effects...)
	wantOccupancies := append([]OccupancyRecord(nil), occ...)
	got, err := BackwardEffects(effects, occ, MixSum, 2, gainUpstream, offsetUpstream, thresholdUpstream)
	if err != nil {
		t.Fatalf("BackwardEffects() error = %v", err)
	}
	if !reflect.DeepEqual(effects, wantEffects) || !reflect.DeepEqual(occ, wantOccupancies) {
		t.Fatalf("BackwardEffects rewrote declarations or records: effects=%+v occ=%+v", effects, occ)
	}
	if len(got) != len(occ) {
		t.Fatalf("sum gradient has %d records, want %d", len(got), len(occ))
	}
	// Sum mixing gives cell 0 total 0.25+0.75 = 1 and cell 1 total 0.5.
	// Each record receives the upstream of every effect that references it.
	want0 := 2*0.5 + 5*(-0.25) + (-11)*0.4
	want1 := (-3)*0.5 + 7*(-0.25) + 13*0.4
	want2 := 2*0.5 + 5*(-0.25)
	gradientAssertClose(t, "sum record 0", gradientRecord(t, got, 0), want0, 1e-14, 1e-12)
	gradientAssertClose(t, "sum record 1", gradientRecord(t, got, 1), want1, 1e-14, 1e-12)
	gradientAssertClose(t, "sum record 2", gradientRecord(t, got, 2), want2, 1e-14, 1e-12)

	// Sum mixing differentiates duplicate declarations too: the same record is
	// counted once per effect by ApplyEffects, so its VJP has both terms.
	duplicateEffects := []Effect{
		{Kind: EffectSensitivity, Receptor: 0, GammaScale: 0.5, BetaScale: -0.25},
		{Kind: EffectSensitivity, Receptor: 0, GammaScale: 0.5, BetaScale: -0.25},
	}
	duplicateOcc := []OccupancyRecord{{Receptor: 0, Cell: 0, Occupancy: 0.25}}
	duplicateUpstream := []float64{2}
	duplicateOffset := []float64{3}
	duplicateThreshold := []float64{0}
	got, err = BackwardEffects(duplicateEffects, duplicateOcc, MixSum, 1, duplicateUpstream, duplicateOffset, duplicateThreshold)
	if err != nil {
		t.Fatalf("duplicate effect BackwardEffects() error = %v", err)
	}
	wantDuplicate := duplicateUpstream[0]*2*0.5 + duplicateOffset[0]*2*(-0.25)
	gradientAssertClose(t, "duplicate effect record", gradientRecord(t, got, 0), wantDuplicate, 1e-14, 1e-12)
}

func TestEffectGradientClampsHaveZeroSlopeAtAndOutsideBounds(t *testing.T) {
	occ := []OccupancyRecord{{Receptor: 0, Cell: 0, Occupancy: 0.5}}
	cases := []struct {
		name                string
		effect              Effect
		gain, offset, theta float64
		want                float64
	}{
		{"gamma inside", Effect{Kind: EffectSensitivity, Receptor: 0, GammaScale: 0.5}, 3, 0, 0, 3 * 0.5},
		{"gamma at upper bound", Effect{Kind: EffectSensitivity, Receptor: 0, GammaScale: 2}, 3, 0, 0, 0},
		{"gamma above upper bound", Effect{Kind: EffectSensitivity, Receptor: 0, GammaScale: 4}, 3, 0, 0, 0},
		{"gamma at lower bound", Effect{Kind: EffectSensitivity, Receptor: 0, GammaScale: -1}, 3, 0, 0, 0},
		{"gamma below lower bound", Effect{Kind: EffectSensitivity, Receptor: 0, GammaScale: -4}, 3, 0, 0, 0},
		{"beta inside", Effect{Kind: EffectSensitivity, Receptor: 0, BetaScale: 0.5}, 0, 3, 0, 3 * 0.5},
		{"beta at upper bound", Effect{Kind: EffectSensitivity, Receptor: 0, BetaScale: 2}, 0, 3, 0, 0},
		{"beta at lower bound", Effect{Kind: EffectSensitivity, Receptor: 0, BetaScale: -2}, 0, 3, 0, 0},
		{"beta above upper bound", Effect{Kind: EffectSensitivity, Receptor: 0, BetaScale: 4}, 0, 3, 0, 0},
		{"theta inside", Effect{Kind: EffectThreshold, Receptor: 0, ThetaScale: 0.5, ThetaAbsMax: 1}, 0, 0, 3, 3 * 0.5},
		{"theta at upper bound", Effect{Kind: EffectThreshold, Receptor: 0, ThetaScale: 1, ThetaAbsMax: 0.5}, 0, 0, 3, 0},
		{"theta below lower bound", Effect{Kind: EffectThreshold, Receptor: 0, ThetaScale: -2, ThetaAbsMax: 0.5}, 0, 0, 3, 0},
		{"theta above upper bound", Effect{Kind: EffectThreshold, Receptor: 0, ThetaScale: 2, ThetaAbsMax: 0.5}, 0, 0, 3, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BackwardEffects([]Effect{tc.effect}, occ, MixSum, 1,
				[]float64{tc.gain}, []float64{tc.offset}, []float64{tc.theta})
			if err != nil {
				t.Fatal(err)
			}
			gradientAssertClose(t, tc.name, gradientRecord(t, got, 0), tc.want, 0, 0)
		})
	}

	// Gamma and beta have separate clamp decisions. Gamma is exactly at its
	// upper bound while beta remains inside, so only beta contributes.
	got, err := BackwardEffects([]Effect{{Kind: EffectSensitivity, Receptor: 0, GammaScale: 2, BetaScale: 0.5}}, occ, MixSum, 1,
		[]float64{3}, []float64{4}, []float64{0})
	if err != nil {
		t.Fatalf("independent clamp BackwardEffects() error = %v", err)
	}
	gradientAssertClose(t, "independent gamma/beta clamp", gradientRecord(t, got, 0), 4*0.5, 0, 0)
}

func TestEffectGradientMaxChoosesFirstPositiveMaximum(t *testing.T) {
	// Effect declaration order is intentionally opposite the occupancy record
	// order. The tied record named by the first declared effect must win.
	effects := []Effect{
		{Kind: EffectSensitivity, Receptor: 1, GammaScale: 0.5, BetaScale: 0.25},
		{Kind: EffectSensitivity, Receptor: 0, GammaScale: 0.5, BetaScale: 0.25},
	}
	occ := []OccupancyRecord{
		{Receptor: 0, Cell: 0, Occupancy: 0.75},
		{Receptor: 1, Cell: 0, Occupancy: 0.75},
	}
	got, err := BackwardEffects(effects, occ, MixMax, 1, []float64{2}, []float64{3}, []float64{0})
	if err != nil {
		t.Fatalf("tie BackwardEffects() error = %v", err)
	}
	gradientAssertClose(t, "tie loser", gradientRecord(t, got, 0), 0, 0, 0)
	gradientAssertClose(t, "declared-order tie winner", gradientRecord(t, got, 1), 2*0.5+3*0.25, 0, 0)

	// Within one declaration, the first record wins a tie as well.
	got, err = BackwardEffects([]Effect{{Kind: EffectSensitivity, Receptor: 0, GammaScale: 0.5, BetaScale: 0.25}}, []OccupancyRecord{
		{Receptor: 0, Cell: 0, Occupancy: 0.5},
		{Receptor: 0, Cell: 0, Occupancy: 0.5},
	}, MixMax, 1, []float64{2}, []float64{3}, []float64{0})
	if err != nil {
		t.Fatalf("record-order tie BackwardEffects() error = %v", err)
	}
	gradientAssertClose(t, "record-order tie winner", gradientRecord(t, got, 0), 2*0.5+3*0.25, 0, 0)
	gradientAssertClose(t, "record-order tie loser", gradientRecord(t, got, 1), 0, 0, 0)

	// A zero maximum is not a positive winner, even when upstream is nonzero.
	got, err = BackwardEffects(effects, []OccupancyRecord{
		{Receptor: 0, Cell: 0, Occupancy: 0},
		{Receptor: 1, Cell: 0, Occupancy: 0},
	}, MixMax, 1, []float64{2}, []float64{3}, []float64{0})
	if err != nil {
		t.Fatalf("zero maximum BackwardEffects() error = %v", err)
	}
	gradientAssertZero(t, "zero maximum gradient", got)
}

func TestEffectGradientFiniteDifferenceAwayFromClampsAndTies(t *testing.T) {
	// The point is strictly inside all bounds. Central differences therefore
	// test the smooth formula without sampling a clamp boundary.
	effects := []Effect{
		{Kind: EffectSensitivity, Receptor: 0, GammaScale: 0.5, BetaScale: -0.4},
		{Kind: EffectThreshold, Receptor: 0, ThetaScale: 0.3, ThetaAbsMax: 1},
	}
	occ := []OccupancyRecord{{Receptor: 0, Cell: 0, Occupancy: 0.2}}
	gainUpstream := []float64{1.7}
	offsetUpstream := []float64{-2.2}
	thresholdUpstream := []float64{0.8}
	got, err := BackwardEffects(effects, occ, MixSum, 1, gainUpstream, offsetUpstream, thresholdUpstream)
	if err != nil {
		t.Fatalf("smooth-point BackwardEffects() error = %v", err)
	}
	h := 1e-6
	plus := gradientEffectObjective(t, effects, gradientPerturbOccupancy(occ, 0, h), MixSum, 1, gainUpstream, offsetUpstream, thresholdUpstream)
	minus := gradientEffectObjective(t, effects, gradientPerturbOccupancy(occ, 0, -h), MixSum, 1, gainUpstream, offsetUpstream, thresholdUpstream)
	fd := (plus - minus) / (2 * h)
	gradientAssertClose(t, "smooth effect central difference", gradientRecord(t, got, 0), fd, 1e-9, 1e-8)

	// The max point is away from both the tie and every clamp. The positive
	// maximum has a finite-difference derivative; the loser remains zero.
	maxEffects := []Effect{
		{Kind: EffectSensitivity, Receptor: 0, GammaScale: 0.5, BetaScale: 0.25},
		{Kind: EffectSensitivity, Receptor: 1, GammaScale: 0.5, BetaScale: 0.25},
	}
	maxOcc := []OccupancyRecord{
		{Receptor: 0, Cell: 0, Occupancy: 0.8},
		{Receptor: 1, Cell: 0, Occupancy: 0.2},
	}
	maxGainUpstream := []float64{1.3}
	maxOffsetUpstream := []float64{-0.4}
	maxThresholdUpstream := []float64{0}
	maxGot, err := BackwardEffects(maxEffects, maxOcc, MixMax, 1, maxGainUpstream, maxOffsetUpstream, maxThresholdUpstream)
	if err != nil {
		t.Fatalf("smooth max BackwardEffects() error = %v", err)
	}
	for index := range maxOcc {
		plus = gradientEffectObjective(t, maxEffects, gradientPerturbOccupancy(maxOcc, index, h), MixMax, 1, maxGainUpstream, maxOffsetUpstream, maxThresholdUpstream)
		minus = gradientEffectObjective(t, maxEffects, gradientPerturbOccupancy(maxOcc, index, -h), MixMax, 1, maxGainUpstream, maxOffsetUpstream, maxThresholdUpstream)
		fd = (plus - minus) / (2 * h)
		gradientAssertClose(t, "max central difference record "+string(rune('0'+index)), gradientRecord(t, maxGot, index), fd, 1e-9, 1e-8)
	}
}

func TestEffectGradientRejectsInvalidInputsAndOverflow(t *testing.T) {
	validEffects := []Effect{{Kind: EffectSensitivity, Receptor: 0, GammaScale: 0.5}}
	validOcc := []OccupancyRecord{{Receptor: 0, Cell: 0, Occupancy: 0.25}}
	validGain := []float64{1}
	validOffset := []float64{2}
	validThreshold := []float64{-3}
	cases := []struct {
		name                string
		effects             []Effect
		occ                 []OccupancyRecord
		mix                 string
		nodes               int
		gain, offset, theta []float64
		want                string
	}{
		{"unsupported mix", validEffects, validOcc, "mean", 1, validGain, validOffset, validThreshold, "mix"},
		{"zero nodes", validEffects, validOcc, MixSum, 0, []float64{}, []float64{}, []float64{}, "nodes"},
		{"unsupported effect", []Effect{{Kind: "gain", Receptor: 0}}, validOcc, MixSum, 1, validGain, validOffset, validThreshold, "kind"},
		{"negative receptor", []Effect{{Kind: EffectSensitivity, Receptor: -1}}, validOcc, MixSum, 1, validGain, validOffset, validThreshold, "receptor"},
		{"threshold without bound", []Effect{{Kind: EffectThreshold, Receptor: 0, ThetaScale: 1}}, validOcc, MixSum, 1, validGain, validOffset, validThreshold, "theta_abs_max"},
		{"nonfinite scale", []Effect{{Kind: EffectSensitivity, Receptor: 0, GammaScale: math.Inf(1)}}, validOcc, MixSum, 1, validGain, validOffset, validThreshold, "finite"},
		{"negative cell", validEffects, []OccupancyRecord{{Receptor: 0, Cell: -1, Occupancy: 0.25}}, MixSum, 1, validGain, validOffset, validThreshold, "cell"},
		{"cell outside nodes", validEffects, []OccupancyRecord{{Receptor: 0, Cell: 1, Occupancy: 0.25}}, MixSum, 1, validGain, validOffset, validThreshold, "cell"},
		{"negative occupancy", validEffects, []OccupancyRecord{{Receptor: 0, Cell: 0, Occupancy: -0.1}}, MixSum, 1, validGain, validOffset, validThreshold, "occupancy"},
		{"occupancy above one", validEffects, []OccupancyRecord{{Receptor: 0, Cell: 0, Occupancy: 1.1}}, MixSum, 1, validGain, validOffset, validThreshold, "occupancy"},
		{"NaN occupancy", validEffects, []OccupancyRecord{{Receptor: 0, Cell: 0, Occupancy: math.NaN()}}, MixSum, 1, validGain, validOffset, validThreshold, "occupancy"},
		{"infinite occupancy", validEffects, []OccupancyRecord{{Receptor: 0, Cell: 0, Occupancy: math.Inf(1)}}, MixSum, 1, validGain, validOffset, validThreshold, "occupancy"},
		{"gain shape", validEffects, validOcc, MixSum, 1, nil, validOffset, validThreshold, "gain"},
		{"offset shape", validEffects, validOcc, MixSum, 1, validGain, nil, validThreshold, "offset"},
		{"threshold shape", validEffects, validOcc, MixSum, 1, validGain, validOffset, nil, "threshold"},
		{"NaN gain upstream", validEffects, validOcc, MixSum, 1, []float64{math.NaN()}, validOffset, validThreshold, "finite"},
		{"infinite offset upstream", validEffects, validOcc, MixSum, 1, validGain, []float64{math.Inf(1)}, validThreshold, "finite"},
		{"infinite threshold upstream", validEffects, validOcc, MixSum, 1, validGain, validOffset, []float64{math.Inf(-1)}, "finite"},
		{"overlapping coefficients", []Effect{
			{Kind: EffectSensitivity, Receptor: 0, GammaScale: 0.5},
			{Kind: EffectSensitivity, Receptor: 1, GammaScale: 0.25},
		}, []OccupancyRecord{{Receptor: 0, Cell: 0, Occupancy: 0.25}, {Receptor: 1, Cell: 0, Occupancy: 0.25}}, MixSum, 1, validGain, validOffset, validThreshold, "same coefficients"},
		// The forward value stays below its explicitly widened upper bound.
		// Its slope times the upstream overflows without being clamped away.
		{"gradient overflow", []Effect{{Kind: EffectSensitivity, Receptor: 0, GammaScale: math.MaxFloat64, GammaMax: math.MaxFloat64}}, validOcc, MixSum, 1, []float64{2}, []float64{0}, []float64{0}, "finite"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BackwardEffects(tc.effects, tc.occ, tc.mix, tc.nodes, tc.gain, tc.offset, tc.theta)
			if err == nil {
				t.Fatalf("accepted invalid %s", tc.name)
			}
			if got != nil {
				t.Fatalf("refused %s with partial gradient: %v", tc.name, got)
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestEffectGradientEmptyAndZeroUpstreamAreFreshZeros(t *testing.T) {
	empty, err := BackwardEffects(nil, nil, MixSum, 3, []float64{1, -2, 3}, []float64{4, 5, -6}, []float64{-7, 8, 9})
	if err != nil {
		t.Fatalf("empty BackwardEffects() error = %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Fatalf("empty BackwardEffects() returned %v, want a fresh zero-length slice", empty)
	}
	nonemptyDeclarations, err := BackwardEffects(
		[]Effect{{Kind: EffectSensitivity, Receptor: 0, GammaScale: 0.5}}, nil, MixMax, 3,
		[]float64{1, 2, 3}, []float64{4, 5, 6}, []float64{7, 8, 9},
	)
	if err != nil {
		t.Fatalf("empty occupancy with declarations BackwardEffects() error = %v", err)
	}
	if nonemptyDeclarations == nil || len(nonemptyDeclarations) != 0 {
		t.Fatalf("empty occupancy with declarations returned %v, want a fresh zero-length slice", nonemptyDeclarations)
	}

	effects := []Effect{
		{Kind: EffectSensitivity, Receptor: 0, GammaScale: 0.5, BetaScale: -0.25},
		{Kind: EffectThreshold, Receptor: 1, ThetaScale: 0.5, ThetaAbsMax: 1},
	}
	occ := []OccupancyRecord{
		{Receptor: 0, Cell: 0, Occupancy: 0.25},
		{Receptor: 1, Cell: 1, Occupancy: 0.75},
	}
	zeroGain := []float64{0, 0}
	zeroOffset := []float64{0, 0}
	zeroThreshold := []float64{0, 0}
	got, err := BackwardEffects(effects, occ, MixSum, 2, zeroGain, zeroOffset, zeroThreshold)
	if err != nil {
		t.Fatalf("all-zero upstream BackwardEffects() error = %v", err)
	}
	if got == nil || len(got) != len(occ) {
		t.Fatalf("all-zero upstream result = %v, want own length-%d slice", got, len(occ))
	}
	gradientAssertZero(t, "all-zero effect gradient", got)

	got[0] = 42
	again, err := BackwardEffects(effects, occ, MixSum, 2, zeroGain, zeroOffset, zeroThreshold)
	if err != nil {
		t.Fatalf("second all-zero BackwardEffects() error = %v", err)
	}
	gradientAssertZero(t, "fresh all-zero effect gradient", again)
	changed, err := BackwardEffects(effects, occ, MixSum, 2, []float64{1, 0}, zeroOffset, zeroThreshold)
	if err != nil {
		t.Fatalf("changed-upstream BackwardEffects() error = %v", err)
	}
	if changed[0] == 0 {
		t.Fatal("different upstream unexpectedly reused the all-zero gradient")
	}
}
