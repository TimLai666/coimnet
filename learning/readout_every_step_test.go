package learning_test

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

// everyStepConfig is a three-neuron continuous core with two readout neurons
// and two outputs, so a per-step readout produces a T x 2 prediction that a
// last-step readout cannot be confused with. The only difference between the
// two modes under test is ReadoutEveryStep.
func everyStepConfig(every bool) learning.Config {
	return learning.Config{
		Dynamics:         dynamics.Config{Nodes: 3, Sources: []int{0, 1, 2}, Targets: []int{1, 2, 0}, DT: .5, Activation: "tanh"},
		InputSize:        1,
		OutputSize:       2,
		ReadoutNodes:     []int{1, 2},
		ReadoutEveryStep: every,
	}
}

// everyStepParameters holds the encoder [1,3], the three core edges and the
// readout [2,2] of that fixture.
func everyStepParameters() learning.Parameters {
	return learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{.3, -.2, .45}, Bias: []float64{.1, -.1, .05}, LogTau: []float64{.1, .2, -.05}},
		Encoder: []float64{.5, .1, -.3},
		Readout: []float64{.8, -.4, .25, .6},
	}
}

func everyStepInput() [][]float64 {
	return [][]float64{{.7}, {-.2}, {.1}, {.4}, {-.6}}
}

func everyStepNetwork(t *testing.T, every bool) *learning.Network {
	t.Helper()
	n, err := learning.NewNetwork(everyStepConfig(every))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// TestReadoutEveryStepLastRowMatchesPredict pins that turning the per-step
// readout on does not move the last step: PredictAll's last row, Predict in
// the per-step mode and Predict in the last-step mode agree bit for bit.
func TestReadoutEveryStepLastRowMatchesPredict(t *testing.T) {
	ctx, p, input := context.Background(), everyStepParameters(), everyStepInput()
	every := everyStepNetwork(t, true)
	all, err := every.PredictAll(ctx, p, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != len(input) {
		t.Fatalf("PredictAll returned %d rows, want %d", len(all), len(input))
	}
	for i, row := range all {
		if len(row) != 2 {
			t.Fatalf("PredictAll row %d has width %d, want 2", i, len(row))
		}
	}
	last, err := every.Predict(ctx, p, input)
	if err != nil {
		t.Fatal(err)
	}
	for j := range last {
		if last[j] != all[len(all)-1][j] {
			t.Fatalf("Predict[%d] = %.17g, PredictAll last row = %.17g", j, last[j], all[len(all)-1][j])
		}
	}
	off, err := everyStepNetwork(t, false).Predict(ctx, p, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(off) != len(last) {
		t.Fatalf("last-step Predict returned %d values, want %d", len(off), len(last))
	}
	for j := range off {
		if off[j] != last[j] {
			t.Fatalf("Predict[%d] diverged between modes: %.17g != %.17g", j, off[j], last[j])
		}
	}
	// The intermediate rows must be real per-step readouts, not copies of the
	// last one, or the test above would pass on a broken implementation.
	same := true
	for j := range all[0] {
		if all[0][j] != all[len(all)-1][j] {
			same = false
		}
	}
	if same {
		t.Fatalf("PredictAll repeated the same row: %v", all)
	}
}

// TestReadoutEveryStepLossGradientMatchesLastStepMode pins that LossGradient
// keeps its meaning when the per-step readout is on: the MSE is still taken
// against the last row alone, so both the loss and every gradient entry must
// match the last-step model.
func TestReadoutEveryStepLossGradientMatchesLastStepMode(t *testing.T) {
	ctx, p, input := context.Background(), everyStepParameters(), everyStepInput()
	target := []float64{.4, -.2}
	everyLoss, everyGradient, err := everyStepNetwork(t, true).LossGradient(ctx, p, input, target, 0)
	if err != nil {
		t.Fatal(err)
	}
	offLoss, offGradient, err := everyStepNetwork(t, false).LossGradient(ctx, p, input, target, 0)
	if err != nil {
		t.Fatal(err)
	}
	if everyLoss != offLoss {
		t.Fatalf("loss diverged: %.17g != %.17g", everyLoss, offLoss)
	}
	var worst float64
	var where string
	for _, pair := range []struct {
		name     string
		got, ref []float64
	}{
		{"core.weights", everyGradient.Core.Weights, offGradient.Core.Weights},
		{"core.bias", everyGradient.Core.Bias, offGradient.Core.Bias},
		{"core.log_tau", everyGradient.Core.LogTau, offGradient.Core.LogTau},
		{"theta_raw", everyGradient.ThetaRaw, offGradient.ThetaRaw},
		{"encoder", everyGradient.Encoder, offGradient.Encoder},
		{"readout", everyGradient.Readout, offGradient.Readout},
	} {
		if len(pair.got) != len(pair.ref) {
			t.Fatalf("%s has %d values, want %d", pair.name, len(pair.got), len(pair.ref))
		}
		for i := range pair.ref {
			if d := math.Abs(pair.got[i] - pair.ref[i]); d > worst {
				worst, where = d, pair.name
			}
		}
	}
	for i := range offGradient.Inputs {
		for j := range offGradient.Inputs[i] {
			if d := math.Abs(everyGradient.Inputs[i][j] - offGradient.Inputs[i][j]); d > worst {
				worst, where = d, "inputs"
			}
		}
	}
	t.Logf("largest gradient difference between the two modes: %.17g (%s)", worst, where)
	if worst > 1e-12 {
		t.Fatalf("gradient difference %.17g exceeds 1e-12 at %s", worst, where)
	}
}

// everyStepWeights is the fixed, asymmetric weighting of the scalar objective
// J = sum_t sum_k w[t][k] * pred[t][k]. No row repeats and no row is zero, so
// a reverse pass that reads only the last step, or that mixes the rows up,
// fails against the finite difference.
func everyStepWeights() [][]float64 {
	return [][]float64{{1, -2}, {.5, .25}, {-1.5, .75}, {2, -.5}, {-.25, 1.25}}
}

// TestReadoutEveryStepGradientMatchesFiniteDifference checks the per-step
// reverse pass against a central difference of that objective.
func TestReadoutEveryStepGradientMatchesFiniteDifference(t *testing.T) {
	ctx, input, w := context.Background(), everyStepInput(), everyStepWeights()
	n := everyStepNetwork(t, true)
	p := everyStepParameters()
	objective := func() float64 {
		pred, err := n.PredictAll(ctx, p, input)
		if err != nil {
			t.Fatal(err)
		}
		var j float64
		for step := range pred {
			for k := range pred[step] {
				j += w[step][k] * pred[step][k]
			}
		}
		return j
	}
	g, err := n.LossGradientFrom(ctx, p, input, w, 0)
	if err != nil {
		t.Fatal(err)
	}
	// The core integrates in float64, but the encoder, the readout and the
	// prediction cross Insyra's float32 boundary, so the probe cannot be
	// arbitrarily small: a 1e-6 probe moves the prediction by about one float32
	// ulp and the difference quotient is then 0.5% noise. The probe below is
	// the one TestInsyraBridgeFullGradient already uses for the same reason,
	// where the quantization floor and the truncation error of the central
	// difference are both small. The comparison is relative, with a floor of 1
	// so that a near-zero gradient is compared absolutely.
	const h = 2e-3
	const tolerance = 1e-3
	var worst float64
	var where string
	for _, pair := range []struct {
		name     string
		value, g []float64
	}{
		{"core.weights", p.Core.Weights, g.Core.Weights},
		{"readout", p.Readout, g.Readout},
		{"encoder", p.Encoder, g.Encoder},
	} {
		if len(pair.g) != len(pair.value) {
			t.Fatalf("%s gradient has %d values, want %d", pair.name, len(pair.g), len(pair.value))
		}
		for i := range pair.value {
			old := pair.value[i]
			pair.value[i] = old + h
			plus := objective()
			pair.value[i] = old - h
			minus := objective()
			pair.value[i] = old
			fd := (plus - minus) / (2 * h)
			scale := math.Max(math.Abs(fd), math.Abs(pair.g[i]))
			if scale < 1 {
				scale = 1
			}
			if d := math.Abs(fd-pair.g[i]) / scale; d > worst {
				worst, where = d, pair.name
			}
			if math.Abs(fd-pair.g[i]) > tolerance*scale {
				t.Errorf("%s[%d] gradient %.17g, finite difference %.17g", pair.name, i, pair.g[i], fd)
			}
		}
	}
	t.Logf("largest relative finite-difference error: %.17g (%s)", worst, where)
}

// TestReadoutEveryStepOffRejectsPredictAll pins that the per-step readout is
// opt-in: a model that did not declare it refuses to invent one.
func TestReadoutEveryStepOffRejectsPredictAll(t *testing.T) {
	ctx, p, input := context.Background(), everyStepParameters(), everyStepInput()
	_, err := everyStepNetwork(t, false).PredictAll(ctx, p, input)
	if err == nil {
		t.Fatal("Network.PredictAll accepted a last-step model")
	}
	if !strings.Contains(err.Error(), "readout_every_step is off") {
		t.Fatalf("Network.PredictAll error %q does not name readout_every_step", err)
	}
	tr, err := learning.NewTrainer(everyStepConfig(false), p, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tr.PredictAll(ctx, input); err == nil {
		t.Fatal("Trainer.PredictAll accepted a last-step model")
	} else if !strings.Contains(err.Error(), "readout_every_step is off") {
		t.Fatalf("Trainer.PredictAll error %q does not name readout_every_step", err)
	}
	on, err := learning.NewTrainer(everyStepConfig(true), p, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	all, err := on.PredictAll(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != len(input) {
		t.Fatalf("Trainer.PredictAll returned %d rows, want %d", len(all), len(input))
	}
}

// TestReadoutEveryStepOffKeepsConfigJSON pins that the new field leaves the
// canonical JSON, and therefore every recorded configuration fingerprint,
// untouched while it is off.
func TestReadoutEveryStepOffKeepsConfigJSON(t *testing.T) {
	off, err := json.Marshal(everyStepConfig(false))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(off), "readout_every_step") {
		t.Fatalf("a disabled per-step readout reached the canonical JSON: %s", off)
	}
	on, err := json.Marshal(everyStepConfig(true))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(on), `"readout_every_step":true`) {
		t.Fatalf("an enabled per-step readout is missing from the canonical JSON: %s", on)
	}
	// The model's own Config copy round-trips the flag, so a snapshot that
	// declares it restores a per-step model rather than a last-step one.
	if !everyStepNetwork(t, true).Config().ReadoutEveryStep {
		t.Fatal("Network.Config dropped readout_every_step")
	}
	if everyStepNetwork(t, false).Config().ReadoutEveryStep {
		t.Fatal("Network.Config invented readout_every_step")
	}
}
