package modulation

import (
	"math"
	"reflect"
	"testing"
)

func cloneClearanceGrid(grid [][]float64) [][]float64 {
	if grid == nil {
		return nil
	}
	copyGrid := make([][]float64, len(grid))
	for r, row := range grid {
		copyGrid[r] = append([]float64(nil), row...)
	}
	return copyGrid
}

func cloneClearanceState(state ChemistryState) ChemistryState {
	state.Concentration = cloneClearanceGrid(state.Concentration)
	return state
}

func assertClearanceClose(t *testing.T, name string, got, want, tolerance float64) {
	t.Helper()
	if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got-want) > tolerance {
		t.Fatalf("%s = %.17g, want %.17g within %.3g", name, got, want, tolerance)
	}
}

func newClearanceOverflowFixture(t *testing.T) (*Kinetics, ChemistryState, [][]float64, [][]float64, [][]float64) {
	t.Helper()
	k, err := NewChemistry(Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{1e-308}})
	if err != nil {
		t.Fatal(err)
	}
	state := k.NewState()
	state.Concentration[0][0] = 7
	release := [][]float64{{1e308}}
	boost := [][]float64{{1e308}}
	upstream := [][]float64{{-3.5}}
	return k, state, release, boost, upstream
}

func TestKineticsRejectsClearanceRateOverflowWithoutMutation(t *testing.T) {
	t.Run("Step", func(t *testing.T) {
		k, state, release, boost, _ := newClearanceOverflowFixture(t)
		wantState := cloneClearanceState(state)
		wantRelease := cloneClearanceGrid(release)
		wantBoost := cloneClearanceGrid(boost)

		got, stepErr := k.Step(state, release, boost)
		if stepErr == nil {
			t.Fatal("Step accepted a finite release and boost whose derived rate overflows")
		}
		if !reflect.DeepEqual(got, ChemistryState{}) {
			t.Fatalf("Step returned a non-empty state on error: %+v", got)
		}
		if !reflect.DeepEqual(state, wantState) {
			t.Fatalf("Step changed the caller state on error: got %+v, want %+v", state, wantState)
		}
		if !reflect.DeepEqual(release, wantRelease) || !reflect.DeepEqual(boost, wantBoost) {
			t.Fatalf("Step changed release or boost on error: release=%v boost=%v", release, boost)
		}
	})

	t.Run("Backward", func(t *testing.T) {
		k, state, release, boost, upstream := newClearanceOverflowFixture(t)
		wantState := cloneClearanceState(state)
		wantRelease := cloneClearanceGrid(release)
		wantBoost := cloneClearanceGrid(boost)
		wantUpstream := cloneClearanceGrid(upstream)

		got, backwardErr := k.Backward(state, release, boost, upstream)
		if backwardErr == nil {
			t.Fatal("Backward accepted the same finite inputs whose derived rate overflows")
		}
		if !reflect.DeepEqual(got, ChemistryGradient{}) {
			t.Fatalf("Backward returned a non-empty gradient on error: %+v", got)
		}
		if !reflect.DeepEqual(state, wantState) || !reflect.DeepEqual(release, wantRelease) || !reflect.DeepEqual(boost, wantBoost) || !reflect.DeepEqual(upstream, wantUpstream) {
			t.Fatalf("Backward changed an input on error: state=%+v release=%v boost=%v upstream=%v", state, release, boost, upstream)
		}
	})
}

func TestKineticsAcceptsLegalClearanceBoundaryCases(t *testing.T) {
	// With tau=4 and dt=1, lambda=exp(-0.25). Symmetric half-half transport
	// sends the local [8*lambda, 0] result to [4*lambda, 4*lambda]. A nil
	// boost and an explicitly zero boost must use the same path and result.
	transported, err := NewChemistry(Chemistry{
		Regions: 2, Channels: 1, DT: 1, Tau: []float64{4},
		Transport: &Transport{Fraction: [][]float64{{0, 0.5}, {0.5, 0}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	transportState := transported.NewState()
	transportState.Concentration[0][0] = 8
	transportRelease := [][]float64{{0}, {0}}
	zeroBoost := [][]float64{{0}, {0}}
	plain, err := transported.Step(transportState, transportRelease, nil)
	if err != nil {
		t.Fatal(err)
	}
	zero, err := transported.Step(transportState, transportRelease, zeroBoost)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plain, zero) {
		t.Fatalf("nil and zero clearance boost differ: nil=%+v zero=%+v", plain, zero)
	}
	lambda := math.Exp(-0.25)
	assertClearanceClose(t, "transported region 0", plain.Concentration[0][0], 4*lambda, 1e-15)
	assertClearanceClose(t, "transported region 1", plain.Concentration[1][0], 4*lambda, 1e-15)

	upstream := [][]float64{{2}, {-3}}
	plainGradient, err := transported.Backward(transportState, transportRelease, nil, upstream)
	if err != nil {
		t.Fatal(err)
	}
	zeroGradient, err := transported.Backward(transportState, transportRelease, zeroBoost, upstream)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plainGradient, zeroGradient) {
		t.Fatalf("nil and zero clearance boost gradients differ: nil=%+v zero=%+v", plainGradient, zeroGradient)
	}
	// The transport transpose maps [2,-3] to [-0.5,-0.5].
	assertClearanceClose(t, "transport concentration gradient region 0", plainGradient.Concentration[0][0], -0.5*lambda, 1e-15)
	assertClearanceClose(t, "transport concentration gradient region 1", plainGradient.Concentration[1][0], -0.5*lambda, 1e-15)
	assertClearanceClose(t, "transport release gradient region 0", plainGradient.Release[0][0], -2*(1-lambda), 1e-15)
	assertClearanceClose(t, "transport release gradient region 1", plainGradient.Release[1][0], -2*(1-lambda), 1e-15)

	// A finite, ordinary boost has rate=1/4+3/4=1. From c=8 and q=1,
	// the declared rule is 8*exp(-1)+(1-exp(-1))*1.
	boosted, err := NewChemistry(Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{4}})
	if err != nil {
		t.Fatal(err)
	}
	boostState := boosted.NewState()
	boostState.Concentration[0][0] = 8
	boostedState, err := boosted.Step(boostState, [][]float64{{1}}, [][]float64{{0.75}})
	if err != nil {
		t.Fatal(err)
	}
	expMinusOne := math.Exp(-1)
	assertClearanceClose(t, "finite boosted concentration", boostedState.Concentration[0][0], 8*expMinusOne+(1-expMinusOne), 1e-15)
	boostedGradient, err := boosted.Backward(boostState, [][]float64{{1}}, [][]float64{{0.75}}, [][]float64{{1}})
	if err != nil {
		t.Fatal(err)
	}
	assertClearanceClose(t, "finite boosted concentration gradient", boostedGradient.Concentration[0][0], expMinusOne, 1e-15)
	assertClearanceClose(t, "finite boosted release gradient", boostedGradient.Release[0][0], 1-expMinusOne, 1e-15)

	// rate=1e200+1e200=2e200 is finite, while exp(-2e200) saturates to
	// zero. The steady state q/rate remains the finite hand value 0.5 and
	// the release derivative remains the non-zero finite 5e-201.
	highRate, err := NewChemistry(Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{1e-200}})
	if err != nil {
		t.Fatal(err)
	}
	highState := highRate.NewState()
	highState.Concentration[0][0] = 7
	highRelease := [][]float64{{1e200}}
	highBoost := [][]float64{{1e200}}
	highStep, err := highRate.Step(highState, highRelease, highBoost)
	if err != nil {
		t.Fatal(err)
	}
	assertClearanceClose(t, "finite saturated concentration", highStep.Concentration[0][0], 0.5, 1e-15)
	highGradient, err := highRate.Backward(highState, highRelease, highBoost, [][]float64{{1}})
	if err != nil {
		t.Fatal(err)
	}
	assertClearanceClose(t, "finite saturated concentration gradient", highGradient.Concentration[0][0], 0, 0)
	assertClearanceClose(t, "finite saturated release gradient", highGradient.Release[0][0], 5e-201, 1e-215)
}
