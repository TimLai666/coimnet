package learning_test

import (
	"context"
	"math"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

// readoutC2Config reuses the TestVectorCoreC2GradientMatchesFiniteDifference
// fixture with per-step readout turned on, so this test differentiates the same
// two-node, C=2 matrix chain under the ReadoutEveryStep contract.
func readoutC2Config() learning.Config {
	c := vectorC2Config()
	c.ReadoutEveryStep = true
	return c
}

// TestVectorCoreReadoutEveryStepFiniteDifference checks the whole training
// graph in the per-step readout mode: LossGradientFrom seeds a caller-supplied
// upstream row per step (no row repeats, no row is zero), and every learnable
// group the upstream touches -- core weights, core bias, encoder and readout --
// must match a central difference of the same objective.
func TestVectorCoreReadoutEveryStepFiniteDifference(t *testing.T) {
	network, err := learning.NewNetwork(readoutC2Config())
	if err != nil {
		t.Fatal(err)
	}
	parameters := vectorC2Parameters()
	input := [][]float64{{1}, {.5}}
	upstream := [][]float64{{.8}, {-.3}}
	gradient, err := network.LossGradientFrom(context.Background(), parameters, input, upstream, 0)
	if err != nil {
		t.Fatal(err)
	}
	objective := func() float64 {
		pred, err := network.PredictAll(context.Background(), parameters, input)
		if err != nil {
			t.Fatal(err)
		}
		var j float64
		for step := range pred {
			for k := range pred[step] {
				j += upstream[step][k] * pred[step][k]
			}
		}
		return j
	}
	const h = 1e-3
	worst, where := 0.0, ""
	check := func(name string, values, grads []float64) {
		t.Helper()
		if len(grads) != len(values) {
			t.Fatalf("%s gradient has %d values, want %d", name, len(grads), len(values))
		}
		for i := range values {
			old := values[i]
			values[i] = old + h
			plus := objective()
			values[i] = old - h
			minus := objective()
			values[i] = old
			fd := (plus - minus) / (2 * h)
			scale := math.Max(math.Abs(fd), math.Abs(grads[i]))
			if scale < 1 {
				scale = 1
			}
			if d := math.Abs(fd-grads[i]) / scale; d > worst {
				worst, where = d, name
			}
			if math.Abs(fd-grads[i]) > 1e-3*scale {
				t.Errorf("%s[%d] gradient %.17g, finite difference %.17g", name, i, grads[i], fd)
			}
		}
	}
	check("core.weights", parameters.Core.Weights, gradient.Core.Weights)
	check("core.bias", parameters.Core.Bias, gradient.Core.Bias)
	check("encoder", parameters.Encoder, gradient.Encoder)
	check("readout", parameters.Readout, gradient.Readout)
	t.Logf("vector core C=2 readout-every-step worst relative finite-difference error: %.3g (%s)", worst, where)
}
