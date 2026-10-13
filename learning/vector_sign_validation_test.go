package learning_test

import (
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

func vectorSignValidationConfig(edgeShape string, signs []int8) learning.Config {
	return learning.Config{
		Dynamics: dynamics.Config{
			Nodes:          2,
			Sources:        []int{0},
			Targets:        []int{1},
			Delays:         []int{0},
			DT:             .5,
			Activation:     "tanh",
			StateDimension: 2,
			EdgeShape:      edgeShape,
		},
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: []int{1},
		EdgeSigns:    signs,
	}
}

func TestNewNetworkRejectsVectorFixedSignsAtConstruction(t *testing.T) {
	for _, edgeShape := range []string{"scalar", "matrix"} {
		cfg := vectorSignValidationConfig(edgeShape, []int8{1})
		if _, err := learning.NewNetwork(cfg); err == nil {
			t.Errorf("NewNetwork accepted a nonzero fixed sign for vector %s edges", edgeShape)
		}
	}
}

func TestNewNetworkKeepsLegalSignDeclarations(t *testing.T) {
	scalar := vectorSignValidationConfig("", nil)
	scalar.Dynamics.StateDimension = 0
	scalar.EdgeSigns = []int8{1}

	cases := map[string]learning.Config{
		"vector scalar without signs":  vectorSignValidationConfig("scalar", nil),
		"vector matrix without signs":  vectorSignValidationConfig("matrix", nil),
		"vector scalar all zero signs": vectorSignValidationConfig("scalar", []int8{0}),
		"vector matrix all zero signs": vectorSignValidationConfig("matrix", []int8{0}),
		"scalar fixed sign":            scalar,
	}
	for name, cfg := range cases {
		if _, err := learning.NewNetwork(cfg); err != nil {
			t.Errorf("%s: NewNetwork() error = %v", name, err)
		}
	}
}
