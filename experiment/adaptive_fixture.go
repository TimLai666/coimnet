package experiment

import (
	"fmt"
	"math/rand/v2"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/plasticity"
)

// adaptiveFixtureItemCount is the fixed number of items each partition of the
// evaluate fixture carries.
const adaptiveFixtureItemCount = 8

// AdaptiveFixture builds the small delayed-correlation individual (2-node
// continuous core with local plasticity enabled on its edge) and 8+8
// deterministic items from seed, for the evaluate example and its tests.
// The adaptation and scoring partitions share the same eight seeded input
// sets, in the same order; only the ids, the allowed feedback and (in fixed
// mode) the presence of the adaptation partition differ. The returned
// declaration always passes Validate.
func AdaptiveFixture(seed uint64, mode string, reset ResetPolicy) (*learning.Individual, AdaptiveEvaluation, error) {
	ind, err := adaptiveFixtureIndividual()
	if err != nil {
		return nil, AdaptiveEvaluation{}, err
	}
	adaptation, scoring := adaptiveFixtureItems(seed)
	if mode != EvaluationModeAdaptive {
		adaptation = nil
	}
	e := AdaptiveEvaluation{
		Mode:              mode,
		Adaptation:        adaptation,
		Scoring:           scoring,
		FeedbackAvailable: []string{"score"},
		Reset:             reset,
		Seed:              seed,
	}
	if err := e.Validate(); err != nil {
		return nil, AdaptiveEvaluation{}, err
	}
	return ind, e, nil
}

// adaptiveFixtureIndividual reproduces the 2-node continuous fixture of the
// adaptive evaluation tests, with local rate plasticity enabled on its single
// edge.
func adaptiveFixtureIndividual() (*learning.Individual, error) {
	config := learning.Config{
		Dynamics:     dynamics.Config{Nodes: 2, DT: 0.5, Activation: "tanh", Sources: []int{0}, Targets: []int{1}},
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: []int{1},
	}
	params := learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{.3}, Bias: []float64{.1, -.1}, LogTau: []float64{.1, .2}},
		Encoder: []float64{.5, .1},
		Readout: []float64{.8},
	}
	ind, err := learning.NewIndividual(config, params, learning.DefaultOptions(), make([]float64, config.Dynamics.Nodes))
	if err != nil {
		return nil, err
	}
	rule := plasticity.Rule{
		Kind:       plasticity.RuleHebbianRate,
		DecayE:     0.5,
		DecayP:     0.5,
		PlasticMax: 1,
		WMin:       0.01,
	}
	if err := ind.EnablePlasticity(plasticity.Config{Rule: rule, Edges: []int{0}}); err != nil {
		return nil, err
	}
	return ind, nil
}

// adaptiveFixtureItems draws adaptiveFixtureItemCount deterministic value sets
// from the seeded PCG generator; each set is one 3x1 input in [-1,1] with a
// target of half the last row. The adaptation partition allows score feedback
// and the scoring partition allows none.
func adaptiveFixtureItems(seed uint64) ([]EvaluationItem, []EvaluationItem) {
	rng := rand.New(rand.NewPCG(seed, 0))
	adaptation := make([]EvaluationItem, adaptiveFixtureItemCount)
	scoring := make([]EvaluationItem, adaptiveFixtureItemCount)
	for i := 0; i < adaptiveFixtureItemCount; i++ {
		input := [][]float64{
			{rng.Float64()*2 - 1},
			{rng.Float64()*2 - 1},
			{rng.Float64()*2 - 1},
		}
		adaptation[i] = EvaluationItem{
			ID:              fmt.Sprintf("adapt-%d", i),
			Input:           input,
			Target:          []float64{0.5 * input[2][0]},
			AllowedFeedback: []string{"score"},
		}
		scoring[i] = EvaluationItem{
			ID:     fmt.Sprintf("score-%d", i),
			Input:  input,
			Target: []float64{0.5 * input[2][0]},
		}
	}
	return adaptation, scoring
}
