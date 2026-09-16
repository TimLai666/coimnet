package plasticity_test

import (
	"math"
	"testing"

	"github.com/TimLai666/coimnet/plasticity"
)

// The fixture is one three-neuron chain: edge 0 is 0->1, edge 1 is 1->2 and
// edge 2 is the 0->2 shortcut. Only edges 0 and 1 enable plasticity, so every
// test also shows that edge 2 is left exactly as the core parametrized it.
func chain() (sources, targets []int) {
	return []int{0, 1, 0}, []int{1, 2, 2}
}

// hebbian is the reference rate rule of root decision 1. Every constant is a
// power of two so the hand-computed table below is exact in float64.
func hebbian() plasticity.Rule {
	return plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 8, WMin: .0625}
}

// stdp is the reference pair rule of root decision 2, with unit amplitudes and
// halving traces for the same reason.
func stdp() plasticity.Rule {
	return plasticity.Rule{
		Kind: plasticity.RuleSTDPPair, DecayE: .5, DecayP: .5, PlasticMax: 8, WMin: .0625,
		DecayPre: .5, DecayPost: .5, APlus: 1, AMinus: 1,
	}
}

func newModel(t *testing.T, rule plasticity.Rule, edges []int) *plasticity.Model {
	t.Helper()
	m, err := plasticity.New(plasticity.Config{Rule: rule, Edges: edges}, 3)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func wantState(t *testing.T, step int, got plasticity.State, elig, plastic []float64) {
	t.Helper()
	for k := range elig {
		if got.Eligibility[k] != elig[k] {
			t.Fatalf("step %d eligibility[%d] = %.17g, want %.17g", step, k, got.Eligibility[k], elig[k])
		}
		if got.Plastic[k] != plastic[k] {
			t.Fatalf("step %d plastic[%d] = %.17g, want %.17g", step, k, got.Plastic[k], plastic[k])
		}
	}
}

// TestHebbianRateCorrelatedActivityStrengthens is the hand-computed table for
// root decision 1 with pre and post rising together. Activity y is the node
// signal both ends read, as a continuous core supplies it.
//
//	rule: decay_e = decay_p = 0.5, gate = 1 at every step
//	y(0) = [1, 0.5, 0]  y(1) = [0.5, 1, 0.5]  y(2) = [0, 0.5, 1]
//
//	edge 0 (0->1)                          edge 1 (1->2)
//	t elig                     plastic     elig                     plastic
//	0 0.5*0   + 1*0.5   = 0.5   0.5        0.5*0   + 0.5*0   = 0     0
//	1 0.5*0.5 + 0.5*1   = 0.75  1          0.5*0   + 1*0.5   = 0.5   0.5
//	2 0.5*0.75+ 0*0.5   = 0.375 0.875      0.5*0.5 + 0.5*1   = 0.75  1
func TestHebbianRateCorrelatedActivityStrengthens(t *testing.T) {
	m := newModel(t, hebbian(), []int{0, 1})
	sources, targets := chain()
	activity := [][]float64{{1, .5, 0}, {.5, 1, .5}, {0, .5, 1}}
	wantElig := [][]float64{{.5, 0}, {.75, .5}, {.375, .75}}
	wantPlastic := [][]float64{{.5, 0}, {1, .5}, {.875, 1}}
	s := m.NewState()
	for t0, y := range activity {
		next, report, err := m.Step(s, y, y, nil, nil, 1, sources, targets)
		if err != nil {
			t.Fatal(err)
		}
		if report.Clamped != 0 {
			t.Fatalf("step %d clamped %d entries under plastic_max 8", t0, report.Clamped)
		}
		if &next.Plastic[0] == &s.Plastic[0] || &next.Eligibility[0] == &s.Eligibility[0] {
			t.Fatal("Step returned a state aliased with its input")
		}
		wantState(t, t0, next, wantElig[t0], wantPlastic[t0])
		s = next
	}
	if s.PreTrace != nil || s.PostTrace != nil {
		t.Fatal("hebbian_rate must not carry the pair traces")
	}
	// The effective weights show the same table on the wire the core reads.
	// Edge 2 is not enabled and keeps its base value.
	base := []float64{.5, -.25, .75}
	w, clamp, err := m.Effective(base, nil, s)
	if err != nil {
		t.Fatal(err)
	}
	if clamp.HeldAtWMin != 0 {
		t.Fatalf("free edges held at w_min %d times", clamp.HeldAtWMin)
	}
	for k, want := range []float64{1.375, .75, .75} {
		if w[k] != want {
			t.Fatalf("effective[%d] = %.17g, want %.17g", k, w[k], want)
		}
	}
	if base[0] != .5 {
		t.Fatal("Effective mutated the base weights")
	}
}

// TestHebbianRateAnticorrelatedActivityWeakens is the mirror table: the post
// signal has the opposite sign at every step, so the same magnitudes appear
// with a negative plastic change.
//
//	y(0) = [1, -0.5, 0]  y(1) = [0.5, -1, 0.5]  y(2) = [0, -0.5, 1]
//	edge 0 (0->1): elig -0.5, -0.75, -0.375; plastic -0.5, -1, -0.875
func TestHebbianRateAnticorrelatedActivityWeakens(t *testing.T) {
	m := newModel(t, hebbian(), []int{0})
	sources, targets := chain()
	activity := [][]float64{{1, -.5, 0}, {.5, -1, .5}, {0, -.5, 1}}
	wantElig := []float64{-.5, -.75, -.375}
	wantPlastic := []float64{-.5, -1, -.875}
	s := m.NewState()
	for t0, y := range activity {
		next, _, err := m.Step(s, y, y, nil, nil, 1, sources, targets)
		if err != nil {
			t.Fatal(err)
		}
		wantState(t, t0, next, wantElig[t0:t0+1], wantPlastic[t0:t0+1])
		s = next
	}
}

// TestSTDPPairPreBeforePostStrengthens is the hand-computed table for root
// decision 2 on the feed-forward chain 0 -> 1 -> 2, one spike per step in
// anatomical order.
//
//	rule: decay_e = decay_p = decay_pre = decay_post = 0.5, a_plus = a_minus = 1
//	spikes: t0 = [1,0,0]  t1 = [0,1,0]  t2 = [0,0,1], gate = 1 at every step
//
//	Within one step: both traces decay, the eligibility reads the decayed
//	traces, and only then does each trace add its own event.
//
//	edge 0 (0->1)                                       plastic
//	t0 pre_trace 0.5*0=0, post_trace 0; pre spike:
//	   elig = 0.5*0 - 1*0 = 0; pre_trace -> 1            0.5*0 + 1*0     = 0
//	t1 pre_trace 0.5*1=0.5, post_trace 0; post spike:
//	   elig = 0.5*0 + 1*0.5 = 0.5; post_trace -> 1       0.5*0 + 1*0.5   = 0.5
//	t2 no spike: elig = 0.5*0.5 = 0.25                   0.5*0.5 + 0.25  = 0.5
//
//	edge 1 (1->2): the same table one step later, ending at plastic 0.5.
func TestSTDPPairPreBeforePostStrengthens(t *testing.T) {
	m := newModel(t, stdp(), []int{0, 1})
	sources, targets := chain()
	spikes := [][]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
	wantElig := [][]float64{{0, 0}, {.5, 0}, {.25, .5}}
	wantPlastic := [][]float64{{0, 0}, {.5, 0}, {.5, .5}}
	wantPre := [][]float64{{1, 0}, {.5, 1}, {.25, .5}}
	wantPost := [][]float64{{0, 0}, {1, 0}, {.5, 1}}
	s := m.NewState()
	if len(s.PreTrace) != 2 || len(s.PostTrace) != 2 {
		t.Fatalf("stdp_pair state must carry both traces: %+v", s)
	}
	for t0, spike := range spikes {
		next, _, err := m.Step(s, nil, nil, spike, spike, 1, sources, targets)
		if err != nil {
			t.Fatal(err)
		}
		wantState(t, t0, next, wantElig[t0], wantPlastic[t0])
		for k := range wantPre[t0] {
			if next.PreTrace[k] != wantPre[t0][k] || next.PostTrace[k] != wantPost[t0][k] {
				t.Fatalf("step %d traces[%d] = (%.17g, %.17g), want (%.17g, %.17g)",
					t0, k, next.PreTrace[k], next.PostTrace[k], wantPre[t0][k], wantPost[t0][k])
			}
		}
		s = next
	}
	if s.Plastic[0] <= 0 || s.Plastic[1] <= 0 {
		t.Fatalf("pre-before-post must strengthen both links: %v", s.Plastic)
	}
}

// TestSTDPPairPostBeforePreWeakens runs the same chain backwards, so every
// pair arrives in the depressing order and both links end at plastic -0.5.
//
//	spikes: t0 = [0,0,1]  t1 = [0,1,0]  t2 = [1,0,0]
//	edge 0 (0->1): t0 nothing; t1 post spike reads pre_trace 0 -> elig 0,
//	post_trace -> 1; t2 pre spike reads post_trace 0.5 -> elig -0.5,
//	plastic 0.5*0 + 1*(-0.5) = -0.5.
func TestSTDPPairPostBeforePreWeakens(t *testing.T) {
	m := newModel(t, stdp(), []int{0, 1})
	sources, targets := chain()
	spikes := [][]float64{{0, 0, 1}, {0, 1, 0}, {1, 0, 0}}
	wantElig := [][]float64{{0, 0}, {0, -.5}, {-.5, -.25}}
	wantPlastic := [][]float64{{0, 0}, {0, -.5}, {-.5, -.5}}
	s := m.NewState()
	for t0, spike := range spikes {
		next, _, err := m.Step(s, nil, nil, spike, spike, 1, sources, targets)
		if err != nil {
			t.Fatal(err)
		}
		wantState(t, t0, next, wantElig[t0], wantPlastic[t0])
		s = next
	}
}

// TestSTDPPairCoincidentSpikePinsTheReadOrder fixes the two orders the package
// documents. A pre and post event in the same step read both traces before
// either adds its own event, so the first coincidence changes nothing, and the
// trace decays before it adds, so a second pre event gives 0.5*1 + 1 = 1.5
// rather than 0.5*(1+1) = 1.
func TestSTDPPairCoincidentSpikePinsTheReadOrder(t *testing.T) {
	m := newModel(t, stdp(), []int{0})
	sources, targets := chain()
	both := []float64{1, 1, 0}
	s := m.NewState()
	first, _, err := m.Step(s, nil, nil, both, both, 1, sources, targets)
	if err != nil {
		t.Fatal(err)
	}
	if first.Eligibility[0] != 0 || first.Plastic[0] != 0 {
		t.Fatalf("a coincident pair must read zero traces: %+v", first)
	}
	if first.PreTrace[0] != 1 || first.PostTrace[0] != 1 {
		t.Fatalf("both traces must add their own event: %+v", first)
	}
	second, _, err := m.Step(first, nil, nil, both, both, 1, sources, targets)
	if err != nil {
		t.Fatal(err)
	}
	// elig = 0.5*0 + 1*0.5 (post reads the decayed pre trace) - 1*0.5 = 0.
	if second.Eligibility[0] != 0 {
		t.Fatalf("symmetric amplitudes must cancel: %.17g", second.Eligibility[0])
	}
	if second.PreTrace[0] != 1.5 || second.PostTrace[0] != 1.5 {
		t.Fatalf("traces must decay before adding: %+v", second)
	}
}

// TestNewRejectsMalformedConfigurations covers every validated clause of the
// contract, so a rule that could never be evaluated is refused at construction
// rather than at the first step.
func TestNewRejectsMalformedConfigurations(t *testing.T) {
	for name, c := range map[string]plasticity.Config{
		"unknown kind":          {Rule: plasticity.Rule{Kind: "oja", DecayE: .5, DecayP: .5, PlasticMax: 1, WMin: .1}, Edges: []int{0}},
		"empty kind":            {Rule: plasticity.Rule{DecayE: .5, DecayP: .5, PlasticMax: 1, WMin: .1}, Edges: []int{0}},
		"decay_e at one":        {Rule: plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: 1, DecayP: .5, PlasticMax: 1, WMin: .1}, Edges: []int{0}},
		"negative decay_p":      {Rule: plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: -.1, PlasticMax: 1, WMin: .1}, Edges: []int{0}},
		"zero plastic_max":      {Rule: plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, WMin: .1}, Edges: []int{0}},
		"zero w_min":            {Rule: plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 1}, Edges: []int{0}},
		"non-finite plastic":    {Rule: plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: math.Inf(1), WMin: .1}, Edges: []int{0}},
		"rate rule with a_plus": {Rule: plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 1, WMin: .1, APlus: 1}, Edges: []int{0}},
		"pair rule decay_pre":   {Rule: plasticity.Rule{Kind: plasticity.RuleSTDPPair, DecayE: .5, DecayP: .5, PlasticMax: 1, WMin: .1, DecayPre: 1, APlus: 1, AMinus: 1}, Edges: []int{0}},
		"pair rule a_minus":     {Rule: plasticity.Rule{Kind: plasticity.RuleSTDPPair, DecayE: .5, DecayP: .5, PlasticMax: 1, WMin: .1, DecayPre: .5, DecayPost: .5, APlus: 1, AMinus: -1}, Edges: []int{0}},
		"no edges":              {Rule: hebbian()},
		"empty edge list":       {Rule: hebbian(), Edges: []int{}},
		"negative edge":         {Rule: hebbian(), Edges: []int{-1}},
		"edge out of range":     {Rule: hebbian(), Edges: []int{3}},
		"repeated edge":         {Rule: hebbian(), Edges: []int{1, 1}},
		"descending edges":      {Rule: hebbian(), Edges: []int{2, 0}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := plasticity.New(c, 3); err == nil {
				t.Fatal("accepted a configuration that cannot be evaluated")
			}
		})
	}
	if _, err := plasticity.New(plasticity.Config{Rule: hebbian(), Edges: []int{0, 2}}, 3); err != nil {
		t.Fatalf("rejected a valid configuration: %v", err)
	}
}

// TestStepRejectsMalformedArguments keeps a caller from silently producing a
// meaningless update: the pair rule needs events, every rule needs node signals
// that cover the declared topology, and the gate must be a number.
func TestStepRejectsMalformedArguments(t *testing.T) {
	sources, targets := chain()
	rate := newModel(t, hebbian(), []int{0})
	pair := newModel(t, stdp(), []int{0})
	y := []float64{1, 1, 1}
	spike := []float64{1, 0, 0}
	for name, call := range map[string]func() error{
		"pair rule without events": func() error {
			_, _, err := pair.Step(pair.NewState(), y, y, nil, nil, 1, sources, targets)
			return err
		},
		"pair rule half events": func() error {
			_, _, err := pair.Step(pair.NewState(), y, y, spike, nil, 1, sources, targets)
			return err
		},
		"event that is not zero or one": func() error {
			_, _, err := pair.Step(pair.NewState(), y, y, []float64{.5, 0, 0}, spike, 1, sources, targets)
			return err
		},
		"short activity": func() error {
			_, _, err := rate.Step(rate.NewState(), []float64{1, 1}, y, nil, nil, 1, sources, targets)
			return err
		},
		"mismatched activity": func() error {
			_, _, err := rate.Step(rate.NewState(), y, []float64{1, 1}, nil, nil, 1, sources, targets)
			return err
		},
		"non-finite activity": func() error {
			_, _, err := rate.Step(rate.NewState(), []float64{math.NaN(), 1, 1}, y, nil, nil, 1, sources, targets)
			return err
		},
		"non-finite gate": func() error {
			_, _, err := rate.Step(rate.NewState(), y, y, nil, nil, math.Inf(1), sources, targets)
			return err
		},
		"topology length": func() error {
			_, _, err := rate.Step(rate.NewState(), y, y, nil, nil, 1, []int{0, 1}, targets)
			return err
		},
		"node index out of range": func() error {
			_, _, err := rate.Step(rate.NewState(), y, y, nil, nil, 1, []int{9, 1, 0}, targets)
			return err
		},
		"state from another rule": func() error {
			_, _, err := rate.Step(pair.NewState(), y, y, nil, nil, 1, sources, targets)
			return err
		},
		"short state": func() error {
			_, _, err := rate.Step(plasticity.State{}, y, y, nil, nil, 1, sources, targets)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil {
				t.Fatal("accepted a malformed step")
			}
		})
	}
}
