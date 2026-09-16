package modulation

import (
	"math"
	"math/rand/v2"
	"strings"
	"testing"
)

// oneChannel is the reference declaration of every hand-computed number in this
// file: dt = 1 and tau = 4, so dt/tau = 0.25 and
//
//	lambda = exp(-0.25) = 0.7788007830714049
//	1 - lambda          = 0.22119921692859512
func oneChannel(regions int) Chemistry {
	return Chemistry{Regions: regions, Channels: 1, DT: 1, Tau: []float64{4}}
}

const (
	lambdaQuarter = 0.7788007830714049 // exp(-0.25)
	expMinusOne   = 0.36787944117144233
)

func relativeError(got, want float64) float64 {
	if want == 0 {
		return math.Abs(got)
	}
	return math.Abs(got-want) / math.Abs(want)
}

// A constant release converges to tau*q. With c(0) = 0 the recurrence
// c' = lambda*c + tau*(1-lambda)*q has the closed form
// c(m) = tau*q*(1 - lambda^m), so the relative distance from tau*q after m
// steps is exactly lambda^m = exp(-m*dt/tau). For dt/tau = 0.25 that is
// exp(-15) = 3.0590232050182594e-07 after 60 steps, already under the 1e-6 the
// ticket asks for, and exp(-50) = 1.9e-22 after 200.
func TestKineticsReachesTheHandComputedSteadyState(t *testing.T) {
	k, err := NewChemistry(oneChannel(1))
	if err != nil {
		t.Fatal(err)
	}
	release := [][]float64{{2}} // q = 2, so tau*q = 8
	state := k.NewState()
	for step := range 200 {
		if state, err = k.Step(state, release, nil); err != nil {
			t.Fatal(err)
		}
		switch step + 1 {
		case 60:
			bound := math.Pow(lambdaQuarter, 60)
			if bound >= 1e-6 {
				t.Fatalf("the geometric bound lambda^60 is %v, which does not prove the 1e-6 claim", bound)
			}
			if got := relativeError(state.Concentration[0][0], 8); got > bound {
				t.Fatalf("after 60 steps the relative error is %v, want at most the geometric bound %v", got, bound)
			}
		case 200:
			if got := relativeError(state.Concentration[0][0], 8); got > 1e-6 {
				t.Fatalf("after 200 steps c = %v, relative error %v, want under 1e-6 of tau*q = 8", state.Concentration[0][0], got)
			}
		}
	}
	if state.Steps != 200 {
		t.Fatalf("steps %d, want 200", state.Steps)
	}
}

// With q = 0 one step multiplies the concentration by exactly lambda.
// 8*exp(-0.25) = 6.230406264571239 and 8*exp(-0.5) = 4.852245277701067.
func TestKineticsClearsExponentiallyWithoutRelease(t *testing.T) {
	k, err := NewChemistry(oneChannel(1))
	if err != nil {
		t.Fatal(err)
	}
	state := k.NewState()
	state.Concentration[0][0] = 8
	state, err = k.Step(state, [][]float64{{0}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := state.Concentration[0][0]; got != 8*lambdaQuarter {
		t.Fatalf("one step of decay gave %v, want 8*lambda = %v", got, 8*lambdaQuarter)
	}
	state, err = k.Step(state, [][]float64{{0}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := state.Concentration[0][0]; got != 8*lambdaQuarter*lambdaQuarter {
		t.Fatalf("two steps of decay gave %v, want 8*lambda^2 = %v", got, 8*lambdaQuarter*lambdaQuarter)
	}
}

// The neutral state is exactly neutral: zero concentration with zero release
// stays at positive zero, not at a rounding crumb and not at a negative zero.
func TestKineticsNeutralStateStaysExactlyZero(t *testing.T) {
	c := oneChannel(2)
	c.Channels = 2
	c.Tau = []float64{4, 0.5}
	c.Transport = &Transport{Fraction: [][]float64{{0, 0.25}, {0.5, 0}}}
	k, err := NewChemistry(c)
	if err != nil {
		t.Fatal(err)
	}
	state := k.NewState()
	release := [][]float64{{0, 0}, {0, 0}}
	for step := range 100 {
		if state, err = k.Step(state, release, nil); err != nil {
			t.Fatal(err)
		}
		for r, row := range state.Concentration {
			for ch, v := range row {
				if math.Float64bits(v) != 0 {
					t.Fatalf("step %d region %d channel %d is %v (bits %#x), want a positive zero", step, r, ch, v, math.Float64bits(v))
				}
			}
		}
	}
}

func TestKineticsStaysNonNegativeOverRandomRelease(t *testing.T) {
	c := Chemistry{Regions: 3, Channels: 2, DT: 0.5, Tau: []float64{4, 0.25},
		Transport: &Transport{Fraction: [][]float64{{0, 0.25, 0.5}, {0.125, 0, 0}, {0, 0.75, 0}}}}
	k, err := NewChemistry(c)
	if err != nil {
		t.Fatal(err)
	}
	prng := rand.New(rand.NewPCG(21, 0))
	state := k.NewState()
	release := make([][]float64, c.Regions)
	for r := range release {
		release[r] = make([]float64, c.Channels)
	}
	for step := range 1000 {
		for r := range release {
			for ch := range release[r] {
				release[r][ch] = prng.Float64() * 3
			}
		}
		if state, err = k.Step(state, release, nil); err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		for r, row := range state.Concentration {
			for ch, v := range row {
				if v < 0 || !finite(v) {
					t.Fatalf("step %d region %d channel %d is %v", step, r, ch, v)
				}
			}
		}
	}
}

// Transport is a redistribution, never a source. The symmetric half-half case
// is exact in binary: from c' = [8*lambda, 0] both regions receive
// 4*lambda = 3.1152031322856195 and the total is unchanged to the last bit.
func TestKineticsTransportMovesMassWithoutCreatingIt(t *testing.T) {
	c := oneChannel(2)
	c.Transport = &Transport{Fraction: [][]float64{{0, 0.5}, {0.5, 0}}}
	k, err := NewChemistry(c)
	if err != nil {
		t.Fatal(err)
	}
	state := k.NewState()
	state.Concentration[0][0] = 8
	before := state.Concentration[0][0] + state.Concentration[1][0]
	state, err = k.Step(state, [][]float64{{0}, {0}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	half := 4 * lambdaQuarter
	if state.Concentration[0][0] != half || state.Concentration[1][0] != half {
		t.Fatalf("transport gave [%v %v], want [%v %v]", state.Concentration[0][0], state.Concentration[1][0], half, half)
	}
	after := state.Concentration[0][0] + state.Concentration[1][0]
	if after != 8*lambdaQuarter {
		t.Fatalf("total after transport is %v, want the decayed total %v", after, 8*lambdaQuarter)
	}
	if after > before {
		t.Fatalf("transport raised the total from %v to %v", before, after)
	}

	// An asymmetric declaration over many steps: the total after a step may
	// never exceed the total the local update produced, within rounding.
	c.Transport = &Transport{Fraction: [][]float64{{0, 0.25}, {0.5, 0}}}
	k, err = NewChemistry(c)
	if err != nil {
		t.Fatal(err)
	}
	state = k.NewState()
	prng := rand.New(rand.NewPCG(2, 1))
	for step := range 200 {
		release := [][]float64{{prng.Float64()}, {prng.Float64()}}
		previous := state.Concentration[0][0] + state.Concentration[1][0]
		if state, err = k.Step(state, release, nil); err != nil {
			t.Fatal(err)
		}
		// Local update bound: c' <= lambda*c + tau*(1-lambda)*q per region,
		// and transport cannot raise the sum above that.
		bound := lambdaQuarter*previous + 4*(1-lambdaQuarter)*(release[0][0]+release[1][0])
		total := state.Concentration[0][0] + state.Concentration[1][0]
		if total > bound*(1+1e-12) {
			t.Fatalf("step %d total %v exceeds the local-update bound %v", step, total, bound)
		}
	}
}

// A clearance boost b adds to the clearance rate: the step uses
// rate = 1/tau + b, lambda_b = exp(-dt*rate) and the same steady state
// q/rate. With tau = 4 and b = 0.75 the rate is exactly 1, so one step from
// c = 8 with q = 0 gives 8*exp(-1) = 2.9430355293715387, well below the
// unboosted 8*exp(-0.25) = 6.230406264571239. With q = 1 the same step gives
// 8*exp(-1) + (1-exp(-1))*1 = 3.5751560882000963.
func TestKineticsClearanceBoostSpeedsDecay(t *testing.T) {
	k, err := NewChemistry(oneChannel(1))
	if err != nil {
		t.Fatal(err)
	}
	start := k.NewState()
	start.Concentration[0][0] = 8

	boosted, err := k.Step(start, [][]float64{{0}}, [][]float64{{0.75}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := boosted.Concentration[0][0], 8*expMinusOne; relativeError(got, want) > 1e-15 {
		t.Fatalf("boosted decay gave %v, want 8*exp(-1) = %v", got, want)
	}
	if boosted.Concentration[0][0] >= 8*lambdaQuarter {
		t.Fatalf("a boost did not speed the decay: %v is not below %v", boosted.Concentration[0][0], 8*lambdaQuarter)
	}

	withRelease, err := k.Step(start, [][]float64{{1}}, [][]float64{{0.75}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := withRelease.Concentration[0][0], 3.5751560882000963; relativeError(got, want) > 1e-15 {
		t.Fatalf("boosted step with q = 1 gave %v, want %v", got, want)
	}

	// A zero boost is exactly the unboosted step, bit for bit.
	zero, err := k.Step(start, [][]float64{{1}}, [][]float64{{0}})
	if err != nil {
		t.Fatal(err)
	}
	plain, err := k.Step(start, [][]float64{{1}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if zero.Concentration[0][0] != plain.Concentration[0][0] {
		t.Fatalf("a zero boost gave %v, want the unboosted %v", zero.Concentration[0][0], plain.Concentration[0][0])
	}
}

func TestNewChemistryRejectsAnInvalidDeclaration(t *testing.T) {
	good := Chemistry{Regions: 2, Channels: 1, DT: 1, Tau: []float64{4},
		Transport: &Transport{Fraction: [][]float64{{0, 0.25}, {0.5, 0}}}}
	if _, err := NewChemistry(good); err != nil {
		t.Fatalf("the reference declaration was refused: %v", err)
	}
	cases := []struct {
		name string
		edit func(*Chemistry)
		want string
	}{
		{"no region", func(c *Chemistry) { c.Regions = 0 }, "regions"},
		{"no channel", func(c *Chemistry) { c.Channels = 0; c.Tau = nil }, "channels"},
		{"zero dt", func(c *Chemistry) { c.DT = 0 }, "dt"},
		{"non-finite dt", func(c *Chemistry) { c.DT = math.Inf(1) }, "dt"},
		{"tau count", func(c *Chemistry) { c.Tau = []float64{4, 4} }, "tau"},
		{"zero tau", func(c *Chemistry) { c.Tau = []float64{0} }, "tau"},
		{"negative tau", func(c *Chemistry) { c.Tau = []float64{-1} }, "tau"},
		{"non-finite tau", func(c *Chemistry) { c.Tau = []float64{math.NaN()} }, "tau"},
		{"frozen tau", func(c *Chemistry) { c.Tau = []float64{1e300} }, "no-op"},
		{"transport shape", func(c *Chemistry) { c.Transport = &Transport{Fraction: [][]float64{{0, 0.25}}} }, "transport"},
		{"transport row shape", func(c *Chemistry) {
			c.Transport = &Transport{Fraction: [][]float64{{0, 0.25}, {0.5}}}
		}, "transport"},
		{"transport diagonal", func(c *Chemistry) {
			c.Transport = &Transport{Fraction: [][]float64{{0.1, 0.25}, {0.5, 0}}}
		}, "diagonal"},
		{"negative fraction", func(c *Chemistry) {
			c.Transport = &Transport{Fraction: [][]float64{{0, -0.25}, {0.5, 0}}}
		}, "negative"},
		{"non-finite fraction", func(c *Chemistry) {
			c.Transport = &Transport{Fraction: [][]float64{{0, math.Inf(1)}, {0.5, 0}}}
		}, "finite"},
		{"row sum above one", func(c *Chemistry) {
			c.Transport = &Transport{Fraction: [][]float64{{0, 1.5}, {0.5, 0}}}
		}, "row"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := good
			c.Tau = append([]float64(nil), good.Tau...)
			tc.edit(&c)
			k, err := NewChemistry(c)
			if err == nil {
				t.Fatalf("accepted %s: %+v", tc.name, k.Config())
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
		})
	}

	// A row that sums to exactly one is a full transfer, not an error.
	full := Chemistry{Regions: 2, Channels: 1, DT: 1, Tau: []float64{4},
		Transport: &Transport{Fraction: [][]float64{{0, 1}, {0.5, 0}}}}
	if _, err := NewChemistry(full); err != nil {
		t.Fatalf("a row summing to exactly one was refused: %v", err)
	}
}

func TestKineticsDeclaresItsUnitWithoutConverting(t *testing.T) {
	k, err := NewChemistry(oneChannel(1))
	if err != nil {
		t.Fatal(err)
	}
	if got := k.Config().Units; got != UnitNormalized {
		t.Fatalf("an empty unit became %q, want %q", got, UnitNormalized)
	}
	declared := oneChannel(1)
	declared.Units = "micromolar"
	k, err = NewChemistry(declared)
	if err != nil {
		t.Fatal(err)
	}
	if got := k.Config().Units; got != "micromolar" {
		t.Fatalf("declared unit became %q", got)
	}
	// The declared unit changes no number: the same step in both.
	plain, err := NewChemistry(oneChannel(1))
	if err != nil {
		t.Fatal(err)
	}
	s1, err := k.Step(k.NewState(), [][]float64{{2}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := plain.Step(plain.NewState(), [][]float64{{2}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s1.Concentration[0][0] != s2.Concentration[0][0] {
		t.Fatalf("the declared unit changed the result: %v against %v", s1.Concentration[0][0], s2.Concentration[0][0])
	}
}

func TestKineticsStepRejectsInvalidStateReleaseOrBoost(t *testing.T) {
	k, err := NewChemistry(oneChannel(2))
	if err != nil {
		t.Fatal(err)
	}
	valid := [][]float64{{1}, {1}}
	cases := []struct {
		name           string
		state          func() ChemistryState
		release, boost [][]float64
		want           string
	}{
		{"negative release", k.NewState, [][]float64{{-1}, {0}}, nil, "negative"},
		{"non-finite release", k.NewState, [][]float64{{math.NaN()}, {0}}, nil, "finite"},
		{"release rows", k.NewState, [][]float64{{1}}, nil, "release"},
		{"release columns", k.NewState, [][]float64{{1, 1}, {1, 1}}, nil, "release"},
		{"nil release", k.NewState, nil, nil, "release"},
		{"negative boost", k.NewState, valid, [][]float64{{-1}, {0}}, "negative"},
		{"non-finite boost", k.NewState, valid, [][]float64{{math.Inf(1)}, {0}}, "finite"},
		{"boost shape", k.NewState, valid, [][]float64{{1}}, "clearance"},
		{"state rows", func() ChemistryState {
			return ChemistryState{Concentration: [][]float64{{0}}}
		}, valid, nil, "concentration"},
		{"state columns", func() ChemistryState {
			return ChemistryState{Concentration: [][]float64{{0, 0}, {0, 0}}}
		}, valid, nil, "concentration"},
		{"negative concentration", func() ChemistryState {
			return ChemistryState{Concentration: [][]float64{{-1}, {0}}}
		}, valid, nil, "negative"},
		{"non-finite concentration", func() ChemistryState {
			return ChemistryState{Concentration: [][]float64{{math.NaN()}, {0}}}
		}, valid, nil, "finite"},
		{"step overflow", func() ChemistryState {
			s := k.NewState()
			s.Steps = math.MaxUint64
			return s
		}, valid, nil, "overflow"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := k.Step(tc.state(), tc.release, tc.boost)
			if err == nil {
				t.Fatalf("accepted %s: %+v", tc.name, got)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
			if got.Concentration != nil || got.Steps != 0 {
				t.Fatalf("a refused step returned %+v, want the zero state", got)
			}
		})
	}
}

func TestKineticsOwnsItsStateAndDeclaration(t *testing.T) {
	c := oneChannel(2)
	c.Transport = &Transport{Fraction: [][]float64{{0, 0.25}, {0.5, 0}}}
	k, err := NewChemistry(c)
	if err != nil {
		t.Fatal(err)
	}
	c.Tau[0] = 999
	c.Transport.Fraction[0][1] = 0.9
	if got := k.Config().Tau[0]; got != 4 {
		t.Fatalf("writing the caller's tau changed the model to %v", got)
	}
	if got := k.Config().Transport.Fraction[0][1]; got != 0.25 {
		t.Fatalf("writing the caller's transport changed the model to %v", got)
	}
	k.Config().Tau[0] = 1
	if got := k.Config().Tau[0]; got != 4 {
		t.Fatalf("writing the returned config changed the model to %v", got)
	}

	state := k.NewState()
	state.Concentration[0][0] = 8
	release := [][]float64{{1}, {1}}
	next, err := k.Step(state, release, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state.Concentration[0][0] != 8 || state.Steps != 0 {
		t.Fatalf("Step modified the state it was given: %+v", state)
	}
	if release[0][0] != 1 {
		t.Fatalf("Step modified the release it was given: %v", release)
	}
	next.Concentration[0][0] = -1
	if state.Concentration[0][0] != 8 {
		t.Fatalf("the returned state shares memory with its argument")
	}
}
