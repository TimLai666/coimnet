package plasticity_test

import (
	"math"
	"testing"

	"github.com/TimLai666/coimnet/plasticity"
)

// TestGateZeroLetsPlasticOnlyDecay is the closed-gate clause of root decision
// 4: the eligibility keeps tracking activity while the gate is zero, and the
// fast change only decays.
//
//	rule: decay_e = decay_p = 0.5, y = [1, 1, 1] at every step, gate = 0
//	start from plastic = 2 on edge 0 (written into the state directly)
//	t elig                       plastic
//	0 0.5*0 + 1*1 = 1            0.5*2 + 0*1 = 1
//	1 0.5*1 + 1*1 = 1.5          0.5*1       = 0.5
//	2 0.5*1.5 + 1*1 = 1.75       0.5*0.5     = 0.25
func TestGateZeroLetsPlasticOnlyDecay(t *testing.T) {
	m := newModel(t, hebbian(), []int{0})
	sources, targets := chain()
	y := []float64{1, 1, 1}
	s := m.NewState()
	s.Plastic[0] = 2
	wantElig := []float64{1, 1.5, 1.75}
	wantPlastic := []float64{1, .5, .25}
	for step := range 3 {
		next, _, err := m.Step(s, y, y, nil, nil, 0, sources, targets)
		if err != nil {
			t.Fatal(err)
		}
		if next.Eligibility[0] != wantElig[step] || next.Plastic[0] != wantPlastic[step] {
			t.Fatalf("step %d = (%.17g, %.17g), want (%.17g, %.17g)",
				step, next.Eligibility[0], next.Plastic[0], wantElig[step], wantPlastic[step])
		}
		s = next
	}
}

// TestDelayedGateUsesResidualEligibility is the delayed-feedback clause: one
// activity step, then two silent steps, then the gate arrives. The fast change
// is exactly the residual eligibility, so a different decay_e gives a
// different, distinguishable result.
//
//	activity only at t0 (y = [1,1,1]), gate 0, 0, 1
//	decay_e 0.5  : elig 1, 0.5, 0.25    -> plastic 0, 0, 0.5*0 + 1*0.25 = 0.25
//	decay_e 0.25 : elig 1, 0.25, 0.0625 -> plastic 0, 0, 0.0625
func TestDelayedGateUsesResidualEligibility(t *testing.T) {
	sources, targets := chain()
	active, silent := []float64{1, 1, 1}, []float64{0, 0, 0}
	for _, tc := range []struct {
		name        string
		decayE      float64
		wantElig    []float64
		wantPlastic float64
	}{
		{"decay_e 0.5", .5, []float64{1, .5, .25}, .25},
		{"decay_e 0.25", .25, []float64{1, .25, .0625}, .0625},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule := hebbian()
			rule.DecayE = tc.decayE
			m := newModel(t, rule, []int{0})
			s := m.NewState()
			for step, gate := range []float64{0, 0, 1} {
				y := silent
				if step == 0 {
					y = active
				}
				next, _, err := m.Step(s, y, y, nil, nil, gate, sources, targets)
				if err != nil {
					t.Fatal(err)
				}
				if next.Eligibility[0] != tc.wantElig[step] {
					t.Fatalf("step %d eligibility = %.17g, want %.17g", step, next.Eligibility[0], tc.wantElig[step])
				}
				if step < 2 && next.Plastic[0] != 0 {
					t.Fatalf("step %d changed plastic while the gate was closed: %.17g", step, next.Plastic[0])
				}
				s = next
			}
			if s.Plastic[0] != tc.wantPlastic {
				t.Fatalf("delayed gate produced plastic %.17g, want %.17g", s.Plastic[0], tc.wantPlastic)
			}
		})
	}
}

// TestEligibilityAndPlasticDecayToZero is the decay clause: with no activity
// and no gate both quantities fall monotonically and are negligible after
// sixty steps.
func TestEligibilityAndPlasticDecayToZero(t *testing.T) {
	m := newModel(t, hebbian(), []int{0})
	sources, targets := chain()
	silent := []float64{0, 0, 0}
	s := m.NewState()
	s.Eligibility[0], s.Plastic[0] = 4, 4
	previous := s
	for step := range 60 {
		next, _, err := m.Step(s, silent, silent, nil, nil, 0, sources, targets)
		if err != nil {
			t.Fatal(err)
		}
		if next.Eligibility[0] >= previous.Eligibility[0] || next.Plastic[0] >= previous.Plastic[0] {
			t.Fatalf("step %d did not decay: %+v after %+v", step, next, previous)
		}
		previous, s = next, next
	}
	if s.Eligibility[0] > 1e-12 || s.Plastic[0] > 1e-12 {
		t.Fatalf("after 60 halvings the state is still %+v", s)
	}
}

// TestPlasticMaxClampsAndCounts is the bound clause of root decision 1. With
// decay_p 0.5, decay_e 0.5, unit activity and an open gate the unbounded
// series would be 1, 2, 2.75; plastic_max 1.5 holds it and each held step is
// reported once.
//
//	t elig                  unbounded plastic     reported
//	0 0.5*0   + 1 = 1       0.5*0   + 1   = 1     1, clamped 0
//	1 0.5*1   + 1 = 1.5     0.5*1   + 1.5 = 2     1.5, clamped 1
//	2 0.5*1.5 + 1 = 1.75    0.5*1.5 + 1.75= 2.5   1.5, clamped 1
func TestPlasticMaxClampsAndCounts(t *testing.T) {
	rule := hebbian()
	rule.PlasticMax = 1.5
	sources, targets := chain()
	for _, tc := range []struct {
		name        string
		post        float64
		wantPlastic []float64
		wantClamped []int
	}{
		{"upper bound", 1, []float64{1, 1.5, 1.5}, []int{0, 1, 1}},
		{"lower bound", -1, []float64{-1, -1.5, -1.5}, []int{0, 1, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newModel(t, rule, []int{0})
			pre := []float64{1, 1, 1}
			post := []float64{tc.post, tc.post, tc.post}
			s := m.NewState()
			for step := range 3 {
				next, report, err := m.Step(s, pre, post, nil, nil, 1, sources, targets)
				if err != nil {
					t.Fatal(err)
				}
				if next.Plastic[0] != tc.wantPlastic[step] || report.Clamped != tc.wantClamped[step] {
					t.Fatalf("step %d plastic %.17g clamped %d, want %.17g and %d",
						step, next.Plastic[0], report.Clamped, tc.wantPlastic[step], tc.wantClamped[step])
				}
				s = next
			}
		})
	}
}

// TestEffectiveHoldsFixedSignEdgesAtWMin is root decision 3: a fixed-sign edge
// is s * max(|w_base| + plastic, w_min), so a large negative fast change can
// only shrink it to w_min and never crosses zero. A free edge has no floor.
//
//	base = [0.5, -0.25, 0.75], signs = [+1, -1, 0], w_min = 0.0625
//	plastic = [-2, -2] on edges 0 and 1; edge 2 has no plasticity
//	edge 0: |0.5|  - 2 = -1.5  < w_min -> +0.0625, held
//	edge 1: |0.25| - 2 = -1.75 < w_min -> -0.0625, held
//	edge 2: untouched 0.75
func TestEffectiveHoldsFixedSignEdgesAtWMin(t *testing.T) {
	m := newModel(t, hebbian(), []int{0, 1})
	base := []float64{.5, -.25, .75}
	signs := []int8{1, -1, 0}
	s := m.NewState()
	s.Plastic[0], s.Plastic[1] = -2, -2
	w, clamp, err := m.Effective(base, signs, s)
	if err != nil {
		t.Fatal(err)
	}
	if clamp.HeldAtWMin != 2 {
		t.Fatalf("held at w_min %d times, want 2", clamp.HeldAtWMin)
	}
	for k, want := range []float64{.0625, -.0625, .75} {
		if w[k] != want {
			t.Fatalf("effective[%d] = %.17g, want %.17g", k, w[k], want)
		}
	}
	for k, sign := range signs {
		if sign != 0 && w[k]*float64(sign) <= 0 {
			t.Fatalf("edge %d crossed zero: %.17g under sign %d", k, w[k], sign)
		}
	}
	// A free edge under the same fast change is allowed to cross zero, which is
	// exactly the difference the fixed-sign rule exists to prevent.
	free, clampFree, err := m.Effective(base, []int8{0, 0, 0}, s)
	if err != nil {
		t.Fatal(err)
	}
	if clampFree.HeldAtWMin != 0 {
		t.Fatalf("free edges were held at w_min %d times", clampFree.HeldAtWMin)
	}
	if free[0] != -1.5 || free[1] != -2.25 || free[2] != .75 {
		t.Fatalf("free effective weights = %v", free)
	}
}

// TestEffectiveRejectsMalformedArguments keeps the weight conversion from
// silently reading a topology it was not built for.
func TestEffectiveRejectsMalformedArguments(t *testing.T) {
	m := newModel(t, hebbian(), []int{0})
	s := m.NewState()
	for name, call := range map[string]func() error{
		"short base": func() error {
			_, _, err := m.Effective([]float64{1, 2}, nil, s)
			return err
		},
		"wrong sign length": func() error {
			_, _, err := m.Effective([]float64{1, 2, 3}, []int8{1}, s)
			return err
		},
		"invalid sign": func() error {
			_, _, err := m.Effective([]float64{1, 2, 3}, []int8{2, 0, 0}, s)
			return err
		},
		"non-finite base": func() error {
			_, _, err := m.Effective([]float64{math.Inf(1), 2, 3}, nil, s)
			return err
		},
		"foreign state": func() error {
			_, _, err := m.Effective([]float64{1, 2, 3}, nil, plasticity.State{})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil {
				t.Fatal("accepted a malformed conversion")
			}
		})
	}
}
