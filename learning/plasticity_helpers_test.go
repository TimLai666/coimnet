package learning_test

import (
	"context"
	"math"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/plasticity"
)

// plasticLIFCore is the two-neuron fixture the hand computation uses: neuron 0
// receives the encoded observation, one instantaneous edge carries its synaptic
// trace to neuron 1, and neuron 1 is the readout. dt equals both time
// constants, so lambda = alpha' = kappa = exp(-1) and every quantity below can
// be written out in closed form.
func plasticLIFCore() dynamics.LIFConfig {
	return dynamics.LIFConfig{
		Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{0},
		DT: 1, TauSyn: 1, ThetaMin: .05, ThetaMax: 1, VReset: -.5, RefractorySteps: 0,
		Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
	}
}

func plasticLIFConfig() learning.Config {
	core := plasticLIFCore()
	return learning.Config{LIF: &core, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{1}, InputNodes: []int{0}}
}

// plasticLIFParameters keeps the encoder and the readout at identity, so the
// only float32 rounding left is the readout boundary. theta_raw -2 puts the
// base threshold at 0.05 + 0.95*sigmoid(-2) = 0.16324277592101166, which is
// above the drive neuron 1 receives at step 4 through the base weight 0.9 and
// below the drive it receives once a fast change has raised that weight.
func plasticLIFParameters() learning.Parameters {
	return learning.Parameters{
		Core:     dynamics.Parameters{Weights: []float64{.9}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}},
		ThetaRaw: []float64{-2, -2},
		Encoder:  []float64{1},
		Readout:  []float64{1},
	}
}

// plasticLIFInput drives neuron 0 at steps 0 and 2 only.
func plasticLIFInput() [][]float64 { return [][]float64{{1.5}, {0}, {1.5}, {0}, {0}} }

// plasticRule is the rate rule of root decision 1 with halving decays, a bound
// no test reaches by accident and a floor well below the fixture weights.
func plasticRule() plasticity.Rule {
	return plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 8, WMin: .0625}
}

func plasticConfig(edges ...int) plasticity.Config {
	return plasticity.Config{Rule: plasticRule(), Edges: edges}
}

func newIndividual(t *testing.T, c learning.Config, p learning.Parameters) *learning.Individual {
	t.Helper()
	nodes := c.Dynamics.Nodes
	if c.LIF != nil {
		nodes = c.LIF.Nodes
	}
	a, err := learning.NewIndividual(c, p, learning.DefaultOptions(), make([]float64, nodes))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func advance(t *testing.T, a *learning.Individual, input [][]float64) [][]float64 {
	t.Helper()
	out, err := a.Advance(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func advanceGated(t *testing.T, a *learning.Individual, input [][]float64, gate []float64) ([][]float64, learning.PlasticReport) {
	t.Helper()
	out, report, err := a.AdvanceGated(context.Background(), input, gate)
	if err != nil {
		t.Fatal(err)
	}
	return out, report
}

func sameRows(got, want [][]float64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if len(got[i]) != len(want[i]) {
			return false
		}
		for j := range got[i] {
			if got[i][j] != want[i][j] {
				return false
			}
		}
	}
	return true
}

// closeEnough compares against a hand-computed constant. The reference values
// are exact decimal expansions of the closed-form result, so the tolerance only
// absorbs the last bits of exp and expm1.
func closeEnough(got, want float64) bool { return math.Abs(got-want) <= 1e-12 }
