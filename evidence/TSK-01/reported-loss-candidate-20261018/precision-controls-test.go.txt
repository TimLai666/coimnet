package categorical

import (
	"math"
	"testing"
)

func TestUnderflowBranchControls149(t *testing.T) {
	for _, tc := range []HalfMinimum149Case{
		{name: "minimum_factor_equal3", logitsBits: []uint64{0x0000000000000000, 0x0000000000000000, 0x0000000000000000}, target: 2, factorBits: 0x0000000000000001, divisorBits: 0x3ff0000000000000, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000000, 0x0000000000000000, 0x8000000000000001}},
		{name: "minimum_factor_equal4", logitsBits: []uint64{0x0000000000000000, 0x0000000000000000, 0x0000000000000000, 0x0000000000000000}, target: 3, factorBits: 0x0000000000000001, divisorBits: 0x3ff0000000000000, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000000, 0x0000000000000000, 0x0000000000000000, 0x8000000000000001}},
		{name: "maximum_factor_rescue_-1_first", logitsBits: []uint64{0xbd3d6c77aa067899, 0xc096bba9ea13b521}, target: 0, factorBits: 0x7fefffffffffffff, divisorBits: 0x3ff0000000000000, lossBits: 0x0000000000000001, gradientBits: []uint64{0x8000000000000001, 0x0000000000000001}},
		{name: "maximum_factor_rescue_-1_last", logitsBits: []uint64{0xc096bba9ea13b521, 0xbd3d6c77aa067899}, target: 1, factorBits: 0x7fefffffffffffff, divisorBits: 0x3ff0000000000000, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000001, 0x8000000000000001}},
		{name: "maximum_factor_rescue_+0_first", logitsBits: []uint64{0xbd3d6c77aa067898, 0xc096bba9ea13b521}, target: 0, factorBits: 0x7fefffffffffffff, divisorBits: 0x3ff0000000000000, lossBits: 0x0000000000000001, gradientBits: []uint64{0x8000000000000001, 0x0000000000000001}},
		{name: "maximum_factor_rescue_+0_last", logitsBits: []uint64{0xc096bba9ea13b521, 0xbd3d6c77aa067898}, target: 1, factorBits: 0x7fefffffffffffff, divisorBits: 0x3ff0000000000000, lossBits: 0x0000000000000001, gradientBits: []uint64{0x0000000000000001, 0x8000000000000001}},
		{name: "maximum_factor_rescue_+1_first", logitsBits: []uint64{0xbd3d6c77aa067897, 0xc096bba9ea13b521}, target: 0, factorBits: 0x7fefffffffffffff, divisorBits: 0x3ff0000000000000, lossBits: 0x0000000000000000, gradientBits: []uint64{0x8000000000000000, 0x0000000000000000}},
		{name: "maximum_factor_rescue_+1_last", logitsBits: []uint64{0xc096bba9ea13b521, 0xbd3d6c77aa067897}, target: 1, factorBits: 0x7fefffffffffffff, divisorBits: 0x3ff0000000000000, lossBits: 0x0000000000000000, gradientBits: []uint64{0x0000000000000000, 0x8000000000000000}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logits := make([]float64, len(tc.logitsBits))
			for i, b := range tc.logitsBits {
				logits[i] = math.Float64frombits(b)
			}
			loss, err := Loss(logits, tc.target, math.Float64frombits(tc.factorBits))
			if err != nil {
				t.Fatal(err)
			}
			g := make([]float64, len(logits))
			if err := Gradient(g, logits, tc.target, math.Float64frombits(tc.factorBits), 1); err != nil {
				t.Fatal(err)
			}
			CheckHalfMinimum149Loss(t, tc.name, loss, tc.lossBits)
			CheckHalfMinimum149Gradient(t, tc.name, g, tc.gradientBits)
			for i, b := range tc.logitsBits {
				if math.Float64bits(logits[i]) != b {
					t.Error("input mutated")
				}
			}
		})
	}
}
