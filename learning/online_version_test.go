package learning

import (
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
)

// TestSameParametersSeesEveryGroup changes one value in each parameter group
// in turn, gate groups included, and requires the cached version to miss.
func TestSameParametersSeesEveryGroup(t *testing.T) {
	base := func() Parameters {
		return Parameters{
			Core:     dynamics.Parameters{Weights: []float64{1}, Bias: []float64{2}, LogTau: []float64{3}, GateGain: []float64{4}, GateBias: []float64{5}},
			ThetaRaw: []float64{6},
			Encoder:  []float64{7},
			Readout:  []float64{8},
		}
	}
	if !sameParameters(base(), base()) {
		t.Fatal("equal parameters reported different")
	}
	groups := map[string]func(*Parameters) []float64{
		"weights":   func(p *Parameters) []float64 { return p.Core.Weights },
		"bias":      func(p *Parameters) []float64 { return p.Core.Bias },
		"log_tau":   func(p *Parameters) []float64 { return p.Core.LogTau },
		"gate_gain": func(p *Parameters) []float64 { return p.Core.GateGain },
		"gate_bias": func(p *Parameters) []float64 { return p.Core.GateBias },
		"theta_raw": func(p *Parameters) []float64 { return p.ThetaRaw },
		"encoder":   func(p *Parameters) []float64 { return p.Encoder },
		"readout":   func(p *Parameters) []float64 { return p.Readout },
	}
	for name, group := range groups {
		changed := base()
		group(&changed)[0] += 1
		if sameParameters(base(), changed) {
			t.Fatalf("a change to %s was not seen", name)
		}
	}
}
