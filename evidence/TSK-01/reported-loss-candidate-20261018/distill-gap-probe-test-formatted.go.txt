package distill_test

import (
	"github.com/TimLai666/coimnet/distill"
	"math"
	"testing"
)

func TestDistillGapResidual148(t *testing.T) {
	for _, tc := range []struct {
		name   string
		bits   []uint64
		target int
	}{{"winner_first", []uint64{0x40774910d52d3051, 0xc0774910d52d3052}, 0}, {"winner_last", []uint64{0xc0774910d52d3052, 0x40774910d52d3051}, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			x := make([]float64, 2)
			for i, b := range tc.bits {
				x[i] = math.Float64frombits(b)
			}
			d := distill.DistributionDistiller{Temperature: 2, Scale: 3, Mix: 0, Alignment: distill.Alignment{Rule: distill.AlignmentIdentical, TeacherVocabHash: "gap148", StudentVocabHash: "gap148"}}
			loss, g, r, err := d.Loss(distill.TeacherDistribution{Probabilities: []float64{1, 0}}, x, tc.target)
			if err != nil {
				t.Fatal(err)
			}
			if len(g) != 2 || r.Mix != 0 {
				t.Fatalf("report/gradient shape changed: %v %+v", g, r)
			}
			for i, b := range tc.bits {
				if math.Float64bits(x[i]) != b {
					t.Fatal("input mutated")
				}
			}
			if math.Float64bits(loss) != 1 {
				t.Fatalf("public label Loss=%016x, want smallest positive float64=1", math.Float64bits(loss))
			}
		})
	}
}
