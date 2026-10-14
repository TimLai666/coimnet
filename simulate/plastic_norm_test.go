package simulate

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/connectome"
)

// The reference squares the exact binary inputs at 256-bit precision. It does
// not share the production float64 reduction or its intermediate range.
func precisePlasticNorm(values []float64) float64 {
	sum := new(big.Float).SetPrec(256)
	term := new(big.Float).SetPrec(256)
	for _, value := range values {
		term.SetFloat64(value)
		term.Mul(term, term)
		sum.Add(sum, term)
	}
	sum.Sqrt(sum)
	result, _ := sum.Float64()
	return result
}

func plasticNormRunner(t *testing.T, g *connectome.Graph, pair [2]string, values []float64, frozen bool, steps int, pulse bool) (*Runner, Protocol, ParameterSet) {
	t.Helper()
	p := nativeReceptorProtocol(pair[0], pair[1], "", 0, frozen)
	p.Plasticity.Rule.PlasticMax = math.MaxFloat64
	p.Stimulus.Inline = plasticStimulus(steps, 0)
	if !pulse {
		p.Stimulus.Inline[0][0] = 0
	}
	params := mustParameters(t, g, *p.Uniform)
	runner, err := Build(context.Background(), g, params, p, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	state := runner.PlasticState()
	copy(state.Plastic, values)
	if err := runner.RestorePlasticState(state); err != nil {
		t.Fatal(err)
	}
	return runner, p, params
}

func checkPlasticNorm(t *testing.T, got, want float64) {
	t.Helper()
	if math.IsInf(got, 0) || math.IsNaN(got) {
		t.Fatalf("norm=%g, want finite %g", got, want)
	}
	if want == 0 {
		if got != 0 {
			t.Fatalf("zero norm=%g", got)
		}
		return
	}
	if got == 0 || math.Abs(got/want-1) > 2e-15 {
		t.Fatalf("norm=%g, independent reference=%g", got, want)
	}
}

func TestNativePlasticNormRepresentableScalars(t *testing.T) {
	g := fixtureGraph(t)
	values := []float64{1e200, 1e-200, 1e-160, math.Ldexp(1, -1060), math.SmallestNonzeroFloat64, 1}
	for _, pair := range nativeReceptorCores {
		for _, frozen := range []bool{false, true} {
			for _, value := range values {
				t.Run(fmt.Sprintf("%s/%s/frozen_%t/value_%g", pair[0], pair[1], frozen, value), func(t *testing.T) {
					runner, p, params := plasticNormRunner(t, g, pair, []float64{value, 0}, frozen, 5, false)
					inputs := nativeReceptorJSON(t, []any{p, params})
					report, err := runner.Run(context.Background(), runner.Stimulus())
					if err != nil {
						t.Fatalf("representable scalar was refused: %v", err)
					}
					if report.Plasticity == nil || report.Plasticity.PlasticL2Before != math.Abs(value) {
						t.Fatalf("before norm=%+v, want exactly abs(%g)", report.Plasticity, value)
					}
					state := runner.PlasticState()
					if state.Plastic[1] != 0 || report.Plasticity.PlasticL2After != math.Abs(state.Plastic[0]) {
						t.Fatalf("after norm=%g, scalar state=%v", report.Plasticity.PlasticL2After, state.Plastic)
					}
					if frozen && state.Plastic[0] != value {
						t.Fatal("frozen scalar changed")
					}
					if !frozen && value >= math.Ldexp(1, -1060) && state.Plastic[0] != value/32 {
						t.Fatalf("five zero-gate decays=%g, want %g", state.Plastic[0], value/32)
					}
					if !bytes.Equal(inputs, nativeReceptorJSON(t, []any{p, params})) {
						t.Fatal("Run changed caller inputs")
					}
				})
			}
		}
	}
}

func TestNativePlasticNormIndependentVectors(t *testing.T) {
	g := fixtureGraph(t)
	diagonal := math.Nextafter(math.MaxFloat64/math.Sqrt2, 0)
	cases := []struct {
		name   string
		values []float64
	}{
		{"zero", []float64{0, 0}},
		{"ordinary", []float64{.1, .2}},
		{"ordinary_tiny_tail", []float64{.1, 1e-200}},
		{"large_triangle", []float64{3e200, 4e200}},
		{"negative_triangle", []float64{-3e200, 4e200}},
		{"tiny_triangle", []float64{3e-200, -4e-200}},
		{"subnormal_triangle", []float64{math.Ldexp(3, -1060), math.Ldexp(4, -1060)}},
		{"upper_single", []float64{math.MaxFloat64, 0}},
		{"upper_diagonal", []float64{diagonal, diagonal}},
		{"upper_small_tail", []float64{math.MaxFloat64, math.Ldexp(math.MaxFloat64, -27)}},
		{"upper_overflow_tail", []float64{math.MaxFloat64, math.Ldexp(math.MaxFloat64, -26)}},
	}
	for _, pair := range nativeReceptorCores {
		for _, tc := range cases {
			t.Run(pair[0]+"/"+pair[1]+"/"+tc.name, func(t *testing.T) {
				runner, p, params := plasticNormRunner(t, g, pair, tc.values, true, 3, false)
				before := nativeReceptorJSON(t, []any{runner.State(), runner.PlasticState(), p, params})
				want := precisePlasticNorm(tc.values)
				report, err := runner.Run(context.Background(), runner.Stimulus())
				if math.IsInf(want, 0) {
					if err == nil || !strings.Contains(err.Error(), "plastic l2") || !reflect.DeepEqual(report, RunReport{}) {
						t.Fatalf("unrepresentable norm returned report/error: %+v / %v", report, err)
					}
					if !bytes.Equal(before, nativeReceptorJSON(t, []any{runner.State(), runner.PlasticState(), p, params})) {
						t.Fatal("norm refusal changed state or caller inputs")
					}
					return
				}
				if err != nil {
					t.Fatalf("finite reference %g was refused: %v", want, err)
				}
				checkPlasticNorm(t, report.Plasticity.PlasticL2Before, want)
				checkPlasticNorm(t, report.Plasticity.PlasticL2After, want)
				if !reflect.DeepEqual(runner.PlasticState().Plastic, tc.values) {
					t.Fatal("frozen vector changed")
				}
			})
		}
	}
}

func TestNativeLargePlasticLearningIsRepresentable(t *testing.T) {
	g := fixtureGraph(t)
	for _, pair := range nativeReceptorCores {
		t.Run(pair[0]+"/"+pair[1], func(t *testing.T) {
			p := nativeReceptorProtocol(pair[0], pair[1], "", 0, false)
			p.Stimulus.Inline = plasticStimulus(3, 1)
			p.Plasticity.GateScale = 1e200
			p.Plasticity.Rule.PlasticMax = 1e200
			runner, report := mustRun(t, g, p)
			state := runner.PlasticState()
			want := precisePlasticNorm(state.Plastic)
			if want == 0 || math.IsInf(want, 0) || want > precisePlasticNorm([]float64{1e200, 1e200}) {
				t.Fatalf("bounded learning produced invalid state %v", state.Plastic)
			}
			checkPlasticNorm(t, report.Plasticity.PlasticL2After, want)
		})
	}
}

func TestNativePlasticNormOverflowDoesNotCommit(t *testing.T) {
	g := fixtureGraph(t)
	for _, pair := range nativeReceptorCores {
		t.Run(pair[0]+"/"+pair[1], func(t *testing.T) {
			values := []float64{1.3e308, 1.3e308}
			if !math.IsInf(precisePlasticNorm(values), 0) {
				t.Fatal("independent overflow fixture is finite")
			}
			control, _, _ := plasticNormRunner(t, g, pair, []float64{1, 0}, true, 3, true)
			controlBefore := nativeReceptorJSON(t, control.State())
			if _, err := control.Run(context.Background(), control.Stimulus()); err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(controlBefore, nativeReceptorJSON(t, control.State())) {
				t.Fatal("pulse did not exercise candidate neural state")
			}
			runner, p, params := plasticNormRunner(t, g, pair, values, true, 3, true)
			before := nativeReceptorJSON(t, []any{runner.State(), runner.PlasticState(), p, params})
			report, err := runner.Run(context.Background(), runner.Stimulus())
			if err == nil || !strings.Contains(err.Error(), "plastic l2") || !reflect.DeepEqual(report, RunReport{}) {
				t.Fatalf("overflow report/error: %+v / %v", report, err)
			}
			if !bytes.Equal(before, nativeReceptorJSON(t, []any{runner.State(), runner.PlasticState(), p, params})) {
				t.Fatal("late norm refusal committed state or changed inputs")
			}
		})
	}
}

func TestPlasticNormReductionIndependentReference(t *testing.T) {
	many := make([]float64, 2049)
	many[0] = 3e-154
	for i := 1; i < len(many); i++ {
		many[i] = 1e-162
	}
	cases := [][]float64{nil, {0, 0}, {1e-160, -1e-160}, many}
	for i, values := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			checkPlasticNorm(t, l2Norm(values), precisePlasticNorm(values))
		})
	}
	// Historical .1/.2 reduction rounded up one ulp. An insignificant tiny
	// tail must not switch ordinary reports to a different reduction.
	if got := l2Norm([]float64{.1, .2, 1e-200}); got != .22360679774997899 {
		t.Fatalf("ordinary legacy norm=%0.17g", got)
	}
}
