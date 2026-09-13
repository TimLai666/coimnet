package dynamics_test

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
)

func close(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-7*(1+math.Abs(want)) {
		t.Fatalf("got %.12g want %.12g", got, want)
	}
}

func TestContinuousSynchronousHandCalculation(t *testing.T) {
	m, err := dynamics.NewContinuous(dynamics.Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, DT: math.Log(2), Activation: "tanh"})
	if err != nil {
		t.Fatal(err)
	}
	p := dynamics.Parameters{Weights: []float64{2}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}}
	tr, err := m.Forward(context.Background(), p, []float64{0, 0}, [][]float64{{2, 0}, {0, 0}})
	if err != nil {
		t.Fatal(err)
	}
	y := tr.Outputs()
	close(t, y[0][0], math.Tanh(1))
	close(t, y[0][1], 0) // zero-delay edges read the previous synchronous state.
	close(t, y[1][0], math.Tanh(.5))
	close(t, y[1][1], math.Tanh(math.Tanh(1)))
	y[0][0] = 99
	close(t, tr.Outputs()[0][0], math.Tanh(1))
}

func TestContinuousFullGradientFiniteDifference(t *testing.T) {
	for _, activation := range []string{"tanh", "softplus"} {
		t.Run(activation, func(t *testing.T) {
			m, err := dynamics.NewContinuous(dynamics.Config{Nodes: 2, Sources: []int{0, 1, 1}, Targets: []int{1, 0, 1}, Delays: []int{0, 1, 0}, DT: .3, Activation: activation})
			if err != nil {
				t.Fatal(err)
			}
			p := dynamics.Parameters{Weights: []float64{.2, -.1, .15}, Bias: []float64{.1, -.2}, LogTau: []float64{.2, -.1}}
			initial := []float64{.2, -.3}
			inputs := [][]float64{{.1, .3}, {-.2, .4}, {.5, -.1}}
			up := [][]float64{{.2, -.3}, {.1, .4}, {-.5, .6}}
			tr, err := m.Forward(context.Background(), p, initial, inputs)
			if err != nil {
				t.Fatal(err)
			}
			g, err := m.Backward(context.Background(), tr, up, 0)
			if err != nil {
				t.Fatal(err)
			}
			objective := func() float64 {
				tr, err := m.Forward(context.Background(), p, initial, inputs)
				if err != nil {
					t.Fatal(err)
				}
				var result float64
				for s, row := range tr.Outputs() {
					for i, v := range row {
						result += v * up[s][i]
					}
				}
				return result
			}
			pairs := []struct{ values, grads []float64 }{{p.Weights, g.Weights}, {p.Bias, g.Bias}, {p.LogTau, g.LogTau}, {initial, g.Initial}}
			for i := range inputs {
				pairs = append(pairs, struct{ values, grads []float64 }{inputs[i], g.Inputs[i]})
			}
			for _, pair := range pairs {
				for i := range pair.values {
					old := pair.values[i]
					const eps = 1e-6
					pair.values[i] = old + eps
					plus := objective()
					pair.values[i] = old - eps
					minus := objective()
					pair.values[i] = old
					close(t, pair.grads[i], (plus-minus)/(2*eps))
				}
			}
		})
	}
}

func TestContinuousTruncationPreservesForwardState(t *testing.T) {
	m, _ := dynamics.NewContinuous(dynamics.Config{Nodes: 1, DT: 1, Activation: "tanh"})
	p := dynamics.Parameters{Bias: []float64{0}, LogTau: []float64{1}}
	tr, err := m.Forward(context.Background(), p, []float64{0}, [][]float64{{1}, {0}, {0}, {0}})
	if err != nil {
		t.Fatal(err)
	}
	before := tr.Outputs()
	up := [][]float64{{0}, {0}, {0}, {1}}
	full, err := m.Backward(context.Background(), tr, up, 0)
	if err != nil {
		t.Fatal(err)
	}
	cut, err := m.Backward(context.Background(), tr, up, 2)
	if err != nil {
		t.Fatal(err)
	}
	if full.Inputs[0][0] == 0 || cut.Inputs[0][0] != 0 {
		t.Fatalf("full=%v cut=%v", full.Inputs, cut.Inputs)
	}
	if !reflect.DeepEqual(before, tr.Outputs()) {
		t.Fatal("backward changed state")
	}
}

func TestContinuousConvergenceAndInvalidInputs(t *testing.T) {
	// Constant drive has an exact exponential solution for every dt.
	for _, dt := range []float64{1, .5, .25} {
		m, err := dynamics.NewContinuous(dynamics.Config{Nodes: 1, DT: dt, Activation: "tanh"})
		if err != nil {
			t.Fatal(err)
		}
		in := make([][]float64, int(2/dt))
		for i := range in {
			in[i] = []float64{1}
		}
		tr, err := m.Forward(context.Background(), dynamics.Parameters{Bias: []float64{0}, LogTau: []float64{0}}, []float64{0}, in)
		if err != nil {
			t.Fatal(err)
		}
		close(t, tr.FinalVoltage()[0], 1-math.Exp(-2))
	}
	m, _ := dynamics.NewContinuous(dynamics.Config{Nodes: 1, DT: 1, Activation: "tanh"})
	p := dynamics.Parameters{Bias: []float64{0}, LogTau: []float64{0}}
	for _, in := range [][][]float64{nil, {{math.NaN()}}, {{math.Inf(1)}}, {{}}, {{0, 1}}} {
		if _, err := m.Forward(context.Background(), p, []float64{0}, in); err == nil {
			t.Fatalf("accepted %v", in)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Forward(ctx, p, []float64{0}, [][]float64{{1}}); err == nil {
		t.Fatal("accepted canceled context")
	}
	if _, err := m.Forward(nil, p, []float64{0}, [][]float64{{1}}); err == nil {
		t.Fatal("accepted nil context")
	}
	for _, logtau := range []float64{math.NaN(), 1000, -1000} {
		bad := dynamics.Parameters{Bias: []float64{0}, LogTau: []float64{logtau}}
		if _, err := m.Forward(context.Background(), bad, []float64{0}, [][]float64{{1}}); err == nil {
			t.Fatal("accepted invalid tau")
		}
	}
}
