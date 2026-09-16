package plasticity_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/plasticity"
)

func TestRuleReceptorFieldsValidated(t *testing.T) {
	zero := 0
	one := 1
	for name, tc := range map[string]struct {
		rule plasticity.Rule
		want bool
	}{
		"GateScale nonzero without GateReceptor": {
			rule: plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 1, WMin: .1, GateScale: 2},
			want: false,
		},
		"GateReceptor negative": {
			rule: plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 1, WMin: .1, GateReceptor: intPtr(-1), GateScale: 1},
			want: false,
		},
		"DecayEMin zero": {
			rule: plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 1, WMin: .1,
				DecayEReceptor: &zero, DecayEBase: .5, DecayESpan: .1, DecayEMin: 0, DecayEMax: .8},
			want: false,
		},
		"DecayEMax one": {
			rule: plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 1, WMin: .1,
				DecayEReceptor: &zero, DecayEBase: .5, DecayESpan: .1, DecayEMin: .1, DecayEMax: 1},
			want: false,
		},
		"DecayEBase out of range": {
			rule: plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 1, WMin: .1,
				DecayEReceptor: &zero, DecayEBase: 2, DecayESpan: .1, DecayEMin: .1, DecayEMax: .8},
			want: false,
		},
		"DecayEReceptor nil but DecayESpan nonzero": {
			rule: plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 1, WMin: .1,
				DecayESpan: .1},
			want: false,
		},
		"valid gate declaration": {
			rule: plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 1, WMin: .1,
				GateReceptor: &one, GateScale: 2},
			want: true,
		},
		"valid decay declaration": {
			rule: plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 1, WMin: .1,
				DecayEReceptor: &zero, DecayEBase: .5, DecayESpan: .4, DecayEMin: .3, DecayEMax: .8},
			want: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := plasticity.New(plasticity.Config{Rule: tc.rule, Edges: []int{0}}, 3)
			if tc.want && err != nil {
				t.Fatalf("rejected valid config: %v", err)
			}
			if !tc.want && err == nil {
				t.Fatal("accepted invalid config")
			}
		})
	}
}

func TestStepWithDecayEOverride(t *testing.T) {
	m := newModel(t, hebbian(), []int{0})
	sources, targets := chain()
	y := []float64{1, 1, 1}

	// Step 1: plain Step, eligibility = 0.5*0 + 1*1 = 1, plastic = 0.5*0 + 1*1 = 1
	s, _, err := m.Step(m.NewState(), y, y, nil, nil, 1, sources, targets)
	if err != nil {
		t.Fatal(err)
	}
	if s.Eligibility[0] != 1 {
		t.Fatalf("step 1 eligibility = %.17g, want 1", s.Eligibility[0])
	}

	// Step 2: StepWith DecayE=0.25, eligibility = 0.25*1 + 1*1 = 1.25
	d25 := 0.25
	s, _, err = m.StepWith(s, plasticity.StepInput{Pre: y, Post: y, DecayE: &d25}, sources, targets)
	if err != nil {
		t.Fatal(err)
	}
	if s.Eligibility[0] != 1.25 {
		t.Fatalf("step 2 eligibility = %.17g, want 1.25", s.Eligibility[0])
	}

	// Step 3: StepWith DecayE=0.75, eligibility = 0.75*1.25 + 1*1 = 1.9375
	d75 := 0.75
	s, _, err = m.StepWith(s, plasticity.StepInput{Pre: y, Post: y, DecayE: &d75}, sources, targets)
	if err != nil {
		t.Fatal(err)
	}
	if s.Eligibility[0] != 1.9375 {
		t.Fatalf("step 3 eligibility = %.17g, want 1.9375", s.Eligibility[0])
	}

	// DecayE=1.0 (out of [0,1)) must error
	d1 := 1.0
	_, _, err = m.StepWith(s, plasticity.StepInput{Pre: y, Post: y, DecayE: &d1}, sources, targets)
	if err == nil {
		t.Fatal("accepted DecayE=1.0")
	}

	// DecayE=NaN must error
	dnan := math.NaN()
	_, _, err = m.StepWith(s, plasticity.StepInput{Pre: y, Post: y, DecayE: &dnan}, sources, targets)
	if err == nil {
		t.Fatal("accepted DecayE=NaN")
	}
}

func TestStepStillMatchesStepWith(t *testing.T) {
	m := newModel(t, hebbian(), []int{0, 1})
	sources, targets := chain()
	y := []float64{1, .5, 0}

	s1, r1, err := m.Step(m.NewState(), y, y, nil, nil, 1, sources, targets)
	if err != nil {
		t.Fatal(err)
	}
	s2, r2, err := m.StepWith(m.NewState(), plasticity.StepInput{Pre: y, Post: y, Gate: 1}, sources, targets)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s1, s2) {
		t.Fatalf("Step and StepWith(nil) differ:\n  Step:     %+v\n  StepWith: %+v", s1, s2)
	}
	if r1 != r2 {
		t.Fatalf("reports differ: %+v vs %+v", r1, r2)
	}
}

func TestWindowAndGateFor(t *testing.T) {
	rule := hebbian()
	zero := 0
	rule.DecayEReceptor = &zero
	rule.DecayEBase = .5
	rule.DecayESpan = .4
	rule.DecayEMin = .3
	rule.DecayEMax = .8
	rule.GateReceptor = &zero
	rule.GateScale = 2
	m := newModel(t, rule, []int{0})

	// WindowFor: occupancy 0 → 0.5, 1 → 0.8 (clamped), -1 → 0.3 (clamped), NaN → 0.5
	for _, tc := range []struct {
		name    string
		occ     float64
		wantVal float64
		wantOk  bool
	}{
		{"zero", 0, .5, true},
		{"one", 1, .8, true},
		{"negative", -1, .3, true},
		{"NaN", math.NaN(), .5, true},
	} {
		t.Run("WindowFor/"+tc.name, func(t *testing.T) {
			val, ok := m.WindowFor(tc.occ)
			if ok != tc.wantOk || val != tc.wantVal {
				t.Fatalf("WindowFor(%g) = (%.17g, %v), want (%.17g, %v)", tc.occ, val, ok, tc.wantVal, tc.wantOk)
			}
		})
	}

	// GateFor: occupancy 0.25 → 2*0.25 = 0.5
	val, ok := m.GateFor(.25)
	if !ok || val != .5 {
		t.Fatalf("GateFor(0.25) = (%.17g, %v), want (0.5, true)", val, ok)
	}

	// No receptor declared → false
	rule2 := hebbian()
	m2 := newModel(t, rule2, []int{0})
	_, ok = m2.WindowFor(0)
	if ok {
		t.Fatal("WindowFor returned true without DecayEReceptor")
	}
	_, ok = m2.GateFor(0)
	if ok {
		t.Fatal("GateFor returned true without GateReceptor")
	}
}

func intPtr(v int) *int { return &v }
