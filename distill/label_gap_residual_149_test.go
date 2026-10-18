package distill_test

import (
	"github.com/TimLai666/coimnet/distill"
	"math"
	"testing"
)

func TestDistillGapResidual149(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  []uint64
		target int
		want   []uint64
	}{
		{"winner_first", []uint64{0x40774910d52d3051, 0xc0774910d52d3052}, 0, []uint64{0x8000000000000001, 0x0000000000000001}},
		{"winner_last", []uint64{0xc0774910d52d3052, 0x40774910d52d3051}, 1, []uint64{0x0000000000000001, 0x8000000000000001}},
		{"ordinary_gap", []uint64{0x0000000000000000, 0xc044000000000000}, 0, []uint64{0xbc539792499b1a24, 0x3c539792499b1a24}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x := make([]float64, len(tc.input))
			for i, b := range tc.input {
				x[i] = math.Float64frombits(b)
			}
			d := distill.DistributionDistiller{Temperature: 2, Scale: 3, Mix: 0, Alignment: distill.Alignment{Rule: distill.AlignmentIdentical, TeacherVocabHash: "gap149", StudentVocabHash: "gap149"}}
			loss, g, report, err := d.Loss(distill.TeacherDistribution{Probabilities: []float64{1, 0}}, x, tc.target)
			if err != nil {
				t.Fatal(err)
			}
			if !(loss > 0) || len(g) != len(tc.want) || report.Mix != 0 {
				t.Fatalf("invalid public result: loss=%g gradient=%v report=%+v", loss, g, report)
			}
			for i, w := range tc.want {
				got := math.Float64bits(g[i])
				distance := got - w
				if got < w {
					distance = w - got
				}
				t.Logf("class=%d got=%016x want=%016x distance=%d", i, got, w, distance)
				if (g[i] == 0 && math.Float64frombits(w) != 0) || math.Signbit(g[i]) != math.Signbit(math.Float64frombits(w)) || distance > 4 {
					t.Errorf("public label gradient lost: class%d got%016x want%016x distance%d", i, got, w, distance)
				}
				if math.Float64bits(x[i]) != tc.input[i] {
					t.Error("input mutated")
				}
			}
		})
	}
}
