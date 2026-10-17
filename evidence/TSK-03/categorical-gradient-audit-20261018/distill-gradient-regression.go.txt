package distill_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/distill"
)

func TestDistributionLabelGradient147(t *testing.T) {
	const (
		// These are the frozen float64 conversions of the independent Decimal180
		// three-class pure-label references. Mix=0 disables KL, so Temperature and
		// Scale do not multiply these CE derivatives.
		wantTarget = -0x1.39792499b1a24p-57
		wantOther  = 0x1.39792499b1a24p-58
	)

	d := distill.DistributionDistiller{
		Temperature: 2,
		Scale:       3,
		Mix:         0,
		Alignment: distill.Alignment{
			Rule:             distill.AlignmentIdentical,
			TeacherVocabHash: "audit147",
			StudentVocabHash: "audit147",
		},
	}

	for _, tc := range []struct {
		name   string
		target int
	}{
		{name: "winner_0", target: 0},
		{name: "winner_1", target: 1},
		{name: "winner_2", target: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			student := []float64{-40, -40, -40}
			student[tc.target] = 0
			studentBefore := append([]float64(nil), student...)
			teacher := distill.TeacherDistribution{Probabilities: []float64{1, 0, 0}}
			teacherBefore := distill.TeacherDistribution{
				Probabilities: append([]float64(nil), teacher.Probabilities...),
				Classes:       append([]int(nil), teacher.Classes...),
			}

			loss, gradient, report, err := d.Loss(teacher, student, tc.target)
			if err != nil {
				t.Fatalf("Loss = %.17g, gradient=%v, report=%+v, err=%v", loss, gradient, report, err)
			}
			if math.IsNaN(loss) || math.IsInf(loss, 0) || loss < 0 {
				t.Errorf("loss=%.17g, want finite nonnegative value", loss)
			}
			if len(gradient) != len(student) {
				t.Fatalf("gradient length=%d, want %d", len(gradient), len(student))
			}
			if report.Temperature != d.Temperature || report.Scale != d.Scale || report.Mix != d.Mix || report.TopK != d.TopK || report.Partial {
				t.Errorf("report=%+v, want parameters temperature=%g scale=%g mix=%g top_k=%d partial=false", report, d.Temperature, d.Scale, d.Mix, d.TopK)
			}
			if !reflect.DeepEqual(report.Alignment, d.Alignment) {
				t.Errorf("report alignment=%+v, want %+v", report.Alignment, d.Alignment)
			}
			for i := range student {
				if math.Float64bits(student[i]) != math.Float64bits(studentBefore[i]) {
					t.Errorf("student[%d]=%.17g, want unchanged %.17g", i, student[i], studentBefore[i])
				}
			}
			if !reflect.DeepEqual(teacher, teacherBefore) {
				t.Errorf("teacher=%+v, want unchanged %+v", teacher, teacherBefore)
			}

			for class, got := range gradient {
				if math.IsNaN(got) || math.IsInf(got, 0) {
					t.Errorf("gradient[%d]=%.17g, want finite", class, got)
					continue
				}
				want := wantOther
				if class == tc.target {
					want = wantTarget
				}
				if class == tc.target {
					if got >= 0 {
						t.Errorf("gradient[%d] target=true = %.17g, want negative", class, got)
						continue
					}
				} else if got <= 0 {
					t.Errorf("gradient[%d] target=false = %.17g, want positive", class, got)
					continue
				}
				// Sign is checked before the bit distance so zero or opposite-sign
				// values cannot produce a misleading ULP diagnostic.
				gotBits, wantBits := math.Float64bits(got), math.Float64bits(want)
				var ulp uint64
				if gotBits > wantBits {
					ulp = gotBits - wantBits
				} else {
					ulp = wantBits - gotBits
				}
				if ulp > 4 {
					t.Errorf("gradient[%d] target=%t = %.17g, want %.17g within 4 ULP; got_bits=0x%016x want_bits=0x%016x ulp=%d", class, class == tc.target, got, want, gotBits, wantBits, ulp)
				}
			}
		})
	}
}
