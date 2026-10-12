package modulation

import (
	"math"
	"reflect"
	"testing"
)

func TestKineticsBackwardHandComputedTransportTranspose(t *testing.T) {
	c := Chemistry{
		Regions:  2,
		Channels: 1,
		DT:       math.Ln2,
		Tau:      []float64{1},
		Transport: &Transport{Fraction: [][]float64{
			{0, 0.25},
			{0.5, 0},
		}},
	}
	k, err := NewChemistry(c)
	if err != nil {
		t.Fatal(err)
	}
	state := k.NewState()
	state.Concentration[0][0] = 1.5
	state.Concentration[1][0] = 0.75
	release := [][]float64{{0.2}, {0.4}}
	upstream := [][]float64{{2}, {-3}}

	got, err := k.Backward(state, release, nil, upstream)
	if err != nil {
		t.Fatal(err)
	}
	assertKineticsGradientShape(t, got, 2, 1)
	// For transport [[0,.25],[.5,0]], the transpose sends the signed
	// upstream [2,-3] to [2*.75 + (-3)*.25, (-3)*.5 + 2*.5] = [.75,-.5].
	assertKineticsGradientClose(t, got.Concentration[0][0], 0.375)
	assertKineticsGradientClose(t, got.Concentration[1][0], -0.25)
	assertKineticsGradientClose(t, got.Release[0][0], 0.375)
	assertKineticsGradientClose(t, got.Release[1][0], -0.25)
}

func TestKineticsBackwardBoostUsesRateAndSignedUpstream(t *testing.T) {
	k, err := NewChemistry(Chemistry{
		Regions:  2,
		Channels: 1,
		DT:       math.Ln2,
		Tau:      []float64{1},
	})
	if err != nil {
		t.Fatal(err)
	}
	state := k.NewState()
	state.Concentration[0][0] = 0.7
	state.Concentration[1][0] = 1.1
	release := [][]float64{{0.4}, {0.9}}
	boost := [][]float64{{1}, {1}}
	upstream := [][]float64{{2}, {-3}}

	got, err := k.Backward(state, release, boost, upstream)
	if err != nil {
		t.Fatal(err)
	}
	assertKineticsGradientShape(t, got, 2, 1)
	// rate = 1/tau + boost = 2, lambda = exp(-ln(2)*2) = .25.
	// The release coefficient is (1-lambda)/rate = .75/2 = .375.
	assertKineticsGradientClose(t, got.Concentration[0][0], 0.5)
	assertKineticsGradientClose(t, got.Concentration[1][0], -0.75)
	assertKineticsGradientClose(t, got.Release[0][0], 0.75)
	assertKineticsGradientClose(t, got.Release[1][0], -1.125)
}

func TestKineticsBackwardNilAndZeroBoostAreBitIdentical(t *testing.T) {
	k, in := newKineticsBackwardFixture(t)
	zeroBoost := make([][]float64, len(in.boost))
	for r := range in.boost {
		zeroBoost[r] = make([]float64, len(in.boost[r]))
	}

	nilBoost, err := k.Backward(in.state, in.release, nil, in.upstream)
	if err != nil {
		t.Fatal(err)
	}
	zero, err := k.Backward(in.state, in.release, zeroBoost, in.upstream)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(nilBoost, zero) {
		t.Fatalf("nil boost gradient %+v differs from zero boost gradient %+v", nilBoost, zero)
	}
}

func TestKineticsBackwardMatchesCentralDifferencesMultiChannel(t *testing.T) {
	k, in := newKineticsBackwardFixture(t)
	in.state.Steps = 5
	got, err := k.Backward(in.state, in.release, in.boost, in.upstream)
	if err != nil {
		t.Fatal(err)
	}
	assertKineticsGradientShape(t, got, 3, 3)

	const epsilon = 1e-6
	for r := range in.state.Concentration {
		for ch := range in.state.Concentration[r] {
			plus := in.state
			plus.Concentration = cloneKineticsGrid(in.state.Concentration)
			plus.Concentration[r][ch] += epsilon
			minus := in.state
			minus.Concentration = cloneKineticsGrid(in.state.Concentration)
			minus.Concentration[r][ch] -= epsilon
			want := (kineticsBackwardObjective(t, k, plus, in.release, in.boost, in.upstream) -
				kineticsBackwardObjective(t, k, minus, in.release, in.boost, in.upstream)) / (2 * epsilon)
			assertKineticsGradientCloseTo(t, "concentration", r, ch, got.Concentration[r][ch], want)
		}
	}
	for r := range in.release {
		for ch := range in.release[r] {
			plus := cloneKineticsGrid(in.release)
			plus[r][ch] += epsilon
			minus := cloneKineticsGrid(in.release)
			minus[r][ch] -= epsilon
			want := (kineticsBackwardObjective(t, k, in.state, plus, in.boost, in.upstream) -
				kineticsBackwardObjective(t, k, in.state, minus, in.boost, in.upstream)) / (2 * epsilon)
			assertKineticsGradientCloseTo(t, "release", r, ch, got.Release[r][ch], want)
		}
	}
}

func TestKineticsBackwardRejectsInvalidInputsWithEmptyResult(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*kineticsBackwardInputs)
	}{
		{"nil upstream", func(in *kineticsBackwardInputs) { in.upstream = nil }},
		{"upstream row shape", func(in *kineticsBackwardInputs) { in.upstream = [][]float64{{1}} }},
		{"upstream column shape", func(in *kineticsBackwardInputs) {
			in.upstream = [][]float64{{1, 2}, {3, 4}}
		}},
		{"upstream NaN", func(in *kineticsBackwardInputs) {
			in.upstream[0][0] = math.NaN()
		}},
		{"upstream positive infinity", func(in *kineticsBackwardInputs) {
			in.upstream[0][0] = math.Inf(1)
		}},
		{"upstream negative infinity", func(in *kineticsBackwardInputs) {
			in.upstream[0][0] = math.Inf(-1)
		}},
		{"state row shape", func(in *kineticsBackwardInputs) {
			in.state.Concentration = [][]float64{{1}}
		}},
		{"nil state concentration", func(in *kineticsBackwardInputs) {
			in.state.Concentration = nil
		}},
		{"state column shape", func(in *kineticsBackwardInputs) {
			in.state.Concentration = [][]float64{{1, 2}, {3, 4}}
		}},
		{"negative state", func(in *kineticsBackwardInputs) {
			in.state.Concentration[0][0] = -1
		}},
		{"NaN state", func(in *kineticsBackwardInputs) {
			in.state.Concentration[0][0] = math.NaN()
		}},
		{"infinite state", func(in *kineticsBackwardInputs) {
			in.state.Concentration[0][0] = math.Inf(1)
		}},
		{"nil release", func(in *kineticsBackwardInputs) { in.release = nil }},
		{"release row shape", func(in *kineticsBackwardInputs) { in.release = [][]float64{{1}} }},
		{"release column shape", func(in *kineticsBackwardInputs) {
			in.release = [][]float64{{1, 2}, {3, 4}}
		}},
		{"negative release", func(in *kineticsBackwardInputs) {
			in.release[0][0] = -1
		}},
		{"NaN release", func(in *kineticsBackwardInputs) {
			in.release[0][0] = math.NaN()
		}},
		{"infinite release", func(in *kineticsBackwardInputs) {
			in.release[0][0] = math.Inf(1)
		}},
		{"boost row shape", func(in *kineticsBackwardInputs) { in.boost = [][]float64{{1}} }},
		{"boost column shape", func(in *kineticsBackwardInputs) {
			in.boost = [][]float64{{1, 2}, {3, 4}}
		}},
		{"negative boost", func(in *kineticsBackwardInputs) {
			in.boost[0][0] = -1
		}},
		{"NaN boost", func(in *kineticsBackwardInputs) {
			in.boost[0][0] = math.NaN()
		}},
		{"infinite boost", func(in *kineticsBackwardInputs) {
			in.boost[0][0] = math.Inf(1)
		}},
		{"step counter overflow", func(in *kineticsBackwardInputs) {
			in.state.Steps = math.MaxUint64
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k, in := newKineticsBackwardFixture(t)
			tc.mutate(in)
			got, err := k.Backward(in.state, in.release, in.boost, in.upstream)
			if err == nil {
				t.Fatalf("accepted invalid %s: %+v", tc.name, got)
			}
			assertEmptyKineticsGradient(t, got)
		})
	}
}

func TestKineticsBackwardRejectsNilAndZeroKinetics(t *testing.T) {
	_, in := newKineticsBackwardFixture(t)
	var nilKinetics *Kinetics
	got, err := nilKinetics.Backward(in.state, in.release, in.boost, in.upstream)
	if err == nil {
		t.Fatalf("nil kinetics was accepted: %+v", got)
	}
	assertEmptyKineticsGradient(t, got)

	got, err = (&Kinetics{}).Backward(in.state, in.release, in.boost, in.upstream)
	if err == nil {
		t.Fatalf("zero kinetics was accepted: %+v", got)
	}
	assertEmptyKineticsGradient(t, got)
}

func TestKineticsBackwardRejectsGradientOverflow(t *testing.T) {
	k, err := NewChemistry(Chemistry{Regions: 1, Channels: 1, DT: 10, Tau: []float64{2}})
	if err != nil {
		t.Fatal(err)
	}
	state := k.NewState()
	state.Concentration[0][0] = 1
	release := [][]float64{{1}}
	if _, err := k.Step(state, release, nil); err != nil {
		t.Fatalf("overflow fixture is not a legal forward step: %v", err)
	}

	got, err := k.Backward(state, release, nil, [][]float64{{math.MaxFloat64}})
	if err == nil {
		t.Fatalf("accepted an overflowing gradient: %+v", got)
	}
	assertEmptyKineticsGradient(t, got)
}

func TestKineticsBackwardOwnsOutputAndPreservesInputs(t *testing.T) {
	k, in := newKineticsBackwardFixture(t)
	stateBefore := cloneKineticsState(in.state)
	releaseBefore := cloneKineticsGrid(in.release)
	boostBefore := cloneKineticsGrid(in.boost)
	upstreamBefore := cloneKineticsGrid(in.upstream)

	got, err := k.Backward(in.state, in.release, in.boost, in.upstream)
	if err != nil {
		t.Fatal(err)
	}
	assertKineticsGradientShape(t, got, 3, 3)
	if !reflect.DeepEqual(in.state, stateBefore) ||
		!reflect.DeepEqual(in.release, releaseBefore) ||
		!reflect.DeepEqual(in.boost, boostBefore) ||
		!reflect.DeepEqual(in.upstream, upstreamBefore) {
		t.Fatal("Backward modified one of its inputs")
	}

	firstConcentration := cloneKineticsGrid(got.Concentration)
	firstRelease := cloneKineticsGrid(got.Release)
	negativeUpstream := negateKineticsGrid(in.upstream)
	second, err := k.Backward(in.state, in.release, in.boost, negativeUpstream)
	if err != nil {
		t.Fatal(err)
	}
	for r := range firstConcentration {
		for ch := range firstConcentration[r] {
			assertKineticsGradientClose(t, second.Concentration[r][ch], -firstConcentration[r][ch])
			assertKineticsGradientClose(t, second.Release[r][ch], -firstRelease[r][ch])
		}
	}

	// Each returned matrix is caller-owned, and a later reverse pass does not
	// reuse or expose the storage returned by an earlier pass.
	got.Concentration[0][0] = 123
	if got.Release[0][0] != firstRelease[0][0] {
		t.Fatal("concentration and release outputs share storage")
	}
	got.Release[0][0] = -456
	if !reflect.DeepEqual(in.state, stateBefore) ||
		!reflect.DeepEqual(in.release, releaseBefore) ||
		!reflect.DeepEqual(in.boost, boostBefore) ||
		!reflect.DeepEqual(in.upstream, upstreamBefore) {
		t.Fatal("mutating a returned gradient changed an input")
	}
	if second.Concentration[0][0] == 123 || second.Release[0][0] == -456 {
		t.Fatal("separate Backward results share output storage")
	}
	again, err := k.Backward(in.state, in.release, in.boost, in.upstream)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again.Concentration, firstConcentration) ||
		!reflect.DeepEqual(again.Release, firstRelease) {
		t.Fatal("mutating a returned gradient changed a later result")
	}
}

type kineticsBackwardInputs struct {
	state                    ChemistryState
	release, boost, upstream [][]float64
}

func newKineticsBackwardFixture(t *testing.T) (*Kinetics, *kineticsBackwardInputs) {
	t.Helper()
	k, err := NewChemistry(Chemistry{
		Regions:  3,
		Channels: 3,
		DT:       0.37,
		Tau:      []float64{0.7, 1.9, 3.2},
		Transport: &Transport{Fraction: [][]float64{
			{0, 0.1, 0.2},
			{0.25, 0, 0.15},
			{0.05, 0.2, 0},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	state := k.NewState()
	state.Concentration = [][]float64{
		{0.9, 1.4, 0.6},
		{1.2, 0.8, 1.7},
		{0.5, 1.1, 1.3},
	}
	return k, &kineticsBackwardInputs{
		state: state,
		release: [][]float64{
			{0.35, 0.65, 0.25},
			{0.55, 0.4, 0.75},
			{0.45, 0.3, 0.6},
		},
		boost: [][]float64{
			{0, 0.23, 1.1},
			{0.4, 0, 0.8},
			{0.2, 0.7, 0},
		},
		upstream: [][]float64{
			{1.3, -0.9, 0.6},
			{-1.1, 0.8, -0.7},
			{0.5, -1.2, 1.4},
		},
	}
}

func kineticsBackwardObjective(t *testing.T, k *Kinetics, state ChemistryState, release, boost, upstream [][]float64) float64 {
	t.Helper()
	next, err := k.Step(state, release, boost)
	if err != nil {
		t.Fatalf("finite-difference forward step failed: %v", err)
	}
	var objective float64
	for r := range next.Concentration {
		for ch := range next.Concentration[r] {
			objective += next.Concentration[r][ch] * upstream[r][ch]
		}
	}
	return objective
}

func cloneKineticsGrid(grid [][]float64) [][]float64 {
	if grid == nil {
		return nil
	}
	clone := make([][]float64, len(grid))
	for r, row := range grid {
		clone[r] = append([]float64(nil), row...)
	}
	return clone
}

func cloneKineticsState(state ChemistryState) ChemistryState {
	state.Concentration = cloneKineticsGrid(state.Concentration)
	return state
}

func negateKineticsGrid(grid [][]float64) [][]float64 {
	negated := cloneKineticsGrid(grid)
	for r := range negated {
		for ch := range negated[r] {
			negated[r][ch] = -negated[r][ch]
		}
	}
	return negated
}

func assertEmptyKineticsGradient(t *testing.T, got ChemistryGradient) {
	t.Helper()
	if got.Concentration != nil || got.Release != nil {
		t.Fatalf("error returned partial gradient: %+v", got)
	}
}

func assertKineticsGradientShape(t *testing.T, got ChemistryGradient, regions, channels int) {
	t.Helper()
	if len(got.Concentration) != regions || len(got.Release) != regions {
		t.Fatalf("gradient has concentration rows %d and release rows %d, want %d", len(got.Concentration), len(got.Release), regions)
	}
	for r := 0; r < regions; r++ {
		if len(got.Concentration[r]) != channels || len(got.Release[r]) != channels {
			t.Fatalf("gradient region %d has concentration width %d and release width %d, want %d", r, len(got.Concentration[r]), len(got.Release[r]), channels)
		}
	}
}

func assertKineticsGradientClose(t *testing.T, got, want float64) {
	t.Helper()
	if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got-want) > 1e-14 {
		t.Fatalf("gradient = %.17g, want %.17g", got, want)
	}
}

func assertKineticsGradientCloseTo(t *testing.T, field string, region, channel int, got, want float64) {
	t.Helper()
	if math.IsNaN(got) || math.IsInf(got, 0) || math.IsNaN(want) || math.IsInf(want, 0) || math.Abs(got-want) > 5e-7+5e-5*math.Abs(want) {
		t.Fatalf("%s[%d][%d] = %.17g, central difference %.17g", field, region, channel, got, want)
	}
}
