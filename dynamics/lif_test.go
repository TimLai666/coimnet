// This test file is an internal package test because the smooth spike mode is a
// package-private field that exists only as a test reference for finite
// differences. The hard event model is the product behaviour.
package dynamics

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const lifTol = 1e-12

func lifNear(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol*(1+math.Abs(want)) {
		t.Fatalf("%s: got %.17g want %.17g", name, got, want)
	}
}

func lifRows(t *testing.T, name string, got, want [][]float64, tol float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d steps want %d", name, len(got), len(want))
	}
	for s := range want {
		if len(got[s]) != len(want[s]) {
			t.Fatalf("%s[%d]: got %d nodes want %d", name, s, len(got[s]), len(want[s]))
		}
		for i := range want[s] {
			lifNear(t, fmt.Sprintf("%s[%d][%d]", name, s, i), got[s][i], want[s][i], tol)
		}
	}
}

// lifHandConfig fixes dt = ln 2 with tau = tau_syn = tau_adapt = 1 so that
// lambda = alpha = kappa = rho = 0.5 exactly, and theta_raw = 0 so that
// theta_base = 0 + (2-0)*sigmoid(0) = 1 exactly.
func lifHandConfig(adapt bool) LIFConfig {
	c := LIFConfig{
		Nodes:           3,
		Sources:         []int{0, 0},
		Targets:         []int{1, 2},
		Delays:          []int{0, 2},
		DT:              math.Ln2,
		TauSyn:          1,
		ThetaMin:        0,
		ThetaMax:        2,
		VReset:          -1,
		RefractorySteps: 1,
		Surrogate:       LIFSurrogate{Kind: "fast_sigmoid", Scale: 1},
	}
	if adapt {
		c.Adaptation = LIFAdaptation{Enabled: true, TauAdapt: 1, Beta: 0.5}
	}
	return c
}

func lifHandInputs() (LIFParameters, []float64, [][]float64) {
	p := LIFParameters{
		Weights:  []float64{2, 4},
		Bias:     []float64{0, 0, 0},
		LogTau:   []float64{0, 0, 0},
		ThetaRaw: []float64{0, 0, 0},
	}
	return p, []float64{0, 0, 0}, [][]float64{{4, 0, 0}, {4, 0, 0}, {3.2, 0, 0}, {0, 0, 0}, {4, 0, 0}}
}

// TestLIFHandCalculatedTiming checks every state of a three neuron fixture
// against values computed by hand. Node 0 is driven by the input, node 1 reads
// node 0 through a zero delay edge (weight 2) and node 2 through a delay 2 edge
// (weight 4). Arithmetic per step, with lambda = alpha = kappa = rho = 0.5:
//
//	v_cand = 0.5*v + 0.5*drive, theta_eff = 1 + a, x' = 0.5*x + spike,
//	a' = 0.5*a + 0.5*spike, refractory holds v at v_reset = -1 and drops drive.
func TestLIFHandCalculatedTiming(t *testing.T) {
	t.Run("adaptation_off", func(t *testing.T) {
		m, err := NewLIF(lifHandConfig(false))
		if err != nil {
			t.Fatal(err)
		}
		p, initial, inputs := lifHandInputs()
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			t.Fatal(err)
		}
		// node 0: 2>=1 spike, refractory drops input 4, 1.1>=1 spike,
		// refractory, 1.5>=1 spike.
		// node 1: 0, then 2*1=2 -> cand 1.0 >= 1 spike, refractory,
		// 2*1.25=2.5 -> cand 0.75, 2*0.625=1.25 -> cand 1.0 >= 1 spike.
		// node 2: delayed edge reads x0 at t-2, so 4*x0[1]=4 -> cand 2 at step 3.
		lifRows(t, "spikes", tr.Spikes(), [][]float64{
			{1, 0, 0}, {0, 1, 0}, {1, 0, 0}, {0, 0, 1}, {1, 1, 0},
		}, lifTol)
		lifRows(t, "voltages", tr.Voltages(), [][]float64{
			{-1, 0, 0}, {-1, -1, 0}, {-1, -1, 0}, {-1, .75, -1}, {-1, -1, -1},
		}, lifTol)
		lifRows(t, "outputs", tr.Outputs(), [][]float64{
			{1, 0, 0}, {.5, 1, 0}, {1.25, .5, 0}, {.625, .25, 1}, {1.3125, 1.125, .5},
		}, lifTol)
		lifRows(t, "adaptation", tr.adapt[1:], [][]float64{
			{0, 0, 0}, {0, 0, 0}, {0, 0, 0}, {0, 0, 0}, {0, 0, 0},
		}, lifTol)
		wantRefract := [][]int{{1, 0, 0}, {0, 1, 0}, {1, 0, 0}, {0, 0, 1}, {1, 1, 0}}
		if !reflect.DeepEqual(tr.refract[1:], wantRefract) {
			t.Fatalf("refractory counters: got %v want %v", tr.refract[1:], wantRefract)
		}
		if got := tr.FinalVoltage(); !reflect.DeepEqual(got, []float64{-1, -1, -1}) {
			t.Fatalf("final voltage %v", got)
		}
	})
	t.Run("adaptation_on", func(t *testing.T) {
		m, err := NewLIF(lifHandConfig(true))
		if err != nil {
			t.Fatal(err)
		}
		p, initial, inputs := lifHandInputs()
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			t.Fatal(err)
		}
		// The only difference from the previous case: at step 2 node 0 has
		// a = 0.25, so theta_eff = 1.25 > cand = 1.1 and the spike is
		// suppressed; the whole downstream chain changes with it.
		lifRows(t, "spikes", tr.Spikes(), [][]float64{
			{1, 0, 0}, {0, 1, 0}, {0, 0, 0}, {0, 0, 1}, {1, 0, 0},
		}, lifTol)
		lifRows(t, "voltages", tr.Voltages(), [][]float64{
			// node 1 at step 3: drive = 2*0.25 = 0.5, so
			// cand = 0.5*(-1) + 0.5*0.5 = -0.25 stays below theta_eff = 1.25.
			{-1, 0, 0}, {-1, -1, 0}, {1.1, -1, 0}, {.55, -.25, -1}, {-1, 0, -1},
		}, lifTol)
		lifRows(t, "outputs", tr.Outputs(), [][]float64{
			{1, 0, 0}, {.5, 1, 0}, {.25, .5, 0}, {.125, .25, 1}, {1.0625, .125, .5},
		}, lifTol)
		lifRows(t, "adaptation", tr.adapt[1:], [][]float64{
			{.5, 0, 0}, {.25, .5, 0}, {.125, .25, 0}, {.0625, .125, .5}, {.53125, .0625, .25},
		}, lifTol)
		wantRefract := [][]int{{1, 0, 0}, {0, 1, 0}, {0, 0, 0}, {0, 0, 1}, {1, 0, 0}}
		if !reflect.DeepEqual(tr.refract[1:], wantRefract) {
			t.Fatalf("refractory counters: got %v want %v", tr.refract[1:], wantRefract)
		}
	})
}

func TestLIFForwardIsDeterministicAndBinary(t *testing.T) {
	m, err := NewLIF(lifHandConfig(true))
	if err != nil {
		t.Fatal(err)
	}
	p, initial, inputs := lifHandInputs()
	first, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.Outputs(), second.Outputs()) {
		t.Fatal("outputs differ between identical runs")
	}
	if !reflect.DeepEqual(first.Spikes(), second.Spikes()) {
		t.Fatal("spikes differ between identical runs")
	}
	if !reflect.DeepEqual(first.Voltages(), second.Voltages()) {
		t.Fatal("voltages differ between identical runs")
	}
	for s, row := range first.Spikes() {
		for i, v := range row {
			if v != 0 && v != 1 {
				t.Fatalf("spike[%d][%d] = %v is not an event", s, i, v)
			}
		}
	}
}

// lifDirection is a tangent direction over every differentiable quantity.
type lifDirection struct {
	weights, bias, logTau, thetaRaw, initial []float64
	inputs                                   [][]float64
}

func newLIFDirection(c LIFConfig, steps int) lifDirection {
	d := lifDirection{
		weights:  make([]float64, len(c.Sources)),
		bias:     make([]float64, c.Nodes),
		logTau:   make([]float64, c.Nodes),
		thetaRaw: make([]float64, c.Nodes),
		initial:  make([]float64, c.Nodes),
		inputs:   make([][]float64, steps),
	}
	for t := range d.inputs {
		d.inputs[t] = make([]float64, c.Nodes)
	}
	return d
}

// lifForwardTangent is an independent forward mode differentiation of the
// declared rules: it carries tangents forward in time instead of accumulating
// them backwards, so agreement with Backward is evidence about the reverse pass
// rather than a restatement of it. It applies the declared surrogate
// psi(u) = 1/(1+scale*|u|)^2, the detached reset and the refractory rule.
func lifForwardTangent(c LIFConfig, p LIFParameters, initial []float64, inputs, up [][]float64, d lifDirection) (float64, [][]float64) {
	n := c.Nodes
	kappa := math.Exp(-c.DT / c.TauSyn)
	rho, beta := 0.0, 0.0
	if c.Adaptation.Enabled {
		rho = math.Exp(-c.DT / c.Adaptation.TauAdapt)
		beta = c.Adaptation.Beta
	}
	lambda, alpha := make([]float64, n), make([]float64, n)
	dLambda := make([]float64, n)
	base, dBase := make([]float64, n), make([]float64, n)
	for i := 0; i < n; i++ {
		tau := math.Exp(p.LogTau[i])
		lambda[i] = math.Exp(-c.DT / tau)
		alpha[i] = -math.Expm1(-c.DT / tau)
		dLambda[i] = lambda[i] * (c.DT / tau) * d.logTau[i]
		s := 1 / (1 + math.Exp(-p.ThetaRaw[i]))
		base[i] = c.ThetaMin + (c.ThetaMax-c.ThetaMin)*s
		dBase[i] = (c.ThetaMax - c.ThetaMin) * s * (1 - s) * d.thetaRaw[i]
	}
	v := append([]float64(nil), initial...)
	dv := append([]float64(nil), d.initial...)
	a, da := make([]float64, n), make([]float64, n)
	refract := make([]int, n)
	x := [][]float64{make([]float64, n)}
	dx := [][]float64{make([]float64, n)}
	var tangent float64
	for t := range inputs {
		drive, dDrive := make([]float64, n), make([]float64, n)
		for i := 0; i < n; i++ {
			drive[i] = inputs[t][i] + p.Bias[i]
			dDrive[i] = d.inputs[t][i] + d.bias[i]
		}
		for e, s := range c.Sources {
			past := 0
			if c.Delays[e] < t {
				past = t - c.Delays[e]
			}
			drive[c.Targets[e]] += p.Weights[e] * x[past][s]
			dDrive[c.Targets[e]] += d.weights[e]*x[past][s] + p.Weights[e]*dx[past][s]
		}
		nextX, nextDX := make([]float64, n), make([]float64, n)
		for i := 0; i < n; i++ {
			cand := lambda[i]*v[i] + alpha[i]*drive[i]
			dCand := lambda[i]*dv[i] + dLambda[i]*v[i] + alpha[i]*dDrive[i] - dLambda[i]*drive[i]
			eff, dEff := base[i]+a[i], dBase[i]+da[i]
			spike, dSpike, next, dNext := 0.0, 0.0, c.VReset, 0.0
			if refract[i] > 0 {
				refract[i]--
			} else {
				u := cand - eff
				den := 1 + c.Surrogate.Scale*math.Abs(u)
				psi := 1 / (den * den)
				if u >= 0 {
					spike = 1
					refract[i] = c.RefractorySteps
				}
				dSpike = psi * (dCand - dEff)
				next = (1-spike)*cand + spike*c.VReset
				dNext = (1 - spike) * dCand
			}
			v[i], dv[i] = next, dNext
			nextX[i] = kappa*x[t][i] + spike
			nextDX[i] = kappa*dx[t][i] + dSpike
			a[i] = rho*a[i] + beta*spike
			da[i] = rho*da[i] + beta*dSpike
			tangent += up[t][i] * nextDX[i]
		}
		x, dx = append(x, nextX), append(dx, nextDX)
	}
	return tangent, x[1:]
}

// lifGradientFixture drives two neurons so that the sequence contains a spike
// with reset, a refractory step that must drop its input, a delayed edge and
// quiet steps whose voltage path crosses several steps.
func lifGradientFixture() (LIFConfig, LIFParameters, []float64, [][]float64, [][]float64) {
	c := LIFConfig{
		Nodes:           2,
		Sources:         []int{0, 1},
		Targets:         []int{1, 0},
		Delays:          []int{0, 1},
		DT:              .5,
		TauSyn:          .7,
		ThetaMin:        .3,
		ThetaMax:        1.4,
		VReset:          -.5,
		RefractorySteps: 1,
		Adaptation:      LIFAdaptation{Enabled: true, TauAdapt: .9, Beta: .4},
		Surrogate:       LIFSurrogate{Kind: "fast_sigmoid", Scale: 1.7},
	}
	p := LIFParameters{
		Weights:  []float64{1.5, -.8},
		Bias:     []float64{.1, -.05},
		LogTau:   []float64{.2, -.1},
		ThetaRaw: []float64{0, .3},
	}
	initial := []float64{.2, -.1}
	inputs := [][]float64{{2.6, 2.6}, {2.5, .4}, {.2, .5}, {.9, 2}}
	up := [][]float64{{.3, -.2}, {.5, .7}, {-.4, .6}, {.2, .35}}
	return c, p, initial, inputs, up
}

func TestLIFSurrogateGradientMatchesForwardModeReference(t *testing.T) {
	c, p, initial, inputs, up := lifGradientFixture()
	m, err := NewLIF(c)
	if err != nil {
		t.Fatal(err)
	}
	tr, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	var spikes, refractory, quiet int
	for s, row := range tr.Spikes() {
		for i, v := range row {
			switch {
			case v == 1:
				spikes++
			case tr.refract[s][i] > 0:
				refractory++
			default:
				quiet++
			}
		}
	}
	if spikes == 0 || refractory == 0 || quiet == 0 {
		t.Fatalf("fixture misses a branch: spikes=%d refractory=%d quiet=%d", spikes, refractory, quiet)
	}
	g, err := m.Backward(context.Background(), tr, up, 0)
	if err != nil {
		t.Fatal(err)
	}
	steps := len(inputs)
	zero := newLIFDirection(c, steps)
	_, reference := lifForwardTangent(c, p, initial, inputs, up, zero)
	lifRows(t, "reference outputs", tr.Outputs(), reference, lifTol)

	type probe struct {
		name string
		grad float64
		seed func(d *lifDirection)
	}
	var probes []probe
	for e := range p.Weights {
		probes = append(probes, probe{fmt.Sprintf("weights[%d]", e), g.Weights[e], func(d *lifDirection) { d.weights[e] = 1 }})
	}
	for i := range p.Bias {
		probes = append(probes, probe{fmt.Sprintf("bias[%d]", i), g.Bias[i], func(d *lifDirection) { d.bias[i] = 1 }})
		probes = append(probes, probe{fmt.Sprintf("log_tau[%d]", i), g.LogTau[i], func(d *lifDirection) { d.logTau[i] = 1 }})
		probes = append(probes, probe{fmt.Sprintf("theta_raw[%d]", i), g.ThetaRaw[i], func(d *lifDirection) { d.thetaRaw[i] = 1 }})
		probes = append(probes, probe{fmt.Sprintf("initial[%d]", i), g.Initial[i], func(d *lifDirection) { d.initial[i] = 1 }})
		for s := range inputs {
			probes = append(probes, probe{fmt.Sprintf("inputs[%d][%d]", s, i), g.Inputs[s][i], func(d *lifDirection) { d.inputs[s][i] = 1 }})
		}
	}
	for _, pr := range probes {
		d := newLIFDirection(c, steps)
		pr.seed(&d)
		tangent, _ := lifForwardTangent(c, p, initial, inputs, up, d)
		lifNear(t, pr.name, pr.grad, tangent, lifTol)
	}
}

// lifSmoothFixture is a four neuron graph with six edges at delays 0, 1 and 2.
func lifSmoothFixture(adapt bool, scale float64) (LIFConfig, LIFParameters, []float64, [][]float64, [][]float64) {
	c := LIFConfig{
		Nodes:           4,
		Sources:         []int{0, 1, 2, 3, 1, 0},
		Targets:         []int{1, 2, 3, 0, 3, 2},
		Delays:          []int{0, 1, 2, 0, 1, 2},
		DT:              .4,
		TauSyn:          .8,
		ThetaMin:        .2,
		ThetaMax:        1.5,
		VReset:          -.4,
		RefractorySteps: 2,
		Surrogate:       LIFSurrogate{Kind: "fast_sigmoid", Scale: scale},
	}
	if adapt {
		c.Adaptation = LIFAdaptation{Enabled: true, TauAdapt: .9, Beta: .35}
	}
	p := LIFParameters{
		Weights:  []float64{.6, -.5, .4, .35, -.3, .45},
		Bias:     []float64{.2, -.15, .1, .05},
		LogTau:   []float64{.1, -.2, .3, 0},
		ThetaRaw: []float64{.2, -.3, .5, -.1},
	}
	initial := []float64{.1, -.2, .05, .3}
	inputs := [][]float64{
		{.5, -.3, .2, .7}, {-.4, .6, .1, -.2}, {.3, .2, -.5, .4},
		{.8, -.1, .3, .2}, {-.2, .4, .6, -.3}, {.1, .5, -.2, .6},
	}
	up := [][]float64{
		{.2, -.3, .1, .4}, {-.1, .5, .3, -.2}, {.4, .1, -.3, .2},
		{.3, -.4, .2, .1}, {-.2, .2, .5, .3}, {.1, .3, -.1, -.4},
	}
	return c, p, initial, inputs, up
}

// TestLIFSmoothModeFiniteDifference differentiates the smooth reference model,
// which is continuous everywhere, with central differences. Hard spikes are
// never finite differenced.
func TestLIFSmoothModeFiniteDifference(t *testing.T) {
	for _, tc := range []struct {
		name  string
		adapt bool
		scale float64
	}{
		{"adaptation_off_scale2", false, 2},
		{"adaptation_on_scale2", true, 2},
		{"adaptation_on_scale1.3", true, 1.3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, p, initial, inputs, up := lifSmoothFixture(tc.adapt, tc.scale)
			m, err := NewLIF(c)
			if err != nil {
				t.Fatal(err)
			}
			m.smooth = true
			objective := func() float64 {
				tr, err := m.Forward(context.Background(), p, initial, inputs)
				if err != nil {
					t.Fatal(err)
				}
				var total float64
				for s, row := range tr.Outputs() {
					for i, v := range row {
						total += v * up[s][i]
					}
				}
				return total
			}
			tr, err := m.Forward(context.Background(), p, initial, inputs)
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range tr.Spikes() {
				for _, v := range row {
					if v <= 0 || v >= 1 {
						t.Fatalf("smooth spike %v is not strictly inside (0,1)", v)
					}
				}
			}
			g, err := m.Backward(context.Background(), tr, up, 0)
			if err != nil {
				t.Fatal(err)
			}
			pairs := []struct {
				name          string
				values, grads []float64
			}{
				{"weights", p.Weights, g.Weights},
				{"bias", p.Bias, g.Bias},
				{"log_tau", p.LogTau, g.LogTau},
				{"theta_raw", p.ThetaRaw, g.ThetaRaw},
				{"initial", initial, g.Initial},
			}
			for s := range inputs {
				pairs = append(pairs, struct {
					name          string
					values, grads []float64
				}{fmt.Sprintf("inputs[%d]", s), inputs[s], g.Inputs[s]})
			}
			for _, pair := range pairs {
				for i := range pair.values {
					old := pair.values[i]
					const eps = 1e-5
					pair.values[i] = old + eps
					plus := objective()
					pair.values[i] = old - eps
					minus := objective()
					pair.values[i] = old
					fd := (plus - minus) / (2 * eps)
					diff := math.Abs(pair.grads[i] - fd)
					if diff > 1e-4*math.Abs(fd) && diff > 1e-7 {
						t.Fatalf("%s[%d]: gradient %.12g finite difference %.12g", pair.name, i, pair.grads[i], fd)
					}
				}
			}
		})
	}
}

func TestLIFBackwardWindowSemantics(t *testing.T) {
	c, p, initial, inputs, up := lifSmoothFixture(true, 2)
	m, err := NewLIF(c)
	if err != nil {
		t.Fatal(err)
	}
	tr, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	before := tr.Outputs()
	full, err := m.Backward(context.Background(), tr, up, 0)
	if err != nil {
		t.Fatal(err)
	}
	wide, err := m.Backward(context.Background(), tr, up, len(inputs)+1)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(full, wide) {
		t.Fatal("a window longer than the sequence must equal the full gradient")
	}
	cut, err := m.Backward(context.Background(), tr, up, 2)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(full, cut) {
		t.Fatal("window 2 must truncate something in this fixture")
	}
	for _, v := range [][]float64{cut.Weights, cut.Bias, cut.LogTau, cut.ThetaRaw, cut.Initial} {
		for i, x := range v {
			if math.IsNaN(x) || math.IsInf(x, 0) {
				t.Fatalf("truncated gradient %d is not finite: %v", i, x)
			}
		}
	}
	if !reflect.DeepEqual(before, tr.Outputs()) {
		t.Fatal("backward changed the trace")
	}
	// A single late output must not reach the first step once the window cuts
	// the state links at step 2.
	late := make([][]float64, len(inputs))
	for s := range late {
		late[s] = make([]float64, c.Nodes)
	}
	late[len(late)-1][0] = 1
	reach, err := m.Backward(context.Background(), tr, late, 0)
	if err != nil {
		t.Fatal(err)
	}
	stop, err := m.Backward(context.Background(), tr, late, 4)
	if err != nil {
		t.Fatal(err)
	}
	var reachSum, stopSum float64
	for i := range reach.Inputs[0] {
		reachSum += math.Abs(reach.Inputs[0][i])
		stopSum += math.Abs(stop.Inputs[0][i])
	}
	if reachSum == 0 || stopSum != 0 {
		t.Fatalf("full reach %v truncated reach %v", reachSum, stopSum)
	}
}

func TestLIFDisabledAdaptationMatchesUnsetAdaptation(t *testing.T) {
	off := lifHandConfig(false)
	off.Adaptation = LIFAdaptation{Enabled: false, TauAdapt: 1, Beta: 1}
	unset := lifHandConfig(false)
	unset.Adaptation = LIFAdaptation{}
	p, initial, inputs := lifHandInputs()
	up := [][]float64{{.2, -.3, .1}, {-.1, .5, .3}, {.4, .1, -.3}, {.3, -.4, .2}, {-.2, .2, .5}}
	var traces []*LIFTrace
	var grads []LIFGradient
	for _, c := range []LIFConfig{off, unset} {
		m, err := NewLIF(c)
		if err != nil {
			t.Fatal(err)
		}
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			t.Fatal(err)
		}
		g, err := m.Backward(context.Background(), tr, up, 0)
		if err != nil {
			t.Fatal(err)
		}
		traces, grads = append(traces, tr), append(grads, g)
	}
	if !reflect.DeepEqual(traces[0].Outputs(), traces[1].Outputs()) ||
		!reflect.DeepEqual(traces[0].Spikes(), traces[1].Spikes()) ||
		!reflect.DeepEqual(traces[0].Voltages(), traces[1].Voltages()) {
		t.Fatal("disabled adaptation changed the forward pass")
	}
	if !reflect.DeepEqual(grads[0], grads[1]) {
		t.Fatal("disabled adaptation changed the reverse pass")
	}
}

func TestLIFConstructionRejectsInvalidConfig(t *testing.T) {
	valid := lifHandConfig(true)
	if _, err := NewLIF(valid); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(c *LIFConfig){
		"zero nodes":          func(c *LIFConfig) { c.Nodes = 0 },
		"negative nodes":      func(c *LIFConfig) { c.Nodes = -1 },
		"zero dt":             func(c *LIFConfig) { c.DT = 0 },
		"negative dt":         func(c *LIFConfig) { c.DT = -1 },
		"nan dt":              func(c *LIFConfig) { c.DT = math.NaN() },
		"inf dt":              func(c *LIFConfig) { c.DT = math.Inf(1) },
		"zero tau_syn":        func(c *LIFConfig) { c.TauSyn = 0 },
		"negative tau_syn":    func(c *LIFConfig) { c.TauSyn = -1 },
		"nan tau_syn":         func(c *LIFConfig) { c.TauSyn = math.NaN() },
		"inf tau_syn":         func(c *LIFConfig) { c.TauSyn = math.Inf(1) },
		"theta order":         func(c *LIFConfig) { c.ThetaMin = c.ThetaMax },
		"theta inverted":      func(c *LIFConfig) { c.ThetaMin = c.ThetaMax + 1 },
		"reset at theta_min":  func(c *LIFConfig) { c.VReset = c.ThetaMin },
		"reset above":         func(c *LIFConfig) { c.VReset = c.ThetaMin + 1 },
		"nan theta_min":       func(c *LIFConfig) { c.ThetaMin = math.NaN() },
		"nan theta_max":       func(c *LIFConfig) { c.ThetaMax = math.NaN() },
		"inf v_reset":         func(c *LIFConfig) { c.VReset = math.Inf(-1) },
		"negative refractory": func(c *LIFConfig) { c.RefractorySteps = -1 },
		"unknown surrogate":   func(c *LIFConfig) { c.Surrogate.Kind = "triangle" },
		"empty surrogate":     func(c *LIFConfig) { c.Surrogate.Kind = "" },
		"zero scale":          func(c *LIFConfig) { c.Surrogate.Scale = 0 },
		"negative scale":      func(c *LIFConfig) { c.Surrogate.Scale = -1 },
		"nan scale":           func(c *LIFConfig) { c.Surrogate.Scale = math.NaN() },
		"inf scale":           func(c *LIFConfig) { c.Surrogate.Scale = math.Inf(1) },
		"zero tau_adapt":      func(c *LIFConfig) { c.Adaptation.TauAdapt = 0 },
		"negative tau_adapt":  func(c *LIFConfig) { c.Adaptation.TauAdapt = -1 },
		"nan tau_adapt":       func(c *LIFConfig) { c.Adaptation.TauAdapt = math.NaN() },
		"negative beta":       func(c *LIFConfig) { c.Adaptation.Beta = -1 },
		"nan beta":            func(c *LIFConfig) { c.Adaptation.Beta = math.NaN() },
		"inf beta":            func(c *LIFConfig) { c.Adaptation.Beta = math.Inf(1) },
		"edge lengths":        func(c *LIFConfig) { c.Targets = []int{1} },
		"delay lengths":       func(c *LIFConfig) { c.Delays = []int{0} },
		"source out of range": func(c *LIFConfig) { c.Sources = []int{0, 3} },
		"negative source":     func(c *LIFConfig) { c.Sources = []int{0, -1} },
		"target out of range": func(c *LIFConfig) { c.Targets = []int{1, 9} },
		"negative delay":      func(c *LIFConfig) { c.Delays = []int{0, -1} },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			c := lifHandConfig(true)
			c.Sources = append([]int(nil), c.Sources...)
			c.Targets = append([]int(nil), c.Targets...)
			c.Delays = append([]int(nil), c.Delays...)
			mutate(&c)
			m, err := NewLIF(c)
			if err == nil || m != nil {
				t.Fatalf("accepted %s: %v", name, m)
			}
		})
	}
}

func TestLIFForwardRejectsInvalidArguments(t *testing.T) {
	m, err := NewLIF(lifHandConfig(true))
	if err != nil {
		t.Fatal(err)
	}
	p, initial, inputs := lifHandInputs()
	bad := []struct {
		name    string
		params  LIFParameters
		initial []float64
		inputs  [][]float64
	}{
		{"nil sequence", p, initial, nil},
		{"empty sequence", p, initial, [][]float64{}},
		{"short input row", p, initial, [][]float64{{0, 0}}},
		{"long input row", p, initial, [][]float64{{0, 0, 0, 0}}},
		{"nan input", p, initial, [][]float64{{math.NaN(), 0, 0}}},
		{"inf input", p, initial, [][]float64{{0, math.Inf(1), 0}}},
		{"short initial", p, []float64{0, 0}, inputs},
		{"nan initial", p, []float64{math.NaN(), 0, 0}, inputs},
		{"short weights", LIFParameters{[]float64{2}, p.Bias, p.LogTau, p.ThetaRaw}, initial, inputs},
		{"nan weight", LIFParameters{[]float64{2, math.NaN()}, p.Bias, p.LogTau, p.ThetaRaw}, initial, inputs},
		{"short bias", LIFParameters{p.Weights, []float64{0}, p.LogTau, p.ThetaRaw}, initial, inputs},
		{"inf bias", LIFParameters{p.Weights, []float64{0, math.Inf(-1), 0}, p.LogTau, p.ThetaRaw}, initial, inputs},
		{"short log_tau", LIFParameters{p.Weights, p.Bias, []float64{0}, p.ThetaRaw}, initial, inputs},
		{"nan log_tau", LIFParameters{p.Weights, p.Bias, []float64{0, math.NaN(), 0}, p.ThetaRaw}, initial, inputs},
		{"overflow tau", LIFParameters{p.Weights, p.Bias, []float64{1000, 0, 0}, p.ThetaRaw}, initial, inputs},
		{"underflow tau", LIFParameters{p.Weights, p.Bias, []float64{-1000, 0, 0}, p.ThetaRaw}, initial, inputs},
		{"short theta_raw", LIFParameters{p.Weights, p.Bias, p.LogTau, []float64{0, 0}}, initial, inputs},
		{"nan theta_raw", LIFParameters{p.Weights, p.Bias, p.LogTau, []float64{0, 0, math.NaN()}}, initial, inputs},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			tr, err := m.Forward(context.Background(), tc.params, tc.initial, tc.inputs)
			if err == nil || tr != nil {
				t.Fatalf("accepted %s, trace %v", tc.name, tr)
			}
		})
	}
	if tr, err := m.Forward(nil, p, initial, inputs); err == nil || tr != nil {
		t.Fatal("accepted nil context")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tr, err := m.Forward(ctx, p, initial, inputs)
	if !errors.Is(err, context.Canceled) || tr != nil {
		t.Fatalf("cancelled forward returned trace %v error %v", tr, err)
	}
	for _, model := range []*LIF{nil, new(LIF)} {
		if tr, err := model.Forward(context.Background(), p, initial, inputs); err == nil || tr != nil {
			t.Fatalf("uninitialized model returned trace %v", tr)
		}
	}
}

func TestLIFBackwardRejectsInvalidArguments(t *testing.T) {
	c := lifHandConfig(true)
	m, err := NewLIF(c)
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewLIF(c)
	if err != nil {
		t.Fatal(err)
	}
	p, initial, inputs := lifHandInputs()
	tr, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := other.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	up := make([][]float64, len(inputs))
	for s := range up {
		up[s] = []float64{.1, .2, .3}
	}
	if _, err := m.Backward(context.Background(), foreign, up, 0); err == nil {
		t.Fatal("accepted a trace from another model")
	}
	if _, err := m.Backward(context.Background(), nil, up, 0); err == nil {
		t.Fatal("accepted a nil trace")
	}
	if _, err := (*LIF)(nil).Backward(context.Background(), tr, up, 0); err == nil {
		t.Fatal("accepted a nil model")
	}
	if _, err := m.Backward(context.Background(), tr, up, -1); err == nil {
		t.Fatal("accepted a negative window")
	}
	if _, err := m.Backward(context.Background(), tr, up[:2], 0); err == nil {
		t.Fatal("accepted a short upstream sequence")
	}
	short := append([][]float64(nil), up...)
	short[1] = []float64{.1, .2}
	if _, err := m.Backward(context.Background(), tr, short, 0); err == nil {
		t.Fatal("accepted a short upstream row")
	}
	broken := append([][]float64(nil), up...)
	broken[2] = []float64{.1, math.NaN(), .3}
	if _, err := m.Backward(context.Background(), tr, broken, 0); err == nil {
		t.Fatal("accepted a non-finite upstream value")
	}
	if _, err := m.Backward(nil, tr, up, 0); err == nil {
		t.Fatal("accepted a nil context")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Backward(ctx, tr, up, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled backward returned %v", err)
	}
}

func TestLIFZeroEdgesRunAndTrain(t *testing.T) {
	c := LIFConfig{
		Nodes:     2,
		DT:        math.Ln2,
		TauSyn:    1,
		ThetaMin:  0,
		ThetaMax:  2,
		VReset:    -1,
		Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 1},
	}
	m, err := NewLIF(c)
	if err != nil {
		t.Fatal(err)
	}
	p := LIFParameters{Bias: []float64{.5, 0}, LogTau: []float64{0, 0}, ThetaRaw: []float64{0, 0}}
	tr, err := m.Forward(context.Background(), p, []float64{0, 0}, [][]float64{{4, 0}, {0, 0}})
	if err != nil {
		t.Fatal(err)
	}
	// node 0: cand = 0.5*0 + 0.5*(4+0.5) = 2.25 >= 1, so it spikes and resets.
	lifRows(t, "spikes", tr.Spikes(), [][]float64{{1, 0}, {0, 0}}, lifTol)
	lifRows(t, "outputs", tr.Outputs(), [][]float64{{1, 0}, {.5, 0}}, lifTol)
	g, err := m.Backward(context.Background(), tr, [][]float64{{1, 0}, {1, 0}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Weights) != 0 {
		t.Fatalf("zero edge model produced %d weight gradients", len(g.Weights))
	}
	if g.Bias[0] == 0 {
		t.Fatal("bias gradient vanished on a zero edge model")
	}
}

func TestLIFTraceAndConfigAreIndependentCopies(t *testing.T) {
	m, err := NewLIF(lifHandConfig(true))
	if err != nil {
		t.Fatal(err)
	}
	p, initial, inputs := lifHandInputs()
	tr, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	before := tr.Outputs()
	beforeSpikes := tr.Spikes()
	beforeVoltages := tr.Voltages()
	p.Weights[0], p.Bias[0], p.LogTau[0], p.ThetaRaw[0] = 99, 99, 99, 99
	initial[0] = 99
	inputs[0][0] = 99
	if !reflect.DeepEqual(before, tr.Outputs()) ||
		!reflect.DeepEqual(beforeSpikes, tr.Spikes()) ||
		!reflect.DeepEqual(beforeVoltages, tr.Voltages()) {
		t.Fatal("mutating caller state changed the trace")
	}
	tr.Outputs()[0][0] = 99
	tr.Spikes()[0][0] = 99
	tr.Voltages()[0][0] = 99
	tr.FinalVoltage()[0] = 99
	if !reflect.DeepEqual(before, tr.Outputs()) ||
		!reflect.DeepEqual(beforeSpikes, tr.Spikes()) ||
		!reflect.DeepEqual(beforeVoltages, tr.Voltages()) {
		t.Fatal("mutating returned slices changed the trace")
	}
	c := m.Config()
	c.Sources[0], c.Targets[0], c.Delays[0] = 2, 2, 7
	if again := m.Config(); again.Sources[0] != 0 || again.Targets[0] != 1 || again.Delays[0] != 0 {
		t.Fatalf("mutating a returned config changed the model: %+v", again)
	}
	if (*LIF)(nil).Config().Nodes != 0 {
		t.Fatal("nil model must return a zero config")
	}
	if (*LIFTrace)(nil).Outputs() != nil || (*LIFTrace)(nil).Spikes() != nil ||
		(*LIFTrace)(nil).Voltages() != nil || (*LIFTrace)(nil).FinalVoltage() != nil {
		t.Fatal("nil trace must return nil slices")
	}
}

// lifHomeostasisHandConfig fixes dt = ln 2 with tau = tau_syn = 1 so that
// lambda = alpha = kappa = 0.5 and theta_raw = 0 so that theta_base = 1. The
// slow stabiliser uses dt/tau_rate = 0.5 and eta*dt = 1, which reduces its two
// rules to r' = 0.5*r + 0.5*spike' and h' = clamp(h + r' - 0.25, 0, 0.4).
// Adaptation stays off so the table isolates homeostasis.
func lifHomeostasisHandConfig() LIFConfig {
	return LIFConfig{
		Nodes:           2,
		Sources:         []int{0},
		Targets:         []int{1},
		Delays:          []int{0},
		DT:              math.Ln2,
		TauSyn:          1,
		ThetaMin:        0,
		ThetaMax:        2,
		VReset:          -1,
		RefractorySteps: 0,
		Surrogate:       LIFSurrogate{Kind: "fast_sigmoid", Scale: 1},
		Homeostasis:     &LIFHomeostasis{Enabled: true, TauRate: 2 * math.Ln2, TargetRate: .25, Eta: 1 / math.Ln2, HMax: .4},
	}
}

// lifHomeostasisHandInputs drives neuron 0 hard for four steps and then stops,
// so its estimate rises past the target, its offset clamps at h_max and both
// fall back once the neuron goes quiet. Neuron 1 sees neuron 0 through a zero
// delay edge of weight 2.5 and lags behind it.
func lifHomeostasisHandInputs() (LIFParameters, []float64, [][]float64) {
	p := LIFParameters{Weights: []float64{2.5}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}, ThetaRaw: []float64{0, 0}}
	inputs := make([][]float64, 8)
	for t := range inputs {
		inputs[t] = []float64{0, 0}
		if t < 4 {
			inputs[t][0] = 4
		}
	}
	return p, []float64{0, 0}, inputs
}

// lifHomeostasisHandTable is the hand-calculated expectation of that fixture.
// theta_eff of a step uses the offset carried into it, so row t of homeostasis
// is the value the step after it compares against.
func lifHomeostasisHandTable() (spikes, voltages, outputs, rate, homeostasis [][]float64) {
	spikes = [][]float64{{1, 0}, {1, 1}, {1, 1}, {1, 1}, {0, 1}, {0, 0}, {0, 0}, {0, 0}}
	voltages = [][]float64{
		{-1, 0}, {-1, -1}, {-1, -1}, {-1, -1},
		{-.5, -1}, {-.25, .671875}, {-.125, .921875}, {-.0625, .75390625},
	}
	outputs = [][]float64{
		{1, 0}, {1.5, 1}, {1.75, 1.5}, {1.875, 1.75},
		{.9375, 1.875}, {.46875, .9375}, {.234375, .46875}, {.1171875, .234375},
	}
	rate = [][]float64{
		{.5, 0}, {.75, .5}, {.875, .75}, {.9375, .875},
		{.46875, .9375}, {.234375, .46875}, {.1171875, .234375}, {.05859375, .1171875},
	}
	homeostasis = [][]float64{
		{.25, 0}, {.4, .25}, {.4, .4}, {.4, .4},
		{.4, .4}, {.384375, .4}, {.2515625, .384375}, {.06015625, .2515625},
	}
	return
}

// TestLIFHomeostasisHandCalculatedTiming checks the rate estimate, the
// threshold offset and every value they change against a table computed by
// hand. Neuron 0 clamps at h_max from step 1 and falls back from step 5;
// neuron 1 shows the lower clamp at step 0, where its estimate is still below
// the target.
func TestLIFHomeostasisHandCalculatedTiming(t *testing.T) {
	m, err := NewLIF(lifHomeostasisHandConfig())
	if err != nil {
		t.Fatal(err)
	}
	p, initial, inputs := lifHomeostasisHandInputs()
	tr, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	spikes, voltages, outputs, rate, homeostasis := lifHomeostasisHandTable()
	lifRows(t, "spikes", tr.Spikes(), spikes, lifTol)
	lifRows(t, "voltages", tr.Voltages(), voltages, lifTol)
	lifRows(t, "outputs", tr.Outputs(), outputs, lifTol)
	lifRows(t, "rate", tr.rate[1:], rate, lifTol)
	lifRows(t, "homeostasis", tr.homeo[1:], homeostasis, lifTol)
	for _, row := range tr.homeo {
		for i, h := range row {
			if h < 0 || h > m.config.Homeostasis.HMax {
				t.Fatalf("offset %g of neuron %d left [0, %g]", h, i, m.config.Homeostasis.HMax)
			}
		}
	}
}

// TestLIFDisabledHomeostasisMatchesUnsetHomeostasis pins the requirement that
// switching the mechanism off restores the reference model bit for bit, in both
// directions, whether the block is absent or present and disabled.
func TestLIFDisabledHomeostasisMatchesUnsetHomeostasis(t *testing.T) {
	unset := lifHandConfig(true)
	off := lifHandConfig(true)
	off.Homeostasis = &LIFHomeostasis{Enabled: false, TauRate: 3, TargetRate: .5, Eta: 2, HMax: 7}
	p, initial, inputs := lifHandInputs()
	up := [][]float64{{.2, -.3, .1}, {-.1, .5, .3}, {.4, .1, -.3}, {.3, -.4, .2}, {-.2, .2, .5}}
	var traces []*LIFTrace
	var grads []LIFGradient
	for _, c := range []LIFConfig{unset, off} {
		m, err := NewLIF(c)
		if err != nil {
			t.Fatal(err)
		}
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			t.Fatal(err)
		}
		g, err := m.Backward(context.Background(), tr, up, 0)
		if err != nil {
			t.Fatal(err)
		}
		traces, grads = append(traces, tr), append(grads, g)
	}
	if !reflect.DeepEqual(traces[0].Outputs(), traces[1].Outputs()) ||
		!reflect.DeepEqual(traces[0].Spikes(), traces[1].Spikes()) ||
		!reflect.DeepEqual(traces[0].Voltages(), traces[1].Voltages()) {
		t.Fatal("disabled homeostasis changed the forward pass")
	}
	if !reflect.DeepEqual(grads[0], grads[1]) {
		t.Fatal("disabled homeostasis changed the reverse pass")
	}
	for _, tr := range traces {
		for _, row := range tr.homeo {
			for _, h := range row {
				if h != 0 {
					t.Fatalf("disabled homeostasis produced a nonzero offset %g", h)
				}
			}
		}
		for _, row := range tr.rate {
			for _, v := range row {
				if v != 0 {
					t.Fatalf("disabled homeostasis produced a nonzero rate estimate %g", v)
				}
			}
		}
	}
}

// TestLIFConfigJSONIsUnchangedWithoutHomeostasis is the compatibility proof of
// the new field: an omitted pointer must leave the canonical LIFConfig JSON,
// and therefore every recorded core_config_hash and protocol_hash, byte
// identical. The first fixture is the LIF block of the NAT-01 whole-graph
// protocol (evidence/NAT-01/protocol-fullgraph-uniform.json) as a protocol
// carries it, with the topology supplied by the graph store.
func TestLIFConfigJSONIsUnchangedWithoutHomeostasis(t *testing.T) {
	protocolBlock := LIFConfig{
		DT: 1, TauSyn: 5, ThetaMin: .1, ThetaMax: 2, VReset: -.5, RefractorySteps: 1,
		Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
	}
	const wantProtocolBlock = `{"nodes":0,"sources":null,"targets":null,"dt":1,"tau_syn":5,"theta_min":0.1,"theta_max":2,"v_reset":-0.5,"refractory_steps":1,"adaptation":{"enabled":false,"tau_adapt":0,"beta":0},"surrogate":{"kind":"fast_sigmoid","scale":2}}`
	populated := LIFConfig{
		Nodes: 3, Sources: []int{0, 1}, Targets: []int{1, 2}, Delays: []int{0, 1},
		DT: 1, TauSyn: 1, ThetaMin: .05, ThetaMax: 1, VReset: -.5, RefractorySteps: 1,
		Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
	}
	const wantPopulated = `{"nodes":3,"sources":[0,1],"targets":[1,2],"delays":[0,1],"dt":1,"tau_syn":1,"theta_min":0.05,"theta_max":1,"v_reset":-0.5,"refractory_steps":1,"adaptation":{"enabled":false,"tau_adapt":0,"beta":0},"surrogate":{"kind":"fast_sigmoid","scale":2}}`
	// Recorded before the homeostasis field existed, so a changed digest here is
	// a changed core_config_hash everywhere.
	const wantPopulatedHash = "8a64827606764d7f9f36dab81bed2c003fee2e30db54dbc2cac7b699f9088172"
	for _, tc := range []struct{ name, want string }{{"nat01_protocol_block", wantProtocolBlock}, {"populated", wantPopulated}} {
		c := protocolBlock
		if tc.name == "populated" {
			c = populated
		}
		encoded, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded) != tc.want {
			t.Fatalf("%s JSON changed:\n got %s\nwant %s", tc.name, encoded, tc.want)
		}
		if strings.Contains(string(encoded), "homeostasis") {
			t.Fatalf("%s emitted a homeostasis key without a declared block", tc.name)
		}
	}
	encoded, err := json.Marshal(populated)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(encoded)); got != wantPopulatedHash {
		t.Fatalf("configuration digest changed: got %s want %s", got, wantPopulatedHash)
	}
	// A declared block is a different configuration and must change the digest,
	// otherwise the fingerprint would not bind the mechanism at all.
	declared := populated
	declared.Homeostasis = &LIFHomeostasis{Enabled: true, TauRate: 10, TargetRate: .2, Eta: .05, HMax: 1}
	withBlock, err := json.Marshal(declared)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(withBlock), `"homeostasis":{"enabled":true,"tau_rate":10,"target_rate":0.2,"eta":0.05,"h_max":1}`) {
		t.Fatalf("declared homeostasis JSON = %s", withBlock)
	}
	if fmt.Sprintf("%x", sha256.Sum256(withBlock)) == wantPopulatedHash {
		t.Fatal("a declared homeostasis block left the configuration digest unchanged")
	}
}

// TestLIFConfigAndTraceDoNotAliasHomeostasis checks that the new pointer is
// owned like every other part of the configuration.
func TestLIFConfigAndTraceDoNotAliasHomeostasis(t *testing.T) {
	c := lifHomeostasisHandConfig()
	m, err := NewLIF(c)
	if err != nil {
		t.Fatal(err)
	}
	c.Homeostasis.HMax = 99
	if got := m.Config().Homeostasis.HMax; got != .4 {
		t.Fatalf("constructor aliased the caller's block: h_max = %g", got)
	}
	owned := m.Config()
	owned.Homeostasis.TargetRate = 99
	if got := m.Config().Homeostasis.TargetRate; got != .25 {
		t.Fatalf("Config aliased the model's block: target_rate = %g", got)
	}
}

func TestLIFConstructionRejectsInvalidHomeostasis(t *testing.T) {
	valid := lifHomeostasisHandConfig()
	if _, err := NewLIF(valid); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(h *LIFHomeostasis, c *LIFConfig){
		"zero tau_rate":     func(h *LIFHomeostasis, c *LIFConfig) { h.TauRate = 0 },
		"negative tau_rate": func(h *LIFHomeostasis, c *LIFConfig) { h.TauRate = -1 },
		"nan tau_rate":      func(h *LIFHomeostasis, c *LIFConfig) { h.TauRate = math.NaN() },
		"inf tau_rate":      func(h *LIFHomeostasis, c *LIFConfig) { h.TauRate = math.Inf(1) },
		"negative target":   func(h *LIFHomeostasis, c *LIFConfig) { h.TargetRate = -.1 },
		"target above one":  func(h *LIFHomeostasis, c *LIFConfig) { h.TargetRate = 1.1 },
		"nan target":        func(h *LIFHomeostasis, c *LIFConfig) { h.TargetRate = math.NaN() },
		"negative eta":      func(h *LIFHomeostasis, c *LIFConfig) { h.Eta = -1 },
		"nan eta":           func(h *LIFHomeostasis, c *LIFConfig) { h.Eta = math.NaN() },
		"inf eta":           func(h *LIFHomeostasis, c *LIFConfig) { h.Eta = math.Inf(1) },
		"negative h_max":    func(h *LIFHomeostasis, c *LIFConfig) { h.HMax = -.1 },
		"nan h_max":         func(h *LIFHomeostasis, c *LIFConfig) { h.HMax = math.NaN() },
		"inf h_max":         func(h *LIFHomeostasis, c *LIFConfig) { h.HMax = math.Inf(1) },
		"unreachable ceiling": func(h *LIFHomeostasis, c *LIFConfig) {
			h.HMax = math.MaxFloat64
			c.ThetaMin = math.MaxFloat64
			c.ThetaMax = math.Inf(1)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			c := lifHomeostasisHandConfig()
			block := *c.Homeostasis
			c.Homeostasis = &block
			mutate(c.Homeostasis, &c)
			if _, err := NewLIF(c); err == nil {
				t.Fatalf("accepted %s", name)
			}
		})
	}
	// A disabled block carries no promise about its numbers, exactly like the
	// existing disabled adaptation block.
	disabled := lifHomeostasisHandConfig()
	disabled.Homeostasis = &LIFHomeostasis{Enabled: false, TauRate: -5, TargetRate: 9, Eta: -1, HMax: -1}
	if _, err := NewLIF(disabled); err != nil {
		t.Fatalf("rejected a disabled homeostasis block: %v", err)
	}
	// Boundary values of the declared ranges stay legal.
	for name, mutate := range map[string]func(h *LIFHomeostasis){
		"zero target": func(h *LIFHomeostasis) { h.TargetRate = 0 },
		"unit target": func(h *LIFHomeostasis) { h.TargetRate = 1 },
		"zero eta":    func(h *LIFHomeostasis) { h.Eta = 0 },
		"zero h_max":  func(h *LIFHomeostasis) { h.HMax = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			c := lifHomeostasisHandConfig()
			block := *c.Homeostasis
			c.Homeostasis = &block
			mutate(c.Homeostasis)
			if _, err := NewLIF(c); err != nil {
				t.Fatalf("rejected %s: %v", name, err)
			}
		})
	}
}

// lifHomeostasisDrivenFixture is one neuron with no edges and a constant drive
// large enough to make it fire on every step while the mechanism is off. The
// only way its rate can fall is the threshold offset.
func lifHomeostasisDrivenFixture(enabled bool) (LIFConfig, LIFParameters, []float64, [][]float64) {
	c := LIFConfig{
		Nodes: 1, DT: 1, TauSyn: 1, ThetaMin: .5, ThetaMax: 1.5, VReset: -.5,
		Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
	}
	if enabled {
		c.Homeostasis = &LIFHomeostasis{Enabled: true, TauRate: 50, TargetRate: .2, Eta: .05, HMax: 5}
	}
	p := LIFParameters{Weights: []float64{}, Bias: []float64{0}, LogTau: []float64{0}, ThetaRaw: []float64{0}}
	inputs := make([][]float64, 4000)
	for t := range inputs {
		inputs[t] = []float64{2}
	}
	return c, p, []float64{0}, inputs
}

// TestLIFHomeostasisConvergesTowardTargetRate is the COR-04 effect evidence:
// the same driven neuron fires on every one of 4,000 steps with the mechanism
// off, and settles at the declared target rate with it on. Setting
// COIMNET_COR04_EVIDENCE to an absolute directory (go test runs in the package
// directory) also writes the reported curve there.
func TestLIFHomeostasisConvergesTowardTargetRate(t *testing.T) {
	const reportEvery = 100
	type sample struct {
		Step          int     `json:"step"`
		RateEstimate  float64 `json:"rate_estimate"`
		Offset        float64 `json:"homeostasis"`
		SpikeFraction float64 `json:"spike_fraction_so_far"`
	}
	run := func(enabled bool) ([]sample, float64) {
		c, p, initial, inputs := lifHomeostasisDrivenFixture(enabled)
		m, err := NewLIF(c)
		if err != nil {
			t.Fatal(err)
		}
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			t.Fatal(err)
		}
		spikes := tr.Spikes()
		var fired float64
		var curve []sample
		for t := range spikes {
			fired += spikes[t][0]
			if (t+1)%reportEvery == 0 {
				curve = append(curve, sample{t + 1, tr.rate[t+1][0], tr.homeo[t+1][0], fired / float64(t+1)})
			}
		}
		return curve, fired / float64(len(spikes))
	}
	onCurve, onFraction := run(true)
	offCurve, offFraction := run(false)
	if offFraction != 1 {
		t.Fatalf("the undriven reference is not a spike on every step: %g", offFraction)
	}
	for _, s := range offCurve {
		if s.RateEstimate != 0 || s.Offset != 0 {
			t.Fatalf("disabled homeostasis accumulated state at step %d: %+v", s.Step, s)
		}
	}
	final := onCurve[len(onCurve)-1]
	if math.Abs(final.RateEstimate-.2) > .05 {
		t.Fatalf("rate estimate %g is not within 0.05 of the target 0.2", final.RateEstimate)
	}
	if onFraction >= offFraction {
		t.Fatalf("homeostasis did not reduce the spike fraction: %g vs %g", onFraction, offFraction)
	}
	if dir := os.Getenv("COIMNET_COR04_EVIDENCE"); dir != "" {
		document := map[string]any{
			"fixture":                    "one LIF neuron, no edges, constant input 2, dt 1, tau 1, tau_syn 1, theta_min 0.5, theta_max 1.5, theta_raw 0 (theta_base 1), v_reset -0.5, no refractory step, adaptation off",
			"homeostasis":                LIFHomeostasis{Enabled: true, TauRate: 50, TargetRate: .2, Eta: .05, HMax: 5},
			"steps":                      4000,
			"report_every":               reportEvery,
			"enabled_curve":              onCurve,
			"enabled_spike_fraction":     onFraction,
			"disabled_spike_fraction":    offFraction,
			"final_rate_estimate":        final.RateEstimate,
			"final_absolute_deviation":   math.Abs(final.RateEstimate - .2),
			"reproduction_command":       "COIMNET_COR04_EVIDENCE=$PWD/evidence/COR-04 go test -count=1 -v -run TestLIFHomeostasisConvergesTowardTargetRate ./dynamics/ (from the repository root)",
			"disabled_curve_is_all_zero": true,
		}
		data, err := json.MarshalIndent(document, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "homeostasis-convergence.json"), append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
