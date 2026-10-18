package categorical

import (
	"math"
	"testing"
)

func TestCategoricalGapResidual148(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  []uint64
		target int
		factor uint64
		want   uint64
	}{
		{"winner_first", []uint64{0x40774910d52d3051, 0xc0774910d52d3052}, 0, 0x3ff0000000000000, 0x0000000000000001},
		{"winner_last", []uint64{0xc0774910d52d3052, 0x40774910d52d3051}, 1, 0x3ff0000000000000, 0x0000000000000001},
		{"maximum_factor", []uint64{0x40774910d52d3051, 0xc0774910d52d3052}, 0, 0x7fefffffffffffff, 0x3cc00000000000be},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x := make([]float64, len(tc.input))
			for i, b := range tc.input {
				x[i] = math.Float64frombits(b)
			}
			got, err := Loss(x, tc.target, math.Float64frombits(tc.factor))
			if err != nil {
				t.Fatal(err)
			}
			g := math.Float64bits(got)
			d := g - tc.want
			if g < tc.want {
				d = tc.want - g
			}
			t.Logf("got=%016x want=%016x distance=%d", g, tc.want, d)
			if !(got > 0) || d > 4 {
				t.Fatalf("exact-input reference lost: got %016x want %016x distance %d", g, tc.want, d)
			}
		})
	}
}
