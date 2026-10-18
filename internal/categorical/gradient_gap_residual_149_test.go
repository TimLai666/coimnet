package categorical

import (
	"math"
	"testing"
)

func TestCategoricalGapResidual149(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  []uint64
		target int
		factor uint64
		want   []uint64
	}{
		{"winner_first", []uint64{0x40774910d52d3051, 0xc0774910d52d3052}, 0, 0x3ff0000000000000, []uint64{0x8000000000000001, 0x0000000000000001}},
		{"winner_last", []uint64{0xc0774910d52d3052, 0x40774910d52d3051}, 1, 0x3ff0000000000000, []uint64{0x0000000000000001, 0x8000000000000001}},
		{"maximum_factor", []uint64{0x40774910d52d3051, 0xc0774910d52d3052}, 0, 0x7fefffffffffffff, []uint64{0xbcc00000000000be, 0x3cc00000000000be}},
		{"ordinary_gap", []uint64{0x0000000000000000, 0xc044000000000000}, 0, 0x3ff0000000000000, []uint64{0xbc539792499b1a24, 0x3c539792499b1a24}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x := make([]float64, len(tc.input))
			for i, b := range tc.input {
				x[i] = math.Float64frombits(b)
			}
			g := make([]float64, len(x))
			if err := Gradient(g, x, tc.target, math.Float64frombits(tc.factor), 1); err != nil {
				t.Fatal(err)
			}
			for i, w := range tc.want {
				got := math.Float64bits(g[i])
				d := got - w
				if got < w {
					d = w - got
				}
				t.Logf("class=%d got=%016x want=%016x distance=%d", i, got, w, d)
				if (g[i] == 0 && math.Float64frombits(w) != 0) || math.Signbit(g[i]) != math.Signbit(math.Float64frombits(w)) || d > 4 {
					t.Errorf("exact-input gradient lost: class%d got%016x want%016x distance%d", i, got, w, d)
				}
				if math.Float64bits(x[i]) != tc.input[i] {
					t.Error("input mutated")
				}
			}
		})
	}
}
