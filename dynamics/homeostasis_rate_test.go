package dynamics

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"testing"
)

func homeostasisRateConfig(q, eta float64) LIFConfig {
	return LIFConfig{Nodes: 2, DT: q, TauSyn: 1, ThetaMin: 0, ThetaMax: 2, VReset: -1,
		Surrogate:   LIFSurrogate{Kind: "fast_sigmoid", Scale: 1},
		Homeostasis: &LIFHomeostasis{Enabled: true, TauRate: 1, TargetRate: .25, Eta: eta, HMax: .4}}
}

// The adapter uses only public state validation and advancement. JSON snapshots
// protect the complete input state, parameters and input rows on both outcomes.
func homeostasisRateAdvance(t *testing.T, mixed bool, q, eta, rate, input float64) (float64, float64, float64, error) {
	t.Helper()
	c := homeostasisRateConfig(q, eta)
	inputs := [][]float64{{input, 0}}
	ctx := context.Background()
	snapshot := func(v any) string {
		b, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		return string(b)
	}
	if mixed {
		m, e := NewMixed(MixedConfig{Nodes: 2, DT: q, NodeRule: []uint8{RuleLIF, RuleContinuous}, Continuous: ContinuousRule{Activation: "tanh"},
			LIF: LIFRule{TauSyn: c.TauSyn, ThetaMin: c.ThetaMin, ThetaMax: c.ThetaMax, VReset: c.VReset, Surrogate: c.Surrogate, Homeostasis: c.Homeostasis}})
		if e != nil {
			t.Fatal(e)
		}
		p := MixedParameters{Bias: []float64{0, 0}, LogTau: []float64{0, 0}, ThetaRaw: []float64{0}}
		s, e := m.NewState([]float64{0, 0})
		if e != nil {
			t.Fatal(e)
		}
		s.LIF.Rate[0], s.LIF.Homeostasis[0] = rate, .2
		if e = m.ValidateState(s); e != nil {
			t.Fatalf("valid finite state rejected: %v", e)
		}
		before := snapshot([]any{s, p, inputs})
		n, out, spk, e := m.Advance(ctx, p, s, inputs)
		if snapshot([]any{s, p, inputs}) != before {
			t.Fatal("Advance mutated its arguments")
		}
		if e != nil {
			if !reflect.DeepEqual(n, MixedState{}) || out != nil || spk != nil {
				t.Fatal("failed Advance published partial results")
			}
			return 0, 0, 0, e
		}
		if e = m.ValidateState(n); e != nil {
			t.Fatal(e)
		}
		return n.LIF.Rate[0], n.LIF.Homeostasis[0], spk[0][0], nil
	}
	m, e := NewLIF(c)
	if e != nil {
		t.Fatal(e)
	}
	p := LIFParameters{Bias: []float64{0, 0}, LogTau: []float64{0, 0}, ThetaRaw: []float64{0, 0}}
	s, e := m.NewState([]float64{0, 0})
	if e != nil {
		t.Fatal(e)
	}
	s.Rate[0], s.Homeostasis[0] = rate, .2
	if e = m.ValidateState(s); e != nil {
		t.Fatalf("valid finite state rejected: %v", e)
	}
	before := snapshot([]any{s, p, inputs})
	n, out, spk, e := m.Advance(ctx, p, s, inputs)
	if snapshot([]any{s, p, inputs}) != before {
		t.Fatal("Advance mutated its arguments")
	}
	if e != nil {
		if !reflect.DeepEqual(n, LIFState{}) || out != nil || spk != nil {
			t.Fatal("failed Advance published partial results")
		}
		return 0, 0, 0, e
	}
	if e = m.ValidateState(n); e != nil {
		t.Fatal(e)
	}
	return n.Rate[0], n.Homeostasis[0], spk[0][0], nil
}

func TestHomeostasisRateFiniteResume(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		for _, rate := range []float64{1e308, -1e308, math.MaxFloat64, -math.MaxFloat64} {
			for _, spike := range []float64{0, 1} {
				for _, eta := range []float64{0, .5} {
					t.Run(fmt.Sprintf("mixed%v_rate%g_spike%g_eta%g", mixed, rate, spike, eta), func(t *testing.T) {
						// Exact rational arithmetic independently evaluates r + 2*(s-r).
						r := new(big.Rat).SetFloat64(rate)
						delta := new(big.Rat).Sub(new(big.Rat).SetFloat64(spike), r)
						want, _ := new(big.Rat).Add(r, new(big.Rat).Mul(big.NewRat(2, 1), delta)).Float64()
						input := 4 * spike
						got, h, event, e := homeostasisRateAdvance(t, mixed, 2, eta, rate, input)
						if e != nil {
							t.Fatalf("representable rate %g rejected: %v", want, e)
						}
						if !finite(got) || math.Float64bits(got) != math.Float64bits(want) || event != spike {
							t.Fatalf("rate/event %g/%g, want %g/%g", got, event, want, spike)
						}
						wantH := .2
						if eta != 0 {
							wantH = 0
							if want > 0 {
								wantH = .4
							}
						}
						if !finite(h) || h != wantH {
							t.Fatalf("offset %g, want %g", h, wantH)
						}
					})
				}
			}
		}
	}
}

func TestHomeostasisRateTrueOverflowStillRejects(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		for _, rate := range []float64{math.MaxFloat64, -math.MaxFloat64} {
			t.Run(fmt.Sprintf("mixed%v_rate%g", mixed, rate), func(t *testing.T) {
				if _, _, _, e := homeostasisRateAdvance(t, mixed, 3, .5, rate, 0); e == nil {
					t.Fatal("unrepresentable rate accepted")
				}
			})
		}
	}
}

// A fresh neutral session exercises the coupled path and its segment restarts.
type homeostasisRateSequence struct{ *lifCoupledSequence }

func newHomeostasisRateSequence() homeostasisRateSequence {
	return homeostasisRateSequence{&lifCoupledSequence{reverseErrAt: -1}}
}
func (homeostasisRateSequence) Forward(context.Context, int, []float64) (*Modulation, error) {
	return &Modulation{}, nil
}
func (homeostasisRateSequence) Checkpoint(context.Context, int) (ModulationRestart, error) {
	return func(context.Context) (ModulationSequence, error) { return newHomeostasisRateSequence(), nil }, nil
}

func TestHomeostasisRateFreshTrajectory(t *testing.T) {
	// The existing domain permits q=3. One initial event gives
	// r(n)=3*(-2)^(n-1); n=1023 is finite and n=1024 is not.
	ctx := context.Background()
	c := homeostasisRateConfig(3, 0)
	m, e := NewLIF(c)
	if e != nil {
		t.Fatal(e)
	}
	p := LIFParameters{Bias: []float64{0, 0}, LogTau: []float64{0, 0}, ThetaRaw: []float64{0, 0}}
	initial := []float64{0, 0}
	inputs := make([][]float64, 1023)
	upstream := make([][]float64, len(inputs))
	for i := range inputs {
		inputs[i] = []float64{0, 0}
		upstream[i] = []float64{0, 0}
	}
	inputs[0][0] = 4
	wantRate := math.Ldexp(3, 1022)
	t.Run("forward", func(t *testing.T) {
		tr, e := m.Forward(ctx, p, initial, inputs)
		if e != nil {
			t.Fatal(e)
		}
		if tr.rate[len(inputs)][0] != wantRate {
			t.Fatalf("rate %g, want %g", tr.rate[len(inputs)][0], wantRate)
		}
		if _, e = m.Backward(ctx, tr, upstream, 0); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("coupled", func(t *testing.T) {
		tr, e := m.ForwardCoupled(ctx, p, initial, inputs, newHomeostasisRateSequence())
		if e != nil {
			t.Fatal(e)
		}
		if tr.rate[len(inputs)][0] != wantRate {
			t.Fatalf("rate %g, want %g", tr.rate[len(inputs)][0], wantRate)
		}
		if _, e = m.Backward(ctx, tr, upstream, 0); e != nil {
			t.Fatal(e)
		}
	})
	for _, segment := range []int{1, 17, 1022} {
		t.Run(fmt.Sprintf("recompute%d", segment), func(t *testing.T) {
			out, e := m.ForwardRecompute(ctx, p, initial, inputs, segment)
			if e != nil {
				t.Fatal(e)
			}
			if len(out) != len(inputs) {
				t.Fatal("incomplete outputs")
			}
			if _, e = m.BackwardRecompute(ctx, p, initial, inputs, upstream, 0, segment); e != nil {
				t.Fatal(e)
			}
		})
		t.Run(fmt.Sprintf("coupled_recompute%d", segment), func(t *testing.T) {
			tr, e := m.ForwardCoupledRecompute(ctx, p, initial, inputs, newHomeostasisRateSequence(), segment)
			if e != nil {
				t.Fatal(e)
			}
			if len(tr.Outputs()) != len(inputs) {
				t.Fatal("incomplete outputs")
			}
			if _, e = m.BackwardCoupledRecompute(ctx, tr, upstream, 0); e != nil {
				t.Fatal(e)
			}
		})
	}
	t.Run("mixed", func(t *testing.T) {
		mixed, e := NewMixed(MixedConfig{Nodes: 2, DT: 3, NodeRule: []uint8{RuleLIF, RuleContinuous}, Continuous: ContinuousRule{Activation: "tanh"},
			LIF: LIFRule{TauSyn: c.TauSyn, ThetaMin: c.ThetaMin, ThetaMax: c.ThetaMax, VReset: c.VReset, Surrogate: c.Surrogate, Homeostasis: c.Homeostasis}})
		if e != nil {
			t.Fatal(e)
		}
		mp := MixedParameters{Bias: p.Bias, LogTau: p.LogTau, ThetaRaw: []float64{0}}
		for _, coupled := range []bool{false, true} {
			var tr *MixedTrace
			if coupled {
				tr, e = mixed.ForwardCoupled(ctx, mp, initial, inputs, newHomeostasisRateSequence())
			} else {
				tr, e = mixed.Forward(ctx, mp, initial, inputs)
			}
			if e != nil {
				t.Fatal(e)
			}
			if tr.rate[len(inputs)][0] != wantRate {
				t.Fatalf("rate %g, want %g", tr.rate[len(inputs)][0], wantRate)
			}
			if _, e = mixed.Backward(ctx, tr, upstream, 0); e != nil {
				t.Fatal(e)
			}
		}
	})
	t.Run("next_step_overflows", func(t *testing.T) {
		longer := append(append([][]float64(nil), inputs...), []float64{0, 0})
		if tr, e := m.Forward(ctx, p, initial, longer); e == nil || tr != nil {
			t.Fatal("true overflow published a trace")
		}
	})
}
